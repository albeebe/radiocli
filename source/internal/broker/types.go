// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

// Package broker lets more than one process drive one scanner.
//
// The scanner speaks one request and response at a time over a single serial
// line, and portlock turns that into a claim covering a whole invocation, so
// that a menu walk cannot be cut in half by somebody else's command. That claim
// is exclusive: while it is held, every other invocation is refused.
//
// A broker is the same rule with a queue in front of it. One process holds the
// port and listens on a unix socket; anything refused the lock submits its
// command there instead and gets back exactly what it would have printed. The
// commands still run one at a time, so nothing about indivisibility changes.
// What changes is that being second means waiting rather than failing.
//
// # The two grains
//
// A single command is indivisible by default, which is enough for the commands
// that walk from the top menu every time. A sequence of commands is not, so a
// caller that needs several to run with nothing in between takes a lease, and
// everybody else queues until it is released. That is what a macro uses.
//
// # Why the wire types live here rather than in a shared package
//
// A front end is a separate program that talks this protocol and shares no code
// with this one. These types are therefore one of two independent
// implementations, and the specification in documentation/daemon_protocol.md is
// what they are both written against. Changing anything here is changing the
// protocol, not an internal detail.
package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/audioin"
	"github.com/albeebe/radiocli/internal/portlock"
)

// The formats audio can be sent in, chosen by each listener for itself.
const (
	// FormatPCM is the samples themselves, 20 ms of signed 16-bit
	// little-endian mono at 48 kHz, which is 1920 bytes a frame.
	//
	// It is the tier that always works. The encoder is a young pure Go port and
	// a listener's decoder may be older than its Opus support, so there has to be
	// somewhere to fall back to, and raw samples need nothing at either end. It
	// costs 768 kbps, which is nothing on the same machine or a local network
	// and too much over anything else.
	FormatPCM = "pcm"

	// FormatOpus is compressed, and is what anything over a network should ask
	// for. Its bitrate can be changed between one frame and the next.
	FormatOpus = "opus"
)

// The framing of an audio connection once it has started streaming.
//
// Up to and including the reply to OpAudio, an audio connection is ordinary
// newline-delimited JSON like every other. After the newline that ends that
// reply, everything the daemon sends is frames, forever.
//
// The direction matters: only daemon-to-client changes. A client keeps sending
// newline JSON, because what it has to say is rare and small, and leaving that
// half alone means the daemon's read loop is untouched.
//
// A frame is four bytes of header and then the payload:
//
//	byte  0     kind, one of the Frame constants below
//	bytes 1..3  payload length, big endian, not counting the header
//	bytes 4..   payload
//
// Which is why a connection that asks for audio may ask for nothing else. Two
// framings on one socket is already the awkward part; two framings and a reply
// that has to find its way back through them is worse, and there is nothing to
// gain from it, since a client wanting both can open a second connection and
// every real one already does.
const (
	// FrameAudio carries audio: four bytes of frame number, big endian, then
	// the encoded frame.
	//
	// The number counts 20 ms frames since the capture started, so it is a clock
	// rather than a count of what was sent. Everything downstream works out
	// timestamps from it, which is what lets audio be dropped anywhere along the
	// way without the far end playing steadily early: a gap in the numbers is
	// what a gap in the audio looks like, whether it was dropped here, dropped
	// by a front end, or one day never sent because a gate decided there was
	// nothing worth sending.
	FrameAudio byte = 1

	// FrameJSON carries one JSON object, with no newline after it.
	//
	// It is how anything that is not audio is said once the stream has started:
	// a level meter, a failure, and in time a transmission starting and ending.
	// It exists from the first version so that adding those needs no change to
	// the framing, and it is used from the first version so that the path is one
	// that has been exercised rather than one that merely compiles.
	FrameJSON byte = 2

	// FrameHeader is how many bytes precede a payload.
	FrameHeader = 4

	// MaxFrame is the largest payload either end will accept.
	//
	// Far above anything real, which is 1279 bytes for Opus and 1924 for raw
	// samples. It is a guard against a length read out of a corrupt or hostile
	// stream turning into an allocation, not a size anything is meant to reach.
	MaxFrame = 1 << 20
)

