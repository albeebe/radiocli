// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package volume

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

// fakeConn is a device.Conn that answers each command from a function the test
// supplies, so the commands can be driven with no scanner attached.
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

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and marked as a command that only reads
//   - HasSet: the set subcommand is attached
//   - Runs: executing the command reports the level the scanner holds
func TestNew(t *testing.T) {
	// Verify the command carries the name and the annotation the tool wires on
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "volume" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "volume")
		}
		if got := cmd.Annotations[appcontext.OnlyReads]; got != "true" {
			t.Errorf("the only-reads annotation is %q, wanted %q", got, "true")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify the writing half of the command is reachable as a subcommand
	t.Run("HasSet", func(t *testing.T) {
		var found bool
		for _, sub := range New(appcontext.New()).Commands() {
			if sub.Name() == "set" {
				found = true
			}
		}
		if !found {
			t.Error("the set subcommand is not attached")
		}
	})

	// Verify running the bare command reads the scanner and reports the level
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "11", nil
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if got, want := out.String(), "volume: 11 of 15\n"; got != want {
			t.Errorf("the command wrote %q, wanted %q", got, want)
		}
	})
}

// Test_newSet covers the set subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering both paths through the closure)
//
// Test cases:
//   - Sets: a level the scanner takes is sent and reported back
//   - RefusesTypo: a level that is not a number is refused before the scanner
//     is opened
func Test_newSet(t *testing.T) {
	// Verify a valid level reaches the scanner and the result is printed
	t.Run("Sets", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		var sent string
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.Contains(command, ",") {
				sent = command
				return "", nil
			}
			return "6", nil
		}}))

		cmd := newSet(app)
		cmd.SetArgs([]string{"6"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if sent != "VOL,6" {
			t.Errorf("the scanner was sent %q, wanted %q", sent, "VOL,6")
		}
		if got, want := out.String(), "volume: 6 of 15\n"; got != want {
			t.Errorf("the subcommand wrote %q, wanted %q", got, want)
		}
	})

	// Verify a level that is not a number is refused without a scanner present
	t.Run("RefusesTypo", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newSet(app)
		cmd.SetArgs([]string{"loud"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("the subcommand reported nothing, wanted the typo refused")
		}
		if !strings.Contains(err.Error(), "invalid volume level") {
			t.Errorf("the subcommand reported %q, wanted it to name the invalid level", err)
		}
	})
}

// Test_parseLevel covers what somebody can type where a level is wanted.
//
// Coverage: 100% (1 test case covering all three branches)
//
// Test cases:
//   - Typed: the levels the scanner takes, and the ones it does not
func Test_parseLevel(t *testing.T) {
	// Verify whole numbers in range are taken and everything else is refused
	t.Run("Typed", func(t *testing.T) {
		cases := []struct {
			typed string
			level int
			ok    bool
		}{
			{"0", 0, true},
			{"8", 8, true},
			{"15", 15, true},

			// The scanner pads numbers with spaces, and so does a person.
			{" 3 ", 3, true},

			// Refused rather than clamped: a level nobody asked for is worse
			// than a message naming what was typed.
			{"16", 0, false},
			{"-1", 0, false},
			{"loud", 0, false},
			{"", 0, false},
			{"8.5", 0, false},
		}

		for _, c := range cases {
			got, err := parseLevel(c.typed)
			if c.ok && err != nil {
				t.Errorf("parseLevel(%q) reported %v, wanted %d", c.typed, err, c.level)
				continue
			}
			if !c.ok && err == nil {
				t.Errorf("parseLevel(%q) took it as %d, wanted it refused", c.typed, got)
				continue
			}
			if got != c.level {
				t.Errorf("parseLevel(%q) is %d, wanted %d", c.typed, got, c.level)
			}
		}
	})
}

// Test_renderLevel covers both ways the level is reported.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Text: the level is written as a sentence
//   - JSON: the level is written with the bounds it is measured against
func Test_renderLevel(t *testing.T) {
	// Verify the text output names the level and what it is out of
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		if err := renderLevel(app, 12); err != nil {
			t.Fatalf("renderLevel: %v", err)
		}
		if got, want := out.String(), "volume: 12 of 15\n"; got != want {
			t.Errorf("renderLevel wrote %q, wanted %q", got, want)
		}
	})

	// Verify the JSON output carries the level along with its bounds
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := renderLevel(app, 12); err != nil {
			t.Fatalf("renderLevel: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		want := report{Level: 12, Min: device.MinLevel, Max: device.MaxLevel}
		if got != want {
			t.Errorf("renderLevel wrote %+v, wanted %+v", got, want)
		}
	})
}

