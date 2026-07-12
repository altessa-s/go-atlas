// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import "testing"

func BenchmarkRateLimit(b *testing.B) {
	i := ServerInterceptor(fakeLimiter(), WithExposeHeaders()).(*interceptor)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_, _ = i.rateLimit(ctx, "/svc/Method")
	}
}
