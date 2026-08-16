// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// serve starts a local stand-in for the GitHub API and points the package at
// it for the length of the test.
//
// Parameters:
//   - t: the test the server's lifetime is tied to
//   - handler: what to answer requests with
//
// Returns:
//   - the server, for a test that needs its address
func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	was := apiBaseURL
	apiBaseURL = server.URL
	t.Cleanup(func() { apiBaseURL = was })

	return server
}

// Test_assetFor tests the choice of release file for a platform.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Mac: both Mac architectures take the one universal file
//   - Linux: each Linux architecture takes its own tarball
//   - Windows: Windows takes the zip, and the binary inside it ends in .exe
//   - Unsupported: a platform with no build says what is published instead
func Test_assetFor(t *testing.T) {
	// Verify that both Mac architectures are served by the universal binary,
	// which is why there is one Mac file rather than two
	t.Run("Mac", func(t *testing.T) {
		for _, arch := range []string{"amd64", "arm64"} {
			a, err := assetFor("darwin", arch)
			if err != nil {
				t.Fatalf("expected no error for darwin/%s, got: %v", arch, err)
			}
			if a.archive != "radiocli-mac.zip" || a.binary != "radiocli" {
				t.Errorf("expected the universal mac zip for darwin/%s, got: %+v", arch, a)
			}
			if a.platform != "darwin/"+arch {
				t.Errorf("expected the platform to be recorded, got: %q", a.platform)
			}
		}
	})

	// Verify that each Linux architecture takes the tarball built for it
	t.Run("Linux", func(t *testing.T) {
		for arch, want := range map[string]string{
			"amd64": "radiocli-linux.tar.gz",
			"arm64": "radiocli-linux-arm64.tar.gz",
		} {
			a, err := assetFor("linux", arch)
			if err != nil {
				t.Fatalf("expected no error for linux/%s, got: %v", arch, err)
			}
			if a.archive != want {
				t.Errorf("expected %q for linux/%s, got: %q", want, arch, a.archive)
			}
		}
	})

	// Verify that the Windows entry names the executable with its extension,
	// since that is what has to be found inside the archive
	t.Run("Windows", func(t *testing.T) {
		a, err := assetFor("windows", "amd64")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if a.archive != "radiocli-windows.zip" || a.binary != "radiocli.exe" {
			t.Errorf("expected the windows zip and radiocli.exe, got: %+v", a)
		}
	})

	// Verify that an unsupported platform is refused with the platform named
	// and a way forward, before anything reaches the network
	t.Run("Unsupported", func(t *testing.T) {
		_, err := assetFor("freebsd", "amd64")
		if err == nil {
			t.Fatal("expected freebsd to be refused")
		}
		for _, want := range []string{"freebsd/amd64", "go build"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("expected %q in the message, got: %v", want, err)
			}
		}
	})
}

// Test_drain tests discarding a response body that is not going to be read.
//
// Coverage: 100% (1 test case covering the function's only path)
//
// Test cases:
//   - Reads: the body is consumed
func Test_drain(t *testing.T) {
	// Verify that the body is read to the end, which is what lets the
	// connection underneath be reused
	t.Run("Reads", func(t *testing.T) {
		body := strings.NewReader("something nobody wants")
		drain(body)

		if body.Len() != 0 {
			t.Errorf("expected the body to be consumed, %d bytes left", body.Len())
		}
	})
}

