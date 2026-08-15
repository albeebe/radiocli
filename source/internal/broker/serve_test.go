// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package broker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/device"
)

// stubConn is a scanner that is not there, so an App can hold a connection
// without a serial port being opened.
type stubConn struct{}

// Info describes the scanner behind this connection, which is nothing.
func (*stubConn) Info() device.Info { return device.Info{} }

// Execute answers any command with an empty success.
func (*stubConn) Execute(ctx context.Context, command string) (string, error) { return "", nil }

// ExecuteXML answers any command with an empty document.
func (*stubConn) ExecuteXML(ctx context.Context, command string) (string, error) { return "", nil }

// Send accepts any command without doing anything with it.
func (*stubConn) Send(ctx context.Context, command string) error { return nil }

// Close gives back a port that was never opened.
func (*stubConn) Close() error { return nil }

// shortDir makes a directory a unix socket path still fits inside.
//
// Not t.TempDir, and the difference matters on macOS: a socket path is limited
// to about a hundred bytes there and the name t.TempDir builds from the test's
// own name is most of that already.
func shortDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// useSocketDir points the socket the daemon binds at a directory of this
// test's own, and puts the real ones back afterwards.
func useSocketDir(t *testing.T, dir string) {
	t.Helper()

	savedPath, savedDir := socketPath, socketDir
	t.Cleanup(func() { socketPath, socketDir = savedPath, savedDir })

	socketDir = func() string { return dir }
	socketPath = func(port string) string { return filepath.Join(dir, "s") }
}

// daemonApp builds an App holding a scanner that is not there, with its
// streams thrown away.
func daemonApp(t *testing.T, run func(ctx context.Context, argv []string) error) *appcontext.App {
	t.Helper()

	app := appcontext.New()
	app.Config = appcontext.Defaults()
	app.Log = slog.New(slog.DiscardHandler)
	app.Stdout, app.Stderr = io.Discard, io.Discard
	app.RunCommand = run
	app.SetDevice(device.New(&stubConn{}))
	return app
}

