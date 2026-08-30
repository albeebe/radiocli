// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

//go:build cgo

package audioout

import (
	"errors"
	"testing"

	"github.com/gen2brain/malgo"
)

// The two backends the tests in this file ask the library for, neither of which
// is a sound card.
//
// A test must never open the machine's real speakers. It would play whatever
// the test produced into the room, and fail on any computer whose outputs are
// not the ones the test expected.
const (
	// missingBackend is a backend the library has no implementation for, which
	// is how the tests reach the branch where opening the audio system fails.
	// It is malgo's BackendNull, which is one short of miniaudio's own, and so
	// names the custom backend that has no callbacks set.
	missingBackend = malgo.Backend(13)

	// nullBackend is miniaudio's software-only backend. It is written as a
	// number because malgo's Backend enum is missing Custom, so its BackendNull
	// is one short and selects the custom backend, which has no callbacks and
	// fails to initialise.
	nullBackend = malgo.Backend(14)

	// nullDeviceName is what the null backend calls the one playback device it
	// reports, spelled the way it spells it.
	nullDeviceName = "NULL Playback Device"
)

// useBackend points the package at one backend for the length of one test.
//
// The previous list is put back afterwards, because it is package state that
// every other test in the file reads.
//
// Parameters:
//   - t: the test to restore the previous backend list at the end of
//   - b: the backend to ask the library for
func useBackend(t *testing.T, b malgo.Backend) {
	t.Helper()
	previous := backends
	t.Cleanup(func() { backends = previous })
	backends = []malgo.Backend{b}
}

// Test_deviceNames tests the deviceNames function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NothingAttached: no devices yields no names
//   - OneEntryPerDevice: the length and the order match what was reported
func Test_deviceNames(t *testing.T) {
	// Verify that a machine reporting no playback devices yields an empty list
	// rather than anything to index into.
	t.Run("NothingAttached", func(t *testing.T) {
		if got := deviceNames(nil); len(got) != 0 {
			t.Errorf("got %d names, want none", len(got))
		}
	})

	// Verify that the position of a name is the position of the device it came
	// from, which is what makes the identifier reachable afterwards.
	t.Run("OneEntryPerDevice", func(t *testing.T) {
		devices := make([]malgo.DeviceInfo, 3)
		devices[1].IsDefault = 1

		got := deviceNames(devices)

		if len(got) != len(devices) {
			t.Fatalf("got %d names for %d devices, want one each", len(got), len(devices))
		}
		for i, name := range got {
			if name != devices[i].Name() {
				t.Errorf("name %d is %q, want %q", i, name, devices[i].Name())
			}
		}
	})
}

// Test_listSinks tests the listSinks function with partial coverage.
//
// Every case here runs against the library's null backend, a software device
// that needs no sound card, so the machine's real speakers are never opened.
//
// Coverage: 90% (2 test cases covering every branch but one)
//
// The failure to list the outputs is covered separately, in
// Test_listSinksDeviceFailure, because the null backend enumerates from a fixed
// table in software and has nothing to fail against.
//
// Test cases:
//   - AudioSystemUnavailable: a backend the library has none of is reported
//   - ReportsWhatIsAttached: the null backend's one device comes back as a Sink
func Test_listSinks(t *testing.T) {
	// Verify that a machine whose audio system cannot be opened is told so,
	// rather than looking like a machine with nothing attached.
	t.Run("AudioSystemUnavailable", func(t *testing.T) {
		useBackend(t, missingBackend)

		got, err := listSinks()

		if err == nil {
			t.Fatal("listSinks succeeded with no backend to open, want an error")
		}
		if got != nil {
			t.Errorf("got %v alongside the error, want nothing", got)
		}
	})

	// Verify that every playback device the library reports comes back as a
	// sink under the library's own spelling.
	t.Run("ReportsWhatIsAttached", func(t *testing.T) {
		useBackend(t, nullBackend)

		got, err := listSinks()

		if err != nil {
			t.Fatalf("listSinks on the null backend failed: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d sinks, want the null backend's one", len(got))
		}
		if got[0].Name != nullDeviceName {
			t.Errorf("sink is %q, want %q", got[0].Name, nullDeviceName)
		}
	})
}

// Test_listSinksDeviceFailure covers the one failure in listSinks that the null
// backend cannot produce.
//
// The backend enumerates from a table built into the library, so asking it for
// devices always works. The call is swapped out instead, which leaves the
// context genuinely opened and torn down around it.
func Test_listSinksDeviceFailure(t *testing.T) {
	// Verify that a library that will not list its devices is reported.
	t.Run("Fails", func(t *testing.T) {
		useBackend(t, nullBackend)

		want := errors.New("the audio system went away")
		previous := listDevices
		t.Cleanup(func() { listDevices = previous })
		listDevices = func(*malgo.AllocatedContext, malgo.DeviceType) ([]malgo.DeviceInfo, error) {
			return nil, want
		}

		if _, err := listSinks(); !errors.Is(err, want) {
			t.Fatalf("listSinks reported %v, want the failed device list", err)
		}
	})
}

// Test_sinksFromNames tests the sinksFromNames function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NothingAttached: no names still yields a list rather than nothing
//   - KeepsOrder: each name becomes one sink, in the order it arrived
func Test_sinksFromNames(t *testing.T) {
	// Verify that a machine with no outputs at all renders as an empty JSON
	// array instead of as null.
	t.Run("NothingAttached", func(t *testing.T) {
		got := sinksFromNames(nil)

		if got == nil {
			t.Fatal("sinksFromNames gave nothing, want an empty list")
		}
		if len(got) != 0 {
			t.Errorf("got %d sinks, want none", len(got))
		}
	})

	// Verify that the library's order survives, since the sort that replaces it
	// happens further out.
	t.Run("KeepsOrder", func(t *testing.T) {
		names := []string{"USB Audio CODEC", "MacBook Pro Speakers", "USB Audio CODEC"}

		got := sinksFromNames(names)

		if len(got) != len(names) {
			t.Fatalf("got %d sinks for %d names, want one each", len(got), len(names))
		}
		for i, s := range got {
			if s.Name != names[i] {
				t.Errorf("sink %d is %q, want %q", i, s.Name, names[i])
			}
		}
	})
}
