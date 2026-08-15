// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package colors

import (
	"encoding/json"
	"os"
	"regexp"
	"time"
)

// cacheVersion is the format of the file on disk. A file written by an older
// version of the tool is discarded rather than migrated: it holds nothing that
// cannot be read again from the scanner.
const cacheVersion = 1

// The menu entries the layouts live behind, from the top menu down.
const (
	displayOptions = "Display Options"
	customize      = "Customize"
	scanDisplay    = "Set Scan Display Mode"

	textColor = "Set Text Color"
	backColor = "Set Back Color"

	simpleEntry = "Simple Mode"
	detailEntry = "Detail Mode"
)

// maxAreas is where the walk gives up, well above the fifty of the largest
// layout, so a scanner that never comes back round stops rather than spins.
const maxAreas = 200

// maxPaletteWalk is where the walk gives up, well above the palette's size, so
// a picker that never comes round stops rather than spins.
const maxPaletteWalk = 400

// pickerRounds is how many times to step and check before giving up. Each round
// gets the walk closer, so more than a couple means the scanner is not moving
// the way the palette says it does.
const pickerRounds = 5

// The menu this command drives, under Customize. Restore is where a layout is
// put back to stock, and the entries in it are the layout names without the
// "Set " that the editors alongside them carry.
const (
	restoreEntry = "Restore Settings"
	allScreens   = "All Screens"
)

// How many rows the scanner's screen has in each of the two scan display
// modes. Measured on an SDS150 on firmware 1.00.37 by setting each mode and
// counting the rows it reported while scanning.
const (
	simpleLines = 14
	detailLines = 17
)

// layouts are the seven, in the order the Customize menu lists them.
//
// The screen names are the scanner's own, as it reports them in its status. A
// mode with no layout of its own, such as direct entry, is absent on purpose:
// this command would rather say it cannot tell than name the wrong one.
var layouts = []layout{
	{"simple-conventional", "Set Simple Conventional", []string{"conventional_scan"}, simpleEntry, simpleLines},
	{"simple-trunk", "Set Simple Trunk", []string{"trunk_scan"}, simpleEntry, simpleLines},
	{"detail-conventional", "Set Detail Conventional", []string{"conventional_scan"}, detailEntry, detailLines},
	{"detail-trunk", "Set Detail Trunk", []string{"trunk_scan"}, detailEntry, detailLines},
	{"search", "Set Search/CC Mode", []string{
		"custom_search", "quick_search", "close_call", "cc_searching",
		"custom_with_scan", "cchits_with_scan",
	}, "", 0},
	{"weather", "Set Weather Mode", []string{"wx_alert"}, "", 0},
	{"tone-out", "Set Tone Out Mode", []string{"tone_out"}, "", 0},
}

// createTemp opens the file the cache is written to before it is renamed into
// place. It is a var so tests can drive the failures a real temporary file in a
// writable directory will not produce.
var createTemp = os.CreateTemp

// marshalJSON encodes the settings for writing. It is a var so tests can
// drive the failure the real encoder will not produce for these types.
var marshalJSON = json.MarshalIndent

// menuScreens are the views that mean the scanner is somewhere no layout
// draws, and will stay there until someone takes it out.
var menuScreens = []string{"menu_selection", "menu_input", "direct_entry"}

// rgb matches the line the color picker shows the value on.
var rgb = regexp.MustCompile(`RGB\s*=\s*([0-9A-Fa-f]{6})h`)

// The wait for a scanning screen to settle. A scanner that has just been sent
// somewhere reports a plain screen for a moment before it reports what it is
// doing, which is long enough to be read by a command that runs straight after
// one that moved it.
var (
	settlePolls = 12
	settleGap   = 250 * time.Millisecond
)

// area is one element of a layout and the two colors it is drawn in.
type area struct {
	// Name is the scanner's name for the area, such as "System_name".
	Name string `json:"area"`

	// Text and Background are the color names the scanner shows, which are the
	// CSS color names.
	Text       string `json:"text"`
	Background string `json:"background"`

	// TextHex and BackgroundHex are those colors as 24 bit values, in the form
	// "#RRGGBB". The scanner reports them alongside the names.
	TextHex       string `json:"textHex"`
	BackgroundHex string `json:"backgroundHex"`

	// Line, Column and Length are where the area sits on the screen, counting
	// from zero at the top left. They come from the built-in map rather than
	// from the scanner: nothing in the menus moves an area, so they cannot
	// change the way the colors can. See positions.go, and --verify-positions
	// for checking that map against the radio in hand.
	//
	// Height is how many rows tall the area is, which is one for all but nine
	// areas across the seven layouts: the scanner draws some names in a large
	// font two rows tall, and gives three layouts a detail area four rows tall.
	//
	// Length is zero for an area the map does not place. The soft keys are what
	// that is for: they are placed from the live screen when the layout being
	// reported is the one in use, and left unplaced otherwise.
	Line   int `json:"line"`
	Column int `json:"column"`
	Length int `json:"length"`
	Height int `json:"height"`
}

