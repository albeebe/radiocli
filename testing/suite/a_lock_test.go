// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

package suite

import (
	"strings"
	"sync"
	"testing"
)

// concurrentRuns is how many copies of the tool are started at once. It only
// has to be enough that they genuinely overlap; more would prove nothing extra
// and would take longer.
const concurrentRuns = 6

// TestRadiocli_OneCommandAtATime starts several copies of the tool together and checks
// that they do not talk over each other.
//
// The scanner answers on one line with nothing in a reply saying who asked for
// it, so two commands running at once read each other's answers. Before the
// port was claimed for the length of a command, this is what that looked like:
// a "scanning" and a "battery" run started together both failed, one on a
// response that stopped halfway and the other on no response at all, and both
// reported it as the scanner being unreachable.
//
// What this pins down is that no run ever sees that. A run either gets the
// scanner to itself or is told the scanner is busy, and those are the only two
// outcomes allowed.
func TestRadiocli_OneCommandAtATime(t *testing.T) {
	needScanner(t)

	results := runTogether(t, concurrentRuns, "scanning", "systems")

	refused := 0
	for _, res := range results {
		if res.code == 0 {
			continue
		}

		if !strings.Contains(res.stderr, "in use by another radiocli") {
			t.Errorf("a run that lost the race failed for the wrong reason, which means it was "+
				"talking over another run rather than being kept out of its way: %s",
				firstLine(res.stderr))
			continue
		}
		refused++
	}

	if refused == len(results) {
		t.Error("every run was refused, so none of them ever had the scanner")
	}
	t.Logf("%d of %d runs were refused, the rest had the scanner to themselves",
		refused, len(results))
}

// TestRadiocli_WaitQueuesBehindOne checks that --wait turns being refused
// into waiting a moment.
//
// Refusing is the default because a command that types a name holds the port
// for minutes, and blocking silently for that long looks like a hang. A caller
// that would rather queue says so, and then every run should get through
// rather than any of them being turned away.
func TestRadiocli_WaitQueuesBehindOne(t *testing.T) {
	needScanner(t)

	results := runTogether(t, concurrentRuns, "--wait", "3m", "scanning", "systems")

	for _, res := range results {
		if res.code != 0 {
			t.Errorf("a run passed --wait and was still refused, so it did not queue: %s",
				firstLine(res.stderr))
		}
	}
}

// runTogether starts n copies of the tool at the same moment and returns what
// each of them did.
//
// They are held at a barrier and released together rather than started in a
// loop, because a loop that starts them one after another can let each finish
// before the next begins, and then nothing has been tested.
func runTogether(t *testing.T, n int, args ...string) []result {
	t.Helper()

	var start sync.WaitGroup
	start.Add(1)

	var done sync.WaitGroup
	results := make([]result, n)
	errs := make([]error, n)

	for i := range n {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			results[i], errs[i] = execute(args...)
		}()
	}

	start.Done()
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("run %d of %q did not happen at all: %v",
				i+1, "radiocli "+strings.Join(args, " "), err)
		}
	}
	return results
}
