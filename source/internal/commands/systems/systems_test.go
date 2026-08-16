// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package systems

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
//   - Runs: running the command reports the list's systems
func TestNew(t *testing.T) {
	// Verify that the command is named and carries every subcommand
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "systems <list>" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "systems <list>")
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

	// Verify that running the command reads the systems and writes them out
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{docs: map[string][]string{
			"GLT,SYS,1": {`<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk" Avoid="Off"/></GLT>`},
		}}))

		cmd := New(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"1"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "FIRE") {
			t.Errorf("expected the listing to name the system, got: %q", out.String())
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

		if err := cmd.RunE(cmd, []string{"10"}); err == nil {
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
//   - ResolveError: a name no system carries is reported
//   - Success: the system's menu is opened and where it landed reported
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

		if err := cmd.RunE(cmd, []string{"10"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a system that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{
			fail: map[string]error{"GLT,FL": errors.New("the port is gone")},
		}))

		cmd := newGoto(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"FIRE"}); err == nil {
			t.Error("expected an error when the system cannot be resolved, got none")
		}
	})

	// Verify that the menu is opened and the scanner's answer reported
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{}
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		app.SetDevice(device.New(conn))

		cmd := newGoto(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"10"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(strings.Join(conn.sent, " "), "MNU,SCAN_SYSTEM,10") {
			t.Errorf("expected the system's menu to be opened, got: %v", conn.sent)
		}
	})
}

// Test_newNew tests the newNew function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its --type flag
//   - BadType: a type the scanner does not offer is refused
//   - Runs: a matched type carries on to the scanner
func Test_newNew(t *testing.T) {
	// Verify that the subcommand is named and carries the flag it requires
	t.Run("Wiring", func(t *testing.T) {
		cmd := newNew(appcontext.New())

		if cmd.Name() != "new" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "new")
		}
		if cmd.Flags().Lookup("type") == nil {
			t.Error("the subcommand has no --type flag")
		}
	})

	// Verify that a type the scanner does not offer is refused before anything else
	t.Run("BadType", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newNew(app)
		cmd.SetContext(context.Background())

		err := cmd.RunE(cmd, []string{"1", "AIRPORT"})
		if err == nil {
			t.Fatal("expected an error for a type the scanner does not offer, got none")
		}
		if !strings.Contains(err.Error(), "no system type is called") {
			t.Errorf("expected the message to say the type is unknown, got: %v", err)
		}
	})

	// Verify that a matched type carries on and asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newNew(app)
		cmd.SetContext(context.Background())
		if err := cmd.Flags().Set("type", "conventional"); err != nil {
			t.Fatalf("setting the flag: %v", err)
		}

		if err := cmd.RunE(cmd, []string{"1", "AIRPORT"}); err == nil {
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

		if err := cmd.RunE(cmd, []string{"10", "AIRPORT"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_confirm tests the confirm function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - ByName: a list named by name was already looked up, so nothing is asked
//   - ReadError: a failed read of the favorites lists is reported
//   - Exists: an index a list carries is accepted
//   - Missing: an index no list carries is reported with the alternatives
func Test_confirm(t *testing.T) {
	// Verify that a list named by name costs no extra exchange
	t.Run("ByName", func(t *testing.T) {
		conn := &stubConn{}

		if err := confirm(context.Background(), device.New(conn), "HOME", "1"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(conn.sent) != 0 {
			t.Errorf("expected nothing to be sent, got: %v", conn.sent)
		}
	})

	// Verify that a failed read of the favorites lists is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if err := confirm(context.Background(), device.New(conn), "1", "1"); err == nil {
			t.Error("expected an error when the lists cannot be read, got none")
		}
	})

	// Verify that an index the scanner really has is accepted
	t.Run("Exists", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
		}}

		if err := confirm(context.Background(), device.New(conn), "1", "1"); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that an index no list carries is reported along with the alternatives
	t.Run("Missing", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {`<GLT><FL Name="HOME" Index="2"/></GLT>`},
		}}

		err := confirm(context.Background(), device.New(conn), "1", "1")
		if err == nil {
			t.Fatal("expected an error when no list has that index, got none")
		}
		if !strings.Contains(err.Error(), "no favorites list has index 1") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
		if !strings.Contains(err.Error(), "HOME") {
			t.Errorf("expected the message to offer the alternatives, got: %v", err)
		}
	})
}

