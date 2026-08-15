// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package devices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/portlock"
)

// failWriter is a stream that refuses everything written to it, which is how
// the write failures are reached without a real broken pipe.
type failWriter struct{}

// Write reports that the stream has gone.
func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("the pipe closed") }

// newApp returns an App writing to buffers, along with the buffers, so a
// command's output and its advice can be read back separately.
func newApp() (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, notes := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, notes
	return app, out, notes
}

// serveDaemon listens on the socket a daemon for port would use, greeting
// whoever connects the way a daemon does.
//
// It is what makes "a busy port somebody is sharing" testable: hasDaemon
// answers by connecting, so the only way to make it say yes is for something
// to be listening.
//
// Parameters:
//   - t: the test the listener is cleaned up at the end of
//   - port: the serial port a daemon would be named after
//   - hello: the greeting to send, which decides whether the caller accepts it
func serveDaemon(t *testing.T, port string, hello broker.Response) {
	t.Helper()

	path := portlock.SocketPath(port)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making the socket directory: %v", err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listening on %s: %v", path, err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				line, _ := json.Marshal(hello)
				conn.Write(append(line, '\n'))

				// Held open until the caller hangs up, so a client that read
				// the greeting is never racing a close it did not ask for.
				io.Copy(io.Discard, conn)
			}(conn)
		}
	}()
}

// sockets points the daemon socket directory at a temporary one of its own.
//
// Nothing here may reach a daemon the developer is actually running, and
// nothing may leave a socket behind. The base is /tmp rather than the default
// temporary directory because macOS refuses a unix socket path much over 100
// bytes and the default is most of that by itself.
//
// Parameters:
//   - t: the test whose temporary directory the sockets are made in
func sockets(t *testing.T) {
	t.Helper()
	t.Setenv("TMPDIR", "/tmp")
	t.Setenv("TMPDIR", t.TempDir())
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (2 test cases covering the command and the closure it holds)
//
// Test cases:
//   - Wiring: the command carries the name, the alias and the argument rule
//   - Runs: executing the command lists what discovery found
func TestNew(t *testing.T) {
	// Verify that the command is described the way the tool wires it
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "devices" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "devices")
		}
		if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "list" {
			t.Errorf("the aliases are %v, wanted just \"list\"", cmd.Aliases)
		}
		if cmd.Args == nil {
			t.Error("the command accepts positional arguments, wanted none")
		}
	})

	// Verify that running the command reaches run, which is what the closure
	// New hands cobra exists to do
	t.Run("Runs", func(t *testing.T) {
		app, out, _ := newApp()

		original := discover
		t.Cleanup(func() { discover = original })
		discover = func(ctx context.Context, log *slog.Logger) ([]device.Info, []string, error) {
			return []device.Info{{Port: "/dev/tty.usbmodem1", Model: "SDS150"}}, nil, nil
		}

		cmd := New(app)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(out.String(), "SDS150") {
			t.Errorf("the listing is %q, wanted the model in it", out.String())
		}
	})
}

// Test_hasDaemon tests the hasDaemon function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both answers)
//
// Test cases:
//   - Listening: something answering as a daemon is reported as one
//   - Nothing: a port with no socket at all is not a daemon
func Test_hasDaemon(t *testing.T) {
	// Verify that a daemon which greets the connection is found
	t.Run("Listening", func(t *testing.T) {
		sockets(t)
		serveDaemon(t, "/dev/tty.usbmodem1", broker.Response{
			Type: broker.TypeHello, Protocol: broker.Version, Version: "test",
		})

		if !hasDaemon("/dev/tty.usbmodem1") {
			t.Error("the port has no daemon, wanted one")
		}
	})

	// Verify that a port nothing is listening for is not reported as shared
	t.Run("Nothing", func(t *testing.T) {
		sockets(t)

		if hasDaemon("/dev/tty.usbmodem9") {
			t.Error("the port has a daemon, wanted none")
		}
	})
}

