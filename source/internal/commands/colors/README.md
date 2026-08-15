# colors

## What this does?
This package is the `colors` command: it reports, changes and restores the text and background color of every area of the scanner's screen. It also says where each area sits, so any character position on the display can be traced back to the color it is drawn in.

## Why we use it?
The scanner draws its screen from seven layouts, one per kind of screen, and each layout is a list of areas with a text color and a background color of its own. None of that is in the remote protocol. The only way to learn a color is to walk the menus that set it, opening each area's two color pickers in turn and reading the value off the screen, which takes about half a minute per layout and stops the scan while it runs. Anything that wants to draw the radio's screen faithfully needs all of it, and a layout nobody has read is drawn white on black while the radio in your hand is in color.

Keeping this as its own package is what makes that walk affordable and safe to repeat. Every reading is cached on disk per scanner, so `--cache` answers instantly and a color changed by `set` amends the stored copy rather than leaving it stale. Two things that cannot change are built in rather than read: where each area sits, since no menu moves an area, and which colors the pickers offer, since that list belongs to the firmware. Both tables would be silently wrong on a firmware that differed, so both come with a check against the radio in hand, `--verify-positions` and `--verify-palette`. The walks themselves are written to fail rather than guess: a read never confirms a picker, and a write reads the color back off the screen instead of trusting the key press.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	colors.New,
}
```

```bash
# The colors of whichever layout the scanner is drawing with right now.
radiocli --device $SDS colors

# The last reading of one layout, straight off the disk, opening no menus.
radiocli --device $SDS colors weather --cache

# Where each area sits, with no colors and no menu walk, which is instant.
radiocli --device $SDS colors --positions -o json

# Change one area, then put the whole screen back to how it left the factory.
radiocli --device $SDS colors set System_name --text Yellow --back Black
radiocli --device $SDS colors reset
```

```json
{
  "layout": "weather",
  "menu": "Set Weather Mode",
  "current": true,
  "areas": [
    {
      "area": "System_name",
      "text": "Yellow",
      "background": "Black",
      "textHex": "#FFFF00",
      "backgroundHex": "#000000",
      "line": 4,
      "column": 0,
      "length": 24,
      "height": 1
    }
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `set`, `reset` and `palette` attached to it as subcommands
- **Caching and invalidation** - A reading is expensive and rarely changes, so it is stored per scanner and amended, dropped or re-read as commands change what it describes
- **Screen scraping** - The colors are not in the protocol, so they are read off the display and out of the menu titles rather than asked for
- **Reverse video and character attributes** - The editor marks the selected area by drawing it inverted, which is how the built-in position map was measured and is checked
- **Idempotent writes** - Setting a color to the one it already is presses nothing, and every write is read back rather than assumed to have taken
