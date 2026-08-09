// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelectit_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/tests/integration/leadelectit"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

const (
	// electionTTL is the lease this suite elects with. It bounds how long a
	// node keeps believing it leads after it stops being able to renew, and
	// therefore how quickly a partitioned node self-demotes.
	electionTTL = 2 * time.Second

	// keyExpiry is the server-side lifetime of the election key. It is the
	// bucket's max-age, fixed by the provider rather than derived from
	// electionTTL, so it — not the lease — is what bounds a failover after a
	// holder dies without resigning.
	keyExpiry = lenats.DefaultBucketKeysTTL

	// samplingInterval is the Observer cadence: well below electionTTL so a
	// handover is sampled many times while it is in flight.
	samplingInterval = 25 * time.Millisecond
)

// natsURL returns the broker address, matching tests/integration/docker-compose.yml.
func natsURL() string {
	if v := os.Getenv("NATS_URL"); v != "" {
		return v
	}
	return "nats://127.0.0.1:14222"
}

// uniqueSuffix names a throwaway bucket so two runs against the same server —
// or two tests in one run — never collide.
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// connect dials the broker, skipping the test when it is unreachable so a
// developer without the compose stack running still gets a green build.
func connect(tb testing.TB) *nats.Conn {
	tb.Helper()

	nc, err := nats.Connect(natsURL(),
		nats.Timeout(2*time.Second),
		nats.RetryOnFailedConnect(false),
		// The suite kills connections on purpose; reconnecting behind its back
		// would mask the very failover it is measuring.
		nats.NoReconnect(),
	)
	if err != nil {
		tb.Skipf("NATS unreachable at %s (%v) — start it with: docker compose -f tests/integration/docker-compose.yml up -d nats", natsURL(), err)
	}
	tb.Cleanup(nc.Close)

	return nc
}

// node is one elector in the scenario, with the connection it rides on so a
// test can sever that connection without going through Stop.
type node struct {
	id      string
	conn    *nats.Conn
	elector *leadelect.Leader
}

// kill severs the node's link to the broker without resigning, standing in for
// a process that dies or is partitioned away. The lease is left behind for the
// server to age out.
func (n *node) kill() { n.conn.Close() }

// cluster is a set of electors competing for one key, each on its own
// connection.
type cluster struct {
	key   string
	nodes []*node
}

// clusterOption configures an elector before its election starts. Registration
// has to happen before Start: leadership is often acquired within milliseconds,
// and a callback registered after the transition is simply never called.
type clusterOption func(le *leadelect.Leader)

// newCluster starts size electors on a bucket of its own.
func newCluster(tb testing.TB, size int, opts ...clusterOption) *cluster {
	tb.Helper()

	key := "election-" + uniqueSuffix()
	c := &cluster{key: key}

	for i := range size {
		id := fmt.Sprintf("node-%d", i)
		conn := connect(tb)

		prov, err := lenats.New(tb.Context(), conn, lenats.WithBucket(key))
		require.NoError(tb, err)

		le := leadelect.New(prov, key, id, leadelect.WithTTL(electionTTL))
		for _, opt := range opts {
			opt(le)
		}
		require.NoError(tb, le.Start(tb.Context()))

		// Stop before the connection Cleanup closes it, so the final resign
		// still has a broker to talk to.
		tb.Cleanup(func() { _ = le.Stop(tb.Context()) })

		c.nodes = append(c.nodes, &node{id: id, conn: conn, elector: le})
	}

	return c
}

// electors adapts the cluster for the Observer.
func (c *cluster) electors() []leadelectit.Elector {
	out := make([]leadelectit.Elector, 0, len(c.nodes))
	for _, n := range c.nodes {
		out = append(out, n.elector)
	}

	return out
}

// leader returns the single node currently claiming leadership, failing when
// the count is anything but one.
func (c *cluster) leader(tb testing.TB) *node {
	tb.Helper()

	var found []*node
	for _, n := range c.nodes {
		if n.elector.IsLeader() {
			found = append(found, n)
		}
	}
	require.Len(tb, found, 1, "expected exactly one leader, got %d", len(found))

	return found[0]
}

// awaitLeader blocks until some node claims leadership.
func (c *cluster) awaitLeader(tb testing.TB, within time.Duration) *node {
	tb.Helper()

	var elected *node
	require.Eventually(tb, func() bool {
		for _, n := range c.nodes {
			if n.elector.IsLeader() {
				elected = n
				return true
			}
		}
		return false
	}, within, samplingInterval, "no leader emerged within %v", within)

	return elected
}

// awaitLeaderOtherThan blocks until a node that is not excluded claims
// leadership, and returns it.
func (c *cluster) awaitLeaderOtherThan(tb testing.TB, excluded string, within time.Duration) *node {
	tb.Helper()

	var elected *node
	require.Eventually(tb, func() bool {
		for _, n := range c.nodes {
			if n.id != excluded && n.elector.IsLeader() {
				elected = n
				return true
			}
		}
		return false
	}, within, samplingInterval, "no successor to %s emerged within %v", excluded, within)

	return elected
}
