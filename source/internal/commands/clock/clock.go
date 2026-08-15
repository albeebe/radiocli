// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package clock implements the "clock" command, which reports the scanner's
// own date and time, and its "set" and "sync" subcommands, which change it.
//
// The bare command reads. Changing the scanner is a separate verb, so no
// reading of the clock can turn into a write by mistake.
package clock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the clock command, with its set and sync subcommands attached,
// bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that reports the scanner's clock when it runs, carrying
//     the set and sync subcommands that change it
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clock",
		Short: "Report the scanner's date and time",
		Long: "Clock reports the date and time the scanner is keeping, whether it is applying\n" +
			"daylight saving, and whether its clock is still running. The scanner keeps its\n" +
			"own clock, which can drift from the computer's or stop entirely if the scanner\n" +
			"is left without power.\n\n" +
			"Run \"radiocli clock sync\" to set it from this computer, or\n" +
			"\"radiocli clock set\" to give it a date and time of your own.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newSet(app), newSync(app))
	return cmd
}

// newReport turns a reading into the shape that gets rendered.
//
// Parameters:
//   - t: the scanner's date and time as it reported them
//   - dst: whether the scanner is applying daylight saving
//   - valid: whether the scanner's clock is still running
//
// Returns:
//   - report holding the date and the time of day as separate values, with the
//     two flags alongside them
func newReport(t time.Time, dst, valid bool) report {
	return report{
		// The scanner reports wall clock digits rather than an instant, so the
		// value is formatted as written rather than converted to a zone.
		Date:           t.Format("2006-01-02"),
		Time:           t.Format("15:04:05"),
		DaylightSaving: dst,
		Valid:          valid,
	}
}

// newSet returns the "clock set" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that writes the date and time it is given to the scanner
func newSet(app *appcontext.App) *cobra.Command {
	var dst bool

	cmd := &cobra.Command{
		Use:   "set <datetime>",
		Short: "Set the scanner's date and time",
		Long: "Set gives the scanner a date and time of your own choosing, written as\n" +
			"\"2026-08-02 14:30:00\", as a date alone such as \"2026-08-02\", or as a time\n" +
			"alone such as \"14:30\". It is read in this computer's time zone.\n\n" +
			"A date alone changes only the date, and a time alone changes only the time of\n" +
			"day. Whichever half is left out keeps the value the scanner already holds.\n\n" +
			"To set the scanner from this computer's own clock, use \"radiocli clock sync\".",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parsing before the scanner is opened means a badly written time
			// costs nothing and reports the same way whether or not a scanner
			// is attached.
			v, err := parseValue(args[0])
			if err != nil {
				return err
			}
			return apply(cmd.Context(), app, v, override(cmd, dst))
		},
	}

	addDSTFlag(cmd, &dst)
	return cmd
}

// newSync returns the "clock sync" subcommand.
//
// Parameters:
//   - app: application context the subcommand reads its settings, output
//     streams and scanner connection from
//
// Returns:
//   - *cobra.Command that writes this computer's current date and time to the
//     scanner
func newSync(app *appcontext.App) *cobra.Command {
	var dst bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Set the scanner's clock from this computer",
		Long: "Sync copies this computer's current date and time onto the scanner, which is\n" +
			"the usual way to correct a clock that has drifted or has been lost after the\n" +
			"scanner sat without power.\n\n" +
			"The time is sent as this computer reads it locally, since the scanner holds a\n" +
			"wall clock rather than a moment in time and has no notion of a time zone.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			now := value{t: time.Now(), hasDate: true, hasTime: true}
			return apply(cmd.Context(), app, now, override(cmd, dst))
		},
	}

	addDSTFlag(cmd, &dst)
	return cmd
}

// addDSTFlag registers the daylight saving override shared by both write
// commands.
//
// Parameters:
//   - cmd: the subcommand to register the flag on
//   - dst: where the flag's value is stored
func addDSTFlag(cmd *cobra.Command, dst *bool) {
	cmd.Flags().BoolVar(dst, "dst", false,
		"change the scanner's daylight saving flag (default: leave it as the scanner has it)")
}

