# testing

An end to end test suite for `radiocli`, run against a real scanner plugged
into this computer.

Nothing here is a unit test. Every test builds the tool, runs it the way a
person would type it, and checks what the scanner did about it. That is the
only thing worth testing in a program whose job is to drive a radio's menus:
the interesting failures are all things the scanner did, not things the code
did.

The suite is its own Go module and imports nothing from `source/`. It tests the
command line surface, so a change to the code that keeps the commands working
keeps these tests passing.

## Running it

```
cd testing
go run .
```

That is the whole suite. It draws the tool's own command tree as it goes:

```
· testing against a SDS150 on /dev/cu.usbmodem00000000000011

RADIOCLI
  │   ├── the help for battery                            ✓ 0.3s
  │   └── refusing an unknown flag                        ✓ 0.0s
  │
  ├── BATTERY
  │   └── printing it as text                             ✓ 0.4s
  │
  ├── VOLUME
  │   │   ├── printing it as text                         ✓ 0.4s
  │   │   └── levels the scanner cannot take              ✓ 0.2s
  │   │
  │   ├── SET
  │   │   ├── level 0                                     ✓ 1.1s
  │   │   └── level 15                                    ✓ 1.1s
```

Every command sits where the tool puts it, and every test hangs off the command
it tests. A command's own tests sit in its gutter, above its subcommands. Tests
belonging to no one command are the root's, and sit at the top under `RADIOCLI`.

A pass is a green `✓` with how long it took, a failure a red `✗` with the same,
and a skip a yellow `Skipped`. Every result carries a time, including `0.0s`,
because a column with holes in it reads as a broken clock rather than as a run
full of quick checks. Why a test skipped, or what the tool said when it failed,
hangs underneath it in the same colour. The lines starting `·` are the harness
reporting what it found and what it put back.

`the command itself` is a test that skipped or failed before any of its checks
ran, so there is nothing more specific to name.

**The tree is drawn as the run goes**, in the order the tests run rather than
alphabetically. That costs commands their closing connector: whether a command
is the last of its parent is not known when its name has to be printed, so
commands always draw as `├──`. The checks under them are exact, because each is
held back until the next line is known, which is one line of lag and nothing a
reader notices.

It also means **a command can be named twice**. Test functions are ordered by
how slow they are rather than by where they sit in the tree, so a run can reach
`channels new`, go elsewhere, and come back to it. When that happens the command
is named again rather than its late checks being printed bare, which would hang
them under whichever subcommand closed above them:

```
  ├── SCANNING
  │   ├── SYSTEMS
  │   │   └── refusing an argument it does not take       ✓ 0.0s
  │
  ├── SCANNING
  │   │   └── printing it as text                         ✓ 0.8s
```

Tests are declared in tree order within a file, so this is rare. It is left
possible rather than forbidden because the two remaining cases are tests that
belong in a `z_` file for the setup they need, and moving them for the sake of
the drawing would cost a run more than it is worth.

A run takes about five minutes, so the line at the bottom names the check
running now, which is the only way to tell a slow test from a stuck one.

Anything passed to `report` goes straight through to `go test`, so `-run` and
the rest work as they always do. It exits with the code `go test` gave. Add
`-logs` to see what the passing tests logged, which is every command the suite
ran and how long it took.

`go test -v -timeout 30m` still works and prints Go's own output, which is what
to reach for when a failure needs the full detail.

### Running less than everything

A plain run does the lot: settings, menus, tuning, location, what is scanned,
renaming entries and renaming them back, and creating entries and deleting them
again. Everything is put back at the end.

For a run that only reads, which is the one to reach for against a radio whose
contents matter and which nobody has tested against before:

```
go run . -writes=false
```

That changes nothing on the scanner: it lists its memory, reads its settings,
and checks what comes back is the shape it is supposed to be. It takes about
fifteen seconds.

A full run takes about five minutes. Most of it is the scanner typing: a rename
is about eight seconds because every character is a key press, and building a
list, a system, a department and a channel to test creation with is most of a
minute each time.

`report` passes `-timeout 30m` for you. Running `go test` directly does not:
its own default is ten minutes, which a full run comes close enough to that
it is not worth risking.

### Other flags

| Flag | Default | What it does |
| ---- | ------- | ------------ |
| `-writes` | on | The tests that change the scanner. `-writes=false` for a run that only reads. |
| `-port` | whichever scanner is attached | The serial port to test against, when more than one is plugged in. |
| `-pace` | whatever the tool chooses | Passed as `--pace` to every command, to run the whole suite slowly or quickly. |

## What it does to the scanner

The suite reads what the scanner is doing before it starts, and puts it back
when it finishes, whether the tests passed or not. It restores the volume,
which favorites lists are being scanned, and the position, and it leaves the
scanner scanning rather than parked in a menu. Each of those is compared before
it is written, so a run that changed nothing writes nothing.

