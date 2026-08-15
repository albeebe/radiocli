// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package beep

import (
	"context"
	"fmt"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/spf13/cobra"
)

// newSet returns the "beep set" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "set" subcommand
func newSet(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <auto|1-15|off>",
		Short: "Set the sound the keypad makes",
		Long: "Set changes the sound the scanner makes when a key is pressed.\n\n" +
			"Pass \"auto\" to let the scanner choose the loudness, a number from \"1\" to\n" +
			"\"15\" for a fixed one, or \"off\" for silence.\n\n" +
			"It walks the scanner's menus, which stops the scan for a moment, and reads\n" +
			"the setting back afterwards rather than trusting the key press.\n\n" +
			"Setting it here does not disturb what \"beep toggle\" has remembered.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parsed before the scanner is opened, so a mistyped value costs
			// nothing and reads the same whether or not one is attached.
			want, ok := lookup(args[0])
			if !ok {
				return fmt.Errorf("%q is not a key beep setting: want %s", args[0], values())
			}
			return runSet(cmd.Context(), app, want)
		},
	}
}

// runSet writes one setting and reports what the scanner ended up on.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//   - want: the setting to write
//
// Returns:
//   - error if the scanner cannot be opened, the setting cannot be written or
//     read back, or the result cannot be written
func runSet(ctx context.Context, app *appcontext.App, want level) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	_, now, err := choose(ctx, client, func(level) level { return want })
	if err != nil {
		return err
	}
	return renderReport(app, report{Level: now.value, On: now.on()})
}
