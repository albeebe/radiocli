// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/15/2026

//go:build !unix

package main

import (
	"os"
	"os/exec"
)

// group does nothing where there are no process groups to put anything in.
func group(cmd *exec.Cmd) {}

// interrupt signals the tests directly.
//
// Without a process group this reaches "go test" and no further, so whether the
// suite itself hears it is go test's business rather than ours. Windows has no
// signals of the kind this relies on, and the suite is developed against a
// scanner on macOS and Linux, so this exists to keep the package building
// rather than to promise the same behaviour.
func interrupt(cmd *exec.Cmd, s os.Signal) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(s)
}
