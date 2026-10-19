// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package utils_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/internal/utils"
)

func BenchmarkResolvePath_Absolute(b *testing.B) {
	for b.Loop() {
		_ = utils.ResolvePath("/etc/ssl/certs/ca.pem")
	}
}

func BenchmarkResolvePath_Relative(b *testing.B) {
	for b.Loop() {
		_ = utils.ResolvePath("certs/ca.pem")
	}
}

func BenchmarkResolvePath_Empty(b *testing.B) {
	for b.Loop() {
		_ = utils.ResolvePath("")
	}
}
