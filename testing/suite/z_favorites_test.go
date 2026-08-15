// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

// This file runs late on purpose. Go runs tests in the order their files sort,
// and everything in here types names into the scanner one character at a time,
// which is the slowest thing the suite does. Named to sort after the quick
// tests so a run reports most of its answers early. See harness_test.go.

import (
	"strings"
	"testing"
)

// favorite is one row of the favorites listing.
type favorite struct {
	Name      string `json:"name"`
	Index     string `json:"index"`
	Monitored bool   `json:"monitored"`
	QuickKey  string `json:"quickKey"`
	NumberTag string `json:"numberTag"`
	BuiltIn   bool   `json:"builtIn"`
}

// readFavorites reads the favorites lists the scanner holds.
func readFavorites(t *testing.T) []favorite {
	t.Helper()

	var lists []favorite
	mustJSON(t, &lists, "favorites")
	if len(lists) == 0 {
		t.Skip("the scanner reports no favorites lists")
	}
	return lists
}

// monitored names the lists currently being scanned.
func monitored(lists []favorite) []string {
	var on []string
	for _, l := range lists {
		if l.Monitored {
			on = append(on, l.Name)
		}
	}
	return on
}

// TestFavorites checks the listing at the top of the scanner's memory.
func TestFavorites(t *testing.T) {
	needScanner(t)

	lists := readFavorites(t)

	indexes := map[string]bool{}
	builtIn := false
	for _, l := range lists {
		if l.Name == "" {
			t.Errorf("a list was reported with no name: %+v", l)
		}
		if l.Index == "" {
			t.Errorf("the list %q was reported with no index", l.Name)
		}
		if indexes[l.Index] {
			t.Errorf("two lists share the index %q", l.Index)
		}
		indexes[l.Index] = true
		builtIn = builtIn || l.BuiltIn
	}

	if !builtIn {
		t.Error("no list is marked built in, but every scanner has the full database")
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "favorites")

		for _, heading := range []string{"NAME", "SCANNED", "QUICK KEY", "NUMBER TAG"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
		for _, l := range lists {
			if !strings.Contains(res.stdout, l.Name) {
				t.Errorf("the table does not list %q:\n%s", l.Name, res.stdout)
			}
		}
	})

	t.Run("a list that does not exist", func(t *testing.T) {
		mustFail(t, "no favorites list is called", "favorites", "scan", "NO SUCH LIST")
	})

	t.Run("flags that contradict each other", func(t *testing.T) {
		mustFail(t, "choose one", "favorites", "scan", "--all", "--none")
		mustFail(t, "choose one", "favorites", "scan", "--none", lists[0].Name)
		mustFail(t, "name the lists to scan", "favorites", "scan")
	})
}

// TestFavoritesScan checks choosing what gets scanned, which is the reason
// most people would reach for this command.
func TestFavoritesScan(t *testing.T) {
	needWrites(t)

	before := readFavorites(t)
	restore := monitored(before)

	t.Cleanup(func() {
		args := append([]string{"favorites", "scan"}, restore...)
		if len(restore) == 0 {
			args = []string{"favorites", "scan", "--none"}
		}
		mustRun(t, args...)
	})

	t.Run("scanning nothing", func(t *testing.T) {
		var after []favorite
		mustJSON(t, &after, "favorites", "scan", "--none")

		if on := monitored(after); len(on) != 0 {
			t.Errorf("after --none the scanner is still scanning %s", strings.Join(on, ", "))
		}
	})

	// The suite's own list, so this never turns somebody's scanning on or off
	// to prove a point. Which lists are scanned is put back either way.
	own := scratch(t)

	t.Run("scanning exactly one list", func(t *testing.T) {
		var after []favorite
		mustJSON(t, &after, "favorites", "scan", own.list)

		on := monitored(after)
		if len(on) != 1 || on[0] != own.list {
			t.Errorf("after naming %q the scanner is scanning %s, wanted only %q",
				own.list, strings.Join(on, ", "), own.list)
		}
	})

	t.Run("naming a list by its index", func(t *testing.T) {
		index := ""
		for _, l := range readFavorites(t) {
			if l.Name == own.list {
				index = l.Index
			}
		}
		if index == "" {
			t.Fatalf("the list %q has no index", own.list)
		}

		var after []favorite
		mustJSON(t, &after, "favorites", "scan", index)

		on := monitored(after)
		if len(on) != 1 || on[0] != own.list {
			t.Errorf("after naming index %q the scanner is scanning %s, wanted only %q",
				index, strings.Join(on, ", "), own.list)
		}
	})
}

