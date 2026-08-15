// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package backlight

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

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its three subcommands
//   - Runs: executing the command reports whether the scanner is lit
func TestNew(t *testing.T) {
	// Verify the command carries the name and its three subcommands
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "backlight" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "backlight")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		want := map[string]bool{"on": false, "off": false, "keys": false}
		for _, sub := range cmd.Commands() {
			if _, ok := want[sub.Use]; ok {
				want[sub.Use] = true
			}
		}
		for name, found := range want {
			if !found {
				t.Errorf("the command has no %s subcommand", name)
			}
		}
	})

	// Verify running the command reads the light and writes what it read
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,2", nil
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "backlight: on") {
			t.Errorf("the command wrote %q, wanted it to report the light on", out.String())
		}
	})
}

// Test_newKeys covers the keys subcommand and its own three verbs.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries enable, disable and toggle
//   - Runs: executing it reports the keypad light setting
func Test_newKeys(t *testing.T) {
	// Verify the subcommand carries its own three verbs
	t.Run("Wiring", func(t *testing.T) {
		cmd := newKeys(appcontext.New())

		if cmd.Use != "keys" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Use, "keys")
		}

		want := map[string]bool{"enable": false, "disable": false, "toggle": false}
		for _, sub := range cmd.Commands() {
			if _, ok := want[sub.Use]; ok {
				want[sub.Use] = true
			}
		}
		for name, found := range want {
			if !found {
				t.Errorf("the subcommand has no %s verb", name)
			}
		}
	})

	// Verify running it reports what the keypad light setting is
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
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				}
				return "0,Enable,*", nil
			},
			execXML: leaves,
		}))

		cmd := newKeys(app)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "keypad light: on") {
			t.Errorf("the subcommand wrote %q, wanted the keypad light on", out.String())
		}
	})
}

// Test_newKeysSet covers the enable and disable verbs.
//
// Coverage: 100% (3 test cases covering both spellings and the run)
//
// Test cases:
//   - Enable: the enable verb is named and worded for switching on
//   - Disable: the disable verb is named and worded for switching off
//   - Runs: executing it writes the setting it was built for
func Test_newKeysSet(t *testing.T) {
	// Verify the enable verb is named and worded for switching the light on
	t.Run("Enable", func(t *testing.T) {
		cmd := newKeysSet(appcontext.New(), true)

		if cmd.Use != "enable" {
			t.Errorf("the verb is %q, wanted %q", cmd.Use, "enable")
		}
		if !strings.Contains(cmd.Short, "start") {
			t.Errorf("the summary is %q, wanted it to say the keypad starts lighting", cmd.Short)
		}
		if !strings.HasPrefix(cmd.Long, "Enable") {
			t.Errorf("the help opens with %q, wanted it to open with Enable", cmd.Long)
		}
	})

	// Verify the disable verb is named and worded for switching the light off
	t.Run("Disable", func(t *testing.T) {
		cmd := newKeysSet(appcontext.New(), false)

		if cmd.Use != "disable" {
			t.Errorf("the verb is %q, wanted %q", cmd.Use, "disable")
		}
		if !strings.Contains(cmd.Short, "stop") {
			t.Errorf("the summary is %q, wanted it to say the keypad stops lighting", cmd.Short)
		}
	})

	// Verify running the verb writes the setting it was built for
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
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				}
				return "0,Enable,*", nil
			},
			execXML: leaves,
		}))

		cmd := newKeysSet(app, true)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the verb: %v", err)
		}
		if !strings.Contains(out.String(), "keypad light: on") {
			t.Errorf("the verb wrote %q, wanted the keypad light on", out.String())
		}
	})
}

// Test_newKeysToggle covers the verb that switches the setting the other way.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the verb is named and has help text
//   - Runs: executing it leaves a setting already off alone when it decides off
func Test_newKeysToggle(t *testing.T) {
	// Verify the verb is named and carries its help
	t.Run("Wiring", func(t *testing.T) {
		cmd := newKeysToggle(appcontext.New())

		if cmd.Use != "toggle" {
			t.Errorf("the verb is %q, wanted %q", cmd.Use, "toggle")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the verb has no help text")
		}
	})

	// Verify running the verb reads the setting and writes the other one
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
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				case 4:
					return "0,Enable,*", nil
				case 5:
					return "0,Disable,*", nil
				case 6:
					return "0,Display Options,*", nil
				case 7:
					return "0,Backlight Options,*", nil
				case 8:
					return "0,Set Key Backlight,*", nil
				case 9:
					return "0,Disable,*", nil
				}
				// The light is out, so nothing has to be cycled afterwards.
				return "0,SCAN,*,0", nil
			},
			execXML: leaves,
		}))

		cmd := newKeysToggle(app)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the verb: %v", err)
		}
		if !strings.Contains(out.String(), "keypad light: off") {
			t.Errorf("the verb wrote %q, wanted the keypad light off", out.String())
		}
	})
}

