// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package departments implements the "departments" command, which lists the
// departments inside one system.
//
// It is the third and deepest level of the scanner's memory that can be read.
// Departments hold the channels, but the request that would return those
// answers with the wrong document on the tested firmware, so this is as far
// down as the tool goes.
package departments

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/navigate"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/albeebe/radiocli/internal/textinput"
	"github.com/spf13/cobra"
)

// New returns the departments command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "departments" command, with its subcommands attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "departments <system>",
		Short: "List the departments in a system",
		Long: "Departments lists the departments inside one system, and says which of them the\n" +
			"scanner is skipping. A department is a group of channels, such as the police\n" +
			"channels of one town.\n\n" +
			"The system may be named by its index, as reported by \"radiocli systems\", or\n" +
			"by its name. A name is more work than it looks: a system's index says nothing\n" +
			"about which favorites list holds it, so every list has to be read to find it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app, args[0])
		},
	}

	cmd.AddCommand(newGoto(app), newRename(app), newNew(app), newDelete(app))
	return cmd
}

// newDelete returns the "departments delete" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "departments delete" subcommand, carrying its --yes flag
func newDelete(app *appcontext.App) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <department> --yes",
		Short: "Delete a department and every channel in it",
		Long: "Delete removes a department from the system holding it, along with every\n" +
			"channel inside it.\n\n" +
			"There is no undo. Nothing on the scanner keeps a copy, and this tool cannot\n" +
			"put back what it removes, so --yes is required: without it the command says\n" +
			"what would go and changes nothing.\n\n" +
			"The departments are read back afterwards to confirm it is gone. This stops\n" +
			"the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), app, args[0], yes)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
	return cmd
}

// newGoto returns the "departments goto" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "departments goto" subcommand
func newGoto(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "goto <department>",
		Short: "Open a department's menu on the scanner",
		Long: "Goto puts the scanner into the menu for one department, where its name, its\n" +
			"quick key, and its channels can be edited.\n\n" +
			"The department may be named by its index or by its name. Naming it by name is\n" +
			"the most expensive lookup in this tool: a department's index says nothing about\n" +
			"which system or favorites list holds it, so every one of them has to be read.\n\n" +
			"This stops the scanner scanning. Run \"radiocli menu close\" when finished,\n" +
			"or \"radiocli key avoid\" from a screen that refuses it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := app.Device(ctx)
			if err != nil {
				return err
			}

			index, err := catalog.ResolveDepartment(ctx, client, args[0])
			if err != nil {
				return err
			}

			return menus.Open(ctx, app, client, device.MenuDepartment, index, "department")
		},
	}
}

// newNew returns the "departments new" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "departments new" subcommand
func newNew(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "new <system> <name>",
		Short: "Create a department in a system",
		Long: "New creates a department inside one system and gives it a name.\n\n" +
			"The scanner has no command for this, so it is done through its menus. Pressing\n" +
			"New Department creates one immediately, called something like \"DEPARTMENT 0\",\n" +
			"and this then types the name you asked for over it.\n\n" +
			"The department is read back afterwards to confirm it is there. This stops the\n" +
			"scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd.Context(), app, args[0], args[1])
		},
	}
}

// newRename returns the "departments rename" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "departments rename" subcommand
func newRename(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <department> <name>",
		Short: "Change a department's name",
		Long: "Rename changes the name of one department.\n\n" +
			"The scanner has no command for this, so it is done the way a person would: the\n" +
			"department's menu is opened, its Edit Name screen selected, and the name typed\n" +
			"there by turning the knob to each character in turn. Every character is checked\n" +
			"after it is set.\n\n" +
			"Nothing is saved until the whole name is typed, so an interrupted rename leaves\n" +
			"the old name untouched. This stops the scanner scanning, and returns it to\n" +
			"scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(cmd.Context(), app, args[0], args[1])
		},
	}
}

