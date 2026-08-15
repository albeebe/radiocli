// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

import (
	"strings"
	"testing"
)

// backlightReport is what the backlight command answers with.
type backlightReport struct {
	On    bool `json:"on"`
	Level int  `json:"level"`
}

// keysReport is what the keys subcommand answers with.
type keysReport struct {
	Enabled bool `json:"enabled"`
}

// readBacklight reports whether the scanner is lit.
func readBacklight(t *testing.T) backlightReport {
	t.Helper()

	var state backlightReport
	mustJSON(t, &state, "backlight")
	return state
}

// readKeypadLight reports whether the keypad lights up with the screen.
func readKeypadLight(t *testing.T) bool {
	t.Helper()

	var state keysReport
	mustJSON(t, &state, "backlight", "keys")
	return state.Enabled
}

// TestBacklight checks the read, which presses nothing.
func TestBacklight(t *testing.T) {
	needScanner(t)

	state := readBacklight(t)

	// The level is the dimmer setting while lit and zero while dark. Those two
	// have to agree: a light reported as on at level zero would mean the
	// command had invented one of them.
	if state.On != (state.Level > 0) {
		t.Errorf("the light is reported %v at level %d, which cannot both be true",
			state.On, state.Level)
	}
	if state.Level < 0 || state.Level > 3 {
		t.Errorf("the level is %d, wanted 0 to 3", state.Level)
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "backlight")

		if !strings.Contains(res.stdout, "backlight:") {
			t.Errorf("the report does not say what the backlight is doing:\n%s", res.stdout)
		}
	})

	t.Run("refusing an argument it does not take", func(t *testing.T) {
		mustFail(t, "", "backlight", "on-please")
	})
}

// restoreBacklight puts the light back the way the run found it.
//
// Both of the tests below leave the light somewhere it was not, and the suite's
// rule is that a run gives the scanner back unchanged.
func restoreBacklight(t *testing.T) {
	t.Helper()

	before := readBacklight(t)
	t.Cleanup(func() {
		want := "off"
		if before.On {
			want = "on"
		}
		mustRun(t, "backlight", want)
	})
}

// TestBacklightOn checks that asking for the light gets the light, whichever
// way round it started.
//
// This is the whole point of the command. The scanner has one key for the light
// and it toggles, so a command that pressed without looking would put the light
// out exactly half the time it was asked to switch it on.
//
// Every check here reads the state the command itself reports, and none of them
// reads it again afterwards. That is not laziness: the scanner switches its own
// light on when the squelch opens, which is to say every time it receives a
// transmission, so what the light is doing a moment later is not this command's
// doing and cannot be asserted. The report is a real reading rather than a
// claim, because the command polls the scanner until it agrees before printing
// anything.
func TestBacklightOn(t *testing.T) {
	needWrites(t)
	restoreBacklight(t)

	// The keypad setting is left out of it here, so this tests the light alone
	// and does not stop the scan. It has its own test below.
	t.Run("the light comes on", func(t *testing.T) {
		if state := lit(t, "on", "--keys=false"); !state.On {
			t.Fatal("the scanner reports itself dark after being told to light up")
		}
	})

	t.Run("asking twice leaves it on", func(t *testing.T) {
		if state := lit(t, "on", "--keys=false"); !state.On {
			t.Fatal("asking twice for the light put it out, which is the toggle showing through")
		}
	})

	t.Run("the level agrees with the light", func(t *testing.T) {
		// The level is the dimmer while lit and zero while dark. A report
		// carrying one without the other would mean the command had invented
		// half of it.
		state := lit(t, "on", "--keys=false")

		if state.On != (state.Level > 0) {
			t.Errorf("the command reports %+v, which cannot be true both ways", state)
		}
	})

	t.Run("the scanner is left scanning", func(t *testing.T) {
		if inMenu(t) {
			t.Error("switching the light left the scanner in a menu")
		}
	})
}

