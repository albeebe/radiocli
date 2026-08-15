# radiocli

A command-line tool for controlling and inspecting a Uniden SDS150 scanner over its USB serial port. The binary is called `radiocli`.

The SDS150 is a handheld radio scanner: it sweeps a large set of frequencies and stops on the ones that are active, trunk-tracking Motorola, P25, DMR and NXDN systems and filtering a nationwide database down to wherever you are. Everything it can do is normally driven by a knob and a keypad. `radiocli` drives the same radio from a terminal instead, so a person or a script can read what the scanner is doing, browse and edit its memory, tune it, and change its settings, with output as either human-readable text or JSON.

The tool is unofficial and not affiliated with Uniden. It was built by working the radio out from the outside, and the notes behind it live in [research/](research/). Everything here was developed and tested against one SDS150 on firmware 1.00.37.

## Why it exists

The SDS150 exposes two low-level primitives over its serial port: a command to press any front-panel key, including the rotary knob, and a command to read the whole screen back as text plus per-character attributes. Between them they are a complete remote-control substrate. Anything the operator can do, a program can do, and anything the screen shows, a program can read. `radiocli` is built on that pair, wrapping the raw key-pressing and screen-reading in commands that name what they do and check that they worked. How the protocol and the display were reverse-engineered is written up in [research/](research/).

## Requirements

- Go 1.25 or newer to build.
- An SDS150 connected by USB, in serial-port mode rather than mass-storage mode.
- A supported operating system: macOS, Linux, or Windows.

The scanner's audio does not travel over the USB control cable. It leaves the radio as an ordinary sound signal from the headphone or record socket, so hearing it on the computer means running a second cable into an audio input. The `audio` command lists the inputs available.

## Building

The Go module lives in [source/](source/). Build the binary from there:

```
cd source
go build -o radiocli .
```

That produces a `radiocli` binary in `source/`. Copy it onto your `PATH`, or run it in place as `./radiocli`.

## Quick start

Find the attached scanners and the serial port to address:

```
radiocli devices
```

Every command that talks to the radio needs to be told which port to use, with `--device`, because nothing is remembered between runs. Putting the port in a shell variable keeps the examples short:

```
SDS=/dev/cu.usbmodem00000000000011

radiocli --device $SDS status        # confirm it answers, and what it is doing
radiocli --device $SDS screen        # the display, as text
radiocli --device $SDS scanning systems   # the systems being cycled through
radiocli --device $SDS tune 162.550       # park it on one frequency
radiocli --device $SDS scan               # back to scanning
```

The commands that never need `--device` are `devices` and `audio`, which look at this computer rather than at a scanner, `version` and `config`, which are about the tool itself, `backup`, which reads the memory card instead of the serial port, and `colors palette`, which prints a built-in table.

## What it can do

The full command reference is in [documentation/commands/](documentation/commands/), and a task-oriented tour is in [documentation/examples.md](documentation/examples.md). The commands group roughly as follows.

Inspect what the scanner is doing right now:

- `status`, `scanning`, `screen` report the current state, the channels being worked through, and the raw display.
- `battery`, `backlight`, `display`, `colors`, `version`, `devices` report hardware and configuration.

Browse the scanner's memory, which is organized as favorites lists, then systems, then departments, then channels, with sites holding a trunked system's shared frequencies:

- `favorites`, `systems`, `departments`, `sites`, `channels`, `banks`.

Control and tune the radio:

- `scan`, `tune`, `weather`, `location`, `volume`, `squelch`, `beep`, `clock`.

Edit the memory, at every level, with `new`, `rename` and `delete` subcommands on `favorites`, `systems`, `departments`, `sites` and `channels`. Deletes take everything underneath with them and require `--yes`.

Drive the radio by hand or read it at a low level:

- `menu` reports and moves around the on-screen menus.
- `key` presses front-panel keys directly. It is the blunt instrument: it presses what you ask and checks nothing, so prefer a command that names what it does, and use `--pace slow` when stepping through by hand.

Manage the tool itself:

- `config` reports and changes the settings the tool keeps.
- `daemon` holds one scanner so that other commands queue for it rather than being refused.
- `backup` copies the scanner's memory card to the computer.
- `audio` lists the sound inputs the scanner's audio cable might be plugged into.

## Output for scripts

Every listing command speaks JSON with `-o json`. Results go to stdout and everything else, progress, prompts and errors, goes to stderr, so redirecting stderr leaves stdout holding only the answer.

```
radiocli --device $SDS scanning systems -o json
radiocli --device $SDS favorites -o json | jq -r '.[] | select(.monitored) | .name'
```

## Global flags and configuration

Flags that work on every command, along with the config file and its default location per operating system, are documented in [documentation/global_flags.md](documentation/global_flags.md). In short: `--device` chooses the scanner, `-o`/`--output` chooses `text` or `json`, `--pace` sets how fast keys are sent, `--wait` queues behind another running command, and `-v`/`--verbose` prints the raw serial exchange.

## One scanner at a time

Only one `radiocli` talks to a given scanner at a time. A command claims the port before opening it and holds the claim until it exits, so a second command started meanwhile is refused rather than allowed to talk over the first. The reason is in the hardware: the scanner answers on one line with nothing identifying who asked, so two programs sharing a port read each other's replies. Pass `--wait` to queue, or run [`daemon`](documentation/commands/daemon.md) to let commands share one held scanner.

## A command that refuses to run

```
$ radiocli systems "Full Database"
error: "Full Database" is the scanner's built-in database rather than a favorites list, and asking it for its systems returns a short, wrong answer and then locks the scanner up until it is power cycled: run "radiocli scanning systems" to read what it is scanning instead
```

The lockup was reproduced twice, so the request is refused rather than merely documented. The refusal covers the reserved index as well as the name, and it lives in the layer that would send the request, which is the last place anything can be stopped. Use [`scanning systems`](documentation/commands/scanning.md) to read the database instead.

## Repository layout

- [source/](source/) is the Go module and all of the tool's code.
- [documentation/](documentation/) is the command reference, the global-flags guide, and worked examples.
- [contributing/](contributing/) is the standard a command is built and documented to, for anyone adding to the tool.
- [research/](research/) is the reverse-engineering: how the serial protocol, the display, the fonts, the colors and the menu tree were worked out, plus write-ups of the process.
- [testing/](testing/) is the end-to-end suite, a Go module of its own that drives a real scanner, plus the renderer that draws a run as it happens.

## License

MIT. See [LICENSE](LICENSE). The tool is provided as is and without warranty, which is worth reading before pointing it at a radio you care about.

## Status

Unofficial and not affiliated with Uniden. Developed and verified against a single SDS150 on firmware 1.00.37. The palette, the font and the protocol are presumed to be the firmware's rather than one unit's, but that has not been tested against a second radio or against the related SDS100 and SDS200.
