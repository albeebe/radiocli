// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// serveFake runs a daemon over an in-memory connection against an App whose
// commands are whatever the test says they are.
//
// It deliberately does not go through Serve, which opens a real serial port.
// Everything this file is about happens above that: the queue, the lease, the
// cancel and the shape of the messages.
func serveFake(t *testing.T, run func(ctx context.Context, argv []string) error) (*fakeDaemon, *bufio.Scanner, func(Request), func()) {
	t.Helper()

	app := appcontext.New()
	app.Config = appcontext.Defaults()
	app.Log = slog.New(slog.DiscardHandler)
	app.Stdout, app.Stderr = io.Discard, io.Discard
	app.RunCommand = run

	ctx, stop := context.WithCancel(context.Background())
	d := &fakeDaemon{
		srv:  &Server{app: app, log: app.Log},
		ctx:  ctx,
		stop: stop,
	}
	d.srv.run = runner{app: app}

	in, send, closeConn := d.connect(t)
	return d, in, send, func() {
		closeConn()
		d.shutdown()
	}
}

// fakeDaemon is one daemon that several connections can be opened against, so
// a test can show what one client does to another. Two separate daemons share
// no scheduler and would prove nothing.
type fakeDaemon struct {
	srv     *Server
	ctx     context.Context
	stop    context.CancelFunc
	clients sync.WaitGroup
}

// connect opens another connection to this daemon.
func (d *fakeDaemon) connect(t *testing.T) (*bufio.Scanner, func(Request), func()) {
	t.Helper()
	in, send, closeConn, _ := d.connectServed(t)
	return in, send, closeConn
}

// connectServed opens another connection and also reports when the daemon has
// finished with it, which is what a test asking whether a client is still
// holding anything needs.
func (d *fakeDaemon) connectServed(t *testing.T) (*bufio.Scanner, func(Request), func(), <-chan struct{}) {
	t.Helper()

	client, server := net.Pipe()
	served := make(chan struct{})
	d.clients.Add(1)
	go func() {
		defer d.clients.Done()
		defer close(served)
		d.srv.serveConn(d.ctx, server)
	}()

	in := bufio.NewScanner(client)
	in.Buffer(make([]byte, 0, 4096), maxRequest)

	send := func(req Request) {
		t.Helper()
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Write(append(data, '\n')); err != nil {
			t.Fatalf("writing a request: %v", err)
		}
	}

	// The hello arrives before anything else, always.
	if got := readMsg(t, in); got.Type != TypeHello {
		t.Fatalf("the daemon opened with %q, wanted %q", got.Type, TypeHello)
	}

	return in, send, func() { client.Close() }, served
}

// shutdown stops the daemon and waits for its connections.
func (d *fakeDaemon) shutdown() {
	d.stop()
	d.clients.Wait()
}

// readMsg takes the next message, failing the test rather than blocking for
// ever if the daemon has stopped talking.
func readMsg(t *testing.T, in *bufio.Scanner) Response {
	t.Helper()

	type result struct {
		msg Response
		ok  bool
	}
	got := make(chan result, 1)
	go func() {
		if !in.Scan() {
			got <- result{}
			return
		}
		var msg Response
		json.Unmarshal(in.Bytes(), &msg)
		got <- result{msg: msg, ok: true}
	}()

	select {
	case r := <-got:
		if !r.ok {
			t.Fatal("the daemon stopped answering")
		}
		return r.msg
	case <-time.After(5 * time.Second):
		t.Fatal("the daemon never answered")
		return Response{}
	}
}

// waitFor reads until a message of this type arrives, so a test can ignore the
// stdout and started messages it does not care about.
func waitFor(t *testing.T, in *bufio.Scanner, kind string) Response {
	t.Helper()
	for range 50 {
		msg := readMsg(t, in)
		if msg.Type == kind {
			return msg
		}
	}
	t.Fatalf("no %q ever arrived", kind)
	return Response{}
}

// TestARunReportsWhatTheCommandWrote covers the ordinary path end to end: the
// streams stay apart and the exit status comes back.
func TestARunReportsWhatTheCommandWrote(t *testing.T) {
	_, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		return nil
	})
	defer stop()

	send(Request{Op: OpRun, ID: "1", Argv: []string{"status"}})

	if got := waitFor(t, in, TypeStarted); got.Argv[0] != "status" {
		t.Errorf("the daemon started %q, wanted status", got.Argv)
	}
	if got := waitFor(t, in, TypeDone); got.Error != "" || got.Code != 0 {
		t.Errorf("a command that worked came back as %+v", got)
	}
}

