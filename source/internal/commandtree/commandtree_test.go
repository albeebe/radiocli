// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package commandtree

import (
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagSet builds a set with the flags a usage line might mention. The type
// matters: a boolean flag is not followed by a value, and anything else is.
func flagSet(t *testing.T) *pflag.FlagSet {
	t.Helper()

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("yes", false, "")
	flags.Bool("off", false, "")
	flags.Bool("status", false, "")
	flags.String("type", "", "")
	flags.String("position", "", "")
	return flags
}

// TestParseUse covers every shape of usage line this tool actually writes.
//
// Positional arguments are described nowhere else, so this parse is the only
// thing standing between a usage line and a wrong form built around it.
func TestParseUse(t *testing.T) {
	tests := []struct {
		name         string
		use          string
		command      string
		want         string
		alternatives bool
	}{
		{name: "no arguments", use: "battery", command: "battery", want: ""},
		{name: "one required", use: "systems <list>", command: "systems", want: "<list>"},
		{name: "one optional", use: "colors [layout]", command: "colors", want: "[layout]"},
		{
			name:    "required then optional",
			use:     "open <menu> [index]",
			command: "open",
			want:    "<menu> [index]",
		},
		{
			name:    "repeated",
			use:     "scan [name]...",
			command: "scan",
			want:    "[name]...",
		},
		{
			name:    "two required",
			use:     "rename <list> <name>",
			command: "rename",
			want:    "<list> <name>",
		},
		{
			name:    "a boolean flag is not an argument",
			use:     "delete <system> --yes",
			command: "delete",
			want:    "<system>",
		},
		{
			// The <type> belongs to --type, not to the command. Counting it as
			// a third argument would put a box on the form that nothing fills.
			name:    "the value after a flag belongs to the flag",
			use:     "new <list> <name> --type <type>",
			command: "new",
			want:    "<list> <name>",
		},
		{
			// Written this way to show they are optional, but they are still
			// flags and are described among the flags.
			name:    "flags in brackets are not arguments",
			use:     "gps [--off] [--status]",
			command: "gps",
			want:    "",
		},
		{
			name:    "a list of values names itself after the command",
			use:     "mode [color|black|white]",
			command: "mode",
			want:    "[mode]{color,black,white}",
		},
		{
			// The first form still has a usable argument, and the later form
			// is the same command driven by a flag that is described anyway.
			name:         "the first of several forms is still read",
			use:          "set <zip> | --position <latitude>,<longitude>",
			command:      "set",
			want:         "<zip>",
			alternatives: true,
		},
		{
			name:         "an unbracketed word is not understood",
			use:          "set datetime",
			command:      "set",
			alternatives: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, alternatives := parseUse(tt.use, tt.command, flagSet(t))

			if alternatives != tt.alternatives {
				t.Fatalf("alternatives = %v, want %v", alternatives, tt.alternatives)
			}
			if got := format(args); got != tt.want {
				t.Errorf("parsed %q as %q, want %q", tt.use, got, tt.want)
			}
		})
	}
}

// format writes parsed arguments back out in the notation they came from, so a
// failure reads like a usage line rather than like a struct dump.
func format(args []appcontext.Arg) string {
	var parts []string

	for _, arg := range args {
		text := "[" + arg.Name + "]"
		if arg.Required {
			text = "<" + arg.Name + ">"
		}
		if arg.Repeated {
			text += "..."
		}
		if len(arg.Values) > 0 {
			text += "{" + strings.Join(arg.Values, ",") + "}"
		}
		parts = append(parts, text)
	}

	return strings.Join(parts, " ")
}

