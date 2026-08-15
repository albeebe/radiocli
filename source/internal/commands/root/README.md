# root

## What this does?
This package builds the top-level `radiocli` command, the one every other command hangs off. It owns the options that apply to the whole tool, such as which scanner to talk to and how to print results, and it works out the final settings before any command runs.

## Why we use it?
Every command in this tool needs the same handful of answers before it can do anything: which config file to read, which serial port the scanner is on, whether to print text or JSON, how fast to press keys, and how long to wait if another copy of the tool already has the radio. Asking each command to collect those answers for itself would mean the same flags declared thirty times, and thirty chances for one of them to resolve settings in a slightly different order. The order matters here: a value can come from a built-in default, from the config file, or from a flag typed on the command line, and a flag has to win, or people would be unable to override what the file remembers.

Keeping that in its own package gives the tool a single front door. The global flags are declared once and inherited by everything below, the resolution order lives in one function that can be read top to bottom, and the App is finished being built before the first command's code runs, so a command can assume its settings and its logger are ready. The package deliberately does not import the commands it will one day carry: the caller adds those, which keeps command packages independent of each other and lets a test build a root command with nothing but the one command it wants to exercise.

## How we use it?
```go
// main builds the App, then the root command, then adds every subcommand
// from the list it keeps of their constructors.
app := appcontext.New()
defer app.Close()

cmd := root.New(app)
for _, newCmd := range commands {
    cmd.AddCommand(newCmd(app))
}

// Execute parses the flags, resolves the settings, and runs the command.
// Errors come back rather than being printed, so main owns the exit code.
if err := cmd.Execute(); err != nil {
    fmt.Fprintln(os.Stderr, "Error:", err)
    os.Exit(1)
}
```

A subcommand that needs its own `PersistentPreRunE` must call this one explicitly, because cobra runs only the nearest hook in the chain and skipping it would leave the App unbuilt.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Cobra** - The command framework this package builds on, which supplies the command tree, flag parsing, and help output
- **Persistent flags** - Flags declared once on the root command and inherited by every subcommand, which is how the global options reach commands that never mention them
- **Configuration precedence** - The rule that defaults are laid down first, the config file next, and flags last, so an explicit flag always wins
- **PersistentPreRunE** - The hook where settings are resolved and dependencies are built; cobra runs only the nearest one in the chain, which is why a subcommand that defines its own must call this one
- **Dependency injection** - The App is built once here and handed down, so commands never construct their own dependencies and a test can substitute fakes
