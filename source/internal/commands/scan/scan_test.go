// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package scan

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
//   - Wiring: the command is named and carries its help
//   - Runs: executing the command returns a scanner to scanning
func TestNew(t *testing.T) {
	// Verify the command carries the name and the help the tool wires on
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "scan" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "scan")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify running the command puts a scanner that is out of the menus back
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes
		app.SetDevice(device.New(&stubConn{execXML: func(command string) (string, error) {
			if command == "GSI" {
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(notes.String(), "already out of the menus") {
			t.Errorf("the command wrote %q, wanted it to say the scanner was already out", notes.String())
		}
	})
}

// Test_held covers naming what the scanner is parked on.
//
// Coverage: 100% (3 test cases covering both branches)
//
// Test cases:
//   - FullPath: the system, department and channel are joined
//   - PartOfThePath: only the names the scanner gave are used
//   - Nothing: a scanner that named nothing falls back to its mode
func Test_held(t *testing.T) {
	// Verify the whole path is joined in the order the scanner reports it
	t.Run("FullPath", func(t *testing.T) {
		info := device.ScannerInfo{
			Mode:       "Scan Hold",
			System:     device.Named{Name: "Statewide"},
			Department: device.Named{Name: "Fire"},
			Channel:    device.Named{Name: "Dispatch"},
		}

		if got, want := held(info), "Statewide / Fire / Dispatch"; got != want {
			t.Errorf("held is %q, wanted %q", got, want)
		}
	})

	// Verify blank names are left out rather than joined as empty steps
	t.Run("PartOfThePath", func(t *testing.T) {
		info := device.ScannerInfo{
			Mode:    "Scan Hold",
			System:  device.Named{Name: "  "},
			Channel: device.Named{Name: "Dispatch"},
		}

		if got, want := held(info), "Dispatch"; got != want {
			t.Errorf("held is %q, wanted %q", got, want)
		}
	})

	// Verify a scanner that named nothing is described by its mode instead
	t.Run("Nothing", func(t *testing.T) {
		info := device.ScannerInfo{Mode: "Scan Hold"}

		if got, want := held(info), "one channel, in Scan Hold"; got != want {
			t.Errorf("held is %q, wanted %q", got, want)
		}
	})
}

// Test_inMenu covers asking whether the scanner is showing a menu.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - InMenu: a scanner showing a menu says so
//   - NotInMenu: a scanner out of the menus is a normal answer, not a failure
//   - Fails: a scanner that will not answer at all is reported
func Test_inMenu(t *testing.T) {
	// Verify a scanner showing a menu is reported as being in one
	t.Run("InMenu", func(t *testing.T) {
		client := device.New(&stubConn{execXML: func(string) (string, error) {
			return `<MenuInfo Name="Menu" MenuType="TypeSelect"/>`, nil
		}})

		got, err := inMenu(context.Background(), client)
		if err != nil {
			t.Fatalf("inMenu: %v", err)
		}
		if !got {
			t.Error("inMenu reported the scanner out of the menus, wanted it in one")
		}
	})

	// Verify a scanner out of the menus answers false without an error
	t.Run("NotInMenu", func(t *testing.T) {
		client := device.New(&stubConn{execXML: func(string) (string, error) {
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}})

		got, err := inMenu(context.Background(), client)
		if err != nil {
			t.Fatalf("inMenu: %v", err)
		}
		if got {
			t.Error("inMenu reported the scanner in a menu, wanted it out")
		}
	})

	// Verify a scanner that will not answer is reported
	t.Run("Fails", func(t *testing.T) {
		client := device.New(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := inMenu(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "asking the scanner where it is") {
			t.Errorf("inMenu reported %v, wanted the failed read", err)
		}
	})
}

// Test_leaveMode covers bringing the scanner back from a mode it was put in.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Success: a scanner in a custom search is brought back and the mode named
//   - AlreadyScanning: a scanner already scanning is left alone
//   - NoMode: a scanner reporting no mode at all is left alone
//   - InfoFails: a scanner that will not say what it is doing is reported
//   - JumpFails: a scanner that refuses to scan is reported
//   - Cancelled: a cancelled context stops the waiting
//   - GivesUp: a scanner that stays in the mode is reported after the last look
func Test_leaveMode(t *testing.T) {
	// Verify a scanner sweeping its own search is brought back, and named
	t.Run("Success", func(t *testing.T) {
		reads := 0
		conn := &stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			if reads == 1 {
				return `<Info Mode="Custom Search" V_Screen="search"/>`, nil
			}
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}}

		got, err := leaveMode(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("leaveMode: %v", err)
		}
		if got != "Custom Search" {
			t.Errorf("leaveMode named %q, wanted the custom search", got)
		}
	})

	// Verify a scanner that is already scanning is left entirely alone
	t.Run("AlreadyScanning", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}}

		got, err := leaveMode(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("leaveMode: %v", err)
		}
		if got != "" {
			t.Errorf("leaveMode named %q, wanted nothing to have been done", got)
		}
		for _, command := range conn.sent {
			if strings.HasPrefix(command, "JPM") {
				t.Error("a scanner that was already scanning was told to scan")
			}
		}
	})

	// Verify a scanner reporting no mode at all is not acted on
	t.Run("NoMode", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return `<Info Mode="  " V_Screen="scan"/>`, nil
		}}

		got, err := leaveMode(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("leaveMode: %v", err)
		}
		if got != "" {
			t.Errorf("leaveMode named %q, wanted nothing to have been done", got)
		}
		for _, command := range conn.sent {
			if strings.HasPrefix(command, "JPM") {
				t.Error("a scanner reporting no mode was told to scan")
			}
		}
	})

	// Verify a scanner that will not say what it is doing is reported
	t.Run("InfoFails", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := leaveMode(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "asking the scanner what it is doing") {
			t.Errorf("leaveMode reported %v, wanted the failed read", err)
		}
	})

	// Verify a scanner that refuses to go back to scanning is reported
	t.Run("JumpFails", func(t *testing.T) {
		conn := &stubConn{
			exec: func(command string) (string, error) {
				return "", errors.New("the scanner refused the jump")
			},
			execXML: func(string) (string, error) {
				return `<Info Mode="Custom Search" V_Screen="search"/>`, nil
			},
		}

		_, err := leaveMode(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "returning the scanner to scanning from Custom Search") {
			t.Errorf("leaveMode reported %v, wanted the refused jump", err)
		}
	})

	// Verify a cancelled context stops the waiting rather than looking on
	t.Run("Cancelled", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return `<Info Mode="Custom Search" V_Screen="search"/>`, nil
		}}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := leaveMode(ctx, device.New(conn))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("leaveMode reported %v, wanted the cancelled context", err)
		}
	})

	// Verify a scanner that stays in the mode is reported after the last look
	t.Run("GivesUp", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return `<Info Mode="Custom Search" V_Screen="search"/>`, nil
		}}

		_, err := leaveMode(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "the scanner is still in Custom Search") {
			t.Errorf("leaveMode reported %v, wanted the mode it would not leave", err)
		}
	})
}

