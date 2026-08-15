# menus

## What this does?
Drives the scanner's on-screen menus: opening one, reading what it holds, moving to an entry by name, choosing it, and getting back out to scanning afterwards. It is the shared set of moves that every command needing to change a setting on the radio is built from.

## Why we use it?
The scanner has no command for "change this setting". Everything below the top level of the menus is reached the way a person reaches it, by turning the knob and pressing keys, and the only feedback is the display. That would be manageable if the radio's own menu listing could be trusted, but it cannot: the listing leaves out entries that are really on screen and really in the knob's path, and it declines to give some lists at all. Counting positions from a listing that is missing rows lands a command one entry off, and on several of these menus the entry next to the one wanted is Delete.

Keeping this in its own package matters because every command that walks a menu has to walk it the same way, and the safe way is not the obvious one. Entries are matched by name read from the display rather than counted, truncated rows are compared as prefixes only when the scanner marked them as truncated, presses are held back until the radio is finished with the last slow thing it was asked to do, and leaving the menus puts the scanner back to scanning rather than abandoning it on a half-finished screen. Each of those exists because of a specific way the radio bites, and a copy of this logic in every command is a copy of it that will drift, in the part of the tool where drifting means deleting the wrong channel.

## How we use it?
```go
import (
	"context"

	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
)

func setBeepOff(ctx context.Context, client *device.Scanner) error {
	// Open the settings menu, step to an entry by name, and press it.
	if err := client.OpenMenu(ctx, device.MenuSettings, ""); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, "Set Beep"); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, "Off"); err != nil {
		return err
	}

	// Always leave the menus, which also returns the scanner to scanning.
	_, err := menus.Leave(ctx, client)
	return err
}
```

Use `menus.Show` to print the menu the scanner is on, `menus.Lookup` and `menus.Names` for the menu names this tool accepts, and `menus.Entries` to read a menu the protocol will not report.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Screen scraping** - Why the highlighted entry is read from the display rather than taken from the scanner's own menu listing, which omits rows
- **Truncation** - The control bytes the scanner marks a shortened name with, and why a row that was not shortened must never be matched as a prefix
- **Polling** - The pattern behind Settle, Resume and Awaken: ask repeatedly on a short gap rather than sleep for a guessed duration
- **Budgets in time, not tries** - Why Awaken counts seconds rather than attempts, and why Settle is not called Ready
- **Idempotence** - Why Leave presses keys until the scanner says it is out, instead of pressing a fixed number of times
- **Dependency injection** - What the poll bounds here would need to become before these waits could be exercised in a unit test
