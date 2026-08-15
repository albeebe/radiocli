# scanning

Reports what the scanner is working through right now, whether that comes from
a favorites list, a zip code, or the GPS.

## Overview

`scanning` answers "what is this thing actually listening to". It covers
whatever is switched on, and it makes no distinction between your own lists and
the scanner's database: the walk does not care what put the entries there.

There are two views. [`scanning systems`](#scanning-systems) lists the systems
being cycled through, which is complete either way and instant unless the full
database is switched on. The bare command lists individual channels, which is
exact for a favorites list and only a sample for the full database.

Both may stop the scanner scanning while they run, and return it afterwards.
They need a scanner, so name one with `--device`.

## Usage

```
radiocli scanning [--limit <n>] [--watch <duration>] [flags]
radiocli scanning systems [flags]
```

## `scanning systems`

Lists the systems the scanner is cycling through:

```
$ radiocli scanning systems
PUBLIC SAFETY
```

**How it answers depends on whether `Full Database` is switched on**, because
the two cases are not the same problem.

### Favorites lists, which is the ordinary case

The systems are read straight out of the scanner's memory, one request per
monitored list. That takes well under a second, presses no keys, and leaves the
scanner scanning the entire time. **It covers every list that is switched on**,
not just the first, and systems you have set to avoid are left out, because the
question is what the scanner is working through rather than what it holds.

The built-in `Search with Scan` source is skipped: it scans frequency ranges
and holds no systems of its own.

### The full database, which has to be walked

The database has no listing to ask for and its systems have no index, so this
case works the way a person does: select the system level with the left soft
key, then click the knob round one system at a time, reading each name off the
screen.

```
$ radiocli scanning systems
Turning the knob to read the systems, because "Full Database" is switched on and cannot be listed. This stops the scanner for a moment.
Businesses
Environmental Police
Northmoreland Emergenc
Northmoreland Railroads
Medevac
Recreation and Sports
Green Bank Police Depart
Commonwealth Of Northm
Dispatch Communication
Northmoreland State Po
American Red Cross - USA
National Interoperabil
PUBLIC SAFETY
Ashford
Bellwood
Carrick
```

**The names are cut short**, because they are read off a 24-character screen and
the database has no listing to get them from intact. `Northmoreland Emergenc` is
the whole of what the scanner shows. Reading a favorites list does not have this
problem, since those names come from memory.

Sixteen systems in under seven seconds, and running it again gives the same
sixteen. **This is the one answer here that is complete rather than sampled**,
because the walk is a loop and finishes by arriving back where it started.

**The order is not fixed.** The walk starts wherever the scanner happens to be
in its cycle, so two runs list the same systems from a different starting
point. Compare the answers as sets, not line by line.

### Why the walk is paced

**Each click waits for the screen to change before the next one goes**, up to a
ceiling of half a second. It does not turn on a clock. On the scanner tested
that works out at about 320ms a click, so sixteen systems take just under seven
seconds, and it settles wherever your scanner's redraw actually lands rather
than at a number written down here.

It used to sleep the full half second after every click regardless, which was
the same sixteen systems in 10.6 seconds. Waiting for the change establishes
what the sleep was guessing at, and stops as soon as it is true.

Four other things about the mechanism constrain the walk, each found by getting
it wrong:

- **The system key is a toggle with no readable state.** Pressing it during the
  walk switches the level back off and the knob quietly starts doing something
  else. It is pressed once at the start, and again only when the knob has
  provably stopped moving anything.
- **A walk must start from plain scanning.** Because the toggle cannot be read,
  a run beginning while the level was already on turns it off. Three runs in a
  row once returned 5, 1 and 18 systems from an unchanged scanner for exactly
  this reason, so the command resets the scanner before it starts.
- **The display redraws more slowly than the knob turns.** Clicking on a fixed
  cadence of five a second lags the readings, misses systems, and circles
  several times without ever closing. Turning faster and reading sooner are not
  the same thing, and it is turning faster that breaks this.
- **The level lapses on its own** if the knob stops being turned, so the walk
  never idles: the moment the screen has caught up, the next click goes.
- **The walk has to know when the knob has left the scanning screen**, because
  past the last system it steps into the scanner's menus, where the lines it
  reads mean something else: one run reported `Copy System` as a system. It
  asks the scanner whether it is in a menu. Watching the line that names the
  favorites list instead looks equivalent and is not, because that line changes
  legitimately every time the walk crosses from one list into the next: with
  two lists switched on, the walk stopped at the first system of the second and
  called that the whole answer.

### A walk with one system to step between

There is nowhere to step to, so the knob moves nothing and the screen never
changes. The walk notices by counting clicks rather than systems: counting
systems alone never advances in this case, which is an infinite loop rather than
an answer.

It takes about twenty clicks to be sure, because a knob that moves nothing looks
exactly like a system level that has lapsed, and the only way to tell them apart
is to switch the level on again and try once more. This is the one case that
pays the full half second per click, since there is never a change to stop
waiting for: twenty key presses the scanner beeps at, against a screen that does
not move, for a one-line answer. It is the price of the database having no
listing, and it is why the favorites case does not pay it.

### Output

Under `--output text`, one system per line. Under `--output json`, an array of
strings.

With nothing switched on to scan, stdout is empty and stderr says so.

If a walk does not come back to where it started, it says so on stderr, and the
list should be read as what was seen rather than everything there is.

## `scanning` (channels)

The bare command lists individual channels with their frequency or talkgroup.
How it answers depends on what is switched on, because the two cases are not
the same question.

**A favorites list is walked exactly.** The scanner is held on a channel and
the knob turned, which lists every channel in about a second:

```
$ radiocli scanning
SYSTEM         DEPARTMENT         CHANNEL       VALUE
PUBLIC SAFETY  FIRE RESCUE        DISPATCH      153.980000MHz
PUBLIC SAFETY  POLICE DEPARTMENT  DISPATCH      155.550000MHz
```

**The full database cannot be walked that way.** Held, the knob browses the
whole database rather than the part being scanned, which is hundreds of
thousands of entries and never finishes. That case is watched instead: the
scanner names each system and department as it cycles past, and this collects
them until they stop being new.

```
$ radiocli scanning
Watching the scan cycle for up to 45s, because "Full Database" is switched on and cannot be walked.
...
Seen in 35 places while the scanner cycled. The database holds more than this:
anything that had not come round yet is missing.
```

**That is a sample and says so.** For a complete answer about the database, use
[`scanning systems`](#scanning-systems), which is complete because it closes a
loop.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--limit` | No | `500` | Stop after this many channels, when walking a favorites list. |
| `--watch` | No | `45s` | How long to watch the cycle, when the full database is switched on. |

### Output

Under `--output text`, a table of system, department, channel and value. Under
`--output json`, an array of objects that also carries `source`, the list each
channel came from.

**`value` is a string and is not always a frequency.** A conventional channel
gives `155.550000MHz`; a channel on a trunked system gives `TGID:3073`, which
is a talkgroup and not a number of megahertz at all.

## What neither of these can do

**Names are truncated to the width of the screen.** `Northmoreland State Po` is
`Northmoreland State Police`. The scanner cuts them itself and there is no
longer form to read.

**Channel-level avoids are invisible.** A channel that has been individually
avoided is still listed. Systems and departments report an `avoided` flag
through [`systems`](systems.md) and [`departments`](departments.md), but
nothing exposes it per channel.

## Why the protocol is not used

Every route that ought to have worked does not:

- The listing request for the full database answers with a short, wrong
  document and then **locks the scanner up until it is power cycled**. That is
  why [`systems`](systems.md) refuses to ask it.
- The database has no menu of its own. `Manage Full Database` holds nothing but
  `Stop All Avoiding` and a version string.
- Its systems have no menu index. Probing indexes 0 to 100 finds only the
  systems in your own favorites lists.

Holding the scanner and turning the knob is what a person would do, and it is
the only thing that reaches the database at all.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from [`devices`](devices.md). |
| `error: limit <n> is not a number of channels: want 1 or more` | `--limit` was zero or negative. | Pass a positive number. |
| `error: the scanner would not hold on a channel after 6 presses` | The scanner never settled on a channel to hold. | Check something is switched on with [`favorites`](favorites.md). A scan with nothing in it has nothing to hold. |
| `error: selecting the system level: <detail>` | `systems` could not select the system level. | Run [`scan`](scan.md) and try again. |
| `error: returning the scanner to scanning: <detail>` | The scanner would not go back to scanning before the walk. | Run [`scan`](scan.md) and try again. |
| `error: turning the knob: <detail>` | A keypress was refused partway through the walk. | Run with `--verbose` to see the raw exchange. |
| `error: the scanner is still holding on a channel: run "radiocli scan"` | The walk finished but the scanner would not resume. | Run [`scan`](scan.md). |

When something fails partway, the scanner is taken off hold before the error is
reported, so a failed walk does not leave it stuck on one channel.
