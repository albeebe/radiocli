// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package beep

import (
	"encoding/json"
	"time"
)

// The menu the setting lives on. Settings is reached directly rather than
// walked to from the top, because the protocol names it.
const adjustKeyBeep = "Adjust Key Beep"

// memoryVersion is the format of the file on disk. A file written by an older
// version of the tool is discarded rather than migrated: the worst that costs
// is one toggle that leaves the beep off.
const memoryVersion = 1

// off is the value that silences the keypad, spelled as this command spells it.
const off = "off"

// levels are the seventeen values, in the order the scanner lists them.
//
// Auto first and Off last is the scanner's own order, kept so that anybody
// comparing this list against the radio in hand finds them in the same places.
var levels = buildLevels()

// marshalJSON encodes the settings for writing. It is a var so tests can
// drive the failure the real encoder will not produce for these types.
var marshalJSON = json.MarshalIndent

// level is one of the seventeen settings the key beep can have.
type level struct {
	// entry is the scanner's own wording, such as "Level 9". It is what the
	// menu is searched for and what the highlighted row reads as.
	entry string

	// value is what this command calls it, such as "9". It is what you type
	// and what the JSON reports, because "Level 9" is an awkward thing to pass
	// on a command line.
	value string
}

// memory is the whole of the file on disk.
type memory struct {
	// Version is memoryVersion at the time the file was written.
	Version int `json:"version"`

	// Scanners holds one entry per scanner a toggle has switched off, keyed by
	// scannerKey. Two scanners on one computer remember separately, because
	// the setting is each scanner's own.
	Scanners map[string]remembered `json:"scanners"`
}

// remembered is the setting one scanner's keypad had before it was switched
// off.
type remembered struct {
	// Model and Port are what the scanner was when this was written. The tool
	// never reads them: they are there so somebody who opens the file can see
	// whose setting this is, since the key on its own is opaque.
	Model string `json:"model,omitempty"`
	Port  string `json:"port,omitempty"`

	// Level is the setting to put back, spelled as this command spells it.
	Level string `json:"level"`

	// Noted is when the toggle that stored this ran.
	Noted time.Time `json:"noted"`
}

// report is the result this command renders.
type report struct {
	// Level is the setting, as this command spells it: "auto", "1" to "15", or
	// "off".
	Level string `json:"level"`

	// On reports whether the keypad makes any sound, which is every setting
	// except off. It is here so a caller can ask the question it usually wants
	// without knowing that "auto" is a sound and "off" is not.
	On bool `json:"on"`

	// Remembered is the setting stored for a later "toggle" to put back.
	// Present only on a run that stored one, which is a toggle that switched a
	// sounding keypad off.
	Remembered string `json:"remembered,omitempty"`

	// Restored reports that this run put back a remembered setting rather than
	// choosing one itself.
	Restored bool `json:"restored,omitempty"`
}
