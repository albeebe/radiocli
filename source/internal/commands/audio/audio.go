// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package audio implements the "audio" command, which lists the sound inputs
// this computer can record from and the speakers it can play on, along with
// "audio output", "audio listen" and "audio record", which use them.
//
// The bare command only looks, and that separation is the point: listing opens
// nothing, so it never raises the microphone permission prompt, and a picker can
// be shown without the operating system interrupting about a device nobody has
// chosen. Putting the audio out is a different verb because it is a different
// act.
package audio

import (
	"fmt"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audioin"
	"github.com/albeebe/radiocli/internal/audioout"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the audio command bound to app.
//
// Parameters:
//   - app: the application context the command reads its configuration and
//     writes its output through
//
// Returns:
//   - the "audio" command, with "audio listen", "audio output" and
//     "audio record" already attached to it
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audio",
		Short: "List the sound inputs and speakers this computer has",
		Long: "Audio lists every sound input attached to this computer, and every speaker\n" +
			"it can play on. The scanner's own audio reaches an input over a cable rather\n" +
			"than over the USB connection, so which input carries it is something only you\n" +
			"can know.\n\n" +
			"It only looks. Nothing here opens an input or a speaker, which is why it\n" +
			"never asks for permission to use the microphone.\n\n" +
			"Run \"radiocli audio listen\" to hear the scanner on these speakers,\n" +
			"\"radiocli audio output\" to send its audio to another program, or\n" +
			"\"radiocli audio record\" to record its transmissions to files.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(app)
		},
	}

	cmd.AddCommand(newListen(app), newOutput(app), newRecord(app))
	return cmd
}

// advise says what to do about a half of the listing that came back empty.
//
// To stderr, after the document rather than instead of it, so that a reader
// which asked for JSON gets JSON either way. Finding nothing is a complete
// answer to "what can this computer do with sound", not a failure, so the
// command still succeeds.
//
// Parameters:
//   - app: the application context whose Stderr receives the advice
//   - found: what was found, which decides whether there is anything to say
func advise(app *appcontext.App, found listing) {
	if len(found.Inputs) == 0 {
		app.Notef("No sound inputs found. Connect a sound card or an audio interface, or " +
			"check that\nthis computer's own microphone is not switched off in its sound settings.\n")
	}

	// Said separately, because a machine can perfectly well have one and not
	// the other: a headless box with a USB audio interface plugged into a
	// scanner has an input and nowhere to play it, and that is worth knowing
	// before somebody runs "audio listen" on it.
	if len(found.Outputs) == 0 {
		app.Notef("No speakers found. Connect some, or check that this computer's own output " +
			"is not\nswitched off in its sound settings.\n")
	}
}

// renderListing writes both halves as aligned tables, one under the other.
//
// Two tables rather than one with a column saying which is which. Nothing can
// be both, the two are used by different flags, and a single list would put the
// answer to "where can I play this" among rows that cannot answer it.
//
// One column each, and tables anyway. It is the shape every other listing in
// the tool has, and the name is the whole point of these: it is what a person
// reads and what they will type.
//
// Parameters:
//   - app: the application context whose Stdout receives the tables
//   - found: the inputs and outputs to list, each in the order it should appear
//
// Returns:
//   - error if the tables cannot be written to Stdout
func renderListing(app *appcontext.App, found listing) error {
	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)

	if len(found.Inputs) > 0 {
		fmt.Fprintln(w, "SOUND INPUTS")
		for _, s := range found.Inputs {
			fmt.Fprintln(w, s.Name)
		}
	}

	if len(found.Outputs) > 0 {
		// A blank line between the two, and only when there is something above
		// to be separated from.
		if len(found.Inputs) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "SPEAKERS")
		for _, s := range found.Outputs {
			fmt.Fprintln(w, s.Name)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the sound device list: %w", err)
	}
	return nil
}

// run lists the sound inputs and the speakers, and renders them.
//
// Parameters:
//   - app: the application context holding the requested output format, the
//     logger and the streams
//
// Returns:
//   - error if either listing cannot be read, or if writing the listing fails
func run(app *appcontext.App) error {
	app.Log.Debug("listing the sound devices")

	sources, err := listSources()
	if err != nil {
		return err
	}

	sinks, err := listSinks()
	if err != nil {
		return err
	}

	// Both are made rather than left nil, so that a machine with nothing
	// attached renders as an empty JSON array rather than as null.
	found := listing{Inputs: sources, Outputs: sinks}
	if found.Inputs == nil {
		found.Inputs = []audioin.Source{}
	}
	if found.Outputs == nil {
		found.Outputs = []audioout.Sink{}
	}

	if app.Config.Output == appcontext.OutputJSON {
		if err := render.JSON(app.Stdout, found); err != nil {
			return err
		}
	} else if len(found.Inputs) > 0 || len(found.Outputs) > 0 {
		// With nothing found there is no table worth printing, and the advice
		// below says everything there is to say.
		if err := renderListing(app, found); err != nil {
			return err
		}
	}

	advise(app, found)
	return nil
}
