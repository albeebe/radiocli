// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audioout

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
)

// tone builds n bytes of audio whose value says where in the stream each byte
// came from, so a test can tell what was played from what was dropped.
//
// Parameters:
//   - from: the value the first byte carries
//   - n: how many bytes to build
//
// Returns:
//   - []byte counting up from "from", wrapping at 256
func tone(from, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte((from + i) % 256)
	}
	return b
}

// primeFrames is the cushion every ring in these tests is built with: the
// smallest Open accepts, so the arithmetic in each case stays small enough to
// check by hand.
const primeFrames = 2

// primed builds a ring with enough audio in it to have started playing, which
// is where most of the interesting behaviour begins.
//
// Returns:
//   - *ring holding exactly primeFrames of audio, not yet read from
func primedRing() *ring {
	r := newRing(primeFrames * FrameBytes)
	r.write(tone(0, primeFrames*FrameBytes))
	return r
}

// TestPlayerClose tests the Player.Close method with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Closes: the device is torn down
//   - Twice: closing again does not tear down a device that has gone
//   - Nil: a player nobody opened can be closed
func TestPlayerClose(t *testing.T) {
	// Verify that closing the player closes the device under it.
	t.Run("Closes", func(t *testing.T) {
		out := &fakeOutput{}
		p := &Player{out: out, ring: newRing(primeFrames * FrameBytes)}

		p.Close()
		if out.closes != 1 {
			t.Errorf("the device was closed %d times, want once", out.closes)
		}
	})

	// Verify that a second close is not a second teardown. Commands close on
	// the way out of more than one path, and the library does not survive being
	// torn down twice.
	t.Run("Twice", func(t *testing.T) {
		out := &fakeOutput{}
		p := &Player{out: out, ring: newRing(primeFrames * FrameBytes)}

		p.Close()
		p.Close()
		if out.closes != 1 {
			t.Errorf("the device was closed %d times, want once", out.closes)
		}
	})

	// Verify that the nil player a command holds when nobody asked to hear
	// anything can be closed like any other.
	t.Run("Nil", func(t *testing.T) {
		var p *Player
		p.Close()
	})
}

// TestPlayerName tests the Player.Name method with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Named: the device's own spelling comes back
//   - Nil: a player nobody opened has no name rather than crashing
func TestPlayerName(t *testing.T) {
	// Verify that what comes back is the system's spelling, which is what a
	// person will have read in a listing.
	t.Run("Named", func(t *testing.T) {
		p := &Player{out: &fakeOutput{name: "MacBook Pro Speakers"}, ring: newRing(primeFrames * FrameBytes)}
		if got := p.Name(); got != "MacBook Pro Speakers" {
			t.Errorf("Name gave %q, want the device's own name", got)
		}
	})

	// Verify that the nil player is answerable, since the commands ask for the
	// name when they say what they are doing.
	t.Run("Nil", func(t *testing.T) {
		var p *Player
		if got := p.Name(); got != "" {
			t.Errorf("Name gave %q, want nothing", got)
		}
	})
}

