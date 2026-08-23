// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audiofeed

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// ParseChannel checks a channel mode and returns it in its canonical spelling.
//
// Parameters:
//   - mode: the channel mode to check; case and surrounding space are ignored,
//     and empty means ChannelAuto
//
// Returns:
//   - the canonical spelling of the mode
//   - error if the mode is not one of Channels
func ParseChannel(mode string) (string, error) {
	want := strings.ToLower(strings.TrimSpace(mode))
	if want == "" {
		return ChannelAuto, nil
	}
	for _, known := range Channels {
		if want == known {
			return known, nil
		}
	}
	return "", fmt.Errorf("there is no channel called %q: it is one of %s",
		mode, strings.Join(Channels, ", "))
}

// newChooser returns a chooser for the mode asked for.
//
// Parameters:
//   - mode: the channel mode to fold with; anything but ChannelAuto is
//     settled before the first frame arrives
//
// Returns:
//   - a chooser ready for its first frame
func newChooser(mode string) *chooser {
	c := &chooser{}
	if mode != ChannelAuto {
		c.settled = mode
	}
	return c
}

// dbfs turns a sample-unit level into decibels below full scale.
//
// Parameters:
//   - rms: a level in sample units
//
// Returns:
//   - the level in dBFS, never lower than quietest
func dbfs(rms float64) float64 {
	if rms <= 0 {
		return quietest
	}
	db := 20 * math.Log10(rms/32768)
	return max(db, quietest)
}

// decide reads the accumulated evidence once there is enough of it.
//
// Returns:
//   - ChannelLeft or ChannelRight when one side dominates, ChannelMix otherwise
func (c *chooser) decide() string {
	switch {
	case c.sumR == 0 && c.sumL == 0:
		// Cannot happen: a frame only counts once a side is above the floor.
		// Answered anyway, because mix is the mode that is never silent and
		// this is not worth a panic.
		return ChannelMix
	case c.sumR == 0:
		return ChannelLeft
	case c.sumL == 0:
		return ChannelRight
	}

	// Ten rather than twenty in front of the logarithm because these are sums
	// of squares, which are already in the units power is measured in.
	diff := 10 * math.Log10(c.sumL/c.sumR)
	switch {
	case diff > dominanceDB:
		return ChannelLeft
	case diff < -dominanceDB:
		return ChannelRight
	}

	// Both sides carry the signal, which is not on its own a reason to fold
	// them together. Two sides can be equally loud and still cancel, and this
	// is the case that matters: an SDS150 has a setting for whether its
	// headphone output is in phase or inverted, and inverted puts the same mono
	// audio on the two sides with opposite polarity.
	//
	// Measured on one, folding those cost eleven decibels and took most of the
	// voice's body with it, because the low frequencies are the most alike
	// between the two sides and cancel the most completely. What is left is
	// thin and reedy, and sounds like the speaker is talking through a kazoo.
	//
	// So the fold is judged by what it produces rather than by the levels going
	// into it. If mixing loses more than a little against the louder side on its
	// own, it is destroying the signal and one side is taken instead.
	if c.sumM > 0 && 10*math.Log10(math.Max(c.sumL, c.sumR)/c.sumM) > cancelDB {
		// Worth saying out loud. Taking one side recovers the level, but the
		// owner can fix it properly in one menu and have it right for
		// everything they plug the radio into, not just this tool.
		c.why = ReasonOutOfPhase
		if c.sumL >= c.sumR {
			return ChannelLeft
		}
		return ChannelRight
	}
	return ChannelMix
}

// decided returns the answer and whether there is one yet.
//
// Returns:
//   - the settled channel mode, empty until there is one
//   - whether the choice has been made
func (c *chooser) decided() (string, bool) {
	return c.settled, c.settled != ""
}

// reason reports why the chooser settled where it did, when that is worth
// telling somebody.
//
// Returns:
//   - ReasonOutOfPhase if the two sides were found to cancel, empty otherwise
func (c *chooser) reason() string {
	return c.why
}

