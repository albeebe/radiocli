// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package colors

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// TestNewPalette tests the newPalette function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is named and carries its help text
//   - Runs: executing the command lists every color
func TestNewPalette(t *testing.T) {
	// Verify that the command carries its name and its help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newPalette(appcontext.New())

		if cmd.Name() != "palette" {
			t.Errorf("the command is %q, wanted %q", cmd.Name(), "palette")
		}
		if cmd.Short == "" || !strings.Contains(cmd.Long, fmt.Sprint(len(palette))) {
			t.Errorf("the help text does not count the colors: %q", cmd.Long)
		}
	})

	// Verify that running the command lists every color, with no scanner
	// attached, since the palette is built in
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := newApp()
		cmd := newPalette(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs(nil)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("listing the palette: %v", err)
		}
		if !strings.Contains(out.String(), fmt.Sprintf("%d colors", len(palette))) {
			t.Errorf("the command wrote:\n%s", out)
		}
	})
}

// Test_runPalette tests the runPalette function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Table: every color is listed with its step and its value
//   - JSON: the JSON form carries the whole ring in the knob's order
//   - WriteError: a stream that refuses the table is reported
func Test_runPalette(t *testing.T) {
	// Verify that every color is listed with the step that reaches it
	t.Run("Table", func(t *testing.T) {
		app, out, _ := newApp()
		if err := runPalette(app); err != nil {
			t.Fatalf("listing the palette: %v", err)
		}

		got := out.String()
		if strings.Count(got, "\n") != len(palette)+3 {
			t.Errorf("the table has %d lines:\n%s", strings.Count(got, "\n"), got)
		}
		for _, want := range []string{"STEP", "Aliceblue", "#EFF7FF", "Yellowgreen"} {
			if !strings.Contains(got, want) {
				t.Errorf("the table does not carry %q", want)
			}
		}
	})

	// Verify that the JSON form carries the whole ring in the knob's order
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := runPalette(app); err != nil {
			t.Fatalf("listing the palette: %v", err)
		}

		var got paletteList
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the JSON does not parse: %v", err)
		}
		if got.Count != len(palette) || len(got.Colors) != len(palette) {
			t.Fatalf("the JSON holds %d of %d colors", len(got.Colors), len(palette))
		}
		if got.Colors[0].Step != 0 || got.Colors[0].Name != palette[0].Name {
			t.Errorf("the first color is %+v", got.Colors[0])
		}
		if got.Colors[len(got.Colors)-1].Step != len(palette)-1 {
			t.Errorf("the last color is %+v", got.Colors[len(got.Colors)-1])
		}
	})

	// Verify that a stream which refuses the table is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		if err := runPalette(app); err == nil {
			t.Error("a refused stream was reported as written")
		}

		app.Config.Output = appcontext.OutputJSON
		if err := runPalette(app); err == nil {
			t.Error("a refused stream was reported as encoded")
		}
	})
}
