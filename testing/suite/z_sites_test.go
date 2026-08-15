// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package suite

// This file runs late for the same reason as the channels tests: everything in
// here creates entries on the scanner, which is the slow end of the suite. See
// harness_test.go.

import (
	"strings"
	"testing"
)

// The names this file's entries are created under. They differ from every other
// name the suite uses, because a name is how everything is looked up and two
// entries sharing one make the lookup ambiguous, which the tool refuses.
const (
	ownTrunkedSystemName = testName + " OWN TRUNK"
	ownSiteName          = testName + " OWN SITE"
	ownTalkgroupDept     = testName + " OWN TG DEPT"
	ownTalkgroupName     = testName + " OWN TG"
)

// The frequencies a test site is built with. They are in the 800 MHz band a
// trunked site really uses, and one of them is on a 6.25 kHz boundary so that
// a value a float cannot hold exactly is carried through the whole round trip.
const (
	siteFrequencyOne = "851.050"
	siteFrequencyTwo = "852.3625"
)

// site is one row of the sites listing.
type site struct {
	Name    string `json:"name"`
	Index   string `json:"index"`
	Avoided bool   `json:"avoided"`
}

// siteFrequency is one row of the site frequencies listing.
type siteFrequency struct {
	Frequency string `json:"frequency"`
	Index     string `json:"index"`
}

// readSites reads the sites of one system.
func readSites(t *testing.T, args ...string) []site {
	t.Helper()

	var sites []site
	mustJSON(t, &sites, append([]string{"sites"}, args...)...)
	return sites
}

// readSiteFrequencies reads the frequencies of one site.
func readSiteFrequencies(t *testing.T, args ...string) []siteFrequency {
	t.Helper()

	var found []siteFrequency
	mustJSON(t, &found, append([]string{"sites", "frequencies"}, args...)...)
	return found
}

// trunkedSystem creates a P25 trunked system inside a list the test owns.
//
// It has to be trunked rather than conventional, because a conventional system
// has no sites at all and none of this applies to one.
func trunkedSystem(t *testing.T, list string) string {
	t.Helper()

	mustRun(t, "systems", "new", list, ownTrunkedSystemName, "--type", "P25 Trunk")

	if !hasSystem(readSystems(t, list), ownTrunkedSystemName) {
		t.Fatalf("the trunked system %q was not created in %q", ownTrunkedSystemName, list)
	}
	return ownTrunkedSystemName
}

// holdsFrequency reports whether a site holds a frequency, comparing the
// numbers rather than the text: the scanner writes back " 851.050000MHz" for a
// frequency given as "851.050".
func holdsFrequency(found []siteFrequency, want string) bool {
	for _, f := range found {
		got := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(f.Frequency), "MHz"))
		if got == "" {
			continue
		}
		if equalFrequency(got, want) {
			return true
		}
	}
	return false
}

// equalFrequency compares two frequencies written to different precision.
func equalFrequency(a, b string) bool {
	return strings.TrimRight(strings.TrimRight(a, "0"), ".") ==
		strings.TrimRight(strings.TrimRight(b, "0"), ".")
}

