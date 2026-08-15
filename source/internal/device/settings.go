// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// QuickKeyState is whether a quick key exists and whether it is enabled.
type QuickKeyState int

// Clock returns the scanner's date and time.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Clock holding the scanner's wall clock time, built in time.Local, along
//     with its daylight saving flag and whether the clock is running
//   - error if the exchange fails, the reply is short, or a field is not a
//     number
func (s *Scanner) Clock(ctx context.Context) (Clock, error) {
	value, err := s.conn.Execute(ctx, "DTM")
	if err != nil {
		return Clock{}, err
	}

	f, err := fields("DTM", value, 8)
	if err != nil {
		return Clock{}, err
	}

	nums := make([]int, 7)
	names := [...]string{"daylight saving flag", "year", "month", "day", "hour", "minute", "second"}
	for i := range nums {
		if nums[i], err = atoi("DTM", names[i], f[i]); err != nil {
			return Clock{}, err
		}
	}

	return Clock{
		Time:           time.Date(nums[1], time.Month(nums[2]), nums[3], nums[4], nums[5], nums[6], 0, time.Local),
		DaylightSaving: nums[0] == 1,
		Valid:          strings.TrimSpace(f[7]) == "1",
	}, nil
}

// DepartmentQuickKeys returns the state of the department quick keys within
// one system of one favorites list.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - favoritesKey: the quick key of the favorites list to look in
//   - systemKey: the quick key of the system within that list
//
// Returns:
//   - the department quick key states, indexed by key number
//   - error if the exchange fails, the reply is short, or a state is not a
//     number
func (s *Scanner) DepartmentQuickKeys(ctx context.Context, favoritesKey, systemKey int) ([]QuickKeyState, error) {
	value, err := s.conn.Execute(ctx, fmt.Sprintf("DQK,%d,%d", favoritesKey, systemKey))
	if err != nil {
		return nil, err
	}

	// The reply repeats the favorites and system key before the states.
	return parseQuickKeys("DQK", value, 2)
}

// FavoriteQuickKeys returns the state of the hundred favorites list quick
// keys, indexed by key number.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - the favorites list quick key states, indexed by key number
//   - error if the exchange fails or a state is not a number
func (s *Scanner) FavoriteQuickKeys(ctx context.Context) ([]QuickKeyState, error) {
	value, err := s.conn.Execute(ctx, "FQK")
	if err != nil {
		return nil, err
	}
	return parseQuickKeys("FQK", value, 0)
}

// Location returns the position the scanner is working from, and the radius it
// draws database channels from.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - Location holding the latitude, longitude, and range the scanner is
//     working from
//   - error if the exchange fails, the reply is short, or a field is not a
//     number
func (s *Scanner) Location(ctx context.Context) (Location, error) {
	value, err := s.conn.Execute(ctx, "LCR")
	if err != nil {
		return Location{}, err
	}

	f, err := fields("LCR", value, 3)
	if err != nil {
		return Location{}, err
	}

	var loc Location
	for i, target := range []*float64{&loc.Latitude, &loc.Longitude, &loc.Range} {
		name := [...]string{"latitude", "longitude", "range"}[i]
		if *target, err = strconv.ParseFloat(strings.TrimSpace(f[i]), 64); err != nil {
			return Location{}, fmt.Errorf("response to %q has an invalid %s: %q", "LCR", name, f[i])
		}
	}

	return loc, nil
}

// Recording reports whether the scanner is recording received audio.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - true if the scanner is recording
//   - error if the exchange fails
func (s *Scanner) Recording(ctx context.Context) (bool, error) {
	value, err := s.conn.Execute(ctx, "URC")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(value) == "1", nil
}

// ServiceTypes returns which service types the scanner will scan, indexed in
// the order the specification lists them: 37 preset types followed by 10
// custom ones.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - one entry per service type, true where the type will be scanned
//   - error if the exchange fails
func (s *Scanner) ServiceTypes(ctx context.Context) ([]bool, error) {
	value, err := s.conn.Execute(ctx, "SVC")
	if err != nil {
		return nil, err
	}

	parts := strings.Split(value, ",")
	on := make([]bool, 0, len(parts))
	for _, p := range parts {
		on = append(on, strings.TrimSpace(p) == "1")
	}
	return on, nil
}