// Test_leaveWeather covers taking the scanner off the weather channels.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Success: the scanner is told to scan and confirms it left
//   - JumpFails: a scanner that refuses to scan is reported
//   - Cancelled: a cancelled context stops the waiting
//   - GivesUp: a scanner that stays on the channels is reported
func Test_leaveWeather(t *testing.T) {
	// Verify the scanner is told to scan and read back until it agrees
	t.Run("Success", func(t *testing.T) {
		reads := 0
		conn := &stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			if reads == 1 {
				return `<Info Mode="WX Scan"><WxMode Mode="Monitor Weather"/></Info>`, nil
			}
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}}

		if err := leaveWeather(context.Background(), device.New(conn)); err != nil {
			t.Fatalf("leaveWeather: %v", err)
		}

		jumped := false
		for _, command := range conn.sent {
			if command == "JPM,SCN_MODE," {
				jumped = true
			}
		}
		if !jumped {
			t.Errorf("the scanner was sent %v, wanted the jump to scanning", conn.sent)
		}
	})

	// Verify a scanner that refuses to go back to scanning is reported
	t.Run("JumpFails", func(t *testing.T) {
		conn := &stubConn{
			exec: func(string) (string, error) {
				return "", errors.New("the scanner refused the jump")
			},
			execXML: func(string) (string, error) {
				return `<Info Mode="WX Scan"><WxMode Mode="Monitor Weather"/></Info>`, nil
			},
		}

		err := leaveWeather(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "from the weather channels") {
			t.Errorf("leaveWeather reported %v, wanted the refused jump", err)
		}
	})

	// Verify a cancelled context stops the waiting rather than looking on
	t.Run("Cancelled", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return `<Info Mode="WX Scan"><WxMode Mode="Monitor Weather"/></Info>`, nil
		}}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := leaveWeather(ctx, device.New(conn))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("leaveWeather reported %v, wanted the cancelled context", err)
		}
	})

	// Verify a scanner that stays on the weather channels is reported
	t.Run("GivesUp", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return `<Info Mode="WX Scan"><WxMode Mode="Monitor Weather"/></Info>`, nil
		}}

		err := leaveWeather(context.Background(), device.New(conn))
		if err == nil || !strings.Contains(err.Error(), "still on the weather channels") {
			t.Errorf("leaveWeather reported %v, wanted the channels it would not leave", err)
		}
	})
}

