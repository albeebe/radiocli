// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package colors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// fakeConn is a device.Conn that answers every command from the fake radio, so
// the walks these commands make can be driven with no scanner attached.
type fakeConn struct {
	reply func(command string) (string, error)
}

// Info describes the scanner the connection is open to.
func (f fakeConn) Info() device.Info {
	return device.Info{Port: "/dev/tty.fake", Model: "SDS150", Serial: "0001"}
}

// Execute answers the command the way the radio would.
func (f fakeConn) Execute(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// ExecuteXML answers the command the way the radio would.
func (f fakeConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// Send reports whatever answering the command would have reported.
func (f fakeConn) Send(ctx context.Context, command string) error {
	_, err := f.reply(command)
	return err
}

// Close releases nothing, because there is no port.
func (f fakeConn) Close() error { return nil }

// failWriter is a stream that refuses everything written to it, which is how a
// closed pipe behaves when the output is being read by something else.
type failWriter struct{}

// Write refuses the bytes and says why.
func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("the pipe closed") }

// line is one row of the scanner's display.
type line struct {
	text  string // The row as the scanner reports it
	attrs string // How each character is drawn, where "*" is reverse video
}

// view is one screen the fake radio can be showing.
type view struct {
	title   string              // The heading the scanner reports for it
	rows    []string            // The rows the knob steps through, in its own order
	opens   map[string]*view    // The screen each row opens when it is pressed
	picker  bool                // Whether this is a color picker rather than a menu
	at      int                 // The palette color a picker is showing
	stuck   bool                // Whether the knob moves a picker at all
	slips   bool                // Whether a picker keeps a color other than the one chosen
	endless bool                // Whether a picker offers a fresh color every step and never comes round
	prompt  []line              // The display to show instead of the rows, for a prompt
	places  map[string]position // Where an editor draws each of its rows
	height  int                 // How many rows an editor's screen has
	bare    bool                // Whether the menu listing reports no entries at all
}

// frame is a screen the walk came through, kept so leaving one returns to it.
type frame struct {
	on     *view // The screen that was showing
	cursor int   // The row the knob was on
	at     int   // The color a picker was showing when it was opened
}

// radio answers as a scanner sitting in its menus, so the walks these commands
// make can be driven with no radio attached.
//
// It holds one screen at a time and remembers the ones it came through, because
// reading a color means going three levels down and coming back. Every display
// request highlights the row the knob is on, which is what keeps a walk from
// waiting for a redraw that is never coming. A radio with nothing on its stack
// is out of the menus and drawing its layout, which is where the soft keys are
// read from.
type radio struct {
	top           *view              // The screen opening a menu over the wire puts on
	modeMenu      *view              // The menu that sets the scan display mode
	customizeMenu *view              // The menu the layout editors hang off
	restoreMenu   *view              // The menu that puts a layout back to stock
	stack         []frame            // The screens walked into, the last of which is showing
	scanning      []line             // The display of the layout it draws out of the menus
	screen        string             // The view it reports it is showing
	doing         string             // The mode it reports it is in
	fails         func(string) error // The failure to answer a command with, if any
	presses       []string           // Every key that was pressed, in order
}

// back leaves the screen the scanner is on and returns to the one it came from.
func (r *radio) back() {
	if len(r.stack) == 0 {
		return
	}

	last := r.stack[len(r.stack)-1]
	if last.on.picker {
		// Leaving a picker by the menu key abandons the knob's position, which
		// is the whole of what keeps a read from writing a color.
		last.on.at = last.at
	}
	r.stack = r.stack[:len(r.stack)-1]
}

// display renders the screen the scanner is showing as the reply to a display
// request.
func (r *radio) display() string {
	lines := r.lines()

	parts := []string{strings.Repeat("0", len(lines))}
	for _, l := range lines {
		parts = append(parts, l.text, l.attrs)
	}
	return strings.Join(parts, ",")
}

// enter presses the row the knob is on.
func (r *radio) enter() {
	on := r.on()
	if on == nil {
		return
	}

	// Choosing a color, or answering a prompt, closes the screen and keeps
	// what it was showing.
	if on.picker || on.prompt != nil {
		if on.slips {
			on.at = (on.at + 1) % len(palette)
		}
		r.stack = r.stack[:len(r.stack)-1]
		return
	}
	if len(on.rows) == 0 {
		return
	}

	next, opens := on.opens[on.rows[r.stack[len(r.stack)-1].cursor]]
	if !opens {
		return
	}
	r.stack = append(r.stack, frame{on: next, at: next.at})
}

// failOn makes the radio refuse one command every time it is asked.
func (r *radio) failOn(command string, err error) {
	r.fails = func(asked string) error {
		if asked == command {
			return err
		}
		return nil
	}
}

// lines returns the display the scanner would be showing.
func (r *radio) lines() []line {
	if len(r.stack) == 0 {
		return r.scanning
	}

	f := r.stack[len(r.stack)-1]
	switch {
	case f.on.picker:
		c := palette[f.on.at%len(palette)]
		if f.on.endless {
			// A picker offering a color nobody has seen before, however far
			// the knob is turned, which is what never coming round looks like.
			c = paletteColor{Name: fmt.Sprintf("Color_%d", f.on.at), Hex: fmt.Sprintf("#%06X", f.on.at)}
		}
		return []line{
			{text: c.Name, attrs: strings.Repeat("*", len(c.Name))},
			{text: "RGB = " + strings.TrimPrefix(c.Hex, "#") + "h"},
		}

	case f.on.prompt != nil:
		return f.on.prompt

	case f.on.places != nil:
		row := ""
		if len(f.on.rows) > 0 {
			row = f.on.rows[f.cursor]
		}
		return spanLines(f.on.places[row], f.on.height)
	}

	out := make([]line, 0, len(f.on.rows))
	for i, row := range f.on.rows {
		l := line{text: row}
		if i == f.cursor {
			l.attrs = strings.Repeat("*", len(row))
		}
		out = append(out, l)
	}
	return out
}

// menu renders the screen the scanner is showing as the reply to a menu
// request. A radio that has left the menus answers the way the scanner does,
// which is with an error document.
func (r *radio) menu() string {
	on := r.on()
	if on == nil {
		return `<MenuInfo MenuType="TypeError"/>`
	}

	items := ""
	if !on.bare {
		for i, row := range on.rows {
			items += fmt.Sprintf(`<MenuItem Name=%q Index="%d"/>`, row, i)
		}
	}
	return fmt.Sprintf(`<MenuInfo Name=%q MenuType="TypeSelect">%s</MenuInfo>`, on.title, items)
}

// on returns the screen the scanner is showing, or nil when it is out of the
// menus.
func (r *radio) on() *view {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1].on
}

// onAreaMenu reports whether the scanner is showing an area's own menu, which
// is the screen the two color entries sit on.
func (r *radio) onAreaMenu() bool {
	on := r.on()
	return on != nil && strings.HasSuffix(on.title, " Area")
}

// onPicker reports whether the scanner is showing a color picker.
func (r *radio) onPicker() bool {
	on := r.on()
	return on != nil && on.picker
}

