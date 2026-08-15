// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/3/2026

package suite

import (
	"runtime"
	"strings"
	"testing"
)

// TestVersion checks the one command that needs no scanner at all. It reports
// what the binary was built from, and the parts it takes from the Go runtime
// can be checked exactly.
func TestVersion(t *testing.T) {
	var report struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
		Go      string `json:"go"`
		OS      string `json:"os"`
		Arch    string `json:"arch"`
	}
	mustJSON(t, &report, "version")

	if report.Version == "" {
		t.Error("the version is empty")
	}
	if report.OS != runtime.GOOS {
		t.Errorf("os is %q, wanted %q", report.OS, runtime.GOOS)
	}
	if report.Arch != runtime.GOARCH {
		t.Errorf("arch is %q, wanted %q", report.Arch, runtime.GOARCH)
	}
	if !strings.HasPrefix(report.Go, "go1.") {
		t.Errorf("go is %q, wanted a Go version", report.Go)
	}

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "version")

		for _, label := range []string{"radiocli " + report.Version, "commit:", "built:", "go:"} {
			if !strings.Contains(res.stdout, label) {
				t.Errorf("the text output has no %q line:\n%s", label, res.stdout)
			}
		}
		if !strings.Contains(res.stdout, runtime.GOOS+"/"+runtime.GOARCH) {
			t.Errorf("the text output does not say what it was built for:\n%s", res.stdout)
		}
	})
}
