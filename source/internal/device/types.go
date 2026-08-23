// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package device

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/albeebe/radiocli/internal/portlock"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// The ways a single character can be drawn.
const (
	// AttrNormal is an ordinary character.
	AttrNormal Attribute = ' '

	// AttrReverse is a character drawn in reverse video, which the scanner
	// uses to mark the selected item.
	AttrReverse Attribute = '*'

	// AttrUnderline is an underlined character, which the scanner uses for
	// headings.
	AttrUnderline Attribute = '_'
)

// The ways an entry can be avoided.
const (
	// AvoidPermanent avoids the entry until it is explicitly restored.
	AvoidPermanent AvoidMode = 1

	// AvoidTemporary avoids the entry until the scanner restarts.
	AvoidTemporary AvoidMode = 2

	// AvoidStop stops avoiding the entry.
	AvoidStop AvoidMode = 3
)

// backlightLevels is how many brightness steps the scanner has, above off.
const backlightLevels = 3

// How the port is opened, and how long the scanner is given to answer on it.
const (
	// baudRate is nominal: the scanner presents a USB CDC port and ignores the
	// line speed, but the port still has to be opened with one.
	baudRate = 115200

	// readPoll bounds a single read so a silent port cannot block forever.
	// The real deadline comes from the caller's context.
	readPoll = 100 * time.Millisecond

	// commandTimeout is how long a normal command may take to answer.
	commandTimeout = 3 * time.Second

	// xmlTimeout is how long a command answering with an XML document may
	// take. The database listings run to thousands of lines, so they get
	// considerably longer than a one-line reply.
	xmlTimeout = 15 * time.Second

	// rebuildTimeout is how long a command that sends the scanner away to
	// rebuild its channel list may take to answer.
	//
	// Writing a position is the one that does this: the scanner works out
	// which of the database's channels are within range of the new position
	// before it acknowledges anything, and answers nothing at all in the
	// meantime. How long that takes depends on how much is in range, so it is
	// unnoticeable at one mile and comfortably over three seconds at
	// twenty-five around a city.
	//
	// Measured on an SDS150 at twenty-five miles around a major city: the write
	// landed correctly every time, and only the acknowledgement was late. So
	// the old three second limit did not prevent anything, it just reported a
	// completed write as a failure, and left the caller believing the position
	// had not changed when it had.
	rebuildTimeout = 30 * time.Second

	// probeTimeout is how long an unidentified port gets to prove it is a
	// scanner. Discover pays this once per candidate port, so it stays short.
	probeTimeout = time.Second
)

// The charger states.
const (
	// ChargeNone means there is no external power or no battery.
	ChargeNone ChargeState = 0

	// ChargeInitializing means the gauge chip is still starting up, so the
	// readings alongside it are not trustworthy yet.
	ChargeInitializing ChargeState = 1

	// ChargeTemperatureFault means the battery is too hot or too cold to
	// charge.
	ChargeTemperatureFault ChargeState = 2

	// ChargePowerFault means the supply is not delivering usable power.
	ChargePowerFault ChargeState = 3

	// ChargeFull means charging has finished.
	ChargeFull ChargeState = 4

	// ChargeTopUp means the battery is being topped back up after falling
	// from full.
	ChargeTopUp ChargeState = 5

	// ChargeCharging means the battery is charging.
	ChargeCharging ChargeState = 6
)

// DefaultPace is used when nothing else says otherwise.
const DefaultPace = PaceTurbo

// The ways the scanner can draw its screen.
const (
	// DisplayColor draws each screen element in the colors set for it under
	// Display Options -> Customize.
	DisplayColor DisplayMode = 0

	// DisplayBlackBackground draws white text on black, ignoring those colors.
	DisplayBlackBackground DisplayMode = 1

	// DisplayWhiteBackground draws black text on white, ignoring those colors.
	DisplayWhiteBackground DisplayMode = 2
)

// frequencyUnit is what one step of the protocol's integer encoding is worth.
const frequencyUnit = Frequency(100)

// FromTopChannel is the channel index that starts scanning from the first
// channel rather than a particular one.
const FromTopChannel = 0xFFFFFFFF

// Common multiples, for callers constructing a Frequency.
const (
	Hertz     Frequency = 1
	Kilohertz           = 1000 * Hertz
	Megahertz           = 1000 * Kilohertz
)

// units are the suffixes a frequency may be written with, and what one of each
// is worth. Matched in lower case, so any spelling of "MHz" is accepted.
var units = map[string]Frequency{
	"hz":  Hertz,
	"khz": Kilohertz,
	"mhz": Megahertz,
}

// holdSuffix is how the scanner names every one of its hold modes: "Scan
// Hold", "Trunk Scan Hold", "Custom Search Hold" and the rest. Matching the
// suffix covers them all, including any a later firmware adds, which matters
// because the specification does not give the list as a set.
const holdSuffix = "Hold"

