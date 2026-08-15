// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// The tests here run the macro command against a config file of this test's
// own. They exist because a macro is the one thing in the config file that is
// not a single value: everything else is a string the file either holds or does
// not, while a macro is a list inside a list, and every way of getting that
// wrong loses somebody's buttons rather than one setting.

// isolate returns an App writing to a config file in a directory of this test's
// own, so a run never reads or writes the config of the person running it, and
// the buffers its output went to.
//
// The file names an empty list of macros rather than none at all, so these
// tests start with nothing rather than with the built-in four. What the
// built-in ones do is a question of its own, in TestMacroDefaults.
func isolate(t *testing.T) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	return isolateWith(t, `{"macros": []}`)
}

// isolateWith is isolate with the config file written out in full, for the
// tests that care what is in it before they start.
func isolateWith(t *testing.T, contents string) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents+"\n"), 0o600); err != nil {
		t.Fatalf("seeding the config file: %v", err)
	}

	cfg := appcontext.Defaults()
	cfg.Path = path
	if err := cfg.Load(); err != nil {
		t.Fatalf("loading the seeded config file: %v", err)
	}

	var out, errs bytes.Buffer
	return &appcontext.App{
		Config: cfg,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stdout: &out,
		Stderr: &errs,
	}, &out, &errs
}

// saved reads the config file back, which is the only thing that says a write
// actually happened.
func saved(t *testing.T, app *appcontext.App) *appcontext.Config {
	t.Helper()

	cfg, err := app.Config.Saved()
	if err != nil {
		t.Fatalf("reading the config file back: %v", err)
	}
	return cfg
}

// TestMacroRoundTrip covers a macro surviving a write and a read, and every
// other setting surviving with it. Config.Update rewrites the whole file, so a
// macro that arrived by wiping somebody's settings would still look like it
// worked from this command's own output.
func TestMacroRoundTrip(t *testing.T) {
	app, _, _ := isolate(t)

	if err := app.Config.Update(func(c *appcontext.Config) {
		c.Verbose = true
	}); err != nil {
		t.Fatalf("setting verbose: %v", err)
	}

	if err := runMacroNew(app, "Night watch", []string{"volume set 4", "backlight on", "scan"}, false); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}

	cfg := saved(t, app)
	if len(cfg.Macros) != 1 {
		t.Fatalf("the file holds %d macros, want 1", len(cfg.Macros))
	}

	m := cfg.Macros[0]
	if m.Name != "Night watch" {
		t.Errorf("the macro is called %q, want %q", m.Name, "Night watch")
	}
	if want := []string{"volume set 4", "backlight on", "scan"}; !sameSteps(m.Steps, want) {
		t.Errorf("the steps came back as %q, want %q in that order", m.Steps, want)
	}
	if m.KeepGoing {
		t.Error("the macro keeps going after a failure, and was not asked to")
	}
	if !cfg.Verbose {
		t.Error("verbose is now false: writing a macro lost an unrelated setting")
	}
}

// TestMacroDefaults covers the buttons somebody gets before they have made any.
//
// Each is checked by name and by the command it runs, because a front end
// puts them in front of somebody who has never used a macro: one that names a
// command the tool does not have would be a broken button on first use.
func TestMacroDefaults(t *testing.T) {
	// A config file naming no macros at all, which is what a machine that has
	// never been configured has.
	app, _, _ := isolateWith(t, "{}")

	want := []appcontext.Macro{
		{Name: "Resume Scanning", Steps: []string{"scan"}},
		{Name: "Color Mode", Steps: []string{"display mode color"}},
		{Name: "Dark Mode", Steps: []string{"display mode black"}},
		{Name: "Light Mode", Steps: []string{"display mode white"}},
		{Name: "Toggle Backlight", Steps: []string{"backlight keys toggle"}},
		{Name: "Mute Speaker", Steps: []string{"volume set 0"}},
		{Name: "Toggle Key Beep", Steps: []string{"beep toggle"}},
		{Name: "Monitor Weather", Steps: []string{"weather"}},
		{Name: "Tune to 107.9 FM", Steps: []string{"tune 107.9"}},
		{Name: "Sync Clock", Steps: []string{"clock sync"}},
		{Name: "Sync Colors", Steps: []string{"colors --all"}},
		{Name: "Reset Colors", Steps: []string{"colors reset"}},
	}

	got := saved(t, app).Macros
	if len(got) != len(want) {
		t.Fatalf("a fresh config holds %d macros, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Name != want[i].Name {
			t.Errorf("macro %d is called %q, want %q", i, got[i].Name, want[i].Name)
		}
		if !sameSteps(got[i].Steps, want[i].Steps) {
			t.Errorf("%q runs %q, want %q", got[i].Name, got[i].Steps, want[i].Steps)
		}
	}

	// Every default has to survive the checks a typed one goes through, or the
	// page would ship a button that this command would refuse to store.
	for _, m := range got {
		if _, err := makeMacro(m.Name, m.Steps, m.KeepGoing); err != nil {
			t.Errorf("the built-in macro %q would be refused if it were typed: %v", m.Name, err)
		}
	}
}

// TestMacroDefaultsCanBeDeletedForGood is the test the whole shape of the
// Macros field exists for. Deleting a default has to stick, and a list that
// emptied itself out of the file would put all four back on the next read.
func TestMacroDefaultsCanBeDeletedForGood(t *testing.T) {
	app, _, _ := isolateWith(t, "{}")

	for _, m := range appcontext.Defaults().Macros {
		if err := runMacroDelete(app, m.Name, true); err != nil {
			t.Fatalf("deleting %q: %v", m.Name, err)
		}
	}

	if got := saved(t, app).Macros; len(got) != 0 {
		t.Fatalf("the defaults came back after being deleted: %+v", got)
	}

	// And again, from a Config that has never seen this file, which is what the
	// next invocation of the tool is.
	fresh := appcontext.Defaults()
	fresh.Path = app.Config.Path
	if err := fresh.Load(); err != nil {
		t.Fatalf("reading the config file: %v", err)
	}
	if len(fresh.Macros) != 0 {
		t.Errorf("a later run got the defaults back: %+v", fresh.Macros)
	}
}

