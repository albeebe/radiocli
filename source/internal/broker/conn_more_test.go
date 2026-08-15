// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package broker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// testConn builds one connection to a daemon over a real socket, along with the
// client end and the server that answers it, so the pieces of a connection can
// be driven one at a time.
func testConn(t *testing.T, run func(ctx context.Context, argv []string) error) (*conn, *Server, *bufio.Scanner, net.Conn) {
	t.Helper()

	app := daemonApp(t, run)
	srv := &Server{app: app, run: runner{app: app}, log: slog.New(slog.DiscardHandler)}

	client, server := socketPair(t)
	c := &conn{
		net:       server,
		srv:       srv,
		cancels:   map[string]context.CancelFunc{},
		bitrate:   make(chan int, 1),
		audioDone: make(chan struct{}),
	}

	in := bufio.NewScanner(client)
	in.Buffer(make([]byte, 0, 4096), maxRequest)
	return c, srv, in, client
}

// nextMsg takes the next message the daemon sent on this connection.
func nextMsg(t *testing.T, in *bufio.Scanner) Response {
	t.Helper()

	if !in.Scan() {
		t.Fatal("the daemon said nothing")
	}
	var msg Response
	if err := json.Unmarshal(in.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

// TestWrite tests the stream.Write method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Nothing: a write of nothing sends nothing
//   - Success: what the command wrote comes back on this run's own stream
func TestWrite(t *testing.T) {
	// Verify that a write of nothing sends nothing.
	t.Run("Nothing", func(t *testing.T) {
		c, _, _, _ := testConn(t, nil)

		n, err := (&stream{conn: c, id: "1", kind: TypeStdout}).Write(nil)
		if n != 0 || err != nil {
			t.Fatalf("writing nothing gave %d and %v", n, err)
		}
	})

	// Verify that what the command wrote comes back on this run's own stream.
	t.Run("Success", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		n, err := (&stream{conn: c, id: "7", kind: TypeStderr}).Write([]byte("a note"))
		if n != 6 || err != nil {
			t.Fatalf("writing output gave %d and %v", n, err)
		}

		msg := nextMsg(t, in)
		if msg.Type != TypeStderr || msg.ID != "7" || msg.Data != "a note" {
			t.Fatalf("the output came back as %+v", msg)
		}
	})
}

// Test_afterLeases tests the afterLeases method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Order: two operations run in the order they were taken off the read loop
//   - Abandoned: a connection that ends drops the operation rather than running it
//   - ChainSurvives: an abandoned operation still lets the ones behind it go
func Test_afterLeases(t *testing.T) {
	// A connection with an empty chain, ready for the first operation.
	fresh := func() *conn {
		started := make(chan struct{})
		close(started)
		return &conn{leases: started}
	}

	// Verify that two operations run in the order they were taken off the loop.
	t.Run("Order", func(t *testing.T) {
		c := fresh()
		var handlers sync.WaitGroup

		ran := make(chan string, 2)
		first := make(chan struct{})
		c.afterLeases(context.Background(), &handlers, func() {
			<-first
			ran <- "first"
		})
		c.afterLeases(context.Background(), &handlers, func() { ran <- "second" })

		close(first)
		handlers.Wait()

		if <-ran != "first" || <-ran != "second" {
			t.Fatal("two operations ran out of the order they arrived in")
		}
	})

	// Verify that a connection that has ended drops the operation.
	t.Run("Abandoned", func(t *testing.T) {
		c := fresh()
		var handlers sync.WaitGroup

		ctx, closed := context.WithCancel(context.Background())
		closed()

		// Nothing has closed the chain this one is waiting on, so the only way
		// out is the context, which is the disconnect this covers.
		c.leases = make(chan struct{})

		ran := false
		c.afterLeases(ctx, &handlers, func() { ran = true })
		handlers.Wait()

		if ran {
			t.Fatal("an operation ran for a connection that had already ended")
		}
	})

	// Verify that an abandoned operation does not strand the ones behind it.
	t.Run("ChainSurvives", func(t *testing.T) {
		c := fresh()
		var handlers sync.WaitGroup

		ctx, closed := context.WithCancel(context.Background())
		closed()
		c.leases = make(chan struct{})

		c.afterLeases(ctx, &handlers, func() {})

		ran := make(chan struct{})
		c.afterLeases(context.Background(), &handlers, func() { close(ran) })
		handlers.Wait()

		select {
		case <-ran:
		default:
			t.Fatal("an abandoned operation stranded the one behind it")
		}
	})
}

// Test_cancelAll tests the cancelAll method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: every run this client started is stopped
func Test_cancelAll(t *testing.T) {
	// Verify that every run this client started is stopped.
	t.Run("Success", func(t *testing.T) {
		c, _, _, _ := testConn(t, nil)

		first, stopFirst := context.WithCancel(context.Background())
		second, stopSecond := context.WithCancel(context.Background())
		c.cancels["1"], c.cancels["2"] = stopFirst, stopSecond

		c.cancelAll()

		if first.Err() == nil || second.Err() == nil {
			t.Fatal("a disconnect left a run going")
		}
	})
}

// Test_commands tests the commands method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Described: the daemon answers with the command tree it was given
//   - Undescribed: a daemon with no command tree answers with an empty list
func Test_commands(t *testing.T) {
	// Verify that the daemon answers with the command tree it was given.
	t.Run("Described", func(t *testing.T) {
		c, srv, in, _ := testConn(t, nil)
		srv.app.Commands = func() []appcontext.Command {
			return []appcontext.Command{{Name: "version", Short: "prints the version"}}
		}

		c.commands(Request{Op: OpCommands, ID: "1"})

		msg := nextMsg(t, in)
		if msg.Type != TypeCommands || len(msg.Commands) != 1 || msg.Commands[0].Name != "version" {
			t.Fatalf("the daemon described %+v", msg)
		}
	})

	// Verify that a daemon with no command tree answers with an empty list.
	t.Run("Undescribed", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.commands(Request{Op: OpCommands, ID: "1"})

		msg := nextMsg(t, in)
		if msg.Type != TypeCommands || len(msg.Commands) != 0 {
			t.Fatalf("a daemon with no command tree described %+v", msg)
		}
	})
}

