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

// TestBattery tests the Battery method with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - Success: the named fields are read into the battery's readings
//   - Error: a failed exchange is reported
//   - BadState: a charge state that is not a number is reported
//   - BadVoltage: a voltage that is not a number is reported
//   - BadPercent: a capacity that is not a number is reported
//   - BadCurrent: a current that is not a number is reported
//   - BadTemperature: a temperature that is not a number is reported
func TestBattery(t *testing.T) {
	// A reply as an SDS150 sends it, with the undocumented trailing field the
	// firmware appends.
	const reply = "CST=6,VOLT=3730mV:023%,CURR=0643mA,TEMP=36.65C,XTRA=1"

	answering := func(value string) *stubConn {
		return &stubConn{exec: func(string) (string, error) { return value, nil }}
	}

	// Verify that every named field lands in the right reading.
	t.Run("Success", func(t *testing.T) {
		c := answering(reply)
		b, err := New(c).Battery(context.Background())
		if err != nil {
			t.Fatalf("reading the battery: %v", err)
		}
		if c.last() != "GCS" {
			t.Errorf("sent %q, want GCS", c.last())
		}
		want := Battery{State: ChargeCharging, Millivolts: 3730, Percent: 23, Milliamps: 643, Celsius: 36.65}
		if b != want {
			t.Errorf("got %+v, want %+v", b, want)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).Battery(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that each malformed field is reported by name.
	for _, tt := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "BadState", value: "CST=x,VOLT=3730mV:023%,CURR=0643mA,TEMP=36.65C", want: "invalid charge state"},
		{name: "BadVoltage", value: "CST=6,VOLT=xxxxmV:023%,CURR=0643mA,TEMP=36.65C", want: "invalid battery voltage"},
		{name: "BadPercent", value: "CST=6,VOLT=3730mV:xxx%,CURR=0643mA,TEMP=36.65C", want: "invalid battery capacity"},
		{name: "BadCurrent", value: "CST=6,VOLT=3730mV:023%,CURR=xxxxmA,TEMP=36.65C", want: "invalid battery current"},
		{name: "BadTemperature", value: "CST=6,VOLT=3730mV:023%,CURR=0643mA,TEMP=xx.xxC", want: "invalid battery temperature"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(answering(tt.value)).Battery(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

// TestBatteryCharging tests the Battery Charging method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Gaining: charging and topping up both count as gaining charge
//   - Otherwise: every other state does not
func TestBatteryCharging(t *testing.T) {
	// Verify that both of the charger's working states count.
	t.Run("Gaining", func(t *testing.T) {
		for _, state := range []ChargeState{ChargeCharging, ChargeTopUp} {
			if !(Battery{State: state}).Charging() {
				t.Errorf("state %v does not report itself charging", state)
			}
		}
	})

	// Verify that nothing else does.
	t.Run("Otherwise", func(t *testing.T) {
		for _, state := range []ChargeState{ChargeNone, ChargeInitializing, ChargeFull, ChargePowerFault} {
			if (Battery{State: state}).Charging() {
				t.Errorf("state %v reports itself charging", state)
			}
		}
	})
}

// TestChargeStateFaulted tests the ChargeState Faulted method with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Faults: the two fault states report themselves
//   - Otherwise: the states reporting progress do not
func TestChargeStateFaulted(t *testing.T) {
	// Verify that both faults report themselves.
	t.Run("Faults", func(t *testing.T) {
		for _, state := range []ChargeState{ChargeTemperatureFault, ChargePowerFault} {
			if !state.Faulted() {
				t.Errorf("state %v does not report itself faulted", state)
			}
		}
	})

	// Verify that ordinary progress is not a fault.
	t.Run("Otherwise", func(t *testing.T) {
		for _, state := range []ChargeState{ChargeNone, ChargeInitializing, ChargeFull, ChargeTopUp, ChargeCharging} {
			if state.Faulted() {
				t.Errorf("state %v reports itself faulted", state)
			}
		}
	})
}

// TestChargeStateString tests the ChargeState String method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Named: every state the scanner reports has words
//   - Unknown: a code this package does not name says so, carrying the code
func TestChargeStateString(t *testing.T) {
	// Verify that each state reads in words.
	t.Run("Named", func(t *testing.T) {
		for state, want := range map[ChargeState]string{
			ChargeNone:             "not charging",
			ChargeInitializing:     "initializing",
			ChargeTemperatureFault: "abnormal temperature",
			ChargePowerFault:       "abnormal power",
			ChargeFull:             "fully charged",
			ChargeTopUp:            "topping up",
			ChargeCharging:         "charging",
		} {
			if got := state.String(); got != want {
				t.Errorf("state %d reads %q, want %q", int(state), got, want)
			}
		}
	})

	// Verify that an unnamed code carries itself.
	t.Run("Unknown", func(t *testing.T) {
		if got := ChargeState(9).String(); got != "unknown(9)" {
			t.Errorf("got %q, want unknown(9)", got)
		}
	})
}

// TestBatteryVolts tests the Battery Volts method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: millivolts are divided down to volts
func TestBatteryVolts(t *testing.T) {
	// Verify that the conversion keeps the fraction.
	t.Run("Success", func(t *testing.T) {
		if got := (Battery{Millivolts: 3730}).Volts(); got != 3.73 {
			t.Errorf("got %v, want 3.73", got)
		}
	})
}

// Test_parseCelsius tests the parseCelsius function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the suffix and padding come off and the decimal is read
//   - Empty: an empty field is zero rather than an error
//   - Invalid: anything that is not a number is reported
func Test_parseCelsius(t *testing.T) {
	// Verify that the C suffix and the padding are not part of the number.
	t.Run("Success", func(t *testing.T) {
		got, err := parseCelsius("  36.65C  ")
		if err != nil || got != 36.65 {
			t.Fatalf("got %v, %v, want 36.65", got, err)
		}
	})

	// Verify that an empty field reads as zero.
	t.Run("Empty", func(t *testing.T) {
		got, err := parseCelsius("   ")
		if err != nil || got != 0 {
			t.Fatalf("got %v, %v, want zero", got, err)
		}
	})

	// Verify that anything else is reported.
	t.Run("Invalid", func(t *testing.T) {
		_, err := parseCelsius("warmC")
		if err == nil || !strings.Contains(err.Error(), "invalid battery temperature") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// Test_parseChargeState tests the parseChargeState function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a code the package names is read
//   - Unnamed: a code it does not name is still read rather than refused
//   - Invalid: a field that is not a number is reported
func Test_parseChargeState(t *testing.T) {
	// Verify that a named code is read.
	t.Run("Success", func(t *testing.T) {
		got, err := parseChargeState(" 6 ")
		if err != nil || got != ChargeCharging {
			t.Fatalf("got %v, %v, want charging", got, err)
		}
	})

	// Verify that an unnamed code is carried rather than refused.
	t.Run("Unnamed", func(t *testing.T) {
		got, err := parseChargeState("9")
		if err != nil || got != ChargeState(9) {
			t.Fatalf("got %v, %v, want the code carried", got, err)
		}
	})

	// Verify that a field that is not a number is reported.
	t.Run("Invalid", func(t *testing.T) {
		_, err := parseChargeState("full")
		if err == nil || !strings.Contains(err.Error(), "invalid charge state") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// Test_parseUnit tests the parseUnit function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the suffix comes off and the zero padded digits read as base ten
//   - Signed: the padded sign of the current field comes off too
//   - Empty: an empty field is zero rather than an error
//   - Invalid: anything that is not a number is reported
func Test_parseUnit(t *testing.T) {
	// Verify that zero padding does not make the number octal.
	t.Run("Success", func(t *testing.T) {
		got, err := parseUnit("battery current", "0643mA", "mA")
		if err != nil || got != 643 {
			t.Fatalf("got %d, %v, want 643", got, err)
		}
	})

	// Verify that a signed reading is read with its sign.
	t.Run("Signed", func(t *testing.T) {
		if got, err := parseUnit("battery current", "+0643mA", "mA"); err != nil || got != 643 {
			t.Fatalf("got %d, %v, want 643", got, err)
		}
		if got, err := parseUnit("battery current", "-0643mA", "mA"); err != nil || got != -643 {
			t.Fatalf("got %d, %v, want -643", got, err)
		}
	})

	// Verify that an empty field reads as zero.
	t.Run("Empty", func(t *testing.T) {
		got, err := parseUnit("battery capacity", "  ", "%")
		if err != nil || got != 0 {
			t.Fatalf("got %d, %v, want zero", got, err)
		}
	})

	// Verify that anything else is reported.
	t.Run("Invalid", func(t *testing.T) {
		_, err := parseUnit("battery voltage", "lotsmV", "mV")
		if err == nil || !strings.Contains(err.Error(), "invalid battery voltage") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}
