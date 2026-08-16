// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package departments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
)

// stubConn is a fake device.Conn that answers the exchanges these commands
// make, so a command can be driven with no scanner attached.
//
// Screens are answered in order, one per read of the display, each holding a
// single line that is highlighted. An empty name fails that read instead,
// which is how a test puts a failure at one step of a walk without waiting for
// any of the menus package's poll loops to give up.
//
// Documents are answered from a queue per command, and the last one in a queue
// stays in place once the rest are used up, so a command that reads the same
// list twice can be given two different answers. An empty document fails that
// read.
//
// A command with no document of its own is answered the way a scanner that is
// out of the menus and scanning answers it, which is what makes leaving the
// menus cost nothing here.
type stubConn struct {
	screens  []string            // The name highlighted on each successive display read, "" to fail that read
	reads    int                 // How many display reads have been answered so far
	docs     map[string][]string // Documents to answer with, in order, keyed by the command asking for them
	fail     map[string]error    // Commands that fail instead of answering
	failFrom map[string]int      // Commands that start failing at the numbered send, counting from one
	sent     []string            // Every command sent to the scanner, in order
}

// Info describes the fake scanner, which nothing here inspects.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute answers a display read with the next screen, and acknowledges every
// other command the way the scanner does when it accepts one.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) {
	c.sent = append(c.sent, command)
	if err := c.fail[command]; err != nil {
		return "", err
	}

	// A key pressed several times over one walk is the same command each time,
	// so a test says which of the presses is the one that fails.
	if from, ok := c.failFrom[command]; ok && c.count(command) >= from {
		return "", errors.New("the scanner refused the key")
	}

	// Menu jumps and key presses are accepted silently.
	if command != "STS" {
		return "", nil
	}

	if c.reads >= len(c.screens) {
		return "", errors.New("the scanner has no screen left to show")
	}
	name := c.screens[c.reads]
	c.reads++

	// An empty name stands for a read the scanner refuses.
	if name == "" {
		return "", errors.New("the screen cannot be read")
	}

	// One line, in the small font, holding the name and marked highlighted.
	return "0," + name + ",*", nil
}

// ExecuteXML answers a list or a status read with the document the test
// supplied, falling back to a scanner that is out of the menus and scanning.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	c.sent = append(c.sent, command)
	if err := c.fail[command]; err != nil {
		return "", err
	}

	if doc, ok := c.next(command); ok {
		if doc == "" {
			return "", errors.New("the scanner refused the list")
		}
		return doc, nil
	}

	switch command {
	case "MSI":
		return `<MenuInfo MenuType="TypeError"/>`, nil
	case "GSI":
		return `<ScannerInfo Mode="Scan" V_Screen="scan"/>`, nil
	}
	return "", errors.New("unexpected command: " + command)
}

// Send is unused by these commands and always succeeds.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close releases nothing, because there is no port.
func (c *stubConn) Close() error { return nil }

// count reports how many times a command has been sent, including the one
// being answered now.
func (c *stubConn) count(command string) int {
	seen := 0
	for _, sent := range c.sent {
		if sent == command {
			seen++
		}
	}
	return seen
}

// next takes the answer a command is due, leaving the last one in place so a
// command read more times than the test scripted keeps getting it.
func (c *stubConn) next(command string) (string, bool) {
	answers, ok := c.docs[command]
	if !ok || len(answers) == 0 {
		return "", false
	}
	if len(answers) == 1 {
		return answers[0], true
	}
	c.docs[command] = answers[1:]
	return answers[0], true
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its subcommands
//   - Runs: running the command reports the system's departments
func TestNew(t *testing.T) {
	// Verify that the command is named and carries every subcommand
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "departments <system>" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "departments <system>")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		for _, want := range []string{"goto", "rename", "new", "delete"} {
			found := false
			for _, sub := range cmd.Commands() {
				if sub.Name() == want {
					found = true
				}
			}
			if !found {
				t.Errorf("the command has no %q subcommand", want)
			}
		}
	})

	// Verify that running the command reads the departments and writes them out
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{docs: map[string][]string{
			"GLT,DEPT,10": {`<GLT><DEPT Name="POLICE" Index="20" Avoid="Off"/></GLT>`},
		}}))

		cmd := New(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"10"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "POLICE") {
			t.Errorf("expected the listing to name the department, got: %q", out.String())
		}
	})
}

