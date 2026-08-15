// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package config

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/cmdline"
	"github.com/spf13/cobra"
)

// Macros are stored here rather than run here, which is the whole shape of this
// command. A front end reads them and sends the steps itself, one at a time, over
// the socket it already has, so a macro is a saved list of command lines and
// nothing in this tool ever executes one on its own. That is what keeps this
// package to what it says it does: nothing here opens the serial port.

// newMacro returns the "config macro" command with its subcommands attached.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config macro", with its subcommands attached
func newMacro(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "macro",
		Short: "Report the macros kept in the config file",
		Long: "Macro reports the macros kept in the config file. A macro is a name and a list\n" +
			"of command lines, stored so that a routine you repeat becomes one named thing\n" +
			"a front end can offer as a single press.\n\n" +
			"Nothing here runs a macro. This command stores them, and whatever runs them\n" +
			"sends each step exactly as if it had been typed, so a macro can do no more\n" +
			"than you could do by hand.\n\n" +
			"Run \"radiocli config macro new\" to create one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMacroList(app)
		},
	}

	cmd.AddCommand(
		newMacroShow(app),
		newMacroNew(app),
		newMacroSet(app),
		newMacroRename(app),
		newMacroMove(app),
		newMacroDelete(app),
	)
	return cmd
}

// newMacroDelete returns the "config macro delete" subcommand.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config macro delete"
func newMacroDelete(app *appcontext.App) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a macro",
		Long: "Delete removes a macro and its steps from the config file, and its button from\n" +
			"anything offering it.\n\n" +
			"There is nothing to undo it with, so it needs --yes. Nothing on the scanner is\n" +
			"touched: a macro is a list of commands rather than anything on the radio.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMacroDelete(app, args[0], yes)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
	return cmd
}

// newMacroMove returns the "config macro move" subcommand.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config macro move"
func newMacroMove(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "move <name> <up|down>",
		Short: "Move a macro up or down the list",
		Long: "Move changes where a macro sits among the others, one place at a time. The\n" +
			"order is the order the buttons appear in, so this is how they are\n" +
			"arranged.\n\n" +
			"\"up\" is towards the start of the list and the top of the panel, \"down\" is\n" +
			"towards the end and the bottom. A macro already at that end is refused, so a\n" +
			"move that could not happen is said rather than passed over.\n\n" +
			"The whole list is printed afterwards, because where one macro ended up is only\n" +
			"answerable by the order they are all in.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMacroMove(app, args[0], args[1])
		},
	}
}

// newMacroNew returns the "config macro new" subcommand.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config macro new"
func newMacroNew(app *appcontext.App) *cobra.Command {
	var keepGoing bool

	cmd := &cobra.Command{
		Use:   "new <name> <command>...",
		Short: "Create a macro",
		Long: "New writes a macro to the config file. Each argument after the name is one\n" +
			"command line, written the way you would type it without the \"radiocli\", and\n" +
			"they run in the order given.\n\n" +
			"It goes at the top of the list, which is the top of the panel a front end draws.\n" +
			"Run \"radiocli config macro move\" to put it somewhere else.\n\n" +
			"Every step is checked before anything is written: a line that could never be\n" +
			"split into a command, such as one with an unclosed quote, is refused now\n" +
			"rather than failing halfway through the macro later.\n\n" +
			"Quote each step, or the shell will hand its words over as separate steps.",
		Example: "  radiocli config macro new \"Night watch\" \"volume set 4\" \"backlight on\" scan",
		Args:    cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMacroNew(app, args[0], args[1:], keepGoing)
		},
	}

	cmd.Flags().BoolVar(&keepGoing, "keep-going", false, "run the remaining steps when one fails")
	return cmd
}

// newMacroRename returns the "config macro rename" subcommand.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config macro rename"
func newMacroRename(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <name> <new-name>",
		Short: "Change a macro's name",
		Long: "Rename changes what a macro is called, and what its button says.\n" +
			"Its steps are left alone.\n\n" +
			"Changing only the capitalization of a name is allowed. Taking the name of a\n" +
			"different macro is not.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMacroRename(app, args[0], args[1])
		},
	}
}

// newMacroSet returns the "config macro set" subcommand.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config macro set"
func newMacroSet(app *appcontext.App) *cobra.Command {
	var keepGoing bool

	cmd := &cobra.Command{
		Use:   "set <name> <command>...",
		Short: "Replace the commands a macro runs",
		Long: "Set replaces every step of a macro that already exists, and leaves its name\n" +
			"alone. There is no way to change one step: the steps are an ordered list, and\n" +
			"giving the whole list is the only way to say what order it ends up in.\n\n" +
			"Run \"radiocli config macro show\" first to see what is there now.\n\n" +
			"Whether the macro keeps going after a failure is set from the flag every time,\n" +
			"so leaving --keep-going off turns it back off.",
		Example: "  radiocli config macro set \"Night watch\" \"volume set 2\" scan",
		Args:    cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMacroSet(app, args[0], args[1:], keepGoing)
		},
	}

	cmd.Flags().BoolVar(&keepGoing, "keep-going", false, "run the remaining steps when one fails")
	return cmd
}

