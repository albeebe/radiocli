// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package textinput types a value into one of the scanner's text entry
// screens.
//
// The scanner has no command for this. Its own MSV command is refused on these
// screens, and the character keys are refused too, so the only way text goes in
// is the way a person does it: turn the knob to cycle the character under the
// cursor, and press a key to move along. This package does that, working out
// the presses from the value it is given.
//
// It is written against any text entry screen rather than any one of them,
// because renaming a favorites list, a system, and a department are the same
// screen reached three ways.
package textinput

import (
	"context"
	"fmt"
	"strings"

	"github.com/albeebe/radiocli/internal/device"
)

// Set types want into the text entry screen the scanner is currently showing,
// and accepts it.
//
// The scanner must already be on such a screen; putting it there is the
// caller's job, because every screen is reached differently. Nothing is pressed
// if the value is already correct.
//
// Every character is verified after it is set, so a press the scanner missed is
// caught where it happened rather than discovered at the end.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner, already showing a text entry screen
//   - want: the value to leave in the screen
//
// Returns:
//   - error if the scanner is not on a text entry screen, the screen cannot
//     hold want, or any press or read fails
func Set(ctx context.Context, client *device.Scanner, want string) error {
	info, err := client.MenuInfo(ctx)
	if err != nil {
		return fmt.Errorf("reading the text entry screen: %w", err)
	}
	if info.Type != inputType {
		return fmt.Errorf("the scanner is not on a text entry screen: it is showing %q", info.Title)
	}

	order := cycleOrder(info.Input.EnableKeys)

	if err := check(want, order, info.Input.MaxLength); err != nil {
		return err
	}

	// A screen that accepts only digits takes them from the number keys
	// directly, where a screen that accepts letters ignores those keys entirely
	// and has to be turned to each character with the knob. Which one this is
	// can be told from the characters it says it accepts.
	if typed(order) {
		return typeDirect(ctx, client, strings.TrimRight(info.Value, " "), want)
	}

	current := strings.TrimRight(info.Value, " ")
	if current == want {
		// Still accept it, so the caller is left in the same place whether or
		// not anything needed changing.
		return press(ctx, client, commit)
	}

	// Every position either holds a character of the new value or has to be
	// blanked, because the old value may be the longer of the two.
	length := len(want)
	if len(current) > length {
		length = len(current)
	}

	for i := 0; i < length; i++ {
		if err := setChar(ctx, client, i, at(want, i), order); err != nil {
			return err
		}

		// The cursor only moves right, so it is advanced after every position
		// except the last, where advancing would step off the value.
		if i < length-1 {
			if err := press(ctx, client, advance); err != nil {
				return err
			}
		}
	}

	return press(ctx, client, commit)
}

// at returns one byte of a value, or a space when the value is shorter.
//
// Parameters:
//   - value: the value to read from
//   - position: the place in the value to read
//
// Returns:
//   - the byte at that position, or a space when the value does not reach it
func at(value string, position int) byte {
	if position < len(value) {
		return value[position]
	}
	return ' '
}

// charAt returns the character the screen holds at one position, and whether
// the value really reaches that far.
//
// Anything past the end reads as a space, and that space is a guess rather than
// a reading: the cell holds whatever was last in it. Callers need to know the
// difference, because turning the knob from a guessed character goes somewhere
// nobody predicted.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner showing the text entry screen
//   - position: the place in the value to read
//
// Returns:
//   - the character the screen holds there, or a space when the value stops
//     short of it
//   - true when the value really reaches that far, false when the space is a
//     guess
//   - error if the screen cannot be read
func charAt(ctx context.Context, client *device.Scanner, position int) (byte, bool, error) {
	info, err := client.MenuInfo(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("reading the text entry screen: %w", err)
	}
	return at(info.Value, position), position < len(info.Value), nil
}

