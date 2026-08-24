// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package device

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
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

	// The scanner writes a frequency with a leading space, and writes the words
	// "TGID None" and "UID None" rather than leaving an identifier out. All of
	// it is tidied here so that everything above this sees an absent value as
	// empty and a present one as the bare number, which are the two spellings
	// worth having. The trunked TGID element carries the same prefixed forms as
	// the conventional one: a live P25 capture arrived as TGID="TGID:10003",
	// and the first version of this block missed it, so a night of trunked
	// recordings was labelled with the scanner's spelling rather than the
	// number.
	info.Frequency.Frequency = strings.TrimSpace(info.Frequency.Frequency)
	info.SiteFrequency.Frequency = strings.TrimSpace(info.SiteFrequency.Frequency)
	info.Frequency.Talkgroup = present(info.Frequency.Talkgroup)
	info.Frequency.UnitID = present(info.Frequency.UnitID)
	info.Talkgroup.ID = present(info.Talkgroup.ID)
	info.Talkgroup.UnitID = present(info.Talkgroup.UnitID)
	info.Unit.ID = present(info.Unit.ID)

	info.XML = doc
	return info, nil
}

// Decoding reports the digital format the scanner is decoding, or empty when
// the transmission is analog.
//
// The scanner writes "None" rather than leaving the attribute out, in the same
// spirit as the "UID None" the unit id uses, so absent is turned into empty
// here and everything above sees one spelling of nothing.
//
// Returns:
//   - the format, such as "P25" or "DMR", or empty for analog
func (p Property) Decoding() string {
	if v := strings.TrimSpace(p.Digital); v != "" && !strings.EqualFold(v, "None") {
		return v
	}
	return ""
}

// Receiving reports whether a signal is actually coming in.
//
// The number of bars is the reading to trust. The raw strength figure is
// reported even when nothing is coming in, as the noise floor, so a value there
// says only that the scanner answered: a scanning radio with nothing on the
// channel reports a strength of -999 and no bars at all.
//
// Returns:
//   - true if the scanner is showing at least one signal bar
func (p Property) Receiving() bool {
	bars, err := strconv.Atoi(strings.TrimSpace(p.Signal))
	return err == nil && bars > 0
}

// Heard flattens what the scanner is listening to into one value.
//
// Returns:
//   - what the scanner is hearing, with everything it did not name left empty
func (i ScannerInfo) Heard() Heard {
	name, value, unit := i.Tuned()

	h := Heard{
		Receiving:  i.Property.Unmuted(),
		List:       i.List.Name,
		System:     i.System.Name,
		Department: i.Department.Name,
		Site:       i.Site.Name,
		Channel:    name,
		Unit:       unit,
		Modulation: i.Frequency.Modulation,
		Digital:    i.Property.Decoding(),
		Signal:     i.Property.Signal,
		RSSI:       i.Property.RSSI,
		Mode:       i.Mode,
	}

	// A trunked call carries both: the talkgroup from the element above, and
	// the voice channel the site handed out, which lives on a different element
	// and is the frequency the radio's own screen shows. The modulation comes
	// from the site for the same reason, since there is no ConvFrequency to
	// take it from.
	if i.Talkgroup.ID != "" {
		h.Frequency = i.SiteFrequency.Frequency
		h.Modulation = i.Site.Modulation
		h.NAC = nac(i.SiteFrequency)
	}

	// A conventional system answers with a frequency and a trunked one with a
	// talkgroup, and they are not the same kind of number, so they are reported
	// in fields of their own rather than in one meaning whichever arrived.
	if i.Talkgroup.ID != "" {
		h.Talkgroup = value
	} else {
		h.Frequency = value
	}
	return h
}

// Unmuted reports whether the scanner's audio gate is open, meaning sound is
// coming out of it right now.
//
// This is a different question from Receiving, and the difference matters to
// anything lining the radio up against its audio. The gate opens at the very
// start of a transmission, before the signal reading has caught up: a document
// captured on the first poll of a transmission read Mute="Unmute" with Sig="0",
// and the next one read Sig="5" on the same unchanged signal. Waiting for bars
// therefore misses the opening of every transmission, which is the part hardest
// to get back.
//
// Returns:
//   - true if the scanner is passing audio through
func (p Property) Unmuted() bool {
	return strings.EqualFold(strings.TrimSpace(p.Mute), "Unmute")
}

