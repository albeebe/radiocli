// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package buildinfo

import "testing"

// TestDefaults tests the package's link-time variables.
//
// Coverage: 100% of coverable statements (the package contains only var
// declarations, so Go's coverage tooling reports "[no statements]")
//
// Test cases:
//   - Commit: unstamped default is "none"
//   - Date: unstamped default is "unknown"
//   - Version: unstamped default is "dev"
func TestDefaults(t *testing.T) {
	// Verify that an unstamped build reports "none" for the git revision
	t.Run("Commit", func(t *testing.T) {
		if Commit != "none" {
			t.Errorf("expected Commit default %q, got %q", "none", Commit)
		}
	})

	// Verify that an unstamped build reports "unknown" for the build timestamp
	t.Run("Date", func(t *testing.T) {
		if Date != "unknown" {
			t.Errorf("expected Date default %q, got %q", "unknown", Date)
		}
	})

	// Verify that an unstamped build reports "dev" for the release version
	t.Run("Version", func(t *testing.T) {
		if Version != "dev" {
			t.Errorf("expected Version default %q, got %q", "dev", Version)
		}
	})
}
