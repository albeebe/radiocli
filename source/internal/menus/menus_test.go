// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package menus

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

// failingWriter is a stream that refuses everything written to it, for the one
// test that has to see a write fail.
type failingWriter struct{}

// Write refuses the bytes and says why.
func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("the pipe is closed")
}

// quicken shortens the poll budgets for the length of one test, and puts the
// real values back afterwards so the shortening cannot leak into another test
// or make the results depend on the order they run in.
func quicken(t *testing.T, polls int) {
	t.Helper()

	wasReadyPolls, wasReadyGap := readyPolls, readyGap
	wasResumePolls, wasResumeGap := resumePolls, resumeGap
	t.Cleanup(func() {
		readyPolls, readyGap = wasReadyPolls, wasReadyGap
		resumePolls, resumeGap = wasResumePolls, wasResumeGap
	})

	readyPolls, readyGap = polls, time.Millisecond
	resumePolls, resumeGap = polls, time.Millisecond
}

// client returns a scanner answering from conn.
func client(conn *stubConn) *device.Scanner {
	return device.New(conn)
}

// selected renders the reply to "STS" for a one line screen with that line
// highlighted, which is what a menu looks like to this package.
func selected(text string) string {
	return "0," + text + "," + strings.Repeat("*", len(text))
}

// menu returns a stub answering as a menu holding rows would, with the knob
// wrapping round the end the way the scanner's does, and records every key
// pressed so a test can check where the walk left the knob.
//
// Parameters:
//   - rows: the menu's entries, in the order the knob reaches them
//   - pressed: filled in with every key command the walk sent
//
// Returns:
//   - a stub connection answering "STS" with whichever row the knob is on
func menu(rows []string, pressed *[]string) *stubConn {
	at := 0
	return &stubConn{exec: func(command string) (string, error) {
		if command == "STS" {
			return selected(rows[at]), nil
		}

		*pressed = append(*pressed, command)
		if command == fmt.Sprintf("KEY,%s,%s", device.KeyRotateLeft, device.KeyPress) {
			at = (at - 1 + len(rows)) % len(rows)
		} else {
			at = (at + 1) % len(rows)
		}
		return "", nil
	}}
}

// plain renders the reply to "STS" for a one line screen with nothing
// highlighted, which is a screen part way through a redraw.
func plain(text string) string {
	return "0," + text + ","
}