// Test_newOff covers the subcommand that puts the light out.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and has help text
//   - Runs: executing it leaves a dark scanner dark
func Test_newOff(t *testing.T) {
	// Verify the subcommand is named and carries its help
	t.Run("Wiring", func(t *testing.T) {
		cmd := newOff(appcontext.New())

		if cmd.Use != "off" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Use, "off")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify running it against a dark scanner reports it dark
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,0", nil
		}}))

		cmd := newOff(app)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "backlight: off") {
			t.Errorf("the subcommand wrote %q, wanted the light off", out.String())
		}
	})
}

// Test_newOn covers the subcommand that lights the scanner.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its --keys flag
//   - Runs: executing it with --keys=false lights only the screen
func Test_newOn(t *testing.T) {
	// Verify the subcommand is named and offers the keys flag, on by default
	t.Run("Wiring", func(t *testing.T) {
		cmd := newOn(appcontext.New())

		if cmd.Use != "on" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Use, "on")
		}
		flag := cmd.Flags().Lookup("keys")
		if flag == nil {
			t.Fatal("the subcommand has no keys flag")
		}
		if flag.DefValue != "true" {
			t.Errorf("the keys flag defaults to %q, wanted true", flag.DefValue)
		}
	})

	// Verify running it with the keys flag off lights the screen and no menu
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if sts == 1 {
				return "0,SCAN,*,0", nil
			}
			return "0,SCAN,*,3", nil
		}}
		app.SetDevice(device.New(conn))

		cmd := newOn(app)
		if err := cmd.Flags().Set("keys", "false"); err != nil {
			t.Fatalf("setting the flag: %v", err)
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "backlight: on") {
			t.Errorf("the subcommand wrote %q, wanted the light on", out.String())
		}
		for _, command := range conn.sent {
			if strings.HasPrefix(command, "MNU") {
				t.Errorf("a menu was opened for a run that was told to leave the keypad alone")
			}
		}
	})
}

// Test_chooseKeys covers reading the keypad setting, writing it, and confirming.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Unchanged: a setting already at the wanted value is left alone
//   - UnchangedLeaveFails: a scanner stuck in the menus is reported
//   - Changed: the setting is written, read back, and reported as changed
//   - ReadFails: a setting that cannot be read is reported
//   - SelectFails: an entry that cannot be chosen is reported
//   - Mismatch: a setting that did not take is reported rather than claimed
func Test_chooseKeys(t *testing.T) {
	// The three menu entries walked to reach the setting.
	walk := func(step int) (string, bool) {
		switch step {
		case 1:
			return "0,Display Options,*", true
		case 2:
			return "0,Backlight Options,*", true
		case 3:
			return "0,Set Key Backlight,*", true
		}
		return "", false
	}

	// Verify a setting already at the wanted value presses nothing
	t.Run("Unchanged", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				if screen, ok := walk(sts); ok {
					return screen, nil
				}
				return "0,Enable,*", nil
			},
			execXML: leaves,
		}

		changed, want, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err != nil {
			t.Fatalf("chooseKeys: %v", err)
		}
		if changed || !want {
			t.Errorf("chooseKeys reported changed=%v want=%v, wanted false and true", changed, want)
		}
		for _, command := range conn.sent {
			if command == "KEY,E,P" && sts > 4 {
				t.Error("an entry was chosen for a setting that was already right")
			}
		}
	})

	// Verify a scanner that will not come out of the menus is reported
	t.Run("UnchangedLeaveFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				if screen, ok := walk(sts); ok {
					return screen, nil
				}
				return "0,Enable,*", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Backlight Options" MenuType="TypeSelect"/>`, nil
			},
		}

		_, _, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("chooseKeys reported %v, wanted the scanner stuck in the menus", err)
		}
	})

	// Verify a setting that has to change is written and then read back
	t.Run("Changed", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				case 5:
					return "0,Enable,*", nil
				case 6, 7, 8:
					screen, _ := walk(sts - 5)
					return screen, nil
				}
				return "0,Enable,*", nil
			},
			execXML: leaves,
		}

		changed, want, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err != nil {
			t.Fatalf("chooseKeys: %v", err)
		}
		if !changed || !want {
			t.Errorf("chooseKeys reported changed=%v want=%v, wanted both true", changed, want)
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

		_, _, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), `looking for "Display Options"`) {
			t.Errorf("chooseKeys reported %v, wanted the failed walk", err)
		}
	})

	// Verify an entry the scanner will not take is reported by name
	t.Run("SelectFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				}
				return "", errors.New("the port is gone")
			},
			execXML: leaves,
		}

		_, _, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), `choosing "Enable" for the keypad light`) {
			t.Errorf("chooseKeys reported %v, wanted the refused entry", err)
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
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				}
				return "0,Enable,*", nil
			},
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Backlight Options" MenuType="TypeSelect"/>`, nil
			},
		}

		_, _, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("chooseKeys reported %v, wanted the scanner stuck in the menus", err)
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
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				case 5:
					return "0,Enable,*", nil
				case 6, 7, 8:
					screen, _ := walk(sts - 5)
					return screen, nil
				}
				return "0,Enable,*", nil
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
				return `<MenuInfo Name="Backlight Options" MenuType="TypeSelect"/>`, nil
			},
		}

		_, _, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("chooseKeys reported %v, wanted the scanner stuck in the menus", err)
		}
	})

	// Verify a setting that cannot be read back after the write is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				case 5:
					return "0,Enable,*", nil
				}
				return "", errors.New("the port is gone")
			},
			execXML: leaves,
		}

		_, _, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), `looking for "Display Options"`) {
			t.Errorf("chooseKeys reported %v, wanted the failed read back", err)
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
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				case 5:
					return "0,Enable,*", nil
				case 6, 7, 8:
					screen, _ := walk(sts - 5)
					return screen, nil
				}
				return "0,Disable,*", nil
			},
			execXML: leaves,
		}

		_, _, err := chooseKeys(context.Background(), device.New(conn),
			func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), "the keypad light is still off") {
			t.Errorf("chooseKeys reported %v, wanted the setting that did not take", err)
		}
	})
}

