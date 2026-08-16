# systems

Lists the systems inside one favorites list, and says which the scanner is
skipping. Run it to see what a favorites list actually contains.

## Overview

The scanner's memory nests: a favorites list holds systems, a system holds
departments, and a department holds channels. `favorites` reports the top
level; `systems` reports the level below it for one list at a time. Each system
is reported with its name, its type, whether the scanner is scanning it, and
the index that names it in turn. Run [`departments`](departments.md) with that
index to go one level further down.

The list to look inside can be named by its index, which
[`favorites`](favorites.md) reports under `-o json`, or by its name, which is
easier to type and costs one extra exchange with the scanner. The command
reads only: it changes nothing on the scanner and writes nothing to the config
file. It needs a scanner, so name one with `--device`.

## Usage

```
radiocli systems <list> [flags]
radiocli systems goto <system> [flags]
radiocli systems rename <system> <name> [flags]
radiocli systems new <list> <name> --type <type> [flags]
radiocli systems delete <system> --yes [flags]
```

## Parameters

`systems` takes one argument and no flags of its own.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<list>` | Yes | none | The favorites list to look inside, by index or by name. |

### `<list>`

Which favorites list to report the systems of. There is no default: the
scanner holds several lists and no one of them is the obvious one to mean.

An argument made only of digits is taken as an index and used as it stands,
which keeps the command to a single exchange with the scanner:

```
$ radiocli systems 2
NAME          TYPE          SCANNED  QUICK KEY  NUMBER TAG
GREENDALE ST  Conventional  yes      -          -
```

Anything else is taken as a name, matched without regard to case. A name costs
one extra exchange, because the favorites lists have to be read to find which
index it refers to:

```
$ radiocli systems "PUBLIC SAFETY"
$ radiocli systems "public safety"
```

A name no list carries is refused, and the message lists what the scanner does
hold, so the next attempt does not need another command:

```
$ radiocli systems "POLICE"
error: no favorites list is called "POLICE": the scanner has "Full Database", "Search with Scan", "Quick Save Favorites List", "PUBLIC SAFETY"
```

Nothing stops two favorites lists sharing a name. When one is ambiguous the
command refuses rather than guessing which was meant, and says to use an index
instead.

The two built-in scan sources are refused outright, whether they are named or
given by their reserved index:

```
$ radiocli systems "Full Database"
error: "Full Database" is the scanner's built-in database rather than a favorites list, and asking it for its systems returns a short, wrong answer and then locks the scanner up until it is power cycled: run "radiocli scanning systems" to read what it is scanning instead

$ radiocli systems "Search with Scan"
error: "Search with Scan" is built into the scanner rather than a favorites list, and holds no systems of its own: it sweeps frequency ranges
```

The database's lockup has been reproduced twice, so the refusal is enforced in
the code rather than left to the reader. Nothing is sent to the scanner in
either case. [`scanning systems`](scanning.md) is how the database gets read.

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column
  of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the systems are printed as a table or as JSON.

`--pace` has no effect on the bare command or on `goto`, neither of which
presses a key. It matters to `rename`, which presses one for every character
typed.

## Examples

Listing the systems in a favorites list by name:

```
$ radiocli systems "PUBLIC SAFETY"
NAME          TYPE          SCANNED  QUICK KEY  NUMBER TAG
GREENDALE ST  Conventional  yes      -          -
```

The same by index, which is one exchange rather than two:

```
$ radiocli systems 2
NAME          TYPE          SCANNED  QUICK KEY  NUMBER TAG
GREENDALE ST  Conventional  yes      -          -
```

As JSON, which carries the index of each system:

```
$ radiocli systems 2 -o json
[
  {
    "name": "GREENDALE ST",
    "index": "4",
    "kind": "Conventional",
    "avoided": false
  }
]
```

A favorites list that holds nothing:

```
$ radiocli systems "Quick Save Favorites List"
That favorites list holds no systems.
```

## `systems goto`

`goto` puts the scanner into the menu for one system, where its name, its
options, and its departments can be edited:

```
$ radiocli systems goto "GREENDALE ST"
menu: GREENDALE ST

   INDEX  ENTRY
>  0      Edit Name
   1      Edit Sys Option
   2      Edit Department
   3      Copy System
   4      Delete System
