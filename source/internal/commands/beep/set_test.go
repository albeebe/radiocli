// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package beep

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// Test_newSet covers the set subcommand and the value it checks before running.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Wiring: the subcommand is named and has help text
//   - RejectsUnknown: a value that is not a setting is refused before the port
//   - Runs: a value this command knows is written to the scanner
func Test_newSet(t *testing.T) {
	// Verify the subcommand is named and carries its help
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSet(appcontext.New())

		if !strings.HasPrefix(cmd.Use, "set") {
			t.Errorf("the subcommand is %q, wanted it to start with set", cmd.Use)
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify a value that is not a setting is refused with the list that works
	t.Run("RejectsUnknown", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newSet(app)
		err := cmd.RunE(cmd, []string{"loud"})
		if err == nil || !strings.Contains(err.Error(), "is not a key beep setting") {
			t.Fatalf("the subcommand reported %v, wanted a refusal naming the values", err)
		}
		if !strings.Contains(err.Error(), "1 to 15") {
			t.Errorf("the refusal is %q, wanted it to list the values that work", err)
		}
	})

	// Verify a value this command knows is written to the scanner
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
				return "0,Auto,*", nil
			},
			execXML: leaves,
		}))

		cmd := newSet(app)
		if err := cmd.RunE(cmd, []string{"auto"}); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if got, want := out.String(), "key beep: auto\n"; got != want {
			t.Errorf("the subcommand wrote %q, wanted %q", got, want)
		}
	})
}

// Test_runSet covers writing one setting and reporting where it ended up.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the setting is written and the result reported
//   - NoDevice: a run with no scanner named is refused
//   - ChooseFails: a setting that cannot be reached is reported
func Test_runSet(t *testing.T) {
	// Verify the setting asked for is what the scanner is left on
	t.Run("Success", func(t *testing.T) {
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
		}))

		silent, _ := lookup(off)
		if err := runSet(context.Background(), app, silent); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		if got, want := out.String(), "key beep: off\n"; got != want {
			t.Errorf("runSet wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		nine, _ := lookup("9")
		if err := runSet(context.Background(), app, nine); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runSet reported %v, wanted a missing device", err)
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

		nine, _ := lookup("9")
		err := runSet(context.Background(), app, nine)
		if err == nil || !strings.Contains(err.Error(), `looking for "Adjust Key Beep"`) {
			t.Errorf("runSet reported %v, wanted the failed walk", err)
		}
	})
}