// apply writes the clock, then reports what the scanner holds afterwards.
//
// A value giving only one half is completed from the scanner's own reading, so
// that setting the date leaves the time of day alone and setting the time
// leaves the date alone.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//   - v: the date and time to write, which may give only one half
//   - dst: what to set daylight saving to, or nil to keep what the scanner has
//
// Returns:
//   - error if the scanner cannot be reached, the clock cannot be read before
//     or after the change, or the clock cannot be set; nil once the new
//     reading has been reported
func apply(ctx context.Context, app *appcontext.App, v value, dst *bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// The scanner is read first whichever way this was called: a partial value
	// needs the half it left out, and the daylight saving flag is carried over
	// from what the scanner holds unless --dst was given.
	base, err := client.Clock(ctx)
	if err != nil {
		return fmt.Errorf("reading the clock before changing it: %w", err)
	}

	t := v.t
	if !v.complete() {
		// A stopped clock holds nothing worth preserving, so the half that was
		// left out has to come from somewhere. This computer is the only other
		// source, and saying so is better than quietly copying a reading the
		// scanner has already disowned.
		if !base.Valid {
			app.Notef("The scanner's clock is not running, so there is no %s to keep.\n"+
				"Taking it from this computer instead.\n\n", missing(v))
			base.Time = time.Now()
		}

		t = v.fill(base.Time)
	}

	if err := client.SetClock(ctx, t, daylightSaving(base, dst)); err != nil {
		return fmt.Errorf("setting the clock: %w", err)
	}

	// Read back rather than echo what was sent. The scanner is the authority
	// on the time it is keeping, and a value it quietly declined would
	// otherwise be reported as a success.
	c, err := client.Clock(ctx)
	if err != nil {
		return fmt.Errorf("reading the clock back: %w", err)
	}

	if err := renderClock(app, c.Time, newReport(c.Time, c.DaylightSaving, c.Valid)); err != nil {
		return err
	}

	if gap(c.Time, t) > tolerance {
		app.Notef("\nThe scanner is at %s, not the %s it was asked for.\n",
			c.Time.Format("2006-01-02 15:04:05"), t.Format("2006-01-02 15:04:05"))
	}
	return nil
}

// complete reports whether both halves were given, meaning nothing has to be
// read off the scanner first.
//
// Returns:
//   - true if the user gave both a date and a time of day, false otherwise
func (v value) complete() bool {
	return v.hasDate && v.hasTime
}

// daylightSaving decides what to tell the scanner.
//
// The flag the scanner already holds is kept unless the user asked to change
// it. Setting the time is not a reason to change a configuration flag, and on a
// scanner with a GPS this one is not cosmetic: the clock is re-derived from the
// satellites with the flag added to the scanner's configured offset, so setting
// it on a scanner already configured for summer time leaves it an hour fast
// within the hour.
//
// Deriving it from the computer's own daylight saving is what this used to do,
// and it is why a synced scanner drifted an hour ahead every time.
//
// Parameters:
//   - current: the clock as the scanner reported it, which carries the flag it
//     is already applying
//   - dst: what the user asked for with --dst, or nil when the flag was left
//     alone
//
// Returns:
//   - true if the scanner should apply daylight saving, false otherwise
func daylightSaving(current device.Clock, dst *bool) bool {
	if dst != nil {
		return *dst
	}
	return current.DaylightSaving
}

// describe rounds a drift to the largest unit that still says something
// useful, since the seconds in a multi-day gap are noise.
//
// Parameters:
//   - d: the drift between the two clocks, as a magnitude
//
// Returns:
//   - string naming the drift in days, hours or minutes, whichever is the
//     largest unit that still says something
func describe(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
}

// fill returns the time to send, taking whichever halves the user left out
// from base, the scanner's own current reading.
//
// Parameters:
//   - base: the scanner's own reading, which supplies whichever half the user
//     left out
//
// Returns:
//   - time.Time in the computer's zone, holding the user's halves where they
//     were given and the scanner's where they were not
func (v value) fill(base time.Time) time.Time {
	year, month, day := base.Date()
	if v.hasDate {
		year, month, day = v.t.Date()
	}

	hour, minute, second := base.Clock()
	if v.hasTime {
		hour, minute, second = v.t.Clock()
	}

	return time.Date(year, month, day, hour, minute, second, 0, time.Local)
}

// gap is the distance between two readings, as a magnitude. Both are read as
// wall clock digits in the local zone, which is the comparison a person makes
// by looking at the two.
//
// Parameters:
//   - a: one of the two readings
//   - b: the other reading
//
// Returns:
//   - time.Duration holding how far apart they are, never negative
func gap(a, b time.Time) time.Duration {
	d := a.Sub(b)
	if d < 0 {
		return -d
	}
	return d
}

// health says what the validity flag means, since "valid" describes the
// reading rather than the clock the reader is asking about.
//
// Parameters:
//   - valid: whether the scanner reported its clock as running
//
// Returns:
//   - string reading "running" or "not running"
func health(valid bool) string {
	if valid {
		return "running"
	}
	return "not running"
}

// missing names the half a value left out, for the message explaining where it
// had to be taken from instead.
//
// Parameters:
//   - v: the value that gave only one of the two halves
//
// Returns:
//   - string naming the half that was left out, either "time of day" or "date"
func missing(v value) string {
	if v.hasDate {
		return "time of day"
	}
	return "date"
}

