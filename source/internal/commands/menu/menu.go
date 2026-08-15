// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package menu implements the "menu" command, which reports the menu the
// scanner is showing, and its subcommands, which move around inside the menus.
//
// The bare command reads. Opening, leaving, and setting a value all drive the
// scanner's own interface, and opening a menu stops it scanning, so each of
// those is a separate verb rather than a flag.
package menu

import (
	"context"
	"fmt"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/spf13/cobra"
)

// New returns the menu command, with its subcommands attached, bound to app.
//
// Parameters:
//   - app: the application context the command and its subcommands read the
//     scanner and the output settings from
//
// Returns:
//   - the "menu" command, with its "open", "back", "close" and "set"
//     subcommands attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use: "menu",
		// Only looks at what the scanner is doing, so it may run while another
		// command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report the menu the scanner is showing",
		Long: "Menu reports the menu the scanner is currently showing, with its entries.\n\n" +
			"Reporting the menu changes nothing. The subcommands do: opening a menu takes\n" +
			"the scanner out of scanning, and most other commands are refused while it is\n" +
			"in one, so remember to close it again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return show(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newOpen(app), newBack(app), newClose(app), newSet(app))
	return cmd
}

// newBack returns the "menu back" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "back" subcommand
func newBack(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "back",
		Short: "Go up one level within the menus",
		Long: "Back climbs one level within the menus, without leaving them.\n\n" +
			"To leave the menus entirely and return to scanning, use \"radiocli menu close\".",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.Device(cmd.Context())
			if err != nil {
				return err
			}
			if err := client.MenuBack(cmd.Context()); err != nil {
				return fmt.Errorf("going back: %w", err)
			}
			return show(cmd.Context(), app)
		},
	}
}

// newClose returns the "menu close" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "close" subcommand
func newClose(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "close",
		Short: "Leave the menus and return to scanning",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.Device(cmd.Context())
			if err != nil {
				return err
			}
			if err := client.CloseMenu(cmd.Context()); err != nil {
				return fmt.Errorf("closing the menu: %w", err)
			}
			app.Notef("The scanner has left the menus.\n")
			return nil
		},
	}
}

// newOpen returns the "menu open" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "open" subcommand
func newOpen(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "open <menu> [index]",
		Short: "Open one of the scanner's menus",
		Long: "Open puts the scanner into one of its menus.\n\n" +
			"The index selects which system, department, site, channel, or search bank the\n" +
			"menu opens on, and is ignored by menus that do not need one.\n\n" +
			"This stops the scanner scanning. Most other commands are refused while it is\n" +
			"in a menu, so run \"radiocli menu close\" when finished.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, ok := menus.Lookup(args[0])
			if !ok {
				return fmt.Errorf("no menu is called %q: want %s", args[0], strings.Join(menus.Names(), ", "))
			}

			index := ""
			if len(args) == 2 {
				index = args[1]
			}

			client, err := app.Device(cmd.Context())
			if err != nil {
				return err
			}
			if err := client.OpenMenu(cmd.Context(), id, index); err != nil {
				return fmt.Errorf("opening the %s menu: %w", args[0], err)
			}

			// Report what was landed on, so a menu that opened somewhere
			// unexpected is visible without a second command.
			return show(cmd.Context(), app)
		},
	}
}

// newSet returns the "menu set" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "set" subcommand
func newSet(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <value>",
		Short: "Set the value of the menu item the scanner is on",
		Long: "Set writes a value into the menu item the scanner is currently on, which is\n" +
			"how a name or a number is entered without pressing a key per character.\n\n" +
			"What counts as a valid value depends entirely on the item. The scanner\n" +
			"refuses anything it will not take, so check with \"radiocli menu\" first that\n" +
			"it is on the item you mean.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.Device(cmd.Context())
			if err != nil {
				return err
			}
			if err := client.SetMenuValue(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("setting the menu value to %q: %w", args[0], err)
			}
			return show(cmd.Context(), app)
		},
	}
}

// show reads the menu the scanner is on and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the menu cannot be read, or the
//     result cannot be written
func show(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}
	return menus.Show(ctx, app, client)
}
