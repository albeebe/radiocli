// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Test_cardDescribe tests the card describe method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - ModelAndSerial: both are named
//   - ModelOnly: the model alone is named when the serial was not read
//   - Neither: an unidentified scanner is reported
func Test_cardDescribe(t *testing.T) {

	// Verify that a fully identified card names the radio and the serial.
	t.Run("ModelAndSerial", func(t *testing.T) {
		c := card{Model: "SDS150", Serial: "1234567"}

		if got, want := c.describe(), "SDS150 1234567"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that a card with no serial still names the radio.
	t.Run("ModelOnly", func(t *testing.T) {
		c := card{Model: "SDS150"}

		if got, want := c.describe(), "SDS150"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that a card that identified itself with nothing says so.
	t.Run("Neither", func(t *testing.T) {
		if got, want := (card{}).describe(), "an unidentified scanner"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// Test_cardDir tests the card dir method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the scanner's directory sits inside the volume
func Test_cardDir(t *testing.T) {

	// Verify that the scanner's directory is found inside the volume.
	t.Run("Success", func(t *testing.T) {
		c := card{Root: filepath.Join("/Volumes", "NO NAME")}

		if got, want := c.dir(), filepath.Join("/Volumes", "NO NAME", cardDir); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// Test_findCard tests the findCard function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: a card mounted under a searched directory is found
//   - NoCard: a mount point holding no card is reported
//   - UnlistableDirectory: a directory that cannot be listed is passed over
func Test_findCard(t *testing.T) {

	// Verify that a mounted card is found by its contents.
	t.Run("Success", func(t *testing.T) {
		mounts := t.TempDir()
		root := filepath.Join(mounts, "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		useMountDirs(t, mounts)

		c, err := findCard()
		if err != nil {
			t.Fatalf("finding the card: %v", err)
		}
		if c.Root != root {
			t.Fatalf("got %q, want %q", c.Root, root)
		}
		if c.Model != "SDS150" {
			t.Fatalf("the card was not identified: %+v", c)
		}
	})

	// Verify that mount points holding no card are reported as no card.
	t.Run("NoCard", func(t *testing.T) {
		mounts := t.TempDir()
		if err := os.MkdirAll(filepath.Join(mounts, "Macintosh HD"), 0o755); err != nil {
			t.Fatalf("making the volume: %v", err)
		}

		useMountDirs(t, mounts)

		if _, err := findCard(); !errors.Is(err, errNoCard) {
			t.Fatalf("expected no card, got %v", err)
		}
	})

	// Verify that a directory that cannot be listed is passed over rather than
	// reported.
	t.Run("UnlistableDirectory", func(t *testing.T) {
		useMountDirs(t, filepath.Join(t.TempDir(), "nothing-is-mounted-here"))

		if _, err := findCard(); !errors.Is(err, errNoCard) {
			t.Fatalf("expected no card, got %v", err)
		}
	})
}

// Test_openCard tests the openCard function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Volume: a path to the volume is accepted
//   - ScannerDirectory: a path to the scanner's directory inside it is accepted
//   - NotACard: a path holding no card is reported
//   - ScannerDirectoryElsewhere: a directory named like the scanner's but not on
//     a card is reported
func Test_openCard(t *testing.T) {

	// Verify that the volume itself is accepted.
	t.Run("Volume", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		c, err := openCard(root)
		if err != nil {
			t.Fatalf("opening the card: %v", err)
		}
		if c.Root != root {
			t.Fatalf("got %q, want %q", c.Root, root)
		}
	})

	// Verify that a path one directory too far still gets the card.
	t.Run("ScannerDirectory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		c, err := openCard(filepath.Join(root, cardDir))
		if err != nil {
			t.Fatalf("opening the card: %v", err)
		}
		if c.Root != root {
			t.Fatalf("got %q, want %q", c.Root, root)
		}
	})

	// Verify that a path holding no card says what was expected.
	t.Run("NotACard", func(t *testing.T) {
		path := t.TempDir()

		if _, err := openCard(path); err == nil {
			t.Fatal("expected a path with no card to be refused")
		} else if !strings.Contains(err.Error(), "does not hold a scanner card") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a directory named like the scanner's, with nothing above it,
	// is still refused.
	t.Run("ScannerDirectoryElsewhere", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), cardDir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("making the directory: %v", err)
		}

		// The directory has no directory of its own inside it, so neither the
		// path nor its parent reads as a card.
		if _, err := openCard(filepath.Join(path, cardDir)); err == nil {
			t.Fatal("expected the path to be refused")
		}
	})
}

// Test_readCard tests the readCard function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Success: the identity file names the model, serial and firmware
//   - NoIdentityFile: a card whose identity file is missing is still a card
//   - OtherRecords: records that are not the scanner's are passed over
//   - ShortRecord: a record with too few fields is passed over
//   - NotACard: a path with no scanner directory is refused
//   - NotADirectory: a file where the scanner's directory should be is refused
func Test_readCard(t *testing.T) {

	// Verify that the identity file is read.
	t.Run("Success", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		c, ok := readCard(root)
		if !ok {
			t.Fatal("expected a card")
		}
		if c.Model != "SDS150" || c.Serial != "1234567" || c.Firmware != "1.06.05" {
			t.Fatalf("the card was not identified: %+v", c)
		}
	})

	// Verify that a damaged card is still backed up rather than refused.
	t.Run("NoIdentityFile", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "NO NAME")
		if err := os.MkdirAll(filepath.Join(root, cardDir), 0o755); err != nil {
			t.Fatalf("making the card: %v", err)
		}

		c, ok := readCard(root)
		if !ok {
			t.Fatal("expected a card")
		}
		if c.Model != "" {
			t.Fatalf("expected nothing to be identified, got %+v", c)
		}
	})

	// Verify that the record naming the radio is picked out of the others.
	t.Run("OtherRecords", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeIdentity(t, root, "Version\t1\t2\t3\r\nScanner\tSDS150\t1234567\t1.06.05\r\n")

		c, ok := readCard(root)
		if !ok {
			t.Fatal("expected a card")
		}
		if c.Model != "SDS150" || c.Firmware != "1.06.05" {
			t.Fatalf("the card was not identified: %+v", c)
		}
	})

	// Verify that a record too short to read is passed over.
	t.Run("ShortRecord", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeIdentity(t, root, "Scanner\tSDS150\n")

		c, ok := readCard(root)
		if !ok {
			t.Fatal("expected a card")
		}
		if c.Model != "" {
			t.Fatalf("expected nothing to be identified, got %+v", c)
		}
	})

	// Verify that a path with no scanner directory is not a card.
	t.Run("NotACard", func(t *testing.T) {
		if _, ok := readCard(t.TempDir()); ok {
			t.Fatal("expected no card")
		}
	})

	// Verify that a file standing where the scanner's directory belongs is not
	// a card.
	t.Run("NotADirectory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, cardDir), []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("writing the file: %v", err)
		}

		if _, ok := readCard(root); ok {
			t.Fatal("expected no card")
		}
	})
}

// Test_volumeDirs tests the volumeDirs function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the platform's mount points are listed
func Test_volumeDirs(t *testing.T) {

	// Verify that this platform's mount points are the ones searched.
	t.Run("Success", func(t *testing.T) {
		dirs := volumeDirs()

		switch runtime.GOOS {
		case "darwin":
			if len(dirs) != 1 || dirs[0] != "/Volumes" {
				t.Fatalf("expected /Volumes, got %v", dirs)
			}
		case "linux":
			if len(dirs) < 3 {
				t.Fatalf("expected the usual mount points, got %v", dirs)
			}
		default:
			if dirs != nil {
				t.Fatalf("expected nothing to be searched, got %v", dirs)
			}
		}
	})
}

// Test_volumeDirsFor tests the volumeDirsFor function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Darwin: macOS mounts everything under one directory
//   - Linux: the usual mount points, plus the per-user directories under them
//   - LinuxWithoutUserDirectories: mount points that cannot be listed are
//     skipped
//   - Unsearched: a platform this does not search lists nothing
func Test_volumeDirsFor(t *testing.T) {

	// Verify that macOS searches the one directory it mounts volumes under.
	t.Run("Darwin", func(t *testing.T) {
		dirs := volumeDirsFor("darwin")

		if len(dirs) != 1 || dirs[0] != "/Volumes" {
			t.Fatalf("expected /Volumes, got %v", dirs)
		}
	})

	// Verify that Linux searches the usual mount points and one level down
	// inside the two that are organized per user.
	t.Run("Linux", func(t *testing.T) {
		mounted := t.TempDir()
		if err := os.MkdirAll(filepath.Join(mounted, "alice"), 0o755); err != nil {
			t.Fatalf("making the user's directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(mounted, "notes.txt"), []byte("not a mount"), 0o600); err != nil {
			t.Fatalf("writing the file: %v", err)
		}

		previous := readDir
		readDir = func(name string) ([]os.DirEntry, error) {
			if name == "/media" {
				return os.ReadDir(mounted)
			}
			return nil, os.ErrNotExist
		}
		t.Cleanup(func() { readDir = previous })

		dirs := volumeDirsFor("linux")

		want := []string{"/media", "/run/media", "/mnt", "/media/alice"}
		if len(dirs) != len(want) {
			t.Fatalf("expected %v, got %v", want, dirs)
		}
		for i, d := range want {
			if dirs[i] != d {
				t.Fatalf("expected %v, got %v", want, dirs)
			}
		}
	})

	// Verify that a mount point that cannot be listed leaves only the usual
	// directories.
	t.Run("LinuxWithoutUserDirectories", func(t *testing.T) {
		previous := readDir
		readDir = func(string) ([]os.DirEntry, error) { return nil, os.ErrNotExist }
		t.Cleanup(func() { readDir = previous })

		dirs := volumeDirsFor("linux")

		if len(dirs) != 3 {
			t.Fatalf("expected only the usual mount points, got %v", dirs)
		}
	})

	// Verify that a platform this does not search is not searched.
	t.Run("Unsearched", func(t *testing.T) {
		if dirs := volumeDirsFor("windows"); dirs != nil {
			t.Fatalf("expected nothing to be searched, got %v", dirs)
		}
		if dirs := volumeDirsFor("plan9"); dirs != nil {
			t.Fatalf("expected nothing to be searched, got %v", dirs)
		}
	})
}
