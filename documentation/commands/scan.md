# scan

Takes the scanner out of whatever menu it has been left in and returns it to
its operating screen. Run it when a command has left the scanner somewhere and
you want it listening again.

## Overview

A scanner can stop scanning your favorites lists in five ways: it can be inside
a menu, holding a single frequency, holding a single channel, sweeping a range
of its own in a custom search, or sitting on a weather channel. Getting back
from any of them is not one reliable command.

The protocol's own way out of the menus is refused on several screens, including
the text entry screens and the favorites list menu. The key that always works is
the AVOID key, but outside the menus that same key avoids the current channel,
so a command that pressed it hopefully would change something nobody asked for.
Holding a frequency is not a menu at all, and needs a different key again.
Holding a channel is neither, and is released by a different means again. A
custom search is not stopped at all: it is moving, and it keeps moving.

`scan` handles all of it. It tries the protocol's way out first, falls back to
the key when that is refused, checks where the scanner is **before every press
and never after**, and then returns it from quick search, from a hold, or from
a custom search if that is where it ended up. On a scanner that is already
scanning it presses nothing at all, which makes it safe to run whenever you are
unsure. It needs a scanner, so name one with `--device`.

**There is a fifth place, and it is the one nothing else puts right: the
weather channels.** A scanner left there by [`weather`](weather.md) is out of
the menus and is parked on a weather channel, and that hold is deliberately
left alone by every other command in the tool, so that changing the volume does
not end the weather monitoring somebody asked for. `scan` is one of the two
commands that does leave it; `radiocli weather stop` is the other, and they do
the same thing.

## Usage

```
radiocli scan [flags]
```

## Parameters

`scan` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | - | - | This command takes no flags and no arguments. |

### Global flags that change this command

- `--device` names the scanner to return. Get the value from the `port` column
  of [`devices`](devices.md).
- `--pace` sets the gap between key presses, for the screens that need more
  than one.

`--output` has no effect. There is no result to format; the outcome is a note
on stderr.

## Examples

Returning a scanner left holding a frequency by [`tune`](tune.md):

```
$ radiocli scan
The scanner is already out of the menus.
The scanner has left quick search and is scanning again.
```

Returning a scanner someone left holding a channel, by turning its knob:

```
$ radiocli scan
The scanner is already out of the menus.
The scanner is holding PUBLIC SAFETY / POLICE DEPARTMENT.
The scanner has been released and is scanning again.
```

Returning a scanner left in a menu:

```
$ radiocli scan
The scanner has left the menus, after one key press.
```

Returning a scanner left sweeping a custom search by
[`banks scan`](banks.md):

```
$ radiocli scan
The scanner is already out of the menus.
The scanner has left Custom Search and is scanning again.
```

Returning a scanner left on the weather channels by [`weather`](weather.md):

```
$ radiocli scan
The scanner is already out of the menus.
The scanner has left the weather channels and is scanning again.
```

Running it on a scanner that is already scanning, which does nothing:

```
$ radiocli scan
The scanner is already out of the menus.
```

Finishing a session that opened a menu:

```
radiocli systems goto "GREENDALE ST"
radiocli screen
radiocli scan
```

## Output

`scan` writes nothing to stdout. The outcome goes to stderr, as do debug logs
from `--verbose`.

The notes that can be printed, in the order they appear:

| Note | Meaning |
| ---- | ------- |
| `The scanner is already out of the menus.` | It was not in a menu. It may still have been holding, in which case further lines follow. |
| `The scanner has left quick search and is scanning again.` | It was holding one frequency, as [`tune`](tune.md) leaves it, and has been returned to scanning. |
| `The scanner is holding <what>.` | It was parked on a channel. `<what>` is as much of the system, department and channel as the scanner reported. |
| `The scanner has been released and is scanning again.` | The hold above has been cleared. |
| `The scanner has left <mode> and is scanning again.` | It was sweeping something of its own, such as `Custom Search`, and has been sent back to the favorites lists. |
| `The scanner has left the menus.` | The protocol's own way out worked first time. |
| `The scanner has left the menus, after <n> key presses.` | The protocol's way out was refused, and the AVOID key was pressed until the scanner was out. |

The press count is worth reading. One press is ordinary. Several means the
scanner was several menus deep, which is a hint that a previous command left it
somewhere unexpected.

## Why it checks before every press

The AVOID key does two different things depending on where the scanner is.
Inside the menus it leaves them, which is what this command wants. Outside the
menus it avoids whatever the scanner is currently on, which stops that channel
being scanned until it is restored.

