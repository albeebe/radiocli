// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

//go:build cgo

package audioout

import (
	"fmt"
	"sync/atomic"
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
	// runtime, which is what every real run wants: on Linux it means one binary
	// works whether the machine is running ALSA, PulseAudio or JACK. It is a
	// var so a test can substitute the null backend, which is a software device
	// needing no sound card.
	backends []malgo.Backend

	// listSinksFn is how Sinks asks what this computer can play on. It is a var
	// so tests can substitute a fake.
	listSinksFn = listSinks

	// openFn is how Open starts playing. It is a var so tests can substitute a
	// fake.
	openFn = open

	// listDevices, initDevice and startDevice are the three library calls whose
	// failures nothing else can produce.
	//
	// The null backend the tests run against enumerates from a table built into
	// the library and accepts every format asked of it, so asking it for
	// devices or for a device always works. These are vars so a test can make
	// each of them fail and check the error is reported rather than swallowed.
	//
	// They are called once per listSinks or open, which is once per command,
	// never per frame of audio. The callback the library asks for audio through
	// is built in playbackCallbacks and is not routed through anything here.
	listDevices = func(ctx *malgo.AllocatedContext, kind malgo.DeviceType) ([]malgo.DeviceInfo, error) {
		return ctx.Devices(kind)
	}
	initDevice  = malgo.InitDevice
	startDevice = func(dev *malgo.Device) error { return dev.Start() }
)

// playback is one open sound output, playing until it is closed.
//
// Both handles are kept because both have to be given back, and in this order:
// the device is a thing the context allocated, so freeing the context first
// would be freeing the ground the device is standing on.
type playback struct {
	ctx  *malgo.AllocatedContext // The library context the device was allocated from, freed last
	dev  *malgo.Device           // The running playback device, stopped and freed before the context
	name string                  // The output being played on, spelled the way the system spells it

	// period is how many frames the last callback asked for. Written by the
	// audio thread and read by whoever is reporting, so it is atomic: a plain
	// int here would be a data race on every callback, and a mutex would be a
	// lock taken on the one path that must never take one.
	period *atomic.Uint32
}

// Close stops the device and gives back everything the library allocated.
//
// It returns nothing to report because there is nothing a caller could do about
// a failure to tear down a device it has already finished with, and every one
// of these steps has to be attempted whatever the one before it did.
//
// Stop before Uninit is not belt and braces. Uninit on a running device stops
// it anyway, but stopping first is what guarantees the callback has returned
// for the last time before the ring it reads out of goes away.
func (p *playback) Close() {
	if p.dev != nil {
		_ = p.dev.Stop()
		p.dev.Uninit()
		p.dev = nil
	}
	if p.ctx != nil {
		_ = p.ctx.Uninit()
		p.ctx.Free()
		p.ctx = nil
	}
}

// Name is what the operating system calls the output this is playing on,
// spelled the way the system spells it rather than the way it was typed.
func (p *playback) Name() string { return p.name }

// Period is how many frames the device last asked for at once, or zero before
// it has asked at all.
//
// The reading is taken in the callback rather than worked out from what was
// asked for at Open, because the two are not the same thing. See Stats.Period.
func (p *playback) Period() int { return int(p.period.Load()) }

// defaultName finds what the library calls this computer's usual output.
//
// Only for saying which speakers the audio went to. The device is opened by
// handing the library no identifier at all rather than by handing it this one,
// so a wrong answer here costs a line of text and not the audio.
//
// Parameters:
//   - devices: what the library reported, in the order it reported it
//
// Returns:
//   - the name of the first device the library marked as the default, or empty
//     if it marked none
func defaultName(devices []malgo.DeviceInfo) string {
	for i := range devices {
		if devices[i].IsDefault != 0 {
			return devices[i].Name()
		}
	}
	return ""
}

// deviceNames reads the name out of each device the library reported, keeping
// the library's order.
//
// Only the name is carried out of here. The library's own identifier stays in
// this file, to be looked up again from the name whenever a sink has to be
// opened, so it never has to be stored, shown or typed.
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

// listSinks asks the audio library what this computer can play on, in whatever
// order it answers.
//
// This and open are the only functions in the tool that name a malgo type on
// the way out, and keeping it that way is the point of the package.
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

