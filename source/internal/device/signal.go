// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
)

// Received reports whether there was a signal to measure.
//
// Returns:
//   - true if the scanner reported a level above the one it uses for nothing
//     received, false otherwise
func (s Signal) Received() bool {
	return s.Level > noSignal
}

// SignalStrength returns the current signal strength and frequency.
//
// PWR and WindowVoltage are not in the SDS series specification: they are
// inherited from Uniden's earlier scanners and still answered by this
// firmware. They are the only way the protocol reports signal strength as a
// number, so they are worth having, but treat them as undocumented: a future
// firmware could drop them, in which case this returns ErrUnsupported.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Signal holding the strength reading and the frequency it was taken on
//   - error if the exchange fails, the reply is malformed, or the firmware
//     does not know the command, which wraps ErrUnsupported
func (s *Scanner) SignalStrength(ctx context.Context) (Signal, error) {
	return s.signal(ctx, "PWR")
}

// WindowVoltage returns the scanner's window voltage reading and the frequency
// it was taken on.
//
// Like SignalStrength this is undocumented for this model. The reading tracks
// how far the received signal sits from the centre of the passband.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Signal holding the window voltage reading and the frequency it was taken
//     on
//   - error if the exchange fails, the reply is malformed, or the firmware
//     does not know the command, which wraps ErrUnsupported
func (s *Scanner) WindowVoltage(ctx context.Context) (Signal, error) {
	return s.signal(ctx, "WIN")
}

// signal reads one of the two undocumented strength commands, which share a
// reply shape of a level followed by a frequency.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - command: which command to send, either PWR or WIN
//
// Returns:
//   - Signal holding the level and the frequency the reading was taken on
//   - error if the exchange fails, the reply has too few fields, or either
//     field is malformed
func (s *Scanner) signal(ctx context.Context, command string) (Signal, error) {
	value, err := s.conn.Execute(ctx, command)
	if err != nil {
		return Signal{}, err
	}

	f, err := fields(command, value, 2)
	if err != nil {
		return Signal{}, err
	}

	level, err := atoi(command, "signal level", f[0])
	if err != nil {
		return Signal{}, err
	}
	frequency, err := parseFrequency(command, "frequency", f[1])
	if err != nil {
		return Signal{}, err
	}

	return Signal{Level: level, Frequency: frequency}, nil
}
