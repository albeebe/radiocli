// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package location

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

// stubConn is a fake device.Conn that answers the exchanges this command
// makes, so it can be driven with no scanner attached.
//
// Screens are answered in order, one per read of the display, each holding a
// single line that is highlighted. An empty name fails that read instead,
// which is how a test puts a failure at one step of a walk without waiting for
// any of the menus package's poll loops to give up.
//
// Values are the answers to everything else sent through Execute, queued per
// command, and the last one in a queue stays in place once the rest are used
// up. A command with no value of its own is acknowledged the way the scanner
// acknowledges a key press.
//
// Documents are answered the same way, and a command with no document of its
// own is answered the way a scanner that is out of the menus and scanning
// answers it, which is what makes leaving the menus cost nothing here.
type stubConn struct {
	screens   []string            // The name highlighted on each successive display read, "" to fail that read
	reads     int                 // How many display reads have been answered so far
	values    map[string][]string // Answers to Execute, in order, keyed by the command asking for them
	docs      map[string][]string // Documents to answer with, in order, keyed by the command asking for them
	fail      map[string]error    // Commands that fail instead of answering
	failFrom  map[string]int      // Commands that start failing at the numbered send, counting from one
	failUntil map[string]int      // Commands that answer as a scanner still busy does, up to the numbered send
	sent      []string            // Every command sent to the scanner, in order
}

// Info describes the fake scanner, which nothing here inspects.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute answers a display read with the next screen, and everything else
// with the value the test supplied, or with the empty acknowledgement the
// scanner gives a key it accepted.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) {
	c.sent = append(c.sent, command)
	if err := c.fail[command]; err != nil {
		return "", err
	}

	// A command sent several times over one walk is the same string each time,
	// so a test says which of the sends is the one that fails.
	if from, ok := c.failFrom[command]; ok && c.count(command) >= from {
		return "", errors.New("the scanner refused the key")
	}

	// A scanner busy with a position it was just given answers nothing at all,
	// which arrives here as the deadline passing.
	if until, ok := c.failUntil[command]; ok && c.count(command) <= until {
		return "", context.DeadlineExceeded
	}

	if command == "STS" {
		if c.reads >= len(c.screens) {
			return "", errors.New("the scanner has no screen left to show")
		}
		name := c.screens[c.reads]
		c.reads++

		// An empty name stands for a read the scanner refuses.
		if name == "" {
			return "", errors.New("the screen cannot be read")
		}

		// One line, in the small font, holding the name and marked highlighted.
		return "0," + name + ",*", nil
	}

	if value, ok := c.next(c.values, command); ok {
		return value, nil
	}
	return "", nil
}

// ExecuteXML answers a menu or a status read with the document the test
// supplied, falling back to a scanner that is out of the menus and scanning.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	c.sent = append(c.sent, command)
	if err := c.fail[command]; err != nil {
		return "", err
	}

	if doc, ok := c.next(c.docs, command); ok {
		if doc == "" {
			return "", errors.New("the scanner refused the document")
		}
		return doc, nil
	}

	switch command {
	case "MSI":
		return `<MenuInfo MenuType="TypeError"/>`, nil
	case "GSI":
		return `<ScannerInfo Mode="Scan" V_Screen="scan"/>`, nil
	}
	return "", errors.New("unexpected command: " + command)
}

// Send is unused by this command and always succeeds.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close releases nothing, because there is no port.
func (c *stubConn) Close() error { return nil }

// count reports how many times a command has been sent, including the one
// being answered now.
func (c *stubConn) count(command string) int {
	seen := 0
	for _, sent := range c.sent {
		if sent == command {
			seen++
		}
	}
	return seen
}

// next takes the answer a command is due, leaving the last one in place so a
// command sent more times than the test scripted keeps getting it.
func (c *stubConn) next(from map[string][]string, command string) (string, bool) {
	answers, ok := from[command]
	if !ok || len(answers) == 0 {
		return "", false
	}
	if len(answers) == 1 {
		return answers[0], true
	}
	from[command] = answers[1:]
	return answers[0], true
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its subcommands
//   - Runs: running the command reports the position
func TestNew(t *testing.T) {
	// Verify that the command is named and carries every subcommand
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "location" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "location")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		for _, want := range []string{"set", "gps"} {
			found := false
			for _, sub := range cmd.Commands() {
				if sub.Name() == want {
					found = true
				}
			}
			if !found {
				t.Errorf("the command has no %q subcommand", want)
			}
		}
	})

	// Verify that running the command reads the position and writes it out
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(&stubConn{values: map[string][]string{
			"LCR": {"38.420800,-79.797200,10.0"},
		}}))

		cmd := New(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "38.420800") {
			t.Errorf("expected the position on the output, got: %q", out.String())
		}
	})
}

// Test_newGPS tests the newGPS function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its three flags
//   - Runs: running the subcommand reaches the scanner
func Test_newGPS(t *testing.T) {
	// Verify that the subcommand is named and carries the flags it offers
	t.Run("Wiring", func(t *testing.T) {
		cmd := newGPS(appcontext.New())

		if cmd.Name() != "gps" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "gps")
		}
		for _, flag := range []string{"wait", "off", "status"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("the subcommand has no --%s flag", flag)
			}
		}
	})

	// Verify that running the subcommand asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newGPS(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, nil); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_newSet tests the newSet function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its two flags
