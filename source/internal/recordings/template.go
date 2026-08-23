// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package recordings

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Tokens lists every name a template may use, in the order a listing should
// show them.
//
// Returns:
//   - the token names, sorted
func Tokens() []string {
	names := make([]string, 0, len(tokens))
	for name := range tokens {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// parse reads a naming template and checks every token in it.
//
// Checking happens here, which is to say once at startup, rather than when the
// first transmission arrives. The difference matters: a mistyped token found at
// startup costs a second, and the same typo found on the first recording of the
// night costs the night.
//
// Parameters:
//   - text: the template, such as "{date}/{time}_{channel}"
//
// Returns:
//   - the template, broken into the pieces it renders from
//   - error if a brace is unclosed or a token is not one this package knows
//
// Errors:
//   - ErrBadTemplate: for every failure here, so a caller can tell a bad
//     template from a disk that would not cooperate
func parse(text string) (template, error) {
	if strings.TrimSpace(text) == "" {
		return template{}, fmt.Errorf("%w: it is empty", ErrBadTemplate)
	}

	t := template{text: text}
	var literal strings.Builder

	for i := 0; i < len(text); {
		switch {
		// A doubled brace is one literal brace, which is the only way to write
		// one in a template whose whole syntax is braces.
		case strings.HasPrefix(text[i:], "{{"), strings.HasPrefix(text[i:], "}}"):
			literal.WriteByte(text[i])
			i += 2

		case text[i] == '}':
			return template{}, fmt.Errorf("%w: %q has a %q with no %q before it, "+
				"and a brace of its own is written %q", ErrBadTemplate, text, "}", "{", "}}")

		case text[i] == '{':
			end := strings.IndexByte(text[i:], '}')
			if end < 0 {
				return template{}, fmt.Errorf("%w: %q has a %q that is never closed",
					ErrBadTemplate, text, "{")
			}
			name := text[i+1 : i+end]
			if _, ok := tokens[name]; !ok {
				return template{}, fmt.Errorf("%w: %q is not a token: the tokens are %s",
					ErrBadTemplate, name, strings.Join(Tokens(), ", "))
			}

			if literal.Len() > 0 {
				t.parts = append(t.parts, part{literal: literal.String()})
				literal.Reset()
			}
			t.parts = append(t.parts, part{token: name})
			i += end + 1

		default:
			literal.WriteByte(text[i])
			i++
		}
	}

	if literal.Len() > 0 {
		t.parts = append(t.parts, part{literal: literal.String()})
	}

	// A template of nothing but literals names one file, which every recording
	// would then overwrite in turn. The collision suffix would keep them all,
	// but as a numbered pile with nothing to tell them apart, so it is refused
	// as the mistake it is.
	if !t.varies() {
		return template{}, fmt.Errorf("%w: %q has no tokens in it, so every recording "+
			"would be given the same name: the tokens are %s",
			ErrBadTemplate, text, strings.Join(Tokens(), ", "))
	}
	return t, nil
}

// render turns a template and a recording into a path below the destination.
//
// Parameters:
//   - e: the recording to name
//
// Returns:
//   - the path, using forward slashes, with no extension
func (t template) render(e Entry) string {
	var b strings.Builder
	for _, p := range t.parts {
		if p.token == "" {
			b.WriteString(p.literal)
			continue
		}
		b.WriteString(sanitize(tokens[p.token](e)))
	}

	// Cleaning up happens per component, after everything has been substituted,
	// which is what makes an empty token disappear along with the separator
	// beside it. A recording the scanner never named comes out as "19-54-03"
	// rather than "19-54-03___".
	var kept []string
	for _, c := range strings.Split(b.String(), "/") {
		if c = tidy(c); c != "" {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		// Every component was empty, which a template with no tokens cannot
		// cause and an unlabelled recording can. It still needs a name.
		kept = []string{tokens["datetime"](e)}
	}
	return shorten(kept)
}

// varies reports whether a template has any token in it, and so whether two
// recordings can be told apart by their names.
//
// Returns:
//   - true if at least one token is present
func (t template) varies() bool {
	for _, p := range t.parts {
		if p.token != "" {
			return true
		}
	}
	return false
}

// sanitize turns a value from the scanner into something safe to put in a path.
//
// Names come off a radio somebody else programmed, and they contain spaces,
// slashes, colons and occasionally worse. A slash is the dangerous one: left
// alone, a channel called "FIRE/EMS" would invent a directory, which is how a
// naming scheme turns into a filesystem the user did not ask for.
//
// Parameters:
//   - value: the value as the scanner reported it
//
// Returns:
//   - the value with everything outside letters, digits, dot, dash and
//     underscore replaced, runs collapsed, and the ends trimmed
func sanitize(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return tidy(b.String())
}

// shorten brings a path within the limits, taking from the longest component
// first.
//
// Doing it here rather than letting the operating system refuse is the point.
// Windows stops at 260 characters by default, and a template carrying a system,
// a department and a channel name reaches that without trying, at which point
// the recorder fails on a transmission rather than on a setting. Taking from
// the longest first means what is lost is spread over the parts that can spare
// it, and the result is the same every time for the same input.
//
// Parameters:
//   - parts: the path components, none of them empty
//
// Returns:
//   - the components joined, within maxComponent each and maxPath overall
func shorten(parts []string) string {
	for i, c := range parts {
		if len(c) > maxComponent {
			parts[i] = c[:maxComponent]
		}
	}

	// Take one character at a time from whichever component is longest, which
	// is a few hundred iterations at worst and needs no arithmetic to be sure
	// it terminates.
	for len(path.Join(parts...)) > maxPath {
		longest, at := 0, -1
		for i, c := range parts {
			if len(c) > longest && len(c) > minComponent {
				longest, at = len(c), i
			}
		}
		if at < 0 {
			// Everything is already as short as it may go. A path this long
			// can only come from a template with a great many components, and
			// truncating past this would leave them unrecognisable.
			break
		}
		parts[at] = parts[at][:len(parts[at])-1]
	}
	return path.Join(parts...)
}

// tidy collapses runs of separators and trims them off both ends.
//
// It is what makes an empty token vanish cleanly. Substituting nothing between
// two underscores leaves them next to each other, and a name ending in a
// separator is the mark of a template whose last token was empty. Neither is
// worth showing to anybody.
//
// A leading dot is trimmed for a different reason: it would hide the recording
// on every system that treats a dot as hidden, and this package uses that
// convention itself for files still being written.
//
// Parameters:
//   - value: one path component, already free of anything unsafe
//
// Returns:
//   - the component with separator runs collapsed and its ends trimmed
func tidy(value string) string {
	var b strings.Builder
	var last rune
	for _, r := range value {
		if separator(r) && separator(last) {
			continue
		}
		b.WriteRune(r)
		last = r
	}
	return strings.Trim(b.String(), "-_.")
}

// separator reports whether a character is one this package collapses runs of.
//
// Parameters:
//   - r: the character to test
//
// Returns:
//   - true if it separates parts of a name rather than being part of one
func separator(r rune) bool {
	return r == '-' || r == '_'
}
