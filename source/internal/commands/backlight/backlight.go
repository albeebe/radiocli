// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package backlight implements the "backlight" command, which reports whether
// the scanner is lit and switches it on and off.
//
// Two separate things are called the backlight, and this command keeps them
// apart because they behave nothing alike.
//
// The light itself is the screen and, if it is switched on to, the keypad. It
// is momentary: there is one key for it and that key toggles. The scanner
// reports whether it is currently lit, so this can press once and check rather
// than pressing and hoping.
//
// The keypad's light is a setting, kept in a menu, which decides whether the
// keys join in when the light comes on. It survives a power cycle, and it is
// not readable except by opening the menu that sets it.
//
// The two are connected in a way nothing on the scanner mentions: changing the
// setting while the light is already on does nothing until the light is
// switched off and on again. Everything here that changes the setting cycles
// the light afterwards, so a caller never has to know that.
package backlight

import (
	"context"
	"fmt"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the backlight command bound to app.
//
// Parameters:
//   - app: the application context the command and its subcommands read the
//     scanner and the output settings from
//
// Returns:
//   - the "backlight" command, with its "on", "off" and "keys" subcommands
//     attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backlight",
		Short: "Report whether the scanner is lit",
		Long: "Backlight reports whether the scanner's light is on, and how bright it is.\n\n" +
			"Run \"radiocli backlight on\" and \"off\" to change it, and\n" +
			"\"radiocli backlight keys\" for whether the keypad lights up with the screen.\n\n" +
			"This is a read and changes nothing. It does not stop the scanner scanning.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newOn(app), newOff(app), newKeys(app))
	return cmd
}

// newKeys returns the "backlight keys" subcommand and its own three verbs.
//
// Parameters:
//   - app: the application context the command and its subcommands read the
//     scanner and the output settings from
//
// Returns:
//   - the "keys" subcommand, with "enable", "disable" and "toggle" attached
func newKeys(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Report whether the keypad lights up with the screen",
		Long: "Keys reports whether the scanner's keypad lights along with its screen.\n\n" +
			"This is a setting rather than a switch: it decides what happens the next time\n" +
			"the light comes on, and it survives a power cycle. The scanner will only say\n" +
			"what it is set to from the menu that sets it, so reading this stops the\n" +
			"scanner scanning for a moment and returns it afterwards.\n\n" +
			"Run \"radiocli backlight keys enable\", \"disable\" or \"toggle\" to change it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysReport(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newKeysSet(app, true), newKeysSet(app, false), newKeysToggle(app))
	return cmd
}

// newKeysSet returns "backlight keys enable" or "backlight keys disable".
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//   - want: true for the "enable" verb, false for "disable"
//
// Returns:
//   - the subcommand for that verb
func newKeysSet(app *appcontext.App, want bool) *cobra.Command {
	use, what := "disable", "stop"
	if want {
		use, what = "enable", "start"
	}

	return &cobra.Command{
		Use:   use,
		Short: fmt.Sprintf("Make the keypad %s lighting up with the screen", what),
		Long: fmt.Sprintf("%s makes the scanner's keypad %s lighting up when the screen does.\n\n"+
			"The setting is read back afterwards to confirm it took. If the light is on at\n"+
			"the time it is switched off and on again, because the scanner only acts on this\n"+
			"setting when the light comes on: changed while lit, it appears to do nothing.\n\n"+
			"This is done through the scanner's menus, so it stops the scan and returns it\n"+
			"when it is done.", title(use), what),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysSet(cmd.Context(), app, want)
		},
	}
}

// newKeysToggle returns the "backlight keys toggle" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "toggle" subcommand
func newKeysToggle(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "toggle",
		Short: "Switch the keypad light to whichever it is not",
		Long: "Toggle makes the scanner's keypad start lighting up with the screen if it was\n" +
			"not, and stop if it was. It is one command for both, for a button that has no\n" +
			"way of knowing which way round things are.\n\n" +
			"It changes the keypad only. The screen's own light is left as it is, and is\n" +
			"not switched off: this is the setting that decides whether the keys join in,\n" +
			"not the light itself.\n\n" +
			"The setting is read back afterwards to confirm it took. If the light is on at\n" +
			"the time it is switched off and on again, because the scanner only acts on\n" +
			"this setting when the light comes on, so the screen blinks once and comes\n" +
			"back lit.\n\n" +
			"This is done through the scanner's menus, so it stops the scan and returns it\n" +
			"when it is done.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysToggle(cmd.Context(), app)
		},
	}
}

