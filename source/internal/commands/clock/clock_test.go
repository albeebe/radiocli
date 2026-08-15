// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package clock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/spf13/cobra"
)

// clockConn is a device.Conn that answers the clock commands with canned
// replies, so the commands can be driven with no scanner attached.
//
// The write and the two reads are answered separately, because apply reads the
// scanner, writes it, and then reads it back, and each of those three has to be
// able to fail on its own.
type clockConn struct {
	// reads are the answers to the DTM query, taken in order. The last one is
	// repeated once they run out.
	reads []string

	// readErrs are the errors to report for the DTM query, taken in order
	// alongside reads. A missing or nil entry means that read succeeds.
	readErrs []error

	// setReply and setErr are what the DTM write answers with. An empty reply
	// with no error is what the scanner sends when it accepts a command.
	setReply string
	setErr   error

	// n counts the DTM queries, so that a read back can differ from the read
	// that came before the write.
	n int
}

// Info describes the connected scanner.
func (c *clockConn) Info() device.Info { return device.Info{} }

// Execute answers the clock write and the clock query separately.
func (c *clockConn) Execute(ctx context.Context, command string) (string, error) {
	if strings.HasPrefix(command, "DTM,") {
		return c.setReply, c.setErr
	}

	i := c.n
	c.n++
	if i >= len(c.reads) {
		i = len(c.reads) - 1
	}

	var err error
	if i < len(c.readErrs) {
		err = c.readErrs[i]
	}
	return c.reads[i], err
}

// ExecuteXML answers as the plain query does, since nothing here reads XML.
func (c *clockConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return c.Execute(ctx, command)
}

// Send reports nothing, since nothing here writes without reading.
func (c *clockConn) Send(ctx context.Context, command string) error { return nil }

// Close releases nothing, because there is no port.
func (c *clockConn) Close() error { return nil }

// failWriter is an output stream that refuses every write, which is how the
// JSON encoder is made to fail.
type failWriter struct{}

// Write always reports the stream has gone away.
func (failWriter) Write(p []byte) (int, error) {
	return 0, errors.New("the output stream closed")
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and carries both subcommands
//   - Runs: executing the command reports the clock it was given
func TestNew(t *testing.T) {
	// Verify the command is named and carries the two subcommands that write
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "clock" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "clock")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}

		found := map[string]bool{}
		for _, sub := range cmd.Commands() {
			found[sub.Name()] = true
		}
		if !found["set"] || !found["sync"] {
			t.Errorf("the subcommands are %v, wanted set and sync", found)
		}
	})

	// Verify running the command reads the scanner and writes what it read
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,14,30,00,1"}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "date:     2026-08-02") {
			t.Errorf("the command wrote %q, wanted it to report the date", out.String())
		}
	})
}

// Test_newReport covers turning a reading into the rendered shape.
//
// Coverage: 100% (1 test case covering the single path)
//
// Test cases:
//   - Fields: the date and the time of day are split, and the flags carried
func Test_newReport(t *testing.T) {
	// Verify the reading is split into a date and a time of day, flags intact
	t.Run("Fields", func(t *testing.T) {
		got := newReport(time.Date(2026, 8, 2, 14, 30, 5, 0, time.Local), true, false)

		want := report{Date: "2026-08-02", Time: "14:30:05", DaylightSaving: true}
		if got != want {
			t.Errorf("the report is %+v, wanted %+v", got, want)
		}
	})
}

