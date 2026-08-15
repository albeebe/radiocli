# screen

## What this does?
This package is the `screen` command: it reads the scanner's display and reports it as text, one line per line, marking the row the scanner has highlighted. It is the plainest answer to "what does the radio say right now".

## Why we use it?
Every other command asks the scanner a question the protocol has a name for, and the protocol does not have a name for everything. A menu reply can leave out entries that are really on the screen and really in the knob's path, and a text input, the box the scanner puts up when it wants a name typed, carries no menu entries at all. When a command reports nothing, there is no way to tell a scanner sitting in an unexpected mode from a command that simply failed.

Reading the display closes that gap, because the display is the one thing the scanner is always willing to describe and the one thing that is always true. Keeping it as its own command gives every other command a way to be checked, gives a person driving the radio by hand something to look at between key presses, and gives a script a machine-readable copy of the screen with the highlight, the per-character attributes and the large font flag intact. The care in here is mostly about not losing any of that on the way out: the screen is bytes rather than text, and its font holds pictures above the ASCII range, such as the two halves of the signal meter, which JSON silently destroys unless each byte is widened to the character of the same value first.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	screen.New,
}
```

```bash
# The display as a person reads it, with "> " marking the highlighted row.
radiocli --device $SDS screen

# The same screen for a script, with the attributes and the large font flag.
radiocli --device $SDS screen -o json
```

```json
{
  "lines": [
    { "text": "SCAN", "highlighted": false, "largeFont": false },
    { "text": "GREENDALE", "highlighted": true, "attributes": "**********", "largeFont": true }
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, which is how every command in this tool is wired into the tree
- **UTF-8 and code points** - The screen is bytes, not text, so anything above 0x7E has to be widened to a code point before JSON can carry it
- **Reverse video and character attributes** - The scanner marks its selection by drawing characters inverted, which is what `highlighted` and `attributes` report
- **Dependency injection** - The command receives the App rather than opening a serial port, which is what lets a test drive it against a fake connection
- **Text and JSON output modes** - One command serves a person and a script by rendering the same report two ways, chosen by `-o`
