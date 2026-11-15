// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest_test

import (
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"

	"github.com/altessa-s/go-atlas/store/mongo/mongotest"
)

// TestNewSkipsWithoutURI: with the environment variable unset the test is skipped, not failed.
func TestNewSkipsWithoutURI(t *testing.T) {
	t.Parallel()

	var skipped bool
	t.Run("gated", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		mongotest.New(t, mongotest.WithEnv("MONGOTEST_SURELY_UNSET_URI"))
		t.Error("New returned without a URI")
	})
	require.True(t, skipped)
}

// TestNewIT runs against the MongoDB of MONGO_URI (skipped when unset): a throwaway database with the prefix, index names,
// replica set check, and the drop on cleanup.
func TestNewIT(t *testing.T) {
	t.Parallel()

	var dbName string
	t.Run("connect", func(t *testing.T) {
		m := mongotest.New(t, mongotest.WithDatabasePrefix("mongotest_it_"))
		dbName = m.DatabaseName()
		require.Regexp(t, regexp.MustCompile(`^mongotest_it_[0-9a-f]{8}_[0-9a-f]{8}$`), dbName)

		require.Empty(t, mongotest.IndexNames(t, m, "missing"))
		coll := m.Database().Collection("things")
		_, err := coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "b", Value: 1}}})
		require.NoError(t, err)
		_, err = coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
		require.NoError(t, err)
		require.Equal(t, []string{"_id_", "a_1", "b_1"}, mongotest.IndexNames(t, m, "things"))

		mongotest.RequireReplicaSet(t, m)
	})
	if dbName == "" {
		t.Skip("MONGO_URI not set")
	}

	// The cleanup dropped the database: a fresh connection no longer lists it.
	check := mongotest.New(t)
	names, err := check.Client().ListDatabaseNames(t.Context(), bson.D{{Key: "name", Value: dbName}})
	require.NoError(t, err)
	require.Empty(t, names)
}

// TestNewWaitsForASlowHandshakeIT: a server answering a new client's first connections later than the fixed ping of Connect (a
// loaded suite) still yields a connection. The server is reached through a proxy that holds the first two connections (the client's
// monitors) for 6s; without the readiness wait New failed with "server selection error: context deadline exceeded".
func TestNewWaitsForASlowHandshakeIT(t *testing.T) {
	// Serial: t.Setenv points the URI variable at the proxy.
	u, upstream := proxyTarget(t)
	u.Host = slowProxy(t, upstream, 2, 6*time.Second)
	t.Setenv(mongotest.DefaultEnv, u.String())

	m := mongotest.New(t)
	_, err := m.Database().Collection("probe").InsertOne(t.Context(), bson.M{"_id": "x"})
	require.NoError(t, err)
}

// TestNewReadinessTimeoutBoundsTheWaitIT: a server that does not answer within the readiness budget fails the test with the
// server selection error instead of hanging.
func TestNewReadinessTimeoutBoundsTheWaitIT(t *testing.T) {
	// Serial: t.Setenv points the URI variable at the proxy.
	u, upstream := proxyTarget(t)
	u.Host = slowProxy(t, upstream, 1<<20, time.Minute)
	t.Setenv(mongotest.DefaultEnv, u.String())

	var failed bool
	start := time.Now()
	t.Run("unready", func(t *testing.T) {
		defer func() { failed = t.Failed() }()
		ft := &fatalTB{TB: t}
		func() {
			defer func() { _ = recover() }()
			mongotest.New(ft, mongotest.WithReadinessTimeout(time.Second))
		}()
		if !ft.fataled.Load() {
			t.Error("New returned for a server that never answered")
		}
	})
	require.False(t, failed)
	require.Less(t, time.Since(start), 30*time.Second)
}

// mongoDefaultPort is the port of a MongoDB host given without one.
const mongoDefaultPort = "27017"

// proxyTarget returns the parsed MONGO_URI and the host:port a proxy must dial for it, skipping the test when MONGO_URI is unset
// or names a server the proxy cannot stand in for: SRV discovery or several hosts (it forwards one direct connection) and TLS (the
// client would verify the certificate against the proxy's address instead of the server's name). Options are read with the
// driver's own parser, so their names are case-insensitive and TLS counts as enabled however the URI turns it on (tls or ssl, or
// implicitly through a CA file or client certificate option).
func proxyTarget(tb testing.TB) (*url.URL, string) {
	tb.Helper()
	uri := os.Getenv(mongotest.DefaultEnv)
	if uri == "" {
		tb.Skip("MONGO_URI not set")
	}
	if !strings.HasPrefix(strings.ToLower(uri), connstring.SchemeMongoDB+"://") {
		tb.Skip("the proxy needs a direct connection to one server") // mongodb+srv resolves hosts through DNS
	}
	cs, err := connstring.ParseAndValidate(uri)
	require.NoError(tb, err)
	switch {
	case len(cs.Hosts) != 1 || !cs.DirectConnection:
		tb.Skip("the proxy needs a direct connection to one server")
	case cs.SSL:
		tb.Skip("the proxy cannot preserve TLS hostname verification")
	}
	u, err := url.Parse(uri)
	require.NoError(tb, err)
	upstream := cs.Hosts[0]
	if _, _, err := net.SplitHostPort(upstream); err != nil {
		upstream = net.JoinHostPort(strings.Trim(upstream, "[]"), mongoDefaultPort)
	}
	return u, upstream
}