// Test_newSet covers the set subcommand, including the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Wiring: the subcommand is named and carries the daylight saving flag
//   - Runs: a written date and time is sent to the scanner
//   - BadValue: an unreadable date is refused before the scanner is opened
func Test_newSet(t *testing.T) {
	// Verify the subcommand is named and carries the flag both writers share
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSet(appcontext.New())

		if cmd.Name() != "set" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "set")
		}
		if cmd.Flags().Lookup("dst") == nil {
			t.Error("the subcommand has no --dst flag")
		}
	})

	// Verify a date and time given on the command line reaches the scanner
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,14,30,00,1"}}))

		cmd := newSet(app)
		cmd.SetArgs([]string{"2026-08-02 14:30:00"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "time:     14:30:00") {
			t.Errorf("the subcommand wrote %q, wanted it to report the time", out.String())
		}
	})

	// Verify an unreadable date is refused rather than sent to the scanner
	t.Run("BadValue", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		cmd := newSet(app)
		cmd.SetArgs([]string{"half past two"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("the subcommand reported nothing, wanted the unreadable date")
		}
		if !strings.Contains(err.Error(), "invalid date and time") {
			t.Errorf("the subcommand reported %q, wanted it to name the bad value", err)
		}
	})
}

// Test_newSync covers the sync subcommand, including the closure it hands
// cobra.
//
// Coverage: 100% (2 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the subcommand is named and carries the daylight saving flag
//   - Runs: this computer's clock is sent to the scanner
func Test_newSync(t *testing.T) {
	// Verify the subcommand is named and carries the flag both writers share
	t.Run("Wiring", func(t *testing.T) {
		cmd := newSync(appcontext.New())

		if cmd.Name() != "sync" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Name(), "sync")
		}
		if cmd.Flags().Lookup("dst") == nil {
			t.Error("the subcommand has no --dst flag")
		}
	})

	// Verify running it writes the scanner and reports what it holds after
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,14,30,00,1"}}))

		cmd := newSync(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(out.String(), "date:     2026-08-02") {
			t.Errorf("the subcommand wrote %q, wanted it to report the clock", out.String())
		}
	})
}

// Test_addDSTFlag covers registering the flag the two write commands share.
//
// Coverage: 100% (1 test case covering the single path)
//
// Test cases:
//   - Registers: the flag exists, defaults to false, and writes through
func Test_addDSTFlag(t *testing.T) {
	// Verify the flag is registered against the variable it was given
	t.Run("Registers", func(t *testing.T) {
		var dst bool
		cmd := &cobra.Command{Use: "example"}

		addDSTFlag(cmd, &dst)

		f := cmd.Flags().Lookup("dst")
		if f == nil {
			t.Fatal("the flag was not registered")
		}
		if f.DefValue != "false" {
			t.Errorf("the flag defaults to %q, wanted %q", f.DefValue, "false")
		}
		if err := cmd.Flags().Set("dst", "true"); err != nil {
			t.Fatalf("setting the flag: %v", err)
		}
		if !dst {
			t.Error("setting the flag did not reach the variable")
		}
	})
}

