# Recording the scanner's audio

What comes out of the SDS150's headphone jack, what I had to do to it before it
was a recording worth keeping, and every part of that which turned out to be
harder than it looked.

Measured on an SDS150, firmware 1.00.37, through a Cubilux CB5 USB line input on
macOS 15, 2026-08-22 and 2026-08-23. The absolute levels belong to that cable,
that sound card and that volume setting. The shapes of the problems do not, and
that is the part worth keeping.

I got two of these wrong before I got them right, and I have left the wrong
answers in. Both times the wrong answer was the plausible one, which means the
next person is going to reach for it too.

[protocol.md](protocol.md) covers the serial side, including the fields this
leans on. [oddities.md](oddities.md) carries the headphone jack as an entry of
its own.

---

## The signal chain

Audio does not come over the USB serial port. The scanner has no way to send it
there. It leaves through the headphone or record socket as analog line level and
gets to the computer through whatever sound card the cable is plugged into,
which means the recording and the labelling arrive by two separate paths that
share no clock. Most of this file is the consequence of that one sentence.

| Stage | Format |
| ----- | ------ |
| Out of the scanner | Analog line level, two conductors |
| Into the sound card | 48 kHz, signed 16-bit little-endian, stereo |
| After the fold | 48 kHz, s16le, mono, cut into 20 ms frames |
| Written to disk | RIFF/WAVE, PCM, 1 channel, 48 kHz, 16-bit |

48 kHz is not a choice. It is what the capture layer wants and what a line input
hands over without resampling. 20 ms is the frame length everything downstream
is built in: short enough that a boundary landing inside one is inaudible, long
enough that an RMS over it means something.

The WAV header is 44 bytes and both of its length fields are unknown while the
file is still being written, so I write zeroes and seek back on close. The
`riff` size sits at byte 4 and `data` at byte 40. `afinfo` reads a one second
file back as `1 ch, 48000 Hz, Int16, estimated duration: 1.000000 sec, audio
data file offset: 44`, which is the whole verification.

## The two sources never line up

The scanner is polled. The audio is streamed. So the news that a transmission is
happening always arrives some unknown amount after the sound of it, and the
amount moves around with the poll interval, the reply time, and how quickly the
radio noticed.

Start writing when the radio says so and you lose the first syllable every time.
The usual fix is to prepend a fixed amount of buffered audio, half a second or
five, and let the user pick. That is wrong in both directions at once. Set it
short and the opening is still clipped when the news was slow. Set it long and
every file carries however much dead air the guess overshot by. A constant
cannot stand in for something that varies, and no amount of tuning fixes that.

So I stopped racing. Hold the last ten seconds of audio and decide nothing in
real time. When a transmission gets noticed, however late, the audio from before
it is still sitting in the buffer, so I can **find** the beginning instead of
guessing at it: walk backwards to where the energy came up out of the noise
floor and start there. Same walk forwards trims the tail.

That divides the work along the line each source is actually good for. The radio
says **that** a transmission happened and **what** it was. The audio says
**exactly when** it started and stopped. Once it is split that way the timing of
the radio's answer stops mattering, because a sample only has to land somewhere
inside the transmission to identify it.

Ten seconds is about an order of magnitude over the worst of the three lags that
feed into this, and it costs 960 KB. Cheap.

## The noise floor has to be measured, and not by its minimum

There is no threshold setting anywhere in this, on purpose. The level that
counts as signal is a fixed margin over a measured noise floor, because the
floor belongs to somebody's cable, dongle and volume knob rather than to audio
in general. Asking a user to configure it is asking them to measure something
they have no instrument for.

My first cut took the floor as the quietest frame in the last fifteen seconds.
That wrote me sixteen seconds of hiss, and the numbers say why:

| Reading | Value |
| ------- | ----- |
| Where the noise actually sat | -78 dBFS |
| Spread of the noise | about 3 dB |
| Quietest single frame in 15 s | -87 dBFS |
| Trigger, at floor + 8 dB margin | -79 dBFS |

The trigger came out a decibel **under** the noise, so the noise read as signal
and the recorder wrote the gaps between transmissions like they were traffic. I
estimated a distribution by its most extreme sample, which is the kind of thing
that works fine until the distribution grows a tail.