// The keys the scanner accepts.
const (
	// KeyMenu is the hamburger key on the left side. It opens the menu from a
	// scanning screen and climbs one level from inside the menus.
	KeyMenu Key = "M"

	// KeyFunction is the modifier that selects each key's second label. It
	// does nothing on its own.
	KeyFunction Key = "F"

	// KeyAvoid is the AVOID key. Inside the menus it leaves them entirely
	// rather than climbing one level.
	KeyAvoid Key = "L"

	// KeyEnter is the E/YES key: it selects the highlighted item.
	KeyEnter Key = "E"

	// KeyNo is the . key, labelled NO.
	KeyNo Key = "."

	// KeyRotateRight and KeyRotateLeft turn the knob on top, which moves the
	// selection down and up by one item.
	KeyRotateRight Key = ">"
	KeyRotateLeft  Key = "<"

	// KeyRotatePush presses the knob in, which selects like KeyEnter.
	KeyRotatePush Key = "^"

	// KeySoft1, KeySoft2 and KeySoft3 are the three unlabelled keys above the
	// 1, 2 and 3 keys. KeySoft1 leaves the menus.
	KeySoft1 Key = "A"
	KeySoft2 Key = "B"
	KeySoft3 Key = "C"

	// KeyReplay is the REPLAY key, RECORD when shifted with KeyFunction.
	KeyReplay Key = "Y"

	// KeyZip is the ZIP key, SERVICES when shifted with KeyFunction.
	KeyZip Key = "Z"

	// KeyRange and KeyServiceType are in the specification's table but have no
	// dedicated key on this model. They are accepted and do nothing.
	KeyRange       Key = "R"
	KeyServiceType Key = "T"

	// KeyBacklight is the combined backlight and power key.
	//
	// Sending it with KeyLong is how the scanner is turned off, so prefer
	// PowerOff, which is explicit about what it does.
	KeyBacklight Key = "V"

	// KeySquelchPush presses the squelch control in.
	KeySquelchPush Key = "Q"

	// The number keys.
	Key0 Key = "0"
	Key1 Key = "1"
	Key2 Key = "2"
	Key3 Key = "3"
	Key4 Key = "4"
	Key5 Key = "5"
	Key6 Key = "6"
	Key7 Key = "7"
	Key8 Key = "8"
	Key9 Key = "9"
)

// The ways a key can be pressed.
const (
	// KeyPress is a normal press and release.
	KeyPress KeyAction = "P"

	// KeyLong is a long press, which many keys treat as a second function.
	KeyLong KeyAction = "L"

	// KeyHeld holds the key down without releasing it.
	KeyHeld KeyAction = "H"

	// KeyRelease releases a key held by KeyHeld.
	KeyRelease KeyAction = "R"
)

// The lists List accepts. Each takes one index, naming its immediate parent: a
// department list takes the index of the system holding it, and nothing else.
// Passing the whole path is rejected, so GLT,DEPT,<system> is right and
// GLT,DEPT,<favorites>,<system> is not.
//
// A keyword the scanner does not know is not refused. It answers with the
// favorites list document instead, and reports no error while doing so, which
// is why every caller must check that the elements it got back are the ones it
// asked for. Two keywords used here were wrong for exactly that reason and read
// as firmware bugs for as long as they were: "CHN" for a department's channels,
// which is not a keyword at all, and "SITE_FRQ" for a site's frequencies, which
// is spelled SFREQ. Both work now, and a department's contents are two lists
// rather than one, since a channel holds a frequency or a talkgroup depending
// on the kind of system above it.
const (
	// ListFavorites lists the favorites lists. It takes no index.
	ListFavorites ListKind = "FL"

	// ListSystems lists the systems in a favorites list.
	ListSystems ListKind = "SYS"

	// ListDepartments lists the departments in a system.
	ListDepartments ListKind = "DEPT"

	// ListFrequencies lists the conventional frequencies in a department. A
	// department on a trunked system holds none, and answers emptily.
	ListFrequencies ListKind = "CFREQ"

	// ListTalkgroups lists the talkgroups in a department. A department on a
	// conventional system holds none, and answers emptily.
	ListTalkgroups ListKind = "TGID"

	// ListSites lists the sites in a system. Only a trunked system has any.
	ListSites ListKind = "SITE"

	// ListSiteFrequencies lists the frequencies of a site.
	ListSiteFrequencies ListKind = "SFREQ"

	// ListTriggeredOut lists the tone-out channels. It takes no index.
	ListTriggeredOut ListKind = "FTO"

	// ListCustomSearchBanks lists the ten custom search banks, with the range
	// each one sweeps. It takes no index.
	//
	// This is the only part of a bank the scanner will report without being
	// walked through its menus, and it is most of one: the name, both limits,
	// the modulation and the step. What it leaves out is the attenuator, the
	// delay, the digital waiting time and the search with scan settings.
	ListCustomSearchBanks ListKind = "CS_BANK"
)

// locationCommand reads and writes the position the scanner works from. A read
// is a bare "LCR" and answers at once; a write carries the position after it
// and is the slow one.
const locationCommand = "LCR"

// The menus OpenMenu accepts.
const (
	// MenuTop is the main menu, the one the MENU key opens. It takes no index.
	MenuTop MenuID = "TOP"

	// MenuMonitorList is Select Lists to Monitor, where each favorites list is
	// switched in or out of scanning. It takes no index.
	MenuMonitorList MenuID = "MONITOR_LIST"

	// MenuSystem, MenuDepartment, MenuSite and MenuChannel are the menus for
	// one entry of the database, each opened on the index of the entry it is
	// to describe.
	MenuSystem     MenuID = "SCAN_SYSTEM"
	MenuDepartment MenuID = "SCAN_DEPARTMENT"
	MenuSite       MenuID = "SCAN_SITE"
	MenuChannel    MenuID = "SCAN_CHANNEL"

	// MenuSearchRange is one custom search bank, opened on the bank's index.
	MenuSearchRange MenuID = "SRCH_RANGE"

	// MenuSearchOptions is the settings shared by searching and by close call.
	// It takes no index.
	MenuSearchOptions MenuID = "SRCH_OPT"

	// MenuCloseCall is the close call menu, and MenuCloseCallBand is the band
	// selection within it. Neither takes an index.
	MenuCloseCall     MenuID = "CC"
	MenuCloseCallBand MenuID = "CC_BAND"

	// MenuWeather is the weather operation menu. It takes no index.
	MenuWeather MenuID = "WX"

	// MenuToneOutChannel is one tone-out channel, opened on the channel's
	// index.
	MenuToneOutChannel MenuID = "FTO_CHANNEL"

	// MenuSettings is the settings menu. It takes no index.
	MenuSettings MenuID = "SETTINGS"

	// MenuBroadcastScreen is where whole bands are kept out of a search. It
	// takes no index.
	MenuBroadcastScreen MenuID = "BRDCST_SCREEN"
)

