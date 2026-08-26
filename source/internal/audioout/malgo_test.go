// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

//go:build cgo

package audioout

import (
	"errors"
	"strings"
	"testing"
	"unsafe"

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

// TestPlaybackClose tests the playback.Close method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - AlreadyGivenBack: closing a playback holding no handles does nothing
//   - LiveHandles: a running device and its context are both given back
func TestPlaybackClose(t *testing.T) {
	// Verify that closing a playback whose handles are already gone is quiet
	// and safe, which is what makes a second Close harmless.
	t.Run("AlreadyGivenBack", func(t *testing.T) {
		p := &playback{name: "MacBook Pro Speakers"}
		p.Close()
		p.Close()

		if p.dev != nil || p.ctx != nil {
			t.Error("Close left a handle behind on a playback that had none")
		}
		if p.Name() != "MacBook Pro Speakers" {
			t.Error("Close forgot the name of the output it was playing on")
		}
	})

	// Verify that a playback holding a running device and the context it was
	// allocated from gives both back, and that closing it again is harmless.
	t.Run("LiveHandles", func(t *testing.T) {
		useBackend(t, nullBackend)

		out, err := open(nullDeviceName, FrameMS, func([]byte) {})
		if err != nil {
			t.Fatalf("open on the null backend failed: %v", err)
		}

		p, ok := out.(*playback)
		if !ok {
			t.Fatalf("open gave %T, want the library's own playback", out)
		}
		if p.dev == nil || p.ctx == nil {
			t.Fatal("open gave a playback missing a handle, so there is nothing to tear down")
		}

		p.Close()

		if p.dev != nil {
			t.Error("Close left the device behind")
		}
		if p.ctx != nil {
			t.Error("Close left the context behind")
		}

		p.Close()

		if p.Name() != nullDeviceName {
			t.Error("Close forgot the name of the output it was playing on")
		}
	})
}

// Test_defaultName tests the defaultName function with 100% coverage.
//
// The library fills a device's name from C and offers no way to set it from
// outside the package, so what is checked here is which device is picked rather
// than what it is called.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NothingAttached: no devices yields no name
//   - NoneMarked: devices with no default among them yields no name
//   - TheMarkedOne: the device the library marked is the one chosen
func Test_defaultName(t *testing.T) {
	// Verify that a machine reporting no playback devices yields nothing rather
	// than the name of a device that is not there.
	t.Run("NothingAttached", func(t *testing.T) {
		if got := defaultName(nil); got != "" {
			t.Errorf("defaultName gave %q, want nothing", got)
		}
	})

	// Verify that a list with no default in it yields nothing, which is what
	// makes the caller say "the system default" rather than name a device.
	t.Run("NoneMarked", func(t *testing.T) {
		devices := make([]malgo.DeviceInfo, 3)

		if got := defaultName(devices); got != "" {
			t.Errorf("defaultName gave %q, want nothing", got)
		}
	})

	// Verify that the marked device is the one taken, rather than the first one
	// in the list.
	t.Run("TheMarkedOne", func(t *testing.T) {
		devices := make([]malgo.DeviceInfo, 3)
		devices[2].IsDefault = 1

		if got := defaultName(devices); got != devices[2].Name() {
			t.Errorf("defaultName gave %q, want the marked device's name", got)
		}
	})
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

// Test_open tests the open function with partial coverage.
//
// Every case here runs against the library's null backend, a software device
// that needs no sound card, so the machine's real speakers are never opened.
//
// Coverage: 83.3% (4 test cases covering every branch but three)
//
// Three failures are not reachable through this package and are covered in
// Test_openLibraryFailures instead: listing the devices cannot fail on a
// backend that enumerates from a table, and opening or starting the device
// cannot fail on one that accepts every format it is asked for.
//
// Test cases:
//   - AudioSystemUnavailable: a backend the library has none of is reported
//   - NoSinkByThatName: a name nothing answers to gives ErrNoSink
//   - PlaysOnTheNamedSink: the named device comes back running
//   - TheDefaultSink: an empty name opens whatever the system is using
func Test_open(t *testing.T) {
	// Verify that a machine whose audio system cannot be opened is told so,
	// rather than being told the speaker was not found.
	t.Run("AudioSystemUnavailable", func(t *testing.T) {
		useBackend(t, missingBackend)

		got, err := open(nullDeviceName, FrameMS, func([]byte) {})

		if err == nil {
			t.Fatal("open succeeded with no backend to open, want an error")
		}
		if got != nil {
			got.Close()
			t.Error("open gave a playback alongside the error, want nothing")
		}
	})

	// Verify that a name nothing attached answers to fails rather than quietly
	// falling back to the default, which would play the scanner somewhere the
	// person did not ask for.
	t.Run("NoSinkByThatName", func(t *testing.T) {
		useBackend(t, nullBackend)

		got, err := open("Kitchen Radio", FrameMS, func([]byte) {})

		if !errors.Is(err, ErrNoSink) {
			t.Fatalf("open gave %v, want ErrNoSink", err)
		}
		if got != nil {
			got.Close()
			t.Error("open gave a playback alongside the error, want nothing")
		}
	})

	// Verify that the sink asked for is the sink played on, and that it comes
	// back running and under the library's own spelling.
	t.Run("PlaysOnTheNamedSink", func(t *testing.T) {
		useBackend(t, nullBackend)

		got, err := open(nullDeviceName, FrameMS, func([]byte) {})
		if err != nil {
			t.Fatalf("open on the null backend failed: %v", err)
		}
		defer got.Close()

		if got.Name() != nullDeviceName {
			t.Errorf("playing on %q, want %q", got.Name(), nullDeviceName)
		}

		p := got.(*playback)
		if p.dev == nil || !p.dev.IsStarted() {
			t.Error("open gave a device that is not running, so nothing would be heard")
		}
	})

	// Verify that no name at all opens the system's own choice rather than
	// failing, which is the ordinary case here and the opposite of audioin.
	t.Run("TheDefaultSink", func(t *testing.T) {
		useBackend(t, nullBackend)

		got, err := open("", FrameMS, func([]byte) {})
		if err != nil {
			t.Fatalf("open of the default on the null backend failed: %v", err)
		}
		defer got.Close()

		p := got.(*playback)
		if p.dev == nil || !p.dev.IsStarted() {
			t.Error("open gave a device that is not running, so nothing would be heard")
		}
	})
}

// Test_openLibraryFailures covers the three failures in open that the null
// backend cannot produce: listing the devices, opening one, and starting it.
//
// Each is swapped out on its own so the rest of open runs for real, which is
// what proves the error is reported from the step it came from rather than
// being swallowed or mistaken for another.
func Test_openLibraryFailures(t *testing.T) {
	// Verify that a library that will not list its devices is reported.
	t.Run("DeviceListFails", func(t *testing.T) {
		useBackend(t, nullBackend)

		want := errors.New("the audio system went away")
		previous := listDevices
		t.Cleanup(func() { listDevices = previous })
		listDevices = func(*malgo.AllocatedContext, malgo.DeviceType) ([]malgo.DeviceInfo, error) {
			return nil, want
		}

		if _, err := open(nullDeviceName, FrameMS, func([]byte) {}); !errors.Is(err, want) {
			t.Fatalf("open reported %v, want the failed device list", err)
		}
	})

	// Verify that a device which cannot be opened is reported, and named.
	t.Run("InitFails", func(t *testing.T) {
		useBackend(t, nullBackend)

		want := errors.New("the device would not open")
		previous := initDevice
		t.Cleanup(func() { initDevice = previous })
		initDevice = func(malgo.Context, malgo.DeviceConfig, malgo.DeviceCallbacks) (*malgo.Device, error) {
			return nil, want
		}

		_, err := open(nullDeviceName, FrameMS, func([]byte) {})
		if !errors.Is(err, want) {
			t.Fatalf("open reported %v, want the failed device open", err)
		}
		if !strings.Contains(err.Error(), nullDeviceName) {
			t.Errorf("open reported %v, which does not name the speaker it failed on", err)
		}
	})

	// Verify that a device which opens but will not start is reported, and that
	// the device it gave up on is handed back rather than left running.
	t.Run("StartFails", func(t *testing.T) {
		useBackend(t, nullBackend)

		want := errors.New("the device would not start")
		previous := startDevice
		t.Cleanup(func() { startDevice = previous })
		startDevice = func(*malgo.Device) error { return want }

		_, err := open(nullDeviceName, FrameMS, func([]byte) {})
		if !errors.Is(err, want) {
			t.Fatalf("open reported %v, want the failed start", err)
		}
		if !strings.Contains(err.Error(), nullDeviceName) {
			t.Errorf("open reported %v, which does not name the speaker it failed on", err)
		}
	})
}

// Test_playbackCallbacks tests the playbackCallbacks function with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - HandsOutputThrough: the buffer to be filled reaches the caller's function
//   - IgnoresInput: the capture buffer a playback device never receives is
//     dropped
func Test_playbackCallbacks(t *testing.T) {
	// Verify that the library's own buffer is what the caller's function is
	// handed, since filling a copy of it would play silence.
	t.Run("HandsOutputThrough", func(t *testing.T) {
		out := []byte{1, 2, 3, 4}
		var got []byte
		calls := 0

		cb := playbackCallbacks(func(buf []byte) {
			calls++
			got = buf
		})
		if cb.Data == nil {
			t.Fatal("playbackCallbacks gave no Data callback, so nothing would be played")
		}
		cb.Data(out, nil, 1)

		if calls != 1 {
			t.Errorf("the caller's function ran %d times, want once per buffer", calls)
		}
		if &got[0] != &out[0] || len(got) != len(out) {
			t.Errorf("got %v, want the library's own output buffer handed straight through", got)
		}
	})

	// Verify that the capture buffer is dropped rather than passed on, since a
	// playback device never has one.
	t.Run("IgnoresInput", func(t *testing.T) {
		in := []byte{9, 9, 9}
		var got []byte

		cb := playbackCallbacks(func(buf []byte) { got = buf })
		cb.Data(nil, in, 0)

		if got != nil {
			t.Errorf("got %v, want the empty output rather than the input buffer", got)
		}
	})
}

// Test_playbackConfig tests the playbackConfig function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - AsksForThePackagesFormat: rate, format and channel count are the
//     package's own rather than the library's defaults
//   - CarriesTheDeviceID: the identifier handed in is the one asked for
//   - TheDefaultDevice: no identifier is left as no identifier
func Test_playbackConfig(t *testing.T) {
	// Verify that the format the package promises is the one asked of the
	// device, so nothing upstream has to convert anything.
	t.Run("AsksForThePackagesFormat", func(t *testing.T) {
		cfg := playbackConfig(nil, 100)

		if cfg.DeviceType != malgo.Playback {
			t.Errorf("DeviceType is %v, want a playback device", cfg.DeviceType)
		}
		if cfg.SampleRate != SampleRate {
			t.Errorf("SampleRate is %d, want %d", cfg.SampleRate, SampleRate)
		}
		if cfg.Playback.Format != malgo.FormatS16 {
			t.Errorf("Playback.Format is %v, want signed 16-bit", cfg.Playback.Format)
		}
		if cfg.Playback.Channels != Channels {
			t.Errorf("Playback.Channels is %d, want %d", cfg.Playback.Channels, Channels)
		}
		if cfg.PeriodSizeInMilliseconds != 100 {
			t.Errorf("PeriodSizeInMilliseconds is %d, want the period handed in",
				cfg.PeriodSizeInMilliseconds)
		}
	})

	// Verify that the device the caller resolved is the device the config
	// names.
	t.Run("CarriesTheDeviceID", func(t *testing.T) {
		var id malgo.DeviceID
		want := unsafe.Pointer(&id)

		if cfg := playbackConfig(want, FrameMS); cfg.Playback.DeviceID != want {
			t.Error("Playback.DeviceID is not the identifier that was handed in")
		}
	})

	// Verify that no identifier stays no identifier, which is what tells the
	// library to follow the system's own choice of output.
	t.Run("TheDefaultDevice", func(t *testing.T) {
		if cfg := playbackConfig(nil, FrameMS); cfg.Playback.DeviceID != nil {
			t.Error("Playback.DeviceID was filled in, want the library's own default")
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
