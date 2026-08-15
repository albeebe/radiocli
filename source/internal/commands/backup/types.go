// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package backup

import (
	"errors"
	"os"
	"path/filepath"
)

// cardDir is the directory the scanner keeps everything in, at the root of the
// card.
//
// The name is the BCD436HP/BCD536HP's, not this radio's. The SDS series
// inherited the card layout along with the command protocol and kept the old
// name, so looking for "SDS" anything finds nothing.
const cardDir = "BCDx36HP"

// databaseDir holds the downloaded radio database, which is the bulk of the
// card and the only part that can be restored from somewhere else.
const databaseDir = "HPDB"

// infoFile identifies the card and says which radio wrote it.
const infoFile = "scanner.inf"

// absPath resolves a path against the working directory. It is a var so tests
// can drive the failure a working directory that cannot be read would cause.
var absPath = filepath.Abs

// createFile creates the file a copy is written into. It is a var so tests can
// substitute a fake.
var createFile = os.Create

// errNoCard is returned when no scanner card is mounted.
//
// This is the failure people will hit most, and almost always for the same
// reason: the radio was allowed to boot normally. It offers the fix rather
// than only reporting the symptom.
var errNoCard = errors.New(
	"no scanner card is mounted\n\n" +
		"The scanner shows a USB prompt for about 15 seconds when it starts, and\n" +
		"boots into scanning if nothing is chosen. To reach the card:\n\n" +
		"  1. Turn the scanner off, then on again with the cable connected.\n" +
		"  2. When it asks, press \"E\" for Mass Storage.\n\n" +
		"The serial port is unavailable while the card is mounted, so no other\n" +
		"radiocli command will reach the scanner until you restart it.")

// mountDirs lists the directories a mounted card is looked for under. It is a
// var so tests can substitute a temporary directory.
var mountDirs = volumeDirs

// readDir lists a directory's entries. It is a var so tests can substitute a
// fake for the mount points this platform is searched at.
var readDir = os.ReadDir

// relPath expresses a path relative to a base. It is a var so tests can drive
// the failure a path outside the directory being walked would cause.
var relPath = filepath.Rel

// card is a scanner's memory card, found and identified.
type card struct {
	// Root is the path to the volume, not to the scanner's directory. The
	// backup copies the scanner's directory, but the volume is what a person
	// recognizes and what they will eject afterwards.
	Root string `json:"root"`

	// Model, Serial and Firmware come from the card's own identity file. They
	// are reported so a backup can be told apart from another radio's, and so
	// restoring the wrong card is a mistake somebody can notice.
	Model    string `json:"model"`
	Serial   string `json:"serial"`
	Firmware string `json:"firmware"`
}

// file is one file to copy, as a path relative to the scanner's directory.
type file struct {
	// rel is where the file sits inside the scanner's directory, and is the
	// same path it is written to inside the backup folder.
	rel string

	// size is the file's size in bytes, read while the card was walked rather
	// than while it is copied.
	size int64
}

// options holds the flags.
type options struct {
	// source is a path to the card, given instead of searching for a mounted
	// one.
	source string

	// name is what to call the backup folder, given instead of the dated
	// default.
	name string

	// verify says to read every file back and compare it against the card.
	verify bool

	// database says to include the downloaded radio database, which is most of
	// the card.
	database bool

	// dryRun says to report what would be copied without writing anything.
	dryRun bool
}

// plan is what a backup will copy, worked out before anything is written.
//
// Walking first and copying second costs one extra pass over a directory that
// is small, and buys the ability to report the total up front, to refuse a
// destination that cannot hold it, and to say what --dry-run would have done.
type plan struct {
	// files is every file to copy, sorted by path so that two backups of an
	// unchanged card can be compared.
	files []file

	// dirs is every directory on the card, so the empty ones survive.
	//
	// Four directories on a working card hold nothing until the scanner uses
	// them: the ones it writes recordings, alerts, discovery sessions and
	// activity logs into. Copying only files would silently drop them, and a
	// card restored without them is missing the places the radio expects to
	// write to.
	dirs []string

	// bytes is what the files add up to, known before anything is written so
	// the total can be reported up front.
	bytes int64

	// skipped is how many items on the card are not ordinary files, and so
	// were put there by the host rather than the scanner.
	skipped int
}

// report is what the command renders.
type report struct {
	// Card identifies the scanner the backup came from.
	Card card `json:"card"`

	// Destination is the folder that was written, absolute. It is empty for a
	// dry run, which writes nothing.
	Destination string `json:"destination,omitempty"`

	// Files and Bytes are what was copied, or what would have been.
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`

	// Directories is how many were recreated, including the empty ones the
	// scanner writes into later.
	Directories int `json:"directories"`

	// Verified says whether every file was read back and compared.
	Verified bool `json:"verified"`

	// DatabaseIncluded says whether the downloaded radio database was copied.
	// It is most of the card's size and can be rebuilt from Sentinel, so it is
	// the one part worth knowing the answer for.
	DatabaseIncluded bool `json:"databaseIncluded"`

	// DryRun says nothing was written.
	DryRun bool `json:"dryRun"`

	// Copied lists every file, relative to the scanner's directory on the
	// card. Digests are present only when the backup was verified.
	Copied []result `json:"copied"`
}

// result is one file's outcome.
type result struct {
	// Path is the file, relative to the scanner's directory on the card.
	Path string `json:"path"`

	// Bytes is the file's size.
	Bytes int64 `json:"bytes"`

	// Digest is the SHA-256 of the file as hex, present only when the backup
	// was verified.
	Digest string `json:"digest,omitempty"`
}
