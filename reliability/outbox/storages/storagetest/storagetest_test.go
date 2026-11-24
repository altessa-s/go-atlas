// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"context"
	"sync"
	"testing"

	"github.com/altessa-s/go-atlas/reliability/outbox"
)

type contendedStore struct {
	outbox.Storage
	mu     sync.Mutex
	events []outbox.Event
	calls  int
}

func (s *contendedStore) SaveEvents(_ context.Context, events ...outbox.Event) error {
	s.events = events
	return nil
}

func (s *contendedStore) FetchUnprocessedEvents(_ context.Context, batch uint32) ([]outbox.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	// One batch succeeds before every worker observes a transient empty fetch.
	if s.calls > 1 && s.calls <= 2*concurrency {
		return nil, nil
	}
	n := min(len(s.events), int(batch))
	claimed := s.events[:n]
	s.events = s.events[n:]
	return claimed, nil
}

func TestConcurrentFetchWithSkippedLocks(t *testing.T) {
	ConcurrentFetch(t, &contendedStore{})
}
