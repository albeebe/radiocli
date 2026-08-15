// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package display

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

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named, annotated, and carries its subcommand
//   - Runs: executing the command reports the mode the scanner is in
func TestNew(t *testing.T) {
	// Verify the command carries the name, the annotation and the subcommand
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "display" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "display")
		}
		if got := cmd.Annotations[appcontext.OnlyReads]; got != "true" {
			t.Errorf("the only-reads annotation is %q, wanted %q", got, "true")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		found := false
		for _, sub := range cmd.Commands() {
			if strings.HasPrefix(sub.Use, "mode") {
				found = true
			}
		}
		if !found {
			t.Error("the command has no mode subcommand")
		}
	})

	// Verify running the command reads the scanner and writes what it read
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,0,0,0,0,0,FM,0,0,0,0,0,0", nil
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "display: color") {
			t.Errorf("the command wrote %q, wanted it to report color", out.String())
		}
	})
}

// Test_newMode covers the subcommand, its argument checking, and its two runs.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Wiring: the subcommand is named and offers the three modes
//   - AcceptsKnown: a mode this command knows is allowed through
//   - AcceptsNone: running it with no mode at all is allowed through
//   - RejectsUnknown: a word that is not a mode is refused by name
//   - RejectsTooMany: more than one mode is refused
//   - Reports: run with no mode it reports the current one
//   - Sets: run with a mode it writes that one
func Test_newMode(t *testing.T) {
	// The scanner's status reply, with the display mode in the eleventh field.
	status := func(mode int) string {
		return "0,SCAN,*,0,0,0,0,0,FM,0,0,0,0," + string(rune('0'+mode)) + ",0"
	}

	// Verify the subcommand is named and offers the modes as valid arguments
	t.Run("Wiring", func(t *testing.T) {
		cmd := newMode(appcontext.New())

		if !strings.HasPrefix(cmd.Use, "mode") {
			t.Errorf("the subcommand is %q, wanted it to start with mode", cmd.Use)
		}
		if len(cmd.ValidArgs) != len(modes) {
			t.Errorf("the subcommand offers %v, wanted the three modes", cmd.ValidArgs)
		}
	})

	// Verify a mode this command knows is allowed through
	t.Run("AcceptsKnown", func(t *testing.T) {
		cmd := newMode(appcontext.New())
		if err := cmd.Args(cmd, []string{"white"}); err != nil {
			t.Errorf("the subcommand refused a known mode: %v", err)
		}
	})

	// Verify running it with no mode at all is allowed through
	t.Run("AcceptsNone", func(t *testing.T) {
		cmd := newMode(appcontext.New())
		if err := cmd.Args(cmd, nil); err != nil {
			t.Errorf("the subcommand refused an empty argument list: %v", err)
		}
	})

	// Verify a word that is not a mode is refused, with the modes named
	t.Run("RejectsUnknown", func(t *testing.T) {
		cmd := newMode(appcontext.New())

		err := cmd.Args(cmd, []string{"purple"})
		if err == nil || !strings.Contains(err.Error(), "is not a display mode") {
			t.Errorf("the subcommand reported %v, wanted a refusal naming the modes", err)
		}
	})

	// Verify more than one mode is refused before anything is read
	t.Run("RejectsTooMany", func(t *testing.T) {
		cmd := newMode(appcontext.New())

		if err := cmd.Args(cmd, []string{"black", "white"}); err == nil {
			t.Error("the subcommand accepted two modes, wanted at most one")
		}
	})

	// Verify running it with no mode reports the mode the scanner is in
	t.Run("Reports", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return status(2), nil
		}}))

		cmd := newMode(app)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "display: white") {
			t.Errorf("the subcommand wrote %q, wanted it to report white", out.String())
		}
	})

	// Verify running it with a mode writes that one
	t.Run("Sets", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return status(0), nil
		}}))

		cmd := newMode(app)
		if err := cmd.RunE(cmd, []string{"color"}); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "display: color") {
			t.Errorf("the subcommand wrote %q, wanted it to report color", out.String())
		}
	})
}

