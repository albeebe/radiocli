// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Command radiocli controls and inspects an SDS150 device.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/commands/audio"
	"github.com/albeebe/radiocli/internal/commands/backlight"
	"github.com/albeebe/radiocli/internal/commands/backup"
	"github.com/albeebe/radiocli/internal/commands/banks"
	"github.com/albeebe/radiocli/internal/commands/battery"
	"github.com/albeebe/radiocli/internal/commands/beep"
	"github.com/albeebe/radiocli/internal/commands/channels"
	"github.com/albeebe/radiocli/internal/commands/clock"
	"github.com/albeebe/radiocli/internal/commands/colors"
	"github.com/albeebe/radiocli/internal/commands/config"
	"github.com/albeebe/radiocli/internal/commands/daemon"
	"github.com/albeebe/radiocli/internal/commands/departments"
	"github.com/albeebe/radiocli/internal/commands/devices"
	"github.com/albeebe/radiocli/internal/commands/display"
	"github.com/albeebe/radiocli/internal/commands/favorites"
	"github.com/albeebe/radiocli/internal/commands/headphone"
	"github.com/albeebe/radiocli/internal/commands/key"
	"github.com/albeebe/radiocli/internal/commands/location"
	"github.com/albeebe/radiocli/internal/commands/menu"
	"github.com/albeebe/radiocli/internal/commands/receiving"
	"github.com/albeebe/radiocli/internal/commands/root"
	"github.com/albeebe/radiocli/internal/commands/scan"
	"github.com/albeebe/radiocli/internal/commands/scanning"
	"github.com/albeebe/radiocli/internal/commands/screen"
	"github.com/albeebe/radiocli/internal/commands/sites"
	"github.com/albeebe/radiocli/internal/commands/squelch"
	"github.com/albeebe/radiocli/internal/commands/status"
	"github.com/albeebe/radiocli/internal/commands/systems"
	"github.com/albeebe/radiocli/internal/commands/tune"
	"github.com/albeebe/radiocli/internal/commands/update"
	"github.com/albeebe/radiocli/internal/commands/version"
	"github.com/albeebe/radiocli/internal/commands/volume"
	"github.com/albeebe/radiocli/internal/commands/weather"
	"github.com/albeebe/radiocli/internal/commandtree"
	"github.com/albeebe/radiocli/internal/portlock"
	"github.com/spf13/cobra"
)

// commands lists every command the tool exposes. Adding one means importing
// its package and adding its constructor here, and nothing else.
var commands = []func(*appcontext.App) *cobra.Command{
	audio.New,
	backlight.New,
	backup.New,
	banks.New,
	battery.New,
	beep.New,
	channels.New,
	clock.New,
	colors.New,
	config.New,
	daemon.New,
	departments.New,
	devices.New,
	display.New,
	favorites.New,
	headphone.New,
	key.New,
	menu.New,
	location.New,
	receiving.New,
	scan.New,
	scanning.New,
	screen.New,
	sites.New,
	squelch.New,
	status.New,
	systems.New,
	tune.New,
	update.New,
	version.New,
	volume.New,
	weather.New,
}

// exit ends the process with a status code. It is a var so tests can
// substitute a fake, since the real one would end the test binary.
var exit = os.Exit

// newApp builds the application context this invocation is wired around. It is
// a var so tests can substitute a fake.
var newApp = appcontext.New

func main() {
	// os.Exit skips deferred functions, so the real work happens in run and
	// main does nothing but turn its error into an exit code.
	exit(run())
}

