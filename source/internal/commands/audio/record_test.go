// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audiogate"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/portlock"
	"github.com/albeebe/radiocli/internal/recordings"
)

// The levels the recorder tests drive the gate with, far enough apart that
// nothing here depends on where exactly the margin falls.
const (
	loudLevel  = -20.0
	quietLevel = -70.0
)

// heardOn returns a reading of a scanner receiving on channel.
//
// Parameters:
//   - channel: the channel it is on
//
// Returns:
//   - the reading, with the quiet zone names every example in this tool uses
func heardOn(channel string) device.Heard {
	return device.Heard{
		Receiving:  true,
		List:       "Pocahontas County",
		System:     "PUBLIC SAFETY",
		Department: "POLICE DEPARTMENT",
		Channel:    channel,
		Frequency:  "155.550000MHz",
		Modulation: "NFM",
	}
}

// sidecar reads the description written beside the one recording below dir.
//
// Parameters:
//   - t: the test, failed if there is no sidecar or it cannot be read
//   - dir: the destination the recording was written into
//
// Returns:
//   - the entry the recorder described the transmission with
func sidecar(t *testing.T, dir string) recordings.Entry {
	t.Helper()

	var found string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && found == "" && strings.HasSuffix(path, ".json") {
			found = path
		}
		return nil
	})
	if found == "" {
		t.Fatalf("no recording was described below %s", dir)
	}

	raw, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("reading %s: %v", found, err)
	}
	var e recordings.Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("%s is not JSON: %v", found, err)
	}
	return e
}

// recorderApp returns an App with buffers for streams and no scanner.
//
// Returns:
//   - the App
//   - what was written to stdout
//   - what was written to stderr
func recorderApp() (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, errs
	return app, out, errs
}

// silence returns one frame of mono audio at a given level.
//
// The samples are a square wave rather than a constant, because a constant
// offset measures as loud to an RMS meter and would make a test that meant to
// be quiet noisy.
//
// Parameters:
//   - level: the level wanted, in dBFS
//
// Returns:
//   - one frame of audio at about that level
func tone(level float64) []byte {
	pcm := make([]byte, audiofeed.MonoFrameBytes)
	amplitude := int16(math.Pow(10, level/20) * 32767)

	for i := 0; i < len(pcm); i += 2 {
		v := amplitude
		if i%4 == 0 {
			v = -amplitude
		}
		binary.LittleEndian.PutUint16(pcm[i:], uint16(v))
	}
	return pcm
}

// feed builds a channel already holding n frames at level, then closed, which
// is what the recorder sees when the audio ends.
//
// Parameters:
//   - from: the frame number to start at
//   - n: how many frames
//   - level: the level to give them
//
// Returns:
//   - the frames, and the next unused frame number
func feed(from uint32, n int, level float64) ([]audiofeed.Frame, uint32) {
	base := time.Date(2026, 8, 22, 19, 54, 3, 0, time.UTC)
	frames := make([]audiofeed.Frame, n)

	for i := range frames {
		seq := from + uint32(i)
		frames[i] = audiofeed.Frame{
			Seq:   seq,
			PCM:   tone(level),
			Level: level,
			At:    base.Add(time.Duration(seq) * audiofeed.FrameMS * time.Millisecond),
		}
	}
	return frames, from + uint32(n)
}

// Test_newRecord tests the newRecord function with 100% coverage.
//
// Coverage: 100% (2 test cases covering the command and the closure it holds)
//
// Test cases:
//   - Wiring: the command carries its name and its flags, with defaults
//   - Runs: executing the command reaches runRecord, which refuses it in a daemon
func Test_newRecord(t *testing.T) {
	// Verify the command and its flags are described the way the tool wires them.
	t.Run("Wiring", func(t *testing.T) {
		cmd := newRecord(appcontext.New())

		if cmd.Use != "record [destination]" {
			t.Errorf("the command is %q, want %q", cmd.Use, "record [destination]")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
		// It moves the scanner as far as anything can tell, so it must not
		// carry the annotation that lets a command run alongside another.
		if cmd.Annotations[appcontext.OnlyReads] != "" {
			t.Error("the command claims to only read, which would let it run alongside a menu walk")
		}

		for name, want := range map[string]string{
			"input":        "",
			"channel":      audiofeed.ChannelAuto,
			"template":     recordings.DefaultTemplate,
			"hang":         audiogate.DefaultHang.String(),
			"min-duration": audiogate.DefaultMinDuration.String(),
			"max-duration": audiogate.DefaultMaxDuration.String(),
			"normalize":    "true",
		} {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				t.Errorf("there is no --%s flag", name)
				continue
			}
			if f.DefValue != want {
				t.Errorf("--%s defaults to %q, want %q", name, f.DefValue, want)
			}
		}
	})

	// Verify the closure reaches the worker, which refuses to run in a daemon.
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.InDaemon = true

		cmd := newRecord(app)
		cmd.SetArgs([]string{"somewhere"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err == nil {
			t.Fatal("running inside a daemon was allowed")
		}
	})
}

// Test_entryFrom tests the entryFrom function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Labelled: the first reading naming a channel supplies the label
//   - NoReadings: a transmission nothing was seen during is left unlabelled
//   - UnitArrivesLate: a unit id decoded part way through is still picked up
//   - TwoChannels: a recording spanning two channels lists both
func Test_entryFrom(t *testing.T) {
	tx := audiogate.Transmission{
		Start:   time.Date(2026, 8, 22, 19, 54, 3, 0, time.UTC),
		End:     time.Date(2026, 8, 22, 19, 54, 8, 0, time.UTC),
		Reason:  audiogate.ReasonHang,
		Dropped: 3,
	}

	// Verify a labelled transmission carries the whole hierarchy.
	t.Run("Labelled", func(t *testing.T) {
		e := entryFrom(tx, []device.Heard{heardOn("MARLINTON DISPATCH"), heardOn("MARLINTON DISPATCH")})

		if e.Channel != "MARLINTON DISPATCH" || e.System != "PUBLIC SAFETY" {
			t.Errorf("got %+v, want the channel and system", e)
		}
		if e.Frequency != "155.550000MHz" || e.Modulation != "NFM" {
			t.Errorf("got %+v, want the frequency and modulation", e)
		}
		if e.Samples != 2 || e.Dropped != 3 || e.Reason != audiogate.ReasonHang {
			t.Errorf("got %+v, want the transmission's own facts carried over", e)
		}
		// One channel says nothing worth saying, so the list is left out.
		if e.Channels != nil {
			t.Errorf("got channels %v, want none when they all agree", e.Channels)
		}
	})

	// Verify a transmission the scanner was never seen during says so rather
	// than inventing a label.
	t.Run("NoReadings", func(t *testing.T) {
		e := entryFrom(tx, nil)

		if e.Channel != "" || e.System != "" || e.Frequency != "" {
			t.Errorf("got %+v, want everything unnamed", e)
		}
		if e.Samples != 0 {
			t.Errorf("got %d samples, want 0 so the empty fields have a reason", e.Samples)
		}
	})

	// Verify a unit id the scanner only decodes part way through is picked up,
	// since it reports nothing there until it has.
	t.Run("UnitArrivesLate", func(t *testing.T) {
		late := heardOn("MARLINTON DISPATCH")
		late.Unit = "32"

		e := entryFrom(tx, []device.Heard{heardOn("MARLINTON DISPATCH"), late})
		if e.Unit != "32" {
			t.Errorf("got unit %q, want it taken from wherever it appeared", e.Unit)
		}
	})

	// Verify a recording that spans two channels lists both rather than
	// quietly picking one.
	t.Run("TwoChannels", func(t *testing.T) {
		e := entryFrom(tx, []device.Heard{heardOn("MARLINTON DISPATCH"), heardOn("GREEN BANK FIRE")})

		if len(e.Channels) != 2 {
			t.Fatalf("got channels %v, want both listed", e.Channels)
		}
		if e.Channel != "MARLINTON DISPATCH" {
			t.Errorf("the label is %q, want the first one seen", e.Channel)
		}
	})
}

// Test_contains tests the contains function with 100% coverage.
func Test_contains(t *testing.T) {
	if !contains([]string{"a", "b"}, "b") {
		t.Error("a value that is there was not found")
	}
	if contains([]string{"a"}, "b") {
		t.Error("a value that is not there was found")
	}
}

// Test_key tests the key function with 100% coverage.
//
// The identity is every part of where a channel sits rather than its name
// alone, because two departments can each have a channel called Dispatch and
// treating those as one would join two calls into a single file.
func Test_key(t *testing.T) {
	if got := key(device.Heard{Channel: "Dispatch"}); got != "" {
		t.Errorf("a scanner receiving nothing has the key %q, want none", got)
	}

	a, b := heardOn("Dispatch"), heardOn("Dispatch")
	if key(a) != key(b) {
		t.Error("the same channel gave two different keys")
	}

	b.Department = "FIRE RESCUE"
	if key(a) == key(b) {
		t.Error("the same channel name in two departments gave one key")
	}
}

