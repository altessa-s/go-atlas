// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/security/secrets"

	filtermemory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
	secretmemory "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

func TestManager_NegativeCache(t *testing.T) {
	ctx := t.Context()

	// Create a memory provider with one secret
	provider, _ := secretmemory.New(map[string]string{
		"existing-key": "secret-value",
	})

	// Create a bloom filter for negative caching
	storage := filtermemory.New(filtermemory.WithExpectedItems(100))
	filter := bloom.New(storage)

	// Pre-fill the filter with existing keys
	_ = filter.Add(ctx, "existing-key")

	// Create manager with negative filter
	mgr, err := secrets.New[string](provider,
		secrets.WithNegativeFilter(filter),
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	t.Run("ExistingKey", func(t *testing.T) {
		val, err := mgr.Value(ctx, "existing-key", true)
		if err != nil {
			t.Fatalf("expected no error for existing key, got %v", err)
		}
		if val.Value != "secret-value" {
			t.Errorf("expected secret-value, got %s", val.Value)
		}
	})

	t.Run("NonExistentKey_NegativeFilterHit", func(t *testing.T) {
		// This key is NOT in the filter, so it should return ErrNotFound immediately
		_, err := mgr.Value(ctx, "non-existent-key", true)
		if !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("expected ErrNotFound from negative filter, got %v", err)
		}
	})

	t.Run("SaveUpdatesNegativeFilter", func(t *testing.T) {
		// Save a new secret
		err := mgr.Save(ctx, "new-key", "new-value")
		if err != nil {
			t.Fatalf("failed to save secret: %v", err)
		}

		// Verify it's now in the negative filter and retrievable
		val, err := mgr.Value(ctx, "new-key", true)
		if err != nil {
			t.Fatalf("expected no error for new key after save, got %v", err)
		}
		if val.Value != "new-value" {
			t.Errorf("expected new-value, got %s", val.Value)
		}
	})

	t.Run("RunUpdateCycleRebuildsFilter", func(t *testing.T) {
		// Add a key directly to provider bypassing manager
		_ = provider.Save(ctx, "manual-key", "manual-value")

		// Confirm it's NOT in the filter yet (should hit negative filter)
		_, err := mgr.Value(ctx, "manual-key", true)
		if !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("expected ErrNotFound from negative filter BEFORE update, got %v", err)
		}

		// Run update cycle to rebuild filter
		if err := mgr.RunUpdateCycle(ctx); err != nil {
			t.Fatalf("RunUpdateCycle failed: %v", err)
		}

		// Now it should be found
		val, err := mgr.Value(ctx, "manual-key", true)
		if err != nil {
			t.Fatalf("expected no error for manual key AFTER update, got %v", err)
		}
		if val.Value != "manual-value" {
			t.Errorf("expected manual-value, got %s", val.Value)
		}
	})
}
