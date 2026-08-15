// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the scanner sends over the transport it was given, at the
//     default pace
func TestNew(t *testing.T) {
	// Verify that the transport and the default pace are taken up.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		s := New(c)
		if s.Conn() != c {
			t.Error("the scanner is not sending over the transport it was given")
		}
		if s.Pace() != DefaultPace {
			t.Errorf("got pace %q, want %q", s.Pace(), DefaultPace)
		}
	})
}

// TestScannerClose tests the Scanner Close method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: closing the scanner closes the transport
//   - CloseError: a transport that will not close is reported
func TestScannerClose(t *testing.T) {
	// Verify that the transport is closed.
	t.Run("Success", func(t *testing.T) {
		closed := false
		s := New(&stubConn{closeFn: func() error { closed = true; return nil }})
		if err := s.Close(); err != nil {
			t.Fatalf("closing: %v", err)
		}
		if !closed {
			t.Error("the transport was left open")
		}
	})

	// Verify that a transport that will not close is reported.
	t.Run("CloseError", func(t *testing.T) {
		s := New(&stubConn{closeFn: func() error { return errors.New("the port is gone") }})
		if err := s.Close(); err == nil {
			t.Fatal("a broken transport reported nothing")
		}
	})
}

// TestScannerConn tests the Scanner Conn method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the transport the scanner sends over is handed back
func TestScannerConn(t *testing.T) {
	// Verify that the transport comes back unchanged.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if New(c).Conn() != c {
			t.Error("got a different transport than the one given")
		}
	})
}

// TestFirmwareVersion tests the FirmwareVersion method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the version the scanner words is returned whole
//   - Error: a failed exchange is reported
func TestFirmwareVersion(t *testing.T) {
	// Verify that the whole string comes back, leading word included.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "Version 1.00.37", nil }}
		got, err := New(c).FirmwareVersion(context.Background())
		if err != nil {
			t.Fatalf("reading the version: %v", err)
		}
		if got != "Version 1.00.37" || c.last() != "VER" {
			t.Errorf("got %q from %q, want the version from VER", got, c.last())
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).FirmwareVersion(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestScannerInfoMethod tests the Scanner Info method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the transport's own description is passed through
func TestScannerInfoMethod(t *testing.T) {
	// Verify that the transport's description is what comes back.
	t.Run("Success", func(t *testing.T) {
		want := Info{Port: "/dev/one", Model: "SDS150"}
		if got := New(&stubConn{info: want}).Info(); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
}

// TestKeepAlive tests the KeepAlive method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: KAL is written and nothing is waited for
//   - Error: a command that cannot be written is reported
func TestKeepAlive(t *testing.T) {
	// Verify that KAL goes out through the send path.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).KeepAlive(context.Background()); err != nil {
			t.Fatalf("sending: %v", err)
		}
		if c.last() != "KAL" {
			t.Errorf("sent %q, want KAL", c.last())
		}
	})

	// Verify that a write that fails is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{send: func(string) error { return errors.New("the port is gone") }}
		if err := New(c).KeepAlive(context.Background()); err == nil {
			t.Fatal("a failed write reported nothing")
		}
	})
}

