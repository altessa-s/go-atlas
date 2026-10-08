// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// readMaskKey is the context key under which a KindRead call exposes its
// per-call interceptor state to the handler.
type readMaskKey struct{}

// ReadMaskFromContext returns the AIP-157 read mask the interceptor captured
// for the current KindRead call, so a handler can push the selection down to
// storage (see data/projection) instead of loading the full resource:
//
//	if mask, ok := fieldmask.ReadMaskFromContext(ctx); ok {
//	    spec, err := parser.ParsePaths(ctx, mask.GetPaths())
//	    …
//	}
//
// The mask is resolved by the same extractor chain the response filter uses
// and is captured from the first request of the call — on a server stream,
// call it after the first Recv. It returns ok=false when the interceptor is
// not installed, the method is not KindRead, the call is skipped, or the
// request carries no mask (AIP-157: all fields). The returned mask is a copy
// the caller may modify.
//
// The interceptor still filters the response afterwards; on a response that
// storage already trimmed to the same paths the filter is a no-op.
func ReadMaskFromContext(ctx context.Context) (*fieldmaskpb.FieldMask, bool) {
	ri, ok := ctx.Value(readMaskKey{}).(*requestInterceptor)
	if !ok {
		return nil, false
	}
	mask := ri.capturedReadMask()
	if mask == nil {
		return nil, false
	}
	clone, ok := proto.Clone(mask).(*fieldmaskpb.FieldMask)
	return clone, ok
}
