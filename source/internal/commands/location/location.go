// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package location implements the "location" command, which reports the
// position the scanner is working from, and sets it from a zip code.
//
// The position is not simply where the scanner is. Left alone it follows the
// built-in GPS, but a zip code entered through the menus replaces it and keeps
// it replaced, so what this reports is the position the scanner is scanning
// around rather than the one it measured.
package location

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/albeebe/radiocli/internal/textinput"
	"github.com/spf13/cobra"
)

// New returns the location command bound to app.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "location" command, with its subcommands attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "location",
		Short: "Report the position the scanner is working from",
		Long: "Location reports the position the scanner is working from, and how far around\n" +
			"it it draws channels out of the database.\n\n" +
			"This is not simply where the scanner is. Left alone it follows the built-in\n" +
			"GPS, and a stationary scanner moves by a metre or two between readings as the\n" +
			"fix refines. But a zip code set through \"location set\" replaces the position\n" +
			"outright and keeps it replaced, across a power cycle, so what this reports is\n" +
			"the position the scanner is scanning around.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newSet(app))
	cmd.AddCommand(newGPS(app))
	return cmd
}

// newGPS returns the "location gps" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "location gps" subcommand, carrying its --wait, --off and --status
//     flags
func newGPS(app *appcontext.App) *cobra.Command {
	var wait, off, status bool

	cmd := &cobra.Command{
		Use:   "gps [--off] [--status]",
		Short: "Switch the built-in GPS on or off, or report which it is",
		Long: "GPS switches the scanner's GPS function on, so the position follows the\n" +
			"receiver again instead of a zip code that was set by hand.\n\n" +
			"Setting a zip code switches the GPS function off, which is why the position\n" +
			"stays where it was put. This is the way back. The scanner's own Set Your\n" +
			"Location menu has no entry for it: the setting lives under its GPS menu.\n\n" +
			"--off switches it off and leaves the position where it is, which is how a\n" +
			"position stays put without setting a zip code to pin it. --status reports\n" +
			"which of the two the setting holds and changes nothing.\n\n" +
			"Switching it on returns as soon as the scanner is following the GPS. Working\n" +
			"out where that is takes the receiver anything from twenty seconds to a couple\n" +
			"of minutes, and it does that on its own. Pass --wait to sit through it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGPS(cmd.Context(), app, wait, off, status)
		},
	}

	cmd.Flags().BoolVar(&wait, "wait", false,
		"wait for the receiver to produce a fix before returning")
	cmd.Flags().BoolVar(&off, "off", false,
		"switch the GPS off, leaving the position where it is")
	cmd.Flags().BoolVar(&status, "status", false,
		"report whether the GPS is on, and change nothing")
	return cmd
}

// newSet returns the "location set" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "location set" subcommand, carrying its --range and --position flags
func newSet(app *appcontext.App) *cobra.Command {
	var miles int
	var position string

	cmd := &cobra.Command{
		Use:   "set <zip> | --position <latitude>,<longitude>",
		Short: "Set the position from a zip code, or from a latitude and longitude",
		Long: "Set points the scanner at a zip code, so it draws database channels from\n" +
			"there rather than from wherever it is.\n\n" +
			"The zip is resolved by the scanner, not by this tool: the zip database lives\n" +
			"on the scanner, and it is the only thing here that can turn 12345 into a\n" +
			"position. So this drives the scanner's own menus, which means it stops\n" +
			"scanning while it runs and returns to scanning when it is done.\n\n" +
			"The position is read back afterwards to confirm it took, and it survives a\n" +
			"power cycle: the GPS does not reclaim it.\n\n" +
			"--position sets a latitude and longitude directly, without the menus and\n" +
			"without stopping the scan. It is how a position is put back where it was, since\n" +
			"the scanner keeps no record of the zip it was given.\n\n" +
			"Unlike a zip, a position does not switch the GPS off, and a scanner following\n" +
			"its GPS overwrites what is written here as soon as the receiver has a fix,\n" +
			"which was measured at just over a minute.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			zip := ""
			if len(args) == 1 {
				zip = args[0]
			}

			// Whether --range was typed matters as well as its value:
			// --position leaves the radius alone unless asked to change it,
			// and the default is indistinguishable from a radius of ten.
			return runSet(cmd.Context(), app, zip, position, miles, cmd.Flags().Changed("range"))
		},
	}

	cmd.Flags().IntVar(&miles, "range", defaultRange,
		fmt.Sprintf("radius in whole miles to draw database channels from (%d to %d)", minRange, maxRange))
	cmd.Flags().StringVar(&position, "position", "",
		"latitude and longitude to set directly, as \"38.433056,-79.839722\"")
	return cmd
}

