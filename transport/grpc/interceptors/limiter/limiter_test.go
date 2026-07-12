// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/metadata"
)

func fakeLimiter() limiters.Limiter {
	return limiters.Func(func(context.Context) (*limiters.LimitInfo, error) {
		return &limiters.LimitInfo{Limit: 100, Remaining: 42, Reset: 12345}, nil
	})
}

func rateLimitMD(t *testing.T, opts ...Option) metadata.MD {
	t.Helper()
	i := ServerInterceptor(fakeLimiter(), opts...).(*interceptor)
	md, err := i.rateLimit(t.Context(), "/svc/Method")
	require.NoError(t, err)
	return md
}

// TestRateLimitHeaders_OptIn pins the secure default: rate-limit headers reveal
// capacity, so they are withheld unless WithExposeHeaders is set.
func TestRateLimitHeaders_OptIn(t *testing.T) {
	t.Parallel()

	off := rateLimitMD(t)
	require.Empty(t, off.Get(RateLimitMetadataKey), "limit header must be off by default")
	require.Empty(t, off.Get(RateLimitRemaining))
	require.Empty(t, off.Get(RateLimitReset))

	on := rateLimitMD(t, WithExposeHeaders())
	require.Equal(t, []string{"100"}, on.Get(RateLimitMetadataKey))
	require.Equal(t, []string{"42"}, on.Get(RateLimitRemaining))
	require.Equal(t, []string{"12345"}, on.Get(RateLimitReset))
}

// TestDependencies_IncludesAuth verifies the limiter is ordered after auth so a
// present auth interceptor runs first.
func TestDependencies_IncludesAuth(t *testing.T) {
	t.Parallel()

	i := ServerInterceptor(fakeLimiter()).(*interceptor)
	require.Contains(t, i.Dependencies(), auth.Name())
}