// Test_newDelete tests the newDelete function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its --yes flag
//   - Runs: running the subcommand reaches the scanner
func Test_newDelete(t *testing.T) {
	// Verify that the subcommand is named and carries the flag that arms it
	t.Run("Wiring", func(t *testing.T) {
		cmd := newDelete(appcontext.New())

		if cmd.Name() != "delete" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "delete")
		}
		if cmd.Flags().Lookup("yes") == nil {
			t.Error("the subcommand has no --yes flag")
		}
	})

	// Verify that running the subcommand asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newDelete(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"20"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_newGoto tests the newGoto function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its help
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no department carries is reported
//   - Success: the department's menu is opened
func Test_newGoto(t *testing.T) {
	// Verify that the subcommand is named and documented
	t.Run("Wiring", func(t *testing.T) {
		cmd := newGoto(appcontext.New())

		if cmd.Name() != "goto" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "goto")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify that running it with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newGoto(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"20"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a department that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{
			fail: map[string]error{"GLT,FL": errors.New("the port is gone")},
		}))

		cmd := newGoto(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"POLICE"}); err == nil {
			t.Error("expected an error when the department cannot be resolved, got none")
		}
	})

	// Verify that the menu is opened on the scanner
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{}
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		app.SetDevice(device.New(conn))

		cmd := newGoto(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"20"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(strings.Join(conn.sent, " "), "MNU,SCAN_DEPARTMENT,20") {
			t.Errorf("expected the department's menu to be opened, got: %v", conn.sent)
		}
	})
}

