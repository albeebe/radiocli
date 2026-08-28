// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/broker"
)

// Where the audio comes from, for the two commands that want it as frames.
//
// "audio record" and "audio listen" both need the same thing: 20 ms frames of
// mono, from a daemon by default and from a named sound input when one is
// given. They are here rather than in either command because they belong to
// neither, and because the daemon-or-direct decision is the one thing about
// this that must not drift between them.
//
// "audio output" is deliberately not on this path. It passes the daemon's bytes
// on exactly as they arrived, Opus packets included, and turning those into
// frames only to write them back out would mean decoding audio for the sake of
// re-encoding it.

// audioViaDaemon takes a copy of the audio a daemon is already holding.
//
// Parameters:
//   - ctx: context that ends the audio when it is cancelled
//   - app: the application context holding the device setting
//
// Returns:
//   - a channel of frames, closed when the daemon stops sending
//   - a shallow channel of the same frames for the speakers, nil unless one
//     was asked for
//   - the name of the sound input the daemon is recording from
//   - a function closing the stream, which is never nil
//   - error if there is no daemon or it will not send audio
func audioViaDaemon(ctx context.Context, app *appcontext.App, speakers bool) (
	<-chan audiofeed.Frame, <-chan audiofeed.Frame, string, func(), error) {

	stream, err := broker.DialAudio(app.Config.Device, formatPCM, 0)
	if err != nil {
		if errors.Is(err, broker.ErrNoDaemon) {
			return nil, nil, "", nil, fmt.Errorf("%w.\n"+
				"Audio comes from a daemon, because a sound input can only be open once and\n"+
				"sharing it is what the daemon is for. Start one with:\n"+
				"  radiocli daemon --device %s --audio \"<sound input>\"\n"+
				"Or pass --input to open a sound input directly, without sharing it",
				err, app.Config.Device)
		}
		return nil, nil, "", nil, err
	}

	frames := make(chan audiofeed.Frame, recordQueue)

	// The speakers get their own shallow queue rather than a place in the deep
	// one. See playQueue: what is late to a disk is worth keeping and what is
	// late to a person is not.
	var play chan audiofeed.Frame
	if speakers {
		play = make(chan audiofeed.Frame, playQueue)
	}

	go func() {
		defer close(frames)
		if play != nil {
			defer close(play)
		}
		// The daemon sends samples with no level and no timestamp, so both are
		// worked out here. The level is measured with the same function the
		// capture uses, because a gate tuned against one definition of loudness
		// must not behave differently depending on where its audio came from.
		for {
			seq, audio, event, err := stream.Next()
			if err != nil {
				return
			}
			if event != nil {
				relayEvent(app, event)
				continue
			}

			frame := audiofeed.Frame{
				Seq:   seq,
				PCM:   audio,
				Level: audiofeed.LevelOf(audio),
				At:    time.Now(),
			}

			// Offered to the speakers first and never waited on. A full queue
			// means they are behind, and the cure for that is the newest frame
			// rather than the oldest, so the oldest is thrown away to make room.
			if play != nil {
				select {
				case play <- frame:
				default:
					select {
					case <-play:
					default:
					}
					select {
					case play <- frame:
					default:
					}
				}
			}

			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()

	return frames, play, stream.Info().Source, func() { stream.Close() }, nil
}

// openAudio starts the audio arriving, either from a sound card this process
// opens or from a daemon already holding one.
//
// Parameters:
//   - ctx: context that ends the audio when it is cancelled
//   - app: the application context holding the device setting and the logger
//   - input: the sound input to open, empty to take the audio from a daemon
//   - channel: the side of the cable to take, as audiofeed.ParseChannel gave it
//   - speakers: whether a second, shallow stream is wanted for playing
//
// Returns:
//   - a channel of frames, closed when the audio ends
//   - a shallow channel of the same frames for the speakers, nil unless one was
//     asked for
//   - the name of the sound input the audio is coming from
//   - a function releasing whatever was opened, which is never nil
//   - error if the sound input or the daemon cannot be reached
func openAudio(ctx context.Context, app *appcontext.App, input, channel string, speakers bool) (
	<-chan audiofeed.Frame, <-chan audiofeed.Frame, string, func(), error) {

	if input == "" {
		return audioViaDaemon(ctx, app, speakers)
	}

	feed := audiofeed.New(app.Log)

	// Subscribed before the card is opened, not after. The capture has things
	// to say the moment it starts, and the loudest of them, that the two sides
	// of the cable cancel, is decided in the first few seconds. A subscription
	// made afterwards can miss the news it exists to carry.
	sub := feed.Subscribe(recordQueue)

	// A subscriber of its own for the speakers, shallow, so that being late
	// costs them the frames they missed rather than a place at the back of the
	// recorder's queue. See playQueue.
	var play *audiofeed.Sub
	if speakers {
		play = feed.Subscribe(playQueue)
	}

	// The feed has things to say that are not audio, and every one of them is
	// about the recording being wrong rather than about the radio: a cable in
	// the wrong socket, a permission never granted, two sides that cancel. They
	// reach a person here or not at all.
	go func() {
		for ev := range sub.Events() {
			report(app, ev)
		}
	}()

	capture, err := startCapture(audiofeed.Options{Source: input, Channel: channel, Log: app.Log}, feed)
	if err != nil {
		sub.Close()
		if play != nil {
			play.Close()
		}
		return nil, nil, "", nil, err
	}

	var playFrames <-chan audiofeed.Frame
	if play != nil {
		playFrames = play.Frames()
	}

	return sub.Frames(), playFrames, capture.Source(), func() {
		capture.Close()
		sub.Close()
		if play != nil {
			play.Close()
		}
	}, nil
}

// quiet returns n bytes of silence to hand to the speakers.
//
// It exists so that playback with the squelch on keeps handing the speakers
// something between transmissions. The ring behind them will not play until it
// holds a whole buffer and gives that up whenever it empties, so a gap in what
// it is handed is paid for twice: once in the gap, and again in the buffer it
// has to rebuild before anything is heard. Silence rather than the frame is
// what keeps the hiss out while still keeping the ring fed.
//
// The same backing array is handed out every time, which is safe because the
// player copies what it is given and never writes to it, and because nothing
// here ever hands out a longer slice than the one frame it was asked for. A
// frame arrives every 20 ms, so allocating one would be 50 a second for the
// length of a run.
//
// Parameters:
//   - n: how many bytes of silence are wanted
//
// Returns:
//   - a slice of n zero bytes
func quiet(n int) []byte {
	if n > len(silence) {
		return make([]byte, n)
	}
	return silence[:n]
}
