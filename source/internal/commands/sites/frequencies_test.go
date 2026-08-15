// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
)

// Test_newFrequencies tests the newFrequencies function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its own subcommands
//   - Runs: running the subcommand reaches the scanner
func Test_newFrequencies(t *testing.T) {
	// Verify that the subcommand is named and carries the ones nested under it
	t.Run("Wiring", func(t *testing.T) {
		cmd := newFrequencies(appcontext.New())

		if cmd.Name() != "frequencies" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "frequencies")
		}
		for _, want := range []string{"add", "delete"} {
			found := false
			for _, sub := range cmd.Commands() {
				if sub.Name() == want {
					found = true
				}
			}
			if !found {
				t.Errorf("the subcommand has no %q subcommand", want)
			}
		}
	})

	// Verify that running the subcommand asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newFrequencies(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"100"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_newFrequenciesAdd tests the newFrequenciesAdd function with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its help
//   - Runs: running the subcommand reaches the scanner
func Test_newFrequenciesAdd(t *testing.T) {
	// Verify that the subcommand is named and documented
	t.Run("Wiring", func(t *testing.T) {
		cmd := newFrequenciesAdd(appcontext.New())

		if cmd.Name() != "add" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "add")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify that running the subcommand asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newFrequenciesAdd(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"100", "851.050"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_newFrequenciesDelete tests the newFrequenciesDelete function with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its --yes flag
//   - Runs: running the subcommand reaches the scanner
func Test_newFrequenciesDelete(t *testing.T) {
	// Verify that the subcommand is named and carries the flag that arms it
	t.Run("Wiring", func(t *testing.T) {
		cmd := newFrequenciesDelete(appcontext.New())

		if cmd.Name() != "delete" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "delete")
		}
		if cmd.Flags().Lookup("yes") == nil {
			t.Error("the subcommand has no --yes flag")
		}
	})

	// Verify that running the subcommand asks for the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		cmd := newFrequenciesDelete(app)
		cmd.SetContext(context.Background())

		if err := cmd.RunE(cmd, []string{"100", "851.050"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})
}

// Test_addOne tests the addOne function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - SelectError: a list holding no New Frequency entry is reported
//   - SetError: a frequency that cannot be typed is reported
//   - Success: the frequency is typed into the entry screen
func Test_addOne(t *testing.T) {
	// The frequency entry screen before and after the digits are typed.
	emptyScreen := `<MenuInfo Name="Frequency" MenuType="TypeInput" Value="">` +
		`<MenuInput MaxLength="16" EnableKeys="0123456789."/></MenuInfo>`
	typedScreen := `<MenuInfo Name="Frequency" MenuType="TypeInput" Value="851.050">` +
		`<MenuInput MaxLength="16" EnableKeys="0123456789."/></MenuInfo>`

	// Verify that a list with no entry screen to open is reported
	t.Run("SelectError", func(t *testing.T) {
		conn := &stubConn{}

		err := addOne(context.Background(), device.New(conn), "851.050")
		if err == nil {
			t.Fatal("expected an error when the entry screen cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "opening the frequency entry screen") {
			t.Errorf("expected the message to say it was opening the screen, got: %v", err)
		}
	})

	// Verify that a frequency the screen will not take is reported
	t.Run("SetError", func(t *testing.T) {
		conn := &stubConn{screens: []string{newFrequency}}

		err := addOne(context.Background(), device.New(conn), "851.050")
		if err == nil {
			t.Fatal("expected an error when the frequency cannot be typed, got none")
		}
		if !strings.Contains(err.Error(), "entering it") {
			t.Errorf("expected the message to say it was entering the frequency, got: %v", err)
		}
	})

	// Verify that the frequency is typed on the keypad and accepted
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{
			screens: []string{newFrequency},
			docs:    map[string][]string{"MSI": {emptyScreen, typedScreen}},
		}

		if err := addOne(context.Background(), device.New(conn), "851.050"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(strings.Join(conn.sent, " "), "KEY,8,P") {
			t.Errorf("expected the digits to be pressed, got: %v", conn.sent)
		}
	})
}

// Test_held tests the held function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Held: the same frequency written either way is recognised
//   - NotHeld: a frequency the site does not hold is not
//   - Unreadable: a frequency that is not a number is not held
//   - SkipsUnreadable: a row that is not a number is passed over
func Test_held(t *testing.T) {
	// Verify that the scanner's own padded spelling matches what was typed
	t.Run("Held", func(t *testing.T) {
		found := []catalog.SiteFrequency{{Frequency: " 851.050000MHz", Index: "200"}}

		if !held(found, "851.05") {
			t.Error("expected the site to hold the frequency, it did not")
		}
	})

	// Verify that another frequency is not taken for the one asked about
	t.Run("NotHeld", func(t *testing.T) {
		found := []catalog.SiteFrequency{{Frequency: " 851.050000MHz", Index: "200"}}

		if held(found, "852.000") {
			t.Error("expected the site not to hold the frequency, it did")
		}
	})

	// Verify that a frequency that is not a number is never held
	t.Run("Unreadable", func(t *testing.T) {
		found := []catalog.SiteFrequency{{Frequency: " 851.050000MHz", Index: "200"}}

		if held(found, "not a frequency") {
			t.Error("expected nothing to match a value that is not a number, it did")
		}
	})

	// Verify that a row the scanner wrote in some other way is passed over
	t.Run("SkipsUnreadable", func(t *testing.T) {
		found := []catalog.SiteFrequency{
			{Frequency: "off", Index: "199"},
			{Frequency: " 851.050000MHz", Index: "200"},
		}

		if !held(found, "851.050") {
			t.Error("expected the readable row to still be found, it was not")
		}
	})
}

// Test_listed tests the listed function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - None: a site holding nothing says so
//   - Several: the frequencies are named in a row
func Test_listed(t *testing.T) {
	// Verify that a site holding nothing reads as words rather than as a blank
	t.Run("None", func(t *testing.T) {
		if got := listed(nil); got != "none at all" {
			t.Errorf("got %q, wanted %q", got, "none at all")
		}
	})

	// Verify that the frequencies are listed without the scanner's padding
	t.Run("Several", func(t *testing.T) {
		got := listed([]catalog.SiteFrequency{
			{Frequency: " 851.050000MHz"},
			{Frequency: " 852.000000MHz"},
		})

		if got != "851.050000MHz, 852.000000MHz" {
			t.Errorf("got %q, wanted %q", got, "851.050000MHz, 852.000000MHz")
		}
	})
}

// Test_megahertz tests the megahertz function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Typed: a frequency as somebody writes it is read
//   - Scanner: the scanner's padded spelling reads as the same number
//   - Rounded: a frequency a float cannot hold exactly is rounded, not cut
//   - NotANumber: anything else is refused
func Test_megahertz(t *testing.T) {
	// Verify that a plain frequency comes back as a number of hertz
	t.Run("Typed", func(t *testing.T) {
		got, err := megahertz("851.050")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != 851_050_000 {
			t.Errorf("got %d hertz, wanted %d", got, 851_050_000)
		}
	})

	// Verify that the scanner's own spelling reads as the same number
	t.Run("Scanner", func(t *testing.T) {
		got, err := megahertz(" 851.050000MHz")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != 851_050_000 {
			t.Errorf("got %d hertz, wanted %d", got, 851_050_000)
		}
	})

	// Verify that a frequency a float cannot hold exactly does not land a hertz low
	t.Run("Rounded", func(t *testing.T) {
		got, err := megahertz("852.3625")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != 852_362_500 {
			t.Errorf("got %d hertz, wanted %d", got, 852_362_500)
		}
	})

	// Verify that something that is not a number is refused
	t.Run("NotANumber", func(t *testing.T) {
		_, err := megahertz("wideband")
		if err == nil {
			t.Fatal("expected an error for a value that is not a number, got none")
		}
		if !strings.Contains(err.Error(), "not a number of megahertz") {
			t.Errorf("expected the message to say it is not a number, got: %v", err)
		}
	})
}

// Test_outputFrequencies tests the outputFrequencies function with 100%
// coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - JSON: asking for JSON encodes the frequencies rather than tabulating them
//   - Text: the default output is the aligned table
//   - EncodeError: a stream the JSON cannot be written to is reported
func Test_outputFrequencies(t *testing.T) {
	found := []catalog.SiteFrequency{{Frequency: " 851.050000MHz", Index: "200"}}

	// Verify that JSON is what a caller asking for JSON gets
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := outputFrequencies(app, found); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got []catalog.SiteFrequency
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if len(got) != 1 || !strings.Contains(got[0].Frequency, "851.050000MHz") {
			t.Errorf("expected the frequency in the JSON, got: %v", got)
		}
	})

	// Verify that the default output is still the table
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}

		if err := outputFrequencies(app, found); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "FREQUENCY") {
			t.Errorf("expected the aligned table, got: %q", out.String())
		}
	})

	// Verify that a stream the JSON cannot be written to is reported
	t.Run("EncodeError", func(t *testing.T) {
		reader, writer := io.Pipe()
		reader.Close()

		app := appcontext.New()
		app.Stdout, app.Stderr = writer, &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		if err := outputFrequencies(app, found); err == nil {
			t.Error("expected an error when the JSON cannot be written, got none")
		}
	})
}

