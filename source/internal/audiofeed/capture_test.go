// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audiofeed

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// collector is a Publisher that keeps everything, so a test can look at what a
// pump produced rather than at what it did.
type collector struct {
	frames []Frame
	events []Event
}

func (c *collector) Publish(f Frame) { c.frames = append(c.frames, f) }
func (c *collector) PublishEvent(kind string, payload any) {
	c.events = append(c.events, Event{kind, payload})
}

func (c *collector) seqs() []uint32 {
	out := make([]uint32, len(c.frames))
	for i, f := range c.frames {
		out[i] = f.Seq
	}
	return out
}

// ramp builds n bytes whose value depends on where they sit, so that anything
// lost, duplicated or reordered on the way through shows up as a mismatch
// rather than as plausible-looking audio.
func ramp(n int, from int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte((from + i) % 251)
	}
	return b
}

func TestRingRoundTrip(t *testing.T) {
	r := newRing(4096)
	want := ramp(3000, 0)
	r.write(want)

	got := make([]byte, len(want))
	n, next, lost := r.take(0, got)

	if lost != 0 {
		t.Errorf("a ring with room to spare lost %d bytes", lost)
	}
	if n != len(want) || next != uint64(len(want)) {
		t.Fatalf("took %d bytes and reached %d, want %d of both", n, next, len(want))
	}
	if !bytes.Equal(got, want) {
		t.Error("what came out is not what went in")
	}
}

// TestRingWrapsWithoutLosingAByte drives the ring several times round, which is
// the case where the two copies inside write and take have to line up.
func TestRingWrapsWithoutLosingAByte(t *testing.T) {
	r := newRing(1024)

	var wrote int
	var read uint64
	var got bytes.Buffer

	// Sizes that share no factor with the ring, so the seam between the two
	// copies lands somewhere different on every pass.
	for _, size := range []int{300, 700, 101, 999, 37, 512} {
		for range 4 {
			r.write(ramp(size, wrote))
			wrote += size

			buf := make([]byte, 4096)
			n, next, lost := r.take(read, buf)
			if lost != 0 {
				t.Fatalf("lost %d bytes with the reader keeping up", lost)
			}
			got.Write(buf[:n])
			read = next
		}
	}

	if !bytes.Equal(got.Bytes(), ramp(wrote, 0)) {
		t.Errorf("after %d bytes through a %d byte ring, what came out is not what went in",
			wrote, r.capacity())
	}
}

// TestRingOverwritesTheOldest is the failure this ring is designed to have. The
// sound card cannot be told to wait, so a reader that stops paying attention
// loses the audio it did not come back for, rather than the card losing the
// audio it was about to record.
func TestRingOverwritesTheOldest(t *testing.T) {
	r := newRing(1024)
	size := int(r.capacity()) * 3

	r.write(ramp(size, 0))

	buf := make([]byte, r.capacity())
	n, next, lost := r.take(0, buf)

	if lost != uint64(size)-r.capacity() {
		t.Errorf("reported %d bytes lost, want %d", lost, uint64(size)-r.capacity())
	}
	if uint64(n) != r.capacity() {
		t.Errorf("took %d bytes, want the %d the ring holds", n, r.capacity())
	}

	// What survived is the newest, not the oldest.
	if !bytes.Equal(buf[:n], ramp(int(r.capacity()), size-int(r.capacity()))) {
		t.Error("the bytes that survived are not the most recent ones")
	}
	if next != uint64(size) {
		t.Errorf("the cursor reached %d, want %d, which is the true position in the stream",
			next, size)
	}
}

// TestRingWriteBiggerThanItself covers a callback handing over more audio at
// once than the ring holds, which no real device does but which must not corrupt
// the ring if one ever did.
func TestRingWriteBiggerThanItself(t *testing.T) {
	r := newRing(1024)
	size := int(r.capacity()) * 2

	r.write(ramp(size, 0))

	buf := make([]byte, r.capacity())
	n, _, _ := r.take(0, buf)

	if !bytes.Equal(buf[:n], ramp(int(r.capacity()), size-int(r.capacity()))) {
		t.Error("an oversized write did not leave the newest audio in the ring")
	}
}

