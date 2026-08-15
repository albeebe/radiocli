# banks

Lists the ten frequency ranges the scanner can sweep, and changes them. Run it
to listen to a band the scanner's database does not cover, such as CB.

## Overview

A custom search bank is a range rather than a list of channels: a lower limit,
an upper limit, and the settings for sweeping between them. The scanner has
exactly ten, numbered `0` to `9`, and they always exist, so unlike a favorites
list there is nothing to create or delete. A bank is configured and then
searched. That makes it the way to hear a band nobody has programmed as
channels, where building a favorites list would mean entering every frequency
by hand. The bare command reads only: it changes nothing on the scanner and
writes nothing to the config file. `set` changes a bank, `scan` chooses which
banks the scanner sweeps and starts it sweeping them, and `goto` leaves the
scanner on one bank's menu to be worked by hand. It needs a scanner, so name
one with `--device`.

## Usage

```
radiocli banks [flags]
radiocli banks set <bank> [flags]
radiocli banks scan <bank>... [flags]
radiocli banks scan --all [flags]
radiocli banks goto <bank> [flags]
```

`<bank>` is a number from `0` to `9` everywhere it appears. Banks have names,
but a name is not accepted in place of a number: the scanner ships them all
called `Custom 0` through `Custom 9`, and nothing stops two of them being
given the same name.

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--full` | No | `false` | Also read the settings that need the scanner's menus opened. |

### `--full`

Without it, `banks` asks the scanner for its own list of banks. That is one
exchange, and it carries the name, both limits, the modulation and the step
for every bank the list reaches. The scanner cuts that list short, so the banks
it leaves out have their menus opened instead; on this radio that is the last
one, and the whole command takes about a second. See
[why the settings go in through the menus](#why-the-settings-go-in-through-the-menus)
for what is going on there.

With it, every bank's menu is opened in turn to read the five settings the
list does not carry: the attenuator, the delay, the digital waiting time, and
the two behind `Search with Scan`. **This stops the scanner scanning**, and
takes about fourteen seconds for all ten. The scanner is returned to scanning
before anything is printed.

```
radiocli banks --full --device /dev/cu.usbmodem00000000000011
```

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column
  of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the banks are printed as a table or as JSON.
- `--pace` sets the gap between key presses. It has no effect on the bare
  command unless the scanner's list stops short, and it matters to `--full`,
  `set`, `scan` and `goto`, which press keys for every step through the menus.

## Examples

Listing the banks:

```
$ radiocli banks
BANK  NAME      LOWER       UPPER       MOD   STEP
0     Custom 0  25.000000   27.999999   Auto  Auto
1     Custom 1  28.000000   29.699999   Auto  Auto
2     Custom 2  29.700000   49.999999   Auto  Auto
3     Custom 3  50.000000   53.999999   Auto  Auto
4     Custom 4  137.000000  143.999999  Auto  Auto
5     Custom 5  144.000000  147.999999  Auto  Auto
6     Custom 6  406.000000  419.999999  Auto  Auto
7     Custom 7  420.000000  449.999999  Auto  Auto
8     Custom 8  450.000000  469.999999  Auto  Auto
9     CB        26.965000   27.405000   AM    10.0 kHz
```

Reading every setting, which opens each bank's menu:

```
$ radiocli banks --full
BANK  NAME      LOWER       UPPER       MOD   STEP      ATT  DELAY  DIGITAL  AVOID          HOLD
0     Custom 0  25.000000   27.999999   Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
1     Custom 1  28.000000   29.699999   Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
2     Custom 2  29.700000   49.999999   Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
3     Custom 3  50.000000   53.999999   Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
4     Custom 4  137.000000  143.999999  Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
5     Custom 5  144.000000  147.999999  Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
6     Custom 6  406.000000  419.999999  Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
7     Custom 7  420.000000  449.999999  Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
8     Custom 8  450.000000  469.999999  Auto  Auto      Off  2 sec  400 ms   Stop Avoiding  2
9     CB        26.965000   27.405000   AM    10.0 kHz  Off  2 sec  400 ms   Stop Avoiding  2
```

Setting up a bank for the CB band and listening to it, which is the whole job
in two commands:

```
$ radiocli banks set 9 --range 26.965-27.405 --name CB --modulation AM --step 10k
BANK  NAME  LOWER      UPPER      MOD  STEP      ATT  DELAY  DIGITAL  AVOID          HOLD
9     CB    26.965000  27.405000  AM   10.0 kHz  Off  2 sec  400 ms   Stop Avoiding  2

