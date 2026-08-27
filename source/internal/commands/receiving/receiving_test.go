// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package receiving

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// The two documents these tests are driven with.
//
// Both are real, read off an SDS150 on firmware 1.00.37 with the names changed
// to places inside the National Radio Quiet Zone, as every example in this tool
// is. They are kept whole rather than trimmed to the attributes under test,
// because the point of having them is that the spellings are the radio's own.
const (
	// scanning is a radio stepping past a channel without stopping. Note that
	// it still names one, which is the trap this command exists to make
	// visible: only Mute separates this from the document below.
	scanning = `<ScannerInfo Mode="Scan Mode" V_Screen="conventional_scan">` +
		`<MonitorList Name="Pocahontas County" Index="2" ListType="FL"/>` +
		`<System Name="PUBLIC SAFETY" Index="10" SystemType="Conventional"/>` +
		`<Department Name="PUBLIC WORKS" Index="17"/>` +
		`<ConvFrequency Name="CH 2" Index="23" Freq=" 151.025000MHz" Mod="NFM"` +
		` TGID="TGID None" U_Id="UID None"/>` +
		`<Property VOL="13" SQL="5" Sig="0" Rec="Off" Mute="Mute" Rssi="-999"/>` +
		`</ScannerInfo>`

	// stopped is the same radio a moment into a transmission. The gate is open
	// and the signal reading has not caught up, which is why Sig is still zero.
	stopped = `<ScannerInfo Mode="Scan Mode" V_Screen="conventional_scan">` +
		`<MonitorList Name="Pocahontas County" Index="2" ListType="FL"/>` +
		`<System Name="PUBLIC SAFETY" Index="10" SystemType="Conventional"/>` +
		`<Department Name="POLICE DEPARTMENT" Index="13"/>` +
		`<ConvFrequency Name="MARLINTON DISPATCH" Index="15" Freq=" 155.550000MHz" Mod="NFM"` +
		` TGID="TGID None" U_Id="UID None"/>` +
		`<Property VOL="13" SQL="5" Sig="0" Rec="Off" Mute="Unmute" Rssi="-87"/>` +
		`</ScannerInfo>`

	// trunked is a talkgroup rather than a frequency, with a unit id decoded.
	// It is assembled from the specification rather than read off hardware,
	// because the radio tested is on a conventional system. See device.Talkgroup.
	trunked = `<ScannerInfo Mode="Trunk Scan" V_Screen="trunk_scan">` +
		`<MonitorList Name="Pocahontas County" Index="2" ListType="FL"/>` +
		`<System Name="STATE POLICE" Index="4" SystemType="P25 Trunk"/>` +
		`<Department Name="TROOP A" Index="6"/>` +
		`<Site Name="Bald Knob" Index="1"/>` +
		`<TGID Name="FIREGROUND" Index="9" TGID="24944" U_Id="32"/>` +
		`<Property VOL="13" SQL="5" Sig="4" Rec="Off" Mute="Unmute" Rssi="-72"/>` +
		`</ScannerInfo>`
)

// fakeConn is a device.Conn that answers each command from a function the test
// supplies, so the command can be driven with no scanner attached.
type fakeConn struct {
	info  device.Info
	reply func(command string) (string, error)
}

// Info describes the connected scanner.
func (f fakeConn) Info() device.Info { return f.info }

// Execute answers the command the way the test asked it to.
func (f fakeConn) Execute(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// ExecuteXML answers the command the way the test asked it to.
func (f fakeConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// Send reports whatever answering the command would have reported.
func (f fakeConn) Send(ctx context.Context, command string) error {
	_, err := f.reply(command)
	return err
}

// Close releases nothing, because there is no port.
func (f fakeConn) Close() error { return nil }

// answering returns an App wired to a scanner that replies with doc.
//
// Parameters:
//   - doc: the GSI document the fake scanner answers with
//
// Returns:
//   - the App, with its streams replaced by buffers
//   - what the command wrote to stdout
//   - what the command wrote to stderr
func answering(doc string) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, errs
	app.SetDevice(device.New(fakeConn{
		info:  device.Info{Port: "/dev/example", Model: "SDS150"},
		reply: func(string) (string, error) { return doc, nil },
	}))
	return app, out, errs
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and marked as a command that only reads
//   - Runs: executing the command reports what the scanner is hearing
func TestNew(t *testing.T) {
	// Verify the command carries the name and the annotation the tool wires on.
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "receiving" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "receiving")
		}
		if got := cmd.Annotations[appcontext.OnlyReads]; got != "true" {
			t.Errorf("the only-reads annotation is %q, wanted %q", got, "true")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify running the command reads the scanner and writes what it read.
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := answering(stopped)

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "MARLINTON DISPATCH") {
			t.Errorf("the command wrote %q, wanted it to name the channel", out.String())
		}
	})
}

