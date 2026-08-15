// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package banks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// fakeConn is a device.Conn that answers each command from a function the test
// supplies, so the commands can be driven with no scanner attached.
type fakeConn struct {
	reply func(command string) (string, error)
}

// Info describes the connected scanner.
func (f fakeConn) Info() device.Info { return device.Info{} }

// Execute answers the command the way the test asked it to.
func (f fakeConn) Execute(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// ExecuteXML answers the command the way the test asked it to.
func (f fakeConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// Send reports whatever answering the command would have reported.
func (f fakeConn) Send(ctx context.Context, command string) error {
	_, err := f.reply(command)
	return err
}

// Close releases nothing, because there is no port.
func (f fakeConn) Close() error { return nil }

// failWriter is a stream that refuses everything written to it, which is how a
// closed pipe behaves when the output is being read by something else.
type failWriter struct{}

// Write refuses the bytes and says why.
func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("the pipe closed") }

// TestNew covers the banks command and the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering the command and its closure)
//
// Test cases:
//   - Wiring: the command is named, documented and carries the --full flag
//   - HasSubcommands: set, scan and goto are attached
//   - Runs: executing the bare command lists what the scanner reported
func TestNew(t *testing.T) {
	// Builds the list document the scanner answers a bank read with.
	catalogue := func(banks int) string {
		doc := &strings.Builder{}
		doc.WriteString("<GLT>")
		for i := 1; i <= banks; i++ {
			fmt.Fprintf(doc, `<CS_BANK Index="%d" Name="Custom %d" Lower="025.0000" Upper="028.0000" Mod="AM" Step="5000"/>`, i, i-1)
		}
		doc.WriteString("</GLT>")
		return doc.String()
	}

	// Verify the command carries the name, the help text and the flag
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "banks" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "banks")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
		if cmd.Flags().Lookup("full") == nil {
			t.Error("the --full flag is not registered")
		}
	})

	// Verify every way of working a bank is reachable as a subcommand
	t.Run("HasSubcommands", func(t *testing.T) {
		found := map[string]bool{}
		for _, sub := range New(appcontext.New()).Commands() {
			found[sub.Name()] = true
		}
		for _, want := range []string{"set", "scan", "goto"} {
			if !found[want] {
				t.Errorf("the %s subcommand is not attached", want)
			}
		}
	})

	// Verify running the bare command lists the banks the scanner reported
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return catalogue(Count), nil
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "BANK") || !strings.Contains(out.String(), "Custom 9") {
			t.Errorf("the command wrote %q, wanted the table of banks", out.String())
		}
	})
}

// Test_bankNumber covers what somebody can type where a bank is wanted.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Typed: the banks the scanner has, and the ones it does not
func Test_bankNumber(t *testing.T) {
	// Verify whole numbers naming a bank are taken and everything else refused
	t.Run("Typed", func(t *testing.T) {
		cases := []struct {
			typed string
			bank  int
			ok    bool
		}{
			{"0", 0, true},
			{"9", 9, true},

			// A person types spaces, and so does a shell script.
			{" 3 ", 3, true},

			// Refused rather than clamped: searching a bank nobody asked for is
			// worse than a message naming what was typed.
			{"10", 0, false},
			{"-1", 0, false},
			{"nine", 0, false},
			{"", 0, false},
			{"1.5", 0, false},
		}

		for _, c := range cases {
			got, err := bankNumber(c.typed)
			if c.ok && err != nil {
				t.Errorf("bankNumber(%q) reported %v, wanted %d", c.typed, err, c.bank)
				continue
			}
			if !c.ok && err == nil {
				t.Errorf("bankNumber(%q) took it as %d, wanted it refused", c.typed, got)
				continue
			}
			if got != c.bank {
				t.Errorf("bankNumber(%q) is %d, wanted %d", c.typed, got, c.bank)
			}
		}
	})
}

// Test_cancel covers leaving a screen without writing what is on it.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Presses: the menu key is what is sent, since back is refused here
//   - Fails: a key press the scanner will not take is reported
func Test_cancel(t *testing.T) {
	// Verify the menu key is pressed rather than the protocol's own back
	t.Run("Presses", func(t *testing.T) {
		var sent string
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			sent = command
			return "", nil
		}})

		if err := cancel(context.Background(), client); err != nil {
			t.Fatalf("cancel: %v", err)
		}
		if sent != "KEY,M,P" {
			t.Errorf("the scanner was sent %q, wanted %q", sent, "KEY,M,P")
		}
	})

	// Verify a key press the scanner refuses is reported rather than swallowed
	t.Run("Fails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})

		if err := cancel(context.Background(), client); err == nil {
			t.Fatal("cancel reported nothing, wanted the refused key press")
		}
	})
}

