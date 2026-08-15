// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/albeebe/radiocli/internal/buildinfo"
	"github.com/albeebe/radiocli/internal/cmdline"
)

// Write forwards one piece of command output.
//
// Long output is split across several messages. Nothing is lost by that: the
// two output streams are byte streams, and a reader joins the pieces back in
// the order they arrive, which is the order they were written.
//
// A failed write is swallowed on purpose. The client may have gone away while
// the command is still running, and a write error travelling back into the
// command would abort it partway through, which for a command walking menus
// means leaving the scanner on a screen nobody chose. Losing the output of a
// command nobody is watching is the better failure.
//
// Parameters:
//   - p: what the command wrote, which may be longer than one message
//
// Returns:
//   - the number of bytes taken, which is always all of them
//   - error, which is always nil, for the reason above
func (w *stream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for rest := p; len(rest) > 0; {
		n := fits(rest, maxData)
		w.conn.send(Response{Type: w.kind, ID: w.id, Data: string(rest[:n])})
		rest = rest[n:]
	}
	return len(p), nil
}

// afterLeases runs one lease operation off the read loop, once every lease
// operation taken off that loop before it has finished.
//
// The two properties are wanted together and pull in opposite directions. Two
// leases asked for in a row must be taken in the order they were asked for, and
// a release must not overtake the lease it is giving back, which is what
// handling them inline used to guarantee. But a lease waits for the scanner,
// which for a client queued behind a macro is a wait measured in seconds, and
// inline that wait stops this connection reading anything else, including the
// client's own cancel for the run it is stuck behind.
//
// So the place in the order is taken here, on the read loop, where the order is
// the order messages arrived; the waiting happens in a goroutine. Each caller
// takes the channel the one before it will close and leaves behind one of its
// own, so the operations form a chain that runs in the order it was built. The
// chain field needs no lock because this is only ever called from the read
// loop, which is one goroutine.
//
// The context is the connection's rather than the daemon's, so a client that
// disconnects while queued gives up its place instead of being handed the
// scanner it can no longer use.
//
// Parameters:
//   - ctx: the connection's lifetime, which abandons the operation when it ends
//   - handlers: the group serveConn waits on before it tears the connection down
//   - f: the operation to run once every one before it has finished
func (c *conn) afterLeases(ctx context.Context, handlers *sync.WaitGroup, f func()) {
	prev := c.leases
	next := make(chan struct{})
	c.leases = next

	handlers.Add(1)
	go func() {
		defer handlers.Done()
		// Closed whichever way this goes, because the operation behind it is
		// waiting on it and an abandoned one that never closed would strand
		// every lease operation this connection has left.
		defer close(next)

		select {
		case <-prev:
			f()
		case <-ctx.Done():
		}
	}()
}

// cancel stops a run already in flight.
//
// Parameters:
//   - req: the OpCancel message, whose ID names the run to stop
func (c *conn) cancel(req Request) {
	c.mu.Lock()
	stop := c.cancels[req.ID]
	c.mu.Unlock()

	if stop != nil {
		stop()
	}
}

// cancelAll stops everything this client started, for a disconnect.
func (c *conn) cancelAll() {
	c.mu.Lock()
	stops := make([]context.CancelFunc, 0, len(c.cancels))
	for _, stop := range c.cancels {
		stops = append(stops, stop)
	}
	c.mu.Unlock()

	for _, stop := range stops {
		stop()
	}
}

// commands answers OpCommands. It needs no scanner and takes no turn, because
// it reads the command tree rather than the radio.
//
// Parameters:
//   - req: the OpCommands message, whose ID the answer is echoed back with
func (c *conn) commands(req Request) {
	msg := Response{Type: TypeCommands, ID: req.ID}
	if c.srv.app.Commands != nil {
		msg.Commands = c.srv.app.Commands()
	}
	c.send(msg)
}

