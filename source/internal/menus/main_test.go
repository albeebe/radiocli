// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/13/2026

package menus

import (
	"os"
	"testing"
	"time"
)

// TestMain shortens the wait for a scanner to come back from rebuilding its
// database.
//
// Awaken's budget is a length of time, so a test driving a scanner that never
// answers would sit through the whole of it. The tests that care about the
// budget set their own and put these back.
func TestMain(m *testing.M) {
	AwakenBudget = 10 * time.Millisecond
	AwakenGap = time.Millisecond

	os.Exit(m.Run())
}
