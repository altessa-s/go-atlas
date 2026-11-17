// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package purgeloop_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/purgeloop"
)

func BenchmarkStartAlreadyStarted(b *testing.B) {
	l := purgeloop.New(time.Hour, func(context.Context) (int64, error) { return 0, nil }, nil, "rows")
	l.Start()
	b.Cleanup(func() {
		if err := l.Close(context.WithoutCancel(b.Context())); err != nil {
			b.Fatal(err)
		}
	})
	b.ReportAllocs()
	for b.Loop() {
		l.Start()
	}
}
