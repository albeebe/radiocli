// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/15/2026

// Drawing a run as the tool's own command tree.
//
// The rest of this program starts the tests and reads what they report. This
// half decides what that looks like on screen: where a check hangs, which
// connector closes a branch, and what colour says how it went.

package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/albeebe/radiocli/testing/suite"
)

// check is one finished test, with whatever it printed underneath.
type check struct {
	gutter  string
	label   string
	mark    string
	detail  []string
	failed  bool
	skipped bool
}

// renderer turns the event stream into something worth watching.
type renderer struct {
	colour bool
	logs   bool

	// out is where everything goes. A field rather than os.Stdout directly, so
	// that the rendering can be read back and checked.
	out io.Writer

	// open is the command whose heading was printed last, and printed is every
	// command that has had something printed under it, which decides whether a
	// new subcommand needs a blank line above it.
	open    []string
	printed map[string]bool

	// held is the last check, kept back until the next line is known.
	//
	// This is what lets a streamed tree still close its branches. Whether a
	// check draws "├──" or "└──" is decided by what comes after it, so it is
	// printed one line late: when the next check under the same command turns
	// up it was not the last, and when anything else turns up it was.
	held *check

	// subtests counts the checks seen directly inside each test, and broke
	// records which of those tests had a check fail underneath them. Together
	// they decide whether a test is worth a leaf of its own: one whose checks
	// each have a line has already said everything, and Go marks it failed
	// merely because a check under it failed, which is the same failure told
	// twice. It earns a leaf when it has no checks at all, or when its own
	// body was what went wrong.
	subtests map[string]int
	broke    map[string]bool

	// output holds what each test printed, kept in case it fails and shown
	// only then, so a passing run stays readable.
	output map[string][]string

	passed  int
	failed  int
	skipped int

	// failures names what failed, as the command and the check, for the
	// summary at the end. A run against hardware is long enough that the
	// failure has scrolled away by the time it finishes.
	failures []string

	// showing is the status line currently on screen, which names the check
	// running now. It is rewritten in place rather than scrolled, and taken off
	// before anything else is printed.
	showing string

	// mu keeps the interrupt notice from being printed into the middle of
	// something the test stream is already writing. Everything else here
	// happens on one goroutine; that notice is the only thing that does not.
	mu sync.Mutex
}

// newRenderer builds a renderer ready to draw to the terminal.
func newRenderer() *renderer {
	return &renderer{
		colour:   colourful(),
		out:      os.Stdout,
		printed:  map[string]bool{},
		subtests: map[string]int{},
		broke:    map[string]bool{},
		output:   map[string][]string{},
	}
}

// announce prints a line of this tool's own, rather than the tests'.
func (r *renderer) announce(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clear()
	fmt.Fprintf(r.out, "%s\n", r.dim("· "+line))
}

// passThrough prints a line that is not an event, such as a compile error Go
// wrote straight to its output, exactly as it arrived.
//
// It takes the lock for the same reason handle does: the interrupt notice is
// printed from another goroutine, and a line written without the lock can be
// cut in half by one that is.
func (r *renderer) passThrough(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clear()
	fmt.Fprintln(r.out, line)
}

// status rewrites the line saying what is running, in place.
//
// Only on a terminal: anywhere else the escape would be noise in a log, and
// there is nothing watching it to be reassured anyway.
func (r *renderer) status(line string) {
	if !r.colour {
		return
	}

	r.clear()
	r.showing = line
	fmt.Fprintf(r.out, "\r%s", r.dim("· "+line))
}

// clear takes the status line back off, so whatever prints next starts on a
// line of its own rather than on top of it.
func (r *renderer) clear() {
	if r.showing == "" {
		return
	}
	fmt.Fprintf(r.out, "\r%s\r", strings.Repeat(" ", utf8.RuneCountInString(r.showing)+2))
	r.showing = ""
}