// Test_enableKeys covers switching the keypad light on.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Already: a keypad light already on is reported as unchanged
func Test_enableKeys(t *testing.T) {
	// Verify a setting already on is left alone and reported as unchanged
	t.Run("Already", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				}
				return "0,Enable,*", nil
			},
			execXML: leaves,
		}

		changed, err := enableKeys(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("enableKeys: %v", err)
		}
		if changed {
			t.Error("enableKeys reported a change for a setting that was already on")
		}
	})
}

// Test_encode covers writing a value as JSON.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Writes: the value is written as indented JSON
func Test_encode(t *testing.T) {
	// Verify the value is written as JSON the reader can parse back
	t.Run("Writes", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := encode(app, keysReport{Enabled: true}); err != nil {
			t.Fatalf("encode: %v", err)
		}

		var got keysReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if !got.Enabled {
			t.Errorf("the report is %+v, wanted the keypad light enabled", got)
		}
	})
}

// Test_flip covers pressing the light key and waiting for the scanner to agree.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Success: the light goes the way it was asked and is reported
//   - PressFails: a scanner that refuses the key is reported
//   - ReadFails: a scanner that will not report the light is reported
//   - Cancelled: a cancelled context stops the waiting
//   - GivesUp: a light that never changes is reported after the last look
func Test_flip(t *testing.T) {
	// Verify a light that comes on is confirmed by reading the scanner back
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "0,SCAN,*,2", nil
			}
			return "", nil
		}}

		got, err := flip(context.Background(), device.New(conn), true)
		if err != nil {
			t.Fatalf("flip: %v", err)
		}
		if !got.On || got.Level != 2 {
			t.Errorf("flip reported %+v, wanted the light on at level 2", got)
		}

		pressed := false
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				pressed = true
			}
		}
		if !pressed {
			t.Errorf("the scanner was sent %v, wanted the light key", conn.sent)
		}
	})

	// Verify a scanner that refuses the light key is reported
	t.Run("PressFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "KEY,V,P" {
				return "", errors.New("the scanner refused the key")
			}
			return "0,SCAN,*,2", nil
		}}

		_, err := flip(context.Background(), device.New(conn), true)
		if err == nil || !strings.Contains(err.Error(), "pressing the light key") {
			t.Errorf("flip reported %v, wanted the refused key", err)
		}
	})

	// Verify a scanner that will not report the light is reported
	t.Run("ReadFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}

		_, err := flip(context.Background(), device.New(conn), true)
		if err == nil {
			t.Fatal("flip reported nothing, wanted the failed read")
		}
	})

	// Verify a cancelled context stops the waiting rather than looking on
	t.Run("Cancelled", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "0,SCAN,*,0", nil
			}
			return "", nil
		}}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := flip(ctx, device.New(conn), true)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("flip reported %v, wanted the cancelled context", err)
		}
	})

	// Verify a light that never changes is reported after the last look
	t.Run("GivesUp", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "0,SCAN,*,0", nil
			}
			return "", nil
		}}

		_, err := flip(context.Background(), device.New(conn), true)
		if err == nil || !strings.Contains(err.Error(), "the light did not come on") {
			t.Errorf("flip reported %v, wanted the light that never came on", err)
		}
	})
}

// Test_onOff covers wording a state.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Words: true reads as on and false as off
func Test_onOff(t *testing.T) {
	// Verify both states are worded the way the output spells them
	t.Run("Words", func(t *testing.T) {
		if got := onOff(true); got != "on" {
			t.Errorf("onOff(true) is %q, wanted %q", got, "on")
		}
		if got := onOff(false); got != "off" {
			t.Errorf("onOff(false) is %q, wanted %q", got, "off")
		}
	})
}