// TestModel tests the Model method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the model the scanner reports is returned
//   - Error: a failed exchange is reported
func TestModel(t *testing.T) {
	// Verify that MDL is sent and its answer returned.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "SDS150", nil }}
		got, err := New(c).Model(context.Background())
		if err != nil {
			t.Fatalf("reading the model: %v", err)
		}
		if got != "SDS150" || c.last() != "MDL" {
			t.Errorf("got %q from %q, want SDS150 from MDL", got, c.last())
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }}
		if _, err := New(c).Model(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestScannerPace tests the Scanner Pace method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the pace set is the pace reported
func TestScannerPace(t *testing.T) {
	// Verify that the pace last set comes back.
	t.Run("Success", func(t *testing.T) {
		s := New(&stubConn{})
		if err := s.SetPace(PaceSlow); err != nil {
			t.Fatalf("setting the pace: %v", err)
		}
		if s.Pace() != PaceSlow {
			t.Errorf("got %q, want slow", s.Pace())
		}
	})
}

// TestPowerOff tests the PowerOff method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: POF is sent
//   - Error: a refusal is reported
func TestPowerOff(t *testing.T) {
	// Verify that POF is what goes out.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).PowerOff(context.Background()); err != nil {
			t.Fatalf("powering off: %v", err)
		}
		if c.last() != "POF" {
			t.Errorf("sent %q, want POF", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).PowerOff(context.Background()); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSetPace tests the SetPace method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: a known pace is taken up
//   - Invalid: an unknown pace is refused rather than treated as the default
func TestSetPace(t *testing.T) {
	// Verify that a known pace is taken up.
	t.Run("Success", func(t *testing.T) {
		s := New(&stubConn{})
		if err := s.SetPace(PaceMedium); err != nil {
			t.Fatalf("setting the pace: %v", err)
		}
		if s.Pace() != PaceMedium {
			t.Errorf("got %q, want medium", s.Pace())
		}
	})

	// Verify that an unknown pace is refused and the old one kept.
	t.Run("Invalid", func(t *testing.T) {
		s := New(&stubConn{})
		err := s.SetPace("glacial")
		if err == nil || !strings.Contains(err.Error(), "invalid pace") {
			t.Fatalf("got %v, want an invalid pace complaint", err)
		}
		if s.Pace() != DefaultPace {
			t.Errorf("got %q, want the default kept", s.Pace())
		}
	})
}

// Test_atoi tests the atoi function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a plain number is read
//   - Padded: the scanner's space padding is ignored
//   - Invalid: a field that is not a number names itself in the error
func Test_atoi(t *testing.T) {
	// Verify that a plain number is read.
	t.Run("Success", func(t *testing.T) {
		got, err := atoi("VOL", "volume level", "7")
		if err != nil || got != 7 {
			t.Fatalf("got %d, %v, want 7", got, err)
		}
	})

	// Verify that padding is not part of the number.
	t.Run("Padded", func(t *testing.T) {
		got, err := atoi("VOL", "volume level", "  12  ")
		if err != nil || got != 12 {
			t.Fatalf("got %d, %v, want 12", got, err)
		}
	})

	// Verify that a field that is not a number names itself.
	t.Run("Invalid", func(t *testing.T) {
		_, err := atoi("VOL", "volume level", "loud")
		if err == nil || !strings.Contains(err.Error(), "invalid volume level") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// Test_awaitPace tests the awaitPace method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NoWait: turbo leaves no gap, so it returns at once
//   - Waits: a gap still to run is waited out
//   - Cancelled: a context that ends during the gap is reported
func Test_awaitPace(t *testing.T) {
	// Verify that turbo never waits.
	t.Run("NoWait", func(t *testing.T) {
		s := New(&stubConn{})
		s.lastKey = time.Now()
		if err := s.awaitPace(context.Background()); err != nil {
			t.Fatalf("waiting: %v", err)
		}
	})

	// Verify that a gap still to run is waited out rather than skipped.
	t.Run("Waits", func(t *testing.T) {
		s := New(&stubConn{})
		if err := s.SetPace(PaceFast); err != nil {
			t.Fatalf("setting the pace: %v", err)
		}

		// Nearly the whole gap has already passed, so this is a wait of about a
		// millisecond rather than the pace's full tenth of a second.
		s.lastKey = time.Now().Add(-PaceFast.Delay() + time.Millisecond)
		if err := s.awaitPace(context.Background()); err != nil {
			t.Fatalf("waiting: %v", err)
		}
	})

	// Verify that a context ending during the gap is reported.
	t.Run("Cancelled", func(t *testing.T) {
		s := New(&stubConn{})
		if err := s.SetPace(PaceSlow); err != nil {
			t.Fatalf("setting the pace: %v", err)
		}
		s.lastKey = time.Now()

		err := s.awaitPace(deadCtx(t))
		if err == nil || !strings.Contains(err.Error(), "waiting to send the next key") {
			t.Fatalf("got %v, want the wait named", err)
		}
	})
}

// Test_set tests the set method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - EchoOnly: a bare echo counts as success
//   - OK: the CMD,OK form counts as success too
//   - Error: a failed exchange is reported
//   - Unexpected: any other answer is reported as unexpected
func Test_set(t *testing.T) {
	// Verify that a bare echo is success.
	t.Run("EchoOnly", func(t *testing.T) {
		s := New(&stubConn{exec: func(string) (string, error) { return "", nil }})
		if err := s.set(context.Background(), "VOL,7"); err != nil {
			t.Fatalf("setting: %v", err)
		}
	})

	// Verify that the OK form is success as well.
	t.Run("OK", func(t *testing.T) {
		s := New(&stubConn{exec: func(string) (string, error) { return okValue, nil }})
		if err := s.set(context.Background(), "VOL,7"); err != nil {
			t.Fatalf("setting: %v", err)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		s := New(&stubConn{exec: func(string) (string, error) { return "", errors.New("the port is gone") }})
		if err := s.set(context.Background(), "VOL,7"); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that anything else is reported rather than assumed to be success.
	t.Run("Unexpected", func(t *testing.T) {
		s := New(&stubConn{exec: func(string) (string, error) { return "MAYBE", nil }})
		err := s.set(context.Background(), "VOL,7")
		if err == nil || !strings.Contains(err.Error(), "unexpected response") {
			t.Fatalf("got %v, want an unexpected response complaint", err)
		}
	})
}
