// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package battery

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

// fakeConn is a device.Conn that answers every command with a canned reply, so
// the command can be driven with no scanner attached.
type fakeConn struct {
	reply string
	err   error
}

// Info describes the connected scanner.
func (f fakeConn) Info() device.Info { return device.Info{} }

// Execute returns the canned reply, which stands in for the scanner's answer.
func (f fakeConn) Execute(ctx context.Context, command string) (string, error) {
	return f.reply, f.err
}

// ExecuteXML returns the canned reply, which stands in for an XML answer.
func (f fakeConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return f.reply, f.err
}

// Send reports the canned error, since nothing here writes without reading.
func (f fakeConn) Send(ctx context.Context, command string) error { return f.err }

// Close releases nothing, because there is no port.
func (f fakeConn) Close() error { return nil }

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and marked as a command that only reads
//   - Runs: executing the command reports the battery it was given
func TestNew(t *testing.T) {
	// Verify the command carries the name and the annotation the tool wires on
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "battery" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "battery")
		}
		if got := cmd.Annotations[appcontext.OnlyReads]; got != "true" {
			t.Errorf("the only-reads annotation is %q, wanted %q", got, "true")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify running the command reads the scanner and writes what it read
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: "CST=6,VOLT=3730mV:023%,CURR=0643mA,TEMP=36.5C"}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "charge:      23%") {
			t.Errorf("the command wrote %q, wanted it to report the charge", out.String())
		}
	})
}

// Test_direction covers the three answers the current can have.
//
// Coverage: 100% (3 test cases covering every branch of the switch)
//
// Test cases:
//   - Charging: a positive current is going into the battery
//   - Discharging: a negative current is coming out of it
//   - NoFlow: a current of zero is moving neither way
func Test_direction(t *testing.T) {
	// Verify current going into the battery reads as charging
	t.Run("Charging", func(t *testing.T) {
		if got := direction(643); got != "charging" {
			t.Errorf("direction is %q, wanted %q", got, "charging")
		}
	})

	// Verify current coming out of the battery reads as discharging
	t.Run("Discharging", func(t *testing.T) {
		if got := direction(-643); got != "discharging" {
			t.Errorf("direction is %q, wanted %q", got, "discharging")
		}
	})

	// Verify no current at all is reported rather than left to the sign
	t.Run("NoFlow", func(t *testing.T) {
		if got := direction(0); got != "no flow" {
			t.Errorf("direction is %q, wanted %q", got, "no flow")
		}
	})
}

// Test_fahrenheit covers the conversion the scanner does not do itself.
//
// Coverage: 100% (3 test cases covering the single expression)
//
// Test cases:
//   - Freezing: zero Celsius is 32 Fahrenheit
//   - Warm: a room temperature reading converts as expected
//   - Negative: a reading below zero converts as expected
func Test_fahrenheit(t *testing.T) {
	// Verify the anchor point of the scale converts correctly
	t.Run("Freezing", func(t *testing.T) {
		if got := fahrenheit(0); got != 32 {
			t.Errorf("fahrenheit(0) is %v, wanted 32", got)
		}
	})

	// Verify an ordinary battery temperature converts correctly
	t.Run("Warm", func(t *testing.T) {
		if got := fahrenheit(100); got != 212 {
			t.Errorf("fahrenheit(100) is %v, wanted 212", got)
		}
	})

	// Verify a reading below zero keeps its sign through the conversion
	t.Run("Negative", func(t *testing.T) {
		if got := fahrenheit(-40); got != -40 {
			t.Errorf("fahrenheit(-40) is %v, wanted -40", got)
		}
	})
}

