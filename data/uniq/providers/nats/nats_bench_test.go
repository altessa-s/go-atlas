// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	uniqnats "github.com/altessa-s/go-atlas/data/uniq/providers/nats"
)

func benchProvider(b *testing.B) *uniqnats.Provider {
	b.Helper()
	ns := testhelpers.StartNATSServer(b)
	nc := testhelpers.ConnectNATS(b, ns)

	p, err := uniqnats.New(nc, uniqnats.WithBucket("bench-uniq"))
	if err != nil {
		b.Fatalf("failed to create provider: %v", err)
	}
	return p
}

func BenchmarkProvider_Add(b *testing.B) {
	p := benchProvider(b)
	ctx := b.Context()
	for b.Loop() {
		_ = p.Add(ctx, "bench-key")
	}
}

func BenchmarkProvider_Exist(b *testing.B) {
	p := benchProvider(b)
	ctx := b.Context()
	_ = p.Add(ctx, "bench-key")
	for b.Loop() {
		_, _ = p.Exist(ctx, "bench-key")
	}
}
