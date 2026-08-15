// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package colors

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/spf13/cobra"
)

// newReset returns the "colors reset" subcommand.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that puts one layout, or all seven, back to the
//     scanner's default colors when it runs
func newReset(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset [layout]",
		Short: "Put a layout's colors back to the scanner's defaults",
		Long: "Reset puts the colors of the scanner's screen back to the ones it left the\n" +
			"factory with, undoing every change made with \"radiocli colors set\" or by\n" +
			"hand on the radio.\n\n" +
			"With no layout it restores all seven at once, which is the scanner's own\n" +
			"\"All Screens\". Name one to restore only that one:\n\n" +
			"  " + strings.Join(layoutNames(), ", ") + "\n\n" +
			"This drives the scanner's own restore rather than writing colors back one\n" +
			"at a time, so what it puts there is whatever that radio calls stock rather\n" +
			"than a table kept in this tool.\n\n" +
			"It writes to the scanner and stops the scan for a few seconds. The scanner\n" +
			"asks for confirmation and this answers yes, so there is nothing to press.\n" +
			"There is no undo: colors changed before this are gone.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var want string
			if len(args) == 1 {
				l, ok := lookup(args[0])
				if !ok {
					return fmt.Errorf("%q is not a layout: want %s",
						args[0], strings.Join(layoutNames(), ", "))
				}
				want = l.name
			}
			return runReset(cmd.Context(), app, want)
		},
	}
	return cmd
}

// confirmRestore checks that the scanner is asking to confirm, and says yes.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while reading and pressing
//   - client: the open scanner connection to read the screen and press through
//   - entry: the Restore menu entry that was opened, which has to be named on
//     the prompt for it to be this restore
//
// Returns:
//   - error if the screen cannot be read, is not the confirmation for this
//     entry, or will not take the press; nil once the restore is confirmed
func confirmRestore(ctx context.Context, client *device.Scanner, entry string) error {
	d, err := client.Display(ctx)
	if err != nil {
		return fmt.Errorf("reading the confirmation: %w", err)
	}

	var shown strings.Builder
	for _, l := range d.Lines {
		shown.WriteString(l.Text)
		shown.WriteString("\n")
	}
	text := shown.String()

	// Both halves are checked. The heading alone would match a confirmation
	// for something else if the walk went wrong, and the entry alone appears
	// on the menu this came from.
	if !strings.Contains(text, "Confirm Restore?") || !strings.Contains(text, entry) {
		return fmt.Errorf("the scanner did not ask to confirm restoring %s, so nothing was restored", entry)
	}

	if err := menus.Enter(ctx, client); err != nil {
		return fmt.Errorf("confirming the restore: %w", err)
	}
	return nil
}

// renderReset writes what was restored.
//
// Parameters:
//   - app: application context the lines are written through, and whose output
//     format decides between the text and the JSON
//   - r: what the restore covered and what was read back afterwards
//
// Returns:
//   - error if the JSON cannot be encoded; nil once the restore has been
//     reported
func renderReset(app *appcontext.App, r resetReport) error {
	if app.Config.Output == appcontext.OutputJSON {
		out, err := marshalJSON(r, "", "  ")
		if err != nil {
			return err
		}
		app.Printf("%s\n", out)
		return nil
	}

	if len(r.Layouts) == 1 {
		app.Printf("Restored %s to the scanner's default colors.\n", r.Layouts[0])
	} else {
		app.Printf("Restored all %d layouts to the scanner's default colors:\n", len(r.Layouts))
		for _, name := range r.Layouts {
			app.Printf("  %s\n", name)
		}
	}

	// Which of them is known is worth saying, because the rest are not: their
	// colors are back to stock on the radio and nothing here has read them, so
	// anything drawing them falls back to white on black until it does.
	if r.Reread != "" {
		app.Printf("Read %s back, so it is the one drawn in its real colors.\n", r.Reread)
	}
	return nil
}

