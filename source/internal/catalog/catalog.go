// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package catalog reads the XML documents the scanner answers list requests
// with.
//
// The device layer returns those documents as text, because their shape
// differs per list and the scanner's database can be large. This package turns
// one into its elements, and guards the mistake that shape invites: the
// scanner answers some list requests with a document of the wrong kind and
// reports no error while doing so, so asking for channels and being handed
// favorites lists looks like success unless somebody checks.
package catalog

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/albeebe/radiocli/internal/device"
)

// Element is one entry from a list document, holding its attributes by name.
// Attributes the scanner does not send are absent rather than empty, so a
// caller cannot tell them apart; nothing so far needs to.
type Element map[string]string

// ErrIncomplete reports a list the scanner cut short.
//
// The reply to a list request is capped at roughly a kilobyte. A list that does
// not fit is ended early and marked with EOT="0" in its footer, and nothing
// found asks for the rest: repeating the request answers with the same first
// part, and a page number in it is ignored. So the rows that did arrive are
// good, and there is no telling from the document how many did not.
//
// It is returned alongside those rows rather than instead of them, because a
// truncated list is useful and a missing one is not. Every function in this
// package that reads a list may return it, and a caller that ignores it reports
// a short list as though it were the whole thing. That was a real bug: forty
// channels were created, seven were listed back, and nothing anywhere said the
// other thirty-three had been left out. Use navigate to read a list that has to
// be complete, which fills in what was cut by walking the scanner's menus.
var ErrIncomplete = errors.New("the scanner cut the list short")

// BuiltInSource returns the scanner's name for the built-in scan source an
// index belongs to.
//
// The built-in indexes are reserved values, so this answers from the index
// alone and costs no exchange. That is what lets a request be refused before it
// is sent.
//
// Parameters:
//   - index: the favorites list index to examine
//
// Returns:
//   - the scanner's name for the built-in source, or "" when the index is an
//     ordinary favorites list someone created
func BuiltInSource(index string) string {
	return builtInNames[index]
}

// CustomSearchBanks reads the answer to a request for the custom search banks.
//
// Parameters:
//   - doc: the document the scanner answered a custom search bank request with
//
// Returns:
//   - the banks the document describes, empty when it describes none
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func CustomSearchBanks(doc string) ([]CustomSearchBank, error) {
	elements, err := Elements(doc, "CS_BANK")
	if !usable(err) {
		return nil, err
	}

	banks := make([]CustomSearchBank, 0, len(elements))
	for _, e := range elements {
		banks = append(banks, CustomSearchBank{
			Index:      e.Get("Index"),
			Name:       e.Get("Name"),
			Lower:      e.Get("Lower"),
			Upper:      e.Get("Upper"),
			Modulation: e.Get("Mod"),
			Step:       e.Get("Step"),
		})
	}
	return banks, err
}

// Departments reads the answer to a request for the departments in a system.
//
// Parameters:
//   - doc: the document the scanner answered a department request with
//
// Returns:
//   - the departments the document describes, empty when it describes none
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func Departments(doc string) ([]Department, error) {
	elements, err := Elements(doc, "DEPT")
	if !usable(err) {
		return nil, err
	}

	departments := make([]Department, 0, len(elements))
	for _, e := range elements {
		departments = append(departments, Department{
			Name:     e.Get("Name"),
			Index:    e.Get("Index"),
			Avoided:  !e.Is("Avoid", "Off"),
			QuickKey: e.Optional("Q_Key"),
		})
	}
	return departments, err
}

