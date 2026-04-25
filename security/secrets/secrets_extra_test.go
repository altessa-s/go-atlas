// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
)

type mockProvider struct {
	values map[string]*secrets.Value[string]
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) List(_ context.Context) ([]*secrets.Value[string], error) {
	result := make([]*secrets.Value[string], 0, len(m.values))
	for _, v := range m.values {
		result = append(result, v)
	}
	return result, nil
}
func (m *mockProvider) Values(_ context.Context) iter.Seq2[*secrets.Value[string], error] {
	return func(yield func(*secrets.Value[string], error) bool) {
		for _, v := range m.values {
			if !yield(v, nil) {
				return
			}
		}
	}
}
func (m *mockProvider) Value(_ context.Context, key string) (*secrets.Value[string], error) {
	if v, ok := m.values[key]; ok {
		return v, nil
	}
	return nil, secrets.ErrNotFound
}
func (m *mockProvider) Delete(_ context.Context, key string) error {
	delete(m.values, key)
	return nil
}
func (m *mockProvider) Save(_ context.Context, key string, value string) error {
	m.values[key] = secrets.NewValue(key, value, []byte(value), "v1")
	return nil
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		values: make(map[string]*secrets.Value[string]),
	}
}

func TestManager_New(t *testing.T) {
	mgr, err := secrets.New[string](newMockProvider())
	require.NoError(t, err)
	require.NotNil(t, mgr)
}

func TestManager_Value_NotFound(t *testing.T) {
	mgr, err := secrets.New[string](newMockProvider())
	require.NoError(t, err)

	_, err = mgr.Value(t.Context(), "nonexistent", true)
	require.Error(t, err)
}

func TestManager_Save(t *testing.T) {
	mgr, err := secrets.New[string](newMockProvider())
	require.NoError(t, err)

	require.NoError(t, mgr.Save(t.Context(), "newkey", "newval"))
}

func TestManager_Delete_NonExistent(t *testing.T) {
	mgr, err := secrets.New[string](newMockProvider())
	require.NoError(t, err)

	// Delete on empty provider - should not error (idempotent via our mock)
	require.NoError(t, mgr.Delete(t.Context(), "nope"))
}

func TestManager_CacheSize_Empty(t *testing.T) {
	mgr, err := secrets.New[string](newMockProvider())
	require.NoError(t, err)

	require.Equal(t, 0, mgr.CacheSize())
}

func TestManager_LastUpdateTime(t *testing.T) {
	mgr, err := secrets.New[string](newMockProvider())
	require.NoError(t, err)

	ut := mgr.LastUpdateTime()
	require.True(t, ut.IsZero(), "LastUpdateTime should be zero initially")
}
