// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package broker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// scriptedDaemon listens on a socket of this test's own and answers each
// connection with whatever the test says, so the client can be driven through
// answers a real daemon only gives when something has gone wrong.
//
// It points the socket path at itself, so Dial and DialAudio find it.
func scriptedDaemon(t *testing.T, serve func(conn net.Conn)) {
	t.Helper()

	dir := shortDir(t)
	useSocketDir(t, dir)

	listener, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}

	var conns sync.WaitGroup
	t.Cleanup(func() {
		listener.Close()
		conns.Wait()
	})

	conns.Add(1)
	go func() {
		defer conns.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go func() {
				defer conns.Done()
				defer conn.Close()
				serve(conn)
			}()
		}
	}()
}

// sendLines writes messages to a client as the daemon would, one line each.
func sendLines(conn net.Conn, msgs ...Response) {
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		if _, err := conn.Write(append(data, '\n')); err != nil {
			return
		}
	}
}

// replyTo answers each request a client sends with whatever the test says,
// after the greeting a daemon always opens with.
func replyTo(answer func(req Request, send func(...Response))) func(net.Conn) {
	return func(conn net.Conn) {
		sendLines(conn, Response{Type: TypeHello, Version: "test", Protocol: Version})

		scan := bufio.NewScanner(conn)
		for scan.Scan() {
			var req Request
			if err := json.Unmarshal(scan.Bytes(), &req); err != nil {
				return
			}
			answer(req, func(msgs ...Response) { sendLines(conn, msgs...) })
		}
	}
}

// hangsUpAfterOneRequest greets a client, waits for it to ask for something,
// and then hangs up without answering.
func hangsUpAfterOneRequest(conn net.Conn) {
	sendLines(conn, Response{Type: TypeHello, Protocol: Version})
	bufio.NewReader(conn).ReadBytes('\n')
}

// failMarshal makes encoding fail for the duration of one test, and puts the
// real encoder back afterwards.
//
// The types this package sends are strings, ints and slices of them, which the
// real encoder never refuses, so this is the only way to reach the branch that
// reports a message that could not be encoded.
//
// Returns:
//   - the error encoding will report while it is in force
func failMarshal(t *testing.T) error {
	t.Helper()

	refused := errors.New("this message could not be written down")
	marshalJSON = func(any) ([]byte, error) { return nil, refused }
	t.Cleanup(func() { marshalJSON = json.Marshal })

	return refused
}

// TestClientSend tests the Client send method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the request goes out as a line
//   - Unencodable: a request that could not be encoded is reported
func TestClientSend(t *testing.T) {
	// Verify that the request goes out as a line.
	t.Run("Success", func(t *testing.T) {
		ours, theirs := net.Pipe()
		defer ours.Close()
		defer theirs.Close()

		c := &Client{net: ours}

		done := make(chan error, 1)
		go func() { done <- c.send(Request{Op: OpLease, ID: "1"}) }()

		var req Request
		if err := json.NewDecoder(theirs).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatalf("sending a request: %v", err)
		}
		if req.Op != OpLease || req.ID != "1" {
			t.Fatalf("the request arrived as %+v", req)
		}
	})

	// Verify that a request that could not be encoded is reported.
	t.Run("Unencodable", func(t *testing.T) {
		refused := failMarshal(t)

		ours, theirs := net.Pipe()
		defer ours.Close()
		defer theirs.Close()

		c := &Client{net: ours}

		if err := c.send(Request{Op: OpLease}); !errors.Is(err, refused) {
			t.Fatalf("a request that could not be encoded gave %v", err)
		}
	})
}

// TestAudioStreamSend tests the AudioStream send method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the request goes out as a line
//   - Unencodable: a request that could not be encoded is reported
func TestAudioStreamSend(t *testing.T) {
	// Verify that the request goes out as a line.
	t.Run("Success", func(t *testing.T) {
		ours, theirs := net.Pipe()
		defer ours.Close()
		defer theirs.Close()

		s := &AudioStream{net: ours}

		done := make(chan error, 1)
		go func() { done <- s.send(Request{Op: OpAudio, ID: "1", Action: "bitrate", Bitrate: 16000}) }()

		var req Request
		if err := json.NewDecoder(theirs).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatalf("sending a request: %v", err)
		}
		if req.Op != OpAudio || req.Bitrate != 16000 {
			t.Fatalf("the request arrived as %+v", req)
		}
	})

	// Verify that a request that could not be encoded is reported.
	t.Run("Unencodable", func(t *testing.T) {
		refused := failMarshal(t)

		ours, theirs := net.Pipe()
		defer ours.Close()
		defer theirs.Close()

		s := &AudioStream{net: ours}

		if err := s.send(Request{Op: OpAudio, Action: "bitrate"}); !errors.Is(err, refused) {
			t.Fatalf("a request that could not be encoded gave %v", err)
		}
	})
}

