// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package daemon implements the "daemon" command, which holds one scanner and
// runs commands on it for anybody else that asks.
//
// It exists because the claim on a scanner covers a whole invocation, so that a
// menu walk cannot be cut in half by somebody else's command. That claim is
// exclusive, and without something like this the second caller is simply
// refused. With a daemon running, the second caller queues instead.
package daemon

import (
	"io"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/spf13/cobra"
)

// New returns the daemon command bound to app.
//
// Parameters:
//   - app: the application context whose scanner is held, and whose Stdin the
//     daemon watches when it is asked to exit with its parent
//
// Returns:
//   - the "daemon" command, with its flags already registered
func New(app *appcontext.App) *cobra.Command {
	var (
		exitWithParent bool
		audio          string
		audioChannel   string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Hold a scanner and run other invocations' commands on it",
		Long: "Daemon claims the scanner named with --device and keeps it, answering commands\n" +
			"from other invocations of this tool over a socket. It runs until interrupted.\n\n" +
			"Without it, the second command to want a scanner is refused while the first is\n" +
			"using it. With it, that command waits its turn and then runs, and so does the\n" +
			"one after that. Commands still run one at a time, because the scanner answers\n" +
			"one at a time; what changes is that being second means waiting rather than\n" +
			"failing.\n\n" +
			"No radiocli command starts this for you. Commands look for a daemon only\n" +
			"after being refused the scanner, and behave exactly as they always did when\n" +
			"there is none, so sharing is something you switch on rather than something\n" +
			"that happens to a script that was relying on being refused.\n\n" +
			"Given --audio it also holds a sound input, for the same reason and to the\n" +
			"same end: the scanner's audio arrives on a cable rather than over USB, and\n" +
			"one program reading that input can serve as many listeners as ask for it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Checked before the scanner is touched, so a misspelled channel is
			// a refusal rather than a daemon that comes up and then cannot fold
			// the audio it was started for.
			channel, err := audiofeed.ParseChannel(audioChannel)
			if err != nil {
				return err
			}

			opts := broker.Options{Audio: audio, AudioChannel: channel}
			if exitWithParent {
				orphaned := make(chan struct{})
				go watchParent(app.Stdin, orphaned)
				opts.Orphaned = orphaned
			}
			return broker.Serve(cmd.Context(), app, app.Config.Device, opts)
		},
	}

	cmd.Flags().BoolVar(&exitWithParent, "exit-with-parent", false,
		"stop once whoever started this has gone and nothing else is connected")
	cmd.Flags().StringVar(&audio, "audio", "",
		"sound input the scanner's audio arrives on, as \"radiocli audio\" names it")
	cmd.Flags().StringVar(&audioChannel, "audio-channel", audiofeed.ChannelAuto,
		"which side of the cable the scanner is on: \"auto\", \"left\", \"right\" or \"mix\"")

	return cmd
}

// watchParent closes orphaned when the program that started the daemon goes
// away, however it goes.
//
// A daemon holds a scanner, so one whose starter has died and left it running
// is a radio nobody can use until somebody notices and kills it by hand. That
// is exactly what happens when the parent is killed outright: it never reaches
// whatever tidying it meant to do.
//
// Stdin is how the parent says it is still there, and it says it by holding the
// write end of a pipe open rather than by writing anything. When the parent
// exits, crashes or is killed, the operating system closes every handle it had,
// this end reads end-of-file, and this fires. Nothing is polled, no process ID
// can be reused underneath it, and it needs no code of its own on any
// particular system: the kernel does the whole of it.
//
// What it does not do is stop the daemon. The daemon is quite possibly being
// used by something that never knew which process started it, and the broker
// waits for those to finish before closing the scanner. This only reports that
// nobody is expected to come back.
//
// It is behind a flag because a daemon somebody started in a terminal must not
// count itself orphaned the moment its stdin happens to be a file that ends,
// or /dev/null.
//
// Parameters:
//   - in: the stream the parent holds open, normally the daemon's own stdin;
//     nothing happens at all when it is nil
//   - orphaned: closed once in reaches end-of-file
func watchParent(in io.Reader, orphaned chan<- struct{}) {
	if in == nil {
		return
	}

	// Anything read is thrown away. The parent is not expected to send
	// anything, and a parent that did would only be keeping the pipe alive.
	io.Copy(io.Discard, in)
	close(orphaned)
}
