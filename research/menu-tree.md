# Menu tree

Every screen reachable from the MENU key on an SDS150, firmware 1.00.37, what
each one is, and which of them this walk has actually opened.

This is a map rather than a manual. Each section is one screen, lists the
entries on it, and says where each entry leads. It was written from walking the
radio rather than from the owner's manual, so an entry missing from Uniden's
menu reference still appears here if the radio draws it.

## How to read it

`Explored` has three values:

| Value | Meaning |
| ----- | ------- |
| `yes` | Opened, and what is inside is written down below. |
| `no, acts` | Not opened on purpose. Choosing it does something the moment it is chosen, rather than opening a screen. |
| `no, editor` | Not opened on purpose. It is a text or number editor, and this walk has no safe way back out of one. |

Entries with no submenu are marked `value list` and their choices are listed
inline. Opening one of those is safe, because the choice is only written when
E/YES is pressed on a value, and this walk never did that.

Nothing in this file was changed. The walk pressed E/YES to go in and MENU to
come back out, and never pressed E/YES on a value or on an entry that acts.

The display shows nine rows at a time onto a longer list, so entries here are in
the order the knob steps through them rather than in any numbering the radio
exposes.

## Screen ids

Every menu screen has an id, and it comes from `MSI`, which answers with an XML
document describing the menu the radio is showing:

```
<MSI Name="Settings" Index="4293092724" MenuType="TypeSelect" Value="..." >
  <MenuItem Name="Adjust Key Beep" Index="0" />
  ...
```

Three things there are worth having, and each screen below records them:

| Field | What it is |
| ----- | ---------- |
| `Index` | The screen's own id. Unique per screen. |
| `Name` | The title, untruncated, which the display often is not. |
| `MenuType` | `TypeSelect` for a menu or a list, `TypeInput` for a single number, `TypeLocation` for a coordinate, `TypeError` for a screen the protocol will not describe. All four were seen. |

Treat `Index` as a firmware address rather than a name. The values sit in a
narrow band that reads as a pointer: the root is `4293093020`, which is
`0xFFD6BFDC`, and Settings is `0xFFD6BEB4`. They were stable across repeated
reads and across leaving and re-entering a screen within this session, and
nothing here shows they survive a firmware update. The shape of them suggests
they will not. Match on `Name` for anything that has to keep working.

Not every screen has one. Readouts such as `Information` answer `MSI` with
nothing at all: no `Index`, no `MenuType`, no items. Those are marked `id: none`
below, and the only thing identifying them is what they draw.

`MSI` answers in one of three shapes, and only one of them identifies a screen.

*One, a full answer.* `Name`, `Index` and a `MenuType` of `TypeSelect`,
`TypeInput` or `TypeLocation`, with the items listed. This is most of the tree.

*Two, a refusal.* `MenuType="TypeError"`, no `Name`, no `Index`, no items, and
a `MenuErrorMsg` carrying one fixed sentence:

```
The scanner is in state that you can't remote control.
Please goto scan mode to remote control.
```

That message is the real finding, and it has nothing to do with errors. These
screens are not being misreported. The firmware is declaring them outside the
part of the menu system it will describe or drive remotely. `TypeError` is a
misleading name for it: nothing has gone wrong, and on most of these screens
nothing is even being refused.

*Three, nothing at all.* An empty `<MSI >` with only a footer. No identity, no
error, no explanation. This is what the readouts give.

### Which screens fall outside

An entry missing from its parent's item list is the same fact seen from the
other side: `MSI` omits the entries whose screens it will not describe, and only
those. Twelve such entries were found, and every one that could safely be opened
gave shape two or three.

| Omitted from | Entry | What that screen actually draws | Shape |
| ------------ | ----- | ------------------------------- | ----- |
| the root | `Analyze` | a normal two-entry menu | refusal |
| Manage Full Database | `Review Avoids` | a refusal message | refusal |
| a favorites list | `Review Avoids` | a refusal message | refusal |
| Srch/CloCall Opt | `Freq Avoids` | a normal two-entry menu | refusal |
| Discovery | `Review Discovery` | a normal two-entry menu | refusal |
| WX Operation | `Weather Alert` | a seven-value list | refusal |
| WX Operation | `Review WX Alerts` | a modal message | refusal |
| WX Operation | `Weather Scan` | starts a scan | not opened, it acts |
| Display Options | `Customize` | a normal eight-entry menu | refusal |
| Settings | `Band Defaults` | the whole band plan table | refusal |
| Settings | `Restore Options` | unknown | not opened, it acts |
| Replay Options | `Review Recordings` | a modal message | refusal |

Ten confirmed, two left alone because opening them does something, and the rule
did not fail once. Separately, `Set Nationwide Systems` and `Information` give
shape three, and neither is omitted from its parent.

The cost of that is real. The entire band plan and the entire display
customization menu are undescribable, and `Customize` is unreachable from the
wire in both directions, since the spec has no `DISPLAY_OPTIONS` id for `MNU`
either.

### What that does to `radiocli menu`

