// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// StartNATSServer starts an embedded NATS server with JetStream enabled for testing.
// The server binds to a random port on 127.0.0.1 and uses tb.TempDir for JetStream
// storage. It waits up to 5 seconds for the server to become ready and calls
// tb.Fatal if it does not. Shutdown is handled automatically via tb.Cleanup.
func StartNATSServer(tb testing.TB) *server.Server {
	tb.Helper()

	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Random port
		NoLog:     true,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  tb.TempDir(),
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		tb.Fatalf("failed to create NATS server: %v", err)
	}

	ns.Start()

	if !ns.ReadyForConnections(5 * time.Second) { //nolint:mnd // test constant
		tb.Fatal("NATS server not ready for connections")
	}

	tb.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	return ns
}

// ConnectNATS connects to the given NATS server and returns the connection.
// The connection is closed automatically via tb.Cleanup.
func ConnectNATS(tb testing.TB, ns *server.Server) *nats.Conn {
	tb.Helper()

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		tb.Fatalf("failed to connect to NATS: %v", err)
	}

	tb.Cleanup(func() {
		nc.Close()
	})

	return nc
}

// ConnectJetStream connects to the NATS server and returns both the underlying
// [nats.Conn] and a [jetstream.JetStream] handle. It calls [ConnectNATS] internally,
// so the connection is cleaned up automatically.
func ConnectJetStream(tb testing.TB, ns *server.Server) (*nats.Conn, jetstream.JetStream) {
	tb.Helper()

	nc := ConnectNATS(tb, ns)
	js, err := jetstream.New(nc)
	if err != nil {
		tb.Fatalf("failed to create JetStream context: %v", err)
	}

	return nc, js
}

// JetStreamKVCapture is a [jetstream.JetStream] test double that records the
// [jetstream.KeyValueConfig] passed to CreateOrUpdateKeyValue. KeyValue always
// reports [jetstream.ErrBucketNotFound] so callers fall through to the bucket
// creation path. All other JetStream methods panic via the embedded nil
// interface. Useful for asserting bucket configuration (replicas, TTL, storage)
// that a single-node test server cannot express.
type JetStreamKVCapture struct {
	jetstream.JetStream

	// KVConfig is the config captured by the last CreateOrUpdateKeyValue call.
	KVConfig jetstream.KeyValueConfig
}

// KeyValue always returns [jetstream.ErrBucketNotFound] to force bucket creation.
func (c *JetStreamKVCapture) KeyValue(context.Context, string) (jetstream.KeyValue, error) {
	return nil, jetstream.ErrBucketNotFound
}

// CreateOrUpdateKeyValue records cfg and returns an inert KeyValue whose
// methods panic when called.
func (c *JetStreamKVCapture) CreateOrUpdateKeyValue(_ context.Context, cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	c.KVConfig = cfg
	return inertKeyValue{}, nil
}

// inertKeyValue is a non-nil [jetstream.KeyValue] whose methods panic via the
// embedded nil interface. It exists only to satisfy post-creation nil checks.
type inertKeyValue struct {
	jetstream.KeyValue
}

// CreateNATSKV creates a NATS JetStream KeyValue bucket backed by in-memory storage.
func CreateNATSKV(tb testing.TB, js jetstream.JetStream, bucket string, ttl time.Duration) jetstream.KeyValue {
	tb.Helper()

	kv, err := js.CreateKeyValue(tb.Context(), jetstream.KeyValueConfig{
		Bucket:  bucket,
		TTL:     ttl,
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		tb.Fatalf("failed to create NATS KV bucket %q: %v", bucket, err)
	}
	return kv
}
