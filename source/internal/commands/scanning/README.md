# scanning

## What this does?
This package is the `scanning` command: it reports the channels the scanner is working through right now, with the frequency or talkgroup of each one. Its `systems` subcommand answers the shorter question of which systems are in the rotation.

## Why we use it?
A scanner will happily tell you what it holds and say nothing at all about what it is doing. The lists are readable from memory, but "switched on to scan" is a property of the whole setup rather than of any one list, and the channels actually in play are spread across systems and departments that no single request returns. The protocol makes it worse rather than better: asking the full database for its contents answers with the wrong document and hangs the scanner, the database has no menu of its own, and its systems carry no menu index. So the only reliable way in is the way a person would do it, which is to hold the scanner on a channel and turn the knob and read the screen it draws.

Keeping that as its own command is what makes the awkward part safe to rely on. Two situations look like one question and are not: a favorites list is a loop of a few dozen channels that the knob walks exactly, in about a second, and the answer is genuinely everything. The full database is hundreds of thousands of entries, and held, the knob browses all of them rather than the part being scanned, so it never finishes. That case is watched instead, collecting systems and departments as the scanner cycles past them until nothing new turns up, which is a sample and is said to be a sample. The command tracks how the walk ended and prints a different note for each ending, because "this is everything" and "this is what I saw" are different answers and printing them the same way is the failure that looks like success. It also puts the scanner back when it is done, including the case where the knob has stepped off the scanning screen into the menus, which is not something the caller should have to clean up.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	scanning.New,
}
```

```bash
# List the channels being scanned, with frequencies and talkgroups.
radiocli --device $SDS scanning

# Walk further than the default 500 channels.
radiocli --device $SDS scanning --limit 2000

# Just the systems in the rotation.
radiocli --device $SDS scanning systems

# The same reading for a script.
radiocli --device $SDS scanning -o json
```

```json
[
  {
    "source": "Greendale Valley",
    "system": "Northmoreland State Police",
    "department": "Troop A",
    "channel": "A1 Dispatch",
    "value": "42112"
  },
  {
    "source": "Greendale Valley",
    "system": "Greendale Fire",
    "department": "Fireground",
    "channel": "Fire 1",
    "value": "154.190000MHz"
  }
]
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `systems` attached as a subcommand, which is how every command in this tool is wired into the tree
- **Hold and knob stepping** - Holding is what makes the knob step from channel to channel instead of doing nothing, and the hold key toggles rather than sets, so it is pressed until the screen agrees rather than counted
- **Cycle detection** - The systems walk records what it sees and looks for the repeating period afterwards, because spotting the loop close as it happens lets one misread screen end the walk early
- **Trunked systems and talkgroups** - A conventional channel has a frequency and a trunked one has a talkgroup, which are not the same kind of number, so both are reported exactly as the scanner writes them
- **Sampling versus enumeration** - The full database can only be watched rather than walked, and knowing which of the two produced a list is the difference between a complete answer and a partial one