// How a run behaves when the scanner is busy.
const (
	// ModeQueue waits its turn. This is what a command somebody asked for
	// uses, and the default when no mode is given.
	ModeQueue = "queue"

	// ModeTry gives up rather than wait, and is answered with TypeSkipped.
	//
	// This is for a caller mirroring the scanner rather than commanding it.
	// A reading is only worth taking now: by the time a queued one ran, the
	// screen it describes would be several frames old, and it would have
	// taken the turn in front of whatever the person watching clicked next.
	ModeTry = "try"

	// ModePeek does not wait and does not take a turn at all. It runs
	// alongside whatever is running, and only commands that cannot move the
	// scanner may be asked for this way; anything else is refused.
	//
	// It exists because giving way is the wrong answer for a mirror. Under
	// ModeTry the display stops being redrawn for as long as a command holds
	// the scanner, which is the whole of a macro and several seconds of a
	// single menu walk, and the person watching sees a frozen screen at
	// exactly the moment something is happening on it.
	//
	// What makes it safe is that a turn covers a whole command while the
	// connection covers one exchange. A menu walk needs the first, because a
	// key pressed into somebody else's screen goes somewhere nobody meant. A
	// read needs neither: it changes nothing the walk depends on, so it can be
	// let between two of the walk's exchanges and the walk cannot tell.
	// Measured on an SDS150, a walk with reads slipped in as fast as the wire
	// allows produced the same values and took eight percent longer.
	ModePeek = "peek"
)

// The operations a client can ask for.
const (
	// OpRun executes one command and streams back what it produced.
	OpRun = "run"

	// OpCancel abandons a run already in flight. The command is stopped
	// where it stands, which is why the scanner may be left mid-walk: it is
	// the caller's Ctrl-C, and the alternative is a radio nobody is watching
	// being driven to the end of something nobody wants.
	OpCancel = "cancel"

	// OpCommands describes every command the daemon can run, so a caller that
	// presents the tool rather than running it does not need a list of its
	// own.
	OpCommands = "commands"

	// OpLease claims the scanner across several runs. Everything else queues
	// until it is released.
	OpLease = "lease"

	// OpRelease gives a lease back. Disconnecting does the same thing, so a
	// client that dies cannot hold the scanner.
	OpRelease = "release"

	// OpAudio subscribes to the sound input the daemon is holding, and turns
	// the connection it was sent on into a one-way stream of audio.
	//
	// It has nothing to do with the scheduler, the lease or the port lock, and
	// that is worth saying outright because every other operation here is about
	// taking turns. The audio does not come from the radio. It arrives on a
	// separate cable into a separate device, so a listener neither waits for a
	// command nor holds one up, and the daemon holds the sound card for the same
	// reason it holds the serial port: so that several things can have it at
	// once without each opening it for itself.
	//
	// A connection that has asked for audio is a connection that does nothing
	// else. See the Frame constants for why.
	OpAudio = "audio"
)

