# opusenc

## What this does?
Turns the scanner's audio into compact Opus packets that any listener can play. Each 20 ms slice of sound goes in and comes out small enough to stream over a slow connection.

## Why we use it?
The tool streams live scanner audio to listeners over connections it knows nothing about: one listener may be on the same machine, another on a phone with a weak signal. Raw audio costs too much bandwidth for the weak case, and most codecs lock in one bitrate for the life of a stream. Opus lets the bitrate change from one frame to the next without the far end resetting anything, which is what makes adapting to a slow listener possible at all.

Keeping the encoding in its own package keeps the third-party codec library out of the rest of the tool. Nothing outside this package names one of the library's types, so replacing the codec later means rewriting one package rather than hunting down every place that touched it. The library is also pure Go and pinned to a specific commit, so it can never be the reason a cross compile fails, and its output cannot drift while the upstream port is still in progress.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/opusenc"

// One encoder per listener, so each can run at its own bitrate.
enc, err := opusenc.New(32000)
if err != nil {
    return err
}

// pcm holds exactly one 20 ms frame of 48 kHz mono 16-bit audio.
pcm := make([]byte, opusenc.FrameBytes)
out := make([]byte, opusenc.MaxPacket)

n, err := enc.Encode(pcm, out)
if err != nil {
    return err
}
send(out[:n])

// A listener falling behind costs less from the next frame on.
if err := enc.SetBitrate(24000); err != nil {
    return err
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
- **Opus** - The audio codec these packets use, chosen because its bitrate can move mid-stream
- **CELT** - The low-latency half of Opus the pinned library implements, tuned for music over speech
- **Constrained VBR** - The bitrate mode that keeps loud frames from spiking a slow listener's queue
- **PCM s16le** - The raw sample format Encode takes: signed 16-bit little-endian mono at 48 kHz
- **Go pseudo-versions** - How the codec dependency stays pinned to one commit instead of a tag
