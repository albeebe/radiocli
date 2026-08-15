// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package beep

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// isolate points the remembered settings at a directory of this test's own, so
// a run never reads or writes the file of the person running it.
func isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LocalAppData", dir)

	path, err := memoryPath()
	if err != nil {
		t.Fatalf("locating the remembered settings: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("the temporary directory already holds %s", path)
	}
	return path
}

// quiet returns an App that writes its warnings nowhere.
func quiet(t *testing.T) *appcontext.App {
	t.Helper()
	return &appcontext.App{
		Log:    slog.New(slog.DiscardHandler),
		Stdout: os.NewFile(0, os.DevNull),
		Stderr: os.NewFile(0, os.DevNull),
	}
}

var testScanner = device.Info{Port: "/dev/tty.fake", Model: "SDS150", Serial: "0001"}

// TestARememberedSettingSurvivesAWriteAndARead is the whole of what the toggle
// rests on: what it wrote down when it silenced the keypad is what it finds
// when it is asked to put it back.
func TestARememberedSettingSurvivesAWriteAndARead(t *testing.T) {
	isolate(t)
	app := quiet(t)

	nine, _ := lookup("9")
	remember(app, testScanner, nine, time.Now())

	got, ok := recall(app, testScanner)
	if !ok {
		t.Fatal("nothing was remembered")
	}
	if got.value != "9" {
		t.Errorf("the remembered setting is %q, wanted \"9\"", got.value)
	}
}

// TestNothingIsRememberedForAScannerNobodyToggled covers the first run, which
// is the case that decides a toggle leaves the keypad silent.
func TestNothingIsRememberedForAScannerNobodyToggled(t *testing.T) {
	isolate(t)
	app := quiet(t)

	if _, ok := recall(app, testScanner); ok {
		t.Error("something was remembered before anything was written down")
	}
}

// TestScannersRememberSeparately covers two radios on one computer. The setting
// belongs to the scanner, so putting one back must not put the other's level on
// it.
func TestScannersRememberSeparately(t *testing.T) {
	isolate(t)
	app := quiet(t)

	other := device.Info{Port: "/dev/tty.other", Model: "SDS150", Serial: "0002"}

	three, _ := lookup("3")
	twelve, _ := lookup("12")
	remember(app, testScanner, three, time.Now())
	remember(app, other, twelve, time.Now())

	first, ok := recall(app, testScanner)
	if !ok || first.value != "3" {
		t.Errorf("the first scanner remembers %q, wanted \"3\"", first.value)
	}
	second, ok := recall(app, other)
	if !ok || second.value != "12" {
		t.Errorf("the second scanner remembers %q, wanted \"12\"", second.value)
	}
}

// TestARememberedOffIsNoAnswer covers a file edited by hand into saying the
// keypad should be put back to silence, which would make the button do nothing
// for ever. A toggle stores what it is replacing and never stores off itself.
func TestARememberedOffIsNoAnswer(t *testing.T) {
	isolate(t)
	app := quiet(t)

	silent, _ := lookup(off)
	remember(app, testScanner, silent, time.Now())

	if _, ok := recall(app, testScanner); ok {
		t.Error("a remembered \"off\" was offered as something to go back to")
	}
}

// TestTheMemorySurvivesRubbish covers a file that is damaged, written by a
// later version, or not JSON at all. None of them is worth failing a command
// over: the answer is that nothing is remembered.
func TestTheMemorySurvivesRubbish(t *testing.T) {
	for _, content := range []string{
		"not json at all",
		"",
		`{"version": 99, "scanners": {"port:/dev/tty.fake": {"level": "9"}}}`,
		`{"version": 1, "scanners": null}`,
	} {
		t.Run(content, func(t *testing.T) {
			path := isolate(t)
			app := quiet(t)

			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			if _, ok := recall(app, testScanner); ok {
				t.Error("a setting was recalled from a file that should have been ignored")
			}

			// And the damaged file must not stop a later toggle writing a good
			// one, or one bad write would silence the button for ever.
			nine, _ := lookup("9")
			remember(app, testScanner, nine, time.Now())
			got, ok := recall(app, testScanner)
			if !ok || got.value != "9" {
				t.Error("a setting written after the bad file was not remembered")
			}
		})
	}
}