// Test_matchType tests the matchType function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Exact: a type spelled the scanner's way is taken as it is
//   - Loose: case and surrounding spaces do not have to be reproduced
//   - Unknown: a type the scanner does not offer is refused with the list
func Test_matchType(t *testing.T) {
	// Verify that the scanner's own spelling is accepted
	t.Run("Exact", func(t *testing.T) {
		got, err := matchType("Conventional")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != "Conventional" {
			t.Errorf("got %q, wanted %q", got, "Conventional")
		}
	})

	// Verify that case and spacing are forgiven, since the entry has to match exactly
	t.Run("Loose", func(t *testing.T) {
		got, err := matchType("  p25 trunk ")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != "P25 Trunk" {
			t.Errorf("got %q, wanted %q", got, "P25 Trunk")
		}
	})

	// Verify that a type the scanner does not offer is refused with the choices
	t.Run("Unknown", func(t *testing.T) {
		_, err := matchType("Analog")
		if err == nil {
			t.Fatal("expected an error for a type the scanner does not offer, got none")
		}
		if !strings.Contains(err.Error(), "Conventional") {
			t.Errorf("expected the message to list the types, got: %v", err)
		}
	})
}

// Test_names tests the names function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - None: a scanner reporting no lists says so
//   - Several: the lists are named in quotes
func Test_names(t *testing.T) {
	// Verify that a scanner with no favorites lists is reported in words
	t.Run("None", func(t *testing.T) {
		if got := names(nil); got != "the scanner reports no favorites lists" {
			t.Errorf("got %q, wanted the note about no lists", got)
		}
	})

	// Verify that the alternatives are named, so nobody has to run another command
	t.Run("Several", func(t *testing.T) {
		got := names([]catalog.FavoritesList{
			{Name: "HOME", Index: "1"},
			{Name: "WORK", Index: "2"},
		})

		if got != `the scanner has "HOME", "WORK"` {
			t.Errorf("got %q, wanted the two names in quotes", got)
		}
	})
}

// Test_renderSystems tests the renderSystems function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Empty: a list holding no systems is reported as an answer
//   - Table: the systems are written as an aligned table
//   - FlushError: a failed write is reported
func Test_renderSystems(t *testing.T) {
	// Verify that a list holding nothing is explained rather than failed
	t.Run("Empty", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		if err := renderSystems(app, nil); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "no systems") {
			t.Errorf("expected a note about a list holding nothing, got: %q", notes.String())
		}
		if out.String() != "" {
			t.Errorf("expected nothing on the output, got: %q", out.String())
		}
	})

	// Verify that the table carries the name, type, whether it is scanned, and the keys
	t.Run("Table", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}

		err := renderSystems(app, []catalog.System{
			{Name: "FIRE", Index: "10", Kind: "P25 Trunk", QuickKey: "3", NumberTag: "7"},
			{Name: "RAIL", Index: "11", Kind: "Conventional", Avoided: true},
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		written := out.String()
		for _, want := range []string{"NAME", "FIRE", "P25 Trunk", "yes", "3", "7", "RAIL", "no", "-"} {
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

		err := renderSystems(app, []catalog.System{{Name: "FIRE", Index: "10", Kind: "P25 Trunk"}})
		if err == nil {
			t.Fatal("expected an error when the listing cannot be written, got none")
		}
		if !strings.Contains(err.Error(), "writing the system list") {
			t.Errorf("expected the message to say it was writing the list, got: %v", err)
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (9 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no favorites list carries is reported
//   - ReadError: a failed read of the systems is reported
//   - EmptyList: a list that exists and holds nothing is reported as that
//   - MissingList: an index no list carries is told apart from an empty one
//   - JSON: the systems are encoded when JSON was asked for
//   - Text: the systems are written as a table otherwise
//   - FullDatabaseByName: the database named as a name is refused
//   - FullDatabaseByIndex: the database named as an index is refused too
func Test_run(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes
		app.SetDevice(device.New(conn))
		return app, out, notes
	}

	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk" Avoid="Off"/></GLT>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := run(context.Background(), app, "1"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a favorites list that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := run(context.Background(), app, "HOME"); err == nil {
			t.Error("expected an error when the list cannot be resolved, got none")
		}
	})

	// Verify that a failed read of the systems is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"GLT,SYS,1": errors.New("the port is gone")}})

		if err := run(context.Background(), app, "1"); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that a list that really exists and holds nothing is reported as that
	t.Run("EmptyList", func(t *testing.T) {
		app, _, notes := appWith(&stubConn{docs: map[string][]string{
			"GLT,SYS,1": {`<GLT></GLT>`},
			"GLT,FL":    {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
		}})

		if err := run(context.Background(), app, "1"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "holds no systems") {
			t.Errorf("expected a note that the list holds nothing, got: %q", notes.String())
		}
	})

	// Verify that an index no list carries is told apart from a list holding nothing
	t.Run("MissingList", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,SYS,1": {`<GLT></GLT>`},
			"GLT,FL":    {`<GLT><FL Name="HOME" Index="2"/></GLT>`},
		}})

		err := run(context.Background(), app, "1")
		if err == nil {
			t.Fatal("expected an error when no list has that index, got none")
		}
		if !strings.Contains(err.Error(), "no favorites list has index 1") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})

	// Verify that asking for JSON encodes the systems rather than tabulating them
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{docs: map[string][]string{"GLT,SYS,1": {systems}}})
		app.Config.Output = appcontext.OutputJSON

		if err := run(context.Background(), app, "1"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var found []catalog.System
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if len(found) != 1 || found[0].Name != "FIRE" {
			t.Errorf("expected the system in the JSON, got: %v", found)
		}
	})

	// Verify that the default output is the aligned table
	t.Run("Text", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{docs: map[string][]string{"GLT,SYS,1": {systems}}})

		if err := run(context.Background(), app, "1"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "FIRE") {
			t.Errorf("expected the listing to name the system, got: %q", out.String())
		}
	})

	// Verify that naming the full database is refused rather than sent. This is
	// the command that leaves the scanner needing a power cycle.
	t.Run("FullDatabaseByName", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL": {`<GLT><FL Name="Full Database" Index="4294967295"/></GLT>`},
		}})

		err := run(context.Background(), app, "Full Database")
		if err == nil {
			t.Fatal("expected the full database to be refused, it was not")
		}
		if !strings.Contains(err.Error(), "power cycled") {
			t.Errorf("expected the message to say what it costs, got: %v", err)
		}
	})

	// Verify that the same refusal covers the index, since the reserved index is
	// as easy to type as the name
	t.Run("FullDatabaseByIndex", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{})

		if err := run(context.Background(), app, "4294967295"); err == nil {
			t.Fatal("expected the database's index to be refused, it was not")
		}
	})
}

