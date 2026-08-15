# weather

Puts the scanner on the NOAA weather channels, measures all seven, and holds
the one with the strongest signal. Run it when you want the forecast rather
than whatever the scanner was listening to.

## Overview

The scanner has seven weather channels carrying the continuous NOAA Weather
Radio broadcast. `weather` switches the scanner onto them and then measures
every one of them itself: it parks the scanner so it stops moving, steps
through all seven reading the signal on each, and goes back to the best one.
That takes a few seconds, and it is the point of the command.

The measuring is there because the scanner left to itself is unreliable about
it. Its own weather scan stops on the first channel that opens its squelch,
which is not the strongest and is sometimes not receivable at all: it was seen
parked on channel 2 at `-108 dBm` and silent, with channel 7 audible at
`-97 dBm` the whole time. So the command reports what it found on all seven
alongside the one it chose, and the choice can be checked rather than trusted.

The seven channels, in the order the scanner steps through them, are:

| Channel | Frequency |
| ------- | --------- |
| `1` | `162.550000MHz` |
| `2` | `162.400000MHz` |
| `3` | `162.475000MHz` |
| `4` | `162.425000MHz` |
| `5` | `162.450000MHz` |
| `6` | `162.500000MHz` |
| `7` | `162.525000MHz` |

The scanner does a second thing on those same channels, called weather alert,
which sits on a channel silent and unmutes only when an alert tone arrives.
Both report themselves as `WX Scan`, and on screen the only difference is one
word. `weather` starts the one that plays, and fails rather than reporting
success if the scanner starts the silent one instead.

Starting the broadcast means walking the scanner's menus, so it **stops the
scan** on the way. It does not put the scanner back to scanning afterwards:
staying on the weather channels is the point. Nothing is written to your
computer and no setting on the scanner is changed. Two commands bring it back:
`radiocli weather stop` and [`scan`](scan.md), which do the same thing. No
other command does, on purpose: the scanner reports itself as holding once it
is parked on a channel, and every command that walks a menu tidies up by
releasing a hold, so weather is made the exception. Changing the volume or the
backlight leaves the broadcast playing.

It needs a scanner, so name one with `--device`.

## Usage

```
radiocli weather [flags]
radiocli weather stop [flags]
```

## Parameters

