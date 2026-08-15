// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package cli holds an end to end test suite for radiocli, run against a real
// scanner attached to this computer.
//
// Nothing here is a unit test. Every test builds the tool, runs it as a person
// would, and checks what the scanner did about it, which is the only way to
// tell whether a command that drives a radio's menus actually works.
//
// The suite is deliberately its own module. It imports nothing from the tool,
// so it tests the command line surface rather than the code behind it, and a
// refactor that keeps the commands working keeps these tests passing.
//
// # The order tests run in
//
// Go runs tests in the order their files sort, then in the order they are
// written within a file. That is the only lever there is, so the slow tests are
// in files whose names begin with z_ and run last.
//
// Slow here means typing. Creating or renaming anything means spelling a name
// into the scanner one character at a time, which takes ten seconds or more per
// entry, and those tests together are most of a run. Sitting them at the end
// means a run reports nearly everything it knows in the first minute or two,
// and a run stopped early has still answered most of the questions.
//
// Renaming those files back is enough to lose this, which is why each one says
// so at the top.
package suite

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// The flags that decide how much of the scanner a run is allowed to touch.
//
// A run does everything by default: settings, tuning, menus, location, what is
// scanned, and the entries the suite creates and deletes for itself. It is safe
// to, because the scanner is read before anything is changed and put back
// afterwards, a run stopped part way still puts it back, and a run killed
// outright is put back by the next one.
//
// Pass -writes=false for a run that only reads, which is the one to reach for
// against a radio whose contents matter and which nobody has tested against
// before.
var (
	writes = flag.Bool("writes", true,
		"run the tests that change the scanner: volume, tuning, menus, location, what is "+
			"scanned, and the entries the suite creates and deletes for itself")
	pace = flag.String("pace", "",
		"value passed as --pace to every command (default: whatever the tool chooses)")
	port = flag.String("port", "",
		"serial port of the scanner to test against (default: whichever one is attached)")
)

// commandTimeout caps a single run of the tool. It is generous because it has
// to cover the slowest thing the tool does: "location gps --wait" sits through
// up to 90 seconds of satellite hunting, and a rename types a name one
// character at a time.
const commandTimeout = 4 * time.Minute

// harness is everything the tests share: the binary under test, the config
// file it reads, and the scanner it is pointed at.
var harness struct {
	binary string
	config string

	// device is the port of the scanner the tests found, and what execute
	// passes as --device to every command. Empty means none is attached, and
	// every test that needs one skips.
	device string
	model  string

	// before is what the scanner held when the run started, so the run can put
	// it back.
	before state
}

// TestMain builds the tool, finds a scanner, remembers what it was doing, and
// puts it back afterwards.
//
// The restore runs whether the tests passed or failed, because a failed test
// is exactly the one most likely to have left the scanner somewhere odd.
func TestMain(m *testing.M) {
	flag.Parse()

	dir, err := os.MkdirTemp("", "radiocli-tests")
	if err != nil {
		log.Fatalf("making a working directory: %v", err)
	}
	defer os.RemoveAll(dir)

	if err := build(dir); err != nil {
		log.Fatalf("building the tool: %v", err)
	}

	// The tests get their own config file, so a run never reads or writes the
	// one belonging to whoever is running it.
	harness.config = filepath.Join(dir, "config.json")
	if err := writeConfig(harness.config); err != nil {
		log.Fatalf("writing the test config: %v", err)
	}

	if err := findScanner(); err != nil {
		log.Printf("no scanner to test against: %v", err)
		log.Printf("the tests that need one will skip")
	}

	if harness.device != "" && *writes {
		// Anything an earlier run was killed in the middle of goes back before
		// this one reads what to put back, so that a run which died holding the
		// volume at 3 does not make 3 the volume this run restores.
		recoverPreviousRun()

		if harness.before, err = readState(); err != nil {
			log.Fatalf("reading what the scanner is doing: %v", err)
		}
		log.Printf("scanner state before the run: %s", harness.before)

		// Written down before anything changes it, so that a run killed outright
		// leaves the next one enough to put the scanner back with.
		writeJournal(harness.before)
	}

	watchInterrupts()

	code := m.Run()

	if harness.device != "" && *writes {
		// The entries the tests worked in go first, so that what the restore
		// reads back afterwards is the scanner as it will be left.
		removeScratch()
		restore(harness.before)

		// Last, and only once the restore has actually run. The note being gone
		// is what tells the next run that this one finished tidily.
		clearJournal()
	}

	os.Exit(code)
}

