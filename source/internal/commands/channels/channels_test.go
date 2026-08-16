// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
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

// screen is a text entry screen the scanner opens, as the entry that opens it
// leaves it.
type screen struct {
	title string   // The heading the scanner puts on it, which is never empty on a real one
	value string   // The value it already holds
	keys  string   // The characters it accepts, which is what decides how text goes into it
	opens []string // The menu the scanner shows once the value is accepted, empty to go back
}

// level is a menu the walk came through, kept so leaving one returns to it.
type level struct {
	rows   []string // The menu that was on screen
	cursor int      // The row the knob was on
}

// radio answers as a scanner sitting in its menus, so the walks these commands
// make can be driven with no radio attached.
//
// It holds one menu at a time, and remembers the ones it came through, because
// reading a channel's frequency means going two levels down and coming back.
// Every display request highlights the row the knob is on, which is what keeps
// a walk from waiting for a redraw that is never coming. Opening a menu over
// the wire puts the scanner back on the menu a walk starts from, the way
// opening a department's menu does.
type radio struct {
	top     []string            // The menu opening one over the wire puts on screen
	rows    []string            // The menu on screen, in the order the knob reaches the rows
	cursor  int                 // Which row the knob is on
	opens   map[string][]string // The menu each entry puts on screen when it is pressed
	inputs  map[string]screen   // The text entry screen each entry opens
	docs    map[string]string   // The document answering each list request, by command
	title   string              // The menu reported when asked, empty for a scanner out of the menus
	input   bool                // Whether a text entry screen is open
	showing screen              // The text entry screen that is open
	stack   []level             // The menus walked through to reach this one
	pressed []string            // Every row a press acted on
}

// reply answers one command the way the scanner would.
func (r *radio) reply(command string) (string, error) {
	switch {
	case command == "STS":
		return r.display(), nil
	case command == "MSI":
		return r.menu(), nil
	case command == "GSI":
		return `<ScannerInfo Mode="Scan Mode" V_Screen="scan"/>`, nil
	case strings.HasPrefix(command, "GLT,"):
		return r.docs[command], nil
	case strings.HasPrefix(command, "MNU,"):
		r.rows, r.cursor, r.stack, r.input = r.top, 0, nil, false
	case strings.HasPrefix(command, "KEY,") && strings.HasSuffix(command, ",P"):
		r.press(strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P"))
	}
	return "", nil
}

// back leaves the screen the scanner is on and returns to the one it came from.
func (r *radio) back() {
	r.input = false
	if len(r.stack) == 0 {
		return
	}

	last := r.stack[len(r.stack)-1]
	r.rows, r.cursor, r.stack = last.rows, last.cursor, r.stack[:len(r.stack)-1]
}

// display renders the row the knob is on as the reply to a display request,
// marked in the reverse video the scanner highlights the selection with.
func (r *radio) display() string {
	row := ""
	if len(r.rows) > 0 {
		row = r.rows[r.cursor]
	}
	return "0," + row + ",****************"
}

// enter presses the row the knob is on.
func (r *radio) enter() {
	// A text entry screen takes this as accepting the value, which either
	// moves the scanner on or leaves it where it came from.
	if r.input {
		accepted := r.showing
		r.back()
		if len(accepted.opens) > 0 {
			r.rows, r.cursor = accepted.opens, 0
		}
		return
	}
	if len(r.rows) == 0 {
		return
	}

	row := r.rows[r.cursor]
	r.pressed = append(r.pressed, row)

	if s, open := r.inputs[row]; open {
		r.stack = append(r.stack, level{rows: r.rows, cursor: r.cursor})
		r.input, r.showing = true, s
		return
	}
	if next, open := r.opens[row]; open {
		r.stack = append(r.stack, level{rows: r.rows, cursor: r.cursor})
		r.rows, r.cursor = next, 0
	}
}

// menu renders the screen the scanner is showing as the reply to a menu
// request. A radio that has left the menus answers the way the scanner does,
// which is with an error document.
func (r *radio) menu() string {
	if r.input {
		// A real entry screen always has a heading, and a scanner reporting an
		// empty one means it is still busy, so this never reports one.
		title := r.showing.title
		if title == "" {
			title = "Name"
		}
		return fmt.Sprintf(`<MenuInfo Name="%s" MenuType="TypeInput" Value="%s">`+
			`<MenuInput MaxLength="16" EnableKeys="%s"/></MenuInfo>`, title, r.showing.value, r.showing.keys)
	}
	if r.title == "" {
		return `<MenuInfo MenuType="TypeError"/>`
	}

	items := ""
	for i, row := range r.rows {
		items += fmt.Sprintf(`<MenuItem Name="%s" Index="%d"/>`, row, i)
	}
	return fmt.Sprintf(`<MenuInfo Name="%s" MenuType="TypeList">%s</MenuInfo>`, r.title, items)
}

// press acts on one key the way the scanner would, given what is on screen.
func (r *radio) press(key string) {
	switch key {
	case ">":
		if len(r.rows) > 0 {
			r.cursor = (r.cursor + 1) % len(r.rows)
		}
	case "E":
		r.enter()
	case "M":
		r.back()
	default:
		// A number entry screen takes its characters from the keypad, so one
		// of those keys types a character rather than moving anything.
		if r.input && len(key) == 1 && strings.Contains("0123456789.", key) {
			r.showing.value += key
		}
	}
}

// quiet returns an application context writing to buffers rather than to the
// terminal, along with the buffers it writes to.
func quiet() (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, notes := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, notes
	return app, out, notes
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the command is named and carries its flag and help text
//   - Subcommands: every subcommand is attached
//   - Runs: executing the command lists what the department holds
func TestNew(t *testing.T) {
	// Verify the command carries the name, the flag and the help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Name() != "channels" {
			t.Errorf("the command is %q, wanted %q", cmd.Name(), "channels")
		}
		if cmd.Flags().Lookup("names") == nil {
			t.Error("the command has no --names flag")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify each of the three subcommands is reachable
	t.Run("Subcommands", func(t *testing.T) {
		attached := map[string]bool{}
		for _, sub := range New(appcontext.New()).Commands() {
			attached[sub.Name()] = true
		}

		for _, name := range []string{"new", "rename", "delete"} {
			if !attached[name] {
				t.Errorf("the %s subcommand is not attached", name)
			}
		}
	})

	// Verify running the command reads the department and lists what it holds
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := quiet()
		r := &radio{docs: map[string]string{
			"GLT,CFREQ,0": `<GLT><CFREQ Name="DISPATCH" Index="0" Freq="153.980" Avoid="Off"/></GLT>`,
		}}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		cmd := New(app)
		cmd.SetArgs([]string{"0"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "DISPATCH") {
			t.Errorf("the command wrote %q, wanted the channel named in it", out.String())
		}
	})
}

// Test_newDelete covers the delete subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the subcommand is named and carries the flag that agrees to it
//   - Runs: executing the subcommand with no scanner named is refused
func Test_newDelete(t *testing.T) {
	// Verify the subcommand is named and carries the flag deleting requires
	t.Run("Wiring", func(t *testing.T) {
		cmd := newDelete(appcontext.New())

		if cmd.Name() != "delete" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "delete")
		}
		if cmd.Flags().Lookup("yes") == nil {
			t.Error("the subcommand has no --yes flag")
		}
	})

	// Verify the closure runs and refuses a run with no scanner named
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()

		cmd := newDelete(app)
		cmd.SetArgs([]string{"0", "DISPATCH", "--yes"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})
}

