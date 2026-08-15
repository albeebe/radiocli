// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package menus

import (
	"time"

	"github.com/albeebe/radiocli/internal/device"
)

// deletePrompt is what the scanner asks before deleting anything. It is the
// same screen at every level of the memory, worded the same way, and it labels
// its own keys: Yes="E" / No=".".
const deletePrompt = "Confirm Delete?"

// leaveAttempts bounds the escape. A menu opened several levels deep needs a
// press per level, and a handful covers the deepest this scanner goes.
const leaveAttempts = 8

// maxSteps bounds a walk so a menu that never shows the wanted entry stops
// rather than turning the knob for ever. No menu on this scanner comes close.
const maxSteps = 64

// quickSearch is the screen the scanner reports while holding one frequency,
// as "tune" leaves it.
const quickSearch = "quick_search"

// byName maps the names this tool accepts to the menus the scanner knows.
var byName = map[string]device.MenuID{
	"top":              device.MenuTop,
	"favorites":        device.MenuMonitorList,
	"system":           device.MenuSystem,
	"department":       device.MenuDepartment,
	"site":             device.MenuSite,
	"channel":          device.MenuChannel,
	"search-range":     device.MenuSearchRange,
	"search-options":   device.MenuSearchOptions,
	"close-call":       device.MenuCloseCall,
	"close-call-band":  device.MenuCloseCallBand,
	"weather":          device.MenuWeather,
	"tone-out":         device.MenuToneOutChannel,
	"settings":         device.MenuSettings,
	"broadcast-screen": device.MenuBroadcastScreen,
}

// escapes are the keys that leave the menus, tried in turn.
//
// Neither works everywhere, and which one fails is not predictable from the
// screen: AVOID leaves a text entry screen that SOFT1 will not, and SOFT1
// leaves a system's menu that AVOID will not. Alternating covers both, at the
// cost of a wasted press on whichever screen wants the other one.
var escapes = []device.Key{device.KeyAvoid, device.KeySoft1}

// AwakenBudget is how long Awaken waits for the scanner to start answering
// again, and AwakenGap is how long it leaves between asking.
//
// A budget in time rather than in attempts, because what it is waiting out is
// the scanner rebuilding from its database, and that takes as long as it takes
// however many questions get asked in the meantime. Counting attempts only
// measured time while every attempt cost the protocol's full timeout: a scanner
// answering promptly with something malformed used the whole allowance in
// milliseconds and reported a rebuild that had never been given a chance to
// finish. The gap is what keeps a fast failure from spinning.
//
// They are exported vars, which nothing in the tool writes to, so that the
// tests of the commands that wait out a rebuild can shorten the wait. Those
// tests drive a scanner that fails instantly and every one of them would
// otherwise sit through the full budget; a package cannot reach another
// package's unexported variables, so these are the seam.
var (
	AwakenBudget = 40 * time.Second
	AwakenGap    = 250 * time.Millisecond
)

// readyPolls and readyGap bound the wait for the scanner to finish what it is
// doing. Saving a name takes around three quarters of a second on an SDS150,
// and the bound is generous enough that reaching it means something is wrong
// rather than merely slow.
var (
	readyPolls = 60
	readyGap   = 50 * time.Millisecond
)

// resumePolls and resumeGap bound the wait for the scanner to redraw after
// being told to go back to scanning.
var (
	resumePolls = 40
	resumeGap   = 50 * time.Millisecond
)

// Item is one entry of a menu.
type Item struct {
	// Name is the entry as the scanner spells it.
	Name string `json:"name"`

	// Index is the scanner's own index for the entry, which is not its
	// position: the indexes have gaps.
	Index string `json:"index"`

	// Highlighted marks the entry the scanner has selected, which is the one a
	// key press acts on.
	Highlighted bool `json:"highlighted"`
}

// Report is a menu as this tool renders it.
type Report struct {
	// Title is the menu's own heading, as the scanner reports it.
	Title string `json:"title"`

	// Kind is the scanner's name for what sort of menu this is.
	Kind string `json:"kind"`

	// Items are the menu's entries, in the order the scanner lists them. It can
	// be empty, because an input screen carries no entries at all.
	Items []Item `json:"items"`
}

// Row is a reading of the highlighted display row, for callers that have to
// compare it against a name themselves rather than through StepTo.
type Row struct {
	// Text is the row as it means to be read, with the padding and the
	// scanner's own markings taken off.
	Text string

	// Cut reports whether the scanner shortened the row to fit the screen, and
	// so whether it may be compared as the start of a longer name. Comparing a
	// row that was not cut as a prefix is how a walk lands on the wrong entry.
	Cut bool
}

// row is one reading of the highlighted display row.
type row struct {
	// text is the row as it means to be read, with the padding and the
	// scanner's own markings taken off.
	text string

	// cut reports whether the scanner shortened the name to fit the screen.
	// It decides whether the row may be compared as a prefix, so it has to be
	// noticed on the raw row, before clean strips the marks that say so.
	cut bool
}
