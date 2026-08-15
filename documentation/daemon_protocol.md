# The daemon protocol

This is the specification for talking to [`daemon`](commands/daemon.md), the
command that holds one scanner and runs commands on it for anybody else.

It is written down separately from the code because it has more than one
implementation. The tool implements both ends of it, and a front end built on
it implements the client end without sharing any code with the tool. **This document is what
those implementations are written against.** Where an implementation and this
file disagree, this file is what needs fixing first, and then the code.

## Why there is a daemon at all

The scanner speaks one request and response at a time over a single serial
line, and nothing in a reply says which process asked. Two invocations talking
at once therefore read each other's answers. Worse, the commands that walk
menus press a key, read the screen, and choose the next key from what they saw,
so a second process pressing keys in the middle of one leaves the scanner
somewhere neither of them meant.

So a claim on the scanner covers a whole invocation rather than one exchange.
That claim is exclusive, and without a daemon the second caller is simply
refused. A daemon is the same rule with a queue in front of it: commands still
run one at a time, but being second means waiting rather than failing.

## Transport

A unix domain socket, one per serial port.

The path is `<temp>/radiocli/sockets-<uid>/<digest>.sock`, where `<temp>` is the
system temporary directory, `<uid>` is the numeric id of the account running the
daemon, and `<digest>` is the first eight bytes of the SHA-256 of the port name,
in lowercase hex. The digest is the same one that names that port's lock file
one directory up, because a client refused the lock has to be able to find the
socket belonging to the port it was refused.

Unlike the lock file, the socket name carries no readable form of the port. A
unix socket path has to fit in `sun_path`, which is 104 bytes on macOS, and the
readable form of a typical port already reaches 102.

The socket is created with mode `0600`, inside a directory created with mode
`0700`. It carries every command the tool can run, so it is restricted to the
account that started the daemon. The directory is why the account gets one of
its own: restricting the socket after binding it leaves it world-connectable for
as long as the two steps are apart, and a private directory around it means
there is no such moment. A lock file cannot live there, because being readable
by every account is the whole point of it.

Messages are JSON objects, one per line, separated by `\n`. A single message
may be at most 65536 bytes. A longer one ends the connection, because nothing
says where an oversized line stopped and neither end could tell where the next
message began. The daemon answers it before hanging up:

```json
{"type": "error", "error": "that request is longer than the 65536 bytes a request may be, so this connection is being closed: nothing after it could be read"}
```

