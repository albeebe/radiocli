// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package banks implements the "banks" command, which reads and changes the
// scanner's custom search banks.
//
// A bank is a frequency range the scanner sweeps, with its own modulation and
// step. There are exactly ten and they always exist, so unlike a favorites list
// there is nothing to create or delete: a bank is configured, and then searched.
//
// This is the way to listen to a range the database does not cover, such as the
// CB band, without building a favorites list of individual channels first.
package banks

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/spf13/cobra"
)

// New returns the banks command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner, the output
//     format and the streams to write to from
//
// Returns:
//   - the "banks" command, with the set, scan and goto subcommands already
//     attached to it
func New(app *appcontext.App) *cobra.Command {
	var full bool

	cmd := &cobra.Command{
		Use:   "banks",
		Short: "List the scanner's custom search banks",
		Long: "Banks lists the ten custom search banks and the frequency range each one\n" +
			"sweeps.\n\n" +
			"A bank is a range rather than a list of channels, which is what makes it the\n" +
			"way to listen to a band the database does not cover. There are always ten and\n" +
			"they cannot be added to or removed, so a bank is configured rather than\n" +
			"created: use \"radiocli banks set\" to change one and \"radiocli banks scan\"\n" +
			"to search it.\n\n" +
			"Most of a bank the scanner will report outright, so by default this costs one\n" +
			"command and leaves it scanning. The attenuator, the delay, the digital waiting\n" +
			"time and the two search with scan settings it will not, and --full reads those\n" +
			"by opening each bank's menu, which stops the scan for as long as it takes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), app, full)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false,
		"also read the settings that need the scanner's menus opened")

	cmd.AddCommand(newSet(app), newScan(app), newGoto(app))
	return cmd
}

// bankNumber reads a bank number from an argument.
//
// Parameters:
//   - arg: the bank as it was typed on the command line, such as "9"
//
// Returns:
//   - the bank number, from 0 to Count-1
//   - error if the argument is not a whole number naming one of the banks
func bankNumber(arg string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n < 0 || n >= Count {
		return 0, fmt.Errorf("no bank called %q: the banks are 0 to %d", arg, Count-1)
	}
	return n, nil
}

// cancel leaves a screen without writing what is on it.
//
// The menu key is what does this. The protocol's own "go back" is refused while
// the scanner is on an entry screen, which is why this presses a key rather
// than asking properly.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the screen being left
//
// Returns:
//   - error if the key press could not be sent to the scanner
func cancel(ctx context.Context, client *device.Scanner) error {
	return client.PressKey(ctx, device.KeyMenu, device.KeyPress)
}

// currentValue reads what an entry screen holds.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the entry screen
//
// Returns:
//   - the value the screen holds, with the spaces around it removed
//   - error if the screen could not be read
func currentValue(ctx context.Context, client *device.Scanner) (string, error) {
	info, err := client.MenuInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("reading the screen: %w", err)
	}
	return strings.TrimSpace(info.Value), nil
}

// extras fills in the settings the list leaves out, by opening the bank's menu
// and reading each one. The scanner is left in that menu.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//   - bank: the bank to read, from 0 to Count-1
//   - into: the report the attenuator, delay, digital waiting time and the two
//     search with scan settings are written into
//
// Returns:
//   - error if the bank's menu could not be opened, or any of the settings
//     could not be read
func extras(ctx context.Context, client *device.Scanner, bank int, into *report) error {
	if err := open(ctx, client, bank); err != nil {
		return err
	}

	for _, field := range []struct {
		entry string
		into  *string
	}{
		{entryAttenuator, &into.Attenuator},
		{entryDelay, &into.Delay},
		{entryDigitalWait, &into.DigitalWait},
	} {
		value, err := readChoice(ctx, client, field.entry)
		if err != nil {
			return err
		}
		*field.into = value
	}
	return readSearchWithScan(ctx, client, into)
}

