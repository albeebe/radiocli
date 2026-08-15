# clock

Reports the date and time the scanner itself is keeping, and corrects them.
Run it when a timestamp on the scanner looks wrong, and follow it with `sync`
to put the scanner back in step with this computer.

## Overview

The scanner keeps its own clock, powered by the scanner rather than by the
computer, and it is what stamps anything the scanner records. That clock drifts
on its own schedule, and a scanner left long enough without power loses the time
entirely and starts again from a date that means nothing. `clock` on its own
reads what the scanner holds and prints it: the date, the time of day, whether
daylight saving is being applied, and whether the clock is still running. It
also says how far the scanner has drifted from this computer, because a gap of
an hour or a day is what the reader is usually looking for and is easy to miss
in a row of digits.

Two subcommands correct it. `clock sync` copies this computer's current time
onto the scanner, which is the usual repair. `clock set` takes a date and time
of your own, for a scanner that should read something other than what this
computer says, and accepts a date or a time on its own to change just that
half while leaving the other exactly as it was. Both print what the scanner
holds afterwards rather than what it was asked for, so a value the scanner did
not take is visible straight away instead of being reported as a success.
Reading and writing are separate words on purpose: the short form can only
ever read, so checking the clock can never turn into changing it by mistake.
All three forms need a scanner, so name one with `--device`.

## Usage

```
radiocli clock [flags]
radiocli clock sync [flags]
radiocli clock set <datetime> [flags]
```

## Parameters

`clock` takes no flags of its own. `clock sync` and `clock set` share one flag,
and `clock set` takes one argument.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<datetime>` | Yes | none | The date and time to set, for `clock set` only. |
| `--dst` | No | whatever this computer applies to that date | Tell the scanner daylight saving is in force. Accepted by `clock set` and `clock sync`. |

### `<datetime>`

The date and time to put on the scanner, read in this computer's time zone.
There is no default: `clock set` with no value is an error, because a command
that changes the clock should say what it changed it to. To set the scanner
from this computer's own clock, use `clock sync` rather than typing the current
time out.

Five forms are accepted, tried in this order:

| Written as | Means |
| ---------- | ----- |
| `2026-08-02 14:30:00` | That date and time, to the second. |
| `2026-08-02T14:30:00` | The same, with a `T` in place of the space. |
| `2026-08-02 14:30` | That date and time, on the minute. |
| `2026-08-02` | That date, keeping the time of day the scanner already shows. |
| `14:30` or `14:30:00` | That time of day, keeping the date the scanner already shows. |

**A half you leave out is kept, not replaced.** Giving a date alone changes only
the date, and giving a time alone changes only the time of day. The half that
was not written is read off the scanner and sent back unchanged, so correcting
the day never disturbs the time and correcting the time never disturbs the day.
Both partial forms therefore read the scanner before they write to it.

Anything else is refused before the scanner is opened, and nothing is sent:

```
$ radiocli clock set "half past two"
error: invalid date and time "half past two": want "2026-08-02 14:30:00", a date alone such as "2026-08-02", or a time alone such as "14:30"
```

When the scanner's clock has stopped, there is no half worth keeping. The
missing half is taken from this computer instead, and a note on stderr says so
rather than letting a reading the scanner has already disowned be copied back
silently:

```
$ radiocli clock set 2026-08-02
The scanner's clock is not running, so there is no time of day to keep.
Taking it from this computer instead.

date:     2026-08-02
time:     14:57:29
daylight: yes
clock:    running
```

### `--dst`

Whether to tell the scanner that daylight saving is in force. Left alone, the
answer is taken from this computer, which already knows whether its own zone
applies daylight saving on the date being set, so the scanner ends up agreeing
with the computer without anything being typed. The date it is judged against
is the one the scanner ends up on, so moving a scanner from June to December
with a date alone turns daylight saving off even though the value written
mentioned nothing about it.

Pass `--dst` to assert it, or `--dst=false` to deny it, when the scanner should
disagree with this computer. The flag is only consulted when it is actually
typed, so its `false` default can never quietly turn daylight saving off on a
scanner that had it on.

The bare command takes no arguments or flags of its own. `radiocli clock 14:30`
is an error, not a shorthand for `clock set`:

```
$ radiocli clock 14:30
error: unknown command "14:30" for "radiocli clock"
```

### Global flags that change these commands

- `--device` names the scanner to read or change. Get the value from the
  `port` column of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the reading is printed as lines or as JSON. It
  applies to `set` and `sync` as well as to the bare command.

`--pace` has no effect here. None of these forms presses keys.

## Examples

Reading a scanner that has drifted:

```
$ radiocli clock
date:     2026-08-02
time:     15:51:10
daylight: yes
clock:    running