// borrowed is the area whose picker the walk used, and the color it found
// there.
type borrowed struct {
	// Area is the scanner's name for the area whose picker was opened.
	Area string `json:"area"`

	// Color and Hex are what that area's text color was when the walk started,
	// and what it should still be now the walk has left without choosing
	// anything.
	Color string `json:"color"`
	Hex   string `json:"hex"`
}

// cache is the whole of the file on disk.
type cache struct {
	// Version is cacheVersion at the time the file was written.
	Version int `json:"version"`

	// Scanners holds one entry per scanner the tool has read colors from, keyed
	// by scannerKey. Two scanners on one computer keep separate colors, because
	// they are separate settings.
	Scanners map[string]cachedScanner `json:"scanners"`
}

// cachedArea is one area's two colors.
//
// Where the area sits is deliberately not stored. Positions come from the
// built-in map and the live screen, both of which are free, so keeping a copy
// here would only create something that can go stale for no gain.
type cachedArea struct {
	// Name is the scanner's name for the area, such as "System_name".
	Name string `json:"area"`

	// Text and Background are the color names the scanner showed for it.
	Text       string `json:"text"`
	Background string `json:"background"`

	// TextHex and BackgroundHex are those colors as the scanner reported them,
	// in the form "#RRGGBB".
	TextHex       string `json:"textHex"`
	BackgroundHex string `json:"backgroundHex"`
}

// cachedLayout is one layout's colors as they were last read.
type cachedLayout struct {
	// Read is when the walk that produced these colors finished.
	Read time.Time `json:"read"`

	// Areas are the layout's areas in the order the scanner listed them.
	Areas []cachedArea `json:"areas"`
}

// cachedScanner is everything stored for one scanner.
type cachedScanner struct {
	// Model and Port are what the scanner was when this was last written. The
	// tool never reads them: they are there so that somebody who opens the file
	// can see whose colors these are, since the key on its own is opaque.
	Model string `json:"model,omitempty"`
	Port  string `json:"port,omitempty"`

	// Layouts are the readings, keyed by layout name.
	Layouts map[string]cachedLayout `json:"layouts"`
}

// change is one color before and after being set.
type change struct {
	// From and FromHex are the color the picker was showing before anything
	// was pressed, which is what the area was drawn in until now.
	From    string `json:"from"`
	FromHex string `json:"fromHex"`

	// To and ToHex are the color it was set to, read back off the picker
	// rather than assumed from what was asked for.
	To    string `json:"to"`
	ToHex string `json:"toHex"`

	// Changed is false when the color was already the one asked for, in which
	// case nothing was written.
	Changed bool `json:"changed"`
}

// difference is one area the scanner and the built-in map disagree about.
type difference struct {
	// Area is the scanner's name for the area the two disagree about.
	Area string `json:"area"`

	// Expected is what the built-in map holds. Absent means the map does not
	// place this area at all.
	Expected *position `json:"expected"`

	// Found is what the scanner draws. Absent means the scanner did not
	// highlight anything readable for it.
	Found *position `json:"found"`
}

// layout is one of the scanner's seven screen layouts.
type layout struct {
	// name is what this command calls the layout.
	name string

	// entry is the layout's entry on the Customize menu.
	entry string

	// screens are the values the scanner reports as its current view when it
	// is drawing with this layout. Empty for the two layouts that are chosen
	// by the scan display mode rather than by what the scanner is doing.
	screens []string

	// detail says which of the scan display modes this layout belongs to, for
	// the four that come in a simple and a detail version. Empty for the rest.
	detail string

	// lines is how many rows the scanner's screen has while drawing with this
	// layout, which is what separates the simple version from the detail one
	// without asking a menu. Zero for the layouts that have no twin and so
	// never need telling apart.
	lines int
}

// paletteColor is one color the scanner offers.
type paletteColor struct {
	// Name is the scanner's name for it, which is a CSS color name.
	Name string

	// Hex is the value the scanner reports for it, as "#RRGGBB".
	Hex string
}

// paletteDifference is one place the scanner and the built-in palette disagree.
type paletteDifference struct {
	// At is how far round the ring it was found, counting from where the walk
	// started.
	At int `json:"at"`

	// Expected and Found are the color the palette holds and the one the
	// scanner showed, as "Name #RRGGBB". Either is empty when there is nothing
	// to compare against.
	Expected string `json:"expected"`
	Found    string `json:"found"`
}

