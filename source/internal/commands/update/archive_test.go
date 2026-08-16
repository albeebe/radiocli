// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// entry is one file to put into a test archive.
type entry struct {
	name string
	body string

	// mode is the entry's file mode. The zero value means an ordinary file.
	mode fs.FileMode

	// dir marks a tar entry that is a directory rather than a file.
	dir bool
}

// makeTarGz writes a gzipped tar holding the given entries.
//
// Parameters:
//   - t: the test the file's lifetime is tied to
//   - entries: what to put in it, in order
//
// Returns:
//   - the path of the archive
func makeTarGz(t *testing.T, entries ...entry) string {
	t.Helper()

	var raw bytes.Buffer
	zipped := gzip.NewWriter(&raw)
	tarball := tar.NewWriter(zipped)

	for _, e := range entries {
		header := &tar.Header{Name: e.name, Size: int64(len(e.body)), Mode: 0o755}
		header.Typeflag = tar.TypeReg
		if e.dir {
			header.Typeflag = tar.TypeDir
			header.Size = 0
		}
		if err := tarball.WriteHeader(header); err != nil {
			t.Fatalf("writing the tar header for %s: %v", e.name, err)
		}
		if !e.dir {
			if _, err := tarball.Write([]byte(e.body)); err != nil {
				t.Fatalf("writing %s into the tar: %v", e.name, err)
			}
		}
	}

	if err := tarball.Close(); err != nil {
		t.Fatalf("closing the tar: %v", err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatalf("closing the gzip: %v", err)
	}

	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(path, raw.Bytes(), 0o644); err != nil {
		t.Fatalf("writing the archive: %v", err)
	}
	return path
}

// makeZip writes a zip holding the given entries.
//
// Parameters:
//   - t: the test the file's lifetime is tied to
//   - entries: what to put in it, in order
//
// Returns:
//   - the path of the archive
func makeZip(t *testing.T, entries ...entry) string {
	t.Helper()

	var raw bytes.Buffer
	archive := zip.NewWriter(&raw)

	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Store}
		if e.mode != 0 {
			header.SetMode(e.mode)
		}

		w, err := archive.CreateHeader(header)
		if err != nil {
			t.Fatalf("adding %s to the zip: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("writing %s into the zip: %v", e.name, err)
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatalf("closing the zip: %v", err)
	}

	path := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(path, raw.Bytes(), 0o644); err != nil {
		t.Fatalf("writing the archive: %v", err)
	}
	return path
}

// unreadableZip writes a zip whose one entry claims a compression method
// nothing implements, which is how an entry that cannot be opened is produced.
//
// The archive is built normally and then the two places recording the method,
// the entry's own header and the index at the end, are overwritten. There is no
// way to ask the zip writer for this directly: it refuses a method it cannot
// write, which is the whole reason the bytes are patched by hand.
//
// Parameters:
//   - t: the test the file's lifetime is tied to
//
// Returns:
//   - the path of the archive
func unreadableZip(t *testing.T) string {
	t.Helper()

	var raw bytes.Buffer
	archive := zip.NewWriter(&raw)
	w, err := archive.CreateHeader(&zip.FileHeader{Name: "radiocli", Method: zip.Store})
	if err != nil {
		t.Fatalf("adding the entry: %v", err)
	}
	if _, err := w.Write([]byte("a program")); err != nil {
		t.Fatalf("writing the entry: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("closing the zip: %v", err)
	}

	// 99 is not a method anybody has assigned, so opening the entry fails
	// rather than producing something.
	const unassigned = 99
	patched := raw.Bytes()
	local := bytes.Index(patched, []byte("PK\x03\x04"))
	index := bytes.Index(patched, []byte("PK\x01\x02"))
	if local < 0 || index < 0 {
		t.Fatal("the zip does not have the headers this expects")
	}
	binary.LittleEndian.PutUint16(patched[local+8:], unassigned)
	binary.LittleEndian.PutUint16(patched[index+10:], unassigned)

	path := filepath.Join(t.TempDir(), "unreadable.zip")
	if err := os.WriteFile(path, patched, 0o644); err != nil {
		t.Fatalf("writing the archive: %v", err)
	}
	return path
}

// Test_extract tests choosing how to read an archive.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Zip: a name ending in .zip is read as one
//   - TarGz: anything else is read as a gzipped tar
func Test_extract(t *testing.T) {
	// Verify that a zip is read as a zip
	t.Run("Zip", func(t *testing.T) {
		archive := makeZip(t, entry{name: "radiocli", body: "a program"})
		want := asset{archive: "radiocli-mac.zip", binary: "radiocli"}

		path, err := extract(archive, want, t.TempDir(), 0o755)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if body, _ := os.ReadFile(path); string(body) != "a program" {
			t.Errorf("expected the program to be written, got: %q", body)
		}
	})

	// Verify that a tarball is read as a tarball
	t.Run("TarGz", func(t *testing.T) {
		archive := makeTarGz(t, entry{name: "radiocli", body: "a program"})
		want := asset{archive: "radiocli-linux.tar.gz", binary: "radiocli"}

		path, err := extract(archive, want, t.TempDir(), 0o755)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if body, _ := os.ReadFile(path); string(body) != "a program" {
			t.Errorf("expected the program to be written, got: %q", body)
		}
	})
}

// Test_extractTarGz tests reading the program out of a gzipped tar.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Found: the named entry is written out and everything else ignored
//   - SkipsDirectories: an entry with the right name that is not a file
//   - Missing: an archive that does not hold the program
//   - NoSuchFile: the archive is not there
//   - NotGzipped: the file is not a gzip at all
//   - Truncated: the archive stops partway through
//   - UnsafeName: an entry whose name is a path
func Test_extractTarGz(t *testing.T) {
	want := asset{archive: "radiocli-linux.tar.gz", binary: "radiocli"}

	// Verify that the named entry is the one written, and that entries before
	// it are passed over
	t.Run("Found", func(t *testing.T) {
		archive := makeTarGz(t,
			entry{name: "readme", body: "ignore me"},
			entry{name: "radiocli", body: "a program"})

		path, err := extractTarGz(archive, want, t.TempDir(), 0o755)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if body, _ := os.ReadFile(path); string(body) != "a program" {
			t.Errorf("expected the program, got: %q", body)
		}
	})

	// Verify that a directory sharing the program's name is not mistaken for it
	t.Run("SkipsDirectories", func(t *testing.T) {
		archive := makeTarGz(t, entry{name: "radiocli", dir: true})

		if _, err := extractTarGz(archive, want, t.TempDir(), 0o755); err == nil {
			t.Error("expected a directory not to be taken for the program")
		}
	})

	// Verify that an archive without the program says so
	t.Run("Missing", func(t *testing.T) {
		archive := makeTarGz(t, entry{name: "readme", body: "nothing useful"})

		_, err := extractTarGz(archive, want, t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "does not contain radiocli") {
			t.Errorf("expected the missing program to be reported, got: %v", err)
		}
	})

	// Verify that an archive which is not there is reported
	t.Run("NoSuchFile", func(t *testing.T) {
		_, err := extractTarGz(filepath.Join(t.TempDir(), "gone"), want, t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "opening") {
			t.Errorf("expected the missing archive to be reported, got: %v", err)
		}
	})

	// Verify that a file which is not a gzip is reported rather than read as an
	// empty archive
	t.Run("NotGzipped", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "plain")
		if err := os.WriteFile(path, []byte("not a gzip"), 0o644); err != nil {
			t.Fatalf("writing the file: %v", err)
		}

		if _, err := extractTarGz(path, want, t.TempDir(), 0o755); err == nil {
			t.Error("expected a file that is not a gzip to be refused")
		}
	})

	// Verify that an archive cut short is a failure rather than a program that
	// happens to be shorter than it should be
	t.Run("Truncated", func(t *testing.T) {
		archive := makeTarGz(t, entry{name: "radiocli", body: strings.Repeat("x", 4096)})
		whole, err := os.ReadFile(archive)
		if err != nil {
			t.Fatalf("reading the archive: %v", err)
		}
		cut := filepath.Join(t.TempDir(), "cut.tar.gz")
		if err := os.WriteFile(cut, whole[:len(whole)-40], 0o644); err != nil {
			t.Fatalf("writing the truncated archive: %v", err)
		}

		if _, err := extractTarGz(cut, want, t.TempDir(), 0o755); err == nil {
			t.Error("expected a truncated archive to be refused")
		}
	})

	// Verify that an entry naming a path rather than a file is refused, which
	// is the guard against an archive that tries to write outside the directory
	t.Run("UnsafeName", func(t *testing.T) {
		archive := makeTarGz(t, entry{name: "../radiocli", body: "somewhere else"})

		_, err := extractTarGz(archive, want, t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "path rather than a file name") {
			t.Errorf("expected the traversing name to be refused, got: %v", err)
		}
	})
}

