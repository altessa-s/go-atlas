// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

func TestNew_Valid(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{
			name: "SingleValue",
			values: map[string]string{
				"key1": "value1",
			},
		},
		{
			name: "MultipleValues",
			values: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		{
			name: "ValidKeyCharacters",
			values: map[string]string{
				"valid_key-123.test": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := memory.New(tt.values)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if store == nil {
				t.Fatal("expected non-nil storage")
			}
		})
	}
}

func TestNew_InvalidKey(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{
			name: "InvalidCharacter_Exclamation",
			values: map[string]string{
				"!invalid": "value",
			},
		},
		{
			name: "InvalidCharacter_Space",
			values: map[string]string{
				"invalid key": "value",
			},
		},
		{
			name: "InvalidCharacter_AtSign",
			values: map[string]string{
				"invalid@key": "value",
			},
		},
		{
			name: "EmptyKey",
			values: map[string]string{
				"": "value",
			},
		},
		{
			name: "SingleCharacterKey",
			values: map[string]string{
				"k": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := memory.New(tt.values)
			if err == nil {
				t.Fatal("expected error for invalid key, got nil")
			}
			if store != nil {
				t.Fatal("expected nil storage on error")
			}
		})
	}
}

func TestNew_Empty(t *testing.T) {
	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("expected no error for empty map, got %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil storage")
	}
}

func TestStorage_Name(t *testing.T) {
	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	name := store.Name()
	if name != "memory" {
		t.Errorf("expected name 'memory', got '%s'", name)
	}
}

func TestStorage_IsStatic(t *testing.T) {
	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if !store.IsStatic() {
		t.Error("expected IsStatic to return true")
	}
}

func TestStorage_Value(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"existing-key": "secret-value",
		"another-key":  "another-value",
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Run("ExistingKey", func(t *testing.T) {
		val, err := store.Value(ctx, "existing-key")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if val == nil {
			t.Fatal("expected non-nil value")
		}
		if val.Key != "existing-key" {
			t.Errorf("expected key 'existing-key', got '%s'", val.Key)
		}
		if val.Value != "secret-value" {
			t.Errorf("expected value 'secret-value', got '%s'", val.Value)
		}
	})

	t.Run("MissingKey", func(t *testing.T) {
		val, err := store.Value(ctx, "missing-key")
		if !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
		if val != nil {
			t.Error("expected nil value for missing key")
		}
	})
}

func TestStorage_Value_InvalidKey(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	tests := []struct {
		name string
		key  string
	}{
		{
			name: "InvalidCharacter",
			key:  "!invalid",
		},
		{
			name: "EmptyKey",
			key:  "",
		},
		{
			name: "WhitespaceOnly",
			key:  "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := store.Value(ctx, tt.key)
			if !errors.Is(err, secrets.ErrInvalidKey) {
				t.Fatalf("expected ErrInvalidKey, got %v", err)
			}
			if val != nil {
				t.Error("expected nil value for invalid key")
			}
		})
	}
}

