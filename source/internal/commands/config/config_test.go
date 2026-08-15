// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// The tests here run the command the way it is typed as well as the way it is
// called, because a setting has two sides that can disagree: what the command
// accepts and what the file ends up holding. Every one of them writes to a
// config file of the test's own, so a run never reads or writes the config of
// the person running it.

// execute runs the config command as it would be typed, with the streams the
// App is already pointed at.
//
// The usage and the error are silenced so a refusal is what comes back rather
// than also being printed, which is what main does with it too.
func execute(t *testing.T, app *appcontext.App, args ...string) error {
	t.Helper()

	cmd := New(app)
	cmd.SetArgs(args)
	cmd.SetOut(app.Stdout)
	cmd.SetErr(app.Stderr)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	return cmd.Execute()
}

// nowhereToWrite points the settings at a config file that can be read and not
// written, so the failure to write one is testable, and puts the permission
// back afterwards.
//
// The restriction is checked rather than assumed: a run as a user the
// permissions do not apply to would otherwise report a failure the code never
// had a chance to produce, so the test is skipped instead.
func nowhereToWrite(t *testing.T, app *appcontext.App) {
	t.Helper()

	path := app.Config.Path
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("taking the write permission off %s: %v", path, err)
	}
	if f, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		t.Skip("this user can write to a read-only file, so there is no failure to test")
	}
}

// unreadable leaves the config file as something that is not JSON, so a read
// of it fails where the test needs it to rather than while it is being set up.
func unreadable(t *testing.T, app *appcontext.App) {
	t.Helper()

	if err := os.WriteFile(app.Config.Path, []byte(`{"macros":`), 0o600); err != nil {
		t.Fatalf("breaking the config file: %v", err)
	}
}

// unlocatable hides the user config directory, so the failure to work out
// where the default config file is can be tested without going near the real
// one.
func unlocatable(t *testing.T) {
	t.Helper()

	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")

	if _, err := os.UserConfigDir(); err == nil {
		t.Skip("this machine still has a user config directory, so there is no failure to test")
	}
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Reports: the bare command reports every setting
//   - Subcommands: every verb is attached
func TestNew(t *testing.T) {
	// Verify that the bare command reports rather than changes, which is what
	// keeps a read from turning into a write by mistake.
	t.Run("Reports", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := execute(t, app); err != nil {
			t.Fatalf("config: %v", err)
		}
		if got := out.String(); !strings.Contains(got, "output") || !strings.Contains(got, "pace") {
			t.Errorf("the listing is %q, which does not name the settings", got)
		}
	})

	// Verify that every verb the command documents is actually attached, since
	// a missing one is only found by typing it.
	t.Run("Subcommands", func(t *testing.T) {
		app, _, _ := isolate(t)

		want := map[string]bool{"get": false, "set": false, "unset": false, "path": false, "macro": false}
		for _, sub := range New(app).Commands() {
			if _, ok := want[sub.Name()]; ok {
				want[sub.Name()] = true
			}
		}
		for name, found := range want {
			if !found {
				t.Errorf("the command %q is not attached", name)
			}
		}
	})
}

// Test_newGet tests the newGet function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Reports: the value alone is printed
//   - Unknown: a name that is not a setting is refused
func Test_newGet(t *testing.T) {
	// Verify that the value is printed on its own, so a whole line can be read
	// straight into a variable.
	t.Run("Reports", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := execute(t, app, "get", "output"); err != nil {
			t.Fatalf("config get: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != string(appcontext.OutputText) {
			t.Errorf("config get answered %q, want %q", got, appcontext.OutputText)
		}
	})

	// Verify that a name that is not a setting is refused by the command as
	// well as by the lookup.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := execute(t, app, "get", "nonsense"); err == nil {
			t.Fatal("config get answered for a setting that does not exist")
		}
	})
}

// Test_newPath tests the newPath function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Reports: the file this invocation reads and writes is printed
func Test_newPath(t *testing.T) {
	// Verify that the path is the only thing on stdout, whether or not the
	// file is there.
	t.Run("Reports", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := execute(t, app, "path"); err != nil {
			t.Fatalf("config path: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != app.Config.Path {
			t.Errorf("config path answered %q, want %q", got, app.Config.Path)
		}
	})
}

