// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package opusenc

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

// tone fills one frame with a sine at hz, at roughly half full scale.
//
// Half rather than full because a signal pinned at the top of the range is the
// one case where an encoder's output size says more about clipping than about
// the bitrate it was asked for, and these tests are about the bitrate.
func tone(hz float64) []byte {
	pcm := make([]byte, FrameBytes)
	for i := range FrameSamples {
		v := math.Sin(2 * math.Pi * hz * float64(i) / SampleRate)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(v*16000)))
	}
	return pcm
}

// The ladder the daemon steps through. Kept here as well so a change to the
// codec that breaks one of the rungs fails in this package rather than in the
// one that chose them.
var rungs = []int{24000, 32000, 40000, 48000}

func TestEncodeProducesAPacketAtEveryRung(t *testing.T) {
	pcm := tone(1000)
	out := make([]byte, MaxPacket)

	for _, bitrate := range rungs {
		enc, err := New(bitrate)
		if err != nil {
			t.Fatalf("New(%d): %v", bitrate, err)
		}

		// Several frames rather than one. The encoder carries state between
		// frames, so the first is the one least like the ones that follow, and
		// a test that only ever encodes a first frame would miss a codec that
		// went wrong on its second.
		for frame := range 10 {
			n, err := enc.Encode(pcm, out)
			if err != nil {
				t.Fatalf("%d kbps, frame %d: %v", bitrate/1000, frame, err)
			}
			if n == 0 {
				t.Errorf("%d kbps, frame %d: empty packet", bitrate/1000, frame)
			}
			if n > MaxPacket {
				t.Errorf("%d kbps, frame %d: packet is %d bytes, over the %d maximum",
					bitrate/1000, frame, n, MaxPacket)
			}
		}
	}
}

// TestSetBitrateChangesThePacketSize is what the adaptation rests on: asking for
// fewer bits has to actually produce fewer bytes, or stepping down when a
// listener cannot keep up would send exactly as much as before.
func TestSetBitrateChangesThePacketSize(t *testing.T) {
	pcm := tone(1000)
	out := make([]byte, MaxPacket)

	enc, err := New(48000)
	if err != nil {
		t.Fatal(err)
	}

	// The average of several frames rather than one packet against one packet.
	// A single pair can differ by a byte or two for reasons that have nothing
	// to do with the rate.
	average := func() float64 {
		total := 0
		for range 20 {
			n, err := enc.Encode(pcm, out)
			if err != nil {
				t.Fatal(err)
			}
			total += n
		}
		return float64(total) / 20
	}

	high := average()

	if err := enc.SetBitrate(24000); err != nil {
		t.Fatal(err)
	}
	low := average()

	if low >= high {
		t.Errorf("packets averaged %.1f bytes at 48 kbps and %.1f at 24 kbps, "+
			"so lowering the bitrate sent no less", high, low)
	}
}

func TestSetBitrateRefusesWhatTheCodecCannotDo(t *testing.T) {
	enc, err := New(32000)
	if err != nil {
		t.Fatal(err)
	}

	for _, bps := range []int{0, MinBitrate - 1, MaxBitrate + 1} {
		if err := enc.SetBitrate(bps); err == nil {
			t.Errorf("SetBitrate(%d) was accepted", bps)
		}
	}
}

func TestNewRefusesABitrateOutOfRange(t *testing.T) {
	for _, bps := range []int{0, MinBitrate - 1, MaxBitrate + 1} {
		if _, err := New(bps); err == nil {
			t.Errorf("New(%d) was accepted", bps)
		}
	}
}

