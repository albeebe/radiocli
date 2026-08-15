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
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// onArea takes the fake radio to one area's menu of one layout's editor, which
// is where the walks that set a color start from.
func onArea(r *radio, entry, name string) {
	editor := r.customizeMenu.opens[entry]
	r.stack = []frame{{on: editor}, {on: editor.opens[name]}}
}

// TestNewSet tests the newSet function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its flags and help text
//   - NothingToSet: a run that names no color is refused
//   - BadColor: a color the scanner does not offer is refused with suggestions
//   - BadLayout: a word that is not a layout is refused by name
//   - Runs: the command sets the color it was given
func TestNewSet(t *testing.T) {
	// Verify that the command carries its name, its flags and its help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSet(appcontext.New())

		if cmd.Name() != "set" {
			t.Errorf("the command is %q, wanted %q", cmd.Name(), "set")
		}
		for _, name := range []string{"text", "back", "layout"} {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("the command has no --%s flag", name)
			}
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify that a run naming no color is refused
	t.Run("NothingToSet", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := newSet(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"Func"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "nothing to set") {
			t.Errorf("a run with no color came back as %v", err)
		}
	})

	// Verify that a color the scanner does not offer is refused, and near
	// misses are named
	t.Run("BadColor", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := newSet(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"Func", "--back", "blueish"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "is not a color the scanner offers") {
			t.Errorf("a color that does not exist came back as %v", err)
		}
	})

	// Verify that a word which is not a layout is refused by name
	t.Run("BadLayout", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := newSet(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"Func", "--text", "Red", "--layout", "purple"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "is not a layout") {
			t.Errorf("a word that is not a layout came back as %v", err)
		}
	})

	// Verify that the command sets the color it was given
	t.Run("Runs", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		use(app, newRadio("conventional_scan", simpleLines, 2))

		cmd := newSet(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"Func", "--text", "Red"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("setting a color: %v", err)
		}
		if !strings.Contains(out.String(), "Red") {
			t.Errorf("the command wrote:\n%s", out)
		}
	})
}

// Test_color tests the color function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Found: a color is found without regard to case, with its value
//   - Missing: a color the scanner does not offer is refused
func Test_color(t *testing.T) {
	// Verify that a color comes back with the value the scanner reports
	t.Run("Found", func(t *testing.T) {
		got, ok := color("orangered")
		if !ok {
			t.Fatal("a color the scanner offers was not found")
		}
		if got.Name != "Orangered" || got.Hex != "#FF4600" {
			t.Errorf("the color came back as %+v", got)
		}
	})

	// Verify that a color the palette does not hold is refused
	t.Run("Missing", func(t *testing.T) {
		if _, ok := color("blueish"); ok {
			t.Error("a color the scanner does not offer was found")
		}
	})
}

// Test_distance tests the distance function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Right: a color further down the list is a positive number of steps
//   - Left: a color further up the list is a negative number of steps
//   - Wraps: the far end of the list is one step from the near one
//   - Unknown: a picker showing a color the palette lacks is reported
func Test_distance(t *testing.T) {
	// Verify that stepping down the list counts up
	t.Run("Right", func(t *testing.T) {
		got, err := distance("Aliceblue", "Aqua")
		if err != nil {
			t.Fatalf("measuring the distance: %v", err)
		}
		if got != 2 {
			t.Errorf("Aliceblue to Aqua is %d steps, wanted 2", got)
		}
	})

	// Verify that stepping up the list counts down
	t.Run("Left", func(t *testing.T) {
		got, err := distance("Aqua", "Aliceblue")
		if err != nil {
			t.Fatalf("measuring the distance: %v", err)
		}
		if got != -2 {
			t.Errorf("Aqua to Aliceblue is %d steps, wanted -2", got)
		}
	})

	// Verify that the list wraps, so no color is more than half a ring away
	t.Run("Wraps", func(t *testing.T) {
		first, last := palette[0].Name, palette[len(palette)-1].Name

		got, err := distance(first, last)
		if err != nil {
			t.Fatalf("measuring the distance: %v", err)
		}
		if got != -1 {
			t.Errorf("%s to %s is %d steps, wanted -1", first, last, got)
		}

		if got, err = distance(last, first); err != nil || got != 1 {
			t.Errorf("%s to %s is %d steps (%v), wanted 1", last, first, got, err)
		}
	})

	// Verify that a picker showing a color the built-in palette does not hold
	// is reported, since the table does not describe that scanner
	t.Run("Unknown", func(t *testing.T) {
		_, err := distance("Puce", "Aqua")
		if err == nil || !strings.Contains(err.Error(), "not a color this tool knows") {
			t.Errorf("an unknown color came back as %v", err)
		}
	})
}