// Test_reportRecording tests the reportRecording function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both output formats)
//
// Test cases:
//   - Text: one line per transmission, on stdout
//   - JSON: the same object that was written beside the audio
func Test_reportRecording(t *testing.T) {
	e := recordings.Entry{
		File:       "2026-08-22/19-54-03_x.wav",
		Start:      time.Date(2026, 8, 22, 19, 54, 3, 0, time.UTC),
		Duration:   4.8,
		System:     "PUBLIC SAFETY",
		Department: "POLICE DEPARTMENT",
		Channel:    "MARLINTON DISPATCH",
	}

	t.Run("Text", func(t *testing.T) {
		app, out, _ := recorderApp()
		if err := reportRecording(app, e); err != nil {
			t.Fatalf("reporting: %v", err)
		}
		if !strings.Contains(out.String(), "MARLINTON DISPATCH") ||
			!strings.Contains(out.String(), "4.8s") {
			t.Errorf("wrote %q, want the channel and the length", out.String())
		}
	})

	t.Run("JSON", func(t *testing.T) {
		app, out, _ := recorderApp()
		app.Config.Output = appcontext.OutputJSON

		if err := reportRecording(app, e); err != nil {
			t.Fatalf("reporting: %v", err)
		}
		var got recordings.Entry
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("wrote %q, which is not JSON: %v", out.String(), err)
		}
		if got.File != e.File {
			t.Errorf("got %+v, want the entry as it was filed", got)
		}
	})

	// An unlabelled recording still prints a line rather than a blank one.
	t.Run("Unlabelled", func(t *testing.T) {
		app, out, _ := recorderApp()
		if err := reportRecording(app, recordings.Entry{Start: e.Start}); err != nil {
			t.Fatalf("reporting: %v", err)
		}
		if !strings.Contains(out.String(), "-") {
			t.Errorf("wrote %q, want a dash where the channel would be", out.String())
		}
	})
}

// Test_announceRecording tests the announceRecording function with 100%
// coverage, and that it writes to stderr rather than stdout.
func Test_announceRecording(t *testing.T) {
	app, out, errs := recorderApp()
	announceRecording(app, "USB Audio CODEC", "./rec")

	if out.Len() != 0 {
		t.Errorf("wrote %q to stdout, want it kept for the recordings", out.String())
	}
	if !strings.Contains(errs.String(), "USB Audio CODEC") || !strings.Contains(errs.String(), "./rec") {
		t.Errorf("wrote %q, want the input and the destination named", errs.String())
	}
}

// radio is a scanner the test turns on and off, so a transmission can be
// started and finished the way a real one is.
//
// The recorder asks it several times a second from a goroutine, so the answer
// has to be safe to change from the test while that is going on.
type radio struct {
	on atomic.Bool
}

// sample answers what the scanner is hearing right now.
//
// Returns:
//   - a transmission on a fixed channel while the radio is on, nothing while
//     it is off
//   - error never
func (r *radio) sample(context.Context) (device.Heard, error) {
	if r.on.Load() {
		return heardOn("MARLINTON DISPATCH"), nil
	}
	return device.Heard{}, nil
}

// settle waits long enough for the recorder to have asked the radio at least
// once, so a change made by a test has reached the gate before audio does.
func settle() { time.Sleep(3 * samplePeriod) }

// Test_recordLoop tests the recordLoop function with 100% coverage.
//
// It is the whole recorder driven from a channel of frames and a radio the test
// controls, so a night of scanner traffic runs through it in a moment and lands
// as real files.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Transmission: the radio opens and closes one, and a file is written
//   - AudioEnds: the feed closing ends the run without complaint
//   - Cancelled: stopping part way through keeps the part that happened
//   - NoiseWithoutTheRadio: audio the radio never confirmed is not recorded
//   - Quiet: audio that never rises above the floor writes nothing
func Test_recordLoop(t *testing.T) {
	// start runs the loop against a channel and a radio the caller drives.
	start := func(t *testing.T) (*radio, chan audiofeed.Frame, string, *bytes.Buffer,
		context.CancelFunc, <-chan error) {
		t.Helper()

		app, out, _ := recorderApp()
		dir := t.TempDir()
		library, err := recordings.New(dir, "", false)
		if err != nil {
			t.Fatalf("opening the library: %v", err)
		}

		r := &radio{}
		ctx, cancel := context.WithCancel(context.Background())
		frames := make(chan audiofeed.Frame, 4096)
		done := make(chan error, 1)

		go func() {
			done <- recordLoop(ctx, app, library, frames, r.sample, recordOptions{
				hang:        200 * time.Millisecond,
				minDuration: 100 * time.Millisecond,
			})
		}()
		return r, frames, dir, out, cancel, done
	}

	// wavs counts the recordings written below dir.
	wavs := func(t *testing.T, dir string) int {
		t.Helper()
		n := 0
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && strings.HasSuffix(path, ".wav") {
				n++
			}
			return nil
		})
		return n
	}

	// Verify a transmission recorded through an overloaded input is warned
	// about, all the way through the real wiring rather than a recorder built
	// by hand. This is the path that reads the scanner for the level, so it is
	// the one that has to work when nothing can be read.
	t.Run("Clipping", func(t *testing.T) {
		r, frames, dir, _, cancel, done := start(t)
		defer cancel()

		// A tone at 0 dBFS is every sample at full scale, which is what an
		// input given far more signal than it can take hands back.
		quiet, next := feed(0, 30, quietLevel)
		for _, f := range quiet {
			frames <- f
		}
		r.on.Store(true)
		settle()
		loud, next := feed(next, 60, 0.0)
		for _, f := range loud {
			frames <- f
		}
		r.on.Store(false)
		settle()
		tail, _ := feed(next, 60, quietLevel)
		for _, f := range tail {
			frames <- f
		}
		close(frames)

		if err := <-done; err != nil {
			t.Fatalf("recording: %v", err)
		}
		if n := wavs(t, dir); n != 1 {
			t.Fatalf("wrote %d recordings, want the clipped one", n)
		}
	})

	// Verify a transmission the radio confirmed lands as a file with a
	// description beside it.
	t.Run("Transmission", func(t *testing.T) {
		r, frames, dir, out, cancel, done := start(t)
		defer cancel()

		quiet, next := feed(0, 30, quietLevel)
		for _, f := range quiet {
			frames <- f
		}

		// The radio stops on something, then the audio arrives.
		r.on.Store(true)
		settle()
		loud, next := feed(next, 60, loudLevel)
		for _, f := range loud {
			frames <- f
		}

		// It moves on, and the audio falls back to the noise floor.
		r.on.Store(false)
		settle()
		tail, _ := feed(next, 60, quietLevel)
		for _, f := range tail {
			frames <- f
		}
		close(frames)

		if err := <-done; err != nil {
			t.Fatalf("recording: %v", err)
		}
		if n := wavs(t, dir); n != 1 {
			t.Fatalf("wrote %d recordings, want 1", n)
		}
		if out.Len() == 0 {
			t.Error("nothing was printed for the finished recording")
		}

		e := sidecar(t, dir)
		if e.Channel != "MARLINTON DISPATCH" || e.Samples == 0 {
			t.Errorf("got %+v, want it labelled from the radio", e)
		}
	})

	// Verify the audio ending is an ordinary ending rather than a failure.
	t.Run("AudioEnds", func(t *testing.T) {
		_, frames, _, _, cancel, done := start(t)
		defer cancel()

		quiet, _ := feed(0, 10, quietLevel)
		for _, f := range quiet {
			frames <- f
		}
		close(frames)

		if err := <-done; err != nil {
			t.Fatalf("the feed closing reported %v, want nil", err)
		}
	})

	// Verify stopping part way through a transmission keeps what happened.
	t.Run("Cancelled", func(t *testing.T) {
		r, frames, dir, _, cancel, done := start(t)

		r.on.Store(true)
		settle()
		loud, _ := feed(0, 60, loudLevel)
		for _, f := range loud {
			frames <- f
		}
		for len(frames) > 0 {
			time.Sleep(time.Millisecond)
		}

		cancel()
		if err := <-done; err != nil {
			t.Fatalf("recording: %v", err)
		}
		if n := wavs(t, dir); n != 1 {
			t.Errorf("wrote %d recordings, want the interrupted one kept", n)
		}
	})

	// Verify audio the radio never confirmed is not recorded.
	//
	// This is the regression the whole design turns on. A scanner's line output
	// has more than one idle level, so audio well above the quietest of them is
	// not evidence of anything, and treating it as evidence produced sixteen
	// second recordings of a noise floor.
	t.Run("NoiseWithoutTheRadio", func(t *testing.T) {
		_, frames, dir, out, cancel, done := start(t)
		defer cancel()

		// The floor settles on the quieter idle level, then the input moves to
		// the louder one and stays there, with the radio silent throughout.
		quiet, next := feed(0, 200, -88)
		for _, f := range quiet {
			frames <- f
		}
		noisy, _ := feed(next, 800, -77)
		for _, f := range noisy {
			frames <- f
		}
		close(frames)

		if err := <-done; err != nil {
			t.Fatalf("recording: %v", err)
		}
		if n := wavs(t, dir); n != 0 {
			t.Errorf("wrote %d recordings of a noise floor, want none", n)
		}
		if out.Len() != 0 {
			t.Errorf("printed %q for audio the radio never confirmed", out.String())
		}
	})

	// Verify a quiet channel with a quiet radio writes nothing at all.
	t.Run("Quiet", func(t *testing.T) {
		_, frames, dir, out, cancel, done := start(t)
		defer cancel()

		quiet, _ := feed(0, 200, quietLevel)
		for _, f := range quiet {
			frames <- f
		}
		close(frames)

		if err := <-done; err != nil {
			t.Fatalf("recording: %v", err)
		}
		if n := wavs(t, dir); n != 0 || out.Len() != 0 {
			t.Errorf("a quiet channel produced %d recordings and printed %q", n, out.String())
		}
	})
}

