// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
)

// singleStorage implements only [storages.Storage] and counts its calls.
type singleStorage struct{ calls int }

func (s *singleStorage) Allow(context.Context, string, int64, time.Duration) (*storages.LimitInfo, error) {
	s.calls++
	return &storages.LimitInfo{}, nil
}

func (s *singleStorage) Reset(context.Context, string) error { return nil }

// multiStorage records the limits of its AllowAll call.
type multiStorage struct {
	singleStorage
	got []storages.Limit
}

func (m *multiStorage) AllowAll(_ context.Context, limits []storages.Limit) ([]storages.LimitInfo, error) {
	m.got = limits
	return make([]storages.LimitInfo, len(limits)), nil
}

func limitsOf(keys ...string) []storages.Limit {
	limits := make([]storages.Limit, len(keys))
	for i, k := range keys {
		limits[i] = storages.Limit{Key: k, Limit: 1, Period: time.Second}
	}
	return limits
}

func TestAllowAll_Unsupported(t *testing.T) {
	t.Parallel()
	s := &singleStorage{}
	infos, err := storages.AllowAll(t.Context(), s, limitsOf("a", "b")...)
	require.ErrorIs(t, err, storages.ErrMultiKeyUnsupported)
	require.Nil(t, infos)
	require.Zero(t, s.calls, "AllowAll must not fall back to sequential Allow calls")

	_, err = storages.AllowAll(t.Context(), nil, limitsOf("a")...)
	require.ErrorIs(t, err, storages.ErrMultiKeyUnsupported)
}

func TestAllowAll_UsesMultiStorage(t *testing.T) {
	t.Parallel()
	m := &multiStorage{}
	limits := limitsOf("a", "b")
	infos, err := storages.AllowAll(t.Context(), m, limits...)
	require.NoError(t, err)
	require.Len(t, infos, 2)
	require.Equal(t, limits, m.got)
	require.Zero(t, m.calls)
}

func TestKeyError(t *testing.T) {
	t.Parallel()
	err := error(&storages.KeyError{Index: 2, Key: "203.0.113.7", Err: storages.ErrLimitExceeded})
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	require.Equal(t, "limit 2: rate limit exceeded", err.Error())
	require.NotContains(t, err.Error(), "203.0.113.7", "the key may carry client identity")

	wrapped := errors.Join(errors.New("context"), err)
	keyErr, ok := errors.AsType[*storages.KeyError](wrapped)
	require.True(t, ok)
	require.Equal(t, "203.0.113.7", keyErr.Key)
}

func TestValidateLimits(t *testing.T) {
	t.Parallel()
	many := make([]string, 12)
	for i := range many {
		many[i] = "k" + strconv.Itoa(i)
	}
	tests := []struct {
		name    string
		keys    []string
		wantDup int // index of the duplicate, -1 for none
	}{
		{name: "single", keys: []string{"a"}, wantDup: -1},
		{name: "distinct", keys: []string{"a", "b", "c"}, wantDup: -1},
		{name: "duplicate", keys: []string{"a", "b", "a"}, wantDup: 2},
		{name: "adjacent duplicate", keys: []string{"a", "a", "b"}, wantDup: 1},
		{name: "many distinct", keys: many, wantDup: -1},
		{name: "many with duplicate", keys: append(many[:len(many):len(many)], "k5"), wantDup: len(many)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := storages.ValidateLimits(limitsOf(tc.keys...))
			if tc.wantDup < 0 {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, storages.ErrDuplicateKey)
			keyErr, ok := errors.AsType[*storages.KeyError](err)
			require.True(t, ok)
			require.Equal(t, tc.wantDup, keyErr.Index)
			require.Equal(t, tc.keys[tc.wantDup], keyErr.Key)
		})
	}
}

func TestValidateLimits_Panics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		limits []storages.Limit
	}{
		{name: "empty", limits: nil},
		{name: "empty key", limits: []storages.Limit{{Limit: 1, Period: time.Second}}},
		{name: "zero limit", limits: []storages.Limit{{Key: "k", Period: time.Second}}},
		{name: "negative period", limits: []storages.Limit{{Key: "k", Limit: 1, Period: -time.Second}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Panics(t, func() { _ = storages.ValidateLimits(tc.limits) })
		})
	}
}
