// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package config implements the "config" command, which reports and changes
// the tool's own settings.
//
// Everything here belongs to radiocli rather than to the scanner. Nothing in
// this package opens the serial port or sends anything to the radio, which is
// the line that separates it from every other command in the tool: "volume" is
// how loud the scanner plays, "config set pace" is how this program behaves.
//
// The bare command reports. Changing anything is a separate verb, so no reading
// of the settings can turn into a write by mistake.
package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the config command, with its subcommands attached, bound to app.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config", with its subcommands attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Report the tool's own settings",
		Long: "Config reports the settings this tool keeps for itself: how to render results,\n" +
			"how quickly to send keys, and whether to log the exchange.\n\n" +
			"These are settings of radiocli, not of the scanner. Nothing here touches the\n" +
			"radio, and no scanner needs to be attached.\n\n" +
			"What is reported is what the config file holds, not what this run resolved:\n" +
			"a flag given on this command line changes this run and is not written down.\n\n" +
			"Run \"radiocli config set\" to change one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(app)
		},
	}

	cmd.AddCommand(newGet(app), newSet(app), newUnset(app), newPath(app), newMacro(app))
	return cmd
}

// newGet returns the "config get" subcommand.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config get"
func newGet(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Report one setting",
		Long: "Get prints one setting and nothing else, so it can be read straight into a\n" +
			"variable.\n\n" +
			"A setting that has never been set prints its default rather than nothing,\n" +
			"because the default is what is in effect.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(app, args[0])
		},
	}
}

// newPath returns the "config path" subcommand.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config path"
func newPath(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Report which config file is being used",
		Long: "Path prints the config file this invocation reads and writes: the one named by\n" +
			"--config, or the default one in the user config directory.\n\n" +
			"It prints the path whether or not the file exists, because the path is the\n" +
			"answer either way: that is where a setting would be written.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPath(app)
		},
	}
}

// newSet returns the "config set" subcommand.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config set"
func newSet(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <value>",
		Short: "Change one setting",
		Long: "Set writes one setting to the config file, creating the file if it is not\n" +
			"there yet, and leaves every other setting alone.\n\n" +
			"The value is checked before anything is written, so a value the tool cannot\n" +
			"use is refused rather than saved and tripped over later.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSet(app, args[0], args[1])
		},
	}
}

// newUnset returns the "config unset" subcommand.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config unset"
func newUnset(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <name>",
		Short: "Put one setting back to its default",
		Long: "Unset puts one setting back to the value it has when nothing is set, and\n" +
			"prints what that turned out to be.\n\n" +
			"There is nothing to undo it with, but nothing is lost either: every setting\n" +
			"here is a preference rather than something the tool discovered.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnset(app, args[0])
		},
	}
}

// allowed reports whether value is one of the accepted ones.
//
// Parameters:
//   - values: the accepted values
//   - value: the value to look for
//
// Returns:
//   - true if value is one of values
func allowed(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// apply parses a value onto cfg and checks that the result is usable.
//
// Checked twice on purpose. The list of accepted values names them, which is
// what somebody who guessed wrong needs; Validate is what the tool itself
// enforces, and a setting that passed the first check but not the second would
// be a setting this command allowed and every other command refused.
//
// Parameters:
//   - s: the setting being changed
//   - cfg: the settings to change, modified in place
//   - value: the value as it was typed
//
// Returns:
//   - error naming the setting if the value is not one it accepts or leaves
//     cfg unusable, nil when the value was applied
func apply(s setting, cfg *appcontext.Config, value string) error {
	if len(s.Values) > 0 && !allowed(s.Values, value) {
		return fmt.Errorf("invalid %s %q: want %s", s.Name, value, list(s.Values))
	}

	if err := s.set(cfg, value); err != nil {
		return fmt.Errorf("%s: %w", s.Name, err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("%s: %w", s.Name, err)
	}
	return nil
}

// describe turns one setting into the shape both formats render.
//
// Parameters:
//   - s: the setting to describe
//   - saved: the settings as the config file has them
//
// Returns:
//   - report holding the setting's name, the value in effect, the default,
//     and whether the two differ
func describe(s setting, saved *appcontext.Config) report {
	value := s.get(saved)
	fallback := s.get(appcontext.Defaults())

	return report{
		Name:    s.Name,
		Value:   value,
		Default: fallback,
		Changed: value != fallback,
	}
}

// encode writes v as JSON.
//
// Parameters:
//   - app: the dependency container holding the stream to write to
//   - v: the value to render
//
// Returns:
//   - error if the JSON could not be written
func encode(app *appcontext.App, v any) error {
	return render.JSON(app.Stdout, v)
}

// list renders accepted values as "a, b, c or d".
//
// Parameters:
//   - values: the values to join, in the order they should be read
//
// Returns:
//   - the values joined by commas with "or" before the last, empty for no
//     values and the value alone for one
func list(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	}
	return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
}

// lookup finds a setting by name.
//
// Parameters:
//   - name: the setting's name as it was typed, matched whatever case and
//     spacing it has
//
// Returns:
//   - the setting of that name
//   - error saying why the name is not one, with the reason when it is a name
//     this command will never accept and the settings there are otherwise
func lookup(name string) (setting, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, s := range settings {
		if s.Name == name {
			return s, nil
		}
	}

	if why, ok := notSettings[name]; ok {
		return setting{}, fmt.Errorf("%q is not a setting: %s", name, why)
	}
	return setting{}, fmt.Errorf("no setting called %q: the settings are %s", name, names())
}

// names lists the settings in a sentence, for the message above.
//
// Returns:
//   - every setting's name, alphabetically, separated by commas
func names() string {
	all := make([]string, 0, len(settings))
	for _, s := range settings {
		all = append(all, s.Name)
	}
	sort.Strings(all)

	return strings.Join(all, ", ")
}

// paceValues lists the paces as plain strings, which is the shape the table
// above wants. device keeps them as its own type and renders them for a
// sentence rather than for a list.
//
// Returns:
//   - every pace name, in the order device keeps them
func paceValues() []string {
	values := make([]string, 0, len(device.Paces))
	for _, p := range device.Paces {
		values = append(values, string(p))
	}
	return values
}

// renderSaved reads the file back and reports one setting from it.
//
// Read back rather than echoed, for the same reason the scanner commands read
// back: what was written is a claim, and what the file holds is the answer.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//   - s: the setting to report
//
// Returns:
//   - error if the config file cannot be read or the output cannot be
//     written
func renderSaved(app *appcontext.App, s setting) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	r := describe(s, saved)
	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, r)
	}

	app.Printf("%s: %s\n", r.Name, shown(r.Value))
	return nil
}