// press acts on one key the way the scanner would, given what is on screen.
func (r *radio) press(key string) {
	r.presses = append(r.presses, key)

	on := r.on()
	if on == nil {
		return
	}

	switch key {
	case ">":
		if on.picker {
			switch {
			case on.stuck:
			case on.endless:
				on.at++
			default:
				on.at = (on.at + 1) % len(palette)
			}
		} else if len(on.rows) > 0 {
			f := &r.stack[len(r.stack)-1]
			f.cursor = (f.cursor + 1) % len(on.rows)
		}
	case "<":
		if on.picker {
			if !on.stuck {
				on.at = (on.at + len(palette) - 1) % len(palette)
			}
		} else if len(on.rows) > 0 {
			f := &r.stack[len(r.stack)-1]
			f.cursor = (f.cursor + len(on.rows) - 1) % len(on.rows)
		}
	case "E":
		r.enter()
	case "M", "L", "A":
		r.back()
	}
}

// reply answers one command the way the scanner would.
func (r *radio) reply(command string) (string, error) {
	if r.fails != nil {
		if err := r.fails(command); err != nil {
			return "", err
		}
	}

	switch {
	case command == "STS":
		return r.display(), nil
	case command == "MSI":
		return r.menu(), nil
	case command == "GSI":
		return fmt.Sprintf(`<ScannerInfo Mode=%q V_Screen=%q/>`, r.doing, r.screen), nil
	case strings.HasPrefix(command, "MNU,"):
		r.stack = nil
		if r.top != nil {
			r.stack = []frame{{on: r.top}}
		}
	case strings.HasPrefix(command, "KEY,"):
		r.press(strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P"))
	}
	return "", nil
}

// areaViewFor returns one area's menu, with a color picker behind each of the
// two entries that set a color.
func areaViewFor(name string) *view {
	return &view{
		title: "Set " + name + " Area",
		rows:  []string{textColor, backColor},
		opens: map[string]*view{
			textColor: {title: textColor, picker: true, at: colorAt("Yellow")},
			backColor: {title: backColor, picker: true, at: colorAt("Black")},
		},
	}
}

// colorAt returns a color's place in the palette, for setting a picker up on
// it.
func colorAt(name string) int {
	i, _ := index(name)
	return i
}

// confirmViewFor returns the prompt the scanner shows before it restores
// anything.
func confirmViewFor(entry string) *view {
	return &view{
		title: restoreEntry,
		prompt: []line{
			{text: "Confirm Restore?"},
			{text: entry},
			{text: `Yes="E" / No="."`},
		},
	}
}

// editorViewFor returns one layout's editor, holding the first count of its
// areas, and drawing each of them where the built-in map says it is.
func editorViewFor(l layout, count int) *view {
	order := areaOrder[l.name]
	if count < len(order) {
		order = order[:count]
	}

	editor := &view{
		title:  l.entry,
		opens:  map[string]*view{},
		places: positions[l.name],
		height: screenRows(l.name),
	}
	for _, name := range order {
		editor.rows = append(editor.rows, name)
		editor.opens[name] = areaViewFor(name)
	}
	return editor
}

// fast shortens the wait for a screen to settle, so a test that has to run one
// of those loops out costs milliseconds rather than seconds. The values are put
// back when the test ends, so no other test sees the shortened ones.
func fast(t *testing.T) {
	t.Helper()

	polls, gap := settlePolls, settleGap
	t.Cleanup(func() { settlePolls, settleGap = polls, gap })
	settlePolls, settleGap = 2, time.Millisecond
}

// newApp returns an application context writing to buffers rather than to the
// terminal, along with the buffers it writes to.
func newApp() (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, notes := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, notes
	return app, out, notes
}

// newRadio returns a fake scanner drawing one of its layouts, with the menu
// tree this command walks behind it: Display Options, Customize, an editor per
// layout, and the Restore menu.
//
// Parameters:
//   - screen: the view the scanner reports it is showing
//   - lines: how many rows its scanning display has
//   - areas: how many of each layout's areas its editors hold
func newRadio(screen string, lines, areas int) *radio {
	r := &radio{screen: screen, doing: "Scan", scanning: scanningLines(lines)}

	r.customizeMenu = &view{title: customize, opens: map[string]*view{}}
	r.restoreMenu = &view{title: restoreEntry, opens: map[string]*view{}}

	for _, l := range layouts {
		r.customizeMenu.rows = append(r.customizeMenu.rows, l.entry)
		r.customizeMenu.opens[l.entry] = editorViewFor(l, areas)

		name := strings.TrimPrefix(l.entry, "Set ")
		r.restoreMenu.rows = append(r.restoreMenu.rows, name)
		r.restoreMenu.opens[name] = confirmViewFor(name)
	}
	r.customizeMenu.rows = append(r.customizeMenu.rows, restoreEntry)
	r.customizeMenu.opens[restoreEntry] = r.restoreMenu

	r.restoreMenu.rows = append(r.restoreMenu.rows, allScreens)
	r.restoreMenu.opens[allScreens] = confirmViewFor(allScreens)

	r.modeMenu = &view{title: scanDisplay, rows: []string{simpleEntry, detailEntry}}

	options := &view{
		title: displayOptions,
		rows:  []string{scanDisplay, customize},
		opens: map[string]*view{scanDisplay: r.modeMenu, customize: r.customizeMenu},
	}
	r.top = &view{
		title: "Menu",
		rows:  []string{displayOptions, "Settings"},
		opens: map[string]*view{displayOptions: options},
	}
	return r
}

// scanningLines returns the display of a scanner drawing one of its layouts:
// plain rows above a bottom row of three soft keys, which is the row the soft
// key positions are read off.
func scanningLines(count int) []line {
	if count == 0 {
		return nil
	}

	out := make([]line, 0, count)
	for i := 0; i < count-1; i++ {
		out = append(out, line{text: fmt.Sprintf("row %d", i)})
	}
	return append(out, line{text: "MENU  SEL  HOLD", attrs: "**** *** ****"})
}

// screenRows returns how many rows a layout's editor screen has: enough for
// every area the built-in map places, and one more for the soft key row the
// editor can say nothing about.
func screenRows(name string) int {
	most := 0
	for _, at := range positions[name] {
		if at.Line+at.Height > most {
			most = at.Line + at.Height
		}
	}
	return most + 1
}

// spanLines returns an editor's screen with one area drawn in reverse video,
// which is how the editor says which area the knob is on.
func spanLines(at position, count int) []line {
	out := make([]line, 0, count)
	for i := 0; i < count; i++ {
		l := line{text: fmt.Sprintf("row %d", i)}
		if at.Length > 0 && i >= at.Line && i < at.Line+at.Height {
			l.attrs = strings.Repeat(" ", at.Column) + strings.Repeat("*", at.Length)
		}
		out = append(out, l)
	}
	return out
}

// use hands the fake radio to an application context as its scanner.
func use(app *appcontext.App, r *radio) {
	app.SetDevice(device.New(fakeConn{reply: r.reply}))
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (10 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its flags and help text
//   - Subcommands: every subcommand is attached
//   - Args: a layout is accepted, and anything else is refused by name
//   - TooManyArgs: naming two layouts is refused
//   - Questions: the three questions cannot be combined
//   - Cache: the cache cannot answer a question that has no colors in it
//   - AllAndLayout: reading every layout cannot also name one
//   - AllAndQuestion: reading every layout cannot be combined with the rest
//   - Dispatch: each flag runs the walk it belongs to
//   - RunsAll: every layout is read and reported
func TestNew(t *testing.T) {
	// Verify that the command carries its name, its flags and its help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Name() != "colors" {
			t.Errorf("the command is %q, wanted %q", cmd.Name(), "colors")
		}
		for _, name := range []string{"positions", "verify-positions", "verify-palette", "cache", "all"} {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("the command has no --%s flag", name)
			}
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
		if len(cmd.ValidArgs) != len(layouts) {
			t.Errorf("the command offers %d layouts to complete, wanted %d", len(cmd.ValidArgs), len(layouts))
		}
	})

	// Verify that each of the three subcommands is reachable
	t.Run("Subcommands", func(t *testing.T) {
		attached := map[string]bool{}
		for _, sub := range New(appcontext.New()).Commands() {
			attached[sub.Name()] = true
		}

		for _, name := range []string{"set", "reset", "palette"} {
			if !attached[name] {
				t.Errorf("the %s subcommand is not attached", name)
			}
		}
	})

	// Verify that a layout is accepted and anything else is refused by name
	t.Run("Args", func(t *testing.T) {
		cmd := New(appcontext.New())

		if err := cmd.Args(cmd, []string{"weather"}); err != nil {
			t.Errorf("a layout was refused: %v", err)
		}

		err := cmd.Args(cmd, []string{"purple"})
		if err == nil {
			t.Fatal("a word that is not a layout was accepted")
		}
		if !strings.Contains(err.Error(), "is not a layout") ||
			!strings.Contains(err.Error(), "simple-conventional") {
			t.Errorf("the refusal does not say what would have been accepted: %v", err)
		}
	})

	// Verify that naming two layouts is refused
	t.Run("TooManyArgs", func(t *testing.T) {
		cmd := New(appcontext.New())
		if err := cmd.Args(cmd, []string{"weather", "search"}); err == nil {
			t.Error("two layouts were accepted")
		}
	})

	// Verify that the three questions cannot be asked at once
	t.Run("Questions", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := New(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"--positions", "--verify-palette"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Errorf("two questions were accepted: %v", err)
		}
	})

	// Verify that the cache cannot answer a question that has no colors in it
	t.Run("Cache", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := New(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"--cache", "--positions"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "nothing for the cache to answer") {
			t.Errorf("--cache was combined with --positions: %v", err)
		}
	})

	// Verify that reading every layout cannot also name one
	t.Run("AllAndLayout", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := New(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"--all", "weather"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "cannot be named as well") {
			t.Errorf("--all was given a layout: %v", err)
		}
	})

	// Verify that reading every layout cannot be combined with the rest
	t.Run("AllAndQuestion", func(t *testing.T) {
		app, _, _ := newApp()
		cmd := New(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"--all", "--cache"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "reads every layout from the scanner") {
			t.Errorf("--all was combined with --cache: %v", err)
		}
	})

	// Verify that each flag runs the walk it belongs to
	t.Run("Dispatch", func(t *testing.T) {
		for _, c := range []struct {
			name  string
			args  []string
			areas int
			wants string
		}{
			{"Colors", nil, 2, "layout: simple-conventional"},
			{"Positions", []string{"--positions"}, 2, "layout: simple-conventional"},
			{"VerifyPositions", []string{"--verify-positions"}, len(areaOrder["simple-conventional"]), "Every area is where"},
			{"VerifyPalette", []string{"--verify-palette"}, 2, "Every color is the one"},
		} {
			t.Run(c.name, func(t *testing.T) {
				isolate(t)

				app, out, _ := newApp()
				use(app, newRadio("conventional_scan", 14, c.areas))

				cmd := New(app)
				cmd.SetOut(app.Stdout)
				cmd.SetErr(app.Stderr)
				cmd.SetArgs(c.args)

				if err := cmd.Execute(); err != nil {
					t.Fatalf("running %v: %v", c.args, err)
				}
				if !strings.Contains(out.String(), c.wants) {
					t.Errorf("running %v wrote:\n%s", c.args, out)
				}
			})
		}
	})

	// Verify that --all reads every layout the scanner has
	t.Run("RunsAll", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		use(app, newRadio("conventional_scan", 14, 2))

		cmd := New(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"--all"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("reading every layout: %v", err)
		}
		if !strings.Contains(out.String(), fmt.Sprintf("%d layouts read", len(layouts))) {
			t.Errorf("--all wrote:\n%s", out)
		}
	})
}

