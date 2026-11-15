// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// teardownTimeout bounds the drop-and-close cleanup.
const teardownTimeout = 30 * time.Second

// databaseSuffixBytes is the entropy of the throwaway database name suffix (8 hex digits).
const databaseSuffixBytes = 4

// New returns a connected [datamongo.Mongo] bound to a throwaway database named <prefix><creation time>_<8 random hex digits> (the
// creation time is 8 hex digits of Unix seconds), and registers a cleanup that drops the database and closes the connection. The
// connection URI comes from the environment variable [DefaultEnv] (or [WithEnv]); the test is skipped when it is unset or empty, so
// a plain go test run needs no MongoDB. A configured but unusable MongoDB fails the test. The connection uses the go-atlas codec
// registry (decimal and civil date codecs) like every [datamongo.Mongo], and Connect runs the registered migrations.
//
// Every test gets its own client. New waits up to [DefaultReadinessTimeout] ([WithReadinessTimeout]) for that client's first server
// handshake before Connect, whose single ping has a fixed timeout and does not retry a server selection timeout: under a loaded
// suite the handshake alone can outlast it.
//
// A run killed before its cleanup leaves its databases behind. The first New of the process for a URI and prefix therefore drops
// every database of that prefix created more than [DefaultStaleAge] ([WithStaleAge]; zero disables) ago; names without a creation
// time are never touched.
func New(tb testing.TB, opts ...Option) *datamongo.Mongo {
	tb.Helper()
	o := newOptions(opts...)

	uri := os.Getenv(o.env)
	if uri == "" {
		tb.Skipf("%s not set; skipping integration test", o.env)
	}

	client, err := readyClient(tb.Context(), uri, o.readinessTimeout)
	if err != nil {
		tb.Fatalf("mongotest: connect mongo: %v", err)
	}
	sweepStale(tb, client, uri, o.databasePrefix, o.staleAge)
	m, err := datamongo.New(databaseName(o.databasePrefix, time.Now()), datamongo.WithClient(client))
	if err == nil {
		err = m.Connect(tb.Context())
	}
	if err != nil {
		disconnect(tb, client)
		tb.Fatalf("mongotest: connect mongo: %v", err)
	}

	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), teardownTimeout)
		defer cancel()
		_ = m.Database().Drop(ctx) //nolint:errcheck // best-effort teardown
		m.Close()
		disconnect(tb, client)
	})
	return m
}

// readyClient connects a client to uri with the go-atlas codec registry and waits up to timeout for the server to answer it: the
// server selection timeout is raised to the same budget, so the ping keeps waiting for the handshake instead of failing early.
func readyClient(ctx context.Context, uri string, timeout time.Duration) (*mongo.Client, error) {
	client, err := mongo.Connect(mongoOptions.Client().
		SetRegistry(bsoncodec.NewRegistry()).
		ApplyURI(uri).
		SetServerSelectionTimeout(timeout))
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		// Tear down at once: an unready server may also stall endSessions, which Disconnect sends first, and nothing would
		// then bound the wait (no socket timeout by default), so New would never report the readiness failure.
		torn, cancelTeardown := context.WithCancel(context.WithoutCancel(ctx))
		cancelTeardown()
		_ = client.Disconnect(torn) //nolint:errcheck // the ping error is the outcome
		return nil, err
	}
	return client, nil
}

// disconnect closes the test's client (best effort): a [datamongo.Mongo] built on a given client does not own it.
func disconnect(tb testing.TB, client *mongo.Client) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), teardownTimeout)
	defer cancel()
	_ = client.Disconnect(ctx) //nolint:errcheck // best-effort teardown
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
