// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package volume

// report is the result both the command and its subcommand render.
//
// The bounds travel with the level because a level on its own says nothing:
// a reader, or a script drawing a slider, needs to know what 8 is out of.
type report struct {
	// Level is the volume level the scanner reports, from Min to Max.
	Level int `json:"level"`

	// Min is the lowest level the scanner takes, which is silent.
	Min int `json:"min"`

	// Max is the highest level the scanner takes, which is loudest.
	Max int `json:"max"`
}