A low percentile fixes it. The floor is the 10th percentile of the last fifteen
seconds now, counted in 1 dB buckets between -90 and -30. Low enough to sit in
the quiet part of a window that also holds a transmission, high enough to say
where the noise really is rather than how far it dipped once. That is also what
lets me measure it on every frame, transmissions included: speech is loud, and a
low percentile ignores loud.

The bounds carry as much weight as the estimator. A floor pinned at -30 is a
microphone in a room, not a squelched scanner. One at -90 is digital zero, which
on macOS is almost always the microphone permission having been refused, and
chasing it down would put the trigger under the noise the card makes when it
comes back.

## A scanner has more than one idle level

Fixing the estimator was not enough, and what beat it is a real property of the
radio rather than bad arithmetic on my end. The SDS150's line output does not
have an idle level. It has two:

| State | Level |
| ----- | ----- |
| Squelch shut | -88 dBFS |
| Idle at other times | -77 dBFS |

Any fixed margin above the quieter one reads the louder one as speech. Eight
decibels over -88 is -80, and the radio sits at -77 for long stretches while
receiving nothing at all.

No threshold separates those, because they are both silence. So the audio does
not get to decide. The radio says plainly whether its audio gate is open and
that settles whether a recording exists; the audio is left doing the one thing
it is genuinely better at, which is finding the exact moment inside that window
when the sound started and stopped.

The failure that buys me is worth naming: a transmission that really lasted
under a second, followed by sixteen seconds of the radio's own idle noise,
written as one recording, with nothing in the file to suggest anything had gone
sideways.

## Folding the two sides together destroys the audio

The headphone jack is wired out of phase. Fold its two sides to mono and most of
the signal cancels. Measured against continuous NOAA weather voice through the
same cable and card:

| Fold | RMS | Peak |
| ---- | --- | ---- |
| Left only | -25.2 dBFS | -9.5 dBFS |
| Right only | -24.0 dBFS | -8.2 dBFS |
| Both, averaged | **-35.0 dBFS** | **-19.5 dBFS** |

Two sides carrying the same mono audio fold with no loss at all, so eleven
decibels means they are fighting each other.

Uniden fixed it in firmware instead of hardware, adding
**Menu > Settings > Headphone L/R output** with the values `In Phase` and
`Invert Phase`. The menu is on the 1.00.37 unit in front of me; which release
first carried it I have secondhand and have not verified. So whether a given
radio has the problem comes down to which way one menu entry got left, and
anything reading this output has to survive both. See [oddities.md](oddities.md)
for the entry, and the `headphone` command for reading and setting it.

**Nothing about the levels gives it away.** Both sides are equally loud. Pick
how to fold by comparing the two sides and you see a perfectly healthy stereo
lead, then choose the one option that destroys the signal. The fold has to be
judged by what comes out of it, not by what goes in: measure what mixing would
give, and refuse to mix when it lands more than 4 dB under the better side
alone. Four decibels is over anything an unbalanced pair produces and well under
the eleven I measured here.

One bug in that logic is worth writing down, because it was invisible and it
guaranteed the worst outcome. The chooser used to give up after thirty seconds
without a clear answer and settle on mixing. On a quiet channel it never heard
enough audio to decide anything, so it timed out like clockwork and locked
itself onto the setting that cancels. A fallback that fires exactly when there
is no evidence, and falls back to the destructive option, is worse than having
no fallback. I pulled it out rather than retuning it.

### Mono compatibility was a solved problem in 1961

This is why I call it a defect and not a quirk. FM stereo was designed from the
start with the main channel carrying L+R and the subcarrier carrying L-R,
specifically so a mono receiver summing them gets full audio. Sixty-five years
of consumer gear has been expected to survive being folded down. This jack does
not, which is why it got a firmware fix rather than a line in the manual.

## What sounded like a codec artefact was arithmetic

The symptom was a thin, reedy buzz in the speaker's voice, there only while
somebody was talking. Sounded like a guy talking through a kazoo.

I called it the P25 vocoder, which band-limits to about 3.4 kHz, and I wrote it
up as normal and not worth fixing. Wrong, and wrong in a useful way: the channel
was analog NOAA weather. There was no vocoder anywhere in the path. The
explanation was plausible, it fit the symptom, and I never checked it against
what the source actually was.

What the fold does to the spectrum is the real answer, same source:

| Band | One side | Folded |
| ---- | -------- | ------ |
| 300-1000 Hz | 67% | 13% |
| 1-2 kHz | 22% | 43% |
| 2-3.4 kHz | 11% | 43% |

