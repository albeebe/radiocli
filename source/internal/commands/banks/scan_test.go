// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package banks

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// Test_newScan covers the scan subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the command and its closure)
//
// Test cases:
//   - Wiring: the subcommand is named, documented and carries the --all flag
//   - Runs: executing it puts the scanner into a custom search of the bank named
func Test_newScan(t *testing.T) {
	// Builds the reply the scanner answers a screen read with: the display
	// form, then each line paired with its attributes, then the status fields.
	screen := func(lines ...string) string {
		parts := []string{strings.Repeat("0", len(lines))}
		for _, line := range lines {
			parts = append(parts, line, "")
		}
		parts = append(parts, "0", "0", "0", "0", "0", "AM", "0", "0", "0", "0", "0", "0")
		return strings.Join(parts, ",")
	}

	// Verify the subcommand carries the name, the help text and the flag
	t.Run("Wiring", func(t *testing.T) {
		cmd := newScan(appcontext.New())

		if cmd.Name() != "scan" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "scan")
		}
		if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
			t.Error("the subcommand has no help text")
		}
		if cmd.Flags().Lookup("all") == nil {
			t.Error("the --all flag is not registered")
		}
	})

	// Verify running the subcommand switches the named bank on and says so
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		var pressed []string
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "GST"):
				return screen("----------"), nil
			case strings.HasPrefix(command, "KEY,"):
				pressed = append(pressed, command)
			}
			return "", nil
		}}))

		cmd := newScan(app)
		cmd.SetArgs([]string{"9"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if len(pressed) != 1 || pressed[0] != "KEY,9,P" {
			t.Errorf("the scanner was sent %v, wanted only %q", pressed, "KEY,9,P")
		}
		if !strings.Contains(notes.String(), "searching bank 9") {
			t.Errorf("the note is %q, wanted it to name the bank", notes.String())
		}
	})
}

// Test_enabled covers reading the row of banks off the scanner's screen.
//
// Coverage: 100% (7 test cases covering every branch of the row match)
//
// Test cases:
//   - Reads: a row of digits and dashes says which banks are on
//   - Inverse: the bank being swept arrives as a blank and counts as on
//   - SkipsShortLines: a line with fewer characters than there are banks is
//     passed over
//   - SkipsOtherLines: a line that is not the row of banks is passed over
//   - SkipsBlankLines: a row of nothing but blanks is an empty line rather than
//     the banks
//   - ScreenFails: a scanner that will not report its screen is reported
//   - NoRow: a screen with no row of banks on it is refused rather than guessed
func Test_enabled(t *testing.T) {
	// Builds the reply the scanner answers a screen read with: the display
	// form, then each line paired with its attributes, then the status fields.
	screen := func(lines ...string) string {
		parts := []string{strings.Repeat("0", len(lines))}
		for _, line := range lines {
			parts = append(parts, line, "")
		}
		parts = append(parts, "0", "0", "0", "0", "0", "AM", "0", "0", "0", "0", "0", "0")
		return strings.Join(parts, ",")
	}

	// Verify the digits in the row name the banks that are switched on
	t.Run("Reads", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return screen("--2----7--"), nil
		}})

		on, err := enabled(context.Background(), client)
		if err != nil {
			t.Fatalf("enabled: %v", err)
		}
		if len(on) != 2 || !on[2] || !on[7] {
			t.Errorf("the banks on are %v, wanted 2 and 7", on)
		}
	})

	// Verify the bank being swept right now arrives blank and still counts as on
	t.Run("Inverse", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return screen("------ ---"), nil
		}})

		on, err := enabled(context.Background(), client)
		if err != nil {
			t.Fatalf("enabled: %v", err)
		}
		if len(on) != 1 || !on[6] {
			t.Errorf("the banks on are %v, wanted 6 alone", on)
		}
	})

	// Verify a line too short to hold the banks is passed over
	t.Run("SkipsShortLines", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return screen("SRCH", "--------8-"), nil
		}})

		on, err := enabled(context.Background(), client)
		if err != nil {
			t.Fatalf("enabled: %v", err)
		}
		if len(on) != 1 || !on[8] {
			t.Errorf("the banks on are %v, wanted 8 alone", on)
		}
	})

	// Verify a line carrying something else, such as the volume, is passed over
	t.Run("SkipsOtherLines", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return screen("VOL: 1 SQL: 6", "0---------"), nil
		}})

		on, err := enabled(context.Background(), client)
		if err != nil {
			t.Fatalf("enabled: %v", err)
		}
		if len(on) != 1 || !on[0] {
			t.Errorf("the banks on are %v, wanted 0 alone", on)
		}
	})

	// Verify a row of nothing but blanks is read as an empty line, not the banks
	t.Run("SkipsBlankLines", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return screen("          ", "---------9"), nil
		}})

		on, err := enabled(context.Background(), client)
		if err != nil {
			t.Fatalf("enabled: %v", err)
		}
		if len(on) != 1 || !on[9] {
			t.Errorf("the banks on are %v, wanted 9 alone", on)
		}
	})

	// Verify a scanner that will not report its screen is reported as a failed read
	t.Run("ScreenFails", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})

		if _, err := enabled(context.Background(), client); err == nil {
			t.Fatal("enabled reported nothing, wanted the failed read")
		}
	})

	// Verify a screen with no row of banks on it at all is refused
	t.Run("NoRow", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(string) (string, error) {
			return screen("VOL: 1 SQL: 6"), nil
		}})

		_, err := enabled(context.Background(), client)
		if err == nil {
			t.Fatal("enabled reported nothing, wanted the unreadable screen")
		}
		if !strings.Contains(err.Error(), "could not tell which banks") {
			t.Errorf("enabled reported %q, wanted it to say the screen could not be read", err)
		}
	})
}