// Test_newNew tests the newNew function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its help
//   - Runs: running the subcommand reaches the scanner
func Test_newNew(t *testing.T) {
	// Verify that the subcommand is named and documented
	t.Run("Wiring", func(t *testing.T) {
		cmd := newNew(appcontext.New())

		if cmd.Name() != "new" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "new")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify that running the subcommand asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newNew(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"10", "POLICE"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_newRename tests the newRename function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its help
//   - Runs: running the subcommand reaches the scanner
func Test_newRename(t *testing.T) {
	// Verify that the subcommand is named and documented
	t.Run("Wiring", func(t *testing.T) {
		cmd := newRename(appcontext.New())

		if cmd.Name() != "rename" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "rename")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify that running the subcommand asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newRename(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"20", "POLICE"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_confirm tests the confirm function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - ByName: a system named by name was already looked up, so nothing is asked
//   - ReadError: a failed read of the systems is reported
//   - Exists: an index a system carries is accepted
//   - Missing: an index no system carries is reported with the alternatives
func Test_confirm(t *testing.T) {
	// Verify that a system named by name costs no extra exchange
	t.Run("ByName", func(t *testing.T) {
		conn := &stubConn{}

		if err := confirm(context.Background(), device.New(conn), "FIRE", "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(conn.sent) != 0 {
			t.Errorf("expected nothing to be sent, got: %v", conn.sent)
		}
	})

	// Verify that a failed read of the systems is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if err := confirm(context.Background(), device.New(conn), "10", "10"); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that an index the scanner really has is accepted
	t.Run("Exists", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL":    {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
			"GLT,SYS,1": {`<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`},
		}}

		if err := confirm(context.Background(), device.New(conn), "10", "10"); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that an index no system carries is reported along with the alternatives
	t.Run("Missing", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL":    {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
			"GLT,SYS,1": {`<GLT><SYS Name="FIRE" Index="11" Type="P25 Trunk"/></GLT>`},
		}}

		err := confirm(context.Background(), device.New(conn), "10", "10")
		if err == nil {
			t.Fatal("expected an error when no system has that index, got none")
		}
		if !strings.Contains(err.Error(), "no system has index 10") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
		if !strings.Contains(err.Error(), "FIRE") {
			t.Errorf("expected the message to offer the alternatives, got: %v", err)
		}
	})
}

// Test_departmentName tests the departmentName function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Found: the scanner's own name for the department is reported
//   - NotFound: an index no department carries is reported
//   - ReadError: a failed read is reported
func Test_departmentName(t *testing.T) {
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`

	// Verify that the name comes from the scanner rather than from the caller
	t.Run("Found", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL":      {favorites},
			"GLT,SYS,1":   {systems},
			"GLT,DEPT,10": {`<GLT><DEPT Name="POLICE" Index="20" Avoid="Off"/></GLT>`},
		}}

		name, err := departmentName(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if name != "POLICE" {
			t.Errorf("got name %q, wanted %q", name, "POLICE")
		}
	})

	// Verify that an index the scanner does not report is refused
	t.Run("NotFound", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL":      {favorites},
			"GLT,SYS,1":   {systems},
			"GLT,DEPT,10": {`<GLT></GLT>`},
		}}

		_, err := departmentName(context.Background(), device.New(conn), "20")
		if err == nil {
			t.Fatal("expected an error when no department has that index, got none")
		}
		if !strings.Contains(err.Error(), "no department has index 20") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})

	// Verify that a failed read is reported rather than read as an empty memory
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if _, err := departmentName(context.Background(), device.New(conn), "20"); err == nil {
			t.Error("expected an error when the departments cannot be read, got none")
		}
	})
}

// Test_names tests the names function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - None: a scanner reporting no systems says so
//   - Several: the systems are named in quotes
func Test_names(t *testing.T) {
	// Verify that a scanner holding no systems at all is reported in words
	t.Run("None", func(t *testing.T) {
		if got := names(nil); got != "the scanner's favorites lists hold no systems at all" {
			t.Errorf("got %q, wanted the note about no systems", got)
		}
	})

	// Verify that the alternatives are named, so nobody has to run another command
	t.Run("Several", func(t *testing.T) {
		got := names([]catalog.System{
			{Name: "FIRE", Index: "10"},
			{Name: "RAIL", Index: "11"},
		})

		if got != `the scanner has "FIRE", "RAIL"` {
			t.Errorf("got %q, wanted the two names in quotes", got)
		}
	})
}

// Test_renderDepartments tests the renderDepartments function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Empty: a system holding no departments is reported as an answer
//   - Table: the departments are written as an aligned table
//   - FlushError: a failed write is reported
func Test_renderDepartments(t *testing.T) {
	// Verify that a system holding nothing is explained rather than failed
	t.Run("Empty", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		if err := renderDepartments(app, nil); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "no departments") {
			t.Errorf("expected a note about a system holding nothing, got: %q", notes.String())
		}
		if out.String() != "" {
			t.Errorf("expected nothing on the output, got: %q", out.String())
		}
	})

	// Verify that the table carries the name, whether it is scanned, and the quick key
	t.Run("Table", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}

		err := renderDepartments(app, []catalog.Department{
			{Name: "POLICE", Index: "20", QuickKey: "3"},
			{Name: "FIRE", Index: "21", Avoided: true},
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		written := out.String()
		for _, want := range []string{"NAME", "POLICE", "yes", "3", "FIRE", "no", "-"} {
			if !strings.Contains(written, want) {
				t.Errorf("expected the table to carry %q, got: %q", want, written)
			}
		}
	})

	// Verify that a stream that cannot be written to is reported
	t.Run("FlushError", func(t *testing.T) {
		reader, writer := io.Pipe()
		reader.Close()

		app := appcontext.New()
		app.Stdout, app.Stderr = writer, &bytes.Buffer{}

		err := renderDepartments(app, []catalog.Department{{Name: "POLICE", Index: "20"}})
		if err == nil {
			t.Fatal("expected an error when the listing cannot be written, got none")
		}
		if !strings.Contains(err.Error(), "writing the department list") {
			t.Errorf("expected the message to say it was writing the list, got: %v", err)
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no system carries is reported
//   - ReadError: a failed read of the departments is reported
//   - EmptySystem: a system that exists and holds nothing is reported as that
//   - MissingSystem: an index no system carries is told apart from an empty one
//   - JSON: the departments are encoded when JSON was asked for
//   - Text: the departments are written as a table otherwise
func Test_run(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes
		app.SetDevice(device.New(conn))
		return app, out, notes
	}

	departments := `<GLT><DEPT Name="POLICE" Index="20" Avoid="Off"/></GLT>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := run(context.Background(), app, "10"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a system that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := run(context.Background(), app, "FIRE"); err == nil {
			t.Error("expected an error when the system cannot be resolved, got none")
		}
	})

	// Verify that a failed read of the departments is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"GLT,DEPT,10": errors.New("the port is gone")}})

		if err := run(context.Background(), app, "10"); err == nil {
			t.Error("expected an error when the departments cannot be read, got none")
		}
	})

	// Verify that a system that really exists and holds nothing is reported as that
	t.Run("EmptySystem", func(t *testing.T) {
		app, _, notes := appWith(&stubConn{docs: map[string][]string{
			"GLT,DEPT,10": {`<GLT></GLT>`},
			"GLT,FL":      {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
			"GLT,SYS,1":   {`<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`},
		}})

		if err := run(context.Background(), app, "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "holds no departments") {
			t.Errorf("expected a note that the system holds nothing, got: %q", notes.String())
		}
	})

	// Verify that an index no system carries is told apart from a system holding nothing
	t.Run("MissingSystem", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,DEPT,10": {`<GLT></GLT>`},
			"GLT,FL":      {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
			"GLT,SYS,1":   {`<GLT><SYS Name="FIRE" Index="11" Type="P25 Trunk"/></GLT>`},
		}})

		err := run(context.Background(), app, "10")
		if err == nil {
			t.Fatal("expected an error when no system has that index, got none")
		}
		if !strings.Contains(err.Error(), "no system has index 10") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})

	// Verify that asking for JSON encodes the departments rather than tabulating them
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{docs: map[string][]string{"GLT,DEPT,10": {departments}}})
		app.Config.Output = appcontext.OutputJSON

		if err := run(context.Background(), app, "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var found []catalog.Department
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if len(found) != 1 || found[0].Name != "POLICE" {
			t.Errorf("expected the department in the JSON, got: %v", found)
		}
	})

	// Verify that the default output is the aligned table
	t.Run("Text", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{docs: map[string][]string{"GLT,DEPT,10": {departments}}})

		if err := run(context.Background(), app, "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "POLICE") {
			t.Errorf("expected the listing to name the department, got: %q", out.String())
		}
	})
}