// TestPlayerPlay tests the Player.Play method with 100% coverage.
//
// Coverage: 100% (3 test cases covering both branches)
//
// Test cases:
//   - Queued: what is handed over reaches the ring
//   - Copied: the caller's slice is theirs again once this returns
//   - Nil: a player nobody opened swallows the audio rather than crashing
func TestPlayerPlay(t *testing.T) {
	// Verify that audio handed to the player is what the device is given when
	// it asks.
	//
	// Past the ramp at the front, which is not the audio arriving differently
	// but the first few milliseconds of it being faded up from silence so the
	// speakers do not click. See fade.
	t.Run("Queued", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing(primeFrames * FrameBytes)}
		want := tone(0, primeFrames*FrameBytes)
		p.Play(want)

		got := make([]byte, len(want))
		p.ring.read(got)
		if !bytes.Equal(got[fadeBytes:], want[fadeBytes:]) {
			t.Error("what came out of the ring is not what was played into it")
		}
	})

	// Verify that the samples are copied. The frames handed in here often
	// belong to a capture callback's own buffer, which is written over the
	// moment it returns.
	t.Run("Copied", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing(primeFrames * FrameBytes)}
		pcm := tone(0, primeFrames*FrameBytes)
		p.Play(pcm)

		// What a capture callback does to its buffer the moment it is handed
		// back.
		clear(pcm)

		got := make([]byte, FrameBytes)
		p.ring.read(got)
		if bytes.Equal(got, make([]byte, FrameBytes)) {
			t.Error("the ring held the caller's slice rather than a copy of it")
		}
	})

	// Verify that the nil player takes audio and does nothing with it, which is
	// what lets a recorder call this without asking whether anybody is
	// listening.
	t.Run("Nil", func(t *testing.T) {
		var p *Player
		p.Play(tone(0, FrameBytes))
	})
}

// TestPlayerStats tests the Player.Stats method with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Counted: what the ring recorded comes back
//   - Nil: a player nobody opened reports nothing rather than crashing
func TestPlayerStats(t *testing.T) {
	// Verify that the counts come from the ring rather than being kept twice.
	t.Run("Counted", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing(primeFrames * FrameBytes)}
		p.ring.stats = Stats{Dropped: 7, Starved: 3}

		if got := p.Stats(); got.Dropped != 7 || got.Starved != 3 {
			t.Errorf("Stats gave %+v, want what the ring recorded", got)
		}
	})

	// Verify that the nil player answers, since a command reports these on the
	// way out whether or not anybody was listening.
	t.Run("Nil", func(t *testing.T) {
		var p *Player
		if got := p.Stats(); got != (Stats{}) {
			t.Errorf("Stats gave %+v, want nothing", got)
		}
	})
}