// Test_newSet tests the newSet function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Writes: the setting reaches the file
//   - Refused: a value the tool cannot use is refused before anything is
//     written
func Test_newSet(t *testing.T) {
	// Verify that the setting is written and read back rather than echoed.
	t.Run("Writes", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := execute(t, app, "set", "pace", string(device.PaceSlow)); err != nil {
			t.Fatalf("config set: %v", err)
		}
		if got := saved(t, app).Pace; got != device.PaceSlow {
			t.Errorf("the file holds pace %q, want %q", got, device.PaceSlow)
		}
		if got := out.String(); !strings.Contains(got, string(device.PaceSlow)) {
			t.Errorf("the answer is %q, which does not report what was written", got)
		}
	})

	// Verify that a value the tool cannot use is refused rather than saved and
	// tripped over later.
	t.Run("Refused", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := execute(t, app, "set", "pace", "nonsense"); err == nil {
			t.Fatal("a pace that is not one was accepted")
		}
		if got := saved(t, app).Pace; got != device.DefaultPace {
			t.Errorf("the file holds pace %q after a refusal, want it unchanged", got)
		}
	})
}

// Test_newUnset tests the newUnset function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - PutsItBack: the setting returns to its default
//   - Unknown: a name that is not a setting is refused
func Test_newUnset(t *testing.T) {
	// Verify that unsetting reports what the default turned out to be, since
	// that is the thing somebody cannot work out from a blank answer.
	t.Run("PutsItBack", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := execute(t, app, "set", "verbose", "true"); err != nil {
			t.Fatalf("config set: %v", err)
		}

		out.Reset()
		if err := execute(t, app, "unset", "verbose"); err != nil {
			t.Fatalf("config unset: %v", err)
		}
		if saved(t, app).Verbose {
			t.Error("verbose is still on after being unset")
		}
		if got := out.String(); !strings.Contains(got, "false") {
			t.Errorf("the answer is %q, which does not report the default", got)
		}
	})

	// Verify that a name that is not a setting is refused.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := execute(t, app, "unset", "nonsense"); err == nil {
			t.Fatal("config unset accepted a setting that does not exist")
		}
	})
}

// Test_allowed tests the allowed function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - One: a value in the list is accepted
//   - None: a value not in the list is not
//   - Empty: nothing is in an empty list
func Test_allowed(t *testing.T) {
	// Verify that a value in the list is found wherever it sits.
	t.Run("One", func(t *testing.T) {
		if !allowed([]string{"text", "json"}, "json") {
			t.Error("json is not accepted, and is one of the values")
		}
	})

	// Verify that a value that is not one is refused.
	t.Run("None", func(t *testing.T) {
		if allowed([]string{"text", "json"}, "yaml") {
			t.Error("yaml is accepted, and is not one of the values")
		}
	})

	// Verify that a setting with no accepted values accepts nothing here, which
	// is why the caller checks the list is not empty first.
	t.Run("Empty", func(t *testing.T) {
		if allowed(nil, "anything") {
			t.Error("an empty list accepted a value")
		}
	})
}

// Test_apply tests the apply function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Applied: an accepted value reaches the settings
//   - NotOneOfThem: a value outside the list is refused by name
//   - Unparseable: a value the setting cannot parse is refused
//   - Unusable: a value that parses and leaves the settings unusable is
//     refused too
func Test_apply(t *testing.T) {
	verbose, err := lookup("verbose")
	if err != nil {
		t.Fatalf("looking up verbose: %v", err)
	}

	// Verify that an accepted value reaches the settings.
	t.Run("Applied", func(t *testing.T) {
		cfg := appcontext.Defaults()

		if err := apply(verbose, cfg, "true"); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !cfg.Verbose {
			t.Error("verbose is off after being set to true")
		}
	})

	// Verify that a value outside the accepted ones names the setting and the
	// values there are, which is what somebody who guessed wrong needs.
	t.Run("NotOneOfThem", func(t *testing.T) {
		err := apply(verbose, appcontext.Defaults(), "maybe")
		if err == nil {
			t.Fatal("a value that is not one of the accepted ones was applied")
		}
		if !strings.Contains(err.Error(), "invalid verbose") {
			t.Errorf("the refusal said %q, which does not name the setting", err)
		}
	})

	// Verify that a setting which parses its own values reports its own
	// refusal, named after the setting.
	t.Run("Unparseable", func(t *testing.T) {
		free := setting{
			Name: "verbose",
			get:  verbose.get,
			set:  verbose.set,
		}

		err := apply(free, appcontext.Defaults(), "maybe")
		if err == nil {
			t.Fatal("a value the setting cannot parse was applied")
		}
		if !strings.Contains(err.Error(), "verbose: invalid value") {
			t.Errorf("the refusal said %q, which does not name the setting and the reason", err)
		}
	})

	// Verify that a value which parses and still leaves the settings unusable
	// is refused. Checked twice on purpose: a setting this command allowed and
	// every other command refused would be worse than a refusal here.
	t.Run("Unusable", func(t *testing.T) {
		free := setting{
			Name: "output",
			get:  func(c *appcontext.Config) string { return string(c.Output) },
			set: func(c *appcontext.Config, v string) error {
				c.Output = appcontext.OutputFormat(v)
				return nil
			},
		}

		err := apply(free, appcontext.Defaults(), "yaml")
		if err == nil {
			t.Fatal("a value that leaves the settings unusable was applied")
		}
		if !strings.Contains(err.Error(), "output: invalid output format") {
			t.Errorf("the refusal said %q, which does not name the setting and the reason", err)
		}
	})
}

