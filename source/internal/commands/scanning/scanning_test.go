// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package scanning

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
)

// fakeConn is a device.Conn that answers each command from a function the test
// supplies, so the command can be driven with no scanner attached.
type fakeConn struct {
	reply func(command string) (string, error)
}

// Info describes the connected scanner.
func (f fakeConn) Info() device.Info { return device.Info{} }

// Execute answers the command the way the test asked it to.
func (f fakeConn) Execute(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// ExecuteXML answers the command the way the test asked it to.
func (f fakeConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// Send reports whatever answering the command would have reported.
func (f fakeConn) Send(ctx context.Context, command string) error {
	_, err := f.reply(command)
	return err
}

// Close releases nothing, because there is no port.
func (f fakeConn) Close() error { return nil }

// failWriter is a stream that refuses everything written to it, which is how a
// closed pipe behaves when the output is being read by something else.
type failWriter struct{}

// Write refuses the bytes and says why.
func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("the pipe closed") }

// quiet returns an application context writing to buffers rather than to the
// terminal, along with the buffers it writes to.
func quiet() (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, notes := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, notes
	return app, out, notes
}

// screen builds the reply to "STS" for a scanning screen, marking the channel
// line the way the scanner marks the one it is holding on.
func screen(source, system, department, channel, value string) string {
	return display(source, system, department, channel, value, true)
}

// display builds the reply to "STS", with the channel line marked as the one
// the scanner is holding on only when selected says so.
func display(source, system, department, channel, value string, selected bool) string {
	texts := make([]string, valueLine+1)
	texts[sourceLine] = source
	texts[systemLine] = system
	texts[departmentLine] = department
	texts[channelLine] = channel
	texts[valueLine] = value

	parts := []string{strings.Repeat("0", valueLine+1)}
	for i, text := range texts {
		attributes := " "
		if selected && i == channelLine {
			attributes = "*"
		}
		parts = append(parts, text, attributes)
	}
	return strings.Join(parts, ",")
}

// shortScreen builds the reply to "STS" for a screen with fewer lines than the
// scanning screen, which is what the scanner draws in its menus.
func shortScreen(lines int) string {
	parts := []string{strings.Repeat("0", lines)}
	for i := 0; i < lines; i++ {
		parts = append(parts, "MENU", " ")
	}
	return strings.Join(parts, ",")
}

// notInMenu is the answer the scanner gives to "MSI" while it is scanning.
func notInMenu() string { return `<MSI MenuType="TypeError"/>` }

// inMenu is the answer the scanner gives to "MSI" once the knob has taken it
// off the scanning screen.
func inMenu() string { return `<MSI Name="Main Menu" Index="1" MenuType="TypeSelect"/>` }

// fast shortens the settle after a reset and the ceiling a click waits out, so
// a test exercising those branches spends milliseconds rather than seconds. The
// originals are put back when the test finishes, so nothing leaks into another
// one.
func fast(t *testing.T) {
	t.Helper()

	oldReset, oldStep := resetSettle, stepCap
	t.Cleanup(func() {
		resetSettle, stepCap = oldReset, oldStep
	})

	resetSettle = 2 * time.Millisecond
	stepCap = 5 * time.Millisecond
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (4 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the command is named and carries its help text
//   - Flags: the limit and watch flags carry their defaults
//   - HasSystems: the systems subcommand is attached
//   - Runs: executing the command reaches the scanner and reports its refusal
func TestNew(t *testing.T) {
	// Verify the command carries the name and the help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "scanning" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "scanning")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify the flags that bound the walk are there with their defaults
	t.Run("Flags", func(t *testing.T) {
		cmd := New(appcontext.New())

		limit := cmd.Flags().Lookup("limit")
		if limit == nil || limit.DefValue != fmt.Sprint(defaultLimit) {
			t.Errorf("the limit flag is %v, wanted a default of %d", limit, defaultLimit)
		}
		watch := cmd.Flags().Lookup("watch")
		if watch == nil || watch.DefValue != watchBudget.String() {
			t.Errorf("the watch flag is %v, wanted a default of %s", watch, watchBudget)
		}
	})

	// Verify the systems half of the command is reachable as a subcommand
	t.Run("HasSystems", func(t *testing.T) {
		var found bool
		for _, sub := range New(appcontext.New()).Commands() {
			if sub.Name() == "systems" {
				found = true
			}
		}
		if !found {
			t.Error("the systems subcommand is not attached")
		}
	})

	// Verify executing the command runs the closure cobra was handed
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err == nil {
			t.Fatal("the command accepted a scanner that could not be read")
		}
	})
}

// Test_newSystems covers the systems subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its help text
//   - Runs: executing it reads the scanner and reports its refusal
func Test_newSystems(t *testing.T) {
	// Verify the subcommand carries the name and the help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSystems(appcontext.New())

		if cmd.Use != "systems" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Use, "systems")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify executing the subcommand runs the closure cobra was handed
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		cmd := newSystems(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err == nil {
			t.Fatal("the subcommand accepted a scanner that could not be read")
		}
	})
}

// Test_confirms tests the confirms function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - ComesRound: the systems that followed the start come round again
//   - OnlyTwoChecked: a long recording is confirmed on the first two only
//   - StepError: a click that could not be sent is reported
//   - NoSystem: a screen naming no system at all ends the confirmation
//   - WrongSystem: a different system coming up means the walk had not closed
func Test_confirms(t *testing.T) {
	// Verify the recorded order coming round again confirms the cycle
	t.Run("ComesRound", func(t *testing.T) {
		names := []string{"BRAVO", "CHARLIE"}
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				clicks++
				return "", nil
			}
			name := names[min(clicks, len(names))-1]
			return screen("HOME", name, "DEPT", "CH", "154.0"), nil
		}})

		ok, err := confirms(context.Background(), client, []string{"ALPHA", "BRAVO", "CHARLIE"})
		if err != nil {
			t.Fatalf("confirming the cycle: %v", err)
		}
		if !ok {
			t.Error("confirms rejected a cycle that came round in the recorded order")
		}
	})

	// Verify only the first two systems past the start are checked
	t.Run("OnlyTwoChecked", func(t *testing.T) {
		names := []string{"BRAVO", "CHARLIE", "DELTA"}
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				clicks++
				return "", nil
			}
			name := names[min(clicks, len(names))-1]
			return screen("HOME", name, "DEPT", "CH", "154.0"), nil
		}})

		ok, err := confirms(context.Background(), client,
			[]string{"ALPHA", "BRAVO", "CHARLIE", "DELTA", "ECHO"})
		if err != nil {
			t.Fatalf("confirming the cycle: %v", err)
		}
		if !ok {
			t.Error("confirms rejected a cycle after checking more than it needed to")
		}
		if clicks > cycleConfirm*confirmClicks {
			t.Errorf("confirms clicked %d times, wanted no more than %d",
				clicks, cycleConfirm*confirmClicks)
		}
	})

	// Verify a click that cannot be sent is reported
	t.Run("StepError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
		}})

		if _, err := confirms(context.Background(), client, []string{"ALPHA", "BRAVO"}); err == nil {
			t.Fatal("confirms accepted a knob that would not turn")
		}
	})

	// Verify a screen with no system on it ends the confirmation
	t.Run("NoSystem", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "", "DEPT", "CH", "154.0"), nil
		}})

		ok, err := confirms(context.Background(), client, []string{"ALPHA", "BRAVO"})
		if err != nil {
			t.Fatalf("confirming the cycle: %v", err)
		}
		if ok {
			t.Error("confirms accepted a screen naming no system")
		}
	})

	// Verify a different system coming up means the walk had not closed
	t.Run("WrongSystem", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "ZULU", "DEPT", "CH", "154.0"), nil
		}})

		ok, err := confirms(context.Background(), client, []string{"ALPHA", "BRAVO"})
		if err != nil {
			t.Fatalf("confirming the cycle: %v", err)
		}
		if ok {
			t.Error("confirms accepted a system that was not the one recorded")
		}
	})
}