// TestRingRead tests the ring.read method with 100% coverage.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Unprimed: nothing plays until the cushion has been built
//   - Primes: the cushion arriving starts the playing
//   - Wraps: audio split across the end of the buffer comes out in one piece
//   - Starves: a ring that runs dry plays silence and says so
//   - SettlesToSilence: draining to exactly empty ramps down instead of stepping
//   - Reprimes: after running dry the cushion has to be built again
//   - FillsEverything: the library's buffer is never left part written
func TestRingRead(t *testing.T) {
	// Verify that a ring holding less than the cushion plays silence rather
	// than handing over what little it has, which would leave it empty again
	// immediately.
	t.Run("Unprimed", func(t *testing.T) {
		r := newRing(primeFrames * FrameBytes)
		r.write(tone(1, FrameBytes))

		out := make([]byte, FrameBytes)
		r.read(out)
		if !bytes.Equal(out, make([]byte, FrameBytes)) {
			t.Error("the ring played audio before the cushion was built")
		}
		if r.length != FrameBytes {
			t.Errorf("%d bytes are waiting, want the frame to have been kept", r.length)
		}
	})

	// Verify that the cushion arriving is what starts the playing, and that
	// nothing was lost while it was being built. Past the ramp at the front,
	// which is the audio being faded up rather than arriving differently.
	t.Run("Primes", func(t *testing.T) {
		r := primedRing()

		out := make([]byte, FrameBytes)
		r.read(out)
		if !bytes.Equal(out[fadeBytes:], tone(0, FrameBytes)[fadeBytes:]) {
			t.Error("the first frame played is not the first frame that arrived")
		}
	})

	// Verify that audio which happens to straddle the end of the buffer is put
	// back together, since the ring wraps wherever it has got to.
	t.Run("Wraps", func(t *testing.T) {
		r := primedRing()

		// Empty it, which leaves the start somewhere in the middle of the
		// buffer, then fill it again so the next write has to wrap.
		out := make([]byte, primeFrames*FrameBytes)
		r.read(out)

		r.write(tone(100, len(r.buf)))
		got := make([]byte, len(r.buf))
		r.primed = true
		r.read(got)

		want := tone(100, len(r.buf))
		if !bytes.Equal(got[:len(got)-fadeBytes], want[:len(want)-fadeBytes]) {
			t.Error("audio that wrapped around the end of the buffer came out in the wrong order")
		}
	})

	// Verify that a ring with less than was asked for fills the rest with
	// silence and counts it, rather than leaving the library's buffer holding
	// whatever it held last time.
	t.Run("Starves", func(t *testing.T) {
		r := primedRing()

		out := make([]byte, (primeFrames+1)*FrameBytes)
		for i := range out {
			out[i] = 0xFF
		}
		r.read(out)

		if !bytes.Equal(out[primeFrames*FrameBytes:], make([]byte, FrameBytes)) {
			t.Error("the part of the buffer with no audio for it was left as it was")
		}
		if r.stats.Starved != 1 {
			t.Errorf("the ring recorded %d starves, want 1", r.stats.Starved)
		}
	})

	// Verify the click fix at the end of every burst. The sizes a real device
	// deals in divide each other, so the ring drains to exactly empty and the
	// starving callback holds no audio to fade: the ramp down to silence has
	// to be made from the sample the previous callback ended on.
	t.Run("SettlesToSilence", func(t *testing.T) {
		const level = 12000
		r := newRing(primeFrames * FrameBytes)
		r.write(loudFrame(primeFrames*FrameSamples, level))

		// Drained in device-sized reads, the way a real callback empties it:
		// to exactly zero, never to a partial buffer.
		for range primeFrames {
			r.read(make([]byte, FrameBytes))
		}

		out := make([]byte, FrameBytes)
		r.read(out)

		if got := sample(out, 0); got < level/2 {
			t.Errorf("the starving read began at %d, want a ramp starting near %d", got, level)
		}
		if got := sample(out, fadeBytes/2-1); got != 0 {
			t.Errorf("the ramp ended on %d, want silence", got)
		}
		if !bytes.Equal(out[fadeBytes:], make([]byte, FrameBytes-fadeBytes)) {
			t.Error("the buffer past the ramp is not silence")
		}
		if r.stats.Starved != 1 {
			t.Errorf("the ring recorded %d starves, want 1", r.stats.Starved)
		}
	})

	// Verify that running dry costs the cushion as well as the audio, so the
	// next burst builds it again rather than starting one frame from empty.
	t.Run("Reprimes", func(t *testing.T) {
		r := primedRing()
		r.read(make([]byte, (primeFrames+1)*FrameBytes))

		if r.primed {
			t.Fatal("the ring is still primed after running dry")
		}

		r.write(tone(0, FrameBytes))
		out := make([]byte, FrameBytes)
		r.read(out)
		if !bytes.Equal(out, make([]byte, FrameBytes)) {
			t.Error("the ring played on before the cushion was rebuilt")
		}
	})

	// Verify the rule the audio library imposes: every byte of the buffer is
	// written, every time. A buffer left short is not silence, it is whatever
	// was there before, played again.
	t.Run("FillsEverything", func(t *testing.T) {
		r := newRing(primeFrames * FrameBytes)

		out := make([]byte, 777) // Not a whole number of frames, as a real device is not
		for i := range out {
			out[i] = 0xFF
		}
		r.read(out)
		if !bytes.Equal(out, make([]byte, len(out))) {
			t.Error("an unprimed ring left part of the library's buffer unwritten")
		}
	})
}

