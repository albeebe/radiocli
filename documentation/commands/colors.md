# colors

Reports the text and background color of every area of one of the scanner's
screen layouts. Run it to find out what color the scanner draws each part of
its screen in.

## Overview

The scanner draws its screen from seven layouts, one per kind of screen, and
each layout is a list of named areas with a text color and a background color of
its own. `colors` reads one of those layouts.

With no argument it reads the layout the scanner is drawing with right now. Name
a layout to read that one instead, whatever the scanner is doing.

None of this is in the scanner's remote protocol. The colors live behind
`MENU → Display Options → Customize`, and this command reads them the only way
there is: it walks in, opens every area's two color pickers in turn, and reads
the value off the screen. That takes about thirty seconds for one layout and
stops the scan until it finishes. **It changes nothing.** No picker is ever
confirmed, and the scanner is returned to scanning at the end.

Whether the colors are drawn at all is a separate setting. See
[`display`](display.md): in either black and white mode everything this command
reports is stored and ignored.

The command needs a scanner, so name one with `--device`.

Every area also carries **where it sits on the screen**, which is what turns a
character position into an area and so into a color. Those positions are built
into the tool rather than read from the scanner, because nothing in its menus
moves an area. That makes them free: `--positions` answers instantly, opens no
menus, and leaves the scanner scanning.

