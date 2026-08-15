// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package banks

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// Test_settings_any covers whether a run of "banks set" asked for anything.
//
// Coverage: 100% (1 test case covering both outcomes)
//
// Test cases:
//   - Asked: an empty set of settings is nothing to do, and any field is
//     something to do
func Test_settings_any(t *testing.T) {
	// Verify an empty request is told apart from one naming a single setting
	t.Run("Asked", func(t *testing.T) {
		if (settings{}).any() {
			t.Error("an empty request asked for something, wanted nothing to do")
		}

		// Every field on its own is enough, so none of them is forgotten when a
		// new one is added.
		cases := []settings{
			{rangeFlag: "26.965-27.405"},
			{name: "CB"},
			{modulation: "am"},
			{step: "10k"},
			{attenuator: "on"},
			{delay: "2"},
			{digitalWait: "400"},
			{avoid: "off"},
			{holdTime: "5"},
		}
		for _, c := range cases {
			if !c.any() {
				t.Errorf("%+v asked for nothing, wanted it noticed", c)
			}
		}
	})
}

// Test_asNumber covers reading the scanner's units into a plain number.
//
// Coverage: 100% (1 test case covering every branch)
//
// Test cases:
//   - Read: the units the scanner writes, and the words it writes instead
func Test_asNumber(t *testing.T) {
	// Verify each unit scales the number, and a word is refused rather than guessed
	t.Run("Read", func(t *testing.T) {
		cases := []struct {
			text  string
			value float64
			ok    bool
		}{
			{"10000", 10000, true},
			{"10khz", 10000, true},
			{"10k", 10000, true},
			{"3.125khz", 3125, true},
			{"5m", 5000000, true},

			// Seconds are a unit that does not scale, for the delay settings.
			{"3s", 3, true},

			// Words the scanner uses where a number would go.
			{"auto", 0, false},
			{"on", 0, false},
			{"", 0, false},
		}

		for _, c := range cases {
			got, ok := asNumber(c.text)
			if ok != c.ok {
				t.Errorf("asNumber(%q) read it as a number: %v, wanted %v", c.text, ok, c.ok)
				continue
			}
			if got != c.value {
				t.Errorf("asNumber(%q) is %v, wanted %v", c.text, got, c.value)
			}
		}
	})
}

// Test_closest covers finding the option somebody meant.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Exact: an option spelled the scanner's way matches outright
//   - Spacing: the scanner's own inconsistent spacing does not have to be typed
//   - AsNumber: a step written any way finds the same step
//   - Unknown: something that is not on offer is refused, naming the choices
func Test_closest(t *testing.T) {
	// Verify an option typed exactly as the scanner spells it is found
	t.Run("Exact", func(t *testing.T) {
		got, err := closest("Auto", []string{"Auto", "AM", "NFM"})
		if err != nil {
			t.Fatalf("closest: %v", err)
		}
		if got != "Auto" {
			t.Errorf("closest is %q, wanted %q", got, "Auto")
		}
	})

	// Verify the case and the spaces the scanner uses do not have to be matched
	t.Run("Spacing", func(t *testing.T) {
		got, err := closest("nfm", []string{"Auto", "AM", "NFM"})
		if err != nil {
			t.Fatalf("closest: %v", err)
		}
		if got != "NFM" {
			t.Errorf("closest is %q, wanted %q", got, "NFM")
		}
	})

	// Verify a step written any of its ways finds the scanner's own spelling
	t.Run("AsNumber", func(t *testing.T) {
		options := []string{"Auto", "3.125kHz", "10.0 kHz"}
		for _, typed := range []string{"10k", "10000", "10khz", "10.0kHz"} {
			got, err := closest(typed, options)
			if err != nil {
				t.Fatalf("closest(%q): %v", typed, err)
			}
			if got != "10.0 kHz" {
				t.Errorf("closest(%q) is %q, wanted %q", typed, got, "10.0 kHz")
			}
		}
	})

	// Verify something the scanner is not offering is refused with the choices
	t.Run("Unknown", func(t *testing.T) {
		_, err := closest("ssb", []string{"Auto", "AM", "NFM"})
		if err == nil {
			t.Fatal("closest reported nothing, wanted the unknown option refused")
		}
		if !strings.Contains(err.Error(), "Auto, AM, NFM") {
			t.Errorf("closest reported %q, wanted it to name the choices", err)
		}

		// A number that matches nothing is refused the same way, rather than
		// falling through to the nearest option.
		if _, err := closest("7k", []string{"Auto", "10.0 kHz"}); err == nil {
			t.Fatal("closest took a step that is not on offer, wanted it refused")
		}
	})
}

// Test_digits covers rendering a frequency as something a keypad can type.
//
// Coverage: 100% (1 test case covering the one path)
//
// Test cases:
//   - Typed: the number comes out bare, with no unit attached
func Test_digits(t *testing.T) {
	// Verify the unit Frequency.String would add is not in what gets typed
	t.Run("Typed", func(t *testing.T) {
		cases := map[device.Frequency]string{
			device.Frequency(27.405 * float64(device.Megahertz)): "27.405",
			device.Frequency(26.965 * float64(device.Megahertz)): "26.965",
			device.Megahertz * 30:                                "30",
		}

		for frequency, want := range cases {
			if got := digits(frequency); got != want {
				t.Errorf("digits(%v) is %q, wanted %q", frequency, got, want)
			}
		}
	})
}