// menuDoc renders the document the scanner answers "MSI" with.
func menuDoc(title string, items ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<MenuInfo Name=%q MenuType="TypeSelect">`, title)
	for i, name := range items {
		fmt.Fprintf(&b, `<MenuItem Name=%q Index="%d"/>`, name, i+1)
	}
	b.WriteString(`</MenuInfo>`)
	return b.String()
}

// notInMenu is the document the scanner answers "MSI" with when it is not in a
// menu.
func notInMenu() string {
	return `<MenuInfo Name="" MenuType="TypeError"/>`
}

// scannerDoc renders the document the scanner answers "GSI" with.
func scannerDoc(mode, screen string) string {
	return fmt.Sprintf(`<ScannerInfo Mode=%q V_Screen=%q/>`, mode, screen)
}

// newApp builds an App writing to buffers of the test's own, with a scanner
// that answers from conn.
func newApp(t *testing.T, conn *stubConn) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	app := appcontext.New()
	var out, errs bytes.Buffer
	app.Stdout = &out
	app.Stderr = &errs
	app.SetDevice(device.New(conn))
	return app, &out, &errs
}

// TestAwaken tests the Awaken function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Answers: a scanner that replies is awake
//   - Rejected: a refusal is a reply, so the scanner is awake
//   - Cancelled: a cancelled run is reported rather than retried through
//   - GivesUp: a scanner that never answers is reported after the last attempt
//   - SpendsTheBudget: a scanner failing instantly is still waited for, since
//     the budget is a length of time rather than a number of tries
//   - CancelledWhileWaiting: a run cancelled during the gap gives up there
func TestAwaken(t *testing.T) {
	// Verify that a scanner which replies is treated as awake.
	t.Run("Answers", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) { return selected("SCAN"), nil }})

		if err := Awaken(context.Background(), c); err != nil {
			t.Fatalf("Awaken() = %v, want nil", err)
		}
	})

	// Verify that a refusal counts as an answer.
	t.Run("Rejected", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) { return "", device.ErrRejected }})

		if err := Awaken(context.Background(), c); err != nil {
			t.Fatalf("Awaken() = %v, want nil", err)
		}
	})

	// Verify that a cancelled run stops rather than retrying through it.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := client(&stubConn{exec: func(string) (string, error) {
			cancel()
			return "", errors.New("the port is gone")
		}})

		if err := Awaken(ctx, c); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want a cancelled context", err)
		}
	})

	// Verify that a scanner which never answers is reported.
	t.Run("GivesUp", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := Awaken(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "stopped answering") {
			t.Fatalf("error = %v, want it to say the scanner stopped answering", err)
		}
	})

	// Verify that a scanner failing instantly is still waited for. The budget
	// used to be a count of attempts, which only measured time while every
	// attempt cost the protocol's full timeout: a scanner that answered at once
	// with something unusable spent all twelve in well under a millisecond and
	// then announced that it might still be rebuilding, having waited for
	// nothing at all. A rebuild takes as long as it takes.
	t.Run("SpendsTheBudget", func(t *testing.T) {
		budget, gap := AwakenBudget, AwakenGap
		AwakenBudget, AwakenGap = 60*time.Millisecond, time.Millisecond
		t.Cleanup(func() { AwakenBudget, AwakenGap = budget, gap })

		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("that is not a display")
		}})

		started := time.Now()
		if err := Awaken(context.Background(), c); err == nil {
			t.Fatal("Awaken() = nil, want a scanner that never came back reported")
		}

		if waited := time.Since(started); waited < 50*time.Millisecond {
			t.Errorf("Awaken gave up after %s, want it to have spent its %s budget", waited, AwakenBudget)
		}
	})

	// Verify that a run cancelled while waiting out the gap gives up there,
	// rather than sleeping to the end of it first.
	t.Run("CancelledWhileWaiting", func(t *testing.T) {
		budget, gap := AwakenBudget, AwakenGap
		AwakenBudget, AwakenGap = time.Minute, 30*time.Second
		t.Cleanup(func() { AwakenBudget, AwakenGap = budget, gap })

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		started := time.Now()
		if err := Awaken(ctx, c); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want a cancelled context", err)
		}
		if waited := time.Since(started); waited > 10*time.Second {
			t.Errorf("Awaken waited %s after being cancelled, want it to stop at once", waited)
		}
	})
}

// TestBack tests the Back function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the menu key is pressed once the scanner is ready
//   - ReadyError: a scanner that cannot be waited for is reported
//   - PressError: a failed key press is reported
func TestBack(t *testing.T) {
	quicken(t, 2)

	// Verify that the menu key is pressed once the scanner is ready.
	t.Run("Success", func(t *testing.T) {
		pressed := ""
		c := client(&stubConn{
			exec:    func(command string) (string, error) { pressed = command; return "", nil },
			execXML: func(string) (string, error) { return menuDoc("Menu", "Settings"), nil },
		})

		if err := Back(context.Background(), c); err != nil {
			t.Fatalf("Back() = %v, want nil", err)
		}
		if pressed != fmt.Sprintf("KEY,%s,%s", device.KeyMenu, device.KeyPress) {
			t.Fatalf("pressed %q, want the menu key", pressed)
		}
	})

	// Verify that a scanner which cannot be waited for is reported.
	t.Run("ReadyError", func(t *testing.T) {
		c := client(&stubConn{
			execXML: func(string) (string, error) { return "", errors.New("the port is gone") },
		})

		err := Back(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "waiting for the scanner") {
			t.Fatalf("error = %v, want the failed wait", err)
		}
	})

	// Verify that a failed key press is reported.
	t.Run("PressError", func(t *testing.T) {
		c := client(&stubConn{
			exec:    func(string) (string, error) { return "", errors.New("the scanner refused the key") },
			execXML: func(string) (string, error) { return menuDoc("Menu", "Settings"), nil },
		})

		err := Back(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "going back one level") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})
}

// TestCommit tests the Commit function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the press is acknowledged and the scanner comes back
//   - Timeout: an unacknowledged press is treated as working, not failed
//   - PressError: any other failed press is reported
func TestCommit(t *testing.T) {
	// Verify that an acknowledged press is followed by the wait.
	t.Run("Success", func(t *testing.T) {
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("SCAN"), nil
			}
			return "", nil
		}})

		if err := Commit(context.Background(), c); err != nil {
			t.Fatalf("Commit() = %v, want nil", err)
		}
	})

	// Verify that a press the scanner never acknowledged is treated as working.
	t.Run("Timeout", func(t *testing.T) {
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("SCAN"), nil
			}
			return "", context.DeadlineExceeded
		}})

		if err := Commit(context.Background(), c); err != nil {
			t.Fatalf("Commit() = %v, want nil", err)
		}
	})

	// Verify that any other failed press is reported.
	t.Run("PressError", func(t *testing.T) {
		c := client(&stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}})

		err := Commit(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})
}

// TestConfirm tests the Confirm function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the enter key answers the prompt
//   - PressError: a failed key press is reported
func TestConfirm(t *testing.T) {
	// Verify that the enter key is what answers the prompt.
	t.Run("Success", func(t *testing.T) {
		pressed := ""
		c := client(&stubConn{exec: func(command string) (string, error) {
			pressed = command
			return "", nil
		}})

		if err := Confirm(context.Background(), c); err != nil {
			t.Fatalf("Confirm() = %v, want nil", err)
		}
		if pressed != fmt.Sprintf("KEY,%s,%s", device.KeyEnter, device.KeyPress) {
			t.Fatalf("pressed %q, want the enter key", pressed)
		}
	})

	// Verify that a failed key press is reported.
	t.Run("PressError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}})

		err := Confirm(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "confirming") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})
}

// TestConfirmDelete tests the ConfirmDelete function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Asking: the prompt is on screen, so it is answered
//   - ReadError: a screen that cannot be read is reported
//   - NotAsking: another screen is named and nothing is pressed
func TestConfirmDelete(t *testing.T) {
	// Verify that the prompt on screen is answered with the enter key.
	t.Run("Asking", func(t *testing.T) {
		pressed := ""
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return "00, ,," + deletePrompt + ",", nil
			}
			pressed = command
			return "", nil
		}})

		if err := ConfirmDelete(context.Background(), c); err != nil {
			t.Fatalf("ConfirmDelete() = %v, want nil", err)
		}
		if pressed != fmt.Sprintf("KEY,%s,%s", device.KeyEnter, device.KeyPress) {
			t.Fatalf("pressed %q, want the enter key", pressed)
		}
	})

	// Verify that a screen which cannot be read is reported.
	t.Run("ReadError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := ConfirmDelete(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "reading the screen before confirming") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})

	// Verify that another screen is named and nothing is pressed.
	t.Run("NotAsking", func(t *testing.T) {
		pressed := false
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("Volume"), nil
			}
			pressed = true
			return "", nil
		}})

		err := ConfirmDelete(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "nothing was deleted") {
			t.Fatalf("error = %v, want it to say nothing was deleted", err)
		}
		if !strings.Contains(err.Error(), `"Volume"`) {
			t.Fatalf("error = %v, want it to name what is on screen", err)
		}
		if pressed {
			t.Fatal("a key was pressed on a screen that was not the prompt")
		}
	})
}

// TestEnter tests the Enter function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the enter key selects the highlighted entry
//   - PressError: a failed key press is reported
func TestEnter(t *testing.T) {
	// Verify that the enter key is what selects the entry.
	t.Run("Success", func(t *testing.T) {
		pressed := ""
		c := client(&stubConn{exec: func(command string) (string, error) {
			pressed = command
			return "", nil
		}})

		if err := Enter(context.Background(), c); err != nil {
			t.Fatalf("Enter() = %v, want nil", err)
		}
		if pressed != fmt.Sprintf("KEY,%s,%s", device.KeyEnter, device.KeyPress) {
			t.Fatalf("pressed %q, want the enter key", pressed)
		}
	})

	// Verify that a failed key press is reported.
	t.Run("PressError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}})

		err := Enter(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})
}

// TestEntries tests the Entries function with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - Success: the knob is turned all the way round and every entry read off
//   - SingleEntry: a menu of one entry is read as one entry
//   - DuplicateNames: a name repeated part way round does not end the walk
//   - FirstError: a screen that cannot be read before the walk is reported
//   - TurnError: a knob that will not turn is reported
//   - StepError: a screen that cannot be read during the walk is reported
//   - TurnBackError: a knob that will not go back to the start is reported
//   - NoWrap: a menu that never comes back round is reported
func TestEntries(t *testing.T) {
	quicken(t, 2)

	// Verify that the knob is turned round, every entry read off, and the knob
	// put back on the entry the caller left it on.
	t.Run("Success", func(t *testing.T) {
		var pressed []string
		c := client(menu([]string{"Volume", "Squelch"}, &pressed))

		got, err := Entries(context.Background(), c)
		if err != nil {
			t.Fatalf("Entries() = %v, want nil", err)
		}
		if strings.Join(got, ",") != "Volume,Squelch" {
			t.Fatalf("entries = %v, want the two the menu holds", got)
		}

		back := fmt.Sprintf("KEY,%s,%s", device.KeyRotateLeft, device.KeyPress)
		if len(pressed) == 0 || pressed[len(pressed)-1] != back {
			t.Fatalf("pressed %v, want the walk to step back to where it started", pressed)
		}
	})

	// Verify that a menu holding one entry is read as one entry. Every turn of
	// the knob lands on the same row, so nothing but the confirming read
	// separates it from a walk that has not moved.
	t.Run("SingleEntry", func(t *testing.T) {
		var pressed []string
		c := client(menu([]string{"Volume"}, &pressed))

		got, err := Entries(context.Background(), c)
		if err != nil {
			t.Fatalf("Entries() = %v, want nil", err)
		}
		if strings.Join(got, ",") != "Volume" {
			t.Fatalf("entries = %v, want the one the menu holds", got)
		}
	})

	// Verify that a name repeated part way round does not end the walk. This
	// is the regression: the scanner's own menus are unique-named, but a
	// favorites list, a system or a channel list is whatever somebody called
	// it, and two entries may be called the same thing.
	t.Run("DuplicateNames", func(t *testing.T) {
		var pressed []string
		c := client(menu([]string{"Fire", "Police", "Fire", "EMS"}, &pressed))

		got, err := Entries(context.Background(), c)
		if err != nil {
			t.Fatalf("Entries() = %v, want nil", err)
		}
		if strings.Join(got, ",") != "Fire,Police,Fire,EMS" {
			t.Fatalf("entries = %v, want all four: the second Fire is not the start", got)
		}
	})

	// Verify that a screen which cannot be read before the walk is reported.
	t.Run("FirstError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := Entries(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})

	// Verify that a knob which will not turn is reported.
	t.Run("TurnError", func(t *testing.T) {
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("Volume"), nil
			}
			return "", errors.New("the scanner refused the key")
		}})

		if _, err := Entries(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("error = %v, want the failed turn", err)
		}
	})

	// Verify that a screen which cannot be read during the walk is reported.
	t.Run("StepError", func(t *testing.T) {
		reads := 0
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command != "STS" {
				return "", nil
			}
			reads++
			if reads > 1 {
				return "", errors.New("the port is gone")
			}
			return selected("Volume"), nil
		}})

		if _, err := Entries(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})

	// Verify that a knob which will not go back to the start is reported. The
	// walk reads one entry past the start to prove it got there, so the step
	// back is a real key press and can fail like any other.
	t.Run("TurnBackError", func(t *testing.T) {
		rows := []string{"Volume", "Squelch"}
		at := 0
		back := fmt.Sprintf("KEY,%s,%s", device.KeyRotateLeft, device.KeyPress)
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected(rows[at%len(rows)]), nil
			}
			if command == back {
				return "", errors.New("the scanner refused the key")
			}
			at++
			return "", nil
		}})

		_, err := Entries(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "turning the knob back") {
			t.Fatalf("error = %v, want the failed step back", err)
		}
	})

	// Verify that a menu which never comes back round is reported.
	t.Run("NoWrap", func(t *testing.T) {
		at := 0
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected(fmt.Sprintf("Entry %d", at)), nil
			}
			at++
			return "", nil
		}})

		_, err := Entries(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "did not come back round") {
			t.Fatalf("error = %v, want the walk to give up", err)
		}
	})
}

// TestHighlighted tests the Highlighted function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the selected row is returned as text
//   - ReadError: a screen that cannot be read is reported
func TestHighlighted(t *testing.T) {
	quicken(t, 2)

	// Verify that the selected row is returned as text.
	t.Run("Success", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) { return selected("Volume"), nil }})

		got, err := Highlighted(context.Background(), c)
		if err != nil {
			t.Fatalf("Highlighted() = %v, want nil", err)
		}
		if got != "Volume" {
			t.Fatalf("row = %q, want %q", got, "Volume")
		}
	})

	// Verify that a screen which cannot be read is reported.
	t.Run("ReadError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := Highlighted(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})
}

// TestHighlightedRow tests the HighlightedRow function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Cut: a row the scanner shortened is reported as shortened
//   - ReadError: a screen that cannot be read is reported
func TestHighlightedRow(t *testing.T) {
	quicken(t, 2)

	// Verify that a shortened row is reported as shortened.
	t.Run("Cut", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return selected("Quick Save Favorites L\x01\x02"), nil
		}})

		got, err := HighlightedRow(context.Background(), c)
		if err != nil {
			t.Fatalf("HighlightedRow() = %v, want nil", err)
		}
		if got.Text != "Quick Save Favorites L" || !got.Cut {
			t.Fatalf("row = %+v, want the shortened name", got)
		}
	})

	// Verify that a screen which cannot be read is reported.
	t.Run("ReadError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := HighlightedRow(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})
}

// TestLeave tests the Leave function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the scanner leaves the menus and goes back to scanning
//   - LeaveError: a scanner that cannot be taken out of the menus is reported
//   - ResumeError: a scanner that will not go back to scanning is reported
func TestLeave(t *testing.T) {
	quicken(t, 2)

	// Verify that leaving the menus is followed by returning to scanning.
	t.Run("Success", func(t *testing.T) {
		c := client(&stubConn{execXML: func(command string) (string, error) {
			if command == "GSI" {
				return scannerDoc("Scan Mode", "conventional"), nil
			}
			return notInMenu(), nil
		}})

		pressed, err := Leave(context.Background(), c)
		if err != nil {
			t.Fatalf("Leave() = %v, want nil", err)
		}
		if pressed != 0 {
			t.Fatalf("presses = %d, want 0 for a scanner already out of the menus", pressed)
		}
	})

	// Verify that a scanner which cannot be taken out of the menus is reported.
	t.Run("LeaveError", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := Leave(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "waiting for the scanner") {
			t.Fatalf("error = %v, want the failed wait", err)
		}
	})

	// Verify that a scanner which will not go back to scanning is reported.
	t.Run("ResumeError", func(t *testing.T) {
		c := client(&stubConn{execXML: func(command string) (string, error) {
			if command == "GSI" {
				return "", errors.New("the port is gone")
			}
			return notInMenu(), nil
		}})

		if _, err := Leave(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "asking the scanner what it is doing") {
			t.Fatalf("error = %v, want the failed question", err)
		}
	})
}

// TestLookup tests the Lookup function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Known: an accepted name, in any case, gives the scanner's own menu
//   - Unknown: a name this tool does not accept is reported as such
func TestLookup(t *testing.T) {
	// Verify that an accepted name gives the scanner's own menu, in any case.
	t.Run("Known", func(t *testing.T) {
		for _, name := range []string{"top", "TOP", "Top"} {
			id, ok := Lookup(name)
			if !ok || id != device.MenuTop {
				t.Fatalf("Lookup(%q) = %q, %v, want the top menu", name, id, ok)
			}
		}
	})

	// Verify that a name this tool does not accept is reported as such.
	t.Run("Unknown", func(t *testing.T) {
		if _, ok := Lookup("nonsense"); ok {
			t.Fatal("Lookup(\"nonsense\") was accepted, want it refused")
		}
	})
}

// TestNames tests the Names function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Sorted: every accepted name is listed once, in order
func TestNames(t *testing.T) {
	// Verify that every accepted name is listed once, in order.
	t.Run("Sorted", func(t *testing.T) {
		got := Names()

		if len(got) != len(byName) {
			t.Fatalf("Names() holds %d names, want %d", len(got), len(byName))
		}
		for i := 1; i < len(got); i++ {
			if got[i-1] >= got[i] {
				t.Fatalf("Names() = %v, want them sorted", got)
			}
		}
	})
}

// TestOpen tests the Open function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the menu is opened and where it landed is reported
//   - OpenError: a menu that will not open is named in the error
func TestOpen(t *testing.T) {
	quicken(t, 2)

	// Verify that the menu is opened and where it landed is reported.
	t.Run("Success", func(t *testing.T) {
		conn := &stubConn{
			exec: func(command string) (string, error) {
				if command == "STS" {
					return selected("Settings"), nil
				}
				return "", nil
			},
			execXML: func(string) (string, error) { return menuDoc("Menu", "Settings"), nil },
		}
		app, out, _ := newApp(t, conn)

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}
		if err := Open(context.Background(), app, c, device.MenuTop, "", "top"); err != nil {
			t.Fatalf("Open() = %v, want nil", err)
		}
		if !strings.Contains(out.String(), "menu: Menu") {
			t.Fatalf("output = %q, want the menu reported", out.String())
		}
	})

	// Verify that a menu which will not open is named in the error.
	t.Run("OpenError", func(t *testing.T) {
		conn := &stubConn{
			exec: func(string) (string, error) { return "", device.ErrRejected },
		}
		app, _, _ := newApp(t, conn)

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}

		err = Open(context.Background(), app, c, device.MenuTop, "", "top")
		if err == nil || !strings.Contains(err.Error(), "opening the top menu") {
			t.Fatalf("error = %v, want the menu named", err)
		}
	})
}

// TestSettle tests the Ready function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NotInMenu: a scanner back to scanning is as ready as it gets
//   - Busy: a scanner naming no menu is waited for, then acted on
//   - AskError: a scanner that cannot be asked where it is is reported
//   - Cancelled: a cancelled run is reported
//   - GivesUp: a scanner still busy at the end is not an error
func TestSettle(t *testing.T) {
	quicken(t, 2)

	// Verify that a scanner back to scanning is ready.
	t.Run("NotInMenu", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) { return notInMenu(), nil }})

		if err := Settle(context.Background(), c); err != nil {
			t.Fatalf("Settle() = %v, want nil", err)
		}
	})

	// Verify that a scanner naming no menu is waited for.
	t.Run("Busy", func(t *testing.T) {
		asked := 0
		c := client(&stubConn{execXML: func(string) (string, error) {
			asked++
			if asked == 1 {
				return menuDoc(""), nil
			}
			return menuDoc("Menu"), nil
		}})

		if err := Settle(context.Background(), c); err != nil {
			t.Fatalf("Settle() = %v, want nil", err)
		}
		if asked != 2 {
			t.Fatalf("asked %d times, want the busy screen to have been waited out", asked)
		}
	})

	// Verify that a scanner which cannot be asked where it is is reported.
	t.Run("AskError", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := Settle(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "waiting for the scanner") {
			t.Fatalf("error = %v, want the failed question", err)
		}
	})

	// Verify that a cancelled run is reported.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := client(&stubConn{execXML: func(string) (string, error) {
			cancel()
			return menuDoc(""), nil
		}})

		if err := Settle(ctx, c); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want a cancelled context", err)
		}
	})

	// Verify that a scanner still busy at the end is not an error.
	t.Run("GivesUp", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) { return menuDoc(""), nil }})

		if err := Settle(context.Background(), c); err != nil {
			t.Fatalf("Settle() = %v, want nil for a scanner that is merely slow", err)
		}
	})
}

// TestResume tests the Resume function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - AskError: a scanner that will not say what it is doing is reported
//   - Weather: a scanner on the weather channels is left there
//   - QuickSearch: a scanner holding one frequency is taken out of it
//   - QuickSearchError: a scanner that will not leave quick search is reported
//   - Holding: a held scanner is put back to scanning
//   - HoldingError: a scanner that will not release its hold is reported
//   - Scanning: a scanner already scanning needs nothing
func TestResume(t *testing.T) {
	quicken(t, 2)

	// Verify that a scanner which will not say what it is doing is reported.
	t.Run("AskError", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := Resume(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "asking the scanner what it is doing") {
			t.Fatalf("error = %v, want the failed question", err)
		}
	})

	// Verify that a scanner on the weather channels is left there.
	t.Run("Weather", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return `<ScannerInfo Mode="WX Hold" V_Screen="wx_alert">` +
				`<WxMode Mode="Monitor Weather"/></ScannerInfo>`, nil
		}})

		did, err := Resume(context.Background(), c)
		if err != nil {
			t.Fatalf("Resume() = %v, want nil", err)
		}
		if did != "" {
			t.Fatalf("Resume() said %q, want nothing done on the weather channels", did)
		}
	})

	// Verify that a scanner holding one frequency is taken out of quick search.
	t.Run("QuickSearch", func(t *testing.T) {
		asked := 0
		c := client(&stubConn{execXML: func(string) (string, error) {
			asked++
			if asked == 1 {
				return scannerDoc("Quick Search Hold", quickSearch), nil
			}
			return scannerDoc("Scan Mode", "conventional"), nil
		}})

		did, err := Resume(context.Background(), c)
		if err != nil {
			t.Fatalf("Resume() = %v, want nil", err)
		}
		if did != "left quick search" {
			t.Fatalf("Resume() said %q, want it to report leaving quick search", did)
		}
	})

	// Verify that a scanner which will not leave quick search is reported.
	t.Run("QuickSearchError", func(t *testing.T) {
		c := client(&stubConn{
			exec:    func(string) (string, error) { return "", errors.New("the scanner refused the key") },
			execXML: func(string) (string, error) { return scannerDoc("Quick Search Hold", quickSearch), nil },
		})

		_, err := Resume(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "leaving quick search") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})

	// Verify that a held scanner is put back to scanning.
	t.Run("Holding", func(t *testing.T) {
		asked := 0
		c := client(&stubConn{execXML: func(string) (string, error) {
			asked++
			if asked == 1 {
				return scannerDoc("Scan Hold", "conventional"), nil
			}
			return scannerDoc("Scan Mode", "conventional"), nil
		}})

		did, err := Resume(context.Background(), c)
		if err != nil {
			t.Fatalf("Resume() = %v, want nil", err)
		}
		if did != "released the hold" {
			t.Fatalf("Resume() said %q, want it to report releasing the hold", did)
		}
	})

	// Verify that a scanner which will not release its hold is reported.
	t.Run("HoldingError", func(t *testing.T) {
		c := client(&stubConn{
			exec:    func(string) (string, error) { return "", errors.New("the scanner refused the command") },
			execXML: func(string) (string, error) { return scannerDoc("Scan Hold", "conventional"), nil },
		})

		_, err := Resume(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "returning the scanner to scanning from Scan Hold") {
			t.Fatalf("error = %v, want the failed jump", err)
		}
	})

	// Verify that a scanner already scanning needs nothing done.
	t.Run("Scanning", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return scannerDoc("Scan Mode", "conventional"), nil
		}})

		did, err := Resume(context.Background(), c)
		if err != nil {
			t.Fatalf("Resume() = %v, want nil", err)
		}
		if did != "" {
			t.Fatalf("Resume() said %q, want nothing done", did)
		}
	})
}

// TestSelect tests the Select function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the entry is stepped to and opened
//   - StepError: an entry that cannot be reached is reported
//   - EnterError: a failed key press is reported
func TestSelect(t *testing.T) {
	quicken(t, 2)

	// Verify that the entry is stepped to and then opened.
	t.Run("Success", func(t *testing.T) {
		pressed := ""
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("Settings"), nil
			}
			pressed = command
			return "", nil
		}})

		if err := Select(context.Background(), c, "Settings"); err != nil {
			t.Fatalf("Select() = %v, want nil", err)
		}
		if pressed != fmt.Sprintf("KEY,%s,%s", device.KeyEnter, device.KeyPress) {
			t.Fatalf("pressed %q, want the enter key", pressed)
		}
	})

	// Verify that an entry which cannot be reached is reported.
	t.Run("StepError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := Select(context.Background(), c, "Settings")
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})

	// Verify that a failed key press is reported.
	t.Run("EnterError", func(t *testing.T) {
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("Settings"), nil
			}
			return "", errors.New("the scanner refused the key")
		}})

		err := Select(context.Background(), c, "Settings")
		if err == nil || !strings.Contains(err.Error(), "selecting the highlighted entry") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})
}

// TestShow tests the Show function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Text: the menu is rendered with the highlighted entry marked
//   - JSON: the menu is written as JSON when that was asked for
//   - NotInMenuText: a scanner outside the menus is said so
//   - NotInMenuJSON: a scanner outside the menus writes a null document
//   - ReadError: a menu that cannot be read is reported
//   - Unlisted: a highlighted row the listing omits is called out
func TestShow(t *testing.T) {
	quicken(t, 2)

	// Verify that the menu is rendered with the highlighted entry marked.
	t.Run("Text", func(t *testing.T) {
		conn := &stubConn{
			exec:    func(string) (string, error) { return selected("Settings"), nil },
			execXML: func(string) (string, error) { return menuDoc("Menu", "Volume", "Settings"), nil },
		}
		app, out, _ := newApp(t, conn)

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}
		if err := Show(context.Background(), app, c); err != nil {
			t.Fatalf("Show() = %v, want nil", err)
		}
		if !strings.Contains(out.String(), ">") || !strings.Contains(out.String(), "Settings") {
			t.Fatalf("output = %q, want the highlighted entry marked", out.String())
		}
	})

	// Verify that the menu is written as JSON when that was asked for.
	t.Run("JSON", func(t *testing.T) {
		conn := &stubConn{
			exec:    func(string) (string, error) { return selected("Settings"), nil },
			execXML: func(string) (string, error) { return menuDoc("Menu", "Settings"), nil },
		}
		app, out, _ := newApp(t, conn)
		app.Config.Output = appcontext.OutputJSON

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}
		if err := Show(context.Background(), app, c); err != nil {
			t.Fatalf("Show() = %v, want nil", err)
		}
		if !strings.Contains(out.String(), `"highlighted": true`) {
			t.Fatalf("output = %q, want the JSON report", out.String())
		}
	})

	// Verify that a scanner outside the menus is said so.
	t.Run("NotInMenuText", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) { return notInMenu(), nil }}
		app, _, errs := newApp(t, conn)

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}
		if err := Show(context.Background(), app, c); err != nil {
			t.Fatalf("Show() = %v, want nil", err)
		}
		if !strings.Contains(errs.String(), "not in a menu") {
			t.Fatalf("notes = %q, want it to say the scanner is not in a menu", errs.String())
		}
	})

	// Verify that a scanner outside the menus writes a null document in JSON.
	t.Run("NotInMenuJSON", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) { return notInMenu(), nil }}
		app, out, _ := newApp(t, conn)
		app.Config.Output = appcontext.OutputJSON

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}
		if err := Show(context.Background(), app, c); err != nil {
			t.Fatalf("Show() = %v, want nil", err)
		}
		if strings.TrimSpace(out.String()) != "null" {
			t.Fatalf("output = %q, want a null document", out.String())
		}
	})

	// Verify that a menu which cannot be read is reported.
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}}
		app, _, _ := newApp(t, conn)

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}

		err = Show(context.Background(), app, c)
		if err == nil || !strings.Contains(err.Error(), "reading the menu") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})

	// Verify that a highlighted row the listing omits is called out.
	t.Run("Unlisted", func(t *testing.T) {
		conn := &stubConn{
			exec:    func(string) (string, error) { return selected("Review Avoids"), nil },
			execXML: func(string) (string, error) { return menuDoc("Menu", "Volume", "Settings"), nil },
		}
		app, _, errs := newApp(t, conn)

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}
		if err := Show(context.Background(), app, c); err != nil {
			t.Fatalf("Show() = %v, want nil", err)
		}
		if !strings.Contains(errs.String(), "which this listing does not include") {
			t.Fatalf("notes = %q, want the omission called out", errs.String())
		}
	})

	// Verify that an unreadable screen still leaves the listing rendered.
	t.Run("Unreadable", func(t *testing.T) {
		conn := &stubConn{
			exec:    func(string) (string, error) { return "", errors.New("the port is gone") },
			execXML: func(string) (string, error) { return menuDoc("Menu", "Settings"), nil },
		}
		app, out, _ := newApp(t, conn)

		c, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("taking the scanner: %v", err)
		}
		if err := Show(context.Background(), app, c); err != nil {
			t.Fatalf("Show() = %v, want nil", err)
		}
		if !strings.Contains(out.String(), "Settings") {
			t.Fatalf("output = %q, want the listing rendered anyway", out.String())
		}
	})
}

// TestStepTo tests the StepTo function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Found: the walk stops on the entry it was looking for
//   - ReadError: a screen that cannot be read is reported
//   - TurnError: a knob that will not turn is reported
//   - NotThere: a menu holding no such entry is reported with what it holds
//   - DuplicateNames: a name repeated part way round does not end the walk
//   - GaveUp: a menu that never repeats is given up on
func TestStepTo(t *testing.T) {
	quicken(t, 2)

	// Verify that the walk stops on the entry it was looking for.
	t.Run("Found", func(t *testing.T) {
		rows := []string{"Volume", "Squelch", "Settings"}
		at := 0
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected(rows[at]), nil
			}
			at++
			return "", nil
		}})

		if err := StepTo(context.Background(), c, "Settings"); err != nil {
			t.Fatalf("StepTo() = %v, want nil", err)
		}
		if at != 2 {
			t.Fatalf("turned the knob %d times, want 2", at)
		}
	})

	// Verify that a screen which cannot be read is reported.
	t.Run("ReadError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := StepTo(context.Background(), c, "Settings")
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})

	// Verify that a knob which will not turn is reported.
	t.Run("TurnError", func(t *testing.T) {
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("Volume"), nil
			}
			return "", errors.New("the scanner refused the key")
		}})

		err := StepTo(context.Background(), c, "Settings")
		if err == nil || !strings.Contains(err.Error(), `turning the knob, after passing "Volume"`) {
			t.Fatalf("error = %v, want the failed turn", err)
		}
	})

	// Verify that a menu holding no such entry is reported with what it holds.
	t.Run("NotThere", func(t *testing.T) {
		var pressed []string
		c := client(menu([]string{"Volume", "Squelch"}, &pressed))

		err := StepTo(context.Background(), c, "Settings")
		if err == nil || !strings.Contains(err.Error(), `no entry called "Settings"`) {
			t.Fatalf("error = %v, want the missing entry", err)
		}
		if !strings.Contains(err.Error(), `"Volume", "Squelch"`) {
			t.Fatalf("error = %v, want it to list what the menu holds", err)
		}
	})

	// Verify that a name repeated part way round does not end the walk short of
	// the entry that is really there. Before the walk confirmed a repeat by the
	// entry after it, this reported a menu holding "Settings" as not holding it.
	t.Run("DuplicateNames", func(t *testing.T) {
		var pressed []string
		c := client(menu([]string{"Fire", "Police", "Fire", "Settings"}, &pressed))

		if err := StepTo(context.Background(), c, "Settings"); err != nil {
			t.Fatalf("StepTo() = %v, want nil: the second Fire is not the start", err)
		}
	})

	// Verify that a menu which never repeats is given up on.
	t.Run("GaveUp", func(t *testing.T) {
		at := 0
		c := client(&stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected(fmt.Sprintf("Entry %d", at)), nil
			}
			at++
			return "", nil
		}})

		err := StepTo(context.Background(), c, "Settings")
		if err == nil || !strings.Contains(err.Error(), "gave up looking") {
			t.Fatalf("error = %v, want the walk to give up", err)
		}
	})
}

// Test_clean tests the clean function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Rows: padding and the scanner's own markings are taken off
func Test_clean(t *testing.T) {
	// Verify that padding and the scanner's markings are taken off.
	t.Run("Rows", func(t *testing.T) {
		for _, tc := range []struct {
			text, want string
		}{
			{"  Volume  ", "Volume"},
			{"Quick Save Favorites L\x01\x02", "Quick Save Favorites L"},
			{"", ""},
		} {
			if got := clean(tc.text); got != tc.want {
				t.Fatalf("clean(%q) = %q, want %q", tc.text, got, tc.want)
			}
		}
	})
}

// Test_cut tests the cut function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Rows: only a row carrying the scanner's markings was shortened
func Test_cut(t *testing.T) {
	// Verify that only a marked row counts as shortened.
	t.Run("Rows", func(t *testing.T) {
		if cut("Volume") {
			t.Fatal("cut(\"Volume\") = true, want false for a whole name")
		}
		if !cut("Quick Save Favorites L\x01") {
			t.Fatal("cut() = false, want true for a marked name")
		}
	})
}

// Test_dash tests the dash function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Values: a missing index reads as a dash and a real one as itself
func Test_dash(t *testing.T) {
	// Verify that a missing index reads as a dash.
	t.Run("Values", func(t *testing.T) {
		if got := dash(""); got != "-" {
			t.Fatalf("dash(\"\") = %q, want %q", got, "-")
		}
		if got := dash("12"); got != "12" {
			t.Fatalf("dash(\"12\") = %q, want %q", got, "12")
		}
	})
}

// Test_highlighted tests the highlighted function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Selected: the highlighted row is returned along with whether it was cut
//   - ReadError: a screen that cannot be read is reported
//   - Cancelled: a cancelled run is reported rather than waited out
//   - NothingHighlighted: a screen highlighting nothing is reported
func Test_highlighted(t *testing.T) {
	quicken(t, 2)

	// Verify that the highlighted row is returned with its shortening.
	t.Run("Selected", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) { return selected("Volume"), nil }})

		got, err := highlighted(context.Background(), c)
		if err != nil {
			t.Fatalf("highlighted() = %v, want nil", err)
		}
		if got.text != "Volume" || got.cut {
			t.Fatalf("row = %+v, want the whole name", got)
		}
	})

	// Verify that a screen which cannot be read is reported.
	t.Run("ReadError", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := highlighted(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "reading the screen") {
			t.Fatalf("error = %v, want the failed read", err)
		}
	})

	// Verify that a cancelled run stops rather than being waited out.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := client(&stubConn{exec: func(string) (string, error) {
			cancel()
			return plain("Please Wait"), nil
		}})

		if _, err := highlighted(ctx, c); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want a cancelled context", err)
		}
	})

	// Verify that a screen highlighting nothing is reported.
	t.Run("NothingHighlighted", func(t *testing.T) {
		c := client(&stubConn{exec: func(string) (string, error) { return plain("Please Wait"), nil }})

		_, err := highlighted(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "highlighted nothing") {
			t.Fatalf("error = %v, want it to say nothing was highlighted", err)
		}
	})
}

// Test_leaveHold tests the leaveHold function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Released: the scanner stops holding and nothing more is needed
//   - JumpError: a scanner that refuses the jump is reported with its mode
//   - Cancelled: a cancelled run is reported rather than waited out
//   - StillHolding: a scanner still holding at the end is reported
func Test_leaveHold(t *testing.T) {
	quicken(t, 2)

	// Verify that a scanner which stops holding needs nothing more.
	t.Run("Released", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return scannerDoc("Scan Mode", "conventional"), nil
		}})

		info := device.ScannerInfo{Mode: "Scan Hold"}
		if err := leaveHold(context.Background(), c, info); err != nil {
			t.Fatalf("leaveHold() = %v, want nil", err)
		}
	})

	// Verify that a scanner refusing the jump is reported with its mode.
	t.Run("JumpError", func(t *testing.T) {
		c := client(&stubConn{
			exec: func(string) (string, error) { return "", errors.New("the scanner refused the command") },
		})

		info := device.ScannerInfo{Mode: "Scan Hold"}
		err := leaveHold(context.Background(), c, info)
		if err == nil || !strings.Contains(err.Error(), "returning the scanner to scanning from Scan Hold") {
			t.Fatalf("error = %v, want the failed jump", err)
		}
	})

	// Verify that a cancelled run stops rather than being waited out.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := client(&stubConn{execXML: func(string) (string, error) {
			cancel()
			return scannerDoc("Scan Hold", "conventional"), nil
		}})

		info := device.ScannerInfo{Mode: "Scan Hold"}
		if err := leaveHold(ctx, c, info); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want a cancelled context", err)
		}
	})

	// Verify that a scanner still holding at the end is reported.
	t.Run("StillHolding", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return scannerDoc("Scan Hold", "conventional"), nil
		}})

		info := device.ScannerInfo{Mode: "Scan Hold"}
		err := leaveHold(context.Background(), c, info)
		if err == nil || !strings.Contains(err.Error(), "still holding") {
			t.Fatalf("error = %v, want it to say the scanner is still holding", err)
		}
	})
}

// Test_leaveMenus tests the leaveMenus function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - AlreadyOut: a scanner outside the menus takes no presses at all
//   - ReadyError: a scanner that cannot be waited for is reported
//   - AskError: a scanner that will not say where it is is reported
//   - PressError: a failed press is reported
//   - PressTimeout: an unacknowledged press is waited out and tried again
//   - AwakenError: a scanner that never comes back from a press is reported
//   - Exhausted: a scanner that will not leave is reported with its screen
func Test_leaveMenus(t *testing.T) {
	quicken(t, 2)

	// Verify that a scanner already outside the menus takes no presses.
	t.Run("AlreadyOut", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) { return notInMenu(), nil }})

		pressed, err := leaveMenus(context.Background(), c)
		if err != nil {
			t.Fatalf("leaveMenus() = %v, want nil", err)
		}
		if pressed != 0 {
			t.Fatalf("presses = %d, want 0", pressed)
		}
	})

	// Verify that a scanner which cannot be waited for is reported.
	t.Run("ReadyError", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := leaveMenus(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "waiting for the scanner") {
			t.Fatalf("error = %v, want the failed wait", err)
		}
	})

	// Verify that a scanner which will not say where it is is reported.
	t.Run("AskError", func(t *testing.T) {
		asked := 0
		c := client(&stubConn{execXML: func(string) (string, error) {
			asked++
			if asked > 1 {
				return "", errors.New("the port is gone")
			}
			return menuDoc("Menu"), nil
		}})

		if _, err := leaveMenus(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "asking the scanner where it is") {
			t.Fatalf("error = %v, want the failed question", err)
		}
	})

	// Verify that a failed press is reported.
	t.Run("PressError", func(t *testing.T) {
		c := client(&stubConn{
			exec:    func(string) (string, error) { return "", errors.New("the scanner refused the key") },
			execXML: func(string) (string, error) { return menuDoc("Menu"), nil },
		})

		if _, err := leaveMenus(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "leaving the menus") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})

	// Verify that an unacknowledged press is waited out and tried again.
	t.Run("PressTimeout", func(t *testing.T) {
		asked := 0
		c := client(&stubConn{
			exec: func(command string) (string, error) {
				if command == "STS" {
					return selected("SCAN"), nil
				}
				return "", context.DeadlineExceeded
			},
			execXML: func(string) (string, error) {
				asked++
				if asked <= 2 {
					return menuDoc("Menu"), nil
				}
				return notInMenu(), nil
			},
		})

		pressed, err := leaveMenus(context.Background(), c)
		if err != nil {
			t.Fatalf("leaveMenus() = %v, want nil", err)
		}
		if pressed != 1 {
			t.Fatalf("presses = %d, want 1", pressed)
		}
	})

	// Verify that a scanner which never comes back from a press is reported.
	t.Run("AwakenError", func(t *testing.T) {
		c := client(&stubConn{
			exec: func(command string) (string, error) {
				if command == "STS" {
					return "", errors.New("the port is gone")
				}
				return "", context.DeadlineExceeded
			},
			execXML: func(string) (string, error) { return menuDoc("Menu"), nil },
		})

		if _, err := leaveMenus(context.Background(), c); err == nil ||
			!strings.Contains(err.Error(), "stopped answering") {
			t.Fatalf("error = %v, want the scanner never coming back", err)
		}
	})

	// Verify that a scanner which will not leave is reported with its screen.
	t.Run("Exhausted", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) { return menuDoc("Settings"), nil }})

		pressed, err := leaveMenus(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), `the "Settings" screen`) {
			t.Fatalf("error = %v, want it to name the screen", err)
		}
		if pressed != leaveAttempts {
			t.Fatalf("presses = %d, want %d", pressed, leaveAttempts)
		}
	})

	// Verify that a scanner which will not even say where it is is described
	// as somewhere it could not be taken out of.
	t.Run("ExhaustedUnknown", func(t *testing.T) {
		asked := 0
		c := client(&stubConn{execXML: func(string) (string, error) {
			asked++
			if asked > leaveAttempts*2 {
				return "", errors.New("the port is gone")
			}
			return menuDoc("Settings"), nil
		}})

		_, err := leaveMenus(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "somewhere it could not be taken out of") {
			t.Fatalf("error = %v, want the unknown screen", err)
		}
	})
}

// Test_leaveQuickSearch tests the leaveQuickSearch function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Left: the scanner redraws on another screen and nothing more is needed
//   - PressError: a failed key press is reported
//   - Cancelled: a cancelled run is reported rather than waited out
//   - StillThere: a scanner still in quick search at the end is reported
func Test_leaveQuickSearch(t *testing.T) {
	quicken(t, 2)

	// Verify that a scanner which redraws elsewhere needs nothing more.
	t.Run("Left", func(t *testing.T) {
		pressed := ""
		c := client(&stubConn{
			exec:    func(command string) (string, error) { pressed = command; return "", nil },
			execXML: func(string) (string, error) { return scannerDoc("Scan Mode", "conventional"), nil },
		})

		if err := leaveQuickSearch(context.Background(), c); err != nil {
			t.Fatalf("leaveQuickSearch() = %v, want nil", err)
		}
		if pressed != fmt.Sprintf("KEY,%s,%s", device.KeySoft1, device.KeyPress) {
			t.Fatalf("pressed %q, want the soft key", pressed)
		}
	})

	// Verify that a failed key press is reported.
	t.Run("PressError", func(t *testing.T) {
		c := client(&stubConn{
			exec: func(string) (string, error) { return "", errors.New("the scanner refused the key") },
		})

		err := leaveQuickSearch(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "leaving quick search") {
			t.Fatalf("error = %v, want the failed press", err)
		}
	})

	// Verify that a cancelled run stops rather than being waited out.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := client(&stubConn{execXML: func(string) (string, error) {
			cancel()
			return scannerDoc("Quick Search Hold", quickSearch), nil
		}})

		if err := leaveQuickSearch(ctx, c); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want a cancelled context", err)
		}
	})

	// Verify that a scanner still in quick search at the end is reported.
	t.Run("StillThere", func(t *testing.T) {
		c := client(&stubConn{execXML: func(string) (string, error) {
			return scannerDoc("Quick Search Hold", quickSearch), nil
		}})

		err := leaveQuickSearch(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "still holding one frequency") {
			t.Fatalf("error = %v, want it to say the scanner is still there", err)
		}
	})
}

// Test_matches tests the matches function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Rows: only a whole name, or a shortened one the wanted name starts with,
//     is the entry wanted
func Test_matches(t *testing.T) {
	// Verify that only the right rows count as the entry wanted.
	t.Run("Rows", func(t *testing.T) {
		for _, tc := range []struct {
			shown row
			want  string
			ok    bool
		}{
			{row{text: "Settings"}, "settings", true},
			{row{text: "Quick Save Favorites L", cut: true}, "Quick Save Favorites List", true},
			{row{text: "TEST CH"}, "TEST CH 2", false},
			{row{text: "", cut: true}, "Settings", false},
			{row{text: "Volume", cut: true}, "Settings", false},
		} {
			if got := matches(tc.shown, tc.want); got != tc.ok {
				t.Fatalf("matches(%+v, %q) = %v, want %v", tc.shown, tc.want, got, tc.ok)
			}
		}
	})
}

// Test_quote tests the quote function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Names: every name comes back in quotes
func Test_quote(t *testing.T) {
	// Verify that every name comes back in quotes.
	t.Run("Names", func(t *testing.T) {
		got := quote([]string{"Volume", "Squelch"})

		if strings.Join(got, ", ") != `"Volume", "Squelch"` {
			t.Fatalf("quote() = %v, want each name quoted", got)
		}
		if len(quote(nil)) != 0 {
			t.Fatal("quote(nil) returned names, want none")
		}
	})
}

// Test_renderMenu tests the renderMenu function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Items: the entries are written as a table, with the highlighted one
//     marked and a missing index shown as a dash
//   - NoItems: an input screen is explained rather than shown as an empty table
//   - WriteError: a stream that refuses the table is reported
func Test_renderMenu(t *testing.T) {
	// Verify that the entries are written as a table.
	t.Run("Items", func(t *testing.T) {
		app, out, _ := newApp(t, &stubConn{})

		r := Report{
			Title: "Menu",
			Items: []Item{
				{Name: "Volume", Index: "1"},
				{Name: "Settings", Highlighted: true},
			},
		}
		if err := renderMenu(app, r); err != nil {
			t.Fatalf("renderMenu() = %v, want nil", err)
		}

		got := out.String()
		if !strings.Contains(got, "menu: Menu") {
			t.Fatalf("output = %q, want the menu's own heading", got)
		}
		if !strings.Contains(got, "> ") || !strings.Contains(got, "-") {
			t.Fatalf("output = %q, want the marker and the missing index", got)
		}
	})

	// Verify that an input screen is explained rather than shown empty.
	t.Run("NoItems", func(t *testing.T) {
		app, _, errs := newApp(t, &stubConn{})

		if err := renderMenu(app, Report{Title: "Name"}); err != nil {
			t.Fatalf("renderMenu() = %v, want nil", err)
		}
		if !strings.Contains(errs.String(), "no entries to choose from") {
			t.Fatalf("notes = %q, want the input screen explained", errs.String())
		}
	})

	// Verify that a stream which refuses the table is reported.
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp(t, &stubConn{})
		app.Stdout = failingWriter{}

		r := Report{Title: "Menu", Items: []Item{{Name: "Volume", Index: "1"}}}
		err := renderMenu(app, r)
		if err == nil || !strings.Contains(err.Error(), "writing the menu") {
			t.Fatalf("error = %v, want the failed write", err)
		}
	})
}

// TestFullEntries tests the FullEntries function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: every entry is read off the screen and given the index the
//     scanner's own listing mentioned it under
//   - Unlisted: an entry the listing never mentions comes back without an index
//   - FirstReadError: a screen that cannot be read at the start is reported
//   - TurnError: a knob the scanner will not turn is reported
func TestFullEntries(t *testing.T) {
	quicken(t, 2)

	// Verify that the names come off the display and the indexes off the
	// listing, which is the whole point of reading both.
	t.Run("Success", func(t *testing.T) {
		var pressed []string
		conn := menu([]string{"Volume", "Squelch"}, &pressed)
		conn.execXML = func(command string) (string, error) {
			return menuDoc("Settings", "Volume", "Squelch"), nil
		}

		got, err := FullEntries(context.Background(), client(conn))
		if err != nil {
			t.Fatalf("FullEntries() = %v, want nil", err)
		}
		if len(got) != 2 {
			t.Fatalf("read %d entries, want the two the menu holds", len(got))
		}
		if got[0].Name != "Volume" || got[0].Index != "1" {
			t.Errorf("first entry = %+v, want Volume with the listing's index", got[0])
		}
		if got[1].Name != "Squelch" || got[1].Index != "2" {
			t.Errorf("second entry = %+v, want Squelch with the listing's index", got[1])
		}
	})

	// Verify that a row the listing leaves out keeps its name and gets no
	// index, rather than borrowing one from another row.
	t.Run("Unlisted", func(t *testing.T) {
		var pressed []string
		conn := menu([]string{"Volume", "Squelch"}, &pressed)
		conn.execXML = func(command string) (string, error) {
			return menuDoc("Settings", "Volume"), nil
		}

		got, err := FullEntries(context.Background(), client(conn))
		if err != nil {
			t.Fatalf("FullEntries() = %v, want nil", err)
		}
		if got[1].Index != "" {
			t.Errorf("second entry index = %q, want none: the listing never mentioned it", got[1].Index)
		}
	})

	// Verify that a screen that cannot be read stops the walk rather than
	// producing an entry list built on nothing.
	t.Run("FirstReadError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if _, err := FullEntries(context.Background(), client(conn)); err == nil {
			t.Error("expected an error when the screen cannot be read, got none")
		}
	})

	// Verify that a knob press the scanner refuses is reported.
	t.Run("TurnError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			if command == "STS" {
				return selected("Volume"), nil
			}
			return "", errors.New("the knob will not turn")
		}}

		if _, err := FullEntries(context.Background(), client(conn)); err == nil {
			t.Error("expected an error when the knob will not turn, got none")
		}
	})
}

// TestMatches tests the Matches function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Exact: a name the screen showed in full matches it
//   - Cut: a name the screen shortened matches the name it is the start of
//   - Different: a name the screen showed in full matches nothing longer
func TestMatches(t *testing.T) {
	// Verify that a whole name matches the name it is.
	t.Run("Exact", func(t *testing.T) {
		if !Matches(Entry{Name: "CH 08"}, "CH 08") {
			t.Error("a whole name did not match itself")
		}
	})

	// Verify that a name the display had to shorten matches the longer name.
	t.Run("Cut", func(t *testing.T) {
		if !Matches(Entry{Name: "Quick Save Favorites L", Cut: true}, "Quick Save Favorites List") {
			t.Error("a shortened name did not match the name it is the start of")
		}
	})

	// Verify that a name shown in full is not treated as the start of a longer
	// one, which is what keeps a walk off the entry next to the one it wants.
	t.Run("Different", func(t *testing.T) {
		if Matches(Entry{Name: "TEST CH"}, "TEST CH 2") {
			t.Error("a whole name matched a longer name it is only the start of")
		}
	})
}

// Test_note tests the note function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Collects: the listing's rows are kept with their indexes
//   - Skips: a row with no name or no index is passed over
//   - Repeat: a row already gathered is not gathered twice
//   - Unreadable: a listing that cannot be read leaves what was gathered alone
func Test_note(t *testing.T) {
	// Verify that the listing's rows arrive with their indexes attached.
	t.Run("Collects", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return menuDoc("Settings", "Volume", "Squelch"), nil
		}}

		got := note(context.Background(), client(conn), nil)
		if len(got) != 2 || got[0].name != "Volume" || got[0].index != "1" {
			t.Fatalf("gathered %+v, want both rows of the listing", got)
		}
	})

	// Verify that a row carrying no index is no use and is passed over.
	t.Run("Skips", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return `<MenuInfo Name="Settings" MenuType="TypeSelect">` +
				`<MenuItem Name="Volume" Index=""/><MenuItem Name="" Index="2"/>` +
				`</MenuInfo>`, nil
		}}

		if got := note(context.Background(), client(conn), nil); len(got) != 0 {
			t.Fatalf("gathered %+v, want nothing usable", got)
		}
	})

	// Verify that walking past the same row twice does not record it twice.
	t.Run("Repeat", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return menuDoc("Settings", "Volume"), nil
		}}

		got := note(context.Background(), client(conn), nil)
		got = note(context.Background(), client(conn), got)
		if len(got) != 1 {
			t.Fatalf("gathered %+v, want the row kept once", got)
		}
	})

	// Verify that a screen with no listing at all is not an error here: the
	// names are what the walk is for and the indexes are the bonus.
	t.Run("Unreadable", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return "", errors.New("this screen has no listing")
		}}

		before := []listed{{name: "Volume", index: "1"}}
		if got := note(context.Background(), client(conn), before); len(got) != 1 {
			t.Fatalf("gathered %+v, want what was already there", got)
		}
	})
}
