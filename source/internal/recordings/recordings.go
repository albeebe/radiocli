// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

// Package recordings files transmissions on disk: the audio, a description
// beside it, and a searchable index of everything recorded.
//
// # Why the metadata is a file of its own
//
// The obvious way to keep what a recording is of is to put it in the filename,
// and the software this feature is measured against does exactly that, with a
// printf-style template and three text fields written inside the audio file.
// The result is that anything wanting the frequency back has to parse a name it
// was never told the format of, which is why the one third-party tool that
// reads those recordings has the tag layout hardcoded and says so.
//
// A JSON object beside the audio needs no such agreement. It carries the fields
// under names, it can gain a field without breaking anything reading it, and it
// can say what is not known instead of leaving a blank that might mean either.
// The filename is then free to be for people, which is what a filename is good
// at.
//
// # Why there is an index as well
//
// A sidecar answers "what is this recording". It cannot answer "which
// recordings", and that is the question anybody actually has: every
// transmission on this talkgroup, everything from Tuesday evening, the longest
// call of the day. Without a listing, answering means opening every sidecar in
// every folder.
//
// So every recording is also appended to one file of newline-delimited JSON at
// the top of the destination. It is the format that can be appended to safely,
// that survives being cut off mid-write, and that ordinary tools already read,
// so the whole collection is searchable with a one-line jq expression and
// nothing to install. The alternative was a database, which would need a schema,
// a migration story and a program to read it.
//
// # Names
//
// A template decides the path, and its tokens are named and spelled out. It is
// checked when the library is opened rather than when a recording is written,
// because a mistyped token found at startup costs a second and the same typo
// found on the first transmission of the night costs the night. The default
// puts the date in a folder, which is not decoration: a scheme that puts the
// date only in the filename does not start a new folder at midnight, and the
// fix is a setting the user has to know exists.
package recordings

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
)

// ValidateTemplate reports whether a naming template can be used, without
// touching the disk.
//
// It exists so a caller can check the template before it opens anything else. A
// template that cannot work should fail while somebody is still watching, and
// it should fail without having created a destination directory for a run that
// is not going to happen.
//
// Parameters:
//   - naming: the naming template, or empty for DefaultTemplate
//
// Returns:
//   - error if a brace is unclosed, a token is unknown, or there are no tokens
//
// Errors:
//   - ErrBadTemplate: for every failure here
func ValidateTemplate(naming string) error {
	if naming == "" {
		return nil
	}
	_, err := parse(naming)
	return err
}

// New opens dir as a place to record into, creating it if it is not there.
//
// The template is checked here, before anything can be recorded, so a template
// that cannot work fails while somebody is still watching.
//
// Parameters:
//   - dir: the destination directory, created along with any parents
//   - naming: the naming template, or empty for DefaultTemplate
//
// Returns:
//   - *Library ready to record into, which the caller must Close
//   - error if the template is bad or the destination cannot be prepared
//
// Errors:
//   - ErrBadTemplate: if the template has an unclosed brace, an unknown token,
//     or no tokens at all
func New(dir, naming string) (*Library, error) {
	if naming == "" {
		naming = DefaultTemplate
	}
	name, err := parse(naming)
	if err != nil {
		return nil, err
	}

	if err := mkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("preparing %s to record into: %w", dir, err)
	}

	index, err := openIndex(filepath.Join(dir, IndexName))
	if err != nil {
		return nil, fmt.Errorf("opening the recording index: %w", err)
	}
	return &Library{dir: dir, name: name, index: index}, nil
}

// Begin opens a recording and returns it ready for audio.
//
// The audio goes to a hidden temporary name and is renamed when the recording
// is closed, because the name it will end up with cannot be known yet: it
// carries the channel and the duration, and neither is settled until the
// transmission has ended.
//
// Returns:
//   - *Recording to write audio into, which the caller must Close
//   - error if the file cannot be created
func (l *Library) Begin() (*Recording, error) {
	l.mu.Lock()
	l.partials++
	n := l.partials
	l.mu.Unlock()

	partial := filepath.Join(l.dir, partialPrefix+strconv.Itoa(n)+".wav")
	wav, err := createWav(partial)
	if err != nil {
		return nil, err
	}
	return &Recording{library: l, wav: wav, partial: partial}, nil
}

// Close finishes the library and stops writing to the index.
//
// Returns:
//   - error if the index cannot be closed
func (l *Library) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.index.Close(); err != nil {
		return fmt.Errorf("closing the recording index: %w", err)
	}
	return nil
}

// Dir reports the destination recordings are written into.
//
// Returns:
//   - the destination directory
func (l *Library) Dir() string { return l.dir }

// Abandon closes the audio and deletes it, for a recording that turned out not
// to be wanted.
//
// Returns:
//   - error if the file cannot be closed or removed
func (r *Recording) Abandon() error {
	if r.done {
		return nil
	}
	r.done = true

	if err := r.wav.Close(); err != nil {
		return err
	}
	if err := removeFile(r.partial); err != nil {
		return fmt.Errorf("removing an abandoned recording: %w", err)
	}
	return nil
}