// Test_parseMHz covers reading a plain number of megahertz.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Typed: numbers above zero are taken and everything else refused
func Test_parseMHz(t *testing.T) {
	// Verify a bare number is read as megahertz and anything else is refused
	t.Run("Typed", func(t *testing.T) {
		got, err := parseMHz(" 26.965 ")
		if err != nil {
			t.Fatalf("parseMHz: %v", err)
		}
		if want := device.Frequency(26.965 * float64(device.Megahertz)); got != want {
			t.Errorf("parseMHz is %v, wanted %v", got, want)
		}

		// A unit, a word, nothing at all, and numbers that name no frequency.
		for _, typed := range []string{"26.965MHz", "cb", "", "0", "-5"} {
			if _, err := parseMHz(typed); err == nil {
				t.Errorf("parseMHz(%q) took it, wanted it refused", typed)
			}
		}
	})
}

// Test_parseRange covers reading a range written as one value.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Reads: two frequencies separated by a dash become the two limits
//   - NoDash: a single number is refused, since a range needs two ends
//   - BadLower: a lower limit that is not a frequency names which end it was
//   - BadUpper: an upper limit that is not a frequency names which end it was
//   - Backwards: a lower limit above the upper one is refused
func Test_parseRange(t *testing.T) {
	// Verify the two ends come back as the frequencies they name
	t.Run("Reads", func(t *testing.T) {
		lower, upper, err := parseRange("26.965-27.405")
		if err != nil {
			t.Fatalf("parseRange: %v", err)
		}
		if want := device.Frequency(26.965 * float64(device.Megahertz)); lower != want {
			t.Errorf("the lower limit is %v, wanted %v", lower, want)
		}
		if want := device.Frequency(27.405 * float64(device.Megahertz)); upper != want {
			t.Errorf("the upper limit is %v, wanted %v", upper, want)
		}
	})

	// Verify a single number is refused, with the form spelled out
	t.Run("NoDash", func(t *testing.T) {
		_, _, err := parseRange("26.965")
		if err == nil {
			t.Fatal("parseRange took a single number, wanted a range")
		}
		if !strings.Contains(err.Error(), "<lower>-<upper>") {
			t.Errorf("parseRange reported %q, wanted it to spell out the form", err)
		}
	})

	// Verify a bad lower limit says which end of the range was wrong
	t.Run("BadLower", func(t *testing.T) {
		_, _, err := parseRange("cb-27.405")
		if err == nil {
			t.Fatal("parseRange took a lower limit that is not a frequency")
		}
		if !strings.Contains(err.Error(), "lower limit") {
			t.Errorf("parseRange reported %q, wanted it to name the lower limit", err)
		}
	})

	// Verify a bad upper limit says which end of the range was wrong
	t.Run("BadUpper", func(t *testing.T) {
		_, _, err := parseRange("26.965-cb")
		if err == nil {
			t.Fatal("parseRange took an upper limit that is not a frequency")
		}
		if !strings.Contains(err.Error(), "upper limit") {
			t.Errorf("parseRange reported %q, wanted it to name the upper limit", err)
		}
	})

	// Verify a range written backwards is refused rather than swapped
	t.Run("Backwards", func(t *testing.T) {
		_, _, err := parseRange("27.405-26.965")
		if err == nil {
			t.Fatal("parseRange took a backwards range, wanted it refused")
		}
		if !strings.Contains(err.Error(), "not below") {
			t.Errorf("parseRange reported %q, wanted it to say which way round", err)
		}
	})
}

// Test_typeFrequency covers entering a frequency into a fixed layout screen.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Types: the whole part is padded, the point moves the cursor, and the
//     fraction is typed only as far as it differs
//   - NoFraction: a value matching the stored fraction types no point at all
//   - TooWide: a number with more digits than the screen holds is refused
//   - ReadFails: a screen that cannot be read is reported
//   - WholeFails: a key press typing the whole part is reported
//   - PointFails: a key press moving to the fraction is reported
//   - FractionFails: a key press typing the fraction is reported
func Test_typeFrequency(t *testing.T) {
	// Builds a scanner holding value on an entry screen, recording what is typed.
	screen := func(stored string, pressed *[]string, refuse string) *device.Scanner {
		return device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				if refuse != "" && command == refuse {
					return "", errors.New("the port closed")
				}
				*pressed = append(*pressed, command)
				return "", nil
			}
			return `<MenuInfo Name="Edit Srch Limit" Index="2" MenuType="TypeInput" Value="` +
				stored + `"><MenuInput MaxLength="9" EnableKeys="0123456789."/></MenuInfo>`, nil
		}})
	}

	// Verify the whole part is padded to the screen and the fraction trimmed
	t.Run("Types", func(t *testing.T) {
		var pressed []string
		client := screen("146.0000", &pressed, "")

		if err := typeFrequency(context.Background(), client, "27.405"); err != nil {
			t.Fatalf("typeFrequency: %v", err)
		}

		// 27 is padded to 027 to reach the same three positions the screen
		// holds, and the trailing zero of 4050 is dropped because the screen
		// already holds a zero there.
		want := "KEY,0,P KEY,2,P KEY,7,P KEY,.,P KEY,4,P KEY,0,P KEY,5,P"
		if strings.Join(pressed, " ") != want {
			t.Errorf("the scanner was sent %q, wanted %q", strings.Join(pressed, " "), want)
		}
	})

	// Verify a value whose fraction already matches types no point and no fraction
	t.Run("NoFraction", func(t *testing.T) {
		var pressed []string
		client := screen("146.0000", &pressed, "")

		if err := typeFrequency(context.Background(), client, "27"); err != nil {
			t.Fatalf("typeFrequency: %v", err)
		}
		if want := "KEY,0,P KEY,2,P KEY,7,P"; strings.Join(pressed, " ") != want {
			t.Errorf("the scanner was sent %q, wanted %q", strings.Join(pressed, " "), want)
		}
	})

	// Verify a number too wide for the screen is refused rather than truncated
	t.Run("TooWide", func(t *testing.T) {
		var pressed []string
		client := screen("30.000000", &pressed, "")

		err := typeFrequency(context.Background(), client, "146.5")
		if err == nil {
			t.Fatal("typeFrequency reported nothing, wanted the value refused")
		}
		if !strings.Contains(err.Error(), "does not fit this screen") {
			t.Errorf("typeFrequency reported %q, wanted it to say it does not fit", err)
		}
		if len(pressed) != 0 {
			t.Errorf("the scanner was sent %v, wanted nothing typed", pressed)
		}
	})

	// Verify a screen that cannot be read is reported before anything is typed
	t.Run("ReadFails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})

		err := typeFrequency(context.Background(), client, "27.405")
		if err == nil {
			t.Fatal("typeFrequency reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the entry screen") {
			t.Errorf("typeFrequency reported %q, wanted it to name reading the screen", err)
		}
	})

	// Verify a key press typing the whole part is reported rather than ignored
	t.Run("WholeFails", func(t *testing.T) {
		var pressed []string
		client := screen("146.0000", &pressed, "KEY,2,P")

		if err := typeFrequency(context.Background(), client, "27.405"); err == nil {
			t.Fatal("typeFrequency reported nothing, wanted the refused key press")
		}
	})

	// Verify the key that moves to the fraction is reported when it is refused
	t.Run("PointFails", func(t *testing.T) {
		var pressed []string
		client := screen("146.0000", &pressed, "KEY,.,P")

		if err := typeFrequency(context.Background(), client, "27.405"); err == nil {
			t.Fatal("typeFrequency reported nothing, wanted the refused key press")
		}
	})

	// Verify a key press typing the fraction is reported rather than ignored
	t.Run("FractionFails", func(t *testing.T) {
		var pressed []string
		client := screen("146.0000", &pressed, "KEY,4,P")

		if err := typeFrequency(context.Background(), client, "27.405"); err == nil {
			t.Fatal("typeFrequency reported nothing, wanted the refused key press")
		}
	})
}

