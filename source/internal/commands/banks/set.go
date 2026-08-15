// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package banks

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/textinput"
	"github.com/spf13/cobra"
)

// newSet returns the "banks set" subcommand.
//
// Parameters:
//   - app: the application context the subcommand reads the scanner and the
//     streams to write to from
//
// Returns:
//   - the "set" subcommand, with a flag for every setting a bank holds
func newSet(app *appcontext.App) *cobra.Command {
	var (
		rangeFlag   string
		name        string
		modulation  string
		step        string
		attenuator  string
		delay       string
		digitalWait string
		avoid       string
		holdTime    string
	)

	cmd := &cobra.Command{
		Use:   "set <bank> [flags]",
		Short: "Change one custom search bank",
		Long: "Set changes one of the ten custom search banks. Only the settings named are\n" +
			"touched; everything else in the bank is left as it was.\n\n" +
			"The range is one value rather than two on purpose. The scanner checks the\n" +
			"limits against each other, so a lower limit above the upper one it already\n" +
			"holds is refused even when the pair you are asking for makes perfect sense.\n" +
			"Given both at once, this writes them in whichever order the scanner accepts.\n\n" +
			"--avoid and --hold-time are about ordinary scanning rather than about a custom\n" +
			"search: they say whether the scanner sweeps this bank as part of scanning the\n" +
			"favorites lists, and how long it spends on it each time round. That is a\n" +
			"different question from \"radiocli banks scan\", which chooses the banks a\n" +
			"custom search sweeps.\n\n" +
			"Every setting is entered by working the scanner's own menus, so this takes a\n" +
			"second or two per setting. It stops the scanner scanning, and returns it when\n" +
			"it is done.",
		Example: "  radiocli banks set 9 --range 26.965-27.405 --name CB --modulation AM --step 10k\n" +
			"  radiocli banks set 9 --name \"CB Radio\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSet(cmd.Context(), app, args[0], settings{
				rangeFlag:   rangeFlag,
				name:        name,
				modulation:  modulation,
				step:        step,
				attenuator:  attenuator,
				delay:       delay,
				digitalWait: digitalWait,
				avoid:       avoid,
				holdTime:    holdTime,
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&rangeFlag, "range", "", "frequencies to sweep, as <lower>-<upper> in MHz, such as 26.965-27.405")
	f.StringVar(&name, "name", "", "what the bank is called")
	f.StringVar(&modulation, "modulation", "", "auto, am, nfm, fm, wfm or fmb")
	f.StringVar(&step, "step", "", "how far apart the frequencies are, such as 10k, or auto")
	f.StringVar(&attenuator, "attenuator", "", "on or off")
	f.StringVar(&delay, "delay", "", "how long to wait on a transmission before moving on")
	f.StringVar(&digitalWait, "digital-wait", "", "how long to wait for a digital signal to resolve")
	f.StringVar(&avoid, "avoid", "",
		"whether ordinary scanning sweeps this bank: off, temporary or permanent")
	f.StringVar(&holdTime, "hold-time", "",
		"seconds ordinary scanning spends on this bank each time round")

	return cmd
}

// any reports whether anything was asked for at all.
//
// Returns:
//   - true if at least one setting was named, false if every field is empty
func (s settings) any() bool {
	return s != settings{}
}