// Test_renderDevices tests the renderDevices function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every column rule and the failure)
//
// Test cases:
//   - Empty: an empty listing is still a table, with its heading
//   - One: a single scanner fills all three columns
//   - Several: every scanner given appears, in the order given
//   - Busy: a port being held stands in for the model with a dash
//   - NoSerial: a port reporting no serial number gets a dash for it
//   - WriteError: a stream that refuses the table is reported
func Test_renderDevices(t *testing.T) {
	// Verify that nothing to list still prints the heading
	t.Run("Empty", func(t *testing.T) {
		app, out, _ := newApp()

		if err := renderDevices(app, nil); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if got := out.String(); got != "MODEL  SERIAL  PORT\n" {
			t.Errorf("the table is %q, wanted the heading alone", got)
		}
	})

	// Verify that one scanner is written across the three columns
	t.Run("One", func(t *testing.T) {
		app, out, _ := newApp()

		entries := []entry{{Info: device.Info{
			Port: "/dev/tty.usbmodem1", Model: "SDS150", Serial: "A900",
		}}}
		if err := renderDevices(app, entries); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		for _, want := range []string{"SDS150", "A900", "/dev/tty.usbmodem1"} {
			if !strings.Contains(got, want) {
				t.Errorf("the table is %q, wanted %q in it", got, want)
			}
		}
	})

	// Verify that several scanners are listed in the order they were given
	t.Run("Several", func(t *testing.T) {
		app, out, _ := newApp()

		entries := []entry{
			{Info: device.Info{Port: "/dev/tty.usbmodem1", Model: "SDS150", Serial: "A900"}},
			{Info: device.Info{Port: "/dev/tty.usbmodem2", Model: "SDS200", Serial: "B100"}},
		}
		if err := renderDevices(app, entries); err != nil {
			t.Fatalf("rendering: %v", err)
		}

		got := out.String()
		first, second := strings.Index(got, "SDS150"), strings.Index(got, "SDS200")
		if first < 0 || second < 0 {
			t.Fatalf("the table is %q, wanted both scanners in it", got)
		}
		if first > second {
			t.Errorf("the table is %q, wanted the scanners in the order given", got)
		}
	})

	// Verify that a busy port says nothing about a model it could not read
	t.Run("Busy", func(t *testing.T) {
		app, out, _ := newApp()

		entries := []entry{{Info: device.Info{Port: "/dev/tty.usbmodem1", Model: "SDS150"}, Busy: true}}
		if err := renderDevices(app, entries); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if strings.Contains(out.String(), "SDS150") {
			t.Errorf("the table is %q, wanted a dash in place of the model", out.String())
		}
		if !strings.Contains(out.String(), "-") {
			t.Errorf("the table is %q, wanted a dash in it", out.String())
		}
	})

	// Verify that a port reporting no serial number gets a dash for it
	t.Run("NoSerial", func(t *testing.T) {
		app, out, _ := newApp()

		entries := []entry{{Info: device.Info{Port: "/dev/tty.usbmodem1", Model: "SDS150"}}}
		if err := renderDevices(app, entries); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "SDS150  -") {
			t.Errorf("the table is %q, wanted a dash for the serial number", out.String())
		}
	})

	// Verify that a stream which cannot take the table says so
	t.Run("WriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}

		err := renderDevices(app, []entry{{Info: device.Info{Port: "/dev/tty.usbmodem1"}}})
		if err == nil || !strings.Contains(err.Error(), "writing the device list") {
			t.Errorf("the failure is %v, wanted the device list to be named", err)
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (9 test cases covering every combination of what is attached)
//
// Test cases:
//   - Found: scanners that answered are listed, with nothing to advise
//   - None: nothing attached is reported as the complete answer it is
//   - JSON: the listing is written as JSON when that is what was asked for
//   - DiscoverError: a failed search is passed back
//   - AllBusy: every port being held is told apart from none being attached
//   - SomeBusy: a port held alongside one that answered is named
//   - Shared: a port a daemon is holding is offered rather than complained of
//   - JSONWriteError: a stream that refuses the JSON is reported
//   - RenderError: a stream that refuses the table is reported
func Test_run(t *testing.T) {
	// fakeDiscover installs a search that answers with what it was given, and
	// restores the real one at the end of the test.
	fakeDiscover := func(t *testing.T, found []device.Info, busy []string, err error) {
		t.Helper()
		original := discover
		t.Cleanup(func() { discover = original })
		discover = func(ctx context.Context, log *slog.Logger) ([]device.Info, []string, error) {
			return found, busy, err
		}
	}

	// Verify that scanners which answered are listed and nothing is advised
	t.Run("Found", func(t *testing.T) {
		app, out, notes := newApp()
		fakeDiscover(t, []device.Info{
			{Port: "/dev/tty.usbmodem1", Model: "SDS150", Serial: "A900"},
		}, nil, nil)

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(out.String(), "SDS150") {
			t.Errorf("the listing is %q, wanted the model in it", out.String())
		}
		if notes.String() != "" {
			t.Errorf("the advice is %q, wanted none", notes.String())
		}
	})

	// Verify that nothing attached is reported as an answer rather than a failure
	t.Run("None", func(t *testing.T) {
		app, out, notes := newApp()
		fakeDiscover(t, nil, nil, nil)

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if out.String() != "" {
			t.Errorf("the listing is %q, wanted nothing printed", out.String())
		}
		if !strings.Contains(notes.String(), "no scanner found") {
			t.Errorf("the advice is %q, wanted it to say nothing was found", notes.String())
		}
	})

	// Verify that JSON is written when that is the format asked for
	t.Run("JSON", func(t *testing.T) {
		sockets(t)
		app, out, _ := newApp()
		app.Config.Output = appcontext.OutputJSON
		fakeDiscover(t, []device.Info{{Port: "/dev/tty.usbmodem1", Model: "SDS150"}},
			[]string{"/dev/tty.usbmodem2"}, nil)

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}

		var entries []entry
		if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
			t.Fatalf("reading the JSON back: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("the listing has %d entries, wanted 2", len(entries))
		}
		if entries[0].Model != "SDS150" || !entries[1].Busy {
			t.Errorf("the listing is %+v, wanted the scanner then the busy port", entries)
		}
	})

	// Verify that a search which failed is passed back rather than swallowed
	t.Run("DiscoverError", func(t *testing.T) {
		app, _, _ := newApp()
		fakeDiscover(t, nil, nil, errors.New("the port is gone"))

		err := run(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "the port is gone") {
			t.Errorf("the failure is %v, wanted the search's own error", err)
		}
	})

	// Verify that every port being held is told apart from none being attached
	t.Run("AllBusy", func(t *testing.T) {
		sockets(t)
		app, out, notes := newApp()
		fakeDiscover(t, nil, []string{"/dev/tty.usbmodem1"}, nil)

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if out.String() != "" {
			t.Errorf("the listing is %q, wanted nothing printed", out.String())
		}
		if !strings.Contains(notes.String(), "/dev/tty.usbmodem1") {
			t.Errorf("the advice is %q, wanted the busy port named", notes.String())
		}
	})

	// Verify that a held port alongside one that answered is named separately
	t.Run("SomeBusy", func(t *testing.T) {
		sockets(t)
		app, out, notes := newApp()
		fakeDiscover(t, []device.Info{{Port: "/dev/tty.usbmodem1", Model: "SDS150"}},
			[]string{"/dev/tty.usbmodem2"}, nil)

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(out.String(), "SDS150") {
			t.Errorf("the listing is %q, wanted the scanner that answered", out.String())
		}
		if !strings.Contains(notes.String(), "In use by another radiocli") {
			t.Errorf("the advice is %q, wanted the port reported as in use", notes.String())
		}
	})

	// Verify that a port a daemon is holding is offered rather than complained of
	t.Run("Shared", func(t *testing.T) {
		sockets(t)
		serveDaemon(t, "/dev/tty.usbmodem1", broker.Response{
			Type: broker.TypeHello, Protocol: broker.Version, Version: "test",
		})

		app, _, notes := newApp()
		fakeDiscover(t, nil, []string{"/dev/tty.usbmodem1"}, nil)

		if err := run(context.Background(), app); err != nil {
			t.Fatalf("running: %v", err)
		}
		if !strings.Contains(notes.String(), "Held by an radiocli daemon") {
			t.Errorf("the advice is %q, wanted the daemon named", notes.String())
		}
		if strings.Contains(notes.String(), "In use by another radiocli") {
			t.Errorf("the advice is %q, wanted no complaint about a shared port", notes.String())
		}
	})

	// Verify that a stream which cannot take the JSON says so
	t.Run("JSONWriteError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		app.Config.Output = appcontext.OutputJSON
		fakeDiscover(t, []device.Info{{Port: "/dev/tty.usbmodem1"}}, nil, nil)

		err := run(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "the pipe closed") {
			t.Errorf("the failure is %v, wanted the stream's own error", err)
		}
	})

	// Verify that a stream which cannot take the table says so
	t.Run("RenderError", func(t *testing.T) {
		app, _, _ := newApp()
		app.Stdout = failWriter{}
		fakeDiscover(t, []device.Info{{Port: "/dev/tty.usbmodem1"}}, nil, nil)

		err := run(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "writing the device list") {
			t.Errorf("the failure is %v, wanted the device list to be named", err)
		}
	})
}
