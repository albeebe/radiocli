// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package menus is the shared machinery for the scanner's menus: the names
// this tool accepts for them, reading the menu currently on screen, and
// rendering it.
//
// It exists so that any command able to put the scanner into a menu can report
// where it landed, without commands having to import one another.
package menus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
)

// Awaken waits for the scanner to start answering the protocol again.
//
// Several things send the scanner away to rebuild from its database: taking a
// new position, and switching the full database on or off. It stops answering
// entirely while it does, so a keypress is acted on but its acknowledgement
// never arrives, and anything sent afterwards times out too. Callers that have
// just done one of those things should wait here before pressing on.
//
// Answering is the test, not succeeding. A scanner sitting on a prompt refuses
// most commands outright, and a refusal is a reply: it proves the scanner is
// listening, which is all this is asking.
//
// The budget is a length of time, not a number of tries, because a rebuild
// takes as long as it takes however often it is asked. Asking twelve times used
// to be a rough way of spending forty seconds, which held only while every ask
// cost the protocol's full timeout; a scanner answering at once with something
// unusable spent the whole allowance in under a millisecond and then reported
// that it might still be rebuilding, having waited for nothing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to wait for
//
// Returns:
//   - error if the run is cancelled, or if the scanner has still not answered
//     within the budget
func Awaken(ctx context.Context, client *device.Scanner) error {
	deadline := time.Now().Add(AwakenBudget)

	for {
		_, err := client.Display(ctx)
		if err == nil || errors.Is(err, device.ErrRejected) {
			return nil
		}

		// A cancelled run is the user's own doing, and retrying through it
		// would ignore Ctrl-C for the length of the budget.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if !time.Now().Before(deadline) {
			return fmt.Errorf("the scanner stopped answering and has not come back: "+
				"it may still be rebuilding from its database: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(AwakenGap):
		}
	}
}

// Back climbs one level out of the menus, without leaving them.
//
// The menu key is used rather than the protocol's own command, because that
// command is refused on several of the screens this needs to climb out of.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - error if the scanner cannot be waited for, or if the key press fails
func Back(ctx context.Context, client *device.Scanner) error {
	if err := Settle(ctx, client); err != nil {
		return err
	}
	if err := client.PressKey(ctx, device.KeyMenu, device.KeyPress); err != nil {
		return fmt.Errorf("going back one level: %w", err)
	}
	return nil
}

// Commit presses enter where doing so makes the scanner rebuild, and waits for
// it to come back.
//
// The acknowledgement to that press never arrives, because the scanner leaves
// the protocol before sending it, so a timeout here means "working" rather
// than "failed". Treating it as a failure reports one for something that
// succeeded.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - error if the press fails for a reason other than a timeout, or if the
//     scanner does not come back
func Commit(ctx context.Context, client *device.Scanner) error {
	if err := Enter(ctx, client); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return Awaken(ctx, client)
}

// Confirm answers a prompt with yes.
//
// The prompt carries no menu of its own, so nothing about it can be read the
// way a menu is read; the scanner simply labels the keys on screen. Only call
// this having just pressed something that asks.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - error if the key press fails
func Confirm(ctx context.Context, client *device.Scanner) error {
	if err := client.PressKey(ctx, device.KeyEnter, device.KeyPress); err != nil {
		return fmt.Errorf("confirming: %w", err)
	}
	return nil
}

