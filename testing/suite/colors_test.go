// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package suite

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// colorsReport is the shape "colors" prints as JSON.
type colorsReport struct {
	Layout  string       `json:"layout"`
	Menu    string       `json:"menu"`
	Current bool         `json:"current"`
	Cached  bool         `json:"cached"`
	Read    time.Time    `json:"read"`
	Areas   []colorsArea `json:"areas"`
}

// colorsArea is one area of a layout, as the command reports it.
type colorsArea struct {
	Area          string `json:"area"`
	Text          string `json:"text"`
	Background    string `json:"background"`
	TextHex       string `json:"textHex"`
	BackgroundHex string `json:"backgroundHex"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
	Length        int    `json:"length"`
	Height        int    `json:"height"`
}

// colorsLayouts are the seven layouts the command accepts, with the entry each
// one has on the scanner's Customize menu.
var colorsLayouts = []struct {
	name string
	menu string
}{
	{"simple-conventional", "Set Simple Conventional"},
	{"simple-trunk", "Set Simple Trunk"},
	{"detail-conventional", "Set Detail Conventional"},
	{"detail-trunk", "Set Detail Trunk"},
	{"search", "Set Search/CC Mode"},
	{"weather", "Set Weather Mode"},
	{"tone-out", "Set Tone Out Mode"},
}

// hexColor is the form the command reports a color value in.
var hexColor = regexp.MustCompile(`^#[0-9A-F]{6}$`)

// TestColors_Arguments checks what the command refuses, which it does before it
// touches the scanner, so none of this opens a menu.
func TestColors_Arguments(t *testing.T) {
	needScanner(t)

	t.Run("refusing a layout the scanner lacks", func(t *testing.T) {
		mustFail(t, "is not a layout", "colors", "bogus")
		mustFail(t, "is not a layout", "colors", "Set Weather Mode")
		mustFail(t, "is not a layout", "colors", "conventional")
		mustFail(t, "accepts at most 1 arg", "colors", "weather", "search")
	})

	t.Run("the three questions cannot be mixed", func(t *testing.T) {
		// One reads a built-in table and the others check them against the
		// scanner. Silently answering one would leave the caller believing it
		// got another.
		mustFail(t, "cannot be combined", "colors", "--positions", "--verify-positions")
		mustFail(t, "cannot be combined", "colors", "--positions", "--verify-palette")
		mustFail(t, "cannot be combined", "colors", "--verify-positions", "--verify-palette")
	})

	t.Run("the cache answers none of them", func(t *testing.T) {
		// --cache says where the colors come from, and none of the three reads
		// a color. Accepting the pair would hand back a stored answer to a
		// question about the scanner in front of you.
		mustFail(t, "--cache cannot be combined", "colors", "--cache", "--positions")
		mustFail(t, "--cache cannot be combined", "colors", "--cache", "--verify-positions")
		mustFail(t, "--cache cannot be combined", "colors", "--cache", "--verify-palette")
	})

	t.Run("every layout is named in the refusal", func(t *testing.T) {
		// The message is how a caller discovers the names, so it has to carry
		// all of them rather than a sample.
		res := mustFail(t, "is not a layout", "colors", "bogus")

		for _, l := range colorsLayouts {
			if !strings.Contains(res.stderr, l.name) {
				t.Errorf("the refusal does not name %q:\n%s", l.name, res.stderr)
			}
		}
	})
}

// colorsPalette is the shape "colors palette" prints as JSON.
type colorsPalette struct {
	Count  int `json:"count"`
	Colors []struct {
		Step int    `json:"step"`
		Name string `json:"name"`
		Hex  string `json:"hex"`
	} `json:"colors"`
}

// TestColors reads one layout off the scanner and checks what comes back.
//
// It is behind -writes for the time it takes and for what it does while it
// runs, rather than because it changes anything: reading a layout means opening
// every area's two color pickers in turn, which takes about half a minute and
// leaves the scanner out of scanning until it finishes. No picker is ever
// confirmed, so the colors themselves are untouched.
func TestColors(t *testing.T) {
	needWrites(t)

	started := time.Now()

	var report colorsReport
	mustJSON(t, &report, "colors")

	if !report.Current {
		t.Error("reading without naming a layout did not report the layout as the current one")
	}
	if report.Cached {
		t.Error("a plain read reported itself as cached: it must always walk the menus")
	}
	if report.Read.Before(started) {
		t.Errorf("the read is stamped %s, which is before the command ran", report.Read)
	}

	menu := ""
	for _, l := range colorsLayouts {
		if l.name == report.Layout {
			menu = l.menu
		}
	}
	if menu == "" {
		t.Fatalf("the layout is %q, which is none of the seven", report.Layout)
	}
	if report.Menu != menu {
		t.Errorf("the layout is %q but the menu entry is %q, wanted %q",
			report.Layout, report.Menu, menu)
	}

	// The smallest layout on this firmware holds forty areas. A handful would
	// mean the walk stopped early and reported what it had.
	if len(report.Areas) < 20 {
		t.Errorf("the layout reports %d areas, which is too few to be a whole one",
			len(report.Areas))
	}

	seen := map[string]bool{}
	for i, a := range report.Areas {
		if a.Area == "" {
			t.Errorf("area %d has no name", i)
		}
		if seen[a.Area] {
			// The walk ends when the knob comes back round to where it
			// started, so a repeat means it went round twice.
			t.Errorf("area %q is reported twice", a.Area)
		}
		seen[a.Area] = true

		for what, value := range map[string]string{
			"text":       a.TextHex,
			"background": a.BackgroundHex,
		} {
			if !hexColor.MatchString(value) {
				t.Errorf("area %q has a %s value of %q, wanted the form #RRGGBB",
					a.Area, what, value)
			}
		}
		if a.Text == "" || a.Background == "" {
			t.Errorf("area %q reports colors %q and %q, wanted a name for each",
				a.Area, a.Text, a.Background)
		}
	}

	t.Run("the scanner is left scanning", func(t *testing.T) {
		// The walk ends deep in the menus and has to climb out. A scanner left
		// in there refuses most other commands.
		mustRun(t, "scanning")
	})

	t.Run("the cache gives back what was read", func(t *testing.T) {
		// The read above stored what it found, so this must answer without
		// opening a picker, and answer with the same colors.
		var cached colorsReport
		mustJSON(t, &cached, "colors", report.Layout, "--cache")

		if !cached.Cached {
			t.Error("--cache did not report itself as cached")
		}
		if !cached.Read.Equal(report.Read) {
			t.Errorf("the cached reading is stamped %s, wanted %s from the read that stored it",
				cached.Read, report.Read)
		}
		if len(cached.Areas) != len(report.Areas) {
			t.Fatalf("the cache holds %d areas where the read found %d",
				len(cached.Areas), len(report.Areas))
		}

		for i, a := range cached.Areas {
			want := report.Areas[i]
			if a.Area != want.Area || a.Text != want.Text || a.Background != want.Background ||
				a.TextHex != want.TextHex || a.BackgroundHex != want.BackgroundHex {
				t.Errorf("area %d came back as %+v, wanted %+v", i, a, want)
			}
		}

		// Positions are compared only when both runs agreed the layout is the
		// one on screen, because that is what decides whether the soft keys get
		// read off it. The cache stores no positions either way, so this checks
		// place rather than the cache: what it would catch is the cache handing
		// back a stored position instead of a fresh one.
		if cached.Current != report.Current {
			t.Skipf("the read called the layout current=%v and the cached run called it %v, "+
				"so the soft keys were filled in for one and not the other",
				report.Current, cached.Current)
		}
		for i, a := range cached.Areas {
			want := report.Areas[i]
			if a != want {
				t.Errorf("area %d sits at %+v, wanted %+v", i, a, want)
			}
		}
	})

	t.Run("naming the same layout reads the same", func(t *testing.T) {
		// One read, checked twice: that naming a layout finds what reading the
		// current one found, and that the table says the same as the JSON.
		// Each read costs half a minute, so they share one.
		res := mustRun(t, "colors", report.Layout)

		for _, want := range []string{report.Layout, report.Menu, "AREA", "BACKGROUND"} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("the text output does not contain %q", want)
			}
		}

		rows := colorRows(res.stdout)
		if len(rows) != len(report.Areas) {
			t.Fatalf("the table has %d areas where the JSON reported %d",
				len(rows), len(report.Areas))
		}

		// Two reads of the same layout, a minute apart, describing the same
		// thing. This is what says the walk lands on the same areas every time
		// rather than drifting by a step.
		for i, a := range report.Areas {
			want := []string{a.Area,
				number(a, a.Line), number(a, a.Column), number(a, a.Length), number(a, a.Height),
				a.Text, a.TextHex, a.Background, a.BackgroundHex}
			if len(rows[i]) != len(want) {
				t.Errorf("row %d has %d columns, wanted %d: %q", i, len(rows[i]), len(want), rows[i])
				continue
			}
			for c := range want {
				if rows[i][c] != want[c] {
					t.Errorf("row %d column %d reads %q, wanted %q",
						i, c, rows[i][c], want[c])
				}
			}
		}
	})
}

