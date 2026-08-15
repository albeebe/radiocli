// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package scanning implements the "scanning" command, which lists every
// channel the scanner is currently set up to scan.
//
// It works by holding the scanner on a channel and turning the knob, which is
// how a person would do it. That turns out to be the only way to see inside
// the full database: the protocol's own listing request answers with the wrong
// document and hangs the scanner, the database has no menu of its own, and its
// systems have no menu index. Held, the knob walks every channel in the scan,
// crossing department and system boundaries on its own, and the screen names
// each one along with its frequency or talkgroup.
package scanning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the scanning command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner, the output
//     format and the streams to write to from
//
// Returns:
//   - the "scanning" command, with the systems subcommand already attached
func New(app *appcontext.App) *cobra.Command {
	var limit int
	var watch time.Duration

	cmd := &cobra.Command{
		Use:   "scanning",
		Short: "Report what the scanner is scanning right now",
		Long: "Scanning reports what the scanner is working through right now, whether that\n" +
			"comes from a favorites list, a zip code, or the GPS.\n\n" +
			"How it answers depends on what is switched on, because the two cases are not\n" +
			"the same question. Favorites lists are walked exactly: the scanner is held on\n" +
			"a channel and the knob turned, which lists every channel with its frequency in\n" +
			"about a second.\n\n" +
			"The full database cannot be walked that way. Held, the knob browses the whole\n" +
			"database rather than the part being scanned, which is hundreds of thousands of\n" +
			"entries and never finishes. So that case is watched instead: the scanner names\n" +
			"each system and department as it cycles past, and this collects them until\n" +
			"they stop being new.\n\n" +
			"This stops the scanner scanning only in the first case, and returns it when\n" +
			"it is done.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app, limit, watch)
		},
	}

	cmd.AddCommand(newSystems(app))

	cmd.Flags().IntVar(&limit, "limit", defaultLimit,
		"stop after this many channels, when walking a favorites list")
	cmd.Flags().DurationVar(&watch, "watch", watchBudget,
		"how long to watch the scan cycle, when the full database is switched on")
	return cmd
}

// newSystems returns the "scanning systems" subcommand.
//
// Parameters:
//   - app: the application context the subcommand reads the scanner and the
//     streams to write to from
//
// Returns:
//   - the "systems" subcommand, which lists the systems being cycled through
func newSystems(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "systems",
		Short: "List the systems the scanner is cycling through",
		Long: "Systems lists the systems the scanner is currently working through, whether\n" +
			"they come from a favorites list, a zip code, or the GPS.\n\n" +
			"How it answers depends on what is switched on. Favorites lists are read\n" +
			"straight from the scanner's memory, which is instant, silent, and leaves the\n" +
			"scanner scanning throughout.\n\n" +
			"The full database cannot be read that way, because it has no listing and its\n" +
			"systems have no index. That case is walked instead: the system level is\n" +
			"selected and the knob turned, stepping from one system to the next. The walk\n" +
			"is a loop, so it finishes when it arrives back where it started, and that\n" +
			"makes the answer complete rather than a sample. It stops the scanner\n" +
			"scanning, and returns it when it is done.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSystems(cmd.Context(), app)
		},
	}
}

// confirms checks the walk really has come round, by clicking on and seeing
// the systems that followed the start the first time.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, with the system level selected
//   - seen: the systems recorded so far, beginning with the one the walk
//     started on
//
// Returns:
//   - true if the systems that followed the start came round again in the same
//     order, false if the knob stopped moving or a different system came up
//   - error if a click could not be sent or the screen could not be read
func confirms(ctx context.Context, client *device.Scanner, seen []string) (bool, error) {
	want := seen[1:]
	if len(want) > cycleConfirm {
		want = want[:cycleConfirm]
	}

	last := seen[0]
	for _, expected := range want {
		moved := false

		// Each click already waits for the screen to change, so a click that
		// comes back with the same name has waited stepCap for it. Only a few
		// of those are worth trying: past that the knob is not moving and the
		// walk has not closed.
		for attempt := 0; attempt < confirmClicks; attempt++ {
			name, err := stepSystem(ctx, client, last)
			if err != nil {
				return false, err
			}
			if name == "" {
				return false, nil
			}
			if name != last {
				last = name
				moved = true
				break
			}
		}
		if !moved || last != expected {
			return false, nil
		}
	}
	return true, nil
}

