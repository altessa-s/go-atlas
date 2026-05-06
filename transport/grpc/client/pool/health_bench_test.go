// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// BenchmarkStateForTarget measures the synchronous read path used by Client
// in pool mode and by [ConnectionPool.CheckHealth].
func BenchmarkStateForTarget(b *testing.B) {
	tr := newStateTracker()
	b.Cleanup(tr.shutdown)

	conn, err := grpc.NewClient("127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = conn.Close() })
	pc := &pooledConnection{conn: conn, target: "test"}
	tr.attach("api", pc)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = tr.stateForTarget("api")
	}
}

// BenchmarkSubscribeFanOut measures the fan-out path executed on every
// state transition; subscriber count = 4 (a realistic upper bound when
// several Clients share a pool).
func BenchmarkSubscribeFanOut(b *testing.B) {
	tr := newStateTracker()
	b.Cleanup(tr.shutdown)

	conn, err := grpc.NewClient("127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = conn.Close() })
	pc := &pooledConnection{conn: conn, target: "test"}
	tr.attach("api", pc)

	for range 4 {
		_ = tr.subscribe("api", func(connectivity.State) {})
	}
	entry := tr.perTarget["api"][pc]

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tr.recordAndFanOut("api", entry, connectivity.Ready)
	}
}
