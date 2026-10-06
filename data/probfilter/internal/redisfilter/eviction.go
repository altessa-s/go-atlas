// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

// CheckEvictionPolicy verifies that Redis never evicts the keys a shared
// filter depends on: the filter, its rebuild metadata and delete records
// carry no TTL, so an allkeys-* maxmemory-policy may drop them and silently
// break the rebuild and delete guarantees. Every master of a cluster is
// checked.
//
// It returns an error wrapping [probfilter.ErrUnsafeEvictionPolicy] for an
// allkeys-* policy. When the policy cannot be read — managed Redis services
// often deny CONFIG — it returns verified false and no error, leaving the
// decision to the caller.
func CheckEvictionPolicy(ctx context.Context, client redis.UniversalClient) (verified bool, err error) {
	var pc policyCheck
	switch c := client.(type) {
	case *redis.ClusterClient:
		// Every master is checked even after one fails or is unsafe:
		// ForEachMaster reports only the first callback error, so results are
		// collected here instead of returned.
		if err := c.ForEachMaster(ctx, func(ctx context.Context, m *redis.Client) error {
			policy, err := policyOf(ctx, m)
			pc.observe(m, policy, err)
			return nil
		}); err != nil {
			pc.fail() // the topology could not be enumerated
		}
	case *redis.Client:
		policy, err := policyOf(ctx, c)
		pc.observe(c, policy, err)
	default:
		return false, nil
	}
	return pc.result()
}

// policyOf reads the maxmemory-policy of one server.
func policyOf(ctx context.Context, c *redis.Client) (string, error) {
	policy, err := c.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return "", err
	}
	p, ok := policy["maxmemory-policy"]
	if !ok || p == "" {
		return "", errNoPolicy
	}
	return p, nil
}

// errNoPolicy reports a CONFIG GET reply without a maxmemory-policy value.
var errNoPolicy = errors.New("no maxmemory-policy in CONFIG GET reply")

// policyCheck aggregates the policies of all checked servers: one unsafe
// policy fails the check whatever the others report, and one unreadable
// policy, a failed topology enumeration or no server checked at all (with
// none unsafe) leaves it unverified. It is safe for concurrent use.
type policyCheck struct {
	mu       sync.Mutex
	unsafe   error
	unread   bool
	observed int
}

// fail records that some servers could not be checked.
func (p *policyCheck) fail() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.unread = true
}

func (p *policyCheck) observe(c *redis.Client, policy string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.observed++
	switch {
	case err != nil:
		p.unread = true
	case strings.HasPrefix(policy, "allkeys-") && p.unsafe == nil:
		p.unsafe = fmt.Errorf("%w: maxmemory-policy %s on %s", probfilter.ErrUnsafeEvictionPolicy, policy, c.Options().Addr)
	}
}

func (p *policyCheck) result() (verified bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unsafe != nil {
		return true, p.unsafe
	}
	return !p.unread && p.observed > 0, nil
}
