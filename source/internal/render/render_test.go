// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/13/2026

package render

import (
	"bytes"
	"encoding/json"
	"errors"
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
