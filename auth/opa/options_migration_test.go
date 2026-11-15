// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"testing"
	"time"
)

func TestWithDecisionCacheBaseline(t *testing.T) {
	got := defaultOptions()
	WithDecisionCache(23, 17*time.Second)(got)
	if got.decisionCache == nil || got.decisionCache.entries == nil {
		t.Fatalf("cache not initialized: %#v", got.decisionCache)
	}
}