// Test_describe tests the describe function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Unchanged: a setting nobody has set matches its default
//   - Changed: a setting that differs from its default is marked
func Test_describe(t *testing.T) {
	pace, err := lookup("pace")
	if err != nil {
		t.Fatalf("looking up pace: %v", err)
	}

	// Verify that a setting nobody has set reports its default as the value in
	// effect, because the default is what is in effect.
	t.Run("Unchanged", func(t *testing.T) {
		r := describe(pace, appcontext.Defaults())

		if r.Name != "pace" {
			t.Errorf("the report is for %q, want pace", r.Name)
		}
		if r.Value != r.Default {
			t.Errorf("the value is %q and the default %q, want them alike", r.Value, r.Default)
		}
		if r.Changed {
			t.Error("a setting nobody has set is reported as changed")
		}
	})

	// Verify that a setting which differs from its default is marked, which is
	// the question somebody is really asking when they look down the list.
	t.Run("Changed", func(t *testing.T) {
		cfg := appcontext.Defaults()
		cfg.Pace = device.PaceSlow

		r := describe(pace, cfg)
		if r.Value != string(device.PaceSlow) {
			t.Errorf("the value is %q, want %q", r.Value, device.PaceSlow)
		}
		if !r.Changed {
			t.Error("a setting that differs from its default is not marked as changed")
		}
	})
}

// Test_encode tests the encode function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Indented: the value is written to stdout as JSON
func Test_encode(t *testing.T) {
	// Verify that what is written is JSON a reader can parse, indented so a
	// person can read it too.
	t.Run("Indented", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := encode(app, report{Name: "output", Value: "json", Default: "text", Changed: true}); err != nil {
			t.Fatalf("encode: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("what was written is not JSON: %v\n%s", err, out.String())
		}
		if got.Name != "output" || got.Value != "json" || !got.Changed {
			t.Errorf("the report came back as %+v", got)
		}
		if !strings.Contains(out.String(), "\n  ") {
			t.Errorf("the JSON is %q, want it indented", out.String())
		}
	})
}

// Test_list tests the list function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - None: no values render as nothing
//   - One: a single value renders alone
//   - Two: two values are joined by "or"
//   - Several: the rest are joined by commas
func Test_list(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{"None", nil, ""},
		{"One", []string{"text"}, "text"},
		{"Two", []string{"text", "json"}, "text or json"},
		{"Several", []string{"slow", "medium", "fast", "turbo"}, "slow, medium, fast or turbo"},
	}

	for _, c := range cases {
		// Verify that the values read as a sentence rather than as a list, so
		// a refusal can name them without a line of its own.
		t.Run(c.name, func(t *testing.T) {
			if got := list(c.values); got != c.want {
				t.Errorf("the values rendered as %q, want %q", got, c.want)
			}
		})
	}
}