// menuTypeError is what the scanner puts in MenuType when it answers MSI with
// no menu to describe. It answers this rather than refusing the command.
const menuTypeError = "TypeError"

// Volume and squelch share a range on this model. The specification gives
// 0-15 for the SDS100 and SDS150, and wider ranges for the SDS200, so these
// bounds are checked here rather than left to the scanner: a rejected command
// tells the caller nothing about why.
const (
	// MinLevel is the lowest volume or squelch level.
	MinLevel = 0

	// MaxLevel is the highest volume or squelch level on the SDS150.
	MaxLevel = 15
)

// The modes the scanner reports.
const (
	// ModeNormal is any ordinary operating screen, including scanning and
	// searching.
	ModeNormal Mode = 0

	// ModeWaterfall is the waterfall display.
	ModeWaterfall Mode = 1

	// ModeMenu is a menu or a direct entry screen. Most commands that change
	// settings are refused while the scanner is here, which is the single
	// most common reason for ErrRejected.
	ModeMenu Mode = 2
)

// noSignal is the level the scanner reports when nothing is being received.
const noSignal = -999

// The available paces.
const (
	// PaceSlow leaves one second between keys. Use it when a screen is slow to
	// settle, or to watch what the tool is doing.
	PaceSlow Pace = "slow"

	// PaceMedium leaves half a second between keys.
	PaceMedium Pace = "medium"

	// PaceFast leaves a tenth of a second between keys.
	PaceFast Pace = "fast"

	// PaceTurbo adds no delay at all, so keys go out as fast as the serial
	// port accepts them. A command round trip is about 17 milliseconds, which
	// is far quicker than the scanner redraws.
	//
	// It is the default, because everything in this tool that presses a run of
	// keys checks the result of each one: a walk through the menus reads which
	// entry is highlighted before it presses again, and typing a name reads
	// each character back. A press the scanner drops is noticed and repeated
	// rather than silently lost.
	//
	// Anything that presses keys without checking should choose a slower pace
	// explicitly.
	PaceTurbo Pace = "turbo"
)

// The states a quick key can be in.
const (
	// QuickKeyAbsent means no entry is assigned to the key.
	QuickKeyAbsent QuickKeyState = 0

	// QuickKeyDisabled means an entry is assigned but will not be scanned.
	QuickKeyDisabled QuickKeyState = 1

	// QuickKeyEnabled means an entry is assigned and will be scanned.
	QuickKeyEnabled QuickKeyState = 2
)

// The modes JumpToMode accepts.
const (
	// ScanModeScan works through the favorites lists. Its index is a channel
	// index, and FromTopChannel starts from the first channel, which is also
	// how a hold is released.
	ScanModeScan ScanMode = "SCN_MODE"

	// ScanModeCustomSearch sweeps the custom search banks. It takes no index.
	ScanModeCustomSearch ScanMode = "CTM_MODE"

	// ScanModeQuickSearch is quick search, which works from a frequency
	// entered directly rather than from a bank. It takes no index.
	ScanModeQuickSearch ScanMode = "QSH_MODE"

	// ScanModeCloseCall watches for strong nearby transmissions. It takes no
	// index.
	ScanModeCloseCall ScanMode = "CC_MODE"

	// ScanModeWeather works the weather channels. The specification gives its
	// index as NORMAL, A_ONLY, SAME_1 through SAME_5, or ALL_FIPS.
	ScanModeWeather ScanMode = "WX_MODE"

	// ScanModeToneOut waits on a tone-out channel for its alert tone. It takes
	// no index.
	ScanModeToneOut ScanMode = "FTO_MODE"

	// ScanModeWaterfall opens the waterfall display. It takes no index.
	ScanModeWaterfall ScanMode = "WF_MODE"

	// ScanModeReviewRecord plays back the recordings held in the scanner
	// itself. It takes no index.
	ScanModeReviewRecord ScanMode = "IREC_MODE"

	// ScanModeUserRecord records to the memory card. Its index is a folder
	// name.
	ScanModeUserRecord ScanMode = "UREC_MODE"

	// ScanModeToneDiscovery and ScanModeCallDiscovery are the two discovery
	// modes, which log what they find into a named session. Each takes that
	// session's name as its index, and both refuse while a temporary clock is
	// set.
	ScanModeToneDiscovery ScanMode = "TDIS_MODE"
	ScanModeCallDiscovery ScanMode = "CDIS_MODE"
)

// The scanner speaks a line protocol. The host writes a command terminated by
// a carriage return, and the scanner answers with a comma separated line, also
// terminated by a carriage return. The first field of the answer echoes the
// command, so a reply echoing the wrong command means the connection is out of
// sync with the scanner rather than merely unexpected.
//
// Three replies mean failure:
//
//	ERR      the scanner could not parse the command at all
//	CMD,ERR  the command is not valid in the scanner's current mode
//	CMD,NG   the command was understood but refused
//
// The distinction matters. Most commands are refused while the scanner sits in
// a menu, and the same command succeeds once the user backs out to the scan
// screen. Callers get ErrRejected for that case so they can say something more
// useful than "it failed".
const (
	// terminator ends every command and every response line.
	terminator = '\r'

	// okValue is what a command that sets something answers on success.
	okValue = "OK"

	// errName is the whole reply to a command the scanner could not parse. It
	// sits where a command name would, but names no command.
	errName = "ERR"

	// xmlValue marks a response whose payload is the XML that follows on the
	// next lines rather than the rest of this line.
	xmlValue = "<XML>"
)

