// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

// Package receiving implements the "receiving" command, which reports what the
// scanner is hearing at this instant.
//
// It exists because "what is it doing" and "what is it listening to" are
// different questions and only the first had an answer. status reports the
// connection, the firmware and the mode, all of which stay true for hours.
// scanning reports the channels in the rotation, which means moving the radio
// and takes seconds. Neither says what is coming out of the speaker right now,
// and that is the reading anything following along needs: it is what labels a
// recording, and what a person or an agent asks when the scanner stops on
// something.
//
// The mute state is what it answers with, rather than the signal strength. Those
// disagree at the moment that matters most. A document captured on the first
// poll of a real transmission read Mute="Unmute" with no signal bars at all, and
// the very next one showed five bars on the same unchanged signal, so anything
// waiting for bars misses the opening of everything. See device.Property.
//
// It reads and never presses a key, so it carries appcontext.OnlyReads and can
// be run while another command has the radio, including repeatedly while
// something long is running. That is what makes it usable as the label source
// for "audio record", which asks it several times a second through a daemon.
package receiving

import (
	"context"
	"fmt"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the receiving command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that reports what the scanner is hearing when it runs
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use: "receiving",
		// Only looks at what the scanner is hearing, so it may run while
		// another command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report what the scanner is hearing right now",
		Long: "Receiving reports what is coming out of the scanner at this instant: the\n" +
			"channel it stopped on, where that channel lives in its memory, and the\n" +
			"frequency or talkgroup behind it.\n\n" +
			"It is a reading taken now, not a summary. Check \"receiving\" before anything\n" +
			"else: a scanning radio still names whatever channel it is stepping past at\n" +
			"the instant it was asked, so a channel here is only one it stopped on when\n" +
			"\"receiving\" is yes.\n\n" +
			"Nothing is moved and no key is pressed, so it is safe to run repeatedly and\n" +
			"safe to run while another command has the scanner.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// cmd.Context() is cancelled when the user interrupts the tool.
			// Pass it down so a scanner that stops answering does not hang.
			return run(cmd.Context(), app)
		},
	}
}

// run reads what the scanner is hearing and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//
// Returns:
//   - error if the scanner cannot be reached, the read fails, or the JSON
//     cannot be written
func run(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	info, err := client.ScannerInfo(ctx)
	if err != nil {
		return fmt.Errorf("asking the scanner what it is hearing: %w", err)
	}
	r := info.Heard()

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("receiving:  %s\n", render.YesNo(r.Receiving))
	app.Printf("list:       %s\n", render.Dash(r.List))
	app.Printf("system:     %s\n", render.Dash(r.System))
	app.Printf("department: %s\n", render.Dash(r.Department))
	if r.Site != "" {
		app.Printf("site:       %s\n", r.Site)
	}
	app.Printf("channel:    %s\n", render.Dash(r.Channel))
	if r.Talkgroup != "" {
		app.Printf("talkgroup:  %s\n", r.Talkgroup)
	} else {
		app.Printf("frequency:  %s\n", render.Dash(r.Frequency))
	}
	if r.Unit != "" {
		app.Printf("unit:       %s\n", r.Unit)
	}
	app.Printf("signal:     %s\n", render.Dash(r.Signal))

	if !r.Receiving {
		// The fields above are still filled in, which is the trap. A scanning
		// radio names whatever channel it is stepping past at the instant it
		// was asked, and that reads exactly like a channel it stopped on.
		app.Notef("\nNothing is being received. The channel above is the one the scanner " +
			"happened to be checking as it scanned past, not one it stopped on.\n")
	}
	return nil
}
