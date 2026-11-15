// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/factory/ext/core/dispatch/observe"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

// BenchmarkObserver_ItemEnqueued measures the per-Submit cost the observer
// adds to the engine's admission path.
func BenchmarkObserver_ItemEnqueued(b *testing.B) {
	o := observe.New(metricstest.NewTestCollector())
	b.ReportAllocs()
	for b.Loop() {
		o.ItemEnqueued()
	}
}
