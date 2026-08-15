// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package navigate

// The menu entries these walks pass through, matched by name rather than by
// position, so a firmware that reorders a menu still works and no walk can
// press the entry next to the one it wanted.
const (
	editChannels    = "Edit Channel"
	editDepartments = "Edit Department"
	editSites       = "Edit Site"
	manageFavorites = "Manage Favorites"
	reviewSystems   = "Review/Edit System"
	setFrequencies  = "Set Frequencies"
)
