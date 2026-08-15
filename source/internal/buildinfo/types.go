// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package buildinfo carries the identity of the running binary.
//
// The values are stamped at link time so a released binary can report exactly
// what it was built from:
//
//	go build -ldflags "\
//	  -X github.com/albeebe/radiocli/internal/buildinfo.Version=v1.2.3 \
//	  -X github.com/albeebe/radiocli/internal/buildinfo.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/albeebe/radiocli/internal/buildinfo.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package buildinfo

// Stamped at link time. The defaults are what an unstamped local build reports.
var (
	// Commit is the git revision the binary was built from.
	Commit = "none"

	// Date is the build timestamp in RFC 3339 format.
	Date = "unknown"

	// Version is the release version, such as v1.2.3.
	Version = "dev"
)
