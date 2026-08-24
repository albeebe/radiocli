// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audio

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audiogate"
	"github.com/albeebe/radiocli/internal/audioout"
)

// fakePlayer is a set of speakers that never made a sound, so everything that
// plays audio can be tested without one and without a noise in the room the
// tests are running in.
type fakePlayer struct {
	// mu guards everything below, because the recorder plays from its own
	// goroutine while a test reads what was played.
	mu sync.Mutex

	// closes counts the teardowns, so a double close can be seen.
	closes int

	// heard is every byte handed over, in order.
	heard []byte

	// name is what Name reports, empty for the system's own output.
	name string

	// stats is what Stats reports.
	stats audioout.Stats
}

// Close counts the teardown rather than doing one.
func (f *fakePlayer) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
}

// Name answers with whatever the test put there.
func (f *fakePlayer) Name() string { return f.name }

// Play keeps a copy of the audio, the way real speakers keep it long enough to
// play it.
func (f *fakePlayer) Play(pcm []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heard = append(f.heard, pcm...)
}

// Stats answers with whatever the test put there.
func (f *fakePlayer) Stats() audioout.Stats { return f.stats }

// played is how many bytes of audio reached the speakers.
func (f *fakePlayer) played() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.heard)
}

// fakeOpenPlayer installs a speaker opener that answers with what it was given,
// and restores the real one at the end of the test.
//
// Parameters:
//   - t: the test the real opener is restored at the end of
//   - p: the speakers to hand back
//   - err: the failure to report instead, if there is one
//
// Returns:
//   - the name the opener was asked for, readable after the command has run
func fakeOpenPlayer(t *testing.T, p *fakePlayer, err error) *string {
	t.Helper()

	asked := new(string)
	original := openPlayer
	t.Cleanup(func() { openPlayer = original })
	openPlayer = func(name string) (player, error) {
		*asked = name
		if err != nil {
			return nil, err
		}
		return p, nil
	}
	return asked
}

// listenFrames returns a channel already holding frames, then closed, which is
// what the loop sees when the audio ends.
//
// Parameters:
//   - frames: the audio to put in it
//
// Returns:
//   - the channel, closed, so a loop reading it ends on its own
func listenFrames(frames []audiofeed.Frame) chan audiofeed.Frame {
	ch := make(chan audiofeed.Frame, len(frames))
	for _, f := range frames {
		ch <- f
	}
	close(ch)
	return ch
}

// Test_newListen tests the newListen function with 100% coverage.
//
// Coverage: 100% (3 test cases covering the command and the closure it holds)
//
// Test cases:
//   - Wiring: the command carries its name and its flags
//   - Defaults: the squelch is on and the speakers are this computer's own
//   - Runs: executing the command reaches runListen, which refuses it in a daemon
func Test_newListen(t *testing.T) {
	// Verify that the command is described the way the tool wires it
	t.Run("Wiring", func(t *testing.T) {
		cmd := newListen(appcontext.New())

		if cmd.Use != "listen" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "listen")
		}
		for _, name := range []string{"input", "channel", "speaker", "squelch", "hang"} {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("the command has no --%s flag", name)
			}
		}
	})

	// Verify the two defaults a person is most likely to rely on without
	// reading: the hiss is kept out, and the audio goes where everything else
	// on this computer goes.
	t.Run("Defaults", func(t *testing.T) {
		cmd := newListen(appcontext.New())

		if got := cmd.Flags().Lookup("squelch").DefValue; got != "true" {
			t.Errorf("--squelch defaults to %q, wanted the transmissions only", got)
		}
		if got := cmd.Flags().Lookup("speaker").DefValue; got != "" {
			t.Errorf("--speaker defaults to %q, wanted this computer's own", got)
		}
		if got := cmd.Flags().Lookup("hang").DefValue; got != audiogate.DefaultQuietHang.String() {
			t.Errorf("--hang defaults to %q, wanted the gate's own quiet hang", got)
		}
	})

	// Verify that running the command reaches runListen, which is what the
	// closure newListen hands cobra exists to do
	t.Run("Runs", func(t *testing.T) {
		app := appcontext.New()
		app.InDaemon = true

		cmd := newListen(app)
		cmd.SetOut(&strings.Builder{})
		cmd.SetErr(&strings.Builder{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), "inside a daemon") {
			t.Errorf("running the command gave %v, wanted the daemon refusal", err)
		}
	})
}