// Test_read covers reading whether the scanner is lit.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Success: the light and its brightness are read
//   - Fails: a scanner that will not answer is reported
func Test_read(t *testing.T) {
	// Verify the light's state and brightness come off the scanner
	t.Run("Success", func(t *testing.T) {
		client := device.New(&stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,3", nil
		}})

		got, err := read(context.Background(), client)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !got.On || got.Level != 3 {
			t.Errorf("read reported %+v, wanted the light on at level 3", got)
		}
	})

	// Verify a scanner that will not answer is reported
	t.Run("Fails", func(t *testing.T) {
		client := device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := read(context.Background(), client); err == nil {
			t.Fatal("read reported nothing, wanted the failed read")
		}
	})
}

// Test_readKeys covers reading the keypad light setting off its menu.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Enabled: a menu highlighting Enable reads as on
//   - Disabled: a menu highlighting Disable reads as off
//   - OpenFails: a scanner that refuses the top menu is reported
//   - WalkFails: an entry on the way that cannot be found is reported
//   - Unknown: a highlighted row that is neither entry is reported
func Test_readKeys(t *testing.T) {
	// The three menu entries walked to reach the setting.
	walk := func(step int) (string, bool) {
		switch step {
		case 1:
			return "0,Display Options,*", true
		case 2:
			return "0,Backlight Options,*", true
		case 3:
			return "0,Set Key Backlight,*", true
		}
		return "", false
	}

	// Verify a menu highlighting Enable reads as the keypad lighting up
	t.Run("Enabled", func(t *testing.T) {
		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if screen, ok := walk(sts); ok {
				return screen, nil
			}
			return "0,Enable,*", nil
		}}

		got, err := readKeys(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("readKeys: %v", err)
		}
		if !got {
			t.Error("readKeys reported the keypad light off, wanted on")
		}
	})

	// Verify a menu highlighting Disable reads as the keypad staying dark
	t.Run("Disabled", func(t *testing.T) {
		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if screen, ok := walk(sts); ok {
				return screen, nil
			}
			return "0,Disable,*", nil
		}}

		got, err := readKeys(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("readKeys: %v", err)
		}
		if got {
			t.Error("readKeys reported the keypad light on, wanted off")
		}
	})

	// Verify a scanner that refuses the top menu is reported
	t.Run("OpenFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "MNU,TOP," {
				return "", errors.New("the scanner refused the menu")
			}
			return "", nil
		}}

		_, err := readKeys(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "opening the top menu") {
			t.Errorf("readKeys reported %v, wanted the refused menu", err)
		}
	})

	// Verify an entry on the way that cannot be found is named in the refusal
	t.Run("WalkFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}

		_, err := readKeys(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), `looking for "Display Options"`) {
			t.Errorf("readKeys reported %v, wanted the entry it could not find", err)
		}
	})

	// Verify a scanner that stops answering at the setting itself is reported
	t.Run("HighlightFails", func(t *testing.T) {
		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if screen, ok := walk(sts); ok {
				return screen, nil
			}
			return "", errors.New("the port is gone")
		}}

		_, err := readKeys(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "reading the keypad light setting") {
			t.Errorf("readKeys reported %v, wanted the failed read", err)
		}
	})

	// Verify a row that is neither Enable nor Disable is reported, not guessed
	t.Run("Unknown", func(t *testing.T) {
		sts := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			if screen, ok := walk(sts); ok {
				return screen, nil
			}
			return "0,Perhaps,*", nil
		}}

		_, err := readKeys(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "which is neither") {
			t.Errorf("readKeys reported %v, wanted the unrecognised row", err)
		}
	})
}

// Test_renderReport covers both ways the light's state is written.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - TextOn: a lit scanner is written with its brightness
//   - TextOff: a dark scanner is written without one
//   - JSON: the same reading is written as JSON
func Test_renderReport(t *testing.T) {
	// Verify a lit scanner is written with the brightness underneath
	t.Run("TextOn", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderReport(app, report{On: true, Level: 2}); err != nil {
			t.Fatalf("renderReport: %v", err)
		}
		want := "backlight: on\nlevel:     2\n"
		if got := out.String(); got != want {
			t.Errorf("renderReport wrote %q, wanted %q", got, want)
		}
	})

	// Verify a dark scanner is written without a brightness that means nothing
	t.Run("TextOff", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderReport(app, report{}); err != nil {
			t.Fatalf("renderReport: %v", err)
		}
		want := "backlight: off\n"
		if got := out.String(); got != want {
			t.Errorf("renderReport wrote %q, wanted %q", got, want)
		}
	})

	// Verify JSON output carries both the state and the brightness
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := renderReport(app, report{On: true, Level: 1}); err != nil {
			t.Fatalf("renderReport: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if !got.On || got.Level != 1 {
			t.Errorf("the report is %+v, wanted the light on at level 1", got)
		}
	})
}

