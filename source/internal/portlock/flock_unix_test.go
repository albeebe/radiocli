// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

//go:build !windows

package portlock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Test_tryLock tests the tryLock function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: an unlocked file is locked
//   - Held: a file already locked reports that it would block
//   - Error: a file that is no longer open is reported
func Test_tryLock(t *testing.T) {

	// Verify that a free lock file is taken.
	t.Run("Success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "free.lock")
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		if err := tryLock(f); err != nil {
			t.Fatalf("locking: %v", err)
		}
		if err := unlock(f); err != nil {
			t.Fatalf("unlocking: %v", err)
		}
	})

	// Verify that a lock file somebody else holds is not waited on.
	t.Run("Held", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		holder, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer holder.Close()
		if err := tryLock(holder); err != nil {
			t.Fatalf("locking: %v", err)
		}

		// A second open file description contends with the first, which is what
		// a second invocation of the tool would have.
		other, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file again: %v", err)
		}
		defer other.Close()

		if err := tryLock(other); !errors.Is(err, errWouldBlock) {
			t.Fatalf("expected the lock to be held, got %v", err)
		}
	})

	// Verify that a lock that fails for another reason is reported as itself.
	t.Run("Error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "gone.lock")
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("closing the lock file: %v", err)
		}

		err = tryLock(f)
		if err == nil {
			t.Fatal("expected locking a closed file to fail")
		}
		if errors.Is(err, errWouldBlock) {
			t.Fatalf("expected a real error, got %v", err)
		}
	})
}

// Test_unlock tests the unlock function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: a held lock is given back and can be taken again
//   - Error: a file that is no longer open is reported
func Test_unlock(t *testing.T) {

	// Verify that giving the lock back leaves it free for somebody else.
	t.Run("Success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		holder, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer holder.Close()
		if err := tryLock(holder); err != nil {
			t.Fatalf("locking: %v", err)
		}
		if err := unlock(holder); err != nil {
			t.Fatalf("unlocking: %v", err)
		}

		other, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file again: %v", err)
		}
		defer other.Close()
		if err := tryLock(other); err != nil {
			t.Fatalf("expected the lock to be free, got %v", err)
		}
	})

	// Verify that an unlock the system refuses is reported.
	t.Run("Error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "gone.lock")
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("closing the lock file: %v", err)
		}

		if err := unlock(f); err == nil {
			t.Fatal("expected unlocking a closed file to fail")
		}
	})
}
