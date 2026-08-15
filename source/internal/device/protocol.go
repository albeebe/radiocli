// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"fmt"
	"strings"
)

// fields splits a response value into its comma separated parts and checks
// that there are enough of them.
//
// It tolerates extra trailing fields, because this firmware returns more than
// the specification documents for at least one command, and refusing the reply
// over a field nobody reads would be worse than ignoring it.
//
// Parameters:
//   - command: the command being parsed, named in any error
//   - value: the response value, with the echoed command already stripped
//   - want: the fewest fields the caller needs
//
// Returns:
//   - the fields, which may number more than want
//   - error if fewer than want fields arrived
func fields(command, value string, want int) ([]string, error) {
	got := strings.Split(value, ",")
	if len(got) < want {
		return nil, fmt.Errorf("response to %q has %d fields, want at least %d: %q", command, len(got), want, value)
	}
	return got, nil
}

// parseResponse strips the echoed command from a response line and turns the
// scanner's failure replies into errors.
//
// Parameters:
//   - command: the command that was sent, with any arguments
//   - raw: the response line as it arrived
//
// Returns:
//   - the response value, empty when the scanner echoed the command alone
//   - error if the scanner reported a failure, or if the line answers some
//     other command
//
// Errors:
//   - ErrUnsupported: if the scanner did not recognise the command
//   - ErrRejected: if the scanner understood the command but refused to run it
func parseResponse(command, raw string) (string, error) {
	// A command sent with arguments is echoed by name alone: sending
	// "VOL,12" is answered with "VOL,OK".
	name, _, _ := strings.Cut(command, ",")
	raw = strings.TrimSpace(raw)
	echo, value, found := strings.Cut(raw, ",")

	switch {
	case echo == errName:
		return "", fmt.Errorf("%w: %s", ErrUnsupported, command)
	case echo != name:
		return "", fmt.Errorf("expected a response to %q, got %q", command, raw)
	case !found:
		// Bare "CMD" with no value. The specification documents this as the
		// success reply for VOL and SQL, though this firmware answers
		// "CMD,OK" instead. Accept it as success rather than an error.
		return "", nil
	case value == "NG", value == "ERR":
		return "", fmt.Errorf("%w: %s", ErrRejected, command)
	}

	return value, nil
}

// stale reports whether raw answers some earlier command rather than command.
//
// Every response echoes the name of the command it answers, so a line naming
// something else belongs to an exchange that timed out and was answered late.
// "ERR" is not a command name but this firmware's reply to a command it does
// not know, so it always belongs to whatever was just sent.
//
// Parameters:
//   - command: the command still waiting for its response
//   - raw: the response line as it arrived
//
// Returns:
//   - true if the line echoes a different command's name
func stale(command, raw string) bool {
	name, _, _ := strings.Cut(command, ",")
	echo, _, _ := strings.Cut(strings.TrimSpace(raw), ",")
	return echo != name && echo != errName
}
