# device

## What this does?
This package is the tool's connection to the scanner: it opens the radio's USB serial port and turns the raw command-and-reply protocol into named operations like "read the screen" or "press this key". Everything else in the tool talks to the radio through here, and nothing else knows the protocol.

## Why we use it?
The scanner exposes only two low-level primitives over its serial port: press any front-panel key, and read the whole screen back as text with per-character attributes. Between them they are a complete remote control, but they are the wrong shape to write commands against. A command that wanted the battery voltage would otherwise have to know the exact command string, the field order in the reply, how to tell a rejection from an unsupported firmware feature, and how fast keys may be sent before the radio drops them. Spread that knowledge across thirty commands and every protocol quirk gets learned thirty times, inconsistently.

Keeping it in one package means the protocol is written down once, and the rest of the tool reads as intent rather than as byte pushing. That has three practical benefits. Replies are checked for sync, because the first field of every answer echoes the command that asked for it, so a mismatch is caught here rather than surfacing as nonsense data upstream. Failures arrive already sorted into kinds, so a caller can tell "the firmware does not know this command" from "the radio refused" and give the right advice. And the port is claimed before it is opened, so two invocations of the tool cannot interleave their exchanges on one radio.

## How we use it?
```go
// Open claims the port, connects, and confirms a scanner answers.
scanner, err := device.Open(ctx, "/dev/cu.usbmodem1101", 0, log)
if err != nil {
    return err
}
defer scanner.Close()

// Named operations instead of raw commands.
info, err := scanner.ScannerInfo(ctx)
if err != nil {
    return err
}
fmt.Println(info.Mode)

// Reading the display back is how anything not in the protocol is learned.
screen, err := scanner.Screen(ctx)
if err != nil {
    return err
}

// Keys are paced, because the radio drops them if they arrive too fast.
if err := scanner.PressKey(ctx, device.KeyMenu, device.KeyPress); err != nil {
    return err
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
- **Serial communication** - The radio speaks a line protocol over USB at a fixed baud rate, which is what every operation here is built on
- **Protocol framing** - Replies echo the command that asked for them, which is how a connection that has fallen out of sync is detected
- **Screen scraping** - The colors, menus and anything else missing from the protocol are read by parsing the display, not by asking for them
- **File locking** - The port is claimed before it is opened so two invocations cannot talk over each other, which the portlock package provides
- **Sentinel errors** - Distinguishing an unsupported command from a rejected one lets callers give advice that matches the actual failure
