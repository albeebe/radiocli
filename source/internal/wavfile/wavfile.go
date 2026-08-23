// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

// Package wavfile writes one transmission of scanner audio to a WAV file.
//
// It exists because nothing else in this tool turns audio into something that
// plays. The audio path ends at raw samples: audiofeed hands out 20 ms frames
// of signed 16-bit mono, and opusenc turns those into bare Opus packets with no
// container around them, which the audio command's own documentation admits
// nothing that plays files will open. A recording has to survive being
// double-clicked years later by somebody who has never heard of this tool, and
// that rules out both.
//
// # Why WAV, and why only WAV
//
// A recording is an archive, and the two things an archive has to be are
// playable anywhere and unchanged from what arrived. WAV is both. It has no
// codec to go out of date, the samples in the file are the samples the sound
// card produced, and the header is 44 bytes of arithmetic rather than a
// dependency.
//
// Opus was the obvious alternative and is deliberately not offered. The encoder
// this tool carries is a pure Go port pinned to one commit of a port that was
// still being written, whose output quality can move between commits even when
// its API does not, and it cannot decode. Compressing an archive with it would
// mean recordings whose fidelity depends on which build made them, and no way
// to read them back. That trade makes sense for a live stream, which is what
// opusenc was built for and where a listener on a phone is the constraint. It
// makes no sense for the copy being kept.
//
// The cost is size: an hour of audio is about 330 MB, against roughly 15 MB of
// Opus. That is the right way round for something recording a channel that is
// silent most of the time, because the recorder only ever writes the
// transmissions, and the silence between them costs nothing at all.
//
// # What it does not do
//
// It writes no metadata. WAV can carry text chunks and this package does not
// use them, because everything worth knowing about a transmission goes in the
// JSON sidecar beside it, where it can be read without parsing a container. It
// also creates no directories; the caller has already decided where the file
// goes.
package wavfile

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// Create opens path and writes the header, ready for audio to be appended.
//
// The header is written now with both of its length fields set to zero, and
// patched by Close once the audio has all arrived. That is the ordinary way to
// write a WAV: the alternative is holding the whole recording in memory to
// measure it first, which for something with no natural end is not an
// alternative at all.
//
// The directory must already exist. Deciding where recordings go is the
// caller's business, and a writer that quietly created directories would make a
// typo in a path template look like it worked.
//
// Parameters:
//   - path: the file to create, which is replaced if it already exists
//
// Returns:
//   - *Writer ready to take audio, which the caller must Close
//   - error if the file cannot be created or the header cannot be written
func Create(path string) (*Writer, error) {
	f, err := createFile(path)
	if err != nil {
		return nil, fmt.Errorf("creating the recording %s: %w", path, err)
	}

	w := &Writer{f: f, path: path}
	if _, err := f.Write(header()); err != nil {
		// The file exists at this point, so leaving it behind would leave a
		// zero length recording that looks like a transmission nobody can
		// play. Closing it is all that can be done about that here; the caller
		// is being told the recording failed either way.
		_ = f.Close()
		return nil, fmt.Errorf("writing the header of %s: %w", path, err)
	}
	return w, nil
}

// Close patches the two length fields in the header and closes the file.
//
// Both lengths are the whole reason this cannot simply be a write and a close.
// A WAV declares how long it is in two places, neither of which can be known
// until the audio has stopped, so the numbers are written last by seeking back
// over them. A file closed without this step holds every sample and still says
// it holds none, and most players believe the header.
//
// It is safe to call more than once, because a recording is closed on the way
// out of the normal path and again by a deferred call when something failed
// part way through.
//
// Returns:
//   - error if the header cannot be patched or the file cannot be closed
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if err := w.patch(riffSizeAt, uint32(w.n+riffSizeOverhead)); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.patch(dataSizeAt, uint32(w.n)); err != nil {
		_ = w.f.Close()
		return err
	}

	if err := w.f.Close(); err != nil {
		return fmt.Errorf("closing the recording %s: %w", w.path, err)
	}
	return nil
}

// Duration reports how much audio has been written so far.
//
// Worked out from the byte count rather than from a clock, so it is the length
// of the recording itself and not how long the recorder was running. The two
// differ whenever audio was dropped, and the file's own length is the honest
// answer for something being named and catalogued by its duration.
//
// Returns:
//   - how much audio the file holds
func (w *Writer) Duration() time.Duration {
	return time.Duration(w.n) * time.Second / time.Duration(ByteRate)
}

// Write appends audio to the recording.
//
// Parameters:
//   - pcm: signed 16-bit little-endian mono samples, a whole number of them
//
// Returns:
//   - error if pcm is not a whole number of samples, if the file has reached
//     the largest size a WAV can describe, or if the write fails
//
// Errors:
//   - ErrSampleAlign: if pcm does not divide into whole sample frames
//   - ErrTooLarge: if the recording has reached the format's size limit
func (w *Writer) Write(pcm []byte) error {
	if len(pcm)%BlockAlign != 0 {
		return fmt.Errorf("%w: got %d bytes, want a multiple of %d", ErrSampleAlign, len(pcm), BlockAlign)
	}
	if w.n+int64(len(pcm)) > maxData {
		return fmt.Errorf("%w: %s has reached %d bytes", ErrTooLarge, w.path, w.n)
	}

	if _, err := w.f.Write(pcm); err != nil {
		return fmt.Errorf("writing audio to %s: %w", w.path, err)
	}
	w.n += int64(len(pcm))
	return nil
}

// header returns the 44 byte WAV header, with both lengths left at zero for
// Close to fill in.
//
// Written out field by field rather than through a struct so that the layout
// reads in the order the bytes appear in the file, which is how the format is
// documented and the only way to check it against a specification.
//
// Returns:
//   - the header, exactly headerBytes long
func header() []byte {
	h := make([]byte, headerBytes)

	copy(h[0:], "RIFF")
	// h[4:8] is the RIFF length, patched by Close.
	copy(h[8:], "WAVE")

	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16) // Length of this chunk
	binary.LittleEndian.PutUint16(h[20:], 1)  // Format 1 is uncompressed PCM
	binary.LittleEndian.PutUint16(h[22:], Channels)
	binary.LittleEndian.PutUint32(h[24:], SampleRate)
	binary.LittleEndian.PutUint32(h[28:], ByteRate)
	binary.LittleEndian.PutUint16(h[32:], BlockAlign)
	binary.LittleEndian.PutUint16(h[34:], BitsPerSample)

	copy(h[36:], "data")
	// h[40:44] is the data length, patched by Close.

	return h
}

// patch seeks to off and overwrites the four byte length there.
//
// Parameters:
//   - off: where in the file the length sits
//   - v: the length to write, little endian as the format requires
//
// Returns:
//   - error if the file cannot be seeked or the length cannot be written
func (w *Writer) patch(off int64, v uint32) error {
	if _, err := w.f.Seek(off, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding %s to complete its header: %w", w.path, err)
	}

	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	if _, err := w.f.Write(b[:]); err != nil {
		return fmt.Errorf("completing the header of %s: %w", w.path, err)
	}
	return nil
}