// Test_descriptions covers the help text built from the modes.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Lists: every mode is named with what it looks like
func Test_descriptions(t *testing.T) {
	// Verify every mode appears with its description
	t.Run("Lists", func(t *testing.T) {
		got := descriptions()

		for _, m := range modes {
			if !strings.Contains(got, m.name) {
				t.Errorf("the descriptions leave out %q: %q", m.name, got)
			}
			if !strings.Contains(got, m.description) {
				t.Errorf("the descriptions leave out %q: %q", m.description, got)
			}
		}
		if strings.Count(got, "\n") != len(modes) {
			t.Errorf("the descriptions have %d lines, wanted %d", strings.Count(got, "\n"), len(modes))
		}
	})
}

// Test_find covers looking a mode up by what the scanner reports.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Known: a value the tool names is found
//   - Unknown: a value the tool has no name for is refused
func Test_find(t *testing.T) {
	// Verify a value the scanner reports is matched to the mode it names
	t.Run("Known", func(t *testing.T) {
		got, ok := find(device.DisplayWhiteBackground)
		if !ok {
			t.Fatal("the white mode was not found")
		}
		if got.name != "white" || got.entry != "White w/Black Text" {
			t.Errorf("the mode is %+v, wanted the white one", got)
		}
	})

	// Verify a value with no name is refused rather than guessed at
	t.Run("Unknown", func(t *testing.T) {
		got, ok := find(device.DisplayMode(9))
		if ok {
			t.Errorf("a mode was found for 9: %+v", got)
		}
	})
}

// Test_lookup covers the names this command accepts on the command line.
//
// Coverage: 100% (3 test cases covering both branches)
//
// Test cases:
//   - Exact: a name typed as written is found
//   - Case: a name typed in another case is found
//   - Unknown: a word that is not a mode is refused
func Test_lookup(t *testing.T) {
	// Verify a name typed as written is found
	t.Run("Exact", func(t *testing.T) {
		got, ok := lookup("black")
		if !ok || got.value != device.DisplayBlackBackground {
			t.Errorf("lookup(\"black\") is %+v, %v", got, ok)
		}
	})

	// Verify the case somebody types in does not matter
	t.Run("Case", func(t *testing.T) {
		got, ok := lookup("COLOR")
		if !ok || got.value != device.DisplayColor {
			t.Errorf("lookup(\"COLOR\") is %+v, %v", got, ok)
		}
	})

	// Verify a word that names no mode is refused
	t.Run("Unknown", func(t *testing.T) {
		if got, ok := lookup("green"); ok {
			t.Errorf("lookup(\"green\") found %+v", got)
		}
	})
}

// Test_names covers the list of names offered in the help and in refusals.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Order: the names are the three modes, in the menu's order
func Test_names(t *testing.T) {
	// Verify the names come back in the order the scanner's menu lists them
	t.Run("Order", func(t *testing.T) {
		got := names()

		want := []string{"color", "black", "white"}
		if len(got) != len(want) {
			t.Fatalf("there are %d names, wanted %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("name %d is %q, wanted %q", i, got[i], want[i])
			}
		}
	})
}

// Test_read covers reading the mode off the scanner.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Success: the scanner's answer is parsed into a mode
//   - Fails: a scanner that will not answer is reported
func Test_read(t *testing.T) {
	// Verify the mode is read out of the scanner's status reply
	t.Run("Success", func(t *testing.T) {
		client := device.New(&stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,0,0,0,0,0,FM,0,0,0,0,2,0", nil
		}})

		got, err := read(context.Background(), client)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if got != device.DisplayWhiteBackground {
			t.Errorf("the mode is %v, wanted white", got)
		}
	})

	// Verify a scanner that will not answer is reported as a failed read
	t.Run("Fails", func(t *testing.T) {
		client := device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := read(context.Background(), client)
		if err == nil {
			t.Fatal("read reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the display mode") {
			t.Errorf("read reported %q, wanted it to name reading the display mode", err)
		}
	})
}