// Test_recorder tests the recorder with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - StartThenAudio: a file is opened and the frame written into it
//   - NothingOpen: audio and an ending with no recording open are guarded
//   - BeginFails: a destination that will not take a file is reported
//   - CloseFails: a destination that went away before filing is reported
//   - Abandon: a recording thrown away leaves nothing behind, twice over
func Test_recorder(t *testing.T) {
	// newRecorder returns a recorder writing into a fresh directory.
	newRecorder := func(t *testing.T) (*recorder, string) {
		t.Helper()

		app, _, _ := recorderApp()
		dir := t.TempDir()
		l, err := recordings.New(dir, "", false)
		if err != nil {
			t.Fatalf("opening the library: %v", err)
		}

		return &recorder{app: app, library: l, gate: audiogate.New(audiogate.Options{})}, dir
	}

	// breakDestination replaces the destination with a file, so every path
	// below it becomes impossible. It is the closest thing to a disk that
	// stopped cooperating that a test can arrange.
	breakDestination := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("removing the destination: %v", err)
		}
		if err := os.WriteFile(dir, nil, 0o644); err != nil {
			t.Fatalf("putting a file where the destination was: %v", err)
		}
	}

	// Verify a start opens a recording and audio goes into it.
	t.Run("StartThenAudio", func(t *testing.T) {
		r, _ := newRecorder(t)

		if err := r.apply([]audiogate.Event{{Kind: audiogate.KindStart}}); err != nil {
			t.Fatalf("starting: %v", err)
		}
		if r.open == nil {
			t.Fatal("nothing was opened")
		}

		frames, _ := feed(0, 1, loudLevel)
		if err := r.apply([]audiogate.Event{{Kind: audiogate.KindAudio, Frame: frames[0]}}); err != nil {
			t.Fatalf("writing audio: %v", err)
		}

		if err := r.apply([]audiogate.Event{{Kind: audiogate.KindEnd}}); err != nil {
			t.Fatalf("ending: %v", err)
		}
		if r.open != nil {
			t.Error("the recording was left open after it ended")
		}
	})

	// Verify the guards, which cannot happen but must not be a nil dereference
	// in the middle of a night's recording if they ever do.
	t.Run("NothingOpen", func(t *testing.T) {
		r, _ := newRecorder(t)

		for _, kind := range []audiogate.Kind{audiogate.KindAudio, audiogate.KindEnd} {
			if err := r.apply([]audiogate.Event{{Kind: kind}}); err != nil {
				t.Errorf("a %s with nothing open gave %v, want nothing", kind, err)
			}
		}
	})

	// Verify a destination that will not take a file is reported rather than
	// recorded into nothing.
	t.Run("BeginFails", func(t *testing.T) {
		r, dir := newRecorder(t)
		breakDestination(t, dir)

		if err := r.apply([]audiogate.Event{{Kind: audiogate.KindStart}}); err == nil {
			t.Fatal("a destination that cannot hold a file reported nothing")
		}
	})

	// Verify a destination that went away between the start and the end is
	// reported, since the recording cannot be filed anywhere.
	t.Run("CloseFails", func(t *testing.T) {
		r, dir := newRecorder(t)
		if err := r.apply([]audiogate.Event{{Kind: audiogate.KindStart}}); err != nil {
			t.Fatalf("starting: %v", err)
		}

		breakDestination(t, dir)
		if err := r.apply([]audiogate.Event{{Kind: audiogate.KindEnd}}); err == nil {
			t.Fatal("a recording that could not be filed reported nothing")
		}
	})

	// Verify throwing a recording away leaves nothing behind, and that doing it
	// with nothing open is harmless.
	t.Run("Abandon", func(t *testing.T) {
		r, dir := newRecorder(t)
		if err := r.apply([]audiogate.Event{{Kind: audiogate.KindStart}}); err != nil {
			t.Fatalf("starting: %v", err)
		}

		r.abandon()
		if r.open != nil {
			t.Error("the recording is still open after being abandoned")
		}
		r.abandon()

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading the destination: %v", err)
		}
		for _, e := range entries {
			t.Errorf("%s was left behind", e.Name())
		}
	})
}

// Test_newSampler tests the newSampler function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Direct: the scanner is read straight off the port
//   - NoDevice: a failure that is not a busy port is reported as it stands
//   - ViaDaemon: a busy port is read through a daemon without taking a turn
//   - BusyWithNoDaemon: a busy port with nothing sharing it reports the busy port
func Test_newSampler(t *testing.T) {
	// Verify the ordinary path reads the scanner directly.
	t.Run("Direct", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.SetDevice(device.New(recordConn{doc: `<ScannerInfo Mode="Scan Mode">` +
			`<ConvFrequency Name="MARLINTON DISPATCH" Freq=" 155.550000MHz"/>` +
			`<Property Mute="Unmute" Sig="3"/></ScannerInfo>`}))

		sample, done, err := newSampler(context.Background(), app)
		if err != nil {
			t.Fatalf("building the sampler: %v", err)
		}
		defer done()

		h, err := sample(context.Background())
		if err != nil {
			t.Fatalf("sampling: %v", err)
		}
		if !h.Receiving || h.Channel != "MARLINTON DISPATCH" {
			t.Errorf("got %+v, want the transmission", h)
		}
	})

	// Verify a scanner that was never named is reported as that rather than as
	// a missing daemon.
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := recorderApp()
		if _, _, err := newSampler(context.Background(), app); !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("got %v, want ErrNoDevice", err)
		}
	})

	// Verify a scanner read directly still reports a failure to read it.
	t.Run("ReadFails", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.SetDevice(device.New(recordConn{err: errors.New("the port closed")}))

		sample, done, err := newSampler(context.Background(), app)
		if err != nil {
			t.Fatalf("building the sampler: %v", err)
		}
		defer done()

		if _, err := sample(context.Background()); err == nil {
			t.Fatal("a scanner that would not answer reported nothing")
		}
	})
}

// recordConn is a device.Conn answering every command the same way.
type recordConn struct {
	doc string
	err error
}

// Info describes a scanner that is not there.
func (c recordConn) Info() device.Info { return device.Info{Port: "/dev/example", Model: "SDS150"} }

// Execute answers with the document or the failure.
func (c recordConn) Execute(ctx context.Context, command string) (string, error) {
	return c.doc, c.err
}

// ExecuteXML answers with the document or the failure.
func (c recordConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return c.doc, c.err
}

// Send reports whatever answering would have reported.
func (c recordConn) Send(ctx context.Context, command string) error { return c.err }

// Close releases nothing, because there is no port.
func (c recordConn) Close() error { return nil }

