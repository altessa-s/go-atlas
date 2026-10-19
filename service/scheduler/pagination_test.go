// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"bytes"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/keyset"
)

func TestPageRequest_ClampLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
		want  int64
	}{
		{"zero_defaults", 0, DefaultPageSize},
		{"negative_defaults", -1, DefaultPageSize},
		{"within_range", 50, 50},
		{"at_max", MaxPageSize, MaxPageSize},
		{"over_max_clamped", MaxPageSize + 1, MaxPageSize},
		{"one", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PageRequest{Limit: tt.limit}
			assert.Equal(t, tt.want, p.ClampLimit())
		})
	}
}

func newPagingScheduler(t *testing.T, opts ...Option) *Scheduler {
	t.Helper()
	return New(nil, append([]Option{WithEphemeralPageTokens()}, opts...)...)
}

func TestTaskCursor_RoundTrip(t *testing.T) {
	t.Parallel()

	s := newPagingScheduler(t)
	token, err := s.encodeTaskCursor("task-42", `status == 1`)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	cur, err := s.decodeTaskCursor(token, `status == 1`)
	require.NoError(t, err)
	assert.Equal(t, "task-42", cur.LastID)
}

func TestTaskCursor_Rejected(t *testing.T) {
	t.Parallel()

	s := newPagingScheduler(t)
	token, err := s.encodeTaskCursor("task-1", "status == 1")
	require.NoError(t, err)
	history, err := s.encodeHistoryCursor("task-1", 100, "h1", "status == 1")
	require.NoError(t, err)
	// The pre-keyset format: unsigned base64 JSON.
	legacy := base64.RawURLEncoding.EncodeToString([]byte(`{"id":"task-1","fh":""}`))

	tests := map[string]struct {
		token, filter string
		failure       keyset.Failure
	}{
		"not base64":      {token: "not-valid-base64!!!", failure: keyset.FailureMalformed},
		"legacy unsigned": {token: legacy, failure: keyset.FailureMalformed},
		"filter changed":  {token: token, filter: "status == 2", failure: keyset.FailureMalformed},
		"history token":   {token: history, filter: "status == 1", failure: keyset.FailureMalformed},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := s.decodeTaskCursor(tc.token, tc.filter)
			require.ErrorIs(t, err, ErrInvalidCursor)
			assert.Equal(t, tc.failure, keyset.Classify(err))
		})
	}
}

func TestHistoryCursor_RoundTrip(t *testing.T) {
	t.Parallel()

	s := newPagingScheduler(t)
	token, err := s.encodeHistoryCursor("task-a", 1700000000, "hist-99", `success == true`)
	require.NoError(t, err)

	cur, err := s.decodeHistoryCursor(token, "task-a", `success == true`)
	require.NoError(t, err)
	assert.Equal(t, int64(1700000000), cur.LastStartedAt)
	assert.Equal(t, "hist-99", cur.LastID)
}

func TestHistoryCursor_BoundToTaskAndFilter(t *testing.T) {
	t.Parallel()

	s := newPagingScheduler(t)
	token, err := s.encodeHistoryCursor("task-a", 100, "h1", "filter-a")
	require.NoError(t, err)

	_, err = s.decodeHistoryCursor(token, "task-a", "filter-b")
	require.ErrorIs(t, err, ErrInvalidCursor)
	_, err = s.decodeHistoryCursor(token, "task-b", "filter-a")
	require.ErrorIs(t, err, ErrInvalidCursor)
	_, err = s.decodeHistoryCursor("%%%invalid%%%", "task-a", "")
	require.ErrorIs(t, err, ErrInvalidCursor)
}

// TestCursor_SharedCodec checks that schedulers sharing a codec accept each
// other's cursors, and that schedulers with their own random keys do not.
func TestCursor_SharedCodec(t *testing.T) {
	t.Parallel()

	codec, err := keyset.New(bytes.Repeat([]byte{7}, keyset.MinKeyLength))
	require.NoError(t, err)
	a := newPagingScheduler(t, WithPageTokens(codec))
	b := newPagingScheduler(t, WithPageTokens(codec))
	token, err := a.encodeTaskCursor("task-1", "")
	require.NoError(t, err)
	cur, err := b.decodeTaskCursor(token, "")
	require.NoError(t, err)
	assert.Equal(t, "task-1", cur.LastID)

	_, err = newPagingScheduler(t).decodeTaskCursor(token, "")
	require.ErrorIs(t, err, ErrInvalidCursor, "an ephemeral scheduler signs with its own key")
}

func TestCursor_Expired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	codec, err := keyset.New(bytes.Repeat([]byte{7}, keyset.MinKeyLength), keyset.WithClock(clock), keyset.WithTTL(time.Hour))
	require.NoError(t, err)
	issuer := newPagingScheduler(t, WithPageTokens(codec))
	token, err := issuer.encodeTaskCursor("task-1", "")
	require.NoError(t, err)

	later, err := keyset.New(bytes.Repeat([]byte{7}, keyset.MinKeyLength),
		keyset.WithClock(func() time.Time { return now.Add(2 * time.Hour) }), keyset.WithTTL(time.Hour))
	require.NoError(t, err)
	_, err = newPagingScheduler(t, WithPageTokens(later)).decodeTaskCursor(token, "")
	require.ErrorIs(t, err, ErrInvalidCursor)
	require.ErrorIs(t, err, keyset.ErrExpiredToken)
	assert.Equal(t, keyset.FailureExpired, keyset.Classify(err))
}
