// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package colors

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// Reading a layout costs a menu walk of about half a minute, and the answer
// changes only when somebody changes it. So a reading is kept on disk and
// --cache hands it back without touching the pickers.
//
// The cache is never consulted unless it is asked for. A plain run always reads
// the scanner and always overwrites what is stored, which is what keeps a
// deliberate read the authority on what the colors are. --cache is the opposite
// promise: it will not open a menu, so a layout that has never been read is an
// error rather than a wait.

// age renders when a reading was taken, as a time with how long ago it was.
// The age is what says whether it is worth trusting, and a bare timestamp
// leaves the reader working that out.
//
// Parameters:
//   - t: when the reading was taken
//
// Returns:
//   - string holding the local time and how long ago it was, or "-" for a
//     reading with no time on it
func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}

	since := time.Since(t)
	switch {
	case since < time.Minute:
		return t.Local().Format(time.DateTime) + " (just now)"
	case since < time.Hour:
		return fmt.Sprintf("%s (%d minutes ago)", t.Local().Format(time.DateTime), int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%s (%d hours ago)", t.Local().Format(time.DateTime), int(since.Hours()))
	}
	return fmt.Sprintf("%s (%d days ago)", t.Local().Format(time.DateTime), int(since.Hours())/24)
}

// amend updates one area of a stored reading in place, so setting a color
// leaves the cache true rather than stale.
//
// It only ever changes a reading that is already there. Building one out of a
// single area would be a cache of one area claiming to be a whole layout, which
// is worse than no cache at all.
//
// Parameters:
//   - app: application context the warnings are logged through
//   - info: the scanner whose stored colors these are
//   - layoutName: the layout to change, by this command's name for it
//   - areaName: the area to change, matched without regard to case
//   - fn: what to change about that area, given the stored copy to write to
func amend(app *appcontext.App, info device.Info, layoutName, areaName string, fn func(*cachedArea)) {
	c, err := loadCache()
	if err != nil {
		app.Log.Warn("could not update the cached colors", "reason", err)
		return
	}

	key := scannerKey(info)
	got, ok := c.lookup(key, layoutName)
	if !ok {
		return
	}

	found := false
	for i := range got.Areas {
		if strings.EqualFold(got.Areas[i].Name, areaName) {
			fn(&got.Areas[i])
			found = true
		}
	}
	if !found {
		return
	}

	c.Scanners[key].Layouts[layoutName] = got

	if err := c.save(); err != nil {
		app.Log.Warn("could not update the cached colors", "reason", err)
	}
}

// areas returns a stored reading as the areas the command renders, with the
// positions left for place to fill in.
//
// Returns:
//   - []area holding the stored colors in the order they were read, every one
//     of them unplaced
func (l cachedLayout) areas() []area {
	out := make([]area, 0, len(l.Areas))
	for _, a := range l.Areas {
		out = append(out, area{
			Name:          a.Name,
			Text:          a.Text,
			Background:    a.Background,
			TextHex:       a.TextHex,
			BackgroundHex: a.BackgroundHex,
		})
	}
	return out
}

// cachePath returns the file the cache is kept in.
//
// Returns:
//   - string holding the path to the cache file, inside this user's cache
//     directory
//   - error if the operating system will not say where that directory is
func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating user cache directory: %w", err)
	}
	return filepath.Join(dir, "radiocli", "colors.json"), nil
}

// forget drops the stored readings for these layouts, so the next command that
// wants them reads the scanner instead.
//
// This is for the changes that make a whole layout wrong at once, such as
// restoring it to stock. Amending is not an option there: the scanner was not
// asked what the new colors are, and storing a guess would be worse than
// storing nothing.
//
// Like remember, every failure is a warning. A cache that could not be trimmed
// costs a re-read at worst, and the command it belongs to has already done what
// it was asked.
//
// Parameters:
//   - app: application context the warnings are logged through
//   - info: the scanner whose stored colors these are
//   - layoutNames: the layouts to drop, by this command's names for them
func forget(app *appcontext.App, info device.Info, layoutNames []string) {
	c, err := loadCache()
	if err != nil {
		app.Log.Warn("could not update the cached colors", "reason", err)
		return
	}

	s, ok := c.Scanners[scannerKey(info)]
	if !ok || s.Layouts == nil {
		return
	}

	dropped := false
	for _, name := range layoutNames {
		if _, held := s.Layouts[name]; held {
			delete(s.Layouts, name)
			dropped = true
		}
	}
	if !dropped {
		return
	}
	c.Scanners[scannerKey(info)] = s

	if err := c.save(); err != nil {
		app.Log.Warn("could not update the cached colors", "reason", err)
	}
}

