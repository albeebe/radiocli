// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package colors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// Test_areaUnderCursor tests the areaUnderCursor function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Reads: the area the knob is on is named, and the editor comes back
//   - EnterError: a scanner that will not open the area is reported
//   - MenuError: an area's menu that cannot be read is reported
//   - BackError: a scanner that will not back out is reported
func Test_areaUnderCursor(t *testing.T) {
	// Verify that the area is named and the walk ends back on the editor
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		editor := r.customizeMenu.opens["Set Simple Conventional"]
		r.stack = []frame{{on: editor}}

		got, err := areaUnderCursor(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the area under the cursor: %v", err)
		}
		if got != "Func" {
			t.Errorf("the area is %q, wanted %q", got, "Func")
		}
		if r.on() != editor {
			t.Errorf("the walk ended on %q", r.on().title)
		}
	})

	// Verify that a scanner which will not open the area is reported
	t.Run("EnterError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		r.failOn("KEY,E,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		if _, err := areaUnderCursor(context.Background(), client); err == nil {
			t.Error("a refused key was reported as an area")
		}
	})

	// Verify that an area menu which cannot be read is reported
	t.Run("MenuError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		r.failOn("MSI", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		_, err := areaUnderCursor(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading an area's menu") {
			t.Errorf("an unreadable menu came back as %v", err)
		}
	})

	// Verify that a scanner which will not back out is reported
	t.Run("BackError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		r.failOn("KEY,M,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		_, err := areaUnderCursor(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "going back one level") {
			t.Errorf("a refused key came back as %v", err)
		}
	})
}

