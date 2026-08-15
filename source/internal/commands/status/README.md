# status

## What this does?
This package is the `status` command: it reports which scanner is connected, what firmware it is running, and what it is doing right now. It is the first thing to run when you want to know whether the radio is answering at all.

## Why we use it?
Every other command assumes a working connection to a particular radio, and when something goes wrong there is no way to tell a broken cable from a busy port from a scanner sitting in a menu. One command that opens the connection and reports the port, the model, the firmware version, the display mode and the current mode answers all of those questions at once, in the time it takes to read five lines. It also calls out the state that causes the most confusion: a scanner that is holding on one channel looks identical to a scanning one from every other angle, and someone brushing the knob is enough to cause it.

This package is also the template the rest of the commands follow. `New` wires the command to the App and declares its flags, and `run` holds the work with no cobra types in sight, reaching the scanner through `app.Device` so a test can inject a fake connection instead of a serial port. Keeping that shape in the smallest command that talks to the radio means the pattern is easy to read, easy to copy, and proven by a package whose tests never touch hardware.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	status.New,
}
```

```bash
# Confirm the scanner answers, and see what it is doing.
radiocli --device $SDS status

# The same reading for a script.
radiocli --device $SDS status -o json
```

```json
{
  "port": "/dev/cu.usbmodem00000000000011",
  "model": "SDS150",
  "firmware": "Version 1.00.37",
  "display": "color",
  "mode": "Scan Hold",
  "holding": true
}
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
- **Dependency injection** - The command receives the App rather than opening a serial port, which is what lets a test drive it against a fake connection
- **Lazy initialization** - The connection opens on first use inside `app.Device`, so commands that never touch the radio pay nothing for it
- **Context cancellation** - The command passes `cmd.Context()` down so a scanner that stops answering does not hang the tool
- **Scan hold** - The scanner state this command exists to make visible, where the radio is parked on one channel rather than working through a list
