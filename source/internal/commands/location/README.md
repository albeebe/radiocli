# location

## What this does?
This package is the `location` command: it reports the position the scanner is working from, and the radius around it that the scanner draws channels out of its database. It also sets that position, either from a zip code the scanner looks up itself or from a latitude and longitude written straight to it, and switches the built-in GPS on and off.

## Why we use it?
A scanner with a database has to know where it is before that database is any use, because what it offers is everything within a radius of a point. Left alone the scanner follows its own GPS, and the position drifts by a metre or two between readings while the receiver refines its fix. A zip code entered through the menus replaces that position outright and keeps it replaced across a power cycle, because entering one also switches the GPS function off. So the number worth reporting is not where the scanner is, it is the position the scanner is scanning around, and those two are only the same thing when the receiver is in charge.

Keeping this as its own command puts the awkward parts in one place. The zip database lives on the scanner and nothing else can turn 12345 into a position, so setting a zip means driving the scanner's own menus, which stops it scanning and raises prompts in an order that has to be answered as it comes: the full database prompt appears first, and only once it is answered does the scanner look the zip up and get the chance to refuse it as out of range. The range screen cannot be driven through the ordinary text entry either, because each digit overwrites the character under a cursor rather than appending, which is why the range is whole miles and why it is read back before it is saved. Writing a latitude and longitude directly avoids all of that, and is the only way back to a position that came from a zip code, since the scanner keeps the result and no record of the zip it was given.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	location.New,
}
```

```bash
# Report the position the scanner is working from.
radiocli --device $SDS location

# Point it at a zip code, which the scanner resolves and which switches the GPS off.
radiocli --device $SDS location set 24944 --range 25

# Put a position back by hand, then hand the scanner back to its receiver.
radiocli --device $SDS location set --position 38.433056,-79.839722
radiocli --device $SDS location gps --wait
```

### Testing

```bash
# Run tests with coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Verify 100% coverage (fails if any package is below 100%)
go test -cover ./... | grep -v "100.0%"
```

## Further reading
- **Database radius** - The scanner offers channels within a range of a point, so a position with no range set behaves differently from one with a radius of zero
- **GPS function** - The setting that decides whether the receiver may overwrite the position, which lives under the scanner's GPS menu rather than its location menu
- **Menu driving** - A zip code can only be resolved by the scanner, so setting one means walking its menus and stopping the scan for the duration
- **Read-after-write** - Every change is confirmed by reading the scanner back, because a menu press that appeared to work is not evidence that anything changed
- **Cold start** - A receiver moved a long way by a zip code has to find itself again from nothing, which is why waiting for a fix is measured in tens of seconds
