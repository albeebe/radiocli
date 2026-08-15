// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package cmdline splits a typed command line into arguments.
//
// It is a package of its own because two places have to agree on it. The daemon
// splits the line it is handed over the socket and runs it, and the config
// command checks a macro's steps with the same splitter before storing them, so
// a line accepted when it is saved has to be a line that splits when it is
// run. One splitter is what makes those the same question.
package cmdline

import (
	"fmt"
	"strings"
)

// Split splits a typed command line into arguments the way a shell would, and
// nothing more.
//
// It understands single quotes, double quotes and backslash escapes, because
// the scanner's own list names contain spaces and commas and have to be
// quotable: "GREENDALE, ST 00000" is one argument. It does not expand
// variables, globs, history or anything else, and the result is passed
// straight to the command tree rather than to a shell.
//
// Parameters:
//   - line: the raw command line as typed, quoting and escapes included
//
// Returns:
//   - the arguments in order; nil when the line holds only whitespace
//   - error if a quote is left unclosed, the line ends in a bare backslash,
//     or an unquoted shell metacharacter appears in the line
func Split(line string) ([]string, error) {
	var (
		args []string
		cur  strings.Builder
		// open tracks whether a token has been started, which is what tells an
		// empty string argument, written as "", apart from no argument at all.
		open bool
	)

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		c := runes[i]

		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			// Whitespace closes the argument being built, if any.
			if open {
				args = append(args, cur.String())
				cur.Reset()
				open = false
			}

		case c == '\'':
			// Single quotes are literal all the way to the closing quote, so
			// there is nothing to interpret in between.
			open = true
			j := i + 1
			for j < len(runes) && runes[j] != '\'' {
				cur.WriteRune(runes[j])
				j++
			}
			if j == len(runes) {
				return nil, fmt.Errorf("unclosed ' quote")
			}
			i = j

		case c == '"':
			open = true
			j := i + 1
			for j < len(runes) && runes[j] != '"' {
				// Only a quote and a backslash can be escaped. Every other
				// backslash is an ordinary character, which is what makes a
				// Windows path survive being typed.
				if runes[j] == '\\' && j+1 < len(runes) && (runes[j+1] == '"' || runes[j+1] == '\\') {
					j++
				}
				cur.WriteRune(runes[j])
				j++
			}
			if j == len(runes) {
				return nil, fmt.Errorf(`unclosed " quote`)
			}
			i = j

		case c == '\\':
			// A backslash outside quotes makes the next character literal.
			if i+1 == len(runes) {
				return nil, fmt.Errorf("trailing backslash")
			}
			i++
			cur.WriteRune(runes[i])
			open = true

		case strings.ContainsRune(metacharacters, c):
			// A bare shell operator is refused rather than passed through.
			return nil, fmt.Errorf("%q is not supported: commands run directly rather than through a shell, so there are no pipes, redirects, background jobs or variables", string(c))

		default:
			// Anything else is an ordinary character in the current argument.
			cur.WriteRune(c)
			open = true
		}
	}

	// Close the final argument if one is still open.
	if open {
		args = append(args, cur.String())
	}
	return args, nil
}
