// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package broker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/opusenc"
)

// fakeSide builds the audio half of a daemon whose sound card is not there, and
// hands back the feed a test publishes into.
func fakeSide(t *testing.T, open func(audiofeed.Options, audiofeed.Publisher) (audioCapture, error)) *audioSide {
	t.Helper()

	side := newAudioSide("Test Line In", audiofeed.ChannelLeft, slog.New(slog.DiscardHandler))
	side.open = open
	t.Cleanup(side.stop)
	return side
}

// nextFrame takes the next framed message a connection sent.
func nextFrame(t *testing.T, in *bufio.Reader) (byte, []byte) {
	t.Helper()

	kind, payload, err := readFrame(in)
	if err != nil {
		t.Fatalf("reading a frame: %v", err)
	}
	return kind, payload
}

// TestNewAudioSide tests the newAudioSide function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - NoSoundInput: a daemon started without one has no audio side at all
//   - Named: a daemon started with one keeps the name and the channel
func TestNewAudioSide(t *testing.T) {
	// Verify that a daemon started without a sound input has no audio side at all.
	t.Run("NoSoundInput", func(t *testing.T) {
		if side := newAudioSide("", audiofeed.ChannelLeft, slog.New(slog.DiscardHandler)); side != nil {
			t.Fatalf("a daemon with no sound input was given %+v", side)
		}
	})

	// Verify that a daemon started with a sound input keeps the name and the channel.
	t.Run("Named", func(t *testing.T) {
		side := newAudioSide("Line In", audiofeed.ChannelRight, slog.New(slog.DiscardHandler))
		if side.name() != "Line In" || side.channel != audiofeed.ChannelRight {
			t.Fatalf("the audio side was built as %+v", side)
		}

		// The default is the real capture, which is not opened here: calling it
		// with nowhere to publish is refused before any device is touched, and
		// that is what shows it is wired to the real one.
		if _, err := side.open(audiofeed.Options{}, nil); err == nil {
			t.Fatal("the default capture accepted a start with nowhere to publish")
		}
	})
}

// Test_acquireRefusesADeviceThatWillNotOpen tests the acquire method's failure
// path with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - WillNotOpen: a sound input that will not open is reported
func Test_acquireRefusesADeviceThatWillNotOpen(t *testing.T) {
	// Verify that a sound input that will not open is reported.
	t.Run("WillNotOpen", func(t *testing.T) {
		side := fakeSide(t, func(audiofeed.Options, audiofeed.Publisher) (audioCapture, error) {
			return nil, errors.New("the sound input is gone")
		})

		if _, _, err := side.acquire(); err == nil || err.Error() != "the sound input is gone" {
			t.Fatalf("a sound input that would not open gave %v", err)
		}
	})
}

// Test_releaseIgnoresADaemonWithNoAudio tests the release method's nil receiver
// with 100% coverage.
//
// Coverage: 100% (1 test case covering all branches)
//
// Test cases:
//   - NoAudioSide: a daemon holding no sound input has nothing to give back
func Test_releaseIgnoresADaemonWithNoAudio(t *testing.T) {
	// Verify that a daemon holding no sound input has nothing to give back.
	t.Run("NoAudioSide", func(t *testing.T) {
		var side *audioSide
		side.release(nil)
	})
}