// Test_presses covers counting the key presses for the message.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Counts: one press reads as words and more than one as a number
func Test_presses(t *testing.T) {
	// Verify one press is spelled out and any other count is a number
	t.Run("Counts", func(t *testing.T) {
		if got, want := presses(1), "one key press"; got != want {
			t.Errorf("presses(1) is %q, wanted %q", got, want)
		}
		if got, want := presses(3), "3 key presses"; got != want {
			t.Errorf("presses(3) is %q, wanted %q", got, want)
		}
		if got, want := presses(0), "0 key presses"; got != want {
			t.Errorf("presses(0) is %q, wanted %q", got, want)
		}
	})
}

// Test_resume covers returning the scanner to scanning and saying what it did.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - AlreadyScanning: a scanner already scanning is left alone quietly
//   - InfoFails: a scanner that will not answer is noted, not failed
//   - Weather: a scanner on the weather channels is taken off them
//   - WeatherFails: a scanner that will not leave the channels is reported
//   - QuickSearch: a scanner in quick search is released and said so
//   - HoldWithName: a held scanner names what it was parked on
//   - HoldWithoutName: a hold found only on the second look is still reported
//   - LeftMode: a scanner brought back from a mode says which one
func Test_resume(t *testing.T) {
	// Verify a scanner that is already scanning is left alone
	t.Run("AlreadyScanning", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		client := device.New(&stubConn{execXML: func(string) (string, error) {
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}})

		if err := resume(context.Background(), app, client); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if notes.Len() != 0 {
			t.Errorf("resume wrote %q, wanted nothing said about a scanner already scanning", notes.String())
		}
	})

	// Verify a scanner that will not answer is noted rather than failed
	t.Run("InfoFails", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		client := device.New(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if err := resume(context.Background(), app, client); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if !strings.Contains(notes.String(), "Could not check what the scanner is doing") {
			t.Errorf("resume wrote %q, wanted the note about not checking", notes.String())
		}
	})

	// Verify a scanner on the weather channels is taken off them and says so
	t.Run("Weather", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		reads := 0
		client := device.New(&stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			if reads == 1 {
				return `<Info Mode="WX Scan"><WxMode Mode="Monitor Weather"/></Info>`, nil
			}
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}})

		if err := resume(context.Background(), app, client); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if !strings.Contains(notes.String(), "left the weather channels") {
			t.Errorf("resume wrote %q, wanted it to say the channels were left", notes.String())
		}
	})

	// Verify a scanner that will not leave the weather channels is reported
	t.Run("WeatherFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		client := device.New(&stubConn{
			exec: func(string) (string, error) {
				return "", errors.New("the scanner refused the jump")
			},
			execXML: func(string) (string, error) {
				return `<Info Mode="WX Scan"><WxMode Mode="Monitor Weather"/></Info>`, nil
			},
		})

		err := resume(context.Background(), app, client)
		if err == nil || !strings.Contains(err.Error(), "from the weather channels") {
			t.Errorf("resume reported %v, wanted the channels it could not leave", err)
		}
	})

	// Verify a scanner holding one frequency in quick search is released
	t.Run("QuickSearch", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		reads := 0
		client := device.New(&stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			if reads <= 2 {
				return `<Info Mode="Quick Search Hold" V_Screen="quick_search"/>`, nil
			}
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}})

		if err := resume(context.Background(), app, client); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if !strings.Contains(notes.String(), "left quick search") {
			t.Errorf("resume wrote %q, wanted it to say quick search was left", notes.String())
		}
	})

	// Verify a held scanner names what it was parked on before releasing it
	t.Run("HoldWithName", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		reads := 0
		client := device.New(&stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			if reads <= 2 {
				return `<Info Mode="Scan Hold" V_Screen="scan_hold">` +
					`<Channel Name="Dispatch"/></Info>`, nil
			}
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}})

		if err := resume(context.Background(), app, client); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if !strings.Contains(notes.String(), "holding Dispatch") {
			t.Errorf("resume wrote %q, wanted it to name what was held", notes.String())
		}
		if !strings.Contains(notes.String(), "been released") {
			t.Errorf("resume wrote %q, wanted it to say the hold was released", notes.String())
		}
	})

	// Verify a hold that only appears on the second look is still reported
	t.Run("HoldWithoutName", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		reads := 0
		client := device.New(&stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			switch reads {
			case 1:
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			case 2:
				return `<Info Mode="Scan Hold" V_Screen="scan_hold"/>`, nil
			}
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}})

		if err := resume(context.Background(), app, client); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if strings.Contains(notes.String(), "is holding") {
			t.Errorf("resume wrote %q, wanted no claim about what was held", notes.String())
		}
		if !strings.Contains(notes.String(), "been released") {
			t.Errorf("resume wrote %q, wanted it to say the hold was released", notes.String())
		}
	})

	// Verify a scanner that stops answering while being released is reported
	t.Run("ResumeFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		reads := 0
		client := device.New(&stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			if reads == 1 {
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			return "", errors.New("the port is gone")
		}})

		err := resume(context.Background(), app, client)
		if err == nil || !strings.Contains(err.Error(), "asking the scanner what it is doing") {
			t.Errorf("resume reported %v, wanted the failed read", err)
		}
	})

	// Verify a scanner that stops answering at the last question is reported
	t.Run("LeaveModeFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		reads := 0
		client := device.New(&stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			if reads <= 2 {
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			return "", errors.New("the port is gone")
		}})

		err := resume(context.Background(), app, client)
		if err == nil || !strings.Contains(err.Error(), "asking the scanner what it is doing") {
			t.Errorf("resume reported %v, wanted the failed read", err)
		}
	})

	// Verify a scanner brought back from a mode of its own says which one
	t.Run("LeftMode", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		reads := 0
		client := device.New(&stubConn{execXML: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			reads++
			switch reads {
			case 1, 2:
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			case 3:
				return `<Info Mode="Custom Search" V_Screen="search"/>`, nil
			}
			return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
		}})

		if err := resume(context.Background(), app, client); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if !strings.Contains(notes.String(), "left Custom Search") {
			t.Errorf("resume wrote %q, wanted it to name the mode it left", notes.String())
		}
	})
}

