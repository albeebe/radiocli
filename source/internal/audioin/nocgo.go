// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

//go:build !cgo

package audioin

import "errors"

// How this file's two stand-in calls are reached from the rest of the package.
//
// They live next to the implementation they hold rather than in types.go,
// because each build variant has its own pair and a var can only stand where the
// function it names is compiled.
var (
	// listSourcesFn is how Sources asks what this computer can record from. It
	// is a var so tests can substitute a fake.
	listSourcesFn = listSources

	// openFn is how Open starts recording. It is a var so tests can substitute a
	// fake.
	openFn = open
)

// Capture stands in for the real one so that everything holding a *Capture
// compiles here too. There is nothing in it, because a build that cannot open a
// sound input never has one to hold.
type Capture struct{}

// Close has nothing to give back.
func (c *Capture) Close() {}

// Name answers with nothing, since there is no device and therefore no name.
func (c *Capture) Name() string { return "" }

// listSources reports that this build cannot ask, because the library that
// knows how was left out of it.
//
// malgo is cgo bindings to miniaudio and has no pure Go implementation to fall
// back to, so a build with cgo switched off has nothing to call. Without this
// file such a build does not fail at the sound card, it fails at the compiler,
// which is what happened: cross compiling from a Mac defaults to CGO_ENABLED=0,
// so "GOOS=linux go build" stopped working the moment this package arrived.
//
// The rest of the tool has nothing to do with sound. It drives a scanner over a
// serial port, and that works perfectly well in a build made this way, so
// refusing to compile the whole program over one command that lists sound cards
// is the wrong trade. This turns a build failure into one command saying it
// cannot help, and leaves every other command working.
//
// Returns:
//   - []Source that is always nil in this build
//   - error, always, explaining that audio support was left out and how to
//     rebuild with it
func listSources() ([]Source, error) {
	return nil, errors.New("this copy of radiocli was built without audio support, " +
		"so it cannot list the sound inputs: rebuild it with CGO_ENABLED=1")
}

// open reports that this build cannot record, for the same reason listSources
// reports that it cannot list.
//
// Worth noticing that only this half is missing. Encoding is pure Go and
// compiles fine here, so a build made this way can still take audio from
// somewhere else and send it on; what it cannot do is be the end that holds the
// sound card.
//
// Parameters:
//   - name: ignored, since there is nothing to open
//   - onFrames: ignored, since no audio will ever arrive
//
// Returns:
//   - *Capture that is always nil in this build
//   - error, always, explaining that audio support was left out and how to
//     rebuild with it
func open(name string, onFrames func(pcm []byte)) (*Capture, error) {
	return nil, errors.New("this copy of radiocli was built without audio support, " +
		"so it cannot record from a sound input: rebuild it with CGO_ENABLED=1")
}
