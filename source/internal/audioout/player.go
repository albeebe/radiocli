// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audioout

import (
	"encoding/binary"
	"math"
)

// Close stops the device and gives back everything the library allocated.
//
// The audio still waiting in the ring is not played out first. Close is what
// Ctrl-C reaches, and a command that took another quarter of a second to exit
// so that it could finish saying something nobody is listening to any more
// would be worse, not better.
//
// It is safe to call more than once, which is what lets a caller close on the
// way out of more than one path, and safe on a nil Player.
func (p *Player) Close() {
	if p == nil {
		return
	}
	p.closed.Do(func() {
		p.out.Close()
	})
}

// Name is what the operating system calls the output being played on, spelled
// the way the system spells it.
//
// It is empty when the default device was opened and the library did not say
// which one that is, so a caller showing this needs something to say for that
// case: "playing on " is not a sentence.
//
// Returns:
//   - the sink's name, or empty for a nil Player or an unnamed default
func (p *Player) Name() string {
	if p == nil {
		return ""
	}
	return p.out.Name()
}

// Play hands over audio to be played as soon as the speakers ask for it.
//
// It never blocks and it never fails. Audio that arrives faster than the
// speakers take it is dropped rather than waited on, because whoever is calling
// this is usually holding audio that is on its way somewhere else as well: the
// recorder's frames are going to a file, and a file that stalled because the
// speakers were behind would be this feature damaging the thing it was added
// to.
//
// The samples are copied, so the slice is the caller's again the moment this
// returns. That matters because the frames handed in here often belong to a
// capture callback's buffer, which is written over as soon as it returns.
//
// Parameters:
//   - pcm: signed 16-bit little-endian mono samples at SampleRate, any length
func (p *Player) Play(pcm []byte) {
	if p == nil {
		return
	}
	p.ring.write(pcm)
}

// SetGain multiplies everything played from now on by gain decibels.
//
// It is here because the live audio and the recording of it are not the same
// loudness and cannot be. A recording is scaled once it has ended, by its own
// loudest moment, which is a number nobody has yet while the transmission is
// still arriving. So a file comes out just under full scale and the same audio
// played as it arrives comes out wherever the radio and the cable left it,
// which on a line input measured 15 to 25 dB quieter. Somebody comparing the
// two turns their speakers up, and then every edge in the audio is 20 dB louder
// as well.
//
// A number rather than anything automatic. An automatic gain would have to
// decide, in the first moment of a transmission, how loud the rest of it is
// going to be, and it would be wrong at the start of every one.
//
// Parameters:
//   - dB: decibels to apply, 0 for the audio exactly as it arrived
func (p *Player) SetGain(dB float64) {
	if p == nil {
		return
	}
	p.ring.gain.Store(math.Float64bits(math.Pow(10, dB/20)))
}

// Stats says what the ring had to do to keep the speakers fed.
//
// Neither number is a fault on its own, and a caller reporting them to a person
// should know why:
//
//   - Starved counts the ring running dry, and it does that at the end of every
//     burst of audio. Something playing only the transmissions rather than the
//     whole feed will collect one of these per transmission, which is the
//     buffer working rather than failing.
//   - Dropped counts audio thrown away because it arrived faster than the
//     speakers took it. That is the one worth looking at, because in a stream
//     that arrives in real time it should never happen at all.
//   - Played is the one that makes the other two readable. Nothing dropped and
//     nothing starved means the speakers kept up with whatever they were given,
//     which is also true of speakers that were given nothing.
//
// Returns:
//   - Stats as of this moment, or the zero value for a nil Player
func (p *Player) Stats() Stats {
	if p == nil {
		return Stats{}
	}
	p.ring.mu.Lock()
	defer p.ring.mu.Unlock()
	return p.ring.stats
}

