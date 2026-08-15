// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package channels

// deleteChannel is the entry on a channel's own menu that removes it.
const deleteChannel = "Delete Channel"

// The menu entries this walks through, matched by name like every other step.
const (
	editChannel   = "Edit Channel"
	editFrequency = "Edit Frequency"

	// newChannel is the entry that creates one. It sits at the top of the list
	// and is not a channel, so it is skipped.
	newChannel = "New Channel"
)

// The titles of the two screens New Channel can open. Which one the scanner
// offers is decided by the system above the department, and is how this knows
// whether it is being asked for a frequency or a talkgroup.
const (
	inputFrequency = "Input Frequency"
	inputTalkgroup = "Input TGID"
)

// The menu entries creating a channel passes through.
const (
	newChannelEntry = "New Channel"
	editNameEntry   = "Edit Name"
)

// talkgroupPrefix marks the positional argument as a talkgroup rather than a
// frequency. It is the same spelling the scanner uses when it reports one.
const talkgroupPrefix = "TGID:"

// channel is one channel as this command reports it.
type channel struct {
	// Name is what the channel is called.
	Name string `json:"name"`

	// Frequency is the channel's frequency as the scanner writes it, on a
	// conventional system. It is a string because that is how the scanner holds
	// it.
	Frequency string `json:"frequency,omitempty"`

	// Talkgroup is what the channel carries instead on a trunked system, where
	// there is no frequency of its own: the system shares a pool of them and
	// hands one out per transmission.
	Talkgroup string `json:"talkgroup,omitempty"`
}
