// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/13/2026

package sites

import (
	"os"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/menus"
)

// TestMain shortens the wait for a scanner to come back from rebuilding its
// database.
//
// The commands in this package send the scanner away to rebuild, and wait for
// it afterwards. That wait is a budget in time, so the tests that drive a
// scanner which never answers would each sit through the whole of it; the fake
// fails in microseconds and there is nothing to wait for.
func TestMain(m *testing.M) {
	menus.AwakenBudget = 10 * time.Millisecond
	menus.AwakenGap = time.Millisecond

	os.Exit(m.Run())
}