// Test_cycleSystems tests the cycleSystems function with 100% coverage.
//
// Coverage: 100% (9 test cases covering every branch)
//
// Test cases:
//   - ComesRound: the walk closes and reports the systems it recorded
//   - ResetError: the scanner could not be put back to plain scanning
//   - ArmError: the system level could not be selected
//   - FirstError: the screen could not be read at the start
//   - NoSystems: a scanner naming no system at all reports nothing
//   - StepError: a click could not be sent
//   - WalksOff: a screen with no system on it ends the walk
//   - IntoTheMenus: the knob walking into the menus ends the walk
//   - NeverCloses: a walk that never comes round is reported as a sample
func Test_cycleSystems(t *testing.T) {
	fast(t)

	// Verify a walk that comes back round reports its systems as complete
	t.Run("ComesRound", func(t *testing.T) {
		names := []string{"ALPHA", "BRAVO", "CHARLIE", "ALPHA", "BRAVO", "CHARLIE"}
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				return "", nil
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				return screen("HOME", names[min(clicks, len(names)-1)], "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		names, closed, err := cycleSystems(context.Background(), client)
		if err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if !closed {
			t.Error("the walk did not report coming back round")
		}
		if len(names) != 3 {
			t.Errorf("the walk found %v, wanted three systems", names)
		}
	})

	// Verify a scanner that cannot be put back to scanning is reported
	t.Run("ResetError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch command {
			case "MSI":
				return notInMenu(), nil
			}
			return "", errors.New("the port is gone")
		}})

		if _, _, err := cycleSystems(context.Background(), client); err == nil {
			t.Fatal("cycleSystems accepted a scanner that would not go back to scanning")
		}
	})

	// Verify a system level that cannot be selected is reported
	t.Run("ArmError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "MSI":
				return notInMenu(), nil
			case strings.HasPrefix(command, "KEY,"):
				return "", errors.New("the scanner refused the key")
			}
			return "", nil
		}})

		_, _, err := cycleSystems(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "selecting the system level") {
			t.Fatalf("cycleSystems reported %v, wanted the system level to be blamed", err)
		}
	})

	// Verify a screen that cannot be read at the start is reported
	t.Run("FirstError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				return "", errors.New("the port is gone")
			}
			return "", nil
		}})

		if _, _, err := cycleSystems(context.Background(), client); err == nil {
			t.Fatal("cycleSystems accepted a screen that could not be read")
		}
	})

	// Verify a scanner naming no system reports nothing rather than guessing
	t.Run("NoSystems", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				return screen("HOME", "", "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		names, closed, err := cycleSystems(context.Background(), client)
		if err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if len(names) != 0 || !closed {
			t.Errorf("the walk found %v and reported closed %v, wanted nothing and true", names, closed)
		}
	})

	// Verify a click that cannot be sent during the walk is reported
	t.Run("StepError", func(t *testing.T) {
		var armed bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "MSI":
				return notInMenu(), nil
			case strings.HasPrefix(command, "KEY,>"):
				return "", errors.New("the scanner refused the key")
			case strings.HasPrefix(command, "KEY,"):
				armed = true
				return "", nil
			case command == "STS":
				return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		if _, _, err := cycleSystems(context.Background(), client); err == nil {
			t.Fatal("cycleSystems accepted a knob that would not turn")
		}
		if !armed {
			t.Error("cycleSystems never selected the system level")
		}
	})

	// Verify a screen the knob has taken off any system ends the walk
	t.Run("WalksOff", func(t *testing.T) {
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				return "", nil
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				if clicks > 0 {
					return screen("HOME", "", "DEPT", "CH", "154.0"), nil
				}
				return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		names, closed, err := cycleSystems(context.Background(), client)
		if err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if closed {
			t.Error("the walk claimed to come round after walking off the scanning screen")
		}
		if len(names) != 1 || names[0] != "ALPHA" {
			t.Errorf("the walk found %v, wanted just ALPHA", names)
		}
	})

	// Verify the knob walking into the menus ends the walk rather than reading
	// the menu as though it were a system
	t.Run("IntoTheMenus", func(t *testing.T) {
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				return "", nil
			case command == "MSI":
				if clicks > 0 {
					return inMenu(), nil
				}
				return notInMenu(), nil
			case command == "STS":
				if clicks > 0 {
					return screen("HOME", "Copy System", "DEPT", "CH", "154.0"), nil
				}
				return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		names, closed, err := cycleSystems(context.Background(), client)
		if err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if !closed {
			t.Error("the walk did not treat the menus as the end of the systems")
		}
		if len(names) != 1 || names[0] != "ALPHA" {
			t.Errorf("the walk found %v, wanted just ALPHA", names)
		}
	})

	// Verify a walk that never repeats is reported as a sample rather than all
	t.Run("NeverCloses", func(t *testing.T) {
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				return "", nil
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				return screen("HOME", fmt.Sprintf("SYSTEM %d", clicks), "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		names, closed, err := cycleSystems(context.Background(), client)
		if err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if closed {
			t.Error("the walk claimed to come round when every system was new")
		}
		if len(names) != systemLimit {
			t.Errorf("the walk found %d systems, wanted the limit of %d", len(names), systemLimit)
		}
	})
}

// Test_cycleSystems_failures covers the failures the walk can meet part way
// through, once it is already recording systems.
//
// Coverage: 100% (2 test cases covering the remaining failure branches)
//
// Test cases:
//   - MenuCheckError: the scanner could not be asked whether it is in a menu
//   - ConfirmError: the click confirming the cycle could not be sent
func Test_cycleSystems_failures(t *testing.T) {
	fast(t)

	// Verify a scanner that cannot be asked whether it is in a menu is reported
	t.Run("MenuCheckError", func(t *testing.T) {
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				return "", nil
			case command == "MSI":
				if clicks > 0 {
					return "", errors.New("the port is gone")
				}
				return notInMenu(), nil
			case command == "STS":
				return screen("HOME", fmt.Sprintf("SYSTEM %d", clicks), "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		_, _, err := cycleSystems(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "whether the scanner is still scanning") {
			t.Fatalf("cycleSystems reported %v, wanted the menu check to be blamed", err)
		}
	})

	// Verify a click that cannot be sent while confirming the cycle is reported
	t.Run("ConfirmError", func(t *testing.T) {
		names := []string{"ALPHA", "BRAVO", "ALPHA"}
		var clicks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				// The walk is back at its starting system by now, so the next
				// click is the one confirming the cycle.
				if clicks > len(names)-1 {
					return "", errors.New("the scanner refused the key")
				}
				return "", nil
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				return screen("HOME", names[min(clicks, len(names)-1)], "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		_, _, err := cycleSystems(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("cycleSystems reported %v, wanted the confirming click to be blamed", err)
		}
	})
}

// Test_cycleSystems_stuck covers the branches a knob that moves nothing
// reaches, which are the ones that re-select the system level and then give up.
//
// Coverage: 100% (1 test case covering both stuck branches)
//
// Test cases:
//   - OneSystem: the level is selected again once, and a knob still moving
//     nothing ends the walk
func Test_cycleSystems_stuck(t *testing.T) {
	fast(t)

	// Verify a scan holding one system re-arms once and is then believed.
	//
	// Every click here has to sit out stepCap waiting for a screen that never
	// changes, which is what this branch means, so the case is deliberately
	// run once rather than for each of the two branches it covers.
	t.Run("OneSystem", func(t *testing.T) {
		var arms int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				arms++
				return "", nil
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		names, closed, err := cycleSystems(context.Background(), client)
		if err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if !closed || len(names) != 1 {
			t.Errorf("the walk found %v and reported closed %v, wanted one system and true",
				names, closed)
		}
		if arms != 2 {
			t.Errorf("the system level was selected %d times, wanted it re-armed exactly once", arms)
		}
	})

	// Verify a system level that cannot be selected again is reported.
	//
	// Reaching this needs the whole stuck run of clicks first, which is why it
	// sits alongside the case above rather than in its own test.
	t.Run("RearmError", func(t *testing.T) {
		var arms int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				arms++
				if arms > 1 {
					return "", errors.New("the scanner refused the key")
				}
				return "", nil
			case command == "MSI":
				return notInMenu(), nil
			case command == "STS":
				return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}})

		_, _, err := cycleSystems(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "selecting the system level") {
			t.Fatalf("cycleSystems reported %v, wanted the system level to be blamed", err)
		}
	})
}

// Test_distinct tests the distinct function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - KeepsFirst: repeats are dropped and the first order is kept
//   - Empty: a recording of nothing comes back as nothing
func Test_distinct(t *testing.T) {
	// Verify repeats are dropped without disturbing the order they first came in
	t.Run("KeepsFirst", func(t *testing.T) {
		got := distinct([]string{"ALPHA", "BRAVO", "ALPHA", "CHARLIE", "BRAVO"})

		want := []string{"ALPHA", "BRAVO", "CHARLIE"}
		if len(got) != len(want) {
			t.Fatalf("distinct returned %v, wanted %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("distinct returned %v, wanted %v", got, want)
			}
		}
	})

	// Verify nothing recorded comes back as nothing
	t.Run("Empty", func(t *testing.T) {
		if got := distinct(nil); len(got) != 0 {
			t.Errorf("distinct returned %v, wanted nothing", got)
		}
	})
}

