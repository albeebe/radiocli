# catalog

## What this does?
This package reads the lists a scanner keeps inside it, such as its favorites lists, the systems and departments in them, and the channels those hold. It turns the raw documents the radio sends back into ordinary values the rest of the tool can work with, and catches the case where the radio answers with the wrong list entirely.

## Why we use it?
The scanner stores everything it can listen to as a set of nested lists, and it reports each one as an XML document over the serial cable. Those documents are not interchangeable: a favorites list, a system, a department, and a channel each carry different attributes, and the database behind them can hold thousands of entries. Worse, the firmware has a habit of refusing a request it cannot satisfy by answering with a document of a completely different kind while reporting success, so code that asks for channels can be handed favorites lists and never notice. Something has to turn these documents into typed values and refuse the ones that answer a different question than the one that was asked.

Keeping this in its own package means every command reads the scanner's database the same way, through one set of functions, instead of each one growing its own parser and its own idea of what a department is. It also gives the tool a single place for the three rules that are easy to get wrong and expensive to relearn: that a name has to be resolved into an index by searching, because an index carries no hint of what holds it; that the built-in scan sources must never be walked, because asking the full RadioReference database for its systems floods the serial port and corrupts every exchange that follows; and that a list reply is capped at about a kilobyte, so a long list arrives as a prefix with a footer admitting it and no way to ask for the rest.

That third rule is why `ErrIncomplete` exists. A truncated list is returned as the rows that did arrive **plus** that error, because the rows are good and throwing them away helps nobody, but a caller who ignores the error reports a short list as though it were the whole thing. That was a real bug: forty channels were created, seven were listed back, and nothing anywhere said so. Anything that needs a list to be complete should read it through `navigate`, which fills in what was cut by walking the scanner's menus.

The name-or-index lookup lives here for the same reason. `ResolveSystem`, `ResolveDepartment`, `ResolveSite` and `ResolveFavorites` are each four lines, and four command packages carried seven copies of them between them, three of `ResolveSystem` alone. They belong here rather than in any one command because the expensive half of the decision is a catalog question: an index is answered without reading anything, and for a department name the read is every list, every system and every department on the scanner.

That second rule is enforced here rather than left to callers. `ReadSystems` refuses a built-in source before it sends anything, by reserved index, so the name and the index are refused by the same check and no command above it can reach the scanner without passing through. The full database's answer leaves the serial interface dead until the radio is power cycled, reproduced twice, which is why the refusal sits at the point where the request would go on the wire instead of in the command that happens to ask today.

## How we use it?
```go
import "github.com/albeebe/radiocli/internal/catalog"

// Read the scanner's favorites lists.
// A user can name a list by name or by index. ResolveFavorites turns either
// into an index, and reads nothing at all when it is already one. There is a
// sibling for each kind: ResolveSystem, ResolveDepartment and ResolveSite.
index, err := catalog.ResolveFavorites(ctx, client, name)
if err != nil {
    return err
}

// Walk down into the systems that list holds. This is one exchange, and it can
// come back short: check for ErrIncomplete, or read it through navigate, which
// finishes the list off the scanner's menus.
systems, err := catalog.ReadSystems(ctx, client, index)
if err != nil && !errors.Is(err, catalog.ErrIncomplete) {
    return err
}

for _, system := range systems {
    fmt.Printf("%s (%s)\n", system.Name, system.Kind)
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
- **Conventional and trunked systems** - The two ways a radio system is organised, and why a department holds frequencies on one and talkgroups on the other
- **Talkgroup** - The address that stands in for a channel on a trunked system, which is why a channel here carries either a frequency or a talkgroup
- **Trunking site** - The transmitter a trunked system speaks through, and why sites sit beside departments rather than inside them
- **XML parsing with encoding/xml** - How the scanner's documents are walked token by token, which is what allows one parser to serve every list shape
- **Truncated protocol replies** - Why a list reply stops at about a kilobyte, how the footer says so, and why there is no way to ask for the second part
- **Generics and type constraints** - How Resolve matches a name against favorites lists, systems, departments, or sites without a copy of itself for each
