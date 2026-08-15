// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

// This file runs late on purpose. Go runs tests in the order their files sort,
// and everything in here types names into the scanner one character at a time,
// which is the slowest thing the suite does. Named to sort after the quick
// tests so a run reports most of its answers early. See harness_test.go.

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMenuOpen_ByIndex opens the menus that belong to one thing, using the
// index the tool reports for it.
//
// The indexes are the scanner's own and have gaps: the only system on a
// scanner can be index 4. Passing a position rather than an index is refused,
// which is why nothing in this suite counts.
func TestMenuOpen_ByIndex(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	// The suite's own system, so this works the same on a scanner with
	// nothing on it as on one full of somebody's lists.
	own := scratch(t)
	parent := systemNamed(t, own.list, own.system)

	t.Run("opening a system by index", func(t *testing.T) {
		report := readMenu(t, "menu", "open", "system", parent.Index)

		if report == nil {
			t.Fatalf("the system menu for index %s did not open", parent.Index)
		}
		if report.Title != parent.Name {
			t.Errorf("the menu that opened is titled %q, wanted the system %q",
				report.Title, parent.Name)
		}
	})

	t.Run("opening a department by index", func(t *testing.T) {
		departments := readDepartments(t, parent.Name)
		if len(departments) == 0 {
			t.Fatalf("the system %q holds no departments", parent.Name)
		}

		report := readMenu(t, "menu", "open", "department", departments[0].Index)

		if report == nil {
			t.Fatalf("the department menu for index %s did not open", departments[0].Index)
		}
		if report.Title != departments[0].Name {
			t.Errorf("the menu that opened is titled %q, wanted the department %q",
				report.Title, departments[0].Name)
		}
	})

	t.Run("opening a tone-out by index", func(t *testing.T) {
		// The tone-out slots are numbered from one, and the scanner labels the
		// first of them zero. Both are always there, configured or not.
		report := readMenu(t, "menu", "open", "tone-out", "1")

		if report == nil {
			t.Fatal("the first tone-out menu did not open")
		}
		if !strings.HasPrefix(report.Title, "Tone-Out") {
			t.Errorf("the menu that opened is titled %q, wanted a tone-out slot", report.Title)
		}
	})

	t.Run("opening a channel by index", func(t *testing.T) {
		channels := readChannels(t, own.department)
		if len(channels) == 0 {
			t.Fatalf("the department %q holds no channels", own.department)
		}

		// The search starts at the department rather than at zero. Channel
		// indexes climb as entries are created, and by the time this runs the
		// tests before it have built and removed several lists: searching from
		// zero found nothing within 32, while the department it is looking
		// inside says exactly where to start.
		from := departmentIndex(t, own.system, own.department)

		index, title := findChannelMenu(t, from, channels)
		if index == "" {
			t.Fatalf("no index from %d to %d opened a channel menu, though %q holds %d channels",
				from, from+indexProbeSpan, own.department, len(channels))
		}
		t.Logf("the channel %q is at index %s, found from its department at %d", title, index, from)
	})

	t.Run("an index nothing is stored at", func(t *testing.T) {
		// Index 0 is a position rather than an index, and no system is stored
		// at it. The scanner refuses, and the tool has to pass that on rather
		// than report having opened something.
		mustFail(t, "opening the system menu", "menu", "open", "system", "0")
	})

	// The indexes are one space shared by every level of the memory, and they
	// are sparse: on a scanner whose only system is 4 and whose departments
	// are 7 and 11, the channels are at 9, 13 and 15. Handed one of the other
	// level's indexes, the scanner does not refuse. It opens the main menu.
	//
	// What this pins down is not that quirk but the thing that makes it
	// survivable: the tool reports the menu it actually landed on, so it never
	// claims to have opened a channel it did not open. A firmware or a tool
	// that starts refusing this instead is an improvement, and passes too.
	t.Run("an index belonging to another level", func(t *testing.T) {
		// Read the memory first, while the scanner is still scanning. Most
		// reads do answer from inside a menu, but not all of them do and not
		// always while the scanner is still busy drawing one, so anything this
		// needs to compare against is collected before it is sent anywhere.
		channels := readChannels(t, own.department)

		// From scanning, and opened once. This lands on the main menu, which
		// takes the scanner a couple of seconds to draw, and asking it to do
		// the same thing again while it is still busy times out.
		mustRun(t, "scan")

		res := run(t, "-o", "json", "menu", "open", "channel", parent.Index)
		if res.code != 0 {
			t.Logf("the scanner refused a system's index as a channel: %s", firstLine(res.stderr))
			return
		}

		var opened *menuReport
		if err := json.Unmarshal([]byte(res.stdout), &opened); err != nil {
			t.Fatalf("opening a channel by a system's index did not print JSON: %v\n%s",
				err, res.stdout)
		}
		if opened == nil {
			t.Fatal("opening a channel by a system's index reported success but opened no menu")
		}

		// Where the tool said it landed has to be where the scanner is.
		where := readMenu(t, "menu")
		if where == nil || where.Title != opened.Title {
			t.Errorf("the tool reported landing on %q, but the scanner is on %v",
				opened.Title, where)
		}

		for _, c := range channels {
			if opened.Title == c.Name {
				t.Errorf("a system's index opened the channel menu for %q, which is not what was asked for",
					c.Name)
			}
		}
		t.Logf("the system's index %s opened %q rather than a channel", parent.Index, opened.Title)
	})
}
