# Oddities

A running log of things that surprised me while working the SDS150 out: places
the official documentation is wrong, firmware that misbehaves, and findings odd
enough to be worth writing down before somebody re-derives them.

Everything here was observed on an SDS150, firmware 1.00.37, against the Uniden
SDS Series Remote Command Specification V2.00. Entries are dated by when they
were found. Where something is a guess rather than a measurement, it says so.

The rule for this file: an entry earns its place by having cost somebody time,
or by being something that would cost time if forgotten.

---

## The documentation is wrong about

### `COLOR_MODE` is not a waterfall setting

*2026-08-04*

The spec lists `COLOR_MODE` in `GST` as "Waterfall display only." It is not. It
is the global display setting at
`MENU -> Display Options -> Set B/W or Color Mode`, and it tracks that setting
with the waterfall never opened:

| Menu entry | `COLOR_MODE` |
| ---------- | ------------ |
| Color Mode | `0` |
| Black w/White Text | `1` |
| White w/Black Text | `2` |

**Why it matters.** This is the _only_ thing about the display's appearance the
protocol reports, and the mislabel is why an earlier search concluded the
protocol carried nothing about appearance at all. Anything redrawing the
scanner's screen has to read it, or it paints a color screen while the radio in
your hand is monochrome.

**The lesson.** That document's claims about a field's scope are not
trustworthy. Test what a field actually tracks rather than believing the note
beside it.

### The nine `RSV` fields in `STS` are undocumented

*2026-08-03*

The spec shows them as reserved and says nothing further. They were decoded
against the legacy BCD specification and corroborated three ways via `GSI`. See
[protocol.md](protocol.md).

One of them is `BK_COLOR`, a vestigial backlight color from the BCD436HP era
where the display was monochrome with a colored backlight. On this model it
reads `OFF` and always will. It is the only color-ish field in the whole
protocol, and it is a dead end.

### `RETURN_PREVOUS_MODE`

*2026-08-03*

The `MSB` argument for leaving the menus is misspelled in the specification, and
the scanner accepts the misspelling. So the tool has to misspell it too. It is
in the code as-is, with a comment, and correcting it would break the command.

---

## Firmware oddities

### `Copper` is magenta

*2026-08-05*

The color picker offers a color named `Copper` whose value is `#BD00DE`. That is
a bright magenta. Copper is not magenta.

Checked against the raw screen rather than through my own reader, in case it was
a parsing fault:

```
'  Copper'
'  RGB = BD00DEh'
```

The scanner really does pair those. `Coolcopper` at `#D68418` is the sensible
copper-colored one, so the palette carries a reasonable copper _and_ a wrong
one.

**Why it matters.** Do not trust a color's name to describe the color. If a name
is being offered to a user, offer its value alongside.

### `Feldsper` is not a word

*2026-08-05*

One of the 147 colors is named `Feldsper`. The mineral is feldspar, and the
German is _Feldspat_. Neither spelling is `Feldsper`. It looks like a
typographical error that shipped and stayed.

### Seven of the 147 colors are not CSS names

*2026-08-05*

The palette is usually described, including here until this was checked, as "the
CSS color names." 140 of them are. These seven are not:

`Brass`, `Coolcopper`, `Copper`, `Cornflower`, `Darkbrown`, `Feldsper`,
`Richblue`

Those belong to the older X11 and Targa era of named color tables, which is a
lineage worth noticing: it suggests the palette was inherited from something
much older than the radio, and carried forward with its mistakes on board.

It also omits 8 CSS names: `rebeccapurple`, `lightgray`, and every British `grey`
spelling (`grey`, `darkgrey`, `dimgrey`, `slategrey`, `darkslategrey`,
`lightslategrey`).

**Why it matters.** Anyone typing a color name from memory of CSS will reach for
`grey` and be refused.

### The color values are near-CSS but not CSS

*2026-08-05*

The names come from CSS; the values do not. The scanner's Orangered is `#FF4600`
where CSS says `#FF4500`, its Darksalmon `#E79473` against `#E9967A`, its
Steelblue `#4280AD` against `#4682B4`. Up to 7 per channel out.

