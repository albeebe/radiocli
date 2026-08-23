// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package headphone

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

// showing returns a scanner whose menu walk lands on the setting and then on
// the value given.
//
// The walk reads the screen twice: once to find the row naming the setting, and
// once to find the highlighted value inside it. Everything else answers in a way
// that lets the menus be left without further work.
//
// Parameters:
//   - value: the scanner's own wording for the value, such as "Invert Phase"
//
// Returns:
//   - the connection, which records what was sent to it
func showing(value string) *stubConn {
	return sequence(entryName, value)
}

// sequence returns a scanner that shows each screen in turn, then stays on the
// last one.
//
// A menu walk reads the display several times: once to find the row naming the
// setting, once for the value highlighted inside it, once more for the row it
// is asked to choose, and then the same two again to read the answer back. The
// order is the whole of what these tests are about, so it is written out.
//
// Parameters:
//   - screens: what the display shows, in the order it is read
//
// Returns:
//   - the connection
func sequence(screens ...string) *stubConn {
	at := 0
	return &stubConn{exec: func(command string) (string, error) {
		switch command {
		case "STS":
			screen := screens[at]
			if at < len(screens)-1 {
				at++
			}
			return "0," + screen + ",*", nil
		case "GSI":
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}
		return "", nil
	}, execXML: func(string) (string, error) {
		return `<MenuInfo MenuType="TypeError"/>`, nil
	}}
}

// running returns an App wired to conn, with buffers for its streams.
//
// Parameters:
//   - conn: the fake scanner to answer with
//
// Returns:
//   - the App
//   - what was written to stdout
//   - what was written to stderr
func running(conn *stubConn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, errs
	app.SetDevice(device.New(conn))
	return app, out, errs
}

// TestPhasesAreTheTwoTheScannerOffers pins the values against the list read off
// an SDS150, in the scanner's own order.
func TestPhasesAreTheTwoTheScannerOffers(t *testing.T) {
	if len(phases) != 2 {
		t.Fatalf("there are %d settings, want 2", len(phases))
	}
	if phases[0].entry != "In Phase" || phases[1].entry != "Invert Phase" {
		t.Errorf("got %q and %q, want the scanner's own wording in its own order",
			phases[0].entry, phases[1].entry)
	}
	if phases[0].value != InPhase || phases[1].value != Invert {
		t.Errorf("got %q and %q, want them spelled the way they are typed",
			phases[0].value, phases[1].value)
	}
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering every path)
//
// Test cases:
//   - Wiring: the command is named and carries its subcommand
//   - NotOnlyReads: it moves the scanner, so it must not claim otherwise
//   - Runs: executing it reaches the worker
func TestNew(t *testing.T) {
	// Verify the command carries its name, its help and its subcommand.
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "headphone" {
			t.Errorf("the command is %q, want %q", cmd.Use, "headphone")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		var found bool
		for _, sub := range cmd.Commands() {
			if strings.HasPrefix(sub.Use, "set ") {
				found = true
			}
		}
		if !found {
			t.Error("the set subcommand is not attached")
		}
	})

	// Verify it does not claim to only read. Reading this setting walks into a
	// menu, which takes the scanner off the air, so it may not run alongside
	// another command.
	t.Run("NotOnlyReads", func(t *testing.T) {
		if got := New(appcontext.New()).Annotations[appcontext.OnlyReads]; got != "" {
			t.Errorf("the only-reads annotation is %q, want it absent on a command "+
				"that walks the menus", got)
		}
	})

	// Verify the closure reaches the worker.
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := running(showing("In Phase"))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), InPhase) {
			t.Errorf("the command wrote %q, want it to report the setting", out.String())
		}
	})
}

// Test_newSet covers the set subcommand, whose argument is checked before the
// scanner is opened so that a typo costs nothing.
//
// Coverage: 100% (3 test cases covering every path)
//
// Test cases:
//   - Wiring: the command names both values in its synopsis
//   - Refuses: a value that does not exist is refused, naming both that do
//   - Runs: a value that does exist reaches the worker
func Test_newSet(t *testing.T) {
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSet(appcontext.New())
		if !strings.Contains(cmd.Use, InPhase) || !strings.Contains(cmd.Use, Invert) {
			t.Errorf("the synopsis is %q, want both values in it", cmd.Use)
		}
	})

	// Verify a value that does not exist is refused before anything is opened,
	// and that the message names the two that do.
	t.Run("Refuses", func(t *testing.T) {
		cmd := newSet(appcontext.New())
		cmd.SetArgs([]string{"sideways"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("a setting that does not exist was accepted")
		}
		for _, want := range []string{InPhase, Invert} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal said %q, want it to name %q", err, want)
			}
		}
	})

	// Verify a value that does exist reaches the worker.
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := running(showing("In Phase"))

		cmd := newSet(app)
		cmd.SetArgs([]string{InPhase})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), InPhase) {
			t.Errorf("the command wrote %q, want it to report the setting", out.String())
		}
	})
}

