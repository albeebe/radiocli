// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package colors

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
)

// renderAll writes what every layout came back with.
//
// The text form is a line per layout rather than every area of every one of
// them. Seven full tables is the best part of three hundred rows, and what
// somebody who just waited three minutes wants to know is that all seven are
// now read, not what color each area of each of them is. The colors themselves
// are a "colors <layout> --cache" away and cost nothing.
//
// The JSON form is the whole of every reading, because the reader on that end
// is a program and truncating for it would only mean asking seven more times.
// It is an array of exactly what one layout renders as on its own, so anything
// that can read the single form can read this one.
//
// Parameters:
//   - app: application context the rows are written through, and whose output
//     format decides between the table and the JSON
//   - reports: one reading per layout, in the order they were read
//
// Returns:
//   - error if the table cannot be written or the JSON cannot be encoded; nil
//     once every layout has been reported
func renderAll(app *appcontext.App, reports []report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, reports)
	}

	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "LAYOUT\tMENU\tAREAS\tCURRENT")
	for _, r := range reports {
		current := "-"
		if r.Current {
			current = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", r.Layout, r.Menu, len(r.Areas), current)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the layouts: %w", err)
	}

	app.Printf("\n%d layouts read\n", len(reports))
	return nil
}

// runAll reads every layout in turn and caches each.
//
// One command rather than seven, because a cache holding one layout is the
// thing this exists to prevent: a mirror draws whatever the scanner is showing,
// and a layout nobody read is drawn white on black while the radio in your hand
// is in color. Reading the one on screen leaves the other six that way until
// somebody happens to be looking at each of them, which is not a thing anybody
// would think to do.
//
// It is also one turn on the scanner rather than seven. A macro of seven steps
// would let a command from somewhere else land between two of them, and would
// take the radio into the menus and back out seven times over.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the several minutes
//     the seven walks take
//   - app: application context holding the scanner connection, the output
//     format and the streams the progress and the table are written to
//
// Returns:
//   - error if the scanner cannot be reached, any layout's walk fails, the
//     scanner will not leave its menus, or the report cannot be written; nil
//     once all seven layouts are read, cached and reported
func runAll(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Which layout is on screen has to be settled before any menu is opened,
	// because opening one is itself a change of view: from inside a menu the
	// scanner reports the menu rather than the screen it came from.
	//
	// A scanner already in a menu, or on a screen no layout covers, has no
	// current layout, and that is not a failure here. Every layout is read
	// either way, and the only thing the answer decides is which one gets its
	// soft keys placed from the live screen.
	on := ""
	if want, err := currentLayout(ctx, client); err == nil {
		on = want.name
	}

	app.Notef("Reading all %d layouts from the scanner's menus. "+
		"This takes a few minutes and stops the scan while it runs.\n", len(layouts))

	reports := make([]report, 0, len(layouts))
	for i, want := range layouts {
		// Counted, because the only thing worse than a command that takes
		// three minutes is one that takes three minutes in silence.
		app.Notef("[%d/%d] %s\n", i+1, len(layouts), want.entry)

		areas, err := readLayout(ctx, client, want)
		if err != nil {
			// Named, because "reading the colors failed" after two minutes of
			// walking does not say which of the seven walks it was.
			return fmt.Errorf("reading %s: %w", want.entry, err)
		}

		read := time.Now()
		remember(app, client.Info(), want.name, areas, read)

		reports = append(reports, report{
			Layout:  want.name,
			Menu:    want.entry,
			Current: want.name == on,
			Read:    read,
			Areas:   areas,
		})
	}

	// Out of the menus before anything is placed, because the soft keys of the
	// layout on screen are read off the live screen and there is no screen to
	// read while the scanner is still in a menu.
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}
	for i := range reports {
		if err := place(ctx, client, layouts[i], reports[i].Areas, reports[i].Current); err != nil {
			return err
		}
	}

	return renderAll(app, reports)
}
