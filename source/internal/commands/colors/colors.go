// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package colors implements the "colors" command, which reports the text and
// background color of every area of a screen layout.
//
// The scanner draws its screen from seven layouts, one per kind of screen, and
// each layout is a list of areas with a text color and a background color of
// its own. None of it is in the remote protocol: the colors are reachable only
// by walking the menus that set them, which is what this command does. Reading
// one layout means opening every area's two color pickers in turn and reading
// the value off the screen, so it takes a few seconds and stops the scan while
// it runs.
//
// Whether the colors are drawn at all is a separate setting. See the display
// command: in either black and white mode everything here is stored and
// ignored.
package colors

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the colors command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command reporting a layout's colors when it runs, with the set,
//     reset and palette subcommands already attached
func New(app *appcontext.App) *cobra.Command {
	var positions, verify, verifyPalette, cached, all bool

	cmd := &cobra.Command{
		Use:   "colors [layout]",
		Short: "Report the colors of a screen layout",
		// With either of these the scanner is only read: --cache answers off
		// the disk and --positions off a built-in table, and both ask no more
		// of the radio than which layout it is drawing. Without them this walks
		// every color picker of a layout in turn and moves the scanner a great
		// deal. See appcontext.OnlyReadsWith.
		Annotations: map[string]string{appcontext.OnlyReadsWith: "cache positions"},
		Long: "Colors reports the text and background color of every area of one of the\n" +
			"scanner's seven screen layouts.\n\n" +
			"With no layout it reads the one the scanner is drawing with right now, which\n" +
			"it works out from what the scanner is doing and how its scan display mode is\n" +
			"set. Name a layout to read that one instead:\n\n" +
			names() + "\n" +
			"Pass --all to read every one of them in turn, which is what fills the cache\n" +
			"for screens you are not looking at: a layout nobody has read is drawn white on\n" +
			"black by anything showing the display. It takes a few minutes.\n\n" +
			"The colors are not in the scanner's remote protocol. This reads them by\n" +
			"walking the menus that set them, opening every area's two color pickers in\n" +
			"turn, so it takes a few seconds and stops the scan while it runs. It changes\n" +
			"nothing: no picker is ever confirmed.\n\n" +
			"Every area also carries where it sits on the screen, which turns any\n" +
			"character position into an area and so into a color. Those positions are\n" +
			"built in rather than read, because nothing in the menus moves an area. Pass\n" +
			"--positions for them on their own, which opens no menus and is instant, or\n" +
			"--verify-positions to check the built-in map against this scanner. The list\n" +
			"of colors the scanner offers is built in the same way; --verify-palette\n" +
			"checks that one.\n\n" +
			"Every read is cached, and --cache hands the last one back rather than walking\n" +
			"the menus again. It opens no color picker: a layout that has not been read\n" +
			"yet is an error, not a wait, and the reading says how old it is.\n\n" +
			"Run \"radiocli colors set\" to change one, and \"radiocli colors palette\" for\n" +
			"the colors it can be changed to. Whether the colors are drawn at all is a\n" +
			"separate setting; see \"radiocli display\".",
		// Cobra's own OnlyValidArgs names the offending word without saying
		// what would have been accepted, which is what the reader needs.
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}
			if len(args) == 1 {
				if _, ok := lookup(args[0]); !ok {
					return fmt.Errorf("%q is not a layout: want %s",
						args[0], strings.Join(layoutNames(), ", "))
				}
			}
			return nil
		},
		ValidArgs: layoutNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			wanted := ""
			if len(args) == 1 {
				wanted = args[0]
			}
			chosen := 0
			for _, on := range []bool{positions, verify, verifyPalette} {
				if on {
					chosen++
				}
			}
			if chosen > 1 {
				return fmt.Errorf("--positions, --verify-positions and --verify-palette " +
					"cannot be combined: each asks the scanner a different question")
			}

			// --cache says where the colors come from, and the other three
			// answer questions that have no colors in them: two check a built-in
			// table against the scanner, which a stored answer cannot do, and
			// the third is already instant.
			if cached && chosen > 0 {
				return fmt.Errorf("--cache cannot be combined with --positions, " +
					"--verify-positions or --verify-palette: none of them reads the " +
					"colors, so there is nothing for the cache to answer")
			}

			// --all is which layouts to read, and every one of the others is a
			// question asked about one layout. Naming a layout as well says both
			// "this one" and "all of them", so it is refused rather than one of
			// them quietly winning.
			if all {
				if wanted != "" {
					return fmt.Errorf("--all reads every layout, so %q cannot be named as well", wanted)
				}
				if cached || chosen > 0 {
					return fmt.Errorf("--all cannot be combined with --cache, --positions, " +
						"--verify-positions or --verify-palette: it reads every layout from " +
						"the scanner, which is the one thing none of those do")
				}
				return runAll(cmd.Context(), app)
			}

			switch {
			case positions:
				return runPositions(cmd.Context(), app, wanted)
			case verify:
				return runVerify(cmd.Context(), app, wanted)
			case verifyPalette:
				return runVerifyPalette(cmd.Context(), app, wanted)
			}
			return run(cmd.Context(), app, wanted, cached)
		},
	}

	cmd.AddCommand(newSet(app))
	cmd.AddCommand(newReset(app))
	cmd.AddCommand(newPalette(app))

	cmd.Flags().BoolVar(&positions, "positions", false,
		"report where each area sits and not its colors, which opens no menus")
	cmd.Flags().BoolVar(&verify, "verify-positions", false,
		"check the built-in positions against the scanner, and report any that differ")
	cmd.Flags().BoolVar(&verifyPalette, "verify-palette", false,
		"check the built-in list of colors against the scanner, and report any that differ")
	cmd.Flags().BoolVar(&cached, "cache", false,
		"report the last reading of this layout instead of walking the menus, and fail if there is none")
	cmd.Flags().BoolVar(&all, "all", false,
		"read every layout in turn and cache each, which takes a few minutes")
	return cmd
}

