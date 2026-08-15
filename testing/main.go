// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/4/2026

// Command testing runs radiocli's end to end test suite and draws it as a
// command tree.
//
// The suite itself is the package below this one, and this is the thing to run:
//
//	cd testing
//	go run .
//
// The suite drives a radio, so a run takes minutes and most of that time is
// spent watching the scanner do something. Go's own output is written for a
// build server reading a log afterwards, which is the wrong shape for sitting
// and following along. This draws the tool's own command tree instead, rooted
// at RADIOCLI, with every command and subcommand nested the way the tool nests
// them and every test hanging off the command it tests.
//
// The tree is drawn as the run goes rather than at the end, so a five minute
// run is something to watch rather than something to wait for. What that costs
// is the closing connector on a command: whether a command is the last of its
// parent is not known when its name has to be printed, so commands always draw
// as branches. The checks under them are exact, because each one is held back
// until the next line is known.
//
// It runs the tests rather than being piped into: the arguments are passed
// straight through to "go test", and the exit code is the one go test gave.
//
//	go run .
//	go run . -run TestVolume
//	go run . -writes=false
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/albeebe/radiocli/testing/suite"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run starts the tests and renders their output. It returns the exit code go
// test gave, so this stands in for it anywhere a script checks the result.
func run(args []string) int {
	r := newRenderer()

	// Our own flag, which is not go test's. Everything else is forwarded
	// untouched, so the suite's flags work exactly as they do without this.
	forward := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-logs" || a == "--logs" {
			r.logs = true
			continue
		}
		forward = append(forward, a)
	}

	// A radio is slower than Go's ten minute default, and caching a run
	// against hardware would be meaningless.
	full := []string{"test", "-json", "-count=1"}
	if !has(forward, "-timeout") {
		full = append(full, "-timeout", "30m")
	}
	full = append(full, forward...)

	// The tests live in their own package, so that this one can be the thing
	// you run. Named last, after whatever was forwarded, because that is where
	// go test looks for it.
	full = append(full, "./suite")

	cmd := exec.Command("go", full...)
	cmd.Stderr = os.Stderr

	// Where this tells the tests to stop. See forwardInterrupts.
	stop := filepath.Join(os.TempDir(), fmt.Sprintf("radiocli-stop-%d", os.Getpid()))
	defer os.Remove(stop)
	cmd.Env = append(os.Environ(), stopVariable+"="+stop)

	// A group of their own, so that the second interrupt can reach every one of
	// them at once.
	group(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read the test output: %v\n", err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "cannot start the tests: %v\n", err)
		return 1
	}

	// An interrupt asks the tests to stop and then waits for them. They answer
	// by finishing the test in progress and putting the scanner back, which
	// takes a few seconds, and this has to stay alive to show them: dying on
	// the same keypress would cut off the very output saying what was being put
	// back, and leave the tests writing to a pipe with nobody on the end.
	//
	// So this does not stop on the interrupt. It says what is happening and
	// waits: the tests decide when the run is over, and this stops when they
	// do.
	//
	// Saying it here rather than from the tests is deliberate. Anything they
	// print while a test is running belongs to that test, and is held back
	// unless it fails, so the notice never reached the screen. Whoever pressed
	// the key needs to be told at once that a pause of several seconds is the
	// scanner being put back rather than a hang.
	go forwardInterrupts(r, cmd, stop)
	go watchParent(r, stop)

	// Lines can be long: a failure quotes whatever the tool printed.
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for lines.Scan() {
		var e event
		if err := json.Unmarshal(lines.Bytes(), &e); err != nil {
			// Anything that is not an event is something Go wrote directly,
			// such as a compile error, and is worth seeing as it stands.
			r.passThrough(lines.Text())
			continue
		}
		r.handle(e)
	}

	// A line longer than the buffer, or a pipe that broke, ends the loop early
	// and would otherwise leave a report that looks finished and is not. The
	// exit code still comes from the tests themselves below, so this says what
	// happened rather than deciding the run failed.
	if err := lines.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "the report stopped early: %v\n", err)
	}

	r.summarise()

	if err := cmd.Wait(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "the tests did not finish: %v\n", err)
		return 1
	}
	return 0
}