// The messages a daemon sends back.
const (
	// TypeHello is sent once when a client connects.
	TypeHello = "hello"

	// TypeStarted says a run has reached the front of the queue and is now
	// executing, which is the point at which it is holding the scanner.
	TypeStarted = "started"

	// TypeStdout and TypeStderr carry what the command wrote, as it wrote it,
	// kept apart so that a caller piping JSON somewhere still can.
	TypeStdout = "stdout"
	TypeStderr = "stderr"

	// TypeDone ends a run, whether it worked or not.
	TypeDone = "done"

	// TypeSkipped says a ModeTry run never happened because the scanner was
	// busy. It is not a failure and carries no error.
	TypeSkipped = "skipped"

	// TypeCommands answers OpCommands.
	TypeCommands = "commands"

	// TypeLeased and TypeReleased answer OpLease and OpRelease. TypeLeased
	// arrives when the lease is actually held, which for a client that queued
	// behind another one is later than when it asked.
	TypeLeased   = "leased"
	TypeReleased = "released"

	// TypeError answers a message the daemon could not make sense of at all,
	// such as an unknown op. A failure of something it did understand is
	// reported by that thing, so a failed run is a TypeDone with an error
	// rather than this.
	TypeError = "error"

	// TypeAudio answers OpAudio, and is the last line an audio connection
	// carries. The newline that ends it is the boundary: everything after it is
	// frames.
	TypeAudio = "audio"
)

// Version is the protocol this build speaks. A client is sent it in the hello
// and can refuse to go on rather than guess at a daemon it does not understand.
//
// It is deliberately separate from the tool's own version. Two builds with
// different version strings speak the same protocol far more often than not,
// and a client that checked the build would refuse perfectly good daemons.
const Version = 3

// audioQueue is how many frames may be waiting for one listener before the
// oldest are dropped.
//
// Half a second. Small, because this is live audio and a backlog is delay
// nobody asked for: a listener behind by more than a moment is better served by
// skipping to the present than by working through the past. It is also the
// point at which a listener stops being merely slow and starts being gone.
const audioQueue = 500 / audiofeed.FrameMS

// defaultBitrate is what a listener gets if it asks for Opus without saying.
//
// See the opusenc package for why the useful band is 32 to 48 rather than the
// 16 to 24 a hybrid encoder would manage on speech.
const defaultBitrate = 32000

// dialTimeout is how long connecting to a daemon may take.
//
// Short, because the socket is on this machine and the daemon either answers at
// once or is not there. What it actually guards against is a socket file left
// behind by a daemon that died, which would otherwise hang a command that was
// only trying to find out whether waiting was an option.
const dialTimeout = 2 * time.Second

// maxData is the most command output put into a single message.
//
// It is far below the maxRequest ceiling on a whole message because the output
// is JSON encoded into the message's data field, and encoding can grow it: a
// quote or a backslash becomes two bytes, and a control character becomes six.
// Six times this still leaves room for the envelope, so no amount of escaping
// can push a message past what the far end will read.
//
// The alternative, sending what a command wrote in one message, is what this
// used to do. It works until a command writes more than the far end will read
// in one go, and then the whole command fails at the very end with a message
// about a token being too long, which says nothing about what happened. Any
// command can write more than that: "colors --all" produces about ninety
// kilobytes of JSON, and a large channel listing more.
const maxData = 8 * 1024

// maxLeaseTTL is the longest lease anybody may ask for.
//
// A lease stops every other caller, so the ceiling is what stops one client's
// mistake from being everybody's problem. It is generous because a macro that
// renames a favorites list types it one character at a time, reading the screen
// back after each, which is genuinely slow.
const maxLeaseTTL = 5 * time.Minute

// maxRequest is the longest single message accepted from a client. Requests are
// a command line and a few settings; anything approaching this is a client that
// has gone wrong.
const maxRequest = 64 * 1024

// maxWaiting is how many commands one client may have queued before the rest
// are refused.
//
// Some queueing is wanted, because pressing two buttons quickly should run both
// rather than losing the second. An unbounded queue is not: a client that sends
// faster than the scanner answers would build a backlog of work nobody is
// waiting for any more.
//
// It is counted per client rather than across the daemon, which is the change
// from when a front end was the only caller. A command must be able to wait
// out a long macro without being told the scanner is busy, so the total queue
// cannot be small; what still has to be bounded is any one client's share of
// it, so that a runaway client cannot fill the queue a terminal is waiting in.
const maxWaiting = 4

