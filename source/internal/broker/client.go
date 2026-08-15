// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package broker

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// Close hangs up, which is how a stream ends. The daemon gives the sound card
// back once nothing is listening to it.
//
// Returns:
//   - error if closing the socket failed
func (s *AudioStream) Close() error { return s.net.Close() }

// Close hangs up. Anything this client was holding is released by the daemon.
//
// Returns:
//   - error if closing the socket failed
func (c *Client) Close() error { return c.net.Close() }

// Commands describes every command the daemon can run.
//
// A caller that presents the tool rather than running it uses this instead of
// keeping a list of its own, so that a command added to the tool appears
// without anybody remembering to describe it twice.
//
// Parameters:
//   - ctx: for cancellation, and unused otherwise since the daemon answers at once
//
// Returns:
//   - every command the daemon can run
//   - error if the daemon stopped answering, or refused the request
func (c *Client) Commands(ctx context.Context) ([]appcontext.Command, error) {
	c.next++
	id := fmt.Sprint(c.next)

	if err := c.send(Request{Op: OpCommands, ID: id}); err != nil {
		return nil, err
	}

	for {
		msg, err := c.read()
		if err != nil {
			return nil, readFailure(err)
		}
		switch msg.Type {
		case TypeCommands:
			return msg.Commands, nil
		case TypeError:
			return nil, errors.New(msg.Error)
		}
	}
}

// Dial connects to the daemon for a port.
//
// It returns ErrNoDaemon when there is nothing to connect to, which is the
// ordinary case rather than a failure: most of the time nobody is sharing this
// scanner and the caller should carry on as it always did.
//
// Parameters:
//   - port: the serial port whose daemon to connect to
//
// Returns:
//   - a connection to the daemon holding that scanner
//   - error if there is no daemon, or it speaks another protocol version
//
// Errors:
//   - ErrNoDaemon: if nothing is listening, or what answered did not greet
//     this as a daemon would
func Dial(port string) (*Client, error) {
	path := socketPath(port)

	network, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		// Anything that stops the connection being made means there is no
		// daemon to talk to, whether the socket is missing or is a leftover
		// nobody is listening on. Both are the same answer to the caller.
		return nil, fmt.Errorf("%w: %v", ErrNoDaemon, err)
	}

	c := &Client{net: network, in: bufio.NewScanner(network)}
	c.in.Buffer(make([]byte, 0, 4096), maxRequest)

	hello, err := c.read()
	if err != nil {
		network.Close()
		return nil, fmt.Errorf("%w: it never introduced itself", ErrNoDaemon)
	}
	if hello.Type != TypeHello {
		network.Close()
		return nil, fmt.Errorf("%w: it opened with %q", ErrNoDaemon, hello.Type)
	}
	if hello.Protocol != Version {
		network.Close()
		return nil, fmt.Errorf("the daemon speaks protocol %d and this build speaks %d: "+
			"stop the daemon and start it from this build", hello.Protocol, Version)
	}

	c.Version, c.Protocol = hello.Version, hello.Protocol
	return c, nil
}

// DialAudio connects to the daemon for a port and asks it for audio.
//
// format is FormatPCM or FormatOpus, and bitrate is only consulted for Opus.
//
// Parameters:
//   - port: the serial port whose daemon to connect to
//   - format: FormatPCM or FormatOpus
//   - bitrate: bits per second for Opus, and 0 for the daemon's default
//
// Returns:
//   - a connection carrying audio and nothing else
//   - error if there is no daemon, it has no audio to give, or it speaks
//     another protocol version
//
// Errors:
//   - ErrNoDaemon: if nothing is listening for this port
func DialAudio(port, format string, bitrate int) (*AudioStream, error) {
	path := socketPath(port)

	network, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoDaemon, err)
	}
	return openAudio(network, format, bitrate)
}

// Info describes the stream, as the daemon reported it when it started.
func (s *AudioStream) Info() AudioInfo { return s.info }

// Lease claims the scanner across several runs, and returns once it is held,
// which for a caller queued behind another one is later than when it asked.
//
// Everything else waits until Release, the ttl runs out, or this connection
// closes. That last one is what stops a caller that crashed mid-sequence from
// holding the radio.
//
// Parameters:
//   - ctx: for cancellation while waiting for the lease to come free
//   - ttl: how long the lease may be held before the daemon takes it back
//
// Returns:
//   - error if the daemon stopped answering, or refused the lease
func (c *Client) Lease(ctx context.Context, ttl time.Duration) error {
	c.next++
	id := fmt.Sprint(c.next)

	if err := c.send(Request{Op: OpLease, ID: id, TTL: ttl.String()}); err != nil {
		return err
	}

	for {
		msg, err := c.read()
		if err != nil {
			return readFailure(err)
		}
		switch msg.Type {
		case TypeLeased:
			return nil
		case TypeError:
			return errors.New(msg.Error)
		}
	}
}

