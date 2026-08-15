// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package screen

// line is one row of the display.
type line struct {
	// Text is the row as displayed, with the trailing spaces that pad it out
	// to the width of the display removed.
	Text string `json:"text"`

	// Highlighted reports whether the row is in reverse video, which is how
	// the scanner shows the selection.
	Highlighted bool `json:"highlighted"`

	// Attributes is how each character of the row is drawn, one character per
	// character of the line: a space for normal, "*" for reverse video, "_"
	// for underline. Empty when the whole row is normal, which is the common
	// case and how the scanner reports it.
	//
	// Highlighted only says a row has reverse video somewhere. This says
	// where, which is what a caller needs to tell one highlighted thing from
	// another on the same row. The soft key labels along the bottom arrive as
	// three separate runs of "*" with normal gaps between them, so their
	// positions come out of this without parsing the text.
	//
	// It can be longer than Text. Both have their trailing spaces removed, and
	// reverse video can extend past the last visible character, which is how
	// the scanner draws a highlight bar across the rest of a row.
	Attributes string `json:"attributes,omitempty"`

	// LargeFont reports whether the scanner drew this row in its large font,
	// which fits 24 characters across the display where the normal font fits
	// 30. The scanner uses it for the system, department and channel names.
	//
	// Anything drawing the screen rather than reading it needs this, because a
	// large character is a quarter wider than a normal one and twice as tall,
	// so 24 of them fill the same width as 30 normal ones. A row of 24 is
	// otherwise indistinguishable from a short row of 30, and drawing it as one
	// leaves the name four fifths of the width and half the height it should
	// be. It is reported on every row rather than only the large ones, because
	// "absent" and "normal" have to be different answers to something laying
	// out every row it is given.
	LargeFont bool `json:"largeFont"`
}

// report is the screen as this command renders it.
type report struct {
	// Lines are the display's rows, top to bottom. Blank rows are kept, so an
	// index into this slice is the row's position on the screen.
	Lines []line `json:"lines"`
}
