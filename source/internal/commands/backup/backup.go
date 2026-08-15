// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package backup implements the "backup" command, which copies a scanner's
// memory card to this computer.
//
// It is the only command that does not talk to the scanner. Everything the
// radio stores that is worth keeping, including the favorites lists and the
// display colors the protocol cannot read, lives on its card, and the card is
// only reachable when the scanner is started in mass storage mode. That mode
// replaces the serial port rather than joining it, so this command and every
// other one in the tool can never run in the same session.
package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the backup command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and logger from
//
// Returns:
//   - *cobra.Command that copies the scanner's memory card when it runs
func New(app *appcontext.App) *cobra.Command {
	var opts options

	cmd := &cobra.Command{
		Use:   "backup [destination]",
		Short: "Copy the scanner's memory card to this computer",
		Long: "Backup copies everything on the scanner's memory card into a dated folder,\n" +
			"including the favorites lists, the settings and the display colors.\n\n" +
			"The card is only reachable when the scanner is started in mass storage mode:\n" +
			"restart it with the cable connected and press \"E\" at the USB prompt. The\n" +
			"serial port does not exist in that mode, so no other command will reach the\n" +
			"scanner until you restart it again.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			destination := "."
			if len(args) == 1 {
				destination = args[0]
			}
			return run(cmd.Context(), app, destination, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.source, "source", "",
		"path to the card, skipping the search for a mounted one")
	f.StringVar(&opts.name, "name", "",
		"name for the backup folder, instead of the dated default")
	f.BoolVar(&opts.verify, "verify", true,
		"read every file back and compare it against the card")
	f.BoolVar(&opts.database, "database", true,
		"include the downloaded radio database, which is most of the card")
	f.BoolVar(&opts.dryRun, "dry-run", false,
		"report what would be copied without writing anything")

	return cmd
}

// announce says which card is being read, and flags anything on it that will
// not be copied.
//
// It is called once everything that can fail has succeeded, so that a refusal
// reads as an error rather than as a report interrupted by one.
//
// Parameters:
//   - app: application context the notes are written through
//   - c: the card that was found
//   - work: what the backup will copy, which says how much of the card is being
//     left behind
func announce(app *appcontext.App, c card, work plan) {
	app.Notef("Found %s on %s\n", c.describe(), c.Root)
	if work.skipped > 0 {
		app.Notef("Ignoring %d item(s) on the card that are not ordinary files.\n", work.skipped)
	}
}

// defaultName builds the folder name for a backup.
//
// The model and the date are what somebody scanning a directory of these needs
// to tell them apart. The time is included because making two backups in one
// day while changing something in between is exactly what a person does around
// a risky change.
//
// Parameters:
//   - c: the card the name is built from
//
// Returns:
//   - string naming the folder, built from the model and the current time, and
//     from "scanner" and the time when the card did not identify itself
func defaultName(c card) string {
	stamp := time.Now().Format("2006-01-02-1504")
	if c.Model == "" {
		return "scanner-" + stamp
	}
	return c.Model + "-" + stamp
}

// locate finds the card, either where the user said or by searching.
//
// Parameters:
//   - source: path to the card, or empty to search where this platform mounts
//     removable media
//
// Returns:
//   - card describing where the card is and which radio wrote it
//   - error if source holds no scanner card, or if nothing was found while
//     searching
func locate(source string) (card, error) {
	if source != "" {
		return openCard(source)
	}
	return findCard()
}

// prepare creates the folder the backup is written into.
//
// It refuses an existing folder rather than merging into it. Two backups
// blended together would look like one complete card and be neither, and that
// is not a mistake worth letting somebody make silently.
//
// Parameters:
//   - destination: directory the backup folder is created inside
//   - name: name for the backup folder, or empty to use the dated default
//   - c: the card the default name is built from
//
// Returns:
//   - string holding the absolute path to the folder that was created
//   - error if the path cannot be resolved, something already exists there, or
//     the folder cannot be created
func prepare(destination, name string, c card) (string, error) {
	if name == "" {
		name = defaultName(c)
	}

	target, err := absPath(filepath.Join(destination, name))
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("%s already exists: pass --name to choose another, or move it out of the way",
			target)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", target, err)
	}
	return target, nil
}

