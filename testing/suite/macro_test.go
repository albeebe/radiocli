// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package suite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// macro is one macro as the tool reports it. Declared here rather than imported
// because this module deliberately cannot see the source: what is checked is
// the shape somebody reading the output gets, not the shape the code has.
type macro struct {
	Name      string   `json:"name"`
	Steps     []string `json:"steps"`
	KeepGoing bool     `json:"keepGoing"`
}

// Macros are a list in the config file rather than anything on the scanner, so
// none of this needs a radio and none of it runs one: the tool stores macros
// and a front end is what runs them. Every test here writes to a config file
// of its own, for the reason scratchConfig gives.

// noMacros returns the path of a config file of the test's own that names an
// empty list of macros.
//
// Empty rather than absent, because a file naming none at all gets the four
// built-in macros. What those do is TestConfigMacro_Defaults; everything else starts
// from nothing so a count is a count of what the test itself created.
func noMacros(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"macros": []}`+"\n"), 0o600); err != nil {
		t.Fatalf("writing a config file to test against: %v", err)
	}
	return path
}

// TestConfigMacro covers creating, listing, reading and changing a macro through the
// built binary, which is the only way to find out whether the file it wrote is
// a file it can read back.
func TestConfigMacro(t *testing.T) {
	path := noMacros(t)
	own := func(args ...string) []string {
		return append([]string{"--config", path}, args...)
	}

	t.Run("no macros to begin with", func(t *testing.T) {
		res := mustRun(t, own("config", "macro")...)

		// Said on stderr, so a script counting the lines of stdout gets zero
		// rather than a sentence about there being none.
		if strings.TrimSpace(res.stdout) != "" {
			t.Errorf("an empty list put %q on stdout, wanted nothing", res.stdout)
		}
		if !strings.Contains(res.stderr, "No macros yet") {
			t.Errorf("an empty list said %q, which does not say there are none", res.stderr)
		}

		var found []macro
		mustJSON(t, &found, own("config", "macro")...)
		if len(found) != 0 {
			t.Errorf("the empty list came back as %+v, wanted an empty list", found)
		}
	})

	mustRun(t, own("config", "macro", "new", "Night watch",
		"volume set 4", "backlight on", "scan")...)

	t.Run("it comes back as it went in", func(t *testing.T) {
		var found []macro
		mustJSON(t, &found, own("config", "macro")...)

		if len(found) != 1 {
			t.Fatalf("the list holds %d macros, wanted 1: %+v", len(found), found)
		}
		if found[0].Name != "Night watch" {
			t.Errorf("the macro is called %q, wanted %q", found[0].Name, "Night watch")
		}
		want := []string{"volume set 4", "backlight on", "scan"}
		if !sameLines(found[0].Steps, want) {
			t.Errorf("the steps came back as %q, wanted %q in that order", found[0].Steps, want)
		}
		if found[0].KeepGoing {
			t.Error("the macro keeps going after a failure and was not asked to")
		}
	})

	t.Run("the config file holds it", func(t *testing.T) {
		// The file is what a front end reads on the next run, so a macro
		// that only existed in the answer would be a macro that vanished.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the config back: %v", err)
		}
		var stored struct {
			Macros []macro `json:"macros"`
		}
		if err := json.Unmarshal(data, &stored); err != nil {
			t.Fatalf("the config file is not valid JSON: %v\n%s", err, data)
		}
		if len(stored.Macros) != 1 || stored.Macros[0].Name != "Night watch" {
			t.Errorf("the file holds %+v, wanted the macro that was just created:\n%s", stored.Macros, data)
		}
	})

	t.Run("a new macro goes on top", func(t *testing.T) {
		mustRun(t, own("config", "macro", "new", "Later", "battery")...)

		var found []macro
		mustJSON(t, &found, own("config", "macro")...)
		if len(found) != 2 || found[0].Name != "Later" {
			t.Errorf("the list is %+v, wanted the newest first", found)
		}

		mustRun(t, own("config", "macro", "delete", "Later", "--yes")...)
	})

	t.Run("show prints the steps and nothing else", func(t *testing.T) {
		// Matched without case, because a macro is picked by pressing a button
		// far more often than by typing its name.
		res := mustRun(t, own("config", "macro", "show", "NIGHT WATCH")...)

		got := strings.Split(strings.TrimSpace(res.stdout), "\n")
		want := []string{"volume set 4", "backlight on", "scan"}
		if !sameLines(got, want) {
			t.Errorf("show printed %q, wanted %q", got, want)
		}
	})

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, own("config", "macro")...)

		for _, heading := range []string{"NAME", "STEPS", "ON FAILURE"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
	})

	t.Run("set replaces every step", func(t *testing.T) {
		mustRun(t, own("config", "macro", "set", "Night watch", "volume set 2", "scan")...)

		var found []macro
		mustJSON(t, &found, own("config", "macro")...)
		if want := []string{"volume set 2", "scan"}; !sameLines(found[0].Steps, want) {
			t.Errorf("the steps are %q after being set, wanted %q", found[0].Steps, want)
		}
		if found[0].Name != "Night watch" {
			t.Errorf("setting the steps renamed the macro to %q", found[0].Name)
		}
	})

	t.Run("the keep going flag survives the file", func(t *testing.T) {
		mustRun(t, own("config", "macro", "set", "Night watch", "battery", "--keep-going")...)

		var on []macro
		mustJSON(t, &on, own("config", "macro")...)
		if !on[0].KeepGoing {
			t.Error("the macro was set to keep going and did not come back that way")
		}

		// Left off again, it goes back off: the flag says what the macro does
		// every time rather than only when it is turned on.
		mustRun(t, own("config", "macro", "set", "Night watch", "battery")...)

		// A variable of its own rather than the one above. Decoding into a
		// slice that already holds something fills in the fields the answer
		// names and leaves the rest as they were, so a key the tool has stopped
		// writing would read as still set.
		var off []macro
		mustJSON(t, &off, own("config", "macro")...)
		if off[0].KeepGoing {
			t.Error("the macro still keeps going after being set without the flag")
		}
	})

	t.Run("renaming a macro", func(t *testing.T) {
		mustRun(t, own("config", "macro", "rename", "Night watch", "Nightwatch")...)

		var found []macro
		mustJSON(t, &found, own("config", "macro")...)
		if found[0].Name != "Nightwatch" {
			t.Errorf("the macro is called %q after being renamed", found[0].Name)
		}
	})

	t.Run("deleting a macro", func(t *testing.T) {
		mustRun(t, own("config", "macro", "delete", "Nightwatch", "--yes")...)

		var found []macro
		mustJSON(t, &found, own("config", "macro")...)
		if len(found) != 0 {
			t.Errorf("the list holds %+v after the only macro was deleted", found)
		}
	})
}

// TestConfigMacro_Refusals covers what the tool says no to. Each of these would
// otherwise reach the config file and fail at run time instead, partway through
// a macro with the steps before it already run.
func TestConfigMacro_Refusals(t *testing.T) {
	path := noMacros(t)
	own := func(args ...string) []string {
		return append([]string{"--config", path}, args...)
	}

	mustRun(t, own("config", "macro", "new", "Night watch", "battery")...)
	mustRun(t, own("config", "macro", "new", "Quick look", "screen")...)

	cases := []struct {
		name string
		want string
		args []string
	}{
		{
			"a name already taken, whatever its case",
			"already a macro",
			own("config", "macro", "new", "NIGHT WATCH", "battery"),
		},
		{
			"a step that could never be split",
			`unclosed " quote`,
			own("config", "macro", "new", "Bad", `favorites scan "GREENDALE`),
		},
		{
			"a step with a shell operator",
			"is not supported",
			own("config", "macro", "new", "Bad", "battery | grep charge"),
		},
		{
			"a name longer than a button",
			"longer than 40 characters",
			own("config", "macro", "new", strings.Repeat("x", 41), "battery"),
		},
		{
			"a macro with no steps",
			"requires at least 2 arg",
			own("config", "macro", "new", "Bare"),
		},
		{
			"setting a macro that does not exist",
			"no macro called",
			own("config", "macro", "set", "Nope", "battery"),
		},
		{
			"showing a macro that does not exist",
			"no macro called",
			own("config", "macro", "show", "Nope"),
		},
		{
			// A different macro, matched without case exactly as every other
			// lookup is. Renaming one to its own name in another capitalization
			// is a rename somebody may well mean, and is allowed.
			"renaming onto another macro",
			"already a macro",
			own("config", "macro", "rename", "Night watch", "QUICK LOOK"),
		},
		{
			"deleting without --yes",
			"--yes",
			own("config", "macro", "delete", "Night watch"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustFail(t, c.want, c.args...)
		})
	}

	t.Run("nothing was written by any of them", func(t *testing.T) {
		var found []macro
		mustJSON(t, &found, own("config", "macro")...)

		// Newest first, because a new macro goes on the top.
		if len(found) != 2 || found[0].Name != "Quick look" || found[1].Name != "Night watch" {
			t.Errorf("the file holds %+v after a run of refusals, wanted the two macros", found)
		}
		if want := []string{"battery"}; !sameLines(found[1].Steps, want) {
			t.Errorf("the steps of %q are %q, wanted %q", found[1].Name, found[1].Steps, want)
		}
	})
}

