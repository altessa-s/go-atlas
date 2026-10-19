// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hostpacer_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters/hostpacer"
)

func BenchmarkPacer_Allow(b *testing.B) {
	p, err := hostpacer.New(time.Nanosecond, 1<<30)
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	for b.Loop() {
		_ = p.Allow(ctx, "api.example.com")
	}
}
