// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"strings"
)

// Attribute is how one character is drawn on the scanner's screen.
type Attribute byte

// Heading reports whether any character on the line is underlined. The scanner
// underlines screen titles.
//
// Returns:
//   - true if any character on the line is drawn with AttrUnderline
func (l Line) Heading() bool {
	return strings.ContainsRune(l.Attributes, rune(AttrUnderline))
}

// Selected reports whether any character on the line is in reverse video. The
// scanner marks the highlighted menu entry that way, so this is how to find
// which item the cursor is on.
//
// Returns:
//   - true if any character on the line is drawn with AttrReverse
func (l Line) Selected() bool {
	return strings.ContainsRune(l.Attributes, rune(AttrReverse))
}

// String renders the screen as the user sees it, one line per line, with
// trailing blank lines removed.
//
// Returns:
//   - the screen's text, one line per screen line, separated by newlines
func (d Display) String() string {
	text := make([]string, 0, len(d.Lines))
	for _, l := range d.Lines {
		text = append(text, l.Text)
	}

	for len(text) > 0 && strings.TrimSpace(text[len(text)-1]) == "" {
		text = text[:len(text)-1]
	}
	return strings.Join(text, "\n")
}

// UnitID reads the transmitting radio's identifier off the screen.
//
// It is here because the scanner does not always send it any other way. On a
// trunked call the identifier arrives in the protocol reply, on its own
// element. On a conventional P25 channel it does not: measured across seven
// complete transmissions on an SDS150, a hundred and six readings taken while
// the radio reported it was decoding P25 carried "UID None" every time, in
// every attribute the reply has for it, while the screen beside them read
// "UID:640006".
//
// So this is a screen scrape, with the honesty that implies. It reads what a
// person would read, and it can only see what the scanner has drawn: the field
// exists on the detailed scanning display and not on the others, so a scanner
// set to a different display mode reports nothing here and nothing is wrong.
//
// The line is found by its label rather than by its position, because the
// position is a property of one display mode on one model and the label is what
// the field is.
//
// Returns:
//   - the identifier alone, such as "640006", or empty when the screen does not
//     show one
func (d Display) UnitID() string {
	for _, l := range d.Lines {
		// The field shares its line with the signal reading, as
		// "UID:640006      RSSI: -98dBm", so what follows is cut at the first
		// space rather than taken to the end.
		_, after, found := strings.Cut(l.Text, "UID:")
		if !found {
			continue
		}

		id := strings.TrimSpace(after)
		if cut := strings.IndexFunc(id, func(r rune) bool { return r == ' ' || r == '\t' }); cut >= 0 {
			id = id[:cut]
		}

		// "UID: ---" is the screen's way of saying nothing has been decoded,
		// which arrives here as dashes once the label is cut away.
		if id == "" || strings.Trim(id, "-") == "" {
			continue
		}
		return id
	}
	return ""
}

// isAttributes reports whether a field could be the attribute string of a line
// rather than another piece of its text.
//
// What it is made of, not how long it is. Empty is how the scanner says the
// whole line is drawn normally; anything else has to be attribute characters
// and nothing but.
//
// Length used to be the test, and it was wrong in both directions. A field
// longer than the text was refused, which is what an underline is: the scanner
// draws it the full width of the panel whatever the text stops at, so a blank
// line drawn that way arrives with no text at all and thirty attribute
// characters. That took the parser off the end of the line pairs and into the
// status fields behind them, which is what broke "display" and "status" on the
// simple conventional layout while "screen" carried on working, since only the
// count of what was left over gave it away.
//
// A field the same length as the text was accepted, which is wrong whenever an
// unescaped comma has split the text into two halves that happen to match.
// "GREENDALE, ST 00000" is exactly that: nine characters either side of the
// comma, so the back half of a favorites list's name was read as a line of
// attributes. Both are the same mistake, which is measuring a field instead of
// reading it.
//
// What reading cannot settle on its own is a field of nothing but AttrNormal,
// because AttrNormal is the space character: "GREENDALE,   " splits into a name
// and a run of padding that is a perfectly good attribute string. blank is how
// the caller says which reading to take, and parseDisplay tries them in the
// order that cannot lose.
//
// Parameters:
//   - field: one field of the response, in the position a line's attributes
//     would occupy
//   - blank: whether a field of nothing but AttrNormal counts as attributes
//
// Returns:
//   - true if the field is empty, or holds nothing but attribute characters
//     and is not ruled out by blank
func isAttributes(field string, blank bool) bool {
	if field == "" {
		return true
	}
	if !onlyAttributes(field) {
		return false
	}
	return blank || !onlyNormal(field)
}

