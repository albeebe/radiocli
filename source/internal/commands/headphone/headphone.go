// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

// Package headphone implements the "headphone" command, which reports and
// changes whether the scanner's headphone jack sends its two sides in phase.
//
// It exists because of a wiring fault the setting is there to work around. The
// headphone jack on an SDS100 and an SDS150 is wired out of phase, and Uniden
// addressed it in firmware by adding this setting rather than by changing the
// hardware. So whether a given radio has the problem depends on which way its
// owner has left this, and there was no way to find out without picking the
// radio up.
//
// The symptom is worth knowing, because it does not look like a phase problem.
// Anything that folds the two sides of the jack together, which is what taking
// mono from a stereo input means, gets audio around eleven decibels too quiet
// with the body taken out of the voice: the low frequencies are the most alike
// between the two sides and cancel the most completely. What is left sounds
// thin and reedy, like the speaker is talking through a kazoo, and it is easy
// to blame on the radio or on a digital voice codec. See research/oddities.md.
//
// "audio record" does not need this. It measures what folding the two sides
// actually produces and takes one side when folding would destroy the signal,
// so it copes with either setting without being told. This command is for
// fixing it at the source, which is better: it is right for everything the
// radio is ever plugged into rather than for this tool alone.
//
// The setting is not in the scanner's remote protocol. It lives in a menu, so
// even reading it walks into Settings and back out, which stops the scan for a
// moment. That is why nothing here happens automatically.
package headphone

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

// New returns the headphone command, with its set subcommand attached, bound
// to app.
//
// Parameters:
//   - app: the application context the command and its subcommand read the
//     scanner and the output settings from
//
// Returns:
//   - the "headphone" command, with its "set" subcommand attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "headphone",
		Short: "Report whether the headphone jack is wired in phase",
		Long: "Headphone reports whether the scanner sends the same audio to both sides of\n" +
			"its headphone jack, or one of them inverted.\n\n" +
			"The jack on this scanner is wired out of phase, and the setting exists to\n" +
			"correct it. On \"invert-phase\", anything that combines the two sides into one\n" +
			"gets audio around eleven decibels too quiet with the body taken out of the\n" +
			"voice, because the two sides cancel each other. It sounds thin and reedy\n" +
			"rather than obviously broken, which is what makes it hard to place.\n\n" +
			"\"radiocli audio record\" copes with either setting on its own, by measuring\n" +
			"the two sides rather than trusting them, so this is not something you have to\n" +
			"set before recording. Changing it here fixes it for everything else the radio\n" +
			"is plugged into as well.\n\n" +
			"The setting is not in the scanner's remote protocol. It lives in a menu, so\n" +
			"even this reading walks into Settings and back out, which stops the scan for\n" +
			"a moment.\n\n" +
			"Run \"radiocli headphone set\" to change it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newSet(app))
	return cmd
}

// newSet returns the "headphone set" command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "headphone set" command
func newSet(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <" + InPhase + "|" + Invert + ">",
		Short: "Choose whether the headphone jack is wired in phase",
		Long: "Set chooses whether the scanner sends the same audio to both sides of its\n" +
			"headphone jack, or one of them inverted.\n\n" +
			"\"" + InPhase + "\" is the one to want. The jack is wired out of phase, so\n" +
			"\"" + Invert + "\" leaves the two sides cancelling each other in anything that\n" +
			"combines them.\n\n" +
			"The setting is stored on the scanner and stays after this command exits and\n" +
			"after the scanner is unplugged. Nothing is written to your computer.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Checked before the scanner is opened, so a typo costs nothing and
			// fails the same way whether or not a scanner is attached.
			want, ok := lookup(args[0])
			if !ok {
				return fmt.Errorf("there is no headphone setting called %q: it is %q or %q",
					args[0], InPhase, Invert)
			}
			return runSet(cmd.Context(), app, want)
		},
	}
}

// lookup finds a setting by the value this command spells it with.
//
// Parameters:
//   - value: the value as it was typed, matched without regard to case
//
// Returns:
//   - the setting
//   - true if there is one by that name
func lookup(value string) (phase, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, p := range phases {
		if p.value == value {
			return p, true
		}
	}
	return phase{}, false
}

