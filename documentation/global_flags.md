# Global flags

These flags work on every `radiocli` command. Each command's own file documents
only the flags belonging to that command, and links here for these.

| Flag | Required | Default | Description |
| ---- | -------- | ------- | ----------- |
| `--config` | No | none | Path to the config file to read. |
| `--device` | No | none | Serial port of the scanner to use. |
| `-o`, `--output` | No | `text` | Output format. Accepts exactly `text` or `json`. |
| `--pace` | No | `turbo` | How quickly keys are sent to the scanner. Accepts exactly `slow`, `medium`, `fast` or `turbo`. |
| `--wait` | No | `0` | How long to wait for another `radiocli` to finish with the scanner. |
| `-v`, `--verbose` | No | `false` | Write debug logs to stderr. |
| `-h`, `--help` | No | `false` | Print help for the command and exit with status `0`. |

`--version` is accepted by the top-level `radiocli` command only, not by
subcommands. It prints one line, `radiocli version <version>`, and exits with
status `0`. For the full build details use [`version`](commands/version.md).

## Where settings come from

Three sources, each overriding the one before it:

1. The built-in defaults listed in the table above.
2. The config file.
3. The flags you type on the command line.

Only a flag you actually type overrides the config file. Leaving a flag out is
not the same as passing its default value: if the config file sets
`"output": "json"` and you do not pass `--output`, the output is JSON.

## `--config`

The config file to read. When omitted, the file is read from the default
location for your operating system:

| System | Path |
| ------ | ---- |
| macOS | `~/Library/Application Support/radiocli/config.json` |
| Linux | `$XDG_CONFIG_HOME/radiocli/config.json`, or `~/.config/radiocli/config.json` when `XDG_CONFIG_HOME` is unset |
| Windows | `%AppData%\radiocli\config.json` |

A missing file at the default location is not an error: the tool runs with its
defaults. A missing file you named with `--config` is an error, because naming
it means you meant to use it.

```
radiocli --config ./scanner.json status
```

The file is JSON with these keys. All are optional.

| Key | Type | Description |
| --- | ---- | ----------- |
| `verbose` | boolean | Same as `--verbose`. |
| `output` | string | Same as `--output`. Accepts exactly `text` or `json`. |
| `pace` | string | Same as `--pace`. Accepts exactly `slow`, `medium`, `fast` or `turbo`. |
| `macros` | array of object | The macros a front end offers as buttons. Absent until one is created, changed or deleted; see below. |

Each entry of `macros` is an object with these keys:

| Key | Type | Description |
| --- | ---- | ----------- |
| `name` | string | What the macro is called, and what its button says. At most 40 characters. |
| `steps` | array of string | The command lines it runs, in order, each written without the `radiocli`. Always at least one. |
| `keepGoing` | boolean | Whether the remaining steps run after one fails. Absent when false. |

```json
{
  "verbose": false,
  "output": "text",
  "pace": "turbo",
  "macros": [
    {
      "name": "Night watch",
      "steps": [
        "volume set 4",
        "backlight on",
        "scan"
      ]
    },
    {
      "name": "Quick look",
      "steps": [
        "battery",
        "screen"
      ],
      "keepGoing": true
    }
  ]
}
```

Two commands write this file, and each leaves every key it did not change
exactly as it was, so a one-off `--output json` or `--verbose` on the command
line is never made permanent. [`config set`](commands/config.md) and
`config unset` change one named setting.
[`config macro`](commands/config_macro.md) changes `macros`.

The scanner is deliberately not in this file. Which radio a command talks to is
a property of that command rather than of the machine, so it is named every
time with `--device` and never written down. A remembered scanner is a command
aimed at whatever was chosen once, which on a machine that has had more than
one attached is a command aimed at the wrong radio with nothing on screen to
say so.

Nothing else reads or writes `macros`, and no command in this tool runs a macro.
They are stored for a front end to offer, which sends each step exactly as if it
had been typed.

**A file with no `macros` key gets twelve built-in macros**: `Resume Scanning`,
`Color Mode`, `Dark Mode`, `Light Mode`, `Toggle Backlight`, `Mute Speaker`,
`Toggle Key Beep`, `Monitor Weather`, `Tune to 107.9 FM`, `Sync Clock`,
`Sync Colors` and `Reset Colors`, listed in
[`config macro`](commands/config_macro.md). They can be edited and deleted like
any others. `macros` is the one key written even when its value is empty, so
`"macros": []` means you want none and the twelve do not come back. Every other
key is left out of the file when it has nothing in it.

The `macros` key is deliberately not checked when the file is read. A macro
edited by hand into something the tool cannot use is reported when you next try
to store one, rather than stopping every command in the tool from running.

## `--device`

The serial port of the scanner to talk to, such as
`/dev/cu.usbmodem00000000000011`. Get the value from the `port` column of
[`devices`](commands/devices.md).

Required by every command that talks to the scanner. Nothing is remembered
between runs, so leaving it off fails with `no scanner named` rather than
falling back to a previous choice.

```
radiocli --device /dev/cu.usbmodem00000000000011 status
```

The port is never written to the config file. It applies to that one command,
and the next command has to name it again.

