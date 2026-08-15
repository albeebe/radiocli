// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package battery

// report is the result this command renders.
//
// It is a separate type from device.Battery so the charger state reads as
// words rather than as the number the scanner sends, and so the voltage is
// reported in the unit a person expects.
type report struct {
	// State is what the charger is doing, in words, such as "charging" or
	// "full".
	State string `json:"state"`

	// Charging reports whether charge is going in, which is the one thing
	// about the state most callers want and would otherwise match strings for.
	Charging bool `json:"charging"`

	// Percent is how much charge is left.
	Percent int `json:"percent"`

	// Volts is the pack voltage, which says more about a tired battery than
	// the percentage does.
	Volts float64 `json:"volts"`

	// Milliamps is the current, positive going into the battery and negative
	// coming out of it.
	Milliamps int `json:"milliamps"`

	// Celsius is the battery temperature as the scanner reports it.
	Celsius float64 `json:"celsius"`

	// Fahrenheit is the same temperature converted, since the scanner reports
	// only Celsius and both are worth having.
	Fahrenheit float64 `json:"fahrenheit"`

	// NeedsAction reports whether the charger has stopped on a fault, which is
	// the one reading here that asks somebody to do something.
	NeedsAction bool `json:"needsAction"`
}
