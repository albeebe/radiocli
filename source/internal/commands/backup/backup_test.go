// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
)

// failingWriter is an output stream that refuses everything written to it, so
// a rendering that cannot reach the terminal can be tested.
type failingWriter struct{}

// Write refuses the output and reports why.
func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("the terminal went away")
}

// newApp builds an application context whose output is captured rather than
// printed.
//
// Parameters:
//   - t: the running test
//
// Returns:
//   - *appcontext.App writing to the buffers below
//   - *bytes.Buffer holding what was written to stdout
//   - *bytes.Buffer holding what was written to stderr
func newApp(t *testing.T) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var out, errOut bytes.Buffer
	return &appcontext.App{
		Config: &appcontext.Config{},
		Log:    slog.New(slog.DiscardHandler),
		Stdout: &out,
		Stderr: &errOut,
	}, &out, &errOut
}

// readFile reads a file back, failing the test if it cannot.
//
// Parameters:
//   - t: the running test
//   - path: the file to read
//
// Returns:
//   - string holding the file's contents
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// skipAsRoot skips a test that depends on a permission the superuser ignores.
//
// Parameters:
//   - t: the running test
func skipAsRoot(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("the superuser is not stopped by file permissions")
	}
}

// useMountDirs points the search for a mounted card at a temporary directory
// for the length of one test, so nothing real is looked at.
//
// Parameters:
//   - t: the running test, whose cleanup restores the real mount points
//   - dirs: the directories to search instead
func useMountDirs(t *testing.T, dirs ...string) {
	t.Helper()

	previous := mountDirs
	mountDirs = func() []string { return dirs }
	t.Cleanup(func() { mountDirs = previous })
}

// writeCard lays out a scanner card at root, with an identity file naming the
// radio.
//
// Parameters:
//   - t: the running test
//   - root: the volume the scanner's directory is created inside
//   - fields: the model, serial and firmware, tab separated
func writeCard(t *testing.T, root, fields string) {
	t.Helper()

	writeIdentity(t, root, "Scanner\t"+fields+"\n")
}

// writeIdentity lays out a scanner card at root, with the identity file written
// exactly as given.
//
// Parameters:
//   - t: the running test
//   - root: the volume the scanner's directory is created inside
//   - contents: what to write into the identity file
func writeIdentity(t *testing.T, root, contents string) {
	t.Helper()

	writeFile(t, filepath.Join(root, cardDir, infoFile), contents)
}

// writeFile writes a file, creating the directories above it.
//
// Parameters:
//   - t: the running test
//   - path: the file to write
//   - contents: what to write into it
func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// TestNew tests the New function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Command: the command carries its flags and its usage
//   - DefaultDestination: a backup with no destination is written where it runs
//   - NamedDestination: a backup written into the destination that was named
func TestNew(t *testing.T) {

	// Verify that the command is built with every flag the backup takes.
	t.Run("Command", func(t *testing.T) {
		app, _, _ := newApp(t)

		cmd := New(app)

		if cmd.Use != "backup [destination]" {
			t.Fatalf("unexpected usage: %q", cmd.Use)
		}
		for _, name := range []string{"source", "name", "verify", "database", "dry-run"} {
			if cmd.Flags().Lookup(name) == nil {
				t.Fatalf("the %s flag is missing", name)
			}
		}
	})

	// Verify that a backup with no destination named is written where the
	// command was run.
	t.Run("DefaultDestination", func(t *testing.T) {
		app, _, _ := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		here := t.TempDir()
		t.Chdir(here)

		cmd := New(app)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"--source", root, "--name", "here"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("backing up: %v", err)
		}
		if _, err := os.Stat(filepath.Join(here, "here", infoFile)); err != nil {
			t.Fatalf("the backup was not written where the command ran: %v", err)
		}
	})

	// Verify that a named destination is where the backup lands.
	t.Run("NamedDestination", func(t *testing.T) {
		app, _, _ := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		into := t.TempDir()
		cmd := New(app)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{into, "--source", root, "--name", "there"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("backing up: %v", err)
		}
		if _, err := os.Stat(filepath.Join(into, "there", infoFile)); err != nil {
			t.Fatalf("the backup was not written where it was asked for: %v", err)
		}
	})
}