// TestFavoritesGoto checks that the tool can put the scanner into one list's
// menu, which is where a list is renamed or given a quick key.
func TestFavoritesGoto(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	// The suite's own list. The built-in lists have no menu of their own, and
	// the tool says so rather than opening something else.
	own := scratch(t)
	mustRun(t, "favorites", "goto", own.list)

	report := readMenu(t, "menu")
	if report == nil {
		t.Fatalf("going to %q left the scanner outside the menus", own.list)
	}
	t.Logf("the scanner landed on the menu titled %q", report.Title)

	t.Run("a list that does not exist", func(t *testing.T) {
		mustFail(t, "no favorites list is called", "favorites", "goto", "NO SUCH LIST")
	})

	t.Run("going to a built-in list", func(t *testing.T) {
		for _, l := range readFavorites(t) {
			if l.BuiltIn {
				mustFail(t, "has no menu of its own", "favorites", "goto", l.Name)
				return
			}
		}
		t.Skip("the scanner reports no built-in lists")
	})
}

// TestFavoritesRename renames a list and renames it back, on one the suite
// created.
//
// Renaming types a name one character at a time, so a run interrupted halfway
// leaves the list called something half typed. That is not a state to leave a
// list somebody owns in, which is why this works on the suite's own.
func TestFavoritesRename(t *testing.T) {
	own := scratch(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	const renamed = testName + " RENAMED"

	mustRun(t, "favorites", "rename", own.list, renamed)
	if !named(readFavorites(t), renamed) {
		t.Fatalf("the list %q was not renamed to %q", own.list, renamed)
	}

	mustRun(t, "favorites", "rename", renamed, own.list)
	if !named(readFavorites(t), own.list) {
		t.Fatalf("the list is still called %q rather than %q", renamed, own.list)
	}
}

// TestFavoritesNew creates a favorites list and deletes it again.
//
// The two belong in one test: creating without deleting leaves something
// behind on a scanner somebody owns, and deleting without creating has nothing
// safe to work on.
func TestFavoritesNew(t *testing.T) {
	needWrites(t)
	keepScannedLists(t)

	before := readFavorites(t)

	res := mustRun(t, "favorites", "new", ownListName)
	if !strings.Contains(res.stdout, ownListName) {
		t.Errorf("creating the list did not report its name:\n%s", res.stdout)
	}

	after := readFavorites(t)
	if !named(after, ownListName) {
		t.Fatalf("the list %q is not there after creating it", ownListName)
	}
	if len(after) != len(before)+1 {
		t.Errorf("the scanner held %d lists and now holds %d", len(before), len(after))
	}

	t.Run("a new list holds nothing", func(t *testing.T) {
		if systems := readSystems(t, ownListName); len(systems) != 0 {
			t.Errorf("a list created a moment ago already holds %d systems", len(systems))
		}
	})

	t.Run("deleting it again", func(t *testing.T) {
		mustFail(t, "pass --yes", "favorites", "delete", ownListName)

		res := mustRun(t, "favorites", "delete", ownListName, "--yes")
		if !strings.Contains(res.stdout, ownListName) {
			t.Errorf("deleting the list did not report its name:\n%s", res.stdout)
		}

		if named(readFavorites(t), ownListName) {
			t.Fatalf("the list %q is still there after deleting it", ownListName)
		}
	})

	t.Run("names the scanner does not have", func(t *testing.T) {
		mustFail(t, "no favorites list is called", "favorites", "delete", "NO SUCH LIST", "--yes")
	})

	t.Run("a built-in list cannot be deleted", func(t *testing.T) {
		for _, l := range readFavorites(t) {
			if l.BuiltIn {
				mustFail(t, "cannot be deleted", "favorites", "delete", l.Name, "--yes")
				return
			}
		}
		t.Skip("the scanner reports no built-in lists")
	})
}

// named reports whether any entry carries this name.
func named(lists []favorite, name string) bool {
	for _, l := range lists {
		if strings.EqualFold(l.Name, name) {
			return true
		}
	}
	return false
}