// loadCache reads the cache file.
//
// Everything that can go wrong with a cache file is answered the same way: hand
// back an empty cache and let the caller read the scanner. A missing file is
// the normal first run, and a corrupt or outdated one holds nothing that is not
// also on the scanner, so neither is worth failing a command over.
//
// Returns:
//   - *cache holding what was stored, or an empty one when the file is
//     missing, unreadable as JSON or written by another version
//   - error if the cache file cannot be located or cannot be read for any
//     reason other than not being there
func loadCache() (*cache, error) {
	path, err := cachePath()
	if err != nil {
		return nil, err
	}

	empty := &cache{Version: cacheVersion, Scanners: map[string]cachedScanner{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var c cache
	if err := json.Unmarshal(data, &c); err != nil || c.Version != cacheVersion {
		return empty, nil
	}
	if c.Scanners == nil {
		c.Scanners = map[string]cachedScanner{}
	}
	return &c, nil
}

// lookup finds a stored reading for one scanner's layout.
//
// Parameters:
//   - key: the scanner, as scannerKey identifies it
//   - layoutName: the layout, by this command's name for it
//
// Returns:
//   - cachedLayout holding the stored colors, zero when there are none
//   - bool reporting whether a reading with areas in it was found, since a
//     stored layout holding no areas is a miss rather than a screen with no
//     colors on it
func (c *cache) lookup(key, layoutName string) (cachedLayout, bool) {
	got, ok := c.Scanners[key].Layouts[layoutName]
	if !ok || len(got.Areas) == 0 {
		return cachedLayout{}, false
	}
	return got, true
}

// missing is the error --cache gives when nothing has been read yet.
//
// Parameters:
//   - layoutName: the layout that has never been read, by this command's name
//     for it
//
// Returns:
//   - error naming that layout and the command that would read it
func missing(layoutName string) error {
	return fmt.Errorf("no cached colors for %s\n"+
		"Run \"radiocli colors %s\" to read them from the scanner, which caches them for next time",
		layoutName, layoutName)
}

// remember stores a fresh reading, and reports how it went for a caller that
// wants to warn rather than fail.
//
// A cache that cannot be written must not fail a command that has already got
// the answer the caller asked for, so every failure here is a warning: the next
// run pays the walk again, which is exactly what would have happened anyway.
//
// Parameters:
//   - app: application context the warnings are logged through
//   - info: the scanner these colors were read from
//   - layoutName: the layout they belong to, by this command's name for it
//   - areas: the reading, in the order the scanner listed it
//   - when: the moment the walk that produced it finished
func remember(app *appcontext.App, info device.Info, layoutName string, areas []area, when time.Time) {
	c, err := loadCache()
	if err == nil {
		c.store(scannerKey(info), info, layoutName, areas, when)
		err = c.save()
	}
	if err != nil {
		app.Log.Warn("could not cache the colors", "layout", layoutName, "reason", err)
	}
}

// save writes the cache back.
//
// The write goes to a temporary file beside the cache and is renamed over it,
// which is what makes a reader see either the old cache or the new one and
// never half of each. Two invocations can be saving at once, which the daemon
// makes ordinary rather than unlikely: it shares one scanner between commands,
// so two of them can finish a colors walk together. A rename cannot merge their
// work, and one of the two saves is lost, but a lost cache costs a re-read
// while a torn one is a file that has to be recognised as broken and thrown
// away.
//
// The temporary file is made in the same directory as the cache, because a
// rename across file systems is not one.
//
// Returns:
//   - error if the cache file cannot be located, its directory cannot be
//     created, the contents cannot be encoded or the file cannot be written;
//     nil once the whole cache is on disk
func (c *cache) save() error {
	path, err := cachePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	c.Version = cacheVersion
	data, err := marshalJSON(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the colors cache: %w", err)
	}

	// CreateTemp picks a name nobody else has and makes the file 0600, which is
	// the mode the cache wants anyway.
	temp, err := createTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	// Remove runs whatever happened. On the way out through a failure it takes
	// the half-written file with it; after a successful rename there is nothing
	// left at that name and the error is the answer to a question nobody asked.
	defer os.Remove(temp.Name())

	// Closed whether or not the write worked, and the first failure of the two
	// is the one reported: a write that failed is why the close has nothing
	// good to say.
	_, writeErr := temp.Write(append(data, '\n'))
	if err := cmp.Or(writeErr, temp.Close()); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// scannerKey identifies a scanner in the cache.
//
// The USB serial number is what tells two identical scanners apart, so it is
// used when there is one. A connection opened by port does not carry one, and
// then the port is the best there is: it is stable for as long as the scanner
// stays in the same socket, and a wrong guess only costs a re-read.
//
// Parameters:
//   - info: what the connection knows about the scanner it is open to
//
// Returns:
//   - string keying this scanner in the cache, by serial number where there is
//     one and by port otherwise
func scannerKey(info device.Info) string {
	if info.Serial != "" {
		return "serial:" + info.Serial
	}
	return "port:" + info.Port
}

// store puts a reading in, replacing whatever was there for that layout.
//
// Parameters:
//   - key: the scanner, as scannerKey identifies it
//   - info: that scanner's model and port, kept so somebody opening the file
//     can see whose colors these are
//   - layoutName: the layout the reading is of, by this command's name for it
//   - areas: the colors, whose positions are deliberately not stored
//   - when: the moment the walk that produced them finished
func (c *cache) store(key string, info device.Info, layoutName string, areas []area, when time.Time) {
	if c.Scanners == nil {
		c.Scanners = map[string]cachedScanner{}
	}

	s, ok := c.Scanners[key]
	if !ok || s.Layouts == nil {
		s.Layouts = map[string]cachedLayout{}
	}
	s.Model, s.Port = info.Model, info.Port

	stored := make([]cachedArea, 0, len(areas))
	for _, a := range areas {
		stored = append(stored, cachedArea{
			Name:          a.Name,
			Text:          a.Text,
			Background:    a.Background,
			TextHex:       a.TextHex,
			BackgroundHex: a.BackgroundHex,
		})
	}

	s.Layouts[layoutName] = cachedLayout{Read: when, Areas: stored}
	c.Scanners[key] = s
}