// Test_currentValue covers reading what an entry screen holds.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Reads: the value on the screen comes back without the spaces around it
//   - Fails: a screen that cannot be read is reported
func Test_currentValue(t *testing.T) {
	// Verify the value is read off the screen and trimmed
	t.Run("Reads", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return `<MenuInfo Name="Edit Srch Limit" Index="2" MenuType="TypeInput" Value=" 026.9650 ">` +
				`<MenuInput MaxLength="9" EnableKeys="0123456789."/></MenuInfo>`, nil
		}})

		got, err := currentValue(context.Background(), client)
		if err != nil {
			t.Fatalf("currentValue: %v", err)
		}
		if got != "026.9650" {
			t.Errorf("currentValue is %q, wanted %q", got, "026.9650")
		}
	})

	// Verify a screen the scanner will not report is named as a failed read
	t.Run("Fails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})

		_, err := currentValue(context.Background(), client)
		if err == nil {
			t.Fatal("currentValue reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("currentValue reported %q, wanted it to name reading the screen", err)
		}
	})
}

// Test_listed covers the banks the scanner reports outright.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Reads: the banks come back keyed from zero, renumbered from the
//     scanner's own count
//   - SkipsUnnumbered: a bank with an index the scanner should not send is
//     passed over rather than filed under the wrong number
//   - Fails: a list the scanner will not send is reported
func Test_listed(t *testing.T) {
	// Verify the scanner's one-based indexes become this command's zero-based ones
	t.Run("Reads", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if !strings.HasPrefix(command, "GLT,") {
				t.Errorf("the scanner was sent %q, wanted a list read", command)
			}
			return `<GLT>` +
				`<CS_BANK Index="1" Name="Custom 0" Lower="025.0000MHz" Upper="028.0000MHz" Mod="AM" Step="5000"/>` +
				`<CS_BANK Index="10" Name="CB" Lower="026.9650" Upper="027.4050" Mod="Auto" Step="Auto"/>` +
				`</GLT>`, nil
		}})

		found, err := listed(context.Background(), client)
		if err != nil {
			t.Fatalf("listed: %v", err)
		}
		if len(found) != 2 {
			t.Fatalf("listed found %d banks, wanted 2", len(found))
		}

		// The unit is stripped and the step is written the one way, so a bank
		// from the list reads the same as a bank from the menus.
		want := report{
			Bank: 0, Name: "Custom 0", Lower: "025.0000", Upper: "028.0000",
			Modulation: "AM", Step: "5.0 kHz",
		}
		if found[0] != want {
			t.Errorf("bank 0 is %+v, wanted %+v", found[0], want)
		}
		if found[9].Name != "CB" || found[9].Step != "Auto" {
			t.Errorf("bank 9 is %+v, wanted the tenth bank renumbered to nine", found[9])
		}
	})

	// Verify an index outside the banks is passed over rather than misfiled
	t.Run("SkipsUnnumbered", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return `<GLT>` +
				`<CS_BANK Index="0" Name="Too low"/>` +
				`<CS_BANK Index="11" Name="Too high"/>` +
				`<CS_BANK Index="none" Name="Not a number"/>` +
				`<CS_BANK Index="4" Name="Custom 3"/>` +
				`</GLT>`, nil
		}})

		found, err := listed(context.Background(), client)
		if err != nil {
			t.Fatalf("listed: %v", err)
		}
		if len(found) != 1 || found[3].Name != "Custom 3" {
			t.Errorf("listed found %+v, wanted only the bank that was numbered", found)
		}
	})

	// Verify a list the scanner will not send is reported rather than read as empty
	t.Run("Fails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})

		if _, err := listed(context.Background(), client); err == nil {
			t.Fatal("listed reported nothing, wanted the failed read")
		}
	})
}

// Test_megahertz covers stripping the unit off a frequency in a list.
//
// Coverage: 100% (1 test case covering the one path)
//
// Test cases:
//   - Strips: the unit and the spaces around it come off, and a bare number is
//     left alone
func Test_megahertz(t *testing.T) {
	// Verify the unit comes off however the scanner spaced it
	t.Run("Strips", func(t *testing.T) {
		cases := map[string]string{
			"025.0000MHz":   "025.0000",
			" 025.0000MHz ": "025.0000",
			"025.0000 MHz":  "025.0000",
			"025.0000":      "025.0000",
			"":              "",
		}

		for value, want := range cases {
			if got := megahertz(value); got != want {
				t.Errorf("megahertz(%q) is %q, wanted %q", value, got, want)
			}
		}
	})
}

