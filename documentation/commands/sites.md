# sites

Lists the sites of one trunked system, and creates them. A site is where a
trunked system's frequencies live.

## Overview

A conventional system is simple: each department holds the frequencies its
channels sit on, and a frequency belongs to one agency forever.

A trunked system does not work that way. The city buys a **pool** of
frequencies and shares it across every department on the system. When somebody
keys up, a computer hands out whichever frequency is free at that instant, for
the length of that one transmission. The next transmission lands somewhere else.

So a trunked system splits into two halves that sit beside each other rather
than one inside the other:

```
System (trunked)
├── Site          ← where the signal comes from, and the pool of frequencies
└── Department    ← who is talking, as talkgroups
```

`sites` is the first half. The site holds the frequency pool, and one of those
frequencies is the **control channel** at any moment, carrying the data that
says where each conversation has been sent. The scanner works out which by
itself, so the frequencies go in as a plain list with no roles attached.

The second half is [`departments`](departments.md) and
[`channels`](channels.md), where the talkgroups live.

Only a trunked system has sites. A conventional system reports none, which is a
complete answer rather than a failure.

This asks the scanner directly and takes one exchange. Listing does not stop
the scanner scanning; creating and deleting do. It needs a scanner, so name
one with `--device`.

## Usage

```
radiocli sites <system> [flags]
radiocli sites new <system> <name> [flags]
radiocli sites rename <site> <name> [flags]
radiocli sites delete <site> --yes [flags]
radiocli sites frequencies <site> [flags]
radiocli sites frequencies add <site> <frequency>... [flags]
radiocli sites frequencies delete <site> <frequency> --yes [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<system>` | Yes | none | The system to look inside, by index or by name. |

### `<system>`

Which system's sites to list. Get the name or index from
[`systems`](systems.md). A name is matched without regard to case, and costs the
lookup described there, because a system's index says nothing about which
favorites list holds it.

## Examples

Listing a trunked system's sites:

```
$ radiocli sites "CITY OF GREEN BANK"
NAME        SCANNED  QUICK KEY
GREEN BANK  yes      -
```

A conventional system has none:

```
$ radiocli sites "PUBLIC SAFETY"
That system has no sites. Only a trunked system does.
```

An index naming no system is told apart from a system that holds no sites. The
scanner answers both the same way, so the index is looked up before the answer
is believed:

```
$ radiocli sites 9999
error: no system has index 9999: run "radiocli systems" to see what there is
```

That costs one extra exchange, and only when there is nothing to report anyway.

Listing a site's frequency pool:

```
$ radiocli sites frequencies "GREEN BANK"
FREQUENCY
851.050000MHz
851.600000MHz
852.362500MHz
```

As JSON:

```
$ radiocli sites "CITY OF GREEN BANK" -o json
[
  {
    "name": "GREEN BANK",
    "index": "33",
    "avoided": false
  }
]
```

## Output

The table goes to stdout. The no-sites message goes to stderr, as do debug logs
from `--verbose`.

Under `--output text`, stdout holds a header row and one row per site:

| Column | Description |
| ------ | ----------- |
| `NAME` | The site's name, exactly as the scanner holds it. |
| `SCANNED` | `yes` unless the site is being avoided. |
| `QUICK KEY` | The quick key, or `-` when none is assigned. |

Under `--output json`, stdout holds an array of objects, in the order the
scanner lists them:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The site's name. |
| `index` | string | How the scanner names this site in other requests. |
| `avoided` | bool | Whether the scanner is skipping it. |
| `quickKey` | string | Absent when none is assigned. |

**An index is not stable.** It is assigned when the scanner loads its data into
memory and changes when the memory is rebuilt, which creating or deleting
anything does. Look a site up by name unless the index came from a read in the
same run.

`sites frequencies` reports one column, `FREQUENCY`, and under `--output json`
an array of objects with `frequency` and `index`, both strings.

A system with no sites prints nothing to stdout, says so on stderr, and exits
`0`. A site with no frequencies does the same, with a different message:

```
$ radiocli sites frequencies "GREEN BANK"
That site holds no frequencies, so the system will not track anything there.
```

## `sites new`

`new` creates a site inside a trunked system:

```
$ radiocli sites new "CITY OF GREEN BANK" "GREEN BANK"
GREEN BANK
```

Pressing `New Site` creates one immediately, called something like `SITE 0`, and
the name you asked for is typed over it.

**A new site holds no frequencies**, and the system will not track anything
until some are added. That is the next command, not an optional extra: a trunked
system with an empty site receives nothing at all.

**Only a trunked system can hold a site.** A conventional system's menu carries
no `Edit Site` entry, and this refuses rather than landing somewhere unexpected:

```
$ radiocli sites new "PUBLIC SAFETY" "NOWHERE"
error: looking for "Edit Site" on the system's menu: no entry called "Edit Site" in this menu: it holds "Edit Name", "Edit Sys Option", "Edit Department", "Copy System", "Delete System"
Only a trunked system has sites: a conventional one holds its frequencies in departments instead
```

**This stops the scanner scanning**, and returns it when finished.

## `sites frequencies add`

`add` puts one or more frequencies into a site's pool:

```
$ radiocli sites frequencies add "GREEN BANK" 851.050 851.600 852.3625
FREQUENCY
851.050000MHz
851.600000MHz
852.362500MHz
```

**Give them all at once.** A site runs a pool, and a system with only some of
its frequencies entered will lose every call handed to one it does not know.
That sounds like a busy system going quiet at random, which is a confusing
symptom to chase.

**Order does not matter and no roles are attached.** One of the pool is the
control channel at any moment, but which one changes, and the scanner finds it
by itself. There is nothing to mark.