// cycleSystems presses the system key once, then clicks the knob round,
// recording what comes up, and finds the repeating cycle in the recording.
//
// Spotting the loop closing as it happens does not work. The screen is read
// while it is being redrawn often enough that a stray reading matches the
// system the walk began on, and the walk stops early believing it has come
// full circle. That produced 15 systems on one run and 24 on the next, both
// claiming to be complete.
//
// Recording first and looking for the period afterwards is not vulnerable to
// that. A cycle is only accepted once the whole of it has been seen twice in
// the same order, which a single bad reading cannot fake.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - the systems the walk saw, in the order they came up
//   - true if the walk came back to where it started, which makes the answer
//     complete rather than a sample
//   - error if the scanner could not be put back to scanning, the system level
//     could not be selected, or the screen could not be read
func cycleSystems(ctx context.Context, client *device.Scanner) ([]string, bool, error) {
	// Start from a known state. The system level is a toggle with no way to
	// read it, so a walk beginning while it is already on switches it off and
	// collects nothing: three runs in a row returned 5, 1 and 18 systems from
	// an unchanged scanner purely because each began wherever the last one
	// left off. Returning the scanner to plain scanning first makes the walk
	// depend on nothing but itself.
	if err := reset(ctx, client); err != nil {
		return nil, false, err
	}

	if err := client.PressKey(ctx, systemArmKey, device.KeyPress); err != nil {
		return nil, false, fmt.Errorf("selecting the system level: %w", err)
	}

	first, err := systemName(ctx, client)
	if err != nil {
		return nil, false, err
	}
	if first == "" {
		return nil, true, nil
	}

	seen := []string{first}

	// Bounded by clicks rather than by systems found. A scan holding a single
	// system has nowhere to step to, so the name never changes and a loop
	// counting only new systems never advances at all.
	unchanged := 0
	rearmed := false
	for clicks := 0; clicks < clickLimit && len(seen) < systemLimit; clicks++ {
		name, err := stepSystem(ctx, client, seen[len(seen)-1])
		if err != nil {
			return nil, false, err
		}
		if name == "" {
			return distinct(seen), false, nil
		}

		// A scan holding a single system has nowhere to step to, and the knob
		// walks off the scanning screen into the scanner's menus instead,
		// where the lines this reads mean something else entirely: one run
		// reported "Copy System" as a system and left the scanner sitting in
		// the main menu.
		//
		// Asking whether the scanner is in a menu is the tell. Comparing the
		// source line against the one the walk started on is not: that line
		// names the favorites list a system belongs to, so it changes every
		// time the walk crosses from one list into the next. With two lists
		// switched on, the walk stopped at the first system of the second and
		// reported it as the whole answer.
		if inMenu, err := inAMenu(ctx, client); err != nil {
			return nil, false, err
		} else if inMenu {
			return distinct(seen), true, nil
		}

		// Reading faster than the scanner redraws shows the same system twice.
		// Collapsing those is what a person sees while turning the knob. But a
		// run of them that never breaks means the knob has nothing to move to,
		// which is what a one-system scan looks like, and that is the end of
		// the walk rather than a reason to keep turning.
		if name == seen[len(seen)-1] {
			unchanged++
			if unchanged < stuckClicks {
				continue
			}

			// The knob has stopped moving anything. Either the system level
			// has lapsed, which it does on its own, or the scan holds a single
			// system and there is nowhere to step to.
			//
			// The key toggles, so pressing it while the level is still active
			// switches it off and the walk quietly breaks: that is why it is
			// pressed once at the start and only here, where the level is
			// known to be gone. If the knob is still dead afterwards, the scan
			// really does hold one system.
			if rearmed {
				return seen, true, nil
			}
			if err := client.PressKey(ctx, systemArmKey, device.KeyPress); err != nil {
				return nil, false, fmt.Errorf("selecting the system level: %w", err)
			}
			rearmed = true
			unchanged = 0
			continue
		}
		unchanged = 0
		rearmed = false

		// Back at the start, probably. Two more clicks decide it: a real cycle
		// carries on with the second and third systems, while a stray reading
		// does not. Confirming is far cheaper than requiring the whole
		// recording to be perfectly periodic, which one bad reading prevents
		// for ever, and which is why this used to run until it hit its limit.
		if name == first && len(seen) > 1 {
			ok, err := confirms(ctx, client, seen)
			if err != nil {
				return nil, false, err
			}
			if ok {
				return seen, true, nil
			}
		}

		seen = append(seen, name)
	}
	return distinct(seen), false, nil
}