//   - NoArgument: nothing named at all is refused before the scanner is asked
//   - WithZip: a zip code given as the argument is carried through
func Test_newSet(t *testing.T) {
	// Verify that the subcommand is named and carries the flags it offers
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSet(appcontext.New())

		if cmd.Name() != "set" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "set")
		}
		for _, flag := range []string{"range", "position"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("the subcommand has no --%s flag", flag)
			}
		}
	})

	// Verify that naming neither a zip code nor a position is refused
	t.Run("NoArgument", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newSet(app)
		cmd.SetContext(context.Background())

		err := cmd.RunE(cmd, nil)
		if err == nil {
			t.Fatal("expected an error when nothing was named, got none")
		}
		if !strings.Contains(err.Error(), "name a zip code") {
			t.Errorf("expected the message to ask for a zip code, got: %v", err)
		}
	})

	// Verify that a zip code given as the argument is carried through to the scanner
	t.Run("WithZip", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newSet(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"12345"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_answerPrompts tests the answerPrompts function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - ScreenError: a screen that cannot be read is reported
//   - OutOfRange: a zip code the scanner does not hold is reported
//   - FullDatabase: the database prompt is answered and the walk carries on
//   - ConfirmTimeout: an acknowledgement that times out is not a failure
//   - ConfirmError: a prompt that cannot be answered is reported
//   - Rounds: a scanner that keeps prompting is left after the last round
func Test_answerPrompts(t *testing.T) {
	// Builds an app writing to buffers.
	appWith := func() (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout, app.Stderr = &bytes.Buffer{}, notes
		return app, notes
	}

	// Verify that a screen that cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		app, _ := appWith()
		conn := &stubConn{screens: []string{""}}

		err := answerPrompts(context.Background(), app, device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the screen cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the screen after entering the zip code") {
			t.Errorf("expected the message to say it was reading the screen, got: %v", err)
		}
	})

	// Verify that a zip code the scanner does not hold is reported as that
	t.Run("OutOfRange", func(t *testing.T) {
		app, _ := appWith()
		conn := &stubConn{screens: []string{outOfRange}}

		err := answerPrompts(context.Background(), app, device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the zip code is out of range, got none")
		}
		if !strings.Contains(err.Error(), "does not hold zip code 12345") {
			t.Errorf("expected the message to name the zip code, got: %v", err)
		}
	})

	// Verify that the full database prompt is answered and said so on stderr
	t.Run("FullDatabase", func(t *testing.T) {
		app, notes := appWith()
		conn := &stubConn{screens: []string{turnOnFullDatabase, "Scanning", "Scanning"}}

		if err := answerPrompts(context.Background(), app, device.New(conn), "12345"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "Full Database") {
			t.Errorf("expected a note about the full database, got: %q", notes.String())
		}
	})

	// Verify that an acknowledgement the scanner is too busy to give is not a failure
	t.Run("ConfirmTimeout", func(t *testing.T) {
		app, _ := appWith()
		conn := &stubConn{
			screens: []string{turnOnFullDatabase, "Scanning", "Scanning"},
			fail:    map[string]error{"KEY,E,P": context.DeadlineExceeded},
		}

		if err := answerPrompts(context.Background(), app, device.New(conn), "12345"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	// Verify that a prompt the scanner refuses to take is reported
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith()
		conn := &stubConn{
			screens:  []string{turnOnFullDatabase},
			failFrom: map[string]int{"KEY,E,P": 1},
		}

		if err := answerPrompts(context.Background(), app, device.New(conn), "12345"); err == nil {
			t.Error("expected an error when the prompt cannot be answered, got none")
		}
	})

	// Verify that a scanner that stops answering while it rebuilds is reported
	t.Run("AwakenError", func(t *testing.T) {
		app, _ := appWith()
		conn := &stubConn{screens: []string{turnOnFullDatabase}}

		err := answerPrompts(context.Background(), app, device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the scanner stops answering, got none")
		}
		if !strings.Contains(err.Error(), "stopped answering") {
			t.Errorf("expected the message to say the scanner stopped answering, got: %v", err)
		}
	})

	// Verify that a scanner still prompting after the last round is left as it is
	t.Run("Rounds", func(t *testing.T) {
		app, _ := appWith()

		screens := make([]string, 0, promptRounds*2)
		for i := 0; i < promptRounds*2; i++ {
			screens = append(screens, turnOnFullDatabase)
		}
		conn := &stubConn{screens: screens}

		if err := answerPrompts(context.Background(), app, device.New(conn), "12345"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

// Test_awaitFix tests the awaitFix function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Moved: a position that has moved is reported as the receiver's own
//   - Cancelled: a read refused while the context is over is reported
//   - Refused: reads the scanner refuses are waited through and then given up on
//   - Waits: a position that stays put waits, and stops when the context does
func Test_awaitFix(t *testing.T) {
	before := report{Latitude: 38.420800, Longitude: -79.797200, Range: 10}

	// Verify that a position that has moved is reported straight away
	t.Run("Moved", func(t *testing.T) {
		conn := &stubConn{values: map[string][]string{"LCR": {"38.500000,-79.500000,10.0"}}}

		latest, moved, err := awaitFix(context.Background(), device.New(conn), before)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !moved {
			t.Error("expected the position to have moved, it had not")
		}
		if latest.Latitude != 38.5 {
			t.Errorf("got latitude %v, wanted the one the receiver found", latest.Latitude)
		}
	})

	// Verify that a read refused after the context is over reports the context
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		conn := &stubConn{fail: map[string]error{"LCR": errors.New("the scanner is still settling")}}

		if _, _, err := awaitFix(ctx, device.New(conn), before); err == nil {
			t.Error("expected an error when the context is over, got none")
		}
	})

	// Verify that reads the scanner refuses are tried again and then given up on
	t.Run("Refused", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"LCR": errors.New("the scanner is still settling")}}

		latest, moved, err := awaitFix(context.Background(), device.New(conn), before)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if moved {
			t.Error("expected the position not to have moved, it had")
		}
		if latest != (report{}) {
			t.Errorf("expected nothing read, got: %v", latest)
		}
	})

	// Verify that a position that stays put waits, and gives up when the context does
	t.Run("Waits", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), fixInterval+200*time.Millisecond)
		defer cancel()
		conn := &stubConn{values: map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}}}

		if _, _, err := awaitFix(ctx, device.New(conn), before); err == nil {
			t.Error("expected an error when the context runs out, got none")
		}
	})
}

// Test_checkZip tests the checkZip function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Valid: five digits are accepted
//   - WrongLength: anything other than five characters is refused
//   - NotDigits: five characters that are not all digits are refused
func Test_checkZip(t *testing.T) {
	// Verify that five digits are accepted
	t.Run("Valid", func(t *testing.T) {
		if err := checkZip("12345"); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a zip code of the wrong length is refused before anything is touched
	t.Run("WrongLength", func(t *testing.T) {
		err := checkZip("0306")
		if err == nil {
			t.Fatal("expected an error for four digits, got none")
		}
		if !strings.Contains(err.Error(), "want five digits") {
			t.Errorf("expected the message to say what a zip code looks like, got: %v", err)
		}
	})

	// Verify that five characters that are not digits are refused
	t.Run("NotDigits", func(t *testing.T) {
		if err := checkZip("0306A"); err == nil {
			t.Error("expected an error for a letter in a zip code, got none")
		}
	})
}

// Test_confirmGPS tests the confirmGPS function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - ReadError: a setting that cannot be read is reported
//   - Mismatch: a setting holding the other value is reported
//   - Match: the setting holding what it was given is accepted
func Test_confirmGPS(t *testing.T) {
	// Verify that a setting that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		if err := confirmGPS(context.Background(), device.New(conn), gpsEnable); err == nil {
			t.Error("expected an error when the setting cannot be read, got none")
		}
	})

	// Verify that a setting still holding the other value is reported
	t.Run("Mismatch", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, setGPSFunction, gpsDisable}}

		err := confirmGPS(context.Background(), device.New(conn), gpsEnable)
		if err == nil {
			t.Fatal("expected an error when the setting did not take, got none")
		}
		if !strings.Contains(err.Error(), setGPSFunction) {
			t.Errorf("expected the message to name the setting, got: %v", err)
		}
	})

	// Verify that a setting holding what it was given is accepted
	t.Run("Match", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, setGPSFunction, gpsEnable}}

		if err := confirmGPS(context.Background(), device.New(conn), gpsEnable); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}

