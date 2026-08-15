# Telling which layout is on screen

How the tool works out which of the seven screen layouts the scanner is drawing
with, why it is sometimes wrong, and what to try next.

Observed on an SDS150, firmware 1.00.37, 2026-08-06. One bug in here is fixed;
the other is open and is the reason this file exists.

The question sounds cosmetic and is not. "Is this layout the one on screen?"
decides whether the soft keys get positions at all. They are the only areas
missing from the built-in map, because their widths follow whatever labels the
current mode is showing, so they have to be read off the live screen and only
when the layout being reported is the one on it. Get the answer wrong and five
areas come back unplaced, which is a wrong answer wearing the costume of a
missing one.

See [colors.md](colors.md) for the colors themselves and
[screen-map.md](screen-map.md) for where every other area sits.

---

## How the answer is worked out

Two paths, depending on whether a layout was named.

With no layout named, `currentLayout` in
`source/internal/commands/colors/colors.go` asks what the scanner is doing.
`conventional_scan` and `trunk_scan` each name a _pair_ of layouts, one simple
and one detail; everything else names one. If it named a pair, the screen's rows
settle it, since simple draws 14 and detail 17, and that is a plain read which
opens nothing. If the count is neither,
`MENU -> Display Options -> Set Scan Display Mode` gets opened and its
highlighted entry read. That is the authority, and the only reason this path
ever touches a menu.

With a layout named, `isCurrent` in the same file, reached through `choose`,
runs the same three steps and turns them into a yes or no about the layout that
was named.

`candidates` wraps the first step and gives the screen up to three seconds to
settle, because [the scanner reports `plain_text` for a moment after being
moved](oddities.md). It deliberately does not wait when the screen is a menu, on
the reasoning that a menu will stay a menu and waiting would only delay a good
error message. That reasoning holds for the no-layout path, where the error is
`the scanner is in a menu, so it is not drawing with any layout`. It is also the
prime suspect for the open bug below.

---

## Fixed: the soft keys were read before the screen came back

*2026-08-06*

A full read of a layout walks the menus for about thirty seconds. It used to
read the live screen for the soft keys the instant it climbed out, at which
point the scanner is still drawing the menu it just left. The bottom row is not
three runs of reverse video yet, `softKeys` correctly declines to guess, and
`Soft1_key` through `Soft3_key` came back with no position.

The tell was a full `colors` and a `colors <layout> --cache` disagreeing about
the same layout on the same scanner seconds apart: the cached run placed the
soft keys and the full read did not. Any path that never enters a menu reads a
settled screen and gets them right, which is how this hid for so long behind
`--positions`.

Fixed by `settledSoftKeys`, which asks until the row looks like soft keys, on
the same budget `candidates` gives a screen to settle. A layout that genuinely
draws no soft keys spends that budget and still reports none, which is the right
answer arrived at slowly.

The lesson generalizes past this one bug. Anything reading the live screen right
after the tool has moved the scanner is reading a screen in transition, and
there is no "redraw finished" signal anywhere in the protocol to wait on. The
only defense is to ask until the answer looks like the thing you are asking
about.

---

## Open: a named layout is sometimes called not-current when it is

*2026-08-06*

Running `colors <layout>` or `colors <layout> --cache` shortly after another
command that moved the scanner sometimes reports `current: false` for the layout
that is in fact on screen, and so reports the five soft key areas as unplaced.

### What was seen

In one run of `TestColors`, a plain `colors` reported the soft keys at line 16
with lengths 9, 1, 10, 1, 9, and the two named reads that followed it seconds
later reported dashes for all five. Same layout, same scanner, same minute.

### How often

One failure in five runs of `TestColors`, which drives the commands back to back
with no pause. Nine targeted attempts to reproduce it in isolation all passed:
five pairs of `scan` then `colors detail-conventional --cache`, and four pairs of
`colors weather` then the same. Whatever the window is, it is narrow.

### Why it fails silently

`choose` reads:

```go
want, _ := lookup(wanted)
current, _ := isCurrent(ctx, client, want)
return want, current, nil
```

The error is dropped on purpose, because whether a named layout happens to be
the one in use is worth saying but not worth failing a read over. The cost of
that trade is that the one signal which would explain a wrong answer gets thrown
on the floor, and a scanner that could not be asked becomes indistinguishable
from one that answered no.

### The two candidate explanations

Both fit everything seen, and they need different fixes.

The first is that the screen was still reporting a menu. `candidates` returns
immediately with no candidates when the screen is `menu_selection`, `menu_input`
or `direct_entry`, and `isCurrent` reads no candidates as "the named layout is
not among them," which is false rather than unknown. The preceding command had
just climbed out of the menus, so the timing fits.

The second is that the row count was read mid-redraw. `byLineCount` takes a
single `Display()` and matches 14 or 17. A partially drawn screen that happens
to report 14 rows would confidently return `simple-conventional`, and `isCurrent`
would then answer no for `detail-conventional`. It declines safely on any _other_
count, so only the value 14 is dangerous, and only on a conventional or trunk
screen.

### What to try next

The first step decides which of the two it is, and is worth doing before writing
any fix.

Log what `isCurrent` actually saw. A debug line carrying the screen name and the
row count, then `TestColors -writes` in a loop until it fails, and read the log.
That single observation separates explanation 1 from 2.

If it is the menu screen, give `isCurrent` a patient variant of `candidates`
that waits through menu screens too. Safe there and only there: with a layout
named, a menu means "cannot tell yet" rather than "you are in a menu and here is
the advice." `currentLayout` has to keep failing fast so its error stays
immediate.

If it is the row count, require the count to be stable across two reads before
trusting it, or let `byLineCount` settle the way `candidates` does.

Either way, stop dropping the error. `isCurrent` failing and `isCurrent`
answering no should not look the same. Log it at minimum; better, let the report
distinguish "not the current layout" from "could not tell," since the second is
the case where the soft keys are missing rather than absent.

### How it would show up in the wild

Quietly, which is the worst way. A script reading `colors <layout> --positions`
back to back with anything else gets five areas with `length: 0` now and then,
and nothing says why. Redrawing the screen from that map leaves the soft key row
uncolored.

---

## Still to check

**Whether `currentLayout` has the same hole.** It waits through `plain_text` but
not through menus, and it is reached with no layout named, so a stale menu
screen produces the "you are in a menu" error rather than a wrong answer. That is
the better failure, and it is still wrong if the scanner has in fact left the
menu.

**How long the transition actually lasts.** Three seconds was picked as "enough"
for `plain_text` and never measured against a menu exit. A measured number would
let every settle loop in the command share one honest constant instead of a
guess that happens to work.

**Whether the row count is ever _stably_ neither 14 nor 17.** If it is not, the
menu walk in step 3 is dead code on this firmware, and the whole question turns
on a single `Display()` call.