// TestMacroDefaultsGiveWayToTheFile checks that macros in the file replace the
// built-in ones rather than merging with them. Decoding a list onto a list that
// already holds something fills in what the file names and leaves the rest, so
// without care a stored macro would take fields from whichever default sat at
// its index.
func TestMacroDefaultsGiveWayToTheFile(t *testing.T) {
	app, _, _ := isolateWith(t, `{"macros":[{"name":"Only","steps":["battery"]}]}`)

	got := saved(t, app).Macros
	if len(got) != 1 {
		t.Fatalf("the file names one macro and %d came back: %+v", len(got), got)
	}
	if got[0].Name != "Only" {
		t.Errorf("the macro is called %q, want %q", got[0].Name, "Only")
	}
	if want := []string{"battery"}; !sameSteps(got[0].Steps, want) {
		t.Errorf("it runs %q, want %q", got[0].Steps, want)
	}
}

// TestMacroNewGoesOnTop pins where a new macro lands.
//
// The end of the list is the far side of everything made before it, which in
// a front end means scrolling to reach the one just written. It goes above the
// built-in macros too: they are what somebody gets before they have made
// anything, not a header the ones they make belong under.
func TestMacroNewGoesOnTop(t *testing.T) {
	app, _, _ := isolateWith(t, "{}")

	before := order(t, app)
	if len(before) == 0 {
		t.Fatal("a fresh config holds no macros to be put in front of")
	}

	if err := runMacroNew(app, "Night watch", []string{"battery"}, false); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}
	if err := runMacroNew(app, "Quick look", []string{"screen"}, false); err != nil {
		t.Fatalf("creating a second macro: %v", err)
	}

	got := order(t, app)
	if want := append([]string{"Quick look", "Night watch"}, before...); !sameSteps(got, want) {
		t.Errorf("the order is %q, want %q", got, want)
	}
}

// TestMacroKeepsItsOrder pins that the steps are a list rather than a set. A
// macro whose steps ran in another order would walk the scanner somewhere
// nobody asked for, and the failure would look like the radio's.
func TestMacroKeepsItsOrder(t *testing.T) {
	app, _, _ := isolate(t)

	want := []string{"menu open Weather", "key down", "key select", "scan"}
	if err := runMacroNew(app, "Weather", want, false); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}

	if got := saved(t, app).Macros[0].Steps; !sameSteps(got, want) {
		t.Errorf("the steps came back as %q, want %q", got, want)
	}
}

// TestMacroRefusals covers every way a macro can be refused. Each of these
// would otherwise reach the file and fail later, mid-run, partway through a
// run with the earlier steps already done.
func TestMacroRefusals(t *testing.T) {
	cases := []struct {
		name  string
		macro string
		steps []string
		want  string
	}{
		{"no name", "", []string{"battery"}, "needs a name"},
		{"a name of spaces", "   ", []string{"battery"}, "needs a name"},
		{"a name with a line break", "one\ntwo", []string{"battery"}, "tab or a line break"},
		{"a name too long for a button", strings.Repeat("x", maxMacroName+1), []string{"battery"}, "longer than 40 characters"},
		{"no steps", "Empty", nil, "at least one step"},
		{"an empty step", "Empty step", []string{"battery", "  "}, "step 2 is empty"},
		{"an unclosed quote", "Bad", []string{`favorites scan "GREENDALE`}, `unclosed " quote`},
		{"a shell operator", "Bad", []string{"battery | grep charge"}, `"|" is not supported`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, _, _ := isolate(t)

			err := runMacroNew(app, c.macro, c.steps, false)
			if err == nil {
				t.Fatal("the macro was accepted, want a refusal")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the refusal said %q, which does not mention %q", err, c.want)
			}
			if got := saved(t, app).Macros; len(got) != 0 {
				t.Errorf("the file holds %d macros after a refusal, want none", len(got))
			}
		})
	}
}

// TestMacroNamesAreUniqueWhateverTheCase covers the check that stops one button
// from quietly replacing another. Names match case-insensitively everywhere, so
// two that differ only in case would be one macro with two rows.
func TestMacroNamesAreUniqueWhateverTheCase(t *testing.T) {
	app, _, _ := isolate(t)

	if err := runMacroNew(app, "Night watch", []string{"battery"}, false); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}

	err := runMacroNew(app, "NIGHT WATCH", []string{"screen"}, false)
	if err == nil {
		t.Fatal("a second macro differing only in case was accepted")
	}
	if !strings.Contains(err.Error(), "already a macro") {
		t.Errorf("the refusal said %q, which does not say the name is taken", err)
	}
	if got := saved(t, app).Macros; len(got) != 1 {
		t.Errorf("the file holds %d macros, want 1", len(got))
	}
}

// TestMacroSetKeepsTheStoredName checks that setting the steps of "night watch"
// does not recapitalize the button. The name is what somebody sees; the typed
// one is only how they reached it.
func TestMacroSetKeepsTheStoredName(t *testing.T) {
	app, _, _ := isolate(t)

	if err := runMacroNew(app, "Night watch", []string{"battery"}, false); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}
	if err := runMacroSet(app, "NIGHT WATCH", []string{"screen", "scan"}, true); err != nil {
		t.Fatalf("setting the steps: %v", err)
	}

	m := saved(t, app).Macros[0]
	if m.Name != "Night watch" {
		t.Errorf("the macro is now called %q, want %q", m.Name, "Night watch")
	}
	if want := []string{"screen", "scan"}; !sameSteps(m.Steps, want) {
		t.Errorf("the steps came back as %q, want %q", m.Steps, want)
	}
	if !m.KeepGoing {
		t.Error("the macro was set to keep going after a failure and did not")
	}
}

