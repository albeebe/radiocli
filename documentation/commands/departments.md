# departments

Lists the departments inside one system, and says which the scanner is
skipping. A department is a group of channels, such as the police channels of
one town.

## Overview

The scanner's memory nests: a favorites list holds systems, a system holds
departments, and a department holds channels. `departments` reports the third
level for one system at a time, giving each department's name, whether the
scanner is scanning it, and the quick key assigned to it.

This is as deep as the tool goes. The channels inside a department cannot be
read at all, for the reason under [Why channels are not
listed](#why-channels-are-not-listed).

The system can be named by its index, which [`systems`](systems.md) reports
under `-o json`, or by its name, which is easier to type but costs several
extra exchanges with the scanner. The command reads only: it changes nothing
on the scanner and writes nothing to the config file. It needs a scanner, so
name one with `--device`.

## Usage

```
radiocli departments <system> [flags]
radiocli departments goto <department> [flags]
radiocli departments rename <department> <name> [flags]
radiocli departments new <system> <name> [flags]
radiocli departments delete <department> --yes [flags]
```

## Parameters

`departments` takes one argument and no flags of its own.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<system>` | Yes | none | The system to look inside, by index or by name. |

### `<system>`

Which system to report the departments of. There is no default: a scanner can
hold many systems and no one of them is the obvious one to mean.

An argument made only of digits is taken as an index and used as it stands,
which keeps the command to a single exchange with the scanner:

```
$ radiocli departments 4
NAME    SCANNED  QUICK KEY
POLICE  yes      -
FIRE    yes      -
```

Anything else is taken as a name, matched without regard to case:

```
$ radiocli departments "GREENDALE ST"
```

A name costs more here than it does in [`systems`](systems.md). A system's
index carries no hint of which favorites list holds it, so resolving a name
means reading the favorites lists and then the systems of every one of them.
That is one exchange per favorites list on top of the first. Use an index when
running this repeatedly or from a script.

A name no system carries is refused, and the message lists the systems the
scanner does hold:

```
$ radiocli departments "POLICE"
error: no system is called "POLICE": the scanner has "GREENDALE ST"
```

Two favorites lists can each hold a system of the same name. When one is
ambiguous the command refuses rather than guessing, and says to use an index.

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column
  of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the departments are printed as a table or as
  JSON.

`--pace` has no effect on the bare command or on `goto`, neither of which
presses a key. It matters to `rename`, which presses one for every character
typed.

## Examples

Listing the departments in a system by index:

```
$ radiocli departments 4
NAME    SCANNED  QUICK KEY
POLICE  yes      -
FIRE    yes      -
```

The same by name, which is easier to type and slower to run:

```
$ radiocli departments "GREENDALE ST"
NAME    SCANNED  QUICK KEY
POLICE  yes      -
FIRE    yes      -
```

As JSON, which carries each department's index:

```
$ radiocli departments 4 -o json
[
  {
    "name": "POLICE",
    "index": "7",
    "avoided": false
  },
  {
    "name": "FIRE",
    "index": "11",
    "avoided": false
  }
]
```

Walking down from the top, which is how the indexes are found:

```
radiocli favorites                 # PUBLIC SAFETY, index 2
radiocli systems 2 -o json         # GREENDALE ST, index 4
radiocli departments 4             # POLICE and FIRE
```

## `departments goto`

`goto` puts the scanner into the menu for one department, where its name, its
quick key, and its channels can be edited:

```
$ radiocli departments goto POLICE
menu: POLICE

   INDEX  ENTRY
>  0      Edit Name
   1      Set Department Quick Key
   2      Edit Channel
   3      Set Location Information
   4      Set Avoid
   5      Delete Department
   6      New Department
```

The department may be named by index or by name. Note that the argument is a
*department*, where the bare command takes a *system*.

Naming a department by name is the most expensive lookup in this tool. A
department's index says nothing about which system or favorites list holds it,
so every favorites list, and every system in each of them, has to be read
first. Use an index when running this repeatedly.

**This stops the scanner scanning.** Run `radiocli menu close` when finished,
or `radiocli key avoid` from a screen that refuses it.

## `departments rename`

`rename` changes a department's name:

```
$ radiocli departments rename FIRE "FIRE RESCUE"
FIRE RESCUE
```

It opens the department's menu, selects `Edit Name`, and types the new name by
turning the knob to each character in turn, checking every one after it is set.
This is the same machinery as [`favorites rename`](favorites.md), and the notes
there about routing, cost, and what happens on failure apply here too.

The department may be named by index or by name, and a name costs the same
search described above. A name no department carries is refused before the
scanner leaves scanning:

```
$ radiocli departments rename NOPE "X"
error: no department is called "NOPE": the scanner has "POLICE", "FIRE"
```

**Nothing is saved until the whole name is typed**, so an interrupted rename
leaves the old name untouched.

**This stops the scanner scanning**, and returns it to scanning when it is done.

## `departments new`

`new` creates a department inside a system:

```
$ radiocli departments new "GREEN BANK" POLICE
POLICE
```

Pressing the scanner's own `New Department` creates one immediately, named
something like `DEPARTMENT 0`, and the name you asked for is typed over it. The
department is read back afterwards to confirm it is there.

A new department holds no channels. Add one with [`channels new`](channels.md).

**This stops the scanner scanning**, and returns it when finished.

## Output

The table goes to stdout. The empty-system message goes to stderr, as do debug
logs from `--verbose`. Redirecting stderr leaves stdout holding the table
alone.

Under `--output text`, stdout holds a header row and one row per department,
with the columns padded so they line up:

```
NAME    SCANNED  QUICK KEY
POLICE  yes      -
FIRE    yes      -
```

| Column | Description |
| ------ | ----------- |
| `NAME` | The department's name, as the scanner holds it. |
| `SCANNED` | `yes` when the scanner is scanning the department, `no` when it is being skipped. |
| `QUICK KEY` | The quick key that switches this department, or `-` when none is assigned. |

There is no number tag column. Departments carry no number tag, unlike the
favorites lists and systems above them.

`SCANNED` is the opposite of what the scanner reports. The protocol says
whether a department is *avoided*, and a table of double negatives is hard to
read, so this reports whether it will be heard.

When the system holds no departments, stdout is empty and stderr says so. The
command exits `0`, because an empty system is a complete answer to what is
inside it.

Under `--output json`, stdout holds an array of objects, one per department, in
the order the scanner reports them:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The department's name. |
| `index` | string | How the scanner names this department. |
| `avoided` | boolean | Whether the scanner is skipping this department. Note this is the scanner's own sense, the opposite of the `SCANNED` column. |
| `quickKey` | string | The quick key assigned, absent when none is. |

## Why this command stops at departments

A department holds channels, which are the frequencies and talk group IDs the
scanner actually receives. They are not listed here, because the protocol
request that would list them does not work.

On SDS150 firmware 1.00.37 the request for the channels in a department answers
with the favorites list document instead, whatever index it is given, including
indexes that do not exist, and reports no error while doing so. The request for
a trunked site's frequencies fails the same way, answering with the tone-out
document. There is no argument spelling that works: passing the full path from
the favorites list down is rejected outright.

Rather than show whatever came back, the shared reader behind these commands
refuses a document whose elements are not the ones asked for. That is why this
command stops here rather than reporting something that looks like an answer.

**The channels are not unreachable, only unlistable this way.** They are on the
scanner's own menus, along with their frequencies and every other setting, and
[`channels`](channels.md) reads them by walking those menus and reading the
screen. That is slower and more fragile than a protocol request would be, which
is why it is a separate command rather than part of this one.

## Telling an empty system from one that is not there

The scanner answers a request naming an index that does not exist exactly as it
answers a request for a system holding nothing: with an empty document and no
error. The two cannot be told apart from that answer alone.

When there is nothing to report, this command reads the favorites lists and
their systems to settle which happened, so a mistyped index is called out
rather than being reported as an empty system:

```
$ radiocli departments 999
error: no system has index 999: the scanner has "GREENDALE ST"
```

Those extra exchanges happen only when the answer is empty. A system with
departments in it costs one exchange when named by index.

## `departments delete`

`delete` removes a department, and every channel in it:

```
$ radiocli departments delete "TEST DEPT" --yes
deleted TEST DEPT
```

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--yes` | Yes | `false` | Go ahead and delete it. |

**`--yes` is required.** Without it nothing is touched and the command says
what would have gone:

```
$ radiocli departments delete "TEST DEPT"
error: deleting the department "TEST DEPT" removes it and every channel in it, and cannot be undone: pass --yes
```

**There is no undo.** Nothing on the scanner keeps a copy and this tool cannot
put back what it removes. The name is resolved before anything is pressed, so a
name the scanner does not have costs nothing, and the departments are read back
afterwards to confirm it is gone.

The department may be named by index or by name, the same as the argument to
`goto` and `rename`.

**This stops the scanner scanning**, and returns it when finished.

## JSON output from `new`, `rename` and `delete`

The three verbs above print a line of text, and under `--output json` they print
an object instead. One shape covers all of them, and the same shape covers the
create, rename and delete verbs on every other level of the scanner's memory, so
a script driving edits does not have to learn a different object for each:

```json
{
  "action": "renamed",
  "kind": "department",
  "name": "NEW NAME",
  "was": "OLD NAME"
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `action` | string | What happened: `created`, `renamed` or `deleted`. |
| `kind` | string | What it happened to, which is always `department` here. |
| `name` | string | What the entry is called now, or was called when it was deleted. It is the scanner's own spelling, read back after the change. |
| `was` | string | Absent. This command never learns the previous name, and reading it back would cost an exchange the command does not otherwise need. |
| `in` | string | the system it is in, as you named it. |

Fields that do not apply are left out rather than written empty, so a consumer
can tell a create from a rename without comparing strings.

The text output is unchanged. It is what people already read, so the machine-
readable mode was added beside it rather than in place of it.

## Long lists

The scanner caps a list reply at roughly a kilobyte and offers no way to ask for
the rest. On a system with a lot of departments, the reply stops early and is marked `EOT="0"` to say
there is more; repeating the request answers with the same first part.

`radiocli` used to hand that straight back, which reported a short list as though
it were the whole thing. It no longer does. When the scanner admits it cut the
list short, the missing names are read the slow way, off the scanner's own menus,
which is the one reading that misses nothing. That costs several seconds and
stops the scan, and it happens only when the scanner has said the list was short.

Only the **names** come off the screen. Everything else about a department lives
in the list the scanner would not finish sending, so those columns are shown as
`?`, which means "not read" and is a different thing from the `-` that means "the
scanner says there is nothing here". A note on stderr says how many departments that
covers. Under `--output json` those entries carry `"partial": true` instead.

```
NAME          SCANNED  QUICK KEY
GREENDALE     yes      3
MILLBROOK     ?        ?
```

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: accepts 1 arg(s), received 0` | No system was named. | Give an index or a name, from [`systems`](systems.md). |
| `error: no system is called "<name>": the scanner has ...` | No system carries that name. | Use one of the names in the message, or an index. |
| `error: no system has index <n>: the scanner has ...` | No system carries that index. | Run [`systems`](systems.md) with `-o json` to see the indexes. |
| `error: <n> systems are called "<name>": name one by its index instead` | Two or more systems share the name given. | Use the index, from [`systems`](systems.md) with `-o json`. |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: reading the favorites lists: <detail>` | The system had to be looked up by name or checked for existence, and that read failed. | Run with `--verbose` to see the raw exchange. |
| `error: reading the systems: <detail>` | The same lookup failed while reading a favorites list's systems. | Run with `--verbose` to see the raw exchange. |
| `error: reading the departments: <detail>` | The scanner did not answer the request for the departments. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |
| `error: reading the departments: the scanner answered with a FL list instead of a DEPT list, which is how this firmware refuses a list it cannot produce` | The scanner returned the wrong kind of document. | Report it: this is the failure mode that stops channels being listed, and it is not expected here. |

An empty system is not an error. It prints nothing to stdout, explains itself
on stderr, and exits `0`.
