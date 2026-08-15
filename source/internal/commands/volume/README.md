# volume

## What this does?
This package is the `volume` command: it reports how loud the scanner is set to play, and its `set` subcommand changes that level. It is the volume knob, reachable from a terminal.

## Why we use it?
The scanner's volume is a knob on top of the radio, which is fine when the radio is in your hand and useless when it is on a shelf feeding audio into a computer. Anything recording or streaming the scanner needs the level set once and left alone, and anything automated needs to be able to quiet it without somebody walking over to it. Reading the level matters as much as setting it, because a recording that came out silent has two explanations and only one of them is the volume.

Keeping reading and writing as two verbs is what makes this safe to script. The bare command only reads, so it carries the annotation that lets it run while another command holds the radio, and no reading can turn into a write by mistake. The `set` subcommand does the write, and it never trusts its own success: it sends the level, then reads the scanner back and reports what the scanner says it is playing at, so a level the radio quietly declined shows up as a printed difference rather than as a silent success. The level is typed and checked before the serial port is ever opened, so a typo costs nothing and reports the same way whether or not a scanner is attached.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	volume.New,
}
```

```bash
# What the scanner is playing at now.
radiocli --device $SDS volume

# Turn it up, then silence it.
radiocli --device $SDS volume set 10
radiocli --device $SDS volume set 0

# The level and the bounds it is measured against, for a script.
radiocli --device $SDS volume -o json
```

```json
{
  "level": 10,
  "min": 0,
  "max": 15
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
- **Cobra subcommands** - Reading and writing are two verbs on one command, which is how the tool keeps a read from becoming a write
- **Read-after-write verification** - The level is read back from the scanner rather than echoed, so a setting the radio declined cannot be reported as a success
- **Input validation at the boundary** - The level is parsed and range checked before the serial port opens, so a typo never reaches the hardware
- **Dependency injection** - The command receives the App rather than opening a serial port, which is what lets a test drive it against a fake connection
- **Text and JSON output modes** - The same report serves a person and a script, with the bounds included so a level means something on its own
