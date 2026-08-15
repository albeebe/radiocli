// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package beep implements the "beep" command, which reports and changes the
// sound the scanner makes when a key is pressed.
//
// The bare command reads. Changing the scanner is a separate verb, so no
// reading of the setting can turn into a write by mistake.
//
// The setting is not in the remote protocol. It lives in a menu, as a list of
// seventeen values, so even reading it walks the scanner into Settings and back
// out and stops the scan for a moment. That is worth knowing before this is put
// somewhere it runs often.
package beep

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the beep command bound to app.
//
// Parameters:
//   - app: the application context the command and its subcommands read the
//     scanner and the output settings from
//
// Returns:
//   - the "beep" command, with its "set" and "toggle" subcommands attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "beep",
		Short: "Report the sound the keypad makes",
		Long: "Beep reports the sound the scanner makes when a key is pressed.\n\n" +
			"The setting has seventeen values: \"auto\", which lets the scanner choose the\n" +
			"loudness, \"1\" to \"15\" for a fixed loudness, and \"off\" for silence.\n\n" +
			"It is not in the scanner's remote protocol. It lives in a menu, so even this\n" +
			"reading walks into Settings and back out, which stops the scan for a moment.\n\n" +
			"Run \"radiocli beep set\" to change it, and \"radiocli beep toggle\" to switch\n" +
			"it off and back on again without having to remember what it was.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newSet(app))
	cmd.AddCommand(newToggle(app))
	return cmd
}

// buildLevels writes out the list rather than holding a literal of seventeen
// near-identical rows, since fifteen of them differ only by a number.
//
// Returns:
//   - the seventeen settings, in the order the scanner lists them
func buildLevels() []level {
	out := make([]level, 0, 17)
	out = append(out, level{entry: "Auto", value: "auto"})
	for i := 1; i <= 15; i++ {
		out = append(out, level{entry: "Level " + strconv.Itoa(i), value: strconv.Itoa(i)})
	}
	return append(out, level{entry: "Off", value: off})
}

// byEntry finds a setting by the scanner's own wording.
//
// Parameters:
//   - entry: the wording to look for, matched without regard to case
//
// Returns:
//   - the setting that wording names, or the zero level when none does
//   - true when a setting was found
func byEntry(entry string) (level, bool) {
	for _, l := range levels {
		if strings.EqualFold(l.entry, entry) {
			return l, true
		}
	}
	return level{}, false
}

// choose walks to the setting, hands what it finds to decide, and writes back
// whatever decide asks for.
//
// decide is given the setting as it stands and returns the one wanted, which is
// what lets a toggle be one walk rather than two: the value it decides from is
// the one this had to read anyway.
//
// A setting already at the wanted value is left alone, so a command with
// nothing to do presses nothing. Both the value found and the value left are
// reported, because a toggle has to know what it is replacing in order to
// remember it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - decide: given the setting as it stands, returns the setting wanted
//
// Returns:
//   - was: the setting found before anything was pressed
//   - now: the setting the scanner was left on
//   - err: error if reading the setting, choosing a value, or leaving the menus
//     failed, or if the scanner did not end up on the value asked for
func choose(ctx context.Context, client *device.Scanner, decide func(level) level) (was, now level, err error) {
	was, err = read(ctx, client)
	if err != nil {
		return level{}, level{}, err
	}

	want := decide(was)
	if want.value == was.value {
		if _, err := menus.Leave(ctx, client); err != nil {
			return was, was, err
		}
		return was, was, nil
	}

	if err := menus.Select(ctx, client, want.entry); err != nil {
		return was, want, fmt.Errorf("choosing %q for the key beep: %w\n"+
			"Run \"radiocli scan\" to return the scanner to scanning", want.entry, err)
	}

	// Read it back rather than trusting the press. The scanner is the authority
	// on what its own setting is, and choosing a value closes the list onto its
	// parent, so this walks in again to look.
	if _, err := menus.Leave(ctx, client); err != nil {
		return was, want, err
	}
	got, err := read(ctx, client)
	if err != nil {
		return was, want, err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return was, got, err
	}

	if got.value != want.value {
		return was, got, fmt.Errorf("the key beep is still %s after setting it to %s",
			got.label(), want.label())
	}
	return was, got, nil
}

// label renders the setting for a person reading the output. It is the
// scanner's own wording in the tool's voice: "level 9", "auto", "off".
//
// Returns:
//   - the scanner's wording in lower case
func (l level) label() string { return strings.ToLower(l.entry) }

// lookup finds a setting by what somebody typed, whatever case they typed it
// in. A number written with a leading zero or spaces around it is not accepted
// here: the values are exact, and a near miss is refused with the list rather
// than guessed at.
//
// Parameters:
//   - value: what somebody typed, such as "auto", "9" or "off"
//
// Returns:
//   - the setting that value names, or the zero level when none does
//   - true when a setting was found
func lookup(value string) (level, bool) {
	for _, l := range levels {
		if strings.EqualFold(l.value, value) {
			return l, true
		}
	}
	return level{}, false
}

// on reports whether this setting makes any sound at all, which every one of
// them does except Off.
//
// Returns:
//   - true for every setting except off
func (l level) on() bool { return l.value != off }

// read opens the key beep setting and reports which value it is on.
//
// It leaves the scanner on that menu, because every caller does something else
// there next: either choosing a value, or leaving. Leaving here and walking
// back in would be two menus of work to arrive where it already is.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the setting from
//
// Returns:
//   - the setting the scanner is on
//   - error if the settings menu cannot be opened, the key beep setting cannot
//     be found or read, or the highlighted row is a value this does not know
func read(ctx context.Context, client *device.Scanner) (level, error) {
	if err := client.OpenMenu(ctx, device.MenuSettings, ""); err != nil {
		return level{}, fmt.Errorf("opening the settings menu: %w", err)
	}
	if err := menus.Select(ctx, client, adjustKeyBeep); err != nil {
		return level{}, fmt.Errorf("looking for %q: %w\n"+
			"Run \"radiocli scan\" to return the scanner to scanning", adjustKeyBeep, err)
	}

	// The scanner shows this as a list with the current value highlighted,
	// which is the whole of how it reports it. The highlight comes off the
	// screen rather than out of the protocol's listing, because the listing can
	// leave out rows that are really there and then name the wrong one as
	// selected.
	row, err := menus.HighlightedRow(ctx, client)
	if err != nil {
		return level{}, fmt.Errorf("reading the key beep setting: %w", err)
	}

	got, ok := byEntry(row.Text)
	if !ok {
		return level{}, fmt.Errorf("the key beep setting shows %q, which is none of the "+
			"seventeen values this scanner is known to offer", row.Text)
	}
	return got, nil
}

// renderReport writes the setting.
//
// Parameters:
//   - app: the application context the output is written through
//   - r: the result to write
//
// Returns:
//   - error if the JSON encoding could not be written
func renderReport(app *appcontext.App, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	got, _ := lookup(r.Level)
	app.Printf("key beep: %s\n", got.label())
	return nil
}

// runReport reads the setting and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the setting cannot be read, the
//     menus cannot be left, or the result cannot be written
func runReport(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	now, err := read(ctx, client)
	if err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	return renderReport(app, report{Level: now.value, On: now.on()})
}

// values lists what may be typed, for the help and for a refusal to quote.
//
// The fifteen numbered levels are written as a range rather than one by one,
// because a message naming all seventeen is longer than the screen and no
// clearer for it.
//
// Returns:
//   - the values that may be typed, written for a person to read
func values() string {
	return "auto, 1 to 15, or off"
}