// Test_renderMode covers both ways the mode is written.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Text: the mode and the menu's own wording are written
//   - TextUnknown: a mode with no name is written without a menu line
//   - JSON: the same reading is written as JSON
func Test_renderMode(t *testing.T) {
	// Verify a known mode is written with the wording the menu uses
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderMode(app, device.DisplayColor); err != nil {
			t.Fatalf("renderMode: %v", err)
		}
		want := "display: color\nmenu:    Color Mode\n"
		if got := out.String(); got != want {
			t.Errorf("renderMode wrote %q, wanted %q", got, want)
		}
	})

	// Verify a mode the tool cannot name is written without pretending
	t.Run("TextUnknown", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderMode(app, device.DisplayMode(9)); err != nil {
			t.Fatalf("renderMode: %v", err)
		}
		want := "display: unknown(9)\n"
		if got := out.String(); got != want {
			t.Errorf("renderMode wrote %q, wanted %q", got, want)
		}
	})

	// Verify JSON output carries the mode, the color flag and the menu wording
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := renderMode(app, device.DisplayBlackBackground); err != nil {
			t.Fatalf("renderMode: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if got.Mode != "black" || got.Color || got.Entry != "Black w/White Text" {
			t.Errorf("the report is %+v, wanted the black mode", got)
		}
	})
}

// Test_runReport covers reporting the mode, including its two refusals.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the mode is read and written
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer is reported
func Test_runReport(t *testing.T) {
	// Verify the mode is read from the scanner and written out
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,0,0,0,0,0,FM,0,0,0,0,1,0", nil
		}}))

		if err := runReport(context.Background(), app); err != nil {
			t.Fatalf("runReport: %v", err)
		}
		if !strings.Contains(out.String(), "display: black") {
			t.Errorf("runReport wrote %q, wanted it to report black", out.String())
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

	// Verify a scanner that will not answer is reported
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := runReport(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "reading the display mode") {
			t.Errorf("runReport reported %v, wanted a failed read", err)
		}
	})
}

