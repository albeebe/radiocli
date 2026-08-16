// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/buildinfo"
)

// newApp builds an App whose streams are buffers, so a test never writes to the
// terminal and nothing reads from it.
//
// Parameters:
//   - t: unused, present so the helper reads like the others
//
// Returns:
//   - the App, what it prints as the answer, and what it prints as a note
func newApp(t *testing.T) (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	app := appcontext.New()
	out, notes := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout = out
	app.Stderr = notes
	app.Stdin = &bytes.Buffer{}
	return app, out, notes
}

// pretendPlatform fixes the platform for the length of a test, so the answer
// does not depend on the machine the tests run on.
//
// Parameters:
//   - t: the test the change is tied to
//   - os: the operating system, as Go names it
//   - arch: the architecture, as Go names it
func pretendPlatform(t *testing.T, os, arch string) {
	t.Helper()

	wasOS, wasArch := goos, goarch
	goos, goarch = os, arch
	t.Cleanup(func() { goos, goarch = wasOS, wasArch })
}

// pretendVersion fixes what the running build calls itself, since a build made
// to run the tests always calls itself "dev".
//
// Parameters:
//   - t: the test the change is tied to
//   - v: the version to report
func pretendVersion(t *testing.T, v string) {
	t.Helper()

	was := buildinfo.Version
	buildinfo.Version = v
	t.Cleanup(func() { buildinfo.Version = was })
}

// releaseServer stands in for GitHub, publishing one release with a Mac zip and
// the checksums file that vouches for it.
//
// Parameters:
//   - t: the test the server's lifetime is tied to
//   - tag: the tag to publish under
//   - sums: what to serve as the checksums file, or empty to serve the real one
//
// Returns:
//   - the release as it would be read back from the API
func releaseServer(t *testing.T, tag, sums string) release {
	t.Helper()

	archive, err := os.ReadFile(makeZip(t, entry{name: "radiocli", body: "the new program"}))
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	if sums == "" {
		sums = sha256Of(string(archive)) + "  radiocli-mac.zip\n"
	}

	serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + checksumsAsset:
			_, _ = w.Write([]byte(sums))
		case "/radiocli-mac.zip":
			_, _ = w.Write(archive)
		default:
			_, _ = fmt.Fprintf(w, `{
				"tag_name": %q,
				"html_url": "https://example.invalid/releases/%s",
				"published_at": "2026-08-16T15:39:01Z",
				"assets": [
					{"name": "radiocli-mac.zip", "browser_download_url": "http://%s/radiocli-mac.zip", "size": %d},
					{"name": %q, "browser_download_url": "http://%s/%s"}
				]
			}`, tag, tag, r.Host, len(archive), checksumsAsset, r.Host, checksumsAsset)
		}
	})

	rel, err := fetchRelease(context.Background(), "")
	if err != nil {
		t.Fatalf("reading the release back: %v", err)
	}
	return rel
}

// installedProgram writes a program to stand in for the running one.
//
// Parameters:
//   - t: the test the file's lifetime is tied to
//
// Returns:
//   - the path of the program
func installedProgram(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "radiocli")
	writeFile(t, path, "the old program", 0o755)
	return path
}

