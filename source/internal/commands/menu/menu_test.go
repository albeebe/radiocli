// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package menu

import (
	"bytes"
	"context"
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
//   - Wiring: the command is named, annotated, and carries its four subcommands
//   - Runs: executing the command reports the menu the scanner is showing
func TestNew(t *testing.T) {
	// Verify the command carries the name, the annotation and its subcommands
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "menu" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "menu")
		}
		if got := cmd.Annotations[appcontext.OnlyReads]; got != "true" {
			t.Errorf("the only-reads annotation is %q, wanted %q", got, "true")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		want := map[string]bool{"open": false, "back": false, "close": false, "set": false}
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

	// Verify running the command reads the scanner and reports what it found
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes
		app.SetDevice(device.New(&stubConn{execXML: func(string) (string, error) {
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(notes.String(), "not in a menu") {
			t.Errorf("the command wrote %q, wanted it to say the scanner is not in a menu", notes.String())
		}
	})
}

// Test_newBack covers the subcommand that climbs one level.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the scanner is sent back and the menu is reported
//   - NoDevice: a run with no scanner named is refused
//   - Fails: a scanner that refuses to go back is reported
func Test_newBack(t *testing.T) {
	// Verify going back sends the scanner's own back command
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{execXML: func(string) (string, error) {
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}
		app.SetDevice(device.New(conn))

		cmd := newBack(app)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}

		sent := false
		for _, command := range conn.sent {
			if command == "MSB," {
				sent = true
			}
		}
		if !sent {
			t.Errorf("the scanner was sent %v, wanted the back command", conn.sent)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newBack(app)
		if err := cmd.RunE(cmd, nil); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that refuses to go back is reported
	t.Run("Fails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}}))

		cmd := newBack(app)
		err := cmd.RunE(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "going back") {
			t.Errorf("the subcommand reported %v, wanted the refused step", err)
		}
	})
}

// Test_newClose covers the subcommand that leaves the menus.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the scanner is closed out of the menus and says so
//   - NoDevice: a run with no scanner named is refused
//   - Fails: a scanner that will not close the menu is reported
func Test_newClose(t *testing.T) {
	// Verify closing sends the scanner's own close command and reports it
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		conn := &stubConn{}
		app.SetDevice(device.New(conn))

		cmd := newClose(app)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}

		sent := false
		for _, command := range conn.sent {
			if command == "MSB,RETURN_PREVOUS_MODE" {
				sent = true
			}
		}
		if !sent {
			t.Errorf("the scanner was sent %v, wanted the close command", conn.sent)
		}
		if !strings.Contains(notes.String(), "left the menus") {
			t.Errorf("the subcommand wrote %q, wanted it to say the menus were left", notes.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newClose(app)
		if err := cmd.RunE(cmd, nil); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not leave the menus is reported
	t.Run("Fails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}}))

		cmd := newClose(app)
		err := cmd.RunE(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "closing the menu") {
			t.Errorf("the subcommand reported %v, wanted the refused close", err)
		}
	})
}

// Test_newOpen covers the subcommand that opens one of the scanner's menus.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Success: a menu named by word is opened and reported
//   - WithIndex: the second argument is passed as the menu's index
//   - UnknownMenu: a word that names no menu is refused with the list
//   - NoDevice: a run with no scanner named is refused
//   - Fails: a scanner that refuses the menu is reported
func Test_newOpen(t *testing.T) {
	// Verify a menu named by word is opened by the id the protocol uses
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{execXML: func(string) (string, error) {
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}
		app.SetDevice(device.New(conn))

		cmd := newOpen(app)
		if err := cmd.RunE(cmd, []string{"settings"}); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}

		sent := false
		for _, command := range conn.sent {
			if command == "MNU,SETTINGS," {
				sent = true
			}
		}
		if !sent {
			t.Errorf("the scanner was sent %v, wanted the settings menu", conn.sent)
		}
	})

	// Verify a second argument is passed to the scanner as the menu's index
	t.Run("WithIndex", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{execXML: func(string) (string, error) {
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}
		app.SetDevice(device.New(conn))

		cmd := newOpen(app)
		if err := cmd.RunE(cmd, []string{"system", "3"}); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}

		sent := false
		for _, command := range conn.sent {
			if command == "MNU,SCAN_SYSTEM,3" {
				sent = true
			}
		}
		if !sent {
			t.Errorf("the scanner was sent %v, wanted the system menu at index 3", conn.sent)
		}
	})

	// Verify a word that names no menu is refused with the names that work
	t.Run("UnknownMenu", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newOpen(app)
		err := cmd.RunE(cmd, []string{"nonsense"})
		if err == nil || !strings.Contains(err.Error(), "no menu is called") {
			t.Fatalf("the subcommand reported %v, wanted a refusal naming the menus", err)
		}
		if !strings.Contains(err.Error(), "settings") {
			t.Errorf("the refusal is %q, wanted it to list the menus that work", err)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newOpen(app)
		if err := cmd.RunE(cmd, []string{"top"}); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that refuses to open the menu is reported by name
	t.Run("Fails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the scanner refused the menu")
		}}))

		cmd := newOpen(app)
		err := cmd.RunE(cmd, []string{"weather"})
		if err == nil || !strings.Contains(err.Error(), "opening the weather menu") {
			t.Errorf("the subcommand reported %v, wanted the refused menu", err)
		}
	})
}

// Test_newSet covers the subcommand that writes a menu item's value.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the value is written and the menu is reported
//   - NoDevice: a run with no scanner named is refused
//   - Fails: a value the scanner will not take is reported
func Test_newSet(t *testing.T) {
	// Verify the value is written with the scanner's own set command
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{execXML: func(string) (string, error) {
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}
		app.SetDevice(device.New(conn))

		cmd := newSet(app)
		if err := cmd.RunE(cmd, []string{"Fire Dispatch"}); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}

		sent := false
		for _, command := range conn.sent {
			if command == "MSV,Fire Dispatch" {
				sent = true
			}
		}
		if !sent {
			t.Errorf("the scanner was sent %v, wanted the value", conn.sent)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newSet(app)
		if err := cmd.RunE(cmd, []string{"anything"}); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})

	// Verify a value the scanner will not take is reported, quoting it
	t.Run("Fails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the scanner refused the value")
		}}))

		cmd := newSet(app)
		err := cmd.RunE(cmd, []string{"nope"})
		if err == nil || !strings.Contains(err.Error(), `setting the menu value to "nope"`) {
			t.Errorf("the subcommand reported %v, wanted the refused value", err)
		}
	})
}

// Test_show covers reading the menu the scanner is on.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the menu on screen is reported
//   - NoDevice: a run with no scanner named is refused
//   - Fails: a scanner that will not answer is reported
func Test_show(t *testing.T) {
	// Verify the menu the scanner is showing is read and written out
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{
			exec: func(string) (string, error) {
				return "0,Settings,*", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Menu" MenuType="TypeSelect">` +
					`<MenuItem Name="Settings" Index="1"/></MenuInfo>`, nil
			},
		}))

		if err := show(context.Background(), app); err != nil {
			t.Fatalf("show: %v", err)
		}
		if !strings.Contains(out.String(), "menu: Menu") {
			t.Errorf("show wrote %q, wanted it to name the menu", out.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := show(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("show reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not answer is reported as a failed read
	t.Run("Fails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := show(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "reading the menu") {
			t.Errorf("show reported %v, wanted the failed read", err)
		}
	})
}
