// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package devices

import (
	"github.com/albeebe/radiocli/internal/device"
)

// discover finds the scanners attached to this computer. It is a var so tests
// can substitute a fake and never enumerate a real serial port.
var discover = device.Discover

// entry is one row of the listing: a discovered scanner plus whether another
// invocation is holding it.
//
// A busy entry carries its port and nothing else. Model and Serial are read
// from the scanner, and reading them means opening the port, which is the one
// thing that cannot be done to a port somebody else has.
type entry struct {
	device.Info
	Busy bool `json:"busy"` // Whether another invocation is holding the port

	// Shared says a daemon is holding the port and will run commands on it for
	// anybody who asks. It is the difference between a scanner to wait for and
	// a scanner to use: a busy port that is shared takes commands now, they
	// just queue behind whatever else is running.
	Shared bool `json:"shared,omitempty"`
}