// TestNew tests the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering all of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and described, and takes no arguments
//   - Flags: the three flags are registered with the defaults documented
//   - Runs: executing it reaches the work
func TestNew(t *testing.T) {
	// Verify the shape of the command, including that it is not marked as one
	// that only reads: it must not be run through a daemon holding the scanner
	t.Run("Wiring", func(t *testing.T) {
		app, _, _ := newApp(t)
		cmd := New(app)

		if cmd.Use != "update" {
			t.Errorf("expected the command to be named update, got: %q", cmd.Use)
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("expected both a short and a long description")
		}
		if cmd.Args == nil {
			t.Error("expected the command to say it takes no arguments")
		}
		if len(cmd.Annotations) != 0 {
			t.Errorf("expected no annotations, got: %v", cmd.Annotations)
		}
	})

	// Verify that every flag is registered, and with the default its
	// documentation gives
	t.Run("Flags", func(t *testing.T) {
		app, _, _ := newApp(t)
		cmd := New(app)

		for name, want := range map[string]string{
			"check":   "false",
			"force":   "false",
			"version": "",
		} {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				t.Fatalf("expected a --%s flag", name)
			}
			if f.DefValue != want {
				t.Errorf("expected --%s to default to %q, got: %q", name, want, f.DefValue)
			}
		}
	})

	// Verify that running the command reaches the work rather than stopping at
	// the closure
	t.Run("Runs", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.1.1")
		releaseServer(t, "v0.1.1", "")

		app, out, _ := newApp(t)
		cmd := New(app)
		cmd.SetOut(app.Stdout)
		cmd.SetErr(app.Stderr)
		cmd.SetArgs([]string{"--check"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "v0.1.1") {
			t.Errorf("expected the release in the output, got: %q", out.String())
		}
	})
}

// Test_install tests downloading a release, checking it, and putting it in
// place.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Installs: the program is replaced and everything staged is cleaned up
//   - NotWritable: the check that runs before anything is downloaded
//   - NoChecksums: a release publishing none is refused
//   - AssetMissing: a release without this platform's file
//   - DownloadFails: the archive could not be fetched
//   - VerifyFails: the archive does not match its published checksum
//   - ExtractFails: the archive does not hold the program
//   - ReplaceFails: the program could not be put in place
func Test_install(t *testing.T) {
	// Verify the whole path: the program is replaced by the one from the
	// archive, and nothing staged along the way is left behind
	t.Run("Installs", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.1.0")
		rel := releaseServer(t, "v0.1.1", "")
		exe := installedProgram(t)
		app, _, _ := newApp(t)

		want, err := assetFor(goos, goarch)
		if err != nil {
			t.Fatalf("choosing the asset: %v", err)
		}

		got, err := install(context.Background(), app, rel, want, exe, options{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if body, _ := os.ReadFile(exe); string(body) != "the new program" {
			t.Errorf("expected the new program in place, got: %q", body)
		}
		if got.From != "v0.1.0" || got.To != "v0.1.1" || got.Path != exe {
			t.Errorf("expected the install to be reported, got: %+v", got)
		}
		if got.Digest == "" || got.Bytes == 0 {
			t.Errorf("expected the download to be described, got: %+v", got)
		}

		left, err := os.ReadDir(filepath.Dir(exe))
		if err != nil {
			t.Fatalf("reading the directory: %v", err)
		}
		if len(left) != 1 {
			t.Errorf("expected only the program to be left, found: %v", left)
		}
	})

	// Verify that a directory which cannot be written to is caught before
	// anything is downloaded, and that the message says what to type instead
	t.Run("NotWritable", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		dir := filepath.Join(t.TempDir(), "locked")
		if err := os.Mkdir(dir, 0o500); err != nil {
			t.Fatalf("making the directory: %v", err)
		}
		app, _, _ := newApp(t)

		_, err := install(context.Background(), app, release{},
			asset{archive: "radiocli-mac.zip"}, filepath.Join(dir, "radiocli"), options{})
		if err == nil {
			t.Fatal("expected an unwritable directory to be refused")
		}
		if !strings.Contains(err.Error(), "sudo radiocli update") {
			t.Errorf("expected the sudo line in the message, got: %v", err)
		}
	})

	// Verify that a release with no checksums file is refused rather than
	// installed unchecked
	t.Run("NoChecksums", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		app, _, _ := newApp(t)

		_, err := install(context.Background(), app, release{TagName: "v0.9.0"},
			asset{archive: "radiocli-mac.zip"}, installedProgram(t), options{})
		if err == nil || !strings.Contains(err.Error(), checksumsAsset) {
			t.Errorf("expected the missing checksums to be refused, got: %v", err)
		}
	})

	// Verify that a release without this platform's file says so
	t.Run("AssetMissing", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		rel := releaseServer(t, "v0.1.1", "")
		app, _, _ := newApp(t)

		_, err := install(context.Background(), app, rel,
			asset{archive: "radiocli-solaris.zip"}, installedProgram(t), options{})
		if err == nil || !strings.Contains(err.Error(), "radiocli-solaris.zip") {
			t.Errorf("expected the missing asset to be reported, got: %v", err)
		}
	})

	// Verify that a download which cannot be made is reported
	t.Run("DownloadFails", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		rel := releaseServer(t, "v0.1.1", "")
		app, _, _ := newApp(t)

		// Point the archive somewhere nothing answers, leaving the checksums
		// where they are so the failure is the download itself.
		for i := range rel.Assets {
			if rel.Assets[i].Name == "radiocli-mac.zip" {
				rel.Assets[i].URL = "http://127.0.0.1:1/radiocli-mac.zip"
			}
		}

		want, _ := assetFor(goos, goarch)
		if _, err := install(context.Background(), app, rel, want,
			installedProgram(t), options{}); err == nil {
			t.Error("expected the failed download to be reported")
		}
	})

	// Verify that an archive not matching its published checksum is refused,
	// which is the check the whole command exists for
	t.Run("VerifyFails", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		rel := releaseServer(t, "v0.1.1", sha256Of("something else")+"  radiocli-mac.zip\n")
		exe := installedProgram(t)
		app, _, _ := newApp(t)

		want, _ := assetFor(goos, goarch)
		_, err := install(context.Background(), app, rel, want, exe, options{})
		if err == nil || !strings.Contains(err.Error(), "does not match the checksum") {
			t.Errorf("expected the mismatch to be refused, got: %v", err)
		}
		if body, _ := os.ReadFile(exe); string(body) != "the old program" {
			t.Errorf("expected the running program untouched, got: %q", body)
		}
	})

	// Verify that an archive which does not hold the program is reported
	t.Run("ExtractFails", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		rel := releaseServer(t, "v0.1.1", "")
		app, _, _ := newApp(t)

		// The archive is the one the checksums vouch for, so the failure is
		// what is inside it rather than the download.
		want := asset{archive: "radiocli-mac.zip", binary: "something-else", platform: "darwin/arm64"}
		_, err := install(context.Background(), app, rel, want, installedProgram(t), options{})
		if err == nil || !strings.Contains(err.Error(), "does not contain") {
			t.Errorf("expected the wrong archive contents to be reported, got: %v", err)
		}
	})

	// Verify that a failure putting the program in place is reported and leaves
	// the running one alone
	t.Run("ReplaceFails", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		rel := releaseServer(t, "v0.1.1", "")
		exe := installedProgram(t)
		app, _, _ := newApp(t)

		was := renameFile
		renameFile = func(string, string) error { return os.ErrPermission }
		t.Cleanup(func() { renameFile = was })

		want, _ := assetFor(goos, goarch)
		if _, err := install(context.Background(), app, rel, want, exe, options{}); err == nil {
			t.Error("expected the failed replacement to be reported")
		}
		if body, _ := os.ReadFile(exe); string(body) != "the old program" {
			t.Errorf("expected the running program untouched, got: %q", body)
		}
	})
}