// Test_renderFrequencies tests the renderFrequencies function with 100%
// coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Empty: a site holding no frequencies is reported as an answer
//   - Table: the frequencies are written as an aligned table
//   - FlushError: a failed write is reported
func Test_renderFrequencies(t *testing.T) {
	// Verify that a site holding nothing is explained rather than failed
	t.Run("Empty", func(t *testing.T) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes

		if err := renderFrequencies(app, nil); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "no frequencies") {
			t.Errorf("expected a note about a site holding nothing, got: %q", notes.String())
		}
		if out.String() != "" {
			t.Errorf("expected nothing on the output, got: %q", out.String())
		}
	})

	// Verify that the table carries every frequency the site holds
	t.Run("Table", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}

		err := renderFrequencies(app, []catalog.SiteFrequency{
			{Frequency: " 851.050000MHz", Index: "200"},
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		written := out.String()
		if !strings.Contains(written, "FREQUENCY") || !strings.Contains(written, "851.050000MHz") {
			t.Errorf("expected the table to carry the frequency, got: %q", written)
		}
	})

	// Verify that a stream that cannot be written to is reported
	t.Run("FlushError", func(t *testing.T) {
		reader, writer := io.Pipe()
		reader.Close()

		app := appcontext.New()
		app.Stdout, app.Stderr = writer, &bytes.Buffer{}

		err := renderFrequencies(app, []catalog.SiteFrequency{{Frequency: " 851.050000MHz"}})
		if err == nil {
			t.Fatal("expected an error when the listing cannot be written, got none")
		}
		if !strings.Contains(err.Error(), "writing the frequency list") {
			t.Errorf("expected the message to say it was writing the list, got: %v", err)
		}
	})
}