// newMacroShow returns the "config macro show" subcommand.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//
// Returns:
//   - *cobra.Command for "config macro show"
func newMacroShow(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Report the commands one macro runs",
		Long: "Show prints one macro's steps, one command line per line and nothing else, so\n" +
			"the list can be read straight into a script or edited and passed back to\n" +
			"\"radiocli config macro set\".\n\n" +
			"The name is matched whatever case it is typed in.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMacroShow(app, args[0])
		},
	}
}

// findMacro looks a macro up by name and reports where it is.
//
// Names match whatever case they are typed in, because a macro is picked by
// pressing a button far more often than by typing its name, and somebody typing
// one is recalling a label rather than a key.
//
// Parameters:
//   - cfg: the settings holding the macros to look in
//   - name: the macro's name as it was typed
//
// Returns:
//   - the macro of that name
//   - where it sits in the list, which is where its button sits in a front end
//   - error naming the macros there are, or saying there are none yet, when
//     nothing matches
func findMacro(cfg *appcontext.Config, name string) (appcontext.Macro, int, error) {
	wanted := strings.TrimSpace(name)

	for i, m := range cfg.Macros {
		if sameMacroName(m.Name, wanted) {
			return m, i, nil
		}
	}

	if len(cfg.Macros) == 0 {
		return appcontext.Macro{}, 0, fmt.Errorf("no macro called %q: there are no macros yet", wanted)
	}
	return appcontext.Macro{}, 0, fmt.Errorf("no macro called %q: the macros are %s", wanted, macroNames(cfg))
}

// macroName checks a name and returns it as it will be stored.
//
// Parameters:
//   - name: the name as it was typed
//
// Returns:
//   - the name as it will be stored, without the spaces around it
//   - error if the name is empty, holds a tab or a line break, or is longer
//     than a button can show
func macroName(name string) (string, error) {
	wanted := strings.TrimSpace(name)

	if wanted == "" {
		return "", fmt.Errorf("a macro needs a name")
	}
	if strings.ContainsAny(wanted, "\t\n\r") {
		return "", fmt.Errorf("a macro's name cannot contain a tab or a line break: it is what its button says")
	}
	if len([]rune(wanted)) > maxMacroName {
		return "", fmt.Errorf("the name %q is longer than %d characters: it has to fit on a button", wanted, maxMacroName)
	}

	return wanted, nil
}

// macroNames lists the macros in a sentence, for the message above.
//
// Parameters:
//   - cfg: the settings holding the macros to name
//
// Returns:
//   - every macro's name in quotes, joined as "a", "b" or "c"
func macroNames(cfg *appcontext.Config) string {
	all := make([]string, 0, len(cfg.Macros))
	for _, m := range cfg.Macros {
		all = append(all, fmt.Sprintf("%q", m.Name))
	}

	// Left in the order they are stored, which is the order they appear as
	// buttons, so reading this list and looking at a front end agree.
	return list(all)
}

// makeMacro builds a macro from what was typed, or explains why it cannot.
//
// Everything is checked here rather than at each caller, so a macro that
// reaches the file has already been proved to be one a front end can run: a name
// that fits on a button and steps that split into commands.
//
// Parameters:
//   - name: the macro's name as it was typed
//   - lines: the steps, each a command line written the way it would be typed
//     without the "radiocli"
//   - keepGoing: whether the remaining steps run after one fails
//
// Returns:
//   - the macro, ready to be written to the config file
//   - error if the name cannot be used, or a step is empty or could never be
//     split into a command
func makeMacro(name string, lines []string, keepGoing bool) (appcontext.Macro, error) {
	wanted, err := macroName(name)
	if err != nil {
		return appcontext.Macro{}, err
	}

	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		step := strings.TrimSpace(line)
		if step == "" {
			return appcontext.Macro{}, fmt.Errorf("step %d is empty: every step is a command line to run", i+1)
		}

		// Split here and thrown away. What matters is that it can be split at
		// all, because a front end sends the line as written and the server splits
		// it with this same splitter: a line refused there would fail in the
		// middle of a macro, with the steps before it already done.
		//
		// The result is not checked for being empty. The step has already been
		// trimmed and refused if nothing was left, and a line with anything in
		// it splits into at least one argument or into an error: even a pair of
		// quotes is one argument that happens to be empty.
		if _, err := cmdline.Split(step); err != nil {
			return appcontext.Macro{}, fmt.Errorf("step %d, %q: %w", i+1, step, err)
		}

		kept = append(kept, step)
	}

	if len(kept) == 0 {
		return appcontext.Macro{}, fmt.Errorf("a macro needs at least one step")
	}

	return appcontext.Macro{Name: wanted, Steps: kept, KeepGoing: keepGoing}, nil
}