**Every read is cached**, and `--cache` hands the last one back instead of
walking the menus again. It never opens a menu: a layout that has not been read
yet is an error, not a wait. See [The cache](#the-cache).

## Usage

```
radiocli colors [layout] [flags]
radiocli colors --all [flags]
radiocli colors [layout] --cache [flags]
radiocli colors [layout] --positions [flags]
radiocli colors [layout] --verify-positions [flags]
radiocli colors [layout] --verify-palette [flags]
radiocli colors set <area> [flags]
radiocli colors palette
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<layout>` | no | the layout in use | Which layout to read. With no layout, the one the scanner is drawing with right now. |

The seven layouts, in the order the scanner's Customize menu lists them:

| Value | The scanner's own entry | What it draws |
| ----- | ----------------------- | ------------- |
| `simple-conventional` | `Set Simple Conventional` | Scanning a conventional system, with the scan display mode set to Simple. |
| `simple-trunk` | `Set Simple Trunk` | Scanning a trunked system, with the scan display mode set to Simple. |
| `detail-conventional` | `Set Detail Conventional` | Scanning a conventional system, with the scan display mode set to Detail. |
| `detail-trunk` | `Set Detail Trunk` | Scanning a trunked system, with the scan display mode set to Detail. |
| `search` | `Set Search/CC Mode` | Searching, and Close Call. |
| `weather` | `Set Weather Mode` | Weather alert. |
| `tone-out` | `Set Tone Out Mode` | Tone-out. |

The argument is matched case insensitively. Anything else is refused before the
scanner is touched.

### Flags

| Flag | Required | Default | Description |
| ---- | -------- | ------- | ----------- |
| `--all` | No | `false` | Read every layout in turn and cache each. Takes about three minutes and stops the scan throughout. See [Reading every layout](#reading-every-layout). |
| `--cache` | No | `false` | Report the last reading of this layout instead of walking the menus. Opens no color pickers, and fails if the layout has never been read. |
| `--positions` | No | `false` | Report where each area sits and not its colors. Opens no menus, takes milliseconds, and does not stop the scan. |
| `--verify-positions` | No | `false` | Check the built-in positions against this scanner and report any that differ. Walks the layout's editor, which takes about ten seconds and stops the scan. |
| `--verify-palette` | No | `false` | Check the built-in list of colors against this scanner and report any that differ. Walks one color picker end to end, which takes about seven seconds and stops the scan. |

No two of `--positions`, `--verify-positions` and `--verify-palette` may be
given together: each asks the scanner a different question, and answering one
silently would leave you believing you got another. `--cache` cannot be combined
with any of them either, for the opposite reason: none of the three reads the
colors, so there is nothing for the cache to answer.

`--all` cannot be combined with any of the other four, and cannot be given a
layout either. It says which layouts to read, and every one of the others is a
question asked about one layout; naming a layout as well says both "this one"
and "all of them", so it is refused rather than one of them quietly winning.

### `colors set`

Changes the color one area is drawn in. This is the only part of the command
that writes to the scanner.

```
radiocli colors set <area> --text <color> --back <color> [--layout <layout>]
```

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<area>` | Yes | - | The area to change, named as the scanner names it, such as `System_name`. Matched case insensitively. |
| `--text` | No | unchanged | The color to draw the area's text in. |
| `--back` | No | unchanged | The color to draw its background in. |
| `--layout` | No | the layout in use | Which layout to change. |

At least one of `--text` and `--back` is required. Both may be given, and then
both are set in one visit to the area.

**Colors are named, not numbered.** The scanner offers 147, which are the CSS
color names, and its picker has no way to enter a value. Passing a hex value is
refused.

**Its colors are not quite the CSS ones.** The scanner's Orangered is `#FF4600`
where CSS says `#FF4500`, and its Darksalmon is `#E79473` where CSS says
`#E9967A`. The display quantizes color: there are 54 distinct levels per channel
across the whole palette. So a name here is a label for the scanner's color, not
for the web's, and the value reported is the scanner's own.

A name that is not one of the 147 is refused before the scanner is touched, with
suggestions when there is a near miss. Run `colors palette` for the whole list.

Setting a color also updates any cached reading of that layout, so the next
`--cache` reports what was just set rather than what it used to be.

### `colors palette`

Lists all 147 colors the scanner's pickers offer, in the order its knob steps
through them. These are the names `colors set` accepts.

```
radiocli colors palette
```

It takes no arguments and no flags of its own.

**It needs no scanner.** The palette is a built-in table rather than a reading,
because it is the firmware's and no menu changes it, so asking the radio would
mean walking a picker end to end to be told what is already written down. That
makes this the one part of `colors` that answers with nothing plugged in, and it
answers instantly.

The order is the knob's, which is alphabetical, and it **wraps**: stepping right
past `Yellowgreen` lands on `Aliceblue`, so no color is more than half a ring
from any other. That is what the `step` value counts, and it is the only sense
in which the scanner numbers its colors: its pickers show a name and never a
number.

To check the table against the radio in front of you, use `--verify-palette`,
which walks a real picker end to end.

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column
  of [`devices`](devices.md).
- `--output` selects whether the colors are printed as a table or as JSON.
- `--pace` sets the gap between key presses. This command presses several
  hundred, so anything slower than the default multiplies the time it takes:
  at `--pace slow` one layout would take most of an hour.

## Examples

Reading the layout the scanner is using:

```
$ radiocli colors
layout: detail-conventional (Set Detail Conventional)
        the scanner is drawing with this one

AREA            TEXT        HEX      BACKGROUND  HEX
Func            White       #FFFFFF  Black       #000000
Option_1        White       #FFFFFF  Black       #000000
Option_2        White       #FFFFFF  Black       #000000
Option_3        White       #FFFFFF  Black       #000000
Option_4        White       #FFFFFF  Black       #000000
[45 more rows]

50 areas
```

Reading a layout the scanner is not using:

```
$ radiocli colors weather
layout: weather (Set Weather Mode)

AREA              TEXT        HEX      BACKGROUND  HEX
Func              White       #FFFFFF  Black       #000000
[44 more rows]

45 areas
```

Reading the same layout again from the cache, which takes no menus and no half
minute:

```
$ radiocli colors weather --cache
layout: weather (Set Weather Mode)
cached: 2026-08-06 09:12:00 (18 hours ago)

AREA              TEXT        HEX      BACKGROUND  HEX
Func              White       #FFFFFF  Black       #000000
[44 more rows]

45 areas
```

Asking the cache for a layout that has never been read, which fails rather than
quietly spending thirty seconds:

```
$ radiocli colors tone-out --cache
error: no cached colors for tone-out
Run "radiocli colors tone-out" to read them from the scanner, which caches them for next time
```

Listing the colors the scanner offers, which needs no scanner at all:

```
$ radiocli colors palette
STEP  NAME                  HEX
0     Aliceblue             #EFF7FF
1     Antiquewhite          #F7EBD6
2     Aqua                  #00FBF7
[143 more rows]
146   Yellowgreen           #94CA31

147 colors
```

Just the positions, which is instant:

```
$ radiocli colors --positions
layout: detail-conventional (Set Detail Conventional)
        the scanner is drawing with this one

AREA            LINE  COL  LEN  ROWS
Func            0     0    2    1
Option_1        0     2    6    1
Option_2        0     8    6    1
[45 more rows]

50 areas
```

Checking the built-in map against your scanner:

```
$ radiocli colors --verify-positions
Checking Set Detail Conventional against the scanner. This stops the scan for a moment.
layout:  detail-conventional (Set Detail Conventional)
checked: 45 areas

Every area is where the built-in map says it is.
```

Checking the built-in list of colors:

```
$ radiocli colors --verify-palette
Walking a color picker on Set Detail Conventional. This stops the scan for a moment.
palette: 147 colors, and the built-in one holds 147
read on: Func, which was White #FFFFFF and still should be

Every color is the one the built-in palette says it is.
```

As JSON, for a program that draws the scanner's screen:

```
$ radiocli colors search -o json
{
  "layout": "search",
  "menu": "Set Search/CC Mode",
  "current": false,
  "areas": [
    {
      "area": "Func",
      "text": "White",
      "background": "Black",
      "textHex": "#FFFFFF",
      "backgroundHex": "#000000",
      "line": 0,
      "column": 0,
      "length": 2,
      "height": 1
    }
  ]
}
```

Pulling out the areas that are not plain white:

```
$ radiocli colors -o json | jq -r '.areas[] | select(.textHex != "#FFFFFF") | "\(.area) \(.text)"'
System_name Orangered
System_option Orangered
System_avoid Orangered
Dept_name Darkorange
Dept_option Darkorange
Dept_avoid Darkorange
Channel_name Gold
Channel_option Gold
Channel_avoid Gold
Option_A_1 Gold
Option_B_1 Gold
Option_A_2 Darksalmon
Option_B_2 Darksalmon
Option_A_3 Darksalmon
Option_B_3 Darksalmon
Option_A_4 Darksalmon
Option_B_4 Darksalmon
Option_A_5 Darksalmon
Option_B_5 Darksalmon
```

A layout that does not exist, refused without touching the scanner:

```
$ radiocli colors bogus
error: "bogus" is not a layout: want simple-conventional, simple-trunk, detail-conventional, detail-trunk, search, weather, tone-out
```

Asking for the current layout while the scanner is in a menu:

```
$ radiocli colors
error: the scanner is in a menu, so it is not drawing with any layout: run "radiocli scan" to put it back to scanning, or name a layout
```

## Changing a color

Setting the system name to cyan and back:

```
$ radiocli colors set System_name --text Cyan
Setting System_name on Set Detail Conventional. This stops the scan for a moment.
layout: detail-conventional (Set Detail Conventional)
area:   System_name
text:   Orangered #FF4600 -> Cyan #00FFFF

$ radiocli colors set System_name --text Orangered
layout: detail-conventional (Set Detail Conventional)
area:   System_name
text:   Cyan #00FFFF -> Orangered #FF4600
```

**The old value is printed because there is no undo.** Setting it back is the
only way to reverse this, so the report is what makes that possible.

Setting a color to the one it already is writes nothing:

```
$ radiocli colors set System_name --text Orangered
text:   Orangered #FF4600 (already)
```

Both colors of one area at once:

```
$ radiocli colors set Channel_name --text Yellow --back Navy
area:   Channel_name
text:   Gold #FFD600 -> Yellow #FFFF00
back:   Black #000000 -> Navy #00007B
```

A color the scanner does not offer, refused without touching it:

```
$ radiocli colors set System_name --text steel
error: "steel" is not a color the scanner offers: did you mean Steelblue, Lightsteelblue?

$ radiocli colors set System_name --text '#FF4600'
error: "#FF4600" is not a color the scanner offers: it offers 147 colors, which are the CSS color names, from Aliceblue to Yellowgreen
```

As JSON:

```
$ radiocli colors set System_name --text Cyan -o json
{
  "layout": "detail-conventional",
  "menu": "Set Detail Conventional",
  "area": "System_name",
  "text": {
    "from": "Orangered",
    "fromHex": "#FF4600",
    "to": "Cyan",
    "toHex": "#00FFFF",
    "changed": true
  }
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `layout`, `menu`, `area` | string | What was changed. |
| `text`, `background` | object | One per color that was asked about. Absent for one that was not. |
| `text.from`, `text.fromHex` | string | What the color was before. |
| `text.to`, `text.toHex` | string | What it is now. |
| `text.changed` | boolean | False when it was already that color, in which case nothing was written. |

### What it does to the scanner

It walks to the area, opens the picker, turns the knob until the picker shows
the color asked for, and presses enter, which is the whole of the write. Then it
reopens the picker and reads it back, so a change that did not take is reported
rather than assumed.

**The knob is the only way to move a picker.** The protocol's own way of writing
a value into a menu item is refused there, by name and by index:

```
error: setting the menu value to "Cyan": scanner rejected the command in its current mode: MSV,Cyan
```

So the walk steps one color at a time. The list wraps, so it goes whichever way
round is shorter and no color is more than 73 steps away. Setting one color
takes about three to five seconds at the default pace, most of it those steps.

If the command fails part way through, the scanner may be left inside a menu.
Run [`scan`](scan.md) to return it to scanning.

## Output

The colors go to stdout. The note about what the command is doing, and debug
logs from `--verbose`, go to stderr.

Under `--output text`, stdout opens with the layout and then an aligned table,
one row per area, in the order the scanner walks them:

```
layout: detail-conventional (Set Detail Conventional)
        the scanner is drawing with this one

AREA            TEXT        HEX      BACKGROUND  HEX
Func            White       #FFFFFF  Black       #000000
[49 more rows]

50 areas
```

The second line appears only when the layout is the one the scanner is drawing
with. Under `--cache` a `cached:` line follows it with when the reading was
taken. A color the scanner does not report is printed as `-`, since a blank cell
reads as a failed reading rather than as an area with nothing set.

Under `--output json`, stdout holds one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `layout` | string | The layout's name, such as `detail-conventional`. |
| `menu` | string | The layout's entry on the scanner's Customize menu, such as `Set Detail Conventional`. |
| `current` | boolean | Whether this is the layout the scanner is drawing with right now. |
| `cached` | boolean | Whether the colors came from the cache rather than from the scanner's menus. The positions never do. |
| `read` | string | When the colors were read off the scanner, as RFC 3339. Absent under `--positions`, which reads none. |
| `areas` | array | One object per area, in the order the scanner walks them. |
| `areas[].area` | string | The scanner's name for the area, such as `System_name`. |
| `areas[].text` | string | The text color's name, which is a CSS color name such as `Orangered`. |
| `areas[].background` | string | The background color's name. |
| `areas[].textHex` | string | The text color as `#RRGGBB`. |
| `areas[].backgroundHex` | string | The background color as `#RRGGBB`. |
| `areas[].line` | number | The topmost row the area occupies, counting from zero at the top. |
| `areas[].column` | number | The first character of the area, counting from zero at the left. |
| `areas[].length` | number | How many characters wide the area is. Zero for an area with no known position. |
| `areas[].height` | number | How many rows tall it is. One for all but nine areas. |

`colors palette` prints a different object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `count` | number | How many colors the scanner offers, which is the length of the ring. |
| `colors` | array | All of them, in the knob's order. |
| `colors[].step` | number | The color's place in the ring, counting from zero, which is also how many turns right it is from the first color. |
| `colors[].name` | string | The scanner's name for it, which is a CSS color name. |
| `colors[].hex` | string | The scanner's own value for it, as `#RRGGBB`. |

Under `--positions` the four color fields are present but empty, since nothing
was read. Everything else is the same shape, so one parser handles both.

The area names are the scanner's own and match the ones its layout editor uses.
An area with no color of its own reports empty strings for its colors.

## How it works out which layout is in use

Two things decide it, and the command asks for them in that order:

1. **What the scanner is doing**, from its status. `conventional_scan` and
   `trunk_scan` each name a pair of layouts; searching, Close Call, weather and
   tone-out each name one.
2. **How many rows the screen has**, which separates the simple layout from the
   detail one: the simple modes draw fourteen and the detail modes seventeen.
   This is a plain read, so it costs nothing and opens nothing.

If the row count is neither, the scanner is asked properly: `MENU → Display
Options → Set Scan Display Mode` shows the current setting as its highlighted
entry. That is a menu walk, and it is the only reason `--positions` or
`--cache` would ever open one. Naming a layout avoids the question entirely.

A scanner that reports a view no layout covers, such as a menu or direct entry,
produces an error rather than a guess. Name a layout, or run [`scan`](scan.md)
first.

A scanner that has just been sent somewhere reports a plain screen for a moment
before it reports what it is doing. The command waits up to three seconds for
that to settle before giving up, so running it straight after a command that
moved the scanner works.

## The two built-in tables, and how to check them

Two things about the scanner are held in tables rather than read from it:

| Table | What it holds | Check |
| ----- | ------------- | ----- |
| The screen map | Where each of the 280 areas sits, across the seven layouts | `--verify-positions` |
| The palette | The 147 colors a picker offers, in the knob's order, with each value | `--verify-palette` |

Both belong to the firmware rather than to you: no menu moves an area or changes
the list of colors. Reading them from the scanner every time would cost seconds
to be told what cannot have changed, and `--positions` would not be instant.

The risk of any table is that a firmware update moves something and nothing says
so. Both checks exist for that, and both exit non-zero when they find a
difference so a script notices. The test suite runs both against a real scanner.

**The two matter differently.** The screen map is used to answer questions
directly, so a wrong entry is a wrong answer, drawn with the right color in the
wrong place. The palette is only used to work out how far and which way to turn
the knob, and [`colors set`](#colors-set) reads the color off the screen and
compares it by name before committing, so a wrong palette makes setting a color
fail rather than write the wrong one. `--verify-palette` catches the slow rot
that would otherwise surface as somebody's set command failing for no apparent
reason.

**The palette check writes nothing**, though it turns the knob across every
color in the list to do its job. It leaves the picker by the menu key rather
than by enter, which abandons the knob's position and keeps the stored color. It
reports which area's picker it borrowed and what that area's color was, so that
this is checkable rather than a claim.

## Why the positions are built in and the colors are not

They are different kinds of thing.

**The colors are yours.** Any of them can be changed from the Customize menu, so
the only honest answer is to go and ask the scanner, every time. That is what
costs thirty seconds.

**The positions are the firmware's.** Nothing in the menus moves an area, so
asking would be asking a question whose answer cannot change. They are held in a
table read off an SDS150 on firmware 1.00.37, which makes `--positions` a lookup
rather than a walk.

That matters for anything drawing the scanner's screen somewhere else: it needs
a position resolved per character per frame, which a menu walk could never
serve.

The risk of a table is that a firmware update moves something and nothing tells
you. The screen would then be drawn with the right colors in the wrong places,
silently. `--verify-positions` is the answer: it walks the editors again and
reports anything that disagrees, and exits non-zero if anything does. The test
suite runs it against a real scanner.

## The soft keys are not in the table

`Soft1_key`, `Space_1`, `Soft2_key`, `Space_2` and `Soft3_key` cannot be, because
their widths follow whatever labels the current mode is showing.

They do not need to be. The bottom row of the live screen is drawn as three runs
of reverse video with a single normal character between them, and those five
regions are the five areas in order. So they are read off the screen at runtime,
and they are reported whenever the layout being asked about is the one on screen.
For any other layout they come back unplaced, with a length of zero.

## Areas taller than one row

Nine areas across the seven layouts occupy more than one row, and `height` says
so:

| Layout | Area | Rows |
| ------ | ---- | ---- |
| `simple-conventional`, `simple-trunk` | `System_name`, `Dept_name`, `Channel_name` | 2 |
| `search`, `weather`, `tone-out` | `Mode_detail_area` | 4 |

The simple layouts draw those three names in a large font that occupies two
rows. Treating one of them as a single row puts half of it in whatever colors
surround it.

## The cache

A full read costs a menu walk of about half a minute, and the answer changes
only when somebody changes it. So every read is written to disk, and `--cache`
hands the last one back.

The file is one JSON object at the platform's cache location, which on macOS is
`~/Library/Caches/radiocli/colors.json`, on Linux `$XDG_CACHE_HOME/radiocli/`
or `~/.cache/radiocli/`, and on Windows under `%LocalAppData%`. Readings are
kept per scanner, keyed by USB serial number where there is one and by port
otherwise, because the colors are that scanner's own settings.

**A plain run never reads the cache.** It always walks the menus and always
overwrites what is stored, which is what keeps a deliberate read the authority
on what the colors are. `--cache` is the opposite promise: it will not open a
picker, so a layout that has never been read is an error rather than a wait.
That is the point of the flag. A caller that wanted to fall back to reading the
scanner can run the plain command instead.

`--cache` still asks the scanner two cheap things:

- **Which layout is meant**, when no layout is named. That is a plain read of
  what the scanner is doing, and the answer changes minute to minute, so it
  could not sensibly be cached. Name a layout to skip it. On a screen whose row
  count is neither fourteen nor seventeen this falls back to one menu, the same
  as `--positions` does; see
  [how it works out which layout is in use](#how-it-works-out-which-layout-is-in-use).
- **Where the soft keys are**, which is read off the live screen because they
  change with what the scanner is showing. Every other position comes from the
  built-in map.

So `--cache` still needs a scanner connected. What it skips is the forty to
fifty pairs of color pickers, which is all of the half minute.

Nothing tells the tool that a color was changed on the scanner itself, by hand
through its own menus. That is why the reading carries when it was taken, in the
`cached:` line and in the JSON `read` field: an old reading is still worth
having, but you get to decide that. Changes made through `colors set` do update
the cache, so only changes made on the radio go unnoticed.

Nothing in the cache cannot be read again from the scanner, so a file that is
missing, damaged, or written by an older version of the tool is treated as
empty rather than as an error, and a cache that cannot be written is a warning
rather than a failed command. That is also why there is no command to manage it:
reading a layout overwrites what is stored, which is the whole of what clearing
one would do, and deleting the file is a `rm` away for anyone who wants the
space back.

Where each area sits is deliberately **not** cached. Positions come from the
built-in map and the live screen, both of which are free, so a stored copy could
only ever be something else to go stale.

## How long it takes

| What | Time | Opens menus |
| ---- | ---- | ----------- |
| `colors palette` | milliseconds | no, and no scanner either |
| `--positions` | milliseconds | no |
| `--cache` | milliseconds | no |
| `--verify-palette` | about seven seconds | yes |
| `--verify-positions` | about ten seconds | yes |
| `colors set` | three to five seconds | yes |
| the full read | about thirty seconds | yes |
| `--all` | about three minutes | yes |

The full read is forty to fifty areas at roughly half a second each, and the
scanner is in its menus and not scanning throughout. There is no faster route:
the colors are not in the protocol, so every one of them costs a walk to a
picker and a screen read.

`--all` is that seven times over: 315 areas on an SDS150, measured at 2 minutes
58 seconds on firmware 1.00.37.

## Reading every layout

`--all` reads all seven layouts in one run and caches each:

```
$ radiocli colors --all
Reading all 7 layouts from the scanner's menus. This takes a few minutes and stops the scan while it runs.
[1/7] Set Simple Conventional
[2/7] Set Simple Trunk
[3/7] Set Detail Conventional
[4/7] Set Detail Trunk
[5/7] Set Search/CC Mode
[6/7] Set Weather Mode
[7/7] Set Tone Out Mode
LAYOUT               MENU                     AREAS  CURRENT
simple-conventional  Set Simple Conventional  40     yes
simple-trunk         Set Simple Trunk         40     -
detail-conventional  Set Detail Conventional  50     -
detail-trunk         Set Detail Trunk         50     -
search               Set Search/CC Mode       45     -
weather              Set Weather Mode         45     -
tone-out             Set Tone Out Mode        45     -

7 layouts read
```

**The layouts you are not looking at are the ones this is for.** Anything
mirroring the display draws whatever the scanner switches to, and a layout that
has never been read has no colors to draw with, so it comes out white on black
while the radio in your hand is in color. Reading only the layout on screen
leaves the other six that way until somebody thinks to run the command again
while looking at each of them in turn, which is not a thing anybody does. So the
`Sync Colors` macro runs this rather than the single-layout read.

It is one run rather than seven for two reasons beyond convenience. The scanner
is taken into its menus once and put back once. And it is one turn on the
scanner, so nothing else can run between two of the layouts, where seven
separate commands would each be a turn somebody else could take.

The progress lines go to stderr as each layout starts, so a run this long says
where it has got to rather than going silent for three minutes.

The text output is a line per layout rather than every area of all seven, which
would be over three hundred rows. What somebody who just waited three minutes
wants to know is that all seven are read; the colors themselves are a
`colors <layout> --cache` away and cost nothing. Under `--output json` the
output is the whole of every reading, as an array of exactly what one layout
prints on its own.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: "<value>" is not a layout: want simple-conventional, ...` | The argument is not one of the seven layouts. The scanner was not touched. | Pass one of the seven names. |
| `error: accepts at most 1 arg(s), received <n>` | More than one layout was given. | Pass one layout, or none for the current one. |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: the scanner is in a menu, so it is not drawing with any layout: ...` | The current layout was asked for while the scanner was in a menu. | Run [`scan`](scan.md), or name a layout. |
| `error: the scanner is showing "<view>", which no layout covers: ...` | The scanner is on a screen none of the seven layouts draws, such as a recording or discovery screen. | Run [`scan`](scan.md), or name a layout. |
| `error: asking the scanner what it is doing: <detail>` | The scanner did not answer the status request. | Run with `--verbose` to see the raw exchange, and check the cable. |
| `error: opening the top menu: <detail>` | The scanner refused to open its menu, which it does while it is busy with something else on screen. | Check what it is showing with [`screen`](screen.md), and clear it before trying again. |
| `error: looking for "<entry>": <detail>` followed by `Run "radiocli scan" to return the scanner to scanning` | The walk could not find a menu entry. The scanner is left inside the menus. | Run [`scan`](scan.md). Then check the entry is still called that on this firmware with [`screen`](screen.md). |
| `error: reading "Set Text Color" for <area>: the screen shows no color value` | A color picker opened but did not show a value. The scanner is left inside the menus. | Run [`scan`](scan.md) and try again. |
| `error: <layout> did not come back round to "<area>" after 200 areas` | The walk never returned to the area it started on, so it stopped rather than spin. | Run [`scan`](scan.md) and try again. If it repeats, the firmware's editor is not walking the way this expects. |
| `error: --positions, --verify-positions and --verify-palette cannot be combined: each asks the scanner a different question` | More than one of those flags was passed. | Pass one. |
| `error: --cache cannot be combined with --positions, --verify-positions or --verify-palette: none of them reads the colors, so there is nothing for the cache to answer` | `--cache` was passed with one of the other three. | Drop `--cache`. Those three are already fast and none of them reads a color. |
| `error: --all reads every layout, so "<value>" cannot be named as well` | A layout was named alongside `--all`. The scanner was not touched. | Drop the layout for all seven, or drop `--all` for that one. |
| `error: --all cannot be combined with --cache, --positions, --verify-positions or --verify-palette: it reads every layout from the scanner, which is the one thing none of those do` | `--all` was passed with one of the other four. The scanner was not touched. | Pass `--all` on its own. |
| `error: reading <entry>: <detail>` | One layout's walk failed during `--all`, named so it is clear which of the seven. The layouts before it were read and cached; the scanner is left inside the menus. | Run [`scan`](scan.md), then run `--all` again. Layouts already read are simply read again. |
| `error: no cached colors for <layout>` followed by `Run "radiocli colors <layout>" to read them from the scanner, which caches them for next time` | `--cache` was asked for a layout that has never been read on this scanner. Nothing was walked. | Run the command without `--cache` once, which reads the layout and stores it. |
| `error: <n> of the scanner's <n> colors are not what the built-in palette says: it was walked off firmware 1.00.37 and this scanner disagrees` | `--verify-palette` found differences, which are listed above the error. | The built-in palette does not describe this scanner. The differences printed are what it should say. |
| `error: the picker stopped moving at <name>: it should come back round to where it started` | The color list did not wrap during `--verify-palette`. | Run [`scan`](scan.md) and try again. If it repeats, this firmware's picker does not behave like the one the palette was read from. |
| `error: <n> of <n> areas of <layout> are not where the built-in map says: the map was read off firmware 1.00.37 and this scanner disagrees` | `--verify-positions` found differences, which are listed above the error. | The built-in map does not describe this scanner. The differences printed are what it should say. |
| `error: unknown command "<value>" for "radiocli colors palette"` | `colors palette` was given an argument. It takes none. | Run it on its own and filter the output. |
| `error: nothing to set: pass --text, --back, or both` | `colors set` was given an area but no color. | Pass `--text`, `--back`, or both. |
| `error: "<value>" is not a color the scanner offers: did you mean <names>?` | The color is not one of the 147, but something close is. The scanner was not touched. | Use one of the suggested names. |
| `error: "<value>" is not a color the scanner offers: it offers 147 colors, which are the CSS color names, from Aliceblue to Yellowgreen` | The color is not one of the 147 and nothing is close. A hex value produces this. | Use a color name. |
| `error: <layout> has no area called "<name>": run "radiocli colors --positions <layout>" for its areas` | The area is not in that layout. The scanner was not touched. | Use a name from the listing. |
| `error: setting "<item>" for <area>: <detail>` followed by `Run "radiocli scan" to return the scanner to scanning` | The write failed part way. The scanner is left inside the menus. | Run [`scan`](scan.md), then read the area with `radiocli colors` to see where it ended up. |
| `error: the color reads back as <name> after setting it to <name>` | The picker was set and the scanner kept something else. | Read the area with `radiocli colors` and set it again. |
| `error: could not get the picker onto <name> after 5 tries` | The knob would not land on the color. | Run [`scan`](scan.md) and try again. If it repeats, the built-in palette does not match this firmware. |
