// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package scan implements the "scan" command, which returns the scanner to
// its operating screen from wherever it has been left.
//
// Getting out of the menus is not one reliable command. The protocol's own
// "leave the menus" command is refused on some screens, and the key that
// always works is the same key that avoids the current channel when the
// scanner is not in a menu. So this checks where the scanner is before every
// press, and stops the moment it is out.
//
// Nor is leaving the menus the whole job. A scanner can be out of them and
// still not scanning: holding one frequency in quick search, as tune leaves it,
// holding one channel, as turning the knob leaves it, or sweeping a range of
// its own in a custom search, as "banks scan" leaves it. All three are handled
// here, because "returned to scanning" is what this command promises.
package scan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/spf13/cobra"
)

// New returns the scan command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "scan" command
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Leave the menus and return the scanner to scanning",
		Long: "Scan takes the scanner out of whatever menu it has been left in and returns it\n" +
			"to its operating screen.\n\n" +
			"It tries the protocol's own way out first, and falls back to the key that\n" +
			"leaves the menus when that is refused, which happens on several screens. It\n" +
			"checks where the scanner is before every press, because the same key avoids\n" +
			"the current channel when the scanner is not in a menu.\n\n" +
			"It also returns a scanner that is out of the menus but still not scanning:\n" +
			"one holding a frequency after \"radiocli tune\", one holding a channel after\n" +
			"someone turned its knob, and one sweeping a range of its own after\n" +
			"\"radiocli banks scan\". None of them looks like a menu and none says so\n" +
			"plainly on screen.\n\n" +
			"Running it on a scanner that is already scanning does nothing at all.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app)
		},
	}
}

// held names what the scanner is parked on, for the message.
//
// The scanner reports as much of the path as it has, so this uses the most
// specific part it gave and falls back to the mode's own name.
//
// Parameters:
//   - info: what the scanner says it is doing
//
// Returns:
//   - the system, department and channel it is parked on, joined into one
//     phrase, or a fallback naming the mode when it gave no names at all
func held(info device.ScannerInfo) string {
	var parts []string
	for _, name := range []string{info.System.Name, info.Department.Name, info.Channel.Name} {
		if name = strings.TrimSpace(name); name != "" {
			parts = append(parts, name)
		}
	}
	if len(parts) == 0 {
		return "one channel, in " + info.Mode
	}
	return strings.Join(parts, " / ")
}

// inMenu reports whether the scanner is showing a menu.
//
// Not being in one is a normal answer rather than a failure, which is what
// makes this the cheapest way to ask.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to ask
//
// Returns:
//   - true when the scanner is showing a menu
//   - error if the scanner could not be asked, which does not include its
//     answering that it is not in a menu
func inMenu(ctx context.Context, client *device.Scanner) (bool, error) {
	if _, err := client.MenuInfo(ctx); err != nil {
		if errors.Is(err, device.ErrNotInMenu) {
			return false, nil
		}
		return false, fmt.Errorf("asking the scanner where it is: %w", err)
	}
	return true, nil
}

// leaveMode brings the scanner back from a mode it was put in front of, and
// names the mode it left. An empty answer means it was already scanning and
// nothing was done.
//
// These modes are not menus and not holds, so nothing else in this command
// reaches them: "radiocli banks scan" leaves the scanner sweeping a custom
// search, and it sweeps it until something says otherwise. Saying otherwise is
// this command's whole job.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to return to scanning
//
// Returns:
//   - the mode it was brought back from, or the empty string when there was
//     nothing to do
//   - error if the scanner could not be asked what it is doing, could not be
//     told to scan, the context was cancelled, or it was still in the mode
//     after modePolls looks
func leaveMode(ctx context.Context, client *device.Scanner) (string, error) {
	info, err := client.ScannerInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("asking the scanner what it is doing: %w", err)
	}
	if info.Scanning() {
		return "", nil
	}

	// A scanner that reports no mode at all is nothing to act on. Jumping on the
	// strength of a blank would restart a scan that was running perfectly well,
	// which is the one thing this command promises not to do.
	mode := strings.TrimSpace(info.Mode)
	if mode == "" {
		return "", nil
	}

	if err := client.JumpToMode(ctx, device.ScanModeScan, ""); err != nil {
		return "", fmt.Errorf("returning the scanner to scanning from %s: %w", mode, err)
	}

	// The scanner reports the mode it is leaving until it has redrawn, so this
	// waits for the change rather than reading once and believing it.
	for poll := 0; poll < modePolls; poll++ {
		after, err := client.ScannerInfo(ctx)
		if err == nil && after.Scanning() {
			return mode, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(modeGap):
		}
	}

	return "", fmt.Errorf("the scanner is still in %s: press its scan key", mode)
}

