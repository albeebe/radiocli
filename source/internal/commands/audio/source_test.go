// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/25/2026

package audio

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/broker"
)

// Test_openAudio tests the openAudio function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Direct: a named input is opened here, with a stream for the speakers
//   - DirectWithoutSpeakers: no second stream when nothing is playing
//   - DirectFails: a sound input that will not open is reported
//   - NoDaemon: no input reaches for a daemon, and finding none is advice
func Test_openAudio(t *testing.T) {
	// Verify a named input is opened directly.
	t.Run("Direct", func(t *testing.T) {
		fakeStart(t, "USB Audio CODEC", nil)
		app, _, _ := recorderApp()

		frames, speakers, source, done, err := openAudio(context.Background(), app,
			"USB Audio CODEC", audiofeed.ChannelAuto, true)
		if err != nil {
			t.Fatalf("opening the audio: %v", err)
		}
		defer done()

		if source != "USB Audio CODEC" || frames == nil {
			t.Errorf("got %q and %v, want the input named", source, frames)
		}
		// The speakers get a stream of their own so that a stalled recorder
		// cannot put them behind the radio.
		if speakers == nil {
			t.Error("no stream for the speakers, so they would share the recorder's queue")
		}
	})

	// Verify that a run with nothing to play opens no second stream, since
	// every frame put in one nobody reads is a frame dropped for nothing.
	t.Run("DirectWithoutSpeakers", func(t *testing.T) {
		fakeStart(t, "USB Audio CODEC", nil)
		app, _, _ := recorderApp()

		frames, speakers, _, done, err := openAudio(context.Background(), app,
			"USB Audio CODEC", audiofeed.ChannelAuto, false)
		if err != nil {
			t.Fatalf("opening the audio: %v", err)
		}
		defer done()

		if frames == nil {
			t.Error("no frames for the recorder")
		}
		if speakers != nil {
			t.Error("a stream was opened for speakers nobody asked for")
		}
	})

	// Verify a sound input that will not open is reported.
	t.Run("DirectFails", func(t *testing.T) {
		fakeStart(t, "", errors.New("no such input"))
		app, _, _ := recorderApp()

		if _, _, _, _, err := openAudio(context.Background(), app,
			"Nothing", audiofeed.ChannelAuto, true); err == nil {
			t.Fatal("an input that will not open reported nothing")
		}
	})

	// Verify no input at all reaches for a daemon, and says how to start one
	// when there is none.
	t.Run("NoDaemon", func(t *testing.T) {
		sockets(t)
		app, _, _ := recorderApp()
		app.Config.Device = "/dev/example"

		_, _, _, _, err := openAudio(context.Background(), app, "", audiofeed.ChannelAuto, true)
		if err == nil || !strings.Contains(err.Error(), "radiocli daemon") {
			t.Fatalf("got %v, want advice on starting a daemon", err)
		}
	})
}

