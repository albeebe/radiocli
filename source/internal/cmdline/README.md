# cmdline

## What this does?
This package takes a command line someone typed and splits it into the separate words a command expects, keeping quoted names like "GREENDALE, ST 00000" together as one piece. It refuses shell tricks like pipes and redirects with a clear message instead of quietly doing the wrong thing.

## Why we use it?
Commands do not arrive pre-split. The `config macro` command stores whole typed lines to be run later, and the daemon accepts a typed line as one string over its socket, so the program has to break each line into arguments itself. Splitting on spaces alone would tear apart the scanner's own list names, which contain spaces and commas, so the splitter understands single quotes, double quotes, and backslash escapes. And because lines run directly against the command tree rather than through a shell, operators like | and > would not do what they appear to do; passing them through silently would let somebody believe a redirect happened when no file was ever written.

Two places have to agree on what a valid line is. The daemon splits the line it is handed before running it, and the config command checks a macro's steps with the same splitter before storing them, so a line accepted when it is saved has to be a line that splits when it is run. Keeping one splitter in its own package makes those the same question. If each caller rolled its own, a macro could save cleanly and then fail the day it runs.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/cmdline"

// A typed line arrives as one string. Quotes keep a name together.
args, err := cmdline.Split(`favorites scan "GREENDALE, ST 00000"`)
if err != nil {
    // The error says what went wrong in plain terms: an unclosed
    // quote, a trailing backslash, or a shell operator like | or >
    // that commands do not support.
    return err
}

// args is now ["favorites", "scan", "GREENDALE, ST 00000"],
// ready to hand to the command tree.
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Tokenization** - Splitting a raw line into arguments is a small lexer, and the open-token state this package tracks is the classic pattern
- **Shell quoting rules** - Single quotes are literal and double quotes allow escaping a quote or a backslash, which is the behavior this package copies
- **Shell metacharacters** - Characters like | & ; < > ( ) $ and the backtick carry special meaning in a shell, and this package refuses them rather than faking them
- **Fail fast** - Refusing a bad line with a clear message beats silently guessing, since a pretend redirect would leave somebody hunting for a file that never existed
- **Command injection** - Running arguments directly instead of through a shell is what keeps typed input from ever becoming shell code
