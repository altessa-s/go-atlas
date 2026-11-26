// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatemask

import (
	"context"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// Selection is the result of resolving a registered unary RPC's original mask.
type Selection struct {
	// Resource is the instance held by the request.
	Resource proto.Message
	// Spec is the registered specification used to resolve Mask.
	Spec *update.Spec
	// Mask contains the canonical scope for validation.
	Mask update.ResolvedMask
	// Err is the error from resolving the original mask, if resolution failed.
	Err error

	sent *fieldmaskpb.FieldMask
}

// OriginalMask returns a copy of the mask the extractor supplied, preserving
// omission, path spelling and whole-message replacement intent.
func (s Selection) OriginalMask() *fieldmaskpb.FieldMask {
	return proto.CloneOf(s.sent)
}

// SelectionFromContext returns the selection for req if a registered unary
// mask interceptor located its resource and examined the original mask.
func SelectionFromContext(ctx context.Context, req proto.Message) (Selection, bool) {
	sel := updatevalidation.FromContext(ctx, req)
	if sel == nil {
		return Selection{}, false
	}
	return Selection{Resource: sel.Resource, Spec: sel.Spec, Mask: sel.Mask, Err: sel.Err, sent: sel.OriginalMask}, true
}