// ConfirmDelete answers the scanner's delete prompt with yes, having first
// checked that is really the question on screen.
//
// Reading before pressing is the point. Enter means something different on
// every other screen, so a walk that landed somewhere unintended would
// otherwise be answered with a keypress that does who knows what. If the
// prompt is not there, nothing is pressed and the caller is told what is on
// screen instead.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - error if the screen cannot be read, if the delete prompt is not the
//     question on screen, or if the key press fails
func ConfirmDelete(ctx context.Context, client *device.Scanner) error {
	d, err := client.Display(ctx)
	if err != nil {
		return fmt.Errorf("reading the screen before confirming: %w", err)
	}

	asking := false
	var shown []string
	for _, l := range d.Lines {
		text := strings.TrimSpace(l.Text)
		if text == "" {
			continue
		}
		shown = append(shown, text)
		if strings.EqualFold(text, deletePrompt) {
			asking = true
		}
	}

	if !asking {
		return fmt.Errorf("the scanner is not asking %q, it is showing %s: nothing was deleted",
			deletePrompt, strings.Join(quote(shown), ", "))
	}

	return Confirm(ctx, client)
}

// Enter selects the highlighted entry.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - error if the key press fails
func Enter(ctx context.Context, client *device.Scanner) error {
	if err := client.PressKey(ctx, device.KeyEnter, device.KeyPress); err != nil {
		return fmt.Errorf("selecting the highlighted entry: %w", err)
	}
	return nil
}

// Entries lists every entry of the menu the scanner is showing, by turning the
// knob all the way round and reading each one off the display.
//
// It is the only way to read a menu the protocol will not report. The entry
// list the scanner gives can omit entries that are really on screen, and some
// lists it declines to give at all, so this counts in the units the knob moves
// in and reads what is actually drawn.
//
// The scanner is left on the entry it started from, so a caller can carry on
// without having moved anything.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//
// Returns:
//   - []string holding every entry of the menu, in the order the knob reaches
//     them, starting from the one the scanner was on
//   - error if the display cannot be read, if the knob cannot be turned, or
//     if the menu does not come back round
func Entries(ctx context.Context, client *device.Scanner) ([]string, error) {
	first, err := Highlighted(ctx, client)
	if err != nil {
		return nil, err
	}

	seen := []string{first}
	for step := 0; step < maxSteps; step++ {
		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return nil, fmt.Errorf("turning the knob: %w", err)
		}

		on, err := Highlighted(ctx, client)
		if err != nil {
			return nil, err
		}
		seen = append(seen, on)

		// Back where it started, so the whole menu has been seen. The walk has
		// gone one entry past the start to prove it, so put the knob back on
		// the entry the caller left it on.
		if length, ok := wrapped(seen); ok {
			if err := client.PressKey(ctx, device.KeyRotateLeft, device.KeyPress); err != nil {
				return nil, fmt.Errorf("turning the knob back to where it started: %w", err)
			}
			return seen[:length], nil
		}
	}

	return nil, fmt.Errorf("the menu did not come back round after %d steps: it holds %s",
		maxSteps, strings.Join(quote(seen), ", "))
}

// Highlighted returns the text of the display row the scanner has selected.
//
// The display is used rather than the protocol's menu listing because the
// listing can omit entries that are really on screen and really in the knob's
// path. Anything stepping through a menu has to count in the same units the
// knob moves in, and those are screen rows.
//
// A screen part way through a redraw highlights nothing at all, which is a
// moment rather than a state, so this waits for the redraw rather than
// reporting it. Without that, a walk running faster than the scanner redraws
// fails on a screen that was about to be perfectly readable.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//
// Returns:
//   - string holding the selected row, with the padding and the scanner's own
//     markings taken off
//   - error if the display cannot be read, if the run is cancelled, or if
//     nothing is highlighted before the polls run out
func Highlighted(ctx context.Context, client *device.Scanner) (string, error) {
	r, err := highlighted(ctx, client)
	return r.text, err
}

// HighlightedRow reads the selected row along with whether it was cut short.
//
// Highlighted throws that away, which is safe for anything using StepTo and
// unsafe for anything comparing names itself: the difference between a name
// that ends there and a name that carries on is the only thing separating two
// entries whose first few characters are the same.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//
// Returns:
//   - Row holding the selected row and whether the scanner cut it short
//   - error if the display cannot be read, if the run is cancelled, or if
//     nothing is highlighted before the polls run out
func HighlightedRow(ctx context.Context, client *device.Scanner) (Row, error) {
	r, err := highlighted(ctx, client)
	return Row{Text: r.text, Cut: r.cut}, err
}

