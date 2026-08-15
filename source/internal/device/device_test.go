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

	"github.com/albeebe/radiocli/internal/portlock"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// fakeDiscovery substitutes the three primitives that reach the hardware, and
// restores them when the test ends.
//
// The claim is handed back as a nil Lock, which Release treats as nothing to
// release, so no lock file is ever created.
//
// Parameters:
//   - t: the test to restore the primitives for
//   - ports: what listing the serial ports reports
//   - portsErr: the error listing the serial ports reports instead
//   - open: what opening a port answers, given the port name
//   - acquire: the error claiming a port reports, given the port name
func fakeDiscovery(t *testing.T, ports []*enumerator.PortDetails, portsErr error,
	open func(port string) (serial.Port, error), acquire func(port string) error) {
	t.Helper()

	oldList, oldOpen, oldAcquire := listPorts, openPort, acquirePort
	t.Cleanup(func() {
		listPorts, openPort, acquirePort = oldList, oldOpen, oldAcquire
	})

	listPorts = func(...func(vid, pid string) bool) ([]*enumerator.PortDetails, error) {
		return ports, portsErr
	}
	openPort = func(port string, mode *serial.Mode) (serial.Port, error) {
		if open == nil {
			return &fakePort{}, nil
		}
		return open(port)
	}
	acquirePort = func(port string, wait time.Duration) (*portlock.Lock, error) {
		if acquire == nil {
			return nil, nil
		}
		return nil, acquire(port)
	}
}

// answering returns a port that identifies itself as a scanner, which is what a
// probe needs to accept it.
//
// Parameters:
//   - model: the model the port reports
//
// Returns:
//   - a fake port whose first reply answers MDL with model
func answering(model string) *fakePort {
	return &fakePort{chunks: []string{"MDL," + model + "\r"}}
}

// TestBusyError tests the BusyError function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the busy ports are named and the error wraps ErrScannersBusy
func TestBusyError(t *testing.T) {
	// Verify that every busy port is named and the sentinel is wrapped.
	t.Run("Success", func(t *testing.T) {
		err := BusyError([]string{"/dev/one", "/dev/two"})
		if !errors.Is(err, ErrScannersBusy) {
			t.Fatalf("got %v, want ErrScannersBusy", err)
		}
		if !strings.Contains(err.Error(), "/dev/one, /dev/two") {
			t.Errorf("error %q does not name both ports", err)
		}
	})
}

