// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audiofeed

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func frameNo(n uint32) Frame {
	return Frame{Seq: n, PCM: make([]byte, MonoFrameBytes), Level: quietest, At: time.Now()}
}

// drain takes everything waiting for a listener without blocking on it.
func drain(s *Sub) []uint32 {
	var got []uint32
	for {
		select {
		case f := <-s.Frames():
			got = append(got, f.Seq)
		default:
			return got
		}
	}
}

// TestFeedGivesEveryListenerTheSameAudio is the whole point of the package: one
// sound card, read once, heard by as many things as ask.
func TestFeedGivesEveryListenerTheSameAudio(t *testing.T) {
	f := New(nil)

	subs := make([]*Sub, 3)
	for i := range subs {
		subs[i] = f.Subscribe(8)
		defer subs[i].Close()
	}

	if got := f.Subscribers(); got != 3 {
		t.Fatalf("the feed counts %d listeners, want 3", got)
	}

	for i := range uint32(4) {
		f.Publish(frameNo(i))
	}

	for i, s := range subs {
		got := drain(s)
		if len(got) != 4 {
			t.Fatalf("listener %d got %d frames, want 4", i, len(got))
		}
		for at, seq := range got {
			if seq != uint32(at) {
				t.Errorf("listener %d got frame %d in position %d", i, seq, at)
			}
		}
	}
}

// TestFeedDropsTheOldestForAListenerThatCannotKeepUp checks the rule that keeps
// a live stream live. A frame queued behind four others is four frames of delay
// that nobody asked for, and playing it before the current one only pushes that
// delay further out.
func TestFeedDropsTheOldestForAListenerThatCannotKeepUp(t *testing.T) {
	f := New(nil)
	s := f.Subscribe(4)
	defer s.Close()

	for i := range uint32(10) {
		f.Publish(frameNo(i))
	}

	got := drain(s)
	if len(got) != 4 {
		t.Fatalf("a queue 4 deep held %d frames", len(got))
	}

	// The newest four, not the oldest four.
	for at, seq := range got {
		if want := uint32(6 + at); seq != want {
			t.Errorf("position %d holds frame %d, want %d: the wrong end was dropped", at, seq, want)
		}
	}

	if s.Dropped() != 6 {
		t.Errorf("the listener counted %d dropped frames, want 6", s.Dropped())
	}
}

// TestFeedSlowListenerDoesNotHoldUpTheOthers is why each listener has a queue of
// its own rather than the feed calling them in turn. A listener on a bad
// connection must not be able to stall the audio going to a good one, or to the
// goroutine cutting frames.
func TestFeedSlowListenerDoesNotHoldUpTheOthers(t *testing.T) {
	f := New(nil)

	slow := f.Subscribe(2)
	defer slow.Close()
	quick := f.Subscribe(64)
	defer quick.Close()

	// Nothing ever reads from slow.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range uint32(50) {
			f.Publish(frameNo(i))
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publishing blocked on a listener that was not reading")
	}

	if got := len(drain(quick)); got != 50 {
		t.Errorf("the listener that was keeping up got %d frames, want all 50", got)
	}
}

// TestFeedForgetsAClosedListener: a listener that has gone must stop costing
// anything, because that count is what decides whether the sound card is open
// at all.
func TestFeedForgetsAClosedListener(t *testing.T) {
	f := New(nil)

	a := f.Subscribe(4)
	b := f.Subscribe(4)
	defer b.Close()

	a.Close()
	if got := f.Subscribers(); got != 1 {
		t.Errorf("after one listener closed the feed counts %d, want 1", got)
	}

	// Its channel is closed, so a reader looping over it ends rather than
	// blocking forever.
	if _, open := <-a.Frames(); open {
		t.Error("a closed listener's frames channel is still open")
	}

	// And publishing does not panic on the one that has gone.
	f.Publish(frameNo(1))
	if got := drain(b); len(got) != 1 {
		t.Errorf("the remaining listener got %d frames, want 1", len(got))
	}
}

// TestSubCloseIsSafeTwice, because the usual caller closes it in a defer and may
// also close it on the way out of a loop.
func TestSubCloseIsSafeTwice(t *testing.T) {
	f := New(nil)
	s := f.Subscribe(4)

	s.Close()
	s.Close()

	if got := f.Subscribers(); got != 0 {
		t.Errorf("the feed counts %d listeners after both closes, want 0", got)
	}
}

// TestFeedCloseDuringPublish is the race between a listener going away and a
// frame being handed out. Both take the feed's lock, and the test is here so
// that a change which moves one of them out from under it fails under -race.
func TestFeedCloseDuringPublish(t *testing.T) {
	f := New(nil)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := uint32(0); ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			f.Publish(frameNo(i))
		}
	}()

	for range 200 {
		s := f.Subscribe(2)
		s.Close()
	}

	close(stop)
	wg.Wait()

	if got := f.Subscribers(); got != 0 {
		t.Errorf("the feed counts %d listeners at the end, want 0", got)
	}
}