// Test_runSet covers writing the mode and confirming it took.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - Success: the mode is written, read back, and reported
//   - AlreadyThere: asking for the mode it is in opens no menu
//   - BadName: a word that is not a mode is refused before the scanner is opened
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not say what mode it is in is reported
//   - SetFails: a menu walk that goes wrong is reported
//   - ReadBackFails: a scanner that stops answering after the write is reported
//   - Mismatch: a mode that did not take is reported rather than claimed
func Test_runSet(t *testing.T) {
	// The scanner's status reply, with the display mode in the eleventh field.
	status := func(mode int) string {
		return "0,SCAN,*,0,0,0,0,0,FM,0,0,0,0," + string(rune('0'+mode)) + ",0"
	}

	// Verify a mode is written, read back off the radio, and reported
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		// The three entries the walk selects, each highlighted as it is reached.
		screens := []string{"0,Display Options,*", "0,Set B/W or Color Mode,*", "0,Black w/White Text,*"}
		at := 0
		reads := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				switch command {
				case "GST":
					reads++
					if reads == 1 {
						return status(0), nil
					}
					return status(1), nil
				case "STS":
					screen := screens[at]
					if at < len(screens)-1 {
						at++
					}
					return screen, nil
				}
				return "", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo MenuType="TypeError"/>`, nil
			},
		}
		app.SetDevice(device.New(conn))

		if err := runSet(context.Background(), app, "black"); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		if !strings.Contains(out.String(), "display: black") {
			t.Errorf("runSet wrote %q, wanted it to report black", out.String())
		}
	})

	// Verify asking for the mode the scanner is already in presses nothing
	t.Run("AlreadyThere", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{exec: func(string) (string, error) { return status(0), nil }}
		app.SetDevice(device.New(conn))

		if err := runSet(context.Background(), app, "color"); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		for _, sent := range conn.sent {
			if strings.HasPrefix(sent, "MNU") || strings.HasPrefix(sent, "KEY") {
				t.Errorf("the scanner was sent %q for a mode it was already in", sent)
			}
		}
	})

	// Verify a word that is not a mode is refused before the scanner is opened
	t.Run("BadName", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runSet(context.Background(), app, "purple")
		if err == nil || !strings.Contains(err.Error(), "is not a display mode") {
			t.Errorf("runSet reported %v, wanted a refusal naming the modes", err)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := runSet(context.Background(), app, "black"); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runSet reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not report its mode is reported
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := runSet(context.Background(), app, "black")
		if err == nil || !strings.Contains(err.Error(), "reading the display mode") {
			t.Errorf("runSet reported %v, wanted a failed read", err)
		}
	})

	// Verify a menu walk that goes wrong is reported rather than ignored
	t.Run("SetFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "GST" {
				return status(0), nil
			}
			if command == "MNU,TOP," {
				return "", errors.New("the scanner refused the menu")
			}
			return "", nil
		}}))

		err := runSet(context.Background(), app, "black")
		if err == nil || !strings.Contains(err.Error(), "opening the top menu") {
			t.Errorf("runSet reported %v, wanted the failed menu", err)
		}
	})

	// Verify a scanner that stops answering after the write is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		screens := []string{"0,Display Options,*", "0,Set B/W or Color Mode,*", "0,Black w/White Text,*"}
		at := 0
		reads := 0
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				switch command {
				case "GST":
					reads++
					if reads == 1 {
						return status(0), nil
					}
					return "", errors.New("the port is gone")
				case "STS":
					screen := screens[at]
					if at < len(screens)-1 {
						at++
					}
					return screen, nil
				}
				return "", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo MenuType="TypeError"/>`, nil
			},
		}))

		err := runSet(context.Background(), app, "black")
		if err == nil || !strings.Contains(err.Error(), "reading the display mode") {
			t.Errorf("runSet reported %v, wanted the failed read back", err)
		}
	})

	// Verify a mode that did not take is reported rather than claimed as done
	t.Run("Mismatch", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		screens := []string{"0,Display Options,*", "0,Set B/W or Color Mode,*", "0,Black w/White Text,*"}
		at := 0
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				switch command {
				case "GST":
					return status(0), nil
				case "STS":
					screen := screens[at]
					if at < len(screens)-1 {
						at++
					}
					return screen, nil
				}
				return "", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo MenuType="TypeError"/>`, nil
			},
		}))

		err := runSet(context.Background(), app, "black")
		if err == nil || !strings.Contains(err.Error(), "the display is still in color mode") {
			t.Errorf("runSet reported %v, wanted the unchanged mode", err)
		}
	})
}

// Test_set covers the menu walk that writes the mode.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Success: the walk selects each entry and leaves the menus
//   - OpenFails: a scanner that refuses the top menu is reported
//   - SelectFails: a menu entry that cannot be found is reported
//   - LeaveFails: a scanner that will not leave the menus is reported
func Test_set(t *testing.T) {
	// Verify the walk chooses each entry by name and then leaves the menus
	t.Run("Success", func(t *testing.T) {
		screens := []string{"0,Display Options,*", "0,Set B/W or Color Mode,*", "0,Color Mode,*"}
		at := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command == "STS" {
					screen := screens[at]
					if at < len(screens)-1 {
						at++
					}
					return screen, nil
				}
				return "", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo MenuType="TypeError"/>`, nil
			},
		}

		if err := set(context.Background(), device.New(conn), "Color Mode"); err != nil {
			t.Fatalf("set: %v", err)
		}

		opened := false
		for _, sent := range conn.sent {
			if sent == "MNU,TOP," {
				opened = true
			}
		}
		if !opened {
			t.Errorf("the top menu was never opened: %v", conn.sent)
		}
	})

	// Verify a scanner that refuses the top menu is reported
	t.Run("OpenFails", func(t *testing.T) {
		client := device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "MNU,TOP," {
				return "", errors.New("the scanner refused the menu")
			}
			return "", nil
		}})

		err := set(context.Background(), client, "Color Mode")
		if err == nil || !strings.Contains(err.Error(), "opening the top menu") {
			t.Errorf("set reported %v, wanted the refused menu", err)
		}
	})

	// Verify an entry the scanner will not show is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		client := device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}})

		err := set(context.Background(), client, "Color Mode")
		if err == nil || !strings.Contains(err.Error(), `looking for "Display Options"`) {
			t.Errorf("set reported %v, wanted the entry it could not find", err)
		}
	})

	// Verify a scanner that will not come out of the menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		screens := []string{"0,Display Options,*", "0,Set B/W or Color Mode,*", "0,Color Mode,*"}
		at := 0
		client := device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command == "STS" {
					screen := screens[at]
					if at < len(screens)-1 {
						at++
					}
					return screen, nil
				}
				return "", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Display Options" MenuType="TypeSelect"/>`, nil
			},
		})

		err := set(context.Background(), client, "Color Mode")
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("set reported %v, wanted the scanner stuck in the menus", err)
		}
	})
}
