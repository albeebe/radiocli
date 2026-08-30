// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pretendExecutable stands in for the running program for the length of a test,
// since the real one is the test binary.
//
// Parameters:
//   - t: the test the change is tied to
//   - path: what the program should report itself as
func pretendExecutable(t *testing.T, path string) {
	t.Helper()

	was := executablePath
	executablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { executablePath = was })
}

// writeFile puts a file on disk and fails the test if it cannot.
//
// Parameters:
//   - t: the test to fail
//   - path: where to write
//   - body: what to write
//   - mode: the permissions to give it
func writeFile(t *testing.T, path, body string, mode fs.FileMode) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// Test_executable tests finding the file this process runs from.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - FollowsLinks: an install that is a link reports the file behind it
//   - UnresolvableFallsBack: a path that cannot be resolved is used as it is
//   - Unknown: the process cannot say where it is
func Test_executable(t *testing.T) {
	// Verify that a link is followed, so an install pointing into another
	// directory replaces the real file rather than the link
	t.Run("FollowsLinks", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "radiocli-real")
		link := filepath.Join(dir, "radiocli")
		writeFile(t, real, "a program", 0o755)
		if err := os.Symlink(real, link); err != nil {
			t.Fatalf("making the link: %v", err)
		}
		pretendExecutable(t, link)

		got, err := executable()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		// The temporary directory itself can sit behind a link on macOS, so
		// the resolved answer is compared with the resolved original.
		want, err := filepath.EvalSymlinks(real)
		if err != nil {
			t.Fatalf("resolving the real path: %v", err)
		}
		if got != want {
			t.Errorf("expected %q, got: %q", want, got)
		}
	})

	// Verify that a path which cannot be resolved is still usable. On Linux a
	// program already replaced under a running process reports a path with
	// "(deleted)" on the end, and that is not a reason to refuse.
	t.Run("UnresolvableFallsBack", func(t *testing.T) {
		gone := filepath.Join(t.TempDir(), "radiocli (deleted)")
		pretendExecutable(t, gone)

		got, err := executable()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != gone {
			t.Errorf("expected the unresolved path back, got: %q", got)
		}
	})

	// Verify that a process which cannot say where it is fails plainly
	t.Run("Unknown", func(t *testing.T) {
		was := executablePath
		executablePath = func() (string, error) { return "", errors.New("no idea") }
		t.Cleanup(func() { executablePath = was })

		if _, err := executable(); err == nil {
			t.Error("expected not knowing the path to be an error")
		}
	})
}

// Test_modeOf tests reading the permissions to give the new program.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Copies: the mode of the program being replaced is kept
//   - Unreadable: a file that cannot be read falls back to 0755
func Test_modeOf(t *testing.T) {
	// Verify that an unusual mode is preserved rather than widened, which is
	// the whole reason this is not a constant
	t.Run("Copies", func(t *testing.T) {
		requireUnixPermissions(t)
		path := filepath.Join(t.TempDir(), "radiocli")
		writeFile(t, path, "a program", 0o500)

		if got := modeOf(path); got != 0o500 {
			t.Errorf("expected mode 0500 to be kept, got: %v", got)
		}
	})

	// Verify that a file whose mode cannot be read gets the ordinary one
	t.Run("Unreadable", func(t *testing.T) {
		if got := modeOf(filepath.Join(t.TempDir(), "gone")); got != 0o755 {
			t.Errorf("expected the 0755 fallback, got: %v", got)
		}
	})
}

