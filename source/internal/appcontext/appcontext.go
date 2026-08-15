// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package appcontext holds the application context: the single value that
// carries every dependency a command needs (logger, config, IO streams, and
// any clients added later).
//
// It exists so commands never construct their own dependencies. main builds
// one App, hands it to each command package, and commands use only what the
// App exposes. That makes commands trivially testable: swap the streams for
// buffers, the logger for a discard handler, and any client for a fake.
package appcontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/portlock"
)

// New returns an App wired to the real process streams and safe defaults.
//
// It deliberately does no I/O and starts no connections: it runs before the
// command line has been parsed, so nothing here can depend on flag values.
// Everything that needs configuration belongs in Init.
//
// Returns:
//   - *App with default settings, a discard logger, and the os process streams
func New() *App {
	return &App{
		Config: defaultConfig(),
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}
}

// Init finishes building the App once Config has been fully resolved.
//
// The root command calls it from PersistentPreRunE after loading the config
// file and applying the global flags, so by the time any command's RunE runs,
// every dependency is ready. Build clients here, not in New: only here do the
// user's settings exist.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while building clients
//
// Returns:
//   - error if a client cannot be built; currently always nil
func (a *App) Init(ctx context.Context) error {
	level := slog.LevelInfo
	if a.Config.Verbose {
		level = slog.LevelDebug
	}
	a.Log = slog.New(slog.NewTextHandler(a.Stderr, &slog.HandlerOptions{Level: level}))

	// Build clients here, for example:
	//
	//	dev, err := device.Dial(ctx, a.Config.Address)
	//	if err != nil {
	//		return fmt.Errorf("connecting to device: %w", err)
	//	}
	//	a.Device = dev
	//	a.OnClose(dev.Close)

	return nil
}

// Borrow returns a second App that shares this one's scanner.
//
// The settings are copied rather than shared, so the two cannot change each
// other's output format or verbosity, and the streams start out as this App's
// and are meant to be replaced. What is shared is the connection, which is
// safe because the connection serializes one exchange at a time.
//
// The borrowed App must not be given work that moves the radio. Nothing here
// enforces that: whoever calls this decides what it is allowed to run, and main
// is what does.
//
// Returns:
//   - *App that shares this one's scanner connection, with copied settings
//     and the borrowed mark set
func (a *App) Borrow() *App {
	return &App{
		Config:   a.Config.Clone(),
		Log:      a.Log,
		Stdout:   a.Stdout,
		Stderr:   a.Stderr,
		Stdin:    a.Stdin,
		device:   a.device,
		borrowed: true,
		InDaemon: a.InDaemon,
	}
}

// Device returns the scanner connection, opening it on first use.
//
// Connecting is deferred rather than done in Init because most invocations do
// not touch the scanner, and opening the serial port is slow enough to be felt
// on commands like version and help. The connection is cached, so a command
// that calls this repeatedly still gets one port.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while opening the port
//
// Returns:
//   - *device.Scanner connected to the configured serial port
//   - error if no scanner was named or the port cannot be opened
//
// Errors:
//   - ErrNoDevice: if Config.Device is empty
//   - portlock.ErrBusy: if another invocation of the tool holds the port
func (a *App) Device(ctx context.Context) (*device.Scanner, error) {
	if a.device != nil {
		// The connection outlives a single invocation in a long-running
		// command like daemon, and every invocation resolves its own settings,
		// so the pace has to be applied again here. Without this the first
		// command to touch the scanner would fix the pace for the whole
		// session and every later --pace would be quietly ignored.
		//
		// A borrowed connection is the exception, and has to be. The pace is a
		// property of the connection rather than of the caller, so setting it
		// here would change how fast somebody else's keys are being pressed
		// half way through their menu walk. A borrowed App presses no keys, so
		// it has no business having an opinion about it.
		if !a.borrowed {
			a.applyPace(a.device)
		}
		return a.device, nil
	}

	if a.Config.Device == "" {
		return nil, fmt.Errorf("%w: name one with --device, or run \"radiocli devices\" to see what is attached", ErrNoDevice)
	}

	client, err := openDevice(ctx, a.Config.Device, a.Config.Wait, a.Log)
	if err != nil {
		// A port held by another invocation is the one failure where picking
		// a different scanner is the wrong answer. This is the right scanner
		// and it is working; something else is mid-command on it, and the
		// advice the busy error already carries is the advice that helps.
		if errors.Is(err, portlock.ErrBusy) {
			return nil, err
		}
		return nil, fmt.Errorf("%w (run \"radiocli devices\" to see what is attached)", err)
	}

	a.SetDevice(client)
	return client, nil
}

