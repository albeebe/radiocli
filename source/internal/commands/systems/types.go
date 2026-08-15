// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package systems

// deleteSystem is the entry on a system's own menu that removes it.
const deleteSystem = "Delete System"

// editName is the entry on a system's menu that opens the text entry screen.
// Matched by name, like every other step through the menus.
const editName = "Edit Name"

// The menu entries creating a system passes through.
const newSystem = "New System"

// SystemTypes are the kinds of system the scanner will create, in the order it
// offers them. Conventional is last on its list and first in most people's
// needs, which is why the type has to be given rather than defaulted.
var SystemTypes = []string{
	"P25 Trunk",
	"P25 X2-TDMA",
	"P25 One Frequency",
	"Motorola",
	"MotoTRBO Trunk",
	"DMR One Frequency",
	"NXDN Trunk",
	"NXDN One Frequency",
	"EDACS",
	"LTR",
	"Conventional",
}
