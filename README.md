# radiocli

Control your Uniden SDS150 scanner from your computer.

The SDS150 is normally driven by its knob and keypad. `radiocli` drives the same radio over its USB cable instead, from a terminal window on macOS, Windows, or Linux, though macOS is where it has been tested so far. You can see what the scanner is doing, browse and edit your favorites lists, tune to a frequency, change settings, and back up the memory card. Everything works as plain text you can read, or as JSON a script can consume.

This tool is unofficial and not affiliated with Uniden. It was built by working the radio out from the outside, and the notes behind it live in [research/](research/). Everything was developed and tested against one SDS150 on firmware 1.00.37. The SDS100 and SDS200 speak the same protocol, so it may well work on them unchanged, but I only own an SDS150 and can't confirm it. If you try one, open an issue and tell me what happened, either way.

## Why this exists

This is a hobby project. I wanted an AI agent to be able to run my scanner, and there was no way to do that, so after thirty years of building software for a living I built one for fun. It turns out an agent needs the same things a person at a keyboard needs: commands that say what they do, check that they worked, and answer in a form that can be parsed. So the same tool serves both. You can type the commands yourself, wire them into a script, or hand the whole thing to an agent and tell it to program your scanner for you.

I write about the project at [radiocli.com](https://radiocli.com), including blog posts on how it was built and some of the cool facts and oddities the radio gave up along the way.

## Installing

You do not need to install anything else first. Download the file for your computer from the [latest release](https://github.com/albeebe/radiocli/releases/latest), and follow the steps for your system.

### Mac

1. Download `radiocli-mac.zip` and double-click it. That leaves a file named `radiocli` in your Downloads folder.
2. Open the Terminal app (press Cmd-Space, type `terminal`, press Return).
3. Type these two lines, pressing Return after each. The first one tells macOS you trust the file, since it was downloaded from the internet:

```
xattr -d com.apple.quarantine ~/Downloads/radiocli
sudo mv ~/Downloads/radiocli /usr/local/bin/
```

The second line asks for your Mac's password and puts the tool where the Terminal can always find it. From then on, typing `radiocli` in any Terminal window works.

### Windows

Heads up: I develop on a Mac, and the Windows build has not been tested against a real scanner yet. It should work, but you may be the first to find out. If something misbehaves, [open an issue](https://github.com/albeebe/radiocli/issues) and tell me about it.

1. Download `radiocli-windows.zip`, right-click it, and choose **Extract All**. That leaves a file named `radiocli.exe`.
2. Open the extracted folder, click in the address bar at the top, type `cmd`, and press Enter. A black command window opens in that folder.
3. Type `radiocli devices` and press Enter.

If Windows shows a blue "Windows protected your PC" screen the first time, click **More info** and then **Run anyway**. That screen appears because the tool is from a small independent project rather than a large company.

### Linux

Same caveat as Windows: I develop on a Mac, and the Linux build has not been tested against a real scanner yet. If it misbehaves, [open an issue](https://github.com/albeebe/radiocli/issues).

```
tar xzf radiocli-linux.tar.gz        # radiocli-linux-arm64.tar.gz on a Raspberry Pi
sudo mv radiocli /usr/local/bin/
```

If the scanner's port later refuses with a permission error, add yourself to the group that owns serial ports and log out and back in:

```
sudo usermod -a -G dialout $USER
```

### Updating later

You only have to do the above once. From then on the tool updates itself:

```
radiocli update --check     # is there a newer one?
radiocli update             # install it
```

It downloads the release built for your computer, checks it against the checksum published with it, and replaces itself. If you put it in `/usr/local/bin` you will need `sudo radiocli update`, and it will tell you so rather than asking for your password on its own. On a Mac there is no `xattr` step this way: the quarantine flag is something your browser attaches to a download, so a file the tool fetches for itself never gets one.

## Plugging in the scanner

Connect the SDS150 to the computer with its USB cable and turn it on. The scanner briefly asks which USB mode to use; press the `.` key for Serial Port, or ignore the prompt and it picks the right mode on its own. Then ask the computer what it sees:

```
radiocli devices
```

That prints the scanner and, in the `PORT` column, the name of the port to use. Every command that talks to the radio needs that port passed with `--device`, because nothing is remembered between runs. Putting it in a shell variable keeps things short:

```
SDS=/dev/cu.usbmodem00000000000011
```

One thing the cable does not carry is sound. The scanner's audio leaves through its headphone socket like any radio, so hearing it on the computer means running an audio cable into an input. The `audio` command lists the inputs available.

## First commands

```
radiocli --device $SDS status             # confirm it answers, and what it is doing
radiocli --device $SDS screen             # the display, as text
radiocli --device $SDS scanning systems   # the systems being cycled through
radiocli --device $SDS tune 162.550       # park it on one frequency
radiocli --device $SDS scan               # back to scanning
```

The commands that never need `--device` are `devices` and `audio`, which look at this computer rather than at a scanner, `version`, `config` and `update`, which are about the tool itself, `backup`, which reads the memory card instead of the serial port, and `colors palette`, which prints a built-in table.

## What it can do

The full command reference is in [documentation/commands/](documentation/commands/), and a task-oriented tour is in [documentation/examples.md](documentation/examples.md). The commands group roughly as follows.

- **See what it is doing right now**: `status`, `scanning`, `screen`, `battery`, `backlight`, `display`, `colors`, `version`.
- **Browse the memory**, which is organized as favorites lists, then systems, then departments, then channels: `favorites`, `systems`, `departments`, `sites`, `channels`, `banks`.
- **Control and tune it**: `scan`, `tune`, `weather`, `location`, `volume`, `squelch`, `beep`, `clock`.
- **Edit the memory** at every level, with `new`, `rename`, and `delete` subcommands on `favorites`, `systems`, `departments`, `sites`, and `channels`. Deletes take everything underneath with them and require `--yes`.
- **Drive it by hand**: `menu` reads and moves around the on-screen menus, and `key` presses front-panel keys directly. `key` is the blunt instrument: it presses what you ask and checks nothing, so prefer a command that names what it does.
- **Manage the tool itself**: `config`, `daemon`, `backup`, `audio`, and `update`, which replaces the tool with the newest release.

## Output for scripts and AI agents

Every listing command speaks JSON with `-o json`. This is the part that makes the tool agent-friendly: an AI agent with terminal access can discover the commands with `--help`, run them, and parse the answers, with no wrapper needed. Results go to stdout and everything else, progress, prompts and errors, goes to stderr, so redirecting stderr leaves stdout holding only the answer.

```
radiocli --device $SDS scanning systems -o json
radiocli --device $SDS favorites -o json | jq -r '.[] | select(.monitored) | .name'
```

Flags that work on every command, along with the config file and its location per operating system, are documented in [documentation/global_flags.md](documentation/global_flags.md).

## Good to know

**One scanner, one command at a time.** The scanner answers on one line with nothing identifying who asked, so two programs sharing the port would read each other's replies. `radiocli` therefore claims the port for the length of a command, and a second command started meanwhile is refused rather than allowed to talk over the first. Pass `--wait` to queue, or run [`daemon`](documentation/commands/daemon.md) to let commands share one held scanner.

**One request is refused on purpose.** Asking for the systems of "Full Database" returns a short, wrong answer and then locks the scanner up until it is power cycled, so the tool refuses to send it and says so. Use [`scanning systems`](documentation/commands/scanning.md) to read what is being scanned instead.

## Building from source

The prebuilt downloads above are all most people need. To build it yourself, install [Go](https://go.dev/dl/) 1.25 or newer; the module lives in [source/](source/):

```
cd source
go build -o radiocli .
```

## Repository layout

- [source/](source/) is the Go module and all of the tool's code.
- [documentation/](documentation/) is the command reference, the global-flags guide, and worked examples.
- [contributing/](contributing/) is the standard a command is built and documented to, for anyone adding to the tool.
- [research/](research/) is the reverse-engineering: how the serial protocol, the display, the fonts, the colors, and the menu tree were worked out.
- [testing/](testing/) is the end-to-end suite, a Go module of its own that drives a real scanner.

## License and status

MIT. See [LICENSE](LICENSE). The tool is provided as is and without warranty, which is worth reading before pointing it at a radio you care about.

Unofficial and not affiliated with Uniden. Developed and verified against a single SDS150 on firmware 1.00.37, from a Mac. The Windows and Linux builds compile and ship, but they have not been run against a real scanner yet, and the related SDS100 and SDS200 have not been tried at all. The palette, the font, and the protocol are presumed to be the firmware's rather than one unit's, but that has not been tested against a second radio. If you run any of the untested combinations, open an issue and say how it went; that is how this list shrinks.
