// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/15/2026

package suite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// nameLimit is how long a check's name may be.
//
// The report draws the suite as the tool's own command tree, and a check four
// commands deep is already indented a long way before its name starts. A cap
// keeps the marks in one readable column instead of the deepest check pushing
// every one of them off the side of the screen.
const nameLimit = 50

// TestRadiocli_TestNamesMatchCommands holds the suite to the naming convention
// the report reads.
//
// Every test function is named for the command it covers, so the report can
// file it under that command without being told. A function named for a command
// that does not exist has nowhere to go and would be drawn under the root, next
// to the global tests, where nobody would look for it. That is invisible in a
// passing run, which is why it is checked here rather than left to review.
//
// The suite reads its own source to do it. That is unusual, and it is the only
// honest way: nothing at runtime knows what a test function is called.
func TestRadiocli_TestNamesMatchCommands(t *testing.T) {
	for name, file := range suiteFiles(t) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			// TestMain is Go's own entry point, not a test of a command.
			if fn.Name.Name == "TestMain" {
				continue
			}

			if _, _, ok := Split(fn.Name.Name); !ok {
				t.Errorf("%s: %s is not named for any command the tool offers.\n"+
					"A test function is named for the command it covers, so TestChannelsNew "+
					"covers \"channels new\". Where one command needs several functions, add "+
					"an underscore and a name: TestChannelsNew_SimilarNames. Tests belonging "+
					"to no one command are the root command's, and start TestRadiocli_.",
					name, fn.Name.Name)
			}
		}
	}
}

// TestRadiocli_TestNamesAreShort keeps the checks readable in the report, which
// draws them as leaves of a tree and lines their results up in one column.
func TestRadiocli_TestNamesAreShort(t *testing.T) {
	for name, file := range suiteFiles(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}

			// t.Run("...", func(t *testing.T) { ... }), and nothing else that
			// happens to take a string first.
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Run" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			label, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if len(label) > nameLimit {
				t.Errorf("%s: the check %q is %d characters, and the limit is %d",
					name, label, len(label), nameLimit)
			}
			return true
		})
	}
}

// suiteFiles parses the suite's own test files.
func suiteFiles(t *testing.T) map[string]*ast.File {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("cannot read the suite's own directory: %v", err)
	}

	set := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		file, err := parser.ParseFile(set, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("cannot read %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = file
	}

	if len(files) == 0 {
		t.Fatal("found no test files to check, which cannot be right")
	}
	return files
}
