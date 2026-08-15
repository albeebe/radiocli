# volume

Reports how loud the scanner is set to play, and changes it. Run it to check
the volume without reaching for the scanner, or to turn it down from the
computer.

## Overview

The scanner holds a volume level from 0, which is silent, to 15, which is
loudest. `volume` on its own reads that level and prints it. `volume set`
followed by a level changes it, and prints what the scanner holds afterwards
rather than what it was asked for, so a level the scanner did not take is
visible straight away instead of being reported as a success. Reading and
setting are separate words on purpose: the short form can only ever read, so
checking the volume can never turn into changing it by mistake. A level
outside 0 to 15 is refused before the scanner is opened, which means a typo
costs nothing and fails the same way whether or not a scanner is attached.
Both forms need a scanner, so name one with `--device`.

## Usage

```
radiocli volume [flags]
radiocli volume set <level> [flags]
```

## Parameters

`volume` takes no flags of its own. `volume set` takes one argument.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<level>` | Yes | none | The level to set, for `volume set` only. A whole number from `0` to `15`. |

### `<level>`

The volume to set, as a whole number from `0` to `15`. `0` is silent and `15`
is loudest. There is no default: `volume set` with no level is an error, so
that the level being changed is always written down in the command that changed
it.

Anything that is not a whole number in that range is refused before the scanner
is opened, and nothing is sent:

```
$ radiocli volume set 20
error: volume level 20 is out of range: want 0 to 15

$ radiocli volume set loud
error: invalid volume level "loud": want a whole number from 0 to 15
```

The bare command takes no arguments. `radiocli volume 8` is an error, not a
shorthand for `volume set 8`:

```
$ radiocli volume 8
error: unknown command "8" for "radiocli volume"
```

### Global flags that change these commands

- `--device` names the scanner to read or change. Get the value from the
  `port` column of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the level is printed as a line or as JSON. It
  applies to `volume set` as well as to `volume`.

`--pace` has no effect here. Neither form presses keys.

## Examples

Reading the current volume:

```
$ radiocli volume
volume: 8 of 15
```

Turning the volume down:

```
$ radiocli volume set 3
volume: 3 of 15
```

Silencing the scanner:

```
$ radiocli volume set 0
volume: 0 of 15
```

Reading the level as JSON:

```
$ radiocli volume -o json
{
  "level": 8,
  "min": 0,
  "max": 15
}
```

Changing the volume on a scanner other than the selected one:

```
radiocli --device /dev/cu.usbmodem00000000000011 volume set 5
```

## Output

The level goes to stdout, from both `volume` and `volume set`. The mismatch
warning goes to stderr, as do debug logs from `--verbose`.

Under `--output text`, stdout holds one line, giving the level and the top of
the range so the number means something on its own:

```
volume: 8 of 15
```

`volume set` prints the same line, holding the level the scanner reports after
the change rather than the one it was asked for. When the two differ, a warning
follows on stderr and the command still exits `0`, because the scanner answered
and its answer is what was printed:

```
The scanner is at 12 rather than the 15 that was asked for.
```

Under `--output json`, stdout holds one object, the same from both forms. All
three fields are always present:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `level` | number | The volume the scanner is set to, from `0` to `15`. After `volume set`, the level the scanner reports afterwards. |
| `min` | number | The quietest level the scanner accepts. Always `0`. |
| `max` | number | The loudest level the scanner accepts. Always `15` on this model. |

`min` and `max` are reported so a program can show the level against its range,
or check a level before sending it, without holding the bounds of its own.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: accepts 1 arg(s), received 0` | `volume set` was run with no level. | Give a level from `0` to `15`. |
| `error: invalid volume level "<value>": want a whole number from 0 to 15` | The level is not a whole number. | Pass a plain number, with no decimal point, sign, or unit. |
| `error: volume level <n> is out of range: want 0 to 15` | The level is a whole number, but outside what the scanner takes. | Use a level from `0` to `15`. |
| `error: unknown command "<value>" for "radiocli volume"` | A level was passed to the bare command. | Use `volume set <level>` to change the volume. |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: opening <port>: no such file or directory (run "radiocli devices" to see what is attached)` | The named port does not exist. | Run [`devices`](devices.md) to find the current port, and pass that. |
| `error: reading the volume: <detail>` | The scanner answered, but not with a volume level it could parse. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |
| `error: setting the volume: <detail>` | The scanner refused the change. It answers a level it cannot take right now the same way it answers one it does not understand. | Check the scanner is not held in a menu, and run with `--verbose` to see the raw exchange. |
| `error: reading the volume back: <detail>` | The change was sent, but the scanner did not report its level afterwards. | Run `volume` to see where the level actually ended up. The change may well have taken. |

The three input errors are produced before the scanner is opened, so they
appear whether or not one is attached, and nothing is sent when they do.

A level the scanner did not take is not an error. It is printed with a warning
and exits `0`, because the scanner answered the question it was asked.
