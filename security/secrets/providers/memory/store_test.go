// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

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
			require.NoError(t, err)
			require.NotNil(t, store)
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
			require.Error(t, err)
			require.Nil(t, store)
		})
	}
}

func TestNew_Empty(t *testing.T) {
	store, err := memory.New(map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, store)
}

func TestStorage_Name(t *testing.T) {
	store, err := memory.New(map[string]string{})
	require.NoError(t, err)
	require.Equal(t, "memory", store.Name())
}

func TestStorage_IsStatic(t *testing.T) {
	store, err := memory.New(map[string]string{})
	require.NoError(t, err)
	require.True(t, store.IsStatic())
}

func TestStorage_Value(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"existing-key": "secret-value",
		"another-key":  "another-value",
	})
	require.NoError(t, err)

	t.Run("ExistingKey", func(t *testing.T) {
		val, err := store.Value(ctx, "existing-key")
		require.NoError(t, err)
		require.NotNil(t, val)
		require.Equal(t, "existing-key", val.Key)
		require.Equal(t, "secret-value", val.Value)
	})

	t.Run("MissingKey", func(t *testing.T) {
		val, err := store.Value(ctx, "missing-key")
		require.ErrorIs(t, err, secrets.ErrNotFound)
		require.Nil(t, val)
	})
}

func TestStorage_Value_InvalidKey(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	require.NoError(t, err)

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
			require.ErrorIs(t, err, secrets.ErrInvalidKey)
			require.Nil(t, val)
		})
	}
}

func TestStorage_Value_NilContext(t *testing.T) {
	store, err := memory.New(map[string]string{
		"key1": "value1",
	})
	require.NoError(t, err)

	val, err := store.Value(nil, "key1")
	require.NoError(t, err)
	require.NotNil(t, val)
	require.Equal(t, "value1", val.Value)
}

func TestStorage_Save(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	require.NoError(t, err)

	t.Run("SaveNewKey", func(t *testing.T) {
		err := store.Save(ctx, "new-key", "new-value")
		require.NoError(t, err)

		// Verify with Value
		val, err := store.Value(ctx, "new-key")
		require.NoError(t, err)
		require.Equal(t, "new-value", val.Value)
	})

	t.Run("UpdateExistingKey", func(t *testing.T) {
		err := store.Save(ctx, "new-key", "updated-value")
		require.NoError(t, err)

		// Verify with Value
		val, err := store.Value(ctx, "new-key")
		require.NoError(t, err)
		require.Equal(t, "updated-value", val.Value)
	})

	t.Run("InvalidKey", func(t *testing.T) {
		err := store.Save(ctx, "!invalid", "value")
		require.ErrorIs(t, err, secrets.ErrInvalidKey)
	})

	t.Run("NilContext", func(t *testing.T) {
		err := store.Save(nil, "key", "value")
		require.NoError(t, err)
	})
}

func TestStorage_Delete(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"existing-key": "value",
	})
	require.NoError(t, err)

	t.Run("DeleteExistingKey", func(t *testing.T) {
		err := store.Delete(ctx, "existing-key")
		require.NoError(t, err)

		// Verify key is deleted
		val, err := store.Value(ctx, "existing-key")
		require.ErrorIs(t, err, secrets.ErrNotFound)
		require.Nil(t, val)
	})

	t.Run("DeleteMissingKey", func(t *testing.T) {
		err := store.Delete(ctx, "missing-key")
		require.ErrorIs(t, err, secrets.ErrNotFound)
	})

	t.Run("InvalidKey", func(t *testing.T) {
		err := store.Delete(ctx, "!invalid")
		require.ErrorIs(t, err, secrets.ErrInvalidKey)
	})

	t.Run("NilContext", func(t *testing.T) {
		err := store.Delete(nil, "missing-key")
		require.ErrorIs(t, err, secrets.ErrNotFound)
	})
}

func TestStorage_List(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	})
	require.NoError(t, err)

	t.Run("ListAll", func(t *testing.T) {
		values, err := store.List(ctx)
		require.NoError(t, err)
		require.Len(t, values, 3)

		// Verify all keys are present
		keys := make(map[string]bool)
		for _, v := range values {
			keys[v.Key] = true
		}

		expectedKeys := []string{"key1", "key2", "key3"}
		for _, key := range expectedKeys {
			require.True(t, keys[key], "expected key '%s' in list", key)
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		values, err := store.List(nil)
		require.NoError(t, err)
		require.Len(t, values, 3)
	})
}

func TestStorage_List_Empty(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	require.NoError(t, err)

	values, err := store.List(ctx)
	require.NoError(t, err)
	require.NotNil(t, values)
	require.Len(t, values, 0)
}

func TestStorage_Values(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	})
	require.NoError(t, err)

	t.Run("IterateAll", func(t *testing.T) {
		collected := make(map[string]string)
		for val, err := range store.Values(ctx) {
			require.NoError(t, err)
			require.NotNil(t, val)
			collected[val.Key] = val.Value
		}

		require.Len(t, collected, 3)

		expected := map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}

		for key, expectedValue := range expected {
			actualValue, ok := collected[key]
			require.True(t, ok, "expected key '%s' in results", key)
			require.Equal(t, expectedValue, actualValue)
		}
	})

	t.Run("EarlyTermination", func(t *testing.T) {
		count := 0
		for _, err := range store.Values(ctx) {
			require.NoError(t, err)
			count++
			if count >= 2 {
				break
			}
		}

		require.Equal(t, 2, count)
	})

	t.Run("NilContext", func(t *testing.T) {
		count := 0
		for val, err := range store.Values(nil) {
			require.NoError(t, err)
			require.NotNil(t, val)
			count++
		}
		require.Equal(t, 3, count)
	})

	t.Run("EmptyStorage", func(t *testing.T) {
		emptyStore, err := memory.New(map[string]string{})
		require.NoError(t, err)

		count := 0
		for _, err := range emptyStore.Values(ctx) {
			require.NoError(t, err)
			count++
		}

		require.Equal(t, 0, count)
	})
}

func TestStorage_CheckConnection(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	require.NoError(t, err)

	require.NoError(t, store.CheckConnection(ctx))
}

func TestStorage_Concurrent(t *testing.T) {
	ctx := t.Context()

	store, err := memory.New(map[string]string{})
	require.NoError(t, err)

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