// newOff returns the "backlight off" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "off" subcommand
func newOff(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Put the scanner's light out",
		Long: "Off puts the scanner's light out. It reads the light first, so running it\n" +
			"while the scanner is already dark leaves it dark rather than switching it on.\n\n" +
			"It leaves the keypad light setting alone, so a later \"backlight on\" lights\n" +
			"whatever it lit before. This does not stop the scanner scanning.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOff(cmd.Context(), app)
		},
	}
}

// newOn returns the "backlight on" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "on" subcommand, carrying its own --keys flag
func newOn(app *appcontext.App) *cobra.Command {
	var keysOnly bool

	cmd := &cobra.Command{
		Use:   "on",
		Short: "Light the scanner up",
		Long: "On lights the scanner. It reads the light first, so running it while the\n" +
			"scanner is already lit leaves it lit rather than switching it off: the\n" +
			"scanner has one key for this and that key toggles.\n\n" +
			"It also switches the keypad light on if it was off, because a request to\n" +
			"light the scanner up that leaves half of it dark is not what was asked for.\n" +
			"That part opens a menu and so stops the scan for a moment. Pass --keys=false\n" +
			"to leave the setting alone and light only the screen.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOn(cmd.Context(), app, keysOnly)
		},
	}

	cmd.Flags().BoolVar(&keysOnly, "keys", true,
		"switch the keypad light on as well, if it is off")
	return cmd
}

// chooseKeys walks to the keypad light setting, sets it to whatever decide
// makes of what is there, and reads it back.
//
// decide is handed the setting as it stands and returns the one wanted, which
// is what lets a toggle be one walk rather than two: the value it decides from
// is the one this had to read anyway.
//
// It reports whether the setting was changed, which is what tells the caller
// the light needs cycling, and what it ended up as. A setting already at the
// wanted value is left alone and reported as unchanged, so nothing flickers for
// a command that had nothing to do.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - decide: given the setting as it stands, returns the setting wanted
//
// Returns:
//   - true when the setting was changed, which is what tells the caller the
//     light needs cycling
//   - the setting wanted, which is what it ended up as on a run that returned
//     no error
//   - error if reading the setting, choosing an entry, or leaving the menus
//     failed, or if the setting did not end up as it was asked to
func chooseKeys(ctx context.Context, client *device.Scanner, decide func(bool) bool) (bool, bool, error) {
	on, err := readKeys(ctx, client)
	if err != nil {
		return false, false, err
	}

	want := decide(on)
	if on == want {
		if _, err := menus.Leave(ctx, client); err != nil {
			return false, want, err
		}
		return false, want, nil
	}

	entry := disableEntry
	if want {
		entry = enableEntry
	}
	if err := menus.Select(ctx, client, entry); err != nil {
		return false, want, fmt.Errorf("choosing %q for the keypad light: %w\n"+
			"Run \"radiocli scan\" to return the scanner to scanning", entry, err)
	}

	// Read it back rather than trusting the press. The menu closes onto its
	// parent when an entry is chosen, so this walks in again.
	if _, err := menus.Leave(ctx, client); err != nil {
		return false, want, err
	}

	now, err := readKeys(ctx, client)
	if err != nil {
		return false, want, err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return false, want, err
	}

	if now != want {
		return false, want, fmt.Errorf("the keypad light is still %s after setting it to %s",
			onOff(now), onOff(want))
	}
	return true, want, nil
}

// enableKeys switches the keypad light on if it is not already, and reports
// whether it had to change anything.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//
// Returns:
//   - true when the setting had to be changed
//   - error if the setting could not be read, written, or confirmed
func enableKeys(ctx context.Context, client *device.Scanner) (bool, error) {
	return setKeys(ctx, client, true)
}

// encode writes a value as JSON.
//
// Parameters:
//   - app: the application context the output is written through
//   - v: the value to encode
//
// Returns:
//   - error if the encoding could not be written
func encode(app *appcontext.App, v any) error {
	return render.JSON(app.Stdout, v)
}

