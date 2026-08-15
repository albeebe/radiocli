// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package navigate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
)

// stubConn is a fake device.Conn that answers the three kinds of exchange these
// walks make: reading the screen with STS, opening a menu or pressing a key
// with a command the scanner acknowledges silently, and reading a list with
// GLT.
//
// Screens are answered in order, one per read, each holding a single line that
// is highlighted. An empty name in that list makes the read fail instead, which
// is how a test puts a failure at one particular step of a walk without waiting
// for any of the menus package's poll loops to give up.
type stubConn struct {
	screens []string          // The name highlighted on each successive screen read, "" to fail that read
	reads   int               // How many screen reads have been answered so far
	docs    map[string]string // List documents, keyed by the command that asks for them
	fail    map[string]error  // Commands that fail instead of answering
}

// Info describes the fake scanner, which nothing here inspects.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute answers a screen read with the next screen, and acknowledges every
// other command the way the scanner does when it accepts one.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) {
	if err := c.fail[command]; err != nil {
		return "", err
	}

	// Menu jumps and key presses are accepted silently.
	if command != "STS" {
		return "", nil
	}

	if c.reads >= len(c.screens) {
		return "", errors.New("the scanner has no screen left to show")
	}
	name := c.screens[c.reads]
	c.reads++

	// An empty name stands for a read the scanner refuses.
	if name == "" {
		return "", errors.New("the screen cannot be read")
	}

	// One line, in the small font, holding the name and marked highlighted.
	return "0," + name + ",*", nil
}

// ExecuteXML answers a list request with the document the test supplied.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	if err := c.fail[command]; err != nil {
		return "", err
	}
	doc, ok := c.docs[command]
	if !ok {
		return "", errors.New("unexpected command: " + command)
	}
	return doc, nil
}

// Send is unused by this package and always succeeds.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close is unused by this package and always succeeds.
func (c *stubConn) Close() error { return nil }

// TestToChannels tests the ToChannels function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the walk reaches the department's channel list
//   - DepartmentError: a failure reaching the department is passed on
//   - SelectError: a menu holding no channel entry is reported
func TestToChannels(t *testing.T) {
	// Verify that naming a department by index walks to its channel list
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{"Edit Channel"}}

		if err := ToChannels(context.Background(), device.New(conn), "20"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	// Verify that a failure reaching the department is passed on unchanged
	t.Run("DepartmentError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,SCAN_DEPARTMENT,20": errors.New("the port is gone")},
		}

		if err := ToChannels(context.Background(), device.New(conn), "20"); err == nil {
			t.Error("expected an error when the department cannot be reached, got none")
		}
	})

	// Verify that a menu with no channel entry is reported rather than ignored
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := ToChannels(context.Background(), device.New(conn), "20")
		if err == nil {
			t.Fatal("expected an error when the channel entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), editChannels) {
			t.Errorf("expected the error to name %q, got: %v", editChannels, err)
		}
	})
}

// TestToDepartment tests the ToDepartment function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: an index is taken straight through and the menu opened
//   - ResolveError: a name no department carries is reported
//   - OpenMenuError: a refused menu jump is reported
func TestToDepartment(t *testing.T) {
	// Verify that an index opens the department's menu and comes back unchanged
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{}

		index, err := ToDepartment(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "20" {
			t.Errorf("expected index %q, got %q", "20", index)
		}
	})

	// Verify that a name no department carries is reported
	t.Run("ResolveError", func(t *testing.T) {
		conn := &stubConn{docs: map[string]string{
			"GLT,FL":      `<GLT><FL Name="Home" Index="1"/></GLT>`,
			"GLT,SYS,1":   `<GLT><SYS Name="Fire" Index="10"/></GLT>`,
			"GLT,DEPT,10": `<GLT><DEPT Name="Engine" Index="20"/></GLT>`,
		}}

		if _, err := ToDepartment(context.Background(), device.New(conn), "Missing"); err == nil {
			t.Error("expected an error when no department carries the name, got none")
		}
	})

	// Verify that a refused menu jump is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,SCAN_DEPARTMENT,20": errors.New("the scanner refused")},
		}

		if _, err := ToDepartment(context.Background(), device.New(conn), "20"); err == nil {
			t.Error("expected an error when the menu cannot be opened, got none")
		}
	})
}

