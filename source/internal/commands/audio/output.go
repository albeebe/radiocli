// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package audio

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/opusenc"
	"github.com/spf13/cobra"
)

// newOutput returns the "audio output" command bound to app.
//
// Parameters:
//   - app: the application context the command reads its configuration and
//     writes its output through
//
// Returns:
//   - the "audio output" command, with its flags already registered
func newOutput(app *appcontext.App) *cobra.Command {
	var (
		input   string
		format  string
		bitrate int
		channel string
	)

	cmd := &cobra.Command{
		Use:   "output",
		Short: "Write the scanner's audio to standard output",
		Long: "Output writes the audio arriving on a sound input to standard output, as it\n" +
			"arrives, until you stop it.\n\n" +
			"By default the audio is raw samples: signed 16-bit little-endian mono at\n" +
			"48000 Hz, with no header and no framing, which is what a player expects to be\n" +
			"handed on a pipe. The exact flags to give one are printed to standard error\n" +
			"when the stream starts, so they are never something to work out.\n\n" +
			"The scanner's audio does not travel over the USB cable this tool controls it\n" +
			"with. It leaves the scanner as an ordinary sound signal on a cable, so which\n" +
			"input carries it is something only you can know. Run \"radiocli audio\" to see\n" +
			"what this computer can record from.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOutput(cmd.Context(), app, outputOptions{
				input:   input,
				format:  format,
				bitrate: bitrate,
				channel: channel,
			})
		},
	}

	cmd.Flags().StringVar(&input, "input", "",
		"sound input to record from, as \"radiocli audio\" names it")
	cmd.Flags().StringVar(&format, "format", formatPCM,
		"how to write the audio: \"pcm\" or \"opus\"")
	cmd.Flags().IntVar(&bitrate, "bitrate", defaultBitrate,
		"bits per second, for --format opus")
	cmd.Flags().StringVar(&channel, "channel", audiofeed.ChannelAuto,
		"which side of the cable the scanner is on: \"auto\", \"left\", \"right\" or \"mix\"")

	return cmd
}

// announce says what is about to arrive on stdout, on stderr.
//
// On stderr because stdout is the audio, and this is the one piece of
// information a person needs that the audio cannot carry: raw samples have no
// header, so the rate, the width and the channel count have to be told rather
// than discovered. Printing the player's own flags saves working them out.
//
// Parameters:
//   - app: the application context whose Stderr receives the line
//   - source: the name of the sound input the audio is coming from
//   - format: the audio format about to be written, formatPCM or formatOpus
//   - bitrate: bits per second, said only when the format is formatOpus
func announce(app *appcontext.App, source, format string, bitrate int) {
	switch format {
	case formatOpus:
		app.Notef("Recording from %q as Opus at %d kbps, 48000 Hz mono, in 20 ms packets.\n"+
			"Each packet is preceded by its length as two bytes, most significant first.\n"+
			"This is for a program to read. Nothing that plays files will open it.\n",
			source, bitrate/1000)
	default:
		app.Notef("Recording from %q: signed 16-bit little-endian mono at 48000 Hz.\n"+
			"Play it with:  ffplay -f s16le -ar 48000 -ac 1 -i -\n"+
			"           or: play -t raw -e signed -b 16 -c 1 -r 48000 -\n",
			source)
	}
}

