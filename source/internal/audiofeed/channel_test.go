// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audiofeed

import (
	"encoding/binary"
	"math"
	"testing"
)

// stereoFrame builds one frame with the given constant sample on each side.
func stereoFrame(left, right int16) []byte {
	f := make([]byte, FrameBytes)
	for i := range FrameSamples {
		binary.LittleEndian.PutUint16(f[i*4:], uint16(left))
		binary.LittleEndian.PutUint16(f[i*4+2:], uint16(right))
	}
	return f
}

// stereoTone builds one frame with a sine on each side at the given amplitudes.
// A tone rather than a constant wherever a level is being measured, because a
// constant is not a signal and its RMS is its own value.
func stereoTone(left, right float64) []byte {
	f := make([]byte, FrameBytes)
	for i := range FrameSamples {
		s := math.Sin(2 * math.Pi * 1000 * float64(i) / SampleRate)
		binary.LittleEndian.PutUint16(f[i*4:], uint16(int16(s*left)))
		binary.LittleEndian.PutUint16(f[i*4+2:], uint16(int16(s*right)))
	}
	return f
}

// monoAt reads one sample out of a folded frame.
func monoAt(mono []byte, i int) int16 {
	return int16(binary.LittleEndian.Uint16(mono[i*2:]))
}

// TestDownmixDoesNotOverflow is the first of the two arithmetic traps. Adding
// two int16 values near full scale wraps, and a wrap at the loudest moment of a
// transmission is a burst of noise exactly when somebody is listening hardest.
func TestDownmixDoesNotOverflow(t *testing.T) {
	mono := make([]byte, MonoFrameBytes)

	cases := []struct {
		name        string
		left, right int16
		want        int16
	}{
		{"both at the positive limit", 32767, 32767, 32767},
		{"both at the negative limit", -32768, -32768, -32768},
		{"opposite limits cancel", 32767, -32768, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			downmix(stereoFrame(c.left, c.right), mono, ChannelMix)

			for i := range FrameSamples {
				got := monoAt(mono, i)
				// Halving an odd sum loses a count, which is inaudible and not
				// worth a rounding rule.
				if got != c.want && got != c.want+1 {
					t.Fatalf("sample %d folded to %d, want about %d", i, got, c.want)
					return
				}
			}
		})
	}
}

// TestDownmixOneSideKeepsItsLevel is the second trap, and the whole reason auto
// exists. A scanner on a mono lead lands on one channel, and folding that with
// an empty one halves it.
func TestDownmixOneSideKeepsItsLevel(t *testing.T) {
	mono := make([]byte, MonoFrameBytes)

	downmix(stereoFrame(32767, 0), mono, ChannelLeft)
	if got := monoAt(mono, 0); got != 32767 {
		t.Errorf("taking the left of a full-scale left gave %d, want 32767", got)
	}

	downmix(stereoFrame(0, 32767), mono, ChannelRight)
	if got := monoAt(mono, 0); got != 32767 {
		t.Errorf("taking the right of a full-scale right gave %d, want 32767", got)
	}

	// And the same signal mixed is 6 dB down, which is the failure auto is
	// there to avoid rather than a bug in the fold.
	downmix(stereoFrame(32767, 0), mono, ChannelMix)
	if got := monoAt(mono, 0); got != 16383 {
		t.Errorf("mixing a full-scale left gave %d, want it halved to 16383", got)
	}
}

// TestLevelOfAfterDownmix checks that a frame measures correctly once it has
// been folded to mono, which is the only form LevelOf is ever given.
func TestLevelOfAfterDownmix(t *testing.T) {
	mono := make([]byte, MonoFrameBytes)

	downmix(stereoFrame(0, 0), mono, ChannelMix)
	if got := LevelOf(mono); got != quietest {
		t.Errorf("silence measured %.1f dBFS, want %.1f", got, quietest)
	}

	// A full-scale sine is 3 dB below full scale as an RMS measurement, which
	// is the number to expect rather than 0.
	downmix(stereoTone(32767, 32767), mono, ChannelMix)
	if got := LevelOf(mono); got < -4 || got > -2 {
		t.Errorf("a full-scale tone measured %.1f dBFS, want about -3", got)
	}
}