// Test_findArea tests the findArea function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Jumps: the built-in order takes the walk straight to the area
//   - Walks: an area the built-in order does not name is walked to
//   - Falls: an order that does not fit this scanner falls back to the walk
//   - JumpError: a scanner that will not take the key press is reported
func Test_findArea(t *testing.T) {
	// Verify that the built-in order steps straight to the area
	t.Run("Jumps", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		if err := findArea(context.Background(), client, "simple-conventional", "Option_2"); err != nil {
			t.Fatalf("finding the area: %v", err)
		}
		if r.on().title != "Set Option_2 Area" {
			t.Errorf("the walk landed on %q", r.on().title)
		}
	})

	// Verify that an area the built-in order does not name is walked to
	t.Run("Walks", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})

		editor := r.customizeMenu.opens["Set Simple Conventional"]
		editor.opens["Made_up"] = areaViewFor("Made_up")
		editor.rows = append(editor.rows, "Made_up")
		r.stack = []frame{{on: editor}}

		if err := findArea(context.Background(), client, "simple-conventional", "Made_up"); err != nil {
			t.Fatalf("finding the area: %v", err)
		}
		if r.on().title != "Set Made_up Area" {
			t.Errorf("the walk landed on %q", r.on().title)
		}
	})

	// Verify that an order which does not describe this scanner costs time
	// rather than correctness
	t.Run("Falls", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})

		// The editor lists the areas in another order, so the jump lands on
		// the wrong one and the walk has to find it.
		editor := r.customizeMenu.opens["Set Simple Conventional"]
		editor.rows = []string{"Option_2", "Option_1", "Func", "Option_3"}
		r.stack = []frame{{on: editor}}

		if err := findArea(context.Background(), client, "simple-conventional", "Func"); err != nil {
			t.Fatalf("finding the area: %v", err)
		}
		if r.on().title != "Set Func Area" {
			t.Errorf("the walk landed on %q", r.on().title)
		}
	})

	// Verify that a refused key press is reported
	t.Run("JumpError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		r.failOn("KEY,>,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		err := findArea(context.Background(), client, "simple-conventional", "Option_2")
		if err == nil || !strings.Contains(err.Error(), "stepping to the next area") {
			t.Errorf("a refused key came back as %v", err)
		}
	})
}

// Test_index tests the index function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Found: a color's place in the ring is found without regard to case
//   - Missing: a color the palette does not hold is refused
func Test_index(t *testing.T) {
	// Verify that a color's place is found whatever its case
	t.Run("Found", func(t *testing.T) {
		got, ok := index("ALICEBLUE")
		if !ok || got != 0 {
			t.Errorf("the first color is at %d (%t), wanted 0", got, ok)
		}
	})

	// Verify that a color the palette does not hold is refused
	t.Run("Missing", func(t *testing.T) {
		if _, ok := index("Puce"); ok {
			t.Error("a color the palette does not hold was found")
		}
	})
}