// TestDescribe tests the Describe function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - SkipsLibraryAndHiddenCommands: only visible commands are described
//   - EmptyRoot: a root with no commands yields nil
func TestDescribe(t *testing.T) {
	// Verify that visible commands are described while hidden commands and
	// cobra's own help command are left out.
	t.Run("SkipsLibraryAndHiddenCommands", func(t *testing.T) {
		root := &cobra.Command{Use: "test"}
		root.AddCommand(&cobra.Command{Use: "battery", Short: "Battery level"})
		root.AddCommand(&cobra.Command{Use: "secret", Hidden: true})
		root.AddCommand(&cobra.Command{Use: "help"})

		list := Describe(root)

		if len(list) != 1 {
			t.Fatalf("described %d commands, want 1", len(list))
		}
		if list[0].Name != "battery" {
			t.Errorf("described %q, want %q", list[0].Name, "battery")
		}
	})

	// Verify that a root with no commands under it yields nothing.
	t.Run("EmptyRoot", func(t *testing.T) {
		if list := Describe(&cobra.Command{Use: "test"}); list != nil {
			t.Errorf("described %v, want nil", list)
		}
	})
}

// Test_describe tests the describe function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - ValidArgsAttachToFirstArgument: a command's accepted values land on its
//     first argument
//   - ValidArgsWithNothingToAttachTo: accepted values are dropped when the
//     usage line has no arguments
//   - VisibleSubcommandsAreNested: subcommands are described recursively and
//     hidden ones are left out
func Test_describe(t *testing.T) {
	// Verify that a command's accepted values are attached to its first
	// argument.
	t.Run("ValidArgsAttachToFirstArgument", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:       "mode <mode>",
			ValidArgs: []string{"color", "black", "white"},
		}

		out := describe(cmd)

		if len(out.Args) != 1 {
			t.Fatalf("described %d arguments, want 1", len(out.Args))
		}
		if got := strings.Join(out.Args[0].Values, ","); got != "color,black,white" {
			t.Errorf("values = %q, want %q", got, "color,black,white")
		}
	})

	// Verify that accepted values are dropped when the usage line names no
	// arguments to attach them to.
	t.Run("ValidArgsWithNothingToAttachTo", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:       "battery",
			ValidArgs: []string{"color"},
		}

		out := describe(cmd)

		if out.Args != nil {
			t.Errorf("described arguments %v, want none", out.Args)
		}
	})

	// Verify that visible subcommands are described recursively while hidden
	// ones are left out, and that a command with a Run function is runnable.
	t.Run("VisibleSubcommandsAreNested", func(t *testing.T) {
		parent := &cobra.Command{Use: "gps", Run: func(*cobra.Command, []string) {}}
		parent.AddCommand(&cobra.Command{Use: "status", Run: func(*cobra.Command, []string) {}})
		parent.AddCommand(&cobra.Command{Use: "debug", Hidden: true})

		out := describe(parent)

		if !out.Runnable {
			t.Error("Runnable = false, want true")
		}
		if len(out.Subcommands) != 1 {
			t.Fatalf("described %d subcommands, want 1", len(out.Subcommands))
		}
		if out.Subcommands[0].Name != "status" {
			t.Errorf("described %q, want %q", out.Subcommands[0].Name, "status")
		}
	})
}

// TestDescribeFlags tests the describeFlags function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - HiddenDeprecatedAndHelpAreLeftOut: only ordinary flags are described
//   - NoFlags: a command without flags yields nil
func TestDescribeFlags(t *testing.T) {
	// Verify that hidden flags, deprecated flags, and the help flag are left
	// out, and that an ordinary flag's fields come through intact.
	t.Run("HiddenDeprecatedAndHelpAreLeftOut", func(t *testing.T) {
		cmd := &cobra.Command{Use: "scan"}
		cmd.Flags().StringP("format", "f", "table", "Output format")
		cmd.Flags().Bool("debug", false, "")
		if err := cmd.Flags().MarkHidden("debug"); err != nil {
			t.Fatalf("failed to hide flag: %v", err)
		}
		cmd.Flags().Bool("legacy", false, "")
		cmd.Flags().Lookup("legacy").Deprecated = "use format"
		cmd.Flags().Bool("help", false, "")

		flags := describeFlags(cmd)

		if len(flags) != 1 {
			t.Fatalf("described %d flags, want 1", len(flags))
		}
		want := appcontext.Flag{
			Name:      "format",
			Shorthand: "f",
			Type:      "string",
			Default:   "table",
			Usage:     "Output format",
		}
		if flags[0] != want {
			t.Errorf("described %+v, want %+v", flags[0], want)
		}
	})

	// Verify that a command without flags yields nothing.
	t.Run("NoFlags", func(t *testing.T) {
		if flags := describeFlags(&cobra.Command{Use: "battery"}); flags != nil {
			t.Errorf("described %v, want none", flags)
		}
	})
}

