# Screen map

Where every element of the SDS150's display sits, how that was worked out, and
what it takes to turn a character position on the screen into the color the
radio draws it in.

Read off an SDS150 on firmware 1.00.37, 2026-08-04 and 2026-08-05. Together with
[colors.md](colors.md) this is enough to redraw the scanner's screen faithfully
somewhere else, and [protocol.md](protocol.md) covers the commands the reading is
built on.

The map itself is built into the tool, in
`source/internal/commands/colors/positions.go`, and `radiocli colors
--verify-positions` walks the radio again and reports anything that disagrees.
The tables at the end of this file are that same data in readable form. This
file is the reasoning; the code is the copy that runs.

---

## The screen

Thirty characters wide. Fourteen rows in Simple Mode, seventeen in Detail Mode,
counting from zero at the top left. The last row is always the three soft key
labels.

Every character belongs to one named area, and every area has one text color and
one background color, stored on the radio and editable under
`MENU -> Display Options -> Customize`.

Areas are the firmware's, not the user's. Nothing in the menus moves one or
changes its size, and only the colors are settable. That asymmetry is the whole
reason the geometry can live in a table while the colors have to be read from
the radio every time.

## Seven layouts, three geometries

The Customize menu offers seven layout editors. They do not describe seven
different screens:

| Editors | Areas | Distinct geometry |
| ------- | ----- | ----------------- |
| Set Simple Conventional, Set Simple Trunk | 40 | **Simple** |
| Set Detail Conventional, Set Detail Trunk | 50 | **Detail** |
| Set Search/CC Mode, Set Weather Mode, Set Tone Out Mode | 45 | **Search** |

Within each group the positions are identical, area for area. The conventional
and trunk editors of a pair differ only in the colors somebody has set on them,
which is the reason they are separate editors at all: the same screen, two color
schemes, one for each kind of system.

Knowing that before mapping anything cuts the work from seven walks to three.
Two of the seven confirmed what the other five already said.

## How the positions were read

The layout editor draws the layout's own artwork rather than a list of names,
and draws the selected area in reverse video. `STS` reports the attribute string
one character per screen character, so the selected area is the run of `*`:

```
text: " SYSTEM      DEPT     CHANNEL"
attr: "********* ********** *********"
```

Stepping the knob through the editor and recording that run for each area gives
`area -> (line, column, length)` directly, at about 18 seconds per layout. The
span is stable rather than blinking, so one read per area is enough.

The approach rests on the editor's geometry matching the live screen's line for
line. That is the load-bearing assumption in this entire file, and it was
checked rather than assumed: see the live reading below.

An early attempt to do this through `radiocli screen -o json` found nothing,
because that command was throwing the attribute string away and emitting only a
per-line `highlighted` boolean. It now emits `attributes`, so the raw `STS` is
no longer needed:

```json
{
  "text": " SYSTEM      DEPT     CHANNEL",
  "highlighted": true,
  "attributes": "********* ********** *********"
}
```

## Height was missed on the first pass

The first map recorded only where each area _started_. That is wrong for nine
areas, because `DSP_FORM` marks some rows as large font and a large-font row
occupies two rows of the display:

| Layout | Area | Rows |
| ------ | ---- | ---- |
| Simple | `System_name`, `Dept_name`, `Channel_name` | 2 |
| Search | `Mode_detail_area` | 4 |

Everything else is one row.

An area read as one row tall when it is two paints half of itself in whatever
colors happen to surround it, and nothing reports the error. The check that
walks the editors and compares caught this on its first run, which is a fair
argument for writing the check before trusting the table rather than after.
Every one of the 280 line, column and length values from the first pass
survived. Only height was missing.

## The soft keys are not in the table

`Soft1_key`, `Space_1`, `Soft2_key`, `Space_2` and `Soft3_key` have no entry.

The bottom row is drawn entirely in reverse video at all times, so in the editor
the selection cannot be told apart from the row itself. There is no run to
record.

They do not need one, because the live row describes its own layout as three
runs of `*` separated by single-character gaps:

```
text: " SYSTEM      DEPT     CHANNEL"
attr: "********* ********** *********"

Soft1_key  reverse  col  0  len  9   " SYSTEM  "
Space_1    gap      col  9  len  1   " "
Soft2_key  reverse  col 10  len 10   "   DEPT   "
Space_2    gap      col 20  len  1   " "
Soft3_key  reverse  col 21  len  9   " CHANNEL "
```