// Test_announceListening tests the announceListening function with 100%
// coverage, and that it writes to stderr.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Named: the speakers are quoted as the system spells them
//   - DefaultSpeakers: an unnamed default is described rather than left blank
//   - Squelched: the transmissions are what it says it is playing
//   - Everything: with the squelch off it says so
func Test_announceListening(t *testing.T) {
	// Verify that the input and the speakers are both named, on stderr, so
	// that a person can see the audio is going where they meant.
	t.Run("Named", func(t *testing.T) {
		app, out, errs := recorderApp()
		announceListening(app, "USB Audio CODEC", "MacBook Pro Speakers", true)

		if out.Len() != 0 {
			t.Errorf("wrote %q to stdout, want the commentary on stderr", out.String())
		}
		if !strings.Contains(errs.String(), "USB Audio CODEC") ||
			!strings.Contains(errs.String(), "MacBook Pro Speakers") {
			t.Errorf("wrote %q, want the input and the speakers named", errs.String())
		}
	})

	// Verify that the system's own output, which the library does not always
	// put a name to, is described rather than quoted as nothing.
	t.Run("DefaultSpeakers", func(t *testing.T) {
		app, _, errs := recorderApp()
		announceListening(app, "USB Audio CODEC", "", true)

		if !strings.Contains(errs.String(), "this computer's own speakers") {
			t.Errorf("wrote %q, want the default speakers described", errs.String())
		}
	})

	// Verify that the squelch being on is said, since it explains the silence
	// between transmissions.
	t.Run("Squelched", func(t *testing.T) {
		app, _, errs := recorderApp()
		announceListening(app, "USB Audio CODEC", "", true)

		if !strings.Contains(errs.String(), "the transmissions") {
			t.Errorf("wrote %q, want it to say only the transmissions are played", errs.String())
		}
	})

	// Verify that the squelch being off is said too, since it explains the hiss.
	t.Run("Everything", func(t *testing.T) {
		app, _, errs := recorderApp()
		announceListening(app, "USB Audio CODEC", "", false)

		if !strings.Contains(errs.String(), "everything the input carries") {
			t.Errorf("wrote %q, want it to say everything is played", errs.String())
		}
	})
}

// Test_listenLoop tests the listenLoop function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Everything: with the squelch off every frame reaches the speakers
//   - OnlyTransmissions: with it on the quiet is not played
//   - AudioEnds: the audio stopping ends the loop without complaint
//   - Cancelled: Ctrl-C ends the loop without complaint
func Test_listenLoop(t *testing.T) {
	// Verify that the squelch being off means exactly what it says: what
	// arrives is what is played, hiss included.
	t.Run("Everything", func(t *testing.T) {
		quiet, _ := feed(0, 20, quietLevel)
		p := &fakePlayer{}

		err := listenLoop(context.Background(), listenFrames(quiet), p, listenOptions{})
		if err != nil {
			t.Fatalf("listening gave %v, want it to end quietly", err)
		}
		if p.played() != 20*len(quiet[0].PCM) {
			t.Errorf("%d bytes were played, want every frame", p.played())
		}
	})

	// Verify that the squelch keeps the noise floor out and lets a transmission
	// through, which is the whole point of it being on by default.
	t.Run("OnlyTransmissions", func(t *testing.T) {
		quiet, next := feed(0, 40, quietLevel)
		loud, next := feed(next, 60, loudLevel)
		tail, _ := feed(next, 40, quietLevel)

		p := &fakePlayer{}
		frames := append(append(append([]audiofeed.Frame{}, quiet...), loud...), tail...)

		err := listenLoop(context.Background(), listenFrames(frames), p,
			listenOptions{squelch: true, hang: 200 * time.Millisecond})
		if err != nil {
			t.Fatalf("listening gave %v, want it to end quietly", err)
		}

		played := p.played()
		if played == 0 {
			t.Fatal("nothing was played, want the transmission")
		}
		// Everything is a possibility worth ruling out: it would mean the gate
		// opened on the noise floor and the squelch does nothing.
		if played >= len(frames)*len(quiet[0].PCM) {
			t.Errorf("%d bytes were played of %d, want the quiet left out",
				played, len(frames)*len(quiet[0].PCM))
		}
	})

	// Verify that the audio ending, which is what a daemon going away looks
	// like, finishes the command rather than failing it.
	t.Run("AudioEnds", func(t *testing.T) {
		p := &fakePlayer{}
		ch := make(chan audiofeed.Frame)
		close(ch)

		if err := listenLoop(context.Background(), ch, p, listenOptions{}); err != nil {
			t.Errorf("listening gave %v, want the audio ending to be an ending", err)
		}
	})

	// Verify that Ctrl-C leaves the exit status alone, since stopping is how a
	// command with no natural end is meant to finish.
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		p := &fakePlayer{}
		if err := listenLoop(ctx, make(chan audiofeed.Frame), p, listenOptions{}); err != nil {
			t.Errorf("listening gave %v, want stopping to be quiet", err)
		}
	})
}

