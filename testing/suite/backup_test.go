// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package suite

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// backupReport mirrors the JSON the backup command writes.
type backupReport struct {
	Card struct {
		Root     string `json:"root"`
		Model    string `json:"model"`
		Serial   string `json:"serial"`
		Firmware string `json:"firmware"`
	} `json:"card"`
	Destination      string `json:"destination"`
	Files            int    `json:"files"`
	Bytes            int64  `json:"bytes"`
	Directories      int    `json:"directories"`
	Verified         bool   `json:"verified"`
	DatabaseIncluded bool   `json:"databaseIncluded"`
	DryRun           bool   `json:"dryRun"`
	Copied           []struct {
		Path   string `json:"path"`
		Bytes  int64  `json:"bytes"`
		Digest string `json:"digest"`
	} `json:"copied"`
}

// fakeCard builds a directory shaped like a scanner's memory card.
//
// The backup command reads a card and never the scanner, so the whole of it
// can be tested against a card made here. That keeps these tests honest
// without a radio attached, and lets them assert on exact counts and bytes,
// which a real card cannot promise because the scanner writes to it.
//
// The shape copies a real SDS150 card: the scanner's directory named for the
// older BCD models, an identity file, configuration beside it, a large
// database directory, and the four directories the scanner leaves empty until
// it records something.
func fakeCard(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	card := filepath.Join(root, "BCDx36HP")

	files := map[string]string{
		"scanner.inf":                 "TargetModel\tBCDx36HP\nFormatVersion\t1.00\nScanner\tSDS150\tTEST-SERIAL-01\t1.00.37\t01\n",
		"app_data.cfg":                "TargetModel\tBCDx36HP\nModeInfo\tScan\n",
		"profile.cfg":                 "DispColors\tDispColorId=1\tColorLayoutId=1\tff4600\t000000\n",
		"favorites_lists/fl_0001.hpd": "favorite list contents\n",
		"firmware/boot.bin":           "firmware\n",
	}
	for name, body := range files {
		path := filepath.Join(card, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("building the card: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("building the card: %v", err)
		}
	}

	// The database is the bulk of a real card and the part --database=false
	// drops, so it needs enough substance to tell the two cases apart.
	big := make([]byte, 4096)
	if _, err := rand.Read(big); err != nil {
		t.Fatalf("building the card: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(card, "HPDB"), 0o755); err != nil {
		t.Fatalf("building the card: %v", err)
	}
	if err := os.WriteFile(filepath.Join(card, "HPDB", "database.dat"), big, 0o644); err != nil {
		t.Fatalf("building the card: %v", err)
	}

	// The directories the scanner writes into later, left empty, plus the junk
	// a desktop operating system leaves on any volume it mounts.
	for _, dir := range []string{"audio", "alert", "activity_log", "discovery"} {
		if err := os.MkdirAll(filepath.Join(card, dir), 0o755); err != nil {
			t.Fatalf("building the card: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".Spotlight-V100"), 0o755); err != nil {
		t.Fatalf("building the card: %v", err)
	}
	if err := os.WriteFile(filepath.Join(card, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatalf("building the card: %v", err)
	}

	return root
}

// TestBackup copies a card and checks that what lands on disk is the card.
func TestBackup(t *testing.T) {
	card := fakeCard(t)
	into := t.TempDir()

	var report backupReport
	mustJSON(t, &report, "backup", into, "--source", card, "--name", "copy")

	if report.Card.Model != "SDS150" {
		t.Errorf("model is %q, wanted %q", report.Card.Model, "SDS150")
	}
	if report.Card.Serial != "TEST-SERIAL-01" {
		t.Errorf("serial is %q, wanted %q", report.Card.Serial, "TEST-SERIAL-01")
	}
	if report.Card.Firmware != "1.00.37" {
		t.Errorf("firmware is %q, wanted %q", report.Card.Firmware, "1.00.37")
	}

	// Six files, because the operating system's junk is not the scanner's.
	if report.Files != 6 {
		t.Errorf("copied %d files, wanted 6", report.Files)
	}
	if !report.Verified {
		t.Error("the backup reports it was not verified, and verifying is the default")
	}
	if report.DryRun {
		t.Error("the backup reports a dry run, and it was not one")
	}

	dest := filepath.Join(into, "copy")
	if report.Destination != dest {
		t.Errorf("destination is %q, wanted %q", report.Destination, dest)
	}

	t.Run("the copy matches the card", func(t *testing.T) {
		for _, name := range []string{
			"scanner.inf", "app_data.cfg", "profile.cfg",
			"favorites_lists/fl_0001.hpd", "firmware/boot.bin", "HPDB/database.dat",
		} {
			want, err := os.ReadFile(filepath.Join(card, "BCDx36HP", name))
			if err != nil {
				t.Fatalf("reading the card: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(dest, name))
			if err != nil {
				t.Errorf("%s is missing from the backup: %v", name, err)
				continue
			}
			if string(got) != string(want) {
				t.Errorf("%s in the backup differs from the card", name)
			}
		}
	})

	t.Run("empty directories survive", func(t *testing.T) {
		// A card restored without these is missing the places the scanner
		// writes recordings, alerts, discovery sessions and logs into.
		for _, dir := range []string{"audio", "alert", "activity_log", "discovery"} {
			info, err := os.Stat(filepath.Join(dest, dir))
			if err != nil {
				t.Errorf("%s is missing from the backup, and it is empty on the card: %v", dir, err)
				continue
			}
			if !info.IsDir() {
				t.Errorf("%s in the backup is not a directory", dir)
			}
		}
	})

	t.Run("junk from this computer is left behind", func(t *testing.T) {
		for _, name := range []string{".DS_Store", ".Spotlight-V100"} {
			if _, err := os.Stat(filepath.Join(dest, name)); err == nil {
				t.Errorf("%s was copied, and it belongs to this computer rather than the scanner", name)
			}
		}
	})

	t.Run("every file gets a checksum", func(t *testing.T) {
		for _, c := range report.Copied {
			if c.Digest == "" {
				t.Errorf("%s has no digest, and the backup reports it was verified", c.Path)
			}
		}
	})
}

// TestBackup_WithoutDatabase checks the flag that drops the bulk of the card.
func TestBackup_WithoutDatabase(t *testing.T) {
	card := fakeCard(t)
	into := t.TempDir()

	var report backupReport
	mustJSON(t, &report, "backup", into, "--source", card, "--name", "copy", "--database=false")

	if report.DatabaseIncluded {
		t.Error("the backup reports the database was included, and it was excluded")
	}
	if report.Files != 5 {
		t.Errorf("copied %d files, wanted 5 with the database excluded", report.Files)
	}
	if _, err := os.Stat(filepath.Join(into, "copy", "HPDB")); err == nil {
		t.Error("the database was copied, and --database=false asked for it to be left")
	}

	// The configuration is the point of excluding the database, so it has to
	// still be there.
	if _, err := os.Stat(filepath.Join(into, "copy", "profile.cfg")); err != nil {
		t.Errorf("the settings are missing from a backup that excluded only the database: %v", err)
	}
}

// TestBackup_DryRun checks that a dry run reports the work and writes nothing.
func TestBackup_DryRun(t *testing.T) {
	card := fakeCard(t)
	into := t.TempDir()

	var report backupReport
	mustJSON(t, &report, "backup", into, "--source", card, "--dry-run")

	if !report.DryRun {
		t.Error("the report does not say it was a dry run")
	}
	if report.Files != 6 {
		t.Errorf("reported %d files, wanted 6", report.Files)
	}
	if report.Destination != "" {
		t.Errorf("a dry run named a destination, %q, and it writes nothing", report.Destination)
	}

	entries, err := os.ReadDir(into)
	if err != nil {
		t.Fatalf("reading the destination: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a dry run wrote %d entries into the destination", len(entries))
	}
}

// TestBackup_Arguments checks the refusals a user will meet.
func TestBackup_Arguments(t *testing.T) {
	card := fakeCard(t)

	t.Run("a source that is not a card", func(t *testing.T) {
		mustFail(t, "does not hold a scanner card", "backup", t.TempDir(), "--source", t.TempDir())
	})

	t.Run("the card directory itself is accepted", func(t *testing.T) {
		var report backupReport
		mustJSON(t, &report, "backup", t.TempDir(),
			"--source", filepath.Join(card, "BCDx36HP"), "--dry-run")
		if report.Files != 6 {
			t.Errorf("reported %d files, wanted 6", report.Files)
		}
	})

	t.Run("an existing folder is refused", func(t *testing.T) {
		into := t.TempDir()
		mustJSON(t, &backupReport{}, "backup", into, "--source", card, "--name", "taken")
		mustFail(t, "already exists", "backup", into, "--source", card, "--name", "taken")
	})

	t.Run("refusing more than one destination", func(t *testing.T) {
		res := run(t, "backup", "one", "two", "--source", card)
		if res.code == 0 {
			t.Error("two destinations were accepted, and the command takes one")
		}
	})
}

// TestBackup_Text checks the human output, which is what most people read.
func TestBackup_Text(t *testing.T) {
	card := fakeCard(t)
	into := t.TempDir()

	res := mustRun(t, "backup", into, "--source", card, "--name", "copy")

	if !strings.Contains(res.stdout, "Backed up 6 files in 7 directories") {
		t.Errorf("the result does not say what was copied:\n%s", res.stdout)
	}
	// The identity and the advice are notes, so they belong on stderr and must
	// not pollute the result a script reads.
	for _, want := range []string{"SDS150 TEST-SERIAL-01", "read back and matched", "Restart the scanner"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr does not mention %q:\n%s", want, res.stderr)
		}
	}
	if strings.Contains(res.stdout, "Restart the scanner") {
		t.Error("advice was written to stdout, where it would survive a pipe")
	}
}
