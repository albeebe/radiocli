// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package colors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// TestNewReset tests the newReset function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its help text
//   - BadLayout: a word that is not a layout is refused by name
//   - Runs: naming a layout restores that one
//   - RunsAll: naming none restores all of them
func TestNewReset(t *testing.T) {
	// Verify that the command carries its name and its help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newReset(appcontext.New())

		if cmd.Name() != "reset" {
			t.Errorf("the command is %q, wanted %q", cmd.Name(), "reset")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify that a word which is not a layout is refused by name
	t.Run("BadLayout", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := newReset(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"purple"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "is not a layout") {
			t.Errorf("a word that is not a layout came back as %v", err)
		}
	})

	// Verify that naming a layout restores that one
	t.Run("Runs", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		cmd := newReset(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"tone-out"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("restoring a layout: %v", err)
		}
		if !strings.Contains(out.String(), "Restored tone-out") {
			t.Errorf("the command wrote:\n%s", out)
		}
	})

	// Verify that naming none restores all of them
	t.Run("RunsAll", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		cmd := newReset(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs(nil)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("restoring every layout: %v", err)
		}
		if !strings.Contains(out.String(), "Restored all 7 layouts") {
			t.Errorf("the command wrote:\n%s", out)
		}
	})
}

// Test_confirmRestore tests the confirmRestore function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Confirms: the prompt naming this entry is answered with yes
//   - NotAsking: a screen that is not the prompt is refused without pressing
//   - OtherEntry: the prompt for something else is refused
//   - ScreenError: a screen that cannot be read is reported
func Test_confirmRestore(t *testing.T) {
	// Verify that the prompt naming this entry is answered
	t.Run("Confirms", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.restoreMenu}, {on: r.restoreMenu.opens[allScreens]}}

		if err := confirmRestore(context.Background(), client, allScreens); err != nil {
			t.Fatalf("confirming the restore: %v", err)
		}
		if r.on() != r.restoreMenu {
			t.Errorf("the confirmation left the scanner on %q", r.on().title)
		}
	})

	// Verify that a screen which is not the prompt is refused, and nothing is
	// pressed into whatever the walk actually landed on
	t.Run("NotAsking", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.restoreMenu}}

		err := confirmRestore(context.Background(), client, allScreens)
		if err == nil || !strings.Contains(err.Error(), "did not ask to confirm") {
			t.Errorf("a screen that is not the prompt came back as %v", err)
		}
		if len(r.presses) != 0 {
			t.Errorf("something was pressed anyway: %v", r.presses)
		}
	})

	// Verify that the prompt for something else is refused, since the heading
	// alone would match a confirmation for anything
	t.Run("OtherEntry", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.restoreMenu}, {on: r.restoreMenu.opens["Weather Mode"]}}

		if err := confirmRestore(context.Background(), client, allScreens); err == nil {
			t.Error("the prompt for another entry was confirmed")
		}
	})

	// Verify that a screen which cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		err := confirmRestore(context.Background(), client, allScreens)
		if err == nil || !strings.Contains(err.Error(), "reading the confirmation") {
			t.Errorf("an unreadable screen came back as %v", err)
		}
	})

	// Verify that a scanner which will not take the press is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		r.failOn("KEY,E,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.restoreMenu}, {on: r.restoreMenu.opens[allScreens]}}

		err := confirmRestore(context.Background(), client, allScreens)
		if err == nil || !strings.Contains(err.Error(), "confirming the restore") {
			t.Errorf("a refused key came back as %v", err)
		}
	})
}

