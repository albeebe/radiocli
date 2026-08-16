// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/16/2026

package navigate

import (
	"context"
	"errors"
	"fmt"

	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
)

// EveryDepartment reads the departments of every system of every favorites
// list, leaving none out.
//
// A system the scanner never gave an index for is skipped, because a department
// request is addressed by that index and there is nothing else to address it
// with.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - every department of every system the scanner will name, in the order they
//     were read
//   - error if any of the reads fails
func EveryDepartment(ctx context.Context, client *device.Scanner) ([]catalog.Department, error) {
	systems, err := EverySystem(ctx, client)
	if err != nil {
		return nil, err
	}

	var all []catalog.Department
	for _, s := range systems {
		if s.Index == "" {
			// A system found on the menus and never mentioned in a listing has
			// no index, and its departments are addressed by that index alone.
			// There is nothing to ask with, so there is nothing to ask.
			continue
		}

		departments, err := ReadDepartments(ctx, client, s.Index)
		if err != nil {
			return nil, err
		}
		all = append(all, departments...)
	}
	return all, nil
}

// EverySite reads the sites of every system, leaving none out.
//
// A system the scanner never gave an index for is skipped, for the reason
// EveryDepartment gives.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - every site of every system the scanner will name, in the order they were
//     read
//   - error if any of the reads fails
func EverySite(ctx context.Context, client *device.Scanner) ([]catalog.Site, error) {
	systems, err := EverySystem(ctx, client)
	if err != nil {
		return nil, err
	}

	var all []catalog.Site
	for _, s := range systems {
		if s.Index == "" {
			continue
		}

		sites, err := ReadSites(ctx, client, s.Index)
		if err != nil {
			return nil, err
		}
		all = append(all, sites...)
	}
	return all, nil
}

// EverySystem reads the systems of every favorites list someone created,
// leaving none out.
//
// The built-in scan sources are skipped, for the reason catalog.EverySystem
// gives: asking the full database for its systems locks the scanner up, and
// nothing this tool can name lives in one of them anyway. So is a list the
// scanner never gave an index for, because a system request is addressed by
// that index and there is nothing else to address it with.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - every system of every list someone created that the scanner will name, in
//     the order they were read
//   - error if any of the reads fails
func EverySystem(ctx context.Context, client *device.Scanner) ([]catalog.System, error) {
	lists, err := ReadFavorites(ctx, client)
	if err != nil {
		return nil, err
	}

	var all []catalog.System
	for _, l := range lists {
		if l.BuiltIn || l.Index == "" {
			continue
		}

		systems, err := ReadSystems(ctx, client, l.Index)
		if err != nil {
			return nil, err
		}
		all = append(all, systems...)
	}
	return all, nil
}

// ReadChannels reads every channel in one department.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - department: the index of the department to read
//
// Returns:
//   - the department's channels, empty when it holds none, with any the
//     scanner's list left out marked Partial
//   - error if the list cannot be read, or if it was cut short and the walk
//     that would have finished it failed
func ReadChannels(ctx context.Context, client *device.Scanner, department string) ([]catalog.Channel, error) {
	found, cut := catalog.ReadChannels(ctx, client, department)
	return filled(ctx, client, found, cut, "channel",
		func() error { return ToChannels(ctx, client, department) },
		newChannel,
		func(c catalog.Channel) string { return c.Name },
		func(e menus.Entry) catalog.Channel {
			return catalog.Channel{Name: e.Name, Index: e.Index, Partial: true}
		})
}

// ReadDepartments reads every department in one system.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - system: the index of the system to read
//
// Returns:
//   - the system's departments, empty when it holds none, with any the
//     scanner's list left out marked Partial
//   - error if the list cannot be read, or if it was cut short and the walk
//     that would have finished it failed
func ReadDepartments(ctx context.Context, client *device.Scanner, system string) ([]catalog.Department, error) {
	found, cut := catalog.ReadDepartments(ctx, client, system)
	return filled(ctx, client, found, cut, "department",
		func() error { return ToDepartments(ctx, client, system) },
		newDepartment,
		func(d catalog.Department) string { return d.Name },
		func(e menus.Entry) catalog.Department {
			return catalog.Department{Name: e.Name, Index: e.Index, Partial: true}
		})
}

