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

func TestDrivenInterceptor_Name(t *testing.T) {
	d := NoopDriver()
	inner := &mockDrivenInterceptor{driver: d}

	// NoopDriver doesn't implement Interceptor, so name should be "driven"
	t.Run("server", func(t *testing.T) {
		if name := ServerDrivenInterceptor(inner).Name(); name != "driven" {
			t.Fatalf("Name() = %q, want %q", name, "driven")
		}
	})
	t.Run("client", func(t *testing.T) {
		if name := ClientDrivenInterceptor(inner).Name(); name != "driven" {
			t.Fatalf("Name() = %q, want %q", name, "driven")
		}
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
	if len(got) != len(want) {
		t.Fatalf("Dependencies() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Dependencies()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
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
	md := &mockNamedDrivenInterceptor{driver: NoopDriver(), name: "metadata"}
	auth := &mockNamedDrivenInterceptor{driver: NoopDriver(), name: "auth", deps: []string{"metadata"}}
	idk := &mockNamedDrivenInterceptor{driver: NoopDriver(), name: "idempotency", deps: []string{"metadata", "auth"}}

	// Pass in reverse order to verify ordering works.
	result, err := OrderServerInterceptors(
		ServerDrivenInterceptor(idk),
		ServerDrivenInterceptor(auth),
		ServerDrivenInterceptor(md),
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"metadata", "auth", "idempotency"}
	if len(result) != len(want) {
		t.Fatalf("len = %d, want %d", len(result), len(want))
	}
	for i, ic := range result {
		if ic.Name() != want[i] {
			t.Fatalf("result[%d].Name() = %q, want %q", i, ic.Name(), want[i])
		}
	}
}
