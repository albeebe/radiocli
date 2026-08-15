# Examples

A tour of `radiocli` by way of things you might actually want to do. Every
command here has its own page under [commands/](commands/) with the full
detail; this is the short version.

Lines marked `(writes)` change something on the scanner. Everything else only
reads.

## Getting started

```
radiocli devices                  # what is plugged in, and the port to use
radiocli version                  # what this binary was built from
```

`devices` prints the port in its `PORT` column. Every command that talks to the
scanner has to be told which one, because nothing is remembered between runs:

```
radiocli status --device /dev/cu.usbmodem00000000000011
```

That gets long, so the examples on the rest of this page are written as if the
port were in a shell variable:

```
SDS=/dev/cu.usbmodem00000000000011
radiocli --device $SDS status     # confirm it is answering, and what it is doing
```

**The lists below leave `--device $SDS` off to stay readable.** Add it to every
command that touches the radio. The ones that never need it are `devices` and
`audio`, which look at this computer rather than at a scanner, `version` and
`config`, which are about the tool itself, `backup`, which reads the memory card
instead of the serial port, and `colors palette`, which prints a built-in table.

## What is it doing right now

```
radiocli scanning                 # the channels being scanned   (writes)
radiocli scanning systems         # the systems being cycled
radiocli screen                   # the display, as text
radiocli menu                     # the menu it is showing, if any
```

`scanning systems` is the one to reach for. It is complete rather than a
sample, and it works whether you are scanning your own lists or the full
database. Scanning your own lists it reads them out of memory, which is instant
and leaves the scanner scanning; with the full database switched on it has to
turn the knob instead, which stops the scanner for a few seconds.

The bare `scanning` command always stops the scanner while it runs, and puts it
back afterwards.

## Looking through the memory

The scanner's memory is four levels deep. Each command takes the name of the
thing above it:

```
radiocli favorites                                # the lists
radiocli systems "GREENDALE, ST 00000"            # systems in a list
radiocli departments "PUBLIC SAFETY"              # departments in a system
radiocli channels "FIRE RESCUE"                   # channels, and what they receive
radiocli channels "FIRE RESCUE" --names           # names only
```

A trunked system has a fifth thing beside its departments, holding the pool of
frequencies it shares out:

```
radiocli sites "CITY OF GREEN BANK"      # sites of a trunked system
radiocli sites frequencies "GREEN BANK"  # the pool at one site
```

For "what is being scanned" rather than "what is in this department", use
`scanning` instead.

## Choosing what gets scanned

```
radiocli favorites scan "GREENDALE, ST 00000"     # only this list        (writes)
radiocli favorites scan "Full Database"           # only the database     (writes)
radiocli favorites scan --all                     # everything            (writes)
radiocli favorites scan --none                    # nothing               (writes)
```

Naming lists means **only** those. Everything else is switched off.

## Where the scanner thinks it is

```
radiocli location                                      # position and radius
radiocli location set 24944                            # point it at a zip     (writes)
radiocli location set 24944 --range 5                  # tighter radius        (writes)
radiocli location set --position 38.433056,-79.839722  # an exact position     (writes)
radiocli location gps                                  # back to the GPS       (writes)
radiocli location gps --wait                           # ...and wait for a fix (writes)
radiocli location gps --off                            # stop following it     (writes)
radiocli location gps --status                         # is it following it?
```

Three things worth knowing, because they surprise people:

- **Setting a zip switches the GPS off.** That is why the position stays put.
  `location gps` is the way back.
- **The zip cannot be read back.** The scanner keeps the position, not the zip
  that produced it. `location set --position` is how a position is put back
  where it was, and `location gps --off` is what stops the receiver
  overwriting it.
- **Setting a zip switches Full Database scanning on**, which undoes any
  narrower choice you made with `favorites scan`. It says so when it does.

## Listening to one frequency

```
radiocli tune 107.9                               # FM broadcast          (writes)
radiocli tune 155.550                             # public safety         (writes)
radiocli tune 162.550                             # NOAA weather          (writes)
radiocli scan                                     # back to scanning      (writes)
```

`tune` refuses a frequency the scanner cannot receive, and says what it does
cover:

```
$ radiocli tune 1030kHz
error: the scanner cannot receive 1.0300 MHz: it covers 25.0000 MHz to 512.0000 MHz, ...
```

## Listening to the weather

```
radiocli weather                                  # NOAA broadcast        (writes)
radiocli weather stop                             # back to scanning      (writes)
radiocli scan                                     # the same thing        (writes)
```

`weather` measures all seven NOAA channels itself and parks the scanner on the
strongest, which takes about six seconds and prints what it found on each:

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

The measuring is the point: the scanner left to itself stops on the first
channel that opens its squelch, which is not the strongest and is sometimes
not receivable at all.

Nothing else knocks it off. Changing the volume or the backlight leaves the
broadcast playing, on purpose. `weather stop` and `scan` are the two commands
that bring it back.

## Editing the memory

```
radiocli favorites new "GREEN BANK, WV 24944"                         (writes)
radiocli systems new "GREEN BANK, WV 24944" "PD" --type Conventional  (writes)
radiocli departments new "PUBLIC SAFETY" "POLICE"                     (writes)
radiocli channels new "POLICE" 155.550 "DISPATCH"                     (writes)
```

`--type` is required, and takes one of the scanner's own names: `Conventional`,
`P25 Trunk`, `Motorola`, `EDACS`, `LTR` and the rest.