// waitForSocket waits until something is listening on path, so a test does not
// race the daemon it just started.
func waitForSocket(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.Dial("unix", path)
		if err == nil {
			c.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("nothing ever listened on %s", path)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestServe tests the Serve function with 100% coverage.
//
// Coverage: 100% (9 test cases covering all branches)
//
// Test cases:
//   - NoPort: a daemon with no port named is refused
//   - NoScanner: a scanner that will not open is reported
//   - NoSocketDirectory: a socket directory that cannot be made is reported
//   - LeftoverInTheWay: something in the socket's place that will not clear is reported
//   - NoListener: a socket that cannot be bound is reported
//   - DirectoryNotRestricted: a socket directory that cannot be made private is reported
//   - NotRestricted: a socket that cannot be restricted is reported
//   - NoAudio: a sound input that does not resolve is reported and stepped over
//   - Serves: the daemon holds the port, answers a client, and stops when told
func TestServe(t *testing.T) {
	// Verify that a daemon with no port named is refused.
	t.Run("NoPort", func(t *testing.T) {
		err := Serve(context.Background(), daemonApp(t, nil), "", Options{})
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("serving without a port gave %v", err)
		}
	})

	// Verify that a scanner that will not open is reported.
	t.Run("NoScanner", func(t *testing.T) {
		app := appcontext.New()
		app.Config = appcontext.Defaults()
		app.Log = slog.New(slog.DiscardHandler)
		app.Stdout, app.Stderr = io.Discard, io.Discard

		err := Serve(context.Background(), app, "port0", Options{})
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("serving without a scanner gave %v", err)
		}
	})

	// Verify that a socket directory that cannot be made is reported.
	t.Run("NoSocketDirectory", func(t *testing.T) {
		dir := shortDir(t)
		useSocketDir(t, dir)

		// A directory cannot be made underneath a regular file.
		blocked := filepath.Join(dir, "f")
		if err := os.WriteFile(blocked, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		socketDir = func() string { return filepath.Join(blocked, "d") }

		err := Serve(context.Background(), daemonApp(t, nil), "port0", Options{})
		if err == nil || !strings.Contains(err.Error(), "making ") {
			t.Fatalf("a socket directory that could not be made gave %v", err)
		}
	})

	// Verify that something in the socket's place that will not clear is reported.
	t.Run("LeftoverInTheWay", func(t *testing.T) {
		dir := shortDir(t)
		useSocketDir(t, dir)

		// A directory with something in it refuses to be removed, which is a
		// failure to clear rather than a socket that was not there.
		inTheWay := filepath.Join(dir, "s")
		if err := os.MkdirAll(filepath.Join(inTheWay, "child"), 0o755); err != nil {
			t.Fatal(err)
		}

		err := Serve(context.Background(), daemonApp(t, nil), "port0", Options{})
		if err == nil || !strings.Contains(err.Error(), "clearing ") {
			t.Fatalf("a leftover that would not clear gave %v", err)
		}
	})

	// Verify that a socket that cannot be bound is reported.
	t.Run("NoListener", func(t *testing.T) {
		dir := shortDir(t)
		useSocketDir(t, dir)

		// Longer than a unix socket path may be, so binding it is refused.
		socketPath = func(string) string {
			return filepath.Join(dir, strings.Repeat("x", 200))
		}

		err := Serve(context.Background(), daemonApp(t, nil), "port0", Options{})
		if err == nil || !strings.Contains(err.Error(), "listening on ") {
			t.Fatalf("a socket that could not be bound gave %v", err)
		}
	})

	// Verify that a socket directory that cannot be made private is reported.
	// It is restricted before anything is bound in it, because a socket living
	// in a directory anybody can reach is reachable for as long as it takes to
	// restrict it.
	t.Run("DirectoryNotRestricted", func(t *testing.T) {
		dir := shortDir(t)
		useSocketDir(t, dir)

		saved := chmodPath
		t.Cleanup(func() { chmodPath = saved })
		chmodPath = func(path string, _ os.FileMode) error {
			if path == dir {
				return errors.New("the directory would not take it")
			}
			return nil
		}

		err := Serve(context.Background(), daemonApp(t, nil), "port0", Options{})
		if err == nil || !strings.Contains(err.Error(), "restricting "+dir) {
			t.Fatalf("a directory that could not be restricted gave %v", err)
		}
	})

	// Verify that a socket that cannot be restricted is reported.
	t.Run("NotRestricted", func(t *testing.T) {
		dir := shortDir(t)
		useSocketDir(t, dir)

		saved := chmodPath
		t.Cleanup(func() { chmodPath = saved })
		chmodPath = func(path string, _ os.FileMode) error {
			if path == dir {
				return nil
			}
			return errors.New("the socket would not take it")
		}

		err := Serve(context.Background(), daemonApp(t, nil), "port0", Options{})
		if err == nil || !strings.Contains(err.Error(), "restricting ") {
			t.Fatalf("a socket that could not be restricted gave %v", err)
		}
	})

	// Verify that a sound input that does not resolve is reported and stepped over.
	t.Run("NoAudio", func(t *testing.T) {
		dir := shortDir(t)
		useSocketDir(t, dir)

		saved := resolveAudioSource
		t.Cleanup(func() { resolveAudioSource = saved })
		resolveAudioSource = func(string) (string, error) { return "", errors.New("nothing is called that") }

		var notes strings.Builder
		app := daemonApp(t, nil)
		app.Stderr = &notes

		ctx, stop := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- Serve(ctx, app, "port0", Options{Audio: "Line In"}) }()

		waitForSocket(t, filepath.Join(dir, "s"))
		stop()

		if err := <-done; err != nil {
			t.Fatalf("a daemon that was told to stop gave %v", err)
		}
		if !strings.Contains(notes.String(), "No audio") {
			t.Fatalf("the daemon never mentioned the missing audio, it said %q", notes.String())
		}
	})

	// Verify that the daemon holds the port, answers a client, and stops when told.
	t.Run("Serves", func(t *testing.T) {
		dir := shortDir(t)
		useSocketDir(t, dir)

		saved := resolveAudioSource
		t.Cleanup(func() { resolveAudioSource = saved })
		resolveAudioSource = func(name string) (string, error) { return name, nil }

		savedStart := audiofeedStart
		t.Cleanup(func() { audiofeedStart = savedStart })
		audiofeedStart = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
			return &fakeCapture{source: opts.Source, channel: opts.Channel}, nil
		}

		app := daemonApp(t, func(ctx context.Context, argv []string) error { return nil })
		app.Reader = app.Borrow

		orphaned := make(chan struct{})
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- Serve(ctx, app, "port0", Options{
				Audio: "Line In", AudioChannel: audiofeed.ChannelLeft, Orphaned: orphaned,
			})
		}()

		waitForSocket(t, filepath.Join(dir, "s"))

		client, err := Dial("port0")
		if err != nil {
			t.Fatalf("dialling the daemon: %v", err)
		}
		if _, err := client.Run(context.Background(), []string{"version"}, ModeQueue, io.Discard, io.Discard); err != nil {
			t.Fatalf("running a command: %v", err)
		}
		client.Close()

		stop()
		if err := <-done; err != nil {
			t.Fatalf("a daemon that was told to stop gave %v", err)
		}
	})
}

// Test_accept tests the accept method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Refused: a listener that fails for a reason other than shutting down is reported
//   - Stopped: a cancelled daemon returns without reporting anything
func Test_accept(t *testing.T) {
	// listen makes a socket in a directory of this test's own.
	listen := func(t *testing.T) (net.Listener, string) {
		t.Helper()

		path := filepath.Join(shortDir(t), "s")
		listener, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		return listener, path
	}

	// Verify that a listener that fails for a reason other than shutting down is reported.
	t.Run("Refused", func(t *testing.T) {
		listener, path := listen(t)
		listener.Close()

		s := &Server{app: daemonApp(t, nil), log: slog.New(slog.DiscardHandler)}
		err := s.accept(context.Background(), listener, path)
		if err == nil || !strings.Contains(err.Error(), "accepting a connection") {
			t.Fatalf("a listener that failed gave %v", err)
		}
	})

	// Verify that a cancelled daemon returns without reporting anything.
	t.Run("Stopped", func(t *testing.T) {
		listener, path := listen(t)

		app := daemonApp(t, func(ctx context.Context, argv []string) error { return nil })
		s := &Server{app: app, run: runner{app: app}, log: slog.New(slog.DiscardHandler)}

		ctx, stop := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- s.accept(ctx, listener, path) }()

		client, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		client.Close()

		stop()
		if err := <-done; err != nil {
			t.Fatalf("a cancelled daemon gave %v", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the socket was left behind: %v", err)
		}
	})
}
