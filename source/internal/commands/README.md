# commands

## What this does?
Holds one package per `radiocli` command, from `battery` and `status` through to `favorites` and `tune`. Each one turns something a person types into work done on the scanner and output on the terminal.

## Why we use it?
Thirty-one commands in one package would be one enormous file and a permanent merge conflict. More to the point, commands genuinely have nothing to do with each other: what `battery` does with a voltage reading has no bearing on how `channels` walks four menus deep to list a department. Giving each its own package means a command can be read start to finish without scrolling past thirty others, its tests name only the thing they test, and adding one touches exactly two lines of code outside its own folder.

Keeping them separate is also what stops the wiring collapsing. No command imports another. When one needs to run another, list the others, or watch the scanner while something else holds it, it is handed a function by `main.go`, which is the only place that knows the full list. The result is that a command package depends on `appcontext` and on whichever layer it needs, and on nothing horizontal, so the set can grow without every addition making every other command heavier to understand.

The rule is about coupling, not about copying. Anything a command owns stays in the command, and two packages solving the same problem differently is the price of the isolation. But a formatter with no command in it belongs in a layer below: `render` holds the indented-JSON writer, the empty-value dash and the yes/no, and `navigate.ResolveSystem` and its siblings hold the name-or-index lookup that four packages used to carry a copy of. Reaching down to those is not a horizontal import, and it is what stops six copies of one four-line function from quietly disagreeing.

## How we use it?
`status` is the template: `New` wires the command to the `App` and declares its flags, and `run` holds the work with no cobra types in sight.

```go
package status

// New returns the status command bound to app.
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use: "status",
		// Only looks at what the scanner is doing, so it may run while
		// another command has the radio.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report the connected scanner",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app)
		},
	}
}
```

Reaching the scanner through `app.Device(ctx)` rather than opening a port is what lets a test inject a fake radio, and it means a command that never touches the scanner pays nothing for the connection. Every command package holds its implementation, its tests, and a README of its own, plus a `types.go` when it has declarations to put there. `root` is the odd one out: it builds the top-level command and owns the global flags and config precedence that every other command assumes is already done.

What every one of these packages is held to, down to the field order in the command literal and the shape of an error message, is written out in [contributing/WRITING_COMMANDS.md](../../../contributing/WRITING_COMMANDS.md). Read it before adding a command, and run the checks at the end of it before calling one done.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **cobra** - The command framework, and the `RunE` and `Annotations` fields every package here uses
- **Command pattern** - One unit per user-visible action, which is what makes the package list and the help output the same list
- **appcontext.OnlyReads** - The annotation that declares a command safe to run alongside another, and why anything unmarked is refused
- **Table-driven tests** - How each command's flags, output formats and failure modes are covered without thirty-one near-identical test functions
- **Separation of parsing and doing** - `New` for the command line, `run` for the work, which is what keeps the tests free of cobra
