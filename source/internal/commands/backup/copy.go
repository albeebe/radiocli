// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// build walks the card and works out what to copy.
//
// Parameters:
//   - ctx: context for cancellation while walking the card
//   - src: the scanner's directory on the card
//   - includeDatabase: if true, includes the downloaded radio database, which
//     is most of the card
//
// Returns:
//   - plan naming every file and directory to copy, in a stable order, with
//     the total size and how much was left behind
//   - error if the card cannot be walked or the context is cancelled
func build(ctx context.Context, src string, includeDatabase bool) (plan, error) {
	var p plan

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := relPath(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		name := d.Name()
		if hidden(name) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if !includeDatabase && rel == databaseDir {
				return filepath.SkipDir
			}
			p.dirs = append(p.dirs, rel)
			return nil
		}

		// Anything that is not a regular file is not the scanner's. The card
		// is FAT32 and holds no links or devices, so this only triggers on
		// something the host put there.
		if !d.Type().IsRegular() {
			p.skipped++
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		p.files = append(p.files, file{rel: rel, size: info.Size()})
		p.bytes += info.Size()
		return nil
	})
	if err != nil {
		return plan{}, fmt.Errorf("reading the card: %w", err)
	}

	// Copy in a stable order so two backups of an unchanged card produce the
	// same log, which makes them comparable.
	sort.Slice(p.files, func(i, j int) bool { return p.files[i].rel < p.files[j].rel })
	sort.Strings(p.dirs)
	return p, nil
}

// copyFile copies one file, returning the source digest when verifying.
//
// Parameters:
//   - from: the file on the card to read
//   - to: the path to write it to
//   - verify: if true, reads the copy back and compares it against the card
//
// Returns:
//   - string holding the source digest as hex when verifying, and empty when
//     not
//   - error if the file cannot be read, written or closed, or if the copy does
//     not match the card
func copyFile(from, to string, verify bool) (string, error) {
	in, err := os.Open(from)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", from, err)
	}
	defer in.Close()

	out, err := createFile(to)
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", to, err)
	}

	var (
		reader io.Reader = in
		sum    hash.Hash
	)
	if verify {
		sum = sha256.New()
		reader = io.TeeReader(in, sum)
	}

	if _, err := io.Copy(out, reader); err != nil {
		out.Close()
		return "", fmt.Errorf("copying %s: %w", from, err)
	}

	// Close before verifying, so what gets read back is what reached the disk
	// rather than what is still buffered.
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("finishing %s: %w", to, err)
	}
	if !verify {
		return "", nil
	}

	want := hex.EncodeToString(sum.Sum(nil))
	got, err := digestOf(to)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf("%s did not copy correctly: the card and the copy differ", from)
	}
	return want, nil
}

// digestOf hashes a file that has already been written.
//
// Parameters:
//   - path: the file to hash
//
// Returns:
//   - string holding the file's SHA-256 digest as hex
//   - error if the file cannot be opened or read
func digestOf(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("checking %s: %w", path, err)
	}
	defer f.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", fmt.Errorf("checking %s: %w", path, err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// hidden reports whether a path component is one the operating system added
// rather than the scanner.
//
// macOS writes Spotlight and FSEvents directories onto any volume it mounts,
// and Windows writes its own. None of it belongs to the scanner, none of it is
// wanted in a backup, and the Spotlight index alone can dwarf the card's real
// contents.
//
// Parameters:
//   - name: one component of a path on the card
//
// Returns:
//   - bool reporting whether the component belongs to the operating system
//     rather than the scanner
func hidden(name string) bool {
	switch {
	case strings.HasPrefix(name, "."):
		return true
	case name == "System Volume Information":
		return true
	default:
		return false
	}
}

// human renders a byte count the way a person reads one.
//
// Parameters:
//   - n: a count of bytes
//
// Returns:
//   - string holding the count in the largest unit it fills, to one decimal
//     place, and in bytes when it is smaller than a kilobyte
func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// run copies the plan into dst, verifying as it goes when asked.
//
// Verification is done by hashing the card's copy of a file as it is read, then
// closing what was written and reading it back off the disk to hash that too,
// and comparing the two. The read-back is a second pass over the copy and is
// meant to be: hashing the bytes on their way out would prove only that the
// program wrote what it read, while reading them back afterwards proves the
// disk holds them. The card itself is still read once, which is the pass worth
// saving, since that is the one going over USB.
//
// Parameters:
//   - ctx: context for cancellation between files
//   - src: the scanner's directory on the card
//   - dst: the folder the backup is written into
//   - verify: if true, reads every file back and compares it against the card
//   - progress: called before each file is copied, or nil to report nothing
//
// Returns:
//   - []result naming every file copied and its size, carrying digests when
//     verifying, and holding what was copied so far when the copy fails
//   - error if a directory or a file cannot be created, read or written, if a
//     copy does not match the card, or if the context is cancelled
func (p plan) run(ctx context.Context, src, dst string, verify bool, progress func(file)) ([]result, error) {
	results := make([]result, 0, len(p.files))

	// Recreate the directory tree first, so the ones holding no files still
	// exist in the copy.
	for _, rel := range p.dirs {
		if err := os.MkdirAll(filepath.Join(dst, rel), 0o755); err != nil {
			return results, fmt.Errorf("creating %s: %w", rel, err)
		}
	}

	for _, f := range p.files {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		if progress != nil {
			progress(f)
		}

		from := filepath.Join(src, f.rel)
		to := filepath.Join(dst, f.rel)

		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return results, fmt.Errorf("creating %s: %w", filepath.Dir(to), err)
		}

		digest, err := copyFile(from, to, verify)
		if err != nil {
			return results, err
		}
		results = append(results, result{Path: f.rel, Bytes: f.size, Digest: digest})
	}

	return results, nil
}