Standing on one of these screens, the tool reports the opposite of what is on
the display:

```
$ radiocli menu
The scanner is not in a menu.

$ radiocli screen
  Freq Avoids
> Stop All Avoiding
  Rvw Search Avoid
```

`screen` is right and `menu` is wrong, because `menu` believes `MSI`. The radio
is in a menu; the protocol has declined to describe it. Anything navigating by
`MSI` alone concludes these screens do not exist.

Which is why every screen below was read twice, once through `MSI` and once
through `STS`. `STS` drew all of them correctly, every time.

`radiocli` does not expose `Index` or `MenuType`; both were read straight off
the port with a raw `MSI`, which is a query and writes nothing.

---

## `--- M E N U ---`, the root

`id: 4293093020` &nbsp; `name: "    --- M E N U ---    "` &nbsp; `type: TypeSelect`

`MSI` omits `Analyze`, which sits at item index 8 and is drawn on screen. The
item indices it does report skip from 7 to 9 around the gap, so the hole is
visible in the numbering.

What the MENU key opens from any operating screen. Seventeen entries, all of
them submenus.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Scan Selection | [Set Scan Selection](#set-scan-selection) | yes |
| Manage Full Database | [Manage Full Database](#manage-full-database) | yes |
| Manage Favorites | [Select Favorites](#select-favorites) | yes |
| Set Your Location | [Set Your Location](#set-your-location) | yes |
| Select Service Types | [Select Service](#select-service) | yes |
| Srch/CloCall Opt | [Srch/CloCall Opt](#srchclocall-opt) | yes |
| Search for... | [Search for...](#search-for) | yes |
| Close Call | [Close Call](#close-call) | yes |
| Analyze | [Analyze](#analyze) | yes |
| Discovery | [Discovery](#discovery) | yes |
| Priority Scan | [Priority Scan](#priority-scan) | yes |
| WX Operation | [WX Operation](#wx-operation) | yes |
| Tone-Out for... | [Tone-Out for...](#tone-out-for) | yes |
| Waterfall | [Waterfall](#waterfall) | yes |
| GPS | [GPS](#gps) | yes |
| Display Options | [Display Options](#display-options) | yes |
| Settings | [Settings](#settings) | yes |

---

## Set Scan Selection

`id: 4293173388` &nbsp; `type: TypeSelect`

Decides what the radio scans at all: which sources are switched on, and which
number keys jump to which of them.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Manage Quick Key Status | [Set Quick Key Status](#set-quick-key-status) | yes |
| Set Nationwide Systems | a modal refusal, or the systems list | yes |
| Select Lists to Monitor | [Select Lists to Monitor](#select-lists-to-monitor) | yes |
| Set All Lists Off | switches every source off | no, acts |
| Set All Lists On | switches every source on | no, acts |

### Set Quick Key Status

`id: 4293173060` &nbsp; `type: TypeSelect`

Which quick key jumps to which list or system.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Favorites Quick Key | a numbered slot list, `00` to `99` | yes |
| System Quick Key | [Select Favorites List](#select-favorites-list) | yes |

#### Favorites Quick Key

`id: 4293172844` &nbsp; `name: "Set Quick Key Status"` &nbsp; `type: TypeSelect`

A list of numbered quick key slots, `00` upward, each showing the favorites
list assigned to it and whether that key is on. Empty slots read as dashes.

#### Select Favorites List

`id: 4293173008` &nbsp; `type: TypeSelect`

What `System Quick Key` opens. One row per favorites list, numbered the same
way, and choosing one presumably opens that list's own system quick keys. Not
descended into.

### Set Nationwide Systems

`id: none` &nbsp; `type: none`, and `MSI` answers with an empty document

Not a menu on this radio. With `Full Database` switched off it draws `Failure`,
tells you to select Full Database in Select Lists to Monitor, and waits on
`Press Any Key`. The same precondition as [Review Avoids](#review-avoids),
reported differently: this one calls it a failure.

### Select Lists to Monitor

`id: 4293173184` &nbsp; `type: TypeSelect`

One row per source the radio can scan, each against `On` or `Off`. The three
built-in sources come first, then one row for every favorites list stored on
the radio.

Rows: `Full Database`, `Search with Scan`, `Quick Save Favorites List`, then the
user's own lists. The display truncates the third to `Quick Save Favorit`;
`MSI` gives the full name, which is the general rule for reading names off this
radio.

This is a toggle list, so E/YES flips the row it is on. Not pressed.

---

## Manage Full Database

`id: 4293168804` &nbsp; `type: TypeSelect`

`MSI` omits `Review Avoids` here, reporting only `Stop All Avoiding` and
`Information`. The display shows all three.

The built-in database, as opposed to anything saved into a favorites list.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Review Avoids | a message, or the avoid list | yes |
| Stop All Avoiding | clears every avoid in the database | no, acts |
| Information | [Information](#information) | yes |

### Review Avoids

`id: none` &nbsp; `type: TypeError`

Not a menu. With `Full Database` switched off in Select Lists to Monitor, it
draws a message saying to switch it on before this operation, and nothing else.
There is no list to review because the database is not being scanned.

The same entry inside a favorites list behaves the same way and words it for
the list rather than the database: `Set to On this list in / Select Lists to
Monitor / before this operation.`

This is the one of the four `TypeError` screens where the type is honest.

What either draws when the source _is_ on has not been seen, because switching
it on would change what the radio scans.

### Information

`id: none`

Not a menu. A single readout giving the database version as a date.

---

## Select Favorites

`id: 4293168612` &nbsp; `type: TypeSelect`

What `Manage Favorites` opens. Note the title differs from the entry that
opens it.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| New Favorites List | creates a list and opens its editor | no, editor |
| Quick Save Favorites L | [a favorites list menu](#a-favorites-list-menu) | yes |
| *(one row per favorites list on the radio)* | [a favorites list menu](#a-favorites-list-menu) | yes |

### A favorites list menu

Every favorites list opens the same menu, titled with the list's own name.

`id: 4293168468` for the Quick Save list. Each list gets its own id, so this is
one id per list rather than one id for the shape.

**Correction from the first pass.** The built-in Quick Save list was recorded as
having a shorter menu than a user list. It does not: `MSI` reports `Delete` and
`Information` on it too. The first pass stopped at the ninth row because that is
all the display shows at once, and read the truncation as the end of the list.
All eleven entries are on both.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Review/Edit System | [Select System](#select-system) | yes |
| Set FL Quick Key | value list | no |
| Set FL Number Tag | number | no |
| Set FL Startup Key | value list | no |
| Use Location Control | value list | no |
| Review Avoids | a list or a modal | no |
| Stop All Avoiding | clears this list's avoids | no, acts |
| Add Current dB Channels | copies database channels in | no, acts |
| Rename | editor | no, editor |
| Delete | deletes the list | no, acts |
| Information | a readout | no |

`Delete` and `Information` are absent from the Quick Save list's copy of this
menu.

### Select System

One row per system in the list, with `New System` first.

`New System` is not an editor prompt, it is a creation flow. Choosing it
goes straight to `Select System Type`, offering `P25 Trunk`, `P25 X2-TDMA`,
`P25 One Frequency`, `Motorola`, `MotoTRBO Trunk`, `DMR One Frequency`, `NXDN
Trunk`, `NXDN One Frequency`, `EDACS`, `LTR` and `Conventional`. MENU backs out
of it without creating anything, which was checked by counting the systems
afterwards.

Below `New System` the memory tree carries on: system, then department, then
channel, each with its own menu. The channel menu at the bottom has fourteen
entries: Edit Name, Edit Frequency, Set Audio Type, Set Channel Number Tag, Set
Modulation, Set Attenuator, Set Service Type, Set Delay Time, Set Priority, Set
Alert, Set Avoid, Volume Offset, Delete Channel, New Channel.

That part of the tree is per-system rather than fixed, so it is described by
shape here rather than enumerated. `radiocli menu open system|department|
channel <index>` reaches any of it directly.

---

## Set Your Location

`id: 4293169816` &nbsp; `type: TypeSelect`

Where the radio thinks it is, which is what the built-in database is filtered
against.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Enter Zip Code | number editor | no, editor |
| Set Manual Location | `Input Latitude`, a coordinate editor | yes, backed straight out |
| Set Range | [Set Range](#set-range) | yes |
| Edit Location | [Select Location](#select-location) | yes |
| Save Location | saves the current location | no, acts |

### Set Manual Location

`id: 4293169620` &nbsp; `name: "Input Latitude"` &nbsp; `type: TypeLocation`

The only `TypeLocation` screen found. `MSI` reports the current latitude as a
decimal number in `Value`, while the display draws it as degrees, minutes and
seconds.

Goes straight into an editor titled `Input Latitude`, showing the current
latitude in degrees, minutes and seconds with a N or S on the end. Presumably a
longitude editor follows it. MENU backs out of it cleanly without writing
anything, which is worth recording: not every editor on this radio traps you.

Not explored further, because the next step is typing over somebody's stored
position.

### Set Range

`id: 4293169664` &nbsp; `type: TypeInput`

Not a menu. One number on its own with `(mile)` written under it, which is how
far from the set location the database is filtered to.

### Select Location

`id: 4293169752` &nbsp; `type: TypeSelect`

What `Edit Location` opens, and the title differs from the entry again. Lists
the saved locations, with `New Location` as the first entry. On a radio with
none saved, `New Location` is the only row.

`New Location` is an editor and was not opened.

---

## Select Service

`id: 4293182276` &nbsp; `name: "Select Service (S1=Exit)"` &nbsp; `type: TypeSelect`

Titled `Select Service (S1=Exit)` on screen, and it means it: soft key 1 is
the way out rather than MENU.

A flat list of 39 service types, each against `On`, `Off`, or `---`. The dashes
mean no monitored source contains that type, so there is nothing to switch.

The types, in the order the knob steps through them: Aircraft, Business,
Corrections, Emergency Ops, EMS Dispatch, EMS-Tac, EMS-Talk, Federal, Fire
Dispatch, Fire-Tac, Fire-Talk, Ham, Hospital, Interop, Law Dispatch, Law Tac,
Law Talk, Media, Military, Multi-Dispatch, Multi-Tac, Multi-Talk, Other, Public
Works, Railroad, Schools, Security, Transportation, Utilities, Racing
Officials, Racing Teams, Custom 1 through Custom 8.

No submenus. E/YES toggles a row, and was not pressed.

---

## Srch/CloCall Opt

`id: 4293099980` &nbsp; `type: TypeSelect`

`MSI` omits `Freq Avoids`, the first entry, which the display shows.

Settings shared by searching and by Close Call.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Freq Avoids | [Freq Avoids](#freq-avoids) | yes |
| Broadcast Screen | [Broadcast Screen](#broadcast-screen) | yes |
| Repeater Find | value list: `On`, `Off` &nbsp; `id: 4293098420` | yes |
| Set Delay Time | value list, ten values &nbsp; `id: 4293098524` | yes |
| Set Attenuator | value list: `On`, `Off` &nbsp; `id: 4293098472` | yes |
| Digital Waiting Time | value list, eleven values &nbsp; `id: 4293099852` | yes |

All four are value screens rather than menus: a title, then the choices, with
the one in force highlighted. MENU leaves them without writing anything, which
was checked by going into one, moving the highlight, backing out, and reading
the setting again.

**Corrected from the first pass.** Set Delay Time does not stop at `10 sec`.
The full ten values are `-10`, `-5`, `0`, `1`, `2`, `3`, `4`, `5`, `10` and
`30 sec`. The first pass read only the nine rows the display shows at once and
took the last visible one for the last one. Digital Waiting Time likewise runs
`0 ms` to `1000 ms` in hundreds, eleven values, not an open-ended climb.

Every one of these reports `MenuType: TypeSelect`, the same as a real menu.
Nothing in `MSI` distinguishes a list of settable values from a list of places
to go.

### Freq Avoids

`id: none` &nbsp; `type: TypeError`

Frequencies set aside during a search.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Stop All Avoiding | clears every search avoid | no, acts |
| Rvw Search Avoid | a modal message, or the avoided frequencies | yes |

One of the screens the protocol will not describe. The display draws a normal
two-entry menu with a title; `MSI` answers with the refusal shape. See
[which screens fall outside](#which-screens-fall-outside).

With nothing set aside, `Rvw Search Avoid` draws `Nothing Avoided` and waits on
`Press Any Key`, the same modal shape as Review WX Alerts.

### Broadcast Screen

`id: 4293099808` &nbsp; `type: TypeSelect`

Whole bands kept out of a search.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set All Band On | switches every band on | no, acts |
| Set All Band Off | switches every band off | no, acts |
| Set Each Band | [Set Each Band](#set-each-band) | yes |
| Program Band | editor | no, editor |

#### Set Each Band

`id: 4293099416` &nbsp; `type: TypeSelect`

A toggle list, each row `On` or `Off`: `Pager`, `FM`, `UHF TV`, `VHF TV`,
`NOAA WX`, then ten user-programmable rows `Band 0` through `Band 9`. The
programmable rows are what `Program Band` edits.

---

## Search for...

`id: 4293180996` &nbsp; `type: TypeSelect`

The searches, and the keys that start them.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Custom Search | starts a custom search | no, acts |
| Edit Custom | [Select Custom](#select-custom) | yes |
| Set Search Key | [Select Key No.](#select-key-no) | yes |
| Search with Scan | starts search with scan | no, acts |

### Select Custom

`id: 4293180664` &nbsp; `type: TypeSelect`

Ten saved search ranges, `Custom 0` through `Custom 9`, each opening its own
editor.

Not descended into: each is a range editor.

### Select Key No.

`id: 4293180828` &nbsp; `type: TypeSelect`

`Search Key 1`, `Search Key 2`, `Search Key 3`. Each assigns a search function
to one of the number keys.

Opening one gives `Select Search Range`, a list of what that key can be set to.
The whole list has now been walked, by turning the knob all the way round until
it repeated. Choosing a row writes the assignment, so no row was chosen and the
walk was made with the knob alone.

| Row | What it is |
| --- | ---------- |
| `.` | drawn as a lone dot, and the first row: presumably none |
| `Custom 0` … `Custom 9` | the ten user-defined search ranges |
| `Tone Out` | the tone-out search |
| `Close Call` | the Close Call search |
| `Waterfall` | the waterfall display |

Fourteen rows, and then it wraps back to `.`.

There is no service search on this radio. No `CB Radio`, no `Public Safety`,
no `Aircraft`: the band-by-band service searches that Uniden's older scanners
put on this list are simply not here, and the only ranges a search key can be
bound to are the ten the user defines. That is consistent with the SDS series
being built around the database rather than around service bands, and it means
searching any particular band, CB included, starts by defining a Custom range
for it.

`Select Service Types` on the top menu is a different thing despite the name. It
filters which service types the database scans, not what a search covers.

Only `Search Key 1` was opened. The other two are assumed identical and are not
marked explored. Search Key 1 was found assigned to `Custom 1` on this radio.

---

## Close Call

`id: 4293098224` &nbsp; `type: TypeSelect`

Watching for a transmitter close by.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Close Call Only | starts a Close Call watch | no, acts |
| Hits with Scan | switches the hits system into the scan | no, acts |
| Set CC Mode | value list: `Off`, `CC DND`, `CC Priority` &nbsp; `id: 4293097592` | yes |
| Set CC Alert | [Set CC Alert](#set-cc-alert) | yes |
| Set CC Bands | toggle list of seven bands | yes |

### Set CC Alert

`id: 4293098160` &nbsp; `type: TypeSelect`

The only real submenu under Close Call.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Alert Tone | value list, titled `Set Tone`: `Off`, `Alert 1` to `Alert 9` &nbsp; `id: 4293097976` | yes |
| Set Alert Light | value list, titled `Set Color`: `Off` and seven colors | yes |
| Set CC Pause | value list: `3` to `60 sec`, and `Infinite` &nbsp; `id: 4293098096` | yes |

Both of the first two are titled differently from the entry that opens them,
which happens all over this tree.

### Set CC Bands

`id: 4293097472` &nbsp; `type: TypeSelect`

A toggle list, one row per band, each against `On` or `Off`: `VHF Low 1`,
`VHF Low 2`, `Air Band`, `VHF High1`, `VHF High2`, `UHF`, `800MHz+`.

---

## Analyze

`id: none` &nbsp; `type: TypeError`

One of the four screens `MSI` cannot identify. It is an ordinary two-entry menu
on the display, and it is also the entry the root's own item list leaves out.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| System Status | starts a measurement | no, acts |
| LCN Finder | starts a measurement | no, acts |

---

## Discovery

`id: 4293103504` &nbsp; `type: TypeSelect`

`MSI` omits `Review Discovery`.

Logging what the radio hears rather than just playing it.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Trunking Discovery | a one-entry screen, `New Session` | yes |
| Conventional Discovery | a one-entry screen, `New Session` | yes |
| Review Discovery | [Review Discovery](#review-discovery) | yes |

Both discovery entries open a screen holding nothing but `New Session`, which
is where a session gets named and configured. Not opened: it is an editor, and
finishing it starts a logging run.

### Review Discovery

`id: none` &nbsp; `type: TypeError`

One of the four screens `MSI` cannot identify, despite being an ordinary menu.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Trunk Discovery Results | readout, empty with no sessions | yes |
| Conv Discovery Results | readout, empty with no sessions | yes |

Both draw `Review Discovery Results` and `No Discovery Results` on a radio with
nothing logged. Unlike the other empty screens in this tree they do not ask for
a key press; MENU leaves them.

---

## Priority Scan

`id: 4293090604` &nbsp; `type: TypeSelect`

Whether some channels are allowed to interrupt the rest.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Priority | value list: `Off`, `Priority DND`, `Priority Scan` &nbsp; `id: 4293090540` | yes |
| Set Interval | `TypeInput`, a number in seconds &nbsp; `id: 4293090464` | yes |
| Max Channels/Pri-Scan | `TypeInput`, a number in channels &nbsp; `id: 4293089324` | yes |

`Set Interval` and `Max Channels/Pri-Scan` are single-number screens of the same
shape as Set Range: the title, the number, and the unit written underneath.

---

## WX Operation

`id: 4293181656` &nbsp; `type: TypeSelect`

`MSI` omits three of the seven entries here: `Weather Scan`, `Weather Alert`
and `Review WX Alerts`. It reports only the three `Set` entries and `WX Alt
Priority`. This is the worst case found: half the screen is invisible to the
protocol.

Weather channels and weather alerts.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Weather Scan | starts a weather scan | no, acts |
| Weather Alert | value list: `Alert Only`, `SAME 0` to `SAME 4`, `All FIPS` | yes |
| Review WX Alerts | a modal message, or the recordings | yes |
| Program SAME | editor | no, editor |
| Set Delay Time | value list: `-10 sec` through `10 sec` | yes |
| Set Attenuator | value list: `On`, `Off` | yes |
| WX Alt Priority | value list: `On`, `Off` &nbsp; `id: 4293181576` | yes |

`Set Delay Time` and `Set Attenuator` here are the same two screens as under
[Srch/CloCall Opt](#srchclocall-opt), reached by a second path. Whether they are
one setting or two separate ones has not been tested.

### Review WX Alerts

`id: none` &nbsp; `type: TypeError`

Not a menu and not a list. With nothing recorded it draws `No Recording
Session` and `Press Any Key`, which is a modal: it waits for any key rather
than for MENU. What it draws with recordings stored has not been seen.

---

## Tone-Out for...

`id: 4293186024` &nbsp; `type: TypeSelect`

Waiting for a pager tone.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Tone-Out Standby | starts waiting | no, acts |
| Tone-Out Setup | [Select Tone-Out](#select-tone-out) | yes |

### Select Tone-Out

`id: 4293185972` &nbsp; `type: TypeSelect`

**Corrected from the first pass: there are thirty-two slots, not ten.**
`Tone-Out 0` through `Tone-Out 31`, each opening its own editor. The first pass
counted the nine rows the display shows and stopped there. `MSI` lists all 32.

Not descended into: they are editors.

---

## Waterfall

`id: 4293188348` &nbsp; `type: TypeSelect`

All eleven entries are reported. This is one of the screens `MSI` gets right.

The spectrum display.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Start Waterfall | starts it | no, acts |
| Start Preset Waterfall | starts it | no, acts |
| Start Custom Waterfall | starts it | no, acts |
| Program WF Custom Band | editor | no, editor |
| Edit Current | editor | no, editor |
| Set Signal (FFT) Display | value list: how the window splits between the graph and the waterfall | yes |
| Set Signal (FFT) Type | value list: `Line`, `Bar` | yes |
| Set Max Hold | value list: `On`, `Off` | yes |
| Set Max Hold Time | value list: `3 sec`, `10 sec`, `Infinite` | yes |
| Set Marker | [Set Marker](#set-marker) | yes |
| Set Color | [Set Color](#set-color-waterfall) | yes |

### Set Marker

`id: 4293186896` &nbsp; `type: TypeSelect`

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Marker Position | value list: `Position Adjustable`, `Fixed in Center Screen` | yes |
| Set Marker Width | value list: `Narrow`, `Default`, `Wide` | yes |

### Set Color (waterfall)

`id: 4293188204` &nbsp; `type: TypeSelect`

Distinct from the `Set Color` under Close Call's alert, which is a plain color
list. This one is a submenu of its own.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set WF Level 1 (Weak) | a color editor | yes |
| Set WF Level 2 | a color editor | assumed identical, not opened |
| Set WF Level 3 | a color editor | assumed identical, not opened |
| Set WF Level 4 | a color editor | assumed identical, not opened |
| Set WF Level 5 (Strong) | a color editor | assumed identical, not opened |
| Set Marker Color | a color editor | yes |
| Demo Screen | draws a demonstration | no, acts |
| Reset to Default Colors | puts the waterfall colors back | no, acts |

Each color editor is two rows: the color's name, and its value as
`RGB = RRGGBBh`. The knob presumably walks the palette. Nothing was chosen.

These screens break `radiocli screen` under `--output text`. They contain
bytes above 0x7E that are not valid UTF-8 on their own, and the text renderer
fails on them with a decode error. `--output json` reads them fine, because it
escapes each byte as the character of the same value. Worth fixing, or worth
knowing about.

---

## GPS

`id: 4293306412` &nbsp; `type: TypeSelect`

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| See GPS Information | readout: `Status:` and a state | yes |
| See Satellite Signal | readout, empty with GPS off | yes |
| Location Format | value list, two formats &nbsp; `id: 4293306248` | yes |
| Set GPS Function | value list: `Enable`, `Disable` &nbsp; `id: 4293306180` | yes |

Neither readout is a menu. With GPS switched off, `See GPS Information` draws
one status line and `See Satellite Signal` draws only its title, which is the
screen saying there is nothing to show rather than failing.

---

## Display Options

`id: 4293089732` &nbsp; `type: TypeSelect`

`MSI` omits `Customize`, and the protocol has no way to reach that menu either:
the spec notes there is no `DISPLAY_OPTIONS` id for `MNU`, so the customization
menu is invisible going in and coming out.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Backlight Options | [Backlight Options](#backlight-options) | yes |
| Customize | [Customize](#customize) | yes |
| Set ID Format (P25/Mot) | value list: `Decimal Format`, `Hex Format` | yes |
| Set ID Format (EDACS) | value list: `AFS Format`, `Decimal Format` | yes |
| Set Scan Display Mode | value list: `Simple Mode`, `Detail Mode` | yes |
| Set B/W or Color Mode | value list: `Color Mode`, `Black w/White Text`, `White w/Black Text` | yes |

`Set Scan Display Mode` is the one that changes the screen's geometry: Simple
Mode draws fourteen rows and Detail Mode seventeen. That is the same split
[screen-map.md](screen-map.md) is built around.

### Backlight Options

`id: 4293089228` &nbsp; `type: TypeSelect`

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Timer | [Select Event](#select-event) | yes |
| Set Dimmer | value list: `10%` through `100%` | yes |
| Set Key Backlight | value list: `Enable`, `Disable` | yes |
| Flash w/Backlight Off | value list: `On`, `Off` | yes |
| External Power | value list: `Backlight On`, `Backlight Off` | yes |

#### Select Event

What `Set Timer` opens: `Squelch` or `Keypress`, which is what the backlight
timer counts from. Neither branch was descended into, so whether each opens a
duration is unknown.

### Customize

`id: none` &nbsp; `type: TypeError`

Draws all eight rows perfectly well. `MSI` reports nothing.

The seven layout editors, whose contents are already mapped in
[screen-map.md](screen-map.md), plus a reset.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Simple Conventional | layout editor | covered by screen-map.md |
| Set Simple Trunk | layout editor | covered by screen-map.md |
| Set Detail Conventional | layout editor | covered by screen-map.md |
| Set Detail Trunk | layout editor | covered by screen-map.md |
| Set Search/CC Mode | layout editor | covered by screen-map.md |
| Set Weather Mode | layout editor | covered by screen-map.md |
| Set Tone Out Mode | layout editor | covered by screen-map.md |
| Restore Settings | [Restore](#restore) | yes |

### Restore

`id: none` &nbsp; `MSI` answers `null`

This does not act when it is opened, which the earlier pass assumed and recorded
as "no, acts." It is an ordinary eight-entry menu, and it is where a layout is
put back to stock one at a time rather than all together.

| Entry | Leads to |
| ----- | -------- |
| All Screens | confirmation, then restores every layout |
| Simple Conventional | confirmation, then restores that layout |
| Simple Trunk | confirmation, then restores that layout |
| Detail Conventional | confirmation, then restores that layout |
| Detail Trunk | confirmation, then restores that layout |
| Search/CC Mode | confirmation, then restores that layout |
| Weather Mode | confirmation, then restores that layout |
| Tone Out Mode | confirmation, then restores that layout |

The seven layout names here are the editor names from Customize with `Set `
removed, and they are in the same order, so one list serves both.

Choosing any of them draws a confirmation rather than acting:

```
  Confirm Restore?
  All Screens
  will be restored.


  Yes="E" / No="."
```

The second line is whichever entry was chosen. `E` is `enter` and `.` is `no`.
Confirming returns to this Restore menu rather than backing out, so several
layouts can be restored without walking in again.

Restoring `All Screens` on a scanner whose Simple Conventional had been
customized changed 31 of its 40 areas, every one of them back to `White on
Black` except the name and option rows, which came back `Orangered`. The other
layouts were already at stock and did not move. That is the first confirmation
that the assignments in [colors.md](colors.md) really are the factory defaults,
at least for this layout, which was open there.

Anything cached about the colors is wrong after this, for every layout the
restore covered rather than only the one on screen.

---

## Settings

`id: 4293092724` &nbsp; `type: TypeSelect`

`MSI` omits `Band Defaults` and `Restore Options`, reporting ten of the
twelve rows the display draws.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Adjust Key Beep | value list: `Auto`, `Level 1` to `Level 15`, `Off` &nbsp; `id: 4293089776` | yes |
| Set Clock | editor | no, editor |
| Upgrade | puts the radio into firmware update mode | no, acts |
| Battery Options | [Battery Options](#battery-options) | yes |
| Site NAC Operation | value list: `Use Site NAC`, `Ignore Site NAC` | yes |
| Band Defaults | [Band Defaults](#band-defaults) | yes |
| Auto Shutoff | value list: `Off`, `5 min` to `3 hour` &nbsp; `id: 4293090648` | yes |
| Bluetooth Options | [Bluetooth Options](#bluetooth-options) | yes |
| Headphone L/R output | value list: `In Phase`, `Invert Phase` | yes |
| Replay Options | [Replay Options](#replay-options) | yes |
| Restore Options | holds the factory reset | no, acts |
| See Scanner Information | [See Scanner Information](#see-scanner-information) | yes |

### Battery Options

`id: 4293090216` &nbsp; `type: TypeSelect`

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Battery Save | value list: `On`, `Off` | yes |
| Set Battery Low | [Set Battery Low](#set-battery-low) | yes |

#### Set Battery Low

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Alert | [Set Alert](#set-alert) | yes |
| Set Voltage | `TypeInput`, millivolts &nbsp; `id: 4293090052` | yes |

##### Set Alert

`id: 4293090112` is the parent `Set Battery Low`; this screen's own id was not read

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Alert Tone | not opened | no, assumed the same shape as the Close Call one |
| Set Alert Interval | not opened | no |
| Set Alert Volume | not opened | no |

### Band Defaults

`id: none` &nbsp; `type: TypeError`

Not a menu but a long list, one row per band, each giving the band's starting
frequency, its default modulation and its default step, in the form
`25.0:AM / 5.0 kHz`.

This is the table the radio falls back on wherever a modulation or a step is
set to `Auto`, which is why a search crossing from one band into another
changes both without being told to.

Whether a row opens an editor was not tested.

### Bluetooth Options

`id: 4293090912` &nbsp; `type: TypeSelect`

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Bluetooth Function | value list: `Enable`, `Disable` | yes |
| Edit Bluetooth Name | editor | no, editor |
| Pairing | starts pairing | no, acts |
| Reset Pairings | forgets paired devices | no, acts |

### Replay Options

`id: 4293170284` &nbsp; `type: TypeSelect`

`MSI` omits `Review Recordings`, reporting only `Set Replay Duration`.

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| Set Replay Duration | value list: `Off`, then seconds | yes |
| Review Recordings | a modal message, or the recordings | yes |

`Review Recordings` behaves like [Review WX Alerts](#review-wx-alerts): with
nothing recorded it draws `No Recording Session` and waits on `Press Any Key`.

### See Scanner Information

`id: 4293091692` &nbsp; `type: TypeSelect`

| Entry | Leads to | Explored |
| ----- | -------- | -------- |
| % Memory Used | readout: percent used, total size, free size | yes |
| Firmware Version | readout: three versions, `Main`, `Sub` and `BT` | yes |

Neither is a menu. `Firmware Version` reporting three separate versions is
worth noting, because `radiocli status` shows only the main one.

---

## Where the ids are missing

Twelve screens have no id, and they split into the two shapes described above:
Ten give the refusal shape, two give an empty document.

| Screen | Shape |
| ------ | ----- |
| Analyze | refusal |
| Review Avoids, under Manage Full Database | refusal |
| Review Avoids, under a favorites list | refusal |
| Freq Avoids | refusal |
| Review Discovery | refusal |
| Weather Alert | refusal |
| Review WX Alerts | refusal |
| Customize | refusal |
| Band Defaults | refusal |
| Review Recordings | refusal |
| Set Nationwide Systems | empty document |
| Information | empty document |

Everything else in this file carries an `id:` line. For those, matching on
`Name` is the safer choice, since the numbers look like firmware addresses.

## What is left

Nothing in the fixed menu tree is marked `not yet reached` any more. What
remains unopened is deliberate:

- **`no, acts`** entries, which do something the moment they are chosen.
- **`no, editor`** entries, which write over stored settings.
- The per-system half of the memory tree under a favorites list, which is
  described by shape above because it differs from radio to radio.
- A handful of leaves marked `assumed identical`, where one of a set was opened
  and the rest were taken on trust: waterfall color levels 2 to 5, search keys 2
  and 3.

## Things worth knowing, found while walking

Screen titles do not match the entry that opens them. `Manage Favorites`
opens `Select Favorites`. `Edit Location` opens `Select Location`. `Set Alert
Tone` opens `Set Tone`. `Set Alert Light` opens `Set Color`. `Tone-Out Setup`
opens `Select Tone-Out`. `Set Manual Location` opens `Input Latitude`. Anything
matching on titles has to know this.

The display truncates names and `MSI` does not. `Quick Save Favorites List`
is drawn as `Quick Save Favorit`. Where the two disagree, `MSI` has the real
name. This cost the first pass two errors on its own.

The nine-row window caused every counting mistake in the first pass. The
display shows nine rows onto a longer list, and stepping the knob to the end
wraps around to the top. A walk that stops when it sees the ninth row records
the wrong length. That is how thirty-two tone-out slots became ten, how a
ten-value delay list became nine, and how a favorites list menu lost two
entries. `MSI` gives the true count wherever it answers at all.

Four kinds of screen hide behind one kind of entry: a menu, a value list, a
readout, and a modal that waits on any key rather than on MENU. `MenuType`
separates the first two from the coordinate editor, but nothing distinguishes a
list of settable values from a list of places to go: both are `TypeSelect`.

**Two entries refuse outright when their source is switched off**, and they word
it differently. `Review Avoids` explains what to switch on. `Set Nationwide
Systems` says `Failure` first.

**The knob does three different things on the operating screen**, which the
second pass turned up by accident when the volume kept drifting during the walk.
While the radio is parked on a channel, each click steps to the next channel.
While it is stopped on a live transmission but not parked, clicks change the
volume. While it is stepping, clicks flip the direction arrow. `radiocli key
left` and `key right` drive that knob, so any walk that uses them on an
operating screen will move something. Both passes lost volume this way and had
to put it back.

**MENU discards.** Every value screen in this tree was entered and left with
MENU and nothing was written, including the `TypeLocation` coordinate editor.
The write happens on E/YES, not on leaving. This was checked by reading settings
back afterwards.

**The waterfall color editors break `radiocli screen` under `--output text`.**
They contain bytes above 0x7E that the text output passes through raw, so its
stdout is not valid UTF-8 and anything decoding it fails with a decode error.
`--output json` is unaffected, because it escapes each byte as the character of
the same value. Reproduce at MENU > Waterfall > Set Color > Set WF Level 1.

## How this was checked

Two passes.

The first walked the tree by knob and read the screen with `STS` through
`radiocli screen`, which is what produced the counting errors above.

The second re-walked every screen and read it twice, once with a raw `MSI`
straight off the port for the identity and the true item list, and once with
`STS` for what is actually drawn. Every disagreement between the two is recorded
in the screen's own section. Corrections the second pass made to the first are
marked **Corrected from the first pass** where they appear.

`MSI` is a query and writes nothing. Afterwards the radio was checked back to
where it started: scanning, volume and squelch unchanged, location and range
unchanged, two systems in the favorites list, priority scan still off.
