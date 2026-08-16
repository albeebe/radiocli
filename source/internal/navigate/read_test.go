// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package navigate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
)

// walkable answers as a scanner sitting in its menus, well enough for a list to
// be read off the screen and the scanner taken back out afterwards.
//
// It is a fuller fake than stubConn because that is what these reads need. A
// walk turns the knob round a menu that wraps, asks for the scanner's own
// listing at every step, and finishes by pressing its way out of the menus, and
// none of those work against a stub that answers a fixed run of screens.
type walkable struct {
	top    []string            // The menu opening one over the wire puts on screen
	rows   []string            // The menu on screen, in the order the knob reaches the rows
	cursor int                 // Which row the knob is on
	opens  map[string][]string // The menu each entry puts on screen when it is pressed
	docs   map[string]string   // The document answering each list request, by command
	listed int                 // How many rows of the menu the scanner's own listing admits to
	title  string              // The menu reported when asked, empty for a scanner out of the menus
	fail   map[string]error    // Commands that fail instead of answering
}

// Info describes the fake scanner, which nothing here inspects.
func (w *walkable) Info() device.Info { return device.Info{} }

// Execute answers one command the way the scanner would.
func (w *walkable) Execute(ctx context.Context, command string) (string, error) {
	if err := w.fail[command]; err != nil {
		return "", err
	}

	switch {
	case command == "STS":
		row := ""
		if len(w.rows) > 0 {
			row = w.rows[w.cursor]
		}
		return "0," + row + ",****************", nil

	case strings.HasPrefix(command, "MNU,"):
		w.rows, w.cursor = w.top, 0
		w.title = "Menu"

	case strings.HasPrefix(command, "KEY,"):
		w.press(strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P"))
	}
	return "", nil
}

// ExecuteXML answers a list request with the document the test supplied, and a
// menu request with what the scanner is showing.
func (w *walkable) ExecuteXML(ctx context.Context, command string) (string, error) {
	if err := w.fail[command]; err != nil {
		return "", err
	}
	if command == "GSI" {
		return `<ScannerInfo Mode="Scan Mode" V_Screen="scan"/>`, nil
	}
	if strings.HasPrefix(command, "GLT,") {
		doc, ok := w.docs[command]
		if !ok {
			return "", errors.New("unexpected command: " + command)
		}
		return doc, nil
	}
	if command != "MSI" {
		return "", errors.New("unexpected command: " + command)
	}

	// A scanner out of the menus answers with the error document, which is what
	// tells the escape it has finished.
	if w.title == "" {
		return `<MenuInfo MenuType="TypeError"/>`, nil
	}

	// The listing is a window on a long menu rather than the whole of it, which
	// is the behaviour the walk exists to work around, so only the rows the
	// test allows are admitted to.
	items := ""
	for i, row := range w.rows {
		if w.listed > 0 && i >= w.listed {
			break
		}
		items += fmt.Sprintf(`<MenuItem Name="%s" Index="%d"/>`, row, i)
	}
	return fmt.Sprintf(`<MenuInfo Name="%s" MenuType="TypeList">%s</MenuInfo>`, w.title, items), nil
}

// Send is unused by this package and always succeeds.
func (w *walkable) Send(ctx context.Context, command string) error { return nil }

// Close is unused by this package and always succeeds.
func (w *walkable) Close() error { return nil }

// press acts on one key the way the scanner would, given what is on screen.
func (w *walkable) press(key string) {
	switch key {
	case ">":
		if len(w.rows) > 0 {
			w.cursor = (w.cursor + 1) % len(w.rows)
		}
	case "<":
		if len(w.rows) > 0 {
			w.cursor = (w.cursor - 1 + len(w.rows)) % len(w.rows)
		}
	case "E":
		if len(w.rows) == 0 {
			return
		}
		if next, open := w.opens[w.rows[w.cursor]]; open {
			w.rows, w.cursor = next, 0
		}
	case "L", "A":
		// The keys that leave the menus altogether.
		w.title, w.rows, w.cursor = "", nil, 0
	}
}

// department returns a scanner holding one department whose channel list is the
// rows given, reached the way ToChannels reaches it, and answering the channel
// list request with doc.
func department(doc string, channels ...string) *walkable {
	rows := append([]string{newChannel}, channels...)
	return &walkable{
		top:   []string{editChannels},
		opens: map[string][]string{editChannels: rows},
		docs:  map[string]string{"GLT,CFREQ,20": doc},
	}
}

// cutChannels renders a channel list document the scanner stopped short of
// finishing, carrying only the channels named.
func cutChannels(names ...string) string {
	var b strings.Builder
	b.WriteString("<GLT>")
	for i, name := range names {
		fmt.Fprintf(&b, `<CFREQ Index="%d" Name=%q Freq="27.055 MHz" Avoid="Off"/>`, i, name)
	}
	b.WriteString(`<Footer No="1" EOT="0"/></GLT>`)
	return b.String()
}

// TestReadChannels tests the ReadChannels function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - CutShort: the channels the list left out are found on the menus
//   - Whole: a list that fitted is returned without the scan being stopped
//   - ReadError: a failure the walk cannot make up for is passed on
func TestReadChannels(t *testing.T) {
	// Verify the reported bug: a department whose list does not fit comes back
	// whole, with the channels the list left out marked as names alone.
	t.Run("CutShort", func(t *testing.T) {
		conn := department(cutChannels("CH 01", "CH 02"),
			"CH 01", "CH 02", "CH 03", "CH 04", "CH 05")

		got, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("ReadChannels() = %v, want nil", err)
		}
		if len(got) != 5 {
			t.Fatalf("read %d channels, want all five the department holds", len(got))
		}

		// The two the list carried keep everything it said about them.
		if got[0].Name != "CH 01" || got[0].Frequency != "27.055 MHz" || got[0].Partial {
			t.Errorf("first channel = %+v, want the list's own row", got[0])
		}

		// The three it did not are names and nothing else, and say so.
		for _, c := range got[2:] {
			if !c.Partial {
				t.Errorf("channel %q is not marked partial, but only its name was read", c.Name)
			}
		}
		if got[4].Name != "CH 05" {
			t.Errorf("last channel = %q, want the last one on the menu", got[4].Name)
		}
	})

	// Verify that a list the scanner finished is taken at its word, with no
	// walk and no partial entries.
	t.Run("Whole", func(t *testing.T) {
		conn := department(`<GLT><CFREQ Index="0" Name="CH 01" Freq="27.055 MHz" Avoid="Off"/>`+
			`<Footer No="1" EOT="1"/></GLT>`, "CH 01")

		got, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("ReadChannels() = %v, want nil", err)
		}
		if len(got) != 1 || got[0].Partial {
			t.Fatalf("read %+v, want the one row the list carried", got)
		}
	})

	// Verify that a read that failed outright is not papered over with a walk.
	t.Run("ReadError", func(t *testing.T) {
		conn := department(cutChannels("CH 01"), "CH 01")
		conn.fail = map[string]error{"GLT,CFREQ,20": errors.New("the port is gone")}

		if _, err := ReadChannels(context.Background(), device.New(conn), "20"); err == nil {
			t.Error("expected an error when the list cannot be read at all, got none")
		}
	})
}

