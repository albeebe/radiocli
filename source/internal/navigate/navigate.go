// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package navigate walks the scanner's menus to the places its own memory
// lives.
//
// The menu machinery knows how to move around a menu; this knows which menus to
// move through to reach a favorites list, and what they are called. It is a
// separate package because more than one command needs the same walk, and
// commands must not import one another.
package navigate

import (
	"context"
	"fmt"

	"github.com/albeebe/radiocli/internal/catalog"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/menus"
)

// ToChannels puts the scanner on the list of channels inside one department,
// which is where a new one is created and where the existing ones are read.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - department: the department to open, by name or by index
//
// Returns:
//   - error if the department cannot be resolved or opened, or if its menu
//     carries no entry for the channels
func ToChannels(ctx context.Context, client *device.Scanner, department string) error {
	if _, err := ToDepartment(ctx, client, department); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, editChannels); err != nil {
		return fmt.Errorf("looking for %q on the department's menu: %w", editChannels, err)
	}
	return nil
}

// ToDepartment puts the scanner on the menu for one department.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - want: the department to open, by name or by index
//
// Returns:
//   - the index the scanner knows the department by
//   - error if the name matches no department, or if the menu cannot be opened
func ToDepartment(ctx context.Context, client *device.Scanner, want string) (string, error) {
	index, err := resolve(ctx, client, want, "department", func() ([]catalog.Department, error) {
		return catalog.EveryDepartment(ctx, client)
	})
	if err != nil {
		return "", err
	}
	if err := client.OpenMenu(ctx, device.MenuDepartment, index); err != nil {
		return "", fmt.Errorf("opening the department's menu: %w", err)
	}
	return index, nil
}

// ToDepartments puts the scanner on the list of departments inside one system,
// which is where a new one is created.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - system: the system to open, by name or by index
//
// Returns:
//   - error if the system cannot be resolved or opened, or if its menu carries
//     no entry for the departments
func ToDepartments(ctx context.Context, client *device.Scanner, system string) error {
	if _, err := ToSystem(ctx, client, system); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, editDepartments); err != nil {
		return fmt.Errorf("looking for %q on the system's menu: %w", editDepartments, err)
	}
	return nil
}

// ToFavorites puts the scanner on the list of favorites lists.
//
// It starts from the top menu rather than from wherever the scanner happens to
// be, so the walk is the same every time whatever was on screen before.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//
// Returns:
//   - error if the top menu cannot be opened, or if it carries no entry for
//     the favorites lists
func ToFavorites(ctx context.Context, client *device.Scanner) error {
	if err := client.OpenMenu(ctx, device.MenuTop, ""); err != nil {
		return fmt.Errorf("opening the top menu: %w", err)
	}
	if err := menus.Select(ctx, client, manageFavorites); err != nil {
		return fmt.Errorf("looking for %q in the top menu: %w", manageFavorites, err)
	}
	return nil
}

// ToFavoritesList puts the scanner on the menu for one favorites list, and
// returns the list it landed on.
//
// The list is resolved before the scanner is touched, so a name the scanner
// does not have costs nothing and leaves it scanning. A built-in scan source is
// refused: those have no menu of their own.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - want: the favorites list to open, by name or by index
//
// Returns:
//   - the favorites list the scanner landed on
//   - error if the lists cannot be read, if the name matches none of them, if
//     the one it matches is built into the scanner, or if the walk fails
func ToFavoritesList(ctx context.Context, client *device.Scanner, want string) (catalog.FavoritesList, error) {
	lists, err := catalog.ReadFavorites(ctx, client)
	if err != nil {
		return catalog.FavoritesList{}, err
	}

	index, err := catalog.Resolve(want, "favorites list", lists)
	if err != nil {
		return catalog.FavoritesList{}, err
	}

	var target catalog.FavoritesList
	for _, l := range lists {
		if l.Index == index {
			target = l
			break
		}
	}
	if target.Name == "" {
		return catalog.FavoritesList{}, fmt.Errorf("no favorites list has index %s", index)
	}
	if target.BuiltIn {
		return catalog.FavoritesList{}, fmt.Errorf("%q is built into the scanner and has no menu of its own: "+
			"only a list you created can be opened this way", target.Name)
	}

	if err := ToFavorites(ctx, client); err != nil {
		return catalog.FavoritesList{}, err
	}
	if err := menus.Select(ctx, client, target.Name); err != nil {
		return catalog.FavoritesList{}, fmt.Errorf("looking for %q: %w", target.Name, err)
	}
	return target, nil
}

