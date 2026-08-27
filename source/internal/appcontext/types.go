// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package appcontext

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/albeebe/radiocli/internal/device"
)

// App is the dependency container passed to every command.
//
// Cheap, always-needed dependencies are plain fields. Anything expensive or
// remote should be an interface field so tests can substitute a fake, and
// should be built in Init rather than in New.
type App struct {
	// Config holds the resolved settings for this invocation, assembled from
	// defaults, the config file, and the global flags.
	Config *Config

	// Log is the structured logger. Its level is set by Init from Config.
	Log *slog.Logger

	// Stdout, Stderr and Stdin are the process streams. Commands must write
	// through these rather than calling fmt.Println so output can be captured
	// in tests.
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// RunCommand executes one invocation of the tool inside this process, as
	// if argv had been typed on the command line. It is set by main, which is
	// the only place that knows every command.
	//
	// It exists so a long-running command can invoke the others without
	// starting a second process. Shelling out cannot work here: the scanner
	// claim covers a whole invocation, so a child would block on the lock its
	// own parent is holding.
	//
	// Two rules for callers. Build the streams before calling, because the
	// command tree binds to Stdout, Stderr and Stdin as it is built. And run
	// one at a time: the scanner speaks one request and response at a time,
	// and this App is shared.
	RunCommand func(ctx context.Context, argv []string) error

	// Commands lists the tool's top-level commands. It is set by main for the
	// same reason as RunCommand: that is the only place that knows them all.
	//
	// It exists so a command that presents the tool to somebody, such as the
	// daemon answering a client that asks what this build can run, can offer
	// every command without keeping its own copy of the list. A second copy is
	// a second place to forget a command.
	Commands func() []Command

	// Reader returns a second App that shares this one's scanner and can run
	// only the commands that read. It is set by main, for the same reason as
	// RunCommand: that is the only place that knows the command tree, and the
	// tree is what says which commands only read.
	//
	// It exists so that something watching the scanner can keep watching while
	// a long command is running. The scanner answers one exchange at a time and
	// the connection enforces that, but a command is many exchanges, and
	// holding the radio for all of them is what makes a menu walk safe. A read
	// slipped between two of them changes nothing the walk depends on, which
	// makes it the one thing that can be let past.
	//
	// It is nil unless main set it, which is every real invocation and not
	// necessarily a test.
	Reader func() *App

	// device is the scanner connection, opened on first use by Device. It is
	// unexported so nothing can use a connection that was never opened.
	device *device.Scanner

	// borrowed marks an App whose scanner belongs to another one. It must not
	// close that scanner, and must not re-apply the pace to it: the App that
	// owns it may be part way through a walk that is being paced deliberately.
	borrowed bool

	// InDaemon marks an App whose commands are running inside a daemon rather
	// than in an invocation of their own.
	//
	// It exists for one command. A daemon runs commands in its own process and
	// lends them its streams for the duration, which works because every
	// command finishes. One does not: "audio output" runs until it is stopped,
	// and inside a daemon it would hold the streams forever and send every
	// other client's output down its own socket. Since a page can type any
	// command line, that has to be refused rather than merely not documented.
	//
	// Anything else that runs without end has the same problem and should look
	// at this too.
	InDaemon bool

	// closers are the shutdown functions registered by OnClose, run in
	// reverse order by Close so dependencies tear down after their users.
	closers []func() error
}

// ErrNoDevice is returned when no scanner was named. Commands can test for it
// to tell "you did not say which scanner" apart from "the scanner is
// unreachable", which need different advice.
var ErrNoDevice = errors.New("no scanner named")

// openDevice opens the scanner connection on the named serial port. It is a
// var so tests can substitute a fake.
var openDevice = device.Open

// Command describes one of the tool's commands to something that presents them
// rather than runs them, such as a page offering a form per command.
//
// Everything here is read from the command tree, so a description cannot drift
// from the command it describes. Nothing here runs anything: a caller builds a
// command line and hands it to RunCommand, which is the path a typed command
// takes too.
type Command struct {
	// Name is the command as typed, such as "backlight".
	Name string `json:"name"`

	// Short is the one-line description shown beside it in the tool's own
	// help, so a list built from this reads the same as the help does.
	Short string `json:"short"`

	// Use is the usage line, such as "open <menu> [index]". It is the only
	// place positional arguments are written down, so it is carried through
	// for anything that would rather show it than trust Args.
	Use string `json:"use,omitempty"`

	// Args are the positional arguments, read from Use.
	Args []Arg `json:"args,omitempty"`

	// Flags are the command's own flags. Flags inherited from parents are
	// left out: they belong to every command and are offered separately.
	Flags []Flag `json:"flags,omitempty"`

	// Subcommands are the commands nested under this one.
	Subcommands []Command `json:"subcommands,omitempty"`

	// Runnable reports whether this command does anything by itself. A
	// command with subcommands may or may not: "favorites" lists lists, while
	// "menu open" only exists to hold its own subcommands.
	Runnable bool `json:"runnable"`

	// Alternatives reports that Use describes more than one way to call the
	// command, separated by a "|", as "set <zip> | --position <lat>,<lon>"
	// does. Args then describes only the first of them, so a caller should
	// show Use as well to make the others findable.
	Alternatives bool `json:"alternatives,omitempty"`
}

// Arg is one positional argument of a command.
type Arg struct {
	// Name is the placeholder as written in Use, without its brackets.
	Name string `json:"name"`

	// Required reports whether Use wrote it as <name> rather than [name].
	Required bool `json:"required"`

	// Repeated reports whether Use ended it with "...", meaning more than one
	// value may be given.
	Repeated bool `json:"repeated,omitempty"`

	// Values are the only accepted values, when the command says what they
	// are. Empty means anything goes, which is the common case: most values
	// are names that only the scanner knows.
	Values []string `json:"values,omitempty"`
}

