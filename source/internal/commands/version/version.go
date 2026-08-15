// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package version implements the "version" command, which reports what the
// running binary was built from.
package version

import (
	"runtime"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/buildinfo"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the version command bound to app.
//
// Parameters:
//   - app: the dependency container the command renders through
//
// Returns:
//   - *cobra.Command for "version"
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use: "version",
		// Only looks at what the scanner is doing, so it may run while another
		// command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Print build information",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(app)
		},
	}
}

// run renders the build information in the configured output format.
//
// Parameters:
//   - app: the dependency container holding the output format and the streams
//     to write to
//
// Returns:
//   - error if the JSON could not be written; the text form reports nothing,
//     since it writes through the App
func run(app *appcontext.App) error {
	i := info{
		Version: buildinfo.Version,
		Commit:  buildinfo.Commit,
		Date:    buildinfo.Date,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, i)
	}

	app.Printf("radiocli %s\n", i.Version)
	app.Printf("  commit: %s\n", i.Commit)
	app.Printf("  built:  %s\n", i.Date)
	app.Printf("  go:     %s (%s/%s)\n", i.Go, i.OS, i.Arch)
	return nil
}
