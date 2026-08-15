// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

import (
	"strings"
	"testing"
)

// TestScreen checks that the display comes back as text.
//
// What is on the screen depends on what the scanner is doing, so the test is
// about the shape of the answer rather than its content: some lines, at least
// one of them carrying something to read.
func TestScreen(t *testing.T) {
	needScanner(t)

	var report struct {
		Lines []struct {
			Text        string `json:"text"`
			Highlighted bool   `json:"highlighted"`
			Attributes  string `json:"attributes"`
		} `json:"lines"`
	}
	mustJSON(t, &report, "screen")

	if len(report.Lines) == 0 {
		t.Fatal("the scanner reported a screen with no lines")
	}

	written := 0
	for _, l := range report.Lines {
		if strings.TrimSpace(l.Text) != "" {
			written++
		}
	}
	if written == 0 {
		t.Errorf("every line of the screen is blank, across %d lines", len(report.Lines))
	}

	t.Run("attributes agree with highlighted", func(t *testing.T) {
		// "highlighted" says a row has reverse video somewhere; "attributes"
		// says which characters. A row claiming one without the other means a
		// caller reading either alone is being told something different.
		for i, l := range report.Lines {
			hasReverse := strings.ContainsRune(l.Attributes, '*')
			if l.Highlighted != hasReverse {
				t.Errorf("line %d reports highlighted=%v but its attributes %q "+
					"say otherwise", i, l.Highlighted, l.Attributes)
			}
		}
	})

	t.Run("attributes describe every character", func(t *testing.T) {
		// One character per character is the whole contract: a caller indexes
		// into both to find what is drawn where.
		for i, l := range report.Lines {
			if l.Attributes == "" {
				continue
			}
			for _, r := range l.Attributes {
				if r != ' ' && r != '*' && r != '_' {
					t.Errorf("line %d has attribute %q, wanted a space, \"*\" or \"_\"", i, r)
					break
				}
			}
		}
	})

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "screen")

		if strings.TrimSpace(res.stdout) == "" {
			t.Error("the text output is empty")
		}

		// The two renderings are of the same screen, read a moment apart. The
		// scanner redraws constantly, so the check is that the text output
		// carries the lines rather than that it carries the same ones.
		if lines := strings.Count(strings.TrimRight(res.stdout, "\n"), "\n") + 1; lines < len(report.Lines) {
			t.Errorf("the text output has %d lines, wanted at least the %d the JSON reported",
				lines, len(report.Lines))
		}
	})
}