// TestDial tests the Dial function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NothingListening: a port with no daemon answers ErrNoDaemon
//   - NoGreeting: a daemon that hangs up without speaking answers ErrNoDaemon
//   - WrongGreeting: something that opened with anything else answers ErrNoDaemon
//   - WrongProtocol: a daemon from another build is reported
//   - Success: the daemon's version and protocol are taken from its greeting
func TestDial(t *testing.T) {
	// Verify that a port with no daemon answers ErrNoDaemon.
	t.Run("NothingListening", func(t *testing.T) {
		useSocketDir(t, shortDir(t))

		if _, err := Dial("port0"); !errors.Is(err, ErrNoDaemon) {
			t.Fatalf("dialling nothing gave %v", err)
		}
	})

	// Verify that a daemon that hangs up without speaking answers ErrNoDaemon.
	t.Run("NoGreeting", func(t *testing.T) {
		scriptedDaemon(t, func(conn net.Conn) {})

		_, err := Dial("port0")
		if !errors.Is(err, ErrNoDaemon) || !strings.Contains(err.Error(), "never introduced itself") {
			t.Fatalf("a silent daemon gave %v", err)
		}
	})

	// Verify that something that opened with anything else answers ErrNoDaemon.
	t.Run("WrongGreeting", func(t *testing.T) {
		scriptedDaemon(t, func(conn net.Conn) {
			sendLines(conn, Response{Type: TypeError, Error: "who are you"})
		})

		_, err := Dial("port0")
		if !errors.Is(err, ErrNoDaemon) || !strings.Contains(err.Error(), "it opened with") {
			t.Fatalf("a daemon that opened with an error gave %v", err)
		}
	})

	// Verify that a daemon from another build is reported.
	t.Run("WrongProtocol", func(t *testing.T) {
		scriptedDaemon(t, func(conn net.Conn) {
			sendLines(conn, Response{Type: TypeHello, Protocol: Version + 1})
		})

		_, err := Dial("port0")
		if err == nil || !strings.Contains(err.Error(), "stop the daemon and start it from this build") {
			t.Fatalf("a daemon from another build gave %v", err)
		}
	})

	// Verify that the daemon's version and protocol are taken from its greeting.
	t.Run("Success", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatalf("dialling a daemon: %v", err)
		}
		defer client.Close()

		if client.Version != "test" || client.Protocol != Version {
			t.Fatalf("the client took %q and %d from the greeting", client.Version, client.Protocol)
		}
	})
}

// TestDialAudio tests the DialAudio function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NothingListening: a port with no daemon answers ErrNoDaemon
//   - Success: the stream describes itself from the daemon's answer
func TestDialAudio(t *testing.T) {
	// Verify that a port with no daemon answers ErrNoDaemon.
	t.Run("NothingListening", func(t *testing.T) {
		useSocketDir(t, shortDir(t))

		if _, err := DialAudio("port0", FormatPCM, 0); !errors.Is(err, ErrNoDaemon) {
			t.Fatalf("dialling nothing gave %v", err)
		}
	})

	// Verify that the stream describes itself from the daemon's answer.
	t.Run("Success", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(Response{Type: TypeAudio, ID: req.ID, Format: req.Format, Rate: 48000,
				Channels: 1, FrameMS: 20, Bitrate: 24000, Source: "Line In", Channel: "left"})
		}))

		stream, err := DialAudio("port0", FormatOpus, 24000)
		if err != nil {
			t.Fatalf("asking for audio: %v", err)
		}
		defer stream.Close()

		if got := stream.Info(); got.Source != "Line In" || got.Bitrate != 24000 || got.Format != FormatOpus {
			t.Fatalf("the stream described itself as %+v", got)
		}
	})
}

