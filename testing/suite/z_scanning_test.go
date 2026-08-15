// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

// This file runs late on purpose. Go runs tests in the order their files sort,
// and everything in here types names into the scanner one character at a time,
// which is the slowest thing the suite does. Named to sort after the quick
// tests so a run reports most of its answers early. See harness_test.go.

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// TestScanning_Channels checks the channel level of the same question.
//
// A favorites list is walked exactly. The full database cannot be walked that
// way and is watched instead, which is a sample and says so, so the test only
// checks the shape of what comes back.
func TestScanning_Channels(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	var found []struct {
		Source     string `json:"source"`
		System     string `json:"system"`
		Department string `json:"department"`
		Channel    string `json:"channel"`
		Value      string `json:"value"`
	}
	// The watch is capped, because a scanner with the full database switched
	// on would otherwise sit here for the default three quarters of a minute.
	mustJSON(t, &found, "scanning", "--watch", "10s")

	if len(found) == 0 {
		t.Skip("the scanner reported nothing being scanned, which means nothing is switched on")
	}

	for _, c := range found {
		if c.System == "" {
			t.Errorf("a channel was reported with no system: %+v", c)
		}

		// The value is a string and is not always a frequency: a channel on a
		// trunked system carries a talkgroup, which is not megahertz at all.
		if v := c.Value; v != "" && !frequencyOrTalkgroup(v) {
			t.Errorf("the channel %q reports %q, which is neither a frequency nor a talkgroup",
				c.Channel, v)
		}
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "scanning", "--watch", "10s")

		for _, heading := range []string{"SYSTEM", "DEPARTMENT", "CHANNEL", "VALUE"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
	})

	t.Run("a limit that is not a number of channels", func(t *testing.T) {
		mustFail(t, "want 1 or more", "scanning", "--limit", "0")
		mustFail(t, "want 1 or more", "scanning", "--limit=-5")
	})
}

// The entries this test builds for itself.
//
// They carry testName, so that a run killed halfway through leaves something
// recognisable behind, and they are short on purpose. Choosing which lists are
// scanned means walking a screen that writes each list's name and state on one
// line, and cuts the name at around nineteen characters to fit: "RADIOCLI TEST
// SCAN A" and "RADIOCLI TEST SCAN B" both arrive there as "RADIOCLI TEST SCAN",
// and no rule can tell them apart afterwards.
//
// Seventeen characters each, differing before that, so both survive the screen
// intact.
const (
	scanningList  = testName + " ONE"
	scanningOther = testName + " TWO"

	scanningA = testName + " A"
	scanningB = testName + " B"
	scanningC = testName + " C"
	scanningD = testName + " D"
)

// Where to point the scanner to make it walk its own database.
//
// Green Bank, West Virginia, which is the zip the tool's own documentation uses
// throughout and the middle of the National Radio Quiet Zone. It is here for one
// reason: the scanner has to recognise it. The zip is resolved by the scanner
// against the database on its own card, not by this tool, so a made up zip is
// answered with "Out of Range" and the test fails on the setup rather than on
// anything it meant to check. That is what 54321 did.
//
// Which systems are within range is not asserted, and on purpose. They come from
// whatever database happens to be loaded, and are none of this suite's business.
// What is asserted is that the walk happened, closed, and repeated. A quiet zone
// holding nothing at all within range is a skip rather than a failure, which is
// the honest answer: there was no walk to check.
const (
	databaseZip   = "24944"
	databaseRange = "25"
)

// TestScanningSystems_Listed checks the answer that does not touch the
// scanner's controls.
//
// With the full database off, every system being scanned is in a list the
// scanner can be asked about, so the command reads them rather than turning the
// knob. That is worth testing on its own and worth pinning: which way the
// command answers depends on what happens to be switched on, so this test
// switches on exactly one list, its own, and leaves the scanner as it found it.
//
// What it proves is that the fast answer is the same answer. It is checked
// against the systems listing for the same list, which comes from a different
// request entirely.
func TestScanningSystems_Listed(t *testing.T) {
	own := scratch(t)

	// Scanning exactly one list, and one this suite built, makes the expected
	// answer knowable. Whatever was being scanned goes back afterwards.
	keepScannedLists(t)
	mustRun(t, "favorites", "scan", own.list)

	var found []string
	res := mustJSON(t, &found, "scanning", "systems")
	t.Logf("%d systems in %s, read rather than walked", len(found), res.took.Round(time.Millisecond))

	var want []string
	for _, s := range readSystems(t, own.list) {
		if !s.Avoided {
			want = append(want, s.Name)
		}
	}

	if !sameNames(found, want) {
		t.Errorf("scanning reports %s, and the list holds %s",
			strings.Join(found, ", "), strings.Join(want, ", "))
	}

	// Reading the systems presses no keys, so unlike the walk it has no reason
	// ever to have interrupted the scan.
	t.Run("the scanner never stopped scanning", func(t *testing.T) {
		if inMenu(t) {
			t.Error("reading the systems left the scanner in a menu")
		}
		if heldFrequency(t) {
			t.Error("reading the systems left the scanner holding one frequency")
		}
	})

	t.Run("nothing switched on to scan", func(t *testing.T) {
		mustRun(t, "favorites", "scan", "--none")

		res := mustRun(t, "scanning", "systems")
		if strings.TrimSpace(res.stdout) != "" {
			t.Errorf("a scanner with nothing switched on reported systems anyway:\n%s", res.stdout)
		}
		if !strings.Contains(res.stderr, "nothing switched on") {
			t.Errorf("nothing said why there are no systems:\n%s", res.stderr)
		}

		// Put the list back, so the check above does not decide what the rest
		// of this test sees.
		mustRun(t, "favorites", "scan", own.list)
	})
}