// Test_runDelete tests the runDelete function with 100% coverage.
//
// Coverage: 100% (12 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no department carries is reported
//   - NameError: a department the scanner does not report is reported
//   - WithoutYes: nothing is deleted without --yes
//   - OpenMenuError: a refused menu jump is reported
//   - SelectError: a menu holding no delete entry is reported
//   - ConfirmError: a scanner not asking to confirm is reported
//   - AwakenError: a scanner that stops answering afterwards is reported
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the delete is reported
//   - StillThere: a department still on the scanner afterwards is reported
//   - Success: the department is deleted and named
func Test_runDelete(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The documents that carry one department, POLICE, in one system.
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`
	departments := `<GLT><DEPT Name="POLICE" Index="20" Avoid="Off"/></GLT>`

	// The walk through the department's menu: its delete entry, the prompt,
	// and the screen the scanner shows once it has finished deleting.
	walk := []string{deleteDepartment, "Confirm Delete?", "Scanning"}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runDelete(context.Background(), app, "20", true); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a department that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runDelete(context.Background(), app, "POLICE", true); err == nil {
			t.Error("expected an error when the department cannot be resolved, got none")
		}
	})

	// Verify that an index no department carries is reported
	t.Run("NameError", func(t *testing.T) {
		app, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {`<GLT></GLT>`},
		}})

		err := runDelete(context.Background(), app, "20", true)
		if err == nil {
			t.Fatal("expected an error when no department has that index, got none")
		}
		if !strings.Contains(err.Error(), "no department has index") {
			t.Errorf("expected the message to say the index names nothing, got: %v", err)
		}
	})

	// Verify that nothing is pressed until --yes is given
	t.Run("WithoutYes", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
		}}
		app, _ := appWith(conn)

		err := runDelete(context.Background(), app, "20", false)
		if err == nil {
			t.Fatal("expected an error without --yes, got none")
		}
		if !strings.Contains(err.Error(), "--yes") {
			t.Errorf("expected the message to ask for --yes, got: %v", err)
		}
		for _, command := range conn.sent {
			if strings.HasPrefix(command, "KEY,") || strings.HasPrefix(command, "MNU,") {
				t.Errorf("expected nothing to be pressed, got: %v", conn.sent)
			}
		}
	})

	// Verify that a refused menu jump is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
			},
			fail: map[string]error{"MNU,SCAN_DEPARTMENT,20": errors.New("the scanner refused the jump")},
		})

		err := runDelete(context.Background(), app, "20", true)
		if err == nil {
			t.Fatal("expected an error when the menu cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "opening the department's menu") {
			t.Errorf("expected the message to say it was opening the menu, got: %v", err)
		}
	})

	// Verify that a menu with no delete entry is reported
	t.Run("SelectError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{""},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
			},
		})

		err := runDelete(context.Background(), app, "20", true)
		if err == nil {
			t.Fatal("expected an error when the delete entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), deleteDepartment) {
			t.Errorf("expected the message to name %q, got: %v", deleteDepartment, err)
		}
	})

	// Verify that a scanner not asking to confirm is reported
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{deleteDepartment, "Scanning"},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
			},
		})

		err := runDelete(context.Background(), app, "20", true)
		if err == nil {
			t.Fatal("expected an error when the scanner is not asking to confirm, got none")
		}
		if !strings.Contains(err.Error(), "nothing was deleted") {
			t.Errorf("expected the message to say nothing was deleted, got: %v", err)
		}
	})

	// Verify that a scanner that stops answering while it rebuilds is reported
	t.Run("AwakenError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{deleteDepartment, "Confirm Delete?"},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
			},
		})

		err := runDelete(context.Background(), app, "20", true)
		if err == nil {
			t.Fatal("expected an error when the scanner stops answering, got none")
		}
		if !strings.Contains(err.Error(), "stopped answering") {
			t.Errorf("expected the message to say the scanner stopped answering, got: %v", err)
		}
	})

	// Verify that a failure returning the scanner to scanning is reported
	t.Run("LeaveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
			},
			fail: map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runDelete(context.Background(), app, "20", true); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the delete is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"GLT,FL": {favorites, ""}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
			},
		})

		if err := runDelete(context.Background(), app, "20", true); err == nil {
			t.Error("expected an error when the departments cannot be read back, got none")
		}
	})

	// Verify that a department still on the scanner afterwards is reported as not deleted
	t.Run("StillThere", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,DEPT,10": {departments},
			},
		})

		err := runDelete(context.Background(), app, "20", true)
		if err == nil {
			t.Fatal("expected an error when the department is still there, got none")
		}
		if !strings.Contains(err.Error(), "nothing was deleted") {
			t.Errorf("expected the message to say nothing was deleted, got: %v", err)
		}
	})

	// Verify that a deleted department is named on the output
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems},
				"GLT,DEPT,10": {departments, `<GLT></GLT>`},
			},
		})

		if err := runDelete(context.Background(), app, "20", true); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "deleted POLICE") {
			t.Errorf("expected the output to name the deleted department, got: %q", out.String())
		}
	})
}

// Test_runNew tests the runNew function with 100% coverage.
//
// Coverage: 100% (9 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - NavigateError: a system that cannot be reached is reported
//   - CreateError: a menu holding no New Department entry is reported
//   - NameScreenError: a department created but not named is reported as that
//   - SetError: a name that cannot be typed is reported
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the create is reported
//   - NotFound: a department that does not appear afterwards is reported
//   - Success: the department is created, named, and reported
func Test_runNew(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`

	// The walk to a new department: the system's departments entry, the entry
	// that creates one, and the name screen it opens.
	walk := []string{"Edit Department", newDepartment, editNameEntry}

	// The text entry screen, holding the name already, so accepting it is one
	// press rather than a walk through the alphabet.
	entryScreen := `<MenuInfo Name="Name" MenuType="TypeInput" Value="POLICE">` +
		`<MenuInput MaxLength="16" EnableKeys="ABCDEFGHIJKLMNOPQRSTUVWXYZ "/></MenuInfo>`
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runNew(context.Background(), app, "10", "POLICE"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a system that cannot be reached is reported
	t.Run("NavigateError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			fail: map[string]error{"MNU,SCAN_SYSTEM,10": errors.New("the scanner refused the jump")},
		})

		if err := runNew(context.Background(), app, "10", "POLICE"); err == nil {
			t.Error("expected an error when the system cannot be reached, got none")
		}
	})

	// Verify that a menu with no New Department entry is reported
	t.Run("CreateError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: walk[:1]})

		err := runNew(context.Background(), app, "10", "POLICE")
		if err == nil {
			t.Fatal("expected an error when the department cannot be created, got none")
		}
		if !strings.Contains(err.Error(), "creating the department") {
			t.Errorf("expected the message to say it was creating the department, got: %v", err)
		}
	})

	// Verify that a department created but left unnamed says so
	t.Run("NameScreenError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: walk[:2]})

		err := runNew(context.Background(), app, "10", "POLICE")
		if err == nil {
			t.Fatal("expected an error when the name screen cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "the department was created") {
			t.Errorf("expected the message to say the department was created, got: %v", err)
		}
	})

	// Verify that a name that cannot be typed is reported
	t.Run("SetError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: walk})

		err := runNew(context.Background(), app, "10", "POLICE")
		if err == nil {
			t.Fatal("expected an error when the name cannot be typed, got none")
		}
		if !strings.Contains(err.Error(), "naming it failed") {
			t.Errorf("expected the message to say naming failed, got: %v", err)
		}
	})

	// Verify that a failure returning the scanner to scanning is reported
	t.Run("LeaveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"MSI": {entryScreen, outOfMenus}},
			fail:    map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runNew(context.Background(), app, "10", "POLICE"); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the create is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"MSI": {entryScreen, outOfMenus}, "GLT,FL": {""},
			},
		})

		if err := runNew(context.Background(), app, "10", "POLICE"); err == nil {
			t.Error("expected an error when the departments cannot be read back, got none")
		}
	})

	// Verify that a department that does not appear afterwards is reported
	t.Run("NotFound", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"MSI": {entryScreen, outOfMenus}, "GLT,FL": {favorites},
				"GLT,SYS,1": {systems}, "GLT,DEPT,10": {`<GLT></GLT>`},
			},
		})

		err := runNew(context.Background(), app, "10", "POLICE")
		if err == nil {
			t.Fatal("expected an error when the department does not appear, got none")
		}
		if !strings.Contains(err.Error(), "does not appear under") {
			t.Errorf("expected the message to say the department did not appear, got: %v", err)
		}
	})

	// Verify that a created department is named on the output
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"MSI": {entryScreen, outOfMenus}, "GLT,FL": {favorites},
				"GLT,SYS,1":   {systems},
				"GLT,DEPT,10": {`<GLT><DEPT Name="POLICE" Index="20" Avoid="Off"/></GLT>`},
			},
		})

		if err := runNew(context.Background(), app, "10", "POLICE"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "POLICE") {
			t.Errorf("expected the output to name the department, got: %q", out.String())
		}
	})
}