// flip presses the light key once and waits for the scanner to agree it went
// the way that was wanted.
//
// The key toggles rather than sets, so pressing without checking is how a
// command asked to light the scanner puts it out instead. The check is cheap,
// because unlike most things about this scanner the light is readable.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to press the key on
//   - want: true to light the scanner, false to put it out
//
// Returns:
//   - the light's state once it agrees it went the way that was wanted
//   - error if the key could not be pressed, the light could not be read, the
//     context was cancelled, or the light did not change within flipPolls looks
func flip(ctx context.Context, client *device.Scanner, want bool) (device.Backlight, error) {
	if err := client.PressKey(ctx, device.KeyBacklight, device.KeyPress); err != nil {
		return device.Backlight{}, fmt.Errorf("pressing the light key: %w", err)
	}

	for poll := 0; poll < flipPolls; poll++ {
		state, err := read(ctx, client)
		if err != nil {
			return device.Backlight{}, err
		}
		if state.On == want {
			return state, nil
		}

		select {
		case <-ctx.Done():
			return device.Backlight{}, ctx.Err()
		case <-time.After(flipGap):
		}
	}

	return device.Backlight{}, fmt.Errorf("the light did not come %s after pressing the light key",
		onOff(want))
}

// onOff renders a state as a word.
//
// Parameters:
//   - on: the state to renderReport
//
// Returns:
//   - "on" or "off"
func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// read reports whether the scanner is lit.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//
// Returns:
//   - the light's state, including its brightness
//   - error if the scanner could not be read
func read(ctx context.Context, client *device.Scanner) (device.Backlight, error) {
	return client.Backlight(ctx)
}

// readKeys opens the keypad light setting and reports which way it is set.
//
// It leaves the scanner on that menu, because both callers do something else
// there next: either choosing an entry, or leaving. Leaving here and walking
// back in would be four menus of work to arrive where it already is.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the setting from
//
// Returns:
//   - true when the keypad lights along with the screen
//   - error if the top menu cannot be opened, a menu entry on the way cannot be
//     found, the setting cannot be read, or the highlighted row is neither
//     Enable nor Disable
func readKeys(ctx context.Context, client *device.Scanner) (bool, error) {
	if err := client.OpenMenu(ctx, device.MenuTop, ""); err != nil {
		return false, fmt.Errorf("opening the top menu: %w", err)
	}

	for _, entry := range []string{displayOptions, backlightOptions, keyBacklight} {
		if err := menus.Select(ctx, client, entry); err != nil {
			return false, fmt.Errorf("looking for %q: %w\n"+
				"Run \"radiocli scan\" to return the scanner to scanning", entry, err)
		}
	}

	// The scanner shows this setting as a two entry menu with the current
	// value highlighted, which is the whole of how it reports it. The
	// highlight comes off the screen rather than out of the protocol's
	// listing, because the listing can leave out rows that are really there
	// and then name the wrong one as selected.
	row, err := menus.HighlightedRow(ctx, client)
	if err != nil {
		return false, fmt.Errorf("reading the keypad light setting: %w", err)
	}

	switch row.Text {
	case enableEntry:
		return true, nil
	case disableEntry:
		return false, nil
	}
	return false, fmt.Errorf("the keypad light setting shows %q, which is neither %q nor %q",
		row.Text, enableEntry, disableEntry)
}

// renderReport writes the light's state.
//
// Parameters:
//   - app: the application context the output is written through
//   - r: the state to write
//
// Returns:
//   - error if the JSON encoding could not be written
func renderReport(app *appcontext.App, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, r)
	}

	app.Printf("backlight: %s\n", onOff(r.On))
	if r.On {
		app.Printf("level:     %d\n", r.Level)
	}
	return nil
}

