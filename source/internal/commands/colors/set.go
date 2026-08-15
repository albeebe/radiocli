// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package colors

import (
	"context"
	"fmt"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// newSet returns the "colors set" subcommand.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that changes one area's text or background color when it
//     runs
func newSet(app *appcontext.App) *cobra.Command {
	var text, back, layoutName string

	cmd := &cobra.Command{
		Use:   "set <area>",
		Short: "Set the text or background color of one area",
		Long: "Set changes the color the scanner draws one area of its screen in.\n\n" +
			"The area is named the way the scanner names it, such as \"System_name\".\n" +
			"Run \"radiocli colors\" to see them, or \"radiocli colors --positions\"\n" +
			"for the same list without the wait.\n\n" +
			"Colors are named, not numbered. The scanner offers 147 of them, which are\n" +
			"CSS color names, and its picker has no way to enter a value. Note that its\n" +
			"colors are not quite the CSS ones: its Orangered is #FF4600 where CSS says\n" +
			"#FF4500. The name is a label for the scanner's color.\n\n" +
			"By default this changes the layout the scanner is drawing with right now.\n" +
			"Pass --layout to change another one.\n\n" +
			"This writes to the scanner. It reports what each color was before, and\n" +
			"reads back what it wrote, so a change that did not take is reported rather\n" +
			"than assumed. Setting a color to the one it already is does nothing.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if text == "" && back == "" {
				return fmt.Errorf("nothing to set: pass --text, --back, or both")
			}
			for _, want := range []string{text, back} {
				if want == "" {
					continue
				}
				if _, ok := color(want); !ok {
					return fmt.Errorf("%q is not a color the scanner offers: %s",
						want, suggest(want))
				}
			}
			if layoutName != "" {
				if _, ok := lookup(layoutName); !ok {
					return fmt.Errorf("%q is not a layout: want %s",
						layoutName, strings.Join(layoutNames(), ", "))
				}
			}
			return runSet(cmd.Context(), app, args[0], text, back, layoutName)
		},
	}

	cmd.Flags().StringVar(&text, "text", "", "the color to draw the area's text in")
	cmd.Flags().StringVar(&back, "back", "", "the color to draw the area's background in")
	cmd.Flags().StringVar(&layoutName, "layout", "",
		"the layout to change (default: the one the scanner is drawing with)")
	return cmd
}

// color finds a color by name.
//
// Parameters:
//   - name: the color's name, matched without regard to case
//
// Returns:
//   - paletteColor holding that name and the value the scanner reports for it,
//     zero when the palette does not hold it
//   - bool reporting whether the scanner offers a color of that name
func color(name string) (paletteColor, bool) {
	i, ok := index(name)
	if !ok {
		return paletteColor{}, false
	}
	return palette[i], true
}

// distance is how many steps from one color to another, going the short way
// round. It is negative to step left.
//
// The list wraps, so the far end is one step from the near one and no color is
// more than half a ring away.
//
// Parameters:
//   - from: the color the picker is showing
//   - to: the color wanted, which the caller has already checked is one of the
//     scanner's
//
// Returns:
//   - int holding how many steps away it is, negative to step left
//   - error if the picker is showing a color the built-in palette does not
//     hold, which means the table does not describe this scanner
func distance(from, to string) (int, error) {
	a, ok := index(from)
	if !ok {
		return 0, fmt.Errorf("the picker is showing %q, which is not a color this tool knows: "+
			"the scanner's palette is not the one built in", from)
	}
	b, _ := index(to)

	steps := b - a
	if steps > len(palette)/2 {
		steps -= len(palette)
	}
	if steps < -len(palette)/2 {
		steps += len(palette)
	}
	return steps, nil
}

