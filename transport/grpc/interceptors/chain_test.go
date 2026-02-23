// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"log/slog"
	"testing"
)

func TestNewChain_Empty(t *testing.T) {
	c := NewChain()
	if c.Len() != 0 {
		t.Fatalf("Len() = %d", c.Len())
	}
}

func TestNewChain_WithInterceptors(t *testing.T) {
	c := NewChain(&NoOpInterceptor{}, &NoOpClientInterceptor{})
	if c.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", c.Len())
	}
}

func TestNewChain_IgnoresInvalidTypes(t *testing.T) {
	c := NewChain("string", 42, &NoOpInterceptor{})
	if c.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", c.Len())
	}
}

func TestChain_ServerOptions(t *testing.T) {
	c := NewChain(&NoOpInterceptor{})
	opts, err := c.ServerOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) == 0 {
		t.Fatal("expected server options")
	}
}

func TestChain_ClientOptions(t *testing.T) {
	c := NewChain(&NoOpClientInterceptor{})
	opts, err := c.ClientOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) == 0 {
		t.Fatal("expected client options")
	}
}

func TestChain_DependencyOrder(t *testing.T) {
	c := NewChain(&NoOpInterceptor{})
	names, err := c.DependencyOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("expected names")
	}
	// metadata should be auto-added first
	if names[0] != "metadata" {
		t.Fatalf("first = %q, want metadata", names[0])
	}
}

func TestChain_DependencyGraph(t *testing.T) {
	c := NewChain(&NoOpInterceptor{})
	graph := c.DependencyGraph()
	if _, ok := graph["noop"]; !ok {
		t.Fatal("expected noop in graph")
	}
}

func TestChain_WithLogger(t *testing.T) {
	c := NewChain()
	ret := c.WithLogger(slog.New(slog.DiscardHandler))
	if ret != c {
		t.Fatal("WithLogger should return same chain")
	}
}