// scanningSystems asks what is being scanned, and reports both the systems and
// whether the command said it was turning the knob to find them.
//
// Which of the two ways it answered is worth pinning rather than inferring
// from the timing. It also makes an exact match meaningful: every system these
// tests create is empty, and an empty system is not something the knob can land
// on, so a walk could never report one.
func scanningSystems(t *testing.T) ([]string, bool) {
	t.Helper()

	var names []string
	res := mustJSON(t, &names, "scanning", "systems")

	t.Logf("%d systems in %s", len(names), res.took.Round(time.Millisecond))
	return names, strings.Contains(res.stderr, "Turning the knob")
}

// scanOnly switches on exactly these lists and nothing else.
func scanOnly(t *testing.T, lists ...string) {
	t.Helper()
	mustRun(t, append([]string{"favorites", "scan"}, lists...)...)
}

// Nothing here creates a system the scanner is avoiding, so the rule that
// avoided systems are left out of the answer is not covered. There is no
// command to avoid one: it is done by hand on the scanner's own menu.

// sameNames reports whether two walks found the same systems, whatever order
// each of them started from.
func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	left := append([]string(nil), a...)
	right := append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)

	return equalNames(left, right)
}

// TestScanningSystems checks the command against scanners set up several
// different ways, because how it answers depends entirely on that.
//
// A list with one system in it is covered by TestScanningSystems_Listed. This
// one covers a list with several, two lists at once, and the full database,
// which is the only case that cannot be read and has to be walked.
//
// Everything it reports on is built here and deleted with the test, so the
// expected answer is an exact set rather than a comparison against whatever
// the scanner happened to hold.
func TestScanningSystems(t *testing.T) {
	needWrites(t)

	// One restore of the scanned lists for the whole test, registered before
	// anything is created. Creating a list switches it on for scanning, and a
	// restore registered per list would run after the list it names had already
	// been deleted.
	keepScannedLists(t)
	t.Cleanup(func() { mustRun(t, "scan") })

	list := listNamed(t, scanningList)
	newSystem(t, list, scanningA)
	newSystem(t, list, scanningB)
	newSystem(t, list, scanningC)

	t.Run("one list holding several systems", func(t *testing.T) {
		scanOnly(t, list)

		want := []string{scanningA, scanningB, scanningC}
		found, walked := scanningSystems(t)

		if walked {
			t.Error("the knob was turned for a favorites list, which can be read instead")
		}
		if !sameNames(found, want) {
			t.Errorf("the systems being scanned are %v, wanted %v", found, want)
		}

		t.Run("printing it as text", func(t *testing.T) {
			res := mustRun(t, "scanning", "systems")

			for _, name := range want {
				if !strings.Contains(res.stdout, name) {
					t.Errorf("the text output does not list %q:\n%s", name, res.stdout)
				}
			}
		})
	})

	t.Run("two lists switched on", func(t *testing.T) {
		// The regression this guards is real and was shipped: the walk used to
		// stop at the end of the first list and report that as the whole
		// answer, because it watched the line naming the list rather than
		// asking whether it had left the scanning screen.
		other := listNamed(t, scanningOther)
		newSystem(t, other, scanningD)

		scanOnly(t, list, other)

		want := []string{scanningA, scanningB, scanningC, scanningD}
		found, _ := scanningSystems(t)

		if !sameNames(found, want) {
			t.Errorf("with two lists switched on the systems are %v, wanted %v", found, want)
		}
	})

	t.Run("the full database, which has to be walked", func(t *testing.T) {
		keepLocation(t)

		// Everything else is switched off first, so what comes back is the
		// database alone. Setting a location is what switches it on: there is
		// no other way to ask for it, and a location does nothing without it.
		mustRun(t, "favorites", "scan", "--none")
		mustRun(t, "location", "set", databaseZip, "--range", databaseRange)

		first, walked := scanningSystems(t)

		if !walked {
			t.Error("the database was read rather than walked, and it cannot be read")
		}
		if len(first) == 0 {
			t.Skipf("this scanner's database holds nothing within %s miles of %s, "+
				"so there is no walk to check", databaseRange, databaseZip)
		}

		for _, name := range first {
			if strings.TrimSpace(name) == "" {
				t.Errorf("a system was reported with a blank name, among %d", len(first))
			}
		}

		// The walk resets the scanner before it starts, precisely so that a
		// second run is not affected by the first. If the two disagree about
		// what is there, it did not.
		//
		// They are compared as sets rather than in order. The walk is a loop
		// and it starts wherever the scanner happens to be in that loop, so two
		// complete answers are rotations of each other: the same systems, from
		// a different starting point.
		second, _ := scanningSystems(t)
		if !sameNames(first, second) {
			t.Errorf("two walks disagree about what is being scanned:\nfirst:  %s\nsecond: %s",
				strings.Join(first, ", "), strings.Join(second, ", "))
		}

		t.Run("the scanner is left scanning", func(t *testing.T) {
			if inMenu(t) {
				t.Error("the walk left the scanner in a menu")
			}
			if heldFrequency(t) {
				t.Error("the walk left the scanner holding one frequency")
			}
		})
	})

	t.Run("refusing an argument it does not take", func(t *testing.T) {
		mustFail(t, "", "scanning", "systems", scanningA)
	})
}
