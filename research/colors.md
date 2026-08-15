# Display colors

What the SDS150 draws its screen in, where those values live, how to read and
write them, and which of them mean anything.

Read off an SDS150 on firmware 1.00.37, 2026-08-04 and 2026-08-05. The palette is
the firmware's and should hold for any SDS150. The per-area assignments in the
second half are one scanner's settings rather than a specification. They look
like factory defaults, and one layout has been confirmed as such; the other six
have not.

With [screen-map.md](screen-map.md) for where each area sits, this is enough to
redraw the scanner's screen faithfully somewhere else. [protocol.md](protocol.md)
covers what the serial port will and will not tell you. Colors are the largest
thing it will not.

All of it is in the tool now, and the tool is what you should ask:

| Question | Command | Cost |
| -------- | ------- | ---- |
| What colors is this layout drawn in? | `colors [layout]` | ~30s, opens menus |
| Where does each area sit? | `colors --positions` | instant, no menus |
| Change one area's color | `colors set <area> --text <name> --back <name>` | ~4s, writes |
| Is the screen map still right? | `colors --verify-positions` | ~10s |
| Is the color list still right? | `colors --verify-palette` | ~7s |
| Are colors even switched on? | `display` | instant |

This file is the reasoning behind those commands and the record of what the
walks found.

---

## Read `COLOR_MODE` before you trust anything else here

A global setting at `MENU -> Display Options -> Set B/W or Color Mode` decides
whether the radio draws colors at all:

| Mode | What is drawn | `COLOR_MODE` |
| ---- | ------------- | ------------ |
| Color Mode | The per-area colors below | `0` |
| Black w/White Text | White on black, colors ignored | `1` |
| White w/Black Text | Black on white, colors ignored | `2` |

This one is readable over serial, unlike the colors themselves. It is the
`COLOR_MODE` field of `GST`, eleventh after the last line. The published spec
labels it "Waterfall display only," which is wrong: it tracks the global setting
with the waterfall never opened. Verified by selecting each of the three entries
and reading `GST` in between.

In modes `1` and `2` everything else in this file is still stored and completely
ignored, and the correct rendering is plain white on black or black on white. A
renderer that skips this check draws a color display for somebody holding a
monochrome radio, and nothing anywhere reports the mistake.

Setting it is a `KEY` walk through Display Options, since that menu has no `MNU`
id. The cursor opens on whichever entry is active, which is a second way to read
it.

## The protocol does not carry the colors

Every field in the remote command protocol was read looking for them. The only
color-adjacent one is a vestigial backlight setting from the BCD436HP era that
reads `OFF` on this model and always will.

The colors live on the radio, reachable through its menus, under
`MENU -> Display Options -> Customize`. Each area is a menu titled
`Set <Area> Area` holding `Select Option Item`, `Set Text Color` and
`Set Back Color`, and the picker screen shows the current value as a name plus
`RGB = xxxxxxh`.

So they are readable after all, by walking the menus and reading each picker off
the screen with `STS`. The full walk is 315 areas across 7 layouts, about 3
minutes 45 at `--pace turbo`. The tool does one layout in about thirty seconds.
Its output was checked against the tables at the end of this file for two
layouts and matched area for area and value for value, which is what says the
tool's walk and the walk that produced this file are doing the same thing.

## All 315 are also sitting on the memory card in plain text

*2026-08-05.* In one 15 KB file. The card holds `BCDx36HP/profile.cfg`, and in
it are 46 tab separated `DispColors` records, each carrying a run of six-digit
hex pairs:

```
DispColors  DispColorId=1  ColorLayoutId=1  ff4600 000000  ff4600 000000  ff8800 000000 ...
```

Each pair is one area's text color and background. They total exactly 315, and
they split by layout as 40, 50, 45, 45, 45, 40, 50, which is the seven layouts'
area counts. The values are the same ones the menu walk returns. Whoever at
Uniden decided this file should be human-readable text saved everyone who comes
after them about four minutes per read, forever.

