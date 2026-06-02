// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func BenchmarkExtractUpdateMask(b *testing.B) {
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "t"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "tenant_id"}},
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = fieldmask.ExtractUpdateMask(req)
	}
}

func BenchmarkExtractReadMask(b *testing.B) {
	req := &pb.GetResourceRequest{
		Name:     "id",
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "tenant_id"}},
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = fieldmask.ExtractReadMask(req)
	}
}

func BenchmarkSetUpdateMask(b *testing.B) {
	cleaned := &fieldmaskpb.FieldMask{Paths: []string{"name"}}

	b.ReportAllocs()
	for b.Loop() {
		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "create_time"}},
		}
		_ = fieldmask.SetUpdateMask(req, cleaned)
	}
}