// ErrNoDaemon reports that nothing is listening for this port. Callers test for
// it to tell "sharing is not available" from "sharing went wrong", which need
// different advice.
var ErrNoDaemon = errors.New("no radiocli daemon is running for this scanner")

// audioLinger is how long the sound card stays open after the last listener has
// left.
//
// Not closed at once, because the common thing is a listener reconnecting, and
// opening a sound card is slow enough to hear. Not held forever either: an open
// input is a microphone indicator lit on somebody's machine for a program nobody
// is listening to, which is exactly the thing that makes people suspicious of
// software.
var audioLinger = 30 * time.Second

// audiofeedStart starts the capture that feeds every listener. It is a var so
// tests can substitute a fake, which is what keeps a test run from opening a
// real sound card.
var audiofeedStart = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
	return audiofeed.Start(opts, out)
}

// chmodPath restricts the daemon's socket, and the directory holding it, to the
// account that started it. It is a var so tests can substitute a fake.
var chmodPath = os.Chmod

// errNoAudio is what a daemon with no sound card answers a listener with.
var errNoAudio = errors.New("this daemon is not holding a sound input, so it has no audio to give: " +
	"start it with --audio naming one")

// errUnreadable marks a message that arrived but could not be read, so that it
// is not reported as the daemon having died.
//
// The two failures reach the same place and read nothing alike. A daemon that
// stopped answering is gone and the advice is to start it again; a message this
// build cannot parse means both ends are running and disagree about the
// protocol, which is a version mismatch and is fixed by rebuilding.
var errUnreadable = errors.New("the daemon sent a message this build could not read")

// marshalJSON encodes a message. It is a var so tests can drive the failure the
// real encoder will not produce for the types this package sends.
var marshalJSON = json.Marshal

// resolveAudioSource turns the name of a sound input into the one that is
// attached. It is a var so tests can substitute a fake.
var resolveAudioSource = audioin.Resolve

// socketDir is the directory the daemon sockets live in. It is a var so tests
// can substitute a fake, which is what keeps a test from binding a socket
// outside its own temporary directory.
var socketDir = portlock.SocketDir

// socketPath is where the daemon socket for a port lives. It is a var so tests
// can substitute a fake, which is what keeps a test from binding a socket
// outside its own temporary directory.
var socketPath = portlock.SocketPath

// audioCapture is the part of a running capture this package uses. It is
// satisfied by *audiofeed.Capture and, in tests, by something with no device
// behind it.
type audioCapture interface {
	Source() string  // Name of the sound input this capture is reading
	Channel() string // Which side of the cable the scanner was found on
	Close()          // Gives the device back
}

// AudioInfo describes a stream, from the daemon's answer to OpAudio.
type AudioInfo struct {
	Format   string // FormatPCM or FormatOpus
	Rate     int    // Samples a second
	Channels int    // How many channels a frame carries, which is always 1
	FrameMS  int    // How much audio one frame holds, in milliseconds
	Bitrate  int    // Bits a second a compressed stream started at, and 0 for raw samples
	Source   string // Sound input the audio is coming from
	Channel  string // Which side of the cable it was found on, when the stream started
}

// AudioStream is a connection carrying audio and nothing else.
//
// It is a separate type from Client rather than a method on it, and that is
// forced by the framing rather than chosen for tidiness. A Client reads with a
// bufio.Scanner, which buffers ahead into memory it does not expose; asking it
// to read the greeting and the reply and then hand the socket over would leave
// the first audio frames stranded inside it, gone without any sign that they
// ever arrived. The reader that reads the lines has to be the reader that reads
// the frames, so an audio connection is built on a *bufio.Reader from its first
// byte.
//
// It is also why a connection carrying audio carries nothing else. Both ends
// stop speaking newline JSON in this direction, so there is nowhere for the
// answer to anything else to go.
type AudioStream struct {
	net  net.Conn      // Socket to the daemon
	in   *bufio.Reader // Reads the opening lines and then the frames, which has to be the one reader
	info AudioInfo     // What the daemon said this stream is, when it started
}