// Test_rowFor tests the rowFor function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - EntriesError: a list that cannot be read is reported
//   - NotANumber: a frequency that is not a number is refused
//   - Found: the row is found by its number rather than by its text
//   - NotFound: a frequency the list does not carry is reported
func Test_rowFor(t *testing.T) {
	// Verify that a list the scanner will not show is reported
	t.Run("EntriesError", func(t *testing.T) {
		conn := &stubConn{}

		_, err := rowFor(context.Background(), device.New(conn), "851.050")
		if err == nil {
			t.Fatal("expected an error when the list cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the site's frequency list") {
			t.Errorf("expected the message to say it was reading the list, got: %v", err)
		}
	})

	// Verify that a frequency that is not a number is refused
	t.Run("NotANumber", func(t *testing.T) {
		conn := &stubConn{screens: []string{
			" 851.050000MHz", " 851.050000MHz", " 851.050000MHz",
		}}

		if _, err := rowFor(context.Background(), device.New(conn), "wideband"); err == nil {
			t.Error("expected an error for a value that is not a number, got none")
		}
	})

	// Verify that the row is matched by its number, and New Frequency passed over
	t.Run("Found", func(t *testing.T) {
		conn := &stubConn{screens: []string{
			newFrequency, " 851.050000MHz", newFrequency, " 851.050000MHz",
		}}

		row, err := rowFor(context.Background(), device.New(conn), "851.05")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if row != "851.050000MHz" {
			t.Errorf("got row %q, wanted %q", row, "851.050000MHz")
		}
	})

	// Verify that a frequency the list does not carry is reported
	t.Run("NotFound", func(t *testing.T) {
		conn := &stubConn{screens: []string{
			newFrequency, " 851.050000MHz", newFrequency, " 851.050000MHz",
		}}

		_, err := rowFor(context.Background(), device.New(conn), "852.000")
		if err == nil {
			t.Fatal("expected an error when no row carries the frequency, got none")
		}
		if !strings.Contains(err.Error(), "no row for") {
			t.Errorf("expected the message to say no row carries it, got: %v", err)
		}
	})
}

// Test_runFrequencies tests the runFrequencies function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - ResolveError: a name no site carries is reported
//   - ReadError: a failed read of the frequencies is reported
//   - JSON: the frequencies are encoded when JSON was asked for
//   - Text: the frequencies are written as a table otherwise
func Test_runFrequencies(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runFrequencies(context.Background(), app, "100"); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a site that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runFrequencies(context.Background(), app, "DOWNTOWN"); err == nil {
			t.Error("expected an error when the site cannot be resolved, got none")
		}
	})

	// Verify that a failed read of the frequencies is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,SFREQ,100": errors.New("the port is gone")}})

		if err := runFrequencies(context.Background(), app, "100"); err == nil {
			t.Error("expected an error when the frequencies cannot be read, got none")
		}
	})

	// Verify that asking for JSON encodes the frequencies rather than tabulating them
	t.Run("JSON", func(t *testing.T) {
		app, out := appWith(&stubConn{docs: map[string][]string{
			"GLT,SFREQ,100": {`<GLT><SFREQ Freq=" 851.050000MHz" Index="200"/></GLT>`},
		}})
		app.Config.Output = appcontext.OutputJSON

		if err := runFrequencies(context.Background(), app, "100"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var found []catalog.SiteFrequency
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if len(found) != 1 || !strings.Contains(found[0].Frequency, "851.050000MHz") {
			t.Errorf("expected the frequency in the JSON, got: %v", found)
		}
	})

	// Verify that the default output is the aligned table
	t.Run("Text", func(t *testing.T) {
		app, out := appWith(&stubConn{docs: map[string][]string{
			"GLT,SFREQ,100": {`<GLT><SFREQ Freq=" 851.050000MHz" Index="200"/></GLT>`},
		}})

		if err := runFrequencies(context.Background(), app, "100"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "851.050000MHz") {
			t.Errorf("expected the listing to carry the frequency, got: %q", out.String())
		}
	})
}

