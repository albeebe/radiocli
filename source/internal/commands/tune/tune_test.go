// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package tune

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// stubConn is a scanner connection that answers from the test rather than from
// a serial port. Each exchange is dispatched on the command string, so a test
// says only what it cares about and lets everything else succeed silently.
type stubConn struct {
	exec    func(command string) (string, error)
	execXML func(command string) (string, error)
}

// Info reports which scanner is on the other end, which no test here reads.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute answers a plain command, defaulting to the empty reply that
// device.Scanner reads as success.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) {
	if c.exec == nil {
		return "", nil
	}
	return c.exec(command)
}

// ExecuteXML answers a command that replies with a document.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	if c.execXML == nil {
		return "", nil
	}
	return c.execXML(command)
}

// Send writes a command without waiting for a reply, which no test here uses.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close releases the connection, which costs nothing here.
func (c *stubConn) Close() error { return nil }

// quicken shortens the settle budget for the length of one test, and puts the
// real values back afterwards so the shortening cannot leak into another test
// or make the results depend on the order they run in.
func quicken(t *testing.T, polls int) {
	t.Helper()

	wasPolls, wasGap := settlePolls, settleGap
	t.Cleanup(func() { settlePolls, settleGap = wasPolls, wasGap })
	settlePolls, settleGap = polls, time.Millisecond
}

// newApp builds an App writing to buffers of the test's own, with a scanner
// that answers from conn.
func newApp(t *testing.T, conn device.Conn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	app := appcontext.New()
	var out, errs bytes.Buffer
	app.Stdout = &out
	app.Stderr = &errs
	app.SetDevice(device.New(conn))
	return app, &out, &errs
}