// Test_newNew covers the new subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the subcommand is named and takes all three arguments
//   - Runs: executing the subcommand with no scanner named is refused
func Test_newNew(t *testing.T) {
	// Verify the subcommand is named and takes the department, the address and
	// the name
	t.Run("Wiring", func(t *testing.T) {
		cmd := newNew(appcontext.New())

		if cmd.Name() != "new" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "new")
		}
		if err := cmd.Args(cmd, []string{"0", "153.980"}); err == nil {
			t.Error("the subcommand took two arguments, wanted all three")
		}
	})

	// Verify the closure runs and refuses a run with no scanner named
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()

		cmd := newNew(app)
		cmd.SetArgs([]string{"0", "153.980", "DISPATCH"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})
}

// Test_newRename covers the rename subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the subcommand is named and takes all three arguments
//   - Runs: executing the subcommand with no scanner named is refused
func Test_newRename(t *testing.T) {
	// Verify the subcommand is named and takes the department, the channel and
	// the new name
	t.Run("Wiring", func(t *testing.T) {
		cmd := newRename(appcontext.New())

		if cmd.Name() != "rename" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "rename")
		}
		if err := cmd.Args(cmd, []string{"0", "DISPATCH"}); err == nil {
			t.Error("the subcommand took two arguments, wanted all three")
		}
	})

	// Verify the closure runs and refuses a run with no scanner named
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()

		cmd := newRename(app)
		cmd.SetArgs([]string{"0", "DISPATCH", "NEW NAME"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})
}

// Test_channel_address covers what a channel receives, however it is addressed.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Addresses: a talkgroup is written with its prefix, a frequency as it is
func Test_channel_address(t *testing.T) {
	// Verify a talkgroup carries the prefix the scanner writes it with, and a
	// frequency is written as the scanner holds it
	t.Run("Addresses", func(t *testing.T) {
		c := channel{Name: "FIRE DISPATCH", Talkgroup: "9051"}
		if got, want := c.address(), "TGID:9051"; got != want {
			t.Errorf("address is %q, wanted %q", got, want)
		}

		c = channel{Name: "DISPATCH", Frequency: "153.980"}
		if got, want := c.address(), "153.980"; got != want {
			t.Errorf("address is %q, wanted %q", got, want)
		}
	})
}

// Test_ask covers reading a department's channels, however they can be reached.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Frequencies: a conventional department answers over the protocol
//   - Talkgroups: a trunked department answers with talkgroups instead
//   - Walks: a firmware answering the wrong document falls back to the menus
//   - GaveUp: a caller who gave up is answered rather than sent down the slow
//     path
func Test_ask(t *testing.T) {
	// Verify a conventional department's frequencies are read in one exchange
	t.Run("Frequencies", func(t *testing.T) {
		r := &radio{docs: map[string]string{
			"GLT,CFREQ,0": `<GLT><CFREQ Name="DISPATCH" Index="0" Freq=" 153.980 " Avoid="Off"/></GLT>`,
		}}
		client := device.New(fakeConn{reply: r.reply})

		found, err := ask(context.Background(), client, "0", false)
		if err != nil {
			t.Fatalf("ask: %v", err)
		}
		if len(found) != 1 || found[0].Frequency != "153.980" {
			t.Errorf("ask read %+v, wanted the one channel and its frequency", found)
		}
	})

	// Verify a trunked department answers with talkgroups rather than
	// frequencies
	t.Run("Talkgroups", func(t *testing.T) {
		r := &radio{docs: map[string]string{
			"GLT,CFREQ,0": `<GLT></GLT>`,
			"GLT,TGID,0":  `<GLT><TGID Name="FIRE DISPATCH" Index="0" TGID="9051" Avoid="Off"/></GLT>`,
		}}
		client := device.New(fakeConn{reply: r.reply})

		found, err := ask(context.Background(), client, "0", false)
		if err != nil {
			t.Fatalf("ask: %v", err)
		}
		if len(found) != 1 || found[0].Talkgroup != "9051" {
			t.Errorf("ask read %+v, wanted the one channel and its talkgroup", found)
		}
	})

	// Verify a firmware answering the wrong document is read off the screen
	// instead, which is what this command was written for
	t.Run("Walks", func(t *testing.T) {
		r := &radio{
			top:  []string{"Edit Channel"},
			rows: []string{"Edit Channel"},
			opens: map[string][]string{
				"Edit Channel": {"New Channel", "DISPATCH"},
				"DISPATCH":     {"Edit Name", "Edit Frequency"},
			},
			inputs: map[string]screen{
				"Edit Frequency": {title: "Frequency", value: "153.980", keys: "0123456789."},
			},
			docs: map[string]string{
				// The request that should list channels answers with a list of
				// another kind, which is the firmware bug this walks around.
				"GLT,CFREQ,0": `<GLT><TGID Name="FIRE" Index="0" TGID="9051"/></GLT>`,
			},
		}
		client := device.New(fakeConn{reply: r.reply})

		found, err := ask(context.Background(), client, "0", false)
		if err != nil {
			t.Fatalf("ask: %v", err)
		}
		if len(found) != 1 || found[0].Name != "DISPATCH" || found[0].Frequency != "153.980" {
			t.Errorf("ask read %+v, wanted the channel read off the screen", found)
		}
	})

	// Verify that a caller who gave up is told so rather than being sent down
	// the fallback. The walk is several seconds and a great many key presses,
	// and starting it because somebody pressed Ctrl-C is spending all of that
	// on an answer nobody is waiting for.
	t.Run("GaveUp", func(t *testing.T) {
		r := &radio{
			top:  []string{"Edit Channel"},
			rows: []string{"Edit Channel"},
			opens: map[string][]string{
				"Edit Channel": {"New Channel", "DISPATCH"},
				"DISPATCH":     {"Edit Name", "Edit Frequency"},
			},
			docs: map[string]string{
				"GLT,CFREQ,0": `<GLT><TGID Name="FIRE" Index="0" TGID="9051"/></GLT>`,
			},
		}
		client := device.New(fakeConn{reply: r.reply})

		ctx, stop := context.WithCancel(context.Background())
		stop()

		if _, err := ask(ctx, client, "0", false); !errors.Is(err, context.Canceled) {
			t.Fatalf("ask reported %v, wanted the caller's own cancellation", err)
		}
		if len(r.pressed) != 0 {
			t.Errorf("ask pressed %v after the caller gave up, wanted no keys at all", r.pressed)
		}
	})
}

// Test_collect covers reading the channel list the scanner is showing.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Names: the names are read and the entry that creates one is skipped
//   - Frequencies: each channel is opened to read what it is tuned to
//   - ListFails: a screen that cannot be read is reported
//   - FrequencyFails: a channel that cannot be opened is reported
func Test_collect(t *testing.T) {
	// Verify the names are read and New Channel is left out, since it is not a
	// channel
	t.Run("Names", func(t *testing.T) {
		r := &radio{rows: []string{"New Channel", "DISPATCH", "FIREGROUND"}}
		client := device.New(fakeConn{reply: r.reply})

		found, err := collect(context.Background(), client, true)
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		if len(found) != 2 || found[0].Name != "DISPATCH" || found[1].Name != "FIREGROUND" {
			t.Errorf("collect read %+v, wanted the two channels", found)
		}
	})

	// Verify each channel is opened to read the frequency it is tuned to
	t.Run("Frequencies", func(t *testing.T) {
		r := &radio{
			rows: []string{"New Channel", "DISPATCH"},
			opens: map[string][]string{
				"DISPATCH": {"Edit Name", "Edit Frequency"},
			},
			inputs: map[string]screen{
				"Edit Frequency": {title: "Frequency", value: "153.980", keys: "0123456789."},
			},
		}
		client := device.New(fakeConn{reply: r.reply})

		found, err := collect(context.Background(), client, false)
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		if len(found) != 1 || found[0].Frequency != "153.980" {
			t.Errorf("collect read %+v, wanted the channel and its frequency", found)
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ListFails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := collect(context.Background(), client, true)
		if err == nil || !strings.Contains(err.Error(), "reading the channel list") {
			t.Fatalf("collect reported %v, wanted the failed read", err)
		}
	})

	// Verify a channel that will not open is reported rather than left blank
	t.Run("FrequencyFails", func(t *testing.T) {
		r := &radio{rows: []string{"New Channel", "DISPATCH"}}
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "KEY,E,P" {
				return "", errors.New("the scanner refused the key")
			}
			return r.reply(command)
		}})

		_, err := collect(context.Background(), client, false)
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("collect reported %v, wanted the refused press", err)
		}
	})
}