// Test_runKeysChange covers writing the keypad setting and cycling the light.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Unchanged: a setting already right is reported without cycling the light
//   - JSON: the same reading is written as JSON
//   - ChangedAndLit: a changed setting on a lit scanner cycles the light
//   - ChangedAndDark: a changed setting on a dark scanner cycles nothing
//   - CycleOffFails: a light that will not go out is reported
//   - CycleOnFails: a light that will not come back is reported
//   - NoDevice: a run with no scanner named is refused
func Test_runKeysChange(t *testing.T) {
	// The three menu entries walked to reach the setting.
	walk := func(step int) (string, bool) {
		switch step {
		case 1:
			return "0,Display Options,*", true
		case 2:
			return "0,Backlight Options,*", true
		case 3:
			return "0,Set Key Backlight,*", true
		}
		return "", false
	}

	// The scanner's answers for a setting that has to be changed to Enable,
	// followed by whatever the light is doing afterwards.
	changing := func(light string) func(string) (string, error) {
		sts := 0
		return func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			switch sts {
			case 1, 2, 3:
				screen, _ := walk(sts)
				return screen, nil
			case 4:
				return "0,Disable,*", nil
			case 5:
				return "0,Enable,*", nil
			case 6, 7, 8:
				screen, _ := walk(sts - 5)
				return screen, nil
			case 9:
				return "0,Enable,*", nil
			}
			return light, nil
		}
	}

	// Verify a setting already right is reported and nothing is cycled
	t.Run("Unchanged", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				if screen, ok := walk(sts); ok {
					return screen, nil
				}
				return "0,Enable,*", nil
			},
			execXML: leaves,
		}
		app.SetDevice(device.New(conn))

		if err := runKeysChange(context.Background(), app, func(bool) bool { return true }); err != nil {
			t.Fatalf("runKeysChange: %v", err)
		}
		if got := out.String(); got != "keypad light: on\n" {
			t.Errorf("runKeysChange wrote %q, wanted the keypad light on", got)
		}
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				t.Error("the light was cycled for a setting that did not change")
			}
		}
	})

	// Verify JSON output carries the setting the scanner was left on
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		sts := 0
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				if screen, ok := walk(sts); ok {
					return screen, nil
				}
				return "0,Enable,*", nil
			},
			execXML: leaves,
		}))

		if err := runKeysChange(context.Background(), app, func(bool) bool { return true }); err != nil {
			t.Fatalf("runKeysChange: %v", err)
		}

		var got keysReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if !got.Enabled {
			t.Errorf("the report is %+v, wanted the keypad light enabled", got)
		}
	})

	// Verify a changed setting on a lit scanner puts the light out and back
	t.Run("ChangedAndLit", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		lit := true
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command == "KEY,V,P" {
					lit = !lit
					return "", nil
				}
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				case 5:
					return "0,Enable,*", nil
				case 6, 7, 8:
					screen, _ := walk(sts - 5)
					return screen, nil
				case 9:
					return "0,Enable,*", nil
				}
				if lit {
					return "0,SCAN,*,2", nil
				}
				return "0,SCAN,*,0", nil
			},
			execXML: leaves,
		}
		app.SetDevice(device.New(conn))

		if err := runKeysChange(context.Background(), app, func(bool) bool { return true }); err != nil {
			t.Fatalf("runKeysChange: %v", err)
		}

		presses := 0
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				presses++
			}
		}
		if presses != 2 {
			t.Errorf("the light key was pressed %d times, wanted 2", presses)
		}
	})

	// Verify a changed setting on a dark scanner cycles nothing
	t.Run("ChangedAndDark", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{exec: changing("0,SCAN,*,0"), execXML: leaves}
		app.SetDevice(device.New(conn))

		if err := runKeysChange(context.Background(), app, func(bool) bool { return true }); err != nil {
			t.Fatalf("runKeysChange: %v", err)
		}
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				t.Error("the light was cycled on a scanner that was already dark")
			}
		}
	})

	// Verify a light that will not go out is reported
	t.Run("CycleOffFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		answer := changing("0,SCAN,*,2")
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command == "KEY,V,P" {
					return "", errors.New("the scanner refused the key")
				}
				return answer(command)
			},
			execXML: leaves,
		}))

		err := runKeysChange(context.Background(), app, func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), "pressing the light key") {
			t.Errorf("runKeysChange reported %v, wanted the refused key", err)
		}
	})

	// Verify a light that goes out but will not come back is reported
	t.Run("CycleOnFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		answer := changing("0,SCAN,*,2")
		presses := 0
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command == "KEY,V,P" {
					presses++
					if presses == 2 {
						return "", errors.New("the scanner refused the key")
					}
					return "", nil
				}
				if command == "STS" && presses == 1 {
					return "0,SCAN,*,0", nil
				}
				return answer(command)
			},
			execXML: leaves,
		}))

		err := runKeysChange(context.Background(), app, func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), "pressing the light key") {
			t.Errorf("runKeysChange reported %v, wanted the refused key", err)
		}
	})

	// Verify a setting that cannot be reached at all is reported
	t.Run("ChooseFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		err := runKeysChange(context.Background(), app, func(bool) bool { return true })
		if err == nil || !strings.Contains(err.Error(), `looking for "Display Options"`) {
			t.Errorf("runKeysChange reported %v, wanted the failed walk", err)
		}
	})

	// Verify a light that cannot be read after the change is left alone
	t.Run("ChangedButLightUnreadable", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1, 2, 3:
					screen, _ := walk(sts)
					return screen, nil
				case 4:
					return "0,Disable,*", nil
				case 5:
					return "0,Enable,*", nil
				case 6, 7, 8:
					screen, _ := walk(sts - 5)
					return screen, nil
				case 9:
					return "0,Enable,*", nil
				}
				return "", errors.New("the port is gone")
			},
			execXML: leaves,
		}
		app.SetDevice(device.New(conn))

		if err := runKeysChange(context.Background(), app, func(bool) bool { return true }); err != nil {
			t.Fatalf("runKeysChange: %v", err)
		}
		if got := out.String(); got != "keypad light: on\n" {
			t.Errorf("runKeysChange wrote %q, wanted the keypad light on", got)
		}
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				t.Error("the light was cycled although it could not be read")
			}
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runKeysChange(context.Background(), app, func(bool) bool { return true })
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runKeysChange reported %v, wanted a missing device", err)
		}
	})
}

