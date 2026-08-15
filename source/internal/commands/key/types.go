// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package key

import (
	"github.com/albeebe/radiocli/internal/device"
)

// actions maps the names this command accepts to the ways a key can be
// pressed.
var actions = map[string]device.KeyAction{
	"press":   device.KeyPress,
	"long":    device.KeyLong,
	"hold":    device.KeyHeld,
	"release": device.KeyRelease,
}

// keys maps the names this command accepts to the keys the scanner knows.
//
// The names are what the key is called on the scanner or in the manual, not
// the single letters the protocol uses, because those are an implementation
// detail nobody should have to memorise.
var keys = map[string]device.Key{
	"menu":     device.KeyMenu,
	"function": device.KeyFunction,
	"avoid":    device.KeyAvoid,
	"enter":    device.KeyEnter,
	"yes":      device.KeyEnter,
	"no":       device.KeyNo,

	"right": device.KeyRotateRight,
	"left":  device.KeyRotateLeft,
	"push":  device.KeyRotatePush,

	"soft1": device.KeySoft1,
	"soft2": device.KeySoft2,
	"soft3": device.KeySoft3,

	"replay":       device.KeyReplay,
	"zip":          device.KeyZip,
	"range":        device.KeyRange,
	"service-type": device.KeyServiceType,
	"backlight":    device.KeyBacklight,
	"squelch":      device.KeySquelchPush,

	"0": device.Key0,
	"1": device.Key1,
	"2": device.Key2,
	"3": device.Key3,
	"4": device.Key4,
	"5": device.Key5,
	"6": device.Key6,
	"7": device.Key7,
	"8": device.Key8,
	"9": device.Key9,
}

// press is one key and the name it was given by, kept together so a failure
// halfway through a run can say which key it was on.
type press struct {
	// name is what the user called the key, lowercased, which is what a
	// failure message names rather than the protocol's letter.
	name string

	// key is the key the scanner knows, which is what actually gets sent.
	key device.Key
}

// report is what a run writes under --output json.
//
// It says what was asked for rather than what the scanner did with it, and that
// is the honest limit of this command: what a key does depends entirely on what
// was on screen when it landed, and nothing here can find that out. A caller
// wanting the result reads the screen afterwards.
//
// It exists because empty stdout is not something a decoder can read. In text
// mode the command still says nothing, which is right there, since the result
// of pressing a key is on the scanner's own screen.
type report struct {
	// Keys are the keys that were pressed, in order, by the names the caller
	// used rather than the protocol's letters.
	Keys []string `json:"keys"`

	// Action is how they were pressed, lowercased: "press", "long", "hold" or
	// "release".
	Action string `json:"action"`
}