// Test_confirmPosition tests the confirmPosition function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Same: a position within the tolerance is the same place
//   - Latitude: a latitude further off than the tolerance is reported
//   - Longitude: a longitude further off than the tolerance is reported
func Test_confirmPosition(t *testing.T) {
	// Verify that a position within a rounding of the one written is accepted
	t.Run("Same", func(t *testing.T) {
		after := report{Latitude: 38.420801, Longitude: -79.797201}

		if err := confirmPosition(after, 38.420800, -79.797200); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a latitude somewhere else is reported as a different place
	t.Run("Latitude", func(t *testing.T) {
		after := report{Latitude: 43.000000, Longitude: -79.797200}

		err := confirmPosition(after, 38.420800, -79.797200)
		if err == nil {
			t.Fatal("expected an error when the scanner is somewhere else, got none")
		}
		if !strings.Contains(err.Error(), "rather than the") {
			t.Errorf("expected the message to compare the two positions, got: %v", err)
		}
	})

	// Verify that a longitude somewhere else is reported as a different place
	t.Run("Longitude", func(t *testing.T) {
		after := report{Latitude: 38.420800, Longitude: -72.000000}

		if err := confirmPosition(after, 38.420800, -79.797200); err == nil {
			t.Error("expected an error when the scanner is somewhere else, got none")
		}
	})
}

// Test_digitKey tests the digitKey function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Digit: a digit is turned into the key that types it
//   - NotDigit: anything else is refused
func Test_digitKey(t *testing.T) {
	// Verify that a digit becomes the key that types it
	t.Run("Digit", func(t *testing.T) {
		key, ok := digitKey('7')
		if !ok {
			t.Fatal("expected a digit to be accepted, it was not")
		}
		if key != device.Key("7") {
			t.Errorf("got key %q, wanted %q", key, "7")
		}
	})

	// Verify that anything that is not a digit is refused
	t.Run("NotDigit", func(t *testing.T) {
		if _, ok := digitKey('-'); ok {
			t.Error("expected a dash to be refused, it was accepted")
		}
	})
}

// Test_disableGPS tests the disableGPS function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - SetError: a setting that cannot be reached is reported
//   - ConfirmError: a setting that did not take is reported
//   - ReadError: a position that cannot be read afterwards is reported
//   - JSON: the position is encoded when JSON was asked for
//   - Text: the position and its range are written out otherwise
func Test_disableGPS(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The walk to the setting, twice: once to set it and once to read it back.
	walk := []string{gpsMenu, setGPSFunction, gpsDisable, gpsMenu, setGPSFunction, gpsDisable}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := disableGPS(context.Background(), app); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a setting that cannot be reached is reported
	t.Run("SetError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{""}})

		if err := disableGPS(context.Background(), app); err == nil {
			t.Error("expected an error when the setting cannot be reached, got none")
		}
	})

	// Verify that a setting that did not take is reported
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsDisable, gpsMenu, setGPSFunction, gpsEnable},
		})

		if err := disableGPS(context.Background(), app); err == nil {
			t.Error("expected an error when the setting did not take, got none")
		}
	})

	// Verify that a position that cannot be read afterwards is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			fail:    map[string]error{"LCR": errors.New("the port is gone")},
		})

		if err := disableGPS(context.Background(), app); err == nil {
			t.Error("expected an error when the position cannot be read, got none")
		}
	})

	// Verify that asking for JSON encodes the position rather than printing it
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})
		app.Config.Output = appcontext.OutputJSON

		if err := disableGPS(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var held report
		if err := json.Unmarshal(out.Bytes(), &held); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if held.Range != 10 {
			t.Errorf("expected the range in the JSON, got: %v", held)
		}
	})

	// Verify that the default output names the position and says it is fixed
	t.Run("Text", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,0.0"}},
		})

		if err := disableGPS(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for _, want := range []string{"fixed", "38.420800", "none set"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected the output to carry %q, got: %q", want, out.String())
			}
		}
	})

	// Verify that a range the scanner is holding is written in miles
	t.Run("TextRange", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := disableGPS(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "10.0 miles") {
			t.Errorf("expected the range in miles, got: %q", out.String())
		}
	})
}

// Test_parsePosition tests the parsePosition function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Valid: a latitude and a longitude separated by a comma are read
//   - Spaces: spaces around either half are forgiven
//   - NoComma: one number on its own is refused
//   - BadLatitude: a latitude that is not a number, or out of range, is refused
//   - BadLongitude: a longitude that is not a number, or out of range, is refused
func Test_parsePosition(t *testing.T) {
	// Verify that a pair as this command prints it is read back
	t.Run("Valid", func(t *testing.T) {
		lat, long, err := parsePosition("38.433056,-79.839722")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if lat != 38.433056 || long != -79.839722 {
			t.Errorf("got %v, %v, wanted the pair that was given", lat, long)
		}
	})

	// Verify that spaces around either half are forgiven
	t.Run("Spaces", func(t *testing.T) {
		lat, long, err := parsePosition("  38.4 , -79.8 ")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if lat != 38.4 || long != -79.8 {
			t.Errorf("got %v, %v, wanted the pair that was given", lat, long)
		}
	})

	// Verify that one number on its own is not a position
	t.Run("NoComma", func(t *testing.T) {
		_, _, err := parsePosition("38.4")
		if err == nil {
			t.Fatal("expected an error for a single number, got none")
		}
		if !strings.Contains(err.Error(), "is not a position") {
			t.Errorf("expected the message to say it is not a position, got: %v", err)
		}
	})

	// Verify that a latitude that is not degrees on the globe is refused
	t.Run("BadLatitude", func(t *testing.T) {
		if _, _, err := parsePosition("north,-79.8"); err == nil {
			t.Error("expected an error for a latitude that is not a number, got none")
		}
		if _, _, err := parsePosition("91,-79.8"); err == nil {
			t.Error("expected an error for a latitude past the pole, got none")
		}
		if _, _, err := parsePosition("-91,-79.8"); err == nil {
			t.Error("expected an error for a latitude past the pole, got none")
		}
	})

	// Verify that a longitude that is not degrees on the globe is refused
	t.Run("BadLongitude", func(t *testing.T) {
		if _, _, err := parsePosition("38.4,west"); err == nil {
			t.Error("expected an error for a longitude that is not a number, got none")
		}
		if _, _, err := parsePosition("38.4,181"); err == nil {
			t.Error("expected an error for a longitude past the meridian, got none")
		}
		if _, _, err := parsePosition("38.4,-181"); err == nil {
			t.Error("expected an error for a longitude past the meridian, got none")
		}
	})
}

// Test_read tests the read function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the position and its range are read
//   - ReadError: a failed read is reported
func Test_read(t *testing.T) {
	// Verify that the position and its range come back as the command renders them
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{values: map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}}}

		r, err := read(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if r.Latitude != 38.4208 || r.Longitude != -79.7972 || r.Range != 10 {
			t.Errorf("got %v, wanted the position the scanner reported", r)
		}
	})

	// Verify that a failed read is reported rather than read as a position
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"LCR": errors.New("the port is gone")}}

		_, err := read(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the position cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the location") {
			t.Errorf("expected the message to say it was reading the location, got: %v", err)
		}
	})
}

// Test_readGPS tests the readGPS function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - WalkError: a setting that cannot be reached is reported
//   - HighlightedError: a screen that cannot be read is reported
//   - LeaveError: a failure returning to scanning is reported
//   - Success: the value the setting holds is reported
func Test_readGPS(t *testing.T) {
	// Verify that a setting that cannot be reached is reported
	t.Run("WalkError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		if _, err := readGPS(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the setting cannot be reached, got none")
		}
	})

	// Verify that a screen that cannot be read is reported
	t.Run("HighlightedError", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, setGPSFunction, ""}}

		err := func() error {
			_, err := readGPS(context.Background(), device.New(conn))
			return err
		}()
		if err == nil {
			t.Fatal("expected an error when the setting cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the GPS setting") {
			t.Errorf("expected the message to say it was reading the setting, got: %v", err)
		}
	})

	// Verify that a failure returning the scanner to scanning is reported
	t.Run("LeaveError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsEnable},
			fail:    map[string]error{"GSI": errors.New("the port is gone")},
		}

		if _, err := readGPS(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that the value in force is the one the screen highlights
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, setGPSFunction, gpsEnable}}

		shown, err := readGPS(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if shown != gpsEnable {
			t.Errorf("got %q, wanted %q", shown, gpsEnable)
		}
	})
}