// Test_open covers putting the scanner on one bank's menu.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Opens: the bank's own menu is what is asked for
//   - Fails: a menu the scanner will not open is reported with the bank named
func Test_open(t *testing.T) {
	// Verify the search range menu is opened at the bank that was asked for
	t.Run("Opens", func(t *testing.T) {
		var sent string
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			sent = command
			return "", nil
		}})

		if err := open(context.Background(), client, 9); err != nil {
			t.Fatalf("open: %v", err)
		}
		if sent != "MNU,SRCH_RANGE,9" {
			t.Errorf("the scanner was sent %q, wanted %q", sent, "MNU,SRCH_RANGE,9")
		}
	})

	// Verify a menu the scanner will not open names the bank in the failure
	t.Run("Fails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})

		err := open(context.Background(), client, 4)
		if err == nil {
			t.Fatal("open reported nothing, wanted the refused menu")
		}
		if !strings.Contains(err.Error(), "opening bank 4") {
			t.Errorf("open reported %q, wanted it to name the bank", err)
		}
	})
}

// Test_stepText covers writing a step the one way whichever place it came from.
//
// Coverage: 100% (1 test case covering every branch)
//
// Test cases:
//   - Written: the list's bare hertz and the menus' own spellings agree
func Test_stepText(t *testing.T) {
	// Verify both spellings of the same step come out identical
	t.Run("Written", func(t *testing.T) {
		cases := map[string]string{
			// The list writes a bare number of hertz.
			"5000":  "5.0 kHz",
			"10000": "10.0 kHz",
			"3125":  "3.125 kHz",
			"8330":  "8.33 kHz",

			// The menus write it with a unit, spaced however they please.
			"10.0 kHz": "10.0 kHz",
			"3.125kHz": "3.125 kHz",

			// Not a number at all, and left exactly as it came.
			"Auto":  "Auto",
			" Auto": "Auto",
			"":      "",

			// Nothing to report rather than a step of no width.
			"0": "0",
		}

		for value, want := range cases {
			if got := stepText(value); got != want {
				t.Errorf("stepText(%q) is %q, wanted %q", value, got, want)
			}
		}
	})
}

// bankScanner is a device.Conn standing in for a scanner working through one
// bank's menus, so the walks this package makes can be driven with no scanner
// attached.
//
// It has to be a working model rather than a canned answer. The menus package
// finds an entry by turning the knob until the screen highlights it, so a fake
// answering with a fixed screen would turn the knob until it gave up. What it
// keeps is what those walks depend on: which row the knob is on, and which
// screens have been opened on the way in.
type bankScanner struct {
	// menu are the bank menu's entries, in the order the knob steps through
	// them.
	menu []string

	// choices are the rows behind an entry, keyed by the entry they belong to.
	// For a setting that is a list of options the first is the one the scanner
	// has selected; for a submenu they are its entries.
	choices map[string][]string

	// values are what an entry's screen holds when it holds a value rather
	// than rows, keyed by the entry.
	values map[string]string

	// limits are the two frequencies Edit Srch Limit shows one after the
	// other, since committing the lower moves on to the upper.
	limits []string

	// keys are the characters a text entry screen accepts, which is what
	// decides whether it is typed on the keypad or turned in with the knob.
	keys string

	// list is the document the scanner answers a list read with, for the
	// commands that read the banks outright as well as walking them.
	list string

	// refuse answers a command with an error. It is given the command and how
	// many times that exact command has been sent, counting this one, so a
	// walk can be broken at any step of it.
	refuse func(command string, nth int) error

	open    []string       // the screens entered so far, innermost last
	cursor  int            // the row the knob is on
	step    int            // which of limits the limit screen is showing
	left    bool           // whether an escape key has taken it out of the menus
	counts  map[string]int // how many times each command has been sent
	pressed []string       // every key that was pressed, in order
}

// Info describes the connected scanner.
func (s *bankScanner) Info() device.Info { return device.Info{} }

// Execute answers the commands the scanner replies to in plain text.
func (s *bankScanner) Execute(ctx context.Context, command string) (string, error) {
	return s.answer(command)
}

