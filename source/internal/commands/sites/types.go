// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package sites

// The menu entries these walk through, matched by name like every other step.
const (
	// deleteFreqEnt is the entry on a frequency's own menu that removes it.
	deleteFreqEnt = "Delete Frequency"

	// deleteSite is the entry on a site's own menu that removes it, along with
	// every frequency the site holds.
	deleteSite = "Delete Site"

	// editName is the entry on a site's menu that opens the text entry screen.
	editName = "Edit Name"

	// newFrequency is the entry on a site's frequency list that opens the
	// entry screen, where the frequency is typed.
	newFrequency = "New Frequency"

	// newSite is the entry on a system's site list that creates a site the
	// moment it is pressed, under a name of the scanner's own choosing.
	newSite = "New Site"
)