// answerPrompts deals with whatever the scanner puts up after a zip code.
//
// Two prompts can appear, and the order is the whole point. Pressing enter on
// the zip raises "Turn on Full Database?" first, when the full database is not
// being scanned, because a location decides nothing without it. Only once that
// is answered does the scanner look the zip up, and only then can it say "Out
// of Range" for one it does not hold.
//
// Checking for the refusal before answering the first prompt therefore never
// sees it, and leaving the menus afterwards dismisses it unread. That is why
// this is a loop over whatever is on screen rather than two checks in a fixed
// order.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - client: the scanner to read the screen from and press the keys on
//   - zip: the zip code that was typed, for the message a refusal produces
//
// Returns:
//   - error if the screen cannot be read, if the scanner does not hold the zip
//     code, or if a prompt cannot be answered
func answerPrompts(ctx context.Context, app *appcontext.App, client *device.Scanner, zip string) error {
	for range promptRounds {
		shown, err := settled(ctx, client)
		if err != nil {
			return fmt.Errorf("reading the screen after entering the zip code: %w", err)
		}

		switch {
		case strings.Contains(shown, outOfRange):
			// The prompt wants a key before the scanner will do anything
			// else, so clear it rather than leaving it stuck there.
			menus.Confirm(ctx, client)
			menus.Leave(ctx, client)
			return fmt.Errorf("the scanner does not hold zip code %s: it answered %q\n"+
				"Nothing has been changed", zip, outOfRange)

		case strings.Contains(shown, turnOnFullDatabase):
			// Answering this is what sends the scanner off to rebuild, so its
			// acknowledgement can time out exactly as the zip's does.
			if err := menus.Confirm(ctx, client); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			app.Notef("Full Database scanning was switched on, because a location does nothing without it.\n")

			// The lookup happens after this is answered, and it can take the
			// scanner off the protocol, so wait before reading again.
			if err := menus.Awaken(ctx, client); err != nil {
				return err
			}

		default:
			return nil
		}
	}
	return nil
}

// awaitFix waits for the position to move off the one that was set by hand,
// and reports whether it did.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the position from
//   - before: the position held before the GPS was switched on
//
// Returns:
//   - the position last read from the scanner
//   - whether that position moved off before
//   - error if the context ends while waiting
func awaitFix(ctx context.Context, client *device.Scanner, before report) (report, bool, error) {
	var latest report
	for attempt := 0; attempt < fixAttempts; attempt++ {
		var err error
		if latest, err = read(ctx, client); err != nil {
			// A scanner still settling refuses the read rather than failing,
			// which is worth waiting through.
			if ctx.Err() != nil {
				return report{}, false, ctx.Err()
			}
			continue
		}
		if latest.Latitude != before.Latitude || latest.Longitude != before.Longitude {
			return latest, true, nil
		}

		select {
		case <-ctx.Done():
			return report{}, false, ctx.Err()
		case <-time.After(fixInterval):
		}
	}
	return latest, false, nil
}

// checkZip rejects anything that is not five digits, before the scanner is
// touched, so a typo costs nothing and leaves it scanning.
//
// Parameters:
//   - zip: the zip code as it was typed
//
// Returns:
//   - error if zip is not five digits
func checkZip(zip string) error {
	if len(zip) != 5 {
		return fmt.Errorf("%q is not a zip code: want five digits, such as 12345", zip)
	}
	for _, r := range zip {
		if r < '0' || r > '9' {
			return fmt.Errorf("%q is not a zip code: want five digits, such as 12345", zip)
		}
	}
	return nil
}

// confirmGPS reads the GPS function setting back, to check it took.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the setting from
//   - want: the value the setting was meant to take
//
// Returns:
//   - error if the setting cannot be read, or if it does not hold want
func confirmGPS(ctx context.Context, client *device.Scanner, want string) error {
	shown, err := readGPS(ctx, client)
	if err != nil {
		return err
	}

	if !strings.EqualFold(shown, want) {
		return fmt.Errorf("the scanner still shows %q for %q: it was not set to %q",
			shown, setGPSFunction, want)
	}
	return nil
}

