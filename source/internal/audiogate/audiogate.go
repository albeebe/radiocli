// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

// Package audiogate finds the transmissions in a stream of scanner audio.
//
// It takes every frame the sound card produces, plus whatever the radio says
// about what it is receiving, and hands back the transmissions: where each one
// started, the audio it is made of, and where it ended.
//
// # The problem this exists to solve
//
// Recording a scanner means reconciling two sources that nothing synchronises.
// The audio arrives on a sound card. What is being received arrives on the
// serial cable, polled, and therefore always some unknown amount late. Software
// that tries to start a recording the moment the radio says so records a
// transmission whose first syllable is already gone.
//
// The usual answer is to prepend a fixed amount of buffered audio, half a
// second or five, chosen by the user. That answer is wrong in both directions at
// once. Set it short and the opening is still clipped when the news was slow.
// Set it long and every recording carries however much dead air the guess
// overshot by. It cannot be right, because it is a constant standing in for
// something that varies.
//
// This package does not race, and does not guess. It holds the last ten seconds
// of audio and decides nothing in real time. When a transmission is noticed,
// however late, the audio from before it is still in the buffer, so the
// beginning can be found by looking for it: walk backwards to where the energy
// rose out of the noise floor, and start there. The same walk forwards trims the
// tail. A late trigger cannot clip the opening, and a generous one cannot pad
// the file, because neither the trigger nor a constant decides where the file
// begins.
//
// # The division of labour
//
// The radio says that a transmission happened, and what it was. The audio says
// exactly when it started and stopped. Neither is asked to do the other's job,
// which is what makes the timing of the radio's answer stop mattering: a sample
// only has to land somewhere inside the transmission to identify it, and the
// file boundaries never depend on when it arrived.
//
// The one thing the audio genuinely cannot do is tell two transmissions apart
// when they follow each other closely. To the sound card a dispatcher and the
// unit answering are one event with a pause in it, and so is one person pausing
// mid-sentence; nothing in the waveform separates those cases.
//
// The radio does, through its mute, which follows the carrier rather than the
// speech: it stays open through a breath and shuts the moment somebody unkeys.
// So a mute that closes for longer than Options.Hang ends a recording, and a
// change of Activity.Key ends one too. On both questions the audio is overruled,
// because on both it is not equipped to answer.
//
// # The noise floor
//
// Nothing here has a threshold setting. The level that counts as signal is a
// fixed margin above a continuously measured estimate of the noise floor, and
// the floor is measured because it is a property of somebody's cable, dongle and
// volume knob rather than of audio in general. Asking a user for it is asking
// them to measure something they have no instrument for, and getting it wrong
// produces recordings full of nothing or no recordings at all.
//
// The estimate is a low percentile of the recent levels rather than their
// minimum. That distinction is not academic: on a real line input the noise sat
// at -78 dBFS and its quietest single frame in fifteen seconds was -87, so a
// floor taken from the minimum put the trigger nine decibels under the noise
// and the recorder wrote the hiss between transmissions as though it were
// traffic.
//
// # What it does not touch
//
// No files, no sound card, no radio, no clock. Time comes from the frames
// themselves, so a test can drive a whole afternoon of scanner traffic through
// this package in a millisecond and get exactly what the hardware would have
// produced.
package audiogate

import (
	"time"

	"github.com/albeebe/radiocli/internal/audiofeed"
)

// New returns a gate that will apply opts, with any zero field defaulted.
//
// Parameters:
//   - opts: the call model; every field may be left zero for its default
//
// Returns:
//   - *Gate ready to be offered frames
func New(opts Options) *Gate {
	if opts.Hang <= 0 {
		// The default depends on what the gate is being asked to watch, because
		// the hang means a different thing in each mode. See DefaultQuietHang.
		opts.Hang = DefaultHang
		if !opts.RequireRadio {
			opts.Hang = DefaultQuietHang
		}
	}
	if opts.MaxDuration <= 0 {
		opts.MaxDuration = DefaultMaxDuration
	}
	if opts.MinDuration <= 0 {
		opts.MinDuration = DefaultMinDuration
	}

	return &Gate{
		opts: opts,
		ring: make([]audiofeed.Frame, 0, maxRingFrames+1),
	}
}