// asNumber reads a value written with the scanner's units into a plain number,
// so that two spellings of the same amount compare equal. It reports whether
// the text was a number at all.
//
// Parameters:
//   - text: the value to read, already lowercased and stripped of spaces, such
//     as "10.0khz", "10k" or "3s"
//
// Returns:
//   - the amount in the unit the suffix implies, so "10k" and "10000" both
//     come back as 10000
//   - true if the text was a number, false if it was something like "Auto"
func asNumber(text string) (float64, bool) {
	text = strings.TrimSuffix(text, "hz")

	scale := 1.0
	switch {
	case strings.HasSuffix(text, "k"):
		text, scale = strings.TrimSuffix(text, "k"), 1000
	case strings.HasSuffix(text, "m"):
		text, scale = strings.TrimSuffix(text, "m"), 1000000
	case strings.HasSuffix(text, "s"):
		text = strings.TrimSuffix(text, "s")
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return value * scale, true
}

// closest finds the option somebody meant.
//
// The scanner writes these with spacing and units of its own, "10.0 kHz" and
// "3.125kHz" in the same list, so matching them as text means matching its
// punctuation exactly. Anything that looks like a number is therefore compared
// as one, which is what lets "10k" find "10.0 kHz" and "10000" find it too.
// Everything else, "Auto" and "On" and "Off", matches on the text alone.
//
// Parameters:
//   - want: the setting as it was typed on the command line
//   - options: the choices the scanner is offering, in its own spelling
//
// Returns:
//   - the option that matched, spelled the way the scanner spells it, which is
//     what selecting it needs
//   - error if nothing in options matched, naming what the choices were
func closest(want string, options []string) (string, error) {
	plain := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	}

	target := plain(want)
	for _, option := range options {
		if plain(option) == target {
			return option, nil
		}
	}

	if wanted, ok := asNumber(target); ok {
		for _, option := range options {
			if got, ok := asNumber(plain(option)); ok && got == wanted {
				return option, nil
			}
		}
	}
	return "", fmt.Errorf("%q is not one of the choices: %s", want, strings.Join(options, ", "))
}

// commitValue types a frequency into the screen the scanner is on and commits
// it. An empty value commits what is already there, which is how the walk moves
// from one limit to the next without changing the one it passes.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on a limit's entry screen
//   - value: the frequency to type as a bare number, or empty to leave the
//     screen as it is
//
// Returns:
//   - error if the value would not fit the screen, a key press could not be
//     sent, or the scanner would not commit
func commitValue(ctx context.Context, client *device.Scanner, value string) error {
	if value != "" {
		if err := typeFrequency(ctx, client, value); err != nil {
			return err
		}
	}
	return menus.Commit(ctx, client)
}

// digits renders a frequency as the bare number a keypad can type.
//
// Frequency.String writes it for a person to read, units and all, which is
// exactly what must not be sent to a screen that only takes number keys.
//
// Parameters:
//   - f: the frequency to renderBanks
//
// Returns:
//   - the frequency in megahertz, as digits and at most one point
func digits(f device.Frequency) string {
	return strconv.FormatFloat(f.MHz(), 'f', -1, 64)
}

// parseMHz reads a plain number of megahertz.
//
// Deliberately narrower than the one behind "tune", which also takes units.
// A search range is written in megahertz on the scanner's own screen, so that
// is what this takes, and there is nothing yet worth sharing a parser for.
//
// Parameters:
//   - text: the number as it was typed, such as "26.965"
//
// Returns:
//   - the frequency it names
//   - error if the text is not a number, or names nothing above zero
func parseMHz(text string) (device.Frequency, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%q is not a frequency in megahertz", text)
	}
	return device.Frequency(value * float64(device.Megahertz)), nil
}

// parseRange reads "<lower>-<upper>" into two frequencies.
//
// Parameters:
//   - text: the range as it was typed, such as "26.965-27.405"
//
// Returns:
//   - lower: the bottom of the range
//   - upper: the top of the range
//   - err: error if the text is not two numbers separated by a dash, either
//     number is not a frequency in megahertz, or the lower is not below the
//     upper
func parseRange(text string) (lower, upper device.Frequency, err error) {
	lo, hi, ok := strings.Cut(text, "-")
	if !ok {
		return 0, 0, fmt.Errorf("a range is written as <lower>-<upper> in MHz, such as 26.965-27.405, not %q", text)
	}

	if lower, err = parseMHz(lo); err != nil {
		return 0, 0, fmt.Errorf("the lower limit %q is not a frequency in MHz", strings.TrimSpace(lo))
	}
	if upper, err = parseMHz(hi); err != nil {
		return 0, 0, fmt.Errorf("the upper limit %q is not a frequency in MHz", strings.TrimSpace(hi))
	}
	if lower >= upper {
		return 0, 0, fmt.Errorf("the lower limit %s is not below the upper limit %s", lower, upper)
	}
	return lower, upper, nil
}

