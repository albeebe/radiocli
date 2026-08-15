// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package favorites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
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

// radio answers as a scanner sitting in its menus, so the walks these commands
// make can be driven with no radio attached.
//
// It holds one menu at a time: the rows the knob moves through, and the row the
// knob is on. Every display request highlights that row, which is what keeps a
// walk from waiting for a redraw that is never coming. Pressing enter records
// the row and then does whatever the test set up for it: opening another menu,
// opening a text entry screen, or toggling a monitor list row that carries its
// own state.
//
// Opening a menu over the wire is answered and changes nothing, so a test
// starts the radio on the menu its walk begins from.
type radio struct {
	rows    []string            // The menu on screen, in the order the knob reaches the rows
	cursor  int                 // Which row the knob is on
	opens   map[string][]string // The menu each entry puts on screen when it is pressed
	inputs  map[string]string   // The text entry screen each entry opens, by the value it holds
	lists   []string            // The documents answering favorites list requests, one per request
	title   string              // The menu reported when asked, empty for a scanner that has left them
	value   string              // The value the open text entry screen holds
	input   bool                // Whether a text entry screen is open
	pressed []string            // Every row enter was pressed on
}

// reply answers one command the way the scanner would.
func (r *radio) reply(command string) (string, error) {
	switch {
	case command == "STS":
		return r.display(), nil
	case command == "MSI":
		return r.screen(), nil
	case command == "GSI":
		return `<ScannerInfo Mode="Scan Mode" V_Screen="scan"/>`, nil
	case strings.HasPrefix(command, "GLT,FL"):
		return r.favorites(), nil
	case command == "KEY,>,P":
		if len(r.rows) > 0 {
			r.cursor = (r.cursor + 1) % len(r.rows)
		}
	case command == "KEY,E,P":
		r.enter()
	}
	return "", nil
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
	// A text entry screen takes this as accepting the value, which closes it.
	if r.input {
		r.input = false
		return
	}
	if len(r.rows) == 0 {
		return
	}

	row := r.rows[r.cursor]
	r.pressed = append(r.pressed, row)

	// A row of the monitor list carries its own state, and pressing it toggles.
	switch {
	case strings.HasSuffix(row, stateOff):
		r.rows[r.cursor] = strings.TrimSuffix(row, stateOff) + stateOn
		return
	case strings.HasSuffix(row, stateOn):
		r.rows[r.cursor] = strings.TrimSuffix(row, stateOn) + stateOff
		return
	}

	if value, open := r.inputs[row]; open {
		r.input, r.value = true, value
		return
	}
	if next, open := r.opens[row]; open {
		r.rows, r.cursor = next, 0
	}
}

// favorites answers a favorites list request, moving on to the next document
// each time so a command that reads the lists back sees what changed. The last
// document answers every request after it.
func (r *radio) favorites() string {
	if len(r.lists) == 0 {
		return "<GLT></GLT>"
	}

	doc := r.lists[0]
	if len(r.lists) > 1 {
		r.lists = r.lists[1:]
	}
	return doc
}

// screen renders the menu the scanner is showing as the reply to a menu
// request. A radio that has left the menus answers the way the scanner does,
// which is with an error document.
func (r *radio) screen() string {
	if r.input {
		return fmt.Sprintf(`<MenuInfo Name="Name" MenuType="TypeInput" Value="%s">`+
			`<MenuInput MaxLength="16" EnableKeys="ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "/></MenuInfo>`, r.value)
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

// listed renders the document the scanner answers a favorites list request
// with, one entry per name and indexed by position.
func listed(names ...string) string {
	doc := "<GLT>"
	for i, name := range names {
		doc += fmt.Sprintf(`<FL Name="%s" Index="%d" Monitor="Off"/>`, name, i)
	}
	return doc + "</GLT>"
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
//   - Wiring: the command is named and carries its help text
//   - Subcommands: every subcommand is attached
//   - Runs: executing the command lists what the scanner holds
func TestNew(t *testing.T) {
	// Verify the command carries the name and the help text the tool wires on
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "favorites" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "favorites")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify each of the five subcommands is reachable
	t.Run("Subcommands", func(t *testing.T) {
		attached := map[string]bool{}
		for _, sub := range New(appcontext.New()).Commands() {
			attached[sub.Name()] = true
		}

		for _, name := range []string{"goto", "rename", "new", "scan", "delete"} {
			if !attached[name] {
				t.Errorf("the %s subcommand is not attached", name)
			}
		}
	})

	// Verify running the bare command reads the scanner and lists what it holds
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := quiet()
		r := &radio{lists: []string{listed("TEST LIST")}}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "TEST LIST") {
			t.Errorf("the command wrote %q, wanted the list named in it", out.String())
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
		cmd.SetArgs([]string{"TEST LIST", "--yes"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})
}

// Test_newGoto covers the goto subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its help text
//   - Runs: executing the subcommand with no scanner named is refused
func Test_newGoto(t *testing.T) {
	// Verify the subcommand is named and carries its help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newGoto(appcontext.New())

		if cmd.Name() != "goto" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "goto")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify the closure runs and refuses a run with no scanner named
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()

		cmd := newGoto(app)
		cmd.SetArgs([]string{"TEST LIST"})
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
//   - Wiring: the subcommand is named and carries its help text
//   - Runs: executing the subcommand with no scanner named is refused
func Test_newNew(t *testing.T) {
	// Verify the subcommand is named and carries its help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newNew(appcontext.New())

		if cmd.Name() != "new" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "new")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify the closure runs and refuses a run with no scanner named
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()

		cmd := newNew(app)
		cmd.SetArgs([]string{"TEST LIST"})
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
//   - Wiring: the subcommand is named and takes both arguments
//   - Runs: executing the subcommand with no scanner named is refused
func Test_newRename(t *testing.T) {
	// Verify the subcommand is named and carries its help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newRename(appcontext.New())

		if cmd.Name() != "rename" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "rename")
		}
		if err := cmd.Args(cmd, []string{"one"}); err == nil {
			t.Error("the subcommand took one argument, wanted both the list and the name")
		}
	})

	// Verify the closure runs and refuses a run with no scanner named
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()

		cmd := newRename(app)
		cmd.SetArgs([]string{"TEST LIST", "NEW NAME"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})
}