// Test_choose tests the choose function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Current: no layout named settles on the one the scanner is drawing with
//   - Named: a named layout is used, and said to be the one on screen or not
//   - Error: naming none when none can be settled on is reported
func Test_choose(t *testing.T) {
	// Verify that naming no layout settles on the one on screen
	t.Run("Current", func(t *testing.T) {
		r := newRadio("tone_out", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, current, err := choose(context.Background(), client, "")
		if err != nil {
			t.Fatalf("choosing a layout: %v", err)
		}
		if got.name != "tone-out" || !current {
			t.Errorf("the layout came back as %q (%t)", got.name, current)
		}
	})

	// Verify that a named layout is used, and is not claimed to be on screen
	t.Run("Named", func(t *testing.T) {
		r := newRadio("tone_out", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, current, err := choose(context.Background(), client, "weather")
		if err != nil {
			t.Fatalf("choosing a layout: %v", err)
		}
		if got.name != "weather" || current {
			t.Errorf("the layout came back as %q (%t)", got.name, current)
		}
	})

	// Verify that naming none when none can be settled on is reported
	t.Run("Error", func(t *testing.T) {
		fast(t)

		r := newRadio("recording", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		if _, _, err := choose(context.Background(), client, ""); err == nil {
			t.Error("a view no layout covers was chosen")
		}
	})
}

// Test_compare tests the compare function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Agrees: a scanner drawing every area where the map says reports nothing
//   - Moved: an area drawn somewhere else is reported with both positions
//   - Extra: an area the map does not place at all is reported as found only
//   - Missing: an area the scanner never showed is reported as expected only
func Test_compare(t *testing.T) {
	want, _ := lookup("simple-conventional")

	// Verify that a scanner agreeing with the map reports nothing, and that
	// the soft keys are not counted as a disagreement
	t.Run("Agrees", func(t *testing.T) {
		found := map[string]position{"Soft1_key": {Line: 13, Column: 0, Length: 4, Height: 1}}
		for name, at := range positions[want.name] {
			found[name] = at
		}

		if got := compare(want, found); len(got) != 0 {
			t.Errorf("a scanner that agrees reported %v", got)
		}
	})

	// Verify that an area drawn somewhere else carries both positions
	t.Run("Moved", func(t *testing.T) {
		found := map[string]position{}
		for name, at := range positions[want.name] {
			found[name] = at
		}
		found["Func"] = position{Line: 9, Column: 9, Length: 9, Height: 1}

		got := compare(want, found)
		if len(got) != 1 || got[0].Area != "Func" {
			t.Fatalf("a moved area reported %v", got)
		}
		if got[0].Expected == nil || got[0].Found == nil {
			t.Errorf("the difference carries only one position: %+v", got[0])
		}
	})

	// Verify that an area the map does not place at all is reported
	t.Run("Extra", func(t *testing.T) {
		found := map[string]position{}
		for name, at := range positions[want.name] {
			found[name] = at
		}
		found["Made_up"] = position{Line: 1, Column: 1, Length: 1, Height: 1}

		got := compare(want, found)
		if len(got) != 1 || got[0].Area != "Made_up" || got[0].Expected != nil {
			t.Errorf("an area the map does not hold reported %v", got)
		}
	})

	// Verify that an area the scanner never showed is just as wrong as one in
	// the wrong place
	t.Run("Missing", func(t *testing.T) {
		got := compare(want, map[string]position{})
		if len(got) != len(positions[want.name]) {
			t.Fatalf("%d areas were reported, wanted %d", len(got), len(positions[want.name]))
		}
		if got[0].Found != nil {
			t.Errorf("an area the scanner never showed carries a position: %+v", got[0])
		}
	})
}

// Test_describe tests the describe function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Position: a position renders its line, column and width
//   - None: nothing to describe renders a dash
func Test_describe(t *testing.T) {
	// Verify that a position renders as the reader would read it
	t.Run("Position", func(t *testing.T) {
		got := describe(&position{Line: 2, Column: 4, Length: 16})
		if got != "line 2, col 4, 16 wide" {
			t.Errorf("the position rendered as %q", got)
		}
	})

	// Verify that nothing to describe renders a dash
	t.Run("None", func(t *testing.T) {
		if got := describe(nil); got != "-" {
			t.Errorf("no position rendered as %q, wanted %q", got, "-")
		}
	})
}

// Test_errDiffers tests the errDiffers function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Counts: the failure says how many areas disagree and out of how many
func Test_errDiffers(t *testing.T) {
	// Verify that the failure counts the areas and names the layout
	t.Run("Counts", func(t *testing.T) {
		err := errDiffers(verifyReport{
			Layout:      "weather",
			Checked:     40,
			Differences: []difference{{Area: "Func"}, {Area: "Battery"}},
		})
		if err == nil || !strings.Contains(err.Error(), "2 of 40 areas of weather") {
			t.Errorf("the failure reads %v", err)
		}
	})
}

// Test_isSoftKey tests the isSoftKey function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Keys: the three keys and the two gaps are soft keys
//   - Others: anything else is not
func Test_isSoftKey(t *testing.T) {
	// Verify that all five regions of the bottom row are soft keys
	t.Run("Keys", func(t *testing.T) {
		for _, name := range []string{"Soft1_key", "Space_1", "Soft2_key", "Space_2", "Soft3_key"} {
			if !isSoftKey(name) {
				t.Errorf("%s is not reported as a soft key", name)
			}
		}
	})

	// Verify that an area elsewhere on the screen is not
	t.Run("Others", func(t *testing.T) {
		for _, name := range []string{"Func", "Space_0", "System_name", ""} {
			if isSoftKey(name) {
				t.Errorf("%q is reported as a soft key", name)
			}
		}
	})
}

// Test_merge tests the merge function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - One: a single run is one area a row tall
//   - Stacked: a run of rows at the same column and width is one taller area
//   - Nothing: a screen with no reverse video at all is unreadable
//   - Apart: runs that do not stack up are unreadable
func Test_merge(t *testing.T) {
	// Verify that a single run is one area a row tall
	t.Run("One", func(t *testing.T) {
		got, ok, err := merge([]position{{Line: 4, Column: 0, Length: 26, Height: 1}})
		if err != nil || !ok {
			t.Fatalf("one run came back as %v (%t)", err, ok)
		}
		if got.Height != 1 || got.Line != 4 {
			t.Errorf("one run came back as %+v", got)
		}
	})

	// Verify that rows at the same column and width fold into one taller area
	t.Run("Stacked", func(t *testing.T) {
		got, ok, err := merge([]position{
			{Line: 2, Column: 0, Length: 24, Height: 1},
			{Line: 3, Column: 0, Length: 24, Height: 1},
		})
		if err != nil || !ok {
			t.Fatalf("two rows came back as %v (%t)", err, ok)
		}
		if got.Height != 2 || got.Line != 2 {
			t.Errorf("two rows came back as %+v", got)
		}
	})

	// Verify that a screen with no reverse video is unreadable rather than
	// guessed at
	t.Run("Nothing", func(t *testing.T) {
		if _, ok, _ := merge(nil); ok {
			t.Error("a screen with nothing highlighted was read")
		}
	})

	// Verify that runs which do not stack up are unreadable
	t.Run("Apart", func(t *testing.T) {
		if _, ok, _ := merge([]position{
			{Line: 2, Column: 0, Length: 24},
			{Line: 5, Column: 4, Length: 10},
		}); ok {
			t.Error("two unrelated runs were read as one area")
		}
	})
}

// Test_placed tests the placed function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Reads: every area the map places comes back, in reading order
func Test_placed(t *testing.T) {
	// Verify that the map's areas come back sorted down the screen
	t.Run("Reads", func(t *testing.T) {
		want, _ := lookup("weather")

		got := placed(want)
		if len(got) != len(positions[want.name]) {
			t.Fatalf("%d areas came back, wanted %d", len(got), len(positions[want.name]))
		}
		for i := 1; i < len(got); i++ {
			if got[i-1].Line > got[i].Line {
				t.Fatalf("the areas are not in reading order at %d: %+v", i, got[i-1])
			}
		}
		if got[0].Text != "" {
			t.Errorf("a positions only reading carries a color: %+v", got[0])
		}
	})
}

// Test_renderVerify tests the renderVerify function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Agrees: a map that is right says so and reports success
//   - Differs: a map that is wrong is tabled and reported as a failure
//   - JSONAgrees: the JSON form of a map that is right reports success
//   - JSONDiffers: the JSON form of a map that is wrong is still a failure
//   - WriteError: a stream that refuses the table is reported
func Test_renderVerify(t *testing.T) {
	r := verifyReport{Layout: "weather", Menu: "Set Weather Mode", Checked: 44}
	wrong := r
	wrong.Differences = []difference{{
		Area:     "Func",
		Expected: &position{Line: 0, Column: 0, Length: 2},
		Found:    &position{Line: 1, Column: 1, Length: 3},
	}}

	// Verify that a map which is right says so
	t.Run("Agrees", func(t *testing.T) {
		app, out, _ := newApp()
		if err := renderVerify(app, r); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "Every area is where the built-in map says") {
			t.Errorf("a map that is right came back as:\n%s", out)
		}
	})

	// Verify that a map which is wrong is tabled and fails, so a script
	// running this notices
	t.Run("Differs", func(t *testing.T) {
		app, out, _ := newApp()

		err := renderVerify(app, wrong)
		if err == nil || !strings.Contains(err.Error(), "1 of 44 areas") {
			t.Errorf("a map that is wrong came back as %v", err)
		}
		if !strings.Contains(out.String(), "ON THE SCANNER") || !strings.Contains(out.String(), "line 1, col 1") {
			t.Errorf("the differences came back as:\n%s", out)
		}
	})

	// Verify that the JSON form of a map which is right reports success
	t.Run("JSONAgrees", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := renderVerify(app, r); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		var got verifyReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the JSON does not parse: %v", err)
		}
		if got.Checked != 44 {
			t.Errorf("the JSON came back as %+v", got)
		}
	})

	// Verify that the JSON form of a map which is wrong is still a failure
	t.Run("JSONDiffers", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := renderVerify(app, wrong); err == nil {
			t.Error("a map that is wrong reported success")
		}
		if !strings.Contains(out.String(), "Func") {
			t.Errorf("the JSON came back as:\n%s", out)
		}
	})

	// Verify that a stream which refuses the output is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		if err := renderVerify(app, wrong); err == nil {
			t.Error("a refused stream was reported as written")
		}

		app.Config.Output = appcontext.OutputJSON
		if err := renderVerify(app, r); err == nil {
			t.Error("a refused stream was reported as encoded")
		}
	})
}

