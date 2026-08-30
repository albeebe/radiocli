// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/27/2026

//go:build cgo && (darwin || windows)

package audioout

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// How this file reaches the library, and why there is a seam here at all.
//
// This plays on macOS and on Windows, where the library needs nothing installed
// alongside it. See malgoplay.go for Linux, where it would, and why that
// settled it.
//
// The same arrangement malgo.go has, for a different reason. There the seam
// exists so a test can make each library call fail; here it exists because
// there is nothing to point a test at. malgo ships a null backend, a software
// device that needs no sound card, so the tests in this package open a real
// device and play into nothing. oto has no equivalent: it opens the machine's
// actual output or it fails. So everything above the one call that touches
// hardware is written to be reachable without it, and that call is the only
// thing in this file a test cannot enter.
var (
	// openFn is how Open starts playing. It is a var so tests can substitute a
	// fake.
	openFn = openOto

	// startOto opens the device and returns something to play through. It is a
	// var so a test can stand in for the hardware.
	startOto = startOtoDevice
)

// otoPeriods is how many periods of audio the device is asked to stand behind.
//
// One, which is as little as oto can be asked for, and the reason is that this
// package already has a jitter buffer and does not want a second one. oto takes
// a plain duration rather than a period and a count, and whatever it holds is
// delay nobody can shorten from out here: it stands in front of the ring rather
// than instead of it. Three of them, which is what malgo defaults to, put a
// third of a second between the radio and the speakers before the ring had
// added anything.
//
// So the device is asked for the smallest cushion that keeps it fed and the
// ring is left to do the absorbing, which is the job it was built for and the
// one --buffer already tunes. If this turns out to be too mean for some sound
// card, the symptom is glitching that gets worse as --buffer goes down, and the
// fix is to raise this rather than to make everybody wait.
const otoPeriods = 1

// The library's context is made once and never given back.
//
// Not a choice. oto says outright that "creating multiple contexts is NOT
// supported", and it has no Close: the context lives as long as the process
// does. One command opens one output and then exits, so this costs nothing
// here, and the Once is what keeps a second Open in the same process from
// being an error the first one did not have.
var (
	otoContextOnce sync.Once    // Guarantees the library's context is built once
	otoContext     *oto.Context // The context every player is made from, nil if it failed
	otoContextErr  error        // Why it failed, kept so every later caller is told the same thing
)

// otoPlayer is the part of oto's player this package uses.
//
// An interface rather than the concrete type, so that everything except the
// call that reaches the sound card can be tested. See the var block above.
type otoPlayer interface {
	// Close stops the player and gives back what the library allocated.
	Close() error

	// Play starts the player pulling audio from the reader it was made with.
	Play()
}

// otoPlayback is one open sound output built on oto, playing until it is
// closed.
//
// Thinner than the malgo one, because oto keeps less on this side: there is no
// context to free per player and no device handle to stop, only the player
// itself.
type otoPlayback struct {
	player otoPlayer // The running player, closed on the way out
}

// Close stops the player and gives back what the library allocated.
//
// It returns nothing to report for the reason the malgo one does not: there is
// nothing a caller could do about a failure to tear down an output it has
// already finished with. The context is deliberately not touched. oto has no
// way to give one back, and it is shared by anything else this process opens.
func (p *otoPlayback) Close() {
	if p.player != nil {
		_ = p.player.Close()
		p.player = nil
	}
}

// fillReader turns a fill function into the io.Reader oto pulls audio from.
//
// This is the whole architectural difference between the two backends, in four
// lines. miniaudio pushes: it calls us with a buffer and we fill it. oto pulls:
// it reads from us whenever its own thread decides to. The ring underneath
// does not care which way round it is, because filling a buffer and answering
// a read are the same act.
type fillReader struct {
	fill func(out []byte) // What the ring fills a buffer with
}

// Read fills p with whatever the ring has and never ends.
//
// Always the whole buffer and always a nil error, which is what keeps the
// player running forever. A short read would have oto treat the stream as
// finished and stop, and the ring is never short: it plays silence when it has
// nothing, which is the same thing the malgo callback gets.
//
// Parameters:
//   - p: the buffer to fill, of whatever length the library asked for
//
// Returns:
//   - the length of p, always
//   - nil, always
func (r fillReader) Read(p []byte) (int, error) {
	r.fill(p)
	return len(p), nil
}

// openOto starts playing on this computer's own output.
//
// There is no choosing which. oto opens whichever output the system is using
// and offers no way to name another or to ask which it took, which is why the
// commands stopped offering one.
//
// Parameters:
//   - periodMS: how much audio the device should take at a time, in
//     milliseconds, as periodFor worked it out
//   - fill: called from the library's own thread to fill each buffer
//
// Returns:
//   - output holding the running player
//   - error if the audio system cannot be opened
func openOto(periodMS int, fill func(out []byte)) (output, error) {
	p, err := startOto(time.Duration(periodMS*otoPeriods)*time.Millisecond, fillReader{fill: fill})
	if err != nil {
		return nil, fmt.Errorf("opening the speakers: %w", err)
	}

	p.Play()
	return &otoPlayback{player: p}, nil
}

// startOtoDevice builds the library's context if it has not been built and
// makes a player that reads from r.
//
// This is the one function in the file that reaches the sound card, and the one
// a test cannot enter, which is why everything else was kept out of it.
//
// Parameters:
//   - buffer: how much audio the device should stand behind, which oto takes as
//     a duration rather than as a period and a count
//   - r: where the player pulls its audio from
//
// Returns:
//   - otoPlayer already made but not yet started
//   - error if the audio system cannot be opened, the same one for every caller
//     once the first has failed
func startOtoDevice(buffer time.Duration, r io.Reader) (otoPlayer, error) {
	otoContextOnce.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   SampleRate,
			ChannelCount: Channels,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   buffer,
		})
		if err != nil {
			otoContextErr = err
			return
		}

		// The context is not usable until this closes, and on some platforms
		// that is not immediate. Waiting here rather than at the first Read
		// keeps the wait out of the path the audio runs on.
		<-ready
		otoContext = ctx
	})

	if otoContextErr != nil {
		return nil, otoContextErr
	}
	return otoContext.NewPlayer(r), nil
}