// Client is a connection to a daemon holding a scanner.
type Client struct {
	net  net.Conn       // Socket to the daemon
	in   *bufio.Scanner // Reads the daemon's messages, one line each
	next int            // Counts up to give each run an ID unique among this client's own

	// Version and Protocol describe the daemon, from its hello.
	Version  string
	Protocol int
}

// Options are the choices a daemon is started with.
type Options struct {
	// Orphaned, when set, is closed once whoever started the daemon has gone.
	//
	// The daemon does not stop at that moment. It stops once nothing is
	// connected to it, which is a different thing: a page that joined a daemon
	// somebody else started is still using the radio, and closing the scanner
	// because the process that happened to spawn the daemon has exited would
	// take the radio away from whoever is actually holding it. Started by one
	// and used by another is the ordinary case, not the exception.
	Orphaned <-chan struct{}

	// Audio names the sound input this daemon should hold for its listeners.
	// Empty means it holds none and has no audio to give.
	//
	// The name is checked at startup and the device is not opened until somebody
	// listens. See audioSide for why those are separate.
	Audio string

	// AudioChannel says which side of the cable the scanner is on, or "auto" to
	// work it out. Empty means auto.
	AudioChannel string
}

// Outcome is how a run ended.
type Outcome struct {
	// Code is the exit status the same command would have produced in a
	// terminal.
	Code int

	// Skipped says a ModeTry run never happened because the scanner was busy.
	// It is not a failure, and it is deliberately not an error: for the caller
	// that asks for it, giving way is the normal case rather than the
	// exceptional one.
	Skipped bool
}

// Request is one message from a client.
type Request struct {
	// Op is which operation this is, from the Op constants above.
	Op string `json:"op"`

	// ID ties a run to the messages it produces, and is what OpCancel names.
	// The client chooses it and only has to keep it unique among its own
	// runs in flight, because the daemon never mixes two clients' messages.
	ID string `json:"id,omitempty"`

	// Command is a command line as it would be typed, without the
	// "radiocli". The daemon splits it.
	//
	// Splitting on this end rather than the client's is what keeps a macro
	// step that was accepted when it was saved from failing when it is run:
	// there is one splitter, and it is the one that checked it.
	Command string `json:"command,omitempty"`

	// Argv is an already split argument list, for a caller that knows its
	// arguments and should not have to quote them into a line for this end to
	// take apart again. Exactly one of Command and Argv is used, and Argv
	// wins if both are given.
	Argv []string `json:"argv,omitempty"`

	// Mode is ModeQueue or ModeTry. Empty means ModeQueue.
	Mode string `json:"mode,omitempty"`

	// TTL bounds a lease, as a Go duration string such as "30s". A lease
	// without one is refused: a lease is a claim on a shared radio, and one
	// that can be held forever by a client that stopped paying attention is
	// the deadlock this whole design exists to avoid.
	TTL string `json:"ttl,omitempty"`

	// Format is FormatPCM or FormatOpus, on OpAudio. Empty means FormatPCM,
	// which is the tier that needs nothing of either end.
	Format string `json:"format,omitempty"`

	// Bitrate is bits per second, on OpAudio with FormatOpus, and on the
	// bitrate action. Empty means the daemon's default.
	Bitrate int `json:"bitrate,omitempty"`

	// Action is what an OpAudio message on an already streaming connection is
	// asking for, which today is only "bitrate".
	//
	// There is deliberately no action to stop. Closing the connection is how a
	// stream ends, the same way it is how a lease ends, and since an audio
	// connection carries nothing else there is nothing to keep it open for.
	Action string `json:"action,omitempty"`
}