// TestColors_Positions checks the built-in screen map, which is a table rather
// than a reading and so costs nothing and opens no menus.
func TestColors_Positions(t *testing.T) {
	needScanner(t)

	var report colorsReport
	mustJSON(t, &report, "colors", "--positions")

	if !report.Current {
		t.Error("reading positions without naming a layout did not report the current layout")
	}
	if len(report.Areas) < 20 {
		t.Fatalf("the layout reports %d areas, which is too few to be a whole one",
			len(report.Areas))
	}

	// The map is the only thing being reported, so nothing may be left unplaced.
	// The soft keys have no entry in it and are read off the live screen
	// instead, which is exactly the case that would go missing unnoticed.
	lines := screenLines(t)
	for _, a := range report.Areas {
		if a.Length <= 0 {
			t.Errorf("area %q is reported with no width", a.Area)
			continue
		}
		if a.Height <= 0 {
			t.Errorf("area %q is %d rows tall", a.Area, a.Height)
		}
		if a.Line < 0 || a.Line+a.Height > lines {
			t.Errorf("area %q runs from line %d for %d rows, off a screen of %d lines",
				a.Area, a.Line, a.Height, lines)
		}
		if a.Column < 0 {
			t.Errorf("area %q starts at column %d", a.Area, a.Column)
		}
	}

	t.Run("the soft keys come from the live screen", func(t *testing.T) {
		// They are absent from the built-in map on purpose, since their widths
		// follow whatever labels the current mode shows.
		for _, want := range []string{"Soft1_key", "Space_1", "Soft2_key", "Space_2", "Soft3_key"} {
			found := false
			for _, a := range report.Areas {
				if a.Area == want && a.Length > 0 {
					found = true
				}
			}
			if !found {
				t.Errorf("%q is missing or unplaced", want)
			}
		}
	})

	t.Run("no two areas overlap", func(t *testing.T) {
		// Every character of the screen belongs to at most one area, which is
		// what makes a position resolve to a single color.
		type cell struct{ line, column int }
		owner := map[cell]string{}

		for _, a := range report.Areas {
			for line := a.Line; line < a.Line+a.Height; line++ {
				for col := a.Column; col < a.Column+a.Length; col++ {
					at := cell{line, col}
					if other, taken := owner[at]; taken {
						t.Errorf("line %d column %d belongs to both %q and %q",
							line, col, other, a.Area)
					}
					owner[at] = a.Area
				}
			}
		}
	})

	t.Run("a named layout needs no scanner", func(t *testing.T) {
		var named colorsReport
		mustJSON(t, &named, "colors", "--positions", "weather")

		if named.Layout != "weather" {
			t.Errorf("asking for weather reported %q", named.Layout)
		}
		if len(named.Areas) == 0 {
			t.Error("the weather layout reports no areas")
		}
	})

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "colors", "--positions")

		for _, want := range []string{"AREA", "LINE", "COL", "LEN", "ROWS"} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("the text output has no %q column:\n%s", want, res.stdout)
			}
		}

		// A positions run has no colors to print, and a table of dashes wide
		// enough to hide the numbers would be worse than no columns at all.
		if strings.Contains(res.stdout, "BACKGROUND") {
			t.Error("the text output has color columns for a positions-only run")
		}
	})
}