// Test_announce tests the announce function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the card is named
//   - Skipped: what will not be copied is flagged
func Test_announce(t *testing.T) {

	// Verify that the card and where it is mounted are reported.
	t.Run("Success", func(t *testing.T) {
		app, _, errOut := newApp(t)

		announce(app, card{Root: "/Volumes/NO NAME", Model: "SDS150"}, plan{})

		if !strings.Contains(errOut.String(), "Found SDS150 on /Volumes/NO NAME") {
			t.Fatalf("the card was not announced: %q", errOut.String())
		}
		if strings.Contains(errOut.String(), "Ignoring") {
			t.Fatalf("nothing should have been ignored: %q", errOut.String())
		}
	})

	// Verify that anything on the card that is not being copied is flagged.
	t.Run("Skipped", func(t *testing.T) {
		app, _, errOut := newApp(t)

		announce(app, card{Root: "/Volumes/NO NAME"}, plan{skipped: 2})

		if !strings.Contains(errOut.String(), "Ignoring 2 item(s)") {
			t.Fatalf("the skipped items were not flagged: %q", errOut.String())
		}
	})
}

// Test_defaultName tests the defaultName function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Identified: the folder is named for the radio and the time
//   - Unidentified: a card that did not identify itself gets a generic name
func Test_defaultName(t *testing.T) {

	// Verify that a known radio is named in the folder.
	t.Run("Identified", func(t *testing.T) {
		got := defaultName(card{Model: "SDS150"})

		if !strings.HasPrefix(got, "SDS150-20") {
			t.Fatalf("the folder is not named for the radio: %q", got)
		}
		if len(got) != len("SDS150-2006-01-02-1504") {
			t.Fatalf("the folder is not dated: %q", got)
		}
	})

	// Verify that a card that said nothing about itself still gets a name.
	t.Run("Unidentified", func(t *testing.T) {
		got := defaultName(card{})

		if !strings.HasPrefix(got, "scanner-20") {
			t.Fatalf("unexpected folder name: %q", got)
		}
	})
}

// Test_locate tests the locate function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Source: a path that was given is opened rather than searched for
//   - Search: an empty path searches this platform's mount points
func Test_locate(t *testing.T) {

	// Verify that a path the user gave is used as it is.
	t.Run("Source", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		c, err := locate(root)
		if err != nil {
			t.Fatalf("locating: %v", err)
		}
		if c.Root != root {
			t.Fatalf("got %q, want %q", c.Root, root)
		}
	})

	// Verify that no path searches the mount points.
	t.Run("Search", func(t *testing.T) {
		mounts := t.TempDir()
		root := filepath.Join(mounts, "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		useMountDirs(t, mounts)

		c, err := locate("")
		if err != nil {
			t.Fatalf("locating: %v", err)
		}
		if c.Root != root {
			t.Fatalf("got %q, want %q", c.Root, root)
		}
	})
}