// TestMacroSetRefusesAnUnknownName pins that set changes a macro rather than
// creating one. Creating here would make a typo a second button rather than an
// error, and the first would look like it had stopped taking changes.
func TestMacroSetRefusesAnUnknownName(t *testing.T) {
	app, _, _ := isolate(t)

	err := runMacroSet(app, "Nope", []string{"battery"}, false)
	if err == nil {
		t.Fatal("setting the steps of a macro that does not exist was accepted")
	}
	if !strings.Contains(err.Error(), "no macro called") {
		t.Errorf("the refusal said %q, which does not say the macro is unknown", err)
	}
}

// TestMacroRenameToItsOwnCase checks that changing only the capitalization is
// allowed. The name resolves back to this macro, so a check that only asked
// whether the name was taken would refuse a rename somebody plainly meant.
func TestMacroRenameToItsOwnCase(t *testing.T) {
	app, _, _ := isolate(t)

	if err := runMacroNew(app, "Night watch", []string{"battery"}, false); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}
	if err := runMacroRename(app, "Night watch", "Night Watch"); err != nil {
		t.Fatalf("recapitalizing the name: %v", err)
	}

	if got := saved(t, app).Macros[0].Name; got != "Night Watch" {
		t.Errorf("the macro is called %q, want %q", got, "Night Watch")
	}
}

// TestMacroRenameOntoAnother pins that a rename cannot swallow a different
// macro. Both names are real, and the answer has to be a refusal rather than a
// choice about which one survives.
func TestMacroRenameOntoAnother(t *testing.T) {
	app, _, _ := isolate(t)

	for _, name := range []string{"One", "Two"} {
		if err := runMacroNew(app, name, []string{"battery"}, false); err != nil {
			t.Fatalf("creating %q: %v", name, err)
		}
	}

	if err := runMacroRename(app, "One", "two"); err == nil {
		t.Fatal("renaming a macro onto another was accepted")
	}
	if got := saved(t, app).Macros; len(got) != 2 {
		t.Fatalf("the file holds %d macros, want 2", len(got))
	}
}

// TestMacroMove covers rearranging the list, which is the order the buttons
// appear in and the only thing about a macro that is not inside it.
func TestMacroMove(t *testing.T) {
	app, _, _ := isolate(t)
	given(t, app, "One", "Two", "Three")

	if err := runMacroMove(app, "Three", "up"); err != nil {
		t.Fatalf("moving a macro up: %v", err)
	}
	if got := order(t, app); !sameSteps(got, []string{"One", "Three", "Two"}) {
		t.Fatalf("the order is %q after moving Three up", got)
	}

	if err := runMacroMove(app, "one", "down"); err != nil {
		t.Fatalf("moving a macro down: %v", err)
	}
	if got := order(t, app); !sameSteps(got, []string{"Three", "One", "Two"}) {
		t.Fatalf("the order is %q after moving One down", got)
	}

	// A macro is moved by name, so the steps of the one that moved have to move
	// with it rather than the names sliding along a fixed row of steps.
	if err := runMacroSet(app, "One", []string{"screen", "scan"}, false); err != nil {
		t.Fatalf("setting the steps: %v", err)
	}
	if err := runMacroMove(app, "One", "up"); err != nil {
		t.Fatalf("moving a macro up: %v", err)
	}

	moved := saved(t, app).Macros[0]
	if moved.Name != "One" {
		t.Fatalf("the macro at the top is %q, want One", moved.Name)
	}
	if want := []string{"screen", "scan"}; !sameSteps(moved.Steps, want) {
		t.Errorf("it runs %q after moving, want %q", moved.Steps, want)
	}
}

// TestMacroMoveRefusals covers the ends of the list and a direction that is not
// one. A move that cannot happen is said rather than passed over, because a
// press that did nothing and reported nothing reads as a broken button.
func TestMacroMoveRefusals(t *testing.T) {
	app, _, _ := isolate(t)
	given(t, app, "One", "Two")

	cases := []struct {
		name      string
		macro     string
		direction string
		want      string
	}{
		{"up from the top", "One", "up", "already first"},
		{"down from the bottom", "Two", "down", "already last"},
		{"sideways", "One", "sideways", "not a direction"},
		{"a macro that does not exist", "Nope", "up", "no macro called"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := runMacroMove(app, c.macro, c.direction)
			if err == nil {
				t.Fatal("the move was accepted, want a refusal")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the refusal said %q, which does not mention %q", err, c.want)
			}
			if got := order(t, app); !sameSteps(got, []string{"One", "Two"}) {
				t.Errorf("the order is %q after a refused move, want it unchanged", got)
			}
		})
	}
}

// order reads the macro names back from the file, in the order they are stored.
func order(t *testing.T, app *appcontext.App) []string {
	t.Helper()

	var names []string
	for _, m := range saved(t, app).Macros {
		names = append(names, m.Name)
	}
	return names
}

// given creates macros so the list ends up in the order named.
//
// Backwards, because a new macro goes on the top: creating them in the order
// they are wanted would leave them in the reverse of it. Worth a helper rather
// than a reversed literal in each test, so a test about moving macros reads as
// the order it is about.
func given(t *testing.T, app *appcontext.App, names ...string) {
	t.Helper()

	for i := len(names) - 1; i >= 0; i-- {
		if err := runMacroNew(app, names[i], []string{"battery"}, false); err != nil {
			t.Fatalf("creating %q: %v", names[i], err)
		}
	}

	if got := order(t, app); !sameSteps(got, names) {
		t.Fatalf("the list starts as %q, want %q", got, names)
	}
}

// TestMacroDeleteNeedsYes covers the refusal and that it changes nothing, which
// is the point of asking.
func TestMacroDeleteNeedsYes(t *testing.T) {
	app, _, _ := isolate(t)

	if err := runMacroNew(app, "Night watch", []string{"battery", "screen"}, false); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}

	err := runMacroDelete(app, "night watch", false)
	if err == nil {
		t.Fatal("the macro was deleted without --yes")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("the refusal said %q, which does not name the flag", err)
	}
	// Named as it is stored and counted, so somebody can tell from the refusal
	// alone whether it is the macro they meant.
	if !strings.Contains(err.Error(), `"Night watch"`) || !strings.Contains(err.Error(), "2 steps") {
		t.Errorf("the refusal said %q, which does not name the macro and its steps", err)
	}
	if got := saved(t, app).Macros; len(got) != 1 {
		t.Errorf("the file holds %d macros after a refusal, want 1", len(got))
	}
}

