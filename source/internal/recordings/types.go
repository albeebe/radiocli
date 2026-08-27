// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package recordings

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/albeebe/radiocli/internal/wavfile"
)

// DefaultTemplate is the layout used when none is given.
//
// The date is a folder rather than part of the name, which is not a style
// choice. Software that puts the date only in the filename does not start a new
// folder at midnight, so a long run piles a whole week into one directory and
// the fix is a setting the user has to know exists. Making the day a folder by
// default means the case nobody thinks about is the one that already works.
const DefaultTemplate = "{date}/{time}_{system}_{department}_{channel}"

// NormalizeTarget is where normalizing puts a recording's loudest sample.
//
// Just under full scale rather than at it. A hair of headroom costs nothing
// audible and means that rounding, and any player that applies its own gain,
// have somewhere to go other than into the ceiling.
const NormalizeTarget = 0.99

// maxCollisions is how many numbered names are tried before giving up.
//
// Far more than two transmissions can genuinely collide on, and a limit at all
// because a name reads as taken whenever it cannot be checked. A folder that
// cannot be read would otherwise turn into a loop that never ends and never
// explains itself.
const maxCollisions = 1000

// The limits on how long a path this package will produce.
//
// They exist because the alternative is discovering the operating system's own
// limit at the moment a transmission is being written, which is the worst
// possible time and produces an error most people read as the recorder being
// broken. Windows refuses a path over 260 characters by default, and a template
// carrying a system, a department and a channel name reaches that easily.
const (
	// maxComponent is how long one folder or file name may be.
	maxComponent = 80

	// maxPath is how long the whole path below the destination may be, leaving
	// room for the destination itself and for the longer of the two suffixes.
	maxPath = 180

	// minComponent is how far a name may be shortened before something else is
	// shortened instead, so that trimming a long path leaves every part of it
	// still recognisable.
	minComponent = 8
)

// partialPrefix marks a recording still being written.
//
// It begins with a dot so the operating system hides it, because a recording in
// progress has a header full of zeroes and will not play. The file is renamed
// into place once it is finished and knows what it is called, which cannot
// happen at the start: the name carries the channel and the duration, and
// neither is known until the transmission ends.
const partialPrefix = ".partial-"

// ErrBadTemplate says a naming template could not be used. It is returned by
// New, before anything is recorded, so a typo costs a second rather than a
// night's recordings.
var ErrBadTemplate = errors.New("invalid naming template")

// Test seams, so the failures a real disk produces only under conditions a test
// cannot arrange can be driven on demand. This is the pattern backup uses.
var (
	// createWav opens the audio file a recording is written to.
	createWav = func(path string) (wavWriter, error) { return wavfile.Create(path) }

	// marshalIndent turns an entry into the sidecar beside the audio.
	//
	// It is a seam only so that the failure it reports can be tested. Every
	// field of Entry is a string, a number or a time, so it cannot fail as the
	// type stands, and the error is still handled because a field added later
	// could change that. A branch that cannot be reached is a branch nothing is
	// checking.
	marshalIndent = json.MarshalIndent

	// mkdirAll creates the folders a template asked for.
	mkdirAll = os.MkdirAll

	// readDir lists the destination, for finding recordings left behind.
	readDir = os.ReadDir

	// removeFile deletes a recording that turned out not to be wanted.
	removeFile = os.Remove

	// renameFile moves a finished recording from its partial name to its real
	// one.
	renameFile = os.Rename

	// statFile reports whether a name is already taken.
	statFile = os.Stat

	// writeFile writes a sidecar.
	writeFile = os.WriteFile
)

