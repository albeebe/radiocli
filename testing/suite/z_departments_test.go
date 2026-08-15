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

// department is one row of the departments listing.
type department struct {
	Name     string `json:"name"`
	Index    string `json:"index"`
	Avoided  bool   `json:"avoided"`
	QuickKey string `json:"quickKey"`
}

// aSystem picks a system to read from, for the tests that only read. It takes
// whatever is on the scanner, the same way and for the same reason as aList.
func aSystem(t *testing.T) system {
	t.Helper()

	systems := readSystems(t, aList(t).Name)
	if len(systems) == 0 {
		t.Skip("this scanner holds no systems to read: run without -writes=false to have the suite " +
			"build its own and test against those")
	}
	return systems[0]
}

// readDepartments reads the departments inside one system.
func readDepartments(t *testing.T, system string) []department {
	t.Helper()

	var departments []department
	mustJSON(t, &departments, "departments", system)
	return departments
}

// TestDepartments checks the level below a system.
func TestDepartments(t *testing.T) {
	needScanner(t)

	parent := aSystem(t)
	departments := readDepartments(t, parent.Name)
	if len(departments) == 0 {
		t.Skipf("the system %q holds no departments", parent.Name)
	}

	indexes := map[string]bool{}
	for _, d := range departments {
		if d.Name == "" {
			t.Errorf("a department in %q was reported with no name: %+v", parent.Name, d)
		}
		if d.Index == "" {
			t.Errorf("the department %q was reported with no index", d.Name)
		}
		if indexes[d.Index] {
			t.Errorf("two departments share the index %q", d.Index)
		}
		indexes[d.Index] = true
	}

	t.Run("naming the system by its index", func(t *testing.T) {
		byIndex := readDepartments(t, parent.Index)

		if len(byIndex) != len(departments) {
			t.Errorf("the system holds %d departments by name and %d by index",
				len(departments), len(byIndex))
		}
	})

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "departments", parent.Name)

		for _, heading := range []string{"NAME", "SCANNED", "QUICK KEY"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
		for _, d := range departments {
			if !strings.Contains(res.stdout, d.Name) {
				t.Errorf("the table does not list %q:\n%s", d.Name, res.stdout)
			}
		}
	})

	t.Run("a system that does not exist", func(t *testing.T) {
		mustFail(t, "no system is called", "departments", "NO SUCH SYSTEM")
	})
}

// TestDepartmentsGoto checks that the tool can put the scanner into one
// department's menu.
func TestDepartmentsGoto(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	own := scratch(t)

	mustRun(t, "departments", "goto", own.department)

	if report := readMenu(t, "menu"); report == nil {
		t.Fatalf("going to the department %q left the scanner outside the menus", own.department)
	}

	t.Run("a department that does not exist", func(t *testing.T) {
		mustFail(t, "no department is called", "departments", "goto", "NO SUCH DEPARTMENT")
	})
}

// TestDepartmentsRename renames a department and renames it back, on one the
// suite created.
func TestDepartmentsRename(t *testing.T) {
	own := scratch(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	const renamed = testName + " RENAMED"

	mustRun(t, "departments", "rename", own.department, renamed)
	if !hasDepartment(readDepartments(t, own.system), renamed) {
		t.Fatalf("the department %q was not renamed to %q", own.department, renamed)
	}

	mustRun(t, "departments", "rename", renamed, own.department)
	if !hasDepartment(readDepartments(t, own.system), own.department) {
		t.Fatalf("the department is still called %q rather than %q", renamed, own.department)
	}
}

// TestDepartmentsNew creates a department and deletes it again, inside a
// favorites list made for the test and removed at the end.
func TestDepartmentsNew(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	system := ownSystem(t, list)
	name := ownDepartmentName + " NEW"

	res := mustRun(t, "departments", "new", system, name)
	if !strings.Contains(res.stdout, name) {
		t.Errorf("creating the department did not report its name:\n%s", res.stdout)
	}

	if !hasDepartment(readDepartments(t, system), name) {
		t.Fatalf("the department %q was not created in %q", name, system)
	}

	t.Run("a new department holds nothing", func(t *testing.T) {
		if channels := readChannels(t, name); len(channels) != 0 {
			t.Errorf("a department created a moment ago already holds %d channels", len(channels))
		}
	})

	t.Run("deleting it again", func(t *testing.T) {
		mustFail(t, "pass --yes", "departments", "delete", name)

		mustRun(t, "departments", "delete", name, "--yes")
		if hasDepartment(readDepartments(t, system), name) {
			t.Fatalf("the department %q is still there after deleting it", name)
		}
	})

	t.Run("a department the scanner does not have", func(t *testing.T) {
		mustFail(t, "no department is called", "departments", "delete", "NO SUCH DEPARTMENT", "--yes")
	})
}

// hasDepartment reports whether a department of this name is in the system.
func hasDepartment(departments []department, name string) bool {
	for _, d := range departments {
		if strings.EqualFold(d.Name, name) {
			return true
		}
	}
	return false
}