// Test_extractZip tests reading the program out of a zip.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Found: the named entry is written out and everything else ignored
//   - SkipsLinks: an entry with the right name that is not an ordinary file
//   - Missing: an archive that does not hold the program
//   - NotAZip: the file is not a zip at all
//   - Unopenable: an entry stored in a way nothing can read
//   - UnsafeName: an entry whose name is a path
func Test_extractZip(t *testing.T) {
	want := asset{archive: "radiocli-mac.zip", binary: "radiocli"}

	// Verify that the named entry is the one written
	t.Run("Found", func(t *testing.T) {
		archive := makeZip(t,
			entry{name: "readme", body: "ignore me"},
			entry{name: "radiocli", body: "a program"})

		path, err := extractZip(archive, want, t.TempDir(), 0o755)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if body, _ := os.ReadFile(path); string(body) != "a program" {
			t.Errorf("expected the program, got: %q", body)
		}
	})

	// Verify that a link sharing the program's name is not mistaken for it
	t.Run("SkipsLinks", func(t *testing.T) {
		archive := makeZip(t, entry{
			name: "radiocli",
			body: "elsewhere",
			mode: fs.ModeSymlink | 0o777,
		})

		if _, err := extractZip(archive, want, t.TempDir(), 0o755); err == nil {
			t.Error("expected a link not to be taken for the program")
		}
	})

	// Verify that an archive without the program says so
	t.Run("Missing", func(t *testing.T) {
		archive := makeZip(t, entry{name: "readme", body: "nothing useful"})

		_, err := extractZip(archive, want, t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "does not contain radiocli") {
			t.Errorf("expected the missing program to be reported, got: %v", err)
		}
	})

	// Verify that a file which is not a zip is reported
	t.Run("NotAZip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "plain")
		if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
			t.Fatalf("writing the file: %v", err)
		}

		if _, err := extractZip(path, want, t.TempDir(), 0o755); err == nil {
			t.Error("expected a file that is not a zip to be refused")
		}
	})

	// Verify that an entry nothing can decompress is reported rather than
	// written out empty
	t.Run("Unopenable", func(t *testing.T) {
		_, err := extractZip(unreadableZip(t), want, t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "reading radiocli") {
			t.Errorf("expected the unreadable entry to be reported, got: %v", err)
		}
	})

	// Verify that an entry naming a path rather than a file is refused
	t.Run("UnsafeName", func(t *testing.T) {
		archive := makeZip(t, entry{name: "sub/radiocli", body: "somewhere else"})

		_, err := extractZip(archive, want, t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "path rather than a file name") {
			t.Errorf("expected the nested name to be refused, got: %v", err)
		}
	})
}

