# config macro

Stores named lists of commands, so a routine you repeat becomes one named thing
a front end can offer as a single press. Use it to set those up, or to read and
change them from a terminal.

## Overview

A macro is a name and a list of command lines, in the order they run. It is kept
in the same config file as the rest of this tool's settings, and a front end
reading that file offers it as one control: pressing it runs the commands one
after another. This command is how macros are created, listed, changed and
deleted, and it is the only thing that writes them.
**Nothing here runs a macro.** Running one is the front end's job, and it does
it by sending each step exactly as if you had typed it, so a macro can do no
more than you could do by hand. Nothing in this command opens the serial port or
sends anything to the radio, and no scanner needs to be attached for any of it.
Every step is checked when it is stored, so a line that could never be split
into a command is refused now rather than failing partway through a macro later.
Macro names are matched whatever case you type them in, and two macros cannot
have names that differ only in case. Twelve macros are built in, so the panel has
something on it before you have made anything; they are described in the next
section and can be edited and deleted like any other.

## Default macros

A config file that names no macros gets these twelve. They are what
`config macro` lists, and what a front end shows, on a machine where nothing has
been set up.

| Name | Runs | What it does |
| ---- | ---- | ------------ |
| `Resume Scanning` | `scan` | Takes the scanner back to scanning from wherever it has been left. |
| `Color Mode` | `display mode color` | Draws each part of the screen in the color set for it. |
| `Dark Mode` | `display mode black` | White text on a black background, ignoring those colors. |
| `Light Mode` | `display mode white` | Black text on a white background, ignoring those colors. |
| `Toggle Backlight` | `backlight keys toggle` | Makes the keypad start or stop lighting up with the screen. |
| `Mute Speaker` | `volume set 0` | Sets the volume to `0`, which is silent. |
| `Toggle Key Beep` | `beep toggle` | Silences the sound the keypad makes, or puts back the loudness it had. |
| `Monitor Weather` | `weather` | Measures the seven NOAA weather channels and holds the strongest. |
| `Tune to 107.9 FM` | `tune 107.9` | Puts the scanner on 107.9 MHz and holds it there. |
| `Sync Clock` | `clock sync` | Copies this computer's date and time onto the scanner. |
| `Sync Colors` | `colors --all` | Reads the colors of all seven screen layouts, so a page shows them. Takes about three minutes. |
| `Reset Colors` | `colors reset` | Puts every screen layout back to the colors the scanner left the factory with. |

`Resume Scanning` is [`scan`](scan.md), and it is first because it is the one
to press when the radio is not doing what it should be. It takes the scanner
back to scanning from anywhere it can be left: inside a menu, holding one
frequency, holding one channel, or on the weather channels. On a scanner that
is already scanning it presses nothing at all.

The three display macros are the modes [`display`](display.md) offers. Setting
one walks the scanner's menus and **stops the scan for as long as it takes**,
which is a few seconds. `Mute Speaker` is [`volume`](volume.md) set to its
lowest level; there is no separate mute on the scanner, and the level it had is
not remembered, so turning the sound back on means setting a level. `Sync Clock`
is [`clock sync`](clock.md), which is the usual repair for a scanner whose own
clock has drifted or was lost while it sat without power; it takes the daylight
saving this computer applies to the current date.

