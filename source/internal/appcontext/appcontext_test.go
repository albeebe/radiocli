// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package appcontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/portlock"
)

// stubConn is a transport that reads and writes nothing, so a Scanner can be
// built and closed without a serial port.
type stubConn struct {
	// closeErr is what Close answers, for the tests about a failing shutdown.
	closeErr error
}

// Info reports what the transport is connected to, which here is nothing.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute answers a command with an empty reply.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) { return "", nil }

// ExecuteXML answers a command with an empty reply.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) { return "", nil }

// Send accepts a command and does nothing with it.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close reports whatever the test asked it to.
func (c *stubConn) Close() error { return c.closeErr }

// TestDeviceAppliesPaceToACachedConnection covers the case where one process
// runs several invocations against one connection, which is what daemon does.
//
// The settings are resolved once per invocation, but the connection is opened
// once and kept. Applying the pace only where the connection is opened means
// the first command to reach the scanner fixes the pace for every command
// after it, and their --pace is accepted, validated, and then ignored. That is
// silent and it is wrong, so it is worth a test rather than a comment.
func TestDeviceAppliesPaceToACachedConnection(t *testing.T) {
	app := New()

	// The first invocation asks for slow and opens the connection.
	app.Config.Pace = device.PaceSlow

	// The transport is never touched: handing back a cached connection reads
	// no bytes, so a nil Conn is enough to test this.
	client := device.New(nil)
	app.SetDevice(client)

	if got := client.Pace(); got != device.PaceSlow {
		t.Fatalf("after SetDevice, pace is %q, want %q", got, device.PaceSlow)
	}

	// A later invocation in the same process resolves its own settings and
	// asks for something else.
	app.Config.Pace = device.PaceTurbo

	got, err := app.Device(context.Background())
	if err != nil {
		t.Fatalf("Device: %v", err)
	}
	if got != client {
		t.Fatal("Device opened a new connection instead of reusing the cached one")
	}
	if pace := client.Pace(); pace != device.PaceTurbo {
		t.Errorf("pace is %q, want %q: a cached connection has to take the pace of the invocation using it", pace, device.PaceTurbo)
	}
}

// TestDeviceKeepsWorkingPaceWhenTheNewOneIsInvalid checks that a bad pace is a
// warning rather than a failure, since the command can still run.
func TestDeviceKeepsWorkingPaceWhenTheNewOneIsInvalid(t *testing.T) {
	app := New()
	app.Config.Pace = device.PaceFast

	client := device.New(nil)
	app.SetDevice(client)

	app.Config.Pace = device.Pace("nonsense")

	if _, err := app.Device(context.Background()); err != nil {
		t.Fatalf("Device: %v", err)
	}
	if pace := client.Pace(); pace != device.PaceFast {
		t.Errorf("pace is %q, want the previous %q to survive an invalid one", pace, device.PaceFast)
	}
}

// configIn writes a config file into a directory of this test's own and
// returns its path, so a run never reads or writes the config of the person
// running it.
func configIn(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seeding the config file: %v", err)
	}
	return path
}

// makeReadOnly takes the write permission off a file or a directory, and puts
// it back afterwards so the temporary directory can still be cleaned up.
//
// The restriction is checked rather than assumed: a run as a user the
// permissions do not apply to would otherwise report a failure the code never
// had a chance to produce, so the test is skipped instead.
func makeReadOnly(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("looking at %s: %v", path, err)
	}

	restore := os.FileMode(0o600)
	if info.IsDir() {
		restore = 0o755
	}
	t.Cleanup(func() { _ = os.Chmod(path, restore) })

	locked := os.FileMode(0o400)
	if info.IsDir() {
		locked = 0o555
	}
	if err := os.Chmod(path, locked); err != nil {
		t.Fatalf("taking the write permission off %s: %v", path, err)
	}

	if info.IsDir() {
		probe := filepath.Join(path, "probe")
		if err := os.Mkdir(probe, 0o755); err == nil {
			_ = os.Remove(probe)
			t.Skip("this user can write to a read-only directory, so there is no failure to test")
		}
		return
	}

	if f, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		t.Skip("this user can write to a read-only file, so there is no failure to test")
	}
}

// useConfigDir points the default config location at a directory of this
// test's own, or at the failure a machine with no such directory gives, and
// puts the real lookup back afterwards.
func useConfigDir(t *testing.T, dir string, err error) {
	t.Helper()

	previous := userConfigDir
	userConfigDir = func() (string, error) { return dir, err }
	t.Cleanup(func() { userConfigDir = previous })
}

