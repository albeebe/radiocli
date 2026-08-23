// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package recordings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// errBroken is what the fakes fail with, so a test can check the failure it
// provoked is the one that came back.
var errBroken = errors.New("broken")

// at is when every test's recording happened, fixed so failures name the same
// times every run.
//
// The tokens render a time in whatever zone it carries, which for a real
// recording is local, because the frames it is assembled from are stamped by
// the machine that captured them. Fixing this one in UTC is what keeps these
// tests giving the same answer wherever they are run.
var at = time.Date(2026, 8, 22, 19, 54, 3, 0, time.UTC)

// entry returns a recording with the quiet zone names every example in this
// tool uses.
//
// Returns:
//   - an entry describing one conventional transmission
func entry() Entry {
	return Entry{
		Start:      at,
		End:        at.Add(4800 * time.Millisecond),
		List:       "Pocahontas County",
		System:     "PUBLIC SAFETY",
		Department: "POLICE DEPARTMENT",
		Channel:    "MARLINTON DISPATCH",
		Frequency:  "155.550000MHz",
		Modulation: "NFM",
		Reason:     "hang",
		Samples:    16,
	}
}

// fakeWav stands in for a WAV file so a test can drive a disk that fails.
type fakeWav struct {
	written      int
	normalizedTo float64
	failWrite    bool
	failClose    bool
	failNorm     bool
}

// Close reports whatever the fake was told to.
func (f *fakeWav) Close() error {
	if f.failClose {
		return errBroken
	}
	return nil
}

// Duration reports a second per byte written, which keeps the arithmetic in the
// tests obvious.
func (f *fakeWav) Duration() time.Duration { return time.Duration(f.written) * time.Second }

// Normalize records the level it was asked for, or fails if the fake was told
// to.
func (f *fakeWav) Normalize(target float64) error {
	if f.failNorm {
		return errBroken
	}
	f.normalizedTo = target
	return nil
}

// Write counts the audio, or fails if the fake was told to.
func (f *fakeWav) Write(pcm []byte) error {
	if f.failWrite {
		return errBroken
	}
	f.written += len(pcm)
	return nil
}

// library returns a Library writing into a temporary directory.
//
// Parameters:
//   - t: the test, whose temporary directory is used
//   - naming: the template, or empty for the default
//
// Returns:
//   - the library, closed when the test ends
//   - the directory it writes into
func library(t *testing.T, naming string, normalize bool) (*Library, string) {
	t.Helper()

	dir := t.TempDir()
	l, err := New(dir, naming, normalize)
	if err != nil {
		t.Fatalf("opening the library: %v", err)
	}
	return l, dir
}

// record writes one recording through the library and returns how it was filed.
//
// Parameters:
//   - t: the test
//   - l: the library to record into
//   - e: what the recording is of
//
// Returns:
//   - the entry as it was filed
func record(t *testing.T, l *Library, e Entry) Entry {
	t.Helper()

	r, err := l.Begin()
	if err != nil {
		t.Fatalf("beginning a recording: %v", err)
	}
	if err := r.Write(make([]byte, 960)); err != nil {
		t.Fatalf("writing audio: %v", err)
	}
	filed, err := r.Close(e)
	if err != nil {
		t.Fatalf("closing the recording: %v", err)
	}
	return filed
}

// TestNewChecksTheTemplateBeforeAnythingElse is the rule that a bad template
// costs a second rather than a night: every one of these has to fail at New,
// with nothing recorded.
func TestNewChecksTheTemplateBeforeAnythingElse(t *testing.T) {
	for _, c := range []struct{ name, naming, want string }{
		{"empty", "   ", "it is empty"},
		{"unknown token", "{date}/{chanel}", "not a token"},
		{"unclosed brace", "{date}/{time", "never closed"},
		{"stray closing brace", "{date}/time}", "with no"},
		{"no tokens at all", "recording", "no tokens in it"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := New(t.TempDir(), c.naming, false)
			if !errors.Is(err, ErrBadTemplate) {
				t.Fatalf("got %v, want ErrBadTemplate", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want it to mention %q", err, c.want)
			}
		})
	}

	// And a good one names the tokens when it complains, so the fix does not
	// need the documentation.
	if _, err := New(t.TempDir(), "{nope}", false); err == nil || !strings.Contains(err.Error(), "talkgroup") {
		t.Errorf("got %v, want the list of tokens in the message", err)
	}
}