// There is deliberately no way to send settings alongside a run.
//
// It was tried and it cannot work. A run executes as a full invocation inside
// the daemon, so it resolves its own settings the way every invocation does:
// the config file first, then the flags on its command line. Anything put onto
// the config beforehand is overwritten by that load before the command ever
// sees it.
//
// So a caller that wants a different pace or output puts it on the command
// line, which is the one mechanism that already wins and already works. That is
// what a front end does with its pace and output controls, and what a proxied
// invocation gets for free by sending the arguments exactly as they were typed.

// Response is one message from the daemon.
type Response struct {
	// Type is which message this is, from the Type constants above.
	Type string `json:"type"`

	// ID is the run this belongs to, echoed from the request. Empty on
	// messages that belong to the connection rather than to a run.
	ID string `json:"id,omitempty"`

	// Data is what the command wrote, on TypeStdout and TypeStderr.
	Data string `json:"data,omitempty"`

	// Argv is how the daemon split Command, sent with TypeStarted so a caller
	// can show what it actually ran.
	Argv []string `json:"argv,omitempty"`

	// Error is why a run failed, written the way the terminal would have
	// written it, without the "error: " prefix.
	Error string `json:"error,omitempty"`

	// Code is the exit status the same command would have produced in a
	// terminal, so a proxied invocation can exit with it.
	Code int `json:"code,omitempty"`

	// Version and Protocol identify the daemon, on TypeHello.
	Version  string `json:"version,omitempty"`
	Protocol int    `json:"protocol,omitempty"`

	// Audio names the sound input this daemon holds, on TypeHello, and is
	// empty when it holds none.
	//
	// It is in the greeting rather than only in the answer to OpAudio because
	// of what starts daemons. A front end asked to listen to one input may find
	// a daemon already running on another, and has to say so to the person at
	// the terminal at that moment, which is before anything exists to ask for
	// audio. The name here is the one the daemon resolved at startup; whether
	// the device can actually be opened is not known until somebody listens, and
	// that is what the answer to OpAudio reports.
	Audio string `json:"audio,omitempty"`

	// What a stream is, on TypeAudio. Rate, Channels and FrameMS are fixed by
	// the encoder and sent anyway, so that a client has no constants of its own
	// to keep in step with this end.
	Format   string `json:"format,omitempty"`
	Rate     int    `json:"rate,omitempty"`
	Channels int    `json:"channels,omitempty"`
	FrameMS  int    `json:"frameMs,omitempty"`

	// Bitrate is the rate a compressed stream started at, on TypeAudio. It is
	// only a starting point: a listener may change it whenever it likes, and
	// nothing announces that it did.
	Bitrate int `json:"bitrate,omitempty"`

	// Source is the sound input the audio is coming from, and Channel is which
	// side of the cable it was found on, on TypeAudio.
	//
	// Channel is the answer at the moment the stream started. Left to itself the
	// daemon spends the first few seconds deciding, so a stream that began in
	// that window reads "mix" here and is told the answer later in a frame of
	// its own. It is not a promise about every frame that follows.
	Source  string `json:"source,omitempty"`
	Channel string `json:"channel,omitempty"`

	// Commands answers OpCommands.
	Commands []appcontext.Command `json:"commands,omitempty"`
}

// Server is a daemon: it holds one scanner and runs commands on it for anybody
// that connects.
type Server struct {
	app     *appcontext.App // The App every command runs against, holding the scanner
	sched   scheduler       // Hands out the scanner one holder at a time, in the order asked
	run     runner          // Runs the commands that take a turn
	clients clients         // Counts the connections, for deciding when nobody is left

	// peek runs the commands that only read, alongside whatever has the
	// scanner. It is a second runner because a runner points the App it holds
	// at one caller's streams and settings for the length of a command, and
	// two callers doing that to the same App at the same time would write into
	// each other's output. The App behind this one shares only the connection.
	//
	// It is nil when nothing built one, and then a peek is refused rather than
	// quietly queued: a caller that asked not to wait should be told it is not
	// available rather than left waiting.
	peek *runner

	// peeking serializes peeks against each other. They may run alongside a
	// command, and they still share one App between themselves.
	peeking sync.Mutex

	// audio is the sound card this daemon holds for its listeners, or nil when
	// it was not given one.
	audio *audioSide

	// log is the daemon's own logger, taken once at startup.
	//
	// Deliberately not read from the App when it is wanted. A runner swaps the
	// App's streams and its logger for one client's for as long as a command
	// runs, so anything fetching the logger later could find itself writing this
	// daemon's diagnostics into somebody's command output, and down a socket
	// where a line of text is a protocol violation. Audio runs alongside
	// commands, so it would meet that swap constantly.
	log *slog.Logger
}