// useOpenDevice substitutes what opening the serial port answers, and puts the
// real one back afterwards.
func useOpenDevice(t *testing.T, client *device.Scanner, err error) {
	t.Helper()

	previous := openDevice
	openDevice = func(ctx context.Context, port string, wait time.Duration, log *slog.Logger) (*device.Scanner, error) {
		return client, err
	}
	t.Cleanup(func() { openDevice = previous })
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Defaults: the App comes back with the built-in settings, a logger that
//     writes nothing, and the process streams
func TestNew(t *testing.T) {
	// Verify that New wires the defaults and the process streams without
	// doing any I/O of its own.
	t.Run("Defaults", func(t *testing.T) {
		app := New()

		if app.Config.Output != OutputText {
			t.Errorf("output is %q, want %q", app.Config.Output, OutputText)
		}
		if app.Config.Pace != device.DefaultPace {
			t.Errorf("pace is %q, want %q", app.Config.Pace, device.DefaultPace)
		}
		if len(app.Config.Macros) == 0 {
			t.Error("a new App has no macros, want the built-in ones")
		}
		if app.Log == nil {
			t.Fatal("a new App has no logger")
		}
		if app.Stdout != os.Stdout || app.Stderr != os.Stderr || app.Stdin != os.Stdin {
			t.Error("a new App is not wired to the process streams")
		}
	})
}

// TestInit tests the Init function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Quiet: without verbose, debug lines are dropped
//   - Verbose: with verbose, debug lines are kept
func TestInit(t *testing.T) {
	// Verify that the logger built without verbose refuses debug lines and
	// writes to Stderr.
	t.Run("Quiet", func(t *testing.T) {
		var errs bytes.Buffer
		app := New()
		app.Stderr = &errs

		if err := app.Init(context.Background()); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if app.Log.Enabled(context.Background(), slog.LevelDebug) {
			t.Error("debug logging is on without --verbose")
		}

		app.Log.Info("a note")
		if !strings.Contains(errs.String(), "a note") {
			t.Errorf("the log went to %q, want it on Stderr", errs.String())
		}
	})

	// Verify that verbose turns debug logging on.
	t.Run("Verbose", func(t *testing.T) {
		var errs bytes.Buffer
		app := New()
		app.Stderr = &errs
		app.Config.Verbose = true

		if err := app.Init(context.Background()); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if !app.Log.Enabled(context.Background(), slog.LevelDebug) {
			t.Error("debug logging is off with --verbose")
		}
	})
}

// TestBorrow tests the Borrow function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - SharesTheScannerAndCopiesTheSettings: the connection is shared, the
//     settings are copied, and the borrowed mark is set
func TestBorrow(t *testing.T) {
	// Verify that the two Apps share one connection and cannot change each
	// other's settings.
	t.Run("SharesTheScannerAndCopiesTheSettings", func(t *testing.T) {
		app := New()
		app.Config.Pace = device.PaceTurbo
		app.InDaemon = true

		client := device.New(&stubConn{})
		app.SetDevice(client)

		borrowed := app.Borrow()
		if !borrowed.borrowed {
			t.Error("the borrowed App is not marked as borrowed")
		}
		if !borrowed.InDaemon {
			t.Error("the borrowed App lost the daemon mark")
		}
		if borrowed.device != client {
			t.Error("the borrowed App does not share the connection")
		}
		if borrowed.Config == app.Config {
			t.Fatal("the borrowed App shares the settings, want a copy")
		}

		borrowed.Config.Output = OutputJSON
		if app.Config.Output != OutputText {
			t.Errorf("the lender's output is now %q: the settings are shared rather than copied", app.Config.Output)
		}

		// A borrowed App presses no keys, so it has no business changing the
		// pace of a connection somebody else is part way through using.
		borrowed.Config.Pace = device.PaceSlow
		if _, err := borrowed.Device(context.Background()); err != nil {
			t.Fatalf("Device: %v", err)
		}
		if pace := client.Pace(); pace != device.PaceTurbo {
			t.Errorf("the pace is %q, want the lender's %q to survive a borrower asking for something else", pace, device.PaceTurbo)
		}
	})
}

// TestDevice tests the Device function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - NoDevice: no scanner was named
//   - Busy: another invocation holds the port, and the advice is left alone
//   - Unreachable: any other failure gains the advice about what is attached
//   - Opened: the connection is paced, cached and closed with the App
func TestDevice(t *testing.T) {
	// Verify that a command with no --device is told to name one.
	t.Run("NoDevice", func(t *testing.T) {
		app := New()

		_, err := app.Device(context.Background())
		if !errors.Is(err, ErrNoDevice) {
			t.Fatalf("Device answered %v, want ErrNoDevice", err)
		}
		if !strings.Contains(err.Error(), "--device") {
			t.Errorf("the refusal said %q, which does not name the flag", err)
		}
	})

	// Verify that a port held by another invocation keeps its own advice,
	// since picking a different scanner is the wrong answer.
	t.Run("Busy", func(t *testing.T) {
		app := New()
		app.Config.Device = "/dev/tty.usbmodem14201"
		useOpenDevice(t, nil, portlock.ErrBusy)

		_, err := app.Device(context.Background())
		if !errors.Is(err, portlock.ErrBusy) {
			t.Fatalf("Device answered %v, want portlock.ErrBusy", err)
		}
		if strings.Contains(err.Error(), "to see what is attached") {
			t.Errorf("the busy error said %q, which adds advice about picking another scanner", err)
		}
	})

	// Verify that any other failure gains the advice about what is attached.
	t.Run("Unreachable", func(t *testing.T) {
		app := New()
		app.Config.Device = "/dev/tty.usbmodem14201"
		useOpenDevice(t, nil, errors.New("the port is gone"))

		_, err := app.Device(context.Background())
		if err == nil {
			t.Fatal("Device opened a port that is not there")
		}
		if !strings.Contains(err.Error(), "the port is gone") {
			t.Errorf("the failure said %q, which loses what went wrong", err)
		}
		if !strings.Contains(err.Error(), "to see what is attached") {
			t.Errorf("the failure said %q, which does not say how to find a scanner", err)
		}
	})

	// Verify that an opened connection is paced, cached, and closed with the
	// App that opened it.
	t.Run("Opened", func(t *testing.T) {
		closed := errors.New("already closed")
		client := device.New(&stubConn{closeErr: closed})

		app := New()
		app.Config.Device = "/dev/tty.usbmodem14201"
		app.Config.Pace = device.PaceSlow
		useOpenDevice(t, client, nil)

		got, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("Device: %v", err)
		}
		if got != client {
			t.Fatal("Device handed back a connection other than the one it opened")
		}
		if pace := client.Pace(); pace != device.PaceSlow {
			t.Errorf("the pace is %q, want the resolved %q", pace, device.PaceSlow)
		}

		// Cached, so a command that asks repeatedly still gets one port.
		again, err := app.Device(context.Background())
		if err != nil {
			t.Fatalf("Device: %v", err)
		}
		if again != client {
			t.Error("Device opened a second port instead of reusing the first")
		}

		if err := app.Close(); !errors.Is(err, closed) {
			t.Errorf("Close answered %v, want the connection to have been closed with the App", err)
		}
	})
}

