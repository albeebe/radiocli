// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package broker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// Serve holds the scanner on port and answers clients until ctx is cancelled.
//
// The port is claimed before the socket is created, and in that order on
// purpose. A socket that existed before the scanner was held would be a socket
// clients could find and submit to while the daemon was still discovering it
// could not have the radio at all, and every one of them would be told the
// scanner was busy by a daemon that never had it.
//
// Parameters:
//   - ctx: the daemon runs until this is cancelled
//   - app: the App every command runs against, and whose scanner is held
//   - listenPort: the serial port to hold and to name the socket after
//   - opts: the choices this daemon was started with
//
// Returns:
//   - error if no port was named, the scanner would not open, the socket could
//     not be created, or accepting connections failed
func Serve(ctx context.Context, app *appcontext.App, listenPort string, opts Options) error {
	if listenPort == "" {
		return fmt.Errorf("%w: name one with --device", appcontext.ErrNoDevice)
	}

	// Marked before anything can run a command, so that the one command which
	// never finishes can refuse to start here. A daemon lends a command its
	// streams for the length of the command, which works only because commands
	// end.
	app.InDaemon = true

	// Opening the scanner here rather than lazily is the difference between a
	// daemon and an ordinary command. Everything else defers it because most
	// invocations never touch the radio; this exists to hold it.
	if _, err := app.Device(ctx); err != nil {
		return err
	}

	path := socketPath(listenPort)
	// The directory is this account's own and is made private before anything
	// is created in it. Restricting the socket after binding it is too late by
	// exactly the time it takes: the socket exists, world-connectable, in a
	// shared temporary directory, for as long as the two calls are apart. A
	// private directory around it means there is no moment when anybody else
	// can reach it.
	//
	// The mode is set as well as passed, because MkdirAll leaves a directory
	// that already exists exactly as it found it. On a machine where the
	// temporary directory is shared, somebody else could have made this one
	// first and left it open; the chmod either corrects that or fails, and
	// failing is the right answer, since it means the directory is not ours.
	if err := os.MkdirAll(socketDir(), 0o700); err != nil {
		return fmt.Errorf("making %s: %w", socketDir(), err)
	}
	if err := chmodPath(socketDir(), 0o700); err != nil {
		return fmt.Errorf("restricting %s: %w", socketDir(), err)
	}

	// A socket left behind by a daemon that was killed would refuse to be
	// bound again. It is safe to remove because the port lock above is what
	// actually decides there is no other daemon: this process holds it, so
	// any socket still sitting here belongs to something that is gone.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clearing %s: %w", path, err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", path, err)
	}

	// The socket carries every command this tool can run, so it is restricted
	// to the account that started the daemon. The private directory above has
	// already made it unreachable by anybody else; this is the same rule stated
	// on the socket itself, so that it survives the directory being loosened by
	// something outside this program.
	if err := chmodPath(path, 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("restricting %s: %w", path, err)
	}

	s := &Server{app: app, run: runner{app: app}, log: app.Log}

	// Resolved now, opened later. Naming an input that is not there is a
	// mistake worth reporting while somebody is still looking at the terminal,
	// and asking what is attached opens nothing, so it costs no permission
	// prompt and no microphone indicator to check.
	//
	// A name that does not resolve is reported and stepped over rather than
	// being fatal. This daemon's job is the scanner; audio is something extra it
	// can also carry, and refusing to hold the radio because a sound card was
	// unplugged would be the wrong thing to take away.
	if opts.Audio != "" {
		source, err := resolveAudioSource(opts.Audio)
		if err != nil {
			app.Notef("No audio: %s\n", err)
		} else {
			s.audio = newAudioSide(source, opts.AudioChannel, s.log)
			app.Notef("Listening for the scanner's audio on %q when somebody asks for it.\n", source)
		}
	}
	defer s.audio.stop()

	// The reader shares this App's scanner and refuses anything that could
	// move it, so a mirror can keep drawing while a command walks the menus.
	// Its streams are pointed at each caller in turn like any other runner's.
	if app.Reader != nil {
		s.peek = &runner{app: app.Reader()}
	}

	app.Notef("Serving %s on %s\n", listenPort, path)
	app.Notef("Other radiocli commands for this scanner will queue here instead of being refused.\n")

	// The stop is derived here rather than taken from the caller, so that the
	// daemon can end itself when it is finished with while still ending when
	// whoever is above it says so.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	if opts.Orphaned != nil {
		go s.stopWhenUnused(ctx, opts.Orphaned, stop)
	}

	return s.accept(ctx, listener, path)
}

// accept takes connections until ctx is cancelled, and waits for the ones in
// flight before returning.
//
// Parameters:
//   - ctx: cancelling it closes the listener, which is what ends this
//   - listener: the socket clients arrive on
//   - path: where that socket is, so it can be removed on the way out
//
// Returns:
//   - error if accepting failed for a reason other than the daemon stopping,
//     and nil for an ordinary shutdown
func (s *Server) accept(ctx context.Context, listener net.Listener, path string) error {
	// Closing the listener is what unblocks Accept, so the shutdown is a
	// goroutine watching ctx rather than a check in the loop.
	go func() {
		<-ctx.Done()
		listener.Close()
		// Removing the socket on the way out means the next client to be
		// refused the lock does not find a socket nobody is listening on.
		os.Remove(path)
	}()

	// Counted twice, for two different questions. The group is how this waits
	// for connections in flight before returning; the count on the server is
	// what says whether anybody is still using the daemon, which a group cannot
	// be asked.
	var inFlight sync.WaitGroup
	defer inFlight.Wait()

	for {
		c, err := listener.Accept()
		if err != nil {
			// A cancelled daemon closed the listener itself, which is not a
			// failure to report.
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accepting a connection: %w", err)
		}

		inFlight.Add(1)
		s.clients.add(1)
		go func() {
			defer inFlight.Done()
			defer s.clients.add(-1)
			s.serveConn(ctx, c)
		}()
	}
}

// add moves the count and wakes anything waiting on it.
//
// Parameters:
//   - delta: how far to move the count, which is 1 for a client arriving and
//     -1 for one going
func (c *clients) add(delta int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.count += delta
	if c.changed != nil {
		close(c.changed)
		c.changed = nil
	}
}

// stopWhenUnused ends the daemon once its starter has gone and nothing is
// connected to it.
//
// Both conditions are needed and neither is enough. A daemon whose starter has
// gone may still be the only way two other programs are reaching the radio, and
// a daemon with nothing connected is doing its job perfectly well as long as
// somebody is still expected to come back to it.
//
// Parameters:
//   - ctx: the daemon's own lifetime, which ends this watch when it ends
//   - orphaned: closed once whoever started the daemon has gone
//   - stop: what ends the daemon, called once both conditions are met
func (s *Server) stopWhenUnused(ctx context.Context, orphaned <-chan struct{}, stop context.CancelFunc) {
	select {
	case <-ctx.Done():
		return
	case <-orphaned:
	}

	for {
		// The count and the notification are taken together, so a client that
		// disconnects between reading one and waiting on the other cannot leave
		// this waiting for a change that has already happened.
		count, changed := s.clients.watch()
		if count == 0 {
			stop()
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
	}
}

// watch returns the count now, and a channel closed the next time it moves.
//
// Returns:
//   - how many clients are connected right now
//   - a channel closed the next time that count moves
func (c *clients) watch() (int, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.changed == nil {
		c.changed = make(chan struct{})
	}
	return c.count, c.changed
}