## `-o`, `--output`

The output format. Accepts exactly two values:

- `text` renders output for a person to read.
- `json` renders output for a program to parse.

Any other value fails before the command runs:

```
$ radiocli -o bogus status
error: invalid output format "bogus": want "text" or "json"
```

In both formats, the result goes to stdout and everything else goes to stderr:
progress messages, prompts, advice, and debug logs. Redirecting stderr leaves
stdout holding only the result.

Under `json`, a command documented to answer with an array always answers with
one. A listing that found nothing is `[]`, never `null`, so a reader can take its
length and range over it without checking which it got:

```
$ radiocli --device $SDS sites "GREEN BANK" -o json
[]
```

Individual fields inside an object are a separate matter: several commands leave
a field out when the scanner reported nothing for it, and each command page says
which of its fields can be absent.

## `--pace`

How quickly keys are sent to the scanner. Accepts exactly four values:

| Value | Gap between keys |
| ----- | ---------------- |
| `slow` | 1 second |
| `medium` | 0.5 seconds |
| `fast` | 0.1 seconds |
| `turbo` | none |

The scanner is a handheld radio, not a terminal. It acts on a keypress,
redraws its screen, and settles; a key sent before it has finished can be
ignored, or acted on from the screen it was about to leave. The pace is the
minimum gap between one key and the next.

The gap is measured from when the last key was sent, not slept after it, so
time already spent reading the screen counts towards the gap rather than being
added to it. `turbo` adds nothing at all, which leaves the serial round trip of
roughly 28ms as the only spacing.

```
radiocli --pace slow menu goto settings
```

`turbo` is the default, and sends keys as fast as the port accepts them, which
is far quicker than the scanner redraws. That is safe for the commands that
press runs of keys, because each of them checks the result of every press: a
walk through the menus reads which entry is highlighted before pressing again,
and typing a name reads each character back, so a dropped press is noticed and
repeated.

[`key`](commands/key.md) is the exception. It presses what you ask and checks
nothing, so a run of presses at `turbo` can be dropped or applied to a screen
the scanner was about to leave. Choose a slower pace when driving the scanner
by hand.

This flag affects the commands that press keys: [`key`](commands/key.md),
[`scan`](commands/scan.md), the `goto` and `rename` subcommands of
[`favorites`](commands/favorites.md), `scan` on the same,
`rename` on
[`systems`](commands/systems.md) and [`departments`](commands/departments.md),
`rename` on [`channels`](commands/channels.md) and
[`sites`](commands/sites.md), [`scanning`](commands/scanning.md), which holds the
scanner and turns the knob, `set` and `gps` on
[`location`](commands/location.md), `mode` on
[`display`](commands/display.md), and
[`colors`](commands/colors.md), which presses several hundred keys to read one
layout and so feels the pace more than anything else here. The
`new` and `delete` subcommands of all four memory commands press keys too. It
changes nothing for commands that only read.

Setting the pace on the command line does not save it. To change it for good,
set the `pace` key in the config file.

## `--wait`

How long to wait for another `radiocli` to finish with the scanner, as a
duration such as `30s` or `5m`. The default is not to wait at all.

**Only one `radiocli` talks to a scanner at a time.** A command claims the
port before it opens it and holds the claim until it exits, so a second command
started while the first is still running is refused rather than allowed to talk
over it:

```
$ radiocli battery
error: /dev/cu.usbmodem00000000000011 is in use by another radiocli: pid 37934 running "scanning" for 21s. Wait for it to finish, or pass --wait to queue behind it
```

The refusal names the process, what it is doing, and how long it has been at
it, because the usual reason to see this is having forgotten that something
else is still going.

`--wait` queues behind it instead of giving up:

```
radiocli --wait 5m battery
```

The command sits there until the port comes free or the duration runs out,
whichever happens first, and reports the same refusal if it runs out.

**Waiting is off by default because the wait can be long.** Anything that
types a name spells it into the scanner one character at a time, so a
`favorites rename` can hold the port for minutes, and a command that blocked
silently for that long would be indistinguishable from one that had hung. A
script that can afford to queue asks for it; a person at a terminal finds out
straight away.

### Sharing a scanner instead of queueing for it

[`daemon`](commands/daemon.md) changes the refusal above into a wait. It holds
one scanner and runs other invocations' commands on it, so a second command
takes its turn rather than being told the port is busy. Commands still run one
at a time, because the scanner still answers one at a time.

Nothing starts it for you, and a command only looks for one after being refused,
so a script written against the refusal above behaves exactly as it always did
until you choose to run a daemon.

### Why the claim covers a whole command

Two commands sharing the port do not merely take turns badly. The scanner
answers on one line with nothing in the reply saying who asked, so each
command reads the other's answers. Measured on an SDS150, a `scanning` and a
`battery` run started together both failed, one on a response that stopped
halfway and the other on no response at all, and both reported it as the
scanner being unreachable.

Two failed reads is the harmless case. The commands that walk menus press a
key, read the screen, and choose the next key from what they saw, so a second
command pressing keys in the middle of one leaves the scanner somewhere neither
of them meant to put it: an entry screen holding half a name, with nothing
reported as having gone wrong. The claim covers a whole command because what
has to be indivisible is the menu walk, not the request.