// Activity records what the radio is doing, as of when it was asked.
//
// It is only recorded here rather than acted on, because everything this gate
// decides is decided against a frame of audio, and frames arrive every twenty
// milliseconds. Acting on the radio's word the instant it arrives would mean
// two paths into the same decisions for no gain.
//
// Parameters:
//   - at: when the radio was asked, which is what the answer describes
//   - a: what it said
func (g *Gate) Activity(at time.Time, a Activity) {
	g.radio, g.radioAt = a, at
}

// Floor reports the current estimate of the noise floor, in dBFS.
//
// It is exposed so the recorder can show it, because an input that is not the
// scanner shows up here first: a floor pinned at the top of its range is a
// microphone in a room, and one at the bottom is a cable delivering digital
// silence.
//
// Returns:
//   - the estimated noise floor in dBFS, or floorMin before any audio has
//     been seen
func (g *Gate) Floor() float64 {
	return g.floor.level()
}

// Flush ends any transmission still open, and reports it.
//
// It is what the recorder calls on the way out, so that stopping part way
// through a transmission still produces a complete recording of the part that
// happened rather than losing it.
//
// Returns:
//   - the events completing the open transmission, or none if nothing was open
func (g *Gate) Flush() []Event {
	if g.tx == nil {
		return nil
	}
	return g.close(ReasonStopped)
}

// Live reports whether a transmission is running right now, for something
// playing audio as it arrives rather than keeping it.
//
// It exists because the events this gate emits are the wrong answer to that
// question, and answering it with them produced audio full of holes. A
// KindStart is held back until the transmission has outlived MinDuration and
// proved itself worth a file, which on the defaults is most of a second, and
// KindAudio is held back further still so that the end of a recording can be
// trimmed. Both are right for a file, which is written once and played later.
// Neither is right for a speaker, where half a second late is not late, it is
// missing: measured against the defaults, a listener following the events lost
// the first 480 ms of every transmission, which in the back and forth of
// dispatch traffic is most of the first word each time.
//
// So it reports the transmission rather than the recording, which is open from
// the moment something opened it. What it deliberately does not do is ask the
// audio directly, though the first version of this did. Offering "or this
// frame is loud" gets the attack down to nothing, and it is wrong for the
// reason Offer gives above: a scanner's own output has more than one noise
// floor, so the static of an open squelch reads as loud against a floor
// measured while the radio was muted. Playing that put fourteen seconds of
// hiss through the speakers between two transmissions of three, which is what
// somebody listening hears as choppy.
//
// The cost is whatever the transmission took to open, which is the honest
// answer either way: with a radio to ask, the radio is asked ten times a
// second, and without one the audio opens it on the first frame above the
// floor.
//
// Parameters:
//   - f: unused, and kept so that a caller asks about the frame it has, rather
//     than about whatever the gate happens to have seen last
//
// Returns:
//   - true if the audio arriving now should be heard
func (g *Gate) Live(f audiofeed.Frame) bool {
	return g.tx != nil
}

// Offer hands one frame to the gate and returns whatever that settled.
//
// Most frames settle nothing and return none. A frame that completes the
// minimum length returns a KindStart followed by every frame back to the onset,
// and a frame that ends a transmission returns the audio held back for trimming
// followed by a KindEnd.
//
// Parameters:
//   - f: the next frame from the feed, in order
//
// Returns:
//   - the events this frame produced, in the order they should be acted on
func (g *Gate) Offer(f audiofeed.Frame) []Event {
	dropped := g.gap(f)

	// Judged before this frame joins the window, so that a frame is never
	// compared against a floor it has itself contributed to. Every frame is
	// measured, transmissions included, because the floor is a minimum over
	// fifteen seconds and speech cannot be the quietest thing in it. That is
	// what lets the estimate recover when the input genuinely changes, rather
	// than being frozen at whatever it was when a transmission opened.
	loud := g.loud(f)
	g.floor.add(f.Level)

	if g.tx == nil {
		// With a radio to ask, only the radio opens a transmission. Audio alone
		// cannot be trusted to say one is happening: the level it would have to
		// clear is derived from a noise floor, and a scanner's own output has
		// more than one of those.
		if !g.opts.RequireRadio && !loud && !g.radio.On {
			g.ring = append(g.ring, f)
			if len(g.ring) > maxRingFrames {
				g.ring = g.ring[1:]
			}
			return nil
		}
		if g.opts.RequireRadio && !g.radio.On {
			g.ring = append(g.ring, f)
			if len(g.ring) > maxRingFrames {
				g.ring = g.ring[1:]
			}
			return nil
		}
		// Something is happening. Find where it actually started, which is
		// somewhere in the audio already held rather than here. This frame is
		// deliberately not in the ring: advance adds it.
		g.open(f, loud)
	}

	return g.advance(f, loud, dropped)
}

