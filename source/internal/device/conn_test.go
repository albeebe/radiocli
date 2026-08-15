// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// newTestConn builds a conn around a fake port, which is the shape every test
// in this file needs.
//
// Parameters:
//   - port: the fake port the connection reads and writes through
//
// Returns:
//   - a conn wired to port, logging nowhere, naming the port in its errors
func newTestConn(port *fakePort) *conn {
	return &conn{port: port, log: discardLog(), info: Info{Port: "/dev/fake"}}
}

// deadCtx returns a context that has already expired, so any wait inside the
// connection ends at once rather than burning a real timeout.
//
// Returns:
//   - a context whose deadline has already passed
func deadCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// Test_connClose tests the conn Close method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - AlreadyClosed: a connection with no port reports nothing
//   - Success: the port is closed and the claim released
//   - CloseError: a port that will not close is reported, naming the port
//   - DoesNotWaitOutACommand: a command in flight is cut short rather than
//     waited out
func Test_connClose(t *testing.T) {
	// Verify that closing a connection twice is not an error.
	t.Run("AlreadyClosed", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		if err := c.Close(); err != nil {
			t.Fatalf("closing: %v", err)
		}
		if err := c.Close(); err != nil {
			t.Errorf("closing again: %v", err)
		}
	})

	// Verify that a successful close drops the port and the claim.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		if err := c.Close(); err != nil {
			t.Fatalf("closing: %v", err)
		}
		if c.port != nil {
			t.Error("the port was kept after closing")
		}
		if c.lock != nil {
			t.Error("the claim was kept after closing")
		}
	})

	// Verify that a port that refuses to close is reported.
	t.Run("CloseError", func(t *testing.T) {
		c := newTestConn(&fakePort{closeErr: errors.New("the port is gone")})
		err := c.Close()
		if err == nil {
			t.Fatal("closing a broken port reported nothing")
		}
		if !strings.Contains(err.Error(), "closing /dev/fake") {
			t.Errorf("error %q does not name the port", err)
		}
	})

	// Verify that a command already on the wire does not hold the close up.
	// A command owns the connection for its whole exchange, which for a
	// position write is thirty seconds, and a Ctrl-C that merely queued behind
	// that lock would wait all of it out.
	t.Run("DoesNotWaitOutACommand", func(t *testing.T) {
		c := newTestConn(&fakePort{})

		// A silent port and an allowance measured in seconds, which is what a
		// close would otherwise be waiting for.
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		running := make(chan struct{})
		go func() {
			defer close(running)
			c.Execute(ctx, "LCR,38.0,-79.0,10.0")
		}()

		if err := c.Close(); err != nil {
			t.Fatalf("closing: %v", err)
		}

		select {
		case <-running:
		case <-time.After(5 * time.Second):
			t.Fatal("the command carried on after the connection was closed")
		}
	})
}

// Test_connExecute tests the conn Execute method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the echoed command is stripped from the reply
//   - ExchangeError: a failed exchange is reported
func Test_connExecute(t *testing.T) {
	// Verify that a normal reply comes back with its echo removed.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"MDL,SDS150\r"}})
		value, err := c.Execute(context.Background(), "MDL")
		if err != nil {
			t.Fatalf("executing: %v", err)
		}
		if value != "SDS150" {
			t.Errorf("got %q, want SDS150", value)
		}
	})

	// Verify that an exchange that never answers is reported.
	t.Run("ExchangeError", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		if _, err := c.Execute(deadCtx(t), "MDL"); err == nil {
			t.Fatal("a silent port reported no error")
		}
	})
}