// Test_runKeysReport covers reporting the keypad light setting.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Text: the setting is read and written
//   - JSON: the same reading is written as JSON
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a setting that cannot be read is reported
//   - LeaveFails: a scanner that will not come out of the menus is reported
func Test_runKeysReport(t *testing.T) {
	// The scanner's answers for a walk that ends on Enable.
	enabled := func() func(string) (string, error) {
		sts := 0
		return func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			switch sts {
			case 1:
				return "0,Display Options,*", nil
			case 2:
				return "0,Backlight Options,*", nil
			case 3:
				return "0,Set Key Backlight,*", nil
			}
			return "0,Enable,*", nil
		}
	}

	// Verify the setting is read off the menu and written out
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: enabled(), execXML: leaves}))

		if err := runKeysReport(context.Background(), app); err != nil {
			t.Fatalf("runKeysReport: %v", err)
		}
		if got := out.String(); got != "keypad light: on\n" {
			t.Errorf("runKeysReport wrote %q, wanted the keypad light on", got)
		}
	})

	// Verify JSON output carries the setting
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON
		app.SetDevice(device.New(&stubConn{exec: enabled(), execXML: leaves}))

		if err := runKeysReport(context.Background(), app); err != nil {
			t.Fatalf("runKeysReport: %v", err)
		}

		var got keysReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if !got.Enabled {
			t.Errorf("the report is %+v, wanted the keypad light enabled", got)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := runKeysReport(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runKeysReport reported %v, wanted a missing device", err)
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

		err := runKeysReport(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), `looking for "Display Options"`) {
			t.Errorf("runKeysReport reported %v, wanted the failed walk", err)
		}
	})

	// Verify a scanner that will not come out of the menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{
			exec: enabled(),
			execXML: func(string) (string, error) {
				return `<MenuInfo Name="Backlight Options" MenuType="TypeSelect"/>`, nil
			},
		}))

		err := runKeysReport(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("runKeysReport reported %v, wanted the scanner stuck in the menus", err)
		}
	})
}

// Test_runKeysSet covers the verb that writes one setting.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Writes: the setting asked for is what the scanner is left on
func Test_runKeysSet(t *testing.T) {
	// Verify the setting asked for is written and reported
	t.Run("Writes", func(t *testing.T) {
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
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				}
				return "0,Disable,*", nil
			},
			execXML: leaves,
		}))

		if err := runKeysSet(context.Background(), app, false); err != nil {
			t.Fatalf("runKeysSet: %v", err)
		}
		if got := out.String(); got != "keypad light: off\n" {
			t.Errorf("runKeysSet wrote %q, wanted the keypad light off", got)
		}
	})
}

// Test_runKeysToggle covers the verb that writes whichever setting it is not.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Flips: a setting found on is written off
func Test_runKeysToggle(t *testing.T) {
	// Verify a setting found on is turned around
	t.Run("Flips", func(t *testing.T) {
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
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				case 4:
					return "0,Enable,*", nil
				case 5:
					return "0,Disable,*", nil
				case 6:
					return "0,Display Options,*", nil
				case 7:
					return "0,Backlight Options,*", nil
				case 8:
					return "0,Set Key Backlight,*", nil
				case 9:
					return "0,Disable,*", nil
				}
				return "0,SCAN,*,0", nil
			},
			execXML: leaves,
		}))

		if err := runKeysToggle(context.Background(), app); err != nil {
			t.Fatalf("runKeysToggle: %v", err)
		}
		if got := out.String(); got != "keypad light: off\n" {
			t.Errorf("runKeysToggle wrote %q, wanted the keypad light off", got)
		}
	})
}