// Next reads the next frame.
//
// Audio comes back with the frame number the capture gave it. Anything else
// comes back with a nil payload and a non-nil event, so a caller looping over
// this sees news in the order it happened relative to the audio rather than on
// a channel of its own.
//
// Returns:
//   - seq: the frame number the capture gave this audio, counting 20 ms frames
//     since it started
//   - audio: the encoded frame, and nil for anything that is not audio
//   - event: one piece of news as raw JSON, and nil for audio
//   - err: the read's own error, which is how a stream ends
func (s *AudioStream) Next() (seq uint32, audio []byte, event json.RawMessage, err error) {
	for {
		kind, payload, err := readFrame(s.in)
		if err != nil {
			return 0, nil, nil, err
		}

		switch kind {
		case FrameAudio:
			if len(payload) < 4 {
				return 0, nil, nil, fmt.Errorf("an audio frame carried %d bytes, too few for a frame number",
					len(payload))
			}
			return binary.BigEndian.Uint32(payload), payload[4:], nil, nil

		case FrameJSON:
			return 0, nil, json.RawMessage(payload), nil

		default:
			// Ignored rather than refused. A later daemon inventing a frame
			// kind should cost this client that one frame, not the connection,
			// and the length in the header is what makes skipping it possible
			// without understanding it.
		}
	}
}

// Release gives a lease back.
//
// Parameters:
//   - ctx: for cancellation, and unused otherwise since the daemon answers at once
//
// Returns:
//   - error if the daemon stopped answering, or held no lease for this connection
func (c *Client) Release(ctx context.Context) error {
	c.next++
	id := fmt.Sprint(c.next)

	if err := c.send(Request{Op: OpRelease, ID: id}); err != nil {
		return err
	}

	for {
		msg, err := c.read()
		if err != nil {
			return readFailure(err)
		}
		switch msg.Type {
		case TypeReleased:
			return nil
		case TypeError:
			return errors.New(msg.Error)
		}
	}
}

// Run executes argv on the daemon, writing what it produces to stdout and
// stderr as it arrives.
//
// mode is ModeQueue or ModeTry; empty means ModeQueue. A cancelled context is
// forwarded, so a Ctrl-C here stops the command there rather than leaving the
// radio being driven for somebody who has gone.
//
// Parameters:
//   - ctx: cancelling it asks the daemon to stop the command
//   - argv: the already split argument list to run
//   - mode: ModeQueue or ModeTry, and empty means ModeQueue
//   - stdout: where the command's own stdout is written as it arrives
//   - stderr: where the command's own stderr is written as it arrives
//
// Returns:
//   - how the run ended, which is its exit status or that it gave way
//   - error if the daemon stopped answering, or the command itself failed
func (c *Client) Run(ctx context.Context, argv []string, mode string, stdout, stderr io.Writer) (Outcome, error) {
	c.next++
	id := fmt.Sprint(c.next)

	if err := c.send(Request{Op: OpRun, ID: id, Argv: argv, Mode: mode}); err != nil {
		return Outcome{Code: 1}, err
	}

	// Cancelling asks the daemon to stop rather than just hanging up, so the
	// command is interrupted at a point the daemon knows about.
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			c.send(Request{Op: OpCancel, ID: id})
		case <-stopped:
		}
	}()

	for {
		msg, err := c.read()
		if err != nil {
			return Outcome{Code: 1}, readFailure(err)
		}
		if msg.ID != "" && msg.ID != id {
			continue
		}

		switch msg.Type {
		case TypeStdout:
			io.WriteString(stdout, msg.Data)
		case TypeStderr:
			io.WriteString(stderr, msg.Data)
		case TypeStarted:
			// Nothing to do. It marks the moment the command reached the
			// front of the queue, which matters to a page drawing progress
			// and not to a terminal that is simply waiting.
		case TypeSkipped:
			return Outcome{Skipped: true}, nil
		case TypeDone:
			if msg.Error != "" {
				return Outcome{Code: msg.Code}, errors.New(msg.Error)
			}
			return Outcome{Code: msg.Code}, nil
		case TypeError:
			return Outcome{Code: 1}, errors.New(msg.Error)
		}
	}
}

// SetBitrate asks for a different rate, in bits per second.
//
// Nothing is returned to wait for. The daemon answers by sending smaller
// packets, and a refusal arrives as an ordinary message on the stream.
//
// Parameters:
//   - bps: the rate to ask for, in bits per second
//
// Returns:
//   - error if the request could not be written, which is the socket having gone
func (s *AudioStream) SetBitrate(bps int) error {
	return s.send(Request{Op: OpAudio, ID: "1", Action: "bitrate", Bitrate: bps})
}