// ReadFavorites reads every favorites list.
//
// The built-in scan sources are part of the answer, as they are in the
// scanner's own list, but they are never among the ones a walk fills in: they
// are not on the Manage Favorites menu, because there is nothing there to
// manage.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//
// Returns:
//   - the scanner's favorites lists, with any its own list left out marked
//     Partial
//   - error if the list cannot be read, or if it was cut short and the walk
//     that would have finished it failed
func ReadFavorites(ctx context.Context, client *device.Scanner) ([]catalog.FavoritesList, error) {
	found, cut := catalog.ReadFavorites(ctx, client)
	return filled(ctx, client, found, cut, "favorites list",
		func() error { return ToFavorites(ctx, client) },
		newFavoritesList,
		func(l catalog.FavoritesList) string { return l.Name },
		func(e menus.Entry) catalog.FavoritesList {
			return catalog.FavoritesList{Name: e.Name, Index: e.Index, Partial: true}
		})
}

// ReadSiteFrequencies reads every frequency of one site.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - site: the index of the site to read
//
// Returns:
//   - the site's frequencies, empty when it holds none, with any the scanner's
//     list left out marked Partial
//   - error if the list cannot be read, or if it was cut short and the walk
//     that would have finished it failed
func ReadSiteFrequencies(ctx context.Context, client *device.Scanner, site string) ([]catalog.SiteFrequency, error) {
	found, cut := catalog.ReadSiteFrequencies(ctx, client, site)
	return filled(ctx, client, found, cut, "site frequency",
		func() error { return ToSiteFrequencies(ctx, client, site) },
		newFrequency,
		func(f catalog.SiteFrequency) string { return f.Frequency },
		func(e menus.Entry) catalog.SiteFrequency {
			return catalog.SiteFrequency{Frequency: e.Name, Index: e.Index, Partial: true}
		})
}

// ReadSites reads every site in one system. A conventional system has none.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - system: the index of the system to read
//
// Returns:
//   - the system's sites, empty when it holds none, with any the scanner's
//     list left out marked Partial
//   - error if the list cannot be read, or if it was cut short and the walk
//     that would have finished it failed
func ReadSites(ctx context.Context, client *device.Scanner, system string) ([]catalog.Site, error) {
	found, cut := catalog.ReadSites(ctx, client, system)
	return filled(ctx, client, found, cut, "site",
		func() error { return ToSites(ctx, client, system) },
		newSite,
		func(s catalog.Site) string { return s.Name },
		func(e menus.Entry) catalog.Site {
			return catalog.Site{Name: e.Name, Index: e.Index, Partial: true}
		})
}

// ReadSystems reads every system in one favorites list.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read from
//   - list: the index of the favorites list to read
//
// Returns:
//   - the list's systems, empty when it holds none, with any the scanner's
//     list left out marked Partial
//   - error if the list is a built-in scan source, if it cannot be read, or if
//     it was cut short and the walk that would have finished it failed
func ReadSystems(ctx context.Context, client *device.Scanner, list string) ([]catalog.System, error) {
	found, cut := catalog.ReadSystems(ctx, client, list)
	return filled(ctx, client, found, cut, "system",
		func() error { _, err := ToSystems(ctx, client, list); return err },
		newSystem,
		func(s catalog.System) string { return s.Name },
		func(e menus.Entry) catalog.System {
			return catalog.System{Name: e.Name, Index: e.Index, Partial: true}
		})
}

// ResolveDepartment turns a name or an index into a department index, over a
// list of departments that leaves none out.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the departments from
//   - want: a department's index, or its name
//
// Returns:
//   - the index of the department want names
//   - error if the departments cannot be read, or if none is called want
func ResolveDepartment(ctx context.Context, client *device.Scanner, want string) (string, error) {
	return resolve(ctx, client, want, "department", func() ([]catalog.Department, error) {
		return EveryDepartment(ctx, client)
	})
}

// ResolveFavorites turns a name or an index into a favorites list index, over a
// list that leaves none out.
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
	return resolve(ctx, client, want, "favorites list", func() ([]catalog.FavoritesList, error) {
		return ReadFavorites(ctx, client)
	})
}

// ResolveSite turns a name or an index into a site index, over a list of sites
// that leaves none out.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the sites from
//   - want: a site's index, or its name
//
// Returns:
//   - the index of the site want names
//   - error if the sites cannot be read, or if none is called want
func ResolveSite(ctx context.Context, client *device.Scanner, want string) (string, error) {
	return resolve(ctx, client, want, "site", func() ([]catalog.Site, error) {
		return EverySite(ctx, client)
	})
}

// ResolveSystem turns a name or an index into a system index, over a list of
// systems that leaves none out.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to read the systems from
//   - want: a system's index, or its name
//
// Returns:
//   - the index of the system want names
//   - error if the systems cannot be read, or if none is called want
func ResolveSystem(ctx context.Context, client *device.Scanner, want string) (string, error) {
	return resolve(ctx, client, want, "system", func() ([]catalog.System, error) {
		return EverySystem(ctx, client)
	})
}