// ExecuteXML answers the commands the scanner replies to with a document.
func (s *bankScanner) ExecuteXML(ctx context.Context, command string) (string, error) {
	return s.answer(command)
}

// Send reports whatever answering the command would have reported.
func (s *bankScanner) Send(ctx context.Context, command string) error {
	_, err := s.answer(command)
	return err
}

// Close releases nothing, because there is no port.
func (s *bankScanner) Close() error { return nil }

// current is the screen the scanner is on, empty on the bank's own menu.
func (s *bankScanner) current() string {
	if len(s.open) == 0 {
		return ""
	}
	return s.open[len(s.open)-1]
}

// rows are what the screen the scanner is on lists.
func (s *bankScanner) rows() []string {
	on := s.current()
	if on == "" {
		return s.menu
	}
	if rows, ok := s.choices[on]; ok {
		return rows
	}
	if on == entryLimits && len(s.limits) > 0 {
		return []string{s.limits[min(s.step, len(s.limits)-1)]}
	}
	return []string{s.values[on]}
}

// screen reports whether a row opens a screen of its own, rather than being an
// option that is chosen or a value that is read.
func (s *bankScanner) screen(row string) bool {
	if _, ok := s.choices[row]; ok {
		return true
	}
	if _, ok := s.values[row]; ok {
		return true
	}
	return row == entryLimits && len(s.limits) > 0
}

// answer works the scanner the way the menus package expects: the display
// highlights one row, the knob moves it, enter opens what it is on, and the
// menu key comes back out.
func (s *bankScanner) answer(command string) (string, error) {
	if s.counts == nil {
		s.counts = map[string]int{}
	}
	s.counts[command]++
	if s.refuse != nil {
		if err := s.refuse(command, s.counts[command]); err != nil {
			return "", err
		}
	}

	switch {
	case command == "STS":
		// The display form, then the title and the highlighted row, each
		// followed by its attributes. The star is the only thing that marks a
		// row as the one the knob is on.
		return "00,Bank 9,," + s.rows()[s.cursor] + ",*", nil

	case command == "MSI":
		// An escape key takes the scanner out of the menus altogether, which is
		// what it answers a menu read with afterwards.
		if s.left {
			return `<MSI MenuType="TypeError"/>`, nil
		}

		on := s.current()
		if on == "" {
			return `<MSI Name="Bank 9" MenuType="TypeSelect"/>`, nil
		}
		if _, ok := s.choices[on]; ok {
			return `<MSI Name="` + on + `" MenuType="TypeSelect"/>`, nil
		}

		value := s.values[on]
		if on == entryLimits && len(s.limits) > 0 {
			value = s.limits[min(s.step, len(s.limits)-1)]
		}
		keys := s.keys
		if keys == "" {
			keys = "0123456789."
		}
		return `<MSI Name="` + on + `" MenuType="TypeInput" Value="` + value +
			`"><MenuInput MaxLength="16" EnableKeys="` + keys + `"/></MSI>`, nil

	case command == "GSI":
		// Scanning rather than holding, so leaving the menus has nothing to put
		// back afterwards.
		return `<ScannerInfo Mode="Scan Mode" V_Screen="scan"></ScannerInfo>`, nil

	case strings.HasPrefix(command, "GLT,"):
		if s.list == "" {
			return "<GLT></GLT>", nil
		}
		return s.list, nil

	case strings.HasPrefix(command, "KEY,"):
		s.pressed = append(s.pressed, command)
		s.press(command)

	case strings.HasPrefix(command, "MNU,"):
		s.open, s.cursor, s.step, s.left = nil, 0, 0, false
	}
	return "", nil
}

// press moves the scanner the way the key would.
func (s *bankScanner) press(command string) {
	switch command {
	case "KEY,>,P":
		s.cursor = (s.cursor + 1) % len(s.rows())

	case "KEY,E,P":
		row := s.rows()[s.cursor]
		switch {
		// Committing a limit moves on to the other one, and committing that
		// leaves the screen.
		case s.current() == entryLimits && len(s.limits) > 0:
			if s.step++; s.step >= len(s.limits) {
				s.open, s.cursor, s.step = s.open[:len(s.open)-1], 0, 0
			}

		case s.screen(row):
			s.open, s.cursor = append(s.open, row), 0

		// Choosing an option, or committing a value, leaves the screen.
		default:
			if len(s.open) > 0 {
				s.open, s.cursor = s.open[:len(s.open)-1], 0
			}
		}

	case "KEY,M,P":
		if len(s.open) > 0 {
			s.open, s.cursor = s.open[:len(s.open)-1], 0
		}

	// The two keys the menus package alternates between to get out, whichever
	// screen the scanner is on.
	case "KEY,L,P", "KEY,A,P":
		s.open, s.cursor, s.step, s.left = nil, 0, 0, true

	// Anything else is a character key, which a text entry screen fills up
	// with. Reading that back is how the text is checked once it is typed.
	default:
		on := s.current()
		if _, ok := s.values[on]; !ok {
			return
		}
		key := strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P")
		if len(key) == 1 && strings.Contains(s.keys, key) {
			s.values[on] += key
		}
	}
}

