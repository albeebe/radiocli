// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package favorites implements the "favorites" command, which lists the
// scanner's favorites lists.
//
// A favorites list is the top of the scanner's memory: it holds systems, which
// hold departments, which hold channels. Use "systems" to go a level down.
package favorites

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

// New returns the favorites command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that lists the favorites lists, carrying the goto,
//     rename, new, scan and delete subcommands
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "favorites",
		Short: "List the scanner's favorites lists",
		Long: "Favorites lists the scanner's favorites lists and says which of them are being\n" +
			"scanned.\n\n" +
			"A favorites list holds systems, which hold departments, which hold channels.\n" +
			"Run \"radiocli systems\" with a list's name or index to see what is inside one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newGoto(app), newRename(app), newNew(app), newScan(app), newDelete(app))
	return cmd
}

// newDelete returns the "favorites delete" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that deletes the favorites list named on the command
//     line, carrying the --yes flag that has to be passed for it to act
func newDelete(app *appcontext.App) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <list> --yes",
		Short: "Delete a favorites list and everything in it",
		Long: "Delete removes a favorites list from the scanner, along with every system,\n" +
			"department and channel inside it.\n\n" +
			"There is no undo. Nothing on the scanner keeps a copy, and this tool cannot\n" +
			"put back what it removes, so --yes is required: without it the command says\n" +
			"what would go and changes nothing.\n\n" +
			"The list is read back afterwards to confirm it is gone. A built-in scan\n" +
			"source cannot be deleted, and is refused. This stops the scanner scanning,\n" +
			"and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), app, args[0], yes)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
	return cmd
}

// newGoto returns the "favorites goto" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that opens the menu of the favorites list named on the
//     command line
func newGoto(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "goto <list>",
		Short: "Open a favorites list's menu on the scanner",
		Long: "Goto puts the scanner into the menu for one favorites list, where it can be\n" +
			"renamed, given a quick key or a number tag, or deleted.\n\n" +
			"Unlike \"systems goto\" and \"departments goto\", which jump straight to their\n" +
			"menu in one exchange, this walks: the protocol offers no menu for a single\n" +
			"favorites list, so the scanner is sent to the top menu and the knob is turned\n" +
			"from there. Each step is checked against the display by name, never by\n" +
			"counting, so it cannot land one entry short or one entry past.\n\n" +
			"It takes a second or two at the default pace, and --pace turbo makes it\n" +
			"quicker at the usual risk. This stops the scanner scanning: run\n" +
			"\"radiocli key avoid\" when finished.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGoto(cmd.Context(), app, args[0])
		},
	}
}

// newNew returns the "favorites new" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that creates a favorites list under the name given on the
//     command line
func newNew(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: "Create a favorites list",
		Long: "New creates a favorites list and gives it a name.\n\n" +
			"The scanner has no command for this, so it is done through its menus. Pressing\n" +
			"New Favorites List creates one immediately, called something like\n" +
			"\"FAVORITES 0\", and this then types the name you asked for over it.\n\n" +
			"A new list holds nothing and is not scanned until something is put in it and\n" +
			"it is switched on. The list is read back afterwards to confirm it is there.\n" +
			"This stops the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd.Context(), app, args[0])
		},
	}
}

// newRename returns the "favorites rename" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that renames the favorites list named on the command line
func newRename(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <list> <name>",
		Short: "Change a favorites list's name",
		Long: "Rename changes the name of one favorites list.\n\n" +
			"The scanner has no command for this, so it is done the way a person would:\n" +
			"the menus are walked to the list's Rename screen, and the name is typed there\n" +
			"by turning the knob to each character in turn. Every character is checked\n" +
			"after it is set, and the walk matches entries by name rather than counting,\n" +
			"which matters on a menu where Delete sits next to Rename.\n\n" +
			"It takes a while at the default pace, because every character is a key press.\n" +
			"Use --pace turbo to speed it up. This stops the scanner scanning: run\n" +
			"\"radiocli scan\" afterwards.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(cmd.Context(), app, args[0], args[1])
		},
	}
}

