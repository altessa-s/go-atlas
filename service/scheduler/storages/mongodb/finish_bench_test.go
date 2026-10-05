// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongodb_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/service/scheduler/storagetest"
)

func BenchmarkFinishRun(b *testing.B) {
	storagetest.BenchmarkFinishRun(b, newClaimIT(b))
}