// Test_runScan covers choosing the banks a custom search sweeps.
//
// Coverage: 100% (10 test cases covering every branch)
//
// Test cases:
//   - Chooses: the banks named are switched on and the unwanted ones off
//   - All: --all switches every bank on
//   - BothWays: naming banks alongside --all is refused
//   - NeitherWay: naming nothing and passing nothing is refused
//   - BadBank: a bank the scanner does not have is refused before it is opened
//   - NoDevice: a run with no scanner named is refused
//   - JumpFails: a scanner that will not enter a custom search is reported
//   - ReadFails: a scanner that will not say which banks are on is reported
//   - SwitchOnFails: a key press that switches a bank on and fails is reported
//   - SwitchOffFails: a key press that switches a bank off and fails is reported
func Test_runScan(t *testing.T) {
	// Builds the reply the scanner answers a screen read with: the display
	// form, then each line paired with its attributes, then the status fields.
	screen := func(lines ...string) string {
		parts := []string{strings.Repeat("0", len(lines))}
		for _, line := range lines {
			parts = append(parts, line, "")
		}
		parts = append(parts, "0", "0", "0", "0", "0", "AM", "0", "0", "0", "0", "0", "0")
		return strings.Join(parts, ",")
	}

	// Verify the wanted banks go on first and the unwanted ones off afterwards
	t.Run("Chooses", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		var pressed []string
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "GST"):
				return screen("0---------"), nil
			case strings.HasPrefix(command, "KEY,"):
				pressed = append(pressed, command)
			}
			return "", nil
		}}))

		if err := runScan(context.Background(), app, []string{"9"}, false); err != nil {
			t.Fatalf("runScan: %v", err)
		}

		// The bank being switched on is pressed before the one being switched
		// off, so the scanner is never left with nothing to sweep.
		want := []string{"KEY,9,P", "KEY,0,P"}
		if strings.Join(pressed, " ") != strings.Join(want, " ") {
			t.Errorf("the scanner was sent %v, wanted %v", pressed, want)
		}
		if !strings.Contains(notes.String(), "searching bank 9") {
			t.Errorf("the note is %q, wanted it to name the bank", notes.String())
		}
	})

	// Verify --all switches on every bank that is not on already
	t.Run("All", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		var pressed int
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "GST"):
				return screen("01--------"), nil
			case strings.HasPrefix(command, "KEY,"):
				pressed++
			}
			return "", nil
		}}))

		if err := runScan(context.Background(), app, nil, true); err != nil {
			t.Fatalf("runScan: %v", err)
		}
		if pressed != 8 {
			t.Errorf("the scanner was sent %d key presses, wanted the 8 banks that were off", pressed)
		}
		if !strings.Contains(notes.String(), "0, 1, 2, 3, 4, 5, 6, 7, 8, 9") {
			t.Errorf("the note is %q, wanted it to name every bank", notes.String())
		}
	})

	// Verify naming banks and passing --all together is refused as contradictory
	t.Run("BothWays", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runScan(context.Background(), app, []string{"1"}, true)
		if err == nil {
			t.Fatal("runScan reported nothing, wanted the contradiction refused")
		}
		if !strings.Contains(err.Error(), "different things") {
			t.Errorf("runScan reported %q, wanted it to name the contradiction", err)
		}
	})

	// Verify asking for no banks at all is refused with a way forward
	t.Run("NeitherWay", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runScan(context.Background(), app, nil, false)
		if err == nil {
			t.Fatal("runScan reported nothing, wanted the empty request refused")
		}
		if !strings.Contains(err.Error(), "name the banks to search") {
			t.Errorf("runScan reported %q, wanted it to say what to do instead", err)
		}
	})

	// Verify a bank the scanner does not have is refused before it is touched
	t.Run("BadBank", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runScan(context.Background(), app, []string{"11"}, false)
		if err == nil {
			t.Fatal("runScan reported nothing, wanted the unknown bank refused")
		}
		if !strings.Contains(err.Error(), "no bank called") {
			t.Errorf("runScan reported %q, wanted it to name the unknown bank", err)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runScan(context.Background(), app, []string{"1"}, false)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runScan reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not start a custom search is reported
	t.Run("JumpFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port closed")
		}}))

		err := runScan(context.Background(), app, []string{"1"}, false)
		if err == nil {
			t.Fatal("runScan reported nothing, wanted the refused custom search")
		}
		if !strings.Contains(err.Error(), "starting a custom search") {
			t.Errorf("runScan reported %q, wanted it to name starting the search", err)
		}
	})

	// Verify a scanner that will not say which banks are on is reported
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "GST") {
				return "", errors.New("the port closed")
			}
			return "", nil
		}}))

		err := runScan(context.Background(), app, []string{"1"}, false)
		if err == nil {
			t.Fatal("runScan reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading which banks are on") {
			t.Errorf("runScan reported %q, wanted it to name reading the banks", err)
		}
	})

	// Verify a bank that cannot be switched on is reported rather than ignored
	t.Run("SwitchOnFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "GST"):
				return screen("----------"), nil
			case strings.HasPrefix(command, "KEY,"):
				return "", errors.New("the port closed")
			}
			return "", nil
		}}))

		err := runScan(context.Background(), app, []string{"3"}, false)
		if err == nil {
			t.Fatal("runScan reported nothing, wanted the refused key press")
		}
		if !strings.Contains(err.Error(), "switching bank 3") {
			t.Errorf("runScan reported %q, wanted it to name the bank", err)
		}
	})

	// Verify a bank that cannot be switched off is reported rather than ignored
	t.Run("SwitchOffFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "GST"):
				return screen("0123456789"), nil
			case strings.HasPrefix(command, "KEY,"):
				return "", errors.New("the port closed")
			}
			return "", nil
		}}))

		// Bank 0 is already on, so nothing is switched on and the first press
		// the command makes is one that switches a bank off.
		err := runScan(context.Background(), app, []string{"0"}, false)
		if err == nil {
			t.Fatal("runScan reported nothing, wanted the refused key press")
		}
		if !strings.Contains(err.Error(), "switching bank 1") {
			t.Errorf("runScan reported %q, wanted it to name the bank", err)
		}
	})
}