// listed reads the banks the scanner will report outright, keyed by number.
//
// This is one command and costs milliseconds, where walking the menus for the
// same five fields costs seconds a bank and stops the scan to do it.
//
// It does not always answer for all ten. The scanner cuts the document at
// about a kilobyte, which with the names it ships is nine banks, and marks it
// as unfinished with EOT="0" in the footer. Nothing found asks for the rest:
// repeating the command answers with the same first part again, and a page
// number in the request is ignored. So this returns what came back and the
// caller reads whatever is missing the slow way. How many that is depends on
// how long the names are, which is why nothing here assumes it is one.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//
// Returns:
//   - the banks the scanner answered with, keyed by bank number from zero, and
//     holding only the fields the list carries
//   - error if the scanner's list could not be read
func listed(ctx context.Context, client *device.Scanner) (map[int]report, error) {
	banks, err := catalog.ReadCustomSearchBanks(ctx, client)
	if err != nil {
		return nil, err
	}

	found := make(map[int]report, len(banks))
	for _, b := range banks {
		// The scanner numbers these from one, where its menus, its screen and
		// this command all number them from zero.
		number, err := strconv.Atoi(strings.TrimSpace(b.Index))
		if err != nil || number < 1 || number > Count {
			continue
		}
		number--

		found[number] = report{
			Bank:       number,
			Name:       b.Name,
			Lower:      megahertz(b.Lower),
			Upper:      megahertz(b.Upper),
			Modulation: b.Modulation,
			Step:       stepText(b.Step),
		}
	}
	return found, nil
}

// megahertz strips the unit the scanner attaches to a frequency in a list.
//
// The limits are shown as bare numbers, because that is how the scanner's own
// limit screens write them and how "banks set --range" takes them. The unit is
// never anything but megahertz.
//
// Parameters:
//   - value: a frequency as the scanner's list writes it, unit and all
//
// Returns:
//   - the same frequency as a bare number
func megahertz(value string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "MHz"))
}

// open puts the scanner on one bank's menu.
//
// It opens the menu rather than going through menus.Open, which prints what it
// found: reading a bank walks through several menus and printing each of them
// would bury the answer.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//   - bank: the bank whose menu to open, from 0 to Count-1
//
// Returns:
//   - error if the scanner would not open the menu
func open(ctx context.Context, client *device.Scanner, bank int) error {
	if err := client.OpenMenu(ctx, device.MenuSearchRange, strconv.Itoa(bank)); err != nil {
		return fmt.Errorf("opening bank %d: %w", bank, err)
	}
	return nil
}

// readChoice opens a menu of options and reports the one that is selected.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the menu that holds the entry
//   - entry: the name of the entry to open, such as entryModulation
//
// Returns:
//   - the option the scanner has selected, with the spaces around it removed
//   - error if the entry could not be opened, nothing was highlighted, or the
//     screen could not be left
func readChoice(ctx context.Context, client *device.Scanner, entry string) (string, error) {
	if err := menus.Select(ctx, client, entry); err != nil {
		return "", fmt.Errorf("opening %q: %w", entry, err)
	}

	// The scanner opens one of these with the current value highlighted, so
	// what is highlighted is what it is set to.
	chosen, err := menus.Highlighted(ctx, client)
	if err != nil {
		return "", err
	}
	if err := cancel(ctx, client); err != nil {
		return "", err
	}
	return strings.TrimSpace(chosen), nil
}

// readLimits opens Edit Srch Limit and reads the two frequencies out of it.
//
// The two screens come one after the other: committing the lower moves to the
// upper, and committing the upper leaves. Reading therefore means walking
// through both rather than picking one.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the bank's own menu
//
// Returns:
//   - lower: the lower limit of the range, as a bare number of megahertz
//   - upper: the upper limit of the range, as a bare number of megahertz
//   - err: error if the entry could not be opened, or either screen could not
//     be read or committed
func readLimits(ctx context.Context, client *device.Scanner) (lower, upper string, err error) {
	if err = menus.Select(ctx, client, entryLimits); err != nil {
		return "", "", fmt.Errorf("opening %q: %w", entryLimits, err)
	}

	if lower, err = currentValue(ctx, client); err != nil {
		return "", "", err
	}
	if err = menus.Commit(ctx, client); err != nil {
		return "", "", err
	}
	if upper, err = currentValue(ctx, client); err != nil {
		return "", "", err
	}
	if err = menus.Commit(ctx, client); err != nil {
		return "", "", err
	}
	return lower, upper, nil
}