// tone builds a stereo frame whose left samples all equal v, so that a frame
// folded with ChannelLeft is trivially checkable.
func markedFrame(v int16) []byte {
	f := make([]byte, FrameBytes)
	for i := range FrameSamples {
		binary.LittleEndian.PutUint16(f[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(f[i*4+2:], 0)
	}
	return f
}

// TestPumpCutsFramesWhateverSizeTheCallbacksAre is the property the whole chain
// rests on: a sound card delivers audio in whatever chunks it feels like, and
// the encoder accepts exactly one size. Nothing may be lost or duplicated across
// the seam between them.
func TestPumpCutsFramesWhateverSizeTheCallbacksAre(t *testing.T) {
	// Sizes chosen to straddle a frame boundary in every direction: far less
	// than a frame, just under, just over, and several at a time.
	for _, chunk := range []int{512, 1023, 1920, 3839, 3841, 4096, 20000} {
		t.Run("", func(t *testing.T) {
			r := newRing(1 << 20)
			out := &collector{}
			p := newPump(r, out, ChannelLeft)

			const frames = 40

			// One long stream, each frame marked with its own number, cut into
			// chunks of the size under test.
			var stream []byte
			for i := range frames {
				stream = append(stream, markedFrame(int16(i+1))...)
			}
			for at := 0; at < len(stream); at += chunk {
				r.write(stream[at:min(at+chunk, len(stream))])
				p.drain(time.Now())
			}
			p.drain(time.Now())

			if len(out.frames) != frames {
				t.Fatalf("chunks of %d produced %d frames, want %d", chunk, len(out.frames), frames)
			}

			for i, f := range out.frames {
				if f.Seq != uint32(i) {
					t.Fatalf("frame %d is numbered %d", i, f.Seq)
				}
				if len(f.PCM) != MonoFrameBytes {
					t.Fatalf("frame %d is %d bytes, want %d", i, len(f.PCM), MonoFrameBytes)
				}
				// Every sample of frame i carries i+1, so one frame made of two
				// halves of different frames fails here.
				want := int16(i + 1)
				for s := range FrameSamples {
					if got := monoAt(f.PCM, s); got != want {
						t.Fatalf("frame %d sample %d is %d, want %d: frames were cut in the wrong place",
							i, s, got, want)
					}
				}
			}
		})
	}
}

// TestPumpFramesDoNotShareABuffer: every listener is handed the same Frame, and
// a pump that reused one buffer would have the newest audio appear in a frame
// somebody had not read yet.
func TestPumpFramesDoNotShareABuffer(t *testing.T) {
	r := newRing(1 << 20)
	out := &collector{}
	p := newPump(r, out, ChannelLeft)

	for i := range 5 {
		r.write(markedFrame(int16(i + 1)))
	}
	p.drain(time.Now())

	if len(out.frames) != 5 {
		t.Fatalf("got %d frames, want 5", len(out.frames))
	}
	for i, f := range out.frames {
		if got := monoAt(f.PCM, 0); got != int16(i+1) {
			t.Errorf("frame %d holds %d, want %d: the frames share one buffer", i, got, i+1)
		}
	}
}

// TestPumpJumpsRatherThanShiftsWhenItFallsBehind is the property every timestamp
// downstream depends on.
//
// A reader that stalls loses audio. What must not happen is that the frames
// which follow carry on numbering from where it left off, because then every
// frame after the stall claims a time earlier than the one it was recorded at,
// and a listener working timestamps out from the numbers plays permanently early
// with no way to notice. The number is the capture's own clock, so a gap in it
// is what a gap in the audio has to look like.
func TestPumpJumpsRatherThanShiftsWhenItFallsBehind(t *testing.T) {
	r := newRing(8 * FrameBytes)
	out := &collector{}
	p := newPump(r, out, ChannelLeft)

	// Two frames read normally.
	r.write(markedFrame(1))
	r.write(markedFrame(2))
	p.drain(time.Now())

	if got := out.seqs(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("the first frames are numbered %v, want [0 1]", got)
	}

	// Now the reader stops paying attention for far longer than the ring holds.
	const missed = 100
	for range missed {
		r.write(markedFrame(3))
	}
	p.drain(time.Now())

	seqs := out.seqs()
	if len(seqs) <= 2 {
		t.Fatal("nothing came out after the stall")
	}

	// The frame after the gap must be numbered for when it was recorded, which
	// is near the end of what was written, not for how many came before it.
	after := seqs[2]
	if after <= 1 {
		t.Fatalf("the frame after the stall is numbered %d, want a jump", after)
	}

	total := uint64(2 + missed)
	if uint64(after) < total-9 {
		t.Errorf("the frame after the stall is numbered %d, but %d frames had been recorded: "+
			"the numbering shifted instead of jumping", after, total)
	}

	// And it keeps counting from the new place rather than restarting.
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("frame numbers went %d then %d, which is backwards", seqs[i-1], seqs[i])
		}
	}

	// The last frame recorded is the last frame delivered, so the listener is
	// caught up rather than a fixed distance behind forever.
	if last := seqs[len(seqs)-1]; uint64(last) != total-1 {
		t.Errorf("the last frame out is numbered %d, want %d", last, total-1)
	}
}

