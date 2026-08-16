// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import "testing"

// Test_compare tests the ordering of two parsed versions.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Major: the first number decides it, in both directions
//   - Minor: the second number decides it when the first matches
//   - Patch: the third number decides it when the first two match
//   - Same: identical releases compare equal
//   - ReleaseBeatsPrerelease: v1.0.0 is newer than v1.0.0-rc.1
//   - PrereleaseLosesToRelease: the same comparison from the other side
//   - TwoPrereleases: both have one, so the prereleases decide it
func Test_compare(t *testing.T) {
	// Verify that the major number decides the order regardless of the rest
	t.Run("Major", func(t *testing.T) {
		older, newer := version{major: 1, minor: 9, patch: 9}, version{major: 2}
		if got := older.compare(newer); got != -1 {
			t.Errorf("expected 1.9.9 to be older than 2.0.0, got: %d", got)
		}
		if got := newer.compare(older); got != 1 {
			t.Errorf("expected 2.0.0 to be newer than 1.9.9, got: %d", got)
		}
	})

	// Verify that the minor number decides it when the major numbers match
	t.Run("Minor", func(t *testing.T) {
		older, newer := version{minor: 1, patch: 9}, version{minor: 2}
		if got := older.compare(newer); got != -1 {
			t.Errorf("expected 0.1.9 to be older than 0.2.0, got: %d", got)
		}
		if got := newer.compare(older); got != 1 {
			t.Errorf("expected 0.2.0 to be newer than 0.1.9, got: %d", got)
		}
	})

	// Verify that the patch number decides it when everything above matches
	t.Run("Patch", func(t *testing.T) {
		older, newer := version{patch: 1}, version{patch: 2}
		if got := older.compare(newer); got != -1 {
			t.Errorf("expected 0.0.1 to be older than 0.0.2, got: %d", got)
		}
		if got := newer.compare(older); got != 1 {
			t.Errorf("expected 0.0.2 to be newer than 0.0.1, got: %d", got)
		}
	})

	// Verify that two identical releases compare equal
	t.Run("Same", func(t *testing.T) {
		v := version{major: 1, minor: 2, patch: 3}
		if got := v.compare(v); got != 0 {
			t.Errorf("expected 1.2.3 to equal itself, got: %d", got)
		}
	})

	// Verify that a release is newer than its own release candidate
	t.Run("ReleaseBeatsPrerelease", func(t *testing.T) {
		release := version{major: 1}
		candidate := version{major: 1, pre: "rc.1"}
		if got := release.compare(candidate); got != 1 {
			t.Errorf("expected 1.0.0 to be newer than 1.0.0-rc.1, got: %d", got)
		}
	})

	// Verify the same comparison from the candidate's side
	t.Run("PrereleaseLosesToRelease", func(t *testing.T) {
		release := version{major: 1}
		candidate := version{major: 1, pre: "rc.1"}
		if got := candidate.compare(release); got != -1 {
			t.Errorf("expected 1.0.0-rc.1 to be older than 1.0.0, got: %d", got)
		}
	})

	// Verify that two prereleases of the same version are ordered by the
	// prerelease itself
	t.Run("TwoPrereleases", func(t *testing.T) {
		first := version{major: 1, pre: "rc.1"}
		second := version{major: 1, pre: "rc.2"}
		if got := first.compare(second); got != -1 {
			t.Errorf("expected rc.1 to be older than rc.2, got: %d", got)
		}
	})
}

// Test_comparePre tests the ordering of two prerelease strings.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Numeric: digit segments are compared as numbers, so rc.9 precedes rc.10
//   - NumericBelowText: a number sorts below a word at the same position
//   - TextBelowNumeric: the same rule seen from the other side
//   - Text: two words are compared as text
//   - Shorter: when every shared segment matches, fewer segments sorts lower
//   - Identical: the same string compares equal
func Test_comparePre(t *testing.T) {
	// Verify that digit segments are compared numerically rather than as text,
	// which is the whole reason this function exists
	t.Run("Numeric", func(t *testing.T) {
		if got := comparePre("rc.9", "rc.10"); got != -1 {
			t.Errorf("expected rc.9 to sort before rc.10, got: %d", got)
		}
		if got := comparePre("rc.10", "rc.9"); got != 1 {
			t.Errorf("expected rc.10 to sort after rc.9, got: %d", got)
		}
	})

	// Verify that a numeric segment sorts below a non-numeric one
	t.Run("NumericBelowText", func(t *testing.T) {
		if got := comparePre("1", "alpha"); got != -1 {
			t.Errorf("expected 1 to sort before alpha, got: %d", got)
		}
	})

	// Verify the same rule with the numeric segment on the right
	t.Run("TextBelowNumeric", func(t *testing.T) {
		if got := comparePre("alpha", "1"); got != 1 {
			t.Errorf("expected alpha to sort after 1, got: %d", got)
		}
	})

	// Verify that two words are compared as text
	t.Run("Text", func(t *testing.T) {
		if got := comparePre("alpha", "beta"); got != -1 {
			t.Errorf("expected alpha to sort before beta, got: %d", got)
		}
	})

	// Verify that a shorter run of otherwise equal segments sorts lower
	t.Run("Shorter", func(t *testing.T) {
		if got := comparePre("rc", "rc.1"); got != -1 {
			t.Errorf("expected rc to sort before rc.1, got: %d", got)
		}
		if got := comparePre("rc.1", "rc"); got != 1 {
			t.Errorf("expected rc.1 to sort after rc, got: %d", got)
		}
	})

	// Verify that identical prereleases compare equal
	t.Run("Identical", func(t *testing.T) {
		if got := comparePre("rc.1", "rc.1"); got != 0 {
			t.Errorf("expected rc.1 to equal itself, got: %d", got)
		}
	})
}

