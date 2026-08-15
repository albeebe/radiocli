// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package status implements the "status" command.
//
// It is the template every other command that talks to the scanner should
// follow: New wires the command to the App and declares its own flags, and run
// holds the work with no cobra types in sight, reaching the scanner through
// app.Device so a test can inject a fake.
package status

import (
	"context"
	"fmt"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the status command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that reports the connected scanner when it runs
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use: "status",
		// Only looks at what the scanner is doing, so it may run while another
		// command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report the connected scanner",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// cmd.Context() is cancelled when the user interrupts the tool.
			// Pass it down so a scanner that stops answering does not hang.
			return run(cmd.Context(), app)
		},
	}
}

// run reads the scanner's identity and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//
// Returns:
//   - error if the scanner cannot be reached, any of the three reads fails, or
//     the JSON cannot be written
func run(ctx context.Context, app *appcontext.App) error {
	// The connection opens on first use, so commands that never call this pay
	// nothing for it.
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	firmware, err := client.FirmwareVersion(ctx)
	if err != nil {
		return fmt.Errorf("reading the firmware version: %w", err)
	}

	display, err := client.DisplayMode(ctx)
	if err != nil {
		return fmt.Errorf("reading the display mode: %w", err)
	}

	info, err := client.ScannerInfo(ctx)
	if err != nil {
		return fmt.Errorf("asking the scanner what it is doing: %w", err)
	}

	r := report{
		Port:     client.Info().Port,
		Model:    client.Info().Model,
		Firmware: firmware,
		Display:  display.String(),
		Mode:     info.Mode,
		Holding:  info.Holding(),
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("port:     %s\n", r.Port)
	app.Printf("model:    %s\n", r.Model)
	app.Printf("firmware: %s\n", r.Firmware)
	app.Printf("display:  %s\n", r.Display)
	app.Printf("mode:     %s\n", r.Mode)
	if r.Holding {
		app.Notef("\nThe scanner is holding rather than scanning. " +
			"Run \"radiocli scan\" to release it.\n")
	}
	return nil
}