// Test_fetchRelease tests asking GitHub about a release.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Latest: no tag asks the endpoint that skips prereleases
//   - Tagged: a tag asks for that release by name
//   - UnknownTag: a tag that does not exist names it in the refusal
//   - NoReleases: the newest release of a project with none
//   - Refused: a status neither 200 nor 404 is reported
//   - Unreadable: a body that is not the expected shape
//   - NoTag: a release GitHub described without a tag
//   - Unreachable: the request could not be made at all
func Test_fetchRelease(t *testing.T) {
	// Verify that asking for the newest release uses the endpoint that skips
	// prereleases and drafts
	t.Run("Latest", func(t *testing.T) {
		var asked string
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			asked = r.URL.Path
			_, _ = w.Write([]byte(`{"tag_name":"v0.1.1","assets":[{"name":"a","size":1}]}`))
		})

		rel, err := fetchRelease(context.Background(), "")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if rel.TagName != "v0.1.1" || len(rel.Assets) != 1 {
			t.Errorf("expected the release to be read, got: %+v", rel)
		}
		if !strings.HasSuffix(asked, "/releases/latest") {
			t.Errorf("expected the latest endpoint, got: %q", asked)
		}
	})

	// Verify that a tag is asked for by name, and that one typed without its
	// leading v still reaches GitHub with one
	t.Run("Tagged", func(t *testing.T) {
		var asked string
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			asked = r.URL.Path
			_, _ = w.Write([]byte(`{"tag_name":"v0.1.0"}`))
		})

		rel, err := fetchRelease(context.Background(), "0.1.0")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if rel.TagName != "v0.1.0" {
			t.Errorf("expected v0.1.0, got: %q", rel.TagName)
		}
		if !strings.HasSuffix(asked, "/releases/tags/v0.1.0") {
			t.Errorf("expected the tag endpoint, got: %q", asked)
		}
	})

	// Verify that a tag nobody published names the tag that was asked for, so
	// a typo is obvious
	t.Run("UnknownTag", func(t *testing.T) {
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		_, err := fetchRelease(context.Background(), "v9.9.9")
		if err == nil || !strings.Contains(err.Error(), `"v9.9.9"`) {
			t.Errorf("expected the tag to be named, got: %v", err)
		}
	})

	// Verify the same status without a tag, which means the project has
	// published nothing at all
	t.Run("NoReleases", func(t *testing.T) {
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		_, err := fetchRelease(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "no releases yet") {
			t.Errorf("expected the no releases message, got: %v", err)
		}
	})

	// Verify that any other refusal is passed on rather than swallowed
	t.Run("Refused", func(t *testing.T) {
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		_, err := fetchRelease(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "500") {
			t.Errorf("expected the status to be reported, got: %v", err)
		}
	})

	// Verify that a body which is not a release is reported rather than read
	// as an empty one
	t.Run("Unreadable", func(t *testing.T) {
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not json"))
		})

		_, err := fetchRelease(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "reading what GitHub said") {
			t.Errorf("expected a parse failure, got: %v", err)
		}
	})

	// Verify that a release with no tag is refused, since every later step
	// names it
	t.Run("NoTag", func(t *testing.T) {
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"assets":[]}`))
		})

		_, err := fetchRelease(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "no tag") {
			t.Errorf("expected a release with no tag to be refused, got: %v", err)
		}
	})

	// Verify that a request which cannot be made at all is reported
	t.Run("Unreachable", func(t *testing.T) {
		server := serve(t, func(w http.ResponseWriter, r *http.Request) {})
		server.Close()

		if _, err := fetchRelease(context.Background(), ""); err == nil {
			t.Error("expected a closed server to fail the request")
		}
	})
}

// Test_findAsset tests picking one published file out of a release.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Found: the file is matched by name
//   - Missing: a release without it says which release and which file
func Test_findAsset(t *testing.T) {
	rel := release{
		TagName: "v0.1.1",
		Assets: []releaseAsset{
			{Name: "radiocli-linux.tar.gz", URL: "http://example.invalid/linux"},
			{Name: "radiocli-mac.zip", URL: "http://example.invalid/mac"},
		},
	}

	// Verify that the file is matched on its exact name
	t.Run("Found", func(t *testing.T) {
		a, err := findAsset(rel, "radiocli-mac.zip")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if a.URL != "http://example.invalid/mac" {
			t.Errorf("expected the mac asset, got: %+v", a)
		}
	})

	// Verify that a missing file names both the release and the file, since an
	// upload that failed looks exactly like this
	t.Run("Missing", func(t *testing.T) {
		_, err := findAsset(rel, checksumsAsset)
		if err == nil {
			t.Fatal("expected a missing asset to be refused")
		}
		for _, want := range []string{"v0.1.1", checksumsAsset} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("expected %q in the message, got: %v", want, err)
			}
		}
	})
}

// Test_get tests making one request.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Headers: the headers GitHub expects are sent
//   - BadURL: a URL that cannot be turned into a request
//   - Unreachable: a request that could not be made
func Test_get(t *testing.T) {
	// Verify that the headers GitHub requires are all present, since it refuses
	// a request with no User-Agent
	t.Run("Headers", func(t *testing.T) {
		var got http.Header
		server := serve(t, func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Clone()
		})

		resp, err := get(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer resp.Body.Close()

		if got.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("expected the GitHub Accept header, got: %q", got.Get("Accept"))
		}
		if got.Get("X-GitHub-Api-Version") != apiVersion {
			t.Errorf("expected the API version header, got: %q", got.Get("X-GitHub-Api-Version"))
		}
		if !strings.HasPrefix(got.Get("User-Agent"), "radiocli/") {
			t.Errorf("expected a radiocli User-Agent, got: %q", got.Get("User-Agent"))
		}
	})

	// Verify that a URL which cannot become a request is reported
	t.Run("BadURL", func(t *testing.T) {
		if _, err := get(context.Background(), "http://\x7f/"); err == nil {
			t.Error("expected an unusable URL to be refused")
		}
	})

	// Verify that a connection failure is reported
	t.Run("Unreachable", func(t *testing.T) {
		server := serve(t, func(w http.ResponseWriter, r *http.Request) {})
		server.Close()

		if _, err := get(context.Background(), server.URL); err == nil {
			t.Error("expected a closed server to fail the request")
		}
	})
}

// Test_normalizeTag tests turning what was typed into the tag GitHub knows.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Empty: nothing typed asks for the newest release
//   - AddsV: a version typed without its leading v gets one
//   - KeepsV: a version typed with one is left alone
//   - Unusual: anything else reaches GitHub as it was typed
func Test_normalizeTag(t *testing.T) {
	// Verify that no tag stays no tag, which is what selects the newest release
	t.Run("Empty", func(t *testing.T) {
		if got := normalizeTag("  "); got != "" {
			t.Errorf("expected nothing, got: %q", got)
		}
	})

	// Verify that the obvious mistake, leaving off the v, is corrected
	t.Run("AddsV", func(t *testing.T) {
		if got := normalizeTag("0.1.1"); got != "v0.1.1" {
			t.Errorf("expected v0.1.1, got: %q", got)
		}
	})

	// Verify that a tag already carrying its v is untouched
	t.Run("KeepsV", func(t *testing.T) {
		if got := normalizeTag("v0.1.1"); got != "v0.1.1" {
			t.Errorf("expected v0.1.1, got: %q", got)
		}
	})

	// Verify that an unusual tag is passed through, so it fails naming the
	// exact string that was asked for
	t.Run("Unusual", func(t *testing.T) {
		if got := normalizeTag("nightly"); got != "nightly" {
			t.Errorf("expected nightly, got: %q", got)
		}
	})
}

// Test_resetAt tests saying when a spent request limit lifts.
//
// Coverage: 100% (2 test cases covering both paths)
//
// Test cases:
//   - Timestamp: the header is read as a time of day
//   - Missing: no usable header falls back to a vaguer phrase
func Test_resetAt(t *testing.T) {
	// Verify that the reset header is turned into a time somebody can wait for
	t.Run("Timestamp", func(t *testing.T) {
		when := time.Now().Add(time.Hour)
		resp := &http.Response{Header: http.Header{}}
		resp.Header.Set("x-ratelimit-reset", strconv.FormatInt(when.Unix(), 10))

		want := "at " + when.Local().Format("3:04pm")
		if got := resetAt(resp); got != want {
			t.Errorf("expected %q, got: %q", want, got)
		}
	})

	// Verify that a missing header still produces something readable
	t.Run("Missing", func(t *testing.T) {
		resp := &http.Response{Header: http.Header{}}
		if got := resetAt(resp); got != "in a few minutes" {
			t.Errorf("expected the vaguer phrase, got: %q", got)
		}
	})
}

// Test_statusError tests explaining a refused response.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - RateLimited: a spent limit is named as one and says when it lifts
//   - Forbidden: a refusal that is not the limit
//   - Other: any other status is reported with what was being asked for
func Test_statusError(t *testing.T) {
	// Verify that a spent request limit is identifiable, since it is the one
	// failure worth retrying later rather than reporting as broken
	t.Run("RateLimited", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
			Body:       http.NoBody,
		}
		resp.Header.Set("x-ratelimit-remaining", "0")

		err := statusError(resp, "http://example.invalid")
		if !errors.Is(err, errRateLimited) {
			t.Errorf("expected the rate limit sentinel, got: %v", err)
		}
	})

	// Verify that a refusal which is not the limit says so plainly
	t.Run("Forbidden", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
			Body:       http.NoBody,
		}

		err := statusError(resp, "http://example.invalid")
		if errors.Is(err, errRateLimited) {
			t.Error("expected this not to be reported as a rate limit")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("expected the status in the message, got: %v", err)
		}
	})

	// Verify that any other status names what was being asked for
	t.Run("Other", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Header:     http.Header{},
			Body:       http.NoBody,
		}

		err := statusError(resp, "http://example.invalid/thing")
		if !strings.Contains(err.Error(), "http://example.invalid/thing") {
			t.Errorf("expected the URL in the message, got: %v", err)
		}
	})
}
