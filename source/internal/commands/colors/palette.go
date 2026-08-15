// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package colors

import (
	"fmt"
	"text/tabwriter"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/render"
	"github.com/spf13/cobra"
)

// This file is the scanner's palette: every color its pickers offer, in the
// order the knob steps through them, and the command that prints it.
//
// It is a table for the same reason the screen map is. The palette is the
// firmware's and no menu changes it, so asking the scanner would mean walking
// the whole ring to answer a question whose answer cannot change. Having it
// here lets a name be checked before the scanner is touched, and lets the walk
// work out the shortest way round rather than stepping blind.
//
// Walked off an SDS150 on firmware 1.00.37, 2026-08-05, by stepping one
// picker from end to end and reading each color off the screen. The list
// **wraps**: stepping past the last color lands on the first, which is why the
// walk can choose a direction and no color is more than half a ring away.
//
// The values are the scanner's own and are not quite the CSS values the names
// come from. Its Orangered is #FF4600 where CSS says #FF4500, and its
// Darksalmon is #E79473 where CSS says #E9967A. The display quantizes color:
// there are 54 distinct levels per channel across the whole palette. So these
// names are labels for the scanner's colors, not for the web's, and the values
// here are what the scanner reports rather than what the name means elsewhere.
//
// Nothing counts steps against this table without checking: the walk reads the
// color off the screen and compares by name before it commits, so a firmware
// with a different palette fails to find its target rather than writing the
// wrong color.

// palette is every color a picker offers, in the knob's order, which is
// alphabetical. Stepping right moves down this list and wraps at the end.
var palette = []paletteColor{
	{"Aliceblue", "#EFF7FF"},
	{"Antiquewhite", "#F7EBD6"},
	{"Aqua", "#00FBF7"},
	{"Aquamarine", "#7BFFCE"},
	{"Azure", "#EFFFFF"},
	{"Beige", "#EFF3D6"},
	{"Bisque", "#FFE3BD"},
	{"Black", "#000000"},
	{"Blanchedalmond", "#FFEBC6"},
	{"Blue", "#0000FF"},
	{"Blueviolet", "#8429DE"},
	{"Brass", "#B5A542"},
	{"Brown", "#A52929"},
	{"Burlywood", "#D6B584"},
	{"Cadetblue", "#5A9C9C"},
	{"Chartreuse", "#7BFF00"},
	{"Chocolate", "#CE6718"},
	{"Coolcopper", "#D68418"},
	{"Copper", "#BD00DE"},
	{"Coral", "#FF7F4A"},
	{"Cornflower", "#BDEFDE"},
	{"Cornflowerblue", "#6390E7"},
	{"Cornsilk", "#FFF7D6"},
	{"Crimson", "#D61039"},
	{"Cyan", "#00FFFF"},
	{"Darkblue", "#000084"},
	{"Darkbrown", "#D60800"},
	{"Darkcyan", "#008884"},
	{"Darkgoldenrod", "#B58408"},
	{"Darkgray", "#A5A5A5"},
	{"Darkgreen", "#006300"},
	{"Darkkhaki", "#B5B56B"},
	{"Darkmagenta", "#840084"},
	{"Darkolivegreen", "#526B29"},
	{"Darkorange", "#FF8800"},
	{"Darkorchid", "#9431C6"},
	{"Darkred", "#840000"},
	{"Darksalmon", "#E79473"},
	{"Darkseagreen", "#8CB98C"},
	{"Darkslateblue", "#423D84"},
	{"Darkslategray", "#294E4A"},
	{"Darkturquoise", "#00CACE"},
	{"Darkviolet", "#8C00CE"},
	{"Deeppink", "#FF108C"},
	{"Deepskyblue", "#00BDFF"},
	{"Dimgray", "#636763"},
	{"Dodgerblue", "#188CFF"},
	{"Feldsper", "#F7CEDE"},
	{"Firebrick", "#AD2121"},
	{"Floralwhite", "#FFF7EF"},
	{"Forestgreen", "#218821"},
	{"Fuchsia", "#F700F7"},
	{"Gainsboro", "#D6DAD6"},
	{"Ghostwhite", "#F7F7FF"},
	{"Gold", "#FFD600"},
	{"Goldenrod", "#D6A118"},
	{"Gray", "#7B7F7B"},
	{"Green", "#007F00"},
	{"Greenyellow", "#ADFF29"},
	{"Honeydew", "#EFFFEF"},
	{"Hotpink", "#FF67AD"},
	{"Indianred", "#C65A5A"},
	{"Indigo", "#4A007B"},
	{"Ivory", "#FFFFEF"},
	{"Khaki", "#EFE38C"},
	{"Lavender", "#DEE3F7"},
	{"Lavenderblush", "#FFEFEF"},
	{"Lawngreen", "#7BFB00"},
	{"Lemonchiffon", "#FFF7C6"},
	{"Lightblue", "#ADD6DE"},
	{"Lightcoral", "#EF7F7B"},
	{"Lightcyan", "#DEFFFF"},
	{"Lightgoldenrodyellow", "#F7F7CE"},
	{"Lightgreen", "#8CEB8C"},
	{"Lightgrey", "#CED2CE"},
	{"Lightpink", "#FFB1BD"},
	{"Lightsalmon", "#FF9C73"},
	{"Lightseagreen", "#18ADA5"},
	{"Lightskyblue", "#84CAF7"},
	{"Lightslategray", "#738494"},
	{"Lightsteelblue", "#ADC2D6"},
	{"Lightyellow", "#FFFFDE"},
	{"Lime", "#00FF00"},
	{"Limegreen", "#31CA31"},
	{"Linen", "#F7EFDE"},
	{"Magenta", "#FF00FF"},
	{"Maroon", "#7B0000"},
	{"Mediumaquamarine", "#63CAA5"},
	{"Mediumblue", "#0000C6"},
	{"Mediumorchid", "#B556CE"},
	{"Mediumpurple", "#8C6FD6"},
	{"Mediumseagreen", "#39B16B"},
	{"Mediumslateblue", "#7367E7"},
	{"Mediumspringgreen", "#00F794"},
	{"Mediumturquoise", "#42CEC6"},
	{"Mediumvioletred", "#C61484"},
	{"Midnightblue", "#18186B"},
	{"Mintcream", "#EFFFF7"},
	{"Mistyrose", "#FFE3DE"},
	{"Moccasin", "#FFE3B5"},
	{"Navajowhite", "#FFDAAD"},
	{"Navy", "#00007B"},
	{"Oldlace", "#F7F3DE"},
	{"Olive", "#7B7F00"},
	{"Olivedrab", "#6B8C21"},
	{"Orange", "#FFA100"},
	{"Orangered", "#FF4600"},
	{"Orchid", "#D66FD6"},
	{"Palegoldenrod", "#E7E7A5"},
	{"Palegreen", "#94FB94"},
	{"Paleturquoise", "#ADEBE7"},
	{"Palevioletred", "#D66F8C"},
	{"Papayawhip", "#FFEFCE"},
	{"Peachpuff", "#FFD6B5"},
	{"Peru", "#C68039"},
	{"Pink", "#FFBDC6"},
	{"Plum", "#D69CD6"},
	{"Powderblue", "#ADDEDE"},
	{"Purple", "#7B007B"},
	{"Red", "#FF0000"},
	{"Richblue", "#08ADDE"},
	{"Rosybrown", "#B58C8C"},
	{"Royalblue", "#3967DE"},
	{"Saddlebrown", "#844610"},
	{"Salmon", "#F77F6B"},
	{"Sandybrown", "#EFA15A"},
	{"Seagreen", "#298852"},
	{"Seashell", "#FFF3E7"},
	{"Sienna", "#9C5229"},
	{"Silver", "#BDBDBD"},
	{"Skyblue", "#84CAE7"},
	{"Slateblue", "#635AC6"},
	{"Slategray", "#6B7F8C"},
	{"Snow", "#FFF7F7"},
	{"Springgreen", "#00FF7B"},
	{"Steelblue", "#4280AD"},
	{"Tan", "#CEB18C"},
	{"Teal", "#007F7B"},
	{"Thistle", "#D6BDD6"},
	{"Tomato", "#FF6342"},
	{"Turquoise", "#39DECE"},
	{"Violet", "#E780E7"},
	{"Wheat", "#EFDAAD"},
	{"White", "#FFFFFF"},
	{"Whitesmoke", "#EFF3EF"},
	{"Yellow", "#FFFF00"},
	{"Yellowgreen", "#94CA31"},
}

