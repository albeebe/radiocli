# menu

## What this does?
This package is the `menu` command: it reports which of the scanner's menus is on screen and what is in it, and its subcommands move around inside those menus. It is how the radio's own interface is driven from a terminal when no purpose-built command exists for what you want.

## Why we use it?
Most of what this tool does is wrapped in a command that names the thing it changes, but the scanner has settings with no protocol command behind them at all, and the only way to reach those is the menu system the operator would use. Doing that by hand means opening a menu, reading what is on screen, climbing back out, and remembering to close it, because opening a menu takes the scanner out of scanning and most other commands are refused while it is in one.

Keeping this as its own command gives that low-level access a safe shape. Reading is the bare command and changes nothing, so it is annotated as read-only and may run while another command holds the radio. Everything that drives the scanner is a separate verb rather than a flag, so `open`, `back`, `close` and `set` each say plainly that they are moving the radio. Every verb that moves the scanner reports where it landed afterwards, so a menu that opened somewhere unexpected is visible without a second command, and a menu named by a word this tool does not know is refused with the list of names it does know rather than guessed at.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	menu.New,
}
```

```bash
# What menu is on screen, and what is in it.
radiocli --device $SDS menu

# Open one, move around, and set the value of the item the knob is on.
radiocli --device $SDS menu open settings
radiocli --device $SDS menu back
radiocli --device $SDS menu set "Fire Dispatch"

# Put the scanner back to scanning when finished.
radiocli --device $SDS menu close
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `open`, `back`, `close` and `set` attached as subcommands, which is how every command in this tool is wired into the tree
- **Read-only annotations** - `appcontext.OnlyReads` marks the bare command as safe to run while another command holds the scanner, because reporting a menu presses no keys
- **Menu ids and indexes** - A menu is opened by name, resolved to the protocol's own id, with an optional index choosing which system, department, site, channel or bank it opens on
- **Modal devices** - The scanner stops scanning while it is in a menu and refuses most other commands, which is why closing is its own verb and worth remembering
- **Screen scraping versus protocol listings** - Which entry is highlighted is read off the display rather than taken from the listing, because the listing can omit rows and then name the wrong one as selected
