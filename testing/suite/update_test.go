// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package suite

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

// TestUpdate checks the command that replaces the tool itself.
//
// It never installs anything. The suite builds the tool from source, so the
// binary under test always reports its version as "dev", and a development
// build refuses to replace itself without --force. That refusal is checked
// here, but only after confirming the build really is one, because a stamped
// binary would install for real and replace the very program these tests are
// running.
//
// It is also the one test that reaches the network. GitHub is asked about the
// newest release, so a machine with no connection, or one that has spent its
// hourly allowance of requests, skips rather than failing: neither says
// anything about the scanner or the tool.
func TestUpdate(t *testing.T) {
	var report struct {
		Current         string `json:"current"`
		Latest          string `json:"latest"`
		UpdateAvailable bool   `json:"updateAvailable"`
		State           string `json:"state"`
		Dev             bool   `json:"dev"`
		Pinned          bool   `json:"pinned"`
		Asset           string `json:"asset"`
		Platform        string `json:"platform"`
		URL             string `json:"url"`
	}

	res := run(t, "-o", "json", "update", "--check")
	if res.code != 0 {
		for _, offline := range []string{"rate limiting", "asking GitHub", "no such host"} {
			if strings.Contains(res.stderr, offline) {
				t.Skipf("GitHub could not be reached: %s", firstLine(res.stderr))
			}
		}
		t.Fatalf("radiocli update --check exited %d, wanted 0\nstderr: %s", res.code, res.stderr)
	}
	if err := json.Unmarshal([]byte(res.stdout), &report); err != nil {
		t.Fatalf("radiocli update --check did not print JSON: %v\nstdout: %s", err, res.stdout)
	}

	if report.Latest == "" || !strings.HasPrefix(report.Latest, "v") {
		t.Errorf("the newest release is %q, wanted a tag such as v0.1.1", report.Latest)
	}
	if report.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Errorf("the platform is %q, wanted %s/%s", report.Platform, runtime.GOOS, runtime.GOARCH)
	}
	if report.Asset == "" {
		t.Error("no release file was named for this platform")
	}
	if report.Pinned {
		t.Error("the report says it was pinned, but no version was named")
	}
	if !strings.Contains(report.URL, report.Latest) {
		t.Errorf("the release page is %q, wanted it to name %s", report.URL, report.Latest)
	}

	// A build made from a checkout has no version, so there is nothing to
	// compare and nothing on offer.
	if report.Dev {
		if report.State != "unknown" {
			t.Errorf("a development build reports state %q, wanted unknown", report.State)
		}
		if report.UpdateAvailable {
			t.Error("a development build says an update is available, wanted false")
		}
	}

	t.Run("printing it as text", func(t *testing.T) {
		out := mustRun(t, "update", "--check").stdout

		if !strings.Contains(out, report.Latest) {
			t.Errorf("the text output does not name the release:\n%s", out)
		}
	})

	t.Run("refusing an argument it does not take", func(t *testing.T) {
		mustFail(t, "", "update", "nonsense")
	})

	t.Run("refusing a release nobody published", func(t *testing.T) {
		mustFail(t, "no release is tagged", "update", "--version", "v99.99.99")
	})

	t.Run("refusing to replace a development build", func(t *testing.T) {
		// Guarded, and the guard is the point. Running this against a stamped
		// binary would install a release over the program running these tests.
		if !report.Dev {
			t.Skipf("this build reports %s, not a development build", report.Current)
		}
		mustFail(t, "development build", "update")
	})
}