Cancellation is not uniform across frequency. The low end is where the two sides
are most alike, so it cancels hardest, and what survives is the high-frequency
residue where they differ. That is a voice with the body taken out of it. That
is a kazoo.

The lesson is not about phase. It is that "this is a known artefact of X" needs
X to actually be present, and checking would have cost me one look at the
channel.

## Low volume was the same fault

The recordings sounded quiet. I raised that as a separate complaint and it
turned out to be the same eleven decibels: the fold was throwing away most of
the signal, and turning anything up would only have lifted the residue and the
noise together. Two symptoms, one cause. The level one was the better clue,
because you can measure a level and you can only describe a timbre.

## Where one transmission ends and the next begins

Recordings had two people in them. Dispatcher, a pause, then the unit answering,
all in one file. Five of nine recordings from one night had internal gaps
between 0.9 and 2.8 seconds.

The obvious fix is to cut on silence in the audio, and it is wrong, because a
pause inside one person's transmission and the gap between two people are the
same thing in a waveform. Somebody reading a plate back says "three four" and
then sits there two seconds before "Adam Boy Charlie". Cut there and you have
invented a boundary that never happened, and nothing in the file would admit the
split was inferred instead of observed.

**There is no keyup event in the protocol.** Nothing is an edge or a
notification. `GSI` is a poll and `PSI` is the same document pushed on a timer,
not on a change. What there is instead is `Mute`, and it is enough, because it
follows the carrier and not the speech. It does not close for a breath, since
the guy is still keyed up. It closes when he lets go.

Polled flat out through a real exchange, printing only the documents that
differed from the one before:

```
 59.907  mute=Mute    rssi=-999  sig=0  ch=Fire/EMS Operations - Analog
 60.112  mute=Mute    rssi=-999  sig=0  ch=Police Operations
 60.390  mute=Unmute  rssi=-86   sig=5  ch=Police Operations   <- dispatcher keys up
 61.921  mute=Mute    rssi=-999  sig=5  ch=Police Operations   <- and unkeys
 62.093  mute=Mute    rssi=-999  sig=0  ch=Police Operations
 62.557  mute=Unmute  rssi=-87   sig=5  ch=Police Operations   <- the unit answers
 63.708  mute=Mute    rssi=-999  sig=5  ch=Police Operations
 63.865  mute=Mute    rssi=-999  sig=0  ch=Police Operations
```

**The gap is 640 milliseconds.** That one number sets nearly everything else.
Poll three times a second and you get two polls inside it, which you cannot tell
from two questions landing either side of nothing, so the poll rate decides
whether the two speakers can be separated at all. The radio held 81 `GSI`
documents per second on this cable, so asking ten times a second is an eighth of
what it will do and costs nothing worth counting.

**`Rssi` adds nothing over `Mute`.** They flip in the same document, both
directions, every time. I went after `Rssi` on the theory that it reports the
receiver while `Mute` reports the audio path, so the scanner's channel delay
would hold the mute open across a gap the RSSI could see through. Theory was
wrong: the delay holds the *channel*, not the mute. `Mute` was the right field
the whole time and I had already been using it.

**`Sig` is late at both ends.** I knew it lagged on the way up, with `Mute`
opening a full poll before any bars show. It lags on the way down too, sitting
at 5 for about 170 ms after the carrier is gone and `Rssi` has already dropped
to -999. Not a receive indicator in either direction.

**`A_Led` is not one either.** The spec presents it as `Blue` while receiving and
`Off` otherwise, and one captured document backs that up. Through both
transmissions above it read `Off` on every single document, including the ones
with the mute open. Best guess is it follows the channel's own alert colour.
Either way I am not building on it.

**Nothing identifies the speaker on analog.** An analog conventional channel
carries no ID at all. No scanner and no software can tell you which radio keyed
up, because the transmitter never sends anything that says. P25 is different:
`U_Id` carries the radio ID and the scanner decodes it. So cutting on the keyup
gets one transmission per file everywhere, and one *name* per file only where
the system bothers to transmit one.

## Audio silence really is not a transmission boundary

This is the measurement that settles the argument above instead of just making
it. A recording from after the keyup split went in:

```
Pocahontas County / Municipalities - Marlinton, Police Operations, NFM
3.22 seconds, one file, with 0.96 seconds of silence from 0.40 s to 1.36 s
```

