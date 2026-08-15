// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package colors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// Test_renderAll tests the renderAll function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Table: a line per layout rather than every area of every one of them
//   - Current: the layout on screen is marked, and the rest are dashed
//   - JSON: the JSON form is the whole of every reading
//   - WriteError: a stream that refuses the table is reported
func Test_renderAll(t *testing.T) {
	reports := []report{
		{Layout: "weather", Menu: "Set Weather Mode", Current: true,
			Areas: []area{{Name: "Func"}, {Name: "Battery"}}},
		{Layout: "tone-out", Menu: "Set Tone Out Mode", Areas: []area{{Name: "Func"}}},
	}

	// Verify that each layout gets one line, since seven tables is not what
	// somebody who just waited three minutes wants
	t.Run("Table", func(t *testing.T) {
		app, out, _ := newApp()
		if err := renderAll(app, reports); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "LAYOUT") || !strings.Contains(got, "2 layouts read") {
			t.Errorf("the table came back as:\n%s", got)
		}
		if strings.Contains(got, "Battery") {
			t.Errorf("the areas were listed as well:\n%s", got)
		}
	})

	// Verify that the layout on screen is marked and the rest are dashed
	t.Run("Current", func(t *testing.T) {
		app, out, _ := newApp()
		if err := renderAll(app, reports); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		for _, row := range strings.Split(out.String(), "\n") {
			switch {
			case strings.HasPrefix(row, "weather") && !strings.HasSuffix(row, "yes"):
				t.Errorf("the layout on screen is not marked: %q", row)
			case strings.HasPrefix(row, "tone-out") && !strings.HasSuffix(row, "-"):
				t.Errorf("a layout that is not on screen is marked: %q", row)
			}
		}
	})

	// Verify that the JSON form is exactly what one layout renders as, seven
	// times over
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON

		if err := renderAll(app, reports); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		var got []report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the JSON does not parse: %v", err)
		}
		if len(got) != 2 || len(got[0].Areas) != 2 {
			t.Errorf("the JSON came back as %+v", got)
		}
	})

	// Verify that a stream which refuses the table is reported
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		if err := renderAll(app, reports); err == nil {
			t.Error("a refused stream was reported as written")
		}
	})
}

// Test_runAll tests the runAll function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NoDevice: a run with no scanner named is reported
//   - Reads: every layout is read, cached and reported
//   - NoCurrent: a scanner in a menu still gets every layout read
//   - LayoutError: a walk that fails names the layout it was on
//   - PlaceError: a live screen that cannot be read is reported
func Test_runAll(t *testing.T) {
	// Verify that a run with no scanner named is reported
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := newApp()
		if err := runAll(context.Background(), app); err == nil {
			t.Error("a run with no scanner reported success")
		}
	})

	// Verify that every layout is read, cached and reported, and that the one
	// on screen is marked
	t.Run("Reads", func(t *testing.T) {
		isolate(t)

		app, out, notes := newApp()
		use(app, newRadio("wx_alert", 14, 2))

		if err := runAll(context.Background(), app); err != nil {
			t.Fatalf("reading every layout: %v", err)
		}
		if !strings.Contains(out.String(), "7 layouts read") {
			t.Errorf("the run came back as:\n%s", out)
		}
		if !strings.Contains(notes.String(), "[7/7]") {
			t.Errorf("the run was not counted:\n%s", notes)
		}

		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading the cache: %v", err)
		}
		for _, l := range layouts {
			if _, ok := c.lookup(scannerKey(fakeConn{}.Info()), l.name); !ok {
				t.Errorf("%s was not cached", l.name)
			}
		}
	})

	// Verify that a scanner with no current layout is not a failure here,
	// since every layout is read either way
	t.Run("NoCurrent", func(t *testing.T) {
		isolate(t)

		app, out, _ := newApp()
		use(app, newRadio("menu_selection", 14, 2))

		if err := runAll(context.Background(), app); err != nil {
			t.Fatalf("reading every layout: %v", err)
		}
		if strings.Contains(out.String(), "yes") {
			t.Errorf("a layout was called current with nothing on screen:\n%s", out)
		}
	})

	// Verify that a walk which fails names the layout it was reading
	t.Run("LayoutError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.customizeMenu.opens["Set Simple Trunk"].bare = true
		use(app, r)

		err := runAll(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "reading Set Simple Trunk") {
			t.Errorf("a walk that failed came back as %v", err)
		}
	})

	// Verify that a live screen which cannot be read is reported
	t.Run("PlaceError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.fails = func(command string) error {
			// Only once the walk is out of the menus, which is where the soft
			// keys of the layout on screen are read from.
			if command == "STS" && len(r.stack) == 0 {
				return errors.New("the port is gone")
			}
			return nil
		}
		use(app, r)

		err := runAll(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Errorf("an unreadable live screen came back as %v", err)
		}
	})

	// Verify that a scanner which will not leave its menus is reported, since
	// the soft keys cannot be read while it is still in one
	t.Run("LeaveError", func(t *testing.T) {
		isolate(t)

		app, _, _ := newApp()
		r := newRadio("wx_alert", 14, 2)
		r.failOn("KEY,L,P", errors.New("the scanner refused the key"))
		use(app, r)

		err := runAll(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "leaving the menus") {
			t.Errorf("a scanner stuck in its menus came back as %v", err)
		}
	})
}