// TestConfigMacro_Defaults covers the buttons somebody gets before they have made any.
//
// Checked through the built binary, against a config file that names no macros,
// which is what a machine that has never been configured has.
func TestConfigMacro_Defaults(t *testing.T) {
	path := scratchConfig(t)
	own := func(args ...string) []string {
		return append([]string{"--config", path}, args...)
	}

	var found []macro
	mustJSON(t, &found, own("config", "macro")...)

	want := []struct{ name, step string }{
		{"Resume Scanning", "scan"},
		{"Color Mode", "display mode color"},
		{"Dark Mode", "display mode black"},
		{"Light Mode", "display mode white"},
		{"Toggle Backlight", "backlight keys toggle"},
		{"Mute Speaker", "volume set 0"},
		{"Toggle Key Beep", "beep toggle"},
		{"Monitor Weather", "weather"},
		{"Tune to 107.9 FM", "tune 107.9"},
		{"Sync Clock", "clock sync"},
		{"Sync Colors", "colors --all"},
		{"Reset Colors", "colors reset"},
	}

	if len(found) != len(want) {
		t.Fatalf("a fresh config holds %d macros, wanted %d: %+v", len(found), len(want), found)
	}
	for i, w := range want {
		if found[i].Name != w.name {
			t.Errorf("macro %d is called %q, wanted %q", i, found[i].Name, w.name)
		}
		if !sameLines(found[i].Steps, []string{w.step}) {
			t.Errorf("%q runs %q, wanted %q", found[i].Name, found[i].Steps, w.step)
		}
	}

	t.Run("reading them writes nothing", func(t *testing.T) {
		// The defaults are what the tool falls back to rather than something it
		// installs, so a machine that only ever reads them never gets a config
		// file it did not ask for.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the config back: %v", err)
		}
		if strings.Contains(string(data), "macros") {
			t.Errorf("listing the macros wrote them to the file:\n%s", data)
		}
	})

	t.Run("every default is a command the tool has", func(t *testing.T) {
		// A default naming a command that does not exist would be a button that
		// fails the first time anybody presses it. --help answers without a
		// scanner, so this checks the command exists rather than running it.
		for _, w := range want {
			args := append(strings.Fields(w.step), "--help")
			mustRun(t, args...)
		}
	})

	t.Run("deleting one is remembered", func(t *testing.T) {
		mustRun(t, own("config", "macro", "delete", "Light Mode", "--yes")...)

		var left []macro
		mustJSON(t, &left, own("config", "macro")...)
		if len(left) != len(want)-1 {
			t.Fatalf("the list holds %d macros after one was deleted, wanted %d: %+v",
				len(left), len(want)-1, left)
		}
		for _, m := range left {
			if m.Name == "Light Mode" {
				t.Error("the deleted default came back")
			}
		}
	})

	t.Run("deleting them all is remembered", func(t *testing.T) {
		for _, w := range want {
			if w.name == "Light Mode" {
				continue
			}
			mustRun(t, own("config", "macro", "delete", w.name, "--yes")...)
		}

		var left []macro
		mustJSON(t, &left, own("config", "macro")...)
		if len(left) != 0 {
			t.Errorf("the defaults came back after every one was deleted: %+v", left)
		}
	})
}

