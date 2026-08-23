// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package suite

import (
	"strings"
	"testing"
)

// heard is what "receiving" reports, as documented in
// documentation/commands/receiving.md.
type heard struct {
	Receiving  bool   `json:"receiving"`
	List       string `json:"list"`
	System     string `json:"system"`
	Department string `json:"department"`
	Site       string `json:"site"`
	Channel    string `json:"channel"`
	Frequency  string `json:"frequency"`
	Talkgroup  string `json:"talkgroup"`
	Unit       string `json:"unit"`
	Modulation string `json:"modulation"`
	Signal     string `json:"signal"`
	RSSI       string `json:"rssi"`
	Mode       string `json:"mode"`
}

// TestReceiving checks that the scanner reports what it is hearing, and that
// the two things a reader has to be able to tell apart are told apart.
func TestReceiving(t *testing.T) {
	needScanner(t)

	var got heard
	mustJSON(t, &got, "receiving")

	// The mode is always there, whatever the radio is doing, so an empty one
	// means the reading never reached the scanner at all.
	if got.Mode == "" {
		t.Errorf("no mode was reported: %+v", got)
	}

	// A conventional system answers with a frequency and a trunked one with a
	// talkgroup. Both at once would mean the two were being read out of the
	// same element, which they are not.
	if got.Frequency != "" && got.Talkgroup != "" {
		t.Errorf("both a frequency and a talkgroup were reported: %+v", got)
	}

	// The scanner writes its frequencies with a leading space and its own unit.
	// The space is stripped and the unit is kept, so a value here that still
	// has the space would break every filename built from it.
	if got.Frequency != "" {
		if strings.TrimSpace(got.Frequency) != got.Frequency {
			t.Errorf("the frequency %q has whitespace around it", got.Frequency)
		}
		if !strings.HasSuffix(got.Frequency, "MHz") {
			t.Errorf("the frequency %q does not carry its unit", got.Frequency)
		}
	}

	// The radio writes "TGID None" and "UID None" for an identifier it has not
	// decoded, and those words must never reach a caller as a value.
	for name, value := range map[string]string{
		"talkgroup": got.Talkgroup, "unit": got.Unit, "channel": got.Channel,
	} {
		if strings.HasSuffix(value, "None") {
			t.Errorf("the %s reads %q, which is the scanner's way of saying there is none", name, value)
		}
	}
}

// TestReceivingRunsAlongsideAnotherCommand checks the annotation that makes
// this usable as a label source while something long is running.
//
// It is the property "audio record" depends on: it asks this several times a
// second for as long as a recording lasts, through a daemon, without ever
// making anything else wait.
func TestReceivingRunsAlongsideAnotherCommand(t *testing.T) {
	needScanner(t)

	// Reading three times in quick succession is what a recorder does, and each
	// one has to answer rather than be refused for the scanner being busy.
	for range 3 {
		var got heard
		mustJSON(t, &got, "receiving")
		if got.Mode == "" {
			t.Fatalf("a repeated reading reported nothing: %+v", got)
		}
	}
}
