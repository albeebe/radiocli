# audio

Lists the sound inputs your computer can record from and the speakers it can
play on. Run it to find out which input a scanner's audio cable is plugged into,
what that input is called, and where the audio can be played back.

## Overview

`audio` asks your computer what it can do with sound and lists what it finds in
two tables. The first is what it can record from: line inputs, microphones, and
any virtual inputs that other applications provide. The second is where it can
play: speakers, headphones, and any virtual outputs. The scanner's own audio
does not travel over the USB cable that
`radiocli` uses to control it. It leaves the scanner as an ordinary sound
signal from the headphone or record socket, so it only reaches your computer if
you have run a cable into one of these inputs, and no command can work out which
one you used. This command changes nothing. It does not open any input, which is
why it never causes your computer to ask for permission to use the microphone,
and it does not touch the scanner at all, so it works with no scanner attached
and while another `radiocli` is busy with one. Finding nothing is a complete
answer rather than a failure, so the command still succeeds and prints advice on
what to check, and each half is reported on its own because a computer can
perfectly well have one and not the other.

To use one of these rather than merely list them, run
[`audio listen`](#audio-listen) to hear the scanner on the speakers,
[`audio output`](#audio-output) to send its audio to another program, or
[`audio record`](#audio-record) to write its transmissions to files.

## Usage

```
radiocli audio [flags]
radiocli audio listen [flags]
radiocli audio output [flags]
radiocli audio record [destination] [flags]
```

## Parameters

`audio` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | — | — | This command takes no flags and no arguments. |

### Global flags that change this command

- `-o`, `--output` selects whether the lists are printed as tables or as JSON.
  Both contain the same devices.
- `-v`, `--verbose` adds one debug line to stderr before the list.

The remaining global flags have no effect here. `--device`, `--config`,
`--pace`, and `--wait` all concern the scanner, and this command never talks to
it, so passing them changes nothing about the list.

## Examples

Listing the sound devices on a computer with a USB audio interface attached:

```
$ radiocli audio
SOUND INPUTS
Cubilux CB5 Line In
MacBook Pro Microphone
Microsoft Teams Audio

SPEAKERS
Cubilux CB5 Headphones
MacBook Pro Speakers
```

The same lists as JSON, which is the form to read from a script because it has
no header rows and quotes names that contain spaces. The two halves are named
separately, so nothing has to guess which is which:

```
$ radiocli audio -o json
{
  "inputs": [
    {
      "name": "Cubilux CB5 Line In"
    },
    {
      "name": "MacBook Pro Microphone"
    },
    {
      "name": "Microsoft Teams Audio"
    }
  ],
  "outputs": [
    {
      "name": "Cubilux CB5 Headphones"
    },
    {
      "name": "MacBook Pro Speakers"
    }
  ]
}
```

Reading one half of it from a script:

```
$ radiocli audio -o json | jq -r '.outputs[].name'
Cubilux CB5 Headphones
MacBook Pro Speakers
```

Confirming that the command never reaches for the scanner. The port named here
does not exist, and the lists are produced anyway:

```
$ radiocli --device /dev/nonexistent audio
SOUND INPUTS
Cubilux CB5 Line In
MacBook Pro Microphone
Microsoft Teams Audio

SPEAKERS
Cubilux CB5 Headphones
MacBook Pro Speakers
```

Watching what the command does before it prints:

```
$ radiocli -v audio
time=2026-08-09T13:29:47.730-04:00 level=DEBUG msg="listing the sound devices"
SOUND INPUTS
Cubilux CB5 Line In
MacBook Pro Microphone
Microsoft Teams Audio

SPEAKERS
Cubilux CB5 Headphones
MacBook Pro Speakers
```

## Output

The lists go to stdout. The advice printed when nothing is found goes to stderr.
Nothing else is written to stderr, so under `--output text` a successful run puts
the headings and the rows on stdout and nothing anywhere else.

Both halves are listed in alphabetical order by name, ignoring case. That is this
tool's order and not your computer's: the order your computer reports changes as
devices are plugged and unplugged, and sorting keeps two runs comparable.

Under `--output text`, stdout holds a `SOUND INPUTS` heading with one row per
input, then a blank line, then a `SPEAKERS` heading with one row per speaker. A
heading with nothing under it is not printed, so a computer with no speakers
shows the inputs alone:

| Column | Description |
| ------ | ----------- |
| `SOUND INPUTS` | What your computer calls each input, such as `Cubilux CB5 Line In`. |
| `SPEAKERS` | What it calls each output, such as `MacBook Pro Speakers`. |

Under `--output json`, stdout holds one object with both halves named. Each is
an array with one object per device, and an array is empty rather than absent
when that half found nothing:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `inputs` | array | Everything your computer can record from. |
| `inputs[].name` | string | What your computer calls the input. |
| `outputs` | array | Everywhere it can play. |
| `outputs[].name` | string | What your computer calls the speaker. |

A device is described by its name and nothing else. Your computer has its own
internal identifier for each one, and `radiocli` does not report it, because it
is far too long to type and it is less stable than the name: on macOS it records
which USB socket the device was plugged into, so moving an interface to a
different socket changes it, and on Linux it can be a position in a list that
shifts when an unrelated sound card is unplugged. A name survives both.

A name is not guaranteed to be unique. Two identical audio interfaces report the
same name, and they appear as two rows that read the same. There is nothing in
this list that tells them apart.

The same name can appear in both halves. A headset is an input and an output,
and an application such as Microsoft Teams often presents one of each under the
same name. They are different devices that happen to be called the same thing.

Your computer treats one of its inputs and one of its outputs as the ones to
hand to programs that do not ask for a particular one. Neither is marked here.
The default input is almost always the built-in microphone, which is not what a
scanner is plugged into, and the default output is simply where
[`audio listen`](#audio-listen) plays, since it cannot be pointed anywhere else.

Finding nothing prints an empty array for that half under `--output json`, and
under `--output text` prints no table for it, while the advice goes to stderr:

```
No sound inputs found. Connect a sound card or an audio interface, or check that
this computer's own microphone is not switched off in its sound settings.
No speakers found. Connect some, or check that this computer's own output is not
switched off in its sound settings.
```

Each half is reported separately, so a computer with an input and no speakers
hears about the speakers alone. All of it exits with status `0`. A valid JSON
document is written to stdout whatever was found, so `--output json` never
produces empty output.

## Errors

`audio` has almost no failures of its own. Finding no inputs is not an error:
the command prints advice to stderr and exits with status `0`.

| Error | Meaning | Fix |
| ----- | ------- | --- |
| `error: opening the audio system: ...` | Your computer's sound system could not be started, so nothing could be asked about. The text after the colon comes from the operating system. | This is a fault on the computer rather than in the command. On Linux, check that a sound server is running. Otherwise restart the computer and try again. |
| `error: listing the audio inputs: ...` | The sound system started but refused to say what it could record from. The text after the colon comes from the operating system. | As above. |
| `error: listing the speakers: ...` | The sound system started but refused to say what it could play on. The text after the colon comes from the operating system. | As above. |
| `error: this copy of radiocli was built without audio support, ...` | The binary was built with `CGO_ENABLED=0`, which leaves out the library that talks to sound cards. Everything that does not touch sound still works. | Rebuild with `CGO_ENABLED=1`, or use a release binary. |
| `error: invalid output format "yaml": want "text" or "json"` | `--output` was given something other than `text` or `json`. | Pass `-o text` or `-o json`. |

All of them exit with status `1`.

This command never asks your operating system for permission to use the
microphone, because listing the devices does not require it. `audio listen`,
`audio output` and `audio record` do, because opening an input requires it.

---

# audio listen

Plays the scanner on this computer's speakers, until you stop it. Run it when
you want to hear the radio at your desk rather than pipe it somewhere or keep
it.

## Overview

`audio listen` takes the scanner's audio and plays it, which is the shortest
path from a radio on a shelf to a speaker on a desk. It keeps nothing and writes
nothing.

By default it plays the input exactly as it arrives, hiss included, the way the
scanner's own speaker does with the squelch down. The hiss is information: it
says the lead is connected and carrying something, so a channel that has gone
quiet sounds different from a tool that has stopped.

Pass `--squelch` to play only the transmissions instead. The same detector that
decides where a recording begins then decides when to open the speakers, so this
and [`audio record`](#audio-record) agree about what a transmission is: if one
would write a file, the other would play it.

**The audio comes from a daemon, not from this command.** A sound input can only
be open once, and sharing it is the daemon's whole job, which is what lets this
run while something else is recording the same radio:

```
$ radiocli daemon --device /dev/cu.usbmodem00000000000011 --audio "Cubilux CB5 Line In"
$ radiocli audio record ~/scanner-recordings    # in one terminal
$ radiocli audio listen                         # in another, at the same time
```

`--input` is the exception, and it is there for checking a cable rather than for
ordinary use. It opens the named input directly, without a daemon and without a
scanner, which is the quickest way to find out whether the lead is in the right
socket and whether there is anything on it. Only one thing can hold an input, so
nothing else can listen or record while it does. To hear and record at once from
a single directly opened input, use [`audio record --listen`](#audio-record)
instead.

Forty milliseconds of audio is held in front of the speakers, so everything
plays that far behind the radio. The cushion is what absorbs everything that
can go wrong between the cable and the speakers, from a frame arriving late to
the operating system coming for audio at a bad moment. `--buffer` moves the
trade: smaller plays sooner, bigger plays more smoothly on a computer that
struggles to keep up. That matters because you are probably sitting next to the
radio,
comparing the two.

The squelch, when it is asked for, costs nothing on top of it: the speakers
open on the first frame above the noise floor, rather than waiting for the
transmission to prove itself long enough to be worth a file.

That is the one place this and `audio record` deliberately disagree. A blip too
short to be recorded is still heard, the way it would be on the scanner's own
speaker.

## Usage

```
radiocli audio listen [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--squelch` | No | `false` | Play only the transmissions. The default plays everything the input carries. |
| `--hang` | No | `2s` | How long the audio must stay quiet before the speakers close again. |
| `--gain` | No | `0` | Decibels to turn the audio up by on the way to the speakers. |
| `--buffer` | No | `250ms` | How much audio to keep between the radio and the speakers. |
| `--input` | No | none | Open this sound input directly instead of asking a daemon. |
| `--channel` | No | `auto` | Which side of the cable the scanner is on, for `--input`. |

### `--squelch`

Off by default, which plays every sample the input carries. That is what makes a
cable checkable: hiss means the lead is connected and carrying something, and
digital silence means it is not.

Pass `--squelch` and the speakers open when a transmission starts and close once
the audio has been quiet for `--hang`, which is the better setting for leaving
running for an evening.

### `--hang`

How long the audio has to stay quiet before the speakers close again. Written as
a Go duration, such as `500ms`, `2s` or `1m`.

Two seconds by default, which is longer than the half second
[`audio record`](#audio-record) uses. That command has the radio to ask and can
watch its mute close, while this one has only the audio to go on, so the quiet
has to be long enough to carry a pause in speech without cutting the speaker
off mid-sentence.

### `--gain`

Decibels added to the audio on its way to the speakers, and nowhere else. A
line input carries the scanner 15 to 25 dB quieter than a normalized recording
plays, so without this, comparing the two means riding the volume control.
Anything pushed past full scale is held there rather than allowed to wrap, so
too much gain plays as flattened speech: turn it down.

### `--buffer`

How much audio stands between the radio and the speakers, written as a Go
duration such as `250ms` or `1s`. Everything is heard this far behind the
radio, and everything that can go wrong underneath the playing has this long
to put itself right before it is audible. The default is `250ms`, which was
measured rather than guessed at: played into a virtual device and captured back
one tone at a time, a `40ms` cushion broke every tone of 120 into as many as six
pieces, while a quarter of a second broke seven. Lower it if you want the
speakers closer to live and your computer can keep up, and note that going above
`250ms` only deepens the queue rather than asking the device for more. It has to
be between `40ms` and `500ms`.

### `--input`

Opens the named sound input directly, without a daemon and without a scanner.
For checking a cable. Only one thing can hold an input at a time, so a daemon
must not already be holding this one.

### `--channel`

Which side of the stereo cable the scanner's mono audio is on: `auto`, `left`,
`right` or `mix`. Only meaningful with `--input`, since a daemon has already
made this decision for the audio it serves. `auto` measures both sides and
chooses, which is what handles a lead wired for one channel and a headphone
socket wired out of phase.

### Global flags that change this command

- `--device` names the scanner whose daemon holds the audio. Required unless
  `--input` is given.
- `-v`, `--verbose` adds debug lines, including what the speakers had to do to
  keep up.

`-o`, `--output` has no effect. This command produces sound rather than a
document, and the only thing it writes is the running commentary on stderr.

## Examples

Listening to the scanner a daemon is holding:

```
$ radiocli audio listen --device /dev/cu.usbmodem00000000000011
Playing the transmissions from "Cubilux CB5 Line In" on this computer's own speakers. Press Ctrl-C to stop.
```

Listening while recording the same radio, which is what the daemon is for:

```
$ radiocli daemon --device /dev/cu.usbmodem00000000000011 --audio "Cubilux CB5 Line In"
$ radiocli audio record ~/scanner-recordings &
$ radiocli audio listen
```

Checking a cable, with no scanner and no daemon involved. The hiss is the point:

```
$ radiocli audio listen --input "Cubilux CB5 Line In"
Playing everything the input carries from "Cubilux CB5 Line In" on this computer's own speakers. Press Ctrl-C to stop.
```

Holding the speakers open a little longer between overs:

```
$ radiocli audio listen --hang 4s
```

## Output

Nothing is written to stdout. The audio goes to the speakers, and the running
commentary goes to stderr, so this command can be left running beside anything
that reads stdout without disturbing it.

Stopping the command with Ctrl-C is how it ends and is not a failure: it exits
with status `0`. Audio still waiting in front of the speakers when you stop is
not played out first, so the command exits at once rather than a fraction of a
second later.

What the speakers had to do to keep up is reported on the way out, and between
them the two lines name every way playback can be choppy while the recording of
the same audio is perfect:

```
The speakers ran dry 3 time(s), and played silence until the audio caught up.
Once per transmission is expected, since the audio stops between them. Many
more than that is what choppy playback sounds like.
```

```
1.4 seconds of audio arrived faster than the speakers could play it and was dropped.
This computer is struggling to keep up, or the sound output is.
```

Running dry once per transmission is the buffer working rather than failing:
the audio stops between transmissions, so the cushion in front of the speakers
empties and has to be built again. Dropped audio should not happen at all, since
what arrives in real time is played in real time.

## Errors

| Error | Meaning | Fix |
| ----- | ------- | --- |
| `error: no scanner named: name one with --device, or pass --input to listen to a sound input directly` | Neither a scanner nor a sound input was given, so there is nothing to play. | Pass `--device`, or `--input` to open an input directly. |
| `error: no radiocli daemon is running for this scanner. ...` | Nothing is holding this scanner's sound input. | Start a daemon with `--audio`, or pass `--input` to open one directly. |
| `error: this daemon is not holding a sound input, ...` | A daemon is running, but it was started without `--audio`. | Stop it and start it again with `--audio`. |
| `error: no sound input by that name: "..."` | `--input`, or the daemon's `--audio`, named something not attached. | Run `radiocli audio` to see the names. |
| `error: "audio listen" runs until it is stopped, so it cannot be run inside a daemon` | The command was sent to a daemon to run, rather than run in a terminal. | Run it in a terminal of its own. A daemon lends a command its output for the length of the command, which only works because commands end. |
| `error: a hang of ... is not a length of quiet to wait for` | `--hang` was zero or negative. | Pass a positive duration, such as `2s`. |
| `error: a buffer of ... is not something the speakers can hold: it has to be between 40ms and 500ms` | `--buffer` was outside the range the player can stand behind. | Pass a duration between `40ms` and `500ms`. |
| `error: this copy of radiocli was built without audio support` | The build has cgo switched off, so it cannot open speakers. | Rebuild with `CGO_ENABLED=1`. |

All of them exit with status `1`.

Hearing nothing at all, with no error, means the audio is silent rather than
absent. With the squelch off, which is the default, a working cable hisses, so
silence there is the lead, the scanner's volume, or on macOS the microphone
permission having been refused. The same warning `audio output` prints appears here too when an input
delivers perfect digital zero for several seconds.

---

# audio output

Writes the scanner's audio to standard output, as it arrives, until you stop it.

## Overview

`audio output` records from a sound input and writes what it hears to stdout, so
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
radiocli audio output [flags]
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
$ radiocli audio output --device /dev/cu.usbmodem00000000000011 | ffplay -f s16le -ar 48000 -ac 1 -i -
Recording from "Cubilux CB5 Line In": signed 16-bit little-endian mono at 48000 Hz.
Play it with:  ffplay -f s16le -ar 48000 -ac 1 -i -
           or: play -t raw -e signed -b 16 -c 1 -r 48000 -
The scanner's audio is on the left channel.
```

The same audio to a second listener at the same time, compressed. One daemon,
one open sound input, two listeners:

```
$ radiocli audio output --device /dev/cu.usbmodem00000000000011 --format opus > /tmp/scanner.opus
```

Checking a cable with no daemon and no scanner:

```
$ radiocli audio output --input "Cubilux CB5 Line In" | ffplay -f s16le -ar 48000 -ac 1 -i -
```

Recording to a file, which is raw samples and needs the format given when it is
read back:

```
$ radiocli audio output --device /dev/cu.usbmodem00000000000011 > /tmp/scanner.raw
$ ffmpeg -f s16le -ar 48000 -ac 1 -i /tmp/scanner.raw /tmp/scanner.wav
```

Without a daemon:

```
$ radiocli audio output --device /dev/cu.usbmodem00000000000011
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
| `error: "audio output" runs until it is stopped, so it cannot be run inside a daemon` | The command was sent to a daemon to run, rather than run in a terminal. | Run it in a terminal of its own. A daemon lends a command its output for the length of the command, which only works because commands end. |
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
than only listen to it. Pass `--listen` to do both at once.

## Overview

`audio record` listens to the scanner and writes a separate WAV file every time
it hears a transmission, together with a JSON file of the same name describing
what that transmission was: the system, the department, the channel, and the
frequency or talkgroup behind it. Those descriptions are what make a night's
recordings searchable with ordinary tools rather than something to scroll
through. It records a scanner rather than a sound card, so `--device` is
required as well as the audio: naming the radio is what lets every file be
labelled, and it is what lets the command check that the input really is the
scanner rather than a microphone picking up the room. Nothing on the scanner is
changed and no key is pressed; everything written goes under the destination
folder, which is created if it is not there. It runs until you stop it with
Ctrl-C, and a transmission in progress at that moment is finished properly
rather than lost.

**Where one recording ends and the next begins is the scanner's squelch.** A
dispatcher and the unit answering are one continuous sound with a pause in it,
and so is one person pausing mid-sentence, so software that cuts on silence
cannot tell those apart and gets both wrong. The scanner's squelch can: it
follows the carrier rather than the voice, so it stays open through a breath and
shuts when somebody unkeys. That is what puts the two halves of an exchange in
separate files, each labelled with what it was.

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
| `--hang` | No | `500ms` | How long the scanner must stop receiving before a transmission is called finished. |
| `--min-duration` | No | `250ms` | Discard any transmission shorter than this. |
| `--max-duration` | No | `5m` | Split any transmission longer than this. |
| `--normalize` | No | `true` | Scale each recording up so its loudest moment is just under full scale. `--normalize=false` keeps the level exactly as it arrived. |
| `--listen` | No | `false` | Play the radio on this computer's speakers as it is recorded. |
| `--squelch` | No | `false` | With `--listen`, play only the transmissions. The default plays everything the input carries. Does not change what is recorded. |
| `--gain` | No | `0` | Decibels `--listen` turns the audio up by. Does not change what is recorded. |
| `--buffer` | No | `250ms` | How much audio `--listen` keeps between the radio and the speakers. Does not change what is recorded. |

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

Which side of the audio cable carries the scanner. `auto` listens to both and
decides, `left` and `right` take one side and ignore the other, and `mix`
averages the two. `auto` is right unless you know the cable. This has no effect
when the audio comes from a daemon, because the daemon has already folded it.

`auto` waits for real audio before deciding, so on a quiet channel it may not
settle until the first transmission. It judges the fold by what mixing actually
produces rather than by comparing the two sides, because **the headphone jack on
an SDS100 and an SDS150 is wired out of phase**: both sides are equally loud, and
averaging them cancels most of the sound. When that is detected, `auto` takes
one side and says so on stderr.

Recording therefore works whichever way the scanner is set, and you do not have
to check first. To fix it at the source instead, which is better because it is
then right for everything the radio is plugged into, see
[`headphone`](headphone.md).

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

How long the scanner has to stop receiving before a transmission is treated as
finished. This is what puts a dispatcher and the unit answering into separate
files instead of one, and the reason it can be this short is that the boundary
is the scanner's squelch rather than the sound. A squelch follows the carrier,
so it stays open through a speaker drawing breath mid-sentence and shuts the
moment they let go of the key. A pause in the middle of somebody talking cannot
split a recording no matter how long it lasts.

What the hang is really allowing for, then, is the carrier itself flickering,
a mobile unit passing behind a hill for a moment. Raise it if a single unit
driving through bad coverage is arriving as several files. Lower it if two
people are still being joined, though there is not much room below the default:
a dispatcher and a cruiser on a Marlinton channel left 640 milliseconds between
them, and the hang has to fit inside that.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --hang 1s
```

### `--min-duration`

The shortest recording worth keeping. A transmission shorter than this is
discarded before any file is created, so nothing has to be cleaned up
afterwards. It exists because a squelch tail or a control channel burst is not
something anybody wants to listen to.

Do not raise this much above the default without meaning to. Recordings are cut
at the keyup and again whenever the transmitting radio changes, so a short reply
is a file of its own rather than something riding along inside the dispatcher's,
and anything set above its length deletes it instead of merging it. A unit
answering "ten four" runs well under half a second: one measured on a live P25
channel was separated correctly and then dropped, which is what brought the
default down to `250ms`.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --min-duration 1s
```

### `--max-duration`

How long one recording may run before it is split and a new one started. The
audio carries on across the split with nothing lost at the join. It exists so a
stuck microphone cannot produce one enormous file.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --max-duration 2m
```

### `--normalize`

Scales each recording up once the transmission has ended, so its loudest sample
sits just under full scale. This is on unless you turn it off.

```
radiocli --device /dev/cu.usbmodem00000000000011 audio record ~/scanner --normalize=false
```

**It is on because recordings otherwise come out too quiet to play.** A line
input applies no gain of its own, and the scanner's volume control does not
change what leaves it for one, so a recording arrives at whatever level the
radio happened to send and stays there however the volume is set. There is
nothing on the radio or on the sound card to turn up. Measured on an SDS150
through a line input, a recording peaked 12 dB below full scale and averaged 36
dB below it, which is quiet enough to need the volume raised on every playback.

**A microphone input has the opposite problem, and this does not fix it.** There
the input applies far too much gain and the audio is already flat-topped by the
time anything here sees it. That is what the clipping warning is for, and no
amount of scaling afterwards puts back peaks that never reached the file.

**It does not make anything clearer.** The noise floor comes up by exactly as
much as the voice, so a recording gets louder and no cleaner.

**What it costs is the comparison between recordings.** Left alone, one
recording being quieter than another says the signal was weaker. Normalized,
every recording is equally loud and that difference is gone. Each one carries
`"normalized": true` in its description so anything reading them later knows
which it is looking at, but the original levels are not recoverable from the
audio. Pass `--normalize=false` if the levels are what you are keeping the
recordings for.

Silence is left alone, and the gain is capped at about 30 dB. A recording that
caught almost nothing would otherwise ask for a factor in the hundreds, which
does not rescue a faint transmission so much as play a noise floor at full
volume. Thirty decibels covers every real case: a line input recording measured
12 dB below full scale, and a genuinely quiet transmission 30 dB below is
brought all the way up. Anything fainter is left short of the target.

**A transmission split by `--max-duration` is normalized in halves.** Each file
is scaled by its own loudest moment, so where a long recording is cut into two
the level can step at the join. Pass `--normalize=false` if you are recording
something long enough to be split and the join matters.

### `--listen`

Plays the radio on this computer's speakers while it is being recorded. Off by
default, because a recorder is usually left running somewhere nobody is sitting.

It is the same audio, played rather than written, so nothing is opened twice and
a directly opened `--input` still works: this is the way to hear and record at
once without a daemon. What is played never changes what is recorded.

It plays everything the input carries, hiss and all, the way the scanner's own
speaker does. Add [`--squelch`](#--squelch-1) to hear only the transmissions.

### `--squelch`

Plays only the transmissions, rather than everything the input carries. Off by
default. It decides what reaches the speakers and nothing else: what is recorded
is the scanner's decision either way, so this cannot change a file.

The speakers open on the first frame above the noise floor and stay open for as
long as the transmission does, `--hang` included. They deliberately do not wait
for a recording to open, which happens only once a transmission has outlived
`--min-duration`: waiting cost the first half second of every transmission,
which in back and forth traffic is most of the first word each time.

So what you hear and what you keep are not quite the same. A transmission
shorter than `--min-duration` is heard and not written, which is what a
scanner's own speaker does with a blip. Everything that is written is heard.

Asking for it without `--listen` is refused, since there would be nothing for it
to decide about.

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
$ find ~/scanner -name '*.json' -exec jq -r 'select(.channel == "MARLINTON DISPATCH") | .file' {} +
2026-08-22/19-54-03_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav
2026-08-22/20-11-47_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav
```

Finding everything over ten seconds long from one evening:

```
$ find ~/scanner -name '*.json' -exec jq -r 'select(.duration > 10 and (.start | startswith("2026-08-22T20"))) | "\(.duration)s \(.channel)"' {} +
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
  2026-08-22/
    19-54-03_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.wav
    19-54-03_PUBLIC-SAFETY_POLICE-DEPARTMENT_MARLINTON-DISPATCH.json
```

Every WAV is signed 16-bit little-endian mono at 48000 Hz, which any player
opens. There is no compressed option: a recording is an archive, and WAV has no
codec in it to fall out of date.

The `.json` file beside each recording is the same object printed by
`--output json`, so there is one shape to learn.

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
| `frequency` | string | The frequency the scanner was on, carrying its unit. On a trunked system this is the voice channel the site handed out for this call, which is a different one next time. |
| `talkgroup` | string | The talkgroup number on a trunked system. Absent on a conventional system. |
| `unit` | string | The radio heard transmitting, when the scanner decoded one. |
| `modulation` | string | How the scanner was demodulating, such as `NFM`. |
| `nac` | string | The network access code, such as `8A1h`, on a trunked P25 system and on a P25 conventional channel alike. Absent on anything that is not P25, and absent on a P25 transmission too short for the scanner to have decoded one: a measured 1.1 second transmission reported the format and no code. |
| `rssi` | string | How strong the signal was, in the scanner's own units, taken from the strongest reading during the transmission. |
| `digital` | string | The digital format the transmission carried, such as `P25` or `DMR`, taken from whichever answer most of the readings agreed on. Absent when it was analog. |
| `reason` | string | Why the recording ended: `hang` when the scanner stopped receiving, `split` when it reached `--max-duration`, `channel` when the scanner moved to another channel, `stopped` when you stopped the command. |
| `samples` | number | How many times the scanner was asked what it was hearing while this was being recorded. Always present. |
| `channels` | array of strings | Every distinct channel seen during the recording. Present only when there was more than one. |
| `dropped` | number | Frames of audio the sound card produced that never arrived. Present only when some were lost. Each frame is 20 ms. |
| `normalized` | boolean | The audio was scaled up after the transmission ended so its loudest sample sits just under full scale. Present unless `--normalize=false` turned that off. |

**A trunked recording carries a frequency as well as a talkgroup.** They answer
different questions: the talkgroup says who was talking, and the frequency says
where the radio actually was. On a trunked system that is a voice channel the
site handed out for that call and will hand to someone else next time, so it is
what lines a recording up against a spectrum capture or against what another
receiver heard. It comes off the site rather than off the channel, because a
trunked reply carries no conventional channel element at all.

**`nac` identifies the system rather than the call.** Two P25 systems can use
the same talkgroup numbers, and this is what tells their recordings apart. On a
conventional P25 channel it does the job a tone squelch does on an analog one,
and the scanner keeps it in the same two fields. The
other identifiers on the scanner's detailed display, `WACN`, `Sys ID`, `RFSS ID`
and `Site ID`, are not in the reply this command reads, so they are not here.

**`digital` is the answer most of the readings gave**, not the first one. The
scanner decides afresh on every reading, and it reports nothing at all until it
has decoded something, so an early answer is both unavoidable to wait for and
unsafe to trust: one measured transmission of 29 seconds was labelled `Link`, a
value this scanner's own documentation does not list, on the strength of a
single reading while the three hundred after it agreed on `P25`.

**Test `digital` for being present, not for being `P25`.** The value is whatever
the scanner's decoder said, and it does not always say a format name: a P25
conventional channel here was measured reporting `Link` for whole transmissions,
alternating with `P25` between one transmission and the next on one frequency
with one access code. Those recordings are ordinary voice and are digital. What
`Link` means is not documented by the scanner's own protocol, which does not
list the value at all. If you need to know a recording was P25 specifically,
read `nac`: only a P25 transmission has one.

**`digital` is how you tell a digital recording from an analog one.**
`modulation` cannot answer it. It reports the demodulator's state, so a channel
programmed `Auto` and carrying P25 is written down as `NFM` like anything else.
The other tempting clue, whether `unit` was filled in, is absence used as
evidence: a digital transmission the scanner joined late names no radio at all
and looks identical to an analog one.

**It is per recording, and that is the point.** One conventional police channel
was measured carrying analog traffic and P25 traffic fifteen minutes apart, both
labelled `NFM`. Two recordings of the same frequency can honestly disagree here,
so the field belongs beside the audio rather than anywhere above it.

**`samples` is how much the label is worth.** A transmission of any ordinary
length is covered many times over, because the scanner is asked three times a
second. A `samples` of `0` means the transmission was over before the scanner
could be asked once, and the fields naming a channel are left empty rather than
guessed at.

**A recording is never labelled from a guess.** If the scanner never named a
channel, the channel fields are absent from the JSON and the recording is named
from its timestamp alone.

**An overloaded input is warned about, in yellow, after every recording it spoils.**

```
15:08:19    3.3s  City of Manchester Fire Fire Tac 4
  warning: 9.3% of that recording is clipped, so the sound input is being
           overloaded. Turn the scanner's volume down from 12 of 15 until this
           stops, or move the cable to a line input rather than a microphone
           input.
```

This is what a microphone input does with a line-level signal. A mic input
expects a few millivolts from a microphone capsule and applies gain on top; the
scanner's headphone socket puts out far more than that, so the samples run past
full scale and are handed back flat-topped. The distortion cannot be undone
afterwards, because the peaks never reached the file.

Nothing else goes wrong. The transmissions are still detected, cut and labelled
correctly, which is exactly why it is worth interrupting about: a night of
recordings can be quietly ruined while every line on screen looks right.

It is said again for each spoiled recording rather than once for the run,
because the warning is only useful beside the thing it is about, and because it
stopping is how you know the volume is now low enough. The level is read from
the scanner each time, so turning the radio down part way through a run is
reflected in the next warning rather than repeating the number you just changed.

**Turn the volume down on the radio, not with `volume set`.** This command holds
the serial port for as long as it runs, so a second `radiocli` is refused as busy
while it is recording. The scanner's own volume control is the one that works
mid-run, and it is what the warning is asking for.

The colour is dropped when stderr is not a terminal, and when `NO_COLOR` is set.

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
| `error: --buffer sets how far behind the radio the speakers play, which only means something with --listen` | `--buffer` was given on its own. | Add `--listen`, or drop `--buffer`. |
| `error: --squelch decides what reaches the speakers, which only means something with --listen` | `--squelch` was given on its own. | Add `--listen`, or drop `--squelch`. |
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