// Test_replaceOn tests putting the new program in place of the running one.
//
// Both platforms are covered here on whichever machine the tests run, which is
// why the operating system is a parameter rather than something this reads.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Unix: one rename puts the new program in place
//   - UnixFails: the rename could not be done
//   - Windows: the running program is moved aside first
//   - WindowsCannotMoveAside: the running program could not be moved
//   - WindowsRestoresOnFailure: a failure after the move puts it back
func Test_replaceOn(t *testing.T) {
	// setUp writes a program and its replacement into a fresh directory.
	setUp := func(t *testing.T) (string, string) {
		t.Helper()

		dir := t.TempDir()
		target := filepath.Join(dir, "radiocli")
		staged := filepath.Join(dir, ".radiocli-update-1")
		writeFile(t, target, "the old program", 0o755)
		writeFile(t, staged, "the new program", 0o755)
		return target, staged
	}

	// Verify that one rename does it, which is what makes the swap atomic
	t.Run("Unix", func(t *testing.T) {
		target, staged := setUp(t)

		if err := replaceOn("darwin", target, staged); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if body, _ := os.ReadFile(target); string(body) != "the new program" {
			t.Errorf("expected the new program in place, got: %q", body)
		}
	})

	// Verify that a rename which cannot be done leaves the running program
	// alone and says so
	t.Run("UnixFails", func(t *testing.T) {
		target, staged := setUp(t)
		was := renameFile
		renameFile = func(string, string) error { return os.ErrPermission }
		t.Cleanup(func() { renameFile = was })

		if err := replaceOn("darwin", target, staged); err == nil {
			t.Error("expected the failed rename to be reported")
		}
		if body, _ := os.ReadFile(target); string(body) != "the old program" {
			t.Errorf("expected the old program untouched, got: %q", body)
		}
	})

	// Verify that Windows moves the running program aside first, since it
	// cannot be overwritten while it runs, and clears the moved copy afterwards
	t.Run("Windows", func(t *testing.T) {
		target, staged := setUp(t)

		if err := replaceOn("windows", target, staged); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if body, _ := os.ReadFile(target); string(body) != "the new program" {
			t.Errorf("expected the new program in place, got: %q", body)
		}
		if _, err := os.Stat(target + oldSuffix); !os.IsNotExist(err) {
			t.Error("expected the moved copy to be cleared away")
		}
	})

	// Verify that failing to move the running program aside stops there
	t.Run("WindowsCannotMoveAside", func(t *testing.T) {
		target, staged := setUp(t)
		was := renameFile
		renameFile = func(string, string) error { return os.ErrPermission }
		t.Cleanup(func() { renameFile = was })

		err := replaceOn("windows", target, staged)
		if err == nil || !strings.Contains(err.Error(), "moving") {
			t.Errorf("expected the move aside to be reported, got: %v", err)
		}
	})

	// Verify that a failure after the program has been moved aside puts it
	// back. Without this the machine would be left with no radiocli at all,
	// which is far worse than not updating.
	t.Run("WindowsRestoresOnFailure", func(t *testing.T) {
		target, staged := setUp(t)

		was := renameFile
		calls := 0
		renameFile = func(from, to string) error {
			calls++
			if calls == 2 {
				return os.ErrPermission
			}
			return was(from, to)
		}
		t.Cleanup(func() { renameFile = was })

		if err := replaceOn("windows", target, staged); err == nil {
			t.Fatal("expected the failed install to be reported")
		}
		if body, _ := os.ReadFile(target); string(body) != "the old program" {
			t.Errorf("expected the old program to be put back, got: %q", body)
		}
	})
}

// Test_sweep tests clearing away the copy a previous Windows update left.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Windows: the moved copy is removed
//   - Elsewhere: nothing is touched on a platform that never makes one
func Test_sweep(t *testing.T) {
	// Verify that a leftover copy is cleared away on the platform that creates
	// them
	t.Run("Windows", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "radiocli")
		writeFile(t, target+oldSuffix, "yesterday's program", 0o755)

		sweep("windows", target)

		if _, err := os.Stat(target + oldSuffix); !os.IsNotExist(err) {
			t.Error("expected the leftover copy to be removed")
		}
	})

	// Verify that nothing is removed on a platform that never leaves one
	t.Run("Elsewhere", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "radiocli")
		writeFile(t, target+oldSuffix, "not ours", 0o755)

		sweep("linux", target)

		if _, err := os.Stat(target + oldSuffix); err != nil {
			t.Error("expected nothing to be touched away from Windows")
		}
	})
}

// Test_writable tests the check that runs before anything is downloaded.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Writable: an ordinary directory
//   - NotWritable: a directory that cannot be written to
func Test_writable(t *testing.T) {
	// Verify that a writable directory passes, and that the probe file does not
	// survive the check
	t.Run("Writable", func(t *testing.T) {
		dir := t.TempDir()

		if err := writable(dir); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		left, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading the directory: %v", err)
		}
		if len(left) != 0 {
			t.Errorf("expected the probe to be cleaned up, found: %v", left)
		}
	})

	// Verify that a directory which cannot be written to is reported through
	// the sentinel, which is what the sudo advice hangs off
	t.Run("NotWritable", func(t *testing.T) {
		requireUnixPermissions(t)
		dir := filepath.Join(t.TempDir(), "locked")
		if err := os.Mkdir(dir, 0o500); err != nil {
			t.Fatalf("making the directory: %v", err)
		}

		err := writable(dir)
		if !errors.Is(err, errNotWritable) {
			t.Errorf("expected the not writable sentinel, got: %v", err)
		}
		if err != nil && !strings.Contains(err.Error(), dir) {
			t.Errorf("expected the directory to be named, got: %v", err)
		}
	})
}
