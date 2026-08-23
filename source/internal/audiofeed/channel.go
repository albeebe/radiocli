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
	default:
		return ChannelMix
	}
}

// decided returns the answer and whether there is one yet.
//
// Returns:
//   - the settled channel mode, empty until there is one
//   - whether the choice has been made
func (c *chooser) decided() (string, bool) {
	return c.settled, c.settled != ""
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
//
// Returns:
//   - the channel mode to fold this frame with: the settled answer, or
//     ChannelMix while there is not one yet
func (c *chooser) observe(left, right float64) string {
	if c.settled != "" {
		return c.settled
	}

	c.seen++
	if left > silenceFloor || right > silenceFloor {
		c.sumL += left * left
		c.sumR += right * right
		c.qualified++
	}

	switch {
	case c.qualified >= chooseFrames:
		c.settled = c.decide()
	case c.seen >= giveUpFrames:
		c.settled = ChannelMix
	}

	if c.settled != "" {
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
func rmsPair(stereo []byte) (left, right float64) {
	var sumL, sumR float64
	pairs := len(stereo) / 4

	for i := range pairs {
		l := float64(int16(binary.LittleEndian.Uint16(stereo[i*4:])))
		r := float64(int16(binary.LittleEndian.Uint16(stereo[i*4+2:])))
		sumL += l * l
		sumR += r * r
	}

	if pairs == 0 {
		return 0, 0
	}
	return math.Sqrt(sumL / float64(pairs)), math.Sqrt(sumR / float64(pairs))
}