There are 54 distinct levels per channel across the palette, so the display
quantizes color, but not on a clean RGB565 grid: only 63 of the 147 values sit
on one. The scheme is unexplained and probably does not matter. What matters is
that these names label the _scanner's_ colors, not the web's.

### A held scanner is indistinguishable from a scanning one

*2026-08-05*

Turning the knob puts the scanner on hold, parked on one channel. Afterwards it
is out of the menus, answers every command correctly, and shows a channel name
where it would otherwise say `Scanning...`. A scanning radio shows a channel
name too, every time it stops on one it is receiving.

Only the mode says which: `Scan Hold` rather than `Scan Mode`. Nothing on the
screen does.

This one bit me. `radiocli scan`, whose entire job is returning the scanner to
scanning, walked away from a held scanner reporting success, because it only
knew about menus and quick search. Found because the radio sat on one channel
for ten minutes and the tool insisted everything was fine.

### The firmware image is encrypted, so there is no font to extract

*2026-08-06*

`SDS150_V1_00_43.bin` is exactly 2 MiB and encrypted end to end. Measured across
the whole file:

| Test | Result | What it rules out |
| ---- | ------ | ----------------- |
| Entropy | **8.000** bits/byte in every one of 32 64 KiB blocks | Any plaintext or structured region |
| Chi-square vs uniform | **248.2** (256 is the expectation for random) | Statistically indistinguishable from random |
| Repeated aligned blocks | **0** of 131,072 at 16 bytes, and 0 at 8 and 32 | ECB, and with it the usual block-repeat way in |
| Longest run of one byte | **3** | Plaintext firmware, which is full of long `00` and `FF` runs |
| Magic, header, footer, strings | none | A container with a readable index |

So it is a stream cipher, or CBC or CTR, with the key in the bootloader. Nothing
can be lifted out of it: not the bitmap font, not the layout tables, not the
strings.

