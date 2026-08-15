# squelch

## What this does?
This package is the `squelch` command: it reports how strong a signal has to be before the scanner plays it, and its `set` subcommand changes that level. Squelch is the knob that keeps the radio quiet on an empty channel.

## Why we use it?
Squelch is the setting that decides whether a scanner is useful or unbearable. Set it too low and every empty channel hisses; set it too high and the quiet transmissions never open the audio at all. Finding the right level is a matter of trying one, listening, and trying another, which is tedious on a handheld and trivial from a terminal or a script that can walk the range.

Keeping reading and writing as two verbs is what makes this safe to script. The bare command only reads, so it carries the annotation that lets it run while another command holds the radio, and no reading can turn into a write by mistake. The `set` subcommand does the write, and it never trusts its own success: it sends the level, then reads the scanner back and reports what the scanner says it is holding, so a level the radio quietly declined shows up as a printed difference rather than as a silent success. The level is typed and checked before the serial port is ever opened, so a typo costs nothing and reports the same way whether or not a scanner is attached.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	squelch.New,
}
```

```bash
# What the scanner is holding now.
radiocli --device $SDS squelch

# Open it up, then quiet it down.
radiocli --device $SDS squelch set 0
radiocli --device $SDS squelch set 8

# The level and the bounds it is measured against, for a script.
radiocli --device $SDS squelch -o json
```

```json
{
  "level": 8,
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
- **Squelch** - The radio idea itself: a threshold that mutes the receiver until a signal is strong enough to be worth hearing
- **Cobra subcommands** - Reading and writing are two verbs on one command, which is how the tool keeps a read from becoming a write
- **Read-after-write verification** - The level is read back from the scanner rather than echoed, so a setting the radio declined cannot be reported as a success
- **Input validation at the boundary** - The level is parsed and range checked before the serial port opens, so a typo never reaches the hardware
- **Text and JSON output modes** - The same report serves a person and a script, with the bounds included so a level means something on its own