// Test_readValue covers reading an entry screen without changing it.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Reads: the value comes back, and the screen is backed out of rather than
//     committed
//   - SelectFails: an entry that cannot be opened names the entry
//   - ReadFails: a screen that cannot be read is reported
//   - CancelFails: a screen that will not be left is reported
func Test_readValue(t *testing.T) {
	// Verify the value is read and the screen is cancelled rather than saved
	t.Run("Reads", func(t *testing.T) {
		s := &bankScanner{
			menu:   []string{entryLimits, entryName},
			values: map[string]string{entryName: " CB "},
			limits: []string{"026.9650", "027.4050"},
		}

		got, err := readValue(context.Background(), device.New(s), entryName)
		if err != nil {
			t.Fatalf("readValue: %v", err)
		}
		if got != "CB" {
			t.Errorf("readValue is %q, wanted %q", got, "CB")
		}

		// The knob is turned onto the entry, opened, and then backed out of
		// with the menu key, so reading a bank never writes to it.
		want := "KEY,>,P KEY,E,P KEY,M,P"
		if strings.Join(s.pressed, " ") != want {
			t.Errorf("the scanner was sent %q, wanted %q", strings.Join(s.pressed, " "), want)
		}
	})

	// Verify an entry the scanner will not show is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		s := &bankScanner{
			menu:   []string{entryName},
			values: map[string]string{entryName: "CB"},
			refuse: func(command string, nth int) error {
				if command == "STS" {
					return errors.New("the port closed")
				}
				return nil
			},
		}

		_, err := readValue(context.Background(), device.New(s), entryName)
		if err == nil {
			t.Fatal("readValue reported nothing, wanted the entry refused")
		}
		if !strings.Contains(err.Error(), `opening "Edit Name"`) {
			t.Errorf("readValue reported %q, wanted it to name the entry", err)
		}
	})

	// Verify a screen that cannot be read is reported rather than read as empty
	t.Run("ReadFails", func(t *testing.T) {
		s := &bankScanner{
			menu:   []string{entryName},
			values: map[string]string{entryName: "CB"},
			refuse: func(command string, nth int) error {
				if command == "MSI" {
					return errors.New("the port closed")
				}
				return nil
			},
		}

		_, err := readValue(context.Background(), device.New(s), entryName)
		if err == nil {
			t.Fatal("readValue reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("readValue reported %q, wanted it to name reading the screen", err)
		}
	})

	// Verify a screen that will not be backed out of is reported
	t.Run("CancelFails", func(t *testing.T) {
		s := &bankScanner{
			menu:   []string{entryName},
			values: map[string]string{entryName: "CB"},
			refuse: func(command string, nth int) error {
				if command == "KEY,M,P" {
					return errors.New("the port closed")
				}
				return nil
			},
		}

		if _, err := readValue(context.Background(), device.New(s), entryName); err == nil {
			t.Fatal("readValue reported nothing, wanted the screen it could not leave")
		}
	})
}

