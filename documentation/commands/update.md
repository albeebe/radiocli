# update

Replaces the copy of `radiocli` you are running with the newest one published.
Use it when you want a fix or a feature from a later release without downloading
and installing anything by hand.

## Overview

`update` asks GitHub which release is newest, downloads the file built for your
operating system and processor, checks that file against the checksum published
with it, and puts it in the place of the program you ran. Nothing on the scanner
is touched and no scanner needs to be attached. The only thing that changes on
your computer is the `radiocli` program itself, and only after the checksum
matches: a download that does not match is discarded and the program you were
running is left exactly as it was. A release that publishes no checksum file is
refused rather than installed unchecked, and there is no flag to turn that off.
Because the program is replaced where it stands, you need permission to write to
the directory holding it, which for an install in `/usr/local/bin` means running
the command with `sudo`. It will never do that for you. The `--check` flag makes
the command report and stop, changing nothing and needing no write permission at
all.

## Usage

```
radiocli update [flags]
```

## Parameters

| Parameter | Required | Default | Description |
| --------- | -------- | ------- | ----------- |
| `--check` | No | `false` | Report whether a newer release exists, then stop without installing anything. |
| `--force` | No | `false` | Install even when this build is a development build, is already the release named, or is newer than it. |
| `--version` | No | none | Install the release with this tag instead of the newest one. |

### `--check`

Reports how the running program stands against the newest release and changes
nothing. It does not download the release, does not need permission to write
anywhere, and **always exits with status `0`**, whether or not an update is
available. Whether one is available is reported in the output, in the
`updateAvailable` field under `--output json`. Nothing about the exit status
tells you the answer.

### `--force`

Installs past the three cases the command otherwise refuses:

- The running program is a development build, which reports its version as
  `dev`. It is refused by default because a build made from a source checkout
  was almost certainly built to test a change, and installing over it would
  throw that change away.
- The running program is already the release that would be installed.
  Reinstalling is how you repair a program whose file has been damaged.
- The running program is newer than the release, which happens after installing
  a release candidate with `--version`.

### `--version`

Takes a release tag, such as `v0.1.0`. A leading `v` is added if you leave it
off, so `--version 0.1.0` and `--version v0.1.0` are the same. Any tag that has
been published can be named, which is how you go back to an earlier release.

Without this flag the newest release is chosen, and GitHub does not count
prereleases or drafts as the newest. Naming the tag is therefore the only way to
install a release candidate.

### Replacing a program you cannot write to

`update` writes to the directory the program is in. When that directory belongs
to another user, which is the case for `/usr/local/bin`, the command reports
this before downloading anything and tells you to run it again with `sudo`. It
never re-runs itself with administrator rights.

Running it under `sudo` leaves the newly installed program owned by `root`,
which is what `/usr/local/bin` expects. If you previously owned the file and
could update without `sudo`, you will need `sudo` from then on.

If you installed `radiocli` through a package manager, update it through that
package manager instead. `update` follows links to the real file and replaces
that, which a later package manager operation will overwrite or complain about.
The `path` field in the output names the file that was actually replaced, so you
can tell when this applies to you.

### Global flags that change this command

- `--output` selects whether the result is printed as lines or as JSON. Under
  `--check` the JSON form is what something polling for a new release should
  read.

The `--device`, `--pace` and `--wait` flags do nothing here, because this
command never opens the scanner.

## Examples

Checking whether a newer release exists, from an older build:

```
$ radiocli update --check
radiocli v0.1.1 is available. This is v0.1.0.
Run "radiocli update" to install it.
```

The same check as JSON, which is the form to poll:

```
$ radiocli update --check -o json
{
  "current": "v0.1.0",
  "latest": "v0.1.1",
  "updateAvailable": true,
  "state": "available",
  "dev": false,
  "pinned": false,
  "asset": "radiocli-mac.zip",
  "platform": "darwin/arm64",
  "url": "https://github.com/albeebe/radiocli/releases/tag/v0.1.1",
  "published": "2026-08-16T15:39:01Z",
  "prerelease": false
}
```

Installing it:

```
$ radiocli update
Downloading radiocli v0.1.1 for darwin/arm64.
Checked it against the checksum published with the release.
radiocli v0.1.1 installed at /usr/local/bin/radiocli
A radiocli daemon that is already running is still the old version. Restart it to pick this up.
```

Running it again, with nothing left to do:

```
$ radiocli update
radiocli v0.1.1 is already the newest release. Nothing to do.
```

Checking from a build made from a source checkout, which has no version to
compare:

```
$ radiocli update --check
radiocli dev cannot be compared against v0.1.1.
This is a development build, so there is nothing to compare. Pass --force to install v0.1.1 over it.
```

Asking about a release that was never published:

```
$ radiocli update --version v9.9.9
error: no release is tagged "v9.9.9": see https://github.com/albeebe/radiocli/releases for the tags that exist
```

## Output

The result goes to stdout. Progress, advice and warnings go to stderr, so
redirecting stderr leaves stdout holding only the result.