// Test_runDelete tests the runDelete function with 100% coverage.
//
// Coverage: 100% (12 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no system carries is reported
//   - NameError: a system the scanner does not report is reported
//   - WithoutYes: nothing is deleted without --yes
//   - OpenMenuError: a refused menu jump is reported
//   - SelectError: a menu holding no delete entry is reported
//   - ConfirmError: a scanner not asking to confirm is reported
//   - AwakenError: a scanner that stops answering afterwards is reported
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the delete is reported
//   - StillThere: a system still on the scanner afterwards is reported
//   - Success: the system is deleted and named
func Test_runDelete(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The documents that carry one system, FIRE, in one favorites list.
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk" Avoid="Off"/></GLT>`

	// The walk through the system's menu: its delete entry, the prompt, and
	// the screen the scanner shows once it has finished deleting.
	walk := []string{deleteSystem, "Confirm Delete?", "Scanning"}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runDelete(context.Background(), app, "10", true); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a system that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runDelete(context.Background(), app, "FIRE", true); err == nil {
			t.Error("expected an error when the system cannot be resolved, got none")
		}
	})

	// Verify that an index no system carries is reported
	t.Run("NameError", func(t *testing.T) {
		app, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {`<GLT></GLT>`},
		}})

		err := runDelete(context.Background(), app, "10", true)
		if err == nil {
			t.Fatal("expected an error when no system has that index, got none")
		}
		if !strings.Contains(err.Error(), "no system has index") {
			t.Errorf("expected the message to say the index names nothing, got: %v", err)
		}
	})

	// Verify that nothing is pressed until --yes is given
	t.Run("WithoutYes", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems},
		}}
		app, _ := appWith(conn)

		err := runDelete(context.Background(), app, "10", false)
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
			docs: map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}},
			fail: map[string]error{"MNU,SCAN_SYSTEM,10": errors.New("the scanner refused the jump")},
		})

		err := runDelete(context.Background(), app, "10", true)
		if err == nil {
			t.Fatal("expected an error when the menu cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "opening the system's menu") {
			t.Errorf("expected the message to say it was opening the menu, got: %v", err)
		}
	})

	// Verify that a menu with no delete entry is reported
	t.Run("SelectError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{""},
			docs:    map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}},
		})

		err := runDelete(context.Background(), app, "10", true)
		if err == nil {
			t.Fatal("expected an error when the delete entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), deleteSystem) {
			t.Errorf("expected the message to name %q, got: %v", deleteSystem, err)
		}
	})

	// Verify that a scanner not asking to confirm is reported
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{deleteSystem, "Scanning"},
			docs:    map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}},
		})

		err := runDelete(context.Background(), app, "10", true)
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
			screens: []string{deleteSystem, "Confirm Delete?"},
			docs:    map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}},
		})

		err := runDelete(context.Background(), app, "10", true)
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
			docs:    map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}},
			fail:    map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runDelete(context.Background(), app, "10", true); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the delete is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"GLT,FL": {favorites, ""}, "GLT,SYS,1": {systems}},
		})

		if err := runDelete(context.Background(), app, "10", true); err == nil {
			t.Error("expected an error when the systems cannot be read back, got none")
		}
	})

	// Verify that a system still on the scanner afterwards is reported as not deleted
	t.Run("StillThere", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}},
		})

		err := runDelete(context.Background(), app, "10", true)
		if err == nil {
			t.Fatal("expected an error when the system is still there, got none")
		}
		if !strings.Contains(err.Error(), "nothing was deleted") {
			t.Errorf("expected the message to say nothing was deleted, got: %v", err)
		}
	})

	// Verify that a deleted system is named on the output
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: walk,
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems, `<GLT></GLT>`},
			},
		})

		if err := runDelete(context.Background(), app, "10", true); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "deleted FIRE") {
			t.Errorf("expected the output to name the deleted system, got: %q", out.String())
		}
	})
}

