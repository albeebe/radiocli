// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/15/2026

package main

import (
	"bytes"
	"strings"
	"testing"
)

// drawing runs a set of events through a renderer and returns what it printed.
func drawing() (*renderer, *bytes.Buffer) {
	out := &bytes.Buffer{}

	r := newRenderer()
	r.colour = false
	r.out = out

	return r, out
}

// feed puts a test through the renderer the way go test would report it: the
// check starts, prints whatever it printed, and then ends.
func feed(r *renderer, name, action string, elapsed float64, output ...string) {
	r.handle(event{Action: "run", Test: name})
	for _, line := range output {
		r.handle(event{Action: "output", Test: name, Output: line + "\n"})
	}
	r.handle(event{Action: action, Test: name, Elapsed: elapsed})
}

// TestDrawsTheCommandTree pins the shape of the tree, which is the whole point
// of this tool and the part with nothing else to catch it.
//
// The gutters are the fragile bit. The tree is printed as the run goes, so
// every line is written before what follows it is known, and the one thing that
// can be fixed up afterwards is the connector on the check held back for it.
func TestDrawsTheCommandTree(t *testing.T) {
	r, out := drawing()

	feed(r, "TestRadiocli_Help/every command explains itself", "pass", 0.8)
	feed(r, "TestRadiocli_Help", "pass", 0.9)

	feed(r, "TestBacklight/reading the backlight setting", "pass", 0.4)
	feed(r, "TestBacklight", "pass", 0.5)
	feed(r, "TestBacklightKeysEnable/lighting the keypad", "pass", 0.6)
	feed(r, "TestBacklightKeysEnable", "pass", 0.7)

	feed(r, "TestChannelsNew/creating a channel", "pass", 2.4)
	feed(r, "TestChannelsNew/a department that does not exist", "skip", 0,
		"harness_test.go:488: no favorites list to write to")
	feed(r, "TestChannelsNew", "pass", 2.5)

	r.summarise()

	want := strings.TrimPrefix(`
RADIOCLI
  │   └── every command explains itself                   ✓ 0.8s
  │
  ├── BACKLIGHT
  │   │   └── reading the backlight setting               ✓ 0.4s
  │   │
  │   ├── KEYS
  │   │   ├── ENABLE
  │   │   │   └── lighting the keypad                     ✓ 0.6s
  │
  ├── CHANNELS
  │   ├── NEW
  │   │   ├── creating a channel                          ✓ 2.4s
  │   │   └── a department that does not exist            Skipped
  │   │       └── no favorites list to write to

4 passed, 1 skipped
`, "\n")

	if got := out.String(); got != want {
		t.Errorf("the tree came out as:\n%s\nwanted:\n%s", got, want)
	}
}

// TestEveryResultCarriesATime checks that nothing in the result column is
// blank, however quick the check was. A column with holes in it reads as a
// broken clock rather than as a run full of fast checks.
func TestEveryResultCarriesATime(t *testing.T) {
	r, out := drawing()

	feed(r, "TestBattery/printing it as text", "pass", 0)
	feed(r, "TestBattery/a reading that takes a moment", "pass", 1.24)
	feed(r, "TestBattery", "pass", 1.3)
	r.summarise()

	for _, want := range []string{"✓ 0.0s", "✓ 1.2s"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no %q in the output:\n%s", want, out.String())
		}
	}
}

