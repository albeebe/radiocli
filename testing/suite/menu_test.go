// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// menuReport is the shape "menu" prints as JSON. It is null when the scanner
// is not in a menu, which is an answer rather than a failure.
type menuReport struct {
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Items []struct {
		Name        string `json:"name"`
		Index       string `json:"index"`
		Highlighted bool   `json:"highlighted"`
	} `json:"items"`
}

// readMenu reads the menu the scanner is on, if it is on one.
func readMenu(t *testing.T, args ...string) *menuReport {
	t.Helper()

	res := mustRun(t, append([]string{"-o", "json"}, args...)...)

	var report *menuReport
	if err := json.Unmarshal([]byte(res.stdout), &report); err != nil {
		t.Fatalf("radiocli %s did not print JSON: %v\nstdout: %s",
			strings.Join(args, " "), err, res.stdout)
	}
	return report
}

// inMenu reports whether the scanner is currently in a menu.
func inMenu(t *testing.T) bool {
	t.Helper()
	return readMenu(t, "menu") != nil
}

// TestMenu checks reading the menu, which changes nothing whichever screen the
// scanner happens to be on.
func TestMenu(t *testing.T) {
	needScanner(t)

	report := readMenu(t, "menu")

	res := mustRun(t, "menu")
	switch {
	case report == nil:
		if !strings.Contains(res.stderr, "not in a menu") {
			t.Errorf("the scanner is not in a menu but the text output does not say so:\n%s%s",
				res.stdout, res.stderr)
		}
	default:
		if report.Title == "" {
			t.Error("the scanner is in a menu with no title")
		}
		if !strings.Contains(res.stdout, report.Title) {
			t.Errorf("the text output does not name the menu %q:\n%s", report.Title, res.stdout)
		}
	}

	t.Run("a menu that does not exist", func(t *testing.T) {
		res := mustFail(t, "no menu is called", "menu", "open", "bogus")

		// The refusal lists the names that would have worked, which is the
		// only place they are written down for a user.
		for _, name := range []string{"top", "settings", "system", "department", "channel"} {
			if !strings.Contains(res.stderr, name) {
				t.Errorf("the refusal does not offer %q as a menu name:\n%s", name, res.stderr)
			}
		}
	})
}

// TestMenu_Navigation opens a menu, moves around inside it, and closes it
// again. The scanner stops scanning while it is in a menu, so this is a write.
func TestMenu_Navigation(t *testing.T) {
	needWrites(t)

	// However this test ends, the scanner goes back to scanning.
	t.Cleanup(func() { mustRun(t, "scan") })

	opened := readMenu(t, "menu", "open", "settings")
	if opened == nil {
		t.Fatal("opening the settings menu left the scanner outside the menus")
	}
	if opened.Title == "" {
		t.Error("the settings menu has no title")
	}
	if len(opened.Items) == 0 {
		t.Fatal("the settings menu has no entries")
	}

	highlighted := 0
	for _, item := range opened.Items {
		if item.Name == "" {
			t.Errorf("an entry of %q has no name: %+v", opened.Title, item)
		}
		if item.Highlighted {
			highlighted++
		}
	}
	if highlighted != 1 {
		t.Errorf("%d entries are highlighted, wanted exactly one", highlighted)
	}

	// Reading the menu separately has to agree with what opening it reported.
	if again := readMenu(t, "menu"); again == nil || again.Title != opened.Title {
		t.Errorf("opening reported %q but reading reports %v", opened.Title, again)
	}

	// Going back and closing are both refused on some screens, which is the
	// scanner's own answer rather than a fault in the tool. What has to hold
	// is that a refusal changes nothing and says so, and that the documented
	// way out works either way.
	t.Run("going back one level", func(t *testing.T) {
		res := run(t, "menu", "back")

		if res.code != 0 {
			t.Logf("the scanner refused to go back from %q: %s", opened.Title, firstLine(res.stderr))
			if !inMenu(t) {
				t.Error("going back was refused but the scanner left the menus anyway")
			}
			return
		}
		if !strings.Contains(res.stderr+res.stdout, "menu") {
			t.Errorf("going back reported nothing about where it landed:\n%s%s", res.stdout, res.stderr)
		}
	})

	t.Run("closing the menu", func(t *testing.T) {
		// Get back into a menu first, since "back" may already have left.
		mustRun(t, "menu", "open", "top")

		res := run(t, "menu", "close")
		if res.code != 0 {
			t.Logf("the scanner refused to close the menu: %s", firstLine(res.stderr))

			// This is what the refusal tells the reader to do, so it had
			// better work.
			mustRun(t, "scan")
			if inMenu(t) {
				t.Error("closing the menu was refused and running scan did not get out either")
			}
			return
		}

		if !strings.Contains(res.stderr, "left the menus") {
			t.Errorf("closing the menu did not say it had:\n%s", res.stderr)
		}
		if inMenu(t) {
			t.Error("the scanner is still in a menu after closing it")
		}
	})
}