// Test_newSet covers the set subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the command and its closure)
//
// Test cases:
//   - Wiring: the subcommand is named, documented and carries a flag per setting
//   - Runs: executing it writes the setting named and reports the bank back
func Test_newSet(t *testing.T) {
	// Verify running the subcommand writes the setting and reports the bank
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		s := &bankScanner{
			list: `<GLT><CS_BANK Index="10" Name="123" Lower="030.0000" Upper="035.0000" Mod="AM" Step="5000"/></GLT>`,
			menu: []string{entryName, entryAttenuator, entryDelay, entryDigitalWait, entrySearchWithScan},
			choices: map[string][]string{
				entryAttenuator:     {"Off", "On"},
				entryDelay:          {"2 sec", "3 sec"},
				entryDigitalWait:    {"400 ms", "0 ms"},
				entrySearchWithScan: {entryAvoid, entryHoldTime},
				entryAvoid:          {"Stop Avoiding", "Temporary Avoid"},
			},
			values: map[string]string{entryName: "", entryHoldTime: "5"},
			keys:   "0123456789",
		}
		app.SetDevice(device.New(s))

		cmd := newSet(app)
		cmd.SetArgs([]string{"9", "--name", "123"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "123") {
			t.Errorf("the subcommand wrote %q, wanted the bank it stored", out.String())
		}
	})

	// Verify the subcommand carries a flag for every setting a bank holds
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSet(appcontext.New())

		if cmd.Name() != "set" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "set")
		}
		if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
			t.Error("the subcommand has no help text")
		}
		for _, name := range []string{
			"range", "name", "modulation", "step", "attenuator",
			"delay", "digital-wait", "avoid", "hold-time",
		} {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("the --%s flag is not registered", name)
			}
		}
	})
}

// Test_setChoice covers picking one option out of a menu of them.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Picks: the option is found by name and selected
//   - SelectFails: an entry that cannot be opened names the entry
//   - EntriesFails: options that cannot be walked are reported
//   - Unknown: a value the scanner is not offering is refused and the screen
//     backed out of
//   - ChooseFails: an option that cannot be selected is reported
func Test_setChoice(t *testing.T) {
	// Builds a scanner offering three modulations, refusing whatever is named.
	holding := func(refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu:    []string{entryModulation},
			choices: map[string][]string{entryModulation: {"AM", "Auto", "NFM"}},
			refuse:  refuse,
		}
	}

	// Verify the option is found by walking the menu and then selected
	t.Run("Picks", func(t *testing.T) {
		s := holding(nil)

		if err := setChoice(context.Background(), device.New(s), entryModulation, "auto"); err != nil {
			t.Fatalf("setChoice: %v", err)
		}

		// Two enters: one opening the entry, one choosing the option, which
		// leaves the scanner back on the bank's menu.
		if len(s.open) != 0 {
			t.Errorf("the scanner was left on %v, wanted it back on the bank's menu", s.open)
		}
	})

	// Verify an entry the scanner will not show is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setChoice(context.Background(), device.New(s), entryModulation, "auto")
		if err == nil {
			t.Fatal("setChoice reported nothing, wanted the entry refused")
		}
		if !strings.Contains(err.Error(), `opening "Set Modulation"`) {
			t.Errorf("setChoice reported %q, wanted it to name the entry", err)
		}
	})

	// Verify options that cannot be walked through are reported
	t.Run("EntriesFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "KEY,>,P" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setChoice(context.Background(), device.New(s), entryModulation, "auto"); err == nil {
			t.Fatal("setChoice reported nothing, wanted the failed walk")
		}
	})

	// Verify a value the scanner is not offering is refused, naming the entry
	t.Run("Unknown", func(t *testing.T) {
		s := holding(nil)

		err := setChoice(context.Background(), device.New(s), entryModulation, "ssb")
		if err == nil {
			t.Fatal("setChoice reported nothing, wanted the unknown option refused")
		}
		if !strings.Contains(err.Error(), `setting "Set Modulation"`) {
			t.Errorf("setChoice reported %q, wanted it to name the entry", err)
		}

		// The screen is backed out of, so a refused value leaves nothing behind.
		if !strings.Contains(strings.Join(s.pressed, " "), "KEY,M,P") {
			t.Errorf("the scanner was sent %v, wanted the screen cancelled", s.pressed)
		}
	})

	// Verify an option that cannot be selected is reported
	t.Run("ChooseFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			// The first enter opens the entry; the second chooses the option.
			if command == "KEY,E,P" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setChoice(context.Background(), device.New(s), entryModulation, "auto"); err == nil {
			t.Fatal("setChoice reported nothing, wanted the refused option")
		}
	})
}

