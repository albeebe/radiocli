# backlight

## What this does?
This package is the `backlight` command: it reports whether the scanner's light is on and how bright it is, and switches it on and off. It also reports and changes the separate setting that decides whether the keypad lights up along with the screen.

## Why we use it?
Two different things on this radio are called the backlight, and they behave nothing alike. The light itself is momentary: there is one key for it, and that key toggles, so a program that presses it without looking first is as likely to put the light out as to turn it on. The keypad's light is not a switch at all but a setting buried three menus deep, readable only by opening the menu that sets it, and it survives a power cycle. Worse, the two are joined in a way nothing on the scanner mentions: changing the setting while the light is already on appears to do nothing at all until the light is cycled off and on again.

Keeping this as its own command puts all of that in one place so no caller has to know it. The light is read before the key is pressed, so "on" leaves a lit scanner lit rather than dark, and it is read back afterwards until the scanner agrees, because the key press is not proof. Every path that changes the keypad setting cycles the light for you, which is what turns a change that looks like it did nothing into one that visibly took. A setting already at the value asked for is left alone, so a command with nothing to do makes nothing flicker.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	backlight.New,
}
```

```bash
# Is it lit, and how bright.
radiocli --device $SDS backlight

# Light it up, keypad included, or put it out.
radiocli --device $SDS backlight on
radiocli --device $SDS backlight off

# Light only the screen and leave the keypad setting alone.
radiocli --device $SDS backlight on --keys=false

# The keypad setting on its own.
radiocli --device $SDS backlight keys
radiocli --device $SDS backlight keys toggle
```

```json
{
  "on": true,
  "level": 5
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
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, with `on`, `off` and `keys` attached as subcommands, which is how every command in this tool is wired into the tree
- **Toggling versus setting** - The scanner offers one key that flips the light, so reaching a known state means reading first and confirming afterwards rather than pressing and hoping
- **Polling for confirmation** - The light takes a moment to answer, so `flip` looks repeatedly with a short gap instead of sleeping once for a guessed duration
- **Menu walking** - The keypad setting is only reachable by driving the scanner's own menus, which stops the scan and has to be left in a known place afterwards
- **Text and JSON output modes** - One command serves a person and a script by rendering the same report two ways, chosen by `-o`