// fatalTB records Fatalf instead of failing the test, and stops the caller with a panic like FailNow stops its goroutine.
type fatalTB struct {
	testing.TB
	fataled atomic.Bool
}

func (f *fatalTB) Fatalf(string, ...any) {
	f.fataled.Store(true)
	panic("fatal")
}

// slowProxy forwards TCP connections to upstream, holding the first slow ones for delay before forwarding, and returns its address.
func slowProxy(tb testing.TB, upstream string, slow int32, delay time.Duration) string {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if accepted.Add(1) <= slow {
					select {
					case <-time.After(delay):
					case <-tb.Context().Done():
						return
					}
				}
				up, err := net.Dial("tcp", upstream)
				if err != nil {
					return
				}
				defer func() { _ = up.Close() }()
				go func() { _, _ = io.Copy(up, conn) }()
				_, _ = io.Copy(conn, up)
			}()
		}
	}()
	return ln.Addr().String()
}

// TestNewReadinessFailureDoesNotWaitOnTeardown: a session-capable server that completes the handshake, fails the readiness ping
// and then never answers endSessions (which Disconnect sends first) still fails New promptly instead of hanging in teardown.
func TestNewReadinessFailureDoesNotWaitOnTeardown(t *testing.T) {
	// Serial: t.Setenv points the URI variable at the fake server.
	srv := newStallingServer(t)
	t.Setenv(mongotest.DefaultEnv, "mongodb://"+srv.addr()+"/?directConnection=true")

	ft := &fatalTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		mongotest.New(ft, mongotest.WithReadinessTimeout(5*time.Second))
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("New is still tearing down the unready client")
	}
	require.True(t, ft.fataled.Load(), "New returned for a server whose ping failed")
	require.Positive(t, srv.pings.Load(), "the readiness ping never reached the session-capable server")
}

// TestProxyTargetNormalizesTheAddress: the proxy dials MongoDB's default port for a host given without one and refuses URIs it
// cannot stand in for.
func TestProxyTargetNormalizesTheAddress(t *testing.T) {
	// Serial: t.Setenv sets the URI variable.
	tests := []struct {
		name, uri, upstream string
	}{
		{name: "no port", uri: "mongodb://localhost/?directConnection=true", upstream: "localhost:27017"},
		{name: "port", uri: "mongodb://db.example:27019/?replicaSet=rs0&directConnection=true", upstream: "db.example:27019"},
		{name: "ipv6 no port", uri: "mongodb://[::1]/?directConnection=true", upstream: "[::1]:27017"},
		{name: "not direct", uri: "mongodb://localhost:27017/"},
		{name: "several hosts", uri: "mongodb://a:1,b:2/?replicaSet=rs0"},
		{name: "srv", uri: "mongodb+srv://cluster.example/?directConnection=true"},
		{name: "mixed-case direct", uri: "mongodb://localhost/?DirectConnection=TRUE", upstream: "localhost:27017"},
		{name: "tls", uri: "mongodb://localhost/?directConnection=true&tls=true"},
		{name: "mixed-case ssl", uri: "mongodb://localhost/?directConnection=true&SSL=true"},
		{name: "mixed-case tls", uri: "mongodb://localhost/?directConnection=true&TLS=true"},
		{name: "implicit tls by ca file", uri: "mongodb://db.example/?directConnection=true&tlsCAFile=/path/ca.pem"},
		{name: "implicit tls by client certificate", uri: "mongodb://db.example/?directConnection=true&tlsCertificateKeyFile=/c.pem"},
		{name: "implicit tls by mixed-case ca file", uri: "mongodb://db.example/?directConnection=true&TLSCAFILE=/path/ca.pem"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(mongotest.DefaultEnv, tc.uri)
			var upstream string
			var skipped bool
			t.Run("target", func(t *testing.T) {
				defer func() { skipped = t.Skipped() }()
				_, upstream = proxyTarget(t)
			})
			if tc.upstream == "" {
				require.True(t, skipped, "proxy target %q for %s", upstream, tc.uri)
				return
			}
			require.False(t, skipped)
			require.Equal(t, tc.upstream, upstream)
		})
	}
}