// TestSetDevice tests the SetDevice function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Installed: the connection is paced, cached and registered for shutdown
func TestSetDevice(t *testing.T) {
	// Verify that an already open connection is used rather than reopened,
	// and is closed with the App.
	t.Run("Installed", func(t *testing.T) {
		closed := errors.New("already closed")
		client := device.New(&stubConn{closeErr: closed})

		app := New()
		app.Config.Pace = device.PaceMedium
		app.SetDevice(client)

		if app.device != client {
			t.Fatal("the connection was not cached")
		}
		if pace := client.Pace(); pace != device.PaceMedium {
			t.Errorf("the pace is %q, want %q", pace, device.PaceMedium)
		}
		if err := app.Close(); !errors.Is(err, closed) {
			t.Errorf("Close answered %v, want the installed connection to be closed", err)
		}
	})
}

// TestOnClose tests the OnClose function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Registered: a shutdown function is kept for Close to run
func TestOnClose(t *testing.T) {
	// Verify that a registered function runs when the App is closed.
	t.Run("Registered", func(t *testing.T) {
		app := New()

		var ran bool
		app.OnClose(func() error {
			ran = true
			return nil
		})

		if err := app.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if !ran {
			t.Error("the registered shutdown function did not run")
		}
	})
}

// TestClose tests the Close function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - ReverseOrder: every closer runs, in reverse, and the first failure is
//     the one reported
//   - Twice: a second Close has nothing left to run
func TestClose(t *testing.T) {
	// Verify that a failing closer does not stop the ones registered before
	// it, and that the first failure met on the way down is the one that
	// comes back.
	t.Run("ReverseOrder", func(t *testing.T) {
		app := New()

		var order []string
		broken := errors.New("the port is gone")
		app.OnClose(func() error {
			order = append(order, "first")
			return nil
		})
		app.OnClose(func() error {
			order = append(order, "second")
			return errors.New("a second failure")
		})
		app.OnClose(func() error {
			order = append(order, "third")
			return broken
		})

		err := app.Close()
		if !errors.Is(err, broken) {
			t.Errorf("Close answered %v, want the first failure %v", err, broken)
		}
		if strings.Join(order, ",") != "third,second,first" {
			t.Errorf("the closers ran %v, want them in reverse registration order", order)
		}
	})

	// Verify that closing twice is harmless, since the closers are dropped
	// once they have run.
	t.Run("Twice", func(t *testing.T) {
		app := New()

		var runs int
		app.OnClose(func() error {
			runs++
			return errors.New("the port is gone")
		})

		if err := app.Close(); err == nil {
			t.Fatal("Close reported nothing, want the closer's failure")
		}
		if err := app.Close(); err != nil {
			t.Errorf("a second Close answered %v, want nothing left to close", err)
		}
		if runs != 1 {
			t.Errorf("the closer ran %d times, want 1", runs)
		}
	})
}

