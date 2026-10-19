// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// teardownTimeout bounds the drop-and-close cleanup.
const teardownTimeout = 30 * time.Second

// databaseSuffixBytes is the entropy of the throwaway database name suffix (8 hex digits).
const databaseSuffixBytes = 4

// New returns a connected [datamongo.Mongo] bound to a throwaway database named by the database prefix and 8 random hex digits,
// and registers a cleanup that drops the database and closes the connection. The connection URI comes from the environment variable
// [DefaultEnv] (or [WithEnv]); the test is skipped when it is unset or empty, so a plain go test run needs no MongoDB. A configured
// but unusable MongoDB fails the test. The connection uses the go-atlas codec registry (decimal and civil date codecs) like every
// [datamongo.Mongo], and Connect runs the registered migrations.
func New(tb testing.TB, opts ...Option) *datamongo.Mongo {
	tb.Helper()
	o := newOptions(opts...)

	uri := os.Getenv(o.env)
	if uri == "" {
		tb.Skipf("%s not set; skipping integration test", o.env)
	}

	suffix := make([]byte, databaseSuffixBytes)
	_, _ = rand.Read(suffix)
	m, err := datamongo.New(o.databasePrefix+hex.EncodeToString(suffix),
		datamongo.WithClientOptions(mongoOptions.Client().ApplyURI(uri)))
	if err != nil {
		tb.Fatalf("mongotest: build mongo: %v", err)
	}
	if err := m.Connect(tb.Context()); err != nil {
		tb.Fatalf("mongotest: connect mongo: %v", err)
	}

	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), teardownTimeout)
		defer cancel()
		_ = m.Database().Drop(ctx) //nolint:errcheck // best-effort teardown
		m.Close()
	})
	return m
}

// IndexNames returns the sorted index names of collection coll, including the implicit "_id_" (none for a missing collection).
func IndexNames(tb testing.TB, m *datamongo.Mongo, coll string) []string {
	tb.Helper()
	specs, err := m.Database().Collection(coll).Indexes().ListSpecifications(tb.Context())
	if err != nil {
		tb.Fatalf("mongotest: list %s indexes: %v", coll, err)
	}
	names := make([]string, len(specs))
	for i, sp := range specs {
		names[i] = sp.Name
	}
	slices.Sort(names)
	return names
}

// RequireReplicaSet fails the test unless m is connected to a replica set member (or a mongos), which transactions and change
// streams need.
func RequireReplicaSet(tb testing.TB, m *datamongo.Mongo) {
	tb.Helper()
	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"`
	}
	if err := m.Client().Database("admin").RunCommand(tb.Context(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		tb.Fatalf("mongotest: hello: %v", err)
	}
	if hello.SetName == "" && hello.Msg != "isdbgrid" {
		tb.Fatalf("mongotest: MongoDB at %s is a standalone server; this test needs a replica set", m.DatabaseName())
	}
}
