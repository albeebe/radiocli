# audioout

## What this does?
This package lists the places this computer can play sound and plays audio on whichever one the computer is using. Hand it 20 ms frames of mono samples and they come out of the speakers, whether they arrived from a sound card, from a daemon over a socket, or from the squelch gate releasing a whole transmission at once.

## Why we use it?
It is the other direction of `audioin`, and it exists for the same reason: to keep the audio libraries out of the rest of the tool. Nothing outside this package names a type belonging to either of them, and `nocgo.go` stands in for both in a build made without cgo so that cross compiling still produces a working binary that simply says it cannot play.

There are two of them, chosen by platform, and the split is not tidiness. Playback on macOS and Windows goes through `oto`, in `oto.go`. Playback on Linux goes through `malgo`, the cgo bindings to miniaudio, in `malgoplay.go`. The reason is a dependency rather than a preference: `oto` links ALSA at build time, so a Linux binary built against it will not start on a machine without that library, and that means the whole program rather than the audio commands, serial port included. A scanner on a headless Raspberry Pi is the machine most likely to be missing it. miniaudio opens the system's sound library at run time and carries on without it, so Linux keeps the download-and-run promise and the rest of the tool gets the library that fixed the fault this package went looking for. `malgo.go` remains on every platform, because listing what a machine can play on is still its job and `oto` cannot enumerate anything.

What is honest to say about that: the choppy playback that caused the switch was heard on a Mac, and neither library has been listened to on Linux. miniaudio's own author calls its Core Audio backend the worst he has worked with, and its ALSA backend is the far more travelled path, which is the reason to expect Linux to be fine rather than a measurement saying so.

The part that is not a mirror image is the buffering, and it is the reason this is a package rather than a function. A capture pushes: audio exists and the callback delivers it whether anything is ready or not. A playback pulls: the sound card asks for audio at a moment of its choosing and plays whatever is in the buffer when the callback returns, including whatever was there before if nothing new was written. Nothing in this tool can answer that question on demand, because its audio arrives in frames from a socket, a capture callback, or a gate that hands over a transmission in one go. The ring buffer here is what makes a pull-shaped device usable by a push-shaped program: a small cushion goes in front of the audio once, late arrivals land in the slack rather than in a hole, and audio arriving faster than the speakers take it loses its oldest rather than stalling whoever is writing it.

The two sizes it is built from are measured against different things. The ring is a second deep because the producer stalls: the recorder finishes a file in the middle of the audio, normalizing a whole WAV and filing a description beside it before it reads another frame, and a 240 ms ring lost a quarter of a second of speech outright to a 300 ms stall. Depth costs nothing, since what actually stands in the ring is the cushion and not the ring.

The cushion is the caller's number, because it is the only part of this package a listener can hear: everything plays that far behind the radio. It has been both too small and too large. Under miniaudio on a Mac a 40 ms cushion broke every one of 120 test tones into as many as six pieces with digital silence spliced into them, none of which showed on this side of the library, and the default went to a quarter of a second to hide it. Once playback moved to `oto` that cost was no longer being paid for anything, and the default came back down: three settings were run for four minutes each against live traffic, and 80 ms was the smallest that kept a frame in hand for a busy machine. The duration is spent twice over, once as the cushion and once as the size of the buffer the device itself runs, and the trade is exposed to the person listening as `--buffer` rather than decided here.

Which output is not a choice this package offers. `oto` plays on whichever one the system is set to and has no way to name another or to say which it took, so rather than let that differ by platform there is no naming anywhere: `Open` takes a buffer and nothing else. `Sinks` still lists what the machine has, because knowing where the sound will come out is worth something even when it cannot be redirected from here.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/audioout"

// Always whichever output this computer is already using. DefaultBuffer is the
// cushion for a listener with no opinion; the trade it makes is described on
// the constant.
player, err := audioout.Open(audioout.DefaultBuffer)
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

Nothing here opens the machine's real speakers. The tests that reach miniaudio run against its null backend, a software device that needs no sound card, so a test run is silent and works on a machine with nothing attached. `oto` has no equivalent, so the one function that opens a real device through it is faked at the seam above it and is the single statement in this package no test enters.

## Further reading
- **Jitter buffer** - The cushion between audio that arrives when it arrives and hardware that asks for it on a schedule of its own, and why it costs latency to remove gaps
- **Ring buffer** - The fixed run of bytes underneath that, written at one end and read at the other by two threads that never wait for each other
- **Buffer underrun** - What a sound card plays when nothing was ready for it, and why filling every byte of its buffer is not optional
- **PCM audio** - The raw signed 16-bit sample format this plays, the same one `audiofeed` produces
- **miniaudio** - The C library underneath `malgo`, which plays on Linux and lists the outputs everywhere, and which opens the system's sound library at run time rather than linking it
- **oto** - The library that plays on macOS and Windows, pulling audio from a reader on a thread of its own rather than calling back for it
