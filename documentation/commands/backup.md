# backup

Copies everything on your scanner's memory card onto your computer. Run it before you change anything on the scanner that you might want to undo.

## Overview

Your scanner keeps its favorites lists, its settings, its recordings and the colors it draws its screen in on a memory card inside it. `backup` copies all of that into a dated folder on your computer and, by default, reads every file back afterwards to confirm the copy matches. It only reads the card and only writes to the folder you choose, so it changes nothing on the scanner.

The card is reachable only when the scanner is started in mass storage mode. The scanner shows a USB prompt for about 15 seconds when it starts and boots into scanning if you do not answer, so you have to restart it with the cable connected and press `E` when asked. Choosing mass storage replaces the serial port rather than joining it, which means the scanner is not reachable by any other `radiocli` command while the card is mounted, and no other command can run alongside this one. Restart the scanner to get the serial port back.

This is the only command in the tool that does not talk to the scanner, so it needs no scanner named and ignores the global `--device`, `--pace` and `--wait` flags.

## Usage

```
radiocli backup [destination] [flags]
```

`destination` is the folder to create the backup inside. It defaults to the current directory. The backup itself goes into a new subfolder of that directory, named for the scanner and the current date and time, such as `SDS150-2026-08-05-1724`.

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--source` | No | none | Path to the card, instead of searching for a mounted one. |
| `--name` | No | none | Name for the backup folder, instead of the dated default. |
| `--verify` | No | `true` | Read every file back and compare it against the card. |
| `--database` | No | `true` | Include the downloaded radio database. |
| `--dry-run` | No | `false` | Report what would be copied and write nothing. |

### `--source`

The path to the scanner's card. Pass it when the search does not find the card, which happens on Windows, where volumes are drive letters rather than directories inside a folder, and on any system that mounts removable media somewhere unusual.

Without it, `backup` searches the places this system mounts removable media: `/Volumes` on macOS, and `/media`, `/run/media`, `/mnt` and their per-user subdirectories on Linux. It identifies the card by finding a `BCDx36HP` directory in it, not by the volume's name, because the card is formatted without one and mounts as `NO NAME` on macOS.

Both the volume and the `BCDx36HP` directory inside it are accepted:

```
radiocli backup --source "/Volumes/NO NAME"
radiocli backup --source "/Volumes/NO NAME/BCDx36HP"
radiocli backup --source D:\
```

### `--name`

The name of the folder to create inside the destination. Use it to label a backup by what it is rather than by when it was taken.

```
radiocli backup ~/sds150-backups --name before-color-changes
```

Without it, the folder is named for the scanner's model and the current date and time, to the minute, such as `SDS150-2026-08-05-1724`.

`backup` refuses to write into a folder that already exists rather than merging into it, because two backups blended together would look like one complete card and be neither.

### `--verify`

Reads every copied file back from your disk and compares it against the card. On by default. Pass `--verify=false` to skip it, which is faster on a large card but leaves you without proof the copy is good.

```
radiocli backup --verify=false
```

Verifying roughly doubles the reading. A full 55 MB card takes about 1 minute 45 seconds verified over USB.

When verifying, `--output json` includes a `digest` for every file. When not verifying, `digest` is absent.

### `--database`

Includes the downloaded radio database, which is the folder the systems and frequencies you scan come from. On by default. It is around 55 MB of a typical 57 MB card, so excluding it makes the backup roughly fifty times smaller and near-instant.

```
radiocli backup --database=false
```

Exclude it only if you can rebuild it, which means re-downloading it from Sentinel. A backup taken this way holds your favorites lists, your settings and your display colors, but is **not** a complete card and cannot restore one on its own.

### `--dry-run`

Reports what would be copied and writes nothing at all, including creating no folder. Use it to check that the card was found and to see how large the backup will be.

```
radiocli backup --dry-run
```

## Examples

The serial number is shown as `XXXXX-XXXXXXXXX-XXX` in the output below. Your
scanner prints its own there. It is the only value in these examples that has
been altered, because it identifies one physical radio.

```
$ radiocli backup
Found SDS150 XXXXX-XXXXXXXXX-XXX on /Volumes/NO NAME
Copying 80 files, 55.3 MB, to /tmp/sds150-backups/SDS150-2026-08-05-1724
  25/80
  50/80
  75/80
  80/80
