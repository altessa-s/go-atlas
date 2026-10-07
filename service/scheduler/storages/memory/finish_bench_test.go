// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/service/scheduler/storages/storagetest"
)

func BenchmarkFinishRun(b *testing.B) {
	storagetest.BenchmarkFinishRun(b, mustNew(b, 10))
}
