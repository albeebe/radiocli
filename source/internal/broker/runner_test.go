// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package broker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// TestRead tests the eofReader.Read method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: a read ends at once rather than waiting for an answer
func TestRead(t *testing.T) {
	// Verify that a read ends at once rather than waiting for an answer.
	t.Run("Success", func(t *testing.T) {
		n, err := eofReader{}.Read(make([]byte, 8))
		if n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("reading a stdin with nobody behind it gave %d and %v", n, err)
		}
	})
}

// Test_run tests the runner.run method with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - NoCommand: a run with nothing to run is refused
//   - Interrupted: a daemon interrupted while the run waited gives up
//   - Success: the command's output goes to this caller and the App is put back
//   - Failed: the command's own error comes back
//   - Panicked: a command that panicked is reported like any other failure
//   - MacrosSurviveTheCommand: a command writing into a macro does not change
//     the daemon's own settings
func Test_run(t *testing.T) {
	// Verify that a run with nothing to run is refused.
	t.Run("NoCommand", func(t *testing.T) {
		r := runner{app: daemonApp(t, nil)}

		err := r.run(context.Background(), nil, io.Discard, io.Discard)
		if err == nil || err.Error() != "no command given" {
			t.Fatalf("a run with nothing to run gave %v", err)
		}
	})

	// Verify that a daemon interrupted while the run waited gives up.
	t.Run("Interrupted", func(t *testing.T) {
		r := runner{app: daemonApp(t, nil)}

		ctx, stop := context.WithCancel(context.Background())
		stop()

		if err := r.run(ctx, []string{"version"}, io.Discard, io.Discard); !errors.Is(err, context.Canceled) {
			t.Fatalf("an interrupted run gave %v", err)
		}
	})

	// Verify that the command's output goes to this caller and the App is put back.
	t.Run("Success", func(t *testing.T) {
		var app = daemonApp(t, nil)
		app.RunCommand = func(ctx context.Context, argv []string) error {
			io.WriteString(app.Stdout, strings.Join(argv, " "))
			io.WriteString(app.Stderr, "a note")

			if _, err := app.Stdin.Read(make([]byte, 4)); !errors.Is(err, io.EOF) {
				t.Errorf("a command reading stdin got %v", err)
			}
			return nil
		}

		savedOut, savedErr := app.Stdout, app.Stderr
		r := runner{app: app}

		var out, errs bytes.Buffer
		if err := r.run(context.Background(), []string{"key", ">"}, &out, &errs); err != nil {
			t.Fatalf("running a command: %v", err)
		}
		if out.String() != "key >" || errs.String() != "a note" {
			t.Fatalf("the command wrote %q and %q", out.String(), errs.String())
		}
		if app.Stdout != savedOut || app.Stderr != savedErr {
			t.Fatal("the App was left pointed at the caller after the command ended")
		}
	})

	// Verify that the command's own error comes back.
	t.Run("Failed", func(t *testing.T) {
		r := runner{app: daemonApp(t, func(ctx context.Context, argv []string) error {
			return errors.New("the scanner refused the key")
		})}

		err := r.run(context.Background(), []string{"key", ">"}, io.Discard, io.Discard)
		if err == nil || err.Error() != "the scanner refused the key" {
			t.Fatalf("a failed command gave %v", err)
		}
	})

	// Verify that a command that panicked is reported like any other failure.
	t.Run("Panicked", func(t *testing.T) {
		r := runner{app: daemonApp(t, func(ctx context.Context, argv []string) error {
			panic("the port is gone")
		})}

		err := r.run(context.Background(), []string{"version"}, io.Discard, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "the command panicked: the port is gone") {
			t.Fatalf("a command that panicked gave %v", err)
		}
	})

	// Verify that a command writing into a macro does not change the daemon's
	// own settings. The save and restore around a run used to copy the struct,
	// which shares the macros rather than copying them, so a write through one
	// element outlived the command that made it and every later client saw it.
	t.Run("MacrosSurviveTheCommand", func(t *testing.T) {
		app := daemonApp(t, nil)
		app.Config.Macros = []appcontext.Macro{{Name: "listen", Steps: []string{"scan"}}}
		app.RunCommand = func(ctx context.Context, argv []string) error {
			app.Config.Macros[0].Name = "changed"
			app.Config.Macros[0].Steps[0] = "tune 162.550"
			return nil
		}

		r := runner{app: app}
		if err := r.run(context.Background(), []string{"config", "macro"}, io.Discard, io.Discard); err != nil {
			t.Fatalf("running a command: %v", err)
		}

		if app.Config.Macros[0].Name != "listen" {
			t.Errorf("the daemon's macro is now named %q: the command's change outlived it", app.Config.Macros[0].Name)
		}
		if app.Config.Macros[0].Steps[0] != "scan" {
			t.Errorf("the daemon's first step is now %q: the command's change outlived it", app.Config.Macros[0].Steps[0])
		}
	})
}