// Test_hold tests the hold function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - AlreadyHolding: a scanner already on a channel is not pressed
//   - Presses: a scanner between channels is pressed until it holds
//   - ReadError: the screen could not be read
//   - PressError: the hold key could not be pressed
//   - NeverHolds: a scanner that will not hold at all is reported
func Test_hold(t *testing.T) {
	// Verify a scanner already holding is left alone
	t.Run("AlreadyHolding", func(t *testing.T) {
		var pressed bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = true
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		if err := hold(context.Background(), client); err != nil {
			t.Fatalf("holding the scanner: %v", err)
		}
		if pressed {
			t.Error("hold pressed the key on a scanner that was already holding")
		}
	})

	// Verify a scanner between channels is pressed and then confirmed
	t.Run("Presses", func(t *testing.T) {
		var pressed bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = true
				return "", nil
			}
			if pressed {
				return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
		}})

		if err := hold(context.Background(), client); err != nil {
			t.Fatalf("holding the scanner: %v", err)
		}
		if !pressed {
			t.Error("hold never pressed the key")
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if err := hold(context.Background(), client); err == nil {
			t.Fatal("hold accepted a screen that could not be read")
		}
	})

	// Verify a refused hold key is reported
	t.Run("PressError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
		}})

		err := hold(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "holding the scanner on a channel") {
			t.Fatalf("hold reported %v, wanted the key to be blamed", err)
		}
	})

	// Verify a scanner that never holds is reported with what to try next
	t.Run("NeverHolds", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
		}})

		err := hold(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "would not hold on a channel") {
			t.Fatalf("hold reported %v, wanted the refusal to be named", err)
		}
	})
}

// Test_holding tests the holding function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - OnAChannel: a scanner stopped on a channel is holding
//   - Cycling: a scanner working through the channels is not
//   - ReadError: the screen could not be read
func Test_holding(t *testing.T) {
	// Verify a scanner stopped on a channel reads as holding
	t.Run("OnAChannel", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		held, err := holding(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the screen: %v", err)
		}
		if !held {
			t.Error("holding said a scanner stopped on a channel is not holding")
		}
	})

	// Verify a scanner mid-cycle does not read as holding
	t.Run("Cycling", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
		}})

		held, err := holding(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the screen: %v", err)
		}
		if held {
			t.Error("holding said a cycling scanner is holding")
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := holding(context.Background(), client); err == nil {
			t.Fatal("holding accepted a screen that could not be read")
		}
	})
}

// Test_inAMenu tests the inAMenu function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Scanning: the scanner refusing the menu request means it is still scanning
//   - InAMenu: a menu answer means the knob has taken it off the scanning screen
//   - ReadError: any other failure is reported
func Test_inAMenu(t *testing.T) {
	// Verify the protocol's refusal is read as the answer rather than a failure
	t.Run("Scanning", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return notInMenu(), nil
		}})

		in, err := inAMenu(context.Background(), client)
		if err != nil {
			t.Fatalf("asking whether the scanner is in a menu: %v", err)
		}
		if in {
			t.Error("inAMenu said a scanning scanner is in a menu")
		}
	})

	// Verify a menu answer is read as the scanner being in one
	t.Run("InAMenu", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return inMenu(), nil
		}})

		in, err := inAMenu(context.Background(), client)
		if err != nil {
			t.Fatalf("asking whether the scanner is in a menu: %v", err)
		}
		if !in {
			t.Error("inAMenu said a scanner in a menu is not in one")
		}
	})

	// Verify any other failure is reported rather than read as an answer
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := inAMenu(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "whether the scanner is still scanning") {
			t.Fatalf("inAMenu reported %v, wanted the question to be named", err)
		}
	})
}

// Test_entry_key tests the entry key method with 100% coverage.
//
// Coverage: 100% (2 test cases covering the method's only path)
//
// Test cases:
//   - EveryField: two entries agreeing on every field share a key
//   - OneFieldApart: a single differing field is enough to tell them apart
func Test_entry_key(t *testing.T) {
	// Verify entries that agree on everything compare equal
	t.Run("EveryField", func(t *testing.T) {
		a := entry{Source: "HOME", System: "ALPHA", Department: "DEPT", Channel: "CH", Value: "154.0"}
		b := entry{Source: "HOME", System: "ALPHA", Department: "DEPT", Channel: "CH", Value: "154.0"}

		if a.key() != b.key() {
			t.Errorf("the keys are %q and %q, wanted them equal", a.key(), b.key())
		}
	})

	// Verify one field apart is enough to keep them separate
	t.Run("OneFieldApart", func(t *testing.T) {
		a := entry{Source: "HOME", System: "ALPHA", Department: "DEPT", Channel: "CH", Value: "154.0"}
		b := a
		b.Value = "154.1"

		if a.key() == b.key() {
			t.Error("two entries differing by their value share a key")
		}
	})
}

// Test_lineText tests the lineText function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - StripsMeter: the signal meter's own glyphs are dropped from the name
//   - NoSuchLine: a screen with fewer lines answers with nothing
//   - ReadError: the screen could not be read
func Test_lineText(t *testing.T) {
	// Verify the meter glyphs and the padding come off the name
	t.Run("StripsMeter", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return screen("HOME", "  ALPHA\x7f\x01 ", "DEPT", "CH", "154.0"), nil
		}})

		got, err := lineText(context.Background(), client, systemLine)
		if err != nil {
			t.Fatalf("reading the line: %v", err)
		}
		if got != "ALPHA" {
			t.Errorf("lineText read %q, wanted %q", got, "ALPHA")
		}
	})

	// Verify a screen without that line answers with nothing at all
	t.Run("NoSuchLine", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return shortScreen(2), nil
		}})

		got, err := lineText(context.Background(), client, systemLine)
		if err != nil {
			t.Fatalf("reading the line: %v", err)
		}
		if got != "" {
			t.Errorf("lineText read %q, wanted nothing", got)
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := lineText(context.Background(), client, systemLine)
		if err == nil || !strings.Contains(err.Error(), "reading the scanner's screen") {
			t.Fatalf("lineText reported %v, wanted the screen to be blamed", err)
		}
	})
}

// Test_listedSystems tests the listedSystems function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Reads: the systems of every list are gathered
//   - SkipsBuiltIn: the built-in search source holds no systems to read
//   - SkipsAvoidedAndRepeats: avoided systems and repeats are left out
//   - ReadError: a list whose systems could not be read is named
func Test_listedSystems(t *testing.T) {
	// Verify the systems of a list are read straight from its memory
	t.Run("Reads", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "GLT,SYS,1" {
				return `<GLT><SYS Name="FIRE" Index="10" Avoid="Off"/></GLT>`, nil
			}
			return "", nil
		}})

		got, err := listedSystems(context.Background(), client,
			[]catalog.FavoritesList{{Name: "HOME", Index: "1"}})
		if err != nil {
			t.Fatalf("reading the systems: %v", err)
		}
		if len(got) != 1 || got[0] != "FIRE" {
			t.Errorf("listedSystems read %v, wanted just FIRE", got)
		}
	})

	// Verify the built-in source is passed over rather than asked
	t.Run("SkipsBuiltIn", func(t *testing.T) {
		var asked bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "GLT,SYS") {
				asked = true
			}
			return `<GLT></GLT>`, nil
		}})

		got, err := listedSystems(context.Background(), client,
			[]catalog.FavoritesList{{Name: "Search with Scan", Index: "4261412864", BuiltIn: true}})
		if err != nil {
			t.Fatalf("reading the systems: %v", err)
		}
		if asked {
			t.Error("listedSystems asked the built-in source for its systems")
		}
		if len(got) != 0 {
			t.Errorf("listedSystems read %v, wanted nothing", got)
		}
	})

	// Verify avoided systems and repeats across lists are left out
	t.Run("SkipsAvoidedAndRepeats", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch command {
			case "GLT,SYS,1":
				return `<GLT><SYS Name="FIRE" Index="10" Avoid="Off"/>` +
					`<SYS Name="SKIPPED" Index="11" Avoid="On"/></GLT>`, nil
			case "GLT,SYS,2":
				return `<GLT><SYS Name="FIRE" Index="12" Avoid="Off"/></GLT>`, nil
			}
			return "", nil
		}})

		got, err := listedSystems(context.Background(), client, []catalog.FavoritesList{
			{Name: "HOME", Index: "1"},
			{Name: "WORK", Index: "2"},
		})
		if err != nil {
			t.Fatalf("reading the systems: %v", err)
		}
		if len(got) != 1 || got[0] != "FIRE" {
			t.Errorf("listedSystems read %v, wanted just FIRE", got)
		}
	})

	// Verify a list that cannot be read is named in the failure
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := listedSystems(context.Background(), client,
			[]catalog.FavoritesList{{Name: "HOME", Index: "1"}})
		if err == nil || !strings.Contains(err.Error(), `reading the systems in "HOME"`) {
			t.Fatalf("listedSystems reported %v, wanted the list to be named", err)
		}
	})
}

