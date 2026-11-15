// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	mongohelpers "github.com/altessa-s/go-atlas/store/mongo/internal/testhelpers"
	cursredis "github.com/altessa-s/go-atlas/store/mongo/storages/redis"
)

func TestStorage_KeyPrefixAndTTL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		opts       []cursredis.Option
		wantPrefix string
		wantTTL    time.Duration
	}{
		{
			name:       "defaults",
			wantPrefix: cursredis.DefaultKeyPrefix,
			wantTTL:    cursredis.DefaultTTL,
		},
		{
			name: "pointer prefix and explicit ttl",
			opts: []cursredis.Option{
				cursredis.WithKeyPrefix(new(" cur: ")), cursredis.WithTTL(time.Minute),
			},
			wantPrefix: "cur:",
			wantTTL:    time.Minute,
		},
		{
			name: "blank and non-positive values are ignored",
			opts: []cursredis.Option{
				cursredis.WithKeyPrefix(" "), cursredis.WithKeyPrefix(new("")), cursredis.WithKeyPrefix((*string)(nil)),
				cursredis.WithTTL(0), cursredis.WithTTL(-time.Second),
			},
			wantPrefix: cursredis.DefaultKeyPrefix,
			wantTTL:    cursredis.DefaultTTL,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)
			s := cursredis.New(client, tc.opts...)
			require.NoError(t, s.Store(t.Context(), "k", mongohelpers.SampleCursorMetadata()))

			keys := mr.Keys()
			require.Equal(t, []string{tc.wantPrefix + "k"}, keys)
			require.Equal(t, tc.wantTTL, mr.TTL(keys[0]))
		})
	}
}