// Test_readChoice covers reading which option a menu of them has selected.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Reads: the highlighted option is the one the setting holds
//   - SelectFails: an entry that cannot be opened names the entry
//   - HighlightFails: a screen that highlights nothing is reported
//   - CancelFails: a screen that will not be left is reported
func Test_readChoice(t *testing.T) {
	// Verify the option the scanner highlights is what comes back
	t.Run("Reads", func(t *testing.T) {
		s := &bankScanner{
			menu:    []string{entryModulation},
			choices: map[string][]string{entryModulation: {" AM ", "Auto", "NFM"}},
		}

		got, err := readChoice(context.Background(), device.New(s), entryModulation)
		if err != nil {
			t.Fatalf("readChoice: %v", err)
		}
		if got != "AM" {
			t.Errorf("readChoice is %q, wanted %q", got, "AM")
		}
		if want := "KEY,E,P KEY,M,P"; strings.Join(s.pressed, " ") != want {
			t.Errorf("the scanner was sent %q, wanted %q", strings.Join(s.pressed, " "), want)
		}
	})

	// Verify an entry the scanner will not show is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		s := &bankScanner{
			menu:    []string{entryModulation},
			choices: map[string][]string{entryModulation: {"AM"}},
			refuse: func(command string, nth int) error {
				if command == "STS" && nth == 1 {
					return errors.New("the port closed")
				}
				return nil
			},
		}

		_, err := readChoice(context.Background(), device.New(s), entryModulation)
		if err == nil {
			t.Fatal("readChoice reported nothing, wanted the entry refused")
		}
		if !strings.Contains(err.Error(), `opening "Set Modulation"`) {
			t.Errorf("readChoice reported %q, wanted it to name the entry", err)
		}
	})

	// Verify a screen whose selection cannot be read is reported
	t.Run("HighlightFails", func(t *testing.T) {
		s := &bankScanner{
			menu:    []string{entryModulation},
			choices: map[string][]string{entryModulation: {"AM"}},
			refuse: func(command string, nth int) error {
				if command == "STS" && nth == 2 {
					return errors.New("the port closed")
				}
				return nil
			},
		}

		if _, err := readChoice(context.Background(), device.New(s), entryModulation); err == nil {
			t.Fatal("readChoice reported nothing, wanted the failed read")
		}
	})

	// Verify a screen that will not be backed out of is reported
	t.Run("CancelFails", func(t *testing.T) {
		s := &bankScanner{
			menu:    []string{entryModulation},
			choices: map[string][]string{entryModulation: {"AM"}},
			refuse: func(command string, nth int) error {
				if command == "KEY,M,P" {
					return errors.New("the port closed")
				}
				return nil
			},
		}

		if _, err := readChoice(context.Background(), device.New(s), entryModulation); err == nil {
			t.Fatal("readChoice reported nothing, wanted the screen it could not leave")
		}
	})
}

// Test_readLimits covers reading both ends of a bank's range.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Reads: the two screens are walked through in order and both come back
//   - SelectFails: an entry that cannot be opened names the entry
//   - LowerFails: a lower limit screen that cannot be read is reported
//   - FirstCommitFails: a scanner that will not move to the upper is reported
//   - UpperFails: an upper limit screen that cannot be read is reported
//   - SecondCommitFails: a scanner that will not leave the screen is reported
func Test_readLimits(t *testing.T) {
	// Builds a scanner holding the two limits, refusing whatever the test names.
	holding := func(refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu:   []string{entryLimits},
			limits: []string{"026.9650", "027.4050"},
			refuse: refuse,
		}
	}

	// Verify committing the lower moves to the upper, and both are read
	t.Run("Reads", func(t *testing.T) {
		s := holding(nil)

		lower, upper, err := readLimits(context.Background(), device.New(s))
		if err != nil {
			t.Fatalf("readLimits: %v", err)
		}
		if lower != "026.9650" || upper != "027.4050" {
			t.Errorf("readLimits is %q to %q, wanted %q to %q",
				lower, upper, "026.9650", "027.4050")
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

		_, _, err := readLimits(context.Background(), device.New(s))
		if err == nil {
			t.Fatal("readLimits reported nothing, wanted the entry refused")
		}
		if !strings.Contains(err.Error(), `opening "Edit Srch Limit"`) {
			t.Errorf("readLimits reported %q, wanted it to name the entry", err)
		}
	})

	// Verify a lower limit that cannot be read is reported
	t.Run("LowerFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "MSI" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, _, err := readLimits(context.Background(), device.New(s)); err == nil {
			t.Fatal("readLimits reported nothing, wanted the failed read")
		}
	})

	// Verify a scanner that will not move on to the upper limit is reported
	t.Run("FirstCommitFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			// The first enter opens the screen; the second is the one that
			// commits the lower limit and moves on.
			if command == "KEY,E,P" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, _, err := readLimits(context.Background(), device.New(s)); err == nil {
			t.Fatal("readLimits reported nothing, wanted the refused commit")
		}
	})

	// Verify an upper limit that cannot be read is reported
	t.Run("UpperFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "MSI" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, _, err := readLimits(context.Background(), device.New(s)); err == nil {
			t.Fatal("readLimits reported nothing, wanted the failed read")
		}
	})

	// Verify a scanner that will not leave the limit screens is reported
	t.Run("SecondCommitFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "KEY,E,P" && nth == 3 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, _, err := readLimits(context.Background(), device.New(s)); err == nil {
			t.Fatal("readLimits reported nothing, wanted the refused commit")
		}
	})
}

