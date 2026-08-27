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
	"github.com/albeebe/radiocli/internal/audioout"
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

// fakeSinks installs a speaker listing that answers with what it was given, and
// restores the real one at the end of the test.
//
// Parameters:
//   - t: the test the real listing is restored at the end of
//   - found: the speakers to report
//   - err: the failure to report instead, if there is one
func fakeSinks(t *testing.T, found []audioout.Sink, err error) {
	t.Helper()
	original := listSinks
	t.Cleanup(func() { listSinks = original })
	listSinks = func() ([]audioout.Sink, error) { return found, err }
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
//   - Wiring: the command carries its name and every subcommand
//   - Runs: executing the command lists the sound inputs
func TestNew(t *testing.T) {
	// Verify that the command is described the way the tool wires it
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "audio" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "audio")
		}

		for _, want := range []string{"listen", "output", "record [destination]"} {
			var found bool
			for _, sub := range cmd.Commands() {
				if sub.Use == want {
					found = true
				}
			}
			if !found {
				t.Errorf("the command has no %q subcommand, wanted one", want)
			}
		}
	})

	// Verify that running the command reaches run, which is what the closure
	// New hands cobra exists to do
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := newApp()
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)
		fakeSinks(t, []audioout.Sink{{Name: "MacBook Pro Speakers"}}, nil)

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

// Test_advise tests the advise function with 100% coverage.
//
// Coverage: 100% (4 test cases covering both branches)
//
// Test cases:
//   - Both: a computer with inputs and speakers is advised of nothing
//   - NoInputs: nothing to record from is said
//   - NoSpeakers: nowhere to play is said separately
//   - Neither: a computer with no sound at all hears about both
func Test_advise(t *testing.T) {
	// Verify that a machine with both halves is left alone, since there is
	// nothing to fix.
	t.Run("Both", func(t *testing.T) {
		app, _, notes := newApp()

		advise(app, listing{
			Inputs:  []audioin.Source{{Name: "Cubilux CB5 Line In"}},
			Outputs: []audioout.Sink{{Name: "MacBook Pro Speakers"}},
		})

		if notes.String() != "" {
			t.Errorf("the advice is %q, wanted none", notes.String())
		}
	})

	// Verify that nothing to record from is said, and that having speakers does
	// not stand in for it.
	t.Run("NoInputs", func(t *testing.T) {
		app, _, notes := newApp()

		advise(app, listing{Outputs: []audioout.Sink{{Name: "MacBook Pro Speakers"}}})

		if !strings.Contains(notes.String(), "No sound inputs found") {
			t.Errorf("the advice is %q, wanted it to say nothing can be recorded from", notes.String())
		}
		if strings.Contains(notes.String(), "No speakers found") {
			t.Errorf("the advice is %q, wanted nothing said about the speakers", notes.String())
		}
	})

	// Verify that nowhere to play is worth saying on its own. A headless box
	// with an interface plugged into a scanner is exactly this, and it is worth
	// knowing before somebody runs "audio listen" on it.
	t.Run("NoSpeakers", func(t *testing.T) {
		app, _, notes := newApp()

		advise(app, listing{Inputs: []audioin.Source{{Name: "Cubilux CB5 Line In"}}})

		if !strings.Contains(notes.String(), "No speakers found") {
			t.Errorf("the advice is %q, wanted it to say there is nowhere to play", notes.String())
		}
		if strings.Contains(notes.String(), "No sound inputs found") {
			t.Errorf("the advice is %q, wanted nothing said about the inputs", notes.String())
		}
	})

	// Verify that a machine with no sound at all is told about both halves
	// rather than only the first.
	t.Run("Neither", func(t *testing.T) {
		app, _, notes := newApp()

		advise(app, listing{})

		if !strings.Contains(notes.String(), "No sound inputs found") ||
			!strings.Contains(notes.String(), "No speakers found") {
			t.Errorf("the advice is %q, wanted both halves reported", notes.String())
		}
	})
}

