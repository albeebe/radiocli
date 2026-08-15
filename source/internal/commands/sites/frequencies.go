// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package sites

import (
	"context"
	"errors"
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

// newFrequencies returns the "sites frequencies" subcommand and its own
// subcommands.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "sites frequencies" subcommand, with its own subcommands attached
func newFrequencies(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "frequencies <site>",
		Short: "List a site's frequencies",
		Long: "Frequencies lists the pool of frequencies one site uses.\n\n" +
			"A trunked system does not give each department a frequency of its own. It\n" +
			"shares a pool across everyone on the system, and a computer hands out whichever\n" +
			"is free for the length of each transmission. These are that pool.\n\n" +
			"One of them is the control channel at any moment, carrying the data that says\n" +
			"where each conversation has been sent. The scanner works out which by itself,\n" +
			"so they are entered as a plain list with no roles attached.\n\n" +
			"This asks the scanner directly and takes one exchange. It does not stop the\n" +
			"scanner scanning.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFrequencies(cmd.Context(), app, args[0])
		},
	}

	cmd.AddCommand(newFrequenciesAdd(app), newFrequenciesDelete(app))
	return cmd
}

// newFrequenciesAdd returns the "sites frequencies add" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "sites frequencies add" subcommand
func newFrequenciesAdd(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "add <site> <frequency>...",
		Short: "Add frequencies to a site",
		Long: "Add puts one or more frequencies into a site's pool.\n\n" +
			"Several can be given at once, which is the usual case: a trunked site runs a\n" +
			"pool of them and a system with only some of its frequencies entered will lose\n" +
			"calls handed to the ones it does not know.\n\n" +
			"The frequencies are written in megahertz, which is the unit the scanner's own\n" +
			"screen asks for. Order does not matter, and no roles are attached: the scanner\n" +
			"finds the control channel among them by itself.\n\n" +
			"A frequency the site already holds is skipped rather than added twice. They are\n" +
			"read back afterwards to confirm they are there. This stops the scanner\n" +
			"scanning, and returns it to scanning when it is done.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFrequenciesAdd(cmd.Context(), app, args[0], args[1:])
		},
	}
}

// newFrequenciesDelete returns the "sites frequencies delete" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "sites frequencies delete" subcommand, carrying its own --yes flag
func newFrequenciesDelete(app *appcontext.App) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <site> <frequency> --yes",
		Short: "Remove a frequency from a site",
		Long: "Delete removes one frequency from a site's pool.\n\n" +
			"There is no undo, so --yes is required: without it the command says what would\n" +
			"go and changes nothing.\n\n" +
			"Removing a frequency a system really uses does not stop the system working, but\n" +
			"the scanner will miss every call handed to that frequency, which sounds like a\n" +
			"busy system going quiet at random.\n\n" +
			"The site's frequencies are read back afterwards to confirm it is gone. This\n" +
			"stops the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFrequenciesDelete(cmd.Context(), app, args[0], args[1], yes)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
	return cmd
}

// addOne types one frequency into the frequency list the scanner is showing.
//
// The scanner returns to the list after each one, so this is called in a loop
// without walking back down for every frequency.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner, already showing the site's frequency list
//   - frequency: the frequency to type, in megahertz
//
// Returns:
//   - error if the entry screen cannot be opened, or the frequency cannot be
//     typed into it
func addOne(ctx context.Context, client *device.Scanner, frequency string) error {
	if err := menus.Select(ctx, client, newFrequency); err != nil {
		return fmt.Errorf("opening the frequency entry screen: %w", err)
	}
	if err := textinput.Set(ctx, client, frequency); err != nil {
		return fmt.Errorf("entering it: %w", err)
	}
	return nil
}

// held reports whether a site already holds a frequency.
//
// The scanner writes a frequency with padding and trailing zeroes, as
// " 851.050000MHz", so this compares the numbers rather than the text.
//
// Parameters:
//   - found: the frequencies the site holds, as the scanner writes them
//   - frequency: the frequency to look for
//
// Returns:
//   - true when the site holds that frequency, and false when it does not or
//     when frequency is not a number of megahertz
func held(found []catalog.SiteFrequency, frequency string) bool {
	want, err := megahertz(frequency)
	if err != nil {
		return false
	}

	for _, f := range found {
		got, err := megahertz(f.Frequency)
		if err != nil {
			continue
		}
		if got == want {
			return true
		}
	}
	return false
}

// listed names the frequencies a site holds, for a failure message.
//
// Parameters:
//   - found: the frequencies the site holds
//
// Returns:
//   - the frequencies separated by commas, or "none at all" when the site
//     holds none
func listed(found []catalog.SiteFrequency) string {
	if len(found) == 0 {
		return "none at all"
	}

	names := make([]string, 0, len(found))
	for _, f := range found {
		names = append(names, strings.TrimSpace(f.Frequency))
	}
	return strings.Join(names, ", ")
}

