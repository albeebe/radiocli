# audioout

## What this does?
This package lists the places this computer can play sound and plays audio on one of them. Hand it 20 ms frames of mono samples and they come out of the speakers, whether they arrived from a sound card, from a daemon over a socket, or from the squelch gate releasing a whole transmission at once.

## Why we use it?
It is the other direction of `audioin`, and it exists for the same reason: to keep one third-party library out of the rest of the tool. `malgo` is cgo bindings to miniaudio, and everything that knows so lives in `malgo.go`, with `nocgo.go` standing in for it in a build made without cgo so that cross compiling still produces a working binary that simply says it cannot play.

The part that is not a mirror image is the buffering, and it is the reason this is a package rather than a function. A capture pushes: audio exists and the callback delivers it whether anything is ready or not. A playback pulls: the sound card asks for audio at a moment of its choosing and plays whatever is in the buffer when the callback returns, including whatever was there before if nothing new was written. Nothing in this tool can answer that question on demand, because its audio arrives in frames from a socket, a capture callback, or a gate that hands over a transmission in one go. The ring buffer here is what makes a pull-shaped device usable by a push-shaped program: a small cushion goes in front of the audio once, late arrivals land in the slack rather than in a hole, and audio arriving faster than the speakers take it loses its oldest rather than stalling whoever is writing it.

The two sizes it is built from were measured rather than picked, and they are measured against different things. The ring is a second deep because the producer stalls: the recorder finishes a file in the middle of the audio, normalizing a whole WAV and filing a description beside it before it reads another frame, and a 240 ms ring lost a quarter of a second of speech outright to a 300 ms stall. Depth costs nothing, since what actually stands in the ring is the cushion and not the ring.

The cushion is the opposite kind of number, because it is the only part of this package a listener can hear. Somebody playing a scanner on their computer is usually sitting next to the scanner, comparing this against the radio's own speaker in the same room, so it is spent as narrowly as the jitter allows: 40 ms, which is two frames of slack. The device end needs none of it, which is what makes that affordable. Measured over two seconds of callbacks, it asks for exactly 10 ms of audio every 10 ms and the longest gap between two requests was 10.19 ms, so the whole cushion is there for the producer.

Naming works exactly as it does in `audioin`: a sink is a name and nothing else, the library's own identifiers never leave `malgo.go`, and a name matching two attached devices is refused rather than guessed at. The default device is the one place the two differ. `audioin` never falls back to it, because the default input on a laptop is the built-in microphone and recording the room instead of the scanner is a failure that sounds like it worked. There is no matching trap on the way out, so an empty name here means the output the person is already listening to everything else on.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/audioout"

// Whichever output this computer is already using, or name one from Sinks().
player, err := audioout.Open("")
if err != nil {
    return err
}
defer player.Close()

// Hand over mono frames as they arrive. It never blocks and never fails.
for frame := range sub.Frames() {
    player.Play(frame.PCM)
}

// What it had to do to keep the speakers fed.
if stats := player.Stats(); stats.Dropped > 0 {
    log.Warn("audio arrived faster than it could be played", "bytes", stats.Dropped)
}
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

Nothing here opens the machine's real speakers. The tests that reach the library run against miniaudio's null backend, a software device that needs no sound card, so a test run is silent and works on a machine with nothing attached.

## Further reading
- **Jitter buffer** - The cushion between audio that arrives when it arrives and hardware that asks for it on a schedule of its own, and why it costs latency to remove gaps
- **Ring buffer** - The fixed run of bytes underneath that, written at one end and read at the other by two threads that never wait for each other
- **Buffer underrun** - What a sound card plays when nothing was ready for it, and why filling every byte of its buffer is not optional
- **PCM audio** - The raw signed 16-bit sample format this plays, the same one `audiofeed` produces
- **miniaudio** - The C library underneath `malgo`, which converts this package's one format into whatever the speakers natively want
