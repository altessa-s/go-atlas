// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
)

func TestServerConditionalInterceptor(t *testing.T) {
	real := &NoOpInterceptor{}

	t.Run("true_returns_real", func(t *testing.T) {
		i := ServerConditionalInterceptor(true, real)
		require.Equal(t, real, i)
	})

	t.Run("false_returns_noop", func(t *testing.T) {
		i := ServerConditionalInterceptor(false, real)
		require.Equal(t, "noop", i.Name())
	})
}

func TestServerConditionalInterceptorFunc(t *testing.T) {
	t.Run("true_calls_fn", func(t *testing.T) {
		called := false
		i := ServerConditionalInterceptorFunc(true, func() ServerInterceptor {
			called = true
			return &NoOpInterceptor{}
		})
		require.True(t, called, "fn should be called")
		_ = i
	})

	t.Run("false_skips_fn", func(t *testing.T) {
		i := ServerConditionalInterceptorFunc(false, func() ServerInterceptor {
			require.Fail(t, "fn should not be called")
			return nil
		})
		require.Equal(t, "noop", i.Name())
	})
}

func TestServerMatchInterceptor(t *testing.T) {
	real := &NoOpInterceptor{}

	t.Run("match_returns_real", func(t *testing.T) {
		i := ServerMatchInterceptor(MatchFunc(func() bool { return true }), real)
		require.Equal(t, real, i)
	})

	t.Run("no_match_returns_noop", func(t *testing.T) {
		i := ServerMatchInterceptor(MatchFunc(func() bool { return false }), real)
		_, ok := i.(*NoOpInterceptor)
		require.True(t, ok, "expected noop")
	})
}

func TestServerMatchInterceptorFunc(t *testing.T) {
	t.Run("match_calls_fn", func(t *testing.T) {
		i := ServerMatchInterceptorFunc(func() bool { return true }, func() ServerInterceptor {
			return &NoOpInterceptor{}
		})
		_ = i
	})

	t.Run("no_match_skips_fn", func(t *testing.T) {
		i := ServerMatchInterceptorFunc(func() bool { return false }, func() ServerInterceptor {
			require.Fail(t, "should not be called")
			return nil
		})
		_ = i
	})
}

func TestDrivenInterceptor_Name(t *testing.T) {
	d := NoopDriver()
	inner := &mockDrivenInterceptor{driver: d}

	// NoopDriver doesn't implement Interceptor, so name should be "driven"
	t.Run("server", func(t *testing.T) {
		name := ServerDrivenInterceptor(inner).Name()
		require.Equal(t, "driven", name)
	})
	t.Run("client", func(t *testing.T) {
		name := ClientDrivenInterceptor(inner).Name()
		require.Equal(t, "driven", name)
	})
}

// mockNamedDrivenInterceptor implements DrivenInterceptor + Interceptor + Dependencies.
type mockNamedDrivenInterceptor struct {
	driver driver.Driver
	name   string
	deps   []string
}

func (m *mockNamedDrivenInterceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	return m.driver, ctx
}

func (m *mockNamedDrivenInterceptor) Name() string { return m.name }

func (m *mockNamedDrivenInterceptor) Dependencies() []string { return m.deps }

// dependencyDeclarer is the common assertion target for both
// DrivenServerInterceptor and DrivenClientInterceptor.
type dependencyDeclarer interface {
	Dependencies() []string
}

func TestDrivenInterceptor_Dependencies(t *testing.T) {
	tests := []struct {
		name     string
		inner    driver.DrivenInterceptor
		wantDeps []string
	}{
		{
			name: "forwards dependencies from underlying interceptor",
			inner: &mockNamedDrivenInterceptor{
				driver: NoopDriver(),
				name:   "test",
				deps:   []string{"metadata", "auth"},
			},
			wantDeps: []string{"metadata", "auth"},
		},
		{
			name:     "returns nil when underlying has no Dependencies method",
			inner:    &mockDrivenInterceptor{driver: NoopDriver()},
			wantDeps: nil,
		},
	}

	for _, tt := range tests {
		t.Run("server/"+tt.name, func(t *testing.T) {
			si := ServerDrivenInterceptor(tt.inner)
			assertDependencies(t, si.(dependencyDeclarer), tt.wantDeps)
		})

		t.Run("client/"+tt.name, func(t *testing.T) {
			ci := ClientDrivenInterceptor(tt.inner)
			assertDependencies(t, ci.(dependencyDeclarer), tt.wantDeps)
		})
	}
}

func assertDependencies(t *testing.T, d dependencyDeclarer, want []string) {
	t.Helper()
	got := d.Dependencies()
	require.Equal(t, len(want), len(got))
	for i := range want {
		require.Equal(t, want[i], got[i])
	}
}

func TestOrderServerInterceptors(t *testing.T) {
	a := &NoOpInterceptor{}
	result, err := OrderServerInterceptors(a, a)
	require.NoError(t, err)
	// Duplicates should be removed
	require.Len(t, result, 1)
}

func TestOrderServerInterceptors_DrivenDependencies(t *testing.T) {
	// Simulate: auth depends on metadata, idempotency depends on metadata+auth.
	// Expected order: metadata, auth, idempotency.
	md := &mockNamedDrivenInterceptor{driver: NoopDriver(), name: "metadata"}
	auth := &mockNamedDrivenInterceptor{driver: NoopDriver(), name: "auth", deps: []string{"metadata"}}
	idk := &mockNamedDrivenInterceptor{driver: NoopDriver(), name: "idempotency", deps: []string{"metadata", "auth"}}

	// Pass in reverse order to verify ordering works.
	result, err := OrderServerInterceptors(
		ServerDrivenInterceptor(idk),
		ServerDrivenInterceptor(auth),
		ServerDrivenInterceptor(md),
	)
	require.NoError(t, err)

	want := []string{"metadata", "auth", "idempotency"}
	require.Equal(t, len(want), len(result))
	for i, ic := range result {
		require.Equal(t, want[i], ic.Name())
	}
}