// ToSite puts the scanner on the menu for one site.
//
// Like a system and a department, the protocol takes a site's index directly,
// so this lands in a single exchange whatever the scanner was showing before.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - want: the site to open, by name or by index
//
// Returns:
//   - the index the scanner knows the site by
//   - error if the name matches no site, or if the menu cannot be opened
func ToSite(ctx context.Context, client *device.Scanner, want string) (string, error) {
	index, err := resolve(ctx, client, want, "site", func() ([]catalog.Site, error) {
		return catalog.EverySite(ctx, client)
	})
	if err != nil {
		return "", err
	}
	if err := client.OpenMenu(ctx, device.MenuSite, index); err != nil {
		return "", fmt.Errorf("opening the site's menu: %w", err)
	}
	return index, nil
}

// ToSiteFrequencies puts the scanner on the list of one site's frequencies,
// which is where a new one is created and where the existing ones are read.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - site: the site to open, by name or by index
//
// Returns:
//   - error if the site cannot be resolved or opened, or if its menu carries
//     no entry for the frequencies
func ToSiteFrequencies(ctx context.Context, client *device.Scanner, site string) error {
	if _, err := ToSite(ctx, client, site); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, setFrequencies); err != nil {
		return fmt.Errorf("looking for %q on the site's menu: %w", setFrequencies, err)
	}
	return nil
}

// ToSites puts the scanner on the list of sites inside one system, which is
// where a new one is created.
//
// Only a trunked system has sites. A conventional system's menu carries no
// Edit Site entry at all, so this fails there rather than landing somewhere
// unexpected, and says why.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - system: the system to open, by name or by index
//
// Returns:
//   - error if the system cannot be resolved or opened, or if its menu carries
//     no entry for the sites, which is what a conventional system looks like
func ToSites(ctx context.Context, client *device.Scanner, system string) error {
	if _, err := ToSystem(ctx, client, system); err != nil {
		return err
	}
	if err := menus.Select(ctx, client, editSites); err != nil {
		return fmt.Errorf("looking for %q on the system's menu: %w\n"+
			"Only a trunked system has sites: a conventional one holds its frequencies "+
			"in departments instead", editSites, err)
	}
	return nil
}

// ToSystem puts the scanner on the menu for one system.
//
// Unlike a favorites list, the protocol takes a system's index directly, so
// this lands in a single exchange whatever the scanner was showing before.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - want: the system to open, by name or by index
//
// Returns:
//   - the index the scanner knows the system by
//   - error if the name matches no system, or if the menu cannot be opened
func ToSystem(ctx context.Context, client *device.Scanner, want string) (string, error) {
	index, err := resolve(ctx, client, want, "system", func() ([]catalog.System, error) {
		return catalog.EverySystem(ctx, client)
	})
	if err != nil {
		return "", err
	}
	if err := client.OpenMenu(ctx, device.MenuSystem, index); err != nil {
		return "", fmt.Errorf("opening the system's menu: %w", err)
	}
	return index, nil
}

// ToSystems puts the scanner on the list of systems inside one favorites list,
// which is where a new one is created.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - client: the scanner to walk
//   - list: the favorites list to open, by name or by index
//
// Returns:
//   - the favorites list the scanner landed on
//   - error if the list cannot be resolved or opened, or if its menu carries
//     no entry for the systems
func ToSystems(ctx context.Context, client *device.Scanner, list string) (catalog.FavoritesList, error) {
	target, err := ToFavoritesList(ctx, client, list)
	if err != nil {
		return catalog.FavoritesList{}, err
	}
	if err := menus.Select(ctx, client, reviewSystems); err != nil {
		return catalog.FavoritesList{}, fmt.Errorf("looking for %q on the %s menu: %w",
			reviewSystems, target.Name, err)
	}
	return target, nil
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