// summarise draws the tree, then the count, and names anything that failed.
func (r *renderer) summarise() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// The last check of the run, which nothing came after to close it.
	r.flush()
	r.clear()

	fmt.Fprintf(r.out, "\n%d passed", r.passed)
	if r.skipped > 0 {
		fmt.Fprintf(r.out, ", %s", r.yellow(fmt.Sprintf("%d skipped", r.skipped)))
	}
	if r.failed > 0 {
		fmt.Fprintf(r.out, ", %s", r.red(fmt.Sprintf("%d failed", r.failed)))
	}
	fmt.Fprintln(r.out)

	for _, name := range r.failures {
		fmt.Fprintf(r.out, "  %s %s\n", r.red("✗"), name)
	}
}

// markColumn is where every result is lined up.
//
// Fixed rather than measured. A tree printed as it goes cannot know how wide its
// widest line will be until the run is over, by which time the early lines are
// on the screen and cannot be moved. A line too long for the column pushes its
// own result across and leaves the rest where they are.
const markColumn = 58

// The two connectors, named so the difference reads at a glance.
const (
	branch = "├── "
	last   = "└── "
)

// show prints one check, with the connector it turned out to need.
func (r *renderer) show(c *check, connector string) {
	r.clear()

	label := c.label
	if pad := markColumn - utf8.RuneCountInString(c.gutter+connector+label); pad > 0 {
		label += strings.Repeat(" ", pad)
	} else {
		label += " "
	}
	fmt.Fprintf(r.out, "%s%s%s%s\n", c.gutter, connector, label, c.mark)

	// What its own lines hang from. A check that closed its branch has nothing
	// below it, so the gutter under it is blank rather than a line.
	under := c.gutter + "│   "
	if connector == last {
		under = c.gutter + "    "
	}

	for i, d := range c.detail {
		leaf := branch
		if i == len(c.detail)-1 {
			leaf = last
		}
		fmt.Fprintf(r.out, "%s%s%s\n", under, leaf, r.paintDetail(c, d))
	}
}

// hold puts a check in the queue of one, printing whatever was already there.
//
// Anything already held is a sibling of what is arriving, because anything else
// would have flushed it on the way past, so it draws as a branch.
func (r *renderer) hold(c *check) {
	if r.held != nil {
		r.show(r.held, branch)
	}
	r.held = c
}

// flush prints whatever is held, as the last thing under its command.
func (r *renderer) flush() {
	if r.held != nil {
		r.show(r.held, last)
		r.held = nil
	}
}

// place prints the headings a command needs, and returns the gutter its own
// checks hang from.
//
// Headings are printed as they are reached rather than held back, because a
// heading has to appear above the checks underneath it. That costs the last
// command of a run its closing connector: whether it is last is not known when
// its name is printed, so every command draws as a branch. The checks
// underneath are exact, which is where the eye goes.
func (r *renderer) place(path []string) string {
	if !r.printed["radiocli"] {
		r.clear()
		fmt.Fprintln(r.out, "RADIOCLI")
		r.printed["radiocli"] = true
	}

	if !slices.Equal(r.open, path) {
		r.flush()

		// Only the part of the path that is new. Moving from "channels new" to
		// "channels rename" reprints neither RADIOCLI nor CHANNELS.
		common := 0
		for common < len(r.open) && common < len(path) && r.open[common] == path[common] {
			common++
		}

		if common == len(path) {
			// Back to a command already left behind, which happens when its
			// tests are not all in one place: "scanning" has a test in the file
			// that covers "scanning systems", so the run reaches the child and
			// then comes back. Its name goes in again, because checks printed
			// bare underneath the subcommand that closed above them would read
			// as that subcommand's own.
			r.heading(path)
		}
		for i := common; i < len(path); i++ {
			r.heading(path[:i+1])
		}
		r.open = slices.Clone(path)
	}

	// A check is about to go under this command, which is what decides whether
	// its first subcommand needs a blank line above it.
	r.printed[key(path)] = true

	return gutterFor(path)
}