### What it does not cover

The claim only holds between programs that take it, which means this tool and
nothing else. Scanner programming software and serial terminals know nothing
about it and can still interleave with a command that is running.

It is also per user: two people signed in to the same machine driving one
scanner will not see each other's claims.

The claim is released by the operating system when the command ends, however it
ends, so a command that is killed leaves nothing behind and there is never a
stale claim to break.

### Discovery

[`devices`](commands/devices.md) leaves a busy port alone rather than
interrupting whatever is using it, so a scanner another command is holding is
reported as busy rather than listed:

```
$ radiocli devices
every scanner found is in use by another radiocli: /dev/cu.usbmodem00000000000011

Wait for it to finish and run this command again
```

## `-v`, `--verbose`

Writes debug logs to stderr, including which ports were checked during a scan
and every command sent to the scanner. It does not change what a command does
or what it writes to stdout.

```
$ radiocli -v devices
time=2026-08-02T13:10:55.904-04:00 level=DEBUG msg="scanning for attached scanners"
time=2026-08-02T13:10:55.911-04:00 level=DEBUG msg="found scanner" port=/dev/cu.usbmodem00000000000011 model=SDS150
   MODEL   SERIAL         PORT
*  SDS150  0000000000001  /dev/cu.usbmodem00000000000011

* currently selected
```

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0` | The command succeeded. |
| `1` | The command failed. The reason is printed to stderr, prefixed with `error: `. |

Interrupting a command with Ctrl-C exits with status `1` and prints nothing.

## Errors common to every command

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: invalid output format "<value>": want "text" or "json"` | `--output`, or the `output` key in the config file, holds an unsupported value. | Use `text` or `json`. |
| `error: invalid pace "<value>": want slow, medium, fast or turbo` | `--pace`, or the `pace` key in the config file, holds an unsupported value. | Use one of the four names. |
| `error: invalid wait <value>: want a duration of zero or more` | `--wait` was given a negative duration. | Pass zero or a positive duration, such as `30s`. |
| `error: <port> is in use by another radiocli: <detail>` | Another command is using the scanner and had not finished. | Wait for the command named in the message, or pass `--wait` to queue behind it. |
| `error: every scanner found is in use by another radiocli: <ports>` | Discovery found only ports another command was holding. | Wait for it to finish and run the command again. |
| `error: reading <path>: open <path>: no such file or directory` | The file named by `--config` does not exist. | Correct the path, or omit `--config` to use the default location. |
| `error: parsing <path>: <detail>` | The config file is not valid JSON. | Fix the file, or delete it to start from the defaults. |
| `error: unknown command "<name>" for "radiocli"` | The command does not exist, or you passed an argument to a command that takes none. | Run `radiocli --help` for the command list. |

## See also

- [`config`](commands/config.md) — report and change the settings this tool keeps for itself, which are the same ones the global flags override for one run.
- [`devices`](commands/devices.md) — list the attached scanners and their ports.
- [`daemon`](commands/daemon.md) — hold a scanner so that other commands queue for it instead of being refused.
- [`status`](commands/status.md) — confirm the named scanner is responding, and report what it is doing.
- [`backlight`](commands/backlight.md) — report whether the scanner is lit, switch it on or off, and choose whether the keypad lights with it.
- [`battery`](commands/battery.md) — report the scanner's battery charge.
- [`backup`](commands/backup.md) — copy the scanner's memory card to this computer. The only command that does not use the serial port, and the only one that works while the scanner is in mass storage mode.
- [`colors`](commands/colors.md) — report the text and background color of every area of a screen layout, where each area sits on the screen, and change either color.
- [`display`](commands/display.md) — report whether the scanner draws its screen in color, and switch it between color and black and white.
- [`channels`](commands/channels.md) — list the channels in a department, with the frequency or talkgroup each receives.
- [`clock`](commands/clock.md) — report or correct the scanner's date and time.
- [`location`](commands/location.md) — report the position the scanner is working from, set it from a zip code or a latitude and longitude, hand it back to the GPS, or take it away again.
- [`favorites`](commands/favorites.md) — list the scanner's favorites lists, and choose which are scanned.
- [`systems`](commands/systems.md) — list the systems inside one favorites list.
- [`departments`](commands/departments.md) — list the departments inside one system.
- [`sites`](commands/sites.md) — list the sites of a trunked system, and the pool of frequencies each holds.
- [`menu`](commands/menu.md) — report the menu the scanner is showing, and move around it.
- [`scan`](commands/scan.md) — leave the menus and return the scanner to scanning.
- [`scanning`](commands/scanning.md) — report what the scanner is working through right now.
- [`screen`](commands/screen.md) — report the scanner's display as text.
- [`key`](commands/key.md) — press keys on the scanner's front panel.
- [`tune`](commands/tune.md) — tune the scanner to one frequency and hold it.
- [`volume`](commands/volume.md) — report or change the scanner's volume level.
- [`version`](commands/version.md) — report what the binary was built from.
