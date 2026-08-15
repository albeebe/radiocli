// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package scan

import (
	"time"
)

// modePolls and modeGap bound the wait for the scanner to redraw after being
// told to go back to scanning.
const (
	modePolls = 40
	modeGap   = 50 * time.Millisecond
)

// weatherPolls and weatherGap bound the wait for the scanner to leave the
// weather channels. It keeps reporting them until it has redrawn.
const (
	weatherPolls = 40
	weatherGap   = 50 * time.Millisecond
)
