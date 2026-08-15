// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package beep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// Test_newToggle covers the toggle subcommand.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and has help text
//   - Runs: executing it silences a keypad that was making a sound
func Test_newToggle(t *testing.T) {
	// Verify the subcommand is named and carries its help
	t.Run("Wiring", func(t *testing.T) {
		cmd := newToggle(appcontext.New())

		if cmd.Use != "toggle" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Use, "toggle")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify running it switches a sounding keypad off
	t.Run("Runs", func(t *testing.T) {
		isolate(t)

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

		cmd := newToggle(app)
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "written down to go back to") {
			t.Errorf("the subcommand wrote %q, wanted it to say what was written down", out.String())
		}
	})
}

// Test_renderToggle covers everything the toggle can say about what it did.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Remembered: a run that stored a setting says what it stored
//   - Restored: a run that put one back says so
//   - NothingToRestore: a run left silent with nothing stored says that
//   - Plain: a run that changed nothing writes the setting alone
//   - JSON: the same result is written as JSON
func Test_renderToggle(t *testing.T) {
	// Verify a run that wrote a setting down says which one
	t.Run("Remembered", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderToggle(app, report{Level: off, Remembered: "9"}); err != nil {
			t.Fatalf("renderToggle: %v", err)
		}
		want := "key beep: off, and level 9 was written down to go back to\n"
		if got := out.String(); got != want {
			t.Errorf("renderToggle wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run that put a setting back says so
	t.Run("Restored", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderToggle(app, report{Level: "9", On: true, Restored: true}); err != nil {
			t.Fatalf("renderToggle: %v", err)
		}
		want := "key beep: level 9, put back\n"
		if got := out.String(); got != want {
			t.Errorf("renderToggle wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run left silent with nothing stored says that, not nothing
	t.Run("NothingToRestore", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderToggle(app, report{Level: off}); err != nil {
			t.Fatalf("renderToggle: %v", err)
		}
		want := "key beep: off, with nothing written down to go back to\n"
		if got := out.String(); got != want {
			t.Errorf("renderToggle wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run that changed nothing writes the setting on its own
	t.Run("Plain", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderToggle(app, report{Level: "auto", On: true}); err != nil {
			t.Fatalf("renderToggle: %v", err)
		}
		want := "key beep: auto\n"
		if got := out.String(); got != want {
			t.Errorf("renderToggle wrote %q, wanted %q", got, want)
		}
	})

	// Verify JSON output carries what the toggle did
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := renderToggle(app, report{Level: off, Remembered: "9"}); err != nil {
			t.Fatalf("renderToggle: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if got.Level != off || got.Remembered != "9" {
			t.Errorf("the report is %+v, wanted off with level 9 remembered", got)
		}
	})
}

// Test_runToggle covers switching the keypad off and putting it back.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - SilencesAndRemembers: a sounding keypad is silenced and written down
//   - RestoresWhatWasStored: a silent keypad is put back to what was stored
//   - StaysSilentWithNothingStored: a silent keypad with nothing stored is left
//   - NoDevice: a run with no scanner named is refused
//   - ChooseFails: a setting that cannot be reached is reported
func Test_runToggle(t *testing.T) {
	// The scanner's answers for a walk that finds one value and sets another.
	changing := func(from, to string) func(string) (string, error) {
		sts := 0
		return func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			sts++
			switch sts {
			case 1:
				return "0,Adjust Key Beep,*", nil
			case 2:
				return "0," + from + ",*", nil
			case 3:
				return "0," + to + ",*", nil
			case 4:
				return "0,Adjust Key Beep,*", nil
			}
			return "0," + to + ",*", nil
		}
	}

	// Verify a keypad making a sound is silenced and what it was is stored
	t.Run("SilencesAndRemembers", func(t *testing.T) {
		isolate(t)

		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{
			exec:    changing("Level 9", "Off"),
			execXML: leaves,
		}))

		if err := runToggle(context.Background(), app); err != nil {
			t.Fatalf("runToggle: %v", err)
		}
		if !strings.Contains(out.String(), "level 9 was written down") {
			t.Errorf("runToggle wrote %q, wanted it to name what was stored", out.String())
		}

		// What it stored has to be what a later toggle finds.
		got, ok := recall(app, device.Info{})
		if !ok || got.value != "9" {
			t.Errorf("the remembered setting is %q, %v, wanted 9", got.value, ok)
		}
	})

	// Verify a silent keypad is put back to the setting written down
	t.Run("RestoresWhatWasStored", func(t *testing.T) {
		isolate(t)

		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		nine, _ := lookup("9")
		remember(app, device.Info{}, nine, time.Now())

		app.SetDevice(device.New(&stubConn{
			exec:    changing("Off", "Level 9"),
			execXML: leaves,
		}))

		if err := runToggle(context.Background(), app); err != nil {
			t.Fatalf("runToggle: %v", err)
		}
		if !strings.Contains(out.String(), "put back") {
			t.Errorf("runToggle wrote %q, wanted it to say the setting was put back", out.String())
		}
	})

	// Verify a silent keypad with nothing stored is left silent, and says so
	t.Run("StaysSilentWithNothingStored", func(t *testing.T) {
		isolate(t)

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
				return "0,Off,*", nil
			},
			execXML: leaves,
		}))

		if err := runToggle(context.Background(), app); err != nil {
			t.Fatalf("runToggle: %v", err)
		}
		if !strings.Contains(out.String(), "with nothing written down") {
			t.Errorf("runToggle wrote %q, wanted it to say nothing was stored", out.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := runToggle(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runToggle reported %v, wanted a missing device", err)
		}
	})

	// Verify a setting that cannot be reached at all is reported
	t.Run("ChooseFails", func(t *testing.T) {
		isolate(t)

		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		err := runToggle(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), `looking for "Adjust Key Beep"`) {
			t.Errorf("runToggle reported %v, wanted the failed walk", err)
		}
	})
}
