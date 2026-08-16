// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// checksumFor finds one file's SHA-256 in a checksums file.
//
// The file is the two-column output of sha256sum, which is what the release
// workflow writes and what "sha256sum -c" reads back. Reading it tolerantly
// costs nothing and means a file produced on Windows, or by a tool that marks
// binary files with a star, still works: carriage returns are dropped, comments
// and blank lines are skipped, and any run of whitespace separates the columns.
//
// Parameters:
//   - data: the checksums file as published
//   - name: the file whose checksum is wanted
//
// Returns:
//   - the checksum as hex, and an error when the file lists no such name
func checksumFor(data []byte, name string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("%s lists no checksum for %s, so the download cannot be "+
		"checked: nothing was installed", checksumsAsset, name)
}

// download streams a release file to a staging file beside the running program,
// hashing it on the way past.
//
// The staging file goes in the program's own directory rather than a temporary
// one so that the rename which installs it is a move within one filesystem,
// which is what makes that step atomic. Staging in /tmp would work until it
// did not: on most Linux systems /tmp is a separate filesystem, and the rename
// out of it fails.
//
// The hash is computed while the bytes go past rather than by reading the file
// again afterwards, so there is no window in which the file on disk and the
// file that was checked could differ.
//
// Parameters:
//   - ctx: cancels the download, which is what Ctrl-C reaches
//   - from: the release file to fetch, whose size is checked against what
//     arrives
//   - dir: the directory to stage in, which is the one holding the program
//
// Returns:
//   - the staging file's path, its SHA-256 as hex, how many bytes were written,
//     and an error. The path is returned even when the download failed, so the
//     caller's cleanup can remove a partial file.
func download(ctx context.Context, from releaseAsset, dir string) (string, string, int64, error) {
	f, err := createTemp(dir, tempPattern)
	if err != nil {
		return "", "", 0, fmt.Errorf("staging a download in %s: %w", dir, err)
	}
	path := f.Name()

	resp, err := get(ctx, from.URL)
	if err != nil {
		f.Close()
		return path, "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.Close()
		return path, "", 0, statusError(resp, from.URL)
	}

	// One byte past the cap, so a file that is exactly at it still arrives and
	// anything beyond it is caught rather than truncated into a valid-looking
	// download.
	sum := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, sum), io.LimitReader(resp.Body, maxArchiveBytes+1))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return path, "", written, fmt.Errorf("downloading %s: %w", from.Name, err)
	}

	if written > maxArchiveBytes {
		return path, "", written, fmt.Errorf("%s is larger than %d bytes, which no "+
			"radiocli release is: nothing was installed", from.Name, maxArchiveBytes)
	}
	if from.Size > 0 && written != from.Size {
		return path, "", written, fmt.Errorf("%s arrived as %d bytes but was published "+
			"as %d: the download did not finish, so nothing was installed",
			from.Name, written, from.Size)
	}
	return path, hex.EncodeToString(sum.Sum(nil)), written, nil
}

// fetchChecksums downloads the file listing what every asset in a release
// should hash to.
//
// A release without one is refused rather than installed unchecked. That is the
// whole point of the file: an update that falls back to trusting the download
// when the checksums are missing is an update with no guarantee at all, because
// missing is exactly what an attacker would arrange.
//
// Parameters:
//   - ctx: cancels the request
//   - rel: the release to read the checksums of
//
// Returns:
//   - the checksums file, and an error when the release publishes none or it
//     could not be fetched
func fetchChecksums(ctx context.Context, rel release) ([]byte, error) {
	a, err := findAsset(rel, checksumsAsset)
	if err != nil {
		return nil, fmt.Errorf("release %s publishes no %s, so its downloads cannot be "+
			"checked: nothing was installed\n\n"+
			"Releases from before this check existed have had one added. If this is a "+
			"new release, the file may still be uploading.", rel.TagName, checksumsAsset)
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := get(ctx, a.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp, a.URL)
	}

	data, err := readCapped(resp.Body, maxChecksumsBytes)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", checksumsAsset, err)
	}
	return data, nil
}

// readCapped reads everything up to a limit, and reports going past it rather
// than quietly returning a truncated result.
//
// Parameters:
//   - r: what to read
//   - limit: the most that may be read
//
// Returns:
//   - what was read, and an error when reading failed or there was more than
//     the limit allows
func readCapped(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("it is larger than %d bytes, which is not the file this "+
			"expects", limit)
	}
	return data, nil
}

// verify checks a downloaded file against what the release says it should be.
//
// Two opinions are compared where both exist. The checksums file is the one
// that matters and the one a person can check by hand, but GitHub also records
// a hash of its own when a file is uploaded, and comparing that as well catches
// the case the checksums file cannot catch by itself: a checksums file that was
// generated from a different build than the one published beside it.
//
// Parameters:
//   - a: the asset that was downloaded, named as the checksums file names it
//   - from: the published file, which may carry GitHub's own hash
//   - sums: the checksums file
//   - digest: what the download actually hashed to
//
// Returns:
//   - error when the file does not match, naming both hashes
func verify(a asset, from releaseAsset, sums []byte, digest string) error {
	want, err := checksumFor(sums, a.archive)
	if err != nil {
		return err
	}

	// Compared plainly rather than in constant time. Both values are public:
	// one was published on a release page and the other was computed from a
	// file anybody can download. There is no secret here for a timing
	// difference to leak, and reaching for the constant-time call would only
	// suggest to a later reader that there is one.
	if !strings.EqualFold(want, digest) {
		return fmt.Errorf("%s does not match the checksum published with it, so nothing "+
			"was installed: got %s, want %s\n\n"+
			"Either the download was corrupted, in which case running this again will "+
			"fix it, or the file is not the one that was released.", a.archive, digest, want)
	}

	if github := strings.TrimPrefix(from.Digest, "sha256:"); github != "" {
		if !strings.EqualFold(github, digest) {
			return fmt.Errorf("%s matches %s but not the hash GitHub recorded when it was "+
				"uploaded, so nothing was installed: got %s, GitHub has %s\n\n"+
				"The release is inconsistent with itself. Report it rather than working "+
				"around it.", a.archive, checksumsAsset, digest, github)
		}
	}
	return nil
}
