# version

Prints which build of the tool you are running. Use it when reporting a problem
or checking that an install or upgrade took effect.

## Overview

`version` reports the release version of `radiocli`, the source revision it
was built from, when it was built, and the Go version and platform it was built
for. Together these identify a build exactly, which is what makes a bug report
actionable. The values are stamped into the binary when it is built, so a
binary built locally without those stamps reports the placeholders `dev`,
`none`, and `unknown` rather than real values. This command touches nothing
outside the binary: it does not read the config file for anything but the
output format, does not connect to a scanner, and works whether or not one is
attached.

## Usage

```
radiocli version [flags]
```

## Parameters

`version` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | — | — | This command takes no flags and no arguments. |

### Global flags that change this command

- `--output` selects whether the build details are printed as lines or as
  JSON. It is the only global flag that changes what this command prints.

The top-level `radiocli --version` flag is not the same thing. It prints only
the version string, while this command prints every field below.

## Examples

Printing the build details of a binary built locally, without version stamps:

```
$ radiocli version
radiocli dev
  commit: none
  built:  unknown
  go:     go1.25.0 (darwin/arm64)
```

The same details as JSON, which is the form to capture in a bug report:

```
$ radiocli version -o json
{
  "version": "dev",
  "commit": "none",
  "date": "unknown",
  "go": "go1.25.0",
  "os": "darwin",
  "arch": "arm64"
}
```

## Output

The build details go to stdout. Nothing is written to stderr, including under
`--verbose`, because this command does no work worth logging.

Under `--output text`, stdout holds four lines. The first names the tool and
its version, and the rest are indented by two spaces with their labels padded
so the values line up:

```
radiocli dev
  commit: none
  built:  unknown
  go:     go1.25.0 (darwin/arm64)
```

The `go:` line combines three values: the Go version, then the operating system
and architecture in parentheses, separated by `/`.

Under `--output json`, stdout holds one object. All six fields are always
present:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `version` | string | Release version, such as `v1.2.3`. `dev` in a build without version stamps. |
| `commit` | string | Source revision the binary was built from. `none` in a build without version stamps. |
| `date` | string | Build time in RFC 3339 format, such as `2026-08-02T17:10:55Z`. `unknown` in a build without version stamps. |
| `go` | string | Go version the binary was built with, such as `go1.25.0`. Always a real value. |
| `os` | string | Operating system the binary was built for, such as `darwin`. Always a real value. |
| `arch` | string | Processor architecture the binary was built for, such as `arm64`. Always a real value. |

The placeholder values `dev`, `none`, and `unknown` mean the binary was built
without version stamps, which is normal for a build made from a source
checkout. A released binary carries real values in all six fields.

## Errors

`version` has no failures of its own. It reads values already inside the
binary, so it succeeds whether or not a scanner is attached and whether or not
one has been selected. The only failures it can produce are the ones any
command can produce, which are listed in [global flags](../global_flags.md):
an invalid `--output` value, or a config file that is missing or unparseable.
