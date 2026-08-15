// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

// Package screen implements the "screen" command, which reports the scanner's
// display as text.
//
// It is the only view of the scanner that is always true. The menu commands
// report what the protocol says a menu holds, and that can omit entries which
// are really on screen and in the knob's path. It is also the only way to see
// a text input, which carries no menu entries at all.
package screen

import (
	"context"
	"fmt"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// New returns the screen command bound to app.
//
// Parameters:
//   - app: application context the command reads its settings, output streams
//     and scanner connection from
//
// Returns:
//   - *cobra.Command that reports the scanner's display when it runs
func New(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use: "screen",
		// Only looks at what the scanner is doing, so it may run while another
		// command has the radio. See appcontext.OnlyReads.
		Annotations: map[string]string{appcontext.OnlyReads: "true"},
		Short:       "Report the scanner's display as text",
		Long: "Screen reports what is on the scanner's display, line by line, marking the row\n" +
			"it has highlighted.\n\n" +
			"This is the most reliable view of the scanner. It works in every mode,\n" +
			"including menus and text inputs where other commands are refused or report\n" +
			"nothing, and it shows entries the menu commands can leave out.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), app)
		},
	}
}

// renderScreen writes the display for a person.
//
// Parameters:
//   - app: application context the rows are written through
//   - r: the screen as this command renders it
//   - d: the display the report was built from, which the text output does not
//     read
//
// Returns:
//   - error, always nil, because writing the rows cannot fail
func renderScreen(app *appcontext.App, r report, d device.Display) error {
	for _, l := range r.Lines {
		marker := "  "
		if l.Highlighted {
			marker = "> "
		}
		app.Printf("%s%s\n", marker, l.Text)
	}

	if len(r.Lines) == 0 {
		app.Notef("The scanner reported an empty display.\n")
	}
	return nil
}

// run reads the display and renders it.
//
// Parameters:
//   - ctx: context for cancellation and timeouts while talking to the scanner
//   - app: application context holding the scanner connection, the output
//     format and the streams to write to
//
// Returns:
//   - error if the scanner cannot be reached, the screen cannot be read, or the
//     JSON cannot be written; nil once the display has been reported
func run(ctx context.Context, app *appcontext.App) error {
	client, err := app.Device(ctx)
	if err != nil {
		return err
	}

	d, err := client.Display(ctx)
	if err != nil {
		return fmt.Errorf("reading the screen: %w", err)
	}

	var r report
	for i, l := range d.Lines {
		r.Lines = append(r.Lines, line{
			// Trailing spaces pad the display to its width and mean nothing.
			Text:        strings.TrimRight(l.Text, " "),
			Highlighted: l.Selected(),
			Attributes:  l.Attributes,
			// The scanner reports one flag per line and the device layer keeps
			// them in a slice of their own, so this is indexed rather than read
			// off the line. Both come from the same reply and are the same
			// length, which parseDisplay guarantees by building them together.
			LargeFont: d.LargeFont[i],
		})
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, widen(r))
	}

	return renderScreen(app, r, d)
}

// widen reports r with every byte of its text carried as the character of the
// same value, so that JSON can hold it.
//
// The screen is bytes, not text. Its font has pictures above 0x7E, and the
// scanner puts them in a line like any other character: the signal meter arrives
// as 0xAC and 0xAD. A Go string holding those raw bytes is not valid UTF-8, and
// encoding/json replaces every invalid byte with U+FFFD *and reports no error*,
// so 0xAC and 0xAD both leave as the same character and the reading is silently
// destroyed. Widening each byte to the character of the same value is lossless,
// is valid UTF-8, and leaves ASCII exactly as it was, so a reader that never saw
// a picture cannot tell the difference.
//
// Only the JSON is widened. device.Line keeps the raw bytes because the device
// layer decides what is an attribute row by comparing the byte lengths of the
// two fields, and a widened string is longer than the line it came from.
//
// Parameters:
//   - r: the report to widen, which is left as it was
//
// Returns:
//   - report holding the same rows with every byte of their text carried as the
//     character of the same value
func widen(r report) report {
	out := report{Lines: make([]line, len(r.Lines))}
	copy(out.Lines, r.Lines)

	for i := range out.Lines {
		out.Lines[i].Text = widenString(out.Lines[i].Text)
		// Attributes are one of three ASCII characters, so this is the identity
		// for every reading the scanner can produce. It is here so that the two
		// fields cannot diverge if that ever stops being true.
		out.Lines[i].Attributes = widenString(out.Lines[i].Attributes)
	}
	return out
}

// widenString reports s with each byte as the character of the same value.
//
// Parameters:
//   - s: the string to widen
//
// Returns:
//   - string holding one character per byte of s, or s itself when every byte
//     is already ASCII
func widenString(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
}