$ radiocli banks scan 9
searching bank 9
```

Going back to scanning the favorites lists afterwards:

```
$ radiocli scan
The scanner is already out of the menus.
The scanner has left Custom Search and is scanning again.
```

## Output

The table goes to stdout. Debug logs from `--verbose` go to stderr.
Redirecting stderr leaves stdout holding the table alone.

Under `--output text`, stdout holds a header row and one row per bank, in bank
order, with the columns padded so they line up. A `-` in any column is a value
the scanner reported as empty, so that a column never silently collapses.

| Column | Description |
| ------ | ----------- |
| `BANK` | The bank's number, `0` to `9`. This is what every subcommand takes. |
| `NAME` | What the bank is called. Banks ship named `Custom 0` through `Custom 9`. |
| `LOWER` | The bottom of the range it sweeps, in megahertz. |
| `UPPER` | The top of the range it sweeps, in megahertz. |
| `MOD` | How it demodulates: `Auto`, `AM`, `NFM`, `FM`, `WFM` or `FMB`. |
| `STEP` | How far apart the frequencies it stops on are, such as `10.0 kHz`, or `Auto`. |

With `--full`, five more columns follow:

| Column | Description |
| ------ | ----------- |
| `ATT` | Whether the attenuator is applied: `On` or `Off`. |
| `DELAY` | How long the scanner waits on a transmission before moving on, such as `2 sec`. |
| `DIGITAL` | How long it waits for a digital signal to resolve, such as `400 ms`. |
| `AVOID` | Whether ordinary scanning sweeps this bank: `Stop Avoiding`, `Temporary Avoid` or `Permanent Avoid`. |
| `HOLD` | How many seconds ordinary scanning spends on this bank each time round. |

`AVOID` reads backwards on purpose: it is the scanner's own wording, and
`Stop Avoiding` is the setting that means the bank **is** swept.

Under `--output json`, stdout holds an array of objects, one per bank, in bank
order:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `bank` | number | The bank's number, `0` to `9`. Always present. |
| `name` | string | What the bank is called. Absent when the scanner reports none. |
| `lower` | string | The bottom of the range, in megahertz, without a unit. Absent when the scanner reports none. |
| `upper` | string | The top of the range, in megahertz, without a unit. Absent when the scanner reports none. |
| `modulation` | string | `Auto`, `AM`, `NFM`, `FM`, `WFM` or `FMB`. Absent when the scanner reports none. |
| `step` | string | The step in kilohertz, such as `10.0 kHz`, or `Auto`. Absent when the scanner reports none. |
| `attenuator` | string | `On` or `Off`. Present only with `--full`. |
| `delay` | string | The delay, as the scanner writes it. Present only with `--full`. |
| `digitalWait` | string | The digital waiting time, as the scanner writes it. Present only with `--full`. |
| `avoid` | string | `Stop Avoiding`, `Temporary Avoid` or `Permanent Avoid`. Present only with `--full`. |
| `holdTime` | string | Seconds, as digits with no unit. Present only with `--full`. |

Every field except `bank` is a string, including the frequencies and the hold
time, because they are reproduced as the scanner writes them rather than
converted.

`step` is the exception, and has to be. The scanner does not agree with itself
about how to write one: its menus say `10.0 kHz` and its list says `10000` for
the same setting, so a bank read one way would report a different value from a
bank read the other. Both are written here as kilohertz to one decimal place or
more, so `10.0 kHz` and `3.125 kHz`. That is the scanner's own wording apart
from the space, which it leaves out of `3.125kHz`, `6.25kHz` and `8.33kHz`.

```
$ radiocli banks --full --output json
[
  {
    "bank": 9,
    "name": "CB",
    "lower": "26.965000",
    "upper": "27.405000",
    "modulation": "AM",
    "step": "10.0 kHz",
    "attenuator": "Off",
    "delay": "2 sec",
    "digitalWait": "400 ms",
    "avoid": "Stop Avoiding",
    "holdTime": "2"
  }
]
```

## `banks set`

`set` changes one bank. Only the settings named are touched; everything else in
the bank is left as it was. The bank as it now stands is printed afterwards,
read back from the scanner rather than echoed from the request, so what you see
is what is stored.

```
$ radiocli banks set 9 --range 26.965-27.405 --name CB --modulation AM --step 10k
BANK  NAME  LOWER      UPPER      MOD  STEP      ATT  DELAY  DIGITAL  AVOID          HOLD
9     CB    26.965000  27.405000  AM   10.0 kHz  Off  2 sec  400 ms   Stop Avoiding  2
```

Every setting is entered by working the scanner's own menus, at roughly a
second per setting. **This stops the scanner scanning**, and returns it when it
is done. At least one setting must be named; with none, nothing is touched and
the command fails.

The read-back goes to stdout, in the same columns and with the same fields as
`banks --full`. Under `--output json` it is an array holding one object, not a
bare object, so the same reader handles it and `banks`.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<bank>` | Yes | none | Which bank to change, `0` to `9`. |
| `--range` | No | none | The frequencies to sweep, written `<lower>-<upper>` in MHz. |
| `--name` | No | none | What the bank is called. |
| `--modulation` | No | none | `auto`, `am`, `nfm`, `fm`, `wfm` or `fmb`. |
| `--step` | No | none | How far apart the frequencies are, such as `10k`, or `auto`. |
| `--attenuator` | No | none | `on` or `off`. |
| `--delay` | No | none | How long to wait on a transmission before moving on. |
| `--digital-wait` | No | none | How long to wait for a digital signal to resolve. |
| `--avoid` | No | none | Whether ordinary scanning sweeps this bank: `off`, `temporary` or `permanent`. |
| `--hold-time` | No | none | Seconds ordinary scanning spends on this bank each time round. |