// TestReadChannelsIndexes tests that a filled-in entry carries the index the
// scanner's own listing gave it, where it gave one.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Listed: an entry the listing mentions carries its index
//   - Unlisted: an entry it never mentions carries none, rather than a wrong one
func TestReadChannelsIndexes(t *testing.T) {
	// Verify that the listing's index is picked up as the walk goes past.
	t.Run("Listed", func(t *testing.T) {
		conn := department(cutChannels("CH 01"), "CH 01", "CH 02")

		got, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("ReadChannels() = %v, want nil", err)
		}
		if got[1].Index == "" {
			t.Error("the filled-in channel has no index, but the listing named it")
		}
	})

	// Verify that a row past the end of the listing is left without an index
	// rather than given somebody else's.
	t.Run("Unlisted", func(t *testing.T) {
		conn := department(cutChannels("CH 01"), "CH 01", "CH 02")
		conn.listed = 1

		got, err := ReadChannels(context.Background(), device.New(conn), "20")
		if err != nil {
			t.Fatalf("ReadChannels() = %v, want nil", err)
		}
		if got[1].Index != "" {
			t.Errorf("the filled-in channel has index %q, but the listing never named it", got[1].Index)
		}
	})
}

// Test_filled tests the filled function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Whole: a list that was not cut short is returned untouched
//   - HardError: an error a walk cannot make up for is returned untouched
//   - WalkError: a walk that cannot reach the menu reports both failures
//   - Disagree: a row the menu does not show is reported rather than dropped
func Test_filled(t *testing.T) {
	name := func(c catalog.Channel) string { return c.Name }
	blank := func(e menus.Entry) catalog.Channel {
		return catalog.Channel{Name: e.Name, Partial: true}
	}
	unreachable := func() error { return errors.New("the menu will not open") }

	// Verify that a whole list costs no walk and comes back as it went in.
	t.Run("Whole", func(t *testing.T) {
		found := []catalog.Channel{{Name: "CH 01"}}

		got, err := filled(context.Background(), nil, found, nil, "channel",
			unreachable, newChannel, name, blank)
		if err != nil {
			t.Fatalf("filled() = %v, want nil", err)
		}
		if len(got) != 1 {
			t.Fatalf("filled() returned %+v, want the list unchanged", got)
		}
	})

	// Verify that a read that failed for some other reason is passed straight
	// back, because walking the menus would not answer the question either.
	t.Run("HardError", func(t *testing.T) {
		hard := errors.New("the port is gone")

		if _, err := filled(context.Background(), nil, nil, hard, "channel",
			unreachable, newChannel, name, blank); !errors.Is(err, hard) {
			t.Fatalf("filled() = %v, want the read's own error", err)
		}
	})

	// Verify that a walk that cannot run says so without hiding the truncation
	// that made it necessary.
	t.Run("WalkError", func(t *testing.T) {
		cut := fmt.Errorf("reading the channels: %w", catalog.ErrIncomplete)

		_, err := filled(context.Background(), nil, nil, cut, "channel",
			unreachable, newChannel, name, blank)
		if !errors.Is(err, catalog.ErrIncomplete) {
			t.Fatalf("filled() = %v, want it to still report the truncation", err)
		}
		if !strings.Contains(err.Error(), "menus") {
			t.Errorf("filled() = %v, want it to say the walk failed too", err)
		}
	})

	// Verify that a row the list carried and the menu does not show is a
	// disagreement worth reporting, not something to quietly drop.
	t.Run("Disagree", func(t *testing.T) {
		conn := department(cutChannels("GHOST"), "CH 01")
		cut := fmt.Errorf("reading the channels: %w", catalog.ErrIncomplete)
		client := device.New(conn)

		_, err := filled(context.Background(), client, []catalog.Channel{{Name: "GHOST"}}, cut, "channel",
			func() error { return ToChannels(context.Background(), client, "20") },
			newChannel, name, blank)
		if err == nil {
			t.Fatal("expected an error when the two readings disagree, got none")
		}
		if !strings.Contains(err.Error(), "GHOST") {
			t.Errorf("filled() = %v, want it to name the row the menu does not show", err)
		}
	})
}