// TestSites builds a trunked site, fills it, and takes it apart again.
//
// A site is the half of a trunked system that says where the signal comes from.
// It holds the pool of frequencies the system shares out, and a system whose
// site is empty receives nothing at all, so the frequencies going in and coming
// back out is the thing worth checking.
func TestSites(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	system := trunkedSystem(t, list)

	res := mustRun(t, "sites", "new", system, ownSiteName)
	if !strings.Contains(res.stdout, ownSiteName) {
		t.Errorf("creating the site did not report its name:\n%s", res.stdout)
	}

	sites := readSites(t, system)
	if len(sites) != 1 {
		t.Fatalf("the system holds %d sites after creating one", len(sites))
	}
	if sites[0].Name != ownSiteName {
		t.Errorf("the site is %q, wanted %q", sites[0].Name, ownSiteName)
	}

	t.Run("a new site starts empty", func(t *testing.T) {
		if found := readSiteFrequencies(t, ownSiteName); len(found) != 0 {
			t.Errorf("a new site holds %d frequencies, wanted none: %+v", len(found), found)
		}
	})

	t.Run("adding frequencies", func(t *testing.T) {
		mustRun(t, "sites", "frequencies", "add", ownSiteName,
			siteFrequencyOne, siteFrequencyTwo)

		found := readSiteFrequencies(t, ownSiteName)
		if len(found) != 2 {
			t.Fatalf("the site holds %d frequencies after adding two: %+v", len(found), found)
		}
		for _, want := range []string{siteFrequencyOne, siteFrequencyTwo} {
			if !holdsFrequency(found, want) {
				t.Errorf("the site does not hold %s afterwards: %+v", want, found)
			}
		}
	})

	t.Run("one already held is skipped", func(t *testing.T) {
		// Written to different precision than it went in with, because the
		// comparison is meant to be by value rather than by text.
		mustRun(t, "sites", "frequencies", "add", ownSiteName, "851.05")

		if found := readSiteFrequencies(t, ownSiteName); len(found) != 2 {
			t.Errorf("adding a frequency the site already holds left %d of them: %+v",
				len(found), found)
		}
	})

	t.Run("a frequency that is not a frequency", func(t *testing.T) {
		mustFail(t, "not a number of megahertz",
			"sites", "frequencies", "add", ownSiteName, "abc")

		// Refused before the scanner was touched, so nothing changed.
		if found := readSiteFrequencies(t, ownSiteName); len(found) != 2 {
			t.Errorf("a refused frequency changed the site: %+v", found)
		}
	})

	t.Run("renaming the site keeps its frequencies", func(t *testing.T) {
		renamed := ownSiteName + " 2"
		mustRun(t, "sites", "rename", ownSiteName, renamed)

		found := readSiteFrequencies(t, renamed)
		if len(found) != 2 {
			t.Errorf("renaming the site left it with %d frequencies, wanted 2: %+v",
				len(found), found)
		}

		mustRun(t, "sites", "rename", renamed, ownSiteName)
	})

	t.Run("removing one frequency", func(t *testing.T) {
		mustFail(t, "pass --yes",
			"sites", "frequencies", "delete", ownSiteName, siteFrequencyTwo)

		mustRun(t, "sites", "frequencies", "delete", ownSiteName, siteFrequencyTwo, "--yes")

		found := readSiteFrequencies(t, ownSiteName)
		if len(found) != 1 {
			t.Fatalf("the site holds %d frequencies after removing one: %+v", len(found), found)
		}
		if holdsFrequency(found, siteFrequencyTwo) {
			t.Errorf("%s is still in the site after removing it", siteFrequencyTwo)
		}
		if !holdsFrequency(found, siteFrequencyOne) {
			t.Errorf("removing %s took %s with it", siteFrequencyTwo, siteFrequencyOne)
		}
	})

	t.Run("a frequency the site does not hold", func(t *testing.T) {
		mustFail(t, "the site does not hold",
			"sites", "frequencies", "delete", ownSiteName, "999.999", "--yes")
	})

	t.Run("deleting the site", func(t *testing.T) {
		mustFail(t, "pass --yes", "sites", "delete", ownSiteName)

		mustRun(t, "sites", "delete", ownSiteName, "--yes")
		if sites := readSites(t, system); len(sites) != 0 {
			t.Errorf("the system holds %d sites after deleting the only one: %+v",
				len(sites), sites)
		}
	})
}

