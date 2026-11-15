// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
)

func BenchmarkLimit_IPHappyPath(b *testing.B) {
	l, err := tokenbucket.New(validDefaults(), memory.New(),
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

func BenchmarkLimit_IPv6HappyPath(b *testing.B) {
	l, err := tokenbucket.New(validDefaults(), memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(context.Context) string { return "2001:db8:1:2::abcd" }),
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

func BenchmarkLimit_TokenHappyPath(b *testing.B) {
	settings := &tokenbucket.RateLimitSettings{Limit: 1 << 40, Period: time.Minute}
	l, err := tokenbucket.New(validDefaults(), memory.New(),
		tokenbucket.WithExtractToken(func(context.Context) string { return "bench-bearer-token" }),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(context.Context, string) (*tokenbucket.RateLimitSettings, error) { return settings, nil })),
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