// TestPumpMeterAndSilence checks the two things the pump says besides audio.
// Both travel the road that a gate will one day use, which is the reason to
// ship them: a road that carries nothing is a road nobody has tested.
func TestPumpMeterAndSilence(t *testing.T) {
	r := newRing(1 << 21)
	out := &collector{}
	p := newPump(r, out, ChannelLeft)

	// Enough perfectly silent audio to be worth a word about.
	for range silentFrames + meterEvery {
		r.write(markedFrame(0))
	}
	p.drain(time.Now())

	var levels, silences int
	for _, e := range out.events {
		switch e.Kind {
		case "level":
			levels++
		case "silent":
			silences++
		}
	}

	if levels == 0 {
		t.Error("no level reading came out of several seconds of audio")
	}
	if silences != 1 {
		t.Errorf("digital silence was reported %d times, want exactly 1", silences)
	}

	// And it stays said rather than being repeated for as long as it is quiet.
	for range silentFrames {
		r.write(markedFrame(0))
	}
	p.drain(time.Now())

	silences = 0
	for _, e := range out.events {
		if e.Kind == "silent" {
			silences++
		}
	}
	if silences != 1 {
		t.Errorf("digital silence was reported %d times over twice as long, want 1", silences)
	}
}

// fakeInput is an open sound input that never touches a sound card.
type fakeInput struct {
	// name is what the operating system is pretending to call this input.
	name string

	// closed records that the device was released.
	closed atomic.Bool
}

// Close releases the device.
func (f *fakeInput) Close() { f.closed.Store(true) }

// Name is what the operating system calls this input.
func (f *fakeInput) Name() string { return f.name }

// waiting is a Publisher that hands what it is given straight to a test, so a
// test can wait for a frame rather than sleep for one. Nothing here blocks,
// because the goroutine cutting frames must never be held up.
type waiting struct {
	// frames is where published audio arrives.
	frames chan Frame

	// events is where published news arrives.
	events chan Event
}

// Publish offers one frame to the test, dropping it if the test is not waiting.
func (w *waiting) Publish(f Frame) {
	select {
	case w.frames <- f:
	default:
	}
}

// PublishEvent offers one piece of news to the test, dropping it if the test is
// not waiting.
func (w *waiting) PublishEvent(kind string, payload any) {
	select {
	case w.events <- Event{Kind: kind, Payload: payload}:
	default:
	}
}