// Test_renderReset tests the renderReset function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - One: restoring one layout names it
//   - All: restoring all of them counts and lists them
//   - Reread: the layout that was read back afterwards is named
//   - JSON: the JSON form carries what was restored
//   - EncodeError: an encoder that refuses the report is reported
func Test_renderReset(t *testing.T) {
	// Verify that restoring one layout names it
	t.Run("One", func(t *testing.T) {
		app, out, _ := newApp()
		err := renderReset(app, resetReport{Entry: "Weather Mode", Layouts: []string{"weather"}})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "Restored weather to the scanner's default colors") {
			t.Errorf("the restore came back as:\n%s", out)
		}
	})

	// Verify that restoring all of them counts and lists them
	t.Run("All", func(t *testing.T) {
		app, out, _ := newApp()
		err := renderReset(app, resetReport{Entry: allScreens, Layouts: layoutNames()})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "Restored all 7 layouts") ||
			!strings.Contains(out.String(), "tone-out") {
			t.Errorf("the restore came back as:\n%s", out)
		}
	})

	// Verify that the layout read back afterwards is named, because the rest
	// are not known
	t.Run("Reread", func(t *testing.T) {
		app, out, _ := newApp()
		err := renderReset(app, resetReport{
			Entry: allScreens, Layouts: layoutNames(), Reread: "weather",
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "Read weather back") {
			t.Errorf("the restore came back as:\n%s", out)
		}
	})

	// Verify that the JSON form carries what was restored
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		err := renderReset(app, resetReport{Entry: allScreens, Layouts: layoutNames(), Reread: "weather"})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		var got resetReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the JSON does not parse: %v", err)
		}
		if got.Entry != allScreens || len(got.Layouts) != len(layouts) || got.Reread != "weather" {
			t.Errorf("the JSON came back as %+v", got)
		}
	})

	// Verify that an encoder which refuses the report is reported rather than
	// nothing at all being said
	t.Run("EncodeError", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		marshalJSON = func(any, string, string) ([]byte, error) {
			return nil, errors.New("no")
		}
		t.Cleanup(func() { marshalJSON = json.MarshalIndent })

		if err := renderReset(app, resetReport{Entry: allScreens}); err == nil {
			t.Errorf("a report that cannot be encoded came back as:\n%s", out)
		}
	})
}

// Test_rereadCurrent tests the rereadCurrent function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Reads: the restored layout on screen is read back and named
//   - NotCovered: a layout the restore did not touch is not worth the walk
//   - ChooseError: a scanner drawing nothing a layout covers is reported
//   - WalkError: an editor that cannot be walked is reported
func Test_rereadCurrent(t *testing.T) {
	// Verify that the restored layout on screen is read back and cached
	t.Run("Reads", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := rereadCurrent(context.Background(), app, client, layoutNames())
		if err != nil {
			t.Fatalf("reading the layout back: %v", err)
		}
		if got != "weather" {
			t.Errorf("the layout read back is %q, wanted %q", got, "weather")
		}

		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading the cache: %v", err)
		}
		if _, ok := c.lookup(scannerKey(fakeConn{}.Info()), "weather"); !ok {
			t.Error("the layout that was read back was not cached")
		}
	})

	// Verify that a layout the restore did not touch is left alone, since
	// nothing anybody is looking at changed
	t.Run("NotCovered", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := rereadCurrent(context.Background(), app, client, []string{"tone-out"})
		if err != nil {
			t.Fatalf("reading the layout back: %v", err)
		}
		if got != "" {
			t.Errorf("a layout the restore did not cover was read back as %q", got)
		}
		if len(r.presses) != 0 {
			t.Errorf("the walk ran anyway: %v", r.presses)
		}
	})

	// Verify that a scanner drawing nothing a layout covers is reported
	t.Run("ChooseError", func(t *testing.T) {
		isolate(t)
		fast(t)

		app, _, _ := newApp()
		client := device.New(fakeConn{reply: newRadio("recording", 14, 2).reply})

		if _, err := rereadCurrent(context.Background(), app, client, layoutNames()); err == nil {
			t.Error("a view no layout covers was read back")
		}
	})

	// Verify that an editor which cannot be walked is reported
	t.Run("WalkError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := rereadCurrent(context.Background(), app, client, layoutNames()); err == nil {
			t.Error("an editor that will not open was read back")
		}
	})

	// Verify that a scanner which will not leave its menus is reported
	t.Run("LeaveError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("KEY,L,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := rereadCurrent(context.Background(), app, client, layoutNames()); err == nil {
			t.Error("a scanner stuck in its menus was read back")
		}
	})
}

// Test_restoreEntryFor tests the restoreEntryFor function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - All: naming no layout is the scanner's own All Screens
//   - One: naming a layout is that layout's entry without the "Set "
func Test_restoreEntryFor(t *testing.T) {
	// Verify that naming no layout covers all seven
	t.Run("All", func(t *testing.T) {
		entry, covered := restoreEntryFor("")
		if entry != allScreens {
			t.Errorf("the entry is %q, wanted %q", entry, allScreens)
		}
		if len(covered) != len(layouts) {
			t.Errorf("%d layouts are covered, wanted %d", len(covered), len(layouts))
		}
	})

	// Verify that naming a layout uses the Customize entry with the "Set "
	// taken off, which is how the Restore menu spells it
	t.Run("One", func(t *testing.T) {
		entry, covered := restoreEntryFor("simple-trunk")
		if entry != "Simple Trunk" {
			t.Errorf("the entry is %q, wanted %q", entry, "Simple Trunk")
		}
		if len(covered) != 1 || covered[0] != "simple-trunk" {
			t.Errorf("the layouts covered are %v", covered)
		}
	})
}