// Test_runFrequenciesAdd tests the runFrequenciesAdd function with 100%
// coverage.
//
// Coverage: 100% (13 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - InvalidFrequency: a frequency the screen would refuse costs no exchange
//   - ResolveError: a name no site carries is reported
//   - ReadError: a failed read of the frequencies is reported
//   - AlreadyHeld: a frequency the site holds is skipped rather than repeated
//   - AlreadyHeldJSON: a no-op add under -o json writes JSON, not a table
//   - NavigateError: a site that cannot be reached is reported
//   - AddError: a frequency that cannot be entered names itself
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the add is reported
//   - MissingAfter: a frequency that does not appear afterwards is reported
//   - JSON: the frequencies are encoded when JSON was asked for
//   - Success: the frequency is added and the site's pool written out
func Test_runFrequenciesAdd(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
		app := appcontext.New()
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		app.Stdout, app.Stderr = out, notes
		app.SetDevice(device.New(conn))
		return app, out, notes
	}

	// The site's frequency pool, before and after the frequency is added.
	empty := `<GLT></GLT>`
	holding := `<GLT><SFREQ Freq=" 851.050000MHz" Index="200"/></GLT>`

	// The frequency entry screen before and after the digits are typed, and
	// the answer a scanner that is out of the menus gives.
	emptyScreen := `<MenuInfo Name="Frequency" MenuType="TypeInput" Value="">` +
		`<MenuInput MaxLength="16" EnableKeys="0123456789."/></MenuInfo>`
	typedScreen := `<MenuInfo Name="Frequency" MenuType="TypeInput" Value="851.050">` +
		`<MenuInput MaxLength="16" EnableKeys="0123456789."/></MenuInfo>`
	outOfMenus := `<MenuInfo MenuType="TypeError"/>`

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a mistyped frequency is refused before the scanner is touched
	t.Run("InvalidFrequency", func(t *testing.T) {
		conn := &stubConn{}
		app, _, _ := appWith(conn)

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"wideband"}); err == nil {
			t.Fatal("expected an error for a frequency that is not a number, got none")
		}
		if len(conn.sent) != 0 {
			t.Errorf("expected nothing to be sent to the scanner, got: %v", conn.sent)
		}
	})

	// Verify that a site that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runFrequenciesAdd(context.Background(), app, "DOWNTOWN", []string{"851.050"}); err == nil {
			t.Error("expected an error when the site cannot be resolved, got none")
		}
	})

	// Verify that a failed read of the site's frequencies is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{fail: map[string]error{"GLT,SFREQ,100": errors.New("the port is gone")}})

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err == nil {
			t.Error("expected an error when the frequencies cannot be read, got none")
		}
	})

	// Verify that a frequency the site already holds is skipped, not added twice
	t.Run("AlreadyHeld", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{"GLT,SFREQ,100": {holding}}}
		app, out, notes := appWith(conn)

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "already has") {
			t.Errorf("expected a note that the site already holds it, got: %q", notes.String())
		}
		if !strings.Contains(out.String(), "851.050000MHz") {
			t.Errorf("expected the site's pool to be written out, got: %q", out.String())
		}
		for _, command := range conn.sent {
			if strings.HasPrefix(command, "KEY,") || strings.HasPrefix(command, "MNU,") {
				t.Errorf("expected nothing to be pressed, got: %v", conn.sent)
			}
		}
	})

	// Verify that a no-op add under -o json still writes JSON, not a table
	t.Run("AlreadyHeldJSON", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{docs: map[string][]string{"GLT,SFREQ,100": {holding}}})
		app.Config.Output = appcontext.OutputJSON

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var found []catalog.SiteFrequency
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if len(found) != 1 || !strings.Contains(found[0].Frequency, "851.050000MHz") {
			t.Errorf("expected the frequency in the JSON, got: %v", found)
		}
	})

	// Verify that a site that cannot be reached is reported
	t.Run("NavigateError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			docs: map[string][]string{"GLT,SFREQ,100": {empty}},
			fail: map[string]error{"MNU,SCAN_SITE,100": errors.New("the scanner refused the jump")},
		})

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err == nil {
			t.Error("expected an error when the site cannot be reached, got none")
		}
	})

	// Verify that a frequency that cannot be entered names itself in the failure
	t.Run("AddError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{"Set Frequencies"},
			docs:    map[string][]string{"GLT,SFREQ,100": {empty}},
		})

		err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"})
		if err == nil {
			t.Fatal("expected an error when the frequency cannot be added, got none")
		}
		if !strings.Contains(err.Error(), "adding 851.050") {
			t.Errorf("expected the message to name the frequency, got: %v", err)
		}
	})

	// Verify that a failure returning the scanner to scanning is reported
	t.Run("LeaveError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{"Set Frequencies", newFrequency},
			docs: map[string][]string{
				"GLT,SFREQ,100": {empty},
				"MSI":           {emptyScreen, typedScreen, outOfMenus},
			},
			fail: map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the add is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{"Set Frequencies", newFrequency},
			docs: map[string][]string{
				"GLT,SFREQ,100": {empty, ""},
				"MSI":           {emptyScreen, typedScreen, outOfMenus},
			},
		})

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err == nil {
			t.Error("expected an error when the frequencies cannot be read back, got none")
		}
	})

	// Verify that a frequency that does not appear afterwards is reported
	t.Run("MissingAfter", func(t *testing.T) {
		app, _, _ := appWith(&stubConn{
			screens: []string{"Set Frequencies", newFrequency},
			docs: map[string][]string{
				"GLT,SFREQ,100": {empty, empty},
				"MSI":           {emptyScreen, typedScreen, outOfMenus},
			},
		})

		err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"})
		if err == nil {
			t.Fatal("expected an error when the frequency does not appear, got none")
		}
		if !strings.Contains(err.Error(), "does not appear in the site") {
			t.Errorf("expected the message to say it did not appear, got: %v", err)
		}
	})

	// Verify that asking for JSON encodes the pool the site holds afterwards
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: []string{"Set Frequencies", newFrequency},
			docs: map[string][]string{
				"GLT,SFREQ,100": {empty, holding},
				"MSI":           {emptyScreen, typedScreen, outOfMenus},
			},
		})
		app.Config.Output = appcontext.OutputJSON

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var found []catalog.SiteFrequency
		if err := json.Unmarshal(out.Bytes(), &found); err != nil {
			t.Fatalf("the output is not JSON: %v", err)
		}
		if len(found) != 1 {
			t.Errorf("expected one frequency in the JSON, got: %v", found)
		}
	})

	// Verify that an added frequency is read back and written out as a table
	t.Run("Success", func(t *testing.T) {
		app, out, _ := appWith(&stubConn{
			screens: []string{"Set Frequencies", newFrequency},
			docs: map[string][]string{
				"GLT,SFREQ,100": {empty, holding},
				"MSI":           {emptyScreen, typedScreen, outOfMenus},
			},
		})

		if err := runFrequenciesAdd(context.Background(), app, "100", []string{"851.050"}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "851.050000MHz") {
			t.Errorf("expected the site's pool to be written out, got: %q", out.String())
		}
	})
}

