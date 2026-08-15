// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"strings"
	"testing"
	"time"
)

// clockReport is the shape "clock" prints as JSON.
type clockReport struct {
	Date           string `json:"date"`
	Time           string `json:"time"`
	DaylightSaving bool   `json:"daylightSaving"`
	Valid          bool   `json:"valid"`
}

// when parses the two halves back into a single reading. The scanner reports
// wall clock digits rather than an instant, so they are read in the local zone,
// which is the comparison a person standing next to the scanner makes.
func (c clockReport) when(t *testing.T) time.Time {
	t.Helper()

	when, err := time.ParseInLocation("2006-01-02 15:04:05", c.Date+" "+c.Time, time.Local)
	if err != nil {
		t.Fatalf("the scanner reported %q %q, which is not a date and time: %v", c.Date, c.Time, err)
	}
	return when
}

// TestClock checks the reading. It cannot check the scanner keeps good time,
// only that what it reports is a date and a time.
func TestClock(t *testing.T) {
	needScanner(t)

	var report clockReport
	mustJSON(t, &report, "clock")

	when := report.when(t)
	if when.Year() < 2000 || when.Year() > 2100 {
		t.Errorf("the scanner reports the year %d", when.Year())
	}
	if !report.Valid {
		t.Log("the scanner says its clock is not running, so the reading above means little")
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "clock")

		for _, label := range []string{"date:", "time:", "daylight:", "clock:"} {
			if !strings.Contains(res.stdout, label) {
				t.Errorf("the text output has no %q line:\n%s", label, res.stdout)
			}
		}
	})

	t.Run("refusing a value that is not a date", func(t *testing.T) {
		mustFail(t, "", "clock", "set", "the day before yesterday")
	})
}

// TestClockSync checks that syncing leaves the scanner agreeing with this
// computer.
//
// It is a write, but a harmless one: the clock it sets is the correct time, so
// nothing needs putting back afterwards.
func TestClockSync(t *testing.T) {
	needWrites(t)

	var before clockReport
	mustJSON(t, &before, "clock")

	var after clockReport
	mustJSON(t, &after, "clock", "sync")

	if !after.Valid {
		t.Skip("the scanner says its clock is not running, so nothing can be checked")
	}

	// The scanner keeps whole seconds and takes a moment to answer, so the
	// tolerance is seconds rather than exact.
	if gap := after.when(t).Sub(time.Now()); gap > 5*time.Second || gap < -5*time.Second {
		t.Errorf("after syncing the scanner is %s from this computer, wanted under 5 seconds",
			gap.Round(time.Second))
	}

	// Daylight saving is the scanner's own setting and syncing must not touch
	// it. Setting it from this computer's zone is what used to leave a synced
	// scanner an hour fast.
	if after.DaylightSaving != before.DaylightSaving {
		t.Errorf("sync changed daylight saving from %v to %v",
			before.DaylightSaving, after.DaylightSaving)
	}
}
