# systems

## What this does?
This package is the `systems` command: it lists the systems inside one favorites list and says which of them the scanner is skipping. It also opens a system's menu on the radio, and creates, renames, or deletes systems.

## Why we use it?
A system is the second level of the scanner's memory: a favorites list holds systems, a system holds departments and sites, and those hold the channels that actually get heard. Reading that level is a single exchange, but changing it is not. The scanner offers no command for creating, naming, or deleting a system, so each of those is a walk through its menus, and the walk for creating one has a step that cannot be taken back: the scanner asks for the system's type before the system exists, and a type cannot be changed afterwards. Getting it wrong means deleting the system and starting again.

Keeping this as its own command puts that walk in one place, along with the rules that make it safe. The type must be given rather than defaulted, and what was typed is matched against the types the scanner really offers, so case and spacing do not have to be reproduced exactly. Deleting takes everything inside the system with it, so it requires `--yes` and reads the scanner back afterwards to confirm the system is gone. An empty answer gets an extra look, because a favorites list that holds nothing and an index that names no list answer identically, and telling somebody their list is empty when they actually mistyped the index sends them looking in the wrong place.

Naming the built-in full database is refused rather than sent. Asking it for its systems returns a short, wrong answer and then leaves the scanner's serial interface dead until it is power cycled, reproduced twice. The refusal lives in [catalog](../../catalog/), where the request would be issued, so it covers the reserved index as well as the name and cannot be stepped around by a future caller here. `scanning systems` reads the database by turning the knob instead.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	systems.New,
}
```

```bash
# List the systems in one favorites list, by index or by name.
radiocli --device $SDS systems HOME

# Create a system, which needs its type, then rename it.
radiocli --device $SDS systems new HOME AIRPORT --type conventional
radiocli --device $SDS systems rename AIRPORT "AIRPORT OPS"

# Open a system's menu on the radio, or remove it and everything in it.
radiocli --device $SDS systems goto "AIRPORT OPS"
radiocli --device $SDS systems delete "AIRPORT OPS" --yes
```

Creating, renaming and deleting a system prints a line of text, and under `-o json` prints a `render.Mutation` instead: an object carrying `action`, `kind`, `name`, and `was` and `in` where they apply. The same shape covers those verbs at every level of the scanner's memory, so a script driving edits does not learn a different object for each. The text output is unchanged, because it is what people already read; the JSON mode was added beside it rather than in place of it.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Favorites lists** - The level above a system, which is why a system has to be named through the list holding it before it can be read
- **System types** - P25, Motorola, EDACS, LTR and conventional describe how a system is organised, and the choice cannot be changed after the system exists
- **Index resolution** - A system's index carries no hint of which list holds it, so a name costs a read of every list before anything can be pressed
- **Read-after-write** - Every change is confirmed by reading the scanner back, because a menu press that appeared to work is not evidence that anything changed
- **Ambiguous empty answers** - A list holding nothing and an index naming no list look identical over the protocol, which is why the empty case is checked rather than reported
