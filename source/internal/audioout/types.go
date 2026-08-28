// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audioout

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// The format every sound output is opened in.
//
// Asked of the library rather than negotiated with the hardware, the same way
// audioin asks. Whatever the speakers natively want, miniaudio converts into it
// on the way out, so a caller hands over the audio it already has and nothing
// in this tool has to learn what a particular set of speakers prefers.
//
// Mono, which is where this differs from audioin. A capture is opened in stereo
// because which side of a cable the scanner landed on is not knowable in
// advance, so both sides are taken and something further along decides. By the
// time audio reaches here that decision has been made and there is one channel
// left, so asking for one channel is asking for exactly what there is. The
// library copies it to both speakers.
const (
	// Channels is 1. See above: the fold to mono has already happened by the
	// time anything reaches this package.
	Channels = 1

	// FrameBytes is one frame: FrameSamples of signed 16-bit little-endian
	// mono. It is the size of the slices this package is handed.
	FrameBytes = FrameSamples * Channels * 2

	// FrameMS is the length of the frame the rest of the tool cuts audio into,
	// fixed by the Opus encoder having exactly one frame size.
	FrameMS = 20

	// FrameSamples is one frame in samples.
	FrameSamples = SampleRate * FrameMS / 1000

	// SampleRate is 48 kHz, because that is what everything upstream produces
	// and resampling audio that is already at the rate the hardware runs at
	// would be work done for nothing.
	SampleRate = 48000
)

// How much audio may be waiting in front of the speakers, and how much has to
// be waiting before they start.
const (
	// bufferFrames is how much the ring holds, at 20 ms each.
	//
	// One second, and it costs nothing to be that big. It is a ceiling rather
	// than a target: what actually stands in the ring is the cushion Open was
	// asked for, because the reader takes out exactly what the writer puts in,
	// so a deep ring adds no delay at all. It matters when the writer stops
	// for a while and then catches up, which the recorder does every time it
	// finishes a file: normalizing a whole WAV and filing a description beside
	// it happen between two frames. At 240 ms a 300 ms stall lost a quarter of
	// a second of audio outright. At one second a 600 ms stall loses nothing.
	// It costs 96 KB.
	bufferFrames = 50

	// DefaultBuffer is the cushion Open builds when its caller has no opinion:
	// three frames.
	//
	// Small, because everything played comes out this far behind the radio and
	// somebody sitting next to a scanner hears lateness as the tool being
	// broken. It was not always: it stood at a quarter of a second while the
	// audio went out through miniaudio, where a cushion this size broke every
	// one of 120 test tones into as many as six pieces, with runs of digital
	// silence spliced into them. None of that showed on this side of the
	// library, because the ring never ran dry and nothing was ever dropped: the
	// audio was always ready and the device was not always there to take it.
	//
	// oto does not do that. Measured the same way, three frames plays clean and
	// puts the speakers about 40 ms behind the radio, which is close enough to
	// live that it reads as the scanner's own speaker rather than as a delay.
	//
	// A default rather than a constant of the package, because that trade,
	// lateness against robustness, belongs to the person listening. Both
	// commands that play expose it as --buffer, and raising it is the first
	// thing to try if a busy machine breaks up.
	DefaultBuffer = 3 * FrameMS * time.Millisecond
)

// The range a buffer may be asked for in.
//
// Both ends are the ring's, not taste. Below two frames the cushion cannot
// absorb a producer that is one frame late, which is the cushion failing at
// the only job it has. Above half the ring there is no room left to catch the
// producer back up after a stall, which is the other job.
const (
	// maxBuffer is half the ring.
	maxBuffer = bufferFrames / 2 * FrameMS * time.Millisecond

	// minBuffer is two frames.
	minBuffer = 2 * FrameMS * time.Millisecond
)

// fadeBytes is how much audio the edges of a burst are ramped over.
//
// 5 ms, and it is there to stop the speakers clicking. Audio does not begin and
// end at zero: a squelch that opens mid-waveform steps from silence to whatever
// sample it landed on, and one that closes steps back, and a step is a click.
// A scanner listening to dispatch traffic does that several times in a
// conversation, once at each end of every over, and measured against real
// traffic that was six clicks in twenty seconds.
//
// Short enough that nothing audible is lost from either end of the speech, long
// enough to turn the step into a slope.
const fadeBytes = SampleRate * 2 * 5 / 1000

// Why a name can fail to name one sound output.
//
// The same two failures audioin has, and separate for the same reason: nothing
// found is usually a typo and the fix is to look at the list, while more than
// one found is two identical devices and there is nothing in the list to look
// at, because both lines of it say the same thing.
var (
	// ErrAmbiguousSink says more than one thing attached is called that.
	ErrAmbiguousSink = errors.New("more than one speaker by that name")

	// ErrNoSink says nothing attached is called that.
	ErrNoSink = errors.New("no speaker by that name")
)

