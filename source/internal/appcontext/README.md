# appcontext

## What this does?
This package holds the application context: one value that carries everything a command needs, like settings, logging, and the connection to the scanner. Every command receives it ready-made instead of building its own.

## Why we use it?
A command line tool with dozens of commands has a wiring problem. Every command needs the same things: the resolved settings, a logger, the output streams, and eventually the scanner connection. If each command built its own, the tool would open the serial port many times, log inconsistently, and be nearly impossible to test, because a test could not swap the real scanner or the real terminal for a fake one.

Keeping the context as its own package makes the wiring one-way and visible. main builds one App and hands it down, commands use only what the App exposes, and nothing else in the codebase constructs a dependency. That single choke point is what makes commands trivially testable: a test replaces the streams with buffers and the connection with a fake, and the command cannot tell the difference. It also gives settings one home, with a clear precedence of defaults, then the config file, then flags.

## How we use it?
```go
// main builds one App and hands it to every command.
app := appcontext.New()
defer app.Close()

// The root command resolves settings, then finishes the build.
if err := app.Config.Load(); err != nil {
    return err
}
if err := app.Config.Validate(); err != nil {
    return err
}
if err := app.Init(ctx); err != nil {
    return err
}

// A command asks the App for what it needs. The serial port opens
// here, on first use, and is reused and closed by app.Close.
scanner, err := app.Device(ctx)
if err != nil {
    return err
}

// Results go to Stdout through the App, so tests can capture them.
app.Printf("connected\n")
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Dependency injection** - Commands receive every dependency through the App rather than constructing their own, which is what makes them testable
- **Composition root** - main is the single place that builds and wires the App, so the dependency graph has one owner
- **Lazy initialization** - The scanner connection opens on first use, keeping commands like help and version fast
- **Configuration precedence** - Settings resolve as defaults, then the config file, then flags, so an explicit flag always wins
- **Structured logging** - Diagnostics go through a leveled slog logger on Stderr, kept separate from the program output on Stdout
