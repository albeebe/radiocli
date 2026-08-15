# menu

Reports the menu the scanner is showing, and moves around inside the menus.
Run it to see where the scanner is before pressing anything.

## Overview

`menu` on its own reports the menu currently on the scanner's screen, with its
entries and their indexes, and says plainly when the scanner is not in a menu
at all. That is a read and changes nothing.

The subcommands do change things. `menu open` puts the scanner into a named
menu, which stops it scanning. `menu back` climbs one level, `menu close`
leaves the menus and returns to scanning, and `menu set` writes a value into
the item the scanner is on, which is how a name or number is entered without a
key press per character. Each write reports the menu it landed on afterwards,
so a step that went somewhere unexpected is visible without a second command.

Menus and [`key`](key.md) are two halves of the same job. This command names
the places worth going; `key` moves the selection around once there. It needs
a scanner, so name one with `--device`.

## Usage

```
radiocli menu [flags]
radiocli menu open <menu> [index] [flags]
radiocli menu back [flags]
radiocli menu close [flags]
radiocli menu set <value> [flags]
```

## Parameters

The bare command takes no arguments. The subcommands take these:

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<menu>` | Yes | none | Which menu to open, for `menu open`. |
| `[index]` | No | none | Which system, department, site, channel, or search bank to open on. |
| `<value>` | Yes | none | The value to write, for `menu set`. |

### `<menu>`

The menu to open. Accepted names:

| Name | Opens |
| ---- | ----- |
| `top` | The top of the menu tree. |
| `favorites` | The favorites list menu. |
| `system`, `department`, `site`, `channel` | The menu for one entry of the scanner's memory. These take an index. |
| `search-range`, `search-options` | The search menus. |
| `close-call`, `close-call-band` | The Close Call menus. |
| `weather` | The weather menu. |
| `tone-out` | The tone-out channel menu. |
| `settings` | The settings menu. |
| `broadcast-screen` | The broadcast screen menu. |

The name is checked before the scanner is opened, so a typo opens nothing.

### `[index]`

Which entry the menu opens on, for the menus that take one. Get indexes from
[`favorites`](favorites.md), [`systems`](systems.md), or
[`departments`](departments.md) under `-o json`. Menus that do not need an
index ignore it.

**The menus that need one are refused without one.** There is no such thing as
"the system menu" on its own, so `menu open system` fails rather than opening
something arbitrary:

```
$ radiocli menu open system
error: opening the system menu: scanner rejected the command in its current mode: MNU,SCAN_SYSTEM,
```

**The indexes are one space shared by every level, and they have gaps.** They
are not positions. On a scanner whose only system is index `4` and whose two
departments are `7` and `11`, the channels answer to `9`, `13` and `15`.
Counting from zero finds nothing: `menu open system 0` is refused because no
system is stored there.

**An index belonging to another level is not refused.** Handed a system's index,
the channel menu opens the main menu instead:

```
$ radiocli menu open channel 4
menu:     --- M E N U ---
```

That is the scanner's own behaviour and there is no way to tell in advance.
It is why this command always reports the menu it landed on rather than
assuming it went where it was sent, and why the reply is worth reading.

Channels are the one level with no index published anywhere:
[`channels`](channels.md) reports names and frequencies. Finding a channel's
index means trying them.

### `<value>`

The value to write into the menu item the scanner is currently on. What counts
as valid depends entirely on the item, and the scanner refuses anything it will
not take. Check with a bare `menu` first that the scanner is on the item you
mean, because this writes to wherever it actually is rather than to wherever
you believed it was.

## Examples

Asking where the scanner is:

```
$ radiocli menu
The scanner is not in a menu.
```

Opening the settings menu, which reports what it landed on:

```
$ radiocli menu open settings
menu: Settings

INDEX  ENTRY
0      Adjust Key Beep
1      Set Clock
2      Upgrade
3      Battery Options
4      Site NAC Operation
6      Auto Shutoff
7      Bluetooth Options
8      Headphone L/R output
9      Replay Options
11     See Scanner Information
```

Moving the selection and opening an entry, with [`key`](key.md):

```
radiocli key right right
radiocli key enter
```

Returning to scanning:

```
$ radiocli menu close
The scanner has left the menus.
```

## Output

The menu goes to stdout. The not-in-a-menu message, the empty-menu note, and
the confirmation from `menu close` go to stderr, as do debug logs from
`--verbose`.

Under `--output text`, stdout holds the menu's title, then a blank line, then a
table of its entries:

```
menu: Settings

   INDEX  ENTRY