// areaName strips the wording the scanner wraps an area's name in, so
// "Set System_name Area" reads as "System_name".
//
// Parameters:
//   - title: the title of an area's menu, as the scanner reports it
//
// Returns:
//   - string holding the area's own name, with the surrounding wording and
//     whitespace taken off
func areaName(title string) string {
	name := strings.TrimSpace(title)
	name = strings.TrimPrefix(name, "Set ")
	name = strings.TrimSuffix(name, " Area")
	return strings.TrimSpace(name)
}

// at renders one number of an area's position, or a dash for an area the map
// does not place. An unplaced area reads as unplaced rather than as one sitting
// at the top left, which is what a bare zero would say.
//
// Parameters:
//   - a: the area the number belongs to, read to tell a placed one from an
//     unplaced one
//   - value: the number to renderReport
//
// Returns:
//   - string holding the number, or "-" when the map does not place the area
func at(a area, value int) string {
	if a.Length == 0 {
		return "-"
	}
	return fmt.Sprint(value)
}

// byLineCount picks between layouts by how many rows the screen has.
//
// This is the cheap half of telling the simple layout from the detail one: the
// scanner reports one row per line of its display, and the two modes have
// fourteen and seventeen. It is a read, so it works while scanning and opens
// nothing.
//
// It declines rather than guesses when the count is neither, which is what a
// screen caught mid redraw, or a firmware with a different display, would look
// like.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - client: the open scanner connection to read the display through
//   - found: the layouts the current view could be drawn with
//
// Returns:
//   - layout whose row count matches the screen, zero when none does
//   - bool reporting whether the count settled it, false when the screen
//     cannot be read or has a row count neither mode draws
func byLineCount(ctx context.Context, client *device.Scanner, found []layout) (layout, bool) {
	d, err := client.Display(ctx)
	if err != nil {
		return layout{}, false
	}

	for _, l := range found {
		if l.lines != 0 && l.lines == len(d.Lines) {
			return l, true
		}
	}
	return layout{}, false
}