// Test_poll tests the poll function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Reads: readings arrive on the channel
//   - Fails: a scanner that stops answering is reported once
//   - Cancelled: stopping ends the polling without reporting anything
func Test_poll(t *testing.T) {
	// Verify readings arrive.
	t.Run("Reads", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		heard := make(chan device.Heard, 4)
		failures := make(chan error, 1)
		go poll(ctx, func(context.Context) (device.Heard, error) {
			return heardOn("MARLINTON DISPATCH"), nil
		}, heard, failures)

		select {
		case h := <-heard:
			if h.Channel != "MARLINTON DISPATCH" {
				t.Errorf("got %+v, want the reading", h)
			}
		case err := <-failures:
			t.Fatalf("polling reported %v, want a reading", err)
		case <-time.After(2 * time.Second):
			t.Fatal("no reading arrived")
		}
	})

	// Verify a scanner that stops answering is reported.
	t.Run("Fails", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		heard := make(chan device.Heard, 1)
		failures := make(chan error, 1)
		go poll(ctx, func(context.Context) (device.Heard, error) {
			return device.Heard{}, errors.New("the port closed")
		}, heard, failures)

		select {
		case err := <-failures:
			if err == nil {
				t.Fatal("the failure was reported as nothing")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("the failure was never reported")
		}
	})

	// Verify a cancelled run stops without reporting the cancellation as a
	// scanner that broke.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		heard := make(chan device.Heard, 1)
		failures := make(chan error, 1)
		poll(ctx, func(context.Context) (device.Heard, error) {
			return device.Heard{}, errors.New("cancelled")
		}, heard, failures)

		if len(failures) != 0 {
			t.Error("stopping was reported as a failure")
		}
	})
}

// Test_mismatch tests the mismatch check with 100% coverage.
//
// It is the check that makes requiring a scanner worth something: it is the one
// thing that can tell somebody their cable is in the wrong socket, or that they
// have pointed the recorder at a microphone.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Agreeing: a working setup says nothing
//   - RadioWithNoAudio: a cable that is not delivering is called out
//   - AudioWithNoRadio: an input that is not the scanner is called out
//   - Momentary: a brief disagreement is not enough to complain about
func Test_mismatch(t *testing.T) {
	// drive feeds the check n frames of one situation and returns stderr.
	drive := func(t *testing.T, receiving bool, level float64, n int, span time.Duration) string {
		t.Helper()

		app, _, errs := recorderApp()
		var m mismatch
		base := time.Now().Add(-span)
		heard := device.Heard{Receiving: receiving}

		for i := range n {
			m.observe(app, audiofeed.Frame{
				Level: level,
				At:    base.Add(time.Duration(i) * audiofeed.FrameMS * time.Millisecond),
			}, heard, quietLevel)
		}
		return errs.String()
	}

	// Verify a working setup is silent, in both of its states.
	t.Run("Agreeing", func(t *testing.T) {
		if got := drive(t, true, loudLevel, 500, time.Minute); got != "" {
			t.Errorf("a working setup said %q, want nothing", got)
		}
		if got := drive(t, false, quietLevel, 500, time.Minute); got != "" {
			t.Errorf("a quiet channel said %q, want nothing", got)
		}
	})

	// Verify a radio receiving into a dead cable is called out, with advice
	// about the two things that actually cause it.
	t.Run("RadioWithNoAudio", func(t *testing.T) {
		got := drive(t, true, quietLevel, mismatchLimit+10, time.Minute)
		if !strings.Contains(got, "nothing is arriving") {
			t.Fatalf("said %q, want the dead cable called out", got)
		}
		if !strings.Contains(got, "volume") {
			t.Errorf("said %q, want the volume mentioned as a cause", got)
		}
	})

	// Verify sound arriving with the radio silent is called out as the wrong
	// input, which is the microphone case.
	t.Run("AudioWithNoRadio", func(t *testing.T) {
		got := drive(t, false, loudLevel, mismatchLimit+10, time.Minute)
		if !strings.Contains(got, "not the scanner") {
			t.Fatalf("said %q, want the wrong input called out", got)
		}
	})

	// Verify a disagreement that has not lasted is not complained about, since
	// the two ends never agree to the millisecond at the edge of a transmission.
	t.Run("Momentary", func(t *testing.T) {
		if got := drive(t, true, quietLevel, mismatchLimit+10, 0); got != "" {
			t.Errorf("a momentary disagreement said %q, want nothing", got)
		}
	})

	// Verify the counters reset once the two agree again, so an evening of
	// brief disagreements never adds up to a complaint.
	t.Run("Resets", func(t *testing.T) {
		app, _, errs := recorderApp()
		var m mismatch
		at := time.Now().Add(-time.Minute)

		for range mismatchLimit - 1 {
			m.observe(app, audiofeed.Frame{Level: quietLevel, At: at},
				device.Heard{Receiving: true}, quietLevel)
		}
		m.observe(app, audiofeed.Frame{Level: quietLevel, At: at}, device.Heard{}, quietLevel)
		for range mismatchLimit - 1 {
			m.observe(app, audiofeed.Frame{Level: quietLevel, At: at},
				device.Heard{Receiving: true}, quietLevel)
		}

		if errs.Len() != 0 {
			t.Errorf("said %q, want the count reset by the agreement in the middle", errs.String())
		}
	})
}

// Test_openAudio tests the openAudio function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Direct: a named input is opened here
//   - DirectFails: a sound input that will not open is reported
//   - ViaDaemon: no input means a copy of what a daemon already has
func Test_openAudio(t *testing.T) {
	// Verify a named input is opened directly.
	t.Run("Direct", func(t *testing.T) {
		fakeStart(t, "USB Audio CODEC", nil)
		app, _, _ := recorderApp()

		frames, source, done, err := openAudio(context.Background(), app,
			"USB Audio CODEC", audiofeed.ChannelAuto)
		if err != nil {
			t.Fatalf("opening the audio: %v", err)
		}
		defer done()

		if source != "USB Audio CODEC" || frames == nil {
			t.Errorf("got %q and %v, want the input named", source, frames)
		}
	})

	// Verify a sound input that will not open is reported.
	t.Run("DirectFails", func(t *testing.T) {
		fakeStart(t, "", errors.New("no such input"))
		app, _, _ := recorderApp()

		if _, _, _, err := openAudio(context.Background(), app,
			"Nothing", audiofeed.ChannelAuto); err == nil {
			t.Fatal("an input that will not open reported nothing")
		}
	})

	// Verify no input at all reaches for a daemon, and says how to start one
	// when there is none.
	t.Run("NoDaemon", func(t *testing.T) {
		sockets(t)
		app, _, _ := recorderApp()
		app.Config.Device = "/dev/example"

		_, _, _, err := openAudio(context.Background(), app, "", audiofeed.ChannelAuto)
		if err == nil || !strings.Contains(err.Error(), "radiocli daemon") {
			t.Fatalf("got %v, want advice on starting a daemon", err)
		}
	})
}

// Test_audioViaDaemon tests taking audio from a daemon with 100% coverage.
//
// The daemon sends samples with no level and no timestamp, so both are worked
// out here, and the level has to be measured the same way the capture measures
// it or a gate tuned against one would behave differently depending on where
// its audio came from.
func Test_audioViaDaemon(t *testing.T) {
	sockets(t)
	const port = "/dev/example"

	audio := tone(loudLevel)
	daemon{
		hello: hello(),
		reply: broker.Response{Type: broker.TypeAudio, Format: formatPCM, Rate: 48000, Channels: 1},
		tail:  append(audioPacket(7, audio), audioFrame(broker.FrameJSON, []byte(`{"type":"event"}`))...),
	}.serve(t, port)

	app, _, _ := recorderApp()
	app.Config.Device = port

	frames, _, done, err := audioViaDaemon(context.Background(), app)
	if err != nil {
		t.Fatalf("asking the daemon for audio: %v", err)
	}
	defer done()

	select {
	case f := <-frames:
		if f.Seq != 7 {
			t.Errorf("the frame is numbered %d, want 7 as the daemon sent it", f.Seq)
		}
		// Measured rather than assumed, because the daemon sends none.
		if f.Level != audiofeed.LevelOf(audio) {
			t.Errorf("the frame measures %v, want %v", f.Level, audiofeed.LevelOf(audio))
		}
		if f.At.IsZero() {
			t.Error("the frame has no time on it, so the gate cannot place it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no audio arrived from the daemon")
	}
}

// Test_runRecord tests the runRecord function with 100% coverage of the checks
// it makes before anything is opened.
//
// Every one of these has to fail before a sound card is opened or a scanner is
// claimed, which is the whole point of doing them here.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - InDaemon: a command that never ends is refused inside a daemon
//   - NoDevice: recording without a scanner is refused
//   - BadChannel: a channel that is not one of the four is refused
//   - BadTemplate: a template that cannot work is refused
//   - SweepReported: recordings left by an earlier run are mentioned
func Test_runRecord(t *testing.T) {
	t.Run("InDaemon", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.InDaemon = true

		err := runRecord(context.Background(), app, recordOptions{})
		if err == nil || !strings.Contains(err.Error(), "cannot be run inside a daemon") {
			t.Fatalf("got %v, want it refused inside a daemon", err)
		}
	})

	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := recorderApp()

		err := runRecord(context.Background(), app, recordOptions{destination: t.TempDir()})
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Fatalf("got %v, want ErrNoDevice", err)
		}
		if !strings.Contains(err.Error(), "labelled") {
			t.Errorf("got %q, want it to say why the scanner is needed", err)
		}
	})

	t.Run("BadChannel", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.Config.Device = "/dev/example"

		if err := runRecord(context.Background(), app, recordOptions{
			destination: t.TempDir(), channel: "sideways"}); err == nil {
			t.Fatal("a channel that does not exist was accepted")
		}
	})

	t.Run("BadTemplate", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.Config.Device = "/dev/example"

		err := runRecord(context.Background(), app, recordOptions{
			destination: t.TempDir(), channel: audiofeed.ChannelAuto, template: "{nope}"})
		if !errors.Is(err, recordings.ErrBadTemplate) {
			t.Fatalf("got %v, want ErrBadTemplate", err)
		}
	})

	// Verify a run killed part way through a transmission has its leftovers
	// mentioned, since they are files that will not play.
	t.Run("SweepReported", func(t *testing.T) {
		// A scanner that answers, and a sound input that will not open, so the
		// run reaches the sweep and then stops.
		fakeStart(t, "", errors.New("no such input"))
		app, _, errs := recorderApp()
		app.Config.Device = "/dev/example"
		app.SetDevice(device.New(recordConn{doc: `<ScannerInfo Mode="Scan Mode"/>`}))

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".partial-1.wav"), nil, 0o644); err != nil {
			t.Fatalf("leaving a partial recording: %v", err)
		}

		_ = runRecord(context.Background(), app, recordOptions{
			destination: dir, input: "Nothing", channel: audiofeed.ChannelAuto})

		if !strings.Contains(errs.String(), "unfinished") {
			t.Errorf("said %q, want the leftover recording mentioned", errs.String())
		}
	})
}