// confirm reports a system that does not exist, for the case where an empty
// answer might mean either that or a system holding nothing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the systems from
//   - system: the system as it was named on the command line
//   - index: the index that naming resolved to
//
// Returns:
//   - error if the systems cannot be read, or if no system has that index
func confirm(ctx context.Context, client *device.Scanner, system, index string) error {
	// A name was already looked up to get here, so the system is known to
	// exist and the empty answer means it holds nothing.
	if !catalog.IsIndex(system) {
		return nil
	}

	all, err := catalog.EverySystem(ctx, client)
	if err != nil {
		return err
	}

	for _, s := range all {
		if s.Index == index {
			return nil
		}
	}
	return fmt.Errorf("no system has index %s: %s", index, names(all))
}

// departmentName is the scanner's own name for the department at an index, so
// that what gets reported is what the scanner calls it rather than what was
// typed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the departments from
//   - index: the department's index
//
// Returns:
//   - the scanner's own name for the department at index
//   - error if the departments cannot be read, or if no department has that
//     index
func departmentName(ctx context.Context, client *device.Scanner, index string) (string, error) {
	all, err := catalog.EveryDepartment(ctx, client)
	if err != nil {
		return "", err
	}
	for _, d := range all {
		if d.Index == index {
			return d.Name, nil
		}
	}
	return "", fmt.Errorf("no department has index %s", index)
}

// names lists what the reader could have meant, since being told the name was
// wrong without being told the alternatives means running another command.
//
// Parameters:
//   - all: every system the scanner's favorites lists hold
//
// Returns:
//   - the names the reader could have meant, or a note that there are none
func names(all []catalog.System) string {
	if len(all) == 0 {
		return "the scanner's favorites lists hold no systems at all"
	}

	quoted := make([]string, 0, len(all))
	for _, s := range all {
		quoted = append(quoted, fmt.Sprintf("%q", s.Name))
	}
	return "the scanner has " + strings.Join(quoted, ", ")
}

// renderDepartments writes the listing as an aligned table.
//
// A system holding no departments is a complete answer, not a failure, so this
// returns nil and puts the explanation on stderr.
//
// Parameters:
//   - app: the application context holding the streams to write to
//   - found: the departments to renderDepartments, in the order the scanner reports them
//
// Returns:
//   - error if the table cannot be written to the output stream
func renderDepartments(app *appcontext.App, found []catalog.Department) error {
	if len(found) == 0 {
		app.Notef("That system holds no departments.\n")
		return nil
	}

	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSCANNED\tQUICK KEY")

	for _, d := range found {
		fmt.Fprintf(w, "%s\t%s\t%s\n", d.Name, render.YesNo(!d.Avoided), render.Dash(d.QuickKey))
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the department list: %w", err)
	}
	return nil
}

// run resolves the system, reads its departments, and renders them.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - system: the system's index, or its name
//
// Returns:
//   - error if the scanner cannot be reached, the system cannot be resolved or
//     confirmed, its departments cannot be read, or the output cannot be
//     written
func run(ctx context.Context, app *appcontext.App, system string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveSystem(ctx, client, system)
	if err != nil {
		return err
	}

	found, err := catalog.ReadDepartments(ctx, client, index)
	if err != nil {
		return err
	}

	// An index naming no system answers exactly as a system holding nothing
	// does, so the difference has to be settled by looking the index up. That
	// is only worth the extra exchanges when there is nothing to report anyway.
	if len(found) == 0 {
		if err := confirm(ctx, client, system, index); err != nil {
			return err
		}
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, found)
	}

	return renderDepartments(app, found)
}