// SetClock sets the scanner's date and time, and its daylight saving flag.
//
// The time is sent as wall clock digits in t's own location, so pass a time
// already in the zone the scanner should show. It takes effect immediately and
// reads back unchanged.
//
// The daylight saving flag is not a label. On a scanner with a GPS the clock is
// re-derived from the satellites, and the flag is added to whatever offset the
// scanner is configured with, so a scanner already configured for summer time
// runs an hour fast with the flag set. Measured on an SDS150 with a fix: the
// flag set leaves it an hour ahead within the hour, whatever was written here,
// and the flag clear leaves it correct.
//
// Because of that, the flag should be carried over from what the scanner
// already holds unless the caller means to change it. Deriving it from the
// host's own daylight saving is exactly the mistake it looks like it is not.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - t: the time to write, read as wall clock digits in its own location
//   - daylightSaving: the flag to write, which the scanner adds to its
//     configured offset rather than treating as a label
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) SetClock(ctx context.Context, t time.Time, daylightSaving bool) error {
	dst := 0
	if daylightSaving {
		dst = 1
	}

	return s.set(ctx, fmt.Sprintf("DTM,%d,%04d,%02d,%02d,%02d,%02d,%02d",
		dst, t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second()))
}

// SetDepartmentQuickKeys sets the department quick keys within one system.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - favoritesKey: the quick key of the favorites list to write in
//   - systemKey: the quick key of the system within that list
//   - states: the states to write, in key order, as returned by
//     DepartmentQuickKeys
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) SetDepartmentQuickKeys(ctx context.Context, favoritesKey, systemKey int, states []QuickKeyState) error {
	return s.set(ctx, fmt.Sprintf("DQK,%d,%d,%s", favoritesKey, systemKey, formatQuickKeys(states)))
}

// SetFavoriteQuickKeys sets the favorites list quick keys. Entries set to
// QuickKeyAbsent are ignored by the scanner rather than clearing the key.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - states: the states to write, in key order, as returned by
//     FavoriteQuickKeys
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) SetFavoriteQuickKeys(ctx context.Context, states []QuickKeyState) error {
	return s.set(ctx, "FQK,"+formatQuickKeys(states))
}

// SetLocation sets the position the scanner works from, and its range.
//
// This is the only way to put a position back where it was. A zip code cannot
// be read off the scanner: it resolves one to a position when it is typed and
// keeps only the position, so "the zip it was set to" is not something that
// survives anywhere. A position is, and this writes one directly.
//
// It is not how "location set <zip>" works, and should not become how. A zip
// is how a person says where they mean, and only the scanner can turn one into
// a position, because the zip database lives on the scanner.
//
// Six decimal places, which is what Location reads back. The scanner keeps
// them to within a millionth of a degree, about a tenth of a metre: 38.433056
// written comes back as 38.433055. That rounding happens once rather than
// accumulating, so a position read and written back repeatedly stays put:
// measured over five cycles, every reading after the first was identical.
// Four places were written here before, which rounded to roughly eleven metres.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - loc: the position to work from, and the radius in miles to draw database
//     channels from
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) SetLocation(ctx context.Context, loc Location) error {
	return s.set(ctx, fmt.Sprintf("LCR,%.6f,%.6f,%.1f", loc.Latitude, loc.Longitude, loc.Range))
}

