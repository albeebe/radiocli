// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/10/2026

package suite

import (
	"strings"
	"testing"
)

// TestStatus checks the command everything else depends on: that the scanner
// the suite is pointed at is there and answering.
func TestStatus(t *testing.T) {
	needScanner(t)

	var report struct {
		Port     string `json:"port"`
		Model    string `json:"model"`
		Firmware string `json:"firmware"`
		Display  string `json:"display"`
		Mode     string `json:"mode"`
		Holding  bool   `json:"holding"`
	}
	mustJSON(t, &report, "status")

	if report.Port != harness.device {
		t.Errorf("status reports port %q, wanted %q", report.Port, harness.device)
	}
	if report.Model != harness.model {
		t.Errorf("status reports model %q, wanted %q", report.Model, harness.model)
	}
	if report.Firmware == "" {
		t.Error("status reports no firmware version")
	}
	if report.Mode == "" {
		t.Error("status reports no mode")
	}

	// The boolean is the mode's own suffix, spelled out. They cannot disagree
	// without one of them being read from somewhere else.
	if want := strings.HasSuffix(report.Mode, "Hold"); report.Holding != want {
		t.Errorf("status reports mode %q with holding=%v", report.Mode, report.Holding)
	}

	switch report.Display {
	case "color", "black", "white":
	default:
		t.Errorf("status reports the display as %q, wanted color, black or white", report.Display)
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "status")

		for _, label := range []string{"port:", "model:", "firmware:", "display:", "mode:"} {
			if !strings.Contains(res.stdout, label) {
				t.Errorf("the text output has no %q line:\n%s", label, res.stdout)
			}
		}
	})

	t.Run("a port that does not exist", func(t *testing.T) {
		mustFail(t, "", "--device", "/dev/radiocli-no-such-port", "status")
	})
}