// Test_areaName tests the areaName function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Strips: the wording the scanner wraps a name in comes off
func Test_areaName(t *testing.T) {
	// Verify that the surrounding wording and whitespace come off
	t.Run("Strips", func(t *testing.T) {
		for _, c := range []struct{ title, want string }{
			{"Set System_name Area", "System_name"},
			{"  Set Channel_name Area  ", "Channel_name"},
			{"Func", "Func"},
			{"", ""},
		} {
			if got := areaName(c.title); got != c.want {
				t.Errorf("%q read as %q, wanted %q", c.title, got, c.want)
			}
		}
	})
}

// Test_at tests the at function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Placed: an area the map places renders its number
//   - Unplaced: an area the map does not place renders a dash
func Test_at(t *testing.T) {
	// Verify that a placed area renders the number it was given
	t.Run("Placed", func(t *testing.T) {
		if got := at(area{Length: 16}, 3); got != "3" {
			t.Errorf("a placed area rendered as %q, wanted %q", got, "3")
		}
	})

	// Verify that an unplaced area reads as unplaced rather than as one at the
	// top left, which is what a bare zero would say
	t.Run("Unplaced", func(t *testing.T) {
		if got := at(area{}, 0); got != "-" {
			t.Errorf("an unplaced area rendered as %q, wanted %q", got, "-")
		}
	})
}

// Test_byLineCount tests the byLineCount function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Matches: the layout whose row count the screen has is picked
//   - NoMatch: a row count neither mode draws settles nothing
//   - ScreenError: a screen that cannot be read settles nothing
func Test_byLineCount(t *testing.T) {
	found := matching("conventional_scan")

	// Verify that the row count picks the simple layout from the detail one
	t.Run("Matches", func(t *testing.T) {
		r := newRadio("conventional_scan", detailLines, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, ok := byLineCount(context.Background(), client, found)
		if !ok {
			t.Fatal("seventeen rows settled nothing")
		}
		if got.name != "detail-conventional" {
			t.Errorf("seventeen rows read as %q, wanted %q", got.name, "detail-conventional")
		}
	})

	// Verify that a row count neither mode draws declines rather than guesses
	t.Run("NoMatch", func(t *testing.T) {
		r := newRadio("conventional_scan", 5, 2)
		client := device.New(fakeConn{reply: r.reply})

		if _, ok := byLineCount(context.Background(), client, found); ok {
			t.Error("five rows settled on a layout")
		}
	})

	// Verify that a screen that cannot be read settles nothing
	t.Run("ScreenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		if _, ok := byLineCount(context.Background(), client, found); ok {
			t.Error("an unreadable screen settled on a layout")
		}
	})
}