// Leave takes the scanner out of the menus and back to scanning, and reports
// how many key presses it took.
//
// It presses a key rather than sending the protocol's own command, because that
// command is refused on several screens, including the text entry ones that a
// command which has just edited something finds itself on.
//
// Getting out of the menus is not the whole job, which is why this finishes by
// calling Resume. A scanner that was tuned to a frequency or parked on a
// channel before the menus were opened goes back to sitting on it, so a command
// that has just changed what is scanned would leave the scanner listening to
// something else and looking like it had done nothing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - int counting the key presses it took to leave the menus
//   - error if the scanner cannot be taken out of the menus, or if returning
//     it to scanning fails
func Leave(ctx context.Context, client *device.Scanner) (int, error) {
	pressed, err := leaveMenus(ctx, client)
	if err != nil {
		return pressed, err
	}

	if _, err := Resume(ctx, client); err != nil {
		return pressed, err
	}
	return pressed, nil
}

// Lookup turns one of this tool's menu names into the scanner's.
//
// Parameters:
//   - name: the menu name this tool accepts, in any case
//
// Returns:
//   - device.MenuID naming the menu to the scanner
//   - bool reporting whether the name is one this tool accepts
func Lookup(name string) (device.MenuID, bool) {
	id, ok := byName[strings.ToLower(name)]
	return id, ok
}

// Names lists the accepted menu names in a stable order.
//
// Returns:
//   - []string holding every accepted menu name, sorted
func Names() []string {
	out := make([]string, 0, len(byName))
	for name := range byName {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Open puts the scanner into a menu and reports where it landed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - client: the scanner to drive
//   - id: which menu to open
//   - index: which system, department, site, channel or search bank the menu
//     opens on, empty for the menus that do not need one
//   - what: the menu as a person would name it, for the error message
//
// Returns:
//   - error if the menu cannot be opened, or if reporting where it landed fails
func Open(ctx context.Context, app *appcontext.App, client *device.Scanner, id device.MenuID, index, what string) error {
	if err := client.OpenMenu(ctx, id, index); err != nil {
		return fmt.Errorf("opening the %s menu: %w", what, err)
	}
	return Show(ctx, app, client)
}

// Settle waits for the scanner to finish what it is doing, so that the next key
// press is acted on rather than dropped.
//
// After saving something the scanner puts "Processing / Please Wait" on its
// screen for the best part of a second, and drops any key pressed during it.
// Throughout that time it still answers the protocol, so a command that only
// checks whether the scanner replies believes it is ready and presses into the
// void. What it does not do is name the menu it is on, so an empty title is the
// signal that it is busy.
//
// This waits for that rather than pacing keys slowly and hoping, so it costs
// only as long as the scanner actually needs.
//
// It is called Settle rather than Ready because it does not promise the scanner
// is ready. A scanner still busy once the polls run out is deliberately not an
// error, for the reason given where that is decided, and a name that reads as a
// state gave the returned nil a meaning it never carried.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to wait for
//
// Returns:
//   - error if the scanner cannot be asked where it is, or if the run is
//     cancelled. A scanner still busy once the polls run out is not an error
func Settle(ctx context.Context, client *device.Scanner) error {
	for poll := 0; poll < readyPolls; poll++ {
		info, err := client.MenuInfo(ctx)
		if err != nil {
			// Not being in a menu means the scanner is back to scanning, which
			// is as ready as it gets.
			if errors.Is(err, device.ErrNotInMenu) {
				return nil
			}
			return fmt.Errorf("waiting for the scanner: %w", err)
		}
		if info.Title != "" {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyGap):
		}
	}

	// Carrying on is better than failing: the caller checks what happened
	// anyway, and a scanner that is merely slower than expected should not turn
	// a working command into an error.
	return nil
}

// Resume returns the scanner to scanning from the states that are not menus but
// are not scanning either, and says what it did. An empty description means the
// scanner was already scanning and nothing was needed.
//
// Two states qualify. Tuning to a frequency puts the scanner into quick search,
// which no menu command reaches because it is not a menu: it answers that it is
// not in one, so every escape key is held back. The scanner offers a key to
// leave it, labelled "to Scan" on its screen, and this presses that. Turning the
// knob on a scanning screen parks it on one channel, and nothing about the
// screen says so plainly, because it names a channel just as it does while
// receiving one. Only the mode says which.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - string saying what was done, empty when the scanner was already
//     scanning and nothing was needed
//   - error if the scanner cannot be asked what it is doing, or if it will
//     not leave quick search or release its hold
func Resume(ctx context.Context, client *device.Scanner) (string, error) {
	info, err := client.ScannerInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("asking the scanner what it is doing: %w", err)
	}

	switch {
	// A scanner on the weather channels is left there. It reports itself as
	// holding once "weather" has parked it on the strongest channel, and the
	// mode reads "WX Hold", so without this every command that walks a menu
	// would tidy up by ending the weather monitoring somebody asked for: the
	// volume, the backlight and the clock would each quietly switch the radio
	// back to scanning. Leaving the weather channels is a thing to be asked
	// for, and "weather stop" and "scan" are what ask.
	case info.Weather.Mode != "":
		return "", nil

	case info.Screen == quickSearch:
		if err := leaveQuickSearch(ctx, client); err != nil {
			return "", err
		}
		return "left quick search", nil

	case info.Holding():
		if err := leaveHold(ctx, client, info); err != nil {
			return "", err
		}
		return "released the hold", nil
	}

	return "", nil
}

