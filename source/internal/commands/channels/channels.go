// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package channels implements the "channels" command, which lists the channels
// inside one department.
//
// It is the only command here that reads something the protocol will not
// report. The request that would list channels answers with the wrong document
// on this firmware, so the channels are read the way a person reads them: by
// walking the scanner's menus and reading its screen.
package channels

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

// New returns the channels command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that lists a department's channels, carrying the --names
//     flag and the new, rename and delete subcommands
func New(app *appcontext.App) *cobra.Command {
	var names bool

	cmd := &cobra.Command{
		Use:   "channels <department>",
		Short: "List the channels in a department",
		Long: "Channels lists the channels inside one department, with the frequency of each.\n\n" +
			"This is the one thing the scanner will not simply report. The protocol request\n" +
			"that should list channels answers with the wrong document, so these are read\n" +
			"by walking the scanner's own menus and reading its screen, which takes a few\n" +
			"seconds rather than one exchange.\n\n" +
			"Reading the frequencies is most of that work, because each one means opening\n" +
			"a channel and coming back out. Use --names to skip it.\n\n" +
			"This stops the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app, args[0], names)
		},
	}

	cmd.AddCommand(newNew(app), newRename(app), newDelete(app))

	cmd.Flags().BoolVar(&names, "names", false,
		"list the names only, without opening each channel to read its frequency")
	return cmd
}

// newDelete returns the "channels delete" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that deletes the channel named on the command line,
//     carrying the --yes flag that has to be passed for it to act
func newDelete(app *appcontext.App) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <department> <name> --yes",
		Short: "Delete a channel",
		Long: "Delete removes one channel from a department.\n\n" +
			"The department comes first because channel names are not unique: a scanner\n" +
			"can hold several channels called DISPATCH, one per department, and naming\n" +
			"the channel alone would not say which.\n\n" +
			"There is no undo. Nothing on the scanner keeps a copy, and this tool cannot\n" +
			"put back what it removes, so --yes is required: without it the command says\n" +
			"what would go and changes nothing.\n\n" +
			"The department's channels are read back afterwards to confirm it is gone.\n" +
			"This stops the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), app, args[0], args[1], yes)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
	return cmd
}

// newNew returns the "channels new" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that creates a channel on the frequency or talkgroup
//     given on the command line
func newNew(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "new <department> <frequency|TGID:id> <name>",
		Short: "Create a channel in a department",
		Long: "New creates a channel in one department, with a name.\n\n" +
			"What the channel carries depends on the system above it, and the scanner\n" +
			"decides which it asks for. A department on a conventional system takes a\n" +
			"frequency in megahertz. A department on a trunked system takes a talkgroup,\n" +
			"written with the TGID: prefix the scanner itself uses to report one:\n\n" +
			"  radiocli channels new \"FIRE RESCUE\" 153.980 DISPATCH\n" +
			"  radiocli channels new \"GREENDALE FIRE\" TGID:9051 \"FIRE DISPATCH\"\n\n" +
			"They are one argument rather than two because they are one thing: the address\n" +
			"of what the channel receives. A trunked system shares a pool of frequencies\n" +
			"across everybody on it and hands one out per transmission, so a talkgroup, not\n" +
			"a frequency, is what identifies a conversation there.\n\n" +
			"Giving the wrong kind for the department is refused before anything is created.\n\n" +
			"It comes before the name because that is the order the scanner asks in: New\n" +
			"Channel opens an entry screen before the channel exists at all, and the name is\n" +
			"given afterwards. The channel is read back afterwards to confirm it is there.\n" +
			"This stops the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd.Context(), app, args[0], args[1], args[2])
		},
	}
}

