// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package colors

import (
	"context"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
)

// areaUnderCursor opens the area the editor is on to read its name, and comes
// straight back out.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while driving the editor
//   - client: the open scanner connection to press keys and read through
//
// Returns:
//   - string holding the scanner's name for that area
//   - error if the area will not open, its menu cannot be read, or the scanner
//     will not back out of it
func areaUnderCursor(ctx context.Context, client *device.Scanner) (string, error) {
	if err := menus.Enter(ctx, client); err != nil {
		return "", err
	}

	info, err := client.MenuInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("reading an area's menu: %w", err)
	}

	if err := menus.Back(ctx, client); err != nil {
		return "", err
	}
	return areaName(info.Title), nil
}

// choose settles which layout a run is about, and whether it is the one in use.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - client: the open scanner connection to ask what it is doing
//   - wanted: the layout that was named, or empty for the one on screen
//
// Returns:
//   - layout the run is about, zero when none was named and none could be
//     settled on
//   - bool reporting whether that layout is the one the scanner is drawing
//     with, which is left false rather than failing when it cannot be told
//   - error if no layout was named and the one on screen cannot be worked out
func choose(ctx context.Context, client *device.Scanner, wanted string) (layout, bool, error) {
	if wanted == "" {
		want, err := currentLayout(ctx, client)
		return want, true, err
	}

	want, _ := lookup(wanted)
	current, _ := isCurrent(ctx, client, want)
	return want, current, nil
}

// compare matches what the scanner drew against the built-in map.
//
// Parameters:
//   - want: the layout being checked, which names the map to compare against
//   - found: where the scanner drew each area, keyed by the area's name
//
// Returns:
//   - []difference holding every area the two disagree about, in name order,
//     empty when the map is right; the soft keys are left out, since the map
//     leaves them out on purpose
func compare(want layout, found map[string]position) []difference {
	known := positions[want.name]

	var out []difference
	for name, at := range found {
		at := at
		expected, ok := known[name]
		if !ok {
			// The soft keys are absent from the map on purpose, and the editor
			// cannot say where they are either, so they are not a difference.
			if isSoftKey(name) {
				continue
			}
			out = append(out, difference{Area: name, Found: &at})
			continue
		}
		if expected != at {
			expected := expected
			out = append(out, difference{Area: name, Expected: &expected, Found: &at})
		}
	}

	// An area the map places and the scanner never showed is just as wrong as
	// one in the wrong place.
	for name, at := range known {
		if _, ok := found[name]; !ok {
			at := at
			out = append(out, difference{Area: name, Expected: &at})
		}
	}

	sortDifferences(out)
	return out
}

// describe renders a position for the table.
//
// Parameters:
//   - at: where the area sits, or nil when there is nothing to describe
//
// Returns:
//   - string holding the line, column and width, or "-" when there is no
//     position
func describe(at *position) string {
	if at == nil {
		return "-"
	}
	return fmt.Sprintf("line %d, col %d, %d wide", at.Line, at.Column, at.Length)
}

// errDiffers is the failure a check reports when the map is wrong, so that a
// script running this notices rather than reading a report nobody looks at.
//
// Parameters:
//   - r: the check's result, which has already been rendered
//
// Returns:
//   - error saying how many of the layout's areas disagree with the built-in
//     map and where that map came from
func errDiffers(r verifyReport) error {
	return fmt.Errorf("%d of %d areas of %s are not where the built-in map says: "+
		"the map was read off firmware 1.00.37 and this scanner disagrees",
		len(r.Differences), r.Checked, r.Layout)
}

// isSoftKey reports whether an area is one of the five along the bottom row,
// which the built-in map deliberately leaves out.
//
// Parameters:
//   - name: the area's name, as the scanner gives it
//
// Returns:
//   - bool reporting whether it is one of the three keys or the two gaps
//     between them
func isSoftKey(name string) bool {
	switch name {
	case "Soft1_key", "Space_1", "Soft2_key", "Space_2", "Soft3_key":
		return true
	}
	return false
}

