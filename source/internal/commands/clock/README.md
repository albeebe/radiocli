# clock

## What this does?
This package is the `clock` command: it reports the date and time the scanner is keeping, and its `set` and `sync` subcommands change them. It also says whether the scanner's clock is still running and how far it has drifted from this computer.

## Why we use it?
The scanner keeps its own clock, and that clock is not decoration. It stamps recordings and it drifts, and a scanner left without power long enough loses it entirely and reports a date that means nothing. Nothing on the radio's own screen says the clock has stopped, so a wrong time looks exactly like a right one until something depends on it.

The subtlety this package exists to hold is the daylight saving flag. On this model the flag is not a label describing the time already written: the scanner adds it to the offset it is already configured with, so a scanner set from a computer that is itself on summer time ends up an hour fast, and syncing again does not fix it because the same hour is added every time. The flag the scanner already holds is therefore carried over unless the user asks for a change, the write is read back rather than assumed, and a drift of almost exactly an hour on a scanner applying daylight saving is explained rather than met with another suggestion to sync. Splitting the reads from the writes into separate verbs is the other half of the care: no reading of the clock can turn into a write by accident.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	clock.New,
}
```

```bash
# What the scanner is keeping, and how far it has drifted.
radiocli --device $SDS clock

# Set it from this computer, which is the usual repair.
radiocli --device $SDS clock sync

# Give it a date and time of your own, or just one half of one.
radiocli --device $SDS clock set "2026-08-02 14:30:00"
radiocli --device $SDS clock set 2026-08-02
radiocli --device $SDS clock set 14:30

# Correct a scanner running an hour fast on daylight saving.
radiocli --device $SDS clock sync --dst=false
```

```json
{
  "date": "2026-08-02",
  "time": "14:30:00",
  "daylightSaving": false,
  "valid": true
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
- **Cobra subcommands** - Reading and writing are separate verbs on one command, which is what keeps a query from turning into a change
- **Wall clock time versus an instant** - The scanner holds digits rather than a moment, so values are parsed and formatted in the local zone rather than converted
- **Daylight saving offsets** - The flag is added to the scanner's configured offset rather than describing the time already set, which is why it is carried over instead of derived
- **Read-after-write verification** - The clock is read back rather than echoed, so a value the scanner quietly declined is not reported as a success
- **Partial input and defaulting** - A date alone or a time alone is completed from the scanner's own reading, so changing one half never disturbs the other
