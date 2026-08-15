// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package banks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// Test_newGoto covers the goto subcommand and the closure it hands cobra.
//
// Coverage: 100% (5 test cases covering every branch of the closure)
//
// Test cases:
//   - Wiring: the subcommand is named and documented
//   - Runs: the bank's menu is opened and what is on it is printed
//   - BadBank: a bank the scanner does not have is refused before it is opened
//   - NoDevice: a run with no scanner named is refused
//   - OpenFails: a menu the scanner will not open is reported
func Test_newGoto(t *testing.T) {
	// Verify the subcommand carries the name and the help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newGoto(appcontext.New())

		if cmd.Name() != "goto" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "goto")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify running it leaves the scanner on the bank's menu and shows it
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		s := &bankScanner{menu: []string{entryName}, values: map[string]string{entryName: "CB"}}
		app.SetDevice(device.New(s))

		cmd := newGoto(app)
		cmd.SetArgs([]string{"9"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "Bank 9") {
			t.Errorf("the subcommand wrote %q, wanted the menu it landed on", out.String())
		}
	})

	// Verify a bank the scanner does not have is refused before it is touched
	t.Run("BadBank", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newGoto(app)
		cmd.SetArgs([]string{"12"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("the subcommand reported nothing, wanted the unknown bank refused")
		}
		if !strings.Contains(err.Error(), "no bank called") {
			t.Errorf("the subcommand reported %q, wanted it to name the unknown bank", err)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newGoto(app)
		cmd.SetArgs([]string{"9"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("the subcommand reported %v, wanted a missing device", err)
		}
	})

	// Verify a menu the scanner will not open is reported with the bank named
	t.Run("OpenFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&bankScanner{
			menu: []string{entryName},
			refuse: func(command string, nth int) error {
				if strings.HasPrefix(command, "MNU,") {
					return errors.New("the port closed")
				}
				return nil
			},
		}))

		cmd := newGoto(app)
		cmd.SetArgs([]string{"9"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("the subcommand reported nothing, wanted the refused menu")
		}
		if !strings.Contains(err.Error(), "opening bank 9") {
			t.Errorf("the subcommand reported %q, wanted it to name the bank", err)
		}
	})
}