The font is the obvious place to look for what the screen's non-ASCII bytes
draw, and this is where that road ends. The one group that does modify these
images, [OpenScanner](https://github.com/x27/openscanner), publishes built
binaries only, documents no algorithm or key, and does not list the SDS150 among
the models it supports.

The font was eventually had from a document rather than the firmware. See
[the screen text carries bytes that are not text](#the-screen-text-carries-bytes-that-are-not-text)
below. The point stands for anything else that might be wanted out of the image.

---

## Protocol surprises

### `GLT` answers an unknown keyword with the favorites list

*2026-08-07*

`GLT,<keyword>` does not refuse a keyword it does not recognise. It answers with
the favorites list document, for any index, and reports no error.

That is indistinguishable from a request that ran and failed strangely, and it
cost this project two "firmware bugs" that never existed. Both were wrong
keywords:

| Used | Actually | Reads |
| ---- | -------- | ----- |
| `CHN` | `CFREQ` and `TGID` | a department's channels |
| `SITE_FRQ` | `SFREQ` | a site's frequencies |

`GLT,CFREQ,<dept>` and `GLT,SFREQ,<site>` both work on 1.00.37 and always did.
The list of real keywords is in [protocol.md](protocol.md), and had the right
ones written down the whole time.

A department is two requests, not one. `CHN` was invented for "the channels,"
and no such request exists: `CFREQ` returns conventional frequencies and `TGID`
returns talkgroups, and a department answers on one of them depending on the
system above it. Asking a conventional department for `TGID` returns an empty
document, correctly, which is not the same as the wrong-keyword answer.

**Why it matters.** Believing the request was broken cost the `channels` command
a menu walk: several seconds per department, the scan stopped while it ran, and
no way at all to report what a talkgroup channel held, because the walk read
frequencies off a screen that talkgroup channels do not have. One exchange
replaced all of it.

**The lesson.** Check the _elements_ in a `GLT` reply, never the error. A reply
carrying `FL` when you asked for something else means the keyword is wrong far
more often than it means the firmware is.

### `MSI` menu listings are truncated, and sometimes empty

*2026-08-04, 2026-08-05*

The protocol's menu listing cannot be trusted for anything long:

| Menu | Real size | What `MSI` lists |
| ---- | --------- | ---------------- |
| A layout editor | up to 50 areas | 20 |
| A color picker | 147 colors | 26 |
| The Customize menu | 8 entries | nothing at all |
| A favorites list menu | all entries | omits `Review Avoids` |

The picker's listing also does not mark which entry is current, so it cannot be
used to locate the selection either.

**Why it matters.** Two separate mistakes in one session traced back to
believing a listing: a walk that stopped at 20 areas thinking it was done, and a
wrap test that computed zero steps and proved nothing. The screen is the only
complete source, and anything counting positions has to count them there.

The listing is still the only thing that carries an **index**, and the screen
never does, so anything that needs both has to read both. Whether the listing is
a fixed first-N or a window that slides with the cursor has not been measured.
See [Still unexplained](#still-unexplained).

### A duplicate frequency raises a popup that waits for ever

*2026-08-16*

Entering a frequency the scanner can already reach does not fail and does not
succeed. It raises a popup: **Frequency Exists / Accept? (Y/N)**. It is the
button kind of popup rather than the transient kind, so it does not clear
itself, and it is not a menu, so `MSI` reports nothing to walk and the knob has
nothing to turn.

Found by re-creating CB channel 8 on 27.055 MHz, which already existed. The tool
did not know about the prompt, went looking for the name screen, turned the knob
against a popup, timed out after three seconds, and left the radio sitting on the
question. Something had been created by then: an unnamed channel on 27.055 was
there afterwards and had to be deleted by hand, alongside the real CH 08, which
was intact.

**Why it matters.** A retry is the ordinary way to meet this. Anything creating
a channel has to read the screen after committing the entry and answer the
question, because everything sent afterwards goes into a radio that is not
listening for it. `radiocli channels new` answers no by default, so running the
same create twice is harmless, and `--allow-duplicate` answers yes.

**Answering no creates nothing.** Measured rather than assumed, since the stray
channel above made it worth checking: a department with one channel on 154.100,
a second create on the same frequency, N, and the department still holds one
channel. The stray came from the timeout, not from the prompt. The end to end
suite checks it, in `TestChannelsNew_DuplicateFrequency`.

### `MSV` is refused inside a color picker

*2026-08-05*

The protocol's own way of writing a value into the menu item the scanner is on
does not work in a picker, by name or by index:

```
-> MSV,Cyan   <- rejected in its current mode
-> MSV,22     <- rejected in its current mode
```

So the knob is the only way to move a picker, one color at a time. That is the
single biggest cost in setting a color, and it is not avoidable.

### Nothing in a reply says who asked

*2026-08-03*

The scanner answers on one line with no request identifier. Two programs on one
port therefore read each other's replies, and neither can tell. Started
together, a `scanning` and a `battery` both failed and both blamed the cable.

`TIOCEXCL`, which `go.bug.st/serial` sets, does not stop a second open of a
`/dev/cu.*` on macOS. An advisory lock file was the fix.

### Mass storage mode is a one-way door

*2026-08-05*

The scanner offers a choice when it starts with a cable attached, for about 15
seconds, before booting normally on its own:

```
Cable Detected
Select USB mode

Mass Storage="E"
Serial Port ="."
```

Choosing mass storage replaces the serial port rather than joining it. The radio
enumerates as a disk, `/dev/cu.*` does not exist, and nothing can be sent to it.
There is no command that brings it back, because there is no channel left to
carry one.

Ejecting the volume does not help either. The card unmounts, the radio stays a
USB disk with nothing on it, and still no serial port appears.

Only the power button gets it out. Every other mode this radio can enter is
escapable over the wire. This one is not.

### The card is still named after a different radio

*2026-08-05*

The scanner's memory card holds one directory, `BCDx36HP`, and inside it every
configuration file opens with a `TargetModel` of `BCDx36HP`. The identity file
names the radio as an SDS150 in a later field, but the format announces itself
as the older model's throughout.

Same story as the protocol, which the SDS series inherited from the same
lineage, and the same practical consequence: anything looking for "SDS" on that
card finds nothing.

One of the configuration files is `discvery.cfg`. The word is discovery.

### The scanner reports `plain_text` for a moment after being moved

*2026-08-05*

Right after a command moves it, `V_Screen` reads `plain_text` briefly before it
reports what it is actually doing. Long enough that a command run immediately
after another sees it and concludes the scanner is on a screen it knows nothing
about.

Anything reading the view has to let it settle. Three seconds is enough.

The same is true of the screen it draws, and that one is still biting: see
[layout-detection.md](layout-detection.md) for a reading of the live screen that
is right most of the time and wrong now and then.

### The screen text carries bytes that are not text

*2026-08-06*

A scanning screen contains exactly two bytes below 0x20: `0x1a` and `0x1b`, at
line 1 columns 28 and 29. They are not corruption and not an escape sequence.
The scanner's font has its own pictures at those codes, and this is how it draws
an icon in a field of the screen.

Those two columns are the `Direction` area of the screen map, which the owner's
manual labels `DIR`. It is one field two cells wide, so the pair is one icon
rather than two characters, and neither byte means anything on its own. On the
radio it is the scan direction arrow.

They are static. Setting the volume to 15, 5 and 0 and re-reading `STS` each
time left both bytes unchanged, so whatever the field shows, it is not a level.

The table exists and is not published. The BCDx36HP remote command specification
says, under `STS`: *"See "Font Data Specification" for not ascii character
code."* That document is on no Uniden page, is linked from no firmware or
support page, and the SDS series specification dropped the reference entirely.
The firmware would be the other place to find it, and it is encrypted.

A copy of it turned up anyway: Uniden Font Data Specification, SDS200 (UB384Z),
version 1.00, issued 2018/08/02. It draws all 256 codes as pixel grids. The 32
below 0x20:

| Code | Picture | Code | Picture |
| ---- | ------- | ---- | ------- |
| `0x01` | filled block | `0x14` | `F`, in NFM |
| `0x02` | FUNC1 | `0x15` | `M`, in NFM |
| `0x03` | FUNC2 | `0x16` | `B`, in FMB |
| `0x05` | degree sign | `0x17` | volume and squelch bar, left end |
| `0x06`, `0x07` | ellipsis | `0x18` | volume and squelch bar, middle |
| `0x08`, `0x0B` | AM | `0x19` | volume and squelch bar, right end |
| `0x0C`, `0x0E` | FM | `0x1A`, `0x1B` | **up arrow**, left and right halves |
| `0x0F` | NFM | `0x1C`, `0x1D` | **down arrow**, left and right halves |
| `0x1E`, `0x1F` | T-1, T-2 | | |

`0x00`, `0x04`, `0x10` to `0x13` are marked `---`, and `0x09`, `0x0A`, `0x0D`
are the ASCII tab, line feed and carriage return, all blank. So the pair on a
scanning screen is an up arrow, drawn across two cells, and the field says which
way the scan is running. The volume marks are real too, at `0x17` to `0x19`,
which is worth knowing before assuming a bar on screen is text.

Every glyph in the small font is a 16 by 16 grid, and the document carries a
second table for the large font at 16 by 32, which is what "a large row is two
rows tall" means in pixels.

The large table is the small one with every row drawn twice. Comparing glyphs
pulled out of the large pages against the small ones doubled gives 97 to 100%
agreement across sixteen letters and digits, `E` and `F` exact, with every
remaining pixel explained by the sampling of the reader rather than a difference
in shape. So there is only one font to keep: the large one is a draw-time
transform, which is a single scaled blit for anything using a canvas.

Both tables draw 16 pixels wide, but the radio gives a large character a 20
pixel cell and stretches the picture into it. The document does not say that
anywhere; see
[a large-font row is wider and taller](#a-large-font-row-is-wider-and-taller-and-the-screen-is-480-by-320).

The pictures themselves, and the full code to name table, are in
[glyphs.md](glyphs.md).

The practical consequence for anything rendering the screen: these bytes cannot
be turned into text, because no character means "left half of an arrow," and
they cannot be passed through either, since a terminal draws a control code as
nothing or as a box of the wrong width and the rest of the row shifts a column
across. Either draw the picture, or replace each with one space so the row stays
30 columns. A visible stand-in such as a dot is worse than a blank: it reads as
something the scanner is displaying, and sends somebody looking for a fault in
the radio.

### Reverse video can extend past the last visible character

*2026-08-04, 2026-08-07*

In `STS`, the attribute string can be longer than the trimmed text, because a
highlight bar is drawn across the rest of a row. Pad the text to the attribute
length before indexing into both.

The text can be absent entirely. The simple conventional layout has two lines
that are nothing but a rule across the panel: the text field is empty and the
attribute field is thirty underscores. So the attributes are not "as long as the
text, sometimes longer," they are the width of the panel or empty, and the text
is whatever the scanner has trimmed it to.

That cost a day. The parser here decided which field was the attributes by
measuring it against the text, and on that layout it measured wrong, glued the
rule onto the line as if it were more text, and went looking for the attributes
in the status fields behind the screen. `screen` carried on working, because what
it drew was still close enough to look right; `display` and `status` failed on
the count of what was left over, and so did the test harness's own probe, which
reported the scanner as not answering. A parse that is wrong in the middle can
surface as an unrelated command refusing to run.

The same measurement was wrong the other way. A field the same length as the
text was taken for the attributes, and an unescaped comma in `GREENDALE, ST
00000` splits it into nine characters either side, so the second half of a
favorites list's name was read as a line of attributes.

The fix is to read the field rather than measure it: the attributes are made of
` `, `*` and `_` and nothing else, so anything holding another character is
text. If a fourth attribute character ever turns up, that is the list to add it
to.

### The soft key row is always reverse video

*2026-08-04*

Which means the layout editor cannot show which of the five soft key areas is
selected: the selection is indistinguishable from the row itself. Their
positions have to come from the live screen instead, where the row's three runs
of reverse video are the three keys.

### A large-font row is wider and taller, and the screen is 480 by 320

*2026-08-05, settled against the radio 2026-08-06*

`DSP_FORM` marks a line as large font. Two things follow.

A large character is a quarter wider and twice as tall. 24 of them fill the same
width as 30 normal ones, so a large row reaches the right hand edge like any
other. In pixels: 30 by 16 or 24 by 20, both 480 across; 16 pixels to a normal
row, 32 to a large one.

The panel is 480 by 320, and nothing measured it. It falls out of the `DSP_FORM`
strings in [colors.md](colors.md), counting 16 for a normal line and 32 for a
large one:

| Layout | Lines | Large | Height |
| ------ | ----- | ----- | ------ |
| `00110110110000` simple | 14 | 6 | **320** |
| `00001010100000000` detail | 17 | 3 | **320** |
| `00001110000000000` search, weather, tone out | 17 | 3 | **320** |
| a menu, every line large | 10 | 10 | **320** |

Four layouts recorded separately landing on the same number. That is both the
confirmation and the check: any rule about large rows that does not bring all
four to one height is wrong.

Simple has six large lines, not three. Each of the three names is drawn as _two_
lines of 24, which is what the owner's manual means by `System Name (24char x 2)`
against detail's `Department Name (24x1)`, and what the three adjacent pairs of
`1`s in `00110110110000` say outright.

The width nearly went the wrong way. The font specification draws the large
glyphs 16 wide, the same as the small ones and with the same shapes, which reads
as "a large character is the same width, so a large row is four fifths of the
screen." Every measurement of the document agreed with that, and it is wrong:
the radio stretches a 16 pixel picture across a 20 pixel cell. What settled it
was looking at the radio. A system name too long to fit is truncated at 24
characters with an ellipsis, and that ellipsis sits hard against the edge of the
screen; a menu's highlight bar runs the full width. Drawn four fifths wide, both
stop short in a way no arithmetic would have flagged, because the heights still
came to 320 either way.

Worth keeping as a lesson rather than a footnote: a specification describes the
_data_, not what the hardware does with it, and the two agreed to the pixel on
everything except the one thing that was never written down.

### JSON silently destroys the screen's pictures

*2026-08-06*

The screen is bytes, not text. Its font has pictures above `0x7E` and the
scanner puts them in a line like any other character, so a row can hold `0xAC`
and `0xAD`, the two halves of the signal meter.

A Go string holding those raw bytes is not valid UTF-8. `encoding/json` replaces
every invalid byte with U+FFFD and returns no error, so the same screen came out
two different ways:

```
$ radiocli screen | cat -v
                Aug.6 21:42 M-,M--        <- 0xAC 0xAD

$ radiocli screen -o json
"text": "              Aug.6 21:43 \ufffd\ufffd"
```

Not merely corrupted: _collapsed_. `0xAC` and `0xAD` are different pictures and
became the same character, so nothing downstream could recover them or even tell
that anything had happened. Codes below `0x80` are unaffected, which is why the
modulation marks at `0x14` and `0x15` survived and the meter did not, and why
this looked for a while like the scanner only sometimes sending icons.

The fix is to widen each byte to the character of the same value at the point of
encoding, which is lossless and leaves ASCII alone. It is done there rather than
where the line is built because the device layer decides what is an attribute row
by comparing the byte lengths of the two fields, and a widened string is longer
than the line it came from.

Worth remembering generally: a marshaller that returns `nil` has not promised it
kept your data. Anything carrying bytes through JSON needs to say what encoding
it is using, and be tested with a byte above `0x7F`.

### The documented `WiFi` field does not exist on this radio

*2026-08-05*

The specification gives `Property` a `WiFi` attribute with values `Off`, `0`
through `3`, and `AP`. On this SDS150 the attribute is not present at all, not
`Off` but absent, and the memory card holds no network configuration of any
kind: nothing naming WiFi, an address, a port or a socket.

Worth knowing before building anything around it, and worth pairing with the
undocumented `UDP` command, whose name invites the wrong conclusion.

### Keys the model does not have are still accepted

*2026-08-03*

`KEY,R` and `KEY,T` (Range, Service Type) have no key on the SDS100/SDS150. The
scanner answers `KEY,OK` and does nothing. So a successful reply is not evidence
that anything happened.

---

## Things that look wrong and are not

### Backing out of a color picker abandons the change

*2026-08-05*

Turning the knob in a picker changes what is on screen but writes nothing.
Pressing enter is the whole of the write. Leaving with the menu key discards the
knob's position and keeps the stored color.

This is what makes it safe to walk all 147 colors to check the palette. Verified
rather than assumed: the check reports which area's picker it borrowed and what
color was there, and the test sets that area to that same color and confirms
nothing changed.

### `GST` is strictly better than `STS`

*2026-08-04*

Same screen, plus both LED colors and a mode field, in one round trip. There is
no reason to use `STS` except habit.

---

## Still unexplained

- **Why `Copper` is magenta.** A transposed table entry is the obvious guess,
  but nothing confirms it.
- **How the display quantizes color.** 54 levels per channel, not a clean
  RGB565 grid.
- **Whether `Feldsper` is meant to be feldspar**, or something else entirely.
- **Whether the SDS200 font matches the SDS150's.** The Font Data Specification
  that names the pictures is the SDS200 one, and it is the only copy there is.
  The codes on a live SDS150 screen land where that document says they should,
  which is evidence but not proof.
- **Whether any of this differs on a fresh radio**, or on an SDS100. Everything
  here is one scanner on one firmware.
- **Whether an `MSI` listing slides with the cursor.** It reports 20 of 50
  areas, but nothing has established whether those 20 are always the first 20 or
  the 20 around wherever the knob is. It matters because it decides whether
  gathering the listing at every step of a walk recovers indexes for a whole
  long menu or only ever for its first screenful. The experiment is one walk
  round a menu of more than 26 entries, recording each listing as it goes and
  comparing the union against the names read off the display.
- **Whether popup buttons can be read from the protocol.** The specification
  says a button popup carries each button's label and key code, which would be
  an exact way to recognise a prompt instead of matching screen text. Nothing
  here has confirmed the radio actually sends those fields, or where.
- **Why a named layout is sometimes called not-current when it is.** Seen once
  in five runs, never reproduced on demand. Written up with the evidence and the
  experiment that would settle it in
  [layout-detection.md](layout-detection.md).
