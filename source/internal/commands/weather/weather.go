// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package weather implements the "weather" command, which puts the scanner on
// the NOAA weather channels and takes it off them again.
//
// The scanner does two different things on those channels and calls both of
// them "WX Scan". Monitor Weather, which this command starts, plays the
// broadcast. Weather Alert sits on a channel silent and unmutes only when an
// alert tone arrives. Nothing in the mode or the screen name separates them:
// both report a mode of "WX Scan" and a screen of "wx_alert", and only the
// WxMode element the scanner reports alongside says which is running.
//
// Monitor Weather is reached by choosing "Weather Scan" in the WX Operation
// menu, and there is no protocol command for it. The protocol's own jump to
// weather ("JPM,WX_MODE") starts Weather Alert instead, which looks identical
// on screen apart from one word and plays nothing, so this walks the menu.
//
// What the scanner does next is the reason this command does not stop there.
// Left to itself it works through the seven channels and stops on the first
// one that opens its squelch, which is not the strongest and is sometimes not
// receivable at all: it was seen parked on channel 2 at -108 dBm, silent, with
// channel 7 audible at -97 dBm the whole time. So this holds the scanner,
// measures every one of the seven itself, and parks it on the best of them.
// Measuring costs a few seconds and is the whole value of the command.
//
// Getting back out is the other half. A scanner left on the weather channels
// is not in a menu and is not holding a channel in the sense "scan" means, so
// "radiocli scan" reports it is already out of the menus and leaves it there.
// "radiocli weather stop" is the way back.
package weather

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the weather command bound to app.
//
// Parameters:
//   - app: the application context the command and its subcommand read the
//     scanner and the output settings from
//
// Returns:
//   - the "weather" command, with its "stop" subcommand attached
func New(app *appcontext.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "weather",
		Short: "Scan the NOAA weather channels and hold the strongest",
		Long: "Weather puts the scanner on the NOAA weather channels, measures all seven, and\n" +
			"parks it on the one with the strongest signal.\n\n" +
			"The measuring is the point. Left to itself the scanner stops on the first\n" +
			"channel that opens its squelch, which is not the strongest and is sometimes not\n" +
			"receivable at all. This holds the scanner, steps it through every channel,\n" +
			"reads the signal on each, and then goes back to the best one, which takes a few\n" +
			"seconds. It reports what it found on all seven, so the choice can be checked.\n\n" +
			"This is not the scanner's weather alert standby, which sits on a channel silent\n" +
			"and unmutes only for an alert tone. The two look almost identical on screen and\n" +
			"this command starts the one that plays.\n\n" +
			"It is done through the scanner's menus, so it stops the scan on the way. It does\n" +
			"not return the scanner to scanning afterwards, because being on the weather\n" +
			"channels is the whole point of it.\n\n" +
			"Run \"radiocli weather stop\" to come back. \"radiocli scan\" will not: the\n" +
			"scanner is not in a menu and is not holding in the way that command means, so\n" +
			"it finds nothing to do.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd.Context(), app)
		},
	}

	cmd.AddCommand(newStop(app))
	return cmd
}

// newStop returns the "weather stop" subcommand.
//
// Parameters:
//   - app: the application context the command reads the scanner and the
//     output settings from
//
// Returns:
//   - the "stop" subcommand
func newStop(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Leave the weather channels and go back to scanning",
		Long: "Stop takes the scanner off the weather channels and returns it to scanning\n" +
			"whatever it was scanning before.\n\n" +
			"It is the way back from \"radiocli weather\". A scanner on the weather channels\n" +
			"is not in a menu, and is not holding a channel in the sense \"radiocli scan\"\n" +
			"means, so that command reports it is already out of the menus and leaves it\n" +
			"where it is.\n\n" +
			"Running it on a scanner that is not on the weather channels does nothing at\n" +
			"all. It opens no menus.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd.Context(), app)
		},
	}
}

