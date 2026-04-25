// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"testing"
)

func BenchmarkCoordinator_CheckStatus(b *testing.B) {
	c := New()
	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusServing }))
	ctx := b.Context()
	b.ResetTimer()
	for b.Loop() {
		c.CheckStatus(ctx, "svc")
	}
}

func BenchmarkWatcher_Notify(b *testing.B) {
	w := newWatcher(1024, StatusUnknown)
	b.ResetTimer()
	for b.Loop() {
		w.notify(StatusServing)
		<-w.ch
	}
}

func BenchmarkServingStatus_String(b *testing.B) {
	s := StatusServing
	b.ResetTimer()
	for b.Loop() {
		_ = s.String()
	}
}