// Test_execute tests the execute method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NoCommand: a request that comes to no command at all is refused
//   - Unwritable: a run whose client has gone is dropped rather than run
//   - Cancelled: a cancelled run exits 1 without an error worth printing
func Test_execute(t *testing.T) {
	// Verify that a request that comes to no command at all is refused.
	t.Run("NoCommand", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.execute(context.Background(), Request{Op: OpRun, ID: "1", Command: "  "})

		msg := nextMsg(t, in)
		if msg.Type != TypeDone || msg.Code != 1 || msg.Error != "no command given" {
			t.Fatalf("a request with no command gave %+v", msg)
		}
	})

	// Verify that a run whose client has gone is dropped rather than run.
	t.Run("Unwritable", func(t *testing.T) {
		ran := make(chan struct{}, 1)
		c, _, _, client := testConn(t, func(ctx context.Context, argv []string) error {
			ran <- struct{}{}
			return nil
		})

		client.Close()
		c.net.Close()

		c.execute(context.Background(), Request{Op: OpRun, ID: "1", Argv: []string{"version"}})

		select {
		case <-ran:
			t.Fatal("a run whose client had gone was started anyway")
		default:
		}
	})

	// Verify that a cancelled run exits 1 without an error worth printing.
	t.Run("Cancelled", func(t *testing.T) {
		c, _, in, _ := testConn(t, func(ctx context.Context, argv []string) error {
			return context.Canceled
		})

		c.execute(context.Background(), Request{Op: OpRun, ID: "1", Argv: []string{"version"}})

		if got := nextMsg(t, in); got.Type != TypeStarted {
			t.Fatalf("the run opened with %+v", got)
		}
		msg := nextMsg(t, in)
		if msg.Type != TypeDone || msg.Code != 1 || msg.Error != "" {
			t.Fatalf("a cancelled run gave %+v", msg)
		}
	})
}