// Test_renderListing tests the renderListing function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Empty: nothing at all prints nothing, since the advice says it instead
//   - Both: both tables appear, headed, with a blank line between them
//   - InputsOnly: the speakers table is left out rather than printed empty
//   - SpeakersOnly: the table stands alone, with nothing above to separate from
//   - WriteError: a stream that refuses the tables is reported
func Test_renderListing(t *testing.T) {
	// Verify that nothing found prints nothing. The headings alone would say a
	// listing happened and found something.
	t.Run("Empty", func(t *testing.T) {
		app, out, _ := newApp()

		if err := renderListing(app, listing{}); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if got := out.String(); got != "" {
			t.Errorf("the tables are %q, wanted nothing", got)
		}
	})

	// Verify that both halves are listed, in the order given, under headings
	// that say which is which.
	t.Run("Both", func(t *testing.T) {
		app, out, _ := newApp()

		err := renderListing(app, listing{
			Inputs: []audioin.Source{
				{Name: "Cubilux CB5 Line In"},
				{Name: "MacBook Pro Microphone"},
			},
			Outputs: []audioout.Sink{{Name: "MacBook Pro Speakers"}},
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "SOUND INPUTS") || !strings.Contains(got, "SPEAKERS") {
			t.Fatalf("the tables are %q, wanted both headings", got)
		}
		first, second := strings.Index(got, "Cubilux"), strings.Index(got, "MacBook Pro Microphone")
		if first < 0 || second < 0 || first > second {
			t.Errorf("the tables are %q, wanted the inputs in the order given", got)
		}
		if !strings.Contains(got, "\n\nSPEAKERS") {
			t.Errorf("the tables are %q, wanted a blank line between them", got)
		}
	})

	// Verify that a machine with nowhere to play prints no speakers heading,
	// rather than a heading with nothing under it.
	t.Run("InputsOnly", func(t *testing.T) {
		app, out, _ := newApp()

		err := renderListing(app, listing{Inputs: []audioin.Source{{Name: "Cubilux CB5 Line In"}}})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if got := out.String(); strings.Contains(got, "SPEAKERS") {
			t.Errorf("the tables are %q, wanted no speakers heading", got)
		}
	})

	// Verify that the speakers table stands on its own without a blank line
	// leading it, since there is nothing above it to be separated from.
	t.Run("SpeakersOnly", func(t *testing.T) {
		app, out, _ := newApp()

		err := renderListing(app, listing{Outputs: []audioout.Sink{{Name: "MacBook Pro Speakers"}}})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		if strings.Contains(got, "SOUND INPUTS") {
			t.Errorf("the tables are %q, wanted no inputs heading", got)
		}
		if !strings.HasPrefix(got, "SPEAKERS") {
			t.Errorf("the tables are %q, wanted them to start at the heading", got)
		}
	})

	// Verify that a stream which cannot take the tables says so
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		err := renderListing(app, listing{Inputs: []audioin.Source{{Name: "Cubilux CB5 Line In"}}})
		if err == nil || !strings.Contains(err.Error(), "writing the sound device list") {
			t.Errorf("the failure is %v, wanted the listing to be named", err)
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (7 test cases covering every path through the listing)
//
// Test cases:
//   - Found: both halves are listed, with nothing to advise
//   - None: no sound at all is reported as the complete answer it is
//   - JSON: the listing is written as one object with both halves named
//   - SourcesError: a failed input listing is passed back
//   - SinksError: a failed speaker listing is passed back
//   - JSONWriteError: a stream that refuses the JSON is reported
//   - RenderError: a stream that refuses the tables is reported
func Test_run(t *testing.T) {
	// Verify that what was found is listed and nothing is advised
	t.Run("Found", func(t *testing.T) {
		app, out, notes := newApp()
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)
		fakeSinks(t, []audioout.Sink{{Name: "MacBook Pro Speakers"}}, nil)

		if err := run(app); err != nil {
			t.Fatalf("running: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "Cubilux CB5 Line In") || !strings.Contains(got, "MacBook Pro Speakers") {
			t.Errorf("the listing is %q, wanted both halves in it", got)
		}
		if notes.String() != "" {
			t.Errorf("the advice is %q, wanted none", notes.String())
		}
	})

	// Verify that a machine with no sound at all is an answer, not a failure
	t.Run("None", func(t *testing.T) {
		app, out, notes := newApp()
		fakeSources(t, nil, nil)
		fakeSinks(t, nil, nil)

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

	// Verify that JSON is one object naming both halves, so that a reader
	// asking for the speakers does not have to filter a mixed list. The empty
	// halves are arrays rather than null, so nothing has to special-case a
	// machine with none.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON
		fakeSources(t, nil, nil)
		fakeSinks(t, []audioout.Sink{{Name: "MacBook Pro Speakers"}}, nil)

		if err := run(app); err != nil {
			t.Fatalf("running: %v", err)
		}

		var found listing
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("reading the JSON back: %v", err)
		}
		if len(found.Outputs) != 1 || found.Outputs[0].Name != "MacBook Pro Speakers" {
			t.Errorf("the listing is %+v, wanted the one speaker", found)
		}
		if !strings.Contains(out.String(), `"inputs": []`) {
			t.Errorf("the JSON is %q, wanted an empty array rather than null", out.String())
		}
	})

	// Verify that an input listing which failed is passed back rather than
	// swallowed
	t.Run("SourcesError", func(t *testing.T) {
		app, _, _ := newApp()
		fakeSources(t, nil, errors.New("the sound library is not there"))
		fakeSinks(t, nil, nil)

		err := run(app)
		if err == nil || !strings.Contains(err.Error(), "the sound library is not there") {
			t.Errorf("the failure is %v, wanted the listing's own error", err)
		}
	})

	// Verify that a speaker listing which failed is passed back too, which is
	// what a build made without audio support produces
	t.Run("SinksError", func(t *testing.T) {
		app, _, _ := newApp()
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)
		fakeSinks(t, nil, errors.New("built without audio support"))

		err := run(app)
		if err == nil || !strings.Contains(err.Error(), "built without audio support") {
			t.Errorf("the failure is %v, wanted the listing's own error", err)
		}
	})

	// Verify that a stream which cannot take the JSON says so
	t.Run("JSONWriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		app.Config.Output = appcontext.OutputJSON
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)
		fakeSinks(t, nil, nil)

		err := run(app)
		if err == nil || !strings.Contains(err.Error(), "the pipe closed") {
			t.Errorf("the failure is %v, wanted the stream's own error", err)
		}
	})

	// Verify that a stream which cannot take the tables says so
	t.Run("RenderError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		fakeSources(t, []audioin.Source{{Name: "Cubilux CB5 Line In"}}, nil)
		fakeSinks(t, nil, nil)

		err := run(app)
		if err == nil || !strings.Contains(err.Error(), "writing the sound device list") {
			t.Errorf("the failure is %v, wanted the listing to be named", err)
		}
	})
}
