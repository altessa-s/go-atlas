// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/store/mongo"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/store/mongo"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func requireRoundTrip(t *testing.T, s mongo.Storage) {
	t.Helper()
	meta := &mongo.CursorMetadata{CursorId: "c1", Sort: "name"}
	require.NoError(t, s.Store(t.Context(), "key-1", meta))
	got, err := s.Load(t.Context(), "key-1")
	require.NoError(t, err)
	require.Equal(t, "c1", got.CursorId)
	require.Equal(t, "name", got.Sort)
}

func TestBuild_MemoryStorage(t *testing.T) {
	t.Parallel()
	reg := &testhelpers.MockTaskRegistrar{}
	s, err := factory.New(&storageconfig.BackendConfig{
		Type:   storageconfig.BackendTypeMemory,
		Memory: &storageconfig.MemoryConfig{CleanupSchedule: "@every 1m"},
	}).UseLogger(slog.New(slog.DiscardHandler)).UseScheduler(reg).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err)
	requireRoundTrip(t, s)

	tasks := reg.Tasks()
	require.Len(t, tasks, 1, "the memory cleanup task must be registered")
	require.Equal(t, "@every 1m", tasks[0].Schedule)
}

func TestBuild_ConfigErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *storageconfig.BackendConfig
		wantErr error
	}{
		{name: "nil config", cfg: nil, wantErr: corefactory.ErrConfigRequired},
		{name: "unsupported", cfg: &storageconfig.BackendConfig{Type: "etcd"}, wantErr: corefactory.ErrUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := factory.New(tc.cfg).Build(t.Context())
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}
