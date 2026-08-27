# wavfile

## What this does?
Writes one transmission of scanner audio to a WAV file: open a file, hand it audio as it arrives, close it, and what is left on disk is something any player on any computer will open by double-clicking it. It can also scale the whole recording up before closing, for audio that arrived quieter than the file could hold.

## Why we use it?
Everything upstream of here deals in audio that cannot be kept. `audiofeed` hands out 20 ms slices of raw samples, which are only meaningful to something that has been told the rate, the width and the channel count, and `opusenc` turns those into bare Opus packets with no container around them, which nothing that plays files will open. A recording is a different thing from a stream, and the difference is time: somebody opens it months later, on a computer that may not have this tool on it, and by then it has to be playable anywhere and identical to what arrived. WAV is both, with no codec to fall out of date, the bytes the sound card produced, and a 44 byte header of arithmetic rather than a dependency. Opus was the obvious alternative and is deliberately not offered, because the encoder this tool carries is pinned to one commit of a port still being written and cannot decode, so an Opus archive would have fidelity that depended on which build made it and no way to be read back. The cost is size, about 345 MB an hour against 15 of Opus, and it is affordable because the recorder only ever writes transmissions and a scanner is silent most of the time.

Keeping this in its own package means the format lives in one place. The two length fields in a WAV header cannot be known until the audio stops, so they are written last by seeking back over them, and a file closed without that step holds every sample while still declaring it holds none; that is the kind of detail to get right once rather than in whichever command needs a file next. Normalizing lives here too, because this is the only layer holding the finished recording: the factor depends on the loudest sample in the whole transmission, which nothing knows until it has ended, so the peak is tracked as the audio is written and the scaling is done in place, a read and a write of the audio with no second file on disk, with rounding held inside what an int16 can carry so no sample wraps to the opposite extreme and clicks.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/wavfile"

// The directory has to exist already. This package creates files, not folders.
w, err := wavfile.Create("recordings/2026-08-22/19-54-03_Marlinton-Dispatch.wav")
if err != nil {
    return err
}
defer w.Close()

// Audio as it arrives, straight from a feed subscription.
for frame := range sub.Frames() {
    if err := w.Write(frame.PCM); err != nil {
        return err
    }
}

// How long the recording is, worked out from the audio in it rather than from
// a clock, so dropped frames make it shorter instead of lying.
fmt.Println(w.Duration())

// Close is what completes the header. Closing twice is safe, which is what
// makes the deferred call above harmless on the path that already closed.
return w.Close()
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **RIFF and WAV** - The container, and why a file that declares its length in two places has to be finished by going back to the beginning
- **PCM s16le** - The sample format written here: signed 16-bit little-endian mono at 48000 Hz, the same samples `audiofeed` produces
- **Seeking to patch a header** - The ordinary way to write a container whose sizes are not known until the end, and the alternative of buffering the whole recording to measure it first
- **Lossless versus lossy archives** - Why the copy being kept is a different decision from the copy being streamed, which is why this package exists alongside `opusenc` rather than instead of it
- **Uncompressed audio sizes** - 96 KB a second, and why that is affordable for something that only records while a channel is actually in use
