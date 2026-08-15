# config

Reports and changes the settings this tool keeps for itself, such as how to
render results. Use it when you want a choice to stick from one run to the next
instead of typing it every time.

## Overview

Every setting here belongs to `radiocli` rather than to the scanner. Nothing
in this command opens the serial port, sends anything to the radio, or changes
anything on it, and no scanner needs to be attached for any of it to work.
Settings are kept in a file on your computer, and every command reads that file
when it starts, so a setting written once applies to every command afterwards.
A setting can always be overridden for a single run by passing the matching
global flag, and doing so changes only that run: the flag is not written to the
file. What this command reports is what the file holds, not what the current run
resolved, so `radiocli -o json config` renders its answer as JSON while still
reporting that the saved `output` is `text`. The three settings are `output`,
`pace` and `verbose`. The file also holds the macros a front end offers as
buttons, which are a list rather than a single value and have a subcommand of
their own, [`config macro`](config_macro.md).

## Usage

```
radiocli config [flags]
radiocli config get <name> [flags]
radiocli config set <name> <value> [flags]
radiocli config unset <name> [flags]
radiocli config path [flags]
radiocli config macro [flags]
```

`config macro` has subcommands and flags of its own, and is documented in
[config_macro.md](config_macro.md).

## Parameters

The bare command takes no arguments and no flags of its own. Its subcommands
take positional arguments and no flags of their own.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<name>` | Yes | none | Which setting to read or change, for `get`, `set` and `unset`. |
| `<value>` | Yes | none | The value to store, for `set` only. |

### `<name>`

One of exactly three settings. Any other name is refused and nothing is written.

| Name | Values | Default | What it does |
| ---- | ------ | ------- | ------------ |
| `output` | `text` or `json` | `text` | How results are rendered. |
| `pace` | `slow`, `medium`, `fast` or `turbo` | `turbo` | How quickly keys are sent to the scanner. |
| `verbose` | `true` or `false` | `false` | Whether the exchange with the scanner is logged to stderr. |

Five names are refused with a reason rather than treated as unknown. `device`
is never a setting, because which scanner to talk to is a property of the
invocation rather than of the machine, so it is only ever the global `--device`
flag; run [`devices`](devices.md) to see what is attached. `wait` is never a
setting, because how long to wait for another `radiocli` to finish with the
scanner is a property of the caller rather than of the machine, so it is only
ever the global `--wait` flag. `config` is never a setting, because the
config file cannot name itself; pass the global `--config` flag to read or write
a different file. `macro` and `macros` are never settings, because a macro is a
list of commands rather than a single value and there can be any number of them;
they are read and written by [`config macro`](config_macro.md).

### `<value>`

The value to store. Only the values listed above are accepted, and anything
else is refused before the file is touched.

## Examples

Reading every setting on a machine where nothing has been set:

```
$ radiocli config
NAME     VALUE  DEFAULT
output   text   text
pace     turbo  turbo
verbose  false  false
```

Reading one setting, with nothing around it, so it can be used directly:

```
$ radiocli config get pace
turbo
```

Slowing the keypresses down for a scanner that drops them:

```
$ radiocli config set pace medium
pace: medium
```

Seeing what has been changed from its default:

```
$ radiocli config
NAME     VALUE   DEFAULT
output   text    text
pace     medium  turbo
verbose  false   false
```

Putting one back:

```
$ radiocli config unset pace
pace: turbo
```

Finding the file:

```
$ radiocli config path
/tmp/cfgtest/Library/Application Support/radiocli/config.json
```

The path is your account's own config directory with `radiocli/config.json`
inside it, so it will not be the one above: that run used a throwaway home
directory so this file would not name anybody's real one.

One setting as JSON:

```
$ radiocli -o json config get pace
{
  "name": "pace",
  "value": "medium",
  "default": "turbo",
  "changed": true
}
```

## Output

Results go to stdout. Advice goes to stderr, as do debug logs from `--verbose`.

Under `--output text`, the bare command writes a header row and one row per
setting:

| Column | Description |
| ------ | ----------- |
| `NAME` | The setting's name, as `get`, `set` and `unset` take it. |
| `VALUE` | What is in effect with no flag overriding it: the file's value, or the default when the file has none. |
| `DEFAULT` | What the value would be with nothing set. |

A value that is empty is written as `-` in both columns.

Under `--output json`, the bare command writes an array of objects, in the
order the table lists them:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | The setting's name. |
| `value` | string | What is in effect. Empty string when nothing is set and there is no default. |
| `default` | string | What the value would be with nothing set. Empty string when there is none. |
| `changed` | boolean | Whether `value` and `default` differ. |

Every value is a string in JSON, including `verbose`, which is `"true"` or
`"false"` rather than a boolean.

`config get` writes the value alone under `--output text`, with no name and no
padding, and one object with the same four fields under `--output json`.

`config set` and `config unset` write the setting back after the file has been
written, in the form `name: value` under `--output text` and as one object under
`--output json`. The value is read back from the file rather than echoed, so
what is printed is what was stored.

`config path` writes the path alone under `--output text`. When the file does
not exist, it also writes `That file does not exist yet. It is written the
first time a setting is set.` to stderr, so stdout still holds nothing but the
path. Under `--output json` it writes one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `path` | string | The config file this invocation reads and writes. |
| `exists` | boolean | Whether that file is there. |

**`config set output json` takes effect immediately**, so that command renders
its own confirmation as JSON.

**The file is created by the first `set` or `unset`**, along with any missing
directories, and is written with permissions `0600`. Settings other than the one
named are left as they are.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no setting called "<name>": the settings are output, pace, verbose` | That name is not a setting. | Use one of the three names in the message. Nothing was written. |
| `error: "device" is not a setting: which scanner to talk to is a property of the invocation rather than of this machine, so it is only ever the --device flag: run "radiocli devices" to see what is attached` | `device` is a flag, never a stored setting. The tool remembers no scanner. | Pass `--device` on every command that talks to one. Nothing was written. |
| `error: "wait" is not a setting: how long to wait for another radiocli is a property of the caller rather than of this machine, so it is only ever the --wait flag` | `wait` is a flag, never a stored setting. | Pass `--wait` on the command that needs it. Nothing was written. |
| `error: "config" is not a setting: the config file cannot name itself: pass --config to read or write a different one` | The path of the config file is not kept inside it. | Pass `--config` to use a different file. Nothing was written. |
| `error: "macros" is not a setting: a macro is a list of commands rather than a single value, and there can be any number of them, so they have a command of their own: run "radiocli config macro"` | Macros are a list, so they are not reachable through `get`, `set` or `unset`. The same message answers `macro`. | Use [`config macro`](config_macro.md). Nothing was written. |
| `error: invalid pace "instant": want slow, medium, fast or turbo` | That is not one of the accepted paces. | Use one of the four values in the message. Nothing was written. |
| `error: invalid output "yaml": want text or json` | That is not one of the accepted formats. | Use `text` or `json`. Nothing was written. |
| `error: invalid verbose "maybe": want true or false` | That is not a true or false value. | Use `true` or `false`. Nothing was written. |
| `error: accepts 1 arg(s), received 0` | `get` or `unset` was given no name. | Give one of the three setting names. |
| `error: accepts 2 arg(s), received 1` | `set` was given a name and no value. | Give a value after the name. |
| `error: reading <path>: open <path>: no such file or directory` | A file named with `--config` is not there. | Create it, or leave `--config` off to use the default file. A missing default file is not an error. |