// TestPrintf tests the Printf function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - ToStdout: the result goes to Stdout and nowhere else
func TestPrintf(t *testing.T) {
	// Verify that program output goes only to Stdout, which is what has to
	// stay parseable.
	t.Run("ToStdout", func(t *testing.T) {
		var out, errs bytes.Buffer
		app := &App{Stdout: &out, Stderr: &errs}

		app.Printf("battery: %d%%\n", 74)

		if got := out.String(); got != "battery: 74%\n" {
			t.Errorf("Stdout holds %q, want %q", got, "battery: 74%\n")
		}
		if errs.Len() != 0 {
			t.Errorf("Stderr holds %q, want nothing", errs.String())
		}
	})
}

// TestNotef tests the Notef function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - ToStderr: the note goes to Stderr, leaving Stdout alone
func TestNotef(t *testing.T) {
	// Verify that a note goes only to Stderr, so a program reading Stdout
	// never sees it.
	t.Run("ToStderr", func(t *testing.T) {
		var out, errs bytes.Buffer
		app := &App{Stdout: &out, Stderr: &errs}

		app.Notef("waiting for %s\n", "the scanner")

		if got := errs.String(); got != "waiting for the scanner\n" {
			t.Errorf("Stderr holds %q, want %q", got, "waiting for the scanner\n")
		}
		if out.Len() != 0 {
			t.Errorf("Stdout holds %q, want nothing", out.String())
		}
	})
}

// TestLoad tests the Load function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - Named: a file named with --config is read over the settings
//   - Default: the file in the user config directory is read
//   - MissingDefault: a missing default file is not a failure
//   - MissingNamed: a missing file the user named is a failure
//   - NoDirectory: the user config directory cannot be located
//   - Unreadable: the file cannot be read
//   - Unparseable: the file is not JSON, and the macros are checked first
func TestLoad(t *testing.T) {
	// Verify that a named file is read over the settings, and that the macros
	// it names replace the built-in ones rather than merging with them.
	t.Run("Named", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, `{"verbose":true,"output":"json","macros":[{"name":"Only","steps":["battery"]}]}`)

		if err := c.Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !c.Verbose {
			t.Error("verbose is off, want the file's value")
		}
		if c.Output != OutputJSON {
			t.Errorf("output is %q, want %q", c.Output, OutputJSON)
		}
		if len(c.Macros) != 1 || c.Macros[0].Name != "Only" {
			t.Errorf("the macros are %+v, want only the file's one", c.Macros)
		}
	})

	// Verify that a file naming no macros keeps the built-in ones, and that
	// the default location is where it is looked for.
	t.Run("Default", func(t *testing.T) {
		dir := t.TempDir()
		useConfigDir(t, dir, nil)

		if err := os.MkdirAll(filepath.Join(dir, "radiocli"), 0o755); err != nil {
			t.Fatalf("making the config directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "radiocli", "config.json"), []byte(`{"pace":"slow"}`), 0o600); err != nil {
			t.Fatalf("seeding the config file: %v", err)
		}

		c := defaultConfig()
		if err := c.Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Pace != device.PaceSlow {
			t.Errorf("pace is %q, want the file's %q", c.Pace, device.PaceSlow)
		}
		if len(c.Macros) != len(defaultMacros()) {
			t.Errorf("the file names no macros and %d came back, want the built-in ones", len(c.Macros))
		}
	})

	// Verify that no config file at all is a working tool rather than an
	// error.
	t.Run("MissingDefault", func(t *testing.T) {
		useConfigDir(t, t.TempDir(), nil)

		c := defaultConfig()
		if err := c.Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Output != OutputText {
			t.Errorf("output is %q, want the default %q", c.Output, OutputText)
		}
	})

	// Verify that a file named with --config has to be there, because naming
	// it says it was meant to be used.
	t.Run("MissingNamed", func(t *testing.T) {
		c := defaultConfig()
		c.Path = filepath.Join(t.TempDir(), "nowhere.json")

		err := c.Load()
		if err == nil {
			t.Fatal("Load accepted a file that is not there")
		}
		if !strings.Contains(err.Error(), "reading") {
			t.Errorf("the failure said %q, which does not say what it was doing", err)
		}
	})

	// Verify that a machine with no user config directory is reported rather
	// than guessed at.
	t.Run("NoDirectory", func(t *testing.T) {
		useConfigDir(t, "", errors.New("there is no home here"))

		err := defaultConfig().Load()
		if err == nil {
			t.Fatal("Load found a config directory that does not exist")
		}
		if !strings.Contains(err.Error(), "locating user config directory") {
			t.Errorf("the failure said %q, which does not say what could not be located", err)
		}
	})

	// Verify that a file that exists but cannot be read is reported, which a
	// directory in its place is the cheapest way to arrange.
	t.Run("Unreadable", func(t *testing.T) {
		c := defaultConfig()
		c.Path = t.TempDir()

		err := c.Load()
		if err == nil {
			t.Fatal("Load read a directory as a config file")
		}
		if !strings.Contains(err.Error(), "reading") {
			t.Errorf("the failure said %q, which does not say what it was doing", err)
		}
	})

	// Verify that both readings of the file are checked. The macros are read
	// on their own first, so a file that survives that and fails the second
	// reading has to be reported too.
	t.Run("Unparseable", func(t *testing.T) {
		broken := defaultConfig()
		broken.Path = configIn(t, `{"macros":`)

		err := broken.Load()
		if err == nil {
			t.Fatal("Load accepted a file that is not JSON")
		}
		if !strings.Contains(err.Error(), "parsing") {
			t.Errorf("the failure said %q, which does not say what it was doing", err)
		}

		// The macros here read cleanly, and the settings beside them do not:
		// an unknown field is skipped by the first reading and refused by the
		// second.
		typed := defaultConfig()
		typed.Path = configIn(t, `{"macros":[],"verbose":"yes"}`)

		err = typed.Load()
		if err == nil {
			t.Fatal("Load accepted a setting of the wrong type")
		}
		if !strings.Contains(err.Error(), "parsing") {
			t.Errorf("the failure said %q, which does not say what it was doing", err)
		}
	})
}

