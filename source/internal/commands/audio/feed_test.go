// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package audio

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/opusenc"
	"github.com/albeebe/radiocli/internal/portlock"
)

// daemon is a fake radiocli daemon: how it greets a connection, how it answers
// the request for audio, and what it sends afterwards.
//
// It writes all three without waiting to be asked, which the client is happy
// with because it reads the greeting, the answer and the frames from one
// buffered reader. That keeps the fake to a single write and no state.
type daemon struct {
	// hello is the greeting, sent as the first line.
	hello broker.Response

	// reply is the answer to the request for audio, sent as the second line.
	reply broker.Response

	// tail is everything after the second line, already framed.
	tail []byte

	// hangUp closes the connection once tail has been sent, which is how a
	// stream that ends is told from one that is merely quiet.
	hangUp bool
}

// audioFrame builds one frame of the daemon's framing: the kind, the length in
// three bytes, then the payload.
//
// Parameters:
//   - kind: broker.FrameAudio, broker.FrameJSON, or something neither end knows
//   - payload: the bytes the frame carries
//
// Returns:
//   - the frame as it travels on the wire
func audioFrame(kind byte, payload []byte) []byte {
	frame := []byte{kind, byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))}
	return append(frame, payload...)
}

// audioPacket builds an audio frame, which carries its frame number in front of
// the audio.
//
// Parameters:
//   - seq: the frame number to put in front of the audio
//   - audio: the encoded audio the frame carries
//
// Returns:
//   - the frame as it travels on the wire
func audioPacket(seq uint32, audio []byte) []byte {
	payload := make([]byte, 4, 4+len(audio))
	binary.BigEndian.PutUint32(payload, seq)
	return audioFrame(broker.FrameAudio, append(payload, audio...))
}

// dialAudio stands up d for port and returns a stream connected to it, which is
// the only way to build a broker.AudioStream from outside that package.
//
// Parameters:
//   - t: the test the listener and the stream are cleaned up at the end of
//   - port: the serial port the daemon is holding
//   - d: the daemon to stand up
//
// Returns:
//   - a stream carrying whatever d was told to send
func dialAudio(t *testing.T, port string, d daemon) *broker.AudioStream {
	t.Helper()
	d.serve(t, port)

	stream, err := broker.DialAudio(port, formatPCM, defaultBitrate)
	if err != nil {
		t.Fatalf("asking the daemon for audio: %v", err)
	}
	t.Cleanup(func() { stream.Close() })
	return stream
}

// fakeCapture is an open sound card that never was, so feedDirect can be run
// without opening one and without the microphone permission prompt that opening
// one raises.
type fakeCapture struct {
	// source is what Source reports the audio is coming from.
	source string
}

// Close stops a recording that never started.
func (c fakeCapture) Close() {}

// Source is what the operating system would call the input.
func (c fakeCapture) Source() string { return c.source }

// fakeStart installs a sound card opener that answers with what it was given,
// and restores the real one at the end of the test.
//
// Parameters:
//   - t: the test the real opener is restored at the end of
//   - source: the name the capture reports it is recording from
//   - err: the failure to report instead, if there is one
func fakeStart(t *testing.T, source string, err error) {
	t.Helper()
	original := startCapture
	t.Cleanup(func() { startCapture = original })
	startCapture = func(opts audiofeed.Options, out audiofeed.Publisher) (capture, error) {
		if err != nil {
			return nil, err
		}
		return fakeCapture{source: source}, nil
	}
}

// hello is the greeting a daemon of this build sends.
//
// Returns:
//   - a greeting the client accepts
func hello() broker.Response {
	return broker.Response{Type: broker.TypeHello, Protocol: broker.Version, Version: "test"}
}

// serve listens on the socket a daemon for port would use and speaks to
// whoever connects.
//
// Parameters:
//   - t: the test the listener is cleaned up at the end of
//   - port: the serial port the daemon is holding
func (d daemon) serve(t *testing.T, port string) {
	t.Helper()

	path := portlock.SocketPath(port)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making the socket directory: %v", err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listening on %s: %v", path, err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()

				for _, msg := range []broker.Response{d.hello, d.reply} {
					if msg.Type == "" {
						continue
					}
					line, _ := json.Marshal(msg)
					conn.Write(append(line, '\n'))
				}
				conn.Write(d.tail)

				// The request for audio is read before anything hangs up,
				// because a client whose request lands on a closed socket sees
				// a broken pipe rather than the answer already waiting for it.
				bufio.NewReader(conn).ReadBytes('\n')

				if d.hangUp {
					return
				}

				// Held open until the caller hangs up, so a stream that is
				// merely quiet is never mistaken for one that ended.
				io.Copy(io.Discard, conn)
			}(conn)
		}
	}()
}