A command that pressed the key and then checked whether it had worked would,
on a scanner that was already scanning, avoid a channel and then report
success. So the check comes first every time, and the already-scanning case
presses nothing.

This is also why `scan` is safe to run repeatedly, and why it is the right
thing to put at the end of a script that opens menus.

## Quick search is not a menu

[`tune`](tune.md) leaves the scanner holding one frequency, and that state is
not a menu: the scanner answers that it is not in one, so every menu command
declines to act and every escape key is held back. It is also not scanning, so
reporting that there was nothing to do would be wrong.

`scan` recognises it and presses the key the scanner labels `to Scan`, then
waits for the screen to change before saying so. That is why the command can
report leaving the menus and leaving quick search in the same run, or report
only the second.

## Holding a channel does not look like anything

This is the case worth knowing about, because nothing on the screen announces
it.

Turning the knob on a scanning scanner puts it on hold, parked on one channel.
Afterwards it is out of the menus, answers every command correctly, and shows a
channel name where it would otherwise say `Scanning...`. But a scanner that is
scanning normally shows a channel name too, every time it stops on one it is
receiving. The two are indistinguishable from a single look at the screen.

The only thing that tells them apart is the mode the scanner reports, which is
`Scan Hold` rather than `Scan Mode`. `scan` reads it, and if the scanner is
holding, sends the protocol's jump to scanning mode, which releases it whatever
it was parked on. It then waits for the scanner to agree it is no longer
holding before saying so.

Every hold the scanner has works this way, not just `Scan Hold`: `Trunk Scan
Hold`, `Custom Search Hold`, `Service Scan Hold` and the rest are all matched,
and all released the same way.

## A custom search is not stopped, it is somewhere else

The other four cases are all a scanner that has stopped. A custom search is a
scanner that is working hard, sweeping between a lower and an upper limit,
looking exactly as busy as a scanning one. It simply is not sweeping your
favorites lists, and it will not go back to them on its own.

That makes it the one case a person is most likely to be surprised by, because
the screen shows a frequency changing and nothing about it says "you are not
scanning what you think you are scanning". Only the mode does, and it reads
`Custom Search` rather than `Scan Mode`.

[`banks scan`](banks.md) is what puts the scanner there, and `scan` is what
brings it back. The same applies to a service search and to close call: each
sweeps a list of its own and stays until told otherwise, and `scan` tells them
otherwise the same way, with the protocol's jump to scanning mode.

The check is a positive one: the scanner is scanning when it reports `Scan
Mode` or `Trunk Scan`, and anything else non-empty is a mode to leave. Naming
the modes to leave instead would mean a firmware that adds one silently stops
being handled.

Nothing in this tool holds the scanner deliberately. Holds turn up because
someone turned the knob, pressed one of the three soft keys along the bottom of
the screen, or ran a command that walks the scanner's memory by turning the knob
for you.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: asking the scanner where it is: <detail>` | The scanner would not say whether it was in a menu. | Run with `--verbose` to see the raw exchange. |
| `error: leaving the menus: <detail>` | The scanner refused the key press. | Press AVOID on the scanner itself. |
| `error: could not leave the menus after 8 attempts: the scanner is on the "<name>" screen, and needs a key press on the scanner itself` | Eight presses did not get it out. | The named screen holds the scanner in a way this command cannot break; press a key on the scanner. |
| `error: the scanner is still holding one frequency in quick search: press the key labelled "to Scan" on its screen` | The key to leave quick search was sent and the scanner stayed there. | Press the soft key labelled `to Scan` on the scanner. |
| `error: returning the scanner to scanning from <mode>: <detail>` | The scanner refused the command that releases a hold. | Run with `--verbose` to see the raw exchange, and press the scanner's own scan key. |
| `error: the scanner is still holding, in <mode>: press its scan key, or turn the knob, to release it` | The release was sent and the scanner stayed on hold. | Turn the scanner's knob, or press the key that resumes scanning. |
| `error: the scanner is still in <mode>: press its scan key` | The jump back to scanning was sent and the scanner stayed in its own search, such as `Custom Search`. | Press the scanner's own scan key. |
| `error: returning the scanner to scanning from the weather channels: <detail>` | The scanner refused the command that leaves the weather channels. | Run with `--verbose` to see the raw exchange, and press the soft key labelled `to Scan` on the scanner. |
| `error: the scanner is still on the weather channels: press the key labelled "to Scan" on its screen` | The command that leaves them was sent and the scanner stayed. | Press the soft key labelled `to Scan` on the scanner. |