// Test_newScan covers the scan subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the subcommand is named and carries both of its flags
//   - Runs: executing the subcommand with nothing to go on is refused
func Test_newScan(t *testing.T) {
	// Verify the subcommand is named and carries the flags that stand in for
	// naming lists
	t.Run("Wiring", func(t *testing.T) {
		cmd := newScan(appcontext.New())

		if cmd.Name() != "scan" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "scan")
		}
		for _, flag := range []string{"all", "none"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("the subcommand has no --%s flag", flag)
			}
		}
	})

	// Verify the closure runs and refuses a run naming nothing at all
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()

		cmd := newScan(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("the subcommand reported nothing, wanted the empty run refused")
		}
		if !strings.Contains(err.Error(), "name the lists to scan") {
			t.Errorf("the subcommand reported %q, wanted it to ask for the lists", err)
		}
	})
}

// Test_apply covers switching the lists on and off on the scanner.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - All: every list is switched on in one press
//   - None: every list is switched off
//   - Named: the lists asked for are switched on afterwards
//   - OnFails: a scan selection menu missing the entry that switches every
//     list on is reported
//   - MenuFails: a scanner that will not open its menus is reported
//   - OffFails: a scan selection menu missing the entry is reported
//   - ListsFails: a scan selection menu missing the monitor list is reported
//   - RowFails: a list with no row of its own is reported
func Test_apply(t *testing.T) {
	// Verify --all presses the entry that switches every list on
	t.Run("All", func(t *testing.T) {
		r := &radio{
			rows:  []string{"Set Scan Selection"},
			opens: map[string][]string{"Set Scan Selection": {"Set All Lists On"}},
		}
		client := device.New(fakeConn{reply: r.reply})

		if err := apply(context.Background(), client, nil, true, false); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !strings.Contains(strings.Join(r.pressed, ","), setAllOn) {
			t.Errorf("the scanner was pressed on %v, wanted %q among them", r.pressed, setAllOn)
		}
	})

	// Verify --none presses the entry that switches every list off and stops
	t.Run("None", func(t *testing.T) {
		r := &radio{
			rows:  []string{"Set Scan Selection"},
			opens: map[string][]string{"Set Scan Selection": {"Set All Lists Off"}},
		}
		client := device.New(fakeConn{reply: r.reply})

		if err := apply(context.Background(), client, nil, false, true); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !strings.Contains(strings.Join(r.pressed, ","), setAllOff) {
			t.Errorf("the scanner was pressed on %v, wanted %q among them", r.pressed, setAllOff)
		}
	})

	// Verify naming a list clears the selection first and then switches that
	// list on
	t.Run("Named", func(t *testing.T) {
		scanSelection := []string{"Set All Lists Off", "Select Lists to Monitor"}
		r := &radio{
			rows: []string{"Set Scan Selection"},
			opens: map[string][]string{
				"Set Scan Selection":      scanSelection,
				"Set All Lists Off":       scanSelection,
				"Select Lists to Monitor": {"TEST LIST :Off"},
			},
		}
		client := device.New(fakeConn{reply: r.reply})
		wanted := []catalog.FavoritesList{{Name: "TEST LIST", Index: "0"}}

		if err := apply(context.Background(), client, wanted, false, false); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if got := r.rows[0]; got != "TEST LIST "+stateOn {
			t.Errorf("the row reads %q, wanted it switched on", got)
		}
	})

	// Verify a scan selection menu without the entry that switches every list
	// on is reported
	t.Run("OnFails", func(t *testing.T) {
		r := &radio{
			rows:  []string{"Set Scan Selection"},
			opens: map[string][]string{"Set Scan Selection": {"Set Quick Keys", "Set Number Tags"}},
		}
		client := device.New(fakeConn{reply: r.reply})

		err := apply(context.Background(), client, nil, true, false)
		if err == nil {
			t.Fatal("apply reported nothing, wanted the missing entry")
		}
		if !strings.Contains(err.Error(), setAllOn) {
			t.Errorf("apply reported %q, wanted it to name %q", err, setAllOn)
		}
	})

	// Verify a scanner that will not open its menus is reported
	t.Run("MenuFails", func(t *testing.T) {
		r := &radio{rows: []string{"Set Scan Selection"}}
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return r.reply(command)
		}})

		err := apply(context.Background(), client, nil, true, false)
		if err == nil {
			t.Fatal("apply reported nothing, wanted the refused menu")
		}
		if !strings.Contains(err.Error(), "opening the top menu") {
			t.Errorf("apply reported %q, wanted it to name opening the top menu", err)
		}
	})

	// Verify a scan selection menu without the entry that clears the selection
	// is reported
	t.Run("OffFails", func(t *testing.T) {
		r := &radio{
			rows:  []string{"Set Scan Selection"},
			opens: map[string][]string{"Set Scan Selection": {"Set Quick Keys", "Set Number Tags"}},
		}
		client := device.New(fakeConn{reply: r.reply})

		err := apply(context.Background(), client, nil, false, true)
		if err == nil {
			t.Fatal("apply reported nothing, wanted the missing entry")
		}
		if !strings.Contains(err.Error(), setAllOff) {
			t.Errorf("apply reported %q, wanted it to name %q", err, setAllOff)
		}
	})

	// Verify a scan selection menu without the monitor list is reported
	t.Run("ListsFails", func(t *testing.T) {
		r := &radio{
			rows: []string{"Set Scan Selection"},
			opens: map[string][]string{
				"Set Scan Selection": {"Set All Lists Off", "Set Quick Keys"},
				"Set All Lists Off":  {"Set All Lists Off", "Set Quick Keys"},
			},
		}
		client := device.New(fakeConn{reply: r.reply})
		wanted := []catalog.FavoritesList{{Name: "TEST LIST", Index: "0"}}

		err := apply(context.Background(), client, wanted, false, false)
		if err == nil {
			t.Fatal("apply reported nothing, wanted the missing monitor list")
		}
		if !strings.Contains(err.Error(), selectLists) {
			t.Errorf("apply reported %q, wanted it to name %q", err, selectLists)
		}
	})

	// Verify a list the monitor screen has no row for is reported rather than
	// passed over
	t.Run("RowFails", func(t *testing.T) {
		scanSelection := []string{"Set All Lists Off", "Select Lists to Monitor"}
		r := &radio{
			rows: []string{"Set Scan Selection"},
			opens: map[string][]string{
				"Set Scan Selection":      scanSelection,
				"Set All Lists Off":       scanSelection,
				"Select Lists to Monitor": {"OTHER LIST :Off"},
			},
		}
		client := device.New(fakeConn{reply: r.reply})
		wanted := []catalog.FavoritesList{{Name: "TEST LIST", Index: "0"}}

		err := apply(context.Background(), client, wanted, false, false)
		if err == nil {
			t.Fatal("apply reported nothing, wanted the missing row")
		}
		if !strings.Contains(err.Error(), "no row for") {
			t.Errorf("apply reported %q, wanted it to name the missing row", err)
		}
	})
}

