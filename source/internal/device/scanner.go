// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// New wraps a transport in the typed command set. Tests use it to drive every
// command against a fake Conn with no scanner attached.
//
// Parameters:
//   - conn: the transport to send commands over
//
// Returns:
//   - a Scanner sending over conn, paced at DefaultPace
func New(conn Conn) *Scanner {
	return &Scanner{conn: conn, pace: DefaultPace}
}

// Close releases the serial port.
//
// Returns:
//   - error if the transport fails to close
func (s *Scanner) Close() error {
	return s.conn.Close()
}

// Conn exposes the underlying transport, for sending a command this package
// does not model yet. Prefer a typed method: anything reached through here is
// undocumented by definition.
//
// Returns:
//   - the transport this Scanner sends over
func (s *Scanner) Conn() Conn {
	return s.conn
}

// FirmwareVersion returns the scanner's firmware version, such as
// "Version 1.00.37". The scanner supplies the whole string, including the
// leading word.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - the firmware version as the scanner words it
//   - error if the exchange fails
func (s *Scanner) FirmwareVersion(ctx context.Context) (string, error) {
	return s.conn.Execute(ctx, "VER")
}

// Info describes the connected scanner.
//
// Returns:
//   - Info holding the port, the model, and the USB serial number
func (s *Scanner) Info() Info {
	return s.conn.Info()
}

// KeepAlive tells the scanner the host is still there.
//
// The scanner sends no reply, so this returns as soon as the command is
// written and cannot report whether the scanner received it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - error if the command cannot be written
func (s *Scanner) KeepAlive(ctx context.Context) error {
	return s.conn.Send(ctx, "KAL")
}

// Model returns the model name the scanner reports, such as SDS150.
//
// The specification says an SDS150 answers SDS150GBT. The unit this package
// was developed against answers SDS150, so treat the value as informational
// rather than as something to match on exactly.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - the model name the scanner reports
//   - error if the exchange fails
func (s *Scanner) Model(ctx context.Context) (string, error) {
	return s.conn.Execute(ctx, "MDL")
}

// Pace returns how quickly keys are being sent.
//
// Returns:
//   - the pace key presses are being sent at
func (s *Scanner) Pace() Pace {
	s.keys.Lock()
	defer s.keys.Unlock()
	return s.pace
}

// PowerOff turns the scanner off.
//
// The scanner does not come back on its own: someone has to press the power
// button. Nothing else in this package is destructive, so this is the one
// method a caller should confirm with the user before invoking.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) PowerOff(ctx context.Context) error {
	_, err := s.conn.Execute(ctx, "POF")
	return err
}

// SetPace changes how quickly keys are sent. An unknown pace is rejected
// rather than silently treated as the default, because a typo in a setting
// should be reported, not obeyed approximately.
//
// Parameters:
//   - p: the pace to send keys at, one of the paces in Paces
//
// Returns:
//   - error if the pace is not one this package knows
func (s *Scanner) SetPace(p Pace) error {
	if !p.Valid() {
		return fmt.Errorf("invalid pace %q: want %s", string(p), PaceNames())
	}

	s.keys.Lock()
	defer s.keys.Unlock()
	s.pace = p
	return nil
}

// atoi converts one field of a response, naming the field in any error so a
// malformed reply says which part was wrong.
//
// Parameters:
//   - command: the command that was sent, for the error message
//   - field: what the field holds, for the error message
//   - value: the field as the scanner sent it, which may be padded
//
// Returns:
//   - the field as a number
//   - error if the field is not a number
func atoi(command, field, value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("response to %q has an invalid %s: %q", command, field, value)
	}
	return n, nil
}

// awaitPace waits until enough time has passed since the last key press.
//
// It measures from when the last key was sent rather than sleeping a fixed
// amount afterwards, so time a caller already spent reading the screen counts
// towards the gap instead of being added to it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - error if the context is cancelled before the gap has passed, nil once
//     enough time has gone by
func (s *Scanner) awaitPace(ctx context.Context) error {
	wait := time.Until(s.lastKey.Add(s.pace.Delay()))
	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("waiting to send the next key: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// set sends a command that changes a setting and confirms the scanner
// accepted it. The reply carries no information beyond success.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - command: the command to send, arguments included
//
// Returns:
//   - error if the exchange fails, the scanner refuses the command, or the
//     reply is neither of the two shapes that mean success
func (s *Scanner) set(ctx context.Context, command string) error {
	value, err := s.conn.Execute(ctx, command)
	if err != nil {
		return err
	}

	// The specification documents a bare echo as the success reply for some
	// commands and CMD,OK for others, and this firmware disagrees with it on
	// at least VOL. Accept both rather than picking a side.
	if value != "" && value != okValue {
		return fmt.Errorf("unexpected response to %q: %q", command, value)
	}
	return nil
}