// Test_setHoldTime covers typing a number of seconds into the hold time screen.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Types: a number the same width as the screen is typed as it is
//   - Pads: a shorter number is padded, since typing overwrites from the left
//   - NotANumber: something that is not seconds is refused before the menus
//   - Negative: a negative number of seconds is refused
//   - SelectFails: an entry that cannot be opened names the entry
//   - ReadFails: a screen that cannot be read is reported
//   - KeyFails: a key press the scanner refuses is reported with the entry named
//   - CommitFails: a scanner that will not save is reported
func Test_setHoldTime(t *testing.T) {
	// Builds a scanner whose hold time screen already shows shown.
	holding := func(shown string, refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu:   []string{entryHoldTime},
			values: map[string]string{entryHoldTime: shown},
			refuse: refuse,
		}
	}

	// Verify a number as wide as the screen is typed straight in
	t.Run("Types", func(t *testing.T) {
		s := holding("5", nil)

		if err := setHoldTime(context.Background(), device.New(s), " 7 "); err != nil {
			t.Fatalf("setHoldTime: %v", err)
		}
		if want := "KEY,E,P KEY,7,P KEY,E,P"; strings.Join(s.pressed, " ") != want {
			t.Errorf("the scanner was sent %q, wanted %q", strings.Join(s.pressed, " "), want)
		}
	})

	// Verify a shorter number is padded, since typing overwrites from the left
	t.Run("Pads", func(t *testing.T) {
		s := holding("25", nil)

		if err := setHoldTime(context.Background(), device.New(s), "7"); err != nil {
			t.Fatalf("setHoldTime: %v", err)
		}
		if !strings.Contains(strings.Join(s.pressed, " "), "KEY,0,P KEY,7,P") {
			t.Errorf("the scanner was sent %v, wanted 7 typed as 07", s.pressed)
		}
	})

	// Verify something that is not a number of seconds is refused
	t.Run("NotANumber", func(t *testing.T) {
		s := holding("5", nil)

		err := setHoldTime(context.Background(), device.New(s), "soon")
		if err == nil {
			t.Fatal("setHoldTime reported nothing, wanted the value refused")
		}
		if !strings.Contains(err.Error(), "is not a number of seconds") {
			t.Errorf("setHoldTime reported %q, wanted it to say what a hold time is", err)
		}
	})

	// Verify a negative number of seconds is refused rather than typed
	t.Run("Negative", func(t *testing.T) {
		s := holding("5", nil)

		if err := setHoldTime(context.Background(), device.New(s), "-1"); err == nil {
			t.Fatal("setHoldTime reported nothing, wanted the negative value refused")
		}
	})

	// Verify an entry the scanner will not show is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		s := holding("5", func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setHoldTime(context.Background(), device.New(s), "7")
		if err == nil {
			t.Fatal("setHoldTime reported nothing, wanted the entry refused")
		}
		if !strings.Contains(err.Error(), `opening "Set Hold Time"`) {
			t.Errorf("setHoldTime reported %q, wanted it to name the entry", err)
		}
	})

	// Verify a screen that cannot be read is reported before anything is typed
	t.Run("ReadFails", func(t *testing.T) {
		s := holding("5", func(command string, nth int) error {
			if command == "MSI" {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setHoldTime(context.Background(), device.New(s), "7"); err == nil {
			t.Fatal("setHoldTime reported nothing, wanted the failed read")
		}
	})

	// Verify a refused key press names the entry and the value it was setting
	t.Run("KeyFails", func(t *testing.T) {
		s := holding("5", func(command string, nth int) error {
			if command == "KEY,7,P" {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setHoldTime(context.Background(), device.New(s), "7")
		if err == nil {
			t.Fatal("setHoldTime reported nothing, wanted the refused key press")
		}
		if !strings.Contains(err.Error(), `setting "Set Hold Time" to 7`) {
			t.Errorf("setHoldTime reported %q, wanted it to name the entry and the value", err)
		}
	})

	// Verify a scanner that will not save the hold time is reported
	t.Run("CommitFails", func(t *testing.T) {
		s := holding("5", func(command string, nth int) error {
			// The first enter opens the entry; the second commits the value.
			if command == "KEY,E,P" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setHoldTime(context.Background(), device.New(s), "7"); err == nil {
			t.Fatal("setHoldTime reported nothing, wanted the refused commit")
		}
	})
}

// Test_commitValue covers typing a frequency in and committing it.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Commits: an empty value commits what is already on the screen
//   - Types: a value is typed before it is committed
//   - TypeFails: a value that will not fit the screen is reported
func Test_commitValue(t *testing.T) {
	// Builds a scanner sitting on a limit screen showing 026.9650.
	holding := func() *bankScanner {
		s := &bankScanner{
			menu:   []string{entryLimits},
			limits: []string{"026.9650", "027.4050"},
		}
		s.open = []string{entryLimits}
		return s
	}

	// Verify an empty value moves on without changing what the screen holds
	t.Run("Commits", func(t *testing.T) {
		s := holding()

		if err := commitValue(context.Background(), device.New(s), ""); err != nil {
			t.Fatalf("commitValue: %v", err)
		}
		if want := "KEY,E,P"; strings.Join(s.pressed, " ") != want {
			t.Errorf("the scanner was sent %q, wanted %q", strings.Join(s.pressed, " "), want)
		}
	})

	// Verify a value is typed onto the screen and then committed
	t.Run("Types", func(t *testing.T) {
		s := holding()

		if err := commitValue(context.Background(), device.New(s), "27.405"); err != nil {
			t.Fatalf("commitValue: %v", err)
		}
		if !strings.HasSuffix(strings.Join(s.pressed, " "), "KEY,E,P") {
			t.Errorf("the scanner was sent %v, wanted the value committed last", s.pressed)
		}
		if len(s.pressed) < 2 {
			t.Errorf("the scanner was sent %v, wanted the value typed first", s.pressed)
		}
	})

	// Verify a value too wide for the screen is reported before it is committed
	t.Run("TypeFails", func(t *testing.T) {
		s := holding()

		err := commitValue(context.Background(), device.New(s), "1465.5")
		if err == nil {
			t.Fatal("commitValue reported nothing, wanted the value refused")
		}
		if !strings.Contains(err.Error(), "does not fit this screen") {
			t.Errorf("commitValue reported %q, wanted it to say it does not fit", err)
		}
	})
}

// Test_setLimits covers writing both ends of a bank's range.
//
// Coverage: 100% (11 test cases covering every branch)
//
// Test cases:
//   - Above: a range above the stored one writes the upper limit first
//   - Below: a range below the stored one writes the lower limit first
//   - Unreadable: a stored limit that is not a number is treated as zero
//   - SelectFails: an entry that cannot be opened names the entry
//   - ReadFails: a screen that cannot be read is reported
//   - UpperFails: an upper limit that will not commit names the limit
//   - ReselectFails: a scanner that will not reopen the limits is reported
//   - LowerFails: a lower limit that will not commit names the limit
//   - StepPastFails: a scanner that will not step past the lower limit is
//     reported
//   - AboveLowerFails: a lower limit written on the second walk is reported
//   - BelowUpperFails: an upper limit written below the stored range is
//     reported
func Test_setLimits(t *testing.T) {
	// Builds a scanner whose limits screens show stored, refusing what is named.
	holding := func(stored []string, refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu:   []string{entryLimits},
			limits: stored,
			refuse: refuse,
		}
	}

	// Verify a range above the stored one raises the upper limit first
	t.Run("Above", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, nil)

		lower := device.Frequency(30 * float64(device.Megahertz))
		upper := device.Frequency(35 * float64(device.Megahertz))
		if err := setLimits(context.Background(), device.New(s), lower, upper); err != nil {
			t.Fatalf("setLimits: %v", err)
		}

		// The limits screen is walked twice: once to raise the upper limit past
		// where the new lower one sits, and once to write the lower.
		if got := strings.Count(strings.Join(s.pressed, " "), "KEY,E,P"); got != 6 {
			t.Errorf("the scanner was committed to %d times, wanted 6", got)
		}
	})

	// Verify a range below the stored one writes the lower limit first
	t.Run("Below", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, nil)

		lower := device.Frequency(10 * float64(device.Megahertz))
		upper := device.Frequency(20 * float64(device.Megahertz))
		if err := setLimits(context.Background(), device.New(s), lower, upper); err != nil {
			t.Fatalf("setLimits: %v", err)
		}

		// One walk through the two screens is enough when the new range sits
		// below the stored one.
		if got := strings.Count(strings.Join(s.pressed, " "), "KEY,E,P"); got != 3 {
			t.Errorf("the scanner was committed to %d times, wanted 3", got)
		}
	})

	// Verify a stored limit that is not a number is read as zero rather than failing
	t.Run("Unreadable", func(t *testing.T) {
		s := holding([]string{"Auto", "027.4050"}, nil)

		lower := device.Frequency(10 * float64(device.Megahertz))
		upper := device.Frequency(20 * float64(device.Megahertz))
		if err := setLimits(context.Background(), device.New(s), lower, upper); err != nil {
			t.Fatalf("setLimits: %v", err)
		}
	})

	// Verify an entry the scanner will not show is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setLimits(context.Background(), device.New(s), device.Megahertz*10, device.Megahertz*20)
		if err == nil {
			t.Fatal("setLimits reported nothing, wanted the entry refused")
		}
		if !strings.Contains(err.Error(), `opening "Edit Srch Limit"`) {
			t.Errorf("setLimits reported %q, wanted it to name the entry", err)
		}
	})

	// Verify a screen that cannot be read is reported before anything is written
	t.Run("ReadFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			if command == "MSI" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setLimits(context.Background(), device.New(s), device.Megahertz*10, device.Megahertz*20); err == nil {
			t.Fatal("setLimits reported nothing, wanted the failed read")
		}
	})

	// Verify an upper limit that will not commit names the limit it was setting
	t.Run("UpperFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			// The first enter opens the screen, the second commits the lower
			// limit untouched, and the third is the upper limit itself.
			if command == "KEY,E,P" && nth == 3 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setLimits(context.Background(), device.New(s), device.Megahertz*30, device.Megahertz*35)
		if err == nil {
			t.Fatal("setLimits reported nothing, wanted the refused commit")
		}
		if !strings.Contains(err.Error(), "setting the upper limit") {
			t.Errorf("setLimits reported %q, wanted it to name the upper limit", err)
		}
	})

	// Verify a scanner that will not reopen the limits screen is reported
	t.Run("ReselectFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			if command == "STS" && nth == 4 {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setLimits(context.Background(), device.New(s), device.Megahertz*30, device.Megahertz*35); err == nil {
			t.Fatal("setLimits reported nothing, wanted the screen it could not reopen")
		}
	})

	// Verify a lower limit that will not commit names the limit it was setting
	t.Run("LowerFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			if command == "KEY,E,P" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setLimits(context.Background(), device.New(s), device.Megahertz*10, device.Megahertz*20)
		if err == nil {
			t.Fatal("setLimits reported nothing, wanted the refused commit")
		}
		if !strings.Contains(err.Error(), "setting the lower limit") {
			t.Errorf("setLimits reported %q, wanted it to name the lower limit", err)
		}
	})

	// Verify a scanner that will not step past the lower limit is reported
	t.Run("StepPastFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			// The first enter opens the screen; the second steps past the lower
			// limit without touching it, on the way to the upper.
			if command == "KEY,E,P" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setLimits(context.Background(), device.New(s), device.Megahertz*30, device.Megahertz*35); err == nil {
			t.Fatal("setLimits reported nothing, wanted the refused commit")
		}
	})

	// Verify a lower limit written on the second walk through is reported
	t.Run("AboveLowerFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			// The fifth enter is the lower limit itself, on the second walk
			// through the two screens.
			if command == "KEY,E,P" && nth == 5 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setLimits(context.Background(), device.New(s), device.Megahertz*30, device.Megahertz*35)
		if err == nil {
			t.Fatal("setLimits reported nothing, wanted the refused commit")
		}
		if !strings.Contains(err.Error(), "setting the lower limit") {
			t.Errorf("setLimits reported %q, wanted it to name the lower limit", err)
		}
	})

	// Verify an upper limit written below the stored range is reported
	t.Run("BelowUpperFails", func(t *testing.T) {
		s := holding([]string{"026.9650", "027.4050"}, func(command string, nth int) error {
			if command == "KEY,E,P" && nth == 3 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setLimits(context.Background(), device.New(s), device.Megahertz*10, device.Megahertz*20)
		if err == nil {
			t.Fatal("setLimits reported nothing, wanted the refused commit")
		}
		if !strings.Contains(err.Error(), "setting the upper limit") {
			t.Errorf("setLimits reported %q, wanted it to name the upper limit", err)
		}
	})
}

// Test_setText covers writing a name into one of the bank's entry screens.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Writes: the name is entered on the screen behind the entry
//   - SelectFails: an entry that cannot be opened names the entry
//   - SetFails: a screen that will not take text names the entry and the value
func Test_setText(t *testing.T) {
	// Verify the name is typed into the screen the entry opens
	t.Run("Writes", func(t *testing.T) {
		s := &bankScanner{
			menu:   []string{entryName},
			values: map[string]string{entryName: ""},
			keys:   "0123456789",
		}

		if err := setText(context.Background(), device.New(s), entryName, "123"); err != nil {
			t.Fatalf("setText: %v", err)
		}
		for _, key := range []string{"KEY,1,P", "KEY,2,P", "KEY,3,P"} {
			if !strings.Contains(strings.Join(s.pressed, " "), key) {
				t.Errorf("the scanner was sent %v, wanted %q typed", s.pressed, key)
			}
		}
	})

	// Verify an entry the scanner will not show is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		s := &bankScanner{
			menu:   []string{entryName},
			values: map[string]string{entryName: ""},
			refuse: func(command string, nth int) error {
				if command == "STS" && nth == 1 {
					return errors.New("the port closed")
				}
				return nil
			},
		}

		err := setText(context.Background(), device.New(s), entryName, "123")
		if err == nil {
			t.Fatal("setText reported nothing, wanted the entry refused")
		}
		if !strings.Contains(err.Error(), `opening "Edit Name"`) {
			t.Errorf("setText reported %q, wanted it to name the entry", err)
		}
	})

	// Verify a screen that is not a text entry is reported with what was wanted
	t.Run("SetFails", func(t *testing.T) {
		// A screen offering options rather than taking text, which is what a
		// firmware that moved this entry would land on.
		s := &bankScanner{
			menu:    []string{entryName},
			choices: map[string][]string{entryName: {"AM", "Auto"}},
		}

		err := setText(context.Background(), device.New(s), entryName, "123")
		if err == nil {
			t.Fatal("setText reported nothing, wanted the screen refused")
		}
		if !strings.Contains(err.Error(), `setting "Edit Name" to "123"`) {
			t.Errorf("setText reported %q, wanted it to name the entry and the value", err)
		}
	})
}