// TestRememberingAgainReplacesWhatWasThere covers pressing the button twice
// with a different level set in between, which must not leave the first one to
// be put back.
func TestRememberingAgainReplacesWhatWasThere(t *testing.T) {
	isolate(t)
	app := quiet(t)

	nine, _ := lookup("9")
	four, _ := lookup("4")
	remember(app, testScanner, nine, time.Now())
	remember(app, testScanner, four, time.Now())

	got, ok := recall(app, testScanner)
	if !ok || got.value != "4" {
		t.Errorf("the setting remembered is %q, wanted \"4\"", got.value)
	}
}

// Test_scannerKey covers how one scanner is told from another in the file.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - Keys: a serial number is preferred, and the port is the fallback
func Test_scannerKey(t *testing.T) {
	// Verify the serial number identifies a scanner, and the port stands in
	t.Run("Keys", func(t *testing.T) {
		withSerial := device.Info{Port: "/dev/tty.fake", Serial: "0001"}
		if got, want := scannerKey(withSerial), "serial:0001"; got != want {
			t.Errorf("scannerKey is %q, wanted %q", got, want)
		}

		withoutSerial := device.Info{Port: "/dev/tty.fake"}
		if got, want := scannerKey(withoutSerial), "port:/dev/tty.fake"; got != want {
			t.Errorf("scannerKey is %q, wanted %q", got, want)
		}
	})
}

// Test_memoryPath covers locating the file the settings are kept in.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Success: the path is under the user's cache directory
//   - NoCacheDir: a computer with no cache directory is reported
func Test_memoryPath(t *testing.T) {
	// Verify the file is named under the user's own cache directory
	t.Run("Success", func(t *testing.T) {
		isolate(t)

		got, err := memoryPath()
		if err != nil {
			t.Fatalf("memoryPath: %v", err)
		}
		if !strings.HasSuffix(got, filepath.Join("radiocli", "keybeep.json")) {
			t.Errorf("the path is %q, wanted it to end in radiocli/keybeep.json", got)
		}
	})

	// Verify a computer with nowhere to put a cache is reported
	t.Run("NoCacheDir", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("LocalAppData", "")

		_, err := memoryPath()
		if err == nil || !strings.Contains(err.Error(), "locating user cache directory") {
			t.Errorf("memoryPath reported %v, wanted the missing cache directory", err)
		}
	})
}

// Test_loadMemory covers reading the file, including everything wrong with it.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Missing: a first run reads as an empty memory rather than a failure
//   - NoCacheDir: a computer with no cache directory is reported
//   - Unreadable: a file that cannot be read at all is reported
func Test_loadMemory(t *testing.T) {
	// Verify the normal first run hands back an empty memory
	t.Run("Missing", func(t *testing.T) {
		isolate(t)

		got, err := loadMemory()
		if err != nil {
			t.Fatalf("loadMemory: %v", err)
		}
		if got.Version != memoryVersion || len(got.Scanners) != 0 {
			t.Errorf("the memory is %+v, wanted an empty one", got)
		}
	})

	// Verify a computer with nowhere to put a cache is reported
	t.Run("NoCacheDir", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("LocalAppData", "")

		if _, err := loadMemory(); err == nil {
			t.Error("loadMemory reported nothing, wanted the missing cache directory")
		}
	})

	// Verify a file that cannot be read at all is reported rather than ignored
	t.Run("Unreadable", func(t *testing.T) {
		path := isolate(t)

		// A directory where the file belongs cannot be read as a file, which
		// is a failure worth reporting rather than an empty memory.
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := loadMemory()
		if err == nil || !strings.Contains(err.Error(), "reading ") {
			t.Errorf("loadMemory reported %v, wanted the unreadable file", err)
		}
	})
}