// TestClientCommands tests the Client.Commands method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the daemon's command list comes back, past anything else on the wire
//   - Refused: a daemon that refused the request reports its own error
//   - Gone: a daemon that stopped answering is reported
//   - Unwritable: a request that could not be sent is reported
func TestClientCommands(t *testing.T) {
	// Verify that the daemon's command list comes back, past anything else on the wire.
	t.Run("Success", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(
				Response{Type: TypeStdout, ID: req.ID, Data: "ignored"},
				Response{Type: TypeCommands, ID: req.ID,
					Commands: []appcontext.Command{{Name: "version", Short: "prints the version"}}},
			)
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		got, err := client.Commands(context.Background())
		if err != nil {
			t.Fatalf("asking for the commands: %v", err)
		}
		if len(got) != 1 || got[0].Name != "version" {
			t.Fatalf("the daemon described %+v", got)
		}
	})

	// Verify that a daemon that refused the request reports its own error.
	t.Run("Refused", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(Response{Type: TypeError, ID: req.ID, Error: "it has no list to give"})
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if _, err := client.Commands(context.Background()); err == nil ||
			err.Error() != "it has no list to give" {
			t.Fatalf("a refused request gave %v", err)
		}
	})

	// Verify that a daemon that stopped answering is reported.
	t.Run("Gone", func(t *testing.T) {
		scriptedDaemon(t, hangsUpAfterOneRequest)

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if _, err := client.Commands(context.Background()); err == nil ||
			!strings.Contains(err.Error(), "the daemon stopped answering") {
			t.Fatalf("a daemon that hung up gave %v", err)
		}
	})

	// Verify that a request that could not be sent is reported.
	t.Run("Unwritable", func(t *testing.T) {
		client := hungUpClient(t)

		if _, err := client.Commands(context.Background()); err == nil {
			t.Fatal("a request written to a closed socket was not reported")
		}
	})
}

// hungUpClient is a client whose socket has already gone, so anything it tries
// to send fails.
func hungUpClient(t *testing.T) *Client {
	t.Helper()

	scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {}))

	client, err := Dial("port0")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	return client
}

// TestClientLease tests the Client.Lease method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the lease is held once the daemon says so
//   - Refused: a daemon that refused the lease reports its own error
//   - Gone: a daemon that stopped answering is reported
//   - Unwritable: a request that could not be sent is reported
func TestClientLease(t *testing.T) {
	// Verify that the lease is held once the daemon says so.
	t.Run("Success", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(
				Response{Type: TypeStarted, ID: req.ID},
				Response{Type: TypeLeased, ID: req.ID},
			)
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if err := client.Lease(context.Background(), 30*time.Second); err != nil {
			t.Fatalf("taking a lease: %v", err)
		}
	})

	// Verify that a daemon that refused the lease reports its own error.
	t.Run("Refused", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(Response{Type: TypeError, ID: req.ID, Error: "this connection already holds a lease"})
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if err := client.Lease(context.Background(), time.Second); err == nil ||
			err.Error() != "this connection already holds a lease" {
			t.Fatalf("a refused lease gave %v", err)
		}
	})

	// Verify that a daemon that stopped answering is reported.
	t.Run("Gone", func(t *testing.T) {
		scriptedDaemon(t, hangsUpAfterOneRequest)

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if err := client.Lease(context.Background(), time.Second); err == nil ||
			!strings.Contains(err.Error(), "the daemon stopped answering") {
			t.Fatalf("a daemon that hung up gave %v", err)
		}
	})

	// Verify that a request that could not be sent is reported.
	t.Run("Unwritable", func(t *testing.T) {
		if err := hungUpClient(t).Lease(context.Background(), time.Second); err == nil {
			t.Fatal("a request written to a closed socket was not reported")
		}
	})
}

