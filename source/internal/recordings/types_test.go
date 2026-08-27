// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/25/2026

package recordings

import "testing"

// TestCmp tests the cmp function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - FirstWins: the first non-empty value is the answer
//   - BlankIsEmpty: whitespace does not count as a value
//   - NothingLeft: no candidates at all comes back empty
func TestCmp(t *testing.T) {
	// Verify the first value that carries anything is returned, and the rest
	// are never considered.
	t.Run("FirstWins", func(t *testing.T) {
		if got := cmp("24944", "155.550000MHz"); got != "24944" {
			t.Errorf("got %q, want the first non-empty value", got)
		}
	})

	// Verify a value of nothing but whitespace is skipped the same as an empty
	// one, since the scanner reports blanks both ways.
	t.Run("BlankIsEmpty", func(t *testing.T) {
		if got := cmp("", "   ", "155.550000MHz"); got != "155.550000MHz" {
			t.Errorf("got %q, want the blank candidates skipped", got)
		}
	})

	// Verify that when every candidate is empty the answer is empty too, which
	// is what lets an unset token vanish from a name.
	t.Run("NothingLeft", func(t *testing.T) {
		if got := cmp("", ""); got != "" {
			t.Errorf("got %q, want nothing", got)
		}
	})
}