// Test_reportGPS tests the reportGPS function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ReadGPSError: a setting that cannot be read is reported
//   - ReadError: a position that cannot be read is reported
//   - JSON: the setting and the position are encoded when JSON was asked for
//   - On: a scanner following its receiver reads as on
//   - Off: a scanner holding a position reads as off, with no range set
func Test_reportGPS(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := reportGPS(context.Background(), app); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a setting that cannot be read is reported
	t.Run("ReadGPSError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{""}})

		if err := reportGPS(context.Background(), app); err == nil {
			t.Error("expected an error when the setting cannot be read, got none")
		}
	})

	// Verify that a position that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsEnable},
			fail:    map[string]error{"LCR": errors.New("the port is gone")},
		})

		if err := reportGPS(context.Background(), app); err == nil {
			t.Error("expected an error when the position cannot be read, got none")
		}
	})

	// Verify that asking for JSON encodes the setting alongside the position
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsEnable},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})
		app.Config.Output = appcontext.OutputJSON

		if err := reportGPS(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var found struct {
			GPS   bool    `json:"gps"`
			Range float64 `json:"range"`
		}
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if !found.GPS || found.Range != 10 {
			t.Errorf("expected the setting and the range in the JSON, got: %v", found)
		}
	})

	// Verify that a scanner following its receiver is reported as on
	t.Run("On", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsEnable},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := reportGPS(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for _, want := range []string{"gps:       on", "10.0 miles"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected the output to carry %q, got: %q", want, out.String())
			}
		}
	})

	// Verify that a scanner holding a position is reported as off, with no range
	t.Run("Off", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsDisable},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,0.0"}},
		})

		if err := reportGPS(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for _, want := range []string{"gps:       off", "none set"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected the output to carry %q, got: %q", want, out.String())
			}
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ReadError: a position that cannot be read is reported
//   - JSON: the position is encoded when JSON was asked for
//   - NoRange: a scanner following its GPS reports no range rather than zero
//   - Text: the position and its range are written out otherwise
func Test_run(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := run(context.Background(), app); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a position that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"LCR": errors.New("the port is gone")}})

		if err := run(context.Background(), app); err == nil {
			t.Error("expected an error when the position cannot be read, got none")
		}
	})

	// Verify that asking for JSON encodes the position rather than printing it
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{values: map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}}})
		app.Config.Output = appcontext.OutputJSON

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var r report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if r.Latitude != 38.4208 {
			t.Errorf("expected the position in the JSON, got: %v", r)
		}
	})

	// Verify that a scanner following its own GPS is reported as having no range
	t.Run("NoRange", func(t *testing.T) {
		app, out := appWith(&stubConn{values: map[string][]string{"LCR": {"38.420800,-79.797200,0.0"}}})

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "none set (following the GPS)") {
			t.Errorf("expected the output to say no range is set, got: %q", out.String())
		}
	})

	// Verify that the position is written out with its range in miles
	t.Run("Text", func(t *testing.T) {
		app, out := appWith(&stubConn{values: map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}}})

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for _, want := range []string{"latitude:", "longitude:", "position:", "10.0 miles"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected the output to carry %q, got: %q", want, out.String())
			}
		}
	})
}

// Test_runGPS tests the runGPS function with 100% coverage.
//
// Coverage: 100% (12 test cases covering all branches)
//
// Test cases:
//   - OffAndStatus: two flags that contradict each other are refused
//   - WaitAndStatus: waiting for a fix while only reading is refused
//   - WaitAndOff: waiting for a fix while switching off is refused
//   - Status: reporting the setting is handed to the reader
//   - Off: switching the GPS off is handed to the other command
//   - DeviceError: a scanner that was never named is reported
//   - ReadError: a position that cannot be read first is reported
//   - SetError: a setting that cannot be reached is reported
//   - ConfirmError: a setting that did not take is reported
//   - Wait: a receiver that finds the scanner is waited for
//   - LatestError: a position that cannot be read afterwards is reported
//   - JSON: the position is encoded when JSON was asked for
//   - Text: a position that has not moved yet is said to be the one set by hand
func Test_runGPS(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes
		app.SetDevice(device.New(conn))
		return app, out, notes
	}

	// The walk to the setting, twice: once to set it and once to read it back.
	walk := []string{gpsMenu, setGPSFunction, gpsEnable, gpsMenu, setGPSFunction, gpsEnable}

	// Verify that switching the setting and only reading it cannot both be asked for
	t.Run("OffAndStatus", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{})

		if err := runGPS(context.Background(), app, false, true, true); err == nil {
			t.Error("expected an error when --off and --status are both given, got none")
		}
	})

	// Verify that waiting for a fix while only reading the setting is refused
	t.Run("WaitAndStatus", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{})

		if err := runGPS(context.Background(), app, true, false, true); err == nil {
			t.Error("expected an error when --wait and --status are both given, got none")
		}
	})

	// Verify that waiting for a fix while switching the GPS off is refused
	t.Run("WaitAndOff", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{})

		if err := runGPS(context.Background(), app, true, true, false); err == nil {
			t.Error("expected an error when --wait and --off are both given, got none")
		}
	})

	// Verify that --status is handed to the reader
	t.Run("Status", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsEnable},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := runGPS(context.Background(), app, false, false, true); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "gps:") {
			t.Errorf("expected the setting to be reported, got: %q", out.String())
		}
	})

	// Verify that --off is handed to the command that switches it off
	t.Run("Off", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsDisable, gpsMenu, setGPSFunction, gpsDisable},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := runGPS(context.Background(), app, false, true, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "fixed") {
			t.Errorf("expected the position to be reported as fixed, got: %q", out.String())
		}
	})

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runGPS(context.Background(), app, false, false, false); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a position that cannot be read first is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"LCR": errors.New("the port is gone")}})

		if err := runGPS(context.Background(), app, false, false, false); err == nil {
			t.Error("expected an error when the position cannot be read, got none")
		}
	})

	// Verify that a setting that cannot be reached is reported
	t.Run("SetError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{""},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := runGPS(context.Background(), app, false, false, false); err == nil {
			t.Error("expected an error when the setting cannot be reached, got none")
		}
	})

	// Verify that a setting that did not take is reported
	t.Run("ConfirmError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{gpsMenu, setGPSFunction, gpsEnable, gpsMenu, setGPSFunction, gpsDisable},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := runGPS(context.Background(), app, false, false, false); err == nil {
			t.Error("expected an error when the setting did not take, got none")
		}
	})

	// Verify that --wait sits through the receiver finding the scanner
	t.Run("Wait", func(t *testing.T) {
		app, out, notes := appWith(&stubConn{
			screens: walk,
			values: map[string][]string{"LCR": {
				"38.420800,-79.797200,10.0",
				"38.500000,-79.500000,10.0",
			}},
		})

		if err := runGPS(context.Background(), app, true, false, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "38.500000") {
			t.Errorf("expected the position the receiver found, got: %q", out.String())
		}
		if strings.Contains(notes.String(), "set by hand") {
			t.Errorf("expected no note about a position set by hand, got: %q", notes.String())
		}
	})

	// Verify that a wait the context puts an end to is reported
	t.Run("WaitError", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		app, _, _ := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := runGPS(ctx, app, true, false, false); err == nil {
			t.Error("expected an error when the context runs out while waiting, got none")
		}
	})

	// Verify that a position that cannot be read afterwards is reported
	t.Run("LatestError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0", ""}},
			fail:    map[string]error{},
		})

		// The second read answers with nothing at all, which is too few fields
		// to be a position.
		if err := runGPS(context.Background(), app, false, false, false); err == nil {
			t.Error("expected an error when the position cannot be read back, got none")
		}
	})

	// Verify that asking for JSON encodes the position rather than printing it
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})
		app.Config.Output = appcontext.OutputJSON

		if err := runGPS(context.Background(), app, false, false, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var latest report
		if err := json.Unmarshal(out.Bytes(), &latest); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if latest.Range != 10 {
			t.Errorf("expected the range in the JSON, got: %v", latest)
		}
	})

	// Verify that a position that has not moved yet is said to be the one set by hand
	t.Run("Text", func(t *testing.T) {
		app, out, notes := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := runGPS(context.Background(), app, false, false, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "source:    GPS") {
			t.Errorf("expected the source to be reported, got: %q", out.String())
		}
		if !strings.Contains(notes.String(), "set by hand") {
			t.Errorf("expected a note that the receiver has not found the scanner, got: %q", notes.String())
		}
	})

	// Verify that a scanner holding no range says so rather than printing a zero
	t.Run("NoRange", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: walk,
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,0.0"}},
		})

		if err := runGPS(context.Background(), app, false, false, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "none set") {
			t.Errorf("expected the output to say no range is set, got: %q", out.String())
		}
	})
}

