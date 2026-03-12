// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"testing"

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
		if i != real {
			t.Fatal("expected real interceptor")
		}
	})

	t.Run("false_returns_noop", func(t *testing.T) {
		i := ClientConditionalInterceptor(false, real)
		if i.Name() != "noop" {
			t.Fatalf("expected noop, got %q", i.Name())
		}
	})
}

func TestClientConditionalInterceptorFunc(t *testing.T) {
	t.Run("true_calls_fn", func(t *testing.T) {
		called := false
		ClientConditionalInterceptorFunc(true, func() ClientInterceptor {
			called = true
			return &NoOpClientInterceptor{}
		})
		if !called {
			t.Fatal("fn should be called")
		}
	})

	t.Run("false_skips_fn", func(t *testing.T) {
		i := ClientConditionalInterceptorFunc(false, func() ClientInterceptor {
			t.Fatal("fn should not be called")
			return nil
		})
		if i.Name() != "noop" {
			t.Fatal("expected noop")
		}
	})
}

func TestClientMatchInterceptor(t *testing.T) {
	real := &NoOpClientInterceptor{}

	t.Run("match", func(t *testing.T) {
		i := ClientMatchInterceptor(MatchFunc(func() bool { return true }), real)
		if i != real {
			t.Fatal("expected real")
		}
	})

	t.Run("no_match", func(t *testing.T) {
		i := ClientMatchInterceptor(MatchFunc(func() bool { return false }), real)
		if _, ok := i.(*NoOpClientInterceptor); !ok {
			t.Fatal("expected noop")
		}
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
			t.Fatal("should not be called")
			return nil
		})
	})
}

func TestClientDrivenInterceptor_Name(t *testing.T) {
	d := NoopDriver()
	i := ClientDrivenInterceptor(&mockDrivenInterceptor{driver: d})
	if i.Name() != "driven" {
		t.Fatalf("Name() = %q", i.Name())
	}
}

func TestOrderClientInterceptors(t *testing.T) {
	a := &NoOpClientInterceptor{}
	result, err := OrderClientInterceptors(a, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
}

func TestClientDrivenInterceptor_Dependencies(t *testing.T) {
	t.Run("forwards dependencies from underlying interceptor", func(t *testing.T) {
		inner := &mockNamedDrivenInterceptor{
			driver: NoopDriver(),
			name:   "logger",
			deps:   []string{"metadata", "requestid", "realip", "tracing"},
		}
		ci := ClientDrivenInterceptor(inner)

		deps := ci.(*DrivenClientInterceptor).Dependencies()
		if len(deps) != 4 || deps[0] != "metadata" {
			t.Fatalf("Dependencies() = %v, want [metadata requestid realip tracing]", deps)
		}
	})

	t.Run("returns nil when underlying has no Dependencies method", func(t *testing.T) {
		inner := &mockDrivenInterceptor{driver: NoopDriver()}
		ci := ClientDrivenInterceptor(inner)

		deps := ci.(*DrivenClientInterceptor).Dependencies()
		if deps != nil {
			t.Fatalf("Dependencies() = %v, want nil", deps)
		}
	})
}

func TestDrivenInterceptorFunc(t *testing.T) {
	d := NoopDriver()
	f := DrivenInterceptorFunc(func(ctx context.Context) (driver.Driver, context.Context) {
		return d, ctx
	})
	got, _ := f.DrivenInterceptor(t.Context())
	if got != d {
		t.Fatal("expected same driver")
	}
}
