// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package systems implements the "systems" command, which lists the systems
// inside one favorites list.
//
// It is the second level of the scanner's memory. The favorites list above is
// named by index or by name, and each system reported here carries the index
// that names it in turn.
package systems

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

// New returns the systems command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "systems" command, with its subcommands attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "systems <list>",
		Short: "List the systems in a favorites list",
		Long: "Systems lists the systems inside one favorites list, and says which of them the\n" +
			"scanner is skipping.\n\n" +
			"The list may be named by its index, as reported by \"radiocli favorites\", or\n" +
			"by its name. A name costs one extra exchange with the scanner, because the\n" +
			"favorites lists have to be read to find which index it refers to.\n\n" +
			"The built-in scan sources are refused, by either name or index. Asking the\n" +
			"full database for its systems locks the scanner up until it is power cycled.\n" +
			"Use \"radiocli scanning systems\" to read the database instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app, args[0])
		},
	}

	cmd.AddCommand(newGoto(app), newRename(app), newNew(app), newDelete(app))
	return cmd
}

// newDelete returns the "systems delete" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "systems delete" subcommand, carrying its own --yes flag
func newDelete(app *appcontext.App) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <system> --yes",
		Short: "Delete a system and everything in it",
		Long: "Delete removes a system from the favorites list holding it, along with every\n" +
			"department and channel inside it.\n\n" +
			"There is no undo. Nothing on the scanner keeps a copy, and this tool cannot\n" +
			"put back what it removes, so --yes is required: without it the command says\n" +
			"what would go and changes nothing.\n\n" +
			"The systems are read back afterwards to confirm it is gone. This stops the\n" +
			"scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), app, args[0], yes)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
	return cmd
}

// newGoto returns the "systems goto" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "systems goto" subcommand
func newGoto(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "goto <system>",
		Short: "Open a system's menu on the scanner",
		Long: "Goto puts the scanner into the menu for one system, where its name, its\n" +
			"options, and its departments can be edited.\n\n" +
			"The system may be named by its index or by its name. This is a single jump\n" +
			"rather than a walk through the menus, so it lands in one step whatever the\n" +
			"scanner was showing before.\n\n" +
			"This stops the scanner scanning. Run \"radiocli menu close\" when finished,\n" +
			"or \"radiocli key avoid\" from a screen that refuses it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := app.Device(ctx)
			if err != nil {
				return err
			}

			index, err := navigate.ResolveSystem(ctx, client, args[0])
			if err != nil {
				return err
			}

			return menus.Open(ctx, app, client, device.MenuSystem, index, "system")
		},
	}
}

// newNew returns the "systems new" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "systems new" subcommand, carrying its own --type flag
func newNew(app *appcontext.App) *cobra.Command {
	var kind string

	cmd := &cobra.Command{
		Use:   "new <list> <name> --type <type>",
		Short: "Create a system in a favorites list",
		Long: "New creates a system inside one favorites list and gives it a name.\n\n" +
			"The type must be given. The scanner asks for it before the system exists and\n" +
			"a system's type cannot be changed afterwards, so there is nothing sensible to\n" +
			"default to: getting it wrong means deleting the system and starting again.\n\n" +
			"Accepted types are " + strings.Join(SystemTypes, ", ") + ".\n\n" +
			"The system is read back afterwards to confirm it is there. This stops the\n" +
			"scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			matched, err := matchType(kind)
			if err != nil {
				return err
			}
			return runNew(cmd.Context(), app, args[0], args[1], matched)
		},
	}

	cmd.Flags().StringVar(&kind, "type", "", "the kind of system to create: "+strings.Join(SystemTypes, ", "))
	cmd.MarkFlagRequired("type")
	return cmd
}

// newRename returns the "systems rename" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "systems rename" subcommand
func newRename(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <system> <name>",
		Short: "Change a system's name",
		Long: "Rename changes the name of one system.\n\n" +
			"The scanner has no command for this, so it is done the way a person would: the\n" +
			"system's menu is opened, its Edit Name screen selected, and the name typed\n" +
			"there by turning the knob to each character in turn. Every character is checked\n" +
			"after it is set.\n\n" +
			"It takes a while at the default pace, because every character is a key press.\n" +
			"Use --pace turbo to speed it up. This stops the scanner scanning, and returns\n" +
			"it to scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(cmd.Context(), app, args[0], args[1])
		},
	}
}