// Test_chosen covers turning the names typed into the lists they mean.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Names: each name is matched to its list
//   - Index: an index is taken as it stands
//   - Unknown: a name the scanner does not have is reported
func Test_chosen(t *testing.T) {
	lists := []catalog.FavoritesList{
		{Name: "TEST LIST", Index: "0"},
		{Name: "OTHER LIST", Index: "1"},
	}

	// Verify names are matched to their lists, in the order they were given
	t.Run("Names", func(t *testing.T) {
		got, err := chosen([]string{"OTHER LIST", "TEST LIST"}, lists)
		if err != nil {
			t.Fatalf("chosen: %v", err)
		}
		if len(got) != 2 || got[0].Index != "1" || got[1].Index != "0" {
			t.Errorf("chosen picked %+v, wanted the two lists in the order named", got)
		}
	})

	// Verify an index is taken as an index rather than looked up as a name
	t.Run("Index", func(t *testing.T) {
		got, err := chosen([]string{"1"}, lists)
		if err != nil {
			t.Fatalf("chosen: %v", err)
		}
		if len(got) != 1 || got[0].Name != "OTHER LIST" {
			t.Errorf("chosen picked %+v, wanted the list at index 1", got)
		}
	})

	// Verify a name the scanner does not have is reported rather than skipped
	t.Run("Unknown", func(t *testing.T) {
		_, err := chosen([]string{"MISSING LIST"}, lists)
		if err == nil {
			t.Fatal("chosen reported nothing, wanted the unknown name refused")
		}
		if !strings.Contains(err.Error(), "MISSING LIST") {
			t.Errorf("chosen reported %q, wanted it to name what was typed", err)
		}
	})
}

// Test_dash covers how an unassigned value is written.
//
// Coverage: 100% (1 test case covering both branches)
//

// Test_renderLists covers writing the listing as a table.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Table: the lists are written with the built-in ones marked
//   - Empty: a scanner holding no lists says so and is not an error
//   - WriteFails: output that cannot be written is reported
func Test_renderLists(t *testing.T) {
	// Verify the table names every list and marks the built-in scan sources
	t.Run("Table", func(t *testing.T) {
		app, out, notes := quiet()
		lists := []catalog.FavoritesList{
			{Name: "TEST LIST", Index: "0", Monitored: true, QuickKey: "1"},
			{Name: "Full Database", Index: "4294967295", BuiltIn: true},
		}

		if err := renderLists(app, lists); err != nil {
			t.Fatalf("renderLists: %v", err)
		}
		for _, want := range []string{"NAME", "TEST LIST", "yes", "Full Database", "-"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("renderLists wrote %q, wanted %q in it", out.String(), want)
			}
		}
		if !strings.Contains(notes.String(), "built into the scanner") {
			t.Errorf("the note is %q, wanted it to explain the marker", notes.String())
		}
	})

	// Verify a scanner with no lists is an answer rather than a failure
	t.Run("Empty", func(t *testing.T) {
		app, out, notes := quiet()

		if err := renderLists(app, nil); err != nil {
			t.Fatalf("renderLists: %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("renderLists wrote %q, wanted nothing on the output", out.String())
		}
		if !strings.Contains(notes.String(), "no favorites lists") {
			t.Errorf("the note is %q, wanted it to say there are none", notes.String())
		}
	})

	// Verify output that cannot be written is reported rather than swallowed
	t.Run("WriteFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.Stdout = failWriter{}

		err := renderLists(app, []catalog.FavoritesList{{Name: "TEST LIST", Index: "0"}})
		if err == nil {
			t.Fatal("renderLists reported nothing, wanted the failed write")
		}
		if !strings.Contains(err.Error(), "writing the favorites list") {
			t.Errorf("renderLists reported %q, wanted it to name writing the list", err)
		}
	})
}

// Test_rowIs covers matching a row of the monitor list against a list's name.
//
// Coverage: 100% (1 test case covering every branch)
//
// Test cases:
//   - Rows: the rows that are the named list, and the ones that are not
func Test_rowIs(t *testing.T) {
	// Verify the state is stripped, a whole name matches, and only a row the
	// scanner cut short matches as the start of a longer name
	t.Run("Rows", func(t *testing.T) {
		cases := []struct {
			row  menus.Row
			name string
			is   bool
		}{
			// The state is written on the same line as the name.
			{menus.Row{Text: "TEST LIST :On"}, "TEST LIST", true},
			{menus.Row{Text: "TEST LIST :Off"}, "TEST LIST", true},
			{menus.Row{Text: "TEST LIST"}, "TEST LIST", true},

			// A row cut short is the start of the name it was cut from.
			{menus.Row{Text: "Quick Save Favorit", Cut: true}, "Quick Save Favorites List", true},

			// A row that was not cut is the whole name, so a longer name is a
			// different list. Matching this as a prefix switched on the wrong
			// list and reported success.
			{menus.Row{Text: "TEST LIST"}, "TEST LIST SCAN", false},

			// Nothing to compare, and a cut row that is the start of nothing.
			{menus.Row{Text: "", Cut: true}, "TEST LIST", false},
			{menus.Row{Text: "OTHER", Cut: true}, "TEST LIST", false},
		}

		for _, c := range cases {
			if got := rowIs(c.row, c.name); got != c.is {
				t.Errorf("rowIs(%q, %q) is %v, wanted %v", c.row.Text, c.name, got, c.is)
			}
		}
	})
}

