// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRequireAllDependencies_AggregatesAllMissing asserts that every missing
// dependency is reported, not just the first one observed during map
// iteration. Before this fix, callers had to re-run validation repeatedly to
// surface each missing dep in turn.
func TestRequireAllDependencies_AggregatesAllMissing(t *testing.T) {
	b := NewBase(nil)

	err := b.RequireAllDependencies(map[string]any{
		"database": nil,
		"cache":    nil,
		"logger":   "present",
		"metrics":  nil,
	})
	require.Error(t, err, "expected aggregated error")

	msg := err.Error()
	for _, name := range []string{"database", "cache", "metrics"} {
		require.Contains(t, msg, name+" is required", "aggregated error missing %q: %s", name, msg)
	}
	require.NotContains(t, msg, "logger is required", "aggregated error unexpectedly reports present dep: %s", msg)
}

// TestRequireAllDependencies_AllPresentReturnsNil is a sanity check that the
// happy path still returns nil after the switch from first-error to
// errors.Join.
func TestRequireAllDependencies_AllPresentStillNil(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireAllDependencies(map[string]any{
		"a": "val",
		"b": 42,
		"c": struct{}{},
	})
	require.NoError(t, err)
}
