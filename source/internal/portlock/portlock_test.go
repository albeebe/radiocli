// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package portlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// useTempDir points the lock directory at a temporary directory for the length
// of one test, so nothing is written into the directory the developer's own
// invocations lock in.
//
// Parameters:
//   - t: the running test, whose cleanup restores the real directory
//
// Returns:
//   - string path of the temporary directory the locks are now kept under
func useTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	previous := tempDir
	tempDir = func() string { return dir }
	t.Cleanup(func() { tempDir = previous })
	return dir
}

// TestAcquire tests the Acquire function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Success: the port is claimed and the claim is described in the file
//   - Busy: a second claim on the same port is refused and names the holder
//   - BusyAfterWaiting: a claim that waits is refused once the wait elapses
//   - DirectoryError: a lock directory that cannot be created is reported
//   - OpenError: a lock file that cannot be opened is reported
//   - LockError: a lock refused for a reason other than contention is reported
func TestAcquire(t *testing.T) {

	// Verify that a free port is claimed and the holder is written down.
	t.Run("Success", func(t *testing.T) {
		useTempDir(t)

		lock, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err != nil {
			t.Fatalf("acquiring: %v", err)
		}
		defer lock.Release()

		if _, err := os.Stat(lock.path); err != nil {
			t.Fatalf("the lock file is missing: %v", err)
		}
		data, err := os.ReadFile(lock.path)
		if err != nil {
			t.Fatalf("reading the lock file: %v", err)
		}
		if !strings.HasPrefix(string(data), fmt.Sprintf("%d\n", os.Getpid())) {
			t.Fatalf("the lock file does not name this process: %q", data)
		}
	})

	// Verify that a port already held is refused, and that the refusal says who
	// is holding it.
	t.Run("Busy", func(t *testing.T) {
		useTempDir(t)

		held, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err != nil {
			t.Fatalf("acquiring: %v", err)
		}
		defer held.Release()

		lock, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err == nil {
			lock.Release()
			t.Fatal("expected the second claim to be refused")
		}
		if !errors.Is(err, ErrBusy) {
			t.Fatalf("expected a busy error, got %v", err)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("pid %d", os.Getpid())) {
			t.Fatalf("the refusal does not name the holder: %v", err)
		}
	})

	// Verify that a claim given time to wait re-tries and is still refused when
	// the holder does not finish.
	t.Run("BusyAfterWaiting", func(t *testing.T) {
		useTempDir(t)

		previous := retryInterval
		retryInterval = time.Millisecond
		t.Cleanup(func() { retryInterval = previous })

		held, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err != nil {
			t.Fatalf("acquiring: %v", err)
		}
		defer held.Release()

		lock, err := Acquire("/dev/cu.usbmodem1101", 5*time.Millisecond)
		if err == nil {
			lock.Release()
			t.Fatal("expected the waiting claim to be refused")
		}
		if !errors.Is(err, ErrBusy) {
			t.Fatalf("expected a busy error, got %v", err)
		}
	})

	// Verify that a lock directory that cannot be created is reported.
	t.Run("DirectoryError", func(t *testing.T) {
		blocked := filepath.Join(t.TempDir(), "a-file")
		if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("writing the blocking file: %v", err)
		}

		previous := tempDir
		tempDir = func() string { return blocked }
		t.Cleanup(func() { tempDir = previous })

		if _, err := Acquire("/dev/cu.usbmodem1101", 0); err == nil {
			t.Fatal("expected creating the directory to fail")
		} else if !strings.Contains(err.Error(), "creating ") {
			t.Fatalf("expected a creating error, got %v", err)
		}
	})

	// Verify that a lock file that cannot be opened is reported.
	t.Run("OpenError", func(t *testing.T) {
		useTempDir(t)

		path := lockPath("/dev/cu.usbmodem1101")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("putting a directory in the way: %v", err)
		}

		if _, err := Acquire("/dev/cu.usbmodem1101", 0); err == nil {
			t.Fatal("expected opening the lock file to fail")
		} else if !strings.Contains(err.Error(), "opening ") {
			t.Fatalf("expected an opening error, got %v", err)
		}
	})

	// Verify that a lock refused for a reason other than contention is reported
	// rather than waited out.
	t.Run("LockError", func(t *testing.T) {
		useTempDir(t)

		previous := takeLock
		takeLock = func(f *os.File) error { return errors.New("the kernel refused the lock") }
		t.Cleanup(func() { takeLock = previous })

		if _, err := Acquire("/dev/cu.usbmodem1101", 0); err == nil {
			t.Fatal("expected the lock to fail")
		} else if !strings.Contains(err.Error(), "locking ") {
			t.Fatalf("expected a locking error, got %v", err)
		}
	})
}

