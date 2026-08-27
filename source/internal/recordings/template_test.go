// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/25/2026

package recordings

import (
	"strings"
	"testing"
)

// TestEveryTokenRenders covers each token once, since a token that is listed
// but produces nothing is worse than one that does not exist.
func TestEveryTokenRenders(t *testing.T) {
	e := entry()
	e.Site = "Bald Knob"
	e.Talkgroup = "24944"
	e.Unit = "32"
	e.Duration = 4.8

	for _, name := range Tokens() {
		tmpl, err := parse("{" + name + "}")
		if err != nil {
			t.Fatalf("the listed token %q does not parse: %v", name, err)
		}
		if got := tmpl.render(e); got == "" {
			t.Errorf("the token %q rendered nothing", name)
		}
	}

	// tuned prefers the talkgroup, since only one of the two is ever set.
	tmpl, _ := parse("{tuned}")
	if got := tmpl.render(e); got != "24944" {
		t.Errorf("tuned rendered %q, want the talkgroup", got)
	}
	e.Talkgroup = ""
	if got := tmpl.render(e); got != "155.550000MHz" {
		t.Errorf("tuned rendered %q, want the frequency once there is no talkgroup", got)
	}
}

// TestLiteralBraces covers the escape, which a template language made entirely
// of braces has to have.
func TestLiteralBraces(t *testing.T) {
	tmpl, err := parse("{{{channel}}}")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	// Literal text is passed through as written, because the user typed it
	// deliberately. Only values coming off the scanner are sanitised.
	if got := tmpl.render(entry()); got != "{MARLINTON-DISPATCH}" {
		t.Errorf("rendered %q, want the braces kept as text", got)
	}
}

// TestShortenGivesUpRatherThanErasing covers a template with more components
// than the limit can hold, where there is nothing left to take.
func TestShortenGivesUpRatherThanErasing(t *testing.T) {
	parts := make([]string, 40)
	for i := range parts {
		parts[i] = strings.Repeat("x", 20)
	}

	got := shorten(parts)
	if len(got) <= maxPath {
		t.Fatalf("the path is %d long, so this test is not reaching the giving up", len(got))
	}
	for _, c := range strings.Split(got, "/") {
		if len(c) < minComponent {
			t.Errorf("the component %q was cut to %d, want no shorter than %d", c, len(c), minComponent)
		}
	}
}

// TestTunedFallsBackToNothing covers a recording with neither a talkgroup nor a
// frequency, which is what an unlabelled one is.
func TestTunedFallsBackToNothing(t *testing.T) {
	tmpl, err := parse("{time}{tuned}")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := tmpl.render(Entry{Start: at}); got != at.Format("15-04-05") {
		t.Errorf("rendered %q, want the empty token to leave nothing behind", got)
	}
}
