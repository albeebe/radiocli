// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"fmt"
	"log"
	"sync"
	"testing"
)

// Everything in this file exists so that the tests never touch memory that
// belongs to whoever is running them.
//
// The suite builds its own favorites list, with its own system, department and
// channels inside it, and every test that needs something to work on works on
// that. Nothing reads a name off the scanner and writes to it. A run on a
// scanner straight out of the box tests exactly as much as a run on a scanner
// full of somebody's own lists, because the tests supply their own subject
// either way.
//
// Two shapes are on offer. Tests that only need something to look at, walk to,
// or rename and rename back share one tree, built once for the whole run,
// because building it costs about a minute of the scanner typing. Tests that
// delete what they are given build their own, so one test removing a
// department cannot leave the next one without a department.

// The names everything the suite builds is given.
//
// They all start with testName, which is what the sweep at the end of a run
// looks for, and they are distinct so that a failure names the level it
// happened at.
const (
	scratchSystemName     = testName + " SYS"
	scratchDepartmentName = testName + " DEPT"
	scratchChannelName    = testName + " CH"
	scratchSecondChannel  = testName + " CH TWO"

	// The names a test that builds its own entries uses. They have to differ
	// from the shared ones above, because a name is how everything here is
	// looked up: two lists called the same thing, or two systems, and the
	// lookup is ambiguous and the tool refuses it. That is the tool behaving
	// correctly, and it is the suite's job not to ask.
	ownListName       = testName + " OWN"
	ownSystemName     = testName + " OWN SYS"
	ownDepartmentName = testName + " OWN DEPT"
	ownChannelName    = testName + " OWN CH"

	// What the two scratch channels are tuned to. Both are inside the band the
	// scanner covers, and they are far enough apart that no rounding could
	// confuse one for the other. They are checked rather than assumed: a
	// channel that quietly did not take its frequency should fail a test
	// rather than pass one.
	scratchFrequency       = "155.550"
	scratchSecondFrequency = "154.100"
)

// tree is a favorites list the suite owns, and what is inside it.
type tree struct {
	list       string
	system     string
	department string

	// channels are in the order they were created, which is the order the
	// scanner lists them in.
	channels    []string
	frequencies []string
}

// shared is the tree that tests which change nothing permanent work in. It is
// built on first use and removed once, after every test has finished.
var shared struct {
	once sync.Once
	tree tree
	err  error
}

// scratch returns the shared tree, building it the first time it is asked for.
//
// It is not built in TestMain, because a run that only asks for read-only
// tests should not have to wait a minute for a tree it will never look at.
func scratch(t *testing.T) tree {
	t.Helper()
	needWrites(t)

	shared.once.Do(func() {
		shared.tree, shared.err = buildTree()
	})

	if shared.err != nil {
		t.Fatalf("the suite could not build the entries it works in: %v", shared.err)
	}
	return shared.tree
}

// buildTree creates the list, system, department and channel the shared tree
// is made of.
//
// It reports errors rather than failing a test, because whichever test asked
// for it first would otherwise be blamed for a scanner that refused something.
func buildTree() (tree, error) {
	made := tree{
		list:        testName,
		system:      scratchSystemName,
		department:  scratchDepartmentName,
		channels:    []string{scratchChannelName, scratchSecondChannel},
		frequencies: []string{scratchFrequency, scratchSecondFrequency},
	}

	// Two channels rather than one, so that a listing has something to get
	// wrong: a walk that lands on the first channel every time reads as
	// correct when there is only one.
	steps := [][]string{
		{"favorites", "new", made.list},
		{"systems", "new", made.list, made.system, "--type", "Conventional"},
		{"departments", "new", made.system, made.department},
		{"channels", "new", made.department, scratchFrequency, scratchChannelName},
		{"channels", "new", made.department, scratchSecondFrequency, scratchSecondChannel},
	}

	for _, step := range steps {
		res, err := execute(step...)
		if err != nil {
			return tree{}, err
		}
		if res.code != 0 {
			return tree{}, fmt.Errorf("radiocli %s: %s", join(step), firstLine(res.stderr))
		}
	}

	log.Printf("built the entries the tests work in, inside the list %q", made.list)
	return made, nil
}

// removeScratch deletes the shared tree, if one was built.
//
// Deleting the list takes the system, department and channels with it, so this
// is one command however much was built inside it.
func removeScratch() {
	if shared.tree.list == "" {
		return
	}

	res, err := execute("favorites", "delete", shared.tree.list, "--yes")
	if err != nil || res.code != 0 {
		log.Printf("the list the tests work in is still on the scanner: "+
			"run \"radiocli favorites delete %q --yes\"", shared.tree.list)
		return
	}
	log.Printf("removed the entries the tests worked in")
}

// ownList creates a favorites list for one test to work in, and deletes it
// when that test finishes.
//
// This is for the tests that delete what they are given. They cannot share,
// because the point of them is that what they were given stops existing.
func ownList(t *testing.T) string {
	t.Helper()

	// Creating a list switches it on for scanning, so what the scanner is
	// working through has to go back too.
	keepScannedLists(t)

	return listNamed(t, ownListName)
}

// listNamed creates a favorites list under a given name and deletes it when the
// test finishes.
//
// It does not put the scanned lists back, because a test making more than one
// list must do that once for itself: each call would otherwise register its own
// restore, and the last of those to run would name a list that had already been
// deleted.
func listNamed(t *testing.T, name string) string {
	t.Helper()
	needWrites(t)

	mustRun(t, "favorites", "new", name)

	t.Cleanup(func() {
		if res := run(t, "favorites", "delete", name, "--yes"); res.code != 0 {
			t.Errorf("the list %q could not be deleted and is still on the scanner: %s",
				name, firstLine(res.stderr))
		}
	})

	if !named(readFavorites(t), name) {
		t.Fatalf("the list %q was not created", name)
	}
	return name
}

// ownSystem creates a system inside a list the test owns.
func ownSystem(t *testing.T, list string) string {
	t.Helper()
	return newSystem(t, list, ownSystemName)
}

// newSystem creates a system under a given name, inside a list the test owns.
//
// Nothing deletes it: it goes when the list it is in goes, and deleting it
// first would be several menus' worth of work to reach the same place.
func newSystem(t *testing.T, list, name string) string {
	t.Helper()

	mustRun(t, "systems", "new", list, name, "--type", "Conventional")

	if !hasSystem(readSystems(t, list), name) {
		t.Fatalf("the system %q was not created in %q", name, list)
	}
	return name
}

// ownDepartment creates a department inside a system the test owns.
func ownDepartment(t *testing.T, system string) string {
	t.Helper()

	mustRun(t, "departments", "new", system, ownDepartmentName)

	if !hasDepartment(readDepartments(t, system), ownDepartmentName) {
		t.Fatalf("the department %q was not created in %q", ownDepartmentName, system)
	}
	return ownDepartmentName
}

// join renders a command for a message.
func join(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