// acquirePort claims a serial port for this invocation. It is a var so tests
// can substitute a fake.
var acquirePort = portlock.Acquire

// ErrNoScanners is returned when no scanner is attached. The advice is part of
// the error because the answer is never "try again": something about the
// physical setup has to change first.
var ErrNoScanners = errors.New(`no scanner found

Check the following, then run this command again:

  1. Connect the scanner to this computer with a USB data cable. Charge-only
     cables carry power but no data, so the scanner will never appear.
  2. Turn the scanner on and wait for it to finish starting up.
  3. If the scanner asks what the USB connection is for, choose the serial or
     PC connection mode rather than mass storage.
  4. Close any other program that might be holding the serial port, such as
     scanner programming software or a serial terminal.

If you know the port already, pass it directly: radiocli status --device /dev/tty.usbmodem14201`)

// ErrFrequencyNotANumber means a frequency written by hand could not be read
// as a number, with or without a unit.
//
// The three frequency errors are separate values rather than one, because the
// commands that parse a frequency each word the advice for what the reader was
// doing, and the advice differs: a typo wants an example to copy, while a
// frequency below the scanner's range wants the range.
var ErrFrequencyNotANumber = errors.New("not a number of megahertz")

// ErrFrequencyNotPositive means a frequency was read as zero or negative.
var ErrFrequencyNotPositive = errors.New("frequency must be greater than zero")

// ErrFrequencyNotTypeable means a frequency is a number, but not one that
// could be typed into the scanner's frequency entry screen, which has keys for
// digits and a decimal point and nothing else.
var ErrFrequencyNotTypeable = errors.New("not a frequency the scanner's screen would accept")

// ErrFrequencyTooSmall means a frequency was positive but rounds to nothing at
// the resolution the scanner tunes, such as "0.00001kHz".
var ErrFrequencyTooSmall = errors.New("frequency is smaller than the scanner can tune")

// ErrNotInMenu means the scanner is not showing a menu, so there is no menu to
// report. It is a normal answer rather than a fault, and it is the cheapest
// way to ask whether the scanner is in a menu at all.
var ErrNotInMenu = errors.New("scanner is not in a menu")

// ErrRejected means the scanner understood the command but would not run it.
// The usual cause is that the scanner is in a menu or another mode where the
// command does not apply.
var ErrRejected = errors.New("scanner rejected the command in its current mode")

// ErrScannersBusy is returned when the only ports that could have been
// scanners were being used by another invocation of the tool.
//
// It is separate from ErrNoScanners because the advice is the opposite. Here
// nothing is wrong with the setup and nothing needs changing: whatever holds
// the port finishes on its own.
var ErrScannersBusy = errors.New("every scanner found is in use by another radiocli")

// ErrUnsupported means the scanner did not recognise the command. Not every
// command in the specification exists on every model or firmware.
var ErrUnsupported = errors.New("scanner does not support the command")

// listPorts lists the serial ports attached to the system, with the detail
// discovery needs to tell a USB device from anything else. It is a var so tests
// can substitute a fake.
var listPorts = enumerator.GetDetailedPortsList

// openPort opens a serial port. It is a var so tests can substitute a fake.
var openPort = serial.Open

// Paces lists every pace, slowest first. It is the order to show a user, and
// the set to validate against.
var Paces = []Pace{PaceSlow, PaceMedium, PaceFast, PaceTurbo}

// scanModes are the scanner's names for working through the favorites lists.
// Trunk Scan is that same scan, named for the kind of system it has reached
// rather than for a different activity, so both count.
var scanModes = []string{"Scan Mode", "Trunk Scan"}

// Conn is the transport to a scanner: it sends command strings and returns
// response strings, and knows nothing about what any command means.
//
// Scanner is built on top of it, so a test can drive every typed command
// against a fake Conn without a scanner attached.
type Conn interface {
	// Info describes the connected scanner.
	Info() Info

	// Execute sends one command and returns its response with the echoed
	// command stripped. It is safe for concurrent use.
	Execute(ctx context.Context, command string) (string, error)

	// ExecuteXML sends one command whose response is an XML document spread
	// over several lines, and returns the document.
	ExecuteXML(ctx context.Context, command string) (string, error)

	// Send writes a command without waiting for a response, for the few
	// commands the scanner does not answer.
	Send(ctx context.Context, command string) error

	// Close releases the serial port.
	Close() error
}

// Backlight is whether the scanner is lit, and how brightly.
type Backlight struct {
	// On reports whether the scanner is lit at all.
	On bool `json:"on"`

	// Level is the brightness, which is the dimmer setting while lit and zero
	// while dark. It is reported as well as On because the two answer
	// different questions: whether the light is on, and how bright the dimmer
	// has it.
	Level int `json:"level"`
}

// Battery is the battery's charge and condition.
type Battery struct {
	// State is what the charger is doing.
	State ChargeState `json:"state"`

	// Millivolts is the battery voltage.
	Millivolts int `json:"millivolts"`

	// Percent is the remaining capacity, from 0 to 100.
	Percent int `json:"percent"`

	// Milliamps is the current flowing. It is positive while charging and
	// negative while the scanner runs on the battery.
	Milliamps int `json:"milliamps"`

	// Celsius is the battery temperature.
	Celsius float64 `json:"celsius"`
}

// Clock is the scanner's date, time, and clock health.
type Clock struct {
	// Time is the scanner's local time. It carries no zone, because the
	// scanner reports wall clock digits rather than an instant, so it is
	// built in time.Local.
	Time time.Time `json:"time"`

	// DaylightSaving reports whether the scanner is applying daylight saving.
	DaylightSaving bool `json:"daylightSaving"`

	// Valid reports whether the real time clock is running. A scanner that
	// has been without power long enough loses the clock, and reports the
	// time it has as unreliable.
	Valid bool `json:"valid"`
}