// Test_missingEntry tests the message for an archive without the program.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Names: both the archive and what was looked for appear
func Test_missingEntry(t *testing.T) {
	// Verify that the message names both halves, since either one alone leaves
	// the reader guessing
	t.Run("Names", func(t *testing.T) {
		err := missingEntry(asset{archive: "radiocli-mac.zip", binary: "radiocli"})

		for _, want := range []string{"radiocli-mac.zip", "radiocli", "nothing was installed"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("expected %q in the message, got: %v", want, err)
			}
		}
	})
}

// Test_safeName tests refusing an archive entry that is a path.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Plain: an ordinary file name is allowed
//   - Nested: a name with a directory in it is refused
//   - Traversal: a name climbing out of the directory is refused
//   - Absolute: a name rooted at the top of the filesystem is refused
//   - Backslash: a name using Windows separators is refused
func Test_safeName(t *testing.T) {
	// Verify that an ordinary name is allowed, since everything else here is a
	// refusal
	t.Run("Plain", func(t *testing.T) {
		if err := safeName("a.zip", "radiocli"); err != nil {
			t.Errorf("expected a plain name to be allowed, got: %v", err)
		}
	})

	// Verify that a name carrying a directory is refused
	t.Run("Nested", func(t *testing.T) {
		if err := safeName("a.zip", "bin/radiocli"); err == nil {
			t.Error("expected a nested name to be refused")
		}
	})

	// Verify that a name climbing out of the directory is refused, which is the
	// case the guard exists for
	t.Run("Traversal", func(t *testing.T) {
		if err := safeName("a.zip", ".."); err == nil {
			t.Error("expected a climbing name to be refused")
		}
	})

	// Verify that an absolute name is refused
	t.Run("Absolute", func(t *testing.T) {
		if err := safeName("a.zip", "/usr/local/bin/radiocli"); err == nil {
			t.Error("expected an absolute name to be refused")
		}
	})

	// Verify that Windows separators are refused too, since the checks above
	// are about slashes and would not see them
	t.Run("Backslash", func(t *testing.T) {
		if err := safeName("a.zip", `..\radiocli`); err == nil {
			t.Error("expected a backslash name to be refused")
		}
	})
}

