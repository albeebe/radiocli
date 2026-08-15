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

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// wholePalette returns the palette as a walk of it would report it, starting
// from the color named.
func wholePalette(from string) []paletteColor {
	at, _ := index(from)

	out := make([]paletteColor, 0, len(palette))
	for i := range palette {
		out = append(out, palette[(at+i)%len(palette)])
	}
	return out
}

// Test_comparePalette tests the comparePalette function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Agrees: a scanner offering the built-in palette reports nothing
//   - Empty: a walk that found nothing reports nothing
//   - Unknown: a walk starting on a color the palette lacks reports one
//   - Differs: a color that is not the one expected is reported at its place
//   - Extra: a scanner offering more colors than the palette holds is reported
//   - Short: a color the palette holds and the scanner never showed is reported
func Test_comparePalette(t *testing.T) {
	// Verify that a scanner offering the built-in palette reports nothing,
	// wherever the walk happened to start
	t.Run("Agrees", func(t *testing.T) {
		if got := comparePalette(wholePalette("Green")); len(got) != 0 {
			t.Errorf("a scanner that agrees reported %v", got)
		}
	})

	// Verify that a walk which found nothing reports nothing
	t.Run("Empty", func(t *testing.T) {
		if got := comparePalette(nil); got != nil {
			t.Errorf("an empty walk reported %v", got)
		}
	})

	// Verify that a walk starting on a color the palette lacks is reported as
	// one difference, since there is nothing to line the rings up by
	t.Run("Unknown", func(t *testing.T) {
		got := comparePalette([]paletteColor{{Name: "Puce", Hex: "#CC8899"}})
		if len(got) != 1 || got[0].At != 0 || got[0].Expected != "" {
			t.Fatalf("an unknown starting color reported %v", got)
		}
		if got[0].Found != "Puce #CC8899" {
			t.Errorf("the difference reads %q", got[0].Found)
		}
	})

	// Verify that a color which is not the one expected is reported where it
	// was found
	t.Run("Differs", func(t *testing.T) {
		found := wholePalette("Aliceblue")
		found[3] = paletteColor{Name: "Puce", Hex: "#CC8899"}

		got := comparePalette(found)
		if len(got) != 1 || got[0].At != 3 {
			t.Fatalf("a changed color reported %v", got)
		}
		if got[0].Expected == "" || got[0].Found != "Puce #CC8899" {
			t.Errorf("the difference reads %+v", got[0])
		}
	})

	// Verify that a scanner offering more colors than the palette holds is
	// reported
	t.Run("Extra", func(t *testing.T) {
		found := append(wholePalette("Aliceblue"), paletteColor{Name: "Puce", Hex: "#CC8899"})

		got := comparePalette(found)
		if len(got) != 1 || got[0].At != len(palette) || got[0].Expected != "" {
			t.Errorf("an extra color reported %v", got)
		}
	})

	// Verify that a color the palette holds and the scanner never showed is
	// reported
	t.Run("Short", func(t *testing.T) {
		got := comparePalette(wholePalette("Aliceblue")[:len(palette)-2])
		if len(got) != 2 {
			t.Fatalf("a short walk reported %d differences, wanted 2", len(got))
		}
		if got[0].Found != "" || got[0].Expected == "" {
			t.Errorf("the difference reads %+v", got[0])
		}
	})
}

// Test_describeColor tests the describeColor function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Both: a color renders as its name and its value
//   - Half: a color missing one of the two carries no stray space
func Test_describeColor(t *testing.T) {
	// Verify that a color renders as the report reads it
	t.Run("Both", func(t *testing.T) {
		if got := describeColor(paletteColor{Name: "Red", Hex: "#FF0000"}); got != "Red #FF0000" {
			t.Errorf("the color rendered as %q", got)
		}
	})

	// Verify that a color missing one half carries no stray space
	t.Run("Half", func(t *testing.T) {
		if got := describeColor(paletteColor{Hex: "#FF0000"}); got != "#FF0000" {
			t.Errorf("a nameless color rendered as %q", got)
		}
	})
}

// Test_errPaletteDiffers tests the errPaletteDiffers function with 100%
// coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Counts: the failure says how many colors disagree and out of how many
func Test_errPaletteDiffers(t *testing.T) {
	// Verify that the failure counts the colors it disagrees about
	t.Run("Counts", func(t *testing.T) {
		err := errPaletteDiffers(paletteReport{
			Found:       147,
			Differences: []paletteDifference{{At: 0}, {At: 4}, {At: 9}},
		})
		if err == nil || !strings.Contains(err.Error(), "3 of the scanner's 147 colors") {
			t.Errorf("the failure reads %v", err)
		}
	})
}

