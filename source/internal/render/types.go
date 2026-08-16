// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/13/2026

package render

// Unread is what a table cell shows when the value behind it was never read,
// which is a different thing from the scanner reporting nothing there.
//
// Dash is the second of those and this is the first. They have to look
// different: a department whose quick key is genuinely unassigned and one whose
// quick key nobody has ever asked about both render as an empty string, and
// printing the same mark for the two turns "unknown" into a confident "none".
const Unread = "?"

// Mutation is one change to the scanner's memory: something created, renamed or
// deleted.
//
// One shape for all of them, across every level of the memory, because a script
// driving edits should not have to learn a different object for each. The verb
// is a field rather than the shape, so a caller can log or dispatch on it
// without knowing which command produced it.
type Mutation struct {
	// Action is what happened: "created", "renamed" or "deleted".
	Action string `json:"action"`

	// Kind is what it happened to, singular and as the tool names it in its
	// messages: "channel", "department", "system", "site", "favorites list".
	Kind string `json:"kind"`

	// Name is what the entry is called now, or was called when it was deleted.
	// It is the scanner's own spelling wherever the command read it back, which
	// is what makes it worth reporting at all.
	Name string `json:"name"`

	// Was is the previous name, on a rename, and empty otherwise.
	Was string `json:"was,omitempty"`

	// In is what holds the entry, as the caller named it, and is empty for the
	// levels that have no parent worth repeating.
	In string `json:"in,omitempty"`
}