// Test_monitored tests the monitored function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - SwitchedOn: only the lists switched on come back
//   - FullDatabase: the built-in database is noticed among them
//   - SearchSource: a built-in that is not the database does not count as it
func Test_monitored(t *testing.T) {
	// Verify a list that is switched off is left out
	t.Run("SwitchedOn", func(t *testing.T) {
		on, database := monitored([]catalog.FavoritesList{
			{Name: "HOME", Monitored: true},
			{Name: "WORK"},
		})

		if len(on) != 1 || on[0].Name != "HOME" {
			t.Errorf("monitored returned %v, wanted just HOME", on)
		}
		if database {
			t.Error("monitored found the full database where there was none")
		}
	})

	// Verify the full database is noticed when it is switched on
	t.Run("FullDatabase", func(t *testing.T) {
		_, database := monitored([]catalog.FavoritesList{
			{Name: fullDatabase, Monitored: true, BuiltIn: true},
		})

		if !database {
			t.Error("monitored did not notice the full database")
		}
	})

	// Verify the other built-in source is not mistaken for the database
	t.Run("SearchSource", func(t *testing.T) {
		on, database := monitored([]catalog.FavoritesList{
			{Name: "Search with Scan", Monitored: true, BuiltIn: true},
		})

		if len(on) != 1 {
			t.Errorf("monitored returned %v, wanted the search source", on)
		}
		if database {
			t.Error("monitored mistook the search source for the full database")
		}
	})
}

// Test_next tests the next function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Moves: the knob turns and the screen catches up with a new channel
//   - PressError: the knob could not be turned
//   - ReadError: the screen could not be read
//   - NeverChanges: a screen that never changes ends the walk
func Test_next(t *testing.T) {
	fast(t)

	// Verify the walk moves on once the screen shows a different channel
	t.Run("Moves", func(t *testing.T) {
		var turned bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				turned = true
				return "", nil
			}
			if turned {
				return screen("HOME", "ALPHA", "DEPT", "CH 2", "154.1"), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		from := entry{Source: "HOME", System: "ALPHA", Department: "DEPT", Channel: "CH 1", Value: "154.0"}
		got, ok, err := next(context.Background(), client, from)
		if err != nil {
			t.Fatalf("stepping to the next channel: %v", err)
		}
		if !ok || got.Channel != "CH 2" {
			t.Errorf("next reported %q and moved %v, wanted CH 2 and true", got.Channel, ok)
		}
	})

	// Verify a knob that will not turn is reported
	t.Run("PressError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		_, _, err := next(context.Background(), client, entry{})
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("next reported %v, wanted the knob to be blamed", err)
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return "", errors.New("the port is gone")
		}})

		if _, _, err := next(context.Background(), client, entry{}); err == nil {
			t.Fatal("next accepted a screen that could not be read")
		}
	})

	// Verify a screen that never changes is the end of the walk, not a failure
	t.Run("NeverChanges", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		from := entry{Source: "HOME", System: "ALPHA", Department: "DEPT", Channel: "CH 1", Value: "154.0"}
		got, ok, err := next(context.Background(), client, from)
		if err != nil {
			t.Fatalf("stepping to the next channel: %v", err)
		}
		if ok || got.Channel != "" {
			t.Errorf("next reported %q and moved %v, wanted nothing and false", got.Channel, ok)
		}
	})
}

// Test_observe tests the observe function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Collects: what comes round while it watches is collected once each
//   - MidCycle: a screen still cycling is read for its system and department
//   - NothingToReport: a screen naming no system at all is passed over
//   - Cancelled: a cancelled run stops the watch
//   - ReadError: the screen could not be read
func Test_observe(t *testing.T) {
	// Verify each channel that comes round is collected exactly once
	t.Run("Collects", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		found, ending, err := observe(context.Background(), client, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("watching the scan: %v", err)
		}
		if ending != endWatched {
			t.Errorf("observe ended as %q, wanted %q", ending, endWatched)
		}
		if len(found) != 1 || found[0].Channel != "CH 1" {
			t.Errorf("observe collected %v, wanted one channel", found)
		}
	})

	// Verify a scanner still working out the talkgroup is read for what it does
	// name, which is the system and the department
	t.Run("MidCycle", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "ALPHA", "DEPT", "ID Scanning...", "", false), nil
		}})

		found, _, err := observe(context.Background(), client, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("watching the scan: %v", err)
		}
		if len(found) != 1 || found[0].System != "ALPHA" || found[0].Channel != "" {
			t.Errorf("observe collected %v, wanted the system and department only", found)
		}
	})

	// Verify a screen with no system on it is passed over
	t.Run("NothingToReport", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "", "DEPT", "Scanning...", "", false), nil
		}})

		found, _, err := observe(context.Background(), client, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("watching the scan: %v", err)
		}
		if len(found) != 0 {
			t.Errorf("observe collected %v, wanted nothing", found)
		}
	})

	// Verify a cancelled run stops rather than watching out the budget
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		if _, _, err := observe(ctx, client, time.Minute); !errors.Is(err, context.Canceled) {
			t.Fatalf("observe reported %v, wanted the cancellation", err)
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, _, err := observe(context.Background(), client, time.Minute); err == nil {
			t.Fatal("observe accepted a screen that could not be read")
		}
	})
}

// Test_partial tests the partial function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Reads: the system and department are taken off a cycling screen
//   - NoSystem: a screen naming no system is not worth reporting
//   - ShortScreen: a screen without those lines is not worth reporting
//   - ReadError: a screen that could not be read is not worth reporting
func Test_partial(t *testing.T) {
	// Verify the lines that are settled mid-cycle are the ones read
	t.Run("Reads", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", " ALPHA ", " DEPT ", "Scanning...", "", false), nil
		}})

		e, ok := partial(context.Background(), client)
		if !ok {
			t.Fatal("partial found nothing worth reporting on a screen naming a system")
		}
		if e.System != "ALPHA" || e.Department != "DEPT" || e.Source != "HOME" {
			t.Errorf("partial read %+v, wanted the source, system and department", e)
		}
	})

	// Verify a screen with no system on it reports nothing
	t.Run("NoSystem", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "", "DEPT", "Scanning...", "", false), nil
		}})

		if _, ok := partial(context.Background(), client); ok {
			t.Error("partial reported a screen naming no system")
		}
	})

	// Verify a screen without those lines reports nothing
	t.Run("ShortScreen", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return shortScreen(2), nil
		}})

		if _, ok := partial(context.Background(), client); ok {
			t.Error("partial reported a screen that has no system line")
		}
	})

	// Verify a screen that cannot be read reports nothing rather than failing
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, ok := partial(context.Background(), client); ok {
			t.Error("partial reported a screen that could not be read")
		}
	})
}

// Test_read tests the read function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Holding: a marked channel is read with everything around it
//   - ShortScreen: a screen without the channel lines shows no channel
//   - NotMarked: an unmarked channel line belongs to something else
//   - EmptyChannel: a blank channel line is not a channel
//   - Cycling: the words the scanner shows while cycling are not a channel
//   - ReadError: the screen could not be read
func Test_read(t *testing.T) {
	// Verify the channel the scanner marks is read along with its surroundings
	t.Run("Holding", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return screen(" HOME ", " ALPHA ", " DEPT ", " CH 1 ", " 154.0 "), nil
		}})

		e, ok, err := read(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the screen: %v", err)
		}
		if !ok {
			t.Fatal("read found no channel on a screen holding one")
		}
		want := entry{Source: "HOME", System: "ALPHA", Department: "DEPT", Channel: "CH 1", Value: "154.0"}
		if e != want {
			t.Errorf("read returned %+v, wanted %+v", e, want)
		}
	})

	// Verify a screen with too few lines shows no channel
	t.Run("ShortScreen", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return shortScreen(2), nil
		}})

		if _, ok, err := read(context.Background(), client); err != nil || ok {
			t.Errorf("read returned %v and %v, wanted no channel and no failure", ok, err)
		}
	})

	// Verify an unmarked channel line is skipped rather than read
	t.Run("NotMarked", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "ALPHA", "DEPT", "CH 1", "154.0", false), nil
		}})

		if _, ok, err := read(context.Background(), client); err != nil || ok {
			t.Errorf("read returned %v and %v, wanted no channel and no failure", ok, err)
		}
	})

	// Verify a blank channel line is not read as a channel
	t.Run("EmptyChannel", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "ALPHA", "DEPT", "", "", true), nil
		}})

		if _, ok, err := read(context.Background(), client); err != nil || ok {
			t.Errorf("read returned %v and %v, wanted no channel and no failure", ok, err)
		}
	})

	// Verify the cycling words are not mistaken for a channel name
	t.Run("Cycling", func(t *testing.T) {
		for _, word := range cycling {
			client := device.New(fakeConn{reply: func(command string) (string, error) {
				return screen("HOME", "ALPHA", "DEPT", word, ""), nil
			}})

			if _, ok, err := read(context.Background(), client); err != nil || ok {
				t.Errorf("read returned %v and %v for %q, wanted no channel", ok, err, word)
			}
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, _, err := read(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading the scanner's screen") {
			t.Fatalf("read reported %v, wanted the screen to be blamed", err)
		}
	})
}

