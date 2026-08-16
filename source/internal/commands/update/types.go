// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"archive/zip"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// apiVersion is the GitHub REST API version this asks for by name, so a later
// default cannot change the shape of what comes back.
const apiVersion = "2022-11-28"

// checksumsAsset is the file every release publishes listing the SHA-256 of
// each of its other assets. Nothing is installed that this file does not
// vouch for.
const checksumsAsset = "checksums.txt"

// devVersion is what buildinfo reports for a binary that was not stamped at
// link time, which is every build made from a source checkout.
const devVersion = "dev"

// maxAPIBytes caps a release description read from the API. A release with
// four assets is a couple of kilobytes, so this bounds the read without ever
// being reached in practice.
const maxAPIBytes = 1 << 20

// maxChecksumsBytes caps the checksums file. Four lines of sixty-odd characters
// is the real size; this leaves room for a release with far more assets while
// still bounding the read.
const maxChecksumsBytes = 1 << 16

// maxErrorBytes caps how much of a failed response is read before it is
// discarded. The body is drained so the connection can be reused, not because
// anything in it is wanted.
const maxErrorBytes = 4096

// oldSuffix names the file the running program is moved aside to on Windows,
// which cannot overwrite the image of a running process but can rename it.
const oldSuffix = ".old"

// repoPath is the repository releases are published to, as GitHub's API spells
// it.
const repoPath = "albeebe/radiocli"

// requestTimeout bounds the small API calls: the release lookup and the
// checksums file.
//
// The archive download deliberately does not use it. A download runs as long as
// it needs to on a slow connection, and interrupting it is what Ctrl-C is for.
const requestTimeout = 30 * time.Second

// The states a build can be in relative to the release it was compared with.
// stateUnknown covers a development build and anything else that is not a
// version two strings can be ordered by.
const (
	stateAhead     = "ahead"
	stateAvailable = "available"
	stateCurrent   = "current"
	stateUnknown   = "unknown"
)

// tempPattern names the temporary files staged beside the running program. The
// leading dot keeps one out of sight on Unix if the process is killed outright
// and the cleanup never runs.
const tempPattern = ".radiocli-update-*"

// apiBaseURL is where the GitHub API lives. It is a var so tests can point it
// at a local server.
var apiBaseURL = "https://api.github.com"

// chmodFile sets a file's permissions. It is a var so tests can drive the
// failure it would cause.
var chmodFile = os.Chmod

// createTemp creates a staging file. It is a var so tests can drive the failure
// a directory that cannot be written to, or a full disk, would cause.
var createTemp = os.CreateTemp

// errDevBuild is returned when a build that was not stamped at link time is
// asked to replace itself.
//
// Refusing is the safer default by a distance. Somebody running a build from
// their own checkout has almost certainly built it to test a change, and
// silently overwriting it with a release would throw that work away with no
// way to get it back.
var errDevBuild = errors.New(
	"this is a development build, so there is nothing to compare against a release\n\n" +
		"It was built from a source checkout rather than downloaded, and replacing it\n" +
		"would overwrite whatever it was built to test. Pass --force to install the\n" +
		"release over it anyway.")

// errNotWritable is returned when the program cannot write to the directory it
// is installed in. It is a sentinel so the message can be wrapped with the
// directory while callers still match on the cause.
var errNotWritable = errors.New("the install directory cannot be written to")

// errRateLimited is returned when GitHub refuses because this computer has made
// too many requests. It is a sentinel because it is the one API failure worth
// telling apart: it is temporary, and it says when to try again.
var errRateLimited = errors.New("github is rate limiting this computer")

// evalSymlinks resolves a path to the real file behind it. It is a var so tests
// can drive the failure a path that no longer exists would cause.
var evalSymlinks = filepath.EvalSymlinks

// executablePath reports the running program's own path. It is a var so tests
// can substitute a fake, since the real one names the test binary.
var executablePath = os.Executable

// goarch and goos are the platform the release is chosen for. They are vars,
// and every function that decides anything from them takes them as parameters,
// so that every platform's answer can be checked on any one of them. The
// Windows path through the replacement is the reason this matters: a file only
// compiled on Windows is a file whose coverage is never measured anywhere.
var (
	goarch = runtime.GOARCH
	goos   = runtime.GOOS
)

// maxArchiveBytes caps the release archive that will be downloaded. The largest
// today is the universal macOS zip at about 5 MB, so this is roomy enough to
// outlast years of growth while still refusing something that is not a release
// at all. It is a var so tests can shrink it rather than producing 64 MB to
// prove the cap works.
var maxArchiveBytes int64 = 64 << 20

// maxBinaryBytes caps what will be written out of an archive.
//
// This is the guard against a decompression bomb. A zip records its own
// uncompressed size, and that number comes from the archive rather than from
// measuring anything, so a small file can honestly claim to expand to something
// enormous. Counting the bytes on the way out is the only figure that is real.
//
// It is a var for the same reason as maxArchiveBytes.
var maxBinaryBytes int64 = 192 << 20