// distinct keeps the first occurrence of each name, for reporting a walk that
// never settled into a cycle.
//
// Parameters:
//   - seen: the systems the walk recorded, in the order they came up
//
// Returns:
//   - the same systems with the repeats removed, in the order they first
//     appeared
func distinct(seen []string) []string {
	var out []string
	had := make(map[string]bool)
	for _, n := range seen {
		if !had[n] {
			had[n] = true
			out = append(out, n)
		}
	}
	return out
}

// hold stops the scanner on a channel, which is what makes the knob step
// through them rather than doing nothing.
//
// The key toggles rather than sets, and a press made while the scanner is
// between channels is ignored, so this presses until the screen shows a
// channel and checks rather than counting presses.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - error if the screen could not be read, a key press could not be sent, or
//     the scanner would not settle on a channel at all
func hold(ctx context.Context, client *device.Scanner) error {
	for attempt := 0; attempt < holdAttempts; attempt++ {
		held, err := settledScan(ctx, client)
		if err != nil {
			return err
		}
		if held {
			return nil
		}

		if err := client.PressKey(ctx, device.KeySoft3, device.KeyPress); err != nil {
			return fmt.Errorf("holding the scanner on a channel: %w", err)
		}
	}
	return fmt.Errorf("the scanner would not hold on a channel after %d presses\n"+
		"It may not be scanning anything: run \"radiocli favorites\" to see what is switched on",
		holdAttempts)
}

// holding reports whether the scanner is stopped on a channel.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - true if the screen is showing a channel the scanner has stopped on
//   - error if the screen could not be read
func holding(ctx context.Context, client *device.Scanner) (bool, error) {
	e, ok, err := read(ctx, client)
	if err != nil {
		return false, err
	}
	return ok && e.Channel != "", nil
}

// inAMenu reports whether the knob has taken the scanner off the scanning
// screen and into its menus.
//
// The protocol answers a menu request with ErrNotInMenu while the scanner is
// scanning, which is exactly the question being asked here, so the error is
// the answer rather than a failure.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - true if the scanner is in a menu, false if it is still scanning
//   - error if the scanner answered with anything other than the two states
func inAMenu(ctx context.Context, client *device.Scanner) (bool, error) {
	if _, err := client.MenuInfo(ctx); err != nil {
		if errors.Is(err, device.ErrNotInMenu) {
			return false, nil
		}
		return false, fmt.Errorf("checking whether the scanner is still scanning: %w", err)
	}
	return true, nil
}

// key identifies an entry, for noticing when the walk has come back round.
//
// Returns:
//   - the entry's fields joined by a character no name can hold, so two
//     entries compare equal only when every field matches
func (e entry) key() string {
	return e.Source + "\x00" + e.System + "\x00" + e.Department + "\x00" + e.Channel + "\x00" + e.Value
}

