// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package device

import "testing"

// bar is a full width run of attribute characters, which is how the scanner
// sends a line it has drawn an underline across.
const bar = "______________________________"

// TestParseDisplayBlankLineWithAttributes is the regression for a defect that
// broke two commands and left a third working, which is what made it puzzling.
//
// The scanner draws an underline the full width of the panel whatever the text
// stops at, so a blank line drawn that way arrives as no text at all and thirty
// attribute characters. The parser used to accept a field as the attributes
// only when it was empty or exactly as long as the text, so it took that bar
// for more text, glued it on, and went looking for the attributes in the status
// fields that follow the screen. Display itself came out close enough to look
// right, which is why "screen" kept working, but "display" and "status" failed
// on the count of what was left over.
//
// Seen on an SDS150 on firmware 1.00.37, on the simple conventional layout.
func TestParseDisplayBlankLineWithAttributes(t *testing.T) {
	status := []string{"1", "0", "4", "", "", "", "", "", "", "", "0", ""}
	parts := append([]string{
		"010",       // three lines, the middle one in the large font
		"first", "", // drawn normally, so no attributes at all
		"BIG", "", // the large line
		"", bar, // nothing drawn, underlined all the way across
	}, status...)

	d, rest, err := parseDisplay("GST", parts)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if len(d.Lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(d.Lines))
	}
	if d.Lines[2].Text != "" {
		t.Errorf("line 2 reads %q, want no text: the bar is not text",
			d.Lines[2].Text)
	}
	if d.Lines[2].Attributes != bar {
		t.Errorf("line 2 has attributes %q, want the underline", d.Lines[2].Attributes)
	}
	if !d.Lines[2].Heading() {
		t.Error("line 2 does not report itself underlined")
	}

	// The count is the half that failed. Everything after the line pairs
	// belongs to the caller's command, and eating into it is what produced
	// "response to GST has 4 status fields, want at least 12".
	if len(rest) != len(status) {
		t.Fatalf("got %d status fields, want %d: the parser ate into them",
			len(rest), len(status))
	}
	for i := range status {
		if rest[i] != status[i] {
			t.Errorf("status field %d is %q, want %q", i, rest[i], status[i])
		}
	}
}

// TestParseDisplayRejoinsSplitText covers the case the fix must not break: an
// unescaped comma in the text splits one field into two, and the pieces have to
// be glued back together rather than the second being taken for the attributes.
//
// The scanner escapes some commas as tabs and not others, so both paths are
// live. A place name is the usual way to meet this one.
func TestParseDisplayRejoinsSplitText(t *testing.T) {
	for _, tt := range []struct {
		name  string
		parts []string
		want  Line
	}{
		{
			// The split kind: a real comma, arriving as two fields.
			name:  "an unescaped comma",
			parts: []string{"0", "GREENDALE", " ST 00000", ""},
			want:  Line{Text: "GREENDALE, ST 00000"},
		},
		{
			// The escaped kind, which never splits and only needs putting back.
			name:  "a comma sent as a tab",
			parts: []string{"0", "GREENDALE\t ST 00000", ""},
			want:  Line{Text: "GREENDALE, ST 00000"},
		},
		{
			// Text and attributes of the same length, the ordinary case.
			name:  "attributes the width of the text",
			parts: []string{"0", "SYSTEM", "**    "},
			want:  Line{Text: "SYSTEM", Attributes: "**"},
		},
		{
			// A split whose second piece is as long as the first. The old rule
			// would have called it the attributes on length alone; it holds
			// letters, so it is text.
			name:  "a split into equal halves",
			parts: []string{"0", "ABCDE", "FGHIJ", ""},
			want:  Line{Text: "ABCDE,FGHIJ"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, rest, err := parseDisplay("STS", tt.parts)
			if err != nil {
				t.Fatalf("parsing: %v", err)
			}
			if len(d.Lines) != 1 {
				t.Fatalf("got %d lines, want 1", len(d.Lines))
			}
			if d.Lines[0] != tt.want {
				t.Errorf("got %+v, want %+v", d.Lines[0], tt.want)
			}
			if len(rest) != 0 {
				t.Errorf("got %d fields left over, want none: %q", len(rest), rest)
			}
		})
	}
}

// TestOnlyAttributes covers what separates a bar from a piece of text, which is
// the whole of what the long form of isAttributes rests on.
func TestOnlyAttributes(t *testing.T) {
	for field, want := range map[string]bool{
		"":                               true,
		"   ":                            true,
		"***":                            true,
		bar:                              true,
		"** __  ":                        true,
		" ST 00000":                      false,
		"Scanning...":                    false,
		"------------------------------": false, // the dashes the scanner draws are text
	} {
		if got := onlyAttributes(field); got != want {
			t.Errorf("onlyAttributes(%q) is %v, want %v", field, got, want)
		}
	}
}

