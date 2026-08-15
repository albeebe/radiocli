# key

Presses keys on the scanner's front panel, as though someone had pressed them.
Run it to reach a screen or setting no other command covers.

## Overview

Everything else in this tool asks the scanner's remote protocol for something.
`key` is the exception: it drives the scanner's own interface. That makes it the
most capable command here, because it can reach anything a person sitting in
front of the radio can reach, and the least predictable, because what a key
does depends entirely on what is already on the screen. Prefer a command that
names what it does whenever one exists.

Several keys may be given at once and are pressed in the order written.
Presses are paced by [`--pace`](../global_flags.md), which leaves a gap
between one key and the next so the scanner has time to act and redraw; this
is the only family of commands that flag affects. Every key name is checked
before the scanner is opened, so a typo in a run of five presses none of them.
It needs a scanner, so name one with `--device`.

## Usage

```
radiocli key <name>... [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `<name>...` | Yes | none | One or more keys to press, in order. |
| `--action` | No | `press` | How to press them: `press`, `long`, `hold`, `release`. |

### `<name>...`

The keys to press. Names are what the key is called on the scanner, not the
single letters the protocol uses.

| Name | Key |
| ---- | --- |
| `menu` | The hamburger key. Opens the menus from a scanning screen, climbs one level from inside them. |
| `function` | The modifier selecting each key's second label. Does nothing on its own. |
| `avoid` | The AVOID key. Inside the menus it leaves them entirely, from every screen tested. Outside them it avoids the current channel, so do not press it blindly; [`scan`](scan.md) checks first. |
| `enter`, `yes` | The E/YES key: selects the highlighted item. |
| `no` | The `.` key, labelled NO. |
| `right`, `left` | Turn the knob, moving the selection down and up by one. |
| `push` | Press the knob in, which selects like `enter`. |
| `soft1`, `soft2`, `soft3` | The three unlabelled keys above 1, 2 and 3. `soft1` leaves most menus, but was refused on the text entry screen where `avoid` worked. |
| `replay` | REPLAY, or RECORD when shifted with `function`. |
| `zip` | ZIP, or SERVICES when shifted with `function`. |
| `backlight` | The combined backlight and power key. |
| `squelch` | Presses the squelch control in. |
| `0` to `9` | The number keys. |
| `range`, `service-type` | In the protocol's key table but absent on this model. Accepted, and do nothing. |

The scanner answers `KEY,OK` for every code it is sent, including codes it has
no key for, so a key that does nothing reports success rather than an error.

### `--action`

How the key is pressed. `press` is a normal press and release. `long` is a long
press, which many keys treat as a second function. `hold` holds a key down and
`release` lets it go, so the two are used as a pair.

The action applies to every key in the invocation.

```
radiocli key backlight --action long
```

Sending `backlight` with `--action long` is how the scanner is turned off.
There is no separate power command yet, so take care with that combination.

## Examples

Opening the menus and stepping down two entries:

```
radiocli key menu right right
```

Selecting the highlighted entry:

```
radiocli key enter
```

Leaving the menus from anywhere, including screens that refuse
[`menu close`](menu.md). Prefer [`scan`](scan.md), which does this and checks
that it worked:

```
radiocli key avoid
```

Typing a number into a direct entry screen:

```
radiocli key 1 5 5 4 7 5
```

Slowing the presses down on a screen that is redrawing heavily:

```
radiocli --pace slow key menu right right enter
```

## Output

Under `--output text`, which is the default, `key` prints nothing when it
succeeds. It changes the scanner rather than reporting anything, and what it
changed is on the scanner's screen. Use [`menu`](menu.md) to see where a run of
presses landed.

Under `--output json` it prints a report of what it pressed:

```
$ radiocli key menu right push -o json
{
  "keys": [
    "menu",
    "right",
    "push"
  ],
  "action": "press"
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `keys` | array of string | The keys that were pressed, in the order they were pressed, by the names you used rather than the protocol's letters. |
| `action` | string | How they were pressed, lowercased: `press`, `long`, `hold` or `release`. |

**It says what was asked for, not what happened.** What a key does depends
entirely on what was on screen when it landed, and this command has no way to
find that out. Read the screen afterwards if you need the result.

Silence is right for a person and wrong for a program: empty stdout is not
something a decoder can read, which is why the two modes differ here at all.

## Errors

Every failure exits with status `1` and prints to stderr.

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no key is called "<name>": want ...` | The name is not one of the keys listed above. | Use a name from the table. Nothing was pressed. |
| `error: invalid action "<value>": want hold, long, press, release` | `--action` is not one of the four. | Use one of the four. Nothing was pressed. |
| `error: pressing "<name>": <detail>` | The scanner refused that key. | Check what is on its screen; some keys are refused on some screens. |
| `error: pressing "<name>", after <names>: <detail>` | A run stopped partway. The keys named were pressed; the one quoted was not. | The scanner is part way through whatever the run was doing. Check with [`menu`](menu.md) before pressing anything else. |

A run that fails partway is the failure worth understanding. The keys before
the failure have already been pressed and cannot be taken back, so the scanner
is left somewhere neither you nor the tool intended. The message names how far
it got for exactly that reason.
