// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

// Package update implements the "update" command, which replaces the running
// program with the newest release published for it.
//
// It is the only command that talks to anything over the network, and the only
// one that changes the tool itself rather than the scanner. Both are worth
// saying out loud, because they are the reasons it is written the way it is.
//
// Nothing is installed that a published checksum does not vouch for. A release
// without a checksums file is refused rather than installed unchecked, and
// there is deliberately no flag to skip the check: an update that falls back to
// trusting whatever arrived when the checksums are missing has no guarantee at
// all, because missing is exactly what somebody interfering would arrange. The
// two releases published before the check existed have had a checksums file
// added to them so that nothing is grandfathered.
//
// It will not ask for administrator rights on its own. An install under
// /usr/local/bin needs sudo to replace, and the command says so and stops. A
// program that can silently re-run itself as root is a program nobody can audit
// by reading the line they typed.
//
// Two things were considered and left out. The first is running the new program
// once before installing it, to prove it works: the checksum already proves the
// file is byte for byte what the build produced, and running an unverified
// program is a larger thing to add than the case it would catch. The second is
// a lock. Two updates at once would each stage their own file and the later
// rename would win, and since both files are the same release the outcome is
// the same either way.
package update

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/buildinfo"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the update command bound to app.
//
// Parameters:
//   - app: the dependency container the command reports through
//
// Returns:
//   - *cobra.Command for "update"
func New(app *appcontext.App) *cobra.Command {
	var opts options

	cmd := &cobra.Command{
		Use: "update",
		// No Annotations. This command cannot move the scanner, but marking it
		// as one that only reads would let it run through a daemon holding the
		// scanner for somebody else, and rewriting the program that daemon is
		// running from is not something to do behind its back.
		Short: "Replace this program with the newest released version",
		Long: "Update downloads the newest release of radiocli, checks it against the\n" +
			"checksum published with it, and replaces this program with it. Nothing on\n" +
			"the scanner is touched, and no scanner needs to be attached.\n\n" +
			"Pass --check to be told whether a newer release exists without changing\n" +
			"anything. Pass --version to install a particular release instead of the\n" +
			"newest one, which is how you go back to an older one.\n\n" +
			"The program is replaced where it stands, so it has to be able to write to\n" +
			"the file it is running from. Where it was installed into a directory that\n" +
			"belongs to another user, such as /usr/local/bin, that means running this\n" +
			"command with sudo. It will never do that for you.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app, opts)
		},
	}

	f := cmd.Flags()
	f.BoolVar(&opts.check, "check", false,
		"report whether a newer release exists without installing it")
	f.BoolVar(&opts.force, "force", false,
		"install even when this build is a development build, or is not older")
	f.StringVar(&opts.version, "version", "",
		"install this published release instead of the newest one")

	return cmd
}

// install downloads a release, checks it, and puts it in place.
//
// The order is chosen so that nothing is downloaded that cannot be installed:
// whether the program's directory can be written to is settled first, because
// being told to use sudo is worth knowing before waiting for several megabytes
// rather than after.
//
// Parameters:
//   - ctx: cancels the download
//   - app: the dependency container progress is written through
//   - rel: the release to install
//   - want: the release file this platform takes
//   - exe: the program to replace, with links already followed
//   - opts: the flags, for the sudo line the failure message suggests
//
// Returns:
//   - what was installed, and an error. On any error the running program is
//     left exactly as it was.
func install(ctx context.Context, app *appcontext.App, rel release, want asset, exe string, opts options) (report, error) {
	dir := filepath.Dir(exe)

	// Every staged file is registered the moment it exists and removed by this
	// one loop, so nothing has to be cleaned up by hand on the way out of any
	// of the failures below.
	var temps []string
	defer func() {
		for _, path := range temps {
			_ = removeFile(path)
		}
	}()

	if err := writable(dir); err != nil {
		return report{}, fmt.Errorf("%w\n\n%s", err, sudoHint(goos, opts))
	}

	sums, err := fetchChecksums(ctx, rel)
	if err != nil {
		return report{}, err
	}
	from, err := findAsset(rel, want.archive)
	if err != nil {
		return report{}, err
	}

	app.Notef("Downloading radiocli %s for %s.\n", rel.TagName, want.platform)
	archive, digest, size, err := download(ctx, from, dir)
	if archive != "" {
		temps = append(temps, archive)
	}
	if err != nil {
		return report{}, err
	}

	if err := verify(want, from, sums, digest); err != nil {
		return report{}, err
	}
	app.Notef("Checked it against the checksum published with the release.\n")

	staged, err := extract(archive, want, dir, modeOf(exe))
	if staged != "" {
		temps = append(temps, staged)
	}
	if err != nil {
		return report{}, err
	}

	if err := replaceOn(goos, exe, staged); err != nil {
		return report{}, err
	}

	return report{
		From:      buildinfo.Version,
		To:        rel.TagName,
		Path:      exe,
		Asset:     want.archive,
		Bytes:     size,
		Digest:    digest,
		URL:       rel.HTMLURL,
		Published: published(rel),
	}, nil
}