// Test_stage tests writing the program out beside the running one.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Writes: the contents land on disk with the permissions asked for
//   - NoDirectory: a directory that cannot be written to
//   - ReadFails: the source stops partway through
//   - TooLarge: more comes out of the archive than any build is
//   - PermissionsFail: the mode could not be set
func Test_stage(t *testing.T) {
	// Verify that the file is written and given the permissions passed in,
	// which is how the replaced program keeps the mode it had
	t.Run("Writes", func(t *testing.T) {
		path, err := stage(strings.NewReader("a program"), t.TempDir(), 0o750)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("reading the staged file: %v", err)
		}
		if info.Mode().Perm() != 0o750 {
			t.Errorf("expected mode 0750, got: %v", info.Mode().Perm())
		}
	})

	// Verify that a directory which cannot be written to is reported
	t.Run("NoDirectory", func(t *testing.T) {
		_, err := stage(strings.NewReader("a program"), "/no/such/place", 0o755)
		if err == nil || !strings.Contains(err.Error(), "staging") {
			t.Errorf("expected staging to fail, got: %v", err)
		}
	})

	// Verify that a source failing partway through does not leave a program
	// that looks complete
	t.Run("ReadFails", func(t *testing.T) {
		path, err := stage(&failingReader{left: 8}, t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "writing the new program") {
			t.Errorf("expected the read failure to be reported, got: %v", err)
		}
		if path == "" {
			t.Error("expected the partial file's path back, so it can be cleaned up")
		}
	})

	// Verify that an archive expanding past the cap is refused, which is the
	// guard against one that claims to be small and is not
	t.Run("TooLarge", func(t *testing.T) {
		shrink(t, 4, 4)

		_, err := stage(strings.NewReader("more than four bytes"), t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "expands past") {
			t.Errorf("expected the cap to refuse it, got: %v", err)
		}
	})

	// Verify that failing to set the permissions is reported, since a program
	// written without the execute bit is not one anybody can run
	t.Run("PermissionsFail", func(t *testing.T) {
		was := chmodFile
		chmodFile = func(string, fs.FileMode) error { return os.ErrPermission }
		t.Cleanup(func() { chmodFile = was })

		_, err := stage(strings.NewReader("a program"), t.TempDir(), 0o755)
		if err == nil || !strings.Contains(err.Error(), "permissions") {
			t.Errorf("expected the permissions failure to be reported, got: %v", err)
		}
	})
}