// execute runs one command and streams back what it produced.
//
// Parameters:
//   - ctx: the daemon's own lifetime, which ends every run when it is cancelled
//   - req: the run to perform, and the mode that says how it waits for its turn
func (c *conn) execute(ctx context.Context, req Request) {
	argv, err := resolveArgv(req)
	if err != nil {
		c.send(Response{Type: TypeDone, ID: req.ID, Error: err.Error(), Code: 1})
		return
	}

	c.mu.Lock()
	c.waiting++
	over := c.waiting > maxWaiting
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.waiting--
		c.mu.Unlock()
	}()

	if over {
		c.send(Response{Type: TypeDone, ID: req.ID, Code: 1,
			Error: "the scanner is busy: too many commands are already waiting"})
		return
	}

	// A peek never takes a turn. It runs on a second App that shares the
	// scanner and can only run commands that read, so it neither waits for the
	// walk in front of it nor disturbs it.
	if req.Mode == ModePeek {
		c.peek(ctx, req, argv)
		return
	}

	// A leaseholder already owns the scanner, so taking a turn for each
	// command would be waiting for itself. The run is registered on the lease
	// rather than the lease merely being looked at, because a look answers
	// "was there a lease a moment ago", and the moment between that answer and
	// this command reaching the serial line is when an expiry could hand the
	// scanner to the next in line. A registered run makes the expiry wait.
	if l := c.leasedRun(); l != nil {
		defer l.end()
	} else {
		if req.Mode == ModeTry {
			if !c.srv.sched.tryAcquire() {
				c.send(Response{Type: TypeSkipped, ID: req.ID})
				return
			}
		} else if err := c.srv.sched.acquire(ctx); err != nil {
			c.send(Response{Type: TypeDone, ID: req.ID, Error: err.Error(), Code: 1})
			return
		}
		defer c.srv.sched.release()
	}

	// One command at a time from this client, whether its turn came from the
	// scheduler or from the lease it is holding.
	c.running.Lock()
	defer c.running.Unlock()

	// The run answers to the daemon's lifetime and to this client's cancel,
	// but not to the connection dropping: a command that walks menus must
	// finish rather than leave the scanner on a half-typed screen because
	// somebody closed a tab. cancelAll is what turns a disconnect into a
	// cancel, deliberately and for every run at once.
	runCtx, stop := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancels[req.ID] = stop
	c.mu.Unlock()
	defer func() {
		stop()
		c.mu.Lock()
		delete(c.cancels, req.ID)
		c.mu.Unlock()
	}()

	if err := c.send(Response{Type: TypeStarted, ID: req.ID, Argv: argv}); err != nil {
		return
	}

	err = c.srv.run.run(runCtx, argv,
		&stream{conn: c, id: req.ID, kind: TypeStdout},
		&stream{conn: c, id: req.ID, kind: TypeStderr},
	)

	done := Response{Type: TypeDone, ID: req.ID}
	if err != nil {
		// A cancelled run is the caller's own doing. It still exits 1, which
		// is what the terminal does, but it is not a message worth printing.
		if !errors.Is(err, context.Canceled) {
			done.Error = err.Error()
		}
		done.Code = 1
	}
	c.send(done)
}

// fits reports how much of p to put in one message, which is limit except that
// it will not cut a character in half.
//
// A character split across two messages is not two halves that join up again:
// each piece is encoded on its own, and a piece that is not valid UTF-8 is
// encoded as replacement characters, so the character is destroyed rather than
// delayed. Backing up to a boundary costs at most three bytes of a message.
//
// Output that is not UTF-8 at all has no boundaries to find. It is cut at the
// limit and passed on as it is, which is what a byte stream deserves: the
// pieces still join back into exactly the bytes the command wrote.
//
// Parameters:
//   - p: the output still to be sent
//   - limit: the most that may go into one message
//
// Returns:
//   - how much of p to take, never over limit and never zero
func fits(p []byte, limit int) int {
	if len(p) <= limit {
		return len(p)
	}

	cut := limit
	for back := 0; back < utf8.UTFMax && cut > 0; back++ {
		if utf8.RuneStart(p[cut]) {
			return cut
		}
		cut--
	}
	return limit
}