// conn is the serial implementation of Conn.
//
// The scanner handles one command at a time, so the mutex is what makes the
// methods safe for concurrent callers rather than merely protecting fields.
type conn struct {
	port serial.Port  // Open serial port, nil once the connection is closed
	log  *slog.Logger // Where each command and its response are logged, at debug level
	info Info         // The scanner on the other end, and the port path errors name

	// lock is this invocation's claim on the port, held for as long as the
	// connection is open so that a whole menu walk is indivisible rather than
	// each exchange within it. Nil when the caller did not take one, which is
	// the case while a port is only being probed.
	lock *portlock.Lock

	// pending holds bytes read from the port but not yet returned as a line.
	// One read can span several lines, and an XML response is many lines, so
	// the remainder has to survive between calls: dropping it would silently
	// lose the middle of a document.
	pending []byte

	// closing is raised by Close before it asks for the mutex, and read by the
	// wait inside readLine. It is deliberately outside the mutex: the whole
	// point of it is to be readable by the exchange that is holding the mutex
	// Close is waiting for.
	closing atomic.Bool

	mu sync.Mutex // Held for a whole exchange, so a command and its response stay together
}

// Display is the scanner's screen as text.
//
// It is the closest thing the protocol has to "what is happening right now":
// the same lines a person would read off the scanner, in the same order.
type Display struct {
	// Lines are the screen's lines, top to bottom. Blank lines are kept, so
	// an index into this slice is the line's position on the screen.
	Lines []Line `json:"lines"`

	// LargeFont holds one flag per line, reporting whether the scanner drew
	// that line in the large font.
	LargeFont []bool `json:"largeFont"`
}

// Info identifies a scanner found on the system.
type Info struct {
	// Port is the serial device path, such as /dev/tty.usbmodem14201. It is
	// what gets passed back to the tool as --device.
	Port string `json:"port"`

	// Model is the model string the scanner reports, such as SDS150.
	Model string `json:"model"`

	// Serial is the USB serial number, empty if the port does not report one.
	// It is the only field that distinguishes two identical scanners.
	Serial string `json:"serial,omitempty"`
}

// Line is one line of the scanner's screen: its text, and an attribute per
// character describing how that character is drawn.
type Line struct {
	// Text is the line as displayed, with trailing padding removed.
	Text string `json:"text"`

	// Attributes holds one Attribute per character of the original, unpadded
	// line. It is empty when the whole line is drawn normally, which is the
	// common case and how the scanner reports it.
	Attributes string `json:"attributes,omitempty"`
}

