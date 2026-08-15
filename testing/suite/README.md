# suite

## What this does?

Holds the end to end tests themselves. Every test builds `radiocli`, runs it the
way a person would type it, and checks what the scanner actually did about it.
Nothing here is a unit test, and nothing here draws anything on screen: the
directory above is what runs these and shows a run as it happens.

## Why we use it?

The tests are a package of their own so that the thing you run can be the thing
at the top. `cd testing && go run .` starts a run; the tests are what it starts.
Before this split the runner lived in a subdirectory of a directory full of test
files, so the one file anybody needed to find was the hardest one to see.

Being a separate package also draws the line the suite depends on. It imports
nothing from `source/`, so it tests the command line surface rather than the
code behind it, and a refactor that keeps the commands working keeps these
tests passing.

`commands.go` is the one file here that is not a test, because it cannot be. It
holds the checklist of every command the tool offers, and both the tests and the
runner above need it: the tests diff it against the binary's own `--help` in
both directions, and the runner reads it to work out which part of a test's name
is the command it covers. A test file cannot be imported, so it lives here as
ordinary code.

## How we use it?

Run the suite through the runner above, which is what draws it:

```bash
cd testing
go run .
```

Go's own tooling still works, and is what to reach for when a failure needs the
full detail:

```bash
cd testing
go test ./suite -v -timeout 30m
go test ./suite -run TestVolume -v -timeout 30m
```

`-timeout 30m` matters when running `go test` directly. The runner passes it for
you; Go's own default is ten minutes, which a full run comes close enough to
that it is not worth risking.

Two rules hold this package together, both checked by `a_naming_test.go`, which
reads these files as source to do it. A test function is named for the command
it covers, so `TestChannelsNew` covers `channels new`, with `_Variant` on the
end where one command needs several functions. And a check's name is at most
fifty characters, written the way somebody who has never read the test would say
it.

Files beginning `a_` run first and hold the root command's own tests, so they
draw at the top of the tree. Files beginning `z_` run last and are the slow ones,
which is everything that types a name into the scanner one key press at a time.

The full picture, including what a run does to your radio and how it puts it
back, is in [../README.md](../README.md).