// gsi renders the document the scanner answers "GSI" with.
func gsi(mode, screen, rssi, signal string) string {
	return fmt.Sprintf(`<ScannerInfo Mode=%q V_Screen=%q><Property Rssi=%q Sig=%q/></ScannerInfo>`,
		mode, screen, rssi, signal)
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Shape: the command is bound with the name and argument count it declares
//   - ParseError: a frequency that is not a number is refused before the
//     scanner is opened
//   - Runs: a good frequency is tuned and reported
func TestNew(t *testing.T) {
	// Verify that the command is declared with one required argument.
	t.Run("Shape", func(t *testing.T) {
		app, _, _ := newApp(t, &stubConn{})

		cmd := New(app)
		if cmd.Use != "tune <frequency>" {
			t.Fatalf("Use = %q, want %q", cmd.Use, "tune <frequency>")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Fatal("the command is missing its help text")
		}
		if err := cmd.Args(cmd, []string{"1", "2"}); err == nil {
			t.Fatal("two arguments were accepted, want exactly one")
		}
	})

	// Verify that a frequency that is not a number is refused.
	t.Run("ParseError", func(t *testing.T) {
		app, _, _ := newApp(t, &stubConn{})

		cmd := New(app)
		cmd.SetArgs([]string{"nonsense"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid frequency") {
			t.Fatalf("error = %v, want it to mention an invalid frequency", err)
		}
	})

	// Verify that a good frequency is tuned and reported.
	t.Run("Runs", func(t *testing.T) {
		quicken(t, 2)

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Quick Search Hold", quickSearch, "-58", "3"), nil
			},
		}
		app, out, _ := newApp(t, conn)

		cmd := New(app)
		cmd.SetArgs([]string{"155.475"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "155.4750 MHz") {
			t.Fatalf("output = %q, want it to name the frequency", out.String())
		}
	})
}

// Test_coverage tests the coverage function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Spans: every band is rendered, separated by commas
func Test_coverage(t *testing.T) {
	// Verify that every band appears in the rendered coverage.
	t.Run("Spans", func(t *testing.T) {
		got := coverage()

		if strings.Count(got, " to ") != len(bands) {
			t.Fatalf("coverage() = %q, want %d spans", got, len(bands))
		}
		for _, b := range bands {
			if !strings.Contains(got, fmt.Sprintf("%s to %s", b.lo, b.hi)) {
				t.Fatalf("coverage() = %q, want it to hold %s to %s", got, b.lo, b.hi)
			}
		}
	})
}

// Test_covered tests the covered function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - InBand: a frequency the scanner receives is accepted
//   - Edge: a frequency on a band's own edge is accepted
//   - Cellular: a frequency in a blocked cellular band is named as blocked
//   - Outside: a frequency outside every band is answered with the coverage
func Test_covered(t *testing.T) {
	// Verify that a frequency inside a band is accepted.
	t.Run("InBand", func(t *testing.T) {
		if err := covered(155_475_000 * device.Hertz); err != nil {
			t.Fatalf("covered() = %v, want nil", err)
		}
	})

	// Verify that both edges of a band are included in it.
	t.Run("Edge", func(t *testing.T) {
		for _, f := range []device.Frequency{25 * device.Megahertz, 512 * device.Megahertz} {
			if err := covered(f); err != nil {
				t.Fatalf("covered(%s) = %v, want nil", f, err)
			}
		}
	})

	// Verify that a blocked cellular frequency is named as blocked.
	t.Run("Cellular", func(t *testing.T) {
		err := covered(830 * device.Megahertz)
		if err == nil || !strings.Contains(err.Error(), "cellular") {
			t.Fatalf("error = %v, want it to mention the cellular bands", err)
		}
	})

	// Verify that a frequency outside every band is answered with the coverage.
	t.Run("Outside", func(t *testing.T) {
		err := covered(1000 * device.Megahertz)
		if err == nil || !strings.Contains(err.Error(), "it covers") {
			t.Fatalf("error = %v, want it to report what the scanner covers", err)
		}
	})
}

// Test_listen tests the listen function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Receiving: the first reading carrying a signal is returned at once
//   - AskError: a failed exchange is reported
//   - Cancelled: a run cancelled part way through the wait is reported
//   - Quiet: a frequency that stays quiet returns the last reading and no error
func Test_listen(t *testing.T) {
	// Verify that a reading carrying a signal is returned without waiting.
	t.Run("Receiving", func(t *testing.T) {
		quicken(t, 20)

		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Quick Search Hold", quickSearch, "-52", "4"), nil
			},
		})

		got, err := listen(context.Background(), client)
		if err != nil {
			t.Fatalf("listen() = %v, want nil", err)
		}
		if got.Signal != "4" || got.RSSI != "-52" {
			t.Fatalf("reading = %+v, want the signal the scanner reported", got)
		}
	})

	// Verify that a failed exchange is reported.
	t.Run("AskError", func(t *testing.T) {
		quicken(t, 2)

		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				return "", errors.New("the port is gone")
			},
		})

		if _, err := listen(context.Background(), client); err == nil ||
			!strings.Contains(err.Error(), "the port is gone") {
			t.Fatalf("error = %v, want the failed exchange", err)
		}
	})

	// Verify that a cancelled run is reported rather than waited out.
	t.Run("Cancelled", func(t *testing.T) {
		quicken(t, 20)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				cancel()
				return gsi("Quick Search Hold", quickSearch, "-110", "0"), nil
			},
		})

		if _, err := listen(ctx, client); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want a cancelled context", err)
		}
	})

	// Verify that a quiet frequency is an answer rather than a failure.
	t.Run("Quiet", func(t *testing.T) {
		quicken(t, 2)

		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Quick Search Hold", quickSearch, "-110", "0"), nil
			},
		})

		got, err := listen(context.Background(), client)
		if err != nil {
			t.Fatalf("listen() = %v, want nil", err)
		}
		if got.Signal != "0" {
			t.Fatalf("reading = %+v, want the last one taken", got)
		}
	})
}