// byEntry finds a setting by the scanner's own wording for it.
//
// Parameters:
//   - entry: the row's text, as the scanner draws it
//
// Returns:
//   - the setting
//   - true if there is one by that wording
func byEntry(entry string) (phase, bool) {
	entry = strings.TrimSpace(entry)
	for _, p := range phases {
		if strings.EqualFold(p.entry, entry) {
			return p, true
		}
	}
	return phase{}, false
}

// read opens the headphone setting and reports which value it is on.
//
// It leaves the scanner on that menu, because every caller does something else
// there next: either choosing a value, or leaving.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the setting from
//
// Returns:
//   - the setting the scanner is on
//   - error if the settings menu cannot be opened, the setting cannot be found
//     or read, or the highlighted row is a value this does not know
func read(ctx context.Context, client *device.Scanner) (phase, error) {
	if err := client.OpenMenu(ctx, device.MenuSettings, ""); err != nil {
		return phase{}, fmt.Errorf("opening the settings menu: %w", err)
	}
	if err := menus.Select(ctx, client, entryName); err != nil {
		return phase{}, fmt.Errorf("looking for %q: %w\n"+
			"It is only on firmware that has the setting at all, and it is what a scanner\n"+
			"with an out of phase headphone jack was given instead of new hardware.\n"+
			"Run \"radiocli scan\" to return the scanner to scanning", entryName, err)
	}

	// The scanner shows this as a list with the current value highlighted,
	// which is the whole of how it reports it. The highlight comes off the
	// screen rather than out of the protocol's listing, because the listing can
	// leave out rows that are really there and then name the wrong one as
	// selected.
	row, err := menus.HighlightedRow(ctx, client)
	if err != nil {
		return phase{}, fmt.Errorf("reading the headphone setting: %w", err)
	}

	got, ok := byEntry(row.Text)
	if !ok {
		return phase{}, fmt.Errorf("the headphone setting shows %q, which is neither of the "+
			"two values this scanner is known to offer", strings.TrimSpace(row.Text))
	}
	return got, nil
}

// runReport reads the setting and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be reached, the setting cannot be read, or
//     the scanner cannot be put back
func runReport(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	got, err := read(ctx, client)
	if err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	return renderPhase(app, got)
}

// renderPhase writes the setting, and says what an inverted one costs.
//
// Parameters:
//   - app: the application context the output is written through
//   - p: the setting the scanner is on
//
// Returns:
//   - error if the JSON could not be written
func renderPhase(app *appcontext.App, p phase) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, report{Phase: p.value})
	}

	app.Printf("headphone: %s\n", p.value)
	if p.value == Invert {
		app.Notef("\nThe two sides of the jack are inverted, so anything that combines them\n" +
			"into one cancels most of the sound. Run \"radiocli headphone set " + InPhase +
			"\"\nto correct it.\n")
	}
	return nil
}

// runSet changes the setting and reports what the scanner holds afterwards.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//   - want: the setting to choose
//
// Returns:
//   - error if the scanner cannot be reached, the setting cannot be changed, or
//     the scanner did not take it
func runSet(ctx context.Context, app *appcontext.App, want phase) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	was, err := read(ctx, client)
	if err != nil {
		return err
	}

	// Already there is not a change, and choosing the row it is already on
	// would be two menu walks to arrive where it started.
	if was.value == want.value {
		if _, err := menus.Leave(ctx, client); err != nil {
			return err
		}
		return renderPhase(app, was)
	}

	if err := menus.Select(ctx, client, want.entry); err != nil {
		return fmt.Errorf("choosing %q for the headphone output: %w\n"+
			"Run \"radiocli scan\" to return the scanner to scanning", want.entry, err)
	}

	// Read it back rather than trusting the press. The scanner is the authority
	// on what its own setting is, and choosing a value closes the list onto its
	// parent, so this walks in again to look.
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}
	got, err := read(ctx, client)
	if err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}
	if got.value != want.value {
		return fmt.Errorf("the headphone output is still %s after setting it to %s",
			got.value, want.value)
	}

	return renderPhase(app, got)
}
