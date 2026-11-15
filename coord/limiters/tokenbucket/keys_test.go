// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
)

// keyRecorder is a storage that allows every request and records the keys.
type keyRecorder struct {
	mu   sync.Mutex
	keys []string
}

func (r *keyRecorder) Allow(_ context.Context, key string, limit int64, _ time.Duration) (*storages.LimitInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys = append(r.keys, key)
	return &storages.LimitInfo{Remaining: limit}, nil
}

func (r *keyRecorder) Reset(context.Context, string) error { return nil }

func (r *keyRecorder) lastKey() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.keys[len(r.keys)-1]
}

func ipKey(t *testing.T, ip string, opts ...tokenbucket.Option) string {
	t.Helper()
	rec := &keyRecorder{}
	opts = append(opts, tokenbucket.WithExtractClientIPAddress(func(context.Context) string { return ip }))
	l, err := tokenbucket.New(validDefaults(), rec, opts...)
	require.NoError(t, err)
	_, err = l.Limit(t.Context())
	require.NoError(t, err)
	return rec.lastKey()
}

func TestStorageKey_IPv6AggregatedToPrefix(t *testing.T) {
	t.Parallel()
	a := ipKey(t, "2001:db8:1:2::1")
	b := ipKey(t, "2001:db8:1:2:ffff:ffff:ffff:ffff")
	other := ipKey(t, "2001:db8:1:3::1")
	require.Equal(t, a, b, "addresses in one /64 must share a bucket")
	require.NotEqual(t, a, other)

	wide := ipKey(t, "2001:db8:1:3::1", tokenbucket.WithIPv6KeyPrefixBits(48))
	require.Equal(t, ipKey(t, "2001:db8:1:2::1", tokenbucket.WithIPv6KeyPrefixBits(48)), wide)

	require.NotEqual(t, ipKey(t, "2001:db8::1", tokenbucket.WithIPv6KeyPrefixBits(128)),
		ipKey(t, "2001:db8::2", tokenbucket.WithIPv6KeyPrefixBits(128)))
}

func TestStorageKey_IPv4Unchanged(t *testing.T) {
	t.Parallel()
	require.Equal(t, "ip-1_2_3_4", ipKey(t, "1.2.3.4"))
	require.Equal(t, "ip-1_2_3_4", ipKey(t, "::ffff:1.2.3.4"), "IPv4-mapped addresses key as IPv4")
	require.NotEqual(t, ipKey(t, "1.2.3.4"), ipKey(t, "1.2.3.5"))
}

func TestNew_RejectsInvalidIPv6KeyPrefixBits(t *testing.T) {
	t.Parallel()
	for _, bits := range []int{-1, 129} {
		_, err := tokenbucket.New(validDefaults(), &keyRecorder{}, tokenbucket.WithIPv6KeyPrefixBits(bits))
		require.Error(t, err, bits)
	}
}

func TestStorageKey_TokenIsHashed(t *testing.T) {
	t.Parallel()
	const token = "secret-bearer-token"
	rec := &keyRecorder{}
	l, err := tokenbucket.New(validDefaults(), rec,
		tokenbucket.WithExtractToken(func(context.Context) string { return token }),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(context.Context, string) (*tokenbucket.RateLimitSettings, error) {
				return &tokenbucket.RateLimitSettings{Limit: 5, Period: time.Minute}, nil
			})),
	)
	require.NoError(t, err)
	_, err = l.Limit(t.Context())
	require.NoError(t, err)

	key := rec.lastKey()
	require.NotContains(t, key, token, "raw bearer tokens must not reach storage")
	require.True(t, strings.HasPrefix(key, "token-"), key)
	require.Len(t, key, len("token-")+64, "SHA-256 hex digest")
}
