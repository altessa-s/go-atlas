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

// mockDrivenInterceptor is a test helper for DrivenInterceptor.
type mockDrivenInterceptor struct {
	driver driver.Driver
}

func (m *mockDrivenInterceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	return m.driver, ctx
}

func TestClientConditionalInterceptor(t *testing.T) {
	real := &NoOpClientInterceptor{}

	t.Run("true_returns_real", func(t *testing.T) {
		i := ClientConditionalInterceptor(true, real)
		require.Equal(t, real, i)
	})

	t.Run("false_returns_noop", func(t *testing.T) {
		i := ClientConditionalInterceptor(false, real)
		require.Equal(t, "noop", i.Name())
	})
}

func TestClientConditionalInterceptorFunc(t *testing.T) {
	t.Run("true_calls_fn", func(t *testing.T) {
		called := false
		ClientConditionalInterceptorFunc(true, func() ClientInterceptor {
			called = true
			return &NoOpClientInterceptor{}
		})
		require.True(t, called, "fn should be called")
	})

	t.Run("false_skips_fn", func(t *testing.T) {
		i := ClientConditionalInterceptorFunc(false, func() ClientInterceptor {
			require.Fail(t, "fn should not be called")
			return nil
		})
		require.Equal(t, "noop", i.Name())
	})
}

func TestClientMatchInterceptor(t *testing.T) {
	real := &NoOpClientInterceptor{}

	t.Run("match", func(t *testing.T) {
		i := ClientMatchInterceptor(MatchFunc(func() bool { return true }), real)
		require.Equal(t, real, i)
	})

	t.Run("no_match", func(t *testing.T) {
		i := ClientMatchInterceptor(MatchFunc(func() bool { return false }), real)
		_, ok := i.(*NoOpClientInterceptor)
		require.True(t, ok, "expected noop")
	})
}

func TestClientMatchInterceptorFunc(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		ClientMatchInterceptorFunc(func() bool { return true }, func() ClientInterceptor {
			return &NoOpClientInterceptor{}
		})
	})

	t.Run("no_match", func(t *testing.T) {
		ClientMatchInterceptorFunc(func() bool { return false }, func() ClientInterceptor {
			require.Fail(t, "should not be called")
			return nil
		})
	})
}

// TestDrivenInterceptor_Name in server_interceptors_test.go covers
// both DrivenServerInterceptor and DrivenClientInterceptor name forwarding.

func TestOrderClientInterceptors(t *testing.T) {
	a := &NoOpClientInterceptor{}
	result, err := OrderClientInterceptors(a, a)
	require.NoError(t, err)
	require.Len(t, result, 1)
}

// TestDrivenInterceptor_Dependencies in server_interceptors_test.go covers
// both DrivenServerInterceptor and DrivenClientInterceptor dependency forwarding.

func TestDrivenInterceptorFunc(t *testing.T) {
	d := NoopDriver()
	f := DrivenInterceptorFunc(func(ctx context.Context) (driver.Driver, context.Context) {
		return d, ctx
	})
	got, _ := f.DrivenInterceptor(t.Context())
	require.Equal(t, d, got)
}
