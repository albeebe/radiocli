// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package broker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// withPeek gives a fake daemon a reader, standing in for the one main builds.
// The reader here allows exactly one command, which is all a test needs to tell
// allowed from refused.
func withPeek(d *fakeDaemon, allow string, run func(argv []string) error) {
	app := appcontext.New()
	app.Config = appcontext.Defaults()
	app.Log = slog.New(slog.DiscardHandler)
	app.Stdout, app.Stderr = io.Discard, io.Discard
	app.RunCommand = func(ctx context.Context, argv []string) error {
		if len(argv) == 0 || argv[0] != allow {
			return errAllowed
		}
		return run(argv)
	}

	d.srv.peek = &runner{app: app}
}

// errAllowed stands in for the refusal a reader gives a command that could move
// the scanner. Its wording belongs to main; only that there is one matters here.
var errAllowed = errRefused{}

type errRefused struct{}

func (errRefused) Error() string { return "that command can move the scanner" }

// TestAPeekRunsWhileTheScannerIsHeld is the whole point of the mode.
//
// A mirror that stands aside for commands shows a frozen screen for exactly as
// long as something is happening on it, which is the moment it is worth most.
func TestAPeekRunsWhileTheScannerIsHeld(t *testing.T) {
	holding := make(chan struct{})
	release := make(chan struct{})

	d, in, send, cleanup := serveFake(t, func(ctx context.Context, argv []string) error {
		close(holding)
		<-release
		return nil
	})
	defer cleanup()

	withPeek(d, "screen", func([]string) error { return nil })

	// A long command takes the scanner and keeps it. The started message has to
	// be taken off the connection before the command body runs, because the
	// pipe under these tests is synchronous.
	send(Request{Op: OpRun, ID: "long", Argv: []string{"banks", "--full"}})
	waitFor(t, in, TypeStarted)

	select {
	case <-holding:
	case <-time.After(5 * time.Second):
		t.Fatal("the long command never started")
	}
	defer close(release)

	// A second connection, because the first is busy with its own run.
	watcher, watch, closeWatcher := d.connect(t)
	defer closeWatcher()

	watch(Request{Op: OpRun, ID: "peek", Argv: []string{"screen"}, Mode: ModePeek})

	done := waitFor(t, watcher, TypeDone)
	if done.Code != 0 || done.Error != "" {
		t.Fatalf("a peek during a command failed: code=%d error=%q", done.Code, done.Error)
	}

	// And the queued path is still queued, so the peek is not a hole in the
	// scheduler for everything else.
	watch(Request{Op: OpRun, ID: "queued", Argv: []string{"screen"}, Mode: ModeTry})
	if got := waitFor(t, watcher, TypeSkipped); got.ID != "queued" {
		t.Fatalf("a try during a command was answered with %q", got.Type)
	}

}

// TestAPeekRefusesACommandThatCouldMoveTheScanner checks the refusal arrives as
// an ordinary failure rather than the command being quietly queued instead.
//
// Queueing it would be worse than refusing: the caller asked not to wait, and a
// key press that arrives whenever the queue drains is a key press into whatever
// screen is showing by then.
func TestAPeekRefusesACommandThatCouldMoveTheScanner(t *testing.T) {
	d, _, _, cleanup := serveFake(t, func(ctx context.Context, argv []string) error { return nil })
	defer cleanup()

	withPeek(d, "screen", func([]string) error { return nil })

	watcher, watch, closeWatcher := d.connect(t)
	defer closeWatcher()

	watch(Request{Op: OpRun, ID: "1", Argv: []string{"key", "1"}, Mode: ModePeek})

	done := waitFor(t, watcher, TypeDone)
	if done.Code == 0 {
		t.Fatal("a command that can move the scanner was allowed to peek")
	}
	if done.Error == "" {
		t.Error("the refusal carried no reason")
	}
}

// TestAPeekIsRefusedWhenTheDaemonHasNoReader covers a daemon built without one.
//
// Saying so beats queueing it, for the same reason as above: the caller asked
// for something this daemon cannot do, and finding out at once is what lets it
// fall back.
func TestAPeekIsRefusedWhenTheDaemonHasNoReader(t *testing.T) {
	_, in, send, cleanup := serveFake(t, func(ctx context.Context, argv []string) error { return nil })
	defer cleanup()

	send(Request{Op: OpRun, ID: "1", Argv: []string{"screen"}, Mode: ModePeek})

	done := waitFor(t, in, TypeDone)
	if done.Code == 0 {
		t.Fatal("a peek was accepted by a daemon with no reader")
	}
}
