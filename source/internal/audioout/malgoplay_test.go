// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/30/2026

//go:build cgo && !darwin && !windows

package audioout

import (
	"errors"
	"testing"

	"github.com/gen2brain/malgo"
)

// useInitDevice makes opening a device fail for the length of one test.
//
// Parameters:
//   - t: the test to put the real call back at the end of
//   - err: what opening should fail with
func useInitDevice(t *testing.T, err error) {
	t.Helper()
	previous := initDevice
	t.Cleanup(func() { initDevice = previous })
	initDevice = func(malgo.Context, malgo.DeviceConfig, malgo.DeviceCallbacks) (*malgo.Device, error) {
		return nil, err
	}
}

// useStartDevice makes starting a device fail for the length of one test.
//
// Parameters:
//   - t: the test to put the real call back at the end of
//   - err: what starting should fail with
func useStartDevice(t *testing.T, err error) {
	t.Helper()
	previous := startDevice
	t.Cleanup(func() { startDevice = previous })
	startDevice = func(*malgo.Device) error { return err }
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
		p := &playback{}
		p.Close()
		p.Close()

		if p.dev != nil || p.ctx != nil {
			t.Error("Close left a handle behind on a playback that had none")
		}
	})

	// Verify that a playback holding a running device and the context it was
	// allocated from gives both back, and that closing it again is harmless.
	t.Run("LiveHandles", func(t *testing.T) {
		useBackend(t, nullBackend)

		out, err := openMalgo(FrameMS, func([]byte) {})
		if err != nil {
			t.Fatalf("opening on the null backend failed: %v", err)
		}

		p, ok := out.(*playback)
		if !ok {
			t.Fatalf("opening gave %T, want the library's own playback", out)
		}
		if p.dev == nil || p.ctx == nil {
			t.Fatal("opening gave a playback missing a handle, so there is nothing to tear down")
		}

		p.Close()

		if p.dev != nil {
			t.Error("Close left the device behind")
		}
		if p.ctx != nil {
			t.Error("Close left the context behind")
		}

		p.Close()
	})
}

// Test_openMalgo tests the openMalgo function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Opens: the device is opened and started, filling from the caller
//   - AudioSystemUnavailable: a machine whose audio system will not open is told
//   - InitFails: a device that will not open is reported rather than half opened
//   - StartFails: a device that opens but will not start is reported
func Test_openMalgo(t *testing.T) {
	// Verify the ordinary path, on the null backend so no sound card is touched
	// and nothing is heard in the room the tests are running in.
	t.Run("Opens", func(t *testing.T) {
		useBackend(t, nullBackend)

		out, err := openMalgo(FrameMS, func([]byte) {})
		if err != nil {
			t.Fatalf("opening on the null backend failed: %v", err)
		}
		if out == nil {
			t.Fatal("opening gave nothing back and no error")
		}
		out.Close()
	})

	// Verify that a machine whose audio system cannot be opened at all is told
	// so, rather than being handed speakers that will never make a sound.
	t.Run("AudioSystemUnavailable", func(t *testing.T) {
		useBackend(t, missingBackend)

		out, err := openMalgo(FrameMS, func([]byte) {})

		if err == nil {
			t.Fatal("opening succeeded with no backend to open, want an error")
		}
		if out != nil {
			t.Error("opening gave a playback as well as an error")
		}
	})

	// Verify that a device which will not open is reported, and that nothing
	// half opened is handed back with the error.
	t.Run("InitFails", func(t *testing.T) {
		useBackend(t, nullBackend)
		useInitDevice(t, errors.New("no such device"))

		out, err := openMalgo(FrameMS, func([]byte) {})

		if err == nil {
			t.Fatal("a device that would not open reported nothing")
		}
		if out != nil {
			t.Error("opening gave a playback as well as an error")
		}
	})

	// Verify that a device which opens but will not start is reported too,
	// since a caller handed one would sit waiting for audio that never plays.
	t.Run("StartFails", func(t *testing.T) {
		useBackend(t, nullBackend)
		useStartDevice(t, errors.New("the device would not start"))

		out, err := openMalgo(FrameMS, func([]byte) {})

		if err == nil {
			t.Fatal("a device that would not start reported nothing")
		}
		if out != nil {
			t.Error("opening gave a playback as well as an error")
		}
	})
}

// Test_playbackCallbacks tests the playbackCallbacks function with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - HandsOutputThrough: the device's own buffer is what the caller fills
//   - IgnoresInput: the capture buffer a playback device never receives is
//     dropped
func Test_playbackCallbacks(t *testing.T) {
	// Verify that the buffer the device wants filled is the one the caller is
	// given, since anything else would mean copying every frame for no reason.
	t.Run("HandsOutputThrough", func(t *testing.T) {
		out := make([]byte, 8)
		var got []byte
		calls := 0

		cb := playbackCallbacks(func(buf []byte) { calls++; got = buf })
		if cb.Data == nil {
			t.Fatal("playbackCallbacks gave no Data callback, so nothing would be played")
		}
		cb.Data(out, nil, 4)

		if calls != 1 {
			t.Errorf("the caller's function ran %d times, want once per buffer", calls)
		}
		if len(got) != len(out) {
			t.Errorf("the caller was given %d bytes, want the device's whole %d", len(got), len(out))
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
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - AsksForThePackagesFormat: rate, format and channels are the package's own
//   - CarriesThePeriod: the period the caller worked out is what is asked for
func Test_playbackConfig(t *testing.T) {
	// Verify the format asked of the library is the one this package produces,
	// so nothing between here and the speakers has to convert it.
	t.Run("AsksForThePackagesFormat", func(t *testing.T) {
		cfg := playbackConfig(FrameMS)

		if cfg.SampleRate != SampleRate {
			t.Errorf("asked for %d Hz, want %d", cfg.SampleRate, SampleRate)
		}
		if cfg.Playback.Format != malgo.FormatS16 {
			t.Errorf("asked for format %v, want signed 16-bit", cfg.Playback.Format)
		}
		if cfg.Playback.Channels != Channels {
			t.Errorf("asked for %d channels, want %d", cfg.Playback.Channels, Channels)
		}
		// No identifier at all, which is the standing instruction to follow
		// whichever output the system is using.
		if cfg.Playback.DeviceID != nil {
			t.Error("a device was named, want the system's own choice followed")
		}
	})

	// Verify the period reaches the library, since it is the whole of what
	// --buffer controls on this side of it.
	t.Run("CarriesThePeriod", func(t *testing.T) {
		cfg := playbackConfig(40)

		if cfg.PeriodSizeInMilliseconds != 40 {
			t.Errorf("asked for a period of %d ms, want 40", cfg.PeriodSizeInMilliseconds)
		}
	})
}