// Test_candidates tests the candidates function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Found: a view a layout covers is reported at once
//   - Menu: a menu is reported as itself rather than waited out
//   - AskError: a scanner that will not say what it is doing is reported
//   - Settles: a view no layout covers is given the settling budget
//   - Cancelled: a run cancelled while waiting is reported
func Test_candidates(t *testing.T) {
	// Verify that a view a layout covers comes back at once
	t.Run("Found", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		found, screen, err := candidates(context.Background(), client)
		if err != nil {
			t.Fatalf("asking what the scanner is doing: %v", err)
		}
		if screen != "wx_alert" || len(found) != 1 || found[0].name != "weather" {
			t.Errorf("%q came back as %v", screen, found)
		}
	})

	// Verify that a menu is reported rather than waited out, since it will
	// stay a menu
	t.Run("Menu", func(t *testing.T) {
		r := newRadio("menu_selection", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		found, screen, err := candidates(context.Background(), client)
		if err != nil {
			t.Fatalf("asking what the scanner is doing: %v", err)
		}
		if len(found) != 0 || screen != "menu_selection" {
			t.Errorf("a menu came back as %q and %v", screen, found)
		}
	})

	// Verify that a scanner that will not answer is reported
	t.Run("AskError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		r.failOn("GSI", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		_, _, err := candidates(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "asking the scanner what it is doing") {
			t.Errorf("a silent scanner came back as %v", err)
		}
	})

	// Verify that a view no layout covers is given the settling budget and
	// then reported as covering nothing
	t.Run("Settles", func(t *testing.T) {
		fast(t)

		r := newRadio("recording", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		found, screen, err := candidates(context.Background(), client)
		if err != nil {
			t.Fatalf("waiting for the screen to settle: %v", err)
		}
		if len(found) != 0 || screen != "recording" {
			t.Errorf("an unknown view came back as %q and %v", screen, found)
		}
	})

	// Verify that a run cancelled while waiting is reported as cancelled
	t.Run("Cancelled", func(t *testing.T) {
		fast(t)

		r := newRadio("recording", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, _, err := candidates(ctx, client); !errors.Is(err, context.Canceled) {
			t.Errorf("a cancelled run came back as %v", err)
		}
	})
}

// Test_clean tests the clean function with 100% coverage.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Strips: the padding and the control bytes come off
func Test_clean(t *testing.T) {
	// Verify that only the printable characters of a row survive
	t.Run("Strips", func(t *testing.T) {
		for _, c := range []struct{ text, want string }{
			{"  Yellow  ", "Yellow"},
			{"Quick Save Favorites L\x01\x02", "Quick Save Favorites L"},
			{"", ""},
		} {
			if got := clean(c.text); got != c.want {
				t.Errorf("%q cleaned to %q, wanted %q", c.text, got, c.want)
			}
		}
	})
}

// Test_colored tests the colored function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Text: a reading with a text color holds colors
//   - Background: a reading with a background color holds colors
//   - None: a positions only reading holds none
func Test_colored(t *testing.T) {
	// Verify that a text color counts
	t.Run("Text", func(t *testing.T) {
		if !colored([]area{{Name: "Func"}, {Name: "Battery", Text: "Yellow"}}) {
			t.Error("a reading with a text color reported no colors")
		}
	})

	// Verify that a background color counts
	t.Run("Background", func(t *testing.T) {
		if !colored([]area{{Name: "Func", Background: "Black"}}) {
			t.Error("a reading with a background color reported no colors")
		}
	})

	// Verify that a positions only reading reports none
	t.Run("None", func(t *testing.T) {
		if colored([]area{{Name: "Func"}}) {
			t.Error("a reading with no colors reported some")
		}
	})
}

// Test_currentLayout tests the currentLayout function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - One: a view only one layout covers settles it
//   - Rows: two layouts are told apart by the screen's row count
//   - Menu: a scanner in a menu is drawing with no layout
//   - Unknown: a view no layout covers is quoted back
//   - Mode: a row count that is neither falls back to the menu that sets it
//   - ModeUnknown: a mode that is neither of the two is reported
func Test_currentLayout(t *testing.T) {
	// Verify that a view only one layout covers settles it with one read
	t.Run("One", func(t *testing.T) {
		r := newRadio("tone_out", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := currentLayout(context.Background(), client)
		if err != nil {
			t.Fatalf("working out the layout: %v", err)
		}
		if got.name != "tone-out" {
			t.Errorf("the layout read as %q, wanted %q", got.name, "tone-out")
		}
	})

	// Verify that the screen's own row count separates the simple layout from
	// the detail one
	t.Run("Rows", func(t *testing.T) {
		r := newRadio("trunk_scan", simpleLines, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := currentLayout(context.Background(), client)
		if err != nil {
			t.Fatalf("working out the layout: %v", err)
		}
		if got.name != "simple-trunk" {
			t.Errorf("fourteen rows read as %q, wanted %q", got.name, "simple-trunk")
		}
	})

	// Verify that a scanner in a menu is drawing with no layout
	t.Run("Menu", func(t *testing.T) {
		r := newRadio("menu_selection", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		_, err := currentLayout(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "the scanner is in a menu") {
			t.Errorf("a scanner in a menu came back as %v", err)
		}
	})

	// Verify that a view no layout covers is quoted back
	t.Run("Unknown", func(t *testing.T) {
		fast(t)

		r := newRadio("recording", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		_, err := currentLayout(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), `"recording"`) {
			t.Errorf("an unknown view came back as %v", err)
		}
	})

	// Verify that a row count neither mode draws falls back to the menu that
	// sets the mode
	t.Run("Mode", func(t *testing.T) {
		r := newRadio("conventional_scan", 5, 2)
		r.modeMenu.rows = []string{detailEntry, simpleEntry}
		client := device.New(fakeConn{reply: r.reply})

		got, err := currentLayout(context.Background(), client)
		if err != nil {
			t.Fatalf("working out the layout: %v", err)
		}
		if got.name != "detail-conventional" {
			t.Errorf("the mode menu settled on %q, wanted %q", got.name, "detail-conventional")
		}
	})

	// Verify that a mode which is neither of the two is reported rather than
	// guessed at
	t.Run("ModeUnknown", func(t *testing.T) {
		r := newRadio("conventional_scan", 5, 2)
		r.modeMenu.rows = []string{"Some Other Mode"}
		client := device.New(fakeConn{reply: r.reply})

		_, err := currentLayout(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "the scan display mode is") {
			t.Errorf("an unknown mode came back as %v", err)
		}
	})

	// Verify that a scanner which will not say what it is doing is reported
	t.Run("AskError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("GSI", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := currentLayout(context.Background(), client); err == nil {
			t.Error("a silent scanner settled on a layout")
		}
	})

	// Verify that a mode menu which will not open is reported
	t.Run("ModeError", func(t *testing.T) {
		r := newRadio("conventional_scan", 5, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := currentLayout(context.Background(), client); err == nil {
			t.Error("a mode menu that will not open settled a layout")
		}
	})
}

// Test_indexOf tests the indexOf function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Found: an entry is found without regard to case or whitespace
//   - Missing: a menu that does not hold it answers -1
func Test_indexOf(t *testing.T) {
	items := []device.MenuItem{{Name: "Set Option"}, {Name: "  set text color  "}, {Name: "Set Back Color"}}

	// Verify that an entry is found whatever its case and padding
	t.Run("Found", func(t *testing.T) {
		if got := indexOf(items, textColor); got != 1 {
			t.Errorf("%q was found at %d, wanted 1", textColor, got)
		}
	})

	// Verify that a menu without the entry answers -1
	t.Run("Missing", func(t *testing.T) {
		if got := indexOf(items, "Set Nothing"); got != -1 {
			t.Errorf("a missing entry was found at %d, wanted -1", got)
		}
	})
}

// Test_isCurrent tests the isCurrent function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Same: the one layout that covers the view is the one asked about
//   - Different: a layout the view does not cover is not the one on screen
//   - Rows: two layouts are told apart by the screen's row count
//   - Mode: a row count that is neither falls back to the menu that sets it
//   - AskError: a scanner that will not say what it is doing is reported
func Test_isCurrent(t *testing.T) {
	weather, _ := lookup("weather")
	simple, _ := lookup("simple-conventional")
	detail, _ := lookup("detail-conventional")

	// Verify that the one layout covering the view is reported as current
	t.Run("Same", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := isCurrent(context.Background(), client, weather)
		if err != nil {
			t.Fatalf("asking whether the layout is current: %v", err)
		}
		if !got {
			t.Error("the layout on screen was not reported as current")
		}
	})

	// Verify that a layout the view does not cover is not current
	t.Run("Different", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := isCurrent(context.Background(), client, simple)
		if err != nil {
			t.Fatalf("asking whether the layout is current: %v", err)
		}
		if got {
			t.Error("a layout that is not on screen was reported as current")
		}
	})

	// Verify that the row count separates the simple layout from the detail one
	t.Run("Rows", func(t *testing.T) {
		r := newRadio("conventional_scan", detailLines, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := isCurrent(context.Background(), client, detail)
		if err != nil {
			t.Fatalf("asking whether the layout is current: %v", err)
		}
		if !got {
			t.Error("seventeen rows did not report the detail layout as current")
		}
	})

	// Verify that a row count neither mode draws falls back to the mode menu
	t.Run("Mode", func(t *testing.T) {
		r := newRadio("conventional_scan", 5, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := isCurrent(context.Background(), client, simple)
		if err != nil {
			t.Fatalf("asking whether the layout is current: %v", err)
		}
		if !got {
			t.Error("the mode menu did not report the simple layout as current")
		}
	})

	// Verify that a scanner which will not answer is reported
	t.Run("AskError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		r.failOn("GSI", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := isCurrent(context.Background(), client, weather); err == nil {
			t.Error("a silent scanner reported an answer")
		}
	})

	// Verify that a mode menu which will not open is reported, since the
	// answer turns on it
	t.Run("ModeError", func(t *testing.T) {
		r := newRadio("conventional_scan", 5, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := isCurrent(context.Background(), client, simple); err == nil {
			t.Error("a mode menu that will not open reported an answer")
		}
	})
}

// Test_layoutNames tests the layoutNames function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Lists: every layout is named, in the Customize menu's order
func Test_layoutNames(t *testing.T) {
	// Verify that every layout is named in the menu's own order
	t.Run("Lists", func(t *testing.T) {
		got := layoutNames()
		if len(got) != len(layouts) {
			t.Fatalf("%d names, wanted %d", len(got), len(layouts))
		}
		for i, l := range layouts {
			if got[i] != l.name {
				t.Errorf("name %d is %q, wanted %q", i, got[i], l.name)
			}
		}
	})
}

// Test_lookup tests the lookup function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Found: a name is matched without regard to case
//   - Missing: anything else is refused
func Test_lookup(t *testing.T) {
	// Verify that a name is matched whatever its case
	t.Run("Found", func(t *testing.T) {
		got, ok := lookup("SIMPLE-TRUNK")
		if !ok {
			t.Fatal("a layout was not found")
		}
		if got.entry != "Set Simple Trunk" {
			t.Errorf("the layout is %q, wanted %q", got.entry, "Set Simple Trunk")
		}
	})

	// Verify that a name which is not one of the seven is refused
	t.Run("Missing", func(t *testing.T) {
		if _, ok := lookup("purple"); ok {
			t.Error("a word that is not a layout was found")
		}
	})
}

// Test_matching tests the matching function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Two: a view with a simple and a detail version reports both
//   - One: a view with only one layout reports it
//   - None: a view no layout covers reports none
func Test_matching(t *testing.T) {
	// Verify that a view drawn by two layouts reports both of them
	t.Run("Two", func(t *testing.T) {
		got := matching("CONVENTIONAL_SCAN")
		if len(got) != 2 {
			t.Fatalf("%d layouts cover the conventional scan screen, wanted 2", len(got))
		}
		if got[0].name != "simple-conventional" || got[1].name != "detail-conventional" {
			t.Errorf("they are %q and %q", got[0].name, got[1].name)
		}
	})

	// Verify that a view drawn by one layout reports it
	t.Run("One", func(t *testing.T) {
		got := matching("close_call")
		if len(got) != 1 || got[0].name != "search" {
			t.Errorf("the close call screen came back as %v", got)
		}
	})

	// Verify that a view no layout covers reports none
	t.Run("None", func(t *testing.T) {
		if got := matching("recording"); len(got) != 0 {
			t.Errorf("a view no layout covers came back as %v", got)
		}
	})
}

// Test_names tests the names function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Lists: every layout gets a line naming it and its menu entry
func Test_names(t *testing.T) {
	// Verify that every layout is listed beside the entry it lives behind
	t.Run("Lists", func(t *testing.T) {
		got := names()
		if lines := strings.Count(got, "\n"); lines != len(layouts) {
			t.Errorf("%d lines, wanted %d:\n%s", lines, len(layouts), got)
		}
		for _, l := range layouts {
			if !strings.Contains(got, l.name) || !strings.Contains(got, l.entry) {
				t.Errorf("%s is not listed:\n%s", l.name, got)
			}
		}
	})
}

// Test_open tests the open function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Walks: the scanner is left on the menu at the end of the path
//   - TopError: a top menu that will not open is reported
//   - EntryError: an entry that cannot be found is reported by name
func Test_open(t *testing.T) {
	// Verify that the walk leaves the scanner on the menu it was sent to
	t.Run("Walks", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		if err := open(context.Background(), client, displayOptions, customize); err != nil {
			t.Fatalf("walking to the Customize menu: %v", err)
		}
		if r.on() != r.customizeMenu {
			t.Errorf("the walk landed on %q", r.on().title)
		}
	})

	// Verify that a top menu which will not open is reported
	t.Run("TopError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		err := open(context.Background(), client, displayOptions)
		if err == nil || !strings.Contains(err.Error(), "opening the top menu") {
			t.Errorf("a refused top menu came back as %v", err)
		}
	})

	// Verify that an entry the menu does not hold is reported by name
	t.Run("EntryError", func(t *testing.T) {
		r := newRadio("wx_alert", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		err := open(context.Background(), client, "Nowhere")
		if err == nil || !strings.Contains(err.Error(), `looking for "Nowhere"`) {
			t.Errorf("a missing entry came back as %v", err)
		}
	})
}

// Test_place tests the place function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Mapped: every area the built-in map places carries its position
//   - SoftKeys: the layout on screen has its soft keys read off the live row
//   - ScreenError: a screen that cannot be read is reported
func Test_place(t *testing.T) {
	want, _ := lookup("simple-conventional")

	// Verify that the built-in map fills in the positions, and that a layout
	// which is not the one on screen has no soft keys read for it
	t.Run("Mapped", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		areas := []area{{Name: "System_name"}, {Name: "Soft1_key"}}
		if err := place(context.Background(), client, want, areas, false); err != nil {
			t.Fatalf("placing the areas: %v", err)
		}

		if areas[0].Line != 2 || areas[0].Length != 24 || areas[0].Height != 2 {
			t.Errorf("System_name was placed at %+v", areas[0])
		}
		if areas[1].Length != 0 {
			t.Errorf("a soft key was placed for a layout that is not on screen: %+v", areas[1])
		}
	})

	// Verify that the soft keys of the layout on screen come off the live row
	t.Run("SoftKeys", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		areas := []area{{Name: "Soft1_key"}, {Name: "Soft3_key"}}
		if err := place(context.Background(), client, want, areas, true); err != nil {
			t.Fatalf("placing the areas: %v", err)
		}

		if areas[0].Line != 13 || areas[0].Column != 0 || areas[0].Length != 4 {
			t.Errorf("the first soft key was placed at %+v", areas[0])
		}
		if areas[1].Column != 9 || areas[1].Length != 4 {
			t.Errorf("the third soft key was placed at %+v", areas[1])
		}
	})

	// Verify that a screen which cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		err := place(context.Background(), client, want, []area{{Name: "Soft1_key"}}, true)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("an unreadable screen came back as %v", err)
		}
	})
}