// TestSites_OnConventionalSystem checks that a conventional system reports no
// sites rather than failing.
//
// Only a trunked system has any. A conventional one holds its frequencies in
// departments, and reporting that as an error would make "sites" unusable for
// asking which kind a system is.
func TestSites_OnConventionalSystem(t *testing.T) {
	needWrites(t)

	own := scratch(t)

	if sites := readSites(t, own.system); len(sites) != 0 {
		t.Errorf("a conventional system reports %d sites: %+v", len(sites), sites)
	}

	t.Run("the text output says so", func(t *testing.T) {
		res := mustRun(t, "sites", own.system)
		if !strings.Contains(res.stderr, "no sites") {
			t.Errorf("a conventional system did not explain itself:\n%s", res.stderr)
		}
		if strings.TrimSpace(res.stdout) != "" {
			t.Errorf("a conventional system wrote a table:\n%s", res.stdout)
		}
	})

	t.Run("creating one is refused", func(t *testing.T) {
		mustFail(t, "Edit Site", "sites", "new", own.system, "NOWHERE")
	})
}

// TestChannelsNew_Talkgroup creates a talkgroup channel on a trunked system, and
// checks that the wrong kind of address is refused on both sorts of department.
//
// A talkgroup and a frequency go in the same argument because they are the same
// thing: what the channel receives. The refusals are the reason that is safe. A
// talkgroup typed into a frequency screen would be taken as a channel on 9051
// MHz, which the scanner accepts without complaint and which receives nothing,
// so a mismatch has to be caught before anything is created rather than found
// later by wondering why a channel is silent.
func TestChannelsNew_Talkgroup(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	system := trunkedSystem(t, list)

	// A trunked system needs a site before it will do anything, but not before
	// its departments will hold talkgroups, which is all this checks.
	mustRun(t, "departments", "new", system, ownTalkgroupDept)

	res := mustRun(t, "channels", "new", ownTalkgroupDept, "TGID:9051", ownTalkgroupName)
	if !strings.Contains(res.stdout, ownTalkgroupName) {
		t.Errorf("creating the talkgroup did not report its name:\n%s", res.stdout)
	}

	found := readChannels(t, ownTalkgroupDept)
	if len(found) != 1 {
		t.Fatalf("the department holds %d channels after creating one: %+v", len(found), found)
	}
	if found[0].Talkgroup != "9051" {
		t.Errorf("the channel carries talkgroup %q, wanted 9051", found[0].Talkgroup)
	}
	if found[0].Frequency != "" {
		t.Errorf("a talkgroup channel also reported the frequency %q", found[0].Frequency)
	}

	t.Run("the prefix is matched whatever its case", func(t *testing.T) {
		mustRun(t, "channels", "new", ownTalkgroupDept, "tgid:9053", ownTalkgroupName+" 2")

		found := readChannels(t, ownTalkgroupDept)
		if len(found) != 2 {
			t.Fatalf("the department holds %d channels after creating two", len(found))
		}
		if found[1].Talkgroup != "9053" {
			t.Errorf("the channel carries talkgroup %q, wanted 9053", found[1].Talkgroup)
		}
	})

	t.Run("a frequency on a trunked department", func(t *testing.T) {
		mustFail(t, "takes a talkgroup, not a frequency",
			"channels", "new", ownTalkgroupDept, "153.980", "WRONG")

		// Refused before anything was created, which is the whole point.
		if after := readChannels(t, ownTalkgroupDept); len(after) != 2 {
			t.Errorf("a refused frequency created something: %+v", after)
		}
	})

	t.Run("a talkgroup on a conventional department", func(t *testing.T) {
		own := scratch(t)

		before := readChannels(t, own.department)
		mustFail(t, "takes a frequency, not a talkgroup",
			"channels", "new", own.department, "TGID:9051", "WRONG")

		if after := readChannels(t, own.department); len(after) != len(before) {
			t.Errorf("a refused talkgroup created something: %+v", after)
		}
	})

	t.Run("a talkgroup with nothing after the prefix", func(t *testing.T) {
		mustFail(t, "no talkgroup was given",
			"channels", "new", ownTalkgroupDept, "TGID:", "WRONG")
	})
}
