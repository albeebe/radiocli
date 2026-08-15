// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// describe names the radio a card came from, for a message or a folder name.
//
// Returns:
//   - string naming the model and the serial, the model alone when the serial
//     was not read, and "an unidentified scanner" when neither was
func (c card) describe() string {
	if c.Model == "" {
		return "an unidentified scanner"
	}
	if c.Serial == "" {
		return c.Model
	}
	return c.Model + " " + c.Serial
}

// dir is the path to the scanner's directory on the card.
//
// Returns:
//   - string holding the path to the scanner's directory inside the volume
func (c card) dir() string {
	return filepath.Join(c.Root, cardDir)
}

// findCard locates a mounted scanner card.
//
// It identifies a card by its contents rather than by its volume name, because
// the card is formatted without one: it mounts as "NO NAME" on macOS and as
// whatever the platform invents elsewhere. The scanner's directory is the only
// reliable marker.
//
// Returns:
//   - card describing the first card found, and which radio wrote it
//   - error if no scanner card is mounted
//
// Errors:
//   - errNoCard: if nothing under this platform's mount points holds the
//     scanner's directory
func findCard() (card, error) {
	for _, dir := range mountDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A directory that cannot be listed is not an error worth
			// reporting: it is far more likely to be a permissions quirk of
			// somewhere we guessed at than the card the user meant.
			continue
		}
		for _, e := range entries {
			root := filepath.Join(dir, e.Name())
			if c, ok := readCard(root); ok {
				return c, nil
			}
		}
	}
	return card{}, errNoCard
}

// openCard identifies the card at an explicit path.
//
// The path may be the volume or the scanner's directory inside it. Accepting
// both means a user who tab-completed one directory too far still gets what
// they meant.
//
// Parameters:
//   - path: the card's volume, or the scanner's directory inside it
//
// Returns:
//   - card describing what is at path, and which radio wrote it
//   - error if path holds no scanner card
func openCard(path string) (card, error) {
	if c, ok := readCard(path); ok {
		return c, nil
	}
	if filepath.Base(path) == cardDir {
		if c, ok := readCard(filepath.Dir(path)); ok {
			return c, nil
		}
	}
	return card{}, fmt.Errorf("%s does not hold a scanner card: expected a %s directory inside it",
		path, cardDir)
}

// readCard reports whether root is a scanner card, and identifies it.
//
// A card whose identity file is missing or unreadable is still a card, so this
// returns true with whatever it could read rather than rejecting it. Refusing
// to back up a damaged card would withhold the one thing that helps.
//
// Parameters:
//   - root: path to the volume the scanner's directory would sit at the root of
//
// Returns:
//   - card holding root, and whatever the identity file named, which is left
//     empty when the file is missing or unreadable
//   - bool reporting whether root is a scanner card at all
func readCard(root string) (card, bool) {
	dir := filepath.Join(root, cardDir)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return card{}, false
	}

	c := card{Root: root}
	data, err := os.ReadFile(filepath.Join(dir, infoFile))
	if err != nil {
		return c, true
	}

	// The file is tab separated, one record per line, and the record naming
	// the radio starts with "Scanner". Its remaining fields are positional.
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) < 4 || fields[0] != "Scanner" {
			continue
		}
		c.Model = strings.TrimSpace(fields[1])
		c.Serial = strings.TrimSpace(fields[2])
		c.Firmware = strings.TrimSpace(fields[3])
		break
	}
	return c, true
}

// volumeDirs lists the directories this platform mounts removable media under.
//
// Windows is absent because it mounts volumes as drive letters rather than
// inside a directory, and openCard handles an explicit path on every platform,
// so a Windows user passes --source instead.
//
// Returns:
//   - []string naming the directories to look for a mounted card in, and nil
//     on a platform this does not search
func volumeDirs() []string {
	return volumeDirsFor(runtime.GOOS)
}

// volumeDirsFor returns the directories to search on the named operating
// system. It takes the name rather than reading runtime.GOOS so every
// platform's answer can be checked on any one of them.
//
// Parameters:
//   - goos: the operating system to answer for, as runtime.GOOS names it
//
// Returns:
//   - []string naming the directories to look for a mounted card in, and nil
//     on a platform this does not search
func volumeDirsFor(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/Volumes"}
	case "linux":
		dirs := []string{"/media", "/run/media", "/mnt"}
		// Most desktop Linux mounts under a per-user directory, so add one
		// level down from the two that are usually organized that way.
		for _, base := range []string{"/media", "/run/media"} {
			if entries, err := readDir(base); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						dirs = append(dirs, filepath.Join(base, e.Name()))
					}
				}
			}
		}
		return dirs
	default:
		return nil
	}
}
