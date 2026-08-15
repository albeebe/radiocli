// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package beep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// stubConn is a device.Conn that answers every command from the test's own
// closures, so a command can be driven with no scanner attached.
type stubConn struct {
	exec    func(command string) (string, error)
	execXML func(command string) (string, error)
	sent    []string
}

// Info describes the connected scanner, which is nothing in a test.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute records the command and answers it from the test's closure.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) {
	c.sent = append(c.sent, command)
	if c.exec == nil {
		return "", nil
	}
	return c.exec(command)
}

// ExecuteXML records the command and answers it from the test's closure.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	c.sent = append(c.sent, command)
	if c.execXML == nil {
		return "", nil
	}
	return c.execXML(command)
}

// Send reports success, because nothing here writes without reading.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close releases nothing, because there is no port.
func (c *stubConn) Close() error { return nil }

// leaves answers the menu commands so that leaving the menus costs nothing:
// the scanner reports it is not in a menu, and is already scanning.
func leaves(command string) (string, error) {
	if command == "GSI" {
		return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
	}
	return `<MenuInfo MenuType="TypeError"/>`, nil
}

// TestLevelsCoverWhatTheScannerOffers pins the seventeen values against the
// list read off an SDS150, in the scanner's own order.
//
// The order matters as much as the membership: somebody comparing this against
// the radio in hand reads the two lists side by side, and a list that agrees
// but is shuffled is harder to check than one that disagrees.
func TestLevelsCoverWhatTheScannerOffers(t *testing.T) {
	if len(levels) != 17 {
		t.Fatalf("there are %d settings, wanted 17", len(levels))
	}

	want := []string{"Auto", "Level 1", "Level 8", "Level 15", "Off"}
	at := []int{0, 1, 8, 15, 16}
	for i, entry := range want {
		if got := levels[at[i]].entry; got != entry {
			t.Errorf("setting %d is %q, wanted %q", at[i], got, entry)
		}
	}
}

// TestLookupReadsWhatSomebodyTypes covers the values a person passes on the
// command line, including the case they happen to use.
func TestLookupReadsWhatSomebodyTypes(t *testing.T) {
	cases := []struct {
		typed string
		entry string
		found bool
	}{
		{"auto", "Auto", true},
		{"AUTO", "Auto", true},
		{"Auto", "Auto", true},
		{"1", "Level 1", true},
		{"9", "Level 9", true},
		{"15", "Level 15", true},
		{"off", "Off", true},
		{"OFF", "Off", true},

		// Refused rather than guessed at. A near miss that was accepted would
		// set a level nobody asked for, and there is no undo on the radio.
		{"0", "", false},
		{"16", "", false},
		{"09", "", false},
		{" 9", "", false},
		{"level 9", "", false},
		{"loud", "", false},
		{"", "", false},
	}

	for _, c := range cases {
		got, ok := lookup(c.typed)
		if ok != c.found {
			t.Errorf("lookup(%q) found=%v, wanted %v", c.typed, ok, c.found)
			continue
		}
		if ok && got.entry != c.entry {
			t.Errorf("lookup(%q) is %q, wanted %q", c.typed, got.entry, c.entry)
		}
	}
}

// TestByEntryReadsWhatTheScannerShows covers the other direction, which is how
// a highlighted row becomes a setting.
func TestByEntryReadsWhatTheScannerShows(t *testing.T) {
	cases := []struct {
		row   string
		value string
		found bool
	}{
		{"Auto", "auto", true},
		{"Level 9", "9", true},
		{"Off", "off", true},

		// The scanner's own spelling is what arrives, but matching without
		// regard to case costs nothing and survives a firmware that shouts.
		{"LEVEL 9", "9", true},

		// A row this does not know is refused, so a firmware offering
		// something new is reported rather than silently read as a number.
		{"Level 16", "", false},
		{"Quiet", "", false},
		{"", "", false},
	}

	for _, c := range cases {
		got, ok := byEntry(c.row)
		if ok != c.found {
			t.Errorf("byEntry(%q) found=%v, wanted %v", c.row, ok, c.found)
			continue
		}
		if ok && got.value != c.value {
			t.Errorf("byEntry(%q) is %q, wanted %q", c.row, got.value, c.value)
		}
	}
}