func TestFeedEvents(t *testing.T) {
	f := New(nil)
	s := f.Subscribe(4)
	defer s.Close()

	f.PublishEvent("level", map[string]any{"dbfs": -42.0})

	select {
	case ev := <-s.Events():
		if ev.Kind != "level" {
			t.Errorf("the event arrived as %q, want %q", ev.Kind, "level")
		}
	default:
		t.Fatal("no event arrived")
	}
}

// TestFeedEventsAreDroppedRatherThanQueued: there is nothing here that is worth
// arriving late, and a level meter from four seconds ago pushing out the frame
// that would have caught a listener up is the wrong trade.
func TestFeedEventsAreDroppedRatherThanQueued(t *testing.T) {
	f := New(nil)
	s := f.Subscribe(2)
	defer s.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			f.PublishEvent("level", nil)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publishing an event blocked on a listener that was not reading")
	}
}

// TestSubscribe tests the Subscribe function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Depth: a listener is queued as deep as it asked for
//   - NoDepth: a listener that asked for no queue at all is given one frame
func TestSubscribe(t *testing.T) {

	// Verify that a listener is queued as deep as it asked for.
	t.Run("Depth", func(t *testing.T) {
		f := New(nil)
		s := f.Subscribe(8)
		defer s.Close()

		if got := cap(s.frames); got != 8 {
			t.Errorf("the listener's queue holds %d frames, want 8", got)
		}
		if got := cap(s.events); got != 8 {
			t.Errorf("the listener's news queue holds %d, want 8", got)
		}
		if got := f.Subscribers(); got != 1 {
			t.Errorf("the feed counts %d listeners, want 1", got)
		}
	})

	// Verify that a listener that asked for no queue at all is given one frame.
	t.Run("NoDepth", func(t *testing.T) {
		f := New(nil)
		s := f.Subscribe(0)
		defer s.Close()

		if got := cap(s.frames); got != 1 {
			t.Errorf("a queue asked for 0 deep holds %d frames, want 1", got)
		}
	})
}

// Test_offer tests the offer function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Room: a listener keeping up is handed the frame
//   - Full: a listener behind loses the oldest frame it had not taken
//   - NoRoomAtAll: a listener that cannot take a frame at all loses it
func Test_offer(t *testing.T) {

	// Verify that a listener keeping up is handed the frame.
	t.Run("Room", func(t *testing.T) {
		f := New(nil)
		s := f.Subscribe(2)
		defer s.Close()

		s.offer(frameNo(1))

		if got := drain(s); len(got) != 1 || got[0] != 1 {
			t.Errorf("the listener was handed %v, want [1]", got)
		}
		if s.Dropped() != 0 {
			t.Errorf("a listener keeping up counted %d dropped frames", s.Dropped())
		}
	})

	// Verify that a listener behind loses the oldest frame it had not taken.
	t.Run("Full", func(t *testing.T) {
		f := New(nil)
		s := f.Subscribe(1)
		defer s.Close()

		s.offer(frameNo(1))
		s.offer(frameNo(2))

		if got := drain(s); len(got) != 1 || got[0] != 2 {
			t.Errorf("the listener holds %v, want [2], which is the newest", got)
		}
		if s.Dropped() != 1 {
			t.Errorf("the listener counted %d dropped frames, want 1", s.Dropped())
		}
	})

	// Verify that a listener that cannot take a frame at all loses it.
	//
	// Built here rather than through Subscribe, which raises a queue of nothing
	// to one frame. A queue with no room in it is what a listener that has just
	// been handed the frame it was given room for looks like to the goroutine
	// cutting the next one, and the rule is that the cutter gives up rather than
	// waits.
	t.Run("NoRoomAtAll", func(t *testing.T) {
		f := New(nil)
		s := &Sub{feed: f, frames: make(chan Frame), events: make(chan Event)}

		s.offer(frameNo(1))

		if s.Dropped() != 1 {
			t.Errorf("the listener counted %d dropped frames, want 1", s.Dropped())
		}
	})
}

// Test_drop tests the drop method with 100% coverage.
//
// Coverage: 100% (1 test case covering both branches)
//
// Test cases:
//   - SaysSoOnce: the first dropped frame is reported and the rest are counted
//     in silence
func Test_drop(t *testing.T) {
	// Verify that the first dropped frame is reported and the ones after it are
	// counted without another word. A listener that has fallen behind drops
	// steadily, so a line for every frame would be fifty a second saying what
	// the first one already said.
	t.Run("SaysSoOnce", func(t *testing.T) {
		var logged bytes.Buffer
		f := New(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))

		s := f.Subscribe(1)
		defer s.Close()

		s.drop()
		if lines := strings.Count(logged.String(), "\n"); lines != 1 {
			t.Fatalf("the first dropped frame wrote %d lines, want 1:\n%s", lines, logged.String())
		}
		if !strings.Contains(logged.String(), "behind") {
			t.Errorf("the line said %q, which does not say the listener is behind", logged.String())
		}

		for range 10 {
			s.drop()
		}
		if lines := strings.Count(logged.String(), "\n"); lines != 1 {
			t.Errorf("eleven dropped frames wrote %d lines, want the first one only", lines)
		}
		if s.Dropped() != 11 {
			t.Errorf("the listener counted %d dropped frames, want 11: the rest still count", s.Dropped())
		}
	})
}
