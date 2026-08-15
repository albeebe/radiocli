# display

Reports whether the scanner draws its screen in color, and switches it between
color and its two black and white modes.

## Overview

The scanner stores a text color and a background color for every element of
every screen layout, set under `MENU → Display Options → Customize`. This
setting decides whether any of them are drawn. In either black and white mode
those colors are still stored and still editable, and completely ignored.

Anything that redraws the scanner's screen somewhere else should read this
before choosing a palette, or it will paint a color screen while the scanner in
your hand is monochrome.

Reading costs one command over the wire, works in every mode including menus,
and does not stop the scan. Writing does not: the scanner offers no way to set
this remotely, so `display mode` walks its menus with key presses, which stops
the scan until it is done and then returns the scanner to scanning.

The command needs a scanner, so name one with `--device`.

## Usage

```
radiocli display [flags]
radiocli display mode [color|black|white] [flags]
```

## Parameters

`radiocli display` takes no flags and no arguments of its own.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | - | - | Reports the current mode. |

`radiocli display mode` takes one optional positional argument.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<mode>` | no | report only | The mode to set: `color`, `black`, or `white`. With no mode, reports the current one, exactly as `radiocli display` does. |

The three modes:

| Value | The scanner's own wording | What it draws |
| ----- | ------------------------- | ------------- |
| `color` | `Color Mode` | Each element in the colors set for it under Customize. |
| `black` | `Black w/White Text` | White text on a black background, ignoring those colors. |
| `white` | `White w/Black Text` | Black text on a white background, ignoring those colors. |

The argument is matched case insensitively. Anything else is refused before the
scanner is touched.

### Global flags that change this command

- `--device` names the scanner to use. Get the value from the `port` column of
  [`devices`](devices.md).
- `--output` selects whether the report is printed as lines or as JSON.
- `--pace` sets the gap between key presses during the menu walk, and so
  affects `display mode <mode>` only. It has no effect when reporting, which
  presses no keys.

## Examples

Reporting the current mode:

```
$ radiocli display
display: color
menu:    Color Mode
```

The same report as JSON:

```
$ radiocli display -o json
{
  "mode": "color",
  "color": true,
  "entry": "Color Mode"
}
```

Switching to white text on black:

```
$ radiocli display mode black
display: black
menu:    Black w/White Text
```

Switching back to color:

```
$ radiocli display mode color
display: color
menu:    Color Mode
```

Reporting through the subcommand, which is the same as the bare command:

```
$ radiocli display mode
display: color
menu:    Color Mode
```

A mode that does not exist, refused without touching the scanner:

```
$ radiocli display mode purple
error: "purple" is not a display mode: want color, black, white
```

Deciding in a script whether to use the scanner's colors:

```
$ radiocli display -o json | jq -r 'if .color then "palette" else "mono" end'
palette
```

## Output

The report goes to stdout. Debug logs from `--verbose` go to stderr. Reporting
and setting print the same thing, so setting confirms what the scanner ended up
in rather than what was asked for.

Under `--output text`, stdout holds two lines, with the labels padded so the
values line up:

```
display: color
menu:    Color Mode
```

The `menu` line is the wording on the scanner's own menu, so that what this tool
calls a mode can be matched against what is on the screen. It is omitted, and
the field left empty, if the scanner ever reports a value that is none of the
three.

Under `--output json`, stdout holds one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `mode` | string | The mode: `color`, `black`, or `white`. A value the scanner reports that is none of the three is rendered as `unknown(<n>)`. |
| `color` | boolean | Whether the per-element colors are being drawn. True only in `color` mode. |
| `entry` | string | How the scanner's menu spells the mode, such as `Black w/White Text`. Empty for a mode this tool does not know. |

## What setting it does to the scanner

`display mode <mode>` reads the current mode first. If the scanner is already in
the mode asked for, it prints the report and stops: no menu is opened and the
scan is not interrupted.

Otherwise it opens the top menu, walks to `Display Options`, then to
`Set B/W or Color Mode`, then chooses the entry, selecting each by name off the
screen rather than by counting positions. Choosing the entry leaves the scanner
on `Display Options`, so the command backs out of the menus and returns it to
scanning. It then reads the mode back and fails if the scanner did not end up
where it was sent.

The scanner keeps this setting across a power cycle.

If the command fails part way through, the scanner may be left inside a menu.
Run [`scan`](scan.md) to return it to scanning; every error that can leave it
there says so.

## Where the value comes from

The mode is the `COLOR_MODE` field of the scanner's status response. The
published remote command specification labels that field as belonging to the
waterfall display, which is wrong: it follows this menu setting with the
waterfall never opened. Measured on an SDS150 on firmware 1.00.37, reading `0`,
`1` and `2` for the menu's three entries in order.

It is the only part of the display's appearance the protocol reports. The
per-element colors are not readable or writable over the wire at all.

[`status`](status.md) reports the same value, as its `display` field.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: "<value>" is not a display mode: want color, black, white` | The argument is not one of the three modes. The scanner was not touched. | Pass `color`, `black`, or `white`. |
| `error: accepts at most 1 arg(s), received <n>` | More than one mode was given. | Pass one mode, or none to report. |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: reading the display mode: <detail>` | The scanner did not answer the status request that carries the mode. | Run with `--verbose` to see the raw exchange, and check the cable. |
| `error: opening the top menu: <detail>` | The scanner refused to open its menu, which it does while it is busy with something else on screen. | Check what it is showing with [`screen`](screen.md), and clear it before trying again. |
| `error: looking for "<entry>": <detail>` followed by `Run "radiocli scan" to return the scanner to scanning` | The walk could not find a menu entry. The scanner is left inside the menus. | Run [`scan`](scan.md). Then check the entry is still called that on this firmware with [`screen`](screen.md). |
| `error: the display is still in <mode> mode after setting it to <mode>` | The entry was chosen but the scanner did not change mode. | Read it again with `radiocli display`, and check the scanner is not showing a prompt with [`screen`](screen.md). |
