// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
)

func TestServerConditionalInterceptor(t *testing.T) {
	real := &NoOpInterceptor{}

	t.Run("true_returns_real", func(t *testing.T) {
		i := ServerConditionalInterceptor(true, real)
		if i != real {
			t.Fatal("expected real interceptor")
		}
	})

	t.Run("false_returns_noop", func(t *testing.T) {
		i := ServerConditionalInterceptor(false, real)
		if i.Name() != "noop" {
			t.Fatalf("expected noop, got %q", i.Name())
		}
	})
}

func TestServerConditionalInterceptorFunc(t *testing.T) {
	t.Run("true_calls_fn", func(t *testing.T) {
		called := false
		i := ServerConditionalInterceptorFunc(true, func() ServerInterceptor {
			called = true
			return &NoOpInterceptor{}
		})
		if !called {
			t.Fatal("fn should be called")
		}
		_ = i
	})

	t.Run("false_skips_fn", func(t *testing.T) {
		i := ServerConditionalInterceptorFunc(false, func() ServerInterceptor {
			t.Fatal("fn should not be called")
			return nil
		})
		if i.Name() != "noop" {
			t.Fatal("expected noop")
		}
	})
}

func TestServerMatchInterceptor(t *testing.T) {
	real := &NoOpInterceptor{}

	t.Run("match_returns_real", func(t *testing.T) {
		i := ServerMatchInterceptor(MatchFunc(func() bool { return true }), real)
		if i != real {
			t.Fatal("expected real interceptor")
		}
	})

	t.Run("no_match_returns_noop", func(t *testing.T) {
		i := ServerMatchInterceptor(MatchFunc(func() bool { return false }), real)
		if _, ok := i.(*NoOpInterceptor); !ok {
			t.Fatal("expected noop")
		}
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
			t.Fatal("should not be called")
			return nil
		})
		_ = i
	})
}

func TestServerDrivenInterceptor_Name(t *testing.T) {
	d := NoopDriver()
	// NoopDriver doesn't implement Interceptor, so name should be "driven"
	i := ServerDrivenInterceptor(&mockDrivenInterceptor{driver: d})
	if i.Name() != "driven" {
		t.Fatalf("Name() = %q", i.Name())
	}
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

func TestServerDrivenInterceptor_Dependencies(t *testing.T) {
	t.Run("forwards dependencies from underlying interceptor", func(t *testing.T) {
		inner := &mockNamedDrivenInterceptor{
			driver: NoopDriver(),
			name:   "test",
			deps:   []string{"metadata", "auth"},
		}
		si := ServerDrivenInterceptor(inner)

		deps := si.(*DrivenServerInterceptor).Dependencies()
		if len(deps) != 2 || deps[0] != "metadata" || deps[1] != "auth" {
			t.Fatalf("Dependencies() = %v, want [metadata auth]", deps)
		}
	})

	t.Run("returns nil when underlying has no Dependencies method", func(t *testing.T) {
		inner := &mockDrivenInterceptor{driver: NoopDriver()}
		si := ServerDrivenInterceptor(inner)

		deps := si.(*DrivenServerInterceptor).Dependencies()
		if deps != nil {
			t.Fatalf("Dependencies() = %v, want nil", deps)
		}
	})
}

func TestOrderServerInterceptors(t *testing.T) {
	a := &NoOpInterceptor{}
	result, err := OrderServerInterceptors(a, a)
	if err != nil {
		t.Fatal(err)
	}
	// Duplicates should be removed
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
}

func TestOrderServerInterceptors_DrivenDependencies(t *testing.T) {
	// Simulate: auth depends on metadata, idempotency depends on metadata+auth.
	// Expected order: metadata, auth, idempotency.
	mdInner := &mockNamedDrivenInterceptor{
		driver: NoopDriver(),
		name:   "metadata",
		deps:   nil,
	}
	authInner := &mockNamedDrivenInterceptor{
		driver: NoopDriver(),
		name:   "auth",
		deps:   []string{"metadata"},
	}
	idkInner := &mockNamedDrivenInterceptor{
		driver: NoopDriver(),
		name:   "idempotency",
		deps:   []string{"metadata", "auth"},
	}

	// Pass in reverse order to verify ordering works.
	result, err := OrderServerInterceptors(
		ServerDrivenInterceptor(idkInner),
		ServerDrivenInterceptor(authInner),
		ServerDrivenInterceptor(mdInner),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}

	names := make([]string, len(result))
	for i, ic := range result {
		names[i] = ic.Name()
	}

	// metadata must come before auth, auth must come before idempotency
	if names[0] != "metadata" || names[1] != "auth" || names[2] != "idempotency" {
		t.Fatalf("order = %v, want [metadata auth idempotency]", names)
	}
}
