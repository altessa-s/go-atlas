// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// purgeExpired reclaims both expired records and tombstones. Limiting the
// stream purge to the observed revision preserves concurrent replacements.
func (s *Storage) purgeExpired(ctx context.Context) (int64, error) {
	cutoff := s.clock.Now().Add(-s.cleanupGrace)
	w, err := s.KV().WatchAll(ctx)
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "scan request IDs")
	}
	defer func() { _ = w.Stop() }()
	var count int64
	for {
		item, err := next(ctx, w)
		if err != nil || item == nil {
			return count, err
		}
		if item.Operation() == jetstream.KeyValuePut {
			current, decodeErr := decode(item.Value())
			if decodeErr != nil {
				return count, decodeErr
			}
			if cutoff.Before(current.ExpiresAt) {
				continue
			}
		}
		subject := "$KV." + item.Bucket() + "." + item.Key()
		err = s.stream.Purge(ctx, jetstream.WithPurgeSubject(subject), jetstream.WithPurgeSequence(item.Revision()+1))
		if err != nil {
			return count, coreerrs.WrapOperation(err, "purge expired request ID revision")
		}
		count++
	}
}
