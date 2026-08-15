# scan

## What this does?
This package is the `scan` command: it takes the scanner out of whatever menu or held state it has been left in and returns it to scanning. It is the one command to reach for when the radio is sitting somewhere it should not be.

## Why we use it?
Getting out of the scanner's menus is not one reliable command. The protocol's own way out is refused on several screens, and the key that always works is the same key that avoids the current channel when the scanner is not in a menu, so pressing it blind either fixes the problem or quietly removes a channel from the scan. Leaving the menus is not the whole job either: a scanner can be out of them and still not scanning, holding one frequency after `tune`, holding one channel because somebody turned the knob, monitoring a weather channel, or sweeping a range of its own after `banks scan`. None of those looks like a menu and none of them says so plainly on screen.

Keeping this as its own command means every other command can leave the scanner somewhere odd and there is still one thing to run afterwards that reliably puts it right. It checks where the scanner is before every press, so the avoid key is never pressed on a scanner that is not in a menu, and it stops the moment the radio is out. It asks after each of the not-scanning states in turn, deliberately leaving the custom search check until last, once nothing else is going to move the scanner again. It reports what it did in plain words rather than silently, and running it on a scanner that is already scanning does nothing at all.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	scan.New,
}
```

```bash
# Put the scanner back to scanning, from wherever it was left.
radiocli --device $SDS scan

# The usual pairing: something that parks the radio, then this.
radiocli --device $SDS tune 162.550
radiocli --device $SDS scan
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
- **Overloaded keys** - The key that leaves a menu is the key that avoids a channel when no menu is open, which is why the scanner's state is read before every press
- **Polling for confirmation** - The scanner keeps reporting the mode it is leaving until it has redrawn, so the wait is a bounded loop of looks rather than one read and a guess
- **Idempotent commands** - Running this against a scanner that is already scanning changes nothing, which is what makes it safe to put at the end of any script
- **Modal devices** - Menus, holds, quick search, the weather channels and custom search are all distinct states, and only asking after each of them makes "returned to scanning" a promise this command can keep
