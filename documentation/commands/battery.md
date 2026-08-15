# battery

Reports how much charge the scanner has left and whether it is charging. Run it
to check the battery without picking the scanner up.

## Overview

`battery` asks the scanner's charger for its readings and prints them: the
remaining charge as a percentage, what the charger is doing, the battery
voltage, the current flowing in or out, and the battery temperature. The
current tells you which way the charge is moving, so a scanner that is plugged
in but not actually charging is visible here rather than only as a battery
that never fills. The command reads only: it changes nothing on the scanner
and writes nothing to the config file. It needs a scanner, so name one with
`--device`.

## Usage

```
radiocli battery [flags]
```

## Parameters

`battery` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | — | — | This command takes no flags and no arguments. |

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column
  of [`devices`](devices.md). Without it the command fails with
  `no scanner named`.
- `--output` selects whether the readings are printed as lines or as JSON.

`--pace` has no effect here. This command presses no keys.

## Examples

Reading the battery while the scanner is charging:

```
$ radiocli battery
charge:      32%
state:       charging
voltage:     3.83 V
current:     643 mA (charging)
temperature: 38.2 C (100.9 F)
```

The same reading as JSON:

```
$ radiocli battery -o json
{
  "state": "charging",
  "charging": true,
  "percent": 32,
  "volts": 3.83,
  "milliamps": 643,
  "celsius": 38.25,
  "fahrenheit": 100.85,
  "needsAction": false
}
```

Reading a scanner other than the selected one:

```
radiocli --device /dev/cu.usbmodem00000000000011 battery
```

## Output

The readings go to stdout. A charger fault warning goes to stderr, as do debug
logs from `--verbose`.

Under `--output text`, stdout holds five lines, with the labels padded so the
values line up:

```
charge:      32%
state:       charging
voltage:     3.83 V
current:     643 mA (charging)
temperature: 38.2 C (100.9 F)
```

The `current` line names the direction in brackets: `charging` when the value
is positive, `discharging` when it is negative, and `no flow` when it is zero.

When the charger reports a fault, a warning follows on stderr:

```
The charger reports a problem: abnormal temperature. Check the power supply and
let the scanner reach room temperature before charging it.
```

Under `--output json`, stdout holds one object. All eight fields are always
present:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `state` | string | What the charger is doing. One of `not charging`, `initializing`, `abnormal temperature`, `abnormal power`, `fully charged`, `topping up`, `charging`. |
| `charging` | boolean | Whether the battery is gaining charge. True when `state` is `charging` or `topping up`. |
| `percent` | number | Remaining capacity, from `0` to `100`. |
| `volts` | number | Battery voltage, such as `3.83`. |
| `milliamps` | number | Current flowing. Positive while charging, negative while running on the battery. |
| `celsius` | number | Battery temperature, such as `38.25`. |
| `fahrenheit` | number | The same temperature in Fahrenheit, such as `100.85`. Converted by this tool; the scanner reports only Celsius. |
| `needsAction` | boolean | True when `state` is `abnormal temperature` or `abnormal power`, which are the two states that need attention rather than reporting progress. |

The text output rounds the voltage to two decimal places and both temperatures
to one. The JSON reports them unrounded.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device, or run "radiocli devices" to see what is attached` | No port was passed with `--device`. | Pass `--device` with a port from the `port` column of [`devices`](devices.md). |
| `error: opening <port>: no such file or directory (run "radiocli devices" to see what is attached)` | The named port does not exist. | Run [`devices`](devices.md) to find the current port, and pass that. |
| `error: reading the battery: <detail>` | The scanner answered, but not with a battery reading it could parse. | Run with `--verbose` to see the raw exchange, and check the firmware version with [`status`](status.md). |

A charger fault is not an error. `abnormal temperature` and `abnormal power`
are printed as readings with a warning, and the command exits with status `0`,
because the scanner answered the question it was asked.
