// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"fmt"
	"time"
)

// Pace is how quickly keys are sent to the scanner.
//
// The scanner is a handheld radio, not a terminal: it processes a keypress,
// redraws its screen, and settles, and pressing the next key before it has
// finished can be ignored or acted on from the wrong screen. Pace is the
// minimum time between one key and the next, so a run of presses arrives at a
// rate the scanner can follow.
type Pace string

// Delay returns the minimum time to leave between two keys. An unrecognised
// pace, including the empty string, delays as DefaultPace does, so a zero
// value is safe rather than reckless.
//
// Returns:
//   - the minimum time between two keys, zero for PaceTurbo
func (p Pace) Delay() time.Duration {
	switch p {
	case PaceSlow:
		return time.Second
	case PaceMedium:
		return 500 * time.Millisecond
	case PaceFast:
		return 100 * time.Millisecond
	case PaceTurbo:
		return 0
	default:
		return DefaultPace.Delay()
	}
}

// Describe returns the pace name and what it means, for showing to a user who
// is choosing between them.
//
// Returns:
//   - the pace name followed by its delay, such as "slow (1s between keys)"
func (p Pace) Describe() string {
	if p == PaceTurbo {
		return "turbo (no delay between keys)"
	}
	return fmt.Sprintf("%s (%v between keys)", string(p), p.Delay())
}

// PaceNames lists the pace names for an error or a flag description.
//
// Returns:
//   - every pace name, slowest first, as "slow, medium, fast or turbo"
func PaceNames() string {
	names := make([]string, 0, len(Paces))
	for _, p := range Paces {
		names = append(names, string(p))
	}
	return joinWithOr(names)
}

// ParsePace converts a name to a Pace, rejecting anything unknown. An empty
// name gives DefaultPace, so an unset flag or config key needs no special
// handling by the caller.
//
// Parameters:
//   - name: the pace name to convert, empty for DefaultPace
//
// Returns:
//   - the matching Pace, or DefaultPace when name is empty
//   - error if name is not one of the known paces
func ParsePace(name string) (Pace, error) {
	if name == "" {
		return DefaultPace, nil
	}

	p := Pace(name)
	if !p.Valid() {
		return "", fmt.Errorf("invalid pace %q: want %s", name, PaceNames())
	}
	return p, nil
}

// String returns the pace name, which is also how it is written to the config
// file. Use Describe for the longer form.
//
// Returns:
//   - the pace name, such as "medium"
func (p Pace) String() string {
	return string(p)
}

// Valid reports whether p is one of the known paces.
//
// Returns:
//   - true if p is one of the paces listed in Paces
func (p Pace) Valid() bool {
	for _, known := range Paces {
		if p == known {
			return true
		}
	}
	return false
}

// joinWithOr renders a list as "a, b, c or d".
//
// Parameters:
//   - items: the items to join, in the order they should be read
//
// Returns:
//   - the items joined by commas with "or" before the last, empty for no
//     items and the item alone for one
func joinWithOr(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}

	out := items[0]
	for _, item := range items[1 : len(items)-1] {
		out += ", " + item
	}
	return out + " or " + items[len(items)-1]
}