// Test_lookup tests the lookup function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Found: a setting is found however it was typed
//   - NotASetting: a name this command will never accept says why
//   - Unknown: any other name names the settings there are
func Test_lookup(t *testing.T) {
	// Verify that the name is matched whatever case and spacing it is typed
	// in, since it is typed rather than picked from a list.
	t.Run("Found", func(t *testing.T) {
		for _, name := range []string{"output", "OUTPUT", "  Output  "} {
			s, err := lookup(name)
			if err != nil {
				t.Fatalf("looking up %q: %v", name, err)
			}
			if s.Name != "output" {
				t.Errorf("looking up %q found %q", name, s.Name)
			}
		}
	})

	// Verify that a name somebody could reasonably try is answered with the
	// reason it is not a setting rather than with a denial.
	t.Run("NotASetting", func(t *testing.T) {
		_, err := lookup("wait")
		if err == nil {
			t.Fatal("wait was accepted as a setting")
		}
		if !strings.Contains(err.Error(), "--wait flag") {
			t.Errorf("the refusal said %q, which does not say where the value comes from", err)
		}
	})

	// Verify that any other name is answered with the settings there are.
	t.Run("Unknown", func(t *testing.T) {
		_, err := lookup("nonsense")
		if err == nil {
			t.Fatal("a name that is not a setting was accepted")
		}
		if !strings.Contains(err.Error(), names()) {
			t.Errorf("the refusal said %q, which does not name the settings", err)
		}
	})
}

// Test_names tests the names function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Alphabetical: every setting is named, in an order somebody can scan
func Test_names(t *testing.T) {
	// Verify that every setting in the table is named, alphabetically, since
	// this is read in a refusal rather than in a listing.
	t.Run("Alphabetical", func(t *testing.T) {
		got := names()

		for _, s := range settings {
			if !strings.Contains(got, s.Name) {
				t.Errorf("the settings read as %q, which leaves out %q", got, s.Name)
			}
		}
		if want := "output, pace, verbose"; got != want {
			t.Errorf("the settings read as %q, want %q", got, want)
		}
	})
}

// Test_paceValues tests the paceValues function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - EveryPace: each pace the device package keeps is offered
func Test_paceValues(t *testing.T) {
	// Verify that the paces offered are the ones the device package has, in
	// its order, so the table cannot drift from what a scanner accepts.
	t.Run("EveryPace", func(t *testing.T) {
		got := paceValues()

		if len(got) != len(device.Paces) {
			t.Fatalf("%d paces are offered, want %d", len(got), len(device.Paces))
		}
		for i, p := range device.Paces {
			if got[i] != string(p) {
				t.Errorf("pace %d is %q, want %q", i, got[i], p)
			}
		}
	})
}

// Test_renderSaved tests the renderSaved function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Text: the setting is named beside its value
//   - JSON: the whole report is written
//   - Unreadable: a config file that cannot be read is reported
func Test_renderSaved(t *testing.T) {
	output, err := lookup("output")
	if err != nil {
		t.Fatalf("looking up output: %v", err)
	}

	// Verify that the setting is named beside its value, which is what tells
	// somebody the write they just asked for happened.
	t.Run("Text", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := renderSaved(app, output); err != nil {
			t.Fatalf("renderSaved: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != "output: text" {
			t.Errorf("the answer is %q, want %q", got, "output: text")
		}
	})

	// Verify that the JSON answer is the whole report rather than the value
	// alone.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := isolate(t)
		app.Config.Output = appcontext.OutputJSON

		if err := renderSaved(app, output); err != nil {
			t.Fatalf("renderSaved: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, out.String())
		}
		if got.Name != "output" || got.Default != "text" {
			t.Errorf("the report came back as %+v", got)
		}
	})

	// Verify that a config file that cannot be read is reported rather than
	// answered from the defaults.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := renderSaved(app, output); err == nil {
			t.Fatal("renderSaved answered from a file that is not JSON")
		}
	})
}

// Test_runGet tests the runGet function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Text: the value alone is printed
//   - JSON: the whole report is written
//   - Unknown: a name that is not a setting is refused
//   - Unreadable: a config file that cannot be read is reported
func Test_runGet(t *testing.T) {
	// Verify that the value is printed with no name and no padding, so the
	// whole line can be used as it is.
	t.Run("Text", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := runGet(app, "pace"); err != nil {
			t.Fatalf("runGet: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != string(device.DefaultPace) {
			t.Errorf("runGet answered %q, want %q", got, device.DefaultPace)
		}
	})

	// Verify that the JSON answer carries the default beside the value, so a
	// reader can see what unsetting it would get them.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := isolate(t)
		app.Config.Output = appcontext.OutputJSON

		if err := runGet(app, "verbose"); err != nil {
			t.Fatalf("runGet: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, out.String())
		}
		if got.Name != "verbose" || got.Value != "false" {
			t.Errorf("the report came back as %+v", got)
		}
	})

	// Verify that a name that is not a setting is refused before the file is
	// read.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := runGet(app, "nonsense"); err == nil {
			t.Fatal("runGet answered for a setting that does not exist")
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runGet(app, "pace"); err == nil {
			t.Fatal("runGet answered from a file that is not JSON")
		}
	})
}

