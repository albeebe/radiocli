# The remote command protocol

What the SDS150 will tell you over its serial port, what it will let you change,
and what it quietly refuses to do at all.

Worked out on an SDS150 running firmware 1.00.37, 2026-08-03 through
2026-08-05, against Uniden's published command specification for the SDS series
(V2.00, covering the SDS100, SDS200 and SDS150). Where the two disagree, the
radio wins and the disagreement gets written down.

The companion files: [screen-map.md](screen-map.md) for where things sit on the
display, [colors.md](colors.md) for what color they are drawn in, and
[oddities.md](oddities.md) for the running list of surprises.

---

## The wire

A line-oriented, request-response protocol over a serial port. One command per
line, one reply per line.

Every line ends in a carriage return rather than a newline, and one command
handled below breaks even that rule. Fields are comma separated, and the first
field of a reply repeats the command that caused it. Failures come back two
ways depending on the command: either the command name followed by `NG`, or the
command name followed by `ERR` and a code.

Two encoding rules will corrupt a parser that does not know them. A comma inside
a value is transmitted as a tab, in both directions, covering any value the
radio sends and any value written back to it; translate it or channel names with
commas will throw off the field count. And a screen line that is entirely spaces
arrives as an empty field rather than a run of spaces, so anything indexing into
it by column has to re-pad first.

### No documented command carries a request identifier

Nothing in a documented reply says which request produced it. Two programs
sharing one port therefore read each other's answers and neither can tell. That
is not a theoretical problem: running two commands concurrently produced two
failures that both looked like cable faults.

One undocumented command breaks this rule. `GS2` returns the screen with a
caller-supplied tag echoed back, which is exactly the correlation the rest of
the protocol lacks. It is the only command known to do so, and it covers only
the screen.

`TIOCEXCL`, which the Go serial library sets, does not prevent a second open of
a `/dev/cu.*` device on macOS. An advisory lock file was the fix.

## The command set, by what it is for

The published specification numbers these in the order they were added over
seven years, which is history rather than structure. Grouped by purpose they
fit in your head:

| Purpose | Commands |
| ------- | -------- |
| **Identify the radio** | `MDL` model, `VER` firmware |
| **Read the screen** | `STS` status, `GST` status with extras |
| **Read the state** | `GSI` scanner info, `GLT` list contents |
| **Move around** | `KEY` press a key, `JPM` jump to a mode, `JNT` jump to a number tag, `NXT` / `PRV` step, `HLD` hold, `QSH` quick search hold |
| **Change what is scanned** | `AVD` avoid, `SVC` service types, `FQK` / `SQK` / `DQK` quick keys |
| **Drive the menus** | `MNU` open, `MSI` read, `MSV` set, `MSB` back |
| **Settings** | `VOL`, `SQL`, `DTM` clock, `LCR` location and range, `URC` recording |
| **Spectrum** | `GWF` / `GW2` waterfall FFT, `PWF` push FFT |
| **Analysis** | `AST` start, `APR` pause and resume |
| **Power** | `POF` power off, `GCS` charge status, `KAL` keep alive |
| **Push mode** | `PSI` scanner info, `PWF` FFT |

Two of those are _push_ commands rather than requests: turn them on and the
radio streams until told otherwise. Everything else is polled.

Nothing in the list touches display color. That absence is covered at the
end.

---

# Reading the radio

## `MDL` and `VER`

```
->  MDL
<-  MDL,<model>
```

The model comes back as `SDS100`, `SDS200` or `SDS150GBT`. Mind the suffix on
the SDS150, because string-matching on an exact `SDS150` misses every time.

```
->  VER
<-  VER,Version x.xx.xx
```

## `STS` and `GST`: the screen

Both return the whole display as text plus per-character attributes. `GST` is
the newer one and returns everything `STS` does with more besides.

```
->  STS
<-  STS,<form>,<line 1 text>,<line 1 attributes>, ... ,<nine trailing fields>

->  GST
<-  GST,<form>,<line 1 text>,<line 1 attributes>, ... ,<fourteen trailing fields>
```

The form field is the key to parsing the rest. It is a string of 5 to 20
digits, one per screen line, and its length tells you how many text and
attribute pairs follow. Each digit says how that line is drawn:

| Digit | Font | Characters per line |
| ----- | ---- | ------------------- |
| `0` | Normal | 30 |
| `1` | Large | 24 |

A large-font line takes two rows of the physical display, which matters
enormously for anything mapping positions. See [screen-map.md](screen-map.md).

The attribute string carries one character per screen character:

| Character | Meaning |
| --------- | ------- |
| space | Drawn normally |
| `*` | Reverse video |
| `_` | Underlined |

The attribute string can be longer than the trimmed text, because a highlight
bar is drawn across the remainder of a row. Pad the text out to the attribute
length before indexing into both, or the highlight will appear to start in the
wrong place.

### `GST` is strictly better than `STS`

Same screen, plus both LED colors and a display mode field, in one round trip.
There is no reason to reach for `STS` except habit. Its extra fields:

| Field | Meaning |
| ----- | ------- |
| `MUTE` | Mute state |
| `LED1` | Alert LED color |
| `LED2` | Battery charge LED color. **SDS150 only** |
| `WF_MODE` | 0 normal, 1 waterfall, 2 menu or direct entry |
| `FREQ` | Marker frequency |
| `MOD` | Modulation |
| `MF_POS` | Marker position |
| `CF` | Center frequency |
| `LOWER`, `UPPER` | Band edges |
| `COLOR_MODE` | Global display mode. See below |
| `FFT_SIZE` | 0 through 3, meaning 25, 50, 75 and 100 percent |

Both LED fields use the same color scale: 0 off, then blue, red, magenta, green,
cyan, yellow, white.

### `COLOR_MODE` is the global display mode, not a waterfall setting

The specification labels this field "Waterfall display only". That label is
wrong, and it matters more than any other error found.

The radio has a whole-display setting under
`MENU -> Display Options -> Set B/W or Color Mode`. Selecting each of its three
entries and reading `GST` immediately afterwards, with the waterfall never
opened:

| Menu entry | `COLOR_MODE` |
| ---------- | ------------ |
| Color Mode | `0` |
| Black w/White Text | `1` |
| White w/Black Text | `2` |

So this is the one piece of the display's appearance the protocol does report,
and it is the piece that decides whether the per-area colors in
[colors.md](colors.md) are being drawn at all. Anything redrawing the screen has
to read it, or it paints a color display while the radio in your hand is
monochrome.

It is read-only over serial. Changing it means walking the menu with key
presses, since that menu has no id for `MNU`. The cursor opens on whichever entry
is currently active, which is a second way to read the setting.

### The nine trailing fields of `STS`

The specification marks all nine as reserved and says nothing further. They are
not reserved. Eight carry data and two are genuinely empty.

The document's own history records that it began as a derivative of the
BCD536HP/BCD436HP specification, and that older document ends its status reply
with exactly nine fields, named. Reading the SDS150's nine against those names,
scanning, nothing being received:

```
0 , 1 , 0 , 0 , , , 0 , OFF , 3
```

Cross-checked against a scanner info read taken in the same state, which reported
`VOL="15" SQL="2" Sig="0" Att="Off" Mute="Mute" Backlight="100" A_Led="Off"
Rssi="-999"`:

| # | Name | Value | What corroborates it |
| - | ---- | ----- | -------------------- |
| 1 | Squelch status | `0` | Closed, consistent with a signal of 0 and an RSSI of -999. **This is status, not level**, which was 2 |
| 2 | Mute | `1` | Matches `Mute="Mute"` |
| 3 | Battery low | `0` | Not low, measured 4.18 V |
| 4 | Weather alert | `0` | No alert active |
| 5 | reserved | empty | genuinely reserved |
| 6 | reserved | empty | genuinely reserved |
| 7 | Signal level | `0` | Matches `Sig="0"` |
| 8 | Backlight color | `OFF` | See below |
| 9 | Backlight dimmer | `3` | Matches `Backlight="100"`, and flips between `3` and `0` with the light |

This rules out the obvious competing reading. Since `GST` ends with
`MUTE, LED1, LED2, ...`, one might guess `STS` carries the same list truncated.
It does not:

- Field 2 would be the alert LED, and `1` would mean blue, but the radio reported
  the alert LED off.
- Field 1 would be mute, and `0` would mean unmuted, but the radio reported
  muted.
- Field 8 would be a center frequency, and `OFF` is not a frequency.

The older reading fits all nine values. The newer one fails on three.

Confidence is high, with one caveat worth stating: the _names_ are inherited
from the older specification rather than confirmed by this one. The values and
the field count are directly observed.

### The backlight color field is a dead end

Field 8 is the only color-carrying field in the entire status reply, and it goes
nowhere.

On the BCD436HP and BCD536HP the display was monochrome with a colored
backlight the user could choose. That field reported the chosen color. The SDS
series has a full color LCD and no backlight-color setting at all, so the field
is vestigial. It read `OFF` across the backlight being switched on and off, and
across both a scanning screen and a menu screen.

Even on the radios where it did something, it was one color for the entire
backlight. It was never per-element color.

## `GSI` and `PSI`: the state

Both return the same XML document describing everything the radio is currently
doing. `GSI` answers once; `PSI` streams.

The specification says `PSI` takes a parameter controlling the push interval but
never says what it is. That parameter remains undocumented.

### What is always there

A root element, plus `Property`, `AGC`, `DispFormat`, a `ViewDescription` when an
override area is showing, and a `ReplayDescription` in replay mode.

### What appears depends on mode

