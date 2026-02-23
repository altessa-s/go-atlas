// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redisbase provides common utilities for Redis-based storage implementations.
//
// This package extracts shared patterns from Redis storage providers,
// including client management and key building.
//
// # Base Provider
//
// The [Base] type encapsulates the common Redis provider fields:
//
//	type MyProvider struct {
//	    redisbase.Base
//	    opts *options
//	}
//
//	func New(client redis.UniversalClient, opt ...Option) *MyProvider {
//	    opts := newOptions(opt...)
//	    return &MyProvider{
//	        Base: redisbase.NewBase(client, opts.keyPrefix),
//	        opts: opts,
//	    }
//	}
//
//	func (p *MyProvider) Get(ctx context.Context, key string) ([]byte, error) {
//	    return p.Client().Get(ctx, p.Key(key)).Bytes()
//	}
//
// # Key Building
//
// The Base type includes a KeyBuilder for consistent key prefixing:
//
//	key := p.Key("user:123")     // Returns "prefix:user:123"
//	keys := p.Keys("a", "b")     // Returns ["prefix:a", "prefix:b"]
package redisbase
