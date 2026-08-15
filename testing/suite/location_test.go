// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// locationReport is the shape every form of this command prints as JSON.
type locationReport struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Range     float64 `json:"range"`
}

// readLocation reads where the scanner thinks it is.
func readLocation(t *testing.T, args ...string) locationReport {
	t.Helper()

	var report locationReport
	mustJSON(t, &report, append([]string{"location"}, args...)...)
	return report
}

// TestLocation checks the reading, which is where the scanner draws database
// channels from rather than simply where it is.
func TestLocation(t *testing.T) {
	needScanner(t)

	report := readLocation(t)

	if report.Latitude < -90 || report.Latitude > 90 {
		t.Errorf("latitude is %.6f, which is not a latitude", report.Latitude)
	}
	if report.Longitude < -180 || report.Longitude > 180 {
		t.Errorf("longitude is %.6f, which is not a longitude", report.Longitude)
	}
	if report.Range < 0 {
		t.Errorf("range is %.1f miles", report.Range)
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "location")

		for _, label := range []string{"latitude:", "longitude:", "position:", "range:"} {
			if !strings.Contains(res.stdout, label) {
				t.Errorf("the text output has no %q line:\n%s", label, res.stdout)
			}
		}

		// A range of zero is not a radius of nothing, it is no range set at
		// all, and the text output has to say so rather than print 0.0 miles.
		if report.Range == 0 && !strings.Contains(res.stdout, "none set") {
			t.Errorf("no range is set but the output does not say so:\n%s", res.stdout)
		}
	})

	t.Run("values that are not zip codes", func(t *testing.T) {
		for _, arg := range []string{"0", "0306", "123450", "greenbank", "0306a"} {
			mustFail(t, "is not a zip code", "location", "set", arg)
		}
	})

	t.Run("ranges the tool will not accept", func(t *testing.T) {
		for _, miles := range []string{"0", "51", "100"} {
			mustFail(t, "want 1 to 50 whole miles", "location", "set", "12345", "--range", miles)
		}
	})
}

// TestLocationSet points the scanner at a zip code and checks it went there.
func TestLocationSet(t *testing.T) {
	needWrites(t)
	keepScannedLists(t)

	before := readLocation(t)

	// Two zips far enough apart that no GPS drift could be mistaken for the
	// move, and neither of them where this is likely to be run.
	const zip, miles = "90210", 5

	after := readLocation(t, "set", zip, "--range", "5")

	if after.Range != miles {
		t.Errorf("the range is %.1f miles, wanted %d", after.Range, miles)
	}
	if sameLocation(state{latitude: before.Latitude, longitude: before.Longitude, miles: before.Range},
		state{latitude: after.Latitude, longitude: after.Longitude, miles: after.Range}) {
		t.Errorf("setting the location to %s left the scanner at %.6f, %.6f",
			zip, after.Latitude, after.Longitude)
	}

	// Beverly Hills, to within the width of a zip code.
	if math.Abs(after.Latitude-34.09) > 0.2 || math.Abs(after.Longitude+118.41) > 0.2 {
		t.Errorf("zip %s put the scanner at %.6f, %.6f, which is not where that zip is",
			zip, after.Latitude, after.Longitude)
	}

	t.Run("read back in a later run", func(t *testing.T) {
		// The position has to survive the connection closing, which is what
		// makes it a setting rather than a live fix.
		again := readLocation(t)

		if again.Latitude != after.Latitude || again.Longitude != after.Longitude {
			t.Errorf("the position was %.6f, %.6f and is now %.6f, %.6f",
				after.Latitude, after.Longitude, again.Latitude, again.Longitude)
		}
	})

	t.Run("a whole number of miles", func(t *testing.T) {
		set := readLocation(t, "set", zip, "--range", "25")

		if set.Range != 25 {
			t.Errorf("the range is %.1f miles, wanted 25", set.Range)
		}
	})

	t.Run("a zip the scanner does not hold", func(t *testing.T) {
		res := run(t, "location", "set", "00000")

		if res.code == 0 {
			t.Errorf("the scanner accepted 00000 as a zip code:\n%s", res.stdout)
			return
		}
		if !strings.Contains(res.stderr, "Out of Range") {
			t.Errorf("the refusal was %q, wanted it to quote the scanner's own answer",
				firstLine(res.stderr))
		}
	})
}