| Element | Present in |
| ------- | ---------- |
| `MonitorList`, `System`, `Department` | conventional scan, trunk scan, custom with scan, CC hits with scan |
| `Site`, `TGID`, `SiteFrequency` | trunk scan |
| `ConvFrequency` | conventional scan |
| `SrchFrequency` | custom with scan, custom search, quick search, close call, weather alert, reverse frequency, repeater find |
| `CcHitsChannel` | CC hits with scan |
| `DualWatch` | all scan and search modes, close call searching, reverse frequency, repeater find |
| `SearchRange` | custom with scan, custom search, quick search |
| `SearchBanks` | custom search |
| `CC_Bands`, `CC_Counters` | close call searching |
| `ToneOutChannel` | tone out |
| `WxChannel`, `WxMode` | weather alert |
| `ConventionalDiscovery` | conventional discovery |
| `TrunkingDiscovery` | trunking discovery |
| `SystemStatus` | analyze system status |
| `Analyze` | analyze |
| `WaterfallBand`, `WaterfallSettings` | waterfall |

### Mode and view

Two attributes describe what the radio is doing, and they answer different
questions.

Mode is one of: Scan Mode, Scan Hold, Tone-Out, Custom Search, Custom Search
Hold, Quick Search, Quick Search Hold, Service Scan, Service Scan Hold, Trunk
Scan, Trunk Scan Hold, Close Call Only, Close Call, Menu tree.

The list is not complete. An undocumented command puts the radio into a mode
reporting `Remote Mode`, which is not among those fourteen. Anything matching
exhaustively on this list will fall through.

Every hold state ends in the word Hold, which is the only reliable way to
detect one. A held radio is otherwise indistinguishable from a scanning one: it
is out of the menus, answers every command correctly, and shows a channel name
where a scanning radio would say `Scanning...`. But a scanning radio shows a
channel name too, every time it stops on something. That cost real time before it
was noticed.

View is one of: plain text, conventional scan, trunk scan, custom with scan,
CC hits with scan, custom search, quick search, close call, CC searching, tone
out, weather alert, conventional discovery, trunking discovery, reverse
frequency, repeater find, direct entry, menu selection, menu input, analyze
system status, analyze.

The radio reports plain text transiently. Right after a command moves it, the
view reads plain text for a moment before settling into what it is actually
doing. A command issued immediately after another will see it and conclude the
radio is on a screen it knows nothing about. Let it settle; three seconds is
enough.

### `Property` and friends

| Attribute | Values |
| --------- | ------ |
| `F` | Off, On |
| `VOL` | 0-15, or 0-29 on the SDS200 |
| `SQL` | 0-15, or 0-19 on the SDS200 |
| `Sig` | 0-4 |
| `WiFi` | Off, 0-3, AP |
| `Battery` | 0.0-3.3 |
| `Att` | Off, On, G-Att |
| `Rec` | Off, On |
| `KeyLock` | Off, On |
| `P25Status` | None, Data, P25, DMR, CAP, CON, DT3, XPT, NX9, NX4, ND9, ND4, IDS, NXD |
| `Mute` | Unmute, Mute |
| `A_Led` | Off, Blue, Red, Magenta, Green, Cyan, Yellow, White |
| `Dir` | Up, Down |
| `Rssi` | 0 and up |

`Backlight` shows up in examples as a percentage but is missing from the
attribute list. It is real; treat it as a percentage.

`AGC` carries analog and digital AGC flags. `DualWatch` carries priority, close
call and weather settings, each off, do-not-disturb or priority (weather being
off or priority only).

### The scan hierarchy

| Element | Attributes |
| ------- | ---------- |
| `MonitorList` | Name (up to 64 ASCII), Index, ListType (FullDb, FL, SWS), quick key, number tag, `DB_Counter` |
| `System` | Name, Index, Avoid, SystemType, quick key, number tag, Hold |
| `Department` | Name, Index, Avoid, quick key, Hold |
| `Site` | Name, Index, Avoid, quick key, Hold, Mod (Auto, NFM, FM) |

Avoid is always one of `Off`, `T-Avoid` or `Avoid`.

System types: Conventional, Motorola, EDACS, LTR, P25 Trunk, P25 One Frequency,
MotoTRBO Trunk, DMR One Frequency, NXDN Trunk, NXDN One Frequency.

`ConvFrequency` carries name, index, avoid, frequency as `xxxx.xxxxMHz`,
modulation (Auto, AM, NFM, FM, WFM, FMB), number tag, hold, service type,
priority channel, the three sub-audio fields, record slot, level, IF exchange,
talkgroup id and unit id.

`TGID` carries name, index, avoid, the id itself, set and record slots, number
tag, hold, service type, priority channel and level.

### The rest of the elements

| Element | Attributes |
| ------- | ---------- |
| `SiteFrequency` | Freq, sub-audio setting, sub-audio decode, IF exchange |
| `SearchBanks` | Index 0-9, ten-digit bank status, Name, bank number |
| `CC_Bands` | Seven-digit band status |
| `SrchFrequency` | Avoid, Freq, Mod, Hold, sub-audio decode, record slot, TGID, unit id, IF exchange |
| `CcHitsChannel` | Name, Index, Avoid, channel number 0-9, Freq, Mod, Hold, sub-audio decode, level, IF exchange |
| `SearchRange` | Lower, Upper, Mod, Step |
| `ToneOutChannel` | Name, Index, channel number 0-31, Freq, Mod, Hold, level, IF exchange, two tones |
| `WxMode` | Monitor Weather or Weather Alert, plus SAME |
| `WxChannel` | Name, Index, channel 1-7, Freq, Mod, Hold, level, IF exchange |
| `ConventionalDiscovery` | Lower, Upper, Mod, Step, Freq, sub-audio, record slot, elapsed time, hit count, TGID, unit id |
| `TrunkingDiscovery` | System and site names, TGID and its name, sub-audio, record slot, elapsed time, hit count, unit id |
| `SystemStatus` | System and site names, signal, quality and activity each 0-100, system and sub ids, site id, WACN, NAC, DMR color code 0-15, RAN 0-63, area, attenuator, frequency count, P25 status |
| `RfPowerPlot` | Frequency, modulation, sample rate (100, 200, 400 or 800 ms), attenuator, 34 bins each 0-100 |
| `Analyze` | Two message fields, system and site names, attenuator |
| `WaterfallBand` | Lower, Center, Upper, Mod, Step, Span, Limit |
| `WaterfallSettings` | Marker frequency, gain (Auto or 0-15) |

### Overrides and popups

`ViewDescription` carries the two info areas at the top of the screen, which show
quick key status while scanning and bank status while custom searching, plus an
overwrite area that replaces the channel name with a message such as
`No thing to scan`.

It also carries popups. Two kinds, behaving differently:

- A plain popup is transient, one or two seconds, like a toast notification.
- A popup with buttons is not cleared automatically and waits for the user.
  Each button carries its label and the key code that activates it, so an
  application can offer the same choice and send the corresponding key press.

The USB mode prompt is the button kind, and it matters later.

`PlainText` is a whole-screen view, one element per line, used when the radio has
nothing structured to report.

### Two rules that bite

_Identifiers._ Systems, departments, sites and channels that came from the full
database carry an id relating them to RadioReference records, and so do copies
made from database entries. User-created entries do not. Neither does Search
with Scan.

| Database id | What it identifies | RadioReference |
| ----------- | ------------------ | -------------- |
| CountyId | Conventional system, county | ctid |
| AgencyId | Conventional system, agency | aid |
| TrunkId | Trunked system | sid |
| CGroupId | Conventional department | scid |
| CFreqId | Conventional frequency | fid |
| SiteId | Trunked site | siteId |
| TGroupId | Trunked department | tgCid |
| Tid | Trunked channel | tgId |

Indexes go stale. The index used to hold or avoid something is assigned when
data is loaded into RAM, and is invalid the moment the monitor list's counter
changes. Comparing that counter is the documented way to notice, rather than
acting on an index that now points somewhere else.

Names are ASCII in the printable range, 64 characters maximum.

## `GLT`: reading lists

One command per kind of list, most taking a parent index:

```
->  GLT,FL                        favorites lists
->  GLT,SYS,<fl index>            systems in a list
->  GLT,DEPT,<system index>       departments
->  GLT,SITE,<system index>       sites
->  GLT,CFREQ,<dept index>        conventional frequencies
->  GLT,TGID,<dept index>         talkgroups
->  GLT,SFREQ,<site index>        site frequencies
->  GLT,AFREQ                     avoided search frequencies
->  GLT,ATGID,<system index>      avoided search talkgroups
->  GLT,FTO                       fire tone out
->  GLT,CS_BANK                   custom search banks
->  GLT,UREC                      recording folders
->  GLT,IREC_FILE                 internal recordings
->  GLT,UREC_FILE,<folder index>  recordings in a folder
->  GLT,TRN_DISCOV                trunk discovery sessions
->  GLT,CNV_DISCOV                conventional discovery sessions
```

The reply is XML, one element per row. What each row carries:

| List | Columns |
| ---- | ------- |
| Favorites lists | Index, Name, Monitor, quick key, number tag |
| Systems | Index, database id, Name, Avoid, Type, quick key, number tag |
| Departments | Index, database id, Name, Avoid, quick key |
| Sites | Index, database id, Name, Avoid, quick key |
| Conventional frequencies | Index, database id, Name, Avoid, Freq, Mod, sub-audio setting and level, service type, number tag |
| Talkgroups | Index, database id, Name, Avoid, TGID, audio type, service type, number tag |
| Site frequencies | Index, Freq |
| Avoided frequencies | Freq, Avoid |
| Avoided talkgroups | TGID, Avoid, Index, Name, department name and index |
| Fire tone out | Index, Freq, Mod, Name, two tones |
| Custom search banks | Index, Name, Lower, Upper, Mod, Step |
| Recording folders | Index, Name |
| Recordings | Index, Name, Time |
| Trunk discovery | Name, Delay, Logging, Duration, compare flag, system name and type, site name, timeout, autostore |
| Conventional discovery | Name, Lower, Upper, Mod, Step, Delay, Logging, compare flag, Duration, timeout, autostore |

A favorites list read looks like this:

```xml
<GLT>
  <FL Index="0" Name="Favorites List 1" Monitor="On" Q_Key="1" N_Tag="None" />
  <FL Index="1" Name="Favorites List 2" Monitor="On" Q_Key="2" N_Tag="2" />
  <FL Index="2" Name="Favorites List 3" Monitor="Off" Q_Key="3" N_Tag="999" />
</GLT>
```