// aboveFloor reports whether a frame already in the ring carried signal.
//
// Separate from loud because it is asked about the past, when the floor may
// have been estimated differently. The current estimate is used deliberately:
// it is the better one, having seen more audio.
//
// Parameters:
//   - f: the frame to judge
//
// Returns:
//   - true if the frame is above the floor by the margin
func (g *Gate) aboveFloor(f audiofeed.Frame) bool {
	return g.floor.ready() && f.Level > g.floor.level()+marginDB
}

// add folds one level into the window, taking the oldest back out once the
// window is full.
//
// Parameters:
//   - level: the frame's level in dBFS, clamped here to the range a real noise
//     floor can occupy
func (n *noiseFloor) add(level float64) {
	if n.recent == nil {
		n.recent = make([]uint8, floorFrames)
	}

	if level < floorMin {
		level = floorMin
	}
	if level > floorMax {
		level = floorMax
	}
	bucket := uint8(level - floorMin)

	if n.n == len(n.recent) {
		n.counts[n.recent[n.at]]--
	} else {
		n.n++
	}
	n.recent[n.at] = bucket
	n.counts[bucket]++
	n.at = (n.at + 1) % len(n.recent)
}

// advance adds one frame to the open transmission and applies every rule that
// could confirm or end it.
//
// Parameters:
//   - f: the frame to add
//   - loud: whether it is above the floor
//   - dropped: how many frames went missing before it
//
// Returns:
//   - the events this frame produced
func (g *Gate) advance(f audiofeed.Frame, loud bool, dropped int) []Event {
	var out []Event

	// Whether this frame ends the open transmission is settled before it is
	// added to anything, so that a frame at a boundary belongs to exactly one
	// recording rather than to both.
	switch tx := g.tx; {
	case tx.info.Key != "" && g.radio.On && g.radio.Key != tx.info.Key:
		// The audio heard one transmission and the radio saw two. The radio is
		// right, so the recording is cut here and this frame begins the next.
		out = append(out, g.close(ReasonChannel)...)
		g.open(f, loud)

	case f.At.Sub(tx.info.Start) >= g.opts.MaxDuration:
		out = append(out, g.close(ReasonSplit)...)
		g.open(f, loud)
	}

	tx := g.tx
	tx.info.Dropped += dropped
	tx.pending = append(tx.pending, f)

	if loud && g.counts(tx, f) {
		if tx.firstLoud.IsZero() {
			// The radio opened this into silence and the sound has only now
			// arrived, so the beginning is here rather than back where the
			// radio spoke. A transmission opened by the audio had its start
			// found in the buffer already, and set firstLoud then, so it never
			// reaches this.
			tx.firstLoud = f.At
			trimToOnset(tx)
		}
		if !tx.heard || f.Level > tx.peak {
			tx.heard, tx.peak = true, f.Level
		}

		// Where the recording ends is the last frame that was still speech,
		// which is a question about this transmission's own level rather than
		// about the floor. Hiss clears the floor by eight decibels on a quiet
		// input and would hold the ending open for as long as the radio stayed
		// keyed.
		if f.Level > tx.peak-tailDropDB {
			tx.lastLoud = f.At
		}
	}

	// The radio naming a channel for a transmission that opened on audio alone
	// is an answer, not a disagreement, so it is adopted rather than treated
	// as a change.
	if tx.info.Key == "" && g.radio.On {
		tx.info.Key = g.radio.Key
	}
	if g.radio.On {
		tx.lastRadio = f.At
	}

	switch {
	case tx.confirmed:
		// Nothing to decide; it is already a recording.

	case tx.firstLoud.IsZero():
		// The radio says it is receiving and nothing is coming through the
		// cable. That is a real situation, usually a lead in the wrong socket,
		// and it must not confirm a recording of silence or grow without
		// bound, so what is held rolls like the ring does.
		if len(tx.pending) > maxRingFrames {
			tx.pending = tx.pending[1:]
			tx.info.Start = tx.pending[0].At
		}

	// Confirmation is what creates a file, so it is held off until the
	// transmission has proved itself long enough to be worth one.
	//
	// The length judged is how much audio there is, from the first frame above
	// the floor to the most recent one, rather than how long the transmission
	// has been open. Those differ by the hang time, and measuring the open
	// duration would keep a blip simply because the gate spent two seconds
	// afterwards waiting to be sure it had ended.
	//
	// The second condition is the safety valve: something has to give before
	// pending outgrows the buffer it is sized against.
	// Long enough is not the whole test. A transmission the radio reported into
	// a quiet cable clears the floor on its own wobble and runs for as long as
	// the radio says it is receiving, so length alone writes a file of noise.
	// Its loudest moment has to be loud enough to be somebody talking.
	case !g.hasSpeech(tx):
		// Nothing heard yet that is worth a file. What is held still rolls, so
		// a transmission that never produces speech cannot grow without bound.
		if len(tx.pending) > maxRingFrames {
			tx.pending = tx.pending[1:]
			tx.info.Start = tx.pending[0].At
		}

	case tx.lastLoud.Sub(tx.firstLoud)+frameDuration >= g.opts.MinDuration ||
		len(tx.pending) >= maxRingFrames:
		tx.confirmed = true
		out = append(out, Event{Kind: KindStart, Tx: tx.info})
	}

	if tx.confirmed {
		// Once confirmed, audio is handed out as it ages past Hang, and never
		// past the last speech. The second bound is what makes trimming the end
		// possible at all: a frame handed over has been written, so quiet that
		// might turn out to be the tail has to be kept back until either more
		// speech arrives and releases it, or the transmission ends and it is
		// dropped.
		cutoff := f.At.Add(-g.opts.Hang)
		if end := tx.lastLoud.Add(padDuration); end.Before(cutoff) {
			cutoff = end
		}
		out = append(out, g.release(cutoff)...)
	}

	// The ordinary ending. With a radio to ask, it is the radio going quiet
	// that ends a transmission, and the audio has no vote: audio that stays
	// above the floor after the radio has stopped is the input's own noise, and
	// letting it hold a recording open is how sixteen seconds of hiss gets
	// written as though it were traffic.
	//
	// This is also what separates one speaker from the next. The mute closing
	// is the only keyup the scanner offers, and it is a real one: it follows
	// the carrier, so it shuts between two people on the same channel and does
	// not shut for a pause in the middle of one of them.
	if g.ended(tx, f, loud) {
		out = append(out, g.close(ReasonHang)...)
	}

	return out
}

