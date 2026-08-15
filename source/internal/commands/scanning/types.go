// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package scanning

import (
	"time"

	"github.com/albeebe/radiocli/internal/device"
)

// Where each part of the held scanning screen sits.
//
// These are positions rather than labels because the screen carries no labels:
// the scanner draws the source, system and department as bare lines above the
// channel. The channel is the line the scanner marks as selected, and that is
// checked rather than assumed, so a layout that moves is noticed instead of
// silently reporting the wrong lines.
const (
	channelLine    = 8
	departmentLine = 6
	sourceLine     = 5
	systemLine     = 4
	valueLine      = 9
)

// The knob steps systems only while the system level is selected, which the
// scanner arms with soft key 1 and drops again after a short idle. So the walk
// cannot dawdle: it clicks again as soon as the screen has caught up with the
// last click, and never waits on a screen that has already changed.
const (
	// clickLimit bounds the turning itself, so a knob that moves nothing can
	// never spin for ever.
	clickLimit = 400

	// stuckClicks is how many clicks showing the same system mean the knob has
	// nowhere else to go.
	stuckClicks = 10

	systemArmKey = device.KeySoft1

	// systemLimit stops a walk that never closes, which would otherwise run
	// until the scanner was switched off.
	systemLimit = 200
)

// confirmClicks is how many clicks are allowed to produce each of those
// systems. More than one is needed because the same name can legitimately come
// up twice running, on a screen too narrow to tell two long names apart.
const confirmClicks = 4

// cycleConfirm is how many systems past the start must match before the walk
// is believed to have closed.
const cycleConfirm = 2

// defaultLimit bounds the walk. A favorites list holds a handful of channels
// and wraps quickly; the full database holds thousands, and walking all of
// them would take many minutes.
const defaultLimit = 500

// How a walk finished. Which one it was decides what the command says
// afterwards, because "this is everything" and "this is what I saw" are
// different answers and must never be printed the same way.
const (
	endLimit    = "limit"
	endReleased = "released"
	endStalled  = "stalled"
	endWatched  = "watched"
	endWrapped  = "wrapped"
)

// fullDatabase is the scanner's own name for its built-in database.
const fullDatabase = "Full Database"

// holdAttempts is how many times to press the hold key before giving up. The
// key toggles rather than sets, and a press made while the scanner is between
// channels does nothing, so more than one may be needed.
const holdAttempts = 6

// settleReads is how many times to look for the screen to change after the
// knob is turned. Each is a round trip of roughly 30ms.
const settleReads = 30

// staleSteps is how many repeats in a row end the walk. The database repeats
// entries constantly, so this has to be well clear of an ordinary run of them.
const staleSteps = 250

// What the channel line reads while the scanner is cycling rather than
// holding on one channel. A trunked system says it differently while it works
// out which talkgroup is on the air.
var cycling = []string{"Scanning...", "ID Scanning..."}

// resetSettle is how long to let the scanner settle onto a system after it is
// put back to plain scanning, before anything is read.
var resetSettle = time.Second

// stepCap is the longest a click waits for the screen to catch up with it.
//
// It is a ceiling rather than a cadence. The walk used to sleep exactly this
// long after every click, on the grounds that the display redraws more slowly
// than the knob turns and reading too early lags the walk into missing systems.
// That is true, and waiting for the screen to actually change establishes it
// directly, in whatever time the scanner takes, rather than paying the worst
// case every time.
//
// The ceiling still matters, because a screen that never changes is a real
// answer: the knob has nowhere to step to, or the system level has lapsed. Both
// are the caller's to interpret, so the wait gives up and hands back the name
// it kept seeing.
var stepCap = 500 * time.Millisecond

// How long to watch the scan cycle when the full database is switched on, and
// how long without anything new before deciding it has been seen.
var (
	watchBudget = 45 * time.Second
	watchQuiet  = 8 * time.Second
)

// entry is one channel as this command reports it.
type entry struct {
	// Source is the list the channel came from, either a favorites list or
	// "Full Database".
	Source string `json:"source"`

	// System is the system the channel belongs to, as the screen names it.
	System string `json:"system"`

	// Department is the department within that system.
	Department string `json:"department"`

	// Channel is the channel itself, as the screen names it.
	Channel string `json:"channel"`

	// Value is the frequency of a conventional channel, or the talkgroup of
	// one on a trunked system. It is reported as the scanner writes it,
	// because the two are not the same kind of thing and pretending otherwise
	// would need a unit that does not exist for talkgroups.
	Value string `json:"value"`
}
