// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audiogate"
	"github.com/albeebe/radiocli/internal/audioout"
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
			"saying what it is: when it was, which channel, and how long it ran.\n\n" +
			"Pass --listen to hear each transmission on this computer's speakers while it\n" +
			"is being recorded. That is the same audio, played rather than written, so\n" +
			"nothing has to be opened twice.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.destination = args[0]
			}
			opts.bufferSet = cmd.Flags().Changed("buffer")
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
	cmd.Flags().BoolVar(&opts.normalize, "normalize", true,
		"scale each recording up so its loudest moment is just under full scale; "+
			"--normalize=false keeps the level exactly as it arrived")
	cmd.Flags().BoolVar(&opts.listen, "listen", false,
		"play each transmission on this computer's speakers as it is recorded")
	cmd.Flags().StringVar(&opts.speaker, "speaker", "",
		"speakers to play on with --listen, as \"radiocli audio\" names them "+
			"(default: this computer's own)")
	cmd.Flags().DurationVar(&opts.buffer, "buffer", audioout.DefaultBuffer,
		"how much audio to keep between the radio and the speakers with --listen: bigger "+
			"plays more smoothly, smaller plays sooner")
	cmd.Flags().Float64Var(&opts.gain, "gain", 0,
		"decibels to turn the audio up by with --listen, which does not change what is "+
			"recorded")

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
//   - p: the speakers each transmission is being played on as well, nil when
//     nobody asked to hear them
func announceRecording(app *appcontext.App, source, dir string, p player) {
	app.Notef("Recording from %q into %s\n"+
		"One file per transmission, with a description beside it. Press Ctrl-C to stop.\n",
		source, dir)

	if p == nil {
		return
	}

	// The name is empty when the system's own choice of output was opened and
	// the library did not put a name to it.
	where := fmt.Sprintf("%q", p.Name())
	if p.Name() == "" {
		where = "this computer's own speakers"
	}
	app.Notef("Playing each transmission on %s as it is recorded.\n", where)
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
	// Every reading of the digital format is collected rather than the first
	// one taken, because they are not all answers to the same question. See
	// mostReported.
	var formats []string
	var radios []string

	for _, h := range seen {
		if e.Channel == "" && h.Channel != "" {
			e.List, e.System, e.Department, e.Site = h.List, h.System, h.Department, h.Site
			e.Channel, e.Frequency, e.Talkgroup = h.Channel, h.Frequency, h.Talkgroup
			e.Modulation = h.Modulation
		}

		// Collected rather than settled here. The scanner reports nothing until
		// it has decoded something, so the first answer cannot simply be taken,
		// and it cannot simply be trusted either.
		if h.Digital != "" {
			formats = append(formats, h.Digital)
		}
		// Collected rather than taken from the first reading. A recording is
		// cut when the transmitting radio changes, but the identifier arrives
		// a moment after the audio does, so the first readings of a recording
		// can still name whoever was talking a second ago. What most of it
		// agreed on is the radio the recording is of.
		if h.Unit != "" {
			radios = append(radios, h.Unit)
		}
		// The same goes for the network access code, which is decoded off the
		// site rather than off the channel and can arrive a moment late.
		if e.NAC == "" {
			e.NAC = h.NAC
		}
		// The strongest reading rather than the first, since a transmission
		// rises and falls across its length. Compared as numbers because these
		// are negative and "-100" sorts before "-97" as text.
		if stronger(h.RSSI, e.RSSI) {
			e.RSSI = h.RSSI
		}
		if h.Channel != "" && !contains(e.Channels, h.Channel) {
			e.Channels = append(e.Channels, h.Channel)
		}
	}

	e.Digital = mostReported(formats)
	e.Unit = mostReported(radios)

	// One channel is the ordinary case and says nothing worth saying, so the
	// list is only kept when it disagrees with itself.
	if len(e.Channels) < 2 {
		e.Channels = nil
	}
	return e
}

