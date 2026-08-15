// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audiofeed

import (
	"fmt"
	"log/slog"
	"math"
	"time"
)

// Channel is how the stereo is being folded: what was asked for, or what auto
// settled on.
//
// While auto is still deciding this reads as mix rather than as auto, because
// mix is what is actually being sent. Auto is a question, and answering a
// listener's "which channel am I hearing" with the name of the question would
// be reporting the setting instead of the fact.
//
// Returns:
//   - the channel mode in use: ChannelLeft, ChannelRight, or ChannelMix
func (c *Capture) Channel() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel == ChannelAuto {
		return ChannelMix
	}
	return c.channel
}

// Close stops recording and waits for the last frame to be published.
//
// Waiting rather than returning straight away, so that a caller which closes the
// capture and then tears down whatever it was publishing to cannot have a frame
// arrive in between.
//
// It is safe to call more than once, and safe for two goroutines to call at the
// same time. The shutdown itself happens once; the waiting happens in every
// caller, so a second one is told the last frame is out rather than being let
// go while the first is still tearing down.
func (c *Capture) Close() {
	c.closed.Do(func() {
		// The sound card first, so nothing new arrives while the cutter is
		// being stopped. Its callback only touches the ring and a buffered
		// channel, both of which outlive this.
		if c.in != nil {
			c.in.Close()
		}

		close(c.stop)
	})

	c.finished.Wait()
}

// Source is what the operating system calls the input this is recording from.
func (c *Capture) Source() string {
	if c.in == nil {
		return ""
	}
	return c.in.Name()
}

// Start opens the sound card and begins publishing frames.
//
// The Publisher is called from one goroutine, in frame order, and never from the
// sound card's own thread.
//
// Parameters:
//   - opts: what to open and how to fold it
//   - out: where finished frames and events are published; must not be nil
//
// Returns:
//   - a running Capture that records until Close is called
//   - error if out is nil, if the channel mode is not one of Channels, or if
//     the sound input cannot be opened
func Start(opts Options, out Publisher) (*Capture, error) {
	if out == nil {
		return nil, fmt.Errorf("starting a capture with nowhere to publish to")
	}

	channel, err := ParseChannel(opts.Channel)
	if err != nil {
		return nil, err
	}

	log := opts.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	c := &Capture{
		ring:    newRing(ringBytes),
		log:     log,
		stop:    make(chan struct{}),
		wake:    make(chan struct{}, 1),
		channel: channel,
	}
	c.pump = newPump(c.ring, out, channel)

	// The callback does two things and neither can block: it copies, and it
	// nudges. See ring.write.
	in, err := openInput(opts.Source, func(pcm []byte) {
		c.ring.write(pcm)
		select {
		case c.wake <- struct{}{}:
		default:
		}
	})
	if err != nil {
		return nil, err
	}
	c.in = in

	c.finished.Add(1)
	go c.run()

	return c, nil
}

// newPump returns a pump that cuts frames from r and hands them to out.
//
// Parameters:
//   - r: the ring the sound card writes into
//   - out: where finished frames and events are published
//   - channel: the channel mode the fold starts with; see ParseChannel
//
// Returns:
//   - a pump ready for its first drain
func newPump(r *ring, out Publisher, channel string) *pump {
	return &pump{
		ring:   r,
		out:    out,
		pick:   newChooser(channel),
		stereo: make([]byte, FrameBytes),
		mono:   make([]byte, MonoFrameBytes),
	}
}

// newRing returns an empty ring holding at least size bytes.
//
// Parameters:
//   - size: the smallest acceptable capacity, in bytes
//
// Returns:
//   - a ring whose capacity is size rounded up to a power of two
func newRing(size int) *ring {
	// Rounded up to a power of two so a position is found by masking rather
	// than by dividing, which matters in the one path that runs on the sound
	// card's thread.
	n := 1
	for n < size {
		n <<= 1
	}
	return &ring{buf: make([]byte, n), mask: uint64(n) - 1}
}

// capacity is how much audio the ring holds before it starts overwriting.
func (r *ring) capacity() uint64 { return uint64(len(r.buf)) }

// cut turns the stereo frame in hand into a mono frame and publishes it.
//
// Parameters:
//   - seq: the frame's number, counted in frames since the capture started
//   - now: when the frame was cut, carried on it as Frame.At
func (p *pump) cut(seq uint32, now time.Time) {
	left, right := rmsPair(p.stereo)
	mode := p.pick.observe(left, right)

	// Said once, when there is an answer, because a listener joining later is
	// told the settled channel when it subscribes.
	if settled, ok := p.pick.decided(); ok && !p.told {
		p.told = true
		p.out.PublishEvent("channel", map[string]any{"channel": settled})
	}

	downmix(p.stereo, p.mono, mode)
	level := levelOf(p.mono)

	// Fresh for each frame. See Frame.PCM for why this is not pooled.
	pcm := make([]byte, MonoFrameBytes)
	copy(pcm, p.mono)

	p.out.Publish(Frame{Seq: seq, PCM: pcm, Level: level, At: now})

	p.meter(level)
	p.watchForSilence(left, right)
}

// drain cuts every whole frame that is waiting and publishes it.
//
// Parameters:
//   - now: when this pass runs, stamped on every frame it cuts
func (p *pump) drain(now time.Time) {
	for {
		n, next, lost := p.ring.take(p.cursor, p.stereo)

		if lost > 0 {
			// The cursor jumped, so it may no longer sit on a frame boundary.
			// Rounding forward keeps every frame a real 20 ms of audio rather
			// than 20 ms straddling the gap, and leaves the frame number
			// telling the truth about how much went missing.
			p.cursor = next
			if rem := p.cursor % FrameBytes; rem != 0 {
				p.cursor += FrameBytes - rem
			}
			if n < FrameBytes {
				return
			}
			continue
		}

		if n < FrameBytes {
			return
		}
		seq := uint32(p.cursor / FrameBytes)
		p.cursor = next

		p.cut(seq, now)
	}
}