// TestRingWrite tests the ring.write method with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Nothing: an empty slice is not a write
//   - Wraps: a write that runs off the end continues at the beginning
//   - DropsOldest: audio arriving faster than it is played loses the oldest
//   - LongerThanTheRing: a single write bigger than the whole ring keeps its tail
//   - Counts: what was dropped is counted in bytes
func TestRingWrite(t *testing.T) {
	// Verify that nothing to play is not treated as audio, since an empty write
	// would otherwise reach the copies below.
	t.Run("Nothing", func(t *testing.T) {
		r := newRing(primeFrames * FrameBytes)
		r.write(nil)
		if r.length != 0 {
			t.Errorf("%d bytes are waiting after writing nothing", r.length)
		}
	})

	// Verify that a write which runs off the end of the buffer continues at the
	// beginning rather than being cut short.
	t.Run("Wraps", func(t *testing.T) {
		r := newRing(primeFrames * FrameBytes)
		r.write(tone(0, len(r.buf)-10))
		r.read(make([]byte, len(r.buf)-10))
		r.write(tone(50, 20))

		if r.length != 20 {
			t.Fatalf("%d bytes are waiting, want 20", r.length)
		}
		got := make([]byte, 20)
		r.primed = true
		r.read(got)
		if !bytes.Equal(got, tone(50, 20)) {
			t.Error("a write that wrapped came back in the wrong order")
		}
	})

	// Verify that a full ring loses its oldest audio rather than refusing the
	// newest, because what is worth hearing is the radio as it is now.
	t.Run("DropsOldest", func(t *testing.T) {
		r := newRing(primeFrames * FrameBytes)
		r.write(tone(0, len(r.buf)))
		r.write(tone(1, FrameBytes))

		r.primed = true
		got := make([]byte, len(r.buf))
		r.read(got)
		if !bytes.Equal(got[len(got)-FrameBytes:], tone(1, FrameBytes)) {
			t.Error("the newest audio is not at the end, so the wrong end was dropped")
		}
		if r.stats.Dropped != uint64(FrameBytes) {
			t.Errorf("%d bytes were counted as dropped, want %d", r.stats.Dropped, FrameBytes)
		}
	})

	// Verify that one write larger than the whole ring keeps its tail. A gate
	// releasing a long transmission in one go is exactly this.
	t.Run("LongerThanTheRing", func(t *testing.T) {
		r := newRing(primeFrames * FrameBytes)
		big := tone(0, len(r.buf)+FrameBytes)
		r.write(big)

		r.primed = true
		got := make([]byte, len(r.buf))
		r.read(got)
		if !bytes.Equal(got, big[FrameBytes:]) {
			t.Error("the ring kept the head of an oversized write, want the tail")
		}
	})

	// Verify that every byte thrown away is counted, since that count is what
	// tells somebody the audio is arriving faster than it can be played.
	t.Run("Counts", func(t *testing.T) {
		r := newRing(primeFrames * FrameBytes)
		r.write(tone(0, len(r.buf)+100))
		if r.stats.Dropped != 100 {
			t.Errorf("%d bytes were counted as dropped, want 100", r.stats.Dropped)
		}
	})
}

// TestRingUnderRace checks that the two ends of the ring can be used at once,
// which is the whole arrangement: Play runs on a goroutine holding the audio
// and read runs on a thread belonging to the audio library.
//
// It exists to be run under -race. What it asserts is only that nothing
// deadlocked, because what a reader sees during a race is by definition not
// fixed.
func TestRingUnderRace(t *testing.T) {
	r := newRing(primeFrames * FrameBytes)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			r.write(tone(i, FrameBytes))
		}
	}()

	go func() {
		defer wg.Done()
		out := make([]byte, FrameBytes)
		for i := 0; i < 500; i++ {
			r.read(out)
		}
	}()

	wg.Wait()
}

// sample reads one signed 16-bit little-endian sample out of pcm.
//
// Parameters:
//   - pcm: the audio to read from
//   - at: which sample, counting from zero
//
// Returns:
//   - the sample's value
func sample(pcm []byte, at int) int16 {
	return int16(binary.LittleEndian.Uint16(pcm[at*2:]))
}

