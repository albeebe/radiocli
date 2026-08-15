# textinput

## What this does?
This package types a word or a number into one of the scanner's on-screen text boxes, such as the one that appears when renaming a favorites list. It does it the same way a person would, by turning the knob and pressing keys, because the radio offers no command for setting text directly.

## Why we use it?
The scanner will not be told what to put in a text box. Its own command for setting a value is refused on these screens, and so are the character keys, so the only way a name gets in is one character at a time: turn the knob until the character under the cursor is the right one, press a key to move along, and repeat to the end. Working that out by hand is fiddly and easy to get wrong, because the knob does not move the way the radio's own reported character set suggests, and a press the scanner misses leaves a silently wrong name behind.

Keeping this in its own package means every command that has to name something reuses one careful implementation instead of each growing its own. That matters because the awkward parts were found by experiment rather than read in a manual: the space sits at the front of the cycle even though the scanner lists it last, a cursor moved past the end of a value reports a space that is not really there, and a screen that accepts only digits has to be typed on the keypad instead of turned to. Each character is read back after it is set, so a missed press is caught at the position it happened rather than discovered as a wrong name afterwards.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/textinput"

// The scanner must already be showing a text entry screen; how it got
// there depends on what is being renamed.
if err := textinput.Set(ctx, client, "Fire Dispatch"); err != nil {
    return err
}
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Screen scraping** - Driving a device through the interface built for a person, which is what this package does and why every step is verified
- **Idempotence** - Why setting a value that is already correct still accepts the screen, so the caller ends up in the same place either way
- **Modular arithmetic** - How the shorter way round a wrapping character cycle is worked out, and which direction to turn the knob
- **Retry with re-observation** - Re-reading the screen and re-planning after every attempt, rather than counting presses and hoping
- **Fail fast validation** - Refusing a value the screen cannot hold before any key is pressed, so a rejected name never lands half-typed