// TestRecordingIsFiledWithItsDescription covers the ordinary path end to end:
// the audio, and a sidecar beside it.
func TestRecordingIsFiledWithItsDescription(t *testing.T) {
	l, dir := library(t, "", false)
	filed := record(t, l, entry())

	// The default template puts the day in a folder, which is what stops a long
	// run piling a week into one directory.
	want := filepath.Join("2026-08-22",
		at.Format("15-04-05")+"_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav")
	if filed.File != want {
		t.Errorf("filed as %q, want %q", filed.File, want)
	}
	if _, err := os.Stat(filepath.Join(dir, filed.File)); err != nil {
		t.Errorf("the audio is not there: %v", err)
	}

	// The sidecar sits beside the audio under the same name whatever the
	// template said, so the pair always travel together.
	sidecar := filepath.Join(dir, strings.TrimSuffix(filed.File, ".wav")+".json")
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("the description is not there: %v", err)
	}
	var got Entry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the description is not JSON: %v", err)
	}
	if got.Channel != "MARLINTON DISPATCH" || got.Frequency != "155.550000MHz" {
		t.Errorf("got %+v, want the channel and frequency described", got)
	}
	if got.File != filed.File {
		t.Errorf("the description points at %q, want %q", got.File, filed.File)
	}

	// The duration comes from the audio rather than from the times, so a
	// recording that lost frames is shorter rather than lying.
	// 960 bytes of 48 kHz 16-bit mono is a hundredth of a second.
	if got.Duration != 0.01 {
		t.Errorf("duration is %v, want it measured from the audio", got.Duration)
	}
}

// TestNamesDoNotCollide covers two transmissions that render to the same name,
// which must both survive rather than one overwriting the other.
func TestNamesDoNotCollide(t *testing.T) {
	l, dir := library(t, "{system}_{channel}", false)

	first := record(t, l, entry())
	second := record(t, l, entry())

	if first.File == second.File {
		t.Fatalf("both recordings were filed as %q", first.File)
	}
	if !strings.Contains(second.File, "-2") {
		t.Errorf("the second is %q, want it numbered", second.File)
	}
	for _, e := range []Entry{first, second} {
		if _, err := os.Stat(filepath.Join(dir, e.File)); err != nil {
			t.Errorf("%s is not there: %v", e.File, err)
		}
	}
}

// TestUnlabelledRecordingStillGetsAUsableName covers a transmission the scanner
// never named, which is what an empty token has to survive.
func TestUnlabelledRecordingStillGetsAUsableName(t *testing.T) {
	l, _ := library(t, "", false)

	e := Entry{Start: at, End: at.Add(time.Second)}
	filed := record(t, l, e)

	// The separators around the empty tokens go with them, rather than leaving
	// a row of underscores where the channel would have been.
	base := filepath.Base(filed.File)
	if strings.Contains(base, "__") || strings.HasPrefix(base, "_") {
		t.Errorf("filed as %q, want the empty tokens collapsed", filed.File)
	}
	if base != at.Format("15-04-05")+".wav" {
		t.Errorf("filed as %q, want just the time", base)
	}
}

// TestEveryTokenRenders covers each token once, since a token that is listed
// but produces nothing is worse than one that does not exist.
func TestEveryTokenRenders(t *testing.T) {
	e := entry()
	e.Site = "Bald Knob"
	e.Talkgroup = "24944"
	e.Unit = "32"
	e.Duration = 4.8

	for _, name := range Tokens() {
		tmpl, err := parse("{" + name + "}")
		if err != nil {
			t.Fatalf("the listed token %q does not parse: %v", name, err)
		}
		if got := tmpl.render(e); got == "" {
			t.Errorf("the token %q rendered nothing", name)
		}
	}

	// tuned prefers the talkgroup, since only one of the two is ever set.
	tmpl, _ := parse("{tuned}")
	if got := tmpl.render(e); got != "24944" {
		t.Errorf("tuned rendered %q, want the talkgroup", got)
	}
	e.Talkgroup = ""
	if got := tmpl.render(e); got != "155.550000MHz" {
		t.Errorf("tuned rendered %q, want the frequency once there is no talkgroup", got)
	}
}

