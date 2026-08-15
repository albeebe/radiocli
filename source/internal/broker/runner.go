// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package broker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// Read reports end of input immediately.
//
// Parameters:
//   - the buffer to fill, which is left untouched
//
// Returns:
//   - the number of bytes read, which is always zero
//   - io.EOF, always
func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

// execute runs the command and turns a panic into an error.
//
// A panic used to be somebody else's problem, because each command was its own
// process. In this one it would take the daemon down and every other client
// with it, so it is caught here and reported like any other failure.
//
// Parameters:
//   - ctx: the run's own lifetime, which the command answers to
//   - app: the App the command runs against, already pointed at the caller
//   - argv: the already split argument list to run
//
// Returns:
//   - error the command's own error, or the panic it was stopped by
func execute(ctx context.Context, app *appcontext.App, argv []string) (err error) {
	defer func() {
		if p := recover(); p != nil {
			app.Log.Error("command panicked", "panic", p, "stack", string(debug.Stack()))
			err = fmt.Errorf("the command panicked: %v", p)
		}
	}()

	return app.RunCommand(ctx, argv)
}

// run executes argv and writes what it produces to stdout and stderr.
//
// The caller must already hold the scanner. Two commands in here at once would
// read each other's replies off the serial line and write into each other's
// output, which is the whole reason the scheduler exists.
//
// It returns the command's own error, so a proxied caller can report exactly
// what the terminal would have reported.
//
// Parameters:
//   - ctx: the run's own lifetime, which the command answers to
//   - argv: the already split argument list to run
//   - stdout: where the command's stdout goes for the length of the command
//   - stderr: where the command's stderr goes for the length of the command
//
// Returns:
//   - error the command's own error, the context's if the daemon was
//     interrupted while waiting, or a refusal if there was no command at all
func (r *runner) run(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	if len(argv) == 0 {
		return errors.New("no command given")
	}

	// Waiting for a turn may have taken a while, and the daemon may have been
	// interrupted in the meantime.
	if err := ctx.Err(); err != nil {
		return err
	}

	app := r.app

	// The command tree binds to these as it is built, and Init rebuilds the
	// logger from Stderr, so all four have to be in place before the command
	// runs and back afterwards. Restoring is what keeps the terminal that
	// started the daemon usable once the command is done.
	savedOut, savedErr, savedIn, savedLog := app.Stdout, app.Stderr, app.Stdin, app.Log

	// Clone rather than copy the struct: the macros are slices, and a plain
	// copy would hand the command the daemon's own backing arrays to write
	// through, which is a change that outlives the restore below.
	savedConfig := app.Config.Clone()
	defer func() {
		app.Stdout, app.Stderr, app.Stdin, app.Log = savedOut, savedErr, savedIn, savedLog
		*app.Config = *savedConfig
	}()

	app.Stdout, app.Stderr = stdout, stderr

	// Nothing can answer a prompt over a socket, so reads end immediately
	// rather than hanging. No command reads stdin today, so this is a guard
	// against one that starts to rather than something any of them rely on.
	app.Stdin = eofReader{}

	return execute(ctx, app, argv)
}