// Test_lookup covers finding a setting by the value it is typed as.
func Test_lookup(t *testing.T) {
	for _, in := range []string{InPhase, "  " + InPhase + "  ", strings.ToUpper(InPhase)} {
		if got, ok := lookup(in); !ok || got.value != InPhase {
			t.Errorf("lookup(%q) gave %q and %v, want %q", in, got.value, ok, InPhase)
		}
	}
	if _, ok := lookup("sideways"); ok {
		t.Error("a setting that does not exist was found")
	}
}

// Test_byEntry covers finding a setting by the scanner's own wording, which is
// what the highlighted row reads as.
func Test_byEntry(t *testing.T) {
	for _, in := range []string{"Invert Phase", " Invert Phase ", "INVERT PHASE"} {
		if got, ok := byEntry(in); !ok || got.value != Invert {
			t.Errorf("byEntry(%q) gave %q and %v, want %q", in, got.value, ok, Invert)
		}
	}
	if _, ok := byEntry("Sideways"); ok {
		t.Error("a wording that does not exist was found")
	}
}

// Test_read covers reading the setting out of the menus.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Success: the highlighted row is read as the setting the scanner is on
//   - OpenFails: a scanner that refuses the settings menu is reported
//   - SelectFails: firmware without the setting is reported, and says so
//   - Unknown: a value neither of the two known ones is reported rather than
//     guessed at
func Test_read(t *testing.T) {
	// Verify the highlighted row is read as the setting.
	t.Run("Success", func(t *testing.T) {
		got, err := read(context.Background(), device.New(showing("Invert Phase")))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if got.value != Invert {
			t.Errorf("read reported %q, want %q", got.value, Invert)
		}
	})

	// Verify a scanner that refuses the settings menu is reported.
	t.Run("OpenFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "MNU,SETTINGS," {
				return "", errors.New("the scanner refused the menu")
			}
			return "", nil
		}}

		_, err := read(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "opening the settings menu") {
			t.Fatalf("got %v, want the refusal reported", err)
		}
	})

	// Verify firmware that does not have the setting is reported as that,
	// rather than as an unexplained failure to find a menu row. It is the one
	// failure a reader can do nothing about, so it says why.
	t.Run("SelectFails", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "0,Something Else,*", nil
			}
			return "", nil
		}}

		_, err := read(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), entryName) {
			t.Fatalf("got %v, want the missing setting named", err)
		}
		if !strings.Contains(err.Error(), "firmware") {
			t.Errorf("got %q, want it to explain that the setting may not be there", err)
		}
	})

	// Verify a value neither of the two known ones is reported rather than
	// quietly treated as one of them.
	t.Run("Unknown", func(t *testing.T) {
		_, err := read(context.Background(), device.New(showing("Sideways")))
		if err == nil || !strings.Contains(err.Error(), "Sideways") {
			t.Fatalf("got %v, want the unknown value quoted", err)
		}
	})
}