// Test_apply covers writing the clock and reporting what the scanner holds.
//
// Coverage: 100% (9 test cases covering every branch)
//
// Test cases:
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer the first read is reported
//   - Complete: both halves given, so nothing is taken from the scanner
//   - PartialKeepsTheRest: one half given, the other kept from the scanner
//   - PartialStoppedClock: one half given and a stopped clock to keep it from
//   - SetFails: a scanner that refuses the write is reported
//   - ReadBackFails: a scanner that will not answer the read back is reported
//   - RenderFails: an output stream that refuses the JSON is reported
//   - Mismatch: a scanner holding a different time than it was given says so
func Test_apply(t *testing.T) {
	// The value both halves of, which needs nothing from the scanner.
	whole := value{t: time.Date(2026, 8, 2, 14, 30, 0, 0, time.Local), hasDate: true, hasTime: true}

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := apply(context.Background(), app, whole, nil)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("apply reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that will not answer the first read is reported
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{
			reads:    []string{""},
			readErrs: []error{errors.New("the port closed")},
		}))

		err := apply(context.Background(), app, whole, nil)
		if err == nil || !strings.Contains(err.Error(), "reading the clock before changing it") {
			t.Fatalf("apply reported %v, wanted the failed first read", err)
		}
	})

	// Verify a value giving both halves is written without a mismatch note
	t.Run("Complete", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,14,30,00,1"}}))

		if err := apply(context.Background(), app, whole, nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !strings.Contains(out.String(), "time:     14:30:00") {
			t.Errorf("apply wrote %q, wanted it to report the time", out.String())
		}
		if strings.Contains(notes.String(), "not the") {
			t.Errorf("apply noted %q, wanted no mismatch", notes.String())
		}
	})

	// Verify a date alone keeps the time of day the scanner already holds
	t.Run("PartialKeepsTheRest", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,14,30,00,1"}}))

		only := value{t: time.Date(2026, 12, 25, 0, 0, 0, 0, time.Local), hasDate: true}
		if err := apply(context.Background(), app, only, nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !strings.Contains(out.String(), "time:     14:30:00") {
			t.Errorf("apply wrote %q, wanted the scanner's own time of day kept", out.String())
		}
	})

	// Verify a stopped clock is said to be unusable before this computer is used
	t.Run("PartialStoppedClock", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,14,30,00,0"}}))

		only := value{t: time.Date(0, 1, 1, 9, 15, 0, 0, time.Local), hasTime: true}
		if err := apply(context.Background(), app, only, nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !strings.Contains(notes.String(), "no date to keep") {
			t.Errorf("the note is %q, wanted it to say the date had to come from here", notes.String())
		}
	})

	// Verify a scanner that refuses the write is reported rather than trusted
	t.Run("SetFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{
			reads:    []string{"0,2026,08,02,14,30,00,1"},
			setReply: "NG",
		}))

		err := apply(context.Background(), app, whole, nil)
		if err == nil || !strings.Contains(err.Error(), "setting the clock") {
			t.Fatalf("apply reported %v, wanted the refused write", err)
		}
	})

	// Verify a scanner that will not answer the read back is reported
	t.Run("ReadBackFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{
			reads:    []string{"0,2026,08,02,14,30,00,1", ""},
			readErrs: []error{nil, errors.New("the port closed")},
		}))

		err := apply(context.Background(), app, whole, nil)
		if err == nil || !strings.Contains(err.Error(), "reading the clock back") {
			t.Fatalf("apply reported %v, wanted the failed read back", err)
		}
	})

	// Verify an output stream that refuses the JSON is reported
	t.Run("RenderFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = failWriter{}
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,14,30,00,1"}}))

		err := apply(context.Background(), app, whole, nil)
		if err == nil || !strings.Contains(err.Error(), "output stream closed") {
			t.Fatalf("apply reported %v, wanted the refused write to stdout", err)
		}
	})

	// Verify a scanner holding a different time than it was given says so
	t.Run("Mismatch", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes
		app.SetDevice(device.New(&clockConn{reads: []string{"0,2026,08,02,09,00,00,1"}}))

		if err := apply(context.Background(), app, whole, nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !strings.Contains(notes.String(), "not the 2026-08-02 14:30:00 it was asked for") {
			t.Errorf("the note is %q, wanted it to name the time that was asked for", notes.String())
		}
	})
}

// Test_complete covers whether a value needs anything from the scanner.
//
// Coverage: 100% (2 test cases covering both outcomes)
//
// Test cases:
//   - Both: a value giving a date and a time needs nothing
//   - Partial: a value giving one half needs the other
func Test_complete(t *testing.T) {
	// Verify a value giving both halves is complete
	t.Run("Both", func(t *testing.T) {
		if !(value{hasDate: true, hasTime: true}).complete() {
			t.Error("a value with both halves is not complete")
		}
	})

	// Verify a value giving one half alone is not complete
	t.Run("Partial", func(t *testing.T) {
		if (value{hasDate: true}).complete() {
			t.Error("a value with only a date is complete")
		}
	})
}