**The position is put back by writing it, not by remembering a zip.** The zip
a position came from cannot be read off the scanner: it resolves one when it is
typed and keeps only the result. The position itself can be read and written,
so that is what the restore does, with `location set --position`. It takes
milliseconds and disturbs nothing else.

**The GPS setting is put back too.** Writing a position does not switch the
GPS off, so the suite reads whether the receiver is in charge before it starts
and sets it back to that at the end. That read is the one place the suite opens
a menu to answer a question that only reads: the setting is shown as the
highlighted entry of the screen that sets it, and there is no way to ask over
the protocol.

The order matters and is deliberate. The GPS goes back first, because it
decides who owns the position: with the receiver in charge there is no point
writing one, since it is overwritten within a minute or two of the next fix.

**The suite works in entries it creates itself.** It builds a favorites list
called `RADIOCLI TEST`, with a system, a department and two channels inside
it, and every test that needs something to look at, walk to, rename, or delete
works on that. Nothing reads a name off your scanner and writes to it.

That is what makes the suite portable. A run on a scanner straight out of the
box tests exactly as much as a run on one full of your own lists, because the
tests supply their own subject either way. Nothing is assumed to be on the
scanner at all.

The shared list is built once, on the first test that asks for it, and removed
at the end. The tests that delete things build their own list instead, since
the point of them is that what they were given stops existing. Deleting a list
takes its systems, departments and channels with it, so one delete cleans up
whatever a test made.

**A run that is killed halfway can still leave one behind.** The restore sweeps
on its way out: it records every favorites list the scanner held before the
tests started, and deletes any list that is there at the end and was not there
at the start. Nothing but the suite creates a list while it runs, so that
catches whatever a test left, whatever it ended up called. Names are not used
for this, because a list is created before it is named and the scanner calls it
something like `FAVORITES 0` in between, so a run that died in that gap leaves a
list whose name says nothing about where it came from.

A crash hard enough to skip the restore leaves a list behind. It will be called
`RADIOCLI TEST` something, or `FAVORITES` something if it died before being
named, and `radiocli favorites delete "<name>" --yes` removes it. The next run
also removes the first kind on its way in.

## Stopping a run part way

**Stopping the run is tidy rather than instant.** No further test starts, the
one already running is allowed to finish, and the scanner is put back the same
way it would be after an ordinary run:

```
· stopping: finishing the test that is running, then putting the scanner back
· this takes a few seconds and is not a hang. Interrupt again to stop at once, and the next run puts the scanner back instead
· putting the scanner back
· scanner state after the run: volume 9, scanning nothing, at 34.090107, -118.406477 within 25 miles, GPS off
```

Everything the run made is deleted and the volume, position and choice of
scanned lists go back to what they were. That holds for Ctrl-C, for a `kill`
sent by an editor or a CI runner, for a closed terminal, and for whatever
started the run being killed out from under it.

**The test in progress is not cut short**, which is why there is a pause. A
command halfway through typing a name into the scanner, killed, leaves it
sitting on an entry screen with half a name in it. Waiting a few seconds is
better than that, and the wait is bounded: no single command runs longer than
four minutes.

**A second interrupt stops at once** and skips the restore, which is the escape
if something really is stuck.

### How the run hears about it

Not by the signal, which cannot be got to the suite reliably. `go test` sits
between `report` and the tests: signal go test and it dies and takes the suite
with it, mid-keystroke, which is the thing being avoided, and there is no
dependable way to signal the suite alone.

So `report` writes a file, named in `RADIOCLI_TEST_STOP`, and the suite watches
for it. `report` also watches its own parent, because `go run` does not pass a
signal on to what it built: killed, it dies on the spot and would otherwise
leave the run driving somebody's radio for another five minutes after they
thought they had cancelled it.

A plain `go test` has no `report` to do any of that, and answers Ctrl-C
directly.

### When it cannot be caught at all

`kill -9`, a flat battery, a pulled cable. Nothing runs, so nothing is put back
at the time.

What makes that recoverable is a note. Before a run changes anything,
it records what the scanner was doing to
`~/.cache/radiocli/unfinished-test-run.json`, or the equivalent on macOS, and
deletes it only once the restore has actually run. A note still there at the
start of the next run means the last one died, and the settings it describes go
back before that run reads its own baseline. Otherwise one killed run poisons
every run after it: the volume it left behind becomes the volume they all
restore to.

Only settings come back that way. Which lists to delete is decided by name,
because the note's idea of which lists existed is too stale to delete against
safely, and a list somebody created since is not the suite's to remove.

## What it will not do

The suite refuses to run `radiocli systems "Full Database"`, whatever a test
asks for. That command returns a short, wrong answer and then leaves the
scanner's serial interface dead until it is power cycled, reproduced twice.
`TestRadiocli_FullDatabaseIsRefused` checks the guard rail itself still works.

The guard looks for the two words anywhere in the arguments rather than at any
particular position, and that is deliberate. It used to check the first
argument, which was the command word only when a test called `execute`
directly; `mustJSON` and `readJSON` both put `-o json` in front first, and on
those paths, which is most of the suite, the command went straight through.