// TestValidate tests the Validate function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Text: the text format is usable
//   - JSON: the JSON format is usable
//   - BadOutput: a format that is neither is refused by name
//   - BadPace: a pace the scanner has no name for is refused
//   - NegativeWait: a wait shorter than nothing is refused
func TestValidate(t *testing.T) {
	// Verify that the settings a fresh install has are usable.
	t.Run("Text", func(t *testing.T) {
		if err := defaultConfig().Validate(); err != nil {
			t.Errorf("the built-in defaults are refused: %v", err)
		}
	})

	// Verify that the other output format is usable too.
	t.Run("JSON", func(t *testing.T) {
		c := defaultConfig()
		c.Output = OutputJSON

		if err := c.Validate(); err != nil {
			t.Errorf("JSON output is refused: %v", err)
		}
	})

	// Verify that an output format that is neither names both of the ones
	// there are.
	t.Run("BadOutput", func(t *testing.T) {
		c := defaultConfig()
		c.Output = OutputFormat("yaml")

		err := c.Validate()
		if err == nil {
			t.Fatal("an output format the tool cannot render was accepted")
		}
		if !strings.Contains(err.Error(), "invalid output format") {
			t.Errorf("the refusal said %q, which does not name the setting", err)
		}
	})

	// Verify that a pace the scanner has no name for is refused.
	t.Run("BadPace", func(t *testing.T) {
		c := defaultConfig()
		c.Pace = device.Pace("nonsense")

		err := c.Validate()
		if err == nil {
			t.Fatal("a pace that is not one was accepted")
		}
		if !strings.Contains(err.Error(), "invalid pace") {
			t.Errorf("the refusal said %q, which does not name the setting", err)
		}
	})

	// Verify that a negative wait is refused rather than treated as no wait
	// at all.
	t.Run("NegativeWait", func(t *testing.T) {
		c := defaultConfig()
		c.Wait = -time.Second

		err := c.Validate()
		if err == nil {
			t.Fatal("a wait shorter than nothing was accepted")
		}
		if !strings.Contains(err.Error(), "invalid wait") {
			t.Errorf("the refusal said %q, which does not name the setting", err)
		}
	})
}

// TestLocation tests the Location function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Named: the file named with --config is the answer
//   - Default: the file in the user config directory is the answer
//   - NoDirectory: the user config directory cannot be located
func TestLocation(t *testing.T) {
	// Verify that a file named with --config is the one reported.
	t.Run("Named", func(t *testing.T) {
		c := &Config{Path: filepath.Join(t.TempDir(), "elsewhere.json")}

		got, err := c.Location()
		if err != nil {
			t.Fatalf("Location: %v", err)
		}
		if got != c.Path {
			t.Errorf("the location is %q, want the named %q", got, c.Path)
		}
	})

	// Verify that the default location is the user config directory plus the
	// tool's own file, whether or not it exists.
	t.Run("Default", func(t *testing.T) {
		dir := t.TempDir()
		useConfigDir(t, dir, nil)

		got, err := (&Config{}).Location()
		if err != nil {
			t.Fatalf("Location: %v", err)
		}
		if want := filepath.Join(dir, "radiocli", "config.json"); got != want {
			t.Errorf("the location is %q, want %q", got, want)
		}
	})

	// Verify that a machine with no user config directory is reported.
	t.Run("NoDirectory", func(t *testing.T) {
		useConfigDir(t, "", errors.New("there is no home here"))

		if _, err := (&Config{}).Location(); err == nil {
			t.Fatal("Location found a config directory that does not exist")
		}
	})
}

