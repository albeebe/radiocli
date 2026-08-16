// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/13/2026

// Package render turns values into the two shapes every command's output takes.
//
// It exists because the command packages deliberately do not import each other,
// and a rule like that has a cost: the same tiny formatter gets written again
// in every package that needs it. That cost is worth paying for anything a
// command owns, because what "channels" does with a talkgroup is genuinely not
// what "sites" does with a frequency. It is not worth paying for turning an
// empty string into a dash.
//
// So the line is drawn at coupling rather than at size. Nothing here knows what
// any one command does, and the only package it depends on is appcontext, which
// is the output setting and the stream to write to. A command importing this is
// importing downward onto a layer, which is not the horizontal dependency the
// commands README rules out.
//
// The divergence is the real argument. Six copies of one four-line function are
// not six times the maintenance of one; they are six chances for the seventh
// caller to find that two of them disagree, and a listing where an empty value
// reads "-" in one column and "" in the next is a bug nobody can see in the
// source of either.
package render

import (
	"encoding/json"
	"io"
	"reflect"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// Changed reports one change to the scanner's memory.
//
// Under --output json it writes the mutation as an object, which is the whole
// point: these verbs used to write prose in both modes, so a script editing the
// scanner got machine-readable results from some commands and English sentences
// from others.
//
// The text line is passed in rather than built here, and that is deliberate.
// What these commands print is what people and scripts already read, down to
// the wording, so this adds the missing mode rather than changing the one that
// works.
//
// Parameters:
//   - app: the application context holding the output setting and the streams
//   - m: what changed
//   - text: the line to write when JSON was not asked for, without its newline
//
// Returns:
//   - error if the JSON cannot be written; nil for the text output, which
//     cannot fail
func Changed(app *appcontext.App, m Mutation, text string) error {
	if app.Config.Output == appcontext.OutputJSON {
		return JSON(app.Stdout, m)
	}

	app.Printf("%s\n", text)
	return nil
}

// Dash renders a value that may be missing, so that an empty column reads as a
// missing reading rather than as nothing at all.
//
// The scanner leaves plenty of fields blank, and a blank cell in an aligned
// table is indistinguishable from a column that ran out of rows. A dash is the
// convention every listing in this tool uses for "the scanner said nothing
// here".
//
// It is for text output only. JSON keeps the empty string, because a consumer
// deciding whether a value is present should not have to know that this tool
// spells absence "-".
//
// Parameters:
//   - value: the value as the scanner reports it, which may be empty
//
// Returns:
//   - value, or "-" when it is empty
func Dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// Filled says that some of a listing's entries came off the scanner's menus
// rather than out of its list, and that only their names are known.
//
// The note goes to stderr with the rest of the commentary, so a script reading
// stdout is unaffected. A script that wants to know reads the partial field,
// which is on the entries themselves.
//
// It is worth a whole sentence rather than a footnote on the table, because the
// alternative is what this tool used to do: report a short list as though it
// were the whole thing, and let somebody find out by creating forty channels and
// being shown seven.
//
// Parameters:
//   - app: the application context holding the output setting and the streams
//   - kind: what the entries are called, plural, such as "departments"
//   - n: how many of them are only partly known, ignored when it is zero
func Filled(app *appcontext.App, kind string, n int) {
	if n == 0 || app.Config.Output == appcontext.OutputJSON {
		return
	}

	app.Notef("\nThe scanner's own list stopped short of %d of these %s, which were read off\n"+
		"its menus instead: their names are right and the %s columns are unknown.\n",
		n, kind, Unread)
}

// JSON writes v as indented JSON followed by a newline, which is what every
// command's --output json path produces.
//
// Indented rather than compact, because the output is read by people at least
// as often as by programs, and anything piping it into jq or a decoder does not
// care either way. The trailing newline is the encoder's own and is what makes
// the output a well-formed line on a terminal.
//
// A listing that found nothing is written as [] rather than null. Go encodes a
// nil slice as null, and a command that found no systems was handing a script
// something with no length to take, no elements to range over, and a type that
// is not the array its documentation promises. Every listing here answers the
// same question, so all of them answer it in the same shape: an empty list of
// results is a result.
//
// Parameters:
//   - w: where the JSON goes, which is the caller's stdout
//   - v: the value to encode
//
// Returns:
//   - error if v cannot be encoded, or the stream cannot be written to
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(listable(v))
}

// listable turns a listing that found nothing into an empty list, so it
// encodes as [] instead of null.
//
// Only the value being encoded is looked at, not inside it. Every listing this
// tool writes is a slice at the top, which is the whole of the problem being
// solved; a nil slice buried in a struct field is a field the command chose to
// have, and quietly rewriting it would be this function deciding something it
// has no business deciding.
//
// Parameters:
//   - v: the value about to be encoded
//
// Returns:
//   - an empty slice of the same type when v is a nil slice, and v otherwise
func listable(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}

// YesNo renders a flag the way the listings write it.
//
// "yes" and "no" rather than "true" and "false", because these columns are read
// by a person and answer questions phrased as questions: is this department
// scanned, is this list built in, is the scanner locked.
//
// It is for text output only, for the same reason as Dash: JSON keeps the
// boolean.
//
// Parameters:
//   - b: the flag to render
//
// Returns:
//   - "yes" when b is true, and "no" when it is false
func YesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
