// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package audiogate

import (
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/audiofeed"
)

// The levels the tests drive the gate with.
//
// Quiet sits where a line input with a squelched scanner on it sits, and loud
// is speech. They are far enough apart that nothing here depends on where
// exactly the margin falls, which is the point: a test that only passed with
// the margin at its current value would be testing the constant rather than the
// detector.
const (
	loudLevel  = -20.0
	quietLevel = -70.0
)

// frameGap is how much time one frame covers, taken from the feed so that a
// change there cannot leave these tests quietly measuring the wrong thing.
const frameGap = audiofeed.FrameMS * time.Millisecond

// start is when every test's audio begins, fixed so failures name the same
// times every run.
var start = time.Date(2026, 8, 22, 19, 54, 3, 0, time.UTC)

// driver feeds a gate synthetic audio and keeps track of where in the stream it
// has got to.
//
// Every clock the gate reads comes from the frames it is given, so a driver can
// push an afternoon of scanner traffic through in a millisecond and get exactly
// what the hardware would have produced.
type driver struct {
	// g is the gate under test.
	g *Gate

	// seq is the next frame number, which the gate reads as a clock and uses
	// to spot audio that went missing.
	seq uint32

	// at is when the next frame was captured.
	at time.Time
}

// newDriver returns a driver wrapping a gate built from opts.
//
// Parameters:
//   - opts: the call model to build the gate with
//
// Returns:
//   - a driver positioned at the start of the stream
func newDriver(opts Options) *driver {
	return &driver{g: New(opts), at: start}
}

// at returns when frame n was captured, for a test checking where a recording
// was cut.
//
// Parameters:
//   - n: the frame number, counting from the start of the stream
//
// Returns:
//   - when that frame was captured
func at(n int) time.Time {
	return start.Add(time.Duration(n) * frameGap)
}

// feed offers n frames at level and returns everything they produced.
//
// Parameters:
//   - n: how many frames to offer
//   - level: the level to give every one of them, in dBFS
//
// Returns:
//   - the events the gate produced, in order
func (d *driver) feed(n int, level float64) []Event {
	var out []Event
	for range n {
		out = append(out, d.g.Offer(audiofeed.Frame{
			Seq:   d.seq,
			PCM:   []byte{byte(d.seq)},
			Level: level,
			At:    d.at,
		})...)
		d.seq++
		d.at = d.at.Add(frameGap)
	}
	return out
}

// radio tells the gate what the scanner is doing, as of now in the stream.
//
// Parameters:
//   - on: whether the radio is receiving
//   - key: what it is receiving, opaque to the gate
func (d *driver) radio(on bool, key string) {
	d.g.Activity(d.at, Activity{On: on, Key: key})
}

// skip advances the frame numbering without offering anything, which is what
// audio lost between the sound card and the gate looks like.
//
// Parameters:
//   - n: how many frames went missing
func (d *driver) skip(n int) {
	d.seq += uint32(n)
	d.at = d.at.Add(time.Duration(n) * frameGap)
}

// audio returns just the frames out of a run of events.
//
// Parameters:
//   - evs: the events to filter
//
// Returns:
//   - the frames, in order
func audio(evs []Event) []audiofeed.Frame {
	var out []audiofeed.Frame
	for _, e := range evs {
		if e.Kind == KindAudio {
			out = append(out, e.Frame)
		}
	}
	return out
}

// only returns the transmissions of one kind out of a run of events.
//
// Parameters:
//   - evs: the events to filter
//   - k: the kind wanted, KindStart or KindEnd
//
// Returns:
//   - the transmission each matching event described, in order
func only(evs []Event, k Kind) []Transmission {
	var out []Transmission
	for _, e := range evs {
		if e.Kind == k {
			out = append(out, e.Tx)
		}
	}
	return out
}

// quick is a call model with everything shortened, so a test spends tens of
// frames on what the defaults would spend hundreds on.
//
// Returns:
//   - options with a short minimum and hang, and the default split
func quick() Options {
	return Options{MinDuration: 100 * time.Millisecond, Hang: 200 * time.Millisecond}
}

// TestNewFillsInDefaults checks that a zero Options is usable, since every
// field of it is something a caller may reasonably have no opinion about.
func TestNewFillsInDefaults(t *testing.T) {
	g := New(Options{})

	// With no radio to ask, the hang is the one measured off the audio.
	if g.opts.Hang != DefaultQuietHang {
		t.Errorf("Hang is %v, want %v", g.opts.Hang, DefaultQuietHang)
	}
	if g.opts.MinDuration != DefaultMinDuration {
		t.Errorf("MinDuration is %v, want %v", g.opts.MinDuration, DefaultMinDuration)
	}
	if g.opts.MaxDuration != DefaultMaxDuration {
		t.Errorf("MaxDuration is %v, want %v", g.opts.MaxDuration, DefaultMaxDuration)
	}

	// And that a gate following the radio gets the shorter one instead, since
	// what it is waiting out is a carrier rather than a pause in speech.
	if g := New(Options{RequireRadio: true}); g.opts.Hang != DefaultHang {
		t.Errorf("Hang with a radio is %v, want %v", g.opts.Hang, DefaultHang)
	}

	// And that a caller who does have an opinion keeps it.
	g = New(Options{Hang: time.Second, MinDuration: time.Second, MaxDuration: time.Minute})
	if g.opts.Hang != time.Second || g.opts.MinDuration != time.Second || g.opts.MaxDuration != time.Minute {
		t.Errorf("New overwrote the options it was given: %+v", g.opts)
	}
}