The tool refuses the same command on its own now, so this is the second of two
locks rather than the only one. It stays because it catches the request before
the binary is even started, and because a suite that depends on the thing it is
testing to protect it is not protected.

`menu set` is exercised only by its help and its argument checking. It writes a
value into whichever menu item the scanner happens to be on, and a value
written to the wrong item is a setting changed by accident.

## One thing worth knowing before reading the menu tests

The scanner's menu indexes are a single space shared by every level of its
memory, and they have gaps. On the scanner these were written against, the only
system is index `4`, its departments are `7` and `11`, and its channels are
`9`, `13` and `15`. Nothing publishes a channel's index, so
`TestMenuOpen_ByIndex` finds one by trying.

Handed an index from another level, the scanner does not refuse: `menu open
channel 4` opens the main menu. The test pins the property that survives that,
which is that the tool reports the menu it actually landed on and never claims
to have opened something it did not. A scanner or a tool that starts refusing
it instead passes too.

## The order they run in

Go runs tests in the order their files sort, and that is the only lever there
is, so **the slow tests are in files beginning with `z_` and run last**, and the root
command's own tests are in files beginning with `a_` and run first, so they draw
at the top of the tree where they belong.

Slow means typing. Creating or renaming anything spells the name into the
scanner one character at a time, which is ten seconds or more per entry, and
those tests together are most of a run. Everything quick answers first:

```
TestBacklight ... TestVolumeSet          the quick ones, a minute or two
z_channels_test.go                       creating and renaming, the rest of the run
z_departments_test.go
z_favorites_test.go
z_menu_index_test.go
z_scanning_test.go
z_sites_test.go
z_systems_test.go
```

So a run stopped early has still answered most of the questions, and a failure
in something quick shows up in the first minute rather than the tenth.

Renaming those files back to their obvious names undoes this, which is why each
one says so at the top.

## The layout

The thing you run is at the top, and the tests it runs are the package below it.

```
testing/
  main.go            starting the tests, reading what they report, stopping a run
  render.go          drawing the run as a tree: gutters, connectors, marks, colours
  signal_unix.go     process groups, per platform
  signal_other.go
  render_test.go     the drawing, checked without a scanner
  suite/             the tests themselves
```

Inside `suite/`, one file per command, named after it, plus:

| File | What is in it |
| ---- | ------------- |
| `commands.go` | Every command the tool offers, and reading a test's name as one of them |
| `harness_test.go` | Building the tool, finding the scanner, running commands, the flags |
| `state_test.go` | Reading what the scanner is doing and putting it back |
| `scratch_test.go` | The favorites list, system, department and channels the tests build to work in |
| `recover_test.go` | Putting the scanner back after a run that was killed before it could |
| `a_globals_test.go` | The flags and behaviour every command shares: help, output formats, config file, exit codes |
| `a_lock_test.go` | Several copies of the tool started at once, and whether they stay out of each other's way |
| `a_naming_test.go` | The two rules below, checked against the suite's own source |

`suite.All` lists every command the tool offers. A command added without a
test shows up there as a missing entry.

`TestRadiocli_CommandChecklist` is what keeps that promise. It walks the tool's
own `--help` output from the root down and diffs the result against the list
both ways, so a command the tool offers and the list does not name fails, and so
does a name the tool no longer offers. Before it existed the list was only ever
read in one direction, and it had silently fallen six subtrees behind the
commands that already had test files of their own.

## Naming a test

Two rules, both checked by `a_naming_test.go`, which reads the suite's own source
to do it.

**A test function is named for the command it covers.** That is how the report
knows where to draw it, and there is nothing else to go on:

| Function | Command |
| -------- | ------- |
| `TestChannels` | `radiocli channels` |
| `TestChannelsNew` | `radiocli channels new` |
| `TestBacklightKeysEnable` | `radiocli backlight keys enable` |
| `TestChannelsNew_Talkgroup` | `radiocli channels new`, again |
| `TestRadiocli_Help` | `radiocli` itself |

Where one command needs more than one function, add an underscore and a name
for the variant. Tests that belong to no one command, such as the global flags
or how arguments are counted, belong to the root and start `TestRadiocli_`.

A function named for a command the tool does not offer fails the guard, rather
than quietly drawing itself under the root where nobody would look for it.

**A check's name is at most 50 characters**, written the way somebody who has
never read the test would say it. The report draws checks as leaves of a tree,
so one four commands deep starts a long way in, and a single long name pushes
every mark in the run off the side of the screen.

## Reading a failure

Every test logs the command it ran, its exit code and how long it took:

```
    harness_test.go:238: radiocli scanning systems (0, 11.204s)
```

`report` keeps that out of the way until it is needed: a failing check prints
it underneath, and `-logs` shows it for the passing ones too.

A failure quotes what the tool wrote to stderr, which is where the tool puts
its own explanation. When something cannot be put back, the restore says so on
its way out and names the command to run by hand.