// Test_renderBanks covers both ways the banks are reported.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Text: the short table carries the fields the list can answer for
//   - Full: the long table carries the settings behind the menus too
//   - Dashes: a field the scanner reports as empty becomes a dash
//   - JSON: the banks are written as a document
//   - WriteFails: output that cannot be written is reported, either way round
func Test_renderBanks(t *testing.T) {
	// Verify the short table names only the columns the list can fill
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		found := []report{{Bank: 0, Name: "CB", Lower: "26.965", Upper: "27.405",
			Modulation: "AM", Step: "10.0 kHz"}}
		if err := renderBanks(app, found, false); err != nil {
			t.Fatalf("renderBanks: %v", err)
		}

		if !strings.Contains(out.String(), "BANK") || !strings.Contains(out.String(), "STEP") {
			t.Errorf("renderBanks wrote %q, wanted the table header", out.String())
		}
		if strings.Contains(out.String(), "DIGITAL") {
			t.Errorf("renderBanks wrote %q, wanted none of the columns the menus fill", out.String())
		}
		if !strings.Contains(out.String(), "CB") {
			t.Errorf("renderBanks wrote %q, wanted the bank in it", out.String())
		}
	})

	// Verify the long table carries the settings only a menu walk can fill
	t.Run("Full", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		found := []report{{Bank: 9, Name: "CB", Attenuator: "Off", Delay: "2 sec",
			DigitalWait: "400 ms", Avoid: "Stop Avoiding", HoldTime: "5"}}
		if err := renderBanks(app, found, true); err != nil {
			t.Fatalf("renderBanks: %v", err)
		}
		for _, column := range []string{"ATT", "DELAY", "DIGITAL", "AVOID", "HOLD"} {
			if !strings.Contains(out.String(), column) {
				t.Errorf("renderBanks wrote %q, wanted the %s column", out.String(), column)
			}
		}
	})

	// Verify an empty field becomes a dash so a column never silently collapses
	t.Run("Dashes", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderBanks(app, []report{{Bank: 3}}, false); err != nil {
			t.Fatalf("renderBanks: %v", err)
		}
		if !strings.Contains(out.String(), "-") {
			t.Errorf("renderBanks wrote %q, wanted dashes where the scanner said nothing", out.String())
		}

		// The same in the long table, which has more columns to lose.
		out.Reset()
		if err := renderBanks(app, []report{{Bank: 3}}, true); err != nil {
			t.Fatalf("renderBanks: %v", err)
		}
		if !strings.Contains(out.String(), "-") {
			t.Errorf("renderBanks wrote %q, wanted dashes where the scanner said nothing", out.String())
		}
	})

	// Verify the JSON output carries the banks as they were read
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		want := []report{{Bank: 9, Name: "CB", Lower: "26.965", Upper: "27.405"}}
		if err := renderBanks(app, want, false); err != nil {
			t.Fatalf("renderBanks: %v", err)
		}

		var got []report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("renderBanks wrote %+v, wanted %+v", got, want)
		}
	})

	// Verify output that cannot be written is reported rather than swallowed
	t.Run("WriteFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = failWriter{}
		app.Stderr = &bytes.Buffer{}

		err := renderBanks(app, []report{{Bank: 0}}, false)
		if err == nil {
			t.Fatal("renderBanks reported nothing, wanted the failed write")
		}
		if !strings.Contains(err.Error(), "writing the bank list") {
			t.Errorf("renderBanks reported %q, wanted it to name writing the list", err)
		}

		// The same write, through the other output format.
		app.Config.Output = appcontext.OutputJSON
		if err := renderBanks(app, []report{{Bank: 0}}, false); err == nil {
			t.Fatal("renderBanks reported nothing, wanted the failed write")
		}
	})
}