// findArea moves the editor to the area with the given name, leaving the
// scanner on that area's menu.
//
// It steps straight to where the built-in order says the area is and opens one
// menu to check, and only walks the whole editor opening every area if that
// lands somewhere unexpected. The walk is what this used to do every time, and
// it is most of what setting a color costs: three key presses and a menu read
// per area passed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while driving the editor
//   - client: the open scanner connection to press keys and read through
//   - layoutName: the layout the editor is open on, which is what the built-in
//     order is looked up by
//   - want: the area to land on, matched without regard to case
//
// Returns:
//   - error if the scanner will not take a key press, an area's menu cannot be
//     read, or no area of that name is found; nil once the scanner is on that
//     area's menu
func findArea(ctx context.Context, client *device.Scanner, layoutName, want string) error {
	if at, ok := orderOf(layoutName, want); ok {
		landed, err := jumpTo(ctx, client, at, want)
		if err != nil {
			return err
		}
		if landed {
			return nil
		}
		// The built-in order does not describe this scanner. Nothing is lost
		// but the time, because the walk below asks the scanner itself.
	}
	return walkToArea(ctx, client, want)
}

// index finds a color's place in the ring.
//
// Parameters:
//   - name: the color's name, matched without regard to case
//
// Returns:
//   - int holding how many steps it sits from the first color, zero when the
//     palette does not hold it
//   - bool reporting whether the palette holds a color of that name
func index(name string) (int, bool) {
	for i, c := range palette {
		if strings.EqualFold(c.Name, name) {
			return i, true
		}
	}
	return 0, false
}

// jumpTo steps the knob to a position and opens what is there, reporting
// whether it is the area wanted. The scanner is left on that area's menu when
// it is, and back on the editor when it is not.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while driving the editor
//   - client: the open scanner connection to press keys and read through
//   - at: how many steps right of where the editor opens the area should be
//   - want: the area expected there, matched without regard to case
//
// Returns:
//   - bool reporting whether that is the area the jump landed on
//   - error if the scanner will not take a key press, the area's menu cannot
//     be read, or backing out of the wrong area fails
func jumpTo(ctx context.Context, client *device.Scanner, at int, want string) (bool, error) {
	for i := 0; i < at; i++ {
		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return false, fmt.Errorf("stepping to the next area: %w", err)
		}
	}

	if err := menus.Enter(ctx, client); err != nil {
		return false, err
	}

	info, err := client.MenuInfo(ctx)
	if err != nil {
		return false, fmt.Errorf("reading an area's menu: %w", err)
	}
	if strings.EqualFold(areaName(info.Title), want) {
		return true, nil
	}

	return false, menus.Back(ctx, client)
}

// orderOf finds an area's place in the editor, from the built-in order.
//
// Parameters:
//   - layoutName: the layout whose order to look in
//   - want: the area to find, matched without regard to case
//
// Returns:
//   - int holding how many steps right of the editor's first area it sits,
//     zero when the order does not name it
//   - bool reporting whether the built-in order names that area at all
func orderOf(layoutName, want string) (int, bool) {
	for i, name := range areaOrder[layoutName] {
		if strings.EqualFold(name, want) {
			return i, true
		}
	}
	return 0, false
}

// renderSet writes what was changed.
//
// Parameters:
//   - app: application context the lines are written through, and whose output
//     format decides between the text and the JSON
//   - r: what was set, holding each color before and after
//
// Returns:
//   - error if the JSON cannot be encoded; nil once the change has been
//     reported
func renderSet(app *appcontext.App, r setReport) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("layout: %s (%s)\n", r.Layout, r.Menu)
	app.Printf("area:   %s\n", r.Area)

	for _, c := range []struct {
		what string
		got  *change
	}{
		{"text", r.Text},
		{"back", r.Background},
	} {
		if c.got == nil {
			continue
		}
		if !c.got.Changed {
			app.Printf("%s:   %s %s (already)\n", c.what, c.got.To, c.got.ToHex)
			continue
		}
		app.Printf("%s:   %s %s -> %s %s\n",
			c.what, c.got.From, c.got.FromHex, c.got.To, c.got.ToHex)
	}
	return nil
}

