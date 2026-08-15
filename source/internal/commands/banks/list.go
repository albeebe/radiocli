// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package banks

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// newGoto returns the "banks goto" subcommand.
//
// Parameters:
//   - app: the application context the subcommand reads the scanner and the
//     streams to write to from
//
// Returns:
//   - the "goto" subcommand, which leaves the scanner on one bank's menu
func newGoto(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "goto <bank>",
		Short: "Open a bank's menu on the scanner",
		Long: "Goto leaves the scanner on one bank's menu, so the rest of it can be worked\n" +
			"by hand. Nothing is changed.\n\n" +
			"Use \"radiocli banks set\" to change a bank from here, and \"radiocli scan\"\n" +
			"to send the scanner back to scanning afterwards.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bank, err := bankNumber(args[0])
			if err != nil {
				return err
			}

			client, err := app.Device(cmd.Context())
			if err != nil {
				return err
			}
			if err := open(cmd.Context(), client, bank); err != nil {
				return err
			}
			return menus.Show(cmd.Context(), app, client)
		},
	}
}

// renderBanks writes the banks as a table or as JSON.
//
// Parameters:
//   - app: the application context the output format and the stream to write to
//     come from
//   - found: the banks to write, in the order they are shown
//   - full: whether to include the columns that only a menu walk can fill
//
// Returns:
//   - error if the banks could not be written to the output stream
func renderBanks(app *appcontext.App, found []report, full bool) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, found)
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	if full {
		fmt.Fprintln(w, "BANK\tNAME\tLOWER\tUPPER\tMOD\tSTEP\tATT\tDELAY\tDIGITAL\tAVOID\tHOLD")
	} else {
		fmt.Fprintln(w, "BANK\tNAME\tLOWER\tUPPER\tMOD\tSTEP")
	}

	for _, r := range found {
		if full {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Bank, render.Dash(r.Name),
				render.Dash(r.Lower), render.Dash(r.Upper), render.Dash(r.Modulation), render.Dash(r.Step),
				render.Dash(r.Attenuator), render.Dash(r.Delay), render.Dash(r.DigitalWait),
				render.Dash(r.Avoid), render.Dash(r.HoldTime))
			continue
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", r.Bank, render.Dash(r.Name),
			render.Dash(r.Lower), render.Dash(r.Upper), render.Dash(r.Modulation), render.Dash(r.Step))
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the bank list: %w", err)
	}
	return nil
}

// runList reads every bank and renders them.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output settings come from
//   - full: whether to also read the settings that need the scanner's menus
//     opened, which stops the scan for as long as it takes
//
// Returns:
//   - error if the scanner could not be reached, a bank could not be read, the
//     scanner could not be sent back to scanning, or the banks could not be
//     written
func runList(ctx context.Context, app *appcontext.App, full bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	fromList, err := listed(ctx, client)
	if err != nil {
		return err
	}

	found := make([]report, 0, Count)
	opened := false
	for bank := range Count {
		r, listedIt := fromList[bank]
		if !listedIt {
			// The scanner's list stops short, so whatever it left out is read
			// the slow way rather than left blank.
			if r, err = walked(ctx, client, bank); err != nil {
				return err
			}
			opened = true
		}

		if full {
			if err := extras(ctx, client, bank, &r); err != nil {
				return err
			}
			opened = true
		}
		found = append(found, r)
	}

	// Back to scanning before anything is printed, so the scanner is not left
	// sitting in a menu while somebody reads the answer. Nothing to put back
	// when the whole answer came from the list, which is the common case and
	// the reason this command no longer interrupts the scan.
	if opened {
		if _, err := menus.Leave(ctx, client); err != nil {
			return err
		}
	}
	return renderBanks(app, found, full)
}
