// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package sites implements the "sites" command, which lists the sites of one
// trunked system.
//
// A site is the transmitter a trunked system speaks through, and it holds the
// pool of frequencies the system uses there. Sites sit beside departments
// rather than inside them: the site says where the signal comes from, the
// department says who is talking on it. A conventional system has no sites at
// all, and holds its frequencies in departments.
package sites

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

// New returns the sites command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "sites" command, with its subcommands attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sites <system>",
		Short: "List the sites of a trunked system",
		Long: "Sites lists the sites of one trunked system.\n\n" +
			"A site is the transmitter the system speaks through, and it holds the pool of\n" +
			"frequencies used there. Sites sit beside departments rather than inside them:\n" +
			"the site says where the signal comes from, the department says who is talking\n" +
			"on it.\n\n" +
			"Only a trunked system has sites. A conventional system holds its frequencies\n" +
			"in departments, and reports none here.\n\n" +
			"Run \"radiocli sites frequencies\" with a site's name to see its frequencies.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app, args[0])
		},
	}

	cmd.AddCommand(newNew(app), newRename(app), newDelete(app), newFrequencies(app))
	return cmd
}

// newDelete returns the "sites delete" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "sites delete" subcommand, carrying its own --yes flag
func newDelete(app *appcontext.App) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <site> --yes",
		Short: "Delete a site and every frequency in it",
		Long: "Delete removes a site from its system, along with every frequency in it.\n\n" +
			"A trunked system with no sites receives nothing at all, since the site is what\n" +
			"carries the frequencies. Deleting the only site of a system leaves its\n" +
			"departments and talkgroups in place but silent.\n\n" +
			"There is no undo. Nothing on the scanner keeps a copy, and this tool cannot put\n" +
			"back what it removes, so --yes is required: without it the command says what\n" +
			"would go and changes nothing.\n\n" +
			"The system's sites are read back afterwards to confirm it is gone. This stops\n" +
			"the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), app, args[0], yes)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
	return cmd
}

// newNew returns the "sites new" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "sites new" subcommand
func newNew(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "new <system> <name>",
		Short: "Create a site in a trunked system",
		Long: "New creates a site inside one trunked system and gives it a name.\n\n" +
			"The scanner has no command for this, so it is done through its menus. Pressing\n" +
			"New Site creates one immediately, called something like \"SITE 0\", and this then\n" +
			"types the name you asked for over it.\n\n" +
			"A new site holds no frequencies and will not track anything until some are\n" +
			"added with \"radiocli sites frequencies add\". Only a trunked system can hold a\n" +
			"site, and a conventional one is refused.\n\n" +
			"The site is read back afterwards to confirm it is there. This stops the scanner\n" +
			"scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd.Context(), app, args[0], args[1])
		},
	}
}

// newRename returns the "sites rename" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "sites rename" subcommand
func newRename(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <site> <name>",
		Short: "Change a site's name",
		Long: "Rename changes the name of one site, leaving its frequencies alone.\n\n" +
			"The scanner has no command for this, so it is done the way a person would: the\n" +
			"site's menu is opened, its Edit Name screen selected, and the name typed there\n" +
			"by turning the knob to each character in turn.\n\n" +
			"Nothing is saved until the whole name is typed, so an interrupted rename leaves\n" +
			"the old name untouched. This stops the scanner scanning, and returns it to\n" +
			"scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(cmd.Context(), app, args[0], args[1])
		},
	}
}

// confirm checks that a system really exists, for the case where its empty
// answer could equally mean it is not there.
//
// A system holding no sites and an index naming no system are the same reply,
// and telling somebody a system has no sites when there is no such system sends
// them looking for a site rather than for the system. Only an index needs
// asking about: a name was matched against the systems the scanner reports to
// get here, so it is known to exist.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//   - want: the system as it was asked for, by name or by index
//   - index: the index that was resolved from it
//
// Returns:
//   - error if the systems cannot be read, or if no system has that index, and
//     nil when the system exists and simply holds no sites
func confirm(ctx context.Context, client *device.Scanner, want, index string) error {
	if !catalog.IsIndex(want) {
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
	return fmt.Errorf("no system has index %s: run \"radiocli systems\" to see what there is", index)
}

// renderSites writes the listing as an aligned table.
//
// A system with no sites is a complete answer rather than a failure, and is
// what a conventional system always reports, so this says so and returns nil.
//
// Parameters:
//   - app: the application context holding the streams to write to
//   - found: the sites to renderSites, in the order the scanner reports them
//
// Returns:
//   - error if the table cannot be written to the output stream
func renderSites(app *appcontext.App, found []catalog.Site) error {
	if len(found) == 0 {
		app.Notef("That system has no sites. Only a trunked system does.\n")
		return nil
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSCANNED\tQUICK KEY")
	for _, s := range found {
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, render.YesNo(!s.Avoided), render.Dash(s.QuickKey))
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the site list: %w", err)
	}
	return nil
}

// run reads a system's sites and renders them.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner, the output format and
//     the streams to write to
//   - want: the system's index, or its name
//
// Returns:
//   - error if the scanner cannot be reached, the system cannot be resolved or
//     does not exist, its sites cannot be read, or the listing cannot be
//     written
func run(ctx context.Context, app *appcontext.App, want string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveSystem(ctx, client, want)
	if err != nil {
		return err
	}

	found, err := catalog.ReadSites(ctx, client, index)
	if err != nil {
		return err
	}

	// An index naming no system answers exactly as a conventional one does, so
	// the difference has to be settled by looking the index up. This is only
	// worth an extra exchange when there is nothing to report anyway.
	if len(found) == 0 {
		if err := confirm(ctx, client, want, index); err != nil {
			return err
		}
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, found)
	}
	return renderSites(app, found)
}

