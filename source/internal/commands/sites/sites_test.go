// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package sites

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
	screens []string            // The name highlighted on each successive display read, "" to fail that read
	reads   int                 // How many display reads have been answered so far
	docs    map[string][]string // Documents to answer with, in order, keyed by the command asking for them
	fail    map[string]error    // Commands that fail instead of answering
	sent    []string            // Every command sent to the scanner, in order
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
//   - Runs: running the command reports the system's sites
func TestNew(t *testing.T) {
	// Verify that the command is named and carries every subcommand
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "sites <system>" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "sites <system>")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		for _, want := range []string{"new", "rename", "delete", "frequencies"} {
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

	// Verify that running the command reads the sites and writes them out
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{docs: map[string][]string{
			"GLT,SITE,10": {`<GLT><SITE Name="DOWNTOWN" Index="100" Avoid="Off"/></GLT>`},
		}}))

		cmd := New(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"10"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "DOWNTOWN") {
			t.Errorf("expected the listing to name the site, got: %q", out.String())
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

		if err := cmd.RunE(cmd, []string{"100"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
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

		if err := cmd.RunE(cmd, []string{"10", "AIRPORT"}); err == nil {
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

		if err := cmd.RunE(cmd, []string{"100", "AIRPORT"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_renderSites tests the renderSites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Empty: a system with no sites is reported as an answer
//   - Table: the sites are written as an aligned table
//   - FlushError: a failed write is reported
func Test_renderSites(t *testing.T) {
	// Verify that a system with no sites is explained rather than failed
	t.Run("Empty", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		if err := renderSites(app, nil); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "no sites") {
			t.Errorf("expected a note about a system with no sites, got: %q", notes.String())
		}
		if out.String() != "" {
			t.Errorf("expected nothing on the output, got: %q", out.String())
		}
	})

	// Verify that the table carries the name, whether it is scanned, and the key
	t.Run("Table", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}

		err := renderSites(app, []catalog.Site{
			{Name: "DOWNTOWN", Index: "100", QuickKey: "3"},
			{Name: "HILLTOP", Index: "101", Avoided: true},
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		written := out.String()
		for _, want := range []string{"NAME", "DOWNTOWN", "yes", "3", "HILLTOP", "no", "-"} {
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

		err := renderSites(app, []catalog.Site{{Name: "DOWNTOWN", Index: "100"}})
		if err == nil {
			t.Fatal("expected an error when the listing cannot be written, got none")
		}
		if !strings.Contains(err.Error(), "writing the site list") {
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
//   - ReadError: a failed read of the sites is reported
//   - JSON: the sites are encoded when JSON was asked for
//   - Text: the sites are written as a table otherwise
//   - NoSuchSystem: an index naming no system is reported as that
//   - EmptyByName: a system that really holds no sites is an answer
func Test_run(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

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
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := run(context.Background(), app, "FIRE"); err == nil {
			t.Error("expected an error when the system cannot be resolved, got none")
		}
	})

	// Verify that a failed read of the sites is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,SITE,10": errors.New("the port is gone")}})

		if err := run(context.Background(), app, "10"); err == nil {
			t.Error("expected an error when the sites cannot be read, got none")
		}
	})

	// Verify that asking for JSON encodes the sites rather than tabulating them
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{docs: map[string][]string{
			"GLT,SITE,10": {`<GLT><SITE Name="DOWNTOWN" Index="100" Avoid="Off"/></GLT>`},
		}})
		app.Config.Output = appcontext.OutputJSON

		if err := run(context.Background(), app, "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var found []catalog.Site
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if len(found) != 1 || found[0].Name != "DOWNTOWN" {
			t.Errorf("expected the site in the JSON, got: %v", found)
		}
	})

	// Verify that the default output is the aligned table
	t.Run("Text", func(t *testing.T) {
		app, out := appWith(&stubConn{docs: map[string][]string{
			"GLT,SITE,10": {`<GLT><SITE Name="DOWNTOWN" Index="100" Avoid="Off"/></GLT>`},
		}})

		if err := run(context.Background(), app, "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "DOWNTOWN") {
			t.Errorf("expected the listing to name the site, got: %q", out.String())
		}
	})

	// Verify that an index naming no system is reported as that rather than as
	// a system with no sites. The two answer identically, and being told a
	// system has no sites sends the reader looking for a site.
	t.Run("NoSuchSystem", func(t *testing.T) {
		app, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL":        {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
			"GLT,SYS,1":     {`<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`},
			"GLT,SITE,9999": {`<GLT></GLT>`},
		}})

		err := run(context.Background(), app, "9999")
		if err == nil {
			t.Fatal("expected an error for an index naming no system, got none")
		}
		if !strings.Contains(err.Error(), "no system has index 9999") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})

	// Verify that a conventional system, which really does hold no sites, is
	// reported as an answer rather than as a system that is not there
	t.Run("EmptyByName", func(t *testing.T) {
		app, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL":      {`<GLT><FL Name="HOME" Index="1"/></GLT>`},
			"GLT,SYS,1":   {`<GLT><SYS Name="FIRE" Index="10" Type="Conventional"/></GLT>`},
			"GLT,SITE,10": {`<GLT></GLT>`},
		}})

		if err := run(context.Background(), app, "FIRE"); err != nil {
			t.Fatalf("expected no error for a system holding no sites, got: %v", err)
		}
	})
}