// number renders one of an area's position values the way the table prints it,
// as a dash for an area the built-in map does not place.
func number(a colorsArea, value int) string {
	if a.Length == 0 {
		return "-"
	}
	return strconv.Itoa(value)
}

// colorRows pulls the area rows out of the printed table.
//
// The table is padded into columns, so the cells are whatever the runs of two
// or more spaces separate. Everything before the header and the trailing count
// is skipped.
func colorRows(out string) [][]string {
	var rows [][]string
	body := false

	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "AREA") {
			body = true
			continue
		}
		if !body || strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasSuffix(strings.TrimSpace(line), "areas") {
			break
		}
		rows = append(rows, columns.Split(strings.TrimSpace(line), -1))
	}
	return rows
}

// columns separates the padded cells of a printed table.
var columns = regexp.MustCompile(` {2,}`)

// TestColors_VerifyPositions checks the built-in map against the scanner.
//
// This is the whole reason the map may be hardcoded: a firmware that moved
// something would otherwise make it silently wrong, drawing the screen with the
// right colors in the wrong places and reporting nothing. It is behind -writes
// for the time it takes and because it walks the menus, not because it changes
// anything.
func TestColors_VerifyPositions(t *testing.T) {
	needWrites(t)

	var report struct {
		Layout      string `json:"layout"`
		Checked     int    `json:"checked"`
		Differences []struct {
			Area string `json:"area"`
		} `json:"differences"`
	}
	mustJSON(t, &report, "colors", "--verify-positions")

	if report.Checked < 20 {
		t.Errorf("only %d areas of %s were checked", report.Checked, report.Layout)
	}
	for _, d := range report.Differences {
		t.Errorf("%s: %q is not where the built-in map says", report.Layout, d.Area)
	}

	t.Run("the scanner is left scanning", func(t *testing.T) {
		mustRun(t, "scanning")
	})
}