// Test_parse tests the parse function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Accepted: every unit, and the bare number that means megahertz
//   - Refused: a value that is not a number, is not positive, or is finer than
//     the scanner tunes
func Test_parse(t *testing.T) {
	// Verify that every accepted spelling reads as the same frequency.
	t.Run("Accepted", func(t *testing.T) {
		for _, tc := range []struct {
			arg  string
			want device.Frequency
		}{
			{"155.475", 155_475_000 * device.Hertz},
			{"155.475MHz", 155_475_000 * device.Hertz},
			{"155475kHz", 155_475_000 * device.Hertz},
			{"155475000Hz", 155_475_000 * device.Hertz},
			{"  155.475  ", 155_475_000 * device.Hertz},
			{"155.475 mhz", 155_475_000 * device.Hertz},
		} {
			got, err := parse(tc.arg)
			if err != nil {
				t.Fatalf("parse(%q) = %v, want nil", tc.arg, err)
			}
			if got != tc.want {
				t.Fatalf("parse(%q) = %s, want %s", tc.arg, got, tc.want)
			}
		}
	})

	// Verify that a value the scanner cannot be given is refused.
	t.Run("Refused", func(t *testing.T) {
		for _, tc := range []struct {
			arg  string
			want string
		}{
			{"nonsense", "want a number in megahertz"},
			{"", "want a number in megahertz"},
			{"0", "greater than zero"},
			{"-155.475", "greater than zero"},
			{"0.0000001hz", "smaller than the scanner can tune"},
		} {
			_, err := parse(tc.arg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parse(%q) = %v, want it to mention %q", tc.arg, err, tc.want)
			}
		}
	})
}

// Test_receiving tests the receiving function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Readings: bars above zero are the only reading that counts as receiving
func Test_receiving(t *testing.T) {
	// Verify that only a positive number of bars counts as receiving.
	t.Run("Readings", func(t *testing.T) {
		for _, tc := range []struct {
			signal string
			want   bool
		}{
			{"3", true},
			{" 2 ", true},
			{"0", false},
			{"", false},
			{"nonsense", false},
		} {
			if got := receiving(device.Property{Signal: tc.signal}); got != tc.want {
				t.Fatalf("receiving(%q) = %v, want %v", tc.signal, got, tc.want)
			}
		}
	})
}

// Test_refused tests the refused function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - AskError: a scanner that will not say what it is doing gets the general
//     explanation
//   - Menu: a scanner on a screen of its own is told to be returned to scanning
//   - Span: a scanner that is not busy is answered with the span it covers
//   - NoSpan: a scanner reporting no search range is answered without one
func Test_refused(t *testing.T) {
	// Verify that a scanner that will not answer gets the general explanation.
	t.Run("AskError", func(t *testing.T) {
		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				return "", errors.New("the port is gone")
			},
		})

		err := refused(context.Background(), client, 155_475_000*device.Hertz)
		if err == nil || !strings.Contains(err.Error(), "gave no reason") {
			t.Fatalf("error = %v, want the general explanation", err)
		}
	})

	// Verify that a scanner on a screen of its own is named and the fix given.
	t.Run("Menu", func(t *testing.T) {
		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Menu tree", "menu_selection", "", ""), nil
			},
		})

		err := refused(context.Background(), client, 155_475_000*device.Hertz)
		if err == nil || !strings.Contains(err.Error(), `"menu_selection" screen`) {
			t.Fatalf("error = %v, want it to name the screen", err)
		}
	})

	// Verify that a scanner which is not busy is answered with its span.
	t.Run("Span", func(t *testing.T) {
		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				return `<ScannerInfo Mode="Quick Search Hold" V_Screen="quick_search">` +
					`<SearchRange Lower=" 025.0000MHz" Upper="1300.0000MHz"/>` +
					`</ScannerInfo>`, nil
			},
		})

		err := refused(context.Background(), client, 155_475_000*device.Hertz)
		if err == nil || !strings.Contains(err.Error(), "It covers 025.0000MHz to 1300.0000MHz.") {
			t.Fatalf("error = %v, want it to report the span", err)
		}
	})

	// Verify that a scanner reporting no search range is answered without one.
	t.Run("NoSpan", func(t *testing.T) {
		client := device.New(&stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Scan Mode", "conventional_scan", "", ""), nil
			},
		})

		err := refused(context.Background(), client, 155_475_000*device.Hertz)
		if err == nil || !strings.Contains(err.Error(), "not busy with anything else") {
			t.Fatalf("error = %v, want the not busy explanation", err)
		}
		if strings.Contains(err.Error(), "It covers") {
			t.Fatalf("error = %v, want no span when the scanner reports none", err)
		}
	})
}

