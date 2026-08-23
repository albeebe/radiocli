// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package audio

import (
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audiogate"
	"github.com/albeebe/radiocli/internal/audioin"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/recordings"
)

// defaultBitrate is what --format opus uses unless told otherwise.
//
// In the middle of the useful band. The encoder here is CELT only, which does
// worse on speech than a hybrid one, so the comfortable range for a voice
// channel is around 32 to 48 kbps rather than the 16 to 24 that libopus would
// hold up at. See the opusenc package.
const defaultBitrate = 32000

// The formats this can write.
const (
	// formatOpus is compressed, for a program rather than a player: there is no
	// container around the packets, so nothing that plays files will read it.
	formatOpus = "opus"

	// formatPCM is the samples themselves, with nothing around them. It is the
	// default because it is the one a player can be pointed straight at.
	formatPCM = "pcm"
)

// outputQueue is how many frames may be waiting to be written before the oldest
// are dropped.
//
// One second. Generous compared with what a remote listener gets, because what is on the
// other end of this is usually a pipe into a player on the same machine, and a
// player that stalls for a moment should catch up rather than lose audio. It is
// still bounded, because the sound card cannot be asked to wait.
const outputQueue = 1000 / audiofeed.FrameMS

// The two numbers that decide whether a recording was overloaded on the way in.
//
// clipCeiling is the sample value at which a converter has run out of room.
// Anything louder than full scale is handed back as full scale, so a run of
// these is the flat top of a waveform that was too big for the input.
//
// clipFraction is how much of a transmission has to be flat before it is worth
// interrupting somebody about. Measured on an SDS150 through the same cable
// into both jacks: twenty-three recordings through a line input contained
// exactly zero full-scale samples between them, out of four and a half million,
// while every overloaded recording through a mic input ran from 1.4% to 19%.
// The gap is wide enough that the threshold hardly matters, so it is set low
// and left there. One sample in a thousand is far above what clean audio
// produces and far below what a mic input does.
const (
	clipCeiling  = 32767
	clipFraction = 0.001
)

// meterEvery is how many frames go by between level readings under --verbose.
//
// About one every two seconds. Fast enough to watch somebody move a volume
// knob, slow enough that a night of it is readable.
const meterEvery = 2000 / audiofeed.FrameMS

// The mismatch check: how much disagreement between the radio and the sound
// card is enough to say the input is not the scanner.
const (
	// mismatchLimit is how many times the two must disagree before it is
	// mentioned. High enough that the ordinary overlap at the edges of a
	// transmission cannot reach it, since the radio and the audio are never
	// going to agree to the millisecond.
	mismatchLimit = 150

	// mismatchWindow is how long one kind of disagreement must persist to
	// count as continuous rather than momentary.
	mismatchWindow = 30 * time.Second
)

// mismatchMargin is how far above the noise floor the mismatch check counts a
// frame as carrying sound.
//
// Wider than the gate's own margin on purpose. This is not deciding what to
// record, it is deciding whether to tell somebody their cable is wrong, and a
// false accusation is worse than a slow one.
const mismatchMargin = 12.0

// recordQueue is how many frames may be waiting for the recorder before the
// oldest are dropped.
//
// Two seconds, which is generous, because what is on the other end of this is a
// disk rather than a person. A write that stalls for a moment should be caught
// up with rather than punched a hole in the recording, and the gate reports the
// hole either way if one happens.
const recordQueue = 2000 / audiofeed.FrameMS

// samplePeriod is how often the scanner is asked what it is hearing while a
// recording is running.
//
// Labelling a recording does not need this to be fast, and that is the point of
// the design rather than a compromise: the audio is buffered, so the boundaries
// of a recording never depend on when the radio was asked, and a sample only
// has to land somewhere inside a transmission to identify it.
//
// Separating one transmission from the next does need it. The scanner has no
// keyup event to subscribe to, so the only way to see one is to catch the
// moment its Mute closes, and that moment can be brief. Two consecutive
// transmissions on the same channel, measured off an SDS150, ran 60.39s to
// 61.92s and 62.56s to 63.64s: a gap of 640 milliseconds with the mute shut.
// Asked three times a second, that is two polls, which is not enough to tell a
// closed squelch from a reply that happened to land between questions, and both
// halves of the exchange end up in one file.
//
// A tenth of a second puts six polls inside the shortest gap seen, and costs
// nothing worth counting: the same radio answered GSI 81 times a second when
// asked flat out, so this is an eighth of what it can do.
const samplePeriod = 100 * time.Millisecond

