// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

//go:build !windows

package portlock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// errWouldBlock reports that the lock is held by another process, as opposed
// to the lock failing for a reason worth telling somebody about.
var errWouldBlock = errors.New("held by another process")

// tryLock takes an exclusive lock on the whole file without waiting.
//
// flock is used rather than the exclusive open the serial library already
// attempts, because that turned out not to hold: measured on macOS, two
// processes both opened the same /dev/cu.usbmodem port and interleaved.
//
// Parameters:
//   - f: open lock file to take the flock on
//
// Returns:
//   - error if the lock cannot be taken; nil once this process holds it
//
// Errors:
//   - errWouldBlock: if another process already holds the lock
func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)

	// EAGAIN and EWOULDBLOCK are the same number here, and either means
	// somebody else has it.
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errWouldBlock
	}
	return err
}

// unlock gives the lock back.
//
// Parameters:
//   - f: open lock file whose flock this process holds
//
// Returns:
//   - error if the operating system refuses to release the lock
func unlock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
