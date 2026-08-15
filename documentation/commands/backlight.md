# backlight

Reports whether the scanner's light is on, and switches it on or off. Run it to
light the scanner up without reaching for it, or to check whether it is lit.

## Overview

The scanner has one key for its light, and that key toggles. Pressing it blind
is therefore as likely to put the light out as to switch it on, which is why
this command exists: it reads the light first and presses only if the press is
the one that gets the answer asked for. Asking for a light that is already on
does nothing at all.

Two different things are called the backlight here. The **light** is the screen,
and the keypad too if the keypad is switched on to it. It is momentary and does
not survive a power cycle. The **keypad light** is a setting held in a menu that
decides whether the keys join in when the light comes on; it is permanent and is
what [`backlight keys`](#backlight-keys) reads and writes.

The two are connected in a way nothing on the scanner mentions: **changing the
setting while the light is already on appears to do nothing**, because the
scanner only acts on it at the moment the light comes on. Every command here
that changes the setting cycles the light afterwards, so the keys light up when
they are supposed to and nobody has to know why.

Reading the light changes nothing and does not stop the scan. Reading or
writing the keypad setting means opening a menu, which stops the scan for a
few seconds and returns it afterwards. It needs a scanner, so name one with
`--device`.

## Usage

```
radiocli backlight [flags]
radiocli backlight on [--keys] [flags]
radiocli backlight off [flags]
radiocli backlight keys [flags]
radiocli backlight keys enable [flags]
radiocli backlight keys disable [flags]
radiocli backlight keys toggle [flags]
```

## Parameters

The bare command and every subcommand except `on` take no arguments and no
flags of their own.

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--keys` | No | `true` | For `backlight on`: switch the keypad light on as well, if it is off. |

### `--keys`

Whether `backlight on` also makes the keypad light up, rather than lighting only
the screen.

Left alone it is on, because a request to light the scanner up that leaves half
of it dark is not what was asked for. It costs a few seconds: the setting is
only readable from the menu that holds it, so the command has to open that menu
to find out whether anything needs changing. That is true even when the setting
is already correct and nothing gets written.

Pass `--keys=false` to skip all of that and press the light key alone. On an
SDS150, that is the difference between about 2.5 seconds and 0.17:

```
$ radiocli backlight on --keys=false
backlight: on
level:     3
```

`--keys=false` never changes the setting, so a keypad that was dark stays dark
and one that was lit stays lit.

Global flags such as `--output` and `--device` work here as they do everywhere;
see [global flags](../global_flags.md).

## Examples

Checking whether the scanner is lit, which presses nothing:

```
$ radiocli backlight
backlight: on
level:     3
```

Lighting the scanner up, keypad and all:

```
$ radiocli backlight on
backlight: on
level:     3
```

Putting the light out:

```
$ radiocli backlight off
backlight: off
```

Running `off` again leaves it off rather than switching it back on, and the same
is true of `on`. That is the whole point of reading before pressing.

Asking whether the keypad lights up with the screen:

```
$ radiocli backlight keys
keypad light: on
```

Switching the keypad light off, which leaves the screen lighting up as before:

```
$ radiocli backlight keys disable
keypad light: off
```

## `backlight keys`

The keypad light setting lives at `Display Options` → `Backlight Options` →
`Set Key Backlight` on the scanner, and offers `Enable` and `Disable`.

Reading it means walking to that menu, which stops the scan. The command reads
which entry the scanner has selected, then leaves the menus and returns the
scanner to scanning. Writing it selects the other entry and **reads the setting
back** before reporting success, so a press that did not take is visible rather
than assumed.

The setting survives a power cycle. The light itself does not.

`toggle` sets it to whichever it is not, which is one command for both and is
what a button wants when it has no way of knowing which way round things are. It
costs the same single walk to the menu that `enable` and `disable` do, because
the value it decides from is the one it had to read anyway.

```
$ radiocli backlight keys toggle
keypad light: off

$ radiocli backlight keys toggle
keypad light: on
```

**Every verb here changes the keypad and not the screen.** The screen's own
light is a different thing, switched by `on` and `off` above, and none of these
turns it off. The one visible effect they have on it is the cycle described
above: if the screen is lit when the setting changes, it blinks off and back on
so the scanner acts on the new value, and ends up lit exactly as it was.

It is the built-in macro `Toggle Backlight`; see [macros](../global_flags.md).

## Output

Results go to stdout. Advice and any note about what the scanner is doing go to
stderr, as do debug logs from `--verbose`, so redirecting stderr leaves only the
answer.

Under `--output text`, `backlight`, `on` and `off` print the state, and the
level only when the light is on, because a level of zero says nothing a reader
does not already have:

```
backlight: on
level:     3
```

`backlight keys`, `enable`, `disable` and `toggle` print one line:

```
keypad light: on
```

Under `--output json`, `backlight`, `on` and `off` print one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `on` | boolean | Whether the scanner is lit. |
| `level` | number | The brightness while lit, `0` while dark. It is the dimmer setting, from `0` to `3`. |

```
$ radiocli backlight -o json
{
  "on": true,
  "level": 3
}
```

`backlight keys`, `enable`, `disable` and `toggle` print one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `enabled` | boolean | Whether the keypad lights up with the screen. |

```
$ radiocli backlight keys -o json
{
  "enabled": true
}
```

## Where the state comes from

**The light is readable, which is unusual for this scanner.** It is the last
field of the status response, and it holds the dimmer level while lit and `0`
while dark. Measured on an SDS150 running firmware 1.00.37: pressing the light
key moves the field between `3` and `0`.

That is what makes `on` and `off` honest rather than hopeful. Both read the
scanner and press only when the press is the one that gets the answer asked
for, and both read it again afterwards, so what they print is a reading rather
than a claim.

**The scanner switches the light on by itself, and this command has no say in
it.** `Backlight Options` → `Set Timer` chooses the event that lights it, and on
the scanner tested that is `Squelch`: the light comes on every time a
transmission is received, and goes out again afterwards. `Keypress` is the other
choice.

So the state can change a moment after any of these commands run, and the change
has nothing to do with them. On a scanner that is actively scanning a busy
system this happens constantly. Two consequences worth knowing:

- **What `backlight` reports is true when it is read and need not be true a
  second later.** Nothing is wrong if two readings disagree.
- **`backlight off` can be undone immediately** by a transmission arriving. The
  command does not fight that, and running it again is the only answer.

**The keypad setting is not in that response.** The status field reads the same
whether the keypad light is enabled or disabled, which is why reading the
setting costs a walk through the menus while reading the light costs nothing.

## Errors

Every failure exits with status `1` and prints to stderr.

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: the light did not come on after pressing the light key` | The key was pressed but the scanner still reports the light as it was. | Run `radiocli backlight` to see the state, and try again. The key may have arrived while the scanner was busy. |
| `error: the light did not come off after pressing the light key` | The same, in the other direction. A transmission arriving at that moment relights the scanner on its own, which can cause this. | Run it again. |
| `error: looking for "Display Options": <detail>` | The walk to the keypad setting could not find a menu entry. Different firmware may name or place it differently. | Run `radiocli scan` to return the scanner to scanning, then check the menu by hand with `radiocli menu open top`. |
| `error: choosing "Enable" for the keypad light: <detail>` | The setting screen was reached but the scanner would not take the choice. | Run `radiocli scan`, then try again. |
| `error: the keypad light is still off after setting it to on` | The choice was made and the scanner still reports the old value. | Check by hand with `radiocli backlight keys`. |
| `error: the keypad light setting shows "<text>", which is neither "Enable" nor "Disable"` | The walk landed somewhere unexpected. | Run `radiocli scan`, then look with `radiocli screen`. |
| `error: response to "STS" has an invalid backlight: "<value>"` | The scanner reported something other than a number for the light. | Run with `--verbose` to see the raw exchange. |
| `error: no scanner named` | No port was passed with `--device`. | Pass `--device` with a port from [`devices`](devices.md). |
