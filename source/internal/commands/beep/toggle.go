// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package beep

import (
	"context"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/spf13/cobra"
)

// newToggle returns the "beep toggle" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "toggle" subcommand
func newToggle(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "toggle",
		Short: "Switch the keypad sound off, or back to what it was",
		Long: "Toggle switches the keypad sound off, and switches it back to whatever it was\n" +
			"the next time it is run.\n\n" +
			"It reads the setting before changing it, and what it finds decides what it\n" +
			"does. A keypad making any sound is written down and then switched off. A\n" +
			"keypad already off is put back to the setting written down last time.\n\n" +
			"With nothing written down it is left off, which is the state whoever pressed\n" +
			"this last asked for. That happens on the first run against a scanner that was\n" +
			"already silent, and if the file the setting is kept in has been cleared.\n\n" +
			"The setting is written down because switching off is what destroys it: the\n" +
			"scanner keeps one value and nothing on the radio remembers the last one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToggle(cmd.Context(), app)
		},
	}
}

// renderToggle writes what the toggle did.
//
// It says more than the plain setting, because the whole point of the command
// is what it did with the value it found: a run that stored one has to say so,
// or the next press looks like it came from nowhere, and a run that left the
// keypad silent because it had nothing to put back has to say that rather than
// look like it did nothing at all.
//
// Parameters:
//   - app: the application context the output is written through
//   - r: what the toggle did
//
// Returns:
//   - error if the JSON encoding could not be written
func renderToggle(app *appcontext.App, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return renderReport(app, r)
	}

	got, _ := lookup(r.Level)
	switch {
	case r.Remembered != "":
		was, _ := lookup(r.Remembered)
		app.Printf("key beep: %s, and %s was written down to go back to\n", got.label(), was.label())
	case r.Restored:
		app.Printf("key beep: %s, put back\n", got.label())
	case !r.On:
		app.Printf("key beep: %s, with nothing written down to go back to\n", got.label())
	default:
		app.Printf("key beep: %s\n", got.label())
	}
	return nil
}

// runToggle switches the keypad sound off, or back to what it was.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the setting cannot be read or
//     written, or the result cannot be written
func runToggle(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}
	info := client.Info()

	// Read before the walk, because deciding needs it and it costs no scanner
	// time: it comes off the disk. Reading it inside the decision would put a
	// file read in the middle of a menu walk for no gain.
	stored, remembered := recall(app, info)

	silent, _ := lookup(off)
	was, now, err := choose(ctx, client, func(found level) level {
		// A keypad making any sound goes quiet, and what it was is written
		// down once the scanner has confirmed the change.
		if found.on() {
			return silent
		}
		// A silent one goes back to what was written down, and stays silent
		// when there is nothing to go back to.
		if remembered {
			return stored
		}
		return found
	})
	if err != nil {
		return err
	}

	r := report{Level: now.value, On: now.on()}

	switch {
	case was.on() && !now.on():
		// Written down only now, after the scanner confirmed it, so a failed
		// change cannot leave a note claiming the keypad was switched off.
		remember(app, info, was, time.Now())
		r.Remembered = was.value
	case !was.on() && now.on():
		r.Restored = true
	}

	return renderToggle(app, r)
}