// Test_entered covers reading what a new channel is to receive.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Frequency: a frequency is read and typed as it was written
//   - Talkgroup: the TGID: prefix is stripped and the rest handed back
//   - NoTalkgroup: a bare prefix is refused as a missing talkgroup
//   - NoFrequency: a blank address is refused as a missing frequency rather
//     than as a missing talkgroup
//   - Units: a frequency carrying a unit is accepted and rewritten
//   - NotAFrequency: an address that is neither is refused before the scanner
//     is touched
func Test_entered(t *testing.T) {
	// Verify a frequency is read and handed back exactly as it was written
	t.Run("Frequency", func(t *testing.T) {
		value, wantTalkgroup, err := entered("153.980")
		if err != nil || wantTalkgroup || value != "153.980" {
			t.Fatalf("entered gave %q, %v and %v", value, wantTalkgroup, err)
		}
	})

	// Verify the prefix is stripped and what follows is what gets typed
	t.Run("Talkgroup", func(t *testing.T) {
		value, wantTalkgroup, err := entered("TGID:9051")
		if err != nil || !wantTalkgroup || value != "9051" {
			t.Fatalf("entered gave %q, %v and %v", value, wantTalkgroup, err)
		}
	})

	// Verify a bare prefix is refused as the missing talkgroup it is
	t.Run("NoTalkgroup", func(t *testing.T) {
		_, _, err := entered("TGID:")
		if err == nil || !strings.Contains(err.Error(), "no talkgroup was given") {
			t.Fatalf("entered reported %v, wanted the missing talkgroup", err)
		}
	})

	// Verify a blank address is refused as a missing frequency. It used to be
	// refused as a missing talkgroup whichever kind was meant, which sent
	// somebody who left a frequency off to go and write "TGID:9051" instead.
	t.Run("NoFrequency", func(t *testing.T) {
		for _, address := range []string{"", "   "} {
			_, _, err := entered(address)
			if err == nil || !strings.Contains(err.Error(), "no frequency was given") {
				t.Errorf("entered(%q) reported %v, wanted the missing frequency", address, err)
			}
			if err != nil && strings.Contains(err.Error(), "no talkgroup was given") {
				t.Errorf("entered(%q) blamed a missing talkgroup for a missing frequency", address)
			}
		}
	})

	// Verify a frequency carrying a unit is taken, and comes back as digits the
	// entry screen has keys for
	t.Run("Units", func(t *testing.T) {
		value, wantTalkgroup, err := entered("153.98MHz")
		if err != nil || wantTalkgroup || value != "153.98" {
			t.Fatalf("entered gave %q, %v and %v", value, wantTalkgroup, err)
		}
	})

	// Verify an address that is neither is refused here rather than halfway
	// through an entry screen with a half-made channel on it
	t.Run("NotAFrequency", func(t *testing.T) {
		for _, address := range []string{"VHF", "1.5e2", "-153.98"} {
			if _, _, err := entered(address); err == nil {
				t.Errorf("entered(%q) took an address the entry screen would not", address)
			}
		}
	})
}

// Test_expected covers checking that the entry screen is the one the argument
// was written for.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Frequency: a frequency written for a frequency screen is accepted
//   - Talkgroup: a talkgroup written for a talkgroup screen is accepted
//   - ReadFails: a screen that cannot be read is reported
//   - Neither: a scanner showing something else entirely is reported
//   - WantedFrequency: a frequency written for a talkgroup screen is refused
//   - WantedTalkgroup: a talkgroup written for a frequency screen is refused
func Test_expected(t *testing.T) {
	// Verify a frequency is accepted on the screen a conventional department
	// opens
	t.Run("Frequency", func(t *testing.T) {
		r := &radio{input: true, showing: screen{title: "Input Frequency"}}
		client := device.New(fakeConn{reply: r.reply})

		if err := expected(context.Background(), client, false, "FIRE"); err != nil {
			t.Fatalf("expected: %v", err)
		}
	})

	// Verify a talkgroup is accepted on the screen a trunked department opens
	t.Run("Talkgroup", func(t *testing.T) {
		r := &radio{input: true, showing: screen{title: "Input TGID"}}
		client := device.New(fakeConn{reply: r.reply})

		if err := expected(context.Background(), client, true, "FIRE"); err != nil {
			t.Fatalf("expected: %v", err)
		}
	})

	// Verify a screen that cannot be read is reported, and says nothing was
	// created
	t.Run("ReadFails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := expected(context.Background(), client, false, "FIRE")
		if err == nil || !strings.Contains(err.Error(), "reading the entry screen") {
			t.Fatalf("expected reported %v, wanted the failed read", err)
		}
	})

	// Verify a scanner that opened neither entry screen is reported plainly
	t.Run("Neither", func(t *testing.T) {
		r := &radio{input: true, showing: screen{title: "Channel Menu"}}
		client := device.New(fakeConn{reply: r.reply})

		err := expected(context.Background(), client, false, "FIRE")
		if err == nil || !strings.Contains(err.Error(), "rather than an entry screen") {
			t.Fatalf("expected reported %v, wanted the unexpected screen", err)
		}
	})

	// Verify a talkgroup written for a department that takes a frequency is
	// refused before anything is typed
	t.Run("WantedFrequency", func(t *testing.T) {
		r := &radio{input: true, showing: screen{title: "Input Frequency"}}
		client := device.New(fakeConn{reply: r.reply})

		err := expected(context.Background(), client, true, "FIRE")
		if err == nil || !strings.Contains(err.Error(), "takes a frequency, not a talkgroup") {
			t.Fatalf("expected reported %v, wanted the mismatch explained", err)
		}
	})

	// Verify a frequency written for a department that takes a talkgroup is
	// refused before anything is typed
	t.Run("WantedTalkgroup", func(t *testing.T) {
		r := &radio{input: true, showing: screen{title: "Input TGID"}}
		client := device.New(fakeConn{reply: r.reply})

		err := expected(context.Background(), client, false, "FIRE")
		if err == nil || !strings.Contains(err.Error(), "takes a talkgroup, not a frequency") {
			t.Fatalf("expected reported %v, wanted the mismatch explained", err)
		}
	})
}