// lineText reads one line of the display as plain text.
//
// The signal meter draws its own glyphs on the end of these lines, which are
// not text and are not part of any name, so anything outside printable ASCII
// is dropped rather than compared.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//   - line: which line of the display to read, counting from zero
//
// Returns:
//   - the line as plain text with the spaces around it removed, or nothing at
//     all when the screen has no such line
//   - error if the screen could not be read
func lineText(ctx context.Context, client *device.Scanner, line int) (string, error) {
	display, err := client.Display(ctx)
	if err != nil {
		return "", fmt.Errorf("reading the scanner's screen: %w", err)
	}
	if len(display.Lines) <= line {
		return "", nil
	}

	var b strings.Builder
	for _, r := range display.Lines[line].Text {
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// listedSystems asks the scanner for the systems in every list it is scanning.
//
// Avoided systems are left out, because the question is what the scanner is
// working through rather than what it holds, and it steps straight past those.
// The built-in search source is left out too: it scans frequency ranges and
// holds no systems of its own.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//   - lists: the favorites lists the scanner is set to scan
//
// Returns:
//   - the systems those lists hold, without the repeats and without the ones
//     the scanner steps past
//   - error if a list's systems could not be read, naming the list
func listedSystems(ctx context.Context, client *device.Scanner, lists []catalog.FavoritesList) ([]string, error) {
	var names []string
	had := make(map[string]bool)

	for _, l := range lists {
		if l.BuiltIn {
			continue
		}

		systems, err := catalog.ReadSystems(ctx, client, l.Index)
		if err != nil {
			return nil, fmt.Errorf("reading the systems in %q: %w", l.Name, err)
		}

		for _, s := range systems {
			if s.Avoided || had[s.Name] {
				continue
			}
			had[s.Name] = true
			names = append(names, s.Name)
		}
	}
	return names, nil
}

// monitored returns the lists switched on for scanning, and whether the full
// database is one of them.
//
// Parameters:
//   - lists: every favorites list the scanner holds
//
// Returns:
//   - the lists that are switched on for scanning
//   - true if the scanner's built-in full database is one of them
func monitored(lists []catalog.FavoritesList) ([]catalog.FavoritesList, bool) {
	var on []catalog.FavoritesList
	database := false
	for _, l := range lists {
		if !l.Monitored {
			continue
		}
		on = append(on, l)
		if l.BuiltIn && strings.EqualFold(l.Name, fullDatabase) {
			database = true
		}
	}
	return on, database
}

// next turns the knob one step and waits for the screen to catch up.
//
// The knob is turned far faster than the scanner redraws, so reading straight
// after a turn returns the channel that was already there. Left unchecked that
// reads as the walk having come back round to where it started, and the whole
// thing stops after one channel.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, held on a channel
//   - from: the channel the walk is stepping away from
//
// Returns:
//   - the channel the scanner moved on to
//   - true if it moved at all, false when the screen never changed, which on a
//     scan holding one channel is the end of the walk rather than a failure
//   - error if the knob could not be turned or the screen could not be read
func next(ctx context.Context, client *device.Scanner, from entry) (entry, bool, error) {
	if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
		return entry{}, false, fmt.Errorf("turning the knob: %w", err)
	}

	for attempt := 0; attempt < settleReads; attempt++ {
		e, ok, err := read(ctx, client)
		if err != nil {
			return entry{}, false, err
		}
		if ok && e.key() != from.key() {
			return e, true, nil
		}
	}

	// The screen never changed. On a scan holding exactly one channel that is
	// the truth rather than a failure, so it is reported as the end of the
	// walk.
	return entry{}, false, nil
}

// observe watches the scan cycle without touching the scanner.
//
// The scanner names each system and department on screen as it works through
// them, so watching enumerates what is actually being scanned. It is the only
// honest answer for the full database: holding and turning the knob browses
// the whole database rather than the part in range, and never finishes.
//
// What this cannot do is be complete. It sees what comes round while it is
// looking, so it stops once nothing new has appeared for a while, and says so.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, left scanning throughout
//   - budget: the longest to watch for before giving up
//
// Returns:
//   - the channels, systems and departments that came round while it watched
//   - endWatched, since watching can never be complete
//   - error if the run was cancelled or the screen could not be read
func observe(ctx context.Context, client *device.Scanner, budget time.Duration) ([]entry, string, error) {
	seen := make(map[string]bool)
	var found []entry

	deadline := time.Now().Add(budget)
	quiet := time.Now()

	for time.Now().Before(deadline) && time.Since(quiet) < watchQuiet {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}

		e, ok, err := read(ctx, client)
		if err != nil {
			return nil, "", err
		}

		// Mid-cycle the channel is not settled, but the system and department
		// are, and those are what this can report.
		if !ok {
			if e, ok = partial(ctx, client); !ok {
				continue
			}
		}

		if !seen[e.key()] {
			seen[e.key()] = true
			found = append(found, e)
			quiet = time.Now()
		}
	}
	return found, endWatched, nil
}