// TestToDepartments tests the ToDepartments function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the walk reaches the system's department list
//   - SystemError: a failure reaching the system is passed on
//   - SelectError: a menu holding no department entry is reported
func TestToDepartments(t *testing.T) {
	// Verify that naming a system by index walks to its department list
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{"Edit Department"}}

		if err := ToDepartments(context.Background(), device.New(conn), "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	// Verify that a failure reaching the system is passed on unchanged
	t.Run("SystemError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,SCAN_SYSTEM,10": errors.New("the port is gone")},
		}

		if err := ToDepartments(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error when the system cannot be reached, got none")
		}
	})

	// Verify that a menu with no department entry is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := ToDepartments(context.Background(), device.New(conn), "10")
		if err == nil {
			t.Fatal("expected an error when the department entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), editDepartments) {
			t.Errorf("expected the error to name %q, got: %v", editDepartments, err)
		}
	})
}

// TestToFavorites tests the ToFavorites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the top menu is opened and the favorites entry selected
//   - OpenMenuError: a refused top menu is reported
//   - SelectError: a top menu holding no favorites entry is reported
func TestToFavorites(t *testing.T) {
	// Verify that the walk opens the top menu and selects the favorites entry
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{"Manage Favorites"}}

		if err := ToFavorites(context.Background(), device.New(conn)); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	// Verify that a refused top menu is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,TOP,": errors.New("the scanner refused")},
		}

		err := ToFavorites(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the top menu cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "top menu") {
			t.Errorf("expected the error to mention the top menu, got: %v", err)
		}
	})

	// Verify that a top menu with no favorites entry is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := ToFavorites(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the favorites entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), manageFavorites) {
			t.Errorf("expected the error to name %q, got: %v", manageFavorites, err)
		}
	})
}

// TestToFavoritesList tests the ToFavoritesList function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - Success: the named list is resolved and walked to
//   - ReadFavoritesError: a failure reading the lists is passed on
//   - ResolveError: a name no list carries is reported
//   - UnknownIndex: an index no list carries is reported
//   - BuiltIn: a built-in scan source is refused
//   - ToFavoritesError: a failure reaching the favorites menu is passed on
//   - SelectError: a favorites menu holding no such list is reported
func TestToFavoritesList(t *testing.T) {
	// The lists the scanner reports, one made by hand and one built in.
	lists := map[string]string{
		"GLT,FL": `<GLT>
			<FL Name="Home" Index="1"/>
			<FL Name="Full Database" Index="4294967295"/>
		</GLT>`,
	}

	// Verify that a named list is resolved and the walk lands on it
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{"Manage Favorites", "Home"},
			docs:    lists,
		}

		target, err := ToFavoritesList(context.Background(), device.New(conn), "Home")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if target.Name != "Home" {
			t.Errorf("expected to land on %q, got %q", "Home", target.Name)
		}
		if target.Index != "1" {
			t.Errorf("expected index %q, got %q", "1", target.Index)
		}
	})

	// Verify that a failure reading the favorites lists is passed on
	t.Run("ReadFavoritesError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"GLT,FL": errors.New("the port is gone")},
		}

		if _, err := ToFavoritesList(context.Background(), device.New(conn), "Home"); err == nil {
			t.Error("expected an error when the lists cannot be read, got none")
		}
	})

	// Verify that a name no list carries is reported before the scanner is touched
	t.Run("ResolveError", func(t *testing.T) {
		conn := &stubConn{docs: lists}

		if _, err := ToFavoritesList(context.Background(), device.New(conn), "Nowhere"); err == nil {
			t.Error("expected an error when no list carries the name, got none")
		}
	})

	// Verify that an index no list carries is reported
	t.Run("UnknownIndex", func(t *testing.T) {
		conn := &stubConn{docs: lists}

		_, err := ToFavoritesList(context.Background(), device.New(conn), "999")
		if err == nil {
			t.Fatal("expected an error when no list carries the index, got none")
		}
		if !strings.Contains(err.Error(), "999") {
			t.Errorf("expected the error to name the index, got: %v", err)
		}
	})

	// Verify that a built-in scan source is refused, having no menu of its own
	t.Run("BuiltIn", func(t *testing.T) {
		conn := &stubConn{docs: lists}

		_, err := ToFavoritesList(context.Background(), device.New(conn), "4294967295")
		if err == nil {
			t.Fatal("expected an error when the list is built in, got none")
		}
		if !strings.Contains(err.Error(), "built into the scanner") {
			t.Errorf("expected the error to say the list is built in, got: %v", err)
		}
	})

	// Verify that a failure reaching the favorites menu is passed on
	t.Run("ToFavoritesError", func(t *testing.T) {
		conn := &stubConn{
			docs: lists,
			fail: map[string]error{"MNU,TOP,": errors.New("the scanner refused")},
		}

		if _, err := ToFavoritesList(context.Background(), device.New(conn), "Home"); err == nil {
			t.Error("expected an error when the favorites menu cannot be reached, got none")
		}
	})

	// Verify that a favorites menu not holding the list is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{"Manage Favorites", ""},
			docs:    lists,
		}

		_, err := ToFavoritesList(context.Background(), device.New(conn), "Home")
		if err == nil {
			t.Fatal("expected an error when the list cannot be found, got none")
		}
	})
}