// close finishes the open transmission, trimming the trailing quiet off it.
//
// A transmission that never reached MinDuration produces nothing at all: no
// KindStart was emitted for it, so there is no file to abandon and nothing to
// tell the caller about. That is the whole reason confirmation is delayed.
//
// Parameters:
//   - reason: why it ended, one of the Reason constants
//
// Returns:
//   - the remaining audio and the KindEnd describing it, or none if the
//     transmission was too short to keep
func (g *Gate) close(reason string) []Event {
	tx := g.tx
	g.tx = nil

	if !tx.confirmed {
		return nil
	}

	// The recording ends a pad after the last audio, not wherever the silence
	// happened to be noticed. Everything after that is dead air the hang time
	// was spent waiting through. What counted as audio was already bounded by
	// the radio, so nothing further is needed here to keep the input's own
	// noise out of the end of the file.
	end := tx.lastLoud.Add(padDuration)
	out := g.releaseFrom(tx, end)

	tx.info.End = end
	tx.info.Reason = reason
	return append(out, Event{Kind: KindEnd, Tx: tx.info})
}

// counts reports whether audio arriving now belongs to the transmission.
//
// With the radio as the authority it does not, once the radio has stopped.
// Otherwise the input's own noise goes on advancing the last-heard time after
// the transmission ended, and a burst of a few hundred milliseconds grows past
// the minimum length on nothing but hiss, which is the whole failure this mode
// exists to prevent.
//
// The slack is what keeps the end of a real transmission. The radio is asked
// several times a second rather than continuously, so the moment it stopped is
// only known to within one interval, and cutting at the last poll that said yes
// would clip that much off every recording.
//
// Parameters:
//   - tx: the transmission being assembled
//   - f: the frame that has just arrived
//
// Returns:
//   - true if this audio is part of the transmission
func (g *Gate) counts(tx *transmission, f audiofeed.Frame) bool {
	if !g.opts.RequireRadio {
		return true
	}
	return g.radio.On || f.At.Sub(tx.lastRadio) <= radioSlack
}

