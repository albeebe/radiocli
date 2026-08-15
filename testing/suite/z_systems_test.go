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

// system is one row of the systems listing.
type system struct {
	Name      string `json:"name"`
	Index     string `json:"index"`
	Kind      string `json:"kind"`
	Avoided   bool   `json:"avoided"`
	QuickKey  string `json:"quickKey"`
	NumberTag string `json:"numberTag"`
}

// aList picks a favorites list to read from, for the tests that only read.
//
// It takes whatever is on the scanner, because a read cannot hurt it. On a
// scanner with nothing in it there is nothing to read, and the test says so
// rather than pretending to have checked something: the same commands are
// tested thoroughly under -writes, against entries the suite builds itself.
//
// The built-in lists are skipped, and the full database is skipped hardest:
// listing its systems locks the scanner up until it is power cycled, which is
// why the harness refuses that command outright.
func aList(t *testing.T) favorite {
	t.Helper()

	for _, l := range readFavorites(t) {
		if l.BuiltIn {
			continue
		}
		if len(readSystems(t, l.Index)) > 0 {
			return l
		}
	}

	t.Skip("this scanner holds no systems to read: run without -writes=false to have the suite " +
		"build its own and test against those")
	return favorite{}
}

// readSystems reads the systems inside one list.
func readSystems(t *testing.T, list string) []system {
	t.Helper()

	var systems []system
	mustJSON(t, &systems, "systems", list)
	return systems
}

// TestSystems checks the level below a favorites list.
func TestSystems(t *testing.T) {
	needScanner(t)

	list := aList(t)
	systems := readSystems(t, list.Name)
	if len(systems) == 0 {
		t.Skipf("the list %q holds no systems", list.Name)
	}

	indexes := map[string]bool{}
	for _, s := range systems {
		if s.Name == "" {
			t.Errorf("a system in %q was reported with no name: %+v", list.Name, s)
		}
		if s.Index == "" {
			t.Errorf("the system %q was reported with no index", s.Name)
		}
		if indexes[s.Index] {
			t.Errorf("two systems share the index %q", s.Index)
		}
		indexes[s.Index] = true

		// The kind is what tells a conventional system from a trunked one, and
		// it decides whether channels carry frequencies or talkgroups.
		if s.Kind == "" {
			t.Errorf("the system %q was reported with no kind", s.Name)
		}
	}

	t.Run("naming the list by its index", func(t *testing.T) {
		byIndex := readSystems(t, list.Index)

		if len(byIndex) != len(systems) {
			t.Errorf("the list holds %d systems by name and %d by index",
				len(systems), len(byIndex))
		}
	})

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "systems", list.Name)

		for _, heading := range []string{"NAME", "TYPE", "SCANNED"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
		for _, s := range systems {
			if !strings.Contains(res.stdout, s.Name) {
				t.Errorf("the table does not list %q:\n%s", s.Name, res.stdout)
			}
		}
	})

	t.Run("a list that does not exist", func(t *testing.T) {
		mustFail(t, "no favorites list is called", "systems", "NO SUCH LIST")
	})
}

// TestSystemsGoto checks that the tool can put the scanner into one system's
// menu.
func TestSystemsGoto(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	own := scratch(t)

	mustRun(t, "systems", "goto", own.system)

	if report := readMenu(t, "menu"); report == nil {
		t.Fatalf("going to the system %q left the scanner outside the menus", own.system)
	}

	t.Run("a system that does not exist", func(t *testing.T) {
		mustFail(t, "no system is called", "systems", "goto", "NO SUCH SYSTEM")
	})
}

// TestSystemsRename renames a system and renames it back.
//
// It renames one the suite created. Renaming types a name one character at a
// time and a run interrupted halfway leaves the name half typed, which is not
// something to risk on an entry somebody owns.
func TestSystemsRename(t *testing.T) {
	own := scratch(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	const renamed = testName + " RENAMED"

	mustRun(t, "systems", "rename", own.system, renamed)
	if !hasSystem(readSystems(t, own.list), renamed) {
		t.Fatalf("the system %q was not renamed to %q", own.system, renamed)
	}

	mustRun(t, "systems", "rename", renamed, own.system)
	if !hasSystem(readSystems(t, own.list), own.system) {
		t.Fatalf("the system is still called %q rather than %q", renamed, own.system)
	}
}

// TestSystemsNew creates a system and deletes it again.
//
// Everything happens inside a favorites list made for the test and removed at
// the end, so nothing here touches a list anybody uses.
func TestSystemsNew(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	name := ownSystemName + " NEW"

	res := mustRun(t, "systems", "new", list, name, "--type", "Conventional")
	if !strings.Contains(res.stdout, name) {
		t.Errorf("creating the system did not report its name:\n%s", res.stdout)
	}

	systems := readSystems(t, list)
	if !hasSystem(systems, name) {
		t.Fatalf("the system %q was not created in %q", name, list)
	}

	t.Run("it is the type it was asked for", func(t *testing.T) {
		for _, s := range systems {
			if strings.EqualFold(s.Name, name) && s.Kind != "Conventional" {
				t.Errorf("the system was created as %q, wanted Conventional", s.Kind)
			}
		}
	})

	t.Run("deleting it again", func(t *testing.T) {
		mustFail(t, "pass --yes", "systems", "delete", name)

		mustRun(t, "systems", "delete", name, "--yes")
		if hasSystem(readSystems(t, list), name) {
			t.Fatalf("the system %q is still there after deleting it", name)
		}
	})

	t.Run("a type the scanner does not have", func(t *testing.T) {
		mustFail(t, "", "systems", "new", list, ownListName+" BAD", "--type", "Telepathy")
	})

	t.Run("no type given at all", func(t *testing.T) {
		mustFail(t, "", "systems", "new", list, ownListName+" BAD")
	})

	t.Run("a system the scanner does not have", func(t *testing.T) {
		mustFail(t, "no system is called", "systems", "delete", "NO SUCH SYSTEM", "--yes")
	})
}

// TestSystemsDelete_TakesItsContents checks that deleting a container takes
// what is inside it.
//
// The whole cleanup scheme rests on this: the suite creates a list, builds a
// tree inside it, and deletes the list. If that left orphans behind, every run
// would litter the scanner a little more.
func TestSystemsDelete_TakesItsContents(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	system := ownSystem(t, list)
	department := ownDepartment(t, system)

	mustRun(t, "systems", "delete", system, "--yes")

	if hasSystem(readSystems(t, list), system) {
		t.Fatalf("the system %q is still there after deleting it", system)
	}

	// The department went with it, so nothing can find it any more.
	res := run(t, "departments", department)
	if res.code == 0 {
		t.Errorf("the department %q outlived the system that held it", department)
	}
}

// hasSystem reports whether a system of this name is in the list.
func hasSystem(systems []system, name string) bool {
	for _, s := range systems {
		if strings.EqualFold(s.Name, name) {
			return true
		}
	}
	return false
}