// Test_runNew tests the runNew function with 100% coverage.
//
// Coverage: 100% (11 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - NavigateError: a favorites list that cannot be reached is reported
//   - CreateError: a menu holding no New System entry is reported
//   - TypeError: a type picker that will not open the type is reported
//   - ConfirmError: a refused confirmation is reported as nothing created
//   - NameScreenError: a system created but not named is reported as that
//   - SetError: a name that cannot be typed is reported
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the create is reported
//   - NotFound: a system that does not appear afterwards is reported
//   - Success: the system is created, named, and reported with its type
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

	// The walk down to the type picker: the top menu's favorites entry, the
	// list itself, its systems, the entry that creates one, and the type.
	walk := []string{"Manage Favorites", "HOME", "Review/Edit System", newSystem, "Conventional"}

	// The text entry screen, holding the name already, so accepting it is one
	// press rather than a walk through the alphabet.
	entryScreen := `<MenuInfo Name="Name" MenuType="TypeInput" Value="AIRPORT">` +
		`<MenuInput MaxLength="16" EnableKeys="ABCDEFGHIJKLMNOPQRSTUVWXYZ "/></MenuInfo>`
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a favorites list that cannot be reached is reported
	t.Run("NavigateError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional"); err == nil {
			t.Error("expected an error when the list cannot be reached, got none")
		}
	})

	// Verify that a menu with no New System entry is reported
	t.Run("CreateError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk[:3],
			docs:    map[string][]string{"GLT,FL": {favorites}},
		})

		err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional")
		if err == nil {
			t.Fatal("expected an error when the system cannot be created, got none")
		}
		if !strings.Contains(err.Error(), "creating the system") {
			t.Errorf("expected the message to say it was creating the system, got: %v", err)
		}
	})

	// Verify that a type picker that will not offer the type is reported
	t.Run("TypeError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk[:4],
			docs:    map[string][]string{"GLT,FL": {favorites}},
		})

		err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional")
		if err == nil {
			t.Fatal("expected an error when the type cannot be chosen, got none")
		}
		if !strings.Contains(err.Error(), "Nothing has been created") {
			t.Errorf("expected the message to say nothing was created, got: %v", err)
		}
	})

	// Verify that a refused confirmation is reported as nothing created
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens:  walk,
			docs:     map[string][]string{"GLT,FL": {favorites}},
			failFrom: map[string]int{"KEY,E,P": 6},
		})

		err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional")
		if err == nil {
			t.Fatal("expected an error when the type cannot be confirmed, got none")
		}
		if !strings.Contains(err.Error(), "confirming the system type") {
			t.Errorf("expected the message to say it was confirming the type, got: %v", err)
		}
	})

	// Verify that a system created but left unnamed says so
	t.Run("NameScreenError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"GLT,FL": {favorites}},
		})

		err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional")
		if err == nil {
			t.Fatal("expected an error when the name screen cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "the system was created") {
			t.Errorf("expected the message to say the system was created, got: %v", err)
		}
	})

	// Verify that a name that cannot be typed is reported
	t.Run("SetError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: append(append([]string{}, walk...), editName),
			docs:    map[string][]string{"GLT,FL": {favorites}},
		})

		err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional")
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
			screens: append(append([]string{}, walk...), editName),
			docs: map[string][]string{
				"GLT,FL": {favorites},
				"MSI":    {entryScreen, outOfMenus},
			},
			fail: map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional"); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the create is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: append(append([]string{}, walk...), editName),
			docs: map[string][]string{
				"GLT,FL": {favorites, ""},
				"MSI":    {entryScreen, outOfMenus},
			},
		})

		if err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional"); err == nil {
			t.Error("expected an error when the systems cannot be read back, got none")
		}
	})

	// Verify that a system that does not appear afterwards is reported
	t.Run("NotFound", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: append(append([]string{}, walk...), editName),
			docs: map[string][]string{
				"GLT,FL":    {favorites},
				"GLT,SYS,1": {`<GLT></GLT>`},
				"MSI":       {entryScreen, outOfMenus},
			},
		})

		err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional")
		if err == nil {
			t.Fatal("expected an error when the system does not appear, got none")
		}
		if !strings.Contains(err.Error(), "does not appear") {
			t.Errorf("expected the message to say the system did not appear, got: %v", err)
		}
	})

	// Verify that a created system is reported with the type it was given
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: append(append([]string{}, walk...), editName),
			docs: map[string][]string{
				"GLT,FL":    {favorites},
				"GLT,SYS,1": {`<GLT><SYS Name="AIRPORT" Index="11" Type="Conventional" Avoid="Off"/></GLT>`},
				"MSI":       {entryScreen, outOfMenus},
			},
		})

		if err := runNew(context.Background(), app, "1", "AIRPORT", "Conventional"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "AIRPORT (Conventional)") {
			t.Errorf("expected the output to name the system and its type, got: %q", out.String())
		}
	})
}