// Test_release tests the release function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - AlreadyScanning: a scanner already scanning is not pressed
//   - Presses: a held scanner is pressed until it carries on
//   - ReadError: the screen could not be read
//   - PressError: the hold key could not be pressed
//   - NeverLetsGo: a scanner that stays held is reported
func Test_release(t *testing.T) {
	// Verify a scanner already scanning is left alone
	t.Run("AlreadyScanning", func(t *testing.T) {
		var pressed bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = true
				return "", nil
			}
			return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
		}})

		let, err := release(context.Background(), client)
		if err != nil {
			t.Fatalf("letting the scanner go: %v", err)
		}
		if !let || pressed {
			t.Errorf("release reported %v and pressed %v, wanted it left alone", let, pressed)
		}
	})

	// Verify a held scanner is pressed and then confirmed to be scanning
	t.Run("Presses", func(t *testing.T) {
		var pressed bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = true
				return "", nil
			}
			if pressed {
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		let, err := release(context.Background(), client)
		if err != nil {
			t.Fatalf("letting the scanner go: %v", err)
		}
		if !let || !pressed {
			t.Errorf("release reported %v and pressed %v, wanted it pressed once", let, pressed)
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := release(context.Background(), client); err == nil {
			t.Fatal("release accepted a screen that could not be read")
		}
	})

	// Verify a refused key is reported
	t.Run("PressError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		_, err := release(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "letting the scanner carry on") {
			t.Fatalf("release reported %v, wanted the key to be blamed", err)
		}
	})

	// Verify a scanner that will not let go is reported with what to run next
	t.Run("NeverLetsGo", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		_, err := release(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "still holding on a channel") {
			t.Fatalf("release reported %v, wanted the hold to be named", err)
		}
	})
}

// Test_renderEntries tests the renderEntries function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Table: the walk is written as an aligned table
//   - Nothing: a walk that found nothing says so instead of drawing a table
//   - WriteError: a stream that refuses the table is reported
func Test_renderEntries(t *testing.T) {
	// Verify the channels are written under their headings
	t.Run("Table", func(t *testing.T) {
		app, out, _ := quiet()

		err := renderEntries(app, []entry{
			{System: "ALPHA", Department: "DEPT", Channel: "CH 1", Value: "154.0"},
		})
		if err != nil {
			t.Fatalf("writing the table: %v", err)
		}
		for _, want := range []string{"SYSTEM", "DEPARTMENT", "ALPHA", "CH 1", "154.0"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("renderEntries wrote %q, wanted %q in it", out, want)
			}
		}
	})

	// Verify a walk that found nothing says so
	t.Run("Nothing", func(t *testing.T) {
		app, out, notes := quiet()

		if err := renderEntries(app, nil); err != nil {
			t.Fatalf("writing the table: %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("renderEntries wrote %q, wanted nothing on the output", out)
		}
		if !strings.Contains(notes.String(), "not scanning anything") {
			t.Errorf("the note was %q, wanted it to say nothing is being scanned", notes)
		}
	})

	// Verify a stream that refuses the table is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := quiet()
		app.Stdout = failWriter{}

		err := renderEntries(app, []entry{{System: "ALPHA", Channel: "CH 1"}})
		if err == nil || !strings.Contains(err.Error(), "writing the channel list") {
			t.Fatalf("renderEntries reported %v, wanted the write to be blamed", err)
		}
	})
}

// Test_report tests the report function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every ending)
//
// Test cases:
//   - Wrapped: a walk that came round says nothing, because it is everything
//   - Limit: stopping at the limit is said to be the limit
//   - Stalled: a walk that gave up is called what was seen
//   - Released: a scanner that let go is called what was seen
//   - Watched: watching is said to be incomplete by its nature
func Test_report(t *testing.T) {
	// Verify a complete walk adds nothing, because the list speaks for itself
	t.Run("Wrapped", func(t *testing.T) {
		app, _, notes := quiet()

		report(app, []entry{{Channel: "CH 1"}}, endWrapped)

		if notes.Len() != 0 {
			t.Errorf("the note was %q, wanted nothing said about a complete walk", notes)
		}
	})

	// Verify stopping at the limit says so and points at the flag
	t.Run("Limit", func(t *testing.T) {
		app, _, notes := quiet()

		report(app, []entry{{Channel: "CH 1"}}, endLimit)

		if !strings.Contains(notes.String(), "--limit") {
			t.Errorf("the note was %q, wanted the flag mentioned", notes)
		}
	})

	// Verify a walk that gave up is described as a sample
	t.Run("Stalled", func(t *testing.T) {
		app, _, notes := quiet()

		report(app, []entry{{Channel: "CH 1"}}, endStalled)

		if !strings.Contains(notes.String(), "rather than everything") {
			t.Errorf("the note was %q, wanted it called a sample", notes)
		}
	})

	// Verify a scanner that let go of the hold is also described as a sample
	t.Run("Released", func(t *testing.T) {
		app, _, notes := quiet()

		report(app, []entry{{Channel: "CH 1"}}, endReleased)

		if !strings.Contains(notes.String(), "rather than everything") {
			t.Errorf("the note was %q, wanted it called a sample", notes)
		}
	})

	// Verify watching is said to be incomplete however much it saw
	t.Run("Watched", func(t *testing.T) {
		app, _, notes := quiet()

		report(app, []entry{{Channel: "CH 1"}}, endWatched)

		if !strings.Contains(notes.String(), "had not come round yet is missing") {
			t.Errorf("the note was %q, wanted it called incomplete", notes)
		}
	})
}

// Test_reportSystems tests the reportSystems function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Names: the systems are written one to a line
//   - JSON: the systems are encoded as JSON
//   - JSONError: a stream that refuses the encoding is reported
//   - Nothing: no systems at all is said rather than printed as a blank
//   - NotClosed: a walk that did not come round is said to be a sample
func Test_reportSystems(t *testing.T) {
	// Verify the systems are written one to a line
	t.Run("Names", func(t *testing.T) {
		app, out, notes := quiet()

		if err := reportSystems(app, []string{"ALPHA", "BRAVO"}, true); err != nil {
			t.Fatalf("writing the systems: %v", err)
		}
		if got, want := out.String(), "ALPHA\nBRAVO\n"; got != want {
			t.Errorf("reportSystems wrote %q, wanted %q", got, want)
		}
		if notes.Len() != 0 {
			t.Errorf("the note was %q, wanted nothing said about a complete walk", notes)
		}
	})

	// Verify JSON mode encodes the systems rather than printing them
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := quiet()
		app.Config.Output = appcontext.OutputJSON

		if err := reportSystems(app, []string{"ALPHA"}, true); err != nil {
			t.Fatalf("writing the systems: %v", err)
		}
		if !strings.Contains(out.String(), `"ALPHA"`) {
			t.Errorf("the JSON was %q, wanted the system in it", out)
		}
	})

	// Verify a stream that refuses the encoding is reported
	t.Run("JSONError", func(t *testing.T) {
		app, _, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		app.Stdout = failWriter{}

		if err := reportSystems(app, []string{"ALPHA"}, true); err == nil {
			t.Fatal("reportSystems accepted a stream that refuses everything")
		}
	})

	// Verify no systems at all is said rather than printed as a blank
	t.Run("Nothing", func(t *testing.T) {
		app, out, notes := quiet()

		if err := reportSystems(app, nil, false); err != nil {
			t.Fatalf("writing the systems: %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("reportSystems wrote %q, wanted nothing on the output", out)
		}
		if !strings.Contains(notes.String(), "not cycling through any systems") {
			t.Errorf("the note was %q, wanted it to say there are none", notes)
		}
	})

	// Verify a walk that did not come round is said to be short of the whole set
	t.Run("NotClosed", func(t *testing.T) {
		app, _, notes := quiet()

		if err := reportSystems(app, []string{"ALPHA"}, false); err != nil {
			t.Fatalf("writing the systems: %v", err)
		}
		if !strings.Contains(notes.String(), "did not come back to where it started") {
			t.Errorf("the note was %q, wanted it called a sample", notes)
		}
	})
}