// TestATypedLineIsSplitByTheDaemon is what keeps a macro step that was accepted
// when it was saved from failing when it is run: one splitter, and it is this
// one.
func TestATypedLineIsSplitByTheDaemon(t *testing.T) {
	var got []string
	_, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		got = argv
		return nil
	})
	defer stop()

	send(Request{Op: OpRun, ID: "1", Command: `favorites rename "Night watch" Evening`})
	waitFor(t, in, TypeDone)

	want := []string{"favorites", "rename", "Night watch", "Evening"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("the line was split into %q, wanted %q", got, want)
	}
}

// TestACancelStopsTheCommand covers a caller pressing Ctrl-C. The command has to
// be told, rather than the daemon carrying on driving a radio for somebody who
// has gone.
func TestACancelStopsTheCommand(t *testing.T) {
	running := make(chan struct{})
	_, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		close(running)
		<-ctx.Done()
		return ctx.Err()
	})
	defer stop()

	send(Request{Op: OpRun, ID: "1", Argv: []string{"scanning"}})
	waitFor(t, in, TypeStarted)

	select {
	case <-running:
	case <-time.After(5 * time.Second):
		t.Fatal("the command never started")
	}

	send(Request{Op: OpCancel, ID: "1"})

	got := waitFor(t, in, TypeDone)
	if got.Code == 0 {
		t.Errorf("a cancelled command came back as a success: %+v", got)
	}
}

// TestALeaseMakesEverybodyElseWait is the promise a macro depends on. A command
// sent while a lease is held must run after it, not during it.
func TestALeaseMakesEverybodyElseWait(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	note := func(what string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, what)
	}

	daemon, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		note(argv[0])
		return nil
	})
	defer stop()

	send(Request{Op: OpLease, ID: "lease", TTL: "30s"})
	if got := waitFor(t, in, TypeLeased); got.ID != "lease" {
		t.Fatalf("the lease was answered as %+v", got)
	}

	// A second connection to the same daemon is the one that has to wait. The
	// leaseholder's own commands are let straight through, so only somebody
	// else can show that the lease is doing anything.
	otherIn, otherSend, otherClose := daemon.connect(t)
	defer otherClose()

	otherDone := make(chan struct{})
	go func() {
		defer close(otherDone)
		otherSend(Request{Op: OpRun, ID: "outside", Argv: []string{"interloper"}})
		waitFor(t, otherIn, TypeDone)
	}()

	// Inside the lease, this connection's own commands run, in order.
	send(Request{Op: OpRun, ID: "1", Argv: []string{"first"}})
	waitFor(t, in, TypeDone)
	send(Request{Op: OpRun, ID: "2", Argv: []string{"second"}})
	waitFor(t, in, TypeDone)

	// The interloper must not have run yet. This is the whole assertion: a
	// macro's steps are consecutive, and a command sent from a terminal
	// halfway through lands after them rather than between them.
	mu.Lock()
	during := append([]string(nil), order...)
	mu.Unlock()
	for _, ran := range during {
		if ran == "interloper" {
			t.Fatalf("a command from another connection ran inside the lease: %v", during)
		}
	}

	send(Request{Op: OpRelease, ID: "release"})
	waitFor(t, in, TypeReleased)

	select {
	case <-otherDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the command that waited for the lease never ran")
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"first", "second", "interloper"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("the commands ran %v, wanted %v", order, want)
	}
}