// filled finishes a list the scanner cut short, by walking the menu that shows
// the same list on screen.
//
// The protocol caps a list reply at about a kilobyte and offers no way to ask
// for the rest, so a long list comes back as a prefix with a footer admitting
// it. The screen has no such cap: the knob reaches every entry, one at a time.
// So the rows that did arrive are kept for their details, and the walk supplies
// the names of the ones that did not.
//
// The two are lined up by name rather than by position, and each row is spent
// once. Position would be the obvious choice, and it is the one that breaks
// quietly if the walk ever starts somewhere other than the top of the list;
// matching by name gives the same answer when the two agree and a sensible one
// when they do not. A walked entry that finds no row is returned as a name
// alone, marked Partial, which is the honest description of what is known about
// it.
//
// This costs several seconds and stops the scan, so it runs only when the
// scanner has said the list was cut short.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - found: the rows the scanner's list did carry
//   - cut: the error that list came with, nil when it was whole
//   - kind: what one entry is called, singular, for the messages
//   - to: puts the scanner on the menu that shows the same list
//   - skip: the menu entry that creates a new one, which is not an entry of the
//     list
//   - name: the name of one of the rows the list carried
//   - blank: builds an entry from a walked row, for the ones the list left out
//
// Returns:
//   - the whole list, in the order the menu holds it
//   - error if the list could not be read at all, or if it was cut short and
//     the walk that would have finished it failed
func filled[T any](ctx context.Context, client *device.Scanner, found []T, cut error, kind string,
	to func() error, skip string, name func(T) string, blank func(menus.Entry) T) ([]T, error) {

	if !errors.Is(cut, catalog.ErrIncomplete) {
		// Either the read worked, in which case the list is whole, or it failed
		// for a reason a walk cannot make up for.
		return found, cut
	}

	entries, err := walked(ctx, client, to, skip)
	if err != nil {
		return found, fmt.Errorf("%w, and reading the rest off its menus failed: %w", cut, err)
	}

	spent := make([]bool, len(found))
	whole := make([]T, 0, len(entries))
	for _, e := range entries {
		matched := false
		for i, row := range found {
			if spent[i] || !menus.Matches(e, name(row)) {
				continue
			}
			whole = append(whole, row)
			spent[i] = true
			matched = true
			break
		}
		if !matched {
			whole = append(whole, blank(e))
		}
	}

	// A row the walk never accounted for means the two readings disagree about
	// what the list holds, which is worth saying rather than dropping.
	for i, row := range found {
		if !spent[i] {
			return nil, fmt.Errorf("the scanner listed a %s called %q that its own menu does not show: "+
				"run \"radiocli scan\" and try again", kind, name(row))
		}
	}
	return whole, nil
}

// resolve turns a name or an index into an index, reading the whole catalogue
// only when a name makes that necessary.
//
// Parameters:
//   - ctx: context for cancellation and timeouts, unused here because all
//     closes over its own
//   - client: the scanner the entries come from, unused here for the same
//     reason
//   - want: the entry to find, by name or by index
//   - kind: what the entries are called in an error, such as "system"
//   - all: reads the whole catalogue of entries, called only when want is a
//     name
//
// Returns:
//   - the index the scanner knows the entry by
//   - error if the catalogue cannot be read, or if the name matches no entry
func resolve[T catalog.Named](ctx context.Context, client *device.Scanner, want, kind string, all func() ([]T, error)) (string, error) {
	if catalog.IsIndex(want) {
		return want, nil
	}

	entries, err := all()
	if err != nil {
		return "", err
	}
	return catalog.Resolve(want, kind, entries)
}

// walked puts the scanner on a list's menu, reads every entry off the screen,
// and takes it back out again.
//
// Leaving is not optional and not the caller's business: this is called from
// inside a read, which nobody expects to park the radio in a menu, and it is
// called for its answer rather than for where it ends up.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - to: puts the scanner on the menu that shows the list
//   - skip: the menu entry that creates a new one, which is not an entry of the
//     list
//
// Returns:
//   - the menu's entries, without the one that creates a new entry
//   - error if the menu cannot be reached, if it cannot be read, or if the
//     scanner cannot be taken back out of the menus
func walked(ctx context.Context, client *device.Scanner, to func() error, skip string) ([]menus.Entry, error) {
	if err := to(); err != nil {
		return nil, err
	}

	entries, err := menus.FullEntries(ctx, client)
	if err != nil {
		menus.Leave(ctx, client)
		return nil, err
	}

	if _, err := menus.Leave(ctx, client); err != nil {
		return nil, err
	}

	out := make([]menus.Entry, 0, len(entries))
	for _, e := range entries {
		if menus.Matches(e, skip) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
