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

// TestAvoid tests the Avoid method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the target and the mode go out in the wire form
//   - Error: a refusal is reported
func TestAvoid(t *testing.T) {
	// Verify that the target's three fields carry the mode after them.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).Avoid(context.Background(), Channel("12"), AvoidPermanent); err != nil {
			t.Fatalf("avoiding: %v", err)
		}
		if c.last() != "AVD,CHN,12,1" {
			t.Errorf("sent %q, want AVD,CHN,12,1", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		err := New(c).Avoid(context.Background(), System("1"), AvoidStop)
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestChannel tests the Channel function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the channel keyword carries the index and nothing else
func TestChannel(t *testing.T) {
	// Verify that only the first index is filled in.
	t.Run("Success", func(t *testing.T) {
		if got := Channel("12"); got != (Target{Kind: "CHN", First: "12"}) {
			t.Errorf("got %+v, want the channel keyword and index", got)
		}
	})
}

// TestDepartment tests the Department function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the department keyword carries the index and nothing else
func TestDepartment(t *testing.T) {
	// Verify that only the first index is filled in.
	t.Run("Success", func(t *testing.T) {
		if got := Department("12"); got != (Target{Kind: "DEPT", First: "12"}) {
			t.Errorf("got %+v, want the department keyword and index", got)
		}
	})
}

// TestHold tests the Hold method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the target goes out after the hold command
//   - Error: a refusal is reported, which is what a favorites list gets
func TestHold(t *testing.T) {
	// Verify that the target's three fields follow the command.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).Hold(context.Background(), Department("7")); err != nil {
			t.Fatalf("holding: %v", err)
		}
		if c.last() != "HLD,DEPT,7," {
			t.Errorf("sent %q, want HLD,DEPT,7,", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).Hold(context.Background(), System("1")); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestJumpToMode tests the JumpToMode method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - WithIndex: a mode that starts on something carries it
//   - NoIndex: a mode that takes nothing still carries the empty field
//   - Error: a refusal is reported
func TestJumpToMode(t *testing.T) {
	// Verify that an index goes out where the mode expects it.
	t.Run("WithIndex", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).JumpToMode(context.Background(), ScanModeScan, "12"); err != nil {
			t.Fatalf("jumping: %v", err)
		}
		if c.last() != "JPM,SCN_MODE,12" {
			t.Errorf("sent %q, want JPM,SCN_MODE,12", c.last())
		}
	})

	// Verify that a mode taking no index still sends the empty field.
	t.Run("NoIndex", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).JumpToMode(context.Background(), ScanModeCloseCall, ""); err != nil {
			t.Fatalf("jumping: %v", err)
		}
		if c.last() != "JPM,CC_MODE," {
			t.Errorf("sent %q, want JPM,CC_MODE,", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		err := New(c).JumpToMode(context.Background(), ScanModeScan, "")
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestJumpToNumberTag tests the JumpToNumberTag method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the three tags go out in order, one level each
//   - Error: a refusal is reported
func TestJumpToNumberTag(t *testing.T) {
	// Verify that the tags go out list, system, channel.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).JumpToNumberTag(context.Background(), 1, 2, 3); err != nil {
			t.Fatalf("jumping: %v", err)
		}
		if c.last() != "JNT,1,2,3" {
			t.Errorf("sent %q, want JNT,1,2,3", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		err := New(c).JumpToNumberTag(context.Background(), 1, 2, 3)
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestNext tests the Next method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the forward command carries the target and the count
//   - OutOfRange: a count the scanner cannot take is refused
func TestNext(t *testing.T) {
	// Verify that NXT is the command and the count follows the target.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).Next(context.Background(), Channel("12"), 3); err != nil {
			t.Fatalf("stepping: %v", err)
		}
		if c.last() != "NXT,CHN,12,,3" {
			t.Errorf("sent %q, want NXT,CHN,12,,3", c.last())
		}
	})

	// Verify that a count out of range is refused before anything is sent.
	t.Run("OutOfRange", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).Next(context.Background(), Channel("12"), 9); err == nil {
			t.Fatal("a count of 9 was accepted")
		}
		if len(c.commands) != 0 {
			t.Errorf("sent %q, want nothing sent", c.commands)
		}
	})
}

// TestPrevious tests the Previous method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the back command carries the target and the count
//   - OutOfRange: a count the scanner cannot take is refused
func TestPrevious(t *testing.T) {
	// Verify that PRV is the command.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).Previous(context.Background(), System("1"), 1); err != nil {
			t.Fatalf("stepping: %v", err)
		}
		if c.last() != "PRV,SYS,1,,1" {
			t.Errorf("sent %q, want PRV,SYS,1,,1", c.last())
		}
	})

	// Verify that a count out of range is refused.
	t.Run("OutOfRange", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).Previous(context.Background(), System("1"), 0); err == nil {
			t.Fatal("a count of 0 was accepted")
		}
		if len(c.commands) != 0 {
			t.Errorf("sent %q, want nothing sent", c.commands)
		}
	})
}