// downmix folds one frame of interleaved stereo into one frame of mono.
//
// stereo must be FrameBytes and mono must be MonoFrameBytes. Both are the
// caller's buffers and neither is allocated here, because this runs fifty times
// a second for the life of the program.
//
// Parameters:
//   - stereo: one frame of interleaved stereo, FrameBytes long
//   - mono: the caller's buffer for the folded frame, MonoFrameBytes long
//   - mode: ChannelLeft or ChannelRight to take one side; anything else mixes
func downmix(stereo, mono []byte, mode string) {
	for i := 0; i < len(mono); i += 2 {
		l := int16(binary.LittleEndian.Uint16(stereo[i*2:]))
		r := int16(binary.LittleEndian.Uint16(stereo[i*2+2:]))

		var out int16
		switch mode {
		case ChannelLeft:
			out = l
		case ChannelRight:
			out = r
		default:
			// Added as int32 and halved before it is narrowed. Adding two
			// int16 values near full scale overflows, and the wrap turns the
			// loudest moment of a transmission into a burst of noise, which is
			// the one moment somebody is listening hardest.
			out = int16((int32(l) + int32(r)) / 2)
		}

		binary.LittleEndian.PutUint16(mono[i:], uint16(out))
	}
}

// LevelOf measures one frame of mono, in dBFS.
//
// Decibels here rather than sample units because what reads this is either a
// person or a gate: a meter on a page, and the level a transmission is judged
// against. Both are naturally spoken in decibels, and neither cares about the
// ratio arithmetic rmsPair exists for.
//
// It is exported because audio taken from a daemon arrives as samples with no
// level attached, and whatever measures it there has to get the same answer as
// the capture would. Two copies of this arithmetic would be two definitions of
// how loud a frame is, and a gate tuned against one of them would behave
// differently depending on where its audio came from.
//
// Parameters:
//   - mono: one frame of mono samples
//
// Returns:
//   - the frame's RMS level in dBFS, or the quietest value for an empty frame
func LevelOf(mono []byte) float64 {
	var sum float64
	samples := len(mono) / 2

	for i := range samples {
		v := float64(int16(binary.LittleEndian.Uint16(mono[i*2:])))
		sum += v * v
	}

	if samples == 0 {
		return quietest
	}
	return dbfs(math.Sqrt(sum / float64(samples)))
}

// observe takes one frame's measurements and answers how to fold that frame.
//
// Parameters:
//   - left: the left side's RMS in sample units
//   - right: the right side's RMS in sample units
//   - mixed: the RMS the two would have if folded together, which is what says
//     whether folding them is safe
//
// Returns:
//   - the channel mode to fold this frame with: the settled answer, or
//     ChannelMix while there is not one yet
func (c *chooser) observe(left, right, mixed float64) string {
	if c.settled != "" {
		return c.settled
	}

	if left > silenceFloor || right > silenceFloor {
		c.sumL += left * left
		c.sumR += right * right
		c.sumM += mixed * mixed
		c.qualified++
	}

	// Settling happens on evidence and on nothing else. A quiet channel simply
	// stays undecided, folding with mix in the meantime, until something is
	// actually heard.
	//
	// There used to be a deadline here: after thirty seconds of silence the
	// answer was fixed at mix, on the reasoning that mix is the one fold that
	// is never silent. That reasoning was wrong, and expensively so. Mix is
	// near-silent whenever the two sides are out of phase, which is how the
	// headphone jack on an SDS100 and SDS150 is wired unless the owner has
	// found the menu that inverts it. A scanner is quiet most of the time, so
	// the deadline reliably expired before the first transmission of the
	// evening and locked in the one answer that destroys the audio, for the
	// whole run.
	//
	// Waiting costs nothing by comparison. Undecided already folds with mix, so
	// the only thing the deadline ever changed was whether a later transmission
	// was allowed to correct it.
	if c.qualified >= chooseFrames {
		c.settled = c.decide()
		return c.settled
	}
	return ChannelMix
}

// rmsPair measures both sides of one frame of interleaved stereo, in sample
// units rather than decibels.
//
// Sample units because everything that compares these does it as a ratio, and a
// ratio of two linear numbers needs no conversion at all. Decibels are worked
// out once, at the point something is going to read them.
//
// Parameters:
//   - stereo: one frame of interleaved stereo
//
// Returns:
//   - left: the left side's RMS in sample units
//   - right: the right side's RMS in sample units
func rmsPair(stereo []byte) (left, right, mixed float64) {
	var sumL, sumR, sumM float64
	pairs := len(stereo) / 4

	for i := range pairs {
		l := float64(int16(binary.LittleEndian.Uint16(stereo[i*4:])))
		r := float64(int16(binary.LittleEndian.Uint16(stereo[i*4+2:])))
		m := (l + r) / 2
		sumL += l * l
		sumR += r * r
		sumM += m * m
	}

	if pairs == 0 {
		return 0, 0, 0
	}
	n := float64(pairs)
	return math.Sqrt(sumL / n), math.Sqrt(sumR / n), math.Sqrt(sumM / n)
}