// Test_runList tests the runList function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Text: every setting is listed in a table
//   - JSON: every setting is written as a report
//   - Unreadable: a config file that cannot be read is reported
func Test_runList(t *testing.T) {
	// Verify that the table names every setting, with the value in effect and
	// the default beside it.
	t.Run("Text", func(t *testing.T) {
		app, out, _ := isolate(t)
		if err := app.Config.Update(func(c *appcontext.Config) { c.Verbose = true }); err != nil {
			t.Fatalf("setting verbose: %v", err)
		}

		out.Reset()
		if err := runList(app); err != nil {
			t.Fatalf("runList: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "NAME") || !strings.Contains(got, "DEFAULT") {
			t.Errorf("the listing is %q, which has no heading", got)
		}
		for _, s := range settings {
			if !strings.Contains(got, s.Name) {
				t.Errorf("the listing is %q, which leaves out %q", got, s.Name)
			}
		}
		if !strings.Contains(got, "true") {
			t.Errorf("the listing is %q, which does not hold the value that was set", got)
		}
	})

	// Verify that the JSON answer is a list a reader can walk without knowing
	// the settings in advance.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := isolate(t)
		app.Config.Output = appcontext.OutputJSON

		if err := runList(app); err != nil {
			t.Fatalf("runList: %v", err)
		}

		var got []report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the listing is not JSON: %v\n%s", err, out.String())
		}
		if len(got) != len(settings) {
			t.Errorf("the listing holds %d settings, want %d", len(got), len(settings))
		}
	})

	// Verify that a config file that cannot be read is reported rather than
	// listed as defaults.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runList(app); err == nil {
			t.Fatal("runList listed a file that is not JSON")
		}
	})
}

// Test_runPath tests the runPath function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Exists: the path is the only thing on stdout
//   - Missing: a file that is not there yet is said once, on stderr
//   - JSON: the path is written with whether it exists
//   - Unlocatable: the user config directory cannot be found
func Test_runPath(t *testing.T) {
	// Verify that nothing but the path reaches stdout when the file is there.
	t.Run("Exists", func(t *testing.T) {
		app, out, errs := isolate(t)

		if err := runPath(app); err != nil {
			t.Fatalf("runPath: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != app.Config.Path {
			t.Errorf("runPath answered %q, want %q", got, app.Config.Path)
		}
		if errs.Len() != 0 {
			t.Errorf("stderr holds %q for a file that is there", errs.String())
		}
	})

	// Verify that a file which does not exist yet is still answered with the
	// path, and the note about it goes to stderr so stdout stays usable.
	t.Run("Missing", func(t *testing.T) {
		app, out, errs := isolate(t)
		if err := os.Remove(app.Config.Path); err != nil {
			t.Fatalf("removing the config file: %v", err)
		}

		if err := runPath(app); err != nil {
			t.Fatalf("runPath: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != app.Config.Path {
			t.Errorf("runPath answered %q, want %q", got, app.Config.Path)
		}
		if !strings.Contains(errs.String(), "does not exist yet") {
			t.Errorf("stderr holds %q, which does not say the file is not there", errs.String())
		}
	})

	// Verify that the JSON answer says whether the file exists, which the text
	// answer says on stderr.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := isolate(t)
		app.Config.Output = appcontext.OutputJSON

		if err := runPath(app); err != nil {
			t.Fatalf("runPath: %v", err)
		}

		var got struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, out.String())
		}
		if got.Path != app.Config.Path || !got.Exists {
			t.Errorf("the answer came back as %+v, want the seeded file", got)
		}
	})

	// Verify that a machine with no user config directory is reported rather
	// than answered with a path that was guessed at.
	t.Run("Unlocatable", func(t *testing.T) {
		app, _, _ := isolate(t)
		app.Config.Path = ""
		unlocatable(t)

		if err := runPath(app); err == nil {
			t.Fatal("runPath answered with a path it could not work out")
		}
	})
}

