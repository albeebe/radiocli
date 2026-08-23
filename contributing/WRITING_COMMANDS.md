# Writing a radiocli command

The standard every command in `source/internal/commands/` is held to. It covers
the code, the comments, the unit tests, the end-to-end suite and the
documentation, because a command is not finished until all five exist and match
the rest.

Read this before adding a command, and read it again before calling one done.
Nothing here is a preference. Every rule below describes what the other
commands already do, and a command that breaks one is the odd one out rather
than the new way.

## How to use this file

This file is written to be enforced, by a person or by an agent asked to make
every command follow it. Each rule is stated so that conformance is decidable:
either the file has the header or it does not, either the function has a
`Returns:` section or it does not. Where a rule can be checked by running
something, the command to run is given with it, and all of them are collected
in [Verification](#verification) at the end.

When enforcing this file across the whole tool, work one package at a time and
run `go test ./...` after each. Coverage is 100% of statements everywhere, and
a change that drops it has broken something the tests were holding.

Two rules outrank the rest, in this order:

1. **Do not change behavior while enforcing style.** A command that prints a
   particular sentence prints that sentence afterwards. Moving code out of a
   closure into a named function is a refactor; rewording its output is not.
2. **Accuracy beats uniformity.** If a rule here would make a command wrong or
   its documentation less complete, break the rule and say why in a comment.
   The reasoning in this codebase is written down at the point of decision, and
   a deliberate exception with a stated reason is part of the standard rather
   than a violation of it.

---

## 1. Package layout

One command, one package, named for the command, at
`source/internal/commands/<name>/`.

| File | Required | Holds |
| --- | --- | --- |
| `<name>.go` | Yes | The package doc, `New`, and the command's own logic |
| `types.go` | If the package declares any | Every `type`, `const` and package-level `var` |
| `<name>_test.go` | Yes | Tests for `<name>.go` |
| `README.md` | Yes | The package README, four sections, see [§13](#13-package-readme) |

Split into more files only when one file gets unwieldy, and split by subcommand
rather than by kind: `beep/set.go`, `beep/toggle.go`, `banks/scan.go`. The file
is named for the subcommand it implements, and its test is
`<subcommand>_test.go` beside it. `colors/` and `backup/` are the worked
examples of a package that outgrew one file.

A command package imports no other command package. Anything two commands both
need moves to `source/internal/` (`catalog`, `menus`, `navigate`, `render`,
`textinput`) and is imported from there.

## 2. File header

Every `.go` file in the package, tests included, opens with exactly three
lines, then a blank line:

```go
// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026
```

The date is `M/D/YYYY` with no leading zeros, and it is the date the file was
created. It is not updated when the file changes.

## 3. Package doc

The package doc lives on `<name>.go`, directly above `package <name>`, and
nowhere else. No other file in the package carries one.

It opens with the fixed form, then says what the command does:

```go
// Package scanning implements the "scanning" command, which lists every
// channel the scanner is currently set up to scan.
```

Then, in as many paragraphs as it takes, the part that cannot be worked out
from the code: why the command does it this way, what was tried instead, and
what the scanner does that forced the design. `scanning`, `portlock` and
`backup` are the models. A package doc that only restates the `Short` string is
not finished.

## 4. Declaration order

Within a file:

1. `New`, first, always.
2. The `newX` subcommand constructors, alphabetized.
3. Everything else, alphabetized, with `runX` functions sorted in among the
   rest rather than grouped.

`types.go` holds the declarations in Go's usual order (`const`, then `var`,
then `type`), each group alphabetized, each with its own doc comment.

## 5. The New function

Exactly one exported symbol per command package, with exactly this signature:

```go
func New(app *appcontext.App) *cobra.Command
```

Its doc comment follows the standard form (see [§9](#9-comments)) and opens
`// New returns the <name> command bound to app.`, or, when it has
subcommands, `// New returns the <name> command, with its <sub> subcommand
attached, bound to app.`

The body is in a fixed order:

```go
func New(app *appcontext.App) *cobra.Command {
	var limit int                       // 1. flag variables, if any

	cmd := &cobra.Command{              // 2. the command literal
		Use:   "scanning",
		Short: "Report what the scanner is scanning right now",
		Long:  "Scanning reports what the scanner is working through right now...",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app, limit)
		},
	}

	cmd.AddCommand(newSystems(app))     // 3. subcommands, in ONE call

	cmd.Flags().IntVar(&limit, "limit", defaultLimit,   // 4. flags
		"stop after this many channels, when walking a favorites list")

	return cmd                          // 5. return
}
```

A command with no subcommands and no flags returns the literal directly, with
no intermediate `cmd` variable. `battery`, `status` and `version` are the
models.

## 6. The command literal

Field order is `Use`, `Annotations`, `Short`, `Long`, `Args`, `RunE`. Omit
`Annotations` when the command can move the scanner.

### `Use`

The command word, followed by its positional arguments. Angle brackets for a
value the user supplies, square brackets for an optional one, `...` for
repeating:

```
"battery"                            no arguments
"colors [layout]"                    one optional
"channels <department>"              one required
"delete <department> <name> --yes"   required flag shown in the synopsis
"add <site> <frequency>..."          repeating
"key <name>..."
```

A required flag appears in `Use` so it shows in the synopsis. Optional flags do
not.

### `Short`

One line, capitalized, imperative, no trailing period, under about sixty
characters. It reads as an instruction to the tool: "Report the connected
scanner", "List the systems the scanner is cycling through", "Leave the menus
and return to scanning". Never "Reports" and never a noun phrase.

### `Long`

Required on every command. It opens with the command's own name capitalized,
as a sentence:

```go
Long: "Tune puts the scanner straight onto one frequency and holds it there.\n\n" +
	"Nothing is stored: the scanner's favorites lists, systems and departments\n" +
	"are untouched, and the frequency is forgotten as soon as you move on.",
```

Written as an explicitly wrapped string with `\n` at the end of each line and
`\n\n` between paragraphs, concatenated with `+`. Never a raw backtick string,
because cobra reflows nothing and the wrapping is the layout. Wrap at 80
columns of rendered text.

It says what the command changes and what it leaves alone, and it names any
command that has to be run before or after it. If a command has a trap, the
`Long` is where the reader is warned.

### `Args`

Required on every command. Use cobra's validators (`cobra.NoArgs`,
`cobra.ExactArgs(n)`, `cobra.MaximumNArgs(n)`, `cobra.RangeArgs(m, n)`,
`cobra.MinimumNArgs(n)`, `cobra.ArbitraryArgs`). A custom `func` is allowed
only when the count alone cannot express the rule, and `ValidArgs` is set as
well when the argument is drawn from a fixed set.

`cobra.NoArgs` is not optional on a command that takes none. Leaving `Args`
unset makes a typo a silently ignored argument.

### `Annotations`

A command that cannot move the scanner is marked, so it may run alongside
another command through the daemon. The comment above it is part of the rule
and is written the same way everywhere:

```go
// Only looks at what the scanner is doing, so it may run while another
// command has the radio. See appcontext.OnlyReads.
Annotations: map[string]string{appcontext.OnlyReads: "true"},
```

A command that only reads under certain flags names those flags instead, space
separated:

```go
Annotations: map[string]string{appcontext.OnlyReadsWith: "cache positions"},
```

Anything unmarked is refused alongside another command. That default is
deliberate: a new command has to be looked at before it can run concurrently,
so leaving the annotation off is the safe mistake.

### `RunE`

Always `RunE`, never `Run`. A command that cannot currently fail still returns
an error, because one day it will.

The body validates and delegates. It may do these things and nothing else:

- Pull positional arguments out of `args`.
- Reject impossible flag combinations.
- Parse an argument into the type the worker takes.
- Call one `runX` function, returning its error.

It does not open the scanner, drive menus, format output, or hold the body of
the command. `app.Device(...)` never appears inside a `RunE` closure. If the
body is more than about ten lines after the flag checks, it belongs in a
function.

## 7. Subcommands

A subcommand is built by an unexported constructor in the same package:

```go
func newSet(app *appcontext.App) *cobra.Command
```

Named `new` plus the subcommand in Pascal case: `newSet`, `newDelete`,
`newGoto`, `newFrequenciesAdd`. Nested subcommands carry the whole path
(`newMacroShow` for `config macro show`). The doc comment is
`// newSet returns the "<parent> set" subcommand.`

All subcommands are attached in a **single** `AddCommand` call, in the order
they should be read rather than alphabetically:

```go
cmd.AddCommand(newGoto(app), newRename(app), newNew(app), newScan(app), newDelete(app))
```

Past four or five, break the arguments across lines inside the one call, as
`config/macro.go` does. Never one `AddCommand` call per subcommand.

## 8. Flags, arguments and workers

### Flags

Registered on the command that uses them, after `AddCommand` and before
`return cmd`, bound to a variable declared at the top of the constructor:

```go
cmd.Flags().BoolVar(&yes, "yes", false, "go ahead and delete it")
```

Help text is lowercase, no trailing period, and describes the effect rather
than the flag: `"go ahead and delete it"`, not `"confirmation flag"`.

`PersistentFlags` belongs to `root` alone. A command package never registers
one: a flag that should work everywhere is a global flag, and it goes in
`root/root.go` and in [global_flags.md](../documentation/global_flags.md).

A destructive command takes `--yes` and refuses without it.

Use `cmd.Flags().Changed("name")` to tell "not passed" from "passed the default
value" when the difference matters, and pass the result down rather than
passing `cmd` into the worker.

### Worker functions

Named `run` when the package's main command has one worker, and `runX` matching
the subcommand otherwise: `runSet`, `runDelete`, `runMacroShow`. A package with
subcommands has `runGet`/`runSet` rather than `run`/`runSet`.

The signature takes what it needs and nothing more. `ctx` first when the
function touches the scanner, `app` second, then the values:

```go
func runSet(ctx context.Context, app *appcontext.App, zip string, miles int) error
```

Never take `*cobra.Command`. That is the seam between the command line and the
work, and crossing it is what makes a worker untestable without building a
whole command tree.

## 9. Comments

Every declaration carries a doc comment. Exported or not, function or constant
or struct field, no exceptions. There are 498 functions in the command packages
and every one is documented.

A function's comment opens with its own name and says what it does, then, when
it has any, carries these sections in this order:

```go
// runSet points the scanner at a zip code and reports what it landed on.
//
// The zip goes in through the menus rather than the protocol, because the
// protocol's own command is accepted and then ignored on this firmware.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - app: the application context the command reads the scanner and the
//     output settings from
//   - zip: the five digit zip code to set
//
// Returns:
//   - error if the scanner cannot be reached, the zip is refused, or the
//     screen cannot be read back
//
// Errors:
//   - ErrNoDevice: when no scanner was named and none could be found
func runSet(ctx context.Context, app *appcontext.App, zip string) error {
```

Rules:

- `Parameters:` is present whenever the function takes one, and lists every
  parameter, in order, as `//   - name: description`.
- `Returns:` is present whenever the function returns anything, one bullet per
  return value, naming the type for anything other than `error`.
- `Errors:` is added only when the function returns a **sentinel** error the
  caller is expected to match with `errors.Is`. Most functions have none.
- The prose between the summary and the sections is where the reasoning goes:
  why this approach, what was tried, what the hardware does. A comment that
  restates the code is worse than no comment, because it has to be maintained
  and says nothing.
- Wrap at 80 columns.

Inline comments explain why, never what. The bar is whether the next reader
would otherwise wonder about it, and whether a future change might undo the
reasoning without noticing.

## 10. Output

All output goes through `internal/render` and `appcontext`. A command never
writes to `app.Stdout` or `app.Stderr` directly.

| Use | For |
| --- | --- |
| `render.JSON(app.Stdout, v)` | The answer, under `--output json` |
| `render.Changed(app, m, text)` | One change to the scanner's memory, both modes at once |
| `render.Dash(s)` | A value that may be empty, rendered as `-` in a table |
| `render.YesNo(b)` | A boolean in text output |
| `app.Printf(...)` | The answer, in text mode, to stdout |
| `app.Notef(...)` | Anything that is not the answer, to stderr |

The separation is a contract the tool documents: results on stdout, everything
else on stderr, so `2>/dev/null` leaves stdout holding only the answer. A
progress line, a warning, a prompt or a "nothing found" message is a note and
goes to `Notef`.

Every listing command supports `--output json`. The JSON shape is a type in
`types.go` with explicit `json:` tags on every field, and it is documented field
by field in the command's `.md` file. A command that creates, renames or deletes
reports it with `render.Changed` and a `render.Mutation`, so a script editing
the scanner gets an object rather than a sentence.

Text output that is a table uses `text/tabwriter`. Columns are lowercase
headings.

### Message style

**Errors** start lowercase, have no trailing period, and put the fix after a
colon:

```go
fmt.Errorf("%q is not a zip code: want five digits, such as 12345", arg)
fmt.Errorf("no macro called %q: the macros are %s", name, macroNames(cfg))
fmt.Errorf("%q is already first: there is nothing above it to move past", name)
```

Validation errors say what was wanted with `want`. Wrapped errors name the
operation as a gerund and wrap with `%w`:

```go
fmt.Errorf("reading the screen after entering the zip code: %w", err)
```

An error a caller needs to match is a sentinel in `types.go`, wrapped with `%w`
and listed in the function's `Errors:` section.

**Notes** are full sentences: capitalized, ending in a period, with the newline
explicit in the format string.

```go
app.Notef("The scanner has left the menus.\n")
app.Notef("That list is already called %q.\n", name)
```

The scanner is "the scanner", not "the radio" or "the device".

## 11. types.go

Every `type`, `const` and package-level `var` the package declares lives in
`types.go`, with the file header and no package doc.

That includes the JSON output struct (conventionally `report`), sentinel
errors, timing constants, menu entry names and default flag values. A magic
number in the middle of a function is a defect: name it, put it in `types.go`,
and say in its comment where the number came from and what happens if the
scanner disagrees.

```go
// maxAreas is where the walk gives up, well above the fifty of the largest
// layout, so a scanner that never comes back round stops rather than spins.
const maxAreas = 200
```

A package that declares nothing has no `types.go`. Do not create an empty one.

## 12. Tests

**100% of statements. Not a target, the standard.** Every package in the tool
is at 100% and a change that lowers it is not finished.

The test file carries the same three-line header and lives beside the file it
tests. Fakes come first, then the tests in the order the functions appear.

Every test function carries a doc comment in this form:

```go
// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering both of the function's paths)
//
// Test cases:
//   - Wiring: the command is named and marked as a command that only reads
//   - HasSet: the set subcommand is attached
//   - Runs: executing the command reports the level the scanner holds
func TestNew(t *testing.T) {
```

The listed cases are the `t.Run` names, in order. Adding or removing a subtest
means updating the count and the list in the same edit.

Conventions:

- The scanner is faked with a `fakeConn` implementing `device.Conn`, answering
  each command from a function the test supplies. It is declared in the test
  file, documented, and not shared between packages.
- A `quiet()` or `newApp()` helper builds an `*appcontext.App` writing to
  `bytes.Buffer` rather than the terminal, and returns the buffers.
- A `failWriter` covers the stream-write error paths.
- Timing constants that would make a test slow are package-level `var` so a
  `fast(t)` helper can shorten them, restoring them with `t.Cleanup`.
- Tests never touch a real scanner, a real serial port, or the user's config
  file. Anything on disk goes in `t.TempDir()`.
- Subtest names are Pascal case and describe the case, not the assertion:
  `NoDevice`, `WrongGreeting`, `TooLong`.

These are the unit tests, and they never touch a radio. Behavior that only a
real scanner can prove goes in the end-to-end suite, which has rules and a
registry of its own: see [§16](#16-the-end-to-end-suite). A command with 100%
unit coverage and no entry in that suite is half wired up.

## 13. Package README

`source/internal/commands/<name>/README.md`, four sections, these headings
exactly, in this order:

```markdown
# <name>

## What this does?
One paragraph. What the package is, in terms of the command it implements.

## Why we use it?
Two or three paragraphs. Why the command exists, why it works the way it does,
and what the scanner forced. This is the section worth writing.

## How we use it?
Runnable examples in a bash block, with a comment above each line saying what
it answers.

### Testing
How to run this package's tests.

## Further reading
- **Topic** - What it is and why it matters here
```

This is the same format every package in the tool uses. It is written for
somebody reading the code, and it is allowed to name Go types and functions,
which the command documentation is not.

## 14. Command documentation

Every command gets `documentation/commands/<name>.md`, written in the same
change as the code.

The full standard is
[WRITING_COMMAND_DOCS.md](WRITING_COMMAND_DOCS.md), which
outranks this section on anything it covers, and which is where the rules on
what to write live. Read it before writing the file. What follows is the
structure an enforcement pass can check, plus the conventions the thirty
existing files share that the standard does not spell out.

### The skeleton

````markdown
# tune

Puts the scanner straight onto one frequency and holds it there. Run it to
listen to something immediately, without adding it to the scanner's memory.

## Overview

Four to eight sentences. What it does, what it changes on the scanner and what
it leaves alone, and any concept the reader needs. End with what it requires:
"It needs a scanner, so name one with `--device`."

## Usage

```
radiocli tune <frequency> [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<frequency>` | Yes | none | The frequency to tune to, in megahertz unless a unit is given. |

### `<frequency>`

A `###` subsection for every parameter needing more than one line.

### Global flags that change this command

The global flags that alter this command, one or two lines each, linking to
[global flags](../global_flags.md) for the rest.

## Examples

Three or more, most common first, each a fenced block holding the exact command
and its real output, then one sentence of explanation.

## Output

Both formats. The text lines or table, then the JSON shape with every field
named and typed. State that results go to stdout and everything else to stderr.

## Errors

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: ...` | What went wrong. | What to do. |
````

### Structural rules

- **The filename is the command**, lowercase, no prefix. A subcommand deep
  enough to need its own file uses an underscore: `config_macro.md`.
- **The H1 is the bare command name**, and there is exactly one H1 in the file.
  A subcommand documented in the same file is an H2, written as code:
  `## \`favorites goto\``.
- **Six H2 sections, in this order**: `Overview`, `Usage`, `Parameters`,
  `Examples`, `Output`, `Errors`. Every one is present, even when the answer is
  "this command has no flags of its own", which is written out as a sentence
  plus the global-flags table rather than omitted.
- **`Errors` is the last section.** Anything extra goes before it.
- **Extra sections are encouraged and go between `Output` and `Errors`.** They
  carry the reasoning: why the command works this way, what it cannot do, what
  the scanner does that is surprising. Give them a title that says what they
  answer, not a generic one.
- **Every flag appears in the `Parameters` table**, with its exact default
  copied from the code, under the `Parameters` heading rather than floating in
  another section.
- **Examples are real.** Run the command, paste the output. Nothing in these
  files is written from memory, because agents read them and then execute what
  they say.
- **Every JSON field is documented** by name, type and meaning.
- **Cross-references are relative links** to the other command's file, placed
  in the prose where the reader depends on them. There is **no `See also`
  section**; `WRITING_COMMAND_DOCS.md` bans it explicitly.

### Table headers, fixed

Two tables recur and their headers do not vary:

```
| Parameter | Required | Default | Description |
| Message   | Meaning  | Fix     |
```

`Message`, not `Error`. Quote the message exactly as the tool prints it,
including the `error: ` prefix. Give the exit code only when it is not 1.

### The three sections that are never omitted

`Parameters`, `Output` and `Errors` are present even when the answer is
"nothing". They are answered in prose rather than left out:

- No flags of its own: say so, then give the global-flags table.
  `battery.md` is the model.
- No failures of its own: say so and link to
  [global flags](../documentation/global_flags.md). `version.md` and `devices.md` are the
  models, and both are correct in omitting the table while keeping the heading.

Omitting the heading is the defect, because a reader who finds no `Parameters`
section cannot tell a command with no flags from a file nobody finished.

### Global flags

Do not document the global flags in a command's file. Name only the ones that
change this command's behavior, one or two lines each, under an H3 headed
exactly:

```
### Global flags that change this command
```

Plural form `### Global flags that change these commands` when the file covers
a command and its subcommands together. Twenty of the thirty files carry this
heading and the rest fold it into prose; the heading is the standard.

### Where it goes and what to update

The file is written in the same change as the code, and it is the only place
the command's reference lives. Nothing generates it and, until the check in
[Verification](#verification) is run, nothing notices its absence.
`documentation/commands/README.md` needs no edit: it describes the folder's
convention rather than listing its files.

The command documentation names no Go package, function or file. It is written
for a reader holding a terminal who will never open the repository.

## 15. Wiring it up

A new command reaches the tool in exactly one place, `source/main.go`: its
package is imported and its `New` is added to the `commands` slice, in
alphabetical order. Nothing else in `source/` changes. If a new command needs
anything else there touched, the design is wrong.

Also update, in the same change:

- `README.md`, in the "What it can do" list, if the command is worth naming
  there.
- `documentation/examples.md`, if the command belongs in the tour.

`documentation/commands/README.md` is not an index and needs no edit: it
describes the folder's convention rather than listing its files.

## 16. The end-to-end suite

The command is not wired up until it is in `testing/` as well. That module is
separate from the tool, imports nothing from it, and drives the built binary
the way a person would, so it is the only place that proves the command
actually works against a radio.

### The checklist, which is enforced

`testing/suite/commands.go` holds `var All [][]string`, every command the
tool offers, as it would be typed. Add an entry for the command **and one for
every subcommand**, each as its own path, keeping the list alphabetical:

```go
{"backlight"},
{"backlight", "keys"},
{"backlight", "keys", "disable"},
{"backlight", "keys", "enable"},
{"backlight", "keys", "toggle"},
{"backlight", "off"},
{"backlight", "on"},
```

`TestRadiocli_CommandChecklist` walks the tool's own `--help` from the root
down and diffs it against this list **both ways**, so a command the tool offers
and the list does not name fails, and so does a name the tool no longer offers.
It needs no scanner. Run it after adding a command:

```bash
cd testing && go test -run TestRadiocli_CommandChecklist ./...
```

This list drifted once, silently, and fell six whole subtrees behind commands
that already had test files. Keeping it current is not bookkeeping: the
help, usage and argument-count tests all run off it, so a missing entry is a
command nothing checks.

### The test file

One file per command, named after it: `testing/<name>_test.go`. It carries the
same three-line header, and it uses only the harness helpers:

- `needScanner(t)` first in **every** test that touches the radio. It skips when
  nothing is attached and when the run was interrupted, which is where an
  interrupted run actually stops.
- `needWrites(t)` instead, in every test that **changes** the radio. It calls
  `needScanner` and then skips if the run passed `-writes=false`. A test that
  changes a scanner somebody owns without being asked is a defect.
- `run(t, ...)` and `mustRun(t, ...)` to invoke the built binary. Never
  `exec.Command` directly, and never an import from `source/`.

Anything the test creates on the scanner is named with the suite's scratch
prefix and deleted by the test that made it. See `scratch_test.go`.

### Slow tests run last

A test that types a name into the scanner spells it one character at a time,
which is ten seconds or more per entry. Those files are named `z_<name>_test.go`
so Go's file-order sorting runs them at the end, and each one says at the top
that its name is load-bearing.

The rule: **if the test creates or renames anything, the file gets the `z_`
prefix.** Everything else answers in the first minute or two, so a run stopped
early has still answered most of the questions.

### Restoring what the command changed

A run puts the scanner back the way it found it, whether it passed or failed.
If the command changes something the suite does not already save, extend
`state_test.go`: add the field to `state`, read it in `readState`, and put it
back in `restore`. A command that moves the radio somewhere `restore` does not
know about leaves the owner's scanner changed after a test run.

### Do not touch the runner

`testing/main.go` needs no change when a command is added. It shells out
to `go test -json` and renders whatever comes back, and it knows nothing about
any individual command. Adding a command there is wasted work at best.

```bash
cd testing && go run . -writes=false     # read-only, safe on an attached radio
cd testing && go run .                   # the whole suite, including writes
```

---

## Verification

Run from `source/`. All of these must be clean.

```bash
gofmt -l .                 # formatting; must print nothing
go vet ./...               # must print nothing
go build ./...             # must succeed
go test ./...              # every package must pass
go mod tidy -diff          # dependencies must be current; must print nothing

# Coverage must be 100.0% for every package
go test -coverprofile=cov.out ./... && go tool cover -func=cov.out | tail -1

# The concurrent packages must be race clean
go test -race ./internal/broker/... ./internal/portlock/... ./internal/device/...
```

The end-to-end suite is a separate module. Run from `testing/`:

```bash
gofmt -l .
go vet ./...

# The command checklist must match the tool, both ways. Needs no scanner.
go test -run TestRadiocli_CommandChecklist ./...

# Everything that needs no scanner: help, usage, argument counts, exit codes
go test -run 'TestHelp|TestArgumentCounts|TestRejectsBadGlobalFlags' ./...

# The full suite against an attached radio. Add -writes=false to only read.
go run .
```

Structural checks, run from `source/internal/commands/`:

```bash
# Every package has README.md and exactly one New
for d in */; do
  [ -f "${d}README.md" ] || echo "no README: $d"
  [ "$(grep -l '^func New(' ${d}*.go | grep -vc _test)" = "1" ] || echo "New: $d"
done

# Every non-test file carries the three-line header
for f in */*.go; do
  case $f in *_test.go) continue;; esac
  head -1 "$f" | grep -q '^// Copyright 2026 Alan Beebe' || echo "header: $f"
done

# One AddCommand call per command, not one per subcommand
grep -rn 'AddCommand' . | grep -v _test

# No bare Run:, no PersistentFlags outside root, no direct stream writes
grep -rn --include='*.go' 'Run: func' . | grep -v _test
grep -rn --include='*.go' 'PersistentFlags' . | grep -v _test | grep -v 'root/root.go'
grep -rn --include='*.go' 'Fprint.*app\.Std' . | grep -v _test

# Nothing opens the scanner inside a RunE closure
grep -rn --include='*.go' -A12 'RunE: func' . | grep -v _test | grep 'app.Device('

# Every command literal has Args and Long
python3 -c '
import glob, re
for path in sorted(glob.glob("*/*.go")):
    if path.endswith("_test.go"): continue
    for m in re.finditer(r"&cobra\.Command\{(.*?)\n\t*\}", open(path).read(), re.S):
        body = m.group(1)
        use = re.search(r"Use:\s*\"([^\"]*)\"", body)
        if not use: continue
        for field in ("Long:", "Args:", "Short:"):
            if field not in body:
                print(f"{path}: {use.group(1)!r} has no {field}")
'
```

`root` is exempt from the `Args` rule: it is the top-level command, it takes no
arguments of its own, and cobra dispatches to the subcommands. Everything else
that this prints is a violation.

Documentation checks, run from `documentation/commands/`:

```bash
# The six required sections are present and in order
for f in *.md; do
  [ "$f" = README.md ] && continue
  got=$(grep '^## ' "$f" | grep -E '^## (Overview|Usage|Parameters|Examples|Output|Errors)$' | tr '\n' '|')
  want='## Overview|## Usage|## Parameters|## Examples|## Output|## Errors|'
  [ "$got" = "$want" ] || echo "sections: $f -> $got"
done

# Full section list per file, for reviewing where the extra sections sit
for f in *.md; do
  [ "$f" = README.md ] && continue
  echo "$f: $(grep '^## ' $f | tr '\n' '|')"
done

# Errors must be the last H2 in every file
for f in *.md; do
  [ "$f" = README.md ] && continue
  [ "$(grep '^## ' $f | tail -1)" = "## Errors" ] || echo "Errors not last: $f"
done

# Exactly one H1 per file, ignoring "#" comments inside fenced code blocks
for f in *.md; do
  [ "$f" = README.md ] && continue
  n=$(awk '/^```/{fence=!fence; next} !fence && /^# /{c++} END{print c+0}' "$f")
  [ "$n" = 1 ] || echo "H1 count $n: $f"
done

# The Errors table header is "Message", never "Error"
grep -l '^| Error | Meaning | Fix |' *.md

# Every command the tool ships has a page, and every page has a command.
# Nothing else checks this, so run it whenever a command is added or removed.
(cd ../../source && go build -o /tmp/radiocli .)
offered() {
  /tmp/radiocli --help 2>&1 |
    awk '/^Available Commands:/{f=1;next} /^Flags:/{f=0} f && NF {print $1}' |
    grep -vx 'help\|completion'
}
offered | while read -r c; do
  [ -f "$c.md" ] || echo "no page for command: $c"
done
for f in *.md; do
  [ "$f" = README.md ] && continue
  c=${f%.md}; c=${c%%_*}
  offered | grep -qx "$c" || echo "no command for page: $f"
done
```

## Checklist

A command is done when all of these are true.

- [ ] Package at `commands/<name>/`, with `<name>.go`, `types.go` if needed,
      tests, and `README.md`.
- [ ] Three-line header on every file, `M/D/YYYY`, no leading zeros.
- [ ] Package doc on `<name>.go`, opening `Package <name> implements the
      "<name>" command, which ...`, and carrying the reasoning.
- [ ] `New(app *appcontext.App) *cobra.Command`, the only exported symbol.
- [ ] Declarations ordered: `New`, then `newX` alphabetized, then the rest
      alphabetized.
- [ ] `Use`, `Short`, `Long`, `Args` all set; `Short` imperative with no
      period; `Long` opening with the command name, wrapped with `\n`.
- [ ] `Annotations` set if the command cannot move the scanner, with the
      standard comment.
- [ ] `RunE` validates and delegates; no `app.Device` in the closure.
- [ ] Subcommands attached in one `AddCommand` call.
- [ ] Flags registered after `AddCommand`, help text lowercase with no period;
      `--yes` on anything destructive.
- [ ] Every declaration documented, with `Parameters:` and `Returns:` where
      they apply and `Errors:` only for sentinels.
- [ ] Output through `render` / `Printf` / `Notef`; results on stdout, notes on
      stderr; `--output json` supported and shaped by a type in `types.go`.
- [ ] Errors lowercase with the fix after a colon; notes full sentences.
- [ ] Unit tests at 100% of statements, with the `Coverage:` and `Test cases:`
      comment on every test function.
- [ ] `documentation/commands/<name>.md` written to
      [WRITING_COMMAND_DOCS.md](WRITING_COMMAND_DOCS.md), six
      sections in order, `Errors` last, every flag and JSON field documented,
      every example actually run.
- [ ] That file has exactly one H1, the
      `| Parameter | Required | Default | Description |` and
      `| Message | Meaning | Fix |` headers, no `See also` section, and keeps
      `Parameters`, `Output` and `Errors` even when the answer is "nothing".
- [ ] Added to the `commands` slice in `source/main.go`, alphabetically.
- [ ] The command **and every subcommand** added to `All` in
      `testing/suite/commands.go`, and
      `go test -run TestRadiocli_CommandChecklist ./...` passes from
      `testing/`.
- [ ] `testing/suite/<name>_test.go` written, every test starting with
      `needScanner(t)`, or `needWrites(t)` if it changes the radio.
- [ ] Every test function named for the command it covers, so `TestChannelsNew`
      covers `channels new`, with `_Variant` on the end where one command needs
      several functions. `go test -run TestRadiocli_TestNames ./...` passes.
- [ ] Every `t.Run` name at most 50 characters, and readable by somebody who
      has not read the test.
- [ ] The file named `suite/z_<name>_test.go` if its tests create or rename anything,
      with the note at the top saying the name matters.
- [ ] `state_test.go` extended if the command changes something a run has to
      put back.
- [ ] `testing/main.go` and `testing/render.go` untouched.
- [ ] Every command in [Verification](#verification) is clean.

## Known deviations

Recorded 8/13/2026, from an audit of all thirty command packages. These are the
places the tool does not currently meet this file. An enforcement pass should
close them; a reader should not copy them.

| Where | Deviation |
| --- | --- |
| `colors`, `beep`, `location` | One `AddCommand` call per subcommand instead of one grouped call |
| `status`, `version`, `menu close` | No `Long` |
| `menu` (4 places), `banks/list.go`, `departments`, `systems` | `app.Device` called inside the `RunE` closure |
| `menu` | Worker named `show` rather than `run` |
| `colors` | 39 lines of flag validation inline in `RunE` |
| `scanning.md` | No `## Parameters`, `## Examples` or `## Output`; the flag table floats unheaded inside a subcommand section |
| `audio.md` | Documents `audio feed` as a second H1 instead of an H2, and heads its Errors table `Message` as `Error` |
| `beep.md` | Carries a `## Subcommands` section no other file has |
| `daemon.md` | `Errors` is not last: it is followed by a `## See also`, which `WRITING_COMMAND_DOCS.md` bans by name |
| `location.md` | `Errors` is not last: it is followed by `## A warning about the full database` |
| 10 files | Fold the global flags into prose instead of the `### Global flags that change this command` heading the other 20 use |
