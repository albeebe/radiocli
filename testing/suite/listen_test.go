// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package suite

import (
	"strings"
	"testing"
)

// TestAudioListen_RefusesWithNothingToListenTo checks that a run which could
// never produce a sound says so at once.
//
// Audio comes from a daemon holding a named scanner, or from a sound input
// given directly. With neither there is nothing to play, and the refusal has to
// name both ways out rather than reporting a daemon that was never going to be
// there.
func TestAudioListen_RefusesWithNothingToListenTo(t *testing.T) {
	// The suite puts --device in front of every command, so this passes an
	// empty one after it: pflag takes the last it is given, which is how a test
	// overrides the port the harness chose.
	res := run(t, "--device", "", "audio", "listen")
	if res.code == 0 {
		t.Fatal("listening with no scanner and no input was allowed")
	}
	for _, want := range []string{"no scanner named", "--input"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal said %q, wanted it to mention %q", firstLine(res.stderr), want)
		}
	}
}

// TestAudioListen_RefusesASpeakerThatDoesNotExist checks that a name nothing
// answers to is caught rather than played somewhere else.
//
// Falling back to the default output would be the worst kind of success: the
// audio would come out of the wrong device and nothing would say so. The check
// happens before the sound input is opened, so a typo costs a moment rather
// than a sound card being taken and given straight back.
func TestAudioListen_RefusesASpeakerThatDoesNotExist(t *testing.T) {
	res := run(t, "--device", "", "audio", "listen",
		"--input", "Nothing Is Called This", "--speaker", "No Speaker Is Called This")
	if res.code == 0 {
		t.Fatal("a speaker that does not exist was accepted")
	}

	// Either the speakers were refused by name, or this build has no audio
	// support at all, which is a different machine rather than a failure.
	said := res.stderr
	if !strings.Contains(said, "speaker") && !strings.Contains(said, "without audio support") {
		t.Errorf("the refusal said %q, wanted the speakers named", firstLine(said))
	}
}
