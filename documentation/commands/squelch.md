# squelch

Reports how strong a signal has to be before the scanner plays it, and changes
it. Run it when the scanner is hissing on an empty channel, or when it is
staying silent through transmissions you expected to hear.

## Overview

The scanner keeps a gate on its audio. A signal weaker than the squelch level
is not played, so the speaker stays quiet instead of carrying the hiss of an
empty channel, and a signal stronger than it opens the gate and comes through.
The level runs from 0, which plays everything including the noise, to 15,
which plays only the strongest signals. `squelch` on its own reads that level
and prints it. `squelch set` followed by a level changes it, and prints what
the scanner holds afterwards rather than what it was asked for, so a level the
scanner did not take is visible straight away instead of being reported as a
success. Reading and setting are separate words on purpose: the short form can
only ever read, so checking the squelch can never turn into changing it by
mistake. A level outside 0 to 15 is refused before the scanner is opened,
which means a typo costs nothing and fails the same way whether or not a
scanner is attached. The level is stored on the scanner and stays after the
command exits and after the scanner is unplugged; nothing is written to your
computer. Both forms need a scanner, so name one with `--device`.

## Usage

```
radiocli squelch [flags]
radiocli squelch set <level> [flags]
```

## Parameters

`squelch` takes no flags of its own. `squelch set` takes one argument.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<level>` | Yes | none | The level to set, for `squelch set` only. A whole number from `0` to `15`. |

### `<level>`

The squelch to set, as a whole number from `0` to `15`. `0` opens the gate
completely, so the scanner plays whatever it receives including the noise of an
empty channel. `15` closes it furthest, so only the strongest signals are
played and weaker transmissions are silently dropped. There is no default:
`squelch set` with no level is an error, so that the level being changed is
always written down in the command that changed it.

Anything that is not a whole number in that range is refused before the scanner
is opened, and nothing is sent:

```
$ radiocli squelch set 20
error: squelch level 20 is out of range: want 0 to 15

$ radiocli squelch set open
error: invalid squelch level "open": want a whole number from 0 to 15
```

The bare command takes no arguments. `radiocli squelch 8` is an error, not a
shorthand for `squelch set 8`:

```
$ radiocli squelch 8
error: unknown command "8" for "radiocli squelch"
```

### Global flags that change these commands

- `--device` names the scanner to read or change. Get the value from the
  `port` column of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the level is printed as a line or as JSON. It
  applies to `squelch set` as well as to `squelch`.

`--pace` has no effect here. Neither form presses keys.

## Examples

Reading the current squelch:

```
$ radiocli squelch
squelch: 2 of 15
```

Closing the gate further, to stop a noisy channel opening it:

```
$ radiocli squelch set 5
squelch: 5 of 15
```

Opening the gate completely, so everything received is played:

```
$ radiocli squelch set 0
squelch: 0 of 15
```

Reading the level as JSON:

```
$ radiocli squelch -o json
{
  "level": 2,
  "min": 0,
  "max": 15
}
```

Changing the squelch on a scanner other than the selected one:

```
radiocli --device /dev/cu.usbmodem00000000000011 squelch set 3
```

## Output

The level goes to stdout, from both `squelch` and `squelch set`. The mismatch
warning goes to stderr, as do debug logs from `--verbose`.

Under `--output text`, stdout holds one line, giving the level and the top of
the range so the number means something on its own:

```
squelch: 2 of 15
```

`squelch set` prints the same line, holding the level the scanner reports after
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
| `level` | number | The squelch the scanner is set to, from `0` to `15`. After `squelch set`, the level the scanner reports afterwards. |
| `min` | number | The most open level the scanner accepts. Always `0`. |
| `max` | number | The most closed level the scanner accepts. Always `15` on this model. |

`min` and `max` are reported so a program can show the level against its range,
or check a level before sending it, without holding the bounds of its own.

The squelch level this command reports is not the squelch status reported by
[`status`](status.md). The level is the threshold the scanner is set to, and it
only changes when something changes it. The status says whether the gate is
open at this instant, and changes on its own as transmissions come and go. A
scanner sitting silent on a quiet channel reports a closed gate and a level of
`2` at the same time.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: accepts 1 arg(s), received 0` | `squelch set` was run with no level. | Give a level from `0` to `15`. |
| `error: invalid squelch level "<value>": want a whole number from 0 to 15` | The level is not a whole number. | Pass a plain number, with no decimal point, sign, or unit. |
| `error: squelch level <n> is out of range: want 0 to 15` | The level is a whole number, but outside what the scanner takes. | Use a level from `0` to `15`. |
| `error: unknown command "<value>" for "radiocli squelch"` | A level was passed to the bare command. | Use `squelch set <level>` to change the squelch. |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: opening <port>: no such file or directory (run "radiocli devices" to see what is attached)` | The named port does not exist. | Run [`devices`](devices.md) to find the current port, and pass that. |
| `error: reading the squelch: <detail>` | The scanner answered, but not with a squelch level it could parse. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |
| `error: setting the squelch: <detail>` | The scanner refused the change. It answers a level it cannot take right now the same way it answers one it does not understand. | Check the scanner is not held in a menu, and run with `--verbose` to see the raw exchange. |
| `error: reading the squelch back: <detail>` | The change was sent, but the scanner did not report its level afterwards. | Run `squelch` to see where the level actually ended up. The change may well have taken. |

The three input errors are produced before the scanner is opened, so they
appear whether or not one is attached, and nothing is sent when they do.

A level the scanner did not take is not an error. It is printed with a warning
and exits `0`, because the scanner answered the question it was asked.