This is the fastest way to get the colors by a wide margin, and the only way to
get all seven layouts at once. The catch governs everything about the card: it
is mass storage or serial, never both. Reading this file means the radio is a
disk and is not answering commands, so nothing can render a live screen from it
without having cached it from an earlier session. It is a better source for what
the colors are and no help at all for what the screen is doing.

Read it with `radiocli backup`, which copies the card, or straight off the
mounted volume.

### The card caught an error the menu walk had made

The counts disagreed by one area: 211 White and one Blue from the card, against
the walk's 212 White. Re-reading the radio settled it, and the card was right.
`Option_1` on the simple trunk layout really is Blue `#0000FF`, and the original
walk recorded it as White.

That mattered well beyond the one value, because it had already been
generalized. These notes described the two simple layouts as identical, and they
are not. A single source is how a bad reading calcifies into documentation. Two
independent sources is how it gets caught, and a third read is what settles it.

## Which layout is on screen, without opening a menu

Two reads:

| Question | Source |
| -------- | ------ |
| Which mode family? | `V_Screen` from `GSI` |
| Simple or detail? | How many rows the screen has |

Simple Mode is 14 lines, Detail Mode is 17, measured by setting each mode and
counting. `V_Screen` separates `conventional_scan` from `trunk_scan` from
`tone_out` and the rest, but says nothing about simple versus detail. Together
they are unique, and both are plain reads, which is what keeps `--positions`
instant.

`MENU -> Display Options -> Set Scan Display Mode` shows the same answer as its
highlighted entry, and is the fallback when the row count is neither 14 nor 17.

## The palette is 147 colors in a ring

Walked end to end on 2026-08-05, from one picker, reading each color off the
screen rather than from any listing.

There are 147 of them, alphabetical from `Aliceblue` to `Yellowgreen`, and the
list wraps: stepping right past `Yellowgreen` lands on `Aliceblue`, so a walk can
choose its direction and nothing is more than 73 steps away. All 147 values are
distinct, so a name maps cleanly to a color and back. An arbitrary hex cannot be
set, because there is nothing to type one into. The picker is the only way in,
which makes those 147 the entire vocabulary.

### The names are CSS, the values are not

An earlier version of this file said the scanner's vocabulary "maps straight onto
web colors." That is wrong in two directions.

The values drift from CSS by up to 7 per channel:

| Name | Scanner | Real CSS | Off by |
| ---- | ------- | -------- | ------ |
| Orangered | `#FF4600` | `#FF4500` | 1 |
| Gold | `#FFD600` | `#FFD700` | 1 |
| Orchid | `#D66FD6` | `#DA70D6` | 4 |
| Darkorange | `#FF8800` | `#FF8C00` | 4 |
| Tomato | `#FF6342` | `#FF6347` | 5 |
| Darksalmon | `#E79473` | `#E9967A` | 7 |
| Steelblue | `#4280AD` | `#4682B4` | 7 |

The names are not all CSS either. Seven are not CSS names at all, one value
flatly contradicts its name, and eight CSS names are missing, including every
British `grey` spelling. See [oddities.md](oddities.md).

There are 54 distinct levels per channel across the palette, so the display
quantizes color somehow. It is not a clean RGB565 grid: only 63 of the 147
values sit on one. The scheme is unexplained and probably does not matter. What
matters is that a name here labels the _scanner's_ color rather than the web's.
Render `Steelblue` at the web's value and you are wrong in all three channels,
with nothing to tell you.

### All 147, in the knob's order

Read off the scanner. The order is what the knob steps through: alphabetical,
wrapping from `Yellowgreen` back to `Aliceblue`.