// runSet writes one area's colors.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk and the write
//   - app: application context holding the scanner connection, the output
//     format and the streams the result is written to
//   - areaName: the area to change, as the scanner names it
//   - text: the color to draw its text in, or empty to leave that alone
//   - back: the color to draw its background in, or empty to leave that alone
//   - layoutName: the layout to change, or empty for the one the scanner is
//     drawing with
//
// Returns:
//   - error if the scanner cannot be reached, the layout has no area of that
//     name, the editor cannot be walked, a color will not take, or the result
//     cannot be written; nil once every color asked for has been set and read
//     back
func runSet(ctx context.Context, app *appcontext.App, areaName, text, back, layoutName string) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	want, _, err := choose(ctx, client, layoutName)
	if err != nil {
		return err
	}

	// A name that is not in this layout is worth catching before the scanner is
	// sent anywhere, since the walk would otherwise go all the way round the
	// editor to find that out.
	if _, ok := positions[want.name][areaName]; !ok && !isSoftKey(areaName) {
		return fmt.Errorf("%s has no area called %q: run \"radiocli colors --positions %s\" for its areas",
			want.name, areaName, want.name)
	}

	app.Notef("Setting %s on %s. This stops the scan for a moment.\n", areaName, want.entry)

	if err := open(ctx, client, displayOptions, customize, want.entry); err != nil {
		return err
	}
	if err := findArea(ctx, client, want.name, areaName); err != nil {
		return err
	}

	r := setReport{Layout: want.name, Menu: want.entry, Area: areaName}

	for _, job := range []struct {
		entry string
		want  string
		into  **change
	}{
		{textColor, text, &r.Text},
		{backColor, back, &r.Background},
	} {
		if job.want == "" {
			continue
		}

		got, err := setColor(ctx, client, job.entry, job.want)
		if err != nil {
			return fmt.Errorf("setting %q for %s: %w\n"+
				"Run \"radiocli scan\" to return the scanner to scanning", job.entry, areaName, err)
		}
		*job.into = got
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return err
	}

	// Keep any cached reading of this layout true, so a color that was just
	// changed is not reported as its old one by the next --cache. Only a
	// reading that is already there is touched: a layout with nothing cached
	// stays that way rather than gaining a cache of one area.
	//
	// This runs only once every color asked for has been set and read back, so
	// what it stores is what the scanner confirmed it is showing.
	amend(app, client.Info(), want.name, areaName, func(a *cachedArea) {
		if r.Text != nil {
			a.Text, a.TextHex = r.Text.To, r.Text.ToHex
		}
		if r.Background != nil {
			a.Background, a.BackgroundHex = r.Background.To, r.Background.ToHex
		}
	})

	return renderSet(app, r)
}

// setColor opens one of an area's color pickers and sets it.
//
// The scanner is left on the area's menu either way, so the second color can be
// set from where the first finished.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while driving the picker
//   - client: the open scanner connection to press keys and read through
//   - entry: the picker to open, which is the text color entry or the back
//     color one
//   - want: the color to set it to
//
// Returns:
//   - *change holding what the color was and what it is now, with Changed
//     false when it was already the one asked for and nothing was written
//   - error if the picker will not open or read, the color is not one the
//     scanner offers, the knob will not reach it, or it does not read back as
//     what was just set
func setColor(ctx context.Context, client *device.Scanner, entry, want string) (*change, error) {
	if err := menus.Select(ctx, client, entry); err != nil {
		return nil, err
	}

	from, fromHex, err := readPicker(ctx, client)
	if err != nil {
		return nil, err
	}

	target, ok := color(want)
	if !ok {
		return nil, fmt.Errorf("%q is not a color the scanner offers", want)
	}

	got := &change{From: from, FromHex: fromHex, To: target.Name, ToHex: target.Hex}

	if strings.EqualFold(from, target.Name) {
		// Already there. Leaving without pressing enter writes nothing, which
		// is the difference between a command that did nothing and one that
		// wrote the value it found.
		return got, menus.Back(ctx, client)
	}

	if err := stepTo(ctx, client, target.Name); err != nil {
		return nil, err
	}

	// Pressing enter is the whole of the write, and it closes the picker.
	if err := menus.Enter(ctx, client); err != nil {
		return nil, err
	}
	got.Changed = true

	// Read it back rather than trusting the press.
	if err := menus.Select(ctx, client, entry); err != nil {
		return nil, err
	}
	now, nowHex, err := readPicker(ctx, client)
	if err != nil {
		return nil, err
	}
	if err := menus.Back(ctx, client); err != nil {
		return nil, err
	}

	if !strings.EqualFold(now, target.Name) {
		return nil, fmt.Errorf("the color reads back as %s %s after setting it to %s %s",
			now, nowHex, target.Name, target.Hex)
	}
	return got, nil
}

