// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

//go:build windows

package portlock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// errWouldBlock reports that the lock is held by another process, as opposed
// to the lock failing for a reason worth telling somebody about.
var errWouldBlock = errors.New("held by another process")

// lockRegion is the byte range LockFileEx is pointed at: one byte, four
// gigabytes into the file.
//
// Windows range locks are enforced rather than advisory, so a range covering
// the start of the file would stop a refused process reading the description
// of who holds it, which is the one thing it most needs. Locking a byte
// nothing will ever be written to leaves the description readable and still
// gives the two processes a range to contend over.
const (
	lockOffsetHigh = 1 // High 32 bits of the lock offset, placing it 4 GiB in
	lockBytes      = 1 // Length of the contended range in bytes
)

// tryLock takes an exclusive lock on the reserved byte without waiting.
//
// Parameters:
//   - f: open lock file to lock the reserved byte of
//
// Returns:
//   - error if the lock cannot be taken; nil once this process holds it
//
// Errors:
//   - errWouldBlock: if another process already holds the lock
func tryLock(f *os.File) error {
	overlapped := &windows.Overlapped{OffsetHigh: lockOffsetHigh}

	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, lockBytes, 0, overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return errWouldBlock
	}
	return err
}

// unlock gives the lock back.
//
// Parameters:
//   - f: open lock file whose reserved byte this process holds
//
// Returns:
//   - error if the operating system refuses to release the lock
func unlock(f *os.File) error {
	overlapped := &windows.Overlapped{OffsetHigh: lockOffsetHigh}
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockBytes, 0, overlapped)
}