// Test_runOff covers putting the light out.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Lit: a lit scanner is put out and reported
//   - AlreadyDark: a dark scanner is left alone
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not report the light is reported
func Test_runOff(t *testing.T) {
	// Verify a lit scanner has its light put out and reported
	t.Run("Lit", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		lit := true
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "KEY,V,P" {
				lit = false
				return "", nil
			}
			if lit {
				return "0,SCAN,*,2", nil
			}
			return "0,SCAN,*,0", nil
		}}
		app.SetDevice(device.New(conn))

		if err := runOff(context.Background(), app); err != nil {
			t.Fatalf("runOff: %v", err)
		}
		if !strings.Contains(out.String(), "backlight: off") {
			t.Errorf("runOff wrote %q, wanted the light off", out.String())
		}
	})

	// Verify a scanner that is already dark is left alone
	t.Run("AlreadyDark", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,0", nil
		}}
		app.SetDevice(device.New(conn))

		if err := runOff(context.Background(), app); err != nil {
			t.Fatalf("runOff: %v", err)
		}
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				t.Error("the light key was pressed on a scanner that was already dark")
			}
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := runOff(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runOff reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not report the light is reported
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		if err := runOff(context.Background(), app); err == nil {
			t.Fatal("runOff reported nothing, wanted the failed read")
		}
	})

	// Verify a light that will not go out is reported
	t.Run("FlipFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "KEY,V,P" {
				return "", errors.New("the scanner refused the key")
			}
			return "0,SCAN,*,2", nil
		}}))

		err := runOff(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "pressing the light key") {
			t.Errorf("runOff reported %v, wanted the refused key", err)
		}
	})
}

// Test_runOn covers lighting the scanner, with and without the keypad.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Dark: a dark scanner is lit and reported
//   - AlreadyLit: a lit scanner whose keypad setting did not change is left alone
//   - LitAndChanged: a lit scanner whose keypad setting changed has its light cycled
//   - KeysFail: a keypad setting that cannot be written is reported
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not report the light is reported
func Test_runOn(t *testing.T) {
	// The scanner's answers for a keypad setting already on.
	keysAlreadyOn := func(light func() string) func(string) (string, error) {
		sts := 0
		return func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			switch sts {
			case 1:
				return "0,Display Options,*", nil
			case 2:
				return "0,Backlight Options,*", nil
			case 3:
				return "0,Set Key Backlight,*", nil
			case 4:
				return "0,Enable,*", nil
			}
			return light(), nil
		}
	}

	// The scanner's answers for a keypad setting that has to be switched on,
	// followed by whatever the light is doing afterwards.
	keysChangeThenLight := func(light string) func(string) (string, error) {
		sts := 0
		return func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			switch sts {
			case 1, 6:
				return "0,Display Options,*", nil
			case 2, 7:
				return "0,Backlight Options,*", nil
			case 3, 8:
				return "0,Set Key Backlight,*", nil
			case 4:
				return "0,Disable,*", nil
			case 5, 9:
				return "0,Enable,*", nil
			}
			return light, nil
		}
	}

	// Verify a dark scanner is lit and reported
	t.Run("Dark", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		lit := false
		answer := keysAlreadyOn(func() string {
			if lit {
				return "0,SCAN,*,3"
			}
			return "0,SCAN,*,0"
		})
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command == "KEY,V,P" {
					lit = true
					return "", nil
				}
				return answer(command)
			},
			execXML: leaves,
		}))

		if err := runOn(context.Background(), app, true); err != nil {
			t.Fatalf("runOn: %v", err)
		}
		if !strings.Contains(out.String(), "backlight: on") {
			t.Errorf("runOn wrote %q, wanted the light on", out.String())
		}
	})

	// Verify a dark scanner whose light will not come on is reported
	t.Run("DarkFlipFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "KEY,V,P" {
				return "", errors.New("the scanner refused the key")
			}
			return "0,SCAN,*,0", nil
		}}))

		err := runOn(context.Background(), app, false)
		if err == nil || !strings.Contains(err.Error(), "pressing the light key") {
			t.Errorf("runOn reported %v, wanted the refused key", err)
		}
	})

	// Verify a light that will not go out during the cycle is reported
	t.Run("CycleOffFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		answer := keysChangeThenLight("0,SCAN,*,2")
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command == "KEY,V,P" {
					return "", errors.New("the scanner refused the key")
				}
				return answer(command)
			},
			execXML: leaves,
		}))

		err := runOn(context.Background(), app, true)
		if err == nil || !strings.Contains(err.Error(), "pressing the light key") {
			t.Errorf("runOn reported %v, wanted the refused key", err)
		}
	})

	// Verify a light that goes out but will not come back is reported
	t.Run("CycleOnFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		answer := keysChangeThenLight("0,SCAN,*,2")
		presses := 0
		app.SetDevice(device.New(&stubConn{
			exec: func(command string) (string, error) {
				if command == "KEY,V,P" {
					presses++
					if presses == 2 {
						return "", errors.New("the scanner refused the key")
					}
					return "", nil
				}
				if command == "STS" && presses == 1 {
					return "0,SCAN,*,0", nil
				}
				return answer(command)
			},
			execXML: leaves,
		}))

		err := runOn(context.Background(), app, true)
		if err == nil || !strings.Contains(err.Error(), "pressing the light key") {
			t.Errorf("runOn reported %v, wanted the refused key", err)
		}
	})

	// Verify a lit scanner whose keypad setting was already right is left alone
	t.Run("AlreadyLit", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		conn := &stubConn{
			exec:    keysAlreadyOn(func() string { return "0,SCAN,*,2" }),
			execXML: leaves,
		}
		app.SetDevice(device.New(conn))

		if err := runOn(context.Background(), app, true); err != nil {
			t.Fatalf("runOn: %v", err)
		}
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				t.Error("the light key was pressed on a scanner that was already lit")
			}
		}
	})

	// Verify a lit scanner whose keypad setting changed has its light cycled
	t.Run("LitAndChanged", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		lit := true
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command == "KEY,V,P" {
					lit = !lit
					return "", nil
				}
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				case 4:
					return "0,Disable,*", nil
				case 5:
					return "0,Enable,*", nil
				case 6:
					return "0,Display Options,*", nil
				case 7:
					return "0,Backlight Options,*", nil
				case 8:
					return "0,Set Key Backlight,*", nil
				case 9:
					return "0,Enable,*", nil
				}
				if lit {
					return "0,SCAN,*,2", nil
				}
				return "0,SCAN,*,0", nil
			},
			execXML: leaves,
		}
		app.SetDevice(device.New(conn))

		if err := runOn(context.Background(), app, true); err != nil {
			t.Fatalf("runOn: %v", err)
		}

		presses := 0
		for _, command := range conn.sent {
			if command == "KEY,V,P" {
				presses++
			}
		}
		if presses != 2 {
			t.Errorf("the light key was pressed %d times, wanted 2", presses)
		}
	})

	// Verify a keypad setting that cannot be written is reported
	t.Run("KeysFail", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		err := runOn(context.Background(), app, true)
		if err == nil || !strings.Contains(err.Error(), `looking for "Display Options"`) {
			t.Errorf("runOn reported %v, wanted the failed walk", err)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := runOn(context.Background(), app, false); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runOn reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not report the light is reported
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		if err := runOn(context.Background(), app, false); err == nil {
			t.Fatal("runOn reported nothing, wanted the failed read")
		}
	})
}