// open resolves name to a device and starts playing on it.
//
// The identifier is looked up here and now rather than stored anywhere, which
// is the arrangement the package doc describes: enumerate, match by name, and
// use the identifier at the matching position.
//
// An empty name is the default device, and it is handed to the library as no
// identifier rather than as the identifier of whichever device is currently
// marked default. The two differ when the person changes their output while
// this is running: no identifier is a standing instruction to follow the
// system's choice, and on the platforms that honour that it is the behaviour
// somebody plugging in headphones expects.
//
// Parameters:
//   - name: the sink to play on, resolved against the library's device list at
//     this moment, or empty for the system's own choice
//   - fill: called from the library's audio thread to fill each buffer
//
// Returns:
//   - output holding the running device and the context it was allocated from
//   - error if the audio system cannot be opened, the devices cannot be listed,
//     the name matches no sink or more than one, or the device cannot be
//     initialized or started
//
// Errors:
//   - ErrNoSink: if nothing the library reports is called name
//   - ErrAmbiguousSink: if more than one reported device is called name
func open(name string, periodMS int, fill func(out []byte)) (output, error) {
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

	devices, err := listDevices(ctx, malgo.Playback)
	if err != nil {
		return nil, fmt.Errorf("listing the speakers: %w", err)
	}

	// In the library's order rather than sorted, because the position in this
	// list is what the identifier is reached by.
	names := deviceNames(devices)

	var deviceID unsafe.Pointer
	chosen := defaultName(devices)
	if name != "" {
		at, err := pickSink(names, name)
		if err != nil {
			return nil, err
		}
		deviceID, chosen = devices[at].ID.Pointer(), names[at]
	}

	period := &atomic.Uint32{}
	dev, err := initDevice(ctx.Context, playbackConfig(deviceID, periodMS),
		playbackCallbacks(fill, period))
	if err != nil {
		return nil, fmt.Errorf("opening the speakers %q: %w", chosen, err)
	}

	if err := startDevice(dev); err != nil {
		dev.Uninit()
		return nil, fmt.Errorf("starting the speakers %q: %w", chosen, err)
	}

	handedOver = true
	return &playback{ctx: ctx, dev: dev, name: chosen, period: period}, nil
}

// playbackCallbacks wraps fill in the shape the audio library calls back in.
//
// The device is opened with two channels and the audio has one, so fill is
// asked for half the bytes the device wants and each sample it writes is put on
// both sides. Handing fill the device's own buffer instead would play the audio
// at twice its speed and empty the ring twice as fast.
//
// Neither buffer is fresh: the library's holds whatever it last put there, and
// the one fill is handed is reused between callbacks, so anything filling less
// than all of it leaves the rest playing again. The rule that read fills every
// byte is what that comes from.
//
// Parameters:
//   - fill: called with each buffer the device wants filled
//   - period: where each callback records how many frames it was asked for, so
//     that what the device settled on can be read rather than assumed
//
// Returns:
//   - malgo.DeviceCallbacks whose Data hands the output buffer to fill and
//     ignores the input buffer a playback device never receives
func playbackCallbacks(fill func(out []byte), period *atomic.Uint32) malgo.DeviceCallbacks {
	// Grown on the first callback and reused after it, because a callback that
	// allocates is a callback that can be made to wait by the garbage
	// collector, and this one must never wait. A device asks for the same size
	// every time, so the growth happens once.
	var mono []byte

	return malgo.DeviceCallbacks{
		Data: func(out, _ []byte, frames uint32) {
			// A store and nothing else, which is the most this path can afford.
			// The library says outright that the frame count "will not
			// necessarily be equal to what you requested" and must not be
			// assumed constant, so it is recorded every time rather than once.
			period.Store(frames)

			need := len(out) / outputChannels
			if cap(mono) < need {
				mono = make([]byte, need)
			}
			m := mono[:need]
			fill(m)

			// One mono sample written to both sides. The alternative is to open
			// the device as mono and let the library widen it, which is what
			// this package did before: it sounds the same in principle, and
			// doing it here is one conversion fewer between the ring and the
			// speakers.
			for i, j := 0, 0; i+1 < need; i, j = i+2, j+4 {
				out[j], out[j+1] = m[i], m[i+1]
				out[j+2], out[j+3] = m[i], m[i+1]
			}
		},
	}
}

// playbackConfig describes the playback this package asks for: the package's
// own rate, channel count and sample format, on one device, taken in pieces
// of the period the caller worked out.
//
// The period is asked for three times over, which is the library's own idea of
// how many buffers a device should stand behind, left alone. The period times
// the periods is the slack the operating system has to come and ask for more:
// the artifacts a night of listening pinned below this package went away when
// that slack grew, and no measurement on our side of the library has ever
// caught them, so room is the one lever this package holds.
//
// Parameters:
//   - deviceID: the library's identifier for the device to play on, or nil for
//     whichever the system considers its own
//   - periodMS: how much audio the device takes per callback, in milliseconds,
//     as periodFor worked it out
//
// Returns:
//   - malgo.DeviceConfig built on the library's playback defaults with this
//     package's format asked for on top of them
func playbackConfig(deviceID unsafe.Pointer, periodMS int) malgo.DeviceConfig {
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInMilliseconds = uint32(periodMS)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = outputChannels
	cfg.Playback.DeviceID = deviceID
	return cfg
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
