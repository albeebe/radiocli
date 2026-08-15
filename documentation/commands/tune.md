# tune

Puts the scanner straight onto one frequency and holds it there. Run it to
listen to something immediately, without adding it to the scanner's memory.

## Overview

`tune` is the quickest way to hear a particular frequency. The scanner stops
scanning, jumps to the frequency, and stays there until told otherwise, which is
what its own Quick Search Hold does. Nothing is stored: the scanner's favorites
lists, systems and departments are untouched, and the frequency is forgotten as
soon as you move on.

It reports whether anything is being received the moment it lands. That is a
reading taken at that instant, not a verdict on the frequency; a channel that is
quiet right now reads the same as one nobody uses.

The frequency is read as megahertz unless it carries a unit. It needs a
scanner, so name one with `--device`.

## Usage

```
radiocli tune <frequency> [flags]
```

## Parameters

`tune` takes one argument and no flags of its own.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<frequency>` | Yes | none | The frequency to tune to, in megahertz unless a unit is given. |

### `<frequency>`

The frequency to listen to. A bare number is megahertz, which is how a scanner
labels everything and how frequencies are usually written down. A unit may be
added to remove any doubt, and case does not matter:

| Written as | Means |
| ---------- | ----- |
| `162.550` | 162.550 MHz |
| `162.550MHz` | The same |
| `162550kHz` | The same |
| `162550000Hz` | The same |

The scanner tunes in steps of 100 Hz, because that is the resolution the
protocol carries. A frequency finer than that is rounded to the nearest step and
the change is reported, rather than being silently truncated:

```
$ radiocli tune 155.4751234
155.4751234 rounds to 155.4751 MHz, which is as fine as the scanner tunes.
frequency: 155.4751 MHz
receiving: no
```

Anything that is not a positive number is refused before the scanner is opened,
so a typo costs nothing:

```
$ radiocli tune abc
error: invalid frequency "abc": want a number in megahertz, such as 155.475, or a number with a unit, such as 155475kHz
```

A frequency the scanner cannot receive is refused before anything is sent, with
an explanation. See [What the scanner can
receive](#what-the-scanner-can-receive).

### Global flags that change this command

- `--device` names the scanner to tune. Get the value from the `port` column
  of [`devices`](devices.md).
- `--output` selects whether the result is printed as lines or as JSON.

`--pace` has no effect here. This command presses no keys.

## Examples

Listening to a frequency:

```
$ radiocli tune 162.550
frequency: 162.5500 MHz
receiving: no
```

One that is being received, which also reports the signal strength:

```
$ radiocli tune 107.9
frequency: 107.9000 MHz
receiving: yes
signal:    -105 (3 bars)
```

Returning the scanner to scanning afterwards:

```
$ radiocli scan
The scanner has left quick search and is scanning again.
```

As JSON:

```
$ radiocli tune 107.9 -o json
{
  "megahertz": 107.9,
  "receiving": true,
  "rssi": "-105",
  "bars": "3"
}
```

## Output

The result goes to stdout. The rounding note, the mismatch warning, and any
complaint about the signal reading go to stderr, as do debug logs from
`--verbose`.

Under `--output text`, stdout holds two lines, or three when something is being
received:

```
frequency: 162.5500 MHz
receiving: no
```

| Line | Description |
| ---- | ----------- |
| `frequency` | The frequency the scanner was put on, after rounding. Shown to four decimal places, which is the resolution it tunes and displays. |
| `receiving` | `yes` when something is being heard at that instant, `no` when not. |
| `signal` | The scanner's own signal strength figure, with the number of bars it is showing. |

The strength figure has no documented units, and is reported even in silence as
the noise floor, so `-999` means the scanner heard nothing at all. The bar count
is what `receiving` is decided from, because it is the reading that distinguishes
a signal from the noise.

The scanner does not measure a new frequency instantly. Asked immediately after
being moved it reports nothing on any frequency, so `tune` waits for a reading,
stopping as soon as something is heard. A frequency in use answers in about a
second; a quiet one costs a little over two before being reported as quiet.

Under `--output json`, stdout holds one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `megahertz` | number | The frequency the scanner was put on, in megahertz. |
| `receiving` | boolean | Whether the scanner reported any signal bars. |
| `rssi` | string | The signal strength figure, as the scanner writes it. A string because the scanner reports it that way and its units are undocumented. Absent when the scanner reported no figure. |
| `bars` | string | How many signal bars the scanner is showing. Absent when the scanner reported no count. |

`megahertz` and `receiving` are always present. `rssi` and `bars` are left out
rather than written empty when the scanner says nothing about them, so a reader
checking whether a field is there is asking a real question about the reading.

## What the scanner can receive

The scanner covers five spans, and refuses everything else:

| From | To |
| ---- | -- |
| 25 MHz | 512 MHz |
| 758 MHz | 823.9875 MHz |
| 849.0125 MHz | 868.9875 MHz |
| 894.0125 MHz | 960 MHz |
| 1240 MHz | 1300 MHz |

`tune` checks against these before sending anything, so an unreachable
frequency costs nothing and says why:

```
$ radiocli tune 1030kHz
error: the scanner cannot receive 1.0300 MHz: it covers 25.0000 MHz to 512.0000 MHz, 758.0000 MHz to 823.9875 MHz, ...
```

The two gaps in the middle, 824 to 849 MHz and 869 to 894 MHz, are the cellular
bands, which are blocked in scanners sold in the United States. Those get their
own message, because a frequency sitting between two supported bands otherwise
looks like an oversight:

```
$ radiocli tune 830
error: the scanner cannot receive 830.0000 MHz: the 824.0000 MHz to 849.0000 MHz and 869.0000 MHz to 894.0000 MHz cellular bands are blocked in scanners sold in the United States, and no setting unblocks them
```

These figures were measured rather than taken from a specification: a frequency
either side of every edge was sent to a scanner and its answer recorded. That
matters because the scanner reports a search range of 25 to 1300 MHz, and that
is a setting rather than what the receiver covers. Quoting it would tell you
600 MHz is fine, and it is not.

Some things worth knowing about what falls outside:

- **AM broadcast**, 530 to 1700 kHz, is far below the bottom of the range. So is
  shortwave. This is a VHF and UHF scanner and no setting reaches them.
- **Aircraft**, around 118 to 137 MHz, is inside the first band and is AM, so it
  is reachable.

## Getting back to scanning

Tuning leaves the scanner holding one frequency, so it is no longer scanning
anything in its memory. [`scan`](scan.md) returns it:

```
$ radiocli scan
The scanner has left quick search and is scanning again.
```

This is worth knowing because the state is not a menu. The scanner reports that
it is not in one, so nothing about the menu commands applies, and until `scan`
learned to recognise it the only way back was a key press on the scanner itself.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: invalid frequency "<value>": want a number in megahertz, such as 155.475, or a number with a unit, such as 155475kHz` | The argument is not a number, with or without a unit. | Write it as a number. Nothing was sent. |
| `error: invalid frequency "<value>": it must be greater than zero` | The frequency is zero or negative. | Give a real frequency. Nothing was sent. |
| `error: invalid frequency "<value>": it is smaller than the scanner can tune` | The frequency is positive but rounds to nothing at the resolution the scanner tunes, such as `0.000001hz`. | Give a frequency inside one of the spans listed above. Nothing was sent. |
| `error: the scanner cannot receive <frequency>: it covers ...` | The frequency is outside every band the scanner has. | Use a frequency inside one of the spans listed above. Nothing was sent. |
| `error: the scanner cannot receive <frequency>: the ... cellular bands are blocked ...` | The frequency is in the cellular block. | Nothing can be done; the block is a legal requirement. Nothing was sent. |
| `error: the scanner would not tune to <frequency>: it is on its "<screen>" screen, and refuses a tune while it is in a menu, entering something, or saving` | The scanner is busy with something else. | Run [`scan`](scan.md) to return it to scanning, then try again. |
| `error: the scanner would not tune to <frequency>, and it is not busy with anything else, so the frequency is most likely outside what it can reach. It covers <lower> to <upper>.` | The scanner refused and is on a normal operating screen, so the frequency itself is the likely cause. The final sentence is left off when the scanner does not report its span. | Use a frequency inside the span named, or inside one of the spans listed above. |
| `error: the scanner would not tune to <frequency>, and gave no reason: it refuses a frequency outside the span it covers, and refuses any frequency while it is in a menu, entering something, or saving` | The scanner refused and then would not say what it was doing, so neither cause could be ruled out. | Run [`scan`](scan.md), then try again. Run with `--verbose` to see the raw exchange. |
| `error: tuning to <frequency>: <detail>` | The scanner refused for another reason. | Run with `--verbose` to see the raw exchange. |

The scanner being in a menu is the common case, and is called out separately
because the fix is one command away. Note that `scan` is not run automatically:
leaving a text entry screen discards whatever was being typed there, and that is
not a decision this command should make for you.