The sub-audio setting covers CTCSS, DCS, P25 NAC, DMR color code, NXDN RAN and
area, depending on the system.

### A long list is cut short, and there is no way to ask for the rest

The reply is capped at roughly a kilobyte. When the list does not fit, the
scanner ends the document early and says so in a footer:

```xml
  <Footer No="1" EOT="0"/>
```

`EOT="0"` means this is not the end. A document that did finish carries
`EOT="1"`, and some carry no footer at all.

Nothing found asks for the second part. Sending the same command again
answers with the same first part, six times out of six. Putting a number in the
request, `GLT,CS_BANK,2`, is ignored and answers with the first part too. Once,
out of many attempts, a second document arrived carrying `No="2" EOT="1"` and
the missing rows; it has not been reproduced, and reads like a late reply to an
earlier request rather than an answer to that one.

Measured on the custom search banks, which are ten rows of about 108 bytes.
Nine come back and the tenth does not, so how many rows survive depends on how
long the names are, not on how many rows there are. Anything reading a list
this way has to check what it got against what it expected rather than trusting
the document to be complete.

Confirmed again on a department of forty CB channels, which are `CFREQ` rows of
about the same size: seven came back. That one cost something, because nothing
was checking the footer at the time. `radiocli channels` reported seven of the
forty and said nothing about the rest, in text and in JSON alike, which reads
exactly like a department holding seven channels. The count was only caught by
cross-checking against `scanning`, which enumerates from a different request.

Every list read now checks the footer, and the ones that have to be complete are
finished the same way `banks` always did it: by walking that list's own menu and
reading the names off the screen. The knob reaches every entry whether or not the
protocol will report it. What the screen cannot supply is everything the row
carried besides the name, so those entries are marked as partly read rather than
filled in with guesses.

---

# Moving the radio

## Target keywords

Five commands (`GLT`, `NXT`, `PRV`, `HLD` and `AVD`) all address things by the
same set of keywords, but each command accepts a different subset, and the
parameters mean different things per command. This matrix is the part worth
having in front of you:

| Target | Keyword | List by | Step by | Step parent | Hold by | Hold parent | Avoid by |
| ------ | ------- | ------- | ------- | ----------- | ------- | ----------- | -------- |
| Favorites list | `FL` | none | no | no | **no** | | **no** |
| System | `SYS` | parent list | index | none | index | none | index |
| Department | `DEPT` | parent system | index | parent system | index | parent system | index |
| Site | `SITE` | parent system | index | none | index | none | index |
| Conventional frequency | `CFREQ` | parent dept | index | none | index | none | index |
| Talkgroup, ID scan | `TGID` | parent dept | index | none | index | none | index |
| Talkgroup, ID search | `STGID` | no | TGID | site index | TGID | site index | use `ATGID` |
| Site frequency | `SFREQ` | parent site | no | | **no** | | **no** |
| Avoided search talkgroup | `ATGID` | parent system | no | | no | | TGID, parent system |
| Avoided search frequency | `AFREQ` | none | no | | no | | frequency |
| Close call | `CC` | no | none | none | none | none | use `AFREQ` |
| Weather | `WX` | none | index | none | index | none | no |
| Tone out | `FTO` | none | index | none | index | none | no |
| Search with scan | `SWS_FREQ` | no | frequency | parent dept | frequency | parent dept | use `AFREQ` |
| Close call hit | `CCHIT` | parent dept | index | none | index | none | index |
| Custom search bank | `CS_BANK` | none | no | | no | | no |
| Custom search frequency | `CS_FREQ` | no | frequency | parent bank | frequency | parent bank | use `AFREQ` |
| Quick search frequency | `QS_FREQ` | no | frequency | none | frequency | none | use `AFREQ` |
| Repeater find frequency | `RPTR_FREQ` | no | frequency | none | frequency | none | cannot |
| Internal recording | `IREC_FILE` | none | none | file index | file index | none | cannot |
| Recording folder | `UREC_FOLDER` | none | cannot | | | | cannot |
| Recording file | `UREC_FILE` | folder index | file index | none | file index | none | cannot |
| Trunk discovery | `TRN_DISCOV` | none | no | | no | | TGID |
| Conventional discovery | `CNV_DISCOV` | none | no | | no | | frequency |
| Band scope | `BAND_SCOPE` | no | frequency | none | frequency | none | no |

Three traps live in that table:

1. Avoiding a search frequency uses the avoid keyword, not the search
   keyword. To avoid 406.0 MHz found in quick search, address it as an avoided
   frequency. Addressing it as a quick search frequency is wrong and will not
   work.
2. Holding or stepping while in repeater find cancels repeater find and
   returns the radio to whatever it was doing before.
3. The "Unknown" department in ID search is virtual. It can be held and
   stepped but not avoided, and unlike every other department it requires its
   parent system index.

## `KEY`: press a key

```
->  KEY,<code>,<mode>
<-  KEY,OK
```

| Code | Key |
| ---- | --- |
| `M` | Menu |
| `F` | Function |
| `L` | Avoid |
| `0` to `9` | Number keys |
| `.` | Dot, also No |
| `E` | Enter, also Yes |
| `>` | Rotary knob right |
| `<` | Rotary knob left |
| `^` | Rotary knob push |
| `V` | Volume knob push, which is Backlight on this model |
| `Y` | Replay |
| `A`, `B`, `C` | Soft keys 1, 2 and 3 |
| `Z` | Zip |
| `Q` | Squelch knob push. **No such key on this model** |
| `T` | Service type. **No such key on this model** |
| `R` | Range. **No such key on this model** |

The radio answers `KEY,OK` for every code it is sent, including codes with no
corresponding key. A successful reply is not evidence that anything happened.
The three keys above are accepted and silently discarded.

On earlier models the three soft keys were labeled System, Department and
Channel; here they take whatever labels the current mode gives them.

## `JPM`: jump to a mode

```
->  JPM,<mode>,<index>
<-  JPM,OK
```

| Mode | Index means |
| ---- | ----------- |
| `SCN_MODE` | Channel index |
| `WX_MODE` | `NORMAL`, `A_ONLY`, `SAME_1` through `SAME_5`, or `ALL_FIPS` |
| `UREC_MODE` | Folder name |
| `TDIS_MODE`, `CDIS_MODE` | Session name |
| `CTM_MODE`, `QSH_MODE`, `CC_MODE`, `FTO_MODE`, `WF_MODE`, `IREC_MODE` | reserved |

Sending a channel index of all ones (`0xFFFFFFFF`) starts scanning from the top
channel, which is the reliable way to say "just start scanning" without knowing
where the radio currently is. That is what releases a hold.

If a temporary clock has been set, both discovery mode and weather alert mode
refuse.

## `HLD`, `NXT`, `PRV`, `JNT`, `QSH`

```
->  HLD,<keyword>,<param>,<parent>
->  NXT,<keyword>,<param>,<parent>,<count>
->  PRV,<keyword>,<param>,<parent>,<count>
->  JNT,<list tag>,<system tag>,<channel tag>
->  QSH,<frequency>
```

Hold works on a system, department or channel. It cannot hold a favorites list
or a site frequency.

Step count runs 1 to 8, and is how many entries to move at once.

Number tags run 0-99 for lists and systems, 0-999 for channels.

Quick search hold is refused while in the menus, during direct entry, or during
a quick save.

## `AVD`: avoid

```
->  AVD,<keyword>,<param>,<parent>,<status>
<-  AVD,OK
```

| Status | Effect |
| ------ | ------ |
| `1` | Avoid permanently |
| `2` | Avoid temporarily |
| `3` | Stop avoiding |

As with hold, a favorites list and a site frequency cannot be avoided. Read the
result back through the scanner info or a list read.

This is the command that would let the tool avoid an individual system, which
the CLI cannot currently do.

## Quick keys

```
->  FQK                              read favorites list quick keys
->  FQK,<s0>, ... ,<s99>             write them
->  SQK,<list key>                   read a system's
->  DQK,<list key>,<system key>      read a department's
```

Each of the hundred status values is one of:

| Value | Meaning |
| ----- | ------- |
| `0` | The quick key does not exist |
| `1` | It exists and is disabled |
| `2` | It exists and is enabled |

Writing `0` is ignored, so a write cannot delete a quick key, only enable or
disable one.

---

# Driving the menus

Four commands, which together are how anything not otherwise exposed gets done.
Everything about display color goes through here.

```
->  MNU,<menu id>,<index>     open a menu by name
->  MSI                       read the current menu
->  MSV,<reserved>,<value>    set the selected item
->  MSB,<reserved>,<level>    go back
```

Menus that can be opened by name:

| Id | Menu |
| -- | ---- |
| `TOP` | The main menu |
| `MONITOR_LIST` | Select lists to monitor |
| `SCAN_SYSTEM`, `SCAN_DEPARTMENT`, `SCAN_SITE`, `SCAN_CHANNEL` | The corresponding menu, by index |
| `SRCH_RANGE` | Custom search bank, by index |
| `SRCH_OPT` | Search and close call options |
| `CC`, `CC_BAND` | Close call and its band menu |
| `WX` | Weather operation |
| `FTO_CHANNEL` | Tone out channel, by index |
| `SETTINGS` | Settings |
| `BRDCST_SCREEN` | Broadcast screen |

There is no id for Display Options, so that entire menu, including everything
about color, can only be reached by pressing keys.

Going back takes either an empty level, meaning one step, or a value meaning
leave the menus entirely. That value is misspelled in the specification, and
the radio accepts the misspelling. Anything talking to the radio has to
misspell it too.

## Reading a menu

The menu read returns XML: a title, an index, a menu type (select, input,
location or error), the current value and the current selection, then its items.
Input menus carry a maximum length and which keys are enabled; location menus add
a flag saying whether they want latitude or longitude; error menus carry the
message and whether the scan button is available.

The item list is truncated, without saying so.