// runKeysChange writes the keypad light setting and confirms it, with decide
// choosing what to write from what is already there.
//
// Written this way so a toggle costs the same one walk to the menu that setting
// it outright does. The walk is the slow part, and reading the setting to find
// out which way to go and then walking back in to set it would take twice as
// long for the same answer.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//   - decide: given the setting as it stands, returns the setting wanted
//
// Returns:
//   - error if the scanner cannot be opened, the setting cannot be written or
//     confirmed, the light cannot be cycled, or the result cannot be written
func runKeysChange(ctx context.Context, app *appcontext.App, decide func(bool) bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	changed, want, err := chooseKeys(ctx, client, decide)
	if err != nil {
		return err
	}

	// A setting changed while the light is on does not show until the light is
	// cycled, so cycle it rather than leaving something that reads as enabled
	// and looks like it is not.
	if changed {
		if state, err := read(ctx, client); err == nil && state.On {
			if _, err := flip(ctx, client, false); err != nil {
				return err
			}
			if _, err := flip(ctx, client, true); err != nil {
				return err
			}
		}
	}

	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, keysReport{Enabled: want})
	}
	app.Printf("keypad light: %s\n", onOff(want))
	return nil
}

// runKeysReport reports the keypad light setting.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the setting cannot be read, the
//     menus cannot be left, or the result cannot be written
func runKeysReport(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	on, err := readKeys(ctx, client)
	if err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, keysReport{Enabled: on})
	}
	app.Printf("keypad light: %s\n", onOff(on))
	return nil
}

// runKeysSet writes the keypad light setting and confirms it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//   - want: the setting to write
//
// Returns:
//   - error if the scanner cannot be opened, the setting cannot be written or
//     confirmed, the light cannot be cycled, or the result cannot be written
func runKeysSet(ctx context.Context, app *appcontext.App, want bool) error {
	return runKeysChange(ctx, app, func(bool) bool { return want })
}

// runKeysToggle sets the keypad light to whichever it is not.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the setting cannot be written or
//     confirmed, the light cannot be cycled, or the result cannot be written
func runKeysToggle(ctx context.Context, app *appcontext.App) error {
	return runKeysChange(ctx, app, func(on bool) bool { return !on })
}

// runOff puts the light out.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the light cannot be read or put
//     out, or the result cannot be written
func runOff(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	state, err := read(ctx, client)
	if err != nil {
		return err
	}
	if state.On {
		if state, err = flip(ctx, client, false); err != nil {
			return err
		}
	}

	return renderReport(app, report{On: state.On, Level: state.Level})
}

// runOn lights the scanner.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//   - withKeys: switch the keypad light on as well, if it is off
//
// Returns:
//   - error if the scanner cannot be opened, the keypad setting cannot be
//     written, the light cannot be read or lit, or the result cannot be written
func runOn(ctx context.Context, app *appcontext.App, withKeys bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// The keypad setting goes first. Changing it while the light is on has no
	// effect until the light is cycled, and the cycle below is what does that,
	// so doing it in this order means one pass rather than two.
	changed := false
	if withKeys {
		if changed, err = enableKeys(ctx, client); err != nil {
			return err
		}
	}

	state, err := read(ctx, client)
	if err != nil {
		return err
	}

	switch {
	case !state.On:
		// Dark, so one press lights it, keypad setting and all.
		if state, err = flip(ctx, client, true); err != nil {
			return err
		}
	case changed:
		// Lit already, but the keypad setting only takes hold when the light
		// comes on, so it has to go off and on again. This is the case that
		// looks like nothing happening if it is skipped: the setting reads as
		// enabled and the keys stay dark.
		if _, err = flip(ctx, client, false); err != nil {
			return err
		}
		if state, err = flip(ctx, client, true); err != nil {
			return err
		}
	}

	return renderReport(app, report{On: state.On, Level: state.Level})
}

// runReport reports whether the scanner is lit.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the light cannot be read, or the
//     result cannot be written
func runReport(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	state, err := read(ctx, client)
	if err != nil {
		return err
	}
	return renderReport(app, report{On: state.On, Level: state.Level})
}

// setKeys walks to the keypad light setting, sets it to want, and reads it
// back.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - want: the setting to write
//
// Returns:
//   - true when the setting had to be changed
//   - error if the setting could not be read, written, or confirmed
func setKeys(ctx context.Context, client *device.Scanner, want bool) (bool, error) {
	changed, _, err := chooseKeys(ctx, client, func(bool) bool { return want })
	return changed, err
}

// title capitalises the first letter of a word, for a sentence opening with a
// subcommand's name.
//
// Parameters:
//   - s: the word to capitalise
//
// Returns:
//   - the word with its first letter in upper case, or the empty string
//     unchanged
func title(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-('a'-'A')) + s[1:]
}
