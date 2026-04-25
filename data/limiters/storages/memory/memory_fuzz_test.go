// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
)

func FuzzProvider_Allow(f *testing.F) {
	f.Add("key1", int64(10), int64(60))
	f.Add("", int64(1), int64(1))
	f.Add("special:key", int64(100), int64(3600))

	p := memory.New()
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key string, limit, periodSec int64) {
		if key == "" || limit <= 0 || periodSec <= 0 {
			return
		}
		period := time.Duration(periodSec) * time.Second
		if period <= 0 {
			return
		}
		// Should not panic
		_, _ = p.Allow(ctx, key, limit, period)
	})
}
