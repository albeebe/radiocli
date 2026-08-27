// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audiofeed

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/albeebe/radiocli/internal/audioin"
)

// The ways the stereo a sound card gives can be folded into the mono everything
// downstream wants.
//
// There has to be a choice because the scanner is mono and the cable decides
// where that mono signal lands. A stereo lead from the headphone socket carries
// it on both. A mono lead, or a record lead wired for one channel, carries it on
// one and leaves the other silent or shorted. Nothing in the audio can be
// inspected ahead of time to know which, and getting it wrong is not subtle:
// folding a one-sided signal halves its level, and taking the wrong side of one
// gives silence.
const (
	// ChannelAuto listens to both for a few seconds and decides.
	ChannelAuto = "auto"

	// ChannelLeft takes the left side and ignores the right.
	ChannelLeft = "left"

	// ChannelMix averages the two, which is right when the signal really is on
	// both and wrong by 6 dB when it is not.
	ChannelMix = "mix"

	// ChannelRight takes the right side and ignores the left.
	ChannelRight = "right"
)

// How long auto listens before it decides, and by how much one side has to win.
const (
	// cancelDB is how much level folding the two sides together may lose before
	// folding is treated as destroying the signal rather than combining it.
	//
	// Two sides carrying the same mono audio fold with no loss at all, so any
	// meaningful loss means they disagree. Four decibels is above anything a
	// slightly unbalanced pair produces and far below the eleven measured on a
	// scanner whose headphone output was set to invert one side.
	cancelDB = 4.0

	// cancelFrames is one second of audio that was above the floor, which is
	// all it takes to see that folding the two sides destroys them.
	//
	// Shorter than chooseFrames because it answers a much louder question. The
	// difference between the two sides is measured in decibels either way, but
	// cancellation is eleven of them rather than the handful that separates a
	// slightly unbalanced pair, so it does not need the same evidence.
	cancelFrames = 1000 / FrameMS

	// chooseFrames is three seconds of audio that was actually above the floor,
	// counted in frames rather than measured on a clock. Three seconds is about
	// one transmission, which is the smallest sample that says anything: half a
	// second is one syllable, and a syllable can be quiet on both sides.
	chooseFrames = 3 * 1000 / FrameMS

	// dominanceDB is how much quieter one side has to be to be called empty.
	//
	// Wide on purpose. Two sides carrying the same signal differ by a fraction
	// of a decibel, and a side that is genuinely unconnected is 40 dB down or
	// more. Twenty is the gap where nothing real lands, so neither a slightly
	// unbalanced stereo lead nor a noisy floor can be mistaken for one.
	dominanceDB = 20.0
)

// meterEvery is how many frames go by between level readings: about four a
// second, which is fast enough to look live and slow enough to read.
const meterEvery = 250 / FrameMS

// quietest is the level reported for a frame with nothing in it.
//
// A real answer would be negative infinity, which is correct and cannot be
// written as JSON. This is far below anything audible, so it reads as silence
// wherever it is shown and survives the trip to a listener.
const quietest = -120.0

// ringBytes is how much audio the ring between the sound card and the frame
// cutter holds.
//
// One second, near enough. That is far more than the cutter needs, which wakes
// every few milliseconds and takes what is there, and the surplus is deliberate:
// the alternative to a ring big enough to absorb one bad scheduling moment is a
// gap in the recording, and a megabyte of memory is a great deal cheaper than
// that. It is a power of two so that a position in it is found by masking.
const ringBytes = 1 << 21

// The shape of a frame, restated from the sound card's side of things.
//
// FrameMS and the rest come from audioin because that is the format the card is
// opened in, and repeating the numbers here rather than importing them would be
// two places to change one fact.
const (
	// FrameBytes is one frame of interleaved stereo as it arrives from the
	// card.
	FrameBytes = audioin.FrameBytes

	// FrameMS is 20 ms, the only frame length the encoder has.
	FrameMS = audioin.FrameMS

	// FrameSamples is one frame in sample pairs.
	FrameSamples = audioin.FrameSamples

	// MonoFrameBytes is one frame after the fold, which is what a listener gets
	// and what the encoder takes.
	MonoFrameBytes = FrameSamples * 2

	// SampleRate is 48 kHz, which the encoder requires and a line input
	// natively provides.
	SampleRate = audioin.SampleRate
)

// silenceFloor is the quietest a frame can be and still count as evidence of
// anything.
//
// In sample units rather than decibels because that is what the comparison is
// made in. It is about -60 dBFS, which is well below speech and well above the
// noise a line input makes with nothing transmitting.
//
// The floor exists because the channel this tool listens to is silent most of
// the time. Between transmissions both sides read as nothing, and a silent frame
// says exactly as much about which side the signal is on as no frame at all.
// Counting them would let thirty seconds of a quiet channel decide the question.
const silenceFloor = 32768.0 / 1000