// TestReadingRealDocuments covers what this command reports for each of the
// documents a scanner actually sends, which is where a conventional channel and
// a trunked talkgroup are told apart.
//
// The flattening itself lives in device.ScannerInfo.Heard, because the recorder
// reads it back over a socket and the two must agree. What is checked here is
// that this command's answer is right for a real document, end to end.
//
// Test cases:
//   - Conventional: the frequency is reported and the talkgroup left empty
//   - Trunked: the talkgroup and unit are reported and the frequency left empty
//   - Scanning: a radio that stopped on nothing still names a channel
func TestReadingRealDocuments(t *testing.T) {
	read := func(t *testing.T, doc string) report {
		t.Helper()
		info, err := device.New(fakeConn{reply: func(string) (string, error) {
			return doc, nil
		}}).ScannerInfo(context.Background())
		if err != nil {
			t.Fatalf("reading the document: %v", err)
		}
		return info.Heard()
	}

	// Verify a conventional channel lands in the frequency field, with the
	// scanner's leading space and its words for an absent unit id gone.
	t.Run("Conventional", func(t *testing.T) {
		r := read(t, stopped)

		if !r.Receiving {
			t.Error("the gate is open and receiving said otherwise")
		}
		if r.Channel != "MARLINTON DISPATCH" || r.Frequency != "155.550000MHz" {
			t.Errorf("got %+v, want the conventional channel", r)
		}
		if r.Talkgroup != "" {
			t.Errorf("got talkgroup %q, want none on a conventional system", r.Talkgroup)
		}
		if r.Unit != "" {
			t.Errorf("got unit %q, want the scanner's \"UID None\" read as empty", r.Unit)
		}
		if r.List != "Pocahontas County" || r.System != "PUBLIC SAFETY" || r.Department != "POLICE DEPARTMENT" {
			t.Errorf("got %+v, want the hierarchy above the channel", r)
		}
	})

	// Verify a talkgroup lands in its own field, and brings a site and a unit
	// with it.
	t.Run("Trunked", func(t *testing.T) {
		r := read(t, trunked)

		if r.Talkgroup != "24944" || r.Channel != "FIREGROUND" {
			t.Errorf("got %+v, want the talkgroup", r)
		}
		if r.Frequency != "" {
			t.Errorf("got frequency %q, want none on a trunked system", r.Frequency)
		}
		if r.Site != "Bald Knob" || r.Unit != "32" {
			t.Errorf("got site %q and unit %q, want them read", r.Site, r.Unit)
		}
	})

	// Verify the trap: a scanning radio names a channel it never stopped on,
	// and only Receiving says so.
	t.Run("Scanning", func(t *testing.T) {
		r := read(t, scanning)

		if r.Receiving {
			t.Error("the scanner is muted and receiving said it was not")
		}
		if r.Channel == "" {
			t.Error("a scanning radio named no channel, so this test proves nothing")
		}
	})
}

// Test_run covers reading the scanner and both ways of reporting it.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Text: a transmission is written as a block of labelled lines
//   - Trunked: a talkgroup replaces the frequency line and adds a site and unit
//   - Scanning: a radio on nothing is called out on stderr
//   - JSON: the same reading is written as one object
//   - NoDevice: a run with no scanner named is refused
//   - InfoFails: a scanner that will not say what it is hearing is reported
func Test_run(t *testing.T) {
	// Verify a transmission is written out with its channel and frequency.
	t.Run("Text", func(t *testing.T) {
		app, out, errs := answering(stopped)
		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running the command: %v", err)
		}

		for _, want := range []string{"receiving:  yes", "MARLINTON DISPATCH", "155.550000MHz"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("the command wrote %q, wanted %q in it", out.String(), want)
			}
		}
		if errs.Len() != 0 {
			t.Errorf("a live transmission wrote %q to stderr, wanted nothing", errs.String())
		}
	})

	// Verify a trunked system reports its talkgroup, site and unit instead.
	t.Run("Trunked", func(t *testing.T) {
		app, out, _ := answering(trunked)
		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running the command: %v", err)
		}

		for _, want := range []string{"talkgroup:  24944", "site:       Bald Knob", "unit:       32"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("the command wrote %q, wanted %q in it", out.String(), want)
			}
		}
		if strings.Contains(out.String(), "frequency:") {
			t.Error("a trunked system reported a frequency line")
		}
	})

	// Verify a radio that stopped on nothing says so, since the channel it
	// names would otherwise read as one it was listening to.
	t.Run("Scanning", func(t *testing.T) {
		app, out, errs := answering(scanning)
		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running the command: %v", err)
		}

		if !strings.Contains(out.String(), "receiving:  no") {
			t.Errorf("the command wrote %q, wanted it to report nothing received", out.String())
		}
		if !strings.Contains(errs.String(), "scanned past") {
			t.Errorf("stderr held %q, wanted the warning that the channel was only passed", errs.String())
		}
	})

	// Verify the same reading as one object, which is what an agent reads.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := answering(stopped)
		app.Config.Output = appcontext.OutputJSON

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running the command: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the command wrote %q, which is not JSON: %v", out.String(), err)
		}
		if !got.Receiving || got.Channel != "MARLINTON DISPATCH" || got.RSSI != "-87" {
			t.Errorf("got %+v, want the transmission", got)
		}
	})

	// Verify a run with no scanner named is refused rather than reported empty.
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}

		if err := run(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("got %v, want ErrNoDevice", err)
		}
	})

	// Verify a scanner that will not answer is reported rather than rendered
	// as a radio hearing nothing.
	t.Run("InfoFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout, app.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: func(string) (string, error) {
			return "", errors.New("the port closed")
		}}))

		err := run(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "asking the scanner what it is hearing") {
			t.Fatalf("got %v, want the read reported", err)
		}
	})
}