// Test_published tests rendering when a release was published.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Reported: a time comes back in a form something can parse
//   - Absent: no time comes back as nothing at all
func Test_published(t *testing.T) {
	// Verify that a published time is rendered in the form the documentation
	// promises
	t.Run("Reported", func(t *testing.T) {
		when := time.Date(2026, 8, 16, 15, 39, 1, 0, time.UTC)

		if got := published(release{PublishedAt: when}); got != "2026-08-16T15:39:01Z" {
			t.Errorf("expected the time in RFC 3339, got: %q", got)
		}
	})

	// Verify that no time is empty rather than a zero date, which would read as
	// a release published in year one
	t.Run("Absent", func(t *testing.T) {
		if got := published(release{}); got != "" {
			t.Errorf("expected nothing, got: %q", got)
		}
	})
}

// Test_renderReport tests printing what was installed.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Text: the human-readable form, with the daemon warning as a note
//   - JSON: the machine-readable form
//   - JSONWriteError: the stream cannot be written to
func Test_renderReport(t *testing.T) {
	installed := report{To: "v0.1.1", Path: "/usr/local/bin/radiocli"}

	// Verify that the answer goes to stdout and the advice to stderr, so
	// redirecting one leaves the other alone
	t.Run("Text", func(t *testing.T) {
		app, out, notes := newApp(t)

		if err := renderReport(app, installed); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "/usr/local/bin/radiocli") {
			t.Errorf("expected the path in the output, got: %q", out.String())
		}
		if !strings.Contains(notes.String(), "daemon") {
			t.Errorf("expected the daemon warning as a note, got: %q", notes.String())
		}
	})

	// Verify the machine-readable form
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp(t)
		app.Config.Output = appcontext.OutputJSON

		if err := renderReport(app, installed); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("expected JSON, got: %q", out.String())
		}
		if got.To != "v0.1.1" {
			t.Errorf("expected the version installed, got: %+v", got)
		}
	})

	// Verify that a stream which cannot be written to is reported
	t.Run("JSONWriteError", func(t *testing.T) {
		app, _, _ := newApp(t)
		app.Config.Output = appcontext.OutputJSON

		reader, writer := io.Pipe()
		if err := reader.Close(); err != nil {
			t.Fatalf("closing the pipe: %v", err)
		}
		app.Stdout = writer

		if err := renderReport(app, installed); err == nil {
			t.Error("expected the write failure to be reported")
		}
	})
}