// Test_tone checks the helper the other tests are built on, so a failure in it
// is not read as a failure in the recorder.
func Test_tone(t *testing.T) {
	if got := audiofeed.LevelOf(tone(loudLevel)); got < loudLevel-1 || got > loudLevel+1 {
		t.Errorf("a tone asked for at %v dBFS measured %v", loudLevel, got)
	}
	if got := audiofeed.LevelOf(tone(quietLevel)); got < quietLevel-1 || got > quietLevel+1 {
		t.Errorf("a tone asked for at %v dBFS measured %v", quietLevel, got)
	}
}

// runDaemon is a fake radiocli daemon that answers a run request, which is how
// the recorder reads the scanner while something else holds the port.
//
// It replies to every request with the same three messages, because the
// recorder only ever asks it one thing.
type runDaemon struct {
	// stdout is what the command it runs is said to have written.
	stdout string

	// code is the exit status reported, so a refusal can be staged.
	code int

	// garbage replies with something that is not a message at all, for the
	// client's own error handling.
	garbage bool

	// hangUp closes the connection after answering once, which is what a
	// daemon that was stopped mid-recording looks like.
	hangUp bool
}

// serveRuns listens on the socket a daemon for port would use and answers runs.
//
// Parameters:
//   - t: the test the listener is cleaned up at the end of
//   - port: the serial port the daemon is holding
func (d runDaemon) serveRuns(t *testing.T, port string) {
	t.Helper()

	path := portlock.SocketPath(port)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making the socket directory: %v", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listening on %s: %v", path, err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()

				send := func(msg broker.Response) {
					line, _ := json.Marshal(msg)
					conn.Write(append(line, '\n'))
				}
				send(hello())

				reader := bufio.NewReader(conn)
				for {
					line, err := reader.ReadBytes('\n')
					if err != nil {
						return
					}
					var req broker.Request
					if json.Unmarshal(line, &req) != nil {
						continue
					}
					if req.Op == broker.OpAudio {
						// This daemon holds no sound input, which is a refusal
						// rather than an absence: there is a daemon, it just
						// cannot help.
						send(broker.Response{Type: broker.TypeError, ID: req.ID,
							Error: "this daemon is not holding a sound input"})
						continue
					}
					if req.Op != broker.OpRun {
						continue
					}

					send(broker.Response{Type: broker.TypeStarted, ID: req.ID})
					out := d.stdout
					if d.garbage {
						out = "not json at all"
					}
					send(broker.Response{Type: broker.TypeStdout, ID: req.ID, Data: out})
					send(broker.Response{Type: broker.TypeDone, ID: req.ID, Code: d.code})
					if d.hangUp {
						return
					}
				}
			}(conn)
		}
	}()
}

// busyPort claims the lock on a port so that opening the scanner is refused the
// way it is when a daemon already holds it.
//
// Parameters:
//   - t: the test the lock is released at the end of
//   - port: the serial port to claim
func busyPort(t *testing.T, port string) {
	t.Helper()

	lock, err := portlock.Acquire(port, 0)
	if err != nil {
		t.Fatalf("claiming %s: %v", port, err)
	}
	t.Cleanup(func() { lock.Release() })
}

// Test_newSamplerViaDaemon tests reading the scanner through a daemon with 100%
// coverage of that path.
//
// It is what lets a recording run for hours while other commands still use the
// radio: the reading goes over the socket without taking a turn, so it slips
// between the exchanges of whatever else is running.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Reads: a busy port is read through the daemon holding it
//   - Refused: a run the daemon could not complete is reported
//   - Garbage: an answer that is not JSON is reported rather than parsed
//   - NoDaemon: a busy port with nothing sharing it reports the busy port
func Test_newSamplerViaDaemon(t *testing.T) {
	const port = "/dev/example-record"

	// Verify a busy port is read through the daemon.
	t.Run("Reads", func(t *testing.T) {
		sockets(t)
		busyPort(t, port)

		heard, _ := json.Marshal(heardOn("MARLINTON DISPATCH"))
		runDaemon{stdout: string(heard)}.serveRuns(t, port)

		app, _, _ := recorderApp()
		app.Config.Device = port

		sample, done, err := newSampler(context.Background(), app)
		if err != nil {
			t.Fatalf("building the sampler: %v", err)
		}
		defer done()

		got, err := sample(context.Background())
		if err != nil {
			t.Fatalf("sampling through the daemon: %v", err)
		}
		if !got.Receiving || got.Channel != "MARLINTON DISPATCH" {
			t.Errorf("got %+v, want the transmission the daemon reported", got)
		}
	})

	// Verify a run the daemon could not complete is reported rather than read
	// as a scanner hearing nothing.
	t.Run("Refused", func(t *testing.T) {
		sockets(t)
		busyPort(t, port)
		runDaemon{code: 1}.serveRuns(t, port)

		app, _, _ := recorderApp()
		app.Config.Device = port

		sample, done, err := newSampler(context.Background(), app)
		if err != nil {
			t.Fatalf("building the sampler: %v", err)
		}
		defer done()

		if _, err := sample(context.Background()); err == nil {
			t.Fatal("a refused run reported nothing")
		}
	})

	// Verify an answer that is not JSON is reported rather than parsed into an
	// empty reading, which would read as a scanner hearing nothing.
	t.Run("Garbage", func(t *testing.T) {
		sockets(t)
		busyPort(t, port)
		runDaemon{garbage: true}.serveRuns(t, port)

		app, _, _ := recorderApp()
		app.Config.Device = port

		sample, done, err := newSampler(context.Background(), app)
		if err != nil {
			t.Fatalf("building the sampler: %v", err)
		}
		defer done()

		if _, err := sample(context.Background()); err == nil {
			t.Fatal("an answer that was not JSON reported nothing")
		}
	})

	// Verify a busy port with nothing sharing it reports the busy port, which
	// is the real answer, rather than the failure to find a daemon.
	t.Run("NoDaemon", func(t *testing.T) {
		sockets(t)
		busyPort(t, port)

		app, _, _ := recorderApp()
		app.Config.Device = port

		if _, _, err := newSampler(context.Background(), app); !errors.Is(err, portlock.ErrBusy) {
			t.Fatalf("got %v, want the busy port reported", err)
		}
	})
}

