// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"strconv"
	"strings"
)

// compare orders two versions, following the precedence rules semantic
// versioning defines.
//
// The numbers decide it almost always. The prerelease only matters when the
// numbers are equal, and then the rule is the one people find surprising:
// v1.0.0-rc.1 is *older* than v1.0.0, because a release candidate comes before
// the release it is a candidate for. Getting that backwards would leave anybody
// running a candidate unable to move to the real thing.
//
// Parameters:
//   - o: the version to compare this one against
//
// Returns:
//   - -1 when v is older than o, 0 when they are the same version, and 1 when
//     v is newer
func (v version) compare(o version) int {
	for _, pair := range [][2]int{
		{v.major, o.major},
		{v.minor, o.minor},
		{v.patch, o.patch},
	} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}

	switch {
	case v.pre == "" && o.pre == "":
		return 0
	case v.pre == "":
		return 1
	case o.pre == "":
		return -1
	}
	return comparePre(v.pre, o.pre)
}

// comparePre orders two prerelease strings, such as "rc.1" against "rc.2".
//
// Both are split on dots and compared segment by segment. A segment of digits
// is compared as a number so that rc.9 comes before rc.10, which comparing the
// text would get wrong. A numeric segment sorts below a non-numeric one, and
// when every shared segment matches, the string with fewer segments sorts
// lower. That is the whole of the specification's rule.
//
// Both arguments are non-empty: compare handles the cases where one or both
// versions have no prerelease before calling this.
//
// Parameters:
//   - a: the first prerelease, without its leading "-"
//   - b: the second prerelease, without its leading "-"
//
// Returns:
//   - -1 when a sorts before b, 0 when they are identical, and 1 when a sorts
//     after b
func comparePre(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")

	for i := 0; i < len(left) && i < len(right); i++ {
		ln, lErr := strconv.Atoi(left[i])
		rn, rErr := strconv.Atoi(right[i])

		switch {
		case lErr == nil && rErr == nil:
			if ln != rn {
				if ln < rn {
					return -1
				}
				return 1
			}
		case lErr == nil:
			return -1
		case rErr == nil:
			return 1
		default:
			if c := strings.Compare(left[i], right[i]); c != 0 {
				return c
			}
		}
	}

	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	}
	return 0
}

// parseVersion reads a release tag such as "v0.1.1" or "v0.2.0-rc.1".
//
// It reports whether the string was a version rather than returning an error,
// because "this is not a version" is an ordinary answer here rather than a
// failure. An unstamped build calls itself "dev", and asking whether "dev" is
// older than v0.1.1 is a question with no answer, not a problem to report.
//
// Build metadata after a "+" is stripped and ignored, which is what the
// specification says to do: it takes no part in precedence.
//
// Parameters:
//   - s: the tag to read, with or without its leading "v"
//
// Returns:
//   - the parsed version, and whether s was one. The version is meaningless
//     when the second return is false.
func parseVersion(s string) (version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if build := strings.IndexByte(s, '+'); build >= 0 {
		s = s[:build]
	}

	var v version
	if dash := strings.IndexByte(s, '-'); dash >= 0 {
		v.pre = s[dash+1:]
		s = s[:dash]
		if v.pre == "" {
			return version{}, false
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return version{}, false
	}

	for i, into := range []*int{&v.major, &v.minor, &v.patch} {
		// A sign cannot reach this point even though strconv.Atoi would accept
		// one. Everything from the first "-" was taken as the prerelease and
		// everything from the first "+" as build metadata, so "1.-2.3" has
		// already failed above with too few parts rather than arriving here as
		// a negative number.
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return version{}, false
		}
		*into = n
	}
	return v, true
}

// stateOf says how the running build stands against the release it was
// compared with.
//
// The interesting answer is stateAhead. Somebody who installed a prerelease
// with --version, or who built from a tag that is not published yet, is running
// something newer than the newest release, and reporting that as an update
// being available would have an agent downgrade itself in a loop.
//
// Parameters:
//   - current: the version of the running build, which is "dev" when it was not
//     stamped at link time
//   - latest: the tag of the release it was compared with
//
// Returns:
//   - one of stateAvailable, stateCurrent, stateAhead, or stateUnknown when
//     either side is not a version that can be compared
func stateOf(current, latest string) string {
	cv, ok := parseVersion(current)
	if !ok {
		return stateUnknown
	}
	lv, ok := parseVersion(latest)
	if !ok {
		return stateUnknown
	}

	switch cv.compare(lv) {
	case -1:
		return stateAvailable
	case 0:
		return stateCurrent
	}
	return stateAhead
}
