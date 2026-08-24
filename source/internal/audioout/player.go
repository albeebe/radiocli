// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audioout

// newRing builds the jitter buffer a Player is opened with.
//
// The whole thing is allocated once, here, and never grows. Growing it would
// mean allocating on the path that Play runs on, which is a path that can be
// reached from an audio callback in the capture direction, and an allocation
// there is a gap in somebody's recording.
//
// Returns:
//   - *ring holding bufferFrames of audio at most, empty and not yet primed
func newRing() *ring {
	return &ring{buf: make([]byte, bufferFrames*FrameBytes)}
}

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

	if !r.primed {
		if r.length < primeFrames*FrameBytes {
			clear(out)
			return
		}
		r.primed = true
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

	if n < len(out) {
		// Running dry costs the cushion as well as the audio. Playing straight
		// on from whatever arrives next would leave the ring empty again, so
		// the next arrival has to build the cushion back up before it is heard.
		clear(out[n:])
		r.primed = false
		r.stats.Starved++
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
