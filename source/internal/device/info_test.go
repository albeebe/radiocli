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

// answeringXML returns a scanner whose XML commands answer with doc.
//
// Parameters:
//   - doc: the document the scanner reports
//
// Returns:
//   - a Scanner answering every XML command with doc
func answeringXML(doc string) *Scanner {
	return New(&stubConn{execXML: func(string) (string, error) { return doc, nil }})
}

// TestCloseMenu tests the CloseMenu method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the leave command is sent
//   - Error: a refusal is reported
func TestCloseMenu(t *testing.T) {
	// Verify that the command that returns to the previous mode is sent.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).CloseMenu(context.Background()); err != nil {
			t.Fatalf("closing the menu: %v", err)
		}
		if c.last() != "MSB,RETURN_PREVOUS_MODE" {
			t.Errorf("sent %q, want MSB,RETURN_PREVOUS_MODE", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).CloseMenu(context.Background()); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestHolding tests the Holding method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Held: every mode named for holding reports itself held
//   - Moving: a mode that is working through a list does not
func TestHolding(t *testing.T) {
	// Verify that matching the suffix catches every hold the scanner names.
	t.Run("Held", func(t *testing.T) {
		for _, mode := range []string{"Scan Hold", "Trunk Scan Hold", "Custom Search Hold", " WX Hold "} {
			if !(ScannerInfo{Mode: mode}).Holding() {
				t.Errorf("mode %q does not report itself holding", mode)
			}
		}
	})

	// Verify that a scanner working through a list is not holding.
	t.Run("Moving", func(t *testing.T) {
		for _, mode := range []string{"Scan Mode", "Trunk Scan", "WX Scan", ""} {
			if (ScannerInfo{Mode: mode}).Holding() {
				t.Errorf("mode %q reports itself holding", mode)
			}
		}
	})
}

// TestList tests the List method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NoIndexes: a list that needs no index is asked for by keyword alone
//   - WithIndexes: the indexes are appended in the order they were given
//   - Error: a failed exchange is reported
func TestList(t *testing.T) {
	// Verify that a list needing no index carries none.
	t.Run("NoIndexes", func(t *testing.T) {
		c := &stubConn{execXML: func(string) (string, error) { return "<GLT/>", nil }}
		if _, err := New(c).List(context.Background(), ListFavorites); err != nil {
			t.Fatalf("listing: %v", err)
		}
		if c.last() != "GLT,FL" {
			t.Errorf("sent %q, want GLT,FL", c.last())
		}
	})

	// Verify that the indexes are appended in order.
	t.Run("WithIndexes", func(t *testing.T) {
		c := &stubConn{execXML: func(string) (string, error) { return "<GLT/>", nil }}
		if _, err := New(c).List(context.Background(), ListDepartments, "12"); err != nil {
			t.Fatalf("listing: %v", err)
		}
		if c.last() != "GLT,DEPT,12" {
			t.Errorf("sent %q, want GLT,DEPT,12", c.last())
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{execXML: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).List(context.Background(), ListFavorites); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestMenuBack tests the MenuBack method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the bare back command is sent, which climbs one level
//   - Error: a refusal is reported
func TestMenuBack(t *testing.T) {
	// Verify that the bare form is sent, since that is the one that climbs.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).MenuBack(context.Background()); err != nil {
			t.Fatalf("going back: %v", err)
		}
		if c.last() != "MSB," {
			t.Errorf("sent %q, want MSB,", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).MenuBack(context.Background()); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestMenuInfo tests the MenuInfo method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the menu, its items and its input screen are read
//   - Error: a failed exchange is reported
//   - ParseError: a document that is not XML is reported
//   - NotInMenu: the scanner's own no menu answer becomes ErrNotInMenu
func TestMenuInfo(t *testing.T) {
	// Verify that the menu and everything in it is read, document kept.
	t.Run("Success", func(t *testing.T) {
		const doc = `<MenuInfo Name="Settings" Index="SETTINGS" MenuType="TypeSelect" Value="On" Selected="1">` +
			`<MenuItem Name="Set Clock" Index="CLOCK"/>` +
			`<MenuItem Name="Set Backlight" Index="BACKLIGHT"/>` +
			`<MenuInput MaxLength="16" EnableKeys="ABC"/>` +
			`</MenuInfo>`

		c := &stubConn{execXML: func(string) (string, error) { return doc, nil }}
		info, err := New(c).MenuInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the menu: %v", err)
		}
		if c.last() != "MSI" {
			t.Errorf("sent %q, want MSI", c.last())
		}
		if info.Title != "Settings" || info.Index != "SETTINGS" || info.Value != "On" || info.Selected != "1" {
			t.Errorf("got %+v, want the menu's own heading and value", info)
		}
		if len(info.Items) != 2 || info.Items[1].Name != "Set Backlight" {
			t.Fatalf("got %+v, want both entries", info.Items)
		}
		if info.Input.MaxLength != 16 || info.Input.EnableKeys != "ABC" {
			t.Errorf("got %+v, want the entry screen described", info.Input)
		}
		if info.XML != doc {
			t.Error("the document the menu was parsed from was not kept")
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{execXML: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).MenuInfo(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a document that will not parse is reported.
	t.Run("ParseError", func(t *testing.T) {
		_, err := answeringXML("<MenuInfo").MenuInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), `parsing the response to "MSI"`) {
			t.Fatalf("got %v, want a parsing complaint", err)
		}
	})

	// Verify that the scanner's own no menu answer is a normal answer.
	t.Run("NotInMenu", func(t *testing.T) {
		_, err := answeringXML(`<MenuInfo MenuType="TypeError"/>`).MenuInfo(context.Background())
		if !errors.Is(err, ErrNotInMenu) {
			t.Fatalf("got %v, want ErrNotInMenu", err)
		}
	})
}

// TestOpenMenu tests the OpenMenu method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NoIndex: a menu needing no index still carries the empty field
//   - WithIndex: a menu opened on an entry carries that entry's index
//   - Error: a refusal is reported
func TestOpenMenu(t *testing.T) {
	// Verify that the empty index field is still sent.
	t.Run("NoIndex", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).OpenMenu(context.Background(), MenuTop, ""); err != nil {
			t.Fatalf("opening the menu: %v", err)
		}
		if c.last() != "MNU,TOP," {
			t.Errorf("sent %q, want MNU,TOP,", c.last())
		}
	})

	// Verify that an index goes out where the menu expects it.
	t.Run("WithIndex", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).OpenMenu(context.Background(), MenuSystem, "12"); err != nil {
			t.Fatalf("opening the menu: %v", err)
		}
		if c.last() != "MNU,SCAN_SYSTEM,12" {
			t.Errorf("sent %q, want MNU,SCAN_SYSTEM,12", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).OpenMenu(context.Background(), MenuTop, ""); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestScannerInfoCommand tests the ScannerInfo method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the database position, the signal and the weather are read
//   - NoMenu: a document carrying no menu leaves the menu empty
//   - Error: a failed exchange is reported
//   - ParseError: a document that is not XML is reported
func TestScannerInfoCommand(t *testing.T) {
	// Verify that every part of the document lands where it belongs.
	t.Run("Success", func(t *testing.T) {
		const doc = `<ScannerInfo Mode="Scan Hold" V_Screen="scan">` +
			`<System Name="Green Bank" Index="1"/>` +
			`<Department Name="Fire" Index="2"/>` +
			`<Channel Name="Dispatch" Index="3"/>` +
			`<MenuSummary name="Top Menu" index="TOP"/>` +
			`<Property Rssi="-50" Sig="3" VOL="12" SQL="7" Mute="Unmute"/>` +
			`<SearchRange Lower="150.0000MHz" Upper="160.0000MHz" Mod="FM" Step="Auto"/>` +
			`<WxMode Mode="Weather Alert"/>` +
			`<WxChannel CH_No="7" Hold="Off" Freq=" 162.525000MHz" Mod="FM"/>` +
			`</ScannerInfo>`

		c := &stubConn{execXML: func(string) (string, error) { return doc, nil }}
		info, err := New(c).ScannerInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the scanner info: %v", err)
		}
		if c.last() != "GSI" {
			t.Errorf("sent %q, want GSI", c.last())
		}
		if info.Mode != "Scan Hold" || info.Screen != "scan" {
			t.Errorf("got mode %q and screen %q, want the scan hold", info.Mode, info.Screen)
		}
		if info.System.Name != "Green Bank" || info.Department.Name != "Fire" || info.Channel.Name != "Dispatch" {
			t.Errorf("got %+v, want the database position", info)
		}
		if info.Menu.Name != "Top Menu" || info.Menu.Index != "TOP" {
			t.Errorf("got menu %+v, want the menu summary read", info.Menu)
		}
		if info.Property.RSSI != "-50" || info.Property.Mute != "Unmute" {
			t.Errorf("got %+v, want the signal and mute state", info.Property)
		}
		if info.SearchRange.Lower != "150.0000MHz" || info.SearchRange.Step != "Auto" {
			t.Errorf("got %+v, want the search range", info.SearchRange)
		}
		if info.Weather.Mode != "Weather Alert" || info.Weather.Channel.Number != "7" {
			t.Errorf("got %+v, want the two weather elements folded into one", info.Weather)
		}
		if info.Weather.Channel.Frequency != "162.525000MHz" {
			t.Errorf("got %q, want the scanner's leading space stripped", info.Weather.Channel.Frequency)
		}
		if info.XML != doc {
			t.Error("the document the info was parsed from was not kept")
		}
	})

	// Verify that a document carrying no menu leaves the menu empty rather than
	// failing.
	t.Run("NoMenu", func(t *testing.T) {
		info, err := answeringXML(`<ScannerInfo Mode="Scan Mode"/>`).ScannerInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the scanner info: %v", err)
		}
		if info.Menu != (Named{}) {
			t.Errorf("got menu %+v, want nothing", info.Menu)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{execXML: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).ScannerInfo(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a document that will not parse is reported.
	t.Run("ParseError", func(t *testing.T) {
		_, err := answeringXML("<ScannerInfo").ScannerInfo(context.Background())
		if err == nil || !strings.Contains(err.Error(), `parsing the response to "GSI"`) {
			t.Fatalf("got %v, want a parsing complaint", err)
		}
	})
}

// TestScanning tests the Scanning method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Scanning: both names for working the favorites lists count
//   - Otherwise: every other mode does not, weather included
func TestScanning(t *testing.T) {
	// Verify that both names for the same activity count.
	t.Run("Scanning", func(t *testing.T) {
		for _, mode := range []string{"Scan Mode", "Trunk Scan", " scan mode "} {
			if !(ScannerInfo{Mode: mode}).Scanning() {
				t.Errorf("mode %q does not report itself scanning", mode)
			}
		}
	})

	// Verify that the modes that sweep something of their own do not count.
	t.Run("Otherwise", func(t *testing.T) {
		for _, mode := range []string{"Custom Search", "Close Call", "WX Scan", "Scan Hold", ""} {
			if (ScannerInfo{Mode: mode}).Scanning() {
				t.Errorf("mode %q reports itself scanning", mode)
			}
		}
	})
}

// TestSetMenuValue tests the SetMenuValue method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the value is written to the selected item
//   - Error: a refusal is reported
func TestSetMenuValue(t *testing.T) {
	// Verify that the value goes out after the command.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).SetMenuValue(context.Background(), "On"); err != nil {
			t.Fatalf("setting the value: %v", err)
		}
		if c.last() != "MSV,On" {
			t.Errorf("sent %q, want MSV,On", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).SetMenuValue(context.Background(), "On"); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestScannerInfoReadsEitherMenuSpelling covers both spellings of the menu
// attributes, because the scanner uses one on some screens and the other on
// others.
//
// The capitalised pair used to be lost entirely. ScannerInfo's own Menu field
// and the outer MenuSummary field both name the MenuSummary element, and the
// decoder hands the element to the shallower of the two, so the capitalised
// field never saw it and the lowercase field did not match it either. A menu
// reported that way came back blank, and the check that was meant to fall back
// to the lowercase spelling was reached every single time instead of only when
// it was needed.
func TestScannerInfoReadsEitherMenuSpelling(t *testing.T) {
	// Verify that the capitalised spelling is read.
	t.Run("Capitalised", func(t *testing.T) {
		const doc = `<ScannerInfo Mode="Menu tree" V_Screen="menu_selection">` +
			`<MenuSummary Name="Set Your Location" Index="LOC"/>` +
			`</ScannerInfo>`

		info, err := New(&stubConn{execXML: func(string) (string, error) { return doc, nil }}).
			ScannerInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the scanner info: %v", err)
		}
		if info.Menu.Name != "Set Your Location" || info.Menu.Index != "LOC" {
			t.Errorf("got menu %+v, want the capitalised Name and Index to be read", info.Menu)
		}
	})

	// Verify that the lowercase spelling is still read.
	t.Run("Lowercase", func(t *testing.T) {
		const doc = `<ScannerInfo Mode="Menu tree" V_Screen="menu_selection">` +
			`<MenuSummary name="Top Menu" index="TOP"/>` +
			`</ScannerInfo>`

		info, err := New(&stubConn{execXML: func(string) (string, error) { return doc, nil }}).
			ScannerInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the scanner info: %v", err)
		}
		if info.Menu.Name != "Top Menu" || info.Menu.Index != "TOP" {
			t.Errorf("got menu %+v, want the lowercase name and index to be read", info.Menu)
		}
	})

	// Verify that the capitalised spelling wins when the scanner sends both.
	t.Run("BothSpellings", func(t *testing.T) {
		const doc = `<ScannerInfo Mode="Menu tree" V_Screen="menu_selection">` +
			`<MenuSummary Name="Upper" Index="U" name="lower" index="L"/>` +
			`</ScannerInfo>`

		info, err := New(&stubConn{execXML: func(string) (string, error) { return doc, nil }}).
			ScannerInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the scanner info: %v", err)
		}
		if info.Menu.Name != "Upper" || info.Menu.Index != "U" {
			t.Errorf("got menu %+v, want the capitalised pair to win", info.Menu)
		}
	})

	// Verify that an index with no name is still read, which the old fallback
	// would have thrown away because it only ever looked at the name.
	t.Run("IndexOnly", func(t *testing.T) {
		const doc = `<ScannerInfo Mode="Menu tree" V_Screen="menu_selection">` +
			`<MenuSummary Index="LOC"/>` +
			`</ScannerInfo>`

		info, err := New(&stubConn{execXML: func(string) (string, error) { return doc, nil }}).
			ScannerInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the scanner info: %v", err)
		}
		if info.Menu.Index != "LOC" {
			t.Errorf("got menu %+v, want the capitalised Index to be read on its own", info.Menu)
		}
	})
}
