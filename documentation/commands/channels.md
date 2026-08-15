# channels

Lists the channels inside one department, with the frequency of each. Run it to
see what a department actually receives.

## Overview

A channel is the bottom of the scanner's memory and the thing everything else
exists to organise: a frequency, or a talkgroup on a trunked system, with a name
attached. `channels` reports them for one department at a time.

What a channel carries depends on the system above it. On a conventional system
it is a frequency. On a trunked system it is a **talkgroup**, because a trunked
system shares a pool of frequencies across everybody on it and hands one out per
transmission, so a frequency identifies nobody. See [`sites`](sites.md) for what
that means and where the frequencies live.

This asks the scanner directly and takes one exchange. It does not stop the
scanner scanning. It needs a scanner, so name one with `--device`.

A department's contents are two separate requests, `CFREQ` for frequencies and
`TGID` for talkgroups, and this asks for both. No department holds a mix: which
one answers is decided by the system above it.

## Usage

```
radiocli channels <department> [flags]
radiocli channels new <department> <frequency|TGID:id> <name> [flags]
radiocli channels rename <department> <name> <new-name> [flags]
radiocli channels delete <department> <name> --yes [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<department>` | Yes | none | The department to look inside, by index or by name. |
| `--names` | No | `false` | List the names only, without reading the frequencies. |

### `<department>`

Which department's channels to list. Get the name or index from
[`departments`](departments.md). A name is matched without regard to case, and
costs the lookup described there, because a department's index says nothing
about which system or favorites list holds it.

### `--names`

Reports the names alone, without the column saying what each channel receives.
This used to be the quick form, back when each frequency meant opening a channel
and climbing back out. Both are now one exchange, so this is a narrower table
rather than a faster command.

## Examples

Listing a conventional department's channels:

```
$ radiocli channels "FIRE RESCUE"
NAME          RECEIVES
DISPATCH      153.980000MHz
FIREGROUND 2  154.415000MHz
FIREGROUND 3  155.235000MHz
```

A trunked department holds talkgroups instead:

```
$ radiocli channels "GREENDALE FIRE"
NAME           RECEIVES
FIRE DISPATCH  TGID:9051
CHANNEL 2      TGID:9053
CHANNEL 3      TGID:9055
```

Names only:

```
$ radiocli channels "FIRE RESCUE" --names
NAME
DISPATCH
FIREGROUND 2
FIREGROUND 3
```

As JSON, where the two are separate fields rather than one:

```
$ radiocli channels "POLICE DEPARTMENT" -o json
[
  {
    "name": "DISPATCH",
    "frequency": "155.550000MHz"
  }
]
```

## Output

The table goes to stdout. The empty-department message goes to stderr, as do
debug logs from `--verbose`.

Under `--output text`, stdout holds a header row and one row per channel:

```
NAME          FREQUENCY
DISPATCH      153.980000
```

| Column | Description |
| ------ | ----------- |
| `NAME` | The channel's name, exactly as the scanner holds it. |
| `RECEIVES` | The frequency in megahertz, or the talkgroup written `TGID:9051`, as the scanner writes either. `-` when the channel has neither. |

Under `--output json`, stdout holds an array of objects, in the order the
scanner lists them:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The channel's name. |
| `frequency` | string | The frequency as the scanner writes it. Absent on a trunked channel. |
| `talkgroup` | string | The talkgroup as the scanner writes it. Absent on a conventional channel. |

**The table puts them in one column and the JSON keeps them apart** on purpose.
A person reading a department wants one answer to "what does this receive", and
no department mixes the two, so a single column is never ambiguous. Anything
parsing the output wants to know which of the two it got without matching on a
prefix.

Both are strings rather than numbers, because that is how the scanner holds
them, and because a talkgroup is not a quantity: it is an identifier that
happens to be written in digits.

A department holding no channels prints nothing to stdout, says so on stderr,
and exits `0`.

## `channels new`

`new` creates a channel in a department, with a name:

```
$ radiocli channels new "POLICE DEPARTMENT" 155.550 "GREEN BANK DISPATCH"
GREEN BANK DISPATCH

$ radiocli channels new "GREENDALE FIRE" TGID:9051 "FIRE DISPATCH"
FIRE DISPATCH
```

**A frequency and a talkgroup are one argument, not two.** They are the same
thing in the same slot: the address of what the channel receives. A conventional
department takes megahertz; a trunked department takes a talkgroup, written with
the `TGID:` prefix the scanner itself uses when it reports one. The prefix is
matched without regard to case.

**Giving the wrong kind is refused before anything is created.** The scanner
decides which entry screen `New Channel` opens, and this checks which one it got
before typing into it:

```
$ radiocli channels new "GREENDALE FIRE" 153.980 "WRONG"
error: "GREENDALE FIRE" is in a trunked system and takes a talkgroup, not a frequency: write it with the prefix the scanner uses, as TGID:9051
Nothing has been created
```

That check matters more than it looks. A talkgroup typed into a frequency screen
would be created as a channel on 9051 MHz, which the scanner accepts without
complaint and which receives nothing.

