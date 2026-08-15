// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package catalog

import (
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/device"
)

// stubConn is a fake device.Conn that answers list requests from a function the
// test supplies. Only ExecuteXML is used by this package, so the rest of the
// interface is satisfied with do-nothing implementations.
type stubConn struct {
	exec func(command string) (string, error)
}

// Info describes the fake scanner, which nothing here inspects.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute is unused by this package and always answers emptily.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) { return "", nil }

// ExecuteXML answers the list request with whatever the test supplied.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return c.exec(command)
}

// Send is unused by this package and always succeeds.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close is unused by this package and always succeeds.
func (c *stubConn) Close() error { return nil }

// TestBuiltInSource tests the BuiltInSource function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - FullDatabase: the reserved database index is named
//   - SearchWithScan: the reserved search index is named
//   - OrdinaryList: an index someone's list occupies names nothing
func TestBuiltInSource(t *testing.T) {
	// Verify that the reserved database index is named
	t.Run("FullDatabase", func(t *testing.T) {
		if got := BuiltInSource("4294967295"); got != "Full Database" {
			t.Errorf("got %q, wanted the scanner's name for the database", got)
		}
	})

	// Verify that the reserved search index is named
	t.Run("SearchWithScan", func(t *testing.T) {
		if got := BuiltInSource("4261412864"); got != "Search with Scan" {
			t.Errorf("got %q, wanted the scanner's name for the search source", got)
		}
	})

	// Verify that an ordinary list's index names nothing
	t.Run("OrdinaryList", func(t *testing.T) {
		if got := BuiltInSource("1"); got != "" {
			t.Errorf("got %q, wanted nothing for a list someone created", got)
		}
	})
}

// TestCustomSearchBanks tests the CustomSearchBanks function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a document of banks becomes the banks it describes
//   - Empty: a document carrying no banks gives an empty result
//   - ParseError: a malformed document is reported rather than ignored
func TestCustomSearchBanks(t *testing.T) {
	// Verify that a bank document becomes the banks it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<CS_BANK Index="0" Name="Custom 0" Lower="025.0000" Upper="028.0000" Mod="AM" Step="5.0 kHz"/>
			<CS_BANK Index="1" Name="Custom 1" Lower="029.0000" Upper="030.0000" Mod="Auto" Step="Auto"/>
		</GLT>`

		banks, err := CustomSearchBanks(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(banks) != 2 {
			t.Fatalf("expected 2 banks, got %d", len(banks))
		}

		// Confirm every field is taken from the attribute it belongs to.
		want := CustomSearchBank{
			Index:      "0",
			Name:       "Custom 0",
			Lower:      "025.0000",
			Upper:      "028.0000",
			Modulation: "AM",
			Step:       "5.0 kHz",
		}
		if banks[0] != want {
			t.Errorf("expected %+v, got %+v", want, banks[0])
		}
	})

	// Verify that a document carrying no banks gives an empty result and no error
	t.Run("Empty", func(t *testing.T) {
		banks, err := CustomSearchBanks(`<GLT><Footer/></GLT>`)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(banks) != 0 {
			t.Errorf("expected no banks, got %d", len(banks))
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := CustomSearchBanks(`<GLT><CS_BANK/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// TestDepartments tests the Departments function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a document of departments becomes the departments it describes
//   - Empty: a document carrying no departments gives an empty result
//   - ParseError: a malformed document is reported rather than ignored
func TestDepartments(t *testing.T) {
	// Verify that a department document becomes the departments it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<DEPT Name="Test Department" Index="20" Avoid="Off" Q_Key="4"/>
			<DEPT Name="Avoided Department" Index="21" Avoid="On" Q_Key="None"/>
		</GLT>`

		departments, err := Departments(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(departments) != 2 {
			t.Fatalf("expected 2 departments, got %d", len(departments))
		}

		// A department the scanner is not skipping keeps its quick key.
		want := Department{Name: "Test Department", Index: "20", Avoided: false, QuickKey: "4"}
		if departments[0] != want {
			t.Errorf("expected %+v, got %+v", want, departments[0])
		}

		// An avoided department with no quick key reports both.
		want = Department{Name: "Avoided Department", Index: "21", Avoided: true, QuickKey: ""}
		if departments[1] != want {
			t.Errorf("expected %+v, got %+v", want, departments[1])
		}
	})

	// Verify that a document carrying no departments gives an empty result
	t.Run("Empty", func(t *testing.T) {
		departments, err := Departments(`<GLT><Footer/></GLT>`)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(departments) != 0 {
			t.Errorf("expected no departments, got %d", len(departments))
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := Departments(`<GLT><DEPT/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// TestElements tests the Elements function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Success: every element of the wanted name is collected
//   - Empty: a document holding nothing wanted gives an empty result
//   - UnknownElements: element names outside the known set are ignored
//   - WrongKind: a document of another known kind is refused
//   - WrongKindRemembersFirst: only the first other kind is named
//   - FoundWithOtherKinds: another kind alongside the wanted one is not an error
//   - InvalidXML: a malformed document is reported rather than ignored
func TestElements(t *testing.T) {
	// Verify that every element of the wanted name is collected with its attributes
	t.Run("Success", func(t *testing.T) {
		doc := `<?xml version="1.0" encoding="utf-8"?>
		<GLT>
			<FL Name="Test List" Index="1"/>
			<FL Name="Second List" Index="2"/>
		</GLT>`

		elements, err := Elements(doc, "FL")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(elements) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(elements))
		}
		if elements[0]["Name"] != "Test List" || elements[0]["Index"] != "1" {
			t.Errorf("expected the first list's attributes, got %+v", elements[0])
		}
	})

	// Verify that a document holding nothing of the wanted name is not an error
	t.Run("Empty", func(t *testing.T) {
		elements, err := Elements(`<GLT></GLT>`, "FL")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(elements) != 0 {
			t.Errorf("expected no elements, got %d", len(elements))
		}
	})

	// Verify that element names outside the known set do not count as a wrong answer
	t.Run("UnknownElements", func(t *testing.T) {
		elements, err := Elements(`<GLT><Footer Count="0"/></GLT>`, "FL")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(elements) != 0 {
			t.Errorf("expected no elements, got %d", len(elements))
		}
	})

	// Verify that being handed a list of another known kind is refused
	t.Run("WrongKind", func(t *testing.T) {
		_, err := Elements(`<GLT><SYS Name="Test System" Index="10"/></GLT>`, "FL")
		if err == nil {
			t.Fatal("expected an error for a list of the wrong kind, got none")
		}
		if !strings.Contains(err.Error(), "SYS") || !strings.Contains(err.Error(), "FL") {
			t.Errorf("expected the message to name both kinds, got: %v", err)
		}
	})

	// Verify that only the first other kind is named when several are present
	t.Run("WrongKindRemembersFirst", func(t *testing.T) {
		doc := `<GLT><SYS Index="10"/><DEPT Index="20"/></GLT>`

		_, err := Elements(doc, "FL")
		if err == nil {
			t.Fatal("expected an error for a list of the wrong kind, got none")
		}
		if !strings.Contains(err.Error(), "SYS") {
			t.Errorf("expected the message to name the first other kind, got: %v", err)
		}
		if strings.Contains(err.Error(), "DEPT") {
			t.Errorf("expected the message to name only the first other kind, got: %v", err)
		}
	})

	// Verify that another kind alongside the wanted one is not an error
	t.Run("FoundWithOtherKinds", func(t *testing.T) {
		doc := `<GLT><FL Name="Test List" Index="1"/><SYS Index="10"/></GLT>`

		elements, err := Elements(doc, "FL")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(elements) != 1 {
			t.Errorf("expected 1 element, got %d", len(elements))
		}
	})

	// Verify that a document that is not valid XML is reported as an error
	t.Run("InvalidXML", func(t *testing.T) {
		_, err := Elements(`<GLT><FL/></BAD>`, "FL")
		if err == nil {
			t.Fatal("expected an error for a malformed document, got none")
		}
		if !strings.Contains(err.Error(), "not valid XML") {
			t.Errorf("expected the message to say the answer is not valid XML, got: %v", err)
		}
	})
}