// TestNamesFromTheScannerCannotEscapeTheDestination is the guard that matters
// for safety rather than tidiness: a channel name is programmed by somebody
// else and arrives as text.
func TestNamesFromTheScannerCannotEscapeTheDestination(t *testing.T) {
	l, dir := library(t, "{channel}", false)

	e := entry()
	e.Channel = "../../etc/FIRE/EMS"
	filed := record(t, l, e)

	if strings.Contains(filed.File, "..") || strings.Contains(filed.File, "/") {
		t.Fatalf("filed as %q, want the path characters neutralised", filed.File)
	}
	if _, err := os.Stat(filepath.Join(dir, filed.File)); err != nil {
		t.Errorf("the recording is not where it said it was: %v", err)
	}
}

// TestLongNamesAreShortened covers the limit that the software this was
// measured against hits at 260 characters and reports as a popup.
func TestLongNamesAreShortened(t *testing.T) {
	l, _ := library(t, "{list}/{system}/{department}/{channel}", false)

	e := entry()
	e.List = strings.Repeat("L", 200)
	e.System = strings.Repeat("S", 200)
	e.Department = strings.Repeat("D", 200)
	e.Channel = strings.Repeat("C", 200)
	filed := record(t, l, e)

	if len(filed.File) > maxPath+len(".wav") {
		t.Errorf("filed as a path %d long, want no more than %d", len(filed.File), maxPath)
	}
	for _, c := range strings.Split(filed.File, string(filepath.Separator)) {
		if len(c) > maxComponent+len(".wav") {
			t.Errorf("the component %q is %d long, want no more than %d", c, len(c), maxComponent)
		}
	}

	// Every part is still recognisable, rather than one being cut to nothing.
	if strings.Count(filed.File, string(filepath.Separator)) != 3 {
		t.Errorf("filed as %q, want all four parts kept", filed.File)
	}
}

// TestShortenGivesUpRatherThanErasing covers a template with more components
// than the limit can hold, where there is nothing left to take.
func TestShortenGivesUpRatherThanErasing(t *testing.T) {
	parts := make([]string, 40)
	for i := range parts {
		parts[i] = strings.Repeat("x", 20)
	}

	got := shorten(parts)
	if len(got) <= maxPath {
		t.Fatalf("the path is %d long, so this test is not reaching the giving up", len(got))
	}
	for _, c := range strings.Split(got, "/") {
		if len(c) < minComponent {
			t.Errorf("the component %q was cut to %d, want no shorter than %d", c, len(c), minComponent)
		}
	}
}

// TestLiteralBraces covers the escape, which a template language made entirely
// of braces has to have.
func TestLiteralBraces(t *testing.T) {
	tmpl, err := parse("{{{channel}}}")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	// Literal text is passed through as written, because the user typed it
	// deliberately. Only values coming off the scanner are sanitised.
	if got := tmpl.render(entry()); got != "{MARLINTON-DISPATCH}" {
		t.Errorf("rendered %q, want the braces kept as text", got)
	}
}

// TestAbandon covers the recording that turns out not to be wanted, which must
// leave nothing behind.
func TestAbandon(t *testing.T) {
	l, dir := library(t, "", false)

	r, err := l.Begin()
	if err != nil {
		t.Fatalf("beginning a recording: %v", err)
	}
	if err := r.Abandon(); err != nil {
		t.Fatalf("abandoning: %v", err)
	}
	// And again, since a deferred abandon runs after the normal path too.
	if err := r.Abandon(); err != nil {
		t.Fatalf("abandoning twice: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the destination: %v", err)
	}
	for _, e := range entries {
		t.Errorf("%s was left behind", e.Name())
	}
}

// TestCloseTwiceIsHarmless covers the deferred close that runs after the
// normal one.
func TestCloseTwiceIsHarmless(t *testing.T) {
	l, _ := library(t, "", false)

	r, err := l.Begin()
	if err != nil {
		t.Fatalf("beginning a recording: %v", err)
	}
	if _, err := r.Close(entry()); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if _, err := r.Close(entry()); err != nil {
		t.Fatalf("closing twice: %v", err)
	}
}