| Color | Hex | Color | Hex | Color | Hex |
| ----- | --- | ----- | --- | ----- | --- |
| Aliceblue | `#EFF7FF` | Antiquewhite | `#F7EBD6` | Aqua | `#00FBF7` |
| Aquamarine | `#7BFFCE` | Azure | `#EFFFFF` | Beige | `#EFF3D6` |
| Bisque | `#FFE3BD` | Black | `#000000` | Blanchedalmond | `#FFEBC6` |
| Blue | `#0000FF` | Blueviolet | `#8429DE` | Brass | `#B5A542` |
| Brown | `#A52929` | Burlywood | `#D6B584` | Cadetblue | `#5A9C9C` |
| Chartreuse | `#7BFF00` | Chocolate | `#CE6718` | Coolcopper | `#D68418` |
| Copper | `#BD00DE` | Coral | `#FF7F4A` | Cornflower | `#BDEFDE` |
| Cornflowerblue | `#6390E7` | Cornsilk | `#FFF7D6` | Crimson | `#D61039` |
| Cyan | `#00FFFF` | Darkblue | `#000084` | Darkbrown | `#D60800` |
| Darkcyan | `#008884` | Darkgoldenrod | `#B58408` | Darkgray | `#A5A5A5` |
| Darkgreen | `#006300` | Darkkhaki | `#B5B56B` | Darkmagenta | `#840084` |
| Darkolivegreen | `#526B29` | Darkorange | `#FF8800` | Darkorchid | `#9431C6` |
| Darkred | `#840000` | Darksalmon | `#E79473` | Darkseagreen | `#8CB98C` |
| Darkslateblue | `#423D84` | Darkslategray | `#294E4A` | Darkturquoise | `#00CACE` |
| Darkviolet | `#8C00CE` | Deeppink | `#FF108C` | Deepskyblue | `#00BDFF` |
| Dimgray | `#636763` | Dodgerblue | `#188CFF` | Feldsper | `#F7CEDE` |
| Firebrick | `#AD2121` | Floralwhite | `#FFF7EF` | Forestgreen | `#218821` |
| Fuchsia | `#F700F7` | Gainsboro | `#D6DAD6` | Ghostwhite | `#F7F7FF` |
| Gold | `#FFD600` | Goldenrod | `#D6A118` | Gray | `#7B7F7B` |
| Green | `#007F00` | Greenyellow | `#ADFF29` | Honeydew | `#EFFFEF` |
| Hotpink | `#FF67AD` | Indianred | `#C65A5A` | Indigo | `#4A007B` |
| Ivory | `#FFFFEF` | Khaki | `#EFE38C` | Lavender | `#DEE3F7` |
| Lavenderblush | `#FFEFEF` | Lawngreen | `#7BFB00` | Lemonchiffon | `#FFF7C6` |
| Lightblue | `#ADD6DE` | Lightcoral | `#EF7F7B` | Lightcyan | `#DEFFFF` |
| Lightgoldenrodyellow | `#F7F7CE` | Lightgreen | `#8CEB8C` | Lightgrey | `#CED2CE` |
| Lightpink | `#FFB1BD` | Lightsalmon | `#FF9C73` | Lightseagreen | `#18ADA5` |
| Lightskyblue | `#84CAF7` | Lightslategray | `#738494` | Lightsteelblue | `#ADC2D6` |
| Lightyellow | `#FFFFDE` | Lime | `#00FF00` | Limegreen | `#31CA31` |
| Linen | `#F7EFDE` | Magenta | `#FF00FF` | Maroon | `#7B0000` |
| Mediumaquamarine | `#63CAA5` | Mediumblue | `#0000C6` | Mediumorchid | `#B556CE` |
| Mediumpurple | `#8C6FD6` | Mediumseagreen | `#39B16B` | Mediumslateblue | `#7367E7` |
| Mediumspringgreen | `#00F794` | Mediumturquoise | `#42CEC6` | Mediumvioletred | `#C61484` |
| Midnightblue | `#18186B` | Mintcream | `#EFFFF7` | Mistyrose | `#FFE3DE` |
| Moccasin | `#FFE3B5` | Navajowhite | `#FFDAAD` | Navy | `#00007B` |
| Oldlace | `#F7F3DE` | Olive | `#7B7F00` | Olivedrab | `#6B8C21` |
| Orange | `#FFA100` | Orangered | `#FF4600` | Orchid | `#D66FD6` |
| Palegoldenrod | `#E7E7A5` | Palegreen | `#94FB94` | Paleturquoise | `#ADEBE7` |
| Palevioletred | `#D66F8C` | Papayawhip | `#FFEFCE` | Peachpuff | `#FFD6B5` |
| Peru | `#C68039` | Pink | `#FFBDC6` | Plum | `#D69CD6` |
| Powderblue | `#ADDEDE` | Purple | `#7B007B` | Red | `#FF0000` |
| Richblue | `#08ADDE` | Rosybrown | `#B58C8C` | Royalblue | `#3967DE` |
| Saddlebrown | `#844610` | Salmon | `#F77F6B` | Sandybrown | `#EFA15A` |
| Seagreen | `#298852` | Seashell | `#FFF3E7` | Sienna | `#9C5229` |
| Silver | `#BDBDBD` | Skyblue | `#84CAE7` | Slateblue | `#635AC6` |
| Slategray | `#6B7F8C` | Snow | `#FFF7F7` | Springgreen | `#00FF7B` |
| Steelblue | `#4280AD` | Tan | `#CEB18C` | Teal | `#007F7B` |
| Thistle | `#D6BDD6` | Tomato | `#FF6342` | Turquoise | `#39DECE` |
| Violet | `#E780E7` | Wheat | `#EFDAAD` | White | `#FFFFFF` |
| Whitesmoke | `#EFF3EF` | Yellow | `#FFFF00` | Yellowgreen | `#94CA31` |

