// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type expirationStore struct {
	savedEvents  []Event
	expireCalls  atomic.Int64
	expireResult int64
	expireErr    error
}

func (s *expirationStore) FetchUnprocessedEvents(context.Context, uint32, time.Time) ([]Event, error) {
	return nil, nil
}
func (s *expirationStore) DeleteProcessedEvents(context.Context, time.Time) error { return nil }
func (s *expirationStore) UnlockStuckEvents(context.Context, time.Time) error     { return nil }
func (s *expirationStore) SaveEvents(_ context.Context, events ...Event) error {
	s.savedEvents = append(s.savedEvents, events...)
	return nil
}
func (s *expirationStore) UpdateEvents(context.Context, ...Event) error { return nil }
func (s *expirationStore) ExpireEvents(_ context.Context, _ time.Time) (int64, error) {
	s.expireCalls.Add(1)
	return s.expireResult, s.expireErr
}

func TestOutbox_Save_AppliesDefaultTTL(t *testing.T) {
	store := &expirationStore{}
	ttl := 5 * time.Minute

	ob := New(store, noopHandler, WithDefaultEventTTL(ttl))

	if err := ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if len(store.savedEvents) != 1 {
		t.Fatalf("saved %d events, want 1", len(store.savedEvents))
	}

	ev := store.savedEvents[0]
	if ev.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt should be set by default TTL")
	}

	expectedExpiry := ev.CreatedAt.Add(ttl)
	if !ev.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("ExpiresAt = %v, want %v", ev.ExpiresAt, expectedExpiry)
	}
}

func TestOutbox_Save_DoesNotOverrideExplicitExpiresAt(t *testing.T) {
	store := &expirationStore{}
	ttl := 5 * time.Minute
	customExpiry := time.Now().UTC().Add(1 * time.Hour)

	ob := New(store, noopHandler, WithDefaultEventTTL(ttl))

	if err := ob.Save(t.Context(), Event{
		Key:       "test",
		Payload:   []byte("data"),
		ExpiresAt: customExpiry,
	}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	ev := store.savedEvents[0]
	if !ev.ExpiresAt.Equal(customExpiry) {
		t.Fatalf("ExpiresAt = %v, want custom %v", ev.ExpiresAt, customExpiry)
	}
}

func TestOutbox_Save_NoTTLWhenDisabled(t *testing.T) {
	store := &expirationStore{}

	ob := New(store, noopHandler) // No TTL option

	if err := ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	ev := store.savedEvents[0]
	if !ev.ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt should be zero when TTL is disabled, got %v", ev.ExpiresAt)
	}
}

func TestWithDefaultEventTTL_IgnoresSubSecond(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(500*time.Millisecond))

	if err := ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	ev := store.savedEvents[0]
	if !ev.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt should be zero when TTL < 1s")
	}
}

func TestOutbox_RunExpireCycle(t *testing.T) {
	store := &expirationStore{expireResult: 3}
	ob := New(store, noopHandler)

	if err := ob.RunExpireCycle(t.Context()); err != nil {
		t.Fatalf("RunExpireCycle failed: %v", err)
	}

	if got := store.expireCalls.Load(); got != 1 {
		t.Fatalf("ExpireEvents calls = %d, want 1", got)
	}
}

func TestOutbox_RunExpireCycle_SchedulerManaged(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler)

	// Register with scheduler to make it managed
	_ = ob.RegisterExpireSchedulerFunc()

	err := ob.RunExpireCycle(t.Context())
	if err != ErrSchedulerManaged {
		t.Fatalf("RunExpireCycle should return ErrSchedulerManaged, got %v", err)
	}
}

func TestOutbox_RunExpireCycle_PropagatesStoreError(t *testing.T) {
	storeErr := errors.New("mongo unavailable")
	store := &expirationStore{expireErr: storeErr}
	ob := New(store, noopHandler)

	err := ob.RunExpireCycle(t.Context())
	if err == nil {
		t.Fatal("RunExpireCycle should return error")
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("error should wrap store error, got %v", err)
	}
}

func TestWithDefaultEventTTL_AcceptsExactOneSecond(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(time.Second))

	if err := ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	ev := store.savedEvents[0]
	if ev.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt should be set when TTL = 1s")
	}
}

func TestWithDefaultEventTTL_IgnoresZero(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(0))

	if err := ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	ev := store.savedEvents[0]
	if !ev.ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt should be zero when TTL = 0, got %v", ev.ExpiresAt)
	}
}
