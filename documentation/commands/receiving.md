# receiving

Reports what the scanner is hearing at this instant. Run it when the scanner has
stopped on something and you want to know what.

## Overview

A scanner spends most of its time working through its lists and stopping
whenever a channel is busy. `receiving` takes a reading at the moment you run
it: whether audio is coming out of the scanner right now, which channel it is
on, where that channel lives in its memory, and the frequency or talkgroup
behind it. The reading is instantaneous rather than a summary, and the first
field is the one to read. **A scanning radio still names a channel**, because it
answers with whichever channel it happened to be checking at the instant it was
asked, and that reply is indistinguishable from one about a real transmission
unless you look at `receiving` first. When `receiving` is `no`, the command says
so on stderr as well. The command presses no keys and changes nothing on the
scanner or on your computer, so it is safe to run repeatedly and safe to run
while another `radiocli` command has the scanner. It needs a scanner, so name
one with `--device`.

## Usage

```
radiocli receiving [flags]
```

## Parameters

`receiving` has no flags of its own. Its behaviour is controlled entirely by the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| none | - | - | This command takes no flags and no arguments. |

### Global flags that change this command

- `--device` names the scanner to read. Get the value from the `port` column of
  [`devices`](devices.md).
- `-o`, `--output` selects whether the reading is printed as lines or as JSON.
  Both carry the same fields.
- `-v`, `--verbose` adds debug lines about the exchange with the scanner to
  stderr.

`--pace` has no effect here. This command presses no keys.

## Examples

A scanner stopped on a transmission:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 receiving
receiving:  yes
list:       Pocahontas County
system:     PUBLIC SAFETY
department: POLICE DEPARTMENT
channel:    MARLINTON DISPATCH
frequency:  155.550000MHz
signal:     3
```

The same scanner a moment later, working through its lists again:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 receiving
receiving:  no
list:       Pocahontas County
system:     PUBLIC SAFETY
department: PUBLIC WORKS
channel:    CH 2
frequency:  151.025000MHz
signal:     0

Nothing is being received. The channel above is the one the scanner happened to be checking as it scanned past, not one it stopped on.
```

Note that the channel fields are filled in both times. Only `receiving` tells
the two apart, which is why the note is printed.

The same reading as JSON, which is the form to read from a script:

```
$ radiocli --device /dev/cu.usbmodem00000000000011 receiving -o json
{
  "receiving": true,
  "list": "Pocahontas County",
  "system": "PUBLIC SAFETY",
  "department": "POLICE DEPARTMENT",
  "channel": "MARLINTON DISPATCH",
  "frequency": "155.550000MHz",
  "modulation": "NFM",
  "signal": "0",
  "rssi": "-87",
  "mode": "Scan Mode"
}
```

Watching the traffic go by, printing only the transmissions:

```
$ while :; do radiocli --device /dev/cu.usbmodem00000000000011 receiving -o json | jq -c 'select(.receiving)'; done
{"receiving":true,"list":"Pocahontas County","system":"PUBLIC SAFETY","department":"POLICE DEPARTMENT","channel":"MARLINTON DISPATCH","frequency":"155.550000MHz","modulation":"NFM","signal":"3","rssi":"-87","mode":"Scan Mode"}
```

To keep the audio of those transmissions rather than only their names, run
[`audio record`](audio.md#audio-record), which uses this same reading to label
every file it writes.

## Output

The reading goes to stdout. The note about nothing being received goes to
stderr, so `2>/dev/null` leaves stdout holding only the reading.

Under `--output text` the command prints one labelled line per field. A field
the scanner did not name is printed as `-`. The `site`, `talkgroup` and `unit`
lines are printed only when they have a value: `site` and `talkgroup` appear on
a trunked system, and `frequency` appears in place of `talkgroup` on a
conventional one. `unit` appears only when the scanner decoded a unit ID.

Under `--output json` the command prints one object. Every field except
`receiving` and `mode` is omitted when it is empty.

| Field | Type | Description |
| ----- | ---- | ----------- |
| `receiving` | boolean | Whether audio is coming out of the scanner right now. Always present. |
| `list` | string | The favorites list or database the channel was found in. |
| `system` | string | The system the channel belongs to. |
| `department` | string | The department the channel belongs to. |
| `site` | string | The trunked site being listened to. Absent on a conventional system. |
| `channel` | string | The channel's alpha tag. |
| `frequency` | string | What the scanner is tuned to on a conventional system, carrying its unit, such as `155.550000MHz`. Absent on a trunked system. |
| `talkgroup` | string | The talkgroup number on a trunked system. Absent on a conventional system. |
| `unit` | string | The radio heard transmitting, when the scanner decoded one. |
| `modulation` | string | How the scanner is demodulating, such as `NFM`. |
| `signal` | string | The number of signal bars, from `0` to `5`. A string because the scanner reports it as one. |
| `rssi` | string | The received signal strength in the scanner's own units. Reads `-999` when nothing is coming in, which means the scanner has nothing to report rather than a measurement of -999. |
| `mode` | string | What the scanner is doing, in its own words, such as `Scan Mode`. Always present. |

**`receiving` is the mute state, not the signal strength.** The scanner opens
its audio gate at the start of a transmission and its signal reading catches up
a moment later, so `signal` reads `0` on the first reading of a transmission
that is already audible. Use `receiving` to tell whether something is coming in,
and `signal` to tell how strong it is once it has.

**`unit` is empty more often than you would expect.** It is empty on every
analog channel, because there is no unit ID to decode, and empty on a digital
one whose transmission had already begun when the scanner stopped on it.

## Errors

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named` | No scanner was given. | Pass `--device` with the port from [`devices`](devices.md). |
| `error: opening <port>: no such file or directory (run "radiocli devices" to see what is attached)` | The port does not exist. | Check the scanner is plugged in and switched on, and run [`devices`](devices.md) for the current port. |
| `error: <port> is in use by another radiocli` | Another invocation has the scanner. | Wait for it, pass `--wait` to queue behind it, or start a [`daemon`](daemon.md) so several commands can share the scanner. |
| `error: asking the scanner what it is hearing: ...` | The scanner stopped answering part way through the exchange. The text after the colon comes from the connection. | Check the cable, then run [`status`](status.md) to confirm the scanner is still answering. |
