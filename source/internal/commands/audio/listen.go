// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/23/2026

package audio

import (
	"context"
	"errors"
	"fmt"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audiogate"
	"github.com/spf13/cobra"
)

// newListen returns the "audio listen" command bound to app.
//
// Parameters:
//   - app: the application context the command reads its configuration and
//     writes its output through
//
// Returns:
//   - the "audio listen" command, with its flags already registered
func newListen(app *appcontext.App) *cobra.Command {
	var opts listenOptions

	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Play the scanner's audio on this computer's speakers",
		Long: "Listen plays the scanner on this computer, until you stop it.\n\n" +
			"By default it plays only the transmissions. The hiss between them is not\n" +
			"worth listening to for an evening, and the same detector that finds them for\n" +
			"\"audio record\" decides when to open the speakers. It opens them on the first\n" +
			"frame above the noise floor rather than waiting to be sure, so nothing of the\n" +
			"first word is lost. Pass --squelch=false to hear the input exactly as it\n" +
			"arrives, hiss included, the way the scanner's own speaker does.\n\n" +
			"The audio comes from a daemon, which is what lets this run while something\n" +
			"else is recording the same radio: a sound input can only be open once, and\n" +
			"sharing it is the daemon's whole job. --input opens a sound input directly\n" +
			"instead, which is for checking a cable, and nothing else can listen while it\n" +
			"does.\n\n" +
			"Run \"radiocli audio\" to see what this computer can record from and play on.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListen(cmd.Context(), app, opts)
		},
	}

	cmd.Flags().StringVar(&opts.input, "input", "",
		"sound input to listen to directly, as \"radiocli audio\" names it")
	cmd.Flags().StringVar(&opts.channel, "channel", audiofeed.ChannelAuto,
		"which side of the cable the scanner is on: \"auto\", \"left\", \"right\" or \"mix\"")
	cmd.Flags().StringVar(&opts.speaker, "speaker", "",
		"speakers to play on, as \"radiocli audio\" names them (default: this computer's own)")
	cmd.Flags().BoolVar(&opts.squelch, "squelch", true,
		"play only the transmissions; --squelch=false plays everything the input carries")
	cmd.Flags().DurationVar(&opts.hang, "hang", audiogate.DefaultQuietHang,
		"how long the audio must stay quiet before the speakers close again")
	cmd.Flags().Float64Var(&opts.gain, "gain", 0,
		"decibels to turn the audio up by on the way to the speakers")

	return cmd
}

// announceListening says what is being played and where.
//
// To stderr, like every other running commentary in this package, so that a
// person watching sees it and anything reading stdout is unaffected.
//
// Parameters:
//   - app: the application context whose Stderr receives the line
//   - source: the name of the sound input the audio is coming from
//   - speaker: the name of the output being played on, empty for one the
//     library did not name
//   - squelch: whether only the transmissions are being played
func announceListening(app *appcontext.App, source, speaker string, squelch bool) {
	// An empty name is the system's own choice of output, which the library
	// does not always put a name to. "Playing on " is not a sentence, so it
	// gets a description instead of a name.
	where := fmt.Sprintf("%q", speaker)
	if speaker == "" {
		where = "this computer's own speakers"
	}

	what := "everything the input carries"
	if squelch {
		what = "the transmissions"
	}

	app.Notef("Playing %s from %q on %s. Press Ctrl-C to stop.\n", what, source, where)
}

// listenLoop plays what arrives until ctx is cancelled or the audio ends.
//
// The frames played are the ones that have just arrived, and the gate is asked
// only whether they are live. Neither of the things the gate emits is any use
// here: it holds audio for the length of the hang so that it can trim the end
// of a recording, and it does not announce a transmission at all until one has
// proved itself long enough to be worth a file. Both are right for a file and
// wrong for a speaker, where late is missing.
//
// Parameters:
//   - ctx: context that ends the listening when it is cancelled
//   - app: the application context holding the logger
//   - frames: the audio arriving
//   - p: the speakers to play on
//   - opts: what the flags asked for
//
// Returns:
//   - nil once ctx is cancelled or the audio ends, which are both ordinary ways
//     for this to finish
func listenLoop(ctx context.Context, app *appcontext.App, frames <-chan audiofeed.Frame,
	p player, opts listenOptions) error {

	// Only built when it is going to be asked something. Without the squelch
	// every frame is played, and a detector nobody consults is work done for
	// nothing on every frame of the evening.
	var gate *audiogate.Gate
	if opts.squelch {
		// MinDuration and MaxDuration are left at their defaults and never
		// matter: they decide what is worth a file, and nothing here is
		// keeping anything.
		gate = audiogate.New(audiogate.Options{
			Hang: opts.hang,
			// No radio to ask. This command does not need the scanner at all,
			// and requiring it would mean nobody could listen to a cable
			// without one plugged in.
			RequireRadio: false,
		})
	}

	var meter playbackMeter

	for {
		select {
		case <-ctx.Done():
			return nil

		case frame, ok := <-frames:
			if !ok {
				// The audio stopping is how this ends when a daemon goes away
				// or a capture is closed. Nothing was being kept, so there is
				// nothing to finish.
				return nil
			}

			if gate == nil {
				p.Play(frame.PCM)
				meter.observe(app, p, true)
				continue
			}

			// Offered for its own sake: the gate has to see every frame to
			// track the noise floor and to know a transmission is running. The
			// events it produces are dropped on the floor, and Live is the
			// question worth asking of it here.
			gate.Offer(frame)

			live := gate.Live(frame)
			if live {
				p.Play(frame.PCM)
			}
			meter.observe(app, p, live)
		}
	}
}

