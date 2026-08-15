// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package tune implements the "tune" command, which puts the scanner straight
// onto one frequency and holds it there.
package tune

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the tune command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that tunes the scanner to the frequency given on the
//     command line
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "tune <frequency>",
		Short: "Tune the scanner to one frequency and hold it",
		Long: "Tune puts the scanner straight onto one frequency and holds it there, which\n" +
			"is the quickest way to listen to something without adding it to the memory.\n\n" +
			"The frequency is read as megahertz unless it carries a unit, so \"155.475\",\n" +
			"\"155.475MHz\", \"155475kHz\" and \"155475000Hz\" all mean the same thing.\n\n" +
			"It reports whether anything is being received the moment it lands, which is a\n" +
			"reading at that instant rather than a verdict on the frequency.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parsing before the scanner is opened means a mistyped frequency
			// costs nothing and reads the same whether or not one is attached.
			f, err := parse(args[0])
			if err != nil {
				return err
			}
			return run(cmd.Context(), app, args[0], f)
		},
	}
}

// coverage renders the bands for a message.
//
// Returns:
//   - the spans the scanner covers, as "lo to hi" separated by commas
func coverage() string {
	spans := make([]string, 0, len(bands))
	for _, b := range bands {
		spans = append(spans, fmt.Sprintf("%s to %s", b.lo, b.hi))
	}
	return strings.Join(spans, ", ")
}

// covered rejects a frequency the scanner cannot receive, before anything is
// sent to it.
//
// The scanner refuses these itself, but its refusal is the same one it gives
// for being busy and carries no reason, so checking here is what turns "it said
// no" into something a reader can act on.
//
// Parameters:
//   - f: the frequency to check, already rounded to what the protocol carries
//
// Returns:
//   - error naming the blocked cellular bands if the frequency falls in one, or
//     naming the spans the scanner does cover if it falls outside them all; nil
//     if the scanner can receive it
func covered(f device.Frequency) error {
	for _, b := range bands {
		if f >= b.lo && f <= b.hi {
			return nil
		}
	}

	for _, b := range cellular {
		if f >= b.lo && f <= b.hi {
			return fmt.Errorf("the scanner cannot receive %s: the %s to %s and %s to %s cellular "+
				"bands are blocked in scanners sold in the United States, and no setting unblocks "+
				"them", f,
				cellular[0].lo, cellular[0].hi, cellular[1].lo, cellular[1].hi)
		}
	}

	return fmt.Errorf("the scanner cannot receive %s: it covers %s", f, coverage())
}

// listen waits for the scanner to report what it can hear on the frequency it
// has just been tuned to.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while polling the scanner
//   - client: the scanner connection to ask
//
// Returns:
//   - the last reading taken, which is the first one carrying a signal, or the
//     final one when the frequency stayed quiet for the whole wait
//   - error if the scanner cannot be asked, or if ctx is cancelled part way
//     through the wait
func listen(ctx context.Context, client *device.Scanner) (device.Property, error) {
	var last device.Property

	for poll := 0; poll < settlePolls; poll++ {
		info, err := client.ScannerInfo(ctx)
		if err != nil {
			return device.Property{}, err
		}

		last = info.Property
		if receiving(last) {
			return last, nil
		}

		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(settleGap):
		}
	}

	// Nothing arrived within the wait, which is a real answer about a quiet
	// frequency rather than a failure.
	return last, nil
}

// parse reads a frequency written with or without a unit.
//
// The reading is device.ParseFrequency, which every command that takes a
// frequency shares. What is done here is the wording: the same three failures
// are told to a reader who typed a frequency at this command, which is why the
// message names the argument and gives an example rather than passing on a
// sentence written for nobody in particular.
//
// Parameters:
//   - arg: the frequency as it was typed, with or without a unit suffix
//
// Returns:
//   - the frequency, read as megahertz unless the argument carried a unit
//   - error if the argument is not a number, is zero or negative, or is finer
//     than the scanner can tune
func parse(arg string) (device.Frequency, error) {
	f, err := device.ParseFrequency(arg)
	if err != nil {
		switch {
		case errors.Is(err, device.ErrFrequencyNotPositive):
			return 0, fmt.Errorf("invalid frequency %q: it must be greater than zero", arg)
		case errors.Is(err, device.ErrFrequencyTooSmall):
			return 0, fmt.Errorf("invalid frequency %q: it is smaller than the scanner can tune", arg)
		default:
			return 0, fmt.Errorf("invalid frequency %q: want a number in megahertz, such as 155.475, "+
				"or a number with a unit, such as 155475kHz", arg)
		}
	}
	return f, nil
}