// partial reads the system and department while the channel is still cycling.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - the entry holding only the source, system and department
//   - true if the screen named a system at all, false when there is nothing
//     worth reporting or the screen could not be read
func partial(ctx context.Context, client *device.Scanner) (entry, bool) {
	display, err := client.Display(ctx)
	if err != nil || len(display.Lines) <= departmentLine {
		return entry{}, false
	}

	e := entry{
		Source:     strings.TrimSpace(display.Lines[sourceLine].Text),
		System:     strings.TrimSpace(display.Lines[systemLine].Text),
		Department: strings.TrimSpace(display.Lines[departmentLine].Text),
	}
	if e.System == "" {
		return entry{}, false
	}
	return e, true
}

// read takes one channel off the screen, and reports whether the screen was
// showing one at all.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - the channel the scanner is holding on, with its system and department
//   - true if the screen was showing a channel, false while it is cycling or
//     between systems
//   - error if the screen could not be read
func read(ctx context.Context, client *device.Scanner) (entry, bool, error) {
	display, err := client.Display(ctx)
	if err != nil {
		return entry{}, false, fmt.Errorf("reading the scanner's screen: %w", err)
	}
	if len(display.Lines) <= valueLine {
		return entry{}, false, nil
	}

	// The scanner marks the channel it is holding on. A screen showing
	// anything else, mid-cycle or between systems, is skipped rather than
	// read, because the lines around it belong to whatever it moved on to.
	if !display.Lines[channelLine].Selected() {
		return entry{}, false, nil
	}

	channel := strings.TrimSpace(display.Lines[channelLine].Text)
	if channel == "" {
		return entry{}, false, nil
	}
	for _, c := range cycling {
		if channel == c {
			return entry{}, false, nil
		}
	}

	return entry{
		Source:     strings.TrimSpace(display.Lines[sourceLine].Text),
		System:     strings.TrimSpace(display.Lines[systemLine].Text),
		Department: strings.TrimSpace(display.Lines[departmentLine].Text),
		Channel:    channel,
		Value:      strings.TrimSpace(display.Lines[valueLine].Text),
	}, true, nil
}

// release lets the scanner carry on scanning.
//
// The key toggles, and the scanner redraws more slowly than it is pressed, so
// pressing again after a look at a stale screen switches the hold straight
// back on. Each press therefore waits for the screen to agree before another
// is considered.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - true once the scanner is scanning again
//   - error if the screen could not be read, a key press could not be sent, or
//     the scanner is still holding after every attempt
func release(ctx context.Context, client *device.Scanner) (bool, error) {
	for attempt := 0; attempt < holdAttempts; attempt++ {
		held, err := settledHold(ctx, client)
		if err != nil {
			return false, err
		}
		if !held {
			return true, nil
		}

		if err := client.PressKey(ctx, device.KeySoft3, device.KeyPress); err != nil {
			return false, fmt.Errorf("letting the scanner carry on: %w", err)
		}
	}
	return false, fmt.Errorf("the scanner is still holding on a channel: run \"radiocli scan\"")
}

// renderEntries writes the walk as an aligned table.
//
// Parameters:
//   - app: the application context the stream to write to comes from
//   - found: the channels to write, in the order they were walked
//
// Returns:
//   - error if the table could not be written to the output stream
func renderEntries(app *appcontext.App, found []entry) error {
	if len(found) == 0 {
		app.Notef("The scanner is not scanning anything.\n")
		return nil
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SYSTEM\tDEPARTMENT\tCHANNEL\tVALUE")
	for _, e := range found {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.System, e.Department, e.Channel, e.Value)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the channel list: %w", err)
	}
	return nil
}