// silentFor is how long a stream has to be perfectly silent before it is worth
// saying so.
//
// Perfectly, meaning every sample exactly zero, which is not what silence from a
// real input sounds like. An unplugged line input still has a noise floor a few
// counts wide. Digital zero from a device that opened successfully means
// something took the audio away, and on macOS that is almost always the
// microphone permission having been refused, which otherwise presents as a cable
// that does not work.
const silentFor = 5 * time.Second

// silentFrames is silentFor counted in frames.
const silentFrames = int(silentFor / (FrameMS * time.Millisecond))

// ReasonOutOfPhase is why the fold was refused, when it was refused because
// folding the two sides together cancelled them.
//
// It exists to be passed on to a person. The headphone jack on an SDS100 and an
// SDS150 is wired out of phase, which Uniden addressed in firmware by adding a
// menu to invert one side rather than by changing the wiring, so a radio has it
// either way depending on whether its owner has ever found that menu. Taking a
// single side works around it, and saying so lets them fix it at the source.
const ReasonOutOfPhase = "out-of-phase"

// Channels is every channel mode, in the order a listing should show them.
var Channels = []string{ChannelAuto, ChannelLeft, ChannelRight, ChannelMix}

// openInput opens the audio input. It is a var so tests can substitute a fake.
var openInput = func(name string, onFrames func(pcm []byte)) (audioInput, error) {
	return audioin.Open(name, onFrames)
}

// wakeEvery is how long the frame cutter waits before looking anyway.
//
// The sound card wakes it as audio arrives, so this is not how frames are
// normally cut. It is what lets the cutter notice that the card has stopped
// calling at all, which is what an unplugged interface looks like, and it is
// slow because that is the only thing it is for.
//
// It is a var so tests can shorten it.
var wakeEvery = 250 * time.Millisecond

// audioInput is the open sound input a capture reads from.
//
// It is declared here, rather than the concrete type audioin.Open returns being
// used directly, so that tests can substitute a fake for the one call in this
// package that touches a sound card. audioin.Capture satisfies it.
type audioInput interface {
	// Close stops the device and releases it.
	Close()

	// Name is what the operating system calls the input.
	Name() string
}

// Publisher is what a capture hands finished frames to.
//
// It exists so a gate can be put between the two later without either end
// changing: a gate takes every frame, keeps a pre-roll of them, decides which go
// out, and hands those on through this same interface. Feed is the only thing
// that implements it today.
type Publisher interface {
	// Publish offers one frame to whoever is listening. It must not block: it
	// is called from the one goroutine that cuts frames, and anything that
	// holds it up stops the next frame being cut.
	Publish(Frame)

	// PublishEvent offers one piece of news.
	PublishEvent(kind string, payload any)
}

// Capture is one open sound card, cut into frames and handed to a Publisher
// until it is closed.
type Capture struct {
	// in is the open sound device, whose callback writes into ring.
	in audioInput

	// ring carries audio from the device's thread to the frame cutter.
	ring *ring

	// pump cuts the ring's audio into frames and publishes them.
	pump *pump

	// log is where trouble is reported, which for a capture is the sound card
	// misbehaving.
	log *slog.Logger

	// stop ends the cutting goroutine when it is closed.
	stop chan struct{}

	// closed makes the shutdown happen once, however many callers arrive and
	// whether or not they arrive together.
	closed sync.Once

	// wake nudges the cutter when the device delivers audio.
	wake chan struct{}

	// finished is what Close waits on until the cutter has stopped.
	finished sync.WaitGroup

	// mu guards channel.
	mu sync.Mutex

	// channel is the fold in use, updated when auto settles.
	channel string
}

// chooser works out which side of the cable the scanner is on.
//
// It is fed every frame and answers, every time, which mode to fold with right
// now. Before it has decided that answer is mix, and audio flows from the very
// first frame: waiting for certainty would mean seconds of silence at the moment
// somebody pressed Listen, and mix is at worst 6 dB quiet.
//
// Once it decides it never changes its mind, which is the whole reason it is a
// type rather than a function. A quiet transmission is not evidence that a side
// went away, and a fold that flipped in the middle of speech would be audible
// and would keep being audible.
type chooser struct {
	// settled is the answer, empty until there is one. For every mode but auto
	// it is set before the first frame arrives and nothing here ever runs.
	settled string

	// Sums of squares rather than levels, so they can be compared as ratios
	// without any being converted first. sumM is what folding the two together
	// would have produced, which is the only thing that says whether folding
	// them is safe.
	sumL, sumR, sumM float64

	// qualified counts the frames loud enough to mean anything, which are the
	// only ones that settle the question.
	qualified int

	// why is why the answer is what it is, when that is worth telling
	// somebody. Empty for an unremarkable decision.
	why string
}

// Event is something worth saying about the audio that is not audio.
//
// It exists ahead of anything that needs it. The wire carries these already, so
// that the day a gate is added it has somewhere to announce a transmission
// starting and ending without the framing, the daemon or anything listening
// changing at all. Shipping it carrying a level meter is what keeps that
// road tested rather than merely built.
type Event struct {
	// Kind is what happened, and becomes the "type" of the JSON that carries
	// it: "level" today, "tx_start" and "tx_end" when there is a gate.
	Kind string

	// Payload is marshalled by whatever puts this on a wire. It is left as any
	// so this package needs to know nothing about how it travels.
	Payload any
}