// Test_daylightSaving covers deciding what flag to send the scanner.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Override: the flag the user typed wins
//   - Keeps: the flag the scanner already holds is carried over
func Test_daylightSaving(t *testing.T) {
	// Verify a flag the user typed is what gets sent
	t.Run("Override", func(t *testing.T) {
		asked := false
		if daylightSaving(device.Clock{DaylightSaving: true}, &asked) {
			t.Error("the scanner's own flag won over the one that was asked for")
		}
	})

	// Verify the scanner's own flag is kept when the user said nothing
	t.Run("Keeps", func(t *testing.T) {
		if !daylightSaving(device.Clock{DaylightSaving: true}, nil) {
			t.Error("the scanner's own flag was not carried over")
		}
	})
}

// Test_describe covers rounding a drift to a useful unit.
//
// Coverage: 100% (3 test cases covering every branch of the switch)
//
// Test cases:
//   - Days: a gap of more than two days reads in days
//   - Hours: a gap of more than two hours reads in hours
//   - Minutes: anything smaller reads in minutes
func Test_describe(t *testing.T) {
	// Verify a long gap is rounded to whole days
	t.Run("Days", func(t *testing.T) {
		if got := describe(72 * time.Hour); got != "3 days" {
			t.Errorf("describe is %q, wanted %q", got, "3 days")
		}
	})

	// Verify a gap of a few hours is rounded to whole hours
	t.Run("Hours", func(t *testing.T) {
		if got := describe(5 * time.Hour); got != "5 hours" {
			t.Errorf("describe is %q, wanted %q", got, "5 hours")
		}
	})

	// Verify a short gap is rounded to whole minutes
	t.Run("Minutes", func(t *testing.T) {
		if got := describe(90 * time.Minute); got != "90 minutes" {
			t.Errorf("describe is %q, wanted %q", got, "90 minutes")
		}
	})
}

// Test_fill covers taking the halves the user left out from the scanner.
//
// Coverage: 100% (4 test cases covering both branches of both halves)
//
// Test cases:
//   - DateOnly: the date is the user's and the time of day the scanner's
//   - TimeOnly: the time of day is the user's and the date the scanner's
//   - Both: everything is the user's
//   - Neither: everything is the scanner's
func Test_fill(t *testing.T) {
	base := time.Date(2020, 1, 2, 3, 4, 5, 0, time.Local)
	given := time.Date(2026, 8, 2, 14, 30, 45, 0, time.Local)

	// Verify a date alone leaves the scanner's time of day untouched
	t.Run("DateOnly", func(t *testing.T) {
		got := value{t: given, hasDate: true}.fill(base)
		want := time.Date(2026, 8, 2, 3, 4, 5, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("fill is %v, wanted %v", got, want)
		}
	})

	// Verify a time alone leaves the scanner's date untouched
	t.Run("TimeOnly", func(t *testing.T) {
		got := value{t: given, hasTime: true}.fill(base)
		want := time.Date(2020, 1, 2, 14, 30, 45, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("fill is %v, wanted %v", got, want)
		}
	})

	// Verify a value giving both halves keeps neither of the scanner's
	t.Run("Both", func(t *testing.T) {
		got := value{t: given, hasDate: true, hasTime: true}.fill(base)
		if !got.Equal(given) {
			t.Errorf("fill is %v, wanted %v", got, given)
		}
	})

	// Verify a value giving nothing keeps the scanner's reading whole
	t.Run("Neither", func(t *testing.T) {
		got := value{t: given}.fill(base)
		if !got.Equal(base) {
			t.Errorf("fill is %v, wanted %v", got, base)
		}
	})
}

// Test_gap covers the distance between two readings.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Ahead: a reading later than the other
//   - Behind: a reading earlier than the other, which is the same magnitude
func Test_gap(t *testing.T) {
	early := time.Date(2026, 8, 2, 14, 0, 0, 0, time.Local)
	late := time.Date(2026, 8, 2, 15, 0, 0, 0, time.Local)

	// Verify a reading ahead of the other measures the distance between them
	t.Run("Ahead", func(t *testing.T) {
		if got := gap(late, early); got != time.Hour {
			t.Errorf("gap is %v, wanted %v", got, time.Hour)
		}
	})

	// Verify a reading behind the other measures the same distance
	t.Run("Behind", func(t *testing.T) {
		if got := gap(early, late); got != time.Hour {
			t.Errorf("gap is %v, wanted %v", got, time.Hour)
		}
	})
}

// Test_health covers naming what the validity flag means.
//
// Coverage: 100% (2 test cases covering both outcomes)
//
// Test cases:
//   - Running: a valid reading means the clock is running
//   - Stopped: an invalid reading means it is not
func Test_health(t *testing.T) {
	// Verify a valid reading is reported as a running clock
	t.Run("Running", func(t *testing.T) {
		if got := health(true); got != "running" {
			t.Errorf("health is %q, wanted %q", got, "running")
		}
	})

	// Verify an invalid reading is reported as a stopped clock
	t.Run("Stopped", func(t *testing.T) {
		if got := health(false); got != "not running" {
			t.Errorf("health is %q, wanted %q", got, "not running")
		}
	})
}

// Test_missing covers naming the half a value left out.
//
// Coverage: 100% (2 test cases covering both outcomes)
//
// Test cases:
//   - NoTime: a date alone is missing the time of day
//   - NoDate: a time alone is missing the date
func Test_missing(t *testing.T) {
	// Verify a value giving a date alone is missing the time of day
	t.Run("NoTime", func(t *testing.T) {
		if got := missing(value{hasDate: true}); got != "time of day" {
			t.Errorf("missing is %q, wanted %q", got, "time of day")
		}
	})

	// Verify a value giving a time alone is missing the date
	t.Run("NoDate", func(t *testing.T) {
		if got := missing(value{hasTime: true}); got != "date" {
			t.Errorf("missing is %q, wanted %q", got, "date")
		}
	})
}

// Test_nearHour covers telling a daylight saving hour from ordinary drift.
//
// Coverage: 100% (2 test cases covering both outcomes)
//
// Test cases:
//   - Near: a gap within a couple of minutes of the hour
//   - Far: a gap nowhere near the hour
func Test_nearHour(t *testing.T) {
	// Verify a gap close to exactly an hour is recognized as the flag
	t.Run("Near", func(t *testing.T) {
		if !nearHour(time.Hour + 2*time.Minute) {
			t.Error("a gap two minutes past the hour was not recognized")
		}
	})

	// Verify a gap nowhere near an hour is treated as ordinary drift
	t.Run("Far", func(t *testing.T) {
		if nearHour(20 * time.Minute) {
			t.Error("a twenty minute gap was mistaken for the daylight saving hour")
		}
	})
}

// Test_override covers reading the daylight saving flag only when it was typed.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Untouched: a flag that was never typed reports nothing
//   - Typed: a flag that was typed reports what it was set to
func Test_override(t *testing.T) {
	// Verify a flag left alone reports nothing, so the scanner's own is kept
	t.Run("Untouched", func(t *testing.T) {
		if got := override(newSet(appcontext.New()), false); got != nil {
			t.Errorf("override is %v, wanted nothing", *got)
		}
	})

	// Verify a flag that was typed reports what the user asked for
	t.Run("Typed", func(t *testing.T) {
		cmd := newSet(appcontext.New())
		if err := cmd.Flags().Set("dst", "true"); err != nil {
			t.Fatalf("setting the flag: %v", err)
		}

		got := override(cmd, true)
		if got == nil || !*got {
			t.Errorf("override is %v, wanted a request to turn it on", got)
		}
	})
}