// loudFrame builds a frame of samples all at level, which is what a ramp or a
// gain can be measured against.
//
// Parameters:
//   - n: how many samples
//   - level: the value to give every one of them
//
// Returns:
//   - the samples, signed 16-bit little-endian
func loudFrame(n int, level int16) []byte {
	pcm := make([]byte, n*2)
	for i := range n {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(level))
	}
	return pcm
}

// Test_settle tests the settle function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Ramps: the slope runs from the sample down to silence
//   - Silence: a callback that ended silent has nothing to settle
//   - Short: a buffer smaller than the fade holds what fits of the slope
func Test_settle(t *testing.T) {
	// Verify the slope: starting near the sample it was handed, ending silent.
	t.Run("Ramps", func(t *testing.T) {
		pcm := make([]byte, FrameBytes)
		settle(pcm, 10000)

		if got := sample(pcm, 0); got < 5000 {
			t.Errorf("the slope began at %d, want near 10000", got)
		}
		if got := sample(pcm, fadeBytes/2-1); got != 0 {
			t.Errorf("the slope ended on %d, want silence", got)
		}
		if !bytes.Equal(pcm[fadeBytes:], make([]byte, FrameBytes-fadeBytes)) {
			t.Error("the buffer past the slope was written")
		}
	})

	// Verify that ending on silence writes nothing, since there is no step to
	// smooth and the buffer is already what it should be.
	t.Run("Silence", func(t *testing.T) {
		pcm := make([]byte, FrameBytes)
		settle(pcm, 0)
		if !bytes.Equal(pcm, make([]byte, FrameBytes)) {
			t.Error("settling from silence wrote something")
		}
	})

	// Verify that a buffer with less room than the fade still ends silent,
	// with the slope squeezed into what there is.
	t.Run("Short", func(t *testing.T) {
		pcm := make([]byte, fadeBytes/2)
		settle(pcm, 10000)
		if got := sample(pcm, len(pcm)/2-1); got != 0 {
			t.Errorf("the squeezed slope ended on %d, want silence", got)
		}
	})
}

// TestPlayerSetGain tests the Player.SetGain method with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - LouderPCM: what is played comes out scaled
//   - ScratchReused: a second frame is scaled into the same scratch buffer
//   - Zero: no gain leaves the audio exactly as it arrived
//   - Nil: a player nobody opened takes the setting and does nothing
func TestPlayerSetGain(t *testing.T) {
	// Verify that decibels reach the samples. 6 dB is a little under double, so
	// a quarter-scale sample lands a little under half scale.
	t.Run("LouderPCM", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing(primeFrames * FrameBytes)}
		p.SetGain(6)
		p.Play(loudFrame(primeFrames*FrameSamples, 8000))

		out := make([]byte, FrameBytes)
		p.ring.read(out)

		// Past the ramp at the front, which is deliberately not at full level.
		if got := sample(out, FrameSamples-1); got < 15000 || got > 16400 {
			t.Errorf("a sample of 8000 played back as %d, want it near doubled", got)
		}
	})

	// Verify that the scratch the scaling happens in is reused rather than
	// grown again, since a steady stream of frames should cost one allocation
	// and not fifty a second.
	t.Run("ScratchReused", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing(primeFrames * FrameBytes)}
		p.SetGain(6)
		p.Play(loudFrame(FrameSamples, 8000))
		first := &p.ring.scratch[0]
		p.Play(loudFrame(FrameSamples, 8000))

		if &p.ring.scratch[0] != first {
			t.Error("the second frame was scaled into a new scratch buffer")
		}
		if got := len(p.ring.scratch); got != FrameBytes {
			t.Errorf("the scratch holds %d bytes, want a frame", got)
		}
	})

	// Verify that the default costs the audio nothing, since scaling every
	// sample by one would be arithmetic done for no reason.
	t.Run("Zero", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing(primeFrames * FrameBytes)}
		p.SetGain(0)

		if got := p.ring.gainNow(); got != 1 {
			t.Errorf("no gain left the ring multiplying by %v, want 1", got)
		}
	})

	// Verify that the nil player a command holds when nobody is listening
	// takes the setting without complaint.
	t.Run("Nil", func(t *testing.T) {
		var p *Player
		p.SetGain(12)
	})
}