// listSources lists the sound inputs this computer can record from. It is a
// var so tests can substitute a fake and never enumerate a real sound card.
var listSources = audioin.Sources

// startCapture opens the sound card and begins publishing frames into out. It
// is a var so tests can substitute a fake and never open a real sound card,
// which on macOS is what raises the microphone permission prompt.
var startCapture = func(opts audiofeed.Options, out audiofeed.Publisher) (capture, error) {
	return audiofeed.Start(opts, out)
}

// capture is the part of an open sound card that outputDirect uses: it says
// what it is recording from, and it can be closed. It is an interface rather
// than *audiofeed.Capture so that startCapture can be faked.
type capture interface {
	// Close stops recording and waits for the last frame to be published.
	Close()

	// Source is what the operating system calls the input this is recording
	// from.
	Source() string
}

// outputOptions is what the flags asked for.
type outputOptions struct {
	input   string // Sound input to open directly, empty to take the audio from a daemon
	format  string // Audio format to write, as --format gave it
	bitrate int    // Bits per second, for --format opus
	channel string // Which side of the cable the scanner is on, as --channel gave it
}

// meter reports how loud the audio is against where the noise floor sits, so a
// cable problem can be seen rather than guessed at.
type meter struct {
	// seen is how many frames have gone by since the last reading, and peak is
	// the loudest of them.
	seen int
	peak float64
}

// mismatch watches the radio against the sound card and notices when they
// disagree for long enough to mean the input is not the scanner.
//
// This check is only possible because the radio is required, and it is worth
// having because getting the input wrong is the single largest source of
// trouble in this corner of the hobby. Two failures look identical from the
// outside, produce nothing anybody wants, and give no clue which has happened:
//
//   - The radio is receiving and the audio stays at the noise floor, which is a
//     lead in the wrong socket, a lead that is not plugged in, or the scanner's
//     volume at zero.
//   - The audio carries steady sound while the radio says it is muted, which is
//     a microphone, and this is recording the room.
//
// Both are counted rather than reported the first time they happen, because the
// radio and the sound card are never going to agree to the millisecond and the
// edges of every transmission disagree briefly.
type mismatch struct {
	// silent counts frames where the radio was receiving and nothing came
	// through, and noisy counts frames where sound arrived with the radio
	// muted.
	silent, noisy int

	// since is when the current run of disagreement began, so a complaint is
	// made about something continuous rather than something that added up over
	// an evening.
	since time.Time

	// told stops the same advice being repeated for as long as the fault lasts.
	told bool
}

// recordOptions is what the flags asked for.
type recordOptions struct {
	destination string        // Where recordings are written
	input       string        // Sound input to open directly, empty to take the audio from a daemon
	channel     string        // Which side of the cable the scanner is on, as --channel gave it
	template    string        // How each recording is named, as --template gave it
	hang        time.Duration // Quiet time before a transmission is called finished
	minDuration time.Duration // Shortest recording worth keeping
	maxDuration time.Duration // Longest a recording may run before it is split
	normalize   bool          // Scale each recording up to just under full scale once it has ended, on unless --normalize=false
}

// recorder is the state one run of "audio record" carries: what it is writing
// into, the gate deciding where transmissions begin and end, the recording in
// progress, and what the scanner has said while it has been open.
//
// It is a type rather than a handful of variables threaded through the loop
// because the recording in progress is written by one path and closed by
// another, and passing a pointer to a pointer between them was the shape that
// made an abandoned file easy to leak.
type recorder struct {
	// app is where a finished recording is reported.
	app *appcontext.App

	// library is where recordings are filed.
	library *recordings.Library

	// gate decides where each transmission begins and ends.
	gate *audiogate.Gate

	// open is the recording being written, nil when there is none.
	open *recordings.Recording

	// seen is every reading of the radio taken while the open recording has
	// been running, and is what labels it when it ends.
	seen []device.Heard

	// volume reads the scanner's volume level, or reports -1 when it cannot be
	// read. It is nil when nothing arranged one.
	//
	// A function rather than a number, and read again for every warning rather
	// than once for the run. The first version of this cached it on the
	// reasoning that turning the volume down is what stops the warning quoting
	// it, so a stale reading could not mislead for long. That was wrong in the
	// case that matters: one notch is not enough to stop a mic input clipping,
	// so the warning fires again and repeats the level the person has just
	// changed. Telling somebody their volume is still 15 immediately after they
	// set it to 14 reads as the tool not seeing the radio at all, which is
	// worse than saying nothing.
	volume func() int

	// clipped and samples count the open recording's audio, for deciding
	// whether the input was overloaded while it was being written.
	clipped int
	samples int
}
