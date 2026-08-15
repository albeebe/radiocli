# source

## What this does?
The Go module that builds the `radiocli` binary. Everything the tool is made of lives here: the entry point that wires the commands together, and the packages under [internal/](internal/) that do the work.

## Why we use it?
The repository holds more than a program. There are reverse-engineering notes, a command reference, and an end-to-end suite that is its own Go module, and none of those should be compiled, vendored or downloaded by somebody who wants the tool. Putting the module in its own directory draws that line: `source/` is the thing that builds, and everything beside it is the thing that explains or checks it.

Almost nothing here is at the top level, which is deliberate. `main.go` owns three responsibilities and no more: listing the commands, building a fresh command tree per invocation, and turning an error into an exit code. Everything else is a package under `internal/`, which the Go toolchain refuses to let another module import, so the tool's insides stay changeable without becoming somebody else's dependency. The wiring in `main.go` is what keeps the command packages unaware of each other: a command that needs to run another one, list the others, or watch the scanner while another holds it gets a function handed to it, rather than an import.

## How we use it?
Build and run:

```bash
cd source
go build -o radiocli .
./radiocli devices
```

Adding a command means writing its package under [internal/commands/](internal/commands/) and adding two lines here:

```go
import "github.com/albeebe/radiocli/internal/commands/battery"

// commands lists every command the tool exposes. Adding one means importing
// its package and adding its constructor here, and nothing else.
var commands = []func(*appcontext.App) *cobra.Command{
	battery.New,
	// ...
}
```

A command that only reads the scanner says so with the `appcontext.OnlyReads` annotation, which is what lets it run alongside another command holding the radio. Anything unmarked is refused, so a new command has to be looked at before it can share.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

The end-to-end suite in [../testing/](../testing/) is separate, needs a real scanner attached, and imports nothing from here.

## Further reading
- **Go modules** - Why `go.mod` lives here rather than at the repository root, and what that means for anybody importing this code
- **internal packages** - The toolchain rule that makes everything under `internal/` unimportable from outside this module
- **cobra** - The command framework the tree is built with, and why a fresh tree is built per invocation rather than reused
- **Dependency injection** - The `App` value carrying config, streams and the scanner connection, which is what lets a test substitute a fake radio
- **signal.NotifyContext** - How Ctrl-C and SIGTERM become a cancelled context that in-flight work can stop on cleanly
