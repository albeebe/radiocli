// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/22/2026

package receiving

import "github.com/albeebe/radiocli/internal/device"

// report is the result this command renders.
//
// It is device.Heard rather than a type of this package's own, because the
// recorder reads this command's JSON back over a daemon socket to label a
// transmission. Two definitions of the same object would be two things to keep
// in step, and a command package may not import another command package to
// share one, so the shape lives a layer down where both can reach it.
type report = device.Heard