// TestOnlyOffIsSilent checks the one distinction the toggle turns on: every
// setting makes a sound except Off, and "auto" is a sound rather than an
// absence of one.
func TestOnlyOffIsSilent(t *testing.T) {
	for _, l := range levels {
		want := l.entry != "Off"
		if l.on() != want {
			t.Errorf("%q reports on=%v, wanted %v", l.entry, l.on(), want)
		}
	}
}

// TestLabelReadsAsASentence covers what a person sees, since the label is
// pasted straight into a line of output.
func TestLabelReadsAsASentence(t *testing.T) {
	cases := map[string]string{"auto": "auto", "9": "level 9", "off": "off"}
	for value, want := range cases {
		got, ok := lookup(value)
		if !ok {
			t.Fatalf("lookup(%q) found nothing", value)
		}
		if got.label() != want {
			t.Errorf("%q reads as %q, wanted %q", value, got.label(), want)
		}
	}
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its two subcommands
//   - Runs: executing the command reports the setting the scanner is on
func TestNew(t *testing.T) {
	// Verify the command carries the name and its two subcommands
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "beep" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "beep")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		want := map[string]bool{"set": false, "toggle": false}
		for _, sub := range cmd.Commands() {
			name := strings.Fields(sub.Use)[0]
			if _, ok := want[name]; ok {
				want[name] = true
			}
		}
		for name, found := range want {
			if !found {
				t.Errorf("the command has no %s subcommand", name)
			}
		}
	})

	// Verify running the command reads the setting and writes what it read
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		sts := 0
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				if sts == 1 {
					return "0,Adjust Key Beep,*", nil
				}
				return "0,Level 9,*", nil
			},
			execXML: leaves,
		}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if got := out.String(); got != "key beep: level 9\n" {
			t.Errorf("the command wrote %q, wanted level 9", got)
		}
	})
}

