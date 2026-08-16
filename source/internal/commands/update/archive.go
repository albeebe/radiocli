// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// extract takes the one program out of a release archive and writes it beside
// the running one.
//
// Which kind of archive it is follows from its name rather than from the
// platform, so the mapping from platform to file stays in one place instead of
// being half here.
//
// Parameters:
//   - archive: the downloaded file
//   - want: the asset, naming the single entry to take out of it
//   - dir: the directory to write the program into
//   - mode: the permissions to give it
//
// Returns:
//   - the path of the extracted program, which is returned even on failure so
//     the caller's cleanup can remove a partial file, and an error
func extract(archive string, want asset, dir string, mode fs.FileMode) (string, error) {
	if strings.HasSuffix(want.archive, ".zip") {
		return extractZip(archive, want, dir, mode)
	}
	return extractTarGz(archive, want, dir, mode)
}

// extractTarGz reads the program out of a gzipped tar.
//
// Parameters:
//   - archive: the downloaded file
//   - want: the asset, naming the single entry to take out of it
//   - dir: the directory to write the program into
//   - mode: the permissions to give it
//
// Returns:
//   - the path of the extracted program, and an error
func extractTarGz(archive string, want asset, dir string, mode fs.FileMode) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", want.archive, err)
	}
	defer f.Close()

	unzipped, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", want.archive, err)
	}
	defer unzipped.Close()

	tarball := tar.NewReader(unzipped)
	for {
		header, err := tarball.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", want.archive, err)
		}

		if err := safeName(want.archive, header.Name); err != nil {
			return "", err
		}
		if header.Name != want.binary || header.Typeflag != tar.TypeReg {
			continue
		}
		return stage(tarball, dir, mode)
	}
	return "", missingEntry(want)
}

// extractZip reads the program out of a zip.
//
// Parameters:
//   - archive: the downloaded file
//   - want: the asset, naming the single entry to take out of it
//   - dir: the directory to write the program into
//   - mode: the permissions to give it
//
// Returns:
//   - the path of the extracted program, and an error
func extractZip(archive string, want asset, dir string, mode fs.FileMode) (string, error) {
	r, err := openZip(archive)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", want.archive, err)
	}
	defer r.Close()

	for _, entry := range r.File {
		if err := safeName(want.archive, entry.Name); err != nil {
			return "", err
		}
		if entry.Name != want.binary || !entry.Mode().IsRegular() {
			continue
		}

		body, err := entry.Open()
		if err != nil {
			return "", fmt.Errorf("reading %s from %s: %w", entry.Name, want.archive, err)
		}
		defer body.Close()

		return stage(body, dir, mode)
	}
	return "", missingEntry(want)
}

// missingEntry reports an archive that does not hold the program.
//
// Parameters:
//   - want: the asset that was expected to hold it
//
// Returns:
//   - an error naming both the archive and what was looked for
func missingEntry(want asset) error {
	return fmt.Errorf("%s does not contain %s, so it is not the release archive this "+
		"expects: nothing was installed", want.archive, want.binary)
}

// safeName refuses an archive entry whose name is anything other than a plain
// file name.
//
// Every entry is checked, including the ones that are skipped afterwards. The
// name is never joined onto a path here, so nothing could escape the directory
// even without this, but that is a property of how the extraction happens to be
// written today rather than a rule anybody stated. The archive arrives over the
// network, and "it is one of ours" is precisely the assumption a compromised
// release breaks.
//
// Parameters:
//   - archive: the archive being read, for the message
//   - name: the entry name as the archive gives it
//
// Returns:
//   - error when the name is a path rather than a file name
func safeName(archive, name string) error {
	if name == filepath.Base(name) && name != "." && name != ".." &&
		!strings.ContainsRune(name, '\\') && !filepath.IsAbs(name) {
		return nil
	}
	return fmt.Errorf("%s contains an entry named %q, which is a path rather than a "+
		"file name: nothing was extracted", archive, name)
}

// stage writes the program out to a file beside the running one.
//
// The copy is capped rather than sized from what the archive claims. A zip
// records its own uncompressed size, and that number comes from the archive
// itself, so a small file can honestly claim to expand to something enormous.
// Counting the bytes as they are written is the only figure that is real.
//
// Parameters:
//   - r: the entry's contents
//   - dir: the directory to write into
//   - mode: the permissions to give the file
//
// Returns:
//   - the path written to, which is returned even on failure so the caller's
//     cleanup can remove a partial file, and an error
func stage(r io.Reader, dir string, mode fs.FileMode) (string, error) {
	f, err := createTemp(dir, tempPattern)
	if err != nil {
		return "", fmt.Errorf("staging the new program in %s: %w", dir, err)
	}
	path := f.Name()

	written, err := io.Copy(f, io.LimitReader(r, maxBinaryBytes+1))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return path, fmt.Errorf("writing the new program: %w", err)
	}
	if written > maxBinaryBytes {
		return path, fmt.Errorf("the program in the archive expands past %d bytes, which "+
			"no radiocli build does: nothing was installed", maxBinaryBytes)
	}

	if err := chmodFile(path, mode); err != nil {
		return path, fmt.Errorf("setting the permissions on the new program: %w", err)
	}
	return path, nil
}