Five regions, in the order the five areas are named. Splitting the bottom row on
its runs at runtime gives the three buttons and their widths, whatever labels the
current mode is showing, and it works across every layout without a table. The
table approach would have needed this anyway: the labels change with the mode,
so their widths do too.

## Turning a position into a color

Read the screen with `GST`, keeping the raw attribute strings. Identify the
layout from `V_Screen` in `GSI` plus the number of lines, because neither alone
is enough: the line count separates Simple from Detail but not conventional from
trunk, and `V_Screen` separates scanning from searching but not simple from
detail. For each character at `(line, column)`, find the area whose span contains
it, where spans are `[column, column+length)` across `[line, line+height)`. Draw
it in that area's text color on its background color. Where the attribute
character is `*`, swap the two; where it is `_`, underline.

That last step has a trap in it. The attribute string can be longer than the
trimmed text, because a highlight bar is drawn across the rest of the row. Pad
the text to the attribute length before indexing into both, or the highlight
runs off the end of what you are indexing.

## Verified against a live screen

Detail Conventional, scanning a conventional system, every position resolved
against what the radio was actually showing:

```
AREA             POSITION      COLOR                  LIVE CONTENT
Option_3         L0 c14+6      White #FFFFFF          'Aug.4'
Option_4         L0 c20+6      White #FFFFFF          '19:22'
Info_area_1      L1 c0+14      White #FFFFFF          'F0:----------'
Info_area_2      L2 c0+16      White #FFFFFF          'S0:----------'
Option_C_1       L2 c16+14     White #FFFFFF          'VOL:15 SQL: 2'
Info_area_3      L3 c0+16      White #FFFFFF          'D0:----------'
Option_C_2       L3 c16+14     White #FFFFFF          'Tag:--.--.---'
System_name      L4 c0+24      Orangered #FF4600      'PUBLIC SAFETY'
System_option    L5 c0+26      Orangered #FF4600      'GREENDALE, ST 00000'
Dept_name        L6 c0+24      Darkorange #FF8800     'POLICE DEPARTMENT'
Channel_name     L8 c0+24      Gold #FFD600           'Scanning...'
Option_A_2       L11 c0+16     Darksalmon #E79473     'Sys ID: ---'
Option_B_2       L11 c16+14    Darksalmon #E79473     'TGID: ---'
Option_A_3       L12 c0+16     Darksalmon #E79473     'RFSS ID: ---'
Option_B_3       L12 c16+14    Darksalmon #E79473     'Site ID: ---'
Option_A_4       L13 c0+16     Darksalmon #E79473     'WACN: ---'
Option_B_4       L13 c16+14    Darksalmon #E79473     'Batt:4.18V'
Option_A_5       L14 c0+16     Darksalmon #E79473     'UID: ---'
Option_B_5       L14 c16+14    Darksalmon #E79473     'RSSI: ---'
```

Content lands where the map says it should, in every case. The colors in that
listing are one radio's settings and are not part of the map; read them live
with `radiocli colors`.

## Two things the tables cannot tell you

The editor's labels are placeholders. An area shows `Frequency` or `Number Tag`
in the editor regardless of what it displays while scanning, so the names below
say what an area is _for_, not what it currently holds.

Blank areas still occupy their span. An area showing nothing is still claiming
its rectangle, and a character position inside it still resolves to its colors.

---

# The map

Line, column and length are characters, counting from zero. Height is rows.
Colors are deliberately absent: they are per-radio settings, not geometry.

## Simple

Fourteen rows. Used by **Set Simple Conventional** and **Set Simple Trunk**,
which are identical in position. 40 areas, 35 with a position.