// TestDefaults tests the Defaults function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Independent: the defaults come back untouched by any file or flag
func TestDefaults(t *testing.T) {
	// Verify that each call hands back settings of its own, so a caller that
	// changes them does not change what the next caller is told.
	t.Run("Independent", func(t *testing.T) {
		first := Defaults()
		if first.Output != OutputText || first.Pace != device.DefaultPace {
			t.Fatalf("the defaults came back as %+v", first)
		}

		first.Output = OutputJSON
		if second := Defaults(); second.Output != OutputText {
			t.Errorf("the defaults are now %q: they are shared rather than built each time", second.Output)
		}
	})
}

// TestSaved tests the Saved function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - WithoutThisRunsFlags: the file's settings come back rather than the
//     resolved ones
//   - Unreadable: a file that cannot be read is reported
func TestSaved(t *testing.T) {
	// Verify that the flags of this invocation are left out, since a one-off
	// "-o json" must not read back as a stored setting.
	t.Run("WithoutThisRunsFlags", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, `{"verbose":true}`)

		// As if the command line had asked for JSON this once.
		c.Output = OutputJSON

		got, err := c.Saved()
		if err != nil {
			t.Fatalf("Saved: %v", err)
		}
		if got.Output != OutputText {
			t.Errorf("the file reads back as %q output: this run's flag was written into the answer", got.Output)
		}
		if !got.Verbose {
			t.Error("verbose is off, want the file's value")
		}
		if got.Path != c.Path {
			t.Errorf("the answer names %q, want the file it was read from", got.Path)
		}
	})

	// Verify that a file that cannot be read is reported rather than
	// answered with the defaults.
	t.Run("Unreadable", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, `{"macros":`)

		if _, err := c.Saved(); err == nil {
			t.Fatal("Saved answered from a file that is not JSON")
		}
	})
}

// TestClone tests the Clone function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - SharesNothing: a change written through the copy's macros does not reach
//     the original
//   - NoMacros: settings that name no macros copy without one being invented
//   - MacroWithoutSteps: a macro holding no steps copies as it is
func TestClone(t *testing.T) {
	// Verify that the copy owns its macros. A plain struct copy passes the
	// first two checks here and fails the last two, which is the bug this
	// exists to stop.
	t.Run("SharesNothing", func(t *testing.T) {
		c := defaultConfig()
		c.Macros = []Macro{{Name: "listen", Steps: []string{"scan", "volume 5"}}}

		clone := c.Clone()
		if len(clone.Macros) != 1 {
			t.Fatalf("the copy holds %d macros, want 1", len(clone.Macros))
		}
		if clone.Macros[0].Name != "listen" {
			t.Errorf("the copy holds macro %q, want %q", clone.Macros[0].Name, "listen")
		}

		clone.Macros[0].Name = "other"
		clone.Macros[0].Steps[0] = "tune 162.550"

		if c.Macros[0].Name != "listen" {
			t.Errorf("the original macro is now named %q: the name is shared", c.Macros[0].Name)
		}
		if c.Macros[0].Steps[0] != "scan" {
			t.Errorf("the original first step is now %q: the steps are shared", c.Macros[0].Steps[0])
		}
	})

	// Verify that settings naming no macros stay that way. Copying nil as an
	// empty list would be the difference between "macros": null and
	// "macros": [], which Load and Update both read as meaning something.
	t.Run("NoMacros", func(t *testing.T) {
		c := defaultConfig()
		c.Macros = nil

		if clone := c.Clone(); clone.Macros != nil {
			t.Errorf("the copy holds %v macros, want none at all", clone.Macros)
		}
	})

	// Verify that a macro with no steps survives the copy. Nothing writes one,
	// but a hand-edited file can hold one, and Validate leaves macros alone.
	t.Run("MacroWithoutSteps", func(t *testing.T) {
		c := defaultConfig()
		c.Macros = []Macro{{Name: "empty"}}

		clone := c.Clone()
		if len(clone.Macros) != 1 {
			t.Fatalf("the copy holds %d macros, want 1", len(clone.Macros))
		}
		if clone.Macros[0].Steps != nil {
			t.Errorf("the copy gave the macro %v for steps, want none at all", clone.Macros[0].Steps)
		}
	})
}

