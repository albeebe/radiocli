// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package key implements the "key" command, which presses keys on the
// scanner's front panel.
//
// This is the one command that drives the scanner's own interface rather than
// asking the remote protocol for something. It can reach anything a person can
// reach, which is why it exists, and it is the least predictable thing in the
// tool, because what a key does depends entirely on what is on screen.
package key

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the key command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that presses the keys it is given when it runs
func New(app *appcontext.App) *cobra.Command {
	var action string

	cmd := &cobra.Command{
		Use:   "key <name>...",
		Short: "Press keys on the scanner",
		Long: "Key presses keys on the scanner's front panel, as though someone had pressed\n" +
			"them. Several keys may be given, and are pressed in the order written.\n\n" +
			"What a key does depends on what is on the scanner's screen, so this is the\n" +
			"least predictable way to control it. Prefer a command that names what it does\n" +
			"when one exists.\n\n" +
			"Presses are paced by --pace, which is the gap left between one key and the\n" +
			"next so the scanner has time to redraw.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Everything is resolved before the scanner is opened, so a
			// misspelled key in a run of five presses none of them.
			pressed, err := resolve(args)
			if err != nil {
				return err
			}

			name := strings.ToLower(action)
			how, ok := actions[name]
			if !ok {
				return fmt.Errorf("invalid action %q: want %s", action, options(actionNames()))
			}

			return run(cmd.Context(), app, pressed, name, how)
		},
	}

	cmd.Flags().StringVar(&action, "action", "press",
		"how to press: "+options(actionNames()))
	return cmd
}

// actionNames lists the accepted action names in a stable order.
//
// Returns:
//   - []string naming every way a key can be pressed, sorted
func actionNames() []string {
	names := make([]string, 0, len(actions))
	for name := range actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// done names the keys already pressed, for a failure message.
//
// Parameters:
//   - pressed: the keys that were pressed before the one that failed
//
// Returns:
//   - string naming those keys in the order they were pressed
func done(pressed []press) string {
	names := make([]string, 0, len(pressed))
	for _, p := range pressed {
		names = append(names, p.name)
	}
	return strings.Join(names, ", ")
}

// keyNames lists the accepted key names in a stable order.
//
// Returns:
//   - []string naming every key this command accepts, sorted
func keyNames() []string {
	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// options renders a list of accepted values for an error or a flag's help.
//
// Parameters:
//   - names: the accepted values, already in the order they should read
//
// Returns:
//   - string holding the values separated by commas
func options(names []string) string {
	return strings.Join(names, ", ")
}

// resolve turns the names into keys, rejecting the whole run if any is unknown.
//
// Parameters:
//   - args: the key names as the user wrote them, in the order to press them
//
// Returns:
//   - []press holding each key alongside the name it was given by
//   - error naming the first unknown key, and listing the ones that exist
func resolve(args []string) ([]press, error) {
	pressed := make([]press, 0, len(args))
	for _, arg := range args {
		name := strings.ToLower(arg)
		k, ok := keys[name]
		if !ok {
			return nil, fmt.Errorf("no key is called %q: want %s", arg, options(keyNames()))
		}
		pressed = append(pressed, press{name: name, key: k})
	}
	return pressed, nil
}

// run presses the keys in order.
//
// Nothing is written on success in text mode, and that is not an oversight: the
// result of pressing a key is on the scanner's screen, and a line saying the
// keys were pressed would add nothing a caller did not already know. Under
// --output json silence is a different thing, because empty stdout is not
// something a decoder can read, so the report goes out there.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection and the streams
//   - pressed: the keys to press, in order
//   - action: what the caller asked for by name, echoed in the JSON report
//   - how: the way to press them, such as a tap or a hold
//
// Returns:
//   - error if the scanner cannot be reached or a key cannot be pressed,
//     naming the key it stopped on and how far it got; nil once every key has
//     been pressed
func run(ctx context.Context, app *appcontext.App, pressed []press, action string, how device.KeyAction) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	for i, p := range pressed {
		if err := client.PressKey(ctx, p.key, how); err != nil {
			// Say how far it got. A run that stopped in the middle has left
			// the scanner somewhere neither the reader nor the tool intended,
			// and knowing which key it reached is the only way to work out
			// where.
			if i > 0 {
				return fmt.Errorf("pressing %q, after %s: %w", p.name, done(pressed[:i]), err)
			}
			return fmt.Errorf("pressing %q: %w", p.name, err)
		}
	}

	if app.Config.Output != appcontext.OutputJSON {
		return nil
	}

	// The names the caller used, not the protocol's letters, so the report
	// reads back as the command that produced it.
	names := make([]string, 0, len(pressed))
	for _, p := range pressed {
		names = append(names, p.name)
	}
	return render.JSON(app.Stdout, report{Keys: names, Action: action})
}