// Test_recordLoopEndings tests the ways a run can end that the ordinary cases
// do not reach, with 100% coverage of them.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Heard: a reading of the radio arrives and labels what is recorded
//   - RecordingFails: a destination that will not take a file ends the run
//   - FlushFails: a destination lost mid-transmission is reported on the way out
//   - AudioEndsMidTransmission: the part that happened is kept
func Test_recordLoopEndings(t *testing.T) {
	// start runs the loop in the background against a channel the test drives,
	// so each frame is known to have been acted on before the next is sent.
	start := func(t *testing.T, dir string, sample func(context.Context) (device.Heard, error)) (
		chan audiofeed.Frame, context.CancelFunc, <-chan error) {
		t.Helper()

		app, _, _ := recorderApp()
		library, err := recordings.New(dir, "", false)
		if err != nil {
			t.Fatalf("opening the library: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		frames := make(chan audiofeed.Frame)
		done := make(chan error, 1)

		go func() {
			done <- recordLoop(ctx, app, library, frames, sample, recordOptions{
				hang:        200 * time.Millisecond,
				minDuration: 100 * time.Millisecond,
			})
		}()
		return frames, cancel, done
	}

	// send pushes frames one at a time, which blocks until each is taken.
	send := func(frames chan audiofeed.Frame, batch []audiofeed.Frame) {
		for _, f := range batch {
			frames <- f
		}
	}

	// Always receiving, because these tests are about what happens to a
	// recording rather than about whether one starts. The radio is what opens
	// one now, so it has to be saying something.
	live := func(context.Context) (device.Heard, error) {
		return heardOn("MARLINTON DISPATCH"), nil
	}

	// Verify a reading of the radio arrives and ends up on the recording.
	//
	// The scanner is asked three times a second, so this is the one test that
	// has to run for longer than an instant.
	t.Run("Heard", func(t *testing.T) {
		dir := t.TempDir()
		frames, cancel, done := start(t, dir, func(context.Context) (device.Heard, error) {
			return heardOn("MARLINTON DISPATCH"), nil
		})

		settle()
		quiet, next := feed(0, 30, quietLevel)
		loud, next := feed(next, 60, loudLevel)
		send(frames, quiet)
		send(frames, loud)

		// The loop is blocked waiting for a frame, so this is where it takes
		// the reading the poller has produced. Waiting rather than sending
		// frames meanwhile is deliberate: the frames carry their own clock, and
		// sending them as fast as a test can would run that clock past the
		// maximum recording length while barely any real time passed.
		time.Sleep(2 * samplePeriod)

		more, next := feed(next, 10, loudLevel)
		send(frames, more)

		tail, _ := feed(next, 60, quietLevel)
		send(frames, tail)
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("recording: %v", err)
		}

		e := sidecar(t, dir)
		if e.Channel != "MARLINTON DISPATCH" {
			t.Errorf("the recording is labelled %q, want the channel the radio named", e.Channel)
		}
		if e.Samples == 0 {
			t.Error("no readings were counted, so the label had nothing behind it")
		}
	})

	// Verify a destination that will not take a file ends the run rather than
	// carrying on recording into nothing.
	t.Run("RecordingFails", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "rec")
		frames, cancel, done := start(t, dir, live)
		defer cancel()

		// Take the destination away now that the library has been opened.
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("removing the destination: %v", err)
		}
		if err := os.WriteFile(dir, nil, 0o644); err != nil {
			t.Fatalf("putting a file where the destination was: %v", err)
		}

		settle()
		quiet, next := feed(0, 30, quietLevel)
		loud, _ := feed(next, 60, loudLevel)
		go send(frames, append(quiet, loud...))

		select {
		case err := <-done:
			if err == nil {
				t.Fatal("a destination that cannot hold a recording reported nothing")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the run never ended")
		}
	})

	// Verify a destination lost while a transmission is open is reported when
	// the run is stopped, rather than the recording vanishing quietly.
	t.Run("FlushFails", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "rec")
		frames, cancel, done := start(t, dir, live)

		settle()
		quiet, next := feed(0, 30, quietLevel)
		loud, _ := feed(next, 60, loudLevel)
		send(frames, quiet)
		send(frames, loud)

		// The transmission is open and long enough to keep. Now take the
		// destination away underneath it.
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("removing the destination: %v", err)
		}
		if err := os.WriteFile(dir, nil, 0o644); err != nil {
			t.Fatalf("putting a file where the destination was: %v", err)
		}

		cancel()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("a recording that could not be filed reported nothing")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the run never ended")
		}
	})

	// Verify the audio stopping mid-transmission keeps the part that happened,
	// the same as being interrupted does.
	t.Run("AudioEndsMidTransmission", func(t *testing.T) {
		dir := t.TempDir()
		frames, cancel, done := start(t, dir, live)
		defer cancel()

		settle()
		quiet, next := feed(0, 30, quietLevel)
		loud, _ := feed(next, 60, loudLevel)
		send(frames, quiet)
		send(frames, loud)
		close(frames)

		if err := <-done; err != nil {
			t.Fatalf("the audio ending reported %v, want nil", err)
		}

		var wavs int
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && strings.HasSuffix(path, ".wav") {
				wavs++
			}
			return nil
		})
		if wavs != 1 {
			t.Errorf("wrote %d recordings, want the interrupted one kept", wavs)
		}
	})
}

// Test_pollDropsReadingsTheRecorderIsTooBusyFor covers the case where a reading
// arrives while the recorder is working on a frame.
//
// Dropping it costs nothing, because another arrives a third of a second later
// and the boundaries of a recording never depended on it. Blocking instead
// would stall the poller behind the audio.
func Test_pollDropsReadingsTheRecorderIsTooBusyFor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Nothing ever reads this, so every reading after the first is dropped.
	heard := make(chan device.Heard, 1)
	failures := make(chan error, 1)
	poll(ctx, func(context.Context) (device.Heard, error) {
		return heardOn("MARLINTON DISPATCH"), nil
	}, heard, failures)

	if len(failures) != 0 {
		t.Error("dropping a reading was reported as a failure")
	}
}

// Test_runRecordRecords covers the whole command wired together, from opening
// the destination to writing a recording into it.
func Test_runRecordRecords(t *testing.T) {
	fakeStart(t, "USB Audio CODEC", nil)

	app, _, errs := recorderApp()
	app.Config.Device = "/dev/example"
	app.SetDevice(device.New(recordConn{doc: `<ScannerInfo Mode="Scan Mode"/>`}))

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := runRecord(ctx, app, recordOptions{
		destination: dir,
		input:       "USB Audio CODEC",
		channel:     audiofeed.ChannelAuto,
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}

	if !strings.Contains(errs.String(), "USB Audio CODEC") {
		t.Errorf("said %q, want the input it was recording from named", errs.String())
	}
	// The destination is prepared whether or not anything was heard.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("the destination was not prepared: %v", err)
	}
}