// newRing builds the jitter buffer a Player is opened with.
//
// The whole thing is allocated once, here, and never grows. Growing it would
// mean allocating on the path that Play runs on, which is a path that can be
// reached from an audio callback in the capture direction, and an allocation
// there is a gap in somebody's recording.
//
// Parameters:
//   - prime: how many bytes must be waiting before playing starts, which is
//     the cushion that then stands in front of the audio for as long as it
//     keeps coming
//
// Returns:
//   - *ring holding bufferFrames of audio at most, empty and not yet primed
func newRing(prime int) *ring {
	r := &ring{buf: make([]byte, bufferFrames*FrameBytes), prime: prime}
	r.gain.Store(math.Float64bits(1))
	return r
}

// fade ramps a run of samples between silence and full, in place.
//
// It is what stops the speakers clicking at the edges of a burst. Audio does
// not start or end at zero, so handing the device a step from silence to the
// middle of a waveform is handing it a click, and doing that at both ends of
// every over is what somebody hears as a run that pops.
//
// Linear rather than anything shaped, because over 5 ms the difference is not
// audible and the arithmetic is a multiply per sample on a path the audio
// thread runs.
//
// Parameters:
//   - pcm: the samples to ramp, signed 16-bit little-endian
//   - in: true to ramp up from silence, false to ramp down to it
func fade(pcm []byte, in bool) {
	samples := len(pcm) / 2
	if samples == 0 {
		return
	}

	for i := range samples {
		at := i * 2
		v := int16(binary.LittleEndian.Uint16(pcm[at:]))

		// The step along the ramp, counted from the silent end so that both
		// directions are the same arithmetic read backwards.
		step := i
		if !in {
			step = samples - 1 - i
		}
		binary.LittleEndian.PutUint16(pcm[at:], uint16(int16(int(v)*step/samples)))
	}
}

// gainNow is what every sample is being multiplied by at this moment.
//
// A load rather than a lock, which is the point of storing the gain as bits:
// write asks this on the way to the speakers fifty times a second, and the
// lock it would otherwise need is the one the audio thread waits on.
//
// Returns:
//   - the multiplier, 1 for audio passed through exactly as it arrived
func (r *ring) gainNow() float64 {
	return math.Float64frombits(r.gain.Load())
}

// read fills out with whatever is waiting, and with silence for whatever is
// not.
//
// This is what the audio library calls, on its own thread, and it has to fill
// the whole buffer every time. A buffer left short is not silence: it is
// whatever the library last had in that memory, played again.
//
// Priming is why the first call after a quiet spell plays nothing. Handing over
// the first frame the moment it arrives would leave the ring empty again
// immediately, and from then on every arrival that was a millisecond late would
// be an audible hole. Waiting until primeFrames are standing puts that cushion
// in front of the audio once, and it stays there for as long as the audio keeps
// coming.
//
// Parameters:
//   - out: the library's own buffer, filled completely, in the format the
//     package constants describe
func (r *ring) read(out []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	starting := false
	if !r.primed {
		if r.length < r.prime {
			clear(out)
			return
		}
		r.primed = true
		starting = true
	}

	n := min(len(out), r.length)

	// Two copies because the audio waiting may be in two pieces: the ring wraps
	// wherever it happens to have got to, and the newest byte can sit before
	// the oldest one in the buffer.
	k := copy(out[:n], r.buf[r.start:])
	if k < n {
		copy(out[k:n], r.buf[:n-k])
	}

	r.start = (r.start + n) % len(r.buf)
	r.length -= n
	r.stats.Played += uint64(n)

	// The two edges of a burst, ramped so that neither is a step. See fade.
	if starting {
		fade(out[:min(fadeBytes, n)], true)
	}

	if n < len(out) {
		// Running dry costs the cushion as well as the audio. Playing straight
		// on from whatever arrives next would leave the ring empty again, so
		// the next arrival has to build the cushion back up before it is heard.
		fade(out[n-min(fadeBytes, n):n], false)
		clear(out[n:])

		// Usually the ring drained to exactly empty last time, the fade above
		// had nothing to ramp, and the step down to silence still has to be
		// made somewhere. See settle.
		settle(out[n:], r.last)
		r.primed = false
		r.stats.Starved++
	}

	// Remembered for the starve that has not happened yet. The starving
	// callback is usually handed nothing at all, so the sample to ramp down
	// from has to have been kept by the callback before it. Every path above
	// leaves the buffer ending on the right value: audio on its final sample,
	// and a starve on the silence it settled to.
	r.last = int16(binary.LittleEndian.Uint16(out[len(out)-2:]))
}