// audioSide is the daemon's one sound card, opened when somebody first listens
// and closed a while after the last of them goes.
//
// Opened late rather than at startup, which is a decision about somebody's
// machine rather than about this program. Listing sound inputs does not ask
// macOS for permission to record and opening one does, so a daemon that opened
// its card on the way up would raise a permission prompt at the moment a web
// server started, attached to no action anybody took, and would then hold the
// microphone open with nothing listening. The name is resolved at startup, so a
// typo still fails immediately; only the device waits.
type audioSide struct {
	source  string       // Sound input this daemon holds, resolved at startup
	channel string       // Which side of the cable the scanner is on, or "auto"
	log     *slog.Logger // The daemon's own logger, never the App's

	// open makes the capture. It is a field rather than a call so that tests
	// can drive everything above it without a sound card, which is all of the
	// framing, the fan-out and the connection handling.
	open func(audiofeed.Options, audiofeed.Publisher) (audioCapture, error)

	mu      sync.Mutex      // Guards everything below, which several listeners reach at once
	feed    *audiofeed.Feed // Fans one capture out to every listener, nil when the card is shut
	capture audioCapture    // The running capture, nil when the card is shut
	holders int             // How many listeners are on it
	closing *time.Timer     // Counts down to giving the card back, nil when nothing is pending
}

// clients counts the connections a daemon is holding, and lets a waiter be
// woken when that count moves.
type clients struct {
	mu      sync.Mutex    // Guards the count and the channel together
	count   int           // How many connections the daemon is holding
	changed chan struct{} // Closed the next time the count moves, nil when nobody is waiting
}

// conn is one client.
type conn struct {
	net net.Conn // Socket this client arrived on
	srv *Server  // The daemon this connection belongs to

	// write serialises the several goroutines that write to this socket, so
	// two of them cannot interleave halfway through a message.
	write sync.Mutex

	// running makes this client's own commands one at a time. The scheduler
	// makes them one at a time across the daemon, but a leaseholder does not
	// go through the scheduler for each command, so without this a client
	// could run two at once inside its own lease.
	running sync.Mutex

	// streaming says this connection has stopped being newline JSON in the
	// daemon-to-client direction and is carrying audio frames instead. It is
	// never set back: a connection that has started streaming does that until
	// it closes.
	streaming atomic.Bool

	// bitrate carries a new rate to the streaming goroutine, from the read
	// goroutine that was told about it.
	bitrate chan int

	// audioDone stops the streaming goroutine, and streams waits for it.
	//
	// Both are needed because of the order the deferred work in serveConn runs
	// in. A stream ends when its write fails, which happens when the socket is
	// closed, which is the last thing to happen; but waiting for the handlers
	// is the first. Without something to stop the stream outright, a connection
	// with audio on it would never finish tearing down.
	audioDone chan struct{}
	streams   sync.WaitGroup

	// leases is the tail of the chain that keeps this connection's lease and
	// release operations in the order they arrived, now that they are handled
	// off the read loop rather than on it. Each one waits for the channel here
	// and leaves its own behind for the next.
	//
	// It carries no lock because it is only ever touched by the read loop,
	// which is one goroutine. Everything the chained operations touch after
	// that is guarded by mu like the rest.
	leases chan struct{}

	mu sync.Mutex // Guards the three fields below
	// held is this client's lease, nil when it has none.
	held *lease
	// waiting counts the commands queued or running for this client.
	waiting int
	// cancels lets OpCancel reach a run already in flight.
	cancels map[string]context.CancelFunc
}