// ended reports whether the open transmission is over.
//
// With the radio as the authority, this is the whole of it: the mute closed and
// has stayed closed for the hang time. Because the mute follows the carrier
// rather than the speech, that boundary is the keyup boundary, and a dispatcher
// and the unit answering land in separate recordings without the audio being
// asked to guess where one of them stopped. The obvious idea is deliberately
// not used here, because it is wrong: a silence inside the audio is not a
// boundary, since somebody pausing mid-sentence is still transmitting and
// cutting there invents a split that never happened.
//
// Parameters:
//   - tx: the transmission being assembled
//   - f: the frame just added
//   - loud: whether that frame was above the floor
//
// Returns:
//   - true if the transmission has finished
func (g *Gate) ended(tx *transmission, f audiofeed.Frame, loud bool) bool {
	if g.opts.RequireRadio {
		return !g.radio.On && f.At.Sub(tx.lastRadio) >= g.opts.Hang
	}
	return !loud && !g.radio.On && f.At.Sub(tx.lastLoud) >= g.opts.Hang
}

// gap reports how many frames went missing before f, and remembers where the
// numbering has got to.
//
// The feed numbers frames from the sound card rather than counting what it
// managed to deliver, so a gap in the numbering is audio that existed and was
// lost. Concatenating across one silently shortens the recording and moves
// everything after it earlier, which nothing downstream could detect.
//
// Parameters:
//   - f: the frame that has just arrived
//
// Returns:
//   - how many frames are missing between the last one and this one
func (g *Gate) gap(f audiofeed.Frame) int {
	if !g.seqSet {
		g.seqSet, g.seq = true, f.Seq
		return 0
	}

	previous := g.seq
	g.seq = f.Seq

	// Numbering that did not move forward is not loss. It happens when the
	// sound card is reopened and the count starts again, and the subtraction
	// has to be guarded rather than allowed to go negative: these are unsigned,
	// so a backwards step wraps to something near four billion instead.
	if f.Seq <= previous {
		return 0
	}
	return int(f.Seq - previous - 1)
}

// hasSpeech reports whether the transmission has held anything loud enough to
// be somebody talking.
//
// It is the difference between a recording and a file of noise. The floor is
// where the input sits with nothing coming through, and a transmission the
// radio reported into a quiet cable never rises meaningfully above it, so the
// loudest moment is what settles whether there was ever a voice.
//
// Measured against the floor rather than an absolute level, because how loud
// speech lands depends on the cable and the sound card and nothing here can
// know either.
//
// A floor that is not quiet is not answered against at all. The estimate only
// means "where the noise is" when most of the window it was taken from was
// noise, and a busy channel fills that window with speech instead. Reading a
// high estimate as a high noise floor would reject the traffic that raised it.
//
// Parameters:
//   - tx: the transmission being assembled
//
// Returns:
//   - true if the loudest frame so far is far enough above the floor, or if
//     the floor is too high to be judging anything by
func (g *Gate) hasSpeech(tx *transmission) bool {
	if !g.floor.ready() {
		return false
	}
	floor := g.floor.level()
	if floor > floorTrusted {
		return true
	}
	return tx.heard && tx.peak > floor+speechMarginDB
}

// key returns the radio's identity for whatever is being received, if it has
// said anything.
//
// Returns:
//   - the current Activity.Key, or empty if the radio is not receiving
func (g *Gate) key() string {
	if !g.radio.On {
		return ""
	}
	return g.radio.Key
}

// level reports the noise floor: the level below which floorPercentile of the
// recent audio sits.
//
// Returns:
//   - the noise floor in dBFS, or floorMin before any audio has been measured
func (n *noiseFloor) level() float64 {
	if n.n == 0 {
		return floorMin
	}

	// Round up, so the answer is a level at least this share of the window is
	// at or below rather than one it merely approaches.
	want := (n.n*floorPercentile + 99) / 100

	// The walk always stops at a bucket, because the counts add up to n.n and
	// want is never more than that. Written to fall out of the loop with the
	// answer rather than to return from inside it, so there is no branch after
	// it that nothing can reach.
	bucket, seen := 0, 0
	for i, count := range n.counts {
		bucket = i
		if seen += count; seen >= want {
			break
		}
	}
	return floorMin + float64(bucket)
}