// key names a command in the map of what has printed something. The root has a
// name of its own because its path is empty.
func key(path []string) string {
	if len(path) == 0 {
		return "radiocli"
	}
	return strings.Join(path, " ")
}

// heading prints one command's name, under a blank line where it needs one.
//
// The root is a name on its own at the left margin, with no connector, because
// nothing sits above it to connect to.
func (r *renderer) heading(path []string) {
	r.clear()

	if len(path) == 0 {
		fmt.Fprintln(r.out, "")
		fmt.Fprintln(r.out, "RADIOCLI")
		return
	}

	parent := key(path[:len(path)-1])

	// A blank line above, once the command it sits under has printed something,
	// so one command's checks do not run into the next command's name.
	if r.printed[parent] {
		fmt.Fprintln(r.out, strings.TrimRight(indent(len(path)-1)+"│", " "))
	}

	fmt.Fprintf(r.out, "%s%s%s\n", indent(len(path)-1), branch, strings.ToUpper(path[len(path)-1]))
	r.printed[parent] = true
}

// indent is what a command at the given depth starts with.
//
// Depth 0 is the root, whose children sit two spaces in. Everything below
// carries one gutter per level above it, all of them drawn open, because a tree
// printed as it goes never learns that a command was its parent's last.
func indent(depth int) string {
	return "  " + strings.Repeat("│   ", depth)
}

// gutterFor is what a command's own checks hang from.
//
// A command with subcommands keeps its checks one level in from them, so the
// branch column belongs to commands alone and a check is never mistaken for
// one. Whether any subcommand runs in this particular run is not the question:
// the checklist says whether the command has any, and answering from that keeps
// the shape the same however far -run narrowed things.
func gutterFor(path []string) string {
	g := indent(len(path))
	if hasSubcommands(path) {
		g += "│   "
	}
	return g
}

// hasSubcommands reports whether the tool offers anything under this command.
func hasSubcommands(path []string) bool {
	for _, c := range suite.All {
		if len(c) == len(path)+1 && slices.Equal(c[:len(path)], path) {
			return true
		}
	}
	return false
}

// mark renders how a check ended, in the colour that says it at a glance.
//
// A skip carries no time because nothing ran to be timed.
func (r *renderer) mark(action string, elapsed float64) string {
	switch action {
	case "skip":
		return r.yellow("Skipped")
	case "fail":
		return r.red("✗" + took(elapsed))
	default:
		return r.green("✓" + took(elapsed))
	}
}

// paintDetail colours a line printed under a check to match the check itself,
// so a wall of skips does not read as a wall of failures.
func (r *renderer) paintDetail(c *check, text string) string {
	switch {
	case c.skipped:
		return r.yellow(text)
	case c.failed:
		return r.red(text)
	default:
		return r.dim(text)
	}
}

// took renders how long a check ran for, always in seconds and always printed.
//
// Seconds throughout, rather than whichever unit fits each number best: a
// column holding "800ms" beside "2.4s" cannot be read down at a glance, and
// reading down it is the only thing the column is for.
//
// Nothing is left off for being quick. A check that took no measurable time
// still says 0.0s, because a column with holes in it reads as a column that
// failed to measure something rather than as one full of fast checks.
func took(seconds float64) string {
	return fmt.Sprintf(" %.1fs", seconds)
}

// The marks and the colours they are drawn in. Colour is used only when the
// output is a terminal, so a redirected run stays plain text.
func (r *renderer) green(s string) string  { return r.paint("32", s) }
func (r *renderer) red(s string) string    { return r.paint("31", s) }
func (r *renderer) yellow(s string) string { return r.paint("33", s) }
func (r *renderer) dim(s string) string    { return r.paint("90", s) }

func (r *renderer) paint(code, s string) string {
	if !r.colour {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

// colourful reports whether anything is going to render the escape codes.
func colourful() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
