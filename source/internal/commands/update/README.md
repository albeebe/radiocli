# update

## What this does?
This package implements the `update` command, which replaces the running copy of the tool with the newest release published for it. It asks GitHub what the newest release is, downloads the file built for this machine, checks it against the checksum published alongside it, and swaps it into place.

## Why we use it?
Before this existed, moving to a newer build meant downloading a file from a web page, unpacking it, clearing the quarantine flag on a Mac, and moving it into place with `sudo`. That is a lot of steps to ask of somebody who just wants the fix in the latest release, and it is not something an AI agent driving the tool can do at all: prose install instructions are not a command it can run. Since the whole point of this project is that an agent and a person can drive the same tool, an upgrade path only a person can follow is a gap.

The design is shaped by three decisions worth knowing before reading the code. Nothing is installed that a published checksum does not vouch for, and there is deliberately no flag to skip that check, because an update that falls back to trusting whatever arrived when the checksums are missing has no guarantee at all. The command will not ask for administrator rights on its own; where the program lives in a directory owned by another user it says exactly what to type and stops, because a program that can silently re-run itself as root is one nobody can audit by reading the line they typed. And the platform is passed around as a value rather than read from the runtime, so the Windows replacement path is compiled, vetted and tested on a Mac like everything else, rather than living in a file that is never measured.

The replacement itself is a single rename on Unix, which is atomic and works on a running program because the process keeps the file it started from. Windows cannot overwrite the image of a running process but can rename it, so the running program is moved aside first and swept up on a later run.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	update.New,
}
```

```bash
# Ask whether a newer release exists, changing nothing.
radiocli update --check

# The same answer for a script or an agent.
radiocli update --check --output json

# Install the newest release.
radiocli update

# Go back to an older one.
radiocli update --version v0.1.0
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

The tests never reach the network. A local server stands in for the GitHub API and serves a release whose asset links point back at itself, along with a zip and a checksums file built in the test, so the download, the checksum and the extraction all run for real against files on disk. The package-level variables in `types.go` cover what a test server cannot cause, such as a rename that fails partway through the Windows path.

## Further reading
- **Atomic file replacement** - Why renaming over a running program works on Unix while writing to it fails with "text file busy", and why the staging file has to live in the same directory as its target
- **SHA-256 verification** - Checking a download against a hash published separately from it, and why the check being mandatory rather than optional is what gives it any value
- **Zip slip** - The archive entry whose name is a path rather than a file name, and why the guard belongs in the code rather than in an assumption about who built the archive
- **Decompression bombs** - Why the size an archive claims its contents expand to cannot be used to decide whether to extract it
- **Semantic versioning precedence** - The ordering rules, including why a release candidate sorts before the release it is a candidate for
- **GitHub Releases API** - The endpoints for the newest release and for one named by tag, the rate limit applied per address, and the per-asset digest GitHub records on upload