// receiving reports whether the scanner is hearing anything.
//
// The number of bars is the reading to trust. The raw strength figure is
// reported even when nothing is coming in, as the noise floor, so a value
// there says only that the scanner answered.
//
// Parameters:
//   - p: the scanner's reading of its own signal
//
// Returns:
//   - true if the scanner is showing at least one signal bar
func receiving(p device.Property) bool {
	bars, err := strconv.Atoi(strings.TrimSpace(p.Signal))
	return err == nil && bars > 0
}

// refused explains a rejection, which the scanner gives no reason for.
//
// The same answer covers a frequency the scanner cannot reach and a scanner
// that is busy with something else, so this asks what it is doing and reports
// the span it covers, rather than guessing at one cause and sending the reader
// after the wrong thing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while asking the scanner what
//     it is doing
//   - client: the scanner connection to ask
//   - want: the frequency the scanner would not take, named in the message
//
// Returns:
//   - error explaining the rejection, naming the screen the scanner is on when
//     that is the cause, and the span it covers when it is not. It is never nil,
//     since it is only called after a refusal
func refused(ctx context.Context, client *device.Scanner, want device.Frequency) error {
	info, err := client.ScannerInfo(ctx)
	if err != nil {
		return fmt.Errorf("the scanner would not tune to %s, and gave no reason: it refuses a "+
			"frequency outside the span it covers, and refuses any frequency while it is in a "+
			"menu, entering something, or saving", want)
	}

	span := ""
	if info.SearchRange.Lower != "" && info.SearchRange.Upper != "" {
		span = fmt.Sprintf(" It covers %s to %s.",
			strings.TrimSpace(info.SearchRange.Lower), strings.TrimSpace(info.SearchRange.Upper))
	}

	// Being somewhere other than a normal operating screen is the other cause,
	// and the one with a fix worth naming.
	if info.Screen != "" && info.Screen != quickSearch && !strings.Contains(info.Screen, "scan") {
		return fmt.Errorf("the scanner would not tune to %s: it is on its %q screen, and refuses "+
			"a tune while it is in a menu, entering something, or saving. Run \"radiocli scan\" "+
			"to return it to scanning, then try again", want, info.Screen)
	}

	return fmt.Errorf("the scanner would not tune to %s, and it is not busy with anything else, "+
		"so the frequency is most likely outside what it can reach.%s", want, span)
}

// round takes a frequency to the nearest step the protocol can carry.
//
// Parameters:
//   - f: the frequency as it was asked for
//
// Returns:
//   - the frequency rounded to the nearest step
func round(f device.Frequency) device.Frequency {
	half := step / 2
	return (f + half) / step * step
}

// run tunes the scanner and reports what it landed on.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//   - typed: the frequency as it was typed, named in the rounding note
//   - want: the frequency that was asked for, before rounding
//
// Returns:
//   - error if the scanner cannot be reached, cannot receive the frequency,
//     refuses the tune, or the JSON cannot be written. A scanner that tuned but
//     would not say what it is hearing is a note rather than an error
func run(ctx context.Context, app *appcontext.App, typed string, want device.Frequency) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	rounded := round(want)
	if err := covered(rounded); err != nil {
		return err
	}
	if rounded != want {
		app.Notef("%s rounds to %s, which is as fine as the scanner tunes.\n",
			typed, rounded)
	}

	if err := client.QuickSearchHold(ctx, rounded); err != nil {
		// Being in a menu is the usual reason, and the fix is a command away,
		// so say so rather than leaving the reader with a bare rejection.
		if errors.Is(err, device.ErrRejected) {
			return refused(ctx, client, rounded)
		}
		return fmt.Errorf("tuning to %s: %w", rounded, err)
	}

	r := report{Megahertz: rounded.MHz()}

	// The scanner does not measure the new frequency instantly. Asked straight
	// away it reports no signal whatever is really there, so this waits for a
	// reading rather than taking the first one and calling a busy frequency
	// silent. It stops the moment something is heard, so only a genuinely quiet
	// frequency costs the full wait.
	property, err := listen(ctx, client)
	if err != nil {
		app.Notef("The scanner tuned, but would not report what it is hearing: %v\n", err)
	} else {
		r.RSSI = property.RSSI
		r.Bars = property.Signal
		r.Receiving = receiving(property)
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("frequency: %s\n", rounded)
	app.Printf("receiving: %s\n", render.YesNo(r.Receiving))
	if r.RSSI != "" {
		app.Printf("signal:    %s", r.RSSI)
		if r.Bars != "" {
			app.Printf(" (%s bars)", r.Bars)
		}
		app.Printf("\n")
	}
	return nil
}
