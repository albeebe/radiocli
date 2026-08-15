# tune

## What this does?
This package is the `tune` command: it puts the scanner straight onto one frequency, holds it there, and says whether anything is coming in. It is the quickest way to listen to something without adding it to the radio's memory.

## Why we use it?
Listening to one frequency on a scanner normally means storing it first, which is several minutes of knob work for something you may want for five minutes. The radio can be told to sit on a frequency directly, and a command that does it in one line is the difference between trying a frequency somebody mentioned and not bothering. Reading the frequency in whatever form it was written helps too, since the same channel is quoted as `155.475`, `155.475MHz`, `155475kHz` and `155475000Hz` depending on who wrote it down, and all four mean the same thing.

Most of this package is about turning the scanner's silence into an answer. The radio refuses a frequency it cannot reach and a frequency it is too busy to accept with exactly the same reply, and it gives no reason for either, so the package checks the coverage itself before sending anything: it carries the five spans an SDS150 actually accepts, measured by sending frequencies either side of every edge, plus the two cellular bands that are blocked in scanners sold in the United States and can be mistaken for a gap in the hardware. When the scanner still refuses, the package asks what it is doing and names the screen it is stuck on. And because the radio does not measure a new frequency instantly, the command waits for a reading rather than taking the first one, so a busy frequency is never reported as silent.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	tune.New,
}
```

```bash
# Megahertz unless a unit says otherwise. These three are the same frequency.
radiocli --device $SDS tune 155.475
radiocli --device $SDS tune 155475kHz
radiocli --device $SDS tune 155475000Hz

# What it landed on, and what it heard, for a script.
radiocli --device $SDS tune 162.550 -o json

# Back to scanning afterwards.
radiocli --device $SDS scan
```

```json
{
  "megahertz": 162.55,
  "receiving": true,
  "rssi": "-072",
  "bars": "4"
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
- **Quick search hold** - The scanner mode this command drives, where the radio parks on one frequency instead of sweeping a list
- **Receiver coverage and band edges** - The scanner accepts five spans rather than one continuous range, which is why a frequency is checked before it is sent
- **Cellular band blocking** - The two gaps in the middle of the coverage are a legal requirement in scanners sold in the United States, not a hardware limit
- **Polling with a settle window** - A freshly tuned radio reports no signal for a few hundred milliseconds, so the reading is waited for rather than taken immediately
- **RSSI and signal bars** - Two views of the same measurement, one raw and one rounded to something a person can act on
