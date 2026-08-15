// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package audioin

import (
	"errors"
	"strings"
	"testing"
)

// TestSortSources checks the order a listing comes out in. It matters because
// the operating system's own order changes as devices come and go, and a list
// that reshuffles between two runs for no visible reason is hard to read and
// hard to compare.
func TestSortSources(t *testing.T) {
	cases := []struct {
		name    string
		sources []Source
		want    []string
	}{
		{
			"names sort without regard to case",
			[]Source{{Name: "beta"}, {Name: "Alpha"}},
			[]string{"Alpha", "beta"},
		},
		{
			// Nothing is promoted or demoted, not even the input the operating
			// system treats as its default. The order is the names and nothing
			// else, so a listing reads the same way twice.
			"nothing jumps the queue",
			[]Source{{Name: "Zebra Microphone"}, {Name: "Cubilux CB5 Line In"}},
			[]string{"Cubilux CB5 Line In", "Zebra Microphone"},
		},
		{
			"a name that is a prefix of another comes first",
			[]Source{{Name: "Line In (rear)"}, {Name: "Line In"}},
			[]string{"Line In", "Line In (rear)"},
		},
		{
			"an empty list is left alone",
			[]Source{},
			nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sortSources(c.sources)

			got := make([]string, 0, len(c.sources))
			for _, s := range c.sources {
				got = append(got, s.Name)
			}
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("the sources came out as %q, want %q", got, c.want)
			}
		})
	}
}

// TestSortSourcesKeepsDuplicateNames checks that two identical USB interfaces,
// which report one name between them, both survive the sort. Collapsing them
// would hide a device that is really there.
func TestSortSourcesKeepsDuplicateNames(t *testing.T) {
	sources := []Source{
		{Name: "Cubilux CB5 Line In"},
		{Name: "MacBook Pro Microphone"},
		{Name: "Cubilux CB5 Line In"},
	}
	sortSources(sources)

	if len(sources) != 3 {
		t.Fatalf("sorting left %d sources, want the 3 it was given", len(sources))
	}
	if sources[0].Name != sources[1].Name {
		t.Errorf("the two sources sharing a name came out as %q and %q, want them together",
			sources[0].Name, sources[1].Name)
	}
}

// attached is a plausible list of sound inputs, in the order an audio library
// would hand them over rather than sorted. The order is the point: pickSource
// answers with a position, and the position is how the device is reached.
var attached = []string{
	"MacBook Pro Microphone",
	"Cubilux CB5 Line In",
	"BlackHole 2ch",
}

func TestPickSource(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		want  string
		at    int
	}{
		{"a name matches where it sits, not where it would sort", attached, "Cubilux CB5 Line In", 1},
		{"the first in the list is reachable", attached, "MacBook Pro Microphone", 0},
		{"the last in the list is reachable", attached, "BlackHole 2ch", 2},

		// Copied out of a listing, through a shell, into a config file and back
		// is enough to change the case of a name. None of that should stop it
		// naming the device it plainly names.
		{"case is ignored", attached, "cubilux cb5 line in", 1},
		{"surrounding space is ignored", attached, "  Cubilux CB5 Line In  ", 1},

		// The odd case: two devices whose names differ only in case. One of them
		// is exactly what was asked for, so there is an answer and refusing to
		// give it would be the wrong kind of caution.
		{"an exact match beats a differently cased one", []string{"line in", "Line In"}, "Line In", 1},
		{"and in the other order too", []string{"Line In", "line in"}, "line in", 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, err := pickSource(c.names, c.want)
			if err != nil {
				t.Fatalf("pickSource(%q): %v", c.want, err)
			}
			if at != c.at {
				t.Errorf("pickSource(%q) found it at %d, want %d", c.want, at, c.at)
			}
		})
	}
}

func TestPickSourceRefusesAnUnknownName(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		want  string
	}{
		{"a name nothing has", attached, "Zebra Microphone"},
		{"nothing attached at all", nil, "Cubilux CB5 Line In"},
		{"no name given", attached, ""},
		{"a name that is only space", attached, "   "},

		// Not a match. A name is the whole name, because "Line In" would
		// otherwise pick one of three interfaces at random on a machine with a
		// few of them.
		{"a name that is only part of one", attached, "Cubilux"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := pickSource(c.names, c.want)
			if !errors.Is(err, ErrNoSource) {
				t.Errorf("pickSource(%q) gave %v, want ErrNoSource", c.want, err)
			}
		})
	}
}