// published renders when a release was published, in a form something reading
// the output can parse.
//
// Parameters:
//   - rel: the release
//
// Returns:
//   - the time in RFC 3339, or empty when GitHub reported none
func published(rel release) string {
	if rel.PublishedAt.IsZero() {
		return ""
	}
	return rel.PublishedAt.UTC().Format("2006-01-02T15:04:05Z")
}

// renderReport prints what was installed.
//
// Parameters:
//   - app: the dependency container holding the output format and the streams
//     to write to
//   - r: what was installed
//
// Returns:
//   - error if the JSON could not be written
func renderReport(app *appcontext.App, r report) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	app.Printf("radiocli %s installed at %s\n", r.To, r.Path)
	app.Notef("A radiocli daemon that is already running is still the old version. " +
		"Restart it to pick this up.\n")
	return nil
}

// renderStatus prints how the running build stands against the release.
//
// Parameters:
//   - app: the dependency container holding the output format and the streams
//     to write to
//   - s: what --check found
//
// Returns:
//   - error if the JSON could not be written
func renderStatus(app *appcontext.App, s status) error {
	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, s)
	}

	switch s.State {
	case stateAvailable:
		app.Printf("radiocli %s is available. This is %s.\n", s.Latest, s.Current)
		app.Notef("Run \"radiocli update\" to install it.\n")
	case stateCurrent:
		app.Printf("radiocli %s is the newest release.\n", s.Current)
	case stateAhead:
		app.Printf("radiocli %s is newer than the newest release, %s.\n", s.Current, s.Latest)
	default:
		app.Printf("radiocli %s cannot be compared against %s.\n", s.Current, s.Latest)
		if s.Dev {
			app.Notef("This is a development build, so there is nothing to compare. "+
				"Pass --force to install %s over it.\n", s.Latest)
		}
	}
	return nil
}

// run decides what to do and, unless only asked to look, does it.
//
// Parameters:
//   - ctx: cancels the requests this makes
//   - app: the dependency container the command reports through
//   - opts: the flags
//
// Returns:
//   - error when nothing could be done, in which case the running program is
//     untouched
//
// Errors:
//   - errDevBuild when an unstamped build is asked to replace itself
func run(ctx context.Context, app *appcontext.App, opts options) error {
	want, err := assetFor(goos, goarch)
	if err != nil {
		return err
	}

	if app.InDaemon {
		return fmt.Errorf("the daemon cannot update the program it is running: " +
			"stop it, run this again, and start it back up")
	}

	rel, err := fetchRelease(ctx, opts.version)
	if err != nil {
		return err
	}
	state := stateOf(buildinfo.Version, rel.TagName)

	if opts.check {
		return renderStatus(app, status{
			Current:         buildinfo.Version,
			Latest:          rel.TagName,
			UpdateAvailable: state == stateAvailable,
			State:           state,
			Dev:             buildinfo.Version == devVersion,
			Pinned:          opts.version != "",
			Asset:           want.archive,
			Platform:        want.platform,
			URL:             rel.HTMLURL,
			Published:       published(rel),
			Prerelease:      rel.Prerelease,
		})
	}

	if !opts.force {
		switch {
		case buildinfo.Version == devVersion:
			return errDevBuild
		case state == stateCurrent:
			app.Notef("radiocli %s is already the newest release. Nothing to do.\n",
				buildinfo.Version)
			return nil
		case state == stateAhead:
			return fmt.Errorf("radiocli %s is newer than %s, the newest release: pass "+
				"--force to install it over this one, or --version to name the release "+
				"you want", buildinfo.Version, rel.TagName)
		}
	}

	exe, err := executable()
	if err != nil {
		return err
	}
	sweep(goos, exe)

	installed, err := install(ctx, app, rel, want, exe, opts)
	if err != nil {
		return err
	}
	return renderReport(app, installed)
}

// sudoHint spells out the line to type when the program's directory belongs to
// somebody else.
//
// It is rebuilt from the flags rather than repeated back from the command line,
// so the suggestion is the same every time and can be checked by a test.
//
// Parameters:
//   - goos: the operating system, as Go names it
//   - opts: the flags this run was given
//
// Returns:
//   - the advice, as a paragraph ending without a newline
func sudoHint(goos string, opts options) string {
	line := "radiocli update"
	if opts.version != "" {
		line += " --version " + opts.version
	}
	if opts.force {
		line += " --force"
	}

	if goos == "windows" {
		return "Replacing the program means writing to that folder, which needs\n" +
			"administrator rights. Open a Command Prompt or PowerShell with \"Run as\n" +
			"administrator\" and type the same command again:\n\n" +
			"  " + line
	}
	return "Replacing the program means writing to that directory, which this user\n" +
		"cannot do. Run the same command again with sudo:\n\n" +
		"  sudo " + line + "\n\n" +
		"radiocli will not do that for you."
}