// newRename returns the "channels rename" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that renames the channel named on the command line
func newRename(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <department> <name> <new-name>",
		Short: "Change a channel's name",
		Long: "Rename changes the name of one channel, leaving its frequency and everything\n" +
			"else about it alone.\n\n" +
			"The department comes first because channel names are not unique: a scanner can\n" +
			"hold several channels called DISPATCH, one per department, and naming the\n" +
			"channel alone would not say which.\n\n" +
			"The scanner has no command for this, so it is done the way a person would: the\n" +
			"department's channel list is walked to the channel, its Edit Name screen is\n" +
			"selected, and the name is typed there by turning the knob to each character in\n" +
			"turn. Every character is checked after it is set.\n\n" +
			"Nothing is saved until the whole name is typed, so an interrupted rename leaves\n" +
			"the old name untouched. Renaming is the only way to correct a name: the channel\n" +
			"keeps its frequency and its settings, which deleting and recreating it would\n" +
			"lose.\n\n" +
			"The channel is read back afterwards to confirm the new name is there. This\n" +
			"stops the scanner scanning, and returns it to scanning when it is done.",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(cmd.Context(), app, args[0], args[1], args[2])
		},
	}
}

// address is what the channel receives, however it is addressed, for the one
// column a table has room for.
//
// Returns:
//   - string holding the talkgroup, with the prefix the scanner writes it
//     with, or the frequency when the channel has one of its own
func (c channel) address() string {
	if c.Talkgroup != "" {
		return talkgroupPrefix + c.Talkgroup
	}
	return c.Frequency
}

