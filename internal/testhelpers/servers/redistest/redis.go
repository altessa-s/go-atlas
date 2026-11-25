// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redistest

import (
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"

	goredis "github.com/redis/go-redis/v9"
)

// RedisClient starts an in-process miniredis server and returns a go-redis
// client connected to it, together with the server handle for direct data
// manipulation (seeding keys, FastForward, FlushAll). Both the client and
// the server are shut down automatically via tb.Cleanup.
func RedisClient(tb testing.TB) (*goredis.Client, *miniredis.Miniredis) {
	tb.Helper()

	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	tb.Cleanup(func() { _ = client.Close() })
	return client, mr
}

// ServeTopology supplies ROLE and INFO cluster, which miniredis does not
// implement. Other commands keep their usual behavior.
func ServeTopology(mr *miniredis.Miniredis, role, clusterEnabled string) {
	mr.Server().SetPreHook(func(c *server.Peer, cmd string, args ...string) bool {
		switch {
		case cmd == "ROLE":
			const primaryRoleFields = 3 // role, replication offset, connected replicas
			c.WriteLen(primaryRoleFields)
			c.WriteBulk(role)
			c.WriteInt(0)
			c.WriteLen(0)
		case cmd == "INFO" && len(args) == 1 && strings.EqualFold(args[0], "cluster"):
			c.WriteBulk("# Cluster\r\ncluster_enabled:" + clusterEnabled + "\r\n")
		default:
			return false
		}
		return true
	})
}