// outputDirect opens the sound card itself and writes what it hears.
//
// Opening the card here means this process holds it, so nothing else can. That
// is the right thing for checking a cable and the wrong thing for anything
// sharing a scanner, which is why the daemon exists.
//
// Parameters:
//   - ctx: context that ends the recording when it is cancelled
//   - app: the application context holding the logger and the streams
//   - input: the sound input to open, as "radiocli audio" names it
//   - format: the audio format to write, formatPCM or formatOpus
//   - bitrate: bits per second, used only when the format is formatOpus
//   - channel: the side of the cable to take, as audiofeed.ParseChannel gave it
//
// Returns:
//   - error if the sound input cannot be opened, if the encoder cannot be
//     built, or if writing the audio out fails; nil once ctx is cancelled
func outputDirect(ctx context.Context, app *appcontext.App, input, format string, bitrate int, channel string) error {
	feed := audiofeed.New(app.Log)
	sub := feed.Subscribe(outputQueue)
	defer sub.Close()

	capture, err := startCapture(audiofeed.Options{
		Source:  input,
		Channel: channel,
		Log:     app.Log,
	}, feed)
	if err != nil {
		return err
	}
	defer capture.Close()

	announce(app, capture.Source(), format, bitrate)

	return write(ctx, app, sub, format, bitrate)
}

// outputViaDaemon takes the audio from whichever daemon is holding this
// scanner's sound input.
//
// This is the one command in the tool that does not try the scanner for itself
// first and fall back to a daemon. Every other command can run either way,
// because either way it ends up talking to the same radio. Audio cannot: a
// sound card can only be open once, and if a daemon has it then opening it here
// would fail, while if no daemon has it there is nothing to share. So there is
// no fallback to arrange, and asking for one would only produce a worse error
// message than saying plainly that a daemon is what serves this.
//
// Parameters:
//   - ctx: context that ends the relay when it is cancelled
//   - app: the application context holding the device name, the logger and the
//     streams
//   - format: the audio format to ask the daemon for, formatPCM or formatOpus
//   - bitrate: bits per second, used only when the format is formatOpus
//
// Returns:
//   - error if no device was named, if no daemon is holding that device, or if
//     the relay itself fails; nil once ctx is cancelled
func outputViaDaemon(ctx context.Context, app *appcontext.App, format string, bitrate int) error {
	if app.Config.Device == "" {
		return fmt.Errorf("%w: name one with --device, or pass --input to open a sound "+
			"input directly", appcontext.ErrNoDevice)
	}

	stream, err := broker.DialAudio(app.Config.Device, format, bitrate)
	if err != nil {
		if errors.Is(err, broker.ErrNoDaemon) {
			return fmt.Errorf("%w.\n"+
				"Audio comes from a daemon, because a sound input can only be open once and\n"+
				"sharing it is what the daemon is for. Start one with:\n"+
				"  radiocli daemon --device %s --audio \"<sound input>\"\n"+
				"Or pass --input to open a sound input directly, without sharing it",
				err, app.Config.Device)
		}
		return err
	}
	defer stream.Close()

	info := stream.Info()
	announce(app, info.Source, format, info.Bitrate)
	if info.Channel != "" {
		app.Log.Debug("the daemon is folding the audio", "channel", info.Channel)
	}

	return relay(ctx, app, stream, format)
}

// relay moves what the daemon sends to stdout until ctx is cancelled.
//
// The audio is written on exactly as it arrived. For raw samples that is the
// samples; for Opus it is the packet, with its length in front so that whatever
// reads this can tell one from the next. Nothing is decoded and nothing is
// re-encoded, so this end has no opinion about the audio at all.
//
// Parameters:
//   - ctx: context that ends the relay when it is cancelled
//   - app: the application context whose Stdout receives the audio
//   - stream: the daemon's audio stream to read from
//   - format: the audio format the stream carries, formatPCM or formatOpus
//
// Returns:
//   - error if the daemon stops sending audio for any reason other than the
//     cancellation, or if writing to Stdout fails; nil once ctx is cancelled
func relay(ctx context.Context, app *appcontext.App, stream *broker.AudioStream, format string) error {
	// The stream blocks in a read, so cancelling has to reach it by closing the
	// socket underneath it rather than by being noticed between frames.
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			stream.Close()
		case <-stopped:
		}
	}()

	var header [2]byte

	for {
		_, audio, event, err := stream.Next()
		if err != nil {
			// Ending is how this command ends, so a read that failed because
			// the socket was closed on the way out is not a failure.
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("the daemon stopped sending audio: %w", err)
		}

		if event != nil {
			relayEvent(app, event)
			continue
		}

		if format == formatOpus {
			binary.BigEndian.PutUint16(header[:], uint16(len(audio)))
			if _, err := app.Stdout.Write(header[:]); err != nil {
				return writeError(err)
			}
		}
		if _, err := app.Stdout.Write(audio); err != nil {
			return writeError(err)
		}
	}
}