// Test_frequency covers opening one channel and reading what it is tuned to.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Reads: the channel is opened, read, and left the way it was found
//   - Talkgroup: a channel with no frequency screen is not a failure
//   - StepFails: a list holding no such channel is reported
//   - OpenFails: a channel that will not open is reported
//   - ScreenFails: a frequency screen that will not open is reported
//   - ReadFails: a frequency screen that cannot be read is reported
func Test_frequency(t *testing.T) {
	// walk builds a radio showing a channel list, where opening a channel
	// leads to its own menu and its frequency screen.
	walk := func() *radio {
		return &radio{
			rows: []string{"New Channel", "DISPATCH"},
			opens: map[string][]string{
				"DISPATCH": {"Edit Name", "Edit Frequency"},
			},
			inputs: map[string]screen{
				"Edit Frequency": {title: "Frequency", value: "153.980", keys: "0123456789."},
			},
		}
	}

	// Verify the frequency is read, and the walk comes back out to the list
	t.Run("Reads", func(t *testing.T) {
		r := walk()
		client := device.New(fakeConn{reply: r.reply})

		got, err := frequency(context.Background(), client, "DISPATCH")
		if err != nil {
			t.Fatalf("frequency: %v", err)
		}
		if got != "153.980" {
			t.Errorf("frequency is %q, wanted %q", got, "153.980")
		}
		if len(r.stack) != 0 {
			t.Errorf("the scanner is %d levels down, wanted it back where it started", len(r.stack))
		}
	})

	// Verify a talkgroup channel, which has no frequency screen at all, is a
	// fact about the channel rather than a failure
	t.Run("Talkgroup", func(t *testing.T) {
		r := walk()
		r.opens["DISPATCH"] = []string{"Edit Name", "Edit TGID"}
		client := device.New(fakeConn{reply: r.reply})

		got, err := frequency(context.Background(), client, "DISPATCH")
		if err != nil {
			t.Fatalf("frequency: %v", err)
		}
		if got != "" {
			t.Errorf("frequency is %q, wanted nothing for a talkgroup channel", got)
		}
	})

	// Verify a channel the list does not hold is reported
	t.Run("StepFails", func(t *testing.T) {
		r := walk()
		client := device.New(fakeConn{reply: r.reply})

		_, err := frequency(context.Background(), client, "MISSING")
		if err == nil || !strings.Contains(err.Error(), "looking for the channel") {
			t.Fatalf("frequency reported %v, wanted the missing channel", err)
		}
	})

	// Verify a channel that will not open is reported
	t.Run("OpenFails", func(t *testing.T) {
		r := walk()
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "KEY,E,P" {
				return "", errors.New("the scanner refused the key")
			}
			return r.reply(command)
		}})

		_, err := frequency(context.Background(), client, "DISPATCH")
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("frequency reported %v, wanted the refused press", err)
		}
	})

	// Verify a frequency screen that will not open is reported
	t.Run("ScreenFails", func(t *testing.T) {
		r := walk()
		presses := 0
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			// The first press opens the channel, and the second is the one
			// that opens its frequency screen.
			if command == "KEY,E,P" {
				if presses++; presses == 2 {
					return "", errors.New("the scanner refused the key")
				}
			}
			return r.reply(command)
		}})

		_, err := frequency(context.Background(), client, "DISPATCH")
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("frequency reported %v, wanted the refused press", err)
		}
	})

	// Verify a frequency screen that cannot be read is reported
	t.Run("ReadFails", func(t *testing.T) {
		r := walk()
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "MSI" {
				return "", errors.New("the port is gone")
			}
			return r.reply(command)
		}})

		_, err := frequency(context.Background(), client, "DISPATCH")
		if err == nil || !strings.Contains(err.Error(), "reading the frequency of") {
			t.Fatalf("frequency reported %v, wanted the failed read", err)
		}
	})
}

// Test_kind covers naming what is being typed.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Words: the two things a channel can be created on
func Test_kind(t *testing.T) {
	// Verify each of the two is named the way a person would name it
	t.Run("Words", func(t *testing.T) {
		if got := kind(true); got != "talkgroup" {
			t.Errorf("kind(true) is %q, wanted %q", got, "talkgroup")
		}
		if got := kind(false); got != "frequency" {
			t.Errorf("kind(false) is %q, wanted %q", got, "frequency")
		}
	})
}

// Test_listed covers naming the channels a department holds.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Names: the names in quotes, and a phrase when there are none
func Test_listed(t *testing.T) {
	// Verify the names are quoted and separated, and an empty department says
	// so in words
	t.Run("Names", func(t *testing.T) {
		got := listed([]channel{{Name: "DISPATCH"}, {Name: "FIREGROUND"}})
		if want := `"DISPATCH", "FIREGROUND"`; got != want {
			t.Errorf("listed is %q, wanted %q", got, want)
		}

		if got, want := listed(nil), "no channels at all"; got != want {
			t.Errorf("listed is %q, wanted %q", got, want)
		}
	})
}

// Test_read covers listing a department's channels for a confirming read.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Reads: the department is opened and its channels listed
//   - WalkFails: a department that cannot be reached is reported
//   - CollectFails: a channel list that cannot be read is reported
func Test_read(t *testing.T) {
	// walk builds a radio whose department menu leads to its channel list.
	walk := func() *radio {
		return &radio{
			top:   []string{"Edit Channel"},
			rows:  []string{"Edit Channel"},
			opens: map[string][]string{"Edit Channel": {"New Channel", "DISPATCH"}},
		}
	}

	// Verify the channels are listed and the scanner is left out of the menus
	t.Run("Reads", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		client := device.New(fakeConn{reply: r.reply})

		found, err := read(context.Background(), app, client, "0", true)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if len(found) != 1 || found[0].Name != "DISPATCH" {
			t.Errorf("read listed %+v, wanted the one channel", found)
		}
	})

	// Verify a department whose menu carries no channel list is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.top, r.rows = []string{"Edit Name"}, []string{"Edit Name"}
		client := device.New(fakeConn{reply: r.reply})

		_, err := read(context.Background(), app, client, "0", true)
		if err == nil || !strings.Contains(err.Error(), "Edit Channel") {
			t.Fatalf("read reported %v, wanted the missing channel list", err)
		}
	})

	// Verify a channel list that cannot be read is reported, and the scanner is
	// still taken out of the menus
	t.Run("CollectFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		reads := 0
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			// The screen is read once to find the channel list, and again once
			// it is open. The second read is the one that fails.
			if command == "STS" {
				if reads++; reads == 2 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}})

		_, err := read(context.Background(), app, client, "0", true)
		if err == nil || !strings.Contains(err.Error(), "reading the channel list") {
			t.Fatalf("read reported %v, wanted the failed read", err)
		}
	})
}

// Test_renderChannels covers writing the listing as a table.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Table: the channels are written with what each receives
//   - Names: the names alone are written when that is all that was asked for
//   - Empty: a department holding nothing says so and is not an error
//   - WriteFails: output that cannot be written is reported
func Test_renderChannels(t *testing.T) {
	// Verify the table names each channel and what it receives, with a dash for
	// a talkgroup channel that has no frequency of its own
	t.Run("Table", func(t *testing.T) {
		app, out, _ := quiet()
		found := []channel{
			{Name: "DISPATCH", Frequency: "153.980"},
			{Name: "FIREGROUND"},
		}

		if err := renderChannels(app, found, false); err != nil {
			t.Fatalf("renderChannels: %v", err)
		}
		for _, want := range []string{"RECEIVES", "DISPATCH", "153.980", "FIREGROUND", "-"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("renderChannels wrote %q, wanted %q in it", out.String(), want)
			}
		}
	})

	// Verify asking for names only leaves the column out altogether
	t.Run("Names", func(t *testing.T) {
		app, out, _ := quiet()

		if err := renderChannels(app, []channel{{Name: "DISPATCH"}}, true); err != nil {
			t.Fatalf("renderChannels: %v", err)
		}
		if strings.Contains(out.String(), "RECEIVES") {
			t.Errorf("renderChannels wrote %q, wanted the names alone", out.String())
		}
		if !strings.Contains(out.String(), "DISPATCH") {
			t.Errorf("renderChannels wrote %q, wanted the channel named in it", out.String())
		}
	})

	// Verify a department holding nothing is an answer rather than a failure
	t.Run("Empty", func(t *testing.T) {
		app, out, notes := quiet()

		if err := renderChannels(app, nil, false); err != nil {
			t.Fatalf("renderChannels: %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("renderChannels wrote %q, wanted nothing on the output", out.String())
		}
		if !strings.Contains(notes.String(), "no channels") {
			t.Errorf("the note is %q, wanted it to say there are none", notes.String())
		}
	})

	// Verify output that cannot be written is reported rather than swallowed
	t.Run("WriteFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.Stdout = failWriter{}

		err := renderChannels(app, []channel{{Name: "DISPATCH"}}, false)
		if err == nil || !strings.Contains(err.Error(), "writing the channel list") {
			t.Fatalf("renderChannels reported %v, wanted the failed write", err)
		}
	})
}

