# audio

## What this does?
This package is the `audio` command: it lists every sound input this computer can record from, its `audio feed` subcommand writes the sound arriving on one of them to standard output, and its `audio record` subcommand keeps that sound as one file per transmission. It is how the scanner's audio gets off the radio and into a player, a recording, or another program.

## Why we use it?
The scanner's audio does not travel over the USB cable this tool controls it with. It leaves the radio as an ordinary sound signal on a cable and arrives at whatever the other end of that cable is plugged into, so which input carries it is something only the person who ran the cable can know. Before anything can listen, there has to be a way to see the choices and name one, and that has to be possible without the operating system interrupting to ask about a microphone nobody has chosen yet.

Keeping the looking and the listening as two verbs is the whole point of this package. Listing opens nothing, so it never raises the microphone permission prompt and can be shown in a picker safely. Listening opens something, which is a different act with different consequences: a sound card can only be open once, so naming an input with `--input` takes it for this process alone, while leaving it out asks the daemon for a copy of what it already has and lets several things hear the same radio at once. Standard output carries the audio and nothing else, which is why every word this command has to say goes to standard error instead, including the exact flags to hand a player.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	audio.New,
}
```

`audio record` is the third verb, and it is a different kind of thing from the
other two. Listing and listening are about the sound card; recording is about
the scanner, so it is the one subcommand here that requires `--device`. That is
not an inconvenience to be worked around, it is what the feature is: naming the
radio is what lets every file be labelled with the channel it came from, and it
is what makes it possible to notice that the input is not the scanner at all.
The rest of it lives a layer down, in `audiogate` for deciding where a
transmission begins and ends, and in `recordings` for filing the result.

```bash
# What can this computer record from?
radiocli audio

# The same listing for a script.
radiocli audio -o json

# Play the scanner through a daemon that is already holding the input.
radiocli --device $SDS audio feed | ffplay -f s16le -ar 48000 -ac 1 -i -

# Open a sound input directly, without sharing it, and record a WAV.
radiocli audio feed --input "USB Audio CODEC" --channel left | \
  sox -t raw -e signed -b 16 -c 1 -r 48000 - scanner.wav

# Compressed, for a program rather than a player.
radiocli --device $SDS audio feed --format opus --bitrate 48000 > stream.raw

# Keep what it hears: one WAV per transmission, described and indexed.
radiocli --device $SDS audio record ~/scanner --input "USB Audio CODEC"

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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, and `audio feed` is a subcommand attached to it, which is how every command in this tool is wired into the tree
- **Raw PCM audio** - Signed 16-bit little-endian mono at 48000 Hz has no header at all, so the rate, the width and the channel count have to be told to a player rather than discovered by it
- **Opus and CELT** - The encoder here is CELT only, which is why the comfortable bitrate for a voice channel is higher than the numbers usually quoted for Opus
- **Standard output versus standard error** - The audio owns stdout, so every message the command has for a person goes to stderr, which is what keeps a pipe into a player clean
- **SIGPIPE and closed pipes** - Quitting the player closes the pipe, and treating that ordinary ending as a failure would make every normal use of this command exit non-zero
