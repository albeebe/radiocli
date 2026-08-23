// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package wavfile

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// errBroken is what the fake file fails with, so a test can check the failure
// it provoked is the one that came back rather than some other error.
var errBroken = errors.New("broken")

// fakeFile stands in for an open file and fails on demand.
//
// It exists because the failures this package has to handle are a full disk, a
// stream that cannot seek and a close that fails, none of which a test can
// arrange on a real filesystem. It records what was written so the header can
// be checked byte for byte.
type fakeFile struct {
	// buf is everything written, with seeks applied, so it ends up holding
	// exactly what the file on disk would.
	buf []byte

	// at is the write position, moved by Seek and advanced by Write.
	at int64

	// failWriteAfter makes the nth Write onwards fail. Zero fails the first.
	failWriteAfter int

	// writes counts calls to Write, so failWriteAfter can be applied.
	writes int

	// failSeek and failClose make those calls fail.
	failSeek  bool
	failClose bool

	// closed records that Close was called, so the tests can check a failed
	// Create does not leave the file open.
	closed bool
}

// Close records the call and fails if the fake was told to.
//
// Returns:
//   - error if the fake was told to fail
func (f *fakeFile) Close() error {
	f.closed = true
	if f.failClose {
		return errBroken
	}
	return nil
}

// Seek moves the write position, and fails if the fake was told to.
//
// Parameters:
//   - off: the offset to move to
//   - whence: ignored, since this package only ever seeks from the start
//
// Returns:
//   - the new position
//   - error if the fake was told to fail
func (f *fakeFile) Seek(off int64, whence int) (int64, error) {
	if f.failSeek {
		return 0, errBroken
	}
	f.at = off
	return off, nil
}

// Write copies p in at the current position, growing the buffer as needed.
//
// Parameters:
//   - p: the bytes to write
//
// Returns:
//   - how many bytes were written
//   - error if the fake was told to fail on this call
func (f *fakeFile) Write(p []byte) (int, error) {
	f.writes++
	if f.failWriteAfter > 0 && f.writes >= f.failWriteAfter {
		return 0, errBroken
	}

	for int64(len(f.buf)) < f.at+int64(len(p)) {
		f.buf = append(f.buf, 0)
	}
	copy(f.buf[f.at:], p)
	f.at += int64(len(p))
	return len(p), nil
}

// withFake points createFile at a fake for the duration of one test.
//
// Parameters:
//   - t: the test, which restores the real createFile when it ends
//   - f: the fake to hand back, or nil to fail the create
//
// Returns:
//   - the fake, for the test to inspect afterwards
func withFake(t *testing.T, f *fakeFile) *fakeFile {
	t.Helper()

	original := createFile
	t.Cleanup(func() { createFile = original })

	createFile = func(string) (file, error) {
		if f == nil {
			return nil, errBroken
		}
		return f, nil
	}
	return f
}

// TestCreateWritesAHeader checks that a new file opens with a complete and
// correct WAV header, since every field in it is arithmetic that nothing else
// would catch.
func TestCreateWritesAHeader(t *testing.T) {
	f := withFake(t, &fakeFile{})

	w, err := Create("recording.wav")
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}

	if len(f.buf) != headerBytes {
		t.Fatalf("header is %d bytes, want %d", len(f.buf), headerBytes)
	}

	for _, c := range []struct {
		at   int
		want string
	}{{0, "RIFF"}, {8, "WAVE"}, {12, "fmt "}, {36, "data"}} {
		if got := string(f.buf[c.at : c.at+4]); got != c.want {
			t.Errorf("bytes at %d are %q, want %q", c.at, got, c.want)
		}
	}

	for _, c := range []struct {
		name string
		got  uint32
		want uint32
	}{
		{"format chunk length", binary.LittleEndian.Uint32(f.buf[16:]), 16},
		{"sample rate", binary.LittleEndian.Uint32(f.buf[24:]), SampleRate},
		{"byte rate", binary.LittleEndian.Uint32(f.buf[28:]), ByteRate},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d, want %d", c.name, c.got, c.want)
		}
	}

	for _, c := range []struct {
		name string
		got  uint16
		want uint16
	}{
		{"format", binary.LittleEndian.Uint16(f.buf[20:]), 1},
		{"channels", binary.LittleEndian.Uint16(f.buf[22:]), Channels},
		{"block align", binary.LittleEndian.Uint16(f.buf[32:]), BlockAlign},
		{"bits per sample", binary.LittleEndian.Uint16(f.buf[34:]), BitsPerSample},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d, want %d", c.name, c.got, c.want)
		}
	}

	// The lengths are not known yet, so they must still be zero.
	if n := binary.LittleEndian.Uint32(f.buf[riffSizeAt:]); n != 0 {
		t.Errorf("RIFF length before Close is %d, want 0", n)
	}
	if n := binary.LittleEndian.Uint32(f.buf[dataSizeAt:]); n != 0 {
		t.Errorf("data length before Close is %d, want 0", n)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close returned %v, want nil", err)
	}
}