// Test_readSearchWithScan covers the two settings behind the submenu.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Reads: both settings come back and the scanner comes out to the bank menu
//   - SelectFails: a submenu that cannot be opened names it
//   - AvoidFails: an avoid setting that cannot be read is reported
//   - HoldFails: a hold time that cannot be read is reported
//   - BackFails: a scanner that will not come back out is reported
func Test_readSearchWithScan(t *testing.T) {
	// Builds a scanner holding the submenu, refusing whatever the test names.
	holding := func(refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu: []string{entrySearchWithScan},
			choices: map[string][]string{
				entrySearchWithScan: {entryAvoid, entryHoldTime},
				entryAvoid:          {"Stop Avoiding", "Temporary Avoid"},
			},
			values: map[string]string{entryHoldTime: "5"},
			refuse: refuse,
		}
	}

	// Verify both settings are read and the scanner is left on the bank's menu
	t.Run("Reads", func(t *testing.T) {
		s := holding(nil)

		var into report
		if err := readSearchWithScan(context.Background(), device.New(s), &into); err != nil {
			t.Fatalf("readSearchWithScan: %v", err)
		}
		if into.Avoid != "Stop Avoiding" || into.HoldTime != "5" {
			t.Errorf("the settings are %q and %q, wanted %q and %q",
				into.Avoid, into.HoldTime, "Stop Avoiding", "5")
		}
		if len(s.open) != 0 {
			t.Errorf("the scanner was left on %v, wanted it back on the bank's menu", s.open)
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

		var into report
		err := readSearchWithScan(context.Background(), device.New(s), &into)
		if err == nil {
			t.Fatal("readSearchWithScan reported nothing, wanted the submenu refused")
		}
		if !strings.Contains(err.Error(), `opening "Search with Scan"`) {
			t.Errorf("readSearchWithScan reported %q, wanted it to name the submenu", err)
		}
	})

	// Verify an avoid setting that cannot be read is reported
	t.Run("AvoidFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "STS" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		var into report
		if err := readSearchWithScan(context.Background(), device.New(s), &into); err == nil {
			t.Fatal("readSearchWithScan reported nothing, wanted the failed read")
		}
	})

	// Verify a hold time that cannot be read is reported
	t.Run("HoldFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "MSI" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		var into report
		if err := readSearchWithScan(context.Background(), device.New(s), &into); err == nil {
			t.Fatal("readSearchWithScan reported nothing, wanted the failed read")
		}
	})

	// Verify a scanner that will not come back out of the submenu is reported
	t.Run("BackFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			// The two cancels inside the submenu come first; the third menu key
			// is the one that backs out of the submenu itself.
			if command == "KEY,M,P" && nth == 3 {
				return errors.New("the port closed")
			}
			return nil
		})

		var into report
		if err := readSearchWithScan(context.Background(), device.New(s), &into); err == nil {
			t.Fatal("readSearchWithScan reported nothing, wanted the refused key press")
		}
	})
}