// TestMacroDeleteLeavesTheRest checks that deleting the middle of three removes
// one row rather than resizing the list. Rebuilding a slice in place is easy to
// get wrong by one either way.
func TestMacroDeleteLeavesTheRest(t *testing.T) {
	app, _, _ := isolate(t)
	given(t, app, "One", "Two", "Three")

	if err := runMacroDelete(app, "Two", true); err != nil {
		t.Fatalf("deleting a macro: %v", err)
	}

	cfg := saved(t, app)
	if len(cfg.Macros) != 2 {
		t.Fatalf("the file holds %d macros, want 2", len(cfg.Macros))
	}
	if cfg.Macros[0].Name != "One" || cfg.Macros[1].Name != "Three" {
		t.Errorf("the macros left are %q and %q, want %q and %q",
			cfg.Macros[0].Name, cfg.Macros[1].Name, "One", "Three")
	}
}

// TestMacroListAsJSON pins the shape a front end reads. It builds its controls
// from this and nothing else, so a null where a list belongs, or a renamed
// field, is a panel with nothing on it.
func TestMacroListAsJSON(t *testing.T) {
	app, out, _ := isolate(t)
	app.Config.Output = appcontext.OutputJSON

	// Empty first: a page asking before anything is saved has to get a list it
	// can walk rather than null.
	if err := runMacroList(app); err != nil {
		t.Fatalf("listing nothing: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Errorf("an empty list rendered as %q, want %q", got, "[]")
	}

	if err := runMacroNew(app, "Night watch", []string{"volume set 4", "scan"}, true); err != nil {
		t.Fatalf("creating a macro: %v", err)
	}

	out.Reset()
	if err := runMacroList(app); err != nil {
		t.Fatalf("listing: %v", err)
	}

	var got []appcontext.Macro
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("the list is not JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 {
		t.Fatalf("the list holds %d macros, want 1", len(got))
	}
	if got[0].Name != "Night watch" || !got[0].KeepGoing {
		t.Errorf("the macro came back as %+v, want its name and keepGoing", got[0])
	}
	if want := []string{"volume set 4", "scan"}; !sameSteps(got[0].Steps, want) {
		t.Errorf("the steps came back as %q, want %q", got[0].Steps, want)
	}
}

// TestMacroIsNotASetting checks the answer somebody gets for guessing that a
// macro is a setting like the others. "no setting called macros" is true and
// unhelpful; where to actually look is the answer they are after.
func TestMacroIsNotASetting(t *testing.T) {
	for _, name := range []string{"macro", "macros"} {
		_, err := lookup(name)
		if err == nil {
			t.Fatalf("%q was accepted as a setting", name)
		}
		if !strings.Contains(err.Error(), "radiocli config macro") {
			t.Errorf("looking up %q said %q, which does not point at the command", name, err)
		}
	}
}

// sameSteps reports whether two step lists hold the same lines in the same
// order.
func sameSteps(got, want []string) bool {
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

// Test_newMacro tests the newMacro function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Reports: the bare command lists the macros
//   - Subcommands: every verb is attached
func Test_newMacro(t *testing.T) {
	// Verify that the bare command reports rather than runs anything, which is
	// the whole shape of this command.
	t.Run("Reports", func(t *testing.T) {
		app, out, _ := isolate(t)
		given(t, app, "Night watch")

		out.Reset()
		if err := execute(t, app, "macro"); err != nil {
			t.Fatalf("config macro: %v", err)
		}
		if got := out.String(); !strings.Contains(got, "Night watch") {
			t.Errorf("the listing is %q, which does not name the macro", got)
		}
	})

	// Verify that every verb the command documents is actually attached, since
	// a missing one is only found by typing it.
	t.Run("Subcommands", func(t *testing.T) {
		app, _, _ := isolate(t)

		want := map[string]bool{"show": false, "new": false, "set": false, "rename": false, "move": false, "delete": false}
		for _, cmd := range New(app).Commands() {
			if cmd.Name() != "macro" {
				continue
			}
			for _, sub := range cmd.Commands() {
				if _, ok := want[sub.Name()]; ok {
					want[sub.Name()] = true
				}
			}
		}
		for name, found := range want {
			if !found {
				t.Errorf("the command %q is not attached", name)
			}
		}
	})
}

// Test_newMacroDelete tests the newMacroDelete function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Deletes: with --yes the macro goes
//   - NeedsYes: without it nothing is touched
func Test_newMacroDelete(t *testing.T) {
	// Verify that the flag the command documents is the one that allows the
	// deletion.
	t.Run("Deletes", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "Night watch")

		if err := execute(t, app, "macro", "delete", "Night watch", "--yes"); err != nil {
			t.Fatalf("config macro delete: %v", err)
		}
		if got := saved(t, app).Macros; len(got) != 0 {
			t.Errorf("the file holds %d macros, want none", len(got))
		}
	})

	// Verify that the deletion is refused without the flag, since there is
	// nothing to undo it with.
	t.Run("NeedsYes", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "Night watch")

		if err := execute(t, app, "macro", "delete", "Night watch"); err == nil {
			t.Fatal("the macro was deleted without --yes")
		}
		if got := saved(t, app).Macros; len(got) != 1 {
			t.Errorf("the file holds %d macros after a refusal, want 1", len(got))
		}
	})
}

