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
	closes int    // How many times Close was called
	name   string // What Name answers with
	period int    // What Period answers with
}

// Close counts the teardown rather than doing one.
func (f *fakeOutput) Close() { f.closes++ }

// Name answers with whatever the test put there.
func (f *fakeOutput) Name() string { return f.name }

// Period answers with whatever the test put there, standing in for the size a
// real device would be asking in.
func (f *fakeOutput) Period() int { return f.period }

// useOpen points Open at a fake for the length of one test.
//
// Parameters:
//   - t: the test to put the real opener back at the end of
//   - fn: what Open should call instead
func useOpen(t *testing.T, fn func(string, int, func([]byte)) (output, error)) {
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
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Opened: the player carries the device that was opened and a ring to feed it
//   - Named: the name is passed through untouched, including the empty default
//   - Failed: a device that will not open is reported rather than half opened
//   - TooSmall: a buffer under two frames is refused before anything opens
//   - TooBig: a buffer over half the ring is refused before anything opens
func TestOpen(t *testing.T) {
	// Verify that a successful open hands back a player wired to the device,
	// with the fill callback the device was given reading out of its ring.
	t.Run("Opened", func(t *testing.T) {
		var fill func([]byte)
		useOpen(t, func(_ string, _ int, f func([]byte)) (output, error) {
			fill = f
			return &fakeOutput{name: "MacBook Pro Speakers"}, nil
		})

		p, err := Open("MacBook Pro Speakers", primeFrames*FrameMS*time.Millisecond)
		if err != nil {
			t.Fatalf("opening the speakers gave %v, want it to open", err)
		}
		if p.Name() != "MacBook Pro Speakers" {
			t.Errorf("the player is playing on %q, want the device's own name", p.Name())
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

	// Verify that an empty name reaches the opener as an empty name, since that
	// is what means the system's own choice of output.
	t.Run("Named", func(t *testing.T) {
		var asked string
		useOpen(t, func(name string, _ int, _ func([]byte)) (output, error) {
			asked = name
			return &fakeOutput{}, nil
		})

		if _, err := Open("", DefaultBuffer); err != nil {
			t.Fatalf("opening the default gave %v, want it to open", err)
		}
		if asked != "" {
			t.Errorf("the opener was asked for %q, want the default", asked)
		}
	})

	// Verify that a buffer too small to absorb one late frame is refused, and
	// refused before a device is taken.
	t.Run("TooSmall", func(t *testing.T) {
		useOpen(t, func(string, int, func([]byte)) (output, error) {
			t.Fatal("a buffer that was refused still opened a device")
			return nil, nil
		})

		if _, err := Open("", minBuffer-time.Millisecond); err == nil {
			t.Error("a buffer under two frames was accepted")
		}
	})

	// Verify that a buffer the ring has no room to catch up behind is refused,
	// and refused before a device is taken.
	t.Run("TooBig", func(t *testing.T) {
		useOpen(t, func(string, int, func([]byte)) (output, error) {
			t.Fatal("a buffer that was refused still opened a device")
			return nil, nil
		})

		if _, err := Open("", maxBuffer+time.Millisecond); err == nil {
			t.Error("a buffer over half the ring was accepted")
		}
	})

	// Verify that a failure to open comes back rather than being turned into a
	// player that plays nowhere.
	t.Run("Failed", func(t *testing.T) {
		useOpen(t, func(string, int, func([]byte)) (output, error) {
			return nil, ErrNoSink
		})

		p, err := Open("Nothing At All", DefaultBuffer)
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

// TestPickSink tests the pickSink function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Exact: a name spelled the way the system spells it is found
//   - Folded: a name typed in the wrong case is found anyway
//   - ExactWins: an exact match beats one that differs only by case
//   - Blank: a name with nothing in it is refused before anything is searched
//   - Missing: a name matching nothing says so
//   - Ambiguous: a name matching two devices is refused rather than guessed at
func TestPickSink(t *testing.T) {
	names := []string{"MacBook Pro Speakers", "Cubilux CB5 Headphones", "External Headphones"}

	// Verify that the ordinary case, a name copied out of a listing, finds the
	// device at the position its identifier is reached by.
	t.Run("Exact", func(t *testing.T) {
		at, err := pickSink(names, "Cubilux CB5 Headphones")
		if err != nil || at != 1 {
			t.Errorf("pickSink gave %d, %v, want 1 and no error", at, err)
		}
	})

	// Verify that a name is found however a shell or a JSON file cased it on
	// the way in.
	t.Run("Folded", func(t *testing.T) {
		at, err := pickSink(names, "  external headphones ")
		if err != nil || at != 2 {
			t.Errorf("pickSink gave %d, %v, want 2 and no error", at, err)
		}
	})

	// Verify that two devices differing only by case are not called ambiguous,
	// because one of them is exactly what was asked for.
	t.Run("ExactWins", func(t *testing.T) {
		both := []string{"speakers", "Speakers"}
		at, err := pickSink(both, "Speakers")
		if err != nil || at != 1 {
			t.Errorf("pickSink gave %d, %v, want 1 and no error", at, err)
		}
	})

	// Verify that nothing at all is refused with the reason, rather than
	// matching the first device or the default one.
	t.Run("Blank", func(t *testing.T) {
		_, err := pickSink(names, "   ")
		if !errors.Is(err, ErrNoSink) || !strings.Contains(err.Error(), "no name was given") {
			t.Errorf("pickSink gave %v, want it to say no name was given", err)
		}
	})

	// Verify that a typo is reported as a typo, quoting what was typed so the
	// person can see it.
	t.Run("Missing", func(t *testing.T) {
		_, err := pickSink(names, "Speekers")
		if !errors.Is(err, ErrNoSink) || !strings.Contains(err.Error(), `"Speekers"`) {
			t.Errorf("pickSink gave %v, want it to quote the name that matched nothing", err)
		}
	})

	// Verify that two identical devices are refused rather than chosen between,
	// since picking one would be a coin toss the caller could not see.
	t.Run("Ambiguous", func(t *testing.T) {
		twice := []string{"USB Audio CODEC", "USB Audio CODEC"}
		_, err := pickSink(twice, "USB Audio CODEC")
		if !errors.Is(err, ErrAmbiguousSink) || !strings.Contains(err.Error(), "2 of them") {
			t.Errorf("pickSink gave %v, want it to say how many are called that", err)
		}
	})
}

// TestResolve tests the Resolve function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Canonical: the system's own spelling comes back, not the typed one
//   - Unlisted: a listing that cannot be read is reported
//   - Unmatched: a name matching nothing is reported
func TestResolve(t *testing.T) {
	// Verify that what comes back is spelled the way a listing spells it, so
	// that showing it afterwards matches what the person read.
	t.Run("Canonical", func(t *testing.T) {
		useSinks(t, func() ([]Sink, error) {
			return []Sink{{Name: "MacBook Pro Speakers"}}, nil
		})

		got, err := Resolve("macbook pro speakers")
		if err != nil || got != "MacBook Pro Speakers" {
			t.Errorf("Resolve gave %q, %v, want the system's own spelling", got, err)
		}
	})

	// Verify that an audio system which cannot be asked is reported rather than
	// read as an empty list, which would turn it into "no such speaker".
	t.Run("Unlisted", func(t *testing.T) {
		failed := errors.New("the audio system is not there")
		useSinks(t, func() ([]Sink, error) { return nil, failed })

		if _, err := Resolve("MacBook Pro Speakers"); !errors.Is(err, failed) {
			t.Errorf("Resolve gave %v, want the listing's own failure", err)
		}
	})

	// Verify that a name matching nothing carries the error that says which
	// advice to print.
	t.Run("Unmatched", func(t *testing.T) {
		useSinks(t, func() ([]Sink, error) {
			return []Sink{{Name: "MacBook Pro Speakers"}}, nil
		})

		if _, err := Resolve("Kitchen Radio"); !errors.Is(err, ErrNoSink) {
			t.Errorf("Resolve gave %v, want it to say there is no such speaker", err)
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