// sockets points the daemon socket directory at a temporary one of its own.
//
// Nothing here may reach a daemon the developer is actually running, and
// nothing may leave a socket behind. The base is /tmp rather than the default
// temporary directory because macOS refuses a unix socket path much over 100
// bytes and the default is most of that by itself.
//
// Parameters:
//   - t: the test whose temporary directory the sockets are made in
func sockets(t *testing.T) {
	t.Helper()
	t.Setenv("TMPDIR", "/tmp")
	t.Setenv("TMPDIR", t.TempDir())
}

// Test_newFeed tests the newFeed function with 100% coverage.
//
// Coverage: 100% (2 test cases covering the command and the closure it holds)
//
// Test cases:
//   - Wiring: the command carries its name and its four flags, with defaults
//   - Runs: executing the command reaches runFeed, which refuses it in a daemon
func Test_newFeed(t *testing.T) {
	// Verify that the command and its flags are described the way the tool wires them
	t.Run("Wiring", func(t *testing.T) {
		cmd := newFeed(appcontext.New())

		if cmd.Use != "feed" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "feed")
		}

		defaults := map[string]string{
			"input":   "",
			"format":  formatPCM,
			"bitrate": "32000",
			"channel": audiofeed.ChannelAuto,
		}
		for name, want := range defaults {
			flag := cmd.Flags().Lookup(name)
			if flag == nil {
				t.Errorf("the command has no --%s flag, wanted one", name)
				continue
			}
			if flag.DefValue != want {
				t.Errorf("--%s defaults to %q, wanted %q", name, flag.DefValue, want)
			}
		}
	})

	// Verify that running the command reaches runFeed, which is what the
	// closure newFeed hands cobra exists to do
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := newApp()
		app.InDaemon = true

		cmd := newFeed(app)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), "cannot be run inside a daemon") {
			t.Errorf("the failure is %v, wanted the daemon to have refused it", err)
		}
	})
}

// Test_announce tests the announce function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both formats)
//
// Test cases:
//   - Opus: the packet framing and the rate are described
//   - PCM: the player's own flags are printed
func Test_announce(t *testing.T) {
	// Verify that Opus is described as something for a program to read
	t.Run("Opus", func(t *testing.T) {
		app, out, notes := newApp()

		announce(app, "Cubilux CB5 Line In", formatOpus, 48000)

		if !strings.Contains(notes.String(), "as Opus at 48 kbps") {
			t.Errorf("the announcement is %q, wanted the rate in kbps", notes.String())
		}
		if !strings.Contains(notes.String(), "Cubilux CB5 Line In") {
			t.Errorf("the announcement is %q, wanted the sound input named", notes.String())
		}
		if out.String() != "" {
			t.Errorf("the audio stream is %q, wanted nothing but audio in it", out.String())
		}
	})

	// Verify that raw samples come with the flags a player needs
	t.Run("PCM", func(t *testing.T) {
		app, _, notes := newApp()

		announce(app, "Cubilux CB5 Line In", formatPCM, 0)

		if !strings.Contains(notes.String(), "ffplay -f s16le -ar 48000 -ac 1 -i -") {
			t.Errorf("the announcement is %q, wanted the player's flags", notes.String())
		}
	})
}