// mostReported returns the value the readings agreed on most often.
//
// The digital format is the one label here that is a fresh observation on every
// reading rather than a fact about the channel, so the first answer is not
// necessarily the best one. Taking it was measured going wrong: a 29 second
// transmission on a P25 channel was labelled "Link", a value the protocol
// documentation does not even list, because one early reading said so and the
// three hundred after it were never consulted.
//
// The cost is that a brief but correct reading can be outvoted by a longer
// wrong one, which is the better way round: a transmission is labelled by what
// it mostly was.
//
// Ties go to whichever was seen first, so that a transmission genuinely
// carrying two formats is labelled by the one it started with rather than by
// however the counting happened to be ordered.
//
// Parameters:
//   - values: every reading taken during the transmission, in order, with the
//     empty ones already left out
//
// Returns:
//   - the most frequent value, or empty if there were none
func mostReported(values []string) string {
	type tally struct {
		value string
		count int
	}

	var counts []tally
	for _, v := range values {
		found := false
		for i := range counts {
			if counts[i].value == v {
				counts[i].count++
				found = true
				break
			}
		}
		if !found {
			counts = append(counts, tally{value: v, count: 1})
		}
	}

	best := ""
	most := 0
	for _, c := range counts {
		if c.count > most {
			best, most = c.value, c.count
		}
	}
	return best
}

// stronger reports whether one signal reading beats another.
//
// The readings arrive as text, they are negative, and "-999" is the scanner
// saying it has nothing to report rather than a measurement. Comparing them as
// text puts "-100" above "-97", which is backwards, so both are read as numbers
// and anything unreadable loses.
//
// Parameters:
//   - candidate: the reading being offered
//   - best: the strongest reading so far, empty when there is none yet
//
// Returns:
//   - true if candidate is a real reading and beats best
func stronger(candidate, best string) bool {
	got, err := strconv.Atoi(strings.TrimSpace(candidate))
	if err != nil || got <= noSignalRSSI {
		return false
	}
	if best == "" {
		return true
	}
	have, err := strconv.Atoi(strings.TrimSpace(best))
	return err != nil || got > have
}

// key builds the identity the gate compares one transmission against the next
// with.
//
// It is every part of where the channel sits rather than the channel name
// alone, because two departments can each have a channel called Dispatch and
// treating those as one transmission would join two calls into a single file.
//
// A mark for the transmitting radio is part of it too, and that is what
// separates two people talking on one channel. The gate's other way of finding
// the boundary is the radio's mute closing, which works on a simplex channel
// and fails on a repeater: the carrier stays up between overs, the mute never
// closes, and two speakers land in one recording. Measured on a live P25
// channel, a 22 second recording held two seconds of one radio and twenty of
// the dispatcher answering it. Nothing in the audio marks that boundary, and
// the identifier changes exactly on it.
//
// It is a mark rather than the identifier itself, and the difference matters.
// The identifier is not known when a transmission starts: it arrives a moment
// after the audio, so putting it here directly made learning who was talking
// look identical to somebody else starting to talk, and every transmission was
// cut into a fragment and a remainder. Measured against a live channel, that
// turned three transmissions into five. The mark only changes when one known
// radio is replaced by a different known one, which is the only thing that is
// actually a boundary.
//
// Parameters:
//   - h: what the scanner is hearing
//   - speaker: the mark for the radio talking, from the caller which watches
//     the identifier change
//
// Returns:
//   - an opaque identity, empty when the scanner is on nothing
func key(h device.Heard, speaker string) string {
	if !h.Receiving {
		return ""
	}
	return strings.Join([]string{h.System, h.Department, h.Site, h.Channel,
		h.Frequency, h.Talkgroup, speaker}, "\x00")
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
		// Hearing rather than ScannerInfo, because on a conventional digital
		// channel the transmitting radio is only on the screen. See its doc.
		return client.Hearing, func() {}, nil
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

// monitor plays the frame that has just arrived, if it is part of a
// transmission and somebody asked to hear it.
//
// Nothing is playing unless --listen asked for it, so the usual case is the
// first line and nothing else.
//
// Three things could decide what reaches the speakers here, and only one of
// them is right. The audio the gate hands back is a hang behind the radio,
// because it is held so that the end of a recording can be trimmed. The
// recording being open is most of a second behind, because a file is not opened
// until the transmission has proved itself worth one. Both were tried, and both
// sound like holes rather than like a radio. What is played is the frame that
// has just arrived, while the gate says a transmission is live, which starts at
// the first frame above the noise floor.
//
// The consequence is that the speakers and the files no longer agree exactly: a
// blip too short to be worth keeping is heard and not written. That is the
// right way round. A scanner's own speaker plays every blip, and somebody
// listening while they record is listening to the radio rather than auditioning
// the disk.
//
// Parameters:
//   - frame: the audio that has just arrived, already offered to the gate
func (r *recorder) monitor(frame audiofeed.Frame) {
	if r.player == nil {
		return
	}

	live := r.gate.Live(frame)
	if live {
		r.player.Play(frame.PCM)
	}
	r.speakers.observe(r.app, r.player, live)
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
		r.clipped, r.samples = 0, 0
		return nil

	case audiogate.KindAudio:
		if r.open == nil {
			// Audio with nothing open cannot happen: the gate emits a start
			// before any frame of a transmission. Guarded rather than trusted,
			// because the alternative is a nil dereference in the middle of a
			// night's recording.
			return nil
		}
		r.count(ev.Frame.PCM)
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
		if err := reportRecording(r.app, filed); err != nil {
			return err
		}
		r.warnIfClipped()
		return nil
	}
}