// run wires the application together and executes the requested command.
func run() int {
	// Ctrl-C and SIGTERM cancel this context, which cobra passes to every
	// command so in-flight work can stop cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := newApp()
	defer func() {
		if err := app.Close(); err != nil {
			fmt.Fprintln(app.Stderr, "error: shutting down:", err)
		}
	}()

	// Everything a command prints goes through these, so that a command
	// refused the scanner before it said anything can be told apart from one
	// refused after. Only the first can be run again somewhere else.
	stdout := &witness{to: app.Stdout}
	stderr := &witness{to: app.Stderr}
	app.Stdout, app.Stderr = stdout, stderr

	// Every invocation gets a tree of its own. Cobra binds flags to variables
	// by pointer as the tree is built, so a tree that ran once carries those
	// values into the next run. Building fresh is cheap and makes that
	// impossible.
	newTree := func(bound *appcontext.App) *cobra.Command {
		cmd := root.New(bound)
		for _, newCmd := range commands {
			cmd.AddCommand(newCmd(bound))
		}
		return cmd
	}

	// Let commands invoke other commands in this process. The closure lives
	// here because this is the only place that knows the command list, and
	// passing it as a function keeps every command package unaware of every
	// other one.
	app.RunCommand = func(ctx context.Context, argv []string) error {
		cmd := newTree(app)
		cmd.SetArgs(argv)
		return cmd.ExecuteContext(ctx)
	}

	// Let a command offer the others. Reading the tree rather than a written
	// out list is what stops the two drifting apart: a command added above
	// appears there without anybody remembering to describe it twice.
	app.Commands = func() []appcontext.Command {
		return commandtree.Describe(newTree(app))
	}

	// Let something watch the scanner while another command is using it. The
	// App handed back shares the connection and refuses anything that could
	// move the radio, which is decided by asking the command tree rather than
	// by a list kept here: a command says for itself whether it only reads.
	app.Reader = func() *appcontext.App {
		reader := app.Borrow()
		reader.RunCommand = func(ctx context.Context, argv []string) error {
			// Asked of a tree of its own, because answering means parsing the
			// flags and a tree that has parsed once carries those values into
			// the run. Building one is cheap.
			found, rest, err := newTree(reader).Find(argv)
			if err != nil {
				return err
			}
			if !onlyReads(found, rest) {
				return fmt.Errorf("%q can move the scanner, so it cannot run alongside another command",
					found.CommandPath())
			}

			cmd := newTree(reader)
			cmd.SetArgs(argv)
			return cmd.ExecuteContext(ctx)
		}
		return reader
	}

	if err := newTree(app).ExecuteContext(ctx); err != nil {
		// A scanner somebody else is holding is the one failure with a second
		// thing worth trying, because a daemon may be holding it precisely so
		// that this command can queue rather than be refused.
		if code, ran := viaDaemon(ctx, app, err, stdout, stderr); ran {
			return code
		}

		// A cancelled run is the user's own doing, not a failure to report.
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(app.Stderr, "error:", err)
		}
		return 1
	}
	return 0
}

// viaDaemon runs this invocation again on a daemon holding the scanner, and
// reports whether it did.
//
// Running a command twice is safe exactly when the first attempt stopped at the
// point of opening the port: before that it has touched neither the radio nor
// the terminal. Being refused the lock is what says it stopped there, and the
// streams having stayed silent is what proves it, rather than a convention
// every command is trusted to keep for ever.
//
// Anything that goes wrong finding or reaching a daemon means there is no
// sharing to be had, and the caller falls back to reporting the busy scanner
// exactly as it always did.
func viaDaemon(ctx context.Context, app *appcontext.App, err error, stdout, stderr *witness) (int, bool) {
	if !errors.Is(err, portlock.ErrBusy) || stdout.written || stderr.written {
		return 0, false
	}

	client, dialErr := broker.Dial(app.Config.Device)
	if dialErr != nil {
		return 0, false
	}
	defer client.Close()

	// Said rather than kept quiet, because a command that would have failed a
	// moment ago now pauses instead, and the reason for the pause should not
	// be a mystery. It is a statement rather than a warning: waiting is the
	// feature, not a problem.
	app.Notef("Running through the radiocli daemon holding this scanner.\n")

	// The arguments go over exactly as they were typed, which is what makes a
	// proxied command do precisely what the same command does in a terminal.
	// It is also how settings travel: a flag on this line is a flag on the
	// line the daemon parses.
	outcome, runErr := client.Run(ctx, os.Args[1:], broker.ModeQueue, stdout.to, stderr.to)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		fmt.Fprintln(stderr.to, "error:", runErr)
	}
	return outcome.Code, true
}

// onlyReads reports whether a command can be run alongside another one, which
// it can only if nothing about it presses a key or opens a menu.
//
// Most commands answer for themselves whatever their flags say. A few answer
// differently depending on them, and those name the flags that make the
// difference rather than being marked one way for all of their forms: see
// appcontext.OnlyReadsWith.
//
// Anything unmarked is refused, so a command added later has to be looked at
// before it can run alongside anything.
func onlyReads(cmd *cobra.Command, args []string) bool {
	if cmd.Annotations[appcontext.OnlyReads] == "true" {
		return true
	}

	names := strings.Fields(cmd.Annotations[appcontext.OnlyReadsWith])
	if len(names) == 0 {
		return false
	}

	// A line that will not parse is refused here rather than guessed at. It
	// fails the same way a moment later when it runs, with the message the
	// terminal would have given.
	if err := cmd.ParseFlags(args); err != nil {
		return false
	}
	for _, name := range names {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			return true
		}
	}
	return false
}

// witness passes writes through and remembers whether there were any.
type witness struct {
	to      io.Writer
	written bool
}

// Unwrap returns the stream underneath, so that anything needing to know what
// the output really is can see past this. Colour is the caller that needs it:
// a wrapper is not a terminal, and asking one would turn the colour off for the
// whole program.
//
// Returns:
//   - the writer this passes everything on to
func (w *witness) Unwrap() io.Writer { return w.to }

// Write records that something was produced and passes it on.
func (w *witness) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.written = true
	}
	return w.to.Write(p)
}
