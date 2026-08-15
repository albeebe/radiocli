# beep

## What this does?
This package is the `beep` command: it reports the sound the scanner makes when a key is pressed, and changes it. It also has a toggle that silences the keypad and puts the old setting back the next time it is run.

## Why we use it?
The key beep is not in the scanner's remote protocol. It lives in a menu as a list of seventeen values, `auto`, `1` through `15`, and `off`, so there is no message to send that asks what it is. Even reading it means walking the radio into Settings, finding the row that is highlighted, and walking back out, which stops the scan for a moment. Somebody doing that by hand on the radio has to hold the list in their head and count rows.

Keeping this as its own command turns that walk into one line, and puts the two dangerous parts of it somewhere they can be reasoned about. Reading is the bare command and writing is a separate verb, so no reading of the setting can turn into a write by mistake. A value typed on the command line is checked before the scanner is opened, so a mistake costs nothing. Every change is read back off the radio rather than trusted, because a key press is not proof that the setting moved. The toggle earns its place separately: switching the keypad off is what destroys the answer, since the scanner keeps one value and nothing on the radio remembers the last one, so this command writes the old setting to a small cache file and is the one thing the tool stores that it cannot get back by asking the scanner.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	beep.New,
}
```

```bash
# The setting as a person reads it.
radiocli --device $SDS beep

# Change it, then read it back off the radio.
radiocli --device $SDS beep set 9

# Silence the keypad, and put it back the next time this runs.
radiocli --device $SDS beep toggle

# The same reading for a script.
radiocli --device $SDS beep -o json
```

```json
{
  "level": "off",
  "on": false,
  "remembered": "9"
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `set` and `toggle` attached as subcommands, which is how every command in this tool is wired into the tree
- **Menu walking** - The setting is only reachable by driving the scanner's own menus, so reading it costs scan time and has to be left in a known place afterwards
- **Read-after-write** - The scanner is the authority on its own setting, so a change is confirmed by reading it back rather than by the key press appearing to succeed
- **Cache directories** - The remembered level is written under the user's cache directory, keyed per scanner, because losing it has a defined and harmless answer
- **Text and JSON output modes** - One command serves a person and a script by rendering the same report two ways, chosen by `-o`