// newScan returns the "favorites scan" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that chooses which favorites lists are scanned, carrying
//     the --all and --none flags that stand in for naming them
func newScan(app *appcontext.App) *cobra.Command {
	var all, none bool

	cmd := &cobra.Command{
		Use:   "scan [name]...",
		Short: "Choose which favorites lists are scanned",
		Long: "Scan chooses which favorites lists the scanner scans.\n\n" +
			"Naming lists scans exactly those and switches every other list off, including\n" +
			"the built-in Full Database. That is usually what is wanted: \"scan only this\"\n" +
			"rather than \"also scan this\".\n\n" +
			"Setting a location switches Full Database scanning back on, so this is often\n" +
			"the command to run after \"radiocli location set\".\n\n" +
			"This stops the scanner scanning, and returns it when it is done.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.Context(), app, args, all, none)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "scan every list, including the built-in ones")
	cmd.Flags().BoolVar(&none, "none", false, "scan nothing")
	return cmd
}

// apply switches the lists on and off on the scanner.
//
// Everything starts from "Set All Lists Off" rather than from whatever was
// already set, because naming lists means "scan exactly these". Turning the
// wanted ones on afterwards is then a known starting point rather than a
// question of what state each row happened to be in.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//   - wanted: the lists to switch on, ignored when all or none is set
//   - all: switch every list on, including the built-in ones
//   - none: switch every list off
//
// Returns:
//   - error if the scan selection menu cannot be reached, if an entry cannot
//     be found or pressed, if a wanted list has no row to switch on, or if the
//     scanner cannot be taken out of the menus
func apply(ctx context.Context, client *device.Scanner, wanted []catalog.FavoritesList, all, none bool) error {
	if err := toScanSelection(ctx, client); err != nil {
		return err
	}

	// Switching the full database on or off sends the scanner away to rebuild,
	// so each of these waits for it to come back before pressing anything else.
	if all {
		if err := stepAndCommit(ctx, client, setAllOn); err != nil {
			return err
		}
		_, err := menus.Leave(ctx, client)
		return err
	}

	if err := stepAndCommit(ctx, client, setAllOff); err != nil {
		return err
	}
	if none {
		_, err := menus.Leave(ctx, client)
		return err
	}

	// "Set All Lists Off" returns to the scan selection menu, so the monitor
	// list is opened from there.
	if err := menus.Select(ctx, client, selectLists); err != nil {
		return fmt.Errorf("looking for %q: %w", selectLists, err)
	}

	for _, list := range wanted {
		if err := switchOn(ctx, client, list.Name); err != nil {
			menus.Leave(ctx, client)
			return err
		}
	}

	_, err := menus.Leave(ctx, client)
	return err
}

// chosen turns the names into the lists they mean.
//
// Parameters:
//   - names: the favorites lists to find, each by name or by index
//   - lists: every favorites list the scanner reports
//
// Returns:
//   - []catalog.FavoritesList holding the lists the names mean, in the order
//     they were given
//   - error if a name matches none of the scanner's lists
func chosen(names []string, lists []catalog.FavoritesList) ([]catalog.FavoritesList, error) {
	wanted := make([]catalog.FavoritesList, 0, len(names))
	for _, name := range names {
		index, err := catalog.Resolve(name, "favorites list", lists)
		if err != nil {
			return nil, err
		}
		for _, l := range lists {
			if l.Index == index {
				wanted = append(wanted, l)
				break
			}
		}
	}
	return wanted, nil
}

// renderLists writes the listing as an aligned table.
//
// A scanner with no favorites lists is a complete answer, not a failure, so
// this returns nil and puts the explanation on stderr.
//
// Parameters:
//   - app: the application context, for output
//   - lists: the favorites lists to write
//
// Returns:
//   - error if writing the table fails
func renderLists(app *appcontext.App, lists []catalog.FavoritesList) error {
	if len(lists) == 0 {
		app.Notef("The scanner reports no favorites lists.\n")
		return nil
	}

	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tNAME\tSCANNED\tQUICK KEY\tNUMBER TAG")

	anyBuiltIn := false
	filled := 0
	for _, l := range lists {
		marker := " "
		if l.BuiltIn {
			marker, anyBuiltIn = "+", true
		}

		if l.Partial {
			filled++
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				marker, l.Name, render.Unread, render.Unread, render.Unread)
			continue
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			marker, l.Name, render.YesNo(l.Monitored), render.Dash(l.QuickKey), render.Dash(l.NumberTag))
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the favorites list: %w", err)
	}

	render.Filled(app, "favorites lists", filled)

	if anyBuiltIn {
		app.Notef("\n+ built into the scanner, not a list you created\n")
	}
	return nil
}

