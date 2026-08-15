// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"strings"
	"testing"
	"time"
)

// TestPaceDelay tests the Pace Delay method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Known: each named pace has its own gap
//   - Unknown: anything else delays as the default does
func TestPaceDelay(t *testing.T) {
	// Verify that each named pace leaves the gap it promises.
	t.Run("Known", func(t *testing.T) {
		for p, want := range map[Pace]time.Duration{
			PaceSlow:   time.Second,
			PaceMedium: 500 * time.Millisecond,
			PaceFast:   100 * time.Millisecond,
			PaceTurbo:  0,
		} {
			if got := p.Delay(); got != want {
				t.Errorf("%q delays %v, want %v", p, got, want)
			}
		}
	})

	// Verify that a zero value is safe rather than reckless.
	t.Run("Unknown", func(t *testing.T) {
		if got := Pace("").Delay(); got != DefaultPace.Delay() {
			t.Errorf("got %v, want the default's delay", got)
		}
		if got := Pace("glacial").Delay(); got != DefaultPace.Delay() {
			t.Errorf("got %v, want the default's delay", got)
		}
	})
}

// TestPaceDescribe tests the Pace Describe method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Turbo: turbo says it adds no delay rather than quoting a zero
//   - Other: every other pace quotes its gap
func TestPaceDescribe(t *testing.T) {
	// Verify that turbo is described in words rather than as 0s.
	t.Run("Turbo", func(t *testing.T) {
		if got := PaceTurbo.Describe(); got != "turbo (no delay between keys)" {
			t.Errorf("got %q, want the no delay wording", got)
		}
	})

	// Verify that the others quote the gap they leave.
	t.Run("Other", func(t *testing.T) {
		if got := PaceSlow.Describe(); got != "slow (1s between keys)" {
			t.Errorf("got %q, want slow (1s between keys)", got)
		}
	})
}

// TestPaceNames tests the PaceNames function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: every pace is listed slowest first, joined for reading
func TestPaceNames(t *testing.T) {
	// Verify that the list reads as a sentence, slowest first.
	t.Run("Success", func(t *testing.T) {
		if got := PaceNames(); got != "slow, medium, fast or turbo" {
			t.Errorf("got %q, want slow, medium, fast or turbo", got)
		}
	})
}

// TestParsePace tests the ParsePace function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Empty: an unset name gives the default rather than an error
//   - Known: a known name converts
//   - Unknown: anything else is refused, listing what is accepted
func TestParsePace(t *testing.T) {
	// Verify that an unset flag or config key needs no special handling.
	t.Run("Empty", func(t *testing.T) {
		got, err := ParsePace("")
		if err != nil || got != DefaultPace {
			t.Fatalf("got %q, %v, want the default", got, err)
		}
	})

	// Verify that a known name converts.
	t.Run("Known", func(t *testing.T) {
		got, err := ParsePace("medium")
		if err != nil || got != PaceMedium {
			t.Fatalf("got %q, %v, want medium", got, err)
		}
	})

	// Verify that a typo is reported rather than obeyed approximately.
	t.Run("Unknown", func(t *testing.T) {
		_, err := ParsePace("glacial")
		if err == nil || !strings.Contains(err.Error(), "slow, medium, fast or turbo") {
			t.Fatalf("got %v, want the accepted names listed", err)
		}
	})
}

// TestPaceString tests the Pace String method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the pace reads as its name, which is how it is stored
func TestPaceString(t *testing.T) {
	// Verify that the pace reads as the name it is written to config with.
	t.Run("Success", func(t *testing.T) {
		if got := PaceMedium.String(); got != "medium" {
			t.Errorf("got %q, want medium", got)
		}
	})
}

// TestPaceValid tests the Pace Valid method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Known: every pace in Paces is valid
//   - Unknown: anything else is not
func TestPaceValid(t *testing.T) {
	// Verify that every listed pace is accepted.
	t.Run("Known", func(t *testing.T) {
		for _, p := range Paces {
			if !p.Valid() {
				t.Errorf("%q is not valid, want it accepted", p)
			}
		}
	})

	// Verify that anything else is refused.
	t.Run("Unknown", func(t *testing.T) {
		if Pace("glacial").Valid() {
			t.Error("an unknown pace was accepted")
		}
		if Pace("").Valid() {
			t.Error("the empty pace was accepted")
		}
	})
}

// Test_joinWithOr tests the joinWithOr function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - None: no items read as nothing
//   - One: a single item stands alone
//   - Two: two items are joined by or
//   - Many: three or more take commas with or before the last
func Test_joinWithOr(t *testing.T) {
	// Verify that an empty list reads as nothing.
	t.Run("None", func(t *testing.T) {
		if got := joinWithOr(nil); got != "" {
			t.Errorf("got %q, want nothing", got)
		}
	})

	// Verify that one item stands alone.
	t.Run("One", func(t *testing.T) {
		if got := joinWithOr([]string{"slow"}); got != "slow" {
			t.Errorf("got %q, want slow", got)
		}
	})

	// Verify that two items take an or and no comma.
	t.Run("Two", func(t *testing.T) {
		if got := joinWithOr([]string{"slow", "fast"}); got != "slow or fast" {
			t.Errorf("got %q, want slow or fast", got)
		}
	})

	// Verify that a longer list takes commas until the last.
	t.Run("Many", func(t *testing.T) {
		if got := joinWithOr([]string{"a", "b", "c", "d"}); got != "a, b, c or d" {
			t.Errorf("got %q, want a, b, c or d", got)
		}
	})
}
