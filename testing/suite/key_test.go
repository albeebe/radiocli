// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"strings"
	"testing"
)

// TestKey_RefusesUnknownNames checks the refusals, which need no scanner
// state changed and are the only part of this command that can be checked
// without pressing anything.
func TestKey_RefusesUnknownNames(t *testing.T) {
	needScanner(t)

	t.Run("a key that does not exist", func(t *testing.T) {
		res := mustFail(t, "", "key", "bogus")

		// The refusal has to list the names that would have worked.
		for _, name := range []string{"menu", "enter", "left", "right", "soft1"} {
			if !strings.Contains(res.stderr, name) {
				t.Errorf("the refusal does not offer %q as a key name:\n%s", name, res.stderr)
			}
		}
	})

	t.Run("an action that does not exist", func(t *testing.T) {
		mustFail(t, "", "key", "--action", "bogus", "menu")
	})

	t.Run("one bad name refuses the whole run", func(t *testing.T) {
		// Nothing should be pressed when part of the run is wrong, so the
		// scanner must be exactly where it was afterwards.
		was := inMenu(t)
		mustFail(t, "", "key", "menu", "bogus")
		if inMenu(t) != was {
			t.Error("a refused run of keys still pressed one of them")
		}
	})
}

// TestKey_Presses presses the menu key and checks the scanner did what a person
// pressing it would see.
//
// This is the blunt instrument of the tool, so it is driven at the pace the
// documentation recommends for driving it by hand rather than at whatever the
// run is configured with.
func TestKey_Presses(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	// Start from scanning, so the key has a known screen to act on. The same
	// key means "avoid this channel" when the scanner is not in a menu, which
	// is exactly why this command is the least predictable one here.
	mustRun(t, "scan")
	if inMenu(t) {
		t.Fatal("the scanner is in a menu after being told to scan")
	}

	mustRun(t, "--pace", "slow", "key", "menu")
	if !inMenu(t) {
		t.Fatal("pressing the menu key did not open a menu")
	}

	// A run of keys, checked by where it ended up rather than by each press,
	// which is all this command promises.
	mustRun(t, "--pace", "slow", "key", "right", "left")
	if !inMenu(t) {
		t.Error("turning the knob inside a menu left the menus")
	}
}
