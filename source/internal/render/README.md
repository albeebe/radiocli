# render

## What this does?
This package holds the small formatters every command's output shares: the indented JSON that `--output json` produces, the dash that keeps a blank table cell from reading as nothing, the yes and no a listing answers with, the one-shape report of an edit to the scanner's memory, the note about entries that are only partly known, and the yellow warning on stderr. Nothing here knows what a command is, what the scanner is, or what is being printed.

## Why we use it?
The command packages deliberately do not import each other, which is the rule that keeps thirty commands from becoming one tangle, and that rule has a price: the same tiny formatter gets written again in every package that needs it. Before this package existed, `dash` was copy-pasted into six command packages plus an inline closure in a seventh, `yesNo` into six more, and the three-line indented-JSON-encoder block appeared thirty-eight times. Six copies of a four-line function are not six times the maintenance of one; they are six chances for the seventh caller to discover that two of them disagree, and a listing where an empty value reads `-` in one column and empty in the next is a bug that is invisible in the source of either.

The line is drawn at coupling rather than at size. A command importing this is importing downward onto a layer, the way it imports `fmt` or `strings`, which is not the horizontal command-to-command dependency the [commands README](../commands/README.md) rules out. `Dash` and `YesNo` are for text output only, because JSON keeps the empty string and the boolean: a program deciding whether a value is present should not have to know that this tool spells absence `-`. `Alert` is here for the same reason as the rest. Colour is a decision about the stream rather than about the message: escape codes written to a pipe or a log file are punctuation in the middle of somebody's text, so they have to be left out unless the destination is a terminal and `NO_COLOR` is unset, and that check is identical everywhere and invisibly wrong until somebody redirects their output.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/render"

// Every command's output function has this shape: the JSON branch first,
// then the aligned table for a person.
func renderSites(app *appcontext.App, found []catalog.Site) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, found)
	}

	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "INDEX\tNAME\tTYPE\tAVOIDED")
	for _, s := range found {
		// An empty cell reads as a missing reading rather than as nothing.
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			s.Index, render.Dash(s.Name), render.Dash(s.Type), render.YesNo(s.Avoid))
	}
	return w.Flush()
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
- **DRY** - The argument for this package is not the lines it saves but the divergence it makes impossible, which is the part of the principle that actually pays
- **Layering** - A command depending on a layer below it is not the horizontal dependency the commands package forbids, and the distinction is what makes this package allowed
- **json.Encoder.SetIndent** - Why the output is indented rather than compact, and where the trailing newline comes from
- **text/tabwriter** - The aligned tables `Dash` exists to serve, and why a blank cell in one is ambiguous
- **NO_COLOR** - The convention for turning colour off once for every program rather than per tool, and why a stream must be a terminal before escape codes mean colour