// report says how the answer was arrived at, so a complete list and a sample
// are never mistaken for each other.
//
// Parameters:
//   - app: the application context the stream to write the note to comes from
//   - found: the channels the walk collected
//   - ending: how the walk finished, which decides what is said about it
func report(app *appcontext.App, found []entry, ending string) {
	switch ending {
	case endWrapped:
		// The walk came back to where it started, so this is everything.
	case endLimit:
		app.Notef("Stopped at %d channels, which is the limit rather than the end. "+
			"Pass --limit to see more.\n", len(found))
	case endStalled, endReleased:
		app.Notef("Stopped after %d channels. This is what was seen rather than everything "+
			"that is scanned.\n", len(found))
	case endWatched:
		app.Notef("Seen in %d places while the scanner cycled. The database holds more than "+
			"this: anything that had not come round yet is missing.\n", len(found))
	}
}

// reportSystems writes the systems out, and says so when the answer may be
// short of the whole set.
//
// Parameters:
//   - app: the application context the output format and the streams come from
//   - names: the systems to write, in the order they were found
//   - closed: whether the walk came back to where it started, which is what
//     makes the answer complete rather than a sample
//
// Returns:
//   - error if the systems could not be written to the output stream
func reportSystems(app *appcontext.App, names []string, closed bool) error {
	if app.Config.Output == appcontext.OutputJSON {
		if err := render.JSON(app.Stdout, names); err != nil {
			return err
		}
	} else {
		if len(names) == 0 {
			app.Notef("The scanner is not cycling through any systems.\n")
			return nil
		}
		for _, n := range names {
			app.Printf("%s\n", n)
		}
	}

	if !closed {
		app.Notef("The walk did not come back to where it started, so this may not be all "+
			"of them. Seen %d.\n", len(names))
	}
	return nil
}

// reset puts the scanner back to plain scanning, out of any menu and off any
// hold, so a walk always starts from the same place.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - error if the scanner could not be taken out of its menus, would not go
//     back to scanning, or the run was cancelled while it settled
func reset(ctx context.Context, client *device.Scanner) error {
	if _, err := client.MenuInfo(ctx); err == nil {
		if _, err := menus.Leave(ctx, client); err != nil {
			return err
		}
	}
	if err := client.JumpToMode(ctx, device.ScanModeScan, ""); err != nil {
		return fmt.Errorf("returning the scanner to scanning: %w", err)
	}

	// Give it a moment to settle onto a system before reading anything.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(resetSettle):
	}
	return nil
}

// run reports what the scanner is scanning.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output settings come from
//   - limit: how many channels to stop after, when walking a favorites list
//   - watch: how long to watch the scan cycle, when the full database is on
//
// Returns:
//   - error if the limit or the watch is not a usable number, the scanner could
//     not be reached, its lists could not be read, the walk failed, or the
//     channels could not be written
func run(ctx context.Context, app *appcontext.App, limit int, watch time.Duration) error {
	if limit < 1 {
		return fmt.Errorf("limit %d is not a number of channels: want 1 or more", limit)
	}
	if watch <= 0 {
		return fmt.Errorf("watch %s is not a length of time: want something above zero", watch)
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	lists, err := catalog.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}

	var scanned []string
	database := false
	for _, l := range lists {
		if !l.Monitored {
			continue
		}
		scanned = append(scanned, l.Name)
		if l.BuiltIn && strings.EqualFold(l.Name, fullDatabase) {
			database = true
		}
	}

	if len(scanned) == 0 {
		app.Notef("The scanner has nothing switched on to scan.\n" +
			"Run \"radiocli favorites\" to see the lists, and \"radiocli favorites scan\" to choose one.\n")
		return nil
	}

	var found []entry
	var ending string
	if database {
		app.Notef("Watching the scan cycle for up to %s, because %q is switched on and cannot be walked.\n",
			watch, fullDatabase)
		found, ending, err = observe(ctx, client, watch)
	} else {
		found, ending, err = walkHeld(ctx, app, client, limit)
	}
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		if err := render.JSON(app.Stdout, found); err != nil {
			return err
		}
	} else if err := renderEntries(app, found); err != nil {
		return err
	}

	report(app, found, ending)
	return nil
}

