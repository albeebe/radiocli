// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/albeebe/radiocli/internal/buildinfo"
)

// assetFor names the release file this machine installs, and what to take out
// of it.
//
// It takes the platform rather than reading it from the runtime so that every
// platform's answer can be checked on any one of them, which is the same reason
// the card search in the backup command takes one.
//
// This runs before anything reaches the network. A machine with no build
// published for it should be told so at once, rather than after waiting for a
// release lookup that was never going to help.
//
// Parameters:
//   - goos: the operating system, as Go names it
//   - goarch: the processor architecture, as Go names it
//
// Returns:
//   - the asset to install, and an error naming what is published when this
//     platform has none
func assetFor(goos, goarch string) (asset, error) {
	platform := goos + "/" + goarch

	switch platform {
	// One universal file covers both Apple Silicon and Intel Macs.
	case "darwin/amd64", "darwin/arm64":
		return asset{archive: "radiocli-mac.zip", binary: "radiocli", platform: platform}, nil
	case "linux/amd64":
		return asset{archive: "radiocli-linux.tar.gz", binary: "radiocli", platform: platform}, nil
	case "linux/arm64":
		return asset{archive: "radiocli-linux-arm64.tar.gz", binary: "radiocli", platform: platform}, nil
	case "windows/amd64":
		return asset{archive: "radiocli-windows.zip", binary: "radiocli.exe", platform: platform}, nil
	}

	return asset{}, fmt.Errorf("no release is built for %s: the releases cover macOS, "+
		"Linux on amd64 and arm64, and Windows on amd64\n\n"+
		"Build it from source instead:\n\n"+
		"  git clone https://github.com/%s\n"+
		"  cd radiocli/source && go build -o radiocli .", platform, repoPath)
}

// drain reads and discards what is left of a response body, so the connection
// underneath can be reused rather than thrown away.
//
// It is capped: a body nobody is going to read is not a body worth reading
// without a limit.
//
// Parameters:
//   - body: the response body, which the caller still closes
func drain(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxErrorBytes))
}

// fetchRelease asks GitHub about a release.
//
// With no tag it asks for the newest one, through an endpoint that skips
// prereleases and drafts. That is why --version is the only way to install a
// release candidate, and why the newest release reported here can be older than
// the newest tag on the repository.
//
// Parameters:
//   - ctx: cancels the request
//   - tag: the release to ask about, or empty for the newest
//
// Returns:
//   - the release, and an error naming what to do when GitHub refused or has
//     nothing under that tag
//
// Errors:
//   - errRateLimited when GitHub is refusing because of the request limit
func fetchRelease(ctx context.Context, tag string) (release, error) {
	tag = normalizeTag(tag)

	endpoint := apiBaseURL + "/repos/" + repoPath + "/releases/latest"
	if tag != "" {
		endpoint = apiBaseURL + "/repos/" + repoPath + "/releases/tags/" + url.PathEscape(tag)
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := get(ctx, endpoint)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		drain(resp.Body)
		if tag == "" {
			return release{}, fmt.Errorf("this project has published no releases yet: "+
				"see https://github.com/%s/releases", repoPath)
		}
		return release{}, fmt.Errorf("no release is tagged %q: "+
			"see https://github.com/%s/releases for the tags that exist", tag, repoPath)
	}
	if resp.StatusCode != http.StatusOK {
		return release{}, statusError(resp, endpoint)
	}

	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIBytes)).Decode(&rel); err != nil {
		return release{}, fmt.Errorf("reading what GitHub said about the release: %w", err)
	}
	if rel.TagName == "" {
		return release{}, fmt.Errorf("GitHub described a release with no tag, which this "+
			"cannot act on: see https://github.com/%s/releases", repoPath)
	}
	return rel, nil
}

// findAsset picks one published file out of a release by name.
//
// Parameters:
//   - rel: the release to look in
//   - name: the file name to find, matched exactly
//
// Returns:
//   - the asset, and an error naming the release and the file when it holds no
//     such thing
func findAsset(rel release, name string) (releaseAsset, error) {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, nil
		}
	}

	return releaseAsset{}, fmt.Errorf("release %s publishes no %s: it may still be "+
		"uploading, or that part of the build failed\n\n"+
		"See https://github.com/%s/releases/tag/%s", rel.TagName, name, repoPath, rel.TagName)
}

// get makes one request, carrying the headers GitHub expects.
//
// The User-Agent is required: GitHub refuses a request without one. Naming the
// version in it also means update traffic can be told apart in a log.
//
// Parameters:
//   - ctx: cancels the request and carries its deadline
//   - target: what to fetch
//
// Returns:
//   - the response, whose body the caller closes, and an error when the request
//     could not be made at all
func get(ctx context.Context, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request for %s: %w", target, err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", "radiocli/"+buildinfo.Version)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking GitHub for %s: %w", target, err)
	}
	return resp, nil
}

// normalizeTag turns what somebody typed into --version into the tag GitHub
// knows.
//
// The releases are tagged with a leading v, and typing the version without one
// is the obvious mistake to make. Anything that is not plainly a version is
// passed through untouched, so an unusual tag still reaches GitHub as it was
// typed and fails naming the exact string that was asked for.
//
// Parameters:
//   - tag: what was typed, which may be empty
//
// Returns:
//   - the tag to ask for, or empty to ask for the newest release
func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if tag[0] >= '0' && tag[0] <= '9' {
		return "v" + tag
	}
	return tag
}

// resetAt says in words when a spent request limit lifts.
//
// Parameters:
//   - resp: the refused response, whose headers carry the reset time as a Unix
//     timestamp
//
// Returns:
//   - a phrase such as "at 4:05pm", or a vaguer one when the header was missing
//     or unreadable
func resetAt(resp *http.Response) string {
	seconds, err := strconv.ParseInt(resp.Header.Get("x-ratelimit-reset"), 10, 64)
	if err != nil {
		return "in a few minutes"
	}
	return "at " + time.Unix(seconds, 0).Local().Format("3:04pm")
}

// statusError explains a response GitHub refused to answer properly.
//
// The rate limit is the one worth telling apart, because it is temporary and
// because the response says exactly when it lifts. Sixty requests an hour is
// the allowance without a token, and it is counted per computer rather than per
// person, so an office behind one address shares it.
//
// Parameters:
//   - resp: the response, whose body is drained but not closed
//   - target: what was being asked for, for the message
//
// Returns:
//   - an error describing the refusal and what to do about it
//
// Errors:
//   - errRateLimited when the response says the request limit is spent
func statusError(resp *http.Response, target string) error {
	drain(resp.Body)

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return fmt.Errorf("%w: it will accept requests again %s", errRateLimited, resetAt(resp))
		}
		return fmt.Errorf("GitHub refused the request (%d): try again in a few minutes",
			resp.StatusCode)
	}
	return fmt.Errorf("GitHub answered %q asking for %s", resp.Status, target)
}