// Test_reset tests the reset function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Scanning: a scanner already scanning is jumped back and left to settle
//   - LeaveError: a scanner stuck in a menu is reported
//   - JumpError: a scanner that will not go back to scanning is reported
//   - Cancelled: a cancelled run does not sit out the settle
func Test_reset(t *testing.T) {
	fast(t)

	// Verify the scanner is jumped back to plain scanning
	t.Run("Scanning", func(t *testing.T) {
		var jumped bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "MSI":
				return notInMenu(), nil
			case strings.HasPrefix(command, "JPM,"):
				jumped = true
				return "", nil
			}
			return "", nil
		}})

		if err := reset(context.Background(), client); err != nil {
			t.Fatalf("putting the scanner back: %v", err)
		}
		if !jumped {
			t.Error("reset never jumped the scanner back to scanning")
		}
	})

	// Verify a scanner that cannot be taken out of its menus is reported
	t.Run("LeaveError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "MSI":
				return inMenu(), nil
			case strings.HasPrefix(command, "KEY,"):
				return "", errors.New("the scanner refused the key")
			}
			return "", nil
		}})

		if err := reset(context.Background(), client); err == nil {
			t.Fatal("reset accepted a scanner it could not take out of the menus")
		}
	})

	// Verify a scanner that will not go back to scanning is reported
	t.Run("JumpError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "MSI":
				return notInMenu(), nil
			case strings.HasPrefix(command, "JPM,"):
				return "", errors.New("the scanner refused the key")
			}
			return "", nil
		}})

		err := reset(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "returning the scanner to scanning") {
			t.Fatalf("reset reported %v, wanted the jump to be blamed", err)
		}
	})

	// Verify a cancelled run stops rather than sitting out the settle
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "MSI" {
				return notInMenu(), nil
			}
			return "", nil
		}})

		if err := reset(ctx, client); !errors.Is(err, context.Canceled) {
			t.Fatalf("reset reported %v, wanted the cancellation", err)
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (9 test cases covering every branch)
//
// Test cases:
//   - Walks: a favorites list is walked and written as a table
//   - JSON: the walk is encoded as JSON when that was asked for
//   - Watches: the full database is watched rather than walked
//   - BadLimit: a limit below one is refused before the scanner is opened
//   - BadWatch: a watch of no time at all is refused too
//   - NoDevice: no scanner was named
//   - ListsError: the favorites lists could not be read
//   - NothingOn: a scanner with nothing switched on is told what to run
//   - WalkError: a walk that failed is reported
//   - WriteError: a stream that refuses the table is reported
func Test_run(t *testing.T) {
	fast(t)

	// Verify a favorites list is held, walked, and written out
	t.Run("Walks", func(t *testing.T) {
		app, out, _ := quiet()
		var turns int
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="HOME" Index="1" Monitor="On"/></GLT>`, nil
			case strings.HasPrefix(command, "KEY,>"):
				turns++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				return "", nil
			case command == "STS":
				if turns == 1 {
					return screen("HOME", "ALPHA", "DEPT", "CH 2", "154.1"), nil
				}
				return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return "", nil
		}}))

		if err := run(context.Background(), app, defaultLimit, watchBudget); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		for _, want := range []string{"CH 1", "CH 2"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("run wrote %q, wanted %q in it", out, want)
			}
		}
	})

	// Verify JSON mode encodes the channels rather than tabulating them
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="HOME" Index="1" Monitor="On"/></GLT>`, nil
			case command == "STS":
				return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return "", nil
		}}))

		if err := run(context.Background(), app, defaultLimit, watchBudget); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), `"channel": "CH 1"`) {
			t.Errorf("the JSON was %q, wanted the channel in it", out)
		}
	})

	// Verify the full database is watched, which leaves the scanner scanning
	t.Run("Watches", func(t *testing.T) {
		app, out, notes := quiet()
		var pressed bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="Full Database" Index="4294967295" Monitor="On"/></GLT>`, nil
			case strings.HasPrefix(command, "KEY,"):
				pressed = true
				return "", nil
			case command == "STS":
				return screen("Full Database", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return "", nil
		}}))

		if err := run(context.Background(), app, defaultLimit, 50*time.Millisecond); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if pressed {
			t.Error("run touched the scanner's keys while watching the database")
		}
		if !strings.Contains(notes.String(), "Watching the scan cycle") {
			t.Errorf("the note was %q, wanted it to say it is watching", notes)
		}
		if !strings.Contains(out.String(), "CH 1") {
			t.Errorf("run wrote %q, wanted the channel it saw", out)
		}
	})

	// Verify a limit below one is refused before anything is opened
	t.Run("BadLimit", func(t *testing.T) {
		app, _, _ := quiet()

		err := run(context.Background(), app, 0, watchBudget)
		if err == nil || !strings.Contains(err.Error(), "is not a number of channels") {
			t.Fatalf("run reported %v, wanted the limit to be refused", err)
		}
	})

	// Verify a watch of no time at all is refused too
	t.Run("BadWatch", func(t *testing.T) {
		app, _, _ := quiet()

		err := run(context.Background(), app, defaultLimit, 0)
		if err == nil || !strings.Contains(err.Error(), "is not a length of time") {
			t.Fatalf("run reported %v, wanted the watch to be refused", err)
		}
	})

	// Verify a run with no scanner named is refused
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		if err := run(context.Background(), app, defaultLimit, watchBudget); err == nil {
			t.Fatal("run ran with no scanner named")
		}
	})

	// Verify lists that cannot be read are reported
	t.Run("ListsError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		if err := run(context.Background(), app, defaultLimit, watchBudget); err == nil {
			t.Fatal("run accepted lists that could not be read")
		}
	})

	// Verify a scanner with nothing switched on is told what to run instead
	t.Run("NothingOn", func(t *testing.T) {
		app, _, notes := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "GLT,FL" {
				return `<GLT><FL Name="HOME" Index="1" Monitor="Off"/></GLT>`, nil
			}
			return "", nil
		}}))

		if err := run(context.Background(), app, defaultLimit, watchBudget); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(notes.String(), "nothing switched on to scan") {
			t.Errorf("the note was %q, wanted it to say nothing is switched on", notes)
		}
	})

	// Verify a walk that failed is reported
	t.Run("WalkError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="HOME" Index="1" Monitor="On"/></GLT>`, nil
			case command == "STS":
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		if err := run(context.Background(), app, defaultLimit, watchBudget); err == nil {
			t.Fatal("run accepted a walk that could not read the screen")
		}
	})

	// Verify a stream that refuses the table is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := quiet()
		app.Stdout = failWriter{}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="HOME" Index="1" Monitor="On"/></GLT>`, nil
			case command == "STS":
				return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return "", nil
		}}))

		if err := run(context.Background(), app, defaultLimit, watchBudget); err == nil {
			t.Fatal("run accepted a stream that refuses everything")
		}
	})

	// Verify a stream that refuses the encoding is reported
	t.Run("JSONError", func(t *testing.T) {
		app, _, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		app.Stdout = failWriter{}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="HOME" Index="1" Monitor="On"/></GLT>`, nil
			case command == "STS":
				return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return "", nil
		}}))

		if err := run(context.Background(), app, defaultLimit, watchBudget); err == nil {
			t.Fatal("run accepted a stream that refuses the encoding")
		}
	})
}

