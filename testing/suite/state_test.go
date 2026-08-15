// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
)

// state is as much of the scanner's settings as the suite knows how to put
// back: how loud it is, which lists it is scanning, and where it thinks it is.
type state struct {
	volume    int
	monitored []string
	latitude  float64
	longitude float64
	miles     float64

	// gps is whether the scanner is following its own receiver. It decides
	// whether the position is worth putting back: when the receiver is in
	// charge, it owns the position and writing one would be overwritten.
	gps bool

	// lists is the index of every favorites list the scanner held. Nothing
	// but this suite creates a list while it runs, so anything carrying an
	// index that is not in here at the end was made by a test.
	lists map[string]bool
}

// String renders the state for the log line printed at the start of a run, so
// that a run which fails to restore something leaves a record of what it was.
func (s state) String() string {
	where := fmt.Sprintf("%.6f, %.6f", s.latitude, s.longitude)
	if s.miles > 0 {
		where += fmt.Sprintf(" within %.0f miles", s.miles)
	}
	if s.gps {
		where += ", GPS on"
	} else {
		where += ", GPS off"
	}

	scanning := "nothing"
	if len(s.monitored) > 0 {
		scanning = strings.Join(s.monitored, ", ")
	}

	return fmt.Sprintf("volume %d, scanning %s, at %s", s.volume, scanning, where)
}