// runSystems reports the systems being scanned.
//
// Which way it finds them matters more than it looks. Turning the knob is the
// only way into the full database, but it is a poor way to answer anything
// else: every click is a key press the scanner beeps at, half a second apart,
// against a screen that does not change when there is only one system to step
// between. Reading a favorites list from memory answers the same question in
// one request, without touching the scanner's controls at all.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output settings come from
//
// Returns:
//   - error if the scanner could not be reached, its lists could not be read,
//     the walk failed, or the systems could not be written
func runSystems(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	lists, err := catalog.ReadFavorites(ctx, client)
	if err != nil {
		return err
	}

	monitored, database := monitored(lists)
	if len(monitored) == 0 {
		app.Notef("The scanner has nothing switched on to scan.\n" +
			"Run \"radiocli favorites\" to see the lists, and \"radiocli favorites scan\" to choose one.\n")
		return nil
	}

	if !database {
		names, err := listedSystems(ctx, client, monitored)
		if err != nil {
			return err
		}
		return reportSystems(app, names, true)
	}

	app.Notef("Turning the knob to read the systems, because %q is switched on and cannot be "+
		"listed. This stops the scanner for a moment.\n", fullDatabase)

	names, closed, err := cycleSystems(ctx, client)

	// Put the scanner back whatever happened. A scan holding one system has
	// nowhere to step to, and the knob walks off the scanning screen into the
	// menus, so taking it off the system level is not enough on its own.
	client.PressKey(ctx, systemArmKey, device.KeyPress)
	if _, menuErr := client.MenuInfo(ctx); menuErr == nil {
		menus.Leave(ctx, client)
	}

	if err != nil {
		return err
	}
	return reportSystems(app, names, closed)
}

// settledHold reports whether the scanner is holding, giving the screen time
// to catch up with the last press before believing it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - true if every look found it still holding, false as soon as one did not
//   - error if the screen could not be read
func settledHold(ctx context.Context, client *device.Scanner) (bool, error) {
	for attempt := 0; attempt < settleReads; attempt++ {
		held, err := holding(ctx, client)
		if err != nil {
			return false, err
		}
		if !held {
			return false, nil
		}
	}
	return true, nil
}

// settledScan waits for the scanner to settle on a channel, and reports
// whether it did.
//
// A trunked system spends real time working out which talkgroup is on the air,
// and says "ID Scanning..." while it does. Pressing again during that window
// toggles the hold back off, so this waits before deciding the press failed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - true as soon as one look found it holding on a channel, false if none did
//   - error if the screen could not be read
func settledScan(ctx context.Context, client *device.Scanner) (bool, error) {
	for attempt := 0; attempt < settleReads; attempt++ {
		held, err := holding(ctx, client)
		if err != nil {
			return false, err
		}
		if held {
			return true, nil
		}
	}
	return false, nil
}

// stepSystem clicks the knob once and waits for the screen to show a different
// system than from.
//
// Waiting for the change is what makes this quick. The scanner usually redraws
// well inside stepCap, and there is nothing to be gained by sitting out the
// remainder: the sooner the screen has caught up, the sooner the next click can
// go. Turning faster is not the same as reading sooner, and it is turning
// faster that broke earlier attempts.
//
// A name equal to from after the full wait means the screen never changed. That
// is reported as it is, rather than as a failure, because the caller is the
// only thing that can tell a one-system scan from a lapsed system level.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, with the system level selected
//   - from: the system the walk is stepping away from
//
// Returns:
//   - the system the screen shows once it has caught up, which is from itself
//     when the screen never changed
//   - error if the knob could not be turned, the screen could not be read, or
//     the run was cancelled
func stepSystem(ctx context.Context, client *device.Scanner, from string) (string, error) {
	if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
		return "", fmt.Errorf("turning the knob: %w", err)
	}

	deadline := time.Now().Add(stepCap)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		name, err := systemName(ctx, client)
		if err != nil {
			return "", err
		}

		// An empty line means the knob has taken the scanner somewhere that has
		// no system on it at all, which the caller checks for. Waiting for that
		// to change would be waiting for something that is not coming.
		if name != from || name == "" {
			return name, nil
		}
		if !time.Now().Before(deadline) {
			return name, nil
		}
	}
}

