// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package audiogate

import (
	"time"

	"github.com/albeebe/radiocli/internal/audiofeed"
)

// How the package reports what happened, as the Kind of an Event.
const (
	// KindAudio carries one frame belonging to the transmission that is open.
	KindAudio Kind = "audio"

	// KindEnd says the transmission is over and describes it. Every KindStart
	// is followed by exactly one of these.
	KindEnd Kind = "end"

	// KindStart says a transmission has begun and is worth keeping. It arrives
	// only once the transmission has outlived MinDuration, so a caller can
	// create a file on it without ever having to delete one afterwards.
	KindStart Kind = "start"
)

// Why a transmission ended, carried on the Transmission an Event describes.
const (
	// ReasonChannel means the radio moved to a different channel. The audio
	// alone showed one transmission and the radio showed two, and the radio is
	// right, so the recording is cut here and the next one opens immediately.
	ReasonChannel = "channel"

	// ReasonHang means the audio went quiet and stayed quiet for Hang with the
	// radio no longer receiving. This is the ordinary ending.
	ReasonHang = "hang"

	// ReasonSplit means MaxDuration was reached. The audio has not stopped, so
	// the next transmission opens immediately and carries on.
	ReasonSplit = "split"

	// ReasonStopped means Flush was called, which is the recorder shutting
	// down part way through a transmission.
	ReasonStopped = "stopped"
)

// What the detector is tuned to, none of which is a setting.
//
// These are deliberately constants rather than options. A caller cannot judge
// any of them by eye, a wrong value is worse than the default in every case,
// and exposing the last one would recreate the fixed audio threshold that
// causes most of the trouble in the software this feature is measured against.
// The call model in Options is where a person genuinely has a preference.
const (
	// bufferWindow is how far back the ring lets the detector look for the
	// start of a transmission.
	//
	// It is sized against what it has to absorb, which is every source of lag
	// between the audio arriving and anything knowing a transmission is under
	// way: the interval the radio is polled at, a reply that took its time, and
	// a scanner that reported activity a moment after it began. Those are
	// hundreds of milliseconds each. Ten seconds is more than an order of
	// magnitude of headroom over the worst of them, and it costs about 960 KB.
	bufferWindow = 10 * time.Second

	// floorMax is as high as the noise floor is allowed to be judged.
	//
	// Above this the input is not a squelched scanner. It is a microphone in a
	// room, or a badly wired cable carrying mains hum, and letting the floor
	// follow it would mean a detector that never triggers on anything. Pinning
	// it means such an input triggers constantly instead, which is the failure
	// that gets noticed and reported rather than the one that looks like a
	// quiet night.
	floorMax = -30.0

	// floorMin is as low as the noise floor is allowed to be judged.
	//
	// A line input with nothing transmitting still has a floor a few counts
	// wide, so a reading below this is not a quiet cable but digital zero,
	// which means something took the audio away. On macOS that is almost
	// always the microphone permission having been refused. Chasing it down
	// would set the trigger level below the noise the sound card makes when it
	// comes back.
	floorMin = -90.0

	// speechMarginDB is how far above the floor a transmission's loudest moment
	// has to reach before it is worth a file at all.
	//
	// marginDB is deliberately close to the floor, because finding the exact
	// moment audio began means catching it while it is still faint. That makes
	// it useless for the other question: a transmission the radio reported into
	// a quiet cable clears eight decibels on the floor's own wobble and gets
	// written as a recording of nothing.
	//
	// Loudest moment rather than any moment, because the two questions are
	// different. Measured across eighteen consecutive recordings on an SDS150
	// through a line input, every one carrying speech peaked between 48 and 71
	// decibels above its floor, and the one holding nothing but noise peaked at
	// 25. Thirty-five sits in the middle of a gap twenty-three decibels wide.
	speechMarginDB = 35.0

	// floorTrusted is how quiet the floor estimate has to be before it is
	// allowed to reject a transmission.
	//
	// The estimate is a low percentile of the last fifteen seconds, which is a
	// noise floor only when most of that window was quiet. A busy channel, or a
	// recorder started in the middle of a transmission, fills the window with
	// speech and the estimate rises to meet it. Rejecting on that reading would
	// throw away the real traffic that produced it, which is the opposite of
	// what the test is for.
	//
	// So a high reading disables the test rather than triggering it. Measured
	// on an SDS150 through a line input, a genuine floor sat between -62 and
	// -80 dBFS across eighteen recordings, so anything above -45 is the window
	// being full of audio rather than a quiet cable.
	floorTrusted = -45.0

	// tailDropDB is how far below a transmission's loudest moment a frame can
	// be and still count as part of it.
	//
	// The end of a recording is the last frame that was still speech, and
	// marginDB cannot answer that either. On a clean line input the floor sits
	// near -80 dBFS, so eight decibels above it is -72, and hiss at -70 goes on
	// advancing the last-heard time long after anybody stopped talking. One
	// recording ran seven seconds for one second of speech that way.
	//
	// Measured against the transmission's own peak rather than the floor,
	// because that is the level the speech in this recording actually has, and
	// it needs no estimate to be right. Forty decibels is far below any
	// syllable and far above the noise the same recording is sitting on.
	tailDropDB = 40.0

	// marginDB is how far above the tracked floor a frame has to be to count
	// as signal.
	//
	// Wide enough that the floor wandering by a decibel cannot trigger it, and
	// far below the twenty or more decibels that separate speech from the floor
	// on any working setup. It works without adjustment precisely because the
	// floor underneath it moves, which is the whole reason the floor is
	// tracked rather than configured.
	marginDB = 8.0

	// maxLookBack is how far before the radio's word the onset may be searched
	// for.
	//
	// The search walks back over audio already above the floor, and the only
	// thing it compensates for is the lag between a transmission starting and
	// the radio being asked about it, which is a fraction of a second. Two
	// seconds is generous for that. Leaving it unbounded means a floor measured
	// a little low lets the walk run back through the whole buffer and put all
	// of it at the front of the recording.
	maxLookBack = 2 * time.Second

	// radioSlack is how long after the radio was last seen receiving its audio
	// still counts as part of the transmission.
	//
	// It covers the gap between polls, since the radio is asked repeatedly
	// rather than continuously, so the moment it stopped is only known to
	// within one interval. Without it a recording loses up to that much off its
	// end every time.
	//
	// It has to stay small next to the minimum length, and that is what sets
	// it rather than taste. Everything it lets through is audio nothing has
	// confirmed, so a scanner whose idle noise sits above the floor spends this
	// long adding itself to the transmission it just finished. At four hundred
	// milliseconds against a minimum of five hundred, a two hundred millisecond
	// blip reached the minimum on noise alone and was kept, which is the
	// recording-the-hiss failure arriving by a new route. Two hundred leaves
	// the blip short, and is still two polls at the rate the radio is now
	// asked.
	radioSlack = 200 * time.Millisecond

	// padDuration is how much audio is kept before the detected onset.
	//
	// The detector finds the last frame that was still at the floor and starts
	// after it, which is right to within one frame. This is the margin for that
	// one frame, plus the fact that a syllable rises out of the noise rather
	// than appearing, so the first genuinely audible frame is preceded by a
	// little that is quieter but not nothing.
	padDuration = 200 * time.Millisecond
)

// floorWindow is how far back the noise floor is measured over.
//
// The floor is a low percentile of what was heard in this much audio, which is
// what makes it immune to the transmissions happening in front of it. Speech is
// loud, and a low percentile ignores loud, so the estimate is not dragged
// upwards by the very audio it is there to detect.
//
// Fifteen seconds is a compromise between two failures. Shorter, and a long
// transmission can fill the whole window, leaving the floor to be estimated
// from speech. Longer, and a genuine change to the input, somebody turning the
// scanner up or a ground loop starting, takes proportionally longer to be
// believed. It also sets how long the detector takes to settle when the tool is
// started in the middle of a transmission, which is the one case where it is
// felt directly.
const floorWindow = 15 * time.Second

// floorPercentile is how far up the recent levels the floor is taken from.
//
// Not the minimum, which was the first thing tried and was wrong. Measured on a
// USB line input with a squelched scanner on it, the noise sat at -78 dBFS with
// a spread of about three decibels, and the quietest single frame in fifteen
// seconds was -87. A floor taken from that one frame sits nine decibels below
// where the noise actually is, which puts the trigger, eight decibels above it,
// at -79: a decibel under the noise itself. So the noise reads as signal and the
// recorder writes the hiss between transmissions as though it were traffic. That is exactly the failure this is all here to avoid, and it
// came from estimating a distribution by its most extreme sample.
//
// A tenth is low enough to sit inside the quiet part of a window that also
// holds a transmission, and high enough to describe where the noise really is
// rather than how far it once dipped.
const floorPercentile = 10

// floorBins is how many one decibel buckets the recent levels are counted in.
//
// One per decibel between floorMin and floorMax, which is finer than the
// estimate needs to be: the margin above it is eight decibels, so a floor
// wrong by less than one changes nothing.
const floorBins = int(floorMax-floorMin) + 1

// frameDuration is how much audio one frame carries, taken from the feed so
// that nothing here has to be kept in step with it by hand.
const frameDuration = audiofeed.FrameMS * time.Millisecond

// Frame counts, worked out from the durations above.
const (
	// floorFrames is floorWindow counted in frames.
	floorFrames = int(floorWindow / frameDuration)

	// lookBackFrames is maxLookBack counted in frames.
	lookBackFrames = int(maxLookBack / frameDuration)

	// maxRingFrames is bufferWindow counted in frames.
	maxRingFrames = int(bufferWindow / frameDuration)

	// padFrames is padDuration counted in frames.
	padFrames = int(padDuration / frameDuration)
)

// The defaults New fills in for an Options left at zero.
//
// They are exported so that a command declaring flags for them can show the
// real value in its help rather than a zero standing in for "whatever this
// package decides". A default nobody can see is one a reader has to guess at.
const (
	// DefaultHang is how long the transmission has to be gone before it is
	// called finished.
	//
	// Two seconds at first, sized to carry a speaker drawing breath
	// mid-sentence. That was the right number for the wrong question. A breath
	// is a pause in the audio, and with RequireRadio set the audio has no vote:
	// what ends a transmission is the radio's mute closing, and a mute closes
	// when the carrier drops rather than when a speaker stops talking. Somebody
	// pausing mid-sentence is still keyed up, so nothing here can split them.
	//
	// What this actually has to survive is the carrier itself flickering, a
	// mobile unit going behind a hill for a moment, which is a far shorter
	// event. What it has to be shorter than is the gap between one person
	// unkeying and the next answering. Measured on an SDS150, a dispatcher and
	// a cruiser left 640 milliseconds of closed mute between them. Half a
	// second sits under that and well over any dropout worth bridging.
	DefaultHang = 500 * time.Millisecond

	// DefaultQuietHang is DefaultHang for a gate with no radio to ask.
	//
	// Without RequireRadio there is no carrier to follow and the audio going
	// quiet is the only ending there is, so the breath mid-sentence is back and
	// the number has to clear it. Two seconds does. This is a worse way to find
	// the end of a transmission and the difference is not a preference: one
	// number is measuring the transmitter and the other is guessing from the
	// speaker, and they cannot share a value because they are not measuring the
	// same thing.
	DefaultQuietHang = 2 * time.Second

	// DefaultMaxDuration is when a transmission is split rather than allowed
	// to grow without bound. Five minutes is far longer than any voice
	// transmission and short enough that a stuck microphone does not produce
	// one enormous file.
	DefaultMaxDuration = 5 * time.Minute

	// DefaultMinDuration is the shortest recording worth keeping. Below this a
	// transmission is a click, a squelch tail or a control channel burst rather
	// than anything anybody wants to listen to.
	//
	// A second while an exchange was one recording, because the short half of
	// it rode along inside the long half and nothing was lost by setting the
	// floor above it. Cutting on the keyup takes that away: "ten four" is its
	// own recording now, it runs well under a second, and a floor of a second
	// does not merely fail to separate it but deletes it. Half a second keeps
	// the acknowledgements and still discards the bursts, which are shorter
	// again by an order of magnitude.
	DefaultMinDuration = 500 * time.Millisecond
)

// Activity is what the radio says it is doing, as of the moment it was asked.
//
// It is offered to the gate whenever the radio is sampled, and the gate uses it
// for two things: as a trigger that can open a transmission the audio has not
// yet made obvious, and as an identity that splits one recording into two when
// it changes.
type Activity struct {
	// On reports whether the radio is receiving.
	On bool

	// Key identifies what it is receiving, and is opaque here.
	//
	// The caller builds it out of whatever distinguishes one channel from
	// another, which is a protocol question. This package only ever compares
	// two of them for equality, so nothing about the scanner's fields, their
	// order or their spelling reaches in here.
	Key string
}

// Event is one thing the gate has to say, returned from Offer and Flush.
type Event struct {
	// Kind says which of the three this is, and therefore which of the fields
	// below carries anything.
	Kind Kind

	// Frame is the audio, on a KindAudio event.
	Frame audiofeed.Frame

	// Tx describes the transmission, on KindStart and KindEnd. On a KindStart
	// its End and Reason are not yet known and are left zero.
	Tx Transmission
}

// Kind is which sort of Event this is.
type Kind string

// Options is the call model: what counts as one transmission rather than two,
// and how long a recording is allowed to be.
//
// Every field may be left zero, in which case New fills in a default. These are
// the only things about the gate a caller chooses, because they are the only
// things a person has an opinion about.
type Options struct {
	// Hang is how long the transmission must be gone before it is called
	// finished, and what "gone" means depends on RequireRadio.
	//
	// With a radio to ask it is the radio's mute staying shut, and because a
	// mute follows the carrier this is what separates one speaker from the
	// next: it does not shut for a pause in speech, so a pause cannot split a
	// recording however long the hang. Without a radio it is the audio staying
	// quiet, and then a pause is all there is to go on and the hang has to be
	// long enough to carry one. The two want very different values. See
	// DefaultHang and DefaultQuietHang.
	Hang time.Duration

	// MaxDuration is how long one recording may run before it is split.
	MaxDuration time.Duration

	// MinDuration is the shortest transmission worth keeping. Anything shorter
	// is dropped before a KindStart is ever emitted, so a caller never creates
	// a file it then has to delete.
	MinDuration time.Duration

	// RequireRadio makes the radio the authority on whether a transmission is
	// happening at all, rather than one of two things that can decide it.
	//
	// Set it whenever there is a radio to ask, which for the recorder is
	// always. Without it the audio alone can open a transmission and keep it
	// open, and that turns out not to survive contact with real hardware: a
	// scanner's line output has more than one idle level, measured at -88 dBFS
	// with the squelch shut and -77 at other times on one SDS150, and any fixed
	// margin above the quieter of those reads the louder one as speech. A
	// sixteen second recording of nothing but noise, from a transmission that
	// actually lasted under a second, is what that looks like.
	//
	// The radio has no such ambiguity. It says plainly whether its audio gate
	// is open, so it decides whether a recording exists and roughly when, and
	// the audio is left to do what it is genuinely better at: finding the exact
	// moment inside that window where the sound began and ended.
	RequireRadio bool
}

// Transmission is one recording, as the gate understands it.
type Transmission struct {
	// Start and End are when the audio actually began and ended, found in the
	// buffer rather than taken from when anything was noticed.
	Start time.Time
	End   time.Time

	// Key is the radio's identity for what was received, empty if the radio
	// never said while this was open.
	Key string

	// Reason is why it ended: ReasonHang, ReasonSplit, ReasonChannel or
	// ReasonStopped. It is empty on a KindStart.
	Reason string

	// Dropped counts frames the sound card produced that never arrived, found
	// as gaps in the feed's frame numbering.
	//
	// It is reported rather than repaired. Audio can go missing between the
	// card and here, and a recording containing a gap that nothing mentions is
	// worse than one that admits it, because the missing time is invisible
	// once the samples are concatenated.
	Dropped int
}

// noiseFloor is a low percentile of the levels seen in the last floorWindow of
// audio.
//
// A percentile rather than an average, because an average is pulled up by every
// transmission until the level needed to trigger climbs past anything the radio
// produces. A percentile rather than the minimum, because the minimum is one
// sample of a distribution and lands well below where the noise actually sits;
// see floorPercentile for the measurement that settled it.
//
// It is counted in one decibel buckets over a ring of the recent frames, so
// adding a frame is two array updates and reading the floor is a walk of sixty
// buckets. That is cheap enough to do on every frame and needs no sorting, no
// allocation after the first frame, and no structure whose cost depends on what
// the audio is doing.
type noiseFloor struct {
	// counts is how many of the frames in the window fell in each bucket,
	// quietest first.
	counts [floorBins]int

	// recent is the bucket each frame in the window fell in, oldest first, so
	// a frame leaving the window can be taken back out of counts.
	recent []uint8

	// at is where the next frame goes in recent, and n is how many of it is in
	// use, which is less than the whole only while the window is filling.
	at, n int
}

// Gate turns a stream of audio frames into transmissions.
//
// It holds the last bufferWindow of audio and decides nothing in real time,
// which is the point of it. When something says a transmission is under way,
// however late that news arrives, the audio from before it is still here to be
// looked at, so the recording can begin where the sound did rather than where
// the news did.
//
// It is not safe for concurrent use. Offer and Activity are called from the one
// goroutine reading the feed.
type Gate struct {
	// opts is the call model, with defaults already filled in.
	opts Options

	// ring holds recent audio while nothing is being recorded, oldest first,
	// so the start of a transmission can be found after the fact.
	ring []audiofeed.Frame

	// floor is the running estimate of the noise floor, in dBFS.
	floor noiseFloor

	// radio is the last thing the radio said, and radioAt is when it said it.
	radio   Activity
	radioAt time.Time

	// tx is the transmission being assembled, nil when nothing is open.
	tx *transmission

	// seq is the frame number last seen, for spotting gaps, and seqSet says
	// whether there has been a first frame to compare against.
	seq    uint32
	seqSet bool
}

// transmission is the state of the one recording currently being assembled.
type transmission struct {
	// info is what will be reported about it.
	info Transmission

	// pending holds frames that have not been handed out yet.
	//
	// Before the transmission is confirmed that is all of them, because
	// nothing should be written for something that turns out to be too short.
	// Afterwards it is the last Hang worth, held back so that trailing quiet
	// can be trimmed off the end instead of being written and regretted.
	pending []audiofeed.Frame

	// confirmed says the transmission has outlived MinDuration, and therefore
	// that a KindStart has been emitted for it.
	confirmed bool

	// firstLoud is when audio was first above the floor, and is zero until it
	// has been, which is the state of a transmission the radio opened while
	// the cable was delivering nothing.
	//
	// It gates confirmation, so a scanner reporting activity into a lead in
	// the wrong socket produces no recordings at all rather than an evening of
	// silent files. With lastLoud it also measures how much audio the
	// transmission actually holds, which is what MinDuration is judged
	// against: the time spent waiting through the hang at the end is not part
	// of what was heard, and counting it would keep blips that a caller asked
	// to be rid of.
	firstLoud time.Time

	// lastLoud is when audio was last loud enough to be speech, which is where
	// the recording is trimmed back to when it ends.
	//
	// Judged against peak rather than against the floor. What ends a recording
	// is somebody stopping talking, and on a quiet input the floor is so far
	// down that hiss clears it and holds the recording open for seconds after
	// the voice has gone.
	lastLoud time.Time

	// peak is the loudest frame heard so far, in dBFS, and is the level
	// everything else in the transmission is judged against. It means nothing
	// until heard is set.
	//
	// It only rises, so the answers it gives only get stricter as the
	// transmission goes on. Early frames are measured against a peak that has
	// not been reached yet, which is the forgiving direction: it keeps the
	// beginning of a recording rather than trimming into it.
	peak float64

	// heard says peak holds a measurement rather than nothing.
	//
	// A separate field because there is no level that can stand for "none".
	// Zero dBFS is not silence, it is the loudest a sample can be, and using it
	// as the empty value makes a transmission at full scale read as one that
	// was never heard at all.
	heard bool

	// lastRadio is when the radio was last seen receiving, and is what bounds
	// the recording when the radio is the authority.
	lastRadio time.Time
}