// Test_runSystems tests the runSystems function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Listed: a favorites list is read from memory without touching the keys
//   - ListedError: the systems of a list could not be read
//   - Database: the full database is walked with the knob and put back after
//   - WalkError: a walk that failed is reported
//   - NoDevice: no scanner was named
//   - ListsError: the favorites lists could not be read
//   - NothingOn: a scanner with nothing switched on is told what to run
func Test_runSystems(t *testing.T) {
	fast(t)

	// Verify a favorites list is answered from memory, silently
	t.Run("Listed", func(t *testing.T) {
		app, out, _ := quiet()
		var pressed bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="HOME" Index="1" Monitor="On"/></GLT>`, nil
			case command == "GLT,SYS,1":
				return `<GLT><SYS Name="FIRE" Index="10" Avoid="Off"/></GLT>`, nil
			case strings.HasPrefix(command, "KEY,"):
				pressed = true
				return "", nil
			}
			return "", nil
		}}))

		if err := runSystems(context.Background(), app); err != nil {
			t.Fatalf("listing the systems: %v", err)
		}
		if pressed {
			t.Error("runSystems pressed a key to answer from a favorites list")
		}
		if got, want := out.String(), "FIRE\n"; got != want {
			t.Errorf("runSystems wrote %q, wanted %q", got, want)
		}
	})

	// Verify a list whose systems cannot be read is reported
	t.Run("ListedError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="HOME" Index="1" Monitor="On"/></GLT>`, nil
			case command == "GLT,SYS,1":
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		if err := runSystems(context.Background(), app); err == nil {
			t.Fatal("runSystems accepted systems that could not be read")
		}
	})

	// Verify the database is walked with the knob and the scanner put back
	t.Run("Database", func(t *testing.T) {
		app, out, notes := quiet()
		names := []string{"ALPHA", "BRAVO", "ALPHA", "BRAVO"}
		var clicks int
		var left bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="Full Database" Index="4294967295" Monitor="On"/></GLT>`, nil
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				return "", nil
			case command == "MSI":
				if left {
					return inMenu(), nil
				}
				return notInMenu(), nil
			case command == "STS":
				return screen("Full Database", names[min(clicks, len(names)-1)], "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}}))

		if err := runSystems(context.Background(), app); err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if !strings.Contains(notes.String(), "Turning the knob") {
			t.Errorf("the note was %q, wanted it to say the knob is being turned", notes)
		}
		if !strings.Contains(out.String(), "ALPHA") {
			t.Errorf("runSystems wrote %q, wanted the systems it walked", out)
		}
	})

	// Verify a walk that failed is reported
	t.Run("WalkError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="Full Database" Index="4294967295" Monitor="On"/></GLT>`, nil
			case command == "MSI":
				return notInMenu(), nil
			case strings.HasPrefix(command, "JPM,"):
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		if err := runSystems(context.Background(), app); err == nil {
			t.Fatal("runSystems accepted a walk that could not start")
		}
	})

	// Verify a knob that walked the scanner into the menus is taken back out
	t.Run("LeavesTheMenus", func(t *testing.T) {
		app, _, _ := quiet()
		names := []string{"ALPHA", "BRAVO", "ALPHA", "BRAVO"}
		var clicks int
		var walked, left bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "GLT,FL":
				return `<GLT><FL Name="Full Database" Index="4294967295" Monitor="On"/></GLT>`, nil
			case strings.HasPrefix(command, "KEY,>"):
				clicks++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				// The key that leaves the menus is refused, which is enough to
				// show the attempt was made without walking the menus here.
				if walked {
					left = true
					return "", errors.New("the scanner refused the key")
				}
				return "", nil
			case command == "MSI":
				// The walk finishes on the menu screen the knob wandered into.
				if clicks >= len(names)-1 {
					walked = true
					return inMenu(), nil
				}
				return notInMenu(), nil
			case command == "STS":
				return screen("Full Database", names[min(clicks, len(names)-1)], "DEPT", "CH", "154.0"), nil
			}
			return "", nil
		}}))

		if err := runSystems(context.Background(), app); err != nil {
			t.Fatalf("walking the systems: %v", err)
		}
		if !left {
			t.Error("runSystems left the scanner sitting in a menu")
		}
	})

	// Verify a run with no scanner named is refused
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		if err := runSystems(context.Background(), app); err == nil {
			t.Fatal("runSystems ran with no scanner named")
		}
	})

	// Verify lists that cannot be read are reported
	t.Run("ListsError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		if err := runSystems(context.Background(), app); err == nil {
			t.Fatal("runSystems accepted lists that could not be read")
		}
	})

	// Verify a scanner with nothing switched on is told what to run instead
	t.Run("NothingOn", func(t *testing.T) {
		app, _, notes := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "GLT,FL" {
				return `<GLT><FL Name="HOME" Index="1" Monitor="Off"/></GLT>`, nil
			}
			return "", nil
		}}))

		if err := runSystems(context.Background(), app); err != nil {
			t.Fatalf("listing the systems: %v", err)
		}
		if !strings.Contains(notes.String(), "nothing switched on to scan") {
			t.Errorf("the note was %q, wanted it to say nothing is switched on", notes)
		}
	})
}

// Test_settledHold tests the settledHold function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - StillHolding: every look finding it held reports it held
//   - LetGo: one look finding it scanning is enough
//   - ReadError: the screen could not be read
func Test_settledHold(t *testing.T) {
	// Verify a scanner held throughout every look reads as held
	t.Run("StillHolding", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		held, err := settledHold(context.Background(), client)
		if err != nil {
			t.Fatalf("looking at the screen: %v", err)
		}
		if !held {
			t.Error("settledHold said a held scanner is not holding")
		}
	})

	// Verify one look finding it scanning ends the checking
	t.Run("LetGo", func(t *testing.T) {
		var looks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			looks++
			if looks > 2 {
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		held, err := settledHold(context.Background(), client)
		if err != nil {
			t.Fatalf("looking at the screen: %v", err)
		}
		if held {
			t.Error("settledHold said a scanning scanner is holding")
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := settledHold(context.Background(), client); err == nil {
			t.Fatal("settledHold accepted a screen that could not be read")
		}
	})
}

// Test_settledScan tests the settledScan function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Holds: one look finding it on a channel is enough
//   - NeverHolds: no look finding it on a channel reports it is not
//   - ReadError: the screen could not be read
func Test_settledScan(t *testing.T) {
	// Verify one look finding it on a channel ends the waiting
	t.Run("Holds", func(t *testing.T) {
		var looks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			looks++
			if looks > 2 {
				return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return display("HOME", "ALPHA", "DEPT", "ID Scanning...", "", false), nil
		}})

		held, err := settledScan(context.Background(), client)
		if err != nil {
			t.Fatalf("looking at the screen: %v", err)
		}
		if !held {
			t.Error("settledScan said a scanner that stopped is not holding")
		}
	})

	// Verify a scanner that never stops reads as not holding
	t.Run("NeverHolds", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
		}})

		held, err := settledScan(context.Background(), client)
		if err != nil {
			t.Fatalf("looking at the screen: %v", err)
		}
		if held {
			t.Error("settledScan said a cycling scanner is holding")
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := settledScan(context.Background(), client); err == nil {
			t.Fatal("settledScan accepted a screen that could not be read")
		}
	})
}

// Test_stepSystem tests the stepSystem function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Changes: the click is followed by the screen catching up
//   - NoSystem: a screen with no system on it is returned rather than waited on
//   - PressError: the knob could not be turned
//   - ReadError: the screen could not be read
//   - Cancelled: a cancelled run stops the wait
//   - NeverChanges: a screen that never changes returns the name it kept seeing
func Test_stepSystem(t *testing.T) {
	fast(t)

	// Verify the name the screen catches up with is the one returned
	t.Run("Changes", func(t *testing.T) {
		var clicked bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				clicked = true
				return "", nil
			}
			if clicked {
				return screen("HOME", "BRAVO", "DEPT", "CH", "154.0"), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
		}})

		got, err := stepSystem(context.Background(), client, "ALPHA")
		if err != nil {
			t.Fatalf("clicking to the next system: %v", err)
		}
		if got != "BRAVO" {
			t.Errorf("stepSystem read %q, wanted %q", got, "BRAVO")
		}
	})

	// Verify a screen naming no system is returned rather than waited out
	t.Run("NoSystem", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "", "DEPT", "CH", "154.0"), nil
		}})

		got, err := stepSystem(context.Background(), client, "")
		if err != nil {
			t.Fatalf("clicking to the next system: %v", err)
		}
		if got != "" {
			t.Errorf("stepSystem read %q, wanted nothing", got)
		}
	})

	// Verify a knob that will not turn is reported
	t.Run("PressError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
		}})

		_, err := stepSystem(context.Background(), client, "ALPHA")
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("stepSystem reported %v, wanted the knob to be blamed", err)
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return "", errors.New("the port is gone")
		}})

		if _, err := stepSystem(context.Background(), client, "ALPHA"); err == nil {
			t.Fatal("stepSystem accepted a screen that could not be read")
		}
	})

	// Verify a cancelled run stops rather than waiting for the screen
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
		}})

		if _, err := stepSystem(ctx, client, "ALPHA"); !errors.Is(err, context.Canceled) {
			t.Fatalf("stepSystem reported %v, wanted the cancellation", err)
		}
	})

	// Verify a screen that never changes gives back the name it kept seeing.
	//
	// This is the one case that has to sit out stepCap, because waiting for the
	// change is exactly what is being tested.
	t.Run("NeverChanges", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH", "154.0"), nil
		}})

		got, err := stepSystem(context.Background(), client, "ALPHA")
		if err != nil {
			t.Fatalf("clicking to the next system: %v", err)
		}
		if got != "ALPHA" {
			t.Errorf("stepSystem read %q, wanted the name it kept seeing", got)
		}
	})
}

