# audioin

## What this does?
Lists the sound inputs attached to this computer and records audio from the one you name, such as the line-in a scanner is plugged into. It is how the tool hears what the scanner is playing.

## Why we use it?
There is no credible pure Go way to take audio off a sound card on macOS, Linux and Windows, so recording has to go through a native library. That brings real problems with it: the library is cgo bindings to miniaudio, device identifiers are unstable blobs that change when a device moves to a different USB socket, and on macOS the act of opening a stream raises the microphone permission prompt. Something has to absorb all of that so the rest of the tool does not have to know it exists.

This package is that something, and keeping it separate is the point. Exactly one file names a malgo type, so replacing the library later means rewriting one file rather than hunting down every caller, and that containment can be checked by grepping for the import. A source is identified by its name alone, which survives reboots and moved cables when the library's own identifiers do not. And because the library is cgo, a companion file stands in when cgo is switched off, so cross compiling the rest of the tool keeps working and only the audio commands say they cannot help.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/audioin"

// Show what can be recorded from. Listing opens nothing,
// so it never raises a permission prompt.
sources, err := audioin.Sources()
if err != nil {
    return err
}
for _, s := range sources {
    fmt.Println(s.Name)
}

// Check a typed name and get its canonical spelling,
// still without opening anything.
name, err := audioin.Resolve("Cubilux CB5 Line In")
if err != nil {
    return err
}

// Open when somebody actually wants to listen. The pcm slice
// is only valid until the callback returns, so copy it out.
capture, err := audioin.Open(name, func(pcm []byte) {
    // Copy pcm somewhere before returning.
})
if err != nil {
    return err
}
defer capture.Close()
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **miniaudio** - The single-file C audio library underneath malgo, and the thing actually talking to the sound card here
- **cgo** - How Go calls C, and why this package needs a fallback file when it is switched off, as cross compiling does by default
- **Build constraints** - The `//go:build cgo` and `//go:build !cgo` tags that choose between the real implementation and the stub at compile time
- **PCM audio** - The raw sample format captures arrive in: interleaved signed 16-bit little-endian stereo at 48 kHz
- **macOS microphone permission** - Why enumerating devices is free but opening a stream prompts the user, which shapes the split between Sources, Resolve and Open