// TestClientRelease tests the Client.Release method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the lease is given back once the daemon says so
//   - Refused: a daemon holding no lease for this connection reports its own error
//   - Gone: a daemon that stopped answering is reported
//   - Unwritable: a request that could not be sent is reported
func TestClientRelease(t *testing.T) {
	// Verify that the lease is given back once the daemon says so.
	t.Run("Success", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(
				Response{Type: TypeStarted, ID: req.ID},
				Response{Type: TypeReleased, ID: req.ID},
			)
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if err := client.Release(context.Background()); err != nil {
			t.Fatalf("giving a lease back: %v", err)
		}
	})

	// Verify that a daemon holding no lease for this connection reports its own error.
	t.Run("Refused", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(Response{Type: TypeError, ID: req.ID, Error: "this connection holds no lease"})
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if err := client.Release(context.Background()); err == nil ||
			err.Error() != "this connection holds no lease" {
			t.Fatalf("a refused release gave %v", err)
		}
	})

	// Verify that a daemon that stopped answering is reported.
	t.Run("Gone", func(t *testing.T) {
		scriptedDaemon(t, hangsUpAfterOneRequest)

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if err := client.Release(context.Background()); err == nil ||
			!strings.Contains(err.Error(), "the daemon stopped answering") {
			t.Fatalf("a daemon that hung up gave %v", err)
		}
	})

	// Verify that a request that could not be sent is reported.
	t.Run("Unwritable", func(t *testing.T) {
		if err := hungUpClient(t).Release(context.Background()); err == nil {
			t.Fatal("a request written to a closed socket was not reported")
		}
	})
}

// TestClientRun tests the Client.Run method with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Success: the two streams stay apart and the exit status comes back
//   - Failed: a command that failed reports its own error
//   - Skipped: a run that gave way is reported as skipped rather than failed
//   - Refused: a daemon that refused the run reports its own error
//   - Cancelled: cancelling asks the daemon to stop the run
//   - Gone: a daemon that stopped answering is reported
//   - Unwritable: a request that could not be sent is reported
func TestClientRun(t *testing.T) {
	// Verify that the two streams stay apart and the exit status comes back.
	t.Run("Success", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(
				Response{Type: TypeStarted, ID: req.ID, Argv: req.Argv},
				Response{Type: TypeStdout, ID: "somebody else", Data: "not mine"},
				Response{Type: TypeStdout, ID: req.ID, Data: "out"},
				Response{Type: TypeStderr, ID: req.ID, Data: "err"},
				Response{Type: TypeDone, ID: req.ID, Code: 3},
			)
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		var out, errs bytes.Buffer
		outcome, err := client.Run(context.Background(), []string{"version"}, "", &out, &errs)
		if err != nil {
			t.Fatalf("running a command: %v", err)
		}
		if outcome.Code != 3 || out.String() != "out" || errs.String() != "err" {
			t.Fatalf("the run gave %+v, %q and %q", outcome, out.String(), errs.String())
		}
	})

	// Verify that a command that failed reports its own error.
	t.Run("Failed", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(Response{Type: TypeDone, ID: req.ID, Code: 1, Error: "the scanner refused the key"})
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		outcome, err := client.Run(context.Background(), []string{"key", ">"}, ModeQueue, io.Discard, io.Discard)
		if err == nil || err.Error() != "the scanner refused the key" || outcome.Code != 1 {
			t.Fatalf("a failed command gave %+v and %v", outcome, err)
		}
	})

	// Verify that a run that gave way is reported as skipped rather than failed.
	t.Run("Skipped", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(Response{Type: TypeSkipped, ID: req.ID})
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		outcome, err := client.Run(context.Background(), []string{"version"}, ModeTry, io.Discard, io.Discard)
		if err != nil || !outcome.Skipped {
			t.Fatalf("a run that gave way gave %+v and %v", outcome, err)
		}
	})

	// Verify that a daemon that refused the run reports its own error.
	t.Run("Refused", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			send(Response{Type: TypeError, ID: req.ID, Error: "unknown op run"})
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		outcome, err := client.Run(context.Background(), []string{"version"}, "", io.Discard, io.Discard)
		if err == nil || err.Error() != "unknown op run" || outcome.Code != 1 {
			t.Fatalf("a refused run gave %+v and %v", outcome, err)
		}
	})

	// Verify that cancelling asks the daemon to stop the run.
	t.Run("Cancelled", func(t *testing.T) {
		scriptedDaemon(t, replyTo(func(req Request, send func(...Response)) {
			// Nothing is sent until the cancel arrives, which is what a daemon
			// running a long command does.
			if req.Op == OpCancel {
				send(Response{Type: TypeDone, ID: req.ID, Code: 1})
			}
		}))

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		ctx, stop := context.WithCancel(context.Background())
		go func() {
			time.Sleep(time.Millisecond)
			stop()
		}()

		outcome, err := client.Run(ctx, []string{"scan"}, "", io.Discard, io.Discard)
		if err != nil || outcome.Code != 1 {
			t.Fatalf("a cancelled run gave %+v and %v", outcome, err)
		}
	})

	// Verify that a daemon that stopped answering is reported.
	t.Run("Gone", func(t *testing.T) {
		scriptedDaemon(t, hangsUpAfterOneRequest)

		client, err := Dial("port0")
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		outcome, err := client.Run(context.Background(), []string{"version"}, "", io.Discard, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "the daemon stopped answering") || outcome.Code != 1 {
			t.Fatalf("a daemon that hung up gave %+v and %v", outcome, err)
		}
	})

	// Verify that a request that could not be sent is reported.
	t.Run("Unwritable", func(t *testing.T) {
		outcome, err := hungUpClient(t).Run(context.Background(), []string{"version"}, "", io.Discard, io.Discard)
		if err == nil || outcome.Code != 1 {
			t.Fatalf("a request written to a closed socket gave %+v and %v", outcome, err)
		}
	})
}