| Menu | Real size | Items listed |
| ---- | --------- | ------------ |
| A display layout editor | up to 50 | 20 |
| A color picker | 147 | 26 |
| The Customize menu | 8 | none at all |
| A favorites list menu | all | omits one entry |

A color picker's listing also does not mark which entry is current, so it
cannot be used to locate the selection either.

Two separate mistakes traced back to trusting a listing: a walk that stopped at
20 items believing it was done, and a wrap test that computed zero steps and
proved nothing. The screen is the only complete source. Anything counting
positions in a long menu has to count them there.

## Setting a value

The set command takes either the index of an item in a select menu or the text
for an input menu, with the usual comma-to-tab rule.

It is refused inside a color picker, by name and by index alike:

```
->  MSV,Cyan   <- rejected in its current mode
->  MSV,22     <- rejected in its current mode
```

So the knob is the only way to move a picker, one color at a time. That is the
single largest cost in setting a color and it is not avoidable.

---

# Settings

## `VOL` and `SQL`

```
->  VOL           read
->  VOL,<level>   write
```

| Command | SDS100 and SDS150 | SDS200 |
| ------- | ----------------- | ------ |
| Volume | 0-15 | 0-29 |
| Squelch | 0-15 | 0-19 |

A successful write is acknowledged with a bare command name, not with `OK`.
Parsers expecting the usual acknowledgement will treat a success as a
malformed reply.

## `DTM`: clock

```
->  DTM
->  DTM,<dst>,<year>,<month>,<day>,<hour>,<minute>,<second>
<-  DTM,<dst>,<year>,<month>,<day>,<hour>,<minute>,<second>,<rtc status>
```

The trailing status is `0` if the real-time clock is bad, `1` if it is good. Note
that a read has one more field than a write.

## `LCR`: location and range

```
->  LCR
->  LCR,<latitude>,<longitude>,<range>
```

Coordinates are in degrees.

## `SVC`: service types

```
->  SVC                                     read
->  SVC,<47 values>                         write
```

Each value is `0` for not scanned or `1` for scanned. The wire order is not the
order the menu displays, which is alphabetical. See the table at the end.

## `URC`: recording

```
->  URC              read
->  URC,<status>     0 stop, 1 start
```

| Error | Meaning |
| ----- | ------- |
| `0001` | File access failed |
| `0002` | Battery too low |
| `0003` | Session limit reached |
| `0004` | Real-time clock lost |

---

# Streaming, power and spectrum

## The three push paths

Most of this protocol is polled, which forces timing jitter on anything watching
the radio. Three commands push instead:

| Command | What it streams |
| ------- | --------------- |
| `PSI` | Scanner info XML, periodically |
| `PWF` | Waterfall FFT, 240 points |
| `AST` | Analysis data, interval depending on mode |

That is a materially different design for anything showing live state, and it is
easy to miss because the polled equivalents are documented first.

## Waterfall

```
->  PWF,<type>,<on/off>     push, type 1 is the displayed FFT
->  GWF,<type>,<on/off>     poll, 240 comma-separated points
```

There is a third form that returns the same 240 points as binary with no
separators, which is what you want for anything drawing at speed. The
specification names the polled command in both the request and the response for
it, which looks like a copy-and-paste slip: the separator-free form has its own
name.

## `KAL`: keep alive

```
->  KAL
```

The radio sends nothing back. Anything that waits for a reply after every
command will hang on this one until it times out. It exists for push mode, where
there is otherwise no traffic to keep a connection alive.

## `POF` and `GCS`

Power off is unremarkable. Charge status is not:

```
->  GCS
<-  GCS,CST=<state>,VOLT=<mV>:<percent>%,CURR=<mA>,TEMP=<C>
```

| State | Meaning |
| ----- | ------- |
| 0 | No external power, or no battery |
| 1 | The gauge chip is initializing |
| 2 | Temperature out of range |
| 3 | Power out of range |
| 4 | Fully charged |
| 5 | Recharging |
| 6 | Charging |

Current is positive while charging and negative while discharging. A real reply:

```
CST=4,VOLT=4184mV:100%,CURR=0000mA,TEMP= 27.65C
```

This reply ends with a newline rather than a carriage return, alone among
every command here. A reader splitting strictly on carriage returns will wait
forever for a line it already has.

### The documented reply is wrong in two ways

*2026-08-05, noticed while sweeping the G block.*

A later read returned this:

```
CST=8,VOLT=4183mV:100%,CURR=0000mA,TEMP=  35.05C,1202
```

There is a sixth field the specification does not mention, here `1202`. It
is undocumented, and anything parsing this reply by counting fields will meet it
eventually.

And the state was `8`, which is outside the documented range. The table above
runs 0 to 6 and has no 8 in it. The radio was plugged into USB, fully charged, at
35 degrees, so `8` plausibly means something like charged-and-running-on-external
-power, but nothing here says so.

Both readings came off the same radio and the same firmware, hours apart. Treat
the state table as incomplete and do not assume the field count.

## `AST` and `APR`: analysis

```
->  AST,<mode>,<parameters>
->  APR,<mode>                  pause or resume
```

| Mode | Parameters | How often it reports |
| ---- | ---------- | -------------------- |
| Current activity | site index | every 200 ms |
| LCN monitor | site index | every second |
| LCN finder | site index | every 500 ms |
| Activity log | site index | on each event |
| Band scope | center, span, step, modulation | every 10 ms |
| Raw data output | frequency, modulation, filter, attenuator | continuous |
| System status | site index | through the push channel |
| RF power plot | frequency, modulation, sample rate | through the push channel |

Every analysis mode requires the radio to be in scan mode first, so that the
database is loaded. Starting one from elsewhere will not work.

Current activity reports one row per logical channel:

```xml
<AST>
  <CurrentActivity LCN="1" Freq="851.0125" SystemID="0001h" SiteID="0" TgidType="Control Channel" />
  <CurrentActivity LCN="2" Freq="851.0375" TGID="16" UnitID="32" MOD="Analog" TgidType="TGID" />
</AST>
```

Modulation is analog, digital or encrypted. Channel type is control channel,
encrypted, patch, unknown, talkgroup or individual call.

The activity log returns a timestamped message and description per event, and
the message and description vocabularies differ per system type, separately
defined for Motorola, P25, EDACS, LTR, DMR and NXDN. That is a large body of
per-system detail; consult the published specification when a decoder needs it.

Raw data output is 10-bit signed discriminator samples, split across high and
low bytes. It is USB only, and other commands need the stream paused before
they will be answered.

Band scope takes a center frequency from 250 kHz to 1.3 GHz, a span from 200 to
8000, and a step from 500 to 10000. The SDS200 accepts wider spans that this
model does not. It reports a frequency and an RSSI from 0 to 100 each time it
moves.

---

# Service types, in wire order

This is the order the 47 service type fields are transmitted in, which is not
the alphabetical order the menu shows them in. Anything reading or writing them
by position needs this table and cannot derive it from the menu.

| Position | Slot | Name |
| -------- | ---- | ---- |
| 0 | Preset 1 | Multi-Dispatch |
| 1 | Preset 2 | Law Dispatch |
| 2 | Preset 3 | Fire Dispatch |
| 3 | Preset 4 | EMS Dispatch |
| 4 | Preset 5 | *unused* |
| 5 | Preset 6 | Multi-Tac |
| 6 | Preset 7 | Law Tac |
| 7 | Preset 8 | Fire-Tac |
| 8 | Preset 9 | EMS-Tac |
| 9 | Preset 10 | *unused* |
| 10 | Preset 11 | Interop |
| 11 | Preset 12 | Hospital |
| 12 | Preset 13 | Ham |
| 13 | Preset 14 | Public Works |
| 14 | Preset 15 | Aircraft |
| 15 | Preset 16 | Federal |
| 16 | Preset 17 | Business |
| 17 | Preset 18 | *unused* |
| 18 | Preset 19 | *unused* |
| 19 | Preset 20 | Railroad |
| 20 | Preset 21 | Other |
| 21 | Preset 22 | Multi-Talk |
| 22 | Preset 23 | Law Talk |
| 23 | Preset 24 | Fire-Talk |
| 24 | Preset 25 | EMS-Talk |
| 25 | Preset 26 | Transportation |
| 26 | Preset 27 | *unused* |
| 27 | Preset 28 | *unused* |
| 28 | Preset 29 | Emergency Ops |
| 29 | Preset 30 | Military |
| 30 | Preset 31 | Media |
| 31 | Preset 32 | Schools |
| 32 | Preset 33 | Security |
| 33 | Preset 34 | Utilities |
| 34 | Preset 35 | *unused* |
| 35 | Preset 36 | *unused* |
| 36 | Preset 37 | Corrections |
| 37 | Custom 1 | Custom 1 |
| 38 | Custom 2 | Custom 2 |
| 39 | Custom 3 | Custom 3 |
| 40 | Custom 4 | Custom 4 |
| 41 | Custom 5 | Custom 5 |
| 42 | Custom 6 | Custom 6 |
| 43 | Custom 7 | Custom 7 |
| 44 | Custom 8 | Custom 8 |
| 45 | Custom 9 | **Racing Officials** |
| 46 | Custom 10 | **Racing Teams** |

Two things fall out of that:

- Eight of the 37 preset slots are unused, leaving 29 named presets.
- Racing Officials and Racing Teams occupy custom slots 9 and 10, not
  presets, even though the menu lists them with fixed names alongside the real
  presets.

29 presets plus 10 custom slots gives 39 menu entries, which is exactly what the
radio's menu shows. The arithmetic working out is the check on the table.

---

# What the protocol does not have

Things looked for deliberately and not found:

- Any command touching display color. The radio stores 24-bit text and
  background colors for every screen area across seven layouts, all of it
  editable in the menus, and none of it is readable or writable over serial.
  The only color-carrying fields anywhere are the two LED colors, the global
  display mode, the DMR color code, and the vestigial backlight color described
  earlier.
- A menu id for Display Options, so that menu cannot even be opened by name.
  It has to be walked with key presses.