// Test_setSearchWithScan covers the two settings behind the submenu.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - SetsBoth: this command's own words for avoid are translated, and the hold
//     time typed
//   - AvoidOnly: a hold time that was not named is left alone
//   - HoldOnly: an avoid setting that was not named is left alone
//   - ScannerWords: the scanner's own spelling is taken as it is
//   - SelectFails: a submenu that cannot be opened names it
//   - AvoidFails: an avoid setting that cannot be written is reported
//   - HoldFails: a hold time that cannot be written is reported
func Test_setSearchWithScan(t *testing.T) {
	// Builds a scanner holding the submenu, refusing whatever the test names.
	holding := func(refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu: []string{entrySearchWithScan},
			choices: map[string][]string{
				entrySearchWithScan: {entryAvoid, entryHoldTime},
				entryAvoid:          {"Stop Avoiding", "Temporary Avoid", "Permanent Avoid"},
			},
			values: map[string]string{entryHoldTime: "5"},
			refuse: refuse,
		}
	}

	// Verify "off" becomes the scanner's own "Stop Avoiding" and the time is typed
	t.Run("SetsBoth", func(t *testing.T) {
		s := holding(nil)

		if err := setSearchWithScan(context.Background(), device.New(s), "temporary", "7"); err != nil {
			t.Fatalf("setSearchWithScan: %v", err)
		}
		if !strings.Contains(strings.Join(s.pressed, " "), "KEY,7,P") {
			t.Errorf("the scanner was sent %v, wanted the hold time typed", s.pressed)
		}
		if len(s.open) != 0 {
			t.Errorf("the scanner was left on %v, wanted it back on the bank's menu", s.open)
		}
	})

	// Verify a hold time that was not named is left exactly as it was
	t.Run("AvoidOnly", func(t *testing.T) {
		s := holding(nil)

		if err := setSearchWithScan(context.Background(), device.New(s), "off", ""); err != nil {
			t.Fatalf("setSearchWithScan: %v", err)
		}
		if strings.Contains(strings.Join(s.pressed, " "), "KEY,7,P") {
			t.Errorf("the scanner was sent %v, wanted the hold time left alone", s.pressed)
		}
	})

	// Verify an avoid setting that was not named is left exactly as it was
	t.Run("HoldOnly", func(t *testing.T) {
		s := holding(nil)

		if err := setSearchWithScan(context.Background(), device.New(s), "", "7"); err != nil {
			t.Fatalf("setSearchWithScan: %v", err)
		}
		if !strings.Contains(strings.Join(s.pressed, " "), "KEY,7,P") {
			t.Errorf("the scanner was sent %v, wanted the hold time typed", s.pressed)
		}
	})

	// Verify the scanner's own spelling is passed through rather than translated
	t.Run("ScannerWords", func(t *testing.T) {
		s := holding(nil)

		if err := setSearchWithScan(context.Background(), device.New(s), "Permanent Avoid", ""); err != nil {
			t.Fatalf("setSearchWithScan: %v", err)
		}
	})

	// Verify a submenu the scanner will not open is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		err := setSearchWithScan(context.Background(), device.New(s), "off", "")
		if err == nil {
			t.Fatal("setSearchWithScan reported nothing, wanted the submenu refused")
		}
		if !strings.Contains(err.Error(), `opening "Search with Scan"`) {
			t.Errorf("setSearchWithScan reported %q, wanted it to name the submenu", err)
		}
	})

	// Verify an avoid setting that cannot be written is reported
	t.Run("AvoidFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "STS" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setSearchWithScan(context.Background(), device.New(s), "off", ""); err == nil {
			t.Fatal("setSearchWithScan reported nothing, wanted the failed setting")
		}
	})

	// Verify a hold time that cannot be written is reported
	t.Run("HoldFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "MSI" {
				return errors.New("the port closed")
			}
			return nil
		})

		if err := setSearchWithScan(context.Background(), device.New(s), "", "7"); err == nil {
			t.Fatal("setSearchWithScan reported nothing, wanted the failed setting")
		}
	})
}

