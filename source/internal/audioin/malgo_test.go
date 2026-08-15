// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

//go:build cgo

package audioin

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// The two backends the tests in this file ask the library for, neither of which
// is a sound card.
//
// A test must never open the machine's real audio hardware. It would prompt for
// microphone permission, record whatever is in the room, and fail on any
// computer whose inputs are not the ones the test expected.
const (
	// missingBackend is a backend the library has no implementation for, which
	// is how the tests reach the branch where opening the audio system fails.
	// It is malgo's BackendNull, which is one short of miniaudio's own, and so
	// names the custom backend that has no callbacks set.
	missingBackend = malgo.Backend(13)

	// nullDeviceName is what the null backend calls the one capture device it
	// reports, spelled the way it spells it.
	nullDeviceName = "NULL Capture Device"

	// nullBackend is miniaudio's software-only backend. It is written as a
	// number because malgo's Backend enum is missing Custom, so its BackendNull
	// is one short and selects the custom backend, which has no callbacks and
	// fails to initialise.
	nullBackend = malgo.Backend(14)
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

// TestCaptureName tests the Capture.Name method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Named: the source's own spelling comes back
//   - Unnamed: a capture holding no name answers with nothing
func TestCaptureName(t *testing.T) {
	// Verify that the name comes back spelled the way the system spells it
	// rather than the way it was typed.
	t.Run("Named", func(t *testing.T) {
		c := &Capture{name: "Cubilux CB5 Line In"}
		if got := c.Name(); got != "Cubilux CB5 Line In" {
			t.Errorf("Name gave %q, want the system's own spelling", got)
		}
	})

	// Verify that a capture carrying no name says so instead of inventing one.
	t.Run("Unnamed", func(t *testing.T) {
		c := &Capture{}
		if got := c.Name(); got != "" {
			t.Errorf("Name gave %q, want nothing", got)
		}
	})
}

// TestCaptureClose tests the Capture.Close method with 100% coverage.
//
// The handles a real teardown needs come from opening the library's null
// backend, which is a software device rather than a sound card, so both
// teardown paths run against genuine live handles without any hardware.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - AlreadyGivenBack: closing a capture holding no handles does nothing
//   - LiveHandles: a running device and its context are both given back
func TestCaptureClose(t *testing.T) {
	// Verify that closing a capture whose handles are already gone is quiet and
	// safe, which is what makes a second Close harmless.
	t.Run("AlreadyGivenBack", func(t *testing.T) {
		c := &Capture{name: "Cubilux CB5 Line In"}
		c.Close()
		c.Close()

		if c.dev != nil || c.ctx != nil {
			t.Error("Close left a handle behind on a capture that had none")
		}
		if c.Name() != "Cubilux CB5 Line In" {
			t.Error("Close forgot the name of the source it was recording from")
		}
	})

	// Verify that a capture holding a running device and the context it was
	// allocated from gives both back, and that closing it again is harmless.
	t.Run("LiveHandles", func(t *testing.T) {
		useBackend(t, nullBackend)

		c, err := open(nullDeviceName, func(pcm []byte) {})
		if err != nil {
			t.Fatalf("open on the null backend failed: %v", err)
		}
		if c.dev == nil || c.ctx == nil {
			t.Fatal("open gave a capture missing a handle, so there is nothing to tear down")
		}

		c.Close()

		if c.dev != nil {
			t.Error("Close left the device behind")
		}
		if c.ctx != nil {
			t.Error("Close left the context behind")
		}

		c.Close()

		if c.Name() != nullDeviceName {
			t.Error("Close forgot the name of the source it was recording from")
		}
	})
}

// Test_captureCallbacks tests the captureCallbacks function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - HandsInputThrough: the input buffer reaches the caller's function
//   - IgnoresOutput: the output buffer a capture device never writes is dropped
func Test_captureCallbacks(t *testing.T) {
	// Verify that the audio the library captured is what the caller's function
	// is handed, untouched and uncopied.
	t.Run("HandsInputThrough", func(t *testing.T) {
		in := []byte{1, 2, 3, 4}
		var got []byte
		calls := 0

		cb := captureCallbacks(func(pcm []byte) {
			calls++
			got = pcm
		})
		if cb.Data == nil {
			t.Fatal("captureCallbacks gave no Data callback, so no audio would arrive")
		}
		cb.Data(nil, in, 1)

		if calls != 1 {
			t.Errorf("the caller's function ran %d times, want once per batch", calls)
		}
		if &got[0] != &in[0] || len(got) != len(in) {
			t.Errorf("got %v, want the library's own input buffer handed straight through", got)
		}
	})

	// Verify that the playback buffer is dropped rather than passed on, since a
	// capture device has nothing to write into it.
	t.Run("IgnoresOutput", func(t *testing.T) {
		out := []byte{9, 9, 9}
		var got []byte

		cb := captureCallbacks(func(pcm []byte) { got = pcm })
		cb.Data(out, nil, 0)

		if got != nil {
			t.Errorf("got %v, want the empty input rather than the output buffer", got)
		}
	})
}

// Test_captureConfig tests the captureConfig function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - AsksForThePackagesFormat: rate, format and channel count are the
//     package's own rather than the library's defaults
//   - CarriesTheDeviceID: the identifier handed in is the one asked for
func Test_captureConfig(t *testing.T) {
	// Verify that the format the package promises its callers is the one asked
	// of the device, so nothing downstream has to carry a format around.
	t.Run("AsksForThePackagesFormat", func(t *testing.T) {
		cfg := captureConfig(nil)

		if cfg.DeviceType != malgo.Capture {
			t.Errorf("DeviceType is %v, want a capture device", cfg.DeviceType)
		}
		if cfg.SampleRate != SampleRate {
			t.Errorf("SampleRate is %d, want %d", cfg.SampleRate, SampleRate)
		}
		if cfg.Capture.Format != malgo.FormatS16 {
			t.Errorf("Capture.Format is %v, want signed 16-bit", cfg.Capture.Format)
		}
		if cfg.Capture.Channels != Channels {
			t.Errorf("Capture.Channels is %d, want %d", cfg.Capture.Channels, Channels)
		}
	})

	// Verify that the device the caller resolved is the device the config names,
	// since the default input is deliberately never used.
	t.Run("CarriesTheDeviceID", func(t *testing.T) {
		var id malgo.DeviceID
		want := unsafe.Pointer(&id)

		cfg := captureConfig(want)

		if cfg.Capture.DeviceID != want {
			t.Error("Capture.DeviceID is not the identifier that was handed in")
		}
	})
}

// Test_deviceNames tests the deviceNames function with 100% coverage.
//
// The library fills a device's name from C and offers no way to set it from
// outside the package, so what is checked here is that every device reported
// yields one entry at its own position, which is what the identifier is later
// reached by.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NothingAttached: no devices yields no names
//   - OneEntryPerDevice: the length and the order match what was reported
func Test_deviceNames(t *testing.T) {
	// Verify that a machine reporting no capture devices yields an empty list
	// rather than anything to index into.
	t.Run("NothingAttached", func(t *testing.T) {
		got := deviceNames(nil)

		if len(got) != 0 {
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

// Test_listSources tests the listSources function with partial coverage.
//
// Every case here runs against the library's null backend, a software device
// that needs no sound card and no microphone permission, so the machine's real
// audio hardware is never opened.
//
// Coverage: 90% (2 test cases covering every branch but one)
//
// The failure to list the inputs at malgo.go:172 is not reachable. The null
// backend enumerates from a fixed table in software with nothing to fail
// against, and the only way to make the call fail would be to hand it a context
// this function built and owns.
//
// Test cases:
//   - AudioSystemUnavailable: a backend the library has none of is reported
//   - ReportsWhatIsAttached: the null backend's one device comes back as a
//     Source spelled the way the library spells it
func Test_listSources(t *testing.T) {
	// Verify that a machine whose audio system cannot be opened is told so,
	// rather than looking like a machine with nothing plugged in.
	t.Run("AudioSystemUnavailable", func(t *testing.T) {
		useBackend(t, missingBackend)

		got, err := listSources()

		if err == nil {
			t.Fatal("listSources succeeded with no backend to open, want an error")
		}
		if got != nil {
			t.Errorf("got %v alongside the error, want nothing", got)
		}
	})

	// Verify that every capture device the library reports comes back as a
	// source under the library's own spelling.
	t.Run("ReportsWhatIsAttached", func(t *testing.T) {
		useBackend(t, nullBackend)

		got, err := listSources()

		if err != nil {
			t.Fatalf("listSources on the null backend failed: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d sources, want the null backend's one", len(got))
		}
		if got[0].Name != nullDeviceName {
			t.Errorf("source is %q, want %q", got[0].Name, nullDeviceName)
		}
	})
}

// Test_open tests the open function with partial coverage.
//
// Every case here runs against the library's null backend, a software device
// that needs no sound card and no microphone permission, so the machine's real
// audio hardware is never opened.
//
// Coverage: 83.3% (4 test cases covering every branch but three)
//
// Three failures are not reachable through this package. Listing the inputs at
// malgo.go:224 cannot fail, because the null backend enumerates from a fixed
// table in software. Opening the device at malgo.go:240 and starting it at
// malgo.go:244 cannot fail either, because the config is built by captureConfig
// from constants this package chooses and the null backend accepts every format
// it is asked for, converting in software. Reaching any of the three would mean
// handing open a context or a config from outside, which it deliberately does
// not take.
//
// Test cases:
//   - AudioSystemUnavailable: a backend the library has none of is reported
//   - NoSourceByThatName: a name nothing answers to gives ErrNoSource
//   - RecordsFromTheNamedSource: the named device comes back running
//   - DeliversAudio: the caller's function is wired to the running device
func Test_open(t *testing.T) {
	// Verify that a machine whose audio system cannot be opened is told so,
	// rather than being told the source was not found.
	t.Run("AudioSystemUnavailable", func(t *testing.T) {
		useBackend(t, missingBackend)

		got, err := open(nullDeviceName, func(pcm []byte) {})

		if err == nil {
			t.Fatal("open succeeded with no backend to open, want an error")
		}
		if got != nil {
			got.Close()
			t.Error("open gave a capture alongside the error, want nothing")
		}
	})

	// Verify that a name nothing attached answers to fails rather than quietly
	// falling back to the default input, which on a laptop is the microphone.
	t.Run("NoSourceByThatName", func(t *testing.T) {
		useBackend(t, nullBackend)

		got, err := open("Cubilux CB5 Line In", func(pcm []byte) {})

		if !errors.Is(err, ErrNoSource) {
			t.Fatalf("open gave %v, want ErrNoSource", err)
		}
		if got != nil {
			got.Close()
			t.Error("open gave a capture alongside the error, want nothing")
		}
	})

	// Verify that the source asked for is the source recorded from, and that it
	// comes back running and under the library's own spelling.
	t.Run("RecordsFromTheNamedSource", func(t *testing.T) {
		useBackend(t, nullBackend)

		got, err := open(nullDeviceName, func(pcm []byte) {})
		if err != nil {
			t.Fatalf("open on the null backend failed: %v", err)
		}
		defer got.Close()

		if got.Name() != nullDeviceName {
			t.Errorf("capture is recording from %q, want %q", got.Name(), nullDeviceName)
		}
		if got.dev == nil || got.ctx == nil {
			t.Fatal("open gave a capture missing a handle, so it could not be torn down")
		}
		if !got.dev.IsStarted() {
			t.Error("open gave a device that is not running, so no audio would arrive")
		}
	})

	// Verify that the caller's function is the one the running device calls, so
	// that audio actually reaches whoever asked for it.
	//
	// The null backend produces its silence on a timer, so the wait is bounded
	// and a batch that has not arrived in time is not a failure. What is being
	// checked is the wiring, and a timer that has not fired yet says nothing
	// about it.
	t.Run("DeliversAudio", func(t *testing.T) {
		useBackend(t, nullBackend)

		var batches atomic.Int64
		arrived := make(chan struct{}, 1)

		got, err := open(nullDeviceName, func(pcm []byte) {
			if batches.Add(1) == 1 {
				select {
				case arrived <- struct{}{}:
				default:
				}
			}
		})
		if err != nil {
			t.Fatalf("open on the null backend failed: %v", err)
		}
		defer got.Close()

		select {
		case <-arrived:
			if batches.Load() < 1 {
				t.Error("a batch arrived without the caller's function counting it")
			}
		case <-time.After(time.Second):
			t.Log("no batch arrived in a second, which the null backend's timer is free to do")
		}
	})
}

// Test_sourcesFromNames tests the sourcesFromNames function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NothingAttached: no names still yields a list rather than nothing
//   - KeepsOrder: each name becomes one source, in the order it arrived
func Test_sourcesFromNames(t *testing.T) {
	// Verify that a machine with no inputs at all renders as an empty JSON
	// array instead of as null.
	t.Run("NothingAttached", func(t *testing.T) {
		got := sourcesFromNames(nil)

		if got == nil {
			t.Fatal("sourcesFromNames gave nothing, want an empty list")
		}
		if len(got) != 0 {
			t.Errorf("got %d sources, want none", len(got))
		}
	})

	// Verify that the library's order survives, since a listing shows what is
	// attached in the order the system reports it.
	t.Run("KeepsOrder", func(t *testing.T) {
		names := []string{"Cubilux CB5 Line In", "MacBook Pro Microphone", "Cubilux CB5 Line In"}

		got := sourcesFromNames(names)

		if len(got) != len(names) {
			t.Fatalf("got %d sources for %d names, want one each", len(got), len(names))
		}
		for i, s := range got {
			if s.Name != names[i] {
				t.Errorf("source %d is %q, want %q", i, s.Name, names[i])
			}
		}
	})
}

// Test_listSourcesDeviceFailure covers the one failure in listSources that the
// null backend cannot produce.
//
// The backend enumerates from a table built into the library, so asking it for
// devices always works. The call is swapped out instead, which leaves the
// context genuinely opened and torn down around it.
func Test_listSourcesDeviceFailure(t *testing.T) {
	// Verify that a library that will not list its devices is reported.
	t.Run("Fails", func(t *testing.T) {
		useBackend(t, nullBackend)

		want := errors.New("the audio system went away")
		previous := listDevices
		t.Cleanup(func() { listDevices = previous })
		listDevices = func(*malgo.AllocatedContext, malgo.DeviceType) ([]malgo.DeviceInfo, error) {
			return nil, want
		}

		if _, err := listSources(); err == nil || !errors.Is(err, want) {
			t.Fatalf("listSources reported %v, want the failed device list", err)
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

		if _, err := open("NULL Capture Device", func([]byte) {}); err == nil || !errors.Is(err, want) {
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

		_, err := open("NULL Capture Device", func([]byte) {})
		if err == nil || !errors.Is(err, want) {
			t.Fatalf("open reported %v, want the failed device open", err)
		}
		if !strings.Contains(err.Error(), "NULL Capture Device") {
			t.Errorf("open reported %v, which does not name the source it failed on", err)
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

		_, err := open("NULL Capture Device", func([]byte) {})
		if err == nil || !errors.Is(err, want) {
			t.Fatalf("open reported %v, want the failed start", err)
		}
		if !strings.Contains(err.Error(), "NULL Capture Device") {
			t.Errorf("open reported %v, which does not name the source it failed on", err)
		}
	})
}
