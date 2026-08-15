# devices

## What this does?
This package is the `devices` command: it lists every scanner attached to this computer, along with the port each one is on. It is the first command anybody runs, because the port it prints is what every other command needs.

## Why we use it?
A scanner arrives as a serial port with a machine-generated name, and nothing about that name says which radio is on the other end or whether a radio is there at all. Finding out means opening the port and asking, which is work nobody should have to do by hand, and the answer changes every time a cable is moved. Worse, a port that is already in use by another invocation of this tool cannot be opened to be identified, and dropping it from the list would report an attached scanner as missing.

Keeping this as its own command is what turns a folder of device files into a list somebody can act on. It only looks and changes nothing, so it is safe to run at any moment. A port that could not be identified is still listed, marked busy, because being busy is a fact about this moment rather than about the hardware. A busy port that a daemon is holding is marked apart from that again, because it needs nothing at all: commands sent to it queue rather than fail, and telling somebody to wait for a scanner they could be using right now is how a working setup gets reported as a broken one.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	devices.New,
}
```

```bash
# What is attached, as a person reads it.
radiocli devices

# The same listing under its other name.
radiocli list

# The same listing for a script.
radiocli devices -o json
```

```json
[
  {
    "port": "/dev/tty.usbmodem14201",
    "model": "SDS150",
    "serial": "0000000000000000",
    "busy": false
  },
  {
    "port": "/dev/tty.usbmodem14301",
    "model": "",
    "busy": true,
    "shared": true
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, which is how every command in this tool is wired into the tree
- **USB serial enumeration** - Scanners appear as USB serial ports whose names are assigned by the operating system, so identifying one means opening it and asking rather than reading its name
- **Advisory locking** - A port another invocation holds cannot be opened to be identified, which is why a busy port is listed with what little is known about it rather than left out
- **Text and JSON output modes** - One command serves a person and a script by rendering the same listing two ways, chosen by `-o`
- **Exit codes versus empty results** - Finding no scanners is a complete answer rather than a failure, so the advice goes to standard error and the command still succeeds
