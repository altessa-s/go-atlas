// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/mongo"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// BenchmarkLockRelease measures one acquire/release round trip pair against a
// live MongoDB (MONGO_URI); it is skipped without one.
func BenchmarkLockRelease(b *testing.B) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		b.Skip("MONGO_URI not set")
	}
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
	if err != nil {
		b.Skip(err)
	}
	b.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("dlock_bench_" + strconv.FormatInt(time.Now().UnixNano(), 10))
	b.Cleanup(func() { _ = db.Drop(context.Background()) })
	l, err := mongo.New(db)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = l.Close(context.Background()) })

	for b.Loop() {
		lk, err := l.Lock(b.Context(), "bench")
		if err != nil {
			b.Fatal(err)
		}
		if err := lk.Release(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
}
