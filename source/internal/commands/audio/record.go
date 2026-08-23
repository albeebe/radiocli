// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audiogate"
	"github.com/albeebe/radiocli/internal/broker"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/portlock"
	"github.com/albeebe/radiocli/internal/recordings"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// newRecord returns the "audio record" command bound to app.
//
// Parameters:
//   - app: the application context the command reads its configuration and
//     writes its output through
//
// Returns:
//   - the "audio record" command, with its flags already registered
func newRecord(app *appcontext.App) *cobra.Command {
	var opts recordOptions

	cmd := &cobra.Command{
		Use:   "record [destination]",
		Short: "Record the scanner's transmissions to files",
		Long: "Record writes one file per transmission, with a description of what it is\n" +
			"beside it, until you stop it.\n\n" +
			"It records a scanner rather than a sound card, so it needs both: --device\n" +
			"names the radio, and the audio comes either from the input named by --input\n" +
			"or from a daemon already holding one. Naming the radio is what makes every\n" +
			"recording labelled, and it is what lets the tool check that the input really\n" +
			"is the scanner rather than a microphone in the room.\n\n" +
			"Where a recording starts is found in the audio rather than taken from when\n" +
			"the radio said it was receiving. Several seconds are kept buffered, so news\n" +
			"arriving late cannot clip the beginning of a transmission and nothing is\n" +
			"padded with silence to make sure it does not.\n\n" +
			"Each recording is a WAV, with a JSON file of the same name beside it\n" +
			"saying what it is: when it was, which channel, and how long it ran.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.destination = args[0]
			}
			return runRecord(cmd.Context(), app, opts)
		},
	}

	cmd.Flags().StringVar(&opts.input, "input", "",
		"sound input to record from, as \"radiocli audio\" names it")
	cmd.Flags().StringVar(&opts.channel, "channel", audiofeed.ChannelAuto,
		"which side of the cable the scanner is on: \"auto\", \"left\", \"right\" or \"mix\"")
	cmd.Flags().StringVar(&opts.template, "template", recordings.DefaultTemplate,
		"how each recording is named, below the destination")
	cmd.Flags().DurationVar(&opts.hang, "hang", audiogate.DefaultHang,
		"how long the scanner must stop receiving before a transmission is finished")
	cmd.Flags().DurationVar(&opts.minDuration, "min-duration", audiogate.DefaultMinDuration,
		"discard any transmission shorter than this")
	cmd.Flags().DurationVar(&opts.maxDuration, "max-duration", audiogate.DefaultMaxDuration,
		"split any transmission longer than this")

	return cmd
}

// abandon throws away a recording that cannot be finished.
//
// It is only reached when something has already gone wrong, and the file it
// removes is one with an incomplete header that nothing would play. Whatever it
// reports is dropped, because there is a real failure on its way to the caller
// already and replacing it with a complaint about the clearing up would bury
// the thing worth reading.
func (r *recorder) abandon() {
	if r.open != nil {
		_ = r.open.Abandon()
		r.open = nil
	}
}

// announceRecording says what is about to be recorded and where it is going.
//
// Parameters:
//   - app: the application context whose Stderr receives the lines
//   - source: the name of the sound input the audio is coming from
//   - dir: the destination recordings are written into
func announceRecording(app *appcontext.App, source, dir string) {
	app.Notef("Recording from %q into %s\n"+
		"One file per transmission, with a description beside it. Press Ctrl-C to stop.\n",
		source, dir)
}

// apply acts on what the gate said.
//
// Parameters:
//   - events: what the gate produced, in the order they should be acted on
//
// Returns:
//   - error if a recording cannot be opened, written or filed
func (r *recorder) apply(events []audiogate.Event) error {
	for _, ev := range events {
		if err := r.one(ev); err != nil {
			return err
		}
	}
	return nil
}

