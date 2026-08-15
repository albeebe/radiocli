// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package banks

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/spf13/cobra"
)

// newScan returns the "banks scan" subcommand.
//
// Parameters:
//   - app: the application context the subcommand reads the scanner and the
//     streams to write to from
//
// Returns:
//   - the "scan" subcommand, which chooses the banks a custom search sweeps
func newScan(app *appcontext.App) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "scan [bank]...",
		Short: "Search only these custom search banks",
		Long: "Scan chooses which custom search banks the scanner sweeps, and starts it\n" +
			"sweeping them.\n\n" +
			"Naming banks searches exactly those and switches every other one off. That is\n" +
			"usually what is wanted: \"search only this\" rather than \"also search this\".\n\n" +
			"Unlike the favorites lists, a choice of banks only means anything in Custom\n" +
			"Search, so this puts the scanner into it. That is what makes naming one bank\n" +
			"the whole of listening to a band: there is nothing to run afterwards.\n\n" +
			"There is no way to choose none of them. The scanner refuses to be left in\n" +
			"Custom Search with nothing to sweep, so \"search nothing\" means leaving Custom\n" +
			"Search, which is what \"radiocli scan\" does. That is also how to go back to\n" +
			"scanning the favorites lists.",
		Example: "  radiocli banks scan 9\n  radiocli banks scan 0 1 2\n  radiocli banks scan --all",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.Context(), app, args, all)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "search every bank")
	return cmd
}

// enabled reports which banks the scanner is currently searching.
//
// Custom Search draws one character per bank along a row: the bank's own digit
// when it is being searched and a dash when it is not, so bank 6 alone reads
// "------6---". Reading that row is what makes this safe to run twice, because
// a digit key toggles its bank rather than switching it on.
//
// It is matched by position rather than by looking for digits anywhere, because
// the same line carries the volume and squelch, and "VOL: 1 SQL: 6" is full of
// digits that mean nothing here. The bank being swept at this instant is drawn
// inverse and arrives as a blank, so "------ ---" is bank 6 alone.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, already in Custom Search
//
// Returns:
//   - which banks are switched on, keyed by bank number, holding an entry only
//     for the ones being searched
//   - error if the screen could not be read, or if no row on it reads as the
//     row of banks
func enabled(ctx context.Context, client *device.Scanner) (map[int]bool, error) {
	screen, err := client.Screen(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading which banks are on: %w", err)
	}

	for _, line := range screen.Display.Lines {
		text := line.Text
		if len(text) < Count {
			continue
		}

		on := map[int]bool{}
		matched, marks := true, 0
		for bank := range Count {
			switch text[bank] {
			case '-':
				marks++
			case byte('0' + bank):
				on[bank] = true
				marks++
			case ' ':
				// The bank being swept right now is drawn inverse, which
				// arrives here as a blank rather than as its digit. It is on
				// by definition: the scanner is in it.
				on[bank] = true
			default:
				matched = false
			}
			if !matched {
				break
			}
		}
		// A row of nothing but blanks is an empty line rather than the banks.
		if matched && marks > 0 {
			return on, nil
		}
	}

	// Refusing rather than guessing. Pressing a toggle without knowing what it
	// is toggling switches off exactly the bank that was wanted, which is worse
	// than saying so.
	return nil, fmt.Errorf("could not tell which banks are switched on from the scanner's screen:\n%s",
		screen.Display)
}

// runScan enables the banks asked for and starts searching.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the streams to write to come
//     from
//   - args: the banks that were named on the command line
//   - all: whether every bank was asked for instead
//
// Returns:
//   - error if banks were named alongside --all or neither was given, if one of
//     the names is not a bank, if the scanner could not be reached or put into
//     Custom Search, if which banks are on could not be read, or if a bank could
//     not be switched
func runScan(ctx context.Context, app *appcontext.App, args []string, all bool) error {
	switch {
	case all && len(args) > 0:
		return fmt.Errorf("naming banks and passing --all ask for different things: choose one")
	case !all && len(args) == 0:
		return fmt.Errorf("name the banks to search, or pass --all\n" +
			"Run \"radiocli banks\" to see what they hold, " +
			"or \"radiocli scan\" to stop searching altogether")
	}

	// Resolve the numbers before touching the scanner, so a bank it does not
	// have costs nothing and leaves it scanning.
	wanted := map[int]bool{}
	for _, arg := range args {
		bank, err := bankNumber(arg)
		if err != nil {
			return err
		}
		wanted[bank] = true
	}
	if all {
		for bank := range Count {
			wanted[bank] = true
		}
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// Custom Search first, because the digit keys only choose banks once the
	// scanner is in it. Anywhere else they mean something different.
	if err := client.JumpToMode(ctx, device.ScanModeCustomSearch, ""); err != nil {
		return fmt.Errorf("starting a custom search: %w", err)
	}

	on, err := enabled(ctx, client)
	if err != nil {
		return err
	}

	// Each digit key toggles its own bank, so only the ones that differ are
	// pressed. Pressing them all would turn off exactly the ones wanted.
	//
	// The ones being switched on go first, and this matters. Switching off the
	// last bank that was on would leave the scanner with nothing to sweep, and
	// it refuses that rather than sitting silent, so the press is lost and the
	// bank stays on. Turning a wanted one on first means there is always
	// something left for the unwanted ones to be taken away from.
	toggle := func(want bool) error {
		for bank := range Count {
			if on[bank] == wanted[bank] || wanted[bank] != want {
				continue
			}
			if err := client.PressKey(ctx, device.Key(fmt.Sprint(bank)), device.KeyPress); err != nil {
				return fmt.Errorf("switching bank %d: %w", bank, err)
			}
		}
		return nil
	}
	if err := toggle(true); err != nil {
		return err
	}
	if err := toggle(false); err != nil {
		return err
	}

	chosen := make([]string, 0, len(wanted))
	for bank := range Count {
		if wanted[bank] {
			chosen = append(chosen, fmt.Sprint(bank))
		}
	}
	slices.Sort(chosen)

	// A note rather than a result, the same way "radiocli scan" reports itself.
	// There is nothing here to format, and writing it to stdout would put a line
	// that is not JSON in front of anything reading --output json.
	app.Notef("searching bank %s\n", strings.Join(chosen, ", "))
	return nil
}