| Area | Line | Col | Len | Height |
| ---- | ---- | --- | --- | ------ |
| Func | 0 | 0 | 2 | 1 |
| Option_1 | 0 | 2 | 6 | 1 |
| Option_2 | 0 | 8 | 6 | 1 |
| Option_3 | 0 | 14 | 6 | 1 |
| Option_4 | 0 | 20 | 6 | 1 |
| Signal_meter | 0 | 26 | 2 | 1 |
| Battery | 0 | 28 | 2 | 1 |
| Space_0 | 1 | 0 | 2 | 1 |
| Option_5 | 1 | 2 | 6 | 1 |
| Option_6 | 1 | 8 | 6 | 1 |
| Option_7 | 1 | 14 | 6 | 1 |
| Option_8 | 1 | 20 | 6 | 1 |
| Key_lock | 1 | 26 | 2 | 1 |
| Direction | 1 | 28 | 2 | 1 |
| System_name | 2 | 0 | 24 | **2** |
| System_option | 4 | 0 | 26 | 1 |
| System_avoid | 4 | 26 | 4 | 1 |
| Dept_name | 5 | 0 | 24 | **2** |
| Dept_option | 7 | 0 | 26 | 1 |
| Dept_avoid | 7 | 26 | 4 | 1 |
| Channel_name | 8 | 0 | 24 | **2** |
| Channel_option | 10 | 0 | 26 | 1 |
| Channel_avoid | 10 | 26 | 4 | 1 |
| Option_A | 11 | 0 | 16 | 1 |
| Option_B | 11 | 16 | 14 | 1 |
| Icon_1 | 12 | 0 | 3 | 1 |
| Icon_2 | 12 | 3 | 3 | 1 |
| Icon_3 | 12 | 6 | 3 | 1 |
| Icon_4 | 12 | 9 | 3 | 1 |
| Icon_5 | 12 | 12 | 3 | 1 |
| Icon_6 | 12 | 15 | 3 | 1 |
| Icon_7 | 12 | 18 | 3 | 1 |
| Icon_8 | 12 | 21 | 3 | 1 |
| Icon_9 | 12 | 24 | 3 | 1 |
| Icon_a | 12 | 27 | 3 | 1 |

Row 13 is the soft keys, read from the live row.

## Detail

Seventeen rows. Used by **Set Detail Conventional** and **Set Detail Trunk**,
which are identical in position. 50 areas, 45 with a position.

The extra three rows at the top are `Info_area_1` through `Info_area_3` with
their companions, and the system, department and channel names are ordinary
height here rather than large font. That is what buys room for five rows of
options at the bottom instead of one.

| Area | Line | Col | Len | Height |
| ---- | ---- | --- | --- | ------ |
| Func | 0 | 0 | 2 | 1 |
| Option_1 | 0 | 2 | 6 | 1 |
| Option_2 | 0 | 8 | 6 | 1 |
| Option_3 | 0 | 14 | 6 | 1 |
| Option_4 | 0 | 20 | 6 | 1 |
| Signal_meter | 0 | 26 | 2 | 1 |
| Battery | 0 | 28 | 2 | 1 |
| Info_area_1 | 1 | 0 | 14 | 1 |
| Option_7 | 1 | 14 | 6 | 1 |
| Option_8 | 1 | 20 | 6 | 1 |
| Key_lock | 1 | 26 | 2 | 1 |
| Direction | 1 | 28 | 2 | 1 |
| Info_area_2 | 2 | 0 | 16 | 1 |
| Option_C_1 | 2 | 16 | 14 | 1 |
| Info_area_3 | 3 | 0 | 16 | 1 |
| Option_C_2 | 3 | 16 | 14 | 1 |
| System_name | 4 | 0 | 24 | 1 |
| System_option | 5 | 0 | 26 | 1 |
| System_avoid | 5 | 26 | 4 | 1 |
| Dept_name | 6 | 0 | 24 | 1 |
| Dept_option | 7 | 0 | 26 | 1 |
| Dept_avoid | 7 | 26 | 4 | 1 |
| Channel_name | 8 | 0 | 24 | 1 |
| Channel_option | 9 | 0 | 26 | 1 |
| Channel_avoid | 9 | 26 | 4 | 1 |
| Option_A_1 | 10 | 0 | 16 | 1 |
| Option_B_1 | 10 | 16 | 14 | 1 |
| Option_A_2 | 11 | 0 | 16 | 1 |
| Option_B_2 | 11 | 16 | 14 | 1 |
| Option_A_3 | 12 | 0 | 16 | 1 |
| Option_B_3 | 12 | 16 | 14 | 1 |
| Option_A_4 | 13 | 0 | 16 | 1 |
| Option_B_4 | 13 | 16 | 14 | 1 |
| Option_A_5 | 14 | 0 | 16 | 1 |
| Option_B_5 | 14 | 16 | 14 | 1 |
| Icon_1 | 15 | 0 | 3 | 1 |
| Icon_2 | 15 | 3 | 3 | 1 |
| Icon_3 | 15 | 6 | 3 | 1 |
| Icon_4 | 15 | 9 | 3 | 1 |
| Icon_5 | 15 | 12 | 3 | 1 |
| Icon_6 | 15 | 15 | 3 | 1 |
| Icon_7 | 15 | 18 | 3 | 1 |
| Icon_8 | 15 | 21 | 3 | 1 |
| Icon_9 | 15 | 24 | 3 | 1 |
| Icon_a | 15 | 27 | 3 | 1 |