// Tuned returns what the scanner is on, whichever kind of thing that is.
//
// A conventional system reports a ConvFrequency and a trunked one a TGID, and
// they are different documents describing the same idea. Callers that only want
// to know what is being listened to should ask here rather than testing both,
// which is the check that gets written once per caller and wrong in half of
// them.
//
// Returns:
//   - the channel's name, empty if the scanner is not on one
//   - what it is tuned to, which is a frequency such as "155.235000MHz" on a
//     conventional system and a talkgroup number on a trunked one
//   - the radio heard transmitting, empty when none was decoded
func (i ScannerInfo) Tuned() (name, value, unit string) {
	if i.Talkgroup.ID != "" {
		// The unit comes from the element beside the talkgroup rather than
		// from the talkgroup itself. A real SDS150 has never populated the
		// attribute, and reading only that reported no transmitting radio on
		// any trunked call ever recorded. The attribute is still read, because
		// the specification describes it and another model may use it.
		return i.Talkgroup.Name, i.Talkgroup.ID, either(i.Unit.ID, i.Talkgroup.UnitID)
	}
	if i.Frequency.Frequency != "" {
		// A conventional channel can be digital too. P25 without trunking is
		// still P25, it still names the transmitting radio, and a channel
		// carrying it looks like any other conventional one here: the document
		// reports a frequency rather than a talkgroup, and Mod reports whatever
		// the demodulator settled on rather than how the channel is programmed.
		// So the element is read on this side as well, rather than assuming
		// that a frequency means analog and cannot have a radio behind it.
		return i.Frequency.Name, i.Frequency.Frequency, either(i.Frequency.UnitID, i.Unit.ID)
	}
	// Neither element named what the scanner is on. The radio may still have
	// decoded a unit, so it is reported rather than dropped for want of a
	// channel to attach it to.
	return i.Channel.Name, "", i.Unit.ID
}

// either returns the first of two readings that says anything.
//
// The transmitting radio arrives in one of two places depending on the document
// and, as far as anything here knows, on the model. Rather than choose, both are
// read and whichever is filled in is used.
//
// Parameters:
//   - first: the reading to prefer
//   - second: the reading to fall back on
//
// Returns:
//   - first when it is not empty, otherwise second
func either(first, second string) string {
	if first != "" {
		return first
	}
	return second
}

// nac reads the network access code out of a site's sub-audio fields.
//
// The scanner writes it as "NAC 8A1h", in the same fields a conventional
// channel uses for CTCSS and DCS, so what is wanted is the part after the name
// and only when the name is the P25 one. Anything else there is a tone or a
// code for some other mode, and reporting one of those as a NAC would be
// worse than reporting nothing.
//
// The decoded field is preferred over the setting. They usually agree, and when
// they do not it is because the site is programmed for a code the radio has not
// heard yet, in which case what arrived is the honest answer.
//
// Parameters:
//   - f: the site frequency element, as the scanner sent it
//
// Returns:
//   - the code alone, such as "8A1h", or empty when there is not one
func nac(f SiteFrequency) string {
	for _, value := range []string{f.SubAudioDecoded, f.SubAudio} {
		value = strings.TrimSpace(value)
		if rest, found := strings.CutPrefix(value, "NAC "); found {
			if code := strings.TrimSpace(rest); code != "" {
				return code
			}
		}
	}
	return ""
}

// present turns the scanner's several ways of writing an identifier into
// either the identifier or nothing.
//
// The radio does not simply leave a talkgroup or a unit id out when it has not
// decoded one, and it does not write them plainly when it has. Read off an
// SDS150 on firmware 1.00.37:
//
//	TGID="TGID None"     conventional channel, nothing decoded
//	TGID="TGID: ---"     trunked, waiting
//	TGID="TGID:10003"    trunked, receiving
//	U_Id="UID None"      nothing decoded
//
// So every value carries the name of the field in front of it, absence is
// spelled two different ways depending on the mode, and the separator is a
// space in one and a colon in the other. Anything comparing against the empty
// string is wrong about all four, and anything stripping only the word "None"
// reports a talkgroup of "TGID: ---" while the scanner sits there waiting.
//
// The active form of the unit id was not observed, since no digital traffic
// came through while this was being written. It is handled by the same rules
// rather than by a pattern guessed for it.
//
// Parameters:
//   - value: the attribute as the scanner wrote it
//
// Returns:
//   - the identifier alone, or empty if the scanner has not decoded one
func present(value string) string {
	value = strings.TrimSpace(value)

	// "TGID:10003" and "TGID: ---" both put the field name in front.
	if _, after, found := strings.Cut(value, ":"); found {
		value = strings.TrimSpace(after)
	}

	// "TGID None" and "UID None" say it the other way round, and a run of
	// dashes is how the trunked side writes the same thing.
	if value == "" || strings.HasSuffix(value, "None") || strings.Trim(value, "-") == "" {
		return ""
	}
	return value
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