// Select moves to the entry named want and opens it.
//
// Stepping to an entry and pressing it is the shape of nearly every move
// through these menus, and doing it in one call keeps the two halves from
// drifting apart at a call site.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//   - want: the name of the entry to move to and open
//
// Returns:
//   - error if the entry cannot be reached, or if the key press fails
func Select(ctx context.Context, client *device.Scanner, want string) error {
	if err := StepTo(ctx, client, want); err != nil {
		return err
	}
	return Enter(ctx, client)
}

// Show reads the menu the scanner is on and renders it in whichever format was
// asked for.
//
// The scanner not being in a menu is an answer rather than a failure, so it is
// reported and nil returned.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context, for configuration and output
//   - client: the scanner to read
//
// Returns:
//   - error if the menu cannot be read, or if writing the report fails
func Show(ctx context.Context, app *appcontext.App, client *device.Scanner) error {
	info, err := client.MenuInfo(ctx)
	if err != nil {
		if errors.Is(err, device.ErrNotInMenu) {
			if app.Config.Output == appcontext.OutputJSON {
				return render.JSON(app.Stdout, nil)
			}
			app.Notef("The scanner is not in a menu.\n")
			return nil
		}
		return fmt.Errorf("reading the menu: %w", err)
	}

	// Which entry is highlighted is taken from the display, not from the
	// listing's own Selected field.
	//
	// Selected counts rows on screen, and the listing can leave out rows that
	// are really there, so the two drift apart by one for every row missing
	// above the cursor. Measured on a favorites list's menu, which omits
	// "Review Avoids": with the knob on Delete, the listing said Information.
	// A reader acting on that would press the wrong thing.
	on, err := highlighted(ctx, client)
	readable := err == nil

	// Started empty rather than left nil, because a text entry screen carries
	// no entries and "items" is documented as an array. A nil slice encodes as
	// null, which is not an array a script can range over.
	r := Report{Title: info.Title, Kind: info.Type, Items: []Item{}}
	marked := false
	for _, item := range info.Items {
		hit := readable && matches(on, item.Name)
		marked = marked || hit

		r.Items = append(r.Items, Item{
			Name:        item.Name,
			Index:       item.Index,
			Highlighted: hit,
		})
	}

	// The row the knob is on is not always in the listing, which is the same
	// omission read from the other end. Saying so beats marking nothing and
	// leaving the reader to wonder.
	if readable && !marked && on.text != "" {
		app.Notef("The scanner is on %q, which this listing does not include. "+
			"Use \"radiocli screen\" to see the rows as they are.\n", on.text)
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	return renderMenu(app, r)
}

// StepTo turns the knob until the entry named want is highlighted.
//
// It compares names rather than counting positions, so it cannot land one
// entry short or one entry past. That matters: on some menus the entry next to
// the one wanted is Delete.
//
// It stops when it has been all the way round without finding the name, and
// reports what it saw instead of guessing. The scanner is left wherever the
// walk reached, because backing out is a decision for the caller.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//   - want: the name of the entry to stop on
//
// Returns:
//   - error if the display cannot be read, if the knob cannot be turned, or
//     if the menu holds no entry by that name
func StepTo(ctx context.Context, client *device.Scanner, want string) error {
	var seen []string

	for step := 0; step < maxSteps; step++ {
		on, err := highlighted(ctx, client)
		if err != nil {
			return err
		}

		if matches(on, want) {
			return nil
		}
		seen = append(seen, on.text)

		// Arriving back where the walk started means the menu holds no such
		// entry. Checking the names rather than counting handles menus that
		// scroll, where the number of rows on screen is not the number of
		// entries. See wrapped for why it takes two names rather than one.
		if length, ok := wrapped(seen); ok {
			return fmt.Errorf("no entry called %q in this menu: it holds %s",
				want, strings.Join(quote(seen[:length]), ", "))
		}

		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return fmt.Errorf("turning the knob, after passing %s: %w", strings.Join(quote(seen), ", "), err)
		}
	}

	return fmt.Errorf("gave up looking for %q after %d steps: the menu holds %s",
		want, maxSteps, strings.Join(quote(seen), ", "))
}