// ask reads a department's channels from the protocol, falling back to the
// scanner's menus if it will not answer.
//
// The protocol is one exchange against several seconds of walking, and it is
// the only one of the two that reports what a talkgroup channel holds, so it is
// tried first. The walk stays because it is the thing that worked when the list
// requests were thought to be broken, and a firmware that answers differently
// would otherwise take this command with it.
//
// A read that failed because the caller gave up is not a firmware that answers
// differently, so it is passed straight back. Falling back on it would answer a
// Ctrl-C by starting the slower of the two ways of doing the work, spending
// several seconds and a great many key presses on a result nobody is waiting
// for any more.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - index: the index of the department to read
//
// Returns:
//   - []channel holding the department's channels, empty when it holds none
//   - error if the caller gave up, if the walk cannot reach the channel list,
//     or if it fails part way through reading it
func ask(ctx context.Context, client *device.Scanner, index string) ([]channel, error) {
	read, err := catalog.ReadChannels(ctx, client, index)
	if err == nil {
		found := make([]channel, 0, len(read))
		for _, c := range read {
			found = append(found, channel{
				Name:      c.Name,
				Frequency: strings.TrimSpace(c.Frequency),
				Talkgroup: strings.TrimSpace(c.Talkgroup),
			})
		}
		return found, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	return walk(ctx, client, index, false)
}

// collect reads the channel list the scanner is showing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner, already showing a department's channel list
//   - namesOnly: read the names only, without opening each channel to read
//     what it is tuned to
//
// Returns:
//   - []channel holding the channels on the list, without the entry that
//     creates one
//   - error if the list cannot be read, or if a channel's frequency cannot be
//     reached
func collect(ctx context.Context, client *device.Scanner, namesOnly bool) ([]channel, error) {
	entries, err := menus.Entries(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("reading the channel list: %w", err)
	}

	found := make([]channel, 0, len(entries))
	for _, name := range entries {
		if strings.EqualFold(name, newChannel) {
			continue
		}

		c := channel{Name: name}
		if !namesOnly {
			if c.Frequency, err = frequency(ctx, client, name); err != nil {
				return nil, err
			}
		}
		found = append(found, c)
	}
	return found, nil
}

// entered reads what a new channel is to receive and checks it before the
// scanner is touched.
//
// The two kinds are told apart by the TGID: prefix, and each is refused in its
// own terms. Blank used to be refused as a missing talkgroup whichever kind was
// meant, so somebody who left a frequency off was told to write "TGID:9051",
// which is advice for a different mistake than the one they made.
//
// A frequency is also read here rather than only being typed, because the entry
// screen has keys for digits and a decimal point alone. Finding out that a
// frequency cannot be typed after the New Channel screen is open means a
// half-made channel to clean up; finding out first costs nothing.
//
// Parameters:
//   - address: what the channel is to receive, as it was written
//
// Returns:
//   - the value to type into the entry screen
//   - true when it is a talkgroup rather than a frequency
//   - error if nothing was given, or if a frequency cannot be read or typed
func entered(address string) (string, bool, error) {
	value, wantTalkgroup := talkgroup(address)
	value = strings.TrimSpace(value)

	if wantTalkgroup {
		if value == "" {
			return "", false, fmt.Errorf("no talkgroup was given: write it as TGID:9051")
		}
		return value, true, nil
	}

	if value == "" {
		return "", false, fmt.Errorf("no frequency was given: write it in megahertz, as 155.475, " +
			"or write a talkgroup as TGID:9051")
	}

	_, typed, err := device.ParseEnteredFrequency(value)
	switch {
	case errors.Is(err, device.ErrFrequencyNotTypeable):
		return "", false, fmt.Errorf("%q is not a frequency the scanner's screen would accept: "+
			"write it in megahertz, as 155.475, or write a talkgroup as TGID:9051", address)
	case err != nil:
		return "", false, fmt.Errorf("%q is not a frequency: "+
			"write it in megahertz, as 155.475, or write a talkgroup as TGID:9051", address)
	}
	return typed, false, nil
}

// expected checks that the entry screen the scanner opened is the one the
// argument was written for, and explains the mismatch in terms of the system
// rather than the screen.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner, already showing the screen New Channel opened
//   - wantTalkgroup: whether the argument was written as a talkgroup
//   - department: the department being written to, for the error message
//
// Returns:
//   - error if the screen cannot be read, if the scanner opened neither entry
//     screen, or if it opened the one the argument was not written for
func expected(ctx context.Context, client *device.Scanner, wantTalkgroup bool, department string) error {
	info, err := client.MenuInfo(ctx)
	if err != nil {
		return fmt.Errorf("reading the entry screen: %w\n"+
			"Nothing has been created. Run \"radiocli scan\"", err)
	}

	title := strings.TrimSpace(info.Title)
	gotTalkgroup := strings.EqualFold(title, inputTalkgroup)

	// Anything that is neither screen means the walk ended somewhere
	// unexpected, which is worth saying plainly rather than guessing at.
	if !gotTalkgroup && !strings.EqualFold(title, inputFrequency) {
		return fmt.Errorf("the scanner opened %q rather than an entry screen: "+
			"nothing has been created. Run \"radiocli scan\"", title)
	}

	if gotTalkgroup == wantTalkgroup {
		return nil
	}

	if wantTalkgroup {
		return fmt.Errorf("%q is in a conventional system and takes a frequency, not a talkgroup: "+
			"write it in megahertz, as 153.980\n"+
			"Nothing has been created", department)
	}
	return fmt.Errorf("%q is in a trunked system and takes a talkgroup, not a frequency: "+
		"write it with the prefix the scanner uses, as TGID:9051\n"+
		"Nothing has been created", department)
}

// frequency opens one channel, reads what it is tuned to, and comes back out.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner, already showing a department's channel list
//   - name: the channel to open
//
// Returns:
//   - string holding the frequency the channel is tuned to, empty for a
//     talkgroup channel, which has no frequency screen at all
//   - error if the channel cannot be reached or opened, or if its frequency
//     screen cannot be read
func frequency(ctx context.Context, client *device.Scanner, name string) (string, error) {
	if err := menus.StepTo(ctx, client, name); err != nil {
		return "", fmt.Errorf("looking for the channel %q: %w", name, err)
	}
	if err := menus.Enter(ctx, client); err != nil {
		return "", err
	}

	// Two levels down, so two levels back, whatever happens from here.
	defer func() {
		menus.Back(ctx, client)
	}()

	if err := menus.StepTo(ctx, client, editFrequency); err != nil {
		// A talkgroup channel on a trunked system has no frequency screen. That
		// is a fact about the channel rather than a failure to read it.
		return "", nil
	}
	if err := menus.Enter(ctx, client); err != nil {
		return "", err
	}
	defer func() {
		menus.Back(ctx, client)
	}()

	info, err := client.MenuInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("reading the frequency of %q: %w", name, err)
	}
	return strings.TrimSpace(info.Value), nil
}