// Test_connExecuteXML tests the conn ExecuteXML method with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Success: the header is accepted and the document read to its root's close
//   - WriteError: a closed connection is reported before anything is sent
//   - ResponseError: a scanner that never answers is reported
//   - RejectedError: a refusal in the header is reported
//   - NotXML: a header that is not an XML header is reported
//   - DocumentCutOff: a document that stops early is reported
func Test_connExecuteXML(t *testing.T) {
	// Verify that a header and a document are read as one exchange.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{
			"GLT,<XML>\r<?xml version=\"1.0\"?>\r<GLT>\r<FL Name=\"A\"/>\r</GLT>\r",
		}})
		doc, err := c.ExecuteXML(context.Background(), "GLT,FL")
		if err != nil {
			t.Fatalf("executing: %v", err)
		}
		if !strings.HasSuffix(doc, "</GLT>\n") {
			t.Errorf("document %q does not end at the root element", doc)
		}
	})

	// Verify that writing to a closed connection is reported.
	t.Run("WriteError", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.port = nil
		if _, err := c.ExecuteXML(context.Background(), "GLT,FL"); err == nil {
			t.Fatal("a closed connection reported no error")
		}
	})

	// Verify that a scanner that never answers the header is reported.
	t.Run("ResponseError", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		if _, err := c.ExecuteXML(deadCtx(t), "GLT,FL"); err == nil {
			t.Fatal("a silent port reported no error")
		}
	})

	// Verify that a refusal in the header is reported as a rejection.
	t.Run("RejectedError", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"GLT,NG\r"}})
		_, err := c.ExecuteXML(context.Background(), "GLT,FL")
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("got %v, want ErrRejected", err)
		}
	})

	// Verify that a header promising no document is reported.
	t.Run("NotXML", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"GLT,OK\r"}})
		_, err := c.ExecuteXML(context.Background(), "GLT,FL")
		if err == nil || !strings.Contains(err.Error(), "expected an XML response") {
			t.Fatalf("got %v, want an XML header complaint", err)
		}
	})

	// Verify that a document that stops before its root closes is reported.
	t.Run("DocumentCutOff", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		c := newTestConn(&fakePort{chunks: []string{"GLT,<XML>\r<GLT>\r"}})
		_, err := c.ExecuteXML(ctx, "GLT,FL")
		if err == nil || !strings.Contains(err.Error(), "cut off") {
			t.Fatalf("got %v, want a cut off document", err)
		}
	})
}

// Test_connInfo tests the conn Info method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the connection reports the scanner it was built for
func Test_connInfo(t *testing.T) {
	// Verify that Info reports what the connection was built with.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.info.Model = "SDS150"
		if got := c.Info(); got.Model != "SDS150" || got.Port != "/dev/fake" {
			t.Errorf("got %+v, want the port and model it was built with", got)
		}
	})
}

// Test_connSend tests the conn Send method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the command is written and nothing is waited for
//   - ContextDone: a context already over is reported before anything is sent
//   - WriteError: a closed connection is reported
func Test_connSend(t *testing.T) {
	// Verify that a command is written with its terminator.
	t.Run("Success", func(t *testing.T) {
		port := &fakePort{}
		c := newTestConn(port)
		if err := c.Send(context.Background(), "KAL"); err != nil {
			t.Fatalf("sending: %v", err)
		}
		if len(port.writes) != 1 || port.writes[0] != "KAL\r" {
			t.Errorf("wrote %q, want KAL with a carriage return", port.writes)
		}
	})

	// Verify that a context already over stops the write.
	t.Run("ContextDone", func(t *testing.T) {
		port := &fakePort{}
		c := newTestConn(port)
		if err := c.Send(deadCtx(t), "KAL"); err == nil {
			t.Fatal("a cancelled context reported no error")
		}
		if len(port.writes) != 0 {
			t.Errorf("wrote %q after the context was over", port.writes)
		}
	})

	// Verify that a closed connection is reported.
	t.Run("WriteError", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.port = nil
		if err := c.Send(context.Background(), "KAL"); err == nil {
			t.Fatal("a closed connection reported no error")
		}
	})
}

