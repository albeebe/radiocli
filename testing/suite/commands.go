// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/15/2026

// Package suite is the end to end test suite for radiocli, run against a real
// scanner attached to this computer.
//
// Almost all of it is test files. This one is not, and that is deliberate: the
// checklist of commands below is needed by the runner in the directory above as
// well as by the tests, and a test file cannot be imported. The runner reads it
// to find where a command name ends and a test's own name begins, which is what
// lets a test function called TestChannelsNew be drawn under "channels new"
// rather than guessed at.
package suite

import "strings"

// All is every command the tool offers, as it would be typed, without the
// "radiocli" that starts the line. Sorted, and one entry per command including
// the parents that only hold subcommands.
//
// TestRadiocli_CommandChecklist is what makes this true. The list drifted once
// already, silently, because nothing compared it against the tool: it was
// missing six whole subtrees that had test files of their own, so the checklist
// was staler than the coverage it was meant to measure.
var All = [][]string{
	{"audio"},
	{"audio", "listen"},
	{"audio", "output"},
	{"audio", "record"},
	{"backlight"},
	{"backlight", "keys"},
	{"backlight", "keys", "disable"},
	{"backlight", "keys", "enable"},
	{"backlight", "keys", "toggle"},
	{"backlight", "off"},
	{"backlight", "on"},
	{"backup"},
	{"banks"},
	{"banks", "goto"},
	{"banks", "scan"},
	{"banks", "set"},
	{"battery"},
	{"beep"},
	{"beep", "set"},
	{"beep", "toggle"},
	{"channels"},
	{"channels", "delete"},
	{"channels", "new"},
	{"channels", "rename"},
	{"clock"},
	{"clock", "set"},
	{"clock", "sync"},
	{"colors"},
	{"colors", "palette"},
	{"colors", "reset"},
	{"colors", "set"},
	{"config"},
	{"config", "get"},
	{"config", "macro"},
	{"config", "macro", "delete"},
	{"config", "macro", "move"},
	{"config", "macro", "new"},
	{"config", "macro", "rename"},
	{"config", "macro", "set"},
	{"config", "macro", "show"},
	{"config", "path"},
	{"config", "set"},
	{"config", "unset"},
	{"daemon"},
	{"departments"},
	{"departments", "delete"},
	{"departments", "goto"},
	{"departments", "new"},
	{"departments", "rename"},
	{"devices"},
	{"display"},
	{"display", "mode"},
	{"favorites"},
	{"favorites", "delete"},
	{"favorites", "goto"},
	{"favorites", "new"},
	{"favorites", "rename"},
	{"favorites", "scan"},
	{"headphone"},
	{"headphone", "set"},
	{"key"},
	{"location"},
	{"location", "gps"},
	{"location", "set"},
	{"menu"},
	{"menu", "back"},
	{"menu", "close"},
	{"menu", "open"},
	{"menu", "set"},
	{"receiving"},
	{"scan"},
	{"scanning"},
	{"scanning", "systems"},
	{"screen"},
	{"sites"},
	{"sites", "delete"},
	{"sites", "frequencies"},
	{"sites", "frequencies", "add"},
	{"sites", "frequencies", "delete"},
	{"sites", "new"},
	{"sites", "rename"},
	{"squelch"},
	{"squelch", "set"},
	{"status"},
	{"systems"},
	{"systems", "delete"},
	{"systems", "goto"},
	{"systems", "new"},
	{"systems", "rename"},
	{"tune"},
	{"update"},
	{"version"},
	{"volume"},
	{"volume", "set"},
	{"weather"},
	{"weather", "stop"},
}

// index is All, joined and keyed for lookup, built once.
var index = func() map[string]bool {
	m := make(map[string]bool, len(All))
	for _, c := range All {
		m[strings.Join(c, " ")] = true
	}
	return m
}()

// Split reads a test function's name as the command it tests.
//
// The suite is written to one convention: a test function is named for the
// command path it covers, with an optional "_Variant" when one command needs
// more than one function. So TestChannelsNew is the "channels new" command,
// TestChannelsNew_SimilarNames is that command again, and TestRadiocli_Help is
// the root command itself, which is where the tests that belong to no one
// command live.
//
// The command is looked up rather than guessed, because nothing in the name
// says where the command stops: "channels new" is two words and
// "backlight keys enable" is three, and only this list knows which.
func Split(function string) (path []string, variant string, ok bool) {
	base := strings.TrimPrefix(function, "Test")
	command, variant, _ := strings.Cut(base, "_")

	if strings.EqualFold(command, "radiocli") {
		return nil, variant, true
	}

	path = strings.Fields(strings.ToLower(Words(command)))
	if !index[strings.Join(path, " ")] {
		return nil, variant, false
	}
	return path, variant, true
}

// Words splits a name written in Go's style into the words it is made of.
//
// A run of capitals is one word, so "LocationGPS" reads as "Location GPS"
// rather than as "Location G P S".
func Words(name string) string {
	runes := []rune(name)

	var out strings.Builder
	for i, r := range runes {
		starts := i > 0 && upper(r) &&
			(!upper(runes[i-1]) || (i+1 < len(runes) && lower(runes[i+1])))
		if starts {
			out.WriteByte(' ')
		}
		out.WriteRune(r)
	}
	return out.String()
}

// upper and lower report the case of a letter, for splitting names.
func upper(r rune) bool { return r >= 'A' && r <= 'Z' }
func lower(r rune) bool { return r >= 'a' && r <= 'z' }