// Test_reverseRuns tests the reverseRuns function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - One: a single run is found with its column and width
//   - Several: every run of a row is found
//   - ToTheEnd: a run reaching the end of the row is closed off
//   - None: a row drawn normally holds no runs
func Test_reverseRuns(t *testing.T) {
	// Verify that a single run is found where it starts and how wide it is
	t.Run("One", func(t *testing.T) {
		got := reverseRuns("  ****  ")
		if len(got) != 1 || got[0].Column != 2 || got[0].Length != 4 {
			t.Errorf("one run came back as %v", got)
		}
	})

	// Verify that every run of the row is found
	t.Run("Several", func(t *testing.T) {
		got := reverseRuns("**** *** ****")
		if len(got) != 3 {
			t.Fatalf("%d runs came back, wanted 3", len(got))
		}
		if got[2].Column != 9 || got[2].Length != 4 {
			t.Errorf("the last run came back as %+v", got[2])
		}
	})

	// Verify that a run reaching the end of the row is closed off
	t.Run("ToTheEnd", func(t *testing.T) {
		got := reverseRuns("  ***")
		if len(got) != 1 || got[0].Column != 2 || got[0].Length != 3 {
			t.Errorf("a run to the end came back as %v", got)
		}
	})

	// Verify that a row drawn entirely normally holds no runs
	t.Run("None", func(t *testing.T) {
		if got := reverseRuns("      "); len(got) != 0 {
			t.Errorf("a plain row came back as %v", got)
		}
	})
}