Both directions use that framing, with one exception, and it is the only place
in this protocol where the two directions differ. A connection that has asked
for `audio` stops being newline JSON in the daemon-to-client direction and
carries binary frames instead, from the newline that ends the reply onwards. The
client-to-daemon direction is unchanged for the life of the connection. See
[`audio`](#audio).

A client therefore cannot read this protocol with anything that buffers ahead
past the end of a line. A reader that fills a buffer from the socket and hands
out lines from it will already be holding the first frames when the framing
changes, and will lose them without any sign that they arrived. The reader that
reads the lines has to be the reader that reads the frames.

## Lifecycle

The daemon claims the port first and creates the socket second. A socket that
existed before the scanner was held would be one clients could find and submit
to while the daemon was still discovering it could not have the radio.

A socket left behind by a daemon that was killed is removed and replaced at
startup. That is safe because the port lock is what actually decides whether
another daemon is running.

Connecting to a socket nobody is listening on is the same answer as there being
no socket: there is no daemon, and the client should behave as it would if
sharing were not available.

## Client messages

Every message has an `op`. An unrecognised `op` is answered with `error`.

`run`, `cancel`, `commands`, `lease` and `release` are all about the scanner, and
all take turns on it. [`audio`](#audio) is about the sound input instead, takes
no turn, and changes the connection it is sent on.

### `run`

```json
{"op": "run", "id": "7", "argv": ["battery", "-o", "json"]}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `id` | string | Ties this run to the messages it produces, and is what `cancel` names. The client chooses it and only has to keep it unique among its own runs in flight. |
| `argv` | array of string | The arguments, already split, without the `radiocli`. |
| `command` | string | A command line as it would be typed, for a client that has not split it. |
| `mode` | string | `queue`, `try` or `peek`. Empty means `queue`. |

Exactly one of `argv` and `command` is used, and `argv` wins if both are given.

**Splitting a typed line is the daemon's job.** A client that has a line rather
than an argument list sends `command` and lets the daemon split it. This is not
a convenience: the tool checks that a macro step can be split when the step is
saved, and a line refused at run time would fail in the middle of a macro with
the steps before it already done. One splitter, on this end, is what makes
those the same question.

`mode` decides what happens when the scanner is busy:

- `queue` waits its turn. This is what a command somebody asked for uses.
- `try` gives up at once and is answered with `skipped`. This is for a caller
  mirroring the scanner rather than commanding it: a reading is only worth
  taking now, and a queued one would describe a screen several frames old and
  would have taken the turn in front of whatever the person watching clicked
  next. A `try` is also refused while anybody is waiting, so a mirror reading
  ten times a second cannot starve the commands.
- `peek` does not wait and does not take a turn at all. It runs alongside
  whatever else is running. **Only commands that cannot move the scanner may be
  asked for this way**, and anything else is refused with `done` carrying an
  error, not queued. See [What a peek may run](#what-a-peek-may-run).

**There is no way to send settings alongside a run, and this is deliberate.** A
run executes as a full invocation inside the daemon, so it resolves its own
settings the way every invocation does: the config file first, then the flags on
its own command line. Anything applied to the configuration beforehand is
overwritten by that load before the command sees it. A client that wants a
different pace or output puts the flag on the command line, which is the one
mechanism that wins.

### `cancel`

```json
{"op": "cancel", "id": "7"}
```

Stops a run already in flight. The command is interrupted where it stands,
which may leave the scanner mid-walk; that is the caller's Ctrl-C, and the
alternative is a radio nobody is watching being driven to the end of something
nobody wants.

Cancelling an `id` that is not running does nothing and is not an error.

### `commands`

```json
{"op": "commands", "id": "1"}
```

Answered with `commands`, describing every command the daemon can run. It needs
no scanner and takes no turn, because it reads the command tree rather than the
radio. A client that presents the tool rather than running it uses this instead
of keeping a list of its own, so that a command added to the tool appears
without anybody remembering to describe it twice.

### `lease` and `release`

```json
{"op": "lease", "id": "2", "ttl": "30s"}
{"op": "release", "id": "3"}
```

A lease claims the scanner across several runs. While it is held, every other
connection's `queue` runs wait, and `try` runs are skipped.

A single command is indivisible without a lease, which is enough for the
commands that walk from the top menu every time and leave the radio where they
found it. It is not enough for pressing a key, opening a menu, tuning or
resuming a scan, which deliberately leave the radio somewhere. A caller running
several of those in a row means each to happen where the last one left off. **A
macro is exactly that, and must run inside a lease.**

`ttl` is required and is a Go duration string such as `"30s"`. It must be
greater than zero and at most five minutes. A lease stops every other caller, so
the ceiling is what stops one client's mistake from being everybody's problem.

`leased` arrives when the lease is actually held, which for a client queued
behind another one is later than when it asked.

A lease is released by `release`, by its `ttl` expiring, or by the connection
closing. The last of those is what stops a page that crashed mid-macro from
holding the radio. A connection that closes while its `lease` is still queued
gives up its place rather than being granted the scanner it can no longer use.

Waiting for a lease does not stop the connection answering anything else. A
client queued behind somebody else's macro can still send `cancel`, `commands`
and further runs, and they are answered while it waits. `lease` and `release`
are the exception only in relation to each other: they are answered in the
order they were sent, so a `release` cannot overtake the `lease` it gives back.

However it ends, a lease with one of the holder's commands still running is not
handed on until that command finishes. The scanner answers on one line with
nothing saying who asked, so handing it to the next caller mid-command would
interleave two conversations. A `release` sent while a command runs is
therefore answered with `released` only once the command is done.

The leaseholder's own runs are let straight through rather than queued, because
it already holds the scanner. They still run one at a time.

### `audio`

```json
{"op": "audio", "id": "4", "format": "opus", "bitrate": 32000}
```

Subscribes to the sound input the daemon is holding, and turns this connection
into a one-way stream of audio.

**Audio takes no turn.** It does not queue, it is not refused while a command is
running, it does not hold anything up, and it never touches the serial port.
Every other operation in this protocol is about taking turns on one radio that
answers one thing at a time; this one is not, because the audio does not come
from the radio. It arrives on a separate cable into a separate sound input. The
daemon holds that input for the same reason it holds the serial port, which is
that one program holding it can serve everybody who wants it, and a sound input
can only be open once.

`format` is `pcm` or `opus`, and defaults to `pcm`. `bitrate` is bits per second
and is only consulted for `opus`, defaulting to 32000.

A daemon started without a sound input answers `error`, and the connection is
left as it was.

Otherwise the reply is the last line this connection carries:

```json
{"type": "audio", "id": "4", "format": "opus", "rate": 48000, "channels": 1,
 "frameMs": 20, "bitrate": 32000, "source": "Cubilux CB5 Line In", "channel": "left"}
```

`rate`, `channels` and `frameMs` are fixed by the encoder and sent anyway, so a
client needs no constants of its own to keep in step with this end. `channel` is
which side of the cable the scanner was found on, as it stood when the stream
started; left to work it out the daemon spends the first few seconds deciding,
so a stream that began in that window reads `mix` here and is told the answer
later in a frame of its own.

**Everything after the newline that ends that reply is frames.**

```
byte  0     kind
bytes 1..3  payload length, big endian, not counting these four bytes
bytes 4..   payload
```

| Kind | Payload |
| ---- | ------- |
| `1` | Audio: four bytes of frame number, big endian, then the encoded frame. |
| `2` | One JSON object, with no newline after it. |

A payload may be at most 1048576 bytes. Nothing real approaches that: an Opus
packet is at most 1279 bytes and a raw frame is 1924. It is a guard against a
length read out of a corrupt stream turning into an allocation of whatever it
says, and a client should refuse rather than allocate.

A kind it does not recognise should be skipped rather than refused. The length
in the header is what makes that possible, and it is what lets a later daemon
add something without breaking an older client.

**The frame number is a clock, not a count of what was sent.** It counts 20 ms
frames since the capture started, so a client works timestamps out from it as
`number × 20 ms`. Everything that can lose audio between the sound card and
somebody's speakers leaves a gap in it: a listener too slow to keep up, a front
end dropping frames to catch up, and one day a gate deciding there was
nothing worth sending. A client that instead counted the frames it received
would play steadily earlier with every one it missed, and nothing would tell it
so.

A client may change the bitrate at any time, on the same connection, still as
newline JSON:

```json
{"op": "audio", "id": "4", "action": "bitrate", "bitrate": 24000}
```

A rate that took effect is answered with nothing at all: the packets that follow
are smaller, which is the answer, and no packet says the rate changed because a
decoder reads it out of each one. A rate that was refused comes back as a kind 2
frame. Changing the **format** is not possible; that is a different stream, and
it is done by opening another connection.

`action` is only for a connection that is already carrying audio. Sent on a
fresh one it is refused as an ordinary line, rather than being read as a request
to subscribe:

```json
{"type": "error", "id": "4", "error": "\"bitrate\" is for a connection that is already carrying audio: ask for audio first, with no action"}
```

There is no operation to stop. Closing the connection is how a stream ends, the
same way it is how a lease ends.

**A connection that has asked for audio may ask for nothing else.** `run`,
`lease` and the rest are refused on it. Both ends have stopped speaking newline
JSON in the daemon-to-client direction, so there is nowhere for the answer to
anything else to go. A client wanting both opens a second connection, which
every real one already does for reasons of its own.

The daemon opens the sound input when the first listener asks and gives it back
a while after the last one leaves. Listing sound inputs asks macOS for nothing,
but opening one raises the microphone permission prompt, so a daemon that opened
its input on the way up would raise that prompt attached to no action anybody
took and then hold the microphone with nothing listening. The name is still
checked at startup, so naming one that is not there fails immediately.

## Daemon messages

Every message has a `type`. Messages belonging to a run carry its `id`.

| Type | Fields | When |
| ---- | ------ | ---- |
| `hello` | `version`, `protocol`, `audio` | Once, when a client connects, before anything else. |
| `started` | `id`, `argv` | A run reached the front of the queue and is now executing. `argv` is how the daemon split `command`. |
| `stdout` | `id`, `data` | The command wrote to stdout, as it wrote it. |
| `stderr` | `id`, `data` | The command wrote to stderr. |
| `done` | `id`, `error`, `code` | A run ended, whether it worked or not. |
| `skipped` | `id` | A `try` run never happened because the scanner was busy. Not a failure. |
| `commands` | `id`, `commands` | Answers `commands`. |
| `leased` | `id` | The lease is now held. |
| `released` | `id` | The lease was given back. |
| `error` | `id`, `error` | A message the daemon could not make sense of, such as an unknown op. |

`stdout` and `stderr` are kept apart so that a caller piping JSON somewhere
still can. The tool's own rule holds through the socket: the result goes to
stdout, and progress and advice go to stderr.

On `done`, `code` is the exit status the same command would have produced in a
terminal, so a proxied invocation can exit with it. `error` is why it failed,
written the way the terminal would have written it, without the `error: `
prefix. A run that was cancelled ends with a non-zero `code` and no `error`,
because being interrupted is the caller's own doing rather than something to
report.

A failure of something the daemon understood is reported by that thing, so a
command that failed is a `done` with an `error` rather than an `error` message.

## What a peek may run

A turn covers a whole command. A connection covers one exchange. That gap is
what `peek` lives in.

A menu walk needs the turn: it presses a key, reads what the screen became, and
presses the next one, and a key pressed into somebody else's screen goes
somewhere nobody meant. A read needs neither. It changes nothing the walk
depends on, so it can be let between two of the walk's exchanges and the walk
cannot tell. The connection keeps the wire coherent either way, because it
carries one request and its response at a time.

So a `peek` is allowed exactly for the commands that press nothing and open
nothing, on every path through them and with every flag:

| Command | Why it qualifies |
| ------- | ---------------- |
| `screen` | Reads the display. |
| `display` | Reads which color mode the scanner is in. Its `mode` subcommand sets one and does not qualify. |
| `menu` | Reads the menu on screen. Its `open`, `set`, `back` and `close` subcommands move around and do not qualify. |
| `status` | Reads what the scanner is doing. |
| `battery` | Reads the charge. |
| `volume` | Reads the level. Its `set` subcommand does not qualify. |
| `squelch` | Reads the level. Its `set` subcommand does not qualify. |
| `version` | Touches no scanner at all. |
| `colors` | Only with `--cache` or `--positions`. See below. |

`colors` is the one whose answer depends on its flags. The bare command reads a
layout's colors by walking every one of its color pickers in turn, which moves
the scanner a great deal; `--cache` hands back what that walk found last time,
and `--positions` answers from a built-in table. Both ask no more of the radio
than which layout it is drawing.

Marking the command either way as a whole is wrong, and both mistakes have been
made. Marking it safe would let the walk run alongside somebody else's. Leaving
it unsafe freezes the colors for the length of every command, and since the
screen underneath them keeps being redrawn, what a page shows is a new screen in
an old screen's colors: switch to the weather screen during a macro and it is
painted in the scanning screen's colors until the macro ends.

The list is not kept in the daemon. Each command says for itself whether it only
reads, and the few whose answer turns on a flag name that flag, so a command
added later is refused until somebody says otherwise rather than being assumed
safe.

A peek runs on a second connection to the scanner's own settings: it cannot
change the pace, the output format, or anything else the command it is running
alongside resolved for itself. Peeks are serialized against each other, so two
clients peeking wait for one another, which is a wait measured in milliseconds
rather than in menu walks.

Measured on an SDS150, a menu walk with reads slipped in as fast as the wire
allows produced the same values as the same walk alone and took eight percent
longer.

## Versions

`hello` carries `protocol`, an integer, currently **3**. A client that does not
understand it should say so and stop rather than guess.

Protocol 2 added the `peek` mode. A client that speaks 1 and finds a daemon
speaking 2 is refusing a daemon it could have talked to, which is the cost of
the check being an equality rather than a minimum, and is the safe direction to
be wrong in.

Protocol 3 added the `audio` op, the `audio` field on `hello`, and with them the
second framing described under [Transport](#transport). That last part is why it
is a version bump rather than an addition older clients could ignore: a client
written against protocol 2 has no reason to expect anything on this socket that
is not a line of JSON.

`version` is the daemon's build, which is informational. Two builds with
different version strings speak the same protocol far more often than not, so a
client that refused on that would refuse perfectly good daemons.

Any change to the meaning of a message, or to when one is sent, is a change to
`protocol`. Adding a field that older clients can ignore is not.

## What a client is expected to do

A client that is a command being proxied should:

1. Try the port lock first and run locally if it gets it. The daemon is for
   when the port is busy, not something to prefer.
2. On being refused, connect to the socket for that port. Failing to connect
   means there is no sharing available, and it should report the busy scanner
   exactly as it would have.
3. Send the arguments exactly as they were typed, so that a proxied command does
   precisely what the same command does in a terminal.
4. Forward its own interruption as `cancel`, so the command stops there rather
   than the radio being driven for somebody who has gone.
5. Exit with the `code` from `done`.

Re-running a command that was refused the lock is safe only because it stopped
at the point of opening the port, before touching the radio or writing anything.
A client must not retry a command that has already produced output.
