# commandtree

## What this does?
This package reads the tool's full menu of commands and turns it into a plain list that other parts of the program can display. Anything that presents the tool to a user gets every command and every flag without keeping its own copy.

## Why we use it?
The tool's commands live inside cobra objects that are wired for execution, not for presentation. Cobra knows how to run a command and print its help, but it does not hand out a simple description of the tree: what the commands are called, what arguments they take, and which flags belong to each one. Anything that wants to offer the commands some other way, such as the daemon answering a client that asks what this build can run, would otherwise have to keep its own list, and a copy is a second place to forget a command when a new one is added.

Keeping this reading in its own package means the rule in main still holds: adding a command means importing its package and adding its constructor and nothing else. The tree is described into the plain data types in appcontext, so presenters depend on data rather than on cobra, and the one tricky part, reading positional arguments out of the usage line, lives in one place where the repository's usage conventions are written down next to the code that parses them.

## How we use it?
```go
package main

import (
	"fmt"

	"github.com/albeebe/radiocli/internal/commandtree"
	"github.com/spf13/cobra"
)

func main() {
	// root is the cobra command the tool is built around, with every
	// command already registered under it.
	root := &cobra.Command{Use: "radiocli"}

	// Describe reads the whole tree into plain data, leaving out hidden
	// commands and cobra's own help and completion.
	for _, cmd := range commandtree.Describe(root) {
		fmt.Println(cmd.Name, cmd.Short)
	}
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
- **cobra** - The command framework this package reads; its Command type holds the names, usage lines, and flags being described
- **pflag** - Cobra's flag library; flag types decide whether a flag mentioned in a usage line swallows the field after it
- **appcontext** - The package holding the plain Command, Arg, and Flag types this tree is described into
- **Usage line conventions** - The `<name>`, `[name]`, and `...` notation that is the only record of what positional arguments are called
- **Single source of truth** - The reason this package exists; a second copy of the command list is a second place to forget a command