// Test_runPositions tests the runPositions function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a run with no scanner named is reported
//   - Current: the layout on screen gets its soft keys from the live row
//   - Named: another layout is reported without its soft keys
//   - ChooseError: a scanner drawing nothing a layout covers is reported
//   - ScreenError: a live screen that cannot be read is reported
func Test_runPositions(t *testing.T) {
	// Verify that a run with no scanner named is reported
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()
		if err := runPositions(context.Background(), app, ""); err == nil {
			t.Error("a run with no scanner reported success")
		}
	})

	// Verify that the layout on screen has its soft keys read off the live row
	t.Run("Current", func(t *testing.T) {
		app, out, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		use(app, r)

		if err := runPositions(context.Background(), app, ""); err != nil {
			t.Fatalf("reporting the positions: %v", err)
		}
		if !strings.Contains(out.String(), "Soft1_key") {
			t.Errorf("the soft keys were not read:\n%s", out)
		}
		if len(r.presses) != 0 {
			t.Errorf("reporting the positions opened a menu: %v", r.presses)
		}
	})

	// Verify that another layout is reported without soft keys, since the
	// live row is not the row that draws them
	t.Run("Named", func(t *testing.T) {
		app, out, _ := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		if err := runPositions(context.Background(), app, "tone-out"); err != nil {
			t.Fatalf("reporting the positions: %v", err)
		}
		if strings.Contains(out.String(), "Soft1_key") {
			t.Errorf("a layout that is not on screen got soft keys:\n%s", out)
		}
	})

	// Verify that a scanner drawing nothing a layout covers is reported
	t.Run("ChooseError", func(t *testing.T) {
		fast(t)

		app, _, _ := newApp()
		use(app, newRadio("recording", 14, 2))

		if err := runPositions(context.Background(), app, ""); err == nil {
			t.Error("a view no layout covers was reported")
		}
	})

	// Verify that a live screen which cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.fails = func(command string) error {
			if command == "STS" {
				return errors.New("the port is gone")
			}
			return nil
		}
		use(app, r)

		err := runPositions(context.Background(), app, "weather")
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("an unreadable screen came back as %v", err)
		}
	})
}

// Test_runVerify tests the runVerify function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a run with no scanner named is reported
//   - Agrees: a scanner drawing every area where the map says reports success
//   - Differs: a scanner that disagrees with the map fails
//   - ChooseError: a scanner drawing nothing a layout covers is reported
//   - WalkError: an editor that cannot be walked is reported
func Test_runVerify(t *testing.T) {
	// Verify that a run with no scanner named is reported
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()
		if err := runVerify(context.Background(), app, ""); err == nil {
			t.Error("a run with no scanner reported success")
		}
	})

	// Verify that a scanner agreeing with the map reports success
	t.Run("Agrees", func(t *testing.T) {
		app, out, notes := newApp()
		use(app, newRadio("wx_alert", 14, len(areaOrder["weather"])))

		if err := runVerify(context.Background(), app, ""); err != nil {
			t.Fatalf("checking the positions: %v", err)
		}
		if !strings.Contains(out.String(), "Every area is where the built-in map says") {
			t.Errorf("the check came back as:\n%s", out)
		}
		if !strings.Contains(notes.String(), "stops the scan") {
			t.Errorf("the check said nothing about stopping the scan:\n%s", notes)
		}
	})

	// Verify that a scanner drawing only some of the areas fails, since an
	// area the map places and the scanner never showed is just as wrong
	t.Run("Differs", func(t *testing.T) {
		app, _, _ := newApp()
		use(app, newRadio("wx_alert", 14, 3))

		err := runVerify(context.Background(), app, "")
		if err == nil || !strings.Contains(err.Error(), "not where the built-in map says") {
			t.Errorf("a partial editor came back as %v", err)
		}
	})

	// Verify that a scanner drawing nothing a layout covers is reported
	t.Run("ChooseError", func(t *testing.T) {
		fast(t)

		app, _, _ := newApp()
		use(app, newRadio("recording", 14, 2))

		if err := runVerify(context.Background(), app, ""); err == nil {
			t.Error("a view no layout covers was checked")
		}
	})

	// Verify that an editor which cannot be walked is reported
	t.Run("WalkError", func(t *testing.T) {
		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 3)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		use(app, r)

		if err := runVerify(context.Background(), app, ""); err == nil {
			t.Error("an editor that will not open was checked")
		}
	})

	// Verify that a scanner which will not leave its menus is reported
	t.Run("LeaveError", func(t *testing.T) {
		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 3)
		r.failOn("KEY,L,P", errors.New("the scanner refused the key"))
		use(app, r)

		err := runVerify(context.Background(), app, "")
		if err == nil || !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("a scanner stuck in its menus came back as %v", err)
		}
	})
}

