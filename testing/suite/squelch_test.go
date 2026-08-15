// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package suite

import (
	"fmt"
	"strings"
	"testing"
)

// squelchReport is the shape "squelch" prints as JSON.
type squelchReport struct {
	Level int `json:"level"`
	Min   int `json:"min"`
	Max   int `json:"max"`
}

// readSquelch reads the level the scanner opens its audio at.
func readSquelch(t *testing.T) squelchReport {
	t.Helper()

	var report squelchReport
	mustJSON(t, &report, "squelch")
	return report
}

// TestSquelch checks the reading and the bounds it comes with.
func TestSquelch(t *testing.T) {
	needScanner(t)

	report := readSquelch(t)

	if report.Min != 0 {
		t.Errorf("the minimum level is %d, wanted 0", report.Min)
	}
	if report.Max <= report.Min {
		t.Errorf("the maximum level is %d, which is not above the minimum of %d",
			report.Max, report.Min)
	}
	if report.Level < report.Min || report.Level > report.Max {
		t.Errorf("the level is %d, outside the %d to %d the same reading reports",
			report.Level, report.Min, report.Max)
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "squelch")

		if want := fmt.Sprintf("squelch: %d of %d", report.Level, report.Max); !strings.Contains(res.stdout, want) {
			t.Errorf("the text output is %q, wanted it to contain %q", firstLine(res.stdout), want)
		}
	})

	t.Run("levels the scanner cannot take", func(t *testing.T) {
		mustFail(t, "out of range", "squelch", "set", fmt.Sprint(report.Max+1))
		// The "--" is for this shell rather than the tool: without it a
		// leading minus reads as a flag.
		mustFail(t, "out of range", "squelch", "set", "--", "-1")
		mustFail(t, "invalid squelch level", "squelch", "set", "open")
		mustFail(t, "invalid squelch level", "squelch", "set", "7.5")
	})
}

// TestSquelchSet checks that setting the squelch takes, at both ends of the
// range and in the middle, and that the level is put back afterwards.
func TestSquelchSet(t *testing.T) {
	needWrites(t)

	before := readSquelch(t)
	t.Cleanup(func() {
		mustRun(t, "squelch", "set", fmt.Sprint(before.Level))
	})

	for _, level := range []int{before.Min, before.Max / 2, before.Max} {
		t.Run(fmt.Sprintf("level %d", level), func(t *testing.T) {
			var set squelchReport
			mustJSON(t, &set, "squelch", "set", fmt.Sprint(level))

			// What the command prints is what the scanner reported when read
			// back, so this catches a level the scanner quietly declined.
			if set.Level != level {
				t.Errorf("setting the level to %d reported %d", level, set.Level)
			}

			// And reading it again in a separate run catches a level that took
			// only until the connection closed.
			if now := readSquelch(t); now.Level != level {
				t.Errorf("the level was set to %d but reads back as %d later", level, now.Level)
			}
		})
	}
}
