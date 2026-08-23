# recordings

## What this does?
Files transmissions on disk: the audio, with a description of what it is beside it. It decides what each recording is called and where it goes.

## Why we use it?
The obvious way to keep what a recording is of is to put it in the filename, and the software this feature was measured against does exactly that, with a printf-style template plus three text fields written inside the audio file itself. The result is that anything wanting the frequency back out has to parse a name it was never told the format of, which is why the one third-party tool that reads those recordings has the tag layout hardcoded and says so in its own documentation.

A JSON object beside the audio needs no such agreement. It carries its fields under names, it can gain a field later without breaking anything already reading it, and it can say that something is not known instead of leaving a blank that might mean either. The filename is then free to be for people, which is what a filename is actually good at.

The naming is a template, and it is checked when the destination is opened rather than when the first transmission arrives, because a mistyped token found at startup costs a second and the same typo found on the first recording of the night costs the night. Everything else about the template exists because of a way the other software fails: an empty token takes its separator with it rather than leaving a row of underscores; a channel name is reduced to safe characters so `FIRE/EMS` cannot invent a directory; two recordings that would collide are numbered rather than one overwriting the other; and the path is shortened before it reaches the length Windows refuses, rather than after. The default puts the date in a folder, which is not decoration either: a scheme that puts the date only in the filename does not start a new folder at midnight, and the fix is a setting the user has to know exists.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/recordings"

// Check the template before opening anything else, so a typo costs a second.
if err := recordings.ValidateTemplate(naming); err != nil {
    return err
}

library, err := recordings.New("./recordings", naming)
if err != nil {
    return err
}

// One recording per transmission. The name is not known yet, because it
// carries the channel and the duration, so the audio goes to a hidden
// temporary file and is moved into place when it is closed.
r, err := library.Begin()
if err != nil {
    return err
}
for _, frame := range transmission {
    if err := r.Write(frame.PCM); err != nil {
        return err
    }
}

// Closing names it and writes the description beside it. Duration and File
// are filled in here.
filed, err := r.Close(recordings.Entry{
    Start:   start,
    End:     end,
    System:  "Pocahontas County",
    Channel: "Marlinton Dispatch",
    Samples: 14,
})
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Sidecar files** - Keeping metadata beside the thing rather than inside it, and why that beats tags a reader has to be told the layout of
- **Path length limits** - The 260 character ceiling Windows enforces by default, and why a naming scheme has to shorten rather than discover it
- **Path traversal** - Why a name programmed into somebody's scanner is untrusted input the moment it is used to build a path
- **Atomic rename** - Writing to a temporary name and moving it into place, so a reader never finds a file that is still being written