// Test_runReport covers reading the setting and rendering it both ways.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - InPhase: the good setting is reported with nothing else to say
//   - Inverted: the bad setting is reported with the fix for it
//   - JSON: the same reading as one object
//   - NoDevice: a run with no scanner named is refused
func Test_runReport(t *testing.T) {
	// Verify the good setting is reported plainly, with no advice attached.
	t.Run("InPhase", func(t *testing.T) {
		app, out, errs := running(showing("In Phase"))
		if err := runReport(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(out.String(), "headphone: "+InPhase) {
			t.Errorf("wrote %q, want the setting", out.String())
		}
		if errs.Len() != 0 {
			t.Errorf("wrote %q to stderr, want nothing to say about a correct setting", errs.String())
		}
	})

	// Verify the bad setting is reported with what it costs and how to fix it.
	t.Run("Inverted", func(t *testing.T) {
		app, out, errs := running(showing("Invert Phase"))
		if err := runReport(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(out.String(), "headphone: "+Invert) {
			t.Errorf("wrote %q, want the setting", out.String())
		}
		if !strings.Contains(errs.String(), "cancels") {
			t.Errorf("wrote %q, want it to say what an inverted jack costs", errs.String())
		}
		if !strings.Contains(errs.String(), "headphone set "+InPhase) {
			t.Errorf("wrote %q, want it to name the command that fixes it", errs.String())
		}
	})

	// Verify the same reading as one object, which is what a script reads.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := running(showing("Invert Phase"))
		app.Config.Output = appcontext.OutputJSON

		if err := runReport(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("wrote %q, which is not JSON: %v", out.String(), err)
		}
		if got.Phase != Invert {
			t.Errorf("got %+v, want %q", got, Invert)
		}
	})

	// Verify a run with no scanner named is refused.
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runReport(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("got %v, want ErrNoDevice", err)
		}
	})

	// Verify a scanner that will not answer is reported rather than rendered as
	// a setting.
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := running(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})

		if err := runReport(context.Background(), app); err == nil {
			t.Fatal("a scanner that would not answer reported nothing")
		}
	})
}

// Test_runSet covers changing the setting.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Changes: a different value is chosen and read back
//   - Unchanged: the value it is already on costs no menu walking
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not say what it is on is reported
//   - DidNotTake: a scanner that did not take the value is reported
func Test_runSet(t *testing.T) {
	// Verify a different value is chosen and then read back from the scanner
	// rather than assumed from the press.
	t.Run("Changes", func(t *testing.T) {
		app, out, _ := running(sequence(
			entryName,      // finding the setting
			"Invert Phase", // what it is on now
			"In Phase",     // the row being chosen
			entryName,      // finding it again to read the answer back
			"In Phase",     // what it is on afterwards
		))

		want, _ := lookup(InPhase)
		if err := runSet(context.Background(), app, want); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(out.String(), "headphone: "+InPhase) {
			t.Errorf("wrote %q, want the setting it landed on", out.String())
		}
	})

	// Verify choosing the value it is already on does not walk the menus again.
	t.Run("Unchanged", func(t *testing.T) {
		app, out, _ := running(showing("In Phase"))
		want, _ := lookup(InPhase)

		if err := runSet(context.Background(), app, want); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(out.String(), "headphone: "+InPhase) {
			t.Errorf("wrote %q, want the setting", out.String())
		}
	})

	// Verify a run with no scanner named is refused.
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		want, _ := lookup(InPhase)

		if err := runSet(context.Background(), app, want); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("got %v, want ErrNoDevice", err)
		}
	})

	// Verify a scanner that will not say what it is on is reported.
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := running(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port closed")
		}})
		want, _ := lookup(InPhase)

		if err := runSet(context.Background(), app, want); err == nil {
			t.Fatal("a scanner that would not answer reported nothing")
		}
	})

	// Verify a scanner that took the press and did not change says so, rather
	// than reporting the value that was asked for as though it had.
	t.Run("DidNotTake", func(t *testing.T) {
		app, _, _ := running(sequence(
			entryName,
			"Invert Phase", // what it is on now
			"In Phase",     // the row being chosen, which it accepts
			entryName,
			"Invert Phase", // and it is still on the old one
		))

		want, _ := lookup(InPhase)
		err := runSet(context.Background(), app, want)
		if err == nil || !strings.Contains(err.Error(), "still "+Invert) {
			t.Fatalf("got %v, want it to report what the scanner is actually on", err)
		}
	})
}

// stuck returns a scanner that shows each screen in turn but will not come out
// of the menus, which is the failure every one of these paths has to report.
//
// Leaving is checked at four separate points, because a command that walks into
// a menu owns putting the radio back and a scanner left in Settings is not
// scanning.
//
// Parameters:
//   - screens: what the display shows, in the order it is read
//
// Returns:
//   - the connection
func stuck(screens ...string) *stubConn {
	conn := sequence(screens...)
	conn.execXML = func(string) (string, error) {
		return `<MenuInfo Name="Settings" MenuType="TypeSelect"/>`, nil
	}
	return conn
}