// candidates returns every layout the scanner's current view could be drawn
// with, along with the view's name for an error to quote.
//
// A view that names no layout is given a moment to become one, because the
// scanner reports a plain screen briefly while it settles. A menu is not: it
// will stay a menu, so waiting would only delay the error.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the polls it waits out
//   - client: the open scanner connection to ask what it is doing
//
// Returns:
//   - []layout holding every layout the view could be drawn with, empty for a
//     menu or for a view no layout covers
//   - string naming the view the scanner last reported, for an error to quote
//   - error if the scanner will not say what it is doing, or the context ends
//     while waiting for the screen to settle
func candidates(ctx context.Context, client *device.Scanner) ([]layout, string, error) {
	screen := ""

	for poll := 0; poll < settlePolls; poll++ {
		info, err := client.ScannerInfo(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("asking the scanner what it is doing: %w", err)
		}
		screen = info.Screen

		if found := matching(screen); len(found) > 0 {
			return found, screen, nil
		}
		for _, m := range menuScreens {
			if strings.EqualFold(screen, m) {
				return nil, screen, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, screen, ctx.Err()
		case <-time.After(settleGap):
		}
	}

	return nil, screen, nil
}

// clean strips a display row down to the text it means, dropping the padding
// and the control bytes the scanner marks a shortened row with.
//
// Parameters:
//   - text: one row of the display, as the scanner reports it
//
// Returns:
//   - string holding only the printable characters of that row, trimmed
func clean(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// colored reports whether a reading holds any colors, which separates a full
// read from a positions-only one.
//
// Parameters:
//   - areas: the reading to look through
//
// Returns:
//   - bool reporting whether any area carries a text or background color
func colored(areas []area) bool {
	for _, a := range areas {
		if a.Text != "" || a.Background != "" {
			return true
		}
	}
	return false
}

// currentLayout works out which layout the scanner is drawing with.
//
// Two things decide it: what the scanner is doing, which names the family, and
// the scan display mode, which picks the simple or the detail version of the
// two families that have both. The first is a plain read; the second is a menu,
// so it is asked for only when it is needed.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - client: the open scanner connection to ask what it is doing
//
// Returns:
//   - layout the scanner is drawing with, zero when none could be settled on
//   - error if the scanner is in a menu, is showing a view no layout covers,
//     will not say what it is doing, or reports a scan display mode that is
//     neither the simple one nor the detail one
func currentLayout(ctx context.Context, client *device.Scanner) (layout, error) {
	found, screen, err := candidates(ctx, client)
	if err != nil {
		return layout{}, err
	}

	switch len(found) {
	case 0:
		for _, m := range menuScreens {
			if strings.EqualFold(screen, m) {
				return layout{}, fmt.Errorf("the scanner is in a menu, so it is not drawing " +
					"with any layout: run \"radiocli scan\" to put it back to scanning, " +
					"or name a layout")
			}
		}
		return layout{}, fmt.Errorf("the scanner is showing %q, which no layout covers: "+
			"run \"radiocli scan\" to put it back to scanning, or name a layout", screen)
	case 1:
		return found[0], nil
	}

	// Two layouts share this view, one simple and one detail. The screen counts
	// its own rows, and the two modes draw a different number of them, so that
	// settles it without opening anything.
	if l, ok := byLineCount(ctx, client, found); ok {
		return l, nil
	}

	// A row count that is neither is a screen this does not know. The menu that
	// sets the mode is the authority, and costs a walk.
	mode, err := scanDisplayMode(ctx, client)
	if err != nil {
		return layout{}, err
	}
	for _, l := range found {
		if l.detail == mode {
			return l, nil
		}
	}
	return layout{}, fmt.Errorf("the scan display mode is %q, which is neither %q nor %q",
		mode, simpleEntry, detailEntry)
}

// indexOf finds a menu entry by name, or -1.
//
// Parameters:
//   - items: the menu's entries, in the order the scanner lists them
//   - want: the entry to find, matched without regard to case or surrounding
//     whitespace
//
// Returns:
//   - int holding that entry's place in the list, or -1 when the menu does not
//     hold it
func indexOf(items []device.MenuItem, want string) int {
	for i, it := range items {
		if strings.EqualFold(strings.TrimSpace(it.Name), want) {
			return i
		}
	}
	return -1
}

// isCurrent reports whether a named layout is the one the scanner is drawing
// with.
//
// It asks the scan display mode only when the answer turns on it, which is when
// the layout's own family is the one in use and that family has both a simple
// and a detail version. Every other case is settled by what the scanner is
// doing, which costs one read and no menus.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - client: the open scanner connection to ask what it is doing
//   - want: the layout being asked about
//
// Returns:
//   - bool reporting whether that layout is the one on screen
//   - error if the scanner will not say what it is doing, or will not report
//     its scan display mode when the answer turns on it
func isCurrent(ctx context.Context, client *device.Scanner, want layout) (bool, error) {
	found, _, err := candidates(ctx, client)
	if err != nil {
		return false, err
	}

	same := false
	for _, l := range found {
		if l.name == want.name {
			same = true
		}
	}
	if !same || len(found) == 1 {
		return same, nil
	}

	if l, ok := byLineCount(ctx, client, found); ok {
		return l.name == want.name, nil
	}

	mode, err := scanDisplayMode(ctx, client)
	if err != nil {
		return false, err
	}
	return want.detail == mode, nil
}

// layoutNames lists the layout names, in the Customize menu's order.
//
// Returns:
//   - []string holding the name of every layout this command accepts
func layoutNames() []string {
	out := make([]string, 0, len(layouts))
	for _, l := range layouts {
		out = append(out, l.name)
	}
	return out
}

// lookup finds a layout by the name this command accepts.
//
// Parameters:
//   - name: the layout's name, matched without regard to case
//
// Returns:
//   - layout of that name, zero when there is none
//   - bool reporting whether the name is one of the seven
func lookup(name string) (layout, bool) {
	for _, l := range layouts {
		if strings.EqualFold(l.name, name) {
			return l, true
		}
	}
	return layout{}, false
}

// matching returns every layout the scanner's current view could be drawn with.
//
// Parameters:
//   - screen: the view the scanner reports, such as "conventional_scan"
//
// Returns:
//   - []layout holding every layout that covers the view, which is two for a
//     view with a simple and a detail version and none for one no layout draws
func matching(screen string) []layout {
	var out []layout
	for _, l := range layouts {
		for _, s := range l.screens {
			if strings.EqualFold(s, screen) {
				out = append(out, l)
				break
			}
		}
	}
	return out
}

// names renders the layouts for the command's help.
//
// Returns:
//   - string holding one indented line per layout, its name beside the
//     Customize menu entry it lives behind
func names() string {
	var b strings.Builder
	for _, l := range layouts {
		fmt.Fprintf(&b, "  %-20s %s\n", l.name, l.entry)
	}
	return b.String()
}

// open walks from wherever the scanner is to a menu, by name at every level.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while driving the menus
//   - client: the open scanner connection to press keys through
//   - path: the entries to select in turn, from the top menu down
//
// Returns:
//   - error if the top menu will not open or any entry on the way cannot be
//     found; nil once the scanner is sitting on the menu at the end of path
func open(ctx context.Context, client *device.Scanner, path ...string) error {
	if err := client.OpenMenu(ctx, device.MenuTop, ""); err != nil {
		return fmt.Errorf("opening the top menu: %w", err)
	}

	for _, entry := range path {
		if err := menus.Select(ctx, client, entry); err != nil {
			return fmt.Errorf("looking for %q: %w\n"+
				"Run \"radiocli scan\" to return the scanner to scanning", entry, err)
		}
	}
	return nil
}

// place fills in where each area sits, from the built-in map.
//
// The soft keys are not in the map and are filled in from the live screen
// instead, which only means anything when the layout being reported is the one
// on screen.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while reading the screen
//   - client: the open scanner connection the soft keys are read through
//   - want: the layout whose map the areas are placed from
//   - areas: the reading to place, written to in place
//   - current: whether this layout is the one on screen, which is the only
//     case where the soft keys can be read
//
// Returns:
//   - error if the soft keys cannot be read off the live screen; nil once
//     every area the map places carries its position
func place(ctx context.Context, client *device.Scanner, want layout, areas []area, current bool) error {
	known := positions[want.name]
	for i := range areas {
		if at, ok := known[areas[i].Name]; ok {
			areas[i].Line, areas[i].Column = at.Line, at.Column
			areas[i].Length, areas[i].Height = at.Length, at.Height
		}
	}

	if !current {
		return nil
	}

	keys, err := settledSoftKeys(ctx, client)
	if err != nil {
		return err
	}
	for _, k := range keys {
		for i := range areas {
			if areas[i].Name == k.Name {
				areas[i].Line, areas[i].Column = k.Line, k.Column
				areas[i].Length, areas[i].Height = k.Length, k.Height
			}
		}
	}
	return nil
}

// readArea opens the area the editor is on and reads both of its colors,
// leaving the scanner on that area's menu.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while driving the menus
//   - client: the open scanner connection to press keys and read through
//
// Returns:
//   - area holding the name the scanner gives it and both of its colors, with
//     a color left empty when the area's menu has no entry to set it
//   - error if a menu will not open, the area's menu cannot be read, or a
//     picker does not show a color value
func readArea(ctx context.Context, client *device.Scanner) (area, error) {
	if err := menus.Enter(ctx, client); err != nil {
		return area{}, err
	}

	info, err := client.MenuInfo(ctx)
	if err != nil {
		return area{}, fmt.Errorf("reading an area's menu: %w", err)
	}

	got := area{Name: areaName(info.Title)}

	// The two colors sit at whatever positions this area's menu puts them,
	// which differs: an area with an option to choose carries a third entry
	// above them. The cursor opens on the first entry and stays on whichever
	// one was opened, so each step is measured from the last.
	at := 0
	for _, want := range []struct {
		entry string
		into  *string
		hex   *string
	}{
		{textColor, &got.Text, &got.TextHex},
		{backColor, &got.Background, &got.BackgroundHex},
	} {
		to := indexOf(info.Items, want.entry)
		if to < 0 {
			continue
		}

		if err := step(ctx, client, to-at); err != nil {
			return area{}, err
		}
		at = to

		if err := menus.Enter(ctx, client); err != nil {
			return area{}, err
		}

		name, hex, err := readPicker(ctx, client)
		if err != nil {
			return area{}, fmt.Errorf("reading %q for %s: %w", want.entry, got.Name, err)
		}
		*want.into, *want.hex = name, hex

		// Out of the picker without choosing anything, which is what keeps
		// this command a read.
		if err := menus.Back(ctx, client); err != nil {
			return area{}, err
		}
	}

	return got, nil
}

// readLayout walks a layout's areas and reads both colors of each.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the whole walk, which
//     takes about half a minute
//   - client: the open scanner connection to press keys and read through
//   - want: the layout to read, whose Customize entry the walk opens
//
// Returns:
//   - []area holding every area of the layout, in the order the editor steps
//     through them, each with both of its colors
//   - error if the editor will not open, reports no areas, cannot be read, or
//     never comes back round to the area the walk started on
func readLayout(ctx context.Context, client *device.Scanner, want layout) ([]area, error) {
	if err := open(ctx, client, displayOptions, customize, want.entry); err != nil {
		return nil, err
	}

	// A quick check that this is an editor rather than something the walk
	// landed on by accident. The listing cannot be used for anything more: it
	// stops at twenty areas whatever the layout holds.
	if info, err := client.MenuInfo(ctx); err != nil {
		return nil, fmt.Errorf("reading the areas of %s: %w", want.entry, err)
	} else if len(info.Items) == 0 {
		return nil, fmt.Errorf("the scanner reports no areas for %s", want.entry)
	}

	// The knob is the only way through the editor. Its rows are the layout's
	// own artwork rather than names, so an area cannot be stepped to by name
	// the way every other menu in this tool is walked: the walk goes one step
	// at a time from where the editor opens, and each area names itself once
	// it is opened.
	//
	// The end is the knob coming back round to the area it started on. Nothing
	// says in advance how many there are: the menu listing gives up at twenty
	// and the largest layout has fifty.
	var out []area
	for len(out) < maxAreas {
		got, err := readArea(ctx, client)
		if err != nil {
			return nil, err
		}

		if len(out) > 0 && got.Name == out[0].Name {
			return out, nil
		}
		out = append(out, got)

		// Back to the editor, and on to the next area.
		if err := menus.Back(ctx, client); err != nil {
			return nil, err
		}
		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return nil, fmt.Errorf("stepping to the next area: %w", err)
		}
	}

	return nil, fmt.Errorf("%s did not come back round to %q after %d areas",
		want.entry, out[0].Name, maxAreas)
}

// readPicker reads the color a picker is showing, as a name and a hex value.
//
// The picker's own menu listing is no use here: it holds all 140 or so color
// names and marks none of them as the current one. The screen shows the current
// color as a name with its value underneath, so that is what this reads.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while reading the screen
//   - client: the open scanner connection to read the display through
//
// Returns:
//   - name the picker is showing, which is a CSS color name
//   - hex value it shows for that color, in the form "#RRGGBB"
//   - err if the screen cannot be read or shows no color value, which is what
//     a screen that is not a picker looks like
func readPicker(ctx context.Context, client *device.Scanner) (name, hex string, err error) {
	d, err := client.Display(ctx)
	if err != nil {
		return "", "", fmt.Errorf("reading the screen: %w", err)
	}

	for i, l := range d.Lines {
		m := rgb.FindStringSubmatch(l.Text)
		if m == nil {
			continue
		}

		// The name is the last thing written above the value.
		for j := i - 1; j >= 0; j-- {
			if text := clean(d.Lines[j].Text); text != "" {
				name = text
				break
			}
		}
		return name, "#" + strings.ToUpper(m[1]), nil
	}

	return "", "", fmt.Errorf("the screen shows no color value")
}

// renderReport writes the layout's colors.
//
// Parameters:
//   - app: application context the table is written through, and whose output
//     format decides between the table and the JSON
//   - r: the reading to write
//
// Returns:
//   - error if the table cannot be written or the JSON cannot be encoded; nil
//     once the reading has been reported
func renderReport(app *appcontext.App, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("layout: %s (%s)\n", r.Layout, r.Menu)
	if r.Current {
		app.Printf("        the scanner is drawing with this one\n")
	}
	if r.Cached {
		// When it was read is the whole of what makes a cached answer worth
		// anything, since nothing tells the tool that somebody has since
		// changed a color on the scanner itself.
		app.Printf("cached: %s\n", age(r.Read))
	}
	app.Printf("\n")

	// Two spaces of padding, so columns stay apart even at their widest.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)

	// A run that read no colors prints no color columns, rather than a table
	// of dashes wide enough to hide the numbers that were asked for.
	colors := colored(r.Areas)
	if colors {
		fmt.Fprintln(w, "AREA\tLINE\tCOL\tLEN\tROWS\tTEXT\tHEX\tBACKGROUND\tHEX")
	} else {
		fmt.Fprintln(w, "AREA\tLINE\tCOL\tLEN\tROWS")
	}

	for _, a := range r.Areas {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s",
			a.Name, at(a, a.Line), at(a, a.Column), at(a, a.Length), at(a, a.Height))
		if colors {
			fmt.Fprintf(w, "\t%s\t%s\t%s\t%s",
				render.Dash(a.Text), render.Dash(a.TextHex), render.Dash(a.Background), render.Dash(a.BackgroundHex))
		}
		fmt.Fprintln(w)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the colors: %w", err)
	}

	app.Printf("\n%d areas\n", len(r.Areas))
	return nil
}