// Test_resolve covers turning a name or an index into a department index.
//
// Coverage: 100% (4 test cases covering every branch)
//

// Test_run covers reporting what is in a department.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Table: the channels are written for a person
//   - Names: asking for names only leaves the frequencies out of the answer
//   - JSON: the channels are written for a program
//   - NoDevice: a run with no scanner named is refused
//   - ResolveFails: a department the scanner does not have is reported
//   - ReadFails: a scanner that answers nothing at all is reported
func Test_run(t *testing.T) {
	docs := map[string]string{
		"GLT,CFREQ,0": `<GLT><CFREQ Name="DISPATCH" Index="0" Freq="153.980" Avoid="Off"/></GLT>`,
	}

	// Verify the channels reach the output as a table
	t.Run("Table", func(t *testing.T) {
		app, out, _ := quiet()
		r := &radio{docs: docs}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := run(context.Background(), app, "0", false); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out.String(), "153.980") {
			t.Errorf("run wrote %q, wanted the frequency in it", out.String())
		}
	})

	// Verify names only means what it says, rather than hiding a column
	t.Run("Names", func(t *testing.T) {
		app, out, _ := quiet()
		r := &radio{docs: docs}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := run(context.Background(), app, "0", true); err != nil {
			t.Fatalf("run: %v", err)
		}
		if strings.Contains(out.String(), "153.980") {
			t.Errorf("run wrote %q, wanted the frequency left out", out.String())
		}
	})

	// Verify the JSON output carries the channels as the scanner reported them
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		r := &radio{docs: docs}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := run(context.Background(), app, "0", false); err != nil {
			t.Fatalf("run: %v", err)
		}

		var got []channel
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if len(got) != 1 || got[0].Frequency != "153.980" {
			t.Errorf("run wrote %+v, wanted the one channel", got)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		if err := run(context.Background(), app, "0", false); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("run reported %v, wanted a missing device", err)
		}
	})

	// Verify a department the scanner does not have is reported
	t.Run("ResolveFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := &radio{docs: map[string]string{"GLT,FL": `<GLT></GLT>`}}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := run(context.Background(), app, "MISSING", false)
		if err == nil || !strings.Contains(err.Error(), "MISSING") {
			t.Fatalf("run reported %v, wanted the unknown department", err)
		}
	})

	// Verify a scanner that answers neither the protocol nor the menus is
	// reported
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := run(context.Background(), app, "0", false)
		if err == nil || !strings.Contains(err.Error(), "opening the department's menu") {
			t.Fatalf("run reported %v, wanted the failed read", err)
		}
	})
}

// Test_runDelete covers removing a channel and confirming it is gone.
//
// Coverage: 100% (12 test cases covering every branch)
//
// Test cases:
//   - Deletes: the channel is removed and the removal confirmed
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not list the department is reported
//   - Unknown: a channel the department does not hold is reported
//   - NeedsYes: a run without the flag that agrees to it changes nothing
//   - WalkFails: a department that cannot be reopened is reported
//   - ChannelMissing: a channel that has gone by the time of the walk is
//     reported
//   - EntryMissing: a channel menu without the delete entry is reported
//   - PromptMissing: a scanner asking something else is not answered
//   - LeaveFails: a scanner that will not leave its menus is reported
//   - ReadBackFails: a scanner that will not say what it holds is reported
//   - StillThere: a channel still on the scanner afterwards is reported
func Test_runDelete(t *testing.T) {
	// walk builds a radio whose department menu leads to a channel list, where
	// a channel's own menu carries Delete Channel and pressing it asks first.
	walk := func() *radio {
		return &radio{
			top:  []string{"Edit Channel"},
			rows: []string{"Edit Channel"},
			opens: map[string][]string{
				"Edit Channel":   {"New Channel", "DISPATCH"},
				"DISPATCH":       {"Edit Name", "Delete Channel"},
				"Delete Channel": {"Confirm Delete?"},
			},
		}
	}

	// Verify the channel is deleted and its absence afterwards is what confirms
	// it
	t.Run("Deletes", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			// Answering the prompt is what removes the channel, so the list
			// stops holding it from then on.
			confirmed := command == "KEY,E,P" && len(r.rows) > 0 && r.rows[r.cursor] == "Confirm Delete?"
			out, err := r.reply(command)
			if confirmed {
				r.opens["Edit Channel"] = []string{"New Channel"}
			}
			return out, err
		}}))

		if err := runDelete(context.Background(), app, "0", "DISPATCH", true); err != nil {
			t.Fatalf("runDelete: %v", err)
		}
		if got, want := out.String(), "deleted DISPATCH\n"; got != want {
			t.Errorf("runDelete wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runDelete reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not list the department is reported. The menu
	// opens, so the walk gets as far as reading the screen and fails there.
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("runDelete reported %v, wanted the failed read", err)
		}
	})

	// Verify a channel the department does not hold is reported, and names what
	// it does hold
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "0", "MISSING", true)
		if err == nil || !strings.Contains(err.Error(), `it holds "DISPATCH"`) {
			t.Fatalf("runDelete reported %v, wanted the unknown channel", err)
		}
	})

	// Verify nothing is deleted without the flag that agrees to it
	t.Run("NeedsYes", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "pass --yes") {
			t.Fatalf("runDelete reported %v, wanted the missing agreement", err)
		}
	})

	// Verify a department that cannot be reopened for the walk is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		opened := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			// The department is opened once to read it, and again to walk to
			// the channel. The second one fails.
			if strings.HasPrefix(command, "MNU,") {
				if opened++; opened == 2 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "opening the department's menu") {
			t.Fatalf("runDelete reported %v, wanted the failed walk", err)
		}
	})

	// Verify a channel that has gone between the reading and the walk is
	// reported rather than pressed at blindly
	t.Run("ChannelMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		opened := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				if opened++; opened == 2 {
					r.opens["Edit Channel"] = []string{"New Channel"}
				}
			}
			return r.reply(command)
		}}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "looking for the channel") {
			t.Fatalf("runDelete reported %v, wanted the vanished channel", err)
		}
	})

	// Verify a channel menu without a delete entry is reported
	t.Run("EntryMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.opens["DISPATCH"] = []string{"Edit Name", "Edit Frequency"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "on the channel's menu") {
			t.Fatalf("runDelete reported %v, wanted the missing entry", err)
		}
	})

	// Verify a scanner showing something other than the delete prompt is not
	// answered with a keypress
	t.Run("PromptMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.opens["Delete Channel"] = []string{"Saving..."}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "nothing was deleted") {
			t.Fatalf("runDelete reported %v, wanted the unanswered prompt", err)
		}
	})

	// Verify a scanner that stays on the prompt afterwards is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			confirmed := command == "KEY,E,P" && len(r.rows) > 0 && r.rows[r.cursor] == "Confirm Delete?"
			out, err := r.reply(command)
			if confirmed {
				// The scanner never comes off the prompt, so nothing gets it
				// out of the menus.
				r.title = "Confirm Delete?"
			}
			return out, err
		}}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Fatalf("runDelete reported %v, wanted the menus reported as stuck", err)
		}
	})

	// Verify a scanner that will not say what it holds afterwards is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		gone := false
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if gone && command == "STS" {
				return "", errors.New("the port is gone")
			}
			confirmed := command == "KEY,E,P" && len(r.rows) > 0 && r.rows[r.cursor] == "Confirm Delete?"
			out, err := r.reply(command)
			if confirmed {
				gone = true
			}
			return out, err
		}}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("runDelete reported %v, wanted the failed read back", err)
		}
	})

	// Verify a channel still on the scanner afterwards is not reported as
	// deleted
	t.Run("StillThere", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "0", "DISPATCH", true)
		if err == nil || !strings.Contains(err.Error(), "still there afterwards") {
			t.Fatalf("runDelete reported %v, wanted the channel reported as still there", err)
		}
	})
}