// audioViaDaemon takes a copy of the audio a daemon is already holding.
//
// Parameters:
//   - ctx: context that ends the audio when it is cancelled
//   - app: the application context holding the device setting
//
// Returns:
//   - a channel of frames, closed when the daemon stops sending
//   - the name of the sound input the daemon is recording from
//   - a function closing the stream, which is never nil
//   - error if there is no daemon or it will not send audio
func audioViaDaemon(ctx context.Context, app *appcontext.App) (
	<-chan audiofeed.Frame, string, func(), error) {

	stream, err := broker.DialAudio(app.Config.Device, formatPCM, 0)
	if err != nil {
		if errors.Is(err, broker.ErrNoDaemon) {
			return nil, "", nil, fmt.Errorf("%w.\n"+
				"Audio comes from a daemon, because a sound input can only be open once and\n"+
				"sharing it is what the daemon is for. Start one with:\n"+
				"  radiocli daemon --device %s --audio \"<sound input>\"\n"+
				"Or pass --input to open a sound input directly, without sharing it",
				err, app.Config.Device)
		}
		return nil, "", nil, err
	}

	frames := make(chan audiofeed.Frame, recordQueue)
	go func() {
		defer close(frames)
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

			select {
			case frames <- audiofeed.Frame{
				Seq:   seq,
				PCM:   audio,
				Level: audiofeed.LevelOf(audio),
				At:    time.Now(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return frames, stream.Info().Source, func() { stream.Close() }, nil
}

// contains reports whether values already holds want.
//
// Parameters:
//   - values: what has been collected so far
//   - want: the value to look for
//
// Returns:
//   - true if want is already there
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// count adds one to a tally and starts the clock if it was not running.
//
// Parameters:
//   - tally: the counter to add to
//   - at: when the frame it came from was captured
func (m *mismatch) count(tally *int, at time.Time) {
	if m.since.IsZero() {
		m.since = at
	}
	*tally++
}

// entryFrom turns a finished transmission and the labels seen during it into
// the record that is written and printed.
//
// Parameters:
//   - tx: what the gate said about the transmission
//   - seen: every reading of the radio taken while it was being recorded
//
// Returns:
//   - the entry, with everything the scanner never named left empty
func entryFrom(tx audiogate.Transmission, seen []device.Heard) recordings.Entry {
	e := recordings.Entry{
		Start:   tx.Start,
		End:     tx.End,
		Reason:  tx.Reason,
		Dropped: tx.Dropped,
		Samples: len(seen),
	}

	// The label is taken from the readings that fell inside the transmission,
	// preferring the first that named a channel. There is normally no argument
	// to settle: at three samples a second a transmission of any recordable
	// length is covered many times over, and the recorder cuts a new file when
	// the scanner moves. Every distinct channel is still listed when there was
	// more than one, because a recording naming two of them is worth knowing
	// about rather than quietly resolving.
	for _, h := range seen {
		if e.Channel == "" && h.Channel != "" {
			e.List, e.System, e.Department, e.Site = h.List, h.System, h.Department, h.Site
			e.Channel, e.Frequency, e.Talkgroup = h.Channel, h.Frequency, h.Talkgroup
			e.Modulation = h.Modulation
		}
		// The unit id is taken from wherever it appears rather than from the
		// first reading, because the scanner decodes it a moment into a
		// transmission and reports nothing there until it has.
		if e.Unit == "" {
			e.Unit = h.Unit
		}
		if h.Channel != "" && !contains(e.Channels, h.Channel) {
			e.Channels = append(e.Channels, h.Channel)
		}
	}

	// One channel is the ordinary case and says nothing worth saying, so the
	// list is only kept when it disagrees with itself.
	if len(e.Channels) < 2 {
		e.Channels = nil
	}
	return e
}

// key builds the identity the gate compares one transmission against the next
// with.
//
// It is every part of where the channel sits rather than the channel name
// alone, because two departments can each have a channel called Dispatch and
// treating those as one transmission would join two calls into a single file.
//
// Parameters:
//   - h: what the scanner is hearing
//
// Returns:
//   - an opaque identity, empty when the scanner is on nothing
func key(h device.Heard) string {
	if !h.Receiving {
		return ""
	}
	return strings.Join([]string{h.System, h.Department, h.Site, h.Channel,
		h.Frequency, h.Talkgroup}, "\x00")
}

// newSampler returns something that can be asked what the scanner is hearing.
//
// It tries the scanner directly first. Being refused because another invocation
// holds the port is the one failure worth a second attempt, because a daemon
// may be holding it precisely so that this can share: reads go over the socket
// without taking a turn, so they slip between the exchanges of whatever else is
// running rather than waiting for it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while opening the scanner
//   - app: the application context holding the device setting
//
// Returns:
//   - a function reading what the scanner is hearing
//   - a function releasing whatever was opened, which is never nil
//   - error if the scanner can be reached neither directly nor through a daemon
func newSampler(ctx context.Context, app *appcontext.App) (func(context.Context) (device.Heard, error), func(), error) {
	client, err := app.Device(ctx)
	if err == nil {
		return func(ctx context.Context) (device.Heard, error) {
			info, err := client.ScannerInfo(ctx)
			if err != nil {
				return device.Heard{}, err
			}
			return info.Heard(), nil
		}, func() {}, nil
	}
	if !errors.Is(err, portlock.ErrBusy) {
		return nil, nil, err
	}

	daemon, dialErr := broker.Dial(app.Config.Device)
	if dialErr != nil {
		// There is no daemon, so the busy scanner is the real answer and the
		// failure to find one is not worth reporting over it.
		return nil, nil, err
	}

	return func(ctx context.Context) (device.Heard, error) {
		var out bytes.Buffer
		// ModePeek runs alongside whatever has the scanner instead of queueing
		// behind it, which a reading has to do: one that waited for a menu walk
		// to finish would describe a transmission that ended long before.
		outcome, err := daemon.Run(ctx, []string{"receiving", "-o", "json"},
			broker.ModePeek, &out, io.Discard)
		if err != nil {
			return device.Heard{}, err
		}
		if outcome.Code != 0 {
			return device.Heard{}, fmt.Errorf("the daemon could not read the scanner")
		}

		var h device.Heard
		if err := json.Unmarshal(out.Bytes(), &h); err != nil {
			return device.Heard{}, fmt.Errorf("reading what the daemon reported: %w", err)
		}
		return h, nil
	}, func() { daemon.Close() }, nil
}

// observe folds one frame into the check and says something when it is sure.
//
// Parameters:
//   - app: the application context whose Stderr receives the advice
//   - frame: the audio frame just measured
//   - heard: the most recent reading of the radio
//   - floor: the noise floor the gate has settled on, in dBFS
func (m *mismatch) observe(app *appcontext.App, frame audiofeed.Frame, heard device.Heard, floor float64) {
	loud := frame.Level > floor+mismatchMargin

	switch {
	case heard.Receiving && !loud:
		m.count(&m.silent, frame.At)
	case !heard.Receiving && loud:
		m.count(&m.noisy, frame.At)
	default:
		// They agree, so whatever was being counted was momentary.
		m.silent, m.noisy, m.since = 0, 0, time.Time{}
		m.told = false
		return
	}

	if m.told || time.Since(m.since) < mismatchWindow {
		return
	}

	switch {
	case m.silent >= mismatchLimit:
		m.told = true
		app.Notef("The scanner says it is receiving and nothing is arriving on the audio input.\n" +
			"Check the cable is in the scanner's headphone or record socket and in the input\n" +
			"you named, and check the scanner's volume is not at zero.\n")
	case m.noisy >= mismatchLimit:
		m.told = true
		app.Notef("Sound is arriving while the scanner says it is receiving nothing.\n" +
			"The input named is probably not the scanner. Run \"radiocli audio\" to see what\n" +
			"else this computer can record from.\n")
	}
}

// observe folds one frame into the meter and logs a reading when enough have
// gone by.
//
// The two numbers together are what diagnoses a cable, and having them saved
// this feature from shipping broken: an input whose noise sat at -78 dBFS was
// being triggered by its own hiss, and the reason was visible the moment the
// floor and the level were printed next to each other. A level barely above the
// floor is a lead in the wrong socket or a scanner turned down; a floor pinned
// near the top of its range is a microphone.
//
// Parameters:
//   - app: the application context whose logger receives the reading
//   - frame: the audio frame just measured
//   - floor: the noise floor the gate has settled on, in dBFS
func (m *meter) observe(app *appcontext.App, frame audiofeed.Frame, floor float64) {
	if m.seen == 0 || frame.Level > m.peak {
		m.peak = frame.Level
	}
	m.seen++

	if m.seen < meterEvery {
		return
	}
	app.Log.Debug("audio", "peak", math.Round(m.peak*10)/10, "floor", math.Round(floor*10)/10)
	m.seen, m.peak = 0, 0
}

// one acts on a single thing the gate said.
//
// Parameters:
//   - ev: what the gate said
//
// Returns:
//   - error if the recording cannot be opened, written or filed
func (r *recorder) one(ev audiogate.Event) error {
	switch ev.Kind {
	case audiogate.KindStart:
		// The gate only says this once a transmission has outlived the minimum
		// length, so a file opened here never has to be deleted again.
		open, err := r.library.Begin()
		if err != nil {
			return err
		}
		r.open = open
		return nil

	case audiogate.KindAudio:
		if r.open == nil {
			// Audio with nothing open cannot happen: the gate emits a start
			// before any frame of a transmission. Guarded rather than trusted,
			// because the alternative is a nil dereference in the middle of a
			// night's recording.
			return nil
		}
		return r.open.Write(ev.Frame.PCM)

	default:
		if r.open == nil {
			return nil
		}
		open := r.open
		r.open = nil

		filed, err := open.Close(entryFrom(ev.Tx, r.seen))
		r.seen = nil
		if err != nil {
			return err
		}
		return reportRecording(r.app, filed)
	}
}

// openAudio starts the audio arriving, either from a sound card this process
// opens or from a daemon already holding one.
//
// Parameters:
//   - ctx: context that ends the audio when it is cancelled
//   - app: the application context holding the device setting and the logger
//   - input: the sound input to open, empty to take the audio from a daemon
//   - channel: the side of the cable to take, as audiofeed.ParseChannel gave it
//
// Returns:
//   - a channel of frames, closed when the audio ends
//   - the name of the sound input the audio is coming from
//   - a function releasing whatever was opened, which is never nil
//   - error if the sound input or the daemon cannot be reached
func openAudio(ctx context.Context, app *appcontext.App, input, channel string) (
	<-chan audiofeed.Frame, string, func(), error) {

	if input == "" {
		return audioViaDaemon(ctx, app)
	}

	feed := audiofeed.New(app.Log)
	capture, err := startCapture(audiofeed.Options{Source: input, Channel: channel, Log: app.Log}, feed)
	if err != nil {
		return nil, "", nil, err
	}

	sub := feed.Subscribe(recordQueue)

	// The feed has things to say that are not audio, and every one of them is
	// about the recording being wrong rather than about the radio: a cable in
	// the wrong socket, a permission never granted, two sides that cancel. They
	// reach a person here or not at all.
	go func() {
		for ev := range sub.Events() {
			report(app, ev)
		}
	}()

	return sub.Frames(), capture.Source(), func() {
		capture.Close()
		sub.Close()
	}, nil
}

// poll asks the scanner what it is hearing, over and over, until ctx ends.
//
// Parameters:
//   - ctx: context that stops the polling when it is cancelled
//   - sample: reads what the scanner is hearing
//   - heard: where readings are sent
//   - failures: where a scanner that stopped answering is reported. It must
//     have room for one, which is all this ever sends, so that reporting a
//     failure on the way out cannot block on a recorder that has already gone
func poll(ctx context.Context, sample func(context.Context) (device.Heard, error),
	heard chan<- device.Heard, failures chan<- error) {

	tick := time.NewTicker(samplePeriod)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}

		h, err := sample(ctx)
		if err != nil {
			// A cancelled run is the tool being stopped rather than the radio
			// going away, and reporting it would end the recording with an
			// error on every ordinary Ctrl-C.
			if ctx.Err() == nil {
				failures <- err
			}
			return
		}

		select {
		case heard <- h:
		case <-ctx.Done():
			return
		default:
			// The recorder is busy with a frame. Dropping this reading costs
			// nothing, because another arrives a tenth of a second later
			// saying the same thing: what a recording is cut against is the
			// time the radio was last seen receiving, not any single reading,
			// so losing one moves nothing.
		}
	}
}

// recordLoop drives the gate with the audio arriving and files what it says.
//
// Parameters:
//   - ctx: context that ends the recording when it is cancelled
//   - app: the application context holding the output streams
//   - library: where finished recordings are written
//   - frames: the audio arriving
//   - sample: reads what the scanner is hearing
//   - opts: what the flags asked for
//
// Returns:
//   - error if a recording cannot be written or the scanner stops answering;
//     nil once ctx is cancelled or the audio ends
func recordLoop(ctx context.Context, app *appcontext.App, library *recordings.Library,
	frames <-chan audiofeed.Frame, sample func(context.Context) (device.Heard, error),
	opts recordOptions) error {

	r := &recorder{
		app:     app,
		library: library,
		gate: audiogate.New(audiogate.Options{
			Hang:        opts.hang,
			MinDuration: opts.minDuration,
			MaxDuration: opts.maxDuration,
			// This command always has a scanner, so the scanner decides what
			// is a transmission. Letting the audio decide as well is what
			// produced sixteen second recordings of a noise floor.
			RequireRadio: true,
		}),
	}

	heard := make(chan device.Heard, 1)
	failures := make(chan error, 1)
	go poll(ctx, sample, heard, failures)

	var (
		last  device.Heard
		watch mismatch
		level meter
	)

	for {
		select {
		case <-ctx.Done():
			// Anything still being recorded is finished properly rather than
			// abandoned, so stopping part way through a transmission keeps the
			// part that happened.
			return r.apply(r.gate.Flush())

		case err := <-failures:
			// The radio going away is the end of the run. Carrying on would
			// mean writing files that claim to be scanner recordings with
			// nothing confirming that they are, which is the situation
			// requiring a scanner exists to prevent.
			r.abandon()
			return fmt.Errorf("the scanner stopped answering, so recording has stopped: %w", err)

		case h := <-heard:
			last = h
			r.gate.Activity(time.Now(), audiogate.Activity{On: h.Receiving, Key: key(h)})
			if h.Receiving {
				r.seen = append(r.seen, h)
			}

		case frame, ok := <-frames:
			if !ok {
				// The audio stopping is treated the same as being interrupted:
				// what has been heard so far is worth keeping.
				return r.apply(r.gate.Flush())
			}
			watch.observe(app, frame, last, r.gate.Floor())
			level.observe(app, frame, r.gate.Floor())

			if err := r.apply(r.gate.Offer(frame)); err != nil {
				r.abandon()
				return err
			}
		}
	}
}

// reportRecording prints one finished recording.
//
// The audio goes to files, so stdout is free for this, which is what makes the
// command usable as a live feed of what the scanner is hearing. Under
// --output json it is the same object that was written beside the audio, so
// anything reading one has learned to read the other.
//
// Parameters:
//   - app: the application context to write through
//   - e: the recording as it was filed
//
// Returns:
//   - error if the JSON cannot be written
func reportRecording(app *appcontext.App, e recordings.Entry) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, e)
	}

	app.Printf("%s  %5.1fs  %s\n", e.Start.Format("15:04:05"), e.Duration,
		render.Dash(strings.TrimSpace(strings.Join([]string{e.System, e.Department, e.Channel}, " "))))
	return nil
}

