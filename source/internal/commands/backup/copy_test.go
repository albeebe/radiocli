// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test_build tests the build function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - Success: files and directories are listed in a stable order
//   - WithoutDatabase: the downloaded database is left behind
//   - Hidden: what the operating system wrote is passed over
//   - WalkError: a card that cannot be walked is reported
//   - Cancelled: a cancelled context stops the walk
//   - InfoError: a file whose size cannot be read is reported
//   - RelError: a path that cannot be placed inside the card is reported
func Test_build(t *testing.T) {

	// Verify that everything on the card is listed, sorted, and totalled.
	t.Run("Success", func(t *testing.T) {
		src := t.TempDir()
		writeFile(t, filepath.Join(src, "menu.cfg"), "second")
		writeFile(t, filepath.Join(src, "favorites", "a.hpe"), "first")
		writeFile(t, filepath.Join(src, databaseDir, "db.dat"), "database")
		if err := os.MkdirAll(filepath.Join(src, "audio"), 0o755); err != nil {
			t.Fatalf("making the directory: %v", err)
		}
		if err := os.Symlink(filepath.Join(src, "menu.cfg"), filepath.Join(src, "link.cfg")); err != nil {
			t.Fatalf("making the link: %v", err)
		}

		p, err := build(context.Background(), src, true)
		if err != nil {
			t.Fatalf("building: %v", err)
		}
		if got, want := len(p.files), 3; got != want {
			t.Fatalf("got %d files, want %d: %+v", got, want, p.files)
		}
		if p.files[0].rel != filepath.Join(databaseDir, "db.dat") {
			t.Fatalf("the files are not sorted: %+v", p.files)
		}
		if got, want := len(p.dirs), 3; got != want {
			t.Fatalf("got %d directories, want %d: %v", got, want, p.dirs)
		}
		if p.skipped != 1 {
			t.Fatalf("expected the link to be skipped, got %d", p.skipped)
		}
		if p.bytes != int64(len("second")+len("first")+len("database")) {
			t.Fatalf("the total is wrong: %d", p.bytes)
		}
	})

	// Verify that the downloaded database can be left off the backup.
	t.Run("WithoutDatabase", func(t *testing.T) {
		src := t.TempDir()
		writeFile(t, filepath.Join(src, "menu.cfg"), "kept")
		writeFile(t, filepath.Join(src, databaseDir, "db.dat"), "database")

		p, err := build(context.Background(), src, false)
		if err != nil {
			t.Fatalf("building: %v", err)
		}
		if len(p.files) != 1 || p.files[0].rel != "menu.cfg" {
			t.Fatalf("the database was not left behind: %+v", p.files)
		}
		if len(p.dirs) != 0 {
			t.Fatalf("the database directory was not left behind: %v", p.dirs)
		}
	})

	// Verify that what the operating system wrote onto the card is passed over.
	t.Run("Hidden", func(t *testing.T) {
		src := t.TempDir()
		writeFile(t, filepath.Join(src, ".DS_Store"), "spotlight")
		writeFile(t, filepath.Join(src, ".Spotlight-V100", "index"), "spotlight")
		writeFile(t, filepath.Join(src, "System Volume Information", "tracking.log"), "windows")
		writeFile(t, filepath.Join(src, "menu.cfg"), "kept")

		p, err := build(context.Background(), src, true)
		if err != nil {
			t.Fatalf("building: %v", err)
		}
		if len(p.files) != 1 || p.files[0].rel != "menu.cfg" {
			t.Fatalf("the host's files were not passed over: %+v", p.files)
		}
		if len(p.dirs) != 0 {
			t.Fatalf("the host's directories were not passed over: %v", p.dirs)
		}
	})

	// Verify that a card that cannot be walked is reported.
	t.Run("WalkError", func(t *testing.T) {
		_, err := build(context.Background(), filepath.Join(t.TempDir(), "gone"), true)
		if err == nil {
			t.Fatal("expected walking a missing card to fail")
		}
		if !strings.Contains(err.Error(), "reading the card") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a cancelled context stops the walk.
	t.Run("Cancelled", func(t *testing.T) {
		src := t.TempDir()
		writeFile(t, filepath.Join(src, "menu.cfg"), "kept")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := build(ctx, src, true); err == nil {
			t.Fatal("expected the walk to be stopped")
		}
	})

	// Verify that a file whose size cannot be read is reported rather than
	// copied blind.
	t.Run("InfoError", func(t *testing.T) {
		skipAsRoot(t)

		src := t.TempDir()
		closed := filepath.Join(src, "closed")
		writeFile(t, filepath.Join(closed, "menu.cfg"), "kept")

		// The directory can be listed but not searched, so the entries are read
		// and then cannot be looked at one by one.
		if err := os.Chmod(closed, 0o400); err != nil {
			t.Fatalf("closing the directory: %v", err)
		}
		t.Cleanup(func() { os.Chmod(closed, 0o755) })

		if _, err := build(context.Background(), src, true); err == nil {
			t.Fatal("expected reading the file to fail")
		}
	})

	// Verify that a path the walk reports that cannot be expressed inside the
	// card is reported rather than copied to somewhere unintended.
	t.Run("RelError", func(t *testing.T) {
		src := t.TempDir()
		writeFile(t, filepath.Join(src, "menu.cfg"), "kept")

		previous := relPath
		relPath = func(string, string) (string, error) { return "", errors.New("not inside the card") }
		t.Cleanup(func() { relPath = previous })

		_, err := build(context.Background(), src, true)
		if err == nil {
			t.Fatal("expected placing the path to fail")
		}
		if !strings.Contains(err.Error(), "not inside the card") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// Test_copyFile tests the copyFile function with 100% coverage.
//
// Coverage: 100% (8 test cases covering all branches)
//
// Test cases:
//   - Success: the file is copied and no digest is reported
//   - Verified: the file is copied and its digest is reported
//   - SourceMissing: a file that cannot be read is reported
//   - DestinationMissing: a file that cannot be written is reported
//   - CopyError: a source that cannot be read through is reported
//   - CloseError: a copy that cannot be finished is reported
//   - DigestError: a copy that cannot be read back is reported
//   - Mismatch: a copy that does not match the card is reported
func Test_copyFile(t *testing.T) {

	// Verify that a file is copied byte for byte.
	t.Run("Success", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "menu.cfg")
		to := filepath.Join(dir, "menu.copy")
		writeFile(t, from, "the scanner's settings")

		digest, err := copyFile(from, to, false)
		if err != nil {
			t.Fatalf("copying: %v", err)
		}
		if digest != "" {
			t.Fatalf("expected no digest, got %q", digest)
		}
		if got := readFile(t, to); got != "the scanner's settings" {
			t.Fatalf("the copy differs: %q", got)
		}
	})

	// Verify that a verified copy reports the digest it was checked against.
	t.Run("Verified", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "menu.cfg")
		to := filepath.Join(dir, "menu.copy")
		writeFile(t, from, "the scanner's settings")

		digest, err := copyFile(from, to, true)
		if err != nil {
			t.Fatalf("copying: %v", err)
		}
		sum := sha256.Sum256([]byte("the scanner's settings"))
		if digest != hex.EncodeToString(sum[:]) {
			t.Fatalf("the digest is wrong: %q", digest)
		}
	})

	// Verify that a file missing from the card is reported.
	t.Run("SourceMissing", func(t *testing.T) {
		dir := t.TempDir()

		_, err := copyFile(filepath.Join(dir, "gone.cfg"), filepath.Join(dir, "gone.copy"), false)
		if err == nil {
			t.Fatal("expected reading a missing file to fail")
		}
		if !strings.Contains(err.Error(), "reading ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a destination that cannot be written is reported.
	t.Run("DestinationMissing", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "menu.cfg")
		writeFile(t, from, "the scanner's settings")

		_, err := copyFile(from, filepath.Join(dir, "gone", "menu.copy"), false)
		if err == nil {
			t.Fatal("expected writing into a missing directory to fail")
		}
		if !strings.Contains(err.Error(), "writing ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a source that cannot be read through is reported.
	t.Run("CopyError", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "a-directory")
		if err := os.MkdirAll(from, 0o755); err != nil {
			t.Fatalf("making the directory: %v", err)
		}

		_, err := copyFile(from, filepath.Join(dir, "menu.copy"), false)
		if err == nil {
			t.Fatal("expected reading a directory to fail")
		}
		if !strings.Contains(err.Error(), "copying ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a copy that cannot be finished is reported rather than
	// treated as written.
	t.Run("CloseError", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "empty.cfg")
		to := filepath.Join(dir, "empty.copy")
		writeFile(t, from, "")

		// A file that is already closed stands in for one the system refuses to
		// finish writing.
		previous := createFile
		createFile = func(name string) (*os.File, error) {
			f, err := os.Create(name)
			if err != nil {
				return nil, err
			}
			return f, f.Close()
		}
		t.Cleanup(func() { createFile = previous })

		_, err := copyFile(from, to, true)
		if err == nil {
			t.Fatal("expected finishing the copy to fail")
		}
		if !strings.Contains(err.Error(), "finishing ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a copy that cannot be read back is reported.
	t.Run("DigestError", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "menu.cfg")
		writeFile(t, from, "the scanner's settings")

		// The copy is written somewhere else, so the path that gets read back
		// holds nothing.
		previous := createFile
		createFile = func(name string) (*os.File, error) {
			return os.Create(filepath.Join(dir, "elsewhere.copy"))
		}
		t.Cleanup(func() { createFile = previous })

		_, err := copyFile(from, filepath.Join(dir, "menu.copy"), true)
		if err == nil {
			t.Fatal("expected reading the copy back to fail")
		}
		if !strings.Contains(err.Error(), "checking ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a copy that does not match the card is refused.
	t.Run("Mismatch", func(t *testing.T) {
		dir := t.TempDir()
		from := filepath.Join(dir, "menu.cfg")
		to := filepath.Join(dir, "menu.copy")
		writeFile(t, from, "the scanner's settings")
		writeFile(t, to, "something else entirely")

		// The copy is written somewhere else, so what gets read back is the
		// file that was already there and does not match.
		previous := createFile
		createFile = func(name string) (*os.File, error) {
			return os.Create(filepath.Join(dir, "elsewhere.copy"))
		}
		t.Cleanup(func() { createFile = previous })

		_, err := copyFile(from, to, true)
		if err == nil {
			t.Fatal("expected the copy to be refused")
		}
		if !strings.Contains(err.Error(), "did not copy correctly") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// Test_digestOf tests the digestOf function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the file's digest is reported as hex
//   - Missing: a file that cannot be opened is reported
//   - Unreadable: a file that cannot be read through is reported
func Test_digestOf(t *testing.T) {

	// Verify that the digest matches the file's contents.
	t.Run("Success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "menu.cfg")
		writeFile(t, path, "the scanner's settings")

		got, err := digestOf(path)
		if err != nil {
			t.Fatalf("hashing: %v", err)
		}
		sum := sha256.Sum256([]byte("the scanner's settings"))
		if got != hex.EncodeToString(sum[:]) {
			t.Fatalf("the digest is wrong: %q", got)
		}
	})

	// Verify that a file that is not there is reported.
	t.Run("Missing", func(t *testing.T) {
		_, err := digestOf(filepath.Join(t.TempDir(), "gone.cfg"))
		if err == nil {
			t.Fatal("expected hashing a missing file to fail")
		}
		if !strings.Contains(err.Error(), "checking ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a file that cannot be read through is reported.
	t.Run("Unreadable", func(t *testing.T) {
		_, err := digestOf(t.TempDir())
		if err == nil {
			t.Fatal("expected hashing a directory to fail")
		}
		if !strings.Contains(err.Error(), "checking ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// Test_hidden tests the hidden function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Dot: anything beginning with a dot belongs to the operating system
//   - SystemVolumeInformation: what Windows writes belongs to it
//   - Scanner: the scanner's own directories are kept
func Test_hidden(t *testing.T) {

	// Verify that what macOS writes onto the card is recognized.
	t.Run("Dot", func(t *testing.T) {
		if !hidden(".Spotlight-V100") {
			t.Fatal("expected a dot directory to be the host's")
		}
	})

	// Verify that what Windows writes onto the card is recognized.
	t.Run("SystemVolumeInformation", func(t *testing.T) {
		if !hidden("System Volume Information") {
			t.Fatal("expected the Windows directory to be the host's")
		}
	})

	// Verify that the scanner's own directories are kept.
	t.Run("Scanner", func(t *testing.T) {
		if hidden(databaseDir) {
			t.Fatal("expected the scanner's directory to be kept")
		}
	})
}

// Test_human tests the human function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Bytes: a count smaller than a kilobyte is reported in bytes
//   - Kilobytes: a count is reported in the largest unit it fills
//   - Gigabytes: a card sized count is reported in gigabytes
func Test_human(t *testing.T) {

	// Verify that a small count is left in bytes.
	t.Run("Bytes", func(t *testing.T) {
		if got, want := human(512), "512 B"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that a count that fills a kilobyte is reported as one.
	t.Run("Kilobytes", func(t *testing.T) {
		if got, want := human(1536), "1.5 KB"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that a card sized count is reported in gigabytes.
	t.Run("Gigabytes", func(t *testing.T) {
		if got, want := human(3*1024*1024*1024), "3.0 GB"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// Test_planRun tests the plan run method with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Success: every file and directory is recreated and reported
//   - NoProgress: a copy with nothing watching it still runs
//   - DirectoryError: a directory tree that cannot be recreated is reported
//   - Cancelled: a cancelled context stops the copy between files
//   - FileDirectoryError: a file's directory that cannot be made is reported
//   - CopyError: a file that cannot be copied is reported with what was done
func Test_planRun(t *testing.T) {

	// Verify that the tree is recreated, the files are copied, and the empty
	// directories survive.
	t.Run("Success", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		writeFile(t, filepath.Join(src, "menu.cfg"), "settings")
		writeFile(t, filepath.Join(src, "favorites", "a.hpe"), "list")

		p := plan{
			files: []file{{rel: "menu.cfg", size: 8}, {rel: filepath.Join("favorites", "a.hpe"), size: 4}},
			dirs:  []string{"audio", "favorites"},
		}

		seen := 0
		results, err := p.run(context.Background(), src, dst, true, func(f file) { seen++ })
		if err != nil {
			t.Fatalf("copying: %v", err)
		}
		if seen != 2 {
			t.Fatalf("expected two files to be reported, got %d", seen)
		}
		if len(results) != 2 || results[0].Digest == "" {
			t.Fatalf("the results are wrong: %+v", results)
		}
		if _, err := os.Stat(filepath.Join(dst, "audio")); err != nil {
			t.Fatalf("the empty directory did not survive: %v", err)
		}
		if got := readFile(t, filepath.Join(dst, "menu.cfg")); got != "settings" {
			t.Fatalf("the copy differs: %q", got)
		}
	})

	// Verify that a copy with nothing watching it still runs.
	t.Run("NoProgress", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		writeFile(t, filepath.Join(src, "menu.cfg"), "settings")

		results, err := plan{files: []file{{rel: "menu.cfg", size: 8}}}.
			run(context.Background(), src, dst, false, nil)
		if err != nil {
			t.Fatalf("copying: %v", err)
		}
		if len(results) != 1 || results[0].Digest != "" {
			t.Fatalf("the results are wrong: %+v", results)
		}
	})

	// Verify that a tree that cannot be recreated is reported.
	t.Run("DirectoryError", func(t *testing.T) {
		skipAsRoot(t)

		dst := t.TempDir()
		if err := os.Chmod(dst, 0o500); err != nil {
			t.Fatalf("closing the destination: %v", err)
		}
		t.Cleanup(func() { os.Chmod(dst, 0o755) })

		_, err := plan{dirs: []string{"audio"}}.
			run(context.Background(), t.TempDir(), dst, false, nil)
		if err == nil {
			t.Fatal("expected making the directory to fail")
		}
		if !strings.Contains(err.Error(), "creating ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a cancelled context stops the copy.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := plan{files: []file{{rel: "menu.cfg"}}}.
			run(ctx, t.TempDir(), t.TempDir(), false, nil)
		if err == nil {
			t.Fatal("expected the copy to be stopped")
		}
	})

	// Verify that a file whose directory cannot be made is reported.
	t.Run("FileDirectoryError", func(t *testing.T) {
		skipAsRoot(t)

		src := t.TempDir()
		dst := t.TempDir()
		writeFile(t, filepath.Join(src, "favorites", "a.hpe"), "list")
		if err := os.Chmod(dst, 0o500); err != nil {
			t.Fatalf("closing the destination: %v", err)
		}
		t.Cleanup(func() { os.Chmod(dst, 0o755) })

		_, err := plan{files: []file{{rel: filepath.Join("favorites", "a.hpe")}}}.
			run(context.Background(), src, dst, false, nil)
		if err == nil {
			t.Fatal("expected making the directory to fail")
		}
		if !strings.Contains(err.Error(), "creating ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a file that cannot be copied is reported along with what was
	// copied before it.
	t.Run("CopyError", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		writeFile(t, filepath.Join(src, "menu.cfg"), "settings")

		results, err := plan{files: []file{{rel: "menu.cfg"}, {rel: "gone.cfg"}}}.
			run(context.Background(), src, dst, false, nil)
		if err == nil {
			t.Fatal("expected copying a missing file to fail")
		}
		if len(results) != 1 {
			t.Fatalf("expected what was copied to be reported, got %+v", results)
		}
	})
}