// TestConfigMacro_IsNotASetting checks the answer somebody gets for guessing that a
// macro is a setting like the others. "no setting called macros" is true and
// unhelpful; where to look instead is what they are after.
func TestConfigMacro_IsNotASetting(t *testing.T) {
	for _, name := range []string{"macro", "macros"} {
		res, err := execute("config", "get", name)
		if err != nil {
			t.Fatalf("running config get %s: %v", name, err)
		}
		if res.code == 0 {
			t.Fatalf("config get %s succeeded, wanted a refusal", name)
		}
		if !strings.Contains(res.stderr, "radiocli config macro") {
			t.Errorf("config get %s said %q, which does not point at the command", name, res.stderr)
		}
	}
}

// TestConfigMacro_LeavesOthersAlone pins the thing a list in the config file
// most easily breaks. Writing a macro rewrites the whole file, so a macro that
// arrived by losing every other setting would still look like it worked.
func TestConfigMacro_LeavesOthersAlone(t *testing.T) {
	path := noMacros(t)
	own := func(args ...string) []string {
		return append([]string{"--config", path}, args...)
	}

	mustRun(t, own("config", "set", "output", "text")...)
	mustRun(t, own("config", "set", "pace", "medium")...)
	mustRun(t, own("config", "macro", "new", "Night watch", "battery", "screen")...)

	for _, want := range []struct{ name, value string }{
		{"output", "text"},
		{"pace", "medium"},
	} {
		res := mustRun(t, own("config", "get", want.name)...)
		if got := strings.TrimSpace(res.stdout); got != want.value {
			t.Errorf("%s is %q after a macro was written, wanted %q", want.name, got, want.value)
		}
	}
}