// Test_runFrequenciesDelete tests the runFrequenciesDelete function with 100%
// coverage.
//
// Coverage: 100% (15 test cases covering all branches)
//
// Test cases:
//   - DeviceError: a scanner that was never named is reported
//   - InvalidFrequency: a frequency the screen would refuse is refused here
//   - ResolveError: a name no site carries is reported
//   - ReadError: a failed read of the frequencies is reported
//   - NotHeld: a frequency the site does not hold is reported as that
//   - WithoutYes: nothing is deleted without --yes
//   - NavigateError: a site that cannot be reached is reported
//   - RowError: a frequency list that cannot be read is reported
//   - SelectRowError: a row that cannot be opened is reported
//   - SelectDeleteError: a menu holding no delete entry is reported
//   - ConfirmError: a scanner not asking to confirm is reported
//   - LeaveError: a failure returning to scanning is reported
//   - ReadBackError: a failed read after the delete is reported
//   - StillThere: a frequency still in the site afterwards is reported
//   - Success: the frequency is deleted and named
func Test_runFrequenciesDelete(t *testing.T) {
	// Builds an app writing to buffers, with the fake scanner installed.
	appWith := func(conn *stubConn) (*appcontext.App, *bytes.Buffer) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout, app.Stderr = out, &bytes.Buffer{}
		app.SetDevice(device.New(conn))
		return app, out
	}

	// The site's frequency pool, before and after the frequency is removed.
	empty := `<GLT></GLT>`
	holding := `<GLT><SFREQ Freq=" 851.050000MHz" Index="200"/></GLT>`

	// The walk to the frequency and through its menu: the site's frequency
	// list, the reads that count the list round and the one that confirms it
	// came back to the start, the row itself, its delete entry, and the prompt.
	walk := []string{
		"Set Frequencies",
		" 851.050000MHz", newFrequency, " 851.050000MHz", newFrequency,
		" 851.050000MHz",
		deleteFreqEnt,
		"Confirm Delete?",
	}

	// Verify that a run with no scanner named is reported
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true); err == nil {
			t.Error("expected an error when no scanner was named, got none")
		}
	})

	// Verify that a mistyped frequency is refused before anything is read
	t.Run("InvalidFrequency", func(t *testing.T) {
		app, _ := appWith(&stubConn{})

		if err := runFrequenciesDelete(context.Background(), app, "100", "wideband", true); err == nil {
			t.Error("expected an error for a frequency that is not a number, got none")
		}
	})

	// Verify that a site that cannot be resolved is reported
	t.Run("ResolveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,FL": errors.New("the port is gone")}})

		if err := runFrequenciesDelete(context.Background(), app, "DOWNTOWN", "851.050", true); err == nil {
			t.Error("expected an error when the site cannot be resolved, got none")
		}
	})

	// Verify that a failed read of the site's frequencies is reported
	t.Run("ReadError", func(t *testing.T) {
		app, _ := appWith(&stubConn{fail: map[string]error{"GLT,SFREQ,100": errors.New("the port is gone")}})

		if err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true); err == nil {
			t.Error("expected an error when the frequencies cannot be read, got none")
		}
	})

	// Verify that a frequency the site does not hold is reported as that
	t.Run("NotHeld", func(t *testing.T) {
		app, _ := appWith(&stubConn{docs: map[string][]string{"GLT,SFREQ,100": {empty}}})

		err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true)
		if err == nil {
			t.Fatal("expected an error when the site does not hold the frequency, got none")
		}
		if !strings.Contains(err.Error(), "none at all") {
			t.Errorf("expected the message to say what the site holds, got: %v", err)
		}
	})

	// Verify that nothing is pressed until --yes is given
	t.Run("WithoutYes", func(t *testing.T) {
		conn := &stubConn{docs: map[string][]string{"GLT,SFREQ,100": {holding}}}
		app, _ := appWith(conn)

		err := runFrequenciesDelete(context.Background(), app, "100", "851.050", false)
		if err == nil {
			t.Fatal("expected an error without --yes, got none")
		}
		if !strings.Contains(err.Error(), "--yes") {
			t.Errorf("expected the message to ask for --yes, got: %v", err)
		}
		for _, command := range conn.sent {
			if strings.HasPrefix(command, "KEY,") || strings.HasPrefix(command, "MNU,") {
				t.Errorf("expected nothing to be pressed, got: %v", conn.sent)
			}
		}
	})

	// Verify that a site that cannot be reached is reported
	t.Run("NavigateError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			docs: map[string][]string{"GLT,SFREQ,100": {holding}},
			fail: map[string]error{"MNU,SCAN_SITE,100": errors.New("the scanner refused the jump")},
		})

		if err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true); err == nil {
			t.Error("expected an error when the site cannot be reached, got none")
		}
	})

	// Verify that a frequency list that cannot be read is reported
	t.Run("RowError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: []string{"Set Frequencies"},
			docs:    map[string][]string{"GLT,SFREQ,100": {holding}},
		})

		err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true)
		if err == nil {
			t.Fatal("expected an error when the list cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the site's frequency list") {
			t.Errorf("expected the message to say it was reading the list, got: %v", err)
		}
	})

	// Verify that a row that cannot be opened is reported
	t.Run("SelectRowError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk[:5],
			docs:    map[string][]string{"GLT,SFREQ,100": {holding}},
		})

		err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true)
		if err == nil {
			t.Fatal("expected an error when the row cannot be opened, got none")
		}
		if !strings.Contains(err.Error(), "looking for 851.050 in the site") {
			t.Errorf("expected the message to name the frequency, got: %v", err)
		}
	})

	// Verify that a menu with no delete entry is reported
	t.Run("SelectDeleteError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk[:6],
			docs:    map[string][]string{"GLT,SFREQ,100": {holding}},
		})

		err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true)
		if err == nil {
			t.Fatal("expected an error when the delete entry cannot be found, got none")
		}
		if !strings.Contains(err.Error(), deleteFreqEnt) {
			t.Errorf("expected the message to name %q, got: %v", deleteFreqEnt, err)
		}
	})

	// Verify that a scanner not asking to confirm is reported
	t.Run("ConfirmError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: append(append([]string{}, walk[:7]...), "Scanning"),
			docs:    map[string][]string{"GLT,SFREQ,100": {holding}},
		})

		err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true)
		if err == nil {
			t.Fatal("expected an error when the scanner is not asking to confirm, got none")
		}
		if !strings.Contains(err.Error(), "nothing was deleted") {
			t.Errorf("expected the message to say nothing was deleted, got: %v", err)
		}
	})

	// Verify that a failure returning the scanner to scanning is reported
	t.Run("LeaveError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"GLT,SFREQ,100": {holding, empty}},
			fail:    map[string]error{"GSI": errors.New("the port is gone")},
		})

		if err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true); err == nil {
			t.Error("expected an error when the scanner cannot be returned to scanning, got none")
		}
	})

	// Verify that a failed read after the delete is reported
	t.Run("ReadBackError", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"GLT,SFREQ,100": {holding, ""}},
		})

		if err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true); err == nil {
			t.Error("expected an error when the frequencies cannot be read back, got none")
		}
	})

	// Verify that a frequency still in the site afterwards is reported
	t.Run("StillThere", func(t *testing.T) {
		app, _ := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"GLT,SFREQ,100": {holding}},
		})

		err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true)
		if err == nil {
			t.Fatal("expected an error when the frequency is still there, got none")
		}
		if !strings.Contains(err.Error(), "nothing was deleted") {
			t.Errorf("expected the message to say nothing was deleted, got: %v", err)
		}
	})

	// Verify that a deleted frequency is named on the output
	t.Run("Success", func(t *testing.T) {
		app, out := appWith(&stubConn{
			screens: walk,
			docs:    map[string][]string{"GLT,SFREQ,100": {holding, empty}},
		})

		if err := runFrequenciesDelete(context.Background(), app, "100", "851.050", true); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "deleted 851.050") {
			t.Errorf("expected the output to name the deleted frequency, got: %q", out.String())
		}
	})
}