// Test_audio tests the audio method with 100% coverage.
//
// Coverage: 100% (6 test cases covering all branches)
//
// Test cases:
//   - ActionBeforeStreaming: a control action on a fresh connection is refused
//   - UnknownFormat: an audio format nobody has heard of is refused
//   - NoEncoder: a compressed stream at a rate the codec refuses is reported
//   - NoSoundInput: a daemon holding no sound input says so
//   - Unwritable: a stream whose client has gone gives the card straight back
//   - AlreadyStreaming: a second request on a streaming connection is a control message
func Test_audio(t *testing.T) {
	// Verify that a control action on a connection that is not streaming is
	// refused rather than treated as a request to start streaming. It used to
	// fall through to the subscribe path, so a client asking to change a rate
	// on a fresh connection was handed a stream of audio it never asked for and
	// could no longer send ordinary messages on.
	t.Run("ActionBeforeStreaming", func(t *testing.T) {
		c, srv, in, _ := testConn(t, nil)
		srv.audio = fakeSide(t, func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
			return &fakeCapture{source: opts.Source}, nil
		})

		c.audio(Request{Op: OpAudio, ID: "1", Action: "bitrate", Bitrate: 16000})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, "already carrying audio") {
			t.Fatalf("a control action before streaming gave %+v", msg)
		}
		if c.streaming.Load() {
			t.Fatal("a control action turned the connection into a stream")
		}
		if srv.audio.holders != 0 {
			t.Fatalf("%d listeners were taken on the card", srv.audio.holders)
		}
	})

	// Verify that an audio format nobody has heard of is refused.
	t.Run("UnknownFormat", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.audio(Request{Op: OpAudio, ID: "1", Format: "flac"})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, "there is no audio format called") {
			t.Fatalf("an unknown format gave %+v", msg)
		}
	})

	// Verify that a compressed stream at a rate the codec refuses is reported.
	t.Run("NoEncoder", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.audio(Request{Op: OpAudio, ID: "1", Format: FormatOpus, Bitrate: opusenc.MaxBitrate + 1})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || msg.Error == "" {
			t.Fatalf("a rate the codec refuses gave %+v", msg)
		}
	})

	// Verify that a daemon holding no sound input says so.
	t.Run("NoSoundInput", func(t *testing.T) {
		c, _, in, _ := testConn(t, nil)

		c.audio(Request{Op: OpAudio, ID: "1"})

		msg := nextMsg(t, in)
		if msg.Type != TypeError || !strings.Contains(msg.Error, "not holding a sound input") {
			t.Fatalf("a daemon with no sound input gave %+v", msg)
		}
	})

	// Verify that a stream whose client has gone gives the card straight back.
	t.Run("Unwritable", func(t *testing.T) {
		c, srv, _, client := testConn(t, nil)

		var capture *fakeCapture
		srv.audio = fakeSide(t, func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
			capture = &fakeCapture{source: opts.Source, channel: opts.Channel}
			return capture, nil
		})

		client.Close()
		c.net.Close()

		c.audio(Request{Op: OpAudio, ID: "1", Format: FormatPCM})

		if c.streaming.Load() {
			t.Fatal("a connection whose reply never went out was left streaming")
		}
		if srv.audio.holders != 0 {
			t.Fatalf("%d listeners were left on the card", srv.audio.holders)
		}
	})

	// Verify that a second request on a streaming connection is a control message.
	t.Run("AlreadyStreaming", func(t *testing.T) {
		c, _, _, client := testConn(t, nil)
		c.streaming.Store(true)

		c.audio(Request{Op: OpAudio, ID: "1", Action: "bitrate", Bitrate: 16000})

		select {
		case got := <-c.bitrate:
			if got != 16000 {
				t.Fatalf("the streaming goroutine was told %d", got)
			}
		default:
			t.Fatal("a control message on a streaming connection went nowhere")
		}
		client.Close()
	})
}

// Test_audioControl tests the audioControl method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Bitrate: the rate reaches the streaming goroutine
//   - Superseded: a rate nobody picked up yet is replaced rather than queued
//   - Unknown: an action nobody has heard of is refused as a frame
func Test_audioControl(t *testing.T) {
	// Verify that the rate reaches the streaming goroutine.
	t.Run("Bitrate", func(t *testing.T) {
		c, _, _, _ := testConn(t, nil)

		c.audioControl(Request{Op: OpAudio, ID: "1", Action: "bitrate", Bitrate: 24000})

		if got := <-c.bitrate; got != 24000 {
			t.Fatalf("the streaming goroutine was told %d", got)
		}
	})

	// Verify that a rate nobody picked up yet is replaced rather than queued.
	t.Run("Superseded", func(t *testing.T) {
		c, _, _, _ := testConn(t, nil)

		c.audioControl(Request{Op: OpAudio, ID: "1", Action: "bitrate", Bitrate: 24000})
		c.audioControl(Request{Op: OpAudio, ID: "2", Action: "bitrate", Bitrate: 16000})

		if got := <-c.bitrate; got != 24000 {
			t.Fatalf("the streaming goroutine was told %d", got)
		}
		select {
		case got := <-c.bitrate:
			t.Fatalf("a second rate was queued behind the first: %d", got)
		default:
		}
	})

	// Verify that an action nobody has heard of is refused as a frame.
	t.Run("Unknown", func(t *testing.T) {
		c, _, _, client := testConn(t, nil)
		c.streaming.Store(true)

		c.audioControl(Request{Op: OpAudio, ID: "1", Action: "levitate"})

		kind, payload := nextFrame(t, bufio.NewReader(client))
		var msg Response
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatal(err)
		}
		if kind != FrameJSON || msg.Type != TypeError ||
			!strings.Contains(msg.Error, "there is nothing an audio connection can do called") {
			t.Fatalf("an action nobody has heard of gave %d and %+v", kind, msg)
		}
	})
}