// runRecord records transmissions until ctx is cancelled.
//
// Parameters:
//   - ctx: context that ends the recording when it is cancelled
//   - app: the application context holding the streams, the logger and the
//     scanner connection
//   - opts: what the flags asked for
//
// Returns:
//   - error if the scanner or the audio cannot be reached, if the destination
//     cannot be prepared, or if a recording cannot be written; nil once ctx is
//     cancelled
func runRecord(ctx context.Context, app *appcontext.App, opts recordOptions) error {
	// A daemon lends a command its streams for as long as the command runs, on
	// the reasonable assumption that a command finishes. This one does not, so
	// inside a daemon it would hold those streams forever. Refused here rather
	// than merely left undocumented, because a client can send any command line.
	if app.InDaemon {
		return errors.New("\"audio record\" runs until it is stopped, so it cannot be run " +
			"inside a daemon:\nrun it in a terminal of its own instead")
	}

	// The radio is required, and is checked before anything is opened. This
	// command records a scanner rather than a sound card, and without the radio
	// there is nothing to label a recording with and no way to tell the
	// scanner's audio from a microphone in the room.
	if app.Config.Device == "" {
		return fmt.Errorf("%w: \"audio record\" needs the scanner as well as its audio, so that "+
			"every recording is labelled and so that the input can be checked against the "+
			"radio.\nName one with --device, from the PORT column of \"radiocli devices\"",
			appcontext.ErrNoDevice)
	}

	channel, err := audiofeed.ParseChannel(opts.channel)
	if err != nil {
		return err
	}

	// Everything a typo can be caught by is caught before anything is opened,
	// so a mistake costs a second rather than a night's recording, and a run
	// that is never going to happen leaves no folder behind.
	if err := recordings.ValidateTemplate(opts.template); err != nil {
		return err
	}

	sample, closeSampler, err := newSampler(ctx, app)
	if err != nil {
		return err
	}
	defer closeSampler()

	destination := opts.destination
	if destination == "" {
		destination = "recordings"
	}
	library, err := recordings.New(destination, opts.template)
	if err != nil {
		return err
	}

	if left, err := library.Sweep(); err == nil && len(left) > 0 {
		app.Notef("%d recording(s) in %s were left unfinished by an earlier run and will not "+
			"play. They are the hidden files beginning %q, and can be deleted.\n",
			len(left), library.Dir(), ".partial-")
	}

	frames, source, closeAudio, err := openAudio(ctx, app, opts.input, channel)
	if err != nil {
		return err
	}
	defer closeAudio()

	announceRecording(app, source, library.Dir())
	return recordLoop(ctx, app, library, frames, sample, opts)
}