// describe turns the sweep into what this command renders, in channel order
// rather than the order the sweep happened to visit them in.
//
// Parameters:
//   - found: the channels the sweep measured
//   - selected: the channel the scanner was left on, or the empty string when
//     it was left on none of them
//
// Returns:
//   - every channel and what was heard on it, in channel order
func describe(found []measurement, selected string) []channel {
	out := make([]channel, 0, len(found))
	for _, m := range found {
		out = append(out, channel{
			Number:    m.Number,
			Frequency: m.Frequency,
			Signal:    m.Signal,
			Selected:  m.Number == selected,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		a, aErr := strconv.Atoi(out[i].Number)
		b, bErr := strconv.Atoi(out[j].Number)
		if aErr != nil || bErr != nil {
			return out[i].Number < out[j].Number
		}
		return a < b
	})
	return out
}

// enterMonitor walks the menu that starts the broadcast and checks the scanner
// arrived in the mode that plays rather than the one that waits.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//
// Returns:
//   - error if the weather menu cannot be opened, the entry cannot be chosen,
//     the scanner does not settle on the weather channels, or it started the
//     silent standby rather than the broadcast
func enterMonitor(ctx context.Context, client *device.Scanner) error {
	// The scanner opens WX Operation with "Weather Scan" already highlighted,
	// which is the entry wanted, but stepping to it by name is what makes that
	// not have to be true.
	if err := client.OpenMenu(ctx, device.MenuWeather, ""); err != nil {
		return fmt.Errorf("opening the weather menu: %w", err)
	}
	if err := menus.Select(ctx, client, weatherScan); err != nil {
		return fmt.Errorf("choosing %q: %w\n"+
			"Run \"radiocli scan\" to return the scanner to scanning", weatherScan, err)
	}

	// Choosing that entry closes the menu by itself and starts the broadcast,
	// so there is nothing to leave. Read the scanner back rather than trusting
	// the press: the entry next to it starts the silent standby instead.
	state, err := settle(ctx, client, func(info device.ScannerInfo) bool {
		return info.Weather.Mode != ""
	})
	if err != nil {
		return err
	}

	switch state.Weather.Mode {
	case monitorWeather:
		return nil
	case weatherAlert:
		return fmt.Errorf("the scanner started %q rather than %q: it is sitting on a weather "+
			"channel silent, and will unmute only for an alert tone", weatherAlert, monitorWeather)
	}
	return fmt.Errorf("the scanner reports %q on the weather channels, which is neither %q nor %q",
		state.Weather.Mode, monitorWeather, weatherAlert)
}

// holding words a hold state for a message.
//
// Parameters:
//   - info: what the scanner says it is doing
//
// Returns:
//   - "holding" or "not holding"
func holding(info device.ScannerInfo) string {
	if info.Weather.Channel.Held == held {
		return "holding"
	}
	return "not holding"
}

// measure reads the channel the scanner is sitting on.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//
// Returns:
//   - the channel and the strongest reading taken on it
//   - error if the scanner could not be read, the context was cancelled, or
//     the scanner would not say which channel it is on
func measure(ctx context.Context, client *device.Scanner) (measurement, error) {
	var m measurement

	for waited := time.Duration(0); waited < dwell; waited += dwellGap {
		info, err := client.ScannerInfo(ctx)
		if err != nil {
			return measurement{}, fmt.Errorf("reading a weather channel: %w", err)
		}

		// A reading taken too soon after the knob turns can still describe the
		// channel being left, so anything collected before the channel number
		// last changed is thrown away rather than credited to this one. Left
		// in, the strong channel the sweep just came off makes the next one
		// look strong too, and the sweep picks the wrong winner.
		if info.Weather.Channel.Number != m.Number {
			m = measurement{
				Number:    info.Weather.Channel.Number,
				Frequency: info.Weather.Channel.Frequency,
			}
		}

		if rssi, ok := signal(info); ok && (m.Signal == nil || rssi > *m.Signal) {
			strength := rssi
			m.Signal = &strength
		}

		select {
		case <-ctx.Done():
			return measurement{}, ctx.Err()
		case <-time.After(dwellGap):
		}
	}

	if m.Number == "" {
		return measurement{}, fmt.Errorf("the scanner would not say which weather channel it is on")
	}
	return m, nil
}

// renderReport writes what the scanner is doing on the weather channels.
//
// Parameters:
//   - app: the application context the output is written through
//   - r: what the scanner is doing, and what the sweep found
//
// Returns:
//   - error if the JSON encoding or the table could not be written
func renderReport(app *appcontext.App, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	if !r.Scanning {
		app.Printf("weather: off\n")
		return nil
	}

	app.Printf("weather:   %s\n", r.Mode)
	if r.Receiving {
		app.Printf("channel:   %s\n", r.Channel)
		app.Printf("frequency: %s\n", r.Frequency)
		app.Printf("signal:    %s\n", strength(r.Signal))
	} else {
		app.Printf("channel:   none receivable\n")
	}

	if len(r.Channels) == 0 {
		return nil
	}

	// The sweep is printed whichever channel won, because it is the evidence
	// for the choice: without it there is no telling a strong channel picked
	// well from the only one that answered.
	app.Printf("\n")
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CHANNEL\tFREQUENCY\tSIGNAL\t")
	for _, c := range r.Channels {
		mark := ""
		if c.Selected {
			mark = "holding"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Number, c.Frequency, strength(c.Signal), mark)
	}
	return w.Flush()
}

// runStart puts the scanner on the best weather channel it can hear.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, the broadcast cannot be started,
//     the scanner cannot be held or released, a channel cannot be measured, the
//     winning channel cannot be returned to, or the result cannot be written
func runStart(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	if err := enterMonitor(ctx, client); err != nil {
		return err
	}

	// Hold before measuring, or the scanner moves under the sweep and every
	// reading belongs to a channel other than the one it is recorded against.
	if err := setHold(ctx, client, true); err != nil {
		return err
	}

	app.Notef("Measuring all %d weather channels.\n", channelCount)
	found, err := sweep(ctx, client)
	if err != nil {
		return err
	}

	best := strongest(found)
	if best < 0 {
		// Nothing anywhere. Let the scanner go back to trying on its own
		// rather than parking it on a channel chosen for no reason.
		if err := setHold(ctx, client, false); err != nil {
			return err
		}
		app.Notef("None of the %d weather channels can be heard from here.\n", channelCount)
		return renderReport(app, report{
			Scanning: true,
			Mode:     monitorWeather,
			Channels: describe(found, ""),
		})
	}

	on, err := stepTo(ctx, client, found[best].Number)
	if err != nil {
		return err
	}

	return renderReport(app, report{
		Scanning:  true,
		Mode:      monitorWeather,
		Receiving: true,
		Channel:   on.Number,
		Frequency: on.Frequency,
		Signal:    found[best].Signal,
		Channels:  describe(found, on.Number),
	})
}

// runStop returns the scanner to ordinary scanning.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the scanner and the output are taken from
//
// Returns:
//   - error if the scanner cannot be opened, cannot be asked what it is doing,
//     cannot be told to scan, does not leave the weather channels, or the
//     result cannot be written
func runStop(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	state, err := client.ScannerInfo(ctx)
	if err != nil {
		return fmt.Errorf("asking the scanner what it is doing: %w", err)
	}
	if state.Weather.Mode == "" {
		app.Notef("The scanner is not on the weather channels.\n")
		return renderReport(app, report{})
	}

	// The key labelled "to Scan" on the screen does this too, but the
	// protocol's own jump says which mode it wants rather than depending on
	// what the soft keys happen to be at the time. It releases the hold on the
	// way, so nothing has to be undone first.
	if err := client.JumpToMode(ctx, device.ScanModeScan, ""); err != nil {
		return fmt.Errorf("returning the scanner to scanning: %w", err)
	}

	if _, err := settle(ctx, client, func(info device.ScannerInfo) bool {
		return info.Weather.Mode == ""
	}); err != nil {
		return err
	}

	app.Notef("The scanner has left the weather channels and is scanning again.\n")
	return renderReport(app, report{})
}

// setHold parks the scanner on the channel it is on, or lets it go.
//
// The key toggles rather than sets, so this reads the scanner first. Pressing
// without checking is how a command that means to hold releases instead.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to park or release
//   - want: true to hold the channel, false to let the scanner go
//
// Returns:
//   - error if the scanner could not be asked whether it is holding, the hold
//     key could not be pressed, or it did not end up the way it was asked to
func setHold(ctx context.Context, client *device.Scanner, want bool) error {
	info, err := client.ScannerInfo(ctx)
	if err != nil {
		return fmt.Errorf("asking whether the scanner is holding a weather channel: %w", err)
	}
	if (info.Weather.Channel.Held == held) == want {
		return nil
	}

	if err := client.PressKey(ctx, device.KeySoft3, device.KeyPress); err != nil {
		return fmt.Errorf("pressing the hold key: %w", err)
	}

	after, err := settle(ctx, client, func(info device.ScannerInfo) bool {
		return (info.Weather.Channel.Held == held) == want
	})
	if err != nil {
		return fmt.Errorf("the scanner is %s a weather channel after pressing hold, wanted the other: %w",
			holding(after), err)
	}
	return nil
}

// settle waits until what the scanner reports satisfies want, and returns the
// reading that did. On giving up it returns the last reading it took, so a
// caller can say what the scanner was doing instead.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read
//   - want: reports whether a reading is the state being waited for
//
// Returns:
//   - the reading that satisfied want, or the last one taken when none did
//   - error if the context was cancelled, or the scanner had not settled after
//     settlePolls looks
func settle(ctx context.Context, client *device.Scanner, want func(device.ScannerInfo) bool) (device.ScannerInfo, error) {
	var last device.ScannerInfo

	for poll := 0; poll < settlePolls; poll++ {
		info, err := client.ScannerInfo(ctx)
		if err == nil {
			if want(info) {
				return info, nil
			}
			last = info
		}

		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(settleGap):
		}
	}

	return last, fmt.Errorf("the scanner is still in %q after %s",
		last.Mode, time.Duration(settlePolls)*settleGap)
}