// onlyAttributes reports whether every character of a field is one the scanner
// draws with. Anything else means it is text.
//
// Parameters:
//   - field: the field to examine
//
// Returns:
//   - true if every character is AttrNormal, AttrReverse or AttrUnderline,
//     which an empty field satisfies as well
func onlyAttributes(field string) bool {
	for i := 0; i < len(field); i++ {
		switch Attribute(field[i]) {
		case AttrNormal, AttrReverse, AttrUnderline:
		default:
			return false
		}
	}
	return true
}

// onlyNormal reports whether a field is a non-empty run of AttrNormal and
// nothing else, which is the one attribute string that says nothing: a line
// drawn entirely normally arrives as an empty field, and parseDisplay trims
// trailing normals off the ones that are not empty.
//
// Parameters:
//   - field: the field to examine
//
// Returns:
//   - true if the field is not empty and every character is AttrNormal
func onlyNormal(field string) bool {
	if field == "" {
		return false
	}
	for i := 0; i < len(field); i++ {
		if Attribute(field[i]) != AttrNormal {
			return false
		}
	}
	return true
}

// parseDisplay reads the display form and the line pairs that follow it, and
// returns the screen along with the fields left over after it.
//
// The number of lines is not fixed: the form is one digit per line, so its
// length says how many [text, attributes] pairs follow. Everything after those
// pairs belongs to the caller's command.
//
// The pairs are rejoined twice when they have to be. A field of nothing but
// spaces sitting where attributes go is genuinely ambiguous, so the reading
// that treats it as text is tried first and the reading that treats it as
// attributes is the fallback. Order matters and this is the order that cannot
// lose: taking the padding for attributes shifts every later field by one and
// produces a wrong screen in silence, while taking real attributes for text
// swallows the next line and comes up a pair short, which is visible from here
// and undone by trying again.
//
// Parameters:
//   - command: the command being parsed, named in any error
//   - parts: the response's fields, starting at the display form
//
// Returns:
//   - the screen those fields describe
//   - the fields that follow the line pairs, for the caller to read
//   - error if the display form is missing, or if fewer line pairs arrived
//     than the form calls for
func parseDisplay(command string, parts []string) (Display, []string, error) {
	if len(parts) == 0 {
		return Display{}, nil, errMissingField(command, "display form")
	}

	form := parts[0]
	lines := len(form)

	rest := rejoin(parts[1:], lines, false)
	if len(rest) < lines*2 {
		rest = rejoin(parts[1:], lines, true)
	}
	if len(rest) < lines*2 {
		return Display{}, nil, errShortResponse(command, len(rest), lines*2, "display lines")
	}

	d := Display{
		Lines:     make([]Line, 0, lines),
		LargeFont: make([]bool, 0, lines),
	}
	for i := range lines {
		d.Lines = append(d.Lines, Line{
			Text:       strings.TrimRight(unescape(rest[i*2]), " "),
			Attributes: strings.TrimRight(unescape(rest[i*2+1]), " "),
		})
		d.LargeFont = append(d.LargeFont, form[i] == '1')
	}

	return d, rest[lines*2:], nil
}

// rejoin puts back together the fields an unescaped comma split apart.
//
// Each line arrives as a pair: the text, then its attributes. The attributes
// are made of attribute characters and nothing else, so a field in that
// position holding anything else is a piece of the text that a comma split off,
// and it is glued back on with the comma it was split on until the pair makes
// sense again. See isAttributes for why that is the test rather than length.
//
// Fields beyond the lines the screen needs are returned untouched: the values
// that follow are genuinely short and must not be joined to anything.
//
// Parameters:
//   - parts: the response's fields after the display form
//   - lines: how many lines the display form says the screen has
//   - blank: whether a field of nothing but spaces counts as attributes, which
//     parseDisplay decides by trying both
//
// Returns:
//   - the fields with each line's text back in one piece, followed by
//     everything that came after the lines, untouched
func rejoin(parts []string, lines int, blank bool) []string {
	out := make([]string, 0, len(parts))

	i := 0
	for line := 0; line < lines && i < len(parts); line++ {
		text := parts[i]
		i++

		// Join until the next field could be this line's attributes.
		for i < len(parts) && !isAttributes(parts[i], blank) {
			text += "," + parts[i]
			i++
		}
		if i >= len(parts) {
			out = append(out, text)
			break
		}

		out = append(out, text, parts[i])
		i++
	}

	return append(out, parts[i:]...)
}

// unescape restores commas the scanner did send as tabs.
//
// It escapes some screen text that way, but not all of it: a menu entry naming
// something the user called "GREENDALE, ST 00000" arrives with a real comma in
// it, which the response format cannot carry. rejoin puts those back together;
// this handles the ones that were escaped properly.
//
// Parameters:
//   - s: a field as the scanner sent it
//
// Returns:
//   - the field with every tab turned back into a comma
func unescape(s string) string {
	return strings.ReplaceAll(s, "\t", ",")
}