// Test_run covers returning the scanner to its operating screen.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - NoDevice: a run with no scanner named is refused
//   - NotInMenu: a scanner already out of the menus is said to be
//   - InMenuGracefully: a scanner that closes its own menu says so
//   - InMenuAfterPresses: a scanner that needed keys says how many
//   - AsksFails: a scanner that will not say where it is is reported
//   - LeaveFails: a scanner that will not come out of the menus is reported
func Test_run(t *testing.T) {
	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := run(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("run reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner already out of the menus is reported as such
	t.Run("NotInMenu", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes
		app.SetDevice(device.New(&stubConn{execXML: func(command string) (string, error) {
			if command == "GSI" {
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(notes.String(), "already out of the menus") {
			t.Errorf("run wrote %q, wanted it to say the scanner was already out", notes.String())
		}
	})

	// Verify a scanner that closes its own menu is reported without a count
	t.Run("InMenuGracefully", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		msi := 0
		app.SetDevice(device.New(&stubConn{execXML: func(command string) (string, error) {
			if command == "GSI" {
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			msi++
			if msi == 1 {
				return `<MenuInfo Name="Menu" MenuType="TypeSelect"/>`, nil
			}
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(notes.String(), "has left the menus.") {
			t.Errorf("run wrote %q, wanted it to say the menus were left", notes.String())
		}
		if strings.Contains(notes.String(), "key press") {
			t.Errorf("run wrote %q, wanted no count for a menu that closed itself", notes.String())
		}
	})

	// Verify a scanner that needed key presses is reported with the count
	t.Run("InMenuAfterPresses", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		msi := 0
		app.SetDevice(device.New(&stubConn{execXML: func(command string) (string, error) {
			if command == "GSI" {
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			msi++
			if msi <= 3 {
				return `<MenuInfo Name="Menu" MenuType="TypeSelect"/>`, nil
			}
			return `<MenuInfo MenuType="TypeError"/>`, nil
		}}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(notes.String(), "one key press") {
			t.Errorf("run wrote %q, wanted it to count the key press", notes.String())
		}
	})

	// Verify a scanner that will not say where it is is reported
	t.Run("AsksFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := run(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "asking the scanner where it is") {
			t.Errorf("run reported %v, wanted the failed read", err)
		}
	})

	// Verify a scanner that will not come out of the menus is reported
	t.Run("LeaveFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{execXML: func(command string) (string, error) {
			if command == "GSI" {
				return `<Info Mode="Scan Mode" V_Screen="scan"/>`, nil
			}
			return `<MenuInfo Name="Menu" MenuType="TypeSelect"/>`, nil
		}}))

		err := run(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "could not leave the menus") {
			t.Errorf("run reported %v, wanted the scanner stuck in the menus", err)
		}
	})
}