// TestAudioStreamSetBitrate tests the AudioStream.SetBitrate method with 100%
// coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the request reaches the daemon
//   - Unwritable: a request that could not be sent is reported
func TestAudioStreamSetBitrate(t *testing.T) {
	// Verify that the request reaches the daemon.
	t.Run("Success", func(t *testing.T) {
		client, server := socketPair(t)
		stream := &AudioStream{net: client, in: bufio.NewReader(client)}

		if err := stream.SetBitrate(16000); err != nil {
			t.Fatalf("asking for a rate: %v", err)
		}

		scan := bufio.NewScanner(server)
		if !scan.Scan() {
			t.Fatal("the daemon never saw the request")
		}
		var req Request
		if err := json.Unmarshal(scan.Bytes(), &req); err != nil {
			t.Fatal(err)
		}
		if req.Op != OpAudio || req.Action != "bitrate" || req.Bitrate != 16000 {
			t.Fatalf("the daemon saw %+v", req)
		}
	})

	// Verify that a request that could not be sent is reported.
	t.Run("Unwritable", func(t *testing.T) {
		client, _ := socketPair(t)
		client.Close()

		stream := &AudioStream{net: client, in: bufio.NewReader(client)}
		if err := stream.SetBitrate(16000); err == nil {
			t.Fatal("a request written to a closed socket was not reported")
		}
	})
}

