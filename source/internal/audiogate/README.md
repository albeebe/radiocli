# audiogate

## What this does?
Finds the transmissions in a stream of scanner audio. It takes every frame the sound card produces, plus whatever the radio says it is receiving, and hands back the transmissions: where each one started, the audio it is made of, and where it ended.

## Why we use it?
Recording a scanner means reconciling two sources that nothing synchronises. The audio arrives on a sound card. What is being received arrives on the serial cable, polled, and therefore always some unknown amount late. Software that starts a recording the moment the radio says so records a transmission whose first syllable has already gone.

The usual answer is to prepend a fixed amount of buffered audio, half a second or five, chosen by the user. That answer is wrong in both directions at once. Set it short and the opening is still clipped when the news was slow. Set it long and every recording carries however much dead air the guess overshot by. It cannot be right, because it is a constant standing in for something that varies.

This package does not race, and does not guess. It holds the last ten seconds of audio and decides nothing in real time. When a transmission is noticed, however late, the audio from before it is still in the buffer, so the beginning can be found by looking for it: walk backwards to where the energy rose out of the noise floor, and start there. The same walk forwards trims the tail. A late trigger cannot clip the opening and a generous one cannot pad the file, because neither the trigger nor a constant decides where the file begins.

That splits the work along the line each source is actually good for. The radio says **that** a transmission happened and **what** it was. The audio says **exactly when** it started and stopped. Neither is asked to do the other's job, which is what makes the timing of the radio's answer stop mattering: a sample only has to land somewhere inside the transmission to identify it, and the file boundaries never depend on when it arrived.

The one thing the audio genuinely cannot do is tell two transmissions apart when they follow each other closely. To a sound card, a dispatcher and the unit answering are one event with a pause in it, and so is one person pausing mid-sentence. Nothing in the waveform separates those two cases, and a detector that cuts on silence gets both wrong in opposite directions.

The radio knows better, and the field that knows is `Mute`. It follows the carrier rather than the speech, so it stays open through somebody drawing breath and shuts the moment they unkey. A mute that closes and reopens is two transmissions, and that is the boundary the recording is cut on. A change of channel cuts it too. On both questions the audio is overruled, because on both it is not equipped to answer.

The catch is that the gap can be short. Measured off an SDS150, a dispatcher and a cruiser left 640 milliseconds of closed mute between them, so the rate the radio is asked at decides whether the two can be separated at all, and the hang has to be shorter than the gap rather than long enough to carry a breath.

One margin above that floor turned out not to be enough. Finding where audio began means catching it while it is still faint, so the trigger sits close to the floor, eight decibels up. That same closeness is useless for the two questions asked at the other end of a transmission. On a clean line input the floor sits near -80 dBFS, so eight decibels above it is -72, and hiss at -70 clears it: it holds the last-heard time advancing for as long as the radio stays keyed, and one recording ran seven seconds for one second of speech. The same hiss makes a transmission the radio reported into a quiet cable look like a recording rather than a file of noise. So where a recording ends is measured against that transmission's own loudest moment instead of against the floor, which needs no estimate to be right, and whether it is worth a file at all is measured against the floor by a much wider margin. Across eighteen consecutive recordings on one radio, everything carrying speech peaked between 48 and 71 decibels above its floor and the one holding only noise peaked at 25.

A floor that is not quiet is not allowed to reject anything. The estimate means "where the noise is" only when most of the window it came from was noise, and a busy channel or a recorder started mid-transmission fills that window with speech instead. Reading a high estimate as a high noise floor would throw away the traffic that raised it, so above a certain reading the test simply does not apply.

Trimming the end also has to be paid for up front. Audio is handed out as it ages, and a frame handed over has been written, so quiet that might turn out to be the tail is held back until either more speech arrives and releases it or the transmission ends and it is dropped. Without that the tail is already on disk by the time anything knows it was a tail.

There is also no threshold setting here, deliberately. The level that counts as signal is a fixed margin above a continuously measured noise floor, and the floor is measured because it is a property of somebody's cable, dongle and volume knob rather than of audio in general. Asking a user for it is asking them to measure something they have no instrument for, and getting it wrong produces either recordings full of nothing or no recordings at all. The floor is a low percentile of what was heard in the last fifteen seconds, which is what lets it be measured straight through a transmission: speech is loud, and a low percentile ignores loud. Not the minimum, which was tried first and put the trigger nine decibels under the noise on a real line input, so the recorder wrote the hiss between transmissions as though it were traffic.

Keeping all of this in a package that touches no files, no sound card, no radio and no clock is what makes it testable. Time comes from the frames themselves, so the tests drive an afternoon of scanner traffic through it in a millisecond, including the cases that cannot be staged against real hardware: a trigger arriving three seconds late, a radio reporting activity into a cable that was never plugged in, a noise floor that climbs, and two transmissions inside one buffer window.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/audiogate"

// Every field may be left zero for a sensible default.
gate := audiogate.New(audiogate.Options{
    RequireRadio: true,
    Hang:         500 * time.Millisecond,
    MinDuration:  500 * time.Millisecond,
    MaxDuration:  5 * time.Minute,
})

// Whenever the radio is asked what it is doing, tell the gate. The key is
// opaque here: build it from whatever distinguishes one channel from another.
gate.Activity(sampledAt, audiogate.Activity{On: true, Key: "Pocahontas/Fire/Marlinton Dispatch"})

// Then hand it every frame, and act on what comes back.
for frame := range sub.Frames() {
    for _, ev := range gate.Offer(frame) {
        switch ev.Kind {
        case audiogate.KindStart:
            // A transmission worth keeping. It has already outlived
            // MinDuration, so a file opened here never has to be deleted.
            open(ev.Tx.Start)
        case audiogate.KindAudio:
            write(ev.Frame.PCM)
        case audiogate.KindEnd:
            // Tx carries the real span, why it ended, and how many frames
            // were lost on the way here.
            done(ev.Tx)
        }
    }
}

// On the way out, so a transmission in progress is still kept.
for _, ev := range gate.Flush() {
    // ...
}
```

Something playing the audio as it arrives asks a different question, and asking
it with the events above produces holes. Both of them are deliberately late: a
`KindStart` waits until the transmission has earned a file, and the frames wait
until they have aged past the hang so the end can be trimmed. For a speaker,
late is missing.

`Live` is that other question. It is true from the first frame above the noise
floor, and it carries the same hang a transmission does, so a pause between two
words does not shut the speakers in the window before the radio has confirmed
anything.

```go
// The same gate, asked what a listener needs to know rather than what a
// recorder needs to know.
for frame := range sub.Frames() {
    gate.Offer(frame)

    if gate.Live(frame) {
        play(frame.PCM)
    }
}
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Sliding window minimum** - The monotonic queue behind the noise floor, and why a minimum rather than an average is what stops a transmission raising the level needed to trigger it
- **Squelch and hang time** - The difference between a pause inside a transmission and the gap between two, which is the whole of the call model. On a scanner the squelch is what tells them apart, because it follows the carrier and not the voice
- **Squelch attack and tail** - Why opening late is heard as a missing word while closing late is only heard as hiss, and why a speaker and a file therefore want the same detector asked two different questions
- **Pre-roll buffering** - The usual fixed-prepend approach this package exists to replace, and why a constant cannot stand in for a varying delay
- **Sensor fusion** - Two sources measuring the same event with different strengths, and the general shape of using each for what it is good at instead of averaging them
- **Deterministic time in tests** - Reading the clock from the data rather than from the system, which is what makes every case here reproducible