// TestEncodeRefusesAnythingButOneFrame guards the seam where this package meets
// the capture. A short buffer means frames were assembled wrongly, and encoding
// it anyway would turn that into audio that is subtly wrong rather than into an
// error naming the size.
func TestEncodeRefusesAnythingButOneFrame(t *testing.T) {
	enc, err := New(32000)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, MaxPacket)

	for _, size := range []int{0, 1, FrameBytes - 2, FrameBytes - 1, FrameBytes + 2} {
		_, err := enc.Encode(make([]byte, size), out)
		if err == nil {
			t.Errorf("Encode accepted %d bytes", size)
			continue
		}
		if size != FrameBytes-1 && size != 1 && !errors.Is(err, ErrFrameSize) {
			t.Errorf("Encode(%d bytes) gave %v, want ErrFrameSize", size, err)
		}
	}
}

func TestEncodeRefusesAShortOutputBuffer(t *testing.T) {
	enc, err := New(32000)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := enc.Encode(tone(1000), make([]byte, MaxPacket-1)); err == nil {
		t.Error("Encode accepted an output buffer smaller than MaxPacket")
	}
}

// TestEncodeAcceptsExactlyMaxPacketAtMaxBitrate is the other half of the guard
// test above: one byte less than MaxPacket is refused, and MaxPacket itself is
// accepted at the rate that produces the largest packet there is.
//
// This test used to assert the opposite. MaxPacket was 1275, the figure the
// format is usually quoted as, which counts the frame and not the table of
// contents byte beside it, so a buffer of exactly MaxPacket passed this
// package's own check and was then refused by the codec. That was read as a
// quirk of the library and written down as expected, when it was this package
// promising a buffer size that did not work.
//
// Nothing a caller can pass now gets past the two guards and still fails inside
// the codec, which is the point of them. The codec's own error is driven with a
// stand-in instead, in TestEncodeWrapsTheCodecsOwnError.
func TestEncodeAcceptsExactlyMaxPacketAtMaxBitrate(t *testing.T) {
	enc, err := New(MaxBitrate)
	if err != nil {
		t.Fatal(err)
	}

	// Noise, because a tone at this rate compresses to far less than the
	// encoder may spend and would never approach the limit.
	pcm := make([]byte, FrameBytes)
	for i := range pcm {
		pcm[i] = byte(i * 7 % 251)
	}

	n, err := enc.Encode(pcm, make([]byte, MaxPacket))
	if err != nil {
		t.Fatalf("Encode at MaxBitrate refused a buffer of exactly MaxPacket: %v", err)
	}
	if n > MaxPacket {
		t.Errorf("packet is %d bytes, over the %d MaxPacket promises", n, MaxPacket)
	}
}

// BenchmarkEncode is the measurement the design waits on.
//
// Every listener gets an encoder of its own, so that each can be given its own
// bitrate, and each one runs 1000/FrameMS frames a second. Real time for one
// listener is therefore 20 ms of encoding per 20 ms of wall clock: a run at
// N ns/op supports roughly 20000000/N listeners on one core before the daemon
// stops keeping up.
//
// libopus does this in tens of microseconds. This library is a young pure Go
// port, so the number is worth knowing before anything depends on it.
func BenchmarkEncode(b *testing.B) {
	enc, err := New(32000)
	if err != nil {
		b.Fatal(err)
	}
	pcm := tone(1000)
	out := make([]byte, MaxPacket)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := enc.Encode(pcm, out); err != nil {
			b.Fatal(err)
		}
	}

	// Frames a second one core could encode, which is the number that decides
	// how many listeners are affordable.
	perFrame := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.ReportMetric(1e9/perFrame/(1000/FrameMS), "listeners/core")
}

