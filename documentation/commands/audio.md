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

---

# audio record

Writes one file per transmission, with a description of what it is beside it,
until you stop it. Run it when you want to keep what the scanner hears rather
than only listen to it.

## Overview

`audio record` listens to the scanner and writes a separate WAV file every time
it hears a transmission, together with a JSON file of the same name describing
what that transmission was: the system, the department, the channel, and the
frequency or talkgroup behind it. Every recording is also appended as one line
to `index.jsonl` at the top of the destination, which is what makes a night's
recordings searchable with ordinary tools rather than something to scroll
through. It records a scanner rather than a sound card, so `--device` is
required as well as the audio: naming the radio is what lets every file be
labelled, and it is what lets the command check that the input really is the
scanner rather than a microphone picking up the room. Nothing on the scanner is
changed and no key is pressed; everything written goes under the destination
folder, which is created if it is not there. It runs until you stop it with
Ctrl-C, and a transmission in progress at that moment is finished properly
rather than lost.

**Where a recording starts is found in the audio, not taken from the radio.**
The scanner tells this command what it is receiving over the USB cable, and that
news always arrives a little after the sound itself, which is why other software
either clips the start of every transmission or pads every file with silence to
avoid it. This command keeps the last ten seconds of audio buffered and decides
nothing in real time, so when the radio says it is receiving, the audio from
before that is still there to look at. The recording then begins where the sound
actually rose out of the noise, and ends where it fell back into it.

## Usage

```
radiocli audio record [destination] [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `[destination]` | No | `recordings` | The folder to write recordings into, created if it does not exist. |
| `--input` | No | none | Sound input to open directly. Without it the audio comes from a daemon. |
| `--channel` | No | `auto` | Which side of the cable the scanner is on: `auto`, `left`, `right` or `mix`. |
| `--template` | No | `{date}/{time}_{system}_{department}_{channel}` | How each recording is named, below the destination. |
| `--hang` | No | `2s` | How long the audio must stay quiet before a transmission is called finished. |
| `--min-duration` | No | `1s` | Discard any transmission shorter than this. |
| `--max-duration` | No | `5m` | Split any transmission longer than this. |

### `[destination]`

The folder recordings are written into, created along with any parent folders if
it is not already there. Leaving it off writes into a folder called `recordings`
below the current directory.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner
```

### `--input`