// meter sends the loudest of the last quarter second.
//
// The loudest rather than the average, because a meter is read to see whether
// there is anything there at all, and an average over 250 ms of speech spends
// most of its time in the gaps between words.
//
// Parameters:
//   - level: the frame just cut, in dBFS
func (p *pump) meter(level float64) {
	p.meterPeak = max(p.meterPeak, level)
	p.meterAt++

	if p.meterAt >= meterEvery {
		p.out.PublishEvent("level", map[string]any{"dbfs": round1(p.meterPeak)})
		p.meterAt = 0
		p.meterPeak = quietest
	}
}

// round1 keeps a decibel reading to one decimal place, which is finer than
// anything can show it and stops the number taking twelve characters on the
// wire.
//
// It rounds rather than truncates. Levels here are negative, and a conversion
// to int truncates towards zero, so truncating made every reading very slightly
// louder than it was: -30.46 dBFS came out as -30.4 rather than -30.5. Half a
// tenth of a decibel is not visible on a meter, but a function called round1
// that does not round is a thing the next reader has to discover.
//
// Parameters:
//   - v: the reading to trim, in dBFS
//
// Returns:
//   - v rounded to one decimal place
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// run cuts frames until the capture is closed.
func (c *Capture) run() {
	defer c.finished.Done()

	tick := time.NewTicker(wakeEvery)
	defer tick.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-c.wake:
		case <-tick.C:
		}

		c.pump.drain(time.Now())

		if settled, ok := c.pump.pick.decided(); ok {
			c.mu.Lock()
			if c.channel != settled {
				c.channel = settled
				c.log.Debug("the scanner's audio is on one channel", "channel", settled)
			}
			c.mu.Unlock()
		}
	}
}

// take copies bytes into into, starting from cursor.
//
// It returns how many bytes were copied, where the cursor has reached, and how
// many bytes were skipped because the writer had already overwritten them. A
// skip is not an error: it is a listener, or a whole machine, that stopped
// paying attention for longer than the ring is deep, and the only thing to do
// with the audio it missed is to notice it is gone.
//
// Parameters:
//   - cursor: the position in the stream to read from, in bytes
//   - into: the caller's buffer, filled from its start
//
// Returns:
//   - n: how many bytes were copied into into
//   - next: where the cursor has reached
//   - lost: how many bytes were skipped because the writer had already
//     overwritten them
func (r *ring) take(cursor uint64, into []byte) (n int, next uint64, lost uint64) {
	w := r.written.Load()

	// The oldest byte still in the ring. Anything before it has been written
	// over by audio that arrived since.
	var oldest uint64
	if w > r.capacity() {
		oldest = w - r.capacity()
	}
	if cursor < oldest {
		lost = oldest - cursor
		cursor = oldest
	}

	avail := w - cursor
	if avail > uint64(len(into)) {
		avail = uint64(len(into))
	}
	if avail == 0 {
		return 0, cursor, lost
	}

	start := cursor & r.mask
	n = copy(into, r.buf[start:min(start+avail, r.capacity())])
	if uint64(n) < avail {
		n += copy(into[n:avail], r.buf)
	}

	// Checked after the copy, not before. The writer may have caught up and
	// overwritten part of what was just read, in which case what is in into is a
	// mix of two moments and is worth nothing. It takes a reader stalled for a
	// whole second to reach this, so it is a guard rather than a path.
	if r.written.Load()-cursor > r.capacity() {
		return 0, r.written.Load(), lost + avail
	}

	return n, cursor + uint64(n), lost
}

// watchForSilence says something when a stream that opened perfectly delivers
// nothing at all.
//
// Exactly zero on both sides is the test, not merely quiet. See silentFor.
//
// Parameters:
//   - left: the left side's RMS in sample units
//   - right: the right side's RMS in sample units
func (p *pump) watchForSilence(left, right float64) {
	if left != 0 || right != 0 {
		p.silent = 0
		p.toldSilent = false
		return
	}

	p.silent++
	if p.silent < silentFrames || p.toldSilent {
		return
	}
	p.toldSilent = true
	p.out.PublishEvent("silent", map[string]any{
		"seconds": int(silentFor / time.Second),
	})
}

// write puts audio in, overwriting the oldest if there is no room.
//
// This is the one function here that runs on the sound card's thread, and it
// does the least it possibly can: two copies and one store. No allocation, no
// lock, no channel, no measurement. Everything else about a frame happens on the
// reading side, where being slow for a moment costs latency rather than a hole
// in the audio.
//
// Parameters:
//   - p: the audio to add, as the sound card delivered it
func (r *ring) write(p []byte) {
	if len(p) == 0 {
		return
	}

	// How much audio was really produced, which is what the clock advances by
	// even when most of it cannot be kept. Taken before the truncation below,
	// because a clock that counted only what fitted would run slow, and every
	// frame number worked out from it afterwards would claim a time earlier
	// than the moment it was recorded.
	produced := uint64(len(p))

	// More than the whole ring in one go means all but the tail of it is
	// already dead. Keeping the tail is what the reader would have kept.
	if produced > r.capacity() {
		p = p[produced-r.capacity():]
	}

	at := r.written.Load()
	start := (at + produced - uint64(len(p))) & r.mask
	n := copy(r.buf[start:], p)
	if n < len(p) {
		copy(r.buf, p[n:])
	}

	// Stored last, so a reader that sees the new position sees the bytes that
	// go with it.
	r.written.Store(at + produced)
}