// Test_selectedSpan tests the selectedSpan function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Reads: the span the editor draws in reverse video comes back
//   - BottomRow: a highlight on the bottom row alone is unreadable
//   - ScreenError: a screen that cannot be read is reported
func Test_selectedSpan(t *testing.T) {
	// Verify that the span the editor highlights comes back where the map
	// says the area is
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 20)
		client := device.New(fakeConn{reply: r.reply})

		editor := r.customizeMenu.opens["Set Simple Conventional"]
		r.stack = []frame{{on: editor, cursor: 14}}

		got, ok, err := selectedSpan(context.Background(), client)
		if err != nil || !ok {
			t.Fatalf("reading the span: %v (%t)", err, ok)
		}
		want := positions["simple-conventional"]["System_name"]
		if got != want {
			t.Errorf("the span came back as %+v, wanted %+v", got, want)
		}
	})

	// Verify that the bottom row is left out, since it is reverse video at
	// all times
	t.Run("BottomRow", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: &view{title: "Editor", rows: []string{"top", "bottom"}}, cursor: 1}}

		_, ok, err := selectedSpan(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the span: %v", err)
		}
		if ok {
			t.Error("a highlight on the bottom row was read as an area")
		}
	})

	// Verify that a screen which cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		_, _, err := selectedSpan(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("an unreadable screen came back as %v", err)
		}
	})
}

// Test_softKeys tests the softKeys function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Reads: three runs of reverse video are the five regions of the row
//   - NotThree: a bottom row that is not three keys reports none
//   - Empty: an empty screen reports none
//   - ScreenError: a screen that cannot be read is reported
func Test_softKeys(t *testing.T) {
	// Verify that the three runs and the two gaps between them come back in
	// the scanner's own order
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := softKeys(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the soft keys: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("%d areas came back, wanted 5", len(got))
		}

		for i, want := range []struct {
			name           string
			column, length int
		}{
			{"Soft1_key", 0, 4},
			{"Space_1", 4, 1},
			{"Soft2_key", 5, 3},
			{"Space_2", 8, 1},
			{"Soft3_key", 9, 4},
		} {
			if got[i].Name != want.name || got[i].Column != want.column || got[i].Length != want.length {
				t.Errorf("region %d came back as %+v, wanted %s at %d for %d",
					i, got[i], want.name, want.column, want.length)
			}
			if got[i].Line != 13 || got[i].Height != 1 {
				t.Errorf("region %d is not on the bottom row: %+v", i, got[i])
			}
		}
	})

	// Verify that a bottom row which is not three keys reports none rather
	// than a guess at which is which
	t.Run("NotThree", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.scanning = []line{{text: "MENU", attrs: "****"}}
		client := device.New(fakeConn{reply: r.reply})

		got, err := softKeys(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the soft keys: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("a row that is not three keys came back as %v", got)
		}
	})

	// Verify that an empty screen reports none
	t.Run("Empty", func(t *testing.T) {
		r := newRadio("conventional_scan", 0, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := softKeys(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the soft keys: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("an empty screen came back as %v", got)
		}
	})

	// Verify that a screen which cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		_, err := softKeys(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("an unreadable screen came back as %v", err)
		}
	})
}

