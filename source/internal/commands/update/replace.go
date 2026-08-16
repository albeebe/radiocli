// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"fmt"
	"io/fs"
)

// executable finds the file this process is running from, following any link
// to the file behind it.
//
// Resolving the link is the point. An install put somewhere convenient is often
// a link into wherever the file really lives, and replacing the link with a
// program would leave the real file untouched and the link no longer a link.
// Following it also means the path this reports is the file that actually
// changed, which is what tells somebody their install came from a package
// manager.
//
// Returns:
//   - the path of the running program, and an error only when the process
//     cannot say where it is
func executable() (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("finding where this program is installed: %w", err)
	}

	// A failure here is not worth stopping for. On Linux a program that has
	// already been replaced under a running process reports a path with
	// "(deleted)" on the end, which resolves to nothing; the unresolved path is
	// still the right file to write.
	resolved, err := evalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

// modeOf reads the permissions to give the new program.
//
// They are copied from the program being replaced rather than set to a fixed
// value, because an install that was deliberately made read-only, or made group
// executable and nothing else, should stay that way. Writing a fixed 0755 over
// it would quietly loosen it.
//
// Parameters:
//   - path: the program being replaced
//
// Returns:
//   - its permission bits, or 0755 when they cannot be read
func modeOf(path string) fs.FileMode {
	info, err := statFile(path)
	if err != nil {
		return 0o755
	}
	return info.Mode().Perm()
}

// replaceOn puts the new program in the place of the running one.
//
// It takes the platform as a parameter rather than reading it, so that both
// paths can be exercised anywhere. Putting the Windows path in a file that only
// compiles on Windows would leave it out of the coverage measured here
// entirely, which would turn a hundred percent into a statement about this
// machine rather than about the code.
//
// On Unix this is one rename. The rename is atomic, so there is no instant at
// which the program is missing or half written, and it works on a running
// program: the process keeps the file it started from, while the name now
// points at the new one. Writing over the file in place would instead fail with
// "text file busy".
//
// Windows cannot overwrite the image of a running process, but it can rename
// it. So the running program is moved aside first, the new one takes its name,
// and the one moved aside is deleted if it can be. If it cannot, because
// something has it open, a later run sweeps it up.
//
// Parameters:
//   - goos: the operating system, as Go names it
//   - target: the program to replace
//   - staged: the new program, in the same directory
//
// Returns:
//   - error when the replacement did not happen, in which case the running
//     program is left as it was
func replaceOn(goos, target, staged string) error {
	if goos != "windows" {
		if err := renameFile(staged, target); err != nil {
			return fmt.Errorf("putting the new program in place of %s: %w", target, err)
		}
		return nil
	}

	old := target + oldSuffix
	_ = removeFile(old)

	if err := renameFile(target, old); err != nil {
		return fmt.Errorf("moving %s aside: %w", target, err)
	}
	if err := renameFile(staged, target); err != nil {
		// Put the working program back. Without this a failure here would
		// leave the machine with no radiocli at all, which is a far worse
		// outcome than not updating.
		_ = renameFile(old, target)
		return fmt.Errorf("putting the new program in place of %s: %w", target, err)
	}

	// Usually gone at once. It survives only when something else has the file
	// open, and then it is untidy rather than broken.
	_ = removeFile(old)
	return nil
}

// sweep removes the copy of the program a previous Windows update moved aside.
//
// Every failure is ignored on purpose. A leftover file that is still locked is
// cosmetic, and failing an update over it would be worse than leaving it.
//
// Parameters:
//   - goos: the operating system, as Go names it
//   - target: the program being run, whose moved-aside copy sits beside it
func sweep(goos, target string) {
	if goos != "windows" {
		return
	}
	_ = removeFile(target + oldSuffix)
}

// writable reports whether the program's own directory can be written to.
//
// This runs before anything is downloaded, so somebody who needs to run the
// command again with sudo is told immediately rather than after waiting for
// several megabytes to arrive.
//
// It answers by creating a file and removing it again, which is the only test
// that is actually true. Reading the permission bits says nothing useful: an
// access list can grant or refuse beyond what they show, and a network share
// that maps root to nobody will happily report a directory as writable that
// is not.
//
// Parameters:
//   - dir: the directory holding the program
//
// Returns:
//   - error wrapping errNotWritable when a file cannot be created there
func writable(dir string) error {
	probe, err := createTemp(dir, tempPattern)
	if err != nil {
		return fmt.Errorf("%w: %s", errNotWritable, dir)
	}

	name := probe.Name()
	probe.Close()
	_ = removeFile(name)
	return nil
}