// interrupted reports that the run has been stopped from the keyboard, and
// that no further test should begin.
var interrupted atomic.Bool

// watchInterrupts makes a run stopped from the keyboard stop tidily.
//
// Without this, Ctrl-C kills the process where it stands. That is worse here
// than in an ordinary test suite, because the suite is halfway through building
// favorites lists on somebody's radio: the entries it made are left there, and
// so are the volume, position and choice of scanned lists it changed.
//
// So the first interrupt stops new tests starting rather than stopping the
// process. Whatever is running finishes, its own cleanup runs, and TestMain
// reaches the restore the way it would after an ordinary run.
//
// The test in progress is deliberately left to finish rather than killed. A
// command that is halfway through typing a name into the scanner, cut off,
// leaves the scanner sitting on an entry screen with half a name in it, which
// is a worse place to be abandoned than a few seconds later. Every command is
// bounded by commandTimeout, so this cannot wait indefinitely.
//
// A run started by report is stopped through the file watchStopFile watches
// for rather than through this, because a signal cannot be got here reliably
// past go test. This is what answers a Ctrl-C typed at a plain "go test".
func watchInterrupts() {
	watchStopFile()

	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		<-signals
		if !interrupted.Swap(true) {
			log.Printf("stopping: no further tests will start, and the scanner will be put back")
			log.Printf("the test already running has to finish first, so this takes a few seconds")
			log.Printf("interrupt again to stop at once, leaving the scanner as it is")
		}

		<-signals
		log.Printf("stopping at once: what this run created is still on the scanner, and its " +
			"settings are as this run left them")
		log.Printf("the next run puts both back, from the note this run wrote " +
			"before it started")
		os.Exit(130)
	}()
}

