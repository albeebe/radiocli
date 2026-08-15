# beep

Reports the sound the scanner makes when a key is pressed, and changes it. Run
it to silence the keypad without losing the loudness it was set to.

## Overview

The key beep is one setting with seventeen values: `Auto`, which lets the
scanner choose the loudness, `Level 1` to `Level 15` for a fixed one, and `Off`
for silence. This command spells them `auto`, `1` to `15`, and `off`.

**The setting is not in the scanner's remote protocol.** It lives in a menu, so
even reading it walks the scanner into Settings and back out, which takes about
two seconds and stops the scan while it runs. That is worth knowing before this
is put anywhere it runs often. It needs a scanner, so name one with `--device`.

The bare command reads. Changing the scanner is a separate verb, so no reading
can turn into a write by mistake. Every write reads the setting back afterwards
rather than trusting the key press, because the scanner is the authority on what
its own setting is.

[`toggle`](#beep-toggle) is the one worth knowing about. Switching the keypad
off is what destroys the answer to what it was: the scanner holds one value and
`Off` is now that value, so nothing on the radio remembers the level. The
command writes it down instead, which is what lets one button both silence the
keypad and put it back the way it was. See
[What is written down](#what-is-written-down).

## Usage

```
radiocli beep [flags]
radiocli beep set <auto|1-15|off> [flags]
radiocli beep toggle [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<auto\|1-15\|off>` | Yes | none | What to set the key beep to, for `set` only. |

The bare command and `toggle` take no arguments and no flags of their own.

### `<auto|1-15|off>`

Which of the seventeen settings to choose. It is matched whatever case you type
it in, so `AUTO` and `auto` are the same.

| Value | The scanner's own entry | What it does |
| ----- | ----------------------- | ------------ |
| `auto` | `Auto` | The scanner chooses the loudness. |
| `1` to `15` | `Level 1` to `Level 15` | A fixed loudness, `1` quietest. |
| `off` | `Off` | Silence. |

Anything else is refused before the scanner is opened, so a mistyped value costs
nothing and reads the same whether or not one is attached. A near miss is
refused rather than guessed at: there is no undo on the radio, so a `16` read as
`15` would set a loudness nobody asked for.

## Subcommands

### `beep set`

Sets the key beep to one value.

```
$ radiocli beep set 9
key beep: level 9
```

Setting it to what it already is presses nothing and reports the same line, so a
command with nothing to do leaves the menus alone.

**It does not disturb what `toggle` has written down.** The two are separate: a
`set` is you saying what the beep should be now, and the written-down value is
what a toggle is holding for later.

### `beep toggle`

Switches the keypad sound off, and switches it back to whatever it was the next
time it is run.

It reads the setting before changing it, and what it finds decides what it does:

| What it finds | What it does |
| ------------- | ------------ |
| Any setting that makes a sound, including `auto` | Writes that setting down, then sets `off`. |
| `off`, with a setting written down | Sets that setting, and leaves what is written down alone. |
| `off`, with nothing written down | Leaves it `off`. |

```
$ radiocli beep toggle
key beep: off, and level 9 was written down to go back to

$ radiocli beep toggle
key beep: level 9, put back
```

The last row of the table is the case worth stating plainly: **a toggle with
nothing written down leaves the keypad silent**, which is the state whoever
pressed it last asked for. It happens on the first run against a scanner that
was already silent, and if the file described below has been cleared.

```
$ radiocli beep toggle
key beep: off, with nothing written down to go back to
```

## What is written down

The setting a toggle replaces is kept in one JSON file at the platform's cache
location, which on macOS is `~/Library/Caches/radiocli/keybeep.json`, on Linux
`$XDG_CACHE_HOME/radiocli/` or `~/.cache/radiocli/`, and on Windows under
`%LocalAppData%`.

```json
{
  "version": 1,
  "scanners": {
    "serial:0123456789": {
      "model": "SDS150",
      "port": "/dev/cu.usbmodem00000000000011",
      "level": "9",
      "noted": "2026-08-11T11:36:11-04:00"
    }
  }
}
```

Settings are kept per scanner, keyed by USB serial number where there is one and
by port otherwise, because the key beep is that scanner's own setting. Two
radios on one computer remember separately.

**This is the one thing the tool stores that it cannot get back by asking the
scanner.** Everything else it keeps on disk, such as the
[colors cache](colors.md#the-cache), is a copy of something the radio still
knows. It is written where a cache goes all the same, because losing it has a
defined and harmless answer: a toggle with nothing written down leaves the
keypad silent. Deleting the file costs one toggle, not a setting.

The file is only ever written by `toggle`, and only when it switches a sounding
keypad off. It is written after the scanner confirms the change, so a change
that failed cannot leave a note claiming it happened. A file that cannot be
written is a warning rather than a failed command, because the scanner has
already been changed by then and reporting a failure would be a lie.

## Examples

Reading it:

```
$ radiocli beep
key beep: level 9
```

Silencing the keypad and putting it back, which is what the `Toggle Key Beep`
macro does:

```
$ radiocli beep toggle
key beep: off, and level 9 was written down to go back to
$ radiocli beep toggle
key beep: level 9, put back
```

Setting a loudness outright:

```
$ radiocli beep set 4
key beep: level 4
```

Letting the scanner choose:

```
$ radiocli beep set auto
key beep: auto
```

Reading it as JSON, which is the form to build on:

```
$ radiocli beep -o json
{
  "level": "9",
  "on": true
}
```

## Output

### Text

One line, on stdout:

| Line | When |
| ---- | ---- |
| `key beep: level 9` | The setting, for the bare command and `set`. `auto` and `off` read as themselves. |
| `key beep: off, and level 9 was written down to go back to` | A `toggle` that silenced a sounding keypad. |
| `key beep: level 9, put back` | A `toggle` that restored what was written down. |
| `key beep: off, with nothing written down to go back to` | A `toggle` that found the keypad already silent and had nothing to restore. |

### JSON

Under `--output json`, one object:

| Field | Type | Present | Description |
| ----- | ---- | ------- | ----------- |
| `level` | string | Always | The setting: `auto`, `1` to `15`, or `off`. |
| `on` | bool | Always | Whether the keypad makes any sound, which is every setting except `off`. `auto` is a sound. |
| `remembered` | string | Only on a `toggle` that wrote one down | The setting stored for a later toggle to put back. |
| `restored` | bool | Only on a `toggle` that put one back | That this run restored a written-down setting rather than choosing one itself. |

```json
{
  "level": "off",
  "on": false,
  "remembered": "9"
}
```

## Errors

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: "16" is not a key beep setting: want auto, 1 to 15, or off` | The value is not one of the seventeen. The scanner was not touched. | Pass `auto`, a number from `1` to `15`, or `off`. |
| `error: accepts 1 arg(s), received 0` | `set` was given no value. | Pass one of the seventeen. |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: opening the settings menu: <detail>` | The scanner refused to open its menu, which it does while it is busy with something else on screen. | Check what it is showing with [`screen`](screen.md), and clear it before trying again. |
| `error: looking for "Adjust Key Beep": <detail>` followed by `Run "radiocli scan" to return the scanner to scanning` | The walk could not find the entry. The scanner is left inside the menus. | Run [`scan`](scan.md). Then check the entry is still called that on this firmware with [`menu open settings`](menu.md). |
| `error: the key beep setting shows "<row>", which is none of the seventeen values this scanner is known to offer` | The highlighted row is not one of the known values, which a firmware offering something new would look like. | Run [`menu open settings`](menu.md) and press into `Adjust Key Beep` to see what it lists. |
| `error: choosing "Off" for the key beep: <detail>` followed by `Run "radiocli scan" to return the scanner to scanning` | The value could not be selected. The scanner is left inside the menus. | Run [`scan`](scan.md) and try again. |
| `error: the key beep is still level 9 after setting it to off` | The value was chosen and the scanner kept something else. | Run `radiocli beep` to see where it ended up, and set it again. |