// rowIs reports whether a row of the monitor list is the named list.
//
// The state is stripped first, because the scanner writes it on the same line:
// "GREENDALE, ST 00000 :On".
//
// What is left is compared as a whole name, and only as the start of one when
// the scanner cut it short to fit the screen, which it marks. "Quick Save
// Favorites List" shows as "Quick Save Favorit" and has to match; "RADIOCLI
// TEST" shows in full and must not match "RADIOCLI TEST SCAN", which is a
// different list. Matching a prefix either way did exactly that: asked to scan
// one list, the tool walked onto the shorter name first, matched it, and
// switched on the wrong one, reporting success.
//
// Two names that survive truncation identically are genuinely indistinguishable
// on this screen, and the first of them wins. Nothing here can do better,
// because the scanner does not say which list a row belongs to.
//
// Parameters:
//   - row: the highlighted row of the monitor list, as read from the display
//   - name: the list being looked for
//
// Returns:
//   - bool reporting whether the row is the list wanted
func rowIs(row menus.Row, name string) bool {
	text := strings.TrimSpace(row.Text)
	text = strings.TrimSuffix(text, stateOn)
	text = strings.TrimSuffix(text, stateOff)
	text = strings.TrimSpace(text)

	if strings.EqualFold(text, name) {
		return true
	}
	if !row.Cut || text == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(text))
}

// run reads the favorites lists and renders them.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//
// Returns:
//   - error if no scanner is named, if the lists cannot be read, or if writing
//     the report fails
func run(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	lists, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, lists)
	}

	return renderLists(app, lists)
}

// runDelete removes a favorites list and confirms it is gone.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - want: the favorites list to delete, by name or by index
//   - yes: whether the caller has agreed to the deletion
//
// Returns:
//   - error if no scanner is named, if the lists cannot be read, if the name
//     matches none of them, if the one it matches is built into the scanner,
//     if yes was not passed, if the walk or the prompt fails, or if the list is
//     still there afterwards
func runDelete(ctx context.Context, app *appcontext.App, want string, yes bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Resolve before asking about --yes, so the refusal names the list the way
	// the scanner does rather than the way it was typed, and a name that does
	// not exist is reported as that rather than as a missing flag.
	lists, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}
	index, err := catalog.Resolve(want, "favorites list", lists)
	if err != nil {
		return err
	}

	var target catalog.FavoritesList
	for _, l := range lists {
		if l.Index == index {
			target = l
		}
	}

	// The built-in scan sources are part of the scanner rather than lists
	// anyone made, and it has no way to remove them.
	if target.BuiltIn {
		return fmt.Errorf("%q is built into the scanner and cannot be deleted: "+
			"only a list you created can be", target.Name)
	}

	if !yes {
		return fmt.Errorf("deleting the favorites list %q removes it and everything in it, "+
			"and cannot be undone: pass --yes", target.Name)
	}

	if _, err := navigate.ToFavoritesList(ctx, client, want); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, deleteEntry); err != nil {
		return fmt.Errorf("looking for %q on the list's menu: %w", deleteEntry, err)
	}
	if err := menus.ConfirmDelete(ctx, client); err != nil {
		return err
	}

	// Removing a list sends the scanner away to rebuild what it is scanning,
	// during which it stops answering.
	if err := menus.Awaken(ctx, client); err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	after, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}
	for _, l := range after {
		if l.Index == index && strings.EqualFold(l.Name, target.Name) {
			return fmt.Errorf("the favorites list %q is still there afterwards: nothing was deleted",
				target.Name)
		}
	}

	return render.Changed(app,
		render.Mutation{Action: "deleted", Kind: "favorites list", Name: target.Name},
		"deleted "+target.Name)
}

