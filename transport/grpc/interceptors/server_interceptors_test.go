// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"testing"
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
