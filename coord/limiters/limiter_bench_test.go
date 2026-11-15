// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"
	"testing"
)

func BenchmarkLimitInfo_IsLimitExceeded(b *testing.B) {
	li := &LimitInfo{Remaining: 5}
	for b.Loop() {
		li.IsLimitExceeded()
	}
}

func BenchmarkFunc_Limit(b *testing.B) {
	fn := Func(func(_ context.Context) (*LimitInfo, error) {
		return &LimitInfo{Limit: 100, Remaining: 50}, nil
	})
	ctx := b.Context()
	for b.Loop() {
		fn.Limit(ctx)
	}
}
