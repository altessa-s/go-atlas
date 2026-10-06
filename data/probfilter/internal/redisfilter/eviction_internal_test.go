// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

// An unsafe master must fail the check whichever order masters complete in,
// even when another master's policy cannot be read.
func TestPolicyCheck_UnsafeWinsOverUnreadable(t *testing.T) {
	t.Parallel()
	node := redis.NewClient(&redis.Options{Addr: "node:6379"})
	t.Cleanup(func() { _ = node.Close() })
	errDenied := errors.New("NOPERM")

	type obs struct {
		policy string
		err    error
	}
	unsafe, unread, safe := obs{policy: "allkeys-lru"}, obs{err: errDenied}, obs{policy: "noeviction"}
	for _, tc := range []struct {
		name         string
		order        []obs
		wantUnsafe   bool
		wantVerified bool
	}{
		{"unsafe_then_unreadable", []obs{unsafe, unread}, true, true},
		{"unreadable_then_unsafe", []obs{unread, unsafe}, true, true},
		{"unreadable_and_safe", []obs{unread, safe}, false, false},
		{"all_safe", []obs{safe, safe}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var pc policyCheck
			for _, o := range tc.order {
				pc.observe(node, o.policy, o.err)
			}
			verified, err := pc.result()
			require.Equal(t, tc.wantVerified, verified)
			if tc.wantUnsafe {
				require.ErrorIs(t, err, probfilter.ErrUnsafeEvictionPolicy)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// A check that observed no server, or could not enumerate the topology, is
// unverified; an unsafe policy still fails it.
func TestPolicyCheck_EnumerationFailure(t *testing.T) {
	t.Parallel()
	node := redis.NewClient(&redis.Options{Addr: "node:6379"})
	t.Cleanup(func() { _ = node.Close() })

	var none policyCheck
	verified, err := none.result()
	require.NoError(t, err)
	require.False(t, verified, "no server checked")

	var failed policyCheck
	failed.observe(node, "noeviction", nil)
	failed.fail()
	verified, err = failed.result()
	require.NoError(t, err)
	require.False(t, verified, "topology not enumerated")

	var unsafe policyCheck
	unsafe.fail()
	unsafe.observe(node, "allkeys-random", nil)
	_, err = unsafe.result()
	require.ErrorIs(t, err, probfilter.ErrUnsafeEvictionPolicy)
}