// TestCreateFails checks the two ways opening a recording can fail.
func TestCreateFails(t *testing.T) {
	t.Run("the file cannot be created", func(t *testing.T) {
		withFake(t, nil)

		if _, err := Create("recording.wav"); !errors.Is(err, errBroken) {
			t.Fatalf("Create returned %v, want it to wrap errBroken", err)
		}
	})

	t.Run("the header cannot be written", func(t *testing.T) {
		f := withFake(t, &fakeFile{failWriteAfter: 1})

		if _, err := Create("recording.wav"); !errors.Is(err, errBroken) {
			t.Fatalf("Create returned %v, want it to wrap errBroken", err)
		}
		// A file that was created and then failed must not be left open.
		if !f.closed {
			t.Error("the file was left open after the header failed")
		}
	})
}

// TestCloseCompletesTheHeader checks that the lengths a player reads are
// patched in, which is the step that turns a file full of samples into one that
// says it holds them.
func TestCloseCompletesTheHeader(t *testing.T) {
	f := withFake(t, &fakeFile{})

	w, err := Create("recording.wav")
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}

	audio := make([]byte, 960)
	if err := w.Write(audio); err != nil {
		t.Fatalf("Write returned %v, want nil", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close returned %v, want nil", err)
	}

	if n := binary.LittleEndian.Uint32(f.buf[dataSizeAt:]); n != uint32(len(audio)) {
		t.Errorf("data length is %d, want %d", n, len(audio))
	}
	if n := binary.LittleEndian.Uint32(f.buf[riffSizeAt:]); n != uint32(len(audio)+riffSizeOverhead) {
		t.Errorf("RIFF length is %d, want %d", n, len(audio)+riffSizeOverhead)
	}
	if len(f.buf) != headerBytes+len(audio) {
		t.Errorf("file is %d bytes, want %d", len(f.buf), headerBytes+len(audio))
	}
}

// TestCloseIsIdempotent checks that closing twice is harmless, because a
// recording is closed on the normal path and again by a deferred call when
// something went wrong part way through.
func TestCloseIsIdempotent(t *testing.T) {
	f := withFake(t, &fakeFile{})

	w, err := Create("recording.wav")
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("first Close returned %v, want nil", err)
	}

	// The second close must not touch the file at all, since by now it is gone.
	f.failSeek, f.failClose = true, true
	if err := w.Close(); err != nil {
		t.Fatalf("second Close returned %v, want nil", err)
	}
}

// TestCloseFails checks each way completing the header can fail, since a
// recording whose lengths were never written is one nothing will play.
func TestCloseFails(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(*fakeFile)
	}{
		{"the file cannot be rewound", func(f *fakeFile) { f.failSeek = true }},
		// Two writes have already happened by then: the header and the audio,
		// so failing from the third catches the first patch.
		{"the RIFF length cannot be written", func(f *fakeFile) { f.failWriteAfter = 3 }},
		{"the data length cannot be written", func(f *fakeFile) { f.failWriteAfter = 4 }},
		{"the file cannot be closed", func(f *fakeFile) { f.failClose = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := withFake(t, &fakeFile{})

			w, err := Create("recording.wav")
			if err != nil {
				t.Fatalf("Create returned %v, want nil", err)
			}
			if err := w.Write(make([]byte, 4)); err != nil {
				t.Fatalf("Write returned %v, want nil", err)
			}

			c.set(f)
			if err := w.Close(); !errors.Is(err, errBroken) {
				t.Fatalf("Close returned %v, want it to wrap errBroken", err)
			}
		})
	}
}