// TestDoesNotCountATestTwice checks the rule that keeps a failure from being
// reported at every level it bubbled up through.
//
// Go fails a test function when a check inside it fails, so the same failure
// arrives twice: once for the check, once for the function holding it. Counting
// both doubles the tally and prints the failure under a leaf nobody wrote.
func TestDoesNotCountATestTwice(t *testing.T) {
	r, _ := drawing()

	feed(r, "TestChannels/listing names only", "pass", 0.9)
	feed(r, "TestChannels/a department that does not exist", "fail", 0.3,
		"want an error, got exit status 0")
	feed(r, "TestChannels", "fail", 1.3)

	if r.passed != 1 || r.failed != 1 {
		t.Errorf("counted %d passed and %d failed, wanted 1 and 1", r.passed, r.failed)
	}
	if len(r.failures) != 1 {
		t.Errorf("named %d failures, wanted 1: %v", len(r.failures), r.failures)
	}

	// The body of a function that fails on its own account still earns a line,
	// because no check underneath it is carrying that failure.
	r, out := drawing()
	feed(r, "TestStatus/reading the state", "pass", 0.4)
	feed(r, "TestStatus", "fail", 0.9, "the scanner did not answer")
	r.summarise()

	if r.failed != 1 {
		t.Errorf("counted %d failed, wanted 1", r.failed)
	}
	if !strings.Contains(out.String(), "the command itself") {
		t.Errorf("a failure in the function's own body was dropped:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "the scanner did not answer") {
		t.Errorf("the failure's own output was dropped:\n%s", out.String())
	}
}

// TestNamesACommandAgainWhenItComesBack checks what happens when a command's
// tests are not all in one place.
//
// The tree is printed as the run goes, so it follows the order the tests run
// in, and that order is not always the tree's. A test for "channels new" lives
// in the file that builds trunked systems, because that is where the system it
// needs is built, so the run reaches "channels new", goes elsewhere, and comes
// back. Printed bare, those late checks would hang under whichever subcommand
// happened to close above them and read as its.
func TestNamesACommandAgainWhenItComesBack(t *testing.T) {
	r, out := drawing()

	feed(r, "TestScanningSystems/two lists switched on", "pass", 1.1)
	feed(r, "TestScanningSystems", "pass", 1.2)

	// Back up to the parent command, which is the case that used to run the
	// two commands' checks together in one column.
	feed(r, "TestScanning_Channels/printing it as text", "pass", 0.8)
	feed(r, "TestScanning_Channels", "pass", 0.9)
	r.summarise()

	got := out.String()
	if strings.Count(got, "SCANNING") != 2 {
		t.Errorf("SCANNING is named %d times, wanted 2:\n%s", strings.Count(got, "SCANNING"), got)
	}

	// The second SCANNING has to come after SYSTEMS, and the check that follows
	// it has to sit under it rather than under SYSTEMS.
	systems := strings.Index(got, "SYSTEMS")
	again := strings.LastIndex(got, "SCANNING")
	text := strings.Index(got, "printing it as text")
	if !(systems < again && again < text) {
		t.Errorf("the command was not named again before its own checks:\n%s", got)
	}
}

// TestFilesTestsUnderTheirCommand checks the naming convention the suite is
// written to, which is what decides where a test lands in the tree.
func TestFilesTestsUnderTheirCommand(t *testing.T) {
	r, _ := drawing()

	cases := []struct {
		test  string
		path  string
		label string
		ok    bool
	}{
		{"TestChannels", "channels", "", true},
		{"TestChannelsNew", "channels new", "", true},
		{"TestChannelsNew/creating a channel", "channels new", "creating a channel", true},
		{"TestBacklightKeysEnable", "backlight keys enable", "", true},
		{"TestChannelsNew_SimilarNames", "channels new", "similar names", true},
		{"TestRadiocli_Help", "", "help", true},
		{"TestNonsense", "", "", false},
	}

	for _, c := range cases {
		path, label, ok := r.locate(c.test)
		if ok != c.ok {
			t.Errorf("%s: matched a command %v, wanted %v", c.test, ok, c.ok)
			continue
		}
		if got := strings.Join(path, " "); got != c.path {
			t.Errorf("%s: filed under %q, wanted %q", c.test, got, c.path)
		}
		if label != c.label {
			t.Errorf("%s: labelled %q, wanted %q", c.test, label, c.label)
		}
	}
}

// TestSkipsCommandsNothingTested checks that a run filtered down to one command
// draws that command rather than the whole tree with empty branches.
func TestSkipsCommandsNothingTested(t *testing.T) {
	r, out := drawing()

	feed(r, "TestVolumeSet/setting the volume", "pass", 0.4)
	feed(r, "TestVolumeSet", "pass", 0.5)
	r.summarise()

	got := out.String()
	if strings.Contains(got, "CHANNELS") || strings.Contains(got, "BACKLIGHT") {
		t.Errorf("commands nothing tested were drawn:\n%s", got)
	}
	if !strings.Contains(got, "VOLUME") || !strings.Contains(got, "SET") {
		t.Errorf("the command that was tested is missing:\n%s", got)
	}
}

// TestKeepsChecksClearOfSubcommands checks the extra gutter that separates a
// command's own checks from its subcommands.
//
// A command that has subcommands indents its checks one level further, so the
// branch column belongs to commands alone. Which side of that a command falls
// on comes from the checklist rather than from what this run happened to
// exercise, so a narrowed run draws the same shape as a full one.
func TestKeepsChecksClearOfSubcommands(t *testing.T) {
	// volume has a subcommand, battery has none.
	if got, want := gutterFor([]string{"volume"}), "  │   │   "; got != want {
		t.Errorf("volume's checks hang from %q, wanted %q", got, want)
	}
	if got, want := gutterFor([]string{"battery"}), "  │   "; got != want {
		t.Errorf("battery's checks hang from %q, wanted %q", got, want)
	}
}
