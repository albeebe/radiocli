// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package status

// report is the result this command renders.
type report struct {
	// Port is the serial device the scanner was reached on, which is what gets
	// passed back to the tool as --device.
	Port string `json:"port"`

	// Model is the model string the scanner reports, such as SDS150.
	Model string `json:"model"`

	// Firmware is the firmware version the scanner reports.
	Firmware string `json:"firmware"`

	// Display is how the scanner is drawing its screen: color, black, or
	// white. It is here because it decides whether the colors the scanner
	// stores for each screen element are drawn at all, and nothing else in the
	// protocol says so. See the display command.
	Display string `json:"display"`

	// Mode is what the scanner is doing, in its own words: "Scan Mode",
	// "Scan Hold", "Menu tree" and so on.
	Mode string `json:"mode"`

	// Holding reports whether the scanner is parked on one thing rather than
	// working through a list.
	//
	// It is the same answer as Mode ending in "Hold", spelled out because it is
	// the question worth asking. A held scanner looks exactly like a scanning
	// one from anywhere else: it is out of the menus, it answers everything,
	// and its screen names a channel. Only the mode says which, and someone
	// turning the knob is enough to cause it. Run "radiocli scan" to release
	// it.
	Holding bool `json:"holding"`
}
