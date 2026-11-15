// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfiltertest

import (
	"context"
	"fmt"

	"github.com/altessa-s/go-atlas/cache/probfilter"

	bloomstorages "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
)

// NewSharedBloom returns an in-memory Bloom storage shared between processes,
// like the Redis one: a rebuild first takes its lease, which a peer holds when
// peerRebuilding is set (BeginRebuild then fails with
// probfilter.ErrRebuildInProgress).
func NewSharedBloom(peerRebuilding bool) bloomstorages.Storage {
	return &sharedStorage{Storage: bloommemory.New(), peerRebuilding: peerRebuilding}
}

type sharedStorage struct {
	bloomstorages.Storage
	peerRebuilding bool
}

func (s *sharedStorage) BeginRebuild(context.Context) (bloomstorages.RebuildLease, error) {
	if s.peerRebuilding {
		return nil, fmt.Errorf("shared filter: %w", probfilter.ErrRebuildInProgress)
	}
	return sharedLease{s.Storage}, nil
}

type sharedLease struct{ storage bloomstorages.Storage }

func (l sharedLease) Stage(ctx context.Context, expectedItems int64) (bloomstorages.Staging, error) {
	return l.storage.Stage(ctx, expectedItems)
}

func (sharedLease) Release(context.Context) error { return nil }
