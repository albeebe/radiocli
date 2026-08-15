// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package audio

import (
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audioin"
)

// defaultBitrate is what --format opus uses unless told otherwise.
//
// In the middle of the useful band. The encoder here is CELT only, which does
// worse on speech than a hybrid one, so the comfortable range for a voice
// channel is around 32 to 48 kbps rather than the 16 to 24 that libopus would
// hold up at. See the opusenc package.
const defaultBitrate = 32000

// The formats this can write.
const (
	// formatOpus is compressed, for a program rather than a player: there is no
	// container around the packets, so nothing that plays files will read it.
	formatOpus = "opus"

	// formatPCM is the samples themselves, with nothing around them. It is the
	// default because it is the one a player can be pointed straight at.
	formatPCM = "pcm"
)

// listenQueue is how many frames may be waiting to be written before the oldest
// are dropped.
//
// One second. Generous compared with what a remote listener gets, because what is on the
// other end of this is usually a pipe into a player on the same machine, and a
// player that stalls for a moment should catch up rather than lose audio. It is
// still bounded, because the sound card cannot be asked to wait.
const listenQueue = 1000 / audiofeed.FrameMS

// listSources lists the sound inputs this computer can record from. It is a
// var so tests can substitute a fake and never enumerate a real sound card.
var listSources = audioin.Sources

// startCapture opens the sound card and begins publishing frames into out. It
// is a var so tests can substitute a fake and never open a real sound card,
// which on macOS is what raises the microphone permission prompt.
var startCapture = func(opts audiofeed.Options, out audiofeed.Publisher) (capture, error) {
	return audiofeed.Start(opts, out)
}

// capture is the part of an open sound card that listenDirect uses: it says
// what it is recording from, and it can be closed. It is an interface rather
// than *audiofeed.Capture so that startCapture can be faked.
type capture interface {
	// Close stops recording and waits for the last frame to be published.
	Close()

	// Source is what the operating system calls the input this is recording
	// from.
	Source() string
}

// listenOptions is what the flags asked for.
type listenOptions struct {
	input   string // Sound input to open directly, empty to take the audio from a daemon
	format  string // Audio format to write, as --format gave it
	bitrate int    // Bits per second, for --format opus
	channel string // Which side of the cable the scanner is on, as --channel gave it
}
