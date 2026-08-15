// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package location

import "time"

// How long to wait for the GPS to work out where it is.
//
// The reads themselves are cheap, a round trip of roughly 30ms, so counting
// attempts alone gives about a second of waiting and calls a working receiver
// dead. The interval is what makes this a wait. A cold receiver indoors took
// around twenty seconds to produce its first fix from a standing start, and
// over fifty after being moved 1,300km by a zip code, which is a cold start as
// far as the receiver is concerned.
const (
	fixAttempts = 90
	fixInterval = time.Second
)

// The menu entries that switch the GPS on and off.
//
// The setting holds one of exactly two values, and the scanner shows the one
// in force as the highlighted entry, so the same screen both sets it and
// reports it.
const (
	gpsMenu        = "GPS"
	setGPSFunction = "Set GPS Function"
	gpsEnable      = "Enable"
	gpsDisable     = "Disable"
)

// menuAttempts is how many times the walk to the location menu is retried
// while the scanner is still busy with a position it was just given.
const menuAttempts = 3

// The range the scanner will be given, in whole miles.
//
// The scanner picks 10 itself when a zip code is entered, which is the only
// value it has been observed to choose. The bounds are this tool's guard
// rather than the scanner's: a range of zero pulls in nothing, and a range
// covering several states stops the database being a filter at all. What the
// scanner itself would refuse has not been established.
//
// Fifty is also the largest the screen can hold, because it takes exactly two
// digits. See setRange for why.
const (
	minRange     = 1
	maxRange     = 50
	defaultRange = 10
)

// outOfRange is what the scanner puts on screen for a zip code it does not
// hold, alongside "Press Any Key".
const (
	outOfRange = "Out of Range"

	// turnOnFullDatabase is the prompt itself, not the words "Full Database".
	// Those appear on the ordinary scanning screen too, as the name of the
	// list being scanned, so matching them alone answers a prompt that is not
	// there and does it once per round.
	turnOnFullDatabase = "Turn on Full Database"
)

// positionTolerance is how far the position read back may sit from the one
// written before it counts as a different place.
//
// The scanner keeps six decimal places to within a millionth of a degree,
// about a tenth of a metre, so this is a hundred times looser than the
// rounding it needs to allow for. A ten thousandth of a degree is roughly
// eleven metres, which is close enough to be the same place and far closer
// than two positions ever are.
const positionTolerance = 0.0001

// promptRounds bounds the prompt loop. Two are known, and the extra round
// costs one screen read while making a third harmless.
const promptRounds = 4

// The menu entries setting a location walks through, matched by name like
// every other step in this tool.
const (
	setYourLocation = "Set Your Location"
	enterZipCode    = "Enter Zip Code"
	setRangeEntry   = "Set Range"

	// countryUSA is the first thing the zip screen asks. Canada is the other
	// option, and a zip is only meaningful against one of them.
	countryUSA = "USA"
)

// settleAttempts is how many reads to give the screen to change. Each read is
// a round trip of roughly 30ms, so this is a fraction of a second.
const settleAttempts = 40

// report is the result this command renders.
type report struct {
	Latitude  float64 `json:"latitude"`  // Degrees north of the equator, as the scanner reports them
	Longitude float64 `json:"longitude"` // Degrees east of the prime meridian, as the scanner reports them

	// Range is the radius in miles the scanner draws database channels from.
	// Zero means no range is set, which is what a scanner following its own
	// GPS reports.
	Range float64 `json:"range"`
}