// newPalette returns the "colors palette" subcommand.
//
// Parameters:
//   - app: application context the command reads its output format and
//     streams from
//
// Returns:
//   - *cobra.Command that lists every color the scanner offers when it runs
func newPalette(app *appcontext.App) *cobra.Command {
	return &cobra.Command{
		Use:   "palette",
		Short: "List every color the scanner offers",
		Long: "Palette lists all " + fmt.Sprint(len(palette)) + " colors the scanner's pickers offer, in the order its\n" +
			"knob steps through them.\n\n" +
			"This is a built-in table rather than a reading. The palette is the firmware's\n" +
			"and no menu changes it, so asking the scanner would mean walking a picker end\n" +
			"to end to be told what is written here. That makes this instant, and the one\n" +
			"command here that needs no scanner at all.\n\n" +
			"The order is the knob's, which is alphabetical, and it wraps: stepping right\n" +
			"past the last color lands on the first, so no color is more than half a ring\n" +
			"away from any other.\n\n" +
			"The names are CSS color names but the values are not quite the CSS values.\n" +
			"The scanner's Orangered is #FF4600 where CSS says #FF4500, and its Darksalmon\n" +
			"is #E79473 where CSS says #E9967A. A name here is a label for the scanner's\n" +
			"color, not for the web's.\n\n" +
			"An arbitrary value cannot be set: the picker has nothing to type one into, so\n" +
			"these are the whole vocabulary. Pass one of these names to\n" +
			"\"radiocli colors set\". Run \"radiocli colors --verify-palette\" to check this\n" +
			"table against the scanner in front of you.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPalette(app)
		},
	}
}

// runPalette renders the built-in palette.
//
// Parameters:
//   - app: application context the table is written through, and whose output
//     format decides between the table and the JSON
//
// Returns:
//   - error if the table cannot be written or the JSON cannot be encoded; nil
//     once every color has been listed
func runPalette(app *appcontext.App) error {
	r := paletteList{Count: len(palette), Colors: make([]paletteEntry, 0, len(palette))}
	for i, c := range palette {
		r.Colors = append(r.Colors, paletteEntry{Step: i, Name: c.Name, Hex: c.Hex})
	}

	if app.Config.Output == appcontext.OutputJSON {
		return render.JSON(app.Stdout, r)
	}

	// Two spaces of padding, the same as the other tables here.
	w := tabwriter.NewWriter(app.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STEP\tNAME\tHEX")
	for _, c := range r.Colors {
		fmt.Fprintf(w, "%d\t%s\t%s\n", c.Step, c.Name, c.Hex)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the palette: %w", err)
	}

	app.Printf("\n%d colors\n", r.Count)
	return nil
}
