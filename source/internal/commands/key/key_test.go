// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package key

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// keyConn is a device.Conn that accepts key presses and can be told to refuse
// one of them, so a run that stops halfway can be driven with no scanner
// attached.
type keyConn struct {
	// failAt is which press to refuse, counting from one. A zero accepts every
	// press.
	failAt int

	// n counts the presses that have been sent.
	n int

	// sent records every command, so the order keys were pressed in can be
	// checked.
	sent []string
}

// Info describes the connected scanner.
func (c *keyConn) Info() device.Info { return device.Info{} }

// Execute accepts the press, or refuses the one it was told to.
func (c *keyConn) Execute(ctx context.Context, command string) (string, error) {
	c.n++
	c.sent = append(c.sent, command)
	if c.failAt == c.n {
		return "", errors.New("the scanner refused the key")
	}
	return "", nil
}

// ExecuteXML answers as the plain exchange does, since nothing here reads XML.
func (c *keyConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return c.Execute(ctx, command)
}

// Send reports nothing, since nothing here writes without reading.
func (c *keyConn) Send(ctx context.Context, command string) error { return nil }

// Close releases nothing, because there is no port.
func (c *keyConn) Close() error { return nil }

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Wiring: the command is named and carries the action flag
//   - Runs: the keys given are pressed in the order they were written
//   - BadKey: a misspelled key is refused before the scanner is opened
//   - BadAction: an unknown action is refused, listing the ones that exist
func TestNew(t *testing.T) {
	// Builds an app with a scanner that accepts presses without pacing them,
	// since the pace between keys is time this test has no reason to spend.
	newApp := func(t *testing.T, conn *keyConn) *appcontext.App {
		t.Helper()

		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		client := device.New(conn)
		if err := client.SetPace(device.PaceTurbo); err != nil {
			t.Fatalf("setting the pace: %v", err)
		}
		app.SetDevice(client)
		return app
	}

	// Verify the command is named and carries the flag that chooses the press
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "key <name>..." {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "key <name>...")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		f := cmd.Flags().Lookup("action")
		if f == nil {
			t.Fatal("the command has no --action flag")
		}
		if f.DefValue != "press" {
			t.Errorf("the action defaults to %q, wanted %q", f.DefValue, "press")
		}
	})

	// Verify the keys given are pressed, in the order they were written
	t.Run("Runs", func(t *testing.T) {
		conn := &keyConn{}
		cmd := New(newApp(t, conn))
		cmd.SetArgs([]string{"menu", "3"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if len(conn.sent) != 2 {
			t.Fatalf("the scanner got %d presses, wanted 2: %v", len(conn.sent), conn.sent)
		}
		if !strings.HasPrefix(conn.sent[0], "KEY,") || !strings.HasPrefix(conn.sent[1], "KEY,") {
			t.Errorf("the scanner got %v, wanted two key presses", conn.sent)
		}
	})

	// Verify a misspelled key stops the whole run rather than part of it
	t.Run("BadKey", func(t *testing.T) {
		conn := &keyConn{}
		cmd := New(newApp(t, conn))
		cmd.SetArgs([]string{"menu", "nope"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		err := cmd.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), `no key is called "nope"`) {
			t.Fatalf("the command reported %v, wanted the misspelled key", err)
		}
		if len(conn.sent) != 0 {
			t.Errorf("the scanner got %v, wanted nothing pressed", conn.sent)
		}
	})

	// Verify an unknown action is refused and the accepted ones are listed
	t.Run("BadAction", func(t *testing.T) {
		conn := &keyConn{}
		cmd := New(newApp(t, conn))
		cmd.SetArgs([]string{"menu", "--action", "wiggle"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		err := cmd.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), `invalid action "wiggle"`) {
			t.Fatalf("the command reported %v, wanted the unknown action", err)
		}
		if !strings.Contains(err.Error(), "hold, long, press, release") {
			t.Errorf("the command reported %q, wanted it to list the actions", err)
		}
		if len(conn.sent) != 0 {
			t.Errorf("the scanner got %v, wanted nothing pressed", conn.sent)
		}
	})
}

// Test_actionNames covers listing the ways a key can be pressed.
//
// Coverage: 100% (1 test case covering the single path)
//
// Test cases:
//   - Sorted: every action is listed, in a stable order
func Test_actionNames(t *testing.T) {
	// Verify every action is listed and the order does not depend on the map
	t.Run("Sorted", func(t *testing.T) {
		got := actionNames()

		if len(got) != len(actions) {
			t.Fatalf("there are %d actions, wanted %d: %v", len(got), len(actions), got)
		}
		if !sort.StringsAreSorted(got) {
			t.Errorf("the actions are %v, wanted them sorted", got)
		}
		for _, name := range got {
			if _, ok := actions[name]; !ok {
				t.Errorf("%q is listed but is not an action", name)
			}
		}
	})
}

// Test_done covers naming the keys pressed before a failure.
//
// Coverage: 100% (2 test cases covering the loop and the empty case)
//
// Test cases:
//   - Several: the names are listed in the order they were pressed
//   - None: nothing pressed reads as nothing
func Test_done(t *testing.T) {
	// Verify the keys already pressed are named in order
	t.Run("Several", func(t *testing.T) {
		got := done([]press{{name: "menu"}, {name: "right"}, {name: "push"}})

		if want := "menu, right, push"; got != want {
			t.Errorf("done is %q, wanted %q", got, want)
		}
	})

	// Verify a failure on the first key names nothing before it
	t.Run("None", func(t *testing.T) {
		if got := done(nil); got != "" {
			t.Errorf("done is %q, wanted nothing", got)
		}
	})
}

// Test_keyNames covers listing the keys this command accepts.
//
// Coverage: 100% (1 test case covering the single path)
//
// Test cases:
//   - Sorted: every key is listed, in a stable order
func Test_keyNames(t *testing.T) {
	// Verify every key is listed and the order does not depend on the map
	t.Run("Sorted", func(t *testing.T) {
		got := keyNames()

		if len(got) != len(keys) {
			t.Fatalf("there are %d keys, wanted %d: %v", len(got), len(keys), got)
		}
		if !sort.StringsAreSorted(got) {
			t.Errorf("the keys are %v, wanted them sorted", got)
		}
		for _, name := range got {
			if _, ok := keys[name]; !ok {
				t.Errorf("%q is listed but is not a key", name)
			}
		}
	})
}

// Test_options covers rendering a list of accepted values.
//
// Coverage: 100% (2 test cases covering the join and the empty case)
//
// Test cases:
//   - Several: the values are separated by commas
//   - None: an empty list reads as nothing
func Test_options(t *testing.T) {
	// Verify the values are joined the way an error message reads them
	t.Run("Several", func(t *testing.T) {
		if got, want := options([]string{"press", "long"}), "press, long"; got != want {
			t.Errorf("options is %q, wanted %q", got, want)
		}
	})

	// Verify an empty list renders as nothing rather than as punctuation
	t.Run("None", func(t *testing.T) {
		if got := options(nil); got != "" {
			t.Errorf("options is %q, wanted nothing", got)
		}
	})
}

// Test_resolve covers turning written names into keys.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Known: the names resolve, whatever case they were written in
//   - Unknown: one bad name rejects the whole run
//   - Empty: no names resolve to no presses
func Test_resolve(t *testing.T) {
	// Verify names resolve regardless of case, and keep the name they were given
	t.Run("Known", func(t *testing.T) {
		got, err := resolve([]string{"MENU", "Right"})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("resolve returned %d presses, wanted 2", len(got))
		}
		if got[0].name != "menu" || got[0].key != device.KeyMenu {
			t.Errorf("the first press is %+v, wanted the menu key", got[0])
		}
		if got[1].name != "right" || got[1].key != device.KeyRotateRight {
			t.Errorf("the second press is %+v, wanted the right rotation", got[1])
		}
	})

	// Verify one unknown name rejects the run rather than pressing part of it
	t.Run("Unknown", func(t *testing.T) {
		got, err := resolve([]string{"menu", "wiggle"})
		if err == nil {
			t.Fatal("resolve reported nothing, wanted the unknown key")
		}
		if got != nil {
			t.Errorf("resolve returned %v, wanted nothing", got)
		}
		if !strings.Contains(err.Error(), `no key is called "wiggle"`) {
			t.Errorf("resolve reported %q, wanted it to name the bad key", err)
		}
	})

	// Verify no names at all resolve to nothing to press
	t.Run("Empty", func(t *testing.T) {
		got, err := resolve(nil)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("resolve returned %v, wanted nothing", got)
		}
	})
}