// TestLocationSet_Position writes a latitude and longitude directly.
//
// This is the one way to put a position back where it was, because the zip
// that produced it cannot be read off the scanner. It is also the only write
// in the tool that goes over the protocol rather than through the menus, so it
// is quick and it leaves the scan running.
func TestLocationSet_Position(t *testing.T) {
	needWrites(t)

	before := readLocation(t)
	t.Cleanup(func() {
		mustRun(t, "location", "set",
			"--position", fmt.Sprintf("%.6f,%.6f", before.Latitude, before.Longitude),
			"--range", fmt.Sprintf("%.0f", before.Range))
	})

	// Beverly Hills, which is nowhere near wherever this is being run, so no
	// drift or fix could be mistaken for the move.
	const latitude, longitude = 34.103131, -118.416253

	set := readLocation(t, "set", "--position", fmt.Sprintf("%f,%f", latitude, longitude))

	// The scanner keeps six decimal places to within a millionth of a degree,
	// about a tenth of a metre, so this is not an exact comparison.
	if math.Abs(set.Latitude-latitude) > 0.00001 || math.Abs(set.Longitude-longitude) > 0.00001 {
		t.Errorf("the position was written as %.6f, %.6f and reads back as %.6f, %.6f",
			latitude, longitude, set.Latitude, set.Longitude)
	}

	t.Run("the range is left alone", func(t *testing.T) {
		if set.Range != before.Range {
			t.Errorf("writing a position changed the range from %.1f to %.1f miles",
				before.Range, set.Range)
		}
	})

	t.Run("read back in a later run", func(t *testing.T) {
		again := readLocation(t)

		if again.Latitude != set.Latitude || again.Longitude != set.Longitude {
			t.Errorf("the position was %.6f, %.6f and is now %.6f, %.6f",
				set.Latitude, set.Longitude, again.Latitude, again.Longitude)
		}
	})

	t.Run("the scan is not interrupted", func(t *testing.T) {
		// Nothing here enters a menu, which is what makes this usable as a
		// restore: it puts the position back without disturbing anything else.
		if inMenu(t) {
			t.Error("writing a position left the scanner in a menu")
		}
	})

	t.Run("setting a range at the same time", func(t *testing.T) {
		set := readLocation(t, "set", "--position", "34.103131,-118.416253", "--range", "25")

		if set.Range != 25 {
			t.Errorf("the range is %.1f miles, wanted 25", set.Range)
		}
	})

	t.Run("positions the tool will not accept", func(t *testing.T) {
		mustFail(t, "is not a position", "location", "set", "--position", "38.43")
		mustFail(t, "is not a latitude", "location", "set", "--position", "99,-79.8")
		mustFail(t, "is not a longitude", "location", "set", "--position", "38.43,-500")
		mustFail(t, "is not a latitude", "location", "set", "--position", "north,west")
		mustFail(t, "want 1 to 50 whole miles",
			"location", "set", "--position", "38.43,-79.84", "--range", "99")
	})

	t.Run("a zip and a position together", func(t *testing.T) {
		mustFail(t, "choose one", "location", "set", "12345", "--position", "38.43,-79.84")
	})

	t.Run("neither a zip nor a position", func(t *testing.T) {
		mustFail(t, "name a zip code, or pass --position", "location", "set")
	})
}