// runSet applies the changes to one bank.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output settings come from
//   - arg: the bank as it was typed on the command line
//   - want: the settings that were named, with an empty field for each one that
//     was not
//
// Returns:
//   - error if the argument is not a bank, nothing was asked for, the range is
//     written wrongly, the scanner could not be reached, any setting could not
//     be written, or the bank could not be read back afterwards
func runSet(ctx context.Context, app *appcontext.App, arg string, want settings) error {
	bank, err := bankNumber(arg)
	if err != nil {
		return err
	}
	if !want.any() {
		return fmt.Errorf("nothing to change: name at least one setting, such as --range or --name\n" +
			"Run \"radiocli banks set --help\" to see them")
	}

	// The range is parsed before the scanner is touched, so a range written
	// wrongly costs nothing and leaves it scanning.
	var lower, upper device.Frequency
	if want.rangeFlag != "" {
		if lower, upper, err = parseRange(want.rangeFlag); err != nil {
			return err
		}
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if err := open(ctx, client, bank); err != nil {
		return err
	}

	if want.rangeFlag != "" {
		if err := setLimits(ctx, client, lower, upper); err != nil {
			return err
		}
	}
	if want.name != "" {
		if err := setText(ctx, client, entryName, want.name); err != nil {
			return err
		}
	}

	for _, choice := range []struct {
		entry string
		value string
	}{
		{entryModulation, want.modulation},
		{entryStep, want.step},
		{entryAttenuator, want.attenuator},
		{entryDelay, want.delay},
		{entryDigitalWait, want.digitalWait},
	} {
		if choice.value == "" {
			continue
		}
		if err := setChoice(ctx, client, choice.entry, choice.value); err != nil {
			return err
		}
	}

	if want.avoid != "" || want.holdTime != "" {
		if err := setSearchWithScan(ctx, client, want.avoid, want.holdTime); err != nil {
			return err
		}
	}

	// Read back what the bank now holds, so what is reported is what is stored
	// rather than what was asked for. The scanner is already in the menu, so the
	// settings that need one are read before leaving, and the rest come from the
	// list afterwards.
	got := report{Bank: bank}
	if err := extras(ctx, client, bank, &got); err != nil {
		return err
	}
	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	fromList, err := listed(ctx, client)
	if err != nil {
		return err
	}
	if r, ok := fromList[bank]; ok {
		got.Name, got.Lower, got.Upper = r.Name, r.Lower, r.Upper
		got.Modulation, got.Step = r.Modulation, r.Step
	} else {
		// The scanner's list stops short of the last bank or two, so a bank it
		// left out is read back through the menus instead. Going back in costs
		// a second and is the difference between reporting what was stored and
		// reporting nothing.
		r, err := walked(ctx, client, bank)
		if err != nil {
			return err
		}
		got.Name, got.Lower, got.Upper = r.Name, r.Lower, r.Upper
		got.Modulation, got.Step = r.Modulation, r.Step

		if _, err := menus.Leave(ctx, client); err != nil {
			return err
		}
	}
	return renderBanks(app, []report{got}, true)
}

// setChoice picks one option out of a menu of them.
//
// The scanner refuses the protocol's own "set this value" on these menus, so
// the option is found by name and selected, the same way a person would.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the menu that holds the entry
//   - entry: the name of the entry to open, such as entryModulation
//   - value: the option to pick, in this command's spelling or the scanner's
//
// Returns:
//   - error if the entry could not be opened, its options could not be read,
//     the value is not one of them, or the option could not be selected
func setChoice(ctx context.Context, client *device.Scanner, entry, value string) error {
	if err := menus.Select(ctx, client, entry); err != nil {
		return fmt.Errorf("opening %q: %w", entry, err)
	}

	options, err := menus.Entries(ctx, client)
	if err != nil {
		return err
	}
	match, err := closest(value, options)
	if err != nil {
		cancel(ctx, client)
		return fmt.Errorf("setting %q: %w", entry, err)
	}
	return menus.Select(ctx, client, match)
}

// setHoldTime types a number of seconds into the Set Hold Time screen.
//
// It is a number rather than a choice, so the option walk the other settings
// use does not apply. The screen behaves the way the limit screens do: typing
// overwrites from the left rather than replacing what is there, so a screen
// showing 25 needs "07" to mean 7 and answers "75" to a bare 7. Typing more
// digits than are shown extends the field instead, so a longer number goes in
// as it is.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the Search with Scan submenu
//   - value: the number of seconds as it was typed on the command line
//
// Returns:
//   - error if the value is not a whole number of seconds that is not negative,
//     the entry could not be opened, the screen could not be read, or a key
//     press could not be sent
func setHoldTime(ctx context.Context, client *device.Scanner, value string) error {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 0 {
		return fmt.Errorf("%q is not a number of seconds", value)
	}
	typed := strconv.Itoa(seconds)

	if err := menus.Select(ctx, client, entryHoldTime); err != nil {
		return fmt.Errorf("opening %q: %w", entryHoldTime, err)
	}

	shown, err := currentValue(ctx, client)
	if err != nil {
		return err
	}
	for len(typed) < len(shown) {
		typed = "0" + typed
	}

	for _, key := range typed {
		if err := client.PressKey(ctx, device.Key(string(key)), device.KeyPress); err != nil {
			return fmt.Errorf("setting %q to %d: %w", entryHoldTime, seconds, err)
		}
	}
	return menus.Commit(ctx, client)
}

// setLimits writes both ends of a bank's range.
//
// The scanner checks each limit against the other as it is entered, so a new
// lower limit above the stored upper one is refused even when the pair being
// asked for is fine. The upper is therefore raised first when the new range
// sits above the old one, and the lower written first when it sits below.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the bank's own menu
//   - lower: the bottom of the new range
//   - upper: the top of the new range
//
// Returns:
//   - error if the entry could not be opened, or either limit could not be
//     typed or committed
func setLimits(ctx context.Context, client *device.Scanner, lower, upper device.Frequency) error {
	if err := menus.Select(ctx, client, entryLimits); err != nil {
		return fmt.Errorf("opening %q: %w", entryLimits, err)
	}

	current, err := currentValue(ctx, client)
	if err != nil {
		return err
	}
	stored, err := parseMHz(current)
	if err != nil {
		stored = 0
	}

	// The two screens come one after the other, so both are always walked
	// through: the lower first, then the upper. Which value goes in first is
	// decided by whether the new range is above or below the stored one, and
	// when it is above, the first pass leaves the lower alone and the second
	// writes it.
	if lower >= stored {
		if err := commitValue(ctx, client, ""); err != nil {
			return err
		}
		if err := commitValue(ctx, client, digits(upper)); err != nil {
			return fmt.Errorf("setting the upper limit to %s: %w", upper, err)
		}
		if err := menus.Select(ctx, client, entryLimits); err != nil {
			return err
		}
		if err := commitValue(ctx, client, digits(lower)); err != nil {
			return fmt.Errorf("setting the lower limit to %s: %w", lower, err)
		}
		return commitValue(ctx, client, "")
	}

	if err := commitValue(ctx, client, digits(lower)); err != nil {
		return fmt.Errorf("setting the lower limit to %s: %w", lower, err)
	}
	if err := commitValue(ctx, client, digits(upper)); err != nil {
		return fmt.Errorf("setting the upper limit to %s: %w", upper, err)
	}
	return nil
}

// setSearchWithScan writes the two settings behind the Search with Scan
// submenu, and comes back out to the bank's menu.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the bank's own menu
//   - avoid: whether ordinary scanning sweeps this bank, or empty to leave it
//   - hold: the seconds to spend on it each time round, or empty to leave it
//
// Returns:
//   - error if the submenu could not be opened, either setting could not be
//     written, or the scanner would not come back out
func setSearchWithScan(ctx context.Context, client *device.Scanner, avoid, hold string) error {
	if err := menus.Select(ctx, client, entrySearchWithScan); err != nil {
		return fmt.Errorf("opening %q: %w", entrySearchWithScan, err)
	}

	if avoid != "" {
		want := avoid
		if named, ok := avoidWords[strings.ToLower(strings.TrimSpace(avoid))]; ok {
			want = named
		}
		if err := setChoice(ctx, client, entryAvoid, want); err != nil {
			return err
		}
	}

	if hold != "" {
		if err := setHoldTime(ctx, client, hold); err != nil {
			return err
		}
	}

	return menus.Back(ctx, client)
}

// setText writes a name into one of the bank's entry screens.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on the bank's own menu
//   - entry: the name of the entry to open, such as entryName
//   - value: the text to write
//
// Returns:
//   - error if the entry could not be opened, or the text could not be entered
func setText(ctx context.Context, client *device.Scanner, entry, value string) error {
	if err := menus.Select(ctx, client, entry); err != nil {
		return fmt.Errorf("opening %q: %w", entry, err)
	}
	if err := textinput.Set(ctx, client, value); err != nil {
		return fmt.Errorf("setting %q to %q: %w", entry, value, err)
	}
	return nil
}

// typeFrequency enters a frequency into an entry screen.
//
// The screen holds a fixed layout of digits with the decimal point in a fixed
// place, and typing overwrites it from the left rather than adding to the end.
// The decimal key does not enter anything: it moves the cursor to the first
// digit after the point. So the whole numbers are typed, then the point, then
// the fraction, and any positions not typed keep whatever was under them.
//
// This is why textinput cannot be used here. That refuses a screen which
// already holds a value, on the grounds that there is no way to clear one, and
// these screens always hold one.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the connected scanner, sitting on a limit's entry screen
//   - value: the frequency to type as a bare number of megahertz
//
// Returns:
//   - error if the screen could not be read, the value has more digits before
//     the point than the screen holds, or a key press could not be sent
func typeFrequency(ctx context.Context, client *device.Scanner, value string) error {
	whole, fraction, _ := strings.Cut(value, ".")

	// The stored value decides how many digits sit before the point, and a
	// shorter number has to be padded to reach the same positions: a screen
	// showing 806.000000 needs 027 to mean 27, where one showing 30.000000
	// needs 27.
	info, err := client.MenuInfo(ctx)
	if err != nil {
		return fmt.Errorf("reading the entry screen: %w", err)
	}
	stored := strings.TrimSpace(info.Value)
	storedWhole, storedFraction, _ := strings.Cut(stored, ".")

	width := len(strings.TrimSpace(storedWhole))
	for len(whole) < width {
		whole = "0" + whole
	}
	if len(whole) > width {
		return fmt.Errorf("%s does not fit this screen, which holds %d digits before the point",
			value, width)
	}

	// The fraction is padded out to the width the screen holds, because a
	// position left untyped keeps the digit already under it rather than
	// clearing to zero. Typing 148 over a screen showing 469.999999 is what
	// stores 148.999999, which is a range nobody asked for.
	for len(fraction) < len(storedFraction) {
		fraction += "0"
	}

	// Then the trailing positions that already hold the wanted digit are
	// dropped again. Those cost a key press each and change nothing, which is
	// most of them: the frequencies worth typing end in zeroes and so do the
	// ones already there.
	for len(fraction) > 0 && len(fraction) <= len(storedFraction) &&
		fraction[len(fraction)-1] == storedFraction[len(fraction)-1] {
		fraction = fraction[:len(fraction)-1]
	}

	for _, key := range whole {
		if err := client.PressKey(ctx, device.Key(string(key)), device.KeyPress); err != nil {
			return err
		}
	}
	if fraction == "" {
		return nil
	}
	if err := client.PressKey(ctx, device.KeyNo, device.KeyPress); err != nil {
		return err
	}
	for _, key := range fraction {
		if err := client.PressKey(ctx, device.Key(string(key)), device.KeyPress); err != nil {
			return err
		}
	}
	return nil
}