// TestOneTransmission is the ordinary case end to end, and pins down exactly
// where the recording was cut at both ends.
//
// The numbers matter. The audio runs from frame 50 to 149, so the recording
// must begin a pad before frame 50 and end a pad after frame 149, and it must
// contain every frame in between and none outside.
func TestOneTransmission(t *testing.T) {
	d := newDriver(Options{})

	evs := d.feed(50, quietLevel)
	evs = append(evs, d.feed(100, loudLevel)...)
	evs = append(evs, d.feed(200, quietLevel)...)

	starts, ends := only(evs, KindStart), only(evs, KindEnd)
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("got %d starts and %d ends, want 1 of each", len(starts), len(ends))
	}

	// The onset is found in the buffer, not taken from where anything was
	// noticed, so it lands a pad before the first loud frame.
	if want := at(50 - padFrames); !ends[0].Start.Equal(want) {
		t.Errorf("recording starts at %v, want %v", ends[0].Start, want)
	}
	// And the tail is trimmed back to a pad after the last loud frame rather
	// than running on through the hang time.
	if want := at(149).Add(padDuration); !ends[0].End.Equal(want) {
		t.Errorf("recording ends at %v, want %v", ends[0].End, want)
	}
	if ends[0].Reason != ReasonHang {
		t.Errorf("ended for %q, want %q", ends[0].Reason, ReasonHang)
	}

	// Every frame in that span and nothing else, so that the audio in the file
	// is exactly as long as the file claims to be.
	got := audio(evs)
	if want := int(ends[0].End.Sub(ends[0].Start) / frameGap); len(got) != want {
		t.Fatalf("recording holds %d frames but spans %d, want them equal", len(got), want)
	}
	if !got[0].At.Equal(ends[0].Start) {
		t.Errorf("first frame is at %v, want the start %v", got[0].At, ends[0].Start)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Fatalf("frames jump from %d to %d, want them consecutive", got[i-1].Seq, got[i].Seq)
		}
	}
}