// paletteEntry is one color of the palette.
type paletteEntry struct {
	// Step is the color's place in the ring, counting from zero at the first.
	// It is also how many times the knob has to be turned right from that
	// first color to reach this one, which is the only sense in which the
	// scanner numbers its colors: its pickers show a name and never a number.
	Step int `json:"step"`

	// Name is the scanner's name for the color, which is a CSS color name.
	Name string `json:"name"`

	// Hex is the value the scanner reports for it, as "#RRGGBB". It is the
	// scanner's own value rather than what the name means on the web.
	Hex string `json:"hex"`
}

// paletteList is what the palette command renders. The name keeps it apart
// from paletteReport, which is what --verify-palette renders: one is the table,
// the other is that table checked against a radio.
type paletteList struct {
	// Count is how many colors the scanner offers, which is the length of the
	// ring the knob steps around.
	Count int `json:"count"`

	// Colors are all of them, in the knob's own order.
	Colors []paletteEntry `json:"colors"`
}

// paletteReport is what the palette check renders.
type paletteReport struct {
	// Borrowed says which area's picker was opened to do the walk, and what
	// that area's color was when it started.
	//
	// It is reported because the walk turns the knob across every color in the
	// list, and the only thing keeping that from recoloring the area is leaving
	// by the menu key rather than by enter. Saying which area was touched and
	// what it should still be is what makes that checkable from outside.
	Borrowed borrowed `json:"borrowed"`

	// Found is how many colors the scanner offered before coming back round.
	Found int `json:"found"`

	// Expected is how many the built-in palette holds.
	Expected int `json:"expected"`

	// Differences is every place the scanner and the palette disagree. Empty
	// when the palette is right.
	Differences []paletteDifference `json:"differences"`
}

// position is where an area sits on the screen, counting from zero at the top
// left.
type position struct {
	// Line is the topmost row the area occupies.
	Line int

	// Column is the first character of the area.
	Column int

	// Length is how many characters wide the area is.
	Length int

	// Height is how many rows tall it is, which is one for all but nine areas.
	// The scanner draws some names in a large font that occupies two rows, and
	// gives the search, weather and tone out layouts a detail area four rows
	// tall. An area read as one row tall when it is two puts half of it in
	// whatever colors surround it.
	Height int
}

// report is the result this command renders.
type report struct {
	// Layout is the layout's name, such as "detail-conventional".
	Layout string `json:"layout"`

	// Menu is the layout's entry on the scanner's own Customize menu.
	Menu string `json:"menu"`

	// Current reports whether this is the layout the scanner is drawing with
	// right now. It is false when a layout was named rather than found.
	Current bool `json:"current"`

	// Cached reports whether the colors came from the cache rather than from
	// the scanner's menus. The positions never do; see place.
	Cached bool `json:"cached"`

	// Read is when the colors were read off the scanner, which is the moment
	// this run finished walking the menus, or the moment the cached run did.
	// Absent from a reading that has no colors in it, such as --positions.
	Read time.Time `json:"read,omitzero"`

	// Areas are the layout's areas, in the order the scanner lists them.
	Areas []area `json:"areas"`
}

// resetReport is what the reset command renders.
type resetReport struct {
	// Entry is the Restore menu entry that was chosen, such as "All Screens".
	Entry string `json:"entry"`

	// Layouts are the layouts the restore covered, by this command's names.
	// All seven for "All Screens".
	Layouts []string `json:"layouts"`

	// Reread is the layout whose new colors were read back afterwards, which
	// is the one the scanner is drawing with. Empty when the restore did not
	// touch what is on screen, and so left nothing worth reading.
	Reread string `json:"reread,omitempty"`
}

// setReport is what the set command renders.
type setReport struct {
	// Layout is the layout that was changed, by this command's name for it,
	// and Menu is its entry on the scanner's own Customize menu.
	Layout string `json:"layout"`
	Menu   string `json:"menu"`

	// Area is the area that was changed, as the scanner names it.
	Area string `json:"area"`

	// Text and Background are what each color was before and after, absent for
	// the one that was not asked about.
	Text       *change `json:"text,omitempty"`
	Background *change `json:"background,omitempty"`
}

// verifyReport is what the check renders.
type verifyReport struct {
	// Layout is the layout that was checked, by this command's name for it,
	// and Menu is its entry on the scanner's own Customize menu.
	Layout string `json:"layout"`
	Menu   string `json:"menu"`

	// Current reports whether this is the layout the scanner is drawing with
	// right now.
	Current bool `json:"current"`

	// Checked is how many areas the scanner was asked about.
	Checked int `json:"checked"`

	// Differences is every area the scanner and the map disagree about. Empty
	// when the map is right, which is the answer this hopes for.
	Differences []difference `json:"differences"`
}
