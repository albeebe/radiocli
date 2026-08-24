// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audioout

import (
	"bytes"
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

// primed builds a ring with enough audio in it to have started playing, which
// is where most of the interesting behaviour begins.
//
// Returns:
//   - *ring holding exactly primeFrames of audio, not yet read from
func primedRing() *ring {
	r := newRing()
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
		p := &Player{out: out, ring: newRing()}

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
		p := &Player{out: out, ring: newRing()}

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
		p := &Player{out: &fakeOutput{name: "MacBook Pro Speakers"}, ring: newRing()}
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
	t.Run("Queued", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing()}
		want := tone(0, primeFrames*FrameBytes)
		p.Play(want)

		got := make([]byte, len(want))
		p.ring.read(got)
		if !bytes.Equal(got, want) {
			t.Error("what came out of the ring is not what was played into it")
		}
	})

	// Verify that the samples are copied. The frames handed in here often
	// belong to a capture callback's own buffer, which is written over the
	// moment it returns.
	t.Run("Copied", func(t *testing.T) {
		p := &Player{out: &fakeOutput{}, ring: newRing()}
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
		p := &Player{out: &fakeOutput{}, ring: newRing()}
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
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Unprimed: nothing plays until the cushion has been built
//   - Primes: the cushion arriving starts the playing
//   - Wraps: audio split across the end of the buffer comes out in one piece
//   - Starves: a ring that runs dry plays silence and says so
//   - Reprimes: after running dry the cushion has to be built again
//   - FillsEverything: the library's buffer is never left part written
func TestRingRead(t *testing.T) {
	// Verify that a ring holding less than the cushion plays silence rather
	// than handing over what little it has, which would leave it empty again
	// immediately.
	t.Run("Unprimed", func(t *testing.T) {
		r := newRing()
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
	// nothing was lost while it was being built.
	t.Run("Primes", func(t *testing.T) {
		r := primedRing()

		out := make([]byte, FrameBytes)
		r.read(out)
		if !bytes.Equal(out, tone(0, FrameBytes)) {
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
		r.read(got)
		if !bytes.Equal(got, tone(100, len(r.buf))) {
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
		r := newRing()

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
		r := newRing()
		r.write(nil)
		if r.length != 0 {
			t.Errorf("%d bytes are waiting after writing nothing", r.length)
		}
	})

	// Verify that a write which runs off the end of the buffer continues at the
	// beginning rather than being cut short.
	t.Run("Wraps", func(t *testing.T) {
		r := newRing()
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
		r := newRing()
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
		r := newRing()
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
		r := newRing()
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
	r := newRing()

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