// TestToSite tests the ToSite function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: an index is taken straight through and the menu opened
//   - ResolveError: a name no site carries is reported
//   - OpenMenuError: a refused menu jump is reported
func TestToSite(t *testing.T) {
	// Verify that an index opens the site's menu and comes back unchanged
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{}

		index, err := ToSite(context.Background(), device.New(conn), "30")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "30" {
			t.Errorf("expected index %q, got %q", "30", index)
		}
	})

	// Verify that a name no site carries is reported
	t.Run("ResolveError", func(t *testing.T) {
		conn := &stubConn{docs: map[string]string{
			"GLT,FL":      `<GLT><FL Name="Home" Index="1"/></GLT>`,
			"GLT,SYS,1":   `<GLT><SYS Name="Fire" Index="10"/></GLT>`,
			"GLT,SITE,10": `<GLT><SITE Name="North" Index="30"/></GLT>`,
		}}

		if _, err := ToSite(context.Background(), device.New(conn), "Missing"); err == nil {
			t.Error("expected an error when no site carries the name, got none")
		}
	})

	// Verify that a refused menu jump is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,SCAN_SITE,30": errors.New("the scanner refused")},
		}

		if _, err := ToSite(context.Background(), device.New(conn), "30"); err == nil {
			t.Error("expected an error when the menu cannot be opened, got none")
		}
	})
}

// TestToSiteFrequencies tests the ToSiteFrequencies function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the walk reaches the site's frequency list
//   - SiteError: a failure reaching the site is passed on
//   - SelectError: a menu holding no frequency entry is reported
func TestToSiteFrequencies(t *testing.T) {
	// Verify that naming a site by index walks to its frequency list
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{"Set Frequencies"}}

		if err := ToSiteFrequencies(context.Background(), device.New(conn), "30"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	// Verify that a failure reaching the site is passed on unchanged
	t.Run("SiteError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,SCAN_SITE,30": errors.New("the port is gone")},
		}

		if err := ToSiteFrequencies(context.Background(), device.New(conn), "30"); err == nil {
			t.Error("expected an error when the site cannot be reached, got none")
		}
	})

	// Verify that a menu with no frequency entry is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := ToSiteFrequencies(context.Background(), device.New(conn), "30")
		if err == nil {
			t.Fatal("expected an error when the frequency entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), setFrequencies) {
			t.Errorf("expected the error to name %q, got: %v", setFrequencies, err)
		}
	})
}

// TestToSites tests the ToSites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the walk reaches the system's site list
//   - SystemError: a failure reaching the system is passed on
//   - SelectError: a conventional system, whose menu has no site entry, is
//     told why it failed
func TestToSites(t *testing.T) {
	// Verify that naming a system by index walks to its site list
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{screens: []string{"Edit Site"}}

		if err := ToSites(context.Background(), device.New(conn), "10"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	// Verify that a failure reaching the system is passed on unchanged
	t.Run("SystemError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,SCAN_SYSTEM,10": errors.New("the port is gone")},
		}

		if err := ToSites(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error when the system cannot be reached, got none")
		}
	})

	// Verify that a system with no site entry is told it is a conventional one
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{screens: []string{""}}

		err := ToSites(context.Background(), device.New(conn), "10")
		if err == nil {
			t.Fatal("expected an error when the site entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), "Only a trunked system has sites") {
			t.Errorf("expected the error to explain trunked systems, got: %v", err)
		}
	})
}