// Test_validFrequency tests the validFrequency function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Valid: a frequency in megahertz is accepted and typed as it was written
//   - NotANumber: anything that is not a number is refused
//   - Exponent: a number the entry screen has no keys for is refused
//   - Units: the unit spellings the tune command takes are taken here too, and
//     are rewritten into what the entry screen has keys for
func Test_validFrequency(t *testing.T) {
	// Verify that a frequency written the way the screen asks for it is
	// accepted, and is typed exactly as it was written rather than rewritten.
	t.Run("Valid", func(t *testing.T) {
		typed, err := validFrequency("851.050")
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if typed != "851.050" {
			t.Errorf("a plain frequency would be typed as %q, want it left as it was written", typed)
		}
	})

	// Verify that a frequency carrying a unit is accepted whatever the case,
	// which is what this command used to refuse while tune took it, and that it
	// comes back as digits and a decimal point. The entry screen has no letter
	// keys, so what is typed cannot be what was written here.
	t.Run("Units", func(t *testing.T) {
		for _, value := range []string{"851.05MHz", "851.05mhz", "851.05 MHZ", "851050khz"} {
			typed, err := validFrequency(value)
			if err != nil {
				t.Errorf("validFrequency(%q) refused a frequency tune would take: %v", value, err)
				continue
			}
			if typed != "851.05" {
				t.Errorf("validFrequency(%q) would type %q, want %q", value, typed, "851.05")
			}
		}
	})

	// Verify that something that is not a number is refused with the advice.
	// The value deliberately carries no "e", which would be caught by the
	// screen check above instead and leave this branch unreached.
	t.Run("NotANumber", func(t *testing.T) {
		_, err := validFrequency("VHF")
		if err == nil {
			t.Fatal("expected an error for a value that is not a number, got none")
		}
		if !strings.Contains(err.Error(), "is not a number of megahertz") {
			t.Errorf("expected the message to say it is not a number, got: %v", err)
		}
		if !strings.Contains(err.Error(), "write it in megahertz") {
			t.Errorf("expected the message to say how to write it, got: %v", err)
		}
	})

	// Verify that a number the screen has no keys for is refused
	t.Run("Exponent", func(t *testing.T) {
		_, err := validFrequency("8.5105e2")
		if err == nil {
			t.Fatal("expected an error for a number written with an exponent, got none")
		}
		if !strings.Contains(err.Error(), "would accept") {
			t.Errorf("expected the message to say the screen would not accept it, got: %v", err)
		}
	})
}