// scale multiplies every sample by the ring's gain, in place, holding anything
// that would overflow at full scale.
//
// Clamped rather than allowed to wrap, because a sample that wraps does not
// come back quieter, it comes back inverted, which is a far worse noise than
// the loud one it was meant to be.
//
// Parameters:
//   - pcm: the samples to scale, signed 16-bit little-endian
//   - gain: what to multiply each of them by
func scale(pcm []byte, gain float64) {
	for at := 0; at+1 < len(pcm); at += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[at:]))) * gain
		if v > math.MaxInt16 {
			v = math.MaxInt16
		}
		if v < math.MinInt16 {
			v = math.MinInt16
		}
		binary.LittleEndian.PutUint16(pcm[at:], uint16(int16(v)))
	}
}

// settle writes a short slope from a sample down to silence at the front of
// pcm, which is already silent.
//
// It is the fade for the moment fade cannot reach: the ring running dry. The
// sizes a real device deals in divide each other, so the ring drains to
// exactly empty and the starving callback has no tail of audio left to ramp.
// Whatever sample the previous callback ended on is still hanging in the air,
// and stepping from it straight to silence is a click at the end of every
// burst. This writes the slope that audio no longer exists to carry.
//
// Parameters:
//   - pcm: the silence to write the slope into, ramped over at most fadeBytes
//   - from: the sample the previous callback ended on, 0 to leave the silence
//     alone
func settle(pcm []byte, from int16) {
	if from == 0 {
		return
	}

	samples := min(len(pcm), fadeBytes) / 2
	for i := range samples {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(int(from)*(samples-1-i)/samples)))
	}
}

// write puts audio at the newest end of the ring, dropping the oldest to make
// room.
//
// Dropping the oldest rather than refusing the newest is the choice that suits
// live audio: what is being played is the radio as it is now, and a listener
// who has fallen behind wants to catch up rather than to hear a queue of what
// they missed. It is also what audiofeed does to a subscriber that cannot keep
// up, so there is one answer to this question in the tool rather than two.
//
// Parameters:
//   - pcm: the samples to play, in the format the package constants describe
func (r *ring) write(pcm []byte) {
	if len(pcm) == 0 {
		return
	}

	// Scaled before the ring's lock is touched, not while it is held. The
	// audio thread waits on that lock, and a thousand multiplications is
	// exactly the kind of wait it must never be handed. The copy into scratch
	// also serves the other rule Play makes: the caller's slice is theirs the
	// moment this returns, and often belongs to a capture callback that is
	// about to write over it anyway.
	if g := r.gainNow(); g != 1 {
		r.scratchMu.Lock()
		defer r.scratchMu.Unlock()
		if cap(r.scratch) < len(pcm) {
			r.scratch = make([]byte, len(pcm))
		}
		scaled := r.scratch[:len(pcm)]
		copy(scaled, pcm)
		scale(scaled, g)
		pcm = scaled
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// More than the ring holds in a single call. Only the tail can be kept, and
	// keeping the tail rather than the head is the same choice as below: the
	// newest audio is the audio worth having.
	if len(pcm) > len(r.buf) {
		r.stats.Dropped += uint64(len(pcm) - len(r.buf))
		pcm = pcm[len(pcm)-len(r.buf):]
	}

	if room := len(r.buf) - r.length; len(pcm) > room {
		drop := len(pcm) - room
		r.start = (r.start + drop) % len(r.buf)
		r.length -= drop
		r.stats.Dropped += uint64(drop)
	}

	at := (r.start + r.length) % len(r.buf)
	k := copy(r.buf[at:], pcm)
	if k < len(pcm) {
		copy(r.buf, pcm[k:])
	}
	r.length += len(pcm)
}