func TestStorage_Value_NilContext(t *testing.T) {
	store, err := memory.New(map[string]string{
		"key1": "value1",
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	val, err := store.Value(nil, "key1")
	if err != nil {
		t.Fatalf("expected no error for nil context, got %v", err)
	}
	if val == nil || val.Value != "value1" {
		t.Error("expected valid value for nil context")
	}
}

func TestStorage_Save(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Run("SaveNewKey", func(t *testing.T) {
		err := store.Save(ctx, "new-key", "new-value")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify with Value
		val, err := store.Value(ctx, "new-key")
		if err != nil {
			t.Fatalf("failed to retrieve saved value: %v", err)
		}
		if val.Value != "new-value" {
			t.Errorf("expected value 'new-value', got '%s'", val.Value)
		}
	})

	t.Run("UpdateExistingKey", func(t *testing.T) {
		err := store.Save(ctx, "new-key", "updated-value")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify with Value
		val, err := store.Value(ctx, "new-key")
		if err != nil {
			t.Fatalf("failed to retrieve updated value: %v", err)
		}
		if val.Value != "updated-value" {
			t.Errorf("expected value 'updated-value', got '%s'", val.Value)
		}
	})

	t.Run("InvalidKey", func(t *testing.T) {
		err := store.Save(ctx, "!invalid", "value")
		if !errors.Is(err, secrets.ErrInvalidKey) {
			t.Fatalf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		err := store.Save(nil, "key", "value")
		if err != nil {
			t.Fatalf("expected no error for nil context, got %v", err)
		}
	})
}

func TestStorage_Delete(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"existing-key": "value",
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Run("DeleteExistingKey", func(t *testing.T) {
		err := store.Delete(ctx, "existing-key")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify key is deleted
		val, err := store.Value(ctx, "existing-key")
		if !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}
		if val != nil {
			t.Error("expected nil value after delete")
		}
	})

	t.Run("DeleteMissingKey", func(t *testing.T) {
		err := store.Delete(ctx, "missing-key")
		if !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for missing key, got %v", err)
		}
	})

	t.Run("InvalidKey", func(t *testing.T) {
		err := store.Delete(ctx, "!invalid")
		if !errors.Is(err, secrets.ErrInvalidKey) {
			t.Fatalf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		err := store.Delete(nil, "missing-key")
		if !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for nil context, got %v", err)
		}
	})
}

func TestStorage_List(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Run("ListAll", func(t *testing.T) {
		values, err := store.List(ctx)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(values) != 3 {
			t.Fatalf("expected 3 values, got %d", len(values))
		}

		// Verify all keys are present
		keys := make(map[string]bool)
		for _, v := range values {
			keys[v.Key] = true
		}

		expectedKeys := []string{"key1", "key2", "key3"}
		for _, key := range expectedKeys {
			if !keys[key] {
				t.Errorf("expected key '%s' in list", key)
			}
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		values, err := store.List(nil)
		if err != nil {
			t.Fatalf("expected no error for nil context, got %v", err)
		}
		if len(values) != 3 {
			t.Errorf("expected 3 values for nil context, got %v", len(values))
		}
	})
}

func TestStorage_List_Empty(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	values, err := store.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if values == nil {
		t.Fatal("expected non-nil slice for empty storage")
	}

	if len(values) != 0 {
		t.Fatalf("expected empty slice, got %d values", len(values))
	}
}

func TestStorage_Values(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Run("IterateAll", func(t *testing.T) {
		collected := make(map[string]string)
		for val, err := range store.Values(ctx) {
			if err != nil {
				t.Fatalf("unexpected error during iteration: %v", err)
			}
			if val == nil {
				t.Fatal("unexpected nil value during iteration")
			}
			collected[val.Key] = val.Value
		}

		if len(collected) != 3 {
			t.Fatalf("expected 3 values, got %d", len(collected))
		}

		expected := map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}

		for key, expectedValue := range expected {
			if actualValue, ok := collected[key]; !ok {
				t.Errorf("expected key '%s' in results", key)
			} else if actualValue != expectedValue {
				t.Errorf("key '%s': expected value '%s', got '%s'", key, expectedValue, actualValue)
			}
		}
	})

	t.Run("EarlyTermination", func(t *testing.T) {
		count := 0
		for _, err := range store.Values(ctx) {
			if err != nil {
				t.Fatalf("unexpected error during iteration: %v", err)
			}
			count++
			if count >= 2 {
				break
			}
		}

		if count != 2 {
			t.Errorf("expected to iterate 2 times, got %d", count)
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		count := 0
		for val, err := range store.Values(nil) {
			if err != nil {
				t.Fatalf("expected no error for nil context, got %v", err)
			}
			if val == nil {
				t.Error("expected valid value for nil context")
			}
			count++
		}
		if count != 3 {
			t.Errorf("expected 3 items for nil context, got %d", count)
		}
	})

	t.Run("EmptyStorage", func(t *testing.T) {
		emptyStore, err := memory.New(map[string]string{})
		if err != nil {
			t.Fatalf("failed to create storage: %v", err)
		}

		count := 0
		for _, err := range emptyStore.Values(ctx) {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			count++
		}

		if count != 0 {
			t.Errorf("expected 0 iterations for empty storage, got %d", count)
		}
	})
}

func TestStorage_CheckConnection(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	err = store.CheckConnection(ctx)
	if err != nil {
		t.Errorf("expected nil error from CheckConnection, got %v", err)
	}
}

func TestStorage_Concurrent(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	const numGoroutines = 100
	const numOperations = 10

	var wg sync.WaitGroup

	// Concurrent Save operations
	for range numGoroutines {
		wg.Go(func() {
			for range numOperations {
				key := "concurrent-key"
				value := "value"
				if err := store.Save(ctx, key, value); err != nil {
					t.Errorf("concurrent Save failed: %v", err)
				}
			}
		})
	}

	// Concurrent Value operations
	for range numGoroutines {
		wg.Go(func() {
			for range numOperations {
				key := "concurrent-key"
				_, _ = store.Value(ctx, key)
			}
		})
	}

	// Concurrent Delete operations
	for range numGoroutines {
		wg.Go(func() {
			for range numOperations {
				key := "concurrent-key"
				_ = store.Delete(ctx, key)
			}
		})
	}

	wg.Wait()

	// If we got here without data races or panics, the test passes
}
