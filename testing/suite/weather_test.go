// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package suite

import (
	"strconv"
	"strings"
	"testing"
)

// weatherReport is what "weather" and "weather stop" print as JSON.
type weatherReport struct {
	Scanning  bool             `json:"scanning"`
	Mode      string           `json:"mode"`
	Receiving bool             `json:"receiving"`
	Channel   string           `json:"channel"`
	Frequency string           `json:"frequency"`
	Signal    *int             `json:"signal"`
	Channels  []weatherChannel `json:"channels"`
}

// weatherChannel is one row of the sweep the command reports.
type weatherChannel struct {
	Number    string `json:"number"`
	Frequency string `json:"frequency"`
	Signal    *int   `json:"signal"`
	Selected  bool   `json:"selected"`
}

// weatherMode reports what the scanner says it is doing, which is how this
// test checks the command from outside the command's own answer.
func weatherMode(t *testing.T) string {
	t.Helper()

	var report struct {
		Mode string `json:"mode"`
	}
	mustJSON(t, &report, "status")
	return report.Mode
}

// TestWeather covers putting the scanner on the NOAA weather channels and
// taking it off again.
//
// Two things are worth checking beyond "it did something". Which of the two
// weather modes it started: both report a mode of "WX Scan", so a command that
// started the silent alert standby instead would look identical to status and
// to the screen apart from one word, and would play nothing. And whether it
// parked on the strongest channel: the scanner left to itself does not, which
// is the reason this command measures them.
//
// Everything about parking is asked for only when the scanner heard something.
// Whether a NOAA transmitter is in range depends on where the radio is sitting
// and what is on the air, neither of which a test can arrange, so a run
// somewhere out of range checks that the command swept every channel and left
// the scanner on them, and leaves the choice between them untested. Insisting
// on a hold there would be a test of the aerial rather than of the command.
func TestWeather(t *testing.T) {
	needWrites(t)

	// Whatever happens in here, the scanner goes back to scanning. Being left
	// on the weather channels is not something a later test would notice, and
	// it is not something "scan" puts right.
	t.Cleanup(func() {
		mustRun(t, "weather", "stop")
		mustRun(t, "scan")
	})

	var started weatherReport
	mustJSON(t, &started, "weather")

	if !started.Scanning {
		t.Fatalf("weather reported the scanner is not on the weather channels: %+v", started)
	}
	if started.Mode != "Monitor Weather" {
		t.Errorf("weather started %q, wanted %q: the other mode sits silent",
			started.Mode, "Monitor Weather")
	}
	// "WX Hold" rather than "WX Scan": the command parks the scanner on the
	// channel it chose, because letting go drops it straight back onto
	// whichever one the radio's own sweep prefers.
	//
	// Only when it heard something, though. Parking is a choice between
	// channels, and with no weather station in range there is nothing to choose
	// between: the scanner is left sweeping them, which is the honest answer
	// rather than a hold on silence. Which of the two happens is not this
	// suite's to decide, because it comes down to where the radio is sitting
	// and what is on the air, so all that is asked for then is that the command
	// left it on the weather channels at all.
	mode := weatherMode(t)
	switch {
	case started.Receiving && mode != "WX Hold":
		t.Errorf("the scanner reports %q after weather with a station in range, wanted %q",
			mode, "WX Hold")
	case !started.Receiving && !strings.HasPrefix(mode, "WX"):
		t.Errorf("the scanner reports %q after weather, wanted it on the weather channels", mode)
	}

	// Every channel has to be measured, whatever was heard on them, or the
	// choice was made from a sample rather than the set.
	if len(started.Channels) != 7 {
		t.Errorf("weather reported %d channels, wanted 7: %+v", len(started.Channels), started.Channels)
	}
	for i, c := range started.Channels {
		if c.Number != strconv.Itoa(i+1) {
			t.Errorf("channel %d of the sweep is numbered %q, wanted %q", i, c.Number, strconv.Itoa(i+1))
		}
		if !strings.Contains(c.Frequency, "MHz") {
			t.Errorf("channel %s is reported at %q, wanted something in MHz", c.Number, c.Frequency)
		}
	}

	// Nothing here can insist the scanner hears a weather station: that
	// depends on where the radio is. When it does hear one, the channel it
	// parked on has to be the best one it measured.
	if started.Receiving {
		if started.Channel == "" {
			t.Error("weather reported it is receiving but named no channel")
		}
		if !strings.Contains(started.Frequency, "MHz") {
			t.Errorf("weather reported the frequency as %q, wanted something in MHz",
				started.Frequency)
		}
		if started.Signal == nil {
			t.Fatal("weather reported it is receiving but no signal strength")
		}

		for _, c := range started.Channels {
			if c.Signal != nil && *c.Signal > *started.Signal {
				t.Errorf("weather held channel %s at %d dBm, but channel %s reads %d dBm",
					started.Channel, *started.Signal, c.Number, *c.Signal)
			}
			if c.Selected != (c.Number == started.Channel) {
				t.Errorf("channel %s is marked selected=%v, wanted %v",
					c.Number, c.Selected, c.Number == started.Channel)
			}
		}
	} else {
		t.Log("no weather channel is receivable here, so the channel it settles on is untested")
	}

	t.Run("starting it again is not a toggle", func(t *testing.T) {
		var again weatherReport
		mustJSON(t, &again, "weather")

		if !again.Scanning || again.Mode != "Monitor Weather" {
			t.Errorf("running weather twice left the scanner in %+v, wanted it still monitoring", again)
		}
	})

	t.Run("another command does not knock it off them", func(t *testing.T) {
		// The scanner reports itself as holding once it is parked on the best
		// channel, and every command that walks a menu tidies up by releasing
		// a hold. Weather has to be the exception, or changing the volume ends
		// the weather monitoring somebody asked for.
		//
		// What it was is what it has to still be, rather than "WX Hold"
		// specifically. Which of the two weather modes the scanner is in comes
		// down to whether it can hear a station from where it is sitting, which
		// is not this test's business and not something it can arrange. That
		// nothing changed it is.
		before := weatherMode(t)
		if !strings.HasPrefix(before, "WX") {
			t.Fatalf("the scanner is in %q, which is not the weather channels at all", before)
		}

		mustRun(t, "volume")
		mustRun(t, "clock")

		if after := weatherMode(t); after != before {
			t.Errorf("the scanner is in %q after two unrelated commands, wanted %q: "+
				"something knocked it off the weather channels", after, before)
		}
	})

	t.Run("stopping the weather channels", func(t *testing.T) {
		var stopped weatherReport
		res := mustJSON(t, &stopped, "weather", "stop")

		if stopped.Scanning {
			t.Errorf("weather stop left the scanner on the weather channels: %+v", stopped)
		}
		if !strings.Contains(res.stderr, "left the weather channels") {
			t.Errorf("weather stop did not say what it did:\n%s", res.stderr)
		}
		if mode := weatherMode(t); mode == "WX Scan" {
			t.Error("the scanner is still in WX Scan after weather stop")
		}
	})

	t.Run("stopping when it is not on them does nothing", func(t *testing.T) {
		res := mustRun(t, "weather", "stop")

		if !strings.Contains(res.stderr, "not on the weather channels") {
			t.Errorf("weather stop on a scanning scanner did not say so:\n%s", res.stderr)
		}
		if inMenu(t) {
			t.Error("weather stop opened a menu when there was nothing to do")
		}
	})
}