// Flag is one flag of a command.
type Flag struct {
	// Name is the long spelling, without the leading dashes.
	Name string `json:"name"`

	// Shorthand is the single letter spelling, empty if it has none.
	Shorthand string `json:"shorthand,omitempty"`

	// Type is what the flag takes, as the flag library names it: "bool",
	// "string", "int", "duration", "uint16".
	Type string `json:"type"`

	// Default is the value used when the flag is not given, written the way
	// the help writes it.
	Default string `json:"default,omitempty"`

	// Usage is the one-line description from the help.
	Usage string `json:"usage,omitempty"`
}

// OnlyReads marks a command that cannot move the scanner: it looks at what the
// radio is doing and reports it, and presses nothing.
//
// It is a cobra annotation rather than a list kept somewhere central, so that
// the answer lives on the command itself and a command added later is not
// quietly assumed to be safe. Nothing may carry it unless every path through it
// is a read, including the ones its flags reach.
const OnlyReads = "onlyReads"

// OnlyReadsWith names the flags that turn a command which otherwise moves the
// scanner into one that only reads, separated by spaces and written without
// their dashes.
//
// It exists for "colors", where the answer genuinely depends on the flags: the
// bare command walks every color picker of a layout in turn, and --cache hands
// back what that walk found last time while --positions answers from a built-in
// table. Marking the command as a whole would let the walk run alongside
// somebody else's, and leaving it unmarked freezes a mirror's colors for the
// length of every command, which is worse than it sounds: the screen underneath
// them keeps moving, so what is drawn is a new screen in an old screen's colors.
//
// A command carrying this and none of its flags is refused, which is the safe
// way round.
const OnlyReadsWith = "onlyReadsWith"

// marshalJSON encodes the settings for writing. It is a var so tests can
// drive the failure the real encoder will not produce for these types.
var marshalJSON = json.MarshalIndent

// userConfigDir locates the directory this machine keeps user configuration
// in. It is a var so tests can substitute a fake.
var userConfigDir = os.UserConfigDir

// Config holds the settings for a single invocation of the tool.
//
// Fields are resolved in increasing order of precedence: defaultConfig sets the
// baseline, Load reads the config file over it, and the root command applies
// the global flags last so an explicit flag always wins.
type Config struct {
	// Path is the config file to read. Empty means the default location.
	// It is set by --config and is never read from the file itself.
	Path string `json:"-"`

	// Verbose enables debug-level logging.
	Verbose bool `json:"verbose"`

	// Output selects the rendering format for command results.
	Output OutputFormat `json:"output"`

	// Device is the serial port of the scanner to talk to, such as
	// /dev/tty.usbmodem14201. It is set by --device, and every command that
	// touches the scanner needs it.
	//
	// It is deliberately not in the config file. A remembered scanner is a
	// command aimed at whatever was chosen once, which on a machine that has
	// had more than one attached is a command aimed at the wrong radio with
	// nothing on screen to say so. Naming the port every time is longer to
	// type and impossible to get quietly wrong.
	Device string `json:"-"`

	// Pace is how quickly keys are sent to the scanner. It is a setting rather
	// than a per-command choice because the right value depends on the
	// scanner, not on what is being asked of it.
	Pace device.Pace `json:"pace"`

	// Wait is how long to wait for another invocation of the tool to finish
	// with the scanner before giving up. Zero gives up at once.
	//
	// It is deliberately not in the config file. How long a caller is willing
	// to queue is a property of that caller, so a script that can afford to
	// wait passes --wait, and an interactive run keeps failing fast rather
	// than inheriting somebody else's patience and looking hung.
	Wait time.Duration `json:"-"`

	// Macros are named command sequences, stored for a front end to offer as
	// single presses.
	//
	// They are deliberately not checked by Validate. Validate runs on every
	// invocation, so a macro somebody mistyped into this file by hand would
	// otherwise stop "battery" from running, and stop the config command from
	// being able to put it right. They are checked where they are written
	// instead, which is the only place a bad one can be created.
	//
	// Deliberately without omitempty, unlike every other optional field here.
	// A file that names no macros gets the built-in ones, so "macros": [] is
	// how somebody says they want none, and a list that vanished when it
	// emptied would bring all four back the next time it was read.
	Macros []Macro `json:"macros"`
}

// OutputFormat selects how commands render structured results.
type OutputFormat string

const (
	// OutputText renders human-readable output.
	OutputText OutputFormat = "text"
	// OutputJSON renders machine-readable output.
	OutputJSON OutputFormat = "json"
)

// Macro is one named shortcut: a name, the commands it runs in order, and what
// to do when one of them fails. Each step is a command line as it would be
// typed, without the "radiocli".
//
// Nothing here runs them. Whatever offers them reads them and sends the steps
// one at a time, exactly as if each had been typed, so a macro can do no more
// than the person running it could do by hand.
type Macro struct {
	// Name is what labels the macro wherever it is offered, and what names it
	// on the command line. Names are unique regardless of case.
	Name string `json:"name"`

	// Steps are the command lines to run, in order. There is always at least
	// one.
	Steps []string `json:"steps"`

	// KeepGoing runs the remaining steps after one fails, instead of stopping.
	//
	// Off by default, because a failed step leaves the scanner wherever it got
	// to, and the steps that walk menus press keys chosen for a screen the
	// radio is no longer on. It is worth turning on for a macro whose steps do
	// not depend on each other.
	KeepGoing bool `json:"keepGoing,omitempty"`
}
