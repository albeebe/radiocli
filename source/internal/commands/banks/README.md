# banks

## What this does?
This package is the `banks` command: it lists the scanner's ten custom search banks, changes what one of them holds, and starts the scanner searching the ones you choose. A bank is a range of frequencies rather than a list of channels, which is how you listen to a band the scanner's database does not cover.

## Why we use it?
The database is what makes a scanner useful, and it is also what limits it. Anything the database does not carry, the CB band, a business licence nobody catalogued, a range you simply want to sweep, cannot be reached by building a favorites list, because there are no channels to put in one. The ten custom search banks are the way in: each is a frequency range with its own modulation and step, and the scanner sweeps them the way it scans everything else. There are always exactly ten, they always exist, and they cannot be added to or removed, so a bank is configured rather than created.

Keeping this as its own command is what lets the reading half stay cheap. Most of what a bank holds the scanner will report outright, in a single request that costs milliseconds and never interrupts the scan, so the bare command is something you can run while listening. The rest, the attenuator, the delay, the digital waiting time and the two search with scan settings, exists only inside the scanner's menus, and reaching it means stopping the scan and walking through screens a second at a time. Those two costs are too different to hide behind one verb, so the slow half is behind `--full` and the writing half is behind `set`. Everything written is read back from the scanner afterwards, so what gets reported is what is stored rather than what was asked for, and the scanner is always returned to scanning when the walk is done.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	banks.New,
}
```

```bash
# What the ten banks hold. One request, and the scan carries on throughout.
radiocli --device $SDS banks

# Everything a bank holds, including the settings only its menus can answer for.
radiocli --device $SDS banks --full

# Point bank 9 at the CB band, then search that bank and nothing else.
radiocli --device $SDS banks set 9 --range 26.965-27.405 --name CB --modulation AM --step 10k
radiocli --device $SDS banks scan 9

# Leave the scanner on a bank's menu, to work the rest of it by hand.
radiocli --device $SDS banks goto 9
```

```json
[
  {
    "bank": 9,
    "name": "CB",
    "lower": "26.965",
    "upper": "27.405",
    "modulation": "AM",
    "step": "10.0 kHz"
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
- **Custom search banks** - Fixed hardware slots holding a frequency range rather than channels, which is why they are numbered and cannot be created or deleted
- **Menu driving** - Settings the protocol will not read or write are reached by walking the scanner's own screens, one key press at a time, exactly as a person would
- **Read-after-write verification** - Every setting is read back off the scanner before it is reported, so a value the radio quietly declined shows up as a printed difference
- **Toggle versus set** - The digit keys that choose banks flip one on or off rather than setting it, so what is already on has to be read before anything is pressed
- **Normalising vendor output** - The same step arrives as "10000" from one part of the scanner and "10.0 kHz" from another, and both are written the one way so they can be compared