// Test_memory_save covers writing the file back.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Success: the file is written where the settings belong
//   - NoCacheDir: a computer with no cache directory is reported
//   - CannotCreate: a directory that cannot be made is reported
//   - CannotWrite: a file that cannot be written is reported
//   - CannotEncode: an encoder that refuses the settings is reported
func Test_memory_save(t *testing.T) {
	// Verify the file is written and can be read back
	t.Run("Success", func(t *testing.T) {
		path := isolate(t)

		m := &memory{Scanners: map[string]remembered{"port:x": {Level: "9"}}}
		if err := m.save(); err != nil {
			t.Fatalf("save: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the file was not written: %v", err)
		}
	})

	// Verify a computer with nowhere to put a cache is reported
	t.Run("NoCacheDir", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("LocalAppData", "")

		m := &memory{}
		if err := m.save(); err == nil {
			t.Error("save reported nothing, wanted the missing cache directory")
		}
	})

	// Verify a directory that cannot be created is reported
	t.Run("CannotCreate", func(t *testing.T) {
		path := isolate(t)

		// A file where the directory belongs stops the directory being made.
		if err := os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Dir(path), []byte("in the way"), 0o600); err != nil {
			t.Fatal(err)
		}

		m := &memory{}
		err := m.save()
		if err == nil || !strings.Contains(err.Error(), "creating ") {
			t.Errorf("save reported %v, wanted the directory it could not create", err)
		}
	})

	// Verify a file that cannot be written is reported
	t.Run("CannotWrite", func(t *testing.T) {
		path := isolate(t)

		// A directory where the file belongs cannot be written as a file.
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}

		m := &memory{}
		err := m.save()
		if err == nil || !strings.Contains(err.Error(), "writing ") {
			t.Errorf("save reported %v, wanted the file it could not write", err)
		}
	})

	// Verify an encoder that refuses the settings is reported rather than a
	// half written file being left behind
	t.Run("CannotEncode", func(t *testing.T) {
		isolate(t)

		marshalJSON = func(any, string, string) ([]byte, error) {
			return nil, errors.New("no")
		}
		t.Cleanup(func() { marshalJSON = json.MarshalIndent })

		m := &memory{}
		err := m.save()
		if err == nil || !strings.Contains(err.Error(), "encoding ") {
			t.Errorf("save reported %v, wanted the settings it could not encode", err)
		}
	})
}

// Test_memory_store covers writing one scanner's setting into the memory.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Empty: a memory holding nothing yet is given somewhere to put it
//   - Replaces: storing again replaces what was there
func Test_memory_store(t *testing.T) {
	// Verify a memory with no map yet is given one rather than panicking
	t.Run("Empty", func(t *testing.T) {
		m := &memory{}

		nine, _ := lookup("9")
		m.store("port:x", device.Info{Model: "SDS150", Port: "/dev/tty.fake"}, nine, time.Now())

		got, ok := m.Scanners["port:x"]
		if !ok || got.Level != "9" || got.Model != "SDS150" {
			t.Errorf("the memory holds %+v, wanted level 9 for the fake scanner", got)
		}
	})

	// Verify storing a second setting replaces the first
	t.Run("Replaces", func(t *testing.T) {
		m := &memory{Scanners: map[string]remembered{}}

		nine, _ := lookup("9")
		four, _ := lookup("4")
		m.store("port:x", device.Info{}, nine, time.Now())
		m.store("port:x", device.Info{}, four, time.Now())

		if got := m.Scanners["port:x"].Level; got != "4" {
			t.Errorf("the memory holds %q, wanted the setting stored last", got)
		}
	})
}

// Test_remember covers writing a setting down when the file will not take it.
//
// Coverage: 100% (1 test case covering the branch the other tests leave)
//
// Test cases:
//   - CannotWrite: a memory that cannot be written warns rather than fails
func Test_remember(t *testing.T) {
	// Verify a memory that cannot be written is a warning, not a failed command
	t.Run("CannotWrite", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("LocalAppData", "")

		app := appcontext.New()
		notes := &bytes.Buffer{}
		app.Stdout = &bytes.Buffer{}
		app.Stderr = notes

		nine, _ := lookup("9")
		remember(app, testScanner, nine, time.Now())

		if !strings.Contains(notes.String(), "Could not write down") {
			t.Errorf("remember wrote %q, wanted the warning about not writing it down", notes.String())
		}
	})
}

// Test_recall covers reading a setting back when the file cannot be read.
//
// Coverage: 100% (1 test case covering the branch the other tests leave)
//
// Test cases:
//   - CannotRead: a memory that cannot be read remembers nothing
func Test_recall(t *testing.T) {
	// Verify a memory that cannot be read is treated as remembering nothing
	t.Run("CannotRead", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("LocalAppData", "")

		app := appcontext.New()
		app.Stdout = &bytes.Buffer{}
		app.Stderr = &bytes.Buffer{}

		if _, ok := recall(app, testScanner); ok {
			t.Error("a setting was recalled from a memory that could not be read")
		}
	})
}