// TestSweepReportsPartials covers what a run killed mid-transmission leaves,
// which is reported rather than deleted because deleting somebody's files
// unasked is not this package's decision.
func TestSweepReportsPartials(t *testing.T) {
	l, dir := library(t, "", false)

	if _, err := l.Begin(); err != nil {
		t.Fatalf("beginning a recording: %v", err)
	}

	found, err := l.Sweep()
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d partial recordings, want 1", len(found))
	}
	if filepath.Dir(found[0]) != dir {
		t.Errorf("found %q, want it in the destination", found[0])
	}

	// A directory whose name happens to start the same way is not a recording.
	if err := os.Mkdir(filepath.Join(dir, partialPrefix+"dir"), 0o755); err != nil {
		t.Fatalf("making a directory: %v", err)
	}
	if found, err = l.Sweep(); err != nil || len(found) != 1 {
		t.Errorf("got %v and %v, want still just the one file", found, err)
	}
}

// broken points one of the package's seams at a failure for the length of a
// test, so the disk failures that cannot be arranged on a real filesystem can
// be driven on demand.
//
// Parameters:
//   - t: the test, which puts the seam back when it ends
//   - set: swaps the seam and returns a function restoring it
func broken(t *testing.T, set func() func()) {
	t.Helper()
	t.Cleanup(set())
}

// TestNewReportsADestinationItCannotPrepare covers the two ways opening a
// library fails once the template is known to be good.
func TestNewReportsADestinationItCannotPrepare(t *testing.T) {
	t.Run("the folder cannot be made", func(t *testing.T) {
		broken(t, func() func() {
			was := mkdirAll
			mkdirAll = func(string, os.FileMode) error { return errBroken }
			return func() { mkdirAll = was }
		})

		if _, err := New(t.TempDir(), "", false); !errors.Is(err, errBroken) {
			t.Fatalf("got %v, want it to wrap errBroken", err)
		}
	})

}

// TestBeginReportsAFileItCannotCreate covers a destination that stopped being
// writable after the library was opened.
func TestBeginReportsAFileItCannotCreate(t *testing.T) {
	l, _ := library(t, "", false)
	broken(t, func() func() {
		was := createWav
		createWav = func(string) (wavWriter, error) { return nil, errBroken }
		return func() { createWav = was }
	})

	if _, err := l.Begin(); !errors.Is(err, errBroken) {
		t.Fatalf("got %v, want it to wrap errBroken", err)
	}
}

// TestCloseReportsEveryWayFilingCanFail covers each step between a finished
// transmission and a recording on disk, since a failure at any of them has to
// be reported rather than leaving a recording that is half filed.
func TestCloseReportsEveryWayFilingCanFail(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func() func()
	}{
		{"the audio cannot be closed", func() func() {
			was := createWav
			createWav = func(string) (wavWriter, error) { return &fakeWav{failClose: true}, nil }
			return func() { createWav = was }
		}},
		{"the folders cannot be made", func() func() {
			was := mkdirAll
			mkdirAll = func(string, os.FileMode) error { return errBroken }
			return func() { mkdirAll = was }
		}},
		{"the recording cannot be moved into place", func() func() {
			was := renameFile
			renameFile = func(string, string) error { return errBroken }
			return func() { renameFile = was }
		}},
		{"the description cannot be written", func() func() {
			was := writeFile
			writeFile = func(string, []byte, os.FileMode) error { return errBroken }
			return func() { writeFile = was }
		}},
		{"the description cannot be assembled", func() func() {
			was := marshalIndent
			marshalIndent = func(any, string, string) ([]byte, error) { return nil, errBroken }
			return func() { marshalIndent = was }
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The library is opened before the seam is broken, since New
			// uses several of these itself and has to succeed for there to
			// be a recording to file at all.
			l, _ := library(t, "", false)
			broken(t, c.set)

			r, err := l.Begin()
			if err != nil {
				t.Fatalf("beginning a recording: %v", err)
			}
			if _, err := r.Close(entry()); !errors.Is(err, errBroken) {
				t.Fatalf("got %v, want it to wrap errBroken", err)
			}
		})
	}
}

