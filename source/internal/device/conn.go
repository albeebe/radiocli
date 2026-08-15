// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
)

// Close releases the serial port, and the claim on it.
//
// The claim goes last. Between closing the port and dropping the lock another
// invocation can be waiting, and letting it in before this one has actually
// let go of the port would hand it exactly the collision the lock is for.
//
// The closing flag is raised before the exchange lock is asked for, and that
// ordering is what stops a Ctrl-C waiting out an in-flight command. A command
// holds the lock for its whole exchange, which for a position write is thirty
// seconds, so a Close that only asked for the lock would sit behind it doing
// nothing. Raising the flag first ends the wait inside readLine within one
// read poll, and this then takes the lock the moment that command unwinds.
//
// Returns:
//   - error if the port fails to close or the claim cannot be released, nil
//     if the connection is already closed
func (c *conn) Close() error {
	c.closing.Store(true)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.port == nil {
		return nil
	}
	err := c.port.Close()
	c.port = nil

	lockErr := c.lock.Release()
	c.lock = nil

	if err != nil {
		return fmt.Errorf("closing %s: %w", c.info.Port, err)
	}
	return lockErr
}

// Execute sends one command and returns its response value.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - command: the command line to send, without its terminator
//
// Returns:
//   - the response value, with the echoed command stripped
//   - error if the exchange fails, which is ErrUnsupported when the scanner
//     does not know the command and ErrRejected when it refuses to run it
func (c *conn) Execute(ctx context.Context, command string) (string, error) {
	raw, err := c.exchange(ctx, command)
	if err != nil {
		return "", err
	}
	return parseResponse(command, raw)
}

// ExecuteXML sends one command and returns the XML document it answers with.
//
// These responses arrive as a header line ending in `<XML>`, followed by the
// document on its own lines. There is no length prefix and no reliable end
// marker: some documents end with a footer carrying EOT="1" and some do not.
// What every document does have is a single root element, so this reads until
// that element closes.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - command: the command line to send, without its terminator
//
// Returns:
//   - the XML document, ending with the closing tag of its root element
//   - error if the exchange fails, the response is not an XML header, or the
//     document is cut off before its root element closes
func (c *conn) ExecuteXML(ctx context.Context, command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, xmlTimeout)
	defer cancel()

	if err := c.write(command); err != nil {
		return "", err
	}

	header, err := c.readResponse(ctx, command)
	if err != nil {
		return "", err
	}
	if _, err := parseResponse(command, header); err != nil {
		return "", err
	}
	if !strings.Contains(header, xmlValue) {
		return "", fmt.Errorf("expected an XML response to %q, got %q", command, header)
	}

	doc, err := c.readXML(ctx, command)
	if err != nil {
		return "", fmt.Errorf("sending %q: %w", command, err)
	}

	c.log.Debug("received XML response", "command", command, "bytes", len(doc))
	return doc, nil
}

// Info describes the connected scanner.
func (c *conn) Info() Info {
	return c.info
}

// Send writes a command and does not wait for a response.
//
// Parameters:
//   - ctx: context for cancellation, checked before anything is written
//   - command: the command line to send, without its terminator
//
// Returns:
//   - error if ctx is already done or the command cannot be written
func (c *conn) Send(ctx context.Context, command string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	return c.write(command)
}

// allowanceFor reports how long a command is given to answer.
//
// Almost everything answers immediately, and a scanner that has not replied in
// three seconds is not going to. Writing a position is the exception: it sends
// the scanner off to work out which of the database's channels are in range of
// the new position, and it says nothing at all until that is done.
//
// It belongs here rather than at the call site because it is a property of the
// command, not of who is asking. The command name is the whole of what decides
// it, so matching on that is as specific as the question is.
//
// Parameters:
//   - command: the command line about to be sent, with any arguments
//
// Returns:
//   - rebuildTimeout for a command that writes a position, commandTimeout for
//     everything else
func allowanceFor(command string) time.Duration {
	if name, _, _ := strings.Cut(command, ","); strings.EqualFold(name, locationCommand) {
		return rebuildTimeout
	}
	return commandTimeout
}

// exchange sends a command and returns its raw response line.
//
// Parameters:
//   - ctx: context for cancellation, bounded further by the command's own
//     allowance
//   - command: the command line to send, without its terminator
//
// Returns:
//   - the response line as it arrived, with the echoed command still on it
//   - error if the command cannot be written or no response arrives in time
func (c *conn) exchange(ctx context.Context, command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, allowanceFor(command))
	defer cancel()

	if err := c.write(command); err != nil {
		return "", err
	}

	return c.readResponse(ctx, command)
}