// Test_jumpTo tests the jumpTo function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Lands: the area expected there is opened and left on screen
//   - Misses: another area is backed out of and reported as a miss
//   - PressError: a scanner that will not take the key press is reported
//   - EnterError: a scanner that will not open the area is reported
//   - MenuError: an area's menu that cannot be read is reported
func Test_jumpTo(t *testing.T) {
	// Verify that landing on the area wanted leaves the scanner on its menu
	t.Run("Lands", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		landed, err := jumpTo(context.Background(), client, 1, "Option_1")
		if err != nil {
			t.Fatalf("jumping to the area: %v", err)
		}
		if !landed || r.on().title != "Set Option_1 Area" {
			t.Errorf("the jump landed on %q (%t)", r.on().title, landed)
		}
	})

	// Verify that landing on another area backs out and reports the miss
	t.Run("Misses", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})
		editor := r.customizeMenu.opens["Set Simple Conventional"]
		r.stack = []frame{{on: editor}}

		landed, err := jumpTo(context.Background(), client, 1, "Func")
		if err != nil {
			t.Fatalf("jumping to the area: %v", err)
		}
		if landed {
			t.Error("the jump claimed to land on an area it did not")
		}
		if r.on() != editor {
			t.Errorf("the jump did not back out, and is on %q", r.on().title)
		}
	})

	// Verify that a refused key press is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		r.failOn("KEY,>,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		_, err := jumpTo(context.Background(), client, 1, "Option_1")
		if err == nil || !strings.Contains(err.Error(), "stepping to the next area") {
			t.Errorf("a refused key came back as %v", err)
		}
	})

	// Verify that a scanner which will not open the area is reported
	t.Run("EnterError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		r.failOn("KEY,E,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		if _, err := jumpTo(context.Background(), client, 0, "Func"); err == nil {
			t.Error("a refused key was reported as a landing")
		}
	})

	// Verify that an area menu which cannot be read is reported
	t.Run("MenuError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		r.failOn("MSI", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		_, err := jumpTo(context.Background(), client, 0, "Func")
		if err == nil || !strings.Contains(err.Error(), "reading an area's menu") {
			t.Errorf("an unreadable menu came back as %v", err)
		}
	})
}

// Test_orderOf tests the orderOf function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Found: an area's place in the editor comes from the built-in order
//   - Missing: an area the order does not name is reported as unknown
func Test_orderOf(t *testing.T) {
	// Verify that an area's place is found whatever its case
	t.Run("Found", func(t *testing.T) {
		got, ok := orderOf("simple-conventional", "option_1")
		if !ok || got != 1 {
			t.Errorf("Option_1 is at %d (%t), wanted 1", got, ok)
		}
	})

	// Verify that an area the order does not name is reported as unknown
	t.Run("Missing", func(t *testing.T) {
		if _, ok := orderOf("simple-conventional", "Made_up"); ok {
			t.Error("an area the order does not name was found")
		}
	})
}

