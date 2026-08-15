// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package display implements the "display" command, which reports whether the
// scanner draws its screen in color and switches it between color and the two
// black and white modes.
//
// The scanner stores a text and a background color for every element of every
// screen layout, and this setting decides whether any of it is drawn. In either
// black and white mode those colors are still stored, still editable, and
// completely ignored. Anything drawing the scanner's screen elsewhere has to
// read this first or it will paint a color screen while the scanner in the
// user's hand is monochrome.
//
// Reading is a single command over the wire and works while the scanner is
// scanning. Writing is not: the setting lives at
// MENU -> Display Options -> Set B/W or Color Mode, the protocol has no menu id
// for Display Options, and so it has to be walked with key presses, which stops
// the scan for as long as it takes.
package display

import (
	"context"
	"fmt"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the display command bound to app.
//
// Parameters:
//   - app: the application context the command and its subcommand read the
//     scanner and the output settings from
//
// Returns:
//   - the "display" command, with its "mode" subcommand attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use: "display",
		// Only looks at what the scanner is doing, so it may run while another
		// command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report whether the scanner draws its screen in color",
		Long: "Display reports whether the scanner is drawing its screen in color or in one\n" +
			"of its two black and white modes.\n\n" +
			"The scanner stores a text and a background color for every element of every\n" +
			"layout. This setting decides whether they are drawn at all: in either black\n" +
			"and white mode they are kept but ignored. Anything redrawing the scanner's\n" +
			"screen elsewhere should read this before choosing a palette.\n\n" +
			"Run \"radiocli display mode\" to change it. This is a read and changes\n" +
			"nothing. It does not stop the scanner scanning.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newMode(app))
	return cmd
}

// newMode returns the "display mode" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "mode" subcommand
func newMode(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:       "mode [" + strings.Join(names(), "|") + "]",
		Short:     "Set whether the scanner draws its screen in color",
		ValidArgs: names(),
		Long: "Mode sets how the scanner draws its screen:\n\n" +
			descriptions() + "\n" +
			"Run it with no mode to report the current one, which is the same as\n" +
			"\"radiocli display\".\n\n" +
			"Setting is done through the scanner's menus, because the protocol offers no\n" +
			"way to write it, so it stops the scan and returns the scanner afterwards. The\n" +
			"setting is read back to confirm it took. Asking for the mode the scanner is\n" +
			"already in does nothing and never opens a menu.",
		// Cobra's own OnlyValidArgs names the offending word without saying what
		// would have been accepted, which is the one thing the reader needs.
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}
			if len(args) == 1 {
				if _, ok := lookup(args[0]); !ok {
					return fmt.Errorf("%q is not a display mode: want %s",
						args[0], strings.Join(names(), ", "))
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runReport(cmd.Context(), app)
			}
			return runSet(cmd.Context(), app, args[0])
		},
	}
}

// descriptions renders the modes for the command's help.
//
// Returns:
//   - one line per mode, naming it and saying what it looks like
func descriptions() string {
	var b strings.Builder
	for _, m := range modes {
		fmt.Fprintf(&b, "  %-6s %s\n", m.name, m.description)
	}
	return b.String()
}

// find looks a mode up by what the scanner reports, which is how a value this
// tool has no name for is rendered without pretending it is one of the three.
//
// Parameters:
//   - value: the mode as the scanner reports it
//
// Returns:
//   - the mode that value names, or the zero mode when this tool has no name
//     for it
//   - true when a mode was found
func find(value device.DisplayMode) (mode, bool) {
	for _, candidate := range modes {
		if candidate.value == value {
			return candidate, true
		}
	}
	return mode{}, false
}

// lookup finds a mode by the name this command accepts.
//
// Parameters:
//   - name: what somebody typed, matched without regard to case
//
// Returns:
//   - the mode that name refers to, or the zero mode when none does
//   - true when a mode was found
func lookup(name string) (mode, bool) {
	for _, candidate := range modes {
		if candidate.name == strings.ToLower(name) {
			return candidate, true
		}
	}
	return mode{}, false
}

// names lists the mode names this command accepts, in the menu's order.
//
// Returns:
//   - the short names, in the order the scanner's menu lists the modes
func names() []string {
	out := make([]string, 0, len(modes))
	for _, m := range modes {
		out = append(out, m.name)
	}
	return out
}

// read reports which mode the scanner is in.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//
// Returns:
//   - the mode the scanner is in
//   - error if the scanner could not be read
func read(ctx context.Context, client *device.Scanner) (device.DisplayMode, error) {
	mode, err := client.DisplayMode(ctx)
	if err != nil {
		return 0, fmt.Errorf("reading the display mode: %w", err)
	}
	return mode, nil
}

// renderMode writes the mode.
//
// Parameters:
//   - app: the application context the output is written through
//   - mode: the mode to write
//
// Returns:
//   - error if the JSON encoding could not be written
func renderMode(app *appcontext.App, mode device.DisplayMode) error {
	r := report{Mode: mode.String(), Color: mode == device.DisplayColor}
	if m, ok := find(mode); ok {
		r.Entry = m.entry
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("display: %s\n", r.Mode)
	if r.Entry != "" {
		app.Printf("menu:    %s\n", r.Entry)
	}
	return nil
}

// runReport reports the current mode.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the mode cannot be read, or the
//     result cannot be written
func runReport(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	mode, err := read(ctx, client)
	if err != nil {
		return err
	}
	return renderMode(app, mode)
}

// runSet writes the mode and confirms it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//   - name: the mode to set, as this command names it
//
// Returns:
//   - error if the name is not a mode, the scanner cannot be opened, the mode
//     cannot be read or written, the scanner did not end up in the mode asked
//     for, or the result cannot be written
func runSet(ctx context.Context, app *appcontext.App, name string) error {
	want, ok := lookup(name)
	if !ok {
		return fmt.Errorf("%q is not a display mode: want %s",
			name, strings.Join(names(), ", "))
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	now, err := read(ctx, client)
	if err != nil {
		return err
	}

	// Already there, so leave the scanner alone. The walk is the expensive part
	// of this command and the only part that interrupts a scan.
	if now == want.value {
		return renderMode(app, now)
	}

	if err := set(ctx, client, want.entry); err != nil {
		return err
	}

	// Read it back rather than trusting the press. The read is cheap and the
	// walk selects by name off a screen the scanner redraws as it likes.
	now, err = read(ctx, client)
	if err != nil {
		return err
	}
	if now != want.value {
		return fmt.Errorf("the display is still in %s mode after setting it to %s",
			now, want.name)
	}

	return renderMode(app, now)
}

// set walks to the setting and chooses one of its entries.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - entry: the menu entry to choose, in the scanner's own wording
//
// Returns:
//   - error if the top menu cannot be opened, a menu entry on the way cannot be
//     found, or the scanner cannot be returned from the menus
func set(ctx context.Context, client *device.Scanner, entry string) error {
	if err := client.OpenMenu(ctx, device.MenuTop, ""); err != nil {
		return fmt.Errorf("opening the top menu: %w", err)
	}

	for _, want := range []string{displayOptions, colorMode, entry} {
		if err := menus.Select(ctx, client, want); err != nil {
			return fmt.Errorf("looking for %q: %w\n"+
				"Run \"radiocli scan\" to return the scanner to scanning", want, err)
		}
	}

	// Choosing an entry drops the scanner back onto Display Options rather than
	// out of the menus, so it still has to be shown the door.
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}
	return nil
}