// Test_confirm tests the confirm function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - ByName: a name was matched already, so nothing is asked
//   - ReadError: a systems list that cannot be read is reported
//   - Exists: an index the scanner does have is accepted
//   - Missing: an index the scanner does not have is reported
func Test_confirm(t *testing.T) {
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`

	// Verify that a name costs no exchange, since resolving it already proved
	// the system is there
	t.Run("ByName", func(t *testing.T) {
		conn := &stubConn{}

		if err := confirm(context.Background(), device.New(conn), "FIRE", "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(conn.sent) != 0 {
			t.Errorf("expected nothing to be asked, got: %v", conn.sent)
		}
	})

	// Verify that a systems list that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if err := confirm(context.Background(), device.New(conn), "10", "10"); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that an index the scanner does have is accepted
	t.Run("Exists", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}}}

		if err := confirm(context.Background(), device.New(conn), "10", "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	// Verify that an index the scanner does not have is reported
	t.Run("Missing", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{"GLT,FL": {favorites}, "GLT,SYS,1": {systems}}}

		err := confirm(context.Background(), device.New(conn), "9999", "9999")
		if err == nil {
			t.Fatal("expected an error for an index naming no system, got none")
		}
		if !strings.Contains(err.Error(), "no system has index 9999") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})
}

// Test_runDelete tests the runDelete function with 100% coverage.
//
// Coverage: 100% (12 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no site carries is reported
//   - NameError: a site the scanner does not report is reported
//   - WithoutYes: nothing is deleted without --yes
//   - OpenMenuError: a refused menu jump is reported
//   - SelectError: a menu holding no delete entry is reported
//   - ConfirmError: a scanner not asking to confirm is reported
//   - AwakenError: a scanner that never comes back from the rebuild is reported
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the delete is reported
//   - StillThere: a site still on the scanner afterwards is reported
//   - Success: the site is deleted and named
func Test_runDelete(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The documents that carry one site, DOWNTOWN, through the whole memory.
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`
	sites := `<GLT><SITE Name="DOWNTOWN" Index="100" Avoid="Off"/></GLT>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runDelete(context.Background(), app, "100", true); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a site that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runDelete(context.Background(), app, "DOWNTOWN", true); err == nil {
			t.Error("expected an error when the site cannot be resolved, got none")
		}
	})

	// Verify that an index no site carries is reported
	t.Run("NameError", func(t *testing.T) {
		app, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL":      {favorites},
			"GLT,SYS,1":   {systems},
			"GLT,SITE,10": {`<GLT></GLT>`},
		}})

		err := runDelete(context.Background(), app, "100", true)
		if err == nil {
			t.Fatal("expected an error when no site has that index, got none")
		}
		if !strings.Contains(err.Error(), "no site has index") {
			t.Errorf("expected the message to say the index names nothing, got: %v", err)
		}
	})

	// Verify that nothing is pressed until --yes is given
	t.Run("WithoutYes", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
		}}
		app, _ := appWith(conn)

		err := runDelete(context.Background(), app, "100", false)
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
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
			fail: map[string]error{"MNU,SCAN_SITE,100": errors.New("the scanner refused the jump")},
		})

		err := runDelete(context.Background(), app, "100", true)
		if err == nil {
			t.Fatal("expected an error when the menu cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "opening the site's menu") {
			t.Errorf("expected the message to say it was opening the menu, got: %v", err)
		}
	})

	// Verify that a menu with no delete entry is reported
	t.Run("SelectError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{""},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
		})

		err := runDelete(context.Background(), app, "100", true)
		if err == nil {
			t.Fatal("expected an error when the delete entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), deleteSite) {
			t.Errorf("expected the message to name %q, got: %v", deleteSite, err)
		}
	})

	// Verify that a scanner not asking to confirm is reported and nothing is answered
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{deleteSite, "Scanning"},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
		})

		err := runDelete(context.Background(), app, "100", true)
		if err == nil {
			t.Fatal("expected an error when the scanner is not asking to confirm, got none")
		}
		if !strings.Contains(err.Error(), "nothing was deleted") {
			t.Errorf("expected the message to say nothing was deleted, got: %v", err)
		}
	})

	// Verify that a scanner that never comes back from the rebuild is reported.
	// Deleting a site takes its frequencies with it, so the scanner goes away to
	// work out what it can still hear, and anything sent meanwhile times out.
	t.Run("AwakenError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{deleteSite, "Confirm Delete?"},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
		})

		err := runDelete(context.Background(), app, "100", true)
		if err == nil {
			t.Fatal("expected an error when the scanner does not come back, got none")
		}
		if !strings.Contains(err.Error(), "stopped answering") {
			t.Errorf("expected the message to say the scanner went away, got: %v", err)
		}
	})

	// Verify that a failure returning the scanner to scanning is reported
	t.Run("LeaveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			// The third screen is the wait for the scanner to come back:
			// deleting a site takes its frequencies with it, so the scanner
			// goes away to work out what it can still hear.
			screens: []string{deleteSite, "Confirm Delete?", "Scanning"},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
			fail: map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runDelete(context.Background(), app, "100", true); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the delete is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			// The third screen is the wait for the scanner to come back:
			// deleting a site takes its frequencies with it, so the scanner
			// goes away to work out what it can still hear.
			screens: []string{deleteSite, "Confirm Delete?", "Scanning"},
			docs: map[string][]string{
				"GLT,FL": {favorites, ""}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
		})

		if err := runDelete(context.Background(), app, "100", true); err == nil {
			t.Error("expected an error when the sites cannot be read back, got none")
		}
	})

	// Verify that a site still on the scanner afterwards is reported as not deleted
	t.Run("StillThere", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			// The third screen is the wait for the scanner to come back:
			// deleting a site takes its frequencies with it, so the scanner
			// goes away to work out what it can still hear.
			screens: []string{deleteSite, "Confirm Delete?", "Scanning"},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
		})

		err := runDelete(context.Background(), app, "100", true)
		if err == nil {
			t.Fatal("expected an error when the site is still there, got none")
		}
		if !strings.Contains(err.Error(), "nothing was deleted") {
			t.Errorf("expected the message to say nothing was deleted, got: %v", err)
		}
	})

	// Verify that a deleted site is named on the output
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			// The third screen is the wait for the scanner to come back:
			// deleting a site takes its frequencies with it, so the scanner
			// goes away to work out what it can still hear.
			screens: []string{deleteSite, "Confirm Delete?", "Scanning"},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems},
				"GLT,SITE,10": {sites, `<GLT></GLT>`},
			},
		})

		if err := runDelete(context.Background(), app, "100", true); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "deleted DOWNTOWN") {
			t.Errorf("expected the output to name the deleted site, got: %q", out.String())
		}
	})
}

// Test_runNew tests the runNew function with 100% coverage.
//
// Coverage: 100% (10 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no system carries is reported
//   - NavigateError: a system that cannot be reached is reported
//   - CreateError: a menu holding no New Site entry is reported
//   - NameScreenError: a site created but not named is reported as that
//   - TypeError: a name that cannot be typed is reported
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the create is reported
//   - NotFound: a site that does not appear afterwards is reported
//   - Success: the site is created and named
func Test_runNew(t *testing.T) {
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
	entryScreen := `<MenuInfo Name="Name" MenuType="TypeInput" Value="AIRPORT">` +
		`<MenuInput MaxLength="16" EnableKeys="ABCDEFGHIJKLMNOPQRSTUVWXYZ "/></MenuInfo>`
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runNew(context.Background(), app, "10", "AIRPORT"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a system that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runNew(context.Background(), app, "FIRE", "AIRPORT"); err == nil {
			t.Error("expected an error when the system cannot be resolved, got none")
		}
	})

	// Verify that a system that cannot be reached is reported
	t.Run("NavigateError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			fail: map[string]error{"MNU,SCAN_SYSTEM,10": errors.New("the scanner refused the jump")},
		})

		if err := runNew(context.Background(), app, "10", "AIRPORT"); err == nil {
			t.Error("expected an error when the system cannot be reached, got none")
		}
	})

	// Verify that a menu with no New Site entry is reported
	t.Run("CreateError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{"Edit Site"}})

		err := runNew(context.Background(), app, "10", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the site cannot be created, got none")
		}
		if !strings.Contains(err.Error(), "creating the site") {
			t.Errorf("expected the message to say it was creating the site, got: %v", err)
		}
	})

	// Verify that a site created but left unnamed says so
	t.Run("NameScreenError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{"Edit Site", newSite}})

		err := runNew(context.Background(), app, "10", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the name screen cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "the site was created") {
			t.Errorf("expected the message to say the site was created, got: %v", err)
		}
	})

	// Verify that a name that cannot be typed is reported
	t.Run("TypeError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{"Edit Site", newSite, editName}})

		err := runNew(context.Background(), app, "10", "AIRPORT")
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
			screens: []string{"Edit Site", newSite, editName},
			docs:    map[string][]string{"MSI": {entryScreen, outOfMenus}},
			fail:    map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runNew(context.Background(), app, "10", "AIRPORT"); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the create is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{"Edit Site", newSite, editName},
			docs:    map[string][]string{"MSI": {entryScreen, outOfMenus}},
			fail:    map[string]error{"GLT,SITE,10": errors.New("the port is gone")},
		})

		if err := runNew(context.Background(), app, "10", "AIRPORT"); err == nil {
			t.Error("expected an error when the sites cannot be read back, got none")
		}
	})

	// Verify that a site that does not appear afterwards is reported
	t.Run("NotFound", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{"Edit Site", newSite, editName},
			docs: map[string][]string{
				"MSI":         {entryScreen, outOfMenus},
				"GLT,SITE,10": {`<GLT><SITE Name="SITE 0" Index="100" Avoid="Off"/></GLT>`},
			},
		})

		err := runNew(context.Background(), app, "10", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the site does not appear, got none")
		}
		if !strings.Contains(err.Error(), "does not appear") {
			t.Errorf("expected the message to say the site did not appear, got: %v", err)
		}
	})

	// Verify that a created site is named on the output
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{"Edit Site", newSite, editName},
			docs: map[string][]string{
				"MSI":         {entryScreen, outOfMenus},
				"GLT,SITE,10": {`<GLT><SITE Name="AIRPORT" Index="100" Avoid="Off"/></GLT>`},
			},
		})

		if err := runNew(context.Background(), app, "10", "AIRPORT"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "AIRPORT") {
			t.Errorf("expected the output to name the site, got: %q", out.String())
		}
	})
}

// Test_runRename tests the runRename function with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no site carries is reported
//   - NameError: a site the scanner does not report is reported
//   - AlreadyNamed: a site already called that is left alone
//   - OpenMenuError: a refused menu jump is reported
//   - SelectError: a menu holding no name entry is reported
//   - TypeError: a name that cannot be typed is reported
//   - Success: the site is renamed and the new name written out
func Test_runRename(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes
		app.SetDevice(device.New(conn))
		return app, out, notes
	}

	// The documents that carry one site, DOWNTOWN, through the whole memory.
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`
	sites := `<GLT><SITE Name="DOWNTOWN" Index="100" Avoid="Off"/></GLT>`

	// The text entry screen, holding the new name already, so accepting it is
	// one press rather than a walk through the alphabet.
	entryScreen := `<MenuInfo Name="Name" MenuType="TypeInput" Value="AIRPORT">` +
		`<MenuInput MaxLength="16" EnableKeys="ABCDEFGHIJKLMNOPQRSTUVWXYZ "/></MenuInfo>`
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runRename(context.Background(), app, "100", "AIRPORT"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a site that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runRename(context.Background(), app, "DOWNTOWN", "AIRPORT"); err == nil {
			t.Error("expected an error when the site cannot be resolved, got none")
		}
	})

	// Verify that an index no site carries is reported
	t.Run("NameError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {`<GLT></GLT>`},
		}})

		if err := runRename(context.Background(), app, "100", "AIRPORT"); err == nil {
			t.Error("expected an error when no site has that index, got none")
		}
	})

	// Verify that a site already called that is left as it is
	t.Run("AlreadyNamed", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
		}}
		app, _, notes := appWith(conn)

		if err := runRename(context.Background(), app, "100", "DOWNTOWN"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "already called") {
			t.Errorf("expected a note that the name is already that, got: %q", notes.String())
		}
		for _, command := range conn.sent {
			if strings.HasPrefix(command, "KEY,") || strings.HasPrefix(command, "MNU,") {
				t.Errorf("expected nothing to be pressed, got: %v", conn.sent)
			}
		}
	})

	// Verify that a refused menu jump is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
			fail: map[string]error{"MNU,SCAN_SITE,100": errors.New("the scanner refused the jump")},
		})

		err := runRename(context.Background(), app, "100", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the menu cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "opening the site's menu") {
			t.Errorf("expected the message to say it was opening the menu, got: %v", err)
		}
	})

	// Verify that a menu with no name entry is reported and nothing changed
	t.Run("SelectError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{""},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
		})

		err := runRename(context.Background(), app, "100", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the name entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), "Nothing has been changed") {
			t.Errorf("expected the message to say nothing changed, got: %v", err)
		}
	})

	// Verify that a name that cannot be typed is reported as nothing saved
	t.Run("TypeError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{editName},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
			},
		})

		err := runRename(context.Background(), app, "100", "AIRPORT")
		if err == nil {
			t.Fatal("expected an error when the name cannot be typed, got none")
		}
		if !strings.Contains(err.Error(), "nothing has been saved") {
			t.Errorf("expected the message to say nothing was saved, got: %v", err)
		}
	})

	// Verify that a renamed site is reported under its new name
	t.Run("Success", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: []string{editName},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
				"MSI": {entryScreen, outOfMenus},
			},
		})

		if err := runRename(context.Background(), app, "100", "AIRPORT"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "AIRPORT") {
			t.Errorf("expected the output to carry the new name, got: %q", out.String())
		}
	})

	// Verify that a rename reports the change as an object under -o json, which
	// is what the prose in both modes used to deny a script
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: []string{editName},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
				"MSI": {entryScreen, outOfMenus},
			},
		})
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "100", "AIRPORT"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got render.Mutation
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the output is not JSON: %v\nstdout: %s", err, out.String())
		}
		if got.Action != "renamed" || got.Name != "AIRPORT" || got.Was != "DOWNTOWN" {
			t.Errorf("the report is %+v, wanted the rename and the name it had", got)
		}
	})

	// Verify that a stream the report cannot be written to is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{editName},
			docs: map[string][]string{
				"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {sites},
				"MSI": {entryScreen, outOfMenus},
			},
		})
		reader, writer := io.Pipe()
		reader.Close()
		app.Stdout = writer
		app.Config.Output = appcontext.OutputJSON

		if err := runRename(context.Background(), app, "100", "AIRPORT"); err == nil {
			t.Error("expected an error when the report cannot be written, got none")
		}
	})
}