The scanner is 59 minutes from this computer's clock. Run "radiocli clock sync"
to correct it.
```

Correcting it from this computer:

```
$ radiocli clock sync
date:     2026-08-02
time:     14:52:03
daylight: yes
clock:    running
```

Setting a date and time by hand:

```
$ radiocli clock set "2026-08-02 14:30:00"
date:     2026-08-02
time:     14:30:00
daylight: yes
clock:    running
```

Correcting only the time of day, on a scanner whose date is already right:

```
$ radiocli clock set 14:30
date:     2026-06-13
time:     14:30:00
daylight: yes
clock:    running
```

Correcting only the date, keeping the time of day the scanner is showing:

```
$ radiocli clock set 2026-03-20
date:     2026-03-20
time:     14:30:06
daylight: yes
clock:    running
```

Setting a scanner that should not apply daylight saving:

```
radiocli clock sync --dst=false
```

Reading the clock as JSON:

```
$ radiocli clock -o json
{
  "date": "2026-08-02",
  "time": "14:52:03",
  "daylightSaving": true,
  "valid": true
}
```

Correcting a scanner other than the selected one:

```
radiocli --device /dev/cu.usbmodem00000000000011 clock sync
```

## Output

The reading goes to stdout, from all three forms. The drift note, the
stopped-clock warning, and the mismatch warning go to stderr, as do debug logs
from `--verbose`.

Under `--output text`, stdout holds four lines, with the labels padded so the
values line up:

```
date:     2026-08-02
time:     14:52:03
daylight: yes
clock:    running
```

The `date` and `time` lines are the digits the scanner reports, printed as
given. They are not converted into this computer's time zone, because the
scanner reports a wall clock reading rather than a moment in time, and it does
not say which zone it believes it is in. For the same reason, a value passed to
`clock set` is read in this computer's zone and sent as written.

The `daylight` line is `yes` or `no`. The `clock` line is `running`, or
`not running` when the scanner's real time clock has stopped, which happens
after the scanner has been without power long enough.

`sync` and `set` print the same four lines, holding what the scanner reports
after the change rather than what it was asked for.

When the two clocks are a minute or more apart, a note follows on stderr,
rounded to whichever of days, hours, or minutes says the most:

```
The scanner is 59 minutes from this computer's clock. Run "radiocli clock sync"
to correct it.
```

When the scanner's clock has stopped, that warning replaces the drift note,
because the digits it is drifting from mean nothing:

```
The scanner reports its clock is not running, so the date and time above
are unreliable. Run "radiocli clock sync" once the scanner has held power
for a while.
```

When the scanner ends up more than three seconds from what `set` or `sync`
asked for, a further warning follows on stderr and the command still exits `0`,
because the scanner answered and its answer is what was printed:

```
The scanner is at 2026-08-02 14:29:00, not the 2026-08-02 14:30:00 it was asked for.
```

Three seconds is the allowance rather than an exact match, because writing the
clock and reading it back takes long enough to cross a second boundary, and the
scanner keeps no resolution finer than a second.

Under `--output json`, stdout holds one object, the same from all three forms.
All four fields are always present, and no drift is reported, since the reading
and this computer's clock are both available to whatever consumes it:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `date` | string | The scanner's date as `YYYY-MM-DD`, such as `2026-08-02`. |
| `time` | string | The scanner's time of day as `HH:MM:SS` on a 24 hour clock, such as `14:52:03`. Seconds are reported; the scanner keeps no finer resolution. |
| `daylightSaving` | boolean | Whether the scanner is applying daylight saving to the time it reports. |
| `valid` | boolean | Whether the scanner's clock is running. False means the reported date and time are unreliable. |

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: accepts 1 arg(s), received 0` | `clock set` was run with no date and time. | Give a value, or use `clock sync` to take this computer's. |
| `error: invalid date and time "<value>": want "2026-08-02 14:30:00", a date alone such as "2026-08-02", or a time alone such as "14:30"` | The value is not in one of the accepted forms. | Rewrite it in one of the forms listed above. |
| `error: unknown command "<value>" for "radiocli clock"` | A date or time was passed to the bare command. | Use `clock set <datetime>`, or `clock sync`. |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: opening <port>: no such file or directory (run "radiocli devices" to see what is attached)` | The named port does not exist. | Run [`devices`](devices.md) to find the current port, and pass that. |
| `error: reading the clock: <detail>` | The scanner answered, but not with a clock reading it could parse. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |
| `error: reading the clock before changing it: <detail>` | `clock set` was given only a date or only a time, and the scanner would not report the half that was to be kept. Nothing was changed. | Give a full date and time, which needs no reading first, or use `clock sync`. |
| `error: setting the clock: <detail>` | The scanner refused the change. It answers a value it cannot take right now the same way it answers one it does not understand. | Check the scanner is not held in a menu, and run with `--verbose` to see the raw exchange. |
| `error: reading the clock back: <detail>` | The change was sent, but the scanner did not report its clock afterwards. | Run `clock` to see where the time actually ended up. The change may well have taken. |

The two input errors are produced before the scanner is opened, so they appear
whether or not one is attached, and nothing is sent when they do.

Neither a stopped clock nor a value the scanner did not take is an error. Both
are printed with a warning and exit `0`, because the scanner answered the
question it was asked.
