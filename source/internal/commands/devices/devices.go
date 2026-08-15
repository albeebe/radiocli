// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package devices implements the "devices" command, which lists every scanner
// attached to this computer without changing anything.
package devices

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the devices command bound to app.
//
// Parameters:
//   - app: the application context the command reads its configuration and
//     writes its output through
//
// Returns:
//   - the "devices" command, which also answers to "list"
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:     "devices",
		Aliases: []string{"list"},
		Short:   "List the scanners attached to this computer",
		Long: "Devices lists every scanner attached to this computer. It only looks and\n" +
			"changes nothing: pass a port it prints to --device to talk to that scanner.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app)
		},
	}
}

// hasDaemon reports whether a busy port is one a daemon is holding, which is
// what decides whether being busy is a wait or a nuisance.
//
// It connects rather than looking for the socket file, because a daemon that
// was killed leaves its socket behind and a socket nobody is listening on is
// the same answer as no socket at all. The connection is dropped straight away:
// this asks a question, it does not queue anything.
//
// Parameters:
//   - port: the serial port a daemon would be named after
//
// Returns:
//   - whether something answered as a daemon on that port
func hasDaemon(port string) bool {
	client, err := broker.Dial(port)
	if err != nil {
		return false
	}
	client.Close()
	return true
}

// renderDevices writes the listing as an aligned table.
//
// Finding nothing is a complete answer to "what is attached", not a failure,
// so the caller puts the advice on Stderr rather than failing the command.
// Scripts checking the exit code care whether the scan ran, and the empty list
// is the result.
//
// Parameters:
//   - app: the application context whose Stdout receives the table
//   - entries: the scanners to list, in the order they should appear
//
// Returns:
//   - error if the table cannot be written to Stdout
func renderDevices(app *appcontext.App, entries []entry) error {
	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tSERIAL\tPORT")

	for _, e := range entries {
		// A dash stands for a field that could not be read, which is both
		// fields of a busy port and the serial number of a port that reports
		// none.
		model, serial := e.Model, e.Serial
		if e.Busy {
			model = "-"
		}
		if serial == "" {
			serial = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", model, serial, e.Port)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the device list: %w", err)
	}
	return nil
}

// run discovers the attached scanners and renders them.
//
// Parameters:
//   - ctx: context that stops the search when it is cancelled
//   - app: the application context holding the requested output format, the
//     logger and the streams
//
// Returns:
//   - error if the search itself fails, or if writing the listing fails;
//     finding nothing is not an error
func run(ctx context.Context, app *appcontext.App) error {
	app.Log.Debug("scanning for attached scanners")
	found, busy, err := discover(ctx, app.Log)
	if err != nil {
		return err
	}

	entries := make([]entry, 0, len(found)+len(busy))
	for _, info := range found {
		entries = append(entries, entry{Info: info})
	}

	// A scanner this tool is already using is still attached, so leaving it out
	// would be wrong: it is almost certainly a scanner, and the only reason it
	// could not be identified is that another invocation got there first.
	var shared, exclusive []string
	for _, port := range busy {
		e := entry{Info: device.Info{Port: port}, Busy: true, Shared: hasDaemon(port)}
		if e.Shared {
			shared = append(shared, port)
		} else {
			exclusive = append(exclusive, port)
		}
		entries = append(entries, e)
	}

	if app.Config.Output == appcontext.OutputJSON {
		if err := render.JSON(app.Stdout, entries); err != nil {
			return err
		}
	} else if len(found) > 0 {
		// With nothing identified there is no table worth printing, and the
		// advice below says everything there is to say.
		if err := renderDevices(app, entries); err != nil {
			return err
		}
	}

	// The advice goes to stderr, after the document rather than instead of it,
	// so a reader that asked for JSON gets JSON however discovery went. Every
	// scanner being busy is kept apart from none being attached because the
	// advice is the opposite: waiting fixes one, and the other needs somebody
	// to go and change something.
	//
	// A shared port is kept apart from both, because it needs nothing at all.
	// Telling somebody to wait for a scanner they could be using this second is
	// how a working setup gets reported as a broken one.
	if len(entries) == 0 {
		app.Notef("%s\n", device.ErrNoScanners)
		return nil
	}

	// A shared port is named first, because it is the one the reader can act
	// on. Somebody looking at a list of scanners they cannot have wants to know
	// straight away that one of them is not like the others.
	if len(shared) > 0 {
		app.Notef("\nHeld by an radiocli daemon, which runs commands on it for anybody who asks: %s\n"+
			"Pass one to --device as usual. Commands queue behind whatever else is running\n"+
			"rather than being refused.\n", strings.Join(shared, ", "))
	}
	switch {
	case len(found) == 0 && len(shared) == 0:
		app.Notef("%s\n", device.BusyError(exclusive))
	case len(exclusive) > 0:
		app.Notef("\nIn use by another radiocli, so not identified: %s\n", strings.Join(exclusive, ", "))
	}
	return nil
}