// Test_readArea tests the readArea function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Reads: both colors come back, and the scanner is left on the area's menu
//   - NoEntries: an area with no color entries comes back named and colorless
//   - MenuError: an area's menu that cannot be read is reported
//   - EnterError: a scanner that will not take the key press is reported
//   - PickerError: a picker showing no color value is reported by area
func Test_readArea(t *testing.T) {
	// Verify that both colors are read and the walk ends on the area's menu
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		editor := r.customizeMenu.opens["Set Simple Conventional"]
		r.stack = []frame{{on: editor}}

		got, err := readArea(context.Background(), client)
		if err != nil {
			t.Fatalf("reading an area: %v", err)
		}
		if got.Name != "Func" {
			t.Errorf("the area is %q, wanted %q", got.Name, "Func")
		}
		if got.Text != "Yellow" || got.TextHex != "#FFFF00" {
			t.Errorf("the text color read as %s %s", got.Text, got.TextHex)
		}
		if got.Background != "Black" || got.BackgroundHex != "#000000" {
			t.Errorf("the background color read as %s %s", got.Background, got.BackgroundHex)
		}
		if r.on().title != "Set Func Area" {
			t.Errorf("the walk ended on %q", r.on().title)
		}
	})

	// Verify that an area whose menu has no color entries is named anyway
	t.Run("NoEntries", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		editor := &view{
			title: "Set Weather Mode",
			rows:  []string{"Battery"},
			opens: map[string]*view{"Battery": {title: "Set Battery Area", rows: []string{"Set Something"}}},
		}
		r.stack = []frame{{on: editor}}

		got, err := readArea(context.Background(), client)
		if err != nil {
			t.Fatalf("reading an area: %v", err)
		}
		if got.Name != "Battery" || got.Text != "" || got.Background != "" {
			t.Errorf("an area with no color entries came back as %+v", got)
		}
	})

	// Verify that an area menu which cannot be read is reported
	t.Run("MenuError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("MSI", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		_, err := readArea(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading an area's menu") {
			t.Errorf("an unreadable menu came back as %v", err)
		}
	})

	// Verify that a scanner refusing the key press is reported
	t.Run("EnterError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("KEY,E,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		_, err := readArea(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Errorf("a refused key came back as %v", err)
		}
	})

	// Verify that a picker showing no color value is reported by area
	t.Run("PickerError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		editor := &view{
			title: "Set Weather Mode",
			rows:  []string{"Battery"},
			opens: map[string]*view{"Battery": {
				title: "Set Battery Area",
				rows:  []string{textColor},
				opens: map[string]*view{textColor: {title: textColor, rows: []string{"nothing here"}}},
			}},
		}
		r.stack = []frame{{on: editor}}

		_, err := readArea(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "for Battery") {
			t.Errorf("a picker with no value came back as %v", err)
		}
	})

	// Verify that a knob which will not turn to the second color is reported
	t.Run("StepError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}
		r.fails = func(command string) error {
			if command == "KEY,>,P" && r.onAreaMenu() {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		_, err := readArea(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Errorf("a refused key came back as %v", err)
		}
	})

	// Verify that a picker which will not open is reported
	t.Run("OpenPickerError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}
		r.fails = func(command string) error {
			if command == "KEY,E,P" && r.onAreaMenu() {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		if _, err := readArea(context.Background(), client); err == nil {
			t.Error("a picker that will not open was read")
		}
	})

	// Verify that a picker which will not close is reported, since leaving it
	// without choosing anything is what keeps this a read
	t.Run("BackError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.stack = []frame{{on: r.customizeMenu.opens["Set Simple Conventional"]}}
		r.fails = func(command string) error {
			if command == "KEY,M,P" && r.onPicker() {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		if _, err := readArea(context.Background(), client); err == nil {
			t.Error("a picker that will not close was read")
		}
	})
}

// Test_readLayout tests the readLayout function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Reads: every area of the layout comes back, once each
//   - OpenError: an editor that will not open is reported
//   - MenuError: an editor whose listing cannot be read is reported
//   - NoAreas: an editor reporting no areas is reported
//   - AreaError: an area that cannot be read is reported
//   - NeverComesRound: an editor that does not wrap is given up on
func Test_readLayout(t *testing.T) {
	want, _ := lookup("simple-conventional")

	// Verify that every area is read once and the walk stops when it wraps
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})

		got, err := readLayout(context.Background(), client, want)
		if err != nil {
			t.Fatalf("reading the layout: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("%d areas came back, wanted 3", len(got))
		}
		if got[0].Name != "Func" || got[2].Name != "Option_2" {
			t.Errorf("the areas came back as %v", got)
		}
		if got[0].Text != "Yellow" {
			t.Errorf("the first area's text color read as %q", got[0].Text)
		}
	})

	// Verify that an editor which will not open is reported
	t.Run("OpenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := readLayout(context.Background(), client, want); err == nil {
			t.Error("an editor that will not open was walked")
		}
	})

	// Verify that an editor whose listing cannot be read is reported
	t.Run("MenuError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "MSI" && r.on() == r.customizeMenu.opens[want.entry] {
				return errors.New("the port is gone")
			}
			return nil
		}

		_, err := readLayout(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "reading the areas of") {
			t.Errorf("an unreadable editor came back as %v", err)
		}
	})

	// Verify that an editor reporting no areas is reported rather than walked
	t.Run("NoAreas", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		r.customizeMenu.opens[want.entry].bare = true
		client := device.New(fakeConn{reply: r.reply})

		_, err := readLayout(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "reports no areas") {
			t.Errorf("an empty editor came back as %v", err)
		}
	})

	// Verify that an area which cannot be read stops the walk
	t.Run("AreaError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "MSI" && r.on() != nil && strings.HasSuffix(r.on().title, " Area") {
				return errors.New("the port is gone")
			}
			return nil
		}

		if _, err := readLayout(context.Background(), client, want); err == nil {
			t.Error("an unreadable area was walked past")
		}
	})

	// Verify that an editor which never comes back round is given up on
	t.Run("NeverComesRound", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)

		editor := &view{title: want.entry, opens: map[string]*view{}}
		for i := 0; i <= maxAreas; i++ {
			name := fmt.Sprintf("Area_%d", i)
			editor.rows = append(editor.rows, name)
			editor.opens[name] = &view{title: "Set " + name + " Area", rows: []string{"Set Something"}}
		}
		r.customizeMenu.opens[want.entry] = editor
		client := device.New(fakeConn{reply: r.reply})

		_, err := readLayout(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "did not come back round") {
			t.Errorf("an editor that never wraps came back as %v", err)
		}
	})

	// Verify that an area the scanner will not back out of is reported
	t.Run("BackError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "KEY,M,P" && r.onAreaMenu() {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		if _, err := readLayout(context.Background(), client, want); err == nil {
			t.Error("an area that will not close was walked past")
		}
	})

	// Verify that a knob which will not step to the next area is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 3)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "KEY,>,P" && r.on() == r.customizeMenu.opens[want.entry] {
				return errors.New("the scanner refused the key")
			}
			return nil
		}

		_, err := readLayout(context.Background(), client, want)
		if err == nil || !strings.Contains(err.Error(), "stepping to the next area") {
			t.Errorf("a refused key came back as %v", err)
		}
	})
}