// Feed hands each frame to everybody subscribed.
type Feed struct {
	// log is where trouble is reported, which for a feed means a listener that
	// has fallen far enough behind to start losing frames.
	log *slog.Logger

	// mu guards subs, and is held for the whole of a publish.
	mu sync.Mutex

	// subs is every current listener.
	subs map[*Sub]struct{}
}

// Frame is 20 ms of the sound input, folded to mono and ready to send.
type Frame struct {
	// Seq counts 20 ms frames since the capture started.
	//
	// It is the capture's own clock rather than a count of frames delivered,
	// which is the point of it. Every way audio can go missing between here and
	// somebody's speakers leaves a gap in this number: a listener too slow to
	// keep up, a front end dropping frames to catch up, and one day a gate that
	// decided there was nothing worth sending. All of them look the same at the
	// far end, and all of them are told apart from a stream that is merely
	// quiet.
	Seq uint32

	// PCM is exactly MonoFrameBytes of signed 16-bit little-endian mono.
	//
	// Freshly allocated for each frame rather than taken from a pool. A pool
	// would need every listener to say when it had finished with a frame, which
	// is a lifetime to get wrong for the sake of 96 KB a second. The encoder
	// each listener runs allocates several times that per frame anyway.
	PCM []byte

	// Level is how loud this frame is, in dBFS. Nothing reads it yet. It is
	// measured because a gate will, and because measuring it costs one pass
	// over samples that have just been touched anyway.
	Level float64

	// At is when the frame was cut, which is not when it was recorded. The
	// difference is however long the audio sat in the ring, normally under a
	// millisecond.
	At time.Time
}

// Options say what to open and how to fold it.
type Options struct {
	// Source is the name of the sound input, as Sources reports it.
	Source string

	// Channel is one of the channel modes. Empty means ChannelAuto.
	Channel string

	// Log is where trouble is reported. It may be nil.
	//
	// Whatever is passed here is kept, rather than being looked up when it is
	// needed. Inside the daemon the logger is swapped for a client's own stream
	// while a command runs, so a goroutine that fetched it each time would write
	// this program's diagnostics into somebody's command output.
	Log *slog.Logger
}

// pump cuts what the sound card produced into frames and hands them on.
//
// It is separate from Capture, and knows nothing about sound libraries, so the
// whole of the framing, the fold to mono and the channel choice can be driven by
// writing into a ring in a test.
type pump struct {
	// ring is where the sound card's audio waits to be cut.
	ring *ring

	// out is where finished frames and events go.
	out Publisher

	// pick decides which side of the stereo carries the scanner.
	pick *chooser

	// cursor is how far into the stream this has read, in bytes. Frames are cut
	// on multiples of FrameBytes, so it stays aligned to one.
	cursor uint64

	// stereo and mono are the working buffers for one frame, reused so the
	// cutting loop allocates nothing but the frames themselves.
	stereo []byte
	mono   []byte

	// Whether the channel choice has been announced, so that it is announced
	// once rather than on the frame after every frame.
	told bool

	// The level meter, which goes out every meterEvery frames rather than every
	// frame. Fifty numbers a second is far more than a meter can show and far
	// more than anybody can read.
	meterAt   int
	meterPeak float64

	// Digital silence, tracked in frames so it needs no clock.
	silent int

	// toldSilent stops the advice about a silent stream being repeated every
	// few seconds for as long as it stays silent.
	toldSilent bool
}

// ring is a circular buffer written by the sound card's thread and read by the
// one goroutine that cuts frames.
//
// One writer and one reader, so it needs no lock, only an ordered publication of
// how far the writer has got. The writer never waits for the reader and never
// refuses to write: it overwrites the oldest audio instead. That is the right
// failure for a live stream, and it is what makes the writer safe to call from a
// thread that must not block.
type ring struct {
	// buf holds the audio; its length is always a power of two.
	buf []byte

	// mask turns a stream position into an index in buf.
	mask uint64

	// written is every byte the sound card has ever produced, which makes it a
	// clock as well as a position. It is never reset, so the frame number
	// worked out from it stays true across a reader that fell behind.
	written atomic.Uint64
}

// Sub is one listener's view of the feed.
//
// Frames arrive on a channel of its own rather than through a callback, so that
// a listener too slow to keep up falls behind on its own and not on everybody
// else's behalf.
type Sub struct {
	// feed is the feed this subscription belongs to.
	feed *Feed

	// frames is where this listener's audio is queued.
	frames chan Frame

	// events is where this listener's news is queued.
	events chan Event

	// dropped counts frames this listener was too slow to take.
	dropped atomic.Uint64

	// closed makes Close safe to call more than once.
	closed sync.Once
}
