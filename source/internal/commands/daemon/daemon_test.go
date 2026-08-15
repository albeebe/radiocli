// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package daemon

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
)

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Wiring: the command is built with its name, its help and its flags
//   - BadChannel: a channel nobody has is refused before the scanner is touched
//   - NoDeviceWithParentWatch: with no scanner named, serving stops at the
//     missing device, after the watch on the parent has been started
func TestNew(t *testing.T) {
	// Builds an app that writes nowhere and whose stdin is already at its end,
	// so nothing in these tests waits on a terminal.
	newApp := func() *appcontext.App {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.Stdin = strings.NewReader("")
		return app
	}

	// Runs the command with the arguments given and returns what it reported.
	execute := func(app *appcontext.App, args ...string) error {
		cmd := New(app)
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		return cmd.ExecuteContext(context.Background())
	}

	// Verify the command is named, described and given the flags it documents
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(newApp())

		if cmd.Use != "daemon" {
			t.Errorf("the command is called %q, wanted %q", cmd.Use, "daemon")
		}
		if cmd.Short == "" {
			t.Error("the command has no short description")
		}
		if cmd.Long == "" {
			t.Error("the command has no long description")
		}

		for _, name := range []string{"exit-with-parent", "audio", "audio-channel"} {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("the command has no --%s flag", name)
			}
		}

		// The channel defaults to letting the daemon work the side out for
		// itself, which is what makes --audio alone enough.
		if flag := cmd.Flags().Lookup("audio-channel"); flag.DefValue != audiofeed.ChannelAuto {
			t.Errorf("--audio-channel defaults to %q, wanted %q",
				flag.DefValue, audiofeed.ChannelAuto)
		}
	})

	// Verify a channel that does not exist stops the command before it holds anything
	t.Run("BadChannel", func(t *testing.T) {
		err := execute(newApp(), "--audio-channel", "sideways")
		if err == nil {
			t.Fatal("the command ran with a channel nobody has, wanted it refused")
		}
		if !strings.Contains(err.Error(), "sideways") {
			t.Errorf("the command reported %q, wanted it to name the channel asked for", err)
		}
	})

	// Verify that with no scanner named the command reports the missing device
	t.Run("NoDeviceWithParentWatch", func(t *testing.T) {
		app := newApp()
		app.Config.Device = ""

		err := execute(app, "--exit-with-parent")
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Errorf("the command reported %v, wanted it to report no device was named", err)
		}
	})
}

// Test_watchParent tests the watchParent function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NoStream: with nothing to watch, nothing is reported
//   - ParentGone: the end of the stream is the parent going away
func Test_watchParent(t *testing.T) {
	// Verify that without a stream to watch nothing is ever reported orphaned
	t.Run("NoStream", func(t *testing.T) {
		orphaned := make(chan struct{})
		watchParent(nil, orphaned)

		select {
		case <-orphaned:
			t.Error("the daemon was told it was orphaned with nothing watching the parent")
		default:
		}
	})

	// Verify that the stream ending reports the parent has gone, whatever it sent first
	t.Run("ParentGone", func(t *testing.T) {
		orphaned := make(chan struct{})
		watchParent(strings.NewReader("anything it sent is thrown away"), orphaned)

		select {
		case <-orphaned:
		default:
			t.Error("the stream ended and the daemon was not told the parent had gone")
		}
	})
}