// Test_newMacroMove tests the newMacroMove function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Moves: the macro changes places and the whole list is printed
func Test_newMacroMove(t *testing.T) {
	// Verify that the move happens and the whole list comes back, because
	// where one macro ended up is only answerable by the order they are in.
	t.Run("Moves", func(t *testing.T) {
		app, out, _ := isolate(t)
		given(t, app, "One", "Two")

		out.Reset()
		if err := execute(t, app, "macro", "move", "Two", "up"); err != nil {
			t.Fatalf("config macro move: %v", err)
		}
		if got := order(t, app); !sameSteps(got, []string{"Two", "One"}) {
			t.Errorf("the order is %q, want Two first", got)
		}
		if got := out.String(); !strings.Contains(got, "One") || !strings.Contains(got, "Two") {
			t.Errorf("the answer is %q, which is not the whole list", got)
		}
	})
}

// Test_newMacroNew tests the newMacroNew function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Creates: the macro is written, with the flag the command offers
func Test_newMacroNew(t *testing.T) {
	// Verify that each argument after the name is one step, and that
	// --keep-going reaches the macro.
	t.Run("Creates", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := execute(t, app, "macro", "new", "Night watch", "volume set 4", "scan", "--keep-going"); err != nil {
			t.Fatalf("config macro new: %v", err)
		}

		m := saved(t, app).Macros[0]
		if m.Name != "Night watch" {
			t.Errorf("the macro is called %q, want %q", m.Name, "Night watch")
		}
		if want := []string{"volume set 4", "scan"}; !sameSteps(m.Steps, want) {
			t.Errorf("the steps came back as %q, want %q", m.Steps, want)
		}
		if !m.KeepGoing {
			t.Error("--keep-going did not reach the macro")
		}
	})
}

// Test_newMacroRename tests the newMacroRename function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Renames: the macro is called something else and keeps its steps
func Test_newMacroRename(t *testing.T) {
	// Verify that only the name changes, since the steps are what the button
	// does.
	t.Run("Renames", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One")

		if err := execute(t, app, "macro", "rename", "One", "Two"); err != nil {
			t.Fatalf("config macro rename: %v", err)
		}

		m := saved(t, app).Macros[0]
		if m.Name != "Two" {
			t.Errorf("the macro is called %q, want %q", m.Name, "Two")
		}
		if want := []string{"battery"}; !sameSteps(m.Steps, want) {
			t.Errorf("the steps came back as %q, want %q", m.Steps, want)
		}
	})
}

// Test_newMacroSet tests the newMacroSet function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Replaces: every step is replaced and the name is left alone
func Test_newMacroSet(t *testing.T) {
	// Verify that the steps are replaced rather than added to, and that the
	// flag is read every time.
	t.Run("Replaces", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "Night watch")

		if err := execute(t, app, "macro", "set", "Night watch", "screen", "scan", "--keep-going"); err != nil {
			t.Fatalf("config macro set: %v", err)
		}

		m := saved(t, app).Macros[0]
		if m.Name != "Night watch" {
			t.Errorf("the macro is called %q, want %q", m.Name, "Night watch")
		}
		if want := []string{"screen", "scan"}; !sameSteps(m.Steps, want) {
			t.Errorf("the steps came back as %q, want %q", m.Steps, want)
		}
		if !m.KeepGoing {
			t.Error("--keep-going did not reach the macro")
		}
	})
}

// Test_newMacroShow tests the newMacroShow function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Shows: the steps alone are printed, one per line
func Test_newMacroShow(t *testing.T) {
	// Verify that the steps come back on their own, so the list can be edited
	// and passed straight back to set.
	t.Run("Shows", func(t *testing.T) {
		app, out, _ := isolate(t)
		if err := runMacroNew(app, "Night watch", []string{"volume set 4", "scan"}, false); err != nil {
			t.Fatalf("creating a macro: %v", err)
		}

		out.Reset()
		if err := execute(t, app, "macro", "show", "NIGHT WATCH"); err != nil {
			t.Fatalf("config macro show: %v", err)
		}
		if got := out.String(); got != "volume set 4\nscan\n" {
			t.Errorf("the answer is %q, want the steps alone", got)
		}
	})
}

// Test_makeMacro tests the makeMacro function with 100% coverage.
//
// Coverage: 94% (5 test cases covering every reachable branch)
//
// The empty split at macro.go:335 is the one branch left out, and it cannot
// fire. The step is trimmed of every space Split treats as one and refused
// when nothing is left, so what reaches Split starts with a character that
// opens an argument: Split either returns at least one argument or refuses the
// line.
//
// Test cases:
//   - Built: a name and steps become a macro
//   - Trimmed: the spaces around a name and a step are dropped
//   - BadName: a name that cannot be used is refused
//   - EmptyStep: a step of nothing but spaces is refused
//   - Unsplittable: a step that could never be split into a command is refused
func Test_makeMacro(t *testing.T) {
	// Verify that what was typed becomes a macro, with the failure setting
	// carried through.
	t.Run("Built", func(t *testing.T) {
		m, err := makeMacro("Night watch", []string{"volume set 4", "scan"}, true)
		if err != nil {
			t.Fatalf("makeMacro: %v", err)
		}
		if m.Name != "Night watch" || !m.KeepGoing {
			t.Errorf("the macro came back as %+v", m)
		}
		if want := []string{"volume set 4", "scan"}; !sameSteps(m.Steps, want) {
			t.Errorf("the steps are %q, want %q", m.Steps, want)
		}
	})

	// Verify that a name and a step are stored without the spaces around them,
	// since a button cannot show them and a command line does not need them.
	t.Run("Trimmed", func(t *testing.T) {
		m, err := makeMacro("  Night watch  ", []string{"  scan  "}, false)
		if err != nil {
			t.Fatalf("makeMacro: %v", err)
		}
		if m.Name != "Night watch" {
			t.Errorf("the macro is called %q, want it trimmed", m.Name)
		}
		if want := []string{"scan"}; !sameSteps(m.Steps, want) {
			t.Errorf("the steps are %q, want them trimmed", m.Steps)
		}
	})

	// Verify that a name that cannot be used is refused before the steps are
	// looked at.
	t.Run("BadName", func(t *testing.T) {
		if _, err := makeMacro("", []string{"scan"}, false); err == nil {
			t.Fatal("a macro with no name was built")
		}
	})

	// Verify that a step of nothing but spaces is refused, and named by its
	// position so it can be found.
	t.Run("EmptyStep", func(t *testing.T) {
		_, err := makeMacro("Night watch", []string{"scan", "   "}, false)
		if err == nil {
			t.Fatal("a step of nothing but spaces was accepted")
		}
		if !strings.Contains(err.Error(), "step 2 is empty") {
			t.Errorf("the refusal said %q, which does not name the step", err)
		}
	})

	// Verify that a step which could never be split into a command is refused
	// now rather than failing halfway through the macro later.
	t.Run("Unsplittable", func(t *testing.T) {
		_, err := makeMacro("Night watch", []string{`favorites scan "GREENDALE`}, false)
		if err == nil {
			t.Fatal("a step with an unclosed quote was accepted")
		}
		if !strings.Contains(err.Error(), "step 1") {
			t.Errorf("the refusal said %q, which does not name the step", err)
		}
	})

	// Verify that a macro with no steps at all is refused, which is what a
	// list that emptied itself would leave.
	t.Run("NoSteps", func(t *testing.T) {
		if _, err := makeMacro("Night watch", nil, false); err == nil {
			t.Fatal("a macro with no steps was built")
		}
	})
}

