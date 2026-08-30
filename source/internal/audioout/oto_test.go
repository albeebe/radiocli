// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/27/2026

//go:build cgo && (darwin || windows)

package audioout

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

// fakeOtoPlayer stands in for oto's player, which cannot be made without a
// sound card.
type fakeOtoPlayer struct {
	closeErr error // What Close answers with
	closes   int   // How many times Close was called
	plays    int   // How many times Play was called
}

// Close counts the teardown rather than doing one.
func (f *fakeOtoPlayer) Close() error { f.closes++; return f.closeErr }

// Play counts the start rather than doing one.
func (f *fakeOtoPlayer) Play() { f.plays++ }

// useStartOto points openOto at a fake for the length of one test.
//
// Parameters:
//   - t: the test to restore the real one at the end of
//   - fn: what openOto should reach the hardware through instead
func useStartOto(t *testing.T, fn func(time.Duration, io.Reader) (otoPlayer, error)) {
	t.Helper()
	was := startOto
	startOto = fn
	t.Cleanup(func() { startOto = was })
}

// TestOtoPlaybackClose tests the otoPlayback.Close method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - AlreadyGivenBack: closing a playback holding no player does nothing
//   - LivePlayer: the player is closed and a second Close is harmless
func TestOtoPlaybackClose(t *testing.T) {
	// Verify that closing a playback whose player is already gone is quiet and
	// safe, which is what makes a second Close harmless.
	t.Run("AlreadyGivenBack", func(t *testing.T) {
		p := &otoPlayback{}
		p.Close()

		if p.player != nil {
			t.Error("Close left a player behind on a playback that had none")
		}
	})

	// Verify that a live player is closed exactly once however many times Close
	// is called, since the commands close on the way out of more than one path.
	t.Run("LivePlayer", func(t *testing.T) {
		// An error from Close is deliberately ignored rather than reported, so
		// a failing one has to be as harmless as a working one.
		f := &fakeOtoPlayer{closeErr: errors.New("the device had already gone")}
		p := &otoPlayback{player: f}

		p.Close()
		p.Close()

		if f.closes != 1 {
			t.Errorf("the player was closed %d times, want once", f.closes)
		}
		if p.player != nil {
			t.Error("Close left the player behind")
		}
	})
}

// Test_fillReaderRead tests the fillReader.Read method with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - FillsAndNeverEnds: the whole buffer is filled and the stream never ends
func Test_fillReaderRead(t *testing.T) {
	// Verify that the buffer comes back filled and that nothing about the
	// return says the audio has finished, since a short read would stop the
	// player for good.
	t.Run("FillsAndNeverEnds", func(t *testing.T) {
		r := fillReader{fill: func(out []byte) {
			for i := range out {
				out[i] = 7
			}
		}}

		buf := make([]byte, 8)
		n, err := r.Read(buf)

		if err != nil {
			t.Errorf("Read gave %v, want nil so the player keeps going", err)
		}
		if n != len(buf) {
			t.Errorf("Read filled %d of %d bytes; a short read ends the stream", n, len(buf))
		}
		if !bytes.Equal(buf, bytes.Repeat([]byte{7}, 8)) {
			t.Errorf("Read gave %v, want the whole buffer filled", buf)
		}
	})
}

// Test_openOto tests the openOto function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - ReportsAFailureToOpen: the library's own failure is passed on
//   - StartsPlaying: the player is started, reads from the ring, and asks the
//     device for the cushion openOto worked out
func Test_openOto(t *testing.T) {
	// Verify that a machine with no audio at all reports it rather than
	// returning a playback that will never play.
	t.Run("ReportsAFailureToOpen", func(t *testing.T) {
		failed := errors.New("no audio device")
		useStartOto(t, func(time.Duration, io.Reader) (otoPlayer, error) {
			return nil, failed
		})

		out, err := openOto(20, func([]byte) {})

		if !errors.Is(err, failed) {
			t.Errorf("got %v, want the library's own failure passed on", err)
		}
		if out != nil {
			t.Error("openOto gave a playback as well as an error")
		}
	})

	// Verify that the player is started, that it reads from the ring, and that
	// the device is asked to stand behind the same amount the other backend
	// asks for, which is what makes the two comparable.
	t.Run("StartsPlaying", func(t *testing.T) {
		f := &fakeOtoPlayer{}
		var gotBuffer time.Duration
		var gotReader io.Reader

		useStartOto(t, func(buffer time.Duration, r io.Reader) (otoPlayer, error) {
			gotBuffer, gotReader = buffer, r
			return f, nil
		})

		filled := false
		out, err := openOto(20, func([]byte) { filled = true })

		if err != nil {
			t.Fatalf("openOto failed: %v", err)
		}
		if f.plays != 1 {
			t.Errorf("the player was started %d times, want once", f.plays)
		}
		if want := 20 * otoPeriods * time.Millisecond; gotBuffer != want {
			t.Errorf("the device was asked to hold %s, want %s", gotBuffer, want)
		}

		// The reader has to reach the fill function, or the speakers play
		// whatever was already in the buffer forever.
		if _, err := gotReader.Read(make([]byte, 4)); err != nil {
			t.Fatalf("reading from the player's source failed: %v", err)
		}
		if !filled {
			t.Error("the player's source did not reach the ring")
		}

		out.Close()
		if f.closes != 1 {
			t.Errorf("the player was closed %d times, want once", f.closes)
		}
	})
}
