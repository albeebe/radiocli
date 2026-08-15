// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package textinput

import (
	"github.com/albeebe/radiocli/internal/device"
)

// The keys that edit one of these screens. What each does was found by pressing
// it and watching which character of the value changed.
const (
	// cycleForward and cycleBack move the character under the cursor one place
	// along the accepted character set, wrapping at either end.
	cycleForward = device.KeyRotateRight
	cycleBack    = device.KeyRotateLeft

	// advance moves the cursor one place to the right. Nothing found so far
	// moves it back, which is why this package only ever works left to right.
	advance = device.KeySoft3

	// blank sets the character under the cursor to a space in one press,
	// without cycling to it.
	blank = device.KeyNo

	// commit accepts the value and leaves the screen.
	commit = device.KeyEnter
)

// fallbackOrder is the cycle to assume when the scanner does not report its
// character set. It is what an SDS150 cycles through on a name field, measured
// by blanking a character and turning the knob until it came back round.
const fallbackOrder = " ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz1234567890[\\]^_`{|}~!\"#$%&'()*+,-./:;<=>?@"

// inputType is what the scanner calls a text entry screen.
const inputType = "TypeInput"

// keypad is the subset of that which can be pressed as a key in its own right.
// The hyphen and the "i" are put in with soft keys that have not been found,
// so a value needing either cannot be typed from here.
const keypad = "0123456789."

// numeric is the character set the scanner's number entry screens draw from.
//
// The frequency screen accepts "0123456789." and the talkgroup screen accepts
// "0123456789-i", where the hyphen separates the halves of a Motorola or EDACS
// talkgroup and "i" marks an I-call. No name screen looks like this: those
// accept the alphabet, which is what tells the two kinds apart.
const numeric = "0123456789.-i"

// rounds is how many times setChar will look at what it got and try again.
// A character with a known starting point takes one; one that has to be nudged
// into a known state first takes two; the rest is headroom.
const rounds = 5