// onFailure says what a macro does when a step fails, for the listing.
//
// Parameters:
//   - m: the macro to report on
//
// Returns:
//   - "keep going" or "stop"
func onFailure(m appcontext.Macro) string {
	if m.KeepGoing {
		return "keep going"
	}
	return "stop"
}

// renderMacro reads the file back and reports one macro from it.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//   - name: the macro to report
//
// Returns:
//   - error if the config file cannot be read, no macro of that name is in
//     it, or the output cannot be written
func renderMacro(app *appcontext.App, name string) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	m, _, err := findMacro(saved, name)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, m)
	}

	app.Printf("%s: %s\n", m.Name, steps(len(m.Steps)))
	for _, step := range m.Steps {
		app.Printf("  %s\n", step)
	}
	return nil
}

// runMacroDelete removes a macro.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//   - name: the macro's name as it was typed
//   - yes: whether --yes was given, which is what allows the deletion
//
// Returns:
//   - error if there is no such macro, --yes was not given, the config file
//     cannot be read or written, or the macro is still there afterwards
func runMacroDelete(app *appcontext.App, name string, yes bool) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	// Resolved before asking about --yes, so the refusal names the macro the
	// way it is stored rather than the way it was typed, and a name that does
	// not exist is reported as that rather than as a missing flag.
	existing, _, err := findMacro(saved, name)
	if err != nil {
		return err
	}

	if !yes {
		return fmt.Errorf("deleting the macro %q removes it and its %s, and cannot be undone: pass --yes",
			existing.Name, steps(len(existing.Steps)))
	}

	if err := app.Config.Update(func(c *appcontext.Config) {
		if _, at, err := findMacro(c, existing.Name); err == nil {
			c.Macros = append(c.Macros[:at], c.Macros[at+1:]...)
		}
	}); err != nil {
		return err
	}

	// Read back rather than announced, for the same reason every other write
	// here reads back: what was written is a claim, and what the file holds is
	// the answer.
	after, err := savedConfig(app)
	if err != nil {
		return err
	}
	if _, _, err := findMacro(after, existing.Name); err == nil {
		return fmt.Errorf("the macro %q is still in the config file", existing.Name)
	}

	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, struct {
			Name    string `json:"name"`
			Deleted bool   `json:"deleted"`
		}{Name: existing.Name, Deleted: true})
	}

	app.Printf("deleted: %s\n", existing.Name)
	return nil
}

// runMacroList reports every macro.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//
// Returns:
//   - error if the config file cannot be read or the output cannot be
//     written
func runMacroList(app *appcontext.App) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		// An empty list rather than null, so a reader can walk it without
		// checking which of the two it got.
		macros := saved.Macros
		if macros == nil {
			macros = []appcontext.Macro{}
		}
		return encode(app, macros)
	}

	if len(saved.Macros) == 0 {
		// On stderr, so stdout stays empty when there is nothing to list and a
		// script counting lines gets zero rather than a sentence.
		app.Notef("No macros yet. Run \"radiocli config macro new\" to create one.\n")
		return nil
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTEPS\tON FAILURE")
	for _, m := range saved.Macros {
		fmt.Fprintf(w, "%s\t%d\t%s\n", m.Name, len(m.Steps), onFailure(m))
	}
	return w.Flush()
}

// runMacroMove changes where a macro sits among the others.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//   - name: the macro's name as it was typed
//   - direction: "up" or "down", matched whatever case it is typed in
//
// Returns:
//   - error if there is no such macro, it is already at that end of the list,
//     the direction is neither of the two, or the config file cannot be read
//     or written
func runMacroMove(app *appcontext.App, name, direction string) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	existing, at, err := findMacro(saved, name)
	if err != nil {
		return err
	}

	var to int
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "up":
		if at == 0 {
			return fmt.Errorf("%q is already first: there is nothing above it to move past", existing.Name)
		}
		to = at - 1
	case "down":
		if at == len(saved.Macros)-1 {
			return fmt.Errorf("%q is already last: there is nothing below it to move past", existing.Name)
		}
		to = at + 1
	default:
		return fmt.Errorf("%q is not a direction: want up or down", direction)
	}

	if err := app.Config.Update(func(c *appcontext.Config) {
		// Found again rather than trusted, because the position above was read
		// from a copy and this runs against what is on disk.
		_, from, err := findMacro(c, existing.Name)
		if err != nil || from != at || to >= len(c.Macros) {
			return
		}
		c.Macros[from], c.Macros[to] = c.Macros[to], c.Macros[from]
	}); err != nil {
		return err
	}

	// The whole list, because where one macro ended up is only answerable by
	// the order they are all in.
	return runMacroList(app)
}