// run reads one layout's colors and renders them.
//
// With cached set the colors come off the disk instead of out of the menus.
// Everything else is the same walk it always was: which layout is meant is
// still asked of the scanner, because that is a plain read and the answer
// changes with what the scanner is doing, and the positions are still filled in
// from the built-in map and the live screen. Only the pickers are skipped,
// which is all of the half minute.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the walk
//   - app: application context holding the scanner connection, the output
//     format and the streams the reading is written to
//   - wanted: the layout to read, or empty for the one the scanner is drawing
//     with
//   - cached: whether to answer from the last stored reading rather than
//     walking the color pickers
//
// Returns:
//   - error if the scanner cannot be reached, no layout can be settled on, the
//     walk fails, --cache was asked for a layout that has never been read, or
//     the reading cannot be written; nil once the colors have been reported
func run(ctx context.Context, app *appcontext.App, wanted string, cached bool) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	// What the scanner is doing has to be read before the menus are opened,
	// because opening them is itself a change of view: from inside a menu the
	// scanner reports the menu rather than the screen it came from.
	var current bool
	var want layout
	if wanted == "" {
		if want, err = currentLayout(ctx, client); err != nil {
			return err
		}
		current = true
	} else {
		want, _ = lookup(wanted)

		// Whether the named layout happens to be the one in use is worth
		// saying but not worth failing over, so a scanner that will not answer
		// leaves it unsaid rather than stopping a read that can go ahead.
		current, _ = isCurrent(ctx, client, want)
	}

	var areas []area
	var read time.Time

	if cached {
		store, err := loadCache()
		if err != nil {
			return err
		}
		got, ok := store.lookup(scannerKey(client.Info()), want.name)
		if !ok {
			return missing(want.name)
		}
		areas, read = got.areas(), got.Read
	} else {
		app.Notef("Reading %s from the scanner's menus. This stops the scan for a moment.\n",
			want.entry)

		if areas, err = readLayout(ctx, client, want); err != nil {
			return err
		}
		if _, err := menus.Leave(ctx, client); err != nil {
			return err
		}

		read = time.Now()
		remember(app, client.Info(), want.name, areas, read)
	}

	// Positions come from the built-in map rather than the walk, which is why
	// this costs nothing on top. The soft keys are read off the live screen,
	// and that has to wait until the scanner is out of the menus and drawing
	// the layout again.
	if err := place(ctx, client, want, areas, current); err != nil {
		return err
	}

	return renderReport(app, report{
		Layout:  want.name,
		Menu:    want.entry,
		Current: current,
		Cached:  cached,
		Read:    read,
		Areas:   areas,
	})
}