`weather` and `weather stop` have no flags of their own. Their behaviour is
controlled entirely by the [global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `-h`, `--help` | No | none | Print the command's help and exit. |

Three global flags change what these commands do:

- `--device` names the port to use. Get the value from the `port` column of
  [`devices`](devices.md).
- `-o`, `--output` selects `text` or `json`. The default is `text`.
- `--wait` queues behind another `radiocli` that is using the scanner instead
  of failing at once.

### `stop`

Takes the scanner off the weather channels and returns it to scanning whatever
it was scanning before. It reads what the scanner is doing first, so running it
on a scanner that is not on the weather channels prints a note and changes
nothing. It opens no menus in either case. [`scan`](scan.md) does the same
thing.

```
radiocli weather stop
```

## Examples

Starting the broadcast:

```
$ radiocli weather
Measuring all 7 weather channels.
weather:   Monitor Weather
channel:   7
frequency: 162.525000MHz
signal:    -97 dBm

CHANNEL  FREQUENCY      SIGNAL
1        162.550000MHz  -
2        162.400000MHz  -
3        162.475000MHz  -112 dBm
4        162.425000MHz  -
5        162.450000MHz  -104 dBm
6        162.500000MHz  -
7        162.525000MHz  -97 dBm   holding
```

Six seconds, and the scanner is parked on channel 7 rather than on channel 3 or
5, which are audible but weak. `-` means the channel answered with nothing at
all, which is not the same as a weak reading. The first line goes to stderr.

Starting it as JSON:

```
$ radiocli -o json weather
{
  "scanning": true,
  "mode": "Monitor Weather",
  "receiving": true,
  "channel": "7",
  "frequency": "162.525000MHz",
  "signal": -97,
  "channels": [
    {
      "number": "1",
      "frequency": "162.550000MHz",
      "selected": false
    },
    {
      "number": "2",
      "frequency": "162.400000MHz",
      "selected": false
    },
    {
      "number": "3",
      "frequency": "162.475000MHz",
      "signal": -111,
      "selected": false
    },
    {
      "number": "4",
      "frequency": "162.425000MHz",
      "selected": false
    },
    {
      "number": "5",
      "frequency": "162.450000MHz",
      "signal": -104,
      "selected": false
    },
    {
      "number": "6",
      "frequency": "162.500000MHz",
      "selected": false
    },
    {
      "number": "7",
      "frequency": "162.525000MHz",
      "signal": -97,
      "selected": true
    }
  ]
}
```

A channel heard nothing on has no `signal` key at all, rather than a zero or a
`-999`. The two runs above measured the same radio moments apart and disagree
about channels 2, 3 and 4, which is what a weak channel does.

Running `weather` again measures all seven again and parks it again, which is
the same thing rather than a toggle.

Coming back:

```
$ radiocli weather stop
The scanner has left the weather channels and is scanning again.
weather: off
```

The first line goes to stderr and the second to stdout.

Coming back when the scanner is not there:

```
$ radiocli weather stop
The scanner is not on the weather channels.
weather: off
```

Nothing was pressed and no menu was opened.

Coming back with [`scan`](scan.md), which does the same thing:

```
$ radiocli scan
The scanner is already out of the menus.
The scanner has left the weather channels and is scanning again.
```

Stopping as JSON:

```
$ radiocli -o json weather stop
The scanner has left the weather channels and is scanning again.
{
  "scanning": false,
  "receiving": false
}
```

The note is on stderr, so a program reading stdout sees only the document.

## Output

Results go to stdout. Notes and progress go to stderr, so they survive being
piped somewhere without contaminating what is piped.
`Measuring all 7 weather channels.` is written to stderr before the sweep
starts, because the sweep takes several seconds.

Under `--output text`, `weather` prints four lines and then the sweep. `signal`
is in dBm, and larger, meaning closer to zero, is stronger:

```
weather:   Monitor Weather
channel:   7
frequency: 162.525000MHz
signal:    -97 dBm

CHANNEL  FREQUENCY      SIGNAL
1        162.550000MHz  -
2        162.400000MHz  -
3        162.475000MHz  -112 dBm
4        162.425000MHz  -
5        162.450000MHz  -104 dBm
6        162.500000MHz  -
7        162.525000MHz  -97 dBm   holding
```

The fourth column reads `holding` on the one channel the scanner was left on,
and is empty on the rest. A `-` in the `SIGNAL` column means the scanner
reported nothing on that channel throughout the time it sat there.

When none of the seven can be heard, `channel:` reads `none receivable`, the
`frequency:` and `signal:` lines are left out, no row is marked `holding`, and
`None of the 7 weather channels can be heard from here.` goes to stderr. The
scanner is left on the weather channels with the hold released, so it carries on
looking on its own. That case was not reproducible on the scanner these examples
were run against, which hears channel 7, so those two lines are quoted from the
source rather than pasted from a terminal.

Under `--output text`, a scanner that is not on the weather channels prints one
line, and this is the whole of what `weather stop` prints on stdout:

```
weather: off
```

Under `--output json`, both commands print one object:

| Field | Type | Present | Meaning |
| ----- | ---- | ------- | ------- |
| `scanning` | boolean | Always | Whether the scanner is on the weather channels. |
| `mode` | string | Only when `scanning` is `true` | The scanner's name for what it is doing there. `weather` only ever reports `Monitor Weather`, because it fails rather than report the other one. |
| `receiving` | boolean | Always | Whether the scanner was parked on a channel it can hear. |
| `channel` | string | Only when `receiving` is `true` | The channel number as the scanner's screen labels it, such as `7`. |
| `frequency` | string | Only when `receiving` is `true` | What that channel is tuned to, carrying its own unit, such as `162.525000MHz`. |
| `signal` | number | Only when `receiving` is `true` | The strength of that channel in dBm, always negative. Larger is stronger. |
| `channels` | array | Only for `weather` | Every weather channel and what was heard on it, in channel order. `weather stop` measures nothing and leaves this out. |
| `channels[].number` | string | Always within `channels` | The channel number, `1` through `7`. |
| `channels[].frequency` | string | Always within `channels` | What that channel is tuned to. |
| `channels[].signal` | number | Only when something was heard | The strongest reading taken on that channel, in dBm. **Absent, not zero,** when the scanner heard nothing there. |
| `channels[].selected` | boolean | Always within `channels` | Marks the channel the scanner was left on. Exactly one is `true` when `receiving` is `true`, and none is when it is `false`. |

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: unknown command "<name>" for "radiocli weather"` | `weather` takes no arguments and has one subcommand, `stop`. | Run `radiocli weather` or `radiocli weather stop`. |
| `error: opening the weather menu: <detail>` | The scanner refused to open its WX Operation menu. | Run with `--verbose` to see the raw exchange. |
| `error: choosing "Weather Scan": <detail>` | The scanner opened the menu but the entry could not be reached. The message names the entries it saw instead, and is followed by `Run "radiocli scan" to return the scanner to scanning`. | Run [`scan`](scan.md) to get the scanner out of the menu it was left in. |
| `error: the scanner started "Weather Alert" rather than "Monitor Weather": it is sitting on a weather channel silent, and will unmute only for an alert tone` | The menu walk landed on the alert standby instead of the broadcast. | Run `radiocli weather stop`, then run `radiocli weather` again. |
| `error: the scanner reports "<name>" on the weather channels, which is neither "Monitor Weather" nor "Weather Alert"` | The scanner named a weather mode this tool does not know. | Run `radiocli screen` to see what it is showing, and `radiocli weather stop` to leave. |
| `error: asking whether the scanner is holding a weather channel: <detail>` | The scanner would not say whether it was parked, so nothing was pressed. | Run with `--verbose` to see the raw exchange. |
| `error: pressing the hold key: <detail>` | The scanner refused the key that parks it on one channel. | Run with `--verbose` to see the raw exchange. |
| `error: the scanner is <holding\|not holding> a weather channel after pressing hold, wanted the other: <detail>` | The hold key was pressed and the scanner did not change. | Press the soft key labelled `HOLD` or `RESUME` on the scanner itself, then run the command again. |
| `error: reading a weather channel: <detail>` | The scanner stopped answering partway through the sweep. | Run with `--verbose` to see the raw exchange. |
| `error: the scanner would not say which weather channel it is on` | The scanner answered throughout the sweep but never named a channel. | Run `radiocli screen` to see what it is showing, and `radiocli weather stop` to leave. |
| `error: turning the knob to the next weather channel: <detail>` | The scanner refused the key that steps channels. | Run with `--verbose` to see the raw exchange. |
| `error: asking which weather channel the scanner is on: <detail>` | The scanner stopped answering while the command was going back to the channel it chose. | Run with `--verbose` to see the raw exchange. |
| `error: could not get back to weather channel <number>: the scanner stopped on <list> instead` | The sweep measured every channel but stepping back to the winner did not reach it. | Run the command again. |
| `error: the scanner is still in "<mode>" after 2s` | The scanner did not reach the state it was asked for. For `weather` that means the broadcast never started; for `weather stop` it means the scanner stayed on the weather channels. | Press the soft key labelled `to Scan` on the scanner itself. |
| `error: asking the scanner what it is doing: <detail>` | `weather stop` could not read what the scanner was doing, so it pressed nothing. | Run with `--verbose` to see the raw exchange. |
| `error: returning the scanner to scanning: <detail>` | The scanner refused the command that leaves the weather channels. | Press the soft key labelled `to Scan` on the scanner itself. |
