// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package screen

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

// fakeConn is a device.Conn that answers every command with a canned reply, so
// the command can be driven with no scanner attached.
type fakeConn struct {
	reply string
	err   error
}

// Info describes the connected scanner.
func (f fakeConn) Info() device.Info { return device.Info{} }

// Execute returns the canned reply, which stands in for the scanner's answer.
func (f fakeConn) Execute(ctx context.Context, command string) (string, error) {
	return f.reply, f.err
}

// ExecuteXML returns the canned reply, which stands in for an XML answer.
func (f fakeConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return f.reply, f.err
}

// Send reports the canned error, since nothing here writes without reading.
func (f fakeConn) Send(ctx context.Context, command string) error { return f.err }

// Close releases nothing, because there is no port.
func (f fakeConn) Close() error { return nil }

// TestWidenKeepsTheScannersPictures is the regression for a defect that reported
// nothing: the screen's bytes went into JSON as invalid UTF-8, encoding/json
// replaced each one with U+FFFD without returning an error, and every picture
// above 0x7E left as the same character.
//
// 0xAC and 0xAD are the two halves of the signal meter, which is on screen
// whenever the scanner is receiving.
func TestWidenKeepsTheScannersPictures(t *testing.T) {
	raw := string([]byte{'A', 0xac, 0xad, 'B', 0x1a, 0x1b, 0xf8})

	out, err := json.Marshal(widen(report{Lines: []line{{Text: raw}}}))
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if strings.ContainsRune(string(out), '�') {
		t.Fatalf("a byte was replaced: %s", out)
	}

	var back report
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}

	// The reader gets the code point of the byte the scanner sent, which is what
	// a renderer indexes the font with.
	want := []rune{'A', 0xac, 0xad, 'B', 0x1a, 0x1b, 0xf8}
	got := []rune(back.Lines[0].Text)
	if len(got) != len(want) {
		t.Fatalf("got %d characters, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("character %d is %#U, want %#U", i, got[i], want[i])
		}
	}
}

// TestWidenLeavesAsciiAlone is what makes the change safe for every reader that
// never sees a picture: an ordinary row must come out byte for byte as before.
func TestWidenLeavesAsciiAlone(t *testing.T) {
	in := report{Lines: []line{
		{Text: "  SYSTEM      DEPT     CHANNEL", Attributes: "********* ********** *********"},
		{Text: "155.550000MHz"},
	}}

	before, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	after, err := json.Marshal(widen(in))
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}

	if string(before) != string(after) {
		t.Errorf("ascii changed:\n before %s\n after  %s", before, after)
	}
}

// TestWidenDoesNotTouchTheReport guards the reason this is done at the point of
// encoding rather than where the line is built: the device layer decides what is
// an attribute row by comparing byte lengths, so the raw string has to survive.
func TestWidenDoesNotTouchTheReport(t *testing.T) {
	raw := string([]byte{0xac, 0xad})
	in := report{Lines: []line{{Text: raw}}}

	widen(in)

	if in.Lines[0].Text != raw {
		t.Errorf("the original was modified: % x", in.Lines[0].Text)
	}
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and marked as a command that only reads
//   - Runs: executing the command reports the display it was given
func TestNew(t *testing.T) {
	// Verify the command carries the name and the annotation the tool wires on
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "screen" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "screen")
		}
		if got := cmd.Annotations[appcontext.OnlyReads]; got != "true" {
			t.Errorf("the only-reads annotation is %q, wanted %q", got, "true")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify running the command reads the scanner and writes what it read
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: "0,SCAN,"}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if got := out.String(); got != "  SCAN\n" {
			t.Errorf("the command wrote %q, wanted %q", got, "  SCAN\n")
		}
	})
}

// Test_renderScreen covers the text output, which is the marker and the note.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Marks: the highlighted row is marked and the others are indented
//   - Empty: a screen with no rows says so, on stderr
func Test_renderScreen(t *testing.T) {
	// Verify the highlighted row is the only one that gets the marker
	t.Run("Marks", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}

		r := report{Lines: []line{
			{Text: "SCAN"},
			{Text: "GREENDALE", Highlighted: true},
		}}

		if err := renderScreen(app, r, device.Display{}); err != nil {
			t.Fatalf("renderScreen: %v", err)
		}
		if got, want := out.String(), "  SCAN\n> GREENDALE\n"; got != want {
			t.Errorf("renderScreen wrote %q, wanted %q", got, want)
		}
	})

	// Verify an empty screen is reported as a note rather than as nothing
	t.Run("Empty", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes

		if err := renderScreen(app, report{}, device.Display{}); err != nil {
			t.Fatalf("renderScreen: %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("renderScreen wrote %q to stdout, wanted nothing", out.String())
		}
		if !strings.Contains(notes.String(), "empty display") {
			t.Errorf("the note is %q, wanted it to mention an empty display", notes.String())
		}
	})
}

// Test_run covers reading the screen and both ways of reporting it.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Text: the display is written as text, marking the highlighted row
//   - JSON: the display is written as JSON, keeping the large font flag
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer is reported
func Test_run(t *testing.T) {
	// The scanner's answer to STS: two lines, the second in the large font and
	// drawn in reverse video, which is how it marks the selection.
	const display = "01,SCAN,          ,GREENDALE,**********"

	// Verify a screen read from the scanner comes out as marked text
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: display}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got, want := out.String(), "  SCAN\n> GREENDALE\n"; got != want {
			t.Errorf("run wrote %q, wanted %q", got, want)
		}
	})

	// Verify JSON output carries the attributes and the large font flag
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON
		app.SetDevice(device.New(fakeConn{reply: display}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if len(got.Lines) != 2 {
			t.Fatalf("there are %d lines, wanted 2: %q", len(got.Lines), out.String())
		}
		if got.Lines[0].Text != "SCAN" || got.Lines[0].Highlighted || got.Lines[0].LargeFont {
			t.Errorf("the first line is %+v, wanted a plain SCAN", got.Lines[0])
		}
		if got.Lines[1].Text != "GREENDALE" || !got.Lines[1].Highlighted || !got.Lines[1].LargeFont {
			t.Errorf("the second line is %+v, wanted a highlighted GREENDALE in the large font", got.Lines[1])
		}
		if got.Lines[1].Attributes != "**********" {
			t.Errorf("the attributes are %q, wanted the ten the scanner sent", got.Lines[1].Attributes)
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

	// Verify a scanner that fails to answer is reported as a failed read
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{err: errors.New("the port closed")}))

		err := run(context.Background(), app)
		if err == nil {
			t.Fatal("run reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("run reported %q, wanted it to name reading the screen", err)
		}
	})
}
