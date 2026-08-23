// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audioin"
)

// cancelWriter takes what is written and then stops the command, which is how
// a stream with no natural end is ended at a known point.
type cancelWriter struct {
	// buf keeps everything written, so it can be read back afterwards.
	buf bytes.Buffer

	// cancel ends the context the command is running under.
	cancel context.CancelFunc
}

// Write records the bytes and stops the command.
func (w *cancelWriter) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	w.cancel()
	return n, err
}

// failWriter is a stream that refuses everything written to it, which is how
// the write failures are reached without a real broken pipe.
type failWriter struct{}

// Write reports that the stream has gone.
func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("the pipe closed") }

// fakeSources installs a listing that answers with what it was given, and
// restores the real one at the end of the test.
//
// Parameters:
//   - t: the test the real listing is restored at the end of
//   - found: the sound inputs to report
//   - err: the failure to report instead, if there is one
func fakeSources(t *testing.T, found []audioin.Source, err error) {
	t.Helper()
	original := listSources
	t.Cleanup(func() { listSources = original })
	listSources = func() ([]audioin.Source, error) { return found, err }
}

// newApp returns an App writing to buffers, along with the buffers, so a
// command's output and its advice can be read back separately.
func newApp() (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, notes := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, notes
	return app, out, notes
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (2 test cases covering the command and the closure it holds)
//
// Test cases:
//   - Wiring: the command carries its name and the feed subcommand
//   - Runs: executing the command lists the sound inputs
func TestNew(t *testing.T) {
	// Verify that the command is described the way the tool wires it
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "audio" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "audio")
		}

		var found bool
		for _, sub := range cmd.Commands() {
			if sub.Use == "feed" {
				found = true
			}
		}
		if !found {
			t.Error("the command has no feed subcommand, wanted one")
		}
	})

	// Verify that running the command reaches run, which is what the closure
	// New hands cobra exists to do
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := newApp()
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)

		cmd := New(app)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "Cubilux CB5 Line In") {
			t.Errorf("the listing is %q, wanted the sound input in it", out.String())
		}
	})
}

// Test_renderSources tests the renderSources function with 100% coverage.
//
// Coverage: 100% (3 test cases covering the empty listing, a full one and the failure)
//
// Test cases:
//   - Empty: an empty listing is still a table, with its heading
//   - Several: every sound input given appears, in the order given
//   - WriteError: a stream that refuses the table is reported
func Test_renderSources(t *testing.T) {
	// Verify that nothing to list still prints the heading
	t.Run("Empty", func(t *testing.T) {
		app, out, _ := newApp()

		if err := renderSources(app, nil); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if got := out.String(); got != "NAME\n" {
			t.Errorf("the table is %q, wanted the heading alone", got)
		}
	})

	// Verify that the sound inputs are listed in the order they were given
	t.Run("Several", func(t *testing.T) {
		app, out, _ := newApp()

		err := renderSources(app, []audioin.Source{
			{Name: "Cubilux CB5 Line In"},
			{Name: "MacBook Pro Microphone"},
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		first, second := strings.Index(got, "Cubilux"), strings.Index(got, "MacBook")
		if first < 0 || second < 0 {
			t.Fatalf("the table is %q, wanted both sound inputs in it", got)
		}
		if first > second {
			t.Errorf("the table is %q, wanted the sound inputs in the order given", got)
		}
	})

	// Verify that a stream which cannot take the table says so
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		err := renderSources(app, []audioin.Source{{Name: "Cubilux CB5 Line In"}})
		if err == nil || !strings.Contains(err.Error(), "writing the sound input list") {
			t.Errorf("the failure is %v, wanted the sound input list to be named", err)
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every path through the listing)
//
// Test cases:
//   - Found: the sound inputs are listed, with nothing to advise
//   - None: nothing to record from is reported as the complete answer it is
//   - JSON: the listing is written as JSON when that is what was asked for
//   - SourcesError: a failed listing is passed back
//   - JSONWriteError: a stream that refuses the JSON is reported
//   - RenderError: a stream that refuses the table is reported
func Test_run(t *testing.T) {
	// Verify that the sound inputs found are listed and nothing is advised
	t.Run("Found", func(t *testing.T) {
		app, out, notes := newApp()
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)

		if err := run(app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(out.String(), "Cubilux CB5 Line In") {
			t.Errorf("the listing is %q, wanted the sound input in it", out.String())
		}
		if notes.String() != "" {
			t.Errorf("the advice is %q, wanted none", notes.String())
		}
	})

	// Verify that nothing to record from is reported as an answer, not a failure
	t.Run("None", func(t *testing.T) {
		app, out, notes := newApp()
		fakeSources(t, nil, nil)

		if err := run(app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if out.String() != "" {
			t.Errorf("the listing is %q, wanted nothing printed", out.String())
		}
		if !strings.Contains(notes.String(), "No sound inputs found") {
			t.Errorf("the advice is %q, wanted it to say nothing was found", notes.String())
		}
	})

	// Verify that JSON is written when that is the format asked for
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)

		if err := run(app); err != nil {
			t.Fatalf("running: %v", err)
		}

		var found []audioin.Source
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("reading the JSON back: %v", err)
		}
		if len(found) != 1 || found[0].Name != "Cubilux CB5 Line In" {
			t.Errorf("the listing is %+v, wanted the one sound input", found)
		}
	})

	// Verify that a listing which failed is passed back rather than swallowed
	t.Run("SourcesError", func(t *testing.T) {
		app, _, _ := newApp()
		fakeSources(t, nil, errors.New("the sound library is not there"))

		err := run(app)
		if err == nil || !strings.Contains(err.Error(), "the sound library is not there") {
			t.Errorf("the failure is %v, wanted the listing's own error", err)
		}
	})

	// Verify that a stream which cannot take the JSON says so
	t.Run("JSONWriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		app.Config.Output = appcontext.OutputJSON
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)

		err := run(app)
		if err == nil || !strings.Contains(err.Error(), "the pipe closed") {
			t.Errorf("the failure is %v, wanted the stream's own error", err)
		}
	})

	// Verify that a stream which cannot take the table says so
	t.Run("RenderError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)

		err := run(app)
		if err == nil || !strings.Contains(err.Error(), "writing the sound input list") {
			t.Errorf("the failure is %v, wanted the sound input list to be named", err)
		}
	})
}
