// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package display

import (
	"github.com/albeebe/radiocli/internal/device"
)

// The menu entries the setting lives behind, from the top menu down. They are
// selected by name rather than by position, like every other menu walk here.
const (
	displayOptions = "Display Options"
	colorMode      = "Set B/W or Color Mode"
)

// modes are the three the scanner offers, in the order its menu lists them.
var modes = []mode{
	{"color", device.DisplayColor, "Color Mode", "each element in the colors set for it"},
	{"black", device.DisplayBlackBackground, "Black w/White Text", "white text on a black background"},
	{"white", device.DisplayWhiteBackground, "White w/Black Text", "black text on a white background"},
}

// mode is one of the ways the scanner can draw its screen, under the name this
// command accepts for it.
//
// The names are short because they are typed. The scanner's own wording is kept
// alongside so the walk can find the entry, and so output can quote what is
// really on the screen.
type mode struct {
	// name is what this command calls the mode.
	name string

	// value is what the scanner reports for it.
	value device.DisplayMode

	// entry is how the menu spells it.
	entry string

	// description is what the mode looks like.
	description string
}

// report is the result this command renders.
type report struct {
	// Mode is the mode's short name: color, black, or white.
	Mode string `json:"mode"`

	// Color reports whether the per-element colors are being drawn, which is
	// the question most callers are really asking.
	Color bool `json:"color"`

	// Entry is how the scanner's own menu spells the mode.
	Entry string `json:"entry"`
}
