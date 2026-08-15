// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package banks

// Count is how many custom search banks the scanner has. It is fixed in the
// hardware, which is why they are numbered rather than named and why there is
// no way to add one.
const Count = 10

// The entries of one bank's menu, matched by name rather than by position so a
// firmware that reorders them still works.
const (
	entryAttenuator     = "Set Attenuator"
	entryDelay          = "Set Delay Time"
	entryDigitalWait    = "Digital Waiting Time"
	entryLimits         = "Edit Srch Limit"
	entryModulation     = "Set Modulation"
	entryName           = "Edit Name"
	entrySearchWithScan = "Search with Scan"
	entryStep           = "Set Step"
)

// The entries inside Search with Scan, which is a submenu rather than a
// setting. What is behind it is whether this bank is swept as part of ordinary
// scanning, which is a different question from whether it is one of the banks a
// custom search sweeps.
const (
	entryAvoid    = "Set Avoid"
	entryHoldTime = "Set Hold Time"
)

// avoidWords are the words this command takes for the Set Avoid choices, whose
// own names read backwards on a command line: "Stop Avoiding" is the setting
// that means the bank is searched. The scanner's own spellings are accepted too,
// since everything else here takes them.
var avoidWords = map[string]string{
	"off":       "Stop Avoiding",
	"none":      "Stop Avoiding",
	"stop":      "Stop Avoiding",
	"temporary": "Temporary Avoid",
	"temp":      "Temporary Avoid",
	"permanent": "Permanent Avoid",
	"perm":      "Permanent Avoid",
}

// report is one bank, as both output formats renderBanks it.
type report struct {
	// Bank is the bank's number, from 0 to Count-1.
	Bank int `json:"bank"`

	// Name is what the bank is called.
	Name string `json:"name,omitempty"`

	// Lower and Upper are the ends of the range it sweeps, as bare numbers of
	// megahertz.
	Lower string `json:"lower,omitempty"`
	Upper string `json:"upper,omitempty"`

	// Modulation is how it demodulates, such as "AM" or "Auto".
	Modulation string `json:"modulation,omitempty"`

	// Step is the spacing it moves in, written as kilohertz.
	Step string `json:"step,omitempty"`

	// Attenuator, Delay and DigitalWait are the settings the scanner's list
	// will not report, so they are filled in only when the bank's menu was
	// opened.
	Attenuator  string `json:"attenuator,omitempty"`
	Delay       string `json:"delay,omitempty"`
	DigitalWait string `json:"digitalWait,omitempty"`

	// Avoid and HoldTime are the two settings behind Search with Scan: whether
	// ordinary scanning sweeps this bank, and how long it stays on it when it
	// does.
	Avoid    string `json:"avoid,omitempty"`
	HoldTime string `json:"holdTime,omitempty"`
}

// settings are the changes one "banks set" was asked to make. An empty field
// means that setting was not named and is left alone.
type settings struct {
	// rangeFlag is the range to sweep, written as <lower>-<upper> in megahertz.
	rangeFlag string

	// name is what to call the bank.
	name string

	// modulation is how to demodulate, such as "am" or "auto".
	modulation string

	// step is how far apart the frequencies are, such as "10k", or "auto".
	step string

	// attenuator is whether to switch the attenuator on or off.
	attenuator string

	// delay is how long to wait on a transmission before moving on.
	delay string

	// digitalWait is how long to wait for a digital signal to resolve.
	digitalWait string

	// avoid is whether ordinary scanning sweeps this bank, in this command's
	// own words or in the scanner's.
	avoid string

	// holdTime is the seconds ordinary scanning spends on this bank each time
	// round.
	holdTime string
}