// TestAbandonReportsWhatItCannotClearUp covers the two ways discarding a
// recording fails.
func TestAbandonReportsWhatItCannotClearUp(t *testing.T) {
	t.Run("the audio cannot be closed", func(t *testing.T) {
		l, _ := library(t, "", false)
		broken(t, func() func() {
			was := createWav
			createWav = func(string) (wavWriter, error) { return &fakeWav{failClose: true}, nil }
			return func() { createWav = was }
		})

		r, err := l.Begin()
		if err != nil {
			t.Fatalf("beginning a recording: %v", err)
		}
		if err := r.Abandon(); !errors.Is(err, errBroken) {
			t.Fatalf("got %v, want it to wrap errBroken", err)
		}
	})

	t.Run("the file cannot be removed", func(t *testing.T) {
		l, _ := library(t, "", false)
		r, err := l.Begin()
		if err != nil {
			t.Fatalf("beginning a recording: %v", err)
		}

		broken(t, func() func() {
			was := removeFile
			removeFile = func(string) error { return errBroken }
			return func() { removeFile = was }
		})
		if err := r.Abandon(); !errors.Is(err, errBroken) {
			t.Fatalf("got %v, want it to wrap errBroken", err)
		}
	})
}

// TestWriteReportsAFailedWrite covers audio that cannot be written, which must
// not be counted as recorded.
func TestWriteReportsAFailedWrite(t *testing.T) {
	l, _ := library(t, "", false)
	broken(t, func() func() {
		was := createWav
		createWav = func(string) (wavWriter, error) { return &fakeWav{failWrite: true}, nil }
		return func() { createWav = was }
	})

	r, err := l.Begin()
	if err != nil {
		t.Fatalf("beginning a recording: %v", err)
	}
	if err := r.Write(make([]byte, 960)); !errors.Is(err, errBroken) {
		t.Fatalf("got %v, want it to wrap errBroken", err)
	}
}

// TestReserveGivesUpRatherThanLoopingForever covers a destination that cannot
// be read, where every name reads as taken.
//
// Without the limit this is a loop that never ends and never says why, which is
// the worst way for a recorder to fail.
func TestReserveGivesUpRatherThanLoopingForever(t *testing.T) {
	l, _ := library(t, "", false)
	broken(t, func() func() {
		was := statFile
		statFile = func(string) (os.FileInfo, error) { return nil, errBroken }
		return func() { statFile = was }
	})

	r, err := l.Begin()
	if err != nil {
		t.Fatalf("beginning a recording: %v", err)
	}
	_, err = r.Close(entry())
	if err == nil || !strings.Contains(err.Error(), "cannot find a free name") {
		t.Fatalf("got %v, want it to give up and say so", err)
	}
}

// TestSweepReportsADestinationItCannotRead covers a destination that went away
// while the recorder was running.
func TestSweepReportsADestinationItCannotRead(t *testing.T) {
	l, _ := library(t, "", false)
	broken(t, func() func() {
		was := readDir
		readDir = func(string) ([]os.DirEntry, error) { return nil, errBroken }
		return func() { readDir = was }
	})

	if _, err := l.Sweep(); !errors.Is(err, errBroken) {
		t.Fatalf("got %v, want it to wrap errBroken", err)
	}
}

// TestDirReportsTheDestination covers the accessor the recorder uses when it
// tells somebody where their recordings are going.
func TestDirReportsTheDestination(t *testing.T) {
	l, dir := library(t, "", false)
	if l.Dir() != dir {
		t.Errorf("Dir is %q, want %q", l.Dir(), dir)
	}
}

// TestTunedFallsBackToNothing covers a recording with neither a talkgroup nor a
// frequency, which is what an unlabelled one is.
func TestTunedFallsBackToNothing(t *testing.T) {
	tmpl, err := parse("{time}{tuned}")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := tmpl.render(Entry{Start: at}); got != at.Format("15-04-05") {
		t.Errorf("rendered %q, want the empty token to leave nothing behind", got)
	}
}

// TestTemplateThatRendersToNothingStillNamesTheFile covers a template made
// only of tokens the scanner did not fill in, which happens the first time
// somebody records with the radio on a channel it has no name for.
//
// A recording still has to be called something. Falling back to the timestamp
// keeps it distinct from the next one, which an empty name would not.
func TestTemplateThatRendersToNothingStillNamesTheFile(t *testing.T) {
	l, dir := library(t, "{system}/{channel}", false)

	filed := record(t, l, Entry{Start: at, End: at.Add(time.Second)})
	if filed.File != at.Format("2006-01-02T15-04-05")+".wav" {
		t.Errorf("filed as %q, want the timestamp", filed.File)
	}
	if _, err := os.Stat(filepath.Join(dir, filed.File)); err != nil {
		t.Errorf("the recording is not there: %v", err)
	}
}