// TestEveryDepartment tests the EveryDepartment function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the departments of every system are gathered together
//   - SystemsError: a failure reading the systems is passed on
//   - DepartmentsError: a failure reading one system's departments is passed on
func TestEveryDepartment(t *testing.T) {
	// Verify that the departments of every system are gathered into one result
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			switch command {
			case "GLT,FL":
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/><SYS Name="Other System" Index="11"/></GLT>`, nil
			case "GLT,DEPT,10":
				return `<GLT><DEPT Name="First Department" Index="20"/></GLT>`, nil
			case "GLT,DEPT,11":
				return `<GLT><DEPT Name="Second Department" Index="21"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		departments, err := EveryDepartment(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(departments) != 2 {
			t.Fatalf("expected 2 departments, got %d", len(departments))
		}
		if departments[0].Name != "First Department" || departments[1].Name != "Second Department" {
			t.Errorf("expected both systems' departments in order, got %+v", departments)
		}
	})

	// Verify that a failure reading the systems is passed on
	t.Run("SystemsError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := EveryDepartment(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that a failure reading one system's departments is passed on
	t.Run("DepartmentsError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			switch command {
			case "GLT,FL":
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/></GLT>`, nil
			}
			return "", errors.New("the port is gone")
		}}

		if _, err := EveryDepartment(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the departments cannot be read, got none")
		}
	})
}

// TestEverySite tests the EverySite function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the sites of every system are gathered together
//   - SystemsError: a failure reading the systems is passed on
//   - SitesError: a failure reading one system's sites is passed on
func TestEverySite(t *testing.T) {
	// Verify that the sites of every system are gathered into one result
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			switch command {
			case "GLT,FL":
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/><SYS Name="Other System" Index="11"/></GLT>`, nil
			case "GLT,SITE,10":
				return `<GLT><SITE Name="First Site" Index="50"/></GLT>`, nil
			case "GLT,SITE,11":
				return `<GLT><Footer/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		sites, err := EverySite(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(sites) != 1 {
			t.Fatalf("expected 1 site, got %d", len(sites))
		}
		if sites[0].Name != "First Site" {
			t.Errorf("expected the trunked system's site, got %+v", sites[0])
		}
	})

	// Verify that a failure reading the systems is passed on
	t.Run("SystemsError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := EverySite(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that a failure reading one system's sites is passed on
	t.Run("SitesError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			switch command {
			case "GLT,FL":
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/></GLT>`, nil
			}
			return "", errors.New("the port is gone")
		}}

		if _, err := EverySite(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the sites cannot be read, got none")
		}
	})
}