// clean strips a display row down to the text it means.
//
// The scanner pads rows with spaces and marks a name it had to cut short with
// control bytes, so a truncated "Quick Save Favorites List" arrives as
// "Quick Save Favorites L" followed by two unprintable characters. Left in,
// those stop the row matching the name it is a truncation of.
//
// Parameters:
//   - text: one row of the display, as the scanner reports it
//
// Returns:
//   - string holding the printable characters of the row, trimmed
func clean(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// cut reports whether the scanner shortened a name to fit the screen.
//
// It marks one it had to cut with control bytes, which is the only difference
// between a name that ends there and a name that carries on. clean strips
// them, so this has to be asked before cleaning.
//
// Parameters:
//   - text: one row of the display, before cleaning has stripped anything
//
// Returns:
//   - bool reporting whether the row carries the marks of a shortened name
func cut(text string) bool {
	for _, r := range text {
		if r < 0x20 || r > 0x7e {
			return true
		}
	}
	return false
}

// dash renders a missing index, since a blank cell reads as a value that
// failed to arrive rather than one the scanner does not give.
//
// Parameters:
//   - value: the index to render, which may be empty
//
// Returns:
//   - string holding the value, or "-" when there is none
func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// highlighted reads the selected display row, and whether it was cut short.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//
// Returns:
//   - row holding the selected row and whether it was cut short
//   - error if the display cannot be read, if the run is cancelled, or if
//     nothing is highlighted before the polls run out
func highlighted(ctx context.Context, client *device.Scanner) (row, error) {
	for poll := 0; poll < readyPolls; poll++ {
		d, err := client.Display(ctx)
		if err != nil {
			return row{}, fmt.Errorf("reading the screen: %w", err)
		}

		for _, l := range d.Lines {
			if l.Selected() {
				return row{text: clean(l.Text), cut: cut(l.Text)}, nil
			}
		}

		select {
		case <-ctx.Done():
			return row{}, ctx.Err()
		case <-time.After(readyGap):
		}
	}

	return row{}, fmt.Errorf("the scanner highlighted nothing for %s: it may not be on a menu",
		time.Duration(readyPolls)*readyGap)
}

// leaveHold puts a held scanner back to scanning.
//
// Holding is not a screen the scanner can be pressed out of the way quick
// search is: the key that releases it depends on what is being held. The
// protocol's own jump to scanning mode does it in one command whatever the
// scanner is parked on.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//   - info: what the scanner is doing, for the mode named in the error
//
// Returns:
//   - error if the jump to scanning fails, if the run is cancelled, or if the
//     scanner is still holding once the polls run out
func leaveHold(ctx context.Context, client *device.Scanner, info device.ScannerInfo) error {
	if err := client.JumpToMode(ctx, device.ScanModeScan, ""); err != nil {
		return fmt.Errorf("returning the scanner to scanning from %s: %w", info.Mode, err)
	}

	for poll := 0; poll < resumePolls; poll++ {
		after, err := client.ScannerInfo(ctx)
		if err == nil && !after.Holding() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(resumeGap):
		}
	}

	return fmt.Errorf("the scanner is still holding, in %s: "+
		"press its scan key, or turn the knob, to release it", info.Mode)
}

