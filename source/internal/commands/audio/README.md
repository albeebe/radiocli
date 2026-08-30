# audio

## What this does?
This package is the `audio` command: it lists every sound input this computer can record from and every speaker it can play on, its `audio listen` subcommand plays the scanner on those speakers, its `audio output` subcommand writes the sound arriving on an input to standard output, and its `audio record` subcommand keeps that sound as one file per transmission. It is how the scanner's audio gets off the radio and into a speaker, a recording, or another program.

## Why we use it?
The scanner's audio does not travel over the USB cable this tool controls it with. It leaves the radio as an ordinary sound signal on a cable and arrives at whatever the other end of that cable is plugged into, so which input carries it is something only the person who ran the cable can know. Before anything can listen, there has to be a way to see the choices and name one, without the operating system interrupting to ask about a microphone nobody has chosen yet. The cable is also the thing most likely to be wrong, and wrong in a way nothing else reports: a microphone input applies a microphone's worth of gain to a line-level signal and hands back samples flat-topped at full scale, while a line input records the scanner so quietly that nothing on the radio or the card can turn it up.

Keeping the looking and the listening as separate verbs is the whole point of this package. Listing opens nothing, so it never raises the microphone permission prompt and can be shown in a picker safely; opening is a different act with different consequences, since a sound card can only be open once, which is why `--input` takes an input for this process alone while leaving it out asks the daemon for a copy of what it already has, so several things can hear the same radio at once. `audio listen` plays the radio and writes nothing, passing on everything the input carries by default so a cable can be told apart from a dead one by ear, with `--squelch` handing the decision to the same gate that finds transmissions for the recorder, opened on the first frame above the noise floor so no first word is lost. `audio output` gives standard output to the audio and says everything else, the flags to hand a player included, on standard error. `audio record` is the one verb that requires `--device`, because naming the radio is what labels every file with its channel and what lets the tool notice the input is not the scanner at all; it warns after each clipped recording, quoting the volume knob that fixes it, and `--normalize` rescues the quiet line-input ones. The detection itself lives a layer down, in `audiogate` for deciding where a transmission begins and ends, and in `recordings` for filing the result.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	audio.New,
}
```

```bash
# What can this computer record from, and where can it play?
radiocli audio

# The same listing for a script.
radiocli audio -o json

# Hear the scanner on this computer's own speakers.
radiocli --device $SDS audio listen

# The same, on a named set of speakers, with the hiss between transmissions left in.
radiocli --device $SDS audio listen --squelch

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
- **Standard output versus standard error** - The audio owns stdout, so every message the command has for a person goes to stderr, which is what keeps a pipe into a player clean
- **Jitter buffers** - Why playing audio that arrives from a socket needs a cushion in front of the speakers, and why that cushion is latency worth paying
- **Clipping and line versus microphone level** - A mic input expects millivolts and adds gain, a line output delivers far more, and the samples past full scale are gone before anything downstream can see them
