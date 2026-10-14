// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

// BenchmarkStreamReadMask filters 100 responses of one stream with the
// read_mask captured from its first request.
func BenchmarkStreamReadMask(b *testing.B) {
	ctx := b.Context()
	const method = "/x.v1.X/ListResources"
	newRI := func() *requestInterceptor {
		opts := newOptions()
		builtinUpdate, builtinRead := newBuiltinExtractors(nil, opts.metadataReadMaskHeader)
		ic := &interceptor{
			BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(interceptorName, nil, nil, opts.logger),
			opts:            opts,
			builtinUpdate:   builtinUpdate,
			builtinRead:     builtinRead,
		}
		ri := &requestInterceptor{
			interceptor: ic,
			meta:        sharedmetadata.NewCallMetadataFromMethod(ctx, method),
			kind:        KindRead,
		}
		ri.captureReadMask(ctx, &pb.GetResourceRequest{
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"id", "name", "description"}},
		})
		return ri
	}
	resp := &pb.Resource{Id: "id-1", Name: "kept", TenantId: "drop", Description: "kept"}

	b.ReportAllocs()
	for b.Loop() {
		ri := newRI()
		for range 100 {
			if err := ri.filterResponse(ctx, proto.Clone(resp), ri.capturedReadMask()); err != nil {
				b.Fatal(err)
			}
		}
	}
}