Almost a full second of nothing, right in the middle, and correctly **not**
split, because the carrier never dropped. One guy, one keyup, one pause. A
silence rule turns that into two files of the same speaker, which is a worse
error than the one it was brought in to fix, and a quiet one.

## The numbers, and what sets each

None of these is taste. Each one is pinned by a measurement or by another number
in the table.

| Constant | Value | What sets it |
| -------- | ----- | ------------ |
| Frame length | 20 ms | The capture format; short enough that a boundary inside one is inaudible |
| Buffer window | 10 s | An order of magnitude over the worst combined lag; about 960 KB |
| Floor window | 15 s | Long enough that a transmission cannot fill it, short enough that a real change to the input is believed quickly |
| Floor percentile | 10th | Below the transmissions in the window, above the dips; the minimum was tried and put the trigger under the noise |
| Floor bounds | -90 to -30 dBFS | Below is digital zero, above is a microphone in a room |
| Margin above floor | 8 dB | Above a floor wandering by a decibel, far below the 20+ dB that separates speech from noise |
| Poll interval | 100 ms | Six polls inside the 640 ms gap measured between two speakers |
| Hang, with a radio | 500 ms | Under the 640 ms gap, over any carrier flicker worth bridging |
| Hang, without one | 2 s | Long enough to carry a breath, because there a pause is all there is to go on |
| Radio slack | 200 ms | Two polls; and it has to stay well under the minimum length, see below |
| Minimum length | 500 ms | Short enough to keep "ten four", which is its own file now |
| Onset pad | 200 ms | One frame of detector error, plus the fact that a syllable rises rather than appears |
| Look-back limit | 2 s | Generous for the trigger lag; unbounded lets a low floor drag the whole buffer into the file |
| Cancellation threshold | 4 dB | Above an unbalanced pair, far below the 11 dB an inverted one produces |

The interaction between radio slack and minimum length is the one that is not
obvious, and it bit me. Slack is how long audio arriving after the radio stopped
still counts toward the transmission, so on a radio whose idle noise sits above
the floor, it is that much noise stapled onto the end of everything. At 400 ms
of slack against a 500 ms minimum, a 200 ms blip reached the minimum on noise
alone and got kept: the record-the-hiss failure coming back by a different door,
a day after I fixed the original, and caught only because a test still covered
it. Change one of those two numbers and you have to check it against the other.

## Things that cost me time and are not obvious

**Frame timestamps are arrival times, not capture times.** Every frame drained
in one wake of the cutter carries the same `time.Now()`. So audio timestamps run
late against serial poll timestamps by the card's buffer plus the callback
batch, and anything comparing the two has to allow for it. The onset search does
not care, because it works on energy and a constant offset cancels out. The
slack at the end does care.

**Frame numbering is unsigned, so check for a gap before you subtract.** The
count restarts when the sound card is reopened, the sequence number steps
backwards, and subtracting wraps to something near four billion instead of going
negative. It shows up silently as a plausible-looking count of dropped frames.
Guard the subtraction rather than letting it produce a number at all.

**Length has to be measured on the audio, not on how long the gate was open.**
Those differ by exactly the hang time, so a 400 ms blip cleared a one second
minimum purely because the gate spent a second afterward making sure it had
ended. The measurement that means anything runs from the first frame above the
floor to the last one.

**A trigger frame can land in the file twice** if it gets appended to the ring
and then handed to the code that appends it to the transmission. Worth saying
out loud because you will never hear it: 20 ms of duplicated audio does not
sound like anything.

## Still unverified

- All of this is one SDS150 on firmware 1.00.37, one cable, one sound card.
  Nothing has been checked against a second radio.
- The unit ID on a digital trunked call has still never been seen populated.
  The talkgroup half got its confirmation on the first night of trunked
  traffic, and the confirmation cut both ways: the element arrived spelled as
  modelled, carrying the same "TGID:" prefix as the conventional one, and the
  code that should have stripped it did not, so the night's recordings were
  labelled "TGID:10003". The stripping was tested on its own and passed; no
  test fed a whole trunked document through and read what came out the far
  end, which is the only place that wiring can fail. Fixed, with that exact
  test added.
- Whether `A_Led` follows the channel's alert colour is inference from it reading
  `Off` the whole time. I have not set an alert colour and watched it change.
- The 640 ms gap between two speakers is a single measurement. I am using it
  like a lower bound and it is really just the shortest one I have seen. Eight
  more minutes of polling that night caught no traffic at all to add to it.
