// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package root builds the top-level radiocli command.
//
// It owns everything that applies to every subcommand: the global flags, the
// order in which config sources override each other, and the point at which
// the App's dependencies get built. Subcommands assume all of that is done.
package root

import (
	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/buildinfo"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/spf13/cobra"
)

// New returns the root command with the global flags registered. The caller
// adds the subcommands, so this package never imports them and command
// packages stay independent of each other.
//
// Parameters:
//   - app: the dependency container the command resolves settings into and
//     shares with every subcommand
//
// Returns:
//   - *cobra.Command for the tool itself, carrying the global flags and the
//     configuration resolution that runs before any subcommand
func New(app *appcontext.App) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "radiocli",
		Short: "Control and inspect an SDS150 device",
		Long: "radiocli is a command line tool for controlling and inspecting an SDS150 device.\n\n" +
			"Settings are resolved from the config file first and the global flags second,\n" +
			"so a flag always overrides the file.",
		Version: buildinfo.Version,

		// Errors are reported by main, which owns the exit code. Usage is only
		// worth printing for bad input, which cobra reports before RunE.
		SilenceErrors: true,
		SilenceUsage:  true,

		// Resolve configuration and build dependencies before any subcommand
		// runs. Cobra runs only the nearest PersistentPreRunE in the chain, so
		// a subcommand that defines its own must call this one explicitly.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			app.Config.Path = flags.config
			if err := app.Config.Load(); err != nil {
				return err
			}
			flags.apply(cmd, app.Config)
			if err := app.Config.Validate(); err != nil {
				return err
			}
			return app.Init(cmd.Context())
		},
	}

	// Route cobra's own output through the App so tests capture it too.
	cmd.SetOut(app.Stdout)
	cmd.SetErr(app.Stderr)
	cmd.SetIn(app.Stdin)

	// Register the flags every command inherits. The defaults given here are
	// only what the help prints, since apply skips any flag the user did not
	// type and leaves the config file's answer standing.
	f := cmd.PersistentFlags()
	f.StringVar(&flags.config, "config", "", "path to the config file (default: user config directory)")
	f.BoolVarP(&flags.verbose, "verbose", "v", false, "enable debug logging")
	f.StringVarP(&flags.output, "output", "o", string(appcontext.OutputText), "output format: text or json")
	f.StringVar(&flags.device, "device", "", "serial port of the scanner to use, such as /dev/tty.usbmodem14201")
	f.StringVar(&flags.pace, "pace", string(device.DefaultPace), "how quickly keys are sent to the scanner: "+device.PaceNames())
	f.DurationVar(&flags.wait, "wait", 0, "how long to wait for another radiocli to finish with the scanner (default: do not wait)")

	return cmd
}

// apply copies the flags the user set explicitly onto cfg. Flags left at their
// default are skipped so they cannot silently undo a config file setting.
//
// Parameters:
//   - cmd: the command being run, which is what knows which flags were typed
//   - cfg: the settings the flags are applied to, modified in place
func (g globalFlags) apply(cmd *cobra.Command, cfg *appcontext.Config) {
	if cmd.Flags().Changed("verbose") {
		cfg.Verbose = g.verbose
	}
	if cmd.Flags().Changed("output") {
		cfg.Output = appcontext.OutputFormat(g.output)
	}
	if cmd.Flags().Changed("device") {
		cfg.Device = g.device
	}
	if cmd.Flags().Changed("pace") {
		cfg.Pace = device.Pace(g.pace)
	}
	if cmd.Flags().Changed("wait") {
		cfg.Wait = g.wait
	}
}
