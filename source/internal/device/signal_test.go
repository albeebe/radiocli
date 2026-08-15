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

// TestSignalReceived tests the Signal Received method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Received: a level above the one meaning nothing counts as a signal
//   - Nothing: the level the scanner uses for nothing received does not
func TestSignalReceived(t *testing.T) {
	// Verify that an ordinary reading counts as received.
	t.Run("Received", func(t *testing.T) {
		if !(Signal{Level: -50}).Received() {
			t.Error("a reading of -50 does not report itself received")
		}
	})

	// Verify that the scanner's no signal level does not.
	t.Run("Nothing", func(t *testing.T) {
		if (Signal{Level: noSignal}).Received() {
			t.Error("the no signal level reports itself received")
		}
	})
}

// TestSignalStrength tests the SignalStrength method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: PWR is sent and its level and frequency read
//   - Unsupported: a firmware that has dropped the command is reported
func TestSignalStrength(t *testing.T) {
	// Verify that PWR is the command and both fields come back.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "-050,01555500", nil }}
		got, err := New(c).SignalStrength(context.Background())
		if err != nil {
			t.Fatalf("reading the signal: %v", err)
		}
		if c.last() != "PWR" {
			t.Errorf("sent %q, want PWR", c.last())
		}
		if got != (Signal{Level: -50, Frequency: Frequency(155550000)}) {
			t.Errorf("got %+v, want -50 on 155.55 MHz", got)
		}
	})

	// Verify that a firmware without the command is reported as such.
	t.Run("Unsupported", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrUnsupported }}
		if _, err := New(c).SignalStrength(context.Background()); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("got %v, want ErrUnsupported", err)
		}
	})
}

// TestWindowVoltage tests the WindowVoltage method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: WIN is sent and its reading returned
func TestWindowVoltage(t *testing.T) {
	// Verify that WIN is the command this one sends.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "0128,01555500", nil }}
		got, err := New(c).WindowVoltage(context.Background())
		if err != nil {
			t.Fatalf("reading the window voltage: %v", err)
		}
		if c.last() != "WIN" || got.Level != 128 {
			t.Errorf("got %+v from %q, want 128 from WIN", got, c.last())
		}
	})
}

// Test_signal tests the signal method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the level and the frequency are both read
//   - Error: a failed exchange is reported
//   - ShortReply: a reply with too few fields is reported
//   - BadLevel: a level that is not a number is reported
//   - BadFrequency: a frequency that is not digits is reported
func Test_signal(t *testing.T) {
	answering := func(value string) *Scanner {
		return New(&stubConn{exec: func(string) (string, error) { return value, nil }})
	}

	// Verify that both fields land where they belong.
	t.Run("Success", func(t *testing.T) {
		got, err := answering("-050,01555500").signal(context.Background(), "PWR")
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if got.Level != -50 || got.Frequency != Frequency(155550000) {
			t.Errorf("got %+v, want -50 on 155.55 MHz", got)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		s := New(&stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }})
		if _, err := s.signal(context.Background(), "PWR"); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a reply carrying only the level is reported.
	t.Run("ShortReply", func(t *testing.T) {
		_, err := answering("-050").signal(context.Background(), "PWR")
		if err == nil || !strings.Contains(err.Error(), "want at least 2") {
			t.Fatalf("got %v, want a short reply complaint", err)
		}
	})

	// Verify that a level that is not a number is reported.
	t.Run("BadLevel", func(t *testing.T) {
		_, err := answering("loud,01555500").signal(context.Background(), "PWR")
		if err == nil || !strings.Contains(err.Error(), "invalid signal level") {
			t.Fatalf("got %v, want the level named", err)
		}
	})

	// Verify that a frequency that is not digits is reported.
	t.Run("BadFrequency", func(t *testing.T) {
		_, err := answering("-050,ABCDEFGH").signal(context.Background(), "PWR")
		if err == nil || !strings.Contains(err.Error(), "invalid frequency") {
			t.Fatalf("got %v, want the frequency named", err)
		}
	})
}
