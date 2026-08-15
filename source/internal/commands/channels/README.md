# channels

## What this does?
This package is the `channels` command: it lists the channels inside one department, with what each one receives, and its `new`, `rename` and `delete` subcommands change that list. A channel is the smallest thing the scanner holds, so this is where a frequency or a talkgroup actually gets written down.

## Why we use it?
This is the one thing the scanner will not simply report. The protocol request that should list a department's channels answers with the wrong document on this firmware, and there is no command at all for creating, renaming or deleting one. Everything here has to be done the way a person does it, by walking the scanner's own menus and reading its screen, which takes a few seconds rather than one exchange. Reading the frequencies is most of that time, because each one means opening a channel and coming back out, which is why `--names` exists for the times only the list is wanted.

Keeping this as its own command is what turns that walk into one line, with the safety a blind walk does not have. The protocol is tried first, since it is one exchange and it is the only one of the two that reports what a talkgroup channel holds, and the menu walk stays behind it as the fallback that keeps a firmware answering differently from taking the command with it. What a channel receives is one argument rather than two, because a frequency and a talkgroup are the same thing said two ways: the address of what the channel hears. Which one a department takes is decided by the system above it, so `new` reads the entry screen the scanner opened and refuses the wrong kind before typing anything, rather than letting a talkgroup of 9051 be created as 9051 MHz. Every write reads the scanner back afterwards, because the radio is the only authority on what it holds, and `delete` requires `--yes` since nothing here keeps a copy of what it removes.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	channels.New,
}
```

```bash
# List a department's channels and what each one receives.
radiocli --device $SDS channels "FIRE RESCUE"

# The names only, skipping the slow read that opens every channel.
radiocli --device $SDS channels "FIRE RESCUE" --names

# Create a channel on a frequency, or on a talkgroup for a trunked system.
radiocli --device $SDS channels new "FIRE RESCUE" 153.980 DISPATCH
radiocli --device $SDS channels new "GREENDALE FIRE" TGID:9051 "FIRE DISPATCH"

# Correct a name without losing the frequency, then remove a channel.
radiocli --device $SDS channels rename "FIRE RESCUE" DISPATCH "FIRE DISPATCH"
radiocli --device $SDS channels delete "FIRE RESCUE" "FIRE DISPATCH" --yes

# The same listing for a script.
radiocli --device $SDS channels "FIRE RESCUE" -o json
```

```json
[
  { "name": "DISPATCH", "frequency": "153.980000MHz" },
  { "name": "FIRE DISPATCH", "talkgroup": "9051" }
]
```

Creating, renaming and deleting a channel prints a line of text, and under `-o json` prints a `render.Mutation` instead: an object carrying `action`, `kind`, `name`, and `was` and `in` where they apply. The same shape covers those verbs at every level of the scanner's memory, so a script driving edits does not learn a different object for each. The text output is unchanged, because it is what people already read; the JSON mode was added beside it rather than in place of it.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Trunked radio** - A system that shares a pool of frequencies and hands one out per transmission, which is why a channel there carries a talkgroup and has no frequency of its own
- **Menu walking** - Channels are listed and written by pressing entries matched by name on the display, since the protocol offers nothing that does it
- **Graceful degradation** - The protocol is tried first and the slower walk is the fallback, so a firmware that answers either way is still served
- **Read-after-write** - Every creation, rename and deletion is confirmed by reading the department back, because a menu press that looked like it worked is not evidence that anything changed
- **Destructive command confirmation** - Deleting has no undo and nothing keeps a copy, which is why `--yes` has to be passed for the command to act at all