// Test_feedDirect tests the feedDirect function with 100% coverage.
//
// Coverage: 100% (3 test cases covering the recording and both failures)
//
// Test cases:
//   - Success: the capture is announced and the audio is written until stopped
//   - StartError: a sound input that cannot be opened is reported
//   - WriteError: a failure once the audio is flowing is reported
func Test_feedDirect(t *testing.T) {
	// Verify that a capture which opened is announced and then written out
	t.Run("Success", func(t *testing.T) {
		app, _, notes := newApp()
		fakeStart(t, "Cubilux CB5 Line In", nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := feedDirect(ctx, app, "Cubilux CB5 Line In", formatPCM, defaultBitrate, audiofeed.ChannelMix)
		if err != nil {
			t.Fatalf("listening: %v", err)
		}
		if !strings.Contains(notes.String(), "Cubilux CB5 Line In") {
			t.Errorf("the announcement is %q, wanted the sound input named", notes.String())
		}
	})

	// Verify that a sound input which cannot be opened is passed back
	t.Run("StartError", func(t *testing.T) {
		app, _, _ := newApp()
		fakeStart(t, "", errors.New("the sound input is gone"))

		err := feedDirect(context.Background(), app, "Nothing", formatPCM, defaultBitrate, audiofeed.ChannelMix)
		if err == nil || !strings.Contains(err.Error(), "the sound input is gone") {
			t.Errorf("the failure is %v, wanted the sound card's own error", err)
		}
	})

	// Verify that a failure once the audio is flowing is passed back
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		fakeStart(t, "Cubilux CB5 Line In", nil)

		err := feedDirect(context.Background(), app, "Cubilux CB5 Line In", formatOpus, 1, audiofeed.ChannelMix)
		if err == nil {
			t.Error("the recording succeeded, wanted the encoder to have refused the rate")
		}
	})
}

// Test_feedViaDaemon tests the feedViaDaemon function with 100% coverage.
//
// Coverage: 100% (4 test cases covering the relay and every way it is refused)
//
// Test cases:
//   - Success: the stream is announced and relayed until the daemon stops
//   - NoDevice: not naming a scanner is refused with what to do instead
//   - NoDaemon: nothing holding the scanner is answered with how to start one
//   - DialError: any other refusal is passed back as it came
func Test_feedViaDaemon(t *testing.T) {
	// Verify that a daemon holding the audio is announced and relayed
	t.Run("Success", func(t *testing.T) {
		sockets(t)
		app, out, notes := newApp()
		app.Config.Device = "/dev/tty.usbmodem1"

		d := daemon{
			hello: hello(),
			reply: broker.Response{
				Type: broker.TypeAudio, Format: formatPCM, Rate: 48000, Channels: 1,
				Source: "Cubilux CB5 Line In", Channel: audiofeed.ChannelLeft,
			},
			tail:   audioPacket(1, []byte{1, 2, 3, 4}),
			hangUp: true,
		}
		d.serve(t, "/dev/tty.usbmodem1")

		err := feedViaDaemon(context.Background(), app, formatPCM, defaultBitrate)
		if err == nil || !strings.Contains(err.Error(), "the daemon stopped sending audio") {
			t.Fatalf("the failure is %v, wanted the daemon hanging up to be reported", err)
		}
		if !strings.Contains(notes.String(), "Cubilux CB5 Line In") {
			t.Errorf("the announcement is %q, wanted the daemon's sound input named", notes.String())
		}
		if out.String() != "\x01\x02\x03\x04" {
			t.Errorf("the audio is %q, wanted the four bytes the daemon sent", out.String())
		}
	})

	// Verify that not naming a scanner is refused with the two ways forward
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()

		err := feedViaDaemon(context.Background(), app, formatPCM, defaultBitrate)
		if err == nil || !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the failure is %v, wanted it to be ErrNoDevice", err)
		}
		if !strings.Contains(err.Error(), "--input") {
			t.Errorf("the failure is %v, wanted --input offered as the other way", err)
		}
	})

	// Verify that no daemon at all is answered with the command to start one
	t.Run("NoDaemon", func(t *testing.T) {
		sockets(t)
		app, _, _ := newApp()
		app.Config.Device = "/dev/tty.usbmodem9"

		err := feedViaDaemon(context.Background(), app, formatPCM, defaultBitrate)
		if err == nil || !errors.Is(err, broker.ErrNoDaemon) {
			t.Fatalf("the failure is %v, wanted it to be ErrNoDaemon", err)
		}
		if !strings.Contains(err.Error(), "radiocli daemon --device /dev/tty.usbmodem9") {
			t.Errorf("the failure is %v, wanted the command to start one", err)
		}
	})

	// Verify that a refusal which is not the absence of a daemon is passed back
	t.Run("DialError", func(t *testing.T) {
		sockets(t)
		app, _, _ := newApp()
		app.Config.Device = "/dev/tty.usbmodem1"

		d := daemon{
			hello:  broker.Response{Type: broker.TypeHello, Protocol: broker.Version + 1},
			hangUp: true,
		}
		d.serve(t, "/dev/tty.usbmodem1")

		err := feedViaDaemon(context.Background(), app, formatPCM, defaultBitrate)
		if err == nil || !strings.Contains(err.Error(), "speaks protocol") {
			t.Errorf("the failure is %v, wanted the protocol mismatch", err)
		}
	})
}