// Test_runPosition tests the runPosition function with 100% coverage.
//
// Coverage: 100% (10 test cases covering all branches)
//
// Test cases:
//   - ParseError: a position that is not a pair of degrees is refused
//   - RangeError: a range outside the bounds is refused
//   - DeviceError: a scanner that was never named is reported
//   - ReadBeforeError: a position that cannot be read first is reported
//   - SetError: a position the scanner will not take is reported
//   - ReadAfterError: a position that cannot be read back is reported
//   - ConfirmError: a scanner that ended up somewhere else is reported
//   - Ranged: --range is written alongside the position
//   - JSON: the position is encoded when JSON was asked for
//   - Text: the position and its range are written out otherwise
func Test_runPosition(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	position := "38.420800,-79.797200"
	held := "38.420800,-79.797200,10.0"

	// Verify that a position that is not a pair of degrees is refused
	t.Run("ParseError", func(t *testing.T) {
		app, _ := appWith(&stubConn{})

		if err := runPosition(context.Background(), app, "somewhere", defaultRange, false); err == nil {
			t.Error("expected an error for a position that is not a pair, got none")
		}
	})

	// Verify that a range outside the bounds is refused before the scanner is touched
	t.Run("RangeError", func(t *testing.T) {
		app, _ := appWith(&stubConn{})

		err := runPosition(context.Background(), app, position, maxRange+1, true)
		if err == nil {
			t.Fatal("expected an error for a range past the bounds, got none")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Errorf("expected the message to say the range is out of bounds, got: %v", err)
		}
	})

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runPosition(context.Background(), app, position, defaultRange, false); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a position that cannot be read first is reported
	t.Run("ReadBeforeError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"LCR": errors.New("the port is gone")}})

		if err := runPosition(context.Background(), app, position, defaultRange, false); err == nil {
			t.Error("expected an error when the position cannot be read, got none")
		}
	})

	// Verify that a position the scanner will not take is reported
	t.Run("SetError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			values: map[string][]string{"LCR": {held}},
			fail: map[string]error{
				"LCR,38.420800,-79.797200,10.0": errors.New("the scanner refused the position"),
			},
		})

		err := runPosition(context.Background(), app, position, defaultRange, false)
		if err == nil {
			t.Fatal("expected an error when the position is refused, got none")
		}
		if !strings.Contains(err.Error(), "setting the position") {
			t.Errorf("expected the message to say it was setting the position, got: %v", err)
		}
	})

	// Verify that a position that cannot be read back is reported
	t.Run("ReadAfterError", func(t *testing.T) {
		app, _ := appWith(&stubConn{values: map[string][]string{"LCR": {held, ""}}})

		if err := runPosition(context.Background(), app, position, defaultRange, false); err == nil {
			t.Error("expected an error when the position cannot be read back, got none")
		}
	})

	// Verify that a scanner that ended up somewhere else is reported
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			values: map[string][]string{"LCR": {held, "43.000000,-79.797200,10.0"}},
		})

		err := runPosition(context.Background(), app, position, defaultRange, false)
		if err == nil {
			t.Fatal("expected an error when the scanner is somewhere else, got none")
		}
		if !strings.Contains(err.Error(), "rather than the") {
			t.Errorf("expected the message to compare the two positions, got: %v", err)
		}
	})

	// Verify that --range is written alongside the position when it was typed
	t.Run("Ranged", func(t *testing.T) {
		conn := &stubConn{values: map[string][]string{"LCR": {held}}}
		app, _ := appWith(conn)

		if err := runPosition(context.Background(), app, position, 20, true); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(strings.Join(conn.sent, " "), "LCR,38.420800,-79.797200,20.0") {
			t.Errorf("expected the range to be written with the position, got: %v", conn.sent)
		}
	})

	// Verify that asking for JSON encodes the position rather than printing it
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{values: map[string][]string{"LCR": {held}}})
		app.Config.Output = appcontext.OutputJSON

		if err := runPosition(context.Background(), app, position, defaultRange, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var after report
		if err := json.Unmarshal(out.Bytes(), &after); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if after.Latitude != 38.4208 {
			t.Errorf("expected the position in the JSON, got: %v", after)
		}
	})

	// Verify that the position is written out with its range in miles
	t.Run("Text", func(t *testing.T) {
		app, out := appWith(&stubConn{values: map[string][]string{"LCR": {held}}})

		if err := runPosition(context.Background(), app, position, defaultRange, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "10.0 miles") {
			t.Errorf("expected the range in miles, got: %q", out.String())
		}
	})

	// Verify that a scanner holding no range says so rather than printing a zero
	t.Run("NoRange", func(t *testing.T) {
		app, out := appWith(&stubConn{values: map[string][]string{"LCR": {"38.420800,-79.797200,0.0"}}})

		if err := runPosition(context.Background(), app, position, defaultRange, false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "none set") {
			t.Errorf("expected the output to say no range is set, got: %q", out.String())
		}
	})
}

