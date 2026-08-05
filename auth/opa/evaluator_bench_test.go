// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func BenchmarkEvaluate_Boolean(b *testing.B) {
	ctx := b.Context()
	dir := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(dir, "policy.rego"), []byte(`
package bench.authz
import rego.v1
allow if input.role == "admin"
`), 0644))

	source, err := filesystem.New(dir)
	require.NoError(b, err)

	manager, err := opa.NewManager(ctx, source, "data.bench.authz.allow")
	require.NoError(b, err)
	b.Cleanup(func() { manager.Close() })

	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	b.ResetTimer()
	for b.Loop() {
		_, err := evaluator.Evaluate(ctx, input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvaluate_Structured(b *testing.B) {
	ctx := b.Context()
	dir := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(dir, "policy.rego"), []byte(`
package bench.authz
import rego.v1

default allow := false

deny contains {"code": "PERMISSION_DENIED", "message": "denied"} if {
    input.role != "admin"
}

allow if count(deny) == 0

result := {"allow": allow, "denials": deny}
`), 0644))

	source, err := filesystem.New(dir)
	require.NoError(b, err)

	manager, err := opa.NewManager(ctx, source, "data.bench.authz.result")
	require.NoError(b, err)
	b.Cleanup(func() { manager.Close() })

	evaluator := manager.Evaluator()
	input := map[string]any{"role": "viewer"}

	b.ResetTimer()
	for b.Loop() {
		_, err := evaluator.Evaluate(ctx, input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHasDenialCode(b *testing.B) {
	r := &opa.Result{
		Allow: false,
		Denials: coremaps.NewImmutableMap(map[string]string{
			"RESOURCE_INACTIVE": "resource is inactive",
			"PERMISSION_DENIED": "insufficient permissions",
			"QUOTA_EXCEEDED":    "quota exceeded",
		}),
	}

	for b.Loop() {
		_ = r.HasDenialCode("PERMISSION_DENIED")
	}
}

// BenchmarkEvaluate_Cached measures the decision cache against the same
// evaluation uncached: the cache trades a Rego evaluation for a JSON encode
// plus a SHA-256, so whether it pays off depends on policy complexity.
func BenchmarkEvaluate_Cached(b *testing.B) {
	dir := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(dir, "policy.rego"), []byte(`
package bench.authz
import rego.v1
allow if input.role == "admin"
`), 0644))

	source, err := filesystem.New(dir)
	require.NoError(b, err)

	manager, err := opa.NewManager(b.Context(), source, "data.bench.authz.allow",
		opa.WithDecisionCache(1000, time.Minute))
	require.NoError(b, err)
	b.Cleanup(func() { _ = manager.Close() })

	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	// Prime the entry so the loop measures hits, not the first miss.
	_, err = evaluator.Evaluate(b.Context(), input)
	require.NoError(b, err)

	ctx := b.Context()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := evaluator.Evaluate(ctx, input); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecisionCacheKey isolates the per-lookup cost the cache adds on top
// of a hit: marshalling the input and hashing it.
func BenchmarkDecisionCacheKey(b *testing.B) {
	dir := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(dir, "policy.rego"), []byte(`
package bench.authz
import rego.v1
allow if input.role == "admin"
`), 0644))

	source, err := filesystem.New(dir)
	require.NoError(b, err)

	manager, err := opa.NewManager(b.Context(), source, "data.bench.authz.allow",
		opa.WithDecisionCache(1000, time.Minute))
	require.NoError(b, err)
	b.Cleanup(func() { _ = manager.Close() })

	evaluator := manager.Evaluator()
	ctx := b.Context()

	// A realistic authorization input rather than a single field.
	input := map[string]any{
		"role":    "admin",
		"subject": map[string]any{"id": "user-1", "tenant": "acme", "groups": []string{"a", "b", "c"}},
		"action":  "documents.read",
		"resource": map[string]any{
			"type": "document", "id": "doc-42", "owner": "user-2",
		},
	}

	_, err = evaluator.Evaluate(ctx, input)
	require.NoError(b, err)

	b.ReportAllocs()

	for b.Loop() {
		if _, err := evaluator.Evaluate(ctx, input); err != nil {
			b.Fatal(err)
		}
	}
}