// merge folds an area drawn across several rows into one span.
//
// Nine areas are taller than one row: the scanner draws some names in a large
// font that occupies two, and gives three layouts a detail area four tall.
// Those arrive as one identical run per row, so a run of rows at the same
// column and width is one area that tall.
//
// Anything else is reported as unreadable rather than guessed at. A screen
// caught mid redraw has no reverse video at all, or has it in two places, and
// that is a moment rather than a state.
//
// Parameters:
//   - spans: the runs of reverse video the screen holds, in row order
//
// Returns:
//   - position holding the one area those rows make up, as tall as there are
//     rows, zero when they do not make one
//   - bool reporting whether the screen was readable, false when there was no
//     reverse video or it was not one area stacked up
//   - error, always nil, because folding rows that are already read cannot
//     fail
func merge(spans []position) (position, bool, error) {
	if len(spans) == 0 {
		return position{}, false, nil
	}

	first := spans[0]
	for i, s := range spans {
		if s.Column != first.Column || s.Length != first.Length || s.Line != first.Line+i {
			return position{}, false, nil
		}
	}

	first.Height = len(spans)
	return first, true, nil
}

// placed returns a layout's areas from the built-in map, in reading order.
//
// Parameters:
//   - want: the layout whose map to read
//
// Returns:
//   - []area holding every area the map places, sorted down the screen and
//     then across it, with no colors on them
func placed(want layout) []area {
	known := positions[want.name]
	out := make([]area, 0, len(known))
	for name, at := range known {
		out = append(out, area{Name: name, Line: at.Line, Column: at.Column,
			Length: at.Length, Height: at.Height})
	}
	sortAreas(out)
	return out
}

// renderVerify writes the result of the check.
//
// Parameters:
//   - app: application context the table is written through, and whose output
//     format decides between the table and the JSON
//   - r: the check's result
//
// Returns:
//   - error if the table cannot be written, the JSON cannot be encoded, or the
//     scanner disagreed with the built-in map, which is reported as a failure
//     so a script notices; nil when every area is where the map says
func renderVerify(app *appcontext.App, r verifyReport) error {
	if app.Config.Output == appcontext.OutputJSON {
		if err := render.JSON(app.Stdout, r); err != nil {
			return err
		}
		if len(r.Differences) > 0 {
			return errDiffers(r)
		}
		return nil
	}

	app.Printf("layout:  %s (%s)\n", r.Layout, r.Menu)
	app.Printf("checked: %d areas\n", r.Checked)

	if len(r.Differences) == 0 {
		app.Printf("\nEvery area is where the built-in map says it is.\n")
		return nil
	}

	app.Printf("\n")
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AREA\tBUILT IN\tON THE SCANNER")
	for _, d := range r.Differences {
		fmt.Fprintf(w, "%s\t%s\t%s\n", d.Area, describe(d.Expected), describe(d.Found))
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the differences: %w", err)
	}

	return errDiffers(r)
}

// reverseRuns finds the runs of reverse video in an attribute string. The line
// of each is left at zero for the caller to fill in.
//
// Parameters:
//   - attributes: how each character of a row is drawn, one character per
//     character of the row, where "*" is reverse video
//
// Returns:
//   - []position holding the column and width of every run, empty for a row
//     drawn entirely normally
func reverseRuns(attributes string) []position {
	var out []position

	start := -1
	for i, r := range attributes {
		switch {
		case r == '*' && start < 0:
			start = i
		case r != '*' && start >= 0:
			out = append(out, position{Column: start, Length: i - start})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, position{Column: start, Length: len(attributes) - start})
	}
	return out
}