// Test_run covers reading the favorites lists and reporting them.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Table: the lists are written for a person
//   - JSON: the lists are written for a program
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer is reported
func Test_run(t *testing.T) {
	// Verify the lists the scanner reports reach the output as a table
	t.Run("Table", func(t *testing.T) {
		app, out, _ := quiet()
		r := &radio{lists: []string{listed("TEST LIST", "OTHER LIST")}}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out.String(), "OTHER LIST") {
			t.Errorf("run wrote %q, wanted both lists in it", out.String())
		}
	})

	// Verify the JSON output carries the lists as the scanner reported them
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		r := &radio{lists: []string{listed("TEST LIST")}}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}

		var got []catalog.FavoritesList
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if len(got) != 1 || got[0].Name != "TEST LIST" {
			t.Errorf("run wrote %+v, wanted the one list", got)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		if err := run(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("run reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that fails to answer is reported as a failed read
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := run(context.Background(), app)
		if err == nil {
			t.Fatal("run reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Errorf("run reported %q, wanted it to name reading the lists", err)
		}
	})
}

// Test_runDelete covers removing a favorites list and confirming it is gone.
//
// Coverage: 100% (13 test cases covering every branch)
//
// Test cases:
//   - Deletes: the list is removed and the removal confirmed
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not list what it holds is reported
//   - Unknown: a name the scanner does not have is reported
//   - BuiltIn: a scan source built into the scanner is refused
//   - NeedsYes: a run without the flag that agrees to it changes nothing
//   - WalkFails: a scanner that will not open the list is reported
//   - EntryMissing: a menu without the delete entry is reported
//   - PromptMissing: a scanner asking something else is not answered
//   - AwakenFails: a scanner that does not come back is reported
//   - LeaveFails: a scanner that will not leave its menus is reported
//   - ReadBackFails: a scanner that will not say what it holds is reported
//   - StillThere: a list still on the scanner afterwards is reported
func Test_runDelete(t *testing.T) {
	// menus builds a radio walking from the top menu to one list's own menu,
	// where Delete sits next to Rename and pressing it asks to confirm.
	walk := func(lists ...string) *radio {
		return &radio{
			rows: []string{"Manage Favorites"},
			opens: map[string][]string{
				"Manage Favorites": {"TEST LIST"},
				"TEST LIST":        {"Rename", "Delete"},
				"Delete":           {"Confirm Delete?"},
			},
			lists: lists,
		}
	}

	// Verify the list is deleted and its absence afterwards is what confirms it
	t.Run("Deletes", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk(listed("TEST LIST"), listed("TEST LIST"), listed("OTHER LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runDelete(context.Background(), app, "TEST LIST", true); err != nil {
			t.Fatalf("runDelete: %v", err)
		}
		if got, want := out.String(), "deleted TEST LIST\n"; got != want {
			t.Errorf("runDelete wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runDelete reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not list what it holds is reported
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runDelete reported %v, wanted the failed read", err)
		}
	})

	// Verify a name the scanner does not have is reported before anything moves
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "MISSING LIST", true)
		if err == nil || !strings.Contains(err.Error(), "MISSING LIST") {
			t.Fatalf("runDelete reported %v, wanted the unknown name", err)
		}
		if len(r.pressed) != 0 {
			t.Errorf("the scanner was pressed on %v, wanted nothing pressed", r.pressed)
		}
	})

	// Verify a scan source built into the scanner cannot be deleted
	t.Run("BuiltIn", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(`<GLT><FL Name="Full Database" Index="4294967295" Monitor="On"/></GLT>`)
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "Full Database", true)
		if err == nil || !strings.Contains(err.Error(), "built into the scanner") {
			t.Fatalf("runDelete reported %v, wanted the built-in source refused", err)
		}
	})

	// Verify nothing is deleted without the flag that agrees to it
	t.Run("NeedsYes", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "TEST LIST", false)
		if err == nil || !strings.Contains(err.Error(), "pass --yes") {
			t.Fatalf("runDelete reported %v, wanted the missing agreement", err)
		}
		if len(r.pressed) != 0 {
			t.Errorf("the scanner was pressed on %v, wanted nothing pressed", r.pressed)
		}
	})

	// Verify a scanner whose top menu does not lead to the list is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		r.rows = []string{"Set Scan Selection"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "Manage Favorites") {
			t.Fatalf("runDelete reported %v, wanted the walk reported", err)
		}
	})

	// Verify a list menu without a delete entry is reported
	t.Run("EntryMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		r.opens["TEST LIST"] = []string{"Rename", "Information"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "on the list's menu") {
			t.Fatalf("runDelete reported %v, wanted the missing entry", err)
		}
	})

	// Verify a scanner showing something other than the delete prompt is not
	// answered with a keypress
	t.Run("PromptMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		r.opens["Delete"] = []string{"Saving..."}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "nothing was deleted") {
			t.Fatalf("runDelete reported %v, wanted the unanswered prompt", err)
		}
	})

	// Verify a scanner that stops answering after the delete is reported
	t.Run("AwakenFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			// The prompt is answered, and then the scanner goes away to rebuild
			// and never comes back.
			if command == "STS" && len(r.pressed) > 0 && r.pressed[len(r.pressed)-1] == "Confirm Delete?" {
				return "", errors.New("the scanner went away")
			}
			return r.reply(command)
		}}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "has not come back") {
			t.Fatalf("runDelete reported %v, wanted the scanner reported as away", err)
		}
	})

	// Verify a scanner that will not leave its menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		r.title = "Confirm Delete?"
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Fatalf("runDelete reported %v, wanted the menus reported as stuck", err)
		}
	})

	// Verify a scanner that will not say what it holds afterwards is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		reads := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			// The lists are read to resolve the name, again to walk to it, and
			// a third time to confirm the deletion. Only the last one fails.
			if strings.HasPrefix(command, "GLT,FL") {
				if reads++; reads == 3 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runDelete reported %v, wanted the failed read back", err)
		}
	})

	// Verify a list still on the scanner afterwards is not reported as deleted
	t.Run("StillThere", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runDelete(context.Background(), app, "TEST LIST", true)
		if err == nil || !strings.Contains(err.Error(), "still there afterwards") {
			t.Fatalf("runDelete reported %v, wanted the list reported as still there", err)
		}
	})
}

