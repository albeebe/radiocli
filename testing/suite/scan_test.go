// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package suite

import (
	"strings"
	"testing"
)

// holding reports whether the scanner is parked on one thing rather than
// working through a list.
//
// The screen cannot answer this: a held scanner shows a channel name, and so
// does a scanning one every time it stops on something it is receiving. The
// mode is the only place the difference shows, and status reports it.
func holding(t *testing.T) bool {
	t.Helper()

	var report struct {
		Mode    string `json:"mode"`
		Holding bool   `json:"holding"`
	}
	mustJSON(t, &report, "status")
	return report.Holding
}

// TestScan checks the command that gets the scanner back to normal, from each
// of the four places it can be left: in a menu, holding one frequency, holding
// one channel, and already scanning.
//
// Everything else in the suite relies on this working, which is a good reason
// to test it explicitly rather than trusting the cleanups that call it.
func TestScan(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	t.Run("scanning out of a menu", func(t *testing.T) {
		mustRun(t, "menu", "open", "top")
		if !inMenu(t) {
			t.Fatal("the top menu would not open, so there is nothing to leave")
		}

		res := mustRun(t, "scan")
		if !strings.Contains(res.stderr, "left the menus") {
			t.Errorf("leaving a menu was not reported:\n%s", res.stderr)
		}
		if inMenu(t) {
			t.Error("the scanner is still in a menu")
		}
	})

	t.Run("scanning off a held frequency", func(t *testing.T) {
		mustRun(t, "tune", "162.550")

		// Being out of the menus is not the same as scanning, and a tuned
		// scanner is the case that proves it.
		mustRun(t, "scan")
		if inMenu(t) {
			t.Error("the scanner is in a menu after being told to scan")
		}
		if held := heldFrequency(t); held {
			t.Error("the scanner is still holding one frequency")
		}
	})

	t.Run("scanning off a held channel", func(t *testing.T) {
		// Holding is the case that looks like nothing is wrong: the scanner is
		// out of the menus, answers everything, and is parked on one channel
		// with no sign of it on screen beyond a channel name where "Scanning..."
		// would be. The third soft key is the one that holds the channel, which
		// is what turning the knob does to a scanner sitting on a desk.
		mustRun(t, "scan")

		mustRun(t, "key", "soft3")
		if !holding(t) {
			t.Skip("the soft key did not hold the scanner, so there is nothing to release")
		}

		res := mustRun(t, "scan")
		if !strings.Contains(res.stderr, "holding") {
			t.Errorf("releasing a held scanner was not reported:\n%s", res.stderr)
		}
		if holding(t) {
			t.Error("the scanner is still holding one channel after being told to scan")
		}
	})

	t.Run("scanning off the weather channels", func(t *testing.T) {
		// The fourth place a scanner can be left, and the one nothing else
		// puts right: it is out of the menus, and the hold it is in is one
		// every other command is told to leave alone.
		mustRun(t, "weather")

		res := mustRun(t, "scan")
		if !strings.Contains(res.stderr, "left the weather channels") {
			t.Errorf("leaving the weather channels was not reported:\n%s", res.stderr)
		}

		var report struct {
			Mode string `json:"mode"`
		}
		mustJSON(t, &report, "status")
		if strings.HasPrefix(report.Mode, "WX") {
			t.Errorf("the scanner is still in %q after being told to scan", report.Mode)
		}
	})

	t.Run("asking again while already scanning", func(t *testing.T) {
		res := mustRun(t, "scan")
		if !strings.Contains(res.stderr, "already out of the menus") {
			t.Errorf("running scan on a scanning scanner did not say so:\n%s", res.stderr)
		}
	})
}

// heldFrequency reports whether the scanner is sitting on one frequency rather
// than working through a list. The screen is the only place that says so.
func heldFrequency(t *testing.T) bool {
	t.Helper()

	var report struct {
		Lines []struct {
			Text string `json:"text"`
		} `json:"lines"`
	}
	mustJSON(t, &report, "screen")

	for _, l := range report.Lines {
		if strings.Contains(strings.ToUpper(l.Text), "QUICK SEARCH") {
			return true
		}
	}
	return false
}
