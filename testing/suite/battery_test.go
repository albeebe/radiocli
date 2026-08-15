// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"math"
	"strings"
	"testing"
)

// TestBattery checks the battery reading, and checks it for sense rather than
// for an exact value: the numbers move on their own, so what can be tested is
// that they are numbers a battery could actually hold.
func TestBattery(t *testing.T) {
	needScanner(t)

	var report struct {
		State       string  `json:"state"`
		Charging    bool    `json:"charging"`
		Percent     int     `json:"percent"`
		Volts       float64 `json:"volts"`
		Milliamps   int     `json:"milliamps"`
		Celsius     float64 `json:"celsius"`
		Fahrenheit  float64 `json:"fahrenheit"`
		NeedsAction bool    `json:"needsAction"`
	}
	mustJSON(t, &report, "battery")

	if report.State == "" {
		t.Error("the charger reports no state")
	}
	if report.Percent < 0 || report.Percent > 100 {
		t.Errorf("charge is %d%%, wanted 0 to 100", report.Percent)
	}
	if report.Volts <= 0 || report.Volts > 20 {
		t.Errorf("voltage is %.2f V, which is not a battery this scanner takes", report.Volts)
	}
	if report.Celsius < -20 || report.Celsius > 80 {
		t.Errorf("temperature is %.1f C, which is not a room this scanner survives", report.Celsius)
	}

	// The Fahrenheit reading is derived rather than measured, so it can be
	// checked exactly.
	if want := report.Celsius*9/5 + 32; math.Abs(report.Fahrenheit-want) > 0.05 {
		t.Errorf("temperature is %.1f F, wanted %.1f F for %.1f C",
			report.Fahrenheit, want, report.Celsius)
	}

	// Current flowing into the battery and the charging flag are two readings
	// of the same fact, so they have to agree.
	if report.Milliamps > 0 && !report.Charging {
		t.Errorf("%d mA is flowing in but the charger says it is not charging", report.Milliamps)
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "battery")

		for _, label := range []string{"charge:", "state:", "voltage:", "current:", "temperature:"} {
			if !strings.Contains(res.stdout, label) {
				t.Errorf("the text output has no %q line:\n%s", label, res.stdout)
			}
		}
	})
}