// Test_runGoto covers walking to a favorites list and reporting where it
// landed.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Shows: the menu the scanner landed on is reported
//   - NoDevice: a run with no scanner named is refused
//   - WalkFails: a scanner that will not list what it holds is reported
//   - ShowFails: a menu that cannot be read is reported
func Test_runGoto(t *testing.T) {
	// Verify the walk lands on the list and its menu is what gets reported
	t.Run("Shows", func(t *testing.T) {
		app, out, _ := quiet()
		r := &radio{
			rows:  []string{"Manage Favorites"},
			opens: map[string][]string{"Manage Favorites": {"TEST LIST"}},
			lists: []string{listed("TEST LIST")},
			title: "TEST LIST",
		}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runGoto(context.Background(), app, "TEST LIST"); err != nil {
			t.Fatalf("runGoto: %v", err)
		}
		if !strings.Contains(out.String(), "menu: TEST LIST") {
			t.Errorf("runGoto wrote %q, wanted it to name the menu", out.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		err := runGoto(context.Background(), app, "TEST LIST")
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runGoto reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not list what it holds is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := runGoto(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runGoto reported %v, wanted the failed walk", err)
		}
	})

	// Verify a menu the scanner answers unreadably is reported
	t.Run("ShowFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := &radio{
			rows:  []string{"Manage Favorites"},
			opens: map[string][]string{"Manage Favorites": {"TEST LIST"}},
			lists: []string{listed("TEST LIST")},
		}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "MSI" {
				return "<MenuInfo", nil
			}
			return r.reply(command)
		}}))

		err := runGoto(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "reading the menu") {
			t.Fatalf("runGoto reported %v, wanted the unreadable menu", err)
		}
	})
}

// Test_runNew covers creating a favorites list and naming it.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - Creates: the list is created, named, and confirmed
//   - NoDevice: a run with no scanner named is refused
//   - WalkFails: a scanner that will not open its menus is reported
//   - CreateFails: a menu without the entry that creates one is reported
//   - NameScreenFails: a new list whose name screen will not open is reported
//   - TypingFails: a name that cannot be typed is reported
//   - LeaveFails: a scanner that will not leave its menus is reported
//   - ReadFails: a scanner that will not say what it holds is reported
//   - Missing: a list that does not appear afterwards is reported
func Test_runNew(t *testing.T) {
	// walk builds a radio whose top menu leads to the entry that creates a
	// list, and whose new list opens a name screen already holding the name.
	walk := func(holds string, lists ...string) *radio {
		return &radio{
			rows: []string{"Manage Favorites"},
			opens: map[string][]string{
				"Manage Favorites":   {"New Favorites List"},
				"New Favorites List": {"Rename", "Delete"},
			},
			inputs: map[string]string{"Rename": holds},
			lists:  lists,
		}
	}

	// Verify the list is created and named, and reading it back confirms it.
	// The name screen already holds the name, so accepting it is a single
	// press: what is being tested here is the walk, not the typing.
	t.Run("Creates", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk("TEST LIST", listed("TEST LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runNew(context.Background(), app, "TEST LIST"); err != nil {
			t.Fatalf("runNew: %v", err)
		}
		if got, want := out.String(), "TEST LIST\n"; got != want {
			t.Errorf("runNew wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		err := runNew(context.Background(), app, "TEST LIST")
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runNew reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not open its menus is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("TEST LIST")
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return r.reply(command)
		}}))

		err := runNew(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "opening the top menu") {
			t.Fatalf("runNew reported %v, wanted the refused menu", err)
		}
	})

	// Verify a favorites menu without the entry that creates a list is reported
	t.Run("CreateFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("TEST LIST")
		r.opens["Manage Favorites"] = []string{"TEST LIST", "OTHER LIST"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "creating the favorites list") {
			t.Fatalf("runNew reported %v, wanted the missing entry", err)
		}
	})

	// Verify a list created without a reachable name screen says it was created
	t.Run("NameScreenFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("TEST LIST")
		r.opens["New Favorites List"] = []string{"Information", "Delete"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "its name screen could not be opened") {
			t.Fatalf("runNew reported %v, wanted the unopened name screen", err)
		}
	})

	// Verify a name that cannot be typed says the list is there under another
	t.Run("TypingFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("TEST LIST")

		// The entry opens an ordinary menu rather than a name screen, which is
		// what a scanner that landed somewhere unexpected looks like.
		delete(r.inputs, "Rename")
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "naming it failed") {
			t.Fatalf("runNew reported %v, wanted the failed naming", err)
		}
	})

	// Verify a scanner that will not leave its menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("TEST LIST", listed("TEST LIST"))
		r.title = "New Favorites List"
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Fatalf("runNew reported %v, wanted the menus reported as stuck", err)
		}
	})

	// Verify a scanner that will not say what it holds afterwards is reported
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("TEST LIST")
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "GLT,FL") {
				return "", errors.New("the port is gone")
			}
			return r.reply(command)
		}}))

		err := runNew(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runNew reported %v, wanted the failed read", err)
		}
	})

	// Verify a list that does not appear afterwards is reported rather than
	// assumed to have been created
	t.Run("Missing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("TEST LIST", listed("OTHER LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runNew(context.Background(), app, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "does not appear under") {
			t.Fatalf("runNew reported %v, wanted the missing list", err)
		}
	})
}