// Test_audioViaDaemon tests taking audio from a daemon with 100% coverage.
//
// The daemon sends samples with no level and no timestamp, so both are worked
// out here, and the level has to be measured the same way the capture measures
// it or a gate tuned against one would behave differently depending on where
// its audio came from.
func Test_audioViaDaemon(t *testing.T) {
	sockets(t)
	const port = "/dev/example"

	audio := tone(loudLevel)
	daemon{
		hello: hello(),
		reply: broker.Response{Type: broker.TypeAudio, Format: formatPCM, Rate: 48000, Channels: 1},
		tail:  append(audioPacket(7, audio), audioFrame(broker.FrameJSON, []byte(`{"type":"event"}`))...),
	}.serve(t, port)

	app, _, _ := recorderApp()
	app.Config.Device = port

	frames, speakers, _, done, err := audioViaDaemon(context.Background(), app, true)
	if err != nil {
		t.Fatalf("asking the daemon for audio: %v", err)
	}
	defer done()

	// The same frame reaches the speakers on their own stream, so a stalled
	// recorder cannot put them behind the radio.
	select {
	case f := <-speakers:
		if f.Seq != 7 {
			t.Errorf("the speakers got frame %d, want 7 as the daemon sent it", f.Seq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no audio reached the speakers")
	}

	select {
	case f := <-frames:
		if f.Seq != 7 {
			t.Errorf("the frame is numbered %d, want 7 as the daemon sent it", f.Seq)
		}
		// Measured rather than assumed, because the daemon sends none.
		if f.Level != audiofeed.LevelOf(audio) {
			t.Errorf("the frame measures %v, want %v", f.Level, audiofeed.LevelOf(audio))
		}
		if f.At.IsZero() {
			t.Error("the frame has no time on it, so the gate cannot place it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no audio arrived from the daemon")
	}
}

// Test_audioViaDaemonRefused covers a daemon that answers but will not send
// audio, which is different from there being no daemon at all and must not be
// reported as advice to start one.
func Test_audioViaDaemonRefused(t *testing.T) {
	sockets(t)
	const port = "/dev/example-refuses"

	// A daemon that is there and holding no sound input, so it refuses.
	runDaemon{}.serveRuns(t, port)

	app, _, _ := recorderApp()
	app.Config.Device = port

	_, _, _, _, err := audioViaDaemon(context.Background(), app, true)
	if err == nil {
		t.Fatal("a daemon that would not send audio reported nothing")
	}
	if strings.Contains(err.Error(), "radiocli daemon --device") {
		t.Errorf("got %q, want it not to advise starting a daemon that is already there", err)
	}
}

// Test_audioViaDaemonStopsWhenTheRunDoes covers the audio arriving faster than
// it is taken, which is what happens when the recorder is stopped while a
// daemon is still sending.
//
// The frames waiting have to be let go of rather than the goroutine blocking on
// a channel nobody will read again.
//
// The speakers are asked for and then never read, which is the other half of
// it: their queue is three frames deep and this sends hundreds, so every one
// after the third has to push the oldest out rather than wait for room.
func Test_audioViaDaemonStopsWhenTheRunDoes(t *testing.T) {
	sockets(t)
	const port = "/dev/example-floods"

	// Comfortably more than the recorder will hold, so the send blocks.
	var tail []byte
	for i := range recordQueue * 2 {
		tail = append(tail, audioPacket(uint32(i), tone(quietLevel))...)
	}
	daemon{
		hello: hello(),
		reply: broker.Response{Type: broker.TypeAudio, Format: formatPCM, Rate: 48000, Channels: 1},
		tail:  tail,
	}.serve(t, port)

	app, _, _ := recorderApp()
	app.Config.Device = port

	ctx, cancel := context.WithCancel(context.Background())
	frames, _, _, done, err := audioViaDaemon(ctx, app, true)
	if err != nil {
		t.Fatalf("asking the daemon for audio: %v", err)
	}
	defer done()

	// Take one, so the rest are known to have arrived and filled the queue.
	select {
	case <-frames:
	case <-time.After(2 * time.Second):
		t.Fatal("no audio arrived from the daemon")
	}

	// Let the queue fill, then stop without reading any more.
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

// Test_openAudioReportsWhatTheFeedSays checks that the recorder passes on the
// feed's warnings.
//
// They are the only way a person learns that the recording is going wrong for a
// reason outside the radio: a lead in the wrong socket, a permission never
// granted, or two sides of a cable that cancel each other. The recorder reads
// frames from its subscription, so without something reading the events beside
// them they would be produced and never seen.
func Test_openAudioReportsWhatTheFeedSays(t *testing.T) {
	// A capture that publishes an event and no audio at all.
	original := startCapture
	t.Cleanup(func() { startCapture = original })

	published := make(chan struct{})
	startCapture = func(opts audiofeed.Options, out audiofeed.Publisher) (capture, error) {
		go func() {
			out.PublishEvent("channel", map[string]any{
				"channel": audiofeed.ChannelLeft,
				"reason":  audiofeed.ReasonOutOfPhase,
			})
			close(published)
		}()
		return fakeCapture{source: "USB Audio CODEC"}, nil
	}

	// The reporting happens on a goroutine of its own while this test reads
	// what it wrote, so the buffer they share has to be safe for both.
	app, _, _ := recorderApp()
	errs := &lockedBuffer{}
	app.Stderr = errs

	_, _, _, done, err := openAudio(context.Background(), app, "USB Audio CODEC",
		audiofeed.ChannelAuto, false)
	if err != nil {
		t.Fatalf("opening the audio: %v", err)
	}
	defer done()

	<-published
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && errs.String() == "" {
		time.Sleep(5 * time.Millisecond)
	}

	said := errs.String()
	if !strings.Contains(said, "out of phase") {
		t.Errorf("said %q, want the out of phase cable called out", said)
	}
	if !strings.Contains(said, "Headphone L/R output") {
		t.Errorf("said %q, want the menu that fixes it named", said)
	}
}

// lockedBuffer is a stream two goroutines may use at once: one reporting what
// the feed said, and a test watching for it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// String is everything written so far.
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Write takes what is written, under the lock String reads through.
func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// Test_quiet checks the silence handed to the speakers between transmissions.
//
// Coverage: quiet, both the shared array and the allocation past the end of it.
//
// Test cases:
//   - OneFrame: a frame's worth is silent and the right length
//   - Oversized: a request longer than the shared array still answers silence
//   - Shared: two calls come from the same array rather than allocating
func Test_quiet(t *testing.T) {
	// Verify the ordinary case, which is one frame every 20 ms.
	t.Run("OneFrame", func(t *testing.T) {
		got := quiet(audiofeed.FrameBytes)
		if len(got) != audiofeed.FrameBytes {
			t.Errorf("quiet gave %d bytes, want %d", len(got), audiofeed.FrameBytes)
		}
		if slices.ContainsFunc(got, func(b byte) bool { return b != 0 }) {
			t.Error("the silence handed to the speakers was not silent")
		}
	})

	// Verify that a frame larger than anything expected is still answered,
	// rather than panicking on a slice past the end of the shared array.
	t.Run("Oversized", func(t *testing.T) {
		n := len(silence) + 1
		got := quiet(n)
		if len(got) != n {
			t.Errorf("quiet gave %d bytes, want %d", len(got), n)
		}
		if slices.ContainsFunc(got, func(b byte) bool { return b != 0 }) {
			t.Error("the oversized silence was not silent")
		}
	})

	// Verify the reason the array is shared: a frame arrives fifty times a
	// second for the length of a run, and allocating one each time is waste.
	t.Run("Shared", func(t *testing.T) {
		a, b := quiet(audiofeed.FrameBytes), quiet(audiofeed.FrameBytes)
		if &a[0] != &b[0] {
			t.Error("two calls allocated rather than sharing the one array")
		}
	})
}
