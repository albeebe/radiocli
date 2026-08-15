// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/9/2026

package suite

// This file runs late on purpose. Go runs tests in the order their files sort,
// and everything in here types names into the scanner one character at a time,
// which is the slowest thing the suite does. Named to sort after the quick
// tests so a run reports most of its answers early. See harness_test.go.

import (
	"strconv"
	"strings"
	"testing"
)

// channel is one row of the channels listing. A channel carries one of the two
// addresses, never both: a frequency on a conventional system, a talkgroup on a
// trunked one.
type channel struct {
	Name      string `json:"name"`
	Frequency string `json:"frequency"`
	Talkgroup string `json:"talkgroup"`
}

// frequencyOrTalkgroup reports whether a value is one of the two things a
// channel can carry.
//
// A conventional channel gives a number of megahertz, written with or without
// the unit depending on which command is reporting it. A channel on a trunked
// system gives a talkgroup, which is not a number of megahertz at all.
func frequencyOrTalkgroup(value string) bool {
	if strings.HasPrefix(value, "TGID:") {
		return true
	}

	_, err := strconv.ParseFloat(strings.TrimSuffix(value, "MHz"), 64)
	return err == nil
}

// aDepartment picks a department to read from, for the tests that only read.
// It takes whatever is on the scanner, the same way and for the same reason as
// aList.
func aDepartment(t *testing.T) department {
	t.Helper()

	departments := readDepartments(t, aSystem(t).Name)
	if len(departments) == 0 {
		t.Skip("this scanner holds no departments to read: run without -writes=false to have the suite " +
			"build its own and test against those")
	}
	return departments[0]
}

// readChannels reads the channels inside one department.
func readChannels(t *testing.T, args ...string) []channel {
	t.Helper()

	var channels []channel
	mustJSON(t, &channels, append([]string{"channels"}, args...)...)
	return channels
}

// TestChannels checks the bottom level of the scanner's memory.
//
// This is a write even though it only reads, because it is the one listing
// command that has to walk the scanner's own menus: the protocol will not
// report a channel's frequency, so the tool goes and looks at the screen. The
// scanner stops scanning while it does.
func TestChannels(t *testing.T) {
	needWrites(t)

	t.Cleanup(func() { mustRun(t, "scan") })

	// The suite's own department, so this reads the same two channels on any
	// scanner, including one with nothing else on it.
	own := scratch(t)
	parent := own.department

	channels := readChannels(t, parent)
	if len(channels) != len(own.channels) {
		t.Fatalf("the department holds %d channels, wanted the %d the suite created",
			len(channels), len(own.channels))
	}

	for i, want := range own.channels {
		if channels[i].Name != want {
			t.Errorf("channel %d is %q, wanted %q", i, channels[i].Name, want)
		}

		// Each channel reporting its own frequency is the thing a walk
		// through the menus gets wrong when it stops on the wrong row.
		if !strings.HasPrefix(channels[i].Frequency, own.frequencies[i]) {
			t.Errorf("the channel %q reports %q, wanted %q",
				want, channels[i].Frequency, own.frequencies[i])
		}
	}

	for _, c := range channels {
		if c.Name == "" {
			t.Errorf("a channel in %q was reported with no name: %+v", parent, c)
		}

		// "New Channel" is the entry that creates one. It sits at the top of
		// the scanner's own list and must never be reported as a channel.
		if strings.EqualFold(c.Name, "New Channel") {
			t.Errorf("the entry that creates a channel was listed as a channel")
		}

		// A frequency is either a frequency or a talkgroup. Both are strings,
		// and a talkgroup is not a number of megahertz at all.
		if f := c.Frequency; f != "" && !frequencyOrTalkgroup(f) {
			t.Errorf("the channel %q reports %q, which is neither a frequency nor a talkgroup",
				c.Name, f)
		}
	}

	t.Run("listing names only", func(t *testing.T) {
		named := readChannels(t, parent, "--names")

		// The quick form reads the same list without going after each
		// frequency, so it has to agree about what is there.
		if len(named) != len(channels) {
			t.Errorf("the department holds %d channels, but --names reports %d",
				len(channels), len(named))
		}
		for i := range named {
			if i < len(channels) && named[i].Name != channels[i].Name {
				t.Errorf("channel %d is %q with frequencies and %q without",
					i, channels[i].Name, named[i].Name)
			}
			if named[i].Frequency != "" {
				t.Errorf("--names reported a frequency for %q", named[i].Name)
			}
		}
	})

	t.Run("printing it as text", func(t *testing.T) {
		res := mustRun(t, "channels", parent)

		for _, heading := range []string{"NAME", "RECEIVES"} {
			if !strings.Contains(res.stdout, heading) {
				t.Errorf("the table has no %s column:\n%s", heading, res.stdout)
			}
		}
	})

	t.Run("a department that does not exist", func(t *testing.T) {
		mustFail(t, "no department is called", "channels", "NO SUCH DEPARTMENT")
	})
}

