// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/health"
)

func FuzzToProto(f *testing.F) {
	f.Add(int32(0))
	f.Add(int32(1))
	f.Add(int32(2))
	f.Add(int32(3))
	f.Add(int32(99))

	f.Fuzz(func(t *testing.T, s int32) {
		// Should not panic for any input
		_ = ToProto(health.ServingStatus(s))
	})
}