// Location is the position the scanner is working from, and how far around it
// it draws channels out of the database.
//
// This is the position in use, which is not the same thing as where the
// scanner is. Left alone it tracks the built-in GPS, and successive readings
// from a stationary scanner differ by a metre or two as the fix refines. But
// entering a zip code through the scanner's menus replaces it outright, and
// what LCR reports afterwards is the zip's position, from wherever the scanner
// physically sits. That override survives a power cycle: an SDS150 in
// Greendale, given zip 24944, reported Green Bank across a reboot and the GPS did
// not reclaim it.
//
// So a reading here answers "what is the scanner scanning around", not "where
// is the scanner".
type Location struct {
	// Latitude and Longitude are in degrees, positive north and east.
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`

	// Range is the radius in miles that the scanner pulls database channels
	// from, and matches the value on its own Set Range screen.
	//
	// It reads 0 while the position is coming from the GPS and nothing has set
	// a range, which is what made it look meaningless. Entering a zip code
	// sets it to 10 without being asked, and the Set Range screen then shows
	// 10.0 in miles, which is what identified it.
	Range float64 `json:"range"`
}

// MenuInfo is the menu the scanner is showing, and the items in it.
type MenuInfo struct {
	// Title is the menu's heading, as shown at the top of the screen.
	Title string `xml:"Name,attr" json:"title"`

	// Index identifies the menu for the commands that navigate it.
	Index string `xml:"Index,attr" json:"index"`

	// Type is the kind of menu, such as "TypeSelect".
	Type string `xml:"MenuType,attr" json:"type,omitempty"`

	// Value is the currently selected item's value.
	Value string `xml:"Value,attr" json:"value,omitempty"`

	// Selected is the index of the highlighted item within Items.
	Selected string `xml:"Selected,attr" json:"selected,omitempty"`

	// Items are the menu's entries, in the order shown.
	Items []MenuItem `xml:"MenuItem" json:"items"`

	// Input describes the text entry screen, when Type is "TypeInput". Those
	// screens carry no Items: their content is Value.
	Input MenuInput `xml:"MenuInput" json:"input,omitempty"`

	// XML is the document exactly as the scanner sent it.
	XML string `xml:"-" json:"-"`
}

// MenuInput describes a text entry screen.
type MenuInput struct {
	// MaxLength is the longest value the screen accepts.
	MaxLength int `xml:"MaxLength,attr" json:"maxLength,omitempty"`

	// EnableKeys is every character the screen accepts, and, as far as this
	// has been observed, the order the knob cycles them in: turning it right
	// moves one character along this string, and past the end wraps to the
	// start.
	EnableKeys string `xml:"EnableKeys,attr" json:"enableKeys,omitempty"`
}

// MenuItem is one entry of a menu.
type MenuItem struct {
	Name  string `xml:"Name,attr" json:"name"`   // The entry as the screen labels it
	Index string `xml:"Index,attr" json:"index"` // The scanner's own index for the entry, which is not its position
}

// ConvFrequency is the conventional channel the scanner is on.
//
// It is the closest thing the protocol has to "the channel", and it is what a
// conventional system reports instead of the Channel element the specification
// describes. Read off an SDS150 on firmware 1.00.37.
type ConvFrequency struct {
	// Name is the channel's alpha tag, such as "FIREGROUND 3".
	Name string `xml:"Name,attr" json:"name,omitempty"`

	// Index is the scanner's own index for the channel.
	Index string `xml:"Index,attr" json:"index,omitempty"`

	// Avoid is "Off", "T-Avoid" or "Avoid".
	Avoid string `xml:"Avoid,attr" json:"avoid,omitempty"`

	// Frequency carries its own unit, such as "155.235000MHz". The scanner
	// writes it with a leading space, which ScannerInfo strips.
	Frequency string `xml:"Freq,attr" json:"frequency,omitempty"`

	// Modulation is how the scanner is demodulating, such as "NFM".
	Modulation string `xml:"Mod,attr" json:"modulation,omitempty"`

	// ServiceType is the category the channel is filed under, such as
	// "Custom 1".
	ServiceType string `xml:"SvcType,attr" json:"serviceType,omitempty"`

	// Held reports whether the scanner is parked on this channel, "On" or
	// "Off".
	Held string `xml:"Hold,attr" json:"held,omitempty"`

	// Talkgroup and UnitID carry the identifiers decoded out of a digital
	// signal on this frequency, and are empty when there are none.
	//
	// The scanner writes the words "TGID None" and "UID None" rather than
	// leaving them out, which ScannerInfo turns into the empty string so that
	// absent reads as absent everywhere above here.
	Talkgroup string `xml:"TGID,attr" json:"talkgroup,omitempty"`
	UnitID    string `xml:"U_Id,attr" json:"unitId,omitempty"`
}

// MonitorList is the favorites list or database the scanner is working
// through, which sits above the system in the hierarchy.
type MonitorList struct {
	// Name is the list's name, such as "Green Bank".
	Name string `xml:"Name,attr" json:"name,omitempty"`

	// Index is the scanner's own index for the list.
	Index string `xml:"Index,attr" json:"index,omitempty"`

	// Type is where the entries come from: "FL" for a favorites list, "FullDb"
	// for the downloaded database, or "SWS" for a service search.
	Type string `xml:"ListType,attr" json:"type,omitempty"`
}

// Talkgroup is the trunked talkgroup the scanner is on.
//
// The attribute names were first taken from the protocol specification,
// following the spelling ConvFrequency uses for the same identifiers, and were
// confirmed against hardware on 2026-08-23 when a live P25 transmission came
// through: Name and TGID arrived spelled as modelled, with the ID carrying the
// same "TGID:" prefix the conventional element uses. UnitID is the one field
// still unconfirmed in its active form, because no capture has yet caught the
// scanner naming the transmitting radio. The raw document is in
// ScannerInfo.XML for checking it when one does.
type Talkgroup struct {
	// Name is the talkgroup's alpha tag.
	Name string `xml:"Name,attr" json:"name,omitempty"`

	// Index is the scanner's own index for the talkgroup.
	Index string `xml:"Index,attr" json:"index,omitempty"`

	// Avoid is "Off", "T-Avoid" or "Avoid".
	Avoid string `xml:"Avoid,attr" json:"avoid,omitempty"`

	// ID is the talkgroup number itself.
	ID string `xml:"TGID,attr" json:"id,omitempty"`

	// UnitID is the radio heard transmitting, empty when none was decoded.
	UnitID string `xml:"U_Id,attr" json:"unitId,omitempty"`

	// ServiceType is the category the talkgroup is filed under.
	ServiceType string `xml:"SvcType,attr" json:"serviceType,omitempty"`

	// Held reports whether the scanner is parked on this talkgroup.
	Held string `xml:"Hold,attr" json:"held,omitempty"`
}

// Heard is what the scanner is listening to at one instant, flattened out of
// the several elements that describe it.
//
// It exists because two very different callers need the same answer in the same
// shape: the "receiving" command renders it, and the recorder reads that
// command's JSON back over a daemon socket to label a transmission. Defining it
// once here is what stops those two drifting apart, since a command package may
// not import another command package and would otherwise carry its own copy of
// the field names.
//
// The channel fields are filled in whether or not anything is being received,
// because a scanning radio names whichever channel it is stepping past at the
// instant it is asked. Receiving is what separates the two, and it is the field
// to read first.
type Heard struct {
	// Receiving reports whether audio is coming out of the scanner right now,
	// and is what makes the rest of this either a transmission or a channel the
	// scanner happened to be passing.
	Receiving bool `json:"receiving"`

	// List is the favorites list or database the channel was found in.
	List string `json:"list,omitempty"`

	// System and Department are where the channel sits in the scanner memory.
	System     string `json:"system,omitempty"`
	Department string `json:"department,omitempty"`

	// Site is the trunked site being listened to, empty on a conventional
	// system.
	Site string `json:"site,omitempty"`

	// Channel is the channel alpha tag, such as "Marlinton Dispatch".
	Channel string `json:"channel,omitempty"`

	// Frequency is what the scanner is tuned to on a conventional system,
	// carrying its own unit. Talkgroup is the number on a trunked one. Only one
	// of the two is ever set.
	Frequency string `json:"frequency,omitempty"`
	Talkgroup string `json:"talkgroup,omitempty"`

	// Unit is the radio heard transmitting, when the scanner decoded one.
	Unit string `json:"unit,omitempty"`

	// Modulation is how the scanner is demodulating, such as "NFM".
	Modulation string `json:"modulation,omitempty"`

	// Signal is the number of bars the scanner is showing, from "0" to "5".
	Signal string `json:"signal,omitempty"`

	// RSSI is the received signal strength in the scanner own units. It reads
	// "-999" when nothing is coming in, which is the scanner saying it has
	// nothing to report rather than a measurement.
	RSSI string `json:"rssi,omitempty"`

	// Mode is what the scanner is doing, in its own words.
	Mode string `json:"mode"`
}

// Named is an element carrying a name and an index, which is how the scanner
// reports most database entries.
type Named struct {
	Name  string `xml:"Name,attr" json:"name,omitempty"`   // The entry's name, as the scanner reports it
	Index string `xml:"Index,attr" json:"index,omitempty"` // The scanner's own index for the entry, empty when it reports none
}

// namedEither unifies the two spellings the scanner uses for the same
// attribute: some elements carry Name and Index, others name and index.
//
// Both are read here, in one type at one depth, because that is the only way to
// get both. Reading them with two fields on the same element does not work: the
// decoder gives the shallower field the element and leaves the other empty, so
// whichever spelling lost is silently dropped. That is what used to happen to
// the capitalised pair, which meant a menu the scanner named that way came back
// blank.
type namedEither struct {
	Name  string `xml:"Name,attr"`  // The entry's name, from the capitalised spelling
	Index string `xml:"Index,attr"` // The entry's index, from the capitalised spelling

	NameLower  string `xml:"name,attr"`  // The entry's name, from the lowercase spelling
	IndexLower string `xml:"index,attr"` // The entry's index, from the lowercase spelling
}

// Property is the scanner's state as it reports it alongside what it is doing.
//
// This is where signal strength lives, and it is the reliable place to read it:
// the separate signal strength command answers -999 for a moment after the
// scanner is moved, whereas these values are populated as soon as the scanner
// reports anything at all.
type Property struct {
	// RSSI is the received signal strength, in the scanner's own units.
	// Larger, meaning closer to zero, is stronger. It is empty when the
	// scanner has nothing to report.
	RSSI string `xml:"Rssi,attr" json:"rssi,omitempty"`

	// Signal is the number of signal bars the scanner is showing, as a string
	// because it is empty when there are none.
	Signal string `xml:"Sig,attr" json:"signal,omitempty"`

	// Volume and Squelch are the current levels.
	Volume  string `xml:"VOL,attr" json:"volume,omitempty"`
	Squelch string `xml:"SQL,attr" json:"squelch,omitempty"`

	// Mute is "Mute" or "Unmute".
	Mute string `xml:"Mute,attr" json:"mute,omitempty"`

	// Recording is "On" or "Off", and reports the scanner's own recorder,
	// which writes to the memory card inside it and has nothing to do with
	// anything this tool records. See SetRecording.
	Recording string `xml:"Rec,attr" json:"recording,omitempty"`
}

// Scanner is a connected scanner, with one method per command it understands.
//
// It is the type CLI commands work with. Each method owns the wire format of
// one scanner command: what to send, how to read the reply, and what the
// fields mean. A caller says what it wants, never how the protocol spells it,
// so command code stays free of comma counting and field indexes.
//
// Scanner is safe for concurrent use, because the Conn beneath it serialises
// exchanges. The scanner itself handles one command at a time.
type Scanner struct {
	conn Conn // Transport every command is written to

	// keys guards the pacing of key presses. It also serialises them, which
	// matches the hardware: the scanner acts on one key at a time.
	keys    sync.Mutex
	pace    Pace      // Minimum gap to leave between one key and the next
	lastKey time.Time // When the last key went out, which the gap is measured from
}

// ScannerInfo is what the scanner is doing right now, in structured form.
//
// It is the richest thing the protocol reports: where the scanner is in its
// database, what it is receiving, and what its screen would show. The fields
// present depend on the mode, so most are pointers or slices that are empty
// when they do not apply.
type ScannerInfo struct {
	// Mode is the scanner's own name for what it is doing, such as
	// "Scan" or "Menu tree".
	Mode string `xml:"Mode,attr" json:"mode"`

	// Screen names the view being shown, such as "menu_selection".
	Screen string `xml:"V_Screen,attr" json:"screen"`

	// System, Department, and Channel describe where the scanner is in the
	// database. They are empty unless it is receiving or holding.
	//
	// Channel is the odd one out and is usually empty even then. The scanner
	// does not send a Channel element on this firmware: a conventional channel
	// arrives as ConvFrequency and a talkgroup as TGID, each carrying its own
	// name. Use Tuned, which returns whichever of them applies. The field is
	// kept because the protocol specification describes it and a different
	// model or firmware may yet send one.
	System     Named `xml:"System" json:"system"`
	Department Named `xml:"Department" json:"department"`
	Channel    Named `xml:"Channel" json:"channel"`

	// List is the favorites list or database the scanner is working through,
	// which sits above System. The scanner calls it the monitor list.
	List MonitorList `xml:"MonitorList" json:"list,omitempty"`

	// Site is the trunked site being listened to, and is absent on a
	// conventional system.
	Site Named `xml:"Site" json:"site,omitempty"`

	// Frequency is the conventional channel the scanner is on, present only in
	// conventional modes.
	Frequency ConvFrequency `xml:"ConvFrequency" json:"frequency,omitempty"`

	// Talkgroup is the trunked talkgroup the scanner is on, present only in
	// trunked modes.
	Talkgroup Talkgroup `xml:"TGID" json:"talkgroup,omitempty"`

	// Menu describes the menu the scanner is in, when it is in one.
	Menu Named `xml:"MenuSummary" json:"menu"`

	// Property is the scanner's current state: signal strength, volume,
	// squelch, and whether it is muted.
	Property Property `xml:"Property" json:"property"`

	// SearchRange is the span the scanner will search and tune within, when it
	// is searching. It is empty in the modes that are not.
	SearchRange SearchRange `xml:"SearchRange" json:"searchRange,omitempty"`

	// Weather is what the scanner is doing on the weather channels. It is
	// empty unless it is on them. The scanner reports it as two elements
	// rather than one, so it is assembled in ScannerInfo rather than being
	// unmarshalled straight from the document.
	Weather Weather `xml:"-" json:"weather,omitempty"`

	// XML is the document exactly as the scanner sent it. The protocol
	// carries more than this type models, and firmware can add fields, so the
	// original is kept for callers that need something not modelled here.
	XML string `xml:"-" json:"-"`
}

// Screen is the scanner's current screen and indicator state.
type Screen struct {
	// Display is the screen as text.
	Display Display `json:"display"`

	// Muted reports whether audio output is muted.
	Muted bool `json:"muted"`

	// AlertLED is the colour of the alert light.
	AlertLED LED `json:"alertLed"`

	// ChargeLED is the colour of the battery charge light. The specification
	// notes this light exists only on the SDS150.
	ChargeLED LED `json:"chargeLed"`

	// Mode is what the scanner is doing, as GST reports it.
	//
	// Do not rely on this to tell whether the scanner is in a menu. On SDS150
	// firmware 1.00.37 the field is static: it reads the same on a scanning
	// screen as it does with the main menu open. ScannerInfo reports the mode
	// correctly, and MenuInfo returns ErrNotInMenu when there is no menu.
	Mode Mode `json:"mode"`

	// DisplayMode is whether the scanner is drawing its screen in color.
	//
	// The specification calls this field COLOR_MODE and says it belongs to the
	// waterfall display. It does not. Measured on an SDS150 on firmware
	// 1.00.37: it follows MENU -> Display Options -> Set B/W or Color Mode with
	// the waterfall never opened, reading 0, 1 and 2 for the menu's three
	// entries in order.
	//
	// It is the only thing about the screen's appearance the protocol reports.
	// The per-element colors are reachable only through the menus.
	DisplayMode DisplayMode `json:"displayMode"`

	// Waterfall holds the waterfall display's frequencies and settings. It is
	// meaningful only when Mode is ModeWaterfall; every field is zero
	// otherwise, because the scanner sends the fields empty.
	Waterfall Waterfall `json:"waterfall"`
}

// SearchRange is the span the scanner searches, as it reports it.
//
// The bounds are strings carrying their own units, exactly as the scanner
// writes them on its screen, because that is the form worth showing a reader
// and nothing here needs to do arithmetic on them.
type SearchRange struct {
	// Lower and Upper are the ends of the span the scanner is working
	// through, low end first.
	Lower string `xml:"Lower,attr" json:"lower,omitempty"`
	Upper string `xml:"Upper,attr" json:"upper,omitempty"`

	// Modulation is how the scanner is demodulating, such as "WFMB" for
	// broadcast FM.
	Modulation string `xml:"Mod,attr" json:"modulation,omitempty"`

	// Step is the spacing it moves in, such as "Auto".
	Step string `xml:"Step,attr" json:"step,omitempty"`
}

// Signal is a signal strength reading and the frequency it was taken on.
type Signal struct {
	// Level is the reading the scanner reports. Its units are not documented.
	// Larger means stronger, and the scanner reports -999 from the PWR
	// command when there is no signal to measure.
	Level int `json:"level"`

	// Frequency is the frequency the reading was taken on.
	Frequency Frequency `json:"frequency"`
}

// Target names one entry in the scanner's database, for the commands that act
// on a system, department, or channel.
//
// The protocol identifies these with a type keyword and up to two indexes
// whose meaning depends on the keyword. That encoding is kept here rather than
// spread across callers.
type Target struct {
	// Kind is the type keyword, such as SYS for a system.
	Kind string

	// First and Second are the indexes the keyword needs. Leave Second empty
	// when the keyword takes only one.
	First  string
	Second string
}

// Waterfall describes the waterfall display's tuning.
type Waterfall struct {
	// Marked is the frequency the marker sits on.
	Marked Frequency `json:"marked"`

	// Modulation is the demodulation mode at the marker, such as FM or NFM.
	Modulation string `json:"modulation,omitempty"`

	// MarkerPosition is the marker's horizontal position on the display.
	MarkerPosition int `json:"markerPosition"`

	// Center, Lower and Upper are the span the display covers.
	Center Frequency `json:"center"`
	Lower  Frequency `json:"lower"`
	Upper  Frequency `json:"upper"`

	// FFTSize is how much of the screen the FFT area fills: 0 for 25%, 1 for
	// 50%, 2 for 75%, 3 for 100%.
	FFTSize int `json:"fftSize"`
}

// Weather is what the scanner is doing on its weather channels.
//
// It is worth having separately from Mode because Mode does not distinguish
// the two things the scanner does there. Both report a Mode of "WX Scan" and a
// screen of "wx_alert", and only this says which is which.
type Weather struct {
	// Mode is which of the two weather modes the scanner is in: "Monitor
	// Weather", which plays the broadcast continuously, or "Weather Alert",
	// which sits silent on the channel until an alert tone arrives. It is
	// empty when the scanner is not on the weather channels at all.
	Mode string `json:"mode,omitempty"`

	// Channel is the weather channel the scanner is sitting on.
	Channel WeatherChannel `json:"channel,omitempty"`
}

// WeatherChannel is the weather channel the scanner is on, as it reports it.
type WeatherChannel struct {
	// Number is the channel as the screen labels it, such as "7".
	Number string `xml:"CH_No,attr" json:"number,omitempty"`

	// Held reports whether the scanner is parked on this channel rather than
	// working through them. The scanner writes "On" or "Off".
	Held string `xml:"Hold,attr" json:"held,omitempty"`

	// Frequency is what the channel is tuned to, carrying its own unit,
	// such as "162.525000MHz". The scanner writes it with a leading space,
	// which is stripped here.
	Frequency string `xml:"Freq,attr" json:"frequency,omitempty"`

	// Modulation is how the scanner is demodulating, such as "FM".
	Modulation string `xml:"Mod,attr" json:"modulation,omitempty"`
}
