// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"errors"
	"strings"
	"testing"
)

// TestFrequencyMHz tests the Frequency MHz method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: hertz are divided down to megahertz, fraction kept
func TestFrequencyMHz(t *testing.T) {
	// Verify that the fraction survives the conversion.
	t.Run("Success", func(t *testing.T) {
		if got := Frequency(155550000).MHz(); got != 155.55 {
			t.Errorf("got %v, want 155.55", got)
		}
		if got := Frequency(0).MHz(); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})
}

// TestFrequencyString tests the Frequency String method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: a frequency is written in megahertz to four places
//   - Zero: no frequency reads as nothing rather than as zero megahertz
func TestFrequencyString(t *testing.T) {
	// Verify that four decimal places are written, which is what the scanner
	// tunes and displays.
	t.Run("Success", func(t *testing.T) {
		if got := Frequency(155550000).String(); got != "155.5500 MHz" {
			t.Errorf("got %q, want 155.5500 MHz", got)
		}
	})

	// Verify that a frequency of zero shows nothing at all.
	t.Run("Zero", func(t *testing.T) {
		if got := Frequency(0).String(); got != "" {
			t.Errorf("got %q, want nothing", got)
		}
	})
}

// TestParseFrequency tests the ParseFrequency function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Units: every way of writing one frequency reads as the same number
//   - NotANumber: text that is not a number at all is reported
//   - NotPositive: zero and negative frequencies are reported apart
//   - TooSmall: a positive frequency finer than the scanner tunes is reported
//     apart again
//   - AnyCase: the case of the unit does not matter, which is what the two
//     parsers this replaced disagreed about
func TestParseFrequency(t *testing.T) {
	// Verify that every way of writing 851.05 MHz reads as the same number of
	// hertz. The last two are what the scanner itself writes back.
	t.Run("Units", func(t *testing.T) {
		for _, text := range []string{
			"851.05", " 851.05 ", "851.050", "851.05MHz", "851.05 mhz",
			"851050khz", "851050000hz", "851.050000MHz", "851.0500000MHZ",
		} {
			got, err := ParseFrequency(text)
			if err != nil {
				t.Errorf("ParseFrequency(%q): %v", text, err)
				continue
			}
			if got != Frequency(851050000) {
				t.Errorf("ParseFrequency(%q) = %d, want 851050000", text, got)
			}
		}
	})

	// Verify that text which is not a number at all is reported as such, and
	// that the failure names what was read.
	t.Run("NotANumber", func(t *testing.T) {
		_, err := ParseFrequency("abc")
		if !errors.Is(err, ErrFrequencyNotANumber) {
			t.Fatalf("reading %q gave %v", "abc", err)
		}
		if !strings.Contains(err.Error(), `"abc"`) {
			t.Errorf("the failure said %q, which does not name what was read", err)
		}
	})

	// Verify that zero and negative frequencies are told apart from nonsense,
	// since the reader who wrote one has a different mistake to fix.
	t.Run("NotPositive", func(t *testing.T) {
		for _, text := range []string{"0", "-1", "-155.475MHz"} {
			if _, err := ParseFrequency(text); !errors.Is(err, ErrFrequencyNotPositive) {
				t.Errorf("reading %q gave %v", text, err)
			}
		}
	})

	// Verify that a frequency too fine to survive the rounding is reported
	// apart from one that was never positive.
	t.Run("TooSmall", func(t *testing.T) {
		if _, err := ParseFrequency("0.000001hz"); !errors.Is(err, ErrFrequencyTooSmall) {
			t.Errorf("reading a frequency below one hertz gave %v", err)
		}
	})

	// Verify that the unit is matched whatever its case. This is the whole of
	// the bug that had one command tune 851.05mhz and another refuse it.
	t.Run("AnyCase", func(t *testing.T) {
		for _, text := range []string{"851.05MHz", "851.05mhz", "851.05MHZ", "851.05Mhz"} {
			got, err := ParseFrequency(text)
			if err != nil || got != Frequency(851050000) {
				t.Errorf("ParseFrequency(%q) = %d, %v", text, got, err)
			}
		}
	})
}