// httpClient makes every request.
//
// It carries no Timeout of its own on purpose. A client timeout covers the
// whole exchange including the body, which would kill a large download on a
// slow connection partway through; the deadlines that belong on the small calls
// are set with a context instead.
var httpClient = &http.Client{}

// openZip opens a zip archive. It is a var so tests can drive the failure a
// truncated or corrupt archive would cause.
var openZip = zip.OpenReader

// removeFile deletes a file. It is a var so tests can watch what the cleanup
// removes.
var removeFile = os.Remove

// renameFile moves a file into place. It is a var so tests can drive the
// failures the replacement has to recover from.
var renameFile = os.Rename

// statFile reads a file's details. It is a var so tests can drive the failure
// a file that cannot be read would cause.
var statFile = os.Stat

// asset is the release file this platform installs, and what to take out of it.
type asset struct {
	// archive is the published file name, such as "radiocli-mac.zip". It is
	// what is looked up in the release and in the checksums file, so it has to
	// match what the release workflow uploads exactly.
	archive string

	// binary is the single entry extracted from the archive. Nothing else in
	// the archive is read or written.
	binary string

	// platform is the operating system and architecture the archive was chosen
	// for, as "darwin/arm64". It is reported so a result from one machine says
	// which machine it came from.
	platform string
}

// options is what the flags set.
type options struct {
	// check reports whether a newer release exists and changes nothing.
	check bool

	// force installs past the refusals: a development build, a version already
	// installed, or a build newer than the release.
	force bool

	// version names a published release to install instead of the newest one,
	// which is how somebody goes back to an older one.
	version string
}

// release is a published release, carrying only the fields this reads.
//
// Naming the fields rather than holding the whole response means a change
// somewhere else in GitHub's schema cannot break parsing here.
type release struct {
	TagName     string         `json:"tag_name"`
	HTMLURL     string         `json:"html_url"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []releaseAsset `json:"assets"`
}

// releaseAsset is one file published with a release.
type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`

	// Digest is the SHA-256 GitHub computed when the file was uploaded, as
	// "sha256:...". It is used as a second opinion against the checksums file,
	// which catches a checksums file generated from the wrong build. It is
	// empty on older releases.
	Digest string `json:"digest"`
}

// report is what an install prints. It is a named type rather than an inline
// map so the JSON has a fixed, reviewable shape.
type report struct {
	// From and To are the version that was running and the tag that replaced
	// it.
	From string `json:"from"`
	To   string `json:"to"`

	// Path is the file that was replaced, with symlinks resolved. That
	// resolution is why it is worth printing: somebody whose install is a link
	// into a package manager's directory can see which file actually changed.
	Path string `json:"path"`

	// Asset, Bytes and Digest describe the file that was downloaded and the
	// SHA-256 it was verified against, so an install can be tied back to a
	// published file.
	Asset  string `json:"asset"`
	Bytes  int64  `json:"bytes"`
	Digest string `json:"digest"`

	// URL is the release page, and Published is when it was published, in RFC
	// 3339. Published is empty when GitHub reported none.
	URL       string `json:"url"`
	Published string `json:"published,omitempty"`
}

// status is what --check prints. It is a named type for the same reason report
// is: something polling this should be able to read the shape once.
type status struct {
	// Current is the running version, straight from buildinfo. It is "dev" for
	// a build that was not stamped at link time.
	Current string `json:"current"`

	// Latest is the release this was compared against: the newest published
	// release, or the one --version named.
	Latest string `json:"latest"`

	// UpdateAvailable is the single field something polling this needs. It is
	// true only when Latest is a version and it is newer than Current, so a
	// development build and a build ahead of the release both report false.
	UpdateAvailable bool `json:"updateAvailable"`

	// State says why UpdateAvailable is what it is: "available", "current",
	// "ahead" when this build is newer than the release, and "unknown" when
	// either side cannot be compared.
	State string `json:"state"`

	// Dev reports a build that was not stamped at link time, and so will refuse
	// to replace itself without --force.
	Dev bool `json:"dev"`

	// Pinned reports that Latest came from --version rather than from the
	// newest release, so nothing is misled into reading it as the newest.
	Pinned bool `json:"pinned"`

	// Asset and Platform are the release file this machine would install and
	// what it was chosen for.
	Asset    string `json:"asset"`
	Platform string `json:"platform"`

	// URL is the release page, for somebody who wants to read the notes.
	URL string `json:"url"`

	// Published is when the release was published, in RFC 3339, and is empty
	// when GitHub reported none.
	Published string `json:"published,omitempty"`

	// Prerelease reports a release marked as one. It can only be true when
	// --version named it: the newest release GitHub reports skips prereleases
	// and drafts.
	Prerelease bool `json:"prerelease"`
}

// version is a release tag broken into the parts that decide precedence.
type version struct {
	major, minor, patch int

	// pre is the prerelease, without its leading "-", and is empty for an
	// ordinary release. It is what makes v1.0.0-rc.1 older than v1.0.0.
	pre string
}