// TestAudioStreamNext tests the AudioStream.Next method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Audio: an audio frame comes back with the number the capture gave it
//   - Short: an audio frame too small to carry a frame number is reported
//   - Event: news comes back as raw JSON with no audio
//   - Unknown: a frame kind this build has never heard of is stepped over
//   - Gone: a stream that ended is reported
func TestAudioStreamNext(t *testing.T) {
	// stream is a client end with the frames a test wants already on it.
	stream := func(t *testing.T, write func(w io.Writer)) *AudioStream {
		t.Helper()

		client, server := socketPair(t)
		go func() {
			write(server)
			server.Close()
		}()
		return &AudioStream{net: client, in: bufio.NewReader(client)}
	}

	// audioFrame is one frame of audio, numbered.
	audioFrame := func(seq uint32, payload []byte) []byte {
		body := make([]byte, 4+len(payload))
		binary.BigEndian.PutUint32(body, seq)
		copy(body[4:], payload)
		return body
	}

	// Verify that an audio frame comes back with the number the capture gave it.
	t.Run("Audio", func(t *testing.T) {
		s := stream(t, func(w io.Writer) {
			writeFrame(w, FrameAudio, audioFrame(7, []byte("samples")))
		})

		seq, audio, event, err := s.Next()
		if err != nil || seq != 7 || string(audio) != "samples" || event != nil {
			t.Fatalf("the frame came back as %d, %q, %s and %v", seq, audio, event, err)
		}
	})

	// Verify that an audio frame too small to carry a frame number is reported.
	t.Run("Short", func(t *testing.T) {
		s := stream(t, func(w io.Writer) {
			writeFrame(w, FrameAudio, []byte{1, 2})
		})

		if _, _, _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "too few for a frame number") {
			t.Fatalf("a short audio frame gave %v", err)
		}
	})

	// Verify that news comes back as raw JSON with no audio.
	t.Run("Event", func(t *testing.T) {
		s := stream(t, func(w io.Writer) {
			writeFrame(w, FrameJSON, []byte(`{"type":"silence"}`))
		})

		seq, audio, event, err := s.Next()
		if err != nil || seq != 0 || audio != nil || string(event) != `{"type":"silence"}` {
			t.Fatalf("the news came back as %d, %q, %s and %v", seq, audio, event, err)
		}
	})

	// Verify that a frame kind this build has never heard of is stepped over.
	t.Run("Unknown", func(t *testing.T) {
		s := stream(t, func(w io.Writer) {
			writeFrame(w, 99, []byte("from a later daemon"))
			writeFrame(w, FrameAudio, audioFrame(1, []byte("samples")))
		})

		seq, audio, _, err := s.Next()
		if err != nil || seq != 1 || string(audio) != "samples" {
			t.Fatalf("the frame after the unknown one came back as %d, %q and %v", seq, audio, err)
		}
	})

	// Verify that a stream that ended is reported.
	t.Run("Gone", func(t *testing.T) {
		s := stream(t, func(w io.Writer) {})

		if _, _, _, err := s.Next(); err == nil {
			t.Fatal("a stream that ended was not reported")
		}
	})
}

// TestAudioStreamClose tests the AudioStream.Close method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: hanging up ends the stream
func TestAudioStreamClose(t *testing.T) {
	// Verify that hanging up ends the stream.
	t.Run("Success", func(t *testing.T) {
		client, _ := socketPair(t)
		stream := &AudioStream{net: client, in: bufio.NewReader(client)}

		if err := stream.Close(); err != nil {
			t.Fatalf("hanging up: %v", err)
		}
	})
}

// TestClientClose tests the Client.Close method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: hanging up ends the connection
func TestClientClose(t *testing.T) {
	// Verify that hanging up ends the connection.
	t.Run("Success", func(t *testing.T) {
		client, _ := socketPair(t)

		if err := (&Client{net: client, in: bufio.NewScanner(client)}).Close(); err != nil {
			t.Fatalf("hanging up: %v", err)
		}
	})
}

// Test_read tests the Client.read method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: a line comes back as the message it carried
//   - HungUp: a daemon that hung up is reported as the end of the input
//   - Unreadable: a read that failed is reported
//   - Malformed: a line that is not a message at all is reported
func Test_read(t *testing.T) {
	// client is a client end with whatever the test wants already on it.
	client := func(t *testing.T, write func(w net.Conn)) *Client {
		t.Helper()

		clientEnd, server := socketPair(t)
		go func() {
			write(server)
			server.Close()
		}()

		c := &Client{net: clientEnd, in: bufio.NewScanner(clientEnd)}
		c.in.Buffer(make([]byte, 0, 4096), maxRequest)
		return c
	}

	// Verify that a line comes back as the message it carried.
	t.Run("Success", func(t *testing.T) {
		c := client(t, func(w net.Conn) { sendLines(w, Response{Type: TypeHello, Version: "test"}) })

		msg, err := c.read()
		if err != nil || msg.Type != TypeHello || msg.Version != "test" {
			t.Fatalf("the line came back as %+v and %v", msg, err)
		}
	})

	// Verify that a daemon that hung up is reported as the end of the input.
	t.Run("HungUp", func(t *testing.T) {
		c := client(t, func(w net.Conn) {})

		if _, err := c.read(); !errors.Is(err, io.EOF) {
			t.Fatalf("a daemon that hung up gave %v", err)
		}
	})

	// Verify that a read that failed is reported.
	t.Run("Unreadable", func(t *testing.T) {
		c := client(t, func(w net.Conn) {
			// Longer than the scanner will take, which is a read that fails
			// rather than an input that ended.
			w.Write(bytes.Repeat([]byte("x"), maxRequest+1))
		})

		if _, err := c.read(); err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("an oversized line gave %v", err)
		}
	})

	// Verify that a line that is not a message at all is reported.
	t.Run("Malformed", func(t *testing.T) {
		c := client(t, func(w net.Conn) { w.Write([]byte("this is not a message\n")) })

		if _, err := c.read(); err == nil {
			t.Fatal("a line that was not a message was not reported")
		}
	})
}