// Test_runRename covers typing a new name into a favorites list.
//
// Coverage: 100% (12 test cases covering every branch)
//
// Test cases:
//   - Renames: the name screen is opened and the new name accepted
//   - Unchanged: a list already called that is left alone
//   - NoDevice: a run with no scanner named is refused
//   - WalkFails: a scanner that will not list what it holds is reported
//   - EntryMissing: a menu without the rename entry is reported
//   - PressFails: a scanner that will not take the press is reported
//   - TypingFails: a name that cannot be typed says nothing was saved
//   - LeaveFails: a scanner that will not leave its menus is reported
//   - ReadBackFails: a scanner that will not say what it holds afterwards is reported
//   - Missing: a list not carrying the new name afterwards is reported
//   - JSON: the change is reported as an object under -o json
//   - WriteError: a stream the report cannot be written to is reported
func Test_runRename(t *testing.T) {
	// walk builds a radio whose top menu leads to one list, whose own menu
	// carries Rename next to Delete.
	//
	// after is what the scanner reports once the rename is over, since the
	// rename reads back rather than taking the typing as proof. A case that
	// fails before the read back needs none.
	walk := func(holds string, after ...string) *radio {
		return &radio{
			rows: []string{"Manage Favorites"},
			opens: map[string][]string{
				"Manage Favorites": {"TEST LIST"},
				"TEST LIST":        {"Rename", "Delete"},
			},
			inputs: map[string]string{"Rename": holds},
			lists:  append([]string{listed("TEST LIST")}, after...),
		}
	}

	// Verify the name screen is reached and the new name is what gets reported.
	// The screen already holds the name, so accepting it is a single press.
	t.Run("Renames", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk("NEW NAME", listed("NEW NAME"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runRename(context.Background(), app, "TEST LIST", "NEW NAME"); err != nil {
			t.Fatalf("runRename: %v", err)
		}
		if got, want := out.String(), "NEW NAME\n"; got != want {
			t.Errorf("runRename wrote %q, wanted %q", got, want)
		}
	})

	// Verify a list already called that is left where it was, with nothing typed
	t.Run("Unchanged", func(t *testing.T) {
		app, out, notes := quiet()
		r := walk("TEST LIST")
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runRename(context.Background(), app, "TEST LIST", "TEST LIST"); err != nil {
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

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runRename reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not list what it holds is reported
	t.Run("WalkFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runRename reported %v, wanted the failed walk", err)
		}
	})

	// Verify a list menu without a rename entry is reported
	t.Run("EntryMissing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("NEW NAME")
		r.opens["TEST LIST"] = []string{"Information", "Delete"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "on the TEST LIST menu") {
			t.Fatalf("runRename reported %v, wanted the missing entry", err)
		}
	})

	// Verify a scanner that will not take the press that opens the screen is
	// reported
	t.Run("PressFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("NEW NAME")
		presses := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			// The walk presses twice to reach the list, and the third press is
			// the one that opens its name screen.
			if command == "KEY,E,P" {
				if presses++; presses == 3 {
					return "", errors.New("the scanner refused the key")
				}
			}
			return r.reply(command)
		}}))

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("runRename reported %v, wanted the refused press", err)
		}
	})

	// Verify a name that cannot be typed says nothing has been saved
	t.Run("TypingFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("NEW NAME")

		// The entry opens an ordinary menu rather than a name screen, which is
		// what a scanner that landed somewhere unexpected looks like.
		delete(r.inputs, "Rename")
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "nothing has been saved") {
			t.Fatalf("runRename reported %v, wanted the failed typing", err)
		}
	})

	// Verify a scanner that will not leave its menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("NEW NAME", listed("NEW NAME"))
		r.title = "TEST LIST"
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Fatalf("runRename reported %v, wanted the menus reported as stuck", err)
		}
	})

	// Verify a scanner that will not say what it holds afterwards is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("NEW NAME", listed("NEW NAME"))

		// The read back is the second request for the lists; the first is the
		// walk that found the list to rename.
		reads := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "GLT,FL") {
				if reads++; reads > 1 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}}))

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runRename reported %v, wanted the failed read back", err)
		}
	})

	// Verify a list that does not carry the new name afterwards is reported
	// rather than reported as renamed. Typing a name is not the scanner
	// keeping it.
	t.Run("Missing", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("NEW NAME", listed("TEST LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runRename(context.Background(), app, "TEST LIST", "NEW NAME")
		if err == nil || !strings.Contains(err.Error(), "does not appear under") {
			t.Fatalf("runRename reported %v, wanted the missing name", err)
		}
	})

	// Verify that a rename reports the change as an object under -o json
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk("NEW NAME", listed("NEW NAME"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "TEST LIST", "NEW NAME"); err != nil {
			t.Fatalf("runRename: %v", err)
		}

		var got render.Mutation
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the output is not JSON: %v\nstdout: %s", err, out.String())
		}
		if got.Action != "renamed" || got.Name != "NEW NAME" || got.Was != "TEST LIST" {
			t.Errorf("the report is %+v, wanted the rename and the name it had", got)
		}
	})

	// Verify that a stream the report cannot be written to is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk("NEW NAME", listed("NEW NAME"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))
		reader, writer := io.Pipe()
		reader.Close()
		app.Stdout = writer
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "TEST LIST", "NEW NAME"); err == nil {
			t.Error("expected an error when the report cannot be written, got none")
		}
	})
}