// TestEverySystem tests the EverySystem function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - SkipsBuiltIn: the built-in scan sources are never asked about
//   - FavoritesError: a failure reading the favorites lists is passed on
//   - SystemsError: a failure reading one list's systems is passed on
func TestEverySystem(t *testing.T) {
	// Verify that the built-in scan sources are skipped and the rest are read
	t.Run("SkipsBuiltIn", func(t *testing.T) {
		asked := make(map[string]bool)
		conn := &stubConn{exec: func(command string) (string, error) {
			asked[command] = true
			switch command {
			case "GLT,FL":
				return `<GLT>
					<FL Name="Test List" Index="1"/>
					<FL Name="Full Database" Index="4294967295"/>
					<FL Name="Search with Scan" Index="4261412864"/>
				</GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		systems, err := EverySystem(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(systems) != 1 {
			t.Fatalf("expected 1 system, got %d", len(systems))
		}

		// The flood the full database would answer with is what this avoids.
		if asked["GLT,SYS,4294967295"] || asked["GLT,SYS,4261412864"] {
			t.Error("expected the built-in scan sources never to be asked about")
		}
	})

	// Verify that a failure reading the favorites lists is passed on
	t.Run("FavoritesError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := EverySystem(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the favorites lists cannot be read, got none")
		}
	})

	// Verify that a failure reading one list's systems is passed on
	t.Run("SystemsError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,FL" {
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			}
			return "", errors.New("the port is gone")
		}}

		if _, err := EverySystem(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})
}

// TestFavorites tests the Favorites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a favorites document becomes the lists it describes
//   - BuiltIn: the two reserved indexes are marked as built-in
//   - ParseError: a malformed document is reported rather than ignored
func TestFavorites(t *testing.T) {
	// Verify that a favorites document becomes the lists it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<FL Name="Test List" Index="1" Monitor="On" Q_Key="1" N_Tag="None"/>
			<FL Name="Second List" Index="2" Monitor="Off" Q_Key="None" N_Tag="7"/>
		</GLT>`

		lists, err := Favorites(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(lists) != 2 {
			t.Fatalf("expected 2 lists, got %d", len(lists))
		}

		// A monitored list with a quick key and no number tag.
		want := FavoritesList{Name: "Test List", Index: "1", Monitored: true, QuickKey: "1", NumberTag: "", BuiltIn: false}
		if lists[0] != want {
			t.Errorf("expected %+v, got %+v", want, lists[0])
		}

		// An unmonitored list with a number tag and no quick key.
		want = FavoritesList{Name: "Second List", Index: "2", Monitored: false, QuickKey: "", NumberTag: "7", BuiltIn: false}
		if lists[1] != want {
			t.Errorf("expected %+v, got %+v", want, lists[1])
		}
	})

	// Verify that the two reserved indexes are marked as built-in scan sources
	t.Run("BuiltIn", func(t *testing.T) {
		doc := `<GLT>
			<FL Name="Full Database" Index="4294967295" Monitor="Off"/>
			<FL Name="Search with Scan" Index="4261412864" Monitor="Off"/>
		</GLT>`

		lists, err := Favorites(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(lists) != 2 {
			t.Fatalf("expected 2 lists, got %d", len(lists))
		}
		if !lists[0].BuiltIn || !lists[1].BuiltIn {
			t.Errorf("expected both reserved indexes to be built-in, got %+v", lists)
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := Favorites(`<GLT><FL/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// TestFrequencies tests the Frequencies function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a frequency document becomes the channels it describes
//   - Empty: a document carrying no frequencies gives an empty result
//   - ParseError: a malformed document is reported rather than ignored
func TestFrequencies(t *testing.T) {
	// Verify that a frequency document becomes the channels it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<CFREQ Name="Test Channel" Index="30" Freq="146.5200" Avoid="Off"/>
			<CFREQ Name="Avoided Channel" Index="31" Freq="147.0000" Avoid="On"/>
		</GLT>`

		channels, err := Frequencies(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(channels) != 2 {
			t.Fatalf("expected 2 channels, got %d", len(channels))
		}

		// A conventional channel carries a frequency and no talkgroup.
		want := Channel{Name: "Test Channel", Index: "30", Frequency: "146.5200", Talkgroup: "", Avoided: false}
		if channels[0] != want {
			t.Errorf("expected %+v, got %+v", want, channels[0])
		}
		if !channels[1].Avoided {
			t.Errorf("expected the second channel to be avoided, got %+v", channels[1])
		}
	})

	// Verify that a document carrying no frequencies gives an empty result
	t.Run("Empty", func(t *testing.T) {
		channels, err := Frequencies(`<GLT><Footer/></GLT>`)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(channels) != 0 {
			t.Errorf("expected no channels, got %d", len(channels))
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := Frequencies(`<GLT><CFREQ/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// TestElement_Get tests the Element.Get method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Present: an attribute the element carries is returned
//   - Padded: surrounding space is removed
//   - Missing: an attribute the element does not carry gives ""
func TestElement_Get(t *testing.T) {
	// Verify that an attribute the element carries is returned
	t.Run("Present", func(t *testing.T) {
		e := Element{"Name": "Test List"}
		if got := e.Get("Name"); got != "Test List" {
			t.Errorf("expected \"Test List\", got %q", got)
		}
	})

	// Verify that the space the scanner pads a value with is removed
	t.Run("Padded", func(t *testing.T) {
		e := Element{"Freq": "  146.5200  "}
		if got := e.Get("Freq"); got != "146.5200" {
			t.Errorf("expected \"146.5200\", got %q", got)
		}
	})

	// Verify that an attribute the element does not carry reads as empty
	t.Run("Missing", func(t *testing.T) {
		e := Element{}
		if got := e.Get("Name"); got != "" {
			t.Errorf("expected \"\", got %q", got)
		}
	})
}

// TestElement_Is tests the Element.Is method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Match: an attribute equal to the value reports true
//   - MatchIgnoringCase: firmware spelling differences still match
//   - NoMatch: a different value reports false
func TestElement_Is(t *testing.T) {
	// Verify that an attribute equal to the value reports true
	t.Run("Match", func(t *testing.T) {
		e := Element{"Monitor": "On"}
		if !e.Is("Monitor", "On") {
			t.Error("expected the attribute to match")
		}
	})

	// Verify that a difference in case still counts as a match
	t.Run("MatchIgnoringCase", func(t *testing.T) {
		e := Element{"Monitor": "ON"}
		if !e.Is("Monitor", "on") {
			t.Error("expected the attribute to match ignoring case")
		}
	})

	// Verify that a different value reports false
	t.Run("NoMatch", func(t *testing.T) {
		e := Element{"Monitor": "Off"}
		if e.Is("Monitor", "On") {
			t.Error("expected the attribute not to match")
		}
	})
}

// TestIsIndex tests the IsIndex function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Digits: a run of digits is an index
//   - Empty: an empty value is not an index
//   - Name: a name is not an index
//   - Mixed: digits with anything else are not an index
func TestIsIndex(t *testing.T) {
	// Verify that a run of digits is treated as an index
	t.Run("Digits", func(t *testing.T) {
		if !IsIndex("4294967295") {
			t.Error("expected a run of digits to be an index")
		}
	})

	// Verify that an empty value is not an index
	t.Run("Empty", func(t *testing.T) {
		if IsIndex("") {
			t.Error("expected an empty value not to be an index")
		}
	})

	// Verify that a name is not an index
	t.Run("Name", func(t *testing.T) {
		if IsIndex("Test System") {
			t.Error("expected a name not to be an index")
		}
	})

	// Verify that digits mixed with anything else are not an index
	t.Run("Mixed", func(t *testing.T) {
		if IsIndex("12a") {
			t.Error("expected a mixed value not to be an index")
		}
	})
}

// TestElement_Optional tests the Element.Optional method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Assigned: a value that is assigned is returned
//   - None: the scanner's word for nothing assigned reads as empty
//   - NoneIgnoringCase: that word is recognised whatever its case
func TestElement_Optional(t *testing.T) {
	// Verify that a value that is assigned is returned unchanged
	t.Run("Assigned", func(t *testing.T) {
		e := Element{"Q_Key": "1"}
		if got := e.Optional("Q_Key"); got != "1" {
			t.Errorf("expected \"1\", got %q", got)
		}
	})

	// Verify that the scanner's word for nothing assigned reads as empty
	t.Run("None", func(t *testing.T) {
		e := Element{"Q_Key": "None"}
		if got := e.Optional("Q_Key"); got != "" {
			t.Errorf("expected \"\", got %q", got)
		}
	})

	// Verify that the word is recognised whatever case the firmware spells it in
	t.Run("NoneIgnoringCase", func(t *testing.T) {
		e := Element{"N_Tag": "NONE"}
		if got := e.Optional("N_Tag"); got != "" {
			t.Errorf("expected \"\", got %q", got)
		}
	})
}

// TestReadChannels tests the ReadChannels function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Frequencies: a conventional department answers with its frequencies
//   - Talkgroups: a trunked department answers with its talkgroups
//   - Empty: a department holding neither is not an error
//   - FrequencyListError: a failed frequency exchange is reported
//   - FrequencyParseError: a malformed frequency answer is reported
//   - TalkgroupListError: a failed talkgroup exchange is reported
//   - TalkgroupParseError: a malformed talkgroup answer is reported
func TestReadChannels(t *testing.T) {
	// Verify that a conventional department answers with its frequencies
	t.Run("Frequencies", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,CFREQ,20" {
				return `<GLT><CFREQ Name="Test Channel" Index="30" Freq="146.5200" Avoid="Off"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		channels, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(channels) != 1 || channels[0].Frequency != "146.5200" {
			t.Errorf("expected the department's frequency, got %+v", channels)
		}
	})

	// Verify that a department with no frequencies is asked for its talkgroups
	t.Run("Talkgroups", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			switch command {
			case "GLT,CFREQ,20":
				return `<GLT><Footer/></GLT>`, nil
			case "GLT,TGID,20":
				return `<GLT><TGID Name="Test Talkgroup" Index="40" TGID="1001" Avoid="Off"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		channels, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(channels) != 1 || channels[0].Talkgroup != "1001" {
			t.Errorf("expected the department's talkgroup, got %+v", channels)
		}
	})

	// Verify that a department holding neither kind is not an error
	t.Run("Empty", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><Footer/></GLT>`, nil
		}}

		channels, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(channels) != 0 {
			t.Errorf("expected no channels, got %d", len(channels))
		}
	})

	// Verify that a failed frequency exchange is reported
	t.Run("FrequencyListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the channels") {
			t.Errorf("expected the message to say it was reading the channels, got: %v", err)
		}
	})

	// Verify that a malformed frequency answer is reported
	t.Run("FrequencyParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><CFREQ/></BAD>`, nil
		}}

		_, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err == nil {
			t.Fatal("expected an error for a malformed answer, got none")
		}
		if !strings.Contains(err.Error(), "reading the channels") {
			t.Errorf("expected the message to say it was reading the channels, got: %v", err)
		}
	})

	// Verify that a failed talkgroup exchange is reported
	t.Run("TalkgroupListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,CFREQ,20" {
				return `<GLT><Footer/></GLT>`, nil
			}
			return "", errors.New("the port is gone")
		}}

		_, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the talkgroups") {
			t.Errorf("expected the message to say it was reading the talkgroups, got: %v", err)
		}
	})

	// Verify that a malformed talkgroup answer is reported
	t.Run("TalkgroupParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,CFREQ,20" {
				return `<GLT><Footer/></GLT>`, nil
			}
			return `<GLT><TGID/></BAD>`, nil
		}}

		_, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err == nil {
			t.Fatal("expected an error for a malformed answer, got none")
		}
		if !strings.Contains(err.Error(), "reading the talkgroups") {
			t.Errorf("expected the message to say it was reading the talkgroups, got: %v", err)
		}
	})
}