// TestStartComesBeforeTheRadioSaysAnything is the whole point of the package.
//
// The radio is told to report activity a full second after the audio began,
// which is far worse than a real poll would ever be. The recording must still
// start where the sound did, because it was in the buffer the entire time.
func TestStartComesBeforeTheRadioSaysAnything(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	evs := d.feed(50, loudLevel) // A second of audio nothing has noticed yet.
	d.radio(true, "green-bank")
	evs = append(evs, d.feed(50, loudLevel)...)
	d.radio(false, "")
	evs = append(evs, d.feed(200, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d transmissions, want 1", len(ends))
	}
	if want := at(50 - padFrames); !ends[0].Start.Equal(want) {
		t.Errorf("recording starts at %v, want %v, a second before the radio spoke", ends[0].Start, want)
	}
	if ends[0].Key != "green-bank" {
		t.Errorf("key is %q, want it adopted from the radio", ends[0].Key)
	}
}

// TestRadioOpensBeforeTheAudioArrives is the same problem the other way round:
// the radio reports activity and the sound turns up later. The silence in
// between must not be recorded.
func TestRadioOpensBeforeTheAudioArrives(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	d.radio(true, "marlinton")
	d.feed(100, quietLevel) // Two seconds of the radio insisting, and nothing.
	evs := d.feed(100, loudLevel)
	d.radio(false, "")
	evs = append(evs, d.feed(200, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d transmissions, want 1", len(ends))
	}
	if want := at(150 - padFrames); !ends[0].Start.Equal(want) {
		t.Errorf("recording starts at %v, want %v: the waiting must not be in the file", ends[0].Start, want)
	}
}

// TestTransmissionAtTheVeryStartOfTheStream checks the trim when there is less
// audio in front of the onset than the pad would like to keep.
//
// It happens when the tool is started and a transmission arrives immediately,
// so there is barely any buffer to walk back into. Keeping everything held is
// the answer, and walking back a fixed pad regardless would run off the front
// of it.
func TestTransmissionAtTheVeryStartOfTheStream(t *testing.T) {
	d := newDriver(quick())

	d.feed(3, quietLevel) // Barely enough to know what the floor is.
	d.radio(true, "frost")
	d.feed(1, quietLevel) // The radio opens it before any sound arrives.
	evs := d.feed(50, loudLevel)
	d.radio(false, "")
	evs = append(evs, d.feed(100, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d recordings, want 1", len(ends))
	}
	// Everything held is kept, back to the first frame there ever was.
	if want := at(0); !ends[0].Start.Equal(want) {
		t.Errorf("recording starts at %v, want %v", ends[0].Start, want)
	}
	if got, want := len(audio(evs)), int(ends[0].End.Sub(ends[0].Start)/frameGap); got != want {
		t.Errorf("recording holds %d frames but spans %d", got, want)
	}
}

// TestRadioIntoADeadCableRecordsNothing checks the case that matters most for
// somebody who has plugged the audio lead into the wrong socket.
//
// The radio reports a long transmission and nothing ever comes through. That
// must produce no recording at all, rather than an evening of silent files, and
// it must not accumulate frames without bound while it waits.
func TestRadioIntoADeadCableRecordsNothing(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	d.radio(true, "arbovale")
	evs := d.feed(4*maxRingFrames, quietLevel)

	if n := len(evs); n != 0 {
		t.Fatalf("got %d events, want none for a transmission with no audio in it", n)
	}
	if held := len(d.g.tx.pending); held > maxRingFrames+1 {
		t.Errorf("holding %d frames, want no more than the buffer's %d", held, maxRingFrames)
	}

	// And nothing is produced when it finally gives up, either.
	d.radio(false, "")
	if evs := d.feed(200, quietLevel); len(evs) != 0 {
		t.Fatalf("got %d events on closing, want none", len(evs))
	}
}

// TestShortTransmissionIsDiscarded checks that a blip never reaches the caller,
// which is what lets the caller create a file on KindStart without ever having
// to delete one.
func TestShortTransmissionIsDiscarded(t *testing.T) {
	d := newDriver(Options{}) // A one second minimum.

	d.feed(50, quietLevel)
	evs := d.feed(20, loudLevel) // 400 ms.
	evs = append(evs, d.feed(200, quietLevel)...)

	if len(evs) != 0 {
		t.Fatalf("got %d events, want none: %v", len(evs), evs)
	}
}

// TestPauseShorterThanHangDoesNotSplit checks the setting that exists so a
// speaker drawing breath does not become two recordings.
// TestTwoKeyupsOnOneChannelAreTwoRecordings drives the exchange that started
// all of this: a dispatcher and a unit answering, on the same channel, with the
// scanner's mute shut between them.
//
// The gap is the one measured off the radio, 640 milliseconds, which is shorter
// than the hang used before the mute was understood to be a keyup and longer
// than the one used now. Nothing about the audio distinguishes it from a pause
// in the middle of one transmission. Only the radio does.
func TestTwoKeyupsOnOneChannelAreTwoRecordings(t *testing.T) {
	d := newDriver(Options{RequireRadio: true})

	d.feed(50, quietLevel)

	// The dispatcher, a second and a half of it.
	d.radio(true, "marlinton")
	evs := d.feed(75, loudLevel)

	// Unkeys. The mute shuts, and the channel is held open by the scanner's
	// own delay, so the radio still names the same channel when it comes back.
	d.radio(false, "")
	evs = append(evs, d.feed(32, quietLevel)...) // 640ms.

	// The unit answering, on the same channel.
	d.radio(true, "marlinton")
	evs = append(evs, d.feed(55, loudLevel)...)

	d.radio(false, "")
	evs = append(evs, d.feed(200, quietLevel)...)

	starts := only(evs, KindStart)
	if len(starts) != 2 {
		t.Fatalf("got %d recordings, want 2: the two speakers were not separated", len(starts))
	}

	// And the boundary is the keyup rather than anything the audio suggested,
	// so the second recording begins after the gap and not inside it.
	ends := only(evs, KindEnd)
	if len(ends) != 2 {
		t.Fatalf("got %d endings, want 2", len(ends))
	}
	if !starts[1].Start.After(ends[0].End) {
		t.Errorf("the second recording starts at %v, before the first ended at %v",
			starts[1].Start, ends[0].End)
	}
}

func TestPauseShorterThanHangDoesNotSplit(t *testing.T) {
	d := newDriver(Options{}) // A two second hang.

	d.feed(50, quietLevel)
	evs := d.feed(100, loudLevel)
	evs = append(evs, d.feed(50, quietLevel)...) // A second of silence mid-sentence.
	evs = append(evs, d.feed(100, loudLevel)...)
	evs = append(evs, d.feed(200, quietLevel)...)

	if starts := only(evs, KindStart); len(starts) != 1 {
		t.Fatalf("got %d recordings, want 1: a pause inside a transmission split it", len(starts))
	}

	// And the pause itself is kept, because it happened inside the recording.
	ends := only(evs, KindEnd)
	if want := at(299).Add(padDuration); !ends[0].End.Equal(want) {
		t.Errorf("recording ends at %v, want %v", ends[0].End, want)
	}
}

// TestGapLongerThanHangSplits is the other half of the same rule.
func TestGapLongerThanHangSplits(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	evs := d.feed(100, loudLevel)
	evs = append(evs, d.feed(200, quietLevel)...) // Four seconds, well past the hang.
	evs = append(evs, d.feed(100, loudLevel)...)
	evs = append(evs, d.feed(200, quietLevel)...)

	if starts := only(evs, KindStart); len(starts) != 2 {
		t.Fatalf("got %d recordings, want 2", len(starts))
	}
	for i, e := range only(evs, KindEnd) {
		if e.Reason != ReasonHang {
			t.Errorf("recording %d ended for %q, want %q", i, e.Reason, ReasonHang)
		}
	}
}

// TestChannelChangeSplits checks the one question the audio cannot answer.
//
// Two transmissions on different channels follow each other with no gap at all,
// so the sound card sees one continuous event. The radio knows better, and the
// radio wins.
func TestChannelChangeSplits(t *testing.T) {
	d := newDriver(quick())

	d.feed(50, quietLevel)
	d.radio(true, "durbin")
	evs := d.feed(50, loudLevel)
	d.radio(true, "cass") // Straight to another channel, no silence between.
	evs = append(evs, d.feed(50, loudLevel)...)
	d.radio(false, "")
	evs = append(evs, d.feed(100, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) != 2 {
		t.Fatalf("got %d recordings, want 2", len(ends))
	}
	if ends[0].Reason != ReasonChannel {
		t.Errorf("the first ended for %q, want %q", ends[0].Reason, ReasonChannel)
	}
	if ends[0].Key != "durbin" || ends[1].Key != "cass" {
		t.Errorf("keys are %q and %q, want durbin and cass", ends[0].Key, ends[1].Key)
	}

	// The frame at the boundary belongs to exactly one of them.
	for i := 1; i < len(audio(evs)); i++ {
		if audio(evs)[i].Seq == audio(evs)[i-1].Seq {
			t.Fatalf("frame %d appears in both recordings", audio(evs)[i].Seq)
		}
	}
}

// TestMaxDurationSplits checks that one transmission cannot grow without bound,
// and that the audio carries on across the cut rather than being lost at it.
func TestMaxDurationSplits(t *testing.T) {
	d := newDriver(Options{MinDuration: 100 * time.Millisecond, Hang: 200 * time.Millisecond,
		MaxDuration: time.Second})

	d.feed(50, quietLevel)
	evs := d.feed(250, loudLevel) // Five seconds of a stuck microphone.
	evs = append(evs, d.feed(100, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) < 4 {
		t.Fatalf("got %d recordings from five seconds at a one second limit, want at least 4", len(ends))
	}
	for _, e := range ends[:len(ends)-1] {
		if e.Reason != ReasonSplit {
			t.Errorf("a recording ended for %q, want %q", e.Reason, ReasonSplit)
		}
	}

	// No frame is dropped at a boundary and none is recorded twice.
	got := audio(evs)
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Fatalf("frames jump from %d to %d across a split", got[i-1].Seq, got[i].Seq)
		}
	}
}

// TestDroppedFramesAreReported checks that audio lost on the way here is
// admitted to, since concatenating across a gap silently shortens the recording
// and nothing downstream could tell.
func TestDroppedFramesAreReported(t *testing.T) {
	d := newDriver(quick())

	d.feed(50, quietLevel)
	evs := d.feed(20, loudLevel)
	d.skip(7) // Seven frames the sound card produced and nothing received.
	evs = append(evs, d.feed(20, loudLevel)...)
	evs = append(evs, d.feed(100, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d recordings, want 1", len(ends))
	}
	if ends[0].Dropped != 7 {
		t.Errorf("reported %d dropped frames, want 7", ends[0].Dropped)
	}
}

// TestFrameNumberGoingBackwardsIsNotCountedAsLoss checks the guard on the
// arithmetic, because the numbering is unsigned and a subtraction that went
// negative would wrap into an enormous count.
func TestFrameNumberGoingBackwardsIsNotCountedAsLoss(t *testing.T) {
	d := newDriver(quick())
	d.feed(50, quietLevel)

	d.seq = 10 // The stream restarted, as it would if the card were reopened.
	evs := d.feed(20, loudLevel)
	evs = append(evs, d.feed(100, quietLevel)...)

	if ends := only(evs, KindEnd); len(ends) != 1 || ends[0].Dropped != 0 {
		t.Fatalf("got %+v, want one recording reporting no loss", ends)
	}
}

// TestFlushClosesAnOpenTransmission checks that stopping part way through keeps
// the part that happened.
func TestFlushClosesAnOpenTransmission(t *testing.T) {
	d := newDriver(quick())

	d.feed(50, quietLevel)
	d.feed(50, loudLevel)

	evs := d.g.Flush()
	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d recordings from Flush, want 1", len(ends))
	}
	if ends[0].Reason != ReasonStopped {
		t.Errorf("ended for %q, want %q", ends[0].Reason, ReasonStopped)
	}
	if len(audio(evs)) == 0 {
		t.Error("Flush produced no audio, want the frames held back for trimming")
	}

	// And flushing again, or with nothing open, says nothing.
	if evs := d.g.Flush(); evs != nil {
		t.Errorf("a second Flush produced %d events, want none", len(evs))
	}
}

// TestFlushOnAnUnconfirmedTransmissionSaysNothing checks that a blip
// interrupted by shutdown is still a blip.
func TestFlushOnAnUnconfirmedTransmissionSaysNothing(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	d.feed(5, loudLevel) // 100 ms, far short of the minimum.

	if evs := d.g.Flush(); len(evs) != 0 {
		t.Fatalf("got %d events, want none", len(evs))
	}
}

// TestFloorFollowsTheInput checks that the level counting as signal is derived
// from the audio rather than set, which is what removes the threshold setting
// this package deliberately does not have.
func TestFloorFollowsTheInput(t *testing.T) {
	d := newDriver(Options{})

	if got := d.g.Floor(); got != floorMin {
		t.Errorf("the floor before any audio is %v, want %v", got, floorMin)
	}

	d.feed(50, quietLevel)
	if got := d.g.Floor(); got != quietLevel {
		t.Errorf("the floor is %v, want it settled on %v", got, quietLevel)
	}

	// One quiet frame does not move it, which is the whole reason this is a
	// percentile. A single dip is a sample of the noise, not the noise.
	d.feed(1, quietLevel-10)
	if got := d.g.Floor(); got != quietLevel {
		t.Errorf("one quiet frame moved the floor to %v, want it left at %v", got, quietLevel)
	}

	// A genuinely quieter input is followed once enough of the window is quiet.
	d.feed(floorFrames, quietLevel-10)
	if got := d.g.Floor(); got != quietLevel-10 {
		t.Errorf("the floor is %v, want it to have followed the input to %v", got, quietLevel-10)
	}
}

// TestNoisyInputDoesNotReadAsSignal is a regression from real hardware.
//
// A USB line input with a squelched scanner on it sat at about -78 dBFS with a
// spread of some three decibels, and its quietest single frame in fifteen
// seconds was -87. The floor was taken from that minimum at first, which put the
// trigger level nine decibels below where the noise actually was, so the noise
// read as signal and the recorder wrote the hiss between transmissions as
// though it were traffic: two files, thirty-eight and fifteen seconds long,
// with nothing in them.
//
// The numbers here are that measurement.
func TestNoisyInputDoesNotReadAsSignal(t *testing.T) {
	d := newDriver(Options{})

	// Noise around -78, dipping to -87 once in a while, exactly as measured.
	for i := range 3 * floorFrames {
		level := -77.7
		switch {
		case i%97 == 0:
			level = -87.4
		case i%7 == 0:
			level = -75.5
		case i%3 == 0:
			level = -78.3
		}
		if evs := d.feed(1, level); len(evs) != 0 {
			t.Fatalf("the noise floor produced %d events at frame %d, want none", len(evs), i)
		}
	}
	if d.g.tx != nil {
		t.Fatal("a transmission was opened on nothing but the noise floor")
	}

	// And real audio on the same input still opens one.
	if evs := d.feed(100, -30); len(evs) == 0 && d.g.tx == nil {
		t.Fatal("speech on the same input did not open a transmission")
	}
}

// TestFloorIsClampedAtBothEnds checks the two readings that are not noise
// floors at all: digital silence, which means something took the audio away,
// and a level so high the input cannot be a squelched scanner.
func TestFloorIsClampedAtBothEnds(t *testing.T) {
	t.Run("digital silence", func(t *testing.T) {
		d := newDriver(Options{})
		d.feed(100, -200) // Far below anything a real input produces.

		if got := d.g.Floor(); got != floorMin {
			t.Errorf("the floor is %v, want it pinned at %v", got, floorMin)
		}
	})

	t.Run("a room rather than a radio", func(t *testing.T) {
		d := newDriver(Options{})
		d.feed(500, 0) // As loud as it gets, continuously.

		if got := d.g.Floor(); got != floorMax {
			t.Errorf("the floor is %v, want it pinned at %v", got, floorMax)
		}
	})
}

// TestSpeechDoesNotRaiseTheFloor checks the feedback loop that would otherwise
// close the gate on itself: a transmission raising the floor until the level
// needed to trigger is above anything the radio produces.
//
// A minimum is what makes this safe, and it is why the floor can be measured
// through a transmission rather than frozen while one is open.
func TestSpeechDoesNotRaiseTheFloor(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	before := d.g.Floor()
	d.feed(300, loudLevel) // Six seconds of speech, well inside the window.

	if got := d.g.Floor(); got != before {
		t.Errorf("the floor moved to %v during a transmission, want it left at %v", got, before)
	}
}

// TestFloorRecoversWhenTheInputChanges checks the other half of measuring
// through a transmission: an input that genuinely got noisier is followed, so
// the gate cannot be left latched open by a floor frozen at an old value.
func TestFloorRecoversWhenTheInputChanges(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	// The floor rises by more than the margin, so every frame now reads as
	// signal against the old estimate.
	d.feed(floorFrames+50, quietLevel+20)

	if got, want := d.g.Floor(), quietLevel+20; got != want {
		t.Errorf("the floor is %v, want it to have followed the input to %v", got, want)
	}
}

// TestTheFirstFrameNeverTriggers checks that the gate does not open a
// transmission on whatever the sound card happened to hand over first, before
// there was any floor to judge it against.
func TestTheFirstFrameNeverTriggers(t *testing.T) {
	d := newDriver(quick())

	if evs := d.feed(1, loudLevel); len(evs) != 0 {
		t.Fatalf("got %d events from the very first frame, want none", len(evs))
	}
	if d.g.tx != nil {
		t.Error("the first frame opened a transmission")
	}
}

// TestRingDoesNotGrow checks that idle audio is bounded, since a scanner is
// silent most of the time and this is the state it spends its life in.
func TestRingDoesNotGrow(t *testing.T) {
	d := newDriver(Options{})
	d.feed(10*maxRingFrames, quietLevel)

	if n := len(d.g.ring); n > maxRingFrames {
		t.Errorf("the buffer holds %d frames, want no more than %d", n, maxRingFrames)
	}
}

// TestOnsetSearchStopsAtTheBuffer checks a transmission longer than the buffer
// it is looked for in, which is the case where the walk backwards would run off
// the end.
func TestOnsetSearchStopsAtTheBuffer(t *testing.T) {
	d := newDriver(Options{})

	// Fill the buffer with audio that is already loud, then let the radio be
	// what opens the transmission, so the walk has nothing to stop it.
	d.feed(50, quietLevel)
	evs := d.feed(2*maxRingFrames, loudLevel)
	evs = append(evs, d.feed(200, quietLevel)...)

	starts := only(evs, KindStart)
	if len(starts) != 1 {
		t.Fatalf("got %d recordings, want 1", len(starts))
	}
	if want := at(50 - padFrames); !starts[0].Start.Equal(want) {
		t.Errorf("recording starts at %v, want %v", starts[0].Start, want)
	}
}

// TestLongTransmissionIsHandedOutAsItGoes checks that a recording longer than
// the buffer does not accumulate in memory, since MaxDuration allows minutes
// and the buffer is ten seconds.
func TestLongTransmissionIsHandedOutAsItGoes(t *testing.T) {
	d := newDriver(Options{})

	d.feed(50, quietLevel)
	evs := d.feed(3*maxRingFrames, loudLevel)

	if len(audio(evs)) == 0 {
		t.Fatal("no audio was handed out during a transmission longer than the buffer")
	}
	// What is still held back is the hang time's worth, and no more.
	held := len(d.g.tx.pending)
	if want := int(DefaultQuietHang/frameGap) + padFrames + 2; held > want {
		t.Errorf("holding %d frames mid-transmission, want no more than about %d", held, want)
	}
}

// TestKeyIsEmptyWhenTheRadioSaysNothing checks that a transmission recorded
// with the radio silent carries no identity rather than a made up one.
func TestKeyIsEmptyWhenTheRadioSaysNothing(t *testing.T) {
	d := newDriver(quick())

	d.feed(50, quietLevel)
	evs := d.feed(50, loudLevel)
	evs = append(evs, d.feed(100, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d recordings, want 1", len(ends))
	}
	if ends[0].Key != "" {
		t.Errorf("key is %q, want it empty", ends[0].Key)
	}
}

// TestRadioAloneOpensAndClosesATransmission checks the path where the radio is
// the only trigger and the audio, while present, never rises far above the
// floor. The radio holding the transmission open is what keeps it in one piece.
func TestRadioAloneOpensAndClosesATransmission(t *testing.T) {
	d := newDriver(quick())

	d.feed(50, quietLevel)
	d.radio(true, "bartow")
	evs := d.feed(30, loudLevel)
	// The audio stops but the radio still says it is receiving, so the hang
	// time must not run out underneath it.
	evs = append(evs, d.feed(100, quietLevel)...)
	if len(only(evs, KindEnd)) != 0 {
		t.Fatal("the recording ended while the radio was still receiving")
	}

	d.radio(false, "")
	evs = append(evs, d.feed(100, quietLevel)...)
	if n := len(only(evs, KindEnd)); n != 1 {
		t.Fatalf("got %d recordings, want 1 once the radio stopped", n)
	}
}

// TestRadioIsTheAuthorityOnWhatIsATransmission is the regression that this
// mode exists for, and the numbers in it are from a real recording.
//
// An SDS150's line output idles at about -88 dBFS with the squelch shut and at
// about -77 at other times. A gate that decides for itself measures the floor
// during the quiet stretch, puts the trigger eight decibels above it, and then
// reads the louder idle level as speech for as long as it lasts. What that
// produced was a sixteen second recording of nothing, from a transmission that
// really lasted under a second, with the radio reporting it was receiving on
// exactly one of the forty-eight times it was asked.
//
// With the radio as the authority there is no such recording, because there was
// no such transmission.
func TestRadioIsTheAuthorityOnWhatIsATransmission(t *testing.T) {
	d := newDriver(Options{RequireRadio: true})

	// The quieter idle level, which is what the floor settles on.
	d.feed(200, -88)

	// One brief transmission, confirmed by the radio, then the input moves to
	// its louder idle level and stays there with the radio silent.
	d.radio(true, "marlinton")
	evs := d.feed(10, -43)
	d.radio(false, "")
	evs = append(evs, d.feed(800, -77)...)

	// The transmission was shorter than the default minimum, so nothing is
	// kept, and crucially nothing runs on into the noise.
	if starts := only(evs, KindStart); len(starts) != 0 {
		t.Fatalf("got %d recordings, want none: %+v", len(starts), starts)
	}
	if d.g.tx != nil {
		t.Error("a transmission is still open on the noise floor")
	}
}

// TestRadioOpensAndClosesTheRecording checks the ordinary path in the mode the
// recorder uses: the radio says when, and the audio says exactly when.
func TestRadioOpensAndClosesTheRecording(t *testing.T) {
	d := newDriver(Options{RequireRadio: true, MinDuration: 100 * time.Millisecond,
		Hang: 200 * time.Millisecond})

	d.feed(50, quietLevel)

	// Audio alone does nothing, however loud it is.
	if evs := d.feed(50, loudLevel); len(evs) != 0 {
		t.Fatalf("audio opened a recording with the radio silent: %d events", len(evs))
	}

	// The radio confirms, and the recording begins.
	d.radio(true, "marlinton")
	evs := d.feed(100, loudLevel)
	d.radio(false, "")
	evs = append(evs, d.feed(100, quietLevel)...)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d recordings, want 1", len(ends))
	}
	if ends[0].Key != "marlinton" {
		t.Errorf("the recording is labelled %q, want the radio's channel", ends[0].Key)
	}
	if ends[0].Reason != ReasonHang {
		t.Errorf("ended for %q, want %q", ends[0].Reason, ReasonHang)
	}
}

// TestTheLookBackIsBounded checks that the walk for the onset cannot run back
// through the whole buffer.
//
// It is what stops a floor measured a little low from putting every second of
// held audio at the front of a recording, which is the same failure as the one
// above wearing a different hat.
func TestTheLookBackIsBounded(t *testing.T) {
	d := newDriver(Options{RequireRadio: true, MinDuration: 100 * time.Millisecond,
		Hang: 200 * time.Millisecond})

	// Settle the floor low, then fill the whole buffer with audio above it,
	// with the radio saying nothing at all.
	d.feed(200, -88)
	d.feed(2*maxRingFrames, -70)

	// Only now does the radio speak.
	at := d.at
	d.radio(true, "marlinton")
	evs := d.feed(100, -30)
	d.radio(false, "")
	evs = append(evs, d.feed(100, -88)...)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d recordings, want 1", len(ends))
	}
	if back := at.Sub(ends[0].Start); back > maxLookBack+padDuration {
		t.Errorf("the recording reaches %v back before the radio spoke, want no more than %v",
			back, maxLookBack+padDuration)
	}
}

// TestTheTailIsBoundedByTheRadio checks that audio still reading as loud after
// the radio has stopped is not kept.
//
// A recording should end where the transmission did, not where the input's own
// noise happens to fall back below a threshold.
func TestTheTailIsBoundedByTheRadio(t *testing.T) {
	d := newDriver(Options{RequireRadio: true, MinDuration: 100 * time.Millisecond,
		Hang: 200 * time.Millisecond})

	d.feed(200, -88)
	d.radio(true, "marlinton")
	d.feed(100, -30)

	// The radio stops, and the input settles at a level still above the floor.
	off := d.at
	d.radio(false, "")
	evs := d.feed(300, -70)

	ends := only(evs, KindEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d recordings, want 1", len(ends))
	}
	if over := ends[0].End.Sub(off); over > radioSlack+padDuration {
		t.Errorf("the recording runs %v past the radio, want no more than %v",
			over, radioSlack+padDuration)
	}
}

// Test_hasSpeech tests the hasSpeech method with 100% coverage.
//
// It is what separates a recording from a file of noise. The radio reporting
// activity is not proof that any sound arrived: a lead in the wrong socket, or
// a channel the scanner opened and nothing came through, both clear the eight
// decibel margin on the floor's own wobble and would otherwise be written as
// recordings.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - NoFloor: nothing measured yet is not evidence of speech
//   - Quiet: a transmission that never rose far above a quiet floor is refused
//   - Speech: one that did is accepted
//   - FloorNotQuiet: a floor too high to be a noise floor refuses nothing,
//     since the audio that raised it is the traffic being asked about
func Test_hasSpeech(t *testing.T) {
	// gateWith returns a gate whose floor has been fed level throughout.
	gateWith := func(level float64) *Gate {
		g := New(Options{RequireRadio: true})
		for i := 0; i < floorFrames; i++ {
			g.floor.add(level)
		}
		return g
	}

	// Verify a gate that has measured nothing says no. Treating an unmeasured
	// floor as permission would confirm a recording on the first frame the
	// card handed over.
	t.Run("NoFloor", func(t *testing.T) {
		g := New(Options{RequireRadio: true})

		if g.hasSpeech(&transmission{heard: true, peak: -10}) {
			t.Error("a gate with no floor yet found speech")
		}
	})

	// Verify a transmission that never rose far above a genuinely quiet floor
	// is refused. Measured on a real recording: a floor of -64.7 dBFS and a
	// loudest moment of -40.0, which is noise rather than a voice.
	t.Run("Quiet", func(t *testing.T) {
		g := gateWith(-64.7)

		if g.hasSpeech(&transmission{heard: true, peak: -40}) {
			t.Error("noise 25 dB above the floor was taken for speech")
		}
	})

	// Verify a transmission that did rise is accepted. Every recording holding
	// speech in the same measurement peaked between 48 and 71 dB above its
	// floor.
	t.Run("Speech", func(t *testing.T) {
		g := gateWith(-70)

		if !g.hasSpeech(&transmission{heard: true, peak: -8}) {
			t.Error("speech 62 dB above the floor was not recognised")
		}
	})

	// Verify a floor too high to be a noise floor refuses nothing. A busy
	// channel fills the window the estimate is taken from with speech, and
	// reading that as a high noise floor would throw away the traffic that
	// raised it.
	t.Run("FloorNotQuiet", func(t *testing.T) {
		g := gateWith(-30)

		if !g.hasSpeech(&transmission{heard: true, peak: -20}) {
			t.Error("a floor full of audio was used to reject a transmission")
		}
	})
}

// TestFullScaleAudioIsHeard covers a transmission whose level reads exactly
// zero dBFS, which is the loudest a sample can be rather than the absence of
// one.
//
// It exists because the first version of the peak tracking used zero as the
// value meaning "nothing measured yet". That is wrong for a level: an
// overloaded input delivers frames at exactly zero dBFS, and a transmission
// made of them read as one that had never been heard, so it was refused a file
// for being too quiet.
func TestFullScaleAudioIsHeard(t *testing.T) {
	g := New(Options{RequireRadio: true, Hang: 200 * time.Millisecond,
		MinDuration: 100 * time.Millisecond})

	at := time.Date(2026, 8, 23, 16, 0, 0, 0, time.UTC)
	next := func(level float64, n int) {
		for i := 0; i < n; i++ {
			g.Offer(audiofeed.Frame{Seq: uint32(i), PCM: make([]byte, audiofeed.MonoFrameBytes),
				Level: level, At: at})
			at = at.Add(frameDuration)
		}
	}

	// A quiet stretch first, so the floor is a floor.
	next(-70, floorFrames)

	g.Activity(at, Activity{On: true, Key: "DISPATCH"})
	before := at
	next(0, 50)

	tx := g.tx
	if tx == nil {
		t.Fatal("full scale audio opened no transmission")
	}
	if !tx.heard {
		t.Error("full scale audio was recorded as never heard")
	}
	if tx.peak != 0 {
		t.Errorf("the peak is %v, want 0 dBFS", tx.peak)
	}
	if !g.hasSpeech(tx) {
		t.Error("full scale audio was not recognised as speech")
	}
	if !tx.lastLoud.After(before) {
		t.Error("full scale audio never advanced the last-heard time")
	}
}

// TestLive checks the question something playing audio as it arrives has to
// ask, which is not the question something writing a file asks.
//
// Coverage: 100% (4 test cases covering both branches)
//
// Test cases:
//   - Quiet: the noise floor between transmissions is not live
//   - AtTheFirstLoudFrame: with no radio to ask, audio is live before anything
//     is worth a file
//   - ThroughTheHang: it stays live while a transmission is open, tail included
//   - StaticWithNoRadio: with a radio to ask, audio alone does not open the
//     speakers, however loud it is
func TestLive(t *testing.T) {
	// frame builds one frame at the driver's current position without moving
	// it, so a test can ask about the frame it is about to feed.
	frame := func(d *driver, level float64) audiofeed.Frame {
		return audiofeed.Frame{Seq: d.seq, PCM: []byte{byte(d.seq)}, Level: level, At: d.at}
	}

	// Verify that the hiss a scanner puts out between transmissions is not
	// something to open the speakers for.
	t.Run("Quiet", func(t *testing.T) {
		d := newDriver(Options{})
		d.feed(60, quietLevel)

		f := frame(d, quietLevel)
		d.g.Offer(f)
		if d.g.Live(f) {
			t.Error("the noise floor reads as live, so the squelch would never close")
		}
	})

	// Verify the whole reason this exists: with nothing but the audio to go on,
	// the first loud frame is live, long before the gate has decided the
	// transmission is worth a file. A listener following KindStart instead
	// loses the front of every transmission.
	t.Run("AtTheFirstLoudFrame", func(t *testing.T) {
		d := newDriver(Options{MinDuration: 500 * time.Millisecond})
		d.feed(60, quietLevel)

		f := frame(d, loudLevel)
		events := d.g.Offer(f)

		for _, ev := range events {
			if ev.Kind == KindStart {
				t.Fatal("a file opened on the first loud frame, so this proves nothing")
			}
		}
		if !d.g.Live(f) {
			t.Error("the first loud frame is not live, so the speakers would open late")
		}
	})

	// Verify that a transmission stays live once it is open, including the
	// quiet inside it and the hang at the end, which is the tail a speaker
	// wants and what stops the speakers shutting between two words.
	t.Run("ThroughTheHang", func(t *testing.T) {
		d := newDriver(Options{Hang: 2 * time.Second})
		d.feed(60, quietLevel)
		d.feed(30, loudLevel)

		f := frame(d, quietLevel)
		d.g.Offer(f)
		if !d.g.Live(f) {
			t.Error("a quiet frame inside an open transmission is not live, so the tail would be cut")
		}
	})

	// Verify that with a radio to ask, loud audio on its own opens nothing.
	//
	// This is the case that produced choppy playback in the field: a squelch
	// that opened on static measured against a floor taken while the radio was
	// muted, playing fourteen seconds of hiss between two transmissions of
	// three. A scanner's own output has more than one noise floor, which is why
	// the recorder requires the radio, and why what is heard has to follow the
	// same rule as what is kept.
	t.Run("StaticWithNoRadio", func(t *testing.T) {
		d := newDriver(Options{RequireRadio: true})
		d.feed(60, quietLevel)
		d.radio(false, "")

		f := frame(d, loudLevel)
		d.g.Offer(f)
		if d.g.Live(f) {
			t.Error("static opened the speakers with the radio saying it is receiving nothing")
		}
	})
}