>  0      Adjust Key Beep
   1      Set Clock
```

A `>` in the first column marks the entry the scanner has highlighted, which is
the one a key press will act on.

Indexes are the scanner's own and are not always contiguous; the settings menu
above skips `5` and `10`. An entry with no index shows `-`.

**This list is not the screen.** It is what the protocol says the menu holds,
and on the firmware tested it can leave entries out: the favorites list menu
omits `Review Avoids`, which is plainly on the display and which the knob steps
through like any other entry. Never count entries here to work out how far to
turn the knob. Use [`screen`](screen.md), which shows what is really there.

When the scanner is not in a menu, stdout is empty and stderr says so, and the
command exits `0`. Not being in a menu is an answer to which menu is showing,
not a failure. Under `--output json` that same case prints `null`.

Under `--output json`, stdout holds one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `title` | string | The menu's heading, as shown at the top of the screen. |
| `kind` | string | The kind of menu, such as `TypeSelect`, or `TypeInput` for a text entry screen. Absent when the scanner does not say. |
| `items` | array | The entries, each with `name`, `index`, and `highlighted`. Empty on a text entry screen. |

## Getting out again

`menu close` is the ordinary way back to scanning, but it does not work
everywhere. Some screens refuse it, and refuse `menu back` too:

```
$ radiocli menu back
error: going back: scanner rejected the command in its current mode: MSB,
```

The reliable escape is [`scan`](scan.md), which tries the protocol's way out
and falls back to a key press when it is refused:

```
$ radiocli scan
The scanner has left the menus, after one key press.
```

By hand, the key that has worked from every screen tested is `avoid`. `soft1`
leaves most menus but was refused on the text entry screen:

```
radiocli key avoid
```

If a command is behaving strangely, check with a bare `menu` whether the
scanner is sitting in one. Some commands are refused while it is, though fewer
than might be expected. Measured with the top menu open,
[`favorites`](favorites.md), [`systems`](systems.md),
[`departments`](departments.md), [`volume`](volume.md) and
[`battery`](battery.md) all answer normally. [`location`](location.md) is the
one read that does not:

```
$ radiocli location
error: reading the location: scanner rejected the command in its current mode: LCR
```

## What the listing leaves out

**The scanner's own menu listing can omit rows that are really on screen.** A
favorites list's menu is shown with ten entries and has eleven rows: `Review
Avoids` is on the screen, in the knob's path, and missing from the listing.
That is why the indexes in the table have gaps in them, and why nothing in this
tool counts positions to reach an entry.

**Which entry is highlighted comes from the screen**, not from the listing.
Taking it from the listing was wrong in exactly the way the omission implies:
with the knob on `Delete`, the listing said `Information`, one entry further
down, because a row above the cursor was missing from its count.

When the row the knob is on is not in the listing at all, the command says so
rather than marking nothing:

```
$ radiocli menu
The scanner is on "Review Avoids", which this listing does not include. Use "radiocli screen" to see the rows as they are.
menu: GREENDALE, ST 00000
...
```

Use [`screen`](screen.md) to see the rows exactly as the scanner draws them.

## Errors

Every failure exits with status `1` and prints to stderr.

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no menu is called "<name>": want ...` | The name is not one of those listed. | Use a name from the table. Nothing was opened. |
| `error: opening the <name> menu: <detail>` | The scanner refused to open it. | Check what is on its screen; some menus are refused from some places. |
| `error: going back: <detail>` | The scanner refused `MSB`. | Use `radiocli key soft1` to leave the menus. |
| `error: closing the menu: <detail>` | The scanner refused to leave the menus. | Use `radiocli key soft1`. |
| `error: setting the menu value to "<value>": <detail>` | The scanner would not take that value for the item it is on. | Check with a bare `menu` which item it is actually on. |
| `error: reading the menu: <detail>` | The scanner did not answer the request. | Run with `--verbose` to see the raw exchange. |

The scanner not being in a menu is not an error.