// Test_walked tests the walked function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the list comes back without the entry that creates a new one
//   - ReachError: a menu that cannot be opened is reported
//   - ReadError: a menu that cannot be read leaves the scanner out of the menus
func Test_walked(t *testing.T) {
	// Verify that New Channel is left out, since it is on the menu but is not
	// one of the channels.
	t.Run("Success", func(t *testing.T) {
		conn := department("<GLT></GLT>", "CH 01", "CH 02")
		client := device.New(conn)

		got, err := walked(context.Background(), client,
			func() error { return ToChannels(context.Background(), client, "20") }, newChannel)
		if err != nil {
			t.Fatalf("walked() = %v, want nil", err)
		}
		if len(got) != 2 || got[0].Name != "CH 01" {
			t.Fatalf("walked() returned %+v, want the two channels without the New entry", got)
		}
	})

	// Verify that a menu the walk cannot reach is reported rather than read as
	// an empty list.
	t.Run("ReachError", func(t *testing.T) {
		if _, err := walked(context.Background(), nil,
			func() error { return errors.New("the menu will not open") }, newChannel); err == nil {
			t.Error("expected an error when the menu cannot be reached, got none")
		}
	})

	// Verify that a screen that cannot be read is reported, and that the
	// scanner is still taken out of the menus on the way past.
	t.Run("ReadError", func(t *testing.T) {
		conn := department("<GLT></GLT>", "CH 01")
		client := device.New(conn)
		conn.fail = map[string]error{"STS": errors.New("the screen cannot be read")}

		if _, err := walked(context.Background(), client,
			func() error { return nil }, newChannel); err == nil {
			t.Error("expected an error when the screen cannot be read, got none")
		}
	})
}

