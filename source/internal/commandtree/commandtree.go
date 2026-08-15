// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

// Package commandtree reads a cobra command tree into plain data.
//
// It exists so anything presenting the tool, such as the daemon answering a
// client that asks what this build can run, can offer every command and every
// flag without keeping its own copy of the list. A copy is a second place to forget a command, and the rule in
// main is that adding one means importing its package and adding its
// constructor "and nothing else".
package commandtree

import (
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Describe reads the commands nested under root, and their commands, and so on
// down.
//
// Cobra's own help and completion commands are left out: they belong to the
// library rather than to this tool. So is anything hidden.
//
// The full help text is deliberately not included. It is by far the largest
// part of the tree and is wanted only when somebody asks for one command, and
// a caller that can run commands can run "<command> --help" to get it.
//
// Parameters:
//   - root: command whose nested commands are described
//
// Returns:
//   - a slice of appcontext.Command, one per visible command under root, nil
//     when there are none
func Describe(root *cobra.Command) []appcontext.Command {
	var list []appcontext.Command

	for _, sub := range root.Commands() {
		if skip(sub) {
			continue
		}
		list = append(list, describe(sub))
	}

	return list
}

// bracketed reports whether a field is a placeholder rather than a literal.
//
// Parameters:
//   - field: one field of a usage line
//
// Returns:
//   - true if the field is wrapped in <> or [], false otherwise
func bracketed(field string) bool {
	return (strings.HasPrefix(field, "<") && strings.HasSuffix(field, ">")) ||
		(strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]"))
}

// describe reads one command and everything under it.
//
// Parameters:
//   - cmd: command to describe
//
// Returns:
//   - an appcontext.Command holding the command's name, usage, arguments,
//     flags, and visible subcommands
func describe(cmd *cobra.Command) appcontext.Command {
	args, alternatives := parseUse(cmd.Use, cmd.Name(), cmd.LocalFlags())

	// Where a command says which values it accepts, that belongs on the first
	// argument. Cobra only has the one list per command, so there is nothing
	// to attach it to beyond that.
	if len(cmd.ValidArgs) > 0 && len(args) > 0 {
		args[0].Values = append([]string(nil), cmd.ValidArgs...)
	}

	out := appcontext.Command{
		Name:         cmd.Name(),
		Short:        cmd.Short,
		Use:          cmd.Use,
		Args:         args,
		Flags:        describeFlags(cmd),
		Runnable:     cmd.Runnable(),
		Alternatives: alternatives,
	}

	for _, sub := range cmd.Commands() {
		if skip(sub) {
			continue
		}
		out.Subcommands = append(out.Subcommands, describe(sub))
	}

	return out
}

// describeFlags reads the flags belonging to this command.
//
// Only the command's own flags are read. The global flags are inherited by
// everything, so repeating them on every command would bury the one or two
// that are actually about the command in front of you.
//
// Parameters:
//   - cmd: command whose local flags are read
//
// Returns:
//   - a slice of appcontext.Flag, one per visible flag, nil when there are
//     none
func describeFlags(cmd *cobra.Command) []appcontext.Flag {
	var flags []appcontext.Flag

	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		// help is added by cobra to every command and does nothing useful
		// here: a form that offers it would just print the help it is built
		// from.
		if f.Hidden || f.Deprecated != "" || f.Name == "help" {
			return
		}
		flags = append(flags, appcontext.Flag{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Default:   f.DefValue,
			Usage:     f.Usage,
		})
	})

	return flags
}

// flagMention reports whether a usage field names a flag rather than an
// argument, and gives the flag's name without its dashes or brackets.
//
// Parameters:
//   - field: one field of a usage line
//
// Returns:
//   - the flag's name stripped of dashes and brackets, empty when the field
//     is not a flag
//   - true if the field names a flag, false otherwise
func flagMention(field string) (string, bool) {
	field = strings.TrimSuffix(strings.TrimPrefix(field, "["), "]")
	field = strings.TrimSuffix(strings.TrimPrefix(field, "<"), ">")

	if !strings.HasPrefix(field, "-") {
		return "", false
	}
	return strings.TrimLeft(field, "-"), true
}