// TestConfigMacroMove covers rearranging the list, which is the order the buttons
// appear in.
func TestConfigMacroMove(t *testing.T) {
	path := noMacros(t)
	own := func(args ...string) []string {
		return append([]string{"--config", path}, args...)
	}

	// Backwards, because a new macro goes on the top. Creating them in the
	// order this test is about would leave them in the reverse of it.
	for _, name := range []string{"Three", "Two", "One"} {
		mustRun(t, own("config", "macro", "new", name, "battery")...)
	}

	names := func() []string {
		t.Helper()

		var found []macro
		mustJSON(t, &found, own("config", "macro")...)

		var got []string
		for _, m := range found {
			got = append(got, m.Name)
		}
		return got
	}

	mustRun(t, own("config", "macro", "move", "Three", "up")...)
	if got := names(); !sameLines(got, []string{"One", "Three", "Two"}) {
		t.Fatalf("the order is %q after moving Three up", got)
	}

	// Matched without case, like every other way of naming a macro.
	mustRun(t, own("config", "macro", "move", "one", "down")...)
	if got := names(); !sameLines(got, []string{"Three", "One", "Two"}) {
		t.Fatalf("the order is %q after moving One down", got)
	}

	t.Run("the whole list is printed", func(t *testing.T) {
		// Where one macro ended up is only answerable by the order they are all
		// in, so a move answers with the list rather than with the one that
		// moved.
		res := mustRun(t, own("config", "macro", "move", "Two", "up")...)

		for _, name := range []string{"Three", "One", "Two"} {
			if !strings.Contains(res.stdout, name) {
				t.Errorf("the answer does not mention %q:\n%s", name, res.stdout)
			}
		}
	})

	t.Run("refusals leave the order alone", func(t *testing.T) {
		before := names()

		mustFail(t, "already first", own("config", "macro", "move", before[0], "up")...)
		mustFail(t, "already last", own("config", "macro", "move", before[len(before)-1], "down")...)
		mustFail(t, "not a direction", own("config", "macro", "move", before[0], "sideways")...)
		mustFail(t, "no macro called", own("config", "macro", "move", "Nope", "up")...)

		if got := names(); !sameLines(got, before) {
			t.Errorf("the order is %q after refused moves, wanted %q", got, before)
		}
	})
}

// sameLines reports whether two lists hold the same lines in the same order.
func sameLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
