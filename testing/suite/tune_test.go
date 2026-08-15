// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestTune_Refusals checks the frequencies the tool turns down before it
// touches the scanner. They cost nothing to run and leave the scanner alone,
// so they need no permission beyond a scanner being attached.
func TestTune_Refusals(t *testing.T) {
	needScanner(t)

	cases := []struct {
		name string
		arg  string
		want string
	}{
		{"not a number", "the fire one", "invalid frequency"},
		{"below the coverage", "1030kHz", "it covers"},
		{"above the coverage", "1400", "it covers"},
		{"between the bands", "600", "it covers"},
		{"inside the blocked cellular band", "880", "cellular"},
		{"inside the other blocked cellular band", "830", "cellular"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := mustFail(t, c.want, "tune", c.arg)

			// A refusal that does not say what would have worked sends the
			// reader to the manual, so every one of these carries either the
			// coverage or an example of what to type instead.
			if !strings.Contains(res.stderr, "MHz") && !strings.Contains(res.stderr, "kHz") {
				t.Errorf("the refusal names no frequencies at all:\n%s", res.stderr)
			}
		})
	}
}

// TestTune puts the scanner on a frequency and checks it went there.
//
// Broadcast FM is used because it is the one band that is reliably occupied
// wherever this is run, and because it is unmistakable if it is left tuned by
// accident.
func TestTune(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	for _, want := range []float64{107.9, 162.550, 155.550} {
		var report struct {
			Megahertz float64 `json:"megahertz"`
			Receiving bool    `json:"receiving"`
			RSSI      string  `json:"rssi"`
			Bars      string  `json:"bars"`
		}
		mustJSON(t, &report, "tune", trim(want))

		if math.Abs(report.Megahertz-want) > 0.0001 {
			t.Errorf("asked for %.4f MHz, the scanner reports %.4f MHz", want, report.Megahertz)
		}
		if report.Receiving && report.RSSI == "" {
			t.Errorf("%.4f MHz is reported as receiving with no signal reading", want)
		}
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "tune", "107.9")

		for _, label := range []string{"frequency:", "receiving:"} {
			if !strings.Contains(res.stdout, label) {
				t.Errorf("the text output has no %q line:\n%s", label, res.stdout)
			}
		}
	})

	t.Run("rounding to what the protocol carries", func(t *testing.T) {
		// The scanner tunes in hundreds of hertz, so a finer frequency is
		// rounded rather than refused, and the rounded value is what gets
		// reported.
		var report struct {
			Megahertz float64 `json:"megahertz"`
		}
		mustJSON(t, &report, "tune", "155.55004")

		if math.Abs(report.Megahertz-155.550) > 0.0002 {
			t.Errorf("155.55004 MHz was tuned as %.6f MHz", report.Megahertz)
		}
	})

	t.Run("the units it accepts", func(t *testing.T) {
		for _, arg := range []string{"107.9", "107.9MHz", "107900kHz", "107900000Hz"} {
			var report struct {
				Megahertz float64 `json:"megahertz"`
			}
			mustJSON(t, &report, "tune", arg)

			if math.Abs(report.Megahertz-107.9) > 0.0001 {
				t.Errorf("%q tuned %.4f MHz, wanted 107.9 MHz", arg, report.Megahertz)
			}
		}
	})
}

// trim renders a frequency the way somebody would type it, with no trailing
// zeroes.
func trim(mhz float64) string {
	return strconv.FormatFloat(mhz, 'f', -1, 64)
}