// Test_renderSet tests the renderSet function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Changed: a color that was written says what it was and what it is
//   - Already: a color that was already right says so and nothing else
//   - Neither: a color that was not asked about is left out
//   - JSON: the JSON form carries both colors
func Test_renderSet(t *testing.T) {
	// Verify that a written color says what it was and what it is now
	t.Run("Changed", func(t *testing.T) {
		app, out, _ := newApp()
		err := renderSet(app, setReport{
			Layout: "weather", Menu: "Set Weather Mode", Area: "Func",
			Text: &change{From: "Yellow", FromHex: "#FFFF00", To: "Red", ToHex: "#FF0000", Changed: true},
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "Yellow #FFFF00 -> Red #FF0000") || !strings.Contains(got, "area:") {
			t.Errorf("the change came back as:\n%s", got)
		}
	})

	// Verify that a color which was already right says so
	t.Run("Already", func(t *testing.T) {
		app, out, _ := newApp()
		err := renderSet(app, setReport{
			Layout: "weather", Menu: "Set Weather Mode", Area: "Func",
			Background: &change{From: "Black", FromHex: "#000000", To: "Black", ToHex: "#000000"},
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "(already)") {
			t.Errorf("a color that was already right came back as:\n%s", out)
		}
	})

	// Verify that a color which was not asked about is left out
	t.Run("Neither", func(t *testing.T) {
		app, out, _ := newApp()
		if err := renderSet(app, setReport{Layout: "weather", Area: "Func"}); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if strings.Contains(out.String(), "text:") || strings.Contains(out.String(), "back:") {
			t.Errorf("a color nobody asked about was reported:\n%s", out)
		}
	})

	// Verify that the JSON form carries both colors
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		err := renderSet(app, setReport{
			Layout: "weather", Area: "Func",
			Text:       &change{To: "Red", ToHex: "#FF0000", Changed: true},
			Background: &change{To: "Black", ToHex: "#000000"},
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		var got setReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the JSON does not parse: %v", err)
		}
		if got.Text == nil || got.Text.To != "Red" || got.Background == nil {
			t.Errorf("the JSON came back as %+v", got)
		}
	})
}

// Test_runSet tests the runSet function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a run with no scanner named is reported
//   - Sets: both colors are written and read back
//   - NoArea: a name the layout has no area for is caught before anything moves
//   - ChooseError: a scanner drawing nothing a layout covers is reported
//   - OpenError: an editor that will not open is reported
//   - FindError: an area that cannot be found is reported
//   - ColorError: a color that will not take is reported by entry
func Test_runSet(t *testing.T) {
	// Verify that a run with no scanner named is reported
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()
		if err := runSet(context.Background(), app, "Func", "Red", "", ""); err == nil {
			t.Error("a run with no scanner reported success")
		}
	})

	// Verify that both colors are written, read back, and cached
	t.Run("Sets", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		r := newRadio("conventional_scan", simpleLines, 2)
		use(app, r)

		remember(quiet(), fakeConn{}.Info(), "simple-conventional",
			[]area{{Name: "Func", Text: "Yellow", Background: "Black"}}, time.Now())

		if err := runSet(context.Background(), app, "Func", "Red", "White", ""); err != nil {
			t.Fatalf("setting the colors: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "Yellow #FFFF00 -> Red #FF0000") {
			t.Errorf("the text color came back as:\n%s", got)
		}
		if !strings.Contains(got, "Black #000000 -> White #FFFFFF") {
			t.Errorf("the background color came back as:\n%s", got)
		}

		// The stored reading is kept true, so the next --cache does not report
		// the color that has just been replaced.
		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading the cache: %v", err)
		}
		stored, ok := c.lookup(scannerKey(fakeConn{}.Info()), "simple-conventional")
		if !ok || stored.Areas[0].Text != "Red" || stored.Areas[0].Background != "White" {
			t.Errorf("the cache still reads as %+v", stored.Areas)
		}
	})

	// Verify that a name the layout has no area for is caught before the
	// scanner is sent anywhere
	t.Run("NoArea", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("conventional_scan", simpleLines, 2)
		use(app, r)

		err := runSet(context.Background(), app, "Made_up", "Red", "", "")
		if err == nil || !strings.Contains(err.Error(), `has no area called "Made_up"`) {
			t.Errorf("an area that does not exist came back as %v", err)
		}
		if len(r.presses) != 0 {
			t.Errorf("the scanner was moved anyway: %v", r.presses)
		}
	})

	// Verify that a scanner drawing nothing a layout covers is reported
	t.Run("ChooseError", func(t *testing.T) {
		isolate(t)
		fast(t)

		app, _, _ := newApp()
		use(app, newRadio("recording", 14, 2))

		if err := runSet(context.Background(), app, "Func", "Red", "", ""); err == nil {
			t.Error("a view no layout covers was written to")
		}
	})

	// Verify that an editor which will not open is reported
	t.Run("OpenError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("conventional_scan", simpleLines, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		use(app, r)

		if err := runSet(context.Background(), app, "Func", "Red", "", "simple-conventional"); err == nil {
			t.Error("an editor that will not open was written to")
		}
	})

	// Verify that an area which cannot be found is reported
	t.Run("FindError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("conventional_scan", simpleLines, 2)
		use(app, r)
		r.fails = func(command string) error {
			if command == "MSI" && r.on() != nil && strings.HasSuffix(r.on().title, " Area") {
				return errors.New("the port is gone")
			}
			return nil
		}

		if err := runSet(context.Background(), app, "Func", "Red", "", "simple-conventional"); err == nil {
			t.Error("an area that could not be found was written to")
		}
	})

	// Verify that a color which will not take is reported by the entry it
	// belongs to
	t.Run("ColorError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("conventional_scan", simpleLines, 2)
		use(app, r)
		r.fails = func(command string) error {
			if command == "STS" && r.on() != nil && r.on().picker {
				return errors.New("the port is gone")
			}
			return nil
		}

		err := runSet(context.Background(), app, "Func", "Red", "", "simple-conventional")
		if err == nil || !strings.Contains(err.Error(), `setting "Set Text Color" for Func`) {
			t.Errorf("a picker that will not read came back as %v", err)
		}
	})

	// Verify that a scanner which will not leave its menus is reported
	t.Run("LeaveError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("conventional_scan", simpleLines, 2)
		r.failOn("KEY,L,P", errors.New("the scanner refused the key"))
		use(app, r)

		err := runSet(context.Background(), app, "Func", "Red", "", "simple-conventional")
		if err == nil || !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("a scanner stuck in its menus came back as %v", err)
		}
	})
}