Under `--output text` with `--check`, stdout holds one line, which is one of
four:

| Line | Meaning |
| ---- | ------- |
| `radiocli v0.1.1 is available. This is v0.1.0.` | A newer release exists. |
| `radiocli v0.1.1 is the newest release.` | You are running it. |
| `radiocli v0.9.9 is newer than the newest release, v0.1.1.` | This build is ahead of anything published. |
| `radiocli dev cannot be compared against v0.1.1.` | This build has no version to compare. |

Under `--output text` after an install, stdout holds one line naming the version
and the file it was written to:

```
radiocli v0.1.1 installed at /usr/local/bin/radiocli
```

Under `--output json` with `--check`, stdout holds one object. Every field is
always present except `published`, which is left out when GitHub reported no
publication time:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `current` | string | Version of the program that is running, such as `v0.1.0`. `dev` for a build made from a source checkout. |
| `latest` | string | Tag of the release this was compared against, such as `v0.1.1`. The one named by `--version` when that flag was given. |
| `updateAvailable` | boolean | `true` only when `latest` is a version and is newer than `current`. `false` for a development build and for a build newer than the release. |
| `state` | string | Why `updateAvailable` is what it is. Exactly one of `available`, `current`, `ahead`, `unknown`. |
| `dev` | boolean | `true` when the running build was not stamped with a version, and so needs `--force` to replace. |
| `pinned` | boolean | `true` when `latest` came from `--version` rather than from the newest release. |
| `asset` | string | The release file this computer would install, such as `radiocli-mac.zip`. |
| `platform` | string | Operating system and processor the file was chosen for, as `darwin/arm64`. |
| `url` | string | Web address of the release page. |
| `published` | string | When the release was published, in RFC 3339, such as `2026-08-16T15:39:01Z`. Absent when GitHub reported none. |
| `prerelease` | boolean | `true` when the release is marked as a prerelease. Can only be `true` when `--version` named it. |

The four values of `state` mean:

| Value | Meaning |
| ----- | ------- |
| `available` | The release is newer than the running program. |
| `current` | They are the same version. |
| `ahead` | The running program is newer than the release. |
| `unknown` | One of the two is not a version that can be compared, which is what a development build reports. |

Under `--output json` after an install, stdout holds one object. Every field is
always present except `published`:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `from` | string | Version that was running before the install. |
| `to` | string | Tag of the release that was installed. |
| `path` | string | File that was replaced, with links followed, such as `/usr/local/bin/radiocli`. |
| `asset` | string | Release file that was downloaded. |
| `bytes` | number | Size of that file in bytes. |
| `digest` | string | SHA-256 of that file, in lowercase hexadecimal, as it was verified. |
| `url` | string | Web address of the release page. |
| `published` | string | When the release was published, in RFC 3339. Absent when GitHub reported none. |

## Errors

All of these exit with status `1`. In every one of them the program you ran is
left exactly as it was.

| Message | Meaning | Fix |
| ------- | ------- | --- |
| `this is a development build, so there is nothing to compare against a release` | The running program was built from a source checkout and reports its version as `dev`. | Pass `--force` to install the release over it, or run the release you already have. |
| `radiocli v0.9.9 is newer than v0.1.1, the newest release` | The running program is ahead of anything published. | Pass `--force` to install the older release anyway, or `--version` to name the release you want. |
| `the install directory cannot be written to` | The directory holding the program belongs to another user. | Run the same command again with `sudo`, as the message shows. |
| `no release is tagged "v9.9.9"` | `--version` named a tag that has not been published. | Check the tags at the releases page named in the message. |
| `this project has published no releases yet` | GitHub reports no releases at all. | Nothing to do here; build from source instead. |
| `no release is built for linux/386` | No file is published for this operating system and processor. | Build from source, using the commands the message gives. |
| `release v0.2.0 publishes no checksums.txt, so its downloads cannot be checked` | The release has no checksum file, so nothing can be verified. | Wait a few minutes if the release is new and still uploading, then try again. |
| `checksums.txt lists no checksum for radiocli-mac.zip` | The checksum file exists but does not cover this platform's file. | Report it; the release is incomplete. |
| `radiocli-mac.zip does not match the checksum published with it` | What arrived is not what was published. | Run the command again, which fixes a corrupted download. If it happens twice, report it. |
| `radiocli-mac.zip arrived as 512 bytes but was published as 4823041` | The download stopped early. | Run the command again. |
| `release v0.2.0 publishes no radiocli-linux-arm64.tar.gz` | The release exists but has no file for this platform. | Wait if the release is new and still uploading, then try again. |
| `github is rate limiting this computer` | Too many requests have been made from this address in the last hour. | Wait until the time the message gives. GitHub allows sixty requests an hour per address, counted for every program on it, not this one alone. |
| `the daemon cannot update the program it is running` | The command was run through a running `radiocli` daemon. | Stop the daemon, run the command again, and start it back up. See [daemon](daemon.md). |

An update leaves any already-running daemon on the old version, because a
running program keeps the file it started from. Restart it to pick up the new
one.