Deleting works at every level, and always needs `--yes`:

```
radiocli favorites delete "OLD LIST" --yes                       (writes)
radiocli systems delete "OLD SYSTEM" --yes                       (writes)
radiocli departments delete "OLD DEPT" --yes                     (writes)
radiocli channels delete "POLICE" "OLD CHANNEL" --yes            (writes)
```

**Deleting takes everything underneath with it.** A list takes its systems,
a system takes its departments, a department takes its channels. There is no
undo, which is why `--yes` is not optional.

Renaming works at every level:

```
radiocli favorites rename "OLD NAME" "GREENDALE, ST 00000"       (writes)
radiocli systems rename "OLD NAME" "NEW NAME"                    (writes)
radiocli departments rename "POLICE" "POLICE DEPARTMENT"         (writes)
radiocli channels rename "POLICE" "DISPACH" "DISPATCH"           (writes)
```

**Rename rather than delete and recreate.** A channel keeps its frequency and
its per-channel settings through a rename, and loses all of them if it is made
again from scratch.

**The frequency comes before the name in `channels new`**, because that is the
order the scanner asks in: it opens a frequency screen before the channel
exists at all.

## Adding a trunked system

A conventional system gives each department its own frequencies. A trunked
system shares a pool across everybody on it and hands one out per transmission,
so it is built in two halves: a site holding the frequencies, and departments
holding talkgroups.

```
radiocli systems new "GREENDALE, ST 00000" "CITY OF GREEN BANK" --type "P25 Trunk"

radiocli sites new "CITY OF GREEN BANK" "GREEN BANK"                  (writes)
radiocli sites frequencies add "GREEN BANK" 851.050 851.600 852.3625  (writes)

radiocli departments new "CITY OF GREEN BANK" "GREENDALE FIRE"        (writes)
radiocli channels new "GREENDALE FIRE" TGID:9051 "FIRE DISPATCH"      (writes)
```

**A talkgroup goes where a frequency would**, written with the `TGID:` prefix
the scanner uses to report one. They are one argument because they are one
thing: the address of what the channel receives. Giving the wrong kind for the
department is refused before anything is created.

**Give a site all its frequencies.** A system that knows only some of the pool
loses every call handed to one it does not know, which sounds like a busy
system going quiet at random.

## Settings

```
radiocli battery                                  # charge and temperature
radiocli backlight                                # is the scanner lit
radiocli backlight on                             # light it up           (writes)
radiocli backlight off                            # put it out            (writes)
radiocli backlight keys disable                   # keypad stays dark     (writes)
radiocli beep                                     # sound the keypad makes
radiocli beep set off                             # silence the keypad     (writes)
radiocli beep toggle                              # silence it, or put it back (writes)
radiocli colors                                   # colors of the layout in use
radiocli colors weather                           # colors of one named layout
radiocli colors --all                             # every layout, about three minutes
radiocli colors --cache                           # the last reading, without the wait
radiocli colors --positions                       # where each area sits, instantly
radiocli colors set System_name --text Cyan       # recolor one area       (writes)
radiocli colors --verify-palette                  # check the built-in color list
radiocli display                                  # color or black and white
radiocli display mode black                       # white text on black   (writes)
radiocli volume                                   # current level
radiocli volume set 8                             # 0 to 15               (writes)
radiocli clock                                    # date and time
radiocli clock sync                               # match this computer   (writes)
```

## Driving it by hand

```
radiocli menu open top                            # open a menu           (writes)
radiocli menu                                     # see what is on it
radiocli key right right enter                    # press keys            (writes)
radiocli menu close                               # back out              (writes)
radiocli scan                                     # back to scanning      (writes)
```

`key` is the blunt instrument: it presses what you ask and checks nothing.
Prefer a command that names what it does. Use `--pace slow` if you are
stepping through something by hand.

## Output for scripts

Every listing command speaks JSON:

```
radiocli scanning systems -o json
radiocli favorites -o json | jq -r '.[] | select(.monitored) | .name'
radiocli channels "FIRE RESCUE" -o json | jq -r '.[].frequency'
radiocli location -o json | jq '.range'
```

Results go to stdout and everything else to stderr, so redirecting stderr
leaves only the answer.

## A worked example

Point the scanner at a town, see what that gets you, then put it back:

```
radiocli location set 54321 --range 10      # 10 miles surrounding a zip
radiocli scanning systems                   # 16 systems, about 7 seconds
radiocli location gps                       # back to where you really are
radiocli favorites scan "GREENDALE, ST 00000"   # and back to your own list
```

That last line matters: `location set` switched Full Database on, and nothing
switches it back off for you.

## Global flags

These work everywhere. See [global flags](global_flags.md) for the detail.

```
radiocli --device /dev/cu.usbmodem00000000000011 status   # a specific scanner
radiocli -o json favorites                                # JSON output
radiocli --pace slow key right right                      # slower keypresses
radiocli -v tune 107.9                                    # show the raw exchange
```

## One thing that will not run

```
$ radiocli systems "Full Database"
error: "Full Database" is the scanner's built-in database rather than a favorites
list, and asking it for its systems returns a short, wrong answer and then locks
the scanner up until it is power cycled: run "radiocli scanning systems" to read
what it is scanning instead
```

The lockup was reproduced twice, so the tool refuses the request instead of
sending it. Naming the database by its reserved index gets the same refusal.
Use [`scanning systems`](commands/scanning.md) to see the database instead.
