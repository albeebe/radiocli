// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package colors

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
)

// comparePalette lines up what the scanner offered against the built-in
// palette.
//
// The walk starts wherever the picker happened to be, so the two are compared
// as rings: the starting color is found in the palette and everything is
// counted from there.
//
// Parameters:
//   - found: the colors the scanner offered, in the order the knob stepped
//     through them
//
// Returns:
//   - []paletteDifference holding every place the two disagree, counted from
//     where the walk started, empty when the palette is right; a walk whose
//     first color is not in the palette at all is reported as a single
//     difference, since there is nothing to line the rings up by
func comparePalette(found []paletteColor) []paletteDifference {
	if len(found) == 0 {
		return nil
	}

	at, ok := index(found[0].Name)
	if !ok {
		return []paletteDifference{{
			At:    0,
			Found: describeColor(found[0]),
		}}
	}

	var out []paletteDifference
	for i, got := range found {
		if i >= len(palette) {
			out = append(out, paletteDifference{At: i, Found: describeColor(got)})
			continue
		}

		expected := palette[(at+i)%len(palette)]
		if got != expected {
			out = append(out, paletteDifference{
				At:       i,
				Expected: describeColor(expected),
				Found:    describeColor(got),
			})
		}
	}

	// A color the palette holds and the scanner never showed.
	for i := len(found); i < len(palette); i++ {
		out = append(out, paletteDifference{
			At:       i,
			Expected: describeColor(palette[(at+i)%len(palette)]),
		})
	}

	return out
}

// describeColor renders a color for the report.
//
// Parameters:
//   - c: the color to renderReport
//
// Returns:
//   - string holding its name and value as "Name #RRGGBB", trimmed so a color
//     missing one of the two does not carry a stray space
func describeColor(c paletteColor) string {
	return strings.TrimSpace(c.Name + " " + c.Hex)
}

// errPaletteDiffers is the failure a check reports when the palette is wrong,
// so a script running this notices rather than reading a report nobody looks at.
//
// Parameters:
//   - r: the check's result, which has already been rendered
//
// Returns:
//   - error saying how many of the scanner's colors are not what the built-in
//     palette says and where that palette came from
func errPaletteDiffers(r paletteReport) error {
	return fmt.Errorf("%d of the scanner's %d colors are not what the built-in palette says: "+
		"it was walked off firmware 1.00.37 and this scanner disagrees",
		len(r.Differences), r.Found)
}

// renderPalette writes the result of the check.
//
// Parameters:
//   - app: application context the table is written through, and whose output
//     format decides between the table and the JSON
//   - r: the check's result
//
// Returns:
//   - error if the table cannot be written, the JSON cannot be encoded, or the
//     scanner disagreed with the built-in palette, which is reported as a
//     failure so a script notices; nil when every color is the one it says
func renderPalette(app *appcontext.App, r paletteReport) error {
	if app.Config.Output == appcontext.OutputJSON {
		if err := render.JSON(app.Stdout, r); err != nil {
			return err
		}
		if len(r.Differences) > 0 {
			return errPaletteDiffers(r)
		}
		return nil
	}

	app.Printf("palette: %d colors, and the built-in one holds %d\n", r.Found, r.Expected)
	app.Printf("read on: %s, which was %s %s and still should be\n",
		r.Borrowed.Area, r.Borrowed.Color, r.Borrowed.Hex)

	if len(r.Differences) == 0 {
		app.Printf("\nEvery color is the one the built-in palette says it is.\n")
		return nil
	}

	app.Printf("\n")
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AT\tBUILT IN\tON THE SCANNER")
	for _, d := range r.Differences {
		fmt.Fprintf(w, "%d\t%s\t%s\n", d.At, render.Dash(d.Expected), render.Dash(d.Found))
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the differences: %w", err)
	}

	return errPaletteDiffers(r)
}

// runVerifyPalette checks the built-in palette against the scanner.
//
// The palette is a table for the same reason the screen map is, and a table can
// go stale without saying so. This walks one picker end to end and compares
// what the scanner offers against what is built in.
//
// It is gentler than the screen map's check in what it protects against. The
// palette is only used to work out how far and which way to turn the knob, and
// the walk that sets a color reads the screen and compares by name before it
// commits, so a wrong palette makes a set fail rather than write the wrong
// color. What this catches is the slow rot: a firmware that added, dropped or
// re-valued a color, which would otherwise be found only by somebody's set
// command failing for no apparent reason.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk
//   - app: application context holding the scanner connection, the output
//     format and the streams the result is written to
//   - wanted: the layout whose picker to borrow, or empty for the one on
//     screen
//
// Returns:
//   - error if the scanner cannot be reached, no layout can be settled on, the
//     picker cannot be walked, the scanner will not leave its menus, or the
//     palette and the scanner disagree; nil when every color is the one the
//     built-in palette says
func runVerifyPalette(ctx context.Context, app *appcontext.App, wanted string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	want, _, err := choose(ctx, client, wanted)
	if err != nil {
		return err
	}

	app.Notef("Walking a color picker on %s. This stops the scan for a moment.\n", want.entry)

	area, found, err := walkPalette(ctx, client, want)
	if err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	return renderPalette(app, paletteReport{
		Borrowed: borrowed{
			Area:  area,
			Color: found[0].Name,
			Hex:   found[0].Hex,
		},
		Found:       len(found),
		Expected:    len(palette),
		Differences: comparePalette(found),
	})
}

// walkPalette steps one picker all the way round, recording what it offers.
//
// It never presses enter, so nothing is written: the picker is left by the menu
// key, which abandons the knob's position and keeps the stored color.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk
//   - client: the open scanner connection to press keys and read through
//   - want: the layout whose first area lends its picker
//
// Returns:
//   - string naming the area whose picker was borrowed, so the report can say
//     what was touched and what its color should still be
//   - []paletteColor holding every color the picker offered, starting from
//     wherever it happened to be
//   - error if no areas are known for the layout, the editor or the picker
//     will not open or read, the scanner will not take a key press, the picker
//     stops moving, or it never comes back round to where it started
func walkPalette(ctx context.Context, client *device.Scanner, want layout) (string, []paletteColor, error) {
	order := areaOrder[want.name]
	if len(order) == 0 {
		return "", nil, fmt.Errorf("no areas are known for %s", want.name)
	}

	if err := open(ctx, client, displayOptions, customize, want.entry); err != nil {
		return "", nil, err
	}
	// Any area's picker offers the same colors, so this uses the first, which
	// is where the editor already is.
	area := order[0]
	if err := findArea(ctx, client, want.name, area); err != nil {
		return "", nil, err
	}
	if err := menus.Select(ctx, client, textColor); err != nil {
		return "", nil, err
	}

	name, hex, err := readPicker(ctx, client)
	if err != nil {
		return "", nil, err
	}

	start := paletteColor{Name: name, Hex: hex}
	found := []paletteColor{start}

	for step := 0; step < maxPaletteWalk; step++ {
		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return "", nil, fmt.Errorf("turning the knob: %w", err)
		}

		name, hex, err := readPicker(ctx, client)
		if err != nil {
			return "", nil, err
		}
		got := paletteColor{Name: name, Hex: hex}

		if got == found[len(found)-1] {
			return "", nil, fmt.Errorf("the picker stopped moving at %s: it should come back "+
				"round to where it started", got.Name)
		}
		if got == start {
			return area, found, nil
		}
		found = append(found, got)
	}

	return "", nil, fmt.Errorf("the picker did not come back round to %s after %d colors",
		start.Name, maxPaletteWalk)
}
