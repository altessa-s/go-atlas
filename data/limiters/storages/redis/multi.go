// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrCrossSlot is wrapped by the [*storages.KeyError] of a multi-key call on
// a Redis Cluster or Ring client whose keys do not share one hash tag. One
// Lua script reaches one node, so every key of the call must hash alike;
// Redis would otherwise refuse the script with CROSSSLOT.
var ErrCrossSlot = errors.New("keys of a multi-key admission must share one Redis hash tag")

// AllowAll admits one request against every limit atomically; see
// [storages.MultiStorage.AllowAll]. One Lua script prunes and counts every
// window and adds the request to all of them only when each admits, so a
// refusal takes no slot. Windows have the millisecond resolution of
// [Provider.Allow].
//
// On a Redis Cluster or Ring client every key of the call must hash to the
// same node: give the keys one hash tag, such as "{tenant-1}:global" and
// "{tenant-1}:host:example.com" (a tag in the key prefix covers every key).
// A call whose keys hash differently fails before any network I/O with a
// [*storages.KeyError] for the first key that differs from the first one,
// wrapping [ErrCrossSlot]. A single-node or failover client accepts any keys.
func (p *Provider) AllowAll(ctx context.Context, limits []storages.Limit) ([]storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	if err := storages.ValidateLimits(limits); err != nil {
		return nil, err
	}

	keys := make([]string, len(limits))
	for i, l := range limits {
		keys[i] = p.Key(l.Key)
	}
	if shardsByKey(p.Client()) {
		if i := firstOtherHashKey(keys); i > 0 {
			return nil, &storages.KeyError{Index: i, Key: limits[i].Key, Err: ErrCrossSlot}
		}
	}

	args := make([]any, 0, 1+3*len(limits))
	args = append(args, time.Now().UnixMilli())
	for _, l := range limits {
		member, err := newMember()
		if err != nil {
			return nil, coreerrs.Wrap(err, "redis rate limit member generation")
		}
		args = append(args, max(l.Period.Milliseconds(), 1), l.Limit, member)
	}

	values, err := luaScriptMulti.Run(ctx, p.Client(), keys, args...).Slice()
	if err != nil {
		return nil, coreerrs.Wrap(err, "redis multi-key rate limit script error")
	}
	if len(values) != 1+2*len(limits) {
		return nil, errors.New("unexpected redis script result format")
	}

	infos := make([]storages.LimitInfo, len(limits))
	for i := range infos {
		infos[i] = storages.LimitInfo{Remaining: toInt64(values[1+2*i]), Reset: toInt64(values[2+2*i])}
	}
	if refused := toInt64(values[0]); refused > 0 {
		i := int(refused - 1)
		return infos, &storages.KeyError{Index: i, Key: limits[i].Key, Err: storages.ErrLimitExceeded}
	}
	return infos, nil
}

// shardsByKey reports whether client routes a command by the hash of its
// keys, so a multi-key script must keep its keys on one node.
func shardsByKey(client redis.UniversalClient) bool {
	switch client.(type) {
	case *redis.ClusterClient, *redis.Ring:
		return true
	default:
		return false
	}
}

// firstOtherHashKey returns the index of the first key whose hash key
// differs from that of keys[0], or 0 when every key hashes alike.
func firstOtherHashKey(keys []string) int {
	first := hashKey(keys[0])
	for i, k := range keys[1:] {
		if hashKey(k) != first {
			return i + 1
		}
	}
	return 0
}

// hashKey is the part of key that Redis Cluster and go-redis Ring hash: the
// text between the first "{" and the first "}" after it when non-empty,
// otherwise the whole key.
func hashKey(key string) string {
	start := strings.IndexByte(key, '{')
	if start < 0 {
		return key
	}
	end := strings.IndexByte(key[start+1:], '}')
	if end <= 0 {
		return key
	}
	return key[start+1 : start+1+end]
}

var _ storages.MultiStorage = (*Provider)(nil)