// Test_renderStatus tests printing how the running build stands.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Available: a newer release, with how to install it as a note
//   - Current: nothing to do
//   - Ahead: this build is newer than the newest release
//   - Dev: a development build cannot be compared
//   - JSON: the machine-readable form
//   - JSONWriteError: the stream cannot be written to
func Test_renderStatus(t *testing.T) {
	// Verify that a newer release is named, and the way to install it is a note
	// rather than part of the answer
	t.Run("Available", func(t *testing.T) {
		app, out, notes := newApp(t)

		err := renderStatus(app, status{Current: "v0.1.0", Latest: "v0.1.1",
			State: stateAvailable, UpdateAvailable: true})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "v0.1.1 is available") {
			t.Errorf("expected the release to be offered, got: %q", out.String())
		}
		if !strings.Contains(notes.String(), "radiocli update") {
			t.Errorf("expected the way to install it as a note, got: %q", notes.String())
		}
	})

	// Verify that being up to date says so plainly
	t.Run("Current", func(t *testing.T) {
		app, out, _ := newApp(t)

		if err := renderStatus(app, status{Current: "v0.1.1", Latest: "v0.1.1",
			State: stateCurrent}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "the newest release") {
			t.Errorf("expected it to report being current, got: %q", out.String())
		}
	})

	// Verify that a build ahead of the release says so rather than offering a
	// downgrade
	t.Run("Ahead", func(t *testing.T) {
		app, out, _ := newApp(t)

		if err := renderStatus(app, status{Current: "v0.2.0", Latest: "v0.1.1",
			State: stateAhead}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "newer than") {
			t.Errorf("expected it to report being ahead, got: %q", out.String())
		}
	})

	// Verify that a development build is told why it cannot be compared and
	// what to do about it
	t.Run("Dev", func(t *testing.T) {
		app, out, notes := newApp(t)

		if err := renderStatus(app, status{Current: devVersion, Latest: "v0.1.1",
			State: stateUnknown, Dev: true}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "cannot be compared") {
			t.Errorf("expected it to say why, got: %q", out.String())
		}
		if !strings.Contains(notes.String(), "--force") {
			t.Errorf("expected the way forward as a note, got: %q", notes.String())
		}
	})

	// Verify the machine-readable form, which is what something polling this
	// reads
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := newApp(t)
		app.Config.Output = appcontext.OutputJSON

		err := renderStatus(app, status{Current: "v0.1.0", Latest: "v0.1.1",
			State: stateAvailable, UpdateAvailable: true})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		var got status
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("expected JSON, got: %q", out.String())
		}
		if !got.UpdateAvailable || got.State != stateAvailable {
			t.Errorf("expected an update to be offered, got: %+v", got)
		}
	})

	// Verify that a stream which cannot be written to is reported
	t.Run("JSONWriteError", func(t *testing.T) {
		app, _, _ := newApp(t)
		app.Config.Output = appcontext.OutputJSON

		reader, writer := io.Pipe()
		if err := reader.Close(); err != nil {
			t.Fatalf("closing the pipe: %v", err)
		}
		app.Stdout = writer

		if err := renderStatus(app, status{State: stateCurrent}); err == nil {
			t.Error("expected the write failure to be reported")
		}
	})
}

