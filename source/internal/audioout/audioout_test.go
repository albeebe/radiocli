// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audioout

import (
	"time"

	"errors"
	"strings"
	"testing"
)

// fakeOutput is an open sound output that never was, so the parts of this
// package above the library can be tested without one.
type fakeOutput struct {
	closes int // How many times Close was called
}

// Close counts the teardown rather than doing one.
func (f *fakeOutput) Close() { f.closes++ }

// useOpen points Open at a fake for the length of one test.
//
// Parameters:
//   - t: the test to put the real opener back at the end of
//   - fn: what Open should call instead
func useOpen(t *testing.T, fn func(int, func([]byte)) (output, error)) {
	t.Helper()
	previous := openFn
	t.Cleanup(func() { openFn = previous })
	openFn = fn
}

// useSinks points Sinks at a fake for the length of one test.
//
// Parameters:
//   - t: the test to put the real listing back at the end of
//   - fn: what Sinks should call instead
func useSinks(t *testing.T, fn func() ([]Sink, error)) {
	t.Helper()
	previous := listSinksFn
	t.Cleanup(func() { listSinksFn = previous })
	listSinksFn = fn
}

// TestOpen tests the Open function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Opened: the player carries the device that was opened and a ring to feed it
//   - Failed: a device that will not open is reported rather than half opened
//   - TooSmall: a buffer under two frames is refused before anything opens
//   - TooBig: a buffer over half the ring is refused before anything opens
func TestOpen(t *testing.T) {
	// Verify that a successful open hands back a player wired to the device,
	// with the fill callback the device was given reading out of its ring.
	t.Run("Opened", func(t *testing.T) {
		var fill func([]byte)
		useOpen(t, func(_ int, f func([]byte)) (output, error) {
			fill = f
			return &fakeOutput{}, nil
		})

		p, err := Open(primeFrames * FrameMS * time.Millisecond)
		if err != nil {
			t.Fatalf("opening the speakers gave %v, want it to open", err)
		}
		// The callback the device was handed has to be this player's ring, or
		// the audio would go nowhere.
		p.Play(make([]byte, primeFrames*FrameBytes))
		out := make([]byte, FrameBytes)
		for i := range out {
			out[i] = 0xFF
		}
		fill(out)
		for _, b := range out {
			if b != 0 {
				t.Fatal("the device was given a callback that is not this player's ring")
			}
		}
	})

	// Verify that a buffer under two frames is refused before anything opens,
	// since a cushion that small cannot absorb a producer that is one frame
	// late.
	t.Run("TooSmall", func(t *testing.T) {
		useOpen(t, func(int, func([]byte)) (output, error) {
			t.Fatal("a buffer that was refused still opened a device")
			return nil, nil
		})

		if _, err := Open(minBuffer - time.Millisecond); err == nil {
			t.Error("a buffer under two frames was accepted")
		}
	})

	// Verify that a buffer the ring has no room to catch up behind is refused,
	// and refused before a device is taken.
	t.Run("TooBig", func(t *testing.T) {
		useOpen(t, func(int, func([]byte)) (output, error) {
			t.Fatal("a buffer that was refused still opened a device")
			return nil, nil
		})

		if _, err := Open(maxBuffer + time.Millisecond); err == nil {
			t.Error("a buffer over half the ring was accepted")
		}
	})

	// Verify that a failure to open comes back rather than being turned into a
	// player that plays nowhere.
	t.Run("Failed", func(t *testing.T) {
		useOpen(t, func(int, func([]byte)) (output, error) {
			return nil, ErrNoSink
		})

		p, err := Open(DefaultBuffer)
		if !errors.Is(err, ErrNoSink) {
			t.Errorf("opening gave %v, want it to say there is no such speaker", err)
		}
		if p != nil {
			t.Error("a player came back from an open that failed")
		}
	})
}

