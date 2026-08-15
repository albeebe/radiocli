// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package cmdline

import (
	"strings"
	"testing"
)

// TestSplit covers the shapes a command line actually arrives in. The quoting
// cases are the ones that matter: the scanner's own names contain spaces and
// commas, so a splitter that got them wrong would make whole lists unreachable
// from a front end and unstorable in a macro.
func TestSplit(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"nothing at all", "", nil},
		{"spaces only", "   \t  ", nil},
		{"one word", "battery", []string{"battery"}},
		{"several words", "volume set 4", []string{"volume", "set", "4"}},
		{"runs of spaces collapse", "  volume   set    4  ", []string{"volume", "set", "4"}},
		{"tabs and newlines separate too", "volume\tset\n4", []string{"volume", "set", "4"}},
		{
			"a double quoted name holds together",
			`favorites scan "GREENDALE, ST 00000"`,
			[]string{"favorites", "scan", "GREENDALE, ST 00000"},
		},
		{
			"a single quoted name holds together",
			`favorites scan 'GREENDALE, ST 00000'`,
			[]string{"favorites", "scan", "GREENDALE, ST 00000"},
		},
		{"quotes join to the word beside them", `say"one word"`, []string{"sayone word"}},
		{"an empty argument is an argument", `menu set ""`, []string{"menu", "set", ""}},
		{"an escaped space is not a separator", `menu set one\ two`, []string{"menu", "set", "one two"}},
		{"an escaped quote is a quote", `menu set \"`, []string{"menu", "set", `"`}},
		{
			"a quote can be escaped inside double quotes",
			`menu set "he said \"go\""`,
			[]string{"menu", "set", `he said "go"`},
		},
		{
			"a backslash can be escaped inside double quotes",
			`menu set "one\\two"`,
			[]string{"menu", "set", `one\two`},
		},
		{
			// A Windows path is the reason every other backslash inside double
			// quotes is left alone rather than treated as an escape.
			"a windows path survives being typed",
			`--config "C:\Users\example\config.json"`,
			[]string{"--config", `C:\Users\example\config.json`},
		},
		{"single quotes take backslashes literally", `menu set 'one\two'`, []string{"menu", "set", `one\two`}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Split(c.line)
			if err != nil {
				t.Fatalf("splitting %q failed: %v", c.line, err)
			}
			if !same(got, c.want) {
				t.Errorf("splitting %q gave %q, want %q", c.line, got, c.want)
			}
		})
	}
}

// TestSplitRefusals checks the lines that cannot be split, and that each says
// why in terms somebody can act on. A silent guess at what was meant is the one
// answer that would be worse than refusing.
func TestSplitRefusals(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"unclosed double quote", `favorites scan "GREENDALE`, `unclosed " quote`},
		{"unclosed single quote", `favorites scan 'GREENDALE`, "unclosed ' quote"},
		{"trailing backslash", `menu set one\`, "trailing backslash"},
		{"a pipe", "battery | grep charge", `"|" is not supported`},
		{"a redirect", "battery > out.txt", `">" is not supported`},
		{"an input redirect", "battery < in.txt", `"<" is not supported`},
		{"a semicolon", "battery; volume", `";" is not supported`},
		{"an ampersand", "battery & volume", `"&" is not supported`},
		{"a subshell open", "battery (volume)", `"(" is not supported`},
		{"a subshell close", "battery )", `")" is not supported`},
		{"a variable", "volume set $LEVEL", `"$" is not supported`},
		{"a backtick", "volume set `level`", "\"`\" is not supported"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Split(c.line)
			if err == nil {
				t.Fatalf("splitting %q gave %q, want a refusal", c.line, got)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("splitting %q said %q, which does not mention %q", c.line, err, c.want)
			}
		})
	}
}

// TestSplitQuotesHideMetacharacters checks that the shell operators are refused
// as operators rather than as characters. A list really can be named "R&D", and
// quoting is how somebody says they meant the character.
func TestSplitQuotesHideMetacharacters(t *testing.T) {
	got, err := Split(`favorites goto "R&D"`)
	if err != nil {
		t.Fatalf("splitting a quoted ampersand failed: %v", err)
	}
	if !same(got, []string{"favorites", "goto", "R&D"}) {
		t.Errorf("a quoted ampersand split into %q, want the name intact", got)
	}
}

// same reports whether two argument lists hold the same words. Empty and nil
// count as the same: both mean no arguments.
func same(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