Names a sound input to open directly, as [`audio`](#audio) lists them. Without
it, the audio comes from a `radiocli daemon` that was started with `--audio`,
which is the way that lets several things hear the scanner at once. A sound
input can only be open once, so passing `--input` takes it for this process
alone and nothing else can listen while the recording runs.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --input "USB Audio CODEC"
```

### `--channel`

Which side of the audio cable carries the scanner. `auto` listens to both for a
few seconds and decides, `left` and `right` take one side and ignore the other,
and `mix` averages the two. `auto` is right unless you know the cable. This has
no effect when the audio comes from a daemon, because the daemon has already
folded it.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --input "USB Audio CODEC" --channel left
```

### `--template`

Sets the path each recording is written to, relative to the destination and
without an extension. A `/` in the template creates a folder. The description
file always takes the same name with `.json` instead of `.wav`, whatever the
template says, so the two always travel together.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --template "{date}/{channel}/{time}"
```

The tokens are:

| Token | Example | Description |
| ----- | ------- | ----------- |
| `{date}` | `2026-08-22` | The date the transmission started, in the computer's local time. |
| `{time}` | `19-54-03` | The time it started, 24-hour, with hyphens because `:` is not allowed in a filename on Windows. |
| `{datetime}` | `2026-08-22T19-54-03` | The date and time joined. |
| `{epoch}` | `1787793243` | The start time as whole seconds since 1970. |
| `{list}` | `Pocahontas-County` | The favorites list or database the channel was found in. |
| `{system}` | `PUBLIC-SAFETY` | The system the channel belongs to. |
| `{department}` | `POLICE-DEPARTMENT` | The department the channel belongs to. |
| `{site}` | `Bald-Knob` | The trunked site. Empty on a conventional system. |
| `{channel}` | `MARLINTON-DISPATCH` | The channel's alpha tag. |
| `{frequency}` | `155.550000MHz` | What the scanner was tuned to on a conventional system. Empty on a trunked one. |
| `{talkgroup}` | `24944` | The talkgroup number on a trunked system. Empty on a conventional one. |
| `{tuned}` | `24944` | Whichever of `{talkgroup}` and `{frequency}` applies. |
| `{unit}` | `32` | The radio heard transmitting, when the scanner decoded one. |
| `{modulation}` | `NFM` | How the scanner was demodulating. |
| `{duration}` | `4.8` | How long the recording is, in seconds, to one decimal place. |

Six rules apply to every template:

- **An unknown token is refused when the command starts**, before anything is
  opened, and the message lists every token there is. A typo costs a second
  rather than a night's recording.
- **A token the scanner did not fill in disappears, along with the separator
  next to it.** An unlabelled recording is named `19-54-03.wav`, not
  `19-54-03___.wav`.
- **Every value is reduced to letters, digits, dot, dash and underscore.**
  A channel called `FIRE/EMS` becomes `FIRE-EMS` and cannot create a folder.
- **`{{` and `}}` are a literal `{` and `}`.**
- **Two recordings that would get the same name are numbered**, `-2`, `-3` and
  so on. Nothing is overwritten.
- **The path is shortened if it would be too long**, by trimming the longest
  part of it first, so it stays under the limits Windows enforces.

A template with no tokens in it is refused, because every recording would be
given the same name.

### `--hang`

How long the audio must stay quiet, with the scanner no longer receiving, before
a transmission is treated as finished. It is what stops a speaker drawing breath
mid-sentence from becoming two recordings. Raise it if single transmissions are
arriving as several files; lower it if separate transmissions are being joined.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --hang 4s
```

### `--min-duration`

The shortest recording worth keeping. A transmission shorter than this is
discarded before any file is created, so nothing has to be cleaned up
afterwards. It exists because a squelch tail or a control channel burst is not
something anybody wants to listen to.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --min-duration 2s
```

### `--max-duration`

How long one recording may run before it is split and a new one started. The
audio carries on across the split with nothing lost at the join. It exists so a
stuck microphone cannot produce one enormous file.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --max-duration 2m
```

### Global flags that change this command

- `--device` is **required**, and names the scanner. Get the value from the
  `port` column of [`devices`](devices.md). Without it the command refuses to
  start, because it would have nothing to label recordings with and no way to
  check the input against the radio.
- `-o`, `--output` selects whether each finished recording is printed as a line
  or as a JSON object.
- `-v`, `--verbose` prints a level reading to stderr every two seconds, which is
  what diagnoses a cable:

  ```
  time=2026-08-22T22:47:07.448-04:00 level=DEBUG msg=audio peak=-74.7 floor=-79
  ```

  `floor` is where the recorder has measured the noise on the input, and `peak`
  is the loudest audio in the last two seconds, both in dBFS. A transmission
  reads about 40 dB above the floor. A `peak` that never rises more than a few
  decibels above `floor` means nothing is arriving: check the cable and the
  scanner's volume. A `floor` pinned at `-30` means the input is not a squelched
  scanner at all.

`--pace` has no effect here. This command presses no keys.

## Examples

Recording with the audio cable plugged straight into this computer:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --input "USB Audio CODEC"
Recording from "USB Audio CODEC" into /Users/you/scanner
One file per transmission, with a description beside it. Press Ctrl-C to stop.
19:54:03    4.8s  PUBLIC SAFETY POLICE DEPARTMENT MARLINTON DISPATCH
19:54:31    2.1s  PUBLIC SAFETY FIRE RESCUE FIREGROUND 2
```

Each line is one finished transmission: when it started, how long it is, and
what it was.

Recording while a daemon holds both the scanner and its audio, so other commands
can still use the radio:

```
$ radiocli daemon --device /dev/cu.usbmodem00000000000011 --audio "USB Audio CODEC" &
$ radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner
Recording from "USB Audio CODEC" into /Users/you/scanner
One file per transmission, with a description beside it. Press Ctrl-C to stop.
```

The readings that label each recording go through the daemon without taking a
turn, so they never make anything else wait. See [`daemon`](daemon.md).

Printing each transmission as JSON, which is the form to read from a script or
an agent:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner -o json
{
  "file": "2026-08-22/19-54-03_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav",
  "start": "2026-08-22T19:54:03.390720-04:00",
  "end": "2026-08-22T19:54:08.190720-04:00",
  "duration": 4.8,
  "list": "Pocahontas County",
  "system": "PUBLIC SAFETY",
  "department": "POLICE DEPARTMENT",
  "channel": "MARLINTON DISPATCH",
  "frequency": "155.550000MHz",
  "modulation": "NFM",
  "reason": "hang",
  "samples": 14
}
```

Finding every recording of one channel afterwards:

```
$ jq -r 'select(.channel == "MARLINTON DISPATCH") | .file' ~/scanner/index.jsonl
2026-08-22/19-54-03_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav
2026-08-22/20-11-47_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav
```

Finding everything over ten seconds long from one evening:

```
$ jq -r 'select(.duration > 10 and (.start | startswith("2026-08-22T20"))) | "\(.duration)s \(.channel)"' ~/scanner/index.jsonl
14.2s FIREGROUND 2
```

## Output

Recordings go to the destination folder. One line per finished transmission goes
to stdout. Everything else, including the opening message and any warning about
the input, goes to stderr, so `2>/dev/null` leaves stdout holding only the
transmissions.

The destination looks like this:

```
scanner/
  index.jsonl
  2026-08-22/
    19-54-03_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav
    19-54-03_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.json
```

Every WAV is signed 16-bit little-endian mono at 48000 Hz, which any player
opens. There is no compressed option: a recording is an archive, and WAV has no
codec in it to fall out of date.

`index.jsonl` holds one JSON object per line, one per recording, appended as each
finishes. Each line is the same object as the `.json` file beside the audio, and
the same object printed by `--output json`, so there is one shape to learn.

| Field | Type | Description |
| ----- | ---- | ----------- |
| `file` | string | Where the audio is, relative to the destination. Always present. |
| `start` | string | When the audio began, RFC 3339 with the computer's offset. Always present. |
| `end` | string | When the audio ended. Always present. |
| `duration` | number | How long the audio is, in seconds, measured from the file rather than from the clock. Always present. |
| `list` | string | The favorites list or database the channel was found in. |
| `system` | string | The system the channel belongs to. |
| `department` | string | The department the channel belongs to. |
| `site` | string | The trunked site. Absent on a conventional system. |
| `channel` | string | The channel's alpha tag. |
| `frequency` | string | What the scanner was tuned to on a conventional system, carrying its unit. Absent on a trunked system. |
| `talkgroup` | string | The talkgroup number on a trunked system. Absent on a conventional system. |
| `unit` | string | The radio heard transmitting, when the scanner decoded one. |
| `modulation` | string | How the scanner was demodulating, such as `NFM`. |
| `reason` | string | Why the recording ended: `hang` when the channel went quiet, `split` when it reached `--max-duration`, `channel` when the scanner moved to another channel, `stopped` when you stopped the command. |
| `samples` | number | How many times the scanner was asked what it was hearing while this was being recorded. Always present. |
| `channels` | array of strings | Every distinct channel seen during the recording. Present only when there was more than one. |
| `dropped` | number | Frames of audio the sound card produced that never arrived. Present only when some were lost. Each frame is 20 ms. |

**`samples` is how much the label is worth.** A transmission of any ordinary
length is covered many times over, because the scanner is asked three times a
second. A `samples` of `0` means the transmission was over before the scanner
could be asked once, and the fields naming a channel are left empty rather than
guessed at.

**A recording is never labelled from a guess.** If the scanner never named a
channel, the channel fields are absent from the JSON and the recording is named
from its timestamp alone.

## Errors

| Error | Meaning | Fix |
| ----- | ------- | --- |
| `error: no scanner named: "audio record" needs the scanner as well as its audio, ...` | `--device` was not given. | Pass `--device` with the port from [`devices`](devices.md). |
| `error: invalid naming template: "..." is not a token: the tokens are ...` | `--template` used a token that does not exist. | Use one of the tokens listed in the message. |
| `error: invalid naming template: "..." has a "{" that is never closed` | A brace in `--template` was left open. | Close it, or write `{{` for a literal brace. |
| `error: invalid naming template: "..." has no tokens in it, so every recording would be given the same name` | `--template` was plain text. | Add at least one token, such as `{time}`. |
| `error: no radiocli daemon is running for this scanner. ...` | No `--input` was given and nothing is holding this scanner's sound input. | Start a daemon with `--audio`, or pass `--input`. |
| `error: no sound input by that name: "..."` | `--input` named something not attached. | Run [`radiocli audio`](#audio) to see the names. |
| `error: "audio record" runs until it is stopped, so it cannot be run inside a daemon` | The command was sent to a daemon to run, rather than run in a terminal. | Run it in a terminal of its own. |
| `error: the scanner stopped answering, so recording has stopped: ...` | The scanner was unplugged or switched off while recording. | Reconnect it and start again. The transmission in progress is written out before the command exits. |
| `error: <port> is in use by another radiocli` | Another invocation has the scanner and no daemon is sharing it. | Wait for it, or start a [`daemon`](daemon.md). |

Two warnings are printed to stderr rather than stopping the recording, because
each means the audio input is probably not the scanner. Both wait until the
disagreement has lasted half a minute, since the radio and the sound card never
agree to the millisecond at the edges of a transmission:

```
The scanner says it is receiving and nothing is arriving on the audio input.
Check the cable is in the scanner's headphone or record socket and in the input
you named, and check the scanner's volume is not at zero.
```

```
Sound is arriving while the scanner says it is receiving nothing.
The input named is probably not the scanner. Run "radiocli audio" to see what
else this computer can record from.
```

If an earlier run was killed part way through a transmission, this is printed
when the next one starts:

```
1 recording(s) in /Users/you/scanner were left unfinished by an earlier run and will not play. They are the hidden files beginning ".partial-", and can be deleted.
```

They are reported rather than deleted. A recording interrupted this way has an
incomplete header, so nothing will play it, but deleting somebody's files
without being asked is not this command's decision.
