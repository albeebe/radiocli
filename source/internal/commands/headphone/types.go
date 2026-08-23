// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package headphone

// entryName is the setting's own wording in the Settings menu. Settings is
// reached directly rather than walked to from the top, because the protocol
// names it.
const entryName = "Headphone L/R output"

// The two values the setting can have, as this command spells them.
//
// The scanner's own wording is "In Phase" and "Invert Phase". These are the
// same words in the form you would type, since a command line argument with a
// space in it is a thing to quote and then to get wrong.
const (
	// InPhase sends the same audio to both sides of the jack.
	InPhase = "in-phase"

	// Invert sends one side inverted, which is what cancels the audio when
	// something folds the two sides together.
	Invert = "invert-phase"
)

// phases are the two settings, in the order the scanner lists them.
var phases = []phase{
	{entry: "In Phase", value: InPhase},
	{entry: "Invert Phase", value: Invert},
}

// phase is one of the two settings the headphone output can have.
type phase struct {
	// entry is the scanner's own wording, such as "Invert Phase". It is what
	// the menu is searched for and what the highlighted row reads as.
	entry string

	// value is what this command calls it, such as "invert-phase". It is what
	// you type and what the JSON reports.
	value string
}

// report is the result this command renders.
type report struct {
	// Phase is the setting the scanner is on: "in-phase" or "invert-phase".
	Phase string `json:"phase"`
}