// TestReadCustomSearchBanks tests the ReadCustomSearchBanks function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the scanner's banks are read and parsed
//   - ListError: a failed exchange is reported
//   - ParseError: a malformed answer is reported
func TestReadCustomSearchBanks(t *testing.T) {
	// Verify that the scanner's banks are read and parsed
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,CS_BANK" {
				return `<GLT><CS_BANK Index="0" Name="Custom 0" Lower="025.0000" Upper="028.0000" Mod="AM" Step="5.0 kHz"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		banks, err := ReadCustomSearchBanks(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(banks) != 1 || banks[0].Name != "Custom 0" {
			t.Errorf("expected the scanner's bank, got %+v", banks)
		}
	})

	// Verify that a failed exchange is reported
	t.Run("ListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := ReadCustomSearchBanks(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the custom search banks") {
			t.Errorf("expected the message to name what was being read, got: %v", err)
		}
	})

	// Verify that a malformed answer is reported
	t.Run("ParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><CS_BANK/></BAD>`, nil
		}}

		if _, err := ReadCustomSearchBanks(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error for a malformed answer, got none")
		}
	})
}

// TestReadDepartments tests the ReadDepartments function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: one system's departments are read and parsed
//   - ListError: a failed exchange is reported
//   - ParseError: a malformed answer is reported
func TestReadDepartments(t *testing.T) {
	// Verify that one system's departments are read and parsed
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,DEPT,10" {
				return `<GLT><DEPT Name="Test Department" Index="20" Avoid="Off"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		departments, err := ReadDepartments(context.Background(), device.New(conn), "10")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(departments) != 1 || departments[0].Name != "Test Department" {
			t.Errorf("expected the system's department, got %+v", departments)
		}
	})

	// Verify that a failed exchange is reported
	t.Run("ListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := ReadDepartments(context.Background(), device.New(conn), "10")
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the departments") {
			t.Errorf("expected the message to name what was being read, got: %v", err)
		}
	})

	// Verify that a malformed answer is reported
	t.Run("ParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><DEPT/></BAD>`, nil
		}}

		if _, err := ReadDepartments(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error for a malformed answer, got none")
		}
	})
}

