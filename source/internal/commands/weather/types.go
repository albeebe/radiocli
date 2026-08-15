// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package weather

import (
	"time"
)

// channelCount is how many weather channels the scanner has. Stepping the knob
// this many times comes back to where it started, which is what bounds the
// sweep.
const channelCount = 7

// held is what the scanner puts in the Hold attribute when it is parked on one
// weather channel rather than working through them.
const held = "On"

// monitorWeather is what the scanner calls the mode that entry starts, and
// weatherAlert is the one it does not. Both are checked, because landing in
// the wrong one is the failure worth naming: it looks like the command worked
// and the radio stays silent.
const (
	monitorWeather = "Monitor Weather"
	weatherAlert   = "Weather Alert"
)

// noSignal is what the scanner reports for signal strength when it is hearing
// nothing at all. It is not a weak reading to be compared against real ones:
// it means there was nothing to read.
const noSignal = -999

// weatherScan is the WX Operation entry that starts the broadcast playing.
//
// The scanner's own menu omits it from the listing the protocol gives, so it
// is found by reading the highlighted row off the display, which is what
// menus.Select does.
const weatherScan = "Weather Scan"

// dwell and dwellGap are how long the sweep sits on each channel and how often
// it reads while it is there.
//
// The scanner reports nothing for the first tenth of a second after the knob
// turns, and a real signal is showing by the time the second reading is taken.
// Sitting longer than this buys nothing: a channel that is going to answer has
// answered.
var (
	dwell    = 500 * time.Millisecond
	dwellGap = 100 * time.Millisecond
)

// settlePolls and settleGap bound the wait for the scanner to redraw into the
// state it was asked for. The scanner keeps reporting the state it is leaving
// until it has, so reading once and believing the answer is how a command that
// worked reports that it did not.
var (
	settlePolls = 40
	settleGap   = 50 * time.Millisecond
)

// turnGap is how long to leave the scanner after the knob turns before reading
// it. The first reading taken sooner than this still describes the channel
// being left.
var turnGap = 150 * time.Millisecond

// channel is one weather channel as the sweep found it.
type channel struct {
	// Number is the channel as the screen labels it, such as "7".
	Number string `json:"number"`

	// Frequency is what it is tuned to, such as "162.525000MHz".
	Frequency string `json:"frequency"`

	// Signal is the strongest reading taken while the scanner sat on it, in
	// dBm. It is absent when nothing was heard, which is not the same as a
	// weak reading.
	Signal *int `json:"signal,omitempty"`

	// Selected marks the one the scanner was left on.
	Selected bool `json:"selected"`
}

// measurement is one weather channel and the best reading taken on it.
type measurement struct {
	// Number is the channel as the screen labels it, and Frequency is what it
	// is tuned to.
	Number    string
	Frequency string

	// Signal is the strongest reading in dBm, or nil when the channel
	// answered with nothing throughout.
	Signal *int
}

// report is what the command and its stop subcommand render.
type report struct {
	// Scanning is whether the scanner is on the weather channels.
	Scanning bool `json:"scanning"`

	// Mode is the scanner's name for what it is doing there, and is empty
	// when it is not there at all.
	Mode string `json:"mode,omitempty"`

	// Receiving is whether the scanner is parked on a channel it can hear.
	Receiving bool `json:"receiving"`

	// Channel and Frequency are the weather channel it was parked on. They are
	// empty when it is not receiving one.
	Channel   string `json:"channel,omitempty"`
	Frequency string `json:"frequency,omitempty"`

	// Signal is the strength of that channel in dBm, and is absent when
	// nothing was heard.
	Signal *int `json:"signal,omitempty"`

	// Channels is every weather channel and what was heard on it, in channel
	// order. It is empty for "weather stop", which measures nothing.
	Channels []channel `json:"channels,omitempty"`
}