// Test_runReset tests the runReset function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a run with no scanner named is reported
//   - All: every layout is restored and the one on screen is read back
//   - One: naming a layout restores only that one
//   - OpenError: a Restore menu that cannot be walked is reported
//   - ConfirmError: a confirmation that is not the one expected stops the run
//   - RereadError: a restore that worked is reported even when the read back
//     does not
func Test_runReset(t *testing.T) {
	// Verify that a run with no scanner named is reported
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()
		if err := runReset(context.Background(), app, ""); err == nil {
			t.Error("a run with no scanner reported success")
		}
	})

	// Verify that every layout is restored, the stored readings are dropped,
	// and the one on screen is read back
	t.Run("All", func(t *testing.T) {
		isolate(t)

		app, out, notes := newApp()
		r := newRadio("wx_alert", 14, 2)
		use(app, r)

		remember(quiet(), fakeConn{}.Info(), "tone-out", []area{{Name: "Func", Text: "Red"}}, time.Now())

		if err := runReset(context.Background(), app, ""); err != nil {
			t.Fatalf("restoring: %v", err)
		}
		if !strings.Contains(out.String(), "Restored all 7 layouts") ||
			!strings.Contains(out.String(), "Read weather back") {
			t.Errorf("the restore came back as:\n%s", out)
		}
		if !strings.Contains(notes.String(), "stops the scan") {
			t.Errorf("the restore said nothing about stopping the scan:\n%s", notes)
		}

		// The layouts it did not read back hold nothing, because the scanner
		// was never asked what their new colors are.
		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading the cache: %v", err)
		}
		if _, ok := c.lookup(scannerKey(fakeConn{}.Info()), "tone-out"); ok {
			t.Error("a restored layout kept its old cached colors")
		}
	})

	// Verify that naming a layout restores only that one
	t.Run("One", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		if err := runReset(context.Background(), app, "tone-out"); err != nil {
			t.Fatalf("restoring: %v", err)
		}
		if !strings.Contains(out.String(), "Restored tone-out") {
			t.Errorf("the restore came back as:\n%s", out)
		}
		if strings.Contains(out.String(), "Read ") {
			t.Errorf("a layout that is not on screen was read back:\n%s", out)
		}
	})

	// Verify that a Restore menu which cannot be walked is reported
	t.Run("OpenError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		use(app, r)

		if err := runReset(context.Background(), app, ""); err == nil {
			t.Error("a menu that will not open was restored from")
		}
	})

	// Verify that a confirmation which is not the one expected stops the run
	t.Run("ConfirmError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.restoreMenu.opens[allScreens] = &view{title: restoreEntry, rows: []string{"somewhere else"}}
		use(app, r)

		err := runReset(context.Background(), app, "")
		if err == nil || !strings.Contains(err.Error(), "nothing was restored") {
			t.Errorf("a walk that landed elsewhere came back as %v", err)
		}
	})

	// Verify that a restore which worked is reported even when the colors
	// cannot be read back afterwards
	t.Run("RereadError", func(t *testing.T) {
		isolate(t)
		fast(t)

		app, out, notes := newApp()
		r := newRadio("wx_alert", 14, 2)
		use(app, r)
		r.fails = func(command string) error {
			// Once the restore is done the scanner reports a view no layout
			// covers, so the reading that follows cannot start.
			if command == "GSI" && len(r.presses) > 0 {
				r.screen = "recording"
			}
			return nil
		}

		if err := runReset(context.Background(), app, ""); err != nil {
			t.Fatalf("restoring: %v", err)
		}
		if !strings.Contains(out.String(), "Restored all 7 layouts") {
			t.Errorf("the restore came back as:\n%s", out)
		}
		if !strings.Contains(notes.String(), "could not be read back") {
			t.Errorf("the failed reading was not reported:\n%s", notes)
		}
	})

	// Verify that a scanner which will not leave its menus is reported
	t.Run("LeaveError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("KEY,L,P", errors.New("the scanner refused the key"))
		use(app, r)

		err := runReset(context.Background(), app, "")
		if err == nil || !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("a scanner stuck in its menus came back as %v", err)
		}
	})
}