// TestToSystem tests the ToSystem function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: an index is taken straight through and the menu opened
//   - ResolveError: a name no system carries is reported
//   - OpenMenuError: a refused menu jump is reported
func TestToSystem(t *testing.T) {
	// Verify that an index opens the system's menu and comes back unchanged
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{}

		index, err := ToSystem(context.Background(), device.New(conn), "10")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "10" {
			t.Errorf("expected index %q, got %q", "10", index)
		}
	})

	// Verify that a name no system carries is reported
	t.Run("ResolveError", func(t *testing.T) {
		conn := &stubConn{docs: map[string]string{
			"GLT,FL":    `<GLT><FL Name="Home" Index="1"/></GLT>`,
			"GLT,SYS,1": `<GLT><SYS Name="Fire" Index="10"/></GLT>`,
		}}

		if _, err := ToSystem(context.Background(), device.New(conn), "Missing"); err == nil {
			t.Error("expected an error when no system carries the name, got none")
		}
	})

	// Verify that a refused menu jump is reported
	t.Run("OpenMenuError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"MNU,SCAN_SYSTEM,10": errors.New("the scanner refused")},
		}

		if _, err := ToSystem(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error when the menu cannot be opened, got none")
		}
	})
}

// TestToSystems tests the ToSystems function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the walk reaches the list's system list and reports the list
//   - ListError: a failure reaching the favorites list is passed on
//   - SelectError: a menu holding no system entry is reported
func TestToSystems(t *testing.T) {
	// The one list the scanner reports.
	lists := map[string]string{
		"GLT,FL": `<GLT><FL Name="Home" Index="1"/></GLT>`,
	}

	// Verify that the walk lands on the systems and reports the list it is in
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{"Manage Favorites", "Home", "Review/Edit System"},
			docs:    lists,
		}

		target, err := ToSystems(context.Background(), device.New(conn), "Home")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if target.Name != "Home" {
			t.Errorf("expected to land in %q, got %q", "Home", target.Name)
		}
	})

	// Verify that a failure reaching the favorites list is passed on
	t.Run("ListError", func(t *testing.T) {
		conn := &stubConn{
			fail: map[string]error{"GLT,FL": errors.New("the port is gone")},
		}

		if _, err := ToSystems(context.Background(), device.New(conn), "Home"); err == nil {
			t.Error("expected an error when the list cannot be reached, got none")
		}
	})

	// Verify that a list menu with no system entry is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{"Manage Favorites", "Home", ""},
			docs:    lists,
		}

		_, err := ToSystems(context.Background(), device.New(conn), "Home")
		if err == nil {
			t.Fatal("expected an error when the system entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), reviewSystems) {
			t.Errorf("expected the error to name %q, got: %v", reviewSystems, err)
		}
	})
}

// Test_resolve tests the resolve function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Index: an index is returned without reading the catalogue
//   - Success: a name is matched against the catalogue
//   - ReadError: a failure reading the catalogue is passed on
//   - NoMatch: a name the catalogue does not carry is reported
func Test_resolve(t *testing.T) {
	// The systems a catalogue read answers with.
	systems := []catalog.System{{Name: "Fire", Index: "10"}}

	// Verify that an index is taken straight through, with no catalogue read
	t.Run("Index", func(t *testing.T) {
		read := false

		index, err := resolve(context.Background(), device.New(&stubConn{}), "10", "system",
			func() ([]catalog.System, error) {
				read = true
				return systems, nil
			})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "10" {
			t.Errorf("expected index %q, got %q", "10", index)
		}

		// Reading the catalogue to answer a question the index already answers
		// is the cost this avoids.
		if read {
			t.Error("expected the catalogue never to be read for an index")
		}
	})

	// Verify that a name is matched against the catalogue and becomes an index
	t.Run("Success", func(t *testing.T) {
		index, err := resolve(context.Background(), device.New(&stubConn{}), "Fire", "system",
			func() ([]catalog.System, error) {
				return systems, nil
			})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "10" {
			t.Errorf("expected index %q, got %q", "10", index)
		}
	})

	// Verify that a failure reading the catalogue is passed on
	t.Run("ReadError", func(t *testing.T) {
		_, err := resolve(context.Background(), device.New(&stubConn{}), "Fire", "system",
			func() ([]catalog.System, error) {
				return nil, errors.New("the port is gone")
			})
		if err == nil {
			t.Error("expected an error when the catalogue cannot be read, got none")
		}
	})

	// Verify that a name the catalogue does not carry is reported by kind
	t.Run("NoMatch", func(t *testing.T) {
		_, err := resolve(context.Background(), device.New(&stubConn{}), "Missing", "system",
			func() ([]catalog.System, error) {
				return systems, nil
			})
		if err == nil {
			t.Fatal("expected an error when no entry carries the name, got none")
		}
		if !strings.Contains(err.Error(), "system") {
			t.Errorf("expected the error to name the kind, got: %v", err)
		}
	})
}
