// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package version

import (
	"bytes"
	"encoding/json"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/buildinfo"
)

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: running the command renders the build information
//   - OnlyReads: the command is marked as one that cannot move the scanner
func TestNew(t *testing.T) {
	// Build an App whose streams are buffers, so a test never writes to the
	// terminal and nothing reads from it.
	newApp := func() (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Stdin = &bytes.Buffer{}
		return app, out
	}

	// Verify that executing the command writes the build information
	t.Run("Success", func(t *testing.T) {
		app, out := newApp()

		cmd := New(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs(nil)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if !strings.Contains(out.String(), buildinfo.Version) {
			t.Errorf("expected the version in the output, got: %q", out.String())
		}
	})

	// Verify that the command says it only reads, so it may run alongside
	// another command that has the radio
	t.Run("OnlyReads", func(t *testing.T) {
		app, _ := newApp()

		cmd := New(app)

		if cmd.Annotations[appcontext.OnlyReads] != "true" {
			t.Errorf("expected the %s annotation, got: %v", appcontext.OnlyReads, cmd.Annotations)
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Text: the human-readable form is written to Stdout
//   - JSON: the machine-readable form is written to Stdout
//   - JSONWriteError: the stream cannot be written to
func Test_run(t *testing.T) {
	// Build an App whose streams are buffers, so a test never writes to the
	// terminal and nothing reads from it.
	newApp := func() (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Stdin = &bytes.Buffer{}
		return app, out
	}

	// Verify that the text form reports the build and the runtime
	t.Run("Text", func(t *testing.T) {
		app, out := newApp()
		app.Config.Output = appcontext.OutputText

		if err := run(app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		for _, want := range []string{
			"radiocli " + buildinfo.Version,
			buildinfo.Commit,
			buildinfo.Date,
			runtime.Version(),
			runtime.GOOS,
			runtime.GOARCH,
		} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected %q in the output, got: %q", want, out.String())
			}
		}
	})

	// Verify that the JSON form carries every field under its own name
	t.Run("JSON", func(t *testing.T) {
		app, out := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := run(app); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got info
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("parsing the output: %v", err)
		}

		want := info{
			Version: buildinfo.Version,
			Commit:  buildinfo.Commit,
			Date:    buildinfo.Date,
			Go:      runtime.Version(),
			OS:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		}
		if got != want {
			t.Errorf("expected %+v, got %+v", want, got)
		}
	})

	// Verify that a stream which cannot be written to is reported rather than
	// ignored
	t.Run("JSONWriteError", func(t *testing.T) {
		app, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		// A pipe whose read end is closed fails every write, which is the
		// closest a test gets to a broken stdout.
		reader, writer := io.Pipe()
		if err := reader.Close(); err != nil {
			t.Fatalf("closing the pipe: %v", err)
		}
		app.Stdout = writer

		if err := run(app); err == nil {
			t.Fatal("expected an error writing to a closed stream")
		}
	})
}