// TestDiscover tests the Discover function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - ListError: a failed listing is reported
//   - Success: a USB port that answers is reported as a scanner
//   - SkipsNonUSB: a port that is not USB is never probed
//   - Busy: a port another invocation holds is reported as busy, not missing
//   - NotAScanner: a port that will not identify itself is skipped
//   - ContextEnded: a context that ended during the walk is reported
func TestDiscover(t *testing.T) {
	usb := func(name string) *enumerator.PortDetails {
		return &enumerator.PortDetails{Name: name, IsUSB: true, SerialNumber: "SN1"}
	}

	// Verify that a listing that fails is reported rather than treated as empty.
	t.Run("ListError", func(t *testing.T) {
		fakeDiscovery(t, nil, errors.New("the port is gone"), nil, nil)

		_, _, err := Discover(context.Background(), discardLog())
		if err == nil || !strings.Contains(err.Error(), "listing serial ports") {
			t.Fatalf("got %v, want a listing complaint", err)
		}
	})

	// Verify that a USB port answering MDL is reported as a scanner.
	t.Run("Success", func(t *testing.T) {
		fakeDiscovery(t, []*enumerator.PortDetails{usb("/dev/one")}, nil,
			func(string) (serial.Port, error) { return answering("SDS150"), nil }, nil)

		found, busy, err := Discover(context.Background(), discardLog())
		if err != nil {
			t.Fatalf("discovering: %v", err)
		}
		if len(busy) != 0 {
			t.Errorf("got %q busy, want none", busy)
		}
		if len(found) != 1 || found[0].Model != "SDS150" || found[0].Serial != "SN1" {
			t.Fatalf("got %+v, want one SDS150 carrying its USB serial", found)
		}
	})

	// Verify that a port that is not USB is skipped without being opened.
	t.Run("SkipsNonUSB", func(t *testing.T) {
		opened := false
		fakeDiscovery(t, []*enumerator.PortDetails{{Name: "/dev/serial", IsUSB: false}}, nil,
			func(string) (serial.Port, error) { opened = true; return answering("SDS150"), nil }, nil)

		found, _, err := Discover(context.Background(), discardLog())
		if err != nil {
			t.Fatalf("discovering: %v", err)
		}
		if opened {
			t.Error("a port that is not USB was opened")
		}
		if len(found) != 0 {
			t.Errorf("got %+v, want nothing found", found)
		}
	})

	// Verify that a claimed port is reported as busy rather than as missing.
	t.Run("Busy", func(t *testing.T) {
		fakeDiscovery(t, []*enumerator.PortDetails{usb("/dev/one")}, nil, nil,
			func(string) error { return portlock.ErrBusy })

		found, busy, err := Discover(context.Background(), discardLog())
		if err != nil {
			t.Fatalf("discovering: %v", err)
		}
		if len(found) != 0 {
			t.Errorf("got %+v, want nothing found", found)
		}
		if len(busy) != 1 || busy[0] != "/dev/one" {
			t.Fatalf("got %q busy, want /dev/one", busy)
		}
	})

	// Verify that a port that will not open is skipped quietly.
	t.Run("NotAScanner", func(t *testing.T) {
		fakeDiscovery(t, []*enumerator.PortDetails{usb("/dev/one")}, nil,
			func(string) (serial.Port, error) { return nil, errors.New("the port is gone") }, nil)

		found, busy, err := Discover(context.Background(), discardLog())
		if err != nil {
			t.Fatalf("discovering: %v", err)
		}
		if len(found) != 0 || len(busy) != 0 {
			t.Errorf("got %+v found and %q busy, want neither", found, busy)
		}
	})

	// Verify that a context that ended during the walk is reported.
	t.Run("ContextEnded", func(t *testing.T) {
		fakeDiscovery(t, nil, nil, nil, nil)

		_, _, err := Discover(deadCtx(t), discardLog())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want the cancellation reported", err)
		}
	})
}

// TestOpen tests the Open function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Success: the scanner is identified and returned holding its port
//   - AcquireError: a port another invocation holds is reported
//   - OpenError: a port that will not open is reported, naming it
//   - ConfigureError: a port that will not take a read timeout is reported
//   - IdentifyError: a port that will not identify itself is reported
func TestOpen(t *testing.T) {
	// Verify that a scanner that identifies itself comes back ready to use.
	t.Run("Success", func(t *testing.T) {
		fakeDiscovery(t, nil, nil,
			func(string) (serial.Port, error) { return answering("SDS150"), nil }, nil)

		s, err := Open(context.Background(), "/dev/one", 0, discardLog())
		if err != nil {
			t.Fatalf("opening: %v", err)
		}
		if got := s.Info(); got.Model != "SDS150" || got.Port != "/dev/one" {
			t.Errorf("got %+v, want the SDS150 on /dev/one", got)
		}
		if err := s.Close(); err != nil {
			t.Errorf("closing: %v", err)
		}
	})

	// Verify that a claim somebody else holds is reported as it stands.
	t.Run("AcquireError", func(t *testing.T) {
		fakeDiscovery(t, nil, nil, nil, func(string) error { return portlock.ErrBusy })

		if _, err := Open(context.Background(), "/dev/one", 0, discardLog()); !errors.Is(err, portlock.ErrBusy) {
			t.Fatalf("got %v, want ErrBusy", err)
		}
	})

	// Verify that a port that will not open names itself in the error.
	t.Run("OpenError", func(t *testing.T) {
		fakeDiscovery(t, nil, nil,
			func(string) (serial.Port, error) { return nil, errors.New("the port is gone") }, nil)

		_, err := Open(context.Background(), "/dev/one", 0, discardLog())
		if err == nil || !strings.Contains(err.Error(), "opening /dev/one") {
			t.Fatalf("got %v, want the port named", err)
		}
	})

	// Verify that a port that will not take a read timeout is reported.
	t.Run("ConfigureError", func(t *testing.T) {
		fakeDiscovery(t, nil, nil, func(string) (serial.Port, error) {
			return &fakePort{timeoutErr: errors.New("the port is gone")}, nil
		}, nil)

		_, err := Open(context.Background(), "/dev/one", 0, discardLog())
		if err == nil || !strings.Contains(err.Error(), "configuring /dev/one") {
			t.Fatalf("got %v, want a configuration complaint", err)
		}
	})

	// Verify that a port that answers nothing is reported as unidentified.
	t.Run("IdentifyError", func(t *testing.T) {
		fakeDiscovery(t, nil, nil,
			func(string) (serial.Port, error) { return &fakePort{}, nil }, nil)

		_, err := Open(deadCtx(t), "/dev/one", 0, discardLog())
		if err == nil || !strings.Contains(err.Error(), "identifying scanner on /dev/one") {
			t.Fatalf("got %v, want an identification complaint", err)
		}
	})
}