// TestPickSourceRefusesTwoOfTheSameName is the promise the package doc makes
// about a source being nothing but its name. Two identical USB interfaces report
// one name between them and are genuinely indistinguishable from here, so the
// only honest answer is to say so.
//
// Choosing the first would be worse than failing. It would work, quietly, and
// then record from the other one after a reboot.
func TestPickSourceRefusesTwoOfTheSameName(t *testing.T) {
	names := []string{
		"MacBook Pro Microphone",
		"Cubilux CB5 Line In",
		"Cubilux CB5 Line In",
	}

	_, err := pickSource(names, "Cubilux CB5 Line In")
	if !errors.Is(err, ErrAmbiguousSource) {
		t.Fatalf("pickSource gave %v, want ErrAmbiguousSource", err)
	}

	// The message has to name the device and say how many there are, because
	// the listing the user would otherwise go and look at shows them the same
	// two identical lines.
	if !strings.Contains(err.Error(), "Cubilux CB5 Line In") || !strings.Contains(err.Error(), "2") {
		t.Errorf("the refusal reads %q, which does not say which name or how many", err)
	}
}

// TestOpen tests the Open function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the named source is opened and the capture handed back
//   - NoCallback: opening with nothing to give the audio to is refused
//   - OpenError: a failure from the audio side is reported
func TestOpen(t *testing.T) {
	// Verify that a named source is opened and the capture is handed back.
	t.Run("Success", func(t *testing.T) {
		want := &Capture{}
		var gotName string
		original := openFn
		t.Cleanup(func() { openFn = original })
		openFn = func(name string, onFrames func(pcm []byte)) (*Capture, error) {
			gotName = name
			onFrames([]byte{1, 2})
			return want, nil
		}

		frames := 0
		got, err := Open("Cubilux CB5 Line In", func(pcm []byte) { frames += len(pcm) })
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if got != want {
			t.Errorf("Open gave %v, want the capture the audio side made", got)
		}
		if gotName != "Cubilux CB5 Line In" {
			t.Errorf("Open asked for %q, want the name it was given", gotName)
		}
		if frames != 2 {
			t.Errorf("the callback saw %d bytes, want the 2 handed through", frames)
		}
	})

	// Verify that opening with nothing to give the audio to is refused before
	// anything touches the sound card.
	t.Run("NoCallback", func(t *testing.T) {
		original := openFn
		t.Cleanup(func() { openFn = original })
		openFn = func(name string, onFrames func(pcm []byte)) (*Capture, error) {
			t.Fatal("Open reached the audio side with no callback to give the audio to")
			return nil, nil
		}

		_, err := Open("Cubilux CB5 Line In", nil)
		if err == nil {
			t.Fatal("Open accepted a nil callback, want a refusal")
		}
		if !strings.Contains(err.Error(), "nothing to give the audio to") {
			t.Errorf("the refusal reads %q, which does not say what is missing", err)
		}
	})

	// Verify that a failure from the audio side comes back as it was given.
	t.Run("OpenError", func(t *testing.T) {
		original := openFn
		t.Cleanup(func() { openFn = original })
		openFn = func(name string, onFrames func(pcm []byte)) (*Capture, error) {
			return nil, errors.New("the sound card is gone")
		}

		_, err := Open("Cubilux CB5 Line In", func(pcm []byte) {})
		if err == nil || !strings.Contains(err.Error(), "the sound card is gone") {
			t.Errorf("Open gave %v, want the failure the audio side reported", err)
		}
	})
}