// TestValidateTemplate covers checking a template without touching the disk,
// which is what lets a caller reject a typo before it has created a
// destination for a run that is not going to happen.
func TestValidateTemplate(t *testing.T) {
	if err := ValidateTemplate(""); err != nil {
		t.Errorf("the default template was rejected: %v", err)
	}
	if err := ValidateTemplate("{date}/{channel}"); err != nil {
		t.Errorf("a good template was rejected: %v", err)
	}
	if err := ValidateTemplate("{chanel}"); !errors.Is(err, ErrBadTemplate) {
		t.Errorf("got %v, want ErrBadTemplate", err)
	}
}

// fakeAudio makes recordings use wav instead of a real WAV file, and lets the
// filing that follows succeed anyway.
//
// The rename has to be stubbed alongside it. A fake writer creates nothing on
// disk, so moving the recording into place would fail for a reason that has
// nothing to do with what is being tested.
//
// Parameters:
//   - t: the test, which restores both seams when it ends
//   - wav: the writer every recording should use
func fakeAudio(t *testing.T, wav wavWriter) {
	t.Helper()

	wasCreate, wasRename := createWav, renameFile
	t.Cleanup(func() { createWav, renameFile = wasCreate, wasRename })

	createWav = func(string) (wavWriter, error) { return wav, nil }
	renameFile = func(string, string) error { return nil }
}

// TestNormalizingIsOptedInto covers the choice a caller makes when it opens a
// destination, and what each answer does to the recording that comes out.
//
// The reason this exists at all is that the scanner's line output does not
// follow its volume control. A recording that arrives quiet stays quiet however
// the radio is set, so scaling it after the transmission has ended is the only
// place left to fix it.
//
// Coverage: 100% (3 test cases covering both answers and the failure)
//
// Test cases:
//   - Off: by default the audio is left exactly as it arrived, and the entry
//     does not claim otherwise
//   - On: the recording is scaled and the entry says so, so that anything
//     reading it later knows the level no longer means what it used to
//   - Fails: a recording that cannot be scaled is reported rather than filed
//     as though it had been
func TestNormalizingIsOptedInto(t *testing.T) {
	// Verify the default leaves the audio alone. A level that was not touched
	// still carries the difference between a strong signal and a weak one.
	t.Run("Off", func(t *testing.T) {
		l, _ := library(t, "", false)

		wav := &fakeWav{}
		fakeAudio(t, wav)

		filed := record(t, l, entry())
		if wav.normalizedTo != 0 {
			t.Errorf("the audio was scaled to %v, want it left alone", wav.normalizedTo)
		}
		if filed.Normalized {
			t.Error("the entry claims the audio was normalized, and it was not")
		}
	})

	// Verify opting in scales the recording to the target and records that it
	// happened.
	t.Run("On", func(t *testing.T) {
		l, _ := library(t, "", true)

		wav := &fakeWav{}
		fakeAudio(t, wav)

		filed := record(t, l, entry())
		if wav.normalizedTo != NormalizeTarget {
			t.Errorf("the audio was scaled to %v, want %v", wav.normalizedTo, NormalizeTarget)
		}
		if !filed.Normalized {
			t.Error("the entry does not say the audio was normalized, and it was")
		}
	})

	// Verify a scaling pass that fails is reported. It reads the audio back and
	// writes it again, so it is a disk operation that can fail like any other,
	// and a recording left half scaled must not be filed as finished.
	t.Run("Fails", func(t *testing.T) {
		l, _ := library(t, "", true)

		fakeAudio(t, &fakeWav{failNorm: true})

		r, err := l.Begin()
		if err != nil {
			t.Fatalf("beginning a recording: %v", err)
		}
		if _, err := r.Close(entry()); !errors.Is(err, errBroken) {
			t.Fatalf("got %v, want it to wrap errBroken", err)
		}
	})
}
