// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWithDecisionCacheBaseline(t *testing.T) {
	t.Parallel()
	got := defaultOptions()
	WithDecisionCache(23, 17*time.Second)(got)
	got.decisionCache.Put("key", Result{Allow: true})
	result, ok := got.decisionCache.Get("key")
	require.True(t, ok)
	require.True(t, result.Allow)
}
