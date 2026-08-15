// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/10/2026

package broker

import (
	"context"
	"testing"
	"time"
)

// settle is how long a test waits before deciding a daemon is not going to
// stop. It only has to outlast a wakeup, since nothing here is timed.
const settle = 150 * time.Millisecond

// TestADaemonOutlivesTheProcessThatStartedIt is the rule that makes a daemon
// shareable rather than owned.
//
// Whoever starts a daemon is very often not the last one using it: a page
// starts one, a second page joins it, and the first page is closed. Stopping
// then would take the scanner away from the second page, which never knew which
// process happened to spawn the thing it is talking to.
func TestADaemonOutlivesTheProcessThatStartedIt(t *testing.T) {
	s := &Server{}
	s.clients.add(1)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	orphaned := make(chan struct{})
	go s.stopWhenUnused(ctx, orphaned, stop)

	close(orphaned)

	select {
	case <-ctx.Done():
		t.Fatal("the daemon closed the scanner while a client was still connected")
	case <-time.After(settle):
	}

	// And once that client goes, there is nobody left to hold it for.
	s.clients.add(-1)

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the daemon kept the scanner after its last client had gone")
	}
}

// TestAnOrphanedDaemonNobodyIsUsingStopsAtOnce is the other half: nothing is
// connected, so there is nobody to wait for.
func TestAnOrphanedDaemonNobodyIsUsingStopsAtOnce(t *testing.T) {
	s := &Server{}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	orphaned := make(chan struct{})
	go s.stopWhenUnused(ctx, orphaned, stop)
	close(orphaned)

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("a daemon with no clients and no starter kept the scanner")
	}
}

// TestAnIdleDaemonKeepsGoingWhileItsStarterIsThere checks that being unused is
// not on its own a reason to stop.
//
// A daemon started in a terminal sits with nothing connected for as long as
// somebody leaves it there, and the whole point of it is to be waiting when a
// command eventually arrives.
func TestAnIdleDaemonKeepsGoingWhileItsStarterIsThere(t *testing.T) {
	s := &Server{}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	// Never closed, which is what a living parent looks like.
	orphaned := make(chan struct{})
	go s.stopWhenUnused(ctx, orphaned, stop)

	select {
	case <-ctx.Done():
		t.Fatal("an idle daemon stopped with its starter still running")
	case <-time.After(settle):
	}
}

// TestTheClientCountWakesAWaiterThatArrivedFirst covers the ordering the count
// exists to get right.
//
// A waiter reads the count and then waits to be told it moved. If those were
// two separate steps, a client disconnecting in between would leave the waiter
// waiting for a change that had already happened, and the daemon would hold the
// scanner for ever.
func TestTheClientCountWakesAWaiterThatArrivedFirst(t *testing.T) {
	var c clients
	c.add(2)

	count, changed := c.watch()
	if count != 2 {
		t.Fatalf("the count is %d, wanted 2", count)
	}

	c.add(-1)

	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("a client disconnecting did not wake the waiter")
	}

	if count, _ := c.watch(); count != 1 {
		t.Errorf("the count is %d after one client left of two, wanted 1", count)
	}
}
