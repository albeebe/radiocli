# location

Reports the position the scanner is working from, and sets it from a zip code.
Run it to see where the scanner thinks it is, or to point it somewhere else.

## Overview

The SDS150 uses a position to decide which database channels are worth
scanning, and `location` reports it.

**This is not simply where the scanner is.** Left alone, the position follows
the built-in GPS: two readings taken from a scanner that has not moved differ by
a metre or two, which is the fix refining rather than anything
moving. But a zip code replaces it outright, and keeps it replaced, because
setting one switches the GPS off. A scanner in Greendale given zip `24944`
reports Green Bank, and still reports Green Bank after a power cycle.
[`gps`](#location-gps) is the way back.

So read this as "what is the scanner scanning around", not "where is the
scanner".

The bare command only reads. [`set`](#location-set) and [`gps`](#location-gps)
write, and they do so by driving the scanner's own menus, so they stop the
scanner scanning while they run and return it afterwards. It needs a scanner,
so name one with `--device`.

## Usage

```
radiocli location [flags]
radiocli location set <zip> [--range <miles>] [flags]
radiocli location set --position <latitude>,<longitude> [--range <miles>] [flags]
radiocli location gps [--wait] [flags]
radiocli location gps --off [flags]
radiocli location gps --status [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<zip>` | Yes, for `set`, unless `--position` is given | none | Five digit US zip code to point the scanner at. |
| `--position` | No | none | Latitude and longitude to set directly, as `38.433056,-79.839722`. |
| `--range` | No | `10` for a zip, unchanged for a position | Radius in whole miles to draw database channels from. `1` to `50`. |
| `--wait` | No | `false` | For `gps`: wait for the receiver to produce a fix before returning. |
| `--off` | No | `false` | For `gps`: switch the GPS off, leaving the position where it is. |
| `--status` | No | `false` | For `gps`: report whether the GPS is on. Changes nothing. |

### `<zip>`

Five digits, such as `12345`. It is checked before the scanner is touched, so a
typo costs nothing and leaves the scanner scanning.

**The scanner resolves the zip, not this tool.** The zip database lives on the
scanner, and it is the only thing here that can turn `12345` into a position.
That is why `set` walks the menus rather than sending a position over the
protocol: resolving a zip locally would mean shipping a second copy of the
scanner's own database.

Only US zip codes are accepted. The scanner's zip screen asks for a country
first and offers Canada as well, but this always answers `USA`, because a bare
five digit number is not a Canadian postal code.

### `--position`

A latitude and longitude, separated by a comma, exactly as the bare command
prints them on its `position` line. That is the point: the pair can be read off
one run and pasted into the next.

```
radiocli location set --position 38.433056,-79.839722
```

Both are degrees, positive north and east. Anything outside `-90` to `90` or
`-180` to `180` is refused before the scanner is touched.

Every position in these examples is the same corner of the world, and none of
them was measured. It is a bad place to actually listen.

**This is the only way to put a position back where it was**, because a zip
cannot be read off the scanner. See
[putting a position back](#putting-a-position-back).

### `--range`

How far around the position the scanner draws database channels from, in miles.

**With `--position`, the range is left alone unless `--range` is typed.** A
position and a radius are two settings, and moving one should not quietly
change the other. With a zip the scanner sets 10 itself, so that is the default
there.

The scanner picks `10` itself when a zip is entered, which is the only value it
has been seen to choose, so that is the default here too. The `1` to `50`
bounds are this tool's guard rather than the scanner's: a range of zero pulls
in nothing, and a range covering several states stops the database being a
filter at all. What the scanner itself would refuse has not been established.

**Whole miles only.** The range screen shows a tenth of a mile, but it cannot
be typed: a digit overwrites the character under a cursor that then advances,
so `10.0` with a `5` typed becomes `50.0`, and the two integer digits are the
whole of what can be reached.

### Global flags that change these commands

- `--device` names the scanner to use. Get the value from the `port` column of
  [`devices`](devices.md).
- `--output` selects whether the result is printed as lines or as JSON.
- `--pace` applies to `set` and `gps`, which press keys. It does nothing for
  the bare command, which only reads.

## Examples

Reading the position:

```
$ radiocli location
latitude:  38.433061
longitude: -79.839718
position:  38.433061, -79.839718
range:     none set (following the GPS)
```

Pointing the scanner at Green Bank:

```
$ radiocli location set 24944
zip:       24944
position:  38.420833, -79.797222
range:     10.0 miles
```

Reading it back afterwards, including after a reboot:

```
$ radiocli location
latitude:  38.420833
longitude: -79.797222
position:  38.420833, -79.797222
range:     10.0 miles
```

A tighter radius:

```
radiocli location set 24944 --range 5
```

Setting a position directly, which takes milliseconds and leaves the scan
running:

```
$ radiocli location set --position 38.433056,-79.839722
position:  38.433055, -79.839722
range:     10.0 miles
```

As JSON:

```
$ radiocli location -o json
{
  "latitude": 38.420833,
  "longitude": -79.797222,
  "range": 10
}
```

## Output

The reading goes to stdout. Notes and debug logs go to stderr.

Under `--output text`, the bare command prints four lines, with the labels
padded so the values line up:

```
latitude:  38.433061
longitude: -79.839718
position:  38.433061, -79.839718
range:     none set (following the GPS)
```

The `position` line repeats the two numbers together because that is the form a
map accepts, and pasting the pair somewhere is a common reason to run this.

Latitude and longitude are degrees, positive north and east, printed to six
decimal places, which is what the scanner sends. The last of those places moves
between readings while the GPS is in charge, so rounding harder would hide the
fix settling.

**A range of `0` is not a radius of nothing, it is no range set at all**, which
is how a scanner following its own GPS reads. The text output says so rather
than printing `0.0 miles`, which would be read as a distance.

Under `--output json`, stdout holds one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `latitude` | number | Degrees north of the equator, negative south. |
| `longitude` | number | Degrees east of Greenwich, negative west. |
| `range` | number | Radius in miles database channels are drawn from. `0` means none is set. |

## `location set`

`set` points the scanner at a zip code:

```
$ radiocli location set 24944
zip:       24944
position:  38.420833, -79.797222
range:     10.0 miles
```

It walks the scanner's menus to `Set Your Location`, answers the country
prompt, types the zip, and then sets the range on a second pass. The position
is read back afterwards, because the scanner is the only authority on whether
the zip meant anything.

**A zip the scanner does not hold is refused**, and the refusal is reported.
The scanner answers `Out of Range` on its own screen, which this reads and
turns into an error. Nothing is changed and the position stays where it was.

**Setting a location can switch on Full Database scanning.** A position only
decides anything if the full database is being scanned, so a scanner with it
switched off asks whether to switch it on. Answering yes is the only answer
that makes the zip do anything, so that is what happens, and a note is written
to stderr saying so. It changes what the scanner scans, which is worth knowing
about rather than discovering later.

**Setting a zip switches the GPS off**, which is what makes the position stay
where it was put. [`location gps`](#location-gps) switches it back on.

**Loading a new location takes a few seconds**, during which the scanner
rebuilds from the database and stops answering the protocol. This waits for it
rather than pressing on.

**This stops the scanner scanning**, and returns it when finished.

## Putting a position back

**The scanner keeps no record of the zip it was given.** It resolves one
against its own database the moment it is typed, stores the position, and
forgets the zip. Four places were checked on the hardware and none of them
holds it:

| Where | What is actually there |
| ----- | ---------------------- |
| `Enter Zip Code` | Opens blank. It does not pre-fill the zip in use. |
| `Edit Location` | A list of locations saved by hand, not the current one. |
| `Set Manual Location` | The position, in degrees and minutes: `42°52'00.08" N`. |
| The `LCR` protocol command | Latitude, longitude, range. That is all it carries. |

So "set it back to the zip it was on" is not a thing that can be done. Setting
it back to the **position** it was on is, and that is what `--position` is for.
Record where the scanner is:

```
$ radiocli location -o json > before.json
$ cat before.json
{
  "latitude": 38.433056,
  "longitude": -79.839722,
  "range": 10
}
```

Then put it back, however far it has been moved in between:

```
$ radiocli location set --position "$(jq -r '"\(.latitude),\(.longitude)"' before.json)" --range "$(jq -r .range before.json)"
position:  38.433055, -79.839722
range:     10.0 miles
```

Three things make this different from setting a zip:

- **It is usually immediate.** A write and a read back, about 20 milliseconds
  against the 26 seconds a zip takes through the menus. See below for when it
  is not.
- **It does not stop the scan**, because it never enters a menu.
- **It does not switch Full Database scanning on**, which setting a zip does.
  Nothing else about what is scanned changes either.

**It is slow when the database has a lot to reconsider.** The scanner works out
which of the database's channels are within range of the new position before it
acknowledges the write, and says nothing at all until it has. At one mile that
is unnoticeable. Moving away from twenty-five miles around a city, measured on
an SDS150, it took **15 seconds**:

```
$ time radiocli location set --position 38.433056,-79.839722 --range 1
position:  38.433056, -79.839722
range:     1.0 miles
15.1s
```

Nothing is wrong during that silence and nothing needs doing about it. It is
worth knowing because it used to be reported as a failure: the command gave up
after three seconds and said the scanner had not answered, when the position had
in fact been written and only the acknowledgement was late. Anything reading the
position back saw the old value until the scanner surfaced.

**The last decimal place moves, once.** The scanner keeps six decimal places to
within a millionth of a degree, so `38.433056` written comes back as
`38.433055`, about a tenth of a metre. It does not accumulate: reading that
value and writing it back five times over left every reading identical, so a
position restored again and again does not walk.

**A position does not switch the GPS off**, and that is the one thing to watch.
A scanner following its GPS overwrites whatever is written here as soon as the
receiver has a fix. Measured on one scanner: the write held for 65 seconds and
was then replaced by the receiver's own position, which then wandered by a
metre or two between readings in the way a live fix does.

Two things switch the GPS off: setting a zip code, and
[`location gps --off`](#--off). Run the second after writing a position and it
stays where it was put.

## `location gps`

`gps` hands the position back to the built-in receiver:

```
$ radiocli location gps
source:    GPS
position:  38.433059, -79.839714
range:     10.0 miles
```

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--off` | No | `false` | Switch the GPS off, leaving the position where it is. |
| `--status` | No | `false` | Report whether the GPS is on. Changes nothing. |
| `--wait` | No | `false` | Wait for the receiver to produce a fix before returning. |

The three are mutually exclusive. Passing two is refused before the scanner is
touched.

**Setting a zip code switches the GPS off.** That is why a zip stays put
instead of being overwritten by the next fix, and it is one of the two reasons
this command exists. The setting lives at `Menu > GPS > Set GPS Function`, and
reads `Disable` immediately after any `location set`. Nothing in the scanner's
own `Set Your Location` menu offers a way back, which is what made this hard to
find.

### `--status`

Reports which of the two values the setting holds, and changes nothing:

```
$ radiocli location gps --status
gps:       off
position:  38.433055, -79.839722
range:     10.0 miles
```

`gps` is `on` or `off`. The scanner's own words for the two are `Enable` and
`Disable`, which name the action rather than the state, so this reports what a
reader asked: whether the receiver is in charge.

Under `--output json`:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `gps` | boolean | `true` when the scanner is following its receiver. |
| `latitude` | number | Degrees north of the equator, negative south. |
| `longitude` | number | Degrees east of Greenwich, negative west. |
| `range` | number | Radius in miles database channels are drawn from. `0` means none is set. |

```
$ radiocli location gps --status -o json
{
  "gps": false,
  "latitude": 38.433055,
  "longitude": -79.839722,
  "range": 10
}
```

**This stops the scanner scanning for a moment**, even though it only reads.
The setting is shown as the highlighted entry of the screen that sets it, so
asking means opening that screen. There is no way to ask over the protocol.

### `--off`

Switches the GPS off and leaves the position exactly where it is:

```
$ radiocli location gps --off
source:    fixed
position:  38.433055, -79.839722
range:     10.0 miles
```

This is how a position stays put without a zip code to pin it. Written on its
own, a position is overwritten as soon as the receiver has a fix; with the GPS
off, it stays. The pair that holds a scanner somewhere it is not:

```
radiocli location set --position 38.433056,-79.839722
radiocli location gps --off
```

**It moves nothing.** The position the scanner was on when the command ran is
the position it keeps, whether that came from a zip, from `--position`, or from
the receiver's own last fix.

The setting is read back afterwards to confirm it took, the same way switching
it on is.

**It returns as soon as the scanner is following the GPS**, in a few seconds.
It does not wait for the receiver to work out where that is, because that is
not what the command did: the command changed a setting, and the setting is
what gets read back to confirm it took. Where the receiver thinks it is arrives
afterwards, on its own schedule.

That schedule varies more than is comfortable to wait on. Measured on one
scanner indoors:

| From | Time to a fix |
| ---- | ------------- |
| Already near the real position | seconds, a warm start |
| A zip 300km away | around 50 seconds |
| A zip 1,300km away | around 135 seconds |

The further the zip had displaced it, the more sky the receiver has to search.
Blocking the command on that would mean waiting on satellites to report work
that had already finished.

**The position reported may still be the old one.** When the receiver has not
produced a fix yet, the scanner keeps reporting whatever was set by hand, and
the two are indistinguishable by looking. A note on stderr says so when it
happens:

```
That position is the one that was set by hand. The receiver has not produced its own fix yet.
Run "radiocli location" in a minute to see it move.
```

Watching it move is how you know the receiver has taken over. A live fix
wanders by a metre or two between readings; a position set by hand does not
move at all.

**`--wait` sits through it**, for a script that needs the real position before
carrying on. It gives up after 90 seconds and reports the position anyway,
rather than failing, because the GPS is genuinely on either way.

The range is left alone, in both directions. A zip sets it to 10 miles, and
that survives switching the GPS on and off again, so the position follows the
receiver while the radius stays where it was put.

**This stops the scanner scanning**, and returns it when finished. That applies
to `--status` as well, which has to open a menu to read the setting.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from [`devices`](devices.md). |
| `error: reading the location: <detail>` | The scanner answered, but not with a position it could parse. | Run with `--verbose` to see the raw exchange. |
| `error: "<value>" is not a zip code: want five digits, such as 12345` | The argument was not five digits. | Give a five digit US zip code. |
| `error: name a zip code, or pass --position` | `set` was run with neither. | Give one of the two. |
| `error: a zip code and --position are two ways of saying the same thing: choose one` | Both were given. | Keep one. |
| `error: "<value>" is not a position: want a latitude and a longitude separated by a comma, such as 38.433056,-79.839722` | `--position` had no comma. | Write the pair as the bare command prints it. |
| `error: "<value>" is not a latitude: want degrees from -90 to 90` | The first half is not a latitude. | Check the two halves are the right way round. |
| `error: "<value>" is not a longitude: want degrees from -180 to 180` | The second half is not a longitude. | Check the two halves are the right way round. |
| `error: the scanner is at <position> rather than the <position> it was given` | The position did not take, which is what a GPS still in charge looks like. | Set a zip, which switches the GPS off. |
| `error: range <n> is out of range: want 1 to 50 whole miles` | `--range` was outside the accepted bounds. | Choose a whole number of miles between `1` and `50`. |
| `error: the scanner does not hold zip code <zip>: it answered "Out of Range"` | The scanner refused the zip. | Check the zip exists in the scanner's database. |
| `error: looking for "Set Your Location" in the top menu: <detail>` | The menu is not laid out as expected, which a firmware change could cause. | Run [`screen`](screen.md) with the top menu open to see what it holds. |
| `error: the position was set to <zip>, but its range was not: <detail>` | The zip took, the range did not. | Run `radiocli location` to see where the scanner is, and set the range again. |
| `error: the scanner still shows "<value>" for "Set GPS Function": it was not set to "<value>"` | The GPS setting did not change. | Run [`screen`](screen.md) with that menu open to see what it holds. |
| `error: --off changes the setting and --status only reads it: choose one` | Both were passed. | Keep one. |
| `error: --status only reads the setting, so there is no fix to wait for` | `--status` was passed with `--wait`. | Drop `--wait`. |
| `error: --wait waits for the receiver to find the scanner, which is not something switching the GPS off does` | `--off` was passed with `--wait`. | Drop `--wait`. |

When something fails partway, the scanner is returned to scanning before the
error is reported, so a failed run does not leave it parked in a menu.

## A warning about the full database

`radiocli systems "Full Database"` is refused, by name and by index. It returns
a short list and then leaves the scanner's serial interface dead, requiring a
power cycle, which has been reproduced twice. It is unrelated to this command,
but a location set by zip is exactly what makes that list interesting, so it is
worth knowing before you go looking. [`scanning systems`](scanning.md) is the
way to read the database.