// runDelete removes a site and confirms it is gone.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - want: the site's index, or its name
//   - yes: whether --yes was passed, without which nothing is deleted
//
// Returns:
//   - error if the scanner cannot be reached, the site cannot be resolved or
//     named, --yes was not passed, its menu cannot be opened or its delete
//     entry pressed, or the site is still there afterwards
func runDelete(ctx context.Context, app *appcontext.App, want string, yes bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Resolve before asking about --yes, so the refusal names the site the way
	// the scanner does, and a name it does not have is reported as that rather
	// than as a missing flag.
	index, err := catalog.ResolveSite(ctx, client, want)
	if err != nil {
		return err
	}
	name, err := siteName(ctx, client, index)
	if err != nil {
		return err
	}

	if !yes {
		return fmt.Errorf("deleting the site %q removes it and every frequency in it, "+
			"and cannot be undone: pass --yes", name)
	}

	if err := client.OpenMenu(ctx, device.MenuSite, index); err != nil {
		return fmt.Errorf("opening the site's menu: %w", err)
	}
	if err := menus.Select(ctx, client, deleteSite); err != nil {
		return fmt.Errorf("looking for %q on the site's menu: %w", deleteSite, err)
	}
	if err := menus.ConfirmDelete(ctx, client); err != nil {
		return err
	}

	// Deleting a site takes its frequencies with it, which sends the scanner
	// away to work out what it can still hear. It answers nothing at all while
	// it does, so anything sent before it comes back times out.
	if err := menus.Awaken(ctx, client); err != nil {
		return err
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	after, err := catalog.EverySite(ctx, client)
	if err != nil {
		return err
	}
	for _, s := range after {
		if s.Index == index && strings.EqualFold(s.Name, name) {
			return fmt.Errorf("the site %q is still there afterwards: nothing was deleted", name)
		}
	}

	return render.Changed(app,
		render.Mutation{Action: "deleted", Kind: "site", Name: name},
		"deleted "+name)
}

// runNew creates a site and names it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - system: the system's index, or its name
//   - name: what to call the new site
//
// Returns:
//   - error if the scanner cannot be reached, the system cannot be resolved or
//     reached, the site cannot be created or named, or it does not appear
//     under that name afterwards
func runNew(ctx context.Context, app *appcontext.App, system, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveSystem(ctx, client, system)
	if err != nil {
		return err
	}

	if err := navigate.ToSites(ctx, client, index); err != nil {
		return err
	}

	// The scanner creates the site the moment this is pressed, with a name of
	// its own choosing, and opens its menu.
	if err := menus.Select(ctx, client, newSite); err != nil {
		return fmt.Errorf("creating the site: %w", err)
	}

	if err := menus.Select(ctx, client, editName); err != nil {
		return fmt.Errorf("the site was created, but its name screen could not be opened: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		return fmt.Errorf("the site was created, but naming it failed: %w\n"+
			"It is on the scanner under whatever name it was given. Run \"radiocli scan\"", err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what was made.
	found, err := catalog.ReadSites(ctx, client, index)
	if err != nil {
		return err
	}
	for _, s := range found {
		if s.Name == name {
			return render.Changed(app,
				render.Mutation{Action: "created", Kind: "site", Name: name, In: system}, name)
		}
	}
	return fmt.Errorf("the site does not appear under %q afterwards: "+
		"run \"radiocli sites %s\" to see what was created", name, system)
}

// runRename opens a site's menu and types a new name into it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - want: the site's index, or its name
//   - name: what to call it instead
//
// Returns:
//   - error if the scanner cannot be reached, the site cannot be resolved or
//     named, its menu or name screen cannot be opened, the name cannot be
//     typed, or the menus cannot be left
func runRename(ctx context.Context, app *appcontext.App, want, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveSite(ctx, client, want)
	if err != nil {
		return err
	}

	current, err := siteName(ctx, client, index)
	if err != nil {
		return err
	}
	if current == name {
		// Nothing to do, and no reason to stop the scanner scanning to do it.
		app.Notef("That site is already called %q.\n", name)
		return nil
	}

	if err := client.OpenMenu(ctx, device.MenuSite, index); err != nil {
		return fmt.Errorf("opening the site's menu: %w", err)
	}

	if err := menus.Select(ctx, client, editName); err != nil {
		return fmt.Errorf("looking for %q on the site's menu: %w\n"+
			"Nothing has been changed. Run \"radiocli scan\"", editName, err)
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		// Leaving the entry screen discards, so nothing has been saved.
		return fmt.Errorf("typing the new name: %w\n"+
			"The scanner is still on the entry screen and nothing has been saved. "+
			"Run \"radiocli scan\" to leave it as it was", err)
	}

	if err := render.Changed(app,
		render.Mutation{Action: "renamed", Kind: "site", Name: name, Was: current},
		name); err != nil {
		return err
	}

	_, err = menus.Leave(ctx, client)
	return err
}

// siteName is the scanner's own name for the site at an index, so that what
// gets reported is what the scanner calls it rather than what was typed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the sites from
//   - index: the index of the site to name
//
// Returns:
//   - the name the scanner gives the site at that index
//   - error if the sites cannot be read, or if no site has that index
func siteName(ctx context.Context, client *device.Scanner, index string) (string, error) {
	all, err := catalog.EverySite(ctx, client)
	if err != nil {
		return "", err
	}
	for _, s := range all {
		if s.Index == index {
			return s.Name, nil
		}
	}
	return "", fmt.Errorf("no site has index %s", index)
}