// Test_readPicker tests the readPicker function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Reads: the name and the value come off the screen
//   - NoName: a value with nothing above it comes back nameless
//   - NoValue: a screen that is not a picker is reported
//   - ScreenError: a screen that cannot be read is reported
func Test_readPicker(t *testing.T) {
	// Verify that the name above the value is the color being shown
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.stack = []frame{{on: &view{title: textColor, picker: true, at: colorAt("Orangered")}}}
		client := device.New(fakeConn{reply: r.reply})

		name, hex, err := readPicker(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the picker: %v", err)
		}
		if name != "Orangered" || hex != "#FF4600" {
			t.Errorf("the picker read as %s %s", name, hex)
		}
	})

	// Verify that a value with nothing readable above it comes back nameless
	t.Run("NoName", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.stack = []frame{{on: &view{title: textColor, rows: []string{"   ", "RGB = 00FF00h"}}}}
		client := device.New(fakeConn{reply: r.reply})

		name, hex, err := readPicker(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the picker: %v", err)
		}
		if name != "" || hex != "#00FF00" {
			t.Errorf("a nameless picker read as %q %q", name, hex)
		}
	})

	// Verify that a screen which is not a picker is reported
	t.Run("NoValue", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.stack = []frame{{on: &view{title: textColor, rows: []string{"Set Text Color"}}}}
		client := device.New(fakeConn{reply: r.reply})

		_, _, err := readPicker(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "no color value") {
			t.Errorf("a screen that is not a picker came back as %v", err)
		}
	})

	// Verify that a screen which cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		_, _, err := readPicker(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("an unreadable screen came back as %v", err)
		}
	})
}

