// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package root

import (
	"time"
)

// globalFlags holds the raw values of the persistent flags. They are kept
// separate from Config so PersistentPreRunE can apply them after the config
// file has been read, and only for the flags the user actually typed.
type globalFlags struct {
	// config is the config file to read, empty for the default location. It is
	// the one flag that is applied before the file is read rather than after,
	// since it is what says which file that is.
	config string

	// verbose turns on debug logging.
	verbose bool

	// output is the rendering format for command results, "text" or "json".
	output string

	// device is the serial port of the scanner to talk to, such as
	// /dev/tty.usbmodem14201.
	device string

	// pace is how quickly keys are sent to the scanner.
	pace string

	// wait is how long to wait for another invocation of the tool to finish
	// with the scanner, zero to give up at once.
	wait time.Duration
}