// TestRelease tests the Release method with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: a held lock is given back
//   - NotHeld: a nil Lock and a released Lock both report nothing
//   - UnlockError: a lock the system refuses to unlock is reported
//   - CloseError: a lock file that cannot be closed is reported
func TestRelease(t *testing.T) {

	// Verify that a held lock is released and the file is let go.
	t.Run("Success", func(t *testing.T) {
		useTempDir(t)

		lock, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err != nil {
			t.Fatalf("acquiring: %v", err)
		}
		if err := lock.Release(); err != nil {
			t.Fatalf("releasing: %v", err)
		}
		if lock.file != nil {
			t.Fatal("expected the lock file to be let go")
		}

		again, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err != nil {
			t.Fatalf("expected the port to be free again: %v", err)
		}
		again.Release()
	})

	// Verify that releasing nothing is not an error.
	t.Run("NotHeld", func(t *testing.T) {
		var nilLock *Lock
		if err := nilLock.Release(); err != nil {
			t.Fatalf("releasing a nil lock: %v", err)
		}
		if err := (&Lock{}).Release(); err != nil {
			t.Fatalf("releasing an empty lock: %v", err)
		}
	})

	// Verify that a refused unlock is reported.
	t.Run("UnlockError", func(t *testing.T) {
		useTempDir(t)

		previous := releaseLock
		releaseLock = func(f *os.File) error { return errors.New("the kernel kept the lock") }
		t.Cleanup(func() { releaseLock = previous })

		lock, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err != nil {
			t.Fatalf("acquiring: %v", err)
		}
		if err := lock.Release(); err == nil {
			t.Fatal("expected releasing to fail")
		} else if !strings.Contains(err.Error(), "unlocking ") {
			t.Fatalf("expected an unlocking error, got %v", err)
		}
	})

	// Verify that a lock file that cannot be closed is reported.
	t.Run("CloseError", func(t *testing.T) {
		useTempDir(t)

		lock, err := Acquire("/dev/cu.usbmodem1101", 0)
		if err != nil {
			t.Fatalf("acquiring: %v", err)
		}

		// Closing the file behind Release leaves the close to fail, and the
		// unlock is stubbed out so that the close is the only failure.
		previous := releaseLock
		releaseLock = func(f *os.File) error { return nil }
		t.Cleanup(func() { releaseLock = previous })
		if err := lock.file.Close(); err != nil {
			t.Fatalf("closing the lock file: %v", err)
		}

		if err := lock.Release(); err == nil {
			t.Fatal("expected releasing to fail")
		} else if !strings.Contains(err.Error(), "closing ") {
			t.Fatalf("expected a closing error, got %v", err)
		}
	})
}

// TestSocketDir tests the SocketDir function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Success: the socket directory is this account's own, below the lock files
func TestSocketDir(t *testing.T) {

	// Verify that the sockets are kept in a directory of this account's own,
	// one level below the lock files. The lock files are shared with every
	// account on the machine and the sockets must not be.
	t.Run("Success", func(t *testing.T) {
		dir := useTempDir(t)

		want := filepath.Join(dir, "radiocli", socketsFor(os.Getuid()))
		if got := SocketDir(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// TestSocketPath tests the SocketPath function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the socket sits in the socket directory under a digest name
//   - DifferentPorts: two ports do not share a socket
func TestSocketPath(t *testing.T) {

	// Verify that the socket is named for the port and lives in the account's
	// own socket directory.
	t.Run("Success", func(t *testing.T) {
		useTempDir(t)

		path := SocketPath("/dev/cu.usbmodem1101")
		if filepath.Dir(path) != SocketDir() {
			t.Fatalf("the socket is not in the socket directory: %q", path)
		}
		if name := filepath.Base(path); len(name) != len("0123456789abcdef")+len(".sock") {
			t.Fatalf("the socket name is not a digest: %q", name)
		}
	})

	// Verify that two different ports get two different sockets.
	t.Run("DifferentPorts", func(t *testing.T) {
		useTempDir(t)

		first := SocketPath("/dev/cu.usbmodem1101")
		second := SocketPath("/dev/cu.usbmodem1102")
		if first == second {
			t.Fatalf("two ports share a socket: %q", first)
		}
	})
}

// Test_describe tests the describe method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the process, the command line and the time are written down
//   - TruncateError: a file that cannot be truncated is left alone
//   - SeekError: a file that cannot be seeked is left alone
func Test_describe(t *testing.T) {

	// Verify that the holder writes down three lines about itself.
	t.Run("Success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		l := &Lock{file: f, path: path}
		l.describe()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the lock file: %v", err)
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected three lines, got %q", data)
		}
		if lines[0] != fmt.Sprintf("%d", os.Getpid()) {
			t.Fatalf("the first line is not this process: %q", lines[0])
		}
		if _, err := time.Parse(time.RFC3339Nano, lines[2]); err != nil {
			t.Fatalf("the third line is not a time: %q", lines[2])
		}
	})

	// Verify that a lock file that cannot be emptied is left as it was.
	t.Run("TruncateError", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		if err := os.WriteFile(path, []byte("left alone"), 0o600); err != nil {
			t.Fatalf("writing the lock file: %v", err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		(&Lock{file: f, path: path}).describe()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the lock file: %v", err)
		}
		if string(data) != "left alone" {
			t.Fatalf("the lock file was changed: %q", data)
		}
	})

	// Verify that a lock file that cannot be seeked back to its start is left
	// empty rather than written to at whatever offset it was at.
	t.Run("SeekError", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		previous := seekFile
		seekFile = func(*os.File, int64, int) (int64, error) {
			return 0, errors.New("seek refused")
		}
		t.Cleanup(func() { seekFile = previous })

		(&Lock{file: f, path: path}).describe()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the lock file: %v", err)
		}
		if len(data) != 0 {
			t.Fatalf("the lock file was written to: %q", data)
		}
	})
}