**The address is read before the scanner is touched.** A frequency may carry a
unit in any spelling, exactly as [`tune`](tune.md) takes one, and is converted to
digits before it is typed, because the entry screen has keys for digits and a
decimal point alone. Anything that is not a frequency at all is refused where it
costs nothing:

```
$ radiocli channels new "POLICE DEPARTMENT" wideband "NO"
error: "wideband" is not a frequency: write it in megahertz, as 155.475, or write a talkgroup as TGID:9051
```

Leaving the address out is refused in the terms of what was left out. A blank
address is a missing frequency, and a bare `TGID:` is a missing talkgroup:

```
$ radiocli channels new "POLICE DEPARTMENT" "" "NO"
error: no frequency was given: write it in megahertz, as 155.475, or write a talkgroup as TGID:9051
```

**It comes before the name because that is the order the scanner asks in.**
Pressing `New Channel` opens the entry screen before any channel exists; the
channel is created by what is typed there, and naming it is a separate step
afterwards. The argument order follows the scanner rather than what would read
more naturally.

The frequency is written in megahertz, which is the unit the scanner's own
screen asks for. Unlike [`tune`](tune.md), no unit suffix is accepted here,
because the screen accepts only digits and a decimal point.

Those screens are also the one place in this tool where text is typed rather
than dialled. A name screen ignores the number keys entirely and has to be
turned to each character with the knob; a number screen takes the digits
directly. Which is which is decided from the characters each screen says it
accepts. The talkgroup screen also accepts `-`, which separates the halves of a
Motorola or EDACS talkgroup, and `i`, which marks an I-call. Neither can be sent
from here, because the keys that produce them have not been found, so a
talkgroup needing either has to be entered on the scanner itself.

The channel is read back afterwards to confirm it is there, on the address asked
for.

**This stops the scanner scanning**, and returns it when finished.

## When to use `scanning` instead

[`scanning`](scanning.md) answers a different question. It reports every channel
in the current scan, across all departments and systems, and it reaches the full
database, which this command cannot.

Use `channels` when you want one named department, including one that is
switched off. Use `scanning` when you want to know what the scanner is set up
to receive right now.

## The keyword that was not broken

This command used to walk the scanner's menus and read its screen, taking
several seconds and stopping the scan while it ran, because the protocol request
for a department's channels was believed to be broken: it answered with the
favorites list document whatever index it was given, and reported no error while
doing so. The request for a trunked site's frequencies did the same.

Neither was a firmware bug. **The keywords were wrong.** A department's contents
are `CFREQ` and `TGID`, not `CHN`, and a site's frequencies are `SFREQ`, not
`SITE_FRQ`. The scanner does not refuse a keyword it does not know; it answers
with the favorites list instead, which looks exactly like a request that failed
in a strange way.

Two things follow that are worth keeping in mind:

- **Check the elements, not the error.** A wrong keyword is reported as success.
  Every list read in this tool checks that the elements it got back are the ones
  it asked for, which is what made this findable at all.
- **A department is two lists, not one.** `CHN` was a guess at a single request
  for "the channels", and no such request exists, because a channel holds a
  frequency or a talkgroup depending on the system above it.

The menu walk is still there as a fallback, used only if the protocol will not
answer. It is what worked for as long as the keywords were wrong, and a firmware
that behaves differently would otherwise take this command with it.