// forgetLease drops this client's record of its lease.
func (c *conn) forgetLease() {
	c.mu.Lock()
	c.held = nil
	c.mu.Unlock()
}

// lease claims the scanner across several commands. It answers only once the
// lease is actually held, which for a client queued behind another one is later
// than when it asked.
//
// Parameters:
//   - ctx: the connection's lifetime, which gives up the place in the queue
//     when the client disconnects or the daemon stops
//   - req: the OpLease message, whose TTL bounds how long it may be held
func (c *conn) lease(ctx context.Context, req Request) {
	ttl, err := time.ParseDuration(req.TTL)
	if err != nil {
		c.send(Response{Type: TypeError, ID: req.ID, Error: "a lease needs a ttl, such as \"30s\": " + err.Error()})
		return
	}
	if ttl <= 0 || ttl > maxLeaseTTL {
		c.send(Response{Type: TypeError, ID: req.ID,
			Error: "a lease must last between nothing and " + maxLeaseTTL.String()})
		return
	}

	c.mu.Lock()
	already := c.held != nil
	c.mu.Unlock()
	if already {
		c.send(Response{Type: TypeError, ID: req.ID, Error: "this connection already holds a lease"})
		return
	}

	if err := c.srv.sched.acquire(ctx); err != nil {
		c.send(Response{Type: TypeError, ID: req.ID, Error: err.Error()})
		return
	}

	c.mu.Lock()
	c.held = newLease(ttl, func(l *lease) {
		// The daemon's own logger, never the App's: the expiry fires while a
		// runner may have the App's logger pointed at some client's stderr,
		// and this warning belongs to the daemon rather than to whichever
		// command happens to be running.
		c.srv.log.Warn("a lease expired and was taken back", "ttl", ttl)
		c.forgetLease()
		l.drain()
		c.srv.sched.release()
	})
	c.mu.Unlock()

	c.send(Response{Type: TypeLeased, ID: req.ID})
}

// leasedRun registers one command as running under this client's lease, and
// reports the lease it was registered on. The caller must call end on it once
// the command has finished.
//
// Returns:
//   - the lease this run is registered on, or nil when this client holds none
//     it can use, in which case the caller takes a turn from the scheduler
func (c *conn) leasedRun() *lease {
	c.mu.Lock()
	l := c.held
	c.mu.Unlock()

	if l == nil || !l.begin() {
		return nil
	}
	return l
}

// peek runs a read alongside whatever has the scanner.
//
// It skips the scheduler entirely, so it neither waits for the command in front
// of it nor makes that command wait. What keeps it honest is the App behind it,
// which shares the connection and refuses any command that could move the
// radio: the refusal is the command's own error, reported the way any other
// failure is.
//
// It is not cancelled by the client's cancel, and does not need to be. A read
// is one exchange and is over before a cancel could arrive.
//
// Parameters:
//   - ctx: the daemon's own lifetime
//   - req: the run to perform, whose ID its messages are echoed back with
//   - argv: the already split argument list to run
func (c *conn) peek(ctx context.Context, req Request, argv []string) {
	if c.srv.peek == nil {
		c.send(Response{Type: TypeDone, ID: req.ID, Code: 1,
			Error: "this daemon cannot run commands alongside another one"})
		return
	}

	// Peeks share one App, so they go one at a time. Waiting here is waiting
	// for another read rather than for a menu walk, which is the difference
	// that matters: it is measured in milliseconds.
	c.srv.peeking.Lock()
	defer c.srv.peeking.Unlock()

	if err := c.send(Response{Type: TypeStarted, ID: req.ID, Argv: argv}); err != nil {
		return
	}

	err := c.srv.peek.run(ctx, argv,
		&stream{conn: c, id: req.ID, kind: TypeStdout},
		&stream{conn: c, id: req.ID, kind: TypeStderr},
	)

	done := Response{Type: TypeDone, ID: req.ID}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			done.Error = err.Error()
		}
		done.Code = 1
	}
	c.send(done)
}