// Test_extras covers the settings the scanner's list leaves out.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Fills: every setting behind the menus is read into the report
//   - OpenFails: a bank whose menu will not open is reported
//   - ChoiceFails: a setting that cannot be read is reported
func Test_extras(t *testing.T) {
	// Builds a scanner holding a whole bank menu, refusing whatever is named.
	holding := func(refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu: []string{entryAttenuator, entryDelay, entryDigitalWait, entrySearchWithScan},
			choices: map[string][]string{
				entryAttenuator:     {"Off", "On"},
				entryDelay:          {"2 sec", "3 sec"},
				entryDigitalWait:    {"400 ms", "0 ms"},
				entrySearchWithScan: {entryAvoid, entryHoldTime},
				entryAvoid:          {"Stop Avoiding", "Temporary Avoid"},
			},
			values: map[string]string{entryHoldTime: "5"},
			refuse: refuse,
		}
	}

	// Verify each setting the list cannot carry is read out of the menus
	t.Run("Fills", func(t *testing.T) {
		s := holding(nil)

		into := report{Bank: 9}
		if err := extras(context.Background(), device.New(s), 9, &into); err != nil {
			t.Fatalf("extras: %v", err)
		}

		want := report{
			Bank: 9, Attenuator: "Off", Delay: "2 sec", DigitalWait: "400 ms",
			Avoid: "Stop Avoiding", HoldTime: "5",
		}
		if into != want {
			t.Errorf("extras filled in %+v, wanted %+v", into, want)
		}
	})

	// Verify a bank whose menu will not open is reported before anything is read
	t.Run("OpenFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if strings.HasPrefix(command, "MNU,") {
				return errors.New("the port closed")
			}
			return nil
		})

		into := report{Bank: 9}
		err := extras(context.Background(), device.New(s), 9, &into)
		if err == nil {
			t.Fatal("extras reported nothing, wanted the refused menu")
		}
		if !strings.Contains(err.Error(), "opening bank 9") {
			t.Errorf("extras reported %q, wanted it to name the bank", err)
		}
	})

	// Verify a setting that cannot be read stops the walk rather than being blank
	t.Run("ChoiceFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "STS" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		into := report{Bank: 9}
		if err := extras(context.Background(), device.New(s), 9, &into); err == nil {
			t.Fatal("extras reported nothing, wanted the failed read")
		}
	})
}

// Test_walked covers reading a bank the scanner's list left out.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Reads: the bank comes back holding what the list would have carried
//   - OpenFails: a bank whose menu will not open is reported
//   - LimitsFail: limits that cannot be read are reported
//   - NameFails: a name that cannot be read is reported
//   - ModulationFails: a modulation that cannot be read is reported
//   - StepFails: a step that cannot be read is reported
func Test_walked(t *testing.T) {
	// Builds a scanner holding a whole bank menu, refusing whatever is named.
	holding := func(refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			menu:   []string{entryLimits, entryName, entryModulation, entryStep},
			limits: []string{"026.9650", "027.4050"},
			values: map[string]string{entryName: "CB"},
			choices: map[string][]string{
				entryModulation: {"AM", "Auto"},
				entryStep:       {"5000", "Auto"},
			},
			refuse: refuse,
		}
	}

	// Verify the menus give up the same fields the list would have carried
	t.Run("Reads", func(t *testing.T) {
		s := holding(nil)

		got, err := walked(context.Background(), device.New(s), 9)
		if err != nil {
			t.Fatalf("walked: %v", err)
		}

		// The step is written the one way, so a bank read this way reads the
		// same as one read from the list.
		want := report{
			Bank: 9, Name: "CB", Lower: "026.9650", Upper: "027.4050",
			Modulation: "AM", Step: "5.0 kHz",
		}
		if got != want {
			t.Errorf("walked read %+v, wanted %+v", got, want)
		}
	})

	// Verify a bank whose menu will not open is reported with only its number
	t.Run("OpenFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if strings.HasPrefix(command, "MNU,") {
				return errors.New("the port closed")
			}
			return nil
		})

		got, err := walked(context.Background(), device.New(s), 9)
		if err == nil {
			t.Fatal("walked reported nothing, wanted the refused menu")
		}
		if got != (report{Bank: 9}) {
			t.Errorf("walked read %+v, wanted nothing but the bank number", got)
		}
	})

	// Verify limits that cannot be read stop the walk
	t.Run("LimitsFail", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "MSI" && nth == 1 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, err := walked(context.Background(), device.New(s), 9); err == nil {
			t.Fatal("walked reported nothing, wanted the failed read")
		}
	})

	// Verify a name that cannot be read stops the walk
	t.Run("NameFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "MSI" && nth == 3 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, err := walked(context.Background(), device.New(s), 9); err == nil {
			t.Fatal("walked reported nothing, wanted the failed read")
		}
	})

	// Verify a modulation that cannot be read stops the walk
	t.Run("ModulationFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			// The knob is turned onto the modulation entry after the name has
			// been read and backed out of.
			if command == "KEY,M,P" && nth == 2 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, err := walked(context.Background(), device.New(s), 9); err == nil {
			t.Fatal("walked reported nothing, wanted the failed read")
		}
	})

	// Verify a step that cannot be read stops the walk
	t.Run("StepFails", func(t *testing.T) {
		s := holding(func(command string, nth int) error {
			if command == "KEY,M,P" && nth == 3 {
				return errors.New("the port closed")
			}
			return nil
		})

		if _, err := walked(context.Background(), device.New(s), 9); err == nil {
			t.Fatal("walked reported nothing, wanted the failed read")
		}
	})
}
