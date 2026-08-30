// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/30/2026

//go:build cgo && !darwin && !windows

package audioout

import (
	"fmt"

	"github.com/gen2brain/malgo"
)

// Playing on Linux, which means playing through miniaudio.
//
// oto plays everywhere else, because a fault in what reached CoreAudio is what
// sent this package looking for another library at all, and on Windows it costs
// nothing to use the same one. Linux is where it would cost something. oto
// links ALSA at build time, so a binary built against it will not start on a
// machine without that library: not the audio commands, the whole program,
// serial port and all. A scanner on a headless Raspberry Pi is exactly the
// machine likely to be missing it and exactly the machine this has to work on.
// miniaudio opens the system's sound library at run time and carries on without
// it, so the download-and-run promise decides this one.
//
// Which leaves Linux playing through a library nobody has heard on Linux. That
// is worth saying plainly rather than dressing up: the fault that started all
// of this was heard on a Mac, and neither library has been listened to on
// anything else. What is known is that miniaudio's Core Audio backend is the
// one its own author calls the worst he has worked with, and that its ALSA
// backend is the far more travelled path. If somebody reports the same choppy
// playback here, that is a real report and this decision is worth revisiting
// with the dependency written down as its price.
//
// malgo is already linked in on every platform for capture, so this costs
// nothing that was not already being paid.
var (
	// openFn is how Open starts playing. It is a var so tests can substitute a
	// fake.
	openFn = openMalgo

	// initDevice and startDevice are the two library calls whose failures
	// nothing else can produce.
	//
	// The null backend the tests run against accepts every format asked of it,
	// so asking it for a device always works. These are vars so a test can make
	// each of them fail and check the error is reported rather than swallowed.
	initDevice  = malgo.InitDevice
	startDevice = func(dev *malgo.Device) error { return dev.Start() }
)

// playback is one open sound output, playing until it is closed.
//
// Both handles are kept because both have to be given back, and in this order:
// the device is a thing the context allocated, so freeing the context first
// would be freeing the ground the device is standing on.
type playback struct {
	ctx *malgo.AllocatedContext // The library context the device was allocated from, freed last
	dev *malgo.Device           // The running playback device, stopped and freed before the context
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

// openMalgo starts playing on this computer's output.
//
// The device is opened by handing the library no identifier at all rather than
// the identifier of whichever device is currently marked default. The two
// differ when somebody changes their output while this is running: no
// identifier is a standing instruction to follow the system's choice, and on
// the platforms that honour it that is the behaviour plugging in headphones
// expects. It is also what keeps this command shaped the same on every
// platform, since the Mac's library cannot be pointed at a named output at all.
//
// Parameters:
//   - periodMS: how much audio the device takes per callback, in milliseconds,
//     as periodFor worked it out
//   - fill: called from the library's audio thread to fill each buffer
//
// Returns:
//   - output holding the running device and the context it was allocated from
//   - error if the audio system cannot be opened, or the device cannot be
//     initialized or started
func openMalgo(periodMS int, fill func(out []byte)) (output, error) {
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

	dev, err := initDevice(ctx.Context, playbackConfig(periodMS), playbackCallbacks(fill))
	if err != nil {
		return nil, fmt.Errorf("opening the speakers: %w", err)
	}

	if err := startDevice(dev); err != nil {
		dev.Uninit()
		return nil, fmt.Errorf("starting the speakers: %w", err)
	}

	handedOver = true
	return &playback{ctx: ctx, dev: dev}, nil
}

// playbackCallbacks wraps fill in the shape the audio library calls back in.
//
// The device is opened with the one channel the audio already has, so the
// buffer the library wants filled is handed to fill exactly as it arrived and
// the library widens it for a stereo output afterwards.
//
// Neither buffer is fresh: the library's holds whatever it last put there, and
// nothing here clears it, so anything filling less than all of it leaves the
// rest playing again. The rule that read fills every byte is what that comes
// from.
//
// Parameters:
//   - fill: called with each buffer the device wants filled
//
// Returns:
//   - malgo.DeviceCallbacks whose Data hands the output buffer to fill and
//     ignores the input buffer a playback device never receives
func playbackCallbacks(fill func(out []byte)) malgo.DeviceCallbacks {
	return malgo.DeviceCallbacks{
		Data: func(out, _ []byte, _ uint32) { fill(out) },
	}
}

// playbackConfig describes the playback this package asks for: the package's
// own rate, channel count and sample format, on whichever device the system is
// using, taken in pieces of the period the caller worked out.
//
// The period is asked for three times over, which is the library's own idea of
// how many buffers a device should stand behind, left alone. The period times
// the periods is the slack the operating system has to come and ask for more.
//
// Parameters:
//   - periodMS: how much audio the device takes per callback, in milliseconds,
//     as periodFor worked it out
//
// Returns:
//   - malgo.DeviceConfig built on the library's playback defaults with this
//     package's format asked for on top of them
func playbackConfig(periodMS int) malgo.DeviceConfig {
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInMilliseconds = uint32(periodMS)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = Channels
	return cfg
}