// Test_relay tests the relay function with 100% coverage.
//
// Coverage: 100% (6 test cases covering both formats, events, and every ending)
//
// Test cases:
//   - PCM: raw samples are written on exactly as they arrived
//   - Opus: each packet is written with its length in front of it
//   - Event: something that was not audio is passed on rather than written
//   - Cancelled: stopping the command is not a failure
//   - HeaderError: a stream that refuses the length is reported
//   - AudioError: a stream that refuses the audio is reported
func Test_relay(t *testing.T) {
	// Verify that raw samples reach stdout unchanged
	t.Run("PCM", func(t *testing.T) {
		sockets(t)
		app, out, _ := newApp()
		stream := dialAudio(t, "/dev/tty.usbmodem1", daemon{
			hello:  hello(),
			reply:  broker.Response{Type: broker.TypeAudio},
			tail:   audioPacket(1, []byte{1, 2, 3, 4}),
			hangUp: true,
		})

		err := relay(context.Background(), app, stream, formatPCM)
		if err == nil || !strings.Contains(err.Error(), "the daemon stopped sending audio") {
			t.Fatalf("the failure is %v, wanted the daemon hanging up to be reported", err)
		}
		if out.String() != "\x01\x02\x03\x04" {
			t.Errorf("the audio is %q, wanted the four bytes the daemon sent", out.String())
		}
	})

	// Verify that an Opus packet is preceded by its length
	t.Run("Opus", func(t *testing.T) {
		sockets(t)
		app, out, _ := newApp()
		stream := dialAudio(t, "/dev/tty.usbmodem1", daemon{
			hello:  hello(),
			reply:  broker.Response{Type: broker.TypeAudio},
			tail:   audioPacket(1, []byte{9, 9, 9}),
			hangUp: true,
		})

		if err := relay(context.Background(), app, stream, formatOpus); err == nil {
			t.Fatal("the relay succeeded, wanted the daemon hanging up to be reported")
		}
		if out.String() != "\x00\x03\x09\x09\x09" {
			t.Errorf("the audio is %q, wanted the length in front of the packet", out.String())
		}
	})

	// Verify that news from the daemon is passed on rather than written as audio
	t.Run("Event", func(t *testing.T) {
		sockets(t)
		app, out, notes := newApp()
		stream := dialAudio(t, "/dev/tty.usbmodem1", daemon{
			hello:  hello(),
			reply:  broker.Response{Type: broker.TypeAudio},
			tail:   audioFrame(broker.FrameJSON, []byte(`{"type":"channel","channel":"left"}`)),
			hangUp: true,
		})

		if err := relay(context.Background(), app, stream, formatPCM); err == nil {
			t.Fatal("the relay succeeded, wanted the daemon hanging up to be reported")
		}
		if !strings.Contains(notes.String(), "on the left channel") {
			t.Errorf("the note is %q, wanted the channel passed on", notes.String())
		}
		if out.String() != "" {
			t.Errorf("the audio is %q, wanted nothing but audio in it", out.String())
		}
	})

	// Verify that stopping the command leaves the exit status alone
	t.Run("Cancelled", func(t *testing.T) {
		sockets(t)
		app, _, _ := newApp()
		stream := dialAudio(t, "/dev/tty.usbmodem1", daemon{
			hello: hello(),
			reply: broker.Response{Type: broker.TypeAudio},
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if err := relay(ctx, app, stream, formatPCM); err != nil {
			t.Errorf("the failure is %v, wanted stopping to be no failure at all", err)
		}
	})

	// Verify that a stream which cannot take the length says so
	t.Run("HeaderError", func(t *testing.T) {
		sockets(t)
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		stream := dialAudio(t, "/dev/tty.usbmodem1", daemon{
			hello:  hello(),
			reply:  broker.Response{Type: broker.TypeAudio},
			tail:   audioPacket(1, []byte{9, 9, 9}),
			hangUp: true,
		})

		err := relay(context.Background(), app, stream, formatOpus)
		if err == nil || !strings.Contains(err.Error(), "writing the audio out") {
			t.Errorf("the failure is %v, wanted the audio to be named", err)
		}
	})

	// Verify that a stream which cannot take the audio says so
	t.Run("AudioError", func(t *testing.T) {
		sockets(t)
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		stream := dialAudio(t, "/dev/tty.usbmodem1", daemon{
			hello:  hello(),
			reply:  broker.Response{Type: broker.TypeAudio},
			tail:   audioPacket(1, []byte{9, 9, 9}),
			hangUp: true,
		})

		err := relay(context.Background(), app, stream, formatPCM)
		if err == nil || !strings.Contains(err.Error(), "writing the audio out") {
			t.Errorf("the failure is %v, wanted the audio to be named", err)
		}
	})
}

// Test_relayEvent tests the relayEvent function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every message this passes on and both
// it ignores)
//
// Test cases:
//   - Channel: the side of the cable the scanner is on is passed on
//   - Silent: a stream carrying nothing at all is explained
//   - Error: the daemon's own complaint is passed on
//   - Level: the meter goes to the log rather than to the terminal
//   - Unknown: a message this does not pass on is ignored
//   - Malformed: something that is not a message at all is ignored
func Test_relayEvent(t *testing.T) {
	// Verify that the chosen channel is passed on
	t.Run("Channel", func(t *testing.T) {
		app, _, notes := newApp()

		relayEvent(app, json.RawMessage(`{"type":"channel","channel":"right"}`))

		if !strings.Contains(notes.String(), "on the right channel") {
			t.Errorf("the note is %q, wanted the channel named", notes.String())
		}
	})

	// Verify that digital silence is explained rather than left to be guessed at
	t.Run("Silent", func(t *testing.T) {
		app, _, notes := newApp()

		relayEvent(app, json.RawMessage(`{"type":"silent"}`))

		if !strings.Contains(notes.String(), "no signal at all") {
			t.Errorf("the note is %q, wanted the silence explained", notes.String())
		}
	})

	// Verify that the daemon's own complaint reaches the person watching
	t.Run("Error", func(t *testing.T) {
		app, _, notes := newApp()

		relayEvent(app, json.RawMessage(`{"type":"error","error":"the sound input is gone"}`))

		if !strings.Contains(notes.String(), "the sound input is gone") {
			t.Errorf("the note is %q, wanted the daemon's complaint", notes.String())
		}
	})

	// Verify that the meter is not printed over a terminal that carries audio
	t.Run("Level", func(t *testing.T) {
		app, out, notes := newApp()

		relayEvent(app, json.RawMessage(`{"type":"level","dbfs":-42.5}`))

		if notes.String() != "" || out.String() != "" {
			t.Errorf("the meter printed %q and %q, wanted neither", out.String(), notes.String())
		}
	})

	// Verify that a message this does not pass on costs nothing
	t.Run("Unknown", func(t *testing.T) {
		app, _, notes := newApp()

		relayEvent(app, json.RawMessage(`{"type":"tx_start"}`))

		if notes.String() != "" {
			t.Errorf("the note is %q, wanted nothing said", notes.String())
		}
	})

	// Verify that something which is not a message at all is ignored
	t.Run("Malformed", func(t *testing.T) {
		app, _, notes := newApp()

		relayEvent(app, json.RawMessage(`not json`))

		if notes.String() != "" {
			t.Errorf("the note is %q, wanted nothing said", notes.String())
		}
	})
}

// Test_report tests the report function with 100% coverage.
//
// Coverage: 100% (4 test cases covering both events this passes on and both it
// ignores)
//
// Test cases:
//   - Channel: the side of the cable the scanner is on is passed on
//   - ChannelMissing: a channel event with no channel in it says nothing
//   - Silent: a stream carrying nothing at all is explained
//   - Unknown: an event this does not pass on is ignored
func Test_report(t *testing.T) {
	// Verify that the chosen channel is passed on
	t.Run("Channel", func(t *testing.T) {
		app, _, notes := newApp()

		report(app, audiofeed.Event{
			Kind:    "channel",
			Payload: map[string]any{"channel": audiofeed.ChannelRight},
		})

		if !strings.Contains(notes.String(), "on the right channel") {
			t.Errorf("the note is %q, wanted the channel named", notes.String())
		}
	})

	// Verify that a channel event carrying no channel is not announced blank
	t.Run("ChannelMissing", func(t *testing.T) {
		app, _, notes := newApp()

		report(app, audiofeed.Event{Kind: "channel", Payload: map[string]any{"channel": 3}})

		if notes.String() != "" {
			t.Errorf("the note is %q, wanted nothing said", notes.String())
		}
	})

	// Verify that digital silence is explained rather than left to be guessed at
	t.Run("Silent", func(t *testing.T) {
		app, _, notes := newApp()

		report(app, audiofeed.Event{Kind: "silent"})

		if !strings.Contains(notes.String(), "no signal at all") {
			t.Errorf("the note is %q, wanted the silence explained", notes.String())
		}
	})

	// Verify that an event this does not pass on costs nothing
	t.Run("Unknown", func(t *testing.T) {
		app, _, notes := newApp()

		report(app, audiofeed.Event{Kind: "level", Payload: map[string]any{"dbfs": -42.5}})

		if notes.String() != "" {
			t.Errorf("the note is %q, wanted nothing said", notes.String())
		}
	})
}

// Test_runFeed tests the runFeed function with 100% coverage.
//
// Coverage: 100% (7 test cases covering every check and both ways to feed)
//
// Test cases:
//   - InDaemon: a command that never ends is refused inside a daemon
//   - BadFormat: a format this cannot write is named back
//   - BadChannel: a channel mode this does not know is refused
//   - BadBitrate: a rate outside what the encoder accepts is refused
//   - Direct: naming an input opens it here
//   - ViaDaemon: naming no input takes the audio from a daemon
//   - DefaultFormat: leaving the format empty writes raw samples
func Test_runFeed(t *testing.T) {
	// Verify that a command which never ends is refused inside a daemon
	t.Run("InDaemon", func(t *testing.T) {
		app, _, _ := newApp()
		app.InDaemon = true

		err := runFeed(context.Background(), app, feedOptions{})
		if err == nil || !strings.Contains(err.Error(), "cannot be run inside a daemon") {
			t.Errorf("the failure is %v, wanted the daemon to have refused it", err)
		}
	})

	// Verify that a format this cannot write is named back with the two it can
	t.Run("BadFormat", func(t *testing.T) {
		app, _, _ := newApp()

		err := runFeed(context.Background(), app, feedOptions{format: "flac"})
		if err == nil || !strings.Contains(err.Error(), `no audio format called "flac"`) {
			t.Errorf("the failure is %v, wanted the format named back", err)
		}
	})

	// Verify that a channel mode this does not know is refused
	t.Run("BadChannel", func(t *testing.T) {
		app, _, _ := newApp()

		err := runFeed(context.Background(), app, feedOptions{
			format: formatPCM, channel: "sideways",
		})
		if err == nil {
			t.Error("the recording started, wanted the channel mode refused")
		}
	})

	// Verify that a rate the encoder will not take is refused before anything opens
	t.Run("BadBitrate", func(t *testing.T) {
		app, _, _ := newApp()

		err := runFeed(context.Background(), app, feedOptions{
			format: formatOpus, channel: audiofeed.ChannelMix, bitrate: 1,
		})
		if err == nil || !strings.Contains(err.Error(), "outside what the encoder accepts") {
			t.Errorf("the failure is %v, wanted the rate refused", err)
		}
	})

	// Verify that naming an input opens it here rather than asking a daemon
	t.Run("Direct", func(t *testing.T) {
		app, _, notes := newApp()
		fakeStart(t, "Cubilux CB5 Line In", nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := runFeed(ctx, app, feedOptions{
			input: "Cubilux CB5 Line In", format: formatPCM,
			channel: audiofeed.ChannelMix, bitrate: defaultBitrate,
		})
		if err != nil {
			t.Fatalf("listening: %v", err)
		}
		if !strings.Contains(notes.String(), "Cubilux CB5 Line In") {
			t.Errorf("the announcement is %q, wanted the sound input named", notes.String())
		}
	})

	// Verify that naming no input takes the audio from a daemon
	t.Run("ViaDaemon", func(t *testing.T) {
		app, _, _ := newApp()

		err := runFeed(context.Background(), app, feedOptions{
			format: formatPCM, channel: audiofeed.ChannelMix, bitrate: defaultBitrate,
		})
		if err == nil || !errors.Is(err, appcontext.ErrNoDevice) {
			t.Errorf("the failure is %v, wanted the daemon path to have been taken", err)
		}
	})

	// Verify that leaving the format out writes raw samples
	t.Run("DefaultFormat", func(t *testing.T) {
		app, _, _ := newApp()

		err := runFeed(context.Background(), app, feedOptions{format: "  "})
		if err == nil || !errors.Is(err, appcontext.ErrNoDevice) {
			t.Errorf("the failure is %v, wanted an empty format to have been accepted", err)
		}
	})
}

// Test_write tests the write function with 100% coverage.
//
// Coverage: 100% (7 test cases covering both formats, events, and every ending)
//
// Test cases:
//   - PCM: raw samples are written on as they arrive
//   - Opus: each frame is encoded and written with its length in front
//   - EncoderError: a rate the encoder will not take is reported
//   - EncodeError: a frame the encoder will not take is reported
//   - Event: news from the capture is passed on rather than written
//   - Closed: a feed that has gone ends the writing without a failure
//   - WriteError: a stream that refuses the audio is reported
func Test_write(t *testing.T) {
	// feed returns a feed and a subscription to it, closed at the end of the test.
	feed := func(t *testing.T) (*audiofeed.Feed, *audiofeed.Sub) {
		t.Helper()
		f := audiofeed.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
		sub := f.Subscribe(feedQueue)
		t.Cleanup(sub.Close)
		return f, sub
	}

	// Verify that raw samples reach stdout as they arrive
	t.Run("PCM", func(t *testing.T) {
		app, _, _ := newApp()
		f, sub := feed(t)

		ctx, cancel := context.WithCancel(context.Background())
		out := &cancelWriter{cancel: cancel}
		app.Stdout = out

		f.Publish(audiofeed.Frame{Seq: 1, PCM: []byte{1, 2, 3, 4}})

		if err := write(ctx, app, sub, formatPCM, defaultBitrate); err != nil {
			t.Fatalf("writing: %v", err)
		}
		if out.buf.String() != "\x01\x02\x03\x04" {
			t.Errorf("the audio is %q, wanted the four bytes published", out.buf.String())
		}
	})

	// Verify that a frame is encoded and written with its length in front
	t.Run("Opus", func(t *testing.T) {
		app, _, _ := newApp()
		f, sub := feed(t)

		ctx, cancel := context.WithCancel(context.Background())
		out := &cancelWriter{cancel: cancel}
		app.Stdout = out

		f.Publish(audiofeed.Frame{Seq: 1, PCM: make([]byte, opusenc.FrameBytes)})

		if err := write(ctx, app, sub, formatOpus, defaultBitrate); err != nil {
			t.Fatalf("writing: %v", err)
		}

		written := out.buf.Bytes()
		if len(written) < 3 {
			t.Fatalf("the audio is %d bytes, wanted a length and a packet", len(written))
		}
		if int(binary.BigEndian.Uint16(written[:2])) != len(written)-2 {
			t.Errorf("the length says %d and the packet is %d bytes",
				binary.BigEndian.Uint16(written[:2]), len(written)-2)
		}
	})

	// Verify that a rate the encoder will not take is reported before anything is written
	t.Run("EncoderError", func(t *testing.T) {
		app, _, _ := newApp()
		_, sub := feed(t)

		if err := write(context.Background(), app, sub, formatOpus, 1); err == nil {
			t.Error("the writing started, wanted the encoder to have refused the rate")
		}
	})

	// Verify that a frame the encoder will not take is reported
	t.Run("EncodeError", func(t *testing.T) {
		app, _, _ := newApp()
		f, sub := feed(t)

		f.Publish(audiofeed.Frame{Seq: 1, PCM: []byte{1, 2, 3, 4}})

		err := write(context.Background(), app, sub, formatOpus, defaultBitrate)
		if err == nil || !errors.Is(err, opusenc.ErrFrameSize) {
			t.Errorf("the failure is %v, wanted the frame size refused", err)
		}
	})

	// Verify that news from the capture is passed on rather than written as audio
	t.Run("Event", func(t *testing.T) {
		app, out, _ := newApp()
		f, sub := feed(t)

		ctx, cancel := context.WithCancel(context.Background())
		notes := &cancelWriter{cancel: cancel}
		app.Stderr = notes

		f.PublishEvent("channel", map[string]any{"channel": audiofeed.ChannelLeft})

		if err := write(ctx, app, sub, formatPCM, defaultBitrate); err != nil {
			t.Fatalf("writing: %v", err)
		}
		if !strings.Contains(notes.buf.String(), "on the left channel") {
			t.Errorf("the note is %q, wanted the channel passed on", notes.buf.String())
		}
		if out.String() != "" {
			t.Errorf("the audio is %q, wanted nothing but audio in it", out.String())
		}
	})

	// Verify that a feed which has gone ends the writing rather than failing it.
	//
	// Both channels close together, so which of the two ends this is whichever
	// the runtime picks. Repeated so that both are taken.
	t.Run("Closed", func(t *testing.T) {
		app, _, _ := newApp()

		for i := 0; i < 32; i++ {
			f := audiofeed.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			sub := f.Subscribe(feedQueue)
			sub.Close()

			if err := write(context.Background(), app, sub, formatPCM, defaultBitrate); err != nil {
				t.Fatalf("writing: %v", err)
			}
		}
	})

	// Verify that a stream which cannot take the audio says so
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		f, sub := feed(t)

		f.Publish(audiofeed.Frame{Seq: 1, PCM: []byte{1, 2, 3, 4}})

		err := write(context.Background(), app, sub, formatPCM, defaultBitrate)
		if err == nil || !strings.Contains(err.Error(), "writing the audio out") {
			t.Errorf("the failure is %v, wanted the audio to be named", err)
		}
	})

	// Verify that a stream which cannot take the packet length says so
	t.Run("HeaderError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		f, sub := feed(t)

		f.Publish(audiofeed.Frame{Seq: 1, PCM: make([]byte, opusenc.FrameBytes)})

		err := write(context.Background(), app, sub, formatOpus, defaultBitrate)
		if err == nil || !strings.Contains(err.Error(), "writing the audio out") {
			t.Errorf("the failure is %v, wanted the audio to be named", err)
		}
	})
}