- Any documentation of the nine trailing status fields, decoded above.
- The push interval parameter, which the text says exists and never defines.

What the protocol does report is whether those colors are being drawn at all,
through the global display mode field, and the specification hides that behind a
label saying it applies to the waterfall only.

## The card is the only other route

The radio's settings, colors included, live on its microSD card. And the radio
does expose that card: the USB prompt it shows on connection offers mass storage
or serial port.

But it is one or the other. A program cannot hold the serial port and read
the card at the same time, so nothing can display live color it read that way
without having cached it from an earlier session.

Which makes "the application uses its own palette" much the most likely
explanation for any third-party software showing the scanner's colors.

---

# Commands the specification does not have

The previous section is about what the radio cannot do. This one is the mirror
image: things the radio does that the document never mentions.

*2026-08-05. The three letter name space has been swept end to end.
Seventy-two commands are known and thirty-six of them are undocumented, so
the published specification describes exactly half of what this radio answers.
The census below lists every one.*

## The radio says which commands exist

Send a command name the radio does not know and it answers with a bare `ERR`.
Send one it does know but cannot act on, and it answers with its own name
first:

| Sent | Reply | Reading |
| ---- | ----- | ------- |
| `MDL` | `MDL,SDS150` | Known, valid, answered |
| `HLD` | `HLD,ERR` | **Known.** Echoes its name, then complains about arguments |
| `ZZZ` | `ERR` | **Unknown.** No echo |
| `QQQ` | `ERR` | Unknown |

The echo is a membership test. Anything that names itself in a reply exists,
whatever else it thinks of the request.

That makes the command space enumerable. Every command here is exactly three
characters and one contains a digit, so the alphabet is 36 and the space is
46,656 names. Unknown commands answer rather than time out, so a sweep runs
at whatever the round trip costs rather than at whatever a timeout costs.

One caveat, unresolved. A bare `ERR` may conflate two different things:
genuinely unknown, and known but not valid from the current mode. Nothing so far
separates them. If they are conflated, a single sweep from one state will miss
whatever only exists in the menus or in the mode below, and covering that means
sweeping again from each state.

## `PRG` enters a program mode

```
->  PRG
<-  PRG,OK
```

Not in the specification anywhere. It came out of the first handful of
guesses, taken from the older BCD-series command set this protocol descends
from.

The radio's display becomes:

```
Remote Mode
Keypad Lock
```

Which is a real mode change, not a no-op. Three things about it:

It reports a mode that is not in the documented list. Scanner info comes back
with the mode reading `Remote Mode`, which appears nowhere among the fourteen
values the specification enumerates. So that list is incomplete, and anything
matching on it exhaustively will fall through.

The screen goes to ten large-font lines. The form field reads `1111111111`,
a shape none of the normal displays use.

Plenty still answers. Model, version, status and scanner info all replied
normally from inside it. It is not a mode that cuts the port off.

### Getting out of it is unsolved

`EPG`, which exits program mode on the older radios, is not a command here.
It returns a bare `ERR` both from normal operation and from inside program mode,
so it is genuinely absent rather than merely inapplicable.

Everything else tried also failed:

| Attempt | Reply |
| ------- | ----- |
| `EPG`, `EPM`, `XPG`, `EXT`, `END`, `QUT` | bare `ERR`, none exist |
| `PRG,0` | `PRG,ERR`, so it takes no argument |
| `JPM,SCN_MODE,<all ones>` | `JPM,ERR`, **refused**, though this is the documented way to start scanning |
| `KEY,M,P` | `KEY,OK` and nothing happened, as every key press claims |

That last pair is worth keeping: jump-to-mode is refused from program mode, and
the keypad lock means key presses are accepted and discarded. Both of the
obvious escapes are closed.

A power cycle is the only exit found. It is clean: the radio comes back
scanning, with nothing changed.

## `PWR` and `WIN` were already being used

Two more, and they were found before anybody went looking: the tool has been
calling them since signal strength was added.

```
->  PWR
<-  PWR,<level>,<frequency>

->  WIN
<-  WIN,<level>,<frequency>
```

Neither is in the specification. `PWR` reports signal strength as a number and
`-999` when there is nothing to measure, and `WIN` reports window voltage, which
tracks how far the signal sits from the center of the passband. They are the
only way the protocol gives signal strength as a number at all, since the
documented fields only offer a zero-to-four bar count.

So the count of confirmed undocumented commands is _three_: `PRG`, `PWR` and
`WIN`. All three are inherited from Uniden's earlier scanners. That is the whole
basis for the guesses below.

## What the guesses found

*Guesses written first, then tested, so the hit rate means something. 62 names
sent on 2026-08-05, blacklist refused in code, controls included.*

Four new commands: `GLG`, `PST`, `GS2` and `SGP`. That took the confirmed
total to seven at the time. Sweeping the whole space afterwards took it to
thirty-six, and found all four of these again except `GS2`, which no
letters-only sweep can reach.

| Tier | Sent | Found | |
| ---- | ---- | ----- | - |
| Most likely | 8 | 1 | `GLG` |
| Program mode | 19 | 1 | `SGP` |
| Lockout | 8 | 0 | |
| Symmetry | 4 | **2** | `PST`, `GS2` |
| Long shot | 20 | 0 | |

The theory behind the ranking was mostly wrong. The older radios' vocabulary
was supposed to be the rich seam, and 47 of its 55 names drew a bare `ERR`.
Guessing from the *shape of this protocol* scored 2 in 4 where guessing from
history scored 2 in 51. Worth remembering before the exhaustive sweep: names
that rhyme with what is already documented are the better bet.

The controls behaved: `MDL` echoed, `ZZZ` came back bare, and `EPG` came back
bare again, so the oracle held for the whole run.

### `GLG` reports what is being received

The prize. One round trip for everything about the current transmission:

```
->  GLG
<-  GLG,0155.5500,NFM,0,0,PUBLIC SAFETY,POLICE DEPARTMENT,DISPATCH,1,0,,,227
```

Twelve fields, matching the shape the older radios used: frequency, modulation,
attenuator, tone or code, then system, department and channel names, then
squelch and mute state, two tag fields, and a trailing number.

When nothing is being received every field is empty:

```
<-  GLG,,,,,,,,,,,,
```

This is what the scanner info XML is normally parsed for, in one short line
instead of a document. It takes no arguments; `GLG,0` is refused.

### `GS2` carries a caller-supplied tag

The one that changes something. It returns the same screen `GST` does, but the
first argument is echoed straight back:

```
->  GS2,HELLO
<-  GS2,HELLO,00001010100000000,...

->  GS2,req-42
<-  GS2,req-42,00001010100000000,...
```

Any string is accepted. `GST` and `STS` refuse an argument outright.

So the protocol does have a request identifier after all, for the screen.
That contradicts what is written further up this file, which was drawn from the
documented commands, where it remains true. Two programs sharing a port still
cannot tell whose `GST` reply is whose, but they can tell whose `GS2` reply is
whose. Anything multiplexing this port should use `GS2` and tag every request.

### `PST` is `GST` under another name

Byte for byte identical: the same 29 fields with the same content, differing
between two reads only by the clock and one line of live text.

The name follows the push commands, and it is not one. Nothing streamed after
sending it, checked by watching the port for five seconds without sending
anything.

### `SGP` exists and refuses everything

Every shape tried came back `SGP,ERR`: no arguments, one, two, three, four,
empty, and a word. It is a real command with an argument list nobody has
guessed. Unresolved.

## The command census

Sweeping the whole three letter space, one first letter at a time.

Membership only. A name earns a row by proving it exists: the radio echoes
the name back rather than answering a bare `ERR`. Nothing found here is probed
for arguments, so a description says what is known and no more. Names that do
not exist are not listed, which is what keeps the table a census rather than a
transcript.

Progress: _complete._ Every three letter name, `AAA` to `ZZZ`, has been sent.

### A

*2026-08-05. 676 names, 1 minute 34 seconds. Six exist.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `ALV` | false | Accepts a bare call and answers `OK`, so it acts rather than reports. Nothing visible changed: the layout held and the radio stayed in Scan Mode. Purpose unknown |
| `APR` | true | Pause or resume a running analysis mode |
| `AST` | true | Start an analysis mode, which must be entered from Scan Mode so the database is loaded |
| `ATL` | false | Refuses a bare call, so it takes arguments. Purpose unknown |
| `AUF` | false | Refuses a bare call, replying `AUF⇥ERR⇥471` with tab separators and a number matching no documented error code. Purpose unknown |
| `AVD` | true | Avoid, temporarily avoid, or stop avoiding a system, department, channel or frequency |

Three of the six are undocumented. Two of those three behave unlike anything
else in this protocol.

`ALV` is the only name in 676 that accepted a bare call and reported success.
Every other find refused for want of arguments. Something happened; what, is
unknown, and finding out means sending it deliberately rather than in passing.

`AUF` answered with tab separators. The whole protocol is comma separated,
with tab reserved for escaping a comma inside a value, so a tab delimited reply
carrying a bare number matches nothing else here.

The run was repeated end to end and returned the same six names with the same
replies.

### B

*2026-08-05. 674 names, 1 minute 33 seconds. None exist.*

Two names were refused by the blacklist rather than sent, `BLD` and `BOT`, both
guessed at as bootloader entry. Nothing else in the block exists, which is weak
evidence that those two do not either, but they stay unsent.

An empty block is worth recording. It says the radio's vocabulary is not spread
evenly across the alphabet, and it is the first evidence of how lumpy the
distribution is: 676 names for six commands under `A`, and 674 for none under
`B`.

### C

*2026-08-05. 675 names, 1 minute 34 seconds. One exists.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `CFC` | false | Accepts a bare call and answers `OK`, so it acts rather than reports. Nothing visible changed: the layout held and the radio stayed in Scan Mode. Purpose unknown |

One name was refused by the blacklist rather than sent: `CLR`, which cleared all
memory on the older radios.

