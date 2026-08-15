// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package broker

import (
	"context"
	"sync"
	"time"
)

// newLease starts the expiry timer. onExpire runs at most once, and not at all
// if the lease is released first.
//
// The timer is armed with the lock already held, because a short enough ttl
// fires before this function has finished storing the timer it is meant to
// stop. Holding the lock makes the callback wait for the field it reads.
//
// onExpire is handed the lease itself rather than closing over it, because the
// variable the caller assigns this result to is not written until after the
// timer is armed, and a short enough ttl would fire the callback into a read
// of a variable still being assigned.
//
// Parameters:
//   - ttl: how long the lease may be held before it is taken back
//   - onExpire: what to do when it is, which runs at most once and is handed
//     the lease it is running for
//
// Returns:
//   - the lease, already counting down
func newLease(ttl time.Duration, onExpire func(*lease)) *lease {
	l := &lease{}
	l.idle = sync.NewCond(&l.mu)
	l.mu.Lock()
	defer l.mu.Unlock()

	l.timer = time.AfterFunc(ttl, func() {
		if l.take() {
			onExpire(l)
		}
	})
	return l
}

// acquire waits for the scanner and returns once it is held.
//
// A cancelled context gives up the place in the queue. The error is the
// context's own, so a caller can tell being interrupted from anything else.
//
// Parameters:
//   - ctx: cancelling it gives up the place in the queue
//
// Returns:
//   - error the context's own if the wait was given up, and nil once the
//     scanner is held
func (s *scheduler) acquire(ctx context.Context) error {
	s.mu.Lock()
	if !s.held && len(s.waiting) == 0 {
		s.held = true
		s.mu.Unlock()
		return nil
	}

	w := &waiter{ready: make(chan struct{})}
	s.waiting = append(s.waiting, w)
	s.mu.Unlock()

	select {
	case <-w.ready:
		return nil
	case <-ctx.Done():
		// Losing the race matters here. The releaser may have already woken
		// this waiter, in which case the scanner is now held by a caller that
		// has gone away, and dropping it on the floor would wedge everybody
		// behind it. So the flag is set under the lock and the channel is
		// re-checked: either this got there first and the place is simply
		// abandoned, or it did not and the scanner has to be passed on.
		s.mu.Lock()
		select {
		case <-w.ready:
			s.mu.Unlock()
			s.release()
		default:
			w.abandoned = true
			s.mu.Unlock()
		}
		return ctx.Err()
	}
}

// begin registers one more command as running under this lease, and reports
// whether it may. A lease that has already been taken back refuses, and the
// refused caller queues with the scheduler like anybody else.
//
// Returns:
//   - whether the run was registered, which is what obliges whoever takes
//     this lease back to wait for it before handing the scanner on
func (l *lease) begin() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return false
	}
	l.runs++
	return true
}

// drain waits until no command is running under this lease.
//
// It is called by whoever won take, before the scanner is handed on. Handing
// it on with a run still in flight would put two commands on one serial line
// at once, each reading the other's replies, which is the collision the
// scheduler exists to prevent and the one place the lease used to allow it.
func (l *lease) drain() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for l.runs > 0 {
		l.idle.Wait()
	}
}

// end unregisters a command that begin let through, and wakes whoever is
// waiting in drain when the last one leaves.
func (l *lease) end() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.runs--
	if l.runs == 0 {
		l.idle.Broadcast()
	}
}

// queued reports how many callers are waiting, for the daemon to log and for
// tests to assert on.
//
// Returns:
//   - how many callers are in the queue, not counting whoever holds the scanner
func (s *scheduler) queued() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.waiting)
}

// release hands the scanner to the next in line, or leaves it free.
func (s *scheduler) release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.waiting) > 0 {
		w := s.waiting[0]
		s.waiting = s.waiting[1:]
		if w.abandoned {
			continue
		}
		// Ownership passes straight to this waiter: held stays true.
		close(w.ready)
		return
	}

	s.held = false
}

// take reports whether the caller is the one that gets to release this lease,
// and is true exactly once.
//
// Returns:
//   - whether this caller is the one that gets to release the lease
func (l *lease) take() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return false
	}
	l.released = true
	l.timer.Stop()
	return true
}

// tryAcquire takes the scanner only if it is free this instant, and reports
// whether it got it.
//
// It refuses when anybody is queued as well as when the scanner is held. A
// caller that would not wait must not be able to step in front of one that is
// waiting, which is what makes a mirror reading every tenth of a second safe
// to run alongside commands rather than something that starves them.
//
// Returns:
//   - whether the scanner was free this instant and is now held
func (s *scheduler) tryAcquire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.held || len(s.waiting) > 0 {
		return false
	}
	s.held = true
	return true
}
