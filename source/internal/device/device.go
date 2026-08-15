// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package device talks to an SDS150 scanner over its USB serial port.
//
// The scanner speaks a line protocol: the host writes a command terminated by
// a carriage return, and the scanner answers with a comma separated line, also
// terminated by a carriage return. The first field of the answer echoes the
// command, so a reply that echoes the wrong command means the connection is
// out of sync.
//
// Commands hold a *Scanner, which talks through the Conn interface, so tests
// can supply a fake Conn without a scanner attached.
package device

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/portlock"
	"go.bug.st/serial"
)

// BusyError reports that discovery found nothing except ports another
// invocation was holding, and names them.
//
// Parameters:
//   - busy: the ports discovery skipped because they were already claimed
//
// Returns:
//   - error naming those ports and telling the user to run the command again
//
// Errors:
//   - ErrScannersBusy: always, since that is the condition being reported
func BusyError(busy []string) error {
	return fmt.Errorf("%w: %s\n\nWait for it to finish and run this command again",
		ErrScannersBusy, strings.Join(busy, ", "))
}

// Discover returns every scanner currently attached, along with the ports it
// could not look at because another invocation of the tool was using them.
//
// It cannot tell a scanner from any other USB serial device by enumeration
// alone, so it asks each candidate port to identify itself and keeps the ones
// that answer. Ports that stay silent, are held by something else, or reply
// with something else are skipped and logged at debug level.
//
// The busy ports are returned rather than merely skipped because the two
// reasons for an empty list need opposite advice. Nothing attached means
// something about the cable or the scanner has to change. A busy port means
// the scanner is attached and working, and the only thing in the way is this
// tool itself, which will be gone in a moment.
//
// Parameters:
//   - ctx: context for cancellation and timeouts, bounded per port by
//     probeTimeout
//   - log: logger that records which ports were skipped and why
//
// Returns:
//   - found: every scanner that answered, empty rather than nil when none did
//   - busy: the ports another invocation of the tool was holding
//   - err: error if the serial ports cannot be listed, or if ctx ended while
//     the candidates were being probed
func Discover(ctx context.Context, log *slog.Logger) (found []Info, busy []string, err error) {
	ports, err := listPorts()
	if err != nil {
		return nil, nil, fmt.Errorf("listing serial ports: %w", err)
	}

	found = make([]Info, 0, 1)
	for _, p := range ports {
		if !p.IsUSB {
			continue
		}

		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		info, err := probe(probeCtx, p.Name, p.SerialNumber)
		cancel()
		if errors.Is(err, portlock.ErrBusy) {
			// Kept apart from the rest because it says nothing about the port:
			// it is very likely the scanner, and the only reason it is not in
			// the list is that this tool is already talking to it.
			log.Debug("port is in use by another radiocli", "port", p.Name)
			busy = append(busy, p.Name)
			continue
		}
		if err != nil {
			log.Debug("port is not a scanner", "port", p.Name, "reason", err)
			continue
		}

		log.Debug("found scanner", "port", info.Port, "model", info.Model)
		found = append(found, info)
	}

	return found, busy, ctx.Err()
}

// Open connects to the scanner at the given serial port and confirms that
// something on the other end actually answers as a scanner.
//
// The port is claimed before it is opened, and the claim is held until the
// connection is closed, so no other invocation of the tool can talk to the
// scanner in the middle of this one. wait is how long to wait for a claim
// somebody else already holds; zero gives up at once and reports who has it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - port: the serial device path to open, such as /dev/tty.usbmodem14201
//   - wait: how long to wait for a claim another invocation holds
//   - log: logger the connection keeps for its own debug output
//
// Returns:
//   - a Scanner holding the port and the claim until it is closed
//   - error if the claim cannot be taken, the port cannot be opened or
//     configured, or nothing on the other end answers as a scanner
func Open(ctx context.Context, port string, wait time.Duration, log *slog.Logger) (*Scanner, error) {
	lock, err := acquirePort(port, wait)
	if err != nil {
		return nil, err
	}

	p, err := openPort(port, &serial.Mode{BaudRate: baudRate})
	if err != nil {
		lock.Release()
		return nil, fmt.Errorf("opening %s: %w", port, err)
	}
	if err := p.SetReadTimeout(readPoll); err != nil {
		p.Close()
		lock.Release()
		return nil, fmt.Errorf("configuring %s: %w", port, err)
	}

	c := &conn{port: p, log: log, info: Info{Port: port}, lock: lock}

	model, err := c.Execute(ctx, "MDL")
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("identifying scanner on %s: %w", port, err)
	}
	c.info.Model = model

	log.Debug("connected to scanner", "port", port, "model", model)
	return New(c), nil
}

// String renders the scanner for a human, in the form used by the picker.
//
// Returns:
//   - the model and port, carrying the USB serial number when there is one
func (i Info) String() string {
	if i.Serial == "" {
		return fmt.Sprintf("%s at %s", i.Model, i.Port)
	}
	return fmt.Sprintf("%s (serial %s) at %s", i.Model, i.Serial, i.Port)
}

// probe opens a candidate port just long enough to ask what it is.
//
// It claims the port first, and gives up immediately rather than waiting. A
// port somebody else is using is a port this must not send anything to: asking
// an unknown device to identify itself is harmless, but doing it in the middle
// of another invocation's menu walk is not, and discovery is never so
// important that it is worth queueing for.
//
// Parameters:
//   - ctx: context bounding how long the port has to identify itself
//   - port: the serial device path to open
//   - usbSerial: the USB serial number enumeration reported, which is empty
//     when the port does not carry one
//
// Returns:
//   - Info describing the scanner that answered on the port
//   - error if the port is already claimed, cannot be opened or configured, or
//     does not answer as a scanner
func probe(ctx context.Context, port, usbSerial string) (Info, error) {
	lock, err := acquirePort(port, 0)
	if err != nil {
		return Info{}, err
	}
	defer lock.Release()

	p, err := openPort(port, &serial.Mode{BaudRate: baudRate})
	if err != nil {
		return Info{}, fmt.Errorf("opening port: %w", err)
	}
	defer p.Close()

	if err := p.SetReadTimeout(readPoll); err != nil {
		return Info{}, fmt.Errorf("configuring port: %w", err)
	}

	c := &conn{port: p, log: slog.New(slog.DiscardHandler)}
	model, err := c.Execute(ctx, "MDL")
	if err != nil {
		return Info{}, err
	}

	return Info{Port: port, Model: model, Serial: usbSerial}, nil
}
