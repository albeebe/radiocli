// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ChargeState is what the battery charger is doing.
type ChargeState int

// Battery returns the battery's charge and condition.
//
// The reply is the one place the protocol uses named fields rather than
// position, in the form CST=6,VOLT=3730mV:023%,CURR=0643mA,TEMP=36.65C. This
// firmware appends one further field the specification does not document,
// which is ignored.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Battery holding the charger's state, the voltage, the remaining
//     capacity, the current flowing, and the temperature
//   - error if the exchange fails or any of the named fields is malformed
func (s *Scanner) Battery(ctx context.Context) (Battery, error) {
	value, err := s.conn.Execute(ctx, "GCS")
	if err != nil {
		return Battery{}, err
	}

	named := map[string]string{}
	for _, part := range strings.Split(value, ",") {
		key, val, found := strings.Cut(part, "=")
		if found {
			named[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}

	var b Battery
	if b.State, err = parseChargeState(named["CST"]); err != nil {
		return Battery{}, err
	}

	// VOLT carries two readings joined by a colon: millivolts and percent.
	millivolts, percent, _ := strings.Cut(named["VOLT"], ":")
	if b.Millivolts, err = parseUnit("battery voltage", millivolts, "mV"); err != nil {
		return Battery{}, err
	}
	if b.Percent, err = parseUnit("battery capacity", percent, "%"); err != nil {
		return Battery{}, err
	}
	if b.Milliamps, err = parseUnit("battery current", named["CURR"], "mA"); err != nil {
		return Battery{}, err
	}
	if b.Celsius, err = parseCelsius(named["TEMP"]); err != nil {
		return Battery{}, err
	}

	return b, nil
}

// Charging reports whether the battery is gaining charge.
//
// Returns:
//   - true if the charger is charging or topping up, false otherwise
func (b Battery) Charging() bool {
	return b.State == ChargeCharging || b.State == ChargeTopUp
}

// Faulted reports whether the charger is in a state that needs attention
// rather than simply reporting progress.
//
// Returns:
//   - true if the charger reports an abnormal temperature or abnormal power,
//     false otherwise
func (c ChargeState) Faulted() bool {
	return c == ChargeTemperatureFault || c == ChargePowerFault
}

// String returns a description of the state, in words rather than as a code.
//
// Returns:
//   - the state in words, such as "charging" or "fully charged", or
//     "unknown(n)" for a code this package does not name
func (c ChargeState) String() string {
	switch c {
	case ChargeNone:
		return "not charging"
	case ChargeInitializing:
		return "initializing"
	case ChargeTemperatureFault:
		return "abnormal temperature"
	case ChargePowerFault:
		return "abnormal power"
	case ChargeFull:
		return "fully charged"
	case ChargeTopUp:
		return "topping up"
	case ChargeCharging:
		return "charging"
	default:
		return fmt.Sprintf("unknown(%d)", int(c))
	}
}

// Volts returns the battery voltage in volts.
//
// Returns:
//   - the voltage in volts, converted from the millivolts the scanner reports
func (b Battery) Volts() float64 {
	return float64(b.Millivolts) / 1000
}

// parseCelsius reads the TEMP field, which carries a decimal and a C suffix
// and is padded with spaces.
//
// Parameters:
//   - value: the TEMP field as the scanner sent it, such as "36.65C"
//
// Returns:
//   - the temperature in degrees Celsius, or zero if the field is empty
//   - error if what is left after the suffix is not a number
func parseCelsius(value string) (float64, error) {
	digits := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "C"))
	if digits == "" {
		return 0, nil
	}

	f, err := strconv.ParseFloat(digits, 64)
	if err != nil {
		return 0, fmt.Errorf("response to %q has an invalid battery temperature: %q", "GCS", value)
	}
	return f, nil
}

// parseChargeState reads the CST field.
//
// Parameters:
//   - value: the CST field as the scanner sent it, such as "6"
//
// Returns:
//   - the state the code stands for, unnamed codes included
//   - error if the field is not a number
func parseChargeState(value string) (ChargeState, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("response to %q has an invalid charge state: %q", "GCS", value)
	}
	return ChargeState(n), nil
}

// parseUnit reads a numeric field carrying its unit as a suffix. The scanner
// zero pads these, and pads the sign of the current field, so the digits are
// taken after stripping the unit.
//
// Parameters:
//   - field: what the field measures, for the error message
//   - value: the field as the scanner sent it, such as "0643mA"
//   - unit: the suffix to strip, such as "mA"
//
// Returns:
//   - the field as a number, or zero if the field is empty
//   - error if what is left after the suffix is not a base 10 number
func parseUnit(field, value, unit string) (int, error) {
	digits := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), unit))
	if digits == "" {
		return 0, nil
	}

	// Zero padding makes "0643" look octal to some parsers and makes "023"
	// ambiguous to a reader; base 10 is always what is meant.
	n, err := strconv.ParseInt(strings.TrimPrefix(digits, "+"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("response to %q has an invalid %s: %q", "GCS", field, value)
	}
	return int(n), nil
}
