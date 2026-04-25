// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/health"
)

func BenchmarkToProto(b *testing.B) {
	for b.Loop() {
		ToProto(health.StatusServing)
	}
}