// readLine reads until the terminator or until ctx runs out.
//
// The port's own read timeout only bounds a single read; a scanner that is
// powered on but not answering would otherwise keep returning zero bytes
// forever, so the context deadline is what actually ends the wait.
//
// Parameters:
//   - ctx: context whose deadline bounds the whole wait, not a single read
//
// Returns:
//   - the next non-empty line, without its terminator
//   - error if the port cannot be read, or if ctx runs out first, naming
//     whatever had arrived by then
func (c *conn) readLine(ctx context.Context) (string, error) {
	buf := make([]byte, 512)

	for {
		// Some responses end with a line feed rather than a carriage return,
		// so either ends a line and neither is content. An empty line is
		// skipped rather than returned, which also absorbs a \r\n pair.
		if i := bytes.IndexAny(c.pending, "\r\n"); i >= 0 {
			line := string(c.pending[:i])
			c.pending = c.pending[i+1:]
			if line == "" {
				continue
			}
			return line, nil
		}

		if err := ctx.Err(); err != nil {
			if len(c.pending) > 0 {
				return "", fmt.Errorf("response was cut off after %q: %w", c.pending, err)
			}
			return "", fmt.Errorf("no response from scanner: %w", err)
		}

		// Somebody is closing the connection and is waiting on the lock this
		// exchange holds. Giving up here rather than reading on is what keeps a
		// Ctrl-C from waiting out a thirty second position write.
		if c.closing.Load() {
			return "", fmt.Errorf("connection to %s is closing", c.info.Port)
		}

		n, err := c.port.Read(buf)

		// Appended before the error is looked at, because the io.Reader
		// contract allows a read to return bytes and an error together.
		// Dropping those bytes would silently lose the front of a line that the
		// next read would otherwise finish.
		c.pending = append(c.pending, buf[:n]...)
		if err != nil {
			return "", fmt.Errorf("reading from %s: %w", c.info.Port, err)
		}
	}
}

// readResponse reads lines until one belongs to command.
//
// A command that ran out of time is not necessarily unanswered: the scanner
// replies when it is ready, and that reply arrives while a later command is
// waiting for its own. Clearing the port before sending cannot catch it,
// because it is still in flight at that point. Every response echoes the name
// of the command it answers, so one carrying a different name is discarded.
//
// This matters wherever the scanner goes quiet under load and comes back:
// accepting a new location sends it away to rebuild from the database for
// several seconds, and without this every command afterwards reads the
// previous command's answer.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - command: the command whose response is being waited for
//
// Returns:
//   - the response line belonging to command, with its echo still on it
//   - error if no line arrives before ctx runs out
func (c *conn) readResponse(ctx context.Context, command string) (string, error) {
	for {
		raw, err := c.readLine(ctx)
		if err != nil {
			return "", fmt.Errorf("sending %q: %w", command, err)
		}

		if stale(command, raw) {
			c.log.Debug("discarding a late response to an earlier command",
				"command", command, "response", raw)
			continue
		}

		c.log.Debug("received response", "command", command, "response", raw)
		return raw, nil
	}
}

// readXML collects lines until the document's root element closes.
//
// A root element that closes itself is the whole document, and the reason this
// has to be recognised rather than waited out is that nothing else will ever
// end the read. An empty listing arrives as a single `<GLT/>` and carries no
// closing tag to look for, so a reader waiting for one parks the caller until
// the deadline and then reports a complete document as cut off.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - command: the command whose document is being read
//
// Returns:
//   - the document, one line per line the scanner sent
//   - error if a line cannot be read before the root element closes, saying
//     how much of the document had arrived
func (c *conn) readXML(ctx context.Context, command string) (string, error) {
	var doc strings.Builder
	var root string

	for {
		line, err := c.readLine(ctx)
		if err != nil {
			if doc.Len() > 0 {
				return "", fmt.Errorf("XML document was cut off after %d characters: %w", doc.Len(), err)
			}
			return "", err
		}

		doc.WriteString(line)
		doc.WriteByte('\n')

		trimmed := strings.TrimSpace(line)
		if root == "" {
			name, closed := rootElement(trimmed)
			if name != "" && closed {
				return doc.String(), nil
			}
			root = name
			continue
		}
		if trimmed == "</"+root+">" {
			return doc.String(), nil
		}
	}
}

// rootElement returns the name of the document's root element, and whether
// that element closed itself on the same line. The XML declaration is skipped.
//
// Parameters:
//   - line: one line of the document, with surrounding space already removed
//
// Returns:
//   - the element's name, or "" if the line does not open one
//   - whether the element closes itself, and so is a document all on its own
func rootElement(line string) (string, bool) {
	if !strings.HasPrefix(line, "<") || strings.HasPrefix(line, "<?") || strings.HasPrefix(line, "</") {
		return "", false
	}

	// The slash of a self-closing element ends the name as surely as a space
	// does, so it is one of the characters cut on rather than a special case.
	name := strings.TrimPrefix(line, "<")
	if i := strings.IndexAny(name, " \t>/"); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		return "", false
	}
	return name, strings.HasSuffix(line, "/>")
}

// write clears any stale input and sends one command.
//
// Parameters:
//   - command: the command line to send, without its terminator
//
// Returns:
//   - error if the connection is closed, the input buffer cannot be cleared,
//     or the write fails
func (c *conn) write(command string) error {
	if c.port == nil {
		return fmt.Errorf("connection to %s is closed", c.info.Port)
	}

	// Drop anything left over from an interrupted exchange, in the port and
	// in our own buffer, so the reply we read belongs to the command we are
	// about to send.
	c.pending = c.pending[:0]
	if err := c.port.ResetInputBuffer(); err != nil {
		return fmt.Errorf("clearing input buffer: %w", err)
	}

	c.log.Debug("sending command", "command", command)
	if _, err := c.port.Write([]byte(command + string(terminator))); err != nil {
		return fmt.Errorf("sending %q: %w", command, err)
	}
	return nil
}