// Test_runNew covers creating a channel, setting what it receives, and naming
// it.
//
// Coverage: 100% (12 test cases covering every branch)
//
// Test cases:
//   - Creates: the channel is created on a frequency, named, and confirmed
//   - AlreadyThere: a frequency the scanner already has is refused at its own
//     prompt, rather than the prompt being walked into
//   - NoDevice: a run with no scanner named is refused
//   - EmptyTalkgroup: a prefix with nothing after it is refused
//   - WalkFails: a department that cannot be opened is reported
//   - CreateFails: a channel list without the entry that creates one is
//     reported
//   - WrongScreen: an address written for the other kind of department is
//     refused
//   - EntryFails: an address the screen will not accept is reported
//   - NameScreenFails: a new channel whose name screen will not open is
//     reported
//   - NamingFails: a name the screen will not accept is reported
//   - LeaveFails: a scanner that will not leave its menus is reported
//   - Missing: a channel that does not appear afterwards is reported
func Test_runNew(t *testing.T) {
	// walk builds a radio whose department menu leads to a channel list, where
	// New Channel opens a frequency screen and accepting it opens the new
	// channel's own menu.
	walk := func() *radio {
		return &radio{
			top:   []string{"Edit Channel"},
			rows:  []string{"Edit Channel"},
			opens: map[string][]string{"Edit Channel": {"New Channel", "DISPATCH"}},
			inputs: map[string]screen{
				"New Channel": {
					title: "Input Frequency",
					keys:  "0123456789.",
					opens: []string{"Edit Name", "Edit Frequency"},
				},
				"Edit Name": {title: "Name", value: "DISPATCH", keys: "ABCDEFGHIJKLMNOPQRSTUVWXYZ "},
			},
		}
	}

	// Verify the frequency is typed into the screen the scanner opened, the
	// name is accepted, and reading the department back confirms it
	t.Run("Creates", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false); err != nil {
			t.Fatalf("runNew: %v", err)
		}
		if got, want := out.String(), "DISPATCH\n"; got != want {
			t.Errorf("runNew wrote %q, wanted %q", got, want)
		}
	})

	// Verify that a frequency the scanner already has in reach is refused at the
	// prompt rather than walked into, which is what used to time out and leave
	// an unnamed channel behind
	t.Run("AlreadyThere", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.inputs["New Channel"] = screen{
			title: "Input Frequency",
			keys:  "0123456789.",
			opens: []string{"Frequency Exists Accept? (Y/N)"},
		}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "27.055", "CH 08", false)
		if err == nil || !strings.Contains(err.Error(), "--allow-duplicate") {
			t.Fatalf("runNew reported %v, wanted the duplicate refused and the flag named", err)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runNew reported %v, wanted a missing device", err)
		}
	})

	// Verify the talkgroup prefix with nothing after it is refused
	t.Run("EmptyTalkgroup", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "TGID:", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "no talkgroup was given") {
			t.Fatalf("runNew reported %v, wanted the empty talkgroup refused", err)
		}
	})

	// Verify a department that cannot be opened is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return r.reply(command)
		}}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "opening the department's menu") {
			t.Fatalf("runNew reported %v, wanted the failed walk", err)
		}
	})

	// Verify a channel list without the entry that creates one is reported
	t.Run("CreateFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.opens["Edit Channel"] = []string{"DISPATCH", "FIREGROUND"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "creating the channel") {
			t.Fatalf("runNew reported %v, wanted the missing entry", err)
		}
	})

	// Verify a frequency given to a department that takes talkgroups is refused
	// before anything is typed
	t.Run("WrongScreen", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		created := r.inputs["New Channel"]
		created.title = "Input TGID"
		r.inputs["New Channel"] = created
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "takes a talkgroup, not a frequency") {
			t.Fatalf("runNew reported %v, wanted the mismatch refused", err)
		}
	})

	// Verify a frequency the screen will not take is reported, and says nothing
	// was created. The address is checked before the scanner is touched now, so
	// the screen is the thing narrowed here rather than the frequency: this is
	// a scanner whose entry screen offers no decimal point, which is what a
	// firmware that took the key away would look like.
	t.Run("EntryFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		created := r.inputs["New Channel"]
		created.keys = "0123456789"
		r.inputs["New Channel"] = created
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "entering the frequency") {
			t.Fatalf("runNew reported %v, wanted the refused address", err)
		}
	})

	// Verify a channel created without a reachable name screen says it was
	// created
	t.Run("NameScreenFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		created := r.inputs["New Channel"]
		created.opens = []string{"Edit Frequency", "Delete Channel"}
		r.inputs["New Channel"] = created
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "its name screen could not be opened") {
			t.Fatalf("runNew reported %v, wanted the unopened name screen", err)
		}
	})

	// Verify a name holding a character the screen will not take says the
	// channel was created
	t.Run("NamingFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH!", false)
		if err == nil || !strings.Contains(err.Error(), "naming it failed") {
			t.Fatalf("runNew reported %v, wanted the refused name", err)
		}
	})

	// Verify a scanner that will not leave its menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.title = "Edit Name"
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Fatalf("runNew reported %v, wanted the menus reported as stuck", err)
		}
	})

	// Verify a channel that does not appear afterwards is reported rather than
	// assumed to have been created
	t.Run("Missing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.opens["Edit Channel"] = []string{"New Channel", "FIREGROUND"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "does not appear under") {
			t.Fatalf("runNew reported %v, wanted the missing channel", err)
		}
	})
}