// volumeNow reads the scanner's volume for the clipping warning to quote.
//
// Every failure is the same answer here. The volume is a nicety on a warning
// about something else, so a scanner that is busy, absent, or simply unwilling
// to say costs the warning one sentence rather than costing the run anything.
//
// Parameters:
//   - ctx: context for the exchange with the scanner
//   - app: the application context holding the scanner connection
//
// Returns:
//   - the volume level, or -1 when it could not be read
func volumeNow(ctx context.Context, app *appcontext.App) int {
	client, err := app.Device(ctx)
	if err != nil {
		return -1
	}
	level, err := client.Volume(ctx)
	if err != nil {
		return -1
	}
	return level
}

// count folds one frame of audio into the clipping tally for the open
// recording.
//
// Parameters:
//   - pcm: signed 16-bit little-endian mono samples
func (r *recorder) count(pcm []byte) {
	for i := 0; i+1 < len(pcm); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(pcm[i : i+2]))
		r.samples++
		if sample >= clipCeiling || sample <= -clipCeiling {
			r.clipped++
		}
	}
}

// warnIfClipped says so when the recording just filed was overloaded on the
// way in.
//
// It is said again for every recording rather than once for the run, because
// the warning is only useful next to the thing it is about. A line that
// scrolled past twenty transmissions ago does not tell somebody that the one
// they are looking at is distorted, and the whole point of repeating it is that
// it stops the moment the volume comes down.
//
// The advice names the volume rather than a target level, because how far down
// is far enough depends on the sound card and the only honest instruction is to
// lower it until this stops.
//
// The level is read here rather than remembered from the start of the run, so
// that somebody who just turned the volume down is told the number they set
// rather than the one they replaced.
//
// It says to turn the radio down rather than naming "radiocli volume set",
// which is the obvious thing to write and does not work. This command holds the
// serial port for as long as it runs, so that invocation is refused as busy by
// the very run that printed the advice. The knob is the way, and it is already
// in the person's hand.
func (r *recorder) warnIfClipped() {
	if r.samples == 0 || float64(r.clipped) < clipFraction*float64(r.samples) {
		return
	}

	volume := -1
	if r.volume != nil {
		volume = r.volume()
	}

	percent := 100 * float64(r.clipped) / float64(r.samples)
	if volume < 0 {
		render.Alert(r.app, "  warning: %.1f%% of that recording is clipped, so the sound input is being\n"+
			"           overloaded. Turn the scanner's volume down until this stops, or move\n"+
			"           the cable to a line input rather than a microphone input.\n", percent)
		return
	}
	render.Alert(r.app, "  warning: %.1f%% of that recording is clipped, so the sound input is being\n"+
		"           overloaded. Turn the scanner's volume down from %d of %d until this\n"+
		"           stops, or move the cable to a line input rather than a microphone\n"+
		"           input.\n",
		percent, volume, device.MaxLevel)
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
//   - p: the speakers to play each transmission on, nil when nobody asked to
//     hear them
//
// Returns:
//   - error if a recording cannot be written or the scanner stops answering;
//     nil once ctx is cancelled or the audio ends
func recordLoop(ctx context.Context, app *appcontext.App, library *recordings.Library,
	frames <-chan audiofeed.Frame, sample func(context.Context) (device.Heard, error),
	opts recordOptions, p player) error {

	r := &recorder{
		app:     app,
		library: library,
		player:  p,
		volume:  func() int { return volumeNow(ctx, app) },
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

		// speaking is the last radio the scanner named, kept so that a reading
		// which does not name one does not read as a different radio. The
		// identifier arrives a moment into a transmission and can drop out of
		// a reading in the middle of one, and treating either as a change
		// would cut a recording in half for no reason.
		speaking string

		// speaker is what goes in the identity a recording is cut on. It
		// changes only when one known radio replaces a different known one,
		// so learning who is talking is not mistaken for somebody new
		// starting to.
		speaker string
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
			// Carried forward while the transmission continues, and dropped
			// the moment it ends so the next one cannot inherit it.
			switch {
			case !h.Receiving:
				speaking, speaker = "", ""
			case h.Unit == "":
				// Nothing said, so nothing learned. Whoever was talking still
				// is, as far as anything here knows.
			case speaking == "":
				// Learning the identifier for the first time is information
				// about this transmission, not the start of another one.
				speaking = h.Unit
			case h.Unit != speaking:
				// Worth a line, because this is what cuts a recording in two
				// and a wrong one cuts a transmission in two. Anything odd
				// here shows up as a recording ending for "channel" with the
				// same radio on both sides of it.
				app.Log.Debug("the transmitting radio changed",
					"from", speaking, "to", h.Unit)
				speaking, speaker = h.Unit, h.Unit
			}
			h.Unit = speaking

			last = h
			r.gate.Activity(time.Now(), audiogate.Activity{On: h.Receiving, Key: key(h, speaker)})
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
			r.monitor(frame)
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

	if opts.speaker != "" && !opts.listen {
		return errors.New("--speaker says where to play the transmissions, which only means " +
			"something with --listen:\nadd --listen, or drop --speaker")
	}
	if opts.gain != 0 && !opts.listen {
		return errors.New("--gain turns up what is played, which only means something with " +
			"--listen:\nit does not change what is recorded, since --normalize already " +
			"scales that")
	}
	if opts.bufferSet && !opts.listen {
		return errors.New("--buffer sets how far behind the radio the speakers play, which " +
			"only means something with --listen:\nit does not change what is recorded")
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

	// The speakers are opened before the recordings folder is made, so that a
	// typo in --speaker costs a moment rather than leaving a folder behind for
	// a run that was never going to happen.
	var p player
	if opts.listen {
		if p, err = openPlayer(opts.speaker, opts.buffer); err != nil {
			return err
		}
		defer p.Close()
		p.SetGain(opts.gain)
		defer reportPlayback(app, p)
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
	library, err := recordings.New(destination, opts.template, opts.normalize)
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

	announceRecording(app, source, library.Dir(), p)
	return recordLoop(ctx, app, library, frames, sample, opts, p)
}