// runDelete removes a department and confirms it is gone.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - want: the department's index, or its name
//   - yes: whether --yes was passed, without which nothing is deleted
//
// Returns:
//   - error if the scanner cannot be reached, the department cannot be resolved
//     or named, --yes was not passed, its menu cannot be opened or its delete
//     entry pressed, or the department is still there afterwards
func runDelete(ctx context.Context, app *appcontext.App, want string, yes bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Resolving first means a name the scanner does not have is reported as
	// that, rather than as a missing flag.
	index, err := catalog.ResolveDepartment(ctx, client, want)
	if err != nil {
		return err
	}

	name, err := departmentName(ctx, client, index)
	if err != nil {
		return err
	}

	if !yes {
		return fmt.Errorf("deleting the department %q removes it and every channel in it, "+
			"and cannot be undone: pass --yes", name)
	}

	if err := client.OpenMenu(ctx, device.MenuDepartment, index); err != nil {
		return fmt.Errorf("opening the department's menu: %w", err)
	}
	if err := menus.Select(ctx, client, deleteDepartment); err != nil {
		return fmt.Errorf("looking for %q on the department's menu: %w", deleteDepartment, err)
	}
	if err := menus.ConfirmDelete(ctx, client); err != nil {
		return err
	}
	if err := menus.Awaken(ctx, client); err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	all, err := catalog.EveryDepartment(ctx, client)
	if err != nil {
		return err
	}
	for _, d := range all {
		if d.Index == index {
			return fmt.Errorf("the department %q is still there afterwards: nothing was deleted", name)
		}
	}

	return render.Changed(app,
		render.Mutation{Action: "deleted", Kind: "department", Name: name},
		"deleted "+name)
}

// runNew creates a department and names it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - system: the system to create the department in, by index or by name
//   - name: the name to give the new department
//
// Returns:
//   - error if the scanner cannot be reached, the system cannot be opened, the
//     department cannot be created or named, or it does not appear afterwards
func runNew(ctx context.Context, app *appcontext.App, system, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if err := navigate.ToDepartments(ctx, client, system); err != nil {
		return err
	}

	// The scanner creates the department the moment this is pressed, with a
	// name of its own choosing, and opens its menu.
	if err := menus.Select(ctx, client, newDepartment); err != nil {
		return fmt.Errorf("creating the department: %w", err)
	}

	if err := menus.Select(ctx, client, editNameEntry); err != nil {
		return fmt.Errorf("the department was created, but its name screen could not be opened: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		return fmt.Errorf("the department was created, but naming it failed: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read it back. The scanner is the only authority on whether this worked,
	// and a created-then-unnamed department is worth knowing about.
	all, err := catalog.EveryDepartment(ctx, client)
	if err != nil {
		return err
	}
	for _, d := range all {
		if d.Name == name {
			return render.Changed(app,
				render.Mutation{Action: "created", Kind: "department", Name: name, In: system}, name)
		}
	}
	return fmt.Errorf("the department does not appear under %q afterwards: "+
		"run \"radiocli departments %s\" to see what was created", name, system)
}

// runRename opens a department's menu and types a new name into it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - want: the department's index, or its name
//   - name: the name to type over the old one
//
// Returns:
//   - error if the scanner cannot be reached, the department cannot be resolved,
//     its menu or name screen cannot be opened, the name cannot be typed, or the
//     menus cannot be left
func runRename(ctx context.Context, app *appcontext.App, want, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveDepartment(ctx, client, want)
	if err != nil {
		return err
	}

	// One exchange, like a system: the protocol takes the index directly.
	if err := client.OpenMenu(ctx, device.MenuDepartment, index); err != nil {
		return fmt.Errorf("opening the department's menu: %w", err)
	}

	if err := menus.StepTo(ctx, client, editName); err != nil {
		return fmt.Errorf("looking for %q on the department's menu: %w", editName, err)
	}
	if err := menus.Enter(ctx, client); err != nil {
		return err
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		// Leaving the entry screen discards, so nothing has been saved. Say so,
		// rather than leaving the reader wondering whether the name is mangled.
		return fmt.Errorf("typing the new name: %w\n"+
			"The scanner is still on the entry screen and nothing has been saved. "+
			"Run \"radiocli scan\" to leave it as it was", err)
	}

	if err := render.Changed(app,
		render.Mutation{Action: "renamed", Kind: "department", Name: name},
		name); err != nil {
		return err
	}

	_, err = menus.Leave(ctx, client)
	return err
}
