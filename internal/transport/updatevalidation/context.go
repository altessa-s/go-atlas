// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatevalidation

import (
	"context"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type selectionKey struct{}
type deferErrorKey struct{}

// Selection holds the input and result of mask resolution for a single RPC.
type Selection struct {
	Request      proto.Message
	Resource     proto.Message
	Route        []protoreflect.FieldDescriptor
	Spec         *update.Spec
	Mask         update.ResolvedMask
	OriginalMask *fieldmaskpb.FieldMask
	Err          error
	MaskError    error
}

// FromContext returns the selection for the given request instance.
func FromContext(ctx context.Context, req proto.Message) *Selection {
	sel, _ := ctx.Value(selectionKey{}).(*Selection)
	if sel == nil || sel.Request != req {
		return nil
	}
	return sel
}

// WithSelection publishes a selection after a mask interceptor locates the resource.
func WithSelection(ctx context.Context, sel *Selection) context.Context {
	if sel == nil || FromContext(ctx, sel.Request) == sel {
		return ctx
	}
	return context.WithValue(ctx, selectionKey{}, sel)
}

// DeferMaskError allows a mask interceptor to publish a refusal to a downstream
// validator. The factory uses it only when that validator follows the interceptor.
func DeferMaskError(ctx context.Context) context.Context {
	return context.WithValue(ctx, deferErrorKey{}, true)
}

// DefersMaskError reports whether a downstream validator will handle a refusal.
func DefersMaskError(ctx context.Context) bool {
	return ctx.Value(deferErrorKey{}) == true
}