// TestMenuOpen_EveryMenu opens each menu the tool names, to check the names
// still map onto menus the scanner has.
//
// The names fall into two kinds, and the difference is the whole point of this
// test. Most menus open from scanning with nothing to say which one. The rest
// belong to a particular system, department, channel, site or tone-out slot,
// and the scanner refuses to open those without being told which, because
// there is no such thing as "the system menu" on its own.
func TestMenuOpen_EveryMenu(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	for _, name := range []string{
		"top", "favorites", "search-range", "search-options",
		"close-call", "close-call-band", "weather", "settings", "broadcast-screen",
	} {
		t.Run(name, func(t *testing.T) {
			mustRun(t, "menu", "open", name)

			if report := readMenu(t, "menu"); report == nil {
				t.Errorf("opening the %s menu reported success but left the scanner outside the menus", name)
			}
		})
	}

	// Refused, and refused in a way that says what the scanner answered. The
	// tool reporting success here would be the real failure: it would mean
	// claiming to have opened a menu that was never opened.
	for _, name := range []string{"system", "department", "channel", "site", "tone-out"} {
		t.Run(name+" without an index", func(t *testing.T) {
			mustFail(t, "opening the "+name+" menu", "menu", "open", name)
		})
	}
}

// systemNamed finds the listing entry for one system of a list, so that its
// index is the scanner's own rather than a guess.
func systemNamed(t *testing.T, list, name string) system {
	t.Helper()

	for _, s := range readSystems(t, list) {
		if s.Name == name {
			return s
		}
	}

	t.Fatalf("the list %q does not hold a system called %q", list, name)
	return system{}
}

// indexProbeSpan is how far past a department's own index to look for the
// channels inside it.
//
// A search is needed because nothing reports a channel's index: "channels"
// lists names and frequencies, and the index its menu answers to is not among
// them.
//
// It is a span rather than a ceiling because the indexes are not small numbers
// on a scanner that has been used. They are one sparse space shared by every
// level, and they climb as entries are created: searching from zero found the
// suite's own channel at 29 on a quiet scanner, and failed to find it at all
// once a few tests had built and removed lists first. Where it never moves is
// relative to its department, which sits just below it and does have a
// published index.
const indexProbeSpan = 64

// findChannelMenu looks for an index that opens one of these channels, starting
// from the index of the department holding them, and returns the index and the
// channel it opened.
//
// The indexes have gaps and are shared with the systems and departments, so
// there is nothing to calculate exactly: the only way to find one is to try.
// Starting from the department turns that from a search of the whole space into
// a few steps.
func findChannelMenu(t *testing.T, from int, channels []channel) (string, string) {
	t.Helper()

	named := map[string]bool{}
	for _, c := range channels {
		named[c.Name] = true
	}

	for i := from; i < from+indexProbeSpan; i++ {
		index := fmt.Sprint(i)

		// Each attempt starts from scanning. An index that lands on the main
		// menu leaves the scanner there, and what the next one does depends on
		// where it was standing when it was asked.
		mustRun(t, "scan")

		res := run(t, "menu", "open", "channel", index)
		if res.code != 0 {
			continue
		}
		if report := readMenu(t, "menu"); report != nil && named[report.Title] {
			return index, report.Title
		}
	}
	return "", ""
}

// departmentIndex reports the index the scanner gives a department, which is
// where a search for the channels inside it starts.
func departmentIndex(t *testing.T, system, name string) int {
	t.Helper()

	for _, d := range readDepartments(t, system) {
		if d.Name != name {
			continue
		}

		index, err := strconv.Atoi(d.Index)
		if err != nil {
			t.Fatalf("the department %q has index %q, which is not a number", name, d.Index)
		}
		return index
	}

	t.Fatalf("the system %q does not hold a department called %q", system, name)
	return 0
}