// SetDevice installs an already open connection, so it is reused instead of
// reopened. A command that opened a port itself uses it, and tests use it to
// inject a fake.
//
// Parameters:
//   - client: an open scanner connection to cache and to close on Close
func (a *App) SetDevice(client *device.Scanner) {
	a.applyPace(client)
	a.device = client
	a.OnClose(client.Close)
}

// OnClose registers a shutdown function to run when Close is called. Functions
// run in reverse registration order, so dependencies tear down after the
// things that use them.
//
// Parameters:
//   - fn: shutdown function to run when Close is called
func (a *App) OnClose(fn func() error) {
	a.closers = append(a.closers, fn)
}

// Close releases everything registered with OnClose. It runs every closer even
// if an earlier one fails, and returns the first error encountered.
//
// Returns:
//   - error from the first closer that failed, nil when all succeed
func (a *App) Close() error {
	var firstErr error
	for i := len(a.closers) - 1; i >= 0; i-- {
		if err := a.closers[i](); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.closers = nil
	return firstErr
}

// Printf writes formatted program output to Stdout. Use it for the result the
// user asked for, and nothing else: in JSON mode Stdout must stay parseable by
// whatever the output is piped into.
//
// Parameters:
//   - format: fmt format string for the result
//   - args: values interpolated into format
func (a *App) Printf(format string, args ...any) {
	fmt.Fprintf(a.Stdout, format, args...)
}

// Notef writes a formatted note to Stderr: progress, prompts, and anything
// else meant for the person watching rather than for a program reading the
// output. Use Log instead for diagnostics that only matter when debugging.
//
// Parameters:
//   - format: fmt format string for the note
//   - args: values interpolated into format
func (a *App) Notef(format string, args ...any) {
	fmt.Fprintf(a.Stderr, format, args...)
}

// Load reads the config file over the current settings.
//
// A missing file at the default location is not an error: the tool must be
// usable with no configuration at all. A missing file the user named
// explicitly with --config is an error, because they meant to use it.
//
// Returns:
//   - error if the file cannot be located, read, or parsed; nil when the file
//     loaded or was absent from the default location
func (c *Config) Load() error {
	path := c.Path
	if path == "" {
		var err error
		if path, err = defaultConfigPath(); err != nil {
			return err
		}
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && c.Path == "" {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// The macros are read separately, into a list of their own, because
	// decoding a list onto a list that already holds something fills in what
	// the file names and leaves the rest as it was. Read with everything else,
	// a macro would take fields from whichever default happened to sit at its
	// index, and a shorter list would keep the tail of a longer one. The
	// pointer is what tells a file that names no macros, and so gets the
	// built-in ones, from one that names an empty list and so gets none.
	var file struct {
		Macros *[]Macro `json:"macros"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if file.Macros != nil {
		c.Macros = *file.Macros
	}

	return nil
}

// Validate reports whether the fully resolved settings are usable. The root
// command calls it after the flags have been applied.
//
// Returns:
//   - error naming the first invalid setting, nil when all are usable
func (c *Config) Validate() error {
	switch c.Output {
	case OutputText, OutputJSON:
	default:
		return fmt.Errorf("invalid output format %q: want %q or %q", c.Output, OutputText, OutputJSON)
	}

	if !c.Pace.Valid() {
		return fmt.Errorf("invalid pace %q: want %s", string(c.Pace), device.PaceNames())
	}

	if c.Wait < 0 {
		return fmt.Errorf("invalid wait %s: want a duration of zero or more", c.Wait)
	}
	return nil
}

// Location returns the config file this invocation reads and writes: the one
// named by --config, or the default one.
//
// Returns:
//   - string path of the config file
//   - error if the user config directory cannot be located
func (c *Config) Location() (string, error) {
	if c.Path != "" {
		return c.Path, nil
	}
	return defaultConfigPath()
}

// Defaults returns the settings used when nothing else specifies them.
//
// It exists for the config command, which has to show what a setting would be
// if it were not set, and cannot work that out by looking at a resolved Config:
// by then the defaults, the file and the flags have already been folded
// together and there is no telling which of them a value came from.
//
// Returns:
//   - *Config holding the built-in defaults, independent of any file or flags
func Defaults() *Config {
	return defaultConfig()
}

// Saved returns the settings as the config file has them, without the flags
// from this invocation.
//
// This is what the config command reports, and it is deliberately not the
// resolved Config the rest of the tool uses. "radiocli -o json config" is
// being asked to print this run's answer as JSON; it is not being asked what
// the file says, and answering "output: json" would be wrong in a way that
// would quietly get written back by the next set.
//
// Returns:
//   - *Config as the file has it, laid over the defaults
//   - error if the file cannot be located, read, or parsed
func (c *Config) Saved() (*Config, error) {
	saved := defaultConfig()
	saved.Path = c.Path

	if err := saved.Load(); err != nil {
		return nil, err
	}
	return saved, nil
}

// Clone returns a copy of the settings that shares nothing with this one.
//
// A plain struct copy is not enough. Macros is a slice, and each Macro holds a
// slice of steps, so a copied Config points at the same backing arrays and a
// change written through one element is a change to both. That matters wherever
// a copy is taken in order to put the original back afterwards: the daemon
// saves the settings around every command it runs, and a command that reached
// into a macro would otherwise leave its change behind after the restore.
//
// Returns:
//   - *Config holding the same settings with its own copy of the macros
func (c *Config) Clone() *Config {
	clone := *c

	if c.Macros != nil {
		clone.Macros = make([]Macro, len(c.Macros))
		for i, macro := range c.Macros {
			clone.Macros[i] = macro
			if macro.Steps != nil {
				clone.Macros[i].Steps = append([]string(nil), macro.Steps...)
			}
		}
	}
	return &clone
}

// Update changes the settings on disk, creating the file if it is not there
// yet, and applies the same change in memory.
//
// It deliberately does not write the in-memory Config: that holds the flags
// from this invocation, and a one-off "-o json" or "--verbose" must not become
// a permanent setting. Instead it re-reads the file, lets fn change only what
// the command means to change, and writes that back.
//
// The in-memory change is applied last, after the file is safely written, so a
// run that could not save reports the failure with memory and disk still
// agreeing. Applying it first would leave this invocation believing a setting
// it failed to store, and the next command reading the old value back.
//
// Parameters:
//   - fn: applies the change; it runs on the on-disk settings and then on c
//
// Returns:
//   - error if the file cannot be read, encoded, or written back
func (c *Config) Update(fn func(*Config)) error {
	path := c.Path
	if path == "" {
		var err error
		if path, err = defaultConfigPath(); err != nil {
			return err
		}
	}

	// Start from what is on disk, not from c, so unrelated settings survive.
	onDisk := defaultConfig()
	onDisk.Path = c.Path
	if err := onDisk.Load(); err != nil {
		return err
	}
	fn(onDisk)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	data, err := marshalJSON(onDisk, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	fn(c)
	return nil
}

// applyPace puts the currently resolved pace onto the connection.
//
// It runs when a connection is installed and again every time a cached one is
// handed out, because the settings are resolved once per invocation and a
// cached connection can outlive several of them.
//
// Validation happened in Config.Validate, so a failure here would mean the
// settings changed underneath us, which is worth a warning rather than
// refusing to run the command.
//
// Parameters:
//   - client: the connection to pace; a failure is logged, not returned
func (a *App) applyPace(client *device.Scanner) {
	if err := client.SetPace(a.Config.Pace); err != nil {
		a.Log.Warn("keeping the default key pace", "reason", err)
	}
}

// defaultConfig returns the settings used when nothing else specifies them.
//
// Returns:
//   - *Config holding the built-in defaults
func defaultConfig() *Config {
	return &Config{
		Output: OutputText,
		Pace:   device.DefaultPace,
		Macros: defaultMacros(),
	}
}

// defaultConfigPath returns the config file location used when --config is not
// given: the user config directory plus radiocli/config.json.
//
// Returns:
//   - string path of the default config file
//   - error if the user config directory cannot be located
func defaultConfigPath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config directory: %w", err)
	}
	return filepath.Join(dir, "radiocli", "config.json"), nil
}

// defaultMacros are the macros a config file with none of its own gets.
//
// A list that starts empty asks somebody to invent the idea of a macro before
// they have seen one work. These are the shortest useful demonstration of what
// a macro is: each is one command anybody could type, each does something to
// the radio the moment it is pressed, and between them they cover the way out
// of wherever the radio has got to, the settings people reach for most, the one
// repair a scanner needs regularly, and the one thing worth listening to that
// is not in anybody's favorites.
//
// They are ordinary macros in every other way. Editing one changes it, deleting
// one removes it, and neither comes back: see the Macros field for what stops
// them returning.
//
// Returns:
//   - []Macro holding the built-in macros, in the order they are offered in
func defaultMacros() []Macro {
	return []Macro{
		// First, because it is the one to reach for when the radio is not
		// doing what it should be. It takes the scanner back to scanning from
		// anywhere: out of a menu, off a held frequency or channel, and off
		// the weather channels.
		{Name: "Resume Scanning", Steps: []string{"scan"}},
		{Name: "Color Mode", Steps: []string{"display mode color"}},
		{Name: "Dark Mode", Steps: []string{"display mode black"}},
		{Name: "Light Mode", Steps: []string{"display mode white"}},
		// The keypad's light, which is a setting rather than a switch and so
		// has no natural "on" button: one that flips it is the useful shape.
		// The screen's own light is left alone.
		{Name: "Toggle Backlight", Steps: []string{"backlight keys toggle"}},
		{Name: "Mute Speaker", Steps: []string{"volume set 0"}},
		// Beside the speaker button because both are about silence, and this
		// is the other half of it: the keypad's own chirp is a separate sound
		// from what is being received.
		//
		// It toggles rather than setting, because the setting is a loudness
		// rather than a switch and there is nothing on the radio that
		// remembers the level once it has been turned off. The command writes
		// it down instead, which is what lets one button both silence the
		// keypad and put it back the way it was.
		{Name: "Toggle Key Beep", Steps: []string{"beep toggle"}},
		// The only one here that changes what the radio is listening to rather
		// than how it looks or sounds, and the only one that takes more than a
		// moment: it measures all seven weather channels before parking on the
		// best. There is no matching "off" button because there is nothing to
		// put back: "weather stop" and "scan" both return the scanner to
		// whatever it was scanning before.
		{Name: "Monitor Weather", Steps: []string{"weather"}},
		// Beside the weather button because it is the same kind of thing: it
		// changes what the radio is listening to rather than how it looks or
		// sounds, and "scan" is what undoes either of them.
		//
		// A broadcast station is an odd thing for a scanner to sit on, which is
		// the point of having it as a button: it is a known signal in a band the
		// receiver covers, so it answers "is this thing working and is the
		// audio reaching me" in one press, without needing anything to be
		// happening on a police channel at the time.
		{Name: "Tune to 107.9 FM", Steps: []string{"tune 107.9"}},
		// The scanner keeps its own clock and it drifts, so this is the one
		// button here that is a repair rather than a preference.
		{Name: "Sync Clock", Steps: []string{"clock sync"}},
		// The colors are not in the remote protocol, so the only way to know
		// what the radio is drawing with is to walk the menus that set them.
		// That is slow, so it is done once and remembered, which leaves a page
		// showing the last colors read rather than the current ones after
		// somebody changes a color on the radio itself. This is the button that
		// puts that right, and the reason it is a button rather than something
		// automatic is that it takes a few minutes and stops the scan.
		//
		// Every layout rather than the one on screen, because the layouts you
		// are not looking at are exactly the ones this has to have read: a page
		// draws whatever the scanner switches to, and a layout nobody read is
		// drawn white on black while the radio is in color. Reading only the
		// current one left the other six that way until somebody thought to
		// press this while looking at each of them in turn.
		{Name: "Sync Colors", Steps: []string{"colors --all"}},
		// Last, because it is the only one that throws anything away. It puts
		// every screen layout back to the colors the radio left the factory
		// with, which is the way out of a screen customized into
		// unreadability, and the only button here whose effect cannot be
		// undone by pressing another.
		{Name: "Reset Colors", Steps: []string{"colors reset"}},
	}
}
