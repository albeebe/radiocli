# status

Connects to the scanner you name and reports what it is. Run it to confirm the
scanner is plugged in and answering before you rely on it.

## Overview

`status` opens the scanner named with `--device`, asks it for its firmware
version, how it is drawing its screen, and what it is doing, and prints those
along with the model and the port. It is the quickest way to tell the
difference between a scanner that is attached and working, one that is
attached but not answering, and one that was never named, because each of
those produces a different message. The command reads only: it changes nothing
on the scanner and writes nothing to the config file.

## Usage

```
radiocli status [flags]
```

## Parameters

`status` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | — | — | This command takes no flags and no arguments. |

### Global flags that change this command

- `--device` names the scanner to report on. Get the value from the `port`
  column of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the report is printed as lines or as JSON.

## Examples

Reporting the selected scanner:

```
$ radiocli status
port:     /dev/cu.usbmodem00000000000011
model:    SDS150
firmware: Version 1.00.37
display:  color
mode:     Scan Mode
```

The same report as JSON:

```
$ radiocli status -o json
{
  "port": "/dev/cu.usbmodem00000000000011",
  "model": "SDS150",
  "firmware": "Version 1.00.37",
  "display": "color",
  "mode": "Scan Mode",
  "holding": false
}
```

Reporting a scanner other than the selected one, without changing the
selection:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 status
port:     /dev/cu.usbmodem00000000000011
model:    SDS150
firmware: Version 1.00.37
display:  color
mode:     Scan Mode
```

Reporting a scanner that is parked on one channel rather than scanning:

```
$ radiocli status
port:     /dev/cu.usbmodem00000000000011
model:    SDS150
firmware: Version 1.00.37
display:  color
mode:     Scan Hold

The scanner is holding rather than scanning. Run "radiocli scan" to release it.
```

Running without naming a scanner:

```
$ radiocli status
error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached
```

## Output

The report goes to stdout. Debug logs from `--verbose` go to stderr.

Under `--output text`, stdout holds five lines, with the labels padded so the
values line up:

```
port:     /dev/cu.usbmodem00000000000011
model:    SDS150
firmware: Version 1.00.37
display:  color
mode:     Scan Mode
```

A scanner that is holding gets one further line, on **stderr** rather than
stdout, so it does not land in a parsed reading:

```
The scanner is holding rather than scanning. Run "radiocli scan" to release it.
```

Under `--output json`, stdout holds one object. All six fields are always
present:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `port` | string | Serial port the scanner was reached on. |
| `model` | string | Model reported by the scanner, such as `SDS150`. |
| `firmware` | string | Firmware version reported by the scanner, such as `Version 1.00.37`. The scanner supplies the whole string, including the word `Version`. |
| `display` | string | How the scanner is drawing its screen: `color`, `black`, or `white`. See [`display`](display.md), which reports the same value with the scanner's own wording for it. |
| `mode` | string | What the scanner is doing, in its own words: `Scan Mode`, `Scan Hold`, `Trunk Scan`, `Quick Search Hold`, `Menu tree` and so on. |
| `holding` | boolean | Whether the scanner is parked on one thing rather than working through a list. True exactly when `mode` ends in `Hold`. |

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: opening <port>: no such file or directory (run "radiocli devices" to see what is attached)` | The named port does not exist. Unplugging and replugging a scanner can change its port. | Run [`devices`](devices.md) to find the current port, and pass that. |
| `error: identifying scanner on <port>: sending "MDL": no response from scanner: context deadline exceeded (run "radiocli devices" to see what is attached)` | The port opened, but nothing answered as a scanner within three seconds. This is what a non-scanner serial port produces. | Check the port with [`devices`](devices.md), and close any other program holding it. |
| `error: reading the firmware version: <detail>` | The scanner identified itself but did not answer the firmware request. | Check the cable, and confirm the scanner is not busy in a menu that stops it answering. |
| `error: reading the display mode: <detail>` | The scanner answered the firmware request but not the status request that carries the display mode. | Run with `--verbose` to see the raw exchange, and check the cable. |
| `error: asking the scanner what it is doing: <detail>` | The scanner did not answer the request that carries its mode. | Run with `--verbose` to see the raw exchange, and check the cable. |