// scanDisplayMode reports whether the scanner is set to its simple or its
// detail scanning screen.
//
// The setting is shown as the highlighted entry of the menu that sets it, and
// that is the whole of how the scanner reports it: there is no way to ask over
// the protocol. This opens that menu and backs out to Display Options, which is
// where the walk to the layouts starts from anyway.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while driving the menus
//   - client: the open scanner connection to press keys and read through
//
// Returns:
//   - string holding the highlighted entry of the menu that sets it, which is
//     the simple mode entry or the detail mode one
//   - error if the menu will not open, the highlighted row cannot be read, or
//     the scanner will not back out of it
func scanDisplayMode(ctx context.Context, client *device.Scanner) (string, error) {
	if err := open(ctx, client, displayOptions, scanDisplay); err != nil {
		return "", err
	}

	row, err := menus.HighlightedRow(ctx, client)
	if err != nil {
		return "", fmt.Errorf("reading the scan display mode: %w", err)
	}

	if err := menus.Back(ctx, client); err != nil {
		return "", err
	}
	return row.Text, nil
}

// settledSoftKeys reads the soft keys once the scanner is drawing its layout
// again.
//
// The walk ends inside the menus and climbing out is not instant: for a moment
// afterwards the scanner is still drawing the menu it was in, whose bottom row
// is not three soft keys. Reading then gets nothing, and nothing is what the
// soft keys used to be reported as after a full read, while the same command
// with --positions placed them because it never went into a menu.
//
// Nothing announces that the screen has arrived. So this asks until the row
// looks like soft keys, on the same budget candidates gives a screen to settle.
// A layout that genuinely draws no soft keys spends that budget and still
// reports none, which is the right answer arrived at slowly.
//
// Parameters:
//   - ctx: context for cancellation and timeouts across the polls it waits out
//   - client: the open scanner connection to read the display through
//
// Returns:
//   - []area holding the five regions of the bottom row, empty when the budget
//     runs out without the row ever looking like soft keys
//   - error if the screen cannot be read, or the context ends while waiting
func settledSoftKeys(ctx context.Context, client *device.Scanner) ([]area, error) {
	for poll := 0; poll < settlePolls; poll++ {
		keys, err := softKeys(ctx, client)
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			return keys, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(settleGap):
		}
	}
	return nil, nil
}

// step turns the knob right count times.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while pressing keys
//   - client: the open scanner connection to press keys through
//   - count: how many steps to turn, where zero or less turns nothing
//
// Returns:
//   - error if the scanner will not take a key press; nil once every step has
//     been sent
func step(ctx context.Context, client *device.Scanner, count int) error {
	for i := 0; i < count; i++ {
		if err := client.PressKey(ctx, device.KeyRotateRight, device.KeyPress); err != nil {
			return fmt.Errorf("turning the knob: %w", err)
		}
	}
	return nil
}
