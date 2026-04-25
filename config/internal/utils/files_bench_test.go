// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package utils_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/internal/utils"
)

func BenchmarkFindFile_Existing(b *testing.B) {
	// Use a file that exists on all systems.
	path := "/dev/null"
	b.ResetTimer()
	for b.Loop() {
		_ = utils.FindFile(path)
	}
}

func BenchmarkFindFile_Nonexistent(b *testing.B) {
	path := "/nonexistent/path/file.txt"
	b.ResetTimer()
	for b.Loop() {
		_ = utils.FindFile(path)
	}
}

func BenchmarkFindFile_Empty(b *testing.B) {
	for b.Loop() {
		_ = utils.FindFile("")
	}
}