// leaveMenus presses the scanner out of the menus and reports how many presses
// it took.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - int counting the key presses it took
//   - error if the scanner cannot be asked where it is, if a press fails, or
//     if it is still in the menus after the last attempt
func leaveMenus(ctx context.Context, client *device.Scanner) (int, error) {
	for attempt := 0; attempt < leaveAttempts; attempt++ {
		// Leaving usually follows something slow, such as saving a name, and a
		// press made while the scanner is still busy is thrown away.
		if err := Settle(ctx, client); err != nil {
			return attempt, err
		}

		if _, err := client.MenuInfo(ctx); err != nil {
			if errors.Is(err, device.ErrNotInMenu) {
				return attempt, nil
			}
			return attempt, fmt.Errorf("asking the scanner where it is: %w", err)
		}

		key := escapes[attempt%len(escapes)]
		if err := client.PressKey(ctx, key, device.KeyPress); err != nil {
			// A press that goes unacknowledged has usually still been acted
			// on: the scanner leaves the protocol to rebuild from its
			// database, which is exactly what switching the full database on
			// or setting a position does, and those are the things a command
			// tends to be leaving the menus after. Wait for it and look
			// again rather than calling the whole thing a failure.
			if !errors.Is(err, context.DeadlineExceeded) {
				return attempt, fmt.Errorf("leaving the menus: %w", err)
			}
			if err := Awaken(ctx, client); err != nil {
				return attempt, err
			}
		}
	}

	where := "somewhere it could not be taken out of"
	if info, err := client.MenuInfo(ctx); err == nil && info.Title != "" {
		where = fmt.Sprintf("the %q screen", strings.TrimSpace(info.Title))
	}
	return leaveAttempts, fmt.Errorf("could not leave the menus after %d attempts: the scanner is on %s, "+
		"and needs a key press on the scanner itself", leaveAttempts, where)
}

// leaveQuickSearch presses the key that leaves quick search, and waits for the
// scanner to agree it has.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to drive
//
// Returns:
//   - error if the key press fails, if the run is cancelled, or if the
//     scanner is still in quick search once the polls run out
func leaveQuickSearch(ctx context.Context, client *device.Scanner) error {
	if err := client.PressKey(ctx, device.KeySoft1, device.KeyPress); err != nil {
		return fmt.Errorf("leaving quick search: %w", err)
	}

	// The scanner reports the screen it is leaving until it has redrawn, so
	// this waits for the change rather than reading once and giving up.
	for poll := 0; poll < resumePolls; poll++ {
		after, err := client.ScannerInfo(ctx)
		if err == nil && after.Screen != quickSearch {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(resumeGap):
		}
	}

	return fmt.Errorf("the scanner is still holding one frequency in quick search: "+
		"press the key labelled %q on its screen", "to Scan")
}