// Test_executeReportsAFailure tests the execute method's failure path with 100%
// coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Failed: a command that failed reports its own error and exits 1
func Test_executeReportsAFailure(t *testing.T) {
	// Verify that a command that failed reports its own error and exits 1.
	t.Run("Failed", func(t *testing.T) {
		c, _, in, _ := testConn(t, func(ctx context.Context, argv []string) error {
			return errors.New("the scanner refused the key")
		})

		c.execute(context.Background(), Request{Op: OpRun, ID: "1", Argv: []string{"key", ">"}})

		if got := nextMsg(t, in); got.Type != TypeStarted {
			t.Fatalf("the run opened with %+v", got)
		}
		msg := nextMsg(t, in)
		if msg.Type != TypeDone || msg.Code != 1 || msg.Error != "the scanner refused the key" {
			t.Fatalf("a failed run gave %+v", msg)
		}
	})
}

// Test_lease tests the lease method with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NoTTL: a lease without a ttl is refused
//   - TooLong: a lease longer than the daemon allows is refused
//   - Already: a second lease on one connection is refused
//   - Interrupted: a daemon interrupted while the lease waited reports it
//   - Expired: a lease held past its ttl is taken back
func Test_lease(t *testing.T) {
	// Verify that a lease without a ttl is refused.
	t.Run("NoTTL", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.lease(context.Background(), Request{Op: OpLease, ID: "1"})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, "a lease needs a ttl") {
			t.Fatalf("a lease without a ttl gave %+v", msg)
		}
	})

	// Verify that a lease longer than the daemon allows is refused.
	t.Run("TooLong", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.lease(context.Background(), Request{Op: OpLease, ID: "1", TTL: (maxLeaseTTL + time.Second).String()})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, "a lease must last between") {
			t.Fatalf("a lease that was too long gave %+v", msg)
		}
	})

	// Verify that a second lease on one connection is refused.
	t.Run("Already", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.lease(context.Background(), Request{Op: OpLease, ID: "1", TTL: "30s"})
		if got := nextMsg(t, in); got.Type != TypeLeased {
			t.Fatalf("the first lease gave %+v", got)
		}

		c.lease(context.Background(), Request{Op: OpLease, ID: "2", TTL: "30s"})
		msg := nextMsg(t, in)
		if msg.Type != TypeError || msg.Error != "this connection already holds a lease" {
			t.Fatalf("a second lease gave %+v", msg)
		}

		c.releaseLease()
	})

	// Verify that a daemon interrupted while the lease waited reports it.
	t.Run("Interrupted", func(t *testing.T) {
		c, srv, in, _ := testConn(t, nil)

		// Held by somebody else, so this lease has to wait for a turn that is
		// never coming.
		if err := srv.sched.acquire(context.Background()); err != nil {
			t.Fatal(err)
		}

		ctx, stop := context.WithCancel(context.Background())
		stop()

		c.lease(ctx, Request{Op: OpLease, ID: "1", TTL: "30s"})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, context.Canceled.Error()) {
			t.Fatalf("an interrupted lease gave %+v", msg)
		}
	})

	// Verify that a lease held past its ttl is taken back.
	t.Run("Expired", func(t *testing.T) {
		c, srv, in, _ := testConn(t, nil)

		c.lease(context.Background(), Request{Op: OpLease, ID: "1", TTL: "1ms"})
		if got := nextMsg(t, in); got.Type != TypeLeased {
			t.Fatalf("the lease gave %+v", got)
		}

		deadline := time.Now().Add(5 * time.Second)
		for !srv.sched.tryAcquire() {
			if time.Now().After(deadline) {
				t.Fatal("an expired lease left the scanner held")
			}
			time.Sleep(time.Millisecond)
		}
		c.mu.Lock()
		still := c.held
		c.mu.Unlock()
		if still != nil {
			t.Fatal("the lease was never forgotten")
		}
	})
}