// Test_sendEvent tests the sendEvent method with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the news goes out as a frame with the kind as its type
//   - Unwritable: a frame that could not be written is reported
//   - Unencodable: news that cannot be written down at all is dropped
func Test_sendEvent(t *testing.T) {
	// Verify that the news goes out as a frame with the kind as its type.
	t.Run("Success", func(t *testing.T) {
		c, _, _, client := testConn(t, nil)

		err := c.sendEvent(audiofeed.Event{Kind: "silence", Payload: map[string]any{"seconds": 4}})
		if err != nil {
			t.Fatalf("sending news: %v", err)
		}

		kind, payload := nextFrame(t, bufio.NewReader(client))
		var got map[string]any
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatal(err)
		}
		if kind != FrameJSON || got["type"] != "silence" || got["seconds"] != float64(4) {
			t.Fatalf("the news came back as %d and %v", kind, got)
		}
	})

	// Verify that a frame that could not be written is reported.
	t.Run("Unwritable", func(t *testing.T) {
		c, _, _, client := testConn(t, nil)
		client.Close()
		c.net.Close()

		if err := c.sendEvent(audiofeed.Event{Kind: "silence"}); err == nil {
			t.Fatal("news written to a closed socket was not reported")
		}
	})

	// Verify that news that cannot be written down at all is dropped.
	t.Run("Unencodable", func(t *testing.T) {
		c, _, _, _ := testConn(t, nil)

		err := c.sendEvent(audiofeed.Event{
			Kind:    "silence",
			Payload: map[string]any{"listener": make(chan int)},
		})
		if err != nil {
			t.Fatalf("news that could not be written down gave %v", err)
		}
	})
}

// Test_sendFrameJSON tests the sendFrameJSON method with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the message goes out as a framed payload
//   - Unencodable: a message that could not be encoded is dropped
func Test_sendFrameJSON(t *testing.T) {
	// Verify that the message goes out as a framed payload.
	//
	// A Response is strings, ints and slices of them, which the real encoder
	// never refuses, so the refusal to encode is driven through marshalJSON.
	t.Run("Success", func(t *testing.T) {
		c, _, _, client := testConn(t, nil)

		c.sendFrameJSON(Response{Type: TypeError, Error: "this stream has no bitrate to change"})

		kind, payload := nextFrame(t, bufio.NewReader(client))
		var msg Response
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatal(err)
		}
		if kind != FrameJSON || msg.Error != "this stream has no bitrate to change" {
			t.Fatalf("the message came back as %d and %+v", kind, msg)
		}
	})

	// Verify that a message that could not be encoded is dropped rather than
	// sent as an empty frame.
	//
	// Nothing is written for the refused message, so the sentinel written after
	// it is the first frame the listener sees.
	t.Run("Unencodable", func(t *testing.T) {
		failMarshal(t)
		c, _, _, client := testConn(t, nil)

		c.sendFrameJSON(Response{Type: TypeError, Error: "this stream has no bitrate to change"})

		if err := writeFrame(c.net, FrameJSON, []byte(`{"type":"sentinel"}`)); err != nil {
			t.Fatal(err)
		}

		kind, payload := nextFrame(t, bufio.NewReader(client))
		if kind != FrameJSON || string(payload) != `{"type":"sentinel"}` {
			t.Fatalf("the listener saw %d and %s before the sentinel", kind, payload)
		}
	})
}

