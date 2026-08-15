// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// answeringStatus returns a scanner whose every command answers with value,
// which is the shape the status commands need.
//
// Parameters:
//   - value: the reply the scanner gives, with the echoed command already off
//
// Returns:
//   - a Scanner answering every command with value
func answeringStatus(value string) *Scanner {
	return New(&stubConn{exec: func(string) (string, error) { return value, nil }})
}

// failingStatus returns a scanner whose every command fails, for the branch
// that reports a broken exchange.
//
// Returns:
//   - a Scanner reporting a dead port for every command
func failingStatus() *Scanner {
	return New(&stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }})
}

// TestBacklight tests the Backlight method with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - On: a lit scanner reports the dimmer setting it is lit at
//   - Off: a dark scanner reports zero and reads as off
//   - Error: a failed exchange is reported
//   - ParseError: a reply the display parser refuses is reported
//   - MissingField: a reply carrying no backlight field is reported
//   - Invalid: a level that is not a number is reported
//   - OutOfRange: a level outside the dimmer's steps is reported
func TestBacklight(t *testing.T) {
	// Verify that a lit scanner reports its dimmer setting.
	t.Run("On", func(t *testing.T) {
		b, err := answeringStatus("0,SCAN,****,3").Backlight(context.Background())
		if err != nil {
			t.Fatalf("reading the backlight: %v", err)
		}
		if !b.On || b.Level != 3 {
			t.Errorf("got %+v, want on at level 3", b)
		}
	})

	// Verify that a dark scanner reads as off rather than as level zero alone.
	t.Run("Off", func(t *testing.T) {
		b, err := answeringStatus("0,SCAN,****,0").Backlight(context.Background())
		if err != nil {
			t.Fatalf("reading the backlight: %v", err)
		}
		if b.On || b.Level != 0 {
			t.Errorf("got %+v, want off", b)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().Backlight(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a reply with fewer line pairs than its form promises is
	// reported.
	t.Run("ParseError", func(t *testing.T) {
		_, err := answeringStatus("01,only").Backlight(context.Background())
		if err == nil || !strings.Contains(err.Error(), "display lines") {
			t.Fatalf("got %v, want a display lines complaint", err)
		}
	})

	// Verify that a reply carrying nothing after the screen is reported.
	t.Run("MissingField", func(t *testing.T) {
		_, err := answeringStatus("").Backlight(context.Background())
		if err == nil || !strings.Contains(err.Error(), "has no backlight") {
			t.Fatalf("got %v, want the missing field named", err)
		}
	})

	// Verify that a level that is not a number is reported.
	t.Run("Invalid", func(t *testing.T) {
		_, err := answeringStatus("0,SCAN,****,bright").Backlight(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid backlight") {
			t.Fatalf("got %v, want the field named", err)
		}
	})

	// Verify that a level the dimmer has no step for is reported.
	t.Run("OutOfRange", func(t *testing.T) {
		_, err := answeringStatus("0,SCAN,****,9").Backlight(context.Background())
		if err == nil || !strings.Contains(err.Error(), "outside 0 to 3") {
			t.Fatalf("got %v, want the range reported", err)
		}
	})
}

// TestDisplay tests the Display method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: STS is sent and its screen parsed
//   - Error: a failed exchange is reported
func TestDisplay(t *testing.T) {
	// Verify that STS is the command and the screen comes back.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "01,SCAN,          ,GREENDALE,**********", nil }}
		d, err := New(c).Display(context.Background())
		if err != nil {
			t.Fatalf("reading the display: %v", err)
		}
		if c.last() != "STS" {
			t.Errorf("sent %q, want STS", c.last())
		}
		if len(d.Lines) != 2 || d.Lines[0].Text != "SCAN" || d.Lines[1].Text != "GREENDALE" {
			t.Fatalf("got %+v, want the two lines", d.Lines)
		}
		if !d.Lines[1].Selected() {
			t.Error("the highlighted line does not report itself selected")
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().Display(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestScannerDisplayMode tests the DisplayMode method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the mode is read out of the screen's status fields
//   - Error: a failed exchange is reported
func TestScannerDisplayMode(t *testing.T) {
	// Verify that the display mode comes out of the status fields.
	t.Run("Success", func(t *testing.T) {
		got, err := answeringStatus(
			"0,SCAN,****,1,2,3,2,01555500,FM,120,01555000,01554000,01556000,2,3").
			DisplayMode(context.Background())
		if err != nil {
			t.Fatalf("reading the display mode: %v", err)
		}
		if got != DisplayWhiteBackground {
			t.Errorf("got %v, want white", got)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().DisplayMode(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestScreen tests the Screen method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: every status field lands where it belongs
//   - Error: a failed exchange is reported
//   - ParseError: a reply the display parser refuses is reported
//   - ShortReply: a reply with too few status fields is reported
func TestScreen(t *testing.T) {
	// Verify that the lights, the modes and the waterfall are all read.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) {
			return "0,SCAN,****,1,2,3,2,01555500,FM,120,01555000,01554000,01556000,1,3", nil
		}}
		scr, err := New(c).Screen(context.Background())
		if err != nil {
			t.Fatalf("reading the screen: %v", err)
		}
		if c.last() != "GST" {
			t.Errorf("sent %q, want GST", c.last())
		}
		if !scr.Muted {
			t.Error("the scanner does not report itself muted")
		}
		if scr.AlertLED != LEDRed || scr.ChargeLED != LEDMagenta {
			t.Errorf("got lights %v and %v, want red and magenta", scr.AlertLED, scr.ChargeLED)
		}
		if scr.Mode != ModeMenu || scr.DisplayMode != DisplayBlackBackground {
			t.Errorf("got mode %v and display mode %v, want menu and black", scr.Mode, scr.DisplayMode)
		}
		want := Waterfall{
			Marked:         Frequency(155550000),
			Modulation:     "FM",
			MarkerPosition: 120,
			Center:         Frequency(155500000),
			Lower:          Frequency(155400000),
			Upper:          Frequency(155600000),
			FFTSize:        3,
		}
		if scr.Waterfall != want {
			t.Errorf("got %+v, want %+v", scr.Waterfall, want)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().Screen(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a reply the display parser refuses is reported.
	t.Run("ParseError", func(t *testing.T) {
		_, err := answeringStatus("01,only").Screen(context.Background())
		if err == nil || !strings.Contains(err.Error(), "display lines") {
			t.Fatalf("got %v, want a display lines complaint", err)
		}
	})

	// Verify that a reply carrying too few status fields is reported.
	t.Run("ShortReply", func(t *testing.T) {
		_, err := answeringStatus("0,SCAN,****,1,2,3").Screen(context.Background())
		if err == nil || !strings.Contains(err.Error(), "status fields, want at least 12") {
			t.Fatalf("got %v, want the counts reported", err)
		}
	})
}

// TestDisplayModeString tests the DisplayMode String method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Named: each of the three modes reads as this tool names it
//   - Unknown: anything else says so, carrying the value
func TestDisplayModeString(t *testing.T) {
	// Verify that the three modes read as color, black and white.
	t.Run("Named", func(t *testing.T) {
		for mode, want := range map[DisplayMode]string{
			DisplayColor:           "color",
			DisplayBlackBackground: "black",
			DisplayWhiteBackground: "white",
		} {
			if got := mode.String(); got != want {
				t.Errorf("mode %d reads %q, want %q", int(mode), got, want)
			}
		}
	})

	// Verify that a value this package does not name carries itself.
	t.Run("Unknown", func(t *testing.T) {
		if got := DisplayMode(7).String(); got != "unknown(7)" {
			t.Errorf("got %q, want unknown(7)", got)
		}
	})
}

// TestLEDString tests the LED String method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Named: every colour the scanner can show has a name
//   - Negative: a negative value says so rather than reading off the table
//   - TooLarge: a value past the table says so as well
func TestLEDString(t *testing.T) {
	// Verify that every colour reads lowercased.
	t.Run("Named", func(t *testing.T) {
		for led, want := range map[LED]string{
			LEDOff:     "off",
			LEDBlue:    "blue",
			LEDRed:     "red",
			LEDMagenta: "magenta",
			LEDGreen:   "green",
			LEDCyan:    "cyan",
			LEDYellow:  "yellow",
			LEDWhite:   "white",
		} {
			if got := led.String(); got != want {
				t.Errorf("led %d reads %q, want %q", int(led), got, want)
			}
		}
	})

	// Verify that a negative value is refused rather than indexing backwards.
	t.Run("Negative", func(t *testing.T) {
		if got := LED(-1).String(); got != "unknown(-1)" {
			t.Errorf("got %q, want unknown(-1)", got)
		}
	})

	// Verify that a value past the table is refused as well.
	t.Run("TooLarge", func(t *testing.T) {
		if got := LED(8).String(); got != "unknown(8)" {
			t.Errorf("got %q, want unknown(8)", got)
		}
	})
}

// TestModeString tests the Mode String method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Named: each of the three modes reads in words
//   - Unknown: anything else says so, carrying the value
func TestModeString(t *testing.T) {
	// Verify that the three modes read as normal, waterfall and menu.
	t.Run("Named", func(t *testing.T) {
		for mode, want := range map[Mode]string{
			ModeNormal:    "normal",
			ModeWaterfall: "waterfall",
			ModeMenu:      "menu",
		} {
			if got := mode.String(); got != want {
				t.Errorf("mode %d reads %q, want %q", int(mode), got, want)
			}
		}
	})

	// Verify that a value this package does not name carries itself.
	t.Run("Unknown", func(t *testing.T) {
		if got := Mode(7).String(); got != "unknown(7)" {
			t.Errorf("got %q, want unknown(7)", got)
		}
	})
}

// Test_errMissingField tests the errMissingField function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the command and the missing field are both named
func Test_errMissingField(t *testing.T) {
	// Verify that the message names what was asked and what was missing.
	t.Run("Success", func(t *testing.T) {
		got := errMissingField("STS", "backlight").Error()
		if got != `response to "STS" has no backlight` {
			t.Errorf("got %q, want the command and field named", got)
		}
	})
}

// Test_errShortResponse tests the errShortResponse function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the command, the count carried, and the count needed are named
func Test_errShortResponse(t *testing.T) {
	// Verify that both counts are in the message.
	t.Run("Success", func(t *testing.T) {
		got := errShortResponse("GST", 4, 12, "status fields").Error()
		if got != `response to "GST" has 4 status fields, want at least 12` {
			t.Errorf("got %q, want both counts named", got)
		}
	})
}

// Test_optionalDigits tests the optionalDigits function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Fields: digits read as a number and everything else reads as zero
func Test_optionalDigits(t *testing.T) {
	// Verify that only a field of nothing but digits reads as a number.
	t.Run("Fields", func(t *testing.T) {
		for field, want := range map[string]int{
			"":         0,
			"   ":      0,
			"0":        0,
			"7":        7,
			"  120  ":  120,
			"01555500": 1555500,
			"12a":      0,
			"-1":       0,
			"12.5":     0,
			"not a なう": 0,
		} {
			if got := optionalDigits(field); got != want {
				t.Errorf("optionalDigits(%q) is %d, want %d", field, got, want)
			}
		}
	})
}