// confirm reports a favorites list that does not exist, for the case where an
// empty answer might mean either that or a list holding nothing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the favorites lists from
//   - list: the favorites list as it was named on the command line
//   - index: the index that naming resolved to
//
// Returns:
//   - error if the favorites lists cannot be read, or if no list has that
//     index, and nil when the list exists and simply holds nothing
func confirm(ctx context.Context, client *device.Scanner, list, index string) error {
	// A name was already looked up to get here, so the list is known to exist
	// and the empty answer means it holds nothing.
	if !catalog.IsIndex(list) {
		return nil
	}

	lists, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}

	for _, l := range lists {
		if l.Index == index {
			return nil
		}
	}
	return fmt.Errorf("no favorites list has index %s: %s", index, names(lists))
}

// matchType turns what was typed into the entry the scanner offers, so that
// case and spacing do not have to be reproduced exactly.
//
// Parameters:
//   - want: the system type as it was typed
//
// Returns:
//   - the type as the scanner spells it on its own menu
//   - error if no system type is called that
func matchType(want string) (string, error) {
	for _, t := range SystemTypes {
		if strings.EqualFold(strings.TrimSpace(want), t) {
			return t, nil
		}
	}
	return "", fmt.Errorf("no system type is called %q: want %s", want, strings.Join(SystemTypes, ", "))
}

// names lists what the reader could have meant, since being told the name was
// wrong without being told the alternatives means running another command.
//
// Parameters:
//   - lists: the favorites lists the scanner reports
//
// Returns:
//   - the names in quotes, or a note that the scanner reports no lists at all
func names(lists []catalog.FavoritesList) string {
	if len(lists) == 0 {
		return "the scanner reports no favorites lists"
	}

	quoted := make([]string, 0, len(lists))
	for _, l := range lists {
		quoted = append(quoted, fmt.Sprintf("%q", l.Name))
	}
	return "the scanner has " + strings.Join(quoted, ", ")
}

// renderSystems writes the listing as an aligned table.
//
// A favorites list holding no systems is a complete answer, not a failure, so
// this returns nil and puts the explanation on stderr.
//
// Parameters:
//   - app: the application context holding the streams to write to
//   - found: the systems to renderSystems, in the order the scanner reports them
//
// Returns:
//   - error if the table cannot be written to the output stream
func renderSystems(app *appcontext.App, found []catalog.System) error {
	if len(found) == 0 {
		app.Notef("That favorites list holds no systems.\n")
		return nil
	}

	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tSCANNED\tQUICK KEY\tNUMBER TAG")

	filled := 0
	for _, s := range found {
		if s.Partial {
			filled++
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.Name, render.Unread, render.Unread, render.Unread, render.Unread)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.Name, s.Kind, render.YesNo(!s.Avoided), render.Dash(s.QuickKey), render.Dash(s.NumberTag))
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the system list: %w", err)
	}

	render.Filled(app, "systems", filled)
	return nil
}

// run resolves the favorites list, reads its systems, and renders them.
//
// A built-in scan source is refused by the read rather than here, so that a
// name and an index are refused by the same check: naming the full database is
// the mistake that costs a power cycle, and it can be named either way.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner, the output format and
//     the streams to write to
//   - list: the favorites list's index, or its name
//
// Returns:
//   - error if the scanner cannot be reached, the list cannot be resolved or
//     does not exist, it is a built-in scan source, its systems cannot be read,
//     or the listing cannot be written
func run(ctx context.Context, app *appcontext.App, list string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := navigate.ResolveFavorites(ctx, client, list)
	if err != nil {
		return err
	}

	found, err := navigate.ReadSystems(ctx, client, index)
	if err != nil {
		return err
	}

	// An index naming no list answers exactly as an empty list does, so the
	// difference has to be settled by looking the index up. This is only worth
	// an extra exchange when there is nothing to report anyway.
	if len(found) == 0 {
		if err := confirm(ctx, client, list, index); err != nil {
			return err
		}
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, found)
	}

	return renderSystems(app, found)
}

