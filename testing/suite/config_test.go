// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/10/2026

package suite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setting is one row of the config listing.
type setting struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Default string `json:"default"`
	Changed bool   `json:"changed"`
}

// scratchConfig returns the path of a config file of the test's own, holding
// nothing.
//
// Every test here that writes uses one. The suite runs against a config file
// holding the port of the scanner it found, and a test that wrote to it would
// change what every test after it talks to. A file named with --config has to
// exist, so it is created empty rather than left to the command.
func scratchConfig(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("writing a config file to test against: %v", err)
	}
	return path
}

// TestConfig reads the settings.
//
// Not a write, and not a test that needs a scanner: this is the one command in
// the tool that never opens the serial port.
func TestConfig(t *testing.T) {
	var found []setting
	mustJSON(t, &found, "config")

	// The three the tool keeps. A fourth appearing here is a setting somebody
	// added without documenting it, and a missing one is a setting that
	// quietly stopped being reachable. The scanner is not among them: it is
	// named per invocation with --device and never written down.
	want := []string{"output", "pace", "verbose"}
	if len(found) != len(want) {
		t.Fatalf("config reports %d settings, wanted %d: %+v", len(found), len(want), found)
	}
	for i, name := range want {
		if found[i].Name != name {
			t.Errorf("setting %d is %q, wanted %q", i, found[i].Name, name)
		}
	}

	for _, s := range found {
		if s.Changed != (s.Value != s.Default) {
			t.Errorf("%q reports changed=%v with value %q and default %q",
				s.Name, s.Changed, s.Value, s.Default)
		}
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "config")

		for _, heading := range []string{"NAME", "VALUE", "DEFAULT"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
	})

	t.Run("reading one setting", func(t *testing.T) {
		// The bare value, with nothing around it, is the whole point of get:
		// it is meant to be read straight into a variable.
		res := mustRun(t, "config", "get", "pace")

		if got := strings.TrimSpace(res.stdout); got == "" || strings.Contains(got, "pace") {
			t.Errorf("config get printed %q, wanted the value alone", res.stdout)
		}
	})

	t.Run("a setting that does not exist", func(t *testing.T) {
		mustFail(t, "no setting called", "config", "get", "nonsense")
	})

	t.Run("a flag that is not a setting", func(t *testing.T) {
		// wait is deliberately not stored, and saying so is more use than
		// saying it is unknown.
		mustFail(t, "is not a setting", "config", "get", "wait")
	})
}

// TestConfigPath reports which file is in use.
func TestConfigPath(t *testing.T) {
	res := mustRun(t, "config", "path")

	// The suite passes --config, so the answer has to be that file rather than
	// the one belonging to whoever is running the tests.
	if got := strings.TrimSpace(res.stdout); got != harness.config {
		t.Errorf("config path reports %q, wanted the config the suite is using, %q",
			got, harness.config)
	}

	t.Run("printing it as json", func(t *testing.T) {
		var where struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
		}
		mustJSON(t, &where, "config", "path")

		if where.Path != harness.config {
			t.Errorf("config path reports %q, wanted %q", where.Path, harness.config)
		}
		if !where.Exists {
			t.Errorf("config path says %q does not exist, but the suite wrote it", where.Path)
		}
	})
}

// TestConfigSet writes settings to a config file of the test's own.
//
// This changes a file rather than the scanner, so it runs without one attached
// and without -writes: nothing here can leave a radio in an odd state.
func TestConfigSet(t *testing.T) {
	path := scratchConfig(t)

	// --config again, after the one the suite adds. The last spelling of a
	// flag is the one that counts, so this is the file that gets written.
	own := func(args ...string) []string {
		return append([]string{"--config", path}, args...)
	}

	mustRun(t, own("config", "set", "pace", "medium")...)

	res := mustRun(t, own("config", "get", "pace")...)
	if got := strings.TrimSpace(res.stdout); got != "medium" {
		t.Fatalf("the pace is %q after setting it to medium", got)
	}

	// Written where the tool will find it again, in the file it was told to
	// use rather than anywhere else.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the config back: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("the config file is not valid JSON: %v\n%s", err, data)
	}
	if stored["pace"] != "medium" {
		t.Errorf("the file holds pace %v, wanted medium:\n%s", stored["pace"], data)
	}

	t.Run("the suite's own config is untouched", func(t *testing.T) {
		// The reason every write here names a file of its own. Writing to the
		// suite's config would change how every test after this one runs.
		res := mustRun(t, "config", "get", "pace")
		if got := strings.TrimSpace(res.stdout); got != "turbo" {
			t.Errorf("the suite's pace is %q after another config was written, wanted turbo", got)
		}
	})

	t.Run("unsetting a value", func(t *testing.T) {
		mustRun(t, own("config", "unset", "pace")...)

		var s setting
		mustJSON(t, &s, own("config", "get", "pace")...)

		if s.Value != s.Default {
			t.Errorf("the pace is %q after unsetting it, wanted the default %q",
				s.Value, s.Default)
		}
		if s.Changed {
			t.Errorf("the pace still reports as changed after being unset: %+v", s)
		}
	})

	t.Run("a value the tool cannot use", func(t *testing.T) {
		mustFail(t, "invalid pace", own("config", "set", "pace", "instant")...)

		// Refused before anything is written, which is the point of checking
		// first: a stored bad value would be refused by every command
		// afterwards, including the one that would put it back.
		res := mustRun(t, own("config", "get", "pace")...)
		if got := strings.TrimSpace(res.stdout); got == "instant" {
			t.Errorf("a refused value was written anyway")
		}
	})

	t.Run("writing one setting leaves the others", func(t *testing.T) {
		mustRun(t, own("config", "set", "verbose", "true")...)
		mustRun(t, own("config", "set", "output", "json")...)

		// Read as JSON rather than as a line, because setting output to json
		// is a setting that takes effect: every run after it renders that way,
		// including this one. That is the behaviour, so the test reads what
		// the tool now writes rather than the text form it no longer does.
		var s setting
		mustJSON(t, &s, own("config", "get", "verbose")...)

		if s.Value != "true" {
			t.Errorf("verbose is %q after another setting was written", s.Value)
		}
	})
}
