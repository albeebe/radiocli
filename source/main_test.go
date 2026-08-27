// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/commands/root"
	"github.com/albeebe/radiocli/internal/portlock"
	"github.com/spf13/cobra"
)

// Test_onlyReads tests the onlyReads function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - RefusedByDefault: anything unmarked cannot share the scanner
//   - AllowsTheReads: the commands a mirror needs are allowed
//   - FollowsTheFlagsForColors: a command whose answer depends on its flags
//   - RefusesALineThatWillNotParse: unparseable flags are refused, not guessed at
func Test_onlyReads(t *testing.T) {
	// tree builds the command tree the way run does, so these tests ask the
	// real commands rather than a copy of what they are believed to say.
	tree := func() *cobra.Command {
		app := appcontext.New()
		cmd := root.New(app)
		for _, newCmd := range commands {
			cmd.AddCommand(newCmd(app))
		}
		return cmd
	}

	// asks resolves a command line the way the reader does and reports whether
	// it would be allowed to run alongside another command.
	asks := func(t *testing.T, line string) bool {
		t.Helper()

		argv := strings.Fields(line)
		found, rest, err := tree().Find(argv)
		if err != nil {
			t.Fatalf("resolving %q: %v", line, err)
		}
		return onlyReads(found, rest)
	}

	// Verify the rule that makes the rest of this safe. A command added later
	// must not become able to run alongside a menu walk because nobody thought
	// about it. Refusing anything unmarked is what makes forgetting the safe
	// mistake rather than the dangerous one.
	t.Run("RefusedByDefault", func(t *testing.T) {
		for _, line := range []string{
			"key 1",
			"tune 154.415",
			"scan",
			"weather",
			"banks",
			"banks scan 8",
			"banks set 8 --name x",
			"favorites",
			"systems 1",
			"clock sync",
			"backlight on",
			"location gps",
			"menu open top",
			"menu close",
			"volume set 5",
			"squelch set 4",
			"display mode color",
			"colors reset",
			"colors set System_name --text White",
		} {
			if asks(t, line) {
				t.Errorf("%q is allowed to run alongside another command, and it can move the scanner", line)
			}
		}
	})

	// Verify the commands a mirror needs are allowed, which are the reason the
	// whole mechanism exists.
	t.Run("AllowsTheReads", func(t *testing.T) {
		for _, line := range []string{
			"screen",
			"screen -o json",
			"display",
			"menu",
			"status",
			"battery",
			"volume",
			"squelch",
			"version",
		} {
			if !asks(t, line) {
				t.Errorf("%q cannot run alongside another command, and it only reads", line)
			}
		}
	})

	// Verify the case that needed a second mechanism, and the one that broke
	// the display mirror when it was got wrong. Reading a layout's colors walks
	// every one of its color pickers, and asking for the stored answer instead
	// opens nothing. Marking the command either way is wrong: one way lets the
	// walk run alongside somebody else's, and the other froze the mirror's
	// colors for the length of every command, which showed as a new screen
	// drawn in an old screen's colors.
	t.Run("FollowsTheFlagsForColors", func(t *testing.T) {
		allowed := []string{
			"colors --cache",
			"colors --cache -o json",
			"colors --positions",
			"colors --positions -o json",
		}
		for _, line := range allowed {
			if !asks(t, line) {
				t.Errorf("%q cannot run alongside another command, and it opens no menus", line)
			}
		}

		refused := []string{
			"colors",
			"colors weather",
			"colors -o json",
			"colors --verify-positions",
			"colors --verify-palette",
		}
		for _, line := range refused {
			if asks(t, line) {
				t.Errorf("%q is allowed to run alongside another command, and it walks the menus", line)
			}
		}
	})

	// Verify the last branch of onlyReads, which is a command whose answer
	// depends on its flags being handed flags that do not parse. Guessing is
	// the wrong answer here: the same line fails a moment later when it runs,
	// with the message the terminal would have given.
	t.Run("RefusesALineThatWillNotParse", func(t *testing.T) {
		argv := []string{"colors", "--cache", "--nosuchflag"}
		found, rest, err := tree().Find(argv)
		if err != nil {
			t.Fatalf("resolving %q: %v", argv, err)
		}
		if onlyReads(found, rest) {
			t.Error("a line that will not parse was allowed to run alongside another command")
		}
	})
}

// program points the program at the test's own config file and streams, so a
// run reads no file but this one and writes to buffers rather than the
// terminal. It reports the config file's path and where the run wrote.
func program(t *testing.T, prepare func(*appcontext.App), argv ...string) (string, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	oldApp := newApp
	newApp = func() *appcontext.App {
		app := appcontext.New()
		app.Stdout, app.Stderr = stdout, stderr
		if prepare != nil {
			prepare(app)
		}
		return app
	}
	t.Cleanup(func() { newApp = oldApp })

	line := append([]string{"radiocli"}, argv...)
	line = append(line, "--config", path)
	oldArgs := os.Args
	os.Args = line
	t.Cleanup(func() { os.Args = oldArgs })

	return path, stdout, stderr
}