// Test_parseVersion tests reading a release tag.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - Tagged: a tag with its leading v
//   - Bare: the same version without the v
//   - Prerelease: the prerelease is kept, without its dash
//   - BuildMetadata: anything after a plus is dropped
//   - Dev: an unstamped build is not a version
//   - TooFewParts: two numbers is not a version
//   - NotANumber: a part that is not digits is not a version
//   - EmptyPrerelease: a trailing dash with nothing after it is not a version
func Test_parseVersion(t *testing.T) {
	// Verify that a published tag parses, leading v and all
	t.Run("Tagged", func(t *testing.T) {
		v, ok := parseVersion("v0.1.1")
		if !ok {
			t.Fatal("expected v0.1.1 to parse")
		}
		if v != (version{major: 0, minor: 1, patch: 1}) {
			t.Errorf("expected 0.1.1, got: %+v", v)
		}
	})

	// Verify that the v is optional, so --version 0.1.1 works as typed
	t.Run("Bare", func(t *testing.T) {
		v, ok := parseVersion("0.1.1")
		if !ok || v != (version{minor: 1, patch: 1}) {
			t.Errorf("expected 0.1.1 to parse, got: %+v %v", v, ok)
		}
	})

	// Verify that a prerelease is kept without its leading dash
	t.Run("Prerelease", func(t *testing.T) {
		v, ok := parseVersion("v0.2.0-rc.1")
		if !ok {
			t.Fatal("expected v0.2.0-rc.1 to parse")
		}
		if v != (version{minor: 2, pre: "rc.1"}) {
			t.Errorf("expected 0.2.0-rc.1, got: %+v", v)
		}
	})

	// Verify that build metadata is dropped, since it takes no part in
	// precedence
	t.Run("BuildMetadata", func(t *testing.T) {
		v, ok := parseVersion("v1.0.0+20260816")
		if !ok || v != (version{major: 1}) {
			t.Errorf("expected 1.0.0 with the metadata dropped, got: %+v %v", v, ok)
		}
	})

	// Verify that an unstamped build reports that it is not a version, which is
	// what makes stateOf answer "unknown" rather than guessing
	t.Run("Dev", func(t *testing.T) {
		if _, ok := parseVersion(devVersion); ok {
			t.Error("expected dev not to parse as a version")
		}
	})

	// Verify that a two-part version is refused
	t.Run("TooFewParts", func(t *testing.T) {
		if _, ok := parseVersion("v1.0"); ok {
			t.Error("expected v1.0 not to parse as a version")
		}
	})

	// Verify that a part which is not digits is refused
	t.Run("NotANumber", func(t *testing.T) {
		if _, ok := parseVersion("v1.x.0"); ok {
			t.Error("expected v1.x.0 not to parse as a version")
		}
	})

	// Verify that a dash with nothing after it is refused rather than read as a
	// release with an empty prerelease
	t.Run("EmptyPrerelease", func(t *testing.T) {
		if _, ok := parseVersion("v1.0.0-"); ok {
			t.Error("expected v1.0.0- not to parse as a version")
		}
	})
}

// Test_stateOf tests how the running build is placed against a release.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Available: the release is newer than the running build
//   - Current: they are the same version
//   - Ahead: the running build is newer than the release
//   - DevBuild: an unstamped build cannot be compared
//   - UnreadableRelease: a tag that is not a version cannot be compared
func Test_stateOf(t *testing.T) {
	// Verify the ordinary case, a release newer than what is running
	t.Run("Available", func(t *testing.T) {
		if got := stateOf("v0.1.0", "v0.1.1"); got != stateAvailable {
			t.Errorf("expected %q, got: %q", stateAvailable, got)
		}
	})

	// Verify that the same version reports as current
	t.Run("Current", func(t *testing.T) {
		if got := stateOf("v0.1.1", "v0.1.1"); got != stateCurrent {
			t.Errorf("expected %q, got: %q", stateCurrent, got)
		}
	})

	// Verify that a build newer than the release reports as ahead, which is
	// what stops something polling this from downgrading itself in a loop
	t.Run("Ahead", func(t *testing.T) {
		if got := stateOf("v0.2.0", "v0.1.1"); got != stateAhead {
			t.Errorf("expected %q, got: %q", stateAhead, got)
		}
	})

	// Verify that an unstamped build reports unknown rather than guessing
	t.Run("DevBuild", func(t *testing.T) {
		if got := stateOf(devVersion, "v0.1.1"); got != stateUnknown {
			t.Errorf("expected %q, got: %q", stateUnknown, got)
		}
	})

	// Verify that a release tag which is not a version reports unknown
	t.Run("UnreadableRelease", func(t *testing.T) {
		if got := stateOf("v0.1.1", "nightly"); got != stateUnknown {
			t.Errorf("expected %q, got: %q", stateUnknown, got)
		}
	})
}