// TestEncodeFitsAPacketAtMaxBitrate covers the boundary the rung test cannot
// reach, because the rungs stop at 48 kbps and the packet only outgrows the
// buffer at the very top of the range.
//
// MaxPacket used to be 1275, the figure the Opus format is usually quoted as,
// which leaves out the table of contents byte every packet carries. A buffer of
// exactly MaxPacket therefore passed Encode's own check and was then refused by
// the codec, so a listener at the top rate got an error frame and a stream that
// ended rather than audio.
func TestEncodeFitsAPacketAtMaxBitrate(t *testing.T) {
	enc, err := New(MaxBitrate)
	if err != nil {
		t.Fatalf("New(%d): %v", MaxBitrate, err)
	}

	// Noise rather than a tone, because a tone at this rate compresses to far
	// less than the encoder is allowed to spend and would never reach the
	// limit being tested.
	pcm := make([]byte, FrameBytes)
	for i := range pcm {
		pcm[i] = byte(i * 7 % 251)
	}

	// Exactly MaxPacket, which is what the documentation promises is enough.
	out := make([]byte, MaxPacket)

	for frame := range 10 {
		n, err := enc.Encode(pcm, out)
		if err != nil {
			t.Fatalf("frame %d at %d kbps: %v", frame, MaxBitrate/1000, err)
		}
		if n > MaxPacket {
			t.Errorf("frame %d: packet is %d bytes, over the %d maximum", frame, n, MaxPacket)
		}
	}
}

// TestMaxPacketCoversTheWholeBitrateRange walks the range rather than the rungs,
// so a codec change that makes packets bigger anywhere is caught here rather
// than by a listener whose stream stops.
func TestMaxPacketCoversTheWholeBitrateRange(t *testing.T) {
	pcm := make([]byte, FrameBytes)
	for i := range pcm {
		pcm[i] = byte(i * 13 % 253)
	}

	for _, bitrate := range []int{MinBitrate, 24000, 48000, 128000, 256000, MaxBitrate} {
		enc, err := New(bitrate)
		if err != nil {
			t.Fatalf("New(%d): %v", bitrate, err)
		}

		out := make([]byte, MaxPacket)
		n, err := enc.Encode(pcm, out)
		if err != nil {
			t.Errorf("%d bps: %v", bitrate, err)
			continue
		}
		if n > MaxPacket {
			t.Errorf("%d bps: packet is %d bytes, over the %d maximum", bitrate, n, MaxPacket)
		}
	}
}

// failingCodec stands in for the library's encoder so the error the real one
// will not produce on demand can still be driven.
type failingCodec struct {
	err error // What both methods report
}

// Encode reports the failure the test asked for.
func (c failingCodec) Encode(pcm []byte, out []byte) (int, error) { return 0, c.err }

// SetBitrate reports the failure the test asked for.
func (c failingCodec) SetBitrate(bps int) error { return c.err }

// TestEncodeWrapsTheCodecsOwnError checks that a failure from inside the codec
// comes back wrapped rather than swallowed.
//
// No buffer a caller can pass reaches this: the two guards in Encode catch
// every size the codec would refuse, which is the point of them. The failure is
// driven with a stand-in codec instead, so the branch is still exercised
// without a buffer size that lies about what the codec accepts.
func TestEncodeWrapsTheCodecsOwnError(t *testing.T) {
	want := errors.New("the codec gave up")
	enc := &Encoder{enc: failingCodec{err: want}}

	_, err := enc.Encode(make([]byte, FrameBytes), make([]byte, MaxPacket))
	if err == nil {
		t.Fatal("Encode swallowed the codec's error")
	}
	if !errors.Is(err, want) {
		t.Errorf("Encode returned %v, which does not wrap the codec's own error", err)
	}
}

// TestSetBitrateWrapsTheCodecsOwnError checks the same for the rate change,
// whose failure the real library only produces for a rate New already refuses.
func TestSetBitrateWrapsTheCodecsOwnError(t *testing.T) {
	want := errors.New("the codec refused the rate")
	enc := &Encoder{enc: failingCodec{err: want}}

	err := enc.SetBitrate(32000)
	if err == nil {
		t.Fatal("SetBitrate swallowed the codec's error")
	}
	if !errors.Is(err, want) {
		t.Errorf("SetBitrate returned %v, which does not wrap the codec's own error", err)
	}
}