// Test_renderReport tests the renderReport function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Colors: the table carries both colors of every area
//   - Positions: a reading with no colors prints no color columns
//   - Cached: a stored reading says how old it is
//   - JSON: the JSON form is the whole reading
//   - WriteError: a stream that refuses the table is reported
func Test_renderReport(t *testing.T) {
	r := report{
		Layout:  "weather",
		Menu:    "Set Weather Mode",
		Current: true,
		Areas: []area{
			{Name: "Func", Line: 0, Column: 0, Length: 2, Height: 1,
				Text: "Yellow", TextHex: "#FFFF00", Background: "Black", BackgroundHex: "#000000"},
			{Name: "Soft1_key"},
		},
	}

	// Verify that the table carries both colors and marks the unplaced area
	t.Run("Colors", func(t *testing.T) {
		app, out, _ := newApp()
		if err := renderReport(app, r); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		for _, want := range []string{"layout: weather", "drawing with this one", "BACKGROUND",
			"Yellow", "#000000", "2 areas"} {
			if !strings.Contains(got, want) {
				t.Errorf("the table does not carry %q:\n%s", want, got)
			}
		}
		for _, row := range strings.Split(got, "\n") {
			if !strings.HasPrefix(row, "Soft1_key") {
				continue
			}
			if strings.Count(row, "-") != 8 {
				t.Errorf("the unplaced area is not dashed out: %q", row)
			}
		}
	})

	// Verify that a reading with no colors prints no color columns
	t.Run("Positions", func(t *testing.T) {
		app, out, _ := newApp()
		bare := r
		bare.Areas = []area{{Name: "Func", Length: 2, Height: 1}}

		if err := renderReport(app, bare); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if strings.Contains(out.String(), "BACKGROUND") {
			t.Errorf("a positions only reading printed color columns:\n%s", out)
		}
	})

	// Verify that a stored reading says how old it is, which is the whole of
	// what makes it worth anything
	t.Run("Cached", func(t *testing.T) {
		app, out, _ := newApp()
		stored := r
		stored.Cached, stored.Read = true, time.Now().Add(-2*time.Hour)

		if err := renderReport(app, stored); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "cached:") || !strings.Contains(out.String(), "hours ago") {
			t.Errorf("a cached reading does not say how old it is:\n%s", out)
		}
	})

	// Verify that the JSON form is the whole reading
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := renderReport(app, r); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the JSON does not parse: %v", err)
		}
		if got.Layout != "weather" || len(got.Areas) != 2 {
			t.Errorf("the JSON came back as %+v", got)
		}
	})

	// Verify that a stream which refuses the table is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		if err := renderReport(app, r); err == nil {
			t.Error("a refused stream was reported as written")
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a run with no scanner named is reported
//   - Current: the layout on screen is read and reported
//   - Named: a named layout is read and said not to be the one on screen
//   - LayoutError: a scanner drawing nothing a layout covers is reported
//   - Cached: a stored reading is handed back without opening a menu
//   - CacheMiss: a layout that has never been read is an error
//   - CacheError: a cache that cannot be located is reported
//   - PlaceError: a live screen that cannot be read is reported
func Test_run(t *testing.T) {
	// Verify that a run with no scanner named is reported
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()
		if err := run(context.Background(), app, "", false); err == nil {
			t.Error("a run with no scanner reported success")
		}
	})

	// Verify that the layout on screen is read and reported as current
	t.Run("Current", func(t *testing.T) {
		isolate(t)

		app, out, notes := newApp()
		use(app, newRadio("conventional_scan", simpleLines, 2))

		if err := run(context.Background(), app, "", false); err != nil {
			t.Fatalf("reading the colors: %v", err)
		}
		if !strings.Contains(out.String(), "layout: simple-conventional") ||
			!strings.Contains(out.String(), "drawing with this one") {
			t.Errorf("the reading came back as:\n%s", out)
		}
		if !strings.Contains(notes.String(), "stops the scan") {
			t.Errorf("the run said nothing about stopping the scan:\n%s", notes)
		}
	})

	// Verify that a named layout is read and not claimed to be on screen
	t.Run("Named", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		if err := run(context.Background(), app, "tone-out", false); err != nil {
			t.Fatalf("reading the colors: %v", err)
		}
		if !strings.Contains(out.String(), "layout: tone-out") {
			t.Errorf("the reading came back as:\n%s", out)
		}
		if strings.Contains(out.String(), "drawing with this one") {
			t.Errorf("a layout that is not on screen was called current:\n%s", out)
		}
	})

	// Verify that a scanner drawing nothing a layout covers is reported
	t.Run("LayoutError", func(t *testing.T) {
		isolate(t)
		fast(t)

		app, _, _ := newApp()
		use(app, newRadio("recording", 14, 2))

		if err := run(context.Background(), app, "", false); err == nil {
			t.Error("a view no layout covers was read")
		}
	})

	// Verify that a stored reading is handed back without opening a menu
	t.Run("Cached", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		use(app, r)

		remember(quiet(), fakeConn{}.Info(), "weather",
			[]area{{Name: "System_name", Text: "Red", TextHex: "#FF0000"}}, time.Now())

		if err := run(context.Background(), app, "weather", true); err != nil {
			t.Fatalf("reading the cache: %v", err)
		}
		if !strings.Contains(out.String(), "cached:") || !strings.Contains(out.String(), "Red") {
			t.Errorf("the cached reading came back as:\n%s", out)
		}
		for _, key := range r.presses {
			if key == "E" {
				t.Fatal("reading from the cache opened a menu")
			}
		}
	})

	// Verify that a layout which has never been read is an error rather than
	// a wait
	t.Run("CacheMiss", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		err := run(context.Background(), app, "weather", true)
		if err == nil || !strings.Contains(err.Error(), "no cached colors for weather") {
			t.Errorf("an empty cache came back as %v", err)
		}
	})

	// Verify that a cache which cannot be located is reported
	t.Run("CacheError", func(t *testing.T) {
		isolate(t)
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("LocalAppData", "")

		app, _, _ := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		if err := run(context.Background(), app, "weather", true); err == nil {
			t.Error("a cache with nowhere to live was read")
		}
	})

	// Verify that a live screen which cannot be read is reported
	t.Run("PlaceError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.fails = func(command string) error {
			// Only once the walk is out of the menus, which is where the soft
			// keys are read from.
			if command == "STS" && len(r.stack) == 0 {
				return errors.New("the port is gone")
			}
			return nil
		}
		use(app, r)

		err := run(context.Background(), app, "weather", false)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("an unreadable live screen came back as %v", err)
		}
	})

	// Verify that an editor which reports no areas is reported
	t.Run("ReadError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.customizeMenu.opens["Set Weather Mode"].bare = true
		use(app, r)

		err := run(context.Background(), app, "weather", false)
		if err == nil || !strings.Contains(err.Error(), "reports no areas") {
			t.Errorf("an empty editor came back as %v", err)
		}
	})

	// Verify that a scanner which will not leave its menus is reported
	t.Run("LeaveError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("KEY,L,P", errors.New("the scanner refused the key"))
		use(app, r)

		err := run(context.Background(), app, "weather", false)
		if err == nil || !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("a scanner stuck in its menus came back as %v", err)
		}
	})
}