// Test_periodFor tests the periodFor function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - HalfTheCushion: an ordinary cushion is taken in halves
//   - Capped: a deep cushion does not mean a slower device
//   - Smallest: the smallest cushion Open accepts comes out as one whole frame
func Test_periodFor(t *testing.T) {
	// Verify the ordinary case: half the cushion, in whole frames, so one late
	// callback spends half of what is standing.
	t.Run("HalfTheCushion", func(t *testing.T) {
		if got := periodFor(6); got != 60 {
			t.Errorf("a cushion of 6 frames got a period of %dms, want 60", got)
		}
	})

	// Verify the cap: past 100 ms the lateness stops buying anything, however
	// deep the cushion is.
	t.Run("Capped", func(t *testing.T) {
		if got := periodFor(20); got != 100 {
			t.Errorf("a cushion of 20 frames got a period of %dms, want the cap of 100", got)
		}
	})

	// Verify the bottom of the range: the smallest cushion Open accepts asks
	// the device for exactly one frame at a time.
	t.Run("Smallest", func(t *testing.T) {
		if got := periodFor(2); got != FrameMS {
			t.Errorf("a cushion of 2 frames got a period of %dms, want one frame", got)
		}
	})
}

// TestSinks tests the Sinks function with 100% coverage.
//
// Coverage: 100% (3 test cases covering both branches)
//
// Test cases:
//   - Sorted: the listing comes back in this package's order
//   - Empty: a computer with no outputs is an answer rather than a failure
//   - Failed: an audio system that cannot be asked is reported
func TestSinks(t *testing.T) {
	// Verify that the operating system's order is replaced by one that reads
	// the same way twice.
	t.Run("Sorted", func(t *testing.T) {
		useSinks(t, func() ([]Sink, error) {
			return []Sink{{Name: "external headphones"}, {Name: "MacBook Pro Speakers"}}, nil
		})

		got, err := Sinks()
		if err != nil {
			t.Fatalf("Sinks gave %v, want a listing", err)
		}
		if got[0].Name != "external headphones" || got[1].Name != "MacBook Pro Speakers" {
			t.Errorf("the sinks came out as %v, want them sorted ignoring case", got)
		}
	})

	// Verify that finding nothing is an empty listing rather than an error,
	// because a machine with no speakers is unusual and not broken.
	t.Run("Empty", func(t *testing.T) {
		useSinks(t, func() ([]Sink, error) { return []Sink{}, nil })

		got, err := Sinks()
		if err != nil || len(got) != 0 {
			t.Errorf("Sinks gave %v, %v, want an empty listing and no error", got, err)
		}
	})

	// Verify that an audio system that could not be asked is reported, since
	// that is a different answer from there being nothing attached.
	t.Run("Failed", func(t *testing.T) {
		failed := errors.New("the audio system is not there")
		useSinks(t, func() ([]Sink, error) { return nil, failed })

		if _, err := Sinks(); !errors.Is(err, failed) {
			t.Errorf("Sinks gave %v, want the listing's own failure", err)
		}
	})
}

// TestSortSinks tests the sortSinks function with 100% coverage.
//
// Coverage: 100% (4 test cases covering the comparison and the empty list)
//
// Test cases:
//   - IgnoresCase: names sort without regard to case
//   - NothingPromoted: the system's default does not jump the queue
//   - Prefix: a name that is a prefix of another comes first
//   - Duplicates: two identical devices both survive
func TestSortSinks(t *testing.T) {
	cases := []struct {
		name  string
		sinks []Sink
		want  []string
	}{
		{
			"IgnoresCase",
			[]Sink{{Name: "beta"}, {Name: "Alpha"}},
			[]string{"Alpha", "beta"},
		},
		{
			"NothingPromoted",
			[]Sink{{Name: "Zebra Speakers"}, {Name: "MacBook Pro Speakers"}},
			[]string{"MacBook Pro Speakers", "Zebra Speakers"},
		},
		{
			"Prefix",
			[]Sink{{Name: "Headphones (rear)"}, {Name: "Headphones"}},
			[]string{"Headphones", "Headphones (rear)"},
		},
		{
			// Two identical USB interfaces report one name between them, and
			// collapsing them would hide a device that is really there.
			"Duplicates",
			[]Sink{{Name: "USB Audio CODEC"}, {Name: "MacBook Pro Speakers"}, {Name: "USB Audio CODEC"}},
			[]string{"MacBook Pro Speakers", "USB Audio CODEC", "USB Audio CODEC"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sortSinks(c.sinks)

			got := make([]string, 0, len(c.sinks))
			for _, s := range c.sinks {
				got = append(got, s.Name)
			}
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("the sinks came out as %q, want %q", got, c.want)
			}
		})
	}
}