// Test_renderBattery covers the text output, which is the rows and the fault note.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Healthy: every reading is written and nothing interrupts
//   - Faulted: a charger fault adds a note on stderr
func Test_renderBattery(t *testing.T) {
	// Verify a normal reading is written as rows and nothing else
	t.Run("Healthy", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes

		b := device.Battery{State: device.ChargeCharging, Millivolts: 3730, Percent: 23, Milliamps: 643, Celsius: 36.5}
		r := report{
			State:      b.State.String(),
			Percent:    b.Percent,
			Volts:      b.Volts(),
			Milliamps:  b.Milliamps,
			Celsius:    b.Celsius,
			Fahrenheit: fahrenheit(b.Celsius),
		}

		if err := renderBattery(app, b, r); err != nil {
			t.Fatalf("renderBattery: %v", err)
		}

		want := "charge:      23%\n" +
			"state:       charging\n" +
			"voltage:     3.73 V\n" +
			"current:     643 mA (charging)\n" +
			"temperature: 36.5 C (97.7 F)\n"
		if got := out.String(); got != want {
			t.Errorf("renderBattery wrote %q, wanted %q", got, want)
		}
		if notes.Len() != 0 {
			t.Errorf("renderBattery noted %q, wanted nothing", notes.String())
		}
	})

	// Verify a charger fault is called out rather than left in the numbers
	t.Run("Faulted", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes

		b := device.Battery{State: device.ChargeTemperatureFault, Millivolts: 3730, Percent: 23, Milliamps: -643, Celsius: 36.5}
		r := report{
			State:       b.State.String(),
			Percent:     b.Percent,
			Volts:       b.Volts(),
			Milliamps:   b.Milliamps,
			Celsius:     b.Celsius,
			Fahrenheit:  fahrenheit(b.Celsius),
			NeedsAction: b.State.Faulted(),
		}

		if err := renderBattery(app, b, r); err != nil {
			t.Fatalf("renderBattery: %v", err)
		}

		if !strings.Contains(out.String(), "current:     -643 mA (discharging)") {
			t.Errorf("renderBattery wrote %q, wanted it to report the current flowing out", out.String())
		}
		if !strings.Contains(notes.String(), "abnormal temperature") {
			t.Errorf("the note is %q, wanted it to name the fault", notes.String())
		}
	})
}

// Test_run covers reading the battery and both ways of reporting it.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Text: the reading is written as rows for a person
//   - JSON: the reading is written as JSON, converted and named
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer is reported
func Test_run(t *testing.T) {
	// The scanner's answer to GCS, which names its fields rather than ordering
	// them: charging, 3.73 volts, 23 percent, 643 mA going in, 36.5 degrees.
	const reading = "CST=6,VOLT=3730mV:023%,CURR=0643mA,TEMP=36.5C"

	// Verify a reading from the scanner comes out as rows of text
	t.Run("Text", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{reply: reading}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}

		want := "charge:      23%\n" +
			"state:       charging\n" +
			"voltage:     3.73 V\n" +
			"current:     643 mA (charging)\n" +
			"temperature: 36.5 C (97.7 F)\n"
		if got := out.String(); got != want {
			t.Errorf("run wrote %q, wanted %q", got, want)
		}
	})

	// Verify JSON output carries the converted readings and the named state
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON
		app.SetDevice(device.New(fakeConn{reply: reading}))

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("run: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if got.State != "charging" || !got.Charging {
			t.Errorf("the state is %q and charging is %v, wanted a charging battery", got.State, got.Charging)
		}
		if got.Percent != 23 || got.Volts != 3.73 || got.Milliamps != 643 {
			t.Errorf("the reading is %+v, wanted 23 percent at 3.73 volts and 643 mA", got)
		}
		if got.Celsius != 36.5 || got.Fahrenheit != 97.7 {
			t.Errorf("the temperature is %v C and %v F, wanted 36.5 C and 97.7 F", got.Celsius, got.Fahrenheit)
		}
		if got.NeedsAction {
			t.Error("the reading asks for action, wanted a healthy charger")
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := run(context.Background(), app)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("run reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that fails to answer is reported as a failed read
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(fakeConn{err: errors.New("the port closed")}))

		err := run(context.Background(), app)
		if err == nil {
			t.Fatal("run reported nothing, wanted the failed read")
		}
		if !strings.Contains(err.Error(), "reading the battery") {
			t.Errorf("run reported %q, wanted it to name reading the battery", err)
		}
	})
}