// Test_renderPalette tests the renderPalette function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Agrees: a palette that is right says so and reports success
//   - Differs: a palette that is wrong is tabled and reported as a failure
//   - JSONAgrees: the JSON form of a palette that is right reports success
//   - JSONDiffers: the JSON form of a palette that is wrong is still a failure
//   - WriteError: a stream that refuses the table is reported
func Test_renderPalette(t *testing.T) {
	r := paletteReport{
		Borrowed: borrowed{Area: "Func", Color: "Yellow", Hex: "#FFFF00"},
		Found:    len(palette),
		Expected: len(palette),
	}
	wrong := r
	wrong.Differences = []paletteDifference{{At: 3, Expected: "Aquamarine #7BFFCE"}}

	// Verify that a palette which is right says so, and says what was touched
	t.Run("Agrees", func(t *testing.T) {
		app, out, _ := newApp()
		if err := renderPalette(app, r); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "Every color is the one the built-in palette says") {
			t.Errorf("a palette that is right came back as:\n%s", got)
		}
		if !strings.Contains(got, "Func, which was Yellow #FFFF00") {
			t.Errorf("the borrowed area is not reported:\n%s", got)
		}
	})

	// Verify that a palette which is wrong is tabled and fails
	t.Run("Differs", func(t *testing.T) {
		app, out, _ := newApp()

		err := renderPalette(app, wrong)
		if err == nil || !strings.Contains(err.Error(), "1 of the scanner's") {
			t.Errorf("a palette that is wrong came back as %v", err)
		}
		if !strings.Contains(out.String(), "Aquamarine") {
			t.Errorf("the differences came back as:\n%s", out)
		}
	})

	// Verify that the JSON form of a palette which is right reports success
	t.Run("JSONAgrees", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := renderPalette(app, r); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		var got paletteReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the JSON does not parse: %v", err)
		}
		if got.Found != len(palette) || got.Borrowed.Area != "Func" {
			t.Errorf("the JSON came back as %+v", got)
		}
	})

	// Verify that the JSON form of a palette which is wrong is still a failure
	t.Run("JSONDiffers", func(t *testing.T) {
		app, _, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := renderPalette(app, wrong); err == nil {
			t.Error("a palette that is wrong reported success")
		}
	})

	// Verify that a stream which refuses the output is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		if err := renderPalette(app, wrong); err == nil {
			t.Error("a refused stream was reported as written")
		}

		app.Config.Output = appcontext.OutputJSON
		if err := renderPalette(app, r); err == nil {
			t.Error("a refused stream was reported as encoded")
		}
	})
}

// Test_runVerifyPalette tests the runVerifyPalette function with 100%
// coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a run with no scanner named is reported
//   - Agrees: a scanner offering the built-in palette reports success
//   - ChooseError: a scanner drawing nothing a layout covers is reported
//   - WalkError: a picker that cannot be walked is reported
func Test_runVerifyPalette(t *testing.T) {
	// Verify that a run with no scanner named is reported
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()
		if err := runVerifyPalette(context.Background(), app, ""); err == nil {
			t.Error("a run with no scanner reported success")
		}
	})

	// Verify that a scanner offering the built-in palette reports success, and
	// that the walk leaves the borrowed color where it found it
	t.Run("Agrees", func(t *testing.T) {
		app, out, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		use(app, r)

		if err := runVerifyPalette(context.Background(), app, ""); err != nil {
			t.Fatalf("checking the palette: %v", err)
		}
		if !strings.Contains(out.String(), "Every color is the one") {
			t.Errorf("the check came back as:\n%s", out)
		}

		picker := r.customizeMenu.opens["Set Weather Mode"].opens["Func"].opens[textColor]
		if palette[picker.at].Name != "Yellow" {
			t.Errorf("the walk left the picker on %q, wanted Yellow", palette[picker.at].Name)
		}
	})

	// Verify that a scanner drawing nothing a layout covers is reported
	t.Run("ChooseError", func(t *testing.T) {
		fast(t)

		app, _, _ := newApp()
		use(app, newRadio("recording", 14, 2))

		if err := runVerifyPalette(context.Background(), app, ""); err == nil {
			t.Error("a view no layout covers was checked")
		}
	})

	// Verify that a picker which cannot be walked is reported
	t.Run("WalkError", func(t *testing.T) {
		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		use(app, r)

		if err := runVerifyPalette(context.Background(), app, ""); err == nil {
			t.Error("a picker that will not open was walked")
		}
	})

	// Verify that a scanner which will not leave its menus is reported
	t.Run("LeaveError", func(t *testing.T) {
		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("KEY,L,P", errors.New("the scanner refused the key"))
		use(app, r)

		err := runVerifyPalette(context.Background(), app, "")
		if err == nil || !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("a scanner stuck in its menus came back as %v", err)
		}
	})
}