// runPositions reports where a layout's areas sit, and nothing else.
//
// This is the fast path and the reason the map is built in: it answers from a
// table, opens no menus, and leaves the scanner scanning. Anything redrawing
// the scanner's screen wants this on every frame, which a menu walk could never
// serve.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while asking the scanner what
//     it is drawing
//   - app: application context holding the scanner connection, the output
//     format and the streams the table is written to
//   - wanted: the layout to report, or empty for the one on screen
//
// Returns:
//   - error if the scanner cannot be reached, no layout can be settled on, the
//     soft keys cannot be read, or the table cannot be written; nil once the
//     positions have been reported
func runPositions(ctx context.Context, app *appcontext.App, wanted string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	want, current, err := choose(ctx, client, wanted)
	if err != nil {
		return err
	}

	areas := placed(want)
	if current {
		// The soft keys are not in the map and cannot be: their widths follow
		// whatever labels the current mode is showing. The live row says where
		// they are, so they are filled in whenever the layout being reported is
		// the one on screen.
		keys, err := softKeys(ctx, client)
		if err != nil {
			return err
		}
		areas = append(areas, keys...)
	}

	return renderReport(app, report{
		Layout:  want.name,
		Menu:    want.entry,
		Current: current,
		Areas:   areas,
	})
}

// runVerify checks the built-in positions against the scanner.
//
// The map is the firmware's geometry rather than the user's settings, so it is
// hardcoded. That is only safe if it can be checked, because a firmware that
// moved something would otherwise make every drawing of the screen quietly
// wrong in a way nothing reports.
//
// It walks the layout's editor, which draws the selected area in reverse video,
// and compares the span the scanner highlights against the span the map holds.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk
//   - app: application context holding the scanner connection, the output
//     format and the streams the result is written to
//   - wanted: the layout to check, or empty for the one on screen
//
// Returns:
//   - error if the scanner cannot be reached, no layout can be settled on, the
//     editor cannot be walked, the scanner will not leave its menus, or the
//     map and the scanner disagree; nil when every area is where the map says
func runVerify(ctx context.Context, app *appcontext.App, wanted string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	want, current, err := choose(ctx, client, wanted)
	if err != nil {
		return err
	}

	app.Notef("Checking %s against the scanner. This stops the scan for a moment.\n", want.entry)

	found, err := walkPositions(ctx, client, want)
	if err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	return renderVerify(app, verifyReport{
		Layout:      want.name,
		Menu:        want.entry,
		Current:     current,
		Checked:     len(found),
		Differences: compare(want, found),
	})
}

// selectedSpan reads the span the editor is drawing in reverse video.
//
// The bottom row is left out. It is reverse video at all times, so the area
// selected there cannot be told apart from the row itself, which is exactly why
// the soft keys are read off the live screen instead.
//
// A screen with anything other than one span is reported as unreadable rather
// than guessed at: the scanner redraws as it likes, and a half drawn screen is
// a moment rather than a state.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while reading the screen
//   - client: the open scanner connection to read the display through
//
// Returns:
//   - position holding where the editor is drawing the selected area, zero
//     when the screen cannot be read that way
//   - bool reporting whether the span was readable
//   - error if the screen cannot be read at all
func selectedSpan(ctx context.Context, client *device.Scanner) (position, bool, error) {
	d, err := client.Display(ctx)
	if err != nil {
		return position{}, false, fmt.Errorf("reading the screen: %w", err)
	}

	var spans []position
	for i, l := range d.Lines {
		if i == len(d.Lines)-1 {
			continue
		}
		for _, run := range reverseRuns(l.Attributes) {
			run.Line = i
			run.Height = 1
			spans = append(spans, run)
		}
	}

	return merge(spans)
}