// signal reads the signal strength in dBm, and reports whether there was one.
//
// The scanner answers -999 for a channel it is hearing nothing on, which has
// to be told from a genuine reading rather than compared with one: taken at
// face value it is the weakest number in the sweep, and a channel with no
// station on it would beat nothing but never lose.
//
// Parameters:
//   - info: what the scanner says it is hearing
//
// Returns:
//   - the strength in dBm, or zero when there was nothing to read
//   - true when there was a genuine reading
func signal(info device.ScannerInfo) (int, bool) {
	rssi, err := strconv.Atoi(info.Property.RSSI)
	if err != nil || rssi <= noSignal {
		return 0, false
	}
	return rssi, true
}

// stepTo turns the knob until the scanner is on the channel named want, and
// returns what it reports there.
//
// The sweep ends where it began, so this is how the scanner gets back to the
// channel that won. It compares the channel the scanner names rather than
// counting turns, for the same reason every other walk in this tool does.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to step
//   - want: the channel to stop on, as the screen labels it
//
// Returns:
//   - the channel the scanner reports once it is there
//   - error if the scanner could not be asked which channel it is on, the knob
//     could not be turned, or the channel was not reached in one time round
func stepTo(ctx context.Context, client *device.Scanner, want string) (device.WeatherChannel, error) {
	var seen []string

	for step := 0; step <= channelCount; step++ {
		info, err := client.ScannerInfo(ctx)
		if err != nil {
			return device.WeatherChannel{}, fmt.Errorf("asking which weather channel the scanner is on: %w", err)
		}
		if info.Weather.Channel.Number == want {
			return info.Weather.Channel, nil
		}
		seen = append(seen, info.Weather.Channel.Number)

		if err := turn(ctx, client); err != nil {
			return device.WeatherChannel{}, err
		}
	}

	return device.WeatherChannel{}, fmt.Errorf(
		"could not get back to weather channel %s: the scanner stopped on %v instead", want, seen)
}