// Test_parseValue covers every form a date and time may be written in.
//
// Coverage: 100% (5 test cases covering every loop and the failure)
//
// Test cases:
//   - Full: a date and a time separated by a space
//   - FullT: the same separated by a T, and without seconds
//   - DateOnly: a date on its own
//   - TimeOnly: a time on its own
//   - Invalid: something that matches none of the accepted forms
func Test_parseValue(t *testing.T) {
	// Verify a full date and time is read with both halves marked as given
	t.Run("Full", func(t *testing.T) {
		v, err := parseValue("  2026-08-02 14:30:45  ")
		if err != nil {
			t.Fatalf("parseValue: %v", err)
		}
		if !v.hasDate || !v.hasTime {
			t.Errorf("the value is %+v, wanted both halves", v)
		}
		if want := time.Date(2026, 8, 2, 14, 30, 45, 0, time.Local); !v.t.Equal(want) {
			t.Errorf("the value is %v, wanted %v", v.t, want)
		}
	})

	// Verify the T separator and a missing seconds field are both accepted
	t.Run("FullT", func(t *testing.T) {
		v, err := parseValue("2026-08-02T14:30")
		if err != nil {
			t.Fatalf("parseValue: %v", err)
		}
		if want := time.Date(2026, 8, 2, 14, 30, 0, 0, time.Local); !v.t.Equal(want) {
			t.Errorf("the value is %v, wanted %v", v.t, want)
		}
	})

	// Verify a date alone is accepted and marked as the only half given
	t.Run("DateOnly", func(t *testing.T) {
		v, err := parseValue("2026-08-02")
		if err != nil {
			t.Fatalf("parseValue: %v", err)
		}
		if !v.hasDate || v.hasTime {
			t.Errorf("the value is %+v, wanted the date alone", v)
		}
	})

	// Verify a time alone is accepted and marked as the only half given
	t.Run("TimeOnly", func(t *testing.T) {
		v, err := parseValue("14:30")
		if err != nil {
			t.Fatalf("parseValue: %v", err)
		}
		if v.hasDate || !v.hasTime {
			t.Errorf("the value is %+v, wanted the time alone", v)
		}
	})

	// Verify something in none of the accepted forms is refused with help
	t.Run("Invalid", func(t *testing.T) {
		_, err := parseValue("half past two")
		if err == nil {
			t.Fatal("parseValue reported nothing, wanted the unreadable value")
		}
		if !strings.Contains(err.Error(), "invalid date and time") {
			t.Errorf("parseValue reported %q, wanted it to name the bad value", err)
		}
	})
}