// tempHome puts the lock and socket files this program uses inside the test's
// own directory, and reports the port name to use with them.
//
// It does it by name rather than by path on purpose. A unix socket path is
// capped near 104 bytes on macOS and a temporary directory's own path already
// spends most of that, so the test runs from inside its directory and asks for
// the files to be kept in ".", which is short whatever the directory is called.
func tempHome(t *testing.T) string {
	t.Helper()

	t.Chdir(t.TempDir())
	t.Setenv("TMPDIR", ".")
	return "/dev/tty.test"
}

// fakeDaemon stands in for an radiocli daemon holding a scanner. It speaks the
// broker protocol on the socket the real client dials, so a proxied run is
// carried by the real client over a real socket with no radio behind it.
//
// The argument list it received is handed back on a channel rather than left
// in a field, so a test reads it only once the daemon is done writing it.
func fakeDaemon(t *testing.T, port string, answers ...broker.Response) <-chan []string {
	t.Helper()

	path := portlock.SocketPath(port)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making the daemon socket directory: %v", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listening on the daemon socket: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	argv := make(chan []string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		out := json.NewEncoder(conn)
		out.Encode(broker.Response{Type: broker.TypeHello, Version: "test", Protocol: broker.Version})

		in := bufio.NewScanner(conn)
		for in.Scan() {
			var req broker.Request
			if err := json.Unmarshal(in.Bytes(), &req); err != nil || req.Op != broker.OpRun {
				continue
			}
			argv <- req.Argv
			for _, answer := range answers {
				answer.ID = req.ID
				out.Encode(answer)
			}
			return
		}
	}()
	return argv
}