// megahertz turns a frequency as either side writes it into a number of hertz,
// so that "851.050", " 851.050000MHz" and "851.05" all compare equal.
//
// Hertz rather than a float, because comparing floats for equality is a way to
// have two identical frequencies disagree.
//
// The reading is device.ParseFrequency, which the tune command shares. This
// used to do its own, trimming the exact string "MHz", so a frequency written
// "851.05mhz" tuned the radio and was refused here. One idea, one parser; what
// is left here is the wording and the hertz the callers compare in.
//
// Parameters:
//   - value: a frequency in megahertz, written either way
//
// Returns:
//   - the frequency in hertz, rounded to the nearest hertz
//   - error if value is not a number of megahertz
func megahertz(value string) (int64, error) {
	f, err := device.ParseFrequency(value)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number of megahertz", value)
	}
	return int64(f), nil
}

// outputFrequencies writes a listing in whichever format was asked for.
//
// Every path that ends in a frequency listing goes through here rather than
// branching for itself, because the one that did not is what broke the promise
// stdout makes under -o json: adding frequencies a site already held printed a
// text table into a script's parser, and it did so exactly when the command had
// changed nothing, which is the case a script is least likely to have tried.
//
// Parameters:
//   - app: the application context holding the output format and the streams to
//     write to
//   - found: the frequencies to write, in the order the scanner reports them
//
// Returns:
//   - error if the listing cannot be written to the output stream
func outputFrequencies(app *appcontext.App, found []catalog.SiteFrequency) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, found)
	}
	return renderFrequencies(app, found)
}

// renderFrequencies writes the listing as an aligned table.
//
// Parameters:
//   - app: the application context holding the streams to write to
//   - found: the frequencies to renderSites, in the order the scanner reports them
//
// Returns:
//   - error if the table cannot be written to the output stream
func renderFrequencies(app *appcontext.App, found []catalog.SiteFrequency) error {
	if len(found) == 0 {
		app.Notef("That site holds no frequencies, so the system will not track anything there.\n")
		return nil
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FREQUENCY")
	for _, f := range found {
		fmt.Fprintf(w, "%s\n", f.Frequency)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the frequency list: %w", err)
	}
	return nil
}

// rowFor finds the row of the frequency list that carries a frequency, so that
// it can be pressed by the name the scanner writes rather than the one typed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner, already showing the site's frequency list
//   - frequency: the frequency to find a row for
//
// Returns:
//   - the row as the scanner writes it
//   - error if the list cannot be read, if frequency is not a number of
//     megahertz, or if no row carries it
func rowFor(ctx context.Context, client *device.Scanner, frequency string) (string, error) {
	entries, err := menus.Entries(ctx, client)
	if err != nil {
		return "", fmt.Errorf("reading the site's frequency list: %w", err)
	}

	want, err := megahertz(frequency)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if strings.EqualFold(entry, newFrequency) {
			continue
		}
		if got, err := megahertz(entry); err == nil && got == want {
			return entry, nil
		}
	}
	return "", fmt.Errorf("no row for %s in the site's frequency list", frequency)
}

// runFrequencies reads a site's frequencies and renders them.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner, the output format and
//     the streams to write to
//   - want: the site's index, or its name
//
// Returns:
//   - error if the scanner cannot be reached, the site cannot be resolved, its
//     frequencies cannot be read, or the listing cannot be written
func runFrequencies(ctx context.Context, app *appcontext.App, want string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveSite(ctx, client, want)
	if err != nil {
		return err
	}

	found, err := catalog.ReadSiteFrequencies(ctx, client, index)
	if err != nil {
		return err
	}

	return outputFrequencies(app, found)
}

