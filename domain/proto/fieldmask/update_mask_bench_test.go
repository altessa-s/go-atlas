// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// BenchmarkApplyUpdateMask_HappyPath measures the steady-state cost of
// ApplyUpdateMask on a typical multi-path mask. The indexed-repeated check
// added for AIP-161 sits on this hot path and must stay cheap.
func BenchmarkApplyUpdateMask_HappyPath(b *testing.B) {
	paths := []string{"name", "description", "profile.display_name"}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(&pb.Resource{
			Name:        "n",
			Description: "d",
			Profile:     &pb.Profile{DisplayName: "p"},
		})
	}
}

// BenchmarkApplyUpdateMask_RejectIndexed measures the rejection path so we
// can spot regressions in the cost of producing the ValidationError.
func BenchmarkApplyUpdateMask_RejectIndexed(b *testing.B) {
	mask := fieldmask.FromPaths("aliases.0.display_name")
	msg := &pb.Resource{}

	b.ReportAllocs()
	for b.Loop() {
		_ = mask.ApplyUpdateMask(msg)
	}
}

// BenchmarkApplyUpdateMask_WithPathValidation measures the happy path with
// WithPathValidation enabled, so the extra Validate pass on a valid mask stays
// cheap relative to the unvalidated steady state.
func BenchmarkApplyUpdateMask_WithPathValidation(b *testing.B) {
	paths := []string{"name", "description", "profile.display_name"}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(&pb.Resource{
			Name:        "n",
			Description: "d",
			Profile:     &pb.Profile{DisplayName: "p"},
		}, fieldmask.WithPathValidation(fieldmask.PathValidationEnforce))
	}
}

// BenchmarkApplyUpdateMask_CollectionSubPaths measures a mask whose paths
// reach into collection elements, which ApplyUpdateMask checks against the
// element schema and strips of OUTPUT_ONLY leaves.
func BenchmarkApplyUpdateMask_CollectionSubPaths(b *testing.B) {
	paths := []string{"labels.k.display_name", "labels.k.updated_at", "aliases.*.display_name", "aliases.updated_at"}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(&pb.Resource{
			Aliases: []*pb.Profile{{Id: "a", DisplayName: "x"}},
			Labels:  map[string]*pb.Profile{"k": {Id: "l", DisplayName: "y"}},
		})
	}
}

// BenchmarkFromSetFieldsForUpdate measures the mask synthesized for an empty
// update_mask on a resource with a populated sub-message and collections.
func BenchmarkFromSetFieldsForUpdate(b *testing.B) {
	res := &pb.Resource{
		Id:          "id-1",
		Description: "d",
		Profile:     &pb.Profile{Id: "p-1", DisplayName: "Ann", UpdatedAt: "t"},
		Aliases:     []*pb.Profile{{Id: "a-1"}},
		Labels:      map[string]*pb.Profile{"k": {Id: "l-1"}},
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = fieldmask.FromSetFieldsForUpdate(res)
	}
}
