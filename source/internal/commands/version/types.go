// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package version

// info is the rendered result. It is a named type rather than an inline map so
// the JSON output has a fixed, reviewable shape.
type info struct {
	// Version, Commit and Date describe the build itself, and come from
	// buildinfo, which is stamped at link time. A build that was not stamped
	// reports buildinfo's placeholders rather than nothing.
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`

	// Go, OS and Arch describe what built it and what it runs on, read from
	// the runtime rather than stamped in. They are what turns "it does not
	// work here" into a report somebody can act on.
	Go   string `json:"go"`
	OS   string `json:"os"`
	Arch string `json:"arch"`
}
