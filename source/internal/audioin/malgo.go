// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

//go:build cgo

package audioin

import (
	"fmt"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// How this file's hardware calls are reached, both from the rest of the package
// and from inside this file.
//
// They live next to the implementation they hold rather than in types.go,
// because each build variant has its own set and a var can only stand where the
// function it names is compiled.
var (
	// backends is the backend list handed to the library. Nil lets it choose at
	// runtime, which is what every real run wants and what the comment in
	// listSources explains. It is a var so a test can substitute the null
	// backend, which is a software device needing no sound card.
	backends []malgo.Backend

	// listSourcesFn is how Sources asks what this computer can record from. It
	// is a var so tests can substitute a fake.
	listSourcesFn = listSources

	// openFn is how Open starts recording. It is a var so tests can substitute a
	// fake.
	openFn = open

	// listDevices, initDevice and startDevice are the three library calls whose
	// failures nothing else can produce.
	//
	// The null backend the tests run against enumerates from a table built into
	// the library and accepts every format asked of it, so asking it for devices
	// or for a device always works. These are vars so a test can make each of
	// them fail and check the error is reported rather than swallowed.
	//
	// They are called once per listSources or open, which is once per command,
	// never per frame of audio. The callback the library delivers audio through
	// is built in captureCallbacks and is not routed through anything here.
	listDevices = func(ctx *malgo.AllocatedContext, kind malgo.DeviceType) ([]malgo.DeviceInfo, error) {
		return ctx.Devices(kind)
	}
	initDevice  = malgo.InitDevice
	startDevice = func(dev *malgo.Device) error { return dev.Start() }
)

// Capture is one open sound input, delivering audio until it is closed.
//
// Both handles are kept because both have to be given back, and in this order:
// the device is a thing the context allocated, so freeing the context first
// would be freeing the ground the device is standing on.
type Capture struct {
	ctx  *malgo.AllocatedContext // The library context the device was allocated from, freed last
	dev  *malgo.Device           // The running capture device, stopped and freed before the context
	name string                  // The source being recorded from, spelled the way the system spells it
}

// Close stops recording and gives back everything the library allocated.
//
// It returns nothing to report because there is nothing a caller could do about
// a failure to tear down a device it has already finished with, and every one of
// these steps has to be attempted whatever the one before it did.
//
// Stop before Uninit is not belt and braces. Uninit on a running device stops it
// anyway, but stopping first is what guarantees the callback has returned for the
// last time before the memory it writes into goes away.
func (c *Capture) Close() {
	if c.dev != nil {
		_ = c.dev.Stop()
		c.dev.Uninit()
		c.dev = nil
	}
	if c.ctx != nil {
		_ = c.ctx.Uninit()
		c.ctx.Free()
		c.ctx = nil
	}
}

// Name is what the operating system calls the source this is recording from,
// spelled the way the system spells it rather than the way it was typed.
func (c *Capture) Name() string { return c.name }

// captureCallbacks wraps onFrames in the shape the audio library calls back in.
//
// The callback is handed straight through. Everything the package doc says
// about what may be done inside it is a rule for whoever passed it in, and
// wrapping it here to enforce any of that would be this package deciding how
// somebody else's audio is buffered.
//
// Parameters:
//   - onFrames: called with each batch of captured audio
//
// Returns:
//   - malgo.DeviceCallbacks whose Data hands the input buffer to onFrames and
//     ignores the output buffer a capture device never writes to
func captureCallbacks(onFrames func(pcm []byte)) malgo.DeviceCallbacks {
	return malgo.DeviceCallbacks{
		Data: func(_, in []byte, _ uint32) {
			onFrames(in)
		},
	}
}

// captureConfig describes the capture this package always asks for: the
// package's own rate, channel count and sample format, on one named device.
//
// Parameters:
//   - deviceID: the library's identifier for the device to record from
//
// Returns:
//   - malgo.DeviceConfig built on the library's capture defaults with this
//     package's format asked for on top of them
func captureConfig(deviceID unsafe.Pointer) malgo.DeviceConfig {
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.SampleRate = SampleRate
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = Channels
	cfg.Capture.DeviceID = deviceID
	return cfg
}

// deviceNames reads the name out of each device the library reported, keeping
// the library's order.
//
// Only the name is carried out of here. The library's own identifier
// stays in this file, to be looked up again from the name whenever a
// source has to be opened, and its notion of a default input is dropped
// because on a laptop that is the built-in microphone, which is never
// what a scanner is plugged into.
//
// Parameters:
//   - devices: what the library reported, in the order it reported it
//
// Returns:
//   - []string of the same length, holding each device's name at the position
//     its identifier is reached by
func deviceNames(devices []malgo.DeviceInfo) []string {
	names := make([]string, len(devices))
	for i := range devices {
		names[i] = devices[i].Name()
	}
	return names
}

// listSources asks the audio library what this computer can record from, in
// whatever order it answers.
//
// This is the only function in the tool that names a malgo type, and keeping it
// that way is the point of the package. Everything it returns is this package's
// own, so replacing the library means rewriting this file and nothing else.
//
// Returns:
//   - []Source naming every capture device the library reports, in its order
//   - error if the audio system cannot be opened or its devices cannot be
//     listed
func listSources() ([]Source, error) {
	// The nil backend list is deliberate. It leaves the library to pick at
	// runtime, which on Linux means one binary works whether the machine is
	// running ALSA, PulseAudio or JACK, rather than having to be built for
	// whichever the user happens to have.
	ctx, err := malgo.InitContext(backends, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("opening the audio system: %w", err)
	}

	// Both of these are needed and in this order. Uninit releases what the
	// library allocated inside the context, and Free releases the context
	// itself, so doing either alone leaks the other.
	//
	// The context is built and torn down for each call rather than kept open
	// for the life of the process. This is called when somebody asks what is
	// attached, not in a loop, and holding a C resource open for the whole run
	// to save a millisecond is a bad trade in a package that exists to keep
	// that library at arm's length.
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	devices, err := listDevices(ctx, malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("listing the audio inputs: %w", err)
	}

	return sourcesFromNames(deviceNames(devices)), nil
}

// open resolves name to a device and starts recording from it.
//
// The identifier is looked up here and now rather than stored anywhere, which is
// the arrangement the package doc describes: enumerate, match by name, and use
// the identifier at the matching position. It is never kept, so it never has to
// survive a restart or a device being moved to a different socket.
//
// The default device is deliberately never used, not even as a fallback when the
// name matches nothing. On a laptop the default is the built-in microphone, and
// recording the room instead of the scanner is a failure that sounds like it
// worked.
//
// Parameters:
//   - name: the source to record from, resolved against the library's device
//     list at this moment
//   - onFrames: called from the library's audio thread with each batch of
//     captured audio
//
// Returns:
//   - *Capture holding the running device and the context it was allocated from
//   - error if the audio system cannot be opened, the devices cannot be listed,
//     the name matches no source or more than one, or the device cannot be
//     initialized or started
//
// Errors:
//   - ErrNoSource: if nothing the library reports is called name
//   - ErrAmbiguousSource: if more than one reported device is called name
func open(name string, onFrames func(pcm []byte)) (*Capture, error) {
	ctx, err := malgo.InitContext(backends, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("opening the audio system: %w", err)
	}

	// Every path from here to the end has to give the context back, and there
	// are several. The flag is cleared only once the device is running and the
	// caller has something it can Close.
	handedOver := false
	defer func() {
		if !handedOver {
			_ = ctx.Uninit()
			ctx.Free()
		}
	}()

	devices, err := listDevices(ctx, malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("listing the audio inputs: %w", err)
	}

	// In the library's order rather than sorted, because the position in this
	// list is what the identifier is reached by.
	names := deviceNames(devices)

	at, err := pickSource(names, name)
	if err != nil {
		return nil, err
	}

	cfg := captureConfig(devices[at].ID.Pointer())

	dev, err := initDevice(ctx.Context, cfg, captureCallbacks(onFrames))
	if err != nil {
		return nil, fmt.Errorf("opening the sound input %q: %w", names[at], err)
	}

	if err := startDevice(dev); err != nil {
		dev.Uninit()
		return nil, fmt.Errorf("starting the sound input %q: %w", names[at], err)
	}

	handedOver = true
	return &Capture{ctx: ctx, dev: dev, name: names[at]}, nil
}

// sourcesFromNames turns the names the library reported into this package's own
// values, keeping the order they arrived in.
//
// Parameters:
//   - names: what the library calls each capture device, in its order
//
// Returns:
//   - []Source of the same length and order, never nil
func sourcesFromNames(names []string) []Source {
	// Made rather than left nil so that a machine with no inputs at all renders
	// as an empty JSON array instead of as null.
	sources := make([]Source, 0, len(names))
	for _, n := range names {
		sources = append(sources, Source{Name: n})
	}
	return sources
}
