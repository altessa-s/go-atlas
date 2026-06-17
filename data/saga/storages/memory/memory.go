// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/saga"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// Store is an in-process, concurrency-safe [saga.Store] backed by a map. It is
// intended for single-node deployments, tests, and as the reference
// implementation of the storage contract. State is lost on process exit; use a
// durable backend (MongoDB, Redis) for crash recovery across restarts.
type Store struct {
	mu        sync.RWMutex
	instances map[string]*saga.Instance
}

var _ saga.Store = (*Store)(nil)

// New returns an empty in-memory saga store.
func New() *Store {
	return &Store{instances: make(map[string]*saga.Instance)}
}

// Create stores a new instance, returning [sagaerrs.ErrInstanceExists] when one
// with the same ID is already present.
func (s *Store) Create(_ context.Context, inst *saga.Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.instances[inst.ID]; ok {
		return sagaerrs.ErrInstanceExists
	}
	s.instances[inst.ID] = inst.Clone()
	return nil
}

// Get returns a copy of the stored instance or [sagaerrs.ErrInstanceNotFound].
func (s *Store) Get(_ context.Context, id string) (*saga.Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inst, ok := s.instances[id]
	if !ok {
		return nil, sagaerrs.ErrInstanceNotFound
	}
	return inst.Clone(), nil
}

// Update overwrites the stored instance using optimistic concurrency: it
// returns [sagaerrs.ErrVersionConflict] when the supplied Version no longer
// matches the stored value, and [sagaerrs.ErrInstanceNotFound] when the
// instance is gone. On success it bumps the stored version and writes the new
// value back into inst.Version.
func (s *Store) Update(_ context.Context, inst *saga.Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.instances[inst.ID]
	if !ok {
		return sagaerrs.ErrInstanceNotFound
	}
	if cur.Version != inst.Version {
		return sagaerrs.ErrVersionConflict
	}

	stored := inst.Clone()
	stored.Version = cur.Version + 1
	s.instances[inst.ID] = stored
	inst.Version = stored.Version
	return nil
}

// FetchRecoverable returns copies of up to limit non-terminal instances that
// are mid-compensation or whose deadline has passed. A non-positive limit means
// no cap.
func (s *Store) FetchRecoverable(_ context.Context, now time.Time, limit int) ([]*saga.Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*saga.Instance
	for _, inst := range s.instances {
		if inst.Status.IsTerminal() {
			continue
		}
		timedOut := !inst.Deadline.IsZero() && !now.Before(inst.Deadline)
		if inst.Status != saga.StatusCompensating && !timedOut {
			continue
		}
		out = append(out, inst.Clone())
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Delete removes an instance. Deleting a missing instance is not an error.
func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	delete(s.instances, id)
	s.mu.Unlock()
	return nil
}

// Len returns the number of stored instances. It is intended for tests and
// metrics, not for control flow.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.instances)
}
