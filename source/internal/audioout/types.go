// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audioout

import (
	"errors"
	"sync"
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
//
// Both are latency, and latency is the whole trade this package makes. Audio
// arrives here in 20 ms frames off a socket or a capture callback, and the
// sound card asks for audio on its own schedule, which is neither the same
// schedule nor a steady one. Without something in between, every jitter in the
// arrivals is an audible gap.
const (
	// bufferFrames is how much the ring holds, at 20 ms each. 240 ms is far
	// more than the arrivals ever drift by, and it is a ceiling rather than a
	// target: what is actually standing in it is primeFrames, because the
	// reader takes out exactly what the writer puts in. It matters only when
	// something stalls, and then it is the difference between a stutter and a
	// gap.
	bufferFrames = 12

	// primeFrames is how much has to arrive before playing starts, and how much
	// therefore stands between the audio and the speakers from then on. 60 ms
	// is the cushion: three frames of slack before a late arrival is a hole,
	// and a delay short enough that nobody listening to a scanner would notice
	// it.
	primeFrames = 3
)

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
// Both numbers count something the listener may have heard, and neither is
// necessarily a fault. See Player.Stats.
type Stats struct {
	// Dropped is how many bytes of audio were thrown away because they arrived
	// faster than the speakers took them.
	Dropped uint64 `json:"dropped"`

	// Starved is how many times the ring ran dry and silence was played
	// instead.
	Starved uint64 `json:"starved"`
}

// output is the part of an open playback device this package uses: it can say
// what it is, and it can be closed.
//
// An interface rather than the concrete type, because there are two of those.
// One is built on the audio library and only exists in a build with cgo; the
// other is the stand-in that lets the rest of the tool compile without it.
type output interface {
	// Close stops the device and gives back everything the library allocated.
	Close()

	// Name is what the operating system calls the output being played on.
	Name() string
}

// ring is the jitter buffer: a fixed run of bytes that Play fills and the audio
// thread empties.
//
// A mutex, in something an audio callback touches, needs justifying. The rule
// the callback lives under is that it must not block, and this cannot make it
// block for meaningfully longer than a copy: the only other holder is Play,
// which does no allocation, no system call and no waiting while it holds the
// lock. A lock-free ring would remove even that, at the cost of being the kind
// of code that is wrong for a year before anybody notices.
type ring struct {
	mu     sync.Mutex // Held for a copy and nothing else, by Play and by the audio thread
	buf    []byte     // Fixed capacity, allocated once at Open and never grown
	start  int        // Where the oldest byte sits in buf
	length int        // How many bytes are waiting
	primed bool       // Whether enough has arrived to start playing, see primeFrames
	stats  Stats      // What has been dropped and how often it has run dry
}
