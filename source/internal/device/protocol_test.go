// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"errors"
	"strings"
	"testing"
)

// Test_fields tests the fields function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the fields come back split
//   - Extra: more fields than asked for are kept rather than refused
//   - TooFew: a reply with fewer fields than the caller needs is reported
func Test_fields(t *testing.T) {
	// Verify that the reply is split on commas.
	t.Run("Success", func(t *testing.T) {
		got, err := fields("PWR", "-050,01555500", 2)
		if err != nil {
			t.Fatalf("splitting: %v", err)
		}
		if len(got) != 2 || got[0] != "-050" || got[1] != "01555500" {
			t.Errorf("got %q, want the two fields", got)
		}
	})

	// Verify that a longer reply than the specification documents is accepted.
	t.Run("Extra", func(t *testing.T) {
		got, err := fields("PWR", "-050,01555500,extra", 2)
		if err != nil {
			t.Fatalf("splitting: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("got %d fields, want the trailing one kept", len(got))
		}
	})

	// Verify that a short reply is reported, quoting what arrived.
	t.Run("TooFew", func(t *testing.T) {
		_, err := fields("PWR", "-050", 2)
		if err == nil || !strings.Contains(err.Error(), "has 1 fields, want at least 2") {
			t.Fatalf("got %v, want the counts reported", err)
		}
	})
}

// Test_parseResponse tests the parseResponse function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Success: the echoed command comes off and the value is returned
//   - Unsupported: a bare ERR is reported as a command the scanner lacks
//   - WrongEcho: a reply answering something else is reported
//   - BareEcho: an echo with no value is success with an empty value
//   - RefusedNG: the NG form is reported as a rejection
//   - RefusedERR: the CMD,ERR form is reported as a rejection too
func Test_parseResponse(t *testing.T) {
	// Verify that a normal reply comes back without its echo.
	t.Run("Success", func(t *testing.T) {
		value, err := parseResponse("VOL", "VOL,7")
		if err != nil || value != "7" {
			t.Fatalf("got %q, %v, want 7", value, err)
		}
	})

	// Verify that the argument form is echoed by name alone.
	t.Run("Unsupported", func(t *testing.T) {
		_, err := parseResponse("PWR", "ERR")
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("got %v, want ErrUnsupported", err)
		}
	})

	// Verify that a reply to another command is reported as out of sync.
	t.Run("WrongEcho", func(t *testing.T) {
		_, err := parseResponse("VOL", "SQL,7")
		if err == nil || !strings.Contains(err.Error(), "expected a response to") {
			t.Fatalf("got %v, want an out of sync complaint", err)
		}
	})

	// Verify that a bare echo counts as success with nothing to report.
	t.Run("BareEcho", func(t *testing.T) {
		value, err := parseResponse("VOL,7", "VOL")
		if err != nil || value != "" {
			t.Fatalf("got %q, %v, want an empty success", value, err)
		}
	})

	// Verify that NG is a refusal rather than a value.
	t.Run("RefusedNG", func(t *testing.T) {
		_, err := parseResponse("VOL,7", "VOL,NG")
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})

	// Verify that the CMD,ERR form is a refusal as well.
	t.Run("RefusedERR", func(t *testing.T) {
		_, err := parseResponse("VOL,7", "VOL,ERR")
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})
}

// Test_stale tests the stale function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Replies: a reply naming another command is stale and nothing else is
func Test_stale(t *testing.T) {
	// Verify that only a reply echoing a different command is discarded.
	t.Run("Replies", func(t *testing.T) {
		for _, tt := range []struct {
			command string
			raw     string
			want    bool
		}{
			{command: "VOL", raw: "VOL,7", want: false},
			{command: "VOL,7", raw: "VOL,OK", want: false},
			{command: "VOL", raw: " VOL,7 ", want: false},
			{command: "VOL", raw: "ERR", want: false},
			{command: "VOL", raw: "LCR,38.0,-79.0,10.0", want: true},
			{command: "VOL", raw: "", want: true},
		} {
			if got := stale(tt.command, tt.raw); got != tt.want {
				t.Errorf("stale(%q, %q) is %v, want %v", tt.command, tt.raw, got, tt.want)
			}
		}
	})
}
