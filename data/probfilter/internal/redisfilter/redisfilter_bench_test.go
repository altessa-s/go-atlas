// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// newBenchCore wires a Core against a miniredis with no-op T.* handlers so the
// benchmarks measure the shared exec plumbing, not RedisBloom itself.
func newBenchCore(b *testing.B) *redisfilter.Core {
	b.Helper()

	client, mr := testhelpers.RedisClient(b)
	srv := mr.Server()
	intReply := func(c *server.Peer, _ string, _ []string) { c.WriteInt(1) }
	require.NoError(b, srv.Register(testCommands.Exists, intReply))
	require.NoError(b, srv.Register(testCommands.Add, intReply))

	return redisfilter.New(client, "bench:f", testCommands)
}

func BenchmarkCore_MightExist(b *testing.B) {
	core := newBenchCore(b)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := core.MightExist(ctx, "value"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCore_Add(b *testing.B) {
	core := newBenchCore(b)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := core.Add(ctx, "value"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInfoFields(b *testing.B) {
	reply := []any{
		"Capacity", int64(100000),
		"Number of items inserted", int64(42),
		"Size", int64(4096),
	}

	b.ReportAllocs()
	for b.Loop() {
		var total int64
		for _, raw := range redisfilter.InfoFields(reply) {
			if v, err := redisfilter.ToInt64(raw); err == nil {
				total += v
			}
		}
		_ = total
	}
}
