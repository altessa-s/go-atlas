// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import "context"

// storageSink adapts an audit Storage to the async.Sink[*Event] interface.
// Single-element batches use Store; larger batches use StoreBatch so storage
// implementations can leverage batch APIs (e.g. Mongo InsertMany).
type storageSink struct {
	storage Storage
}

func (s storageSink) StoreBatch(ctx context.Context, items []*Event) error {
	if len(items) == 1 {
		return s.storage.Store(ctx, items[0])
	}
	return s.storage.StoreBatch(ctx, items)
}