// TestReadFavorites tests the ReadFavorites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the scanner's favorites lists are read and parsed
//   - ListError: a failed exchange is reported
//   - ParseError: a malformed answer is reported
func TestReadFavorites(t *testing.T) {
	// Verify that the scanner's favorites lists are read and parsed
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,FL" {
				return `<GLT><FL Name="Test List" Index="1" Monitor="On"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		lists, err := ReadFavorites(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(lists) != 1 || lists[0].Name != "Test List" {
			t.Errorf("expected the scanner's list, got %+v", lists)
		}
	})

	// Verify that a failed exchange is reported
	t.Run("ListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := ReadFavorites(context.Background(), device.New(conn))
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the favorites lists") {
			t.Errorf("expected the message to name what was being read, got: %v", err)
		}
	})

	// Verify that a malformed answer is reported
	t.Run("ParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><FL/></BAD>`, nil
		}}

		if _, err := ReadFavorites(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error for a malformed answer, got none")
		}
	})
}

// TestReadSiteFrequencies tests the ReadSiteFrequencies function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: one site's frequencies are read and parsed
//   - ListError: a failed exchange is reported
//   - ParseError: a malformed answer is reported
func TestReadSiteFrequencies(t *testing.T) {
	// Verify that one site's frequencies are read and parsed
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,SFREQ,50" {
				return `<GLT><SFREQ Freq="851.0125" Index="60"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		frequencies, err := ReadSiteFrequencies(context.Background(), device.New(conn), "50")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(frequencies) != 1 || frequencies[0].Frequency != "851.0125" {
			t.Errorf("expected the site's frequency, got %+v", frequencies)
		}
	})

	// Verify that a failed exchange is reported
	t.Run("ListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := ReadSiteFrequencies(context.Background(), device.New(conn), "50")
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the site's frequencies") {
			t.Errorf("expected the message to name what was being read, got: %v", err)
		}
	})

	// Verify that a malformed answer is reported
	t.Run("ParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><SFREQ/></BAD>`, nil
		}}

		if _, err := ReadSiteFrequencies(context.Background(), device.New(conn), "50"); err == nil {
			t.Error("expected an error for a malformed answer, got none")
		}
	})
}

// TestReadSites tests the ReadSites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: one system's sites are read and parsed
//   - ListError: a failed exchange is reported
//   - ParseError: a malformed answer is reported
func TestReadSites(t *testing.T) {
	// Verify that one system's sites are read and parsed
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,SITE,10" {
				return `<GLT><SITE Name="Test Site" Index="50" Avoid="Off"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		sites, err := ReadSites(context.Background(), device.New(conn), "10")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(sites) != 1 || sites[0].Name != "Test Site" {
			t.Errorf("expected the system's site, got %+v", sites)
		}
	})

	// Verify that a failed exchange is reported
	t.Run("ListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := ReadSites(context.Background(), device.New(conn), "10")
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the sites") {
			t.Errorf("expected the message to name what was being read, got: %v", err)
		}
	})

	// Verify that a malformed answer is reported
	t.Run("ParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><SITE/></BAD>`, nil
		}}

		if _, err := ReadSites(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error for a malformed answer, got none")
		}
	})
}