// runDelete removes a system and confirms it is gone.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - want: the system's index, or its name
//   - yes: whether --yes was passed, without which nothing is deleted
//
// Returns:
//   - error if the scanner cannot be reached, the system cannot be resolved or
//     named, --yes was not passed, its menu cannot be opened or its delete
//     entry pressed, or the system is still there afterwards
func runDelete(ctx context.Context, app *appcontext.App, want string, yes bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Resolving first means a name the scanner does not have is reported as
	// that, rather than as a missing flag.
	index, err := navigate.ResolveSystem(ctx, client, want)
	if err != nil {
		return err
	}

	name, err := systemName(ctx, client, index)
	if err != nil {
		return err
	}

	if !yes {
		return fmt.Errorf("deleting the system %q removes it and every department and "+
			"channel in it, and cannot be undone: pass --yes", name)
	}

	if err := client.OpenMenu(ctx, device.MenuSystem, index); err != nil {
		return fmt.Errorf("opening the system's menu: %w", err)
	}
	if err := menus.Select(ctx, client, deleteSystem); err != nil {
		return fmt.Errorf("looking for %q on the system's menu: %w", deleteSystem, err)
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
	//
	// The name is checked as well as the index. Indexes are reused: the system
	// that moves into a deleted one's place would otherwise be read as the one
	// that was meant to go, and a delete that worked would be reported as
	// having done nothing.
	all, err := navigate.EverySystem(ctx, client)
	if err != nil {
		return err
	}
	for _, sys := range all {
		if sys.Index == index && strings.EqualFold(sys.Name, name) {
			return fmt.Errorf("the system %q is still there afterwards: nothing was deleted", name)
		}
	}

	return render.Changed(app,
		render.Mutation{Action: "deleted", Kind: "system", Name: name},
		"deleted "+name)
}

// runNew creates a system and names it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - list: the favorites list's index, or its name
//   - name: what to call the new system
//   - kind: the system type, as the scanner spells it on its own menu
//
// Returns:
//   - error if the scanner cannot be reached, the list cannot be resolved or
//     reached, the system cannot be created, typed or named, or it does not
//     appear under that name afterwards
func runNew(ctx context.Context, app *appcontext.App, list, name, kind string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if _, err := navigate.ToSystems(ctx, client, list); err != nil {
		return err
	}

	// Nothing exists yet: this opens the type picker, and the system is created
	// by what is chosen there.
	if err := menus.Select(ctx, client, newSystem); err != nil {
		return fmt.Errorf("creating the system: %w", err)
	}

	if err := menus.Select(ctx, client, kind); err != nil {
		return fmt.Errorf("choosing the system type %q: %w\n"+
			"Nothing has been created. Run \"radiocli scan\"", kind, err)
	}

	// The scanner asks before committing to a type, since a system's type
	// cannot be changed afterwards. Nothing exists until this is answered.
	if err := menus.Confirm(ctx, client); err != nil {
		return fmt.Errorf("confirming the system type: %w\n"+
			"Nothing has been created. Run \"radiocli scan\"", err)
	}

	if err := menus.Select(ctx, client, editName); err != nil {
		return fmt.Errorf("the system was created, but its name screen could not be opened: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		return fmt.Errorf("the system was created, but naming it failed: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	all, err := navigate.EverySystem(ctx, client)
	if err != nil {
		return err
	}
	for _, s := range all {
		if s.Name == name {
			return render.Changed(app,
				render.Mutation{Action: "created", Kind: "system", Name: name, In: list},
				fmt.Sprintf("%s (%s)", name, s.Kind))
		}
	}
	return fmt.Errorf("the system does not appear under %q afterwards: "+
		"run \"radiocli systems %q\" to see what was created", name, list)
}

// runRename opens a system's menu and types a new name into it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - want: the system's index, or its name
//   - name: what to call it instead
//
// Returns:
//   - error if the scanner cannot be reached, the system cannot be resolved or
//     named, its menu or name screen cannot be opened, the name cannot be
//     typed, or the menus cannot be left
func runRename(ctx context.Context, app *appcontext.App, want, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := navigate.ResolveSystem(ctx, client, want)
	if err != nil {
		return err
	}

	current, err := systemName(ctx, client, index)
	if err != nil {
		return err
	}
	if current == name {
		// Nothing to do, and no reason to stop the scanner scanning to do it.
		app.Notef("That system is already called %q.\n", name)
		return nil
	}

	// One exchange, unlike a favorites list: the protocol takes the index.
	if err := client.OpenMenu(ctx, device.MenuSystem, index); err != nil {
		return fmt.Errorf("opening the system's menu: %w", err)
	}

	if err := menus.StepTo(ctx, client, editName); err != nil {
		return fmt.Errorf("looking for %q on the system's menu: %w", editName, err)
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
		render.Mutation{Action: "renamed", Kind: "system", Name: name, Was: current},
		name); err != nil {
		return err
	}

	_, err = menus.Leave(ctx, client)
	return err
}

// systemName is the scanner's own name for the system at an index, so that
// what gets reported is what the scanner calls it rather than what was typed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the systems from
//   - index: the index of the system to name
//
// Returns:
//   - the name the scanner gives the system at that index
//   - error if the systems cannot be read, or if no system has that index
func systemName(ctx context.Context, client *device.Scanner, index string) (string, error) {
	all, err := navigate.EverySystem(ctx, client)
	if err != nil {
		return "", err
	}
	for _, sys := range all {
		if sys.Index == index {
			return sys.Name, nil
		}
	}
	return "", fmt.Errorf("no system has index %s", index)
}
