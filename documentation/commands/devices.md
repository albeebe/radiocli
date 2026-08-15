# devices

Lists every scanner attached to your computer. Run it when you want to see what
is plugged in, or to find the port to pass to `--device`.

## Overview

`devices` checks each USB serial port on your computer, asks what is on the
other end, and lists the ones that answer as a scanner. For each it reports the
model, the USB serial number, and the serial port. It changes nothing: it does
not connect for longer than the check takes, it does not write the config file,
and it does not alter any setting on the scanner. Finding nothing attached is a
complete answer rather than a failure, so the command still succeeds and prints
advice on what to check.

This is the command that answers "what do I pass to `--device`". The tool
remembers no scanner, so every command that talks to one is told which one, and
the `PORT` column is where that value comes from.

This command is also available as `list`. The two names behave identically.

## Usage

```
radiocli devices [flags]
```

```
radiocli list [flags]
```

## Parameters

`devices` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | — | — | This command takes no flags and no arguments. |

### Global flags that change this command

- `--output` selects whether the list is printed as a table or as JSON. The
  table and the JSON contain the same scanners.

`--device` is accepted, as it is everywhere, but changes nothing here. This
command is how you find a port rather than a command you point at one, so it
always checks every USB serial port and always lists every scanner that
answers.

## Examples

Listing the attached scanners:

```
$ radiocli devices
MODEL   SERIAL         PORT
SDS150  0000000000001  /dev/cu.usbmodem00000000000011
```

Taking the port and using it:

```
$ radiocli status --device /dev/cu.usbmodem00000000000011
```

The same list as JSON:

```
$ radiocli devices -o json
[
  {
    "port": "/dev/cu.usbmodem00000000000011",
    "model": "SDS150",
    "serial": "0000000000001",
    "busy": false
  }
]
```

Watching which ports get checked:

```
$ radiocli -v devices
time=2026-08-02T13:10:55.904-04:00 level=DEBUG msg="scanning for attached scanners"
time=2026-08-02T13:10:55.911-04:00 level=DEBUG msg="found scanner" port=/dev/cu.usbmodem00000000000011 model=SDS150
MODEL   SERIAL         PORT
SDS150  0000000000001  /dev/cu.usbmodem00000000000011
```

## Output

The list goes to stdout. The advice printed when nothing is found, and the note
about ports in use, go to stderr.

Under `--output text`, stdout holds a header row and one row per scanner, with
columns aligned by padding:

| Column | Description |
| ------ | ----------- |
| `MODEL` | Model reported by the scanner, such as `SDS150`. `-` when another `radiocli` is using the port, because the model cannot be read without interrupting it. |
| `SERIAL` | USB serial number, or `-` when the port reports none and when another `radiocli` is using the port. |
| `PORT` | Serial port, which is the value to pass to `--device`. |

A port another `radiocli` is using is listed as a row with `-` for its model
and serial number, and a line naming it follows the table on stderr:

```
In use by another radiocli, so not identified: /dev/cu.usbmodem00000000000012
```

When every port found is in use, no table is printed at all, and only the
advice shown at the end of this file goes to stderr.

Under `--output json`, stdout holds an array with one object per scanner. The
array is empty when nothing is attached:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `port` | string | Serial port of the scanner, which is the value to pass to `--device`. |
| `model` | string | Model reported by the scanner. Empty when `busy` is `true`, because the model cannot be read without interrupting whoever holds the port. |
| `serial` | string | USB serial number. Absent when the port reports none, and absent when `busy` is `true`. |
| `busy` | boolean | Whether another `radiocli` is using this port. `true` means the scanner is attached and working but is held by another command. |
| `shared` | boolean | Whether the thing holding it is a [`daemon`](daemon.md), which runs commands on the scanner for anybody who asks. Absent when false. |

`port` and `busy` are always present. A port in use by another `radiocli` is
in the array like any other, so a program reading this list must check `busy`
before trying to use a port:

```
$ radiocli devices -o json
[
  {
    "port": "/dev/cu.usbmodem00000000000011",
    "model": "",
    "busy": true,
    "shared": true
  }
]
```

**`busy` and `shared` together are the useful test, not `busy` alone.** A busy
port that is shared can be used right now: pass it to `--device` as usual and
the command queues behind whatever else is running instead of being refused. A
busy port that is not shared is a plain command in the middle of something, and
that one has to be waited for. A program that treats every busy port as
unusable will refuse to work against a scanner that is available to it.

`shared` is answered by connecting to the daemon's socket rather than by looking
for the socket file, because a daemon that was killed leaves its socket behind
and one nobody is listening on is the same answer as none at all. The connection
is dropped straight away, so asking costs nothing and queues nothing.

Nothing attached prints `[]` under `--output json`, and under `--output text`
prints nothing to stdout while the advice goes to stderr. Both exit with status
`0`. A valid JSON document is written to stdout however discovery went, so
`--output json` never produces empty output.

## Errors

`devices` has no failures of its own beyond the ones any command can produce,
which are listed in [global flags](../global_flags.md). Finding no scanners is
not an error: the command prints advice to stderr and exits with status `0`.

Ports that cannot be checked are skipped rather than reported. Only USB serial
ports are checked at all, so a built-in or Bluetooth serial port never appears
and is never mentioned. A USB port that is already held by another program, or
that does not answer within one second, is left out of the list silently. Run
with `--verbose` to see each USB port that was checked and rejected, and why.

A port another `radiocli` is using is the exception, because it is almost
certainly the scanner and nothing is wrong with it. It is left alone rather
than interrupted, and it is listed with `busy` set rather than dropped. If it
was the only candidate, that is said rather than reported as nothing being
attached:

```
$ radiocli devices
every scanner found is in use by another radiocli: /dev/cu.usbmodem00000000000011

Wait for it to finish and run this command again
```

A port held by a [`daemon`](daemon.md) says something different, because
waiting is not what to do about it:

```
$ radiocli devices

Held by an radiocli daemon, which runs commands on it for anybody who asks: /dev/cu.usbmodem00000000000011
Pass one to --device as usual. Commands queue behind whatever else is running
rather than being refused.
```

Both exit with status `0`. See [`--wait`](../global_flags.md) for why only one
command uses a scanner at a time.