// runGoto walks the menus to one favorites list and reports where it landed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - want: the favorites list to open, by name or by index
//
// Returns:
//   - error if no scanner is named, if the list cannot be reached, or if
//     reporting the menu fails
func runGoto(ctx context.Context, app *appcontext.App, want string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if _, err := navigate.ToFavoritesList(ctx, client, want); err != nil {
		return err
	}
	return menus.Show(ctx, app, client)
}

// runNew creates a favorites list and names it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - name: what to call the new favorites list
//
// Returns:
//   - error if no scanner is named, if the list cannot be created or named, if
//     the scanner cannot be taken out of the menus, if the lists cannot be read
//     back, or if the new list does not appear among them
func runNew(ctx context.Context, app *appcontext.App, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if err := navigate.ToFavorites(ctx, client); err != nil {
		return err
	}

	// The scanner creates the list the moment this is pressed, with a name of
	// its own choosing, and opens its menu.
	if err := menus.Select(ctx, client, newFavoritesList); err != nil {
		return fmt.Errorf("creating the favorites list: %w", err)
	}

	if err := menus.Select(ctx, client, renameEntry); err != nil {
		return fmt.Errorf("the list was created, but its name screen could not be opened: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		return fmt.Errorf("the list was created, but naming it failed: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	lists, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}
	for _, l := range lists {
		if l.Name == name {
			return render.Changed(app,
				render.Mutation{Action: "created", Kind: "favorites list", Name: name}, name)
		}
	}
	return fmt.Errorf("the list does not appear under %q afterwards: "+
		"run \"radiocli favorites\" to see what was created", name)
}

// runRename walks to a favorites list and types a new name into it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - want: the favorites list to rename, by name or by index
//   - name: what to call it instead
//
// Returns:
//   - error if no scanner is named, if the list cannot be reached, if the
//     Rename entry cannot be found or opened, if the name cannot be typed, or
//     if the scanner cannot be taken out of the menus
func runRename(ctx context.Context, app *appcontext.App, want, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	target, err := navigate.ToFavoritesList(ctx, client, want)
	if err != nil {
		return err
	}

	if target.Name == name {
		// Nothing to do, and the scanner is sitting in a menu it should not be
		// left in for no reason.
		app.Notef("That list is already called %q.\n", name)
		_, err = menus.Leave(ctx, client)
		return err
	}

	if err := menus.StepTo(ctx, client, renameEntry); err != nil {
		return fmt.Errorf("looking for %q on the %s menu: %w", renameEntry, target.Name, err)
	}
	if err := menus.Enter(ctx, client); err != nil {
		return err
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		// The scanner is left on the entry screen, where leaving discards the
		// half-typed name, so say that rather than leaving the reader guessing
		// whether the list has been mangled.
		return fmt.Errorf("typing the new name: %w\n"+
			"The scanner is still on the entry screen and nothing has been saved. "+
			"Run \"radiocli scan\" to leave it as it was", err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	// Reporting success from inside the menus says the name was typed, which is
	// not the same as the scanner having kept it.
	after, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}
	for _, l := range after {
		if l.Index == target.Index && l.Name == name {
			return render.Changed(app,
				render.Mutation{Action: "renamed", Kind: "favorites list", Name: name, Was: target.Name},
				name)
		}
	}
	return fmt.Errorf("the list does not appear under %q afterwards: "+
		"run \"radiocli favorites\" to see what it is called", name)
}

// runScan sets which lists are scanned and reports what the scanner ended up
// with.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - names: the favorites lists to scan, each by name or by index
//   - all: scan every list, including the built-in ones
//   - none: scan nothing
//
// Returns:
//   - error if the flags and the names ask for different things, if no scanner
//     is named, if the lists cannot be read, if a name matches none of them, if
//     the scanner will not take the change, or if writing the report fails
func runScan(ctx context.Context, app *appcontext.App, names []string, all, none bool) error {
	switch {
	case all && none:
		return fmt.Errorf("--all and --none ask for opposite things: choose one")
	case (all || none) && len(names) > 0:
		return fmt.Errorf("naming lists and passing --all or --none ask for different things: choose one")
	case !all && !none && len(names) == 0:
		return fmt.Errorf("name the lists to scan, or pass --all or --none\n" +
			"Run \"radiocli favorites\" to see what there is")
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Resolve the names before touching the scanner, so a name it does not
	// have costs nothing and leaves it scanning.
	lists, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}
	wanted, err := chosen(names, lists)
	if err != nil {
		return err
	}

	if err := apply(ctx, client, wanted, all, none); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what is being
	// scanned.
	after, err := navigate.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, after)
	}
	return renderLists(app, after)
}

