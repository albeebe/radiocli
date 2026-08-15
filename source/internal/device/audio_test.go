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

// TestSetSquelch tests the SetSquelch method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the level is sent as SQL with the level after it
//   - OutOfRange: a level the scanner cannot take is refused without sending
//   - Error: a refusal is reported
func TestSetSquelch(t *testing.T) {
	// Verify that the level goes out in the wire form.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).SetSquelch(context.Background(), 7); err != nil {
			t.Fatalf("setting the squelch: %v", err)
		}
		if c.last() != "SQL,7" {
			t.Errorf("sent %q, want SQL,7", c.last())
		}
	})

	// Verify that an impossible level is refused before anything is sent.
	t.Run("OutOfRange", func(t *testing.T) {
		c := &stubConn{}
		err := New(c).SetSquelch(context.Background(), MaxLevel+1)
		if err == nil || !strings.Contains(err.Error(), "squelch level 16 is out of range") {
			t.Fatalf("got %v, want the range reported", err)
		}
		if len(c.commands) != 0 {
			t.Errorf("sent %q, want nothing sent", c.commands)
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).SetSquelch(context.Background(), 7); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSetVolume tests the SetVolume method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the level is sent as VOL with the level after it
//   - OutOfRange: a level below the minimum is refused without sending
//   - Error: a refusal is reported
func TestSetVolume(t *testing.T) {
	// Verify that the level goes out in the wire form.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).SetVolume(context.Background(), 12); err != nil {
			t.Fatalf("setting the volume: %v", err)
		}
		if c.last() != "VOL,12" {
			t.Errorf("sent %q, want VOL,12", c.last())
		}
	})

	// Verify that a negative level is refused before anything is sent.
	t.Run("OutOfRange", func(t *testing.T) {
		c := &stubConn{}
		err := New(c).SetVolume(context.Background(), MinLevel-1)
		if err == nil || !strings.Contains(err.Error(), "volume level -1 is out of range") {
			t.Fatalf("got %v, want the range reported", err)
		}
		if len(c.commands) != 0 {
			t.Errorf("sent %q, want nothing sent", c.commands)
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).SetVolume(context.Background(), 12); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSquelch tests the Squelch method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the level the scanner reports is returned
//   - Error: a failed exchange is reported
//   - Malformed: a reply that is not a number is reported
func TestSquelch(t *testing.T) {
	// Verify that SQL is sent and its level read.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "7", nil }}
		got, err := New(c).Squelch(context.Background())
		if err != nil {
			t.Fatalf("reading the squelch: %v", err)
		}
		if got != 7 || c.last() != "SQL" {
			t.Errorf("got %d from %q, want 7 from SQL", got, c.last())
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).Squelch(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a reply that is not a number is reported.
	t.Run("Malformed", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "loud", nil }}
		_, err := New(c).Squelch(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid squelch level") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// TestVolume tests the Volume method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the level the scanner reports is returned
//   - Error: a failed exchange is reported
//   - Malformed: a reply that is not a number is reported
func TestVolume(t *testing.T) {
	// Verify that VOL is sent and its level read.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return " 12 ", nil }}
		got, err := New(c).Volume(context.Background())
		if err != nil {
			t.Fatalf("reading the volume: %v", err)
		}
		if got != 12 || c.last() != "VOL" {
			t.Errorf("got %d from %q, want 12 from VOL", got, c.last())
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).Volume(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a reply that is not a number is reported.
	t.Run("Malformed", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "loud", nil }}
		_, err := New(c).Volume(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid volume level") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// Test_checkLevel tests the checkLevel function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - InRange: every level the scanner accepts passes
//   - TooLow: a level below the minimum is refused
//   - TooHigh: a level above the maximum is refused
func Test_checkLevel(t *testing.T) {
	// Verify that the whole accepted range passes.
	t.Run("InRange", func(t *testing.T) {
		for level := MinLevel; level <= MaxLevel; level++ {
			if err := checkLevel("volume", level); err != nil {
				t.Errorf("level %d was refused: %v", level, err)
			}
		}
	})

	// Verify that a level below the minimum is refused.
	t.Run("TooLow", func(t *testing.T) {
		if err := checkLevel("volume", MinLevel-1); err == nil {
			t.Fatal("a negative level was accepted")
		}
	})

	// Verify that a level above the maximum is refused.
	t.Run("TooHigh", func(t *testing.T) {
		err := checkLevel("squelch", MaxLevel+1)
		if err == nil || !strings.Contains(err.Error(), "want 0 to 15") {
			t.Fatalf("got %v, want the bounds reported", err)
		}
	})
}