// runFrequenciesAdd types frequencies into a site's frequency list.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner, the output format and
//     the streams to write to
//   - want: the site's index, or its name
//   - wanted: the frequencies to add, in megahertz
//
// Returns:
//   - error if one of the frequencies is not one the entry screen would take,
//     the scanner cannot be reached, the site cannot be resolved or reached, a
//     frequency cannot be typed, or one does not appear afterwards
func runFrequenciesAdd(ctx context.Context, app *appcontext.App, want string, wanted []string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Check what was typed before touching the scanner, so a mistyped frequency
	// costs nothing and leaves it scanning. What comes back is what will be
	// typed, which is what was written unless it carried a unit the entry
	// screen has no keys for.
	wanted = append([]string(nil), wanted...)
	for i, f := range wanted {
		typed, err := validFrequency(f)
		if err != nil {
			return err
		}
		wanted[i] = typed
	}

	index, err := catalog.ResolveSite(ctx, client, want)
	if err != nil {
		return err
	}

	before, err := catalog.ReadSiteFrequencies(ctx, client, index)
	if err != nil {
		return err
	}

	// Adding one the site already holds would make a duplicate the scanner is
	// happy to keep, so skip those rather than creating them.
	todo := make([]string, 0, len(wanted))
	for _, f := range wanted {
		if held(before, f) {
			app.Notef("The site already has %s.\n", f)
			continue
		}
		todo = append(todo, f)
	}
	// Nothing to do, but still a listing: a caller that asked for one gets what
	// the site holds, in the format it asked for, rather than nothing at all.
	if len(todo) == 0 {
		return outputFrequencies(app, before)
	}

	if err := navigate.ToSiteFrequencies(ctx, client, index); err != nil {
		return err
	}

	for _, f := range todo {
		if err := addOne(ctx, client, f); err != nil {
			// Say which one, since the ones before it are already in.
			menus.Leave(ctx, client)
			return fmt.Errorf("adding %s: %w\n"+
				"Any frequency added before it is on the scanner. "+
				"Run \"radiocli sites frequencies %s\" to see what is there", f, err, want)
		}
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	after, err := catalog.ReadSiteFrequencies(ctx, client, index)
	if err != nil {
		return err
	}
	for _, f := range todo {
		if !held(after, f) {
			return fmt.Errorf("%s does not appear in the site afterwards: "+
				"run \"radiocli sites frequencies %s\" to see what is there", f, want)
		}
	}

	return outputFrequencies(app, after)
}

// runFrequenciesDelete removes one frequency from a site.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - want: the site's index, or its name
//   - frequency: the frequency to remove, in megahertz
//   - yes: whether --yes was passed, without which nothing is deleted
//
// Returns:
//   - error if the frequency is not one the entry screen would take, the site
//     does not hold it, --yes was not passed, the scanner cannot be reached or
//     its row found and deleted, or the frequency is still there afterwards
func runFrequenciesDelete(ctx context.Context, app *appcontext.App, want, frequency string, yes bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	frequency, err = validFrequency(frequency)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveSite(ctx, client, want)
	if err != nil {
		return err
	}

	// Look before asking about --yes, so a frequency the site does not hold is
	// reported as that rather than as a missing flag.
	before, err := catalog.ReadSiteFrequencies(ctx, client, index)
	if err != nil {
		return err
	}
	if !held(before, frequency) {
		return fmt.Errorf("the site does not hold %s: it holds %s",
			frequency, listed(before))
	}

	if !yes {
		return fmt.Errorf("removing %s from the site cannot be undone: pass --yes", frequency)
	}

	if err := navigate.ToSiteFrequencies(ctx, client, index); err != nil {
		return err
	}

	// The rows are the frequencies themselves, written the scanner's way, so
	// the one to press has to be found by comparing numbers rather than text.
	row, err := rowFor(ctx, client, frequency)
	if err != nil {
		menus.Leave(ctx, client)
		return err
	}
	if err := menus.Select(ctx, client, row); err != nil {
		menus.Leave(ctx, client)
		return fmt.Errorf("looking for %s in the site: %w", frequency, err)
	}
	if err := menus.Select(ctx, client, deleteFreqEnt); err != nil {
		menus.Leave(ctx, client)
		return fmt.Errorf("looking for %q on the frequency's menu: %w", deleteFreqEnt, err)
	}
	if err := menus.ConfirmDelete(ctx, client); err != nil {
		menus.Leave(ctx, client)
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	after, err := catalog.ReadSiteFrequencies(ctx, client, index)
	if err != nil {
		return err
	}
	if held(after, frequency) {
		return fmt.Errorf("%s is still in the site afterwards: nothing was deleted", frequency)
	}

	app.Printf("deleted %s\n", frequency)
	return nil
}

// validFrequency reports whether what was typed is a frequency the entry screen
// would take. The screen accepts digits and a decimal point and nothing else.
//
// Parameters:
//   - value: the frequency as it was typed
//
// Returns:
//   - the frequency as it should be typed, which is what was written unless it
//     carried a unit
//   - error if value is not a number of megahertz, or carries a sign or an
//     exponent the entry screen has no key for
func validFrequency(value string) (string, error) {
	_, typed, err := device.ParseEnteredFrequency(value)
	switch {
	case errors.Is(err, device.ErrFrequencyNotTypeable):
		// Said apart from a number that could not be read, because a sign or an
		// exponent is a thing this screen has no key for rather than a mistake
		// in the number, and telling somebody who wrote "-5" that it is not a
		// number sends them to fix the wrong half of it.
		return "", fmt.Errorf("%q is not a frequency the scanner's screen would accept: "+
			"write it in megahertz, as 851.050", value)
	case err != nil:
		return "", fmt.Errorf("%q is not a number of megahertz: "+
			"write it in megahertz, as 851.050", value)
	}
	return typed, nil
}
