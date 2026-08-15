// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package audioin lists and reads the computer's audio inputs.
//
// It exists to keep one third-party library out of the rest of the tool. There
// is no credible pure Go way to take audio off a sound card on three platforms,
// so this is built on malgo, which is cgo bindings to miniaudio. Everything
// that knows that lives in malgo.go, and nothing outside this package names a
// malgo type, so deciding against the library later is a matter of rewriting
// one file rather than finding every place that touched it.
//
// There is deliberately no interface here to swap an implementation behind. One
// function that needs a real sound card has nothing worth faking, and
// containment is the stronger guarantee anyway: it can be checked, by grepping
// for the import and finding one file.
//
// # Builds without cgo
//
// Being cgo means the library cannot be compiled into a build that has cgo
// switched off, which is what cross compiling does by default. The rest of the
// tool is a serial port and works fine in such a build, so rather than let one
// command stop the program compiling, nocgo.go stands in for malgo.go there and
// answers that this build cannot list sound inputs. Everything else carries on.
//
// The name is audioin rather than audio because device/audio.go already means
// the scanner's own volume and squelch, which is a different thing carried over
// a different wire.
//
// # A source is its name
//
// The audio library has its own identifiers, and they are not used here. They
// are a fixed size blob whose contents mean something different on every
// platform, they are far too long to type, and they are less durable than they
// look: on macOS one carries the USB socket the device was plugged into, so
// moving it changes the identifier, and on Linux one can be a position in a
// list that shifts when an unrelated sound card is unplugged. A name survives
// both of those.
//
// So a source is named and nothing else. When one has to be opened, the
// identifier is looked up fresh from the name at that moment, inside malgo.go,
// which means it never has to be stored, shown or typed.
//
// The cost is that two identical interfaces report one name and cannot be told
// apart from here. Anything resolving a typed name therefore has to find both
// and say so rather than choose, because choosing would be a coin toss the user
// could not see.
//
// # Listing does not open, and that is load bearing
//
// Sources opens nothing, which on macOS is what keeps it out of the microphone
// permission prompt: enumeration does not ask for access, and only opening a
// stream does. So a picker can be shown, and a typed name can be checked, without
// the operating system interrupting to ask about a device nobody has chosen yet.
//
// Open is the one thing here that does ask. Whatever calls it should therefore
// call it when somebody actually wants to listen rather than when the program
// starts, or the prompt arrives attached to the wrong action and the microphone
// indicator stays lit for a device nothing is reading.
package audioin

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Open starts recording from the source called name and calls onFrames with the
// audio as it arrives.
//
// The format is the one the constants above describe: interleaved signed 16-bit
// little-endian stereo at 48 kHz. It is asked of the library rather than
// discovered, and the library converts if the hardware disagrees, so it holds
// whatever was plugged in.
//
// onFrames is called on a thread belonging to the audio library, not a goroutine,
// and the rules that come with that are strict enough to be worth stating twice:
//
//   - The slice is only valid until the call returns. It is the library's own
//     buffer and it will be written over. Anything that has to outlive the call
//     has to be copied out of it.
//   - It must not block, allocate heavily, or take a lock that anything slow
//     holds. The audio hardware is waiting, and time spent here is a gap in the
//     recording rather than a delay in delivering it.
//   - It says nothing about how many frames arrive at a time. The count depends
//     on the platform and the device and changes between callbacks, so anything
//     wanting fixed-size frames has to cut them itself.
//
// The count of frames is not passed because the length of the slice already
// says it, and two ways of saying one thing is two ways to get it wrong.
//
// This is the call that asks macOS for permission to record, so the first one on
// a machine raises a prompt. A refusal does not fail here: the stream opens and
// delivers digital silence, which is indistinguishable from a cable nobody
// plugged in, so whatever is listening has to notice the silence and say so.
//
// Parameters:
//   - name: the source to record from, matched the way Resolve matches
//   - onFrames: called with each batch of captured audio, under the rules above
//
// Returns:
//   - *Capture that delivers audio until Close is called on it
//   - error if onFrames is nil, the audio system cannot be asked, the name
//     matches no source or more than one, the device cannot be opened or
//     started, or this build was made without audio support
//
// Errors:
//   - ErrNoSource: if nothing attached is called name
//   - ErrAmbiguousSource: if more than one attached source is called name
func Open(name string, onFrames func(pcm []byte)) (*Capture, error) {
	if onFrames == nil {
		return nil, errors.New("opening a sound input with nothing to give the audio to")
	}
	return openFn(name, onFrames)
}