// Player is one open sound output, playing whatever is handed to it until it is
// closed.
//
// It is two halves that never meet: Play runs on whatever goroutine is holding
// the audio, and the sound card asks for that audio on a thread belonging to
// the library. The ring is what stands between them, and it is the only thing
// they share.
type Player struct {
	closed sync.Once // Guarantees the device is only torn down once
	out    output    // The running playback device, held to close it
	ring   *ring     // What stands between the audio arriving and the card asking
}

// Sink is one place this computer can play sound: speakers, headphones, or a
// virtual device some other application presents.
//
// It is a struct around one field rather than a bare string for the reason
// audioin.Source is: a listing is about to want to say more than the name, and
// something reading this as JSON should not have to be rewritten when it does.
type Sink struct {
	// Name is what the operating system calls the output, such as
	// "MacBook Pro Speakers". It is what a listing shows and what somebody
	// types to choose one.
	//
	// It is not unique. Two identical USB interfaces report the same name.
	Name string `json:"name"`
}

// Stats is what the ring has to say about how the playing went.
//
// Every number counts something the listener may have heard, and none is
// necessarily a fault. See Player.Stats.
type Stats struct {
	// Dropped is how many bytes of audio were thrown away because they arrived
	// faster than the speakers took them.
	Dropped uint64 `json:"dropped"`

	// Played is how many bytes of audio the speakers actually took, which is
	// not the same as how many were handed over: it excludes anything dropped,
	// and anything still waiting when the device was closed.
	//
	// It is here because silence has two causes and the other two numbers
	// cannot tell them apart. A squelched run that plays nothing all evening
	// and a squelched run that is working perfectly on a quiet channel both
	// report nothing dropped and nothing starved.
	Played uint64 `json:"played"`

	// Starved is how many times the ring ran dry and silence was played
	// instead.
	Starved uint64 `json:"starved"`

	// Waiting is how many bytes are in the ring at this moment, which is the
	// delay somebody hears: everything plays this far behind the radio.
	//
	// It is the reading the other three cannot give. A sound input and a set of
	// speakers keep their own clocks and never agree exactly, so the audio
	// arrives a little faster or a little slower than it leaves, and this
	// creeps in one direction for as long as a run lasts. Creeping down ends in
	// a starve, which is heard, and creeping up ends against the far wall of
	// the ring, where every arrival evicts the oldest audio and is heard as
	// well. Both take minutes to arrive and neither shows in a count that is
	// still zero, so the depth is what says a run is drifting while there is
	// still time to say it.
	Waiting int `json:"waiting"`
}

// output is the part of an open playback device this package uses, which is
// only that it can be closed.
//
// An interface rather than the concrete type, because there are two of those.
// One is built on the audio library and only exists in a build with cgo; the
// other is the stand-in that lets the rest of the tool compile without it.
type output interface {
	// Close stops the device and gives back everything the library allocated.
	Close()
}

// ring is the jitter buffer: a fixed run of bytes that Play fills and the audio
// thread empties.
//
// A mutex, in something an audio callback touches, needs justifying. The rule
// the callback lives under is that it must not block, and this cannot make it
// block for meaningfully longer than a copy: the only other holder is Play,
// which does no allocation, no system call and no waiting while it holds the
// lock. That rule is why the gain lives in an atomic and the scaling happens
// into scratch before the lock is taken: multiplying a thousand samples while
// the audio thread stands waiting would be exactly the wait the rule forbids.
// A lock-free ring would remove even the copy, at the cost of being the kind
// of code that is wrong for a year before anybody notices.
type ring struct {
	mu     sync.Mutex // Held for a copy and nothing else, by Play and by the audio thread
	buf    []byte     // Fixed capacity, allocated once at Open and never grown
	start  int        // Where the oldest byte sits in buf
	length int        // How many bytes are waiting
	primed bool       // Whether enough has arrived to start playing, see prime
	prime  int        // How many bytes must be waiting before playing starts, fixed at Open
	last   int16      // The final sample most recently played, to ramp from when the ring runs dry
	stats  Stats      // What has been dropped and how often it has run dry

	// gain is the float64 bits of what every sample is multiplied by on the
	// way in, 1 for none. Atomic rather than under mu so that write can read
	// it, and SetGain change it, without ever touching the lock the audio
	// thread waits on. See gainNow.
	gain atomic.Uint64

	// scratchMu guards scratch and nothing else. The audio thread never takes
	// it, so however long a scaling holds it, the speakers are not waiting.
	scratchMu sync.Mutex

	// scratch is where samples are scaled ahead of the lock, kept so that a
	// steady stream of frames costs one allocation rather than fifty a second.
	scratch []byte
}
