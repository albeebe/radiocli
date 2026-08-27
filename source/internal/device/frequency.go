// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Frequency is a radio frequency in hertz.
//
// The protocol sends frequencies as an eight digit integer counting hundreds
// of hertz, so 01555500 means 155.55 MHz. Callers should never see that
// encoding: they get a Frequency, and this type converts at the boundary.
type Frequency int64

// MHz returns the frequency in megahertz.
//
// Returns:
//   - the frequency in megahertz, fraction included
func (f Frequency) MHz() float64 {
	return float64(f) / float64(Megahertz)
}

// ParseEnteredFrequency reads a frequency that is about to be typed into one
// of the scanner's frequency entry screens, and says how to type it.
//
// It is ParseFrequency with one more question asked: can this be typed at all?
// The entry screen has keys for the digits and the decimal point and nothing
// else, so a sign or an exponent is not a frequency the scanner could be given
// however valid a number it is. Checking here means a mistyped frequency costs
// nothing and leaves the scanner where it was, rather than being discovered
// halfway through an entry screen with a half-made channel on it.
//
// What comes back for typing is what was written, whenever what was written is
// already digits and a decimal point. Only a frequency carrying a unit gets
// rewritten, because it has to be: rewriting one that did not need it would
// change what the scanner is given, and what the scanner is given is the part
// that has been proven against the hardware.
//
// Parameters:
//   - text: the frequency as it was typed
//
// Returns:
//   - the frequency, read as megahertz unless the text carried a unit
//   - the text to type into the entry screen, in digits and a decimal point
//   - error wrapping ErrFrequencyNotTypeable when it could not be typed, or
//     whatever ParseFrequency returns when it could not be read
func ParseEnteredFrequency(text string) (Frequency, string, error) {
	if strings.ContainsAny(text, "+-eE") {
		return 0, "", fmt.Errorf("%q is %w", text, ErrFrequencyNotTypeable)
	}

	f, err := ParseFrequency(text)
	if err != nil {
		return 0, "", err
	}

	trimmed := strings.TrimSpace(text)
	if strings.IndexFunc(trimmed, func(r rune) bool { return r != '.' && (r < '0' || r > '9') }) < 0 {
		return f, trimmed, nil
	}
	return f, strconv.FormatFloat(f.MHz(), 'f', -1, 64), nil
}

// ParseFrequency reads a frequency written by a person or written back by the
// scanner, with or without a unit.
//
// A bare number means megahertz, which is how frequencies are spoken and how
// every example in the documentation writes them. The suffixes "MHz", "kHz"
// and "Hz" are accepted in any case, with or without a space in front, and
// "851.050000MHz" is the form the scanner itself sends back.
//
// This lives here, beside the type it produces, because it used to live in two
// commands at once. They drifted, as two implementations of one idea do: one
// took a unit suffix in any case and the other trimmed the exact string "MHz",
// so "851.05mhz" tuned the radio but was refused by the command that adds a
// frequency to a site. One user-facing idea, one parser.
//
// Parameters:
//   - text: the frequency as it was typed or as the scanner wrote it
//
// Returns:
//   - the frequency, read as megahertz unless the text carried a unit
//   - error wrapping ErrFrequencyNotANumber, ErrFrequencyNotPositive or
//     ErrFrequencyTooSmall, for the caller to word for its own reader
func ParseFrequency(text string) (Frequency, error) {
	lowered := strings.ToLower(strings.TrimSpace(text))

	// Longest suffix first, so "mhz" is not read as "hz".
	unit := Megahertz
	for _, suffix := range []string{"mhz", "khz", "hz"} {
		if cut, found := strings.CutSuffix(lowered, suffix); found {
			unit = units[suffix]
			lowered = strings.TrimSpace(cut)
			break
		}
	}

	value, err := strconv.ParseFloat(lowered, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is %w", text, ErrFrequencyNotANumber)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%q: %w", text, ErrFrequencyNotPositive)
	}

	// Rounded rather than truncated, since a float cannot hold 852.3625
	// exactly and truncating would land one hertz low.
	f := Frequency(math.Round(value * float64(unit)))
	if f <= 0 {
		return 0, fmt.Errorf("%q: %w", text, ErrFrequencyTooSmall)
	}
	return f, nil
}

// String renders the frequency in megahertz to four decimal places, which is
// the resolution the scanner tunes and displays.
//
// Returns:
//   - the frequency written in megahertz, or "" when there is no frequency to
//     show
func (f Frequency) String() string {
	if f == 0 {
		return ""
	}
	return fmt.Sprintf("%.4f MHz", f.MHz())
}

// parseFrequency reads the scanner's eight digit encoding. An empty field
// means the scanner had no frequency to report, which is not an error.
//
// Parameters:
//   - command: the command being parsed, named in any error
//   - field: what the field holds, named in any error
//   - value: the field as the scanner sent it, padded and zero filled
//
// Returns:
//   - the frequency, zero when the field is empty or all zeros
//   - error if the field holds something other than digits
func parseFrequency(command, field, value string) (Frequency, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	n, err := strconv.ParseInt(strings.TrimLeft(value, "0"), 10, 64)
	if err != nil {
		if strings.Trim(value, "0") == "" {
			return 0, nil
		}
		return 0, fmt.Errorf("response to %q has an invalid %s: %q", command, field, value)
	}
	return Frequency(n) * frequencyUnit, nil
}

// wire renders the frequency in the eight digit form the scanner expects.
//
// Returns:
//   - the frequency as eight zero padded digits counting hundreds of hertz
func (f Frequency) wire() string {
	return fmt.Sprintf("%08d", int64(f/frequencyUnit))
}