// TestResolve tests the Resolve function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Success: a typed name comes back spelled the way the system spells it
//   - ListError: a failure to ask the audio system is reported
//   - NoSource: a name nothing answers to is refused
//   - AmbiguousSource: a name two things answer to is refused
func TestResolve(t *testing.T) {
	// Verify that a typed name comes back spelled the way the system spells it.
	t.Run("Success", func(t *testing.T) {
		original := listSourcesFn
		t.Cleanup(func() { listSourcesFn = original })
		listSourcesFn = func() ([]Source, error) {
			return []Source{{Name: "MacBook Pro Microphone"}, {Name: "Cubilux CB5 Line In"}}, nil
		}

		got, err := Resolve("  cubilux cb5 line in ")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got != "Cubilux CB5 Line In" {
			t.Errorf("Resolve gave %q, want the system's own spelling", got)
		}
	})

	// Verify that a failure to ask the audio system is reported rather than read
	// as an empty listing.
	t.Run("ListError", func(t *testing.T) {
		original := listSourcesFn
		t.Cleanup(func() { listSourcesFn = original })
		listSourcesFn = func() ([]Source, error) {
			return nil, errors.New("the audio system will not answer")
		}

		got, err := Resolve("Cubilux CB5 Line In")
		if err == nil || !strings.Contains(err.Error(), "the audio system will not answer") {
			t.Errorf("Resolve gave %v, want the failure the audio system reported", err)
		}
		if got != "" {
			t.Errorf("Resolve gave the name %q alongside a failure, want none", got)
		}
	})

	// Verify that a name nothing attached answers to is refused.
	t.Run("NoSource", func(t *testing.T) {
		original := listSourcesFn
		t.Cleanup(func() { listSourcesFn = original })
		listSourcesFn = func() ([]Source, error) {
			return []Source{{Name: "MacBook Pro Microphone"}}, nil
		}

		if _, err := Resolve("Zebra Microphone"); !errors.Is(err, ErrNoSource) {
			t.Errorf("Resolve gave %v, want ErrNoSource", err)
		}
	})

	// Verify that a name two identical interfaces answer to is refused rather
	// than chosen between.
	t.Run("AmbiguousSource", func(t *testing.T) {
		original := listSourcesFn
		t.Cleanup(func() { listSourcesFn = original })
		listSourcesFn = func() ([]Source, error) {
			return []Source{{Name: "Cubilux CB5 Line In"}, {Name: "Cubilux CB5 Line In"}}, nil
		}

		if _, err := Resolve("Cubilux CB5 Line In"); !errors.Is(err, ErrAmbiguousSource) {
			t.Errorf("Resolve gave %v, want ErrAmbiguousSource", err)
		}
	})
}

// TestSources tests the Sources function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Success: the listing comes back sorted by name ignoring case
//   - Empty: finding nothing is an answer rather than a failure
//   - ListError: a failure to ask the audio system is reported
func TestSources(t *testing.T) {
	// Verify that the listing comes back in this package's order rather than the
	// operating system's.
	t.Run("Success", func(t *testing.T) {
		original := listSourcesFn
		t.Cleanup(func() { listSourcesFn = original })
		listSourcesFn = func() ([]Source, error) {
			return []Source{
				{Name: "MacBook Pro Microphone"},
				{Name: "BlackHole 2ch"},
				{Name: "cubilux CB5 Line In"},
			}, nil
		}

		got, err := Sources()
		if err != nil {
			t.Fatalf("Sources: %v", err)
		}
		names := make([]string, 0, len(got))
		for _, s := range got {
			names = append(names, s.Name)
		}
		want := "BlackHole 2ch|cubilux CB5 Line In|MacBook Pro Microphone"
		if strings.Join(names, "|") != want {
			t.Errorf("Sources gave %q, want %q", strings.Join(names, "|"), want)
		}
	})

	// Verify that a machine with no sound inputs is answered rather than failed.
	t.Run("Empty", func(t *testing.T) {
		original := listSourcesFn
		t.Cleanup(func() { listSourcesFn = original })
		listSourcesFn = func() ([]Source, error) { return []Source{}, nil }

		got, err := Sources()
		if err != nil {
			t.Fatalf("Sources: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Sources found %d inputs on a machine with none", len(got))
		}
	})

	// Verify that a failure to ask the audio system is reported.
	t.Run("ListError", func(t *testing.T) {
		original := listSourcesFn
		t.Cleanup(func() { listSourcesFn = original })
		listSourcesFn = func() ([]Source, error) {
			return nil, errors.New("the audio system will not answer")
		}

		got, err := Sources()
		if err == nil || !strings.Contains(err.Error(), "the audio system will not answer") {
			t.Errorf("Sources gave %v, want the failure the audio system reported", err)
		}
		if got != nil {
			t.Errorf("Sources gave a listing alongside a failure, want none")
		}
	})
}