// Test_allowanceFor tests the allowanceFor function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - LocationWrite: writing a position gets the long allowance
//   - LocationRead: the bare read gets it too, since the name is the whole test
//   - OtherCommand: everything else gets the ordinary allowance
func Test_allowanceFor(t *testing.T) {
	// Verify that a position write gets the rebuild allowance.
	t.Run("LocationWrite", func(t *testing.T) {
		if got := allowanceFor("LCR,38.000000,-79.000000,10.0"); got != rebuildTimeout {
			t.Errorf("got %v, want %v", got, rebuildTimeout)
		}
	})

	// Verify that the command name alone decides it, whatever its case.
	t.Run("LocationRead", func(t *testing.T) {
		if got := allowanceFor("lcr"); got != rebuildTimeout {
			t.Errorf("got %v, want %v", got, rebuildTimeout)
		}
	})

	// Verify that every other command gets the ordinary allowance.
	t.Run("OtherCommand", func(t *testing.T) {
		if got := allowanceFor("STS"); got != commandTimeout {
			t.Errorf("got %v, want %v", got, commandTimeout)
		}
	})
}

// Test_connExchange tests the conn exchange method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the command is written and its raw reply returned
//   - WriteError: a closed connection is reported before anything is read
func Test_connExchange(t *testing.T) {
	// Verify that the raw reply comes back with its echo still on it.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"VOL,7\r"}})
		raw, err := c.exchange(context.Background(), "VOL")
		if err != nil {
			t.Fatalf("exchanging: %v", err)
		}
		if raw != "VOL,7" {
			t.Errorf("got %q, want VOL,7", raw)
		}
	})

	// Verify that a closed connection is reported.
	t.Run("WriteError", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.port = nil
		if _, err := c.exchange(context.Background(), "VOL"); err == nil {
			t.Fatal("a closed connection reported no error")
		}
	})
}

// Test_connReadLine tests the conn readLine method with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - Success: a line ending in a carriage return is returned without it
//   - AcrossReads: a line spread over two reads is assembled
//   - SkipsBlankLines: an empty line is skipped rather than returned
//   - ReadError: a port that cannot be read is reported, naming the port
//   - KeepsBytesReadWithAnError: bytes handed back with an error are not lost
//   - Closing: a connection being closed gives up rather than reading on
//   - TimeoutEmpty: a silent port is reported as no response
//   - TimeoutPartial: a half arrived line is reported as cut off, quoting it
func Test_connReadLine(t *testing.T) {
	// Verify that a whole line is returned without its terminator.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"STS,0\r"}})
		line, err := c.readLine(context.Background())
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if line != "STS,0" {
			t.Errorf("got %q, want STS,0", line)
		}
	})

	// Verify that a line split across two reads is put back together.
	t.Run("AcrossReads", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"STS,", "0\r"}})
		line, err := c.readLine(context.Background())
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if line != "STS,0" {
			t.Errorf("got %q, want STS,0", line)
		}
	})

	// Verify that a carriage return and line feed pair yields one line.
	t.Run("SkipsBlankLines", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"\r\nSTS,0\r\n"}})
		line, err := c.readLine(context.Background())
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if line != "STS,0" {
			t.Errorf("got %q, want STS,0", line)
		}
	})

	// Verify that a port that cannot be read names itself in the error.
	t.Run("ReadError", func(t *testing.T) {
		c := newTestConn(&fakePort{readErr: errors.New("the port is gone")})
		_, err := c.readLine(context.Background())
		if err == nil || !strings.Contains(err.Error(), "reading from /dev/fake") {
			t.Fatalf("got %v, want the port named", err)
		}
	})

	// Verify that bytes handed back alongside an error are kept. The io.Reader
	// contract allows a read to return both, and dropping the bytes loses the
	// front of a line that the next read would have finished.
	t.Run("KeepsBytesReadWithAnError", func(t *testing.T) {
		c := newTestConn(&fakePort{partial: "STS,", readErr: errors.New("the port is gone")})
		if _, err := c.readLine(context.Background()); err == nil {
			t.Fatal("a failed read was reported as a line")
		}
		if string(c.pending) != "STS," {
			t.Errorf("pending holds %q, want the bytes that arrived with the error", c.pending)
		}
	})

	// Verify that a connection being closed stops waiting on the port. This is
	// what keeps a Ctrl-C from sitting behind a thirty second position write.
	t.Run("Closing", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.closing.Store(true)

		_, err := c.readLine(context.Background())
		if err == nil || !strings.Contains(err.Error(), "is closing") {
			t.Fatalf("got %v, want the close reported", err)
		}
	})

	// Verify that a silent port is reported as no response at all.
	t.Run("TimeoutEmpty", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		_, err := c.readLine(deadCtx(t))
		if err == nil || !strings.Contains(err.Error(), "no response from scanner") {
			t.Fatalf("got %v, want no response", err)
		}
	})

	// Verify that a line that stopped halfway is quoted back.
	t.Run("TimeoutPartial", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.pending = []byte("STS,0")
		_, err := c.readLine(deadCtx(t))
		if err == nil || !strings.Contains(err.Error(), `cut off after "STS,0"`) {
			t.Fatalf("got %v, want the partial line quoted", err)
		}
	})
}

