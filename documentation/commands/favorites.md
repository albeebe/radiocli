# favorites

Lists the scanner's favorites lists and says which of them are being scanned.
Run it to see what the scanner is set up to listen to.

## Overview

A favorites list is the top of the scanner's memory. Each one holds systems,
which hold departments, which hold channels, and the scanner scans the lists
you have switched on. `favorites` reports those lists by name, says which are
included in scanning, and shows the quick key and number tag assigned to each.
Alongside the lists you created, the scanner reports two of its own scan
sources, the full RadioReference database and Search with Scan; both are
marked so they are not mistaken for lists you can edit. To see what is inside
a list, run [`systems`](systems.md) with its name or index. The bare command
reads only: it changes nothing on the scanner and writes nothing to the config
file. [`scan`](#favorites-scan) chooses which lists are scanned. It needs a
scanner, so name one with `--device`.

## Usage

```
radiocli favorites [flags]
radiocli favorites goto <list> [flags]
radiocli favorites rename <list> <name> [flags]
radiocli favorites new <name> [flags]
radiocli favorites scan <name>... [flags]
radiocli favorites scan --all | --none [flags]
radiocli favorites delete <list> --yes [flags]
```

## Parameters

`favorites` has no flags of its own. Its behaviour is controlled entirely by
the [global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | - | - | This command takes no flags and no arguments. |

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column
  of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the lists are printed as a table or as JSON.

`--pace` has no effect on the bare command, which presses no keys. It matters a
great deal to `goto`, `rename` and `scan`, which press one for every step
through the menus and every character typed.

## Examples

Listing the favorites lists:

```
$ radiocli favorites
   NAME                       SCANNED  QUICK KEY  NUMBER TAG
+  Full Database              no       -          -
+  Search with Scan           no       -          -
   Quick Save Favorites List  no       -          -
   PUBLIC SAFETY              yes      -          -

+ built into the scanner, not a list you created
```

The same listing as JSON:

```
$ radiocli favorites -o json
[
  {
    "name": "Full Database",
    "index": "4294967295",
    "monitored": false,
    "builtIn": true
  },
  {
    "name": "PUBLIC SAFETY",
    "index": "2",
    "monitored": true,
    "builtIn": false
  }
]
```

Listing a scanner other than the selected one:

```
radiocli --device /dev/cu.usbmodem00000000000011 favorites
```

## Output

The table goes to stdout. The footnote and the empty-list message go to stderr,
as do debug logs from `--verbose`. Redirecting stderr leaves stdout holding the
table alone.

Under `--output text`, stdout holds a header row and one row per list, with the
columns padded so they line up:

```
   NAME                       SCANNED  QUICK KEY  NUMBER TAG
+  Full Database              no       -          -
   PUBLIC SAFETY              yes      -          -
```

| Column | Description |
| ------ | ----------- |
| `NAME` | The list's name, as the scanner holds it. |
| `SCANNED` | `yes` when the list is included while scanning, `no` when it is not. |
| `QUICK KEY` | The quick key that switches this list, or `-` when none is assigned. |
| `NUMBER TAG` | The number tag assigned to the list, or `-` when none is assigned. |

A `+` in the first column marks a scan source built into the scanner rather
than a list you created, and a footnote on stderr says so. There are two: the
full RadioReference database, and Search with Scan. They appear here because
whether they are scanned is the same question being asked of every other row.

`SCANNED` reads `yes` and `no` rather than the scanner's own `On` and `Off`,
which read as though the list itself were switched off rather than included in
scanning.

When the scanner reports no lists at all, stdout is empty and stderr explains
why. The command still exits `0`, because an empty memory is a complete answer
to what is stored.

Under `--output json`, stdout holds an array of objects, one per list, in the
order the scanner reports them:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The list's name. |
| `index` | string | How the scanner names this list internally. A string rather than a number, because the built-in sources use reserved values far outside the range of an ordinary list. |
| `monitored` | boolean | Whether the list is included while scanning. |
| `quickKey` | string | The quick key assigned, absent when none is. |
| `numberTag` | string | The number tag assigned, absent when none is. |
| `builtIn` | boolean | True for the scanner's own scan sources, false for a list you created. |

## `favorites goto`

`goto` puts the scanner into the menu for one favorites list, where it can be
renamed, given a quick key or a number tag, or deleted:

```
$ radiocli favorites goto "PUBLIC SAFETY"
menu: PUBLIC SAFETY

   INDEX  ENTRY
>  0      Review/Edit System
   1      Set FL Quick Key
   2      Set FL Number Tag
   3      Set FL Startup Key
   4      Use Location Control
   6      Stop All Avoiding
   7      Add Current dB Channels
   8      Rename
   9      Delete
   10     Information
```

The list may be named by index or by name. A built-in scan source is refused,
because those have no menu of their own:

```
$ radiocli favorites goto "Full Database"
error: "Full Database" is built into the scanner and has no menu of its own: only a list you created can be opened this way
```

That menu is worth knowing about: `Set FL Quick Key` is the only way to assign
a quick key, which the protocol can read and enable but not assign.

### This one walks

[`systems goto`](systems.md) and [`departments goto`](departments.md) jump
straight to their menu in a single exchange, because the protocol takes an
index for those. It offers no menu for a single favorites list, so this one
walks instead: the scanner is sent to the top menu, the knob is turned to
`Manage Favorites`, and then to the list.

Every step is checked against the display by name, never by counting, so it
cannot land one entry short or one past. That matters on this menu in
particular, where `Delete` sits directly after `Rename`. If a menu turns all
the way round without the name appearing, the walk stops and reports what it
saw rather than guessing.

It starts by opening the top menu, so it lands in the same place whatever the
scanner was showing before.

The list is resolved before the scanner is touched, so a name the scanner does
not have costs nothing and leaves it scanning.

**This stops the scanner scanning.** Run [`scan`](scan.md) when finished.

## `favorites rename`

`rename` changes a favorites list's name:

```
$ radiocli favorites rename "PUBLIC SAFETY" "GREENDALE ST 00000"
GREENDALE ST 00000
```

The scanner has no command for this. `rename` walks to the list exactly as
`goto` does, selects `Rename`, and types the new name the only way the scanner
allows: by turning the knob to cycle the character under the cursor and pressing
a key to move along. Each character is read back after it is set, so a press the
scanner missed is caught where it happened.

The work is proportional to how much of the name changes, not how long it is.
Characters that already match are stepped over for the cost of one press, and
each one that does not is routed the short way round the cycle, forwards or
backwards, never more than 47 presses. A character being set to a space is one
press rather than a journey.

At the default pace a substantial rename takes a few seconds. At `--pace slow`
the same edit takes minutes, which is occasionally what you want in order to
watch it happen on the scanner.

The name may hold anything in the scanner's character set, which includes
lowercase, digits, punctuation and spaces, up to the length the screen reports.
Anything outside it is refused, though not until the entry screen has been
reached, because the accepted set is something that screen reports rather than
something known in advance:

```
$ radiocli favorites rename "GREENDALE ST" "café"
error: typing the new name: the scanner does not accept "Ã" on this screen, which is character 4 of "café"
The scanner is still on the entry screen and nothing has been saved. Run "radiocli scan" to leave it as it was
```

The character quoted is a byte rather than the letter, because the scanner
works in single bytes and anything outside ASCII arrives as more than one.

**Nothing is saved until the whole name is typed.** Leaving the entry screen
discards, so a rename that fails partway, or that you interrupt, leaves the old
name untouched. The error says so explicitly rather than leaving you to wonder.

**The list is read back afterwards**, the way `new` and `delete` read back, so
what gets reported is the name the scanner kept rather than the name that was
typed. A list that does not carry the new name afterwards is reported as that:

```
$ radiocli favorites rename "GREENDALE ST" "PUBLIC SAFETY"
error: the list does not appear under "PUBLIC SAFETY" afterwards: run "radiocli favorites" to see what it is called
```

**A comma in a name is legal but awkward.** The scanner accepts one, and this
tool handles it, but the screen-reading protocol is comma separated and does not
escape it, so every command that reads the display has to reassemble the line.
That is done for you; it is worth knowing if you compare raw protocol output.

**This stops the scanner scanning**, and returns it to scanning when it is done.

## `favorites new`

`new` creates a favorites list:

```
$ radiocli favorites new "GREEN BANK, WV 24944"
GREEN BANK, WV 24944
```

Pressing the scanner's own `New Favorites List` creates one immediately, named
something like `FAVORITES 0`, and the name you asked for is typed over it. The
list is read back afterwards to confirm it is there, because a list created and
then left unnamed is worth knowing about.

A new list holds nothing. Put a system in it with [`systems new`](systems.md),
and note that it is switched on for scanning from the moment it exists, so an
empty list is scanned and finds nothing until you fill it.

**This stops the scanner scanning**, and returns it when finished.

## `favorites delete`

`delete` removes a favorites list, and everything inside it:

```
$ radiocli favorites delete "GREEN BANK, WV 24944" --yes
deleted GREEN BANK, WV 24944
```

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--yes` | Yes | `false` | Go ahead and delete it. |

**`--yes` is required.** Without it nothing is touched and the command says
what would have gone:

```
$ radiocli favorites delete "GREEN BANK, WV 24944"
error: deleting the favorites list "GREEN BANK, WV 24944" removes it and everything in it, and cannot be undone: pass --yes
```

**It takes the systems, departments and channels with it.** A list is a
container, and deleting one deletes the tree under it. Measured on a list
holding one system, one department and one channel: after the delete, none of
the four could be found anywhere on the scanner.

**There is no undo.** Nothing on the scanner keeps a copy and this tool cannot
put back what it removes. The name is resolved before anything is pressed, so a
name the scanner does not have costs nothing, and the list is read back
afterwards to confirm it is gone.

**A built-in scan source cannot be deleted**, because the scanner has no way to
remove one:

```
$ radiocli favorites delete "Full Database" --yes
error: "Full Database" is built into the scanner and cannot be deleted: only a list you created can be
```

**This stops the scanner scanning**, and returns it when finished.

## `favorites scan`

`scan` chooses which lists the scanner scans:

```
$ radiocli favorites scan "GREENDALE, ST 00000"
   NAME                       SCANNED  QUICK KEY  NUMBER TAG
+  Full Database              no       -          -
+  Search with Scan           no       -          -
   Quick Save Favorites List  no       -          -
   GREENDALE, ST 00000        yes      -          -
```

| Form | What it does |
| ---- | ------------ |
| `scan <name>...` | Scans exactly those lists and switches every other one off |
| `scan --all` | Scans everything, including the built-in sources |
| `scan --none` | Scans nothing |

**Naming lists means only those.** It is "scan this" rather than "also scan
this", because that is almost always the question being asked. Everything
starts from the scanner's own `Set All Lists Off`, so the result does not
depend on what each list happened to be set to beforehand.

The lists are resolved before the scanner is touched, so a name it does not
have costs nothing and leaves it scanning. Built-in sources can be named like
any other, so `favorites scan "Full Database"` is a way to scan the database
alone.

**This is the command to run after [`location set`](location.md)**, which
switches Full Database scanning on every time and so undoes any narrower
choice.

### Why this one is fiddly

The scanner draws these rows with their state baked into the line, as
`GREENDALE, ST 00000 :On`, so they are not ordinary menu entries and the shared
menu walk cannot match them. Two consequences:

- **The name is matched with the state stripped off**, and only as a prefix when
  the scanner cut it short to fit: `Quick Save Favorites List` shows as `Quick
  Save Favorit`, and has to match. A row shown in full is the whole name and
  must not. Matching a prefix either way looks equivalent and is not: asked to
  scan `RADIOCLI TEST ONE`, the walk met the shorter `RADIOCLI TEST` first,
  matched it, switched on the wrong list and reported success.
- **Pressing a row toggles it rather than setting it.** So each row's state is
  read before pressing and read again afterwards. Pressing blind would switch
  off the very list you asked for, whenever it happened to be on already.

Switching the full database on or off also sends the scanner away to rebuild,
during which it stops answering entirely. This waits for it rather than
reporting a failure for work that succeeded.

**Two lists whose names are the same up to where the screen cuts them cannot be
told apart here**, and the first one the walk reaches wins. The row carries no
index, so there is nothing else to go on. About nineteen characters survive;
give lists that have to be switched on separately names that differ inside that.

**This stops the scanner scanning**, and returns it when finished.

## How deep this goes

The scanner's memory is four levels deep: favorites list, system, department,
channel. This command reports the first, [`systems`](systems.md) the second,
and [`departments`](departments.md) the third, each for one parent at a time.

The fourth level cannot be read the same way. On SDS150 firmware 1.00.37 the
request for the channels in a department answers with the favorites list
document instead, for any index, including indexes that do not exist, and
reports no error while doing so. The request for a site's frequencies fails the
same way, answering with the tone-out document. Anything reading these documents
therefore has to check that the elements it got back are the ones it asked for,
and every command here does, so a wrong document is reported as an error rather
than shown as though it answered the question.

Channels are still readable, just not from those documents.
[`channels`](channels.md) walks the scanner's own menus and reads them off the
screen, which is how it reports the frequencies as well as the names.

## JSON output from `new`, `rename` and `delete`

The three verbs above print a line of text, and under `--output json` they print
an object instead. One shape covers all of them, and the same shape covers the
create, rename and delete verbs on every other level of the scanner's memory, so
a script driving edits does not have to learn a different object for each:

```json
{
  "action": "renamed",
  "kind": "favorites list",
  "name": "NEW NAME",
  "was": "OLD NAME"
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `action` | string | What happened: `created`, `renamed` or `deleted`. |
| `kind` | string | What it happened to, which is always `favorites list` here. |
| `name` | string | What the entry is called now, or was called when it was deleted. It is the scanner's own spelling, read back after the change. |
| `was` | string | The previous name, on a rename. Absent otherwise. |

Fields that do not apply are left out rather than written empty, so a consumer
can tell a create from a rename without comparing strings.

The text output is unchanged. It is what people already read, so the machine-
readable mode was added beside it rather than in place of it.

## Long lists

The scanner caps a list reply at roughly a kilobyte and offers no way to ask for
the rest. On a scanner with a lot of favorites lists, the reply stops early and is marked `EOT="0"` to say
there is more; repeating the request answers with the same first part.

`radiocli` used to hand that straight back, which reported a short list as though
it were the whole thing. It no longer does. When the scanner admits it cut the
list short, the missing names are read the slow way, off the scanner's own menus,
which is the one reading that misses nothing. That costs several seconds and
stops the scan, and it happens only when the scanner has said the list was short.

Only the **names** come off the screen. Everything else about a favorites list lives
in the list the scanner would not finish sending, so those columns are shown as
`?`, which means "not read" and is a different thing from the `-` that means "the
scanner says there is nothing here". A note on stderr says how many favorites lists that
covers. Under `--output json` those entries carry `"partial": true` instead.

```
  NAME       SCANNED  QUICK KEY  NUMBER TAG
  GREENDALE  yes      3          1
  MILLBROOK  ?        ?          ?
```

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: opening <port>: no such file or directory (run "radiocli devices" to see what is attached)` | The named port does not exist. | Run [`devices`](devices.md) to find the current port, and pass that. |
| `error: reading the favorites lists: <detail>` | The scanner did not answer the request for its lists. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |
| `error: reading the favorites lists: the scanner's answer is not valid XML: <detail>` | The scanner answered, but not with a document that could be parsed. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |

| `error: deleting the favorites list "<name>" removes it and everything in it, and cannot be undone: pass --yes` | `delete` was run without `--yes`. | Pass `--yes` if you mean it. |
| `error: "<name>" is built into the scanner and cannot be deleted: only a list you created can be` | `delete` was pointed at a built-in scan source. | Only lists you made can be deleted. |
| `error: the favorites list "<name>" is still there afterwards: nothing was deleted` | The scanner reported the list again after the delete. | Run [`favorites`](favorites.md) to see what it holds, and try again. |
| `error: the list does not appear under "<name>" afterwards: run "radiocli favorites" to see what it is called` | The name was typed, but the scanner does not report the list under it. | Run [`favorites`](favorites.md) to see what it is called, and try again. |
| `error: name the lists to scan, or pass --all or --none` | `scan` was run with no lists and no flag. | Name a list, or pass `--all` or `--none`. |
| `error: --all and --none ask for opposite things: choose one` | Both flags were passed. | Pass one. |
| `error: naming lists and passing --all or --none ask for different things: choose one` | A name was given alongside a flag. | Pass one or the other. |
| `error: <name> did not switch on: the scanner shows "<row>"` | A row was pressed but did not end up on. | Run `radiocli favorites` to see the current state and try again. |
| `error: no row for "<name>" in the scanner's list of lists` | The list exists but has no row on the monitor screen. | Run [`screen`](screen.md) with that screen open to see what it holds. |

An empty list is not an error. A scanner holding no favorites lists prints
nothing to stdout, explains itself on stderr, and exits `0`.
