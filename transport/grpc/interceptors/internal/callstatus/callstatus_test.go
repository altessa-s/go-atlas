// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package callstatus

import (
	"context"
	"sync"
	"testing"
)

func TestCauseConcurrentWithLateHandler(t *testing.T) {
	// A timeout interceptor can return to the observer while its downstream
	// handler is still unwinding through error conversion.
	ctx := Observe(t.Context())
	var g sync.WaitGroup
	g.Go(func() { Save(ctx, context.Canceled) })
	_ = Cause(ctx)
	g.Wait()
	if got := Cause(ctx); got != context.Canceled {
		t.Fatalf("saved cause = %v, want context.Canceled", got)
	}
}
