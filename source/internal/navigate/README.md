# navigate

## What this does?
This package drives the scanner through its own on-screen menus to reach a particular place in its memory, such as the channels inside one department. It is the difference between knowing how to press buttons on a menu and knowing which menus to walk through to get somewhere.

## Why we use it?
Editing anything on the scanner means being on the right screen first. Creating a channel is a matter of pressing keys once the radio is showing that department's channel list, but getting to that list means opening the top menu, finding the favorites lists, picking one, stepping into its systems, picking one of those, stepping into its departments, picking one of those, and only then asking for the channels. Every one of those steps is a menu entry with a particular name, and none of the commands that need to make the walk should have to remember the sequence or the spelling.

It also owns the reads that have to be complete. The scanner caps a list reply at about a kilobyte and offers no way to ask for the rest, so `catalog` can only report a long list as a prefix and an admission. This package has what makes up the difference: it already knows how to put the radio on the menu showing that same list, and the knob reaches every entry whether or not the protocol will report it. So `navigate.ReadChannels` and its siblings ask the protocol first, and walk the menus only when the scanner has said the answer was short. Anything that needs a whole list reads it here rather than from `catalog`.

Keeping the walk in its own package means every command reaches a given screen the same way, by the same route, rather than each one carrying a slightly different copy of the sequence that drifts as firmware changes. It also gives the tool somewhere to put the rules the walk depends on: that menu entries are matched by name rather than by position, so a firmware that reorders a menu still works and no walk can press the entry next to the one it wanted; that a walk starts from the top menu rather than from wherever the radio happens to be; and that a name is resolved into an index before the scanner is touched, so asking for something that does not exist costs nothing. It is a separate package rather than a shared helper inside one command because more than one command needs the same walk, and commands must not import one another.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/navigate"

// Put the scanner on the channel list of one department. The department can be
// named or given by index, and the walk down to it happens along the way.
if err := navigate.ToChannels(ctx, client, department); err != nil {
    return err
}

// The scanner is now showing that department's channels, so the menu can be
// read or edited from here.

// Read a list that has to be complete. This is one exchange when the list fits,
// and a walk down the menus when the scanner says it did not.
channels, err := navigate.ReadChannels(ctx, client, department)
if err != nil {
    return err
}
for _, c := range channels {
    // Partial marks an entry found on the menus, whose name is all that is known.
    fmt.Printf("%s %s\n", c.Name, c.Frequency)
}

// Some walks report what they landed on, because the caller needs it afterwards.
list, err := navigate.ToSystems(ctx, client, "Home")
if err != nil {
    return err
}
fmt.Printf("creating a system in %s\n", list.Name)
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Menu tree** - The shape of the scanner's menus, which is what decides the order of every walk in this package
- **Favorites list, system, department, channel** - The nesting the scanner stores its memory in, and why reaching a channel means passing through all three levels above it
- **Trunking site** - Why sites hang off a system beside its departments, and why a conventional system has no Edit Site entry to find
- **Name versus index resolution** - Why a walk turns a name into an index before touching the radio, and why an index can be taken straight through
- **Import cycles in Go** - Why a walk two commands share has to live in a package of its own rather than in either command