// observe counts one frame and, once a second's worth have gone by, says what
// became of them.
//
// At debug, because it is a reading a second and belongs in a log rather than
// on somebody's terminal. It is the same shape as the level meter the recorder
// keeps, and for the same reason: a fault that only shows up on real traffic
// has to be visible while the traffic is happening.
//
// Parameters:
//   - app: the application context holding the logger
//   - p: the speakers being played, for the counts they keep
//   - played: whether this frame was handed to them
func (m *playbackMeter) observe(app *appcontext.App, p player, played bool) {
	m.seen++
	if played {
		m.played++
	}
	if m.seen < meterFrames {
		return
	}

	stats := p.Stats()
	app.Log.Debug("playback",
		"played", fmt.Sprintf("%d/%d", m.played, m.seen),
		"starved", stats.Starved-m.last.Starved,
		"dropped", stats.Dropped-m.last.Dropped)

	m.played, m.seen, m.last = 0, 0, stats
}

// reportPlayback says what the speakers had to do to keep up, on the way out.
//
// Both numbers are said, because between them they name the two ways playing
// can go wrong and neither is visible while it is happening: audio that never
// reached the speakers, and holes where the speakers had nothing to play. What
// somebody hears in either case is choppy audio, and until this was printed the
// only way to tell that from a bad cable was to open the recording and find it
// perfect.
//
// Running dry needs the yardstick that goes with it. With the squelch on the
// audio stops between transmissions, so the speakers run dry once per
// transmission by design, and only a count far above that means the audio was
// arriving in bursts too big to smooth out.
//
// Parameters:
//   - app: the application context whose Stderr and logger receive it
//   - p: the speakers that have been playing
func reportPlayback(app *appcontext.App, p player) {
	stats := p.Stats()
	app.Log.Debug("playback finished",
		"played", float64(stats.Played)/float64(playedBytesPerSecond),
		"dropped", stats.Dropped, "starved", stats.Starved)

	if stats.Dropped > 0 {
		app.Notef("%.1f seconds of audio arrived faster than the speakers could play it and "+
			"was dropped.\nThis computer is struggling to keep up, or the sound output is.\n",
			float64(stats.Dropped)/float64(playedBytesPerSecond))
	}

	if stats.Starved > 0 {
		app.Notef("The speakers ran dry %d time(s), and played silence until the audio caught "+
			"up.\nOnce per transmission is expected, since the audio stops between them. Many\n"+
			"more than that is what choppy playback sounds like.\n", stats.Starved)
	}
}

// runListen checks what was asked for and starts playing.
//
// Parameters:
//   - ctx: context that ends the listening when it is cancelled
//   - app: the application context holding the configuration and the streams
//   - opts: what the flags asked for
//
// Returns:
//   - error if the command was run inside a daemon, if the channel or the
//     speakers are not something this can use, or if the audio cannot be
//     reached; nil once ctx is cancelled
func runListen(ctx context.Context, app *appcontext.App, opts listenOptions) error {
	// A daemon lends a command its streams for as long as the command runs, on
	// the reasonable assumption that a command finishes. This one does not, so
	// inside a daemon it would hold those streams forever and play the radio
	// out of whichever machine the daemon is on rather than the one that asked.
	if app.InDaemon {
		return errors.New("\"audio listen\" runs until it is stopped, so it cannot be run " +
			"inside a daemon:\nrun it in a terminal of its own instead")
	}

	if opts.input == "" && app.Config.Device == "" {
		return fmt.Errorf("%w: name one with --device, or pass --input to listen to a sound "+
			"input directly", appcontext.ErrNoDevice)
	}

	if opts.hang <= 0 {
		return fmt.Errorf("a hang of %s is not a length of quiet to wait for", opts.hang)
	}

	channel, err := audiofeed.ParseChannel(opts.channel)
	if err != nil {
		return err
	}

	// The speakers are opened before the audio is asked for, so that a typo in
	// --speaker costs a moment rather than a sound card being taken and given
	// straight back.
	p, err := openPlayer(opts.speaker)
	if err != nil {
		return err
	}
	defer p.Close()
	p.SetGain(opts.gain)

	frames, source, closeAudio, err := openAudio(ctx, app, opts.input, channel)
	if err != nil {
		return err
	}
	defer closeAudio()

	announceListening(app, source, p.Name(), opts.squelch)
	defer reportPlayback(app, p)

	return listenLoop(ctx, app, frames, p, opts)
}