// Test_setColor tests the setColor function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Writes: the color is set and read back off the picker
//   - Already: a color that is already right is left alone
//   - SelectError: a picker that will not open is reported
//   - PickerError: a picker that shows no color value is reported
//   - Unknown: a color the scanner does not offer is refused
//   - ReadsBackWrong: a color that does not stick is reported rather than
//     claimed
func Test_setColor(t *testing.T) {
	// Verify that the color is set and confirmed off the picker
	t.Run("Writes", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		got, err := setColor(context.Background(), client, textColor, "Red")
		if err != nil {
			t.Fatalf("setting the color: %v", err)
		}
		if !got.Changed || got.From != "Yellow" || got.To != "Red" || got.ToHex != "#FF0000" {
			t.Errorf("the change came back as %+v", got)
		}
		if r.on().title != "Set Func Area" {
			t.Errorf("the walk ended on %q", r.on().title)
		}
	})

	// Verify that a color which is already right writes nothing
	t.Run("Already", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		got, err := setColor(context.Background(), client, backColor, "black")
		if err != nil {
			t.Fatalf("setting the color: %v", err)
		}
		if got.Changed {
			t.Errorf("a color that was already right was written: %+v", got)
		}
		if got.To != "Black" || got.ToHex != "#000000" {
			t.Errorf("the color came back as %+v", got)
		}
	})

	// Verify that a picker which will not open is reported
	t.Run("SelectError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("KEY,E,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		if _, err := setColor(context.Background(), client, textColor, "Red"); err == nil {
			t.Error("a picker that will not open was written to")
		}
	})

	// Verify that a picker showing no color value is reported
	t.Run("PickerError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		editor := &view{title: "Set Weather Mode", rows: []string{"Func"}, opens: map[string]*view{
			"Func": {title: "Set Func Area", rows: []string{textColor}, opens: map[string]*view{
				textColor: {title: textColor, rows: []string{"nothing here"}},
			}},
		}}
		r.stack = []frame{{on: editor}, {on: editor.opens["Func"]}}

		_, err := setColor(context.Background(), client, textColor, "Red")
		if err == nil || !strings.Contains(err.Error(), "no color value") {
			t.Errorf("a picker with no value came back as %v", err)
		}
	})

	// Verify that a color the scanner does not offer is refused
	t.Run("Unknown", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		_, err := setColor(context.Background(), client, textColor, "Puce")
		if err == nil || !strings.Contains(err.Error(), "is not a color the scanner offers") {
			t.Errorf("a color that does not exist came back as %v", err)
		}
	})

	// Verify that a color which does not stick is reported rather than
	// claimed as written
	t.Run("ReadsBackWrong", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		editor := r.customizeMenu.opens["Set Simple Conventional"]
		editor.opens["Func"].opens[textColor].slips = true

		_, err := setColor(context.Background(), client, textColor, "Red")
		if err == nil || !strings.Contains(err.Error(), "reads back as") {
			t.Errorf("a color that did not stick came back as %v", err)
		}
	})

	// Verify that a picker the knob will not move is reported
	t.Run("StepError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")
		r.customizeMenu.opens["Set Simple Conventional"].opens["Func"].opens[textColor].stuck = true

		_, err := setColor(context.Background(), client, textColor, "Red")
		if err == nil || !strings.Contains(err.Error(), "could not get the picker onto") {
			t.Errorf("a stuck picker came back as %v", err)
		}
	})

	// Verify that a scanner which will not take the press that writes the
	// color is reported
	t.Run("ChooseError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")
		r.fails = func(command string) error {
			if command == "KEY,E,P" && r.onPicker() {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		if _, err := setColor(context.Background(), client, textColor, "Red"); err == nil {
			t.Error("a refused key was reported as a written color")
		}
	})

	// Verify that a picker which will not reopen for the read back is
	// reported, since the press is not trusted on its own
	t.Run("ReopenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		written := false
		r.fails = func(command string) error {
			if command == "KEY,E,P" && r.onPicker() {
				written = true
				return nil
			}
			if command == "KEY,E,P" && written {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		if _, err := setColor(context.Background(), client, textColor, "Red"); err == nil {
			t.Error("a picker that would not reopen was reported as read back")
		}
	})

	// Verify that a read back which cannot be made is reported
	t.Run("ReadBackError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		written := false
		r.fails = func(command string) error {
			if command == "KEY,E,P" && r.onPicker() {
				written = true
			}
			if command == "STS" && written && r.onPicker() {
				return errors.New("the port is gone")
			}
			return nil
		}

		if _, err := setColor(context.Background(), client, textColor, "Red"); err == nil {
			t.Error("a picker that could not be read back was reported as set")
		}
	})

	// Verify that a picker which will not close after the read back is
	// reported
	t.Run("BackError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		onArea(r, "Set Simple Conventional", "Func")

		written := false
		r.fails = func(command string) error {
			if command == "KEY,E,P" && r.onPicker() {
				written = true
			}
			if command == "KEY,M,P" && written {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		if _, err := setColor(context.Background(), client, textColor, "Red"); err == nil {
			t.Error("a picker that will not close was reported as set")
		}
	})
}