// TestAnExpiredLeaseWaitsForTheRunningCommand is the regression test for the
// scanner being handed away mid-command. A lease whose ttl fires while one of
// the leaseholder's commands is still running used to release the scheduler at
// once, which started the next caller's command on the same serial line; the
// expiry has to wait for the command to finish before anybody else runs.
func TestAnExpiredLeaseWaitsForTheRunningCommand(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	note := func(what string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, what)
	}

	finish := make(chan struct{})
	daemon, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		note("start " + argv[0])
		if argv[0] == "slow" {
			<-finish
		}
		note("end " + argv[0])
		return nil
	})
	defer stop()

	// A short lease, and a command inside it that outlives the ttl by holding
	// on until the test lets go.
	send(Request{Op: OpLease, ID: "lease", TTL: "30ms"})
	waitFor(t, in, TypeLeased)
	send(Request{Op: OpRun, ID: "1", Argv: []string{"slow"}})
	waitFor(t, in, TypeStarted)

	// Somebody else queues behind the lease.
	otherIn, otherSend, otherClose := daemon.connect(t)
	defer otherClose()
	otherSend(Request{Op: OpRun, ID: "2", Argv: []string{"interloper"}})

	// Long enough that the ttl has certainly fired, so the expiry is now
	// either waiting for the command, which is the fix, or has already handed
	// the scanner on, which is the bug.
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	ranEarly := len(order) > 1
	mu.Unlock()
	if ranEarly {
		t.Fatalf("the expiry handed the scanner on mid-command: %v", order)
	}

	close(finish)
	waitFor(t, in, TypeDone)
	waitFor(t, otherIn, TypeDone)

	mu.Lock()
	defer mu.Unlock()
	want := []string{"start slow", "end slow", "start interloper", "end interloper"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("the commands ran %v, wanted %v", order, want)
	}
}

// TestAQueuedLeaseStillLetsItsClientBeHeard is the regression test for a lease
// handled on the read loop. A lease waits for the scanner, and waiting for it
// there used to stop the connection reading anything else at all, so a client
// queued behind somebody else's macro went silent for the length of it, even
// for the operations that need no scanner and take no turn.
func TestAQueuedLeaseStillLetsItsClientBeHeard(t *testing.T) {
	daemon, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		return nil
	})
	defer stop()

	// The first connection takes the scanner and does not give it back.
	send(Request{Op: OpLease, ID: "lease", TTL: "30s"})
	waitFor(t, in, TypeLeased)

	// The second asks for a lease it cannot be granted, and then for something
	// that needs nothing the first connection is holding.
	otherIn, otherSend, otherClose := daemon.connect(t)
	defer otherClose()
	otherSend(Request{Op: OpLease, ID: "queued", TTL: "30s"})
	otherSend(Request{Op: OpCommands, ID: "commands"})

	got := waitFor(t, otherIn, TypeCommands)
	if got.ID != "commands" {
		t.Errorf("the daemon answered %+v, wanted the commands asked for behind the lease", got)
	}
}

// TestAReleaseCannotOvertakeItsOwnLease is the other half of moving leases off
// the read loop. Handling them there kept them in the order they were sent for
// free; handling them in goroutines would not, and a release that overtook the
// lease it was giving back would be answered as a connection holding no lease
// while the lease it asked for went on being granted behind it.
func TestAReleaseCannotOvertakeItsOwnLease(t *testing.T) {
	daemon, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		return nil
	})
	defer stop()

	send(Request{Op: OpLease, ID: "first", TTL: "30s"})
	waitFor(t, in, TypeLeased)

	// Both sent while the scanner is held elsewhere, so the lease is certainly
	// still queued when the release arrives.
	otherIn, otherSend, otherClose := daemon.connect(t)
	defer otherClose()
	otherSend(Request{Op: OpLease, ID: "queued", TTL: "30s"})
	otherSend(Request{Op: OpRelease, ID: "giveback"})

	send(Request{Op: OpRelease, ID: "done"})
	waitFor(t, in, TypeReleased)

	if got := readMsg(t, otherIn); got.Type != TypeLeased || got.ID != "queued" {
		t.Fatalf("the queued connection was answered %+v, wanted its lease first", got)
	}
	if got := readMsg(t, otherIn); got.Type != TypeReleased || got.ID != "giveback" {
		t.Errorf("the queued connection was answered %+v, wanted its release second", got)
	}
}

// TestADisconnectedClientGivesUpItsQueuedLease covers a client that goes away
// while waiting for a lease. Its place in the queue used to be kept until the
// lease was granted, which briefly assigned the scanner to a connection that
// was already gone; it now gives the place up when the connection does.
func TestADisconnectedClientGivesUpItsQueuedLease(t *testing.T) {
	daemon, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		return nil
	})
	defer stop()

	// The first connection holds the scanner for the whole test.
	send(Request{Op: OpLease, ID: "lease", TTL: "30s"})
	waitFor(t, in, TypeLeased)

	// The second queues for a lease it will never be granted, and leaves.
	_, otherSend, otherClose, otherServed := daemon.connectServed(t)
	otherSend(Request{Op: OpLease, ID: "queued", TTL: "30s"})
	otherClose()

	// The daemon must be finished with it while the first connection still
	// holds the lease. Waiting for the grant instead would mean the daemon
	// cannot let go of a dead client until a live one chooses to let go first.
	select {
	case <-otherServed:
	case <-time.After(5 * time.Second):
		t.Fatal("the daemon held on to a disconnected client's queued lease")
	}
}