// Test_lockPath tests the lockPath function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the name carries a readable form of the port and a digest
//   - DifferentPorts: two ports that sanitize the same do not share a lock
func Test_lockPath(t *testing.T) {

	// Verify that the lock file is named for the port and lives in the tool's
	// own directory.
	t.Run("Success", func(t *testing.T) {
		dir := useTempDir(t)

		path := lockPath("/dev/cu.usbmodem1101")
		if filepath.Dir(path) != filepath.Join(dir, "radiocli") {
			t.Fatalf("the lock file is not in the lock directory: %q", path)
		}
		if !strings.HasPrefix(filepath.Base(path), "dev-cu.usbmodem1101-") {
			t.Fatalf("the lock file is not readable: %q", path)
		}
		if !strings.HasSuffix(path, ".lock") {
			t.Fatalf("the lock file is not a lock: %q", path)
		}
	})

	// Verify that the digest keeps two ports apart when the readable part
	// cannot.
	t.Run("DifferentPorts", func(t *testing.T) {
		useTempDir(t)

		first := lockPath("/dev/ttyS0")
		second := lockPath("/dev:ttyS0")
		if first == second {
			t.Fatalf("two ports share a lock file: %q", first)
		}
	})
}

// Test_readHolder tests the readHolder function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Success: the process, the command line and how long it has held are named
//   - UnreadableTime: a record whose time does not parse still names the process
//   - ShortRecord: a half written record reports nothing
//   - UnreadablePid: a record whose process is not a number reports nothing
//   - SeekError: a closed file reports nothing
//   - ReadError: a file that cannot be read reports nothing
func Test_readHolder(t *testing.T) {

	// Verify that a complete record is rendered for the refusal.
	t.Run("Success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		started := time.Now().Add(-90 * time.Second).Format(time.RFC3339Nano)
		if err := os.WriteFile(path, []byte("4321\nbackup --verify\n"+started+"\n"), 0o600); err != nil {
			t.Fatalf("writing the lock file: %v", err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		got := readHolder(f)
		if !strings.Contains(got, `pid 4321 running "backup --verify"`) {
			t.Fatalf("the holder is not named: %q", got)
		}
		if !strings.Contains(got, "for 1m30s") {
			t.Fatalf("how long it has held is not reported: %q", got)
		}
	})

	// Verify that a record whose time is unreadable still names the process.
	t.Run("UnreadableTime", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		if err := os.WriteFile(path, []byte("4321\nbackup\nnot a time\n"), 0o600); err != nil {
			t.Fatalf("writing the lock file: %v", err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		got := readHolder(f)
		if !strings.Contains(got, `pid 4321 running "backup"`) {
			t.Fatalf("the holder is not named: %q", got)
		}
		if strings.Contains(got, " for ") {
			t.Fatalf("expected no holding time, got %q", got)
		}
	})

	// Verify that a record caught half written reports nothing.
	t.Run("ShortRecord", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		if err := os.WriteFile(path, []byte("4321\nbackup\n"), 0o600); err != nil {
			t.Fatalf("writing the lock file: %v", err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		if got := readHolder(f); got != "" {
			t.Fatalf("expected nothing, got %q", got)
		}
	})

	// Verify that a record whose process is not a number reports nothing.
	t.Run("UnreadablePid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		if err := os.WriteFile(path, []byte("nobody\nbackup\nnot a time\n"), 0o600); err != nil {
			t.Fatalf("writing the lock file: %v", err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		defer f.Close()

		if got := readHolder(f); got != "" {
			t.Fatalf("expected nothing, got %q", got)
		}
	})

	// Verify that a file that cannot be sought reports nothing.
	t.Run("SeekError", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "held.lock")
		if err := os.WriteFile(path, []byte("4321\nbackup\nnot a time\n"), 0o600); err != nil {
			t.Fatalf("writing the lock file: %v", err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening the lock file: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("closing the lock file: %v", err)
		}

		if got := readHolder(f); got != "" {
			t.Fatalf("expected nothing, got %q", got)
		}
	})

	// Verify that a file that cannot be read reports nothing.
	t.Run("ReadError", func(t *testing.T) {
		f, err := os.Open(t.TempDir())
		if err != nil {
			t.Fatalf("opening the directory: %v", err)
		}
		defer f.Close()

		if got := readHolder(f); got != "" {
			t.Fatalf("expected nothing, got %q", got)
		}
	})
}

// Test_socketsFor tests the socketsFor function with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - Accounts: a numbered account gets its own directory and an unnumbered
//     one shares the default
func Test_socketsFor(t *testing.T) {
	// Verify that the account's number is in the name, and that a system which
	// does not number accounts still gets a name.
	t.Run("Accounts", func(t *testing.T) {
		for uid, want := range map[int]string{
			0:   "sockets-0",
			501: "sockets-501",
			-1:  "sockets",
		} {
			if got := socketsFor(uid); got != want {
				t.Fatalf("socketsFor(%d) = %q, want %q", uid, got, want)
			}
		}
	})
}

// Test_sanitize tests the sanitize function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Unix: a device path loses its slashes
//   - Windows: a device path loses its backslashes
//   - Portable: letters, digits, dots, underscores and dashes are kept
//   - Unusual: everything else becomes a dash
func Test_sanitize(t *testing.T) {

	// Verify that a unix device path becomes a file name.
	t.Run("Unix", func(t *testing.T) {
		if got, want := sanitize("/dev/cu.usbmodem1101"), "dev-cu.usbmodem1101"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that a Windows device path becomes a file name.
	t.Run("Windows", func(t *testing.T) {
		if got, want := sanitize(`\\.\COM3`), ".-COM3"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that the portable characters survive untouched.
	t.Run("Portable", func(t *testing.T) {
		if got, want := sanitize("aZ09._-"), "aZ09._-"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that anything else is replaced.
	t.Run("Unusual", func(t *testing.T) {
		if got, want := sanitize("com 1:ttyÅ"), "com-1-tty-"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// Test_selfDescription tests the selfDescription function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - NoArguments: the bare tool name is used
//   - Arguments: the command line is reported as it was typed
//   - Newlines: a line break in an argument becomes a space
//   - TooLong: a long command line is truncated
//   - TooLongMultibyte: truncation counts characters and cuts between them
func Test_selfDescription(t *testing.T) {
	previous := os.Args
	t.Cleanup(func() { os.Args = previous })

	// Verify that an invocation with no arguments names the tool.
	t.Run("NoArguments", func(t *testing.T) {
		os.Args = []string{"radiocli"}

		if got, want := selfDescription(), "radiocli"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that the command line is reported the way it was typed.
	t.Run("Arguments", func(t *testing.T) {
		os.Args = []string{"radiocli", "backup", "--verify"}

		if got, want := selfDescription(), "backup --verify"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that a line break cannot shift the fields after it.
	t.Run("Newlines", func(t *testing.T) {
		os.Args = []string{"radiocli", "favorites", "a\nb\rc"}

		if got, want := selfDescription(), "favorites a b c"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// Verify that a long command line cannot fill the lock file.
	t.Run("TooLong", func(t *testing.T) {
		os.Args = []string{"radiocli", strings.Repeat("x", maxDescription+10)}

		got := selfDescription()
		if got != strings.Repeat("x", maxDescription)+"..." {
			t.Fatalf("the command line was not truncated: %q", got)
		}
	})

	// Verify that a command line of multibyte characters is cut between
	// characters rather than through one, and that the limit counts characters
	// rather than the bytes they happen to take.
	t.Run("TooLongMultibyte", func(t *testing.T) {
		// Three bytes each, so a byte limit would cut this well short and land
		// in the middle of a character while doing it.
		os.Args = []string{"radiocli", strings.Repeat("あ", maxDescription+10)}

		got := selfDescription()
		if want := strings.Repeat("あ", maxDescription) + "..."; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("the truncation broke a character: %q", got)
		}
	})
}