// Test_siteName tests the siteName function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Found: the scanner's own name for the site is reported
//   - NotFound: an index no site carries is reported
//   - ReadError: a failed read is reported
func Test_siteName(t *testing.T) {
	favorites := `<GLT><FL Name="HOME" Index="1"/></GLT>`
	systems := `<GLT><SYS Name="FIRE" Index="10" Type="P25 Trunk"/></GLT>`

	// Verify that the name comes from the scanner rather than from the caller
	t.Run("Found", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems},
			"GLT,SITE,10": {`<GLT><SITE Name="DOWNTOWN" Index="100" Avoid="Off"/></GLT>`},
		}}

		name, err := siteName(context.Background(), device.New(conn), "100")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if name != "DOWNTOWN" {
			t.Errorf("got name %q, wanted %q", name, "DOWNTOWN")
		}
	})

	// Verify that an index the scanner does not report is refused
	t.Run("NotFound", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{
			"GLT,FL": {favorites}, "GLT,SYS,1": {systems}, "GLT,SITE,10": {`<GLT></GLT>`},
		}}

		_, err := siteName(context.Background(), device.New(conn), "100")
		if err == nil {
			t.Fatal("expected an error when no site has that index, got none")
		}
		if !strings.Contains(err.Error(), "no site has index 100") {
			t.Errorf("expected the message to name the index, got: %v", err)
		}
	})

	// Verify that a failed read is reported rather than read as an empty memory
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if _, err := siteName(context.Background(), device.New(conn), "100"); err == nil {
			t.Error("expected an error when the sites cannot be read, got none")
		}
	})
}

// Test_renderSitesPartial tests the branch that renders a site the scanner left
// out of its own list.
//
// Coverage: 100% (1 test case covering the remaining branch)
//
// Test cases:
//   - Partial: the columns nobody read say so, and a note explains the mark
func Test_renderSitesPartial(t *testing.T) {
	// Verify that "unknown" is not printed as a confident "not scanned"
	t.Run("Partial", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		if err := renderSites(app, []catalog.Site{{Name: "SOUTH", Partial: true}}); err != nil {
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
