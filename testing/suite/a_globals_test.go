// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package suite

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// children asks a command what subcommands it has, by reading its own help.
//
// The tool is asked rather than the source, which is the point: this suite is
// its own module and imports nothing from the tool, so the only honest answer
// to "what commands are there" is the one the binary gives a person typing
// --help.
func children(t *testing.T, path []string) []string {
	t.Helper()

	res := mustRun(t, append(append([]string{}, path...), "--help")...)

	var found []string
	listing := false
	for _, line := range strings.Split(res.stdout, "\n") {
		if strings.HasPrefix(line, "Available Commands:") {
			listing = true
			continue
		}
		if !listing {
			continue
		}
		// The listing is one command per line and ends at the blank line
		// before the flags.
		if strings.TrimSpace(line) == "" {
			break
		}

		name := strings.Fields(line)[0]
		// cobra gives these to every command that has subcommands. They are
		// not the tool's, and nothing here documents or tests them.
		if name == "help" || name == "completion" {
			continue
		}
		found = append(found, name)
	}
	return found
}

// TestRadiocli_CommandChecklist is what makes the commands list above mean
// what it says.
//
// Without it the list was a checklist of whatever somebody remembered to add.
// TestRadiocli_Help walks it in one direction only, so a command with no entry was not a
// failure but an absence, and six whole subtrees went missing that way while
// each had e2e tests of its own. The diff runs both ways: a command the tool
// offers and the list does not name is a failure, and so is a name the tool no
// longer offers.
//
// None of this needs a scanner, because help is answered before the tool goes
// looking for one.
func TestRadiocli_CommandChecklist(t *testing.T) {
	listed := map[string]bool{}
	for _, c := range All {
		listed[strings.Join(c, " ")] = true
	}

	offered := map[string]bool{}
	var walk func(path []string)
	walk = func(path []string) {
		for _, name := range children(t, path) {
			child := append(append([]string{}, path...), name)
			offered[strings.Join(child, " ")] = true
			walk(child)
		}
	}
	walk(nil)

	for _, name := range slices.Sorted(maps.Keys(offered)) {
		if !listed[name] {
			t.Errorf("the tool offers %q but the checklist does not name it: add it, "+
				"along with whatever tests the command deserves", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(listed)) {
		if !offered[name] {
			t.Errorf("the checklist names %q but the tool does not offer it", name)
		}
	}
}

// TestRadiocli_Help checks that every command explains itself. None of this needs a
// scanner: help is answered before the tool goes looking for one.
func TestRadiocli_Help(t *testing.T) {
	t.Run("top level lists every command", func(t *testing.T) {
		res := mustRun(t, "--help")

		for _, c := range All {
			if len(c) > 1 {
				continue
			}
			if !strings.Contains(res.stdout, c[0]) {
				t.Errorf("the top level help does not mention %q", c[0])
			}
		}
	})

	// One check across all ninety commands rather than one check each. The
	// report draws every check as a line, and ninety of them under the root
	// buries the rest of the run before it starts. Nothing is lost by
	// collapsing them: each complaint below names the command it is about, and
	// a run that gets this far has already been told the list is complete.
	t.Run("every command explains itself", func(t *testing.T) {
		for _, c := range All {
			name := strings.Join(c, " ")
			res := mustRun(t, append(append([]string{}, c...), "--help")...)

			if !strings.Contains(res.stdout, "Usage:") {
				t.Errorf("the help for %q has no usage line:\n%s", name, res.stdout)
			}
			if !strings.Contains(res.stdout, "radiocli "+name) {
				t.Errorf("the usage for %q does not name the command:\n%s", name, res.stdout)
			}
		}
	})
}

// TestRadiocli_RejectsBadGlobalFlags checks the errors that any command can produce.
// They are documented in documentation/global_flags.md, and each one is
// checked here by the message a reader would actually see.
func TestRadiocli_RejectsBadGlobalFlags(t *testing.T) {
	t.Run("refusing an unknown command", func(t *testing.T) {
		mustFail(t, `unknown command "bogus"`, "bogus")
	})

	t.Run("refusing an unknown output format", func(t *testing.T) {
		mustFail(t, "invalid output format", "-o", "bogus", "status")
	})

	t.Run("refusing an unknown pace", func(t *testing.T) {
		mustFail(t, "invalid pace", "--pace", "bogus", "status")
	})

	t.Run("refusing an unknown flag", func(t *testing.T) {
		mustFail(t, "unknown flag", "--bogus", "status")
	})

	t.Run("a config file that is not there", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nowhere.json")
		mustFail(t, missing, "--config", missing, "status")
	})

	t.Run("a config file that is not json", func(t *testing.T) {
		broken := filepath.Join(t.TempDir(), "broken.json")
		if err := os.WriteFile(broken, []byte("not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		mustFail(t, "parsing", "--config", broken, "status")
	})
}

// TestRadiocli_ConfigFileSetsDefaults checks the rule that a flag left out is not the
// same as a flag set to its default: a format chosen in the config file has to
// survive a command that says nothing about the format.
func TestRadiocli_ConfigFileSetsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"output":"json"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// The later --config wins, so this run reads the file written above.
	res := mustRun(t, "--config", path, "version")

	var report map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &report); err != nil {
		t.Fatalf("the config file asked for JSON but the output was not JSON: %v\nstdout: %s",
			err, res.stdout)
	}

	// And a flag on the command line still beats the file.
	res = mustRun(t, "--config", path, "-o", "text", "version")
	if strings.HasPrefix(strings.TrimSpace(res.stdout), "{") {
		t.Errorf("--output text did not override the config file:\n%s", res.stdout)
	}
}

// TestRadiocli_VerboseGoesToStderr checks that debug logging never contaminates the
// result. Anything piped into another program reads stdout, so a log line in
// there is a broken pipeline.
func TestRadiocli_VerboseGoesToStderr(t *testing.T) {
	needScanner(t)

	quiet := mustRun(t, "-o", "json", "status")
	loud := mustRun(t, "-v", "-o", "json", "status")

	if quiet.stdout != loud.stdout {
		t.Errorf("--verbose changed the result:\nwithout: %s\nwith:    %s", quiet.stdout, loud.stdout)
	}
	if !strings.Contains(loud.stderr, "DEBUG") {
		t.Errorf("--verbose wrote no debug logging to stderr:\n%s", loud.stderr)
	}
	if strings.Contains(loud.stdout, "DEBUG") {
		t.Errorf("--verbose wrote debug logging to stdout:\n%s", loud.stdout)
	}
}

// TestRadiocli_VersionFlag checks the one flag the root command has to itself.
func TestRadiocli_VersionFlag(t *testing.T) {
	res := mustRun(t, "--version")

	if !strings.HasPrefix(res.stdout, "radiocli version ") {
		t.Errorf("--version printed %q, wanted a line starting \"radiocli version \"",
			firstLine(res.stdout))
	}
	if lines := strings.Count(strings.TrimSpace(res.stdout), "\n"); lines != 0 {
		t.Errorf("--version printed %d lines, wanted one:\n%s", lines+1, res.stdout)
	}
}

// TestRadiocli_ArgumentCounts checks that commands refuse the wrong number of
// arguments. Cobra answers these before the scanner is opened, so a typo costs
// nothing and reports the same way whether or not one is attached.
func TestRadiocli_ArgumentCounts(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"battery takes none", []string{"battery", "extra"}, "unknown command"},
		{"backlight keys toggle takes none", []string{"backlight", "keys", "toggle", "extra"}, "unknown command"},
		{"status takes none", []string{"status", "extra"}, "unknown command"},
		{"version takes none", []string{"version", "extra"}, "unknown command"},
		{"channels needs a department", []string{"channels"}, "accepts 1 arg"},
		{"systems needs a list", []string{"systems"}, "accepts 1 arg"},
		{"departments needs a system", []string{"departments"}, "accepts 1 arg"},
		{"tune needs a frequency", []string{"tune"}, "accepts 1 arg"},
		{"key needs a key", []string{"key"}, "requires at least 1 arg"},
		{"volume set needs a level", []string{"volume", "set"}, "accepts 1 arg"},
		{"location set needs a zip or a position", []string{"location", "set"}, "name a zip code"},
		{"systems rename needs two names", []string{"systems", "rename", "one"}, "accepts 2 arg"},
		{"channels new needs three", []string{"channels", "new", "one", "two"}, "accepts 3 arg"},
		{"channels delete needs two", []string{"channels", "delete", "one"}, "accepts 2 arg"},
		{"systems delete needs a system", []string{"systems", "delete"}, "accepts 1 arg"},
		{"config get needs a name", []string{"config", "get"}, "accepts 1 arg"},
		{"config set needs a name and a value", []string{"config", "set", "pace"}, "accepts 2 arg"},
		{"config path takes none", []string{"config", "path", "extra"}, "unknown command"},
		{"config macro show needs a name", []string{"config", "macro", "show"}, "accepts 1 arg"},
		{"config macro new needs a name and a step", []string{"config", "macro", "new", "one"}, "requires at least 2 arg"},
		{"config macro set needs a name and a step", []string{"config", "macro", "set", "one"}, "requires at least 2 arg"},
		{"config macro rename needs two names", []string{"config", "macro", "rename", "one"}, "accepts 2 arg"},
		{"config macro move needs a name and a direction", []string{"config", "macro", "move", "one"}, "accepts 2 arg"},
		{"config macro delete needs a name", []string{"config", "macro", "delete"}, "accepts 1 arg"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustFail(t, c.want, c.args...)
		})
	}
}