// kind names what is being typed, for a failure message.
//
// Parameters:
//   - talkgroup: whether a talkgroup is being typed rather than a frequency
//
// Returns:
//   - string holding the word for what is being typed
func kind(talkgroup bool) string {
	if talkgroup {
		return "talkgroup"
	}
	return "frequency"
}

// listed names the channels a department holds, for a failure message.
//
// Parameters:
//   - found: the channels the department holds
//
// Returns:
//   - string holding every channel name in quotes, or a phrase saying there
//     are none
func listed(found []channel) string {
	if len(found) == 0 {
		return "no channels at all"
	}

	names := make([]string, 0, len(found))
	for _, c := range found {
		names = append(names, fmt.Sprintf("%q", c.Name))
	}
	return strings.Join(names, ", ")
}

// read lists a department's channels, for confirming what was just created.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, unused here because the walk reports
//     through its error rather than through the output
//   - client: the scanner to read from
//   - department: the department to read, by name or by index
//   - namesOnly: read the names only, without opening each channel to read
//     what it is tuned to
//
// Returns:
//   - []channel holding the department's channels, empty when it holds none
//   - error if the department cannot be reached, if the list cannot be read,
//     or if the scanner cannot be taken out of the menus
func read(ctx context.Context, app *appcontext.App, client *device.Scanner, department string, namesOnly bool) ([]channel, error) {
	if err := navigate.ToChannels(ctx, client, department); err != nil {
		return nil, err
	}

	found, err := collect(ctx, client, namesOnly)
	if err != nil {
		menus.Leave(ctx, client)
		return nil, err
	}

	_, err = menus.Leave(ctx, client)
	return found, err
}