// rereadCurrent reads back whichever restored layout the scanner is drawing
// with, and returns its name. Empty means there was nothing worth reading.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk
//   - app: application context the progress is written through and the new
//     reading is cached with
//   - client: the open scanner connection to walk the menus through
//   - covered: the layouts the restore covered, since a layout it did not
//     touch is not worth the walk
//
// Returns:
//   - string naming the layout that was read back, empty when the one on
//     screen was not among those restored
//   - error if the layout on screen cannot be settled on, the walk fails, or
//     the scanner will not leave its menus
func rereadCurrent(ctx context.Context, app *appcontext.App, client *device.Scanner, covered []string) (string, error) {
	want, _, err := choose(ctx, client, "")
	if err != nil {
		return "", err
	}

	// Restoring a layout the scanner is not showing changes nothing anybody is
	// looking at, so it buys nothing to spend the walk here. What it left
	// uncached is read the first time something wants it.
	if !slices.Contains(covered, want.name) {
		return "", nil
	}

	app.Notef("Reading %s back so the new colors are known. This takes a moment longer.\n", want.entry)

	areas, err := readLayout(ctx, client, want)
	if err != nil {
		return "", err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return "", err
	}

	remember(app, client.Info(), want.name, areas, time.Now())
	return want.name, nil
}

// restoreEntryFor returns the Restore menu entry that restores a layout, and
// the layouts that entry covers.
//
// The Restore menu names a layout the way the Customize menu does with "Set "
// taken off the front, and lists them in the same order, so one table serves
// both rather than a second copy that could drift from it.
//
// Parameters:
//   - layoutName: the layout to restore, or empty for all of them
//
// Returns:
//   - string naming the Restore menu entry to open, which is "All Screens"
//     when no layout was named
//   - []string holding the layouts that entry covers, which is all seven for
//     "All Screens" and the named one otherwise
func restoreEntryFor(layoutName string) (string, []string) {
	if layoutName == "" {
		return allScreens, layoutNames()
	}

	l, _ := lookup(layoutName)
	return strings.TrimPrefix(l.entry, "Set "), []string{l.name}
}

// runReset restores one layout, or all of them.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the restore and the
//     reading that follows it
//   - app: application context holding the scanner connection, the output
//     format and the streams the progress and the result are written to
//   - layoutName: the layout to restore, or empty for all seven
//
// Returns:
//   - error if the scanner cannot be reached, the Restore menu cannot be
//     walked, the confirmation is not the one expected, or the result cannot
//     be written; nil once the restore is done, whether or not the colors
//     could be read back afterwards
func runReset(ctx context.Context, app *appcontext.App, layoutName string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	entry, covered := restoreEntryFor(layoutName)

	app.Notef("Restoring %s to the scanner's default colors. This stops the scan for a moment.\n", entry)

	if err := open(ctx, client, displayOptions, customize, restoreEntry, entry); err != nil {
		return err
	}

	// The scanner asks before it does it: "Confirm Restore?", with the entry
	// named on the line below and "Yes=\"E\" / No=\".\"" at the bottom. Opening
	// the entry is therefore not the restore, and this press is.
	//
	// The screen is read first rather than pressing on trust. A press of "E"
	// against something that is not this prompt is a press into whatever the
	// walk actually landed on, and this one is not recoverable by backing out.
	if err := confirmRestore(ctx, client, entry); err != nil {
		return fmt.Errorf("%w\nRun \"radiocli scan\" to return the scanner to scanning", err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Every layout this restored now reads differently from whatever was
	// cached for it, and unlike setting one color there is nothing read back
	// to store instead. So the readings are dropped rather than amended.
	forget(app, client.Info(), covered)

	r := resetReport{Entry: entry, Layouts: covered}

	// Dropping them is not enough on its own. Nothing anywhere knows the new
	// colors, because the scanner will not report a color except by having its
	// menus walked, so anything drawing the screen from the cache has just gone
	// from stale colors to none at all. Anything redrawing the screen live is
	// the case that matters: resetting made the radio change and the drawing
	// fall back to white on black, which looks like the reset did nothing.
	//
	// So the layout the scanner is drawing with is read back, exactly as
	// "colors" would read it. Only that one: it is the one being looked at, and
	// the walk is half a minute apiece. The rest stay uncached and are read
	// when something asks for them.
	if reread, err := rereadCurrent(ctx, app, client, covered); err != nil {
		// The restore itself worked, and saying so is more useful than failing
		// over the reading that came after it.
		app.Log.Warn("could not read the restored colors back", "reason", err)
		app.Notef("Restored, but the new colors could not be read back: %v\n", err)
	} else {
		r.Reread = reread
	}

	return renderReset(app, r)
}