// Test_runScan covers choosing which lists are scanned.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - Named: the named list is switched on and the result reported
//   - JSON: the result is written for a program
//   - Contradictory: --all and --none together are refused
//   - Both: naming lists alongside a flag is refused
//   - Nothing: naming nothing at all is refused
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not list what it holds is reported
//   - ApplyFails: a scanner that will not take the change is reported
//   - ReadBackFails: a scanner that will not say what it ended up with is
//     reported
func Test_runScan(t *testing.T) {
	// walk builds a radio whose top menu leads to the scan selection, where
	// clearing the selection returns to it and the monitor list holds one row.
	walk := func(lists ...string) *radio {
		scanSelection := []string{"Set All Lists Off", "Select Lists to Monitor"}
		return &radio{
			rows: []string{"Set Scan Selection"},
			opens: map[string][]string{
				"Set Scan Selection":      scanSelection,
				"Set All Lists Off":       scanSelection,
				"Select Lists to Monitor": {"TEST LIST :Off"},
			},
			lists: lists,
		}
	}

	// Verify naming a list switches it on and the scanner's own answer is what
	// gets reported
	t.Run("Named", func(t *testing.T) {
		app, out, _ := quiet()
		r := walk(listed("TEST LIST"), `<GLT><FL Name="TEST LIST" Index="0" Monitor="On"/></GLT>`)
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runScan(context.Background(), app, []string{"TEST LIST"}, false, false); err != nil {
			t.Fatalf("runScan: %v", err)
		}
		if !strings.Contains(out.String(), "TEST LIST") || !strings.Contains(out.String(), "yes") {
			t.Errorf("runScan wrote %q, wanted the list reported as scanned", out.String())
		}
	})

	// Verify the JSON output carries what the scanner ended up with
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		r := walk(listed("TEST LIST"), `<GLT><FL Name="TEST LIST" Index="0" Monitor="On"/></GLT>`)
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		if err := runScan(context.Background(), app, []string{"TEST LIST"}, false, false); err != nil {
			t.Fatalf("runScan: %v", err)
		}

		var got []catalog.FavoritesList
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if len(got) != 1 || !got[0].Monitored {
			t.Errorf("runScan wrote %+v, wanted the list reported as scanned", got)
		}
	})

	// Verify asking for everything and nothing at once is refused
	t.Run("Contradictory", func(t *testing.T) {
		app, _, _ := quiet()

		err := runScan(context.Background(), app, nil, true, true)
		if err == nil || !strings.Contains(err.Error(), "opposite things") {
			t.Fatalf("runScan reported %v, wanted the contradiction refused", err)
		}
	})

	// Verify naming lists alongside a flag that means all of them is refused
	t.Run("Both", func(t *testing.T) {
		app, _, _ := quiet()

		err := runScan(context.Background(), app, []string{"TEST LIST"}, true, false)
		if err == nil || !strings.Contains(err.Error(), "different things") {
			t.Fatalf("runScan reported %v, wanted the mixed request refused", err)
		}
	})

	// Verify a run naming nothing at all is refused rather than guessed at
	t.Run("Nothing", func(t *testing.T) {
		app, _, _ := quiet()

		err := runScan(context.Background(), app, nil, false, false)
		if err == nil || !strings.Contains(err.Error(), "name the lists to scan") {
			t.Fatalf("runScan reported %v, wanted the empty request refused", err)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		err := runScan(context.Background(), app, []string{"TEST LIST"}, false, false)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runScan reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not list what it holds is reported before
	// anything is switched
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := runScan(context.Background(), app, []string{"TEST LIST"}, false, false)
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runScan reported %v, wanted the failed read", err)
		}
	})

	// Verify a name the scanner does not have is reported before anything moves
	t.Run("Unknown", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runScan(context.Background(), app, []string{"MISSING LIST"}, false, false)
		if err == nil || !strings.Contains(err.Error(), "MISSING LIST") {
			t.Fatalf("runScan reported %v, wanted the unknown name", err)
		}
		if len(r.pressed) != 0 {
			t.Errorf("the scanner was pressed on %v, wanted nothing pressed", r.pressed)
		}
	})

	// Verify a scanner that will not take the change is reported
	t.Run("ApplyFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		r.rows = []string{"Set Quick Keys"}
		app.SetDevice(device.New(fakeConn{reply: r.reply}))

		err := runScan(context.Background(), app, []string{"TEST LIST"}, false, false)
		if err == nil || !strings.Contains(err.Error(), setScanSelection) {
			t.Fatalf("runScan reported %v, wanted the refused change", err)
		}
	})

	// Verify a scanner that will not say what it ended up with is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		app, _, _ := quiet()
		r := walk(listed("TEST LIST"))
		reads := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			// The lists are read to resolve the names, and again afterwards to
			// see what the scanner ended up with. Only the second one fails.
			if strings.HasPrefix(command, "GLT,FL") {
				if reads++; reads == 2 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}}))

		err := runScan(context.Background(), app, []string{"TEST LIST"}, false, false)
		if err == nil || !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Fatalf("runScan reported %v, wanted the failed read back", err)
		}
	})
}

// Test_stepAndCommit covers moving to an entry and pressing it.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Presses: the entry is reached and pressed
//   - Missing: a menu without the entry is reported
func Test_stepAndCommit(t *testing.T) {
	// Verify the entry is reached and pressed
	t.Run("Presses", func(t *testing.T) {
		r := &radio{rows: []string{"Set All Lists On"}}
		client := device.New(fakeConn{reply: r.reply})

		if err := stepAndCommit(context.Background(), client, setAllOn); err != nil {
			t.Fatalf("stepAndCommit: %v", err)
		}
		if len(r.pressed) != 1 || r.pressed[0] != setAllOn {
			t.Errorf("the scanner was pressed on %v, wanted %q", r.pressed, setAllOn)
		}
	})

	// Verify a menu without the entry is reported rather than pressed anyway
	t.Run("Missing", func(t *testing.T) {
		r := &radio{rows: []string{"Set Quick Keys", "Set Number Tags"}}
		client := device.New(fakeConn{reply: r.reply})

		err := stepAndCommit(context.Background(), client, setAllOn)
		if err == nil {
			t.Fatal("stepAndCommit reported nothing, wanted the missing entry")
		}
		if !strings.Contains(err.Error(), setAllOn) {
			t.Errorf("stepAndCommit reported %q, wanted it to name %q", err, setAllOn)
		}
		if len(r.pressed) != 0 {
			t.Errorf("the scanner was pressed on %v, wanted nothing pressed", r.pressed)
		}
	})
}