// Test_renderClock covers both output formats and every note the text one adds.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - JSON: the reading is written as JSON
//   - JSONFails: an output stream that refuses the JSON is reported
//   - Agrees: a scanner keeping this computer's time adds no note
//   - Stopped: a stopped clock is called out and the drift check skipped
//   - Drifted: a scanner well away from this computer is told to sync
//   - DaylightHour: a scanner an hour out with the flag set is told why
func Test_renderClock(t *testing.T) {
	// Verify JSON output carries the reading rather than the text rows
	t.Run("JSON", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		r := report{Date: "2026-08-02", Time: "14:30:00", DaylightSaving: true, Valid: true}
		if err := renderClock(app, time.Now(), r); err != nil {
			t.Fatalf("renderClock: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshalling %q: %v", out.String(), err)
		}
		if got != r {
			t.Errorf("renderClock wrote %+v, wanted %+v", got, r)
		}
	})

	// Verify an output stream that refuses the JSON is reported
	t.Run("JSONFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = failWriter{}
		app.Stderr = &bytes.Buffer{}
		app.Config.Output = appcontext.OutputJSON

		err := renderClock(app, time.Now(), report{Valid: true})
		if err == nil || !strings.Contains(err.Error(), "output stream closed") {
			t.Fatalf("renderClock reported %v, wanted the refused write", err)
		}
	})

	// Verify a scanner agreeing with this computer is reported without notes
	t.Run("Agrees", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		notes := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = notes

		r := report{Date: "2026-08-02", Time: "14:30:00", Valid: true}
		if err := renderClock(app, time.Now(), r); err != nil {
			t.Fatalf("renderClock: %v", err)
		}

		want := "date:     2026-08-02\n" +
			"time:     14:30:00\n" +
			"daylight: no\n" +
			"clock:    running\n"
		if got := out.String(); got != want {
			t.Errorf("renderClock wrote %q, wanted %q", got, want)
		}
		if notes.Len() != 0 {
			t.Errorf("renderClock noted %q, wanted nothing", notes.String())
		}
	})

	// Verify a stopped clock is called out rather than left in the digits
	t.Run("Stopped", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		r := report{Date: "2026-08-02", Time: "14:30:00"}
		if err := renderClock(app, time.Now().Add(-48*time.Hour), r); err != nil {
			t.Fatalf("renderClock: %v", err)
		}
		if !strings.Contains(notes.String(), "clock is not running") {
			t.Errorf("the note is %q, wanted it to say the clock is stopped", notes.String())
		}
		if strings.Contains(notes.String(), "from this computer's clock") {
			t.Errorf("the note is %q, wanted the drift check skipped", notes.String())
		}
	})

	// Verify a scanner well away from this computer is told to sync
	t.Run("Drifted", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		r := report{Date: "2026-08-02", Time: "14:30:00", Valid: true}
		if err := renderClock(app, time.Now().Add(-5*time.Hour), r); err != nil {
			t.Fatalf("renderClock: %v", err)
		}
		if !strings.Contains(notes.String(), "5 hours from this computer's clock") {
			t.Errorf("the note is %q, wanted it to name the drift", notes.String())
		}
		if !strings.Contains(notes.String(), "clock sync\" to correct it") {
			t.Errorf("the note is %q, wanted it to offer sync", notes.String())
		}
	})

	// Verify an hour out with daylight saving on is explained rather than synced
	t.Run("DaylightHour", func(t *testing.T) {
		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		r := report{Date: "2026-08-02", Time: "14:30:00", DaylightSaving: true, Valid: true}
		if err := renderClock(app, time.Now().Add(time.Hour), r); err != nil {
			t.Fatalf("renderClock: %v", err)
		}
		if !strings.Contains(notes.String(), "clock sync --dst=false") {
			t.Errorf("the note is %q, wanted it to name the daylight saving hour", notes.String())
		}
	})
}

// Test_runGet covers reading the clock and reporting it.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Success: the reading is written for a person
//   - NoDevice: a run with no scanner named is refused
//   - ReadFails: a scanner that will not answer is reported
func Test_runGet(t *testing.T) {
	// Verify a reading from the scanner comes out as rows of text
	t.Run("Success", func(t *testing.T) {
		app := appcontext.New()
		out := &bytes.Buffer{}
		app.Stdout = out
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{reads: []string{"1,2026,08,02,14,30,00,1"}}))

		if err := runGet(context.Background(), app); err != nil {
			t.Fatalf("runGet: %v", err)
		}
		if !strings.Contains(out.String(), "daylight: yes") {
			t.Errorf("runGet wrote %q, wanted it to report daylight saving", out.String())
		}
	})

	// Verify a run with no scanner named is refused rather than attempted
	t.Run("NoDevice", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		err := runGet(context.Background(), app)
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("runGet reported %v, wanted a missing device", err)
		}
	})

	// Verify a scanner that fails to answer is reported as a failed read
	t.Run("ReadFails", func(t *testing.T) {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.SetDevice(device.New(&clockConn{
			reads:    []string{""},
			readErrs: []error{errors.New("the port closed")},
		}))

		err := runGet(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "reading the clock") {
			t.Fatalf("runGet reported %v, wanted the failed read", err)
		}
	})
}

// Test_yesNo covers rendering a flag as a word.
//
// Coverage: 100% (2 test cases covering both outcomes)
//