// Test_pumpAudio tests the pumpAudio method with 100% coverage.
//
// Coverage: 100% (9 test cases covering all branches)
//
// Test cases:
//   - Stopped: a connection tearing down ends the stream
//   - NotCompressed: a rate asked for on an uncompressed stream is refused
//   - RefusedRate: a rate the codec will not take is reported and the stream carries on
//   - Rate: a rate the codec takes is applied without an answer
//   - News: what the capture has to say goes out between the frames
//   - Unencodable: a frame the codec will not take ends the stream
//   - NewsUnwritable: news that could not be written ends the stream
//   - FeedEnded: a feed that ended ends the stream
//   - Gone: a client that hung up ends the stream
func Test_pumpAudio(t *testing.T) {
	// pump starts a stream on a connection, and hands back the feed behind it.
	pump := func(t *testing.T, enc *opusenc.Encoder) (*conn, *audiofeed.Feed, *audiofeed.Sub, *bufio.Reader, chan struct{}) {
		t.Helper()

		c, _, _, client := testConn(t, nil)
		c.streaming.Store(true)

		feed := audiofeed.New(slog.New(slog.DiscardHandler))
		sub := feed.Subscribe(audioQueue)

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.pumpAudio(sub, enc)
		}()
		t.Cleanup(func() {
			sub.Close()
			<-done
		})
		return c, feed, sub, bufio.NewReader(client), done
	}

	// Verify that a connection tearing down ends the stream.
	t.Run("Stopped", func(t *testing.T) {
		c, _, _, _, done := pump(t, nil)

		close(c.audioDone)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the stream never ended")
		}
	})

	// Verify that a rate asked for on an uncompressed stream is refused.
	t.Run("NotCompressed", func(t *testing.T) {
		c, _, _, in, _ := pump(t, nil)

		c.bitrate <- 16000

		_, payload := nextFrame(t, in)
		var msg Response
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != TypeError || !strings.Contains(msg.Error, "not compressed") {
			t.Fatalf("a rate on an uncompressed stream gave %+v", msg)
		}
	})

	// Verify that a rate the codec will not take is reported and the stream carries on.
	t.Run("RefusedRate", func(t *testing.T) {
		enc, err := opusenc.New(opusenc.MinBitrate)
		if err != nil {
			t.Fatal(err)
		}
		c, feed, _, in, _ := pump(t, enc)

		c.bitrate <- opusenc.MaxBitrate + 1

		_, payload := nextFrame(t, in)
		var msg Response
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != TypeError || msg.Error == "" {
			t.Fatalf("a rate the codec refuses gave %+v", msg)
		}

		// Still carrying audio afterwards, which is the point of reporting it
		// rather than ending the stream.
		feed.Publish(pcmFrame(1, 2))
		if kind, _ := nextFrame(t, in); kind != FrameAudio {
			t.Fatalf("the stream carried %d after a refused rate", kind)
		}
	})

	// Verify that a rate the codec takes is applied without an answer.
	t.Run("Rate", func(t *testing.T) {
		// Both rates are ones whose packets fit, because which of the two the
		// stream applies to this frame is decided for it.
		enc, err := opusenc.New(32000)
		if err != nil {
			t.Fatal(err)
		}
		c, feed, _, in, _ := pump(t, enc)

		c.bitrate <- 16000

		// The packets that follow are the answer, so the next thing on the wire
		// is audio rather than an acknowledgement.
		feed.Publish(pcmFrame(1, 2))
		if kind, payload := nextFrame(t, in); kind != FrameAudio {
			t.Fatalf("the stream carried %d and %s after a rate it took", kind, payload)
		}
	})

	// Verify that what the capture has to say goes out between the frames.
	t.Run("News", func(t *testing.T) {
		_, feed, _, in, _ := pump(t, nil)

		feed.PublishEvent("silence", map[string]any{"seconds": 4})

		kind, payload := nextFrame(t, in)
		var got map[string]any
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatal(err)
		}
		if kind != FrameJSON || got["type"] != "silence" {
			t.Fatalf("the news came back as %d and %v", kind, got)
		}
	})

	// Verify that a frame the codec will not take ends the stream.
	t.Run("Unencodable", func(t *testing.T) {
		enc, err := opusenc.New(opusenc.MinBitrate)
		if err != nil {
			t.Fatal(err)
		}
		_, feed, _, in, done := pump(t, enc)

		// Half a frame, which the encoder refuses rather than guesses at.
		feed.Publish(audiofeed.Frame{Seq: 1, PCM: bytes.Repeat([]byte{2}, audiofeed.MonoFrameBytes/2), At: time.Now()})

		_, payload := nextFrame(t, in)
		var msg Response
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != TypeError || msg.Error == "" {
			t.Fatalf("a frame the codec refuses gave %+v", msg)
		}

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the stream never ended")
		}
	})

	// Verify that news that could not be written ends the stream.
	t.Run("NewsUnwritable", func(t *testing.T) {
		c, feed, _, _, done := pump(t, nil)

		c.net.Close()
		feed.PublishEvent("silence", map[string]any{"seconds": 4})

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the stream never ended")
		}
	})

	// Verify that a feed that ended ends the stream, whichever of its two
	// channels the stream notices first. Which one that is is decided for it,
	// so this is run enough times for both to happen.
	t.Run("FeedEnded", func(t *testing.T) {
		for range 60 {
			_, _, sub, _, done := pump(t, nil)

			sub.Close()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the stream never ended")
			}
		}
	})

	// Verify that a client that hung up ends the stream.
	t.Run("Gone", func(t *testing.T) {
		c, feed, _, _, done := pump(t, nil)

		c.net.Close()
		feed.Publish(pcmFrame(1, 2))

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the stream never ended")
		}
	})
}
