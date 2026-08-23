// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/13/2026

package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// failingWriter refuses everything, which is the only way to reach the error
// JSON returns: the values the commands encode are structs of strings, numbers
// and bools, and none of those can fail to marshal.
type failingWriter struct{}

// Write refuses the write and says why.
func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("the stream is gone")
}

// TestChanged covers both output modes of a mutation report. The JSON mode is
// the one this exists for: these verbs used to write prose whatever was asked
// for, so a script editing the scanner got sentences it could not parse.
func TestChanged(t *testing.T) {
	// Builds an app writing to buffers, in the format asked for.
	appWith := func(format appcontext.OutputFormat) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.Config.Output = format
		return app, out
	}

	rename := Mutation{
		Action: "renamed", Kind: "channel", Name: "EVENING", Was: "NIGHT WATCH", In: "FIRE",
	}

	// Verify that JSON mode writes the mutation rather than the text line.
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(appcontext.OutputJSON)

		if err := Changed(app, rename, "EVENING"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got Mutation
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the output is not JSON: %v\nstdout: %s", err, out.String())
		}
		if got != rename {
			t.Errorf("Changed wrote %+v, wanted %+v", got, rename)
		}
	})

	// Verify that the empty fields are left out rather than written as empty
	// strings, so a consumer can tell a rename from a create.
	t.Run("OmitsWhatDidNotHappen", func(t *testing.T) {
		app, out := appWith(appcontext.OutputJSON)

		err := Changed(app, Mutation{Action: "deleted", Kind: "site", Name: "AIRPORT"}, "deleted AIRPORT")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if strings.Contains(out.String(), "\"was\"") || strings.Contains(out.String(), "\"in\"") {
			t.Errorf("expected the unused fields to be left out, got: %s", out.String())
		}
	})

	// Verify that text mode writes the line it was handed, unchanged. What
	// these commands print is what people already read, so the JSON mode was
	// added beside it rather than in place of it.
	t.Run("Text", func(t *testing.T) {
		app, out := appWith(appcontext.OutputText)

		if err := Changed(app, rename, "EVENING"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if out.String() != "EVENING\n" {
			t.Errorf("Changed wrote %q, wanted %q", out.String(), "EVENING\n")
		}
	})

	// Verify that a stream that cannot be written to is reported
	t.Run("WriteError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = failingWriter{}, &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := Changed(app, rename, "EVENING"); err == nil {
			t.Error("expected an error when the stream refuses the write, got none")
		}
	})
}

// TestDash covers the two answers there are. The point of the function is that
// a blank cell in an aligned table is indistinguishable from a column that ran
// out of rows, so the empty case is the one that matters.
func TestDash(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"a value is left alone", "P25 Trunk", "P25 Trunk"},
		{"nothing becomes a dash", "", "-"},
		{"a space is a value", " ", " "},
		{"a dash stays a dash", "-", "-"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Dash(c.value); got != c.want {
				t.Errorf("Dash(%q) = %q, wanted %q", c.value, got, c.want)
			}
		})
	}
}

