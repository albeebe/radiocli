// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package squelch implements the "squelch" command, which reports the level at
// which the scanner opens its audio, and its "set" subcommand, which changes
// it.
//
// The bare command reads. Changing the scanner is a separate verb, so no
// reading of the squelch can turn into a write by mistake.
package squelch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the squelch command, with its set subcommand attached, bound to
// app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that reports the squelch level, carrying the set
//     subcommand that changes it
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use: "squelch",
		// Only looks at what the scanner is doing, so it may run while another
		// command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report the scanner's squelch level",
		Long: fmt.Sprintf("Squelch reports how strong a signal has to be before the scanner plays it, as\n"+
			"a level from %d to %d.\n\n"+
			"Run \"radiocli squelch set\" to change it.", device.MinLevel, device.MaxLevel),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newSet(app))
	return cmd
}

// newSet returns the "squelch set" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that sets the squelch level to the one given on the
//     command line
func newSet(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <level>",
		Short: "Set the scanner's squelch level",
		Long: fmt.Sprintf("Set changes how strong a signal has to be before the scanner plays it. The\n"+
			"level is a whole number from %d, which plays everything including the noise of\n"+
			"an empty channel, to %d, which plays only the strongest signals.\n\n"+
			"The level the scanner reports afterwards is printed, so a level it did not take\n"+
			"is visible straight away.", device.MinLevel, device.MaxLevel),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parsing before the scanner is opened means a typo costs nothing
			// and reports the same way whether or not a scanner is attached.
			level, err := parseLevel(args[0])
			if err != nil {
				return err
			}
			return runSet(cmd.Context(), app, level)
		},
	}
}

// parseLevel turns the argument into a level, rejecting anything the scanner
// could not take. The device layer checks the range again before sending; this
// check exists so the message names the level the user typed and arrives
// without opening the scanner.
//
// Parameters:
//   - arg: the level as it was typed on the command line, spaces and all
//
// Returns:
//   - the level, from device.MinLevel to device.MaxLevel
//   - error if the argument is not a whole number, or is a number outside the
//     range the scanner takes
func parseLevel(arg string) (int, error) {
	level, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil {
		return 0, fmt.Errorf("invalid squelch level %q: want a whole number from %d to %d",
			arg, device.MinLevel, device.MaxLevel)
	}
	if level < device.MinLevel || level > device.MaxLevel {
		return 0, fmt.Errorf("squelch level %d is out of range: want %d to %d",
			level, device.MinLevel, device.MaxLevel)
	}
	return level, nil
}

// renderLevel writes the level in whichever format was asked for.
//
// Parameters:
//   - app: application context holding the output format and the streams to
//     write to
//   - level: the squelch level to report
//
// Returns:
//   - error if the JSON cannot be written; nil for the text output, which
//     cannot fail
func renderLevel(app *appcontext.App, level int) error {
	r := report{Level: level, Min: device.MinLevel, Max: device.MaxLevel}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("squelch: %d of %d\n", r.Level, r.Max)
	return nil
}

// runGet reads the squelch and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//
// Returns:
//   - error if the scanner cannot be reached, the level cannot be read, or the
//     output cannot be written
func runGet(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	level, err := client.Squelch(ctx)
	if err != nil {
		return fmt.Errorf("reading the squelch: %w", err)
	}

	return renderLevel(app, level)
}

// runSet changes the squelch, then reports what the scanner holds afterwards.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//   - level: the level to set, already checked by parseLevel
//
// Returns:
//   - error if the scanner cannot be reached, refuses the new level, cannot be
//     read back afterwards, or the output cannot be written
func runSet(ctx context.Context, app *appcontext.App, level int) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if err := client.SetSquelch(ctx, level); err != nil {
		return fmt.Errorf("setting the squelch: %w", err)
	}

	// Read back rather than echo the requested level. The scanner is the
	// authority on what it is actually set to, and a level it quietly
	// declined would otherwise be reported as a success.
	actual, err := client.Squelch(ctx)
	if err != nil {
		return fmt.Errorf("reading the squelch back: %w", err)
	}

	if err := renderLevel(app, actual); err != nil {
		return err
	}

	if actual != level {
		app.Notef("\nThe scanner is at %d rather than the %d that was asked for.\n", actual, level)
	}
	return nil
}