// loud reports whether f is above the noise floor by enough to be signal.
//
// Parameters:
//   - f: the frame to judge
//
// Returns:
//   - true if the frame carries signal rather than the floor
func (g *Gate) loud(f audiofeed.Frame) bool {
	if !g.floor.ready() {
		// Nothing has been measured yet, so there is no floor to be above.
		// Treating the very first frame as signal would open a transmission on
		// whatever the card happened to hand over first.
		return false
	}
	return f.Level > g.floor.level()+marginDB
}

// open starts a transmission, reaching back into the ring to find where the
// audio it is made of actually began.
//
// This is the part the whole package exists for. The frame passed in is where
// something was noticed, which is not where the transmission started: the radio
// may have taken a moment to say so, and the audio may have been rising for a
// second before anything looked. So the ring is walked backwards over every
// frame that was already above the floor, and the recording begins a pad before
// the first one of them.
//
// When the trigger was the radio rather than the audio, there is nothing above
// the floor to walk back over, because audio above the floor would have opened
// the transmission itself. Such a transmission starts out having heard nothing,
// and its beginning is found again by trimToOnset when the sound finally
// arrives.
//
// Parameters:
//   - f: the frame that triggered the transmission, which is not part of the
//     ring and is added by the caller
//   - loud: whether that frame was above the floor, which is what distinguishes
//     a transmission triggered by audio from one triggered by the radio
func (g *Gate) open(f audiofeed.Frame, loud bool) {
	// Walk back over audio that was already above the floor. What stops the
	// walk is the last frame that was still at the floor, which is the silence
	// the transmission began out of.
	//
	// It is bounded, because the only thing it compensates for is the lag
	// between a transmission starting and anything noticing, which is a
	// fraction of a second. A floor measured a little low would otherwise let
	// the walk run back through the whole buffer and put all of it at the front
	// of the recording.
	start, limit := len(g.ring), len(g.ring)-lookBackFrames
	for start > 0 && start > limit && g.aboveFloor(g.ring[start-1]) {
		start--
	}

	// Then a pad further back, because a syllable rises out of the noise
	// rather than appearing, and the frames where it was still rising belong
	// to it.
	start -= padFrames
	if start < 0 {
		start = 0
	}

	tx := &transmission{
		info:     Transmission{Start: f.At, Key: g.key()},
		lastLoud: f.At,
	}
	if loud {
		// Opened by the audio, so the walk above has already settled where the
		// transmission began and advance must not go looking again.
		tx.firstLoud = f.At
	}
	if start < len(g.ring) {
		tx.pending = append(tx.pending, g.ring[start:]...)
		tx.info.Start = tx.pending[0].At
	}

	g.tx = tx
	g.ring = g.ring[:0]
}

// ready reports whether any audio has been measured yet.
//
// Returns:
//   - true once there is a floor to judge a frame against
func (n *noiseFloor) ready() bool {
	return n.n > 0
}

// release hands out every pending frame older than cutoff.
//
// Parameters:
//   - cutoff: frames at or after this are kept back
//
// Returns:
//   - a KindAudio event for each frame released
func (g *Gate) release(cutoff time.Time) []Event {
	return g.releaseFrom(g.tx, cutoff)
}

// releaseFrom hands out every frame of tx pending before cutoff.
//
// Parameters:
//   - tx: the transmission to take frames from
//   - cutoff: frames at or after this are kept back
//
// Returns:
//   - a KindAudio event for each frame released
func (g *Gate) releaseFrom(tx *transmission, cutoff time.Time) []Event {
	n := 0
	for n < len(tx.pending) && tx.pending[n].At.Before(cutoff) {
		n++
	}
	if n == 0 {
		return nil
	}

	out := make([]Event, n)
	for i, f := range tx.pending[:n] {
		out[i] = Event{Kind: KindAudio, Frame: f}
	}
	tx.pending = tx.pending[n:]
	return out
}

// trimToOnset drops the silence a radio-triggered transmission was waiting
// through, now that the audio has arrived.
//
// The last frame held is the first one above the floor, so everything before it
// but the pad is the gap between the radio saying it was receiving and the
// sound reaching the sound card. Keeping it would put that gap at the front of
// the recording, which is the padding this package exists to avoid.
//
// Parameters:
//   - tx: the transmission to trim, whose last pending frame is the first loud
//     one
func trimToOnset(tx *transmission) {
	start := len(tx.pending) - 1 - padFrames
	if start < 0 {
		start = 0
	}
	tx.pending = tx.pending[start:]
	tx.info.Start = tx.pending[0].At
}