// line reads one newline-delimited message, for the two that come before the
// framing changes.
//
// Returns:
//   - the message that line carried
//   - error if the read failed, or one wrapping errUnreadable if the line was
//     not a message at all
func (s *AudioStream) line() (Response, error) {
	data, err := s.in.ReadBytes('\n')
	if err != nil {
		return Response{}, err
	}

	var msg Response
	if err := json.Unmarshal(data, &msg); err != nil {
		return Response{}, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	return msg, nil
}

// openAudio does the handshake on an already connected socket.
//
// Split out from DialAudio so the part that matters can be tested over a pipe.
// What matters is the handover from lines to frames, and that is a property of
// this code rather than of the socket underneath it.
//
// Parameters:
//   - network: a socket already connected to the daemon
//   - format: FormatPCM or FormatOpus
//   - bitrate: bits per second for Opus, and 0 for the daemon's default
//
// Returns:
//   - a connection carrying audio and nothing else
//   - error if the daemon never greeted this, speaks another protocol version,
//     or refused to give audio
//
// Errors:
//   - ErrNoDaemon: if what answered did not greet this as a daemon would
func openAudio(network net.Conn, format string, bitrate int) (*AudioStream, error) {
	s := &AudioStream{net: network, in: bufio.NewReader(network)}

	hello, err := s.line()
	if err != nil {
		network.Close()
		return nil, fmt.Errorf("%w: it never introduced itself", ErrNoDaemon)
	}
	if hello.Type != TypeHello {
		network.Close()
		return nil, fmt.Errorf("%w: it opened with %q", ErrNoDaemon, hello.Type)
	}
	if hello.Protocol != Version {
		network.Close()
		return nil, fmt.Errorf("the daemon speaks protocol %d and this build speaks %d: "+
			"stop the daemon and start it from this build", hello.Protocol, Version)
	}

	if err := s.send(Request{Op: OpAudio, ID: "1", Format: format, Bitrate: bitrate}); err != nil {
		network.Close()
		return nil, err
	}

	// The reply is the last line this connection carries. Everything after the
	// newline that ends it is frames, read from this same reader.
	reply, err := s.line()
	if err != nil {
		network.Close()
		return nil, readFailure(err)
	}
	if reply.Type == TypeError {
		network.Close()
		return nil, errors.New(reply.Error)
	}
	if reply.Type != TypeAudio {
		network.Close()
		return nil, fmt.Errorf("the daemon answered a request for audio with %q", reply.Type)
	}

	s.info = AudioInfo{
		Format: reply.Format, Rate: reply.Rate, Channels: reply.Channels,
		FrameMS: reply.FrameMS, Bitrate: reply.Bitrate,
		Source: reply.Source, Channel: reply.Channel,
	}
	return s, nil
}

// read takes the next message.
//
// Returns:
//   - the message the daemon sent
//   - error if the read failed, io.EOF if the daemon hung up, or one wrapping
//     errUnreadable if what arrived was not a message
func (c *Client) read() (Response, error) {
	if !c.in.Scan() {
		if err := c.in.Err(); err != nil {
			return Response{}, err
		}
		return Response{}, io.EOF
	}

	var msg Response
	if err := json.Unmarshal(c.in.Bytes(), &msg); err != nil {
		return Response{}, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	return msg, nil
}

// readFailure names a failed read for the caller.
//
// Every one of them used to be reported as the daemon having stopped
// answering, which is right for a socket that went away and wrong for a message
// that arrived and would not parse: that is two builds disagreeing about the
// protocol, and telling somebody their daemon died sends them looking in the
// wrong place.
//
// Parameters:
//   - err: whatever read returned
//
// Returns:
//   - err itself when it already says what happened, and otherwise err wrapped
//     in the daemon having stopped answering
func readFailure(err error) error {
	if errors.Is(err, errUnreadable) {
		return err
	}
	return fmt.Errorf("the daemon stopped answering: %w", err)
}

// send writes one request as a line. The client-to-daemon direction stays
// newline JSON for the life of the connection.
//
// Parameters:
//   - req: the request to send
//
// Returns:
//   - error if the request could not be encoded or written
func (s *AudioStream) send(req Request) error {
	data, err := marshalJSON(req)
	if err != nil {
		return err
	}
	_, err = s.net.Write(append(data, '\n'))
	return err
}

// send writes one request.
//
// Parameters:
//   - req: the request to send
//
// Returns:
//   - error if the request could not be encoded or written
func (c *Client) send(req Request) error {
	data, err := marshalJSON(req)
	if err != nil {
		return err
	}
	_, err = c.net.Write(append(data, '\n'))
	return err
}
