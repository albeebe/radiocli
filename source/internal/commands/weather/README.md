# weather

## What this does?
This package is the `weather` command: it puts the scanner on the NOAA weather channels, measures all seven, and parks it on the one with the strongest signal. Its `stop` subcommand is the way back to ordinary scanning.

## Why we use it?
Left to itself the scanner stops on the first weather channel that opens its squelch, which is not the strongest and is sometimes not receivable at all. It was seen parked on channel 2 at -108 dBm, silent, while channel 7 was audible at -97 dBm the whole time. There is also a trap in getting there: the scanner does two different things on these channels and calls both of them "WX Scan". Monitor Weather plays the broadcast, Weather Alert sits silent and unmutes only for an alert tone, and nothing in the mode or the screen name tells them apart. The protocol's own jump to weather starts the silent one.

Keeping this as its own command is what turns those traps into one line that works. It walks the menu that starts the mode which actually plays, then reads the scanner back to confirm which one it landed in, because the failure worth naming is the one that looks like success and stays quiet. It holds the scanner before measuring, since an unheld radio moves under the sweep and every reading gets credited to the wrong channel, reads each channel several times and keeps the strongest, and treats the scanner's -999 as "nothing heard" rather than as a very weak signal. It prints what it found on all seven, so the choice can be checked rather than trusted. Coming back is its own subcommand because `radiocli scan` cannot do it: a scanner on the weather channels is not in a menu and is not holding in the sense that command means, so it finds nothing to do.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	weather.New,
}
```

```bash
# Measure all seven channels and hold the best one.
radiocli --device $SDS weather

# Come back to ordinary scanning. "scan" will not do this.
radiocli --device $SDS weather stop

# The same reading for a script.
radiocli --device $SDS weather -o json
```

```json
{
  "scanning": true,
  "mode": "Monitor Weather",
  "receiving": true,
  "channel": "7",
  "frequency": "162.525000MHz",
  "signal": -97,
  "channels": [
    { "number": "1", "frequency": "162.400000MHz", "selected": false },
    { "number": "7", "frequency": "162.525000MHz", "signal": -97, "selected": true }
  ]
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `stop` attached as a subcommand, which is how every command in this tool is wired into the tree
- **NOAA weather radio** - Seven channels between 162.400 and 162.550 MHz carry the same service from different transmitters, which is why picking the strongest one matters more than finding any
- **RSSI and sentinel values** - The scanner answers -999 when it hears nothing, and treating that as a reading rather than an absence is how a dead channel wins a comparison
- **Settling and polling** - The scanner keeps reporting the state it is leaving until it has redrawn, so every change is confirmed by a bounded loop of reads rather than one read and a guess
- **Menu walking** - The mode that plays has no protocol command, so it is started by selecting an entry the protocol's own listing omits, read off the display instead
