# departments

## What this does?
This package is the `departments` command: it lists the departments inside one system and says which of them the scanner is skipping. It also opens a department's menu on the radio, and creates, renames, or deletes departments.

## Why we use it?
A department is the third level of the scanner's memory: a favorites list holds systems, a system holds departments, and a department holds the channels that actually get heard. It is as far down as this tool goes, because the request that would return the channels themselves answers with the wrong document on the tested firmware. Reading a system's departments is a single exchange, but naming one by name is the most expensive lookup here, since a department's index carries no hint of which system or favorites list holds it and every one of them has to be read to find it.

Keeping this as its own command puts that lookup and the walks that depend on it in one place. The scanner offers no command for creating, naming, or deleting a department, so each of those is a walk through its menus: creating one brings the department into existence the moment the entry is pressed, under a name of the scanner's choosing, and the name asked for is then typed over it. Deleting takes every channel inside with it and cannot be undone, so it requires `--yes` and reads the scanner back afterwards to confirm the department is gone. An empty answer gets a second look as well, because a system holding nothing and an index naming no system answer identically over the protocol, and telling somebody their system is empty when they mistyped the index sends them looking in the wrong place.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	departments.New,
}
```

```bash
# List the departments in one system, by index or by name.
radiocli --device $SDS departments GREENDALE

# Create a department, then rename it.
radiocli --device $SDS departments new GREENDALE POLICE
radiocli --device $SDS departments rename POLICE "POLICE OPS"

# Open a department's menu on the radio, or remove it and every channel in it.
radiocli --device $SDS departments goto "POLICE OPS"
radiocli --device $SDS departments delete "POLICE OPS" --yes
```

Creating, renaming and deleting a department prints a line of text, and under `-o json` prints a `render.Mutation` instead: an object carrying `action`, `kind`, `name`, and `was` and `in` where they apply. The same shape covers those verbs at every level of the scanner's memory, so a script driving edits does not learn a different object for each. The text output is unchanged, because it is what people already read; the JSON mode was added beside it rather than in place of it.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Memory hierarchy** - Favorites lists hold systems, systems hold departments, and departments hold channels, which is why a department has to be named through the level above it
- **Index resolution** - A department's index says nothing about which system or list holds it, so a name costs a read of the whole catalogue before anything can be pressed
- **Menu driving** - Creating and renaming have no protocol command, so they are done the way a person would, by walking the scanner's menus and typing on its entry screens
- **Read-after-write** - Every change is confirmed by reading the scanner back, because a menu press that appeared to work is not evidence that anything changed
- **Ambiguous empty answers** - A system holding nothing and an index naming no system look identical over the protocol, which is why the empty case is checked rather than reported