// Resolve checks that exactly one attached source is called name, and returns
// it spelled the way the operating system spells it.
//
// It opens nothing, which is the whole reason to have it separately from Open.
// A name can be checked the moment it is typed, while somebody is still looking
// at the terminal to see the mistake, and checking costs no permission prompt
// and lights no microphone indicator. Opening can then wait until there is
// something to listen for.
//
// The canonical spelling is returned rather than the one that was typed, so that
// what gets shown afterwards matches what a listing shows.
//
// Parameters:
//   - name: what somebody typed or saved, matched case-insensitively with
//     surrounding space ignored
//
// Returns:
//   - string holding the matched source's name as the operating system spells it
//   - error if the audio system cannot be asked, or the name matches no source
//     or more than one
//
// Errors:
//   - ErrNoSource: if nothing attached is called name
//   - ErrAmbiguousSource: if more than one attached source is called name
func Resolve(name string) (string, error) {
	sources, err := Sources()
	if err != nil {
		return "", err
	}

	names := make([]string, len(sources))
	for i, s := range sources {
		names[i] = s.Name
	}

	at, err := pickSource(names, name)
	if err != nil {
		return "", err
	}
	return names[at], nil
}

// Sources returns every audio input attached to this computer, sorted by name.
//
// Finding none is an answer rather than a failure, the same way finding no
// scanners is: a machine with no sound input is unusual and not broken, and the
// caller has better advice to offer than this package does. An error means the
// audio system itself could not be asked.
//
// The order is this package's and not the operating system's. The system
// returns devices in whatever order it currently knows them, which changes as
// they come and go, and a listing that reshuffles between two runs is hard to
// read and hard to compare. Sorting by name is what stops that.
//
// It takes no context, unlike everything else in this tool that talks to
// hardware. Those take one because the scanner is slow and a caller has to be
// able to give up on it. This is a local query that answers in milliseconds,
// and the call into the audio library cannot be interrupted once it has
// started, so a context here would be a promise that could not be kept.
//
// Returns:
//   - []Source holding every attached input, sorted by name ignoring case
//   - error if the audio system itself could not be asked, or this build was
//     made without audio support
func Sources() ([]Source, error) {
	sources, err := listSourcesFn()
	if err != nil {
		return nil, err
	}
	sortSources(sources)
	return sources, nil
}

// pickSource finds the one source called name among names, and returns where it
// sits in that list.
//
// The index rather than the name, because the caller is about to want the audio
// library's own identifier for it, and that is only reachable by position. names
// therefore has to be in the library's order rather than in the sorted order
// Sources hands out, which is why this takes a list of strings rather than a
// list of Source: passing the wrong one would type-check.
//
// Matching is case-insensitive, so that a name copied out of a listing works
// whatever a shell or a JSON file did to it on the way. An exactly matching name
// wins over a differently cased one, which matters only in the odd case of two
// devices whose names differ by case alone: there, one of them is exactly what
// was asked for, and calling that ambiguous would be refusing to answer a
// question that has an answer.
//
// Two sources with the same name are refused rather than chosen between. That is
// the promise this package's doc makes about a source being nothing but its
// name: the two are genuinely indistinguishable from here, so picking one would
// be a coin toss the caller could not see and could not repeat.
//
// Parameters:
//   - names: every attached source's name, in the audio library's own order
//   - name: the name to find, matched case-insensitively with surrounding
//     space ignored
//
// Returns:
//   - int index into names of the one matching source
//   - error if the name is blank, matches nothing, or matches more than one
//
// Errors:
//   - ErrNoSource: if name is blank or nothing in names matches it
//   - ErrAmbiguousSource: if more than one entry in names matches it
func pickSource(names []string, name string) (int, error) {
	want := strings.TrimSpace(name)
	if want == "" {
		return 0, fmt.Errorf("%w: no name was given", ErrNoSource)
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
		return 0, fmt.Errorf("%w: %q", ErrNoSource, want)
	default:
		return 0, fmt.Errorf("%w: %d of them are called %q, and nothing here can tell them apart",
			ErrAmbiguousSource, len(found), want)
	}
}

// sortSources puts a listing in this package's order: by name, ignoring case.
//
// Two sources can share a name, because two identical USB interfaces do. The
// sort is stable so that those keep the order the system gave them. That makes
// no difference while a source is only a name, since two sources sharing one
// are the same value and swapping them changes nothing, but it is the behaviour
// to want the moment a source carries anything else.
//
// Parameters:
//   - sources: the listing to sort, reordered in place
func sortSources(sources []Source) {
	slices.SortStableFunc(sources, func(a, b Source) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}