// relayEvent passes on something the daemon said that was not audio.
//
// Parameters:
//   - app: the application context whose Stderr and logger receive it
//   - raw: the JSON message the daemon sent in place of audio, ignored when it
//     cannot be decoded or names a type this does not pass on
func relayEvent(app *appcontext.App, raw json.RawMessage) {
	var msg struct {
		Type    string  `json:"type"`
		Error   string  `json:"error"`
		Channel string  `json:"channel"`
		Seconds int     `json:"seconds"`
		DBFS    float64 `json:"dbfs"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "channel":
		app.Notef("The scanner's audio is on the %s channel.\n", msg.Channel)
	case "silent":
		app.Notef("There has been no signal at all for several seconds, not even noise.\n" +
			"Check the cable, and on macOS check that the daemon is allowed to record:\n" +
			"System Settings > Privacy & Security > Microphone.\n")
	case "error":
		app.Notef("%s\n", msg.Error)
	case "level":
		// The meter is for something drawing one. There is nothing useful to do
		// with four numbers a second on a terminal that is also carrying audio.
		app.Log.Debug("audio level", "dbfs", msg.DBFS)
	}
}

// report passes on the things the capture has to say that are not audio.
//
// To stderr, for the same reason the format line goes there: stdout is the
// audio and nothing else may appear in it.
//
// Parameters:
//   - app: the application context whose Stderr receives the message
//   - ev: the event the capture raised, ignored when its kind is not one this
//     passes on
func report(app *appcontext.App, ev audiofeed.Event) {
	fields, _ := ev.Payload.(map[string]any)

	switch ev.Kind {
	case "channel":
		if channel, ok := fields["channel"].(string); ok {
			app.Notef("The scanner's audio is on the %s channel.\n", channel)
		}
		if fields["reason"] == audiofeed.ReasonOutOfPhase {
			app.Notef("The two sides of the audio cable are out of phase, so this is recording one\n" +
				"side rather than mixing them, which would have cancelled most of the sound.\n" +
				"The scanner's headphone jack is wired that way. To fix it at the source, set\n" +
				"Menu > Settings > Headphone L/R output to the other of \"In Phase\" and\n" +
				"\"Invert Phase\".\n")
		}

	case "silent":
		// The one failure that looks exactly like working. A stream that opened
		// and delivers digital zero is almost never a quiet radio: a real input
		// has a noise floor a few counts wide even with nothing plugged in.
		app.Notef("There has been no signal at all for several seconds, not even noise.\n" +
			"Check the cable, and on macOS check that this tool is allowed to record:\n" +
			"System Settings > Privacy & Security > Microphone.\n")
	}
}

// runOutput checks what was asked for and starts writing.
//
// Parameters:
//   - ctx: context that ends the recording when it is cancelled
//   - app: the application context holding the configuration and the streams
//   - opts: what the flags asked for
//
// Returns:
//   - error if the command was run inside a daemon, if the format, the channel
//     or the bitrate is not one this accepts, or if the recording itself fails;
//     nil once ctx is cancelled
func runOutput(ctx context.Context, app *appcontext.App, opts outputOptions) error {
	// A daemon lends a command its streams for as long as the command runs, on
	// the reasonable assumption that a command finishes. This one does not, so
	// inside a daemon it would hold those streams forever and pour audio down
	// the socket of whichever client happened to ask. Refused here rather than
	// merely left undocumented, because a client can send any command line.
	if app.InDaemon {
		return errors.New("\"audio output\" runs until it is stopped, so it cannot be run " +
			"inside a daemon:\nrun it in a terminal of its own instead")
	}

	format := strings.ToLower(strings.TrimSpace(opts.format))
	if format == "" {
		format = formatPCM
	}
	if format != formatPCM && format != formatOpus {
		return fmt.Errorf("there is no audio format called %q: it is %q or %q",
			opts.format, formatPCM, formatOpus)
	}

	channel, err := audiofeed.ParseChannel(opts.channel)
	if err != nil {
		return err
	}

	if format == formatOpus && (opts.bitrate < opusenc.MinBitrate || opts.bitrate > opusenc.MaxBitrate) {
		return fmt.Errorf("a bitrate of %d is outside what the encoder accepts, which is %d to %d",
			opts.bitrate, opusenc.MinBitrate, opusenc.MaxBitrate)
	}

	// Naming an input means opening it here, which is the exception. Without
	// one the audio comes from the daemon, which is the way that lets more than
	// one thing listen at once.
	if opts.input != "" {
		return outputDirect(ctx, app, opts.input, format, opts.bitrate, channel)
	}
	return outputViaDaemon(ctx, app, format, opts.bitrate)
}

// write moves frames from the feed to stdout until ctx is cancelled.
//
// Nothing is buffered between the frames and the stream. A buffer would trade
// latency for syscalls at 50 writes a second, which is a trade worth making
// nowhere and least of all here: the whole value of this is hearing the radio
// at the moment it says something.
//
// Parameters:
//   - ctx: context that ends the writing when it is cancelled
//   - app: the application context whose Stdout receives the audio
//   - sub: the subscription carrying frames and events from the capture
//   - format: the audio format to write, formatPCM or formatOpus
//   - bitrate: bits per second, used only when the format is formatOpus
//
// Returns:
//   - error if the encoder cannot be built, if encoding a frame fails, or if
//     writing to Stdout fails; nil once ctx is cancelled or the feed closes
func write(ctx context.Context, app *appcontext.App, sub *audiofeed.Sub, format string, bitrate int) error {
	var enc *opusenc.Encoder
	var packet []byte
	if format == formatOpus {
		var err error
		if enc, err = opusenc.New(bitrate); err != nil {
			return err
		}
		packet = make([]byte, opusenc.MaxPacket)
	}

	// Two bytes of length in front of each packet, which is enough for any Opus
	// packet several times over.
	var header [2]byte

	for {
		select {
		case <-ctx.Done():
			// Stopping is how this ends, so it is not a failure. Ctrl-C on a
			// command with no natural end should leave the exit status alone.
			return nil

		case ev, ok := <-sub.Events():
			if !ok {
				return nil
			}
			report(app, ev)

		case frame, ok := <-sub.Frames():
			if !ok {
				return nil
			}

			out := frame.PCM
			if enc != nil {
				n, err := enc.Encode(frame.PCM, packet)
				if err != nil {
					return err
				}
				binary.BigEndian.PutUint16(header[:], uint16(n))
				if _, err := app.Stdout.Write(header[:]); err != nil {
					return writeError(err)
				}
				out = packet[:n]
			}

			if _, err := app.Stdout.Write(out); err != nil {
				return writeError(err)
			}
		}
	}
}

// writeError reports a failure to write the audio out.
//
// A closed pipe is how this normally ends. Somebody pressed q in the player, or
// closed the window it was in, and reporting that as an error would make every
// ordinary use of this command exit non-zero and print a complaint about a
// pipeline that did exactly what it was meant to.
//
// Parameters:
//   - err: the failure the write returned
//
// Returns:
//   - nil if err is a closed pipe, otherwise err wrapped with what was being
//     written when it happened
func writeError(err error) error {
	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE) {
		return nil
	}
	return fmt.Errorf("writing the audio out: %w", err)
}