// renderChannels writes the listing as an aligned table.
//
// Parameters:
//   - app: the application context, for output
//   - found: the channels to write
//   - namesOnly: write the names only, without the column saying what each
//     channel receives
//
// Returns:
//   - error if writing the table fails
func renderChannels(app *appcontext.App, found []channel, namesOnly bool) error {
	if len(found) == 0 {
		app.Notef("That department holds no channels.\n")
		return nil
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	if namesOnly {
		fmt.Fprintln(w, "NAME")
		for _, c := range found {
			fmt.Fprintf(w, "%s\n", c.Name)
		}
	} else {
		fmt.Fprintln(w, "NAME\tRECEIVES")
		for _, c := range found {
			fmt.Fprintf(w, "%s\t%s\n", c.Name, render.Dash(c.address()))
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the channel list: %w", err)
	}
	return nil
}

// run reports what is in a department.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - want: the department to list, by name or by index
//   - namesOnly: list the names only, without opening each channel to read
//     what it is tuned to
//
// Returns:
//   - error if no scanner is named, if the department cannot be resolved or
//     read, or if writing the report fails
func run(ctx context.Context, app *appcontext.App, want string, namesOnly bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	index, err := catalog.ResolveDepartment(ctx, client, want)
	if err != nil {
		return err
	}

	found, err := ask(ctx, client, index)
	if err != nil {
		return err
	}

	// Asking for names only means what it says in both forms of output, rather
	// than only hiding a column the text table would have drawn.
	if namesOnly {
		for i := range found {
			found[i].Frequency, found[i].Talkgroup = "", ""
		}
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, found)
	}

	return renderChannels(app, found, namesOnly)
}

// runDelete removes a channel and confirms it is gone.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - department: the department holding the channel, by name or by index
//   - name: the channel to delete
//   - yes: whether the caller has agreed to the deletion
//
// Returns:
//   - error if no scanner is named, if the department cannot be read, if it
//     holds no channel by that name, if yes was not passed, if the walk or the
//     prompt fails, or if the channel is still there afterwards
func runDelete(ctx context.Context, app *appcontext.App, department, name string, yes bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Look before asking about --yes, so a channel the department does not
	// hold is reported as that rather than as a missing flag. Names only,
	// which is the quick read: the frequencies are not needed to find one.
	before, err := read(ctx, app, client, department, true)
	if err != nil {
		return err
	}

	found := ""
	for _, c := range before {
		if strings.EqualFold(c.Name, name) {
			found = c.Name
			break
		}
	}
	if found == "" {
		return fmt.Errorf("no channel called %q in %q: it holds %s",
			name, department, listed(before))
	}

	if !yes {
		return fmt.Errorf("deleting the channel %q from %q cannot be undone: pass --yes",
			found, department)
	}

	if err := navigate.ToChannels(ctx, client, department); err != nil {
		return err
	}

	// Selecting a channel by name opens that channel's own menu, the same way
	// selecting New Channel opens a frequency screen.
	if err := menus.Select(ctx, client, found); err != nil {
		return fmt.Errorf("looking for the channel %q: %w", found, err)
	}
	if err := menus.Select(ctx, client, deleteChannel); err != nil {
		return fmt.Errorf("looking for %q on the channel's menu: %w", deleteChannel, err)
	}
	if err := menus.ConfirmDelete(ctx, client); err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	after, err := read(ctx, app, client, department, true)
	if err != nil {
		return err
	}
	for _, c := range after {
		if strings.EqualFold(c.Name, found) {
			return fmt.Errorf("the channel %q is still there afterwards: nothing was deleted", found)
		}
	}

	return render.Changed(app,
		render.Mutation{Action: "deleted", Kind: "channel", Name: found, In: department},
		"deleted "+found)
}

// runNew creates a channel, sets what it receives, and names it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - department: the department to create the channel in, by name or by index
//   - address: what the channel receives, as a frequency in megahertz or a
//     talkgroup written with the TGID: prefix
//   - name: what to call the new channel
//
// Returns:
//   - error if no scanner is named, if no talkgroup was given after the
//     prefix, if the department cannot be reached, if the entry screen is the
//     one the address was not written for, if the address or the name cannot be
//     typed, or if the channel does not appear afterwards
func runNew(ctx context.Context, app *appcontext.App, department, address, name string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	value, wantTalkgroup, err := entered(address)
	if err != nil {
		return err
	}

	if err := navigate.ToChannels(ctx, client, department); err != nil {
		return err
	}

	// This opens an entry screen. Nothing exists yet: the channel is created by
	// what is typed into it.
	if err := menus.Select(ctx, client, newChannelEntry); err != nil {
		return fmt.Errorf("creating the channel: %w", err)
	}

	// Which screen it opened says what the department deals in. Checking it
	// before typing means the wrong kind is refused rather than entered: a
	// talkgroup typed into a frequency screen would be created as a frequency
	// of 9051 MHz, which the scanner would take and nobody would want.
	if err := expected(ctx, client, wantTalkgroup, department); err != nil {
		menus.Leave(ctx, client)
		return err
	}

	if err := textinput.Set(ctx, client, value); err != nil {
		return fmt.Errorf("entering the %s: %w\n"+
			"Nothing has been created. Run \"radiocli scan\" to leave the scanner as it was",
			kind(wantTalkgroup), err)
	}

	if err := menus.Select(ctx, client, editNameEntry); err != nil {
		return fmt.Errorf("the channel was created on %s, but its name screen could not be "+
			"opened: %w\nRun \"radiocli scan\"", address, err)
	}

	if err := textinput.Set(ctx, client, name); err != nil {
		return fmt.Errorf("the channel was created on %s, but naming it failed: %w\n"+
			"Run \"radiocli scan\"", address, err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read it back, since the scanner is the only authority on what was made.
	found, err := read(ctx, app, client, department, true)
	if err != nil {
		return err
	}
	for _, c := range found {
		if c.Name == name {
			return render.Changed(app,
				render.Mutation{Action: "created", Kind: "channel", Name: name, In: department}, name)
		}
	}
	return fmt.Errorf("the channel does not appear under %q afterwards: "+
		"run \"radiocli channels %s\" to see what was created", name, department)
}

// runRename walks to a channel and types a new name into it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - department: the department holding the channel, by name or by index
//   - name: the channel to rename
//   - newName: what to call it instead
//
// Returns:
//   - error if no scanner is named, if the department cannot be read, if it
//     holds no channel by that name, if the walk fails, if the name cannot be
//     typed, or if the channel does not appear under the new name afterwards
func runRename(ctx context.Context, app *appcontext.App, department, name, newName string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Look first, so a channel the department does not hold is reported as that
	// rather than as a failed walk. Names only, which is the quick read: the
	// frequencies are not needed to find one.
	before, err := read(ctx, app, client, department, true)
	if err != nil {
		return err
	}

	found := ""
	for _, c := range before {
		if strings.EqualFold(c.Name, name) {
			found = c.Name
			break
		}
	}
	if found == "" {
		return fmt.Errorf("no channel called %q in %q: it holds %s",
			name, department, listed(before))
	}

	if found == newName {
		// Nothing to do, and no reason to stop the scanner scanning to do it.
		app.Notef("That channel is already called %q.\n", newName)
		return nil
	}

	if err := navigate.ToChannels(ctx, client, department); err != nil {
		return err
	}

	// Selecting a channel by name opens that channel's own menu, the same way
	// "channels delete" reaches Delete Channel.
	if err := menus.Select(ctx, client, found); err != nil {
		return fmt.Errorf("looking for the channel %q: %w", found, err)
	}
	if err := menus.Select(ctx, client, editNameEntry); err != nil {
		return fmt.Errorf("looking for %q on the channel's menu: %w\n"+
			"Nothing has been changed. Run \"radiocli scan\"", editNameEntry, err)
	}

	if err := textinput.Set(ctx, client, newName); err != nil {
		// Leaving the entry screen discards, so nothing has been saved. Say so,
		// rather than leaving the reader wondering whether the name is mangled.
		return fmt.Errorf("typing the new name: %w\n"+
			"The scanner is still on the entry screen and nothing has been saved. "+
			"Run \"radiocli scan\" to leave it as it was", err)
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Read back, because the scanner is the only authority on what it holds.
	after, err := read(ctx, app, client, department, true)
	if err != nil {
		return err
	}
	for _, c := range after {
		if c.Name == newName {
			return render.Changed(app,
				render.Mutation{Action: "renamed", Kind: "channel", Name: newName, Was: name, In: department},
				newName)
		}
	}
	return fmt.Errorf("the channel does not appear under %q afterwards: "+
		"run \"radiocli channels %s\" to see what it is called", newName, department)
}

// talkgroup splits the positional argument into the value to type and whether
// it is a talkgroup.
//
// The prefix is matched without regard to case, since the argument is typed by
// a person and the scanner writes it capitalised.
//
// Parameters:
//   - value: the address as it was typed, with or without the prefix
//
// Returns:
//   - string holding the value to type into the scanner, with the prefix taken
//     off when there was one
//   - bool reporting whether the value is a talkgroup
func talkgroup(value string) (string, bool) {
	if len(value) < len(talkgroupPrefix) {
		return value, false
	}
	if !strings.EqualFold(value[:len(talkgroupPrefix)], talkgroupPrefix) {
		return value, false
	}
	return strings.TrimSpace(value[len(talkgroupPrefix):]), true
}

// walk reads a department's channels off the scanner's screen.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - index: the index of the department to read
//   - namesOnly: read the names only, without opening each channel to read
//     what it is tuned to
//
// Returns:
//   - []channel holding the department's channels, empty when it holds none
//   - error if the department's menu cannot be opened, if it carries no entry
//     for the channels, if the list cannot be read, or if the scanner cannot be
//     taken out of the menus
func walk(ctx context.Context, client *device.Scanner, index string, namesOnly bool) ([]channel, error) {
	if err := client.OpenMenu(ctx, device.MenuDepartment, index); err != nil {
		return nil, fmt.Errorf("opening the department's menu: %w", err)
	}

	if err := menus.StepTo(ctx, client, editChannel); err != nil {
		return nil, fmt.Errorf("looking for %q on the department's menu: %w", editChannel, err)
	}
	if err := menus.Enter(ctx, client); err != nil {
		return nil, err
	}

	found, err := collect(ctx, client, namesOnly)
	if err != nil {
		// Leave the scanner scanning rather than parked in a menu, but report
		// what went wrong rather than what tidying up did.
		menus.Leave(ctx, client)
		return nil, err
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return nil, err
	}
	return found, nil
}