// Test_stepTo tests the stepTo function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Right: a color further down the ring is stepped right to
//   - Left: a color further up the ring is stepped left to
//   - Already: a picker already showing it turns nothing
//   - PickerError: a picker that cannot be read is reported
//   - Unknown: a picker showing a color the palette lacks is reported
//   - Stuck: a picker that will not move is given up on
func Test_stepTo(t *testing.T) {
	// Verify that a color further down the ring is reached by turning right
	t.Run("Right", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		picker := &view{title: textColor, picker: true, at: colorAt("Aliceblue")}
		r.stack = []frame{{on: picker}}

		if err := stepTo(context.Background(), client, "Aqua"); err != nil {
			t.Fatalf("stepping the picker: %v", err)
		}
		if palette[picker.at].Name != "Aqua" {
			t.Errorf("the picker stopped on %q", palette[picker.at].Name)
		}
	})

	// Verify that a color further up the ring is reached by turning left
	t.Run("Left", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		picker := &view{title: textColor, picker: true, at: colorAt("Aqua")}
		r.stack = []frame{{on: picker}}

		if err := stepTo(context.Background(), client, "Aliceblue"); err != nil {
			t.Fatalf("stepping the picker: %v", err)
		}
		if palette[picker.at].Name != "Aliceblue" {
			t.Errorf("the picker stopped on %q", palette[picker.at].Name)
		}
	})

	// Verify that a picker already showing the color turns nothing
	t.Run("Already", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: &view{title: textColor, picker: true, at: colorAt("Red")}}}

		if err := stepTo(context.Background(), client, "red"); err != nil {
			t.Fatalf("stepping the picker: %v", err)
		}
		if len(r.presses) != 0 {
			t.Errorf("%d keys were pressed, wanted none", len(r.presses))
		}
	})

	// Verify that a picker which cannot be read is reported
	t.Run("PickerError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: &view{title: textColor, picker: true}}}

		if err := stepTo(context.Background(), client, "Red"); err == nil {
			t.Error("an unreadable picker was stepped")
		}
	})

	// Verify that a picker showing a color the palette does not hold is
	// reported
	t.Run("Unknown", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: &view{title: textColor, rows: []string{"Puce", "RGB = CC8899h"}}}}

		err := stepTo(context.Background(), client, "Red")
		if err == nil || !strings.Contains(err.Error(), "not a color this tool knows") {
			t.Errorf("an unknown color came back as %v", err)
		}
	})

	// Verify that a picker which will not move is given up on rather than
	// turned for ever
	t.Run("Stuck", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: &view{title: textColor, picker: true, stuck: true, at: colorAt("Aliceblue")}}}

		err := stepTo(context.Background(), client, "Red")
		if err == nil || !strings.Contains(err.Error(), "could not get the picker onto Red") {
			t.Errorf("a stuck picker came back as %v", err)
		}
	})

	// Verify that a refused key press is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("KEY,>,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: &view{title: textColor, picker: true, at: colorAt("Aliceblue")}}}

		err := stepTo(context.Background(), client, "Aqua")
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Errorf("a refused key came back as %v", err)
		}
	})
}

