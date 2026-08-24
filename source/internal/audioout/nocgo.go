// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

//go:build !cgo

package audioout

import "errors"

// How this file's two stand-in calls are reached from the rest of the package.
//
// They live next to the implementation they hold rather than in types.go,
// because each build variant has its own pair and a var can only stand where
// the function it names is compiled.
var (
	// listSinksFn is how Sinks asks what this computer can play on. It is a var
	// so tests can substitute a fake.
	listSinksFn = listSinks

	// openFn is how Open starts playing. It is a var so tests can substitute a
	// fake.
	openFn = open
)

// listSinks reports that this build cannot ask, because the library that knows
// how was left out of it.
//
// malgo is cgo bindings to miniaudio and has no pure Go implementation to fall
// back to, so a build with cgo switched off has nothing to call. Cross
// compiling defaults to CGO_ENABLED=0, and the rest of the tool is a serial
// port that works perfectly well in such a build, so refusing to compile the
// whole program over the commands that touch sound is the wrong trade. This
// turns a build failure into one command saying it cannot help.
//
// Returns:
//   - []Sink that is always nil in this build
//   - error, always, explaining that audio support was left out and how to
//     rebuild with it
func listSinks() ([]Sink, error) {
	return nil, errors.New("this copy of radiocli was built without audio support, " +
		"so it cannot list the speakers: rebuild it with CGO_ENABLED=1")
}

// open reports that this build cannot play, for the same reason listSinks
// reports that it cannot list.
//
// Parameters:
//   - name: ignored, since there is nothing to open
//   - fill: ignored, since nothing will ever ask for audio
//
// Returns:
//   - output that is always nil in this build
//   - error, always, explaining that audio support was left out and how to
//     rebuild with it
func open(name string, fill func(out []byte)) (output, error) {
	return nil, errors.New("this copy of radiocli was built without audio support, " +
		"so it cannot play audio: rebuild it with CGO_ENABLED=1")
}
