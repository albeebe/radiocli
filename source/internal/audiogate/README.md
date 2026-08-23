# audiogate

## What this does?
Finds the transmissions in a stream of scanner audio. It takes every frame the sound card produces, plus whatever the radio says it is receiving, and hands back the transmissions: where each one started, the audio it is made of, and where it ended.

## Why we use it?
Recording a scanner means reconciling two sources that nothing synchronises. The audio arrives on a sound card. What is being received arrives on the serial cable, polled, and therefore always some unknown amount late. Software that starts a recording the moment the radio says so records a transmission whose first syllable has already gone.

The usual answer is to prepend a fixed amount of buffered audio, half a second or five, chosen by the user. That answer is wrong in both directions at once. Set it short and the opening is still clipped when the news was slow. Set it long and every recording carries however much dead air the guess overshot by. It cannot be right, because it is a constant standing in for something that varies.

This package does not race, and does not guess. It holds the last ten seconds of audio and decides nothing in real time. When a transmission is noticed, however late, the audio from before it is still in the buffer, so the beginning can be found by looking for it: walk backwards to where the energy rose out of the noise floor, and start there. The same walk forwards trims the tail. A late trigger cannot clip the opening and a generous one cannot pad the file, because neither the trigger nor a constant decides where the file begins.

That splits the work along the line each source is actually good for. The radio says **that** a transmission happened and **what** it was. The audio says **exactly when** it started and stopped. Neither is asked to do the other's job, which is what makes the timing of the radio's answer stop mattering: a sample only has to land somewhere inside the transmission to identify it, and the file boundaries never depend on when it arrived.

The one thing the audio genuinely cannot do is tell two transmissions apart when they follow each other closely on different channels. To a sound card that is one event. The radio knows better, so a change of channel cuts the recording, and the audio is overruled on the single question it is not equipped to answer.

There is also no threshold setting here, deliberately. The level that counts as signal is a fixed margin above a continuously measured noise floor, and the floor is measured because it is a property of somebody's cable, dongle and volume knob rather than of audio in general. Asking a user for it is asking them to measure something they have no instrument for, and getting it wrong produces either recordings full of nothing or no recordings at all. The floor is the quietest thing heard in the last fifteen seconds, which is what lets it be measured straight through a transmission: speech is loud, and a minimum ignores loud.

Keeping all of this in a package that touches no files, no sound card, no radio and no clock is what makes it testable. Time comes from the frames themselves, so the tests drive an afternoon of scanner traffic through it in a millisecond, including the cases that cannot be staged against real hardware: a trigger arriving three seconds late, a radio reporting activity into a cable that was never plugged in, a noise floor that climbs, and two transmissions inside one buffer window.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/audiogate"

// Every field may be left zero for a sensible default.
gate := audiogate.New(audiogate.Options{
    Hang:        2 * time.Second,
    MinDuration: time.Second,
    MaxDuration: 5 * time.Minute,
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

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Sliding window minimum** - The monotonic queue behind the noise floor, and why a minimum rather than an average is what stops a transmission raising the level needed to trigger it
- **Squelch and hang time** - The difference between a pause inside a transmission and the gap between two, which is the whole of the call model
- **Pre-roll buffering** - The usual fixed-prepend approach this package exists to replace, and why a constant cannot stand in for a varying delay
- **Sensor fusion** - Two sources measuring the same event with different strengths, and the general shape of using each for what it is good at instead of averaging them
- **Deterministic time in tests** - Reading the clock from the data rather than from the system, which is what makes every case here reproducible

