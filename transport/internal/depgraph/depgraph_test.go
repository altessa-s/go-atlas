// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package depgraph

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var discardLogger = slog.New(slog.DiscardHandler)

type testItem struct {
	name string
	deps []string
}

func (t *testItem) Name() string           { return t.name }
func (t *testItem) Dependencies() []string { return t.deps }

type simpleItem struct{ name string }

func (s *simpleItem) Name() string { return s.name }

func TestGraph_EmptySort(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	result, err := g.TopologicalSort()
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestGraph_SingleNode(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})

	result, err := g.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "a", result[0].Name())
}

func TestGraph_LinearDependency(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})
	g.AddNode("b", &simpleItem{"b"})
	g.AddNode("c", &simpleItem{"c"})
	g.AddEdge("a", "b") // a before b
	g.AddEdge("b", "c") // b before c

	result, err := g.TopologicalSort()
	require.NoError(t, err)
	names := make([]string, len(result))
	for i, r := range result {
		names[i] = r.Name()
	}
	require.Equal(t, "a,b,c", strings.Join(names, ","))
}

func TestGraph_CycleDetection(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})
	g.AddNode("b", &simpleItem{"b"})
	g.AddEdge("a", "b")
	g.AddEdge("b", "a")

	_, err := g.TopologicalSort()
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "circular dependency"))
}

func TestGraph_HasNode(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})

	require.True(t, g.HasNode("a"), "HasNode(a) = false")
	require.False(t, g.HasNode("b"), "HasNode(b) = true")
}

func TestGraph_DuplicateAddNode(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})
	g.AddNode("a", &simpleItem{"a2"}) // should be ignored

	result, _ := g.TopologicalSort()
	require.Len(t, result, 1)
}

func TestGraph_InsertionOrderStable(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("c", &simpleItem{"c"})
	g.AddNode("b", &simpleItem{"b"})
	g.AddNode("a", &simpleItem{"a"})

	result, err := g.TopologicalSort()
	require.NoError(t, err)
	// Should maintain insertion order: c, b, a (no dependencies)
	names := make([]string, len(result))
	for i, r := range result {
		names[i] = r.Name()
	}
	require.Equal(t, "c,b,a", strings.Join(names, ","))
}

func TestBuild(t *testing.T) {
	items := []*testItem{
		{name: "c", deps: []string{"a", "b"}},
		{name: "a", deps: nil},
		{name: "b", deps: []string{"a"}},
	}

	sorted, err := Build(items, discardLogger)
	require.NoError(t, err)

	names := make([]string, len(sorted))
	for i, s := range sorted {
		names[i] = s.Name()
	}

	// a must come before b and c; b must come before c
	aIdx, bIdx, cIdx := -1, -1, -1
	for i, n := range names {
		switch n {
		case "a":
			aIdx = i
		case "b":
			bIdx = i
		case "c":
			cIdx = i
		}
	}
	require.True(t, aIdx <= bIdx && aIdx <= cIdx && bIdx <= cIdx, "wrong order: %v", names)
}

func TestBuild_Empty(t *testing.T) {
	result, err := Build[*testItem](nil, discardLogger)
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestBuild_MissingDependency(t *testing.T) {
	items := []*testItem{
		{name: "a", deps: []string{"missing"}},
	}

	sorted, err := Build(items, discardLogger)
	require.NoError(t, err)
	require.Len(t, sorted, 1)
}

type requiredTestItem struct {
	name    string
	deps    []string
	reqDeps []string
}

func (t *requiredTestItem) Name() string                   { return t.name }
func (t *requiredTestItem) Dependencies() []string         { return t.deps }
func (t *requiredTestItem) RequiredDependencies() []string { return t.reqDeps }

func TestBuild_RequiredDependencyPresent(t *testing.T) {
	items := []*requiredTestItem{
		{name: "realip"},
		{name: "limiter", reqDeps: []string{"realip"}},
	}

	sorted, err := Build(items, discardLogger)
	require.NoError(t, err)

	require.Len(t, sorted, 2)
	require.Equal(t, "realip", sorted[0].Name())
	require.Equal(t, "limiter", sorted[1].Name())
}

func TestBuild_RequiredDependencyMissing(t *testing.T) {
	items := []*requiredTestItem{
		{name: "limiter", reqDeps: []string{"realip"}},
	}

	_, err := Build(items, discardLogger)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "requires dependency"))
	require.True(t, strings.Contains(err.Error(), "realip"))
}

func TestBuild_MixedRequiredAndOptional(t *testing.T) {
	items := []*requiredTestItem{
		{name: "realip"},
		{name: "metadata"},
		{name: "audit", deps: []string{"metadata", "tracing"}, reqDeps: []string{"realip"}},
	}

	sorted, err := Build(items, discardLogger)
	require.NoError(t, err)

	// realip and metadata must come before audit; tracing is optional and missing
	names := make([]string, len(sorted))
	for i, s := range sorted {
		names[i] = s.Name()
	}

	auditIdx := -1
	for i, n := range names {
		if n == "audit" {
			auditIdx = i
		}
	}
	require.GreaterOrEqual(t, auditIdx, 2, "audit should be after realip and metadata, got order: %v", names)
}

func TestBuild_OverlappingRequiredAndOptional(t *testing.T) {
	// Same dep declared as both required and optional should not create
	// a duplicate edge (which would inflate inDegree and break topo sort).
	items := []*requiredTestItem{
		{name: "realip"},
		{name: "limiter", deps: []string{"realip"}, reqDeps: []string{"realip"}},
	}

	sorted, err := Build(items, discardLogger)
	require.NoError(t, err)
	require.Len(t, sorted, 2)
	require.Equal(t, "realip", sorted[0].Name())
	require.Equal(t, "limiter", sorted[1].Name())
}

func TestGraph_PriorityOrder(t *testing.T) {
	// Simulates the interceptor ordering bug: errstatus (order 1) depends on
	// requestid (order 3) which depends on metadata (order 0). limiter
	// (order 4) depends on realip (order 2). Without the priority-queue fix,
	// limiter would be emitted before errstatus because it becomes zero-degree
	// earlier. With the fix, errstatus (lower insertion order) comes first.
	g := New[*simpleItem](discardLogger)
	g.AddNode("metadata", &simpleItem{"metadata"})   // order 0
	g.AddNode("errstatus", &simpleItem{"errstatus"}) // order 1
	g.AddNode("realip", &simpleItem{"realip"})       // order 2
	g.AddNode("requestid", &simpleItem{"requestid"}) // order 3
	g.AddNode("limiter", &simpleItem{"limiter"})     // order 4

	g.AddEdge("metadata", "requestid")  // requestid depends on metadata
	g.AddEdge("requestid", "errstatus") // errstatus depends on requestid
	g.AddEdge("realip", "limiter")      // limiter depends on realip

	result, err := g.TopologicalSort()
	require.NoError(t, err)

	names := make([]string, len(result))
	for i, r := range result {
		names[i] = r.Name()
	}

	expected := "metadata,realip,requestid,errstatus,limiter"
	require.Equal(t, expected, strings.Join(names, ","))
}

func TestBuild_Cycle(t *testing.T) {
	items := []*testItem{
		{name: "a", deps: []string{"b"}},
		{name: "b", deps: []string{"a"}},
	}

	_, err := Build(items, discardLogger)
	require.Error(t, err)
}