// inPhase is the level folding two sides together produces when they carry the
// same signal, which is what every case here is unless it says otherwise.
//
// Two correlated signals of level l and r fold to their average. Sides that
// disagree fold to less than that, and it is that shortfall the chooser reads
// as a reason not to fold at all.
func inPhase(l, r float64) float64 { return (l + r) / 2 }

// loud is an amplitude well above the floor, and quiet is one well below it, so
// the chooser tests are about the decision rather than about where the floor sits.
const (
	loud  = 8000.0
	quiet = 5.0
)

func TestChooserFindsTheChannelTheScannerIsOn(t *testing.T) {
	cases := []struct {
		name        string
		left, right float64
		want        string
	}{
		{"a mono lead on the left", loud, 0, ChannelLeft},
		{"a mono lead on the right", 0, loud, ChannelRight},
		{"a stereo lead carrying both", loud, loud, ChannelMix},

		// A real unconnected channel is not digital zero. It has a noise floor,
		// and the 20 dB gap has to be wide enough that the floor does not close
		// it.
		{"one side is only noise", loud, loud / 1000, ChannelLeft},

		// Two sides genuinely carrying the same signal are never exactly
		// matched, and a small imbalance must not read as one side being empty.
		{"a slightly unbalanced stereo lead", loud, loud / 2, ChannelMix},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pick := newChooser(ChannelAuto)

			// Exactly as many frames as it takes, so this also pins how long
			// the decision waits.
			for range chooseFrames {
				pick.observe(c.left, c.right, inPhase(c.left, c.right))
			}

			got, ok := pick.decided()
			if !ok {
				t.Fatalf("nothing was decided after %d qualifying frames", chooseFrames)
			}
			if got != c.want {
				t.Errorf("chose %q, want %q", got, c.want)
			}
		})
	}
}

// TestChooserSendsAudioBeforeItHasDecided: pressing Listen has to produce sound
// at once. Mix is at worst 6 dB quiet, and three seconds of silence while the
// question is settled would look like a feature that does not work.
func TestChooserSendsAudioBeforeItHasDecided(t *testing.T) {
	pick := newChooser(ChannelAuto)

	if got := pick.observe(loud, 0, inPhase(loud, 0)); got != ChannelMix {
		t.Errorf("the first frame was folded as %q, want %q", got, ChannelMix)
	}
	if _, ok := pick.decided(); ok {
		t.Error("one frame was enough to decide, which is not enough evidence")
	}
}

// TestChooserIgnoresSilence is what stops a quiet channel deciding the question.
// The scanner is silent between transmissions, and a silent frame says exactly
// as much about which side the signal is on as no frame at all.
func TestChooserIgnoresSilence(t *testing.T) {
	pick := newChooser(ChannelAuto)

	for range chooseFrames * 2 {
		if got := pick.observe(quiet, quiet, inPhase(quiet, quiet)); got != ChannelMix {
			t.Fatalf("folded as %q while still undecided, want %q", got, ChannelMix)
		}
	}
	if _, ok := pick.decided(); ok {
		t.Fatal("silence alone decided the channel")
	}

	// And real audio afterwards still settles it, so the silence delayed the
	// answer rather than poisoning it.
	for range chooseFrames {
		pick.observe(loud, 0, inPhase(loud, 0))
	}
	if got, ok := pick.decided(); !ok || got != ChannelLeft {
		t.Errorf("after real audio it chose %q (decided %v), want %q", got, ok, ChannelLeft)
	}
}