// TestChannelsNew creates channels and deletes them again, inside a favorites
// list made for the test and removed at the end.
//
// The frequency comes before the name, because that is the order the scanner
// asks in: it opens a frequency screen before the channel exists at all.
func TestChannelsNew(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	system := ownSystem(t, list)
	department := ownDepartment(t, system)

	res := mustRun(t, "channels", "new", department, "155.550", ownChannelName)
	if !strings.Contains(res.stdout, ownChannelName) {
		t.Errorf("creating the channel did not report its name:\n%s", res.stdout)
	}

	found := readChannels(t, department)
	if len(found) != 1 {
		t.Fatalf("the department holds %d channels after creating one", len(found))
	}
	if !strings.HasPrefix(found[0].Frequency, "155.55") {
		t.Errorf("the channel was created on %q, wanted 155.550 MHz", found[0].Frequency)
	}

	t.Run("deleting it again", func(t *testing.T) {
		mustFail(t, "pass --yes", "channels", "delete", department, ownChannelName)

		mustRun(t, "channels", "delete", department, ownChannelName, "--yes")
		if channels := readChannels(t, department); len(channels) != 0 {
			t.Errorf("the department still holds %d channels after deleting the only one",
				len(channels))
		}
	})

	t.Run("a channel the department does not hold", func(t *testing.T) {
		mustFail(t, "no channel called", "channels", "delete", department, "NO SUCH CHANNEL", "--yes")
	})
}

// TestChannelsRename renames a channel and checks it keeps its frequency.
//
// Keeping the frequency is the point of the command. A name can always be
// corrected by deleting the channel and creating it again, but that loses the
// frequency and every per-channel setting along with it, so a rename that
// disturbed either would be no better than the thing it replaces.
func TestChannelsRename(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	system := ownSystem(t, list)
	department := ownDepartment(t, system)

	mustRun(t, "channels", "new", department, "155.550", ownChannelName)

	// A name holding a character the knob has to be turned to, which is how the
	// names this command was written to correct went wrong in the first place.
	renamed := ownChannelName + " ("

	res := mustRun(t, "channels", "rename", department, ownChannelName, renamed)
	if !strings.Contains(res.stdout, renamed) {
		t.Errorf("renaming the channel did not report its new name:\n%s", res.stdout)
	}

	found := readChannels(t, department)
	if len(found) != 1 {
		t.Fatalf("the department holds %d channels after renaming the only one", len(found))
	}
	if found[0].Name != renamed {
		t.Errorf("the channel is %q after renaming, wanted %q", found[0].Name, renamed)
	}
	if !strings.HasPrefix(found[0].Frequency, "155.55") {
		t.Errorf("the channel reports %q after renaming, wanted the 155.550 MHz it was created on",
			found[0].Frequency)
	}

	t.Run("renaming it back", func(t *testing.T) {
		mustRun(t, "channels", "rename", department, renamed, ownChannelName)

		after := readChannels(t, department)
		if len(after) != 1 || after[0].Name != ownChannelName {
			t.Fatalf("renaming back left %+v, wanted the one channel %q", after, ownChannelName)
		}
		if !strings.HasPrefix(after[0].Frequency, "155.55") {
			t.Errorf("the channel reports %q after a second rename, wanted 155.550 MHz",
				after[0].Frequency)
		}
	})

	t.Run("the name it already has", func(t *testing.T) {
		// Answered without touching the scanner, so this checks the answer
		// rather than that it was a no-op, which nothing here can see.
		mustRun(t, "channels", "rename", department, ownChannelName, ownChannelName)

		if after := readChannels(t, department); len(after) != 1 || after[0].Name != ownChannelName {
			t.Errorf("renaming to the name it already has changed the department: %+v", after)
		}
	})

	t.Run("a channel the department does not hold", func(t *testing.T) {
		mustFail(t, "no channel called",
			"channels", "rename", department, "NO SUCH CHANNEL", "SOMETHING ELSE")
	})
}

// TestChannelsDelete_SimilarNames checks that a channel whose name is the start
// of another channel's name is still told apart from it.
//
// This is a regression test with a story. The walk that steps through a menu
// treats a row as the entry it is looking for when the row is a prefix of that
// name, because the display cuts long names short. That rule matched "TEST CH"
// against a walk looking for "TEST CH 2", so reading the second channel landed
// on the first and reported its frequency. Two channels on different
// frequencies read as though they were on the same one, and deleting either
// would have removed the wrong channel.
func TestChannelsDelete_SimilarNames(t *testing.T) {
	needWrites(t)

	list := ownList(t)
	system := ownSystem(t, list)
	department := ownDepartment(t, system)

	// Three names, each the start of the next, on frequencies far enough apart
	// that no rounding could confuse them.
	made := []struct{ name, frequency string }{
		{ownChannelName, "155.550"},
		{ownChannelName + " 2", "154.100"},
		{ownChannelName + " 3", "462.5625"},
	}
	for _, c := range made {
		mustRun(t, "channels", "new", department, c.frequency, c.name)
	}

	found := readChannels(t, department)
	if len(found) != len(made) {
		t.Fatalf("created %d channels but the department holds %d", len(made), len(found))
	}

	for i, c := range made {
		if found[i].Name != c.name {
			t.Errorf("channel %d is %q, wanted %q", i, found[i].Name, c.name)
		}

		// The frequency is the tell. Every channel reading the same one is
		// what the bug looked like.
		if !strings.HasPrefix(found[i].Frequency, c.frequency) {
			t.Errorf("the channel %q reports %q, wanted %q",
				c.name, found[i].Frequency, c.frequency)
		}
	}

	t.Run("deleting the shortest name leaves the others", func(t *testing.T) {
		mustRun(t, "channels", "delete", department, made[0].name, "--yes")

		after := readChannels(t, department)
		if len(after) != len(made)-1 {
			t.Fatalf("deleting one channel of %d left %d", len(made), len(after))
		}
		for _, c := range after {
			if c.Name == made[0].name {
				t.Errorf("the channel %q is still there after deleting it", made[0].name)
			}
		}
	})
}