// Test_readFailure tests the readFailure function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Gone: a socket that went away is the daemon having stopped answering
//   - Unreadable: a message that would not parse says so, and says nothing
//     about the daemon having stopped
func Test_readFailure(t *testing.T) {
	// Verify that a socket that went away is reported as the daemon stopping.
	t.Run("Gone", func(t *testing.T) {
		err := readFailure(io.EOF)
		if err == nil || !strings.Contains(err.Error(), "the daemon stopped answering") {
			t.Fatalf("a socket that went away gave %v", err)
		}
	})

	// Verify that a message which would not parse is reported as that. Both
	// ends are running and disagree about the protocol, which is a rebuild
	// rather than a daemon to restart, and calling it a dead daemon sends the
	// reader looking in the wrong place.
	t.Run("Unreadable", func(t *testing.T) {
		err := readFailure(fmt.Errorf("%w: unexpected character", errUnreadable))
		if !errors.Is(err, errUnreadable) {
			t.Fatalf("a message that would not parse gave %v", err)
		}
		if strings.Contains(err.Error(), "stopped answering") {
			t.Fatalf("a message that would not parse was blamed on a dead daemon: %v", err)
		}
	})
}

// Test_line tests the AudioStream.line method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a line comes back as the message it carried
//   - Gone: a stream that ended is reported
//   - Malformed: a line that is not a message at all is reported
func Test_line(t *testing.T) {
	// stream is a client end with whatever the test wants already on it.
	stream := func(t *testing.T, write func(w net.Conn)) *AudioStream {
		t.Helper()

		client, server := socketPair(t)
		go func() {
			write(server)
			server.Close()
		}()
		return &AudioStream{net: client, in: bufio.NewReader(client)}
	}

	// Verify that a line comes back as the message it carried.
	t.Run("Success", func(t *testing.T) {
		s := stream(t, func(w net.Conn) { sendLines(w, Response{Type: TypeAudio, Rate: 48000}) })

		msg, err := s.line()
		if err != nil || msg.Type != TypeAudio || msg.Rate != 48000 {
			t.Fatalf("the line came back as %+v and %v", msg, err)
		}
	})

	// Verify that a stream that ended is reported.
	t.Run("Gone", func(t *testing.T) {
		s := stream(t, func(w net.Conn) {})

		if _, err := s.line(); err == nil {
			t.Fatal("a stream that ended was not reported")
		}
	})

	// Verify that a line that is not a message at all is reported.
	t.Run("Malformed", func(t *testing.T) {
		s := stream(t, func(w net.Conn) { w.Write([]byte("this is not a message\n")) })

		if _, err := s.line(); err == nil {
			t.Fatal("a line that was not a message was not reported")
		}
	})
}

