// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		New()
	}
}

func BenchmarkNewWithOptions(b *testing.B) {
	for b.Loop() {
		New(WithSize(20))
	}
}

func BenchmarkGetAndReturnConnection(b *testing.B) {
	factory := func(ctx context.Context, target string) (*grpc.ClientConn, error) {
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	p := New(WithClientFactory(factory))
	ctx := b.Context()

	stop, err := p.Start(ctx)
	if err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	defer stop()

	for b.Loop() {
		conn, err := p.GetConnection(ctx, "localhost:0")
		if err != nil {
			b.Fatalf("GetConnection() error = %v", err)
		}
		p.ReturnConnection(conn)
	}
}

func BenchmarkGetAndReturnConnection_ParallelTargets(b *testing.B) {
	factory := func(ctx context.Context, target string) (*grpc.ClientConn, error) {
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	p := New(WithClientFactory(factory), WithSize(64))
	ctx := b.Context()

	stop, err := p.Start(ctx)
	if err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	defer stop()

	targets := []string{"localhost:1", "localhost:2", "localhost:3", "localhost:4"}
	var next atomic.Uint32
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		target := targets[int(next.Add(1))%len(targets)]
		for pb.Next() {
			conn, err := p.GetConnection(ctx, target)
			if err != nil {
				b.Errorf("GetConnection() error = %v", err)
				return
			}
			p.ReturnConnection(conn)
		}
	})
}
