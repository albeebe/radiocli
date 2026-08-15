// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package tune

import (
	"time"

	"github.com/albeebe/radiocli/internal/device"
)

// quickSearch is the screen the scanner reports while holding one frequency.
const quickSearch = "quick_search"

// step is the finest the scanner tunes. The protocol counts hundreds of hertz,
// so nothing smaller survives the trip.
const step = 100 * device.Hertz

// bands are the spans an SDS150 accepts.
//
// They were measured rather than copied from a specification: a frequency
// either side of every edge was sent to the scanner and its answer recorded.
// The scanner reports a search range of 25 to 1300 MHz, but that is a setting
// rather than what the receiver covers, and it refuses plenty inside it.
var bands = []band{
	{25 * device.Megahertz, 512 * device.Megahertz},
	{758 * device.Megahertz, 823_987_500 * device.Hertz},
	{849_012_500 * device.Hertz, 868_987_500 * device.Hertz},
	{894_012_500 * device.Hertz, 960 * device.Megahertz},
	{1240 * device.Megahertz, 1300 * device.Megahertz},
}

// cellular are the two spans missing from the middle of that coverage. They are
// barred from scanners sold in the United States, which is worth saying rather
// than leaving someone to wonder why a frequency between two supported bands is
// refused.
var cellular = []band{
	{824 * device.Megahertz, 849 * device.Megahertz},
	{869 * device.Megahertz, 894 * device.Megahertz},
}

// settlePolls and settleGap bound the wait for the scanner to measure the
// frequency it has just been moved to. Measured on an SDS150, a signal appears
// within a few hundred milliseconds.
var (
	settlePolls = 20
	settleGap   = 100 * time.Millisecond
)

// band is one span the scanner will tune.
type band struct {
	lo, hi device.Frequency // The lowest and highest frequencies the span holds, both included
}

// report is the result this command renders.
type report struct {
	// Megahertz is the frequency the scanner was actually put on, which is the
	// requested one rounded to what the protocol can carry.
	Megahertz float64 `json:"megahertz"`

	// Receiving reports whether anything is being heard on it right now. It is
	// a reading taken the instant after tuning, not a promise about the
	// frequency.
	Receiving bool `json:"receiving"`

	// RSSI is the scanner's own received signal strength figure. Larger, which
	// is to say closer to zero, is stronger. It is absent when the scanner has
	// nothing to report.
	RSSI string `json:"rssi,omitempty"`

	// Bars is how many signal bars the scanner is showing, which is the same
	// reading rounded to something a person can act on.
	Bars string `json:"bars,omitempty"`
}