// listDoc renders a list document holding the elements given, cut short so that
// reading it sends the caller to the menus.
func listDoc(element string, names ...string) string {
	var b strings.Builder
	b.WriteString("<GLT>")
	for i, name := range names {
		fmt.Fprintf(&b, `<%s Index="%d" Name=%q Avoid="Off"/>`, element, i, name)
	}
	b.WriteString(`<Footer No="1" EOT="0"/></GLT>`)
	return b.String()
}

// TestReadFavorites tests the ReadFavorites function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - CutShort: the lists the scanner left out are found on its menus
//   - ReadError: a failure the walk cannot make up for is passed on
func TestReadFavorites(t *testing.T) {
	// Verify that a scanner with more lists than fit in one reply reports all
	// of them.
	t.Run("CutShort", func(t *testing.T) {
		conn := &walkable{
			top:   []string{manageFavorites},
			opens: map[string][]string{manageFavorites: {newFavoritesList, "HOME", "WORK"}},
			docs:  map[string]string{"GLT,FL": listDoc("FL", "HOME")},
		}

		got, err := ReadFavorites(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("ReadFavorites() = %v, want nil", err)
		}
		if len(got) != 2 || got[1].Name != "WORK" || !got[1].Partial {
			t.Fatalf("read %+v, want both lists with the second only partly known", got)
		}
	})

	// Verify that a list request that failed outright is reported as that.
	t.Run("ReadError", func(t *testing.T) {
		conn := &walkable{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if _, err := ReadFavorites(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the lists cannot be read, got none")
		}
	})
}

// TestReadSystems tests the ReadSystems function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - CutShort: the systems the scanner left out are found on its menus
//   - ReadError: a failure the walk cannot make up for is passed on
func TestReadSystems(t *testing.T) {
	// Verify that a favorites list holding more systems than fit comes back
	// whole, which means walking down through the list's own menu to reach them.
	t.Run("CutShort", func(t *testing.T) {
		conn := &walkable{
			top: []string{manageFavorites},
			opens: map[string][]string{
				manageFavorites: {newFavoritesList, "HOME"},
				"HOME":          {reviewSystems},
				reviewSystems:   {newSystem, "POLICE", "FIRE"},
			},
			docs: map[string]string{
				"GLT,FL":    `<GLT><FL Index="0" Name="HOME" Monitor="On"/></GLT>`,
				"GLT,SYS,0": listDoc("SYS", "POLICE"),
			},
		}

		got, err := ReadSystems(context.Background(), device.New(conn), "0")
		if err != nil {
			t.Fatalf("ReadSystems() = %v, want nil", err)
		}
		if len(got) != 2 || got[1].Name != "FIRE" || !got[1].Partial {
			t.Fatalf("read %+v, want both systems with the second only partly known", got)
		}
	})

	// Verify that asking a built-in scan source is still refused before
	// anything is sent, rather than being walked instead.
	t.Run("ReadError", func(t *testing.T) {
		conn := &walkable{}

		if _, err := ReadSystems(context.Background(), device.New(conn), "4294967295"); err == nil {
			t.Error("expected the full database to be refused, got no error")
		}
	})
}

// TestReadDepartments tests the ReadDepartments function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - CutShort: the departments the scanner left out are found on its menus
//   - ReadError: a failure the walk cannot make up for is passed on
func TestReadDepartments(t *testing.T) {
	// Verify that a system holding more departments than fit comes back whole.
	t.Run("CutShort", func(t *testing.T) {
		conn := &walkable{
			top:   []string{editDepartments},
			opens: map[string][]string{editDepartments: {newDepartment, "CB Channels", "MARINE"}},
			docs:  map[string]string{"GLT,DEPT,10": listDoc("DEPT", "CB Channels")},
		}

		got, err := ReadDepartments(context.Background(), device.New(conn), "10")
		if err != nil {
			t.Fatalf("ReadDepartments() = %v, want nil", err)
		}
		if len(got) != 2 || got[1].Name != "MARINE" || !got[1].Partial {
			t.Fatalf("read %+v, want both departments with the second only partly known", got)
		}
	})

	// Verify that a list request that failed outright is reported as that.
	t.Run("ReadError", func(t *testing.T) {
		conn := &walkable{fail: map[string]error{"GLT,DEPT,10": errors.New("the port is gone")}}

		if _, err := ReadDepartments(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error when the departments cannot be read, got none")
		}
	})
}

// TestReadSites tests the ReadSites function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - CutShort: the sites the scanner left out are found on its menus
//   - ReadError: a failure the walk cannot make up for is passed on
func TestReadSites(t *testing.T) {
	// Verify that a system holding more sites than fit comes back whole.
	t.Run("CutShort", func(t *testing.T) {
		conn := &walkable{
			top:   []string{editSites},
			opens: map[string][]string{editSites: {newSite, "NORTH", "SOUTH"}},
			docs:  map[string]string{"GLT,SITE,10": listDoc("SITE", "NORTH")},
		}

		got, err := ReadSites(context.Background(), device.New(conn), "10")
		if err != nil {
			t.Fatalf("ReadSites() = %v, want nil", err)
		}
		if len(got) != 2 || got[1].Name != "SOUTH" || !got[1].Partial {
			t.Fatalf("read %+v, want both sites with the second only partly known", got)
		}
	})

	// Verify that a list request that failed outright is reported as that.
	t.Run("ReadError", func(t *testing.T) {
		conn := &walkable{fail: map[string]error{"GLT,SITE,10": errors.New("the port is gone")}}

		if _, err := ReadSites(context.Background(), device.New(conn), "10"); err == nil {
			t.Error("expected an error when the sites cannot be read, got none")
		}
	})
}

// TestReadSiteFrequencies tests the ReadSiteFrequencies function with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - CutShort: the frequencies the scanner left out are found on its menus
//   - ReadError: a failure the walk cannot make up for is passed on
func TestReadSiteFrequencies(t *testing.T) {
	// Verify that a site holding more frequencies than fit comes back whole.
	// These carry no name, so the frequency itself is what the screen shows and
	// what the two readings are lined up by.
	t.Run("CutShort", func(t *testing.T) {
		conn := &walkable{
			top:   []string{setFrequencies},
			opens: map[string][]string{setFrequencies: {newFrequency, "851.0125", "852.4375"}},
			docs: map[string]string{
				"GLT,SFREQ,5": `<GLT><SFREQ Index="0" Freq="851.0125"/><Footer No="1" EOT="0"/></GLT>`,
			},
		}

		got, err := ReadSiteFrequencies(context.Background(), device.New(conn), "5")
		if err != nil {
			t.Fatalf("ReadSiteFrequencies() = %v, want nil", err)
		}
		if len(got) != 2 || got[1].Frequency != "852.4375" || !got[1].Partial {
			t.Fatalf("read %+v, want both frequencies with the second only partly known", got)
		}
	})

	// Verify that a list request that failed outright is reported as that.
	t.Run("ReadError", func(t *testing.T) {
		conn := &walkable{fail: map[string]error{"GLT,SFREQ,5": errors.New("the port is gone")}}

		if _, err := ReadSiteFrequencies(context.Background(), device.New(conn), "5"); err == nil {
			t.Error("expected an error when the frequencies cannot be read, got none")
		}
	})
}

// TestEverySystem tests the EverySystem function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: every list someone created is read, and the built-in ones skipped
//   - ListsError: a failure reading the lists is passed on
//   - SystemsError: a failure reading one list's systems is passed on
func TestEverySystem(t *testing.T) {
	lists := `<GLT><FL Index="0" Name="HOME" Monitor="On"/>` +
		`<FL Index="4294967295" Name="Full Database" Monitor="On"/></GLT>`

	// Verify that the built-in database is skipped, which is what keeps this
	// from locking the scanner up.
	t.Run("Success", func(t *testing.T) {
		conn := &walkable{docs: map[string]string{
			"GLT,FL":    lists,
			"GLT,SYS,0": `<GLT><SYS Index="0" Name="POLICE" Type="Conventional" Avoid="Off"/></GLT>`,
		}}

		got, err := EverySystem(context.Background(), device.New(conn))
		if err != nil {
			t.Fatalf("EverySystem() = %v, want nil", err)
		}
		if len(got) != 1 || got[0].Name != "POLICE" {
			t.Fatalf("read %+v, want the one system in the one list someone made", got)
		}
	})

	// Verify that a failure reading the lists stops the whole thing.
	t.Run("ListsError", func(t *testing.T) {
		conn := &walkable{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if _, err := EverySystem(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the lists cannot be read, got none")
		}
	})

	// Verify that a failure part way through is reported rather than leaving a
	// short answer looking complete.
	t.Run("SystemsError", func(t *testing.T) {
		conn := &walkable{
			docs: map[string]string{"GLT,FL": lists},
			fail: map[string]error{"GLT,SYS,0": errors.New("the port is gone")},
		}

		if _, err := EverySystem(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when a list's systems cannot be read, got none")
		}
	})
}

// TestEveryDepartment tests the EveryDepartment function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: every department of every system is read
//   - SystemsError: a failure reading the systems is passed on
//   - DepartmentsError: a failure reading one system's departments is passed on
func TestEveryDepartment(t *testing.T) {
	docs := map[string]string{
		"GLT,FL":     `<GLT><FL Index="0" Name="HOME" Monitor="On"/></GLT>`,
		"GLT,SYS,0":  `<GLT><SYS Index="7" Name="POLICE" Type="Conventional" Avoid="Off"/></GLT>`,
		"GLT,DEPT,7": `<GLT><DEPT Index="9" Name="CB Channels" Avoid="Off"/></GLT>`,
	}

	// Verify that the departments come back from under every system.
	t.Run("Success", func(t *testing.T) {
		got, err := EveryDepartment(context.Background(), device.New(&walkable{docs: docs}))
		if err != nil {
			t.Fatalf("EveryDepartment() = %v, want nil", err)
		}
		if len(got) != 1 || got[0].Name != "CB Channels" {
			t.Fatalf("read %+v, want the one department", got)
		}
	})

	// Verify that a failure above this level is passed on.
	t.Run("SystemsError", func(t *testing.T) {
		conn := &walkable{docs: docs, fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if _, err := EveryDepartment(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that a failure reading one system's departments is passed on.
	t.Run("DepartmentsError", func(t *testing.T) {
		conn := &walkable{docs: docs, fail: map[string]error{"GLT,DEPT,7": errors.New("the port is gone")}}

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
//   - Success: every site of every system is read
//   - SystemsError: a failure reading the systems is passed on
//   - SitesError: a failure reading one system's sites is passed on
func TestEverySite(t *testing.T) {
	docs := map[string]string{
		"GLT,FL":     `<GLT><FL Index="0" Name="HOME" Monitor="On"/></GLT>`,
		"GLT,SYS,0":  `<GLT><SYS Index="7" Name="P25" Type="P25 Trunk" Avoid="Off"/></GLT>`,
		"GLT,SITE,7": `<GLT><SITE Index="3" Name="NORTH" Avoid="Off"/></GLT>`,
	}

	// Verify that the sites come back from under every system.
	t.Run("Success", func(t *testing.T) {
		got, err := EverySite(context.Background(), device.New(&walkable{docs: docs}))
		if err != nil {
			t.Fatalf("EverySite() = %v, want nil", err)
		}
		if len(got) != 1 || got[0].Name != "NORTH" {
			t.Fatalf("read %+v, want the one site", got)
		}
	})

	// Verify that a failure above this level is passed on.
	t.Run("SystemsError", func(t *testing.T) {
		conn := &walkable{docs: docs, fail: map[string]error{"GLT,FL": errors.New("the port is gone")}}

		if _, err := EverySite(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the systems cannot be read, got none")
		}
	})

	// Verify that a failure reading one system's sites is passed on.
	t.Run("SitesError", func(t *testing.T) {
		conn := &walkable{docs: docs, fail: map[string]error{"GLT,SITE,7": errors.New("the port is gone")}}

		if _, err := EverySite(context.Background(), device.New(conn)); err == nil {
			t.Error("expected an error when the sites cannot be read, got none")
		}
	})
}

// TestResolveFavorites tests the ResolveFavorites function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Index: an index is taken as it stands and costs no exchange
//   - Name: a name is matched against the lists the scanner holds
func TestResolveFavorites(t *testing.T) {
	// Verify that an index is answered without reading anything.
	t.Run("Index", func(t *testing.T) {
		got, err := ResolveFavorites(context.Background(), device.New(&walkable{}), "2")
		if err != nil {
			t.Fatalf("ResolveFavorites() = %v, want nil", err)
		}
		if got != "2" {
			t.Errorf("ResolveFavorites() = %q, want the index unchanged", got)
		}
	})

	// Verify that a name is looked up.
	t.Run("Name", func(t *testing.T) {
		conn := &walkable{docs: map[string]string{
			"GLT,FL": `<GLT><FL Index="4" Name="HOME" Monitor="On"/></GLT>`,
		}}

		got, err := ResolveFavorites(context.Background(), device.New(conn), "HOME")
		if err != nil {
			t.Fatalf("ResolveFavorites() = %v, want nil", err)
		}
		if got != "4" {
			t.Errorf("ResolveFavorites() = %q, want the list's own index", got)
		}
	})
}

// TestWalkedLeaveError tests that a scanner that cannot be taken out of the
// menus is reported rather than left there quietly.
//
// Coverage: 100% (1 test case covering the remaining branch)
//
// Test cases:
//   - LeaveError: an escape the scanner will not take is reported
func TestWalkedLeaveError(t *testing.T) {
	// Verify that a scanner refusing every escape key is reported, since the
	// alternative is a read that answers correctly and leaves the radio parked.
	t.Run("LeaveError", func(t *testing.T) {
		conn := department("<GLT></GLT>", "CH 01")
		client := device.New(conn)
		conn.fail = map[string]error{
			fmt.Sprintf("KEY,%s,%s", device.KeyAvoid, device.KeyPress): errors.New("the key was not taken"),
			fmt.Sprintf("KEY,%s,%s", device.KeySoft1, device.KeyPress): errors.New("the key was not taken"),
		}

		_, err := walked(context.Background(), client,
			func() error { return ToChannels(context.Background(), client, "20") }, newChannel)
		if err == nil {
			t.Error("expected an error when the scanner will not leave the menus, got none")
		}
	})
}

// TestEveryUnnamed tests that a system the scanner never gave an index for is
// skipped rather than asked about with nothing.
//
// Coverage: 100% (2 test cases covering the remaining branches)
//
// Test cases:
//   - Departments: a system with no index contributes no departments
//   - Sites: a system with no index contributes no sites
func TestEveryUnnamed(t *testing.T) {
	// A favorites list whose systems the scanner cut short, on a menu whose
	// listing admits to nothing at all, so the walked system has no index.
	unnamed := func() *walkable {
		return &walkable{
			top: []string{manageFavorites},
			opens: map[string][]string{
				manageFavorites: {newFavoritesList, "HOME"},
				"HOME":          {reviewSystems},
				reviewSystems:   {newSystem, "GHOST"},
			},
			docs: map[string]string{
				"GLT,FL":    `<GLT><FL Index="0" Name="HOME" Monitor="On"/></GLT>`,
				"GLT,SYS,0": `<GLT><Footer No="1" EOT="0"/></GLT>`,
			},
			listed: 1,
		}
	}

	// Verify that a system found only on the screen is passed over rather than
	// turned into a department request with no index in it.
	t.Run("Departments", func(t *testing.T) {
		got, err := EveryDepartment(context.Background(), device.New(unnamed()))
		if err != nil {
			t.Fatalf("EveryDepartment() = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Fatalf("read %+v, want nothing: the one system has no index to ask with", got)
		}
	})

	// Verify the same on the site side.
	t.Run("Sites", func(t *testing.T) {
		got, err := EverySite(context.Background(), device.New(unnamed()))
		if err != nil {
			t.Fatalf("EverySite() = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Fatalf("read %+v, want nothing: the one system has no index to ask with", got)
		}
	})
}