// Test_reportPlayback tests the reportPlayback function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both branches)
//
// Test cases:
//   - Clean: speakers that kept up say nothing
//   - Dropped: audio thrown away is reported, in seconds
func Test_reportPlayback(t *testing.T) {
	// Verify that the ordinary run is silent. Running dry happens at the end of
	// every transmission with the squelch on, and reporting it would be a
	// complaint about the buffer working.
	t.Run("Clean", func(t *testing.T) {
		app, _, errs := recorderApp()
		reportPlayback(app, &fakePlayer{stats: audioout.Stats{Starved: 12}})

		if errs.Len() != 0 {
			t.Errorf("wrote %q, want nothing said about a run that kept up", errs.String())
		}
	})

	// Verify that audio which never reached the speakers is reported in a unit
	// a person can picture, since it is the one number here worth acting on.
	t.Run("Dropped", func(t *testing.T) {
		app, _, errs := recorderApp()
		reportPlayback(app, &fakePlayer{stats: audioout.Stats{Dropped: playedBytesPerSecond * 2}})

		if !strings.Contains(errs.String(), "2.0 seconds") {
			t.Errorf("wrote %q, want the dropped audio in seconds", errs.String())
		}
	})
}

// Test_runListen tests the runListen function with 100% coverage.
//
// Coverage: 100% (7 test cases covering every check and both ways to listen)
//
// Test cases:
//   - InDaemon: a command that never ends is refused inside a daemon
//   - NoDevice: with no scanner named and no input given there is nothing to hear
//   - BadHang: a hang that is not a length of quiet is refused
//   - BadChannel: a side of the cable that does not exist is refused
//   - SpeakersFail: speakers that will not open are reported before anything
//     else is
//   - AudioFails: a sound input that will not open is reported
//   - Listens: a named input is opened and played, and given back afterwards
func Test_runListen(t *testing.T) {
	// Verify that a daemon is refused, since this command holds the streams it
	// was lent until somebody stops it.
	t.Run("InDaemon", func(t *testing.T) {
		app, _, _ := recorderApp()
		app.InDaemon = true

		err := runListen(context.Background(), app, listenOptions{})
		if err == nil || !strings.Contains(err.Error(), "inside a daemon") {
			t.Errorf("listening gave %v, want the daemon refusal", err)
		}
	})

	// Verify that with nothing to listen to, the advice names both ways out
	// rather than reporting a daemon that was never going to be there.
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := recorderApp()

		err := runListen(context.Background(), app, listenOptions{})
		if !errors.Is(err, appcontext.ErrNoDevice) {
			t.Errorf("listening gave %v, want it to say no scanner was named", err)
		}
		if !strings.Contains(err.Error(), "--input") {
			t.Errorf("listening gave %v, want the other way out named as well", err)
		}
	})

	// Verify that a hang of nothing is caught rather than handed to the gate,
	// which would quietly replace it with its own default.
	t.Run("BadHang", func(t *testing.T) {
		app, _, _ := recorderApp()

		err := runListen(context.Background(), app, listenOptions{input: "Line In", hang: -time.Second})
		if err == nil || !strings.Contains(err.Error(), "not a length of quiet") {
			t.Errorf("listening gave %v, want the hang refused", err)
		}
	})

	// Verify that a channel nobody has is refused before a sound card is
	// opened.
	t.Run("BadChannel", func(t *testing.T) {
		app, _, _ := recorderApp()

		err := runListen(context.Background(), app, listenOptions{
			input: "Line In", channel: "middle", hang: time.Second,
		})
		if err == nil {
			t.Fatal("listening succeeded with a channel that does not exist")
		}
	})

	// Verify that the speakers are opened before the audio, so that a typo in
	// --speaker costs a moment rather than a sound card being taken and given
	// straight back.
	t.Run("SpeakersFail", func(t *testing.T) {
		app, _, _ := recorderApp()
		fakeOpenPlayer(t, nil, errors.New("no speaker by that name"))

		opened := false
		original := startCapture
		t.Cleanup(func() { startCapture = original })
		startCapture = func(audiofeed.Options, audiofeed.Publisher) (capture, error) {
			opened = true
			return fakeCapture{source: "Line In"}, nil
		}

		err := runListen(context.Background(), app, listenOptions{
			input: "Line In", channel: audiofeed.ChannelAuto, speaker: "Kitchen Radio",
			hang: time.Second,
		})
		if err == nil || !strings.Contains(err.Error(), "no speaker by that name") {
			t.Errorf("listening gave %v, want the speakers' own failure", err)
		}
		if opened {
			t.Error("the sound input was opened even though the speakers had already failed")
		}
	})

	// Verify that a sound input which will not open is reported, and that the
	// speakers opened for it are given back.
	t.Run("AudioFails", func(t *testing.T) {
		app, _, _ := recorderApp()
		p := &fakePlayer{}
		fakeOpenPlayer(t, p, nil)
		fakeStart(t, "", errors.New("the input is already open"))

		err := runListen(context.Background(), app, listenOptions{
			input: "Line In", channel: audiofeed.ChannelAuto, hang: time.Second,
		})
		if err == nil || !strings.Contains(err.Error(), "already open") {
			t.Errorf("listening gave %v, want the input's own failure", err)
		}
		if p.closes != 1 {
			t.Errorf("the speakers were closed %d times, want them given back", p.closes)
		}
	})

	// Verify the whole path: the named speakers are asked for, the input is
	// opened, and stopping ends it quietly with everything given back.
	t.Run("Listens", func(t *testing.T) {
		app, _, errs := recorderApp()
		p := &fakePlayer{name: "MacBook Pro Speakers"}
		asked := fakeOpenPlayer(t, p, nil)
		fakeStart(t, "Cubilux CB5 Line In", nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := runListen(ctx, app, listenOptions{
			input: "Line In", channel: audiofeed.ChannelAuto,
			speaker: "MacBook Pro Speakers", hang: time.Second,
		})
		if err != nil {
			t.Fatalf("listening gave %v, want stopping to be quiet", err)
		}
		if *asked != "MacBook Pro Speakers" {
			t.Errorf("the speakers asked for were %q, want the ones named", *asked)
		}
		if !strings.Contains(errs.String(), "Cubilux CB5 Line In") {
			t.Errorf("wrote %q, want the input it settled on named", errs.String())
		}
		if p.closes != 1 {
			t.Errorf("the speakers were closed %d times, want them given back", p.closes)
		}
	})
}

// Test_openPlayer tests the openPlayer variable with 100% coverage.
//
// Coverage: 100% (1 test case covering the one call the real opener makes)
//
// The name below cannot belong to anything attached, so the refusal happens
// while the library is still reading its device list and no speakers are ever
// opened. That is the whole of the real opener that can be reached without
// making a noise in the room the tests are running in.
//
// Test cases:
//   - NoSuchSpeaker: a name nothing answers to is refused, and what comes back
//     with the refusal is harmless to close
func Test_openPlayer(t *testing.T) {
	// Verify that the default opener is the real one, refusing what it refuses.
	//
	// The player handed back alongside a refusal is a nil *audioout.Player in an
	// interface, which does not read as nil. Nothing checks it, because every
	// caller checks the error first, and closing it anyway has to be harmless
	// or that arrangement would be a crash waiting for a typo in --speaker.
	t.Run("NoSuchSpeaker", func(t *testing.T) {
		p, err := openPlayer("no speaker is called this, and none ever will be")
		if err == nil {
			p.Close()
			t.Fatal("the speakers opened, wanted a name nothing answers to to be refused")
		}
		p.Close()
	})
}
