// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/15/2026

//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// group puts the tests in a process group of their own.
//
// Setpgid makes the child's group id its own process id, so the group can be
// signalled later by negating that. Everything the tests start inherits the
// group, which is what makes one signal reach "go test" and the test binary it
// runs without either of them having to pass anything on.
func group(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// interrupt passes a signal on to the tests.
//
// The group is signalled rather than the process, for the reason above. If that
// fails, which it does if the tests have already exited or were never given a
// group, the process is signalled directly: reaching go test alone is worth
// more than reaching nobody.
func interrupt(cmd *exec.Cmd, s os.Signal) {
	if cmd.Process == nil {
		return
	}

	sig, ok := s.(syscall.Signal)
	if !ok {
		_ = cmd.Process.Signal(s)
		return
	}

	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil {
		_ = cmd.Process.Signal(s)
	}
}
