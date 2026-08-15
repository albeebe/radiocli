// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package backlight

import (
	"time"
)

// The menu entries the keypad light setting lives behind, from the top menu
// down. They are selected by name rather than by position, like every other
// menu walk in this tool.
const (
	displayOptions   = "Display Options"
	backlightOptions = "Backlight Options"
	keyBacklight     = "Set Key Backlight"

	enableEntry  = "Enable"
	disableEntry = "Disable"
)

// flipPolls is how many times to look for the light to have changed after the
// key is pressed, and flipGap the wait between looks.
const (
	flipPolls = 20
	flipGap   = 50 * time.Millisecond
)

// keysReport is what the keys subcommand renders.
type keysReport struct {
	// Enabled is whether the keypad lights along with the screen.
	Enabled bool `json:"enabled"`
}

// report is what the bare command renders.
type report struct {
	// On is whether the scanner is lit right now.
	On bool `json:"on"`

	// Level is the brightness while lit, and zero while dark.
	Level int `json:"level"`
}
