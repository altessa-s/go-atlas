// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/data/keyset"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/factory"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func pagingConfig(key string) *schedulerconfig.Config {
	cfg := schedulerconfig.Default()
	cfg.Paging = &schedulerconfig.Paging{SigningKey: redacted.RedactedString(key)}
	return &cfg
}

// firstCursor registers three tasks on s and returns the cursor of a page of
// two.
func firstCursor(t *testing.T, s *scheduler.Scheduler) string {
	t.Helper()
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
			ID: id, Schedule: "@every 1h", Func: func(context.Context) error { return nil },
		}))
	}
	page, err := s.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2}, "")
	require.NoError(t, err)
	require.NotNil(t, page.NextCursor)
	return *page.NextCursor
}

func TestPageTokensFromConfig(t *testing.T) {
	t.Parallel()

	key := strings.Repeat("k", keyset.MinKeyLength)
	a, err := factory.New(pagingConfig(key)).Build()
	require.NoError(t, err)
	cursor := firstCursor(t, a)

	b, err := factory.New(pagingConfig(key)).Build()
	require.NoError(t, err)
	_, err = b.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2, Cursor: cursor}, "")
	require.NoError(t, err, "a scheduler built from the same paging config accepts the cursor")

	plain := schedulerconfig.Default()
	none, err := factory.New(&plain).Build()
	require.NoError(t, err, "a scheduler without paging still builds")
	_, err = none.TasksPaginated(t.Context(), scheduler.PageRequest{}, "")
	require.ErrorIs(t, err, scheduler.ErrPageTokensNotConfigured)

	other, err := keyset.New(bytes.Repeat([]byte{3}, keyset.MinKeyLength))
	require.NoError(t, err)
	injected, err := factory.New(pagingConfig(key)).UsePageTokens(other).Build()
	require.NoError(t, err)
	_, err = injected.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2, Cursor: cursor}, "")
	require.ErrorIs(t, err, scheduler.ErrInvalidCursor, "an injected codec takes precedence over the config")

	_, err = factory.New(pagingConfig("short")).Build()
	require.ErrorIs(t, err, keyset.ErrWeakKey)
}