// runMacroNew creates a macro.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//   - name: the macro's name as it was typed
//   - steps: the command lines to run, in order
//   - keepGoing: whether the remaining steps run after one fails
//
// Returns:
//   - error if the name or a step cannot be used, a macro of that name
//     already exists, or the config file cannot be read or written
func runMacroNew(app *appcontext.App, name string, steps []string, keepGoing bool) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	m, err := makeMacro(name, steps, keepGoing)
	if err != nil {
		return err
	}

	// Named the way it is stored rather than the way it was typed, so somebody
	// who reached an existing macro by its capitalization can see that is what
	// happened.
	if existing, _, err := findMacro(saved, m.Name); err == nil {
		return fmt.Errorf("there is already a macro called %q: "+
			"run \"radiocli config macro set\" to change its steps, or pick another name", existing.Name)
	}

	// On the top rather than the end. A macro is made because it is wanted now,
	// and the end of the list is the far side of everything made before it,
	// which in a front end means scrolling to reach the one just written. Somewhere
	// else is a move away; the bottom of a list nobody asked to be at is not.
	if err := app.Config.Update(func(c *appcontext.Config) {
		c.Macros = append([]appcontext.Macro{m}, c.Macros...)
	}); err != nil {
		return err
	}

	return renderMacro(app, m.Name)
}

// runMacroRename changes a macro's name.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//   - name: the macro's name as it was typed
//   - newName: what to call it instead
//
// Returns:
//   - error if there is no such macro, the new name cannot be used or belongs
//     to a different macro, or the config file cannot be read or written
func runMacroRename(app *appcontext.App, name, newName string) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	existing, _, err := findMacro(saved, name)
	if err != nil {
		return err
	}

	wanted, err := macroName(newName)
	if err != nil {
		return err
	}

	// Only a different macro is in the way. A name that resolves back to this
	// one is a change of capitalization, which is a rename somebody may well
	// mean.
	if other, _, err := findMacro(saved, wanted); err == nil && !sameMacroName(other.Name, existing.Name) {
		return fmt.Errorf("there is already a macro called %q: pick another name", other.Name)
	}

	if err := app.Config.Update(func(c *appcontext.Config) {
		if _, at, err := findMacro(c, existing.Name); err == nil {
			c.Macros[at].Name = wanted
		}
	}); err != nil {
		return err
	}

	return renderMacro(app, wanted)
}

// runMacroSet replaces the steps of a macro that already exists.
//
// Parameters:
//   - app: the dependency container holding the config file to change and the
//     streams to write to
//   - name: the macro's name as it was typed
//   - steps: the command lines to run, in order, replacing the ones it has
//   - keepGoing: whether the remaining steps run after one fails
//
// Returns:
//   - error if there is no such macro, a step cannot be used, or the config
//     file cannot be read or written
func runMacroSet(app *appcontext.App, name string, steps []string, keepGoing bool) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	existing, _, err := findMacro(saved, name)
	if err != nil {
		return err
	}

	// The stored name wins over the typed one, so setting the steps of "night
	// watch" does not quietly recapitalize the button.
	m, err := makeMacro(existing.Name, steps, keepGoing)
	if err != nil {
		return err
	}

	if err := app.Config.Update(func(c *appcontext.Config) {
		if _, at, err := findMacro(c, m.Name); err == nil {
			c.Macros[at] = m
		}
	}); err != nil {
		return err
	}

	return renderMacro(app, m.Name)
}

// runMacroShow reports one macro's steps.
//
// Parameters:
//   - app: the dependency container holding the output format and the
//     streams to write to
//   - name: the macro's name as it was typed
//
// Returns:
//   - error if the config file cannot be read, no macro of that name is in
//     it, or the output cannot be written
func runMacroShow(app *appcontext.App, name string) error {
	saved, err := savedConfig(app)
	if err != nil {
		return err
	}

	m, _, err := findMacro(saved, name)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return encode(app, m)
	}

	// The steps alone, one per line, so the whole answer can be used without
	// anything having to be cut off it first.
	for _, step := range m.Steps {
		app.Printf("%s\n", step)
	}
	return nil
}

// sameMacroName reports whether two names refer to the same macro.
//
// Parameters:
//   - a: one name
//   - b: the other name
//
// Returns:
//   - true if the two name the same macro, whatever case they are typed in
func sameMacroName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// steps renders a step count with its noun, so a message reads as a sentence
// rather than as "1 steps".
//
// Parameters:
//   - n: how many steps there are
//
// Returns:
//   - the count with its noun, such as "1 step" or "3 steps"
func steps(n int) string {
	if n == 1 {
		return "1 step"
	}
	return fmt.Sprintf("%d steps", n)
}