// Test_scanDisplayMode tests the scanDisplayMode function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Reads: the highlighted entry is the mode the scanner is set to
//   - OpenError: a menu that will not open is reported
//   - RowError: a highlighted row that cannot be read is reported
//   - BackError: a scanner that will not back out is reported
func Test_scanDisplayMode(t *testing.T) {
	// Verify that the highlighted entry is what the mode is
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.modeMenu.rows = []string{detailEntry, simpleEntry}
		client := device.New(fakeConn{reply: r.reply})

		got, err := scanDisplayMode(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the scan display mode: %v", err)
		}
		if got != detailEntry {
			t.Errorf("the mode read as %q, wanted %q", got, detailEntry)
		}
	})

	// Verify that a menu which will not open is reported
	t.Run("OpenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("MNU,TOP,", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := scanDisplayMode(context.Background(), client); err == nil {
			t.Error("a menu that will not open was read")
		}
	})

	// Verify that a highlighted row which cannot be read is reported
	t.Run("RowError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})
		r.fails = func(command string) error {
			if command == "STS" && r.on() == r.modeMenu {
				return errors.New("the port is gone")
			}
			return nil
		}

		_, err := scanDisplayMode(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading the scan display mode") {
			t.Errorf("an unreadable row came back as %v", err)
		}
	})

	// Verify that a scanner which will not back out is reported
	t.Run("BackError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("KEY,M,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		_, err := scanDisplayMode(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "going back one level") {
			t.Errorf("a refused key came back as %v", err)
		}
	})
}

// Test_settledSoftKeys tests the settledSoftKeys function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Reads: a row that is three keys comes back at once
//   - Settles: a layout drawing no soft keys spends the budget and reports none
//   - ScreenError: a screen that cannot be read is reported
//   - Cancelled: a run cancelled while waiting is reported
func Test_settledSoftKeys(t *testing.T) {
	// Verify that a bottom row of three keys comes back straight away
	t.Run("Reads", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		got, err := settledSoftKeys(context.Background(), client)
		if err != nil {
			t.Fatalf("reading the soft keys: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("%d soft key areas came back, wanted 5", len(got))
		}
	})

	// Verify that a layout drawing no soft keys reports none once the budget
	// runs out, which is the right answer arrived at slowly
	t.Run("Settles", func(t *testing.T) {
		fast(t)

		r := newRadio("conventional_scan", 14, 2)
		r.scanning = []line{{text: "nothing highlighted"}}
		client := device.New(fakeConn{reply: r.reply})

		got, err := settledSoftKeys(context.Background(), client)
		if err != nil {
			t.Fatalf("waiting for the soft keys: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("a row that is not soft keys came back as %v", got)
		}
	})

	// Verify that a screen which cannot be read is reported
	t.Run("ScreenError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("STS", errors.New("the port is gone"))
		client := device.New(fakeConn{reply: r.reply})

		if _, err := settledSoftKeys(context.Background(), client); err == nil {
			t.Error("an unreadable screen came back as soft keys")
		}
	})

	// Verify that a run cancelled while waiting is reported as cancelled
	t.Run("Cancelled", func(t *testing.T) {
		fast(t)

		r := newRadio("conventional_scan", 14, 2)
		r.scanning = []line{{text: "nothing highlighted"}}
		client := device.New(fakeConn{reply: r.reply})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := settledSoftKeys(ctx, client); !errors.Is(err, context.Canceled) {
			t.Errorf("a cancelled run came back as %v", err)
		}
	})
}

// Test_step tests the step function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Turns: the knob is turned once per step
//   - None: a count of zero or less turns nothing
//   - PressError: a scanner that will not take a key press is reported
func Test_step(t *testing.T) {
	// Verify that the knob is turned once per step
	t.Run("Turns", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		if err := step(context.Background(), client, 3); err != nil {
			t.Fatalf("turning the knob: %v", err)
		}
		if len(r.presses) != 3 {
			t.Errorf("%d keys were pressed, wanted 3", len(r.presses))
		}
	})

	// Verify that a count of zero or less turns nothing
	t.Run("None", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		client := device.New(fakeConn{reply: r.reply})

		if err := step(context.Background(), client, -1); err != nil {
			t.Fatalf("turning nothing: %v", err)
		}
		if len(r.presses) != 0 {
			t.Errorf("%d keys were pressed, wanted none", len(r.presses))
		}
	})

	// Verify that a refused key press is reported
	t.Run("PressError", func(t *testing.T) {
		r := newRadio("conventional_scan", 14, 2)
		r.failOn("KEY,>,P", errors.New("the scanner refused the key"))
		client := device.New(fakeConn{reply: r.reply})

		err := step(context.Background(), client, 1)
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Errorf("a refused key came back as %v", err)
		}
	})
}
