// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

// Package audioout lists the computer's speakers and plays audio on one.
//
// It is the other direction of audioin, and it is a separate package for the
// same reason that one exists: to keep malgo, which is cgo bindings to
// miniaudio, out of the rest of the tool. Everything that names a malgo type
// lives in malgo.go, and nocgo.go stands in for it in a build made without cgo,
// so a cross compiled binary still builds and simply says it cannot play.
//
// # It buffers, and audioin does not
//
// This is the one place the two directions are not mirror images. audioin hands
// its callback straight through to whoever asked for it and refuses to buffer
// anything, on the grounds that only the caller knows how its audio should be
// framed. Here the buffering is the job.
//
// The difference is which end sets the pace. A capture pushes: audio exists,
// and the callback delivers it whether anything is ready or not. A playback
// pulls: the sound card asks for audio at a moment of its choosing and will
// play whatever is in that buffer when it returns, including whatever was left
// there from last time if nothing new was written. Nothing in this tool can
// answer that question on demand. The audio it wants to play is arriving in
// 20 ms frames from a socket, a capture callback or a gate that releases a
// whole transmission at once, and none of those are on speaking terms with a
// sound card's schedule.
//
// So the ring is not a convenience this package provides on top of the library.
// It is the thing that makes a pull-shaped device usable by a push-shaped
// program, and every caller would otherwise write it, identically and probably
// wrong.
//
// # A sink is its name, except the default one
//
// Naming works exactly as it does in audioin: a sink is a name and nothing
// else, the library's own identifiers never leave malgo.go, and a name that
// matches two attached devices is refused rather than guessed at.
//
// The default device is where the two part company. audioin never falls back to
// it, because on a laptop the default input is the built-in microphone and
// recording the room instead of the scanner is a failure that sounds like it
// worked. There is no matching trap on the way out: the default output is
// wherever the person is already listening to everything else, which is exactly
// where they would expect a scanner to come out. So an empty name here means
// the default, and it is the ordinary case rather than a fallback.
package audioout

import (
	"fmt"
	"slices"
	"strings"
)

// Open starts playing on the sink called name.
//
// It opens the device and starts it immediately, playing silence until
// something is handed to Play, because a device that is started only once audio
// arrives would spend the first transmission starting.
//
// This is the call that reaches the hardware, and on some systems it is what
// makes the volume indicator appear. It costs nothing but a device handle while
// nothing is playing.
//
// Parameters:
//   - name: the sink to play on, matched the way Resolve matches, or empty for
//     whichever output this computer is already using
//
// Returns:
//   - *Player that plays whatever is handed to it until Close is called
//   - error if the audio system cannot be asked, the name matches no sink or
//     more than one, the device cannot be opened or started, or this build was
//     made without audio support
//
// Errors:
//   - ErrNoSink: if nothing attached is called name
//   - ErrAmbiguousSink: if more than one attached sink is called name
func Open(name string) (*Player, error) {
	r := newRing()

	out, err := openFn(name, r.read)
	if err != nil {
		return nil, err
	}
	return &Player{out: out, ring: r}, nil
}

// Resolve checks that exactly one attached sink is called name, and returns it
// spelled the way the operating system spells it.
//
// It opens nothing, so a name can be checked the moment it is typed rather than
// when there is finally something to play. The canonical spelling comes back
// rather than the one that was typed, so what gets shown afterwards matches
// what a listing shows.
//
// Parameters:
//   - name: what somebody typed, matched case-insensitively with surrounding
//     space ignored
//
// Returns:
//   - string holding the matched sink's name as the operating system spells it
//   - error if the audio system cannot be asked, or the name matches no sink or
//     more than one
//
// Errors:
//   - ErrNoSink: if nothing attached is called name
//   - ErrAmbiguousSink: if more than one attached sink is called name
func Resolve(name string) (string, error) {
	sinks, err := Sinks()
	if err != nil {
		return "", err
	}

	names := make([]string, len(sinks))
	for i, s := range sinks {
		names[i] = s.Name
	}

	at, err := pickSink(names, name)
	if err != nil {
		return "", err
	}
	return names[at], nil
}

// Sinks returns every place this computer can play sound, sorted by name.
//
// Finding none is an answer rather than a failure, the same way audioin finding
// no inputs is. An error means the audio system itself could not be asked.
//
// The order is this package's rather than the operating system's, for the
// reason audioin sorts: the system's order changes as devices come and go, and
// a listing that reshuffles between two runs is hard to read and hard to
// compare.
//
// Returns:
//   - []Sink holding every attached output, sorted by name ignoring case
//   - error if the audio system itself could not be asked, or this build was
//     made without audio support
func Sinks() ([]Sink, error) {
	sinks, err := listSinksFn()
	if err != nil {
		return nil, err
	}
	sortSinks(sinks)
	return sinks, nil
}

// pickSink finds the one sink called name among names, and returns where it
// sits in that list.
//
// The index rather than the name, because the caller is about to want the audio
// library's own identifier for it, and that is only reachable by position. This
// is audioin.pickSource with the errors changed, and it is copied rather than
// shared because sharing it would mean one of these two packages importing the
// other for twenty lines, and the direction of that import would say something
// about the two that is not true.
//
// Parameters:
//   - names: every attached sink's name, in the audio library's own order
//   - name: the name to find, matched case-insensitively with surrounding space
//     ignored
//
// Returns:
//   - int index into names of the one matching sink
//   - error if the name is blank, matches nothing, or matches more than one
//
// Errors:
//   - ErrNoSink: if name is blank or nothing in names matches it
//   - ErrAmbiguousSink: if more than one entry in names matches it
func pickSink(names []string, name string) (int, error) {
	want := strings.TrimSpace(name)
	if want == "" {
		return 0, fmt.Errorf("%w: no name was given", ErrNoSink)
	}

	var exact, folded []int
	for i, have := range names {
		switch {
		case have == want:
			exact = append(exact, i)
		case strings.EqualFold(have, want):
			folded = append(folded, i)
		}
	}

	found := exact
	if len(found) == 0 {
		found = folded
	}

	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return 0, fmt.Errorf("%w: %q", ErrNoSink, want)
	default:
		return 0, fmt.Errorf("%w: %d of them are called %q, and nothing here can tell them apart",
			ErrAmbiguousSink, len(found), want)
	}
}

// sortSinks puts a listing in this package's order: by name, ignoring case.
//
// Stable, so that two devices sharing a name keep the order the system gave
// them.
//
// Parameters:
//   - sinks: the listing to sort, reordered in place
func sortSinks(sinks []Sink) {
	slices.SortStableFunc(sinks, func(a, b Sink) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}