// Test_runSet tests the runSet function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Writes: the setting reaches the file and the settings in memory
//   - Unknown: a name that is not a setting is refused
//   - Refused: a value the tool cannot use is refused before anything is
//     written
//   - Unreadable: a config file that cannot be read is reported
//   - Unwritable: a config file that cannot be written is reported
func Test_runSet(t *testing.T) {
	// Verify that the value reaches the file, and that what is reported is
	// read back from it rather than echoed.
	t.Run("Writes", func(t *testing.T) {
		app, out, _ := isolate(t)

		if err := runSet(app, "output", string(appcontext.OutputJSON)); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		if got := saved(t, app).Output; got != appcontext.OutputJSON {
			t.Errorf("the file holds output %q, want %q", got, appcontext.OutputJSON)
		}

		// Applied in memory too, which is why the answer came back as JSON.
		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, out.String())
		}
		if got.Value != string(appcontext.OutputJSON) {
			t.Errorf("the report came back as %+v", got)
		}
	})

	// Verify that a name that is not a setting is refused before the file is
	// read.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := runSet(app, "nonsense", "true"); err == nil {
			t.Fatal("runSet wrote a setting that does not exist")
		}
	})

	// Verify that a value the tool cannot use is refused, and that nothing was
	// written on the way to refusing it.
	t.Run("Refused", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := runSet(app, "output", "yaml"); err == nil {
			t.Fatal("an output format the tool cannot render was accepted")
		}
		if got := saved(t, app).Output; got != appcontext.OutputText {
			t.Errorf("the file holds output %q after a refusal, want it unchanged", got)
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runSet(app, "verbose", "true"); err == nil {
			t.Fatal("runSet wrote over a file it could not read")
		}
	})

	// Verify that a config file that cannot be written is reported rather than
	// the change being announced when nothing reached the disk.
	t.Run("Unwritable", func(t *testing.T) {
		app, _, _ := isolate(t)
		nowhereToWrite(t, app)

		if err := runSet(app, "verbose", "true"); err == nil {
			t.Fatal("runSet reported a change to a file it cannot write")
		}
	})
}

// Test_runUnset tests the runUnset function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - PutsItBack: the setting returns to its default
//   - Unknown: a name that is not a setting is refused
//   - Unwritable: a config file that cannot be written is reported
func Test_runUnset(t *testing.T) {
	// Verify that the default is written rather than the setting being removed
	// from the file, and that what it turned out to be is reported.
	t.Run("PutsItBack", func(t *testing.T) {
		app, out, _ := isolateWith(t, `{"macros":[],"pace":"slow"}`)

		if err := runUnset(app, "pace"); err != nil {
			t.Fatalf("runUnset: %v", err)
		}
		if got := saved(t, app).Pace; got != device.DefaultPace {
			t.Errorf("the file holds pace %q, want the default %q", got, device.DefaultPace)
		}
		if got := strings.TrimSpace(out.String()); got != "pace: "+string(device.DefaultPace) {
			t.Errorf("the answer is %q, which does not report what the default turned out to be", got)
		}
	})

	// Verify that a name that is not a setting is refused before anything is
	// written.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := runUnset(app, "nonsense"); err == nil {
			t.Fatal("runUnset wrote a setting that does not exist")
		}
	})

	// Verify that a config file that cannot be written is reported.
	t.Run("Unwritable", func(t *testing.T) {
		app, _, _ := isolate(t)
		nowhereToWrite(t, app)

		if err := runUnset(app, "verbose"); err == nil {
			t.Fatal("runUnset reported a change to a file it cannot write")
		}
	})
}

// Test_shown tests the shown function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Value: a value is rendered as it is
//   - Empty: an empty setting is rendered as a dash
func Test_shown(t *testing.T) {
	// Verify that an ordinary value is left alone.
	t.Run("Value", func(t *testing.T) {
		if got := shown("json"); got != "json" {
			t.Errorf("json rendered as %q", got)
		}
	})

	// Verify that an empty setting reads as a real state rather than as a
	// blank column somebody would take for a bug.
	t.Run("Empty", func(t *testing.T) {
		if got := shown(""); got != "-" {
			t.Errorf("an empty setting rendered as %q, want %q", got, "-")
		}
	})
}
