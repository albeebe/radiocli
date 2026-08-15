# daemon

## What this does?
This package is the `daemon` command: it claims one scanner, keeps it, and runs other invocations' commands on it over a socket. It is what turns a radio that only one command at a time can have into one that several can share.

## Why we use it?
The claim on a scanner covers a whole invocation, and it has to. A menu walk is several commands to the radio that only mean anything together, so letting somebody else's command land in the middle of one would produce a scanner sitting on a screen nobody asked for. The cost of that safety is that the second caller is simply refused while the first is working, which is fine for one person at a keyboard and useless the moment anything else wants to look.

Keeping this as its own command is what makes sharing something you switch on rather than something that happens to you. No other command starts a daemon: commands look for one only after being refused the scanner, and behave exactly as they always did when there is none, so a script that was relying on being refused keeps working. With a daemon running, being second means waiting a turn instead of failing. Given `--audio` it holds a sound input for the same reason and to the same end, because a sound card can only be open once and one program reading it can serve as many listeners as ask.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	daemon.New,
}
```

```bash
# Hold a scanner and answer other invocations until interrupted.
radiocli daemon --device $SDS

# Hold the scanner's audio as well, so several things can listen at once.
radiocli daemon --device $SDS --audio "USB Audio CODEC"

# Take the left side of the cable rather than working it out.
radiocli daemon --device $SDS --audio "USB Audio CODEC" --audio-channel left

# Started by another program: stop once that program has gone.
radiocli daemon --device $SDS --exit-with-parent
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
- **Unix domain sockets** - The daemon answers other invocations over a socket named after the serial port it holds, which is what lets them find it without being told
- **Advisory locking** - The claim on the scanner is what makes the daemon the one holder, and it is taken before the socket exists so nobody can submit work to a daemon that never got the radio
- **Orphan detection with pipes** - `--exit-with-parent` watches stdin for end-of-file, so the kernel reports a dead parent without polling or process IDs that can be reused
- **Serialized access to a shared resource** - Commands still run one at a time because the radio answers one at a time; what the daemon changes is that being second means queueing rather than failing