// SetRecording starts or stops recording received audio.
//
// The scanner reports its own reasons for refusing separately from the usual
// rejection, so a full memory card or a flat battery comes back as a specific
// error rather than as ErrRejected.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - on: true to start recording, false to stop
//
// Returns:
//   - error if the exchange fails, the scanner names a reason it could not
//     record, or the reply is not one it uses for success
func (s *Scanner) SetRecording(ctx context.Context, on bool) error {
	state := 0
	if on {
		state = 1
	}

	value, err := s.conn.Execute(ctx, fmt.Sprintf("URC,%d", state))
	if err != nil {
		return err
	}
	if code, found := strings.CutPrefix(value, "ERR,"); found {
		return fmt.Errorf("scanner could not change recording: %s", recordingError(strings.TrimSpace(code)))
	}
	if value != "" && value != okValue {
		return fmt.Errorf("unexpected response to %q: %q", "URC", value)
	}
	return nil
}

// SetServiceTypes sets which service types the scanner will scan. Pass the
// full list as returned by ServiceTypes.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - on: one entry per service type, true where the type should be scanned
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) SetServiceTypes(ctx context.Context, on []bool) error {
	parts := make([]string, 0, len(on))
	for _, v := range on {
		if v {
			parts = append(parts, "1")
			continue
		}
		parts = append(parts, "0")
	}
	return s.set(ctx, "SVC,"+strings.Join(parts, ","))
}

// SetSystemQuickKeys sets the system quick keys within one favorites list.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - favoritesKey: the quick key of the favorites list to write in
//   - states: the states to write, in key order, as returned by
//     SystemQuickKeys
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) SetSystemQuickKeys(ctx context.Context, favoritesKey int, states []QuickKeyState) error {
	return s.set(ctx, fmt.Sprintf("SQK,%d,%s", favoritesKey, formatQuickKeys(states)))
}

// SystemQuickKeys returns the state of the system quick keys within one
// favorites list.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - favoritesKey: the quick key of the favorites list to look in
//
// Returns:
//   - the system quick key states, indexed by key number
//   - error if the exchange fails, the reply is short, or a state is not a
//     number
func (s *Scanner) SystemQuickKeys(ctx context.Context, favoritesKey int) ([]QuickKeyState, error) {
	value, err := s.conn.Execute(ctx, fmt.Sprintf("SQK,%d", favoritesKey))
	if err != nil {
		return nil, err
	}

	// The reply repeats the favorites and system key before the states.
	return parseQuickKeys("SQK", value, 2)
}

// formatQuickKeys renders quick key states for sending.
//
// Parameters:
//   - states: the states to render, in key order
//
// Returns:
//   - the states as comma separated numbers, ready to embed in a command
func formatQuickKeys(states []QuickKeyState) string {
	parts := make([]string, 0, len(states))
	for _, st := range states {
		parts = append(parts, strconv.Itoa(int(st)))
	}
	return strings.Join(parts, ",")
}

// parseQuickKeys reads a run of quick key states, skipping the echoed key
// numbers some of these replies begin with.
//
// Parameters:
//   - command: the command being answered, for the error message
//   - value: the reply to read
//   - skip: how many echoed fields precede the states
//
// Returns:
//   - the quick key states the reply carries, in the order it gives them
//   - error if the reply is too short to hold any states, or a state is not a
//     number
func parseQuickKeys(command, value string, skip int) ([]QuickKeyState, error) {
	parts := strings.Split(value, ",")
	if len(parts) <= skip {
		return nil, errShortResponse(command, len(parts), skip+1, "quick key states")
	}

	states := make([]QuickKeyState, 0, len(parts)-skip)
	for _, p := range parts[skip:] {
		n, err := atoi(command, "quick key state", p)
		if err != nil {
			return nil, err
		}
		states = append(states, QuickKeyState(n))
	}
	return states, nil
}

// recordingError turns a recording error code into words.
//
// Parameters:
//   - code: the code the scanner reported, such as "0002"
//
// Returns:
//   - the reason in words, or the code itself when it is not one of the
//     documented ones
func recordingError(code string) string {
	switch code {
	case "0001":
		return "the memory card could not be written"
	case "0002":
		return "the battery is too low"
	case "0003":
		return "the session limit has been reached"
	case "0004":
		return "the clock has been lost, so recordings cannot be timestamped"
	default:
		return fmt.Sprintf("error code %s", code)
	}
}