```

The system may be named by index or by name, the same as the argument to the
bare command. This is a single jump rather than a walk through the menus: the
protocol takes the system's index directly, so it lands in one step whatever
the scanner was showing before, and nothing is counted or stepped through.

Naming it by name costs the lookup described under
[`departments`](departments.md), because a system's index says nothing about
which favorites list holds it.

**This stops the scanner scanning.** Run `radiocli menu close` when finished,
or `radiocli key avoid` from a screen that refuses it. See [`menu`](menu.md)
for which screens refuse what.

## `systems rename`

`rename` changes a system's name:

```
$ radiocli systems rename "GREENDALE ST" "PUBLIC SAFETY"
PUBLIC SAFETY
```

It opens the system's menu, selects `Edit Name`, and types the new name by
turning the knob to each character in turn, checking every one after it is set.
This is the same machinery as [`favorites rename`](favorites.md), and the notes
there about routing, cost, and what happens on failure apply here too.

It is quicker than renaming a favorites list, because reaching a system's menu
is a single exchange rather than a walk.

**A system already called that is left alone.** The name the scanner has is read
before anything is pressed, and a rename that would change nothing says so and
stops rather than stopping the scan to type the same name back:

```
$ radiocli systems rename "PUBLIC SAFETY" "PUBLIC SAFETY"
That system is already called "PUBLIC SAFETY".
```

**Nothing is saved until the whole name is typed**, so an interrupted rename
leaves the old name untouched.

**This stops the scanner scanning**, and returns it to scanning when it is done.

## `systems new`

`new` creates a system inside a favorites list:

```
$ radiocli systems new "GREENDALE, ST 00000" "GREEN BANK" --type Conventional
GREEN BANK (Conventional)
```

**`--type` is required and cannot be changed afterwards.** The scanner asks for
it before the system exists, and offers no way to alter it later, so getting it
wrong means deleting the system and starting again. That is why nothing is
defaulted, even though `Conventional` is what most non-trunked setups want.

| Accepted types |
| -------------- |
| `Conventional` |
| `P25 Trunk`, `P25 X2-TDMA`, `P25 One Frequency` |
| `Motorola` |
| `MotoTRBO Trunk`, `DMR One Frequency` |
| `NXDN Trunk`, `NXDN One Frequency` |
| `EDACS`, `LTR` |

Matching ignores case, so `conventional` is accepted. An unknown type is refused
before the scanner is touched:

```
$ radiocli systems new "GREENDALE, ST 00000" "GREEN BANK" --type bogus
error: no system type is called "bogus": want P25 Trunk, P25 X2-TDMA, ...
```

The scanner asks to confirm the type before creating anything, and that prompt
is answered for you. Nothing exists until it is, so a failure before that point
leaves the scanner exactly as it was.

**This stops the scanner scanning**, and returns it when finished.

## Output

The table goes to stdout. The empty-list message goes to stderr, as do debug
logs from `--verbose`. Redirecting stderr leaves stdout holding the table
alone.

Under `--output text`, stdout holds a header row and one row per system, with
the columns padded so they line up:

```
NAME          TYPE          SCANNED  QUICK KEY  NUMBER TAG
GREENDALE ST  Conventional  yes      -          -
```

| Column | Description |
| ------ | ----------- |
| `NAME` | The system's name, as the scanner holds it. |
| `TYPE` | `Conventional`, or the name of the trunking technology for a trunked system. Passed through as the scanner spells it. |
| `SCANNED` | `yes` when the scanner is scanning the system, `no` when it is being skipped. |
| `QUICK KEY` | The quick key that switches this system, or `-` when none is assigned. |
| `NUMBER TAG` | The number tag assigned, or `-` when none is assigned. |

`SCANNED` is the opposite of what the scanner reports. The protocol says
whether a system is *avoided*, and a table of double negatives is hard to read,
so this reports whether it will be heard.

When the favorites list holds no systems, stdout is empty and stderr says so.
The command exits `0`, because an empty list is a complete answer to what is
inside it.

Under `--output json`, stdout holds an array of objects, one per system, in the
order the scanner reports them:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The system's name. |
| `index` | string | How the scanner names this system, and what [`departments`](departments.md) takes to list what is inside it. |
| `kind` | string | The system type, as the scanner spells it. |
| `avoided` | boolean | Whether the scanner is skipping this system. Note this is the scanner's own sense, the opposite of the `SCANNED` column. |
| `quickKey` | string | The quick key assigned, absent when none is. |
| `numberTag` | string | The number tag assigned, absent when none is. |

## Telling an empty list from one that is not there

The scanner answers a request naming an index that does not exist exactly as it
answers a request for a list holding nothing: with an empty document and no
error. The two cannot be told apart from that answer alone.

When there is nothing to report, this command reads the favorites lists to
settle which happened, so a mistyped index is called out rather than being
reported as an empty list:

```
$ radiocli systems 999
error: no favorites list has index 999: the scanner has "Full Database", ...
```

That extra exchange happens only when the answer is empty. A list with systems
in it costs one exchange, or two when named by name.

## `systems delete`

`delete` removes a system, and everything inside it:

```
$ radiocli systems delete "TEST SYS" --yes
deleted TEST SYS
```

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--yes` | Yes | `false` | Go ahead and delete it. |