// preview turns a plan into results for a dry run, without digests, since
// nothing was read.
//
// Parameters:
//   - p: the plan to describe
//
// Returns:
//   - []result naming every file the plan would copy, and its size
func preview(p plan) []result {
	out := make([]result, 0, len(p.files))
	for _, f := range p.files {
		out = append(out, result{Path: f.rel, Bytes: f.size})
	}
	return out
}

// renderReport writes the outcome.
//
// Parameters:
//   - app: application context holding the output format and the streams to
//     write to
//   - r: the outcome to report
//
// Returns:
//   - error if the JSON cannot be written; nil for the text output, which
//     cannot fail
func renderReport(app *appcontext.App, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	if r.DryRun {
		app.Printf("Would copy %d files in %d directories, %s, from %s\n",
			r.Files, r.Directories, human(r.Bytes), r.Card.Root)
		if !r.DatabaseIncluded {
			app.Notef("The radio database is excluded. Drop --database=false to include it.\n")
		}
		app.Notef("Nothing was written.\n")
		return nil
	}

	app.Printf("Backed up %d files in %d directories, %s, to %s\n",
		r.Files, r.Directories, human(r.Bytes), r.Destination)

	if r.Verified {
		app.Notef("Every file was read back and matched the card.\n")
	} else {
		app.Notef("Files were not verified. Pass --verify to read them back and compare.\n")
	}
	if !r.DatabaseIncluded {
		app.Notef("The radio database was excluded, so this backup is not a complete card.\n")
	}
	app.Notef("Restart the scanner to use the serial port again.\n")
	return nil
}

// run performs the backup.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while reading and copying the
//     card
//   - app: application context holding the output format, the streams to write
//     to and the logger
//   - destination: directory the backup folder is created inside
//   - opts: the flags the command was given
//
// Returns:
//   - error if the card cannot be found or read, it holds no files, the folder
//     cannot be created, or a file cannot be copied or verified; nil once the
//     backup has been reported
func run(ctx context.Context, app *appcontext.App, destination string, opts options) error {
	found, err := locate(opts.source)
	if err != nil {
		return err
	}

	app.Log.Debug("found scanner card", "root", found.Root, "model", found.Model)

	work, err := build(ctx, found.dir(), opts.database)
	if err != nil {
		return err
	}
	if len(work.files) == 0 {
		return fmt.Errorf("the card at %s holds no files, which means it is not the card this expects",
			found.Root)
	}

	rep := report{
		Card:             found,
		Files:            len(work.files),
		Bytes:            work.bytes,
		Directories:      len(work.dirs),
		Verified:         opts.verify && !opts.dryRun,
		DatabaseIncluded: opts.database,
		DryRun:           opts.dryRun,
	}

	if opts.dryRun {
		rep.Copied = preview(work)
		announce(app, found, work)
		return renderReport(app, rep)
	}

	// Everything that can fail happens before the first note, so a run that
	// refuses puts the reason at the top of stderr rather than below a banner
	// about a card it then declined to copy.
	target, err := prepare(destination, opts.name, found)
	if err != nil {
		return err
	}
	rep.Destination = target

	announce(app, found, work)
	app.Notef("Copying %d files, %s, to %s\n", len(work.files), human(work.bytes), target)

	// Progress goes to stderr so it survives neither a pipe nor a redirect of
	// the result, which is what a caller parsing the output wants.
	done := 0
	copied, err := work.run(ctx, found.dir(), target, opts.verify, func(f file) {
		done++
		app.Log.Debug("copying", "file", f.rel, "bytes", f.size)
		if done%25 == 0 || done == len(work.files) {
			app.Notef("  %d/%d\n", done, len(work.files))
		}
	})
	if err != nil {
		// Say where the partial copy is rather than leaving it to be found.
		// Removing it would be worse: a half backup somebody knows about beats
		// no backup and a deleted folder.
		app.Notef("The backup did not finish. What was copied is in %s\n", target)
		return err
	}
	rep.Copied = copied

	return renderReport(app, rep)
}
