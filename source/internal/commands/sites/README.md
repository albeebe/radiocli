# sites

## What this does?
This package is the `sites` command: it lists the sites of one trunked system, and creates, renames, or deletes them. It also manages the pool of frequencies each site uses, which is what the system actually receives on.

## Why we use it?
A trunked system does not transmit from one place on one frequency. It speaks through a site, and each site holds a pool of frequencies that a computer hands out for the length of each conversation. That makes a site the part of the scanner's memory a listener most often has to fix: a system entered without its site receives nothing at all, and a site missing some of its frequencies loses every call handed to the ones it does not know. The scanner offers no command for any of this. Sites can only be created, named, and deleted by walking its menus, and the frequency pool can only be edited by typing digits into an entry screen.

Keeping this as its own command means the walk through those menus is written down once, with the safety that walking a menu blind does not have. Every write reads the scanner back afterwards, because the radio is the only authority on what it holds, and the destructive verbs require `--yes` rather than assuming what was meant. Frequencies are compared as numbers rather than as text, so `851.050` typed by a person and ` 851.050000MHz` written by the scanner are recognised as the same frequency, which is what stops a duplicate being added to a pool that is happy to keep it. An empty answer gets an extra look, because a conventional system holding no sites and an index that names no system answer identically, and telling somebody a system has no sites when they actually mistyped the index sends them looking for a site rather than for the system. Sites sit beside departments rather than inside them, so this is a command of its own rather than a subcommand of `departments`: the site says where the signal comes from, the department says who is talking on it.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	sites.New,
}
```

```bash
# List the sites of one trunked system, by name or by index.
radiocli --device $SDS sites "FIRE DISPATCH"

# Create a site, then give it the frequencies it receives on.
radiocli --device $SDS sites new "FIRE DISPATCH" AIRPORT
radiocli --device $SDS sites frequencies add AIRPORT 851.050 852.3625

# See what a site holds, and remove one frequency from the pool.
radiocli --device $SDS sites frequencies AIRPORT
radiocli --device $SDS sites frequencies delete AIRPORT 852.3625 --yes
```

Creating, renaming and deleting a site prints a line of text, and under `-o json` prints a `render.Mutation` instead: an object carrying `action`, `kind`, `name`, and `was` and `in` where they apply. The same shape covers those verbs at every level of the scanner's memory, so a script driving edits does not learn a different object for each. The text output is unchanged, because it is what people already read; the JSON mode was added beside it rather than in place of it.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Trunked radio** - A system that shares a pool of frequencies across everyone on it, which is why a site holds frequencies and a talkgroup does not
- **Control channel** - The frequency carrying the data that says where each conversation went, found by the scanner itself, which is why the pool is entered as a plain list with no roles attached
- **Menu walking** - Sites are created and deleted by pressing entries by name rather than by any command the protocol offers, so every step is matched against what is on screen
- **Read-after-write** - Every change is confirmed by reading the scanner back, because a menu press that appeared to work is not evidence that anything changed
- **Floating point equality** - Frequencies are compared in whole hertz rather than as floats, since two identical frequencies can disagree once they have been through a float