// TestParseDisplayCountsLinesFromTheForm covers the form being the authority on
// how many lines follow, since that is what the walk over the pairs relies on.
func TestParseDisplayCountsLinesFromTheForm(t *testing.T) {
	parts := []string{"0110", "a", "", "b", "", "c", "", "d", "", "left", "over"}

	d, rest, err := parseDisplay("GST", parts)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(d.Lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(d.Lines))
	}
	if want := []bool{false, true, true, false}; len(d.LargeFont) != len(want) {
		t.Fatalf("got %d font flags, want %d", len(d.LargeFont), len(want))
	} else {
		for i := range want {
			if d.LargeFont[i] != want[i] {
				t.Errorf("line %d largeFont is %v, want %v", i, d.LargeFont[i], want[i])
			}
		}
	}
	if len(rest) != 2 || rest[0] != "left" || rest[1] != "over" {
		t.Errorf("left over %q, want [left over]", rest)
	}
}

// TestParseDisplayMissingForm covers the reply that carries no display form at
// all, which is the one shape the parser cannot work from: the form is what
// says how many line pairs follow, so without it there is nothing to count.
func TestParseDisplayMissingForm(t *testing.T) {
	_, _, err := parseDisplay("STS", nil)
	if err == nil {
		t.Fatal("a reply with no display form was accepted")
	}
	if got := err.Error(); got != `response to "STS" has no display form` {
		t.Errorf("got %q, want the missing form named", got)
	}
}

// TestLineHeading covers what marks a screen title, which is the underline the
// scanner draws across it.
func TestLineHeading(t *testing.T) {
	if !(Line{Text: "SYSTEM", Attributes: bar}).Heading() {
		t.Error("an underlined line does not report itself a heading")
	}
	if (Line{Text: "SYSTEM", Attributes: "****"}).Heading() {
		t.Error("a highlighted line reports itself a heading")
	}
	if (Line{Text: "SYSTEM"}).Heading() {
		t.Error("a plain line reports itself a heading")
	}
}

// TestLineSelected covers how the cursor is found: the scanner draws the
// highlighted entry in reverse video and marks it no other way.
func TestLineSelected(t *testing.T) {
	if !(Line{Text: "SYSTEM", Attributes: "**    "}).Selected() {
		t.Error("a highlighted line does not report itself selected")
	}
	if (Line{Text: "SYSTEM", Attributes: bar}).Selected() {
		t.Error("an underlined line reports itself selected")
	}
	if (Line{Text: "SYSTEM"}).Selected() {
		t.Error("a plain line reports itself selected")
	}
}

// TestDisplayString covers the screen read as a person would read it, which
// means the blank lines the scanner pads the bottom with are dropped and the
// ones between the text are not.
func TestDisplayString(t *testing.T) {
	d := Display{Lines: []Line{
		{Text: "SCAN"},
		{Text: ""},
		{Text: "GREENDALE"},
		{Text: "   "},
		{Text: ""},
	}}
	if got := d.String(); got != "SCAN\n\nGREENDALE" {
		t.Errorf("got %q, want the trailing blanks dropped and the middle one kept", got)
	}

	if got := (Display{}).String(); got != "" {
		t.Errorf("got %q, want an empty screen to read as nothing", got)
	}
	if got := (Display{Lines: []Line{{Text: "  "}}}).String(); got != "" {
		t.Errorf("got %q, want a screen of blanks to read as nothing", got)
	}
}

// TestIsAttributes covers the short form of the test rejoin rests on, which is
// that an empty field is how the scanner says a line is drawn normally, and
// that a field of nothing but spaces is whichever the caller asks for.
func TestIsAttributes(t *testing.T) {
	for field, want := range map[string]bool{
		"":            true,
		"****":        true,
		bar:           true,
		"GREENDALE":   false,
		" ST 00000":   false,
		"Scanning...": false,
		"   ":         false, // padding, until the fallback pass says otherwise
	} {
		if got := isAttributes(field, false); got != want {
			t.Errorf("isAttributes(%q, false) is %v, want %v", field, got, want)
		}
	}

	if !isAttributes("   ", true) {
		t.Error("the fallback pass refused a field of nothing but spaces")
	}
}

// TestOnlyNormal covers the one attribute string that says nothing, which is
// what separates real attributes from a run of the text's own padding.
func TestOnlyNormal(t *testing.T) {
	for field, want := range map[string]bool{
		"":          false,
		" ":         true,
		"   ":       true,
		"** ":       false,
		bar:         false,
		"GREENDALE": false,
	} {
		if got := onlyNormal(field); got != want {
			t.Errorf("onlyNormal(%q) is %v, want %v", field, got, want)
		}
	}
}

// TestParseDisplayNameEndingInAComma is the regression for the third and last
// way the text/attribute boundary can be read wrong.
//
// AttrNormal is the space character, so an entry someone called "GREENDALE, "
// arrives as the name, then the padding that follows the comma, and that
// padding is a perfectly good attribute string. Reading it as one shifts every
// field after it by one and produces a wrong screen without saying so.
func TestParseDisplayNameEndingInAComma(t *testing.T) {
	d, rest, err := parseDisplay("STS", []string{"0", "GREENDALE", "   ", ""})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(d.Lines))
	}
	if want := (Line{Text: "GREENDALE,"}); d.Lines[0] != want {
		t.Errorf("got %+v, want %+v: the padding is the text's, not the line's attributes",
			d.Lines[0], want)
	}
	if len(rest) != 0 {
		t.Errorf("got %d fields left over, want none: %q", len(rest), rest)
	}
}