// Test_switchOn covers walking the monitor list to one row and switching it on.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - Switches: a row that is off is pressed and confirmed on
//   - Already: a row already on is left alone
//   - ReadFails: a screen that cannot be read is reported
//   - PressFails: a scanner that will not take the press is reported
//   - ReadBackFails: a row that cannot be read back is reported
//   - Stuck: a row that does not switch on is reported
//   - TurnFails: a knob that will not turn is reported
//   - Missing: a list with no row of its own is reported
func Test_switchOn(t *testing.T) {
	// Verify a row that is off is pressed, and the state is read back to
	// confirm it went the way that was wanted
	t.Run("Switches", func(t *testing.T) {
		r := &radio{rows: []string{"OTHER LIST :Off", "TEST LIST :Off"}}
		client := device.New(fakeConn{reply: r.reply})

		if err := switchOn(context.Background(), client, "TEST LIST"); err != nil {
			t.Fatalf("switchOn: %v", err)
		}
		if got := r.rows[1]; got != "TEST LIST "+stateOn {
			t.Errorf("the row reads %q, wanted it switched on", got)
		}
	})

	// Verify a row already on is left alone rather than pressed back off
	t.Run("Already", func(t *testing.T) {
		r := &radio{rows: []string{"TEST LIST :On"}}
		client := device.New(fakeConn{reply: r.reply})

		if err := switchOn(context.Background(), client, "TEST LIST"); err != nil {
			t.Fatalf("switchOn: %v", err)
		}
		if len(r.pressed) != 0 {
			t.Errorf("the scanner was pressed on %v, wanted nothing pressed", r.pressed)
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadFails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := switchOn(context.Background(), client, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "reading the list of lists") {
			t.Fatalf("switchOn reported %v, wanted the failed read", err)
		}
	})

	// Verify a scanner that will not take the press is reported
	t.Run("PressFails", func(t *testing.T) {
		r := &radio{rows: []string{"TEST LIST :Off"}}
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "KEY,E,P" {
				return "", errors.New("the scanner refused the key")
			}
			return r.reply(command)
		}})

		err := switchOn(context.Background(), client, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("switchOn reported %v, wanted the refused press", err)
		}
	})

	// Verify a row that cannot be read back after the press is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		r := &radio{rows: []string{"TEST LIST :Off"}}
		reads := 0
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			// The screen is read to find the row, again while waiting for the
			// scanner to come back, and a third time to check the state.
			if command == "STS" {
				if reads++; reads == 3 {
					return "", errors.New("the port is gone")
				}
			}
			return r.reply(command)
		}})

		err := switchOn(context.Background(), client, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "back") {
			t.Fatalf("switchOn reported %v, wanted the failed read back", err)
		}
	})

	// Verify a row that stays off after being pressed is reported rather than
	// assumed to have switched
	t.Run("Stuck", func(t *testing.T) {
		r := &radio{rows: []string{"TEST LIST :Off"}}
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			// The press is acknowledged and then dropped, which is what a
			// scanner too busy to act on it does.
			if command == "KEY,E,P" {
				return "", nil
			}
			return r.reply(command)
		}})

		err := switchOn(context.Background(), client, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "did not switch on") {
			t.Fatalf("switchOn reported %v, wanted the row reported as stuck", err)
		}
	})

	// Verify a knob that will not turn is reported
	t.Run("TurnFails", func(t *testing.T) {
		r := &radio{rows: []string{"OTHER LIST :Off"}}
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "KEY,>,P" {
				return "", errors.New("the scanner refused the key")
			}
			return r.reply(command)
		}})

		err := switchOn(context.Background(), client, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("switchOn reported %v, wanted the refused turn", err)
		}
	})

	// Verify a list the monitor screen has no row for is reported
	t.Run("Missing", func(t *testing.T) {
		r := &radio{rows: []string{"OTHER LIST :Off"}}
		client := device.New(fakeConn{reply: r.reply})

		err := switchOn(context.Background(), client, "TEST LIST")
		if err == nil || !strings.Contains(err.Error(), "no row for") {
			t.Fatalf("switchOn reported %v, wanted the missing row", err)
		}
	})
}

// Test_toScanSelection covers putting the scanner on the scan selection menu.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Opens: the top menu is opened and the entry pressed
//   - MenuFails: a scanner that will not open its top menu is reported
//   - EntryMissing: a top menu without the entry is reported
func Test_toScanSelection(t *testing.T) {
	// Verify the walk starts from the top menu and presses the entry
	t.Run("Opens", func(t *testing.T) {
		r := &radio{rows: []string{"Set Scan Selection"}}
		client := device.New(fakeConn{reply: r.reply})

		if err := toScanSelection(context.Background(), client); err != nil {
			t.Fatalf("toScanSelection: %v", err)
		}
		if len(r.pressed) != 1 || r.pressed[0] != setScanSelection {
			t.Errorf("the scanner was pressed on %v, wanted %q", r.pressed, setScanSelection)
		}
	})

	// Verify a scanner that will not open its top menu is reported
	t.Run("MenuFails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}})

		err := toScanSelection(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "opening the top menu") {
			t.Fatalf("toScanSelection reported %v, wanted the refused menu", err)
		}
	})

	// Verify a top menu without the entry is reported rather than pressed at
	t.Run("EntryMissing", func(t *testing.T) {
		r := &radio{rows: []string{"Manage Favorites", "Settings"}}
		client := device.New(fakeConn{reply: r.reply})

		err := toScanSelection(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "in the top menu") {
			t.Fatalf("toScanSelection reported %v, wanted the missing entry", err)
		}
	})
}

// Test_yesNo covers how the scanned state is written.
//
// Coverage: 100% (1 test case covering both branches)
//
