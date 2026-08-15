# config

## What this does?
This package implements the `config` command, which reports and changes the settings the tool keeps for itself, such as how results are printed and how quickly keys are sent to the scanner. It also keeps the macros: named lists of command lines, stored in the config file for a front end to offer as one-press shortcuts.

## Why we use it?
Settings that live only in flags have to be typed again every time, and settings that live only in a file are edited with a text editor, which means a typo is discovered later by a different command that refuses to run. People need a way to see what is set, see what it would be if it were not set, and change one thing without touching the rest of the file. They also need to be told why a name they tried is not a setting, because "no setting called wait" is true and useless when the real answer is that waiting is a property of the caller and so is only ever a flag.

Keeping this as its own command puts every one of those answers in one place, and keeps it honest in a specific way: what it reports is what the config file holds, not what this particular run resolved. A run with `--output json` is asking for its answer as JSON, not asking to have JSON written into the file, and a command that confused the two would quietly save this run's flags as somebody's permanent settings. Every value is checked before it is written and read back afterwards, so a setting the rest of the tool would refuse cannot reach the file, and what is reported is what the file actually says rather than what was just claimed. Nothing here opens the serial port, which is the line that separates it from the rest of the tool: `volume` is how loud the scanner plays, `config set pace` is how this program behaves.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	config.New,
}
```

```bash
# What is set, and what it would be if it were not.
radiocli config

# One setting, alone on a line, so a script can read it.
radiocli config get pace

# Change one, and see it read back from the file.
radiocli config set pace medium

# Which file all of that is being read from and written to.
radiocli config path

# Store a named list of command lines as a macro.
radiocli config macro new "Night watch" "volume set 4" "backlight on" scan
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Configuration precedence** - Defaults, then the config file, then flags; this command reports and writes the middle one, deliberately not the resolved result
- **Read-after-write** - Every change is read back from the file before it is reported, because what was written is a claim and what the file holds is the answer
- **Validation before persistence** - A value is checked against the accepted list and against the tool's own validator before anything is stored, so a bad setting cannot be saved and then refused by every later command
- **Idempotent updates** - Writing one setting re-reads the file and changes only that key, so settings written earlier and macros stored beside them survive
- **Command line quoting** - Macro steps are checked with the same splitter that runs them later, so a line that would fail halfway through a macro is refused when it is saved