// TestLocationGPS switches the built-in receiver on and off again.
//
// Switching it on checks that the command returns rather than sitting on
// satellites: what it changes is a setting, and where the receiver decides it
// is arrives afterwards on its own schedule.
func TestLocationGPS(t *testing.T) {
	needWrites(t)
	keepScannedLists(t)

	// Whatever the setting was, it goes back that way. This is the only test
	// that touches it, and it can put it back itself, which is why it needs
	// nothing to be passed on the command line.
	before := readGPS(t)
	t.Cleanup(func() {
		if before.GPS {
			mustRun(t, "location", "gps")
			return
		}
		mustRun(t, "location", "gps", "--off")
	})

	res := mustRun(t, "location", "gps")

	if !strings.Contains(res.stdout, "source:    GPS") {
		t.Errorf("the output does not say the position now comes from the GPS:\n%s", res.stdout)
	}

	// Under a minute is the point of the command. Waiting for a fix is what
	// --wait is for.
	if res.took > time.Minute {
		t.Errorf("switching the GPS on took %s, which means it waited for a fix",
			res.took.Round(time.Second))
	}

	// Read the setting back independently of the command, which checked it
	// too. A disagreement gets one more look before it counts: switching the
	// GPS sends the scanner away for a moment, and this read walks its menus.
	now := readGPS(t)
	if !now.GPS {
		t.Logf("the GPS read as off straight after switching it on, at %.6f, %.6f: looking again",
			now.Latitude, now.Longitude)
		time.Sleep(2 * time.Second)

		if again := readGPS(t); !again.GPS {
			t.Fatalf("the scanner reports the GPS off after switching it on, twice over.\n"+
				"The command reported:\n%s", res.stdout)
		}
	}

	t.Run("switching it off again", func(t *testing.T) {
		// The position stays where it is, which is what makes this the way
		// back: nothing has to be moved to stop following the receiver.
		where := readLocation(t)

		res := mustRun(t, "location", "gps", "--off")
		if !strings.Contains(res.stdout, "source:    fixed") {
			t.Errorf("the output does not say the position is now fixed:\n%s", res.stdout)
		}

		after := readGPS(t)
		if after.GPS {
			t.Error("the scanner reports the GPS is still on after switching it off")
		}
		if after.Latitude != where.Latitude || after.Longitude != where.Longitude {
			t.Errorf("switching the GPS off moved the position from %.6f, %.6f to %.6f, %.6f",
				where.Latitude, where.Longitude, after.Latitude, after.Longitude)
		}
	})

	t.Run("the range survives either way", func(t *testing.T) {
		// A zip sets the range to 10 miles and that outlives the switch, so
		// the position follows the receiver while the radius stays put.
		if now := readLocation(t); now.Range == 0 {
			t.Error("the range was lost while switching the GPS on and off")
		}
	})

	t.Run("flags that contradict each other", func(t *testing.T) {
		mustFail(t, "choose one", "location", "gps", "--off", "--status")
		mustFail(t, "no fix to wait for", "location", "gps", "--status", "--wait")
		mustFail(t, "not something switching the GPS off does", "location", "gps", "--off", "--wait")
	})
}

// gpsReport is the shape "location gps --status" prints as JSON.
type gpsReport struct {
	GPS       bool    `json:"gps"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Range     float64 `json:"range"`
}

// readGPS reads whether the scanner is following its own receiver.
//
// This one stops the scan for a moment. The setting is only readable off the
// screen that sets it, so asking means opening a menu.
func readGPS(t *testing.T) gpsReport {
	t.Helper()

	var report gpsReport
	mustJSON(t, &report, "location", "gps", "--status")
	return report
}

// keepLocation puts the position back when the test finishes.
//
// The zip a position came from cannot be read off the scanner, which reports
// where it ended up rather than what was typed to get there. Writing the
// position straight back leaves it in the same place, which is what matters.
func keepLocation(t *testing.T) {
	t.Helper()

	before := readLocation(t)

	t.Cleanup(func() {
		mustRun(t, "location", "set",
			"--position", fmt.Sprintf("%.6f,%.6f", before.Latitude, before.Longitude),
			"--range", fmt.Sprintf("%.0f", before.Range))
	})
}

// keepScannedLists puts back which lists are scanned when the test finishes.
//
// Setting a location switches full database scanning on, which undoes any
// narrower choice that was in force. Left that way, everything after it would
// be scanning hundreds of thousands of channels.
func keepScannedLists(t *testing.T) {
	t.Helper()

	before := monitored(readFavorites(t))

	t.Cleanup(func() {
		args := append([]string{"favorites", "scan"}, before...)
		if len(before) == 0 {
			args = []string{"favorites", "scan", "--none"}
		}
		mustRun(t, args...)
	})
}