// Test_startCapture tests the startCapture variable with 100% coverage.
//
// Coverage: 100% (1 test case covering the one call the real opener makes)
//
// The real opener is only ever called with somewhere to publish to, so the
// refusal below is the whole of it that can be reached without a sound card.
// Nothing here opens one: audiofeed.Start checks its Publisher before it goes
// anywhere near the hardware.
//
// Test cases:
//   - NoPublisher: opening a capture with nowhere to publish to is refused
func Test_startCapture(t *testing.T) {
	// Verify that the default opener is the real one, refusing what it refuses
	t.Run("NoPublisher", func(t *testing.T) {
		if _, err := startCapture(audiofeed.Options{}, nil); err == nil {
			t.Error("the capture opened, wanted nowhere to publish to to be refused")
		}
	})
}

// Test_writeError tests the writeError function with 100% coverage.
//
// Coverage: 100% (3 test cases covering both ways a pipe closes and a real failure)
//
// Test cases:
//   - ClosedPipe: a player that was closed is how this command ends
//   - EPIPE: the same ending as the operating system reports it
//   - Other: anything else is reported with what was being written
func Test_writeError(t *testing.T) {
	// Verify that a closed pipe leaves the exit status alone
	t.Run("ClosedPipe", func(t *testing.T) {
		if err := writeError(io.ErrClosedPipe); err != nil {
			t.Errorf("the failure is %v, wanted a closed pipe to be no failure at all", err)
		}
	})

	// Verify that the operating system's own name for it is treated the same
	t.Run("EPIPE", func(t *testing.T) {
		if err := writeError(syscall.EPIPE); err != nil {
			t.Errorf("the failure is %v, wanted a closed pipe to be no failure at all", err)
		}
	})

	// Verify that anything else says what was being written when it happened
	t.Run("Other", func(t *testing.T) {
		err := writeError(errors.New("the disk is full"))
		if err == nil || !strings.Contains(err.Error(), "writing the audio out") {
			t.Errorf("the failure is %v, wanted the audio to be named", err)
		}
	})
}
