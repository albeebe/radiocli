# recordings

## What this does?
Files transmissions on disk: the audio, with a description of what it is beside it. It decides what each recording is called and where it goes, and can scale a quiet recording up once the transmission has ended.

## Why we use it?
The obvious way to keep what a recording is of is to put it in the filename, and the software this feature was measured against does exactly that, with a printf-style template plus three text fields written inside the audio file itself. The result is that anything wanting the frequency back out has to parse a name it was never told the format of, which is why the one third-party tool that reads those recordings has the tag layout hardcoded and says so in its own documentation.

A JSON object beside the audio needs no such agreement: it carries its fields under names, it can gain a field later without breaking anything already reading it, and it can say that something is not known instead of leaving a blank that might mean either. Keeping this in its own package puts every decision about names in one place, and each of those decisions exists because of a way the other software fails. The template is checked when the destination is opened rather than on the first transmission, so a typo costs a second and not the night; an empty token takes its separator with it rather than leaving a row of underscores; a channel name is reduced to safe characters so `FIRE/EMS` cannot invent a directory; two recordings that would collide are numbered rather than one overwriting the other; and the path is shortened before it reaches the length Windows refuses, rather than after. The default template puts the date in a folder rather than only in the filename, so a new day starts a new folder without a setting the user has to know exists. Normalizing is a choice the caller makes: a line input applies no gain, so recordings arrive at whatever level the radio sent, and scaling them after the transmission ends is the only place left to fix it, with the entry carrying a flag so a level no longer means signal strength without saying so.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/recordings"

// Check the template before opening anything else, so a typo costs a second.
if err := recordings.ValidateTemplate(naming); err != nil {
    return err
}

// The last argument opts into normalizing: each recording's loudest sample
// is brought up to just under full scale once the transmission has ended,
// and the entry records that it happened.
library, err := recordings.New("./recordings", naming, false)
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
- **Audio normalization** - Scaling a recording so its loudest sample sits just under full scale, and what that costs in comparing one recording's level to another's