// Test_peek tests the peek method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Unwritable: a peek whose client has gone is dropped rather than run
func Test_peek(t *testing.T) {
	// Verify that a peek whose client has gone is dropped rather than run.
	t.Run("Unwritable", func(t *testing.T) {
		ran := make(chan struct{}, 1)
		c, srv, _, client := testConn(t, func(ctx context.Context, argv []string) error {
			ran <- struct{}{}
			return nil
		})
		srv.peek = &runner{app: srv.app}

		client.Close()
		c.net.Close()

		c.peek(context.Background(), Request{Op: OpRun, ID: "1", Mode: ModePeek}, []string{"version"})

		select {
		case <-ran:
			t.Fatal("a peek whose client had gone was started anyway")
		default:
		}
	})
}

// Test_release tests the release method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Held: a lease this connection holds is given back
//   - NotHeld: a connection holding no lease is told so
func Test_release(t *testing.T) {
	// Verify that a lease this connection holds is given back.
	t.Run("Held", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.lease(context.Background(), Request{Op: OpLease, ID: "1", TTL: "30s"})
		if got := nextMsg(t, in); got.Type != TypeLeased {
			t.Fatalf("the lease gave %+v", got)
		}

		c.release(Request{Op: OpRelease, ID: "2"})
		if got := nextMsg(t, in); got.Type != TypeReleased {
			t.Fatalf("giving the lease back gave %+v", got)
		}
	})

	// Verify that a connection holding no lease is told so.
	t.Run("NotHeld", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.release(Request{Op: OpRelease, ID: "1"})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || msg.Error != "this connection holds no lease" {
			t.Fatalf("a release with no lease gave %+v", msg)
		}
	})
}

// Test_resolveArgv tests the resolveArgv function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Argv: an already split argument list is taken as it is
//   - Line: a typed line is split by the daemon
//   - Unsplittable: a line that cannot be split is reported
//   - Nothing: a line that comes to nothing is refused
func Test_resolveArgv(t *testing.T) {
	// Verify that an already split argument list is taken as it is.
	t.Run("Argv", func(t *testing.T) {
		argv, err := resolveArgv(Request{Argv: []string{"key", ">"}, Command: "ignored"})
		if err != nil || len(argv) != 2 || argv[0] != "key" {
			t.Fatalf("an argument list came back as %q and %v", argv, err)
		}
	})

	// Verify that a typed line is split by the daemon.
	t.Run("Line", func(t *testing.T) {
		argv, err := resolveArgv(Request{Command: `favorites "My List"`})
		if err != nil || len(argv) != 2 || argv[1] != "My List" {
			t.Fatalf("a typed line came back as %q and %v", argv, err)
		}
	})

	// Verify that a line that cannot be split is reported.
	t.Run("Unsplittable", func(t *testing.T) {
		if _, err := resolveArgv(Request{Command: `favorites "unfinished`}); err == nil {
			t.Fatal("a line that could not be split was not reported")
		}
	})

	// Verify that a line that comes to nothing is refused.
	t.Run("Nothing", func(t *testing.T) {
		_, err := resolveArgv(Request{Command: "   "})
		if err == nil || err.Error() != "no command given" {
			t.Fatalf("a line that came to nothing gave %v", err)
		}
	})
}