// Test_runRename covers typing a new name into a channel.
//
// Coverage: 100% (11 test cases covering every branch)
//
// Test cases:
//   - Renames: the name screen is reached and the new name accepted
//   - Unchanged: a channel already called that is left alone
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not list the department is reported
//   - Unknown: a channel the department does not hold is reported
//   - WalkFails: a department that cannot be reopened is reported
//   - ChannelMissing: a channel that has gone by the time of the walk is
//     reported
//   - EntryMissing: a channel menu without the name entry is reported
//   - TypingFails: a name the screen will not accept says nothing was saved
//   - LeaveFails: a scanner that will not leave its menus is reported
//   - Missing: a channel that does not answer to the new name is reported
func Test_runRename(t *testing.T) {
	// walk builds a radio whose department menu leads to a channel list, where
	// a channel's own menu carries a name screen holding the new name.
	walk := func() *radio {
		return &radio{
			top:  []string{"Edit Channel"},
			rows: []string{"Edit Channel"},
			opens: map[string][]string{
				"Edit Channel": {"New Channel", "DISPATCH"},
				"DISPATCH":     {"Edit Name", "Delete Channel"},
			},
			inputs: map[string]screen{
				"Edit Name": {title: "Name", value: "NEW NAME", keys: "ABCDEFGHIJKLMNOPQRSTUVWXYZ "},
			},
		}
	}

	// Verify the name screen is reached and the new name is what gets reported.
	// The screen already holds the name, so accepting it is a single press.
	t.Run("Renames", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			// Accepting the name screen is what renames the channel, so the
			// list answers to the new name from then on.
			renamed := command == "KEY,E,P" && r.input
			out, err := r.reply(command)
			if renamed {
				r.opens["Edit Channel"] = []string{"New Channel", "NEW NAME"}
			}
			return out, err
		}}))

		if err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME"); err != nil {
			t.Fatalf("runRename: %v", err)
		}
		if got, want := out.String(), "NEW NAME\n"; got != want {
			t.Errorf("runRename wrote %q, wanted %q", got, want)
		}
	})

	// Verify a channel already called that is left alone, with the scanner left
	// scanning
	t.Run("Unchanged", func(t *testing.T) {
		app, out, notes := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runRename(context.Background(), app, "0", "DISPATCH", "DISPATCH"); err != nil {
			t.Fatalf("runRename: %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("runRename wrote %q, wanted nothing on the output", out.String())
		}
		if !strings.Contains(notes.String(), "already called") {
			t.Errorf("the note is %q, wanted it to say the name was already that", notes.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runRename reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not list the department is reported. The menu
	// opens, so the walk gets as far as reading the screen and fails there.
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("runRename reported %v, wanted the failed read", err)
		}
	})

	// Verify a channel the department does not hold is reported, and names what
	// it does hold
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "0", "MISSING", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), `it holds "DISPATCH"`) {
			t.Fatalf("runRename reported %v, wanted the unknown channel", err)
		}
	})

	// Verify a department that cannot be reopened for the walk is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		opened := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				if opened++; opened == 2 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "opening the department's menu") {
			t.Fatalf("runRename reported %v, wanted the failed walk", err)
		}
	})

	// Verify a channel that has gone between the reading and the walk is
	// reported
	t.Run("ChannelMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		opened := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				if opened++; opened == 2 {
					r.opens["Edit Channel"] = []string{"New Channel"}
				}
			}
			return r.reply(command)
		}}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "looking for the channel") {
			t.Fatalf("runRename reported %v, wanted the vanished channel", err)
		}
	})

	// Verify a channel menu without a name entry is reported, and says nothing
	// has been changed
	t.Run("EntryMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		r.opens["DISPATCH"] = []string{"Edit Frequency", "Delete Channel"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "Nothing has been changed") {
			t.Fatalf("runRename reported %v, wanted the missing entry", err)
		}
	})

	// Verify a name holding a character the screen will not take says nothing
	// has been saved
	t.Run("TypingFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME!")
		if err == nil || !strings.Contains(err.Error(), "nothing has been saved") {
			t.Fatalf("runRename reported %v, wanted the refused name", err)
		}
	})

	// Verify a scanner that will not leave its menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			renamed := command == "KEY,E,P" && r.input
			out, err := r.reply(command)
			if renamed {
				// The scanner never comes off the name screen, so nothing gets
				// it out of the menus.
				r.title = "Edit Name"
			}
			return out, err
		}}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Fatalf("runRename reported %v, wanted the menus reported as stuck", err)
		}
	})

	// Verify a channel that does not answer to the new name afterwards is
	// reported rather than assumed to have been renamed
	t.Run("Missing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk()
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "does not appear under") {
			t.Fatalf("runRename reported %v, wanted the unchanged name", err)
		}
	})
}

// Test_talkgroup covers reading the positional argument as an address.
//
// Coverage: 100% (1 test case covering every branch)
//
// Test cases:
//   - Addresses: what a person can type where an address is wanted
func Test_talkgroup(t *testing.T) {
	// Verify the prefix is recognised whatever its case, its value is trimmed,
	// and anything else is taken as a frequency
	t.Run("Addresses", func(t *testing.T) {
		cases := []struct {
			typed     string
			value     string
			talkgroup bool
		}{
			{"TGID:9051", "9051", true},

			// Typed by a person, and the scanner writes it capitalised.
			{"tgid: 9051", "9051", true},

			{"153.980", "153.980", false},

			// Shorter than the prefix, so there is nothing to compare.
			{"99", "99", false},
		}

		for _, c := range cases {
			value, got := talkgroup(c.typed)
			if value != c.value || got != c.talkgroup {
				t.Errorf("talkgroup(%q) is (%q, %v), wanted (%q, %v)",
					c.typed, value, got, c.value, c.talkgroup)
			}
		}
	})
}

// Test_walk covers reading a department's channels off the scanner's screen.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Reads: the channel list is walked and read
//   - OpenFails: a department that will not open is reported
//   - StepFails: a department menu without the channel list is reported
//   - EnterFails: a scanner that will not take the press is reported
//   - CollectFails: a channel list that cannot be read is reported
//   - LeaveFails: a scanner that will not leave its menus is reported
func Test_walk(t *testing.T) {
	// menu builds a radio whose department menu leads to a channel list.
	menu := func() *radio {
		return &radio{
			top:   []string{"Edit Channel"},
			rows:  []string{"Edit Channel"},
			opens: map[string][]string{"Edit Channel": {"New Channel", "DISPATCH"}},
		}
	}

	// Verify the walk reaches the channel list and reads what is on it
	t.Run("Reads", func(t *testing.T) {
		r := menu()
		client := device.New(fakeConn{reply: r.reply})

		found, err := walk(context.Background(), client, "0", true)
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		if len(found) != 1 || found[0].Name != "DISPATCH" {
			t.Errorf("walk read %+v, wanted the one channel", found)
		}
	})

	// Verify a department that will not open is reported
	t.Run("OpenFails", func(t *testing.T) {
		r := menu()
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return r.reply(command)
		}})

		_, err := walk(context.Background(), client, "0", true)
		if err == nil || !strings.Contains(err.Error(), "opening the department's menu") {
			t.Fatalf("walk reported %v, wanted the refused menu", err)
		}
	})

	// Verify a department menu carrying no channel list is reported
	t.Run("StepFails", func(t *testing.T) {
		r := menu()
		r.top, r.rows = []string{"Edit Name"}, []string{"Edit Name"}
		client := device.New(fakeConn{reply: r.reply})

		_, err := walk(context.Background(), client, "0", true)
		if err == nil || !strings.Contains(err.Error(), "on the department's menu") {
			t.Fatalf("walk reported %v, wanted the missing entry", err)
		}
	})

	// Verify a scanner that will not take the press is reported
	t.Run("EnterFails", func(t *testing.T) {
		r := menu()
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "KEY,E,P" {
				return "", errors.New("the scanner refused the key")
			}
			return r.reply(command)
		}})

		_, err := walk(context.Background(), client, "0", true)
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("walk reported %v, wanted the refused press", err)
		}
	})

	// Verify a channel list that cannot be read is reported, and the scanner is
	// still taken out of the menus
	t.Run("CollectFails", func(t *testing.T) {
		r := menu()
		reads := 0
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			// The screen is read once to find the channel list, and again once
			// it is open. The second read is the one that fails.
			if command == "STS" {
				if reads++; reads == 2 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}})

		_, err := walk(context.Background(), client, "0", true)
		if err == nil || !strings.Contains(err.Error(), "reading the channel list") {
			t.Fatalf("walk reported %v, wanted the failed read", err)
		}
	})

	// Verify a scanner that will not leave its menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		r := menu()
		r.title = "Edit Channel"
		client := device.New(fakeConn{reply: r.reply})

		_, err := walk(context.Background(), client, "0", true)
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Fatalf("walk reported %v, wanted the menus reported as stuck", err)
		}
	})
}

