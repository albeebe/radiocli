// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package opusenc

import (
	"errors"
)

// The shape of the only frame this package encodes.
//
// These are the encoder's constraints rather than choices, and they are restated
// here so a caller can size its buffers without importing the codec library,
// which is the whole point of the package.
const (
	// FrameBytes is one frame as the bytes Encode takes, which is
	// FrameSamples of signed 16-bit little-endian mono.
	FrameBytes = FrameSamples * 2

	// FrameMS is the only frame length the encoder produces.
	FrameMS = 20

	// FrameSamples is one frame in samples: 48000 * 20 / 1000.
	FrameSamples = SampleRate * FrameMS / 1000

	// SampleRate is the only rate the encoder accepts.
	SampleRate = 48000
)

// What SetBitrate will accept, which is what the library accepts.
//
// The useful range is much narrower. See the package doc: CELT without the
// hybrid mode wants roughly 24 to 48 kbps for speech, and the ladder the daemon
// steps through lives in that band. These are only the outer walls.
const (
	// MaxBitrate is the highest rate, in bits per second, the encoder accepts.
	MaxBitrate = 510000

	// MinBitrate is the lowest rate, in bits per second, the encoder accepts.
	MinBitrate = 6000
)

// MaxPacket is the largest packet the encoder can produce, and therefore how
// big an output buffer has to be.
//
// It is the Opus format's own limit for a single frame rather than anything
// about this encoder, so it holds whatever the bitrate is set to. At the rates
// this tool uses a packet is nearer 120 bytes, but a buffer sized for the
// common case is a buffer that overflows on the uncommon one.
//
// The format is usually quoted as 1275, which is the frame data alone. A packet
// also carries the one table of contents byte that says how to read it, so the
// whole thing is one longer. At MaxBitrate the encoder produces exactly this
// many bytes and a buffer a single byte short of it fails on that frame, which
// is how the 1275 this used to be was found to be wrong.
const MaxPacket = 1276

// ErrFrameSize says Encode was given something other than exactly one frame.
var ErrFrameSize = errors.New("not one 20 ms frame")

// codec is the part of the library's encoder this package uses.
//
// It exists so a test can stand in for the codec and drive the failures the
// real one will not produce on demand. *opus.Encoder satisfies it already, and
// New still builds exactly that, so nothing about the containment this package
// exists for changes: the interface names the library's methods, not its types.
type codec interface {
	// Encode turns one frame into one packet written into out.
	Encode(pcm []byte, out []byte) (int, error)

	// SetBitrate changes the rate from the next frame on.
	SetBitrate(bps int) error
}

// Encoder turns frames into packets. It is not safe for concurrent use, which
// suits how it is used: one encoder belongs to one listener, so that each can
// be given a different bitrate.
type Encoder struct {
	enc codec // The codec library's encoder that every call delegates to
}
