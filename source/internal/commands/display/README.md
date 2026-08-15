# display

## What this does?
This package is the `display` command: it reports whether the scanner is drawing its screen in color or in one of its two black and white modes, and switches between them. It answers the question anything redrawing the scanner's screen has to ask before it picks a palette.

## Why we use it?
The scanner stores a text color and a background color for every element of every screen layout, and one setting decides whether any of that is drawn. In either black and white mode the colors are still stored and still editable, and completely ignored. Software that paints the scanner's screen somewhere else, on a computer or in a web page, will show a color display while the radio in somebody's hand is monochrome unless it reads this first.

Keeping this as its own command is worth it because reading and writing this setting cost wildly different things. The read is a single command over the wire and works while the scanner is scanning, so it is annotated as read-only and may run while another command holds the radio. The write has no protocol command at all: the setting sits behind Display Options, which the protocol has no menu id for, so it has to be walked with key presses and the scan stops for as long as that takes. Asking for the mode the scanner is already in therefore does nothing and never opens a menu, and every change is read back afterwards rather than trusted, because the walk selects entries by name off a screen the scanner redraws as it likes.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	display.New,
}
```

```bash
# Which mode is it in.
radiocli --device $SDS display

# Change it. This one walks the menus and stops the scan.
radiocli --device $SDS display mode black

# The same reading for a script.
radiocli --device $SDS display -o json
```

```json
{
  "mode": "color",
  "color": true,
  "entry": "Color Mode"
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `mode` attached as a subcommand, which is how every command in this tool is wired into the tree
- **Read-only annotations** - `appcontext.OnlyReads` marks the bare command as safe to run while another command holds the scanner, because reading this setting presses no keys
- **Menu walking** - The write has no protocol command, so it is driven through the scanner's own menus by selecting entries by name, which stops the scan
- **Read-after-write** - The mode is read back off the radio to confirm the change took, rather than trusting that the key presses landed where they were aimed
- **Text and JSON output modes** - One command serves a person and a script by rendering the same report two ways, chosen by `-o`