// readState reads what the scanner is doing now.
func readState() (state, error) {
	var s state

	var volume struct {
		Level int `json:"level"`
	}
	if err := readJSON(&volume, "volume"); err != nil {
		return s, err
	}
	s.volume = volume.Level

	lists, err := readLists()
	if err != nil {
		return s, err
	}

	s.lists = map[string]bool{}
	for _, l := range lists {
		s.lists[l.Index] = true
		if l.Monitored {
			s.monitored = append(s.monitored, l.Name)
		}
	}

	// This one reads the setting off the scanner's own screen, which means
	// opening a menu and stopping the scan for a moment. It is the only way
	// to ask, and it is worth asking: the position cannot be put back without
	// knowing whether the receiver is going to overwrite it.
	var gps struct {
		GPS       bool    `json:"gps"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Range     float64 `json:"range"`
	}
	if err := readJSON(&gps, "location", "gps", "--status"); err != nil {
		return s, err
	}
	s.gps = gps.GPS
	s.latitude, s.longitude, s.miles = gps.Latitude, gps.Longitude, gps.Range

	return s, nil
}

// readJSON runs a reading command and decodes it, for the callers that have no
// test to fail and have to report an error instead.
func readJSON(v any, args ...string) error {
	res, err := execute(append([]string{"-o", "json"}, args...)...)
	if err != nil {
		return err
	}
	if res.code != 0 {
		return fmt.Errorf("radiocli %s: %s", strings.Join(args, " "), firstLine(res.stderr))
	}
	return json.Unmarshal([]byte(res.stdout), v)
}

// restore puts the scanner back the way the run found it.
//
// Each setting is compared before it is written, so a run that changed nothing
// writes nothing. The order matters: setting a location switches full database
// scanning on, which would undo the choice of lists if it came afterwards.
func restore(before state) {
	restoreWith(before, true)
}

// restoreSettings puts the settings back without deciding anything about which
// lists exist.
//
// It is for putting the scanner back to a reading taken by a run that has since
// died. That reading says which lists that run could see, which is not the same
// as which lists are somebody's own: a list created between then and now is not
// debris, and sweeping against a stale reading would delete it.
func restoreSettings(before state) {
	restoreWith(before, false)
}

// restoreWith is the body of both. clean says whether to remove the lists the
// reading did not have, which is only safe when the reading is this run's own.
func restoreWith(before state, clean bool) {
	log.Printf("putting the scanner back")

	now, err := readState()
	if err != nil {
		log.Printf("cannot read the scanner to put it back: %v", err)
		log.Printf("it was %s when the run started", before)
		return
	}

	restoreLocation(before, now)

	// Read again before putting the lists back. Setting a location switches
	// full database scanning on, so what was true a moment ago is not true
	// now, and comparing against the stale reading leaves the database on.
	if after, err := readState(); err == nil {
		now = after
	}

	if clean {
		sweep(before)
	}
	restoreLists(before, now)
	restoreVolume(before, now)

	// Whatever else happened, leave it scanning rather than parked in a menu
	// or holding one frequency.
	if res, err := execute("scan"); err != nil || res.code != 0 {
		log.Printf("could not return the scanner to scanning: run \"radiocli scan\"")
	}

	after, err := readState()
	if err != nil {
		log.Printf("cannot confirm the scanner was put back: %v", err)
		return
	}
	log.Printf("scanner state after the run: %s", after)
}

// restoreLocation puts the GPS setting and the position back.
//
// The GPS goes first. It decides who owns the position: with the receiver in
// charge, anything written is overwritten as soon as it has a fix, so there
// would be no point writing one.
//
// The zip a position came from cannot be read off the scanner, which reports
// where it is rather than what was typed to get there. Nothing needs it: a
// position written directly, with the GPS switched off, leaves the scanner in
// the same state a zip would have.
func restoreLocation(before, now state) {
	if before.gps != now.gps {
		args := []string{"location", "gps"}
		if !before.gps {
			args = append(args, "--off")
		}

		if res, err := execute(args...); err != nil || res.code != 0 {
			log.Printf("could not put the GPS back: run \"radiocli %s\"", strings.Join(args[1:], " "))
		} else {
			log.Printf("GPS put back to %s", onOff(before.gps))
		}
	}

	if sameLocation(before, now) {
		return
	}

	if before.gps {
		// The receiver owns the position and will settle on its own. Writing
		// one here would be overwritten within a minute or two.
		log.Printf("the position is left to the receiver, which was in charge when the run started")
		return
	}

	position := fmt.Sprintf("%.6f,%.6f", before.latitude, before.longitude)
	miles := int(math.Round(before.miles))

	if res, err := execute("location", "set", "--position", position, "--range", fmt.Sprint(miles)); err != nil || res.code != 0 {
		log.Printf("could not put the position back: run "+
			"\"radiocli location set --position %s --range %d\"", position, miles)
		return
	}
	log.Printf("position put back to %s within %d miles", position, miles)
}

// list is one favorites list, as much of it as putting the scanner back needs.
type list struct {
	Name      string `json:"name"`
	Index     string `json:"index"`
	Monitored bool   `json:"monitored"`
	BuiltIn   bool   `json:"builtIn"`
}

// readLists reads the favorites lists the scanner holds.
func readLists() ([]list, error) {
	var lists []list
	err := readJSON(&lists, "favorites")
	return lists, err
}

// sweep deletes any favorites list that was not there when the run started.
//
// Every test deletes what it made, so this finds nothing on an ordinary run.
// It is here for the run that was interrupted between creating something and
// deleting it, which would otherwise leave an entry on the scanner for good.
//
// It works by identity rather than by name. A list is created before it is
// named, and the scanner calls it something like "FAVORITES 0" in between, so
// a run that died in that gap leaves a list whose name says nothing about
// where it came from. What is reliable is that nothing except this suite
// creates a list while it is running, so a list holding an index the run did
// not start with is one of ours.
//
// Only favorites lists are swept. Everything the suite builds is inside one,
// and deleting a list takes its contents with it.
func sweep(before state) {
	if len(before.lists) == 0 {
		// Nothing was recorded, so nothing can be told apart. Deleting on a
		// guess is worse than leaving something behind.
		return
	}

	lists, err := readLists()
	if err != nil {
		log.Printf("cannot check for leftovers: %v", err)
		return
	}

	for _, l := range lists {
		if l.BuiltIn || before.lists[l.Index] {
			continue
		}

		if res, err := execute("favorites", "delete", l.Name, "--yes"); err != nil || res.code != 0 {
			log.Printf("a list this run created is still on the scanner: "+
				"delete %q by hand", l.Name)
			continue
		}
		log.Printf("swept up %q, which this run created and did not remove", l.Name)
	}
}

// onOff renders a setting for a log line.
func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// sameLocation reports whether two readings are the same place.
//
// A live GPS fix wanders by a metre or two between readings, so the comparison
// is deliberately loose: 0.001 degrees is about a hundred metres, far below
// the distance between two zip codes and far above the drift.
func sameLocation(a, b state) bool {
	return a.miles == b.miles &&
		math.Abs(a.latitude-b.latitude) < 0.001 &&
		math.Abs(a.longitude-b.longitude) < 0.001
}

// restoreLists puts back which favorites lists are being scanned.
func restoreLists(before, now state) {
	if equalNames(before.monitored, now.monitored) {
		return
	}

	args := append([]string{"favorites", "scan"}, before.monitored...)
	if len(before.monitored) == 0 {
		args = []string{"favorites", "scan", "--none"}
	}

	if res, err := execute(args...); err != nil || res.code != 0 {
		log.Printf("could not put the scanned lists back: run \"radiocli %s\"",
			strings.Join(args[1:], " "))
		return
	}
	log.Printf("scanned lists put back to %s", strings.Join(before.monitored, ", "))
}

// restoreVolume puts the volume back.
func restoreVolume(before, now state) {
	if before.volume == now.volume {
		return
	}
	if res, err := execute("volume", "set", fmt.Sprint(before.volume)); err != nil || res.code != 0 {
		log.Printf("could not set the volume back to %d", before.volume)
		return
	}
	log.Printf("volume put back to %d", before.volume)
}

// equalNames reports whether two lists hold the same names in the same order,
// which is how the scanner reports them either way.
func equalNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
