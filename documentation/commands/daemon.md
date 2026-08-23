# daemon

Holds one scanner and runs other invocations' commands on it, so that a second
command waits its turn instead of being refused. Run it when you want more than
one thing driving the same radio.

## Overview

Only one thing can use a scanner at a time. The claim covers a whole command
rather than a single exchange, because the commands that walk menus press a key,
read the screen, and choose the next key from what they saw, and a second
process pressing keys in the middle of that leaves the scanner somewhere neither
of them meant. So while one command is using the scanner, the next one is
refused with `in use by another radiocli`.

`daemon` claims the scanner named with `--device`, keeps it, and answers
commands from other invocations over a socket. Commands still run one at a time,
because the scanner still answers one at a time. What changes is that being
second means waiting rather than failing.

**No `radiocli` command starts this for you.** Commands look for a daemon only
after being refused the scanner, and behave exactly as they always did when
there is none. Sharing is something you switch on, not something that happens to
a script that was relying on being refused. The command runs until interrupted.

Nothing in this tool starts one for you. A front end built on the daemon
protocol may start its own, using a daemon already running if there is one.

Its socket, and what a program other than this tool would send to it, are
specified in [the daemon protocol](../daemon_protocol.md).

## Usage

```
radiocli daemon [flags]
```

## Parameters

