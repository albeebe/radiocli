# backup

## What this does?
This package is the `backup` command: it copies everything on the scanner's memory card into a dated folder on this computer, then checks that what was written matches what was read. It is the tool's answer to "make a copy of my radio before I change anything".

## Why we use it?
The scanner keeps its real contents on a memory card rather than in anything the serial protocol will hand over. The favorites lists, the settings and the display colors all live there, and some of it, the colors especially, cannot be read back over the wire at all. That makes the card the only complete copy of a radio that somebody has spent hours setting up, and it sits in a handheld device that gets dropped, goes flat, and can be reset by a menu entry two keypresses away from one people use.

Backup is its own command because it is the one that does not talk to the scanner. The card is only reachable when the radio is started in mass storage mode, and that mode replaces the serial port instead of joining it, so this command and every other one in the tool can never run in the same session. Keeping it separate means the rest of the tool can assume a serial connection while this one assumes a mounted volume, and the care that belongs here, finding the card by its contents rather than a volume name it does not have, keeping the empty directories the radio expects to write into, refusing to merge two backups into one folder that would look complete and be neither, stays in the one place that needs it.

## How we use it?
```go
// main.go lists every command the tool exposes; adding one means importing its
// package and adding its constructor to this slice.
var commands = []func(*appcontext.App) *cobra.Command{
	backup.New,
}
```

```bash
# Restart the scanner with the cable connected and press "E" at the USB
# prompt, then copy the card into a dated folder in the current directory.
radiocli backup

# See what would be copied without writing anything.
radiocli backup --dry-run

# Skip the downloaded radio database, which is most of the card's size.
radiocli backup ~/scanner-backups --database=false

# Point at the card directly, and name the folder yourself.
radiocli backup --source /Volumes/NO\ NAME --name before-firmware-update
```

```json
{
  "card": { "root": "/Volumes/NO NAME", "model": "SDS150", "serial": "…", "firmware": "1.00.37" },
  "destination": "/Users/example/SDS150-2026-08-12-0930",
  "files": 412,
  "bytes": 1073741824,
  "directories": 37,
  "verified": true,
  "databaseIncluded": true,
  "dryRun": false,
  "copied": [{ "path": "favorites/FL_001.hpd", "bytes": 4096, "digest": "…" }]
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
- **USB mass storage mode** - The card is only reachable when the scanner boots into it, and that mode removes the serial port every other command needs
- **Cobra commands** - The command is a `*cobra.Command` built by `New`, which is how every command in this tool is wired into the tree
- **SHA-256 and checksums** - Verification hashes the card as it is read and the copy as it is written, so a truncated or corrupted file is caught rather than trusted
- **io.TeeReader** - Hashing and copying in one pass is what lets a large card be verified without reading every file twice over USB
- **filepath.WalkDir** - Walking the card first and copying second is what makes a total, a dry run and a refusal possible before anything is written
