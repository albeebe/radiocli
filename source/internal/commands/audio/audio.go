// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package audio implements the "audio" command, which lists the sound inputs
// this computer can record from, and "audio output", which records from one.
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
//   - the "audio" command, with "audio output" and "audio record" already
//     attached to it
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audio",
		Short: "List the sound inputs this computer can record from",
		Long: "Audio lists every sound input attached to this computer. The scanner's own\n" +
			"audio reaches one of them over a cable rather than over the USB connection,\n" +
			"so which input carries it is something only you can know.\n\n" +
			"It only looks. Nothing here opens an input, which is why it never asks for\n" +
			"permission to use the microphone.\n\n" +
			"Run \"radiocli audio output\" to hear one, or \"radiocli audio record\" to\n" +
			"record the scanner's transmissions to files.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(app)
		},
	}

	cmd.AddCommand(newOutput(app), newRecord(app))
	return cmd
}

// renderSources writes the listing as an aligned table.
//
// Finding nothing is a complete answer to "what can this computer record
// from", not a failure, so the caller puts the advice on Stderr rather than
// failing the command. Scripts checking the exit code care whether the listing
// ran, and the empty list is the result.
//
// One column, and a table anyway. It is the shape every other listing in the
// tool has, and the name is the whole point of this one: it is what a person
// reads and, once there is a command to choose a source, what they will type.
//
// Parameters:
//   - app: the application context whose Stdout receives the table
//   - sources: the sound inputs to list, in the order they should appear
//
// Returns:
//   - error if the table cannot be written to Stdout
func renderSources(app *appcontext.App, sources []audioin.Source) error {
	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME")

	for _, s := range sources {
		fmt.Fprintln(w, s.Name)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the sound input list: %w", err)
	}
	return nil
}

// run lists the sound inputs and renders them.
//
// Parameters:
//   - app: the application context holding the requested output format, the
//     logger and the streams
//
// Returns:
//   - error if the sound inputs cannot be listed, or if writing the listing
//     fails
func run(app *appcontext.App) error {
	app.Log.Debug("listing the sound inputs")
	sources, err := listSources()
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		if err := render.JSON(app.Stdout, sources); err != nil {
			return err
		}
	} else if len(sources) > 0 {
		// With nothing found there is no table worth printing, and the advice
		// below says everything there is to say.
		if err := renderSources(app, sources); err != nil {
			return err
		}
	}

	// The advice goes to stderr, after the document rather than instead of it,
	// so a reader that asked for JSON gets JSON either way.
	if len(sources) == 0 {
		app.Notef("No sound inputs found. Connect a sound card or an audio interface, or " +
			"check that\nthis computer's own microphone is not switched off in its sound settings.\n")
	}
	return nil
}
