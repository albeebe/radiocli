# audiogate

## What this does?
Finds the transmissions in a stream of scanner audio. It takes every frame the sound card produces, plus whatever the radio says it is receiving, and hands back the transmissions: where each one started, the audio it is made of, and where it ended.

## Why we use it?
Recording a scanner means reconciling two sources that nothing synchronises. The audio arrives on a sound card, while what is being received arrives on the serial cable, polled, and therefore always some unknown amount late, so software that starts a recording the moment the radio says so records a transmission whose first syllable has already gone. The usual answer is to prepend a fixed amount of buffered audio chosen by the user, and that answer is wrong in both directions at once: set it short and the opening is still clipped when the news was slow, set it long and every recording carries however much dead air the guess overshot by. This package holds the last ten seconds of audio and decides nothing in real time. When a transmission is noticed, however late, the beginning is found by walking backwards through the buffer to where the energy rose out of the noise floor, and the same walk forwards trims the tail, so neither the trigger nor a constant decides where the file begins. Each source is used for what it is good at: the radio says that a transmission happened and what it was, and because its mute follows the carrier rather than the speech, its closing is what separates a dispatcher from the unit answering, a boundary nothing in the waveform can find. The audio says when the sound actually began and ended. There is no threshold setting anywhere, because the level that counts as signal is a margin above a continuously measured noise floor, a property of somebody's cable and volume knob that no user can be asked to measure.

Keeping the detector in its own package is what makes any of this testable. It touches no files, no sound card, no radio and no clock, and time comes from the frames themselves, so the tests drive an afternoon of scanner traffic through it in a millisecond, including the cases that cannot be staged against real hardware: a trigger arriving three seconds late, a radio reporting activity into a cable that was never plugged in, and a noise floor that climbs mid-recording. It also keeps the recorder honest about what it depends on. Everything downstream sees only three kinds of event, a start that is never emitted for a transmission too short to keep, the audio, and an end that says why, so a caller can open a file on a start and never has to delete one afterwards.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/audiogate"

// Every field may be left zero for a sensible default.
gate := audiogate.New(audiogate.Options{RequireRadio: true})

// Whenever the radio is asked what it is doing, tell the gate. The key is
// opaque here: build it from whatever distinguishes one channel from another.
gate.Activity(sampledAt, audiogate.Activity{On: true, Key: "Pocahontas/Fire/Marlinton Dispatch"})

// Then hand it every frame, and act on what comes back.
for frame := range sub.Frames() {
    for _, ev := range gate.Offer(frame) {
        switch ev.Kind {
        case audiogate.KindStart:
            open(ev.Tx.Start) // Already past MinDuration, so the file is worth creating
        case audiogate.KindAudio:
            write(ev.Frame.PCM)
        case audiogate.KindEnd:
            done(ev.Tx) // Carries the real span, why it ended, and frames lost on the way
        }
    }
}

// On the way out, so a transmission in progress is still kept.
for _, ev := range gate.Flush() {
    handle(ev)
}
```

Something playing the audio as it arrives should ask `gate.Live(frame)` instead of following the events, which are deliberately late so that files can be trimmed. `Live` is true from the moment a transmission opens, so a speaker does not lose the first word of every exchange.

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Squelch and hang time** - The difference between a pause inside a transmission and the gap between two, which is the whole of the call model. On a scanner the mute follows the carrier, not the voice, and that is what tells them apart
- **Noise floor estimation** - Why the trigger level is a margin above a measured percentile of recent audio rather than a setting, and why the minimum of the window was tried first and was wrong
- **Pre-roll buffering** - The usual fixed-prepend approach this package exists to replace, and why a constant cannot stand in for a varying delay
- **Sensor fusion** - Two sources measuring the same event with different strengths, and the general shape of using each for what it is good at instead of averaging them
- **Deterministic time in tests** - Reading the clock from the data rather than from the system, which is what makes every case here reproducible