// Test_connReadResponse tests the conn readResponse method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the line belonging to the command is returned
//   - DiscardsStale: a late answer to an earlier command is skipped
//   - ReadError: a silent port is reported, naming the command
func Test_connReadResponse(t *testing.T) {
	// Verify that the matching line is returned.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"VOL,7\r"}})
		raw, err := c.readResponse(context.Background(), "VOL")
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if raw != "VOL,7" {
			t.Errorf("got %q, want VOL,7", raw)
		}
	})

	// Verify that an answer to an earlier command is thrown away.
	t.Run("DiscardsStale", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"LCR,38.0,-79.0,10.0\rVOL,7\r"}})
		raw, err := c.readResponse(context.Background(), "VOL")
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if raw != "VOL,7" {
			t.Errorf("got %q, want the late LCR reply discarded", raw)
		}
	})

	// Verify that a silent port names the command that went unanswered.
	t.Run("ReadError", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		_, err := c.readResponse(deadCtx(t), "VOL")
		if err == nil || !strings.Contains(err.Error(), `sending "VOL"`) {
			t.Fatalf("got %v, want the command named", err)
		}
	})
}

// Test_connReadXML tests the conn readXML method with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Success: the document is collected up to its root element's close
//   - SkipsDeclaration: a declaration does not open a root
//   - SelfClosingRoot: a root that closes itself is the whole document
//   - CutOff: a document that stops early reports how much had arrived
//   - NothingArrived: a document that never starts reports the read error alone
func Test_connReadXML(t *testing.T) {
	// Verify that the document ends at the close of its root element.
	t.Run("Success", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"<GLT>\r<FL/>\r</GLT>\rTRAILING\r"}})
		doc, err := c.readXML(context.Background(), "GLT,FL")
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if doc != "<GLT>\n<FL/>\n</GLT>\n" {
			t.Errorf("got %q, want the document up to its root's close", doc)
		}
	})

	// Verify that a declaration is not the root.
	t.Run("SkipsDeclaration", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{
			"<?xml version=\"1.0\"?>\r<GSI>\r</GSI>\r",
		}})
		doc, err := c.readXML(context.Background(), "GSI")
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if !strings.HasSuffix(doc, "</GSI>\n") {
			t.Errorf("got %q, want the root taken as GSI", doc)
		}
	})

	// Verify that a root closing itself ends the document there. Nothing else
	// ever will: it carries no closing tag to wait for, so a reader waiting for
	// one parks the caller until the deadline and then calls a complete
	// document cut off.
	t.Run("SelfClosingRoot", func(t *testing.T) {
		c := newTestConn(&fakePort{chunks: []string{"<?xml version=\"1.0\"?>\r<GLT/>\r"}})
		doc, err := c.readXML(context.Background(), "GLT,FL")
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if doc != "<?xml version=\"1.0\"?>\n<GLT/>\n" {
			t.Errorf("got %q, want the document ending at the self closing root", doc)
		}
	})

	// Verify that a document cut short says how much had arrived.
	t.Run("CutOff", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.pending = []byte("<GLT>\r")
		_, err := c.readXML(deadCtx(t), "GLT,FL")
		if err == nil || !strings.Contains(err.Error(), "XML document was cut off after 6 characters") {
			t.Fatalf("got %v, want the length reported", err)
		}
	})

	// Verify that a document that never started reports the read failure alone.
	t.Run("NothingArrived", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		_, err := c.readXML(deadCtx(t), "GLT,FL")
		if err == nil || strings.Contains(err.Error(), "cut off") {
			t.Fatalf("got %v, want the bare read failure", err)
		}
	})
}