Omitting a flag leaves that setting alone. There is no way to clear one: every
setting always holds a value, so a bank is changed rather than emptied.

### `--range`

Both limits in one value, written lower first, in megahertz, separated by a
hyphen:

```
radiocli banks set 9 --range 26.965-27.405
```

It is one flag rather than two on purpose. The scanner checks each limit
against the other as it is typed, so a new lower limit above the upper limit
the bank currently holds is refused even when the pair you are asking for makes
perfect sense. Given both at once, `set` writes them in whichever order the
scanner accepts: the upper first when the new range sits above the old one, the
lower first when it sits below.

The range is parsed before the scanner is touched, so a range written wrongly
costs nothing and leaves the scanner scanning. Megahertz is the only unit
accepted; `26965k` is not a range.

### `--modulation`

One of `auto`, `am`, `nfm`, `fm`, `wfm` or `fmb`, in any case. `Auto` lets the
scanner choose from the band. `FMB` is broadcast FM.

```
radiocli banks set 9 --modulation AM
```

### `--step`

How far apart the frequencies the scanner stops on are. The scanner offers
`Auto`, `2.5 kHz`, `3.125kHz`, `5.0 kHz`, `6.25kHz`, `7.5 kHz`, `8.33kHz`,
`10.0 kHz`, `12.5 kHz`, `15.0 kHz`, `20.0 kHz`, `25.0 kHz`, `50.0 kHz` and
`100.0 kHz`, and writes them with the spacing shown, inconsistently. That is
what the error message lists when the value is not one of them.

You do not have to reproduce that spacing. Anything that reads as a number is
compared as one, so `10k`, `10.0k`, `10.0 kHz` and `10000` all find the same
setting. `auto` matches on the word.

```
radiocli banks set 9 --step 10k
```

### `--attenuator`

`on` or `off`, in any case. It reduces the signal reaching the receiver, which
helps where a strong transmitter nearby is swamping everything else.

### `--delay`