`daemon` has three flags of its own. The rest of its behaviour comes from the
[global flags](../global_flags.md).

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--exit-with-parent` | No | `false` | Stop once whoever started this has gone and nothing else is connected. |
| `--audio` | No | none | Sound input the scanner's audio arrives on, as `radiocli audio` names it. |
| `--audio-channel` | No | `auto` | Which side of the cable the scanner is on: `auto`, `left`, `right` or `mix`. |

### `--exit-with-parent`

For a daemon started by another program rather than by a person.

A daemon holds a scanner, so one whose owner has died and left it running is a
radio nobody can use until somebody notices and kills it by hand. That is what
happens when the parent is killed outright: it never reaches whatever tidying it
meant to do, and `kill -9` reaches nothing at all.

With this flag the daemon watches its own stdin. The parent holds the write end
of a pipe and never writes to it; holding it open is the whole message. When the
parent exits, crashes or is killed, the operating system closes every handle it
had and the daemon reads end of file. Nothing is polled and no process ID can be
reused underneath it.

**End of file is not a stop, it is a notice.** The daemon then keeps the scanner
until nothing is connected to it, and stops at that moment. Both conditions are
needed and neither is enough on its own:

- A daemon whose starter has gone may be the only way somebody else is reaching
  the radio. One page starts a daemon, a second page joins it, the first is
  closed. Stopping there would take the scanner from the second page, which
  never knew which process happened to spawn the thing it is talking to. The
  same goes for a command halfway through a menu walk in another terminal.
- A daemon with nothing connected is doing its job. One started in a terminal
  sits idle until a command arrives, and being idle is the whole point of it.

So the last thing to let go of a daemon is what ends it, whoever started it.

It is a flag rather than the default because a daemon started in a terminal must
not stop the moment its stdin happens to be a file that ends, or `/dev/null`.
You would pass it if you were writing something that starts a daemon of its own
and wants it to go when that program does.

### `--audio`

Names a sound input for the daemon to hold, alongside the scanner.

The scanner's audio does not travel over the USB cable this tool controls it
with. It leaves the scanner as an ordinary sound signal on a cable, so it reaches
this computer only through whatever sound input you ran that cable into, and no
command can work out which one that is. Run [`audio`](audio.md) to see the names.

**The daemon holds a sound input for the same reason it holds a serial port.** A
sound input can only be open once, so without something in the middle, two things
wanting the scanner's audio means one of them gets it and the other is refused.
With this flag the daemon reads the input once and hands a copy to everybody who
asks, exactly as it runs commands one at a time for everybody who asks.

Unlike the scanner, the input is **not opened when the daemon starts**. The name
is checked then, so a mistake is reported while you are still looking at the
terminal, but the device itself is opened when the first listener arrives and
given back half a minute after the last one leaves. Listing sound inputs asks
macOS for nothing; opening one raises the microphone permission prompt. A daemon
that opened its input on the way up would raise that prompt attached to nothing
you did, and would then hold the microphone open with nobody listening, which is
the sort of thing that makes people suspicious of software.

A name that matches nothing is reported and stepped over rather than being
fatal. Holding the radio is this command's job; audio is something extra it can
also carry, and an unplugged sound card is no reason to stop doing the first.

Two identical interfaces report the same name and cannot be told apart, so a name
matching both is refused rather than guessed at.

### `--audio-channel`

Which side of the cable the scanner's audio is on.

It has to be asked because the scanner is mono and the lead decides where that
mono signal lands. A stereo lead from the headphone socket carries it on both
sides. A mono lead, or a record lead wired for one channel, carries it on one and
leaves the other silent. Getting it wrong is not subtle: folding a one-sided
signal together with an empty one halves its level, and taking the empty side
gives silence.

`auto`, the default, listens for a few seconds and decides. Audio flows from the
first frame while it is deciding, folded as `mix`, because silence at the moment
somebody presses Listen is worse than a few seconds at half level. It only counts
frames loud enough to mean anything, since the channel is silent most of the time
and a silent frame says nothing about which side the signal is on. Once it has
decided it does not change its mind: a quiet transmission is not evidence that a
channel went away, and a fold that flipped mid-speech would be audible.

`left`, `right` and `mix` say outright and skip the listening. They are the
escape hatch for when `auto` is wrong, which beats having to explain how to start
a daemon by hand.

### Global flags that change this command

- `--device` names the scanner to hold, and is required. Get the value from the
  `port` column of [`devices`](devices.md).
- `--verbose` logs each connection and each command run, which is the way to see
  what the clients are doing.

`--wait` applies to claiming the scanner at startup, as it does everywhere.
`--output` and `--pace` do not change what `daemon` prints; each command that
arrives resolves its own, from its own command line.

## Examples

Holding a scanner so other terminals can use it:

```
$ radiocli daemon --device /dev/cu.usbmodem00000000000011
Serving /dev/cu.usbmodem00000000000011 on /var/folders/.../T/radiocli/sockets-501/b79c92102cfaed83.sock
Other radiocli commands for this scanner will queue here instead of being refused.
```

In another terminal, with the daemon running, an ordinary command works as it
always did, except that it waits rather than failing:

```
$ radiocli status --device /dev/cu.usbmodem00000000000011
Running through the radiocli daemon holding this scanner.
port:     /dev/cu.usbmodem00000000000011
model:    SDS150
firmware: Version 1.00.37
display:  color
mode:     Scan Mode
```

The note is on stderr, so a command whose output is being read is unaffected:

```
$ radiocli battery --device /dev/cu.usbmodem00000000000011 -o json 2>/dev/null | head -3
{
  "state": "charging",
  "charging": true,
```

Without a daemon running, nothing changes. Two commands at once still means one
of them is refused:

```
$ radiocli status --device /dev/cu.usbmodem00000000000011
error: /dev/cu.usbmodem00000000000011 is in use by another radiocli: pid 4823 running "scanning systems" for 12s. Wait for it to finish, or pass --wait to queue behind it
```

Holding a sound input as well, so that more than one thing can hear the scanner:

```
$ radiocli daemon --device /dev/cu.usbmodem00000000000011 --audio "Cubilux CB5 Line In"
Listening for the scanner's audio on "Cubilux CB5 Line In" when somebody asks for it.
Serving /dev/cu.usbmodem00000000000011 on /var/folders/.../T/radiocli/sockets-501/b79c92102cfaed83.sock
Other radiocli commands for this scanner will queue here instead of being refused.
```

Nothing has opened the input yet. It is opened by the first listener, and two of
them share it:

```
$ radiocli audio feed --device /dev/cu.usbmodem00000000000011 | ffplay -f s16le -ar 48000 -ac 1 -i -
```

```
$ radiocli audio feed --device /dev/cu.usbmodem00000000000011 --format opus > /tmp/scanner.opus
```

Commands keep working while that audio is playing, and the audio keeps playing
while they run. Neither waits for the other, because they are not using the same
thing:

```
$ radiocli screen --device /dev/cu.usbmodem00000000000011
```

Naming an input that is not attached is reported, and the daemon carries on
holding the radio:

```
$ radiocli daemon --device /dev/cu.usbmodem00000000000011 --audio "Cubilux"
No audio: no sound input by that name: "Cubilux"
Serving /dev/cu.usbmodem00000000000011 on /var/folders/.../T/radiocli/sockets-501/b79c92102cfaed83.sock
Other radiocli commands for this scanner will queue here instead of being refused.
```

## Output

Everything `daemon` writes goes to stderr, so it can be redirected away from a
terminal without losing anything a client produced. What a client's command
prints goes to that client, not here.

Two lines are written at startup: the port being held and the socket it is
listening on, then a line saying what that means. With `--verbose`, each
connection and each command is logged as it happens.

`daemon` has no `--output json` form. It is a server rather than a report.

## Errors

Every failure exits with status `1` and prints to stderr. Errors that any
command can produce are listed in [global flags](../global_flags.md).

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `error: no scanner named: name one with --device` | No port was passed. A daemon holds one specific scanner, so it cannot guess. | Pass `--device` with a port from [`devices`](devices.md). |
| `error: <port> is in use by another radiocli` | Something else already holds this scanner, possibly another daemon. | Stop the other one, or pass `--wait` to queue behind it. |
| `error: making <path>: <detail>` | The directory the socket goes in could not be created. | Check the temporary directory is writable. |
| `error: restricting <path>: <detail>` | The socket directory, or the socket itself, could not be locked down to this account. Something else on the machine owns that path. | Remove the offending directory under the temporary directory, or check nothing else is writing there. |
| `error: listening on <path>: <detail>` | The socket could not be created. | Check the temporary directory is writable. |

A client that is refused for reasons of its own is answered on its own
connection rather than here. `the scanner is busy: too many commands are already
waiting` means one client has queued more commands than it is allowed; the bound
is per client, so that a runaway one cannot fill the queue another is waiting in.

## See also

- [The daemon protocol](../daemon_protocol.md) — the socket and its messages.
- [`devices`](devices.md) — find the port to hold.
