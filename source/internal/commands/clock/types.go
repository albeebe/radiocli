// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package clock

import (
	"time"
)

// tolerance is how far the scanner may sit from the time it was given before
// the difference is worth reporting. Writing the clock and reading it back
// takes long enough to cross a second boundary, and the scanner keeps no
// resolution finer than a second, so a small difference means nothing.
const tolerance = 3 * time.Second

// dateLayouts are the forms giving a date alone.
var dateLayouts = []string{
	"2006-01-02",
}

// fullLayouts are the forms giving both a date and a time of day, tried in
// order. Each is parsed in the computer's own zone, because the scanner holds
// wall clock digits rather than an instant.
var fullLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
}

// timeLayouts are the forms giving a time of day alone.
var timeLayouts = []string{
	"15:04:05",
	"15:04",
}

// report is the result these commands render.
//
// It is a separate type from device.Clock so the date and the time of day are
// reported as the separate values a person reads them as, and so the clock's
// health reads as words rather than as a flag named after the protocol.
type report struct {
	// Date is the scanner's date, written as the scanner holds it rather than
	// converted to a zone.
	Date string `json:"date"`

	// Time is the scanner's time of day, to the second, which is the finest
	// resolution the scanner keeps.
	Time string `json:"time"`

	// DaylightSaving reports whether the scanner is applying the extra hour.
	DaylightSaving bool `json:"daylightSaving"`

	// Valid reports whether the clock is still running. A scanner left without
	// power long enough loses it, and the digits above mean nothing when this
	// is false.
	Valid bool `json:"valid"`
}

// value is a date and time as the user wrote it, remembering which halves were
// actually given. A half that was left out is filled from the scanner rather
// than invented, so setting the date never disturbs the time and setting the
// time never disturbs the date.
type value struct {
	// t is what was parsed, in the computer's own zone. Only the halves the
	// user actually gave are meaningful.
	t time.Time

	// hasDate reports whether the user gave a date.
	hasDate bool

	// hasTime reports whether the user gave a time of day.
	hasTime bool
}
