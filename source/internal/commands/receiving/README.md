# receiving

## What this does?
This package is the `receiving` command: it reports what the scanner is hearing at this instant, meaning the channel it stopped on, where that channel lives in its memory, and the frequency or talkgroup behind it.

## Why we use it?
"What is it doing" and "what is it listening to" are different questions, and only the first had an answer. `status` reports the connection, the firmware and the mode, all of which stay true for hours. `scanning` reports the channels in the rotation, which means moving the radio and takes seconds. Neither says what is coming out of the speaker right now, which is the reading anything following the radio actually needs: it is what labels a recording, and it is what a person or an agent asks the moment the scanner stops on something. The trap is that a scanning radio names a channel too. Asked while it is stepping through its lists, it answers with whatever channel it happened to be checking at that instant, and that reply is indistinguishable from one about a transmission unless something says which it is.

That distinction is why this is its own command, and everything about its shape follows from it. `receiving` is the first field, the one to read before any other, and when it is no the command says on stderr that the channel was only passed rather than stopped on. What it answers with is the mute state rather than the signal strength, because those two disagree at the moment that matters most: a document captured on the first poll of a real transmission read `Mute="Unmute"` with no signal bars at all, and the very next one showed five bars on the same unchanged signal, so anything waiting for bars misses the opening of every transmission. And because it presses no keys and moves nothing, it carries `appcontext.OnlyReads`: a daemon holding the radio runs it alongside whatever else is running instead of queueing it, several times a second for as long as you like, which is what lets `audio record` use it as the label source while a recording runs.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	receiving.New,
}
```

```bash
# What is it hearing right now?
radiocli --device $SDS receiving

# The same reading for a script or an agent.
radiocli --device $SDS receiving -o json

# Watch the traffic go by.
while :; do radiocli --device $SDS receiving -o json | jq -c 'select(.receiving)'; done
```

```json
{
  "receiving": true,
  "list": "Pocahontas County",
  "system": "PUBLIC SAFETY",
  "department": "POLICE DEPARTMENT",
  "channel": "MARLINTON DISPATCH",
  "frequency": "155.550000MHz",
  "modulation": "NFM",
  "signal": "0",
  "rssi": "-87",
  "mode": "Scan Mode"
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
- **appcontext.OnlyReads** - The annotation that lets this run alongside another command, and why a command that presses a key may not carry it
- **Mute versus signal strength** - Why the audio gate opens before the bars appear, and what that costs anything that waits for the bars
- **Conventional channels and trunked talkgroups** - Two different kinds of answer to "what is it on", reported in separate fields because they are not the same sort of number
- **device.ScannerInfo** - The `GSI` document behind all of this, and the raw XML it keeps for anything not modelled