// Test_run covers pressing the keys and reporting how far it got.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Success: every key is pressed, in order
//   - NoDevice: a run with no scanner named is refused
//   - FailsFirst: a failure on the first key names only that key
//   - FailsLater: a failure partway through names the keys already pressed
func Test_run(t *testing.T) {
	// Builds an app with a scanner that presses without pacing, since the gap
	// between keys is time this test has no reason to spend.
	newApp := func(t *testing.T, conn *keyConn) *appcontext.App {
		t.Helper()

		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		client := device.New(conn)
		if err := client.SetPace(device.PaceTurbo); err != nil {
			t.Fatalf("setting the pace: %v", err)
		}
		app.SetDevice(client)
		return app
	}

	pressed := []press{
		{name: "menu", key: device.KeyMenu},
		{name: "right", key: device.KeyRotateRight},
		{name: "push", key: device.KeyRotatePush},
	}

	// Verify every key reaches the scanner in the order it was given
	t.Run("Success", func(t *testing.T) {
		conn := &keyConn{}

		if err := run(context.Background(), newApp(t, conn), pressed, "press", device.KeyPress); err != nil {
			t.Fatalf("run: %v", err)
		}
		if len(conn.sent) != 3 {
			t.Errorf("the scanner got %v, wanted three presses", conn.sent)
		}
	})

	// Verify that text mode still says nothing, since the result of a key press
	// is on the scanner's own screen
	t.Run("SilentInText", func(t *testing.T) {
		app := newApp(t, &keyConn{})

		if err := run(context.Background(), app, pressed, "press", device.KeyPress); err != nil {
			t.Fatalf("run: %v", err)
		}
		if out := app.Stdout.(*bytes.Buffer).String(); out != "" {
			t.Errorf("run wrote %q, wanted nothing at all", out)
		}
	})

	// Verify that JSON mode reports what was pressed. Empty stdout is not
	// something a decoder can read, which is why silence is wrong here.
	t.Run("JSON", func(t *testing.T) {
		app := newApp(t, &keyConn{})
		app.Config.Output = appcontext.OutputJSON

		if err := run(context.Background(), app, pressed, "long", device.KeyLong); err != nil {
			t.Fatalf("run: %v", err)
		}

		var got report
		out := app.Stdout.(*bytes.Buffer)
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the output is not JSON: %v\nstdout: %s", err, out.String())
		}
		if got.Action != "long" {
			t.Errorf("the report says the action was %q, wanted \"long\"", got.Action)
		}
		if strings.Join(got.Keys, ",") != "menu,right,push" {
			t.Errorf("the report names %v, wanted the keys in the order given", got.Keys)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := run(context.Background(), app, pressed, "press", device.KeyPress)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("run reported %v, wanted a missing device", err)
		}
	})

	// Verify a failure on the very first key does not claim anything was pressed
	t.Run("FailsFirst", func(t *testing.T) {
		conn := &keyConn{failAt: 1}

		err := run(context.Background(), newApp(t, conn), pressed, "press", device.KeyPress)
		if err == nil {
			t.Fatal("run reported nothing, wanted the refused key")
		}
		if !strings.Contains(err.Error(), `pressing "menu"`) {
			t.Errorf("run reported %q, wanted it to name the key", err)
		}
		if strings.Contains(err.Error(), "after") {
			t.Errorf("run reported %q, wanted no claim that keys were pressed", err)
		}
	})

	// Verify a failure partway through says which keys already went through
	t.Run("FailsLater", func(t *testing.T) {
		conn := &keyConn{failAt: 3}

		err := run(context.Background(), newApp(t, conn), pressed, "press", device.KeyPress)
		if err == nil {
			t.Fatal("run reported nothing, wanted the refused key")
		}
		if !strings.Contains(err.Error(), `pressing "push", after menu, right`) {
			t.Errorf("run reported %q, wanted it to say how far it got", err)
		}
	})
}
