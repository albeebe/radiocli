# version

## What this does?
This package implements the `version` command, which says what the copy of the tool you are running was built from. It reports the release version, the exact source revision, when it was built, and which Go version and machine it was built for.

## Why we use it?
Bug reports are worth very little without knowing which build produced them. A person can have several copies of a command line tool on one machine, installed at different times from different places, and the first thing anybody looking at a problem needs is which one actually ran. Asking somebody to work that out for themselves means asking them to hunt through their path, and the answer they give is a guess.

Keeping this as its own command makes the answer something the binary tells you rather than something you deduce. The build details are stamped into the binary at link time, so the command reports the truth about itself even when the file has been renamed or copied somewhere else. The runtime details come from the running program, which is what turns "it does not work on my machine" into a report naming an operating system and an architecture. It is one of the few commands that never touches the scanner, so it works with no radio attached and can run while another command has the radio.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	version.New,
}
```

```bash
# The human-readable form.
radiocli version

# The same answer for a script.
radiocli version --output json
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Link-time variable stamping** - The `-ldflags -X` mechanism that writes the version, commit, and build date into the binary, which is where this command's answers come from
- **Build provenance** - Being able to trace a running binary back to the exact source it was built from, which is what makes a bug report actionable
- **Cobra annotations** - How a command declares properties such as `OnlyReads`, which says it cannot move the scanner and so may run alongside another command
- **Structured output** - Offering the same answer as JSON so scripts can read it without parsing human-readable text
- **runtime package** - Where the Go version, operating system, and architecture are read from at execution time rather than stamped in at build time
