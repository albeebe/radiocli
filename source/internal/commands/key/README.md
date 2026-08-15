# key

## What this does?
This package is the `key` command: it presses keys on the scanner's front panel, exactly as though somebody had reached over and pressed them. Several keys can be given at once, and they are pressed in the order they were written.

## Why we use it?
The scanner's remote protocol has a name for a lot of things, but not for everything. Some screens can only be reached by working the menus the way a person does, and some corners of the radio have no command of their own at all. Pressing keys reaches all of it, because anything the operator can do from the front panel is reachable this way.

That reach is also why this is the bluntest instrument in the tool, and the package is shaped around admitting it. A key means whatever the current screen says it means, so the same press can do two different things a second apart, and this command checks nothing about the result. Prefer a command that names what it does when one exists. What this package can do is make the failures honest: every name is resolved to a key before the scanner is opened, so a typo in a run of five presses none of them, and a run that stops partway through says which key it stopped on and which ones already went through, because the scanner has been left somewhere nobody intended and that is the only way to work out where.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	key.New,
}
```

```bash
# One key.
radiocli --device $SDS key menu

# Several, pressed in order, with time between them to let the screen redraw.
radiocli --device $SDS --pace slow key menu right right push

# Hold a key rather than tapping it.
radiocli --device $SDS key --action long function

# Under -o json it reports what it pressed, because empty stdout is not
# something a decoder can read.
radiocli --device $SDS key menu -o json
```

On success the command says nothing in text mode, which is right: the result of a key press is on the scanner's own screen. Under `-o json` it writes the keys and the action instead, so a script gets something parseable rather than nothing. It reports what was asked for rather than what happened, which is the honest limit here, since what a key does depends entirely on what was on screen when it landed.

Names are what the key is called on the radio: `menu`, `function`, `avoid`, `enter` (or `yes`), `no`, `left`, `right`, `push`, `soft1` through `soft3`, `replay`, `zip`, `range`, `service-type`, `backlight`, `squelch`, and the digits `0` through `9`. The ways to press are `press`, `long`, `hold` and `release`.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, which is how every command in this tool is wired into the tree
- **Fail fast validation** - Every key name is resolved before the scanner is opened, so a run either presses everything or nothing
- **Error wrapping with %w** - A refused key is wrapped rather than replaced, so the reason from the device layer survives to the top
- **Rate limiting and pacing** - Keys are spaced by `--pace` so the scanner has time to redraw between presses, which is what makes a long run land where it should
- **Stateful interfaces** - What a key does depends entirely on the current screen, which is why this reaches everything and guarantees nothing