How long the scanner stays on a frequency after a transmission ends, so that a
reply is heard rather than missed. The scanner offers `-10 sec`, `- 5 sec`,
`0 sec`, `1 sec`, `2 sec`, `3 sec`, `4 sec`, `5 sec`, `10 sec`, `30 sec`.

**The unit is part of the value.** `--delay 5` is refused, because `5 sec` is
not a number the way `10.0 kHz` is; write `--delay "5 sec"` or `--delay 5sec`.
The spacing does not matter, so `--delay -5sec` finds `- 5 sec`.

### `--digital-wait`

How long the scanner waits for a digital signal to resolve before deciding
there is nothing there. The scanner offers `0 ms`, `100 ms`, `200 ms`,
`300 ms`, `400 ms`, `500 ms`, `600 ms`, `700 ms`, `800 ms`, `900 ms`,
`1000 ms`. As with `--delay`, the unit is part of the value: `--digital-wait
400` is refused and `--digital-wait "400 ms"` is not.

### `--avoid`

Whether the scanner sweeps this bank while it is scanning your favorites
lists. This is a different question from [`banks scan`](#banks-scan), which
chooses the banks a custom search sweeps.

| Value | Means |
| ----- | ----- |
| `off` | Ordinary scanning sweeps this bank. |
| `temporary` | It does not, until the scanner is powered off and on again. |
| `permanent` | It does not, until this is set back. |

The scanner's own names for these are `Stop Avoiding`, `Temporary Avoid` and
`Permanent Avoid`, and those are accepted too. `off`, `none` and `stop` all
mean `Stop Avoiding`, `temp` means `temporary`, and `perm` means `permanent`.

```
radiocli banks set 9 --avoid off
```

### `--hold-time`

How many seconds ordinary scanning spends sweeping this bank each time it comes
round to it. A whole number, with no unit.

```
radiocli banks set 9 --hold-time 5
```

## `banks scan`

`scan` chooses which banks a custom search sweeps, and starts the scanner
sweeping them:

```
$ radiocli banks scan 9
searching bank 9
```

Naming banks searches exactly those and switches every other one off. That is
usually what is wanted: "search only this" rather than "also search this".

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<bank>...` | Yes, unless `--all` | none | Which banks to search, `0` to `9`, separated by spaces. |
| `--all` | No | `false` | Search every bank. |

Naming banks and passing `--all` together is refused. Naming none of either is
refused too.

`searching bank 9` is a note on stderr, not a result. Nothing goes to stdout,
so `--output json` prints nothing at all and exits `0`. It takes under a tenth
of a second: the banks are chosen with key presses on the operating screen
rather than through the menus.

**There is no way to search none of them.** The scanner refuses to be left in
Custom Search with nothing to sweep: the keypress that would switch off the
last remaining bank is dropped and the bank stays on. "Search nothing" means
leaving Custom Search, and [`scan`](scan.md) is what does that.

### This puts the scanner into Custom Search

A choice of banks only means anything in Custom Search, so this command puts
the scanner into it. That is what makes naming one bank the whole of listening
to a band: there is nothing to run afterwards. It is also why the scanner does
not go back to your favorites lists on its own, and why [`scan`](scan.md) is
what brings it back.

Unlike the settings, this is not done through the menus. In Custom Search the
number keys toggle the banks, one key per bank, and that is what this presses.
Each key toggles rather than switches on, so the command first reads which
banks are already on from the scanner's screen and presses only the ones that
differ. It is therefore safe to run twice.

The banks being switched on are pressed before the ones being switched off,
which matters for the reason above: turning a wanted one on first means there
is always something left for the unwanted ones to be taken away from.

If the screen cannot be read, nothing is pressed and the command fails.
Pressing a toggle without knowing what it is toggling switches off exactly the
bank that was wanted.

## `banks goto`

`goto` leaves the scanner on one bank's menu, so the rest of it can be worked
by hand. Nothing is changed.

```
$ radiocli banks goto 9
menu: CB

   INDEX  ENTRY
>  0      Edit Name
   1      Edit Srch Limit
   2      Set Delay Time
   3      Set Modulation
   4      Set Attenuator
   5      Set Step
   6      Digital Waiting Time
   7      Search with Scan
```

It takes a bank number and no flags of its own. The menu is printed as JSON
under `--output json`, in the same shape [`menu`](menu.md) uses.

Everything on that menu except `Edit Name` and `Edit Srch Limit` is reachable
through `set`, so this is for the occasions when it is quicker to turn the knob
than to remember a flag. **This stops the scanner scanning.** Run
[`scan`](scan.md) when finished.

## Why the settings go in through the menus

The protocol has a command for setting the value of a menu entry, and the
scanner refuses it on every screen a bank is made of. It is refused on the text
entry screens, on the frequency entry screens, and on the selection menus such
as `Set Modulation`, where it looks as though it should work. So `set` does
what a person does: it turns the knob to the entry, presses it, and either
types or picks from the list.

That is the whole reason a change takes about a second rather than being
instant, and the reason the scanner has to stop scanning to accept one.

Reading is different. Most of a bank the scanner will report outright, in a
single exchange, which is why the bare command is quick and does not interrupt
anything. What it will not report is read the same way it is written.

**The scanner's own list is not always complete.** It cuts the answer at about
a kilobyte and marks it as unfinished, and nothing asks for the rest: repeating
the command answers with the same first part, and a page number in the request
is ignored. With the names this radio ships, nine of the ten banks fit and the
tenth does not. Whatever the list leaves out is read by opening that bank's
menu instead, so the answer is always all ten. How many that is depends on how
long the names are, so a bank list full of long names takes longer to read than
one left at the factory names.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: no bank called "10": the banks are 0 to 9` | The bank number is outside the ten the scanner has, or is not a number. | Use a number from `0` to `9`. |
| `error: nothing to change: name at least one setting, such as --range or --name` | `set` was run with a bank and no settings. | Name a setting. Run `radiocli banks set --help` to see them. |
| `error: a range is written as <lower>-<upper> in MHz, such as 26.965-27.405, not "26.965"` | `--range` was given one frequency rather than two. | Write both limits, separated by a hyphen. |
| `error: the lower limit "abc" is not a frequency in MHz` | One end of `--range` is not a number. | Write it in megahertz, as digits. |
| `error: the lower limit 27.4050 MHz is not below the upper limit 26.9650 MHz` | The two ends of `--range` are the wrong way round, or equal. | Write the lower limit first. |
| `error: setting "Set Modulation": "zzz" is not one of the choices: Auto, AM, NFM, FM, WFM, FMB` | The value is not one the scanner offers. The choices are listed as the scanner spells them, starting from the one currently set, so the order changes between runs. | Use one of the listed values. This applies to `--modulation`, `--step`, `--attenuator`, `--delay`, `--digital-wait` and `--avoid`, each naming its own entry. |
| `error: "abc" is not a number of seconds` | `--hold-time` was given something that is not a whole number, or a negative one. | Write a whole number of seconds. |
| `error: setting the upper limit to 1027.4050 MHz: 1027.405 does not fit this screen, which holds 2 digits before the point` | A limit has more digits before the decimal point than the screen the scanner is showing. The screen is sized by the value the bank already holds, so a bank sitting at 26 MHz cannot be sent straight to 1027 MHz. | Set a limit of the same width first, in a bank whose current value has room, or work the bank up in steps. Nothing was written. |
| `error: naming banks and passing --all ask for different things: choose one` | `banks scan` was given both. | Pass one or the other. |
| `error: name the banks to search, or pass --all` | `banks scan` was given neither. | Name the banks, or pass `--all`. To stop searching altogether, run [`scan`](scan.md). |
| `error: could not tell which banks are switched on from the scanner's screen: <screen>` | `banks scan` reached Custom Search but could not read the row that says which banks are on. | Run `radiocli screen` to see what the scanner is showing. Nothing was pressed. |
| `error: opening bank 9: <detail>` | The scanner refused to open the bank's menu. | Run with `--verbose` to see the raw exchange. |
| `error: opening "Set Modulation": no entry called "Set Modulation" in this menu: it holds <entries>` | The walk through a bank's menu did not find the entry it wanted, which means the scanner was not where it was expected. | Run [`scan`](scan.md) to get the scanner back to a known place, then try again. |