// Test_runRename tests the runRename function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no department carries is reported
//   - OpenMenuError: a refused menu jump is reported
//   - StepToError: a menu holding no name entry is reported
//   - EnterError: a refused press on the name entry is reported
//   - SetError: a name that cannot be typed is reported as nothing saved
//   - Success: the department is renamed and reported
func Test_runRename(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The text entry screen, holding the name already, so accepting it is one
	// press rather than a walk through the alphabet.
	entryScreen := `<MenuInfo Name="Name" MenuType="TypeInput" Value="POLICE">` +
		`<MenuInput MaxLength="16" EnableKeys="ABCDEFGHIJKLMNOPQRSTUVWXYZ "/></MenuInfo>`
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runRename(context.Background(), app, "20", "POLICE"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a department that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runRename(context.Background(), app, "POLICE", "FIRE"); err == nil {
			t.Error("expected an error when the department cannot be resolved, got none")
		}
	})

	// Verify that a refused menu jump is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			fail: map[string]error{"MNU,SCAN_DEPARTMENT,20": errors.New("the scanner refused the jump")},
		})

		err := runRename(context.Background(), app, "20", "POLICE")
		if err == nil {
			t.Fatal("expected an error when the menu cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "opening the department's menu") {
			t.Errorf("expected the message to say it was opening the menu, got: %v", err)
		}
	})

	// Verify that a menu with no name entry is reported
	t.Run("StepToError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{""}})

		err := runRename(context.Background(), app, "20", "POLICE")
		if err == nil {
			t.Fatal("expected an error when the name entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), editName) {
			t.Errorf("expected the message to name %q, got: %v", editName, err)
		}
	})

	// Verify that a refused press on the name entry is reported
	t.Run("EnterError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens:  []string{editName},
			failFrom: map[string]int{"KEY,E,P": 1},
		})

		if err := runRename(context.Background(), app, "20", "POLICE"); err == nil {
			t.Error("expected an error when the entry cannot be pressed, got none")
		}
	})

	// Verify that a name that cannot be typed is reported as nothing saved
	t.Run("SetError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{editName}})

		err := runRename(context.Background(), app, "20", "POLICE")
		if err == nil {
			t.Fatal("expected an error when the name cannot be typed, got none")
		}
		if !strings.Contains(err.Error(), "nothing has been saved") {
			t.Errorf("expected the message to say nothing was saved, got: %v", err)
		}
	})

	// Verify that a renamed department is reported under its new name
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{editName},
			docs:    map[string][]string{"MSI": {entryScreen, outOfMenus}},
		})

		if err := runRename(context.Background(), app, "20", "POLICE"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "POLICE") {
			t.Errorf("expected the output to carry the new name, got: %q", out.String())
		}
	})

	// Verify that a rename reports the change as an object under -o json
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{editName},
			docs:    map[string][]string{"MSI": {entryScreen, outOfMenus}},
		})
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "20", "POLICE"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got render.Mutation
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the output is not JSON: %v\nstdout: %s", err, out.String())
		}
		if got.Action != "renamed" || got.Kind != "department" || got.Name != "POLICE" {
			t.Errorf("the report is %+v, wanted the rename and the new name", got)
		}
	})

	// Verify that a stream the report cannot be written to is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{editName},
			docs:    map[string][]string{"MSI": {entryScreen, outOfMenus}},
		})
		reader, writer := io.Pipe()
		reader.Close()
		app.Stdout = writer
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "20", "POLICE"); err == nil {
			t.Error("expected an error when the report cannot be written, got none")
		}
	})
}

// Test_renderDepartmentsPartial tests the branch that renders a department the
// scanner left out of its own list.
//
// Coverage: 100% (1 test case covering the remaining branch)
//
// Test cases:
//   - Partial: the columns nobody read say so, and a note explains the mark
func Test_renderDepartmentsPartial(t *testing.T) {
	// Verify that "unknown" is not printed as a confident "not scanned"
	t.Run("Partial", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		err := renderDepartments(app, []catalog.Department{{Name: "MARINE", Partial: true}})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), render.Unread) {
			t.Errorf("expected the unread mark in the table, got: %q", out.String())
		}
		if !strings.Contains(notes.String(), "read off") {
			t.Errorf("expected a note explaining the mark, got: %q", notes.String())
		}
	})
}
