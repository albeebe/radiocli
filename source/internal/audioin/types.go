// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audioin

import "errors"

// The format every capture is opened in.
//
// Asked of the device rather than discovered from it. The audio library converts
// whatever the hardware natively does into this, so a caller can rely on these
// numbers instead of carrying a format around, and the one interface these were
// measured against needs no conversion at all: line inputs are natively 44.1 or
// 48 kHz, and none of the ones worth using offer a mono mode.
//
// Stereo even though the scanner is mono, because which side of the cable the
// mono signal landed on is not knowable from here. A record lead wired for one
// channel and a headphone lead wired for both are both ordinary, and asking the
// library to fold them together would quietly halve the level of the first.
// Both channels come out of here and something further along decides.
const (
	// Channels is 2. See above: the fold to mono happens elsewhere.
	Channels = 2

	// FrameBytes is one frame as it arrives here: FrameSamples of interleaved
	// signed 16-bit little-endian stereo.
	FrameBytes = FrameSamples * Channels * 2

	// FrameMS is the length of the frame everything downstream is cut into,
	// fixed by the Opus encoder having exactly one frame size.
	FrameMS = 20

	// FrameSamples is one frame in sample pairs.
	FrameSamples = SampleRate * FrameMS / 1000

	// SampleRate is 48 kHz because that is the only rate the Opus encoder
	// accepts, and a line input is natively at or above it, so nothing is
	// resampled on the way in or on the way out.
	SampleRate = 48000
)

// Why a name can fail to name one source.
//
// These are separate errors because the advice differs. Nothing found is very
// often a typo or a cable pulled out, and the fix is to look at the list. More
// than one found is two identical interfaces, and there is nothing in the list
// to look at, because both lines of it say the same thing.
var (
	// ErrAmbiguousSource says more than one thing attached is called that.
	ErrAmbiguousSource = errors.New("more than one sound input by that name")

	// ErrNoSource says nothing attached is called that.
	ErrNoSource = errors.New("no sound input by that name")
)

// Source is one audio input this computer can record from: a line in, a
// microphone, or a virtual device that some other application presents.
//
// Nothing here comes from the scanner. Its audio arrives as an ordinary analog
// signal on a cable, so as far as this package is concerned the scanner is
// whichever of these somebody plugged it into, and there is no way to tell
// which one that is by looking.
//
// It is a struct around one field rather than a bare string because a listing
// is about to need more than that: choosing a source will add a mark for the
// chosen one, and something reading this as JSON should not have to be
// rewritten when it does.
type Source struct {
	// Name is what the operating system calls the source, such as
	// "Cubilux CB5 Line In". It is what a listing shows, what somebody types to
	// choose one, and what gets saved once they have.
	//
	// It is not unique. Two identical USB interfaces report the same name.
	Name string `json:"name"`
}