// readSearchWithScan reads the two settings behind the Search with Scan
// submenu, and comes back out to the bank's menu.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the bank's own menu
//   - into: the report the avoid setting and the hold time are written into
//
// Returns:
//   - error if the submenu could not be opened, either setting could not be
//     read, or the scanner would not come back out
func readSearchWithScan(ctx context.Context, client *device.Scanner, into *report) error {
	if err := menus.Select(ctx, client, entrySearchWithScan); err != nil {
		return fmt.Errorf("opening %q: %w", entrySearchWithScan, err)
	}

	avoid, err := readChoice(ctx, client, entryAvoid)
	if err != nil {
		return err
	}
	hold, err := readValue(ctx, client, entryHoldTime)
	if err != nil {
		return err
	}
	into.Avoid, into.HoldTime = avoid, hold

	// Back up to the bank's own menu. This one is a submenu rather than an
	// entry screen, so it is left the same way it was entered rather than
	// cancelled.
	return menus.Back(ctx, client)
}

// readValue opens an entry screen, reads it, and comes back without changing it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the menu that holds the entry
//   - entry: the name of the entry to open, such as entryName
//
// Returns:
//   - the value the entry screen held, with the spaces around it removed
//   - error if the entry could not be opened, the screen could not be read, or
//     it could not be left again
func readValue(ctx context.Context, client *device.Scanner, entry string) (string, error) {
	if err := menus.Select(ctx, client, entry); err != nil {
		return "", fmt.Errorf("opening %q: %w", entry, err)
	}
	value, err := currentValue(ctx, client)
	if err != nil {
		return "", err
	}

	// Backing out rather than committing, so reading a bank never writes to it.
	// The protocol's own back command is refused on an entry screen, so this is
	// the menu key, which is what cancels one.
	if err := cancel(ctx, client); err != nil {
		return "", err
	}
	return value, nil
}

// stepText writes a step the one way, whichever of the two places it came from.
//
// The scanner does not agree with itself about these. Its menus write "10.0
// kHz" and "3.125kHz", inconsistently spaced, and its list writes the same
// settings as a bare number of hertz. A bank read from the list would otherwise
// report "10000" beside a bank read from the menus reporting "10.0 kHz", which
// reads as two different settings.
//
// "Auto" is not a number and passes through untouched.
//
// Parameters:
//   - value: a step as either the menus or the list writes it
//
// Returns:
//   - the step written as a number of kilohertz, or the value with the spaces
//     around it removed when it is not a number at all
func stepText(value string) string {
	value = strings.TrimSpace(value)

	hertz, ok := asNumber(strings.ToLower(strings.ReplaceAll(value, " ", "")))
	if !ok || hertz <= 0 {
		return value
	}

	// One decimal place at least, so 10 kHz reads as a step rather than as a
	// count, and as many more as the value needs: the scanner has steps of
	// 3.125 kHz and 8.33 kHz.
	text := strconv.FormatFloat(hertz/1000, 'f', -1, 64)
	if !strings.Contains(text, ".") {
		text += ".0"
	}
	return text + " kHz"
}

// walked reads a bank by opening its menu, for the ones the list left out. The
// scanner is left in that menu.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner
//   - bank: the bank to read, from 0 to Count-1
//
// Returns:
//   - the bank, holding the same fields the scanner's list would have carried
//   - error if the menu could not be opened or any of the fields could not be
//     read, in which case the report holds only the bank number
func walked(ctx context.Context, client *device.Scanner, bank int) (report, error) {
	r := report{Bank: bank}

	if err := open(ctx, client, bank); err != nil {
		return r, err
	}

	lower, upper, err := readLimits(ctx, client)
	if err != nil {
		return r, err
	}
	r.Lower, r.Upper = lower, upper

	if r.Name, err = readValue(ctx, client, entryName); err != nil {
		return r, err
	}
	if r.Modulation, err = readChoice(ctx, client, entryModulation); err != nil {
		return r, err
	}
	step, err := readChoice(ctx, client, entryStep)
	if err != nil {
		return r, err
	}
	r.Step = stepText(step)
	return r, nil
}
