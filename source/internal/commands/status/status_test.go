// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package status

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
// supplies, so the command can be driven with no scanner attached.
type fakeConn struct {
	info  device.Info
	reply func(command string) (string, error)
}

// Info describes the connected scanner.
func (f fakeConn) Info() device.Info { return f.info }

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

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and marked as a command that only reads
//   - Runs: executing the command reports the scanner it was given
func TestNew(t *testing.T) {
	// Verify the command carries the name and the annotation the tool wires on
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "status" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "status")
		}
		if got := cmd.Annotations[appcontext.OnlyReads]; got != "true" {
			t.Errorf("the only-reads annotation is %q, wanted %q", got, "true")
		}
		if cmd.Short == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify running the command reads the scanner and writes what it read
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{
			info: device.Info{Port: "/dev/example", Model: "SDS150"},
			reply: func(command string) (string, error) {
				switch command {
				case "VER":
					return "Version 1.00.37", nil
				case "GST":
					return "0,SCAN,,0,0,0,1,0,,0,0,0,0,0,0", nil
				default:
					return `<ScannerInfo Mode="Scan Mode"></ScannerInfo>`, nil
				}
			},
		}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "model:    SDS150") {
			t.Errorf("the command wrote %q, wanted it to name the model", out.String())
		}
	})
}

// Test_run covers reading the scanner's identity and both ways of reporting it.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Text: everything read is written as a block of labelled lines
//   - Holding: a scanner parked on one thing is called out on stderr
//   - JSON: the same reading is written as one object
//   - NoDevice: a run with no scanner named is refused
//   - FirmwareFails: a scanner that will not report its version is reported
//   - DisplayFails: a scanner that will not report its display mode is reported
//   - InfoFails: a scanner that will not say what it is doing is reported
func Test_run(t *testing.T) {
	// A scanner that answers everything: a version, a one line screen with the
	// twelve status fields behind it, and a mode.
	answering := func(mode string) func(string) (string, error) {
		return func(command string) (string, error) {
			switch command {
			case "VER":
				return "Version 1.00.37", nil
			case "GST":
				return "0,SCAN,,0,0,0,1,0,,0,0,0,0,0,0", nil
			default:
				return `<ScannerInfo Mode="` + mode + `"></ScannerInfo>`, nil
			}
		}
	}

	// A scanner that answers everything except the one command named.
	refusing := func(refuse string) func(string) (string, error) {
		answers := answering("Scan Mode")
		return func(command string) (string, error) {
			if command == refuse {
				return "", errors.New("the port closed")
			}
			return answers(command)
		}
	}

	// Verify a scanner that is scanning is reported line by line, with no note
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes
		app.SetDevice(device.New(fakeConn{
			info:  device.Info{Port: "/dev/example", Model: "SDS150"},
			reply: answering("Scan Mode"),
		}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}

		want := "port:     /dev/example\n" +
			"model:    SDS150\n" +
			"firmware: Version 1.00.37\n" +
			"display:  color\n" +
			"mode:     Scan Mode\n"
		if got := out.String(); got != want {
			t.Errorf("run wrote %q, wanted %q", got, want)
		}
		if notes.Len() != 0 {
			t.Errorf("run noted %q, wanted nothing said about a scanning scanner", notes.String())
		}
	})

	// Verify a held scanner is called out, since it looks like a scanning one
	t.Run("Holding", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes
		app.SetDevice(device.New(fakeConn{
			info:  device.Info{Port: "/dev/example", Model: "SDS150"},
			reply: answering("Scan Hold"),
		}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out.String(), "mode:     Scan Hold") {
			t.Errorf("run wrote %q, wanted it to name the mode", out.String())
		}
		if !strings.Contains(notes.String(), "holding rather than scanning") {
			t.Errorf("the note is %q, wanted it to say the scanner is holding", notes.String())
		}
	})

	// Verify the JSON output carries every field the text output prints
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON
		app.SetDevice(device.New(fakeConn{
			info:  device.Info{Port: "/dev/example", Model: "SDS150"},
			reply: answering("Scan Hold"),
		}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		want := report{
			Port:     "/dev/example",
			Model:    "SDS150",
			Firmware: "Version 1.00.37",
			Display:  "color",
			Mode:     "Scan Hold",
			Holding:  true,
		}
		if got != want {
			t.Errorf("run wrote %+v, wanted %+v", got, want)
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := run(context.Background(), app)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("run reported %v, wanted a missing device", err)
		}
	})

	// Verify each read that can fail is named in the error it produces
	for _, c := range []struct {
		name    string
		command string
		names   string
	}{
		{"FirmwareFails", "VER", "reading the firmware version"},
		{"DisplayFails", "GST", "reading the display mode"},
		{"InfoFails", "GSI", "asking the scanner what it is doing"},
	} {
		// Verify the failed read is reported, and says which read it was
		t.Run(c.name, func(t *testing.T) {
			app := appcontext.New()
			app.Stdout = &bytes.Buffer{}
			app.Stderr = &bytes.Buffer{}
			app.SetDevice(device.New(fakeConn{reply: refusing(c.command)}))

			err := run(context.Background(), app)
			if err == nil {
				t.Fatalf("run reported nothing, wanted the failed %q", c.command)
			}
			if !strings.Contains(err.Error(), c.names) {
				t.Errorf("run reported %q, wanted it to name %q", err, c.names)
			}
		})
	}
}