// Test_runReport covers reporting whether the scanner is lit.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the light is read and written
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer is reported
func Test_runReport(t *testing.T) {
	// Verify the light is read from the scanner and written out
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(string) (string, error) {
			return "0,SCAN,*,1", nil
		}}))

		if err := runReport(context.Background(), app); err != nil {
			t.Fatalf("runReport: %v", err)
		}
		if !strings.Contains(out.String(), "level:     1") {
			t.Errorf("runReport wrote %q, wanted the brightness", out.String())
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

		if err := runReport(context.Background(), app); err == nil {
			t.Fatal("runReport reported nothing, wanted the failed read")
		}
	})
}

// Test_setKeys covers writing one keypad setting.
//
// Coverage: 100% (1 test case covering the only path)
//
// Test cases:
//   - Already: a setting already at the wanted value is reported as unchanged
func Test_setKeys(t *testing.T) {
	// Verify a setting already right is left alone and reported as unchanged
	t.Run("Already", func(t *testing.T) {
		sts := 0
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command != "STS" {
					return "", nil
				}
				sts++
				switch sts {
				case 1:
					return "0,Display Options,*", nil
				case 2:
					return "0,Backlight Options,*", nil
				case 3:
					return "0,Set Key Backlight,*", nil
				}
				return "0,Disable,*", nil
			},
			execXML: leaves,
		}

		changed, err := setKeys(context.Background(), device.New(conn), false)
		if err != nil {
			t.Fatalf("setKeys: %v", err)
		}
		if changed {
			t.Error("setKeys reported a change for a setting that was already off")
		}
	})
}

// Test_title covers capitalising a subcommand's name for a sentence.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Words: a word is capitalised and an empty string is left alone
func Test_title(t *testing.T) {
	// Verify a word gains a capital and an empty string is returned unchanged
	t.Run("Words", func(t *testing.T) {
		if got := title("enable"); got != "Enable" {
			t.Errorf("title(\"enable\") is %q, wanted %q", got, "Enable")
		}
		if got := title(""); got != "" {
			t.Errorf("title(\"\") is %q, wanted an empty string", got)
		}
	})
}
