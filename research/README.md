# research

## What this does?
Holds the reverse-engineering notes behind `radiocli`: how the scanner's serial protocol, screen layout, colors, font and menu tree were worked out from the outside. It is the record of what the radio actually does, as opposed to what its published specification says it does.

## Why we use it?
The SDS150 is not documented for this. Uniden publishes a remote command specification, and the radio disagrees with it in places: commands that are listed and refused, fields that report twenty of fifty areas, colors that exist nowhere in the protocol and have to be read off the scanner's own screen one menu at a time. Every one of those was a day of somebody's life to find. Written down, it is a fact anybody can build on; left in the code, it is a magic constant nobody dares touch.

The notes live apart from `source/` because they outlast it. A finding about firmware 1.00.37 is true whether or not this tool still parses it that way, and it is useful to somebody writing a different tool in a different language. Each file pairs a reference document with a blog post: the reference is the tables and the measurements, and the post is the story of getting them, which is where the reasoning lives. Every file states the radio and firmware it was read from, and marks a guess as a guess, because the whole value of the folder is that a reader can tell measurement from inference.

## How we use it?
One reference document per subject, each stating the radio and firmware it was read from:

| Document | Covers |
| --- | --- |
| [protocol.md](protocol.md) | What the serial port will and will not do |
| [screen-map.md](screen-map.md) | Where every element of the display sits |
| [colors.md](colors.md) | The palette, and reading it off the glass |
| [glyphs.md](glyphs.md) | All 256 character codes, as pixel grids |
| [menu-tree.md](menu-tree.md) | Every screen reachable from the MENU key |
| [layout-detection.md](layout-detection.md) | Which of the seven layouts is on screen |
| [oddities.md](oddities.md) | The running log of surprises |

[screen-map.md](screen-map.md) plus [colors.md](colors.md) is enough to redraw the scanner's display faithfully somewhere else, and [glyphs.md](glyphs.md) is the character shapes that redrawing needs.

## Further reading
- **Reverse engineering** - Working a system out from its observable behaviour, and why the radio wins when it disagrees with its own specification
- **Trunked radio** - Motorola, P25, DMR and NXDN systems, which is what makes the scanner's screens mean several different things
- **Firmware versioning** - Why every file names the unit and version it was read from, and what that costs a claim that has never been checked against a second radio
- **Screen scraping** - Reading the answer off the display because the protocol refuses to say, which is how the colors were obtained
- **Bitmap fonts** - Fixed pixel grids per character code, and why the codes below 0x20 and above 0x7E are pictures rather than text