// Test_sortAreas tests the sortAreas function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Order: areas come out down the screen, then across it, then by name
func Test_sortAreas(t *testing.T) {
	// Verify that reading order is line, then column, then name
	t.Run("Order", func(t *testing.T) {
		got := []area{
			{Name: "Third", Line: 2, Column: 0},
			{Name: "Second", Line: 1, Column: 8},
			{Name: "B", Line: 1, Column: 0},
			{Name: "A", Line: 1, Column: 0},
		}
		sortAreas(got)

		var names []string
		for _, a := range got {
			names = append(names, a.Name)
		}
		if strings.Join(names, ",") != "A,B,Second,Third" {
			t.Errorf("the areas came out as %v", names)
		}
	})
}

// Test_sortDifferences tests the sortDifferences function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Order: differences come out by name, since some have no position
func Test_sortDifferences(t *testing.T) {
	// Verify that differences come out in a stable order
	t.Run("Order", func(t *testing.T) {
		got := []difference{{Area: "Zebra"}, {Area: "Apple"}, {Area: "Mango"}}
		sortDifferences(got)

		if got[0].Area != "Apple" || got[2].Area != "Zebra" {
			t.Errorf("the differences came out as %v", got)
		}
	})
}

// Test_walkPositions tests the walkPositions function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Walks: every area the editor draws is recorded where it drew it
//   - OpenError: an editor that will not open is reported
//   - SpanError: a screen that cannot be read is reported
//   - NameError: an area that cannot be named is reported
//   - PressError: a scanner that will not take the key press is reported
//   - NeverComesRound: an editor that does not wrap is given up on
func Test_walkPositions(t *testing.T) {
	want, _ := lookup("simple-conventional")

	// Verify that every area is recorded, and an area the editor cannot draw
	// a span for is left out rather than guessed at
	t.Run("Walks", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, len(areaOrder[want.name]))
		client := device.New(fakeConn{reply: r.reply})

		got, err := walkPositions(context.Background(), client, want)
		if err != nil {
			t.Fatalf("walking the editor: %v", err)
		}
		if len(got) != len(positions[want.name]) {
			t.Fatalf("%d areas were recorded, wanted %d", len(got), len(positions[want.name]))
		}
		if got["System_name"] != positions[want.name]["System_name"] {
			t.Errorf("System_name was recorded at %+v", got["System_name"])
		}
		if _, held := got["Soft1_key"]; held {
			t.Error("a soft key was recorded, which the editor cannot say anything about")
		}
	})

	// Verify that an editor which will not open is reported
	t.Run("OpenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := walkPositions(context.Background(), client, want); err == nil {
			t.Error("an editor that will not open was walked")
		}
	})

	// Verify that a screen which cannot be read is reported
	t.Run("SpanError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "STS" && r.on() == r.customizeMenu.opens[want.entry] {
				return errors.New("the port is gone")
			}
			return nil
		}

		if _, err := walkPositions(context.Background(), client, want); err == nil {
			t.Error("an unreadable screen was walked")
		}
	})

	// Verify that an area which cannot be named is reported
	t.Run("NameError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "MSI" && r.on() != nil && strings.HasSuffix(r.on().title, " Area") {
				return errors.New("the port is gone")
			}
			return nil
		}

		if _, err := walkPositions(context.Background(), client, want); err == nil {
			t.Error("an area that could not be named was walked past")
		}
	})

	// Verify that a refused key press is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "KEY,>,P" && r.on() == r.customizeMenu.opens[want.entry] {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		_, err := walkPositions(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "stepping to the next area") {
			t.Errorf("a refused key came back as %v", err)
		}
	})

	// Verify that an editor which never comes back round is given up on
	t.Run("NeverComesRound", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)

		editor := &view{title: want.entry, opens: map[string]*view{}}
		for i := 0; i <= maxAreas; i++ {
			name := fmt.Sprintf("Area_%d", i)
			editor.rows = append(editor.rows, name)
			editor.opens[name] = &view{title: "Set " + name + " Area", rows: []string{"Set Something"}}
		}
		r.customizeMenu.opens[want.entry] = editor
		client := device.New(fakeConn{reply: r.reply})

		_, err := walkPositions(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "did not come back round") {
			t.Errorf("an editor that never wraps came back as %v", err)
		}
	})
}