// Test_sendLine tests the sendLine method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the message goes out as a line
//   - Unwritable: a message that could not be written is reported
//   - Unencodable: a message that could not be encoded is reported
func Test_sendLine(t *testing.T) {
	// Verify that the message goes out as a line.
	t.Run("Success", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		if err := c.sendLine(Response{Type: TypeAudio, ID: "1", Rate: 48000}); err != nil {
			t.Fatalf("sending a line: %v", err)
		}
		if got := nextMsg(t, in); got.Type != TypeAudio || got.Rate != 48000 {
			t.Fatalf("the line came back as %+v", got)
		}
	})

	// Verify that a message that could not be written is reported.
	t.Run("Unwritable", func(t *testing.T) {
		c, _, _, client := testConn(t, nil)
		client.Close()
		c.net.Close()

		if err := c.sendLine(Response{Type: TypeAudio}); err == nil {
			t.Fatal("a line written to a closed socket was not reported")
		}
	})

	// Verify that a message that could not be encoded is reported.
	t.Run("Unencodable", func(t *testing.T) {
		refused := failMarshal(t)
		c, _, _, _ := testConn(t, nil)

		if err := c.sendLine(Response{Type: TypeAudio}); !errors.Is(err, refused) {
			t.Fatalf("a line that could not be encoded gave %v", err)
		}
	})
}

// Test_serveConn tests the serveConn method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Commands: the command tree is answered on the connection it was asked on
//   - Unknown: an operation nobody has heard of is refused
//   - Malformed: a request that will not parse is refused without ending the connection
//   - Oversized: a request over the limit is answered before the connection ends
func Test_serveConn(t *testing.T) {
	// Verify that the command tree is answered on the connection it was asked on.
	t.Run("Commands", func(t *testing.T) {
		d, in, send, cleanup := serveFake(t, func(ctx context.Context, argv []string) error { return nil })
		defer cleanup()

		d.srv.app.Commands = func() []appcontext.Command {
			return []appcontext.Command{{Name: "version", Short: "prints the version"}}
		}

		send(Request{Op: OpCommands, ID: "1"})
		msg := waitFor(t, in, TypeCommands)
		if len(msg.Commands) != 1 || msg.Commands[0].Name != "version" {
			t.Fatalf("the daemon described %+v", msg)
		}
	})

	// Verify that an operation nobody has heard of is refused.
	t.Run("Unknown", func(t *testing.T) {
		_, in, send, cleanup := serveFake(t, func(ctx context.Context, argv []string) error { return nil })
		defer cleanup()

		send(Request{Op: "levitate", ID: "1"})
		msg := waitFor(t, in, TypeError)
		if !strings.Contains(msg.Error, "unknown op levitate") {
			t.Fatalf("an operation nobody has heard of gave %+v", msg)
		}
	})

	// Verify that a request that will not parse is refused without ending the connection.
	t.Run("Malformed", func(t *testing.T) {
		app := daemonApp(t, func(ctx context.Context, argv []string) error { return nil })
		srv := &Server{app: app, run: runner{app: app}, log: slog.New(slog.DiscardHandler)}

		client, server := socketPair(t)
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			srv.serveConn(ctx, server)
		}()

		in := bufio.NewScanner(client)
		in.Buffer(make([]byte, 0, 4096), maxRequest)
		if got := nextMsg(t, in); got.Type != TypeHello {
			t.Fatalf("the daemon opened with %+v", got)
		}

		if _, err := client.Write([]byte("this is not a request\n")); err != nil {
			t.Fatal(err)
		}
		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, "could not read that request") {
			t.Fatalf("a request that would not parse gave %+v", msg)
		}

		stop()
		<-done
	})

	// Verify that a request over the limit is answered before the connection
	// ends. It cannot be skipped, because nothing says where the oversized line
	// stopped, so the connection goes; what it must not do is go in silence,
	// which the client can only read as the daemon having died.
	t.Run("Oversized", func(t *testing.T) {
		app := daemonApp(t, func(ctx context.Context, argv []string) error { return nil })
		srv := &Server{app: app, run: runner{app: app}, log: slog.New(slog.DiscardHandler)}

		client, server := socketPair(t)
		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		done := make(chan struct{})
		go func() {
			defer close(done)
			srv.serveConn(ctx, server)
		}()

		in := bufio.NewScanner(client)
		in.Buffer(make([]byte, 0, 4096), maxRequest)
		if got := nextMsg(t, in); got.Type != TypeHello {
			t.Fatalf("the daemon opened with %+v", got)
		}

		// Written from a goroutine that ignores the outcome. The daemon gives
		// up part way through this and closes the socket, so the tail of the
		// write fails, which is the point rather than a problem.
		go client.Write(append(bytes.Repeat([]byte("x"), maxRequest+1), '\n'))

		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, "longer than") {
			t.Fatalf("an oversized request gave %+v", msg)
		}
		<-done
	})
}