## Writing a color is `KEY` presses and nothing else

Proven on 2026-08-05 by moving `System_name` from `Orangered` to `Orchid`,
reading it back, and putting it back.

Pressing enter in the picker is the whole of the write. It commits and closes.
Leaving with the menu key abandons the knob's position, so a walk that moves
across every color and then backs out writes nothing, which is the property that
makes `--verify-palette` safe to run. That is checked rather than assumed: the
check reports which area's picker it borrowed and what color was there, and the
test sets that area to that same color and confirms nothing changed. Reopening
the picker reads back what was written, so the write is verifiable.

### `MSV` is refused inside a picker

The protocol's own way of writing a value into the menu item the scanner is on
does not work here, by name or by index:

```
-> MSV,Cyan   <- rejected in its current mode
-> MSV,22     <- rejected in its current mode
```

Turning the knob is the only way to move a picker, one color at a time. That is
why setting a color takes seconds rather than milliseconds, and it is the single
largest cost in the command. The ring wrapping is what caps it: worst case 73
steps, not 146.

## `MSI` truncates without saying so

| Menu | Real size | What `MSI` lists |
| ---- | --------- | ---------------- |
| A layout editor | up to 50 areas | 20 |
| A color picker | 147 colors | 26 |
| The Customize menu | 8 entries | nothing at all |

An early attempt to find a color's position from the picker's listing failed for
this reason, and would have silently mis-stepped had it not also been wrong
about the current color, which the listing does not mark at all. Silent
truncation is the worst shape a wrong answer can take, because everything looks
fine until it isn't. Anything walking a long list has to read the screen.

## Where the areas are

Each area owns a rectangle, so any character position on the screen resolves to
an area and therefore to a color. That map is in
[screen-map.md](screen-map.md), and `colors --positions` prints it instantly.

The part worth repeating here: nine areas are more than one row tall, because
the scanner draws some names in a large font that occupies two rows. Treating
one as a single row paints half of it in whatever colors surround it, and
nothing reports the error.

## A dead end worth keeping: `DSP_FORM` fingerprinting

Before the row count was measured, the plan was to identify a layout by the
shape of its `DSP_FORM`. It does not work, because the editors share
fingerprints:

| Editor form | Layouts sharing it |
| ----------- | ------------------ |
| `00110110110000` | Set Simple Conventional, Set Simple Trunk |
| `00001010100000000` | Set Detail Conventional, Set Detail Trunk |
| `00001110000000000` | Set Search/CC Mode, Set Weather Mode, Set Tone Out Mode |

Those are the forms of the _editor_ screens rather than the live ones, so they
were never conclusive to begin with. The grouping turned out to be the useful
part: it is the same three-way split the geometry has, and it was the first hint
that the seven layouts are really three.

---

# The colors on this radio

Every one of the 315 backgrounds is Black `#000000`, and 211 of the 315 text
colors are White `#FFFFFF`. Only the exceptions are listed below. Anything
absent from a table is white on black.

These counts are corroborated twice, by walking the menus over serial and by
reading the radio's own configuration file off its memory card, which agrees
area for area.

Seven of the 147 colors are used at all:

| Color | Hex | Areas |
| ----- | --- | ----- |
| White | `#FFFFFF` | 211 |
| Darksalmon | `#E79473` | 38 |
| Gold | `#FFD600` | 34 |
| Orangered | `#FF4600` | 18 |
| Darkorange | `#FF8800` | 12 |
| Blue | `#0000FF` | 1 |
| Tomato | `#FF6342` | 1 |

Two of those are used once each, and both are on trunk layouts.

## Color encodes depth in the memory hierarchy

Warm to bright, as you go down:

| Level | Color | Hex |
| ----- | ----- | --- |
| System | Orangered | `#FF4600` |
| Department | Darkorange | `#FF8800` |
| Channel | Gold | `#FFD600` |
| Channel options | Darksalmon | `#E79473` |
| Func indicator, detail trunk only | Tomato | `#FF6342` |
| `Option_1`, simple trunk only | Blue | `#0000FF` |
| Everything else | White | `#FFFFFF` |

Glance at a line and its color tells you what level of the hierarchy you are
looking at before you have read a word of it. That is the part worth reproducing
in any UI that redraws this screen, and it is a genuinely good piece of design.
The other 211 areas are plain white on black and carry no meaning in their
color.

The two single-use colors are not part of the scheme and look like somebody's
edit rather than a decision: one area on each trunk layout, picked out in a
color used nowhere else.

## Simple

**Set Simple Conventional** and **Set Simple Trunk**, which differ in one area:
`Option_1` is White on the conventional editor and Blue `#0000FF` on the trunk
one. That is the only use of Blue on the radio.

| Area | Text | Hex |
| ---- | ---- | --- |
| Option_1 | White, or Blue on trunk | `#FFFFFF` / `#0000FF` |
| System_name | Orangered | `#FF4600` |
| System_option | Orangered | `#FF4600` |
| System_avoid | Orangered | `#FF4600` |
| Dept_name | Darkorange | `#FF8800` |
| Dept_option | Darkorange | `#FF8800` |
| Dept_avoid | Darkorange | `#FF8800` |
| Channel_name | Gold | `#FFD600` |
| Channel_option | Gold | `#FFD600` |
| Channel_avoid | Gold | `#FFD600` |
| Option_A | Darksalmon | `#E79473` |
| Option_B | Darksalmon | `#E79473` |

The other 28 areas are white on black.

## Detail

**Set Detail Conventional** and **Set Detail Trunk**, which differ in one area:
`Func` is White on the conventional editor and Tomato `#FF6342` on the trunk
one. That single area is the only place in 315 where the conventional and trunk
pairs disagree, and it is the only use of Tomato on the radio.

