# broker

## What this does?
A scanner has one cable and will only talk to one program at a time, so the second program to ask is normally turned away. The broker lets one process hold the radio and take turns on everybody else's behalf, so being second means waiting a moment instead of failing, and it shares the scanner's audio with as many listeners as want it.

## Why we use it?
Holding the scanner has to be exclusive. A command that walks the radio's menus presses one key, reads the screen, and presses the next, and a second command slipping in between those steps would type into somebody else's screen and read back an answer meant for them. The port lock makes that safe by refusing everybody but the first caller, which is the right rule and the wrong experience: a person with a terminal open and a front end pointed at the same radio finds that whichever one started first has taken the scanner away from the other for as long as it runs.

This package is the same rule with a queue in front of it. One process holds the port and listens on a unix socket, and anything refused the lock submits its command there instead and gets back exactly what it would have printed, so the commands still run one at a time and nothing about indivisibility changes. It is a package of its own because it owns a protocol rather than a helper: the daemon, the client, the queue, the lease that spans several commands, and the audio fan-out are all written against one definition of the wire format, and anything else that speaks to a daemon has to implement that same format. Changing anything here is changing the protocol rather than an internal detail.

## How we use it?
The common case is a command that would rather queue than be refused. Ask for the daemon, and carry on as normal when there is not one:

```go
import (
	"errors"
	"os"

	"github.com/albeebe/radiocli/internal/broker"
)

client, err := broker.Dial(port)
if err != nil {
	// No daemon is sharing this scanner, so hold it directly as always.
	if errors.Is(err, broker.ErrNoDaemon) {
		return runLocally(ctx, argv)
	}
	return err
}
defer client.Close()

// Waits its turn, then streams back what the command produced.
outcome, err := client.Run(ctx, argv, broker.ModeQueue, os.Stdout, os.Stderr)
if err != nil {
	return err
}
os.Exit(outcome.Code)
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Unix domain sockets** - How a daemon on this machine is reached, and why the permissions on the socket and on the directory holding it are the whole of its access control
- **Length-prefixed framing** - Why a stream of audio needs a header saying how long each frame is, and why nothing can resynchronise without one
- **Head-of-line blocking** - What a single queue in front of one radio costs a caller that only wanted a quick reading, and why ModeTry and ModePeek exist
- **Lease expiry** - Why a claim held across several commands needs a deadline, and what happens to a shared resource without one
- **Opus** - The codec behind the compressed audio tier, and why its bitrate can be changed between one frame and the next
