# audio

## What this does?
This package is the `audio` command: it lists every sound input this computer can record from and every speaker it can play on, its `audio listen` subcommand plays the scanner on those speakers, its `audio output` subcommand writes the sound arriving on an input to standard output, and its `audio record` subcommand keeps that sound as one file per transmission. It is how the scanner's audio gets off the radio and into a speaker, a recording, or another program.

## Why we use it?
The scanner's audio does not travel over the USB cable this tool controls it with. It leaves the radio as an ordinary sound signal on a cable and arrives at whatever the other end of that cable is plugged into, so which input carries it is something only the person who ran the cable can know. Before anything can listen, there has to be a way to see the choices and name one, and that has to be possible without the operating system interrupting to ask about a microphone nobody has chosen yet.

Keeping the looking and the listening as separate verbs is the whole point of this package. Listing opens nothing, so it never raises the microphone permission prompt and can be shown in a picker safely. Listening opens something, which is a different act with different consequences: a sound card can only be open once, so naming an input with `--input` takes it for this process alone, while leaving it out asks the daemon for a copy of what it already has and lets several things hear the same radio at once. For `audio output`, standard output carries the audio and nothing else, which is why every word that command has to say goes to standard error instead, including the exact flags to hand a player.

`audio listen` is the shortest of the three verbs and the one most people want: it plays the radio on this computer, and writes nothing anywhere. It squelches by default, because between transmissions a scanner's output is hiss and an evening of that is not worth listening to, and the detector it squelches with is `audiogate`, the same one that decides where a recording begins. That is deliberate: the two commands agree about what a transmission is, so what you hear is what would have been kept. What it does not do is play the audio the gate hands back, because the gate holds audio for the length of its hang so that it can trim the end of a recording, and playing that would put every word two seconds behind the radio. The gate is asked only whether a transmission is open, and the frame that has just arrived is the one that reaches the speakers. The cost is the front of each transmission, up to the point the detector is sure, which is the same trade a scanner's own speaker makes.

`audio record --listen` is the same idea from the other end: a recorder that also plays what it is writing, so one process can hear and keep at once even when it has opened a sound input directly and no daemon is involved. It plays while a recording is open and nothing in between, so the flags that shape what is written shape what is played.

The cable the person ran is also the thing most likely to be wrong, and wrong in a way nothing else reports. A sound card with only a microphone input applies a microphone's worth of gain to a line-level signal, and the samples come back flat-topped at full scale. Every other part of the run still works: the scanner still says when it is receiving, so transmissions are still detected, cut and labelled correctly, and a night of ruined audio scrolls past looking exactly like a night of good audio. So `audio record` counts the samples that hit full scale and says so in yellow after each recording it spoils, naming the scanner's volume because that is the number the person is about to change. The level is read from the radio for each warning rather than remembered from the start of the run, since somebody who has just turned the volume down and is told the old number reads it as the tool not seeing the radio at all. The advice is to turn the knob rather than to run `volume set`, which would be refused: this command holds the serial port for as long as it records, so the very run printing the advice is what makes that invocation busy. It is repeated for every spoiled recording rather than said once, because the warning stopping is how they know they have turned it down far enough.

A line input has the opposite problem and no such remedy. It applies no gain of its own, and the scanner's volume control does not change what leaves it for one, so recordings come out quiet however the radio is set and there is nothing on either the radio or the card to turn up. `--normalize` is for that, and it is on by default: each recording is scaled once its transmission has ended so its loudest moment sits just under full scale. Defaulting it on is the judgement that a recording nobody can hear without reaching for the volume is the worse outcome. What it gives up is the difference between a strong signal and a weak one, since every recording comes out equally loud, so the entry records that it happened and `--normalize=false` keeps the levels for anybody who is collecting them. The gain is capped at about thirty decibels, because a recording that caught almost nothing would otherwise ask for a factor in the hundreds and get a noise floor played at full volume rather than a rescued transmission.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	audio.New,
}
```

`audio record` is the last of the verbs, and it is a different kind of thing
from the others. Listing and playing are about the sound card; recording is about
the scanner, so it is the one subcommand here that requires `--device`. That is
not an inconvenience to be worked around, it is what the feature is: naming the
radio is what lets every file be labelled with the channel it came from, and it
is what makes it possible to notice that the input is not the scanner at all.
The rest of it lives a layer down, in `audiogate` for deciding where a
transmission begins and ends, and in `recordings` for filing the result.

```bash
# What can this computer record from, and where can it play?
radiocli audio

# The same listing for a script.
radiocli audio -o json

# Hear the scanner on this computer's own speakers.
radiocli --device $SDS audio listen

# The same, on a named set of speakers, with the hiss between transmissions left in.
radiocli --device $SDS audio listen --speaker "Cubilux CB5 Headphones" --squelch=false

# Send the audio to another program instead, through a daemon holding the input.
radiocli --device $SDS audio output | ffplay -f s16le -ar 48000 -ac 1 -i -

# Open a sound input directly, without sharing it, and record a WAV.
radiocli audio output --input "USB Audio CODEC" --channel left | \
  sox -t raw -e signed -b 16 -c 1 -r 48000 - scanner.wav

# Compressed, for a program rather than a player.
radiocli --device $SDS audio output --format opus --bitrate 48000 > stream.raw

# Keep what it hears: one WAV per transmission, with a description beside it.
radiocli --device $SDS audio record ~/scanner --input "USB Audio CODEC"

# The same, played on the speakers as it is written.
radiocli --device $SDS audio record ~/scanner --input "USB Audio CODEC" --listen

# The same, as a live feed of labelled transmissions for an agent.
radiocli --device $SDS audio record ~/scanner -o json
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, and `audio listen`, `audio output` and `audio record` are subcommands attached to it, which is how every command in this tool is wired into the tree
- **Raw PCM audio** - Signed 16-bit little-endian mono at 48000 Hz has no header at all, so the rate, the width and the channel count have to be told to a player rather than discovered by it
- **Opus and CELT** - The encoder here is CELT only, which is why the comfortable bitrate for a voice channel is higher than the numbers usually quoted for Opus
- **Standard output versus standard error** - The audio owns stdout, so every message the command has for a person goes to stderr, which is what keeps a pipe into a player clean
- **SIGPIPE and closed pipes** - Quitting the player closes the pipe, and treating that ordinary ending as a failure would make every normal use of this command exit non-zero
- **Jitter buffers** - Why playing audio that arrives from a socket needs a cushion in front of the speakers, and why that cushion is latency worth paying
- **Clipping and line versus microphone level** - A mic input expects millivolts and adds gain, a line output delivers far more, and the samples past full scale are gone before anything downstream can see them