// Test_openAudio tests the openAudio function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - NoGreeting: a daemon that hangs up without speaking answers ErrNoDaemon
//   - WrongGreeting: something that opened with anything else answers ErrNoDaemon
//   - WrongProtocol: a daemon from another build is reported
//   - Unwritable: a request that could not be sent is reported
//   - NoReply: a daemon that hung up before answering is reported
//   - Refused: a daemon with no audio to give reports its own error
//   - WrongReply: an answer that is not about audio is reported
func Test_openAudio(t *testing.T) {
	// paired is a socket with a daemon on the far end saying what the test says.
	paired := func(t *testing.T, speak func(w net.Conn)) net.Conn {
		t.Helper()

		client, server := socketPair(t)
		go func() {
			speak(server)
			server.Close()
		}()
		return client
	}

	// Verify that a daemon that hangs up without speaking answers ErrNoDaemon.
	t.Run("NoGreeting", func(t *testing.T) {
		_, err := openAudio(paired(t, func(w net.Conn) {}), FormatPCM, 0)
		if !errors.Is(err, ErrNoDaemon) || !strings.Contains(err.Error(), "never introduced itself") {
			t.Fatalf("a silent daemon gave %v", err)
		}
	})

	// Verify that something that opened with anything else answers ErrNoDaemon.
	t.Run("WrongGreeting", func(t *testing.T) {
		_, err := openAudio(paired(t, func(w net.Conn) {
			sendLines(w, Response{Type: TypeError, Error: "who are you"})
		}), FormatPCM, 0)

		if !errors.Is(err, ErrNoDaemon) || !strings.Contains(err.Error(), "it opened with") {
			t.Fatalf("a daemon that opened with an error gave %v", err)
		}
	})

	// Verify that a daemon from another build is reported.
	t.Run("WrongProtocol", func(t *testing.T) {
		_, err := openAudio(paired(t, func(w net.Conn) {
			sendLines(w, Response{Type: TypeHello, Protocol: Version + 1})
		}), FormatPCM, 0)

		if err == nil || !strings.Contains(err.Error(), "stop the daemon and start it from this build") {
			t.Fatalf("a daemon from another build gave %v", err)
		}
	})

	// Verify that a request that could not be sent is reported.
	t.Run("Unwritable", func(t *testing.T) {
		greeting, err := json.Marshal(Response{Type: TypeHello, Protocol: Version})
		if err != nil {
			t.Fatal(err)
		}

		conn := &haltingConn{in: bytes.NewReader(append(greeting, '\n'))}
		if _, err := openAudio(conn, FormatPCM, 0); err == nil {
			t.Fatal("a request that could not be written was not reported")
		}
	})

	// Verify that a daemon that hung up before answering is reported.
	t.Run("NoReply", func(t *testing.T) {
		_, err := openAudio(paired(t, hangsUpAfterOneRequest), FormatPCM, 0)

		if err == nil || !strings.Contains(err.Error(), "the daemon stopped answering") {
			t.Fatalf("a daemon that hung up gave %v", err)
		}
	})

	// Verify that a daemon with no audio to give reports its own error.
	t.Run("Refused", func(t *testing.T) {
		_, err := openAudio(paired(t, func(w net.Conn) {
			sendLines(w, Response{Type: TypeHello, Protocol: Version})
			bufio.NewReader(w).ReadBytes('\n')
			sendLines(w, Response{Type: TypeError, Error: "it is holding no sound input"})
		}), FormatPCM, 0)

		if err == nil || err.Error() != "it is holding no sound input" {
			t.Fatalf("a daemon with no audio gave %v", err)
		}
	})

	// Verify that an answer that is not about audio is reported.
	t.Run("WrongReply", func(t *testing.T) {
		_, err := openAudio(paired(t, func(w net.Conn) {
			sendLines(w, Response{Type: TypeHello, Protocol: Version})
			bufio.NewReader(w).ReadBytes('\n')
			sendLines(w, Response{Type: TypeDone})
		}), FormatPCM, 0)

		if err == nil || !strings.Contains(err.Error(), "answered a request for audio with") {
			t.Fatalf("an answer that was not about audio gave %v", err)
		}
	})
}

// haltingConn greets a client and then refuses to carry anything, which is a
// socket that went away between the greeting and the first request.
type haltingConn struct {
	in *bytes.Reader
}

// Read gives back the greeting, and then reports the end of the input.
func (c *haltingConn) Read(p []byte) (int, error) { return c.in.Read(p) }

// Write refuses everything.
func (c *haltingConn) Write(p []byte) (int, error) { return 0, errors.New("the socket has gone") }

// Close closes a socket that was never open.
func (c *haltingConn) Close() error { return nil }

// LocalAddr names this end, which is nowhere.
func (c *haltingConn) LocalAddr() net.Addr { return nil }

// RemoteAddr names the far end, which is nowhere.
func (c *haltingConn) RemoteAddr() net.Addr { return nil }

// SetDeadline accepts a deadline nothing here waits for.
func (c *haltingConn) SetDeadline(time.Time) error { return nil }

// SetReadDeadline accepts a deadline nothing here waits for.
func (c *haltingConn) SetReadDeadline(time.Time) error { return nil }

// SetWriteDeadline accepts a deadline nothing here waits for.
func (c *haltingConn) SetWriteDeadline(time.Time) error { return nil }
