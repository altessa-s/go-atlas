// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas-core/id"
	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/internal/redis/redisutils"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	limiters "github.com/altessa-s/go-atlas/coord/limiters"
)

// ErrCrossSlot is wrapped by the [*storages.KeyError] of a multi-key call on
// an unverified or sharded client whose keys do not share one hash tag. One
// Lua script reaches one node, so every key of the call must hash alike;
// Redis would otherwise refuse the script with CROSSSLOT.
var ErrCrossSlot = errors.New("keys of a multi-key admission must share one Redis hash tag")

// DefaultTopologyVerificationWindow is how long AllowAll reuses a successful
// standalone-primary check. Thirty seconds amortizes two Redis round trips
// while bounding the time a provider can rely on an old role or a virtual IP
// that has been repointed during failover.
const DefaultTopologyVerificationWindow = 30 * time.Second

type primaryVerification struct {
	client    *redis.Client
	checkedAt time.Time
}

// AllowAll admits one request against every limit atomically; see
// [storages.MultiStorage.AllowAll]. One Lua script prunes and counts every
// window and adds the request to all of them only when each admits, so a
// refusal takes no slot. Windows have the millisecond resolution of
// [Provider.Allow].
//
// Give the keys one shared hash tag, such as "{tenant-1}:global" and
// "{tenant-1}:host:example.com". A shared tag in the prefix also suffices.
// For untagged or differently tagged keys, a raw *redis.Client must prove
// its endpoint is a single non-cluster primary. The provider shares concurrent
// checks and reuses success for [DefaultTopologyVerificationWindow].
// All other client types return a [*storages.KeyError] wrapping [ErrCrossSlot]
// before any network I/O.
func (p *Provider) AllowAll(ctx context.Context, limits []storages.Limit) ([]storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	if err := storages.ValidateLimits(limits); err != nil {
		return nil, err
	}

	keys := make([]string, len(limits))
	for i, l := range limits {
		keys[i] = p.Key(l.Key)
	}
	if i := firstOtherHashTag(keys); i > 0 {
		client, ok := p.Client().(*redis.Client)
		if !ok {
			return nil, &storages.KeyError{Index: i, Key: limits[i].Key, Err: ErrCrossSlot}
		}
		if err := p.verifyPrimary(ctx, client); err != nil {
			return nil, &storages.KeyError{Index: i, Key: limits[i].Key, Err: fmt.Errorf("%w: %w", ErrCrossSlot, err)}
		}
	}

	args := make([]any, 0, 1+3*len(limits))
	args = append(args, p.clock.Now().UnixMilli())
	for _, l := range limits {
		member := id.NewHex(memberBytes)
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
		return infos, &storages.KeyError{Index: i, Key: limits[i].Key, Err: limiters.ErrLimitExceeded}
	}
	return infos, nil
}

func (p *Provider) verifyPrimary(ctx context.Context, client *redis.Client) error {
	if p.recentlyVerified(client, p.clock.Now()) {
		return nil
	}
	_, err := p.verification.Do(ctx, client, func(ctx context.Context) (struct{}, error) {
		// Another flight may have completed between the cache read and Do.
		now := p.clock.Now()
		if p.recentlyVerified(client, now) {
			return struct{}{}, nil
		}
		p.verified.Store(nil)
		err := redisutils.VerifyStandalonePrimary(ctx, client)
		if err == nil {
			p.verified.Store(&primaryVerification{client: client, checkedAt: now})
		}
		return struct{}{}, err
	})
	return err
}

func (p *Provider) recentlyVerified(client *redis.Client, now time.Time) bool {
	v := p.verified.Load()
	if v == nil || v.client != client {
		return false
	}
	age := now.Sub(v.checkedAt)
	return age >= 0 && age < DefaultTopologyVerificationWindow
}

// firstOtherHashTag returns the index of the first key whose hash tag
// differs from that of keys[0], or 0 when no shared tag is required or all keys share one.
func firstOtherHashTag(keys []string) int {
	first := hashTag(keys[0])
	for i, k := range keys[1:] {
		if first == "" || hashTag(k) != first {
			return i + 1
		}
	}
	return 0
}

// hashTag returns the
// text between the first "{" and the first "}" after it when non-empty,
// otherwise an empty string.
func hashTag(key string) string {
	start := strings.IndexByte(key, '{')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(key[start+1:], '}')
	if end <= 0 {
		return ""
	}
	return key[start+1 : start+1+end]
}

var _ storages.MultiStorage = (*Provider)(nil)