// TestTooManyWaitingIsRefused covers one client trying to build a backlog. The
// bound is per client, so a runaway page cannot fill the queue a terminal is
// waiting in.
func TestTooManyWaitingIsRefused(t *testing.T) {
	release := make(chan struct{})
	_, in, send, stop := serveFake(t, func(ctx context.Context, argv []string) error {
		<-release
		return nil
	})
	defer func() { close(release); stop() }()

	// One to hold the scanner, then enough behind it to go over the bound.
	for i := range maxWaiting + 2 {
		send(Request{Op: OpRun, ID: string(rune('a' + i)), Argv: []string{"status"}})
	}

	refused := false
	for range maxWaiting + 2 {
		msg := readMsg(t, in)
		if msg.Type == TypeDone && strings.Contains(msg.Error, "too many commands") {
			refused = true
			break
		}
	}
	if !refused {
		t.Error("a client was allowed to queue past the bound")
	}
}

// TestLongOutputArrivesWhole is the regression test for a command whose output
// was longer than one message.
//
// The output used to go out as one message however long it was, so a command
// that wrote more than the far end would read failed at the very end, after all
// of its work was done, with "token too long" and nothing said about which
// command or why. "colors --all" writes about ninety kilobytes of JSON, which
// is what found this.
//
// What is checked is that the pieces join back into exactly what was written,
// because output arriving in pieces is only correct if joining them is lossless.
func TestLongOutputArrivesWhole(t *testing.T) {
	// Longer than one message by some margin, and deliberately not a multiple
	// of the chunk size, so the last piece is a short one.
	want := strings.Repeat("abcdefghij", 9000) + "tail"

	// Written to the App's own stdout, which is what the runner points at the
	// stream for the length of a command. Writing there rather than to a stream
	// built by hand is what makes this the path a real command takes.
	var d *fakeDaemon
	d, in, send, done := serveFake(t, func(ctx context.Context, argv []string) error {
		_, err := io.WriteString(d.srv.app.Stdout, want)
		return err
	})
	defer done()

	send(Request{Op: OpRun, ID: "1", Argv: []string{"anything"}})

	var got strings.Builder
	for {
		msg := readMsg(t, in)
		switch msg.Type {
		case TypeStarted:
		case TypeStdout:
			if len(msg.Data) > maxRequest {
				t.Fatalf("one message carried %d bytes, which is more than the %d a reader will take",
					len(msg.Data), maxRequest)
			}
			got.WriteString(msg.Data)
		case TypeDone:
			if got.String() != want {
				t.Fatalf("the output came back as %d bytes, wanted %d", got.Len(), len(want))
			}
			return
		default:
			t.Fatalf("unexpected message %q", msg.Type)
		}
	}
}

// TestFitsNeverSplitsACharacter checks the rule that keeps a multi-byte
// character out of two messages, since each piece is encoded on its own and a
// piece that is not valid UTF-8 comes out as replacement characters.
//
// From a limit of one character upwards, which is every limit this is ever
// asked for: maxData is eight kilobytes. Below that there is no answer that is
// both within the limit and a whole character, and making progress is worth
// more than a rule that cannot be kept.
func TestFitsNeverSplitsACharacter(t *testing.T) {
	// Three byte characters, so a limit that is not a multiple of three lands
	// inside one of them and has to be backed off.
	p := []byte(strings.Repeat("★", 40))

	for limit := utf8.UTFMax; limit < len(p); limit++ {
		n := fits(p, limit)
		if n > limit {
			t.Fatalf("fits returned %d for a limit of %d, which is over it", n, limit)
		}
		if n <= 0 {
			t.Fatalf("fits returned %d for a limit of %d, which makes no progress", n, limit)
		}
		if !utf8.Valid(p[:n]) {
			t.Fatalf("a limit of %d cut a character in half", limit)
		}
	}
}

// TestFitsPassesBytesThatAreNotText checks that output which is not UTF-8 is
// still forwarded rather than stalled looking for a boundary that is not there.
func TestFitsPassesBytesThatAreNotText(t *testing.T) {
	p := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80}
	if n := fits(p, 3); n != 3 {
		t.Fatalf("fits returned %d for bytes with no boundaries, wanted 3", n)
	}
}
