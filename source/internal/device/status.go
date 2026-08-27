// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// LED is the colour of one of the scanner's indicator lights.
type LED int

// Mode is what the scanner is doing, as far as the remote protocol is
// concerned. It is coarse: every scanning and searching screen is ModeNormal.
type Mode int

// DisplayMode is whether the scanner draws its screen in color.
//
// It is a setting, kept under MENU -> Display Options -> Set B/W or Color Mode,
// and it applies to the whole display rather than to one screen.
type DisplayMode int

// Backlight reports whether the scanner's light is on.
//
// It comes from the last field of the status response, which is the only place
// the scanner says so. Measured on an SDS150: pressing the light key moves the
// field between 3 and 0, it holds whichever it was left on for as long as it
// was watched, and 3 matches the dimmer setting rather than meaning "on".
//
// The keypad's own light is not in here. It is a menu setting, and this field
// reads the same whether it is enabled or not.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Backlight reporting whether the light is on and what the dimmer is set
//     to
//   - error if the exchange fails, the reply carries no backlight field, or
//     the level is not a number within the range the scanner uses
func (s *Scanner) Backlight(ctx context.Context) (Backlight, error) {
	value, err := s.conn.Execute(ctx, "STS")
	if err != nil {
		return Backlight{}, err
	}

	_, rest, err := parseDisplay("STS", strings.Split(value, ","))
	if err != nil {
		return Backlight{}, err
	}
	if len(rest) == 0 {
		return Backlight{}, errMissingField("STS", "backlight")
	}

	last := strings.TrimSpace(rest[len(rest)-1])
	level, err := strconv.Atoi(last)
	if err != nil {
		return Backlight{}, fmt.Errorf("response to %q has an invalid backlight: %q", "STS", last)
	}
	if level < 0 || level > backlightLevels {
		return Backlight{}, fmt.Errorf("response to %q reports the backlight as %d, "+
			"which is outside 0 to %d", "STS", level, backlightLevels)
	}

	return Backlight{On: level > 0, Level: level}, nil
}

// Display returns the scanner's screen as text.
//
// This is the cheapest way to see what the scanner is doing, and it works in
// every mode, including menus where most other commands are refused. Use
// Screen instead when the indicator lights or the mode matter.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Display holding the screen's lines, top to bottom, with the blank ones
//     kept
//   - error if the exchange fails or the reply cannot be parsed
func (s *Scanner) Display(ctx context.Context) (Display, error) {
	value, err := s.conn.Execute(ctx, "STS")
	if err != nil {
		return Display{}, err
	}

	d, _, err := parseDisplay("STS", strings.Split(value, ","))
	return d, err
}

// DisplayMode reports whether the scanner is drawing its screen in color.
//
// This is a read over the wire and works in every mode, including menus, which
// is what separates it from every other display setting: the per-element colors
// can only be reached by walking the menus, and walking stops the scan.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - the display mode the scanner reports, one of DisplayColor,
//     DisplayBlackBackground or DisplayWhiteBackground
//   - error if the exchange fails or the reply cannot be parsed
func (s *Scanner) DisplayMode(ctx context.Context) (DisplayMode, error) {
	scr, err := s.Screen(ctx)
	if err != nil {
		return 0, err
	}
	return scr.DisplayMode, nil
}

// Screen returns the scanner's screen along with its indicator lights, its
// mode, and the waterfall's tuning.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Screen holding the display, the mute state, the indicator lights, the
//     mode, the display mode, and the waterfall's frequencies and settings
//   - error if the exchange fails, the reply cannot be parsed, or it carries
//     fewer status fields than the command needs
func (s *Scanner) Screen(ctx context.Context) (Screen, error) {
	value, err := s.conn.Execute(ctx, "GST")
	if err != nil {
		return Screen{}, err
	}

	display, rest, err := parseDisplay("GST", strings.Split(value, ","))
	if err != nil {
		return Screen{}, err
	}
	if len(rest) < 12 {
		return Screen{}, errShortResponse("GST", len(rest), 12, "status fields")
	}

	scr := Screen{Display: display}
	scr.Muted = strings.TrimSpace(rest[0]) == "1"
	scr.AlertLED = LED(optionalDigits(rest[1]))
	scr.ChargeLED = LED(optionalDigits(rest[2]))
	scr.Mode = Mode(optionalDigits(rest[3]))
	scr.DisplayMode = DisplayMode(optionalDigits(rest[10]))
	scr.Waterfall = Waterfall{
		Marked:         Frequency(optionalDigits(rest[4])) * frequencyUnit,
		Modulation:     strings.TrimSpace(rest[5]),
		MarkerPosition: optionalDigits(rest[6]),
		Center:         Frequency(optionalDigits(rest[7])) * frequencyUnit,
		Lower:          Frequency(optionalDigits(rest[8])) * frequencyUnit,
		Upper:          Frequency(optionalDigits(rest[9])) * frequencyUnit,
		FFTSize:        optionalDigits(rest[11]),
	}

	return scr, nil
}

// String returns the mode name, as this tool names it rather than as the menu
// spells it.
//
// Returns:
//   - the mode as "color", "black" or "white", or "unknown(n)" for a value
//     this package does not name
func (m DisplayMode) String() string {
	switch m {
	case DisplayColor:
		return "color"
	case DisplayBlackBackground:
		return "black"
	case DisplayWhiteBackground:
		return "white"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// String returns the colour name, lowercased.
//
// Returns:
//   - the colour name, such as "off" or "magenta", or "unknown(n)" for a value
//     outside the colours the scanner can show
func (l LED) String() string {
	names := [...]string{"off", "blue", "red", "magenta", "green", "cyan", "yellow", "white"}
	if l < 0 || int(l) >= len(names) {
		return fmt.Sprintf("unknown(%d)", int(l))
	}
	return names[l]
}

// String returns the mode name.
//
// Returns:
//   - the mode as "normal", "waterfall" or "menu", or "unknown(n)" for a value
//     this package does not name
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeWaterfall:
		return "waterfall"
	case ModeMenu:
		return "menu"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// errMissingField reports a reply missing a field entirely.
//
// Parameters:
//   - command: the command that was sent, for the error message
//   - what: the name of the field the reply should have carried
//
// Returns:
//   - error naming the command and the missing field
func errMissingField(command, what string) error {
	return fmt.Errorf("response to %q has no %s", command, what)
}

// errShortResponse reports a reply with fewer fields than the command needs.
//
// Parameters:
//   - command: the command that was sent, for the error message
//   - got: how many fields the reply carried
//   - want: how many fields the command needs at least
//   - what: what the fields are, for the error message
//
// Returns:
//   - error naming the command, the count it carried, and the count needed
func errShortResponse(command string, got, want int, what string) error {
	return fmt.Errorf("response to %q has %d %s, want at least %d", command, got, what, want)
}

// optionalDigits parses a field of digits that the scanner leaves empty when it
// does not apply. An empty field, or one holding anything but digits, becomes
// zero rather than an error, because most of these fields are blank in every
// mode but one and a blank one is not a fault.
//
// It reads digits and nothing else, which is the whole of its contract and the
// reason it is not called optionalInt. There is no sign, so "-5" is zero rather
// than minus five, and no partial reading, so "12x" is zero rather than twelve.
// Every field it serves is an unsigned count or an enumeration the scanner
// writes in digits; a signed field must not be routed through it, because it
// would come back as zero with nothing said.
//
// Parameters:
//   - s: the field as the scanner sent it, which may be empty or padded
//
// Returns:
//   - the field as a number, or zero if it is empty or holds anything that is
//     not a digit
func optionalDigits(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
