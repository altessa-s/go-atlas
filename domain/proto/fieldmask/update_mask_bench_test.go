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