// TestParseDisplayFallsBackToBlankAttributes covers the other reading of the
// same ambiguity, which is why it is tried second rather than not at all.
//
// A scanner that padded an all-normal line's attributes out rather than sending
// the empty field it is observed to send would have that padding taken for
// text, and the line would swallow the one after it. That comes up a pair
// short, which is visible, so the parse is simply run again the other way.
func TestParseDisplayFallsBackToBlankAttributes(t *testing.T) {
	d, rest, err := parseDisplay("STS", []string{"0", "AB", "   "})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(d.Lines))
	}
	if want := (Line{Text: "AB"}); d.Lines[0] != want {
		t.Errorf("got %+v, want %+v", d.Lines[0], want)
	}
	if len(rest) != 0 {
		t.Errorf("got %d fields left over, want none: %q", len(rest), rest)
	}
}

// TestParseDisplayShortOfLines covers the reply that runs out of pairs whichever
// way it is read, which is the only case left once both passes have been tried.
func TestParseDisplayShortOfLines(t *testing.T) {
	_, _, err := parseDisplay("STS", []string{"00", "a", ""})
	if err == nil {
		t.Fatal("a reply carrying one pair for two lines was accepted")
	}
	if got := err.Error(); got != `response to "STS" has 2 display lines, want at least 4` {
		t.Errorf("got %q, want the shortfall named", got)
	}
}

// TestUnescape covers the commas the scanner does escape, which arrive as tabs
// and have to be put back before the text is shown.
func TestUnescape(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "",
		"GREENDALE":            "GREENDALE",
		"GREENDALE\t ST 00000": "GREENDALE, ST 00000",
		"\t\t":                 ",,",
	} {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) is %q, want %q", in, got, want)
		}
	}
}

// TestDisplayUnitID tests the Display.UnitID method with 100% coverage.
//
// The screen is where the transmitting radio's identifier lives on a
// conventional P25 channel. The protocol reply does not carry it there, which
// was measured across seven complete transmissions: every reading said "UID
// None" while the screen beside it named a radio.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - OnTheLine: the identifier is read out from beside its label
//   - NothingDecoded: the dashes the screen shows instead are not an identifier
//   - Absent: a display without the field at all reports nothing
//   - Trunked: a five digit identifier is read whole
//   - EndOfLine: the field with nothing after it is still read
//   - Empty: a screen with no lines is not a crash
func TestDisplayUnitID(t *testing.T) {
	screen := func(lines ...string) Display {
		d := Display{}
		for _, text := range lines {
			d.Lines = append(d.Lines, Line{Text: text})
		}
		return d
	}

	// Verify the ordinary case, which is the field sharing its line with the
	// signal reading.
	t.Run("OnTheLine", func(t *testing.T) {
		d := screen("WACN: ---       Batt:4.17V", "UID:640006      RSSI: -98dBm")

		if got := d.UnitID(); got != "640006" {
			t.Errorf("UnitID gave %q, want the identifier beside the label", got)
		}
	})

	// Verify that the screen's own way of saying nothing has been decoded is
	// not read as an identifier, which would put dashes in every recording.
	t.Run("NothingDecoded", func(t *testing.T) {
		d := screen("UID: ---        RSSI: -99dBm")

		if got := d.UnitID(); got != "" {
			t.Errorf("UnitID gave %q, want nothing", got)
		}
	})

	// Verify that a display mode without the field reports nothing rather than
	// guessing, since the field is only drawn on the detailed screen.
	t.Run("Absent", func(t *testing.T) {
		d := screen("PUBLIC SAFETY", "POLICE DEPARTMENT", "DISPATCH")

		if got := d.UnitID(); got != "" {
			t.Errorf("UnitID gave %q, want nothing", got)
		}
	})

	// Verify a longer identifier is not truncated, since these run to six
	// digits on the systems measured.
	t.Run("Trunked", func(t *testing.T) {
		d := screen("UID:100045      RSSI: -87dBm")

		if got := d.UnitID(); got != "100045" {
			t.Errorf("UnitID gave %q, want the whole identifier", got)
		}
	})

	// Verify the field with nothing following it, which is what a narrower
	// screen or a different layout would produce.
	t.Run("EndOfLine", func(t *testing.T) {
		d := screen("UID:202")

		if got := d.UnitID(); got != "202" {
			t.Errorf("UnitID gave %q, want the identifier", got)
		}
	})

	// Verify that a screen with nothing on it answers rather than panicking,
	// since a scanner in a menu can send exactly that.
	t.Run("Empty", func(t *testing.T) {
		if got := (Display{}).UnitID(); got != "" {
			t.Errorf("UnitID gave %q, want nothing", got)
		}
	})
}