// systemName reads the system the scanner is showing.
//
// The line carries the signal meter's own glyphs on the end, which are not
// text and are not part of the name, so anything outside printable ASCII is
// dropped rather than compared.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - the system the screen names, or nothing at all when the screen has no
//     such line
//   - error if the screen could not be read
func systemName(ctx context.Context, client *device.Scanner) (string, error) {
	return lineText(ctx, client, systemLine)
}

// walk steps through the channels, and reports whether it reached the end
// rather than the limit.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, held on a channel
//   - limit: how many channels to stop after
//
// Returns:
//   - the channels the walk collected, in the order they came up
//   - how the walk finished, which is what decides whether the answer is
//     everything or only what was seen
//   - error if the knob could not be turned or the screen could not be read
func walk(ctx context.Context, client *device.Scanner, limit int) ([]entry, string, error) {
	var found []entry
	seen := make(map[string]bool)

	first, ok, err := read(ctx, client)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, endWrapped, nil
	}
	found = append(found, first)
	seen[first.key()] = true

	current := first
	stale := 0
	for len(found) < limit {
		if current, ok, err = next(ctx, client, current); err != nil {
			return nil, "", err
		}
		if !ok {
			// The scanner lets go of the hold by itself during a long walk,
			// which is what made every earlier attempt cover a different
			// arbitrary slice of the database. Taking hold again and carrying
			// on covers far more of it; giving up only when it will not hold
			// at all.
			if err := hold(ctx, client); err != nil {
				return found, endReleased, nil
			}
			if current, ok, err = read(ctx, client); err != nil {
				return nil, "", err
			}
			if !ok {
				return found, endReleased, nil
			}
		}

		// Arriving back at the entry the walk began on closes the loop. This
		// is the only sound test: a favorites list is a short cycle and closes
		// quickly, and stopping at any previously seen entry instead cuts the
		// walk short, because the database lists the same channel under more
		// than one department and truncated names make still more look alike.
		if current.key() == first.key() {
			return found, endWrapped, nil
		}

		if seen[current.key()] {
			// A repeat that is not the starting point is a duplicate rather
			// than the end. Skip it and carry on, but give up if they are all
			// that is left, because the database does not always come back
			// round to where it started and the walk would otherwise run for
			// ever.
			stale++
			if stale >= staleSteps {
				return found, endStalled, nil
			}
			continue
		}

		stale = 0
		seen[current.key()] = true
		found = append(found, current)
	}
	return found, endLimit, nil
}

// walkHeld holds the scanner and turns the knob, which is exact for a
// favorites list because the walk comes back round to where it started.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the stream to write notes to comes from
//   - client: the connected scanner
//   - limit: how many channels to stop after
//
// Returns:
//   - the channels the walk collected, in the order they came up
//   - how the walk finished, which is what decides whether the answer is
//     everything or only what was seen
//   - error if the scanner would not hold on a channel, the knob could not be
//     turned, or the screen could not be read
func walkHeld(ctx context.Context, app *appcontext.App, client *device.Scanner, limit int) ([]entry, string, error) {
	if err := hold(ctx, client); err != nil {
		return nil, "", err
	}

	found, ending, err := walk(ctx, client, limit)

	// Put the scanner back whatever happened, so a failure does not leave it
	// held on one channel.
	if _, releaseErr := release(ctx, client); releaseErr != nil {
		app.Notef("The scanner was left holding on a channel: %v\n", releaseErr)
	}
	return found, ending, err
}