// confirmPosition reports a position that did not take.
//
// Parameters:
//   - after: the position read back from the scanner
//   - latitude: the latitude the scanner was given
//   - longitude: the longitude the scanner was given
//
// Returns:
//   - error if the position read back is further than positionTolerance from
//     the one that was written
func confirmPosition(after report, latitude, longitude float64) error {
	if math.Abs(after.Latitude-latitude) <= positionTolerance &&
		math.Abs(after.Longitude-longitude) <= positionTolerance {
		return nil
	}

	return fmt.Errorf("the scanner is at %.6f, %.6f rather than the %.6f, %.6f it was given\n"+
		"A written position is overwritten by the receiver while the scanner is following its\n"+
		"GPS. Set a zip code, which switches the GPS off, or accept where the receiver says it is",
		after.Latitude, after.Longitude, latitude, longitude)
}

// digitKey turns a digit into the key that types it.
//
// Parameters:
//   - digit: the character to type
//
// Returns:
//   - the key that types digit
//   - whether digit is a digit at all
func digitKey(digit rune) (device.Key, bool) {
	if digit < '0' || digit > '9' {
		return "", false
	}
	return device.Key(string(digit)), true
}

// disableGPS switches the GPS off and leaves the position where it is.
//
// This is what makes a position stay put without setting a zip code to pin it,
// and it is the only way back from "location gps" that does not move the
// scanner somewhere else on the way.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//
// Returns:
//   - error if the scanner cannot be reached, the setting cannot be changed or
//     read back, the position cannot be read, or the output cannot be written
func disableGPS(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if err := setGPS(ctx, client, gpsDisable); err != nil {
		return err
	}
	if err := confirmGPS(ctx, client, gpsDisable); err != nil {
		return err
	}

	// Read after the change rather than before it, so what is reported is the
	// position the scanner is now holding on to.
	held, err := read(ctx, client)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, held)
	}

	app.Printf("source:    fixed\n")
	app.Printf("position:  %.6f, %.6f\n", held.Latitude, held.Longitude)
	if held.Range == 0 {
		app.Printf("range:     none set\n")
	} else {
		app.Printf("range:     %.1f miles\n", held.Range)
	}
	return nil
}

// parsePosition reads a latitude and longitude written as one argument.
//
// One argument rather than two, because a position is one thing: the pair is
// what a map accepts and what "location" prints, so it can be pasted straight
// back in.
//
// Parameters:
//   - value: a latitude and a longitude separated by a comma
//
// Returns:
//   - the latitude, in degrees from -90 to 90
//   - the longitude, in degrees from -180 to 180
//   - error if value is not a comma separated pair of degrees in range
func parsePosition(value string) (float64, float64, error) {
	latitude, longitude, found := strings.Cut(value, ",")
	if !found {
		return 0, 0, fmt.Errorf("%q is not a position: want a latitude and a longitude "+
			"separated by a comma, such as 38.433056,-79.839722", value)
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(latitude), 64)
	if err != nil || lat < -90 || lat > 90 {
		return 0, 0, fmt.Errorf("%q is not a latitude: want degrees from -90 to 90",
			strings.TrimSpace(latitude))
	}

	long, err := strconv.ParseFloat(strings.TrimSpace(longitude), 64)
	if err != nil || long < -180 || long > 180 {
		return 0, 0, fmt.Errorf("%q is not a longitude: want degrees from -180 to 180",
			strings.TrimSpace(longitude))
	}

	return lat, long, nil
}

// read fetches the position and range.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the position from
//
// Returns:
//   - the position and range the scanner is working from
//   - error if the scanner cannot be read
func read(ctx context.Context, client *device.Scanner) (report, error) {
	loc, err := client.Location(ctx)
	if err != nil {
		return report{}, fmt.Errorf("reading the location: %w", err)
	}
	return report{
		Latitude:  loc.Latitude,
		Longitude: loc.Longitude,
		Range:     loc.Range,
	}, nil
}

// readGPS reports which of the two values the GPS function setting holds.
//
// The scanner shows the value in force as the highlighted entry of the screen
// that sets it, so reading it means opening that screen. There is no way to
// ask over the protocol, which is why this stops the scan for a moment to
// answer a question that only reads.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the setting from
//
// Returns:
//   - the value the GPS function setting holds, as the scanner words it
//   - error if the setting screen cannot be reached, read, or left
func readGPS(ctx context.Context, client *device.Scanner) (string, error) {
	if err := toGPSFunction(ctx, client); err != nil {
		return "", err
	}

	shown, err := menus.Highlighted(ctx, client)
	if err != nil {
		return "", fmt.Errorf("reading the GPS setting: %w", err)
	}

	// Leave without pressing, so reading the setting cannot change it.
	if _, err := menus.Leave(ctx, client); err != nil {
		return "", err
	}
	return strings.TrimSpace(shown), nil
}

