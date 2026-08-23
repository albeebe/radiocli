// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package suite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAudioRecordRefusesWithoutAScanner checks the rule that this command
// records a scanner rather than a sound card.
//
// It is the guard that stops somebody pointing the recorder at a microphone and
// filling a disk with the room, so it is checked before anything else about the
// command.
func TestAudioRecordRefusesWithoutAScanner(t *testing.T) {
	// The suite puts --device in front of every command, so this passes an
	// empty one after it: pflag takes the last it is given, which is how a test
	// overrides the port the harness chose.
	res := run(t, "--device", "", "audio", "record", t.TempDir())
	if res.code == 0 {
		t.Fatal("recording without a scanner was allowed")
	}
	for _, want := range []string{"no scanner named", "labelled"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal said %q, wanted it to mention %q", firstLine(res.stderr), want)
		}
	}
}

// TestAudioRecordChecksItsTemplateFirst checks that a naming template that
// cannot work is refused before anything is opened.
//
// A typo found at startup costs a second. The same typo found on the first
// transmission of the night costs the night, which is why this is checked
// rather than assumed.
func TestAudioRecordChecksItsTemplateFirst(t *testing.T) {
	needScanner(t)

	dir := t.TempDir()
	res := mustFail(t, "is not a token", "--device", harness.device,
		"audio", "record", dir, "--template", "{chanel}")

	// The message lists every token there is, so the fix does not need the
	// documentation open beside it.
	for _, want := range []string{"channel", "talkgroup", "duration"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal did not name the %q token: %s", want, firstLine(res.stderr))
		}
	}

	// And nothing was created for a run that never happened.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the destination: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused run left %d files behind, wanted none", len(entries))
	}
}

// TestAudioRecordRefusesATemplateThatNamesEveryFileTheSame checks the template
// with no tokens in it, which would have every recording overwrite the last.
func TestAudioRecordRefusesATemplateThatNamesEveryFileTheSame(t *testing.T) {
	needScanner(t)

	mustFail(t, "no tokens in it", "--device", harness.device,
		"audio", "record", t.TempDir(), "--template", "recording")
}

// TestAudioRecordWritesRecordings runs the recorder against whatever sound
// input the scanner's audio is cabled into and checks what lands on disk.
//
// It is skipped unless -audio names an input, because the audio reaches this
// computer over a cable somebody has to have run, and nothing in the tool can
// know whether they have.
func TestAudioRecordWritesRecordings(t *testing.T) {
	needScanner(t)
	if *audioInput == "" {
		t.Skip("no sound input named: pass -audio \"<input>\" to record from the scanner")
	}

	dir := t.TempDir()
	res := runFor(t, recordFor, "--device", harness.device, "-o", "json",
		"audio", "record", dir, "--input", *audioInput, "--min-duration", "500ms")

	// Stopping it is how it ends, so being stopped is not a failure.
	if res.code != 0 {
		t.Fatalf("recording exited %d, wanted 0\nstderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, *audioInput) {
		t.Errorf("stderr never named the input it was recording from: %s", res.stderr)
	}

	// The destination is prepared whether or not anything was heard, so it is
	// the one thing that must be there.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("the destination was not prepared: %v", err)
	}

	// Every recording is described by a JSON file beside its audio, so the
	// sidecars are the listing of what the run caught.
	var sidecars []string
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".json") {
			sidecars = append(sidecars, path)
		}
		return err
	}); err != nil {
		t.Fatalf("looking through %s: %v", dir, err)
	}
	if len(sidecars) == 0 {
		t.Skip("nothing was transmitted while the recorder was running")
	}

	for i, path := range sidecars {
		line, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}

		var e struct {
			File     string  `json:"file"`
			Start    string  `json:"start"`
			End      string  `json:"end"`
			Duration float64 `json:"duration"`
			Channel  string  `json:"channel"`
			Reason   string  `json:"reason"`
			Samples  int     `json:"samples"`
		}
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("%s is not JSON: %v\n%s", path, err, line)
		}

		if e.File == "" || e.Start == "" || e.End == "" {
			t.Errorf("recording %d is missing a field every recording has: %+v", i+1, e)
		}
		if e.Duration <= 0 {
			t.Errorf("recording %d has a duration of %v", i+1, e.Duration)
		}
		if e.Reason == "" {
			t.Errorf("recording %d does not say why it ended: %+v", i+1, e)
		}

		// The audio and its description sit beside each other under the same
		// name, whatever the template said, and File points at the audio from
		// the top of the destination.
		audio := filepath.Join(dir, e.File)
		if _, err := os.Stat(audio); err != nil {
			t.Errorf("the audio for recording %d is not there: %v", i+1, err)
		}
		if want := strings.TrimSuffix(audio, ".wav") + ".json"; want != path {
			t.Errorf("%s describes %s, want the audio beside it", path, want)
		}

		// A WAV nothing will play is the failure worth catching, so the header
		// is checked rather than only the file's existence.
		header, err := os.ReadFile(audio)
		if err != nil || len(header) < 44 {
			t.Fatalf("the audio for recording %d is too short to be a WAV: %v", i+1, err)
		}
		if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
			t.Errorf("the audio for recording %d is not a WAV", i+1)
		}
		if len(header) == 44 {
			t.Errorf("the audio for recording %d is a header with no audio in it", i+1)
		}

		// A recording the scanner named has to have been asked at least once,
		// and one it did not name must not have been given a channel anyway.
		if e.Channel != "" && e.Samples == 0 {
			t.Errorf("recording %d names a channel with no readings behind it: %+v", i+1, e)
		}
	}
}
