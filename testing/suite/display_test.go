// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

import (
	"strings"
	"testing"
)

// displayReport is the shape "display" prints as JSON.
type displayReport struct {
	Mode  string `json:"mode"`
	Color bool   `json:"color"`
	Entry string `json:"entry"`
}

// displayMode is one way the scanner can draw its screen, under the name the
// tool reports for it and the wording the scanner's own menu uses.
type displayMode struct {
	name  string
	entry string
	color bool
}

// displayModes are the three the scanner offers. The menu wording is checked
// along with the mode because it is what a caller matches against the screen,
// and a firmware that renamed one would otherwise be found out only by a menu
// walk that quietly failed.
var displayModes = []displayMode{
	{"color", "Color Mode", true},
	{"black", "Black w/White Text", false},
	{"white", "White w/Black Text", false},
}

// readDisplay reads how the scanner is drawing its screen.
func readDisplay(t *testing.T) displayReport {
	t.Helper()

	var report displayReport
	mustJSON(t, &report, "display")
	return report
}

// TestDisplay checks the reading, which is a plain read over the wire and
// works whatever the scanner is doing.
func TestDisplay(t *testing.T) {
	needScanner(t)

	report := readDisplay(t)

	want, ok := findDisplayMode(report.Mode)
	if !ok {
		t.Fatalf("the display mode is %q, which is none of color, black or white", report.Mode)
	}
	if report.Color != want.color {
		t.Errorf("the mode is %q but color reports %v", report.Mode, report.Color)
	}
	if report.Entry != want.entry {
		t.Errorf("the mode is %q but the menu wording is %q, wanted %q",
			report.Mode, report.Entry, want.entry)
	}

	t.Run("the subcommand reports the same thing", func(t *testing.T) {
		var same displayReport
		mustJSON(t, &same, "display", "mode")

		if same.Mode != report.Mode {
			t.Errorf("\"display mode\" reports %q where \"display\" reports %q",
				same.Mode, report.Mode)
		}
	})

	t.Run("the status command agrees", func(t *testing.T) {
		// The two commands read the same field, so a disagreement means one of
		// them is reading a stale or a different one.
		var status struct {
			Display string `json:"display"`
		}
		mustJSON(t, &status, "status")

		if status.Display != report.Mode {
			t.Errorf("status reports the display as %q where display reports %q",
				status.Display, report.Mode)
		}
	})

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "display")

		for _, want := range []string{"display: " + report.Mode, "menu:", report.Entry} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("the text output is %q, wanted it to contain %q",
					firstLine(res.stdout), want)
			}
		}
	})

	t.Run("modes the scanner does not have", func(t *testing.T) {
		// These are refused before the scanner is touched, so they are safe to
		// run without -writes.
		mustFail(t, "is not a display mode", "display", "mode", "purple")
		mustFail(t, "is not a display mode", "display", "mode", "colour")
		mustFail(t, "is not a display mode", "display", "mode", "0")
		mustFail(t, "accepts at most 1 arg", "display", "mode", "color", "black")
	})
}

// TestDisplayMode checks that each of the three modes takes, and that the mode
// is put back afterwards.
//
// Each one is a menu walk, so this is the slow test of the three commands that
// touch this setting. It is also the only one that proves the walk still finds
// the entries by name on the firmware under test.
func TestDisplayMode(t *testing.T) {
	needWrites(t)

	before := readDisplay(t)
	t.Cleanup(func() {
		mustRun(t, "display", "mode", before.Mode)
	})

	for _, mode := range displayModes {
		t.Run(mode.name, func(t *testing.T) {
			var set displayReport
			mustJSON(t, &set, "display", "mode", mode.name)

			// What the command prints is what the scanner reported when read
			// back, so this catches a mode the scanner quietly declined.
			if set.Mode != mode.name {
				t.Errorf("setting the mode to %q reported %q", mode.name, set.Mode)
			}
			if set.Color != mode.color {
				t.Errorf("the mode is %q but color reports %v", set.Mode, set.Color)
			}

			// And reading it again in a separate run catches a mode that took
			// only until the connection closed.
			if now := readDisplay(t); now.Mode != mode.name {
				t.Errorf("the mode was set to %q but reads back as %q later",
					mode.name, now.Mode)
			}

			// Setting the mode it is already in must not touch the scanner, and
			// must still report.
			var again displayReport
			mustJSON(t, &again, "display", "mode", mode.name)
			if again.Mode != mode.name {
				t.Errorf("setting the mode to %q when it was already there reported %q",
					mode.name, again.Mode)
			}
		})
	}

	t.Run("the scanner is left scanning", func(t *testing.T) {
		// The walk ends inside Display Options and has to back out of it. A
		// scanner left in a menu refuses most other commands, so this is the
		// failure that would break every test after it.
		mustRun(t, "display", "mode", before.Mode)
		mustRun(t, "scanning")
	})
}

// findDisplayMode looks a mode up by the name the tool reports for it.
func findDisplayMode(name string) (displayMode, bool) {
	for _, mode := range displayModes {
		if mode.name == name {
			return mode, true
		}
	}
	return displayMode{}, false
}