// Test_round tests the round function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Steps: a frequency is taken to the nearest step, up or down
func Test_round(t *testing.T) {
	// Verify that a frequency lands on the nearest step the protocol carries.
	t.Run("Steps", func(t *testing.T) {
		for _, tc := range []struct {
			f, want device.Frequency
		}{
			{155_475_000, 155_475_000},
			{155_475_555, 155_475_600},
			{155_475_549, 155_475_500},
			{149, 100},
			{0, 0},
		} {
			if got := round(tc.f); got != tc.want {
				t.Fatalf("round(%d) = %d, want %d", tc.f, got, tc.want)
			}
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - DeviceError: no scanner to talk to is reported
//   - NotCovered: a frequency the scanner cannot receive is refused
//   - Rounded: a frequency finer than the scanner tunes is noted
//   - Refused: a rejected tune is explained
//   - TuneError: any other failed tune is reported
//   - ListenError: a scanner that tuned but will not report is a note
//   - JSON: the report is written as JSON when that was asked for
//   - Quiet: a reading with no strength figure leaves the signal line out
func Test_run(t *testing.T) {
	// Verify that having no scanner to talk to is reported.
	t.Run("DeviceError", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if err := run(context.Background(), app, "155.475", 155_475_000*device.Hertz); err == nil {
			t.Fatal("run() = nil, want a missing scanner")
		}
	})

	// Verify that a frequency the scanner cannot receive is refused.
	t.Run("NotCovered", func(t *testing.T) {
		app, _, _ := newApp(t, &stubConn{})

		err := run(context.Background(), app, "1000", 1000*device.Megahertz)
		if err == nil || !strings.Contains(err.Error(), "cannot receive") {
			t.Fatalf("error = %v, want the frequency to be refused", err)
		}
	})

	// Verify that a frequency finer than the scanner tunes is noted.
	t.Run("Rounded", func(t *testing.T) {
		quicken(t, 2)

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Quick Search Hold", quickSearch, "-58", "3"), nil
			},
		}
		app, _, errs := newApp(t, conn)

		if err := run(context.Background(), app, "155.475555", 155_475_555*device.Hertz); err != nil {
			t.Fatalf("run() = %v, want nil", err)
		}
		if !strings.Contains(errs.String(), "rounds to") {
			t.Fatalf("notes = %q, want the rounding note", errs.String())
		}
	})

	// Verify that a rejected tune is explained rather than passed on bare.
	t.Run("Refused", func(t *testing.T) {
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if strings.HasPrefix(command, "QSH,") {
					return "", device.ErrRejected
				}
				return "", nil
			},
			execXML: func(command string) (string, error) {
				return gsi("Menu tree", "menu_selection", "", ""), nil
			},
		}
		app, _, _ := newApp(t, conn)

		err := run(context.Background(), app, "155.475", 155_475_000*device.Hertz)
		if err == nil || !strings.Contains(err.Error(), `"menu_selection" screen`) {
			t.Fatalf("error = %v, want the rejection explained", err)
		}
	})

	// Verify that any other failed tune is reported as it happened.
	t.Run("TuneError", func(t *testing.T) {
		conn := &stubConn{
			exec: func(command string) (string, error) {
				return "", errors.New("the port is gone")
			},
		}
		app, _, _ := newApp(t, conn)

		err := run(context.Background(), app, "155.475", 155_475_000*device.Hertz)
		if err == nil || !strings.Contains(err.Error(), "tuning to 155.4750 MHz") {
			t.Fatalf("error = %v, want the failed tune", err)
		}
	})

	// Verify that a scanner which tuned but will not report is a note.
	t.Run("ListenError", func(t *testing.T) {
		quicken(t, 2)

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return "", errors.New("the port is gone")
			},
		}
		app, out, errs := newApp(t, conn)

		if err := run(context.Background(), app, "155.475", 155_475_000*device.Hertz); err != nil {
			t.Fatalf("run() = %v, want nil", err)
		}
		if !strings.Contains(errs.String(), "would not report what it is hearing") {
			t.Fatalf("notes = %q, want the note about the reading", errs.String())
		}
		if !strings.Contains(out.String(), "receiving: no") {
			t.Fatalf("output = %q, want the frequency reported anyway", out.String())
		}
	})

	// Verify that the report is written as JSON when that was asked for.
	t.Run("JSON", func(t *testing.T) {
		quicken(t, 2)

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Quick Search Hold", quickSearch, "-58", "3"), nil
			},
		}
		app, out, _ := newApp(t, conn)
		app.Config.Output = appcontext.OutputJSON

		if err := run(context.Background(), app, "155.475", 155_475_000*device.Hertz); err != nil {
			t.Fatalf("run() = %v, want nil", err)
		}
		if !strings.Contains(out.String(), `"receiving": true`) {
			t.Fatalf("output = %q, want the JSON report", out.String())
		}
	})

	// Verify that a reading with no strength figure leaves the signal line out.
	t.Run("Quiet", func(t *testing.T) {
		quicken(t, 1)

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return gsi("Quick Search Hold", quickSearch, "", ""), nil
			},
		}
		app, out, _ := newApp(t, conn)

		if err := run(context.Background(), app, "155.475", 155_475_000*device.Hertz); err != nil {
			t.Fatalf("run() = %v, want nil", err)
		}
		if strings.Contains(out.String(), "signal:") {
			t.Fatalf("output = %q, want no signal line when there is no reading", out.String())
		}
	})
}