// TestUpdate tests the Update function with 100% coverage.
//
// Coverage: 100% (8 test cases covering every branch)
//
// Test cases:
//   - WritesTheFileAndTheSettings: the change reaches disk and memory, and
//     this run's flags do not
//   - Creates: the file and its directory are made when they are missing
//   - NoDirectory: the user config directory cannot be located
//   - Unreadable: what is on disk cannot be read
//   - NoDirectoryToWriteIn: the directory cannot be created
//   - Unwritable: the file cannot be written
//   - Unencodable: the settings cannot be encoded
//   - LeavesMemoryAloneWhenTheWriteFails: a failed write does not change the
//     settings this run is using
func TestUpdate(t *testing.T) {
	// Verify that the change is written and applied, that unrelated settings
	// survive, and that the flags of this run are not written down.
	t.Run("WritesTheFileAndTheSettings", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, `{"verbose":true}`)

		// As if the command line had asked for JSON this once.
		c.Output = OutputJSON

		if err := c.Update(func(cfg *Config) { cfg.Pace = device.PaceSlow }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if c.Pace != device.PaceSlow {
			t.Errorf("the pace in memory is %q, want %q", c.Pace, device.PaceSlow)
		}

		data, err := os.ReadFile(c.Path)
		if err != nil {
			t.Fatalf("reading the config file back: %v", err)
		}

		var file Config
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatalf("the config file is not JSON: %v\n%s", err, data)
		}
		if file.Pace != device.PaceSlow {
			t.Errorf("the file holds pace %q, want %q", file.Pace, device.PaceSlow)
		}
		if !file.Verbose {
			t.Error("verbose is off in the file: an unrelated setting was lost")
		}
		if file.Output != OutputText {
			t.Errorf("the file holds output %q: a flag from this run was written down", file.Output)
		}
	})

	// Verify that a machine that has never been configured gets a directory
	// and a file rather than a failure.
	t.Run("Creates", func(t *testing.T) {
		dir := t.TempDir()
		useConfigDir(t, dir, nil)

		c := defaultConfig()
		if err := c.Update(func(cfg *Config) { cfg.Verbose = true }); err != nil {
			t.Fatalf("Update: %v", err)
		}

		if _, err := os.Stat(filepath.Join(dir, "radiocli", "config.json")); err != nil {
			t.Fatalf("the config file was not created: %v", err)
		}
	})

	// Verify that a machine with no user config directory is reported.
	t.Run("NoDirectory", func(t *testing.T) {
		useConfigDir(t, "", errors.New("there is no home here"))

		err := defaultConfig().Update(func(cfg *Config) { cfg.Verbose = true })
		if err == nil {
			t.Fatal("Update wrote to a config directory that does not exist")
		}
		if !strings.Contains(err.Error(), "locating user config directory") {
			t.Errorf("the failure said %q, which does not say what could not be located", err)
		}
	})

	// Verify that a file that cannot be read is reported before anything is
	// written, since what is on disk is where the change starts from.
	t.Run("Unreadable", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, `{"macros":`)

		if err := c.Update(func(cfg *Config) { cfg.Verbose = true }); err == nil {
			t.Fatal("Update wrote over a file it could not read")
		}
	})

	// Verify that a directory that cannot be made is reported, which a config
	// directory nothing may be created in is the way to arrange.
	t.Run("NoDirectoryToWriteIn", func(t *testing.T) {
		dir := t.TempDir()
		makeReadOnly(t, dir)
		useConfigDir(t, dir, nil)

		err := defaultConfig().Update(func(cfg *Config) { cfg.Verbose = true })
		if err == nil {
			t.Fatal("Update created a directory it has no permission to create")
		}
		if !strings.Contains(err.Error(), "creating") {
			t.Errorf("the failure said %q, which does not say what it was doing", err)
		}
	})

	// Verify that a file that cannot be written is reported, rather than the
	// change being reported as made when nothing reached the disk.
	t.Run("Unwritable", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, "{}")
		makeReadOnly(t, c.Path)

		err := c.Update(func(cfg *Config) { cfg.Verbose = true })
		if err == nil {
			t.Fatal("Update wrote a file it has no permission to write")
		}
		if !strings.Contains(err.Error(), "writing") {
			t.Errorf("the failure said %q, which does not say what it was doing", err)
		}
	})

	// Verify that settings which cannot be encoded are reported, rather than
	// an empty file being written over what was there.
	t.Run("Unencodable", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, "{}")

		marshalJSON = func(any, string, string) ([]byte, error) {
			return nil, errors.New("no")
		}
		t.Cleanup(func() { marshalJSON = json.MarshalIndent })

		err := c.Update(func(cfg *Config) { cfg.Verbose = true })
		if err == nil {
			t.Fatal("Update wrote settings it could not encode")
		}
		if !strings.Contains(err.Error(), "encoding") {
			t.Errorf("the failure said %q, which does not say what it was doing", err)
		}
	})

	// Verify that a write that fails leaves this run using what is still on
	// disk. Believing a setting that was never stored would have this run
	// behave one way and the next one, reading the file, behave another.
	t.Run("LeavesMemoryAloneWhenTheWriteFails", func(t *testing.T) {
		c := defaultConfig()
		c.Path = configIn(t, "{}")
		makeReadOnly(t, c.Path)

		if err := c.Update(func(cfg *Config) { cfg.Verbose = true }); err == nil {
			t.Fatal("Update wrote a file it has no permission to write")
		}
		if c.Verbose {
			t.Error("verbose is on in memory after a write that failed: the change was applied anyway")
		}
	})
}