// matches reports whether a highlighted row is the entry wanted.
//
// The display cuts a long name to the width of the screen, so "Quick Save
// Favorites List" shows as "Quick Save Favorites L". A row that was cut is
// therefore treated as the name it is the start of.
//
// A row that was not cut is the whole name, and anything else is a different
// entry. That distinction matters more than it looks: without it, "TEST CH"
// matches a walk looking for "TEST CH 2", and the walk stops on the wrong
// channel. Every command that steps by name is built on this, including the
// ones that delete.
//
// Parameters:
//   - shown: the highlighted row, as read from the display
//   - want: the name being looked for
//
// Returns:
//   - bool reporting whether the row is the entry wanted
func matches(shown row, want string) bool {
	if strings.EqualFold(shown.text, want) {
		return true
	}
	if !shown.cut || shown.text == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(want), strings.ToLower(shown.text))
}

// quote renders a list of entry names for a message.
//
// Parameters:
//   - names: the entry names to render
//
// Returns:
//   - []string holding each name in quotes
func quote(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("%q", n))
	}
	return out
}

// renderMenu writes the menu for a person.
//
// Parameters:
//   - app: the application context, for output
//   - r: the menu to write
//
// Returns:
//   - error if writing the table fails
func renderMenu(app *appcontext.App, r Report) error {
	app.Printf("menu: %s\n", r.Title)

	if len(r.Items) == 0 {
		// Input screens carry no entries at all. Their content is the value
		// being edited, which only the display shows.
		app.Notef("\nThis screen has no entries to choose from. If it is a text input, use\n" +
			"\"radiocli screen\" to see the value it holds.\n")
		return nil
	}

	app.Printf("\n")
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tINDEX\tENTRY")
	for _, item := range r.Items {
		marker := " "
		if item.Highlighted {
			marker = ">"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", marker, dash(item.Index), item.Name)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the menu: %w", err)
	}

	app.Notef("\n> currently highlighted. This list comes from the scanner and can omit\n" +
		"entries that are really on screen; \"radiocli screen\" shows what is.\n")
	return nil
}

// wrapped reports whether a walk has come back to where it started, and how
// many entries the menu holds if it has.
//
// A walk over a menu reads the same entries round and round, so the menu's
// length is the shortest run that repeats. The obvious test is the second
// reading of the first name, and it is wrong: the scanner's built-in menus are
// unique-named, but the same walks run over user-created favorites, systems and
// channel lists, where two entries may be called the same thing. A menu holding
// "A", "B", "A", "C" would be truncated to two entries, silently, and a step to
// "C" would fail with a report that the menu does not hold it.
//
// So a repeat has to be confirmed by the entry after it. Reading one entry past
// the candidate settles "A", "B", "A", "C" for the cost of a single extra turn
// of the knob, on the last lap only. What it cannot settle is a menu that
// genuinely repeats, such as "A", "B", "A", "B": no amount of reading
// distinguishes that from "A", "B", because the names are all there is to go on
// and they are identical either way.
//
// Parameters:
//   - seen: the names read so far, in the order the knob reached them, with no
//     entry left out
//
// Returns:
//   - how many entries the menu holds, meaningful only when the walk has wrapped
//   - whether the walk has come back round
func wrapped(seen []string) (int, bool) {
	// The candidate is the entry before the last, because the last is what
	// confirms it. Two readings are not enough to hold both.
	last := len(seen) - 1
	length := last - 1
	if length < 1 {
		return 0, false
	}

	if !strings.EqualFold(seen[length], seen[0]) {
		return 0, false
	}

	// The entry after the start, which for a menu of one is the start itself.
	if !strings.EqualFold(seen[last], seen[1%length]) {
		return 0, false
	}
	return length, true
}
