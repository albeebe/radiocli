// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Key is a key on the scanner's front panel.
//
// The codes come from the specification's key table, and what each one does
// was confirmed against an SDS150: the scanner accepts every code and answers
// KEY,OK regardless, so a code it has no key for does nothing rather than
// reporting an error. What a key does depends on what is on screen.
type Key string

// KeyAction is how a key is pressed.
type KeyAction string

// ScanMode is a mode the scanner can be told to jump to.
type ScanMode string

// AvoidMode is how thoroughly an entry is avoided.
type AvoidMode int

// Avoid stops the scanner scanning an entry, or starts it again.
//
// The protocol offers no way to read back what is avoided. Use ScannerInfo or
// List to see the current state.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - t: the entry to avoid, from Channel, Department, or System
//   - mode: how thoroughly to avoid it, or AvoidStop to scan it again
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) Avoid(ctx context.Context, t Target, mode AvoidMode) error {
	return s.set(ctx, fmt.Sprintf("AVD,%s%d", t.wire(), mode))
}

// Channel returns a Target for one channel.
//
// Parameters:
//   - index: the channel's index, as the scanner reports it
//
// Returns:
//   - Target naming that channel, for the commands that take one
func Channel(index string) Target { return Target{Kind: "CHN", First: index} }

// Department returns a Target for one department.
//
// Parameters:
//   - index: the department's index, as the scanner reports it
//
// Returns:
//   - Target naming that department, for the commands that take one
func Department(index string) Target { return Target{Kind: "DEPT", First: index} }

// Hold holds the scanner on one system, department, or channel.
//
// A favorites list and a site frequency cannot be held; the scanner refuses
// those.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - t: the entry to hold on, from Channel, Department, or System
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) Hold(ctx context.Context, t Target) error {
	return s.set(ctx, "HLD,"+t.wire())
}

// JumpToMode switches the scanner to another mode.
//
// The index means something different in each mode: a channel index for
// ScanModeScan, a folder or session name for the recording and discovery
// modes, and nothing at all for the rest, where it should be empty. Pass an
// empty string when the mode takes no index.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - mode: the mode to switch to, such as ScanModeScan or ScanModeWeather
//   - index: what the mode needs to start on, empty for the modes that take
//     nothing
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) JumpToMode(ctx context.Context, mode ScanMode, index string) error {
	return s.set(ctx, fmt.Sprintf("JPM,%s,%s", mode, index))
}

// JumpToNumberTag jumps to a channel by its number tags. Each tag identifies
// one level: the favorites list, the system within it, and the channel within
// that.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - favoritesList: the favorites list's number tag
//   - system: the system's number tag within that list
//   - channel: the channel's number tag within that system
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) JumpToNumberTag(ctx context.Context, favoritesList, system, channel int) error {
	return s.set(ctx, fmt.Sprintf("JNT,%d,%d,%d", favoritesList, system, channel))
}

// Next moves forward through entries of one kind, by count steps.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - t: which kind of entry to step through, from Channel, Department, or
//     System
//   - count: how many steps to take, from 1 to 8
//
// Returns:
//   - error if count is out of range, the exchange fails, or the scanner
//     refuses the command
func (s *Scanner) Next(ctx context.Context, t Target, count int) error {
	return s.step(ctx, "NXT", t, count)
}

// PressKey presses a key on the scanner, as though someone had pressed it.
//
// This drives the scanner's own interface rather than the remote protocol, so
// it can reach anything a person can reach, including screens no command
// covers. It is also the least predictable way to control the scanner, since
// what a key does depends on what is on screen. Prefer a typed command when
// one exists.
//
// Presses are paced: this blocks until the configured Pace has elapsed since
// the previous key, so a run of presses arrives at a rate the scanner can
// follow. See SetPace.
//
// Parameters:
//   - ctx: context for cancellation and timeouts, including the wait for the
//     pace to elapse
//   - key: which key to press
//   - action: how to press it, such as KeyPress or KeyLong
//
// Returns:
//   - error if the context ends while waiting for the pace, the exchange
//     fails, or the scanner refuses the command
func (s *Scanner) PressKey(ctx context.Context, key Key, action KeyAction) error {
	s.keys.Lock()
	defer s.keys.Unlock()

	if err := s.awaitPace(ctx); err != nil {
		return err
	}

	// Record the attempt whether or not it succeeded. A key the scanner
	// refused still occupied it, and pacing from the attempt is the safer
	// reading of "how long since we last bothered it".
	defer func() { s.lastKey = time.Now() }()

	return s.set(ctx, fmt.Sprintf("KEY,%s,%s", key, action))
}

// Previous moves back through entries of one kind, by count steps.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - t: which kind of entry to step through, from Channel, Department, or
//     System
//   - count: how many steps to take, from 1 to 8
//
// Returns:
//   - error if count is out of range, the exchange fails, or the scanner
//     refuses the command
func (s *Scanner) Previous(ctx context.Context, t Target, count int) error {
	return s.step(ctx, "PRV", t, count)
}

// QuickSearchHold puts the scanner into quick search hold on one frequency.
//
// The scanner refuses this while it is in a menu, during direct entry, and
// during a quick save.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - f: the frequency to hold on
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) QuickSearchHold(ctx context.Context, f Frequency) error {
	return s.set(ctx, "QSH,"+f.wire())
}

// System returns a Target for one system.
//
// Parameters:
//   - index: the system's index, as the scanner reports it
//
// Returns:
//   - Target naming that system, for the commands that take one
func System(index string) Target { return Target{Kind: "SYS", First: index} }

// step sends one of the two stepping commands, which share a shape.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - command: the command name, NXT or PRV
//   - t: which kind of entry to step through
//   - count: how many steps to take, from 1 to 8
//
// Returns:
//   - error if count is out of range, the exchange fails, or the scanner
//     refuses the command
func (s *Scanner) step(ctx context.Context, command string, t Target, count int) error {
	if count < 1 || count > 8 {
		return fmt.Errorf("step count %d is out of range: want 1 to 8", count)
	}
	return s.set(ctx, fmt.Sprintf("%s,%s,%d", command, t.wire(), count))
}

// wire renders the target as the three comma separated fields the protocol
// expects, including the empty one when the keyword takes a single index.
//
// Returns:
//   - the target as "kind,first,second", ready to embed in a command
func (t Target) wire() string {
	return strings.Join([]string{t.Kind, t.First, t.Second}, ",")
}