`CFC` is the second command found that answers `OK` to a bare call, after
`ALV`. That is now two of the four undocumented finds from sweeping, against
every documented command in the same blocks refusing for want of arguments.
Whatever these two are, they take no parameters and they do something, and the
pair of them is more interesting than either alone.

### D

*2026-08-05. 674 names, 1 minute 33 seconds. Four exist.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `DIR` | false | Answers `DIR,NG` rather than `ERR`, so it exists and is refusing the request rather than the arguments. Purpose unknown |
| `DLC` | false | Answers `DLC,0` to a bare call: a working read returning a single value, which was `0` while scanning. Purpose unknown |
| `DQK` | true | Read or set the enabled state of a department's quick keys |
| `DTM` | true | Read or set the clock. A bare call returned `DTM,0,2026,8,5,18,21,26,1`, matching the documented order of daylight saving, date, time and real-time clock health |

Two names were refused by the blacklist rather than sent: `DCH`, which deleted a
channel on the older radios, and `DEF`, guessed at as restoring defaults.

`DLC` is the first name found by sweeping that answers with data. Everything
else found so far either refused or reported `OK`. A bare call returns one
field, `0`, with no arguments needed, which makes it a read of something. What,
is unknown.

`DIR` refuses with `NG` rather than `ERR`, and it is the first command seen
in the wild using that form. The specification describes both, saying failures
come back either way depending on the command, but every other refusal
encountered across four blocks has been `ERR`. On the older radios `NG` meant
the command was understood but not valid right now, which would suggest `DIR`
is waiting for the radio to be in some other state.

### E

*2026-08-05. 675 names, 1 minute 34 seconds. Two exist, both undocumented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `EFM` | false | Refuses a bare call with `ERR`, so it takes arguments. Purpose unknown |
| `ESN` | false | Returns the radio's serial number, as `ESN,<14 digits>,<3 characters>,1`. Needs no arguments |

One name was refused by the blacklist rather than sent: `EWP`, guessed at as an
erase.

`ESN` is the serial number, and it checks out against the card. The identity
file on the memory card records the serial as three hyphenated groups; `ESN`
returns the first two joined, then the third, then a trailing `1`. Character for
character the same value.

That is worth having. The protocol otherwise offers no way to identify which
radio you are talking to: `MDL` gives only the model, so two SDS150s on one
machine are indistinguishable over the wire. Everything that currently tells
them apart, including this tool's own device list, reads the serial from the USB
descriptor rather than from the radio. `ESN` asks the radio itself.

The actual value is left out of this file deliberately. It identifies one
physical unit, and a serial number is the sort of thing worth not committing to
a repository.

### F

*2026-08-05. 673 names, 1 minute 33 seconds. Four exist.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `FPR` | false | Answers `FPR,NG`, so it exists and is refusing the request rather than the arguments. Purpose unknown |
| `FQK` | true | Read or set the enabled state of the hundred favorites list quick keys. A bare call returns all hundred |
| `FRX` | false | Answers `FRX,NG`. Purpose unknown |
| `FTX` | false | Answers `FTX,NG`. Purpose unknown |

Three names were refused by the blacklist rather than sent: `FMT`, `FWD` and
`FWU`, guessed at as formatting the card and as firmware operations.

The three new ones all refuse with `NG` rather than `ERR`, which is the
first cluster of that behavior. Counting `DIR` from the D block, four commands
now answer `NG` and every other refusal across six blocks has been `ERR`.

If `NG` carries the meaning it had on the older radios, understood but not valid
right now, then these four are waiting for the radio to be in a state it is not
currently in. Program mode is the obvious candidate, since `PRG` is
confirmed and gated exactly that sort of command on those radios. That is a
hypothesis and nothing more: none of the four has been sent from inside program
mode, because program mode has no known exit but the power button.

`FRX` and `FTX` are hard to read as anything but receive and transmit, which
would be odd on a radio that does not transmit. A service or alignment function
is the likeliest reading, and it is a guess.

### G

*2026-08-05. 676 names, 1 minute 34 seconds. Ten exist, the busiest block so
far.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `GCS` | true | Charge status: state, voltage, percentage, current and temperature. Returned a state and a field count the specification does not cover, corrected above |
| `GFM` | false | Refuses a bare call with `ERR`, so it takes arguments. Purpose unknown |
| `GGP` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `GLG` | false | Reports the current transmission in one line: frequency, modulation, and the system, department and channel names. Every field is empty when nothing is being received |
| `GLT` | true | Read one of the scanner's lists, such as favorites, systems, departments or channels |
| `GRR` | false | Accepts a bare call and answers `OK`, so it acts rather than reports. Nothing visible changed. Purpose unknown |
| `GSI` | true | Everything the radio is doing, as an XML document |
| `GSP` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `GST` | true | The screen, plus both LED colors and a display mode field |
| `GWF` | true | Waterfall FFT, 240 points |

Nothing in this block was blacklisted.

`GRR` is the third command that answers `OK` to a bare call, after `ALV` and
`CFC`. All three are undocumented, all three take no arguments, and all three do
something that leaves no visible trace.

### The sweep cannot find `GS2`

Worth stating plainly, because this block is where it would have shown up.

`GS2` is confirmed to exist and did not appear here. The sweep sends letters
only, and `GS2` ends in a digit. So does the documented `GW2`, which the
census will never list either.

The real space is 36 characters per position, not 26: 46,656 names rather than
17,576. Sweeping letters alone covers 38 percent of it and will miss every
command with a digit in it, which is a category the firmware demonstrably uses
for variants of existing commands.

### H

*2026-08-05. 676 names, 1 minute 34 seconds. One exists, and it is documented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `HLD` | true | Hold on a system, department or channel. Cannot hold a favorites list or a site frequency |

Nothing in this block was blacklisted, and nothing new was found. The first
block since `B` to add nothing to the count.

### I

*2026-08-05. 675 names, 1 minute 34 seconds. None exist.*

One name was refused by the blacklist rather than sent: `INI`, guessed at as an
initialize.

The second empty block, after `B`.

### J

*2026-08-05. 676 names, 1 minute 34 seconds. Two exist, both documented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `JNT` | true | Jump to a number tag, given list, system and channel tags |
| `JPM` | true | Jump to a mode. Sending a channel index of all ones starts scanning from the top channel |

### K

*2026-08-05. 676 names, 1 minute 35 seconds. Two exist, both documented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `KAL` | true | Keep alive. **Answers nothing at all**, which the sweep recorded as silence rather than as a refusal, matching the specification |
| `KEY` | true | Press a key. Answers `OK` for every code, including codes this model has no key for |

`KAL` is the only name in nine and a half thousand to answer with silence. That
is worth noting as a positive result: the sweep's classifier distinguishes a
command that exists and says nothing from one that does not exist, and this is
the case that proves it.

### L

*2026-08-05. 676 names, 1 minute 35 seconds. One exists, and it is documented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `LCR` | true | Read or set the location and range the radio scans around. A bare call returns latitude, longitude and range |

The reply carries a real position, so the value is left out of this file for the
same reason the serial number is.

### M

*2026-08-05. 676 names across three runs, and the most eventful block by far.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `MDL` | true | The model name, returned as `SDS150GBT` on this radio |
| `MNU` | true | Open a menu by name. **A bare call opens the menu tree**, which is what stopped the sweep |
| `MSB` | true | Go back a level in the menus, or leave them |
| `MSI` | true | Read the menu currently on screen, as XML |
| `MSM` | false | **Switches the radio into USB mass storage mode.** Answers `OK`, then the serial port disappears and the memory card mounts |
| `MSN` | false | Answers nothing. Sent immediately after `MSM`, so its silence may only mean the radio was already leaving |
| `MSV` | true | Write a value into the menu item the radio is on. Refused inside a color picker |

Two commands ended runs in this block, which is why it took three:

1. `MNU` with no arguments opened the menu tree. The guard caught it 25
   names later at `MOK`, so the block ran `MAA` to `MOK` and stopped. The radio
   was returned to scanning and the sweep resumed at `MOL`.
2. `MSM` put the radio into mass storage mode, killing the serial port
   outright. The run died at `MSO` with the port no longer configured.

After the radio was restarted into serial mode, `MSO` to `MZZ` ran clean and
added `MSV`, which is documented. The block is now complete.

### `MSM` is the find of the sweep

The memory card holds everything the protocol cannot reach: the favorites lists,
the settings, and all 315 display colors. Getting at it has meant restarting the
radio and pressing `E` at a prompt that waits 15 seconds and then gives up.

`MSM` does it over the wire. Send it and the card mounts.

Confirmed as far as it can be without a serial port: after the run died, the
volume was mounted and holding `BCDx36HP`, and `/dev/cu.*` was gone.

`MSM` alone was confirmed later the same day, sent by itself with `MSN`
nowhere near it. It is the whole mechanism.

Nothing undoes it. Mass storage removes the serial port, so there is no channel
left to ask, and the way back is the power button.

This changes what `radiocli backup` can be. The command currently opens with
instructions to restart the radio and catch a prompt. It could instead send
`MSM`, wait for the volume to appear, and copy. The awkward half of the command
would disappear. It would still have to say restart the radio afterwards, since
nothing brings the serial port back.

### N

*2026-08-05. 676 names, 1 minute 34 seconds. One exists, and it is documented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `NXT` | true | Step forward to the next system, department, channel or frequency |

This block was swept twice. The first run went out immediately after `MNU`
had opened the menu tree, so all 676 names reached a radio sitting in a menu
rather than scanning, and the results were discarded rather than trusted. A
command that refuses in one state can answer in another, which is the whole
reason the `NG` family is interesting.

The second run, from Scan Mode, returned the same single name. So nothing was
actually lost this time, which is luck rather than vindication: the discarded
run could not have been known to agree without redoing it.

### O

*2026-08-05. 676 names, 1 minute 34 seconds. None exist.*

The third empty block, after `B` and `I`.

### P