// Test_systemName tests the systemName function with 100% coverage.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Reads: the system the screen names is read
//   - ReadError: the screen could not be read
func Test_systemName(t *testing.T) {
	// Verify the system line is the one read
	t.Run("Reads", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		got, err := systemName(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the system: %v", err)
		}
		if got != "ALPHA" {
			t.Errorf("systemName read %q, wanted %q", got, "ALPHA")
		}
	})

	// Verify a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := systemName(context.Background(), client); err == nil {
			t.Fatal("systemName accepted a screen that could not be read")
		}
	})
}

// Test_walk tests the walk function with 100% coverage.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - ComesRound: arriving back at the first channel closes the walk
//   - NothingShowing: a screen showing no channel at all ends it at once
//   - FirstReadError: the screen could not be read at the start
//   - Limit: the walk stops at the limit it was given
//   - StepError: the knob could not be turned
//   - RetakesHold: a scanner that let go is taken hold of again
//   - GivesUpHold: a scanner that will not hold again ends the walk
//   - StillNotShowing: a re-held scanner showing nothing ends the walk
//   - Stalls: nothing but repeats for long enough ends the walk
func Test_walk(t *testing.T) {
	fast(t)

	// Verify arriving back at the starting channel closes the walk
	t.Run("ComesRound", func(t *testing.T) {
		channels := []string{"CH 1", "CH 2", "CH 1"}
		var turns int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				turns++
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", channels[min(turns, len(channels)-1)], "154.0"), nil
		}})

		found, ending, err := walk(context.Background(), client, defaultLimit)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if ending != endWrapped {
			t.Errorf("the walk ended as %q, wanted %q", ending, endWrapped)
		}
		if len(found) != 2 {
			t.Errorf("the walk found %v, wanted two channels", found)
		}
	})

	// Verify a screen showing no channel ends the walk immediately
	t.Run("NothingShowing", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
		}})

		found, ending, err := walk(context.Background(), client, defaultLimit)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if ending != endWrapped || len(found) != 0 {
			t.Errorf("the walk found %v and ended as %q, wanted nothing and %q",
				found, ending, endWrapped)
		}
	})

	// Verify a screen that cannot be read at the start is reported
	t.Run("FirstReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, _, err := walk(context.Background(), client, defaultLimit); err == nil {
			t.Fatal("walk accepted a screen that could not be read")
		}
	})

	// Verify the walk stops at the limit rather than running on
	t.Run("Limit", func(t *testing.T) {
		var turns int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				turns++
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", fmt.Sprintf("CH %d", turns), "154.0"), nil
		}})

		found, ending, err := walk(context.Background(), client, 3)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if ending != endLimit || len(found) != 3 {
			t.Errorf("the walk found %d channels and ended as %q, wanted 3 and %q",
				len(found), ending, endLimit)
		}
	})

	// Verify a knob that will not turn is reported
	t.Run("StepError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		if _, _, err := walk(context.Background(), client, defaultLimit); err == nil {
			t.Fatal("walk accepted a knob that would not turn")
		}
	})

	// Verify a scanner that let go of the hold is taken hold of and carries on
	t.Run("RetakesHold", func(t *testing.T) {
		var turns, holds int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				turns++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				holds++
				return "", nil
			}
			// The scanner lets go of the hold after the first turn and shows
			// nothing again until the hold key has been pressed.
			switch {
			case holds > 0:
				return screen("HOME", "ALPHA", "DEPT", "CH 2", "154.1"), nil
			case turns > 0:
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		found, _, err := walk(context.Background(), client, 2)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if holds == 0 {
			t.Error("walk never took hold of the scanner again")
		}
		if len(found) != 2 {
			t.Errorf("the walk found %v, wanted both channels", found)
		}
	})

	// Verify a scanner that will not hold again ends the walk with what it has
	t.Run("GivesUpHold", func(t *testing.T) {
		var turns int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				turns++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				return "", errors.New("the scanner refused the key")
			}
			// The scanner lets go after the first turn, and the key that would
			// take hold of it again is refused.
			if turns > 0 {
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		found, ending, err := walk(context.Background(), client, defaultLimit)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if ending != endReleased || len(found) != 1 {
			t.Errorf("the walk found %d channels and ended as %q, wanted 1 and %q",
				len(found), ending, endReleased)
		}
	})

	// Verify a re-held scanner still showing nothing ends the walk
	t.Run("StillNotShowing", func(t *testing.T) {
		var turns, holds, afterHold int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				turns++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				holds++
				return "", nil
			}
			// The scanner lets go after the first turn. The press takes hold
			// long enough to be confirmed once, and it has let go again by the
			// time the walk reads the channel it was supposed to have landed on.
			if holds > 0 {
				afterHold++
				if afterHold == 1 {
					return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
				}
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			if turns > 0 {
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		found, ending, err := walk(context.Background(), client, defaultLimit)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if ending != endReleased {
			t.Errorf("the walk ended as %q, wanted %q", ending, endReleased)
		}
		if len(found) != 1 {
			t.Errorf("the walk found %v, wanted the one channel it saw", found)
		}
	})

	// Verify a screen that cannot be read after taking hold again is reported
	t.Run("RetakenReadError", func(t *testing.T) {
		var turns, holds, afterHold int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				turns++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				holds++
				return "", nil
			}
			// The press takes hold long enough to be confirmed once, and the
			// port goes away before the channel it landed on can be read.
			if holds > 0 {
				afterHold++
				if afterHold == 1 {
					return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
				}
				return "", errors.New("the port is gone")
			}
			if turns > 0 {
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		if _, _, err := walk(context.Background(), client, defaultLimit); err == nil {
			t.Fatal("walk accepted a screen that could not be read after taking hold")
		}
	})

	// Verify nothing but repeats for long enough ends the walk as a sample
	t.Run("Stalls", func(t *testing.T) {
		repeats := []string{"CH 2", "CH 3"}
		var turns int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				turns++
				return "", nil
			}
			// Never the starting channel again, and after the first two steps
			// never anything new: every step lands on one already recorded.
			if turns == 0 {
				return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
			}
			return screen("HOME", "ALPHA", "DEPT", repeats[turns%2], "154.0"), nil
		}})

		found, ending, err := walk(context.Background(), client, defaultLimit)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if ending != endStalled {
			t.Errorf("the walk ended as %q, wanted %q", ending, endStalled)
		}
		if len(found) != 3 {
			t.Errorf("the walk found %v, wanted the three channels it saw", found)
		}
	})
}

// Test_walkHeld tests the walkHeld function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Walks: the scanner is held, walked, and let go again
//   - HoldError: a scanner that would not hold is reported
//   - ReleaseError: a scanner left holding is said so in a note
func Test_walkHeld(t *testing.T) {
	fast(t)

	// Verify the scanner is held for the walk and let go afterwards
	t.Run("Walks", func(t *testing.T) {
		app, _, notes := quiet()
		channels := []string{"CH 1", "CH 2", "CH 1"}
		var turns int
		var released bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "KEY,>"):
				turns++
				return "", nil
			case strings.HasPrefix(command, "KEY,"):
				released = true
				return "", nil
			}
			if released {
				return display("HOME", "ALPHA", "DEPT", "Scanning...", "", false), nil
			}
			return screen("HOME", "ALPHA", "DEPT", channels[min(turns, len(channels)-1)], "154.0"), nil
		}})

		found, ending, err := walkHeld(context.Background(), app, client, defaultLimit)
		if err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if ending != endWrapped || len(found) != 2 {
			t.Errorf("the walk found %v and ended as %q, wanted two channels and %q",
				found, ending, endWrapped)
		}
		if notes.Len() != 0 {
			t.Errorf("the note was %q, wanted nothing said about a scanner that was let go", notes)
		}
	})

	// Verify a scanner that would not hold is reported
	t.Run("HoldError", func(t *testing.T) {
		app, _, _ := quiet()
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, _, err := walkHeld(context.Background(), app, client, defaultLimit); err == nil {
			t.Fatal("walkHeld accepted a scanner that could not be read")
		}
	})

	// Verify a scanner left holding is said so rather than left silently
	t.Run("ReleaseError", func(t *testing.T) {
		app, _, notes := quiet()
		var turns int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,>") {
				turns++
				return "", nil
			}
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return screen("HOME", "ALPHA", "DEPT", "CH 1", "154.0"), nil
		}})

		if _, _, err := walkHeld(context.Background(), app, client, defaultLimit); err != nil {
			t.Fatalf("walking the channels: %v", err)
		}
		if !strings.Contains(notes.String(), "was left holding on a channel") {
			t.Errorf("the note was %q, wanted it to say the scanner was left holding", notes)
		}
	})
}
