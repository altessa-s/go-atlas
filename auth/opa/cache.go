// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"encoding/binary"
	"encoding/hex"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/cache/lru"
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

// DecisionCache memoizes deterministic evaluations under opaque keys derived from
// the query, policy revision and parsed input. Implementations must be safe for
// concurrent use and bound their own retention. Managers may share a cache when
// a revision identifies the same policy and data across their sources.
type DecisionCache interface {
	Get(key string) (Result, bool)
	Put(key string, result Result)
}

func decisionCacheBackend(cache DecisionCache) DecisionCache {
	if nilcheck.IsNil(cache) {
		return nil
	}
	return cache
}

type decisionCache struct {
	entries *lru.ExpirableCache[string, Result]
}

func newDecisionCache(size int, ttl time.Duration) DecisionCache {
	if size <= 0 {
		size = DefaultDecisionCacheSize
	}
	if ttl <= 0 {
		ttl = DefaultDecisionCacheTTL
	}

	return &decisionCache{entries: lru.NewExpirableCache[string, Result](size, ttl)}
}

func (c *decisionCache) Get(key string) (Result, bool) {
	return c.entries.Get(key)
}

func (c *decisionCache) Put(key string, result Result) {
	c.entries.Put(key, result)
}

func (c *decisionCache) purge() { c.entries.Purge() }

// decisionCacheKey hashes the same parsed input passed to Rego.
func decisionCacheKey(query, revision string, input ast.Value) string {
	encoded, ok := appendInput(nil, input)
	if !ok {
		return ""
	}
	return hex.EncodeToString(keys.Digest([]byte(query), []byte(revision), encoded))
}

// Each node has a type tag. Strings and numbers carry their byte length;
// arrays and objects carry their element count. Object keys are in AST order.
func appendInput(dst []byte, input ast.Value) ([]byte, bool) {
	switch v := input.(type) {
	case ast.Null:
		return append(dst, '0'), true
	case ast.Boolean:
		if v {
			return append(dst, 't'), true
		}
		return append(dst, 'f'), true
	case ast.String:
		return appendScalar(dst, 's', string(v)), true
	case ast.Number:
		return appendScalar(dst, 'n', string(v)), true
	case *ast.Array:
		dst = binary.AppendUvarint(append(dst, 'a'), uint64(v.Len()))
		for i := range v.Len() {
			var ok bool
			dst, ok = appendInput(dst, v.Elem(i).Value)
			if !ok {
				return nil, false
			}
		}
	case ast.Object:
		dst = binary.AppendUvarint(append(dst, 'o'), uint64(v.Len()))
		for _, k := range v.Keys() {
			key, ok := k.Value.(ast.String)
			if !ok {
				return nil, false
			}
			dst = appendScalar(dst, 's', string(key))
			dst, ok = appendInput(dst, v.Get(k).Value)
			if !ok {
				return nil, false
			}
		}
	default:
		return nil, false
	}
	return dst, true
}

func appendScalar(dst []byte, tag byte, s string) []byte {
	dst = binary.AppendUvarint(append(dst, tag), uint64(len(s)))
	return append(dst, s...)
}
