// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package portlock keeps two invocations of the tool off the same scanner.
//
// The scanner speaks a request and response protocol over a single serial
// line, and nothing in a reply says which process asked for it. Two
// invocations talking at once therefore read each other's answers. Measured on
// an SDS150: a "scanning" and a "battery" run started together both failed,
// one on a response that stopped halfway through and the other on no response
// at all, and both reported it as the scanner being unreachable.
//
// Two failed reads is the harmless case. The commands that walk menus press a
// key, read the screen, and choose the next key from what they saw, so a
// second process pressing keys in the middle of one leaves the scanner
// somewhere neither of them meant to put it: an entry screen holding half a
// name, with nothing reported as having gone wrong. Preventing that is why
// this exists.
//
// The claim therefore covers a whole invocation rather than one exchange,
// because what has to be indivisible is the menu walk, not the request.
//
// # What it does not cover
//
// The lock is advisory and lives in a file, so it only holds between programs
// that take it, which means this tool and nothing else. Scanner programming
// software and serial terminals know nothing about it and can still interleave
// with a command that is running.
//
// The file sits in the temporary directory, which on macOS and Windows is per
// user. Two people signed in to the same machine driving one scanner will not
// see each other's locks. That is not worth solving here: the case this is for
// is one person's scripts running at once.
//
// The operating system releases the lock when the process ends, however it
// ends, so a command that is killed leaves nothing behind to clean up and no
// stale lock to break. That is the reason for a file lock rather than a file
// holding a process ID.
//
// # Why the lock files are never deleted
//
// Release unlocks and closes the file but leaves it where it is, so the
// directory accumulates one empty file per serial port ever used. That is
// deliberate. A lock is held on an open file rather than on a path, so a holder
// that deleted its file on the way out would leave the next caller to create a
// fresh file at the same path and lock that instead: two processes each holding
// a real lock, on two different files, believing they have the same scanner. The
// races that come from unlinking a lock file are worse than the few empty files
// avoided by unlinking it.
//
// It does leave one window this tool cannot close. If something outside the tool
// deletes a lock file while a command holds it, which the periodic cleanup of
// the temporary directory will eventually do to a file left alone for a few
// days, the next caller makes a new file at that path and locks it, and both are
// then talking to one scanner. It needs a lock held across the days that cleanup
// waits for, which for a tool whose invocations last seconds means a daemon left
// running over a long weekend. Nothing here detects it; it is recorded so that
// the next person to read this code knows it was considered rather than missed.
package portlock

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Acquire claims port for this process, and describes the claim in the lock
// file so that whoever is refused can be told who has it.
//
// wait is how long to keep trying before giving up. Zero gives up at once,
// which is the right default: a menu walk types names one character at a time
// and can run for minutes, and a command that silently blocks that long is
// indistinguishable from one that has hung.
//
// Parameters:
//   - port: serial port whose scanner this invocation is claiming
//   - wait: how long to keep retrying before giving up; zero gives up at once
//
// Returns:
//   - *Lock representing the held claim, to be given back with Release
//   - error if the lock directory or file cannot be created, the lock attempt
//     fails for a reason other than contention, or the port stays busy past wait
//
// Errors:
//   - ErrBusy: wrapped into the returned error when another invocation still
//     holds the port after wait has elapsed
func Acquire(port string, wait time.Duration) (*Lock, error) {
	path := lockPath(port)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	// Opened read-write because the holder describes itself in here and
	// whoever is refused reads that description back out.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	deadline := time.Now().Add(wait)
	for {
		err := takeLock(f)
		if err == nil {
			l := &Lock{file: f, path: path}
			l.describe()
			return l, nil
		}
		if !errors.Is(err, errWouldBlock) {
			f.Close()
			return nil, fmt.Errorf("locking %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			held := readHolder(f)
			f.Close()
			return nil, fmt.Errorf("%s is %w%s. Wait for it to finish, "+
				"or pass --wait to queue behind it", port, ErrBusy, held)
		}
		time.Sleep(retryInterval)
	}
}

// Release gives the port up. Closing the file would release the lock on its
// own, but unlocking first keeps the two steps in an order that reads the way
// it happens.
//
// Returns:
//   - error if unlocking or closing the lock file fails; nil on a nil or
//     already released Lock
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	unlockErr := releaseLock(l.file)
	closeErr := l.file.Close()
	l.file = nil

	if unlockErr != nil {
		return fmt.Errorf("unlocking %s: %w", l.path, unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing %s: %w", l.path, closeErr)
	}
	return nil
}

// SocketDir is where SocketPath puts its sockets, exported so the daemon can
// create it before listening. Acquire already creates the directory above it
// for the lock file, but the daemon listens before anything has taken a lock.
//
// The sockets live one level below the lock files, in a directory of this
// account's own, and the two cannot share one. A lock file has to be readable
// by every account on the machine, because the whole point of it is that two
// people cannot drive one scanner at once, and the second one is owed the name
// of the process holding it. A socket is the opposite: it carries every command
// this tool can run, so nobody but the account that started the daemon has any
// business reaching it. Splitting them lets the daemon's directory be created
// private and stay private, which closes the gap between binding the socket and
// restricting it, during which the socket is otherwise there for anybody to
// connect to.
//
// Returns:
//   - string path of the directory holding this account's daemon sockets
func SocketDir() string {
	return filepath.Join(tempDir(), "radiocli", socketsFor(os.Getuid()))
}

// SocketPath returns the daemon socket for a port, in this account's socket
// directory and keyed off the port the same way the lock file is.
//
// It lives here rather than with the daemon because the lock and the socket
// have to agree on what "the same scanner" means. A client that fails to take
// the lock goes looking for the socket belonging to the port it was refused,
// and a disagreement between the two schemes would mean it never found one.
//
// Unlike the lock file, the name is the digest alone with no readable part.
// A unix socket path has to fit in sun_path, which is 104 bytes on macOS, and
// the readable form of a typical port already reaches 102: two characters of
// headroom is not a margin, it is a bug waiting for a longer port name. The
// digest is twice as long as the lock file's to make up for the readable part
// no longer distinguishing anything.
//
// The account's own directory in the middle costs a dozen of those bytes and
// buys the socket a private place to be created in. On macOS, where the
// temporary directory is itself per-account and long, that leaves a typical
// path around ninety bytes, which is inside the limit with room to spare.
//
// Parameters:
//   - port: serial port whose daemon socket is being located
//
// Returns:
//   - string path of the socket for port
func SocketPath(port string) string {
	sum := sha256.Sum256([]byte(port))
	name := hex.EncodeToString(sum[:8]) + ".sock"
	return filepath.Join(SocketDir(), name)
}

// describe records who holds the lock, as three lines: process ID, the command
// line, and when it started.
//
// The truncate comes first on purpose. Between taking the lock and finishing
// this write there is a moment when the file still holds the previous holder's
// details, and reporting those would name a process that finished long ago.
// Emptying it first means anyone reading during that moment sees nothing and
// is told only that the port is busy, which is true, rather than something
// false and specific.
//
// Failures here are ignored. Not being able to say who holds the lock is worth
// nothing next to holding it, and there is nobody to report the failure to.
func (l *Lock) describe() {
	if err := l.file.Truncate(0); err != nil {
		return
	}
	if _, err := seekFile(l.file, 0, io.SeekStart); err != nil {
		return
	}

	fmt.Fprintf(l.file, "%d\n%s\n%s\n",
		os.Getpid(), selfDescription(), time.Now().Format(time.RFC3339Nano))
	l.file.Sync()
}

// lockPath returns the lock file for a port.
//
// The name carries both a readable form of the port and a digest of it. The
// readable part is for whoever goes looking in the directory; the digest is
// because the readable part is lossy, and two different ports that differ only
// in a character the sanitiser rewrites would otherwise share a lock and block
// each other for no reason.
//
// Parameters:
//   - port: serial port whose lock file is being located
//
// Returns:
//   - string path of the lock file for port
func lockPath(port string) string {
	sum := sha256.Sum256([]byte(port))
	name := fmt.Sprintf("%s-%s.lock", sanitize(port), hex.EncodeToString(sum[:4]))
	return filepath.Join(tempDir(), "radiocli", name)
}

// readHolder renders what the lock file says about the process holding it, as
// a fragment to append to the busy error, or an empty string if it does not
// say anything usable.
//
// Everything here degrades to an empty string rather than to an error. The
// file is written by another process and can legitimately be caught empty or
// half written, and none of that changes the answer the caller gets, which is
// that the port is busy.
//
// Parameters:
//   - f: open lock file written by the process holding the lock
//
// Returns:
//   - string fragment naming the holder, or "" when the file cannot be read
//     or does not parse
func readHolder(f *os.File) string {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 3 {
		return ""
	}

	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return ""
	}

	held := ""
	if started, err := time.Parse(time.RFC3339Nano, lines[2]); err == nil {
		held = fmt.Sprintf(" for %s", time.Since(started).Round(time.Second))
	}

	return fmt.Sprintf(": pid %d running %q%s", pid, lines[1], held)
}

