// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/10/2026

package suite

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// TestDevices checks that the tool finds the scanner it is going to spend the
// rest of the suite talking to.
func TestDevices(t *testing.T) {
	needScanner(t)

	type entry struct {
		Port   string `json:"port"`
		Model  string `json:"model"`
		Serial string `json:"serial"`
		Busy   bool   `json:"busy"`
	}

	// Discovery works by opening every serial port on the machine, so it finds
	// nothing while another command still holds one. The operating system
	// takes a moment to release a port after the process using it has exited,
	// and a test suite runs commands back to back, so this waits rather than
	// calling a busy port a missing scanner.
	var found []entry
	for attempt := range 10 {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		found = nil
		mustJSON(t, &found, "devices")

		// A port still held by the last command is listed as busy rather than
		// left out, so waiting for one to be released means waiting for an
		// entry that is not busy. Breaking on any entry at all would end the
		// wait on the very thing it is waiting for.
		if slices.ContainsFunc(found, func(d entry) bool { return !d.Busy }) {
			break
		}
	}

	if len(found) == 0 {
		t.Fatal("no scanners were listed, but one answered when the run started")
	}

	seen := false
	for _, d := range found {
		if d.Port == "" {
			t.Errorf("a scanner was listed with no port: %+v", d)
		}
		// A busy port is listed with nothing but its port, because reading a
		// model means opening a port another command is holding. Every other
		// entry was opened and answered, so it has a model.
		if d.Model == "" && !d.Busy {
			t.Errorf("the scanner on %s was listed with no model", d.Port)
		}
		if d.Busy && d.Model != "" {
			t.Errorf("the busy port %s was listed with a model of %q, which could not have been read",
				d.Port, d.Model)
		}
		if d.Port == harness.device {
			seen = true
		}
	}
	if !seen {
		t.Errorf("the scanner being tested (%s) was not in the list", harness.device)
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "devices")

		for _, heading := range []string{"MODEL", "SERIAL", "PORT"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
		if !strings.Contains(res.stdout, harness.device) {
			t.Errorf("the table does not list %s:\n%s", harness.device, res.stdout)
		}
	})
}
