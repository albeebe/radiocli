// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/15/2026

// Putting the scanner back after a run that could not put it back itself.
//
// A run stopped from the keyboard, or by anything else that can be caught,
// tidies up on its way out: the test in progress finishes, nothing new starts,
// and TestMain reaches the restore. That covers everything except the one case
// nothing can catch. A run killed outright, by "kill -9" or a machine going to
// sleep or a cable pulled, stops between one instruction and the next, and
// whatever it had changed stays changed.
//
// What that leaves behind is of two kinds, and they need different answers.
//
// The entries the tests build are recognisable: nothing but this suite makes a
// favorites list called RADIOCLI TEST, so anything by that name at the start of
// a run is debris and is removed. The settings are not recognisable at all. A
// volume of 9 is a volume of 9 whoever set it, so the only way to know it was
// 12 before the run that died is to have written 12 down first. That is what
// the journal below is.
package suite

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// journal is the state as it goes to disk.
//
// It exists because state's fields are unexported, which is right for a type
// nothing outside this package reads, and unhelpful for one that has to
// survive the process.
type journal struct {
	Volume    int             `json:"volume"`
	Monitored []string        `json:"monitored"`
	Latitude  float64         `json:"latitude"`
	Longitude float64         `json:"longitude"`
	Miles     float64         `json:"miles"`
	GPS       bool            `json:"gps"`
	Lists     map[string]bool `json:"lists"`
}

// journalPath is where the note is kept.
//
// The cache directory rather than the config directory: this is something the
// suite can rebuild by being run again, and it has no business sitting next to
// anything a person edits.
func journalPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "radiocli", "unfinished-test-run.json"), nil
}

// writeJournal records what the scanner was doing before the run changed it.
//
// Failing to write is logged and not fatal. The journal only matters to a run
// that dies, and refusing to test because the note could not be written would
// turn a rare inconvenience into a certain one.
func writeJournal(s state) {
	path, err := journalPath()
	if err != nil {
		log.Printf("cannot work out where to record the scanner's state: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Printf("cannot record the scanner's state: %v", err)
		return
	}

	body, err := json.Marshal(journal{
		Volume:    s.volume,
		Monitored: s.monitored,
		Latitude:  s.latitude,
		Longitude: s.longitude,
		Miles:     s.miles,
		GPS:       s.gps,
		Lists:     s.lists,
	})
	if err != nil {
		log.Printf("cannot record the scanner's state: %v", err)
		return
	}

	if err := os.WriteFile(path, body, 0o600); err != nil {
		log.Printf("cannot record the scanner's state: %v", err)
	}
}

// readJournal reads what a previous run recorded, and reports whether there
// was one. A missing file is the ordinary case and not an error: it means the
// last run finished and cleared up after itself.
func readJournal() (state, bool) {
	path, err := journalPath()
	if err != nil {
		return state{}, false
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return state{}, false
	}

	var j journal
	if err := json.Unmarshal(body, &j); err != nil {
		log.Printf("the record of an earlier run cannot be read, and is being ignored: %v", err)
		return state{}, false
	}

	return state{
		volume:    j.Volume,
		monitored: j.Monitored,
		latitude:  j.Latitude,
		longitude: j.Longitude,
		miles:     j.Miles,
		gps:       j.GPS,
		lists:     j.Lists,
	}, true
}

// clearJournal removes the note, which is what says the run finished tidily.
func clearJournal() {
	path, err := journalPath()
	if err != nil {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("cannot clear the record of this run: %v", err)
	}
}

// recoverPreviousRun puts the scanner back to what a run that died recorded,
// and removes anything it left standing.
//
// This runs before the state for this run is read, so that what this run
// promises to restore is the scanner as its owner left it rather than as the
// dead run left it. Without that, one killed run would poison every run after
// it: the volume it changed and never put back would be read as the volume to
// return to.
func recoverPreviousRun() {
	if before, ok := readJournal(); ok {
		log.Printf("an earlier run was killed before it could put the scanner back")
		log.Printf("it recorded: %s", before)

		// Settings only. The lists this run can see are not the lists that run
		// could see, and anything created since is somebody else's, so deciding
		// what to delete from a stale reading would risk deleting it.
		restoreSettings(before)
		clearJournal()
	}

	sweepLeftovers()
}

// sweepLeftovers deletes any favorites list a previous run left behind.
//
// By name, which is the one thing that survives. The end of an ordinary run
// tells its leftovers apart by index, because it knows which lists were there
// when it started; a run that comes along afterwards knows nothing of the
// kind, and the name is what is left to go on. Nothing but this suite creates
// a list called RADIOCLI TEST, which is why the name is as obvious as it is.
//
// A list created but killed before it was named is not caught here. The
// scanner calls it something like "FAVORITES 0" in the meantime, which is a
// name a person might have chosen, and deleting on that guess is worse than
// leaving it. The end-of-run sweep catches those, by index, within the run
// that made them.
func sweepLeftovers() {
	lists, err := readLists()
	if err != nil {
		log.Printf("cannot check for leftovers from an earlier run: %v", err)
		return
	}

	for _, l := range lists {
		if l.BuiltIn || !strings.HasPrefix(l.Name, testName) {
			continue
		}

		if res, err := execute("favorites", "delete", l.Name, "--yes"); err != nil || res.code != 0 {
			log.Printf("an earlier run left %q on the scanner and it cannot be removed: "+
				"run \"radiocli favorites delete %q --yes\"", l.Name, l.Name)
			continue
		}
		log.Printf("removed %q, which an earlier run left behind", l.Name)
	}
}