// Test_fade tests the fade function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - In: a ramp up starts at silence and ends near full
//   - Out: a ramp down starts near full and ends at silence
//   - Nothing: an empty run is not a ramp
//   - Monotonic: every step of a ramp up is at least as loud as the last
func Test_fade(t *testing.T) {
	// Verify that a burst begins at silence rather than at whatever sample the
	// audio happened to be at, which is the step that clicks.
	t.Run("In", func(t *testing.T) {
		pcm := loudFrame(100, 10000)
		fade(pcm, true)

		if got := sample(pcm, 0); got != 0 {
			t.Errorf("the first sample is %d, want silence", got)
		}
		if got := sample(pcm, 99); got < 9800 {
			t.Errorf("the last sample is %d, want it back at full level", got)
		}
	})

	// Verify the same at the other end, which is the click when the squelch
	// closes.
	t.Run("Out", func(t *testing.T) {
		pcm := loudFrame(100, 10000)
		fade(pcm, false)

		if got := sample(pcm, 0); got < 9800 {
			t.Errorf("the first sample is %d, want it still at full level", got)
		}
		if got := sample(pcm, 99); got != 0 {
			t.Errorf("the last sample is %d, want silence", got)
		}
	})

	// Verify that nothing to ramp is not an error, since a burst can be shorter
	// than the ramp.
	t.Run("Nothing", func(t *testing.T) {
		fade(nil, true)
		fade([]byte{}, false)
	})

	// Verify that the ramp only ever goes one way. A ramp that went up and down
	// would be audible as a warble rather than inaudible as a fade.
	t.Run("Monotonic", func(t *testing.T) {
		pcm := loudFrame(240, 10000)
		fade(pcm, true)

		for i := 1; i < 240; i++ {
			if sample(pcm, i) < sample(pcm, i-1) {
				t.Fatalf("sample %d is quieter than the one before it, so the ramp is not a ramp", i)
			}
		}
	})
}

// Test_scale tests the scale function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Louder: samples come back multiplied
//   - Quieter: a gain below one comes back smaller
//   - ClampsHigh: a sample that would overflow is held at full scale
//   - ClampsLow: and the same at the negative end
func Test_scale(t *testing.T) {
	// Verify the ordinary case, which is turning the audio up.
	t.Run("Louder", func(t *testing.T) {
		pcm := loudFrame(4, 1000)
		scale(pcm, 2)

		if got := sample(pcm, 0); got != 2000 {
			t.Errorf("a sample of 1000 scaled to %d, want 2000", got)
		}
	})

	// Verify that scaling down works too, since a gain is a number and nothing
	// stops somebody passing a negative one in decibels.
	t.Run("Quieter", func(t *testing.T) {
		pcm := loudFrame(4, 1000)
		scale(pcm, 0.5)

		if got := sample(pcm, 0); got != 500 {
			t.Errorf("a sample of 1000 scaled to %d, want 500", got)
		}
	})

	// Verify that a sample too loud to fit is held at full scale rather than
	// allowed to wrap. A wrapped sample does not come back quieter, it comes
	// back inverted, which is a far worse noise than the loud one.
	t.Run("ClampsHigh", func(t *testing.T) {
		pcm := loudFrame(4, 30000)
		scale(pcm, 4)

		if got := sample(pcm, 0); got != 32767 {
			t.Errorf("an overflowing sample came back as %d, want full scale", got)
		}
	})

	// Verify the same at the negative end, which is a different constant and
	// therefore a different mistake to make.
	t.Run("ClampsLow", func(t *testing.T) {
		pcm := loudFrame(4, -30000)
		scale(pcm, 4)

		if got := sample(pcm, 0); got != -32768 {
			t.Errorf("an overflowing sample came back as %d, want full scale", got)
		}
	})
}
