// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package audiofeed turns one sound card into audio for as many listeners as
// ask for it.
//
// It is the audio half of what the daemon already does for the scanner. The
// serial port is claimed once and every command queues for it, because the radio
// answers one at a time. A sound card is not like that: it produces audio
// whether or not anybody is listening, and two programs opening the same input
// is at best wasteful and on some systems refused outright. So the same shape
// applies for a different reason. One capture runs, and everybody who wants the
// audio is given a copy of it.
//
// Reading here rather than in each listener is what makes several listeners
// affordable. It also means the daemon knows the whole truth about the input,
// where a listener only ever knows its own share.
//
// # What is in a frame
//
// Everything downstream works in 20 ms frames of mono at 48 kHz, because that is
// the one thing the Opus encoder accepts. The sound card is opened in stereo,
// since which side of the cable the scanner landed on is not knowable in advance,
// and the fold to mono happens once here rather than in every listener.
//
// # What is not here
//
// No sound library and no codec. This package is given bytes and hands out
// frames, so all of it but the one call that opens a device can be tested
// without one.
//
// No gate either, yet. The channel this tool listens to is silent most of the
// time, and one day it will be worth sending only the parts that are not. The
// shape for that is already here: see Publisher, which exists so a gate can be
// dropped between the capture and the fan-out without either end knowing.
package audiofeed

import (
	"log/slog"
)

// New returns an empty feed.
//
// Parameters:
//   - log: where trouble is reported; nil means discard
//
// Returns:
//   - a feed ready for subscribers
func New(log *slog.Logger) *Feed {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Feed{log: log, subs: make(map[*Sub]struct{})}
}

// Close ends this listener's subscription. It is safe to call more than once,
// which matters because the usual caller closes it in a defer and the feed may
// already have gone.
func (s *Sub) Close() {
	s.closed.Do(func() {
		s.feed.mu.Lock()
		defer s.feed.mu.Unlock()

		// Unsubscribed and closed under the one lock that Publish holds for the
		// whole of its loop. Closing outside it would work by argument, since a
		// listener already deleted can no longer be offered anything, but this
		// way it needs no argument: while this runs, nothing is publishing.
		delete(s.feed.subs, s)
		close(s.frames)
		close(s.events)
	})
}

// Dropped is how many frames this listener was not fast enough to take.
func (s *Sub) Dropped() uint64 { return s.dropped.Load() }

// Events is where this listener's news arrives.
func (s *Sub) Events() <-chan Event { return s.events }

// Frames is where this listener's audio arrives. It is closed when the listener
// is closed.
func (s *Sub) Frames() <-chan Frame { return s.frames }

// Publish offers one frame to every listener, dropping the oldest waiting for
// anybody who cannot keep up.
//
// Dropping the oldest rather than the newest, because this is live audio. A
// frame that has been queued for half a second is half a second of delay that
// nobody asked for, and playing it before the current one only pushes that delay
// further out. Skipping to the present is what gets a listener back to the
// radio.
//
// It never blocks, so the goroutine cutting frames is never held up by a
// listener, and the order listeners are offered a frame in is the order they
// subscribed.
//
// Parameters:
//   - frame: the frame to offer to every listener
func (f *Feed) Publish(frame Frame) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for s := range f.subs {
		s.offer(frame)
	}
}

// PublishEvent offers one piece of news to every listener.
//
// News is dropped rather than queued when a listener is behind, exactly like
// audio. There is nothing here that is worth arriving late, and the alternative
// is a level meter from four seconds ago pushing out the frame that would have
// caught the listener up.
//
// Parameters:
//   - kind: what happened, carried as the event's Kind
//   - payload: what to say about it, carried as the event's Payload
func (f *Feed) PublishEvent(kind string, payload any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	ev := Event{Kind: kind, Payload: payload}
	for s := range f.subs {
		select {
		case s.events <- ev:
		default:
		}
	}
}

// Subscribe adds a listener and returns its view.
//
// depth is how many frames may be waiting for it before the oldest are dropped.
// It is small for everybody: a listener behind by more than a fraction of a
// second is a listener whose audio is late, and the fix for late audio is to
// skip to the present rather than to work through the past.
//
// Parameters:
//   - depth: how many frames and events may queue for this listener; values
//     below one are raised to one
//
// Returns:
//   - the listener's view of the feed
func (f *Feed) Subscribe(depth int) *Sub {
	if depth < 1 {
		depth = 1
	}

	s := &Sub{
		feed:   f,
		frames: make(chan Frame, depth),
		events: make(chan Event, depth),
	}

	f.mu.Lock()
	f.subs[s] = struct{}{}
	f.mu.Unlock()

	return s
}

// Subscribers is how many listeners there are, which is what decides whether
// the sound card needs to be open at all.
func (f *Feed) Subscribers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

// drop counts one frame this listener was not fast enough to take, and says so
// the first time it happens to it.
//
// Only the first, because a listener that has fallen behind drops steadily and
// a line per frame would be fifty a second saying the same thing. The first one
// is the event worth having: it carries the moment the audio started being
// late, which is what a maintainer is looking for afterwards, and the running
// total is on the listener already for anybody who wants the rest.
//
// The caller holds the feed's lock, which is what makes reading the count back
// straight after adding to it safe: nothing else is publishing to this listener
// while that happens.
func (s *Sub) drop() {
	if s.dropped.Add(1) == 1 {
		s.feed.log.Warn("dropping audio frames for a listener that is behind",
			"depth", cap(s.frames))
	}
}

// offer puts one frame in front of this listener without waiting for it.
//
// The caller holds the feed's lock, which is what makes the send safe: Close
// takes the same lock before it closes the channel, so a channel cannot be
// closed between the check and the send.
//
// Parameters:
//   - frame: the frame to offer
func (s *Sub) offer(frame Frame) {
	select {
	case s.frames <- frame:
		return
	default:
	}

	// Full. Make room by taking the oldest out, then try once more and give up
	// rather than insist: a listener this far behind has already lost frames,
	// and spinning here would hold up every other listener to no purpose.
	select {
	case <-s.frames:
		s.drop()
	default:
	}
	select {
	case s.frames <- frame:
	default:
		s.drop()
	}
}