// Test_bands tests the bands table.
//
// Coverage: not a function; the table is checked for the shape the code that
// walks it depends on
//
// Test cases:
//   - Ordered: every span runs low to high, and the spans do not overlap
func Test_bands(t *testing.T) {
	// Verify that every span runs low to high, in ascending order.
	t.Run("Ordered", func(t *testing.T) {
		if len(bands) == 0 {
			t.Fatal("bands is empty, want the spans the scanner covers")
		}
		for i, b := range bands {
			if b.lo >= b.hi {
				t.Fatalf("bands[%d] = %s to %s, want the low end first", i, b.lo, b.hi)
			}
			if i > 0 && b.lo <= bands[i-1].hi {
				t.Fatalf("bands[%d] starts at %s, which overlaps the span before it", i, b.lo)
			}
		}
	})
}

// Test_cellular tests the cellular table.
//
// Coverage: not a function; the table is checked for the shape the code that
// walks it depends on
//
// Test cases:
//   - Blocked: both blocked spans sit in a gap between two covered bands
func Test_cellular(t *testing.T) {
	// Verify that both blocked spans fall outside every band the scanner covers.
	t.Run("Blocked", func(t *testing.T) {
		if len(cellular) != 2 {
			t.Fatalf("cellular holds %d spans, want 2", len(cellular))
		}
		for i, c := range cellular {
			if c.lo >= c.hi {
				t.Fatalf("cellular[%d] = %s to %s, want the low end first", i, c.lo, c.hi)
			}
			for _, b := range bands {
				if c.lo <= b.hi && b.lo <= c.hi {
					t.Fatalf("cellular[%d] overlaps the covered span %s to %s", i, b.lo, b.hi)
				}
			}
		}
	})
}