// TestParseEnteredFrequency tests the ParseEnteredFrequency function with 100%
// coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Typeable: a frequency of digits and a decimal point is read and typed
//     exactly as it was written
//   - Rewritten: a frequency carrying a unit is typed as digits instead
//   - NotTypeable: a sign or an exponent is refused as untypeable rather than
//     as unreadable
//   - NotANumber: a value that is neither is still reported for what it is
func TestParseEnteredFrequency(t *testing.T) {
	// Verify that a frequency the screen has keys for is read as usual, and is
	// handed back for typing exactly as it was written. Trailing zeroes and the
	// number of decimal places are what somebody chose to write, and rewriting
	// them would change what the scanner is given for no reason.
	t.Run("Typeable", func(t *testing.T) {
		got, typed, err := ParseEnteredFrequency("851.050")
		if err != nil || got != Frequency(851050000) {
			t.Fatalf("reading a plain frequency gave %d and %v", got, err)
		}
		if typed != "851.050" {
			t.Errorf("it would be typed as %q, want it left as it was written", typed)
		}
	})

	// Verify that a frequency carrying a unit comes back as digits and a
	// decimal point, since the entry screen has no letter keys to type "MHz"
	// with.
	t.Run("Rewritten", func(t *testing.T) {
		for _, text := range []string{"851.05MHz", "851.05 mhz", "851050khz"} {
			got, typed, err := ParseEnteredFrequency(text)
			if err != nil || got != Frequency(851050000) {
				t.Errorf("reading %q gave %d and %v", text, got, err)
				continue
			}
			if typed != "851.05" {
				t.Errorf("%q would be typed as %q, want %q", text, typed, "851.05")
			}
		}
	})

	// Verify that a number the screen has no keys for is refused as that. It
	// parses perfectly well as a number, which is exactly why the reason has to
	// be told apart: the fix is to write it differently, not to write a
	// different frequency.
	t.Run("NotTypeable", func(t *testing.T) {
		for _, text := range []string{"8.5105e2", "+851.05", "-851.05", "8.5105E2"} {
			if _, _, err := ParseEnteredFrequency(text); !errors.Is(err, ErrFrequencyNotTypeable) {
				t.Errorf("reading %q gave %v", text, err)
			}
		}
	})

	// Verify that something that is not a number at all still says so.
	t.Run("NotANumber", func(t *testing.T) {
		if _, _, err := ParseEnteredFrequency("VHF"); !errors.Is(err, ErrFrequencyNotANumber) {
			t.Errorf("reading a value that is not a number gave %v", err)
		}
	})
}

// Test_parseFrequency tests the parseFrequency function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the eight digit encoding is read as hundreds of hertz
//   - Empty: an empty field is no frequency rather than an error
//   - AllZeros: a zero filled field is no frequency either
//   - Invalid: a field holding anything else is reported
func Test_parseFrequency(t *testing.T) {
	// Verify that the encoding counts hundreds of hertz.
	t.Run("Success", func(t *testing.T) {
		got, err := parseFrequency("PWR", "frequency", " 01555500 ")
		if err != nil {
			t.Fatalf("parsing: %v", err)
		}
		if got != Frequency(155550000) {
			t.Errorf("got %v, want 155.55 MHz", got)
		}
	})

	// Verify that an empty field means the scanner had nothing to report.
	t.Run("Empty", func(t *testing.T) {
		got, err := parseFrequency("PWR", "frequency", "   ")
		if err != nil || got != 0 {
			t.Fatalf("got %v, %v, want no frequency", got, err)
		}
	})

	// Verify that a field of nothing but zeros is no frequency either.
	t.Run("AllZeros", func(t *testing.T) {
		got, err := parseFrequency("PWR", "frequency", "00000000")
		if err != nil || got != 0 {
			t.Fatalf("got %v, %v, want no frequency", got, err)
		}
	})

	// Verify that a field that is not digits names itself in the error.
	t.Run("Invalid", func(t *testing.T) {
		_, err := parseFrequency("PWR", "frequency", "0155ABCD")
		if err == nil || !strings.Contains(err.Error(), "invalid frequency") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// Test_frequencyWire tests the Frequency wire method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the frequency is written as eight zero padded digits
func Test_frequencyWire(t *testing.T) {
	// Verify that the wire form is eight digits counting hundreds of hertz.
	t.Run("Success", func(t *testing.T) {
		if got := Frequency(155550000).wire(); got != "01555500" {
			t.Errorf("got %q, want 01555500", got)
		}
		if got := Frequency(0).wire(); got != "00000000" {
			t.Errorf("got %q, want 00000000", got)
		}
	})
}