// eofReader is a stdin with nobody behind it. Reads end at once instead of
// blocking forever on an answer that is never coming.
type eofReader struct{}

// lease is a claim held across several commands.
//
// It exists because a single command being indivisible is not enough for
// everything. The commands that read the scanner's memory walk from the top
// menu every time and leave it where they found it, so interleaving two of them
// is safe. Pressing a key, opening a menu, tuning and resuming a scan are not:
// they deliberately leave the radio somewhere, and a caller doing several in a
// row means the ones in between to happen to the place the last one left.
//
// A macro is exactly that, which is why one runs inside a lease.
type lease struct {
	// timer force-releases the lease if the client stops paying attention.
	// A lease is a claim on a radio somebody else is waiting for, and one
	// that can be held forever by a client that wandered off is the wedge
	// this design exists to prevent. Disconnecting releases it too, so this
	// is for a client that is still connected and simply idle.
	timer *time.Timer

	// released guards against the timer and an explicit release both firing,
	// which would hand the scanner on twice.
	released bool

	// runs counts the leaseholder's commands still executing. The leaseholder
	// skips the scheduler for each command, so the lease is the only record
	// that one of its commands is on the serial line, and whoever takes the
	// lease back waits for this to reach zero before handing the scanner on.
	// Without it, an expiry firing mid-command would start the next caller's
	// command on top of this one.
	runs int

	// idle wakes drain when the last running command leaves.
	idle *sync.Cond

	mu sync.Mutex // Guards released and runs, and is held while the timer is armed
}

// runner executes one command at a time against the daemon's App.
//
// It does no scheduling. Whose turn it is belongs to the scheduler, because a
// lease spans several of these and the two questions have different lifetimes.
// This is only the part that points the App at one caller's streams, runs the
// command, and puts the App back.
type runner struct {
	app *appcontext.App // The App commands run against, pointed at one caller at a time
}

// scheduler hands out the scanner one holder at a time, in the order asked.
//
// It replaces the mutex used back when one program was the only thing driving
// the radio. A mutex was enough then because the only two callers were a
// command and the display mirror, and the mirror gave way by not taking it. It
// is not enough now, for two reasons.
//
// A lease is held across several commands, so the thing being waited on
// outlives any one call and cannot be a mutex somebody is blocked in. And the
// order matters: a caller that has waited out a thirty-second macro must not
// then lose its turn to a command that arrived a moment ago. Go's mutex is
// roughly fair under contention and makes no promise about it, so the order is
// kept here instead, where it can be relied on.
type scheduler struct {
	mu sync.Mutex // Guards the two fields below

	// held is whether anybody has the scanner right now, by a run or a lease.
	held bool

	// waiting is the queue, oldest first. Each waiter is woken by closing its
	// channel, and a woken waiter already owns the scanner: ownership passes
	// directly from the releaser rather than being competed for again, which
	// is what makes the order actually hold.
	waiting []*waiter
}

// stream forwards what a command writes as it is produced, rather than
// collecting it and sending the lot at the end.
type stream struct {
	conn *conn  // Connection the output goes back on
	id   string // Run this output belongs to, echoed on every message
	kind string // TypeStdout or TypeStderr, which keeps the two apart
}

// waiter is one caller's place in the queue.
type waiter struct {
	ready chan struct{} // Closed when this caller's turn comes, which hands it the scanner

	// abandoned is set when the caller gave up before its turn came. The
	// releaser skips it rather than handing the scanner to nobody.
	abandoned bool
}