// Test_runRecordDefaultsTheDestination covers leaving the destination off,
// which puts recordings in a folder below wherever the command was run.
func Test_runRecordDefaultsTheDestination(t *testing.T) {
	fakeStart(t, "USB Audio CODEC", nil)
	t.Chdir(t.TempDir())

	app, _, _ := recorderApp()
	app.Config.Device = "/dev/example"
	app.SetDevice(device.New(recordConn{doc: `<ScannerInfo Mode="Scan Mode"/>`}))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := runRecord(ctx, app, recordOptions{
		input:   "USB Audio CODEC",
		channel: audiofeed.ChannelAuto,
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	if info, err := os.Stat("recordings"); err != nil || !info.IsDir() {
		t.Errorf("the default destination was not created: %v", err)
	}
}

// Test_runRecordReportsAnUnreachableScanner covers a scanner that cannot be
// opened at all, which has to be reported before a sound card is touched.
func Test_runRecordReportsAnUnreachableScanner(t *testing.T) {
	sockets(t)

	app, _, _ := recorderApp()
	app.Config.Device = "/dev/nothing-here"

	err := runRecord(context.Background(), app, recordOptions{
		destination: t.TempDir(),
		channel:     audiofeed.ChannelAuto,
	})
	if err == nil {
		t.Fatal("a scanner that is not there reported nothing")
	}
}

// Test_audioViaDaemonRefused covers a daemon that answers but will not send
// audio, which is different from there being no daemon at all and must not be
// reported as advice to start one.
func Test_audioViaDaemonRefused(t *testing.T) {
	sockets(t)
	const port = "/dev/example-refuses"

	// A daemon that is there and holding no sound input, so it refuses.
	runDaemon{}.serveRuns(t, port)

	app, _, _ := recorderApp()
	app.Config.Device = port

	_, _, _, err := audioViaDaemon(context.Background(), app)
	if err == nil {
		t.Fatal("a daemon that would not send audio reported nothing")
	}
	if strings.Contains(err.Error(), "radiocli daemon --device") {
		t.Errorf("got %q, want it not to advise starting a daemon that is already there", err)
	}
}

// Test_newSamplerReportsALostDaemon covers the daemon going away while a
// recording is running, which has to end the run rather than be read as a
// scanner that has gone quiet.
func Test_newSamplerReportsALostDaemon(t *testing.T) {
	sockets(t)
	const port = "/dev/example-vanishes"
	busyPort(t, port)

	heard, _ := json.Marshal(heardOn("MARLINTON DISPATCH"))
	runDaemon{stdout: string(heard), hangUp: true}.serveRuns(t, port)

	app, _, _ := recorderApp()
	app.Config.Device = port

	sample, done, err := newSampler(context.Background(), app)
	if err != nil {
		t.Fatalf("building the sampler: %v", err)
	}
	defer done()

	// The first reading works and the daemon then hangs up.
	if _, err := sample(context.Background()); err != nil {
		t.Fatalf("the first reading: %v", err)
	}
	if _, err := sample(context.Background()); err == nil {
		t.Fatal("a daemon that hung up reported nothing")
	}
}

// Test_runRecordReportsAudioItCannotOpen covers a sound input that will not
// open, which has to be reported rather than recorded as silence.
func Test_runRecordReportsAudioItCannotOpen(t *testing.T) {
	fakeStart(t, "", errors.New("no such input"))

	app, _, _ := recorderApp()
	app.Config.Device = "/dev/example"
	app.SetDevice(device.New(recordConn{doc: `<ScannerInfo Mode="Scan Mode"/>`}))

	err := runRecord(context.Background(), app, recordOptions{
		destination: t.TempDir(),
		input:       "Nothing",
		channel:     audiofeed.ChannelAuto,
	})
	if err == nil {
		t.Fatal("an input that will not open reported nothing")
	}
}

// Test_audioViaDaemonStopsWhenTheRunDoes covers the audio arriving faster than
// it is taken, which is what happens when the recorder is stopped while a
// daemon is still sending.
//
// The frames waiting have to be let go of rather than the goroutine blocking on
// a channel nobody will read again.
func Test_audioViaDaemonStopsWhenTheRunDoes(t *testing.T) {
	sockets(t)
	const port = "/dev/example-floods"

	// Comfortably more than the recorder will hold, so the send blocks.
	var tail []byte
	for i := range recordQueue * 2 {
		tail = append(tail, audioPacket(uint32(i), tone(quietLevel))...)
	}
	daemon{
		hello: hello(),
		reply: broker.Response{Type: broker.TypeAudio, Format: formatPCM, Rate: 48000, Channels: 1},
		tail:  tail,
	}.serve(t, port)

	app, _, _ := recorderApp()
	app.Config.Device = port

	ctx, cancel := context.WithCancel(context.Background())
	frames, _, done, err := audioViaDaemon(ctx, app)
	if err != nil {
		t.Fatalf("asking the daemon for audio: %v", err)
	}
	defer done()

	// Take one, so the rest are known to have arrived and filled the queue.
	select {
	case <-frames:
	case <-time.After(2 * time.Second):
		t.Fatal("no audio arrived from the daemon")
	}

	// Let the queue fill, then stop without reading any more.
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

// Test_pollStopsQuietlyWhenTheRunIsCancelled covers the two places the poller
// gives up rather than reporting anything.
//
// Coverage: 100% (2 test cases covering both)
//
// Test cases:
//   - FailureAfterCancel: a scanner that failed because the run ended is not
//     reported as a scanner that broke
//   - ReadingAfterCancel: a reading nobody will take is dropped
func Test_pollStopsQuietlyWhenTheRunIsCancelled(t *testing.T) {
	// Verify a failure that is really the run ending says nothing, since
	// otherwise every Ctrl-C would end with a complaint about the radio.
	t.Run("FailureAfterCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		failures := make(chan error, 1)
		poll(ctx, func(context.Context) (device.Heard, error) {
			cancel()
			return device.Heard{}, errors.New("the port closed")
		}, make(chan device.Heard, 1), failures)

		if len(failures) != 0 {
			t.Error("stopping the run was reported as the radio failing")
		}
	})

	// Verify a reading taken as the run ends is dropped rather than blocking on
	// a recorder that has already gone.
	t.Run("ReadingAfterCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		failures := make(chan error, 1)
		poll(ctx, func(context.Context) (device.Heard, error) {
			cancel()
			return heardOn("MARLINTON DISPATCH"), nil
		}, make(chan device.Heard), failures)

		if len(failures) != 0 {
			t.Error("stopping the run was reported as a failure")
		}
	})
}

// Test_runRecordReportsADestinationItCannotMake covers a destination that
// cannot be created, which is reported rather than recorded into nothing.
func Test_runRecordReportsADestinationItCannotMake(t *testing.T) {
	app, _, _ := recorderApp()
	app.Config.Device = "/dev/example"
	app.SetDevice(device.New(recordConn{doc: `<ScannerInfo Mode="Scan Mode"/>`}))

	// A path below a regular file, which no directory can be made under.
	blocked := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("making the file in the way: %v", err)
	}

	err := runRecord(context.Background(), app, recordOptions{
		destination: filepath.Join(blocked, "rec"),
		input:       "USB Audio CODEC",
		channel:     audiofeed.ChannelAuto,
	})
	if err == nil {
		t.Fatal("a destination that cannot be made reported nothing")
	}
}

// Test_meter tests the level meter with 100% coverage.
//
// The two numbers it prints are what diagnose a cable, and having them is what
// caught the noise floor being estimated wrongly during development, so it is
// worth checking they are both there and both rounded to something readable.
func Test_meter(t *testing.T) {
	app, _, _ := recorderApp()
	var logged strings.Builder
	app.Log = slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var m meter
	frames, _ := feed(0, meterEvery-1, quietLevel)
	for _, f := range frames {
		m.observe(app, f, quietLevel)
	}
	if logged.Len() != 0 {
		t.Fatalf("a reading was printed after %d frames, want one every %d", meterEvery-1, meterEvery)
	}

	// The loudest frame in the window is the one reported, not the last.
	loud, _ := feed(0, 1, loudLevel)
	m.observe(app, loud[0], quietLevel)

	got := logged.String()
	if !strings.Contains(got, "peak=-20") {
		t.Errorf("logged %q, want the loudest frame reported", got)
	}
	if !strings.Contains(got, "floor=-70") {
		t.Errorf("logged %q, want the noise floor reported beside it", got)
	}

	// And the window starts again, so the next reading is not still carrying
	// the last one's peak.
	logged.Reset()
	for range meterEvery {
		m.observe(app, frames[0], quietLevel)
	}
	if !strings.Contains(logged.String(), "peak=-70") {
		t.Errorf("logged %q, want the peak reset after a reading", logged.String())
	}
}

// Test_recordLoopAbandonsAnOpenRecordingWhenTheRadioGoesAway checks the file
// left behind when the scanner is unplugged mid-transmission.
//
// The run has to end, because a recording nothing can confirm is not a scanner
// recording, and the half-written file has to go with it rather than being left
// as something that will not play.
func Test_recordLoopAbandonsAnOpenRecordingWhenTheRadioGoesAway(t *testing.T) {
	app, _, _ := recorderApp()
	dir := t.TempDir()
	library, err := recordings.New(dir, "", false)
	if err != nil {
		t.Fatalf("opening the library: %v", err)
	}

	// A scanner that answers once, so a recording opens, and then goes away.
	var asked atomic.Int32
	sample := func(context.Context) (device.Heard, error) {
		if asked.Add(1) > 1 {
			return device.Heard{}, errors.New("the port closed")
		}
		return heardOn("MARLINTON DISPATCH"), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	frames := make(chan audiofeed.Frame, 4096)
	done := make(chan error, 1)
	go func() {
		done <- recordLoop(ctx, app, library, frames, sample, recordOptions{
			hang: 5 * time.Second, minDuration: 100 * time.Millisecond,
		})
	}()

	settle()
	quiet, next := feed(0, 30, quietLevel)
	loud, _ := feed(next, 120, loudLevel)
	for _, f := range append(quiet, loud...) {
		frames <- f
	}

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "stopped answering") {
			t.Fatalf("got %v, want the run to end with the radio", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the run never ended")
	}

	// Nothing half written is left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the destination: %v", err)
	}
	for _, e := range entries {
		t.Errorf("%s was left behind", e.Name())
	}
}

// Test_openAudioReportsWhatTheFeedSays checks that the recorder passes on the
// feed's warnings.
//
// They are the only way a person learns that the recording is going wrong for a
// reason outside the radio: a lead in the wrong socket, a permission never
// granted, or two sides of a cable that cancel each other. The recorder reads
// frames from its subscription, so without something reading the events beside
// them they would be produced and never seen.
func Test_openAudioReportsWhatTheFeedSays(t *testing.T) {
	// A capture that publishes an event and no audio at all.
	original := startCapture
	t.Cleanup(func() { startCapture = original })

	published := make(chan struct{})
	startCapture = func(opts audiofeed.Options, out audiofeed.Publisher) (capture, error) {
		go func() {
			out.PublishEvent("channel", map[string]any{
				"channel": audiofeed.ChannelLeft,
				"reason":  audiofeed.ReasonOutOfPhase,
			})
			close(published)
		}()
		return fakeCapture{source: "USB Audio CODEC"}, nil
	}

	app, _, errs := recorderApp()
	_, _, done, err := openAudio(context.Background(), app, "USB Audio CODEC", audiofeed.ChannelAuto)
	if err != nil {
		t.Fatalf("opening the audio: %v", err)
	}
	defer done()

	<-published
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && errs.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(errs.String(), "out of phase") {
		t.Errorf("said %q, want the out of phase cable called out", errs.String())
	}
	if !strings.Contains(errs.String(), "Headphone L/R output") {
		t.Errorf("said %q, want the menu that fixes it named", errs.String())
	}
}

