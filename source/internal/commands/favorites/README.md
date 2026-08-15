# favorites

## What this does?
This package is the `favorites` command: it lists the scanner's favorites lists and says which of them are being scanned. Its subcommands create a list, rename one, delete one, open one on the scanner's own screen, and choose which ones the radio scans.

## Why we use it?
A favorites list is the top of the scanner's memory. It holds systems, which hold departments, which hold channels, so everything the radio listens to hangs off one of these. The protocol will happily report the lists, and then stops being helpful: it has no command to create a list, none to rename one, none to delete one, and none to say which lists are scanned. Those are things a person does by holding the radio and turning its knob, which is exactly what somebody with the scanner on a shelf cannot do.

This package does them the way a person would, and that is why it is worth a package of its own. It walks the scanner's menus, matching each step against what is actually on the display by name rather than by counting positions, because on a favorites list's own menu the entry next to Rename is Delete and landing one entry past would be unrecoverable. Every change is read back from the scanner afterwards, since the radio is the only authority on what it holds: a list that does not appear after being created, or one still present after being deleted, is reported rather than assumed. The destructive subcommand refuses to act without `--yes`, and a built-in scan source such as the full RadioReference database is refused outright, because the scanner has no way to remove one.

## How we use it?
```go
// main lists the command's constructor, and the tool builds its command tree
// from that list.
var commands = []func(*appcontext.App) *cobra.Command{
	favorites.New,
}
```

```bash
# What lists the scanner holds, and which of them it is scanning.
radiocli --device $SDS favorites

# Create one, rename it, and put the scanner on its menu.
radiocli --device $SDS favorites new "TEST LIST"
radiocli --device $SDS favorites rename "TEST LIST" "LOCAL FIRE"
radiocli --device $SDS favorites goto "LOCAL FIRE"

# Scan exactly these and nothing else, then hand the result to a script.
radiocli --device $SDS favorites scan "LOCAL FIRE"
radiocli --device $SDS favorites -o json

# There is no undo, so this is refused without the flag.
radiocli --device $SDS favorites delete "LOCAL FIRE" --yes
```

```json
[
  {
    "name": "LOCAL FIRE",
    "index": "0",
    "monitored": true,
    "quickKey": "1",
    "builtIn": false
  }
]
```

Creating, renaming and deleting a favorites list prints a line of text, and under `-o json` prints a `render.Mutation` instead: an object carrying `action`, `kind`, `name`, and `was` and `in` where they apply. The same shape covers those verbs at every level of the scanner's memory, so a script driving edits does not learn a different object for each. The text output is unchanged, because it is what people already read; the JSON mode was added beside it rather than in place of it.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Menu walking by name** - Each step is checked against the display rather than counted, which is the only thing keeping a walk off the Delete entry sitting next to Rename
- **Read-after-write verification** - The lists are read back from the scanner after every change, because the radio is the only authority on what it actually holds
- **Destructive command confirmation** - Deleting a list removes every system, department and channel inside it and cannot be undone, so `--yes` is required before anything is pressed
- **Truncated display matching** - The screen cuts long names and marks that it did, and only a row marked as cut may be matched as the start of a longer name
- **Dependency injection** - The command receives the App rather than opening a serial port, which is what lets the tests drive every walk against a fake connection