// TestBacklightOn_EnablesTheKeypad checks the case the two halves of this
// command exist for.
//
// With the keypad light switched off and the screen already lit, "backlight on"
// has work to do that looks like nothing: the setting has to be switched on,
// and then the light has to be cycled, because the scanner only acts on that
// setting at the moment the light comes on. Skip the cycle and the setting
// reads as enabled while the keys stay dark.
func TestBacklightOn_EnablesTheKeypad(t *testing.T) {
	needWrites(t)

	before := readKeypadLight(t)
	beforeLight := readBacklight(t)
	t.Cleanup(func() {
		want := "disable"
		if before {
			want = "enable"
		}
		mustRun(t, "backlight", "keys", want)

		state := "off"
		if beforeLight.On {
			state = "on"
		}
		mustRun(t, "backlight", state, "--keys=false")
		mustRun(t, "scan")
	})

	mustRun(t, "backlight", "keys", "disable")

	if state := lit(t, "on", "--keys=false"); !state.On {
		t.Fatal("the setup did not leave the scanner lit")
	}

	// The keypad setting is what is checked afterwards, rather than the light.
	// The setting is the scanner's own and stays put; the light does not, since
	// the scanner switches it on itself whenever it hears something.
	if state := lit(t, "on"); !state.On {
		t.Error("the scanner reports itself dark after a command that lights it up")
	}

	if !readKeypadLight(t) {
		t.Error("the keypad light was left off by a command that lights the scanner up")
	}
}

// lit runs a backlight subcommand and returns the state it reports.
func lit(t *testing.T, args ...string) backlightReport {
	t.Helper()

	var state backlightReport
	mustJSON(t, &state, append([]string{"backlight"}, args...)...)
	return state
}

// TestBacklightOff is the other half of the same toggle, and matters for the
// same reason: a command that pressed the key without looking would light the
// scanner up half the times it was asked to put it out.
func TestBacklightOff(t *testing.T) {
	needWrites(t)
	restoreBacklight(t)

	t.Run("the light goes out", func(t *testing.T) {
		if state := lit(t, "off"); state.On {
			t.Fatal("the scanner reports itself lit after being told to go dark")
		}
	})

	t.Run("asking twice leaves it off", func(t *testing.T) {
		if state := lit(t, "off"); state.On {
			t.Fatal("asking twice for the light out switched it on")
		}
	})

	t.Run("the scanner is left scanning", func(t *testing.T) {
		if inMenu(t) {
			t.Error("switching the light left the scanner in a menu")
		}
	})
}

// TestBacklightKeys checks the setting that decides whether the keypad lights
// up with the screen.
//
// It is a menu setting rather than a key, so unlike the light it survives a
// power cycle, and unlike the light it can only be read by opening the menu
// that holds it.
func TestBacklightKeys(t *testing.T) {
	needWrites(t)

	before := readKeypadLight(t)
	t.Cleanup(func() {
		want := "disable"
		if before {
			want = "enable"
		}
		mustRun(t, "backlight", "keys", want)
		mustRun(t, "scan")
	})

	t.Run("turning the keypad light off", func(t *testing.T) {
		mustRun(t, "backlight", "keys", "disable")

		if readKeypadLight(t) {
			t.Fatal("the keypad light is still on after being switched off")
		}
	})

	t.Run("turning the keypad light on", func(t *testing.T) {
		mustRun(t, "backlight", "keys", "enable")

		if !readKeypadLight(t) {
			t.Fatal("the keypad light is still off after being switched on")
		}
	})

	t.Run("setting it to what it already is", func(t *testing.T) {
		mustRun(t, "backlight", "keys", "enable")

		if !readKeypadLight(t) {
			t.Fatal("asking for the keypad light again switched it off")
		}
	})

	t.Run("the scanner is left scanning", func(t *testing.T) {
		if inMenu(t) {
			t.Error("reading the keypad light setting left the scanner in a menu")
		}
	})
}