// Test_runSet tests the runSet function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Both: a zip code and a position together are refused
//   - Neither: naming nothing at all is refused
//   - Position: a position is handed to the command that writes one
//   - Zip: a zip code is handed to the command that types one
func Test_runSet(t *testing.T) {
	// Builds an app writing to buffers.
	appWith := func() *appcontext.App {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		return app
	}

	// Verify that two ways of saying where cannot both be given
	t.Run("Both", func(t *testing.T) {
		err := runSet(context.Background(), appWith(), "12345", "38.4,-79.8", defaultRange, false)
		if err == nil {
			t.Fatal("expected an error when both are given, got none")
		}
		if !strings.Contains(err.Error(), "choose one") {
			t.Errorf("expected the message to ask for one of them, got: %v", err)
		}
	})

	// Verify that naming neither is refused with what the two of them are
	t.Run("Neither", func(t *testing.T) {
		err := runSet(context.Background(), appWith(), "", "", defaultRange, false)
		if err == nil {
			t.Fatal("expected an error when nothing is given, got none")
		}
		if !strings.Contains(err.Error(), "name a zip code") {
			t.Errorf("expected the message to ask for a zip code, got: %v", err)
		}
	})

	// Verify that a position is handed to the command that writes one
	t.Run("Position", func(t *testing.T) {
		err := runSet(context.Background(), appWith(), "", "somewhere", defaultRange, false)
		if err == nil {
			t.Fatal("expected an error for a position that is not a pair, got none")
		}
		if !strings.Contains(err.Error(), "is not a position") {
			t.Errorf("expected the position to have been parsed, got: %v", err)
		}
	})

	// Verify that a zip code is handed to the command that types one
	t.Run("Zip", func(t *testing.T) {
		err := runSet(context.Background(), appWith(), "0306", "", defaultRange, false)
		if err == nil {
			t.Fatal("expected an error for a zip code of the wrong length, got none")
		}
		if !strings.Contains(err.Error(), "is not a zip code") {
			t.Errorf("expected the zip code to have been checked, got: %v", err)
		}
	})
}

// Test_runZip tests the runZip function with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - ZipError: a zip code that is not five digits is refused
//   - RangeError: a range outside the bounds is refused
//   - DeviceError: a scanner that was never named is reported
//   - SetZipError: a zip code that cannot be entered is reported
//   - SetRangeError: a range that cannot be set after the zip code is reported
//   - ReadError: a position that cannot be read back is reported
//   - JSON: the position is encoded when JSON was asked for
//   - Text: the zip code, the position and the range are written out otherwise
func Test_runZip(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The text entry screen for the zip code, before and after it is typed.
	entry := func(value string) string {
		return `<MenuInfo Name="Zip Code" MenuType="TypeInput" Value="` + value + `">` +
			`<MenuInput MaxLength="5" EnableKeys="0123456789."/></MenuInfo>`
	}
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// The walk that enters a zip code, and the one that sets the range after it.
	zipWalk := []string{setYourLocation, enterZipCode, countryUSA, "Scanning", "Scanning"}
	rangeWalk := []string{setYourLocation, setRangeEntry, "10.0", "Scanning"}

	// Verify that a zip code of the wrong shape is refused before anything is touched
	t.Run("ZipError", func(t *testing.T) {
		app, _ := appWith(&stubConn{})

		if err := runZip(context.Background(), app, "0306", defaultRange); err == nil {
			t.Error("expected an error for a zip code that is not five digits, got none")
		}
	})

	// Verify that a range outside the bounds is refused before anything is touched
	t.Run("RangeError", func(t *testing.T) {
		app, _ := appWith(&stubConn{})

		err := runZip(context.Background(), app, "12345", maxRange+1)
		if err == nil {
			t.Fatal("expected an error for a range past the bounds, got none")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Errorf("expected the message to say the range is out of bounds, got: %v", err)
		}
	})

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runZip(context.Background(), app, "12345", defaultRange); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a zip code that cannot be entered is reported
	t.Run("SetZipError", func(t *testing.T) {
		app, _ := appWith(&stubConn{screens: []string{""}})

		if err := runZip(context.Background(), app, "12345", defaultRange); err == nil {
			t.Error("expected an error when the zip code cannot be entered, got none")
		}
	})

	// Verify that a range that will not set after the zip code did is reported
	t.Run("SetRangeError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: zipWalk,
			docs:    map[string][]string{"MSI": {entry(""), entry("12345"), outOfMenus}},
		})

		err := runZip(context.Background(), app, "12345", defaultRange)
		if err == nil {
			t.Fatal("expected an error when the range cannot be set, got none")
		}
		if !strings.Contains(err.Error(), "but its range was not") {
			t.Errorf("expected the message to say the position was set, got: %v", err)
		}
	})

	// Verify that a position that cannot be read back is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: append(append([]string{}, zipWalk...), rangeWalk...),
			docs:    map[string][]string{"MSI": {entry(""), entry("12345"), outOfMenus}},
			fail:    map[string]error{"LCR": errors.New("the port is gone")},
		})

		if err := runZip(context.Background(), app, "12345", defaultRange); err == nil {
			t.Error("expected an error when the position cannot be read back, got none")
		}
	})

	// Verify that asking for JSON encodes the position rather than printing it
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: append(append([]string{}, zipWalk...), rangeWalk...),
			docs:    map[string][]string{"MSI": {entry(""), entry("12345"), outOfMenus}},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})
		app.Config.Output = appcontext.OutputJSON

		if err := runZip(context.Background(), app, "12345", defaultRange); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var after report
		if err := json.Unmarshal(out.Bytes(), &after); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if after.Range != 10 {
			t.Errorf("expected the range in the JSON, got: %v", after)
		}
	})

	// Verify that the zip code, the position and the range are all written out
	t.Run("Text", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: append(append([]string{}, zipWalk...), rangeWalk...),
			docs:    map[string][]string{"MSI": {entry(""), entry("12345"), outOfMenus}},
			values:  map[string][]string{"LCR": {"38.420800,-79.797200,10.0"}},
		})

		if err := runZip(context.Background(), app, "12345", defaultRange); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for _, want := range []string{"zip:       12345", "38.420800", "10.0 miles"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected the output to carry %q, got: %q", want, out.String())
			}
		}
	})
}

// Test_screenText tests the screenText function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the display comes back as one string
//   - ReadError: a display that cannot be read is reported
func Test_screenText(t *testing.T) {
	// Verify that the lines of the display are joined into one string
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{"Scanning"}}

		text, err := screenText(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(text, "Scanning") {
			t.Errorf("expected the screen in the text, got: %q", text)
		}
	})

	// Verify that a display that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		if _, err := screenText(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the display cannot be read, got none")
		}
	})
}

// Test_setGPS tests the setGPS function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - WalkError: a setting that cannot be reached is reported
//   - SelectError: a value the screen does not offer is reported
//   - Success: the value is selected and the menus left
func Test_setGPS(t *testing.T) {
	// Verify that a setting that cannot be reached is reported
	t.Run("WalkError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		if err := setGPS(context.Background(), device.New(conn), gpsEnable); err == nil {
			t.Error("expected an error when the setting cannot be reached, got none")
		}
	})

	// Verify that a value the screen does not offer is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, setGPSFunction, ""}}

		err := setGPS(context.Background(), device.New(conn), gpsEnable)
		if err == nil {
			t.Fatal("expected an error when the value cannot be found, got none")
		}
		if !strings.Contains(err.Error(), gpsEnable) {
			t.Errorf("expected the message to name the value, got: %v", err)
		}
	})

	// Verify that the value is selected and the scanner returned to scanning
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, setGPSFunction, gpsEnable}}

		if err := setGPS(context.Background(), device.New(conn), gpsEnable); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}