// TestJSON covers what every command's --output json path produces.
func TestJSON(t *testing.T) {
	// Verify that the output is indented and ends in a newline, which is what
	// makes it a well-formed line on a terminal.
	t.Run("Indented", func(t *testing.T) {
		out := &bytes.Buffer{}

		if err := JSON(out, map[string]string{"name": "AIRPORT"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		want := "{\n  \"name\": \"AIRPORT\"\n}\n"
		if out.String() != want {
			t.Errorf("JSON wrote %q, wanted %q", out.String(), want)
		}
	})

	// Verify that an empty listing is still JSON rather than nothing.
	t.Run("Empty", func(t *testing.T) {
		out := &bytes.Buffer{}

		if err := JSON(out, []string{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if strings.TrimSpace(out.String()) != "[]" {
			t.Errorf("JSON wrote %q, wanted an empty array", out.String())
		}
	})

	// Verify that a listing which found nothing is written as an empty array
	// rather than as null. Go encodes a nil slice as null, and a script told
	// null by a command documented to answer with an array has nothing to range
	// over and no length to take.
	t.Run("NothingFound", func(t *testing.T) {
		var found []string

		out := &bytes.Buffer{}
		if err := JSON(out, found); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if strings.TrimSpace(out.String()) != "[]" {
			t.Errorf("JSON wrote %q for a listing that found nothing, wanted an empty array", out.String())
		}
	})

	// Verify that a value which is not a list is written as it is. An object
	// with nothing in it is not a listing that found nothing, and the two must
	// not be confused for one another.
	t.Run("NotAList", func(t *testing.T) {
		out := &bytes.Buffer{}

		if err := JSON(out, map[string]string{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if strings.TrimSpace(out.String()) != "{}" {
			t.Errorf("JSON wrote %q, wanted the object it was given", out.String())
		}
	})

	// Verify that a stream that cannot be written to is reported rather than
	// swallowed, since a command returns this error as its own.
	t.Run("WriteError", func(t *testing.T) {
		if err := JSON(failingWriter{}, "anything"); err == nil {
			t.Error("expected an error when the stream refuses the write, got none")
		}
	})
}

// TestYesNo covers both answers.
func TestYesNo(t *testing.T) {
	if got := YesNo(true); got != "yes" {
		t.Errorf("YesNo(true) = %q, wanted \"yes\"", got)
	}
	if got := YesNo(false); got != "no" {
		t.Errorf("YesNo(false) = %q, wanted \"no\"", got)
	}
}

// TestFilled tests the Filled function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Some: the note names how many entries are only partly known
//   - None: a whole listing says nothing at all
//   - JSON: the note is left out of the machine-readable path
func TestFilled(t *testing.T) {
	// Verify that the note goes to stderr, names the count, and says which
	// mark stands for the columns nobody read.
	t.Run("Some", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		Filled(app, "departments", 3)

		if !strings.Contains(notes.String(), "3 of these departments") {
			t.Errorf("the note reads %q, wanted it to name the count and the kind", notes.String())
		}
		if !strings.Contains(notes.String(), Unread) {
			t.Errorf("the note reads %q, wanted it to name the mark it explains", notes.String())
		}
		if out.Len() != 0 {
			t.Errorf("the note wrote %q to stdout, which belongs to the listing", out.String())
		}
	})

	// Verify that a listing with nothing missing says nothing, rather than
	// reporting that none of it is missing.
	t.Run("None", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout, app.Stderr = &bytes.Buffer{}, notes

		Filled(app, "departments", 0)

		if notes.Len() != 0 {
			t.Errorf("the note reads %q, wanted nothing", notes.String())
		}
	})

	// Verify that a script asking for JSON gets none of the prose: it reads the
	// partial field on the entries instead.
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout, app.Stderr = &bytes.Buffer{}, notes
		app.Config.Output = appcontext.OutputJSON

		Filled(app, "departments", 3)

		if notes.Len() != 0 {
			t.Errorf("the note reads %q, wanted nothing on the JSON path", notes.String())
		}
	})
}

// TestAlert tests the Alert function with 100% coverage.
//
// Coverage: 100% (3 test cases covering both the coloured and the plain path)
//
// Test cases:
//   - Plain: a buffer is not a terminal, so the message arrives with no escape
//     codes in it
//   - Formatted: the arguments are substituted the way Printf does it
//   - Terminal: a real terminal gets the escapes, and NO_COLOR takes them away
func TestAlert(t *testing.T) {
	appWith := func() (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		errs := &bytes.Buffer{}
		app.Stdout, app.Stderr = &bytes.Buffer{}, errs
		return app, errs
	}

	// Verify that a stream which is not a terminal is written to plainly,
	// which is what keeps escape codes out of a redirected log.
	t.Run("Plain", func(t *testing.T) {
		app, errs := appWith()

		Alert(app, "the input is overloaded\n")

		if got := errs.String(); got != "the input is overloaded\n" {
			t.Errorf("Alert wrote %q, wanted it uncoloured", got)
		}
	})

	// Verify that the arguments reach the message.
	t.Run("Formatted", func(t *testing.T) {
		app, errs := appWith()

		Alert(app, "%.1f%% of %d\n", 9.3, 15)

		if got := errs.String(); got != "9.3% of 15\n" {
			t.Errorf("Alert wrote %q, wanted the arguments substituted", got)
		}
	})

	// Verify the coloured path, and that NO_COLOR takes it away again.
	//
	// /dev/null stands in for a terminal because the test colorful applies is
	// whether the stream is a character device, which it is. Writing escapes
	// into it is exactly what the check is being asked about, and it needs no
	// pty to arrange.
	t.Run("CharacterDevice", func(t *testing.T) {
		dev, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			t.Fatalf("opening %s: %v", os.DevNull, err)
		}
		defer dev.Close()

		os.Unsetenv("NO_COLOR")
		if !colorful(dev) {
			t.Fatal("a character device was not recognised as colourable")
		}

		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, dev
		Alert(app, "overloaded\n")

		t.Setenv("NO_COLOR", "1")
		if colorful(dev) {
			t.Error("NO_COLOR did not turn the colour off")
		}
	})
}

// wrapped stands in for the witness main puts around both streams before any
// command runs, which is why colorful has to see past a wrapper at all.
type wrapped struct{ to io.Writer }

// Write passes everything on.
func (w wrapped) Write(p []byte) (int, error) { return w.to.Write(p) }

// Unwrap returns the stream underneath.
func (w wrapped) Unwrap() io.Writer { return w.to }

// TestColorful tests the colorful function with 100% coverage.
//
// The unwrapping cases are the ones that matter. Every stream a command writes
// to has been wrapped by the time it gets there, so a check that only accepts a
// bare *os.File says "not a terminal" about the whole program and the colour
// never appears anywhere. It fails silently, which is why it is tested here
// rather than left to be noticed.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - NotAFile: a buffer cannot be a terminal
//   - Closed: a file that cannot be inspected is not a terminal either
//   - Wrapped: a wrapper around a character device is one
//   - WrappedBuffer: a wrapper around a buffer is not
//   - WrapsNothing: a wrapper hiding nil is refused rather than followed
func TestColorful(t *testing.T) {
	// Verify that anything which is not an *os.File is refused before it is
	// asked, since only a file has a mode to check.
	t.Run("NotAFile", func(t *testing.T) {
		os.Unsetenv("NO_COLOR")

		if colorful(&bytes.Buffer{}) {
			t.Error("a buffer was treated as a terminal")
		}
	})

	// Verify that a file whose Stat fails is refused rather than assumed.
	t.Run("Closed", func(t *testing.T) {
		os.Unsetenv("NO_COLOR")

		f, err := os.CreateTemp(t.TempDir(), "closed")
		if err != nil {
			t.Fatalf("making a file: %v", err)
		}
		f.Close()

		if colorful(f) {
			t.Error("a closed file was treated as a terminal")
		}
	})

	// Verify a wrapper is followed to what it wraps, which is the case every
	// real run takes and the one a bare type assertion silently fails.
	t.Run("Wrapped", func(t *testing.T) {
		os.Unsetenv("NO_COLOR")

		dev, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			t.Fatalf("opening %s: %v", os.DevNull, err)
		}
		defer dev.Close()

		if !colorful(wrapped{to: wrapped{to: dev}}) {
			t.Error("a wrapped character device was not followed to the device")
		}
	})

	// Verify unwrapping reports what it finds rather than assuming a wrapper
	// means a terminal.
	t.Run("WrappedBuffer", func(t *testing.T) {
		os.Unsetenv("NO_COLOR")

		if colorful(wrapped{to: &bytes.Buffer{}}) {
			t.Error("a wrapped buffer was treated as a terminal")
		}
	})

	// Verify a wrapper with nothing behind it ends the walk instead of being
	// dereferenced.
	t.Run("WrapsNothing", func(t *testing.T) {
		os.Unsetenv("NO_COLOR")

		if colorful(wrapped{to: nil}) {
			t.Error("a wrapper around nothing was treated as a terminal")
		}
	})
}