// Close finishes the recording, files it under the name e describes, and
// writes its sidecar and index line.
//
// The duration in the entry is taken from the audio rather than from the times
// in it. They differ whenever frames were lost, and the file's own length is
// the honest answer for something being catalogued by how long it is.
//
// Parameters:
//   - e: what the recording is of, with File and Duration filled in here
//
// Returns:
//   - the entry as it was filed, carrying the path it ended up at
//   - error if the audio cannot be closed, the folders cannot be made, the
//     recording cannot be moved into place, or either the sidecar or the index
//     cannot be written
func (r *Recording) Close(e Entry) (Entry, error) {
	if r.done {
		return e, nil
	}
	r.done = true

	e.Duration = r.wav.Duration().Seconds()
	if err := r.wav.Close(); err != nil {
		return e, err
	}

	rel, err := r.library.reserve(e)
	if err != nil {
		return e, err
	}
	e.File = rel + ".wav"

	if err := renameFile(r.partial, filepath.Join(r.library.dir, e.File)); err != nil {
		return e, fmt.Errorf("filing the recording as %s: %w", e.File, err)
	}

	sidecar, err := marshalIndent(e, "", "  ")
	if err != nil {
		// Every field is a string, a number or a time, so this cannot happen
		// with the type as it stands. It is reported rather than ignored
		// because a field added later could change that.
		return e, fmt.Errorf("describing the recording %s: %w", e.File, err)
	}
	if err := writeFile(filepath.Join(r.library.dir, rel+".json"), append(sidecar, '\n'), 0o644); err != nil {
		return e, fmt.Errorf("writing the description of %s: %w", e.File, err)
	}

	if err := r.library.append(e); err != nil {
		return e, err
	}
	return e, nil
}

// Write appends audio to the recording.
//
// Parameters:
//   - pcm: signed 16-bit little-endian mono samples at 48 kHz
//
// Returns:
//   - error if the audio cannot be written
func (r *Recording) Write(pcm []byte) error {
	return r.wav.Write(pcm)
}

// append adds one line to the index.
//
// Parameters:
//   - e: the recording to record
//
// Returns:
//   - error if the line cannot be written
func (l *Library) append(e Entry) error {
	line, err := marshal(e)
	if err != nil {
		return fmt.Errorf("describing %s for the index: %w", e.File, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.index.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("adding %s to the recording index: %w", e.File, err)
	}
	return nil
}

// reserve works out where a recording goes and makes sure the name is free.
//
// Two transmissions can land on one name: a template with no time in it, or two
// calls on the same channel in the same second. Overwriting the first is the
// behaviour to avoid, so a number is added instead, and the name is only
// considered taken if either the audio or its sidecar is already there.
//
// Parameters:
//   - e: the recording to name
//
// Returns:
//   - the path below the destination, with no extension
//   - error if the folders it asks for cannot be made
func (l *Library) reserve(e Entry) (string, error) {
	base := l.name.render(e)

	dir := filepath.Dir(filepath.Join(l.dir, base))
	if err := mkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("preparing %s to record into: %w", dir, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Bounded, because free reports a name as taken for any reason it cannot
	// see it, a directory it may not read included. Without a limit that
	// becomes a loop that never ends and never says why.
	for n := 2; n < maxCollisions; n++ {
		if free(filepath.Join(l.dir, base)) {
			return base, nil
		}
		base = l.name.render(e) + "-" + strconv.Itoa(n)
	}
	return "", fmt.Errorf("cannot find a free name for a recording near %s: "+
		"check the folder is writable", filepath.Join(l.dir, base))
}

// free reports whether a name is available for a recording.
//
// Parameters:
//   - base: the full path below which the two files would be written, with no
//     extension
//
// Returns:
//   - true if neither the audio nor its sidecar is already there
func free(base string) bool {
	for _, ext := range []string{".wav", ".json"} {
		if _, err := statFile(base + ext); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

// Sweep reports the recordings left behind by a run that was killed part way
// through a transmission.
//
// They are reported rather than deleted, because deleting somebody's files
// without being asked is not this package's decision to make. Each one is a WAV
// whose header was never completed, so nothing will play it.
//
// Returns:
//   - the paths of any partial recordings found
//   - error if the destination cannot be read
func (l *Library) Sweep() ([]string, error) {
	entries, err := readDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("looking through %s: %w", l.dir, err)
	}

	var found []string
	for _, entry := range entries {
		if name := entry.Name(); !entry.IsDir() && len(name) > len(partialPrefix) &&
			name[:len(partialPrefix)] == partialPrefix {
			found = append(found, filepath.Join(l.dir, name))
		}
	}
	return found, nil
}