// release gives a lease back.
//
// Parameters:
//   - req: the OpRelease message, whose ID the answer is echoed back with
func (c *conn) release(req Request) {
	if c.releaseLease() {
		c.send(Response{Type: TypeReleased, ID: req.ID})
		return
	}
	c.send(Response{Type: TypeError, ID: req.ID, Error: "this connection holds no lease"})
}

// releaseLease hands the scanner back if this client is holding it, and reports
// whether it was. It is safe to call more than once, which is what lets both an
// explicit release and a disconnect run it.
//
// A command of this client's own still on the serial line is waited for before
// the scanner is handed on, because the next in line would otherwise start
// talking over it. The wait is short and bounded: it is one command, already
// underway, and cancelled runs count down the same as finished ones.
//
// Returns:
//   - whether this call was the one that gave the scanner back
func (c *conn) releaseLease() bool {
	c.mu.Lock()
	l := c.held
	c.mu.Unlock()

	if l == nil || !l.take() {
		return false
	}
	c.forgetLease()
	l.drain()
	c.srv.sched.release()
	return true
}

// resolveArgv turns a request into the arguments to run.
//
// Splitting a typed line happens here rather than on the client, so that the
// splitter which accepted a macro step when it was saved is the one that splits
// it when it runs. A line refused at run time would fail in the middle of a
// macro, with the steps before it already done.
//
// Parameters:
//   - req: the run, carrying either an argument list or a line to split
//
// Returns:
//   - the arguments to run
//   - error if the line could not be split, or came to nothing at all
func resolveArgv(req Request) ([]string, error) {
	if len(req.Argv) > 0 {
		return req.Argv, nil
	}

	argv, err := cmdline.Split(req.Command)
	if err != nil {
		return nil, err
	}
	if len(argv) == 0 {
		return nil, errors.New("no command given")
	}
	return argv, nil
}

// send writes one message, in whichever framing this connection is using.
//
// The check for streaming belongs here rather than at each call site, and that
// is not tidiness. Two of the places that send on this connection do not know
// what kind of connection it is: the reply to a message that would not parse,
// and the reply to an operation nobody has heard of. Both are reached from the
// read loop, on any connection, at any time. If either wrote a bare line onto a
// connection that had started streaming, it would land in the middle of the
// audio, and nothing in that stream says where a frame begins, so the far end
// could never resynchronise. Deciding once, here, is what makes that
// unrepresentable.
//
// Parameters:
//   - msg: the message to send
//
// Returns:
//   - error if the message could not be encoded or written
func (c *conn) send(msg Response) error {
	data, err := marshalJSON(msg)
	if err != nil {
		return err
	}

	c.write.Lock()
	defer c.write.Unlock()

	if c.streaming.Load() {
		return writeFrame(c.net, FrameJSON, data)
	}
	_, err = c.net.Write(append(data, '\n'))
	return err
}

// sendLine writes one message as a line whatever this connection is doing.
//
// It exists for exactly one message: the reply to OpAudio, which is the last
// line an audio connection carries and the thing that makes it a streaming one.
// Sending that through send would mean deciding whether the connection is
// streaming while in the act of deciding it, and the answer would depend on
// which side of the store the read happened.
//
// The caller holds c.write, because the point of this is that the reply and the
// switch happen without anything getting between them.
//
// Parameters:
//   - msg: the message to send, which is the reply to OpAudio
//
// Returns:
//   - error if the message could not be encoded or written
func (c *conn) sendLine(msg Response) error {
	data, err := marshalJSON(msg)
	if err != nil {
		return err
	}
	_, err = c.net.Write(append(data, '\n'))
	return err
}