// sanitize turns a port into something that can be a file name on any of the
// systems this runs on.
//
// Parameters:
//   - port: serial port name to rewrite
//
// Returns:
//   - string with every character outside the portable set replaced by a dash
//     and any leading or trailing slashes removed
func sanitize(port string) string {
	keep := func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return true
		case r == '.', r == '_', r == '-':
			return true
		}
		return false
	}

	var b strings.Builder
	for _, r := range strings.Trim(port, "/\\") {
		if keep(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

// selfDescription names this invocation the way its user typed it, so the
// process holding the port is recognizable rather than just a number.
//
// Newlines are replaced because the lock file is read back a line at a time,
// and an argument containing one would otherwise shift every field after it.
//
// The limit counts characters rather than bytes. A name typed in any language
// but English reaches a byte limit early and, worse, is cut in the middle of a
// character, which leaves the holder described by a line ending in a broken
// one. Counting characters costs a pass over a string of at most a few hundred
// bytes, once, on a path that already writes a file.
//
// Returns:
//   - string holding the command line as typed, truncated to maxDescription
//     characters, or the bare tool name when there were no arguments
func selfDescription() string {
	if len(os.Args) < 2 {
		return "radiocli"
	}

	s := strings.Join(os.Args[1:], " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if utf8.RuneCountInString(s) > maxDescription {
		s = string([]rune(s)[:maxDescription]) + "..."
	}
	return s
}

// socketsFor names the directory an account's daemon sockets live in.
//
// The account is in the name rather than the permissions alone because two
// accounts sharing a machine each need a directory of their own to create
// sockets in, and neither may create one inside the other's. An account this
// system does not number, which is Windows, gets one directory shared by
// whoever is signed in; the permissions there are the ones the filesystem
// gives it, and no unix daemon socket is created on that platform anyway.
//
// Parameters:
//   - uid: the account's numeric id, or -1 on a system that does not number
//     them
//
// Returns:
//   - string naming the directory, without any path in front of it
func socketsFor(uid int) string {
	if uid < 0 {
		return "sockets"
	}
	return fmt.Sprintf("sockets-%d", uid)
}