// Test_onFailure tests the onFailure function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Stops: the default is to stop
//   - KeepsGoing: a macro asked to carry on says so
func Test_onFailure(t *testing.T) {
	// Verify that a macro says it stops, which is what it does unless it was
	// asked otherwise.
	t.Run("Stops", func(t *testing.T) {
		if got := onFailure(appcontext.Macro{Name: "Night watch"}); got != "stop" {
			t.Errorf("a macro that stops reads as %q, want %q", got, "stop")
		}
	})

	// Verify that a macro asked to carry on says so in the listing.
	t.Run("KeepsGoing", func(t *testing.T) {
		if got := onFailure(appcontext.Macro{Name: "Night watch", KeepGoing: true}); got != "keep going" {
			t.Errorf("a macro that carries on reads as %q, want %q", got, "keep going")
		}
	})
}

// Test_renderMacro tests the renderMacro function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Text: the name and the steps are printed
//   - JSON: the whole macro is written
//   - Unknown: a macro that is not in the file is reported
//   - Unreadable: a config file that cannot be read is reported
func Test_renderMacro(t *testing.T) {
	// Verify that the macro is read back from the file rather than echoed, and
	// that its steps are counted as a sentence.
	t.Run("Text", func(t *testing.T) {
		app, out, _ := isolate(t)
		if err := runMacroNew(app, "Night watch", []string{"volume set 4", "scan"}, false); err != nil {
			t.Fatalf("creating a macro: %v", err)
		}

		out.Reset()
		if err := renderMacro(app, "night watch"); err != nil {
			t.Fatalf("renderMacro: %v", err)
		}
		if got := out.String(); got != "Night watch: 2 steps\n  volume set 4\n  scan\n" {
			t.Errorf("the answer is %q", got)
		}
	})

	// Verify that the JSON answer is the whole macro, which is what a front end
	// builds a button from.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := isolate(t)
		if err := runMacroNew(app, "Night watch", []string{"scan"}, true); err != nil {
			t.Fatalf("creating a macro: %v", err)
		}

		app.Config.Output = appcontext.OutputJSON
		out.Reset()
		if err := renderMacro(app, "Night watch"); err != nil {
			t.Fatalf("renderMacro: %v", err)
		}

		var got appcontext.Macro
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, out.String())
		}
		if got.Name != "Night watch" || !got.KeepGoing {
			t.Errorf("the macro came back as %+v", got)
		}
	})

	// Verify that a macro the file does not hold is reported rather than
	// printed as an empty one.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		if err := renderMacro(app, "Nope"); err == nil {
			t.Fatal("renderMacro printed a macro that is not there")
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := renderMacro(app, "Night watch"); err == nil {
			t.Fatal("renderMacro answered from a file that is not JSON")
		}
	})
}