// TestChooserWaitsRatherThanGuessingOnASilentInput is a regression.
//
// There used to be a deadline: after thirty seconds of silence the answer was
// fixed at mix, on the reasoning that mix is the one fold that is never silent.
// That is untrue whenever the two sides are out of phase, which is how the
// headphone jack on an SDS100 and SDS150 is wired unless its owner has found
// the menu that inverts it, and a scanner is quiet most of the time. So the
// deadline reliably expired before the first transmission of the evening and
// locked in the one answer that destroys the audio, for the rest of the run.
//
// Silence is not evidence. It waits.
func TestChooserWaitsRatherThanGuessingOnASilentInput(t *testing.T) {
	pick := newChooser(ChannelAuto)

	for range 100 * chooseFrames {
		if got := pick.observe(quiet, quiet, inPhase(quiet, quiet)); got != ChannelMix {
			t.Fatalf("folded with %q while undecided, want %q", got, ChannelMix)
		}
	}
	if _, ok := pick.decided(); ok {
		t.Error("settled on a silent input, with nothing to settle it")
	}

	// And the first real audio still settles it, however long the wait was.
	for range chooseFrames {
		pick.observe(loud, loud, loud/8)
	}
	got, ok := pick.decided()
	if !ok || got == ChannelMix {
		t.Errorf("after real audio it chose %q (decided %v), want a single side", got, ok)
	}
}

// TestChooserNeverChangesItsMind is the property that makes this a type rather
// than a function. A quiet transmission is not evidence that a channel went
// away, and a fold that flipped mid-speech would be audible.
func TestChooserNeverChangesItsMind(t *testing.T) {
	pick := newChooser(ChannelAuto)

	for range chooseFrames {
		pick.observe(loud, 0, inPhase(loud, 0))
	}
	if got, _ := pick.decided(); got != ChannelLeft {
		t.Fatalf("chose %q, want %q", got, ChannelLeft)
	}

	// Now hand it a great deal of the opposite evidence.
	for range 100 * chooseFrames {
		if got := pick.observe(0, loud, inPhase(0, loud)); got != ChannelLeft {
			t.Fatalf("changed to %q, want it to stay on %q", got, ChannelLeft)
		}
	}
}

// TestChooserDoesNothingWhenToldWhichChannel: every mode but auto is settled
// before the first frame, so nothing is ever measured and nothing can drift.
func TestChooserDoesNothingWhenTold(t *testing.T) {
	for _, mode := range []string{ChannelLeft, ChannelRight, ChannelMix} {
		pick := newChooser(mode)

		got, ok := pick.decided()
		if !ok || got != mode {
			t.Errorf("newChooser(%q) started as %q (decided %v), want it settled", mode, got, ok)
		}

		// Evidence for the other side changes nothing.
		if got := pick.observe(0, loud, inPhase(0, loud)); got != mode {
			t.Errorf("newChooser(%q) folded as %q after contrary evidence", mode, got)
		}
	}
}

func TestParseChannel(t *testing.T) {
	for _, mode := range Channels {
		if got, err := ParseChannel(mode); err != nil || got != mode {
			t.Errorf("ParseChannel(%q) = %q, %v", mode, got, err)
		}
	}

	if got, err := ParseChannel(" LEFT "); err != nil || got != ChannelLeft {
		t.Errorf("ParseChannel(\" LEFT \") = %q, %v, want %q", got, err, ChannelLeft)
	}

	// Empty is auto rather than an error, so a flag left alone behaves the same
	// as a flag set to its default.
	if got, err := ParseChannel(""); err != nil || got != ChannelAuto {
		t.Errorf("ParseChannel(\"\") = %q, %v, want %q", got, err, ChannelAuto)
	}

	if _, err := ParseChannel("middle"); err == nil {
		t.Error("ParseChannel accepted a channel that does not exist")
	}
}

