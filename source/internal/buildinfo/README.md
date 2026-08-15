# buildinfo

## What this does?
This package holds the identity of a radiocli binary: its release version, the exact source snapshot it was built from, and when it was built. It lets any copy of the program report exactly what it is.

## Why we use it?
When a user reports a bug or asks for help, the first question is always "which build are you running?" A compiled program does not naturally know its own version, the git commit it came from, or its build date, so without a dedicated home for that information every release would be a guessing game. These values are stamped into the binary at link time with `go build -ldflags "-X ..."`, and when no stamping happens (a local development build) the package falls back to honest defaults: `dev`, `none`, and `unknown`.

Keeping this in its own package matters because several parts of the program need the same answer: the root command shows the version in help output, the `version` command prints all three values, and the broker connection sends the version during its handshake. A single, dependency-free package gives them one source of truth, and because it imports nothing, anything in the codebase can import it without risk of a dependency cycle.

## How we use it?
```go
package main

import (
	"fmt"

	"github.com/albeebe/radiocli/internal/buildinfo"
)

func main() {
	fmt.Printf("radiocli %s (commit %s, built %s)\n",
		buildinfo.Version, buildinfo.Commit, buildinfo.Date)
}
```

Release builds stamp the real values at link time:

```bash
go build -ldflags "\
  -X github.com/albeebe/radiocli/internal/buildinfo.Version=v1.2.3 \
  -X github.com/albeebe/radiocli/internal/buildinfo.Commit=$(git rev-parse --short HEAD) \
  -X github.com/albeebe/radiocli/internal/buildinfo.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Link-time variable injection** - The `-ldflags "-X ..."` mechanism that overwrites package variables at build time, which is how these values get their real content
- **Semantic versioning** - The `vMAJOR.MINOR.PATCH` convention that gives the `Version` value its meaning
- **Git commit hashes** - The short revision identifiers stored in `Commit` that pin a binary to an exact source snapshot
- **RFC 3339 timestamps** - The date format used for `Date` so build times are unambiguous across time zones
- **Internal packages** - Go's `internal/` directory rule, which keeps this package private to the radiocli module