// parseArg reads one field of a usage line. name is the command's own name,
// used when the field lists its accepted values instead of naming itself.
//
// Parameters:
//   - field: one field of a usage line
//   - name: the command's own name
//
// Returns:
//   - the parsed argument, zero valued when the field does not follow the
//     convention
//   - true if the field was understood, false otherwise
func parseArg(field, name string) (appcontext.Arg, bool) {
	var arg appcontext.Arg

	if strings.HasSuffix(field, "...") {
		arg.Repeated = true
		field = strings.TrimSuffix(field, "...")
	}

	switch {
	case strings.HasPrefix(field, "<") && strings.HasSuffix(field, ">"):
		arg.Required = true
		field = field[1 : len(field)-1]
	case strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]"):
		field = field[1 : len(field)-1]
	default:
		return appcontext.Arg{}, false
	}

	if field == "" {
		return appcontext.Arg{}, false
	}

	// A field written as [color|black|white] lists its accepted values instead
	// of naming itself, so the alternatives become the values and the argument
	// takes the command's name, which is what it is choosing.
	if strings.Contains(field, "|") {
		arg.Values = strings.Split(field, "|")
		arg.Name = name
		return arg, true
	}

	arg.Name = field
	return arg, true
}

// parseUse reads the positional arguments out of a usage line.
//
// Positional arguments exist nowhere else. Cobra records how many are allowed
// but never what they are called, so the usage line is the only description of
// them, and this repository writes it to one convention, the same one stated
// in the documentation standard: <name> is a value you must supply, [name] is
// optional, and a trailing ... means more than one may be given.
//
// Flags turn up in usage lines too, for emphasis, written plainly as in
// "delete <system> --yes" or in brackets as in "gps [--off] [--status]". They
// are described properly among the flags, so they are skipped here, and so is
// the placeholder after one that takes a value: the <type> in
// "new <list> <name> --type <type>" belongs to the flag, not to the command.
//
// A "|" between whole arguments means the command has more than one form, as
// "set <zip> | --position <latitude>,<longitude>" does. The arguments of the
// first form are still worth having, since the later forms are usually the
// same command driven by a flag and the flags are described anyway, so the
// first form is read and the caller is told there are others.
//
// Parameters:
//   - use: the usage line, beginning with the command's own name
//   - name: the command's own name, given to an argument that lists its
//     accepted values instead of naming itself
//   - flags: the command's flags, used to tell whether a mentioned flag is
//     followed by a value
//
// Returns:
//   - args: the positional arguments of the first form, nil when the line
//     holds something the convention does not cover
//   - alternatives: true when the usage line describes more ways to call the
//     command than args covers
func parseUse(use, name string, flags *pflag.FlagSet) (args []appcontext.Arg, alternatives bool) {
	fields := strings.Fields(use)

	// The first field is the command's own name, not an argument.
	if len(fields) > 0 {
		fields = fields[1:]
	}

	for i := 0; i < len(fields); i++ {
		field := fields[i]

		// Everything past this describes another way to call the command.
		// The arguments gathered so far are the first way, which is the one
		// the form is built around.
		if field == "|" {
			return args, true
		}

		if flag, ok := flagMention(field); ok {
			if takesValue(flags, flag) && i+1 < len(fields) && bracketed(fields[i+1]) {
				i++
			}
			continue
		}

		arg, ok := parseArg(field, name)
		if !ok {
			// Something the convention does not cover. Rather than invent a
			// form around it, drop the arguments and let the caller fall back
			// to showing the usage line.
			return nil, true
		}
		args = append(args, arg)
	}

	return args, false
}

// skip reports whether a command should be left out of the description.
//
// Parameters:
//   - cmd: command being considered
//
// Returns:
//   - true if the command is hidden, deprecated, or one of cobra's own help
//     and completion commands, false otherwise
func skip(cmd *cobra.Command) bool {
	return cmd.Hidden ||
		cmd.Deprecated != "" ||
		cmd.Name() == "help" ||
		cmd.Name() == "completion"
}

// takesValue reports whether a named flag is followed by a value. An unknown
// flag is assumed to take one, because a usage line that mentions a flag this
// command does not have is already wrong and swallowing the next field is the
// safer of the two guesses.
//
// Parameters:
//   - flags: the command's flags, searched by long name and then by shorthand
//   - name: the flag's name without its dashes
//
// Returns:
//   - true if the flag takes a value or is unknown, false for a boolean flag
func takesValue(flags *pflag.FlagSet, name string) bool {
	flag := flags.Lookup(name)
	if flag == nil && len(name) == 1 {
		flag = flags.ShorthandLookup(name)
	}
	if flag == nil {
		return true
	}
	return flag.Value.Type() != "bool"
}
