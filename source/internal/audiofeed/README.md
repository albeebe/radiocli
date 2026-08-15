# audiofeed

## What this does?
This package listens to the sound card the scanner is plugged into and shares that audio with everyone who asks to hear it. One recording runs, and every listener receives its own copy, so several programs can listen at the same time without fighting over the hardware.

## Why we use it?
A sound card produces audio whether or not anybody is listening, and two programs opening the same input is at best wasteful and on some systems refused outright. The audio also arrives in whatever chunk sizes the hardware feels like delivering, on a thread that must never be held up, while everything downstream wants tidy 20 ms frames of mono. Something has to sit between those two worlds: reading the card exactly once, cutting the stream into frames, folding stereo down to mono, and noticing problems such as a silent input or a listener that fell behind.

Keeping this in its own package means the daemon and everything downstream of it never touch a sound library, and almost all of the audio path can be tested by writing bytes into a buffer rather than by plugging in a radio. It also gives the platform one place to grow: the Publisher interface exists so a squelch gate can later be dropped between the capture and the fan-out without any listener changing, and the frame numbering gives every consumer an honest clock for telling dropped audio apart from silence.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/audiofeed"

// One feed, shared by every listener.
feed := audiofeed.New(logger)

// Open the sound input and start cutting frames into the feed.
capture, err := audiofeed.Start(audiofeed.Options{
    Source:  "USB Audio CODEC",
    Channel: audiofeed.ChannelAuto,
    Log:     logger,
}, feed)
if err != nil {
    return err
}
defer capture.Close()

// Each listener subscribes for its own copy of the audio.
sub := feed.Subscribe(8)
defer sub.Close()

for frame := range sub.Frames() {
    // frame.PCM is 20 ms of mono, ready for an encoder.
    encode(frame.PCM)
}
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Ring buffer** - The lock-free structure that carries audio from the sound card's thread to the frame cutter without ever blocking the hardware callback
- **PCM audio** - The raw signed 16-bit sample format every frame here is made of, and what an encoder such as Opus consumes
- **RMS and dBFS** - How loudness is measured here, both for the level meter and for deciding which stereo channel carries the scanner
- **Downmixing** - Folding two stereo channels into one mono signal, and why picking the wrong fold halves the level or produces silence
- **Fan-out (publish/subscribe)** - The pattern that lets one capture serve many listeners, each with its own queue so a slow one cannot stall the rest