`Sync Colors` is [`colors --all`](colors.md#reading-every-layout), which reads
the colors of all seven screen layouts. The colors are not in the remote
protocol, so the only way to know them is to walk the menus that set them, which
takes about three minutes for all seven and stops the scan throughout. That is
why it is a button rather than something that happens on its own.

**All seven rather than the one on screen**, because the layouts you are not
looking at are the ones this has to have read. A page draws whatever the scanner
switches to, and a layout that has never been read has no colors to draw with,
so it comes out white on black while the radio is in color. Reading only the
current layout left the other six that way until somebody thought to press this
while looking at each of them in turn.

It is also the button to press after changing a color on the radio itself, which
nothing tells the tool about: a page shows the last colors read until this is
pressed.

`Reset Colors` is [`colors reset`](colors.md), and it is last because it is the
only one that throws anything away. It puts every layout back to the colors the
scanner left the factory with, which is the way out of a screen customized into
unreadability, and it is the only one here whose effect no other button undoes. `Toggle Backlight` is
[`backlight keys toggle`](backlight.md), which switches the keypad's light and
leaves the screen's own light alone. It is one command for both ways because
the keypad light is a setting rather than a switch, and that setting lives in a
menu: running it walks there and back, which stops the scan for a few seconds.
`Monitor Weather` is [`weather`](weather.md), and it is one of the two defaults
that change what the scanner is listening to rather than how it looks or sounds.
It takes about six seconds, because it measures all seven weather channels
before parking on the best one. There is no matching button to undo it: run
`radiocli weather stop` or [`scan`](scan.md) to return the scanner to whatever
it was scanning.

`Tune to 107.9 FM` is [`tune`](tune.md) on one frequency, and it sits beside the
weather button because it is the same kind of thing: it changes what the radio
is listening to, and [`scan`](scan.md) is what undoes either of them. A
broadcast station is an odd thing for a scanner to sit on, which is rather the
point of having it as a button: it is a known, always-there signal in a band the
receiver covers, so it answers "is this thing working, and is the audio reaching
me" in one press, without waiting for something to happen on a police channel.
Change the frequency to whatever is strong where you are with
`config macro set "Tune to 107.9 FM" "tune <frequency>"`, and rename it to
match.

`Toggle Key Beep` is [`beep toggle`](beep.md#beep-toggle), and it sits beside
the speaker button because both are about silence: the keypad's own chirp is a
separate sound from what is being received, and muting one leaves the other.
It toggles rather than setting because the key beep is a loudness rather than a
switch, and **nothing on the radio remembers the level once it has been turned
off**. The command writes it down instead, which is what lets one button both
silence the keypad and put it back the way it was. Pressed on a scanner that is
already silent with nothing written down, it leaves it silent.

They are ordinary macros in every other way. `set`, `rename` and `delete` work
on them exactly as on one you made.

**A default you delete does not come back.** The first write to the config file
puts the macros it holds into the file, and deleting the last one leaves
`"macros": []` there, which says you want none. This is the only value in the
file that is written even when it is empty, and that is why.

**A default you have not changed is not in the config file.** Reading the macros
does not write anything, so on a machine where nothing has been set up the twelve
exist only as the answer, not as a file. The first time any macro is created,
changed, renamed, moved or deleted, the rest are written out alongside whatever
you did.

## Usage

```
radiocli config macro [flags]
radiocli config macro show <name> [flags]
radiocli config macro new <name> <command>... [flags]
radiocli config macro set <name> <command>... [flags]
radiocli config macro rename <name> <new-name> [flags]
radiocli config macro move <name> <up|down> [flags]
radiocli config macro delete <name> --yes [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<name>` | Yes | none | Which macro to read or change, for `show`, `new`, `set`, `rename`, `move` and `delete`. |
| `<new-name>` | Yes | none | What to call the macro instead, for `rename` only. |
| `<command>...` | Yes | none | The command lines the macro runs, in order, for `new` and `set`. At least one. |
| `<up\|down>` | Yes | none | Which way to move the macro, for `move` only. Exactly `up` or `down`. |
| `--keep-going` | No | `false` | Run the remaining steps when one fails, for `new` and `set` only. |
| `--yes` | No | `false` | Go ahead and delete it, for `delete` only. |

The bare command takes no arguments and no flags of its own.

### `<name>`

What the macro is called, and what its button says. It is matched
whatever case you type it in, so `config macro show "night watch"` finds a macro
stored as `Night watch`.

A name is trimmed of leading and trailing spaces before it is stored. It must
not be empty, must not contain a tab or a line break, and must be 40 characters
or fewer, because it has to fit on a button. Any other characters are accepted,
including spaces and punctuation.

Two macros cannot have names that differ only in case: `new` refuses a name that
is already taken, and `rename` refuses a name held by a different macro.
Renaming a macro to its own name in another capitalization is allowed, and is
how you change how the button is written.

### `<command>...`

The command lines the macro runs, in the order given. Each one is written as you
would type it in a terminal, without the `radiocli`, so `volume set 4` is one
step. Quote each step, or your shell will hand its words over as separate steps.

```
radiocli config macro new "Night watch" "volume set 4" "backlight on" scan
```

A macro needs at least one step. Every step is checked before anything is
written:

- A step that is empty, or that is nothing but spaces, is refused.
- A step is split the way a terminal would split it, understanding single
  quotes, double quotes and backslash escapes, so
  `favorites scan "GREENDALE, ST 00000"` is one step with the list name intact.
- A step that cannot be split is refused, naming which step it was. That covers
  an unclosed `"` quote, an unclosed `'` quote, and a line ending in a
  backslash.
- A step containing any of the characters `|`, `&`, `;`, `<`, `>`, `(`, `)`,
  `$`, or a backtick is refused. There is no shell anywhere in the path, so
  there are no pipes, redirects, background jobs or variables. To save the
  output of a command, run it in a terminal and redirect it there.

The steps of a macro are not checked against the list of commands the tool has.
A step naming a command that does not exist is stored, and fails when a front end
runs it.

**A new macro goes at the top of the list**, above the built-in ones and above
anything made before it, so the one just written is the first button on the
panel rather than the last. Use `move` to put it somewhere else.

`new` refuses a name that already exists. `set` refuses a name that does not,
and replaces every step of the macro it names. There is no way to change one
step: the steps are an ordered list, and giving the whole list is the only way
to say what order it ends up in. Run `config macro show` first to see what is
there now.

`set` leaves the macro's name exactly as it is stored, including its
capitalization, however you typed the name to reach it.

### `<up|down>`

Which way to move a macro among the others, one place per run. Exactly `up` or
`down`; any other value is refused and nothing is written.

`up` is towards the start of the list, which is the top of the panel on the
page. `down` is towards the end, which is the bottom.

```
radiocli config macro move "Mute Speaker" up
```

The order macros are stored in is the order their buttons appear in, and this is
the only thing that changes it. Moving a macro carries its steps with it.

A macro already at that end of the list is refused rather than left where it is,
so a move that could not happen is said rather than passed over. Both messages
are under Errors.

### `--keep-going`

Runs the remaining steps when one fails, instead of stopping at the first
failure. It applies to `new` and `set`, and stores the choice with the macro.

```
radiocli config macro new "Quick look" battery screen --keep-going
```

Off by default, because a failed step leaves the scanner wherever it got to, and
the steps that walk the radio's menus press keys chosen for a screen it is no
longer on. Turn it on for a macro whose steps do not depend on each other.

**`set` writes this every time, from the flag alone.** Running `set` without
`--keep-going` on a macro that had it turns it back off.

### `--yes`

Confirms a deletion. `delete` refuses to do anything without it, and names the
macro and how many steps it has so you can tell from the refusal whether it is
the one you meant.

```
radiocli config macro delete "Quick look" --yes
```

There is nothing to undo a deletion with. Nothing on the scanner is touched: a
macro is a list of commands rather than anything on the radio.

### Global flags that change this command

`--config` chooses which config file is read and written, so macros can be kept
in a file of their own. `--output` chooses between the table and JSON described
under Output. The rest are described in [global flags](../global_flags.md).

**A macro you create with `--config` is not one a front end will show**, unless
that front end was started against the same file. Its default location is
described in [global flags](../global_flags.md).

## Examples

Listing them on a machine where nothing has been set up, which is the twelve
built-in macros:

```
$ radiocli config macro
NAME              STEPS  ON FAILURE
Resume Scanning   1      stop
Color Mode        1      stop
Dark Mode         1      stop
Light Mode        1      stop
Toggle Backlight  1      stop
Mute Speaker      1      stop
Toggle Key Beep   1      stop
Monitor Weather   1      stop
Tune to 107.9 FM  1      stop
Sync Clock        1      stop
Sync Colors       1      stop
Reset Colors      1      stop
```

Creating one, which prints back what was stored:

```
$ radiocli config macro new "Night watch" "volume set 4" "backlight on" scan
Night watch: 3 steps
  volume set 4
  backlight on
  scan
```

It goes on the top, which is where its button appears:

```
$ radiocli config macro
NAME              STEPS  ON FAILURE
Night watch       3      stop
Resume Scanning   1      stop
Color Mode        1      stop
Dark Mode         1      stop
Light Mode        1      stop
Toggle Backlight  1      stop
Mute Speaker      1      stop
Toggle Key Beep   1      stop
Monitor Weather   1      stop
Tune to 107.9 FM  1      stop
Sync Clock        1      stop
Sync Colors       1      stop
Reset Colors      1      stop
```

Creating one that carries on past a failure:

```
$ radiocli config macro new "Quick look" battery screen --keep-going
Quick look: 2 steps
  battery
  screen
```

Reading one macro's steps, which prints them and nothing else:

```
$ radiocli config macro show "Night watch"
volume set 4
backlight on
scan
```

Replacing every step of one:

```
$ radiocli config macro set "Night watch" "volume set 2" scan
Night watch: 2 steps
  volume set 2
  scan
```

Moving one up the list, which answers with the whole list in its new order:

```
$ radiocli config macro move "Mute Speaker" up
NAME              STEPS  ON FAILURE
Quick look        2      keep going
Night watch       2      stop
Resume Scanning   1      stop
Color Mode        1      stop
Dark Mode         1      stop
Light Mode        1      stop
Mute Speaker      1      stop
Toggle Backlight  1      stop
Toggle Key Beep   1      stop
Monitor Weather   1      stop
Tune to 107.9 FM  1      stop
Sync Clock        1      stop
Sync Colors       1      stop
Reset Colors      1      stop
```

Renaming a built-in one, which is a macro like any other:

```
$ radiocli config macro rename "Mute Speaker" "Silence"
Silence: 1 step
  volume set 0
```

Deleting it, which needs `--yes`:

```
$ radiocli config macro delete "Silence" --yes
deleted: Silence
```

It does not come back:

```
$ radiocli config macro
NAME              STEPS  ON FAILURE
Quick look        2      keep going
Night watch       2      stop
Resume Scanning   1      stop
Color Mode        1      stop
Dark Mode         1      stop
Light Mode        1      stop
Toggle Backlight  1      stop
Toggle Key Beep   1      stop
Monitor Weather   1      stop
Tune to 107.9 FM  1      stop
Sync Clock        1      stop
Sync Colors       1      stop
Reset Colors      1      stop
```

Reading one as JSON, which is the shape a front end builds its buttons from:

```
$ radiocli -o json config macro show "Night watch"
{
  "name": "Night watch",
  "steps": [
    "volume set 2",
    "scan"
  ]
}
```

On a machine where every macro has been deleted:

```
$ radiocli config macro
No macros yet. Run "radiocli config macro new" to create one.
```

## Output

Results go to stdout. Advice goes to stderr, as do debug logs from `--verbose`.

Under `--output text`, the bare command writes a header row and one row per
macro, in the order they are stored, which is the order their buttons appear
in:

| Column | Description |
| ------ | ----------- |
| `NAME` | The macro's name, as the other subcommands take it. |
| `STEPS` | How many command lines it runs. |
| `ON FAILURE` | `stop` or `keep going`. |

When there are no macros, nothing is written to stdout and
`No macros yet. Run "radiocli config macro new" to create one.` is written to
stderr, so a script counting the lines of stdout gets zero rather than a
sentence. That happens only when every macro has been deleted: a config file
that names none at all gets the twelve built-in ones instead.

`config macro show` writes the steps alone under `--output text`, one per line,
with nothing around them.

`config macro new`, `config macro set` and `config macro rename` write the macro
back after the file has been written, as `name: N steps` followed by one
indented line per step. A macro with one step reads `name: 1 step`. The macro is
read back from the file rather than echoed, so what is printed is what was
stored.

`config macro move` writes the whole list afterwards, in the same form the bare
command does, because where one macro ended up is only answerable by the order
they are all in.

`config macro delete` writes `deleted: <name>` under `--output text`.

Under `--output json`, the bare command and `move` each write an array of
objects, in the same order, and an empty array when there are none. `show`,
`new`, `set` and `rename` each write one object of the same shape:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The macro's name. |
| `steps` | array of string | The command lines it runs, in order. Always at least one. |
| `keepGoing` | boolean | Whether the remaining steps run after one fails. **Absent when false.** |

`config macro delete` writes a different object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The macro that was deleted, named as it was stored. |
| `deleted` | boolean | Always `true`. A failure to delete is an error rather than `false`. |

**Every write creates the config file if it is not there yet**, along with any
missing directories, and writes it with permissions `0600`. Macros are stored
under the `macros` key, described in [global flags](../global_flags.md). Every
other setting in the file is left exactly as it was.

**Every write stores the whole list**, including any built-in macros you have
not touched, so the first change to any of them puts all of them in the file.
The `macros` key is written even when the list is empty, which is what makes a
deleted macro stay deleted.

**Nothing is written when a macro is refused.** Every check happens before the
file is touched.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no macro called "Nope": the macros are "Night watch"` | No macro has that name. The message names every macro there is. | Use one of the names in the message, or create it with `new`. |
| `error: no macro called "X": there are no macros yet` | The same, on a config file holding no macros at all. | Create one with `new`. |
| `error: there is already a macro called "Night watch": run "radiocli config macro set" to change its steps, or pick another name` | `new` was given a name that is taken, or `rename` a name held by a different macro. The name is printed as it is stored, which may differ in case from what you typed. | Run `set` to change the existing macro, or pick another name. |
| `error: a macro needs a name` | The name given was empty or nothing but spaces. | Give a name. |
| `error: a macro's name cannot contain a tab or a line break: it is what its button says` | The name held a tab, a newline or a carriage return. | Remove it. |
| `error: the name "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" is longer than 40 characters: it has to fit on a button` | The name is over 40 characters. | Shorten it to 40 or fewer. |
| `error: step 1 is empty: every step is a command line to run` | A step was empty or nothing but spaces. The number is whichever step it was. | Remove it, or write a command in it. |
| `error: step 1, "favorites scan \"GREENDALE": unclosed " quote` | A step opened a quote and never closed it. The same message appears for `unclosed ' quote` and for `trailing backslash`. | Close the quote, or remove the backslash. |
| `error: step 1, "battery > out.txt": ">" is not supported: commands run directly rather than through a shell, so there are no pipes, redirects, background jobs or variables` | A step used a shell operator. The message names whichever character it was, out of `\|`, `&`, `;`, `<`, `>`, `(`, `)`, `$` and a backtick. | Remove it. To save output, run the command in a terminal and redirect it there. |
| `error: "Color Mode" is already first: there is nothing above it to move past` | `move ... up` was run on the macro at the top of the list. The order is unchanged. | Nothing to do: it is already where you were moving it. |
| `error: "Light Mode" is already last: there is nothing below it to move past` | `move ... down` was run on the macro at the bottom of the list. The order is unchanged. | Nothing to do: it is already where you were moving it. |
| `error: "sideways" is not a direction: want up or down` | `move` was given something other than `up` or `down`. The order is unchanged. | Pass `up` or `down`. |
| `error: deleting the macro "Look" removes it and its 2 steps, and cannot be undone: pass --yes` | `delete` was run without `--yes`. Nothing was deleted. | Pass `--yes` if it is the macro you meant. |
| `error: requires at least 2 arg(s), only received 1` | `new` or `set` was given a name and no steps. | Give at least one command after the name. |
| `error: accepts 1 arg(s), received 0` | `show` or `delete` was given no name. | Give a macro's name. |
| `error: accepts 2 arg(s), received 1` | `rename` was given one name and no new one. | Give the new name after the old one. |
| `error: "macros" is not a setting: a macro is a list of commands rather than a single value, and there can be any number of them, so they have a command of their own: run "radiocli config macro"` | `config get macros` or `config set macros` was run. Macros are not a single value, so they are not reachable that way. | Use this command. The same message answers `macro`. |
