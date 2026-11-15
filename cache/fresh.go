// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import "context"

type freshReadsKey struct{}

// WithFreshReads marks ctx so that read-through calls made with it skip the
// cache and read the source of truth: [Cache.GetWithFallback],
// [GetWithFallbackT] and [ReadThrough] run the fill directly in the caller
// and store nothing. Use it for the load of a read-modify-write that checks
// an expected version or etag, so a stale cached copy can neither turn a
// valid etag into a conflict nor let a stale etag pass, and for reads right
// after a write that must observe it.
//
// The marker is inherited by every context derived from ctx. Get, GetManyRaw
// and the other plain reads ignore it: they have no source of truth to read.
func WithFreshReads(ctx context.Context) context.Context {
	return context.WithValue(ctx, freshReadsKey{}, true)
}

// FreshReads reports whether ctx was marked by [WithFreshReads].
func FreshReads(ctx context.Context) bool {
	v, ok := ctx.Value(freshReadsKey{}).(bool)
	return ok && v
}
