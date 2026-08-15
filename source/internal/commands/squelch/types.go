// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package squelch

// report is the result both the command and its subcommand render.
//
// The bounds travel with the level because a level on its own says nothing:
// a reader, or a script drawing a slider, needs to know what 2 is out of.
type report struct {
	// Level is the squelch level the scanner reports, from Min to Max.
	Level int `json:"level"`

	// Min is the lowest level the scanner takes, which plays everything
	// including the noise of an empty channel.
	Min int `json:"min"`

	// Max is the highest level the scanner takes, which plays only the
	// strongest signals.
	Max int `json:"max"`
}