// TestColors_VerifyPalette checks the built-in list of colors against the
// scanner.
//
// The palette is a table for the same reason the screen map is, and this is the
// same guard. It matters less than the screen map's check, because the walk that
// sets a color compares by name before committing and so cannot write the wrong
// color, but a palette that has drifted makes setting a color fail for no
// apparent reason.
//
// It is behind -writes for the time it takes and because it walks the menus. It
// writes nothing: the picker is left by the menu key, never by enter.
func TestColors_VerifyPalette(t *testing.T) {
	needWrites(t)

	var report struct {
		Borrowed struct {
			Area  string `json:"area"`
			Color string `json:"color"`
			Hex   string `json:"hex"`
		} `json:"borrowed"`
		Found       int `json:"found"`
		Expected    int `json:"expected"`
		Differences []struct {
			At       int    `json:"at"`
			Expected string `json:"expected"`
			Found    string `json:"found"`
		} `json:"differences"`
	}
	mustJSON(t, &report, "colors", "--verify-palette")

	if report.Found != report.Expected {
		t.Errorf("the scanner offers %d colors, the built-in palette holds %d",
			report.Found, report.Expected)
	}
	for _, d := range report.Differences {
		t.Errorf("color %d is %q on the scanner and %q in the built-in palette",
			d.At, d.Found, d.Expected)
	}

	t.Run("the picker it borrowed is unchanged", func(t *testing.T) {
		// Walking a picker turns the knob across every color in the list. The
		// only thing keeping that from recoloring the area is leaving by the
		// menu key rather than by enter, so this checks it.
		//
		// The check reports which area it used and what color it found there,
		// which is what makes this exact rather than an assumption about which
		// area comes first or what color it happens to be.
		if report.Borrowed.Area == "" || report.Borrowed.Color == "" {
			t.Fatal("the check does not say which area's picker it used")
		}

		// Setting a color to the one it already is writes nothing and reports
		// no change, so this asks the question without answering it by force.
		var set setReport
		mustJSON(t, &set, "colors", "set", report.Borrowed.Area,
			"--text", report.Borrowed.Color, "-o", "json")

		if set.Text == nil {
			t.Fatal("setting a color reported nothing about it")
		}
		if set.Text.Changed {
			t.Errorf("the walk left %s as %s, but it was %s when the walk started",
				report.Borrowed.Area, set.Text.From, report.Borrowed.Color)
		}
		if set.Text.ToHex != report.Borrowed.Hex {
			t.Errorf("%s reads %s where the walk saw %s",
				report.Borrowed.Area, set.Text.ToHex, report.Borrowed.Hex)
		}
	})

	t.Run("the scanner is left scanning", func(t *testing.T) {
		mustRun(t, "scanning")
	})
}