*2026-08-05. 672 names, 1 minute 33 seconds. Three found, four skipped.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `POF` | true | Power the radio off. **Skipped, not sent** |
| `PRG` | false | Enter program mode, which has no known exit. **Skipped, not sent** |
| `PRV` | true | Step back to the previous system, department, channel or frequency |
| `PSI` | true | Push scanner info periodically. **Skipped, not sent** |
| `PST` | false | The screen, identical to `GST` field for field |
| `PWF` | true | Push waterfall FFT. **Skipped, not sent** |
| `PWR` | false | Signal strength and the frequency it was measured on. Returns `-999` when there is nothing to receive |

Four names were skipped rather than sent, and all four are known to exist, so
nothing is lost by not proving it. `POF` and `PRG` are on the blacklist. `PSI`
and `PWF` start a stream that never stops, which every later probe in the
block would have read as its own reply.

This block was swept twice. The first attempt died at `PMH` when the cable was
pulled out by accident. The 319 names it had covered were discarded rather than
resumed from, because probes taken in the seconds around a disconnect can return
anything.

### Q

*2026-08-05. 675 names, 1 minute 34 seconds. One found, one skipped.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `QKS` | false | Refuses a bare call with `ERR`, so it takes arguments. Purpose unknown |
| `QSH` | true | Go to quick search hold on a frequency. **Skipped, not sent**, because it changes the radio's mode |

### R

*2026-08-05. 675 names, 1 minute 33 seconds. Three exist, all undocumented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `RBS` | false | Refuses a bare call with `ERR`, so it takes arguments. Purpose unknown |
| `RGP` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `RMT` | false | Refuses with `RMT⇥ERR⇥494`, tab separated, carrying a number that is not a documented error code. Purpose unknown |

One name was refused by the blacklist rather than sent: `RST`, guessed at as a
reset.

`RMT` is the second command to answer with tab separators, after `AUF` in
the A block. Both refuse, both use tabs where the whole protocol uses commas,
and both carry a bare three digit number: `471` for `AUF` and `494` for `RMT`.
Two of a kind is a family rather than a curiosity, and the numbers are close
enough to look like they index into the same table.

### S

*2026-08-05. 676 names, 1 minute 35 seconds. Ten exist, matching `G` as the
busiest block.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `SGP` | false | Refuses every argument shape tried. Purpose unknown |
| `SLK` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `SNF` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `SQK` | true | Read or set a system's quick keys |
| `SQL` | true | Read or set the squelch level, 0 to 15 on this model |
| `SQV` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `SSC` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `STS` | true | The screen, with nine trailing fields the specification calls reserved |
| `SUS` | false | **Answers nothing at all**, like `KAL`. Purpose unknown |
| `SVC` | true | Read or set the 47 service type flags |

Nothing in this block was blacklisted.

`SUS` is the second command in the whole sweep to answer with silence, after
the documented `KAL`. Since `KAL` is a keep alive, whose entire job is to make
noise on the wire without expecting an answer, a second silent command is worth
noticing. It is undocumented and its purpose is unknown.

`SVC` returned its 47 flags, enabled at positions 0, 1, 3, 8, 24, 28 and 37,
which is the same reading recorded as unresolved at the end of this file and
still disagrees with what the radio's own menu displays.

### T

*2026-08-05. 676 names, 1 minute 34 seconds. One exists, undocumented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `TST` | false | Refuses a bare call with `ERR`, so it takes arguments. The name suggests a test or self test, and that is a guess from three letters |

### U

*2026-08-05. 675 names, 1 minute 36 seconds. Three exist.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `UDP` | false | **Answers nothing at all**, like `KAL` and `SUS`. Purpose unknown |
| `UHC` | false | Refuses a bare call with `ERR`. Purpose unknown |
| `URC` | true | Start or stop recording. A bare call returned `0`, meaning stopped |

One name was refused by the blacklist rather than sent: `UPD`, guessed at as a
firmware update.

`UDP` is the third silent command, after `KAL` and `SUS`.

### V

*2026-08-05. 676 names, 1 minute 34 seconds. Two exist, both documented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `VER` | true | Firmware version, returned as `Version 1.00.37` |
| `VOL` | true | Read or set the volume, 0 to 15 on this model |

### W

*2026-08-05. 676 names, 1 minute 34 seconds. One exists, undocumented.*

| Command | Documented | What it does |
| ------- | ---------- | ------------ |
| `WIN` | false | Window voltage and the frequency it was measured on, tracking how far the signal sits from the center of the passband |

### X, Y and Z

*2026-08-05. 2,028 names, 4 minutes 42 seconds. None exist.*

Three empty blocks in a row, which makes six empty blocks in all with `B`, `I`
and `O`.

### The result

Every three letter name has now been sent. Seventy commands exist.

| | Commands |
| - | -------- |
| Found by the sweep | **70** |
| Documented | 35 |
| Undocumented | 35 |
| Names that cannot be reached by a letters-only sweep | `GW2` documented, `GS2` not |
| **Known in total** | **72: 36 documented, 36 undocumented** |

The specification describes exactly half of what this radio answers.

### The sweep found every documented command

Worth stating, because it is the only available check on whether the method
misses things. Thirty-five of the thirty-six documented commands are spellable
in letters alone, and the sweep found all thirty-five. No false negatives
against a known set of that size is reasonable evidence that a command answering
in Scan Mode did not slip past.

It says nothing about commands that answer only from some other state. Four
names refuse with `NG` rather than `ERR`, which on the older radios meant
understood but not valid right now, so there may be a second population that a
Scan Mode sweep can only ever see refusing.

### Three commands answer with silence

`KAL`, `SUS` and `UDP`. Three names out of 17,576, against roughly 17,500 that
answered a bare `ERR` and 70 that answered properly.

Silence is a distinct, deliberate behavior, not a failure to reply. It is
rare enough at three in seventeen thousand to be a design choice, and one of the
three is documented: `KAL` is the keep alive, whose whole job is to put traffic
on the wire without expecting an answer.

`SUS` and `UDP` are undocumented and behave identically to it:

```
->  UDP        <- nothing
->  UDP,0      <- nothing
->  UDP,1      <- nothing
->  UDP,ON     <- nothing
->  UDP,OFF    <- nothing
```

`SUS` does the same. Arguments change nothing, which is the same for `KAL`,
so all three ignore what follows the name.

Nothing observable happened. Nothing streamed on the serial port afterwards,
checked by watching it for five seconds without sending anything, and the
scanner info document was unchanged.

`UDP` is a suggestive name and the evidence does not support the obvious
reading. A UDP listener would need a network, and:

- The scanner info `Property` element on this radio has no `WiFi` attribute at
  all, though the specification documents one with values `Off`, `0` to `3`
  and `AP`. The field is simply absent here.
- The memory card holds no network configuration of any kind. There is no
  key mentioning WiFi, an address, a port or a socket anywhere in its
  configuration.

The likelier reading is that these are commands for a different model. This
firmware is shared across the SDS100, SDS200 and SDS150, and the protocol is
full of things that exist for hardware this radio does not have: `BK_COLOR` is a
backlight color from a monochrome ancestor, `LED2` is documented as SDS150 only,
and `KEY,R` and `KEY,T` are accepted for keys this model does not physically
have. A command accepted and silently ignored is an established pattern here,
not a new one.

That is a hypothesis. What would settle it is running the same three names
against an SDS200, which has the networking this one appears to lack.

### The skipped commands, tested deliberately

*2026-08-05. Everything the sweep refused to send, sent on purpose, ordered
least destructive first and `MSM` last.*

None of the twelve destructive guesses exist. Every one answered a bare
`ERR`:

`CLR`, `DCH`, `EWP`, `FMT`, `RST`, `INI`, `DEF`, `UPD`, `BLD`, `BOT`, `FWU`,
`FWD`.

The favorites lists were counted after each of the four worst and stayed at
four throughout. So the blacklist protected nothing, because there was nothing
to protect against. That is the same result the rest of the guessing produced:
names taken from the older radios' vocabulary are mostly not in this firmware.
It was still the right call to withhold them until the value of sending them
was worth the risk, since the cost of being wrong was somebody's configuration.

The rest did exist, and three of them are worth more than the sweep suggested.

| Command | Bare call | What it turned out to be |
| ------- | --------- | ------------------------ |
| `MNU` | `MNU,OK` | Opens the menu tree. Recovered with one key press |
| `QSH` | `QSH,NG` | Refuses without a frequency |
| `PWF` | `PWF,ERR` | Wants arguments |
| `PSI` | `PSI,0` | **Reads the push interval**, and `0` means off |
| `MSM` | `MSM,OK` | **Switches to mass storage, on its own** |

### `PSI` takes an interval, and `0` turns it off

The specification says the push interval can be changed by a parameter and never
says what the parameter is. It is the first argument, and a bare call reads it
back:

```
->  PSI        <- PSI,0        the interval, off
->  PSI,1      <- PSI,OK       then the documents start arriving
->  PSI,0      <- PSI,OK       and stop
```

At an interval of `1` the radio pushed 296,758 bytes in five seconds, around
60 KB per second of scanner info documents. Turning it off was verified by
watching the port afterwards and seeing nothing.

This is the piece that makes live state practical. Everything reading this
radio has been polling it, which forces a choice between stale data and wasted
round trips. The push side was documented but unusable without knowing the
parameter.

`PWF` accepts `PWF,<type>,<on/off>` and answers `OK` both ways, but streamed
nothing with the waterfall closed. It presumably needs that display open.

### `MSM` alone is confirmed

Sent by itself, with `MSN` nowhere near it:

```
->  MSM
<-  MSM,OK
```

Six seconds later the serial port was gone and the card was mounted. `MSM` is
the whole mechanism, and `MSN` had nothing to do with it.

### Nothing was destroyed, and that is checked rather than assumed

Because `MSM` was left until last, the card could be mounted immediately
afterwards and compared against the verified backup taken earlier the same day.

One file differed: `app_data.cfg`, in the two lines recording the department
and channel the radio was last stopped on. Those change every time it receives
anything. `profile.cfg` with all 315 colors, every favorites list and the whole
database were byte for byte identical.

So twelve commands guessed to be destructive were sent, and the radio's
configuration came through untouched, proven against a copy rather than assumed
from the absence of visible damage.

