# audio

Lists the sound inputs your computer can record from. Run it to find out which
input a scanner's audio cable is plugged into, and what that input is called.

## Overview

`audio` asks your computer what it can record sound from and lists what it
finds: line inputs, microphones, and any virtual inputs that other applications
provide. The scanner's own audio does not travel over the USB cable that
`radiocli` uses to control it. It leaves the scanner as an ordinary sound
signal from the headphone or record socket, so it only reaches your computer if
you have run a cable into one of these inputs, and no command can work out which
one you used. This command changes nothing. It does not open any input, which is
why it never causes your computer to ask for permission to use the microphone,
and it does not touch the scanner at all, so it works with no scanner attached
and while another `radiocli` is busy with one. Finding nothing is a complete
answer rather than a failure, so the command still succeeds and prints advice on
what to check. To record from one of these inputs rather than merely list them,
run [`audio listen`](#audio-listen).

## Usage

```
radiocli audio [flags]
radiocli audio listen [flags]
```

## Parameters

`audio` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | — | — | This command takes no flags and no arguments. |

### Global flags that change this command

- `-o`, `--output` selects whether the list is printed as a table or as JSON.
  Both contain the same inputs.
- `-v`, `--verbose` adds one debug line to stderr before the list.

The remaining global flags have no effect here. `--device`, `--config`,
`--pace`, and `--wait` all concern the scanner, and this command never talks to
it, so passing them changes nothing about the list.

## Examples

Listing the sound inputs on a computer with a USB audio interface attached:

```
$ radiocli audio
NAME
Cubilux CB5 Line In
MacBook Pro Microphone
Microsoft Teams Audio
```

The same list as JSON, which is the form to read from a script because it has no
header row and quotes names that contain spaces:

```
$ radiocli audio -o json
[
  {
    "name": "Cubilux CB5 Line In"
  },
  {
    "name": "MacBook Pro Microphone"
  },
  {
    "name": "Microsoft Teams Audio"
  }
]
```

Piping the table to another program. Everything on stdout is the header and the
rows, so nothing has to be filtered out:

```
$ radiocli audio 2>/dev/null | cat
NAME
Cubilux CB5 Line In
MacBook Pro Microphone
Microsoft Teams Audio
```

Confirming that the command never reaches for the scanner. The port named here
does not exist, and the list is produced anyway:

```
$ radiocli --device /dev/nonexistent audio
NAME
Cubilux CB5 Line In
MacBook Pro Microphone
Microsoft Teams Audio
```

Watching what the command does before it prints:

```
$ radiocli -v audio
time=2026-08-09T13:29:47.730-04:00 level=DEBUG msg="listing the sound inputs"
NAME
Cubilux CB5 Line In
MacBook Pro Microphone
Microsoft Teams Audio
```

## Output

The list goes to stdout. The advice printed when nothing is found goes to
stderr. Nothing else is written to stderr, so under `--output text` a successful
run puts the header and the rows on stdout and nothing anywhere else.

Inputs are listed in alphabetical order by name, ignoring case. That is this
tool's order and not your computer's: the order your computer reports changes as
devices are plugged and unplugged, and sorting keeps two runs comparable.

Under `--output text`, stdout holds a header row and one row per input:

| Column | Description |
| ------ | ----------- |
| `NAME` | What your computer calls the input, such as `Cubilux CB5 Line In`. |

Under `--output json`, stdout holds an array with one object per input. The
array is empty when nothing is found:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | What your computer calls the input. |

An input is described by its name and nothing else. Your computer has its own
internal identifier for each one, and `radiocli` does not report it, because it
is far too long to type and it is less stable than the name: on macOS it records
which USB socket the device was plugged into, so moving an interface to a
different socket changes it, and on Linux it can be a position in a list that
shifts when an unrelated sound card is unplugged. A name survives both.

A name is not guaranteed to be unique. Two identical audio interfaces report the
same name, and they appear as two rows that read the same. There is nothing in
this list that tells them apart.

Your computer treats one of its inputs as the one to hand to programs that do
not ask for a particular one. That input is not marked here and is not treated
differently, because on a laptop it is almost always the built-in microphone,
which is not what a scanner is plugged into.

Finding no inputs prints `[]` under `--output json`, and under `--output text`
prints nothing to stdout, while this goes to stderr:

```
No sound inputs found. Connect a sound card or an audio interface, or check that
this computer's own microphone is not switched off in its sound settings.
```

Both exit with status `0`. A valid JSON document is written to stdout whatever
was found, so `--output json` never produces empty output.

## Errors

`audio` has almost no failures of its own. Finding no inputs is not an error:
the command prints advice to stderr and exits with status `0`.

| Error | Meaning | Fix |
| ----- | ------- | --- |
| `error: opening the audio system: ...` | Your computer's sound system could not be started, so nothing could be asked about. The text after the colon comes from the operating system. | This is a fault on the computer rather than in the command. On Linux, check that a sound server is running. Otherwise restart the computer and try again. |
| `error: listing the audio inputs: ...` | The sound system started but refused to say what it could record from. The text after the colon comes from the operating system. | As above. |
| `error: invalid output format "yaml": want "text" or "json"` | `--output` was given something other than `text` or `json`. | Pass `-o text` or `-o json`. |

All three exit with status `1`.

This command never asks your operating system for permission to use the
microphone, because listing the inputs does not require it. `audio listen` does,
because opening an input requires it.

---

# audio listen

Writes the scanner's audio to standard output, as it arrives, until you stop it.

## Overview

`audio listen` records from a sound input and writes what it hears to stdout, so
it can be piped straight into a player or into a file. By default it writes raw
samples: signed 16-bit little-endian mono at 48000 Hz, with no header and no
framing at all, which is what a player expects to be handed on a pipe. The exact
flags to give one are printed to stderr when the stream starts, so they are never
something to work out.

**The audio comes from a daemon, not from this command.** That is the opposite of
every other command in the tool, which tries the scanner itself first and falls
back to a daemon only when refused. Audio has no such fallback to arrange: a
sound input can only be open once, so if a daemon holds it then opening it here
would fail, and if no daemon holds it then there is nothing to share. Rather than
produce a worse error by trying, this says plainly that a daemon is what serves
audio. Start one with [`daemon --audio`](daemon.md).

`--input` is the exception, and it is there for checking a cable rather than for
ordinary use. It opens the named input directly, without a daemon and without a
scanner, which is the quickest way to find out whether the lead is in the right
socket. Only one thing can hold an input, so nothing else can listen while it
does.

## Usage

```
radiocli audio listen [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--format` | No | `pcm` | How to write the audio: `pcm` or `opus`. |
| `--bitrate` | No | `32000` | Bits per second, for `--format opus`. |
| `--input` | No | none | Open this sound input directly instead of asking a daemon. |
| `--channel` | No | `auto` | Which side of the cable the scanner is on, for `--input`. |

### `--format`

`pcm` writes the samples themselves, with nothing around them: signed 16-bit
little-endian mono at 48000 Hz. There is no header, so nothing in the stream says
what it is, which is why the format line is printed to stderr.

`opus` writes compressed packets, each preceded by its length as two bytes, most
significant first. It is 20 ms per packet at 48000 Hz mono. **This is for a
program to read, not a player.** There is no container around the packets, so
nothing that plays files will open it; wrapping them in Ogg is a job for whatever
reads them.

### `--bitrate`

Bits per second for `--format opus`, between 6000 and 510000.

The useful band is roughly 32000 to 48000 for a voice channel. The encoder is
CELT only, which is the half of Opus meant for music and low latency, and it does
worse on speech at low rates than the hybrid mode a fuller encoder would pick. It
is still better than the 64000 that telephone-grade audio would cost.

### `--input`

Opens the named sound input directly, rather than asking a daemon for audio.

For checking that the cable is in the right socket, and for hearing an input on a
computer with no scanner attached at all. `--device` is not needed and is not
used.

Because it opens the input itself, nothing else can listen at the same time,
including a daemon. This is the flag that raises the microphone permission prompt
on macOS.

### `--channel`

Which side of the cable the scanner is on, for `--input`. It is described under
[`daemon --audio-channel`](daemon.md), and means the same thing here.

It has no effect without `--input`. With a daemon the fold is the daemon's
decision, made once for every listener, because which channel the scanner landed
on is a fact about the cable rather than about who is listening.

### Global flags that change this command

- `--device` names the scanner whose daemon to ask for audio. Required unless
  `--input` is given.
- `-v`, `--verbose` logs the level meter and the daemon's own messages.

`--output` and `--pace` have no effect. The audio is bytes rather than a
document, and nothing here talks to the scanner.

## Examples

Playing the scanner's audio, with a daemon holding the input:

```
$ radiocli audio listen --device /dev/cu.usbmodem00000000000011 | ffplay -f s16le -ar 48000 -ac 1 -i -
Recording from "Cubilux CB5 Line In": signed 16-bit little-endian mono at 48000 Hz.
Play it with:  ffplay -f s16le -ar 48000 -ac 1 -i -
           or: play -t raw -e signed -b 16 -c 1 -r 48000 -
The scanner's audio is on the left channel.
```

The same audio to a second listener at the same time, compressed. One daemon,
one open sound input, two listeners:

```
$ radiocli audio listen --device /dev/cu.usbmodem00000000000011 --format opus > /tmp/scanner.opus
```

Checking a cable with no daemon and no scanner:

```
$ radiocli audio listen --input "Cubilux CB5 Line In" | ffplay -f s16le -ar 48000 -ac 1 -i -
```

Recording to a file, which is raw samples and needs the format given when it is
read back:

```
$ radiocli audio listen --device /dev/cu.usbmodem00000000000011 > /tmp/scanner.raw
$ ffmpeg -f s16le -ar 48000 -ac 1 -i /tmp/scanner.raw /tmp/scanner.wav
```

Without a daemon:

```
$ radiocli audio listen --device /dev/cu.usbmodem00000000000011
error: no radiocli daemon is running for this scanner.
Audio comes from a daemon, because a sound input can only be open once and
sharing it is what the daemon is for. Start one with:
  radiocli daemon --device /dev/cu.usbmodem00000000000011 --audio "<sound input>"
Or pass --input to open a sound input directly, without sharing it
```

## Output

The audio goes to stdout and nothing else ever does. Everything else, including
the format line, goes to stderr, so a pipe carries audio alone.

Stopping the command with Ctrl-C is how it ends and is not a failure: it exits
with status `0`. So does the player at the other end of the pipe being closed,
which is what happens when you quit it.

The stream is written on as it arrives, unbuffered. A buffer would trade latency
for fewer writes at fifty writes a second, which is a poor trade here: the value
of this is hearing the radio at the moment it says something.

## Errors

| Error | Meaning | Fix |
| ----- | ------- | --- |
| `error: no radiocli daemon is running for this scanner. ...` | Nothing is holding this scanner's sound input. | Start a daemon with `--audio`, or pass `--input` to open one directly. |
| `error: this daemon is not holding a sound input, ...` | A daemon is running, but it was started without `--audio`. | Stop it and start it again with `--audio`. |
| `error: no sound input by that name: "..."` | `--input`, or the daemon's `--audio`, named something not attached. | Run `radiocli audio` to see the names. |
| `error: more than one sound input by that name: ...` | Two identical interfaces report one name and cannot be told apart. | Unplug one, or rename it in your computer's sound settings. |
| `error: "audio listen" runs until it is stopped, so it cannot be run inside a daemon` | The command was sent to a daemon to run, rather than run in a terminal. | Run it in a terminal of its own. A daemon lends a command its output for the length of the command, which only works because commands end. |
| `error: there is no audio format called "..."` | `--format` was given something other than `pcm` or `opus`. | Pass `--format pcm` or `--format opus`. |
| `error: a bitrate of N is outside what the encoder accepts` | `--bitrate` was outside 6000 to 510000. | Pass something in range; 32000 is a good starting point. |
| `error: this copy of radiocli was built without audio support` | The build has cgo switched off, so it cannot open a sound input. | Rebuild with `CGO_ENABLED=1`. Only opening an input needs this; a build like that can still receive audio from a daemon. |

If a stream opens and produces nothing but digital silence for several seconds,
this is printed to stderr:

```
There has been no signal at all for several seconds, not even noise.
Check the cable, and on macOS check that this tool is allowed to record:
System Settings > Privacy & Security > Microphone.
```

It is worth saying because it is the one failure that looks like success. A real
sound input has a noise floor a few counts wide even with nothing plugged into
it, so perfect digital zero means something took the audio away rather than that
the radio is quiet. On macOS that is almost always the microphone permission
having been refused, which otherwise presents as a cable that does not work.