// TestStart tests the Start function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NoPublisher: a capture with nowhere to publish to is refused
//   - UnknownChannel: a channel mode that does not exist is refused
//   - CannotOpen: a sound input that will not open is reported
//   - Records: a frame handed over by the device is cut and published
//   - NoLogger: a capture without a logger runs anyway
func TestStart(t *testing.T) {

	// Verify that a capture with nowhere to publish to is refused.
	t.Run("NoPublisher", func(t *testing.T) {
		_, err := Start(Options{}, nil)
		if err == nil {
			t.Fatal("a capture with nowhere to publish to was allowed")
		}
		if !strings.Contains(err.Error(), "nowhere to publish to") {
			t.Errorf("the refusal reads %q", err.Error())
		}
	})

	// Verify that a channel mode that does not exist is refused.
	t.Run("UnknownChannel", func(t *testing.T) {
		_, err := Start(Options{Channel: "middle"}, &collector{})
		if err == nil {
			t.Fatal("a channel that does not exist was accepted")
		}
		if !strings.Contains(err.Error(), "there is no channel called") {
			t.Errorf("the refusal reads %q", err.Error())
		}
	})

	// Verify that a sound input that will not open is reported.
	t.Run("CannotOpen", func(t *testing.T) {
		original := openInput
		t.Cleanup(func() { openInput = original })
		openInput = func(name string, onFrames func(pcm []byte)) (audioInput, error) {
			return nil, errors.New("the sound card is not there")
		}

		_, err := Start(Options{Source: "Line In"}, &collector{})
		if err == nil {
			t.Fatal("a sound input that would not open was reported as running")
		}
		if !strings.Contains(err.Error(), "the sound card is not there") {
			t.Errorf("the refusal reads %q", err.Error())
		}
	})

	// Verify that a frame handed over by the device is cut and published.
	t.Run("Records", func(t *testing.T) {
		original := openInput
		t.Cleanup(func() { openInput = original })

		var device *fakeInput
		var deliver func(pcm []byte)
		openInput = func(name string, onFrames func(pcm []byte)) (audioInput, error) {
			device = &fakeInput{name: name}
			deliver = onFrames
			return device, nil
		}

		out := &waiting{frames: make(chan Frame, 8), events: make(chan Event, 8)}
		c, err := Start(Options{
			Source:  "Line In",
			Channel: ChannelLeft,
			Log:     slog.New(slog.DiscardHandler),
		}, out)
		if err != nil {
			t.Fatalf("starting a capture on a fake input: %v", err)
		}
		defer c.Close()

		if got := c.Source(); got != "Line In" {
			t.Errorf("the capture is recording from %q, want %q", got, "Line In")
		}
		if got := c.Channel(); got != ChannelLeft {
			t.Errorf("the capture is folding as %q, want %q", got, ChannelLeft)
		}

		deliver(markedFrame(9))

		select {
		case f := <-out.frames:
			if got := monoAt(f.PCM, 0); got != 9 {
				t.Errorf("the published frame holds %d, want 9", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("nothing was published after the device delivered a frame")
		}

		c.Close()
		if !device.closed.Load() {
			t.Error("closing the capture left the sound input open")
		}
	})

	// Verify that a capture without a logger runs anyway.
	t.Run("NoLogger", func(t *testing.T) {
		original := openInput
		t.Cleanup(func() { openInput = original })
		openInput = func(name string, onFrames func(pcm []byte)) (audioInput, error) {
			return &fakeInput{name: name}, nil
		}

		c, err := Start(Options{Source: "Line In"}, &collector{})
		if err != nil {
			t.Fatalf("starting a capture without a logger: %v", err)
		}
		c.Close()
	})
}

// TestCaptureChannel tests the Capture.Channel function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Deciding: a capture still deciding reports what it is really sending
//   - Told: a capture told which side reports that side
func TestCaptureChannel(t *testing.T) {

	// Verify that a capture still deciding reports what it is really sending.
	t.Run("Deciding", func(t *testing.T) {
		c := &Capture{channel: ChannelAuto}
		if got := c.Channel(); got != ChannelMix {
			t.Errorf("a capture still deciding reports %q, want %q", got, ChannelMix)
		}
	})

	// Verify that a capture told which side reports that side.
	t.Run("Told", func(t *testing.T) {
		c := &Capture{channel: ChannelRight}
		if got := c.Channel(); got != ChannelRight {
			t.Errorf("the capture reports %q, want %q", got, ChannelRight)
		}
	})
}

// TestCaptureClose tests the Capture.Close function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a capture that never opened anything closes cleanly
//   - Twice: closing twice does not close the same channel twice
//   - StopsTheDevice: the sound input is released before the cutter is stopped
//   - TwiceAtOnce: two goroutines closing together do not close the same
//     channel twice
func TestCaptureClose(t *testing.T) {

	// Verify that a capture that never opened anything closes cleanly.
	t.Run("NoDevice", func(t *testing.T) {
		c := &Capture{stop: make(chan struct{})}
		c.Close()

		select {
		case <-c.stop:
		default:
			t.Error("closing the capture left the cutter running")
		}
	})

	// Verify that closing twice does not close the same channel twice.
	t.Run("Twice", func(t *testing.T) {
		c := &Capture{stop: make(chan struct{})}
		c.Close()
		c.Close()
	})

	// Verify that the sound input is released before the cutter is stopped.
	t.Run("StopsTheDevice", func(t *testing.T) {
		device := &fakeInput{name: "Line In"}
		c := &Capture{in: device, stop: make(chan struct{})}
		c.Close()

		if !device.closed.Load() {
			t.Error("the sound input was left open")
		}
	})

	// Verify that two goroutines closing together do not close the same channel
	// twice. The guard used to be a look at the stop channel followed by a
	// close of it, which two callers can both pass before either closes, and
	// the second close panics and takes the daemon with it.
	t.Run("TwiceAtOnce", func(t *testing.T) {
		for range 200 {
			c := &Capture{in: &fakeInput{name: "Line In"}, stop: make(chan struct{})}

			var start, done sync.WaitGroup
			start.Add(1)
			for range 2 {
				done.Add(1)
				go func() {
					defer done.Done()
					start.Wait()
					c.Close()
				}()
			}

			start.Done()
			done.Wait()
		}
	})
}

// TestCaptureSource tests the Capture.Source function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a capture with nothing open is recording from nothing
//   - Open: an open capture reports what the system calls the input
func TestCaptureSource(t *testing.T) {

	// Verify that a capture with nothing open is recording from nothing.
	t.Run("NoDevice", func(t *testing.T) {
		c := &Capture{}
		if got := c.Source(); got != "" {
			t.Errorf("a capture with nothing open reports %q, want nothing", got)
		}
	})

	// Verify that an open capture reports what the system calls the input.
	t.Run("Open", func(t *testing.T) {
		c := &Capture{in: &fakeInput{name: "USB Audio CODEC"}}
		if got := c.Source(); got != "USB Audio CODEC" {
			t.Errorf("the capture reports %q, want %q", got, "USB Audio CODEC")
		}
	})
}

// Test_round1 tests the round1 function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Rounds: readings go to the nearest tenth, in both directions and on
//     either side of zero
func Test_round1(t *testing.T) {
	// Verify that readings round rather than truncate. Levels here are
	// negative and a conversion to int truncates towards zero, so the old
	// version made every reading very slightly louder than it was: the -30.46
	// case is the one that used to come back as -30.4.
	t.Run("Rounds", func(t *testing.T) {
		cases := []struct {
			in, want float64
		}{
			{-30.46, -30.5},
			{-30.44, -30.4},
			{-30.45, -30.5},
			{-0.04, 0},
			{0, 0},
			{12.36, 12.4},
			{-96, -96},
		}

		for _, c := range cases {
			if got := round1(c.in); got != c.want {
				t.Errorf("round1(%v) = %v, want %v", c.in, got, c.want)
			}
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Nudged: audio the device announced is cut at once
//   - LooksAnyway: audio nobody announced is cut when the cutter looks anyway
//   - SettlesTheChannel: the fold the chooser settles on becomes the capture's
func Test_run(t *testing.T) {

	// start builds a capture around a ring and runs its cutter, stopping it when
	// the test ends.
	start := func(t *testing.T, channel string, out Publisher) *Capture {
		t.Helper()

		c := &Capture{
			ring:    newRing(1 << 21),
			log:     slog.New(slog.DiscardHandler),
			stop:    make(chan struct{}),
			wake:    make(chan struct{}, 1),
			channel: channel,
		}
		c.pump = newPump(c.ring, out, channel)

		c.finished.Add(1)
		go c.run()
		t.Cleanup(c.Close)

		return c
	}

	// Verify that audio the device announced is cut at once.
	t.Run("Nudged", func(t *testing.T) {
		out := &waiting{frames: make(chan Frame, 8), events: make(chan Event, 8)}
		c := start(t, ChannelLeft, out)

		c.ring.write(markedFrame(3))
		c.wake <- struct{}{}

		select {
		case f := <-out.frames:
			if got := monoAt(f.PCM, 0); got != 3 {
				t.Errorf("the frame that came out holds %d, want 3", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("nudging the cutter published nothing")
		}
	})

	// Verify that audio nobody announced is cut when the cutter looks anyway.
	t.Run("LooksAnyway", func(t *testing.T) {
		original := wakeEvery
		t.Cleanup(func() { wakeEvery = original })
		wakeEvery = time.Millisecond

		out := &waiting{frames: make(chan Frame, 8), events: make(chan Event, 8)}
		c := start(t, ChannelLeft, out)

		// Written without nudging, so only the ticker can find it.
		c.ring.write(markedFrame(4))

		select {
		case f := <-out.frames:
			if got := monoAt(f.PCM, 0); got != 4 {
				t.Errorf("the frame that came out holds %d, want 4", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the cutter never looked at audio nobody announced")
		}
	})

	// Verify that the fold the chooser settles on becomes the capture's.
	t.Run("SettlesTheChannel", func(t *testing.T) {
		out := &waiting{frames: make(chan Frame, 8), events: make(chan Event, 8)}
		c := start(t, ChannelAuto, out)

		// A mono lead on the left, for as long as it takes to be sure.
		for range chooseFrames {
			c.ring.write(markedFrame(8000))
		}
		c.wake <- struct{}{}

		deadline := time.Now().Add(5 * time.Second)
		for c.Channel() != ChannelLeft && time.Now().Before(deadline) {
			// Frames are taken so the publisher never fills up and the cutter
			// keeps going round.
			select {
			case <-out.frames:
			case <-time.After(time.Millisecond):
			}
		}

		if got := c.Channel(); got != ChannelLeft {
			t.Errorf("the capture is folding as %q, want %q once the chooser settled", got, ChannelLeft)
		}
	})
}

// Test_drain tests the drain function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Nothing: a ring with less than a frame in it publishes nothing
//   - Frames: every whole frame waiting is cut and published
//   - Overwritten: a cursor left behind is put back on a frame boundary
//   - OverwrittenAndShort: a jump that lands short of a frame waits for more
func Test_drain(t *testing.T) {

	// Verify that a ring with less than a frame in it publishes nothing.
	t.Run("Nothing", func(t *testing.T) {
		r := newRing(1 << 16)
		out := &collector{}
		p := newPump(r, out, ChannelLeft)

		r.write(ramp(FrameBytes-1, 0))
		p.drain(time.Now())

		if len(out.frames) != 0 {
			t.Errorf("half a frame produced %d frames, want none", len(out.frames))
		}
	})

	// Verify that every whole frame waiting is cut and published.
	t.Run("Frames", func(t *testing.T) {
		r := newRing(1 << 16)
		out := &collector{}
		p := newPump(r, out, ChannelLeft)

		r.write(markedFrame(1))
		r.write(markedFrame(2))
		p.drain(time.Now())

		if got := out.seqs(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
			t.Errorf("two frames of audio came out as %v, want [0 1]", got)
		}
	})

	// Verify that a cursor left behind is put back on a frame boundary.
	t.Run("Overwritten", func(t *testing.T) {
		r := newRing(4 * FrameBytes)
		out := &collector{}
		p := newPump(r, out, ChannelLeft)

		// A byte more than a whole number of frames, so the jump lands off a
		// boundary and has to be rounded forward.
		r.write(ramp(1, 0))
		for range 20 {
			r.write(markedFrame(5))
		}
		p.drain(time.Now())

		if len(out.frames) == 0 {
			t.Fatal("nothing came out after the ring was overwritten")
		}
		if p.cursor%FrameBytes != 0 {
			t.Errorf("the cursor was left at %d, which is not on a frame boundary", p.cursor)
		}
		for i, f := range out.frames {
			if len(f.PCM) != MonoFrameBytes {
				t.Fatalf("frame %d is %d bytes, want %d", i, len(f.PCM), MonoFrameBytes)
			}
		}
	})

	// Verify that a jump that lands short of a frame waits for more.
	t.Run("OverwrittenAndShort", func(t *testing.T) {
		// A ring that holds less than one frame, so what survives an overwrite
		// is never a whole frame.
		r := newRing(FrameBytes / 2)
		out := &collector{}
		p := newPump(r, out, ChannelLeft)

		r.write(ramp(2*int(r.capacity()), 0))
		p.drain(time.Now())

		if len(out.frames) != 0 {
			t.Errorf("a ring smaller than a frame produced %d frames, want none", len(out.frames))
		}
		if p.cursor%FrameBytes != 0 {
			t.Errorf("the cursor was left at %d, which is not on a frame boundary", p.cursor)
		}
	})
}

// Test_take tests the take function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Nothing: a cursor that has caught up copies nothing
//   - Wrapped: a read that straddles the end of the buffer is put together
//   - Overwritten: a cursor older than the ring is told what it missed
//   - CaughtByTheWriter: audio overwritten during the copy is thrown away
func Test_take(t *testing.T) {

	// Verify that a cursor that has caught up copies nothing.
	t.Run("Nothing", func(t *testing.T) {
		r := newRing(1024)
		r.write(ramp(100, 0))

		n, next, lost := r.take(100, make([]byte, 64))
		if n != 0 || next != 100 || lost != 0 {
			t.Errorf("took %d bytes and reached %d, losing %d, want 0, 100, 0", n, next, lost)
		}
	})

	// Verify that a read that straddles the end of the buffer is put together.
	t.Run("Wrapped", func(t *testing.T) {
		r := newRing(1024)
		r.write(ramp(900, 0))
		r.write(ramp(100, 900))

		got := make([]byte, 1000)
		n, _, lost := r.take(0, got)
		if lost != 0 {
			t.Fatalf("a reader keeping up lost %d bytes", lost)
		}
		if !bytes.Equal(got[:n], ramp(1000, 0)) {
			t.Error("a read across the end of the buffer came out in the wrong order")
		}
	})

	// Verify that a cursor older than the ring is told what it missed.
	t.Run("Overwritten", func(t *testing.T) {
		r := newRing(1024)
		r.write(ramp(3000, 0))

		n, next, lost := r.take(0, make([]byte, 1024))
		if lost != 3000-r.capacity() {
			t.Errorf("reported %d bytes lost, want %d", lost, 3000-r.capacity())
		}
		if uint64(n) != r.capacity() || next != 3000 {
			t.Errorf("took %d bytes and reached %d, want %d and 3000", n, next, r.capacity())
		}
	})

	// Verify that audio overwritten during the copy is thrown away.
	t.Run("CaughtByTheWriter", func(t *testing.T) {
		// The reader and the writer have to run at the same time for the writer
		// to catch the reader in the middle of a copy, so there has to be room
		// for both.
		if runtime.GOMAXPROCS(0) < 2 {
			was := runtime.GOMAXPROCS(2)
			t.Cleanup(func() { runtime.GOMAXPROCS(was) })
		}

		// A ring big enough that one copy out of it takes long enough to be
		// interrupted.
		r := newRing(1 << 20)
		into := make([]byte, r.capacity())

		// A writer that runs the whole ring over several times a moment, so that
		// what a reader copies is overwritten while it is being copied. Only the
		// clock is touched, because what the bytes are does not matter here.
		stop := make(chan struct{})
		var writing sync.WaitGroup
		writing.Add(1)
		go func() {
			defer writing.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				r.written.Add(r.capacity() * 2)
			}
		}()

		var caught bool
		deadline := time.Now().Add(5 * time.Second)
		for !caught && time.Now().Before(deadline) {
			w := r.written.Load()
			var cursor uint64
			if w > r.capacity() {
				cursor = w - r.capacity()
			}

			n, next, lost := r.take(cursor, into)
			// Nothing copied and nothing lost is a reader that had simply
			// caught up, which is not the case being looked for.
			if n != 0 || lost == 0 {
				continue
			}
			if next < cursor {
				t.Fatalf("the cursor went backwards, from %d to %d", cursor, next)
			}
			caught = true
		}

		close(stop)
		writing.Wait()

		if !caught {
			t.Fatal("the writer never caught the reader mid-copy")
		}
	})
}

// Test_write tests the write function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Nothing: a callback with no audio in it changes nothing
//   - Fits: audio that fits is kept whole
//   - BiggerThanTheRing: more audio than the ring holds keeps the newest of it
func Test_write(t *testing.T) {

	// Verify that a callback with no audio in it changes nothing.
	t.Run("Nothing", func(t *testing.T) {
		r := newRing(1024)
		r.write(ramp(10, 0))

		r.write(nil)
		r.write([]byte{})

		if got := r.written.Load(); got != 10 {
			t.Errorf("the clock reads %d after two empty callbacks, want 10", got)
		}
	})

	// Verify that audio that fits is kept whole.
	t.Run("Fits", func(t *testing.T) {
		r := newRing(1024)
		want := ramp(500, 0)
		r.write(want)

		got := make([]byte, 500)
		n, _, _ := r.take(0, got)
		if !bytes.Equal(got[:n], want) {
			t.Error("what came out is not what went in")
		}
	})

	// Verify that more audio than the ring holds keeps the newest of it.
	t.Run("BiggerThanTheRing", func(t *testing.T) {
		r := newRing(1024)
		size := int(r.capacity()) * 3
		r.write(ramp(size, 0))

		if got := r.written.Load(); got != uint64(size) {
			t.Errorf("the clock reads %d, want %d, which is everything the card produced",
				got, size)
		}

		got := make([]byte, r.capacity())
		n, _, _ := r.take(uint64(size)-r.capacity(), got)
		if !bytes.Equal(got[:n], ramp(int(r.capacity()), size-int(r.capacity()))) {
			t.Error("the audio left in the ring is not the newest of what was written")
		}
	})
}

// Test_openInput tests the openInput var with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - NothingToGiveTheAudioTo: the real audioin.Open is reached and refuses
func Test_openInput(t *testing.T) {

	// Verify that the real audioin.Open is reached and refuses.
	//
	// Called with no callback, which audioin refuses before it asks the audio
	// system anything at all. That is what makes this the one safe way to run
	// the real opener: no sound card is touched, no permission is asked for and
	// no microphone light comes on.
	t.Run("NothingToGiveTheAudioTo", func(t *testing.T) {
		if _, err := openInput("", nil); err == nil {
			t.Error("opening a sound input with nothing to give the audio to was allowed")
		}
	})
}
