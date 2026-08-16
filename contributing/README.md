# contributing

## What this does?
Holds the standards anyone adding to `radiocli` is expected to follow. It is the folder to read before writing a command, not after.

## Why we use it?
A tool with thirty commands only stays usable if all thirty behave the same way. The flags mean the same thing everywhere, the errors read the same way, the tests cover the same ground, and every command's file answers the same questions in the same order. That consistency is not something a reviewer can hold in their head across a year of changes, so it is written down here instead: what a command package contains, how its output is shaped, what its documentation must include, and what to run before calling any of it done. A contributor who follows these files produces a command indistinguishable from the ones already in the tool, which is the point.

These standards live apart from both the code and the command reference because they are aimed at a third reader. `source/` is for someone changing the tool, [documentation/](../documentation/) is for someone using it, and this folder is for someone adding to it. Keeping the standards out of the reference also keeps the reference honest: a reader looking up what `battery` prints should not have to step over the rules that govern how that file was written. The separation matters more than usual here because AI agents read all three, and an agent asked to add a command needs the standard without the reference's thirty files in the way.

## How we use it?
Read both files before starting, and run the checks at the end of the first one before finishing:

| Document | Covers |
| --- | --- |
| [WRITING_COMMANDS.md](WRITING_COMMANDS.md) | The standard a command's code, comments, tests and docs are held to |
| [WRITING_COMMAND_DOCS.md](WRITING_COMMAND_DOCS.md) | The standard the command's reference file in [documentation/commands/](../documentation/commands/) is held to |

[WRITING_COMMANDS.md](WRITING_COMMANDS.md) covers the whole command: the package layout, the command literal, flags, output, errors, comments, tests, the end-to-end suite and the package README. [WRITING_COMMAND_DOCS.md](WRITING_COMMAND_DOCS.md) covers the one file that goes in [documentation/commands/](../documentation/commands/), fixing its eight headings, their order, and the rule that a command links to another command only where the reader depends on it.

## Further reading
- **Coding standards** - Why a written, enforceable standard beats a reviewer's memory once a project outlives one contributor
- **Docs as code** - Why the standard, the code and the reference all change in the same commit
- **Decidable rules** - Writing a rule so that conformance can be checked rather than argued about, which is what makes these files enforceable by an agent
- **Reference documentation** - The genre the command files belong to, and why completeness outranks readability there
- **Definition of done** - The idea that a command is not finished until its code, comments, tests, suite entry and documentation all exist and agree
