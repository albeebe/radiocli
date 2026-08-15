// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package catalog

// The two entries the scanner reports alongside the favorites lists someone
// created. They are scan sources rather than lists, and carry reserved
// indexes: the whole RadioReference database, and Search with Scan.
const (
	fullDatabaseIndex   = "4294967295"
	searchWithScanIndex = "4261412864"
)

// builtInNames is the scanner's own name for each built-in scan source. The
// indexes are reserved, so a refusal can name one without reading the favorites
// lists first, which matters when the read is the thing being refused.
var builtInNames = map[string]string{
	fullDatabaseIndex:   "Full Database",
	searchWithScanIndex: "Search with Scan",
}

// known is every element name a list document can be built from. A document
// carrying one of these when a different one was asked for is the scanner
// answering the wrong question, rather than answering emptily.
var known = []string{"FL", "SYS", "DEPT", "CFREQ", "TGID", "SITE", "SFREQ", "FTO", "CS_BANK"}

// Named is anything this package reports that carries a name and an index.
type Named interface {
	// named returns the entry's name and the index the scanner knows it by.
	named() (name, index string)
}

// Channel is one channel of a department: a frequency on a conventional
// system, or a talkgroup on a trunked one.
//
// The two are one type because they are one thing to anybody reading a
// department: the address of what the channel receives. Which of the two a
// department holds is decided by the system above it, and no department mixes
// them.
type Channel struct {
	// Name is what the channel is called.
	Name string `json:"name"`

	// Index is how the scanner names this channel in other requests.
	Index string `json:"index"`

	// Frequency is set on a conventional channel, as the scanner writes it.
	Frequency string `json:"frequency,omitempty"`

	// Talkgroup is set on a trunked channel, as the scanner writes it.
	Talkgroup string `json:"talkgroup,omitempty"`

	// Avoided reports whether the scanner is skipping this channel.
	Avoided bool `json:"avoided"`
}

// CustomSearchBank is one of the ten custom search banks.
//
// A bank is a range rather than a list of channels, so it carries limits where
// everything else here carries a frequency or a talkgroup. Whether it is being
// searched is not part of this: the scanner keeps that on its screen and
// nowhere the protocol will report it.
type CustomSearchBank struct {
	// Index is the bank's number, "0" to "9".
	Index string `json:"index"`

	// Name is what the bank is called. A bank that has never been named
	// carries the scanner's own default, such as "Custom 0", rather than
	// nothing.
	Name string `json:"name,omitempty"`

	// Lower and Upper are the ends of the range it sweeps, as the scanner
	// writes them.
	Lower string `json:"lower,omitempty"`
	Upper string `json:"upper,omitempty"`

	// Modulation is how it demodulates, such as "AM" or "Auto".
	Modulation string `json:"modulation,omitempty"`

	// Step is the spacing it moves in, such as "10.0 kHz" or "Auto".
	Step string `json:"step,omitempty"`
}

// Department is one department inside a system. Departments hold the channels,
// and the manual describes them as being like channel groups.
type Department struct {
	// Name is what the department is called.
	Name string `json:"name"`

	// Index is how the scanner names this department in other requests.
	Index string `json:"index"`

	// Avoided reports whether the scanner is skipping this department.
	Avoided bool `json:"avoided"`

	// QuickKey is empty when nothing is assigned. Departments carry no number
	// tag, unlike the levels above them.
	QuickKey string `json:"quickKey,omitempty"`
}

// FavoritesList is one of the scanner's favorites lists.
type FavoritesList struct {
	// Name is what the favorites list is called.
	Name string `json:"name"`

	// Index is how the scanner names this list in other requests. It is a
	// string because the built-in sources use reserved values far outside the
	// range an ordinary list occupies.
	Index string `json:"index"`

	// Monitored reports whether the list is included when the scanner scans.
	Monitored bool `json:"monitored"`

	// QuickKey and NumberTag are empty when nothing is assigned.
	QuickKey  string `json:"quickKey,omitempty"`
	NumberTag string `json:"numberTag,omitempty"`

	// BuiltIn marks a scan source built into the scanner rather than a list
	// someone created. Those cannot be edited or deleted.
	BuiltIn bool `json:"builtIn"`
}

// Site is one site of a trunked system. A site is the transmitter a system
// speaks through, and it holds the frequencies the system uses there.
//
// Sites and departments are siblings rather than one inside the other, which is
// the shape a trunked system has and a conventional one does not: the site says
// where the signal comes from, the department says who is talking on it.
type Site struct {
	// Name is what the site is called.
	Name string `json:"name"`

	// Index is how the scanner names this site in other requests, including the
	// request for the frequencies it holds.
	Index string `json:"index"`

	// Avoided reports whether the scanner is skipping this site.
	Avoided bool `json:"avoided"`

	// QuickKey is empty when nothing is assigned.
	QuickKey string `json:"quickKey,omitempty"`
}

// SiteFrequency is one frequency of a site.
//
// A site frequency carries no name and cannot be avoided, unlike everything
// else the scanner holds: it is one of the pool the trunking computer hands
// out, not a channel anybody listens to on purpose.
type SiteFrequency struct {
	// Frequency is the frequency as the scanner writes it, in megahertz.
	Frequency string `json:"frequency"`

	// Index is the slot the frequency occupies in the site.
	Index string `json:"index"`
}

// System is one system inside a favorites list.
type System struct {
	// Name is what the system is called.
	Name string `json:"name"`

	// Index is how the scanner names this system in other requests, including
	// the request for the departments it holds.
	Index string `json:"index"`

	// Kind is the scanner's own word for the system type, such as
	// "Conventional". It is passed through rather than translated, because the
	// trunked types are named after the technologies they are.
	Kind string `json:"kind"`

	// Avoided reports whether the scanner is skipping this system.
	Avoided bool `json:"avoided"`

	// QuickKey and NumberTag are empty when nothing is assigned.
	QuickKey  string `json:"quickKey,omitempty"`
	NumberTag string `json:"numberTag,omitempty"`
}