// softKeys reads the three keys along the bottom of the live screen.
//
// The row is entirely reverse video, drawn as three runs with a single normal
// character between them. Those five regions are the five areas the scanner
// names for that row, in order, so splitting the row on its runs gives their
// positions without a table and without caring what the labels say.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while reading the screen
//   - client: the open scanner connection to read the display through
//
// Returns:
//   - []area holding the three keys and the two gaps between them, empty when
//     the screen is empty or its bottom row is not three runs of reverse video
//   - error if the screen cannot be read
func softKeys(ctx context.Context, client *device.Scanner) ([]area, error) {
	d, err := client.Display(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the screen: %w", err)
	}
	if len(d.Lines) == 0 {
		return nil, nil
	}

	line := len(d.Lines) - 1
	runs := reverseRuns(d.Lines[line].Attributes)
	if len(runs) != 3 {
		// Some screens do not draw three keys. Reporting none is better than
		// reporting a guess at which is which.
		return nil, nil
	}

	// The row is one tall, whatever else is on the screen.
	names := []string{"Soft1_key", "Space_1", "Soft2_key", "Space_2", "Soft3_key"}
	regions := []position{
		runs[0],
		{Line: line, Column: runs[0].Column + runs[0].Length, Length: runs[1].Column - (runs[0].Column + runs[0].Length)},
		runs[1],
		{Line: line, Column: runs[1].Column + runs[1].Length, Length: runs[2].Column - (runs[1].Column + runs[1].Length)},
		runs[2],
	}

	out := make([]area, 0, len(names))
	for i, name := range names {
		out = append(out, area{
			Name:   name,
			Line:   line,
			Column: regions[i].Column,
			Length: regions[i].Length,
			Height: 1,
		})
	}
	return out, nil
}

// sortAreas puts areas in reading order: down the screen, then across it.
//
// The map is a map, so it comes out in no order at all. Reading order is the
// one that makes the table look like the screen.
//
// Parameters:
//   - areas: the areas to sort, reordered in place, with the name breaking a
//     tie so two runs of the same layout come out the same way
func sortAreas(areas []area) {
	sort.Slice(areas, func(i, j int) bool {
		if areas[i].Line != areas[j].Line {
			return areas[i].Line < areas[j].Line
		}
		if areas[i].Column != areas[j].Column {
			return areas[i].Column < areas[j].Column
		}
		return areas[i].Name < areas[j].Name
	})
}

// sortDifferences puts differences in a stable order, by name, since an area
// the scanner never showed has no position to sort by.
//
// Parameters:
//   - out: the differences to sort, reordered in place
func sortDifferences(out []difference) {
	sort.Slice(out, func(i, j int) bool { return out[i].Area < out[j].Area })
}

// walkPositions steps through a layout's editor and records the span the
// scanner draws in reverse video for each area.
//
// The span is read before the area is opened, because opening it replaces the
// editor's drawing with a menu. The name comes from opening it, since the
// editor's own rows are the layout's artwork rather than names.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk
//   - client: the open scanner connection to press keys and read through
//   - want: the layout whose editor to walk
//
// Returns:
//   - map[string]position holding where the scanner drew each area it named,
//     keyed by that name, leaving out the ones whose span was unreadable
//   - error if the editor will not open, the screen or a menu cannot be read,
//     the scanner will not take a key press, or the walk never comes back
//     round to where it started
func walkPositions(ctx context.Context, client *device.Scanner, want layout) (map[string]position, error) {
	if err := open(ctx, client, displayOptions, customize, want.entry); err != nil {
		return nil, err
	}

	found := map[string]position{}
	first := ""

	for len(found) < maxAreas {
		at, ok, err := selectedSpan(ctx, client)
		if err != nil {
			return nil, err
		}

		name, err := areaUnderCursor(ctx, client)
		if err != nil {
			return nil, err
		}
		if name == first {
			return found, nil
		}
		if first == "" {
			first = name
		}
		if ok {
			found[name] = at
		}

		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return nil, fmt.Errorf("stepping to the next area: %w", err)
		}
	}

	return nil, fmt.Errorf("%s did not come back round to %q after %d areas",
		want.entry, first, maxAreas)
}