// tokens is every name a template may use, and how each is rendered.
//
// Named rather than lettered, and one per idea. The software this was measured
// against grew a set by accretion into four overlapping ways to write the time
// and four more for what the radio was tuned to, which nobody can hold in their
// head and which produced filenames that sorted wrongly.
var tokens = map[string]func(Entry) string{
	// When it happened. All three sort correctly as text, which the obvious
	// alternative of a locale-formatted time does not.
	"date":     func(e Entry) string { return e.Start.Format("2006-01-02") },
	"datetime": func(e Entry) string { return e.Start.Format("2006-01-02T15-04-05") },
	"epoch":    func(e Entry) string { return strconv.FormatInt(e.Start.Unix(), 10) },
	"time":     func(e Entry) string { return e.Start.Format("15-04-05") },

	// Where it sits in the scanner's memory, outermost first.
	"channel":    func(e Entry) string { return e.Channel },
	"department": func(e Entry) string { return e.Department },
	"list":       func(e Entry) string { return e.List },
	"site":       func(e Entry) string { return e.Site },
	"system":     func(e Entry) string { return e.System },

	// What it was on. A conventional system answers with a frequency and a
	// trunked one with a talkgroup, so tuned exists for a template that wants
	// whichever applies without caring which.
	"frequency": func(e Entry) string { return e.Frequency },
	"talkgroup": func(e Entry) string { return e.Talkgroup },
	"tuned":     func(e Entry) string { return cmp(e.Talkgroup, e.Frequency) },

	// The rest.
	"duration":   func(e Entry) string { return strconv.FormatFloat(e.Duration, 'f', 1, 64) },
	"modulation": func(e Entry) string { return e.Modulation },
	"unit":       func(e Entry) string { return e.Unit },
}

// wavWriter is the part of a WAV file a recording uses. It is an interface so
// tests can stand in for one and drive a disk that fails.
type wavWriter interface {
	// Close completes the header and closes the file.
	Close() error

	// Duration reports how much audio has been written.
	Duration() time.Duration

	// Normalize scales the audio so its loudest sample sits at the given
	// fraction of full scale.
	Normalize(target float64) error

	// Write appends audio.
	Write(pcm []byte) error
}