// Test_setRange tests the setRange function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - MenuError: a location menu that cannot be reached is reported
//   - SelectError: a menu holding no range entry is reported
//   - DigitError: a range that is not two digits is reported
//   - KeyError: a key the scanner refuses is reported
//   - VerifyError: a screen showing something else is left without saving
//   - ReadyError: a scanner that does not come back after saving is reported
//   - Success: the range is typed, checked, and saved
func Test_setRange(t *testing.T) {
	walk := []string{setYourLocation, setRangeEntry}

	// Verify that a location menu that cannot be reached is reported
	t.Run("MenuError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		if err := setRange(context.Background(), device.New(conn), defaultRange); err == nil {
			t.Error("expected an error when the menu cannot be reached, got none")
		}
	})

	// Verify that a menu with no range entry is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{setYourLocation, ""}}

		err := setRange(context.Background(), device.New(conn), defaultRange)
		if err == nil {
			t.Fatal("expected an error when the range entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), setRangeEntry) {
			t.Errorf("expected the message to name the entry, got: %v", err)
		}
	})

	// Verify that a range that does not write as two digits is reported
	t.Run("DigitError", func(t *testing.T) {
		conn := &stubConn{screens: walk}

		err := setRange(context.Background(), device.New(conn), -1)
		if err == nil {
			t.Fatal("expected an error for a range that is not two digits, got none")
		}
		if !strings.Contains(err.Error(), "is not a digit") {
			t.Errorf("expected the message to say it is not a digit, got: %v", err)
		}
	})

	// Verify that a key the scanner refuses is reported
	t.Run("KeyError", func(t *testing.T) {
		conn := &stubConn{
			screens:  walk,
			failFrom: map[string]int{"KEY,1,P": 1},
		}

		err := setRange(context.Background(), device.New(conn), defaultRange)
		if err == nil {
			t.Fatal("expected an error when a key is refused, got none")
		}
		if !strings.Contains(err.Error(), "entering the range") {
			t.Errorf("expected the message to say it was entering the range, got: %v", err)
		}
	})

	// Verify that a screen showing something else is left without saving
	t.Run("VerifyError", func(t *testing.T) {
		conn := &stubConn{screens: []string{setYourLocation, setRangeEntry, "20.0"}}

		err := setRange(context.Background(), device.New(conn), defaultRange)
		if err == nil {
			t.Fatal("expected an error when the screen shows another range, got none")
		}
		if !strings.Contains(err.Error(), "nothing has been saved") {
			t.Errorf("expected the message to say nothing was saved, got: %v", err)
		}
	})

	// Verify that a range the scanner will not accept is reported
	t.Run("CommitError", func(t *testing.T) {
		conn := &stubConn{
			screens:  []string{setYourLocation, setRangeEntry, "10.0"},
			failFrom: map[string]int{"KEY,E,P": 3},
		}

		if err := setRange(context.Background(), device.New(conn), defaultRange); err == nil {
			t.Error("expected an error when the range cannot be saved, got none")
		}
	})

	// Verify that a scanner that does not come back after saving is reported
	t.Run("ReadyError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{setYourLocation, setRangeEntry, "10.0", "Scanning"},
			fail:    map[string]error{"MSI": errors.New("the port is gone")},
		}

		if err := setRange(context.Background(), device.New(conn), defaultRange); err == nil {
			t.Error("expected an error when the scanner does not come back, got none")
		}
	})

	// Verify that the range is typed, checked and saved
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{setYourLocation, setRangeEntry, "10.0", "Scanning"}}

		if err := setRange(context.Background(), device.New(conn), defaultRange); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for _, want := range []string{"KEY,1,P", "KEY,0,P"} {
			if !strings.Contains(strings.Join(conn.sent, " "), want) {
				t.Errorf("expected %q to be typed, got: %v", want, conn.sent)
			}
		}
	})
}