// stepTo turns the knob until the picker is showing the color wanted.
//
// The number of steps comes from the built-in palette, and the result is
// checked against the screen, which is what keeps a wrong table from writing
// the wrong color: it can only make the walk take a wrong number of steps, and
// the check then refuses rather than committing.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while turning the knob
//   - client: the open scanner connection to press keys and read through
//   - want: the color the picker should end up showing
//
// Returns:
//   - error if the picker cannot be read, is showing a color the built-in
//     palette does not hold, will not take a key press, or has not reached the
//     color wanted after the rounds run out; nil once it is showing it
func stepTo(ctx context.Context, client *device.Scanner, want string) error {
	for round := 0; round < pickerRounds; round++ {
		now, _, err := readPicker(ctx, client)
		if err != nil {
			return err
		}
		if strings.EqualFold(now, want) {
			return nil
		}

		steps, err := distance(now, want)
		if err != nil {
			return err
		}

		key := device.KeyRotateRight
		if steps < 0 {
			key, steps = device.KeyRotateLeft, -steps
		}
		for i := 0; i < steps; i++ {
			if err := client.PressKey(ctx, key, device.KeyPress); err != nil {
				return fmt.Errorf("turning the knob: %w", err)
			}
		}
	}

	return fmt.Errorf("could not get the picker onto %s after %d tries", want, pickerRounds)
}

// suggest names the colors closest to what was asked for, so a near miss says
// what was meant rather than only that it was wrong.
//
// Parameters:
//   - want: the color name that was asked for and is not one of the scanner's
//
// Returns:
//   - string naming up to six colors whose names start with or contain it, or
//     describing the palette when none of them do
func suggest(want string) string {
	var starts, contains []string
	lower := strings.ToLower(want)

	for _, c := range palette {
		name := strings.ToLower(c.Name)
		switch {
		case strings.HasPrefix(name, lower):
			starts = append(starts, c.Name)
		case strings.Contains(name, lower):
			contains = append(contains, c.Name)
		}
	}

	if found := append(starts, contains...); len(found) > 0 {
		if len(found) > 6 {
			found = found[:6]
		}
		return "did you mean " + strings.Join(found, ", ") + "?"
	}
	return fmt.Sprintf("it offers %d colors, which are the CSS color names, from %s to %s",
		len(palette), palette[0].Name, palette[len(palette)-1].Name)
}

// walkToArea opens every area in turn until it finds the one wanted, which is
// the only way to be sure when the built-in order does not fit.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk
//   - client: the open scanner connection to press keys and read through
//   - want: the area to land on, matched without regard to case
//
// Returns:
//   - error if an area's menu cannot be read, the scanner will not take a key
//     press, the walk comes back round to where it started, or it gives up
//     after the maximum number of areas; nil once the scanner is on that
//     area's menu
func walkToArea(ctx context.Context, client *device.Scanner, want string) error {
	first := ""

	for i := 0; i < maxAreas; i++ {
		if err := menus.Enter(ctx, client); err != nil {
			return err
		}

		info, err := client.MenuInfo(ctx)
		if err != nil {
			return fmt.Errorf("reading an area's menu: %w", err)
		}

		name := areaName(info.Title)
		if strings.EqualFold(name, want) {
			return nil
		}
		if name == first {
			return fmt.Errorf("no area called %q in this layout", want)
		}
		if first == "" {
			first = name
		}

		if err := menus.Back(ctx, client); err != nil {
			return err
		}
		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return fmt.Errorf("stepping to the next area: %w", err)
		}
	}

	return fmt.Errorf("gave up looking for %q after %d areas", want, maxAreas)
}
