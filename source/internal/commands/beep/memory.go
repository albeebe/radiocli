// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package beep

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// A toggle that switches the keypad off has to write down what it was, because
// switching it off is what destroys the answer: the scanner keeps one value,
// and Off is now that value. Nothing on the radio remembers the level it used
// to be on.
//
// That makes this the one thing this tool stores that it cannot get back by
// asking the scanner. Everything else kept on disk is a copy of something the
// radio still knows. This is written where a cache goes all the same, because
// losing it has a defined and harmless answer: a toggle with nothing remembered
// leaves the keypad silent, which is the state somebody who last pressed the
// button asked for.

// loadMemory reads the file.
//
// Everything that can go wrong with it is answered the same way: hand back an
// empty memory. A missing file is the normal first run, and a damaged or
// outdated one is worth no more than that, because a toggle that finds nothing
// remembered has somewhere sensible to go.
//
// Returns:
//   - the file's contents, or an empty memory when there is nothing usable to
//     read
//   - error if the file cannot be located, or exists but cannot be read
func loadMemory() (*memory, error) {
	path, err := memoryPath()
	if err != nil {
		return nil, err
	}

	empty := &memory{Version: memoryVersion, Scanners: map[string]remembered{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var m memory
	if err := json.Unmarshal(data, &m); err != nil || m.Version != memoryVersion {
		return empty, nil
	}
	if m.Scanners == nil {
		m.Scanners = map[string]remembered{}
	}
	return &m, nil
}

// lookup finds the setting remembered for one scanner.
//
// A remembered "off" is no answer at all, and is reported as nothing being
// remembered. It could only get there by being written by hand, since a toggle
// stores what it is replacing and never switches off something already off, and
// putting "off" back would make the button do nothing for ever.
//
// Parameters:
//   - key: the scanner to look up, as scannerKey spells it
//
// Returns:
//   - the setting remembered for that scanner, or the zero level when there is
//     none worth putting back
//   - true when a setting was found
func (m *memory) lookup(key string) (level, bool) {
	got, ok := m.Scanners[key]
	if !ok {
		return level{}, false
	}
	found, ok := lookup(got.Level)
	if !ok || !found.on() {
		return level{}, false
	}
	return found, true
}

// memoryPath returns the file the remembered settings are kept in.
//
// Returns:
//   - the path to the file
//   - error if the user's cache directory cannot be located
func memoryPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating user cache directory: %w", err)
	}
	return filepath.Join(dir, "radiocli", "keybeep.json"), nil
}

// recall reads back the setting remembered for one scanner.
//
// A file that cannot be read is treated as one that remembers nothing, for the
// same reason loadMemory treats a damaged one that way.
//
// Parameters:
//   - app: the application context the warning is logged through
//   - info: the scanner whose setting is wanted
//
// Returns:
//   - the setting remembered for that scanner, or the zero level when there is
//     none
//   - true when a setting was found
func recall(app *appcontext.App, info device.Info) (level, bool) {
	m, err := loadMemory()
	if err != nil {
		app.Log.Warn("could not read the remembered key beep", "reason", err)
		return level{}, false
	}
	return m.lookup(scannerKey(info))
}

// remember stores a setting and reports nothing.
//
// A memory that cannot be written is a warning rather than a failed command:
// the scanner has already been changed by the time this runs, and reporting a
// failure for a command that did what it was asked would be a lie. What it
// costs is the next toggle having nothing to put back, which is a state the
// toggle already knows how to be in.
//
// Parameters:
//   - app: the application context the warning is logged and noted through
//   - info: the scanner the setting belongs to
//   - l: the setting to write down
//   - when: the time the toggle that stored this ran
func remember(app *appcontext.App, info device.Info, l level, when time.Time) {
	m, err := loadMemory()
	if err == nil {
		m.store(scannerKey(info), info, l, when)
		err = m.save()
	}
	if err != nil {
		app.Log.Warn("could not write down the key beep setting", "level", l.value, "reason", err)
		app.Notef("Could not write down that the key beep was %s, so a later toggle will leave it off.\n",
			l.label())
	}
}

// save writes the file back.
//
// Returns:
//   - error if the file cannot be located, its directory cannot be created,
//     the contents cannot be encoded, or the write fails
func (m *memory) save() error {
	path, err := memoryPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	m.Version = memoryVersion
	data, err := marshalJSON(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the remembered key beep: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// scannerKey identifies a scanner in the file.
//
// The USB serial number is what tells two identical scanners apart, so it is
// used when there is one. A connection opened by port does not carry one, and
// then the port is the best there is: it is stable for as long as the scanner
// stays in the same socket, and a wrong guess only costs a toggle that puts
// back the other radio's level.
//
// Parameters:
//   - info: the scanner to identify
//
// Returns:
//   - the key that scanner's setting is filed under
func scannerKey(info device.Info) string {
	if info.Serial != "" {
		return "serial:" + info.Serial
	}
	return "port:" + info.Port
}

// store writes down one scanner's setting, replacing whatever was there.
//
// Parameters:
//   - key: the scanner to file the setting under, as scannerKey spells it
//   - info: the scanner the setting belongs to, recorded for a person reading
//     the file
//   - l: the setting to write down
//   - when: the time the toggle that stored this ran
func (m *memory) store(key string, info device.Info, l level, when time.Time) {
	if m.Scanners == nil {
		m.Scanners = map[string]remembered{}
	}
	m.Scanners[key] = remembered{
		Model: info.Model,
		Port:  info.Port,
		Level: l.value,
		Noted: when,
	}
}