// TestPressKey tests the PressKey method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the key and the action go out in the wire form
//   - PaceCancelled: a context ending during the pace is reported
//   - Error: a refusal is reported and still counts as having bothered the
//     scanner
func TestPressKey(t *testing.T) {
	// Verify that the key and the action are what go out.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).PressKey(context.Background(), KeyRotateRight, KeyPress); err != nil {
			t.Fatalf("pressing: %v", err)
		}
		if c.last() != "KEY,>,P" {
			t.Errorf("sent %q, want KEY,>,P", c.last())
		}
	})

	// Verify that a context ending while waiting for the pace stops the press.
	t.Run("PaceCancelled", func(t *testing.T) {
		c := &stubConn{}
		s := New(c)
		if err := s.SetPace(PaceSlow); err != nil {
			t.Fatalf("setting the pace: %v", err)
		}
		s.lastKey = time.Now()

		if err := s.PressKey(deadCtx(t), KeyEnter, KeyPress); err == nil {
			t.Fatal("a cancelled pace reported nothing")
		}
		if len(c.commands) != 0 {
			t.Errorf("sent %q, want nothing sent", c.commands)
		}
	})

	// Verify that a refused key still counts as having occupied the scanner.
	t.Run("Error", func(t *testing.T) {
		s := New(&stubConn{exec: func(string) (string, error) { return "", ErrRejected }})
		if err := s.PressKey(context.Background(), KeyEnter, KeyLong); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
		if s.lastKey.IsZero() {
			t.Error("a refused key was not recorded as an attempt")
		}
	})
}

// TestQuickSearchHold tests the QuickSearchHold method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the frequency goes out in the eight digit wire form
//   - Error: a refusal is reported, which is what a menu gets
func TestQuickSearchHold(t *testing.T) {
	// Verify that the frequency is written in the encoding the scanner reads.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).QuickSearchHold(context.Background(), 155550000); err != nil {
			t.Fatalf("holding: %v", err)
		}
		if c.last() != "QSH,01555500" {
			t.Errorf("sent %q, want QSH,01555500", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		err := New(c).QuickSearchHold(context.Background(), 155550000)
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSystem tests the System function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the system keyword carries the index and nothing else
func TestSystem(t *testing.T) {
	// Verify that only the first index is filled in.
	t.Run("Success", func(t *testing.T) {
		if got := System("1"); got != (Target{Kind: "SYS", First: "1"}) {
			t.Errorf("got %+v, want the system keyword and index", got)
		}
	})
}

// Test_step tests the step method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the command, the target and the count go out together
//   - TooFew: a count below one is refused, naming the range
//   - TooMany: a count above eight is refused, naming the range
//   - Error: a refusal is reported
func Test_step(t *testing.T) {
	// Verify that all three parts go out in order.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).step(context.Background(), "NXT", Department("3"), 8); err != nil {
			t.Fatalf("stepping: %v", err)
		}
		if c.last() != "NXT,DEPT,3,,8" {
			t.Errorf("sent %q, want NXT,DEPT,3,,8", c.last())
		}
	})

	// Verify that zero steps is refused, naming what is accepted.
	t.Run("TooFew", func(t *testing.T) {
		err := New(&stubConn{}).step(context.Background(), "NXT", Channel("1"), 0)
		if err == nil || !strings.Contains(err.Error(), "want 1 to 8") {
			t.Fatalf("got %v, want the range named", err)
		}
	})

	// Verify that more than eight steps is refused as well.
	t.Run("TooMany", func(t *testing.T) {
		err := New(&stubConn{}).step(context.Background(), "PRV", Channel("1"), 9)
		if err == nil || !strings.Contains(err.Error(), "want 1 to 8") {
			t.Fatalf("got %v, want the range named", err)
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		err := New(c).step(context.Background(), "NXT", Channel("1"), 1)
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// Test_targetWire tests the Target wire method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - OneIndex: a keyword taking one index still sends the empty second field
//   - TwoIndexes: a keyword taking two sends both
func Test_targetWire(t *testing.T) {
	// Verify that the empty second field is still sent.
	t.Run("OneIndex", func(t *testing.T) {
		if got := Channel("12").wire(); got != "CHN,12," {
			t.Errorf("got %q, want CHN,12,", got)
		}
	})

	// Verify that a keyword needing two indexes sends both.
	t.Run("TwoIndexes", func(t *testing.T) {
		got := Target{Kind: "DEPT", First: "1", Second: "2"}.wire()
		if got != "DEPT,1,2" {
			t.Errorf("got %q, want DEPT,1,2", got)
		}
	})
}