**`--yes` is required.** Without it nothing is touched and the command says
what would have gone:

```
$ radiocli systems delete "TEST SYS"
error: deleting the system "TEST SYS" removes it and every department and channel in it, and cannot be undone: pass --yes
```

**It takes the departments and channels with it.** Measured on a system holding
one department: after the delete, the department could not be found anywhere on
the scanner.

**There is no undo.** Nothing on the scanner keeps a copy and this tool cannot
put back what it removes. The name is resolved before anything is pressed, so a
name the scanner does not have costs nothing, and the systems are read back
afterwards to confirm it is gone.

The system may be named by index or by name, the same as the argument to the
bare command.

**This stops the scanner scanning**, and returns it when finished.

## JSON output from `new`, `rename` and `delete`

The three verbs above print a line of text, and under `--output json` they print
an object instead. One shape covers all of them, and the same shape covers the
create, rename and delete verbs on every other level of the scanner's memory, so
a script driving edits does not have to learn a different object for each:

```json
{
  "action": "renamed",
  "kind": "system",
  "name": "NEW NAME",
  "was": "OLD NAME"
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `action` | string | What happened: `created`, `renamed` or `deleted`. |
| `kind` | string | What it happened to, which is always `system` here. |
| `name` | string | What the entry is called now, or was called when it was deleted. It is the scanner's own spelling, read back after the change. |
| `was` | string | What the entry was called before a rename, as the scanner spelled it. Absent on a create or a delete. |
| `in` | string | the favorites list it is in, as you named it. |

Fields that do not apply are left out rather than written empty, so a consumer
can tell a create from a rename without comparing strings.

The text output is unchanged. It is what people already read, so the machine-
readable mode was added beside it rather than in place of it.

## Long lists

The scanner caps a list reply at roughly a kilobyte and offers no way to ask for
the rest. On a favorites list with a lot of systems, the reply stops early and is marked `EOT="0"` to say
there is more; repeating the request answers with the same first part.

`radiocli` used to hand that straight back, which reported a short list as though
it were the whole thing. It no longer does. When the scanner admits it cut the
list short, the missing names are read the slow way, off the scanner's own menus,
which is the one reading that misses nothing. That costs several seconds and
stops the scan, and it happens only when the scanner has said the list was short.

Only the **names** come off the screen. Everything else about a system lives
in the list the scanner would not finish sending, so those columns are shown as
`?`, which means "not read" and is a different thing from the `-` that means "the
scanner says there is nothing here". A note on stderr says how many systems that
covers. Under `--output json` those entries carry `"partial": true` instead.

```
NAME       TYPE  SCANNED  QUICK KEY  NUMBER TAG
GREENDALE  P25   yes      3          1
MILLBROOK  ?     ?        ?          ?
```

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: accepts 1 arg(s), received 0` | No favorites list was named. | Give an index or a name, from [`favorites`](favorites.md). |
| `error: no favorites list is called "<name>": the scanner has ...` | No list carries that name. | Use one of the names in the message, or an index. |
| `error: no favorites list has index <n>: the scanner has ...` | No list carries that index. | Run [`favorites`](favorites.md) with `-o json` to see the indexes. |
| `error: <n> favorites lists are called "<name>": name one by its index instead` | Two or more lists share the name given. | Use the index, from [`favorites`](favorites.md) with `-o json`. |
| `error: "Full Database" is the scanner's built-in database rather than a favorites list, and asking it for its systems ...` | The database was named, by name or by its reserved index. The request it would send locks the scanner up. | Run [`scanning systems`](scanning.md), which reads the database by turning the knob. |
| `error: "Search with Scan" is built into the scanner rather than a favorites list, and holds no systems of its own ...` | The built-in search source was named. It sweeps frequency ranges and holds no systems. | Name a favorites list instead, from [`favorites`](favorites.md). |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: reading the favorites lists: <detail>` | The list had to be looked up by name or checked for existence, and that read failed. | Run with `--verbose` to see the raw exchange. |
| `error: reading the systems: <detail>` | The scanner did not answer the request for the systems. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |
| `error: reading the systems: the scanner answered with a FL list instead of a SYS list, which is how this firmware refuses a list it cannot produce` | The scanner returned the wrong kind of document. | Report it: this is the failure mode that stops channels being listed, and it is not expected here. |

An empty favorites list is not an error. It prints nothing to stdout, explains
itself on stderr, and exits `0`.
