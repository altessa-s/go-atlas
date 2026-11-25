// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"iter"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/internal/redis/redisutils"
)

// globEscaper escapes the Redis glob metacharacters of a literal key prefix
// for a SCAN MATCH pattern.
var globEscaper = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `]`, `\]`)

// scanBatch is the COUNT hint passed to SCAN. It bounds how many keys Redis
// examines per round trip during a stream; it is a hint, not a hard page size.
const scanBatch = 256

type rawSource struct {
	client redis.UniversalClient
	prefix string
}

// RawSource returns the bare revocation keys used by a mirror refresh.
func (s *Store) RawSource() mirror.Source {
	return rawSource{s.client, s.opts.keyPrefix}
}

// DataLoader returns the same bare keys for a probabilistic filter rebuild.
func (s *Store) DataLoader() probfilter.DataLoader {
	return rawSource{s.client, s.opts.keyPrefix}
}

// serverID verifies topology and identifies the Redis process serving this connection.
func serverID(ctx context.Context, conn *redis.Conn) (string, error) {
	if err := redisutils.VerifyStandalonePrimary(ctx, conn); err != nil {
		return "", err
	}
	info, err := conn.Info(ctx, "server").Result()
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(info) {
		if id, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "run_id:"); ok && id != "" {
			return id, nil
		}
	}
	return "", ErrUnsupportedClient
}

func (s rawSource) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	ctx = redis.WithoutCache(ctx)
	return func(yield func(string, error) bool) {
		client, ok := s.client.(*redis.Client)
		if !ok || client == nil {
			yield("", ErrUnsupportedClient)
			return
		}
		conn := client.Conn()
		defer func() { _ = conn.Close() }()
		id, err := serverID(ctx, conn)
		if err != nil {
			yield("", operationError{"verify scan source", errors.Join(ErrUnsupportedClient, err)})
			return
		}
		match := globEscaper.Replace(s.prefix) + "*"
		var cursor uint64
		for {
			keys, next, err := conn.Scan(ctx, cursor, match, scanBatch).Result()
			if err != nil {
				yield("", operationError{"scan revoked keys", err})
				return
			}
			// Validate before yielding even the final page. Drift invalidates the entire stream.
			current, err := serverID(ctx, conn)
			if err != nil || current != id {
				yield("", operationError{"verify scan source", errors.Join(ErrUnsupportedClient, err)})
				return
			}
			for _, k := range keys {
				if err := ctx.Err(); err != nil {
					yield("", operationError{"scan revoked keys", err})
					return
				}
				if key, ok := strings.CutPrefix(k, s.prefix); ok && !yield(key, nil) {
					return
				}
			}
			cursor = next
			if cursor == 0 {
				return
			}
		}
	}
}

// Count reports unknown size because counting would require a second scan.
func (rawSource) Count(context.Context) (int64, error) { return -1, nil }
