// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

// ListKind names one of the lists the scanner can report.
type ListKind string

// MenuID names a menu the scanner can be sent to.
type MenuID string

// CloseMenu leaves the menu the scanner is in and returns to what it was doing
// before.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) CloseMenu(ctx context.Context) error {
	return s.set(ctx, "MSB,RETURN_PREVOUS_MODE")
}

// Holding reports whether the scanner is parked on one thing rather than
// working through a list.
//
// This is the only place the difference shows. A held scanner is out of the
// menus, answers every command correctly, and shows a channel name on its
// screen, which is also what a scanning one does every time it stops on
// something it is receiving. Nothing but the mode tells them apart.
//
// Holding survives almost everything, so anything that means to leave the
// scanner scanning has to check. Turning the knob is enough to cause it.
//
// Returns:
//   - true if the scanner is parked on one thing, false if it is working
//     through a list
func (i ScannerInfo) Holding() bool {
	return strings.HasSuffix(strings.TrimSpace(i.Mode), holdSuffix)
}

// List returns one of the scanner's lists as the XML document it reports.
//
// The document's shape depends on the list, and the scanner's database can be
// large, so this returns the document rather than a parsed type. Callers that
// need one list in particular should parse it into a type of their own.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - kind: which list to report, such as ListFavorites or ListSystems
//   - indexes: the indexes that narrow the list, in the order the list
//     expects; pass none for a list that needs none
//
// Returns:
//   - the XML document the scanner reports, unparsed
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) List(ctx context.Context, kind ListKind, indexes ...string) (string, error) {
	command := "GLT," + string(kind)
	if len(indexes) > 0 {
		command += "," + strings.Join(indexes, ",")
	}
	return s.conn.ExecuteXML(ctx, command)
}

// MenuBack goes up one level within the menus, without leaving them.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) MenuBack(ctx context.Context) error {
	return s.set(ctx, "MSB,")
}

// MenuInfo returns the menu the scanner is showing.
//
// It returns ErrNotInMenu when the scanner is not in a menu. Use OpenMenu or
// the menu key to put it in one.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - MenuInfo describing the menu on screen, carrying the document it was
//     parsed from in its XML field
//   - error if the exchange fails, the document cannot be parsed, or the
//     scanner is not in a menu
//
// Errors:
//   - ErrNotInMenu: if the scanner is not showing a menu
func (s *Scanner) MenuInfo(ctx context.Context) (MenuInfo, error) {
	doc, err := s.conn.ExecuteXML(ctx, "MSI")
	if err != nil {
		return MenuInfo{}, err
	}

	var info MenuInfo
	if err := xml.Unmarshal([]byte(doc), &info); err != nil {
		return MenuInfo{}, fmt.Errorf("parsing the response to %q: %w", "MSI", err)
	}
	if info.Type == menuTypeError {
		return MenuInfo{}, ErrNotInMenu
	}

	info.XML = doc
	return info, nil
}

// OpenMenu puts the scanner into one of its menus.
//
// The index selects which system, department, site, channel, or search bank
// the menu opens on, and is ignored by the menus that do not need one. Pass an
// empty string for those.
//
// Opening a menu takes the scanner out of scanning, and most other commands
// are refused while it is in one.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - menu: which menu to open, such as MenuTop or MenuSettings
//   - index: which system, department, site, channel, or search bank the menu
//     opens on, empty for the menus that do not need one
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) OpenMenu(ctx context.Context, menu MenuID, index string) error {
	return s.set(ctx, fmt.Sprintf("MNU,%s,%s", menu, index))
}

// ScannerInfo returns what the scanner is doing, parsed from the XML document
// it reports.
//
// This is the command to reach for when a caller needs to know what is being
// received rather than what is on the screen. It works in every mode, and
// reports the mode itself, so it also answers "why was my last command
// refused".
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - ScannerInfo describing what the scanner is doing, carrying the document
//     it was parsed from in its XML field
//   - error if the exchange fails or the document cannot be parsed
func (s *Scanner) ScannerInfo(ctx context.Context) (ScannerInfo, error) {
	doc, err := s.conn.ExecuteXML(ctx, "GSI")
	if err != nil {
		return ScannerInfo{}, err
	}

	var parsed struct {
		ScannerInfo
		MenuSummary namedEither `xml:"MenuSummary"`

		// The weather channels are reported as two sibling elements, which no
		// single field can hold, so they are read here and folded into one.
		WxMode struct {
			Mode string `xml:"Mode,attr"`
		} `xml:"WxMode"`
		WxChannel WeatherChannel `xml:"WxChannel"`
	}
	if err := xml.Unmarshal([]byte(doc), &parsed); err != nil {
		return ScannerInfo{}, fmt.Errorf("parsing the response to %q: %w", "GSI", err)
	}

	// The menu is read from the outer field rather than the embedded one, which
	// the decoder leaves empty whichever spelling arrived, and the capitalised
	// pair wins when the scanner sent both.
	info := parsed.ScannerInfo
	info.Menu = Named{Name: parsed.MenuSummary.Name, Index: parsed.MenuSummary.Index}
	if info.Menu.Name == "" && info.Menu.Index == "" {
		info.Menu = Named{Name: parsed.MenuSummary.NameLower, Index: parsed.MenuSummary.IndexLower}
	}

	parsed.WxChannel.Frequency = strings.TrimSpace(parsed.WxChannel.Frequency)
	info.Weather = Weather{Mode: parsed.WxMode.Mode, Channel: parsed.WxChannel}
	info.XML = doc
	return info, nil
}

// Scanning reports whether the scanner is working through its favorites lists,
// rather than sweeping something it was put in front of.
//
// This is a different question from Holding, which asks whether the scanner is
// moving at all. A scanner in Custom Search is moving, and is not scanning:
// Custom Search, Service Scan, Quick Search, Close Call and Tone-Out each sweep
// something of their own and stay there until told otherwise. Nothing takes the
// scanner out of one by accident, so anything promising to leave it scanning has
// to ask this as well.
//
// The weather channels are deliberately not counted, and are not left alone by
// accident either: they report "WX Scan", which is neither of these names.
//
// Returns:
//   - true if the scanner is working through its favorites lists, false in
//     every other mode
func (i ScannerInfo) Scanning() bool {
	mode := strings.TrimSpace(i.Mode)
	for _, name := range scanModes {
		if strings.EqualFold(mode, name) {
			return true
		}
	}
	return false
}

// SetMenuValue sets the value of the menu item the scanner is on.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - value: the value to write to the selected menu item
//
// Returns:
//   - error if the exchange fails or the scanner refuses the command
func (s *Scanner) SetMenuValue(ctx context.Context, value string) error {
	return s.set(ctx, "MSV,"+value)
}
