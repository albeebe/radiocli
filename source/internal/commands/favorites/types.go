// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package favorites

// deleteEntry is the entry on a favorites list's own menu that removes it. It
// sits directly after Rename, which is why nothing here counts positions.
const deleteEntry = "Delete"

// monitorSteps bounds a walk through the monitor list. It is the number of
// lists the scanner can hold plus room for the walk to notice it has wrapped.
const monitorSteps = 40

// The menu entries creating a favorites list passes through.
const newFavoritesList = "New Favorites List"

// renameEntry is the entry on a favorites list's menu that opens the text
// entry screen. Matched by name, like every other step of the walk.
const renameEntry = "Rename"

// The menu entries choosing what gets scanned.
const (
	setScanSelection = "Set Scan Selection"
	selectLists      = "Select Lists to Monitor"
	setAllOff        = "Set All Lists Off"
	setAllOn         = "Set All Lists On"
)

// The state a row of the monitor list ends with. The scanner writes the name
// and its state on one line, as "GREENDALE, ST 00000 :On".
const (
	stateOn  = ":On"
	stateOff = ":Off"
)