// build compiles the tool into dir. It is built once and reused, so the cost
// is paid once however many tests run.
func build(dir string) error {
	harness.binary = filepath.Join(dir, "radiocli")

	cmd := exec.Command("go", "build", "-o", harness.binary, ".")
	cmd.Dir = "../../source"
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

// writeConfig writes the config file the tests run with.
//
// The scanner is deliberately not in it. The tool does not remember one, so
// every command execute runs carries --device, which is also how a real caller
// runs it.
func writeConfig(path string) error {
	cfg := map[string]any{"output": "text", "pace": "turbo"}

	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

// findScanner works out which scanner the run is talking to, and confirms it
// answers before any test bothers trying.
func findScanner() error {
	res, err := execute("devices", "-o", "json")
	if err != nil {
		return err
	}
	if res.code != 0 {
		return fmt.Errorf("listing the attached scanners: %s", res.stderr)
	}

	type listed struct {
		Port  string `json:"port"`
		Model string `json:"model"`
		Busy  bool   `json:"busy"`
	}
	var found []listed
	if err := json.Unmarshal([]byte(res.stdout), &found); err != nil {
		return fmt.Errorf("reading the list of scanners: %w", err)
	}

	// A busy scanner is attached and working, and simply held by another
	// invocation of the tool. It cannot be tested against, but calling it a
	// missing scanner would send somebody to go and check a cable that is
	// fine, so the two are counted apart.
	var usable []listed
	busy := 0
	for _, d := range found {
		if d.Busy {
			busy++
			continue
		}
		usable = append(usable, d)
	}
	if len(usable) == 0 {
		if busy > 0 {
			return errors.New("every scanner attached is in use by another radiocli")
		}
		return errors.New("none are attached")
	}

	// The port named with -port when there is one, otherwise the first that
	// answered. A -port naming something that did not answer is a mistake
	// worth reporting, rather than quietly testing a different scanner.
	pick := usable[0]
	if *port != "" {
		named := false
		for _, d := range usable {
			if d.Port == *port {
				pick, named = d, true
				break
			}
		}
		if !named {
			return fmt.Errorf("no scanner answered on %s", *port)
		}
	}

	harness.device, harness.model = pick.Port, pick.Model

	// Being listed only means the port exists. Talking to it is the test. This
	// is also the first call that carries --device, because execute adds it
	// once harness.device is known.
	res, err = execute("status")
	if err != nil {
		return err
	}
	if res.code != 0 {
		return fmt.Errorf("the scanner on %s is not answering: %s", harness.device, res.stderr)
	}

	log.Printf("testing against a %s on %s", harness.model, harness.device)
	return nil
}

// result is one run of the tool.
type result struct {
	args   []string
	stdout string
	stderr string
	code   int
	took   time.Duration
}

// execute runs the tool with args and reports what happened. A non-zero exit
// is a result rather than an error: plenty of tests are about the tool
// refusing something. The error return is for the run not happening at all.
func execute(args ...string) (result, error) {
	if err := permitted(args); err != nil {
		return result{}, err
	}

	// Global flags go in front of the command, where cobra accepts them
	// whatever the command's own arguments look like.
	full := []string{"--config", harness.config}

	// The tool remembers no scanner, so every command that needs one has to be
	// told. It goes in front of args rather than after, so a test that names a
	// port of its own overrides this one: pflag takes the last it is given.
	// Empty until findScanner has run, which is why "devices" can discover.
	if harness.device != "" {
		full = append(full, "--device", harness.device)
	}
	if *pace != "" {
		full = append(full, "--pace", *pace)
	}
	full = append(full, args...)

	var stdout, stderr strings.Builder
	cmd := exec.Command(harness.binary, full...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// A hung scanner should fail the test that hung it rather than the whole
	// run, so the command is killed rather than waited on forever.
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return result{}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var err error
	select {
	case err = <-done:
	case <-time.After(commandTimeout):
		_ = cmd.Process.Kill()
		<-done
		return result{}, fmt.Errorf("radiocli %s did not finish within %s",
			strings.Join(args, " "), commandTimeout)
	}

	res := result{
		args:   args,
		stdout: stdout.String(),
		stderr: stderr.String(),
		took:   time.Since(started),
	}

	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		res.code = exit.ExitCode()
	default:
		return res, err
	}
	return res, nil
}

// permitted refuses the one command that must never run.
//
// "systems" on the full database returns a short, wrong answer and then leaves
// the scanner's serial interface dead until it is power cycled. It has been
// reproduced twice. A test suite that does it by accident costs somebody a
// walk across the room, so it is blocked here rather than merely documented.
//
// The tool refuses it too. This stays because it stops the request before the
// binary runs at all, and because a suite relying on the thing under test to
// protect it is only protected while that thing is correct.
//
// Nothing here looks at where an argument sits, and that is the whole lesson of
// the bug this replaced. The old guard matched args[0] against "systems", which
// was true of the arguments a test wrote and false of the arguments that
// actually ran: mustJSON and readJSON both put "-o" and "json" in front, which
// pushed the command word to index two and let the one command that must never
// run straight through, on the path most of the suite uses. A rule that depends
// on counting positions is a rule that a helper can quietly break by adding a
// flag, so this one counts nothing.
//
// Refusing on the two words appearing anywhere costs nothing real. "Full
// Database" is not a name any other command in this suite passes, and the
// alternative, "scanning systems", takes no name at all.
func permitted(args []string) error {
	named, systems := false, false
	for _, a := range args {
		switch {
		case strings.EqualFold(a, "full database"):
			named = true
		case strings.EqualFold(a, "systems"):
			systems = true
		}
	}

	if named && systems {
		return errors.New(`"systems \"Full Database\"" locks the scanner up until it is ` +
			"power cycled, so this suite will not run it. Use \"scanning systems\" instead")
	}
	return nil
}

// run runs the tool and reports how long it took, which is worth knowing for a
// tool whose slowest commands take ten seconds or more.
func run(t *testing.T, args ...string) result {
	t.Helper()

	res, err := execute(args...)
	if err != nil {
		t.Fatalf("running %q: %v", "radiocli "+strings.Join(args, " "), err)
	}
	t.Logf("radiocli %s (%d, %s)", strings.Join(args, " "), res.code, res.took.Round(time.Millisecond))
	return res
}

// mustRun runs the tool and fails the test unless it succeeded.
func mustRun(t *testing.T, args ...string) result {
	t.Helper()

	res := run(t, args...)
	if res.code != 0 {
		t.Fatalf("radiocli %s exited %d, wanted 0\nstderr: %s",
			strings.Join(args, " "), res.code, res.stderr)
	}
	return res
}

// mustFail runs the tool and fails the test unless it was refused. The message
// is checked too: a refusal that does not say why is not much use, and every
// one of them is documented as starting with "error: ".
func mustFail(t *testing.T, want string, args ...string) result {
	t.Helper()

	res := run(t, args...)
	if res.code == 0 {
		t.Fatalf("radiocli %s was accepted, wanted it refused\nstdout: %s",
			strings.Join(args, " "), res.stdout)
	}
	if res.code != 1 {
		t.Errorf("radiocli %s exited %d, wanted 1", strings.Join(args, " "), res.code)
	}
	if !strings.HasPrefix(res.stderr, "error: ") {
		t.Errorf("radiocli %s reported %q, wanted it to start with \"error: \"",
			strings.Join(args, " "), firstLine(res.stderr))
	}
	if want != "" && !strings.Contains(res.stderr, want) {
		t.Errorf("radiocli %s reported %q, wanted it to mention %q",
			strings.Join(args, " "), firstLine(res.stderr), want)
	}
	return res
}

// mustJSON runs the tool asking for JSON and decodes stdout into v.
//
// Decoding is itself the check that the tool keeps its two streams apart: a
// note or a progress message written to stdout by mistake stops the result
// being parseable.
func mustJSON(t *testing.T, v any, args ...string) result {
	t.Helper()

	res := mustRun(t, append([]string{"-o", "json"}, args...)...)
	if err := json.Unmarshal([]byte(res.stdout), v); err != nil {
		t.Fatalf("radiocli %s did not print JSON: %v\nstdout: %s",
			strings.Join(args, " "), err, res.stdout)
	}
	return res
}

// firstLine trims a message down to the part worth quoting in a failure.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// needScanner skips a test when there is nothing to test against, or when the
// run is stopping.
//
// This is where an interrupted run actually stops. Every test that touches the
// scanner comes through here, so refusing at this point means nothing new is
// started, while everything already under way finishes and tidies up after
// itself.
func needScanner(t *testing.T) {
	t.Helper()
	if interrupted.Load() {
		t.Skip("the run was stopped from the keyboard")
	}
	if harness.device == "" {
		t.Skip("no scanner is attached")
	}
}

// needWrites skips a test that would change the scanner unless the run asked
// for it.
func needWrites(t *testing.T) {
	t.Helper()
	needScanner(t)
	if !*writes {
		t.Skip("this test changes the scanner, and -writes=false was passed")
	}
}

// The name everything this suite creates is called, or starts with.
//
// It is deliberately obvious. Each test deletes what it made, and the run
// deletes anything still standing on its way out, but a run killed between the
// two leaves something behind, and this is what makes it recognisable on a
// scanner full of real entries.
const testName = "RADIOCLI TEST"

// stopVariable names the environment variable holding the path to watch for.
// report sets it before starting the tests, and writes the file to stop them.
const stopVariable = "RADIOCLI_TEST_STOP"

// watchStopFile stops the run when report asks it to.
//
// A signal cannot be relied on to get here. The tests are run by "go test",
// which sits between report and this process: signalling go test kills the
// suite outright, half way through whatever it was typing into the scanner,
// and there is no dependable way to signal the suite alone, because which
// process it is depends on how go test chose to run it.
//
// So report writes a file and this watches for it. The effect is the same as
// Ctrl-C: the test in progress finishes, nothing new starts, and TestMain
// reaches the restore. The signal handler beside this stays for the case it
// does work, which is a Ctrl-C typed at a terminal, where the kernel delivers
// to every process in the group and this hears it directly.
func watchStopFile() {
	path := os.Getenv(stopVariable)
	if path == "" {
		// Started by "go test" rather than by report, where there is nobody to
		// write the file and the signal handler is all there is.
		return
	}

	go func() {
		for {
			time.Sleep(250 * time.Millisecond)
			if _, err := os.Stat(path); err != nil {
				continue
			}
			if interrupted.Swap(true) {
				return
			}
			log.Printf("stopping: no further tests will start, and the scanner will be put back")
			log.Printf("the test already running has to finish first, so this takes a few seconds")
			return
		}
	}()
}