// leaveWeather takes the scanner off the weather channels.
//
// The protocol's own jump does it in one command, and it releases the hold
// "weather" put the scanner into on the way, so there is nothing to undo
// first.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to take off the weather channels
//
// Returns:
//   - error if the scanner could not be told to scan, the context was
//     cancelled, or it was still on the weather channels after weatherPolls
//     looks
func leaveWeather(ctx context.Context, client *device.Scanner) error {
	if err := client.JumpToMode(ctx, device.ScanModeScan, ""); err != nil {
		return fmt.Errorf("returning the scanner to scanning from the weather channels: %w", err)
	}

	for poll := 0; poll < weatherPolls; poll++ {
		after, err := client.ScannerInfo(ctx)
		if err == nil && after.Weather.Mode == "" {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(weatherGap):
		}
	}

	return fmt.Errorf("the scanner is still on the weather channels: " +
		"press the key labelled \"to Scan\" on its screen")
}

// presses counts the key presses it took, for the message.
//
// Parameters:
//   - n: how many key presses were made
//
// Returns:
//   - the count as a phrase, in the singular when it was one
func presses(n int) string {
	if n == 1 {
		return "one key press"
	}
	return fmt.Sprintf("%d key presses", n)
}

// resume returns the scanner to scanning and says what it did, for the states
// that are not menus but are not scanning either.
//
// The work itself lives in the menus package, so that leaving the menus after
// any command ends the same way this command does. What is left here is the
// commentary, which is this command's job rather than that package's.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the commentary is written through
//   - client: the scanner to return to scanning
//
// Returns:
//   - error if the scanner could not be taken off the weather channels,
//     released from a hold, or brought back from a mode it was put in front of
func resume(ctx context.Context, app *appcontext.App, client *device.Scanner) error {
	// What the scanner is parked on makes a better sentence than the bare
	// description Resume hands back, and it has to be read before the hold is
	// released or there is nothing left to name.
	info, err := client.ScannerInfo(ctx)
	if err != nil {
		// The scanner is out of the menus, which was the main job. Failing to
		// ask a further question should not undo that.
		app.Notef("Could not check what the scanner is doing now: %v\n", err)
		return nil
	}
	// The weather channels are their own case. The shared machinery leaves a
	// scanner there alone, so that a command changing the volume does not end
	// the weather monitoring as a side effect, which means this command has to
	// say it means it.
	if info.Weather.Mode != "" {
		if err := leaveWeather(ctx, client); err != nil {
			return err
		}
		app.Notef("The scanner has left the weather channels and is scanning again.\n")
		return nil
	}

	parked := ""
	if info.Holding() {
		parked = held(info)
	}

	did, err := menus.Resume(ctx, client)
	if err != nil {
		return err
	}

	switch did {
	case "left quick search":
		app.Notef("The scanner has left quick search and is scanning again.\n")
	case "released the hold":
		if parked != "" {
			app.Notef("The scanner is holding %s.\n", parked)
		}
		app.Notef("The scanner has been released and is scanning again.\n")
	}

	// A scanner sweeping a custom search is out of the menus, is not holding,
	// and is not scanning either. Releasing a hold can also land in one, so this
	// is asked last, once nothing else is going to move the scanner again.
	left, err := leaveMode(ctx, client)
	if err != nil {
		return err
	}
	if left != "" {
		app.Notef("The scanner has left %s and is scanning again.\n", left)
	}
	return nil
}

// run returns the scanner to its operating screen.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the commentary are taken
//     from
//
// Returns:
//   - error if the scanner cannot be opened, where it is cannot be read, the
//     menus cannot be left, or it cannot be returned to scanning
func run(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	open, err := inMenu(ctx, client)
	if err != nil {
		return err
	}
	if !open {
		// Being out of the menus is not the same as scanning: tuning leaves the
		// scanner holding a frequency, which is neither.
		app.Notef("The scanner is already out of the menus.\n")
		return resume(ctx, app, client)
	}

	// The protocol's own way out, which is refused on some screens.
	graceful := client.CloseMenu(ctx)

	// The pressing is shared with the commands that edit a name, which have to
	// get the scanner back out afterwards for the same reasons.
	pressed, err := menus.Leave(ctx, client)
	if err != nil {
		return err
	}

	if pressed == 0 && graceful == nil {
		app.Notef("The scanner has left the menus.\n")
	} else {
		app.Notef("The scanner has left the menus, after %s.\n", presses(pressed))
	}

	return resume(ctx, app, client)
}