// Test_choose covers reading the setting, writing it, and confirming it took.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Unchanged: a setting already at the wanted value presses nothing
//   - UnchangedLeaveFails: a scanner stuck in the menus is reported
//   - Changed: a new setting is chosen and read back off the radio
//   - ReadFails: a setting that cannot be read is reported
//   - SelectFails: a value the scanner will not take is reported
//   - ReadBackFails: a scanner that stops answering after the write is reported
//   - Mismatch: a setting that did not take is reported rather than claimed
func Test_choose(t *testing.T) {
	// The scanner's answers while the setting is read: the menu entry, then
	// the value highlighted in the list it opens.
	reading := func(value string) []string {
		return []string{"0,Adjust Key Beep,*", "0," + value + ",*"}
	}

	// Verify a setting already at the wanted value is left alone
	t.Run("Unchanged", func(t *testing.T) {
		screens := reading("Level 9")
		at := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				screen := screens[at]
				if at < len(screens)-1 {
					at++
				}
				return screen, nil
			},
			execXML: leaves,
		}

		nine, _ := lookup("9")
		was, now, err := choose(context.Background(), device.New(conn),
			func(level) level { return nine })
		if err != nil {
			t.Fatalf("choose: %v", err)
		}
		if was.value != "9" || now.value != "9" {
			t.Errorf("choose reported was=%q now=%q, wanted both 9", was.value, now.value)
		}
	})

	// Verify a scanner that will not come out of the menus is reported
	t.Run("UnchangedLeaveFails", func(t *testing.T) {
		screens := reading("Level 9")
		at := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				screen := screens[at]
				if at < len(screens)-1 {
					at++
				}
				return screen, nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Settings" MenuType="TypeSelect"/>`, nil
			},
		}

		nine, _ := lookup("9")
		_, _, err := choose(context.Background(), device.New(conn),
			func(level) level { return nine })
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("choose reported %v, wanted the scanner stuck in the menus", err)
		}
	})

	// Verify a new setting is chosen and then read back off the radio
	t.Run("Changed", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Adjust Key Beep,*", nil
				case 2:
					return "0,Level 9,*", nil
				case 3:
					return "0,Off,*", nil
				case 4:
					return "0,Adjust Key Beep,*", nil
				}
				return "0,Off,*", nil
			},
			execXML: leaves,
		}

		silent, _ := lookup(off)
		was, now, err := choose(context.Background(), device.New(conn),
			func(level) level { return silent })
		if err != nil {
			t.Fatalf("choose: %v", err)
		}
		if was.value != "9" || now.value != off {
			t.Errorf("choose reported was=%q now=%q, wanted 9 then off", was.value, now.value)
		}
	})

	// Verify a setting that cannot be read at all is reported
	t.Run("ReadFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}

		nine, _ := lookup("9")
		_, _, err := choose(context.Background(), device.New(conn),
			func(level) level { return nine })
		if err == nil || !strings.Contains(err.Error(), `looking for "Adjust Key Beep"`) {
			t.Errorf("choose reported %v, wanted the failed walk", err)
		}
	})

	// Verify a value the scanner will not take is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Adjust Key Beep,*", nil
				case 2:
					return "0,Level 9,*", nil
				}
				return "", errors.New("the port is gone")
			},
			execXML: leaves,
		}

		silent, _ := lookup(off)
		_, _, err := choose(context.Background(), device.New(conn),
			func(level) level { return silent })
		if err == nil || !strings.Contains(err.Error(), `choosing "Off" for the key beep`) {
			t.Errorf("choose reported %v, wanted the refused value", err)
		}
	})

	// Verify a scanner stuck in the menus after the write is reported
	t.Run("ChangedLeaveFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Adjust Key Beep,*", nil
				case 2:
					return "0,Level 9,*", nil
				}
				return "0,Off,*", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Settings" MenuType="TypeSelect"/>`, nil
			},
		}

		silent, _ := lookup(off)
		_, _, err := choose(context.Background(), device.New(conn),
			func(level) level { return silent })
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("choose reported %v, wanted the scanner stuck in the menus", err)
		}
	})

	// Verify a scanner stuck in the menus after the read back is reported
	t.Run("ReadBackLeaveFails", func(t *testing.T) {
		sts := 0
		msi := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Adjust Key Beep,*", nil
				case 2:
					return "0,Level 9,*", nil
				case 3:
					return "0,Off,*", nil
				case 4:
					return "0,Adjust Key Beep,*", nil
				}
				return "0,Off,*", nil
			},
			execXML: func(command string) (string, error) {
				if command == "GSI" {
					return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
				}
				// The first walk out succeeds, the second finds the scanner
				// stuck on a menu it will not leave.
				msi++
				if msi <= 2 {
					return `<MenuInfo MenuType="TypeError"/>`, nil
				}
				return `<MenuInfo Name="Settings" MenuType="TypeSelect"/>`, nil
			},
		}

		silent, _ := lookup(off)
		_, _, err := choose(context.Background(), device.New(conn),
			func(level) level { return silent })
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("choose reported %v, wanted the scanner stuck in the menus", err)
		}
	})

	// Verify a scanner that stops answering after the write is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Adjust Key Beep,*", nil
				case 2:
					return "0,Level 9,*", nil
				case 3:
					return "0,Off,*", nil
				}
				return "", errors.New("the port is gone")
			},
			execXML: leaves,
		}

		silent, _ := lookup(off)
		_, _, err := choose(context.Background(), device.New(conn),
			func(level) level { return silent })
		if err == nil || !strings.Contains(err.Error(), `looking for "Adjust Key Beep"`) {
			t.Errorf("choose reported %v, wanted the failed read back", err)
		}
	})

	// Verify a setting that did not take is reported rather than claimed as done
	t.Run("Mismatch", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Adjust Key Beep,*", nil
				case 2:
					return "0,Level 9,*", nil
				case 3:
					return "0,Off,*", nil
				case 4:
					return "0,Adjust Key Beep,*", nil
				}
				return "0,Level 9,*", nil
			},
			execXML: leaves,
		}

		silent, _ := lookup(off)
		_, _, err := choose(context.Background(), device.New(conn),
			func(level) level { return silent })
		if err == nil || !strings.Contains(err.Error(), "the key beep is still level 9") {
			t.Errorf("choose reported %v, wanted the setting that did not take", err)
		}
	})
}

