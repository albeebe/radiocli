# headphone

Reports whether the scanner sends the same sound to both sides of its headphone
socket, or one side flipped. Run it when recorded or streamed audio sounds thin,
hollow, or unexpectedly quiet.

## Overview

The headphone socket on this scanner is wired the wrong way round, and this
setting exists to correct it. Uniden fixed the fault in the software the scanner
runs rather than in the socket itself, so a given radio has the problem or does
not depending on which way this has been left. It matters as soon as anything
combines the two sides of the socket into a single signal, which is what happens
whenever the scanner is recorded through a computer: on `invert-phase` the two
sides cancel each other, and what comes out is around eleven decibels quieter
with the body taken out of the voice. It sounds reedy and far away rather than
obviously broken, which is what makes it hard to place. `in-phase` is the setting
to want. The command changes nothing on your computer and writes no files; the
setting is stored on the scanner and stays after the command exits and after the
scanner is unplugged. It needs a scanner, so name one with `--device`.

**This setting is not in the scanner's remote protocol.** It lives in a menu, so
even reading it walks the scanner into Settings and back out, which stops the
scan for a moment and leaves the scanner scanning again afterwards. Nothing else
in the tool reads it automatically for that reason.

**Recording does not depend on this.**
[`audio record`](audio.md#audio-record) measures the two sides and works around
an inverted socket on its own, so you do not have to set this before recording.
Setting it is still worth doing, because it fixes the problem for everything
else the scanner is plugged into as well.

## Usage

```
radiocli headphone [flags]
radiocli headphone set <in-phase|invert-phase> [flags]
```

## Parameters

The bare command takes no arguments and no flags of its own. Its subcommand
takes one positional argument and no flags of its own.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<in-phase\|invert-phase>` | Yes | none | Which way to wire the socket, for `set` only. |

### `<in-phase|invert-phase>`

Exactly one of two values. Any other word is refused before the scanner is
opened, so a typo costs nothing and fails the same way whether or not a scanner
is attached.

| Value | What it does |
| ----- | ------------ |
| `in-phase` | Sends the same sound to both sides. This is the one to want. |
| `invert-phase` | Sends one side flipped, which cancels the sound in anything that combines them. |

```
radiocli --device /dev/cu.usbmodem00000000000011 headphone set in-phase
```

The scanner is read back afterwards rather than the requested value being
echoed, so a setting the scanner did not take is reported as a failure instead
of as a success.

### Global flags that change this command

- `--device` names the scanner. Get the value from the `port` column of
  [`devices`](devices.md).
- `-o`, `--output` selects whether the setting is printed as a line or as JSON.
- `--pace` changes how quickly the keys that walk the menu are sent. The
  default is fine; slow it down only if the scanner is dropping presses.

## Examples

Reading a scanner that has the problem:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 headphone
headphone: invert-phase

The two sides of the jack are inverted, so anything that combines them
into one cancels most of the sound. Run "radiocli headphone set in-phase"
to correct it.
```

Correcting it:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 headphone set in-phase
headphone: in-phase
```

Reading it back, as JSON, which is the form to read from a script:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 headphone -o json
{
  "phase": "in-phase"
}
```

Setting it to the value it is already on, which is not an error and walks no
further than it has to:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 headphone set in-phase
headphone: in-phase
```

## Output

The setting goes to stdout. The advice about an inverted socket goes to stderr,
so `2>/dev/null` leaves stdout holding only the setting.

Under `--output text` the command prints one line, `headphone:` followed by the
value. When the value is `invert-phase` it also prints the note above to stderr,
naming the command that corrects it.

Under `--output json` the command prints one object.

| Field | Type | Description |
| ----- | ---- | ----------- |
| `phase` | string | The setting the scanner is on: `in-phase` or `invert-phase`. Always present. |

`headphone set` prints exactly what `headphone` prints, reporting the setting
the scanner holds after the change rather than the one that was asked for.

## Errors

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named` | No scanner was given. | Pass `--device` with the port from [`devices`](devices.md). |
| `error: there is no headphone setting called "..."` | `set` was given something other than the two values. | Pass `in-phase` or `invert-phase`. |
| `error: opening the settings menu: ...` | The scanner refused to open Settings, usually because it is busy with something else. | Run [`scan`](scan.md) to put it back to scanning, then try again. |
| `error: looking for "Headphone L/R output": ...` | The setting is not in this scanner's menu. Firmware old enough to predate the fix does not have it. | Update the scanner's firmware, or use [`audio record`](audio.md#audio-record), which works around an inverted socket without the setting. |
| `error: the headphone setting shows "...", which is neither of the two values this scanner is known to offer` | The menu showed something this command does not recognise. | Report it as an issue, quoting the value. |
| `error: choosing "..." for the headphone output: ...` | The scanner would not take the value. | Run [`scan`](scan.md) to put it back to scanning, then try again. |
| `error: the headphone output is still ... after setting it to ...` | The press was accepted and the setting did not change. | Try again. If it persists, change it on the radio by hand. |
| `error: the scanner could not leave the menus` | The scanner is stuck in a menu. | Run [`scan`](scan.md), or press the AVOID key on the radio. |
| `error: <port> is in use by another radiocli` | Another invocation has the scanner. | Wait for it, or pass `--wait` to queue behind it. |