// stopVariable names the environment variable holding the path the tests watch
// for. The suite reads the same name.
const stopVariable = "RADIOCLI_TEST_STOP"

// forwardInterrupts tells the tests to stop, and says what is happening while
// they wind down.
//
// It never stops the process itself. The tests own the shutdown, because they
// are the ones holding a half-built favorites list and a scanner whose volume
// and position have been changed, and this has nothing useful to add to that
// beyond staying out of the way and reporting it.
//
// The telling is done by writing a file rather than by passing the signal on,
// which looks roundabout and is not. The tests do not run here: "go test" runs
// them, and it sits between this process and the suite. Signal go test and it
// dies and takes the suite down with it, half way through typing a name into
// the scanner, which is the exact thing being avoided. Signal only the suite
// and there is no reliable way to find it, because which process that is comes
// down to how go test chose to run it.
//
// A file has none of those problems. The suite watches for it, and stops the
// way it stops for Ctrl-C: the test in progress finishes, nothing new starts,
// and the restore runs. It works the same on every platform, and go test never
// has to be involved.
//
// A second interrupt is a different instruction, and does go to the signal:
// stop now, wherever you are. That leaves the scanner as the run left it, which
// is recoverable, because the state was written down before the run began.
func forwardInterrupts(r *renderer, cmd *exec.Cmd, stop string) {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	<-signals
	askToStop(r, stop)

	for s := range signals {
		r.announce("stopping at once")
		interrupt(cmd, s)
	}
}

// watchParent treats losing the parent process as an interrupt.
//
// "go run ." is the documented way to start a run, and go run does not
// pass a signal on to what it built: killed, it dies on the spot and leaves
// this process running with no parent. Whoever sent that signal meant the run
// to stop, so this stops it, rather than carrying on driving somebody's radio
// for another five minutes after they thought they had cancelled.
func watchParent(r *renderer, stop string) {
	if os.Getppid() == 1 {
		// Started with no parent worth watching, so there is nothing to lose.
		return
	}

	for {
		time.Sleep(time.Second)
		if os.Getppid() == 1 {
			r.announce("whatever started this run has gone away, so the run is stopping")
			askToStop(r, stop)
			return
		}
	}
}

// askToStop writes the file the tests watch for, and says what happens next.
func askToStop(r *renderer, stop string) {
	if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
		r.announce(fmt.Sprintf("cannot tell the tests to stop: %v", err))
		return
	}

	r.announce("stopping: finishing the test that is running, then putting the scanner back")
	r.announce("this takes a few seconds and is not a hang. Interrupt again to stop at once, " +
		"and the next run puts the scanner back instead")
}