// Elements returns every element named want.
//
// An empty result is not an error: a favorites list holding no systems answers
// with a document carrying nothing but its footer, and so does a request naming
// an index that does not exist. The two cannot be told apart here, and callers
// that need to distinguish them have to look elsewhere.
//
// Finding elements of a different known kind is an error, because that is the
// scanner answering a question other than the one asked.
//
// A document the scanner cut short returns the elements that did arrive along
// with ErrIncomplete, because they are as good as any other elements and
// throwing them away helps nobody. Callers have to check for it.
//
// Parameters:
//   - doc: the document the scanner answered a list request with
//   - want: the element name to collect, such as "SYS"
//
// Returns:
//   - every element named want, empty when the document carries none
//   - ErrIncomplete if the document's footer says the scanner cut the list
//     short, wrapped alongside the elements that did arrive
//   - error if the document is not valid XML, or carries elements of another
//     known kind instead of the one asked for
func Elements(doc, want string) ([]Element, error) {
	dec := xml.NewDecoder(strings.NewReader(doc))

	var found []Element
	var other string
	cut := false

	for {
		token, err := dec.Token()
		if err != nil {
			// The end of the document arrives as an error, and is the normal
			// way out of this loop.
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("the scanner's answer is not valid XML: %w", err)
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		name := start.Name.Local
		if name == want {
			found = append(found, attributes(start))
			continue
		}
		if strings.EqualFold(name, footerElement) {
			cut = incomplete(attributes(start))
			continue
		}
		if other == "" && isKnown(name) {
			other = name
		}
	}

	if len(found) == 0 && other != "" {
		return nil, fmt.Errorf("the scanner answered with a %s list instead of a %s list, "+
			"which is how this firmware refuses a list it cannot produce", other, want)
	}

	if cut {
		return found, fmt.Errorf("%w: it sent %d %s and a footer saying there are more, "+
			"and there is no way to ask for the rest", ErrIncomplete, len(found), want)
	}
	return found, nil
}

// EveryDepartment reads the departments of every system of every favorites
// list. It is the most expensive read in the tool, and exists only so a
// department can be named by name rather than by index.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - every department of every system, in the order they were read
//   - ErrIncomplete alongside those rows if any of the lists was cut short
//   - error if any of the reads fails
func EveryDepartment(ctx context.Context, client *device.Scanner) ([]Department, error) {
	systems, cut := EverySystem(ctx, client)
	if !usable(cut) {
		return nil, cut
	}

	var all []Department
	for _, s := range systems {
		departments, err := ReadDepartments(ctx, client, s.Index)
		if !usable(err) {
			return nil, err
		}
		if cut == nil {
			cut = err
		}
		all = append(all, departments...)
	}
	return all, cut
}

// EverySite reads the sites of every system, so a site can be named by name
// rather than by index. A site's index says nothing about which system holds
// it, the same way a department's does not.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - every site of every system, in the order they were read
//   - ErrIncomplete alongside those rows if any of the lists was cut short
//   - error if any of the reads fails
func EverySite(ctx context.Context, client *device.Scanner) ([]Site, error) {
	systems, cut := EverySystem(ctx, client)
	if !usable(cut) {
		return nil, cut
	}

	var all []Site
	for _, s := range systems {
		sites, err := ReadSites(ctx, client, s.Index)
		if !usable(err) {
			return nil, err
		}
		if cut == nil {
			cut = err
		}
		all = append(all, sites...)
	}
	return all, cut
}

// EverySystem reads the systems of every favorites list.
//
// A system's index carries no hint of which list holds it, so a name can only
// be resolved by looking through all of them. That costs one exchange per
// favorites list on top of the first.
//
// The built-in sources are not among them, and asking about them is actively
// harmful rather than merely wasteful. The full database answers with every
// system it holds, which is thousands: the replies arrive faster than they can
// be read, the exchange after it collects the tail of that flood instead of its
// own answer, and everything from there on is reading somebody else's post. It
// showed up as "systems new" reporting that the system it had just created did
// not exist, because the read that checked was looking at the wrong answer.
//
// Nothing is lost by skipping them. These lookups exist to turn a name into an
// index, and nothing this tool can name lives in a built-in source: the
// database is read-only, and a system created here is always in a list someone
// made.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - every system of every list someone created, in the order they were read
//   - ErrIncomplete alongside those rows if any of the lists was cut short
//   - error if any of the reads fails
func EverySystem(ctx context.Context, client *device.Scanner) ([]System, error) {
	lists, cut := ReadFavorites(ctx, client)
	if !usable(cut) {
		return nil, cut
	}

	var all []System
	for _, l := range lists {
		if l.BuiltIn {
			continue
		}

		systems, err := ReadSystems(ctx, client, l.Index)
		if !usable(err) {
			return nil, err
		}
		if cut == nil {
			cut = err
		}
		all = append(all, systems...)
	}
	return all, cut
}

// Favorites reads the answer to a favorites list request.
//
// Parameters:
//   - doc: the document the scanner answered a favorites list request with
//
// Returns:
//   - the lists the document describes, empty when it describes none, with the
//     built-in scan sources marked
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func Favorites(doc string) ([]FavoritesList, error) {
	elements, err := Elements(doc, "FL")
	if !usable(err) {
		return nil, err
	}

	lists := make([]FavoritesList, 0, len(elements))
	for _, e := range elements {
		index := e.Get("Index")
		lists = append(lists, FavoritesList{
			Name:      e.Get("Name"),
			Index:     index,
			Monitored: e.Is("Monitor", "On"),
			QuickKey:  e.Optional("Q_Key"),
			NumberTag: e.Optional("N_Tag"),
			BuiltIn:   BuiltInSource(index) != "",
		})
	}
	return lists, err
}

// Frequencies reads the answer to a request for a department's conventional
// frequencies.
//
// Parameters:
//   - doc: the document the scanner answered a frequency request with
//
// Returns:
//   - the channels the document describes, empty when it describes none
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func Frequencies(doc string) ([]Channel, error) {
	elements, err := Elements(doc, "CFREQ")
	if !usable(err) {
		return nil, err
	}

	channels := make([]Channel, 0, len(elements))
	for _, e := range elements {
		channels = append(channels, Channel{
			Name:      e.Get("Name"),
			Index:     e.Get("Index"),
			Frequency: e.Get("Freq"),
			Avoided:   !e.Is("Avoid", "Off"),
		})
	}
	return channels, err
}

// Get returns an attribute with its surrounding space removed, or "" when the
// element does not carry it.
//
// Parameters:
//   - name: the attribute to read
//
// Returns:
//   - the attribute's value without surrounding space
func (e Element) Get(name string) string {
	return strings.TrimSpace(e[name])
}

// Is reports whether an attribute equals value, ignoring case. The scanner
// spells its flags as words, "On" and "Off", and is inconsistent about case
// across firmware.
//
// Parameters:
//   - name: the attribute to read
//   - value: the word to compare it against
//
// Returns:
//   - true if the attribute equals value, ignoring case
func (e Element) Is(name, value string) bool {
	return strings.EqualFold(e.Get(name), value)
}

// IsIndex reports whether a value is a run of digits, which is what the
// scanner's indexes are. Anything else is treated as a name.
//
// Parameters:
//   - value: the name or index to examine
//
// Returns:
//   - true if value is a non-empty run of digits
func IsIndex(value string) bool {
	if value == "" {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

// Optional returns an attribute, treating the scanner's word for "nothing
// assigned" as nothing, so callers do not each have to know that word.
//
// Parameters:
//   - name: the attribute to read
//
// Returns:
//   - the attribute's value, or "" when nothing is assigned
func (e Element) Optional(name string) string {
	value := e.Get(name)
	if strings.EqualFold(value, "None") {
		return ""
	}
	return value
}

// ReadChannels asks the scanner for the channels in one department.
//
// A department holds frequencies or talkgroups depending on the system above
// it, and the scanner reports them as two separate lists, so this asks for both
// and returns whichever answered. Both being empty means the department is
// empty, which is not an error.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - department: the index of the department to read
//
// Returns:
//   - the department's frequencies, or its talkgroups when it holds no
//     frequencies, empty when it holds neither
//   - ErrIncomplete alongside those rows if the scanner cut the list short
//   - error if there is no index to ask with, if either exchange fails, or if
//     either answer cannot be read
func ReadChannels(ctx context.Context, client *device.Scanner, department string) ([]Channel, error) {
	if err := addressable(department, "department"); err != nil {
		return nil, err
	}

	doc, err := client.List(ctx, device.ListFrequencies, department)
	if err != nil {
		return nil, fmt.Errorf("reading the channels: %w", err)
	}
	frequencies, err := Frequencies(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the channels: %w", err)
	}
	if len(frequencies) > 0 {
		return frequencies, wrap("reading the channels", err)
	}

	doc, err = client.List(ctx, device.ListTalkgroups, department)
	if err != nil {
		return nil, fmt.Errorf("reading the talkgroups: %w", err)
	}
	talkgroups, err := Talkgroups(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the talkgroups: %w", err)
	}
	return talkgroups, wrap("reading the talkgroups", err)
}

// ReadCustomSearchBanks asks the scanner for its custom search banks.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - the scanner's custom search banks
//   - ErrIncomplete alongside those rows if the scanner cut the list short,
//     which it does at nine banks with the names it ships
//   - error if the exchange fails or the answer cannot be read
func ReadCustomSearchBanks(ctx context.Context, client *device.Scanner) ([]CustomSearchBank, error) {
	doc, err := client.List(ctx, device.ListCustomSearchBanks)
	if err != nil {
		return nil, fmt.Errorf("reading the custom search banks: %w", err)
	}

	banks, err := CustomSearchBanks(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the custom search banks: %w", err)
	}
	return banks, wrap("reading the custom search banks", err)
}

// ReadDepartments asks the scanner for the departments in one system.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - system: the index of the system to read
//
// Returns:
//   - the system's departments, empty when it holds none
//   - ErrIncomplete alongside those rows if the scanner cut the list short
//   - error if there is no index to ask with, if the exchange fails, or if the
//     answer cannot be read
func ReadDepartments(ctx context.Context, client *device.Scanner, system string) ([]Department, error) {
	if err := addressable(system, "system"); err != nil {
		return nil, err
	}

	doc, err := client.List(ctx, device.ListDepartments, system)
	if err != nil {
		return nil, fmt.Errorf("reading the departments: %w", err)
	}

	departments, err := Departments(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the departments: %w", err)
	}
	return departments, wrap("reading the departments", err)
}

// ReadFavorites asks the scanner for its favorites lists.
//
// The reading functions live here, rather than in whichever command needed
// them first, so that commands never have to import one another to reach a
// level above their own.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - the scanner's favorites lists, including its built-in scan sources
//   - ErrIncomplete alongside those rows if the scanner cut the list short
//   - error if the exchange fails or the answer cannot be read
func ReadFavorites(ctx context.Context, client *device.Scanner) ([]FavoritesList, error) {
	doc, err := client.List(ctx, device.ListFavorites)
	if err != nil {
		return nil, fmt.Errorf("reading the favorites lists: %w", err)
	}

	lists, err := Favorites(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the favorites lists: %w", err)
	}
	return lists, wrap("reading the favorites lists", err)
}

// ReadSiteFrequencies asks the scanner for one site's frequencies.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - site: the index of the site to read
//
// Returns:
//   - the site's frequencies, empty when it holds none
//   - ErrIncomplete alongside those rows if the scanner cut the list short
//   - error if there is no index to ask with, if the exchange fails, or if the
//     answer cannot be read
func ReadSiteFrequencies(ctx context.Context, client *device.Scanner, site string) ([]SiteFrequency, error) {
	if err := addressable(site, "site"); err != nil {
		return nil, err
	}

	doc, err := client.List(ctx, device.ListSiteFrequencies, site)
	if err != nil {
		return nil, fmt.Errorf("reading the site's frequencies: %w", err)
	}

	frequencies, err := SiteFrequencies(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the site's frequencies: %w", err)
	}
	return frequencies, wrap("reading the site's frequencies", err)
}

// ReadSites asks the scanner for the sites in one system. A conventional
// system has none and answers emptily.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - system: the index of the system to read
//
// Returns:
//   - the system's sites, empty when it holds none
//   - ErrIncomplete alongside those rows if the scanner cut the list short
//   - error if there is no index to ask with, if the exchange fails, or if the
//     answer cannot be read
func ReadSites(ctx context.Context, client *device.Scanner, system string) ([]Site, error) {
	if err := addressable(system, "system"); err != nil {
		return nil, err
	}

	doc, err := client.List(ctx, device.ListSites, system)
	if err != nil {
		return nil, fmt.Errorf("reading the sites: %w", err)
	}

	sites, err := Sites(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the sites: %w", err)
	}
	return sites, wrap("reading the sites", err)
}

// ReadSystems asks the scanner for the systems in one favorites list.
//
// A built-in scan source is refused before anything is sent. The full database
// answers this request with a short, wrong answer and then stops responding
// until the radio is power cycled, reproduced twice, so the refusal lives here
// rather than in the command: this is where the request goes on the wire, and
// nothing above it can reach the scanner without passing through.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - list: the index of the favorites list to read
//
// Returns:
//   - the list's systems, empty when it holds none
//   - ErrIncomplete alongside those rows if the scanner cut the list short
//   - error if the list is a built-in scan source, if there is no index to ask
//     with, if the exchange fails, or if the answer cannot be read
func ReadSystems(ctx context.Context, client *device.Scanner, list string) ([]System, error) {
	if source := BuiltInSource(list); source != "" {
		return nil, builtInRefusal(source)
	}

	if err := addressable(list, "favorites list"); err != nil {
		return nil, err
	}

	doc, err := client.List(ctx, device.ListSystems, list)
	if err != nil {
		return nil, fmt.Errorf("reading the systems: %w", err)
	}

	systems, err := Systems(doc)
	if !usable(err) {
		return nil, fmt.Errorf("reading the systems: %w", err)
	}
	return systems, wrap("reading the systems", err)
}

// Resolve turns a name or an index into an index.
//
// A run of digits is returned unchanged, without consulting the list, so the
// common case costs nothing. A name is matched without regard to case, and an
// ambiguous one is refused rather than guessed at, because two entries really
// can share a name.
//
// The kind is used in the messages, and should be singular: "system".
//
// Parameters:
//   - value: the name or index to resolve
//   - kind: what one entry is called, singular, for the messages
//   - all: the entries to match the name against
//
// Returns:
//   - the index of the one entry that matches, or value itself when it is
//     already an index
//   - error if nothing carries that name, or more than one thing does
func Resolve[T Named](value, kind string, all []T) (string, error) {
	if IsIndex(value) {
		return value, nil
	}

	var matched []string
	names := make([]string, 0, len(all))
	for _, entry := range all {
		name, index := entry.named()
		names = append(names, fmt.Sprintf("%q", name))
		if strings.EqualFold(name, value) {
			matched = append(matched, index)
		}
	}

	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		if len(names) == 0 {
			return "", fmt.Errorf("no %s is called %q: the scanner reports none at all", kind, value)
		}
		return "", fmt.Errorf("no %s is called %q: the scanner has %s", kind, value, strings.Join(names, ", "))
	default:
		return "", fmt.Errorf("%d %ss are called %q: name one by its index instead", len(matched), kind, value)
	}
}

// ResolveDepartment turns a name or an index into a department index.
//
// An index is used as it stands. A name is the most expensive lookup in this
// tool, because a department's index says nothing about which system or
// favorites list holds it, so all of them have to be read.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the departments from
//   - want: a department's index, or its name
//
// Returns:
//   - the index of the department want names
//   - error if the departments cannot be read, or if no department is called
//     want
func ResolveDepartment(ctx context.Context, client *device.Scanner, want string) (string, error) {
	return resolveNamed(ctx, client, want, "department", EveryDepartment)
}

// ResolveFavorites turns a name or an index into a favorites list index.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the favorites lists from
//   - want: a favorites list's index, or its name
//
// Returns:
//   - the index of the favorites list want names
//   - error if the lists cannot be read, or if none is called want
func ResolveFavorites(ctx context.Context, client *device.Scanner, want string) (string, error) {
	return resolveNamed(ctx, client, want, "favorites list", ReadFavorites)
}

// ResolveSite turns a name or an index into a site index.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the sites from
//   - want: a site's index, or its name
//
// Returns:
//   - the index of the site want names
//   - error if the sites cannot be read, or if no site is called want
func ResolveSite(ctx context.Context, client *device.Scanner, want string) (string, error) {
	return resolveNamed(ctx, client, want, "site", EverySite)
}

// ResolveSystem turns a name or an index into a system index.
//
// An index is used as it stands. A name costs a read of every favorites list
// and every system in them, because a system's index says nothing about which
// list holds it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the systems from
//   - want: a system's index, or its name
//
// Returns:
//   - the index of the system want names
//   - error if the systems cannot be read, or if no system is called want
func ResolveSystem(ctx context.Context, client *device.Scanner, want string) (string, error) {
	return resolveNamed(ctx, client, want, "system", EverySystem)
}

// SiteFrequencies reads the answer to a request for a site's frequencies.
//
// Parameters:
//   - doc: the document the scanner answered a site frequency request with
//
// Returns:
//   - the frequencies the document describes, empty when it describes none
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func SiteFrequencies(doc string) ([]SiteFrequency, error) {
	elements, err := Elements(doc, "SFREQ")
	if !usable(err) {
		return nil, err
	}

	frequencies := make([]SiteFrequency, 0, len(elements))
	for _, e := range elements {
		frequencies = append(frequencies, SiteFrequency{
			Frequency: e.Get("Freq"),
			Index:     e.Get("Index"),
		})
	}
	return frequencies, err
}

// Sites reads the answer to a request for the sites in a system.
//
// Parameters:
//   - doc: the document the scanner answered a site request with
//
// Returns:
//   - the sites the document describes, empty when it describes none
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func Sites(doc string) ([]Site, error) {
	elements, err := Elements(doc, "SITE")
	if !usable(err) {
		return nil, err
	}

	sites := make([]Site, 0, len(elements))
	for _, e := range elements {
		sites = append(sites, Site{
			Name:     e.Get("Name"),
			Index:    e.Get("Index"),
			Avoided:  !e.Is("Avoid", "Off"),
			QuickKey: e.Optional("Q_Key"),
		})
	}
	return sites, err
}

// Systems reads the answer to a request for the systems in a favorites list.
//
// Parameters:
//   - doc: the document the scanner answered a system request with
//
// Returns:
//   - the systems the document describes, empty when it describes none
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func Systems(doc string) ([]System, error) {
	elements, err := Elements(doc, "SYS")
	if !usable(err) {
		return nil, err
	}

	systems := make([]System, 0, len(elements))
	for _, e := range elements {
		systems = append(systems, System{
			Name:      e.Get("Name"),
			Index:     e.Get("Index"),
			Kind:      e.Get("Type"),
			Avoided:   !e.Is("Avoid", "Off"),
			QuickKey:  e.Optional("Q_Key"),
			NumberTag: e.Optional("N_Tag"),
		})
	}
	return systems, err
}

// Talkgroups reads the answer to a request for a department's talkgroups.
//
// Parameters:
//   - doc: the document the scanner answered a talkgroup request with
//
// Returns:
//   - the channels the document describes, empty when it describes none
//   - ErrIncomplete alongside those rows if the document's footer says the
//     scanner cut the list short
//   - error if the document is not valid XML, or is a list of another kind
func Talkgroups(doc string) ([]Channel, error) {
	elements, err := Elements(doc, "TGID")
	if !usable(err) {
		return nil, err
	}

	channels := make([]Channel, 0, len(elements))
	for _, e := range elements {
		channels = append(channels, Channel{
			Name:      e.Get("Name"),
			Index:     e.Get("Index"),
			Talkgroup: e.Get("TGID"),
			Avoided:   !e.Is("Avoid", "Off"),
		})
	}
	return channels, err
}

// addressable refuses a request that has nothing to address.
//
// An index is how every list below the favorites lists is asked for, and an
// empty one produces a command with a trailing comma and no argument, which the
// scanner answers with something nobody wants. It can happen honestly: an entry
// this tool found by walking the menus carries a name and, when the scanner's
// own listing never mentioned it, no index at all. Saying so beats sending it.
//
// Parameters:
//   - index: the index the request would be addressed by
//   - kind: what the index names, singular, for the message
//
// Returns:
//   - error if there is no index to address the request with
func addressable(index, kind string) error {
	if strings.TrimSpace(index) != "" {
		return nil
	}
	return fmt.Errorf("no %s index was given: the scanner addresses this request by index, "+
		"and this entry was read off its menus, which do not carry one", kind)
}

// attributes collects an element's attributes by name.
//
// Parameters:
//   - start: the opening tag to read the attributes from
//
// Returns:
//   - the attributes by name, empty when the tag carries none
func attributes(start xml.StartElement) Element {
	e := make(Element, len(start.Attr))
	for _, a := range start.Attr {
		e[a.Name.Local] = a.Value
	}
	return e
}

// builtInRefusal explains why a built-in scan source cannot be listed.
//
// The two are refused for different reasons and the message says which, because
// "run it a different way" and "there is nothing there" send a reader to
// different places.
//
// Parameters:
//   - source: the scanner's name for the built-in source, as BuiltInSource
//     reports it
//
// Returns:
//   - the refusal, naming the source and what to do instead
func builtInRefusal(source string) error {
	if source == builtInNames[searchWithScanIndex] {
		return fmt.Errorf("%q is built into the scanner rather than a favorites list, and "+
			"holds no systems of its own: it sweeps frequency ranges", source)
	}

	return fmt.Errorf("%q is the scanner's built-in database rather than a favorites list, "+
		"and asking it for its systems returns a short, wrong answer and then locks the "+
		"scanner up until it is power cycled: run \"radiocli scanning systems\" to read "+
		"what it is scanning instead", source)
}

// incomplete reads a list document's footer and reports whether the scanner
// said there was more it did not send.
//
// Only the word for "not the end" counts. A footer saying the document finished
// means it finished, and a footer that says nothing either way is treated the
// same as the documents that carry no footer at all, which is most of them: a
// list is assumed whole unless the scanner says otherwise, because reporting
// every short list as suspect would send every read down the slow path.
//
// Parameters:
//   - footer: the attributes of the document's footer element
//
// Returns:
//   - true if the footer says the scanner cut the list short
func incomplete(footer Element) bool {
	value, carried := footer[endOfTextAttribute]
	if !carried {
		return false
	}
	return strings.TrimSpace(value) == notTheEnd
}

// wrap puts a read's own words in front of an error, and keeps nil as nil.
//
// It exists because a truncated list is returned as rows plus an error, so the
// error has to be passed along a path where it is usually absent. fmt.Errorf
// would happily wrap nil into a real error saying nothing went wrong.
//
// Parameters:
//   - what: what the read was doing, as it would start an error message
//   - err: the error to wrap, which is usually nil
//
// Returns:
//   - the error with what in front of it, or nil when there was no error
func wrap(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}

// isKnown reports whether name is one of the list element names.
//
// Parameters:
//   - name: the element name to look for
//
// Returns:
//   - true if a list document can be built from elements of that name
func isKnown(name string) bool {
	for _, k := range known {
		if k == name {
			return true
		}
	}
	return false
}

// named returns the department's name and index.
func (d Department) named() (string, string) { return d.Name, d.Index }

// resolveNamed is the shape the four ResolveX functions above share: an index
// is taken as it stands, and anything else is matched by name against a read.
//
// The index check is here rather than left to Resolve, which does it too, and
// the difference is the whole point of these wrappers. Resolve is handed a list
// that has already been read; this decides whether to read one at all. For a
// department that read is every list, every system and every department on the
// scanner, so answering an index without it turns the most expensive lookup in
// the tool into no exchange whatsoever.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from when a name has to be matched
//   - want: an index, or a name
//   - kind: what one entry is called, singular, for the messages
//   - read: how to read every entry of that kind from the scanner
//
// Returns:
//   - the index want names, or want itself when it is already an index
//   - error if the entries cannot be read, or if none is called want
func resolveNamed[T Named](ctx context.Context, client *device.Scanner, want, kind string,
	read func(context.Context, *device.Scanner) ([]T, error)) (string, error) {

	if IsIndex(want) {
		return want, nil
	}

	all, err := read(ctx, client)
	if err != nil {
		return "", err
	}
	return Resolve(want, kind, all)
}

// usable reports whether the rows an error came with are worth keeping.
//
// Only one error means that: the scanner having cut the list short, which
// leaves the rows it did send as good as any others. Everything else means the
// document could not be read at all, and there are no rows to keep.
//
// Parameters:
//   - err: the error a read or a parse returned, which may be nil
//
// Returns:
//   - true if there was no error, or if the error was ErrIncomplete
func usable(err error) bool {
	return err == nil || errors.Is(err, ErrIncomplete)
}

// named returns the favorites list's name and index.
func (l FavoritesList) named() (string, string) { return l.Name, l.Index }

// named returns the site's name and index.
func (s Site) named() (string, string) { return s.Name, s.Index }

// named returns the system's name and index.
func (s System) named() (string, string) { return s.Name, s.Index }