One consequence of the old walk survives in the data. **Names are stored exactly
as they were typed**, including oddities: a channel really can be called
`FIREGROUND (`, because `(` sits a couple of positions from the digits on the
character wheel and a knob that overshoots while the name is being typed lands
there. A name like that is the stored name rather than a misreading, and
[`channels rename`](#channels-rename) is how to correct one without losing the
channel's frequency.

## `channels rename`

`rename` changes a channel's name and leaves everything else about it alone:

```
$ radiocli channels rename "FIRE RESCUE" "FIREGROUND (" "FIREGROUND 2"
FIREGROUND 2
```

**The department comes first**, the same way round as `new` and `delete`, and
for the same reason: channel names are not unique, so naming the channel alone
would not say which one to rename.

**The frequency is kept.** That is the point of the command. A name can always
be corrected by deleting the channel and creating it again, but that loses the
frequency and every per-channel setting with it: the tone, the modulation, the
alert color, the audio type. `rename` touches only the name.

**Nothing is saved until the whole name is typed.** The name screen has to be
turned to each character with the knob, the same as everywhere else in this
tool, and leaving it partway discards. An interrupted rename leaves the old
name untouched rather than a half-typed one.

**A channel the department does not hold is refused**, and the refusal lists
what it does hold:

```
$ radiocli channels rename "DPW" "NO SUCH CHANNEL" "SOMETHING ELSE"
error: no channel called "NO SUCH CHANNEL" in "DPW": it holds "CH 1", "CH 2"
```

**Renaming to the name it already has does nothing**, and says so on stderr
without stopping the scanner scanning:

```
$ radiocli channels rename "DPW" "CH 1" "CH 1"
That channel is already called "CH 1".
```

The channel is read back afterwards to confirm the new name is there.

**This stops the scanner scanning**, and returns it when finished.

## `channels delete`

`delete` removes one channel from a department:

```
$ radiocli channels delete "TEST DEPT" "TEST CH" --yes
deleted TEST CH
```

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--yes` | Yes | `false` | Go ahead and delete it. |

**The department comes first**, the same way round as `new`. It has to be:
channel names are not unique. A scanner can hold several channels called
`DISPATCH`, one per department, and naming the channel alone would not say
which one to remove.

**`--yes` is required.** Without it nothing is touched and the command says
what would have gone:

```
$ radiocli channels delete "TEST DEPT" "TEST CH"
error: deleting the channel "TEST CH" from "TEST DEPT" cannot be undone: pass --yes
```

**A channel the department does not hold is refused**, and the refusal lists
what it does hold:

```
$ radiocli channels delete "TEST DEPT" "NO SUCH CHANNEL" --yes
error: no channel called "NO SUCH CHANNEL" in "TEST DEPT": it holds "TEST CH", "TEST CH 2", "TEST CH 3"
```

**There is no undo.** Nothing on the scanner keeps a copy and this tool cannot
put back what it removes. The department's channels are read back afterwards to
confirm it is gone.

**A name that is the start of another name is still told apart from it.**
Deleting `TEST CH` from a department that also holds `TEST CH 2` and
`TEST CH 3` removes only the first. The walk through the menus only treats a
row as a longer name when the scanner marked that row as one it had to cut
short to fit the screen.

**This stops the scanner scanning**, and returns it when finished.

## JSON output from `new`, `rename` and `delete`

The three verbs above print a line of text, and under `--output json` they print
an object instead. One shape covers all of them, and the same shape covers the
create, rename and delete verbs on every other level of the scanner's memory, so
a script driving edits does not have to learn a different object for each:

```json
{
  "action": "renamed",
  "kind": "channel",
  "name": "NEW NAME",
  "was": "OLD NAME"
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `action` | string | What happened: `created`, `renamed` or `deleted`. |
| `kind` | string | What it happened to, which is always `channel` here. |
| `name` | string | What the entry is called now, or was called when it was deleted. It is the scanner's own spelling, read back after the change. |
| `was` | string | The previous name, on a rename. Absent otherwise. |
| `in` | string | the department it is in, as you named it. |

Fields that do not apply are left out rather than written empty, so a consumer
can tell a create from a rename without comparing strings.

The text output is unchanged. It is what people already read, so the machine-
readable mode was added beside it rather than in place of it.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: accepts 1 arg(s), received 0` | No department was named. | Give an index or a name, from [`departments`](departments.md). |
| `error: no department is called "<name>": the scanner has ...` | No department carries that name. | Use one of the names in the message, or an index. |
| `error: opening the department's menu: <detail>` | The scanner would not open it. | Check it is not held in a menu; run [`scan`](scan.md) and try again. |
| `error: looking for "Edit Channel" on the department's menu: <detail>` | That entry was not found. | The menu is not laid out as expected, which a firmware change could cause. Run [`screen`](screen.md) with the department's menu open to see what it holds. |
| `error: reading the channel list: <detail>` | The channel list could not be read off the screen. | Run with `--verbose` to see the raw exchange. |
| `error: no channel called "<name>" in "<department>": it holds ...` | That department holds no channel by that name. | Use one of the names in the message. Names are matched without regard to case. |
| `error: no frequency was given: write it in megahertz, as 155.475, or write a talkgroup as TGID:9051` | The address was blank. | Give a frequency, or a talkgroup with the `TGID:` prefix. Nothing was created. |
| `error: no talkgroup was given: write it as TGID:9051` | The address was the `TGID:` prefix with nothing after it. | Write the talkgroup after the prefix. Nothing was created. |
| `error: "<value>" is not a frequency: write it in megahertz, as 155.475, or write a talkgroup as TGID:9051` | The address is neither a number nor a talkgroup. | Write it in megahertz, or with the `TGID:` prefix. Nothing was created. |
| `error: "<value>" is not a frequency the scanner's screen would accept: write it in megahertz, as 155.475, or write a talkgroup as TGID:9051` | The frequency is a number, but carries a sign or an exponent, and the entry screen has keys for digits and a decimal point alone. | Write the same frequency in plain digits. Nothing was created. |
| `error: looking for "Edit Name" on the channel's menu: <detail>` | The rename screen was not found. Nothing was changed. | The menu is not laid out as expected, which a firmware change could cause. Run [`screen`](screen.md) with the channel's menu open to see what it holds. |
| `error: typing the new name: <detail>` | A character would not go in. Nothing was saved, and the old name stands. | Run [`scan`](scan.md) to leave the entry screen, then try again. |

When something fails partway, the scanner is returned to scanning before the
error is reported, so a failed listing does not leave it parked in a menu.
Nothing is ever written: every screen this opens is read and left.
