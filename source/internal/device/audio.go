// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package device

import (
	"context"
	"fmt"
)

// SetSquelch sets the squelch level.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - level: squelch level, from MinLevel to MaxLevel
//
// Returns:
//   - error if the level is out of range or the scanner refuses the command
func (s *Scanner) SetSquelch(ctx context.Context, level int) error {
	if err := checkLevel("squelch", level); err != nil {
		return err
	}
	return s.set(ctx, fmt.Sprintf("SQL,%d", level))
}

// SetVolume sets the volume level. It reports an out-of-range level as an
// error without sending anything, because the scanner answers a bad level the
// same way it answers a command it cannot run right now.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - level: volume level, from MinLevel to MaxLevel
//
// Returns:
//   - error if the level is out of range or the scanner refuses the command
func (s *Scanner) SetVolume(ctx context.Context, level int) error {
	if err := checkLevel("volume", level); err != nil {
		return err
	}
	return s.set(ctx, fmt.Sprintf("VOL,%d", level))
}

// Squelch returns the current squelch level, from MinLevel to MaxLevel.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - the squelch level the scanner reports
//   - error if the exchange fails or the reply is malformed
func (s *Scanner) Squelch(ctx context.Context) (int, error) {
	value, err := s.conn.Execute(ctx, "SQL")
	if err != nil {
		return 0, err
	}
	return atoi("SQL", "squelch level", value)
}

// Volume returns the current volume level, from MinLevel to MaxLevel.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - the volume level the scanner reports
//   - error if the exchange fails or the reply is malformed
func (s *Scanner) Volume(ctx context.Context) (int, error) {
	value, err := s.conn.Execute(ctx, "VOL")
	if err != nil {
		return 0, err
	}
	return atoi("VOL", "volume level", value)
}

// checkLevel rejects a level the scanner cannot accept.
//
// Parameters:
//   - name: which level is being checked, for the error message
//   - level: the level to check
//
// Returns:
//   - error if level is outside MinLevel to MaxLevel
func checkLevel(name string, level int) error {
	if level < MinLevel || level > MaxLevel {
		return fmt.Errorf("%s level %d is out of range: want %d to %d", name, level, MinLevel, MaxLevel)
	}
	return nil
}