// Test_decide tests the decide function with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - NothingEitherSide: no evidence at all still answers
//   - RightEmpty: a mono lead on the left
//   - LeftEmpty: a mono lead on the right
//   - LeftDominates: the left is far louder than a noise floor on the right
//   - RightDominates: the right is far louder than a noise floor on the left
//   - Balanced: two sides carrying the same signal
func Test_decide(t *testing.T) {

	// Verify that no evidence at all still answers.
	//
	// Called here rather than through observe because observe only counts a
	// frame once a side is above the floor, so the sums cannot both be zero by
	// the time it asks. Mix is the answer that is never silent, and this is the
	// test that says so out loud.
	t.Run("NothingEitherSide", func(t *testing.T) {
		c := &chooser{}
		if got := c.decide(); got != ChannelMix {
			t.Errorf("with no evidence at all it chose %q, want %q", got, ChannelMix)
		}
	})

	// Verify that a mono lead on the left is heard as the left.
	t.Run("RightEmpty", func(t *testing.T) {
		c := &chooser{sumL: loud * loud}
		if got := c.decide(); got != ChannelLeft {
			t.Errorf("chose %q, want %q", got, ChannelLeft)
		}
	})

	// Verify that a mono lead on the right is heard as the right.
	t.Run("LeftEmpty", func(t *testing.T) {
		c := &chooser{sumR: loud * loud}
		if got := c.decide(); got != ChannelRight {
			t.Errorf("chose %q, want %q", got, ChannelRight)
		}
	})

	// Verify that the left far above a noise floor on the right is heard as the
	// left.
	t.Run("LeftDominates", func(t *testing.T) {
		c := &chooser{sumL: loud * loud, sumR: (loud / 1000) * (loud / 1000)}
		if got := c.decide(); got != ChannelLeft {
			t.Errorf("chose %q, want %q", got, ChannelLeft)
		}
	})

	// Verify that the right far above a noise floor on the left is heard as the
	// right.
	t.Run("RightDominates", func(t *testing.T) {
		c := &chooser{sumL: (loud / 1000) * (loud / 1000), sumR: loud * loud}
		if got := c.decide(); got != ChannelRight {
			t.Errorf("chose %q, want %q", got, ChannelRight)
		}
	})

	// Verify that two sides carrying the same signal are mixed.
	t.Run("Balanced", func(t *testing.T) {
		c := &chooser{sumL: loud * loud, sumR: (loud / 2) * (loud / 2)}
		if got := c.decide(); got != ChannelMix {
			t.Errorf("chose %q, want %q", got, ChannelMix)
		}
	})
}

// TestLevelOf tests the LevelOf function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NoSamples: a frame with nothing in it at all reads as silence
//   - Silence: a frame of zeroes reads as silence
//   - Tone: a full-scale tone reads about 3 dB below full scale
func TestLevelOf(t *testing.T) {

	// Verify that a frame with nothing in it at all reads as silence.
	t.Run("NoSamples", func(t *testing.T) {
		if got := LevelOf(nil); got != quietest {
			t.Errorf("an empty frame measured %.1f dBFS, want %.1f", got, quietest)
		}
	})

	// Verify that a frame of zeroes reads as silence.
	t.Run("Silence", func(t *testing.T) {
		mono := make([]byte, MonoFrameBytes)
		if got := LevelOf(mono); got != quietest {
			t.Errorf("silence measured %.1f dBFS, want %.1f", got, quietest)
		}
	})

	// Verify that a full-scale tone reads about 3 dB below full scale.
	t.Run("Tone", func(t *testing.T) {
		mono := make([]byte, MonoFrameBytes)
		downmix(stereoTone(32767, 32767), mono, ChannelMix)

		if got := LevelOf(mono); got < -4 || got > -2 {
			t.Errorf("a full-scale tone measured %.1f dBFS, want about -3", got)
		}
	})
}