// Test_setZip tests the setZip function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - MenuError: a location menu that cannot be reached is reported
//   - ZipEntryError: a menu holding no zip entry is reported
//   - CountryError: a screen that will not take the country is reported
//   - TypeError: a zip code that cannot be typed is reported
//   - AwakenError: a scanner that stops answering afterwards is reported
//   - PromptError: a prompt that cannot be answered is reported
//   - Success: the zip code is typed and the menus left
func Test_setZip(t *testing.T) {
	// Builds an app writing to buffers.
	appWith := func() *appcontext.App {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		return app
	}

	// The text entry screen for the zip code, before and after it is typed.
	entry := func(value string) string {
		return `<MenuInfo Name="Zip Code" MenuType="TypeInput" Value="` + value + `">` +
			`<MenuInput MaxLength="5" EnableKeys="0123456789."/></MenuInfo>`
	}
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`
	typed := map[string][]string{"MSI": {entry(""), entry("12345"), outOfMenus}}

	// Verify that a location menu that cannot be reached is reported
	t.Run("MenuError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		if err := setZip(context.Background(), appWith(), device.New(conn), "12345"); err == nil {
			t.Error("expected an error when the menu cannot be reached, got none")
		}
	})

	// Verify that a menu with no zip entry is reported
	t.Run("ZipEntryError", func(t *testing.T) {
		conn := &stubConn{screens: []string{setYourLocation, ""}}

		err := setZip(context.Background(), appWith(), device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the zip entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), enterZipCode) {
			t.Errorf("expected the message to name the entry, got: %v", err)
		}
	})

	// Verify that a screen that will not offer the country is reported
	t.Run("CountryError", func(t *testing.T) {
		conn := &stubConn{screens: []string{setYourLocation, enterZipCode, ""}}

		err := setZip(context.Background(), appWith(), device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the country cannot be chosen, got none")
		}
		if !strings.Contains(err.Error(), countryUSA) {
			t.Errorf("expected the message to name the country, got: %v", err)
		}
	})

	// Verify that a zip code that cannot be typed is reported as nothing changed
	t.Run("TypeError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{setYourLocation, enterZipCode, countryUSA},
			docs:    map[string][]string{"MSI": {`<MenuInfo MenuType="TypeSelect" Name="Zip Code"/>`}},
		}

		err := setZip(context.Background(), appWith(), device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the zip code cannot be typed, got none")
		}
		if !strings.Contains(err.Error(), "entering the zip code") {
			t.Errorf("expected the message to say it was entering the zip code, got: %v", err)
		}
	})

	// Verify that a scanner that stops answering after the zip code is reported
	t.Run("AwakenError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{setYourLocation, enterZipCode, countryUSA},
			docs:    map[string][]string{"MSI": {entry(""), entry("12345"), outOfMenus}},
		}

		if err := setZip(context.Background(), appWith(), device.New(conn), "12345"); err == nil {
			t.Error("expected an error when the scanner stops answering, got none")
		}
	})

	// Verify that a zip code the scanner does not hold is reported
	t.Run("PromptError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{setYourLocation, enterZipCode, countryUSA, "Scanning", outOfRange},
			docs:    map[string][]string{"MSI": {entry(""), entry("12345"), outOfMenus}},
		}

		err := setZip(context.Background(), appWith(), device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the zip code is out of range, got none")
		}
		if !strings.Contains(err.Error(), "does not hold zip code") {
			t.Errorf("expected the message to name the zip code, got: %v", err)
		}
	})

	// Verify that a scanner that does not come back after the zip code is reported
	t.Run("ReadyError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{setYourLocation, enterZipCode, countryUSA, "Scanning", "Scanning"},
			docs:    map[string][]string{"MSI": {entry(""), entry("12345"), ""}},
		}

		err := setZip(context.Background(), appWith(), device.New(conn), "12345")
		if err == nil {
			t.Fatal("expected an error when the scanner does not come back, got none")
		}
		if !strings.Contains(err.Error(), "waiting for the scanner") {
			t.Errorf("expected the message to say it was waiting for the scanner, got: %v", err)
		}
	})

	// Verify that the zip code is typed and the scanner returned to scanning
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{setYourLocation, enterZipCode, countryUSA, "Scanning", "Scanning"},
			docs:    typed,
		}

		if err := setZip(context.Background(), appWith(), device.New(conn), "12345"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(strings.Join(conn.sent, " "), "KEY,3,P") {
			t.Errorf("expected the zip code to be typed, got: %v", conn.sent)
		}
	})
}

// Test_settled tests the settled function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Settled: a screen that has left the entry screen is returned
//   - ReadError: a screen that cannot be read is reported
//   - StillEntering: a screen that never changes is returned as it is
func Test_settled(t *testing.T) {
	// Verify that a screen that has moved on is returned straight away
	t.Run("Settled", func(t *testing.T) {
		conn := &stubConn{screens: []string{outOfRange}}

		shown, err := settled(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(shown, outOfRange) {
			t.Errorf("expected the screen that was showing, got: %q", shown)
		}
	})

	// Verify that a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		if _, err := settled(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the screen cannot be read, got none")
		}
	})

	// Verify that a screen still on the entry screen is reported as it is
	t.Run("StillEntering", func(t *testing.T) {
		screens := make([]string, 0, settleAttempts)
		for i := 0; i < settleAttempts; i++ {
			screens = append(screens, enterZipCode)
		}
		conn := &stubConn{screens: screens}

		shown, err := settled(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(shown, enterZipCode) {
			t.Errorf("expected the entry screen, got: %q", shown)
		}
	})
}

// Test_toGPSFunction tests the toGPSFunction function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - OpenMenuError: a top menu that will not open is reported
//   - GPSMenuError: a top menu with no GPS entry is reported
//   - SettingError: a GPS menu with no function entry is reported
//   - Success: the scanner lands on the setting
func Test_toGPSFunction(t *testing.T) {
	// Verify that a top menu that will not open is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"MNU,TOP,": errors.New("the scanner refused the jump")}}

		err := toGPSFunction(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the top menu will not open, got none")
		}
		if !strings.Contains(err.Error(), "opening the top menu") {
			t.Errorf("expected the message to say it was opening the top menu, got: %v", err)
		}
	})

	// Verify that a top menu with no GPS entry is reported
	t.Run("GPSMenuError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := toGPSFunction(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the GPS menu cannot be found, got none")
		}
		if !strings.Contains(err.Error(), gpsMenu) {
			t.Errorf("expected the message to name the menu, got: %v", err)
		}
	})

	// Verify that a GPS menu with no function entry is reported
	t.Run("SettingError", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, ""}}

		err := toGPSFunction(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the setting cannot be found, got none")
		}
		if !strings.Contains(err.Error(), setGPSFunction) {
			t.Errorf("expected the message to name the setting, got: %v", err)
		}
	})

	// Verify that the walk lands on the setting
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{gpsMenu, setGPSFunction}}

		if err := toGPSFunction(context.Background(), device.New(conn)); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}

// Test_toLocationMenu tests the toLocationMenu function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Success: a walk that works first time asks for nothing else
//   - WalkError: a failure that is not the scanner being busy is reported
//   - Retried: a scanner still busy is waited for and the walk tried again
//   - AwakenError: a scanner that never comes back is reported
//   - Exhausted: a scanner busy every time is given up on
func Test_toLocationMenu(t *testing.T) {
	// Verify that a walk that works first time is not retried
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{setYourLocation}}

		if err := toLocationMenu(context.Background(), device.New(conn)); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a failure that is not the scanner being busy is reported at once
	t.Run("WalkError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"MNU,TOP,": errors.New("the scanner refused the jump")}}

		if err := toLocationMenu(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the walk fails, got none")
		}
	})

	// Verify that a scanner still busy is waited for and the walk tried again
	t.Run("Retried", func(t *testing.T) {
		conn := &stubConn{
			screens:   []string{"Scanning", setYourLocation},
			failUntil: map[string]int{"MNU,TOP,": 1},
		}

		if err := toLocationMenu(context.Background(), device.New(conn)); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a scanner that never comes back is reported
	t.Run("AwakenError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"MNU,TOP,": context.DeadlineExceeded}}

		err := toLocationMenu(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the scanner never comes back, got none")
		}
		if !strings.Contains(err.Error(), "stopped answering") {
			t.Errorf("expected the message to say the scanner stopped answering, got: %v", err)
		}
	})

	// Verify that a scanner busy every time is given up on after the last attempt
	t.Run("Exhausted", func(t *testing.T) {
		screens := make([]string, 0, menuAttempts)
		for i := 0; i < menuAttempts; i++ {
			screens = append(screens, "Scanning")
		}
		conn := &stubConn{
			screens: screens,
			fail:    map[string]error{"MNU,TOP,": context.DeadlineExceeded},
		}

		if err := toLocationMenu(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the scanner is busy every time, got none")
		}
	})

	// Verify that a context that is over stops the walk being retried
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		conn := &stubConn{fail: map[string]error{"MNU,TOP,": context.DeadlineExceeded}}

		if err := toLocationMenu(ctx, device.New(conn)); err == nil {
			t.Error("expected an error when the context is over, got none")
		}
	})
}

// Test_verifyRange tests the verifyRange function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - ReadError: a screen that cannot be read is reported
//   - Mismatch: a screen showing another range is reported
//   - Match: the range the screen shows is the one that was typed
func Test_verifyRange(t *testing.T) {
	// Verify that a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := verifyRange(context.Background(), device.New(conn), defaultRange)
		if err == nil {
			t.Fatal("expected an error when the screen cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the range back") {
			t.Errorf("expected the message to say it was reading the range, got: %v", err)
		}
	})

	// Verify that a screen showing another range is reported
	t.Run("Mismatch", func(t *testing.T) {
		conn := &stubConn{screens: []string{"20.0"}}

		if err := verifyRange(context.Background(), device.New(conn), defaultRange); err == nil {
			t.Error("expected an error when the screen shows another range, got none")
		}
	})

	// Verify that the range the screen shows is accepted
	t.Run("Match", func(t *testing.T) {
		conn := &stubConn{screens: []string{"10.0"}}

		if err := verifyRange(context.Background(), device.New(conn), defaultRange); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}

// Test_walkToLocationMenu tests the walkToLocationMenu function with 100%
// coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - OpenMenuError: a top menu that will not open is reported
//   - SelectError: a top menu with no location entry is reported
//   - Success: the scanner lands on the location menu
func Test_walkToLocationMenu(t *testing.T) {
	// Verify that a top menu that will not open is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		conn := &stubConn{fail: map[string]error{"MNU,TOP,": errors.New("the scanner refused the jump")}}

		err := walkToLocationMenu(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the top menu will not open, got none")
		}
		if !strings.Contains(err.Error(), "opening the top menu") {
			t.Errorf("expected the message to say it was opening the top menu, got: %v", err)
		}
	})

	// Verify that a top menu with no location entry is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := walkToLocationMenu(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the location entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), setYourLocation) {
			t.Errorf("expected the message to name the entry, got: %v", err)
		}
	})

	// Verify that the walk lands on the location menu
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{setYourLocation}}

		if err := walkToLocationMenu(context.Background(), device.New(conn)); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}