// Test_runNew_readBack covers the read that confirms a created channel, which
// runs after everything has already been written to the scanner.
//
// Coverage: the branch reporting a failed confirming read
//
// Test cases:
//   - ReadBackFails: a scanner that will not be reopened after the channel was
//     created is reported rather than treated as a success
func Test_runNew_readBack(t *testing.T) {
	// Verify a confirming read that fails is reported, even though the channel
	// itself was created and named without complaint
	t.Run("ReadBackFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := &radio{
			top:   []string{"Edit Channel"},
			rows:  []string{"Edit Channel"},
			opens: map[string][]string{"Edit Channel": {"New Channel", "DISPATCH"}},
			inputs: map[string]screen{
				"New Channel": {
					title: "Input Frequency",
					keys:  "0123456789.",
					opens: []string{"Edit Name", "Edit Frequency"},
				},
				"Edit Name": {title: "Name", value: "DISPATCH", keys: "ABCDEFGHIJKLMNOPQRSTUVWXYZ "},
			},
		}

		// The first menu opened is the walk that creates the channel. The
		// second is the read that confirms it, which is the one that fails.
		opened := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				if opened++; opened == 2 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}}))

		err := runNew(context.Background(), app, "0", "153.980", "DISPATCH", false)
		if err == nil || !strings.Contains(err.Error(), "opening the department's menu") {
			t.Fatalf("runNew reported %v, wanted the failed confirming read", err)
		}
	})
}

// Test_runRename_readBack covers the read that confirms a renamed channel,
// which runs after the new name has already been typed and saved.
//
// Coverage: the branch reporting a failed confirming read
//
// Test cases:
//   - ReadBackFails: a scanner that will not be reopened after the name was
//     typed is reported rather than treated as a success
func Test_runRename_readBack(t *testing.T) {
	// Verify a confirming read that fails is reported, even though the name was
	// typed and accepted without complaint
	t.Run("ReadBackFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := &radio{
			top:  []string{"Edit Channel"},
			rows: []string{"Edit Channel"},
			opens: map[string][]string{
				"Edit Channel": {"New Channel", "DISPATCH"},
				"DISPATCH":     {"Edit Name", "Delete Channel"},
			},
			inputs: map[string]screen{
				"Edit Name": {title: "Name", value: "NEW NAME", keys: "ABCDEFGHIJKLMNOPQRSTUVWXYZ "},
			},
		}

		// The first menu opened is the read that finds the channel, the second
		// is the walk that renames it, and the third is the read that confirms
		// it, which is the one that fails.
		opened := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				if opened++; opened == 3 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}}))

		err := runRename(context.Background(), app, "0", "DISPATCH", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "opening the department's menu") {
			t.Fatalf("runRename reported %v, wanted the failed confirming read", err)
		}
	})
}

// Test_accepted tests the accepted function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NoPrompt: an ordinary screen is left alone and costs one read
//   - Refused: the prompt is answered no and the command fails saying so
//   - Allowed: --allow-duplicate answers it yes and the command carries on
//   - ReadError: a screen that cannot be read is reported
//   - PressError: a no the scanner will not take says the prompt is still up
func Test_accepted(t *testing.T) {
	// Verify that the ordinary case, where nothing is being asked, does nothing.
	t.Run("NoPrompt", func(t *testing.T) {
		conn := fakeConn{reply: func(command string) (string, error) {
			return "0,Edit Name,****", nil
		}}

		if err := accepted(context.Background(), device.New(conn), false, "CB", "27.055"); err != nil {
			t.Fatalf("accepted() = %v, want nil", err)
		}
	})

	// Verify the reported bug: the prompt is answered rather than walked into,
	// and the caller is told the duplicate was refused.
	t.Run("Refused", func(t *testing.T) {
		var pressed []string
		conn := fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = append(pressed, command)
				return "", nil
			}
			if command == "MSI" {
				return `<MenuInfo MenuType="TypeError"/>`, nil
			}
			if command == "GSI" {
				return `<ScannerInfo Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			return "0,Frequency Exists Accept?,****", nil
		}}

		err := accepted(context.Background(), device.New(conn), false, "CB Channels", "27.055")
		if err == nil {
			t.Fatal("expected an error when the frequency is already there, got none")
		}
		if !strings.Contains(err.Error(), "--allow-duplicate") {
			t.Errorf("accepted() = %v, want it to name the flag that says yes", err)
		}

		want := fmt.Sprintf("KEY,%s,%s", device.KeyNo, device.KeyPress)
		if len(pressed) == 0 || pressed[0] != want {
			t.Errorf("pressed %v, want the no key answered first", pressed)
		}
	})

	// Verify that somebody who asked for the duplicate gets it, and that the
	// command carries on rather than failing.
	t.Run("Allowed", func(t *testing.T) {
		var pressed []string
		conn := fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = append(pressed, command)
				return "", nil
			}
			return "0,Frequency Exists Accept?,****", nil
		}}

		if err := accepted(context.Background(), device.New(conn), true, "CB Channels", "27.055"); err != nil {
			t.Fatalf("accepted() = %v, want nil", err)
		}

		want := fmt.Sprintf("KEY,%s,%s", device.KeyEnter, device.KeyPress)
		if len(pressed) != 1 || pressed[0] != want {
			t.Errorf("pressed %v, want the yes key and nothing else", pressed)
		}
	})

	// Verify that a screen that cannot be read is reported rather than assumed
	// to be free of the prompt.
	t.Run("ReadError", func(t *testing.T) {
		conn := fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if err := accepted(context.Background(), device.New(conn), false, "CB", "27.055"); err == nil {
			t.Error("expected an error when the screen cannot be read, got none")
		}
	})

	// Verify that a no the scanner will not take leaves the reader knowing the
	// prompt is still on screen and how to clear it by hand.
	t.Run("PressError", func(t *testing.T) {
		conn := fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the key was not taken")
			}
			return "0,Frequency Exists Accept?,****", nil
		}}

		err := accepted(context.Background(), device.New(conn), false, "CB", "27.055")
		if err == nil {
			t.Fatal("expected an error when the no key is refused, got none")
		}
		if !strings.Contains(err.Error(), "still asking") {
			t.Errorf("accepted() = %v, want it to say the prompt is still up", err)
		}
	})
}

// Test_partial tests the partial function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Some: a channel read off the menus alone is noticed
//   - None: a list read entirely from the scanner's own list is not
//   - Empty: a department holding nothing has nothing missing
func Test_partial(t *testing.T) {
	// Verify that one name-only channel is enough to say the read is not whole.
	t.Run("Some", func(t *testing.T) {
		if !partial([]catalog.Channel{{Name: "CH 01"}, {Name: "CH 02", Partial: true}}) {
			t.Error("a name-only channel was not noticed")
		}
	})

	// Verify that a list the scanner reported in full is not reported as partly
	// unknown, which would send the command down the slow path for nothing.
	t.Run("None", func(t *testing.T) {
		if partial([]catalog.Channel{{Name: "CH 01"}}) {
			t.Error("a complete list was reported as partly unknown")
		}
	})

	// Verify that an empty department is not partly unknown either.
	t.Run("Empty", func(t *testing.T) {
		if partial(nil) {
			t.Error("an empty department was reported as partly unknown")
		}
	})
}