Backed up 80 files in 11 directories, 55.3 MB, to /tmp/sds150-backups/SDS150-2026-08-05-1724
Every file was read back and matched the card.
Restart the scanner to use the serial port again.
```

A whole card, verified, into a dated folder in the current directory.

```
$ radiocli backup --dry-run
Found SDS150 XXXXX-XXXXXXXXX-XXX on /Volumes/NO NAME
Would copy 80 files in 11 directories, 55.3 MB, from /Volumes/NO NAME
Nothing was written.
```

Checks that the card was found and reports its size without writing anything.

```
$ radiocli backup --database=false --name settings-only
Found SDS150 XXXXX-XXXXXXXXX-XXX on /Volumes/NO NAME
Copying 9 files, 1.2 MB, to /tmp/sds150-backups/settings-only
  9/9
Backed up 9 files in 10 directories, 1.2 MB, to /tmp/sds150-backups/settings-only
Every file was read back and matched the card.
The radio database was excluded, so this backup is not a complete card.
Restart the scanner to use the serial port again.
```

Your configuration without the bulk of the database, into a folder named for the occasion.

## Output

Results go to stdout. The card that was found, the copying progress, and the advice afterwards go to stderr, so a pipe or a redirect of the result carries only the result.

Under `--output text`, stdout holds one line: `Backed up <files> files in <directories> directories, <size>, to <path>`, or `Would copy <files> files in <directories> directories, <size>, from <path>` for a dry run.

Under `--output json`, stdout holds one object:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `card` | object | The scanner the backup came from. |
| `card.root` | string | Path to the mounted card. |
| `card.model` | string | Model the card reports, such as `SDS150`. Empty if the card's identity file is missing or unreadable. |
| `card.serial` | string | Serial number the card reports. Empty if unreadable. |
| `card.firmware` | string | Firmware version the card reports. Empty if unreadable. |
| `destination` | string | Absolute path of the folder written. Absent for a dry run. |
| `files` | number | How many files were copied, or would have been. |
| `bytes` | number | Their total size in bytes. |
| `directories` | number | How many directories were created, including the empty ones. |
| `verified` | boolean | Whether every file was read back and compared. Always `false` for a dry run. |
| `databaseIncluded` | boolean | Whether the radio database was copied. |
| `dryRun` | boolean | Whether the run wrote anything. |
| `copied` | array | One entry per file. |
| `copied[].path` | string | Path relative to the scanner's directory on the card. |
| `copied[].bytes` | number | Size in bytes. |
| `copied[].digest` | string | SHA-256 of the file. Absent when the backup was not verified and for a dry run. |

```
$ radiocli backup --database=false --name j -o json
{
  "card": {
    "root": "/Volumes/NO NAME",
    "model": "SDS150",
    "serial": "XXXXX-XXXXXXXXX-XXX",
    "firmware": "1.00.37"
  },
  "destination": "/tmp/sds150-backups/j",
  "files": 9,
  "bytes": 1279946,
  "directories": 10,
  "verified": true,
  "databaseIncluded": false,
  "dryRun": false,
  "copied": [
    {
      "path": "app_data.cfg",
      "bytes": 370,
      "digest": "6a3f90526ce9f06ed38466f242f73de5df1a7bdb548ede0dfec5fc8e01812a1f"
    }
  ]
}
```

Files that belong to your computer rather than to the scanner are never copied. That means anything whose name begins with a dot, which covers the Spotlight and FSEvents folders macOS writes onto any volume it mounts, and `System Volume Information`, which Windows writes. Empty directories on the card are recreated, because the scanner writes its recordings, alerts, discovery sessions and activity logs into them and expects them to exist.

## Errors

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `no scanner card is mounted` | No mounted volume holds a `BCDx36HP` directory. The full message explains how to reach the card. | Restart the scanner with the cable connected and press `E` at the USB prompt. If it is mounted somewhere the search does not look, pass `--source`. |
| `<path> does not hold a scanner card: expected a BCDx36HP directory inside it` | The path given to `--source` is not a card. | Point `--source` at the volume the card is mounted on. |
| `<path> already exists: pass --name to choose another, or move it out of the way` | The backup folder is already there. | Pass `--name`, or move the existing folder. |
| `the card at <path> holds no files, which means it is not the card this expects` | A `BCDx36HP` directory was found but is empty. | Check that the card is the scanner's and is not damaged. |
| `<path> did not copy correctly: the card and the copy differ` | A file read back differently from how it was written. | Run it again. If it repeats, the card or the cable is failing. |

When a copy fails partway, the files already written are left in place and their location is printed, because a partial backup you know about is worth more than none.
