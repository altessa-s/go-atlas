// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
)

func BenchmarkLimit_IPHappyPath(b *testing.B) {
	l, err := tokenbucket.New(validConfig(), memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(context.Context) string { return "192.168.1.100" }),
	)
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_, _ = l.Limit(ctx)
	}
}
