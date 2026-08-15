# screen

Reports what is on the scanner's display, line by line. Run it to see exactly
what the scanner is showing without looking at it.

## Overview

`screen` is the most reliable view of the scanner this tool has. It works in
every mode, including menus and text entry screens where other commands are
refused or have nothing to report, and it marks the row the scanner has
highlighted. While scanning it shows what is being received along with the
volume, squelch, and battery voltage, all in one reading.

It is also the only honest view of a menu. [`menu`](menu.md) reports the
entries the protocol says a menu holds, and that list can omit entries which
are really on screen and really in the path of the knob. Anything counting
menu positions should count them here. The command reads only: it changes
nothing on the scanner and writes nothing to the config file. It needs a
scanner, so name one with `--device`.

## Usage

```
radiocli screen [flags]
```

## Parameters

`screen` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | - | - | This command takes no flags and no arguments. |

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column
  of [`devices`](devices.md).
- `--output` selects whether the display is printed as lines or as JSON.

`--pace` has no effect here. This command presses no keys.

## Examples

Reading the scanning screen:

```
$ radiocli screen
                  Aug.2 16:23
  F0:----------
  S0:----------   VOL:15 SQL: 2
  D0:----------   Tag:--.--.---
  GREENDALE ST
  PUBLIC SAFETY
  FIRE

  Scanning...

  Sys ID: ---     TGID: ---
  RFSS ID: ---    Site ID: ---
  WACN: ---       Batt:3.97V
  UID: ---        RSSI: ---
>  SYSTEM      DEPT     CHANNEL
```

Reading a menu, where `>` marks the highlighted entry:

```
$ radiocli screen
  POLICE
> Edit Name
  Set Department Quick Key
  Edit Channel
  Set Location Information
  Set Avoid
  Delete Department
  New Department
```

Reading a text entry screen, which [`menu`](menu.md) cannot show at all:

```
$ radiocli screen
  Edit Name
> PUBLIC SAFETY
```

As JSON, for a script that needs to find the highlighted row:

```
$ radiocli screen -o json
{
  "lines": [
    { "text": "  POLICE", "highlighted": false, "largeFont": false },
    { "text": "Edit Name", "highlighted": true, "attributes": "*********", "largeFont": false }
  ]
}
```

## Output

The display goes to stdout. Debug logs from `--verbose` go to stderr.

Under `--output text`, stdout holds one line per row of the display. Rows are
indented two spaces, and the highlighted row is marked with `> ` in place of
that indent, so the marker does not shift the text:

```
  POLICE
> Edit Name
```

Trailing spaces are removed. The scanner pads every row to the display's width,
and that padding carries no meaning.

Blank rows are printed as blank lines rather than dropped, because the spacing
is part of how the screen reads.

Under `--output json`, stdout holds one object with a `lines` array, in display
order:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `lines` | array | One object per row of the display. |
| `lines[].text` | string | The row's text, with trailing padding removed. |
| `lines[].highlighted` | boolean | Whether the row is in reverse video, which is how the scanner shows the selection. |
| `lines[].attributes` | string | How each character of the row is drawn, one character per character. Absent when the whole row is drawn normally. |
| `lines[].largeFont` | boolean | Whether the row is drawn in the scanner's large font. Always present, on every row. |

Exactly one row is normally highlighted. A screen with no selection, such as
some status screens, reports none.

### `attributes`

One character per character of the row:

| Character | Meaning |
| --------- | ------- |
| ` ` (space) | Drawn normally |
| `*` | Reverse video, which marks the selection |
| `_` | Underline, which marks a heading |

`highlighted` only says that a row has reverse video somewhere. `attributes`
says where, which is what tells one highlighted thing from another on the same
row.

The soft key labels along the bottom are the useful case. They arrive as three
runs of `*` with normal gaps between them, so their positions fall out without
parsing the text:

```
$ radiocli screen -o json
...
{
  "text": " SYSTEM      DEPT     CHANNEL",
  "highlighted": true,
  "attributes": "********* ********** *********"
}
```

That is three buttons at columns 0, 10 and 21, nine, ten and nine characters
wide.

**`attributes` can be longer than `text`.** Both have their trailing spaces
removed, and reverse video can extend past the last visible character, which is
how the scanner draws a highlight bar across the rest of a row. Pad `text` to
the length of `attributes` before indexing into both.

### `largeFont`

The scanner has two fonts and says which one it drew each row in. The normal
font fits 30 characters across the display. The large font fits 24, so a row
with `largeFont` set is 24 characters wide rather than 30.

A large character is **a quarter wider** than a normal one and **twice as
tall**, so 24 of them fill the same width as 30 normal ones and a large row
reaches the right hand edge like any other row. In pixels the display is 480
across and 320 down: 30 characters of 16 or 24 of 20, and 16 pixels to a normal
row against 32 for a large one. That is why the simple layout's 14 rows and the
detail layout's 17 come to the same screen, and why a menu of 10 rows, every one
of them large, comes to it as well.

The scanner uses it for the three names it wants read at a glance: the system,
the department and the channel. On a scanning screen those are rows 4, 6 and 8:

```
$ radiocli screen -o json
{
  "lines": [
    { "text": "              Aug.6 17:57", "highlighted": false, "largeFont": false },
    { "text": "PUBLIC SAFETY", "highlighted": false, "largeFont": true },
    { "text": "GREENDALE, ST 00000", "highlighted": false, "largeFont": false }
  ]
}
```

The field is on every row, not only the large ones, so anything laying out a
row can read it without having to treat a missing field as a third answer.

This only matters to something drawing the screen rather than reading it. A row
of 24 characters cannot otherwise be told apart from a short row of 30, and
drawing it as one leaves the name four fifths of the width and half the height
it should be. Reading the text alone needs nothing from this field.

### Characters above `0x7E`

The scanner's font has pictures where a font normally has nothing, and the
display carries them in a row like any other character: the signal meter is
`0xAC` and `0xAD`, the scan direction arrow is `0x1A` and `0x1B`, and the
modulation marks are `0x14` and `0x15`. They are not text and there is no
character that means "left half of an arrow".

Each byte is reported as the character of the same value, so a byte `0xAC`
arrives as `U+00AC` and its number is the number the scanner sent. That is what
to index the font with. [research/glyphs.md](../../research/glyphs.md) draws all
256 of them.

Anything rendering the text has to decide what to do with these. They cannot be
passed through, because a terminal draws a control code as nothing or as a box
of the wrong width and the rest of the row shifts a column across. One space
each keeps the row the width the scanner drew it.

## Why this is the reliable view

The scanner reports its menus two ways, and they disagree.

[`menu`](menu.md) asks the protocol what the menu holds. On SDS150 firmware
1.00.37 that answer can be incomplete: the favorites list menu omits
`Review Avoids`, which is plainly on the screen and which the knob steps
through like any other entry. Counting entries from that list to work out how
far to turn the knob gives the wrong answer, and the entry it lands on may be
next to something destructive.

`screen` reports what the display actually shows, including that entry, and
marks what is highlighted. Anything that navigates by stepping should check its
position here after each step, by name rather than by count.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: reading the screen: <detail>` | The scanner did not answer the request for its display. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |
