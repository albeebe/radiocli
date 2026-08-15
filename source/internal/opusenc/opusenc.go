// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package opusenc encodes one 20 ms frame of scanner audio into one Opus packet.
//
// It exists for the same reason audioin does: to keep one third-party library
// out of the rest of the tool. Everything that knows the codec library lives in
// this package, and nothing outside it names one of its types, so deciding
// against it later is a matter of rewriting one package rather than finding
// every place that touched it. That containment can be checked, by grepping
// for the import and finding only this package.
//
// Unlike audioin there is no build-tagged twin, because the library is pure Go.
// That is worth stating plainly: a build with cgo switched off cannot open a
// sound card at all, but it can still encode, so a daemon built that way and
// handed audio some other way would work. It also means nothing here can be the
// reason a cross compile fails.
//
// # Why Opus, and why this narrow
//
// The tool sends this audio to listeners over connections it knows nothing
// about. Opus is the one codec that lets the bitrate move between one frame and
// the next without the far end resetting anything, which is what makes adapting
// to a slow listener possible at all. A codec fixed at one rate would mean
// choosing, once, between a listener on the same machine and one on a phone.
//
// The library implements only part of Opus, and this package deliberately
// exposes no more than that part:
//
//   - 48 kHz, because the encoder rejects every other rate. This costs nothing:
//     a line input is natively 44.1 or 48 kHz, so the card is opened at 48 and
//     fed straight through with no resampling.
//   - Mono, because the scanner is mono. Which channel of the cable it arrived
//     on is settled before anything reaches here.
//   - 20 ms frames, because the encoder has one frame size.
//   - CELT rather than the hybrid mode libopus would pick. CELT is the music
//     and low-latency half of Opus and does worse on speech at low rates, so
//     the useful floor here is around 24 kbps rather than the 16 a hybrid
//     encoder stays clear at. Still better than the 64 kbps that telephone-grade
//     mu-law would cost, which is the alternative this was weighed against.
//
// # The pin
//
// github.com/pion/opus is pinned to commit a38c746 of 2026-08-10, as a
// pseudo-version rather than a tag, and that is not tidiness. The encoder does
// not exist in the v0.1.0 tag at all; it is only on master, where the CELT
// signal processing was being ported from libopus commit by commit on the days
// either side of that one.
//
// So the pin holds two things still, not one. A build that floated would pick up
// a half-finished port, and the output *quality* can move between commits while
// the port is in progress even when nothing about the API does.
//
// Checking it after a change means listening, and the tests here cannot do that.
// What they can do is prove the packets are the right size and that the bitrate
// knob moves them. What they cannot prove is that anything else can read them,
// which is the part that matters, since the decoder at the far end is somebody
// else's and not this library's. That check is done by hand: take the packets from
// "radiocli audio listen --format opus", put them in an Ogg container, and play
// the result with something that has nothing to do with this code. It was done
// at the pinned commit and the audio came back correct.
//
// # What it costs
//
// Measured on an Apple M4 Pro at 32 kbps, a frame takes about 100us to encode.
// That is half a percent of the 20 ms it represents, so one core carries on the
// order of two hundred listeners, and the cost of giving every listener its own
// encoder rather than sharing one is not worth avoiding. A slower machine has
// room to be several times slower before that stops being true.
package opusenc

import (
	"fmt"

	"github.com/pion/opus"
)

// New returns an encoder at bitrate, in bits per second.
//
// Complexity is left at the library's default of 5. Higher settings buy quality
// by spending processor time, and this runs one encoder per listener at fifty
// frames a second each, so the middle of the scale is where the trade sits until
// there is a measurement saying otherwise.
//
// Constrained variable bitrate, also the default, is what makes the rate asked
// for the rate that arrives. Unconstrained VBR would let a loud frame spend
// several times the average, which is exactly the spike that fills a slow
// listener's queue and provokes the step down that was being avoided.
//
// Parameters:
//   - bitrate: the starting rate in bits per second, between MinBitrate and MaxBitrate
//
// Returns:
//   - *Encoder ready to encode frames at bitrate
//   - error if the library rejects the configuration, such as a bitrate outside
//     MinBitrate to MaxBitrate
func New(bitrate int) (*Encoder, error) {
	enc, err := opus.NewEncoder(
		opus.WithSampleRate(SampleRate),
		opus.WithChannels(1),
		opus.WithBitrate(bitrate),
	)
	if err != nil {
		return nil, fmt.Errorf("preparing the audio encoder: %w", err)
	}
	return &Encoder{enc: enc}, nil
}

// Encode turns one frame into one packet, written into out, and returns how
// many bytes of it were used.
//
// pcm must be exactly FrameBytes and out must be at least MaxPacket. Both are
// checked, because the library's own message for a wrong length says nothing
// about which of the tool's buffers was wrong.
//
// Nothing is allocated per call, which is what lets a caller run this fifty
// times a second for every listener without giving the garbage collector a
// reason to notice.
//
// Parameters:
//   - pcm: one frame of signed 16-bit little-endian mono samples, exactly FrameBytes long
//   - out: the buffer the packet is written into, at least MaxPacket bytes
//
// Returns:
//   - the number of bytes of out the packet occupies
//   - error if either buffer is the wrong size or the library fails to encode
//
// Errors:
//   - ErrFrameSize: if pcm is not exactly FrameBytes long
func (e *Encoder) Encode(pcm []byte, out []byte) (int, error) {
	if len(pcm) != FrameBytes {
		return 0, fmt.Errorf("%w: got %d bytes, want %d", ErrFrameSize, len(pcm), FrameBytes)
	}
	if len(out) < MaxPacket {
		return 0, fmt.Errorf("audio packet buffer is %d bytes, want at least %d", len(out), MaxPacket)
	}

	n, err := e.enc.Encode(pcm, out)
	if err != nil {
		return 0, fmt.Errorf("encoding audio: %w", err)
	}
	return n, nil
}

// SetBitrate changes the rate, in bits per second, from the next frame on.
//
// This is the whole of the adaptation, and it is this cheap on purpose: no
// packet says the rate changed, and nothing at the far end has to be told or
// reset. The frames simply get smaller. A decoder reads the rate out of each
// packet, so it never knew what to expect in the first place.
//
// What must not change this way is the format. Moving between Opus and raw
// audio is a different stream, and it is done by ending this one and starting
// another.
//
// Parameters:
//   - bps: the new rate in bits per second, between MinBitrate and MaxBitrate
//
// Returns:
//   - error if the library rejects bps, such as a value outside MinBitrate to
//     MaxBitrate, and nil otherwise
func (e *Encoder) SetBitrate(bps int) error {
	if err := e.enc.SetBitrate(bps); err != nil {
		return fmt.Errorf("setting the audio bitrate to %d: %w", bps, err)
	}
	return nil
}
