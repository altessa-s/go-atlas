// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package depgraph

import (
	"log/slog"
	"strings"
	"testing"
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
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	if result != nil {
		t.Fatalf("TopologicalSort() = %v, want nil", result)
	}
}

func TestGraph_SingleNode(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})

	result, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	if len(result) != 1 || result[0].Name() != "a" {
		t.Fatalf("TopologicalSort() = %v, want [a]", result)
	}
}

func TestGraph_LinearDependency(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})
	g.AddNode("b", &simpleItem{"b"})
	g.AddNode("c", &simpleItem{"c"})
	g.AddEdge("a", "b") // a before b
	g.AddEdge("b", "c") // b before c

	result, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	names := make([]string, len(result))
	for i, r := range result {
		names[i] = r.Name()
	}
	if strings.Join(names, ",") != "a,b,c" {
		t.Fatalf("TopologicalSort() = %v, want [a,b,c]", names)
	}
}

func TestGraph_CycleDetection(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})
	g.AddNode("b", &simpleItem{"b"})
	g.AddEdge("a", "b")
	g.AddEdge("b", "a")

	_, err := g.TopologicalSort()
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Fatalf("error = %q, want containing 'circular dependency'", err.Error())
	}
}

func TestGraph_HasNode(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})

	if !g.HasNode("a") {
		t.Fatal("HasNode(a) = false")
	}
	if g.HasNode("b") {
		t.Fatal("HasNode(b) = true")
	}
}

func TestGraph_DuplicateAddNode(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("a", &simpleItem{"a"})
	g.AddNode("a", &simpleItem{"a2"}) // should be ignored

	result, _ := g.TopologicalSort()
	if len(result) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result))
	}
}

func TestGraph_InsertionOrderStable(t *testing.T) {
	g := New[*simpleItem](discardLogger)
	g.AddNode("c", &simpleItem{"c"})
	g.AddNode("b", &simpleItem{"b"})
	g.AddNode("a", &simpleItem{"a"})

	result, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	// Should maintain insertion order: c, b, a (no dependencies)
	names := make([]string, len(result))
	for i, r := range result {
		names[i] = r.Name()
	}
	if strings.Join(names, ",") != "c,b,a" {
		t.Fatalf("got %v, want [c,b,a]", names)
	}
}

func TestBuild(t *testing.T) {
	items := []*testItem{
		{name: "c", deps: []string{"a", "b"}},
		{name: "a", deps: nil},
		{name: "b", deps: []string{"a"}},
	}

	sorted, err := Build[*testItem](items, discardLogger)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

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
	if aIdx > bIdx || aIdx > cIdx || bIdx > cIdx {
		t.Fatalf("wrong order: %v", names)
	}
}

func TestBuild_Empty(t *testing.T) {
	result, err := Build[*testItem](nil, discardLogger)
	if err != nil {
		t.Fatalf("Build(nil) error = %v", err)
	}
	if result != nil {
		t.Fatalf("Build(nil) = %v, want nil", result)
	}
}

func TestBuild_MissingDependency(t *testing.T) {
	items := []*testItem{
		{name: "a", deps: []string{"missing"}},
	}

	sorted, err := Build[*testItem](items, discardLogger)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(sorted) != 1 {
		t.Fatalf("expected 1 item, got %d", len(sorted))
	}
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

	sorted, err := Build[*requiredTestItem](items, discardLogger)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(sorted) != 2 {
		t.Fatalf("expected 2 items, got %d", len(sorted))
	}
	if sorted[0].Name() != "realip" || sorted[1].Name() != "limiter" {
		t.Fatalf("wrong order: %v, %v", sorted[0].Name(), sorted[1].Name())
	}
}

func TestBuild_RequiredDependencyMissing(t *testing.T) {
	items := []*requiredTestItem{
		{name: "limiter", reqDeps: []string{"realip"}},
	}

	_, err := Build[*requiredTestItem](items, discardLogger)
	if err == nil {
		t.Fatal("expected error for missing required dependency")
	}
	if !strings.Contains(err.Error(), "requires dependency") {
		t.Fatalf("error = %q, want containing 'requires dependency'", err.Error())
	}
	if !strings.Contains(err.Error(), "realip") {
		t.Fatalf("error = %q, want containing 'realip'", err.Error())
	}
}

func TestBuild_MixedRequiredAndOptional(t *testing.T) {
	items := []*requiredTestItem{
		{name: "realip"},
		{name: "metadata"},
		{name: "audit", deps: []string{"metadata", "tracing"}, reqDeps: []string{"realip"}},
	}

	sorted, err := Build[*requiredTestItem](items, discardLogger)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

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
	if auditIdx < 2 {
		t.Fatalf("audit should be after realip and metadata, got order: %v", names)
	}
}

func TestBuild_OverlappingRequiredAndOptional(t *testing.T) {
	// Same dep declared as both required and optional should not create
	// a duplicate edge (which would inflate inDegree and break topo sort).
	items := []*requiredTestItem{
		{name: "realip"},
		{name: "limiter", deps: []string{"realip"}, reqDeps: []string{"realip"}},
	}

	sorted, err := Build[*requiredTestItem](items, discardLogger)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(sorted) != 2 {
		t.Fatalf("expected 2 items, got %d", len(sorted))
	}
	if sorted[0].Name() != "realip" || sorted[1].Name() != "limiter" {
		t.Fatalf("wrong order: %v, %v", sorted[0].Name(), sorted[1].Name())
	}
}

func TestBuild_Cycle(t *testing.T) {
	items := []*testItem{
		{name: "a", deps: []string{"b"}},
		{name: "b", deps: []string{"a"}},
	}

	_, err := Build[*testItem](items, discardLogger)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}
