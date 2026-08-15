# commands

## What this does?
Holds one reference page per `radiocli` command, each covering every flag, every output field and every error that command can produce. It is where you look when you know which command you want and need the detail.

## Why we use it?
Help text is a reminder for somebody who has used a command before. It cannot say what the JSON fields are called, which other command produces the value a flag expects, what happens if you leave the flag off, or what the command changes on the scanner and what it leaves alone. Those are the questions a reader actually has, and the ones that decide whether a command is safe to run. Splitting the answer into one file per command means the reader lands on exactly the page they need instead of scrolling a single enormous manual.

These files are also read by AI agents that then run what they say, which raises the cost of a defect above the usual. A person who hits a gap runs `--help` and recovers. An agent has no way to tell a flag that does not exist from one nobody wrote down, so an omitted flag becomes a capability lost and a wrong default becomes a wrong command executed. That is why the standard for this folder, [WRITING_COMMAND_DOCS.md](../../contributing/WRITING_COMMAND_DOCS.md), ranks completeness and accuracy above every other rule, bans abbreviated lists, and requires every example to have been run rather than written.

## How we use it?
One file per command, named after the command, with underscores where the command has spaces:

| File | Command |
| --- | --- |
| [status.md](status.md) | The command `status`, named for the command |
| [config_macro.md](config_macro.md) | A subcommand with flags of its own gets its own file: `config macro` |

Every command the tool ships has a file here, and the folder listing is the index.

Adding a command means adding its file here in the same change. Read [WRITING_COMMAND_DOCS.md](../../contributing/WRITING_COMMAND_DOCS.md) first: it fixes the eight required headings, their order, and the rule that a command links to another command only where the reader depends on it, at the point where they need it.

Nothing checks that a shipped command has a page here, so a new command's file is part of the same change or it is missing.

## Further reading
- **Prompt injection surface** - Why documentation an agent executes literally has to be verified line by line rather than written from memory
- **Exit codes** - The part of a command's contract a script depends on, documented per command in its Errors table
- **stdout versus stderr** - Results on one, progress and prompts on the other, which is what makes `-o json` safe to pipe
- **Relative markdown links** - How these files point at each other so the folder reads as one manual rather than a pile
- **Single source of truth** - The rule that the source wins when it disagrees with a doc, and the doc is fixed in the same change as the code