// Test_suggest tests the suggest function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Starts: colors whose names start with it are named first
//   - Contains: colors whose names hold it are named too
//   - Caps: a word matching half the palette names only the first few
//   - None: a word matching nothing describes the palette instead
func Test_suggest(t *testing.T) {
	// Verify that a color whose name starts with it is named
	t.Run("Starts", func(t *testing.T) {
		got := suggest("orang")
		if !strings.Contains(got, "did you mean") || !strings.Contains(got, "Orange") {
			t.Errorf("the suggestion is %q", got)
		}
	})

	// Verify that a color whose name merely holds it is named too
	t.Run("Contains", func(t *testing.T) {
		got := suggest("chiffon")
		if !strings.Contains(got, "Lemonchiffon") {
			t.Errorf("the suggestion is %q", got)
		}
	})

	// Verify that a word matching a great many colors names only a few
	t.Run("Caps", func(t *testing.T) {
		got := suggest("dark")
		if strings.Count(got, ",") != 5 {
			t.Errorf("the suggestion names more than six colors: %q", got)
		}
	})

	// Verify that a word matching nothing describes the palette instead
	t.Run("None", func(t *testing.T) {
		got := suggest("zzz")
		if !strings.Contains(got, fmt.Sprintf("it offers %d colors", len(palette))) {
			t.Errorf("the suggestion is %q", got)
		}
	})
}

// Test_walkToArea tests the walkToArea function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Finds: the area wanted is opened and left on screen
//   - ComesRound: an area this layout does not hold is reported
//   - EnterError: a scanner that will not open an area is reported
//   - MenuError: an area's menu that cannot be read is reported
//   - PressError: a scanner that will not take the key press is reported
//   - GivesUp: an editor that never comes back round is given up on
func Test_walkToArea(t *testing.T) {
	// Verify that the walk stops on the area wanted
	t.Run("Finds", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		if err := walkToArea(context.Background(), client, "option_2"); err != nil {
			t.Fatalf("walking to the area: %v", err)
		}
		if r.on().title != "Set Option_2 Area" {
			t.Errorf("the walk landed on %q", r.on().title)
		}
	})

	// Verify that coming back round means this layout has no such area
	t.Run("ComesRound", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		err := walkToArea(context.Background(), client, "Made_up")
		if err == nil || !strings.Contains(err.Error(), `no area called "Made_up"`) {
			t.Errorf("an area that does not exist came back as %v", err)
		}
	})

	// Verify that a scanner which will not open an area is reported
	t.Run("EnterError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		r.failOn("KEY,E,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		if err := walkToArea(context.Background(), client, "Func"); err == nil {
			t.Error("a refused key was walked past")
		}
	})

	// Verify that an area menu which cannot be read is reported
	t.Run("MenuError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		r.failOn("MSI", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		err := walkToArea(context.Background(), client, "Func")
		if err == nil || !strings.Contains(err.Error(), "reading an area's menu") {
			t.Errorf("an unreadable menu came back as %v", err)
		}
	})

	// Verify that a refused key press is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		r.failOn("KEY,>,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}

		err := walkToArea(context.Background(), client, "Option_3")
		if err == nil || !strings.Contains(err.Error(), "stepping to the next area") {
			t.Errorf("a refused key came back as %v", err)
		}
	})

	// Verify that an editor which never comes back round is given up on
	t.Run("GivesUp", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)

		editor := &view{title: "Set Simple Conventional", opens: map[string]*view{}}
		for i := 0; i <= maxAreas; i++ {
			name := fmt.Sprintf("Area_%d", i)
			editor.rows = append(editor.rows, name)
			editor.opens[name] = &view{title: "Set " + name + " Area", rows: []string{"Set Something"}}
		}
		r.stack = []frame{{on: editor}}
		client := device.New(fakeConn{reply: r.reply})

		err := walkToArea(context.Background(), client, "Made_up")
		if err == nil || !strings.Contains(err.Error(), "gave up looking for") {
			t.Errorf("an editor that never wraps came back as %v", err)
		}
	})

	// Verify that an area the scanner will not back out of is reported
	t.Run("BackError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 4)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}
		r.fails = func(command string) error {
			if command == "KEY,M,P" && r.onAreaMenu() {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		if err := walkToArea(context.Background(), client, "Option_3"); err == nil {
			t.Error("an area that will not close was walked past")
		}
	})
}