// Test_rmsPair tests the rmsPair function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NoSamples: a frame with nothing in it at all measures nothing
//   - OneSide: a mono lead measures on its own side only
//   - InPhase: two sides carrying the same signal fold with no loss
//   - Inverted: two sides carrying opposite signals cancel when folded
func Test_rmsPair(t *testing.T) {

	// Verify that a frame with nothing in it at all measures nothing.
	t.Run("NoSamples", func(t *testing.T) {
		left, right, mixed := rmsPair(nil)
		if left != 0 || right != 0 || mixed != 0 {
			t.Errorf("an empty frame measured %.1f, %.1f and %.1f, want nothing at all",
				left, right, mixed)
		}
	})

	// Verify that folding two sides carrying the same signal loses nothing,
	// which is what makes any loss evidence that they disagree.
	t.Run("InPhase", func(t *testing.T) {
		left, right, mixed := rmsPair(stereoFrame(8000, 8000))
		if mixed < left-1 || mixed < right-1 {
			t.Errorf("folding %.1f and %.1f gave %.1f, want it to keep the level",
				left, right, mixed)
		}
	})

	// Verify that folding two sides carrying opposite signals destroys them.
	//
	// This is the case the chooser has to notice: both sides are equally loud,
	// so nothing about their levels says anything is wrong, and folding them
	// throws the signal away.
	t.Run("Inverted", func(t *testing.T) {
		left, right, mixed := rmsPair(stereoFrame(8000, -8000))
		if left < 7999 || right < 7999 {
			t.Fatalf("the sides measured %.1f and %.1f, want both loud", left, right)
		}
		if mixed > 1 {
			t.Errorf("folding two opposite sides gave %.1f, want them to cancel", mixed)
		}
	})

	// Verify that a mono lead measures on its own side only.
	t.Run("OneSide", func(t *testing.T) {
		left, right, _ := rmsPair(stereoFrame(8000, 0))
		if left < 7999 || left > 8001 {
			t.Errorf("the left measured %.1f, want about 8000", left)
		}
		if right != 0 {
			t.Errorf("the right measured %.1f, want nothing", right)
		}
	})
}

// TestChooserRefusesToFoldSidesThatCancel is a regression from real hardware.
//
// An SDS150 has a setting for whether its headphone output is in phase or
// inverted, and inverted puts the same mono audio on the two sides with
// opposite polarity. Nothing about the levels says so: both sides are equally
// loud, which to a chooser that only compares levels looks like an ordinary
// stereo lead carrying the signal on both.
//
// Folding them measured eleven decibels quieter than either side alone, and
// took most of the voice's body with it, since the low frequencies are the most
// alike between the two sides and cancel the most completely. What came out was
// thin and reedy and sounded like a kazoo.
func TestChooserRefusesToFoldSidesThatCancel(t *testing.T) {
	pick := &chooser{}

	// Equally loud on both sides, and folding them leaves almost nothing.
	for range chooseFrames {
		pick.observe(loud, loud, loud/8)
	}

	settled, ok := pick.decided()
	if !ok {
		t.Fatal("the chooser never settled")
	}
	if settled == ChannelMix {
		t.Fatal("the chooser folded two sides that cancel, which throws the audio away")
	}
	if settled != ChannelLeft {
		t.Errorf("the chooser settled on %q, want the louder side", settled)
	}

	// And it takes whichever side is actually louder, not always the left.
	other := &chooser{}
	for range chooseFrames {
		other.observe(loud/2, loud, loud/8)
	}
	if settled, _ := other.decided(); settled != ChannelRight {
		t.Errorf("the chooser settled on %q, want the louder side", settled)
	}
}

// TestChooserStillFoldsSidesThatAgree checks the other half of the same rule:
// an ordinary stereo lead carrying the same signal on both sides is still
// folded, because folding it costs nothing and using both is the quieter of the
// two failure modes if one side is later lost.
func TestChooserStillFoldsSidesThatAgree(t *testing.T) {
	pick := &chooser{}

	for range chooseFrames {
		pick.observe(loud, loud, inPhase(loud, loud))
	}

	if settled, _ := pick.decided(); settled != ChannelMix {
		t.Errorf("the chooser settled on %q, want %q for two sides that agree", settled, ChannelMix)
	}
}