// Test_runMacroDelete tests the runMacroDelete function with 100% coverage.
//
// Coverage: 94% (5 test cases covering every reachable branch)
//
// The read back at macro.go:437 cannot fail. Update wrote the file from a
// Config a moment earlier, and nothing else touches it in between, so a file
// that was written is a file that parses.
//
// Test cases:
//   - JSON: the deletion is written as a report
//   - Unknown: a macro that is not there is reported
//   - StillThere: a deletion that did not take is refused rather than reported
//     as done
//   - Unreadable: a config file that cannot be read is reported
//   - Unwritable: a config file that cannot be written is reported
func Test_runMacroDelete(t *testing.T) {
	// Verify that the JSON answer says what went, which is the only thing left
	// to report once the macro is gone.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := isolate(t)
		given(t, app, "Night watch")

		app.Config.Output = appcontext.OutputJSON
		out.Reset()
		if err := runMacroDelete(app, "night watch", true); err != nil {
			t.Fatalf("runMacroDelete: %v", err)
		}

		var got struct {
			Name    string `json:"name"`
			Deleted bool   `json:"deleted"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, out.String())
		}
		if got.Name != "Night watch" || !got.Deleted {
			t.Errorf("the answer came back as %+v, want the stored name and the deletion", got)
		}
	})

	// Verify that a macro that is not there is reported as that rather than as
	// a missing flag.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		err := runMacroDelete(app, "Nope", false)
		if err == nil {
			t.Fatal("a macro that does not exist was deleted")
		}
		if !strings.Contains(err.Error(), "no macro called") {
			t.Errorf("the refusal said %q, which does not say the macro is unknown", err)
		}
	})

	// Verify that a macro still in the file afterwards is reported. Two macros
	// whose names differ only in case are one macro to every lookup, so
	// removing the one that was found leaves the other answering to the same
	// name.
	t.Run("StillThere", func(t *testing.T) {
		app, _, _ := isolateWith(t, `{"macros":[{"name":"One","steps":["battery"]},{"name":"one","steps":["screen"]}]}`)

		err := runMacroDelete(app, "One", true)
		if err == nil {
			t.Fatal("the deletion was reported as done with the name still in the file")
		}
		if !strings.Contains(err.Error(), "still in the config file") {
			t.Errorf("the failure said %q, which does not say what is wrong", err)
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runMacroDelete(app, "Night watch", true); err == nil {
			t.Fatal("runMacroDelete answered from a file that is not JSON")
		}
	})

	// Verify that a config file that cannot be written is reported rather than
	// the deletion being announced when nothing reached the disk.
	t.Run("Unwritable", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "Night watch")
		nowhereToWrite(t, app)

		if err := runMacroDelete(app, "Night watch", true); err == nil {
			t.Fatal("runMacroDelete reported a deletion it could not write")
		}
	})
}

// Test_runMacroList tests the runMacroList function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Text: the macros are listed in a table
//   - Empty: nothing to list is said on stderr, leaving stdout empty
//   - NoList: a file whose macros are null still answers with a list
//   - Unreadable: a config file that cannot be read is reported
func Test_runMacroList(t *testing.T) {
	// Verify that the table names each macro, counts its steps and says what
	// it does when one fails.
	t.Run("Text", func(t *testing.T) {
		app, out, _ := isolate(t)
		if err := runMacroNew(app, "Night watch", []string{"volume set 4", "scan"}, true); err != nil {
			t.Fatalf("creating a macro: %v", err)
		}
		if err := runMacroNew(app, "Quick look", []string{"screen"}, false); err != nil {
			t.Fatalf("creating a second macro: %v", err)
		}

		out.Reset()
		if err := runMacroList(app); err != nil {
			t.Fatalf("runMacroList: %v", err)
		}

		got := out.String()
		for _, want := range []string{"NAME", "STEPS", "ON FAILURE", "Night watch", "Quick look", "keep going", "stop"} {
			if !strings.Contains(got, want) {
				t.Errorf("the listing is %q, which leaves out %q", got, want)
			}
		}
	})

	// Verify that a script counting lines gets zero rather than a sentence
	// when there is nothing to list.
	t.Run("Empty", func(t *testing.T) {
		app, out, errs := isolate(t)

		if err := runMacroList(app); err != nil {
			t.Fatalf("runMacroList: %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("stdout holds %q, want nothing to list", out.String())
		}
		if !strings.Contains(errs.String(), "No macros yet") {
			t.Errorf("stderr holds %q, which does not say there are none", errs.String())
		}
	})

	// Verify that a file naming its macros as null still answers a page with a
	// list it can walk rather than with null.
	t.Run("NoList", func(t *testing.T) {
		app, out, _ := isolateWith(t, `{"macros":null}`)
		app.Config.Output = appcontext.OutputJSON

		if err := runMacroList(app); err != nil {
			t.Fatalf("runMacroList: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != "[]" {
			t.Errorf("the listing is %q, want %q", got, "[]")
		}
	})

	// Verify that a config file that cannot be read is reported rather than
	// listed as no macros at all.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runMacroList(app); err == nil {
			t.Fatal("runMacroList listed a file that is not JSON")
		}
	})
}

// Test_runMacroMove tests the runMacroMove function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - LeavesTheFileAlone: a list that has moved on since it was read is not
//     rearranged blind
//   - Unreadable: a config file that cannot be read is reported
//   - Unwritable: a config file that cannot be written is reported
func Test_runMacroMove(t *testing.T) {
	// Verify that the position read from a copy is checked again against what
	// is on disk, so a list that no longer looks the same is left as it is.
	t.Run("LeavesTheFileAlone", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One", "Two")

		// The settings this invocation resolved no longer hold the macros the
		// file does, which is what the change is applied to second.
		app.Config.Macros = nil

		if err := runMacroMove(app, "Two", "up"); err != nil {
			t.Fatalf("runMacroMove: %v", err)
		}
		if got := order(t, app); !sameSteps(got, []string{"Two", "One"}) {
			t.Errorf("the order on disk is %q, want the move to have happened there", got)
		}
		if len(app.Config.Macros) != 0 {
			t.Errorf("the settings in memory now hold %+v, want the change to have been skipped", app.Config.Macros)
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runMacroMove(app, "One", "up"); err == nil {
			t.Fatal("runMacroMove answered from a file that is not JSON")
		}
	})

	// Verify that a config file that cannot be written is reported rather than
	// the move being reported as made.
	t.Run("Unwritable", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One", "Two")
		nowhereToWrite(t, app)

		if err := runMacroMove(app, "Two", "up"); err == nil {
			t.Fatal("runMacroMove reported a move it could not write")
		}
	})
}

// Test_runMacroNew tests the runMacroNew function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Unreadable: a config file that cannot be read is reported
//   - Unwritable: a config file that cannot be written is reported
func Test_runMacroNew(t *testing.T) {
	// Verify that a config file that cannot be read is reported before a macro
	// is built from what was typed.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runMacroNew(app, "Night watch", []string{"scan"}, false); err == nil {
			t.Fatal("runMacroNew wrote over a file it could not read")
		}
	})

	// Verify that a config file that cannot be written is reported rather than
	// the macro being printed back as though it had been stored.
	t.Run("Unwritable", func(t *testing.T) {
		app, _, _ := isolate(t)
		nowhereToWrite(t, app)

		if err := runMacroNew(app, "Night watch", []string{"scan"}, false); err == nil {
			t.Fatal("runMacroNew reported a macro it could not write")
		}
	})
}

// Test_runMacroRename tests the runMacroRename function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Unknown: a macro that is not there is reported
//   - BadName: a new name that cannot be used is refused
//   - Unreadable: a config file that cannot be read is reported
//   - Unwritable: a config file that cannot be written is reported
func Test_runMacroRename(t *testing.T) {
	// Verify that renaming something that is not there is refused rather than
	// creating it under the new name.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)

		err := runMacroRename(app, "Nope", "Something")
		if err == nil {
			t.Fatal("a macro that does not exist was renamed")
		}
		if !strings.Contains(err.Error(), "no macro called") {
			t.Errorf("the refusal said %q, which does not say the macro is unknown", err)
		}
	})

	// Verify that a new name a button could not show is refused, and that the
	// macro keeps the name it had.
	t.Run("BadName", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One")

		if err := runMacroRename(app, "One", strings.Repeat("x", maxMacroName+1)); err == nil {
			t.Fatal("a name longer than a button was accepted")
		}
		if got := saved(t, app).Macros[0].Name; got != "One" {
			t.Errorf("the macro is called %q after a refusal, want %q", got, "One")
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runMacroRename(app, "One", "Two"); err == nil {
			t.Fatal("runMacroRename answered from a file that is not JSON")
		}
	})

	// Verify that a config file that cannot be written is reported rather than
	// the new name being printed back.
	t.Run("Unwritable", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One")
		nowhereToWrite(t, app)

		if err := runMacroRename(app, "One", "Two"); err == nil {
			t.Fatal("runMacroRename reported a name it could not write")
		}
	})
}

// Test_runMacroSet tests the runMacroSet function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - BadStep: a step that could never be split is refused
//   - Unreadable: a config file that cannot be read is reported
//   - Unwritable: a config file that cannot be written is reported
func Test_runMacroSet(t *testing.T) {
	// Verify that a step that could never run is refused, and that the macro
	// keeps the steps it had.
	t.Run("BadStep", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One")

		if err := runMacroSet(app, "One", []string{"battery | grep charge"}, false); err == nil {
			t.Fatal("a step with a shell operator was accepted")
		}
		if want := []string{"battery"}; !sameSteps(saved(t, app).Macros[0].Steps, want) {
			t.Errorf("the steps changed after a refusal, want %q", want)
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runMacroSet(app, "One", []string{"scan"}, false); err == nil {
			t.Fatal("runMacroSet wrote over a file it could not read")
		}
	})

	// Verify that a config file that cannot be written is reported rather than
	// the steps being printed back as though they had been stored.
	t.Run("Unwritable", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One")
		nowhereToWrite(t, app)

		if err := runMacroSet(app, "One", []string{"scan"}, false); err == nil {
			t.Fatal("runMacroSet reported steps it could not write")
		}
	})
}

// Test_runMacroShow tests the runMacroShow function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Text: the steps alone are printed, one per line
//   - JSON: the whole macro is written
//   - Unknown: a macro that is not there is reported
//   - Unreadable: a config file that cannot be read is reported
func Test_runMacroShow(t *testing.T) {
	// Verify that nothing but the steps is printed, so the answer can be used
	// without anything being cut off it first.
	t.Run("Text", func(t *testing.T) {
		app, out, _ := isolate(t)
		if err := runMacroNew(app, "Night watch", []string{"volume set 4", "scan"}, false); err != nil {
			t.Fatalf("creating a macro: %v", err)
		}

		out.Reset()
		if err := runMacroShow(app, "night watch"); err != nil {
			t.Fatalf("runMacroShow: %v", err)
		}
		if got := out.String(); got != "volume set 4\nscan\n" {
			t.Errorf("the answer is %q, want the steps alone", got)
		}
	})

	// Verify that the JSON answer is the whole macro rather than its steps.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := isolate(t)
		if err := runMacroNew(app, "Night watch", []string{"scan"}, false); err != nil {
			t.Fatalf("creating a macro: %v", err)
		}

		app.Config.Output = appcontext.OutputJSON
		out.Reset()
		if err := runMacroShow(app, "Night watch"); err != nil {
			t.Fatalf("runMacroShow: %v", err)
		}

		var got appcontext.Macro
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, out.String())
		}
		if got.Name != "Night watch" {
			t.Errorf("the macro came back as %+v", got)
		}
	})

	// Verify that a macro that is not there is reported, with the macros there
	// are.
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "One")

		err := runMacroShow(app, "Nope")
		if err == nil {
			t.Fatal("runMacroShow printed a macro that is not there")
		}
		if !strings.Contains(err.Error(), `"One"`) {
			t.Errorf("the refusal said %q, which does not name the macros there are", err)
		}
	})

	// Verify that a config file that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := isolate(t)
		unreadable(t, app)

		if err := runMacroShow(app, "One"); err == nil {
			t.Fatal("runMacroShow answered from a file that is not JSON")
		}
	})
}

// Test_runMacroDelete_readBack covers the read back that follows a successful
// write, which is the one failure the config file itself cannot be made to
// produce.
//
// Every other read here is failed by leaving something that is not JSON on
// disk. That does not reach this one: by the time it runs, Update has written
// the file, so what it reads is valid JSON in a readable file. The read is
// swapped out instead, and only on the second call, so the delete itself
// happens for real and only the check that it took is made to fail.
func Test_runMacroDelete_readBack(t *testing.T) {
	// Verify that a read back which fails is reported rather than the deletion
	// being announced on the strength of the write alone.
	t.Run("ReadBackFails", func(t *testing.T) {
		app, _, _ := isolate(t)
		given(t, app, "Night watch")

		real := savedConfig
		want := errors.New("the config file went away")
		calls := 0
		savedConfig = func(a *appcontext.App) (*appcontext.Config, error) {
			calls++
			if calls > 1 {
				return nil, want
			}
			return real(a)
		}
		t.Cleanup(func() { savedConfig = real })

		err := runMacroDelete(app, "Night watch", true)
		if err == nil {
			t.Fatal("runMacroDelete announced a deletion it never read back")
		}
		if !errors.Is(err, want) {
			t.Errorf("runMacroDelete reported %v, want the failed read back", err)
		}
		if calls < 2 {
			t.Errorf("savedConfig was called %d times, so the read back never happened", calls)
		}
	})
}