// Entry is one recording, and is the only shape this package reports.
//
// The same object is the sidecar beside the audio and what the command prints.
// One schema for both is deliberate: anything reading these has one thing to
// learn, and a field cannot mean something different depending on where it was
// read.
//
// It is also the whole of the metadata. The software this was measured against
// smuggles its metadata into the filename and three text tags inside the audio
// file, which means anything wanting the frequency back has to parse a name it
// was not given the format of. A JSON object beside the audio needs no such
// agreement.
type Entry struct {
	// File is where the audio is, relative to the destination, so a sidecar
	// copied elsewhere with its recording still points at it.
	File string `json:"file"`

	// Start and End are when the audio began and ended, found in the buffer
	// rather than taken from when anything was noticed.
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`

	// Duration is how long the audio is, in seconds, measured from the file
	// rather than from the clock.
	Duration float64 `json:"duration"`

	// List, System, Department and Site are where the channel sits in the
	// scanner's memory, outermost first. Site is empty on a conventional
	// system.
	List       string `json:"list,omitempty"`
	System     string `json:"system,omitempty"`
	Department string `json:"department,omitempty"`
	Site       string `json:"site,omitempty"`

	// Channel is the channel's alpha tag.
	Channel string `json:"channel,omitempty"`

	// Frequency is what the scanner was tuned to on a conventional system,
	// carrying its own unit. Talkgroup is the number on a trunked one.
	//
	// A trunked recording carries both. The talkgroup says who was talking and
	// the frequency says where the radio actually was, which on a trunked
	// system is a voice channel the site handed out for that call and a
	// different one next time. It is what lines a recording up against a
	// spectrum capture, or against what another receiver heard.
	Frequency string `json:"frequency,omitempty"`
	Talkgroup string `json:"talkgroup,omitempty"`

	// Unit is the radio heard transmitting, when the scanner decoded one.
	//
	// It is empty on an analog channel, and empty on a digital one whose
	// transmission had already begun when the scanner stopped on it. An empty
	// value here means the scanner never reported one, and Samples says how
	// many chances it had to.
	Unit string `json:"unit,omitempty"`

	// Modulation is how the scanner was demodulating, such as "NFM".
	Modulation string `json:"modulation,omitempty"`

	// NAC is the network access code of the P25 system this was heard on, such
	// as "8A1h", and is absent on anything that is not P25.
	//
	// It is also absent on a transmission too short to have decoded one. A
	// measured 1.1 second transmission reported its format and no code, in the
	// middle of a run of longer ones on the same channel that all reported
	// both, so this is the scanner needing a moment rather than the channel
	// changing.
	//
	// It identifies the system itself rather than the call, which is what makes
	// it worth keeping beside a recording: two systems can use the same
	// talkgroup numbers, and this is what tells their recordings apart.
	NAC string `json:"nac,omitempty"`

	// RSSI is how strong the signal was, in the scanner's own units, taken from
	// the loudest reading during the transmission.
	//
	// The strongest rather than the first or the last, because a transmission
	// from a moving vehicle rises and falls across it, and what is worth
	// keeping is the best the receiver managed rather than wherever the reading
	// happened to be when the recording opened.
	RSSI string `json:"rssi,omitempty"`

	// Digital is the digital format the transmission was carrying, such as
	// "P25" or "DMR", and is absent when it was analog.
	//
	// It is the only field that answers whether a recording is of a digital
	// transmission. Modulation cannot: it reports the demodulator's state, so
	// a channel programmed Auto and carrying P25 says "NFM" like any analog
	// one. Without this, the only clue is whether a unit id happened to be
	// caught, which is absence used as evidence and wrong as often as not.
	//
	// It is per recording rather than per channel, and that is the point. One
	// frequency was measured carrying analog traffic and P25 traffic fifteen
	// minutes apart, so the answer genuinely differs from one transmission to
	// the next and nothing above a recording can be asked for it.
	Digital string `json:"digital,omitempty"`

	// Reason is why the recording ended: "hang" for the ordinary case, "split"
	// for one that reached the maximum length, "channel" for one cut because
	// the scanner moved, and "stopped" for one interrupted by shutting down.
	Reason string `json:"reason,omitempty"`

	// Samples is how many times the scanner was asked what it was hearing
	// while this was being recorded.
	//
	// It is the confidence in everything above. A transmission of any ordinary
	// length is covered many times over; a zero means the transmission was over
	// before the scanner could be asked, and the fields naming a channel are
	// empty rather than guessed.
	Samples int `json:"samples"`

	// Channels lists every distinct channel seen while this was recording, and
	// is present only when there was more than one.
	//
	// One recording naming two channels means the scanner moved during it. The
	// recorder normally cuts a new file at that point, so this is rare and
	// worth saying when it happens rather than silently picking a winner.
	Channels []string `json:"channels,omitempty"`

	// Dropped counts frames of audio the sound card produced that never
	// arrived, so a recording with a gap in it says so instead of concatenating
	// across it silently.
	Dropped int `json:"dropped,omitempty"`

	// Normalized says the audio was scaled up after the transmission ended, so
	// its loudest sample sits just under full scale.
	//
	// It is recorded because it changes what the file's level means. Left
	// alone, one recording being quieter than another says the signal was
	// weaker; normalized, every recording is equally loud and that comparison
	// is gone. Anything reading these later should know which it is looking at
	// rather than infer it.
	Normalized bool `json:"normalized,omitempty"`
}

// Library is a destination directory that recordings are written into.
//
// It is safe for concurrent use, though the recorder does not need that: one
// goroutine reads the feed and finishes one recording at a time. The lock is
// there because the counter behind the temporary names is shared by every
// recording begun.
type Library struct {
	// dir is the destination every path is relative to.
	dir string

	// name renders a recording's path from its metadata.
	name template

	// normalize is the level each recording's loudest sample is brought to, or
	// zero to leave the audio exactly as it arrived.
	normalize float64

	// mu guards partials.
	mu sync.Mutex

	// partials counts recordings begun, so two starting in the same second
	// cannot collide on a temporary name.
	partials int
}

// Recording is one transmission being written.
type Recording struct {
	// library is where this will be filed when it is closed.
	library *Library

	// wav is the open audio file.
	wav wavWriter

	// partial is where the audio is being written, which is not where it will
	// end up.
	partial string

	// done makes Close safe to call twice, which it has to be: a recording is
	// closed on the normal path and again by a deferred call when something
	// failed part way through.
	done bool
}

// part is one piece of a template.
type part struct {
	// literal is the text to emit, when this is not a token.
	literal string

	// token is the name to look up, empty when this is a literal.
	token string
}

// template is a naming template, already checked and broken into the pieces it
// renders from.
type template struct {
	// parts is the template in order, each either a literal or a token.
	parts []part

	// text is the template as it was written, for error messages.
	text string
}

// cmp returns the first of its arguments that is not empty.
//
// Parameters:
//   - values: the candidates, in order of preference
//
// Returns:
//   - the first non-empty value, or empty if there is none
func cmp(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