// Test_runList covers reading every bank and reporting them.
//
// Coverage: 100% (9 test cases covering every branch)
//
// Test cases:
//   - Lists: a scanner answering for every bank costs one command and no menus
//   - Walks: the banks the list left out are read through the menus instead
//   - Full: --full reads the settings the list cannot carry
//   - NoDevice: a run with no scanner named is refused
//   - ListFails: a list the scanner will not send is reported
//   - WalkFails: a bank that cannot be walked is reported
//   - ExtrasFails: a setting that cannot be read is reported
//   - LeaveFails: a scanner that will not go back to scanning is reported
//   - RenderFails: output that cannot be written is reported
func Test_runList(t *testing.T) {
	// Builds the list document the scanner answers a bank read with. The
	// scanner numbers these from one, where this command numbers them from zero.
	catalogue := func(banks int) string {
		doc := &strings.Builder{}
		doc.WriteString("<GLT>")
		for i := 1; i <= banks; i++ {
			fmt.Fprintf(doc, `<CS_BANK Index="%d" Name="Custom %d" Lower="025.0000" Upper="028.0000" Mod="AM" Step="5000"/>`, i, i-1)
		}
		doc.WriteString("</GLT>")
		return doc.String()
	}

	// Builds a scanner holding a whole bank menu alongside its list.
	holding := func(list string, refuse func(command string, nth int) error) *bankScanner {
		return &bankScanner{
			list:   list,
			menu:   []string{entryLimits, entryName, entryModulation, entryStep, entryAttenuator, entryDelay, entryDigitalWait, entrySearchWithScan},
			limits: []string{"026.9650", "027.4050"},
			values: map[string]string{entryName: "CB", entryHoldTime: "5"},
			choices: map[string][]string{
				entryModulation:     {"AM", "Auto"},
				entryStep:           {"5000", "Auto"},
				entryAttenuator:     {"Off", "On"},
				entryDelay:          {"2 sec", "3 sec"},
				entryDigitalWait:    {"400 ms", "0 ms"},
				entrySearchWithScan: {entryAvoid, entryHoldTime},
				entryAvoid:          {"Stop Avoiding", "Temporary Avoid"},
			},
			refuse: refuse,
		}
	}

	// Verify a scanner that answers for every bank is never taken off scanning
	t.Run("Lists", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		s := holding(catalogue(Count), nil)
		app.SetDevice(device.New(s))

		if err := runList(context.Background(), app, false); err != nil {
			t.Fatalf("runList: %v", err)
		}
		if !strings.Contains(out.String(), "Custom 9") {
			t.Errorf("runList wrote %q, wanted every bank in it", out.String())
		}

		// Nothing was opened, so there was nothing to put back and the scan was
		// never interrupted.
		if len(s.pressed) != 0 {
			t.Errorf("the scanner was sent %v, wanted its scan left alone", s.pressed)
		}
	})

	// Verify a list that stops short is finished off through the menus
	t.Run("Walks", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		// The scanner cuts the document short, so the last bank is missing.
		s := holding(catalogue(Count-1), nil)
		app.SetDevice(device.New(s))

		if err := runList(context.Background(), app, false); err != nil {
			t.Fatalf("runList: %v", err)
		}
		if !strings.Contains(out.String(), "CB") {
			t.Errorf("runList wrote %q, wanted the walked bank in it", out.String())
		}
		if len(s.pressed) == 0 {
			t.Error("the scanner was sent nothing, wanted the missing bank walked")
		}
	})

	// Verify --full reads the settings the scanner's list cannot carry
	t.Run("Full", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(holding(catalogue(Count), nil)))

		if err := runList(context.Background(), app, true); err != nil {
			t.Fatalf("runList: %v", err)
		}
		if !strings.Contains(out.String(), "Stop Avoiding") {
			t.Errorf("runList wrote %q, wanted the settings behind the menus", out.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := runList(context.Background(), app, false); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runList reported %v, wanted a missing device", err)
		}
	})

	// Verify a list the scanner will not send is reported rather than read as empty
	t.Run("ListFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(holding("", func(command string, nth int) error {
			if strings.HasPrefix(command, "GLT,") {
				return errors.New("the port closed")
			}
			return nil
		})))

		if err := runList(context.Background(), app, false); err == nil {
			t.Fatal("runList reported nothing, wanted the failed list")
		}
	})

	// Verify a bank that cannot be walked stops the command
	t.Run("WalkFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(holding(catalogue(Count-1), func(command string, nth int) error {
			if strings.HasPrefix(command, "MNU,") {
				return errors.New("the port closed")
			}
			return nil
		})))

		err := runList(context.Background(), app, false)
		if err == nil {
			t.Fatal("runList reported nothing, wanted the failed walk")
		}
		if !strings.Contains(err.Error(), "opening bank 9") {
			t.Errorf("runList reported %q, wanted it to name the bank", err)
		}
	})

	// Verify a setting behind the menus that cannot be read stops the command
	t.Run("ExtrasFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(holding(catalogue(Count), func(command string, nth int) error {
			if strings.HasPrefix(command, "MNU,") {
				return errors.New("the port closed")
			}
			return nil
		})))

		if err := runList(context.Background(), app, true); err == nil {
			t.Fatal("runList reported nothing, wanted the failed read")
		}
	})

	// Verify a scanner that will not go back to scanning is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(holding(catalogue(Count-1), func(command string, nth int) error {
			if command == "KEY,L,P" {
				return errors.New("the port closed")
			}
			return nil
		})))

		err := runList(context.Background(), app, false)
		if err == nil {
			t.Fatal("runList reported nothing, wanted the scanner it could not put back")
		}
		if !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("runList reported %q, wanted it to name leaving the menus", err)
		}
	})

	// Verify output that cannot be written is reported rather than swallowed
	t.Run("RenderFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = failWriter{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(holding(catalogue(Count), nil)))

		if err := runList(context.Background(), app, false); err == nil {
			t.Fatal("runList reported nothing, wanted the failed write")
		}
	})
}