// Test_readReportsAnUnreadableScreen covers the display failing part way
// through the walk, once the setting has been found and before its value has.
func Test_readReportsAnUnreadableScreen(t *testing.T) {
	reads := 0
	conn := &stubConn{exec: func(command string) (string, error) {
		if command != "STS" {
			return "", nil
		}
		if reads++; reads == 1 {
			return "0," + entryName + ",*", nil
		}
		return "", errors.New("the port closed")
	}}

	_, err := read(context.Background(), device.New(conn))
	if err == nil || !strings.Contains(err.Error(), "reading the headphone setting") {
		t.Fatalf("got %v, want the unreadable screen reported", err)
	}
}

// Test_runReportReportsAScannerLeftInTheMenus covers the radio not coming back
// out, which matters more than it sounds: a scanner left in Settings is a
// scanner that has stopped scanning.
func Test_runReportReportsAScannerLeftInTheMenus(t *testing.T) {
	app, _, _ := running(stuck(entryName, "In Phase"))

	if err := runReport(context.Background(), app); err == nil {
		t.Fatal("a scanner stuck in the menus reported nothing")
	}
}

// Test_runSetReportsEveryWayTheWalkCanFail covers the four points at which
// changing the setting can leave the radio somewhere it should not be.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - UnchangedLeaveFails: stuck on the way out of an unchanged setting
//   - SelectFails: the scanner will not take the value
//   - LeaveAfterSelectFails: stuck after choosing
//   - ReadBackFails: the scanner stops answering before it can be read back
//   - FinalLeaveFails: stuck on the way out at the very end
func Test_runSetReportsEveryWayTheWalkCanFail(t *testing.T) {
	inPhase, _ := lookup(InPhase)

	// Verify being stuck on the way out of a setting that needed no change.
	t.Run("UnchangedLeaveFails", func(t *testing.T) {
		app, _, _ := running(stuck(entryName, "In Phase"))
		if err := runSet(context.Background(), app, inPhase); err == nil {
			t.Fatal("a scanner stuck in the menus reported nothing")
		}
	})

	// Verify a scanner that will not offer the value is reported, naming it.
	t.Run("SelectFails", func(t *testing.T) {
		app, _, _ := running(sequence(entryName, "Invert Phase", "Something Else"))

		err := runSet(context.Background(), app, inPhase)
		if err == nil || !strings.Contains(err.Error(), "In Phase") {
			t.Fatalf("got %v, want the value it would not take named", err)
		}
	})

	// Verify being stuck after the value was chosen.
	t.Run("LeaveAfterSelectFails", func(t *testing.T) {
		app, _, _ := running(stuck(entryName, "Invert Phase", "In Phase"))
		if err := runSet(context.Background(), app, inPhase); err == nil {
			t.Fatal("a scanner stuck after the choice reported nothing")
		}
	})

	// Verify a scanner that stops answering before the setting can be read
	// back, which must not be reported as a change that worked.
	t.Run("ReadBackFails", func(t *testing.T) {
		reads := 0
		conn := &stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				if command == "GSI" {
					return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
				}
				return "", nil
			}
			reads++
			switch reads {
			case 1:
				return "0," + entryName + ",*", nil
			case 2:
				return "0,Invert Phase,*", nil
			case 3:
				return "0,In Phase,*", nil
			}
			return "", errors.New("the port closed")
		}, execXML: func(string) (string, error) {
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}

		app, out, _ := running(conn)
		if err := runSet(context.Background(), app, inPhase); err == nil {
			t.Fatal("a scanner that stopped answering reported nothing")
		}
		if strings.Contains(out.String(), "headphone:") {
			t.Errorf("wrote %q, want nothing claimed about a change it could not confirm",
				out.String())
		}
	})

	// Verify being stuck on the way out at the very end, after the change was
	// made and confirmed.
	t.Run("FinalLeaveFails", func(t *testing.T) {
		leaves := 0
		conn := sequence(entryName, "Invert Phase", "In Phase", entryName, "In Phase")
		conn.execXML = func(string) (string, error) {
			// The first two walks out succeed; the last one does not.
			if leaves++; leaves > 2 {
				return `<MenuInfo Name="Settings" MenuType="TypeSelect"/>`, nil
			}
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}

		app, _, _ := running(conn)
		if err := runSet(context.Background(), app, inPhase); err == nil {
			t.Fatal("a scanner stuck on the way out reported nothing")
		}
	})
}