// check rejects a value the screen cannot hold, before anything is pressed.
//
// Parameters:
//   - want: the value to be entered
//   - order: the characters the screen accepts
//   - maxLength: the longest value the screen holds, or zero when it says no
//     limit
//
// Returns:
//   - error if want is too long, or holds a character the screen does not
//     accept
func check(want, order string, maxLength int) error {
	if maxLength > 0 && len(want) > maxLength {
		return fmt.Errorf("%q is %d characters: this screen holds at most %d",
			want, len(want), maxLength)
	}

	for i := 0; i < len(want); i++ {
		if strings.IndexByte(order, want[i]) < 0 {
			return fmt.Errorf("the scanner does not accept %q on this screen, "+
				"which is character %d of %q", string(want[i]), i+1, want)
		}
	}
	return nil
}

// cycleOrder turns the character set the scanner reports into the order the
// knob actually moves through.
//
// The two are not the same. The scanner lists the space last, and the knob
// reaches it first: blanking a character and turning the knob once gives "A",
// and turning it back gives the space again. Taking the reported order at face
// value sends every hop that crosses the space the wrong way round, which is a
// wrong character rather than a slow one.
//
// Parameters:
//   - enableKeys: the character set the scanner reports, empty when it reports
//     none
//
// Returns:
//   - the characters in the order the knob moves through them
func cycleOrder(enableKeys string) string {
	if enableKeys == "" {
		return fallbackOrder
	}
	if trimmed, found := strings.CutSuffix(enableKeys, " "); found {
		return " " + trimmed
	}
	return enableKeys
}

// press sends one key.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to press the key on
//   - key: the key to press
//
// Returns:
//   - error if the scanner does not take the press
func press(ctx context.Context, client *device.Scanner, key device.Key) error {
	if err := client.PressKey(ctx, key, device.KeyPress); err != nil {
		return fmt.Errorf("pressing a key on the text entry screen: %w", err)
	}
	return nil
}

// route works out how to get from one character to another: how many turns of
// the knob, and which way round is shorter.
//
// Parameters:
//   - from: the character the screen holds now
//   - to: the character it should hold
//   - order: the characters in the order the knob moves through them
//
// Returns:
//   - how many turns of the knob to make
//   - which way to turn it, whichever way round is shorter
//   - error if the screen does not accept the character wanted, or there is no
//     space to count from when the current character is not in the set
func route(from, to byte, order string) (int, device.Key, error) {
	start := strings.IndexByte(order, from)
	end := strings.IndexByte(order, to)

	if end < 0 {
		return 0, "", fmt.Errorf("the scanner does not accept %q on this screen", string(to))
	}
	if start < 0 {
		// The screen holds something outside the set it says it accepts. Going
		// forward still reaches the target, it just cannot be the short way
		// round, so blank it first and count from the space.
		start = strings.IndexByte(order, ' ')
		if start < 0 {
			return 0, "", fmt.Errorf("cannot work out how to reach %q", string(to))
		}
	}

	n := len(order)
	forward := ((end-start)%n + n) % n
	if forward <= n-forward {
		return forward, cycleForward, nil
	}
	return n - forward, cycleBack, nil
}

