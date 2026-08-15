// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package battery implements the "battery" command, which reports the
// scanner's battery charge and condition.
package battery

import (
	"context"
	"fmt"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the battery command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that reports the scanner's battery when it runs
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use: "battery",
		// Only looks at what the scanner is doing, so it may run while another
		// command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report the scanner's battery charge",
		Long: "Battery reports how much charge the scanner has left, whether it is charging,\n" +
			"and the condition of the battery: its voltage, the current flowing in or out,\n" +
			"and its temperature.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app)
		},
	}
}

// direction says which way the current is flowing, since the sign alone is
// easy to overlook.
//
// Parameters:
//   - milliamps: the current, positive going into the battery and negative
//     coming out of it
//
// Returns:
//   - string reading "charging", "discharging", or "no flow" when nothing is
//     moving either way
func direction(milliamps int) string {
	switch {
	case milliamps > 0:
		return "charging"
	case milliamps < 0:
		return "discharging"
	default:
		return "no flow"
	}
}

// fahrenheit converts a Celsius reading, since the scanner reports only
// Celsius and the command shows both.
//
// Parameters:
//   - celsius: the temperature as the scanner reports it
//
// Returns:
//   - float64 holding the same temperature in Fahrenheit
func fahrenheit(celsius float64) float64 {
	return celsius*9/5 + 32
}

// renderBattery writes the reading for a person.
//
// Parameters:
//   - app: application context the reading is written through
//   - b: the battery as the device layer read it, which carries the raw
//     current the text output describes in words
//   - r: the reading as this command reports it
//
// Returns:
//   - error, always nil, because writing the reading cannot fail
func renderBattery(app *appcontext.App, b device.Battery, r report) error {
	app.Printf("charge:      %d%%\n", r.Percent)
	app.Printf("state:       %s\n", r.State)
	app.Printf("voltage:     %.2f V\n", r.Volts)
	app.Printf("current:     %d mA (%s)\n", b.Milliamps, direction(b.Milliamps))
	app.Printf("temperature: %.1f C (%.1f F)\n", r.Celsius, r.Fahrenheit)

	// A fault is the one thing here worth interrupting for, and it is easy to
	// miss in a row of numbers.
	if r.NeedsAction {
		app.Notef("\nThe charger reports a problem: %s. Check the power supply and\n"+
			"let the scanner reach room temperature before charging it.\n", r.State)
	}
	return nil
}

// run reads the battery and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//
// Returns:
//   - error if the scanner cannot be reached, the battery cannot be read, or
//     the JSON cannot be written; nil once the reading has been reported
func run(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	b, err := client.Battery(ctx)
	if err != nil {
		return fmt.Errorf("reading the battery: %w", err)
	}

	r := report{
		State:       b.State.String(),
		Charging:    b.Charging(),
		Percent:     b.Percent,
		Volts:       b.Volts(),
		Milliamps:   b.Milliamps,
		Celsius:     b.Celsius,
		Fahrenheit:  fahrenheit(b.Celsius),
		NeedsAction: b.State.Faulted(),
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	return renderBattery(app, b, r)
}