// Test_walkPalette tests the walkPalette function with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - Walks: every color the picker offers comes back, in the knob's order
//   - NoAreas: a layout with no known areas is reported
//   - OpenError: an editor that will not open is reported
//   - FindError: an area that cannot be found is reported
//   - SelectError: a picker that will not open is reported
//   - PickerError: a picker that shows no color value is reported
//   - Stuck: a picker that stops moving is reported
//   - NeverComesRound: a picker that does not come back round is given up on
func Test_walkPalette(t *testing.T) {
	want, _ := lookup("weather")

	// Verify that the whole ring comes back, starting wherever the picker was
	t.Run("Walks", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		area, found, err := walkPalette(context.Background(), client, want)
		if err != nil {
			t.Fatalf("walking the picker: %v", err)
		}
		if area != "Func" {
			t.Errorf("the picker was borrowed from %q", area)
		}
		if len(found) != len(palette) {
			t.Fatalf("%d colors came back, wanted %d", len(found), len(palette))
		}
		if found[0].Name != "Yellow" {
			t.Errorf("the walk started on %q", found[0].Name)
		}
	})

	// Verify that a layout with no known areas is reported rather than walked
	t.Run("NoAreas", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		_, _, err := walkPalette(context.Background(), client, layout{})
		if err == nil || !strings.Contains(err.Error(), "no areas are known") {
			t.Errorf("a layout with no areas came back as %v", err)
		}
	})

	// Verify that an editor which will not open is reported
	t.Run("OpenError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, _, err := walkPalette(context.Background(), client, want); err == nil {
			t.Error("an editor that will not open was walked")
		}
	})

	// Verify that an area which cannot be found is reported
	t.Run("FindError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "MSI" && r.on() != nil && strings.HasSuffix(r.on().title, " Area") {
				return errors.New("the port is gone")
			}
			return nil
		}

		if _, _, err := walkPalette(context.Background(), client, want); err == nil {
			t.Error("an area that could not be found was walked")
		}
	})

	// Verify that a picker which will not open is reported
	t.Run("SelectError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		delete(r.customizeMenu.opens["Set Weather Mode"].opens["Func"].opens, textColor)
		r.customizeMenu.opens["Set Weather Mode"].opens["Func"].rows = []string{backColor}

		_, _, err := walkPalette(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "no entry called") {
			t.Errorf("a missing picker came back as %v", err)
		}
	})

	// Verify that a picker showing no color value is reported
	t.Run("PickerError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.customizeMenu.opens["Set Weather Mode"].opens["Func"].opens[textColor] =
			&view{title: textColor, rows: []string{"nothing here"}}

		_, _, err := walkPalette(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "no color value") {
			t.Errorf("a picker with no value came back as %v", err)
		}
	})

	// Verify that a picker which stops moving is reported, since it should
	// come back round to where it started
	t.Run("Stuck", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.customizeMenu.opens["Set Weather Mode"].opens["Func"].opens[textColor].stuck = true

		_, _, err := walkPalette(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "the picker stopped moving") {
			t.Errorf("a stuck picker came back as %v", err)
		}
	})

	// Verify that a picker which never comes back round is given up on
	t.Run("NeverComesRound", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.customizeMenu.opens["Set Weather Mode"].opens["Func"].opens[textColor].endless = true

		_, _, err := walkPalette(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "did not come back round") {
			t.Errorf("an endless picker came back as %v", err)
		}
	})

	// Verify that a knob which will not turn is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "KEY,>,P" && r.onPicker() {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		_, _, err := walkPalette(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Errorf("a refused key came back as %v", err)
		}
	})

	// Verify that a picker which stops reading part way round is reported
	t.Run("StopsReading", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			// Once the knob has moved off the color the walk started on, which
			// is after the first step of the ring.
			if command == "STS" && r.onPicker() && palette[r.on().at].Name != "Yellow" {
				return errors.New("the port is gone")
			}
			return nil
		}

		if _, _, err := walkPalette(context.Background(), client, want); err == nil {
			t.Error("a picker that stopped reading was walked")
		}
	})
}