// TestInfoString tests the Info String method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - WithoutSerial: a scanner reporting no USB serial number is named plainly
//   - WithSerial: a scanner reporting one carries it, since that is what tells
//     two identical scanners apart
func TestInfoString(t *testing.T) {
	// Verify that a scanner with no serial number reads as model at port.
	t.Run("WithoutSerial", func(t *testing.T) {
		got := Info{Model: "SDS150", Port: "/dev/one"}.String()
		if got != "SDS150 at /dev/one" {
			t.Errorf("got %q, want SDS150 at /dev/one", got)
		}
	})

	// Verify that a serial number is carried when there is one.
	t.Run("WithSerial", func(t *testing.T) {
		got := Info{Model: "SDS150", Port: "/dev/one", Serial: "SN1"}.String()
		if got != "SDS150 (serial SN1) at /dev/one" {
			t.Errorf("got %q, want the serial number carried", got)
		}
	})
}

// Test_probe tests the probe function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Success: a port that answers MDL is described, USB serial included
//   - AcquireError: a claimed port is reported without being opened
//   - OpenError: a port that will not open is reported
//   - ConfigureError: a port that will not take a read timeout is reported
//   - IdentifyError: a port that answers nothing is reported
func Test_probe(t *testing.T) {
	// Verify that an answering port is described by what it said.
	t.Run("Success", func(t *testing.T) {
		fakeDiscovery(t, nil, nil,
			func(string) (serial.Port, error) { return answering("SDS150"), nil }, nil)

		info, err := probe(context.Background(), "/dev/one", "SN1")
		if err != nil {
			t.Fatalf("probing: %v", err)
		}
		if info != (Info{Port: "/dev/one", Model: "SDS150", Serial: "SN1"}) {
			t.Errorf("got %+v, want the port, model and serial", info)
		}
	})

	// Verify that a claimed port is never opened.
	t.Run("AcquireError", func(t *testing.T) {
		opened := false
		fakeDiscovery(t, nil, nil,
			func(string) (serial.Port, error) { opened = true; return &fakePort{}, nil },
			func(string) error { return portlock.ErrBusy })

		if _, err := probe(context.Background(), "/dev/one", ""); !errors.Is(err, portlock.ErrBusy) {
			t.Fatalf("got %v, want ErrBusy", err)
		}
		if opened {
			t.Error("a claimed port was opened anyway")
		}
	})

	// Verify that a port that will not open is reported.
	t.Run("OpenError", func(t *testing.T) {
		fakeDiscovery(t, nil, nil,
			func(string) (serial.Port, error) { return nil, errors.New("the port is gone") }, nil)

		_, err := probe(context.Background(), "/dev/one", "")
		if err == nil || !strings.Contains(err.Error(), "opening port") {
			t.Fatalf("got %v, want an opening complaint", err)
		}
	})

	// Verify that a port that will not take a read timeout is reported.
	t.Run("ConfigureError", func(t *testing.T) {
		fakeDiscovery(t, nil, nil, func(string) (serial.Port, error) {
			return &fakePort{timeoutErr: errors.New("the port is gone")}, nil
		}, nil)

		_, err := probe(context.Background(), "/dev/one", "")
		if err == nil || !strings.Contains(err.Error(), "configuring port") {
			t.Fatalf("got %v, want a configuration complaint", err)
		}
	})

	// Verify that a silent port is reported rather than accepted.
	t.Run("IdentifyError", func(t *testing.T) {
		fakeDiscovery(t, nil, nil,
			func(string) (serial.Port, error) { return &fakePort{}, nil }, nil)

		if _, err := probe(deadCtx(t), "/dev/one", ""); err == nil {
			t.Fatal("a silent port identified itself as a scanner")
		}
	})
}