**The frequencies are written in megahertz**, the unit the scanner's own screen
asks for. A unit suffix is accepted as well, in any spelling: `851.05`,
`851.05MHz`, `851.05mhz` and `851050kHz` are the same frequency, and are read
exactly as [`tune`](tune.md) reads them. The scanner's entry screen has keys for
digits and a decimal point alone, so a frequency written with a unit is
converted before it is typed; one written without is typed as it was written.

A number the screen could not be given at all, such as one carrying a sign or an
exponent, is refused rather than converted.

**A frequency the site already holds is skipped** rather than added twice:

```
$ radiocli sites frequencies add "GREEN BANK" 851.05
The site already has 851.05.
```

Comparison is by value rather than by text, so `851.05`, `851.050` and the
`851.050000MHz` the scanner writes back are all the same frequency. The note is
written to stderr; the site's pool still follows on stdout, in whichever format
was asked for, including when every frequency given was already there and
nothing was added.

**A mistyped frequency costs nothing.** Everything is checked before the scanner
is touched, so a bad value leaves it scanning:

```
$ radiocli sites frequencies add "GREEN BANK" abc
error: "abc" is not a number of megahertz: write it in megahertz, as 851.050
```

A number the entry screen has no keys for is refused separately, since the fix
is to write the same frequency differently rather than to write a different one:

```
$ radiocli sites frequencies add "GREEN BANK" 8.5105e2
error: "8.5105e2" is not a frequency the scanner's screen would accept: write it in megahertz, as 851.050
```

They are read back afterwards to confirm they are there. **This stops the
scanner scanning**, and returns it when finished.

## `sites rename` and `sites delete`

`rename` changes a site's name and leaves its frequencies alone:

```
$ radiocli sites rename "SITE 0" "GREEN BANK"
GREEN BANK
```

`delete` removes a site and every frequency in it, and requires `--yes`:

```
$ radiocli sites delete "GREEN BANK" --yes
deleted GREEN BANK
```

**Deleting the only site of a system leaves it silent.** The departments and
talkgroups stay where they are, but nothing carries them, so the system receives
nothing. That is worth knowing before removing a site to tidy up.

**There is no undo** for either. The scanner is read back afterwards to confirm.

Deleting takes the site's frequencies with it, which sends the scanner away to
work out what it can still hear. It answers nothing at all while it does, so the
command waits for it to come back before reading anything else.

## `sites frequencies delete`

`delete` removes one frequency from a site's pool, and requires `--yes`:

```
$ radiocli sites frequencies delete "GREEN BANK" 852.3625 --yes
deleted 852.3625
```

**A frequency the site does not hold is refused**, and the refusal lists what it
does hold:

```
$ radiocli sites frequencies delete "GREEN BANK" 999.999 --yes
error: the site does not hold 999.999: it holds 851.050000MHz, 852.362500MHz
```

Removing one the system really uses does not break the system, but the scanner
will miss every call handed to that frequency.

## JSON output from `new`, `rename` and `delete`

The three verbs above print a line of text, and under `--output json` they print
an object instead. One shape covers all of them, and the same shape covers the
create, rename and delete verbs on every other level of the scanner's memory, so
a script driving edits does not have to learn a different object for each:

```json
{
  "action": "renamed",
  "kind": "site",
  "name": "NEW NAME",
  "was": "OLD NAME"
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `action` | string | What happened: `created`, `renamed` or `deleted`. |
| `kind` | string | What it happened to, which is always `site` here. |
| `name` | string | What the entry is called now, or was called when it was deleted. It is the scanner's own spelling, read back after the change. |
| `was` | string | The previous name, on a rename. Absent otherwise. |
| `in` | string | the system it is in, as you named it. |

Fields that do not apply are left out rather than written empty, so a consumer
can tell a create from a rename without comparing strings.

The text output is unchanged. It is what people already read, so the machine-
readable mode was added beside it rather than in place of it.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: accepts 1 arg(s), received 0` | No system was named. | Give an index or a name, from [`systems`](systems.md). |
| `error: no system is called "<name>": the scanner has ...` | No system carries that name. | Use one of the names in the message, or an index. |
| `error: no site is called "<name>": the scanner has ...` | No site carries that name. | Run `sites <system>` to see what there is. |
| `error: no system has index <n>: run "radiocli systems" to see what there is` | The index given names no system. The scanner answers that the same way it answers a system holding no sites, so it is checked. | Use an index from [`systems`](systems.md), or a name. |
| `error: the scanner stopped answering and has not come back: it may still be rebuilding from its database: <detail>` | The site was deleted and the scanner did not return from rebuilding what it can hear. The detail is the last thing that went wrong while waiting. | Wait, then run `sites <system>` to see what is there. The delete itself will have landed. |
| `error: looking for "Edit Site" on the system's menu: <detail>` | The system is conventional, or the menu is not laid out as expected. | Only a trunked system has sites. Check the type with [`systems`](systems.md). |
| `error: "<value>" is not a number of megahertz: write it in megahertz, as 851.050` | A frequency could not be read, with or without a unit. | Write it in megahertz, as `851.050`. Nothing was changed. |
| `error: "<value>" is not a frequency the scanner's screen would accept: write it in megahertz, as 851.050` | The frequency is a number, but carries a sign or an exponent, and the entry screen has keys for digits and a decimal point alone. | Write the same frequency in plain digits. Nothing was changed. |
| `error: the site does not hold <frequency>: it holds ...` | That frequency is not in the pool. | Use one of the frequencies in the message. |
| `error: removing <frequency> from the site cannot be undone: pass --yes` | `--yes` was missing. | Add `--yes` if that is what you meant. Nothing was changed. |

When something fails partway through adding several frequencies, the ones
already entered stay on the scanner. The error says which one failed, and
`sites frequencies` shows what is there.
