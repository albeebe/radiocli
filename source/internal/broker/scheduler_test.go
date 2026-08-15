// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/10/2026

package broker

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestTurnsAreTakenInOrder is the promise a lease depends on. A caller that has
// waited out a long macro must not lose its turn to a command that arrived a
// moment ago, or waiting would be worse than failing.
func TestTurnsAreTakenInOrder(t *testing.T) {
	var s scheduler
	if err := s.acquire(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	const waiters = 5
	var (
		mu     sync.Mutex
		order  []int
		queued sync.WaitGroup
		done   sync.WaitGroup
	)

	for i := range waiters {
		queued.Add(1)
		done.Add(1)
		go func() {
			defer done.Done()
			// Each goroutine announces itself only once it is actually in the
			// queue, so the test builds a known order rather than whatever
			// the scheduler happened to see.
			queued.Done()
			if err := s.acquire(context.Background()); err != nil {
				t.Errorf("waiter %d: %v", i, err)
				return
			}
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			s.release()
		}()
		queued.Wait()
		// Wait for this one to be counted before starting the next, so the
		// queue is built in a defined order.
		for s.queued() != i+1 {
			time.Sleep(time.Millisecond)
		}
	}

	s.release()
	done.Wait()

	for i, got := range order {
		if got != i {
			t.Fatalf("turns were taken in order %v, wanted 0 to %d in order", order, waiters-1)
		}
	}
}

// TestTryDoesNotWait covers the display mirror's whole way of giving way.
func TestTryDoesNotWait(t *testing.T) {
	var s scheduler

	if !s.tryAcquire() {
		t.Fatal("a free scanner refused a try")
	}
	if s.tryAcquire() {
		t.Error("a held scanner was handed out to a try")
	}

	s.release()
	if !s.tryAcquire() {
		t.Error("a released scanner refused a try")
	}
}

// TestTryRefusesWhileAnybodyIsWaiting is what stops a reading every tenth of a
// second from starving the commands. Without it a mirror could take every gap
// between two queued commands forever.
func TestTryRefusesWhileAnybodyIsWaiting(t *testing.T) {
	var s scheduler
	if err := s.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	waiting := make(chan struct{})
	go func() {
		close(waiting)
		s.acquire(context.Background())
		s.release()
	}()

	<-waiting
	for s.queued() == 0 {
		time.Sleep(time.Millisecond)
	}

	if s.tryAcquire() {
		t.Error("a try was let in front of a caller that was already waiting")
	}
	s.release()
}

// TestGivingUpDoesNotWedgeTheQueue covers a client disconnecting or pressing
// Ctrl-C while queued. The place is abandoned and everybody behind it still
// gets a turn.
func TestGivingUpDoesNotWedgeTheQueue(t *testing.T) {
	var s scheduler
	if err := s.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	gaveUp := make(chan error, 1)
	go func() { gaveUp <- s.acquire(ctx) }()
	for s.queued() != 1 {
		time.Sleep(time.Millisecond)
	}

	got := make(chan struct{})
	go func() {
		if err := s.acquire(context.Background()); err != nil {
			t.Errorf("the caller behind the abandoned one: %v", err)
		}
		close(got)
	}()
	for s.queued() != 2 {
		time.Sleep(time.Millisecond)
	}

	cancel()
	if err := <-gaveUp; err == nil {
		t.Error("a cancelled acquire reported success")
	}

	s.release()

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("the caller behind an abandoned place never got its turn")
	}
	s.release()
}

// TestAScannerIsNeverDroppedOnTheFloor covers the race the cancel path exists
// for: a waiter cancelled at the very moment it was handed the scanner still
// has to pass it on, or everybody behind it waits forever.
//
// The race is provoked by cancelling and releasing at once, many times over.
func TestAScannerIsNeverDroppedOnTheFloor(t *testing.T) {
	for range 200 {
		var s scheduler
		if err := s.acquire(context.Background()); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		var racing sync.WaitGroup
		racing.Add(1)
		go func() {
			defer racing.Done()
			// Winning the race means owning the scanner, cancelled or not,
			// so this has to give it back. Both outcomes are correct; what
			// must never happen is neither.
			if err := s.acquire(ctx); err == nil {
				s.release()
			}
		}()
		for s.queued() != 1 {
			time.Sleep(time.Microsecond)
		}

		racing.Add(2)
		go func() { defer racing.Done(); cancel() }()
		go func() { defer racing.Done(); s.release() }()
		racing.Wait()

		// Whatever happened, the scanner has to be reachable again.
		done := make(chan struct{})
		go func() {
			s.acquire(context.Background())
			close(done)
		}()
		select {
		case <-done:
			s.release()
		case <-time.After(2 * time.Second):
			t.Fatal("the scanner was left held by a caller that had gone away")
		}
	}
}

// TestALeaseIsReleasedOnce covers the timer and an explicit release both
// firing, which would otherwise hand the scanner on twice.
func TestALeaseIsReleasedOnce(t *testing.T) {
	var expired int
	l := newLease(time.Hour, func(*lease) { expired++ })

	if !l.take() {
		t.Fatal("the first take was refused")
	}
	if l.take() {
		t.Error("a lease was taken twice")
	}
	if expired != 0 {
		t.Error("the expiry ran for a lease that was released first")
	}
}

// TestATakenLeaseWaitsOutItsRuns covers the race between an expiry and a
// command already on the serial line. Whoever takes the lease back must wait
// in drain until the run has ended, and a run arriving after the take must be
// refused rather than let through on a lease that is already gone.
func TestATakenLeaseWaitsOutItsRuns(t *testing.T) {
	l := newLease(time.Hour, func(*lease) {})
	if !l.begin() {
		t.Fatal("a live lease refused a run")
	}

	if !l.take() {
		t.Fatal("the take was refused")
	}
	drained := make(chan struct{})
	go func() {
		l.drain()
		close(drained)
	}()

	// The drain must be waiting, not returning, while the run is still on the
	// lease. A sleep long enough for the goroutine to reach the wait is what
	// makes the select below a real check rather than a coin toss.
	time.Sleep(20 * time.Millisecond)
	select {
	case <-drained:
		t.Fatal("drain returned with a run still on the lease")
	default:
	}

	if l.begin() {
		t.Error("a taken lease accepted a new run")
	}

	l.end()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("drain never woke after the last run ended")
	}
}

// TestALeaseExpires covers a client that holds the scanner and stops paying
// attention.
func TestALeaseExpires(t *testing.T) {
	expired := make(chan struct{})
	newLease(10*time.Millisecond, func(*lease) { close(expired) })

	select {
	case <-expired:
	case <-time.After(2 * time.Second):
		t.Fatal("a lease with a ttl was still held after it")
	}
}