// Test_run tests deciding what to do and doing it.
//
// Coverage: 100% (11 test cases covering every branch)
//
// Test cases:
//   - Checks: --check reports without changing anything
//   - Installs: the ordinary path replaces the program
//   - Unsupported: a platform with no build, refused before any request
//   - InDaemon: the daemon refuses to replace what it is running
//   - NoRelease: GitHub could not be asked
//   - DevBuild: an unstamped build refuses to replace itself
//   - DevBuildForced: --force installs over it
//   - AlreadyCurrent: nothing to do, and nothing downloaded
//   - Ahead: a build newer than the release is refused
//   - Unlocatable: the program cannot say where it is
//   - InstallFails: the install could not be finished
func Test_run(t *testing.T) {
	// Verify that --check changes nothing, which is what makes it safe to poll
	t.Run("Checks", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.1.0")
		releaseServer(t, "v0.1.1", "")
		app, out, _ := newApp(t)

		if err := run(context.Background(), app, options{check: true}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(out.String(), "v0.1.1 is available") {
			t.Errorf("expected the newer release to be offered, got: %q", out.String())
		}
	})

	// Verify the ordinary path end to end
	t.Run("Installs", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.1.0")
		releaseServer(t, "v0.1.1", "")
		exe := installedProgram(t)
		pretendExecutable(t, exe)
		app, out, _ := newApp(t)

		if err := run(context.Background(), app, options{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if body, _ := os.ReadFile(exe); string(body) != "the new program" {
			t.Errorf("expected the new program in place, got: %q", body)
		}
		if !strings.Contains(out.String(), "v0.1.1 installed") {
			t.Errorf("expected the install to be reported, got: %q", out.String())
		}
	})

	// Verify that a platform with no build is refused at once, without waiting
	// on a request that was never going to help
	t.Run("Unsupported", func(t *testing.T) {
		pretendPlatform(t, "freebsd", "amd64")
		app, _, _ := newApp(t)

		err := run(context.Background(), app, options{check: true})
		if err == nil || !strings.Contains(err.Error(), "freebsd/amd64") {
			t.Errorf("expected the platform to be refused, got: %v", err)
		}
	})

	// Verify that the daemon will not replace the program it is running from
	t.Run("InDaemon", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		app, _, _ := newApp(t)
		app.InDaemon = true

		err := run(context.Background(), app, options{})
		if err == nil || !strings.Contains(err.Error(), "daemon") {
			t.Errorf("expected the daemon to refuse, got: %v", err)
		}
	})

	// Verify that a release that cannot be read is reported
	t.Run("NoRelease", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		server := serve(t, func(w http.ResponseWriter, r *http.Request) {})
		server.Close()
		app, _, _ := newApp(t)

		if err := run(context.Background(), app, options{}); err == nil {
			t.Error("expected the failed lookup to be reported")
		}
	})

	// Verify that a build from a source checkout refuses to overwrite itself,
	// since it was almost certainly built to test something
	t.Run("DevBuild", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, devVersion)
		releaseServer(t, "v0.1.1", "")
		app, _, _ := newApp(t)

		if err := run(context.Background(), app, options{}); err != errDevBuild {
			t.Errorf("expected a development build to refuse, got: %v", err)
		}
	})

	// Verify that --force is the way past that refusal
	t.Run("DevBuildForced", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, devVersion)
		releaseServer(t, "v0.1.1", "")
		exe := installedProgram(t)
		pretendExecutable(t, exe)
		app, _, _ := newApp(t)

		if err := run(context.Background(), app, options{force: true}); err != nil {
			t.Fatalf("expected --force to install, got: %v", err)
		}
		if body, _ := os.ReadFile(exe); string(body) != "the new program" {
			t.Errorf("expected the new program in place, got: %q", body)
		}
	})

	// Verify that being up to date downloads nothing and is not an error
	t.Run("AlreadyCurrent", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.1.1")
		releaseServer(t, "v0.1.1", "")
		app, _, notes := newApp(t)

		if err := run(context.Background(), app, options{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !strings.Contains(notes.String(), "Nothing to do") {
			t.Errorf("expected it to say there is nothing to do, got: %q", notes.String())
		}
	})

	// Verify that a build newer than the release is refused rather than quietly
	// downgraded
	t.Run("Ahead", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.2.0")
		releaseServer(t, "v0.1.1", "")
		app, _, _ := newApp(t)

		err := run(context.Background(), app, options{})
		if err == nil || !strings.Contains(err.Error(), "--force") {
			t.Errorf("expected a newer build to be refused, got: %v", err)
		}
	})

	// Verify that a program which cannot say where it is fails before anything
	// is downloaded
	t.Run("Unlocatable", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.1.0")
		releaseServer(t, "v0.1.1", "")
		app, _, _ := newApp(t)

		was := executablePath
		executablePath = func() (string, error) { return "", os.ErrNotExist }
		t.Cleanup(func() { executablePath = was })

		if err := run(context.Background(), app, options{}); err == nil {
			t.Error("expected not knowing the path to be an error")
		}
	})

	// Verify that an install which cannot be finished is reported rather than
	// printed as a success
	t.Run("InstallFails", func(t *testing.T) {
		pretendPlatform(t, "darwin", "arm64")
		pretendVersion(t, "v0.1.0")
		releaseServer(t, "v0.1.1", "")
		exe := installedProgram(t)
		pretendExecutable(t, exe)
		app, out, _ := newApp(t)

		was := renameFile
		renameFile = func(string, string) error { return os.ErrPermission }
		t.Cleanup(func() { renameFile = was })

		if err := run(context.Background(), app, options{}); err == nil {
			t.Fatal("expected the failed install to be reported")
		}
		if out.String() != "" {
			t.Errorf("expected nothing printed as an answer, got: %q", out.String())
		}
	})
}

