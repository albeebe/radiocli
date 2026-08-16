# documentation

## What this does?
Holds the written reference for `radiocli`: what every command does, what the flags mean, and worked examples of using the tool. It is the folder to read when you want to know how to drive the scanner rather than how the code is put together.

## Why we use it?
A command-line tool that drives a radio has more surface than its help text can carry. `radiocli` has thirty commands, most of them with subcommands, and the interesting details are the ones `--help` has no room for: which command has to be run before this one, what the JSON fields are called, what a failure message means, and which flags quietly do nothing here. Somebody deciding whether a command is safe to run needs that written down, and so does an AI agent asked to operate the tool, because an agent cannot tell a capability that does not exist from one nobody documented.

This folder is kept apart from the source because it is written for a reader holding a terminal, not the code. Nothing in here names a Go package, a function or a file under `source/`, so the documentation stays true across a refactor and stays readable to somebody who will never open the repository. It is also the only place where behaviour spanning several commands can live: the global flags, where settings come from, and the daemon protocol belong to the tool as a whole rather than to any one command.

## How we use it?
Start with the task-oriented tour, then look up the command you landed on:

| Document | Covers |
| --- | --- |
| [examples.md](examples.md) | The short tour: what you would actually want to do, in order |
| [commands/](commands/) | One file per command, named after the command, such as [scanning.md](commands/scanning.md) |
| [global_flags.md](global_flags.md) | The flags that work everywhere, and how config precedence resolves |
| [daemon_protocol.md](daemon_protocol.md) | The wire format, for anything implementing a daemon client |

Writing a new command means writing its file in [commands/](commands/) as part of the same change. The standard those files are held to is [WRITING_COMMAND_DOCS.md](../contributing/WRITING_COMMAND_DOCS.md), and the standard the command itself is held to, covering the code, the comments, the tests and the documentation together, is [WRITING_COMMANDS.md](../contributing/WRITING_COMMANDS.md). Both live in [contributing/](../contributing/).

## Further reading
- **Reference documentation** - The genre these files belong to: complete and lookup-shaped, as opposed to a tutorial that teaches one path through
- **Docs as code** - Why the documentation lives in the repository and changes in the same commit as the command it describes
- **Machine-readable output** - Why `--output json` field names are documented as carefully as the human tables, since agents read these files and then run what they say
- **Configuration precedence** - Defaults, then config file, then flags: the rule that decides what a command actually uses
- **Serial line protocol** - What the tool is really talking to, and the reason a command can be refused because another one holds the scanner
