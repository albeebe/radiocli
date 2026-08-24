// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package suite

import (
	"strings"
	"testing"
)

// TestHeadphone checks that the phase setting can be read off the radio.
//
// The setting exists to correct a headphone socket wired out of phase, and
// which way it is left decides whether anything combining the two sides gets
// usable audio, so being able to see it without picking the radio up is the
// point of the command.
func TestHeadphone(t *testing.T) {
	needScanner(t)

	var got struct {
		Phase string `json:"phase"`
	}
	mustJSON(t, &got, "headphone")

	if got.Phase != "in-phase" && got.Phase != "invert-phase" {
		t.Errorf("the setting reads %q, want one of the two values the scanner offers", got.Phase)
	}

	// The scanner has to be left scanning, since reading this walks it into a
	// menu and every command that does owes putting it back.
	var state struct {
		Mode string `json:"mode"`
	}
	mustJSON(t, &state, "status")
	if strings.Contains(strings.ToLower(state.Mode), "menu") {
		t.Errorf("the scanner was left on %q, want it back out of the menus", state.Mode)
	}
}

// TestHeadphone_RefusesAValueThatDoesNotExist checks that a typo is caught
// before the scanner is touched, which is what makes it free.
func TestHeadphone_RefusesAValueThatDoesNotExist(t *testing.T) {
	needScanner(t)

	res := mustFail(t, "there is no headphone setting called", "headphone", "set", "sideways")
	for _, want := range []string{"in-phase", "invert-phase"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal said %q, want it to name %q", firstLine(res.stderr), want)
		}
	}
}

// TestHeadphoneSet checks that the setting can be changed and reads back
// changed, then puts it back the way it was found.
func TestHeadphoneSet(t *testing.T) {
	needWrites(t)

	var before struct {
		Phase string `json:"phase"`
	}
	mustJSON(t, &before, "headphone")

	other := "in-phase"
	if before.Phase == "in-phase" {
		other = "invert-phase"
	}
	t.Cleanup(func() { mustRun(t, "headphone", "set", before.Phase) })

	var after struct {
		Phase string `json:"phase"`
	}
	mustRun(t, "headphone", "set", other)
	mustJSON(t, &after, "headphone")

	if after.Phase != other {
		t.Errorf("the setting reads %q after being set to %q", after.Phase, other)
	}
}