// nearHour reports whether a gap is close enough to exactly an hour to be the
// daylight saving flag rather than a clock running fast. A scanner that has
// genuinely drifted lands on an arbitrary figure; this one lands within a
// minute or two of the hour.
//
// Parameters:
//   - d: the drift between the two clocks, as a magnitude
//
// Returns:
//   - true if the drift is within three minutes of exactly an hour, false
//     otherwise
func nearHour(d time.Duration) bool {
	return gap(time.Time{}.Add(d), time.Time{}.Add(time.Hour)) <= 3*time.Minute
}

// override reports what the user asked for with --dst, or nil when the flag
// was left alone. It is only consulted when actually typed, so the flag's
// "false" default cannot silently turn daylight saving off on a scanner that
// had it on.
//
// Parameters:
//   - cmd: the subcommand that was run, which knows whether the flag was typed
//   - dst: the value the flag was given
//
// Returns:
//   - *bool holding what the user asked for, or nil when --dst was not given
func override(cmd *cobra.Command, dst bool) *bool {
	if !cmd.Flags().Changed("dst") {
		return nil
	}
	return &dst
}

// parseValue reads a date and time written in any of the accepted forms,
// recording which halves were given so the rest can be kept as it is.
//
// Parameters:
//   - arg: the date and time as the user wrote it
//
// Returns:
//   - value holding what was parsed, and which of the two halves were given
//   - error if the argument matches none of the accepted forms
func parseValue(arg string) (value, error) {
	arg = strings.TrimSpace(arg)

	for _, layout := range fullLayouts {
		if t, err := time.ParseInLocation(layout, arg, time.Local); err == nil {
			return value{t: t, hasDate: true, hasTime: true}, nil
		}
	}

	// A date on its own is worth accepting, because moving a scanner to the
	// right day is a separate job from correcting the time it shows.
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, arg, time.Local); err == nil {
			return value{t: t, hasDate: true}, nil
		}
	}

	// A time on its own is worth accepting, because correcting the time of day
	// on a scanner whose date is already right is the common repair.
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, arg, time.Local); err == nil {
			return value{t: t, hasTime: true}, nil
		}
	}

	return value{}, fmt.Errorf("invalid date and time %q: want \"2026-08-02 14:30:00\", "+
		"a date alone such as \"2026-08-02\", or a time alone such as \"14:30\"", arg)
}

// renderClock writes the reading in whichever format was asked for.
//
// Parameters:
//   - app: application context holding the output format and the streams to
//     write to
//   - scanner: the scanner's reading, which the text output compares against
//     this computer's clock
//   - r: the reading as these commands report it
//
// Returns:
//   - error if the JSON cannot be written; nil for the text output, which
//     cannot fail
func renderClock(app *appcontext.App, scanner time.Time, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("date:     %s\n", r.Date)
	app.Printf("time:     %s\n", r.Time)
	app.Printf("daylight: %s\n", render.YesNo(r.DaylightSaving))
	app.Printf("clock:    %s\n", health(r.Valid))

	// A stopped clock makes the time above meaningless, and nothing in the
	// digits themselves says so.
	if !r.Valid {
		app.Notef("\nThe scanner reports its clock is not running, so the date and time above\n" +
			"are unreliable. Run \"radiocli clock sync\" once the scanner has held power\n" +
			"for a while.\n")
		return nil
	}

	// Drift is the reason to run this command, and it is easy to miss when the
	// two clocks are only minutes apart.
	if d := gap(scanner, time.Now()); d >= time.Minute {
		app.Notef("\nThe scanner is %s from this computer's clock.\n", describe(d))

		// A gap of almost exactly an hour on a scanner applying daylight
		// saving is not drift, and syncing will not fix it: the scanner adds
		// the hour to the offset it is already configured with. Saying "run
		// sync" here sends the reader round in a circle.
		if r.DaylightSaving && nearHour(d) {
			app.Notef("That is almost exactly an hour, and the scanner is applying daylight\n" +
				"saving. On this model that combination puts the clock an hour out however\n" +
				"often it is synced, because the hour is added to the offset the scanner is\n" +
				"already configured with. Try \"radiocli clock sync --dst=false\".\n")
		} else {
			app.Notef("Run \"radiocli clock sync\" to correct it.\n")
		}
	}
	return nil
}

// runGet reads the clock and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//
// Returns:
//   - error if the scanner cannot be reached, the clock cannot be read, or the
//     JSON cannot be written; nil once the reading has been reported
func runGet(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	c, err := client.Clock(ctx)
	if err != nil {
		return fmt.Errorf("reading the clock: %w", err)
	}

	return renderClock(app, c.Time, newReport(c.Time, c.DaylightSaving, c.Valid))
}