| Area | Text | Hex |
| ---- | ---- | --- |
| Func | White, or Tomato on trunk | `#FFFFFF` / `#FF6342` |
| System_name | Orangered | `#FF4600` |
| System_option | Orangered | `#FF4600` |
| System_avoid | Orangered | `#FF4600` |
| Dept_name | Darkorange | `#FF8800` |
| Dept_option | Darkorange | `#FF8800` |
| Dept_avoid | Darkorange | `#FF8800` |
| Channel_name | Gold | `#FFD600` |
| Channel_option | Gold | `#FFD600` |
| Channel_avoid | Gold | `#FFD600` |
| Option_A_1 | Gold | `#FFD600` |
| Option_B_1 | Gold | `#FFD600` |
| Option_A_2 | Darksalmon | `#E79473` |
| Option_B_2 | Darksalmon | `#E79473` |
| Option_A_3 | Darksalmon | `#E79473` |
| Option_B_3 | Darksalmon | `#E79473` |
| Option_A_4 | Darksalmon | `#E79473` |
| Option_B_4 | Darksalmon | `#E79473` |
| Option_A_5 | Darksalmon | `#E79473` |
| Option_B_5 | Darksalmon | `#E79473` |

The other 30 areas are white on black. The first row of options is Gold rather
than Darksalmon, which groups it with the channel above rather than with the
options below.

## Search

**Set Search/CC Mode**, **Set Weather Mode** and **Set Tone Out Mode**, which are
identical.

| Area | Text | Hex |
| ---- | ---- | --- |
| Primary_area_1 | Orangered | `#FF4600` |
| Primary_area_2 | Orangered | `#FF4600` |
| Primary_area_3 | Gold | `#FFD600` |
| Sub_info_area | Gold | `#FFD600` |
| Modulation | Gold | `#FFD600` |
| Avoid | Gold | `#FFD600` |
| Hold | Gold | `#FFD600` |
| Mode_detail_area | Gold | `#FFD600` |
| Option_A_1 | Darksalmon | `#E79473` |
| Option_B_1 | Darksalmon | `#E79473` |
| Option_A_2 | Darksalmon | `#E79473` |
| Option_B_2 | Darksalmon | `#E79473` |
| Option_A_3 | Darksalmon | `#E79473` |
| Option_B_3 | Darksalmon | `#E79473` |

The other 31 areas are white on black. There is no department level here, so the
hierarchy collapses to two: Orangered for what is being searched, Gold for what
was found.

---

## Still open

**Whether these assignments are the factory defaults.** Partly answered, and no
fresh radio was needed to do it.
`MENU -> Display Options -> Customize -> Restore Settings` puts a layout back to
stock, and restoring Simple Conventional on a scanner that had been customized
brought it back to what is recorded here. That layout is confirmed. The other
six have not been restored and compared, which is now a two minute job rather
than something needing a second radio. See
[menu-tree.md](menu-tree.md#restore).

**Whether the palette is the same on other models.** The 147 are presumably the
firmware's rather than the unit's, untested on a second scanner.

**How the display quantizes color.** 54 levels per channel, on no grid anyone
has identified.

**Whether a background other than black is ever used by default.** All 315 are
black here, which makes the `Set Back Color` menu look like something nobody at
Uniden used either.

**What reverse video does inside a colored area.** Every area drawn in reverse
video on this radio, the soft key row and the highlight bars, is white on black,
so the question never comes up: the swap looks the same either way. It would
matter for a Gold or Orangered area under a highlight, and the display reply
carries no color, so only looking at the radio can answer it. A renderer should
assume the area's own two colors swap, which keeps a highlight inside a colored
area in the color it belongs to. That is an assumption rather than a
measurement. Setting one highlighted area to a non-white text color and looking
would settle it.

**Why `Option_1` is Blue on the simple trunk layout, and `Func` is Tomato on the
detail trunk one.** Two areas, each used once, each in a color used nowhere
else, both on trunk layouts. Neither fits the hierarchy. Whether they are
defaults or somebody's edit is the question a fresh radio would answer in two
minutes.

**What the `DispColorId` groups in the card's configuration mean.** Areas are
written in 46 records rather than 315, so the groups carry some structure, and
nothing maps a record and an offset to an area name yet. Working that out would
make the card a complete substitute for the menu walk rather than a faster way
to get the same totals.