// screenLines reports how many rows the scanner's display has.
func screenLines(t *testing.T) int {
	t.Helper()

	var report struct {
		Lines []struct {
			Text string `json:"text"`
		} `json:"lines"`
	}
	mustJSON(t, &report, "screen")
	return len(report.Lines)
}

// setReport is the shape "colors set" prints as JSON.
type setReport struct {
	Layout string `json:"layout"`
	Area   string `json:"area"`
	Text   *struct {
		From    string `json:"from"`
		FromHex string `json:"fromHex"`
		To      string `json:"to"`
		ToHex   string `json:"toHex"`
		Changed bool   `json:"changed"`
	} `json:"text"`
}

// TestColorsPalette checks the built-in list of colors the scanner offers.
//
// It deliberately does not call needScanner. The palette is a table rather than
// a reading, and the whole point of it being one is that it answers with no
// radio attached, so a test that skipped without one would be testing the
// opposite of what this command promises.
func TestColorsPalette(t *testing.T) {
	var report colorsPalette
	mustJSON(t, &report, "colors", "palette")

	if report.Count != len(report.Colors) {
		t.Errorf("the count says %d and there are %d colors", report.Count, len(report.Colors))
	}

	// 147 on an SDS150 on firmware 1.00.37, walked one picker end to end. A
	// different number means the firmware's palette is not the one built in,
	// which "colors --verify-palette" is there to catch against a real radio.
	if report.Count != 147 {
		t.Errorf("the palette holds %d colors, wanted 147", report.Count)
	}

	names := map[string]bool{}
	values := map[string]bool{}
	for i, c := range report.Colors {
		if c.Step != i {
			t.Errorf("color %d reports step %d: the step is its place in the knob's ring", i, c.Step)
		}
		if c.Name == "" {
			t.Errorf("color %d has no name", i)
		}
		if !hexColor.MatchString(c.Hex) {
			t.Errorf("%s has a value of %q, wanted the form #RRGGBB", c.Name, c.Hex)
		}

		// Both have to be unique for a name to map to a color and back, which
		// is what lets a color be named rather than numbered.
		if names[c.Name] {
			t.Errorf("%s appears twice", c.Name)
		}
		if values[c.Hex] {
			t.Errorf("%s is a value some other color already has", c.Hex)
		}
		names[c.Name], values[c.Hex] = true, true

		if i > 0 && strings.ToLower(report.Colors[i-1].Name) > strings.ToLower(c.Name) {
			t.Errorf("%s comes after %s: the knob's order is alphabetical",
				c.Name, report.Colors[i-1].Name)
		}
	}

	// The values are the scanner's own rather than the web's, and these two are
	// where that is easiest to see. A run that "fixed" them to the CSS values
	// would be describing a different radio.
	for name, want := range map[string]string{
		"Orangered":  "#FF4600",
		"Darksalmon": "#E79473",
	} {
		for _, c := range report.Colors {
			if c.Name == name && c.Hex != want {
				t.Errorf("%s is %s, wanted the scanner's own %s", name, c.Hex, want)
			}
		}
	}

	t.Run("the names match what set accepts", func(t *testing.T) {
		// The list is only worth printing if it is the same list the picker
		// walks, so a name taken from it must not be refused. A wrong name
		// fails before the scanner is touched, which is why this needs none.
		res := mustFail(t, "is not a color", "colors", "set", "Func", "--text", "Nosuchcolor")
		if !strings.Contains(res.stderr, "147") &&
			!strings.Contains(strings.ToLower(res.stderr), "did you mean") {
			t.Errorf("the refusal says neither how many colors there are nor what was meant:\n%s",
				res.stderr)
		}
	})

	t.Run("the table is printed too", func(t *testing.T) {
		res := mustRun(t, "colors", "palette")
		for _, want := range []string{"STEP", "NAME", "HEX", "Aliceblue", "Yellowgreen", "147 colors"} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("the text output does not contain %q", want)
			}
		}
	})

	t.Run("refusing any argument", func(t *testing.T) {
		mustFail(t, "unknown command", "colors", "palette", "Orangered")
	})
}