### How the commands are spread

| Block | Found | | Block | Found | | Block | Found |
| ----- | ----- | - | ----- | ----- | - | ----- | ----- |
| A | 6 | | J | 2 | | S | 10 |
| B | 0 | | K | 2 | | T | 1 |
| C | 1 | | L | 1 | | U | 3 |
| D | 4 | | M | 7 | | V | 2 |
| E | 2 | | N | 1 | | W | 1 |
| F | 4 | | O | 0 | | X | 0 |
| G | 10 | | P | 7 | | Y | 0 |
| H | 1 | | Q | 2 | | Z | 0 |
| I | 0 | | R | 3 | | | |

Nothing at all under `B`, `I`, `O`, `X`, `Y` or `Z`. Ten each under `G` and `S`,
seven each under `M` and `P`.

That is not a random scatter. The firmware names commands after what they do:
`G` for get, `S` for set and status, `M` for menu and model, `P` for push and
power. The letters with nothing behind them are the ones no verb starts with.

### What it cost

17,576 names at about 139 ms each: roughly 41 minutes of wire time, spread
over a few hours of runs, recoveries and one accidental unplug.

Three runs ended early and had to be repaired. `MNU` opened the menu tree, `MSM`
took the serial port away, and the cable came out of the back of the radio once.
Each cost a restart and a re-run of the affected block. The `N` block was swept
twice because the first pass ran while the radio sat in a menu, and its results
were discarded rather than trusted.

This covers letters only. The real name space is 36 characters per position,
so 46,656 rather than 17,576, and every name containing a digit is still
unswept. `GS2` and `GW2` both prove the firmware uses that space.

### What the sweep needed to work

The guard that watches for a probe changing the radio's mode cannot compare
the text of the display. A scanning radio rewrites it constantly, and the
first attempt stopped after 175 names because a status icon appeared and
disappeared. Comparing the layout shape instead is stable across normal scanning
and still changes the moment the radio enters a menu or another mode.

## The guesses, as written beforehand

These come from the command sets Uniden published for the BC and BCD series
radios this protocol descends from. The reasoning is narrow but well
supported: three of three confirmed finds are inherited, the card still calls
itself `BCDx36HP`, and the misspelling in the menu-back argument is the older
document's. If the firmware kept a command it no longer documents, the older
vocabulary is where it came from.

Ranked by how much would be believed if it turned up.

### Most likely

| Guess | What it should do | Why |
| ----- | ----------------- | --- |
| `GLG` | Report what is being received now: frequency, talkgroup, modulation, mute and squelch state, in one reply | The single most used command on every earlier Uniden. If one survived, this one did |
| `BAV` | Battery voltage as a number | Sits beside `PWR` and `WIN` in the older set, and this radio already reports voltage elsewhere |
| `GID` | The talkgroup being received | Same family as `GLG` |
| `BLT` | Backlight setting | The radio has the setting and no documented command for it |
| `WXS` | Weather alert settings | Documented on the older radios, and this model has the feature |
| `CLC` | Close call settings | As above |
| `SCO` | Search and close call options | As above |
| `PRI` | Priority mode | As above |

### Program mode

`PRG` is confirmed, and on the older radios it was the gate for a whole second
command set that reads and writes memory directly. If any of these exist, they
are the largest undocumented surface here, because they would reach
configuration the menus expose slowly and the protocol does not expose at all.

| Guess | What it should do |
| ----- | ----------------- |
| `SIN` | Read or write system information by index |
| `CIN` | Read or write channel information |
| `GIN` | Read or write group or department information |
| `TIN` | Read or write trunked system information |
| `SIF`, `SIH`, `SIS` | Site and system detail |
| `MEM` | How much memory is used |
| `SGP` | System group |
| `CSG`, `CSP` | Custom search group and program |
| `SSP` | Service search program |
| `TON` | Tone out setup |
| `AGC`, `AGV`, `AGT` | Automatic gain settings |
| `P25` | P25 threshold settings |
| `CNT` | Display contrast, a setting the SDS may no longer have |
| `BSV` | Battery save |

Worth noting that `EPG` is already ruled out. It exited program mode on
every older radio and returns a bare `ERR` here, from inside program mode and
outside it, which is why getting out is still unsolved.

### Avoid and lockout

The older radios had a separate vocabulary for this, and the documented `AVD`
covers only part of what the menus can do.

| Guess | What it should do |
| ----- | ----------------- |
| `GLF` | Get the next locked-out frequency |
| `ULF` | Unlock a frequency |
| `LOF` | Lock out a frequency |
| `GLI` | Get the next locked-out talkgroup |
| `ULI` | Unlock a talkgroup |
| `LIH`, `LIN` | Lock out a talkgroup |
| `QGL` | Quick group lockout |

### Symmetry with what is documented

The protocol pairs a polled command with a pushed one twice: scanner info and
the waterfall. It also has a separator-free binary form of the waterfall. If
either pattern was applied more widely, the names are predictable.

| Guess | What it should do |
| ----- | ----------------- |
| `PST` | Push the screen, as `PSI` pushes scanner info |
| `GS2` | The screen without separators, as `GW2` is the waterfall without separators |
| `PCS` | Push charge status |
| `PWR` on a timer, or `PPW` | Push signal strength, which is what a live meter wants |

### Long shots

`COM`, `SUM`, `TRN`, `REV`, `RMB`, `SHK`, `TFQ`, `UNT`, `DGR`, `DLA`, `OMS`,
`PDI`, `RIE`, `ACC`, `ACT`, `CBP`, `ABP`, `GDO`, `DBC`, `EWP`, `FWD`, `SCT`.

These appeared in one or another of the older command sets. Several are
obscure enough that nothing is known about what they did, which is a reason to
probe them rather than a reason not to.

### The ones to never send blind

These are guesses about names, not knowledge, and if any exists it destroys
something. They belong on the blacklist before a sweep starts, not in it.

| Guess | What it might do |
| ----- | ---------------- |
| `CLR` | Clear all memory. Documented on the older radios, and it did exactly that |
| `DCH` | Delete a channel |
| `EWP` | Erase |
| `FMT` | Format the card |
| `RST`, `INI`, `DEF` | Reset, initialize, restore defaults |
| `UPD`, `BLD`, `BOT`, `FWU` | Firmware or bootloader entry |

`POF` already proves a three-letter command with no arguments can do something
drastic, and `PRG` proves an undocumented one can change the radio's mode. There
is no reason to assume the destructive end of the older vocabulary was dropped
while the useful end survived.

## Hazards, learned the hard way

`POF` needs no arguments. Three letters, nothing else, and the radio powers
off and stays off until somebody presses the button. It is proof that the
dangerous end of this space is reachable by exactly the probes a sweep sends.
`PRG` is the same shape. Assume there are others: a factory reset, a firmware or
bootloader entry, a write to the card.

A line-based reader desynchronizes on multi-line replies. Scanner info
returns an XML document across many lines. A probe that reads one line per
command will hand the second line to the next command, and the next, silently
attributing every subsequent reply to the wrong probe. That is how `POF` came to
be sent blind here. Drain until the port goes quiet, not until the first line.

## If this gets swept properly

- Back up the card first. The settings, the favorites and the display
  customization all live there.
- Probe with no arguments. Most destructive operations need parameters, and
  `PRG` and `POF` show that the ones that do not are exactly what a sweep finds.
- Blacklist `POF` and anything else known to act.
- Checkpoint to disk, so a power cycle resumes rather than restarts.
- Re-read the screen every few hundred probes, or a mode change like `PRG`
  goes unnoticed and the next thousand replies get read against the wrong state.
- Sweep the inherited BCD vocabulary first. Roughly a hundred names, and
  `PRG` is evidence the inheritance is still live. Far better odds per probe than
  brute force.

What no sweep can find is parameters. Knowing a command exists says nothing
about what it takes. Arity can be probed by appending commas and watching the
error change, but the argument space itself has no bound.

---

# Things worth acting on

| Finding | Why it matters |
| ------- | -------------- |
| Scanner info can be pushed | State can be **streamed rather than polled**, which changes the design of anything live and removes the jitter polling forces |
| The FFT can be pushed | The same, for spectrum |
| Keep alive returns nothing | Necessary for push mode, and it will hang any transport that waits for a reply |
| `GST` carries both LEDs and a mode field | Strictly better than `STS`, one round trip, already implemented but not surfaced in the CLI |
| `AVD` takes a system index | The missing piece for avoiding an individual system |
| `MNU` opens menus by id | Named navigation instead of counting key presses |
| A changed database counter invalidates every index | The documented way to notice a stale index instead of acting on it |
| Service types are transmitted out of menu order | Any work on them needs the table above |
| A reply that echoes its command name proves the command exists | Makes the whole 46,656-name command space enumerable, and seven commands are already found this way |
| `GLG` reports the current transmission in one line | Frequency, modulation, and the system, department and channel names, without parsing the scanner info document |
| `GS2` echoes a caller-supplied tag | The only way to tell whose reply is whose, which is what anything sharing the port needs |
| `MSM` switches to mass storage over the wire | Turns reaching the memory card from a restart and a timed button press into one command. See the backup command |
| `PSI,<interval>` pushes scanner info, `PSI,0` stops it | The undocumented parameter. Makes live state a stream rather than a poll |
| `PRG` exists and is undocumented | Proof there is undocumented surface, and a mode with no known exit but the power button |

## Unresolved

The Select Service Types menu shows a second column reading `---` for nearly
every row and `On` for Custom 1. A live read of the service type command on the
same radio returned enabled at positions 0, 1, 3, 8, 24, 28 and 37, which by the
table above is Multi-Dispatch, Law Dispatch, EMS Dispatch, EMS-Tac, EMS-Talk,
Emergency Ops and Custom 1.

Only Custom 1 agrees between the two. So either that menu column is not the
enable flag, or it means something else entirely. Toggling a single row and
re-reading would settle it, and until somebody does, nothing should trust either
source over the other.