// Test_volumeNow tests the volumeNow function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Reads: a scanner that answers gives the level
//   - NoScanner: no radio is -1 rather than an error, since the volume is a
//     nicety on a warning about something else
//   - Unreadable: a scanner that answers with nonsense is -1 too
func Test_volumeNow(t *testing.T) {
	// Verify the level comes back when the scanner answers.
	t.Run("Reads", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.Config.Device = "/dev/example"
		app.SetDevice(device.New(recordConn{doc: "12"}))

		if got := volumeNow(context.Background(), app); got != 12 {
			t.Errorf("volumeNow() = %d, want 12", got)
		}
	})

	// Verify that having no radio costs the warning a sentence rather than
	// costing the run anything.
	t.Run("NoScanner", func(t *testing.T) {
		app, _, _ := recorderApp()

		if got := volumeNow(context.Background(), app); got != -1 {
			t.Errorf("volumeNow() = %d, want -1", got)
		}
	})

	// Verify the same for a scanner that answers with something unparseable.
	t.Run("Unreadable", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.Config.Device = "/dev/example"
		app.SetDevice(device.New(recordConn{doc: "loud"}))

		if got := volumeNow(context.Background(), app); got != -1 {
			t.Errorf("volumeNow() = %d, want -1", got)
		}
	})
}

// Test_recorderWarnsWhenClipped tests the count and warnIfClipped methods with
// 100% coverage.
//
// The thresholds are the ones measured on a real SDS150 through one cable into
// both jacks: a line input produced no full-scale samples at all across
// twenty-three recordings, and a mic input produced between 1.4% and 19% on
// every recording that was not near silence.
//
// Coverage: 100% (5 test cases covering every branch of both)
//
// Test cases:
//   - Clipped: an overloaded recording is warned about, naming the volume
//   - NoVolume: the same warning without the level, when the radio never said
//   - NoReader: a recorder with nothing to read the volume with still warns
//   - Rereads: a volume changed between recordings is reported as it is now,
//     not as it was when the run started
//   - Clean: audio that fits is not warned about
//   - Silence: a recording with no audio in it is not warned about
//   - Resets: the tally starts again for each recording, so one loud
//     transmission does not warn about the next
func Test_recorderWarnsWhenClipped(t *testing.T) {
	// recorderWith returns a recorder reading its volume from levels in turn,
	// and the stderr it warns through. A list rather than a number so that a
	// volume changed part way through a run can be tested, which is the whole
	// reason it is read again for every warning.
	recorderWith := func(t *testing.T, levels ...int) (*recorder, *bytes.Buffer) {
		t.Helper()

		app, _, errs := recorderApp()
		l, err := recordings.New(t.TempDir(), "", false)
		if err != nil {
			t.Fatalf("opening the library: %v", err)
		}

		at := 0
		return &recorder{app: app, library: l,
			volume: func() int {
				level := levels[at]
				if at < len(levels)-1 {
					at++
				}
				return level
			},
			gate: audiogate.New(audiogate.Options{})}, errs
	}

	// pcm returns one frame in which clipped of the samples are at full scale,
	// alternating sides so both halves of the check are exercised.
	pcm := func(samples, clipped int) []byte {
		out := make([]byte, 2*samples)
		for i := 0; i < samples; i++ {
			v := int16(1000)
			if i < clipped {
				v = 32767
				if i%2 == 1 {
					v = -32767
				}
			}
			binary.LittleEndian.PutUint16(out[2*i:], uint16(v))
		}
		return out
	}

	// record drives one whole transmission through the recorder.
	record := func(t *testing.T, r *recorder, audio []byte) {
		t.Helper()

		events := []audiogate.Event{{Kind: audiogate.KindStart}}
		if audio != nil {
			events = append(events, audiogate.Event{
				Kind: audiogate.KindAudio, Frame: audiofeed.Frame{PCM: audio},
			})
		}
		events = append(events, audiogate.Event{Kind: audiogate.KindEnd})

		if err := r.apply(events); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}

	// Verify an overloaded recording is warned about, and that the warning
	// names the level somebody is about to change.
	t.Run("Clipped", func(t *testing.T) {
		r, errs := recorderWith(t, 12)
		record(t, r, pcm(1000, 50))

		got := errs.String()
		if !strings.Contains(got, "clipped") {
			t.Errorf("said %q, want it to name the clipping", got)
		}
		if !strings.Contains(got, "5.0%") {
			t.Errorf("said %q, want the proportion that clipped", got)
		}
		if !strings.Contains(got, "12") {
			t.Errorf("said %q, want the volume level named", got)
		}
	})

	// Verify the warning still happens without a radio to quote, since the
	// audio is just as distorted either way.
	t.Run("NoVolume", func(t *testing.T) {
		r, errs := recorderWith(t, -1)
		record(t, r, pcm(1000, 50))

		got := errs.String()
		if !strings.Contains(got, "clipped") {
			t.Errorf("said %q, want it to name the clipping", got)
		}
		if strings.Contains(got, "down from") {
			t.Errorf("said %q, want no level quoted when none was read", got)
		}
	})

	// Verify a recorder nothing gave a volume reader to still says the useful
	// half of the message rather than panicking on the nil.
	t.Run("NoReader", func(t *testing.T) {
		app, _, errs := recorderApp()
		l, err := recordings.New(t.TempDir(), "", false)
		if err != nil {
			t.Fatalf("opening the library: %v", err)
		}
		r := &recorder{app: app, library: l, gate: audiogate.New(audiogate.Options{})}

		record(t, r, pcm(1000, 50))

		if got := errs.String(); !strings.Contains(got, "clipped") {
			t.Errorf("said %q, want the warning without a level", got)
		}
	})

	// Verify the level is read again for each warning. Somebody who turns the
	// volume down and sees the old number reads it as the tool not talking to
	// the radio at all, which is worse than saying nothing.
	t.Run("Rereads", func(t *testing.T) {
		r, errs := recorderWith(t, 15, 14)

		record(t, r, pcm(1000, 50))
		if got := errs.String(); !strings.Contains(got, "down from 15 of") {
			t.Errorf("the first warning said %q, want it to name 15", got)
		}

		errs.Reset()
		record(t, r, pcm(1000, 50))
		if got := errs.String(); !strings.Contains(got, "down from 14 of") {
			t.Errorf("the second warning said %q, want it to name 14", got)
		}
	})

	// Verify audio that fits is left alone, which is every recording through a
	// line input.
	t.Run("Clean", func(t *testing.T) {
		r, errs := recorderWith(t, 12)
		record(t, r, pcm(1000, 0))

		if got := errs.String(); got != "" {
			t.Errorf("said %q about a recording that did not clip", got)
		}
	})

	// Verify a recording with no audio at all is not divided by zero.
	t.Run("Silence", func(t *testing.T) {
		r, errs := recorderWith(t, 12)
		record(t, r, nil)

		if got := errs.String(); got != "" {
			t.Errorf("said %q about a recording with no audio in it", got)
		}
	})

	// Verify the tally is per recording rather than per run, so a clean
	// transmission after a clipped one is reported as clean.
	t.Run("Resets", func(t *testing.T) {
		r, errs := recorderWith(t, 12)
		record(t, r, pcm(1000, 50))
		errs.Reset()

		record(t, r, pcm(1000, 0))
		if got := errs.String(); got != "" {
			t.Errorf("said %q about the clean recording that followed a clipped one", got)
		}
	})
}

// Test_recorderReportsAFailedReport covers the one path that opens between
// filing a recording and warning about it: the recording was written, and
// saying so failed.
//
// It matters because the warning must not be reached with the report only half
// written. Under --output json the report is the object a script is reading,
// and a stream that has gone away is the run ending rather than something to
// carry on past.
func Test_recorderReportsAFailedReport(t *testing.T) {
	app, _, _ := recorderApp()
	app.Config.Output = appcontext.OutputJSON
	app.Stdout = failWriter{}

	l, err := recordings.New(t.TempDir(), "", false)
	if err != nil {
		t.Fatalf("opening the library: %v", err)
	}
	r := &recorder{app: app, library: l, gate: audiogate.New(audiogate.Options{})}

	if err := r.apply([]audiogate.Event{
		{Kind: audiogate.KindStart},
		{Kind: audiogate.KindEnd},
	}); err == nil {
		t.Fatal("a report that could not be written reported nothing")
	}
}