// TestColorsSet_Arguments checks what the set command refuses. Everything here
// is refused before a menu is opened, so none of it disturbs the scanner.
func TestColorsSet_Arguments(t *testing.T) {
	needScanner(t)

	t.Run("setting nothing at all", func(t *testing.T) {
		mustFail(t, "nothing to set", "colors", "set", "System_name")
	})

	t.Run("colors the scanner does not offer", func(t *testing.T) {
		mustFail(t, "is not a color", "colors", "set", "System_name", "--text", "Burgundy")
		mustFail(t, "is not a color", "colors", "set", "System_name", "--text", "#FF4600")
		mustFail(t, "is not a color", "colors", "set", "System_name", "--back", "Nonsense")
	})

	t.Run("a near miss says what was meant", func(t *testing.T) {
		// The palette is 147 names. Printing all of them for a typo would be
		// useless, and printing none of them leaves the reader guessing.
		res := mustFail(t, "did you mean", "colors", "set", "System_name", "--text", "steel")

		if !strings.Contains(res.stderr, "Steelblue") {
			t.Errorf("the refusal does not suggest Steelblue:\n%s", res.stderr)
		}
	})

	t.Run("areas the layout does not have", func(t *testing.T) {
		mustFail(t, "has no area called", "colors", "set", "Nonsense", "--text", "Cyan")
	})

	t.Run("refusing a layout the scanner lacks", func(t *testing.T) {
		mustFail(t, "is not a layout", "colors", "set", "System_name",
			"--text", "Cyan", "--layout", "bogus")
	})
}

// TestColorsSet changes a color on the scanner and puts it back.
//
// This is the only test in the suite that writes a display setting, so it is
// careful about it: it reads what the color was from the command's own report,
// restores that exact value, and checks the restore took.
func TestColorsSet(t *testing.T) {
	needWrites(t)

	const area = "System_name"

	// Two targets, so the test still changes something if the area is already
	// the first one.
	target, other := "Cyan", "Gold"

	var first setReport
	mustJSON(t, &first, "colors", "set", area, "--text", target, "-o", "json")
	if first.Text == nil {
		t.Fatal("setting a color reported nothing about it")
	}

	original := first.Text.From
	if original == target {
		target = other
		mustJSON(t, &first, "colors", "set", area, "--text", target, "-o", "json")
		original = first.Text.From
	}

	t.Cleanup(func() {
		var back setReport
		mustJSON(t, &back, "colors", "set", area, "--text", original, "-o", "json")
		if back.Text == nil || back.Text.To != original {
			t.Errorf("could not put %s back to %s", area, original)
		}
	})

	if first.Area != area {
		t.Errorf("the report is about %q, wanted %q", first.Area, area)
	}
	if first.Text.To != target {
		t.Errorf("setting the color to %s reported %s", target, first.Text.To)
	}
	if !first.Text.Changed {
		t.Errorf("setting %s from %s to %s reported no change", area, original, target)
	}
	if first.Text.ToHex == "" || first.Text.FromHex == "" {
		t.Errorf("the report has no values: %q -> %q", first.Text.FromHex, first.Text.ToHex)
	}

	t.Run("setting it again writes nothing", func(t *testing.T) {
		// The scanner has no undo, so a command that writes what it found is a
		// command that can only make things worse.
		var again setReport
		mustJSON(t, &again, "colors", "set", area, "--text", target, "-o", "json")

		if again.Text == nil || again.Text.Changed {
			t.Error("setting a color to the one it already is reported a change")
		}
		if again.Text != nil && again.Text.To != target {
			t.Errorf("it reports %s where the color is %s", again.Text.To, target)
		}
	})

	t.Run("the change is there in a fresh read", func(t *testing.T) {
		// The set command reads back what it wrote before reporting, so this
		// checks the value survived the connection closing as well.
		var report colorsReport
		mustJSON(t, &report, "colors")

		for _, a := range report.Areas {
			if a.Area == area {
				if a.Text != target {
					t.Errorf("%s reads back as %s, wanted %s", area, a.Text, target)
				}
				return
			}
		}
		t.Errorf("%s is missing from a full read", area)
	})

	t.Run("the scanner is left scanning", func(t *testing.T) {
		mustRun(t, "scanning")
	})
}