// event is one line of "go test -json".
type event struct {
	Action  string  `json:"Action"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// handle renders one event.
func (r *renderer) handle(e event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch {
	case e.Test == "":
		r.packageEvent(e)
	case e.Action == "run":
		r.started(e.Test)
	case e.Action == "output":
		r.collect(e)
	case e.Action == "pass", e.Action == "fail", e.Action == "skip":
		r.finished(e)
	}
}

// packageEvent renders the output that belongs to no test, which is the
// harness saying what it found, what it changed, and what it put back.
func (r *renderer) packageEvent(e event) {
	if e.Action != "output" {
		return
	}

	line := strings.TrimRight(e.Output, "\n")
	switch {
	case line == "", strings.HasPrefix(line, "PASS"), strings.HasPrefix(line, "FAIL"),
		strings.HasPrefix(line, "ok "), strings.HasPrefix(line, "---"),
		strings.HasPrefix(line, "?   "), strings.HasPrefix(line, "testing:"),
		strings.HasPrefix(line, "exit status"):
		return
	}

	r.clear()
	fmt.Fprintf(r.out, "%s\n", r.dim("· "+undated(line)))
}

// started notes a check beginning, and says so on the status line.
//
// Nothing is drawn into the tree here. A run is minutes long and the tree only
// arrives at the end, so this is what shows the run is moving.
func (r *renderer) started(name string) {
	if parent, ok := above(name); ok {
		r.subtests[parent]++
	}

	path, label, _ := r.locate(name)
	where := strings.Join(append([]string{"radiocli"}, path...), " ")
	if label != "" {
		where += " · " + label
	}
	r.status(where)
}

// collect keeps what a test printed, to show if it fails.
func (r *renderer) collect(e event) {
	line := strings.TrimRight(e.Output, "\n")
	if strings.TrimSpace(line) == "" {
		return
	}
	if strings.HasPrefix(strings.TrimSpace(line), "=== ") ||
		strings.HasPrefix(strings.TrimSpace(line), "--- ") {
		return
	}

	r.output[e.Test] = append(r.output[e.Test], strings.TrimSpace(line))
}

// finished files a result under the command it belongs to.
func (r *renderer) finished(e event) {
	defer delete(r.output, e.Test)

	if e.Action == "fail" {
		if parent, ok := above(e.Test); ok {
			r.broke[parent] = true
		}
	}

	// A test whose checks each have a line of their own has already said
	// everything it has to say. Drawing it as well would count every one of
	// those checks twice, and a failure underneath it is the same failure
	// reported at both levels. What is left is its own body, which is worth a
	// leaf only when that body is what went wrong.
	if r.subtests[e.Test] > 0 {
		switch {
		case e.Action == "pass", e.Action == "fail" && r.broke[e.Test]:
			return
		}
	}

	path, label, ok := r.locate(e.Test)
	if !ok {
		// A name that matches no command. The guard in the suite is what stops
		// this happening; showing it under the root is better than dropping it.
		label = e.Test
	}
	if label == "" {
		label = "the command itself"
	}

	c := &check{
		label:   label,
		failed:  e.Action == "fail",
		skipped: e.Action == "skip",
		mark:    r.mark(e.Action, e.Elapsed),
	}

	switch e.Action {
	case "pass":
		r.passed++
		if r.logs {
			c.detail = r.output[e.Test]
		}

	case "skip":
		r.skipped++
		if why := r.reason(e.Test); why != "" {
			c.detail = []string{why}
		}

	case "fail":
		r.failed++
		c.detail = r.output[e.Test]

		where := strings.Join(append([]string{"radiocli"}, path...), " ")
		r.failures = append(r.failures, where+" · "+label)
	}

	// The heading first, then the check, which is printed one line late so that
	// the last check under a command can close its branch.
	c.gutter = r.place(path)
	r.hold(c)
}

// locate splits a test's name into the command it tests and the name of the
// check itself.
//
// The command comes from suite.Split, which is the same reading the suite's
// own naming guard uses, so a test that renders in the wrong place and a test
// that fails the guard are always the same test.
func (r *renderer) locate(name string) (path []string, label string, ok bool) {
	function, sub, nested := strings.Cut(name, "/")

	path, variant, ok := suite.Split(function)

	switch {
	case nested:
		// Go writes spaces in a subtest's name as underscores, and nests
		// deeper ones with slashes.
		label = strings.ReplaceAll(strings.ReplaceAll(sub, "_", " "), "/", " > ")
	case variant != "":
		label = strings.ToLower(suite.Words(variant))
	}

	return path, label, ok
}

// above names the test a check sits directly inside, if it sits inside one.
//
// Go writes that nesting into the name, so "TestChannels/names_only" is a
// check inside TestChannels, and a check inside that one would be a third
// segment. Only the immediate parent matters here, because every level counts
// its own children.
func above(name string) (string, bool) {
	i := strings.LastIndex(name, "/")
	if i < 0 {
		return "", false
	}
	return name[:i], true
}

// reason is why a test skipped, without the file and line it was written on.
func (r *renderer) reason(test string) string {
	lines := r.output[test]
	if len(lines) == 0 {
		return ""
	}

	last := lines[len(lines)-1]
	if _, rest, found := strings.Cut(last, ".go:"); found {
		if _, message, found := strings.Cut(rest, ": "); found {
			return message
		}
	}
	return last
}

// undated strips the date and time the standard logger stamps on the harness's
// own messages, which nobody watching a run in real time needs.
func undated(line string) string {
	fields := strings.SplitN(line, " ", 3)
	if len(fields) < 3 {
		return line
	}
	if strings.Count(fields[0], "/") == 2 && strings.Count(fields[1], ":") == 2 {
		return fields[2]
	}
	return line
}

// has reports whether a flag was passed, in either of the forms Go accepts.
func has(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}