// runGet reports one setting.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//   - name: the setting's name as it was typed
//
// Returns:
//   - error if the name is not a setting, the config file cannot be read, or
//     the output cannot be written
func runGet(app *appcontext.App, name string) error {
	s, err := lookup(name)
	if err != nil {
		return err
	}

	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	r := describe(s, saved)
	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, r)
	}

	// The value alone, with no name and no padding, so the whole line can be
	// used without anything having to be cut off it first.
	app.Printf("%s\n", r.Value)
	return nil
}

// runList reports every setting.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//
// Returns:
//   - error if the config file cannot be read or the output cannot be
//     written
func runList(app *appcontext.App) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	reports := make([]report, 0, len(settings))
	for _, s := range settings {
		reports = append(reports, describe(s, saved))
	}

	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, reports)
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVALUE\tDEFAULT")
	for _, r := range reports {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, shown(r.Value), shown(r.Default))
	}
	return w.Flush()
}

// runPath reports which file is being used.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//
// Returns:
//   - error if the config file's location cannot be worked out or the output
//     cannot be written
func runPath(app *appcontext.App) error {
	path, err := app.Config.Location()
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		_, statErr := os.Stat(path)
		return encode(app, struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
		}{Path: path, Exists: statErr == nil})
	}

	app.Printf("%s\n", path)

	// On stderr, so the path is still the only thing on stdout. A file that is
	// not there yet is worth saying once, because "config" printing defaults
	// and "config path" printing a path look identical either way.
	if _, err := os.Stat(path); err != nil {
		app.Notef("That file does not exist yet. It is written the first time a setting is set.\n")
	}
	return nil
}

// runSet changes one setting.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//   - name: the setting's name as it was typed
//   - value: the value to store, as it was typed
//
// Returns:
//   - error if the name is not a setting, the value is not one it accepts, or
//     the config file cannot be read or written
func runSet(app *appcontext.App, name, value string) error {
	s, err := lookup(name)
	if err != nil {
		return err
	}

	// Checked against a copy of what is saved before anything is written. A
	// value that does not survive Validate would otherwise be stored and then
	// refused by every command afterwards, including the one that would put it
	// back.
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}
	if err := apply(s, saved, value); err != nil {
		return err
	}

	if err := app.Config.Update(func(c *appcontext.Config) {
		// Already parsed and validated above, on a copy holding the same
		// settings, so this cannot fail here.
		_ = s.set(c, s.get(saved))
	}); err != nil {
		return err
	}

	return renderSaved(app, s)
}

// runUnset puts one setting back to its default.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//   - name: the setting's name as it was typed
//
// Returns:
//   - error if the name is not a setting, or the config file cannot be read
//     or written
func runUnset(app *appcontext.App, name string) error {
	s, err := lookup(name)
	if err != nil {
		return err
	}

	fallback := s.get(appcontext.Defaults())
	if err := app.Config.Update(func(c *appcontext.Config) {
		_ = s.set(c, fallback)
	}); err != nil {
		return err
	}

	return renderSaved(app, s)
}

// shown renders a value for a person reading it. An empty setting is a real
// state rather than a missing one, and a blank column would read as a bug.
//
// Parameters:
//   - value: the value to render
//
// Returns:
//   - the value, or "-" when it is empty
func shown(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