// Test_main tests the main function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the run's exit code is handed to exit
func Test_main(t *testing.T) {
	// Verify that main runs the program and exits with the code run reported.
	t.Run("Success", func(t *testing.T) {
		_, stdout, _ := program(t, nil, "version")

		code, called := 1, false
		old := exit
		exit = func(c int) { code, called = c, true }
		t.Cleanup(func() { exit = old })

		main()

		if !called {
			t.Fatal("main returned without exiting")
		}
		if code != 0 {
			t.Errorf("exited with %d, and the command succeeded", code)
		}
		if stdout.Len() == 0 {
			t.Error("the version command printed nothing")
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - Success: a command that succeeds gives an exit code of zero
//   - CommandFailed: a failing command is reported and gives an exit code of one
//   - ShutdownFailed: a closer that fails is reported
//   - RanOnDaemon: a busy scanner sends the invocation to the daemon holding it
//   - RunCommand: a command can run another command in this process
//   - Commands: the command list is read from the tree that was built
//   - Reader: a reader runs what only reads and refuses everything else
func Test_run(t *testing.T) {
	// Verify that a command that succeeds gives an exit code of zero.
	t.Run("Success", func(t *testing.T) {
		_, stdout, stderr := program(t, nil, "version")

		if code := run(); code != 0 {
			t.Errorf("exited with %d, and the command succeeded", code)
		}
		if stdout.Len() == 0 {
			t.Error("the version command printed nothing")
		}
		if stderr.Len() != 0 {
			t.Errorf("the command complained: %q", stderr.String())
		}
	})

	// Verify that a failing command is reported and gives an exit code of one.
	t.Run("CommandFailed", func(t *testing.T) {
		_, _, stderr := program(t, nil, "nosuchcommand")

		if code := run(); code != 1 {
			t.Errorf("exited with %d, and the command failed", code)
		}
		if !strings.Contains(stderr.String(), "error:") {
			t.Errorf("the failure was not reported: %q", stderr.String())
		}
	})

	// Verify that a closer that fails on the way out is reported.
	t.Run("ShutdownFailed", func(t *testing.T) {
		prepare := func(app *appcontext.App) {
			app.OnClose(func() error { return errors.New("the port is gone") })
		}
		_, _, stderr := program(t, prepare, "version")

		run()

		if !strings.Contains(stderr.String(), "shutting down") {
			t.Errorf("the failed shutdown was not reported: %q", stderr.String())
		}
	})

	// Verify that a scanner held by somebody else sends the invocation to the
	// daemon holding it, and that the exit code comes back from there.
	t.Run("RanOnDaemon", func(t *testing.T) {
		port := tempHome(t)

		lock, err := portlock.Acquire(port, 0)
		if err != nil {
			t.Fatalf("holding the port: %v", err)
		}
		t.Cleanup(func() { lock.Release() })

		argv := fakeDaemon(t, port,
			broker.Response{Type: broker.TypeStdout, Data: "7.4 volts\n"},
			broker.Response{Type: broker.TypeDone, Code: 3},
		)
		_, stdout, _ := program(t, nil, "battery", "--device", port)

		if code := run(); code != 3 {
			t.Errorf("exited with %d, and the daemon reported 3", code)
		}
		if !strings.Contains(stdout.String(), "7.4 volts") {
			t.Errorf("what the daemon printed did not arrive: %q", stdout.String())
		}
		if got := <-argv; len(got) == 0 || got[0] != "battery" {
			t.Errorf("the daemon was asked to run %q, and the line was battery", got)
		}
	})

	// Verify that a command can run another command in this process.
	t.Run("RunCommand", func(t *testing.T) {
		var app *appcontext.App
		prepare := func(built *appcontext.App) { app = built }
		path, stdout, _ := program(t, prepare, "version")

		run()

		stdout.Reset()
		if err := app.RunCommand(context.Background(), []string{"version", "--config", path}); err != nil {
			t.Fatalf("running a command in this process: %v", err)
		}
		if stdout.Len() == 0 {
			t.Error("the command that was run printed nothing")
		}
	})

	// Verify that the command list is read from a tree that was really built,
	// so a command added to the slice appears in it.
	t.Run("Commands", func(t *testing.T) {
		var app *appcontext.App
		prepare := func(built *appcontext.App) { app = built }
		program(t, prepare, "version")

		run()

		described := app.Commands()
		if len(described) == 0 {
			t.Fatal("the tree described no commands")
		}
		found := false
		for _, cmd := range described {
			if strings.HasPrefix(cmd.Name, "battery") {
				found = true
			}
		}
		if !found {
			t.Errorf("battery is in the command list and was not described: %+v", described)
		}
	})

	// Verify that a reader runs what only reads, and refuses a line that could
	// move the scanner or that names no command at all.
	t.Run("Reader", func(t *testing.T) {
		var app *appcontext.App
		prepare := func(built *appcontext.App) { app = built }
		path, stdout, _ := program(t, prepare, "version")

		run()

		reader := app.Reader()
		if reader == nil {
			t.Fatal("no reader was handed back")
		}

		err := reader.RunCommand(context.Background(), []string{"nosuchcommand"})
		if err == nil {
			t.Error("a line naming no command was accepted")
		}

		err = reader.RunCommand(context.Background(), []string{"key", "1"})
		if err == nil || !strings.Contains(err.Error(), "can move the scanner") {
			t.Errorf("pressing a key alongside another command was allowed: %v", err)
		}

		stdout.Reset()
		if err := reader.RunCommand(context.Background(), []string{"version", "--config", path}); err != nil {
			t.Fatalf("reading alongside another command: %v", err)
		}
		if stdout.Len() == 0 {
			t.Error("the command the reader ran printed nothing")
		}
	})
}

// Test_viaDaemon tests the viaDaemon function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NotBusy: an ordinary failure is left alone
//   - AlreadySpoke: a command that has already printed is not run again
//   - NoDaemon: a busy scanner with nobody sharing it is left alone
//   - Ran: the invocation is carried by the daemon and its code comes back
//   - RunFailed: a daemon that reports a failure has it printed
func Test_viaDaemon(t *testing.T) {
	// setup builds the pieces run holds, pointed at a port of the test's own.
	setup := func(t *testing.T, port string) (*appcontext.App, *witness, *witness, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		out, errs := &bytes.Buffer{}, &bytes.Buffer{}
		app := appcontext.New()
		app.Config.Device = port
		stdout, stderr := &witness{to: out}, &witness{to: errs}
		app.Stdout, app.Stderr = stdout, stderr
		return app, stdout, stderr, out, errs
	}

	// Verify that a failure that is not a busy scanner is left alone.
	t.Run("NotBusy", func(t *testing.T) {
		app, stdout, stderr, _, _ := setup(t, "/dev/tty.nothing")

		if _, ran := viaDaemon(context.Background(), app, errors.New("the port is gone"), stdout, stderr); ran {
			t.Error("an ordinary failure was sent to a daemon")
		}
	})

	// Verify that a command which already printed something is not run again,
	// since running it twice would say everything twice.
	t.Run("AlreadySpoke", func(t *testing.T) {
		app, stdout, stderr, _, _ := setup(t, "/dev/tty.nothing")
		stdout.Write([]byte("half an answer\n"))

		err := fmt.Errorf("opening the port: %w", portlock.ErrBusy)
		if _, ran := viaDaemon(context.Background(), app, err, stdout, stderr); ran {
			t.Error("a command that had already printed was run a second time")
		}
	})

	// Verify that a busy scanner with nobody sharing it is left alone, which
	// is the ordinary case of a second terminal being told to wait.
	t.Run("NoDaemon", func(t *testing.T) {
		app, stdout, stderr, _, _ := setup(t, tempHome(t))

		err := fmt.Errorf("opening the port: %w", portlock.ErrBusy)
		if _, ran := viaDaemon(context.Background(), app, err, stdout, stderr); ran {
			t.Error("a daemon that is not there answered")
		}
	})

	// Verify that the invocation is carried by the daemon exactly as it was
	// typed, and that the daemon's exit code is the one reported.
	t.Run("Ran", func(t *testing.T) {
		port := tempHome(t)

		argv := fakeDaemon(t, port,
			broker.Response{Type: broker.TypeStdout, Data: "on channel\n"},
			broker.Response{Type: broker.TypeDone, Code: 5},
		)

		app, stdout, stderr, out, errs := setup(t, port)
		oldArgs := os.Args
		os.Args = []string{"radiocli", "status", "-o", "json"}
		t.Cleanup(func() { os.Args = oldArgs })

		err := fmt.Errorf("opening the port: %w", portlock.ErrBusy)
		code, ran := viaDaemon(context.Background(), app, err, stdout, stderr)
		if !ran {
			t.Fatal("the daemon holding the scanner was not used")
		}
		if code != 5 {
			t.Errorf("reported %d, and the daemon reported 5", code)
		}
		if !strings.Contains(out.String(), "on channel") {
			t.Errorf("what the daemon printed did not arrive: %q", out.String())
		}
		if !strings.Contains(errs.String(), "daemon") {
			t.Errorf("the pause was not explained: %q", errs.String())
		}
		if got := <-argv; strings.Join(got, " ") != "status -o json" {
			t.Errorf("the daemon was asked to run %q, and the line was status -o json", got)
		}
	})

	// Verify that a failure from the daemon is printed rather than swallowed.
	t.Run("RunFailed", func(t *testing.T) {
		port := tempHome(t)

		fakeDaemon(t, port, broker.Response{Type: broker.TypeDone, Code: 1, Error: "the scanner refused the key"})

		app, stdout, stderr, _, errs := setup(t, port)
		oldArgs := os.Args
		os.Args = []string{"radiocli", "key", "1"}
		t.Cleanup(func() { os.Args = oldArgs })

		err := fmt.Errorf("opening the port: %w", portlock.ErrBusy)
		code, ran := viaDaemon(context.Background(), app, err, stdout, stderr)
		if !ran || code != 1 {
			t.Errorf("reported %d and ran %v, and the daemon reported a failure with 1", code, ran)
		}
		if !strings.Contains(errs.String(), "the scanner refused the key") {
			t.Errorf("the daemon's failure was not reported: %q", errs.String())
		}
	})
}

// TestWrite tests the witness Write method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wrote: what is written passes through and is remembered
//   - WroteNothing: an empty write is not something having been said
func TestWrite(t *testing.T) {
	// Verify that what is written passes through and is remembered.
	t.Run("Wrote", func(t *testing.T) {
		var buf bytes.Buffer
		w := &witness{to: &buf}

		n, err := w.Write([]byte("7.4 volts\n"))
		if err != nil {
			t.Fatalf("writing: %v", err)
		}
		if n != 10 {
			t.Errorf("wrote %d bytes, and the line is 10 bytes", n)
		}
		if !w.written {
			t.Error("something was written and the witness did not remember it")
		}
		if buf.String() != "7.4 volts\n" {
			t.Errorf("passed on %q, and the line was 7.4 volts", buf.String())
		}
	})

	// Verify that an empty write is not counted as having said anything, so a
	// command that produced nothing can still be run somewhere else.
	t.Run("WroteNothing", func(t *testing.T) {
		var buf bytes.Buffer
		w := &witness{to: &buf}

		if _, err := w.Write(nil); err != nil {
			t.Fatalf("writing nothing: %v", err)
		}
		if w.written {
			t.Error("nothing was written and the witness remembered something")
		}
	})
}

// TestUnwrap tests the witness Unwrap method with 100% coverage.
//
// Coverage: 100% (1 test case, since the method has one branch)
//
// Test cases:
//   - Returns: the stream underneath comes back, so a caller can ask what the
//     output really is
func TestUnwrap(t *testing.T) {
	// Verify the wrapped stream is handed back. Colour is what needs this: a
	// wrapper is not a terminal, so a check that stops here would turn the
	// colour off for every stream in the program.
	t.Run("Returns", func(t *testing.T) {
		var buf bytes.Buffer
		w := &witness{to: &buf}

		if w.Unwrap() != io.Writer(&buf) {
			t.Error("Unwrap did not return the stream underneath")
		}
	})
}