// Test_stopWhenUnused tests the stopWhenUnused method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Interrupted: a daemon that ended while clients were still connected stops watching
func Test_stopWhenUnused(t *testing.T) {
	// Verify that a daemon that ended while clients were still connected stops watching.
	t.Run("Interrupted", func(t *testing.T) {
		s := &Server{}
		s.clients.add(1)

		orphaned := make(chan struct{})
		close(orphaned)

		ctx, stop := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.stopWhenUnused(ctx, orphaned, func() {
				t.Error("a daemon with a client still connected was stopped")
			})
		}()

		// Waited for, so that the daemon ends while the watch is waiting on a
		// count that is not going to move rather than before it started.
		deadline := time.Now().Add(5 * time.Second)
		for {
			s.clients.mu.Lock()
			waiting := s.clients.changed != nil
			s.clients.mu.Unlock()
			if waiting {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the watch never waited on the count")
			}
			time.Sleep(time.Millisecond)
		}
		stop()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the watch never gave up")
		}
	})
}

// Test_acquire tests the scheduler.acquire method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - WokenAsItGaveUp: a turn that arrived as the caller gave up is passed on
func Test_acquire(t *testing.T) {
	// Verify that a turn that arrived as the caller gave up is passed on rather
	// than dropped, which is the race the inner check exists for. It is run
	// many times because which of the two the select takes is decided for it.
	t.Run("WokenAsItGaveUp", func(t *testing.T) {
		for range 200 {
			s := &scheduler{}
			if err := s.acquire(context.Background()); err != nil {
				t.Fatal(err)
			}

			ctx, stop := context.WithCancel(context.Background())
			gaveUp := make(chan error, 1)
			go func() { gaveUp <- s.acquire(ctx) }()

			for s.queued() == 0 {
				time.Sleep(time.Microsecond)
			}

			// The turn and the giving up happen together, which is the whole
			// point: whichever of them the waiter sees first, the scanner must
			// end up back in the queue rather than on the floor.
			s.release()
			stop()

			if err := <-gaveUp; err == nil {
				s.release()
			}

			if !s.tryAcquire() {
				t.Fatal("the scanner was dropped on the floor")
			}
			s.release()
			if s.queued() != 0 {
				t.Fatalf("%d callers were left in the queue", s.queued())
			}
		}
	})
}

// Test_send tests the conn.send method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the message goes out as a line on a connection carrying lines
//   - Unwritable: a message that could not be written is reported
//   - Unencodable: a message that could not be encoded is reported
func Test_send(t *testing.T) {
	// Verify that the message goes out as a line on a connection carrying lines.
	t.Run("Success", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		refused := errors.New("the scanner refused the key")

		if err := c.send(Response{Type: TypeError, Error: refused.Error()}); err != nil {
			t.Fatalf("sending a message: %v", err)
		}
		if got := nextMsg(t, in); got.Type != TypeError || got.Error != refused.Error() {
			t.Fatalf("the message came back as %+v", got)
		}
	})

	// Verify that a message that could not be written is reported.
	t.Run("Unwritable", func(t *testing.T) {
		c, _, _, client := testConn(t, nil)
		client.Close()
		c.net.Close()

		if err := c.send(Response{Type: TypeError}); err == nil {
			t.Fatal("a message written to a closed socket was not reported")
		}
	})

	// Verify that a message that could not be encoded is reported.
	t.Run("Unencodable", func(t *testing.T) {
		refused := failMarshal(t)
		c, _, _, _ := testConn(t, nil)

		if err := c.send(Response{Type: TypeError}); !errors.Is(err, refused) {
			t.Fatalf("a message that could not be encoded gave %v", err)
		}
	})
}
