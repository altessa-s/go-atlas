// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/lru"
)

const (
	// DefaultDecisionCacheTTL is how long a cached decision stays valid when
	// [WithDecisionCache] is given a non-positive TTL.
	DefaultDecisionCacheTTL = 5 * time.Minute

	// DefaultDecisionCacheSize is the entry cap applied when
	// [WithDecisionCache] is given a non-positive size. Decisions are small and
	// the key space is the set of distinct inputs, so this is sized to absorb a
	// busy service's working set without becoming a memory concern.
	DefaultDecisionCacheSize = 10000
)

// decisionCache memoizes policy evaluations keyed by (policy revision, input).
//
// It caches the *evaluation*, not the request handling: callers still record
// the decision with the audit recorder and still count it in metrics on a hit,
// because an authorization decision that is served but never recorded is a hole
// in the audit trail, not an optimization.
// Entries are stored by value, and every hit is handed a fresh *Result:
// [Result] has exported mutable fields, so sharing one pointer across hits
// would let a caller enriching the value corrupt every later evaluation — the
// same reason the evaluator never returns a shared singleton.
type decisionCache struct {
	entries *lru.ExpirableCache[string, Result]
}

func newDecisionCache(size int, ttl time.Duration) *decisionCache {
	if size <= 0 {
		size = DefaultDecisionCacheSize
	}
	if ttl <= 0 {
		ttl = DefaultDecisionCacheTTL
	}

	return &decisionCache{entries: lru.NewExpirableCache[string, Result](size, ttl)}
}

// get returns a copy of the cached decision for the input under revision.
//
// Denials is shared by pointer, which is safe because it is an ImmutableMap.
func (c *decisionCache) get(revision string, input any) (*Result, bool) {
	key, ok := decisionCacheKey(revision, input)
	if !ok {
		return nil, false
	}

	entry, ok := c.entries.Get(key)
	if !ok {
		return nil, false
	}

	return &entry, true
}

// put stores a decision. It is a no-op for inputs that have no stable key.
func (c *decisionCache) put(revision string, input any, result *Result) {
	key, ok := decisionCacheKey(revision, input)
	if !ok || result == nil {
		return
	}

	c.entries.Put(key, *result)
}

// purge drops every entry. Called on policy reload: the revision is part of the
// key, so stale entries are already unreachable — this only stops them from
// occupying the cache until they age out.
func (c *decisionCache) purge() {
	c.entries.Purge()
}

// decisionCacheKey derives a stable key from the policy revision and the
// evaluation input, reporting false when the input has no stable
// representation.
//
// The revision is part of the key rather than merely a purge trigger: it makes
// a decision from one policy bundle unreachable under another, so a reload
// racing an in-flight evaluation cannot surface an answer computed from
// policies that are no longer loaded.
//
// The input is hashed rather than stored: it can be arbitrarily large and often
// carries the whole request (tokens, claims, headers), which has no business
// sitting in memory longer than the evaluation needs it.
//
// Keying on the JSON encoding costs no generality: OPA marshals the input to
// JSON to evaluate it, so an input this cannot encode was never evaluable. The
// false return is a belt-and-braces path for encoders that disagree — the
// evaluation then simply runs uncached.
func decisionCacheKey(revision string, input any) (string, bool) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", false
	}

	h := sha256.New()

	// Length-prefix the revision so that it cannot run into the payload: without
	// it, revisions "a" + input `bc` and "ab" + input `c` would hash alike.
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(revision)))
	h.Write(length[:])
	h.Write([]byte(revision))
	h.Write(encoded)

	return hex.EncodeToString(h.Sum(nil)), true
}
