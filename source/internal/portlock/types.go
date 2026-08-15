// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package portlock

import (
	"errors"
	"os"
	"time"
)

const (
	// maxDescription bounds what is written about the holding command, so a
	// command line carrying a long name cannot fill the file. It counts
	// characters rather than bytes, so the bound is the same length however
	// the name is spelled.
	maxDescription = 80
)

// ErrBusy reports that another invocation of the tool is using the port.
//
// Callers test for it to tell contention apart from a scanner that is missing
// or silent, which need opposite advice: one resolves itself in a moment, and
// the other needs somebody to go and change something.
var ErrBusy = errors.New("in use by another radiocli")

// releaseLock gives the operating system lock back. It is a var so tests can
// substitute a fake.
var releaseLock = unlock

// retryInterval is how often a waiting command re-tries the lock. It is
// short enough to feel immediate once the holder finishes and long enough
// that waiting minutes for a menu walk costs nothing measurable.
//
// It is a var so tests can substitute a shorter interval.
var retryInterval = 100 * time.Millisecond

// seekFile moves an open file's offset. It is a var so tests can drive the
// failure a handle that has just been truncated does not otherwise produce.
var seekFile = (*os.File).Seek

// takeLock takes the operating system lock on the lock file without waiting.
// It is a var so tests can substitute a fake.
var takeLock = tryLock

// tempDir returns the directory the lock files and sockets are kept under. It
// is a var so tests can substitute a temporary directory.
var tempDir = os.TempDir

// Lock is a held claim on one serial port.
type Lock struct {
	file *os.File // Open lock file whose operating system lock backs the claim
	path string   // Where the lock file lives, named in errors from Release
}