// Test_runRename tests the runRename function with 100% coverage.
//
// Coverage: 100% (11 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no system carries is reported
//   - NameError: an index no system carries is reported
//   - Unchanged: a name the system already has costs nothing
//   - OpenMenuError: a refused menu jump is reported
//   - StepToError: a menu holding no name entry is reported
//   - EnterError: a refused key press is reported
//   - SetError: a name that cannot be typed is reported as nothing saved
//   - Success: the system is renamed and the new name written out
//   - JSON: the change is reported as an object under -o json
//   - WriteError: a stream the report cannot be written to is reported
func Test_runRename(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The text entry screen, holding the new name already, so accepting it is
	// one press rather than a walk through the alphabet.
	entryScreen := `<MenuInfo Name="Name" MenuType="TypeInput" Value="AIRPORT">` +
		`<MenuInput MaxLength="16" EnableKeys="ABCDEFGHIJKLMNOPQRSTUVWXYZ "/></MenuInfo>`
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// The documents that carry one system, FIRE, in one favorites list. The
	// rename reads the name the scanner has before touching anything, so that
	// it can report what the system was called and can decline to stop the
	// scan for a name it already has.
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk" Avoid="Off"/></GLT>`
	named := map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}}

	// withNamed is the documents above plus whatever else a case needs.
	withNamed := func(docs map[string][]string) map[string][]string {
		all := map[string][]string{}
		for k, v := range named {
			all[k] = v
		}
		for k, v := range docs {
			all[k] = v
		}
		return all
	}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runRename(context.Background(), app, "10", "AIRPORT"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a system that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runRename(context.Background(), app, "FIRE", "AIRPORT"); err == nil {
			t.Error("expected an error when the system cannot be resolved, got none")
		}
	})

	// Verify that an index no system carries is reported
	t.Run("NameError", func(t *testing.T) {
		app, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {`<GLT></GLT>`},
		}})

		err := runRename(context.Background(), app, "10", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when no system has that index, got none")
		}
		if !strings.Contains(err.Error(), "no system has index 10") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})

	// Verify that renaming a system to the name it already has changes nothing.
	// Driving the menus for it would stop the scan for no reason.
	t.Run("Unchanged", func(t *testing.T) {
		conn := &stubConn{docs: withNamed(nil)}
		app, out := appWith(conn)

		if err := runRename(context.Background(), app, "10", "FIRE"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if out.String() != "" {
			t.Errorf("expected nothing on the output, got: %q", out.String())
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
			docs: withNamed(nil),
			fail: map[string]error{"MNU,SCAN_SYSTEM,10": errors.New("the scanner refused the jump")},
		})

		err := runRename(context.Background(), app, "10", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the menu cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "opening the system's menu") {
			t.Errorf("expected the message to say it was opening the menu, got: %v", err)
		}
	})

	// Verify that a menu with no name entry is reported
	t.Run("StepToError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{""}, docs: withNamed(nil)})

		err := runRename(context.Background(), app, "10", "AIRPORT")
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
			docs:     withNamed(nil),
			failFrom: map[string]int{"KEY,E,P": 1},
		})

		if err := runRename(context.Background(), app, "10", "AIRPORT"); err == nil {
			t.Error("expected an error when the entry cannot be pressed, got none")
		}
	})

	// Verify that a name that cannot be typed is reported as nothing saved
	t.Run("SetError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{editName}, docs: withNamed(nil)})

		err := runRename(context.Background(), app, "10", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the name cannot be typed, got none")
		}
		if !strings.Contains(err.Error(), "nothing has been saved") {
			t.Errorf("expected the message to say nothing was saved, got: %v", err)
		}
	})

	// Verify that a renamed system is reported under its new name
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{editName},
			docs:    withNamed(map[string][]string{"MSI": {entryScreen, outOfMenus}}),
		})

		if err := runRename(context.Background(), app, "10", "AIRPORT"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "AIRPORT") {
			t.Errorf("expected the output to carry the new name, got: %q", out.String())
		}
	})

	// Verify that a rename reports the change as an object under -o json
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{editName},
			docs:    withNamed(map[string][]string{"MSI": {entryScreen, outOfMenus}}),
		})
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "10", "AIRPORT"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got render.Mutation
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the output is not JSON: %v\nstdout: %s", err, out.String())
		}
		if got.Action != "renamed" || got.Kind != "system" || got.Name != "AIRPORT" {
			t.Errorf("the report is %+v, wanted the rename and the new name", got)
		}
		if got.Was != "FIRE" {
			t.Errorf("the report says it was %q, wanted the name the scanner had", got.Was)
		}
	})

	// Verify that a stream the report cannot be written to is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{editName},
			docs:    withNamed(map[string][]string{"MSI": {entryScreen, outOfMenus}}),
		})
		reader, writer := io.Pipe()
		reader.Close()
		app.Stdout = writer
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "10", "AIRPORT"); err == nil {
			t.Error("expected an error when the report cannot be written, got none")
		}
	})
}