// TestReadSystems tests the ReadSystems function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Success: one favorites list's systems are read and parsed
//   - ListError: a failed exchange is reported
//   - ParseError: a malformed answer is reported
//   - FullDatabase: the database is refused without anything being sent
//   - SearchWithScan: the search source is refused without anything being sent
func TestReadSystems(t *testing.T) {
	// Verify that one favorites list's systems are read and parsed
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "GLT,SYS,1" {
				return `<GLT><SYS Name="Test System" Index="10" Type="Conventional" Avoid="Off"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}

		systems, err := ReadSystems(context.Background(), device.New(conn), "1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(systems) != 1 || systems[0].Name != "Test System" {
			t.Errorf("expected the list's system, got %+v", systems)
		}
	})

	// Verify that a failed exchange is reported
	t.Run("ListError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, err := ReadSystems(context.Background(), device.New(conn), "1")
		if err == nil {
			t.Fatal("expected an error when the exchange fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the systems") {
			t.Errorf("expected the message to name what was being read, got: %v", err)
		}
	})

	// Verify that a malformed answer is reported
	t.Run("ParseError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return `<GLT><SYS/></BAD>`, nil
		}}

		if _, err := ReadSystems(context.Background(), device.New(conn), "1"); err == nil {
			t.Error("expected an error for a malformed answer, got none")
		}
	})

	// Verify that the full database is refused before anything reaches the wire,
	// since the request it would send is the one that costs a power cycle
	t.Run("FullDatabase", func(t *testing.T) {
		sent := false
		conn := &stubConn{exec: func(command string) (string, error) {
			sent = true
			return "", nil
		}}

		_, err := ReadSystems(context.Background(), device.New(conn), "4294967295")
		if err == nil {
			t.Fatal("expected the full database to be refused, it was not")
		}
		if sent {
			t.Error("the request was sent to the scanner despite being refused")
		}
		if !strings.Contains(err.Error(), "power cycled") {
			t.Errorf("expected the message to say what it costs, got: %v", err)
		}
	})

	// Verify that the search source is refused too, for its own reason
	t.Run("SearchWithScan", func(t *testing.T) {
		sent := false
		conn := &stubConn{exec: func(command string) (string, error) {
			sent = true
			return "", nil
		}}

		_, err := ReadSystems(context.Background(), device.New(conn), "4261412864")
		if err == nil {
			t.Fatal("expected the search source to be refused, it was not")
		}
		if sent {
			t.Error("the request was sent to the scanner despite being refused")
		}
		if !strings.Contains(err.Error(), "holds no systems of its own") {
			t.Errorf("expected the message to say why there is nothing to read, got: %v", err)
		}
	})
}

// TestResolve tests the Resolve function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Index: a run of digits is returned without consulting the entries
//   - Name: a name is matched to its index
//   - NameIgnoringCase: a name is matched without regard to case
//   - NoMatch: a name nothing carries is refused, listing what there is
//   - NoEntries: a name is refused differently when there is nothing at all
//   - Ambiguous: a name two entries share is refused rather than guessed at
func TestResolve(t *testing.T) {
	// Verify that a run of digits is returned unchanged
	t.Run("Index", func(t *testing.T) {
		index, err := Resolve("10", "system", []System(nil))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "10" {
			t.Errorf("expected \"10\", got %q", index)
		}
	})

	// Verify that a name is matched to the index the scanner knows it by
	t.Run("Name", func(t *testing.T) {
		all := []System{
			{Name: "Test System", Index: "10"},
			{Name: "Other System", Index: "11"},
		}

		index, err := Resolve("Other System", "system", all)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "11" {
			t.Errorf("expected \"11\", got %q", index)
		}
	})

	// Verify that a name is matched without regard to case
	t.Run("NameIgnoringCase", func(t *testing.T) {
		all := []Department{{Name: "Test Department", Index: "20"}}

		index, err := Resolve("test department", "department", all)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "20" {
			t.Errorf("expected \"20\", got %q", index)
		}
	})

	// Verify that a name nothing carries is refused, listing what there is
	t.Run("NoMatch", func(t *testing.T) {
		all := []FavoritesList{{Name: "Test List", Index: "1"}}

		_, err := Resolve("Missing List", "favorites list", all)
		if err == nil {
			t.Fatal("expected an error for a name nothing carries, got none")
		}
		if !strings.Contains(err.Error(), "Test List") {
			t.Errorf("expected the message to list what there is, got: %v", err)
		}
	})

	// Verify that a name is refused differently when the scanner reports nothing
	t.Run("NoEntries", func(t *testing.T) {
		_, err := Resolve("Missing Site", "site", []Site{})
		if err == nil {
			t.Fatal("expected an error when there is nothing at all, got none")
		}
		if !strings.Contains(err.Error(), "none at all") {
			t.Errorf("expected the message to say there are none at all, got: %v", err)
		}
	})

	// Verify that a name two entries share is refused rather than guessed at
	t.Run("Ambiguous", func(t *testing.T) {
		all := []System{
			{Name: "Test System", Index: "10"},
			{Name: "Test System", Index: "11"},
		}

		_, err := Resolve("Test System", "system", all)
		if err == nil {
			t.Fatal("expected an error for an ambiguous name, got none")
		}
		if !strings.Contains(err.Error(), "name one by its index instead") {
			t.Errorf("expected the message to suggest naming it by index, got: %v", err)
		}
	})
}

// TestResolveDepartment tests the ResolveDepartment function with 100%
// coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Index: an index is answered without reading anything at all
//   - Name: a name is matched to the index the scanner knows it by
//   - ReadError: a failure reading the departments is passed on
func TestResolveDepartment(t *testing.T) {
	// The scanner behind a name lookup: one list, one system, one department.
	answering := func(asked map[string]bool) *stubConn {
		return &stubConn{exec: func(command string) (string, error) {
			asked[command] = true
			switch command {
			case "GLT,FL":
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/></GLT>`, nil
			case "GLT,DEPT,10":
				return `<GLT><DEPT Name="Test Department" Index="20"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}
	}

	// Verify that an index costs no exchange. This is the whole reason these
	// wrappers exist: a department name is the most expensive lookup in the
	// tool, and an index must not pay for it.
	t.Run("Index", func(t *testing.T) {
		asked := map[string]bool{}

		index, err := ResolveDepartment(context.Background(), device.New(answering(asked)), "20")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "20" {
			t.Errorf("expected \"20\", got %q", index)
		}
		if len(asked) != 0 {
			t.Errorf("expected an index to be answered without reading, got: %v", asked)
		}
	})

	// Verify that a name is matched to the index the scanner knows it by
	t.Run("Name", func(t *testing.T) {
		client := device.New(answering(map[string]bool{}))

		index, err := ResolveDepartment(context.Background(), client, "test department")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "20" {
			t.Errorf("expected \"20\", got %q", index)
		}
	})

	// Verify that a failed read is passed on rather than reported as a name
	// nothing carries
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := ResolveDepartment(context.Background(), device.New(conn), "ANY"); err == nil {
			t.Error("expected an error when the departments cannot be read, got none")
		}
	})
}

// TestResolveFavorites tests the ResolveFavorites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Index: an index is answered without reading anything at all
//   - Name: a name is matched to the index the scanner knows it by
//   - ReadError: a failure reading the lists is passed on
func TestResolveFavorites(t *testing.T) {
	lists := func(asked map[string]bool) *stubConn {
		return &stubConn{exec: func(command string) (string, error) {
			asked[command] = true
			if command == "GLT,FL" {
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}
	}

	// Verify that an index costs no exchange
	t.Run("Index", func(t *testing.T) {
		asked := map[string]bool{}

		index, err := ResolveFavorites(context.Background(), device.New(lists(asked)), "1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "1" || len(asked) != 0 {
			t.Errorf("expected \"1\" and no reads, got %q and %v", index, asked)
		}
	})

	// Verify that a name is matched to the index the scanner knows it by
	t.Run("Name", func(t *testing.T) {
		client := device.New(lists(map[string]bool{}))

		index, err := ResolveFavorites(context.Background(), client, "Test List")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "1" {
			t.Errorf("expected \"1\", got %q", index)
		}
	})

	// Verify that a failed read is passed on
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := ResolveFavorites(context.Background(), device.New(conn), "ANY"); err == nil {
			t.Error("expected an error when the lists cannot be read, got none")
		}
	})
}

// TestResolveSite tests the ResolveSite function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Index: an index is answered without reading anything at all
//   - Name: a name is matched to the index the scanner knows it by
//   - ReadError: a failure reading the sites is passed on
func TestResolveSite(t *testing.T) {
	sites := func(asked map[string]bool) *stubConn {
		return &stubConn{exec: func(command string) (string, error) {
			asked[command] = true
			switch command {
			case "GLT,FL":
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/></GLT>`, nil
			case "GLT,SITE,10":
				return `<GLT><SITE Name="Test Site" Index="30"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}
	}

	// Verify that an index costs no exchange
	t.Run("Index", func(t *testing.T) {
		asked := map[string]bool{}

		index, err := ResolveSite(context.Background(), device.New(sites(asked)), "30")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "30" || len(asked) != 0 {
			t.Errorf("expected \"30\" and no reads, got %q and %v", index, asked)
		}
	})

	// Verify that a name is matched to the index the scanner knows it by
	t.Run("Name", func(t *testing.T) {
		client := device.New(sites(map[string]bool{}))

		index, err := ResolveSite(context.Background(), client, "Test Site")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "30" {
			t.Errorf("expected \"30\", got %q", index)
		}
	})

	// Verify that a failed read is passed on
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := ResolveSite(context.Background(), device.New(conn), "ANY"); err == nil {
			t.Error("expected an error when the sites cannot be read, got none")
		}
	})
}

// TestResolveSystem tests the ResolveSystem function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Index: an index is answered without reading anything at all
//   - Name: a name is matched to the index the scanner knows it by
//   - ReadError: a failure reading the systems is passed on
//   - NoSuchName: a name no system carries is refused
func TestResolveSystem(t *testing.T) {
	systems := func(asked map[string]bool) *stubConn {
		return &stubConn{exec: func(command string) (string, error) {
			asked[command] = true
			switch command {
			case "GLT,FL":
				return `<GLT><FL Name="Test List" Index="1"/></GLT>`, nil
			case "GLT,SYS,1":
				return `<GLT><SYS Name="Test System" Index="10"/></GLT>`, nil
			}
			return "", errors.New("unexpected command: " + command)
		}}
	}

	// Verify that an index costs no exchange
	t.Run("Index", func(t *testing.T) {
		asked := map[string]bool{}

		index, err := ResolveSystem(context.Background(), device.New(systems(asked)), "10")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "10" || len(asked) != 0 {
			t.Errorf("expected \"10\" and no reads, got %q and %v", index, asked)
		}
	})

	// Verify that a name is matched to the index the scanner knows it by
	t.Run("Name", func(t *testing.T) {
		client := device.New(systems(map[string]bool{}))

		index, err := ResolveSystem(context.Background(), client, "Test System")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if index != "10" {
			t.Errorf("expected \"10\", got %q", index)
		}
	})

	// Verify that a failed read is passed on
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := ResolveSystem(context.Background(), device.New(conn), "ANY"); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that a name nothing carries is refused, with the message naming
	// what the scanner does have
	t.Run("NoSuchName", func(t *testing.T) {
		client := device.New(systems(map[string]bool{}))

		_, err := ResolveSystem(context.Background(), client, "NO SUCH SYSTEM")
		if err == nil {
			t.Fatal("expected an error for a name no system carries, got none")
		}
		if !strings.Contains(err.Error(), "Test System") {
			t.Errorf("expected the message to name what the scanner has, got: %v", err)
		}
	})
}

// TestSiteFrequencies tests the SiteFrequencies function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a site frequency document becomes the frequencies it describes
//   - Empty: a document carrying no frequencies gives an empty result
//   - ParseError: a malformed document is reported rather than ignored
func TestSiteFrequencies(t *testing.T) {
	// Verify that a site frequency document becomes the frequencies it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<SFREQ Freq="851.0125" Index="60"/>
			<SFREQ Freq="852.4375" Index="61"/>
		</GLT>`

		frequencies, err := SiteFrequencies(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(frequencies) != 2 {
			t.Fatalf("expected 2 frequencies, got %d", len(frequencies))
		}

		want := SiteFrequency{Frequency: "851.0125", Index: "60"}
		if frequencies[0] != want {
			t.Errorf("expected %+v, got %+v", want, frequencies[0])
		}
	})

	// Verify that a document carrying no frequencies gives an empty result
	t.Run("Empty", func(t *testing.T) {
		frequencies, err := SiteFrequencies(`<GLT><Footer/></GLT>`)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(frequencies) != 0 {
			t.Errorf("expected no frequencies, got %d", len(frequencies))
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := SiteFrequencies(`<GLT><SFREQ/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// TestSites tests the Sites function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a site document becomes the sites it describes
//   - Empty: a conventional system answers emptily
//   - ParseError: a malformed document is reported rather than ignored
func TestSites(t *testing.T) {
	// Verify that a site document becomes the sites it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<SITE Name="Test Site" Index="50" Avoid="Off" Q_Key="5"/>
			<SITE Name="Avoided Site" Index="51" Avoid="On" Q_Key="None"/>
		</GLT>`

		sites, err := Sites(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(sites) != 2 {
			t.Fatalf("expected 2 sites, got %d", len(sites))
		}

		want := Site{Name: "Test Site", Index: "50", Avoided: false, QuickKey: "5"}
		if sites[0] != want {
			t.Errorf("expected %+v, got %+v", want, sites[0])
		}
		if !sites[1].Avoided || sites[1].QuickKey != "" {
			t.Errorf("expected the second site to be avoided with no quick key, got %+v", sites[1])
		}
	})

	// Verify that a conventional system, which has no sites, answers emptily
	t.Run("Empty", func(t *testing.T) {
		sites, err := Sites(`<GLT><Footer/></GLT>`)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(sites) != 0 {
			t.Errorf("expected no sites, got %d", len(sites))
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := Sites(`<GLT><SITE/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// TestSystems tests the Systems function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a system document becomes the systems it describes
//   - Empty: a document carrying no systems gives an empty result
//   - ParseError: a malformed document is reported rather than ignored
func TestSystems(t *testing.T) {
	// Verify that a system document becomes the systems it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<SYS Name="Test System" Index="10" Type="Conventional" Avoid="Off" Q_Key="2" N_Tag="None"/>
			<SYS Name="Trunked System" Index="11" Type="P25 Standard" Avoid="On" Q_Key="None" N_Tag="3"/>
		</GLT>`

		systems, err := Systems(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(systems) != 2 {
			t.Fatalf("expected 2 systems, got %d", len(systems))
		}

		// The scanner's own word for the type is passed through untranslated.
		want := System{Name: "Test System", Index: "10", Kind: "Conventional", Avoided: false, QuickKey: "2", NumberTag: ""}
		if systems[0] != want {
			t.Errorf("expected %+v, got %+v", want, systems[0])
		}

		want = System{Name: "Trunked System", Index: "11", Kind: "P25 Standard", Avoided: true, QuickKey: "", NumberTag: "3"}
		if systems[1] != want {
			t.Errorf("expected %+v, got %+v", want, systems[1])
		}
	})

	// Verify that a document carrying no systems gives an empty result
	t.Run("Empty", func(t *testing.T) {
		systems, err := Systems(`<GLT><Footer/></GLT>`)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(systems) != 0 {
			t.Errorf("expected no systems, got %d", len(systems))
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := Systems(`<GLT><SYS/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// TestTalkgroups tests the Talkgroups function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a talkgroup document becomes the channels it describes
//   - Empty: a document carrying no talkgroups gives an empty result
//   - ParseError: a malformed document is reported rather than ignored
func TestTalkgroups(t *testing.T) {
	// Verify that a talkgroup document becomes the channels it describes
	t.Run("Success", func(t *testing.T) {
		doc := `<GLT>
			<TGID Name="Test Talkgroup" Index="40" TGID="1001" Avoid="Off"/>
			<TGID Name="Avoided Talkgroup" Index="41" TGID="1002" Avoid="On"/>
		</GLT>`

		channels, err := Talkgroups(doc)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(channels) != 2 {
			t.Fatalf("expected 2 channels, got %d", len(channels))
		}

		// A trunked channel carries a talkgroup and no frequency.
		want := Channel{Name: "Test Talkgroup", Index: "40", Frequency: "", Talkgroup: "1001", Avoided: false}
		if channels[0] != want {
			t.Errorf("expected %+v, got %+v", want, channels[0])
		}
		if !channels[1].Avoided {
			t.Errorf("expected the second channel to be avoided, got %+v", channels[1])
		}
	})

	// Verify that a document carrying no talkgroups gives an empty result
	t.Run("Empty", func(t *testing.T) {
		channels, err := Talkgroups(`<GLT><Footer/></GLT>`)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(channels) != 0 {
			t.Errorf("expected no channels, got %d", len(channels))
		}
	})

	// Verify that a malformed document is reported as an error
	t.Run("ParseError", func(t *testing.T) {
		if _, err := Talkgroups(`<GLT><TGID/></BAD>`); err == nil {
			t.Error("expected an error for a malformed document, got none")
		}
	})
}

// Test_attributes tests the attributes function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Attributes: every attribute is collected by name
//   - None: a tag carrying no attributes gives an empty element
func Test_attributes(t *testing.T) {
	// Verify that every attribute of an opening tag is collected by name
	t.Run("Attributes", func(t *testing.T) {
		start := xml.StartElement{
			Name: xml.Name{Local: "FL"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "Name"}, Value: "Test List"},
				{Name: xml.Name{Local: "Index"}, Value: "1"},
			},
		}

		e := attributes(start)
		if len(e) != 2 {
			t.Fatalf("expected 2 attributes, got %d", len(e))
		}
		if e["Name"] != "Test List" || e["Index"] != "1" {
			t.Errorf("expected the tag's attributes, got %+v", e)
		}
	})

	// Verify that a tag carrying no attributes gives an empty element
	t.Run("None", func(t *testing.T) {
		e := attributes(xml.StartElement{Name: xml.Name{Local: "Footer"}})
		if len(e) != 0 {
			t.Errorf("expected no attributes, got %d", len(e))
		}
	})
}

// Test_isKnown tests the isKnown function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - First: the first known name is recognised
//   - Last: the last known name is recognised
//   - Unknown: a name outside the set is not recognised
func Test_isKnown(t *testing.T) {
	// Verify that the first of the known element names is recognised
	t.Run("First", func(t *testing.T) {
		if !isKnown("FL") {
			t.Error("expected \"FL\" to be a known element name")
		}
	})

	// Verify that the last of the known element names is recognised
	t.Run("Last", func(t *testing.T) {
		if !isKnown("CS_BANK") {
			t.Error("expected \"CS_BANK\" to be a known element name")
		}
	})

	// Verify that a name outside the set is not recognised
	t.Run("Unknown", func(t *testing.T) {
		if isKnown("Footer") {
			t.Error("expected \"Footer\" not to be a known element name")
		}
	})
}

// TestDepartment_named tests the Department.named method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - NameAndIndex: the department reports its name and index
func TestDepartment_named(t *testing.T) {
	// Verify that a department reports its name and index for resolving
	t.Run("NameAndIndex", func(t *testing.T) {
		name, index := Department{Name: "Test Department", Index: "20"}.named()
		if name != "Test Department" || index != "20" {
			t.Errorf("expected \"Test Department\" and \"20\", got %q and %q", name, index)
		}
	})
}

// TestFavoritesList_named tests the FavoritesList.named method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - NameAndIndex: the favorites list reports its name and index
func TestFavoritesList_named(t *testing.T) {
	// Verify that a favorites list reports its name and index for resolving
	t.Run("NameAndIndex", func(t *testing.T) {
		name, index := FavoritesList{Name: "Test List", Index: "1"}.named()
		if name != "Test List" || index != "1" {
			t.Errorf("expected \"Test List\" and \"1\", got %q and %q", name, index)
		}
	})
}

// TestSite_named tests the Site.named method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - NameAndIndex: the site reports its name and index
func TestSite_named(t *testing.T) {
	// Verify that a site reports its name and index for resolving
	t.Run("NameAndIndex", func(t *testing.T) {
		name, index := Site{Name: "Test Site", Index: "50"}.named()
		if name != "Test Site" || index != "50" {
			t.Errorf("expected \"Test Site\" and \"50\", got %q and %q", name, index)
		}
	})
}

// TestSystem_named tests the System.named method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - NameAndIndex: the system reports its name and index
func TestSystem_named(t *testing.T) {
	// Verify that a system reports its name and index for resolving
	t.Run("NameAndIndex", func(t *testing.T) {
		name, index := System{Name: "Test System", Index: "10"}.named()
		if name != "Test System" || index != "10" {
			t.Errorf("expected \"Test System\" and \"10\", got %q and %q", name, index)
		}
	})
}
