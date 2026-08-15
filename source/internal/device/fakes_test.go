// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"context"
	"io"
	"log/slog"
	"time"

	"go.bug.st/serial"
)

// stubConn is a Conn that answers from the functions it is given, so a test can
// drive every typed command without a scanner attached.
type stubConn struct {
	info    Info
	exec    func(command string) (string, error)
	execXML func(command string) (string, error)
	send    func(command string) error
	closeFn func() error

	// commands records every command the scanner sent, in order, so a test can
	// assert the wire form a typed method produced.
	commands []string
}

// Info describes the scanner the stub is standing in for.
func (c *stubConn) Info() Info { return c.info }

// Execute records the command and answers from the stub's exec function.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) {
	c.commands = append(c.commands, command)
	if c.exec == nil {
		return "", nil
	}
	return c.exec(command)
}

// ExecuteXML records the command and answers from the stub's execXML function.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	c.commands = append(c.commands, command)
	if c.execXML == nil {
		return "", nil
	}
	return c.execXML(command)
}

// Send records the command and answers from the stub's send function.
func (c *stubConn) Send(ctx context.Context, command string) error {
	c.commands = append(c.commands, command)
	if c.send == nil {
		return nil
	}
	return c.send(command)
}

// Close answers from the stub's closeFn function.
func (c *stubConn) Close() error {
	if c.closeFn == nil {
		return nil
	}
	return c.closeFn()
}

// last returns the most recent command the stub was given, or "" if it was
// given none.
func (c *stubConn) last() string {
	if len(c.commands) == 0 {
		return ""
	}
	return c.commands[len(c.commands)-1]
}

// fakePort is a serial.Port that reads from a queue of canned chunks and
// records what was written to it.
//
// A read past the end of the queue returns no bytes and no error, which is what
// a real port with a read timeout does when the scanner has gone quiet. That is
// what lets a test drive the context deadline rather than the port's own.
type fakePort struct {
	chunks     []string // What the port hands back, one chunk per Read
	readErr    error    // Returned by Read once the chunks run out, if set
	partial    string   // Handed back together with readErr, for the read that returns both
	writes     []string // Every command written to the port, terminator included
	writeErr   error    // Returned by Write instead of writing, if set
	resetErr   error    // Returned by ResetInputBuffer, if set
	closeErr   error    // Returned by Close, if set
	timeoutErr error    // Returned by SetReadTimeout, if set
}

// Break does nothing, since nothing in this package sends one.
func (p *fakePort) Break(time.Duration) error { return nil }

// Close reports the fake's canned close result.
func (p *fakePort) Close() error { return p.closeErr }

// Drain does nothing, since nothing in this package drains.
func (p *fakePort) Drain() error { return nil }

// GetModemStatusBits reports nothing, since nothing in this package asks.
func (p *fakePort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

// Read hands back the next queued chunk. Once the queue is empty it reports the
// canned read error, or no bytes at all when there is none.
//
// The error is reported alongside partial rather than instead of it, which the
// io.Reader contract allows and which is the shape that used to lose bytes.
func (p *fakePort) Read(b []byte) (int, error) {
	if len(p.chunks) == 0 {
		if p.readErr != nil {
			return copy(b, p.partial), p.readErr
		}
		return 0, nil
	}

	chunk := p.chunks[0]
	p.chunks = p.chunks[1:]
	return copy(b, chunk), nil
}

// ResetInputBuffer reports the fake's canned reset result.
func (p *fakePort) ResetInputBuffer() error { return p.resetErr }

// ResetOutputBuffer does nothing, since nothing in this package resets it.
func (p *fakePort) ResetOutputBuffer() error { return nil }

// SetDTR does nothing, since nothing in this package sets it.
func (p *fakePort) SetDTR(bool) error { return nil }

// SetMode does nothing, since the mode is fixed when the port is opened.
func (p *fakePort) SetMode(*serial.Mode) error { return nil }

// SetRTS does nothing, since nothing in this package sets it.
func (p *fakePort) SetRTS(bool) error { return nil }

// SetReadTimeout reports the fake's canned result, since configuring the port
// is one of the ways opening it can fail.
func (p *fakePort) SetReadTimeout(time.Duration) error { return p.timeoutErr }

// Write records the command, or reports the fake's canned write error.
func (p *fakePort) Write(b []byte) (int, error) {
	if p.writeErr != nil {
		return 0, p.writeErr
	}
	p.writes = append(p.writes, string(b))
	return len(b), nil
}

// discardLog is a logger that throws everything away, for the conn tests, which
// care about what came back rather than what was logged.
func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