// setChar makes the character at one position equal to want.
//
// It works by looking rather than by counting. The number of turns is worked
// out from where the character is now, the turns are made, and the result is
// read back; if it is not what was wanted, the whole thing is worked out again
// from wherever it actually landed.
//
// That matters because the cycle does not behave uniformly. Turning the knob
// from a space does not move the number of places the character set says it
// should, so a single calculated hop from a space lands somewhere else
// entirely. Re-reading and re-routing absorbs that without needing to know why
// it happens.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner showing the text entry screen
//   - position: the place in the value to set
//   - want: the character that position should hold
//   - order: the characters in the order the knob moves through them
//
// Returns:
//   - error if the screen cannot be read, a press is refused, the character
//     cannot be reached, or the position still holds something else after
//     every attempt
func setChar(ctx context.Context, client *device.Scanner, position int, want byte, order string) error {
	for round := 0; round < rounds; round++ {
		current, real, err := charAt(ctx, client, position)
		if err != nil {
			return err
		}
		if current == want && real {
			return nil
		}

		// A space is one press, whatever it currently holds, which is cheaper
		// than cycling round to it.
		if want == ' ' {
			if err := press(ctx, client, blank); err != nil {
				return err
			}
			continue
		}

		// A position past the end of the value does not hold what it appears
		// to. Moving the cursor there leaves the previous character in it, and
		// the value the scanner reports simply stops short, so reading it gives
		// a space that is not what the knob will turn from. Routing on that
		// sends the knob most of the way round the cycle and back.
		//
		// One press settles it: whatever the cell really held, it now holds
		// something the value reports, and the next round routes from the
		// truth. The same press fixes a genuine space, which the knob also does
		// not move away from by the number of places the character set implies.
		if !real || current == ' ' {
			if err := press(ctx, client, cycleForward); err != nil {
				return err
			}
			continue
		}

		steps, key, err := route(current, want, order)
		if err != nil {
			return fmt.Errorf("at position %d: %w", position+1, err)
		}

		for i := 0; i < steps; i++ {
			if err := press(ctx, client, key); err != nil {
				return err
			}
		}
	}

	got, _, err := charAt(ctx, client, position)
	if err != nil {
		return err
	}
	return fmt.Errorf("position %d still holds %q after %d attempts at setting it to %q",
		position+1, string(got), rounds, string(want))
}

// typeDirect enters a value on a screen that takes its characters from the
// keypad, pressing one key per character and reading the result back.
//
// It only handles a screen that starts empty, which is what creating something
// gives. Clearing a numeric screen that already holds a value needs a key that
// has not been found, so rather than type digits onto the end of an existing
// number and call it a change, this refuses.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner showing the number entry screen
//   - current: the value the screen already holds
//   - want: the value to leave in the screen
//
// Returns:
//   - error if the screen already holds a different value, want needs a key
//     that has not been found, a press is refused, or the screen does not hold
//     want afterwards
func typeDirect(ctx context.Context, client *device.Scanner, current, want string) error {
	if current == want {
		return press(ctx, client, commit)
	}
	if current != "" {
		return fmt.Errorf("this screen already holds %q and there is no known way to clear it: "+
			"change it on the scanner itself", current)
	}

	// The screen accepts more than the number keys can send. Refuse before
	// pressing anything, rather than typing the part that works and leaving a
	// value that is not the one asked for.
	for i := 0; i < len(want); i++ {
		if strings.IndexByte(keypad, want[i]) < 0 {
			return fmt.Errorf("%q cannot be entered from here, because it holds %q, which this "+
				"screen takes from a key that has not been found: enter it on the scanner itself",
				want, string(want[i]))
		}
	}

	for i := 0; i < len(want); i++ {
		if err := press(ctx, client, device.Key(want[i:i+1])); err != nil {
			return err
		}
	}

	info, err := client.MenuInfo(ctx)
	if err != nil {
		return fmt.Errorf("reading the entry screen back: %w", err)
	}
	if got := strings.TrimRight(info.Value, " "); got != want {
		return fmt.Errorf("the screen holds %q after typing %q: the scanner did not take one of "+
			"the presses", got, want)
	}

	return press(ctx, client, commit)
}

// typed reports whether a screen takes its characters from the number keys
// rather than from the knob.
//
// This asks about the screen rather than about the value, because it decides
// which of two entirely different ways of entering text is used. A screen
// accepting anything outside the numeric set is a name screen, which ignores
// the number keys and has to be turned to each character.
//
// Parameters:
//   - order: the characters the screen accepts
//
// Returns:
//   - true if every character the screen accepts is one the number keys can
//     send
func typed(order string) bool {
	for i := 0; i < len(order); i++ {
		if strings.IndexByte(numeric, order[i]) < 0 {
			return false
		}
	}
	return len(order) > 0
}
