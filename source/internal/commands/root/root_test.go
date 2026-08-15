// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package root

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/spf13/cobra"
)

// TestNew tests the New function, including the configuration resolution it
// installs as PersistentPreRunE, with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: the config file is read and the flags override it
//   - FileOnly: no flags are typed, so the config file keeps its say
//   - LoadError: the named config file does not exist
//   - ValidateError: the resolved settings are not usable
func TestNew(t *testing.T) {
	// Build an App whose streams are buffers, so a test never writes to the
	// terminal and nothing reads from it.
	newApp := func() *appcontext.App {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.Stdin = &bytes.Buffer{}
		return app
	}

	// Write a config file into a temporary directory. Every case names one
	// with --config, so the developer's own config file is never read.
	writeConfig := func(t *testing.T, contents string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("writing the config file: %v", err)
		}
		return path
	}

	// Give the root command something runnable to dispatch to, since cobra
	// runs PersistentPreRunE only for a command that has a Run of its own.
	addStub := func(cmd *cobra.Command) {
		cmd.AddCommand(&cobra.Command{
			Use:  "stub",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		})
	}

	// Verify that a flag overrides the value the config file gives
	t.Run("Success", func(t *testing.T) {
		app := newApp()
		path := writeConfig(t, `{"output":"json","pace":"slow"}`)

		cmd := New(app)
		addStub(cmd)
		cmd.SetArgs([]string{
			"stub",
			"--config", path,
			"--verbose",
			"--output", "text",
			"--device", "/dev/tty.usbmodemTEST",
			"--pace", "fast",
			"--wait", "2s",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if app.Config.Path != path {
			t.Errorf("expected config path %q, got %q", path, app.Config.Path)
		}
		if !app.Config.Verbose {
			t.Error("expected verbose to be on")
		}
		if app.Config.Output != appcontext.OutputText {
			t.Errorf("expected the flag to override the file, got output %q", app.Config.Output)
		}
		if app.Config.Device != "/dev/tty.usbmodemTEST" {
			t.Errorf("expected the device from the flag, got %q", app.Config.Device)
		}
		if app.Config.Pace != device.Pace("fast") {
			t.Errorf("expected the flag to override the file, got pace %q", app.Config.Pace)
		}
		if app.Config.Wait != 2*time.Second {
			t.Errorf("expected a wait of 2s, got %v", app.Config.Wait)
		}
	})

	// Verify that settings from the config file survive when no flag is typed
	t.Run("FileOnly", func(t *testing.T) {
		app := newApp()
		path := writeConfig(t, `{"verbose":true,"output":"json","pace":"slow"}`)

		cmd := New(app)
		addStub(cmd)
		cmd.SetArgs([]string{"stub", "--config", path})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if !app.Config.Verbose {
			t.Error("expected verbose from the file to survive")
		}
		if app.Config.Output != appcontext.OutputJSON {
			t.Errorf("expected output json from the file, got %q", app.Config.Output)
		}
		if app.Config.Pace != device.Pace("slow") {
			t.Errorf("expected pace slow from the file, got %q", app.Config.Pace)
		}
		if app.Config.Device != "" {
			t.Errorf("expected no device, got %q", app.Config.Device)
		}
		if app.Config.Wait != 0 {
			t.Errorf("expected no wait, got %v", app.Config.Wait)
		}
	})

	// Verify that naming a config file that is not there fails the command
	t.Run("LoadError", func(t *testing.T) {
		app := newApp()
		missing := filepath.Join(t.TempDir(), "missing.json")

		cmd := New(app)
		addStub(cmd)
		cmd.SetArgs([]string{"stub", "--config", missing})

		if err := cmd.Execute(); err == nil {
			t.Fatal("expected an error for a config file that does not exist")
		}
	})

	// Verify that settings which are not usable fail before any command runs
	t.Run("ValidateError", func(t *testing.T) {
		app := newApp()
		path := writeConfig(t, `{}`)

		cmd := New(app)
		addStub(cmd)
		cmd.SetArgs([]string{"stub", "--config", path, "--output", "yaml"})

		if err := cmd.Execute(); err == nil {
			t.Fatal("expected an error for an unusable output format")
		}
	})
}

// Test_globalFlags_apply tests the apply method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - AllChanged: every flag was typed, so every setting is overwritten
//   - NoneChanged: no flag was typed, so no setting is touched
func Test_globalFlags_apply(t *testing.T) {
	// Build an App whose streams are buffers, so a test never writes to the
	// terminal and nothing reads from it.
	newApp := func() *appcontext.App {
		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}
		app.Stdin = &bytes.Buffer{}
		return app
	}

	// Parse args with the real root command, so the flag names the method asks
	// about are the ones the tool actually registers.
	parse := func(t *testing.T, args []string) *cobra.Command {
		t.Helper()
		cmd := New(newApp())
		if err := cmd.ParseFlags(args); err != nil {
			t.Fatalf("parsing the flags: %v", err)
		}
		return cmd
	}

	// Verify that every flag the user typed lands on the settings
	t.Run("AllChanged", func(t *testing.T) {
		cmd := parse(t, []string{
			"--verbose",
			"--output", "json",
			"--device", "/dev/tty.usbmodemTEST",
			"--pace", "slow",
			"--wait", "3s",
		})

		cfg := appcontext.Defaults()
		g := globalFlags{
			verbose: true,
			output:  "json",
			device:  "/dev/tty.usbmodemTEST",
			pace:    "slow",
			wait:    3 * time.Second,
		}
		g.apply(cmd, cfg)

		if !cfg.Verbose {
			t.Error("expected verbose to be on")
		}
		if cfg.Output != appcontext.OutputJSON {
			t.Errorf("expected output json, got %q", cfg.Output)
		}
		if cfg.Device != "/dev/tty.usbmodemTEST" {
			t.Errorf("expected the device from the flag, got %q", cfg.Device)
		}
		if cfg.Pace != device.Pace("slow") {
			t.Errorf("expected pace slow, got %q", cfg.Pace)
		}
		if cfg.Wait != 3*time.Second {
			t.Errorf("expected a wait of 3s, got %v", cfg.Wait)
		}
	})

	// Verify that flags left alone cannot undo what the config file said
	t.Run("NoneChanged", func(t *testing.T) {
		cmd := parse(t, nil)

		cfg := appcontext.Defaults()
		cfg.Verbose = true
		cfg.Output = appcontext.OutputJSON
		cfg.Device = "/dev/tty.usbmodemSAVED"
		cfg.Pace = device.Pace("medium")
		cfg.Wait = 5 * time.Second

		g := globalFlags{
			verbose: false,
			output:  string(appcontext.OutputText),
			device:  "",
			pace:    string(device.DefaultPace),
			wait:    0,
		}
		g.apply(cmd, cfg)

		if !cfg.Verbose {
			t.Error("expected verbose to be left alone")
		}
		if cfg.Output != appcontext.OutputJSON {
			t.Errorf("expected output to be left alone, got %q", cfg.Output)
		}
		if cfg.Device != "/dev/tty.usbmodemSAVED" {
			t.Errorf("expected the device to be left alone, got %q", cfg.Device)
		}
		if cfg.Pace != device.Pace("medium") {
			t.Errorf("expected the pace to be left alone, got %q", cfg.Pace)
		}
		if cfg.Wait != 5*time.Second {
			t.Errorf("expected the wait to be left alone, got %v", cfg.Wait)
		}
	})
}
