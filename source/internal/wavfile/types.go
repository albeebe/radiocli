// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package wavfile

import (
	"errors"
	"io"
	"math"
	"os"
)

// The format every file this package writes.
//
// One format, fixed, because the tool records one thing: the mono audio a
// scanner puts out, taken off a sound card that audiofeed has already opened at
// 48 kHz. A configurable writer would mean more code and more tests for
// combinations nothing in this tool produces.
//
// The numbers are restated here rather than imported from audiofeed so that
// anything sizing a buffer, or working out what one of these files holds, needs
// this package and nothing else. They are the same numbers, and audiofeed is
// where they come from.
const (
	// BitsPerSample is the sample width. Signed 16-bit little-endian is what
	// the sound card delivers and what a WAV's PCM format means without
	// further qualification.
	BitsPerSample = 16

	// BlockAlign is one sample frame in bytes: every channel's sample for a
	// single instant. Audio is only a whole number of these, which is what
	// Write checks.
	BlockAlign = Channels * BitsPerSample / 8

	// ByteRate is how many bytes one second of audio occupies, which is what
	// turns a file's length back into a duration.
	ByteRate = SampleRate * BlockAlign

	// Channels is one, because a scanner is mono. Which side of the cable the
	// signal arrived on is settled long before anything reaches here.
	Channels = 1

	// SampleRate is 48 kHz, the rate the sound card is opened at, so audio
	// passes through untouched rather than being resampled on the way to disk.
	SampleRate = 48000
)

// The parts of the RIFF container this package has to count in bytes.
const (
	// dataSizeAt is where the data chunk's length sits in the header, patched
	// by Close once the length is known.
	dataSizeAt = 40

	// headerBytes is the whole header written ahead of the audio: the RIFF
	// chunk, a 16 byte PCM format chunk, and the data chunk's own header.
	headerBytes = 44

	// riffSizeAt is where the RIFF chunk's length sits in the header, also
	// patched by Close.
	riffSizeAt = 4

	// riffSizeOverhead is what the RIFF length counts besides the audio:
	// everything after its own four bytes, which is the header less the eight
	// bytes of "RIFF" and the length itself.
	riffSizeOverhead = headerBytes - 8
)

// maxData is the most audio one file can hold.
//
// A WAV addresses its chunks with unsigned 32-bit lengths, so the format cannot
// describe a longer file however much disk there is. That is about 12 hours at
// this rate, and the recorder splits on a duration far below it, so nothing
// should ever reach this. It is checked because the failure if it were not is
// silent: the length would wrap and the file would claim to be small rather
// than refusing to grow.
const maxData = math.MaxUint32 - riffSizeOverhead

// ErrSampleAlign says Write was given a part of a sample.
//
// Audio is only ever a whole number of sample frames, and a file holding half
// of one is not merely short: every sample after the ragged edge is assembled
// from the wrong pair of bytes, so the whole rest of the file is noise.
var ErrSampleAlign = errors.New("not a whole number of samples")

// ErrTooLarge says the file has reached the largest size a WAV can describe.
var ErrTooLarge = errors.New("recording is larger than a WAV file can address")

// createFile opens the file a Writer writes to. It is a var so tests can
// substitute a fake and exercise the failures a real disk will not produce on
// demand, in the same way backup does.
var createFile = func(path string) (file, error) {
	return os.Create(path)
}

// file is the part of an open file a Writer uses.
//
// Seeking is in here because the two length fields in a WAV header cannot be
// known until the audio has all been written, so Close goes back and fills them
// in. That is the ordinary way to write a WAV and the reason this cannot be a
// plain io.Writer. *os.File satisfies it.
type file interface {
	io.Closer
	io.Seeker
	io.Writer
}

// Writer is one open WAV file that audio is appended to.
//
// It is not safe for concurrent use. One recording is written by one goroutine,
// which is how the recorder uses it.
type Writer struct {
	// f is the open file, kept as an interface so tests can supply a fake.
	f file

	// n counts the bytes of audio written, which is both the data chunk's
	// length and, divided by ByteRate, the duration.
	n int64

	// closed makes Close idempotent. A recording is closed on the normal path
	// and again by a deferred call when something failed part way through, and
	// patching the header twice would mean seeking a file that is already gone.
	closed bool

	// path is remembered for error messages, because a failure to write audio
	// says nothing useful without naming the file it was going to.
	path string
}
