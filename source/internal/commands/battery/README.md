# battery

## What this does?
This package is the `battery` command: it reports how much charge the scanner has left, whether it is charging, and the condition of the battery underneath that number. It is the answer to "how long have I got, and is this pack still healthy".

## Why we use it?
A percentage on a screen is the least useful thing a battery can tell you. The scanner knows far more than it shows: the voltage of the pack, the current going in or out in milliamps, its temperature, and what the charging circuit itself is doing, including whether it has stopped on a fault. A radio left on a charger that is refusing to charge looks exactly like a radio that is charging, right up until it is needed and flat.

Keeping this as its own command turns those raw numbers into a reading somebody can act on. The charger state arrives as a bare code, so it is reported in words. The voltage arrives in millivolts, so it is reported in volts. The temperature arrives in Celsius alone, so both scales are shown. The sign of the current is the difference between charging and discharging and is easy to miss in a row of digits, so the direction is spelled out beside it, and a fault, the one reading here that asks somebody to do something, is pulled out of the numbers and said plainly.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	battery.New,
}
```

```bash
# The battery as a person reads it.
radiocli --device $SDS battery

# The same reading for a script.
radiocli --device $SDS battery -o json
```

```json
{
  "state": "charging",
  "charging": true,
  "percent": 23,
  "volts": 3.73,
  "milliamps": 643,
  "celsius": 36.5,
  "fahrenheit": 97.7,
  "needsAction": false
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, which is how every command in this tool is wired into the tree
- **Lithium-ion charge states** - The charger reports what it is doing as a code, including the two faults that mean charging has stopped rather than progressed
- **Unit conversion at the edges** - Millivolts and Celsius are what the radio sends, and volts and Fahrenheit are what a person reads, so the conversion belongs in the command rather than the protocol
- **Dependency injection** - The command receives the App rather than opening a serial port, which is what lets a test drive it against a fake connection
- **Text and JSON output modes** - One command serves a person and a script by rendering the same report two ways, chosen by `-o`
