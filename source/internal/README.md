# internal

## What this does?
Holds every package the `radiocli` tool is built from, from the low-level serial conversation with the scanner up to the individual commands a person types. Nothing outside this module can import any of it, so the insides stay free to change.

## Why we use it?
Driving a radio from a terminal is several unrelated jobs stacked on top of each other. Something has to speak the scanner's line protocol, something has to stop two invocations talking over each other on one serial port, something has to know that a channel lives three menus deep and how to walk there, and something has to turn all that into commands with flags and help text. Written as one layer those concerns tangle immediately: the code that types a name into the radio ends up knowing about output formats, and the code that prints a table ends up knowing about carriage returns.

Splitting them means each package can be reasoned about and tested on its own, and the awkward parts stay contained. The two third-party libraries with C dependencies, for sound cards and for the Opus codec, are each walled into a single package so nothing else has to know they exist. The knowledge of how the scanner's memory is laid out sits in `navigate`, so the commands that list a department or rename a channel do not each carry their own copy of the route. Everything meets in `appcontext`, the one value carrying config, output streams and the scanner connection, which is what lets a test hand a command a fake radio and read back exactly what it would have printed.

## How we use it?
The layers, roughly bottom up:

```go
import (
	"github.com/albeebe/radiocli/internal/device"     // the serial line protocol
	"github.com/albeebe/radiocli/internal/portlock"   // one invocation per scanner
	"github.com/albeebe/radiocli/internal/broker"     // several processes, one scanner
	"github.com/albeebe/radiocli/internal/catalog"    // the XML the scanner answers lists with
	"github.com/albeebe/radiocli/internal/menus"      // reading and rendering a menu
	"github.com/albeebe/radiocli/internal/navigate"   // walking to where memory lives
	"github.com/albeebe/radiocli/internal/textinput"  // typing a value, one key at a time
	"github.com/albeebe/radiocli/internal/appcontext" // the value every command is handed
	"github.com/albeebe/radiocli/internal/commands"   // one package per command
)
```

The rest are supporting: `audioin` and `opusenc` wall off the audio libraries, `audiofeed` serves one sound card to many listeners, `audiogate` finds the transmissions in that audio, `wavfile` writes one of them to a playable file, `recordings` files those on disk with their descriptions and a searchable index, `buildinfo` carries what the binary was built from, `cmdline` splits a typed line into arguments, `render` holds the three formatters every command's output uses, and `commandtree` reads the cobra tree into plain data so anything presenting the tool can offer every command without keeping its own list.

Each package has its own README with the detail.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Layered architecture** - Why the serial protocol, the menu walking and the commands are separate rather than one file that does all three
- **Package boundaries as firewalls** - Keeping cgo-backed audio libraries in one package each, so a build problem stays where it started
- **Interface segregation** - Commands reaching the radio through a narrow surface on `App`, which is what makes a fake scanner possible in tests
- **Trunked radio memory model** - Favorites, then systems, then departments, then channels, with sites underneath: the hierarchy `navigate` exists to walk
- **Context propagation** - The cancellable context threaded from Ctrl-C down to the serial read, so a scanner that stops answering does not hang the tool