// stepAndCommit moves to an entry and presses it, allowing for the scanner
// disappearing to rebuild as a result.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//   - entry: the name of the entry to move to and press
//
// Returns:
//   - error if the entry cannot be reached, or if the scanner does not come
//     back from the rebuild
func stepAndCommit(ctx context.Context, client *device.Scanner, entry string) error {
	if err := menus.StepTo(ctx, client, entry); err != nil {
		return fmt.Errorf("looking for %q: %w", entry, err)
	}
	return menus.Commit(ctx, client)
}

// switchOn walks the monitor list to one row and switches it on.
//
// The rows carry their own state, as "GREENDALE, ST 00000 :On", so this cannot
// use the shared menu walk: it has to compare the name without the state, and
// it has to read the state to know whether pressing would switch the row on or
// back off again.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//   - name: the list whose row is to be switched on
//
// Returns:
//   - error if the display cannot be read, if the knob cannot be turned, if
//     the row does not end up switched on, or if the monitor list holds no row
//     by that name
func switchOn(ctx context.Context, client *device.Scanner, name string) error {
	for step := 0; step < monitorSteps; step++ {
		row, err := menus.HighlightedRow(ctx, client)
		if err != nil {
			return fmt.Errorf("reading the list of lists: %w", err)
		}

		if rowIs(row, name) {
			if strings.HasSuffix(strings.TrimSpace(row.Text), stateOn) {
				return nil
			}
			if err := menus.Commit(ctx, client); err != nil {
				return err
			}

			// Pressing toggles, so confirm it went the way that was wanted
			// rather than assuming.
			row, err = menus.HighlightedRow(ctx, client)
			if err != nil {
				return fmt.Errorf("reading %q back: %w", name, err)
			}
			if !strings.HasSuffix(strings.TrimSpace(row.Text), stateOn) {
				return fmt.Errorf("%q did not switch on: the scanner shows %q", name, strings.TrimSpace(row.Text))
			}
			return nil
		}

		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return fmt.Errorf("turning the knob: %w", err)
		}
	}
	return fmt.Errorf("no row for %q in the scanner's list of lists", name)
}

// toScanSelection puts the scanner on the scan selection menu, starting from
// the top menu so the walk is the same whatever was on screen before.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - error if the top menu cannot be opened, or if it carries no entry for
//     the scan selection
func toScanSelection(ctx context.Context, client *device.Scanner) error {
	if err := client.OpenMenu(ctx, device.MenuTop, ""); err != nil {
		return fmt.Errorf("opening the top menu: %w", err)
	}
	if err := menus.Select(ctx, client, setScanSelection); err != nil {
		return fmt.Errorf("looking for %q in the top menu: %w", setScanSelection, err)
	}
	return nil
}
