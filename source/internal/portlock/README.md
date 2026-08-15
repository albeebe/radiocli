# portlock

## What this does?
Stops two copies of the tool from talking to the same scanner at the same time. The first command to start claims the scanner, and any other command is told who has it and can give up right away or wait its turn.

## Why we use it?
The scanner answers over a single serial line, and nothing in a reply says which process asked for it. Two commands running at once read each other's answers, and the failures range from harmless (both report the scanner as unreachable) to genuinely bad: the commands that walk menus press a key, read the screen, and choose the next key from what they saw, so a second process pressing keys mid-walk leaves the scanner on some half-finished entry screen with nothing reported as wrong. The claim has to cover a whole invocation, not one exchange, because the menu walk is the thing that must not be interleaved.

Keeping this in its own package matters because the lock and the daemon socket must agree on what "the same scanner" means: a client refused the lock goes looking for the socket that belongs to the port it was refused, so both names are derived here from the port in one place. They are derived to different places, though, and deliberately: a lock file has to be readable by every account on the machine so the second person to ask can be told who has the radio, while a daemon socket carries every command the tool can run and belongs to one account alone, so the sockets sit in a directory of their own named after the account that owns them. The lock itself is an operating system file lock rather than a file holding a process ID, because the OS releases it when the process ends, however it ends, which means a killed command leaves no stale lock for anyone to detect or break. The platform-specific parts (flock on Unix-like systems, LockFileEx on Windows) live behind one small pair of functions so the rest of the package reads the same everywhere.

## How we use it?
```go
import (
	"errors"
	"time"

	"github.com/albeebe/radiocli/internal/portlock"
)

func talkToScanner(port string) error {
	// Zero wait gives up at once; pass a duration to queue behind the holder.
	lock, err := portlock.Acquire(port, 0)
	if err != nil {
		if errors.Is(err, portlock.ErrBusy) {
			// The error already names the holding command and how long
			// it has had the port.
		}
		return err
	}
	defer lock.Release()

	// Talk to the scanner. Nothing else using this package can
	// interleave until Release, or until this process exits.
	_ = time.Now()
	return nil
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
- **Advisory locking** - The lock only binds programs that ask for it, which is why other serial software can still interleave with a running command
- **flock** - The Unix system call behind the claim, taken without blocking and retried on an interval so waiting stays interruptible
- **LockFileEx** - The Windows equivalent, pointed at a byte 4 GiB into the file because Windows range locks are mandatory and would otherwise block reading who holds the lock
- **Build constraints** - The `//go:build` tags that pick the flock or LockFileEx implementation per platform at compile time
- **Unix domain sockets** - Why SocketPath names sockets by digest alone: the path must fit in sun_path, which is 104 bytes on macOS
