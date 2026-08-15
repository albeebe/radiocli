// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package colors

import (
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

// isolate points the cache at a directory of this test's own, so a run never
// reads or writes the cache of the person running it.
func isolate(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LocalAppData", dir)

	path, err := cachePath()
	if err != nil {
		t.Fatalf("locating the cache: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("the temporary cache directory already holds %s", path)
	}
}

// quiet returns an App that writes its warnings nowhere, which is all the cache
// helpers use it for.
func quiet() *appcontext.App {
	return &appcontext.App{Log: slog.New(slog.DiscardHandler)}
}

var testScanner = device.Info{Port: "/dev/tty.fake", Model: "SDS150", Serial: "0001"}

// TestCacheRoundTrip covers a reading surviving a write and a read, which is
// the whole of what --cache rests on.
func TestCacheRoundTrip(t *testing.T) {
	isolate(t)

	when := time.Now().Add(-time.Hour).Round(time.Second)
	areas := []area{
		{Name: "System_name", Text: "Yellow", TextHex: "#FFFF00", Background: "Black", BackgroundHex: "#000000",
			Line: 2, Column: 0, Length: 16, Height: 1},
	}
	remember(quiet(), testScanner, "weather", areas, when)

	c, err := loadCache()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	got, ok := c.lookup(scannerKey(testScanner), "weather")
	if !ok {
		t.Fatal("the reading that was just stored is not there")
	}
	if !got.Read.Equal(when) {
		t.Errorf("read at %s, want %s", got.Read, when)
	}

	back := got.areas()
	if len(back) != 1 {
		t.Fatalf("got %d areas, want 1", len(back))
	}
	if back[0].Name != "System_name" || back[0].Text != "Yellow" || back[0].BackgroundHex != "#000000" {
		t.Errorf("the colors came back as %+v", back[0])
	}

	// Positions are not stored, because place fills them in from the built-in
	// map every run. A cached one could only ever be a copy that goes stale.
	if back[0].Length != 0 || back[0].Line != 0 {
		t.Errorf("the cache gave back a position: %+v", back[0])
	}
}

// TestCacheMissesAreMisses covers the three ways --cache has nothing to say,
// each of which has to be a miss rather than an empty layout: a miss sends the
// caller to read the scanner, and an empty layout would report a screen with no
// colors on it.
func TestCacheMissesAreMisses(t *testing.T) {
	isolate(t)

	c, err := loadCache()
	if err != nil {
		t.Fatalf("loading an absent cache: %v", err)
	}
	if _, ok := c.lookup(scannerKey(testScanner), "weather"); ok {
		t.Error("an empty cache reported a hit")
	}

	remember(quiet(), testScanner, "weather", []area{{Name: "System_name"}}, time.Now())
	c, err = loadCache()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if _, ok := c.lookup(scannerKey(testScanner), "search"); ok {
		t.Error("a layout that was never read reported a hit")
	}

	// A second scanner keeps its own colors, since they are its own settings.
	other := device.Info{Port: "/dev/tty.other", Model: "SDS150", Serial: "0002"}
	if _, ok := c.lookup(scannerKey(other), "weather"); ok {
		t.Error("one scanner answered for another")
	}
}

// TestCacheSurvivesRubbish covers a cache file that cannot be used. Nothing in
// it is not also on the scanner, so every kind of damage has to read as a miss
// rather than fail a command.
func TestCacheSurvivesRubbish(t *testing.T) {
	isolate(t)

	path, err := cachePath()
	if err != nil {
		t.Fatalf("locating the cache: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating the cache directory: %v", err)
	}

	for _, content := range []string{
		"not json at all",
		`{"version":0,"scanners":{}}`,
		`{"version":99,"scanners":{"serial:0001":{"layouts":{"weather":{"areas":[{"area":"x"}]}}}}}`,
		`{"version":1}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("writing the cache: %v", err)
		}

		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading %q: %v", content, err)
		}
		if _, ok := c.lookup(scannerKey(testScanner), "weather"); ok {
			t.Errorf("%q reported a hit", content)
		}

		// And it has to be writable again, so one bad file does not stop the
		// tool caching anything ever after.
		remember(quiet(), testScanner, "weather", []area{{Name: "System_name", Text: "Red"}}, time.Now())
		c, err = loadCache()
		if err != nil {
			t.Fatalf("loading after %q: %v", content, err)
		}
		if _, ok := c.lookup(scannerKey(testScanner), "weather"); !ok {
			t.Errorf("could not write a cache over %q", content)
		}
	}
}

// TestAmendKeepsTheCacheTrue covers what "colors set" does to a stored reading:
// the area it changed is updated, and a layout with nothing stored is left with
// nothing stored rather than gaining a cache of one area.
func TestAmendKeepsTheCacheTrue(t *testing.T) {
	isolate(t)

	when := time.Now().Add(-time.Hour).Round(time.Second)
	remember(quiet(), testScanner, "weather", []area{
		{Name: "System_name", Text: "Yellow", TextHex: "#FFFF00", Background: "Black", BackgroundHex: "#000000"},
		{Name: "Department_name", Text: "White", TextHex: "#FFFFFF", Background: "Black", BackgroundHex: "#000000"},
	}, when)

	amend(quiet(), testScanner, "weather", "system_name", func(a *cachedArea) {
		a.Text, a.TextHex = "Red", "#FF0000"
	})

	c, err := loadCache()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	got, ok := c.lookup(scannerKey(testScanner), "weather")
	if !ok {
		t.Fatal("the reading is gone")
	}

	if got.Areas[0].Text != "Red" || got.Areas[0].TextHex != "#FF0000" {
		t.Errorf("the changed area still reads as %+v", got.Areas[0])
	}
	if got.Areas[0].Background != "Black" {
		t.Errorf("setting the text color changed the background: %+v", got.Areas[0])
	}
	if got.Areas[1].Text != "White" {
		t.Errorf("another area changed: %+v", got.Areas[1])
	}

	// The rest of the layout was not re-read, so the reading is no fresher
	// than it was.
	if !got.Read.Equal(when) {
		t.Errorf("read at %s, want %s: amending one area does not refresh the rest", got.Read, when)
	}

	// A layout nothing is stored for gains nothing.
	amend(quiet(), testScanner, "search", "System_name", func(a *cachedArea) { a.Text = "Red" })
	if c, err = loadCache(); err != nil {
		t.Fatalf("loading: %v", err)
	}
	if _, ok := c.lookup(scannerKey(testScanner), "search"); ok {
		t.Error("setting a color built a cache out of one area")
	}
}

// TestScannerKeyPrefersTheSerial covers two scanners on one computer, which the
// USB serial number is the only thing that tells apart. A connection opened by
// port carries none, and then the port has to do.
func TestScannerKeyPrefersTheSerial(t *testing.T) {
	withSerial := scannerKey(device.Info{Port: "/dev/tty.a", Serial: "0001"})
	moved := scannerKey(device.Info{Port: "/dev/tty.b", Serial: "0001"})
	if withSerial != moved {
		t.Errorf("the same scanner in another socket keyed as %q and %q", withSerial, moved)
	}

	byPort := scannerKey(device.Info{Port: "/dev/tty.a"})
	if byPort == withSerial {
		t.Errorf("a scanner with no serial keyed the same as one with: %q", byPort)
	}
	if byPort != scannerKey(device.Info{Port: "/dev/tty.a"}) {
		t.Error("the same port keyed two ways")
	}
}

// blocked puts a directory where the cache file goes, so the file can be
// neither read nor written.
func blocked(t *testing.T) string {
	t.Helper()

	path, err := cachePath()
	if err != nil {
		t.Fatalf("locating the cache: %v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("blocking the cache: %v", err)
	}
	return path
}

// homeless takes away the directory the cache lives in, so it cannot even be
// located.
func homeless(t *testing.T) {
	t.Helper()

	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("LocalAppData", "")
}

// readOnly makes the cache file unwritable, so a reading can be loaded but not
// stored.
// The directory rather than the file is what gets closed off, because the cache
// is replaced by renaming a temporary file over it. A rename asks the directory
// for permission and never the file, so a read-only cache file is still
// perfectly replaceable; a directory nothing may be created in is what actually
// stops a save, and it leaves the existing file readable so the load that comes
// first still works.
func readOnly(t *testing.T) {
	t.Helper()

	path, err := cachePath()
	if err != nil {
		t.Fatalf("locating the cache: %v", err)
	}

	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("making the cache directory read only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// Test_age tests the age function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - None: a reading with no time on it renders a dash
//   - JustNow: a reading taken within the minute says so
//   - Minutes: a reading taken within the hour counts the minutes
//   - Hours: a reading taken within the day counts the hours
//   - Days: anything older counts the days
func Test_age(t *testing.T) {
	// Verify that a reading with no time on it renders a dash
	t.Run("None", func(t *testing.T) {
		if got := age(time.Time{}); got != "-" {
			t.Errorf("a reading with no time rendered as %q, wanted %q", got, "-")
		}
	})

	// Verify that the age is what says whether a reading is worth trusting
	for _, c := range []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"JustNow", 5 * time.Second, "(just now)"},
		{"Minutes", 30 * time.Minute, "(30 minutes ago)"},
		{"Hours", 5 * time.Hour, "(5 hours ago)"},
		{"Days", 50 * time.Hour, "(2 days ago)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := age(time.Now().Add(-c.ago))
			if !strings.Contains(got, c.want) {
				t.Errorf("a reading %s old rendered as %q, wanted %q in it", c.ago, got, c.want)
			}
		})
	}
}

// Test_amend tests the amend function with 100% coverage.
//
// Coverage: 100% (3 test cases covering the branches the round trip leaves)
//
// Test cases:
//   - NoArea: an area the reading does not hold changes nothing
//   - LoadError: a cache that cannot be located is warned about, not failed on
//   - SaveError: a cache that cannot be written is warned about
func Test_amend(t *testing.T) {
	// Verify that an area the stored reading does not hold changes nothing
	t.Run("NoArea", func(t *testing.T) {
		isolate(t)

		when := time.Now().Add(-time.Hour).Round(time.Second)
		remember(quiet(), testScanner, "weather", []area{{Name: "Func", Text: "Yellow"}}, when)

		amend(quiet(), testScanner, "weather", "Made_up", func(a *cachedArea) { a.Text = "Red" })

		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading: %v", err)
		}
		got, _ := c.lookup(scannerKey(testScanner), "weather")
		if got.Areas[0].Text != "Yellow" {
			t.Errorf("an area nobody named was changed: %+v", got.Areas[0])
		}
	})

	// Verify that a cache which cannot be located costs a warning rather than
	// the command that has already done what it was asked
	t.Run("LoadError", func(t *testing.T) {
		isolate(t)
		homeless(t)

		amend(quiet(), testScanner, "weather", "Func", func(a *cachedArea) { a.Text = "Red" })
	})

	// Verify that a cache which cannot be written costs a warning
	t.Run("SaveError", func(t *testing.T) {
		isolate(t)

		remember(quiet(), testScanner, "weather", []area{{Name: "Func", Text: "Yellow"}}, time.Now())
		readOnly(t)

		amend(quiet(), testScanner, "weather", "Func", func(a *cachedArea) { a.Text = "Red" })
	})
}

// Test_forget tests the forget function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Drops: the stored readings named are dropped
//   - NoScanner: a scanner with nothing stored changes nothing
//   - NotHeld: layouts that were never read change nothing
//   - LoadError: a cache that cannot be located is warned about
//   - SaveError: a cache that cannot be written is warned about
func Test_forget(t *testing.T) {
	// Verify that the readings named are dropped and the rest are left
	t.Run("Drops", func(t *testing.T) {
		isolate(t)

		remember(quiet(), testScanner, "weather", []area{{Name: "Func"}}, time.Now())
		remember(quiet(), testScanner, "tone-out", []area{{Name: "Func"}}, time.Now())

		forget(quiet(), testScanner, []string{"weather"})

		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading: %v", err)
		}
		if _, ok := c.lookup(scannerKey(testScanner), "weather"); ok {
			t.Error("a reading that was dropped is still there")
		}
		if _, ok := c.lookup(scannerKey(testScanner), "tone-out"); !ok {
			t.Error("a reading nobody named was dropped as well")
		}
	})

	// Verify that a scanner with nothing stored changes nothing
	t.Run("NoScanner", func(t *testing.T) {
		isolate(t)

		forget(quiet(), testScanner, []string{"weather"})
	})

	// Verify that layouts which were never read change nothing
	t.Run("NotHeld", func(t *testing.T) {
		isolate(t)

		remember(quiet(), testScanner, "weather", []area{{Name: "Func"}}, time.Now())
		forget(quiet(), testScanner, []string{"tone-out"})

		c, err := loadCache()
		if err != nil {
			t.Fatalf("loading: %v", err)
		}
		if _, ok := c.lookup(scannerKey(testScanner), "weather"); !ok {
			t.Error("the reading that was kept is gone")
		}
	})

	// Verify that a cache which cannot be located costs a warning
	t.Run("LoadError", func(t *testing.T) {
		isolate(t)
		homeless(t)

		forget(quiet(), testScanner, []string{"weather"})
	})

	// Verify that a cache which cannot be written costs a warning
	t.Run("SaveError", func(t *testing.T) {
		isolate(t)

		remember(quiet(), testScanner, "weather", []area{{Name: "Func"}}, time.Now())
		readOnly(t)

		forget(quiet(), testScanner, []string{"weather"})
	})
}

// Test_loadCache tests the loadCache function with 100% coverage.
//
// Coverage: 100% (2 test cases covering the branches the round trip leaves)
//
// Test cases:
//   - NoHome: a cache that cannot be located is reported
//   - ReadError: a cache file that cannot be read for any reason other than
//     not being there is reported
func Test_loadCache(t *testing.T) {
	// Verify that a cache with nowhere to live is reported
	t.Run("NoHome", func(t *testing.T) {
		isolate(t)
		homeless(t)

		if _, err := loadCache(); err == nil {
			t.Error("a cache with nowhere to live was loaded")
		}
	})

	// Verify that a cache file which cannot be read is reported, since that is
	// not the same as one that is simply not there yet
	t.Run("ReadError", func(t *testing.T) {
		isolate(t)
		path := blocked(t)

		_, err := loadCache()
		if err == nil || !strings.Contains(err.Error(), "reading "+path) {
			t.Errorf("an unreadable cache came back as %v", err)
		}
	})
}

// Test_missing tests the missing function with 100% coverage.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Names: the failure names the layout and the command that would read it
func Test_missing(t *testing.T) {
	// Verify that the failure says how to fill the cache
	t.Run("Names", func(t *testing.T) {
		err := missing("weather")
		if err == nil || !strings.Contains(err.Error(), "no cached colors for weather") {
			t.Fatalf("the failure reads %v", err)
		}
		if !strings.Contains(err.Error(), `radiocli colors weather`) {
			t.Errorf("the failure does not say how to read them: %v", err)
		}
	})
}

// Test_remember tests the remember function with 100% coverage.
//
// Coverage: 100% (1 test case covering the branch the round trip leaves)
//
// Test cases:
//   - LoadError: a cache that cannot be located is warned about rather than
//     failing a command that already has its answer
func Test_remember(t *testing.T) {
	// Verify that a cache which cannot be located costs a warning
	t.Run("LoadError", func(t *testing.T) {
		isolate(t)
		homeless(t)

		remember(quiet(), testScanner, "weather", []area{{Name: "Func"}}, time.Now())
	})
}

// Test_save tests the save method with 100% coverage.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - NoHome: a cache that cannot be located is reported
//   - DirectoryError: a directory that cannot be created is reported
//   - WriteError: a file that cannot be written is reported
//   - EncodeError: an encoder that refuses the cache is reported
//   - Replaces: a saved cache lands whole, with nothing left beside it
//   - TempError: a temporary file that cannot be made is reported
//   - TempWriteError: a temporary file that cannot be written is reported
func Test_save(t *testing.T) {
	// Verify that a cache with nowhere to live is reported
	t.Run("NoHome", func(t *testing.T) {
		isolate(t)
		homeless(t)

		if err := (&cache{}).save(); err == nil {
			t.Error("a cache with nowhere to live was written")
		}
	})

	// Verify that a directory which cannot be created is reported
	t.Run("DirectoryError", func(t *testing.T) {
		isolate(t)

		path, err := cachePath()
		if err != nil {
			t.Fatalf("locating the cache: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0o755); err != nil {
			t.Fatalf("creating the cache directory: %v", err)
		}
		// A file where the directory belongs, which is as far as the walk gets.
		if err := os.WriteFile(filepath.Dir(path), []byte("in the way"), 0o600); err != nil {
			t.Fatalf("blocking the cache directory: %v", err)
		}

		err = (&cache{}).save()
		if err == nil || !strings.Contains(err.Error(), "creating ") {
			t.Errorf("a blocked directory came back as %v", err)
		}
	})

	// Verify that a file which cannot be written is reported
	t.Run("WriteError", func(t *testing.T) {
		isolate(t)
		path := blocked(t)

		err := (&cache{}).save()
		if err == nil || !strings.Contains(err.Error(), "writing "+path) {
			t.Errorf("a blocked file came back as %v", err)
		}
	})

	// Verify that an encoder which refuses the cache is reported rather than a
	// half written file being left behind
	t.Run("EncodeError", func(t *testing.T) {
		isolate(t)

		marshalJSON = func(any, string, string) ([]byte, error) {
			return nil, errors.New("no")
		}
		t.Cleanup(func() { marshalJSON = json.MarshalIndent })

		err := (&cache{}).save()
		if err == nil || !strings.Contains(err.Error(), "encoding the colors cache") {
			t.Errorf("a cache that cannot be encoded came back as %v", err)
		}
	})

	// Verify that a saved cache lands whole and leaves nothing beside it. The
	// write goes to a temporary file and is renamed over the cache, so a reader
	// sees the old file or the new one and never half of each; what this checks
	// is that the temporary file does not survive the rename, since a cache
	// directory filling up with leftovers is how that goes wrong quietly.
	t.Run("Replaces", func(t *testing.T) {
		isolate(t)

		path, err := cachePath()
		if err != nil {
			t.Fatalf("locating the cache: %v", err)
		}

		c := &cache{}
		c.store(scannerKey(testScanner), testScanner, "weather", []area{{Name: "Func"}}, time.Now())
		if err := c.save(); err != nil {
			t.Fatalf("save: %v", err)
		}

		if _, err := loadCache(); err != nil {
			t.Errorf("the cache that was just written cannot be read back: %v", err)
		}

		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatalf("reading the cache directory: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("the cache directory holds %v, want only %q", names, filepath.Base(path))
		}
	})

	// Verify that a temporary file which cannot be made is reported
	t.Run("TempError", func(t *testing.T) {
		isolate(t)

		createTemp = func(string, string) (*os.File, error) {
			return nil, errors.New("no room")
		}
		t.Cleanup(func() { createTemp = os.CreateTemp })

		path, err := cachePath()
		if err != nil {
			t.Fatalf("locating the cache: %v", err)
		}

		err = (&cache{}).save()
		if err == nil || !strings.Contains(err.Error(), "writing "+path) {
			t.Errorf("a temporary file that could not be made came back as %v", err)
		}
	})

	// Verify that a temporary file which cannot be written is reported. The
	// file is real and is opened read only, which is the one way to have a
	// write fail without inventing a file type.
	t.Run("TempWriteError", func(t *testing.T) {
		isolate(t)

		createTemp = func(dir, pattern string) (*os.File, error) {
			f, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			name := f.Name()
			f.Close()
			return os.Open(name)
		}
		t.Cleanup(func() { createTemp = os.CreateTemp })

		path, err := cachePath()
		if err != nil {
			t.Fatalf("locating the cache: %v", err)
		}

		err = (&cache{}).save()
		if err == nil || !strings.Contains(err.Error(), "writing "+path) {
			t.Errorf("a temporary file that could not be written came back as %v", err)
		}
	})
}

// Test_store tests the store method with 100% coverage.
//
// Coverage: 100% (1 test case covering the branch the round trip leaves)
//
// Test cases:
//   - Empty: a cache holding nothing at all gains the scanner
func Test_store(t *testing.T) {
	// Verify that a cache with no scanners in it gains one
	t.Run("Empty", func(t *testing.T) {
		c := &cache{}
		c.store(scannerKey(testScanner), testScanner, "weather",
			[]area{{Name: "Func", Text: "Yellow"}}, time.Now())

		got, ok := c.lookup(scannerKey(testScanner), "weather")
		if !ok {
			t.Fatal("the reading that was just stored is not there")
		}
		if got.Areas[0].Text != "Yellow" {
			t.Errorf("the reading came back as %+v", got.Areas)
		}
	})
}
