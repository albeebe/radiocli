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

// TestClock tests the Clock method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the digits are read into a local time and the flags alongside it
//   - Error: a failed exchange is reported
//   - ShortReply: a reply with too few fields is reported
//   - BadField: a field that is not a number is reported, naming it
func TestClock(t *testing.T) {
	// Verify that the wall clock digits are read in the host's own location.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "1,2026,08,12,14,30,45,1", nil }}
		got, err := New(c).Clock(context.Background())
		if err != nil {
			t.Fatalf("reading the clock: %v", err)
		}
		if c.last() != "DTM" {
			t.Errorf("sent %q, want DTM", c.last())
		}
		want := time.Date(2026, time.August, 12, 14, 30, 45, 0, time.Local)
		if !got.Time.Equal(want) {
			t.Errorf("got %v, want %v", got.Time, want)
		}
		if !got.DaylightSaving || !got.Valid {
			t.Errorf("got %+v, want both flags set", got)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().Clock(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a short reply is reported.
	t.Run("ShortReply", func(t *testing.T) {
		_, err := answeringStatus("1,2026,08").Clock(context.Background())
		if err == nil || !strings.Contains(err.Error(), "want at least 8") {
			t.Fatalf("got %v, want the counts reported", err)
		}
	})

	// Verify that a field that is not a number names itself.
	t.Run("BadField", func(t *testing.T) {
		_, err := answeringStatus("1,2026,August,12,14,30,45,1").Clock(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid month") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// TestDepartmentQuickKeys tests the DepartmentQuickKeys method with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the echoed keys are skipped and the states read
//   - Error: a failed exchange is reported
func TestDepartmentQuickKeys(t *testing.T) {
	// Verify that the two echoed key numbers are not taken for states.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "1,2,0,1,2", nil }}
		got, err := New(c).DepartmentQuickKeys(context.Background(), 1, 2)
		if err != nil {
			t.Fatalf("reading the quick keys: %v", err)
		}
		if c.last() != "DQK,1,2" {
			t.Errorf("sent %q, want DQK,1,2", c.last())
		}
		want := []QuickKeyState{QuickKeyAbsent, QuickKeyDisabled, QuickKeyEnabled}
		if len(got) != len(want) {
			t.Fatalf("got %d states, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("state %d is %v, want %v", i, got[i], want[i])
			}
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().DepartmentQuickKeys(context.Background(), 1, 2); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestFavoriteQuickKeys tests the FavoriteQuickKeys method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: every state is read, since this reply echoes no keys first
//   - Error: a failed exchange is reported
func TestFavoriteQuickKeys(t *testing.T) {
	// Verify that nothing is skipped, because nothing is echoed.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "2,0,1", nil }}
		got, err := New(c).FavoriteQuickKeys(context.Background())
		if err != nil {
			t.Fatalf("reading the quick keys: %v", err)
		}
		if c.last() != "FQK" {
			t.Errorf("sent %q, want FQK", c.last())
		}
		if len(got) != 3 || got[0] != QuickKeyEnabled {
			t.Errorf("got %v, want all three states", got)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().FavoriteQuickKeys(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestLocation tests the Location method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the position and the range are read
//   - Error: a failed exchange is reported
//   - ShortReply: a reply with too few fields is reported
//   - BadField: a field that is not a number is reported, naming it
func TestLocation(t *testing.T) {
	// Verify that all three numbers are read, padding and all.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return " 38.433056,-79.839000, 10.0", nil }}
		got, err := New(c).Location(context.Background())
		if err != nil {
			t.Fatalf("reading the location: %v", err)
		}
		if c.last() != "LCR" {
			t.Errorf("sent %q, want LCR", c.last())
		}
		want := Location{Latitude: 38.433056, Longitude: -79.839, Range: 10}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().Location(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that a short reply is reported.
	t.Run("ShortReply", func(t *testing.T) {
		_, err := answeringStatus("38.0,-79.0").Location(context.Background())
		if err == nil || !strings.Contains(err.Error(), "want at least 3") {
			t.Fatalf("got %v, want the counts reported", err)
		}
	})

	// Verify that a field that is not a number names itself.
	t.Run("BadField", func(t *testing.T) {
		_, err := answeringStatus("38.0,west,10.0").Location(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid longitude") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// TestRecording tests the Recording method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Recording: the scanner's one means it is recording
//   - NotRecording: anything else means it is not
//   - Error: a failed exchange is reported
func TestRecording(t *testing.T) {
	// Verify that one means recording.
	t.Run("Recording", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return " 1 ", nil }}
		got, err := New(c).Recording(context.Background())
		if err != nil {
			t.Fatalf("reading the recording state: %v", err)
		}
		if !got || c.last() != "URC" {
			t.Errorf("got %v from %q, want true from URC", got, c.last())
		}
	})

	// Verify that anything else means not recording.
	t.Run("NotRecording", func(t *testing.T) {
		got, err := answeringStatus("0").Recording(context.Background())
		if err != nil || got {
			t.Fatalf("got %v, %v, want false", got, err)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().Recording(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestServiceTypes tests the ServiceTypes method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: one entry comes back per type, in the order the reply gives them
//   - Error: a failed exchange is reported
func TestServiceTypes(t *testing.T) {
	// Verify that each field becomes one flag, in order.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "1,0, 1 ,0", nil }}
		got, err := New(c).ServiceTypes(context.Background())
		if err != nil {
			t.Fatalf("reading the service types: %v", err)
		}
		if c.last() != "SVC" {
			t.Errorf("sent %q, want SVC", c.last())
		}
		want := []bool{true, false, true, false}
		if len(got) != len(want) {
			t.Fatalf("got %d types, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("type %d is %v, want %v", i, got[i], want[i])
			}
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().ServiceTypes(context.Background()); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// TestSetClock tests the SetClock method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the digits go out zero padded with the flag clear
//   - DaylightSaving: the flag is written when the caller means to set it
//   - Error: a refusal is reported
func TestSetClock(t *testing.T) {
	when := time.Date(2026, time.August, 12, 9, 5, 3, 0, time.Local)

	// Verify that every field is zero padded to the width the scanner reads.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).SetClock(context.Background(), when, false); err != nil {
			t.Fatalf("setting the clock: %v", err)
		}
		if c.last() != "DTM,0,2026,08,12,09,05,03" {
			t.Errorf("sent %q, want the padded digits with the flag clear", c.last())
		}
	})

	// Verify that the flag is written when it is asked for.
	t.Run("DaylightSaving", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).SetClock(context.Background(), when, true); err != nil {
			t.Fatalf("setting the clock: %v", err)
		}
		if !strings.HasPrefix(c.last(), "DTM,1,") {
			t.Errorf("sent %q, want the flag set", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).SetClock(context.Background(), when, false); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSetDepartmentQuickKeys tests the SetDepartmentQuickKeys method with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the two keys precede the states
//   - Error: a refusal is reported
func TestSetDepartmentQuickKeys(t *testing.T) {
	// Verify that the keys are written before the states.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		states := []QuickKeyState{QuickKeyAbsent, QuickKeyEnabled}
		if err := New(c).SetDepartmentQuickKeys(context.Background(), 1, 2, states); err != nil {
			t.Fatalf("setting the quick keys: %v", err)
		}
		if c.last() != "DQK,1,2,0,2" {
			t.Errorf("sent %q, want DQK,1,2,0,2", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		err := New(c).SetDepartmentQuickKeys(context.Background(), 1, 2, nil)
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSetFavoriteQuickKeys tests the SetFavoriteQuickKeys method with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the states follow the command with no keys before them
//   - Error: a refusal is reported
func TestSetFavoriteQuickKeys(t *testing.T) {
	// Verify that nothing precedes the states.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		states := []QuickKeyState{QuickKeyEnabled, QuickKeyDisabled}
		if err := New(c).SetFavoriteQuickKeys(context.Background(), states); err != nil {
			t.Fatalf("setting the quick keys: %v", err)
		}
		if c.last() != "FQK,2,1" {
			t.Errorf("sent %q, want FQK,2,1", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).SetFavoriteQuickKeys(context.Background(), nil); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSetLocation tests the SetLocation method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: six decimal places are written, which is what reads back
//   - Error: a refusal is reported
func TestSetLocation(t *testing.T) {
	// Verify that the position keeps six places and the range one.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		loc := Location{Latitude: 38.433056, Longitude: -79.839, Range: 10}
		if err := New(c).SetLocation(context.Background(), loc); err != nil {
			t.Fatalf("setting the location: %v", err)
		}
		if c.last() != "LCR,38.433056,-79.839000,10.0" {
			t.Errorf("sent %q, want six decimal places on the position", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).SetLocation(context.Background(), Location{}); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSetRecording tests the SetRecording method with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Start: recording is started with a one
//   - Stop: recording is stopped with a zero
//   - Error: a failed exchange is reported
//   - Refused: a reason the scanner names comes back in words
//   - Unexpected: any other answer is reported
func TestSetRecording(t *testing.T) {
	// Verify that starting sends a one.
	t.Run("Start", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).SetRecording(context.Background(), true); err != nil {
			t.Fatalf("setting recording: %v", err)
		}
		if c.last() != "URC,1" {
			t.Errorf("sent %q, want URC,1", c.last())
		}
	})

	// Verify that stopping sends a zero.
	t.Run("Stop", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return okValue, nil }}
		if err := New(c).SetRecording(context.Background(), false); err != nil {
			t.Fatalf("setting recording: %v", err)
		}
		if c.last() != "URC,0" {
			t.Errorf("sent %q, want URC,0", c.last())
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if err := failingStatus().SetRecording(context.Background(), true); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})

	// Verify that the scanner's own reason comes back rather than a rejection.
	t.Run("Refused", func(t *testing.T) {
		err := answeringStatus("ERR, 0002 ").SetRecording(context.Background(), true)
		if err == nil || !strings.Contains(err.Error(), "the battery is too low") {
			t.Fatalf("got %v, want the reason in words", err)
		}
	})

	// Verify that anything else is reported rather than assumed to be success.
	t.Run("Unexpected", func(t *testing.T) {
		err := answeringStatus("MAYBE").SetRecording(context.Background(), true)
		if err == nil || !strings.Contains(err.Error(), "unexpected response") {
			t.Fatalf("got %v, want an unexpected response complaint", err)
		}
	})
}

// TestSetServiceTypes tests the SetServiceTypes method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: each flag becomes a one or a zero, in order
//   - Error: a refusal is reported
func TestSetServiceTypes(t *testing.T) {
	// Verify that the flags are written as ones and zeros in order.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		if err := New(c).SetServiceTypes(context.Background(), []bool{true, false, true}); err != nil {
			t.Fatalf("setting the service types: %v", err)
		}
		if c.last() != "SVC,1,0,1" {
			t.Errorf("sent %q, want SVC,1,0,1", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		err := New(c).SetServiceTypes(context.Background(), []bool{true})
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSetSystemQuickKeys tests the SetSystemQuickKeys method with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the favorites key precedes the states
//   - Error: a refusal is reported
func TestSetSystemQuickKeys(t *testing.T) {
	// Verify that the favorites key is written before the states.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{}
		states := []QuickKeyState{QuickKeyDisabled, QuickKeyEnabled}
		if err := New(c).SetSystemQuickKeys(context.Background(), 3, states); err != nil {
			t.Fatalf("setting the quick keys: %v", err)
		}
		if c.last() != "SQK,3,1,2" {
			t.Errorf("sent %q, want SQK,3,1,2", c.last())
		}
	})

	// Verify that a refusal is reported.
	t.Run("Error", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "", ErrRejected }}
		if err := New(c).SetSystemQuickKeys(context.Background(), 3, nil); !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// TestSystemQuickKeys tests the SystemQuickKeys method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the echoed keys are skipped and the states read
//   - Error: a failed exchange is reported
func TestSystemQuickKeys(t *testing.T) {
	// Verify that the echoed keys are not taken for states.
	t.Run("Success", func(t *testing.T) {
		c := &stubConn{exec: func(string) (string, error) { return "3,0,1,2", nil }}
		got, err := New(c).SystemQuickKeys(context.Background(), 3)
		if err != nil {
			t.Fatalf("reading the quick keys: %v", err)
		}
		if c.last() != "SQK,3" {
			t.Errorf("sent %q, want SQK,3", c.last())
		}
		if len(got) != 2 || got[0] != QuickKeyDisabled || got[1] != QuickKeyEnabled {
			t.Errorf("got %v, want the two states after the echo", got)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("Error", func(t *testing.T) {
		if _, err := failingStatus().SystemQuickKeys(context.Background(), 3); err == nil {
			t.Fatal("a failed exchange reported nothing")
		}
	})
}

// Test_formatQuickKeys tests the formatQuickKeys function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the states are written as comma separated numbers
//   - None: no states read as nothing at all
func Test_formatQuickKeys(t *testing.T) {
	// Verify that each state is written as its number.
	t.Run("Success", func(t *testing.T) {
		got := formatQuickKeys([]QuickKeyState{QuickKeyAbsent, QuickKeyDisabled, QuickKeyEnabled})
		if got != "0,1,2" {
			t.Errorf("got %q, want 0,1,2", got)
		}
	})

	// Verify that an empty list writes nothing.
	t.Run("None", func(t *testing.T) {
		if got := formatQuickKeys(nil); got != "" {
			t.Errorf("got %q, want nothing", got)
		}
	})
}

// Test_parseQuickKeys tests the parseQuickKeys function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the echoed fields are skipped and the rest read as states
//   - TooShort: a reply holding nothing but the echo is reported
//   - BadState: a state that is not a number is reported
func Test_parseQuickKeys(t *testing.T) {
	// Verify that the skipped fields are not read as states.
	t.Run("Success", func(t *testing.T) {
		got, err := parseQuickKeys("SQK", "3, 1 ,2", 1)
		if err != nil {
			t.Fatalf("parsing: %v", err)
		}
		if len(got) != 2 || got[0] != QuickKeyDisabled || got[1] != QuickKeyEnabled {
			t.Errorf("got %v, want the two states", got)
		}
	})

	// Verify that a reply with no states at all is reported.
	t.Run("TooShort", func(t *testing.T) {
		_, err := parseQuickKeys("DQK", "1,2", 2)
		if err == nil || !strings.Contains(err.Error(), "quick key states, want at least 3") {
			t.Fatalf("got %v, want the counts reported", err)
		}
	})

	// Verify that a state that is not a number is reported.
	t.Run("BadState", func(t *testing.T) {
		_, err := parseQuickKeys("FQK", "0,on,2", 0)
		if err == nil || !strings.Contains(err.Error(), "invalid quick key state") {
			t.Fatalf("got %v, want the field named", err)
		}
	})
}

// Test_recordingError tests the recordingError function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Documented: every code the specification gives has words
//   - Unknown: a code it does not give comes back as itself
func Test_recordingError(t *testing.T) {
	// Verify that each documented code reads in words.
	t.Run("Documented", func(t *testing.T) {
		for code, want := range map[string]string{
			"0001": "the memory card could not be written",
			"0002": "the battery is too low",
			"0003": "the session limit has been reached",
			"0004": "the clock has been lost, so recordings cannot be timestamped",
		} {
			if got := recordingError(code); got != want {
				t.Errorf("code %s reads %q, want %q", code, got, want)
			}
		}
	})

	// Verify that an undocumented code carries itself.
	t.Run("Unknown", func(t *testing.T) {
		if got := recordingError("0099"); got != "error code 0099" {
			t.Errorf("got %q, want the code carried", got)
		}
	})
}