// TestRadiocli_FullDatabaseIsRefused checks the suite's own guard rail rather than the
// tool. Running "systems" on the full database leaves the scanner needing a
// power cycle, so the harness refuses to run it at all.
//
// The flag-prefixed forms are the ones that matter. The guard used to look only
// at the first argument, so it saw the command in the bare form below and
// missed it entirely once mustJSON or readJSON had put "-o json" in front,
// which is the form most of this suite runs commands in.
func TestRadiocli_FullDatabaseIsRefused(t *testing.T) {
	refused := []struct {
		name string
		args []string
	}{
		{"asking for it plainly", []string{"systems", "Full Database"}},
		{"asking for it with a short flag first", []string{"-o", "json", "systems", "Full Database"}},
		{"asking for it with a long flag first", []string{"--output", "json", "systems", "Full Database"}},
		{"asking for it in lower case", []string{"-v", "--pace", "turbo", "systems", "full database"}},
	}

	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			if _, err := execute(c.args...); err == nil {
				t.Fatal("the harness allowed the command that locks the scanner up")
			}
		})
	}

	// And it still lets the alternative through, which is the command the
	// refusal tells people to use.
	t.Run("scanning systems is allowed", func(t *testing.T) {
		if err := permitted([]string{"-o", "json", "scanning", "systems"}); err != nil {
			t.Fatalf("the guard refused the command it recommends: %v", err)
		}
	})
}
