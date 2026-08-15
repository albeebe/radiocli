// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// maxMacroName is the longest a macro's name may be.
//
// A macro's name is what a front end offering it has to label the control with.
// Forty characters is already a wide button; past that the name stops being a
// label and starts being a description, and the control it has to fit in cannot
// grow to match.
const maxMacroName = 40

// savedConfig reads the settings as the config file has them.
//
// It is a var so a test can fail the read back that follows a write. That one
// cannot be driven through the file: what Update has just written is valid JSON
// sitting in a readable file, so the read after it always succeeds, and the
// check that the write actually took is the only thing left unproven. Every
// other read in this package goes through it too, so there is one way in.
var savedConfig = func(app *appcontext.App) (*appcontext.Config, error) {
	return app.Config.Saved()
}

// notSettings are names somebody could reasonably try that this command will
// never accept, with the reason.
//
// They are worth answering properly. "no setting called wait" is true and
// unhelpful; the reason it is not a setting is the answer somebody is actually
// after, and it is written down in the Config type rather than anywhere they
// would think to look.
var notSettings = map[string]string{
	"device": "which scanner to talk to is a property of the invocation rather " +
		"than of this machine, so it is only ever the --device flag: run " +
		"\"radiocli devices\" to see what is attached",
	"wait": "how long to wait for another radiocli is a property of the caller " +
		"rather than of this machine, so it is only ever the --wait flag",
	"config": "the config file cannot name itself: pass --config to read or write " +
		"a different one",
	"macros": "a macro is a list of commands rather than a single value, and there " +
		"can be any number of them, so they have a command of their own: run " +
		"\"radiocli config macro\"",
	"macro": "a macro is a list of commands rather than a single value, and there " +
		"can be any number of them, so they have a command of their own: run " +
		"\"radiocli config macro\"",
}

// settings is every setting the config file holds.
//
// Kept in the order somebody would want to see them rather than
// alphabetically: the things that change how output looks first, then the
// noisy one.
var settings = []setting{
	{
		Name:    "output",
		Summary: "How results are rendered.",
		Values:  []string{string(appcontext.OutputText), string(appcontext.OutputJSON)},
		get:     func(c *appcontext.Config) string { return string(c.Output) },
		set: func(c *appcontext.Config, v string) error {
			c.Output = appcontext.OutputFormat(v)
			return nil
		},
	},
	{
		Name:    "pace",
		Summary: "How quickly keys are sent to the scanner.",
		Values:  paceValues(),
		get:     func(c *appcontext.Config) string { return string(c.Pace) },
		set: func(c *appcontext.Config, v string) error {
			c.Pace = device.Pace(v)
			return nil
		},
	},
	{
		Name:    "verbose",
		Summary: "Whether to log the exchange with the scanner.",
		Values:  []string{"true", "false"},
		get:     func(c *appcontext.Config) string { return strconv.FormatBool(c.Verbose) },
		set: func(c *appcontext.Config, v string) error {
			on, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return fmt.Errorf("invalid value %q: want true or false", v)
			}
			c.Verbose = on
			return nil
		},
	},
}

// report is one setting, as both output formats render it.
type report struct {
	// Name is the setting, called what it is called on the command line and in
	// the config file.
	Name string `json:"name"`

	// Value is what is in effect when no flag overrides it: the file's value,
	// or the default when the file does not have one.
	Value string `json:"value"`

	// Default is what the value would be with nothing set, so a reader can see
	// what unsetting it would get them without having to try it.
	Default string `json:"default"`

	// Changed reports whether Value and Default differ, which is the question
	// somebody is really asking when they look down the list.
	Changed bool `json:"changed"`
}

// setting is one thing that can be kept in the config file.
//
// The settings table is the only place a setting is named. A setting missing from
// it cannot be read, written or listed, which is what keeps the command from
// offering something the file does not actually hold.
type setting struct {
	// Name is what the setting is called on the command line. It matches the
	// key in the file, so what is reported and what is stored read alike.
	Name string

	// Summary is the one-line description shown when the settings are listed
	// and when a name is not recognised.
	Summary string

	// Values are the only accepted values, or nil when anything goes.
	Values []string

	// get reads the setting, written the way it is written in the file.
	get func(*appcontext.Config) string

	// set parses a value and applies it, or explains why it cannot.
	set func(*appcontext.Config, string) error
}