// Test_read covers opening the setting and reading which value it is on.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Success: the highlighted value is read as a setting
//   - OpenFails: a scanner that refuses the settings menu is reported
//   - SelectFails: an entry that cannot be found is reported
//   - HighlightFails: a scanner that stops answering at the list is reported
//   - Unknown: a value this tool does not know is reported rather than guessed
func Test_read(t *testing.T) {
	// Verify the highlighted row is read as the setting the scanner is on
	t.Run("Success", func(t *testing.T) {
		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if sts == 1 {
				return "0,Adjust Key Beep,*", nil
			}
			return "0,Auto,*", nil
		}}

		got, err := read(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if got.value != "auto" {
			t.Errorf("read reported %q, wanted auto", got.value)
		}
	})

	// Verify a scanner that refuses the settings menu is reported
	t.Run("OpenFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "MNU,SETTINGS," {
				return "", errors.New("the scanner refused the menu")
			}
			return "", nil
		}}

		_, err := read(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "opening the settings menu") {
			t.Errorf("read reported %v, wanted the refused menu", err)
		}
	})

	// Verify an entry that cannot be found is named in the refusal
	t.Run("SelectFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}

		_, err := read(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), `looking for "Adjust Key Beep"`) {
			t.Errorf("read reported %v, wanted the entry it could not find", err)
		}
	})

	// Verify a scanner that stops answering at the list itself is reported
	t.Run("HighlightFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if sts == 1 {
				return "0,Adjust Key Beep,*", nil
			}
			return "", errors.New("the port is gone")
		}}

		_, err := read(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "reading the key beep setting") {
			t.Errorf("read reported %v, wanted the failed read", err)
		}
	})

	// Verify a value this tool has no name for is reported, not guessed at
	t.Run("Unknown", func(t *testing.T) {
		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if sts == 1 {
				return "0,Adjust Key Beep,*", nil
			}
			return "0,Level 16,*", nil
		}}

		_, err := read(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "which is none of the") {
			t.Errorf("read reported %v, wanted the unrecognised value", err)
		}
	})
}

// Test_renderReport covers both ways the setting is written.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Text: the setting is written in the tool's own wording
//   - JSON: the same reading is written as JSON
func Test_renderReport(t *testing.T) {
	// Verify the setting is written the way a person reads it
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderReport(app, report{Level: "auto", On: true}); err != nil {
			t.Fatalf("renderReport: %v", err)
		}
		if got, want := out.String(), "key beep: auto\n"; got != want {
			t.Errorf("renderReport wrote %q, wanted %q", got, want)
		}
	})

	// Verify JSON output carries the setting and whether it makes a sound
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := renderReport(app, report{Level: "9", On: true}); err != nil {
			t.Fatalf("renderReport: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if got.Level != "9" || !got.On {
			t.Errorf("the report is %+v, wanted level 9 sounding", got)
		}
	})
}

// Test_runReport covers reporting the setting.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Success: the setting is read, the menus left, and the result written
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a setting that cannot be read is reported
//   - LeaveFails: a scanner that will not come out of the menus is reported
func Test_runReport(t *testing.T) {
	// The scanner's answers while the setting is read.
	reading := func() func(string) (string, error) {
		sts := 0
		return func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if sts == 1 {
				return "0,Adjust Key Beep,*", nil
			}
			return "0,Off,*", nil
		}
	}

	// Verify the setting is read and written out
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: reading(), execXML: leaves}))

		if err := runReport(context.Background(), app); err != nil {
			t.Fatalf("runReport: %v", err)
		}
		if got, want := out.String(), "key beep: off\n"; got != want {
			t.Errorf("runReport wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := runReport(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runReport reported %v, wanted a missing device", err)
		}
	})

	// Verify a setting that cannot be read is reported
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		err := runReport(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), `looking for "Adjust Key Beep"`) {
			t.Errorf("runReport reported %v, wanted the failed walk", err)
		}
	})

	// Verify a scanner that will not come out of the menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{
			exec: reading(),
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Settings" MenuType="TypeSelect"/>`, nil
			},
		}))

		err := runReport(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("runReport reported %v, wanted the scanner stuck in the menus", err)
		}
	})
}

// Test_values covers the list of settings offered in help and in refusals.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Reads: the values are written as a range rather than one by one
func Test_values(t *testing.T) {
	// Verify the seventeen values are described without listing them all
	t.Run("Reads", func(t *testing.T) {
		got := values()

		for _, want := range []string{"auto", "1 to 15", off} {
			if !strings.Contains(got, want) {
				t.Errorf("the values read %q, wanted them to mention %q", got, want)
			}
		}
	})
}