// reportGPS says whether the GPS is switched on, and changes nothing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//
// Returns:
//   - error if the scanner cannot be reached, the setting or the position
//     cannot be read, or the output cannot be written
func reportGPS(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	shown, err := readGPS(ctx, client)
	if err != nil {
		return err
	}

	where, err := read(ctx, client)
	if err != nil {
		return err
	}

	on := strings.EqualFold(shown, gpsEnable)

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, struct {
			GPS       bool    `json:"gps"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Range     float64 `json:"range"`
		}{on, where.Latitude, where.Longitude, where.Range})
	}

	// The scanner's own words for the two values are "Enable" and "Disable",
	// which name the action rather than the state and read as an instruction
	// here. What the reader asked is whether it is on.
	state := "off"
	if on {
		state = "on"
	}
	app.Printf("gps:       %s\n", state)
	app.Printf("position:  %.6f, %.6f\n", where.Latitude, where.Longitude)
	if where.Range == 0 {
		app.Printf("range:     none set\n")
	} else {
		app.Printf("range:     %.1f miles\n", where.Range)
	}
	return nil
}

// run reads the position and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//
// Returns:
//   - error if the scanner cannot be reached or read, or if the output cannot
//     be written
func run(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	r, err := read(ctx, client)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	// Six decimal places is what the scanner sends, and the last of them moves
	// between readings while the GPS is in charge, so rounding any harder
	// would hide the fix refining itself.
	app.Printf("latitude:  %.6f\n", r.Latitude)
	app.Printf("longitude: %.6f\n", r.Longitude)

	// The pair is worth repeating on one line because that is the form a map
	// takes, and reading two numbers off separate lines to paste them together
	// is the thing this command would otherwise be used for.
	app.Printf("position:  %.6f, %.6f\n", r.Latitude, r.Longitude)

	// A range of zero is not a radius of nothing, it is no range set at all,
	// which is how a scanner following its own GPS reads. Saying so is worth
	// more than printing "0.0 miles" and letting it be read as a distance.
	if r.Range == 0 {
		app.Printf("range:     none set (following the GPS)\n")
	} else {
		app.Printf("range:     %.1f miles\n", r.Range)
	}
	return nil
}

// runGPS switches the GPS on or off, or reports which it is.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - wait: whether to sit through the receiver working out where it is
//   - off: whether to switch the GPS off rather than on
//   - status: whether to report the setting and change nothing
//
// Returns:
//   - error if the flags contradict each other, the scanner cannot be reached,
//     the setting cannot be changed or read back, or the output cannot be
//     written
func runGPS(ctx context.Context, app *appcontext.App, wait, off, status bool) error {
	switch {
	case off && status:
		return fmt.Errorf("--off changes the setting and --status only reads it: choose one")
	case wait && status:
		return fmt.Errorf("--status only reads the setting, so there is no fix to wait for")
	case wait && off:
		return fmt.Errorf("--wait waits for the receiver to find the scanner, which is not " +
			"something switching the GPS off does")
	case status:
		return reportGPS(ctx, app)
	case off:
		return disableGPS(ctx, app)
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	before, err := read(ctx, client)
	if err != nil {
		return err
	}

	if err := setGPS(ctx, client, gpsEnable); err != nil {
		return err
	}

	// What this command does is change a setting, so that is what gets checked.
	// The position moving is a consequence of it, arriving on the receiver's
	// own schedule, and waiting for it would mean blocking on satellites to
	// report work that is already finished.
	if err := confirmGPS(ctx, client, gpsEnable); err != nil {
		return err
	}

	moved := false
	if wait {
		if _, moved, err = awaitFix(ctx, client, before); err != nil {
			return err
		}
	}

	// Read once at the end either way, so the position reported is the one the
	// scanner holds now rather than the one it held before the switch.
	latest, err := read(ctx, client)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, latest)
	}

	app.Printf("source:    GPS\n")
	app.Printf("position:  %.6f, %.6f\n", latest.Latitude, latest.Longitude)
	if latest.Range == 0 {
		app.Printf("range:     none set\n")
	} else {
		app.Printf("range:     %.1f miles\n", latest.Range)
	}

	// Saying which of the two the position is matters: they look identical.
	if !moved && latest.Latitude == before.Latitude && latest.Longitude == before.Longitude {
		app.Notef("That position is the one that was set by hand. The receiver has not " +
			"produced its own fix yet.\nRun \"radiocli location\" in a minute to see it move.\n")
	}
	return nil
}

// runPosition writes a latitude and longitude straight to the scanner.
//
// This is the one way back to a position that was set from a zip code. The
// scanner resolves a zip when it is typed and keeps only the result, so
// nothing can ask it which zip it was given, but the position it produced can
// be read and written like any other setting.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - position: a latitude and a longitude separated by a comma
//   - miles: the radius in whole miles to draw database channels from
//   - ranged: whether --range was typed, without which the radius is left alone
//
// Returns:
//   - error if the position or the range is not usable, the scanner cannot be
//     reached, the position cannot be written or read back, the scanner ended
//     up somewhere else, or the output cannot be written
func runPosition(ctx context.Context, app *appcontext.App, position string, miles int, ranged bool) error {
	latitude, longitude, err := parsePosition(position)
	if err != nil {
		return err
	}
	if ranged && (miles < minRange || miles > maxRange) {
		return fmt.Errorf("range %d is out of range: want %d to %d whole miles", miles, minRange, maxRange)
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// The radius is a separate setting and is left alone unless --range was
	// typed, so moving a position does not quietly widen or narrow it.
	before, err := read(ctx, client)
	if err != nil {
		return err
	}
	radius := before.Range
	if ranged {
		radius = float64(miles)
	}

	if err := client.SetLocation(ctx, device.Location{
		Latitude:  latitude,
		Longitude: longitude,
		Range:     radius,
	}); err != nil {
		return fmt.Errorf("setting the position: %w", err)
	}

	// Read back rather than reporting what was asked for. The scanner is the
	// authority on where it ended up, and it is the only thing that would
	// notice a position it declined.
	after, err := read(ctx, client)
	if err != nil {
		return err
	}
	if err := confirmPosition(after, latitude, longitude); err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, after)
	}

	app.Printf("position:  %.6f, %.6f\n", after.Latitude, after.Longitude)
	if after.Range == 0 {
		app.Printf("range:     none set\n")
	} else {
		app.Printf("range:     %.1f miles\n", after.Range)
	}
	return nil
}

// runSet decides which of the two ways of saying where was meant.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - zip: the zip code that was typed, or empty
//   - position: the latitude and longitude that was passed, or empty
//   - miles: the radius in whole miles to draw database channels from
//   - ranged: whether --range was typed, without which the radius is left alone
//
// Returns:
//   - error if neither or both ways of saying where were given, or whatever
//     setting the position reports
func runSet(ctx context.Context, app *appcontext.App, zip, position string, miles int, ranged bool) error {
	switch {
	case zip != "" && position != "":
		return fmt.Errorf("a zip code and --position are two ways of saying the same thing: choose one")
	case zip == "" && position == "":
		return fmt.Errorf("name a zip code, or pass --position\n" +
			"A zip is resolved by the scanner; --position takes a latitude and longitude")
	case position != "":
		return runPosition(ctx, app, position, miles, ranged)
	}
	return runZip(ctx, app, zip, miles)
}

// runZip points the scanner at a zip code and gives it a range.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - zip: the zip code to point the scanner at
//   - miles: the radius in whole miles to draw database channels from
//
// Returns:
//   - error if the zip code or the range is not usable, the scanner cannot be
//     reached, the zip code or the range cannot be set, the position cannot be
//     read back, or the output cannot be written
func runZip(ctx context.Context, app *appcontext.App, zip string, miles int) error {
	if err := checkZip(zip); err != nil {
		return err
	}
	if miles < minRange || miles > maxRange {
		return fmt.Errorf("range %d is out of range: want %d to %d whole miles", miles, minRange, maxRange)
	}

	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if err := setZip(ctx, app, client, zip); err != nil {
		return err
	}
	if err := setRange(ctx, client, miles); err != nil {
		return fmt.Errorf("the position was set to %s, but its range was not: %w\n"+
			"Run \"radiocli location\" to see where the scanner now is", zip, err)
	}

	// Read back rather than reporting what was asked for, because the scanner
	// is the only authority on where it ended up.
	after, err := read(ctx, client)
	if err != nil {
		return err
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, after)
	}

	app.Printf("zip:       %s\n", zip)
	app.Printf("position:  %.6f, %.6f\n", after.Latitude, after.Longitude)
	app.Printf("range:     %.1f miles\n", after.Range)
	return nil
}

// screenText returns the scanner's display as one string.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the display from
//
// Returns:
//   - the display's lines, one per line, in the order they are shown
//   - error if the display cannot be read
func screenText(ctx context.Context, client *device.Scanner) (string, error) {
	display, err := client.Display(ctx)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for _, line := range display.Lines {
		text.WriteString(line.Text)
		text.WriteString("\n")
	}
	return text.String(), nil
}

// setGPS walks to the GPS function setting and selects one of its two values.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk through the menus
//   - value: the value to select, either gpsEnable or gpsDisable
//
// Returns:
//   - error if the setting cannot be reached, value cannot be found, or the
//     menus cannot be left
func setGPS(ctx context.Context, client *device.Scanner, value string) error {
	if err := toGPSFunction(ctx, client); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, value); err != nil {
		return fmt.Errorf("looking for %q: %w", value, err)
	}

	_, err := menus.Leave(ctx, client)
	return err
}

// setRange walks to the range screen and types a radius in miles.
//
// This types keys directly rather than going through textinput, because the
// range screen is neither of the two things textinput knows how to drive. It
// holds a fixed width value like "10.0" that cannot be cleared, and a digit
// does not append: it overwrites the character under a cursor that then
// advances. Typing "0" then "5" over "10.0" leaves "05.0", and further presses
// do nothing, so the two integer digits are the whole of what can be set.
//
// That is also why the range is whole miles. The tenth of a mile the screen
// displays sits past where typing can reach.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk through the menus and press the keys on
//   - miles: the radius in whole miles, which has to fit in two digits
//
// Returns:
//   - error if the range screen cannot be reached, a key cannot be pressed, the
//     screen does not show what was typed, or the value cannot be saved
func setRange(ctx context.Context, client *device.Scanner, miles int) error {
	if err := toLocationMenu(ctx, client); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, setRangeEntry); err != nil {
		return fmt.Errorf("looking for %q: %w", setRangeEntry, err)
	}

	// Always two digits, because each press lands on a fixed position: 5 miles
	// is "05", and typing a bare "5" would set the tens digit instead.
	for _, digit := range fmt.Sprintf("%02d", miles) {
		key, ok := digitKey(digit)
		if !ok {
			return fmt.Errorf("entering the range: %q is not a digit", digit)
		}
		if err := client.PressKey(ctx, key, device.KeyPress); err != nil {
			return fmt.Errorf("entering the range: %w", err)
		}
	}

	if err := verifyRange(ctx, client, miles); err != nil {
		// Leave without saving, so a misread screen does not commit a range
		// nobody asked for.
		menus.Leave(ctx, client)
		return err
	}

	// Accepting a range re-filters the database, which takes the scanner off
	// the protocol exactly as accepting a position does.
	if err := menus.Commit(ctx, client); err != nil {
		return err
	}
	if err := menus.Settle(ctx, client); err != nil {
		return err
	}

	_, err := menus.Leave(ctx, client)
	return err
}

// setZip walks to the zip screen, types the zip, and answers what follows.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context holding the scanner and the streams to
//     write to
//   - client: the scanner to walk through the menus and type into
//   - zip: the zip code to type
//
// Returns:
//   - error if the zip screen cannot be reached, the country or the zip code
//     cannot be entered, the scanner does not hold the zip code, or the menus
//     cannot be left
func setZip(ctx context.Context, app *appcontext.App, client *device.Scanner, zip string) error {
	if err := toLocationMenu(ctx, client); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, enterZipCode); err != nil {
		return fmt.Errorf("looking for %q: %w", enterZipCode, err)
	}

	// The zip screen asks which country first, because a zip means different
	// places in each.
	if err := menus.Select(ctx, client, countryUSA); err != nil {
		return fmt.Errorf("choosing %q: %w", countryUSA, err)
	}

	if err := textinput.Set(ctx, client, zip); err != nil {
		return fmt.Errorf("entering the zip code: %w\n"+
			"Nothing has been changed. Run \"radiocli scan\" to leave the scanner as it was", err)
	}
	// No enter here: textinput.Set ends with one. Pressing a second dismissed
	// the scanner's "Out of Range" refusal as the "Press Any Key" it asks for,
	// destroying the answer before it could be read.
	if err := menus.Awaken(ctx, client); err != nil {
		return err
	}

	if err := answerPrompts(ctx, app, client, zip); err != nil {
		return err
	}

	if err := menus.Settle(ctx, client); err != nil {
		return err
	}

	_, err := menus.Leave(ctx, client)
	return err
}

// settled reads the screen once it has left the zip entry screen.
//
// Accepting the zip and refusing it both take a moment to draw, and the entry
// screen is still showing in between. Reading once, immediately, catches that
// in-between state and concludes there is no prompt, which is exactly the
// mistake that hid the refusal: "Out of Range" was always going to appear, a
// beat after the first look.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the display from
//
// Returns:
//   - the screen as one string, which is the entry screen itself if it never
//     changed
//   - error if the display cannot be read
func settled(ctx context.Context, client *device.Scanner) (string, error) {
	var shown string
	for attempt := 0; attempt < settleAttempts; attempt++ {
		var err error
		if shown, err = screenText(ctx, client); err != nil {
			return "", err
		}
		if !strings.Contains(shown, enterZipCode) {
			return shown, nil
		}
	}

	// Still on the entry screen after all that. Report it as it is and let the
	// caller decide, rather than pretending it settled.
	return shown, nil
}

// toGPSFunction puts the scanner on the GPS function setting, starting from
// the top menu so the walk is the same whatever was on screen before.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk through the menus
//
// Returns:
//   - error if the top menu cannot be opened, or if either step of the walk
//     cannot be found
func toGPSFunction(ctx context.Context, client *device.Scanner) error {
	if err := client.OpenMenu(ctx, device.MenuTop, ""); err != nil {
		return fmt.Errorf("opening the top menu: %w", err)
	}
	if err := menus.Select(ctx, client, gpsMenu); err != nil {
		return fmt.Errorf("looking for %q in the top menu: %w", gpsMenu, err)
	}
	if err := menus.Select(ctx, client, setGPSFunction); err != nil {
		return fmt.Errorf("looking for %q: %w", setGPSFunction, err)
	}
	return nil
}

// toLocationMenu puts the scanner on the location menu, starting from the top
// menu so the walk is the same whatever was on screen before.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk through the menus
//
// Returns:
//   - error if the walk fails for anything other than a scanner that is still
//     busy, or if it is still busy after menuAttempts tries
func toLocationMenu(ctx context.Context, client *device.Scanner) error {
	// A scanner that has just taken a new position answers the protocol again
	// before it is willing to act on keys, so the walk is retried rather than
	// abandoned when a press goes unacknowledged.
	var err error
	for attempt := 0; attempt < menuAttempts; attempt++ {
		if err = walkToLocationMenu(ctx, client); err == nil {
			return nil
		}
		if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return err
		}
		if err := menus.Awaken(ctx, client); err != nil {
			return err
		}
	}
	return err
}

// verifyRange checks the screen shows the range asked for, before it is saved.
//
// The value is read back here rather than after saving because this screen can
// be left without committing, so a mistake caught now costs nothing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the screen from
//   - miles: the radius that was typed
//
// Returns:
//   - error if the screen cannot be read, or if it shows anything other than
//     miles
func verifyRange(ctx context.Context, client *device.Scanner, miles int) error {
	shown, err := menus.Highlighted(ctx, client)
	if err != nil {
		return fmt.Errorf("reading the range back: %w", err)
	}

	want := fmt.Sprintf("%02d.0", miles)
	if strings.TrimSpace(shown) != want {
		return fmt.Errorf("the range screen shows %q after typing %d, not %q: "+
			"nothing has been saved", strings.TrimSpace(shown), miles, want)
	}
	return nil
}

// walkToLocationMenu is one attempt at the walk.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk through the menus
//
// Returns:
//   - error if the top menu cannot be opened, or if the location entry cannot
//     be found in it
func walkToLocationMenu(ctx context.Context, client *device.Scanner) error {
	if err := client.OpenMenu(ctx, device.MenuTop, ""); err != nil {
		return fmt.Errorf("opening the top menu: %w", err)
	}
	if err := menus.Select(ctx, client, setYourLocation); err != nil {
		return fmt.Errorf("looking for %q in the top menu: %w", setYourLocation, err)
	}
	return nil
}
