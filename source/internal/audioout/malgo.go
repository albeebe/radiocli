// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

//go:build cgo

package audioout

import (
	"fmt"

	"github.com/gen2brain/malgo"
)

// This file lists what this computer can play on, and nothing else.
//
// It used to do the playing as well. The audio now goes out through oto, which
// cannot enumerate anything, and malgo cannot be pointed at what oto chose, so
// what is left here is a listing that nothing selects from: it says what the
// machine has, for somebody wondering where the sound will come out. See
// oto.go.
var (
	// backends is the backend list handed to the library. Nil lets it choose at
	// runtime, which is what every real run wants: on Linux it means one binary
	// works whether the machine is running ALSA, PulseAudio or JACK. It is a
	// var so a test can substitute the null backend, which is a software device
	// needing no sound card.
	backends []malgo.Backend

	// listSinksFn is how Sinks asks what this computer can play on. It is a var
	// so tests can substitute a fake.
	listSinksFn = listSinks

	// listDevices is the one library call whose failure nothing else can
	// produce.
	//
	// The null backend the tests run against enumerates from a table built into
	// the library, so asking it for devices always works. It is a var so a test
	// can make it fail and check the error is reported rather than swallowed.
	listDevices = func(ctx *malgo.AllocatedContext, kind malgo.DeviceType) ([]malgo.DeviceInfo, error) {
		return ctx.Devices(kind)
	}
)

// deviceNames reads the name out of each device the library reported, keeping
// the library's order.
//
// Parameters:
//   - devices: what the library reported, in the order it reported it
//
// Returns:
//   - []string of the same length, holding each device's name
func deviceNames(devices []malgo.DeviceInfo) []string {
	names := make([]string, len(devices))
	for i := range devices {
		names[i] = devices[i].Name()
	}
	return names
}

// listSinks asks the audio library what this computer can play on, in whatever
// order it answers.
//
// This is the only function in the tool that names a malgo type, and keeping it
// that way is the point of the package.
//
// Returns:
//   - []Sink naming every playback device the library reports, in its order
//   - error if the audio system cannot be opened or its devices cannot be
//     listed
func listSinks() ([]Sink, error) {
	ctx, err := malgo.InitContext(backends, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("opening the audio system: %w", err)
	}

	// Both of these are needed and in this order. Uninit releases what the
	// library allocated inside the context, and Free releases the context
	// itself, so doing either alone leaks the other.
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	devices, err := listDevices(ctx, malgo.Playback)
	if err != nil {
		return nil, fmt.Errorf("listing the speakers: %w", err)
	}

	return sinksFromNames(deviceNames(devices)), nil
}

// sinksFromNames turns the names the library reported into this package's own
// values, keeping the order they arrived in.
//
// Parameters:
//   - names: what the library calls each playback device, in its order
//
// Returns:
//   - []Sink of the same length and order, never nil
func sinksFromNames(names []string) []Sink {
	// Made rather than left nil so that a machine with no outputs at all
	// renders as an empty JSON array instead of as null.
	sinks := make([]Sink, 0, len(names))
	for _, n := range names {
		sinks = append(sinks, Sink{Name: n})
	}
	return sinks
}
