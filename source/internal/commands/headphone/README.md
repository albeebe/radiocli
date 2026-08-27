# headphone

## What this does?
This package is the `headphone` command: it reports whether the scanner sends the same audio to both sides of its headphone jack or one of them inverted, and its `set` subcommand changes it.

## Why we use it?
The jack on this scanner is wired out of phase, and the setting exists to correct it. Uniden addressed the fault in firmware rather than in hardware, by adding a menu to flip one side, which means whether a given radio has the problem depends entirely on which way its owner has left that menu, and there was no way to find out without picking the radio up and walking the menus by hand. The symptom does not look like a phase problem, which is what makes it hard to place. Anything that combines the two sides of the jack into one signal, which is what taking mono off a stereo input means, gets audio around eleven decibels too quiet with the body taken out of the voice: the low frequencies are the most alike between the two sides and cancel the most completely, so what survives is thin and reedy, like the speaker is talking through a kazoo. Measured on an SDS150, folding the two sides moved the energy in a voice from 67% below 1 kHz to 13%, and the natural places to lay the blame are the radio, the cable, or a digital voice codec, none of which are at fault. The measurements are written up in [research/oddities.md](../../../../research/oddities.md).

Recording does not depend on this: `audio record` measures what folding the two sides actually produces and takes a single side when folding would destroy the signal, so it copes with either setting without being told which one the radio is on. This command is for fixing it at the source instead, which is better where it is possible, because it is then right for everything the radio is ever plugged into rather than for this tool alone. The setting is not in the scanner's remote protocol. It lives in a menu, so even reading it walks into Settings and back out, which stops the scan for a moment. That is why nothing about it happens automatically, and why this command does not carry `appcontext.OnlyReads`.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	headphone.New,
}
```

```bash
# Which way is this radio wired?
radiocli --device $SDS headphone

# The same reading for a script.
radiocli --device $SDS headphone -o json

# Fix it at the source.
radiocli --device $SDS headphone set in-phase
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `set` attached as a subcommand, which is how every command in this tool is wired into the tree
- **Absolute and relative polarity** - Why two copies of the same signal cancel when one is inverted, and why the low frequencies go first
- **Menu-backed settings** - The read-select-read-back walk this shares with `beep`, and why the setting is read back off the radio rather than assumed from the press
- **appcontext.OnlyReads** - The annotation this command deliberately does not carry, because walking a menu takes the scanner off the air
- **audiofeed's channel chooser** - The other half of the answer, which measures the fold rather than trusting the setting, and so covers a cable that inverts a side as well