// strength words a signal reading, which is absent rather than zero when the
// scanner heard nothing.
//
// Parameters:
//   - rssi: the reading in dBm, or nil when the scanner heard nothing
//
// Returns:
//   - the reading with its unit, or "-" when there was none
func strength(rssi *int) string {
	if rssi == nil {
		return "-"
	}
	return fmt.Sprintf("%d dBm", *rssi)
}

// strongest returns the index of the best channel found, or -1 when none of
// them answered.
//
// Ties go to the first, which is the channel the sweep reached first. Nothing
// distinguishes two channels reading the same strength, and picking the same
// one every time is worth more than picking either.
//
// Parameters:
//   - found: the channels the sweep measured
//
// Returns:
//   - the index of the strongest channel, or -1 when none of them answered
func strongest(found []measurement) int {
	best := -1
	for i, m := range found {
		if m.Signal == nil {
			continue
		}
		if best < 0 || *m.Signal > *found[best].Signal {
			best = i
		}
	}
	return best
}

// sweep steps the scanner through every weather channel and measures each one.
//
// The scanner must already be held, or it moves on its own between readings.
// It is left on whichever channel the sweep ends on, which is the one it
// started from: stepping through all seven comes back round.
//
// Each channel is read several times and the strongest reading kept, because a
// channel that is barely there flickers between a number and nothing, and one
// sample lands on whichever it happened to be.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to step through the channels
//
// Returns:
//   - every channel and the best reading taken on it, in the order they were
//     visited
//   - error if a channel could not be measured or the knob could not be turned
func sweep(ctx context.Context, client *device.Scanner) ([]measurement, error) {
	found := make([]measurement, 0, channelCount)

	for step := 0; step < channelCount; step++ {
		m, err := measure(ctx, client)
		if err != nil {
			return nil, err
		}
		found = append(found, m)

		if err := turn(ctx, client); err != nil {
			return nil, err
		}
	}

	return found, nil
}

// turn moves the scanner on one weather channel.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to turn the knob on
//
// Returns:
//   - error if the knob could not be turned or the context was cancelled
func turn(ctx context.Context, client *device.Scanner) error {
	if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
		return fmt.Errorf("turning the knob to the next weather channel: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(turnGap):
	}
	return nil
}
