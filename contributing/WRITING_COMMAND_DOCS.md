# Writing a radiocli command's documentation

The standard every file in `documentation/commands/` is held to. It covers what
goes in the file, in what order, in what words, and how much of it has to be
verified before the file counts as written.

Read this before documenting a command, and read it again before calling one
done. Nothing here is a preference. Every rule below describes what the thirty
existing files already do, and a file that breaks one is the odd one out rather
than the new way.

[WRITING_COMMANDS.md](WRITING_COMMANDS.md) is the standard for the command
itself, covering the code, the comments and the tests. This file outranks it on
anything both cover.

## How to use this file

The file is written in the same change as the command, by whoever wrote the
command. Nothing generates it, and nothing but the checks in
[section 7](#7-checklist) notices its absence.

One rule outranks the rest:
[completeness and accuracy](#1-completeness-and-accuracy-outrank-everything-else)
beat every other rule here. Where following some other rule would make a file
less complete or less accurate, break that rule instead and say why.

## 1. Completeness and accuracy outrank everything else

These files are not only read by people. AI agents read them to learn how to
operate the tool, and then run the commands. That changes what a defect costs.

A person who hits a gap notices, runs `--help`, tries something, and recovers.
An agent cannot. It has no way to tell a command that does not exist from one
nobody wrote down, and no way to tell a wrong default from a right one. So:

- An omitted flag is a capability that does not exist. The agent never uses it,
  and works around its absence with something worse.
- A wrong default is a wrong command executed. The agent relies on the stated
  default rather than passing the value, and gets behaviour the file describes
  but the tool does not have.
- An invented example is an instruction to run something that fails, or worse,
  something that succeeds and does the wrong thing.
- A missing precondition is a command run in the wrong order. Agents do not
  infer that one command must follow another. They do what the file says.
- A vague sentence is resolved by guessing, and the guess is confident.

Every line is read as something that will be executed literally, by a reader
with no judgement, no terminal, and no way to ask a question.

### What that requires

**Every fact is verified.** Each statement comes from the source or from a
command that was actually run. Nothing comes from memory, from how similar
tools behave, or from what the design implies. What was not confirmed does not
go in the file.

**Everything is documented, with no exceptions for the obvious.** Every flag,
every accepted value, every positional argument, every output field, every exit
code. A flag that seems self-explanatory is a flag the agent has never seen.
Completeness is measured against the source, not against what seems worth
mentioning.

**Lists are exhaustive.** Banned: "etc.", "and so on", "among others", "such
as" when it introduces a set of legal values, and any trailing "...". A flag
that accepts two values names both. A flag that accepts an open-ended string
says so and describes what makes a value valid.

**Strings are reproduced exactly.** Flag spelling, error text, JSON field
names, enum values and file paths are copied character for character, case and
punctuation included. Agents match on these. `--output` is not `--Output`, and
a field named `checkedAt` is not `checked_at`.

**Preconditions and ordering are explicit.** A command that requires another to
have been run says so as a sentence in the Overview, and again in the parameter
it affects. Nothing relies on the reader connecting two files.

**Every side effect is stated.** What is written, where, when, and whether it
survives the command exiting. Anything that changes a file, a setting or the
device is stated where the reader is standing when it happens.

**One meaning per sentence.** No "usually", "typically", "generally", "should",
"may" or "in most cases" unless the behaviour is genuinely non-deterministic,
and then what it depends on is named. Conditional behaviour is written as its
condition: "If X, then Y. Otherwise Z."

**The machine-readable path gets the same care as the human one.** Agents
prefer `--output json`. Its field names, types, nesting, and which fields can be
absent matter as much as the table a person reads.

**Unknowns are stated.** A behaviour that could not be determined is named as
undetermined in the file, and raised with the user. An explicit gap is safe. A
silent gap is the defect this section exists to prevent.

**Nothing fills space.** A plausible sentence written to make a section look
finished is worse than a section labelled empty, because it reads well and is
wrong.

### Before the file is done

Do a completeness sweep rather than a proofread. List every flag in the source,
then find each one in the file. List every section this standard requires, then
find each one. List every claim about a default, an output field or an error
message, then point at the line of source or the terminal output it came from.
Anything that cannot be pointed at is removed or verified.

When a command changes later, its file changes in the same commit. Stale
documentation is more dangerous than none, because an agent trusts it either
way.

## 2. Read the source first

A command is never documented from its help text alone, and never from memory
of how similar tools behave.

| Path | Holds |
| --- | --- |
| `source/internal/commands/<command>/` | The command itself |
| `source/internal/commands/root/root.go` | The global flags every command has |
| `source/internal/appcontext/config.go` | Settings and their precedence |
| `source/internal/device/` | What talking to the scanner involves |

Then run the command, including its failure modes, and paste what it actually
printed. Invented output is the most common defect in these files, and the
easiest for a reader to catch.

Where the source and the help text disagree, the source wins, and the
disagreement is raised with the user so the help text gets fixed.

## 3. File location and naming

One command per file:

```
documentation/commands/<command_name>.md
```

The name is the command as typed, lowercased, with spaces replaced by
underscores. The command `battery` becomes `battery.md`.

- A subcommand shares its parent's file only when it has no flags of its own.
  Otherwise it gets its own file, named `parent_child.md`.
- An alias never gets a file. It is documented inside the real command's file,
  and nothing links to it.
- A command that does not exist yet gets no file.

## 4. Required structure

Eight sections, in this order, with these exact headings. Nothing is added,
nothing is reordered, and nothing is dropped for being thin. A section that
genuinely does not apply keeps its heading and gets one sentence saying so.

### `# <command>`

Two sentences, plain language, no jargon. The first says what the command does,
the second says when the reader would reach for it. It assumes a reader who has
never used the tool and does not know what the scanner is. It names no flags,
no file paths and no Go types.

### `## Overview`

One paragraph, four to eight sentences. What the command does in more depth,
what problem it solves, and why that is worth having. It says what the command
changes on the reader's machine and what it leaves alone, because a reader
deciding whether it is safe to run gets the answer here. Any concept the command
depends on is explained, such as which scanner is "selected" and where that is
stored. It is written for somebody deciding whether this is the command they
want, not for somebody who has already decided.

### `## Usage`

One fenced block holding the synopsis, in the tool's own conventions:

```
radiocli <command> [flags]
```

Square brackets mean optional, angle brackets mean a value the reader supplies.
Positional arguments are shown, in order.

### `## Parameters`

Every flag the command accepts, starting with a table:

```
| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--flag`  | No       | `value` | What it does. |
```

Then a `###` subsection for every parameter that needs more than one line: what
it controls, what happens when it is omitted, what values are legal, and at
least one complete runnable example. A simple boolean can stay in the table
alone.

- Every flag is covered, the obvious ones included.
- The default is the literal value, not a description of it: `text`, not "text
  format". A flag with no default gets "none".
- "Required" means the command fails without it. A flag with a default is not
  required.
- Short forms appear with the long form: `-o`, `--output`.
- The global flags are not documented here. They belong to every command. Only
  the ones that change this command's behaviour are named, one or two lines
  each, linking to `../global_flags.md` for the rest.
- A flag that is dangerous, irreversible, or writes to disk says so in its own
  subsection, never in a note at the bottom.

### `## Examples`

Three or more, ordered from the most common use to the least. Each is a fenced
block holding the exact command and its real output, followed by one sentence
of explanation. Values are realistic: a real-looking port path, not `<port>`.
Long output is shown whole, because truncating hides the thing the reader is
checking against.

### `## Output`

What the command writes, and where. Both formats are covered: the human table
or lines under `--output text`, and the shape of the document under
`--output json`, with JSON field names and types. It states plainly that results
go to stdout and that progress, prompts and advice go to stderr, so the reader
knows what survives a pipe.

### `## Errors`

A table of the failures a reader will actually hit, what each one means, and how
to fix it. The message is quoted exactly as printed. The exit code is given when
it is not 1. Errors that only occur when the program is broken are left out.

`Errors` is the last section. There is no "See also" list: a command links to
another command only where the reader depends on it, in the prose at that point.
See [section 5](#5-linking-to-other-commands).

## 5. Linking to other commands

This is the rule that makes the folder usable rather than a pile of files.

A file links to another command only where this one relies on it, at the point
where the reader needs it. Links are never gathered into a list at the bottom,
and a command is never linked merely for being related or similar.

That covers four cases:

- The value is something another command prints. Link to that command and name
  the field the value comes from.
- The command only works after another has been run. Say so in the first line
  of the parameter's description, not in a footnote.
- The value is written to the config file by another command. Name the setting
  and the command that writes it.
- Omitting the parameter makes the tool fall back to something another command
  chose. This is the case readers miss most often, so it is spelled out.

Links are relative markdown to the file, never to a heading in it and never to a
bare command name:

```
Correct:   Get this from the `port` column of [`devices`](devices.md).
Correct:   Name a scanner with `--device`; [`devices`](devices.md) lists them.
Wrong:     Get this from the devices command.
Wrong:     See [devices](devices.md#output).
```

Every link is followed before the file is done, to confirm the target exists.
A link to a file nobody has written yet is still written, and the missing files
are named to the user.

## 6. Style

- The reader is "you". The tool is named, never "we".
- Present tense, active voice. "The command saves your choice", never "your
  choice will have been saved".
- Plain words over precise-sounding ones. "Finds", not "enumerates".
- No marketing. Never "simply", "just", "easily", "powerful", "seamless" or
  "blazing". The Overview earns the command's keep by describing what it does,
  not by praising it.
- Say what happens, not what is supposed to happen. A flag with a sharp edge
  gets the edge documented.
- Backticks for anything the reader types or the machine prints: commands,
  flags, paths, values, field names, error text.
- Sentence case for headings, with no trailing punctuation.
- Prose wrapped at a readable width, with a blank line between blocks.
- No emoji, no badges, and no "Last updated" line: git knows.
- The implementation stays out. Package names, function names and Go types
  never appear in these files. The reader has a terminal, not the source.

## 7. Checklist

Every line of this is confirmed before the file is done:

- [ ] The file is `documentation/commands/<command_name>.md`, named for the command.
- [ ] All eight required headings are present, in order, spelled as specified.
- [ ] The opening is exactly two sentences and names no flags.
- [ ] Every flag in the source appears in the parameters table.
- [ ] Every default matches the source, as a literal value.
- [ ] Every example was run, and its output is pasted rather than written.
- [ ] Both output formats are documented, with JSON field names.
- [ ] Every value that comes from another command links to that command's file.
- [ ] Every link resolves to a file in `documentation/commands/`.
- [ ] No package names, function names or file paths from the source.
- [ ] No banned words from [section 6](#6-style).
- [ ] Every claim traces to a line of source or to output seen in a terminal.
- [ ] No abbreviated lists: no "etc.", no "and so on", no trailing "...".
- [ ] Every legal value of every flag is named rather than summarised.
- [ ] Every precondition and ordering requirement is stated in the file itself.
- [ ] Every side effect that outlives the command is stated where it happens.
- [ ] Anything that could not be verified is marked unverified rather than left out.

## 8. Worked example

`battery.md`, abridged in the Examples section only. It is the model for a
command with no flags of its own, and `devices.md` and `version.md` are the
models for one with no failures of its own.

````markdown
# battery

Reports how much charge your scanner has left and whether it is charging. Run
it to check the battery without picking the scanner up.

## Overview

`battery` asks the scanner for its power readings and prints them: the
remaining charge as a percentage, what the charger is doing, the battery
voltage, and the current flowing in or out. The current tells you which way the
charge is moving, so a scanner that is plugged in but not actually charging is
visible here rather than only as a battery that never fills. The command reads
only: it changes nothing on the scanner and writes nothing to the config file.
It needs a scanner, so name one with `--device`.

## Usage

```
radiocli battery [flags]
```

## Parameters

`battery` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | — | — | This command takes no flags and no arguments. |

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column of
  [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the readings are printed as lines or as JSON.

`--pace` has no effect here. This command presses no keys.

## Examples

```
$ radiocli battery --device /dev/cu.usbmodem00000000000011
Charge:       87%
Charger:      charging
Voltage:      4.02 V
Current:      340 mA
```

The usual case, with the scanner on the charger.

## Output

Under `--output text`, the readings go to stdout, one per line, with the labels
padded so the values line up.

Under `--output json`, stdout holds one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `charge` | number | Remaining charge as a percentage. |
| `charger` | string | What the charger is doing. |
| `voltage` | number | Battery voltage in volts. |
| `current` | number | Current in milliamps. Negative when discharging. |

## Errors

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `no scanner named` | No port was passed with `--device`. | Pass `--device` with a port from [`devices`](devices.md). |
| `reading the battery: <detail>` | The scanner answered, but not with a reading it could parse. | Run with `--verbose` to see the raw exchange. |
````