// Test_prepare tests the prepare function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - Named: the folder that was asked for is created
//   - Default: an unnamed backup gets the dated default
//   - Exists: an existing folder is refused rather than merged into
//   - Unreachable: a folder that cannot be looked at is reported
//   - Unwritable: a folder that cannot be created is reported
//   - Unresolvable: a path that cannot be resolved is reported
func Test_prepare(t *testing.T) {

	// Verify that the folder is created where it was asked for.
	t.Run("Named", func(t *testing.T) {
		into := t.TempDir()

		target, err := prepare(into, "tonight", card{Model: "SDS150"})
		if err != nil {
			t.Fatalf("preparing: %v", err)
		}
		if target != filepath.Join(into, "tonight") {
			t.Fatalf("unexpected folder: %q", target)
		}
		if info, err := os.Stat(target); err != nil || !info.IsDir() {
			t.Fatalf("the folder was not created: %v", err)
		}
	})

	// Verify that an unnamed backup falls back to the dated default.
	t.Run("Default", func(t *testing.T) {
		into := t.TempDir()

		target, err := prepare(into, "", card{Model: "SDS150"})
		if err != nil {
			t.Fatalf("preparing: %v", err)
		}
		if !strings.HasPrefix(filepath.Base(target), "SDS150-20") {
			t.Fatalf("the folder is not named for the radio: %q", target)
		}
	})

	// Verify that two backups are not blended into one folder.
	t.Run("Exists", func(t *testing.T) {
		into := t.TempDir()
		if err := os.MkdirAll(filepath.Join(into, "tonight"), 0o755); err != nil {
			t.Fatalf("making the folder: %v", err)
		}

		_, err := prepare(into, "tonight", card{})
		if err == nil {
			t.Fatal("expected an existing folder to be refused")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a destination that cannot even be looked at is reported.
	t.Run("Unreachable", func(t *testing.T) {
		blocked := filepath.Join(t.TempDir(), "a-file")
		writeFile(t, blocked, "not a directory")

		if _, err := prepare(blocked, "tonight", card{}); err == nil {
			t.Fatal("expected the destination to be refused")
		}
	})

	// Verify that a destination that cannot be written to is reported.
	t.Run("Unwritable", func(t *testing.T) {
		skipAsRoot(t)

		into := t.TempDir()
		if err := os.Chmod(into, 0o500); err != nil {
			t.Fatalf("closing the destination: %v", err)
		}
		t.Cleanup(func() { os.Chmod(into, 0o755) })

		_, err := prepare(into, "tonight", card{})
		if err == nil {
			t.Fatal("expected making the folder to fail")
		}
		if !strings.Contains(err.Error(), "creating ") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a path that cannot be resolved is reported, which is what a
	// working directory that cannot be read produces.
	t.Run("Unresolvable", func(t *testing.T) {
		previous := absPath
		absPath = func(string) (string, error) { return "", errors.New("no working directory") }
		t.Cleanup(func() { absPath = previous })

		_, err := prepare(t.TempDir(), "tonight", card{})
		if err == nil {
			t.Fatal("expected resolving the path to fail")
		}
		if !strings.Contains(err.Error(), "no working directory") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// Test_preview tests the preview function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: every file in the plan is described without a digest
//   - Empty: a plan with nothing in it describes nothing
func Test_preview(t *testing.T) {

	// Verify that a dry run describes the files without reading them.
	t.Run("Success", func(t *testing.T) {
		got := preview(plan{files: []file{{rel: "menu.cfg", size: 8}}})

		if len(got) != 1 {
			t.Fatalf("expected one file, got %+v", got)
		}
		if got[0].Path != "menu.cfg" || got[0].Bytes != 8 || got[0].Digest != "" {
			t.Fatalf("unexpected result: %+v", got[0])
		}
	})

	// Verify that an empty plan describes nothing.
	t.Run("Empty", func(t *testing.T) {
		if got := preview(plan{}); len(got) != 0 {
			t.Fatalf("expected nothing, got %+v", got)
		}
	})
}

// Test_renderReport tests the renderReport function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - JSON: the outcome is written as JSON
//   - JSONError: output that cannot be written is reported
//   - DryRun: a dry run says what it would have done and that it wrote nothing
//   - Verified: a finished backup says everything matched the card
//   - Unverified: a backup that was not checked says so
func Test_renderReport(t *testing.T) {

	// Verify that JSON output carries the whole report.
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp(t)
		app.Config.Output = appcontext.OutputJSON

		err := renderReport(app, report{Card: card{Model: "SDS150"}, Files: 2, Bytes: 2048})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("parsing: %v", err)
		}
		if got.Card.Model != "SDS150" || got.Files != 2 {
			t.Fatalf("unexpected report: %+v", got)
		}
	})

	// Verify that output which cannot be written is reported.
	t.Run("JSONError", func(t *testing.T) {
		app, _, _ := newApp(t)
		app.Config.Output = appcontext.OutputJSON
		app.Stdout = failingWriter{}

		if err := renderReport(app, report{}); err == nil {
			t.Fatal("expected writing the report to fail")
		}
	})

	// Verify that a dry run reports what it would do and writes nothing.
	t.Run("DryRun", func(t *testing.T) {
		app, out, errOut := newApp(t)

		err := renderReport(app, report{
			DryRun:      true,
			Files:       2,
			Directories: 1,
			Bytes:       2048,
			Card:        card{Root: "/Volumes/NO NAME"},
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "Would copy 2 files in 1 directories, 2.0 KB, from /Volumes/NO NAME") {
			t.Fatalf("unexpected output: %q", out.String())
		}
		if !strings.Contains(errOut.String(), "The radio database is excluded") {
			t.Fatalf("the excluded database was not flagged: %q", errOut.String())
		}
		if !strings.Contains(errOut.String(), "Nothing was written") {
			t.Fatalf("the dry run was not made clear: %q", errOut.String())
		}
	})

	// Verify that a finished backup reports where it went and that it matched.
	t.Run("Verified", func(t *testing.T) {
		app, out, errOut := newApp(t)

		err := renderReport(app, report{
			Files:            2,
			Directories:      1,
			Bytes:            2048,
			Destination:      "/backups/tonight",
			Verified:         true,
			DatabaseIncluded: true,
		})
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(out.String(), "Backed up 2 files in 1 directories, 2.0 KB, to /backups/tonight") {
			t.Fatalf("unexpected output: %q", out.String())
		}
		if !strings.Contains(errOut.String(), "read back and matched the card") {
			t.Fatalf("the verification was not reported: %q", errOut.String())
		}
		if !strings.Contains(errOut.String(), "Restart the scanner") {
			t.Fatalf("the restart was not mentioned: %q", errOut.String())
		}
	})

	// Verify that a backup nobody checked says so, along with what it left out.
	t.Run("Unverified", func(t *testing.T) {
		app, _, errOut := newApp(t)

		if err := renderReport(app, report{Destination: "/backups/tonight"}); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		if !strings.Contains(errOut.String(), "Files were not verified") {
			t.Fatalf("the missing verification was not reported: %q", errOut.String())
		}
		if !strings.Contains(errOut.String(), "not a complete card") {
			t.Fatalf("the excluded database was not flagged: %q", errOut.String())
		}
	})
}

// Test_run tests the run function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - Success: the card is copied and the outcome is reported
//   - DryRun: nothing is written and what would be copied is reported
//   - NoCard: a source holding no card is reported
//   - Unreadable: a card that cannot be walked is reported
//   - Empty: a card holding no files is refused
//   - DestinationExists: a folder that is already there is refused
//   - CopyError: a copy that does not finish says where the partial backup is
func Test_run(t *testing.T) {

	// Verify that a card is copied into a folder and reported.
	t.Run("Success", func(t *testing.T) {
		app, out, errOut := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")
		writeFile(t, filepath.Join(root, cardDir, "favorites", "a.hpe"), "list")
		into := t.TempDir()

		err := run(context.Background(), app, into, options{
			source:   root,
			name:     "tonight",
			verify:   true,
			database: true,
		})
		if err != nil {
			t.Fatalf("backing up: %v", err)
		}
		if got := readFile(t, filepath.Join(into, "tonight", "favorites", "a.hpe")); got != "list" {
			t.Fatalf("the copy differs: %q", got)
		}
		if !strings.Contains(out.String(), "Backed up 2 files") {
			t.Fatalf("unexpected output: %q", out.String())
		}
		if !strings.Contains(errOut.String(), "Copying 2 files") {
			t.Fatalf("the progress was not reported: %q", errOut.String())
		}
	})

	// Verify that a dry run writes nothing.
	t.Run("DryRun", func(t *testing.T) {
		app, out, _ := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")
		into := t.TempDir()

		err := run(context.Background(), app, into, options{
			source:   root,
			database: true,
			dryRun:   true,
		})
		if err != nil {
			t.Fatalf("backing up: %v", err)
		}
		if !strings.Contains(out.String(), "Would copy 1 files") {
			t.Fatalf("unexpected output: %q", out.String())
		}
		entries, err := os.ReadDir(into)
		if err != nil {
			t.Fatalf("reading the destination: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("the dry run wrote something: %v", entries)
		}
	})

	// Verify that a source holding no card is reported.
	t.Run("NoCard", func(t *testing.T) {
		app, _, _ := newApp(t)

		err := run(context.Background(), app, t.TempDir(), options{source: t.TempDir()})
		if err == nil {
			t.Fatal("expected a source with no card to be refused")
		}
		if !strings.Contains(err.Error(), "does not hold a scanner card") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a card that cannot be read is reported.
	t.Run("Unreadable", func(t *testing.T) {
		skipAsRoot(t)

		app, _, _ := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		closed := filepath.Join(root, cardDir)
		if err := os.Chmod(closed, 0o000); err != nil {
			t.Fatalf("closing the card: %v", err)
		}
		t.Cleanup(func() { os.Chmod(closed, 0o755) })

		err := run(context.Background(), app, t.TempDir(), options{source: root, database: true})
		if err == nil {
			t.Fatal("expected an unreadable card to be reported")
		}
		if !strings.Contains(err.Error(), "reading the card") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a card with nothing on it is refused rather than backed up.
	t.Run("Empty", func(t *testing.T) {
		app, _, _ := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		if err := os.MkdirAll(filepath.Join(root, cardDir), 0o755); err != nil {
			t.Fatalf("making the card: %v", err)
		}

		err := run(context.Background(), app, t.TempDir(), options{source: root, database: true})
		if err == nil {
			t.Fatal("expected an empty card to be refused")
		}
		if !strings.Contains(err.Error(), "holds no files") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify that a folder which already exists is refused before anything is
	// announced.
	t.Run("DestinationExists", func(t *testing.T) {
		app, _, errOut := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		into := t.TempDir()
		if err := os.MkdirAll(filepath.Join(into, "tonight"), 0o755); err != nil {
			t.Fatalf("making the folder: %v", err)
		}

		err := run(context.Background(), app, into, options{
			source:   root,
			name:     "tonight",
			database: true,
		})
		if err == nil {
			t.Fatal("expected the existing folder to be refused")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("unexpected error: %v", err)
		}
		if errOut.Len() != 0 {
			t.Fatalf("the refusal was announced under a banner: %q", errOut.String())
		}
	})

	// Verify that a copy which does not finish says where what was copied is.
	t.Run("CopyError", func(t *testing.T) {
		skipAsRoot(t)

		app, _, errOut := newApp(t)
		root := filepath.Join(t.TempDir(), "NO NAME")
		writeCard(t, root, "SDS150\t1234567\t1.06.05")

		// The card is walked while the file can be read and copied after it
		// cannot, so the copy fails part way through.
		closed := filepath.Join(root, cardDir, infoFile)
		if err := os.Chmod(closed, 0o000); err != nil {
			t.Fatalf("closing the file: %v", err)
		}
		t.Cleanup(func() { os.Chmod(closed, 0o600) })

		into := t.TempDir()
		err := run(context.Background(), app, into, options{
			source:   root,
			name:     "tonight",
			database: true,
		})
		if err == nil {
			t.Fatal("expected the copy to fail")
		}
		if !strings.Contains(errOut.String(), "The backup did not finish") {
			t.Fatalf("the partial backup was not reported: %q", errOut.String())
		}
	})
}

// Test_prepareDestinationIsAbsolute is the regression for a folder reported as
// a relative path.
//
// The folder is named in the note that says where a partial backup was left, so
// a path relative to a working directory the reader cannot see would not help
// them find it.
func Test_prepareDestinationIsAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())

	target, err := prepare(".", "tonight", card{})
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}
	if !filepath.IsAbs(target) {
		t.Fatalf("the folder is not absolute: %q", target)
	}
}