// serveConn answers one client until it disconnects or the daemon stops.
//
// Parameters:
//   - ctx: the daemon's own lifetime, which closes this connection when it ends
//   - network: the socket this client arrived on
func (s *Server) serveConn(ctx context.Context, network net.Conn) {
	// The lease chain starts closed, so the first lease operation on this
	// connection has nothing in front of it to wait for.
	started := make(chan struct{})
	close(started)

	c := &conn{
		net:       network,
		srv:       s,
		cancels:   map[string]context.CancelFunc{},
		bitrate:   make(chan int, 1),
		audioDone: make(chan struct{}),
		leases:    started,
	}
	defer network.Close()

	// Whatever this client was holding goes with it. A page that crashed
	// mid-macro must not leave the radio claimed for the length of its lease.
	defer c.releaseLease()
	defer c.cancelAll()

	// Requests are handled in their own goroutines so a long command does not
	// stop this connection reading the next one, which is what lets a cancel
	// arrive while the thing it cancels is still running.
	var handlers sync.WaitGroup
	defer handlers.Wait()

	// This connection's own lifetime, ending when it does. It is what a queued
	// lease waits on, so that a client which goes away while waiting gives up
	// its place rather than being granted the scanner it can no longer use.
	// Deferred after the wait above and therefore run before it: the wait would
	// otherwise be waiting for a lease that is waiting for a client that has
	// already gone.
	connCtx, closed := context.WithCancel(ctx)
	defer closed()

	// Deferred after the wait above, and therefore run before it, which is the
	// whole point. A command ends by itself, so waiting for one is safe. A
	// stream does not: it ends when its write fails, and the write fails when
	// the socket closes, which is the very last deferred thing here. Waiting
	// first and closing afterwards would be waiting for something that is
	// waiting for the wait to finish.
	defer c.streams.Wait()
	defer close(c.audioDone)

	if err := c.send(Response{
		Type:     TypeHello,
		Version:  buildinfo.Version,
		Protocol: Version,
		Audio:    s.audio.name(),
	}); err != nil {
		return
	}

	// Reading stops when the daemon does, because a client sitting idle would
	// otherwise hold the shutdown open until it happened to send something.
	go func() {
		<-ctx.Done()
		network.Close()
	}()

	scan := bufio.NewScanner(network)
	scan.Buffer(make([]byte, 0, 4096), maxRequest)

	for scan.Scan() {
		var req Request
		if err := json.Unmarshal(scan.Bytes(), &req); err != nil {
			c.send(Response{Type: TypeError, Error: "could not read that request: " + err.Error()})
			continue
		}

		switch req.Op {
		case OpCommands:
			c.commands(req)
		case OpLease:
			// Off the read loop, but still in the order asked for. A lease
			// waits for the scanner, and waiting here would stop this
			// connection reading anything else in the meantime, including the
			// cancel for the very command being waited on. See afterLeases.
			c.afterLeases(connCtx, &handlers, func() { c.lease(connCtx, req) })
		case OpRelease:
			// In the same order as the leases, so a release cannot overtake
			// the lease it is giving back.
			c.afterLeases(connCtx, &handlers, func() { c.release(req) })
		case OpCancel:
			c.cancel(req)
		case OpAudio:
			// Inline, so the reply and the switch to frames are finished
			// before the next message on this connection is read. Handled in a
			// goroutine, a second message could arrive while the connection was
			// half converted.
			c.audio(req)
		case OpRun, "":
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				c.execute(ctx, req)
			}()
		default:
			c.send(Response{Type: TypeError, ID: req.ID, Error: "unknown op " + req.Op})
		}
	}

	// A request too long for the scanner's buffer ends the loop rather than
	// being skipped, because a scanner cannot resynchronise: it has no way to
	// know where the oversized line stopped. Saying so before the socket closes
	// is the difference between a client that knows what it did and one that
	// sees a bare EOF and concludes the daemon died.
	if errors.Is(scan.Err(), bufio.ErrTooLong) {
		c.send(Response{Type: TypeError,
			Error: fmt.Sprintf("that request is longer than the %d bytes a request may be, "+
				"so this connection is being closed: nothing after it could be read", maxRequest)})
	}
}