// Test_systemName tests the systemName function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Found: the scanner's own name for the system is reported
//   - NotFound: an index no system carries is reported
//   - ReadError: a failed read is reported
func Test_systemName(t *testing.T) {
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`

	// Verify that the name comes from the scanner rather than from the caller
	t.Run("Found", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL":    {favorites},
			"GLT,SYS,1": {`<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`},
		}}

		name, err := systemName(context.Background(), device.New(conn), "10")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if name != "FIRE" {
			t.Errorf("got name %q, wanted %q", name, "FIRE")
		}
	})

	// Verify that an index the scanner does not report is refused
	t.Run("NotFound", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {`<GLT></GLT>`},
		}}

		_, err := systemName(context.Background(), device.New(conn), "10")
		if err == nil {
			t.Fatal("expected an error when no system has that index, got none")
		}
		if !strings.Contains(err.Error(), "no system has index 10") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})

	// Verify that a failed read is reported rather than read as an empty memory
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if _, err := systemName(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})
}

// Test_renderSystemsPartial tests the branch that renders a system the scanner
// left out of its own list.
//
// Coverage: 100% (1 test case covering the remaining branch)
//
// Test cases:
//   - Partial: the columns nobody read say so, and a note explains the mark
func Test_renderSystemsPartial(t *testing.T) {
	// Verify that "unknown" is not printed as a confident "not scanned"
	t.Run("Partial", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		if err := renderSystems(app, []catalog.System{{Name: "FIRE", Partial: true}}); err != nil {
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