// Test_sudoHint tests the advice given when the program's directory belongs to
// somebody else.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Unix: the sudo line, and the refusal to run it for you
//   - Windows: administrator rights instead, with no sudo
//   - RepeatsFlags: the flags that were typed are part of the suggestion
func Test_sudoHint(t *testing.T) {
	// Verify that the advice gives the exact line to type and says plainly that
	// the tool will not escalate on its own
	t.Run("Unix", func(t *testing.T) {
		got := sudoHint("darwin", options{})

		if !strings.Contains(got, "sudo radiocli update") {
			t.Errorf("expected the sudo line, got: %q", got)
		}
		if !strings.Contains(got, "will not do that for you") {
			t.Errorf("expected it to say it will not escalate, got: %q", got)
		}
	})

	// Verify that Windows is told about administrator rights rather than sudo,
	// which does not exist there
	t.Run("Windows", func(t *testing.T) {
		got := sudoHint("windows", options{})

		if strings.Contains(got, "sudo") {
			t.Errorf("expected no mention of sudo on Windows, got: %q", got)
		}
		if !strings.Contains(got, "administrator") {
			t.Errorf("expected administrator rights, got: %q", got)
		}
	})

	// Verify that the suggestion carries the flags that were typed, so it is
	// the command they meant rather than a different one
	t.Run("RepeatsFlags", func(t *testing.T) {
		got := sudoHint("linux", options{version: "v0.1.0", force: true})

		if !strings.Contains(got, "sudo radiocli update --version v0.1.0 --force") {
			t.Errorf("expected the flags to be repeated, got: %q", got)
		}
	})
}