Row 16 is the soft keys, read from the live row.

## Search

Seventeen rows. Used by **Set Search/CC Mode**, **Set Weather Mode** and **Set
Tone Out Mode**, which are identical in position. 45 areas, 40 with a position.

The top four rows match Detail exactly. Below that it diverges: there is no
system or department, and `Mode_detail_area` takes four rows in the middle,
which is where the search or tone out display puts its working.

| Area | Line | Col | Len | Height |
| ---- | ---- | --- | --- | ------ |
| Func | 0 | 0 | 2 | 1 |
| Option_1 | 0 | 2 | 6 | 1 |
| Option_2 | 0 | 8 | 6 | 1 |
| Option_3 | 0 | 14 | 6 | 1 |
| Option_4 | 0 | 20 | 6 | 1 |
| Signal_meter | 0 | 26 | 2 | 1 |
| Battery | 0 | 28 | 2 | 1 |
| Info_area_1 | 1 | 0 | 14 | 1 |
| Option_7 | 1 | 14 | 6 | 1 |
| Option_8 | 1 | 20 | 6 | 1 |
| Key_lock | 1 | 26 | 2 | 1 |
| Direction | 1 | 28 | 2 | 1 |
| Info_area_2 | 2 | 0 | 16 | 1 |
| Option_C_1 | 2 | 16 | 14 | 1 |
| Info_area_3 | 3 | 0 | 16 | 1 |
| Option_C_2 | 3 | 16 | 14 | 1 |
| Primary_area_1 | 4 | 0 | 24 | 1 |
| Primary_area_2 | 5 | 0 | 24 | 1 |
| Primary_area_3 | 6 | 0 | 24 | 1 |
| Sub_info_area | 7 | 0 | 16 | 1 |
| Modulation | 7 | 16 | 6 | 1 |
| Avoid | 7 | 22 | 4 | 1 |
| Hold | 7 | 26 | 4 | 1 |
| Mode_detail_area | 8 | 0 | 30 | **4** |
| Option_A_1 | 12 | 0 | 16 | 1 |
| Option_B_1 | 12 | 16 | 14 | 1 |
| Option_A_2 | 13 | 0 | 16 | 1 |
| Option_B_2 | 13 | 16 | 14 | 1 |
| Option_A_3 | 14 | 0 | 16 | 1 |
| Option_B_3 | 14 | 16 | 14 | 1 |
| Icon_1 | 15 | 0 | 3 | 1 |
| Icon_2 | 15 | 3 | 3 | 1 |
| Icon_3 | 15 | 6 | 3 | 1 |
| Icon_4 | 15 | 9 | 3 | 1 |
| Icon_5 | 15 | 12 | 3 | 1 |
| Icon_6 | 15 | 15 | 3 | 1 |
| Icon_7 | 15 | 18 | 3 | 1 |
| Icon_8 | 15 | 21 | 3 | 1 |
| Icon_9 | 15 | 24 | 3 | 1 |
| Icon_a | 15 | 27 | 3 | 1 |

Row 16 is the soft keys, read from the live row.

---

## Still open

**Whether this holds on another radio.** Everything here is one SDS150 on one
firmware. The geometry ought to be the firmware's rather than the unit's, and
that has not been tested against a second scanner, let alone an SDS100.

**What the remaining editor-only areas are for.** Several `Option_` areas were
never seen holding anything on this radio's configuration.

**Whether any mode uses a layout that is not one of these seven.** The menu
offers seven editors, which is not proof that the firmware has only seven
screens.