// Test_runGet covers reading the level off the scanner.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Reads: the level the scanner reports is rendered
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer is reported
func Test_runGet(t *testing.T) {
	// Verify the level read from the scanner reaches the output
	t.Run("Reads", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "7", nil
		}}))

		if err := runGet(context.Background(), app); err != nil {
			t.Fatalf("runGet: %v", err)
		}
		if got, want := out.String(), "volume: 7 of 15\n"; got != want {
			t.Errorf("runGet wrote %q, wanted %q", got, want)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runGet(context.Background(), app)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runGet reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that fails to answer is reported as a failed read
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}}))

		err := runGet(context.Background(), app)
		if err == nil {
			t.Fatal("runGet reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the volume") {
			t.Errorf("runGet reported %q, wanted it to name reading the volume", err)
		}
	})
}

// Test_runSet covers changing the level and reporting what the scanner holds.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Sets: the level is sent and the level read back is reported
//   - Differs: a scanner that lands somewhere else says so
//   - NoDevice: a run with no scanner named is refused
//   - SetFails: a scanner that refuses the level is reported
//   - OutputFails: output that cannot be written is reported
//   - ReadBackFails: a scanner that will not say where it landed is reported
func Test_runSet(t *testing.T) {
	// Verify the level is sent and the scanner's own answer is what is printed
	t.Run("Sets", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.Contains(command, ",") {
				return "", nil
			}
			return "9", nil
		}}))

		if err := runSet(context.Background(), app, 9); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		if got, want := out.String(), "volume: 9 of 15\n"; got != want {
			t.Errorf("runSet wrote %q, wanted %q", got, want)
		}
		if notes.Len() != 0 {
			t.Errorf("runSet noted %q, wanted nothing said about a level it took", notes.String())
		}
	})

	// Verify a scanner that quietly lands elsewhere is not reported as a success
	t.Run("Differs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.Contains(command, ",") {
				return "", nil
			}
			return "5", nil
		}}))

		if err := runSet(context.Background(), app, 9); err != nil {
			t.Fatalf("runSet: %v", err)
		}
		if got, want := out.String(), "volume: 5 of 15\n"; got != want {
			t.Errorf("runSet wrote %q, wanted the level the scanner reported, %q", got, want)
		}
		if !strings.Contains(notes.String(), "rather than the 9") {
			t.Errorf("the note is %q, wanted it to name the level that was asked for", notes.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runSet(context.Background(), app, 9)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runSet reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not take the level is reported as such
	t.Run("SetFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}}))

		err := runSet(context.Background(), app, 9)
		if err == nil {
			t.Fatal("runSet reported nothing, wanted the refused level")
		}
		if !strings.Contains(err.Error(), "setting the volume") {
			t.Errorf("runSet reported %q, wanted it to name setting the volume", err)
		}
	})

	// Verify output that cannot be written is reported rather than swallowed
	t.Run("OutputFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = failWriter{}
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.Contains(command, ",") {
				return "", nil
			}
			return "9", nil
		}}))

		err := runSet(context.Background(), app, 9)
		if err == nil {
			t.Fatal("runSet reported nothing, wanted the failed write")
		}
		if !strings.Contains(err.Error(), "the pipe closed") {
			t.Errorf("runSet reported %q, wanted the write failure", err)
		}
	})

	// Verify a scanner that takes the level but will not say where it landed is
	// reported rather than assumed
	t.Run("ReadBackFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.Contains(command, ",") {
				return "", nil
			}
			return "", errors.New("the port closed")
		}}))

		err := runSet(context.Background(), app, 9)
		if err == nil {
			t.Fatal("runSet reported nothing, wanted the failed read back")
		}
		if !strings.Contains(err.Error(), "reading the volume back") {
			t.Errorf("runSet reported %q, wanted it to name reading the volume back", err)
		}
	})
}