// TestSkip tests the skip function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every condition)
//
// Test cases:
//   - Hidden: a hidden command is skipped
//   - Deprecated: a deprecated command is skipped
//   - Help: cobra's help command is skipped
//   - Completion: cobra's completion command is skipped
//   - Ordinary: an ordinary command is kept
func TestSkip(t *testing.T) {
	// Verify that a hidden command is skipped.
	t.Run("Hidden", func(t *testing.T) {
		if !skip(&cobra.Command{Use: "secret", Hidden: true}) {
			t.Error("skip = false, want true")
		}
	})

	// Verify that a deprecated command is skipped.
	t.Run("Deprecated", func(t *testing.T) {
		if !skip(&cobra.Command{Use: "old", Deprecated: "use new"}) {
			t.Error("skip = false, want true")
		}
	})

	// Verify that cobra's help command is skipped.
	t.Run("Help", func(t *testing.T) {
		if !skip(&cobra.Command{Use: "help"}) {
			t.Error("skip = false, want true")
		}
	})

	// Verify that cobra's completion command is skipped.
	t.Run("Completion", func(t *testing.T) {
		if !skip(&cobra.Command{Use: "completion"}) {
			t.Error("skip = false, want true")
		}
	})

	// Verify that an ordinary command is kept.
	t.Run("Ordinary", func(t *testing.T) {
		if skip(&cobra.Command{Use: "battery"}) {
			t.Error("skip = true, want false")
		}
	})
}

// TestParseArg tests the empty-placeholder branch of parseArg. The rest of
// parseArg is exercised through TestParseUse, which drives it with every shape
// of usage line this tool writes.
//
// Coverage: 100% (2 test cases covering the remaining branch)
//
// Test cases:
//   - EmptyRequired: a bare <> names nothing and is not understood
//   - EmptyOptional: a bare [] names nothing and is not understood
func TestParseArg(t *testing.T) {
	// Verify that an empty required placeholder is not understood.
	t.Run("EmptyRequired", func(t *testing.T) {
		if _, ok := parseArg("<>", "test"); ok {
			t.Error("parsed <>, want not understood")
		}
	})

	// Verify that an empty optional placeholder is not understood.
	t.Run("EmptyOptional", func(t *testing.T) {
		if _, ok := parseArg("[]", "test"); ok {
			t.Error("parsed [], want not understood")
		}
	})
}

// TestTakesValue tests the shorthand and unknown-flag branches of takesValue.
// The long-name branches are exercised through TestParseUse.
//
// Coverage: 100% (3 test cases covering the remaining branches)
//
// Test cases:
//   - ShorthandBoolean: a single-letter boolean flag is found by shorthand
//   - UnknownLongFlag: an unknown long flag is assumed to take a value
//   - UnknownShorthand: an unknown single-letter flag is assumed to take a
//     value
func TestTakesValue(t *testing.T) {
	// Verify that a single-letter mention of a boolean flag is found by its
	// shorthand and takes no value.
	t.Run("ShorthandBoolean", func(t *testing.T) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.BoolP("verbose", "v", false, "")

		if takesValue(flags, "v") {
			t.Error("takesValue = true, want false")
		}
	})

	// Verify that an unknown long flag is assumed to take a value.
	t.Run("UnknownLongFlag", func(t *testing.T) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

		if !takesValue(flags, "mystery") {
			t.Error("takesValue = false, want true")
		}
	})

	// Verify that an unknown single-letter flag is assumed to take a value.
	t.Run("UnknownShorthand", func(t *testing.T) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

		if !takesValue(flags, "x") {
			t.Error("takesValue = false, want true")
		}
	})
}