// Test_applyPace tests the applyPace function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Applied: the resolved pace reaches the connection
//   - Refused: a pace the connection will not take is a warning rather than a
//     failure
func Test_applyPace(t *testing.T) {
	// Verify that the pace resolved for this invocation reaches the
	// connection.
	t.Run("Applied", func(t *testing.T) {
		app := New()
		app.Config.Pace = device.PaceMedium

		client := device.New(&stubConn{})
		app.applyPace(client)

		if pace := client.Pace(); pace != device.PaceMedium {
			t.Errorf("the pace is %q, want %q", pace, device.PaceMedium)
		}
	})

	// Verify that a pace that is not one leaves the connection working, since
	// the command can still run.
	t.Run("Refused", func(t *testing.T) {
		var errs bytes.Buffer
		app := New()
		app.Stderr = &errs
		app.Log = slog.New(slog.NewTextHandler(&errs, nil))
		app.Config.Pace = device.Pace("nonsense")

		client := device.New(&stubConn{})
		before := client.Pace()
		app.applyPace(client)

		if pace := client.Pace(); pace != before {
			t.Errorf("the pace is %q, want the working %q to survive", pace, before)
		}
		if !strings.Contains(errs.String(), "keeping the default key pace") {
			t.Errorf("the log holds %q, which does not warn about the pace", errs.String())
		}
	})
}

// Test_defaultConfig tests the defaultConfig function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - BuiltIn: the settings used when nothing else specifies them
func Test_defaultConfig(t *testing.T) {
	// Verify that the built-in settings are usable, which is what a machine
	// with no config file runs on.
	t.Run("BuiltIn", func(t *testing.T) {
		c := defaultConfig()

		if c.Output != OutputText {
			t.Errorf("output is %q, want %q", c.Output, OutputText)
		}
		if c.Pace != device.DefaultPace {
			t.Errorf("pace is %q, want %q", c.Pace, device.DefaultPace)
		}
		if c.Path != "" || c.Device != "" || c.Verbose || c.Wait != 0 {
			t.Errorf("the defaults hold an invocation's own settings: %+v", c)
		}
		if err := c.Validate(); err != nil {
			t.Errorf("the built-in settings are refused: %v", err)
		}
	})
}

// Test_defaultConfigPath tests the defaultConfigPath function with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Located: the user config directory plus the tool's own file
//   - NoDirectory: the user config directory cannot be located
func Test_defaultConfigPath(t *testing.T) {
	// Verify that the path is the user config directory plus the tool's own
	// directory and file.
	t.Run("Located", func(t *testing.T) {
		dir := t.TempDir()
		useConfigDir(t, dir, nil)

		got, err := defaultConfigPath()
		if err != nil {
			t.Fatalf("defaultConfigPath: %v", err)
		}
		if want := filepath.Join(dir, "radiocli", "config.json"); got != want {
			t.Errorf("the path is %q, want %q", got, want)
		}
	})

	// Verify that a machine with no user config directory is reported with
	// what was being looked for.
	t.Run("NoDirectory", func(t *testing.T) {
		useConfigDir(t, "", errors.New("there is no home here"))

		_, err := defaultConfigPath()
		if err == nil {
			t.Fatal("defaultConfigPath found a directory that does not exist")
		}
		if !strings.Contains(err.Error(), "locating user config directory") {
			t.Errorf("the failure said %q, which does not say what could not be located", err)
		}
	})
}

// Test_defaultMacros tests the defaultMacros function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Buttons: every built-in macro has a name and at least one step, and no
//     two share a name
func Test_defaultMacros(t *testing.T) {
	// Verify that each built-in button is a macro somebody could have typed:
	// a name that fits on a button and at least one command to run.
	t.Run("Buttons", func(t *testing.T) {
		macros := defaultMacros()
		if len(macros) == 0 {
			t.Fatal("there are no built-in macros")
		}

		seen := make(map[string]bool, len(macros))
		for _, m := range macros {
			if m.Name == "" {
				t.Errorf("a built-in macro running %q has no name", m.Steps)
			}
			if len(m.Steps) == 0 {
				t.Errorf("the built-in macro %q has no steps", m.Name)
			}
			if seen[strings.ToLower(m.Name)] {
				t.Errorf("two built-in macros are called %q", m.Name)
			}
			seen[strings.ToLower(m.Name)] = true
		}
	})
}