// Test_rootElement tests the rootElement function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Lines: every shape of line is named or refused as the table says
func Test_rootElement(t *testing.T) {
	// Verify that only a line opening an element names one, and that an element
	// closing itself is named and reported closed.
	t.Run("Lines", func(t *testing.T) {
		for line, want := range map[string]struct {
			name   string
			closed bool
		}{
			"<GLT>":                   {"GLT", false},
			"<GLT Name=\"A\">":        {"GLT", false},
			"<GLT\tName=\"A\">":       {"GLT", false},
			"<?xml version=\"1.0\"?>": {"", false},
			"</GLT>":                  {"", false},
			"<GLT/>":                  {"GLT", true},
			"<FL Name=\"A\"/>":        {"FL", true},
			"plain text":              {"", false},
			"":                        {"", false},
			"<Unclosed":               {"Unclosed", false},
			"<>":                      {"", false},
		} {
			name, closed := rootElement(line)
			if name != want.name || closed != want.closed {
				t.Errorf("rootElement(%q) is (%q, %v), want (%q, %v)",
					line, name, closed, want.name, want.closed)
			}
		}
	})
}

// Test_connWrite tests the conn write method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the command goes out with its terminator and the buffer cleared
//   - Closed: a connection with no port is reported, naming the port
//   - ResetError: a buffer that cannot be cleared is reported
//   - WriteError: a port that will not take the command is reported
func Test_connWrite(t *testing.T) {
	// Verify that the command is written and stale input dropped first.
	t.Run("Success", func(t *testing.T) {
		port := &fakePort{}
		c := newTestConn(port)
		c.pending = []byte("left over")

		if err := c.write("VOL,7"); err != nil {
			t.Fatalf("writing: %v", err)
		}
		if len(c.pending) != 0 {
			t.Errorf("kept %q, want the stale input dropped", c.pending)
		}
		if len(port.writes) != 1 || port.writes[0] != "VOL,7\r" {
			t.Errorf("wrote %q, want VOL,7 with a carriage return", port.writes)
		}
	})

	// Verify that a closed connection names the port it used to hold.
	t.Run("Closed", func(t *testing.T) {
		c := newTestConn(&fakePort{})
		c.port = nil
		err := c.write("VOL,7")
		if err == nil || !strings.Contains(err.Error(), "connection to /dev/fake is closed") {
			t.Fatalf("got %v, want the closed connection named", err)
		}
	})

	// Verify that a buffer that cannot be cleared is reported.
	t.Run("ResetError", func(t *testing.T) {
		c := newTestConn(&fakePort{resetErr: errors.New("the port is gone")})
		err := c.write("VOL,7")
		if err == nil || !strings.Contains(err.Error(), "clearing input buffer") {
			t.Fatalf("got %v, want a cleared buffer complaint", err)
		}
	})

	// Verify that a refused write is reported, naming the command.
	t.Run("WriteError", func(t *testing.T) {
		c := newTestConn(&fakePort{writeErr: errors.New("the port is gone")})
		err := c.write("VOL,7")
		if err == nil || !strings.Contains(err.Error(), `sending "VOL,7"`) {
			t.Fatalf("got %v, want the command named", err)
		}
	})
}