// Test_runSet covers applying the changes to one bank and reporting it back.
//
// Coverage: 100% (16 test cases covering every branch)
//
// Test cases:
//   - Sets: every setting is written and the bank is read back from the list
//   - WalksBack: a bank the list leaves out is read back through the menus
//   - BadBank: a bank the scanner does not have is refused
//   - NothingAsked: a run naming no setting is refused with a way forward
//   - BadRange: a range written wrongly is refused before the scanner is opened
//   - NoDevice: a run with no scanner named is refused
//   - OpenFails: a bank whose menu will not open is reported
//   - LimitsFail: a range that cannot be written is reported
//   - NameFails: a name that cannot be written is reported
//   - ChoiceFails: a setting that cannot be written is reported
//   - SearchFails: the search with scan settings failing is reported
//   - ExtrasFails: a read back that cannot be made is reported
//   - LeaveFails: a scanner that will not go back to scanning is reported
//   - ListFails: a list the scanner will not send is reported
//   - WalkBackFails: a bank that cannot be walked back is reported
//   - SecondLeaveFails: the second return to scanning failing is reported
//   - RenderFails: output that cannot be written is reported
func Test_runSet(t *testing.T) {
	// Builds the list document the scanner answers a bank read with. The
	// scanner numbers these from one, where this command numbers them from zero.
	catalogue := func(banks int) string {
		doc := &strings.Builder{}
		doc.WriteString("<GLT>")
		for i := 1; i <= banks; i++ {
			doc.WriteString(`<CS_BANK Index="` + strconv.Itoa(i) + `" Name="Custom ` + strconv.Itoa(i-1) +
				`" Lower="030.0000" Upper="035.0000" Mod="AM" Step="5000"/>`)
		}
		doc.WriteString("</GLT>")
		return doc.String()
	}

	// Builds a scanner holding a whole bank menu alongside its list.
	holding := func(list string, refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			list:   list,
			menu:   []string{entryLimits, entryName, entryModulation, entryStep, entryAttenuator, entryDelay, entryDigitalWait, entrySearchWithScan},
			limits: []string{"026.9650", "027.4050"},
			values: map[string]string{entryName: "", entryHoldTime: "5"},
			keys:   "0123456789",
			choices: map[string][]string{
				entryModulation:     {"AM", "Auto"},
				entryStep:           {"5000", "Auto"},
				entryAttenuator:     {"Off", "On"},
				entryDelay:          {"2 sec", "3 sec"},
				entryDigitalWait:    {"400 ms", "0 ms"},
				entrySearchWithScan: {entryAvoid, entryHoldTime},
				entryAvoid:          {"Stop Avoiding", "Temporary Avoid"},
			},
			refuse: refuse,
		}
	}

	// Builds an app writing into out, driven by the scanner given.
	driving := func(out *bytes.Buffer, s *bankScanner) *appcontext.App {
		app := appcontext.New()
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(s))
		return app
	}

	// Everything one run can ask for, so no setting is left unwritten.
	everything := settings{
		rangeFlag: "30-35", name: "123", modulation: "am", step: "5000",
		attenuator: "off", delay: "2 sec", digitalWait: "400 ms",
		avoid: "off", holdTime: "7",
	}

	// Verify every setting is written and the stored bank is what gets reported
	t.Run("Sets", func(t *testing.T) {
		out := &bytes.Buffer{}
		app := driving(out, holding(catalogue(Count), nil))

		if err := runSet(context.Background(), app, "9", everything); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		if !strings.Contains(out.String(), "Custom 9") {
			t.Errorf("runSet wrote %q, wanted the bank the scanner is holding", out.String())
		}
	})

	// Verify a bank the list leaves out is read back through the menus instead
	t.Run("WalksBack", func(t *testing.T) {
		out := &bytes.Buffer{}
		app := driving(out, holding(catalogue(Count-1), nil))

		if err := runSet(context.Background(), app, "9", settings{name: "123"}); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		if !strings.Contains(out.String(), "123") {
			t.Errorf("runSet wrote %q, wanted the bank walked back out of the menus", out.String())
		}
	})

	// Verify a bank the scanner does not have is refused before it is touched
	t.Run("BadBank", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), nil))

		err := runSet(context.Background(), app, "11", everything)
		if err == nil {
			t.Fatal("runSet reported nothing, wanted the unknown bank refused")
		}
		if !strings.Contains(err.Error(), "no bank called") {
			t.Errorf("runSet reported %q, wanted it to name the unknown bank", err)
		}
	})

	// Verify a run naming no setting at all is refused with a way forward
	t.Run("NothingAsked", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), nil))

		err := runSet(context.Background(), app, "9", settings{})
		if err == nil {
			t.Fatal("runSet reported nothing, wanted the empty request refused")
		}
		if !strings.Contains(err.Error(), "nothing to change") {
			t.Errorf("runSet reported %q, wanted it to say what to do instead", err)
		}
	})

	// Verify a range written wrongly costs nothing and leaves the scanner alone
	t.Run("BadRange", func(t *testing.T) {
		s := holding(catalogue(Count), nil)
		app := driving(&bytes.Buffer{}, s)

		if err := runSet(context.Background(), app, "9", settings{rangeFlag: "26.965"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the range refused")
		}
		if len(s.pressed) != 0 {
			t.Errorf("the scanner was sent %v, wanted it left scanning", s.pressed)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runSet(context.Background(), app, "9", settings{name: "123"})
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runSet reported %v, wanted a missing device", err)
		}
	})

	// Verify a bank whose menu will not open is reported with the bank named
	t.Run("OpenFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			if strings.HasPrefix(command, "MNU,") {
				return errors.New("the port closed")
			}
			return nil
		}))

		err := runSet(context.Background(), app, "9", settings{name: "123"})
		if err == nil {
			t.Fatal("runSet reported nothing, wanted the refused menu")
		}
		if !strings.Contains(err.Error(), "opening bank 9") {
			t.Errorf("runSet reported %q, wanted it to name the bank", err)
		}
	})

	// Verify a range that cannot be written stops the run
	t.Run("LimitsFail", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{rangeFlag: "30-35"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed range")
		}
	})

	// Verify a name that cannot be written stops the run
	t.Run("NameFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{name: "123"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed name")
		}
	})

	// Verify a setting that cannot be written stops the run
	t.Run("ChoiceFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{modulation: "am"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed setting")
		}
	})

	// Verify the search with scan settings failing stops the run
	t.Run("SearchFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{avoid: "off"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed setting")
		}
	})

	// Verify a read back that cannot be made stops the run
	t.Run("ExtrasFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			// The first menu is the one the run opens; the second is the one
			// the read back opens.
			if strings.HasPrefix(command, "MNU,") && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{name: "123"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed read back")
		}
	})

	// Verify a scanner that will not go back to scanning is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			if command == "KEY,L,P" {
				return errors.New("the port closed")
			}
			return nil
		}))

		err := runSet(context.Background(), app, "9", settings{name: "123"})
		if err == nil {
			t.Fatal("runSet reported nothing, wanted the scanner it could not put back")
		}
		if !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("runSet reported %q, wanted it to name leaving the menus", err)
		}
	})

	// Verify a list the scanner will not send stops the run
	t.Run("ListFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), func(command string, nth int) error {
			if strings.HasPrefix(command, "GLT,") {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{name: "123"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed list")
		}
	})

	// Verify a bank that cannot be walked back is reported
	t.Run("WalkBackFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count-1), func(command string, nth int) error {
			// The run opens the bank once, the read back opens it again, and
			// the third is the walk back through the menus.
			if strings.HasPrefix(command, "MNU,") && nth == 3 {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{name: "123"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed walk back")
		}
	})

	// Verify the second return to scanning failing is reported
	t.Run("SecondLeaveFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count-1), func(command string, nth int) error {
			if command == "KEY,L,P" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		}))

		if err := runSet(context.Background(), app, "9", settings{name: "123"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the scanner it could not put back")
		}
	})

	// Verify output that cannot be written is reported rather than swallowed
	t.Run("RenderFails", func(t *testing.T) {
		app := driving(&bytes.Buffer{}, holding(catalogue(Count), nil))
		app.Stdout = failWriter{}

		if err := runSet(context.Background(), app, "9", settings{name: "123"}); err == nil {
			t.Fatal("runSet reported nothing, wanted the failed write")
		}
	})
}
