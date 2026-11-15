// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatemask

import (
	"context"

	"github.com/altessa-s/go-aip/fieldmask/extract"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// DefaultExtractor extends extract.DefaultUpdate to locate omitted masks.
// Its descriptor selection and resource ambiguity rules remain unchanged.
func DefaultExtractor(opts ...extract.Option) extract.UpdateFunc {
	fallback := extract.DefaultUpdate(opts...)
	return func(ctx context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		mask, resource, writeback, ok := fallback(ctx, req)
		if ok || req == nil {
			return mask, resource, writeback, ok
		}
		// Probe on a shallow reflection-built request to preserve resource identity
		// without modifying the caller's request, even temporarily.
		probe := req.ProtoReflect().New().Interface()
		req.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			probe.ProtoReflect().Set(fd, v)
			return true
		})
		if err := extract.SetUpdateMask(probe, &fieldmaskpb.FieldMask{}, opts...); err != nil {
			return nil, nil, nil, false
		}
		_, resource, _, ok = fallback(ctx, probe)
		if !ok {
			return nil, nil, nil, false
		}
		// The original fallback only misses a valid resource when its mask is absent.
		return nil, resource, func(m *fieldmaskpb.FieldMask) { _ = extract.SetUpdateMask(req, m, opts...) }, true
	}
}
