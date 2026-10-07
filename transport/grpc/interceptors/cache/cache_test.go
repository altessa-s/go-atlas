// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/storages"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// recordingCacher is an always-missing [Cacher] that records Get calls and
// the TTL of every Save.
type recordingCacher struct {
	mu   sync.Mutex
	gets int
	ttls []time.Duration
}

func (c *recordingCacher) Save(_ context.Context, _ string, _ any, ttl ...time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttls = append(c.ttls, ttl...)
	return nil
}

func (c *recordingCacher) Get(context.Context, string, any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	return storages.ErrMissing
}

func (c *recordingCacher) Exists(context.Context, string) (bool, error) { return false, nil }
func (c *recordingCacher) Delete(context.Context, string) error         { return nil }
func (c *recordingCacher) DeleteMany(context.Context, ...string) error  { return nil }

func (c *recordingCacher) snapshot() (int, []time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gets, append([]time.Duration(nil), c.ttls...)
}

const testMethod = "/test.Service/Get"

// callUnary runs one unary call for testMethod through a cache interceptor.
func callUnary(t *testing.T, cacher Cacher, req any, opts ...Option) any {
	t.Helper()
	opts = append([]Option{WithMethod(testMethod, &wrapperspb.StringValue{}), WithCacheHeaders(false)}, opts...)
	unary := ServerInterceptor(cacher, opts...).ServerUnaryInterceptor()

	want := wrapperspb.String("response")
	resp, err := unary(t.Context(), req, &grpc.UnaryServerInfo{FullMethod: testMethod},
		func(context.Context, any) (any, error) { return want, nil })
	require.NoError(t, err)
	require.Same(t, want, resp)
	return resp
}

func TestServerInterceptor_KeyGenerationFailureSkipsCache(t *testing.T) {
	t.Parallel()
	errKey := errors.New("key generation failed")
	cacher := &recordingCacher{}

	callUnary(t, cacher, wrapperspb.String("req"), WithKeyGenerator(
		func(context.Context, string, any) (string, error) { return "", errKey },
	))

	gets, ttls := cacher.snapshot()
	require.Zero(t, gets)
	require.Empty(t, ttls)
}

func TestServerInterceptor_DefaultTTLOptionsReachStorage(t *testing.T) {
	t.Parallel()
	const ttl = 10 * time.Second

	for name, opt := range map[string]Option{
		"WithDefaultTTL": WithDefaultTTL(ttl),
		"WithCacheTTL":   WithCacheTTL(ttl),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cacher := &recordingCacher{}
			callUnary(t, cacher, wrapperspb.String("req"), opt)

			_, ttls := cacher.snapshot()
			require.Equal(t, []time.Duration{ttl}, ttls)
		})
	}
}

func TestServerInterceptor_ExplicitDecisionKeepsItsTTL(t *testing.T) {
	t.Parallel()
	const decisionTTL = 42 * time.Second
	cacher := &recordingCacher{}

	callUnary(t, cacher, wrapperspb.String("req"),
		WithDefaultTTL(10*time.Second),
		WithCacheDecision(DefaultSuccessOnlyDecision(decisionTTL)),
	)

	_, ttls := cacher.snapshot()
	require.Equal(t, []time.Duration{decisionTTL}, ttls)
}

func TestServerInterceptor_DecisionReceivesRequest(t *testing.T) {
	t.Parallel()

	decision := func(_ context.Context, _ string, req, _ any, err error) Decision {
		r, ok := req.(*wrapperspb.StringValue)
		if !ok || err != nil || r.GetValue() == "no-cache" {
			return Decision{}
		}
		return Decision{ShouldCache: true, TTL: time.Minute}
	}

	t.Run("declined by request field", func(t *testing.T) {
		t.Parallel()
		cacher := &recordingCacher{}
		callUnary(t, cacher, wrapperspb.String("no-cache"), WithCacheDecision(decision))
		_, ttls := cacher.snapshot()
		require.Empty(t, ttls)
	})

	t.Run("same request instance", func(t *testing.T) {
		t.Parallel()
		req := wrapperspb.String("cache-me")
		var got any
		cacher := &recordingCacher{}
		callUnary(t, cacher, req, WithCacheDecision(func(ctx context.Context, method string, r, resp any, err error) Decision {
			got = r
			return decision(ctx, method, r, resp, err)
		}))
		require.Same(t, req, got)
		_, ttls := cacher.snapshot()
		require.Equal(t, []time.Duration{time.Minute}, ttls)
	})
}