// TestDuration checks that length is read out of the bytes written rather than
// off a clock, which is what makes it the length of the recording instead of
// how long the recorder ran.
func TestDuration(t *testing.T) {
	withFake(t, &fakeFile{})

	w, err := Create("recording.wav")
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}

	if d := w.Duration(); d != 0 {
		t.Errorf("a new recording is %v long, want 0", d)
	}

	// One second of audio, by definition.
	if err := w.Write(make([]byte, ByteRate)); err != nil {
		t.Fatalf("Write returned %v, want nil", err)
	}
	if d := w.Duration(); d != time.Second {
		t.Errorf("Duration is %v, want %v", d, time.Second)
	}

	// And 20 ms more, the size of one frame from the feed.
	if err := w.Write(make([]byte, ByteRate/50)); err != nil {
		t.Fatalf("Write returned %v, want nil", err)
	}
	if want := time.Second + 20*time.Millisecond; w.Duration() != want {
		t.Errorf("Duration is %v, want %v", w.Duration(), want)
	}
}

// TestWriteRefusesPartOfASample checks the alignment guard, which matters
// because the damage from a ragged write is not a short file but every sample
// after it being assembled from the wrong pair of bytes.
func TestWriteRefusesPartOfASample(t *testing.T) {
	f := withFake(t, &fakeFile{})

	w, err := Create("recording.wav")
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}

	if err := w.Write(make([]byte, 3)); !errors.Is(err, ErrSampleAlign) {
		t.Fatalf("Write returned %v, want ErrSampleAlign", err)
	}
	if len(f.buf) != headerBytes {
		t.Errorf("a refused write left %d bytes behind, want none", len(f.buf)-headerBytes)
	}
}

// TestWriteRefusesMoreThanAWavCanAddress checks the size guard. The lengths in
// the header are unsigned 32-bit, so without this the count wraps and the file
// claims to be small rather than the write failing.
func TestWriteRefusesMoreThanAWavCanAddress(t *testing.T) {
	withFake(t, &fakeFile{})

	w, err := Create("recording.wav")
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}

	// Pretend the file is already at the limit rather than writing four
	// gigabytes to find out.
	w.n = maxData
	if err := w.Write(make([]byte, BlockAlign)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Write returned %v, want ErrTooLarge", err)
	}
}

// TestWriteFails checks that a failure to write audio is reported rather than
// counted, since a recording that lost samples but still counted them would
// declare a length it does not have.
func TestWriteFails(t *testing.T) {
	withFake(t, &fakeFile{failWriteAfter: 2})

	w, err := Create("recording.wav")
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}

	if err := w.Write(make([]byte, 4)); !errors.Is(err, errBroken) {
		t.Fatalf("Write returned %v, want it to wrap errBroken", err)
	}
	if w.n != 0 {
		t.Errorf("a failed write counted %d bytes, want 0", w.n)
	}
}

// TestRoundTripOnDisk writes a real file through the real os.Create and reads
// it back, because everything else here runs against a fake and something has
// to prove the package works on an actual filesystem.
func TestRoundTripOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.wav")

	w, err := Create(path)
	if err != nil {
		t.Fatalf("Create returned %v, want nil", err)
	}

	// A quarter second of a recognisable pattern, so a truncated or misaligned
	// file would not read back the same.
	audio := make([]byte, ByteRate/4)
	for i := range audio {
		audio[i] = byte(i)
	}
	if err := w.Write(audio); err != nil {
		t.Fatalf("Write returned %v, want nil", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close returned %v, want nil", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the file back returned %v, want nil", err)
	}
	if len(got) != headerBytes+len(audio) {
		t.Fatalf("file is %d bytes, want %d", len(got), headerBytes+len(audio))
	}
	if n := binary.LittleEndian.Uint32(got[dataSizeAt:]); n != uint32(len(audio)) {
		t.Errorf("data length is %d, want %d", n, len(audio))
	}
	if string(got[headerBytes:]) != string(audio) {
		t.Error("the audio read back does not match what was written")
	}
}

// TestCreateOnDiskFails checks that a path that cannot be opened is reported
// through the real os.Create, which is the seam every other test replaces.
func TestCreateOnDiskFails(t *testing.T) {
	// A path whose parent is not a directory, which this package deliberately
	// does not create.
	if _, err := Create(filepath.Join(t.TempDir(), "no-such-directory", "recording.wav")); err == nil {
		t.Fatal("Create returned nil, want an error")
	}
}

// TestFileInterfaceIsSatisfiedByOsFile checks the assumption the createFile
// default rests on, which the compiler would otherwise only check at that one
// line.
func TestFileInterfaceIsSatisfiedByOsFile(t *testing.T) {
	var _ file = (*os.File)(nil)
	var _ io.WriteSeeker = (*os.File)(nil)
}
